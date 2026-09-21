package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/gitrepo"
	"github.com/justin-efficient/enzo/internal/version"
)

// Start puts you on a branch with a draft pull request open against an issue,
// creating whichever of the three does not exist yet.
//
//	enzo start 42             work on #42
//	enzo start "fix it"       open an issue with that title, then work on it
func Start(ctx context.Context, env Env, args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	titleFlag := fs.String("title", "", "title for the issue enzo opens")
	body := fs.String("body", "", "body for the issue enzo opens")

	number, rest, err := parseStartArgs(args)
	if err != nil {
		return err
	}
	// The flag package stops at the first non-flag argument, so the title has
	// to come off the front before the flags are parsed.
	leading, flagArgs := splitLeadingPositional(rest)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	title, err := titleFrom(append(append([]string{}, leading...), fs.Args()...), *titleFlag)
	if err != nil {
		return err
	}

	switch {
	case number > 0 && title != "":
		return fmt.Errorf("#%d already has a title of its own; give a number to work on an issue, or a title to open one, not both", number)
	case number == 0 && title == "":
		return errors.New("nothing to start: give an issue number, or a title to open a new issue")
	}

	root, slug, err := repoContext(env)
	if err != nil {
		return err
	}
	client, login, err := clientFor(ctx, env, root)
	if err != nil {
		return err
	}

	return runStart(ctx, env, root, client, login, slug, startRequest{
		number: number,
		title:  title,
		body:   strings.TrimSpace(*body),
	})
}

// startRequest is what `enzo start` was asked for, once parsed.
type startRequest struct {
	// number is the issue to work on; 0 means open a new one titled title.
	number int
	title  string
	body   string

	// issue, when set, is an issue the caller already has in hand. The list
	// picker supplies it so starting from the list costs no extra request.
	issue *ghclient.Issue
}

// runStart is the body of `enzo start`, taking an authenticated client so
// `enzo list` can start the issue you highlighted without authenticating again.
//
// Every GitHub read happens before the worktree is touched, so a failure that
// was going to happen anyway leaves you on the branch you started on.
func runStart(ctx context.Context, env Env, root string, client ghclient.Client, login string, slug gitrepo.Slug, req startRequest) error {
	issue, err := startIssue(ctx, env, root, client, login, slug, req)
	if err != nil {
		return err
	}
	if !strings.EqualFold(issue.State, "open") && issue.State != "" {
		fmt.Fprintf(env.Stderr, "enzo: #%d is %s; starting it anyway\n", issue.Number, issue.State)
	}

	branch := branchName(login, issue.Number, issue.Title)

	base, err := client.DefaultBranch(ctx, slug)
	if err != nil {
		return err
	}
	existing, err := client.PullRequestForBranch(ctx, slug, branch)
	if err != nil {
		return err
	}

	// Resolved before the worktree is touched: it decides where a new branch
	// starts, and a fetch that is going to fail should fail while you are
	// still on the branch you were on.
	baseRev, baseErr := resolveBase(root, base)

	createdBranch, err := checkout(root, branch, baseRev)
	if err != nil {
		return err
	}

	if existing != nil {
		// Already set up. Say where things stand rather than doing it twice.
		report(env, *issue, branch, *existing, createdBranch, false)
		return nil
	}

	// GitHub will not open a pull request between two identical branches, so a
	// branch with nothing on it needs a commit before there is a PR to draft.
	// See docs/decisions/0004-draft-pr-needs-a-commit.md.
	if needsCommit(root, baseRev, baseErr) {
		// The subject says what put it there, so an empty commit at the root
		// of a branch is not a mystery months later. The issue it belongs to
		// is in the branch name and in the pull request.
		if err := gitrepo.CommitEmpty(root, version.Credit()); err != nil {
			return err
		}
	}
	if err := gitrepo.Push(root, "origin", branch); err != nil {
		return err
	}

	pr, err := client.CreatePullRequest(ctx, slug, ghclient.NewPullRequest{
		Title: issue.Title,
		// The closing keyword is what keeps the PR and the issue linked, and
		// what lets `enzo finish` close the issue by merging. It is signed the
		// same way an issue body is.
		Body:  signedBody(fmt.Sprintf("Closes #%d", issue.Number)),
		Head:  branch,
		Base:  base,
		Draft: true,
	})
	if err != nil {
		return err
	}
	record(env, root, slug.String(), "drafted",
		fmt.Sprintf("PR #%d for #%d on %s", pr.Number, issue.Number, branch), pr.URL)

	report(env, *issue, branch, pr, createdBranch, true)
	return nil
}

// startIssue settles which issue is being started, opening a new one when no
// number was given.
func startIssue(ctx context.Context, env Env, root string, client ghclient.Client, login string, slug gitrepo.Slug, req startRequest) (*ghclient.Issue, error) {
	if req.issue != nil {
		return req.issue, nil
	}
	if req.number > 0 {
		issue, err := client.Issue(ctx, slug, req.number)
		if err != nil {
			return nil, err
		}
		return &issue, nil
	}
	issue, err := runNew(ctx, env, root, client, login, slug, newRequest{title: req.title, body: req.body})
	if err != nil {
		return nil, err
	}
	// `enzo start "a title"` opened this one; say what it made before it says
	// what it did with it.
	reportNew(env, issue)
	return issue, nil
}

