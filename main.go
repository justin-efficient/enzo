// Command enzo is a small CLI that keeps GitHub issues and PRs linked.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/justin-efficient/enzo/internal/browser"
	"github.com/justin-efficient/enzo/internal/cli"
	"github.com/justin-efficient/enzo/internal/eventlog"
	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/ui"
	"github.com/justin-efficient/enzo/internal/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(version.Banner())
		return
	}

	cwd, err := os.Getwd()
	if err != nil {
		fail(err)
	}

	interactive := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))

	env := cli.Env{
		Dir:         cwd,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Getenv:      os.Getenv,
		Interactive: interactive,
		NewClient: func(token, host string) (ghclient.Client, error) {
			return ghclient.New(token, host)
		},
		AskToken:   askToken,
		Pick:       pick,
		PickParent: pickParent,
		AskDraft:   askDraft,
		Await:      await,
		OpenURL:    browser.Open,
		Log:        eventlog.Append,
	}

	if err := cli.Run(ctx, env, os.Args[1:]); err != nil {
		if errors.Is(err, cli.ErrCanceled) {
			return
		}
		fail(err)
	}
}

// askToken runs the masked token prompt.
func askToken(repo string) (string, error) {
	m, err := tea.NewProgram(ui.NewTokenPrompt(repo, ui.DefaultStyles())).Run()
	if err != nil {
		return "", err
	}
	p, ok := m.(ui.TokenPrompt)
	if !ok || p.Canceled() {
		return "", cli.ErrCanceled
	}
	return p.Value(), nil
}

// pick runs the issue picker.
func pick(repo string, issues []ghclient.Issue) (ui.Result, error) {
	return runPicker(ui.NewPicker(repo, issues, ui.DefaultStyles()))
}

// pickParent runs the parent-issue picker for `enzo new sub`.
func pickParent(repo string, issues []ghclient.Issue) (ui.Result, error) {
	return runPicker(ui.NewParentPicker(repo, issues, ui.DefaultStyles()))
}

func runPicker(model ui.Picker) (ui.Result, error) {
	m, err := tea.NewProgram(model).Run()
	if err != nil {
		return ui.Result{}, err
	}
	p, ok := m.(ui.Picker)
	if !ok {
		return ui.Result{}, nil
	}
	return p.Result(), nil
}

// await shows a spinner until poll reports true. Giving up, whether by timeout
// or because the user pressed esc, is not an error: the list is shown either
// way, just possibly without the newest issue yet.
func await(message string, poll func() (bool, error)) error {
	m, err := tea.NewProgram(ui.NewWaiter(message, poll, ui.DefaultStyles())).Run()
	if err != nil {
		return err
	}
	w, ok := m.(ui.Waiter)
	if !ok {
		return nil
	}
	return w.Err()
}

// askDraft runs the new-issue form.
func askDraft(repo string, parent *ghclient.Issue) (ui.Draft, error) {
	m, err := tea.NewProgram(ui.NewDraftForm(repo, parent, ui.DefaultStyles())).Run()
	if err != nil {
		return ui.Draft{}, err
	}
	f, ok := m.(ui.DraftForm)
	if !ok || f.Canceled() {
		return ui.Draft{}, cli.ErrCanceled
	}
	return f.Draft(), nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "enzo:", err)
	os.Exit(1)
}
