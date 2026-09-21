# Design notes

Why enzo behaves the way it does. The README says what it does; this says why.
A single decision with a clear before and after lives in
[decisions](decisions/) instead — this file is the standing reasoning that no
one decision covers.

## Output

Every command that reports writes the same shape: an emoji headline naming what
it is doing, then indented `verb: noun` rows naming what it did, with the nouns
in a column.

| command | emoji | headline |
| --- | --- | --- |
| `enzo setup` | 🔑 | `set up enzo for <repo>` |
| `enzo new` | ✨ | `created a new issue #<n>, "<title>"` |
| `enzo start` | 🟢 | `starting work on Issue #<n> "<title>"` |
| `enzo abort` | ❌ | `aborting work in <repo>:` |
| `enzo finish` | 🏁 | `finishing Issue #<n> "<title>"` |

```
$ enzo start "fix the parser crash"
✨ created a new issue #12, "fix the parser crash"
   url: https://github.com/justin-efficient/enzo/issues/12
🟢 starting work on Issue #12 "fix the parser crash"
   created: justin-efficient/12-fix-the-parser-crash
   drafted: PR #13 https://github.com/justin-efficient/enzo/pull/13
```

The verbs are what enzo *did*, not what the command is for. Starting an issue
that is already started says so rather than claiming to have redone the work:

```
$ enzo start 12
🟢 starting work on Issue #12 "fix the parser crash"
   switched to: justin-efficient/12-fix-the-parser-crash
   found:       PR #13 https://github.com/justin-efficient/enzo/pull/13
```

`enzo abort` inverts the rows, because it is asking rather than reporting:
before the phrase each row is `noun : what will become of it`, and afterwards
it reports in the usual `verb: noun` form as each step succeeds.

The emoji live in one place, `internal/cli/output.go`, alongside the helpers
that print the headline and the rows. `enzo list` is exempt: it owns the
terminal, and anything printed under it is drawn over by the next frame.

`enzo list` opens the picker only when stdin and stdout are both terminals, so
it composes in pipes and scripts without a flag.

## The log

Creating an issue from the picker prints nothing. What enzo creates goes to the
log instead, so the picker is not interrupted by output and there is still a
record afterwards. The log is for whoever is working on enzo — it is not part
of the interface, and nothing reads it back.

enzo appends a line for each thing it creates:

```
2026-09-18T01:41:53Z justin-efficient/enzo created #11 bork2 https://github.com/justin-efficient/enzo/issues/11
2026-09-18T01:44:02Z justin-efficient/enzo linked #12 under #1 https://github.com/justin-efficient/enzo/issues/12
2026-09-18T02:10:14Z justin-efficient/enzo drafted PR #13 for #11 on justin-efficient/11-bork2 https://github.com/justin-efficient/enzo/pull/13
2026-09-18T02:31:07Z justin-efficient/enzo aborted PR #13 for #11 on justin-efficient/11-bork2 https://github.com/justin-efficient/enzo/pull/13
```

It lives at `~/.local/state/enzo/enzo.log` (`$XDG_STATE_HOME/enzo/enzo.log`
when that is set), and is overridden by `$ENZO_LOG`, or by `"log"` in `.enzo`,
which wins over both. It is mode `0600` because it names private repositories.

A log that cannot be written is a warning on stderr, never a failed command:
enzo will not tell you an issue was not created when it was.

## Signing

Every issue and pull request enzo opens carries a `Created by a human with 🚘
enzo` footer, so an issue that turns up with no obvious author says where it
came from. An issue opened with no body at all is still signed; that is the
case where the question gets asked.

The footer credits the person, not the tool. enzo opened the issue, but
somebody asked it to, and a footer naming only the tool invites the reader to
file it as machine output and stop reading.

Everything that shows a version calls `version.Banner()` or `version.Credit()`:

| where | which |
| --- | --- |
| `enzo --version` | `Banner()` |
| the first line of `enzo help` | `Banner()` |
| the help line under the issue list | `Banner()` |
| the footer of an issue `enzo new` opens | `Credit()` |
| the footer of a pull request `enzo start` opens | `Credit()` |
| the empty commit `enzo start` puts at the root of a branch | `Credit()` |

A hardcoded copy of that string anywhere else fails the suite, because it would
keep printing the old number after a bump.

## Listing

Enter opens the highlighted issue in a browser and comes back to the list. It
used to run `enzo start`, which branches, commits, pushes and drafts a pull
request — five changes from the key you press to stop moving around a list.
Nothing in the picker starts work now; `enzo start <n>` is the only way in. See
[decision 0007](decisions/0007-enter-opens-the-issue.md).

