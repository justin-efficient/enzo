package cli

import (
	"fmt"
	"io"
)

// Emoji open a command's report. One per command, so what enzo just did is
// recognisable before a word of it is read, and so the same command always
// looks the same in a scrollback full of them.
//
// emojiFinish is here although `enzo finish` does not exist yet. The set is
// the thing being designed, not each command's decoration, so the choice
// belongs beside the others rather than with whoever writes the command.
const (
	emojiNew    = "✨"
	emojiStart  = "🟢"
	emojiAbort  = "❌"
	emojiFinish = "🏁"
	emojiSetup  = "🔑"
)

// headline opens a report: the command's emoji, then what it is doing.
func headline(w io.Writer, emoji, format string, a ...any) {
	fmt.Fprintf(w, "%s %s\n", emoji, fmt.Sprintf(format, a...))
}

// indent is the left margin for everything under a headline. Three columns,
// which is what the headline's own text is inset by: an emoji is two columns
// wide, plus the space after it. Two would very nearly line up, which reads
// worse than either lining up or plainly not.
const indent = "   "

// row is one line under a headline: what was done, and what it was done to.
type row struct{ verb, noun string }

// rows prints a block of rows with their nouns in a column, and returns the
// width it lined them up at. A later row that belongs to the same block —
// `enzo finish` prints its result only once the merge has gone through — is
// printed with printRow at that width, so it joins the column rather than
// starting one of its own.
func rows(w io.Writer, rr ...row) int {
	width := 0
	for _, r := range rr {
		if n := len(r.verb); n > width {
			width = n
		}
	}
	for _, r := range rr {
		printRow(w, width, r.verb, r.noun)
	}
	return width
}

// printRow prints one row, padding the verb out to width.
//
// It is separate from rows because `enzo abort` prints each line as that step
// succeeds — a report of what actually happened, ending wherever it ended —
// and so cannot measure the block before it starts printing it.
func printRow(w io.Writer, width int, verb, noun string) {
	fmt.Fprintf(w, "%s%-*s %s\n", indent, width+1, verb+":", noun)
}

// abortWidth is the widest verb `enzo abort` reports with, so its rows line up
// despite being printed one at a time.
const abortWidth = len("switched to")