// checkout puts the worktree on branch, creating it at baseRev if it is not
// there yet.
//
// A new branch starts from the base — origin's default branch — and not from
// whatever you happened to be standing on, so work for one issue never arrives
// carrying work for another. A branch that already exists is switched to as it
// is; enzo does not move other people's branches around.
//
// Uncommitted work comes along, the same as a hand-typed `git switch`. When it
// cannot be carried, git refuses and enzo stops there, still on the branch you
// started on.
func checkout(root, branch, baseRev string) (created bool, err error) {
	current, err := gitrepo.CurrentBranch(root)
	if err != nil {
		return false, err
	}
	if current == branch {
		return false, nil
	}
	if gitrepo.BranchExists(root, branch) {
		return false, gitrepo.Switch(root, branch)
	}
	if baseRev == "" {
		// Nothing to start from; `git switch -c` off HEAD is still better
		// than refusing to start work at all.
		return true, gitrepo.CreateBranch(root, branch, "")
	}
	return true, gitrepo.CreateBranch(root, branch, baseRev)
}

// needsCommit reports whether the branch holds nothing GitHub could open a
// pull request on. A base that would not resolve counts as needing one: a
// spare empty commit is easy to drop, where GitHub's "no commits between" is
// just confusing.
func needsCommit(root, baseRev string, baseErr error) bool {
	if baseErr != nil {
		return true
	}
	n, err := gitrepo.CommitsAhead(root, baseRev, "HEAD")
	if err != nil {
		return true
	}
	return n == 0
}

// resolveBase resolves the commit new branches start from and pull requests
// are measured against.
//
// It fetches, because GitHub compares against the base as *GitHub* has it. A
// refs/remotes/origin/<base> on disk is only as fresh as the last fetch, and a
// stale one reads as "this branch is ahead" for a base that has since absorbed
// those very commits — whereupon enzo skips the empty commit and GitHub
// refuses the pull request with "No commits between".
//
// A fetch that fails falls back to what is on disk. That is what enzo used to
// do always, and offline there is nothing better to consult — nor any way to
// push, so the pull request was not going to happen either way.
func resolveBase(root, base string) (string, error) {
	if rev, err := gitrepo.FetchBranch(root, "origin", base); err == nil {
		return rev, nil
	}
	for _, ref := range []string{"refs/remotes/origin/" + base, "refs/heads/" + base} {
		if rev, err := gitrepo.Rev(root, ref); err == nil {
			return rev, nil
		}
	}
	return "", fmt.Errorf("cannot resolve %s locally or from origin", base)
}

// report prints where the issue, branch and pull request ended up.
//
// The verbs are what enzo actually did, not what the command is for: starting
// an issue twice reports that it switched and found, so a second `enzo start`
// does not read like it just redid the work of the first.
func report(env Env, issue ghclient.Issue, branch string, pr ghclient.PullRequest, createdBranch, openedPR bool) {
	branchVerb, prVerb := "switched to", "found"
	if createdBranch {
		branchVerb = "created"
	}
	if openedPR {
		prVerb = "drafted"
	}
	headline(env.Stdout, emojiStart, "starting work on Issue #%d %q", issue.Number, issue.Title)
	rows(env.Stdout,
		row{"", branchVerb, branch},
		row{"", prVerb, fmt.Sprintf("PR #%d %s", pr.Number, pr.URL)},
	)
}

// parseStartArgs pulls a leading issue number off args, leaving the title and
// any flags behind.
func parseStartArgs(args []string) (number int, rest []string, err error) {
	rest = args
	if len(rest) == 0 {
		return 0, rest, nil
	}
	n, ok := asIssueNumber(rest[0])
	if !ok {
		return 0, rest, nil
	}
	if n <= 0 {
		return 0, nil, fmt.Errorf("issue numbers start at 1, got %d", n)
	}
	return n, rest[1:], nil
}

// maxSlug bounds the title part of a branch name. Long enough to recognise the
// issue, short enough to type and to fit in a terminal prompt.
const maxSlug = 48

// branchName is where an issue's work goes: "<login>/<number>-<title>".
func branchName(login string, number int, title string) string {
	prefix := fmt.Sprintf("%s/%d", login, number)
	if s := slugify(title); s != "" {
		return prefix + "-" + s
	}
	return prefix
}

// slugify reduces a title to lowercase words joined by dashes, so it is safe
// as a git ref and readable in a branch listing.
func slugify(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			// One dash for any run of punctuation or spacing.
			if n := b.String(); n != "" && !strings.HasSuffix(n, "-") {
				b.WriteByte('-')
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > maxSlug {
		s = s[:maxSlug]
		// Cut back to a word boundary rather than mid-word.
		if i := strings.LastIndexByte(s, '-'); i > 0 {
			s = s[:i]
		}
	}
	return strings.Trim(s, "-")
}
