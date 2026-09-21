package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/justin-efficient/enzo/internal/ghclient"
)

var keyCtrlN = tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}

func pressKey(t *testing.T, m Picker, keys ...tea.KeyPressMsg) Picker {
	t.Helper()
	var model tea.Model = m
	for _, k := range keys {
		model, _ = model.Update(k)
	}
	return model.(Picker)
}

// ctrl+n on a highlighted issue asks for a sub-issue of it.
func TestCtrlNOnIssueMakesSubIssue(t *testing.T) {
	m := send(t, NewPicker("o/r", sampleIssues(3), PlainStyles()), "down", "down")
	after := pressKey(t, m, keyCtrlN)

	res := after.Result()
	if res.Action != ActionNewSub {
		t.Fatalf("Action = %v, want ActionNewSub", res.Action)
	}
	if res.Issue.Number != 2 {
		t.Errorf("parent = #%d, want the highlighted #2", res.Issue.Number)
	}
	if !after.Done() {
		t.Error("ctrl+n should resolve the picker")
	}
}

// The parent carries its id, which is what the sub-issue link needs.
func TestCtrlNCarriesTheParentID(t *testing.T) {
	issues := []ghclient.Issue{{ID: 4242, Number: 7, Title: "the parent"}}
	after := pressKey(t, send(t, NewPicker("o/r", issues, PlainStyles()), "down"), keyCtrlN)

	if got := after.Result().Issue.ID; got != 4242 {
		t.Errorf("parent id = %d, want 4242", got)
	}
}

// On the pinned "new" row there is nothing to parent to, so ctrl+n opens a
// top-level issue rather than doing nothing.
func TestCtrlNOnNewRowIsATopLevelIssue(t *testing.T) {
	m := NewPicker("o/r", sampleIssues(3), PlainStyles())
	after := pressKey(t, m, keyCtrlN)

	if got := after.Result().Action; got != ActionNew {
		t.Errorf("Action = %v, want ActionNew", got)
	}
	if after.Result().Issue.Number != 0 {
		t.Error("a top-level issue should carry no parent")
	}
}

func TestCtrlNOnEmptyList(t *testing.T) {
	after := pressKey(t, NewPicker("o/r", nil, PlainStyles()), keyCtrlN)
	if got := after.Result().Action; got != ActionNew {
		t.Errorf("Action = %v, want ActionNew", got)
	}
}

// Enter still just chooses the row; ctrl+n is the only way to get a sub-issue.
func TestEnterStillChooses(t *testing.T) {
	m := send(t, NewPicker("o/r", sampleIssues(3), PlainStyles()), "down", "enter")
	if got := m.Result().Action; got != ActionChoose {
		t.Errorf("Action = %v, want ActionChoose", got)
	}
}

// ctrl+n picks the parent from a nested tree by what is on screen.
func TestCtrlNWithNestedIssues(t *testing.T) {
	// Display order: #4, #1, #5, #3 — see nested().
	m := send(t, NewPicker("o/r", nested(), PlainStyles()), "down", "down", "down")
	after := pressKey(t, m, keyCtrlN)

	res := after.Result()
	if res.Action != ActionNewSub {
		t.Fatalf("Action = %v, want ActionNewSub", res.Action)
	}
	if res.Issue.Number != 5 {
		t.Errorf("parent = #%d, want the highlighted #5", res.Issue.Number)
	}
}

// The parent picker is for choosing among existing issues; ctrl+n has no
// meaning there and must not resolve it.
func TestCtrlNIgnoredInParentPicker(t *testing.T) {
	after := pressKey(t, NewParentPicker("o/r", sampleIssues(3), PlainStyles()), keyCtrlN)
	if after.Done() {
		t.Error("ctrl+n should do nothing in the parent picker")
	}
	if after.Result().Action == ActionNewSub {
		t.Error("the parent picker must never produce ActionNewSub")
	}
}

func TestHelpLineMentionsCtrlN(t *testing.T) {
	out := NewPicker("o/r", sampleIssues(2), PlainStyles()).View().Content
	if !strings.Contains(out, "ctrl+n new sub-issue") {
		t.Errorf("the list help should name ctrl+n:\n%s", out)
	}

	parent := NewParentPicker("o/r", sampleIssues(2), PlainStyles()).View().Content
	if strings.Contains(parent, "ctrl+n") {
		t.Errorf("the parent picker help should not offer ctrl+n:\n%s", parent)
	}
}

func TestCtrlNQuits(t *testing.T) {
	m := send(t, NewPicker("o/r", sampleIssues(2), PlainStyles()), "down")
	_, cmd := m.Update(keyCtrlN)
	if cmd == nil {
		t.Fatal("ctrl+n should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+n should quit, got %T", cmd())
	}
}
