package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/justin-efficient/enzo/internal/ghclient"
)

func windowSize(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }

// nested builds a parent with two sub-issues, in the order the API returns
// them: most recently updated first, so children come before the parent.
func nested() []ghclient.Issue {
	child := func(n, parent int, title string) ghclient.Issue {
		return ghclient.Issue{
			ID: int64(n * 100), Number: n, Title: title,
			ParentRepo: "o/r", ParentNumber: parent,
		}
	}
	return []ghclient.Issue{
		child(5, 1, "second child"),
		child(3, 1, "first child"),
		{ID: 400, Number: 4, Title: "unrelated"},
		{ID: 100, Number: 1, Title: "the parent"},
	}
}

// issueLines returns the rendered rows that show an issue.
func issueLines(t *testing.T, out string) []string {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "#") && (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "> ")) {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestPickerNestsSubIssues(t *testing.T) {
	out := NewPicker("o/r", nested(), PlainStyles()).View().Content
	lines := issueLines(t, out)
	if len(lines) != 4 {
		t.Fatalf("got %d issue rows, want 4:\n%s", len(lines), out)
	}

	// Display order: roots in input order, each followed by its children.
	wantOrder := []string{"#4", "#1", "#5", "#3"}
	for i, want := range wantOrder {
		if !strings.Contains(lines[i], want) {
			t.Errorf("row %d = %q, want it to show %s\nfull view:\n%s", i, lines[i], want, out)
		}
	}
}

// Children are indented two spaces past their parent.
func TestPickerIndentsChildren(t *testing.T) {
	out := NewPicker("o/r", nested(), PlainStyles()).View().Content

	col := func(marker string) int {
		for _, line := range strings.Split(out, "\n") {
			if i := strings.Index(line, marker); i >= 0 {
				return i
			}
		}
		t.Fatalf("did not find %q in:\n%s", marker, out)
		return -1
	}

	parent, first, second, unrelated := col("#1"), col("#3"), col("#5"), col("#4")
	if parent != unrelated {
		t.Errorf("roots should share a column: #1 at %d, #4 at %d", parent, unrelated)
	}
	if first != second {
		t.Errorf("siblings should share a column: #3 at %d, #5 at %d", first, second)
	}
	if first-parent != IndentWidth {
		t.Errorf("children indented %d columns past the parent, want %d", first-parent, IndentWidth)
	}
}

func TestIndent(t *testing.T) {
	tests := []struct {
		depth int
		want  string
	}{
		{-1, ""},
		{0, ""},
		{1, "  "},
		{2, "    "},
		{3, "      "},
	}
	for _, tt := range tests {
		if got := Indent(tt.depth); got != tt.want {
			t.Errorf("Indent(%d) = %q, want %q", tt.depth, got, tt.want)
		}
	}
}

func TestPickerIndentsDeeply(t *testing.T) {
	issues := []ghclient.Issue{
		{ID: 1, Number: 1, Title: "root"},
		{ID: 2, Number: 2, Title: "level one", ParentRepo: "o/r", ParentNumber: 1},
		{ID: 3, Number: 3, Title: "level two", ParentRepo: "o/r", ParentNumber: 2},
	}
	out := NewPicker("o/r", issues, PlainStyles()).View().Content

	col := func(marker string) int {
		for _, line := range strings.Split(out, "\n") {
			if i := strings.Index(line, marker); i >= 0 {
				return i
			}
		}
		return -1
	}
	if got := col("#3") - col("#2"); got != IndentWidth {
		t.Errorf("level two indented %d past level one, want %d:\n%s", got, IndentWidth, out)
	}
	if got := col("#2") - col("#1"); got != IndentWidth {
		t.Errorf("level one indented %d past root, want %d:\n%s", got, IndentWidth, out)
	}
}

// Nesting reorders the rows, so the cursor must still select what is shown.
func TestPickerSelectionFollowsTreeOrder(t *testing.T) {
	m := NewPicker("o/r", nested(), PlainStyles())
	// Rows: new, #4, #1, #5, #3
	wantByPosition := []int{4, 1, 5, 3}
	for i, want := range wantByPosition {
		down := make([]string, i+1)
		for j := range down {
			down[j] = "down"
		}
		after := send(t, m, append(down, "enter")...)
		res := after.Result()
		if res.Action != ActionGrab {
			t.Fatalf("%d downs then enter: Action = %v, want ActionGrab", i+1, res.Action)
		}
		if res.Issue.Number != want {
			t.Errorf("%d downs selected #%d, want #%d", i+1, res.Issue.Number, want)
		}
	}
}

func TestParentPickerNestsToo(t *testing.T) {
	out := NewParentPicker("o/r", nested(), PlainStyles()).View().Content
	lines := issueLines(t, out)
	if len(lines) != 4 {
		t.Fatalf("got %d rows, want 4:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[1], "#1") {
		t.Errorf("row 1 = %q, want the parent #1:\n%s", lines[1], out)
	}
	if !strings.Contains(lines[2], "#5") {
		t.Errorf("row 2 = %q, want the child #5 nested under it:\n%s", lines[2], out)
	}
}

// An issue whose parent is not in the list must still be shown.
func TestPickerShowsOrphans(t *testing.T) {
	issues := []ghclient.Issue{
		{ID: 500, Number: 5, Title: "child of someone else's issue", ParentRepo: "o/r", ParentNumber: 99},
	}
	out := NewPicker("o/r", issues, PlainStyles()).View().Content
	if !strings.Contains(out, "#5") {
		t.Errorf("an orphaned sub-issue should still be listed:\n%s", out)
	}
	if got := send(t, NewPicker("o/r", issues, PlainStyles()), "down", "enter").Result(); got.Issue.Number != 5 {
		t.Errorf("selected #%d, want #5", got.Issue.Number)
	}
}

// Scrolling counts tree rows, not just top-level issues.
func TestPickerScrollingCountsNestedRows(t *testing.T) {
	var issues []ghclient.Issue
	issues = append(issues, ghclient.Issue{ID: 1, Number: 1, Title: "root"})
	for n := 2; n <= 20; n++ {
		issues = append(issues, ghclient.Issue{
			ID: int64(n), Number: n, Title: "child",
			ParentRepo: "o/r", ParentNumber: 1,
		})
	}
	m := NewPicker("o/r", issues, PlainStyles())
	model, _ := m.Update(windowSize(80, 9)) // 5 visible
	m = model.(Picker)

	if !strings.Contains(m.View().Content, "more") {
		t.Errorf("a 20-issue tree in a 5-row window should report more:\n%s", m.View().Content)
	}
	m = send(t, m, "end")
	if !strings.Contains(m.View().Content, "> ") {
		t.Errorf("the cursor should be visible after jumping to the end:\n%s", m.View().Content)
	}
}
