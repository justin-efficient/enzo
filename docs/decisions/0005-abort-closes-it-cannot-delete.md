# 5. `enzo abort` closes the pull request, because nothing can delete one

- **Date:** 2026-09-18
- **Status:** decided — implemented
- **Affects:** `enzo abort`, `internal/ghclient`, `internal/gitrepo`

## Context

`enzo abort` was specified as "delete the PR and the branch, if user confirms
by typing `nukefromorbit`".

## What we found

**A pull request cannot be deleted.** Not in the web UI, not in REST, not in
GraphQL, not by a repository admin, not by an organization owner. GitHub offers
no delete operation for pull requests at any permission level. The number and
the conversation are permanent once opened.

This is easy to get wrong because **issues are different**: an issue *can* be
deleted, by a repository admin, through the GraphQL `deleteIssue` mutation —
see [decision 0001](0001-no-priority-column.md) for why enzo does not reach for
GraphQL. The asymmetry is GitHub's, not ours.

Branches are unremarkable: deleting one needs push access, not admin.

| operation | possible | permission |
| --- | --- | --- |
| delete a pull request | **no** | — |
| close a pull request | yes | write |
| delete a remote branch | yes | write |
| delete an issue | yes | admin, GraphQL only |

## Decision

`enzo abort` closes the pull request and deletes the branch, locally and on
origin. It needs no admin rights. The command keeps the name — it is what the
user is doing — but the prompt says plainly that a pull request can only be
closed, so nobody is left expecting the number to disappear.

**The issue is left open.** Abandoning an attempt is not abandoning the work:
the usual reason to abort is to start the same issue again from a clean branch,
and closing the issue would make `enzo start <n>` the wrong next command.
`ghclient.Client` has no way to close an issue at all, so this is enforced by
the interface rather than by the command.

## Order of operations

Everything reversible happens before anything that is not, and the step most
likely to be refused goes first:

1. **switch to the base branch** — refused if uncommitted work cannot be
   carried, in which case nothing has been destroyed and enzo says so;
2. **close the pull request** — reopenable, and GitHub offers "Restore branch"
   on a closed PR for a while afterwards;
3. **delete the branch on origin**;
4. **delete the local branch**, with `-D`: an aborted branch is unmerged by
   definition.

## The confirmation

The phrase is `nukefromorbit`, not `y`. Before it is asked for, the prompt
names what will not survive: commits that are not on origin, and files with
uncommitted changes. Warning after the fact would be decoration.

Typing anything else cancels and touches nothing. A phrase read from a pipe
counts — the phrase is the safety, not the terminal — so the command is
scriptable without a `--force` flag that would be easier to fire by accident
than the phrase itself.

## Scope

`enzo abort` takes no arguments and acts only on the branch you are standing
on, and only when that branch is one enzo named (`<login>/<number>-<title>`).
Anything else is refused. `parseBranch` is the inverse of `branchName`, and a
test asserts it round-trips, so the two cannot drift apart into a command that
refuses branches enzo itself created.
