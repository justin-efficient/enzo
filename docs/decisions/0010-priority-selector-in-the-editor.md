# 10. The issue forms have a priority selector

- **Date:** 2026-10-12
- **Status:** decided — implemented
- **Affects:** `enzo list`, `internal/ghclient`, `internal/ui`
- **Builds on:** [0009](0009-priority-colors-the-issue-number.md)

## Decision

The edit form `enzo list` opens on enter, and the new-issue form, get a third
field, **priority**, after title and body. `tab` reaches it, `←`/`→` change it, `ctrl+s` saves. The chosen
option is written to the issue's `Priority` field on its project board.

- **The choices are the board's.** They are read from the Priority field of the
  board the issue is on, so `P0`/`P1`/`P2`/`P∞` and `High`/`Medium`/`Low` both
  work, in the colors GitHub gives them.
- **Default P2.** The selector starts on the issue's current priority, or on
  `P2` when it has none. A board with no option named `P2` starts with nothing
  chosen, and saving writes nothing.
- **Only changes are written.** Leaving the selector alone on an issue that
  already has that priority writes nothing. An issue with no priority that is
  saved with the default shown *does* get P2.
- **No board, no selector.** An issue on no board with a Priority field, or a
  token that cannot read boards, gets the form without the selector. An issue on
  several such boards uses the first, as 0009 does.
- **Writing needs the `project` token scope**, not just `read:project`. If
  setting fails after the text was saved, the error says the text went through.
- Saves are logged as `prioritized`.

## New issues

A new issue is on no board yet, and a repository need not be linked to one
(`eff_libs` is not), so there is nothing to read the choices from. enzo uses the
**first board with a Priority field that any open issue assigned to you is on**.
After creating the issue it adds it to that board and sets the priority. Adding
is idempotent in GitHub's API, so a board that auto-adds new issues is fine.

- No such board, or a token that cannot read boards: no selector, and nothing
  is added anywhere.
- The selector is only on the form. `enzo new --title ...` skips the form, so it
  never touches a board.
- If adding or setting fails, the issue already exists and the error says so.
- Guessing the board from your assigned issues is a heuristic. If it picks the
  wrong one, the fix is a `project` setting in `.enzo`; not built yet.
