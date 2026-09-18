# 3. `enzo list` waits for GitHub rather than remembering what it created

- **Date:** 2026-09-18
- **Status:** decided — implemented
- **Supersedes:** [0002](0002-no-refetch-after-creating.md)
- **Affects:** `enzo list`, `internal/cli`, `internal/ui`

## Context

GitHub's issue listing lags several seconds behind a creation — measured at
4.0s and 4.4s in [0002](0002-no-refetch-after-creating.md), against under a
poll for edits to issues already in the list. So a list re-read immediately
after creating an issue reliably comes back without it.

Decision 0002 solved this by not re-reading at all: the issue returned by the
create call was spliced into the list enzo already had in memory.

That worked, but it made enzo hold issue state of its own. The list on screen
was then partly GitHub's answer and partly enzo's recollection, and the README
says plainly that **enzo is stateless**. A tool that remembers things has to be
right about when to forget them, and that is a class of bug worth not having.

## Decision

Re-read the list from GitHub after creating, as before, but do not show it
until it actually contains the new issue. A spinner runs while enzo polls the
listing; when the listing catches up, the list is drawn from that response.

What is on screen is therefore always exactly what GitHub reported. enzo
remembers nothing between renders.

The condition is stricter than "the issue is present". For a sub-issue the
listing must also carry the parent link, because nesting is derived from
`parent_issue_url`; without that check the issue would appear at the top level
for a moment before jumping under its parent on a later read.

Polling is every 400ms, giving up after 30s. The timeout has to clear the
measured lag by a wide margin — it exists so a condition that will never hold
cannot hang the tool, not to bound normal waiting. Esc stops the wait early.
Giving up, by timeout or by esc, shows the list anyway; only a failed API call
is an error.

## Consequence

Creating an issue from the list now takes a few seconds before the list comes
back, where the splice was instant. Measured end to end against the real API,
including the parent link: **8 polls, 5.3s**.

That is the honest cost of showing only what GitHub knows, and it is visible
rather than hidden — the spinner says what it is waiting for. If the wait ever
becomes annoying, the answer is to shorten the poll interval, not to start
remembering issues again.
