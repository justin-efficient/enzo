# 🚘 enzo

A tiny CLI for GitHub that makes issue lifecycle management easier.

enzo keeps PRs and issues linked.

enzo is stateless.

enzo is pretty.

## Commands

| command | status | what it does |
| --- | --- | --- |
| `enzo setup` | **done** | create the `.enzo` file with the token used to reach this repo |
| `enzo list` | **done** | list the open issues assigned to me in current repo — selecting one starts it, the top option is "new", `ctrl+n` opens a sub-issue of the highlighted one, esc cancels |
| `enzo new [sub] [issue-number]` | **done** | create an new issue, or sub issue of current and assign to me |
| `enzo start [issue-number] ["title"]` | **done** | calls `enzo new` if the issue doesn't exist. Then branch off `main`, push, and create a linked draft PR with no reviewers (if the PR doesn't exist). Then switch to that local branch.
| `enzo review` | planned | take the current branch's PR out of draft and attach the repo's default reviewers |
| `enzo finish` | planned | merge the current branch once the PR is ready — waits on, skips, or cancels against pending builds |
| `enzo abort` | **done** | close the PR and delete the branch, here and on origin, if user confirms by typing "nukefromorbit". GitHub cannot delete a PR, only close it

Running `enzo` with no arguments is the same as `enzo list`.

## Setup

`enzo setup` stores a GitHub token with `repo` scope in `.enzo` at the
repository root, mode `0600`, and adds `.enzo` to `.gitignore`. The token is
checked against GitHub before anything is written.

```sh
enzo setup                        # prompts, masked
enzo setup --token ghp_xxx        # non-interactive
echo "$TOKEN" | enzo setup        # from stdin
enzo setup --host https://ghe.internal/api/v3/   # GitHub Enterprise
```

If you would rather not keep a token on disk, enzo also reads `$ENZO_TOKEN`
and `$GITHUB_TOKEN`, in that order, after `.enzo`.

## New issues

`enzo new` opens an issue in the current repo, assigned to you. With no title
it shows a form: tab switches between title and body, `ctrl+n` creates, `esc`
cancels. A title is required.

```sh
enzo new                                  # form
enzo new "fix the thing"                  # title as an argument, empty body
enzo new "fix it" --body "detail"
enzo new --title "fix the thing"          # --title works too
enzo new sub "fix it"                     # pick the parent from a list
enzo new sub 12                           # a sub-issue of #12, title prompted
enzo new sub 12 "fix it"                  # a sub-issue of #12 with that title
```

Every issue enzo opens is signed. The body it sends is what you wrote, a
horizontal rule, then `Created by 🚘 enzo v0.2.0` in italics — so an issue that
turns up with no obvious author says where it came from. An issue opened with
no body at all is still signed; that is the case where the question gets asked.
Pull requests `enzo start` opens get the same footer.

A title given on the command line skips the form entirely, so the body is left
to the footer unless `--body` says otherwise. A bare number straight after `sub` is the
parent issue; anything else there is the title. Quote the title — enzo rejects
an unquoted one rather than guessing where it ends.

Sub-issues use GitHub's native parent/child link, so they show up in the
parent's sub-issue list and progress bar.

Picking "new" in `enzo list` runs the same flow and then returns to the list,
with the issue you just opened already in it. Backing out of the form returns
to the list too. Esc on the list itself is what exits.

GitHub's issue listing takes a few seconds to catch up with a creation, so
after creating, enzo shows a spinner until the listing has the new issue — and
for a sub-issue, its parent link too — then draws the list from that response.
The list is always exactly what GitHub reported; enzo remembers nothing between
renders. Esc stops waiting early and shows the list as it stands. See
[decision 0003](docs/decisions/0003-wait-for-github-to-catch-up.md).

In the list, `ctrl+n` on a highlighted issue opens a sub-issue of it — no need
to name the parent, it is the row you are on. On the "new" row `ctrl+n` opens a
top-level issue, the same as enter.

Creating an issue prints nothing. What enzo creates goes to the log instead, so
the picker is not interrupted by output and there is a record afterwards.

## Starting work

