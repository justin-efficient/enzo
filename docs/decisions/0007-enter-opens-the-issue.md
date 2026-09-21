# 7. Enter in `enzo list` opens the issue, and nothing starts it

- **Date:** 2026-09-21
- **Status:** decided — implemented
- **Affects:** `enzo list`, `internal/ui`, `internal/browser`

## Context

`enzo list` showed a picker of the open issues assigned to you, and enter on a
row ran `enzo start` on it. The help line called this "enter select".

## The problem

"Select" reads like "look at this one". What it did was branch off a freshly
fetched `main`, write a commit if the branch had nothing on it, push, and open
a draft pull request — five remote and worktree changes, from the key you press
to stop moving around a list.

Arrow keys and enter are how you browse. Hanging the most consequential command
in the tool off the end of that gesture means the list cannot be browsed at
all: every row is one keystroke from a push.

## Decision

Enter **opens the highlighted issue in a browser** and returns to the list.

Nothing in the picker starts work any more. Starting is `enzo start <n>`, typed
deliberately, and that is the only way in.

We considered moving start to `ctrl+s`, next to the existing `ctrl+n`, and
decided against it: the list is now a place to look at what you have, and the
command that changes five things on a remote is worth typing out.

## What that cost

- A `internal/browser` package — `open` on macOS, `rundll32` on Windows,
  `xdg-open` everywhere else. `xdg-open` already honours `$BROWSER`, so enzo
  does not read it.
- `Env.OpenURL`, so the suite records what would have been opened instead of
  launching anything.
- Opening is not terminal, so `enzo list` loops back and re-reads the issues,
  the same as it already did after creating one. That is one extra listing call
  per look, which is the price of enzo keeping no state.

A browser that will not open is a warning with the URL in it, not a failed
command — the same rule the log follows.
