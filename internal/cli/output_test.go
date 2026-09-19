package cli

import (
	"bytes"
	"strings"
	"testing"
)

// The nouns line up in a column, so a block of rows reads down the page.
func TestRowsAlignNouns(t *testing.T) {
	var b bytes.Buffer
	rows(&b,
		row{"switched to", "main"},
		row{"closed", "PR #4"},
		row{"left", "#2 open"},
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
		if !strings.HasPrefix(line, "  ") {
			t.Errorf("row is not indented: %q", line)
		}
	}
}

// A single row still gets its colon and its indent.
func TestRowsSingle(t *testing.T) {
	var b bytes.Buffer
	rows(&b, row{"url", "https://example.invalid/1"})
	if got, want := b.String(), "  url: https://example.invalid/1\n"; got != want {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

// printRow is what abort uses, pinned to the same width its rows are printed
// with, so the block lines up though it is written a line at a time.
func TestPrintRowMatchesRows(t *testing.T) {
	var one, many bytes.Buffer
	printRow(&one, abortWidth, "closed", "PR #4")
	rows(&many, row{"switched to", "x"}, row{"closed", "PR #4"})

	if want := strings.Split(many.String(), "\n")[1] + "\n"; one.String() != want {
		t.Errorf("printRow = %q, want %q", one.String(), want)
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
