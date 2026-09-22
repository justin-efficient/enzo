package cli

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// column is where a substring starts on screen rather than in the byte slice.
// The marks are wide glyphs, so the two differ wherever one appears.
func column(line, sub string) int {
	i := strings.Index(line, sub)
	if i < 0 {
		return -1
	}
	return lipgloss.Width(line[:i])
}

// The nouns line up in a column, so a block of rows reads down the page.
func TestRowsAlignNouns(t *testing.T) {
	var b bytes.Buffer
	rows(&b,
		row{"", "switched to", "main"},
		row{"", "closed", "PR #4"},
		row{"", "left", "#2 open"},
	)

	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	nouns := []string{"main", "PR #4", "#2 open"}
	if len(lines) != len(nouns) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(nouns), b.String())
	}
	// Every noun ends the line, so where it starts is the column it sits in.
	want := len(lines[0]) - len(nouns[0])
	for i, line := range lines {
		if !strings.HasSuffix(line, nouns[i]) {
			t.Fatalf("line %d = %q, want it to end with %q", i, line, nouns[i])
		}
		if got := len(line) - len(nouns[i]); got != want {
			t.Errorf("noun starts at column %d, want %d: %q", got, want, line)
		}
		if !strings.HasPrefix(line, indent) {
			t.Errorf("row is not indented: %q", line)
		}
	}
}

// A single row still gets its colon and its indent.
func TestRowsSingle(t *testing.T) {
	var b bytes.Buffer
	rows(&b, row{"", "url", "https://example.invalid/1"})
	if got, want := b.String(), indent+"url: https://example.invalid/1\n"; got != want {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

// printRow is what abort uses, pinned to the same width its rows are printed
// with, so the block lines up though it is written a line at a time.
func TestPrintRowMatchesRows(t *testing.T) {
	var one, many bytes.Buffer
	printRow(&one, abortWidth, "closed", "PR #4")
	rows(&many, row{"", "switched to", "x"}, row{"", "closed", "PR #4"})

	if want := strings.Split(many.String(), "\n")[1] + "\n"; one.String() != want {
		t.Errorf("printRow = %q, want %q", one.String(), want)
	}
}

// A marked block puts the status glyph in a gutter of its own, and the nouns
// still line up behind it.
func TestRowsAlignBehindMarks(t *testing.T) {
	var b bytes.Buffer
	rows(&b,
		row{markPass, "changes", "worktree is clean"},
		row{markFail, "checks", "failed: dist"},
	)

	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), b.String())
	}
	if !strings.HasPrefix(lines[0], indent+markPass+" ") {
		t.Errorf("a passing row should open with %s: %q", markPass, lines[0])
	}
	if !strings.HasPrefix(lines[1], indent+markFail+" ") {
		t.Errorf("a failing row should open with %s: %q", markFail, lines[1])
	}
}

// An unmarked row inside a marked block gets the blank gutter, so it does not
// hang left of the rest.
func TestRowsPadUnmarkedRowsInAMarkedBlock(t *testing.T) {
	var b bytes.Buffer
	rows(&b,
		row{markPass, "changes", "clean"},
		row{"", "note", "something"},
	)
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if !strings.HasPrefix(lines[1], indent+markNone+" ") {
		t.Errorf("an unmarked row should get the blank gutter: %q", lines[1])
	}
	if a, c := column(lines[0], "clean"), column(lines[1], "something"); a != c {
		t.Errorf("nouns start at columns %d and %d, want the same:\n%s", a, c, b.String())
	}
}

// markNone stands in for a mark, so it has to be exactly as wide as one. If a
// terminal ever disagrees about the glyphs' width, every marked block skews.
func TestMarksAreTheWidthOfTheBlank(t *testing.T) {
	want := lipgloss.Width(markNone)
	for _, m := range []string{markPass, markFail} {
		if got := lipgloss.Width(m); got != want {
			t.Errorf("%s is %d columns, want %d to match the blank gutter", m, got, want)
		}
	}
}

// An unmarked block is untouched: every command but `enzo finish` reports
// actions, not checks.
func TestRowsLeaveUnmarkedBlocksAlone(t *testing.T) {
	var b bytes.Buffer
	rows(&b, row{"", "created", "a-branch"})
	if got, want := b.String(), indent+"created: a-branch\n"; got != want {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

func TestHeadline(t *testing.T) {
	var b bytes.Buffer
	headline(&b, emojiStart, "starting work on Issue #%d %q", 5, "hello again")
	want := "🟢 starting work on Issue #5 \"hello again\"\n"
	if b.String() != want {
		t.Errorf("headline = %q, want %q", b.String(), want)
	}
}

// One emoji per command, and no two commands sharing one — the point of them
// is telling apart two reports in the same scrollback.
func TestEmojiAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for name, e := range map[string]string{
		"new": emojiNew, "start": emojiStart, "abort": emojiAbort,
		"finish": emojiFinish, "setup": emojiSetup,
	} {
		if other, dup := seen[e]; dup {
			t.Errorf("%s and %s share %q", name, other, e)
		}
		seen[e] = name
	}
}
