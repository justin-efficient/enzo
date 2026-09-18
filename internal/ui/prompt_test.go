package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func typeString(t *testing.T, m TokenPrompt, s string) TokenPrompt {
	t.Helper()
	var model tea.Model = m
	for _, r := range s {
		model, _ = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	p, ok := model.(TokenPrompt)
	if !ok {
		t.Fatalf("Update returned %T, want TokenPrompt", model)
	}
	return p
}

func TestTokenPromptCapturesValue(t *testing.T) {
	m := typeString(t, NewTokenPrompt("o/r", PlainStyles()), "ghp_secret")
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(TokenPrompt)

	if m.Canceled() {
		t.Error("enter should not cancel")
	}
	if got := m.Value(); got != "ghp_secret" {
		t.Errorf("Value = %q, want %q", got, "ghp_secret")
	}
}

func TestTokenPromptTrimsWhitespace(t *testing.T) {
	m := typeString(t, NewTokenPrompt("o/r", PlainStyles()), "  ghp_secret  ")
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := model.(TokenPrompt).Value(); got != "ghp_secret" {
		t.Errorf("Value = %q, want the token without surrounding spaces", got)
	}
}

func TestTokenPromptCancel(t *testing.T) {
	for name, k := range map[string]tea.KeyPressMsg{
		"esc":    {Code: tea.KeyEscape},
		"ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
	} {
		t.Run(name, func(t *testing.T) {
			m := typeString(t, NewTokenPrompt("o/r", PlainStyles()), "ghp_secret")
			model, _ := m.Update(k)
			p := model.(TokenPrompt)
			if !p.Canceled() {
				t.Errorf("%s should cancel the prompt", name)
			}
			if p.Value() != "" {
				t.Errorf("canceling should not yield a token, got %q", p.Value())
			}
		})
	}
}

// The token must never be echoed to the screen.
func TestTokenPromptMasksInput(t *testing.T) {
	m := typeString(t, NewTokenPrompt("o/r", PlainStyles()), "ghp_supersecret")
	out := m.View().Content
	if strings.Contains(out, "ghp_supersecret") {
		t.Errorf("the token is visible in the view:\n%s", out)
	}
	if !strings.Contains(out, "*") {
		t.Errorf("view should show the mask character:\n%s", out)
	}
}

func TestTokenPromptShowsRepo(t *testing.T) {
	out := NewTokenPrompt("justin-efficient/enzo", PlainStyles()).View().Content
	for _, want := range []string{"justin-efficient/enzo", "repo", "esc cancel"} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing %q:\n%s", want, out)
		}
	}
}

func TestTokenPromptViewEmptyOnceDone(t *testing.T) {
	m := typeString(t, NewTokenPrompt("o/r", PlainStyles()), "x")
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if out := model.(TokenPrompt).View().Content; out != "" {
		t.Errorf("view after enter = %q, want empty", out)
	}
}

func TestTokenPromptEnterQuits(t *testing.T) {
	m := NewTokenPrompt("o/r", PlainStyles())
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("enter should quit, got %T", cmd())
	}
}

func TestTokenPromptInitBlinks(t *testing.T) {
	if NewTokenPrompt("o/r", PlainStyles()).Init() == nil {
		t.Error("Init should start the cursor blinking")
	}
}
