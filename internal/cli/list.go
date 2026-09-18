package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/gitrepo"
	"github.com/justin-efficient/enzo/internal/ui"
)

// List shows the open issues assigned to you and acts on the one you pick.
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
		issues, err := client.AssignedIssues(ctx, slug, login)
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

		case ui.ActionGrab:
			fmt.Fprintf(env.Stdout, "#%d %s\n%s\n", res.Issue.Number, res.Issue.Title, res.Issue.URL)
			fmt.Fprintf(env.Stdout, "run `enzo grab %d` to switch to its branch\n", res.Issue.Number)
			return nil

		default:
			return nil
		}
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
