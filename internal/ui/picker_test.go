package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/version"
)

func sampleIssues(n int) []ghclient.Issue {
	out := make([]ghclient.Issue, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, ghclient.Issue{
			Number: i,
			Title:  "issue " + string(rune('a'+i-1)),
			URL:    "https://github.com/o/r/issues/" + string(rune('0'+i)),
		})
	}
	return out
}

// key builds the message bubbletea delivers for a keypress, so tests exercise
// the same path a real terminal does.
func key(s string) tea.KeyPressMsg {
	if len(s) == 1 {
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	switch s {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	panic("unhandled key in test: " + s)
}

// send feeds a sequence of keys and returns the resulting model.
func send(t *testing.T, m Picker, keys ...string) Picker {
	t.Helper()
	var model tea.Model = m
	for _, k := range keys {
		model, _ = model.Update(key(k))
	}
	p, ok := model.(Picker)
	if !ok {
		t.Fatalf("Update returned %T, want Picker", model)
	}
	return p
}

func TestCursorStartsOnNew(t *testing.T) {
	m := NewPicker("o/r", sampleIssues(3), PlainStyles())
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (the new-issue row)", m.cursor)
	}
}

func TestEnterOnNewRow(t *testing.T) {
	m := send(t, NewPicker("o/r", sampleIssues(3), PlainStyles()), "enter")
	if got := m.Result().Action; got != ActionNew {
		t.Errorf("Action = %v, want ActionNew", got)
	}
	if !m.Done() {
		t.Error("picker should be done after enter")
	}
}

func TestEnterOnIssue(t *testing.T) {
	m := send(t, NewPicker("o/r", sampleIssues(3), PlainStyles()), "down", "down", "enter")
	res := m.Result()
	if res.Action != ActionGrab {
		t.Fatalf("Action = %v, want ActionGrab", res.Action)
	}
	if res.Issue.Number != 2 {
		t.Errorf("selected issue #%d, want #2", res.Issue.Number)
	}
}

func TestCancelKeys(t *testing.T) {
	for _, k := range []string{"esc", "q", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			m := send(t, NewPicker("o/r", sampleIssues(3), PlainStyles()), "down", k)
			if got := m.Result().Action; got != ActionCancel {
				t.Errorf("Action after %q = %v, want ActionCancel", k, got)
			}
			if m.Result().Issue.Number != 0 {
				t.Error("canceling should not carry an issue")
			}
			if !m.Done() {
				t.Error("picker should be done after cancel")
			}
		})
	}
}

func TestNavigationWraps(t *testing.T) {
	issues := sampleIssues(3) // rows: new, #1, #2, #3
	tests := []struct {
		name string
		keys []string
		want int
	}{
		{"down from top", []string{"down"}, 1},
		{"down past the end wraps to new", []string{"down", "down", "down", "down"}, 0},
		{"up from new wraps to last", []string{"up"}, 3},
		{"j and k work like arrows", []string{"j", "j", "k"}, 1},
		{"home jumps to new", []string{"down", "down", "home"}, 0},
		{"end jumps to last", []string{"end"}, 3},
		{"g and G alias home and end", []string{"G", "g"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := send(t, NewPicker("o/r", issues, PlainStyles()), tt.keys...)
			if m.cursor != tt.want {
				t.Errorf("cursor = %d, want %d", m.cursor, tt.want)
			}
		})
	}
}

// With nothing assigned, "new" is the only row and must not wrap onto a
// nonexistent issue.
func TestEmptyListNavigation(t *testing.T) {
	m := NewPicker("o/r", nil, PlainStyles())
	for _, k := range []string{"down", "up", "end", "home"} {
		m = send(t, m, k)
		if m.cursor != 0 {
			t.Fatalf("cursor = %d after %q, want 0", m.cursor, k)
		}
	}
	m = send(t, m, "enter")
	if m.Result().Action != ActionNew {
		t.Errorf("enter on an empty list = %v, want ActionNew", m.Result().Action)
	}
}

func TestEmptyListView(t *testing.T) {
	out := NewPicker("o/r", nil, PlainStyles()).View().Content
	if !strings.Contains(out, "nothing assigned to you here") {
		t.Errorf("empty view should say so:\n%s", out)
	}
}

func TestViewShowsRepoAndIssues(t *testing.T) {
	issues := []ghclient.Issue{
		{Number: 12, Title: "fix the thing", Labels: []string{"bug"}},
		{Number: 34, Title: "add the other thing"},
	}
	out := NewPicker("justin-efficient/enzo", issues, PlainStyles()).View().Content

	for _, want := range []string{"justin-efficient/enzo", "+ new issue", "#12", "fix the thing", "#34", "[bug]", "esc cancel"} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing %q:\n%s", want, out)
		}
	}
}

