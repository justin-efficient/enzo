# 🚘 enzo

A tiny, opinionated CLI for GitHub that makes issue & PR management simpler for engineers.

1. enzo keeps PRs and issues linked.
2. enzo is stateless.
3. enzo is pretty.

## Commands

| command | status | what it does |
| --- | --- | --- |
| `enzo setup` | **done** | create the `.enzo` file with the token used to reach this repo |
| `enzo list` | **done** | list the open issues assigned to me in current repo — selecting one opens it in a browser, the top option is "new", `ctrl+n` opens a sub-issue of the highlighted one, esc cancels |
| `enzo new [sub] "title" | **done** | create an new issue, or sub issue of current and assign to me |
| `enzo start [issue-number] ["title"]` | **done** | calls `enzo new` if the issue doesn't exist. Then branch off `main`, push, and create a linked draft PR with no reviewers (if the PR doesn't exist). Then switch to that local branch.
| `enzo finish` | **done** | check the worktree is clean, take the PR out of draft, check it can merge — mergeable, review, its own checks, and the base branch's build — then merge it |
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

A title given on the command line skips the form entirely, so the body is left
to the footer unless `--body` says otherwise. A bare number straight after
`sub` is the parent issue; anything else there is the title. Quote the title —
enzo rejects an unquoted one.

Sub-issues use GitHub's native parent/child link, so they show up in the
parent's sub-issue list and progress bar.

Every issue and pull request enzo opens is signed: what you wrote, a horizontal
rule, then `Created by 🚘 enzo v0.5.0` in italics. An issue opened with no body
at all is still signed.

Picking "new" in `enzo list` runs the same flow and then returns to the list,
with the issue you just opened already in it. Backing out of the form returns
to the list too; esc on the list itself is what exits. After creating, enzo
shows a spinner until GitHub's listing has the new issue — and, for a
sub-issue, its parent link too. Esc stops waiting early and shows the list as
it stands.

Creating an issue from the picker prints nothing.

## Listing issues

```sh
enzo list            # picker when stdin and stdout are both terminals
enzo list --plain    # plain list, never the picker
```

Without a terminal on both ends, `enzo list` prints the plain list, so it
composes in pipes and scripts.

| key | in the picker |
| --- | --- |
| ↑ / ↓ | move |
| enter | show the highlighted issue in your browser, then come back to the list — on the "new issue" row, open the form |
| `ctrl+n` | open a sub-issue of the highlighted issue — a top-level issue on the "new issue" row |
| esc | cancel |

```
open issues assigned to you in justin-efficient/enzo

> + new issue
  #4 enzo finish should wait on required checks [bug]
  #1 implement enzo start
    #5 sub-issue created by enzo new

🚘 enzo v0.5.0 · ↑/↓ move · enter open in browser · ctrl+n new sub-issue · esc cancel
```

Sub-issues are nested under their parent, two spaces per level. An issue whose
parent is not in the list — not assigned to you, closed, or in another
repository — stays at the top level.

The list starts nothing. To work on an issue, run `enzo start <n>`.

The browser is `open` on macOS, `rundll32` on Windows and `xdg-open` elsewhere,
so `$BROWSER` and your desktop default are honoured. If none of them will run,
enzo prints the URL and the list carries on.

## Starting work

`enzo start` creates whichever of the issue, the branch and the draft pull
request does not exist yet, so running it twice on the same issue settles
rather than doing anything again.

```sh
enzo start 12                 # work on #12
enzo start "#12"              # the same
enzo start "fix the parser"   # open an issue with that title, then work on it
enzo start "fix it" --body "detail"
```

An issue number *and* a title is an error. So is neither.

What it does, in order:

1. **The issue** — fetched when you gave a number, opened and assigned to you
   when you gave a title.
2. **The branch** — `<your-login>/<number>-<slugified-title>`, for example
   `justin-efficient/12-fix-the-parser-crash`. The title part is cut at 48
   characters, on a word boundary.

   A new branch is cut from **origin's default branch**, freshly fetched — not
   from whatever you were standing on. An existing branch of that name is
   switched to as it is; enzo will not rebase it for you.

   Uncommitted work comes along, the same as a hand-typed `git switch`. When it
   cannot, git refuses and enzo stops there, leaving you where you were.
3. **A commit, if the branch has nothing the base does not** — an empty
   `Created by 🚘 enzo v0.5.0` commit, so there is something to open a pull
   request on. A branch you have really worked on gets nothing.
4. **The push** — `git push -u origin <branch>`.
5. **The draft PR** — titled after the issue, against the repository's default
   branch, with no reviewers, and bodied:

   ```markdown
   Closes #12

   ---

   *Created by 🚘 enzo v0.5.0*
   ```

   That closing keyword is what keeps the PR and the issue linked, and what
   lets `enzo finish` close the issue by merging.

Every GitHub read happens before the worktree is touched, so a call that was
going to fail leaves you on the branch you started on. Once enzo starts
changing things it stops at the first error and says what it got done.

Starting a closed issue is a warning on stderr, not a refusal.

## Finishing

`enzo finish` takes the pull request on the branch you are standing on out of
draft, checks everything between it and the base branch, and merges it. Like
`enzo abort` it takes no arguments — what it finishes is where you are.

```
$ enzo finish
🏁 finishing Issue #12 "fix the thing"
   ✅ changes:   none, the worktree is clean
   ✅ undrafted: PR #77
   ✅ mergeable: yes, clean
   ✅ review:    not required here
   ✅ checks:    all passed
   ✅ main:      passing at 0d752a6
   ✅ merged:    PR #77 into main
