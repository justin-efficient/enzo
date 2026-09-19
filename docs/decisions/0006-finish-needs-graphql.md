# 6. `enzo finish` uses GraphQL, because REST cannot undraft a pull request

- **Date:** 2026-09-19
- **Status:** decided — implemented
- **Affects:** `enzo finish`, `internal/ghclient`

## Context

[Decision 0001](0001-no-priority-column.md) turned down a feature partly to
avoid reaching for GraphQL: enzo was a REST client, and staying one was worth
something. `enzo finish` ends that, not as a preference but because its first
step has no REST route at all.

## What we found

**REST cannot take a pull request out of draft, and does not say so.**
`PATCH /repos/{owner}/{repo}/pulls/{n}` with `"draft": false` answers **HTTP
200** and returns the pull request with `"draft": true` still set. The field is
ignored. There is no error, no warning, and no other endpoint.

Measured against `justin-efficient/enzo_test#6` on 2026-09-19:

| route | result |
| --- | --- |
| `PATCH /pulls/6` with `draft=false` | 200 OK, `"draft": true` — unchanged |
| GraphQL `markPullRequestReadyForReview` | `isDraft: false` |
| GraphQL `convertPullRequestToDraft` | `isDraft: true` — reversible |

The silent success is the dangerous part. A REST implementation would print
"undrafted" and have done nothing, and the failure would only surface later as
a merge that GitHub refuses for no visible reason.

**Asking whether review is required is the same story, for a different
reason.** Over REST it means reading branch protection
(`GET /branches/{branch}/protection`), which **requires admin rights** — which
the author of a pull request usually does not have on the repository they are
contributing to. GraphQL's `reviewDecision` answers the same question with
ordinary read access, and answers it better: `REVIEW_REQUIRED`, `APPROVED` or
`CHANGES_REQUESTED`, rather than a rule to interpret.

## Decision

`internal/ghclient` keeps go-github and REST for everything it already did, and
adds a small hand-rolled GraphQL client — `graphql.go`, about eighty lines, no
new dependency — for the three things REST does badly or not at all:

| operation | why not REST |
| --- | --- |
| take a pull request out of draft | REST ignores the field and returns 200 |
| is review required, and satisfied? | REST needs admin to read branch protection |
| the five readiness questions | REST needs four calls; GraphQL needs one |

Merging stays on REST: `PUT /pulls/{n}/merge` works, and go-github already
wraps it.

`MarkReadyForReview` **asserts the result** rather than trusting the response,
because the route it replaces fails by reporting success.

## The one query

Steps 2 through 5 of `enzo finish` are one decision, so they are one round
trip: `mergeable`, `mergeStateStatus`, `reviewDecision`, `reviewRequests`, the
head commit's `statusCheckRollup`, and the default branch's `statusCheckRollup`
all come back together. Over REST that is four calls, one of which may be
forbidden.

## What is not a blocker

The base branch's build is **reported and not enforced**. A red `main` is worth
knowing about before you add to it, but it is not yours to fix and it does not
make your work unmergeable. Every other check that fails stops the merge.

Likewise, `enzo finish` **requests no reviewers of its own**. GitHub already
requests CODEOWNERS when a pull request leaves draft; enzo reports who was
asked and waits. Nobody is notified by a decision enzo made.

## Hosting

GraphQL lives at `/graphql` under the API root on github.com, and at
`/api/graphql` on GitHub Enterprise — a *sibling* of the `/api/v3/` REST root,
not a child of it. `graphqlURL` derives one from the other so the host is
configured in exactly one place, and a test covers both shapes.
