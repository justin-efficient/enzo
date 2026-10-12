package ghclient

import (
	"context"
	"net/http"
	"testing"
)

func TestPriorities(t *testing.T) {
	var vars []map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		req.Variables = decodeBody(t, r)["variables"].(map[string]any)
		vars = append(vars, req.Variables)

		issues := map[string]any{}
		if req.Variables["after"] == nil {
			issues = map[string]any{
				"pageInfo": map[string]any{"hasNextPage": true, "endCursor": "c1"},
				"nodes": []any{
					// On two boards: the first board with a priority wins.
					map[string]any{"number": 1, "projectItems": map[string]any{"nodes": []any{
						map[string]any{"fieldValueByName": nil},
						map[string]any{"fieldValueByName": map[string]any{"name": "P0", "color": "RED"}},
						map[string]any{"fieldValueByName": map[string]any{"name": "P2", "color": "YELLOW"}},
					}}},
					// On no board at all.
					map[string]any{"number": 2, "projectItems": map[string]any{"nodes": []any{}}},
				},
			}
		} else {
			issues = map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": "c2"},
				"nodes": []any{
					map[string]any{"number": 3, "projectItems": map[string]any{"nodes": []any{
						map[string]any{"fieldValueByName": map[string]any{"name": "High", "color": "GREEN"}},
					}}},
				},
			}
		}
		writeJSON(t, w, map[string]any{"data": map[string]any{
			"repository": map[string]any{"issues": issues},
		}})
	}))

	got, err := api.Priorities(context.Background(), testSlug, "justin-efficient")
	if err != nil {
		t.Fatalf("Priorities: %v", err)
	}

	want := map[int]Priority{1: {"P0", "RED"}, 3: {"High", "GREEN"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for n, p := range want {
		if got[n] != p {
			t.Errorf("#%d = %v, want %v", n, got[n], p)
		}
	}
	if len(vars) != 2 || vars[1]["after"] != "c1" {
		t.Errorf("did not follow the page cursor: %v", vars)
	}
	if vars[0]["login"] != "justin-efficient" {
		t.Errorf("login = %v", vars[0]["login"])
	}
}

func TestPrioritiesError(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"errors": []map[string]any{{"message": "needs read:project"}}})
	}))
	if _, err := api.Priorities(context.Background(), testSlug, "x"); err == nil {
		t.Fatal("a GraphQL error should come back as an error")
	}
}
