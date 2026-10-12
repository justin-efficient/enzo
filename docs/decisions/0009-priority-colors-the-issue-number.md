# 9. `enzo list` colors the issue number by board priority

- **Date:** 2026-10-12
- **Status:** decided — implemented
- **Affects:** `enzo list`, `internal/ghclient`, `internal/ui`
- **Revisits:** [0001](0001-no-priority-column.md)

## Context

0001 declined a priority *column* because priority is a Projects v2 field, which
needs GraphQL, the `read:project` scope and a rule for issues on several boards,
all for a column that is blank wherever there is no board. GraphQL arrived with
[0006](0006-finish-needs-graphql.md), and a color on the number needs no column
and no layout, so the cost is now one query.

## Decision

The `#number` in the picker is drawn in the color GitHub gives the issue's
`Priority` option (`RED`, `ORANGE`, `YELLOW`, `GREEN`, `BLUE`, `PURPLE`, `PINK`,
`GRAY`). The name is not shown; enzo does not need to know `P0` from `High`.

- One GraphQL query per render, paged, filtered to open issues assigned to you.
- Read from the board, never from labels (see 0001).
- An issue on several boards takes the first priority GitHub lists. There is no
  config to pin a board; add one if that turns out to matter.
- **Decoration, not data.** A token without `read:project`, an issue on no
  board, or a failed query leaves the number in the default gray. It never
  fails the list and prints nothing.
- Only the picker is colored. `--plain` output is unchanged.
