# 12. The issue forms have an iteration selector

- **Date:** 2026-10-13
- **Status:** decided — implemented
- **Affects:** `enzo list`, `enzo new`, `internal/ghclient`, `internal/ui`
- **Builds on:** [0010](0010-priority-selector-in-the-editor.md), [0011](0011-setup-pins-the-priority-board.md)

## Decision

When the board an issue is prioritized on also has an **Iteration** field
(a Projects v2 iteration field; the System software board has one), the edit
and new-issue forms get a fourth field, **iteration**, after priority.
`←`/`→` step through the board's current and upcoming iterations, wrapping.

- **Same board as priority.** The pinned board (0011), else the issue's own.
  A board with no Iteration field, or no iterations, gets no selector.
- **Starts on the current value, else "none".** There is no sensible default
  iteration, so unlike priority nothing is guessed.
- **Only changes are written.** Leaving it alone, or on "none", writes nothing.
- **Finished iterations are not offered.** GitHub lists them separately. An issue
  in one keeps it until you pick another.
- **One at a time on screen.** Boards can have many iterations, so the row shows
  the chosen one with its dates and position (`‹ Iteration 64 (Oct 19 – Nov 8 · 1/8) ›`)
  instead of all of them side by side.
- New issues are added to the board and moved into the iteration, as with
  priority. Saves are logged as `iterated`.
- Writing needs the `project` token scope, as priority does.

## Not built

- **Clearing an iteration.** The selector can only choose one.
- **Boards with an iteration field but no Priority field**, which get no selector:
  the board is found through its Priority field.
