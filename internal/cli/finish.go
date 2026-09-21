package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/gitrepo"
)

// Finish takes the pull request on the current branch out of draft, checks
// that everything standing between it and the base branch is satisfied, and
// merges it when it is.
//
// It takes no arguments: what it finishes is where you are, the same way
// `enzo abort` throws away where you are.
func Finish(ctx context.Context, env Env, args []string) error {
	fs := flag.NewFlagSet("finish", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("`enzo finish` takes no arguments; it acts on the branch you are on (%s)", fs.Arg(0))
	}

	root, slug, err := repoContext(env)
	if err != nil {
		return err
	}
	branch, err := gitrepo.CurrentBranch(root)
	if err != nil {
		return err
	}
	client, login, err := clientFor(ctx, env, root)
	if err != nil {
		return err
	}

	number, ok := parseBranch(login, branch)
	if !ok {
		return fmt.Errorf("%q is not a branch enzo started; finish only merges branches named %s",
			branch, branchName(login, 0, "")+"-<title>")
	}
	pr, err := client.PullRequestForBranch(ctx, slug, branch)
	if err != nil {
		return err
	}
	if pr == nil {
		return fmt.Errorf("no open pull request on %s; `enzo start %d` opens one", branch, number)
	}

	headline(env.Stdout, emojiFinish, "finishing Issue #%d %q", number, pr.Title)

	// 1. The worktree. Uncommitted work on a tracked file is not in the pull
	// request, so merging would ship something other than what you have. It
	// reports and joins the blockers like every other check, rather than
	// cutting the run short: one run names everything that is wrong.
	//
	// Untracked files are left out: a scratch file or a build artefact nobody
	// told git about is not work the pull request is missing.
	dirty, err := gitrepo.DirtyTrackedFiles(root)
	if err != nil {
		return err
	}
	var blockers []string
	rr := []row{{mark(len(dirty) == 0), "changes", describeDirty(dirty)}}
	if len(dirty) > 0 {
		blockers = append(blockers, plural(len(dirty), "file")+" uncommitted — commit or stash them")
	}

	// 2. Out of draft next: the merge state and the review decision are both
	// reported differently for a draft, so reading them first would be
	// reading the wrong pull request.
	if pr.Draft {
		if err := client.MarkReadyForReview(ctx, pr.NodeID); err != nil {
			return err
		}
		rr = append(rr, row{markPass, "undrafted", fmt.Sprintf("PR #%d", pr.Number)})
	} else {
		rr = append(rr, row{markPass, "undrafted", fmt.Sprintf("PR #%d was already out of draft", pr.Number)})
	}

	ready, err := client.Readiness(ctx, slug, pr.Number)
	if err != nil {
		return err
	}

	checked, remaining := assess(ready)
	blockers = append(blockers, remaining...)
	width := rows(env.Stdout, append(rr, checked...)...)

	if len(blockers) > 0 {
		// Everything was reported before refusing, so one run names every
		// reason rather than one reason at a time.
		return fmt.Errorf("not merged: %s", strings.Join(blockers, "; "))
	}

	if err := client.MergePullRequest(ctx, slug, pr.Number); err != nil {
		return err
	}
	record(env, root, slug.String(), "merged",
		fmt.Sprintf("PR #%d for #%d into %s", pr.Number, number, ready.BaseBranch), pr.URL)
	printMarkedRow(env.Stdout, markPass, width, "merged", fmt.Sprintf("PR #%d into %s", pr.Number, ready.BaseBranch))
	return nil
}

// describeDirty names the uncommitted files, up to a few. The whole list is a
// `git status` away, and a row that runs to a screen of paths stops being a
// row.
func describeDirty(paths []string) string {
	if len(paths) == 0 {
		return "none, the worktree is clean"
	}
	const most = 3
	shown := paths
	if len(shown) > most {
		shown = shown[:most]
	}
	out := plural(len(paths), "file") + " uncommitted: " + strings.Join(shown, ", ")
	if n := len(paths) - len(shown); n > 0 {
		out += fmt.Sprintf(", and %d more", n)
	}
	return out
}

