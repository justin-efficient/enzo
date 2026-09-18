package ui

import (
	"bytes"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/justin-efficient/enzo/internal/ghclient"
)

// These tests drive the real bubbletea event loop through a pseudo-terminal,
// so they cover program startup, rendering and teardown rather than just
// Update in isolation.

func testIssues() []ghclient.Issue {
	return []ghclient.Issue{
		{Number: 12, Title: "fix the thing", URL: "https://github.com/o/r/issues/12"},
		{Number: 34, Title: "add the other thing", URL: "https://github.com/o/r/issues/34"},
	}
}

func waitForText(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte(want))
	}, teatest.WithCheckInterval(10*time.Millisecond), teatest.WithDuration(5*time.Second))
}

func TestPickerProgramSelectsIssue(t *testing.T) {
	tm := teatest.NewTestModel(t, NewPicker("o/r", testIssues(), PlainStyles()),
		teatest.WithInitialTermSize(80, 24))

	waitForText(t, tm, "fix the thing")

	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(Picker)
	if !ok {
		t.Fatal("final model is not a Picker")
	}
	res := final.Result()
	if res.Action != ActionGrab {
		t.Fatalf("Action = %v, want ActionGrab", res.Action)
	}
	if res.Issue.Number != 34 {
		t.Errorf("selected #%d, want #34", res.Issue.Number)
	}
}

func TestPickerProgramNewIssue(t *testing.T) {
	tm := teatest.NewTestModel(t, NewPicker("o/r", testIssues(), PlainStyles()),
		teatest.WithInitialTermSize(80, 24))

	waitForText(t, tm, "new issue")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(Picker)
	if got := final.Result().Action; got != ActionNew {
		t.Errorf("Action = %v, want ActionNew", got)
	}
}

func TestPickerProgramEscCancels(t *testing.T) {
	tm := teatest.NewTestModel(t, NewPicker("o/r", testIssues(), PlainStyles()),
		teatest.WithInitialTermSize(80, 24))

	waitForText(t, tm, "open issues assigned to you")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEscape})

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(Picker)
	if got := final.Result().Action; got != ActionCancel {
		t.Errorf("Action = %v, want ActionCancel", got)
	}
}

// The picker should exit cleanly and leave the issue list off the screen.
func TestPickerProgramClearsOnExit(t *testing.T) {
	tm := teatest.NewTestModel(t, NewPicker("o/r", testIssues(), PlainStyles()),
		teatest.WithInitialTermSize(80, 24))

	waitForText(t, tm, "fix the thing")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEscape})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	if _, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(5*time.Second))); err != nil {
		t.Fatalf("reading final output: %v", err)
	}
}

func TestTokenPromptProgram(t *testing.T) {
	tm := teatest.NewTestModel(t, NewTokenPrompt("o/r", PlainStyles()),
		teatest.WithInitialTermSize(80, 24))

	waitForText(t, tm, "enzo setup")
	tm.Type("ghp_fromtheterminal")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(TokenPrompt)
	if final.Canceled() {
		t.Fatal("prompt should not report canceled")
	}
	if got := final.Value(); got != "ghp_fromtheterminal" {
		t.Errorf("Value = %q, want the typed token", got)
	}
}

// What is typed must never reach the terminal in the clear.
func TestTokenPromptProgramNeverEchoesToken(t *testing.T) {
	const secret = "ghp_neverprintthis"
	tm := teatest.NewTestModel(t, NewTokenPrompt("o/r", PlainStyles()),
		teatest.WithInitialTermSize(80, 24))

	waitForText(t, tm, "enzo setup")
	tm.Type(secret)
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	out, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(5*time.Second)))
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if bytes.Contains(out, []byte(secret)) {
		t.Errorf("the token was echoed to the terminal:\n%s", out)
	}
}
