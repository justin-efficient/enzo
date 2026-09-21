// Package cli wires enzo's commands together. Everything the commands touch
// beyond the filesystem is injected through Env so tests can drive them
// without a terminal or a network.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"time"

	"github.com/justin-efficient/enzo/internal/config"
	"github.com/justin-efficient/enzo/internal/eventlog"
	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/gitrepo"
	"github.com/justin-efficient/enzo/internal/ui"
	"github.com/justin-efficient/enzo/internal/version"
)

// ErrCanceled means the user backed out; callers should exit quietly.
var ErrCanceled = errors.New("canceled")

// Env is everything a command needs from the outside world.
type Env struct {
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string

	// Interactive reports whether we may take over the terminal.
	Interactive bool

	// NewClient builds a GitHub client. Tests swap in a fake.
	NewClient func(token, host string) (ghclient.Client, error)

	// AskToken prompts for a token. Tests swap in a canned answer.
	AskToken func(repo string) (string, error)

	// Pick runs the issue picker. Tests swap in a canned choice.
	Pick func(repo string, issues []ghclient.Issue) (ui.Result, error)

	// PickParent runs the parent-issue picker for `enzo new sub`.
	PickParent func(repo string, issues []ghclient.Issue) (ui.Result, error)

	// AskDraft runs the new-issue form. parent may be nil.
	AskDraft func(repo string, parent *ghclient.Issue) (ui.Draft, error)

	// Await shows a spinner while poll is retried, until it reports true.
	Await func(message string, poll func() (bool, error)) error

	// OpenURL shows a URL in the user's browser. Tests swap in a recorder.
	OpenURL func(url string) error

	// Log records something enzo created. It is given the configured log path
	// and must not fail the command it is reporting on.
	Log func(path string, e eventlog.Entry) error
}

// record writes an entry to the log. A log that cannot be written is worth a
// warning but must never turn a successful action into a failure.
func record(env Env, root, repo, action, text, url string) {
	if env.Log == nil {
		return
	}
	cfg, err := config.Load(root)
	if err != nil && !errors.Is(err, config.ErrNotFound) {
		fmt.Fprintf(env.Stderr, "enzo: could not read %s for the log path: %v\n", config.FileName, err)
		return
	}
	configured := ""
	if cfg != nil {
		configured = cfg.Log
	}
	path, err := eventlog.DefaultPath(configured, env.Getenv)
	if err != nil {
		fmt.Fprintf(env.Stderr, "enzo: not logging: %v\n", err)
		return
	}
	entry := eventlog.Entry{Time: time.Now(), Repo: repo, Action: action, Text: text, URL: url}
	if err := env.Log(path, entry); err != nil {
		fmt.Fprintf(env.Stderr, "enzo: could not write the log: %v\n", err)
	}
}

// signedBody signs what enzo is about to post — an issue body, or a pull
// request body — with the same footer. The signature sits under a horizontal
// rule so it reads as a footer rather than as part of what was written, and a
// body that is otherwise empty is still signed: that is the case where "where
// did this come from?" gets asked.
func signedBody(body string) string {
	footer := "*" + version.Credit() + "*"
	if body = strings.TrimSpace(body); body == "" {
		return footer
	}
	return body + "\n\n---\n\n" + footer
}

// Usage is the help text printed for `enzo help` and unknown commands. It is
// built rather than declared so the banner carries the running version.
func Usage() string { return version.Banner() + usageBody }

const usageBody = ` — issue lifecycle for GitHub

usage:
  enzo setup            store the token enzo uses for this repo
  enzo list             browse the open issues assigned to you,
                        opening the one you pick in a browser
  enzo new ["title"]    open an issue assigned to you
  enzo new sub [n] ["title"]
                        open it as a sub-issue of #n, or pick a parent
  enzo start [n] ["title"]
                        branch and draft a PR for #n, opening it first
                        if you gave a title instead of a number
  enzo abort            close the PR and delete the branch you are on
  enzo finish           undraft, check, and merge the PR you are on
  enzo help             show this message
`

// Run dispatches a command. args excludes the program name.
func Run(ctx context.Context, env Env, args []string) error {
	cmd := "list"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "setup":
		return Setup(ctx, env, args)
	case "list":
		return List(ctx, env, args)
	case "new":
		return New(ctx, env, args)
	case "start":
		return Start(ctx, env, args)
	case "abort":
		return Abort(ctx, env, args)
	case "finish":
		return Finish(ctx, env, args)
	case "help", "-h", "--help":
		fmt.Fprint(env.Stdout, Usage())
		return nil
	default:
		fmt.Fprint(env.Stderr, Usage())
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// clientFor builds an authenticated client for the repo at root and returns it
// alongside the login it authenticated as.
func clientFor(ctx context.Context, env Env, root string) (ghclient.Client, string, error) {
	cfg, err := config.Load(root)
	if err != nil && !errors.Is(err, config.ErrNotFound) {
		return nil, "", err
	}
	token := config.ResolveToken(cfg, env.Getenv)
	if token == "" {
		return nil, "", fmt.Errorf("no token for this repo; run `enzo setup`")
	}
	host := ""
	if cfg != nil {
		host = cfg.Host
	}

	client, err := env.NewClient(token, host)
	if err != nil {
		return nil, "", err
	}
	login, err := client.Viewer(ctx)
	if err != nil {
		return nil, "", err
	}
	return client, login, nil
}

// repoContext resolves the repository root and the GitHub slug for env.Dir.
func repoContext(env Env) (root string, slug gitrepo.Slug, err error) {
	root, err = gitrepo.Root(env.Dir)
	if err != nil {
		return "", gitrepo.Slug{}, err
	}
	slug, err = gitrepo.OriginSlug(root)
	if err != nil {
		return "", gitrepo.Slug{}, err
	}
	return root, slug, nil
}
