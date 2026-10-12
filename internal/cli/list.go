package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/gitrepo"
	"github.com/justin-efficient/enzo/internal/ui"
)

// List shows the open issues assigned to you. Enter edits the highlighted one
// and ctrl+o opens it in a browser.
func List(ctx context.Context, env Env, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	plain := fs.Bool("plain", false, "print the issues instead of showing the picker")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root, slug, err := repoContext(env)
	if err != nil {
		return err
	}

	client, login, err := clientFor(ctx, env, root)
	if err != nil {
		return err
	}

	if *plain || !env.Interactive {
		issues, err := client.AssignedIssues(ctx, slug, login)
		if err != nil {
			return err
		}
		printIssues(env, slug.String(), issues)
		return nil
	}

	// Creating an issue returns to the list rather than exiting, so the picker
	// runs in a loop, re-reading the issues each time round. enzo keeps no
	// issue state of its own: the list is always what GitHub reports.
	for {
		issues, err := assignedWithPriority(ctx, client, slug, login)
		if err != nil {
			return err
		}

		res, err := env.Pick(slug.String(), issues)
		if err != nil {
			return err
		}

		switch res.Action {
		case ui.ActionCancel:
			// Esc on the list itself means there is nothing to do.
			return nil

		case ui.ActionNew:
			created, err := runNew(ctx, env, root, client, login, slug, newRequest{})
			if err != nil && !errors.Is(err, ErrCanceled) {
				return err
			}
			// Created or backed out, either way show the list again.
			if err := awaitListed(ctx, env, client, slug, login, created); err != nil {
				return err
			}

		case ui.ActionNewSub:
			// The highlighted issue is the parent, so it needs no lookup.
			parent := res.Issue
			created, err := runNew(ctx, env, root, client, login, slug, newRequest{
				sub:    true,
				parent: &parent,
			})
			if err != nil && !errors.Is(err, ErrCanceled) {
				return err
			}
			if err := awaitListed(ctx, env, client, slug, login, created); err != nil {
				return err
			}

		case ui.ActionOpen:
			// Opening shows the issue and comes back to the list. It does not
			// start work on it: branching and pushing is too much to hang off
			// a key in a list, so starting stayed in `enzo start`. See
			// docs/decisions/0007-enter-opens-the-issue.md.
			openIssue(env, res.Issue)

		case ui.ActionEdit:
			edited, err := runEdit(ctx, env, root, client, slug, res.Issue)
			if err != nil && !errors.Is(err, ErrCanceled) {
				return err
			}
			if err := awaitEdited(ctx, env, client, slug, login, edited); err != nil {
				return err
			}

		default:
			return nil
		}
	}
}

// assignedWithPriority lists the issues and fills in each one's board
// priority, which colors its number in the picker.
//
// Priority is decoration. It needs GraphQL and the `read:project` token scope,
// and plenty of issues are on no board, so any failure to read it leaves the
// numbers uncolored instead of failing the list.
func assignedWithPriority(ctx context.Context, client ghclient.Client, slug gitrepo.Slug, login string) ([]ghclient.Issue, error) {
	issues, err := client.AssignedIssues(ctx, slug, login)
	if err != nil {
		return nil, err
	}
	prios, err := client.Priorities(ctx, slug, login)
	if err != nil {
		return issues, nil
	}
	out := make([]ghclient.Issue, len(issues))
	for i, iss := range issues {
		iss.Priority = prios[iss.Number]
		out[i] = iss
	}
	return out, nil
}

// runEdit asks for a new title and body and saves them.
//
// The issue is re-read first: the listing lags and may be stale, and saving
// over a body edited elsewhere a moment ago would silently lose that edit.
//
// It returns the saved issue, or nil when nothing was changed.
func runEdit(ctx context.Context, env Env, root string, client ghclient.Client, slug gitrepo.Slug, listed ghclient.Issue) (*ghclient.Issue, error) {
	current, err := client.Issue(ctx, slug, listed.Number)
	if err != nil {
		return nil, err
	}

	draft, err := env.AskEdit(slug.String(), current)
	if err != nil {
		return nil, err
	}
	title, body := strings.TrimSpace(draft.Title), normalizeBody(draft.Body)
	if title == "" {
		return nil, ErrCanceled
	}
	if title == strings.TrimSpace(current.Title) && body == normalizeBody(current.Body) {
		return nil, nil
	}

	saved, err := client.UpdateIssue(ctx, slug, current.Number, ghclient.IssueEdit{Title: title, Body: body})
	if err != nil {
		return nil, err
	}
	record(env, root, slug.String(), "edited",
		fmt.Sprintf("#%d %s", saved.Number, saved.Title), saved.URL)
	return &saved, nil
}

