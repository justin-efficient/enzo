# 8. Enter in `enzo list` edits the issue; ctrl+o opens it

- **Date:** 2026-10-12
- **Status:** decided — implemented
- **Affects:** `enzo list`, `internal/ui`, `internal/ghclient`
- **Revisits:** [0007](0007-enter-opens-the-issue.md)

## Context

Decision 0007 made enter open the highlighted issue in a browser. Most of the
time you look at an issue in the list to fix its title or tidy its body, and a
round trip through the browser is a lot for that.

## Decision

- **Enter** opens an inline form, the same one `enzo new` uses, filled with the
  issue's title and body. `ctrl+s` saves and `esc` backs out; enter edits text
  as it does in the new-issue form (it moves on from the title, and is a newline
  in the body).
- **`ctrl+o`** opens the issue in a browser, which is what enter did.

0007's reasoning still holds: nothing in the picker starts work. Editing
changes one issue's text, and unlike `enzo start` it is one remote write you
asked for by name.

## Details

- The issue is **re-read before the form opens**. The listing lags and can be
  stale, and saving over a body edited elsewhere a moment ago would lose it.
- The body is edited **as GitHub has it**, including the signature footer on
  issues enzo created. Hiding the footer would mean parsing it back out.
- CRLF line endings are turned into LF for editing. A save that changes nothing
  once line endings and surrounding whitespace are ignored writes nothing.
- Like creating, saving **waits for the listing to show the change** rather
  than splicing in what enzo remembers (see [0003](0003-wait-for-github-to-catch-up.md)).
- Saves are logged as `edited`.
- The parent picker in `enzo new sub` is unchanged: enter selects, and ctrl+o
  does nothing there.

## Reversed: enter does not save

For a short while enter saved and closed the edit form, with `alt+enter` for a
newline in the body. That made editing a multi-line body awkward and made the
two forms behave differently, so it was reverted: in both forms **`ctrl+s`
saves** (`ctrl+n` still works in the new-issue form) and enter is always text.
