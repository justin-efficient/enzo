package ghclient

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestDefaultBranch(t *testing.T) {
	var path string
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		writeJSON(t, w, map[string]any{"name": "enzo", "default_branch": "trunk"})
	}))

	got, err := api.DefaultBranch(context.Background(), testSlug)
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if want := "/api/v3/repos/justin-efficient/enzo"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if got != "trunk" {
		t.Errorf("DefaultBranch = %q, want trunk", got)
	}
}

// A repo with no default branch would send enzo to open a PR against "", which
// is a worse error than saying so here.
func TestDefaultBranchMissing(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"name": "enzo"})
	}))

	if _, err := api.DefaultBranch(context.Background(), testSlug); err == nil {
		t.Fatal("expected an error when there is no default branch")
	}
}

func TestPullRequestForBranch(t *testing.T) {
	var query string
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		writeJSON(t, w, []any{map[string]any{
			"number": 77, "title": "fix the thing", "state": "open", "draft": true,
			"html_url": "https://github.com/justin-efficient/enzo/pull/77",
			"head":     map[string]any{"ref": "justin-efficient/12-fix-the-thing"},
			"base":     map[string]any{"ref": "main"},
		}})
	}))

	got, err := api.PullRequestForBranch(context.Background(), testSlug, "justin-efficient/12-fix-the-thing")
	if err != nil {
		t.Fatalf("PullRequestForBranch: %v", err)
	}
	if got == nil {
		t.Fatal("want the pull request, got nil")
	}
	// The head filter is what keeps this to one request instead of a scan.
	for _, want := range []string{"head=justin-efficient%3Ajustin-efficient%2F12-fix-the-thing", "state=open"} {
		if !strings.Contains(query, want) {
			t.Errorf("query = %q, want it to contain %q", query, want)
		}
	}
	if got.Number != 77 || !got.Draft || got.Base != "main" {
		t.Errorf("PullRequestForBranch = %+v", *got)
	}
}

func TestPullRequestForBranchNone(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, []any{})
	}))

	got, err := api.PullRequestForBranch(context.Background(), testSlug, "nope")
	if err != nil {
		t.Fatalf("PullRequestForBranch: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for a branch with no PR, got %+v", *got)
	}
}

// GitHub's head filter is advisory across forks, so a mismatched ref is not
// this branch's pull request.
func TestPullRequestForBranchIgnoresAnotherRef(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, []any{map[string]any{
			"number": 5,
			"head":   map[string]any{"ref": "someone-else/9-other"},
		}})
	}))

	got, err := api.PullRequestForBranch(context.Background(), testSlug, "justin-efficient/12-fix-the-thing")
	if err != nil {
		t.Fatalf("PullRequestForBranch: %v", err)
	}
	if got != nil {
		t.Errorf("want nil, got PR #%d on %q", got.Number, got.Head)
	}
}

func TestCreatePullRequest(t *testing.T) {
	var method, path string
	var body map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		body = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, map[string]any{
			"number": 300, "title": "fix the thing", "state": "open", "draft": true,
			"html_url": "https://github.com/justin-efficient/enzo/pull/300",
			"head":     map[string]any{"ref": "justin-efficient/12-fix-the-thing"},
			"base":     map[string]any{"ref": "main"},
		})
	}))

	got, err := api.CreatePullRequest(context.Background(), testSlug, NewPullRequest{
		Title: "fix the thing",
		Body:  "Closes #12",
		Head:  "justin-efficient/12-fix-the-thing",
		Base:  "main",
		Draft: true,
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}

	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	if want := "/api/v3/repos/justin-efficient/enzo/pulls"; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if body["draft"] != true {
		t.Errorf("draft = %v, want true — enzo never opens a PR ready for review", body["draft"])
	}
	if body["head"] != "justin-efficient/12-fix-the-thing" || body["base"] != "main" {
		t.Errorf("head/base = %v/%v", body["head"], body["base"])
	}
	if body["body"] != "Closes #12" {
		t.Errorf("body = %v, want the closing keyword", body["body"])
	}
	if got.Number != 300 || got.URL == "" {
		t.Errorf("CreatePullRequest = %+v", got)
	}
}

// A token without push rights is the common failure here; it should read as
// something to fix rather than a raw 403.
func TestCreatePullRequestErrorIsReadable(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		writeJSON(t, w, map[string]any{"message": "Forbidden"})
	}))

	_, err := api.CreatePullRequest(context.Background(), testSlug, NewPullRequest{Head: "b", Base: "main"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("error = %q, want it to mention permission", err)
	}
}
