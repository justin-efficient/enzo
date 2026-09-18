package ghclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/justin-efficient/enzo/internal/gitrepo"
)

var testSlug = gitrepo.Slug{Owner: "justin-efficient", Name: "enzo"}

// newTestAPI points a real go-github client at h, so requests and responses
// travel the same code path they do against github.com.
func newTestAPI(t *testing.T, h http.Handler) *API {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	api, err := NewWithHTTPClient(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("NewWithHTTPClient: %v", err)
	}
	return api
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encoding response: %v", err)
	}
}

func TestViewer(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/v3/user"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		writeJSON(t, w, map[string]any{"login": "justin-efficient"})
	}))

	got, err := api.Viewer(context.Background())
	if err != nil {
		t.Fatalf("Viewer: %v", err)
	}
	if got != "justin-efficient" {
		t.Errorf("Viewer = %q, want %q", got, "justin-efficient")
	}
}

// A 200 with no login is malformed; enzo should not proceed with an empty user.
func TestViewerEmptyLogin(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"id": 1})
	}))
	if _, err := api.Viewer(context.Background()); err == nil {
		t.Fatal("Viewer with no login should fail")
	}
}

func TestViewerErrorMessages(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{"unauthorized points at setup", http.StatusUnauthorized, "enzo setup"},
		{"forbidden mentions permission", http.StatusForbidden, "permission"},
		{"not found says so", http.StatusNotFound, "not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				fmt.Fprint(w, `{"message":"boom"}`)
			}))
			_, err := api.Viewer(context.Background())
			if err == nil {
				t.Fatalf("status %d should produce an error", tt.status)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should mention %q", err, tt.want)
			}
		})
	}
}

func TestAssignedIssuesQuery(t *testing.T) {
	var got map[string]string
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "/api/v3/repos/justin-efficient/enzo/issues"; r.URL.Path != want {
			t.Errorf("path = %q, want %q", r.URL.Path, want)
		}
		got = map[string]string{}
		for k, v := range r.URL.Query() {
			got[k] = v[0]
		}
		writeJSON(t, w, []any{})
	}))

	if _, err := api.AssignedIssues(context.Background(), testSlug, "justin-efficient"); err != nil {
		t.Fatalf("AssignedIssues: %v", err)
	}

	want := map[string]string{
		"state":     "open",
		"assignee":  "justin-efficient",
		"sort":      "updated",
		"direction": "desc",
		"per_page":  "100",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("query %s = %q, want %q (full query: %v)", k, got[k], v, got)
		}
	}
}

func TestAssignedIssuesExcludesPullRequests(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, []map[string]any{
			{"number": 7, "title": "a real issue", "state": "open"},
			{"number": 8, "title": "a pull request", "state": "open",
				"pull_request": map[string]any{"url": "https://example.com/pulls/8"}},
			{"number": 9, "title": "another issue", "state": "open"},
		})
	}))

	issues, err := api.AssignedIssues(context.Background(), testSlug, "justin-efficient")
	if err != nil {
		t.Fatalf("AssignedIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2: %+v", len(issues), issues)
	}
	for _, iss := range issues {
		if iss.Number == 8 {
			t.Errorf("pull request #8 leaked into the issue list")
		}
	}
}

func TestAssignedIssuesFields(t *testing.T) {
	updated := time.Date(2026, 9, 1, 12, 30, 0, 0, time.UTC)
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, []map[string]any{{
			"number":     42,
			"title":      "make enzo pretty",
			"state":      "open",
			"html_url":   "https://github.com/justin-efficient/enzo/issues/42",
			"updated_at": updated.Format(time.RFC3339),
			"labels": []map[string]any{
				{"name": "ui"}, {"name": "good first issue"},
			},
		}})
	}))

	issues, err := api.AssignedIssues(context.Background(), testSlug, "justin-efficient")
	if err != nil {
		t.Fatalf("AssignedIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(issues))
	}
	got := issues[0]
	if got.Number != 42 {
		t.Errorf("Number = %d, want 42", got.Number)
	}
	if got.Title != "make enzo pretty" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.State != "open" {
		t.Errorf("State = %q, want open", got.State)
	}
	if got.URL != "https://github.com/justin-efficient/enzo/issues/42" {
		t.Errorf("URL = %q", got.URL)
	}
	if !got.UpdatedAt.Equal(updated) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, updated)
	}
	if strings.Join(got.Labels, ",") != "ui,good first issue" {
		t.Errorf("Labels = %v", got.Labels)
	}
}

