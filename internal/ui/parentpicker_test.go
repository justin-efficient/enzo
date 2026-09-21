package ui

import (
	"strings"
	"testing"

	"github.com/justin-efficient/enzo/internal/ghclient"
)

func parentCandidates() []ghclient.Issue {
	return []ghclient.Issue{
		{ID: 100, Number: 1, Title: "candidate one"},
		{ID: 200, Number: 2, Title: "candidate two"},
		{ID: 300, Number: 3, Title: "candidate three"},
	}
}

// Choosing a parent means choosing among issues that exist, so there is no
// "new" row to land on by accident.
func TestParentPickerHasNoNewRow(t *testing.T) {
	out := NewParentPicker("o/r", parentCandidates(), PlainStyles()).View().Content
	if strings.Contains(out, "new issue") {
		t.Errorf("the parent picker should not offer a new row:\n%s", out)
	}
	if !strings.Contains(out, "choose a parent issue in o/r") {
		t.Errorf("heading should name the task:\n%s", out)
	}
}

func TestParentPickerStartsOnFirstIssue(t *testing.T) {
	m := NewParentPicker("o/r", parentCandidates(), PlainStyles())
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
	if !strings.Contains(m.View().Content, "> #1") {
		t.Errorf("the first issue should be selected:\n%s", m.View().Content)
	}
}

func TestParentPickerEnterSelectsIssue(t *testing.T) {
	m := send(t, NewParentPicker("o/r", parentCandidates(), PlainStyles()), "down", "enter")
	res := m.Result()
	if res.Action != ActionChoose {
		t.Fatalf("Action = %v, want ActionChoose", res.Action)
	}
	if res.Issue.Number != 2 {
		t.Errorf("selected #%d, want #2", res.Issue.Number)
	}
	// The id is what sub-issue linking uses.
	if res.Issue.ID != 200 {
		t.Errorf("selected id %d, want 200", res.Issue.ID)
	}
}

// Enter on the first row must pick that issue, not fall through to ActionNew.
func TestParentPickerFirstRowIsAnIssue(t *testing.T) {
	m := send(t, NewParentPicker("o/r", parentCandidates(), PlainStyles()), "enter")
	res := m.Result()
	if res.Action != ActionChoose {
		t.Fatalf("Action = %v, want ActionChoose", res.Action)
	}
	if res.Issue.Number != 1 {
		t.Errorf("selected #%d, want #1", res.Issue.Number)
	}
}

func TestParentPickerWraps(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want int
	}{
		{"up from the top wraps to the last", []string{"up"}, 3},
		{"down past the end wraps to the first", []string{"down", "down", "down"}, 1},
		{"end jumps to the last", []string{"end"}, 3},
		{"home jumps to the first", []string{"end", "home"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := send(t, NewParentPicker("o/r", parentCandidates(), PlainStyles()), tt.keys...)
			m = send(t, m, "enter")
			if got := m.Result().Issue.Number; got != tt.want {
				t.Errorf("selected #%d, want #%d", got, tt.want)
			}
		})
	}
}

func TestParentPickerCancels(t *testing.T) {
	m := send(t, NewParentPicker("o/r", parentCandidates(), PlainStyles()), "esc")
	if got := m.Result().Action; got != ActionCancel {
		t.Errorf("Action = %v, want ActionCancel", got)
	}
}

// With nothing to pick, enter must not resolve to anything.
func TestParentPickerEmpty(t *testing.T) {
	m := NewParentPicker("o/r", nil, PlainStyles())
	if !strings.Contains(m.View().Content, "no open issues here to parent this one") {
		t.Errorf("empty parent picker should explain itself:\n%s", m.View().Content)
	}
	after := send(t, m, "enter")
	if after.Done() {
		t.Error("enter on an empty parent picker should do nothing")
	}
	if after.Result().Action == ActionNew {
		t.Error("the parent picker must never produce ActionNew")
	}
}

// The list picker keeps its "new" row; generalising must not have cost it.
func TestListPickerStillHasNewRow(t *testing.T) {
	m := NewPicker("o/r", parentCandidates(), PlainStyles())
	out := m.View().Content
	if !strings.Contains(out, "+ new issue") {
		t.Errorf("the list picker should still pin a new row:\n%s", out)
	}
	if !strings.Contains(out, "> + new issue") {
		t.Errorf("the new row should start selected:\n%s", out)
	}
	if got := send(t, m, "enter").Result().Action; got != ActionNew {
		t.Errorf("enter on the new row = %v, want ActionNew", got)
	}
}