`enzo start` is the one command between an issue and a branch with a draft pull
request on it. It creates whichever of the three does not exist yet, so running
it twice on the same issue settles rather than doing anything again.

```sh
enzo start 12                 # work on #12
enzo start "#12"              # the same
enzo start "fix the parser"   # open an issue with that title, then work on it
enzo start "fix it" --body "detail"
```

An issue number *and* a title is an error: #12 already has a title, and two
sources for it is a mistake worth naming rather than silently resolving. With
neither, there is nothing to start, which is also an error.

What it does, in order:

1. **The issue** — fetched when you gave a number, opened and assigned to you
   when you gave a title.
2. **The branch** — `<your-login>/<number>-<slugified-title>`, for example
   `justin-efficient/12-fix-the-parser-crash`. The title part is cut at 48
   characters, on a word boundary.

   A new branch is cut from **origin's default branch**, freshly fetched — not
   from whatever you were standing on. Run `enzo start` from the middle of
   another feature branch and the new one still comes off `main`, so one
   issue's work never arrives carrying another's. An existing branch of that
   name is switched to as it is; enzo will not rebase it for you.

   Uncommitted work comes along, the same as a hand-typed `git switch`. When it
   cannot, git refuses and enzo stops there, leaving you where you were.
3. **A commit, if the branch has nothing the base does not** — GitHub will not
   open a pull request between two identical branches, so such a branch gets an
   empty `Created by 🚘 enzo v0.2.0` commit to hang one on. enzo **fetches the
   base first**: a stale `origin/main` makes a branch look ahead of a base that
   has already absorbed it, which is exactly the state GitHub rejects. A branch
   you have really worked on gets nothing. See
   [decision 0004](docs/decisions/0004-draft-pr-needs-a-commit.md).
4. **The push** — `git push -u origin <branch>`.
5. **The draft PR** — titled after the issue, against the repository's default
   branch, with no reviewers, and bodied:

   ```markdown
   Closes #12

   ---

   *Created by 🚘 enzo v0.2.0*
   ```

   That closing keyword is what keeps the PR and the issue linked, and what
   will let `enzo finish` close the issue by merging. The footer is the same
   one an issue body gets, from the same function.

Every GitHub read happens before the worktree is touched, so a call that was
going to fail leaves you on the branch you started on rather than stranded on a
new one. Once enzo starts changing things it stops at the first error and says
what it got done.

Starting a closed issue is a warning on stderr, not a refusal — you may be
reopening work.

In `enzo list`, picking an issue runs exactly this, on the row you highlighted.

## Aborting

`enzo abort` throws away the attempt on the branch you are standing on: the
pull request is closed, the branch is deleted here and on origin. It takes no
arguments — what it destroys is where you are.

```
$ enzo abort
about to destroy, in justin-efficient/enzo:
  branch justin-efficient/12-fix-the-thing, here and on origin
  PR #77 fix the parser crash
  ⚠ 2 commits not on origin — deleting the branch destroys them
  ⚠ 3 files with uncommitted changes, which move to main

#12 stays open. A closed PR cannot be deleted, only closed.
type nukefromorbit to confirm:
```

**The issue is left open.** Aborting an attempt is not abandoning the work —
the usual reason to abort is to start the same issue again from a clean branch,
and `enzo start 12` is exactly the right next command.

**A pull request cannot be deleted.** Not by you, not by an admin, not through
any API — GitHub only lets one be closed. Issues are the confusing exception:
those can be deleted, by an admin, through GraphQL. See
[decision 0005](docs/decisions/0005-abort-closes-it-cannot-delete.md).

Anything that will not survive is named *before* the phrase is asked for.
Typing anything other than `nukefromorbit` cancels and touches nothing. The
phrase is the safety rather than the terminal, so `echo nukefromorbit | enzo
abort` works in a script; there is no `--force`, which would be easier to fire
by accident than the phrase.

enzo only aborts branches it named (`<login>/<number>-<title>`) and refuses
anything else, the default branch included. It switches you to the default
branch before deleting, and if uncommitted work cannot come with you, git
refuses and enzo stops having destroyed nothing.

## Log

