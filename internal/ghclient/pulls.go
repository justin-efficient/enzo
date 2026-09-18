package ghclient

import (
	"context"
	"fmt"

	"github.com/google/go-github/v92/github"

	"github.com/justin-efficient/enzo/internal/gitrepo"
)

// PullRequest is the subset of a GitHub pull request enzo works with.
type PullRequest struct {
	Number int
	Title  string
	State  string
	URL    string
	Draft  bool
	// Head is the branch the PR merges from, Base the one it merges into.
	Head string
	Base string
}

// NewPullRequest describes a pull request to open.
type NewPullRequest struct {
	Title string
	Body  string
	Head  string
	Base  string
	Draft bool
}

// DefaultBranch returns the branch new pull requests should target.
func (a *API) DefaultBranch(ctx context.Context, slug gitrepo.Slug) (string, error) {
	repo, _, err := a.gh.Repositories.Get(ctx, slug.Owner, slug.Name)
	if err != nil {
		return "", wrap(err, fmt.Sprintf("could not read %s", slug))
	}
	if repo.GetDefaultBranch() == "" {
		return "", fmt.Errorf("%s reports no default branch", slug)
	}
	return repo.GetDefaultBranch(), nil
}

// PullRequestForBranch returns the open pull request whose head is branch, or
// nil when there is none. A closed one does not count: GitHub will happily
// open a second PR from the same branch, and `enzo start` should.
func (a *API) PullRequestForBranch(ctx context.Context, slug gitrepo.Slug, branch string) (*PullRequest, error) {
	opts := &github.PullRequestListOptions{
		State:       "open",
		Head:        slug.Owner + ":" + branch,
		ListOptions: github.ListOptions{PerPage: 10},
	}
	prs, _, err := a.gh.PullRequests.List(ctx, slug.Owner, slug.Name, opts)
	if err != nil {
		return nil, wrap(err, fmt.Sprintf("could not look for a pull request on %s", branch))
	}
	for _, pr := range prs {
		// The head filter is advisory on forks, so confirm the branch.
		if pr.GetHead().GetRef() != branch {
			continue
		}
		out := convertPR(pr)
		return &out, nil
	}
	return nil, nil
}

// CreatePullRequest opens a pull request and returns it.
func (a *API) CreatePullRequest(ctx context.Context, slug gitrepo.Slug, in NewPullRequest) (PullRequest, error) {
	req := github.CreatePullRequest{
		Title: github.Ptr(in.Title),
		Head:  in.Head,
		Base:  in.Base,
		Draft: github.Ptr(in.Draft),
	}
	if in.Body != "" {
		req.Body = github.Ptr(in.Body)
	}

	pr, _, err := a.gh.PullRequests.Create(ctx, slug.Owner, slug.Name, req)
	if err != nil {
		return PullRequest{}, wrap(err, fmt.Sprintf("could not open a pull request on %s", in.Head))
	}
	return convertPR(pr), nil
}

func convertPR(pr *github.PullRequest) PullRequest {
	return PullRequest{
		Number: pr.GetNumber(),
		Title:  pr.GetTitle(),
		State:  pr.GetState(),
		URL:    pr.GetHTMLURL(),
		Draft:  pr.GetDraft(),
		Head:   pr.GetHead().GetRef(),
		Base:   pr.GetBase().GetRef(),
	}
}

// ClosePullRequest closes a pull request without merging it. GitHub has no way
// to delete one — not in the UI, not in REST, not in GraphQL, at any
// permission level — so closing is as far as `enzo abort` can go.
func (a *API) ClosePullRequest(ctx context.Context, slug gitrepo.Slug, number int) error {
	_, _, err := a.gh.PullRequests.Edit(ctx, slug.Owner, slug.Name, number,
		&github.PullRequest{State: github.Ptr("closed")})
	if err != nil {
		return wrap(err, fmt.Sprintf("could not close %s#%d", slug, number))
	}
	return nil
}
