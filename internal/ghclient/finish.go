package ghclient

import (
	"context"
	"fmt"

	"github.com/google/go-github/v92/github"

	"github.com/justin-efficient/enzo/internal/gitrepo"
)

// Readiness is everything `enzo finish` has to know before it merges: whether
// the branches combine, whether review is satisfied, whether the pull
// request's own checks passed, and what state the base branch is in.
//
// The zero values are all "GitHub said nothing about this", which for most of
// these is the normal answer on a repository with no review rules and no CI.
type Readiness struct {
	// Mergeable is MERGEABLE, CONFLICTING or UNKNOWN. UNKNOWN means GitHub is
	// still working it out; it settles a moment later.
	Mergeable string
	// MergeState is the fuller picture: CLEAN, BLOCKED, BEHIND, UNSTABLE,
	// DIRTY, DRAFT, HAS_HOOKS or UNKNOWN.
	MergeState string

	// ReviewDecision is APPROVED, CHANGES_REQUESTED or REVIEW_REQUIRED, and
	// empty when the repository requires no review at all.
	ReviewDecision string
	// Reviewers are the logins and team slugs GitHub has asked to review.
	Reviewers []string

	// Checks is the rollup over the pull request's head commit: SUCCESS,
	// FAILURE, PENDING, ERROR or EXPECTED. It is empty when the commit has no
	// checks, which is not the same as passing.
	Checks string
	// Failed and Pending name the checks in those states, for a report that
	// says which one rather than only that one of them did.
	Failed  []string
	Pending []string

	// BaseBranch is the branch the pull request merges into, and BaseChecks
	// the rollup over its head commit — the "has main been built, and did it
	// pass" question. BaseHead is that commit, abbreviated.
	BaseBranch string
	BaseHead   string
	BaseChecks string
}

// readinessQuery asks for all five of `enzo finish`'s questions at once. They
// are one round trip because they are one decision.
const readinessQuery = `
query($owner:String!, $name:String!, $number:Int!) {
  repository(owner:$owner, name:$name) {
    pullRequest(number:$number) {
      mergeable
      mergeStateStatus
      reviewDecision
      reviewRequests(first:25) {
        nodes { requestedReviewer {
          __typename
          ... on User { login }
          ... on Team { slug }
        } }
      }
      commits(last:1) { nodes { commit { statusCheckRollup {
        state
        contexts(first:50) { nodes {
          __typename
          ... on CheckRun { name conclusion status }
          ... on StatusContext { context state }
        } }
      } } } }
    }
    defaultBranchRef {
      name
      target { ... on Commit { oid statusCheckRollup { state } } }
    }
  }
}`