// assess turns a Readiness into the rows to print and the reasons not to
// merge. Every check reports, whether or not it passed, so the output is the
// same shape either way and a run that refuses says why on every count.
func assess(r ghclient.Readiness) (checked []row, blockers []string) {
	// 3. Is review satisfied? An empty decision means the repository asks for
	// none, which is not the same as having been approved.
	switch r.ReviewDecision {
	case "":
		checked = append(checked, row{markPass, "review", "not required here"})
	case "APPROVED":
		checked = append(checked, row{markPass, "review", "approved"})
	case "CHANGES_REQUESTED":
		checked = append(checked, row{markFail, "review", "changes requested"})
		blockers = append(blockers, "a reviewer asked for changes")
	default:
		checked = append(checked, row{markFail, "review", "required — " + describeReviewers(r.Reviewers)})
		blockers = append(blockers, "review is required and has not been given")
	}

	// 4. The pull request's own checks. Still running counts against it: the
	// merge cannot go ahead on a check that has not answered yet.
	passing := r.Checks == "" || r.Checks == "SUCCESS"
	checked = append(checked, row{mark(passing), "checks", describeChecks(r)})
	switch r.Checks {
	case "", "SUCCESS":
	case "PENDING", "EXPECTED":
		blockers = append(blockers, "checks are still running")
	default:
		blockers = append(blockers, "checks failed")
	}

	// 5. The base branch's own build, which is not a blocker: a broken main is
	// a reason to know, not a reason your work cannot land on it.
	base := r.BaseBranch
	if base == "" {
		base = "base"
	}
	// A cross here means the base build is red, not that your work is stuck:
	// this row adds nothing to blockers. A base still building is not red.
	baseRed := base != "" && r.BaseChecks != "" && r.BaseChecks != "SUCCESS" &&
		r.BaseChecks != "PENDING" && r.BaseChecks != "EXPECTED"
	checked = append(checked, row{mark(!baseRed), base, describeBase(r)})

	// 6. Can it merge at all? Last, because it is GitHub's verdict on
	// everything above: the rows before it are the reasons, this is the
	// answer.
	switch r.Mergeable {
	case "MERGEABLE":
		// BLOCKED is GitHub's way of saying a rule is unsatisfied without
		// saying which; the review and check rows above usually name it, and
		// this catches the case where neither does.
		blocked := r.MergeState == "BLOCKED" || r.MergeState == "BEHIND"
		checked = append(checked, row{mark(!blocked), "mergeable", describeMergeState(r.MergeState)})
		if r.MergeState == "BLOCKED" {
			blockers = append(blockers, "a branch protection rule is unsatisfied")
		}
		if r.MergeState == "BEHIND" {
			blockers = append(blockers, "the branch is behind its base; update it first")
		}
	case "CONFLICTING":
		checked = append(checked, row{markFail, "mergeable", "no — conflicts with the base"})
		blockers = append(blockers, "it conflicts with the base branch")
	default:
		// GitHub computes mergeability lazily, so the first read of a pull
		// request that has just changed can land here.
		checked = append(checked, row{markFail, "mergeable", "unknown — GitHub is still working it out"})
		blockers = append(blockers, "GitHub has not finished working out whether it merges; try again shortly")
	}

	return checked, blockers
}

func describeMergeState(state string) string {
	switch state {
	case "CLEAN":
		return "yes, clean"
	case "UNSTABLE":
		return "yes, but a non-required check is failing"
	case "BEHIND":
		return "behind the base branch"
	case "BLOCKED":
		return "blocked by a branch protection rule"
	case "":
		return "yes"
	}
	return "yes, " + strings.ToLower(state)
}

func describeReviewers(who []string) string {
	if len(who) == 0 {
		return "nobody has been asked yet"
	}
	return "waiting on " + strings.Join(who, ", ")
}

func describeChecks(r ghclient.Readiness) string {
	switch {
	case r.Checks == "":
		return "none on this commit"
	case len(r.Failed) > 0:
		return "failed: " + strings.Join(r.Failed, ", ")
	case len(r.Pending) > 0:
		return "still running: " + strings.Join(r.Pending, ", ")
	case r.Checks == "SUCCESS":
		return "all passed"
	}
	return strings.ToLower(r.Checks)
}

func describeBase(r ghclient.Readiness) string {
	at := ""
	if r.BaseHead != "" {
		at = " at " + r.BaseHead
	}
	switch r.BaseChecks {
	case "":
		return "no build" + at
	case "SUCCESS":
		return "passing" + at
	case "PENDING", "EXPECTED":
		return "building" + at
	}
	return strings.ToLower(r.BaseChecks) + at
}
