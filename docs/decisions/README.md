# Decisions

One file per decision that shaped enzo, newest last. These record *why*
something is the way it is — especially where we chose not to build something,
so the same ground is not re-covered later.

- [0001 — No priority column in `enzo list`](0001-no-priority-column.md)
- [0002 — `enzo list` does not re-read the list after creating an issue](0002-no-refetch-after-creating.md) — superseded by 0003
- [0003 — `enzo list` waits for GitHub rather than remembering what it created](0003-wait-for-github-to-catch-up.md)
- [0004 — `enzo start` puts an empty commit on a new branch](0004-draft-pr-needs-a-commit.md)
- [0005 — `enzo abort` closes the pull request, because nothing can delete one](0005-abort-closes-it-cannot-delete.md)
- [0006 — `enzo finish` uses GraphQL, because REST cannot undraft a pull request](0006-finish-needs-graphql.md) — revisits 0001
