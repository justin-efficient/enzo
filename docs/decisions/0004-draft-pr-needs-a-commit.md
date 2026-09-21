# 4. `enzo start` puts an empty commit on a new branch

- **Date:** 2026-09-18
- **Status:** decided — implemented
- **Affects:** `enzo start`, `internal/gitrepo`

## Context

`enzo start` is meant to leave you on a branch with a draft pull request open
against an issue, so the work has somewhere to go before any of it exists.

A branch created at the tip of the default branch is identical to it. GitHub
refuses to open a pull request between two identical refs:

```
422 Unprocessable Entity
No commits between main and justin-efficient/12-fix-the-thing
```

So "create the branch, then create the PR" cannot work as written. Something
has to be on the branch first.

## Options

| Option | Cost |
| --- | --- |
| **Empty commit, then push** | One throwaway commit on the branch |
| Create the branch, then tell the user to commit and re-run | `enzo start` does not finish what it was asked to do; the PR half of the command is deferred to a second run the user has to remember |
| Push and let GitHub reject it | The user sees a raw 422 for a situation enzo could have handled |

## Decision

`enzo start` commits `--allow-empty` with the subject `Created by a human with
🚘 enzo vX.Y.Z`, pushes the branch, and opens the draft PR against the
repository's default branch.

The subject names the tool and its version rather than the issue, because the
question someone asks on finding an empty commit at the root of a branch is
"what put this here?" — the issue is already in the branch name and in the pull
request.

The commit is made **only when the branch has nothing the base does not**, so
running `enzo start` on a branch you have already worked on adds nothing.

**The comparison fetches.** GitHub measures the pull request against the base
as *GitHub* has it, so enzo has to as well. `refs/remotes/origin/<base>` on
disk is only as fresh as the last fetch, and it goes stale in the most ordinary
way there is: a pull request merges, `main` moves, and nothing local notices.
Comparing against the stale ref then reads as "this branch is ahead" for a base
that has since absorbed those very commits — enzo skips the empty commit, and
GitHub refuses with `No commits between main and <branch>`.

This was not hypothetical; it is how the bug was found, one merged PR after
`enzo start` first worked. A fetch that fails falls back to the local refs
(`origin/<base>`, then `<base>`): offline there is nothing better to consult,
and there is no way to push either, so the pull request was not happening
regardless. A base that resolves to nothing at all counts as needing a commit,
because a spare empty commit is easy to drop and a 422 is not easy to read.

**A new branch is cut from the base**, not from `HEAD`. Cutting from `HEAD` is
what `git switch -c` does on its own, and it was enzo's first behaviour, but it
means running `enzo start` while standing on another feature branch puts that
branch's commits in the new pull request. Two issues' work then arrives in one
review. Starting from the base is the only thing that makes the resulting PR
mean what it says.

A branch that **already exists** is switched to as it is. enzo does not move
someone's branch onto the base behind their back; rebasing is a decision with
consequences for anyone who has fetched it.

## Consequence

Every branch enzo starts has one empty commit at its root. `git commit --amend`
replaces it with real work, and it squashes away to nothing on merge.

The alternative — no commit, no PR until the user makes one — was rejected
because it splits one command into two and leaves the tool's central promise
("enzo keeps PRs and issues linked") conditional on the user coming back.