// Readiness reports what stands between a pull request and being merged.
func (a *API) Readiness(ctx context.Context, slug gitrepo.Slug, number int) (Readiness, error) {
	var out struct {
		Repository struct {
			PullRequest struct {
				Mergeable        string `json:"mergeable"`
				MergeStateStatus string `json:"mergeStateStatus"`
				ReviewDecision   string `json:"reviewDecision"`
				ReviewRequests   struct {
					Nodes []struct {
						RequestedReviewer struct {
							Login string `json:"login"`
							Slug  string `json:"slug"`
						} `json:"requestedReviewer"`
					} `json:"nodes"`
				} `json:"reviewRequests"`
				Commits struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								State    string `json:"state"`
								Contexts struct {
									Nodes []struct {
										Type       string `json:"__typename"`
										Name       string `json:"name"`
										Conclusion string `json:"conclusion"`
										Status     string `json:"status"`
										Context    string `json:"context"`
										State      string `json:"state"`
									} `json:"nodes"`
								} `json:"contexts"`
							} `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"pullRequest"`
			DefaultBranchRef struct {
				Name   string `json:"name"`
				Target struct {
					Oid               string `json:"oid"`
					StatusCheckRollup *struct {
						State string `json:"state"`
					} `json:"statusCheckRollup"`
				} `json:"target"`
			} `json:"defaultBranchRef"`
		} `json:"repository"`
	}

	vars := map[string]any{"owner": slug.Owner, "name": slug.Name, "number": number}
	if err := a.graphql(ctx, readinessQuery, vars, &out); err != nil {
		return Readiness{}, fmt.Errorf("could not read %s#%d: %w", slug, number, err)
	}

	pr := out.Repository.PullRequest
	r := Readiness{
		Mergeable:      pr.Mergeable,
		MergeState:     pr.MergeStateStatus,
		ReviewDecision: pr.ReviewDecision,
	}
	for _, n := range pr.ReviewRequests.Nodes {
		if who := n.RequestedReviewer.Login; who != "" {
			r.Reviewers = append(r.Reviewers, who)
		} else if who := n.RequestedReviewer.Slug; who != "" {
			r.Reviewers = append(r.Reviewers, who)
		}
	}

	if len(pr.Commits.Nodes) > 0 {
		if rollup := pr.Commits.Nodes[0].Commit.StatusCheckRollup; rollup != nil {
			r.Checks = rollup.State
			for _, c := range rollup.Contexts.Nodes {
				// A CheckRun carries a conclusion once it finishes and a
				// status until then; a StatusContext has only a state.
				name, outcome := c.Name, c.Conclusion
				if name == "" {
					name, outcome = c.Context, c.State
				}
				if outcome == "" {
					outcome = c.Status
				}
				switch outcome {
				case "SUCCESS", "NEUTRAL", "SKIPPED":
				case "QUEUED", "IN_PROGRESS", "PENDING", "WAITING", "EXPECTED", "REQUESTED":
					r.Pending = append(r.Pending, name)
				default:
					r.Failed = append(r.Failed, name)
				}
			}
		}
	}

	base := out.Repository.DefaultBranchRef
	r.BaseBranch = base.Name
	if oid := base.Target.Oid; len(oid) >= 7 {
		r.BaseHead = oid[:7]
	}
	if rollup := base.Target.StatusCheckRollup; rollup != nil {
		r.BaseChecks = rollup.State
	}
	return r, nil
}

// MarkReadyForReview takes a pull request out of draft.
//
// This is the mutation REST has no equivalent for. `PATCH /pulls/{n}` accepts
// "draft": false, answers 200, and leaves the pull request a draft.
func (a *API) MarkReadyForReview(ctx context.Context, nodeID string) error {
	const mutation = `
mutation($id:ID!) {
  markPullRequestReadyForReview(input:{pullRequestId:$id}) {
    pullRequest { number isDraft }
  }
}`
	var out struct {
		MarkPullRequestReadyForReview struct {
			PullRequest struct {
				Number  int  `json:"number"`
				IsDraft bool `json:"isDraft"`
			} `json:"pullRequest"`
		} `json:"markPullRequestReadyForReview"`
	}
	if err := a.graphql(ctx, mutation, map[string]any{"id": nodeID}, &out); err != nil {
		return fmt.Errorf("could not take the pull request out of draft: %w", err)
	}
	// Asserted rather than assumed, because the REST route for this fails by
	// reporting success.
	if out.MarkPullRequestReadyForReview.PullRequest.IsDraft {
		return fmt.Errorf("GitHub accepted the request but the pull request is still a draft")
	}
	return nil
}

// MergePullRequest merges a pull request. The commit message is left to
// GitHub, which composes it from the title and the repository's settings.
func (a *API) MergePullRequest(ctx context.Context, slug gitrepo.Slug, number int) error {
	res, _, err := a.gh.PullRequests.Merge(ctx, slug.Owner, slug.Name, number, "",
		&github.PullRequestOptions{})
	if err != nil {
		return wrap(err, fmt.Sprintf("could not merge %s#%d", slug, number))
	}
	if !res.GetMerged() {
		return fmt.Errorf("GitHub refused to merge %s#%d: %s", slug, number, res.GetMessage())
	}
	return nil
}
