package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/gitrepo"
	"github.com/justin-efficient/enzo/internal/ui"
)

// New creates an issue assigned to you, optionally as a sub-issue.
//
//	enzo new                  a top-level issue, title prompted for
//	enzo new "fix the thing"  a top-level issue with that title
//	enzo new sub              a sub-issue; the parent is chosen from a list
//	enzo new sub "fix it"     the same, with the title given
//	enzo new sub 42           a sub-issue of #42
//	enzo new sub 42 "fix it"  a sub-issue of #42 with that title
func New(ctx context.Context, env Env, args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	titleFlag := fs.String("title", "", "issue title (otherwise enzo prompts)")
	body := fs.String("body", "", "issue body")

	sub, parentNumber, rest, err := parseSubArgs(args)
	if err != nil {
		return err
	}
	// Go's flag package stops at the first non-flag argument, so the title has
	// to come off the front before the flags are parsed. Anything left over
	// afterwards is a title written after the flags.
	leading, flagArgs := splitLeadingPositional(rest)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	positional := append(append([]string{}, leading...), fs.Args()...)

	title, err := titleFrom(positional, *titleFlag)
	if err != nil {
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

	_, err = runNew(ctx, env, root, client, login, slug, newRequest{
		sub:          sub,
		parentNumber: parentNumber,
		title:        title,
		body:         strings.TrimSpace(*body),
	})
	return err
}

// newRequest is everything `enzo new` was asked for, once the arguments are
// parsed.
type newRequest struct {
	sub          bool
	parentNumber int
	// parent, when set, is an already-fetched parent issue. The list picker
	// supplies it so choosing a parent on screen costs no extra request.
	parent *ghclient.Issue
	title  string
	body   string
}

// runNew creates the issue using a client the caller already authenticated, so
// `enzo list` can loop back into it without re-authenticating each time.
//
// It returns the issue it created so the caller can show it straight away.
// GitHub's issue listing takes a few seconds to catch up with a creation, so
// re-reading the list here would reliably come back without it.
func runNew(ctx context.Context, env Env, root string, client ghclient.Client, login string, slug gitrepo.Slug, req newRequest) (*ghclient.Issue, error) {
	parent, err := resolveParent(ctx, env, client, slug, req)
	if err != nil {
		return nil, err
	}

	draft, err := draftFor(env, slug.String(), parent, req.title, req.body)
	if err != nil {
		return nil, err
	}
	if draft.Title == "" {
		return nil, ErrCanceled
	}

	issue, err := client.CreateIssue(ctx, slug, ghclient.NewIssue{
		Title:    draft.Title,
		Body:     draft.Body,
		Assignee: login,
	})
	if err != nil {
		return nil, err
	}
	record(env, root, slug.String(), "created",
		fmt.Sprintf("#%d %s", issue.Number, issue.Title), issue.URL)

	if parent != nil {
		if err := client.LinkSubIssue(ctx, slug, parent.Number, issue.ID); err != nil {
			// The issue exists; say so rather than implying nothing happened.
			return nil, fmt.Errorf("created #%d but could not link it under #%d: %w", issue.Number, parent.Number, err)
		}
		record(env, root, slug.String(), "linked",
			fmt.Sprintf("#%d under #%d", issue.Number, parent.Number), issue.URL)

		// The creation response predates the link, so fill the parent in here;
		// otherwise the issue would render unnested until the next fetch.
		issue.ParentRepo, issue.ParentNumber = slug.String(), parent.Number
	}
	return &issue, nil
}

// parseSubArgs pulls the leading "sub [number]" off args, leaving the title and
// flags behind. A bare number straight after "sub" is the parent; anything else
// is left alone, so `enzo new sub "a title"` reads as a title rather than a
// malformed issue number.
func parseSubArgs(args []string) (sub bool, parent int, rest []string, err error) {
	rest = args
	if len(rest) == 0 || rest[0] != "sub" {
		return false, 0, rest, nil
	}
	sub, rest = true, rest[1:]

	if len(rest) > 0 {
		if n, ok := asIssueNumber(rest[0]); ok {
			if n <= 0 {
				return false, 0, nil, fmt.Errorf("issue numbers start at 1, got %d", n)
			}
			parent, rest = n, rest[1:]
		}
	}
	return sub, parent, rest, nil
}

// splitLeadingPositional separates the arguments before the first flag from
// the flags and everything after them.
func splitLeadingPositional(args []string) (positional, flags []string) {
	i := 0
	for i < len(args) && !strings.HasPrefix(args[i], "-") {
		i++
	}
	return args[:i], args[i:]
}

// asIssueNumber reads "12" or "#12". Anything else is not an issue number, so
// it can be a title instead.
func asIssueNumber(s string) (int, bool) {
	digits := strings.TrimPrefix(s, "#")
	if digits == "" {
		return 0, false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			// A leading "-" lands here too, so flags are never eaten.
			return 0, false
		}
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return n, true
}

// titleFrom settles the title from the positional argument and the --title
// flag, rejecting input that could mean two different things.
func titleFrom(positional []string, flagValue string) (string, error) {
	flagValue = strings.TrimSpace(flagValue)

	switch len(positional) {
	case 0:
		return flagValue, nil
	case 1:
		if flagValue != "" {
			return "", fmt.Errorf("both a title argument (%q) and --title (%q) were given; use one", positional[0], flagValue)
		}
		return strings.TrimSpace(positional[0]), nil
	default:
		// Almost always an unquoted title. Say so, with the fix.
		return "", fmt.Errorf("unexpected extra arguments %q; quote the title, as in: enzo new %q",
			strings.Join(positional[1:], " "), strings.Join(positional, " "))
	}
}

// resolveParent settles which issue the new one hangs under, prompting when
// `sub` was asked for without a number.
func resolveParent(ctx context.Context, env Env, client ghclient.Client, slug gitrepo.Slug, req newRequest) (*ghclient.Issue, error) {
	if !req.sub {
		return nil, nil
	}

	// Already in hand, from the row the user highlighted.
	if req.parent != nil {
		return req.parent, nil
	}

	if req.parentNumber > 0 {
		issue, err := client.Issue(ctx, slug, req.parentNumber)
		if err != nil {
			return nil, err
		}
		return &issue, nil
	}

	if !env.Interactive {
		return nil, errors.New("`enzo new sub` needs a parent issue number when there is no terminal to pick one in")
	}

	issues, err := client.OpenIssues(ctx, slug)
	if err != nil {
		return nil, err
	}
	if len(issues) == 0 {
		return nil, fmt.Errorf("%s has no open issues to parent this one", slug)
	}

	res, err := env.PickParent(slug.String(), issues)
	if err != nil {
		return nil, err
	}
	if res.Action != ui.ActionGrab {
		return nil, ErrCanceled
	}
	return &res.Issue, nil
}

// draftFor takes the title and body from flags, or asks for them.
func draftFor(env Env, repo string, parent *ghclient.Issue, title, body string) (ui.Draft, error) {
	if strings.TrimSpace(title) != "" {
		return ui.Draft{Title: strings.TrimSpace(title), Body: strings.TrimSpace(body)}, nil
	}
	if !env.Interactive {
		return ui.Draft{}, errors.New("no title given: pass --title")
	}
	draft, err := env.AskDraft(repo, parent)
	if err != nil {
		return ui.Draft{}, err
	}
	// Trim here rather than trusting the form, so a whitespace-only title is
	// treated as backing out.
	draft.Title = strings.TrimSpace(draft.Title)
	draft.Body = strings.TrimSpace(draft.Body)
	return draft, nil
}
