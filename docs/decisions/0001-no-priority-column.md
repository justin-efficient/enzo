# 1. No priority column in `enzo list`

- **Date:** 2026-09-18
- **Status:** decided — not implementing
- **Affects:** `enzo list`, `internal/ghclient`, `internal/ui`

## Context

We wanted `enzo list` to show a priority column, rendered as
`#<number> P1 <title>`, colored the way GitHub colors priority.

The assumption behind the request was that priority is an intrinsic GitHub
issue field, the way title, assignee and labels are. It is not.

## What we found

Priority on GitHub lives in **Projects v2 custom fields**, not on the issue.
An issue carries no priority of its own; a *project item* — the row created
when an issue is added to a board — carries a `Priority` single-select value.

Surveying the `EfficientComputer` org (18 projects, 2026-09-18):

| Finding | Detail |
| --- | --- |
| Projects with a `Priority` field | 12 of 18 |
| Dominant convention | `P0` / `P1` / `P2` / `P∞` → RED / ORANGE / YELLOW / GRAY (11 projects) |
| Divergent convention | Maestro (#23): `High` / `Medium` / `Low` / `Unplanned` → RED / YELLOW / GREEN / GRAY |
| Colors | Returned per item by the API, so they never need to be hardcoded |

`EfficientComputer/eff_libs#142` is the clearest illustration: it has **no
labels at all**, yet the board shows it as a red `P0`.

## Why we stopped

A first pass derived priority from `P0`–`P9` labels. That implementation was
wrong in two independent ways, and the second is the one that mattered:

1. **Wrong source.** Issues that are prioritized on a board frequently carry no
   labels, so the column would have been blank exactly where it was most
   needed.
2. **Wrong value model.** A `P0`–`P9` integer cannot represent `P∞`, and cannot
   represent `High` / `Medium` / `Low` at all. Any numeric priority type is
   wrong for at least one team in this org.

The correct design — read the single-select option's **name and color straight
from GitHub and render them verbatim** — handles every convention without
enzo knowing any of them. But it carries real cost:

- **GraphQL.** Projects v2 has no REST API. `go-github` is REST-only, so this
  means a second client and a second set of error paths.
- **A token scope.** `read:project`, on top of `repo`.
- **A disambiguation rule.** An issue can sit on several boards with different
  priorities, so the config needs a way to pin one.
- **A new failure mode.** Issues on no board still have no priority, so the
  column is empty in any repo that is not on one — including this one.

That is a lot of machinery for one column, in a tool whose README says it is
tiny, simple and stateless. We chose not to pay it.

## Decision

`enzo` does not show priority. It shows what an issue intrinsically has:
number, title and labels.

## If this is revisited

Do not reintroduce label-derived priority — it is wrong for this org. Start
from the project field instead:

```graphql
query($owner:String!, $name:String!, $login:String!) {
  repository(owner:$owner, name:$name) {
    issues(first:100, states:OPEN, filterBy:{assignee:$login}) {
      nodes {
        number title
        projectItems(first:10) {
          nodes {
            project { number title }
            fieldValueByName(name:"Priority") {
              ... on ProjectV2ItemFieldSingleSelectValue { name color }
            }
          }
        }
      }
    }
  }
}
```

Render `name` using `color` (GitHub returns `RED`, `ORANGE`, `YELLOW`, `GREEN`,
`BLUE`, `PURPLE`, `PINK`, `GRAY`), add an optional `project` number to `.enzo`
to disambiguate multi-board issues, and treat a missing `read:project` scope as
"no priority" rather than an error.