enzo appends a line for each thing it creates:

```
2026-09-18T01:41:53Z justin-efficient/enzo created #11 bork2 https://github.com/justin-efficient/enzo/issues/11
2026-09-18T01:44:02Z justin-efficient/enzo linked #12 under #1 https://github.com/justin-efficient/enzo/issues/12
2026-09-18T02:10:14Z justin-efficient/enzo drafted PR #13 for #11 on justin-efficient/11-bork2 https://github.com/justin-efficient/enzo/pull/13
2026-09-18T02:31:07Z justin-efficient/enzo aborted PR #13 for #11 on justin-efficient/11-bork2 https://github.com/justin-efficient/enzo/pull/13
```

It lives at `~/.local/state/enzo/enzo.log` (`$XDG_STATE_HOME/enzo/enzo.log`
when that is set), mode `0600` — it names private repositories. Override it
with `$ENZO_LOG`, or with `"log"` in `.enzo`, which wins over both.

A log that cannot be written is a warning on stderr, never a failed command:
enzo will not tell you an issue was not created when it was.

## Output

`enzo list` opens the picker when stdin and stdout are both terminals, and
prints a plain list otherwise, so it composes in pipes and scripts. `--plain`
forces the plain form.

In the picker, the key hints are signed with the running version:

```
open issues assigned to you in justin-efficient/enzo

> + new issue
  #4 enzo finish should wait on required checks [bug]
  #1 implement enzo start
    #5 sub-issue created by enzo new

🚘 enzo v0.2.0 · ↑/↓ move · enter select · ctrl+n sub-issue · esc cancel
```

Sub-issues are nested under their parent, two spaces per level:

```
#4 enzo finish should wait on required checks
#1 implement enzo start
  #5 sub-issue created by enzo new
```

Nesting comes from the listing itself, so it costs no extra API calls. An issue
whose parent is not in the list — not assigned to you, closed, or in another
repository — stays at the top level rather than disappearing.

## Versioning

enzo is versioned `MAJOR.MINOR.PATCH`, starting at **0.1.0**. The version lives
in `internal/version` and is the source of truth. `version.Banner()` is the one
function that formats it, and `version.Credit()` — "Created by" plus the banner
— is how enzo signs what it leaves behind. Everything that shows a version
calls one of the two:

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

```sh
enzo --version     # 🚘 enzo v0.2.0
```

Cutting a release is a tag: `git tag -a v0.2.0 -m "0.2.0" && git push --tags`.
`make` passes `git describe` into the binary when the tree has a tag, so a
build from three commits past `v0.2.0` reports `0.2.0-3-gabc123` rather than
claiming to be the release. An untagged tree keeps whatever is in source.

Bump `Version` in `internal/version/version.go` in the same commit as the tag.

## Development

```sh
make          # vet, test, build
make test     # go test ./...
make race     # with the race detector
make cover    # coverage summary
make dist     # static linux/amd64 and linux/arm64 binaries in dist/
```

### Layout

- `internal/config` — the `.enzo` file and token resolution
- `internal/gitrepo` — repo root, origin remote parsing, branch, fetch and push
- `internal/ghclient` — the GitHub surface enzo uses, behind a `Client` interface
- `internal/ui` — the bubbletea models (issue picker, token prompt)
- `internal/cli` — command dispatch; everything external arrives through `cli.Env`
- `internal/version` — the version number, in one place

### Testing

Commands take their dependencies through `cli.Env`, so they run without a
terminal or a network. The suite covers:

- pure logic (remote URL parsing, token precedence, `.gitignore` handling) with table tests
- the GitHub client against an `httptest` server, so query construction, pagination and error mapping are exercised over real HTTP
- the bubbletea models two ways — `Update` called directly for state, and full programs driven through a pseudo-terminal with `teatest`
- the commands end to end against fake GitHub and real temporary git repositories — `origin` keeps a GitHub URL, since that is what the slug is parsed from, but `url.<path>.insteadOf` rewrites it to a local bare repo at transport time, so fetches and pushes in the suite are real git operations that cannot leave the machine
- the built binary as a subprocess, for argument handling and exit codes
