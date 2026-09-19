package ghclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// The test API is built with the enterprise URLs, so its REST root ends in
// /api/v3/ and GraphQL must come out beside it rather than under it.
func TestGraphQLURLForEnterprise(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	got := api.graphqlURL()
	if strings.Contains(got, "/api/v3/") {
		t.Errorf("graphqlURL = %q, want GraphQL beside the REST root, not under it", got)
	}
	if !strings.HasSuffix(got, "/api/graphql") {
		t.Errorf("graphqlURL = %q, want it to end with /api/graphql", got)
	}
}

// github.com serves GraphQL from /graphql at the API root.
func TestGraphQLURLForDotCom(t *testing.T) {
	api, err := New("token", "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := api.graphqlURL(), "https://api.github.com/graphql"; got != want {
		t.Errorf("graphqlURL = %q, want %q", got, want)
	}
}

// A GraphQL failure arrives as HTTP 200 with an "errors" array, so a client
// that trusts the status code reports success on every failure.
func TestGraphQLErrorsInA200(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"data": nil,
			"errors": []map[string]any{
				{"message": "Could not resolve to a PullRequest"},
				{"message": "and another thing"},
			},
		})
	}))

	err := api.graphql(context.Background(), "query{}", nil, &struct{}{})
	if err == nil {
		t.Fatal("a 200 carrying errors must not read as success")
	}
	// Every message, not just the first: they are usually different problems.
	for _, want := range []string{"Could not resolve", "and another thing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

// The query and its variables go in the body as JSON, which is the only way
// GitHub accepts them.
func TestGraphQLSendsQueryAndVariables(t *testing.T) {
	var body map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("body is not JSON: %v", err)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		writeJSON(t, w, map[string]any{"data": map[string]any{"ok": true}})
	}))

	var out struct {
		OK bool `json:"ok"`
	}
	if err := api.graphql(context.Background(), "query($n:Int!){}", map[string]any{"n": 7}, &out); err != nil {
		t.Fatalf("graphql: %v", err)
	}
	if got := body["query"]; got != "query($n:Int!){}" {
		t.Errorf("query = %v", got)
	}
	vars, _ := body["variables"].(map[string]any)
	if vars["n"] != float64(7) {
		t.Errorf("variables = %v, want n=7", body["variables"])
	}
	if !out.OK {
		t.Error("data was not decoded into out")
	}
}

// Readiness asks its five questions in one round trip, and folds the answers
// into the shape the command reasons about.
func TestReadiness(t *testing.T) {
	calls := 0
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		writeJSON(t, w, map[string]any{"data": map[string]any{
			"repository": map[string]any{
				"pullRequest": map[string]any{
					"mergeable":        "MERGEABLE",
					"mergeStateStatus": "BLOCKED",
					"reviewDecision":   "REVIEW_REQUIRED",
					"reviewRequests": map[string]any{"nodes": []map[string]any{
						{"requestedReviewer": map[string]any{"login": "someone"}},
						{"requestedReviewer": map[string]any{"slug": "a-team"}},
					}},
					"commits": map[string]any{"nodes": []map[string]any{
						{"commit": map[string]any{"statusCheckRollup": map[string]any{
							"state": "FAILURE",
							"contexts": map[string]any{"nodes": []map[string]any{
								{"name": "test", "conclusion": "SUCCESS"},
								{"name": "dist", "conclusion": "FAILURE"},
								{"name": "slow", "conclusion": "", "status": "IN_PROGRESS"},
								{"context": "legacy/ci", "state": "SUCCESS"},
							}},
						}},
						},
					}},
				},
				"defaultBranchRef": map[string]any{
					"name": "main",
					"target": map[string]any{
						"oid":               "0d752a65ca05e566a28b3bf2ad624d1d4fc495cc",
						"statusCheckRollup": map[string]any{"state": "SUCCESS"},
					},
				},
			},
		}})
	}))

	got, err := api.Readiness(context.Background(), testSlug, 77)
	if err != nil {
		t.Fatalf("Readiness: %v", err)
	}
	if calls != 1 {
		t.Errorf("made %d requests, want 1 — the five questions are one decision", calls)
	}
	if got.Mergeable != "MERGEABLE" || got.MergeState != "BLOCKED" {
		t.Errorf("mergeable = %q/%q", got.Mergeable, got.MergeState)
	}
	if got.ReviewDecision != "REVIEW_REQUIRED" {
		t.Errorf("reviewDecision = %q", got.ReviewDecision)
	}
	// Users come back as logins and teams as slugs; both are reviewers.
	if strings.Join(got.Reviewers, ",") != "someone,a-team" {
		t.Errorf("reviewers = %v, want both the user and the team", got.Reviewers)
	}
	if strings.Join(got.Failed, ",") != "dist" {
		t.Errorf("failed = %v, want dist", got.Failed)
	}
	if strings.Join(got.Pending, ",") != "slow" {
		t.Errorf("pending = %v, want the one that has not concluded", got.Pending)
	}
	if got.BaseBranch != "main" || got.BaseChecks != "SUCCESS" {
		t.Errorf("base = %q/%q", got.BaseBranch, got.BaseChecks)
	}
	// Abbreviated, because the full forty characters say nothing more.
	if got.BaseHead != "0d752a6" {
		t.Errorf("BaseHead = %q, want the short form", got.BaseHead)
	}
}

