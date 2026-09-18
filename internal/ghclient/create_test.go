package ghclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// decodeBody reads the JSON a request carried.
func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body %s is not JSON: %v", b, err)
	}
	return m
}

func TestCreateIssue(t *testing.T) {
	var method, path string
	var body map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		body = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, map[string]any{
			"id": 42424242, "number": 57, "title": "a new thing",
			"html_url": "https://github.com/justin-efficient/enzo/issues/57",
		})
	}))

	got, err := api.CreateIssue(context.Background(), testSlug, NewIssue{
		Title: "a new thing", Body: "some detail", Assignee: "justin-efficient",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	if want := "/api/v3/repos/justin-efficient/enzo/issues"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if body["title"] != "a new thing" {
		t.Errorf("title = %v", body["title"])
	}
	if body["body"] != "some detail" {
		t.Errorf("body = %v", body["body"])
	}
	// The issue must come back assigned, or `enzo list` will not show it.
	assignees, _ := body["assignees"].([]any)
	if len(assignees) != 1 || assignees[0] != "justin-efficient" {
		t.Errorf("assignees = %v, want [justin-efficient]", body["assignees"])
	}

	// The id is what sub-issue linking needs; losing it breaks `new sub`.
	if got.ID != 42424242 {
		t.Errorf("ID = %d, want 42424242", got.ID)
	}
	if got.Number != 57 {
		t.Errorf("Number = %d, want 57", got.Number)
	}
	if got.URL == "" {
		t.Error("URL should come back so the CLI can print it")
	}
}

// An empty body must be omitted rather than sent as "".
func TestCreateIssueOmitsEmptyFields(t *testing.T) {
	var body map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = decodeBody(t, r)
		writeJSON(t, w, map[string]any{"id": 1, "number": 1, "title": "t"})
	}))

	if _, err := api.CreateIssue(context.Background(), testSlug, NewIssue{Title: "t"}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if _, ok := body["body"]; ok {
		t.Errorf("an empty body should be omitted, got %v", body["body"])
	}
	if _, ok := body["assignees"]; ok {
		t.Errorf("an empty assignee should be omitted, got %v", body["assignees"])
	}
}

func TestCreateIssueError(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Forbidden"}`)
	}))
	_, err := api.CreateIssue(context.Background(), testSlug, NewIssue{Title: "t"})
	if err == nil {
		t.Fatal("a 403 should produce an error")
	}
	if !strings.Contains(err.Error(), testSlug.String()) {
		t.Errorf("error %q should name the repo", err)
	}
}

func TestLinkSubIssue(t *testing.T) {
	var method, path string
	var body map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		body = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, map[string]any{"id": 42424242, "number": 57})
	}))

	if err := api.LinkSubIssue(context.Background(), testSlug, 12, 42424242); err != nil {
		t.Fatalf("LinkSubIssue: %v", err)
	}

	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	// The parent goes in the path as a number; the child goes in the body as
	// a database id. Swapping them silently links the wrong issues.
	if want := "/api/v3/repos/justin-efficient/enzo/issues/12/sub_issues"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if got, ok := body["sub_issue_id"].(float64); !ok || int64(got) != 42424242 {
		t.Errorf("sub_issue_id = %v, want 42424242", body["sub_issue_id"])
	}
}

func TestLinkSubIssueError(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message":"Validation Failed"}`)
	}))
	err := api.LinkSubIssue(context.Background(), testSlug, 12, 99)
	if err == nil {
		t.Fatal("a 422 should produce an error")
	}
	if !strings.Contains(err.Error(), "#12") {
		t.Errorf("error %q should name the parent", err)
	}
}

func TestIssue(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "/api/v3/repos/justin-efficient/enzo/issues/12"; r.URL.Path != want {
			t.Errorf("path = %q, want %q", r.URL.Path, want)
		}
		writeJSON(t, w, map[string]any{"id": 777, "number": 12, "title": "parent"})
	}))

	got, err := api.Issue(context.Background(), testSlug, 12)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if got.ID != 777 || got.Number != 12 || got.Title != "parent" {
		t.Errorf("Issue = %+v", got)
	}
}

// A PR number must not be accepted as a parent issue.
func TestIssueRejectsPullRequests(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"id": 1, "number": 12, "title": "a PR",
			"pull_request": map[string]any{"url": "https://example.com/pulls/12"},
		})
	}))
	_, err := api.Issue(context.Background(), testSlug, 12)
	if err == nil {
		t.Fatal("a pull request should be rejected as an issue")
	}
	if !strings.Contains(err.Error(), "pull request") {
		t.Errorf("error %q should say it is a pull request", err)
	}
}

func TestIssueNotFound(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	if _, err := api.Issue(context.Background(), testSlug, 404); err == nil {
		t.Fatal("a missing issue should produce an error")
	}
}

func TestOpenIssuesIsNotFilteredByAssignee(t *testing.T) {
	var query map[string]string
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = map[string]string{}
		for k, v := range r.URL.Query() {
			query[k] = v[0]
		}
		writeJSON(t, w, []map[string]any{
			{"id": 1, "number": 1, "title": "mine"},
			{"id": 2, "number": 2, "title": "a PR", "pull_request": map[string]any{"url": "x"}},
			{"id": 3, "number": 3, "title": "someone else's"},
		})
	}))

	issues, err := api.OpenIssues(context.Background(), testSlug)
	if err != nil {
		t.Fatalf("OpenIssues: %v", err)
	}
	// Any open issue can be a parent, so no assignee filter.
	if _, ok := query["assignee"]; ok {
		t.Errorf("OpenIssues should not filter by assignee, sent %q", query["assignee"])
	}
	if query["state"] != "open" {
		t.Errorf("state = %q, want open", query["state"])
	}
	if len(issues) != 2 {
		t.Errorf("got %d issues, want 2 with the PR excluded: %+v", len(issues), issues)
	}
}

// Listing must carry ids through, since sub-issue linking needs them.
func TestListedIssuesCarryIDs(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, []map[string]any{{"id": 555, "number": 1, "title": "x"}})
	}))
	issues, err := api.AssignedIssues(context.Background(), testSlug, "justin-efficient")
	if err != nil {
		t.Fatal(err)
	}
	if issues[0].ID != 555 {
		t.Errorf("ID = %d, want 555", issues[0].ID)
	}
}

// The list payload carries parent_issue_url, so nesting costs no extra calls.
func TestAssignedIssuesCarryParentReference(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, []map[string]any{
			{"id": 500, "number": 5, "title": "a child",
				"parent_issue_url": "https://api.github.com/repos/justin-efficient/enzo/issues/1"},
			{"id": 100, "number": 1, "title": "a parent"},
		})
	}))

	issues, err := api.AssignedIssues(context.Background(), testSlug, "justin-efficient")
	if err != nil {
		t.Fatalf("AssignedIssues: %v", err)
	}
	if got := issues[0]; got.ParentNumber != 1 || got.ParentRepo != "justin-efficient/enzo" {
		t.Errorf("child parent = %s#%d, want justin-efficient/enzo#1", got.ParentRepo, got.ParentNumber)
	}
	if !issues[0].HasParent() {
		t.Error("the child should report HasParent")
	}
	if issues[1].HasParent() {
		t.Errorf("the parent should have no parent of its own, got %s#%d", issues[1].ParentRepo, issues[1].ParentNumber)
	}
}
