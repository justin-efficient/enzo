package ghclient

import (
	"fmt"
	"strings"
	"testing"
)

const testRepo = "justin-efficient/enzo"

// iss builds an issue; parent 0 means no parent.
func iss(number, parent int) Issue {
	i := Issue{ID: int64(number * 100), Number: number, Title: fmt.Sprintf("issue %d", number)}
	if parent > 0 {
		i.ParentRepo, i.ParentNumber = testRepo, parent
	}
	return i
}

// layout renders the arrangement as "depth:number" lines, for easy comparison.
func layout(nodes []Node) string {
	var b strings.Builder
	for _, n := range nodes {
		fmt.Fprintf(&b, "%d:%d ", n.Depth, n.Issue.Number)
	}
	return strings.TrimSpace(b.String())
}

func TestArrangeNestsChildUnderParent(t *testing.T) {
	// Listing order is newest first, so the child arrives before the parent.
	got := Arrange(testRepo, []Issue{iss(5, 1), iss(4, 0), iss(1, 0)})
	if want := "0:4 0:1 1:5"; layout(got) != want {
		t.Errorf("layout = %q, want %q", layout(got), want)
	}
}

func TestArrangeKeepsRootOrder(t *testing.T) {
	got := Arrange(testRepo, []Issue{iss(9, 0), iss(3, 0), iss(7, 0)})
	if want := "0:9 0:3 0:7"; layout(got) != want {
		t.Errorf("layout = %q, want %q — root order should be preserved", layout(got), want)
	}
}

func TestArrangeKeepsSiblingOrder(t *testing.T) {
	got := Arrange(testRepo, []Issue{iss(1, 0), iss(8, 1), iss(2, 1), iss(5, 1)})
	if want := "0:1 1:8 1:2 1:5"; layout(got) != want {
		t.Errorf("layout = %q, want %q — siblings should keep input order", layout(got), want)
	}
}

func TestArrangeNestsDeeply(t *testing.T) {
	got := Arrange(testRepo, []Issue{iss(1, 0), iss(2, 1), iss(3, 2), iss(4, 3)})
	if want := "0:1 1:2 2:3 3:4"; layout(got) != want {
		t.Errorf("layout = %q, want %q", layout(got), want)
	}
}

func TestArrangeMultipleTrees(t *testing.T) {
	got := Arrange(testRepo, []Issue{iss(1, 0), iss(2, 1), iss(10, 0), iss(11, 10)})
	if want := "0:1 1:2 0:10 1:11"; layout(got) != want {
		t.Errorf("layout = %q, want %q", layout(got), want)
	}
}

// A sub-issue whose parent is not in the list — not assigned to you, or
// closed — must still appear, at the top level.
func TestArrangeOrphanStaysAtRoot(t *testing.T) {
	got := Arrange(testRepo, []Issue{iss(5, 99), iss(4, 0)})
	if want := "0:5 0:4"; layout(got) != want {
		t.Errorf("layout = %q, want %q", layout(got), want)
	}
}

// A parent in a different repository must not capture a local issue that
// happens to share its number.
func TestArrangeIgnoresOtherRepoParents(t *testing.T) {
	child := iss(5, 0)
	child.ParentRepo, child.ParentNumber = "someone-else/other", 1

	got := Arrange(testRepo, []Issue{iss(1, 0), child})
	if want := "0:1 0:5"; layout(got) != want {
		t.Errorf("layout = %q, want %q — a foreign parent should not nest", layout(got), want)
	}
}

func TestArrangeIgnoresSelfParent(t *testing.T) {
	got := Arrange(testRepo, []Issue{iss(1, 1)})
	if want := "0:1"; layout(got) != want {
		t.Errorf("layout = %q, want %q", layout(got), want)
	}
}

