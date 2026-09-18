# enzo

A tiny simple CLI for GitHub that makes issue lifecycle management easier.

enzo keeps PRs and issues linked.
enzo is stateless.
enzo is pretty.

## Commands

| command | status | what it does |
| --- | --- | --- |
| `enzo setup` | **done** | create the `.enzo` file with the token used to reach this repo |
| `enzo list` | **done** | list the open issues assigned to me in current repo — selecting one grabs it, the top option is "new", `ctrl+n` opens a sub-issue of the highlighted one, esc cancels |
| `enzo new [sub] [issue-number]` | **done** | create an new issue, or sub issue of current and assign to me |
| `enzo start [sub] [issue-number]` | planned | create an issue in the current repo assigned to you, with a linked draft PR and no reviewers. switch to that local branch. `sub` offers existing issues as parents |
| `enzo grab <issue-number>` | planned | switch to the branch linked to an issue, or create a branch and draft PR and switch to it |
| `enzo review` | planned | take the current branch's PR out of draft and attach the repo's default reviewers |
| `enzo finish` | planned | merge the current branch once the PR is ready — waits on, skips, or cancels against pending builds |

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
empty unless `--body` says otherwise. A bare number straight after `sub` is the
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

## Log

enzo appends a line for each thing it creates:

```
2026-09-18T01:41:53Z justin-efficient/enzo created #11 bork2 https://github.com/justin-efficient/enzo/issues/11
2026-09-18T01:44:02Z justin-efficient/enzo linked #12 under #1 https://github.com/justin-efficient/enzo/issues/12
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

Sub-issues are nested under their parent, two spaces per level:

```
#4 enzo finish should wait on required checks
#1 implement enzo grab
  #5 sub-issue created by enzo new
```

Nesting comes from the listing itself, so it costs no extra API calls. An issue
whose parent is not in the list — not assigned to you, closed, or in another
repository — stays at the top level rather than disappearing.

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
- `internal/gitrepo` — repo root, origin remote parsing, branch
- `internal/ghclient` — the GitHub surface enzo uses, behind a `Client` interface
- `internal/ui` — the bubbletea models (issue picker, token prompt)
- `internal/cli` — command dispatch; everything external arrives through `cli.Env`

### Testing

Commands take their dependencies through `cli.Env`, so they run without a
terminal or a network. The suite covers:

- pure logic (remote URL parsing, token precedence, `.gitignore` handling) with table tests
- the GitHub client against an `httptest` server, so query construction, pagination and error mapping are exercised over real HTTP
- the bubbletea models two ways — `Update` called directly for state, and full programs driven through a pseudo-terminal with `teatest`
- the commands end to end against fake GitHub and real temporary git repositories
- the built binary as a subprocess, for argument handling and exit codes