// The help line is signed, so the list says which enzo drew it. The string is
// built in exactly one place; a second copy of it would drift at the next
// version bump.
func TestViewSignsTheHelpLine(t *testing.T) {
	for _, tt := range []struct {
		name  string
		build func() Picker
	}{
		{"the issue list", func() Picker { return NewPicker("o/r", sampleIssues(2), PlainStyles()) }},
		{"the parent picker", func() Picker { return NewParentPicker("o/r", sampleIssues(2), PlainStyles()) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := tt.build().View().Content
			want := version.Banner() + " · "
			if !strings.Contains(out, want) {
				t.Errorf("view should sign the help line with %q:\n%s", want, out)
			}
			// The signature goes before the keys, not instead of them.
			if i, j := strings.Index(out, version.Banner()), strings.Index(out, "esc cancel"); i < 0 || j < i {
				t.Errorf("the banner should come before the key hints:\n%s", out)
			}
		})
	}
}

func TestViewMarksTheCursor(t *testing.T) {
	m := NewPicker("o/r", sampleIssues(2), PlainStyles())

	out := m.View().Content
	if !strings.Contains(out, "> + new issue") {
		t.Errorf("cursor should start on the new row:\n%s", out)
	}

	out = send(t, m, "down").View().Content
	if !strings.Contains(out, "> #1") {
		t.Errorf("cursor should move to #1:\n%s", out)
	}
	if strings.Contains(out, "> + new issue") {
		t.Errorf("new row should no longer be marked:\n%s", out)
	}
}

// Exactly one row carries the cursor marker, whatever the selection.
func TestViewHasOneCursor(t *testing.T) {
	m := NewPicker("o/r", sampleIssues(4), PlainStyles())
	for i := 0; i < 5; i++ {
		out := m.View().Content
		if n := strings.Count(out, "> "); n != 1 {
			t.Errorf("after %d downs: %d cursors, want 1:\n%s", i, n, out)
		}
		m = send(t, m, "down")
	}
}

func TestWindowSizeSetsVisibleRows(t *testing.T) {
	m := NewPicker("o/r", sampleIssues(50), PlainStyles())
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 14})
	m = model.(Picker)
	if m.visible != 10 {
		t.Errorf("visible = %d, want 10 (height 14 minus 4 chrome rows)", m.visible)
	}
}

// A pathologically short terminal must not produce a zero or negative window.
func TestTinyWindowKeepsAtLeastOneRow(t *testing.T) {
	m := NewPicker("o/r", sampleIssues(5), PlainStyles())
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 2})
	m = model.(Picker)
	if m.visible < 1 {
		t.Errorf("visible = %d, want at least 1", m.visible)
	}
	if out := m.View().Content; out == "" {
		t.Error("view should still render something in a tiny window")
	}
}

func TestScrollingFollowsCursor(t *testing.T) {
	m := NewPicker("o/r", sampleIssues(20), PlainStyles())
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 9}) // 5 visible
	m = model.(Picker)

	// Walk down past the window and confirm the selected issue is rendered.
	for i := 0; i < 12; i++ {
		m = send(t, m, "down")
		out := m.View().Content
		want := "> #" + itoa(m.cursor)
		if m.cursor > 0 && !strings.Contains(out, want) {
			t.Fatalf("after %d downs, %q is not visible:\n%s", i+1, want, out)
		}
	}
}

func TestScrollingShowsMoreCount(t *testing.T) {
	m := NewPicker("o/r", sampleIssues(20), PlainStyles())
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 9}) // 5 visible
	out := model.(Picker).View().Content
	if !strings.Contains(out, "15 more") {
		t.Errorf("view should report the hidden rows:\n%s", out)
	}
}

// Jumping to the end must scroll the window, not just move the cursor.
func TestEndScrollsToLastIssue(t *testing.T) {
	m := NewPicker("o/r", sampleIssues(20), PlainStyles())
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 9})
	m = send(t, model.(Picker), "end")
	if !strings.Contains(m.View().Content, "> #20") {
		t.Errorf("end should reveal the last issue:\n%s", m.View().Content)
	}
}

func TestViewIsEmptyOnceDone(t *testing.T) {
	m := send(t, NewPicker("o/r", sampleIssues(2), PlainStyles()), "enter")
	if out := m.View().Content; out != "" {
		t.Errorf("view after quitting = %q, want empty", out)
	}
}

func TestUnknownKeysAreIgnored(t *testing.T) {
	m := NewPicker("o/r", sampleIssues(3), PlainStyles())
	after := send(t, m, "z", "!", "5")
	if after.cursor != 0 || after.Done() {
		t.Errorf("unknown keys changed state: cursor=%d done=%v", after.cursor, after.Done())
	}
}

func TestInitReturnsNoCommand(t *testing.T) {
	if cmd := NewPicker("o/r", nil, PlainStyles()).Init(); cmd != nil {
		t.Error("Init should not issue a command")
	}
}

func TestEnterReturnsQuitCommand(t *testing.T) {
	m := NewPicker("o/r", sampleIssues(1), PlainStyles())
	_, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("enter should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("enter should return tea.Quit, got %T", cmd())
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
