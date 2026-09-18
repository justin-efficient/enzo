# 2. `enzo list` does not re-read the list after creating an issue

- **Date:** 2026-09-18
- **Status:** **superseded by [0003](0003-wait-for-github-to-catch-up.md)** — the
  splice it describes held issue state inside enzo, which the project does not
  want. The measurements below still stand and motivate 0003.
- **Affects:** `enzo list`, `internal/cli`

## Context

`enzo list` runs the picker in a loop so that creating an issue returns to the
list rather than exiting. The first implementation re-read the issues from
GitHub at the top of every iteration, on the reasoning that a fresh read is the
most accurate thing to show.

A sub-issue created from the list did not appear in the list that came back.
Leaving `enzo` and running `enzo list` again showed it.

## What we found

GitHub's issue **listing** endpoint lags several seconds behind a creation.
Measured against `GET /repos/{owner}/{repo}/issues?state=open&assignee=…`,
polling roughly three times a second:

| Operation | Time until the listing reflects it |
| --- | --- |
| Creating an issue (sample 1) | 4.4s |
| Creating an issue (sample 2) | 4.0s |
| Editing an already-listed issue (3 samples) | under one poll, ~0.5s |

So the lag is specific to a row *entering* the listing, not to changes on rows
already in it. enzo re-read within milliseconds of creating, which is well
inside that window, so the refreshed list was reliably missing the new issue.

This was never a race in enzo: the command is single threaded and the re-read
happens strictly after the create returns. It is read-after-write lag on
GitHub's side, and no amount of ordering on our part fixes it.

## Decision

The list is read once, before the loop. An issue created during the loop is
added to the in-memory list directly, from the object the create call already
returned.

Waiting or retrying was rejected: it would put a multi-second stall between
pressing create and seeing the list, to obtain information enzo already has.

Two details make the spliced issue indistinguishable from a fetched one:

- it goes to the front, matching the listing's newest-first ordering;
- for a sub-issue, `ParentRepo` and `ParentNumber` are filled in after the link
  succeeds, since the creation response predates the link and would otherwise
  render the issue unnested.

## Consequence

A long-lived `enzo list` session does not pick up changes made elsewhere —
issues assigned to you by someone else while the picker is open, for example.
Esc and re-run to get a fresh read. This is an acceptable trade for a tool that
is meant to be opened, used and closed; if it becomes a problem, the fix is to
re-read *and* merge in locally created issues, rather than to re-read alone.