// A commit with no checks is not a commit whose checks passed.
func TestReadinessWithNoChecks(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"data": map[string]any{
			"repository": map[string]any{
				"pullRequest": map[string]any{
					"mergeable": "MERGEABLE",
					"commits": map[string]any{"nodes": []map[string]any{
						{"commit": map[string]any{"statusCheckRollup": nil}},
					}},
				},
				"defaultBranchRef": map[string]any{
					"name":   "main",
					"target": map[string]any{"oid": "abcdef1234", "statusCheckRollup": nil},
				},
			},
		}})
	}))

	got, err := api.Readiness(context.Background(), testSlug, 77)
	if err != nil {
		t.Fatalf("Readiness: %v", err)
	}
	if got.Checks != "" || len(got.Failed) != 0 || len(got.Pending) != 0 {
		t.Errorf("checks = %q %v %v, want all empty", got.Checks, got.Failed, got.Pending)
	}
	if got.BaseChecks != "" {
		t.Errorf("BaseChecks = %q, want empty", got.BaseChecks)
	}
}

// REST answers 200 and leaves the pull request a draft, so this mutation
// checks what it actually did rather than that it was accepted.
func TestMarkReadyForReviewVerifiesTheResult(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"data": map[string]any{
			"markPullRequestReadyForReview": map[string]any{
				"pullRequest": map[string]any{"number": 77, "isDraft": true},
			},
		}})
	}))

	err := api.MarkReadyForReview(context.Background(), "PR_node77")
	if err == nil {
		t.Fatal("a mutation that left the PR a draft must not report success")
	}
	if !strings.Contains(err.Error(), "still a draft") {
		t.Errorf("error = %q, want it to say the draft survived", err)
	}
}

func TestMarkReadyForReviewSucceeds(t *testing.T) {
	var sent map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &sent)
		writeJSON(t, w, map[string]any{"data": map[string]any{
			"markPullRequestReadyForReview": map[string]any{
				"pullRequest": map[string]any{"number": 77, "isDraft": false},
			},
		}})
	}))

	if err := api.MarkReadyForReview(context.Background(), "PR_node77"); err != nil {
		t.Fatalf("MarkReadyForReview: %v", err)
	}
	vars, _ := sent["variables"].(map[string]any)
	if vars["id"] != "PR_node77" {
		t.Errorf("sent id %v, want the node id", vars["id"])
	}
}