// normalizeBody makes bodies comparable: GitHub often stores CRLF, the form
// edits LF, and the form trims.
func normalizeBody(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

// awaitEdited blocks until GitHub's issue listing shows the saved title and
// body, so the list does not come back with the old text. edited is nil when
// nothing was saved.
func awaitEdited(ctx context.Context, env Env, client ghclient.Client, slug gitrepo.Slug, login string, edited *ghclient.Issue) error {
	if edited == nil || env.Await == nil {
		return nil
	}

	want := *edited
	return env.Await(fmt.Sprintf("waiting for GitHub to list the edit to #%d", want.Number), func() (bool, error) {
		issues, err := client.AssignedIssues(ctx, slug, login)
		if err != nil {
			return false, err
		}
		for _, iss := range issues {
			if iss.Number == want.Number {
				return iss.Title == want.Title && normalizeBody(iss.Body) == normalizeBody(want.Body), nil
			}
		}
		// Not listed (unassigned or closed meanwhile): nothing to wait for.
		return true, nil
	})
}

// openIssue shows an issue in the browser. Like the log, a browser that will
// not open is a warning and not a failed command: the URL is printed so the
// issue is still reachable, and the list carries on.
func openIssue(env Env, iss ghclient.Issue) {
	if env.OpenURL == nil {
		fmt.Fprintln(env.Stderr, iss.URL)
		return
	}
	if err := env.OpenURL(iss.URL); err != nil {
		fmt.Fprintf(env.Stderr, "enzo: could not open a browser: %v\n%s\n", err, iss.URL)
	}
}

// awaitListed blocks until GitHub's issue listing includes the issue just
// created, showing a spinner meanwhile. The listing lags several seconds
// behind a creation, so without this the list would come back without it.
//
// created is nil when nothing was made, in which case there is nothing to wait
// for.
func awaitListed(ctx context.Context, env Env, client ghclient.Client, slug gitrepo.Slug, login string, created *ghclient.Issue) error {
	if created == nil || env.Await == nil {
		return nil
	}

	want := *created
	return env.Await(fmt.Sprintf("waiting for GitHub to list #%d", want.Number), func() (bool, error) {
		issues, err := client.AssignedIssues(ctx, slug, login)
		if err != nil {
			return false, err
		}
		return listed(issues, want), nil
	})
}

// listed reports whether the listing has caught up with the new issue. For a
// sub-issue the parent link has to be there too, or the issue would appear
// briefly at the top level instead of under its parent.
func listed(issues []ghclient.Issue, want ghclient.Issue) bool {
	for _, iss := range issues {
		if iss.Number != want.Number {
			continue
		}
		if want.HasParent() {
			return iss.ParentNumber == want.ParentNumber
		}
		return true
	}
	return false
}

// printIssues writes the plain, pipe-friendly form: "#12  title", with
// sub-issues indented under their parent.
func printIssues(env Env, repo string, issues []ghclient.Issue) {
	if len(issues) == 0 {
		fmt.Fprintln(env.Stdout, "no open issues assigned to you here")
		return
	}

	numW := 0
	for _, iss := range issues {
		if n := len(fmt.Sprintf("#%d", iss.Number)); n > numW {
			numW = n
		}
	}
	for _, n := range ghclient.Arrange(repo, issues) {
		fmt.Fprintf(env.Stdout, "%s%-*s %s\n",
			ui.Indent(n.Depth), numW, fmt.Sprintf("#%d", n.Issue.Number), n.Issue.Title)
	}
}