// An issue with no labels must yield an empty slice, never a nil the view
// layer has to special-case.
func TestAssignedIssuesEmptyLabels(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, []map[string]any{{"number": 1, "title": "x"}})
	}))
	issues, err := api.AssignedIssues(context.Background(), testSlug, "justin-efficient")
	if err != nil {
		t.Fatal(err)
	}
	if issues[0].Labels == nil {
		t.Error("Labels should be an empty slice, not nil")
	}
	if len(issues[0].Labels) != 0 {
		t.Errorf("Labels = %v, want empty", issues[0].Labels)
	}
}

func TestAssignedIssuesPaginates(t *testing.T) {
	var pages []string
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		switch page {
		case "", "1":
			w.Header().Set("Link", `<`+"http://"+r.Host+`/api/v3/repos/justin-efficient/enzo/issues?page=2>; rel="next"`)
			writeJSON(t, w, []map[string]any{{"number": 1, "title": "one"}})
		case "2":
			w.Header().Set("Link", `<`+"http://"+r.Host+`/api/v3/repos/justin-efficient/enzo/issues?page=3>; rel="next"`)
			writeJSON(t, w, []map[string]any{{"number": 2, "title": "two"}})
		default:
			writeJSON(t, w, []map[string]any{{"number": 3, "title": "three"}})
		}
	}))

	issues, err := api.AssignedIssues(context.Background(), testSlug, "justin-efficient")
	if err != nil {
		t.Fatalf("AssignedIssues: %v", err)
	}
	if len(issues) != 3 {
		t.Fatalf("got %d issues across pages, want 3: %+v", len(issues), issues)
	}
	for i, want := range []int{1, 2, 3} {
		if issues[i].Number != want {
			t.Errorf("issue[%d].Number = %d, want %d", i, issues[i].Number, want)
		}
	}
	if len(pages) != 3 {
		t.Errorf("made %d requests (%v), want 3", len(pages), pages)
	}
}

func TestAssignedIssuesError(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	_, err := api.AssignedIssues(context.Background(), testSlug, "justin-efficient")
	if err == nil {
		t.Fatal("a 404 should produce an error")
	}
	if !strings.Contains(err.Error(), testSlug.String()) {
		t.Errorf("error %q should name the repo %s", err, testSlug)
	}
}

func TestContextCancellation(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"login": "x"})
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := api.Viewer(ctx); err == nil {
		t.Error("a canceled context should produce an error")
	}
}

func TestNewSetsAuthHeader(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		writeJSON(t, w, map[string]any{"login": "x"})
	}))
	defer srv.Close()

	api, err := New("ghp_secret", srv.URL+"/")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := api.Viewer(context.Background()); err != nil {
		t.Fatalf("Viewer: %v", err)
	}
	if auth != "Bearer ghp_secret" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer ghp_secret")
	}
}

func TestNewDefaultsToGitHubCom(t *testing.T) {
	if _, err := New("tok", ""); err != nil {
		t.Errorf("New with no host: %v", err)
	}
	if _, err := New("tok", "   "); err != nil {
		t.Errorf("New with blank host: %v", err)
	}
}

// API must satisfy Client so main can pass it where the interface is expected.
func TestAPIImplementsClient(t *testing.T) {
	var _ Client = (*API)(nil)
}