Because opening is not terminal, the picker loop re-reads the issues afterwards,
the same as it does after creating one. A browser that will not open is a
warning with the URL in it, never a failed command.

The parent picker `enzo new sub` shows keeps enter as "select": it is a chooser,
and choosing is all it does. That is why the help line's enter verb is a field
on the picker rather than a constant.

Nesting in `enzo list` comes from the listing itself, so it costs no extra API
calls. An issue whose parent is not in the list — not assigned to you, closed,
or in another repository — stays at the top level rather than disappearing.

GitHub's issue listing takes a few seconds to catch up with a creation, so
after creating, enzo waits until the listing has the new issue rather than
splicing in what it remembers. The list is always exactly what GitHub reported;
enzo remembers nothing between renders. See
[decision 0003](decisions/0003-wait-for-github-to-catch-up.md).

Priority is not shown, because GitHub issues do not have one. See
[decision 0001](decisions/0001-no-priority-column.md).

## Starting work

`enzo start` is the one command between an issue and a branch with a draft pull
request on it. It creates whichever of the three does not exist yet, so running
it twice on the same issue settles rather than doing anything again.

An issue number *and* a title is an error: #12 already has a title, and two
sources for it is a mistake worth naming rather than silently resolving.

A new branch is cut from origin's default branch, freshly fetched — not from
whatever you were standing on — so one issue's work never arrives carrying
another's.

GitHub will not open a pull request between two identical branches, so a branch
with nothing the base does not have gets an empty commit to hang one on. enzo
fetches the base first: a stale `origin/main` makes a branch look ahead of a
base that has already absorbed it, which is exactly the state GitHub rejects.
See [decision 0004](decisions/0004-draft-pr-needs-a-commit.md).

The `Closes #12` keyword in the pull request body is what keeps the PR and the
issue linked, and what lets `enzo finish` close the issue by merging.

Every GitHub read happens before the worktree is touched, so a call that was
going to fail leaves you on the branch you started on rather than stranded on a
new one.

Starting a closed issue is a warning rather than a refusal — you may be
reopening work.

## Finishing

**The worktree is the first check, and it blocks like any other.** Uncommitted
work is not in the pull request, so merging would ship something other than
what you have. It reports and joins the list of blockers rather than cutting
the run short, because the value of this report is that one run names
everything that is wrong.

That does mean a dirty worktree still gets as far as undrafting, which asks
CODEOWNERS for review. Undrafting has to come first for the rows beneath it to
be true — a draft reports its merge state and its review decision differently —
so the alternative was a complete report or an untouched pull request, and the
complete report won.

Untracked files count. A file you never added is the case most worth catching —
the pull request merges without it — and it is the same rule `enzo abort`
already uses when it warns about what will move to the default branch.

**Every check row opens with ✅ or ❌**, and the mark means only "this check is
satisfied". `enzo finish` is the only command that marks its rows: the others
report actions they took, which either happened or stopped the command, so a
tick beside them would say nothing. The marks live in `internal/cli/output.go`
with the headline emoji, and `markNone` is the two-space blank an unmarked row
gets inside a marked block — both marks are two columns wide, which a test
pins, or every marked block would skew.

The base branch row is the one that can be crossed without blocking: **reported,
not enforced**. A red `main` is worth seeing
before you add to it, but it is not yours to fix and it does not make your work
unmergeable. Everything else that fails stops the merge.

**enzo requests no reviewers of its own.** GitHub already asks CODEOWNERS when
a pull request leaves draft; enzo reports who was asked and waits. Nobody is
notified by a decision enzo made.

A run that refuses prints all five checks and then says why, so one run names
everything that is wrong rather than one thing per attempt.

**Taking a pull request out of draft is the one thing REST cannot do.**
`PATCH /pulls/{n}` with `draft: false` answers 200 and changes nothing, so this
is the command that put a small GraphQL client in `internal/ghclient`. See
[decision 0006](decisions/0006-finish-needs-graphql.md).

## Aborting

**The issue is left open.** Aborting an attempt is not abandoning the work —
the usual reason to abort is to start the same issue again from a clean branch,
and `enzo start 12` is exactly the right next command.

**A pull request cannot be deleted.** Not by you, not by an admin, not through
any API — GitHub only lets one be closed. Issues are the confusing exception:
those can be deleted, by an admin, through GraphQL. See
[decision 0005](decisions/0005-abort-closes-it-cannot-delete.md).

The confirmation phrase is the safety rather than the terminal, so
`echo nukefromorbit | enzo abort` works in a script. There is no `--force`,
which would be easier to fire by accident than the phrase.