// A parent chain that loops must not hang, and must not silently drop issues.
func TestArrangeSurvivesCycles(t *testing.T) {
	done := make(chan []Node, 1)
	go func() { done <- Arrange(testRepo, []Issue{iss(1, 2), iss(2, 1)}) }()

	var got []Node
	select {
	case got = <-done:
	case <-timeoutAfterSeconds(5):
		t.Fatal("Arrange did not finish; a cycle sent it into infinite recursion")
	}

	if len(got) != 2 {
		t.Fatalf("got %d nodes, want both issues kept: %s", len(got), layout(got))
	}
	seen := map[int]bool{}
	for _, n := range got {
		if seen[n.Issue.Number] {
			t.Errorf("issue #%d appears twice: %s", n.Issue.Number, layout(got))
		}
		seen[n.Issue.Number] = true
	}
}

// Every input issue must come out exactly once, whatever the shape.
func TestArrangePreservesEveryIssue(t *testing.T) {
	in := []Issue{iss(5, 1), iss(4, 0), iss(1, 0), iss(6, 99), iss(7, 5)}
	got := Arrange(testRepo, in)

	if len(got) != len(in) {
		t.Fatalf("got %d nodes from %d issues: %s", len(got), len(in), layout(got))
	}
	count := map[int]int{}
	for _, n := range got {
		count[n.Issue.Number]++
	}
	for _, i := range in {
		if count[i.Number] != 1 {
			t.Errorf("issue #%d appears %d times, want 1", i.Number, count[i.Number])
		}
	}
}

// A child must never be rendered before its parent.
func TestArrangeChildAlwaysFollowsParent(t *testing.T) {
	got := Arrange(testRepo, []Issue{iss(5, 1), iss(3, 1), iss(1, 0)})
	pos := map[int]int{}
	for i, n := range got {
		pos[n.Issue.Number] = i
	}
	for _, child := range []int{5, 3} {
		if pos[child] < pos[1] {
			t.Errorf("#%d appears before its parent #1: %s", child, layout(got))
		}
	}
}

func TestArrangeEmpty(t *testing.T) {
	if got := Arrange(testRepo, nil); len(got) != 0 {
		t.Errorf("Arrange(nil) = %v, want empty", got)
	}
}

func TestIssuesFlattens(t *testing.T) {
	nodes := Arrange(testRepo, []Issue{iss(5, 1), iss(1, 0)})
	flat := Issues(nodes)
	if len(flat) != 2 {
		t.Fatalf("got %d issues, want 2", len(flat))
	}
	if flat[0].Number != 1 || flat[1].Number != 5 {
		t.Errorf("flattened order = %d,%d, want display order 1,5", flat[0].Number, flat[1].Number)
	}
}

func TestHasParent(t *testing.T) {
	if iss(1, 0).HasParent() {
		t.Error("an issue with no parent should report HasParent false")
	}
	if !iss(5, 1).HasParent() {
		t.Error("a sub-issue should report HasParent true")
	}
}

func TestParseIssueURL(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantRepo   string
		wantNumber int
	}{
		{"api url", "https://api.github.com/repos/justin-efficient/enzo/issues/1", "justin-efficient/enzo", 1},
		{"multi digit", "https://api.github.com/repos/o/r/issues/1234", "o/r", 1234},
		{"enterprise path", "https://ghe.internal/api/v3/repos/o/r/issues/7", "o/r", 7},
		{"trailing slash", "https://api.github.com/repos/o/r/issues/7/", "o/r", 7},
		{"empty", "", "", 0},
		{"not an issue url", "https://api.github.com/repos/o/r/pulls/7", "", 0},
		{"too short", "https://api.github.com/issues/7", "", 0},
		{"non numeric", "https://api.github.com/repos/o/r/issues/abc", "", 0},
		{"zero", "https://api.github.com/repos/o/r/issues/0", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, number := parseIssueURL(tt.url)
			if repo != tt.wantRepo || number != tt.wantNumber {
				t.Errorf("parseIssueURL(%q) = (%q, %d), want (%q, %d)", tt.url, repo, number, tt.wantRepo, tt.wantNumber)
			}
		})
	}
}