```

Each row opens with ✅ when that check is satisfied and ❌ when it is not. The
six rows are the six things it checks, in order. A run that refuses prints the
same six and then says why, so one run names everything that is wrong rather
than one thing per attempt:

```
$ enzo finish
🏁 finishing Issue #12 "fix the thing"
   ❌ changes:   2 files uncommitted: parser.go, parser_test.go
   ✅ undrafted: PR #77
   ✅ mergeable: yes, clean
   ❌ review:    required — waiting on someone, a-team
   ❌ checks:    failed: dist
   ✅ main:      passing at 0d752a6
enzo: not merged: 2 files uncommitted — commit or stash them; review is required and has not been given; checks failed
```

`changes` is anything uncommitted: staged, unstaged, or a file you never added.
An untracked file counts, and that is the case most worth catching — the pull
request merges without it. Up to three are named and the rest counted; `git
status` has the full list.

A ❌ on `main` is the one cross that does not stop the merge: the base branch
is **reported, not enforced**. A red `main` is worth seeing before you add to
it, but the merge goes ahead on the line below. A base that is still building
is not red. Everything else that fails does stop the merge.

**enzo requests no reviewers of its own.** It reports whoever GitHub asked when
the pull request left draft, and waits.

## Aborting

`enzo abort` throws away the attempt on the branch you are standing on: the
pull request is closed, the branch is deleted here and on origin. It takes no
arguments — what it destroys is where you are.

```
$ enzo abort
❌ aborting work in justin-efficient/enzo:
   justin-efficient/12-fix-the-thing : will be deleted forever
   PR #77 fix the parser crash : will be closed
   Issue #12 : will remain open, ready for a new pr
   ⚠ 2 commits not on origin — deleting the branch destroys them
   ⚠ 3 files with uncommitted changes, which move to main

type nukefromorbit to confirm: nukefromorbit

   switched to: main
   closed:      PR #77
   deleted:     origin/justin-efficient/12-fix-the-thing
   deleted:     justin-efficient/12-fix-the-thing
   left:        #12 open
```

The issue is left open — `enzo start 12` is the right next command if you want
the same issue on a clean branch.

Anything that will not survive is named *before* the phrase is asked for.
Typing anything other than `nukefromorbit` cancels and touches nothing. There
is no `--force`, but the phrase can be piped: `echo nukefromorbit | enzo
abort`.

enzo only aborts branches it named (`<login>/<number>-<title>`) and refuses
anything else, the default branch included. It switches you to the default
branch before deleting, and if uncommitted work cannot come with you, git
refuses and enzo stops having destroyed nothing.

## Versioning

enzo is versioned `MAJOR.MINOR.PATCH`, starting at **0.1.0**. The version lives
in `internal/version` and is the source of truth: `version.Banner()` formats
it, and `version.Credit()` — "Created by" plus the banner — signs what enzo
leaves behind. Nothing else hardcodes the string, and the suite fails if
anything does.

```sh
enzo --version     # 🚘 enzo v0.5.0
```

Cutting a release is a tag: `git tag -a v0.5.0 -m "0.5.0" && git push --tags`.
`make` passes `git describe` into the binary when the tree has a tag, so a
build from three commits past `v0.5.0` reports `0.5.0-3-gabc123` rather than
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
- `internal/browser` — opening a URL on macOS, Windows and everything else
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

### Why enzo is like this

- [docs/design.md](docs/design.md) — the reasoning behind the output shape, the
  log, signing, and each command's behaviour
- [docs/decisions/](docs/decisions/) — one file per decision that shaped enzo,
  especially where we chose not to build something
