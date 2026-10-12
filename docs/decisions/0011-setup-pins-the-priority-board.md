# 11. `enzo setup` pins the board priorities live on

- **Date:** 2026-10-12
- **Status:** decided — implemented
- **Affects:** `enzo setup`, `.enzo`, `enzo new`, `enzo list`
- **Builds on:** [0010](0010-priority-selector-in-the-editor.md)

## Context

0010 had new issues guess their board from the issues assigned to you, and left
an issue on no board without a selector (eff_libs#259). A survey of apps,
riptools, eff_sdk, eff_libs and chip showed that works when you have a single
board, but riptools has two, and nothing says where an unboarded issue belongs.

## Decision

`enzo setup` picks a board and writes it to `.enzo`:

```json
{ "token": "...", "project": { "owner": "EfficientComputer", "number": 12 } }
```

- **Candidates** are the open boards with a Priority field that the repository
  is linked to, then the ones your assigned issues are on. The **first** is
  chosen. The rest are listed in setup's report, so changing it is an edit of
  `.enzo`, not a prompt.
- **Re-running setup keeps a pin**, including one set by hand that is not among
  the candidates. It does not hop to whichever board is first today.
- No candidates, or a token that cannot read boards: setup carries on without
  one, and enzo falls back to the guess from 0010.

With a pinned board:

- **New issues** offer that board's priorities, and are added to it on create.
- **Editing an issue not on the board adds it on save, with no confirmation.**
  This happens whether or not the priority changed, because being on the board
  is the point of saving. Backing out adds nothing. An issue already on the board
  keeps its item and is not added again.
- If the pinned board cannot be read at run time, enzo uses the issue's own board
  as before, so a deleted or renamed board never blocks editing.

Without a pinned board, an unboarded issue still gets no selector and is not
added anywhere: there is nowhere to put it.

## Costs

- Setup now needs `read:project`, and saving a priority needs `project`.
- Pulling issues onto a board is an irreversible-feeling write you do not see
  coming; it is documented in the form's help and here instead of confirmed.
- The first-candidate default is still a guess, now made once and visible.
