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

func TestIssuePriorityField(t *testing.T) {
	var vars map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars = decodeBody(t, r)["variables"].(map[string]any)
		writeJSON(t, w, map[string]any{"data": map[string]any{"repository": map[string]any{"issue": map[string]any{
			"projectItems": map[string]any{"nodes": []any{
				// A board with no Priority field is skipped.
				map[string]any{"id": "item-a", "project": map[string]any{"id": "proj-a", "field": map[string]any{}}},
				map[string]any{"id": "item-b", "project": map[string]any{"id": "proj-b", "field": map[string]any{
					"id": "field-b",
					"options": []any{
						map[string]any{"id": "o0", "name": "P0", "color": "RED"},
						map[string]any{"id": "o2", "name": "P2", "color": "YELLOW"},
					},
				}}, "fieldValueByName": map[string]any{"optionId": "o2"}},
			}},
		}}}})
	}))

	got, err := api.IssuePriorityField(context.Background(), testSlug, 276, "")
	if err != nil {
		t.Fatalf("IssuePriorityField: %v", err)
	}
	if got == nil {
		t.Fatal("want the second board's field")
	}
	if got.ProjectID != "proj-b" || got.ItemID != "item-b" || got.FieldID != "field-b" || got.Current != "o2" {
		t.Errorf("got %+v", got)
	}
	if len(got.Options) != 2 || got.Options[0] != (PriorityOption{"o0", "P0", "RED"}) {
		t.Errorf("options = %+v", got.Options)
	}
	if vars["number"] != float64(276) {
		t.Errorf("number = %v", vars["number"])
	}
}

func TestIssuePriorityFieldNoneWhenOnNoBoard(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"data": map[string]any{"repository": map[string]any{"issue": map[string]any{
			"projectItems": map[string]any{"nodes": []any{}},
		}}}})
	}))
	got, err := api.IssuePriorityField(context.Background(), testSlug, 1, "")
	if err != nil || got != nil {
		t.Errorf("got %v, %v; want nil, nil", got, err)
	}
}

func TestSetPriority(t *testing.T) {
	var vars map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars = decodeBody(t, r)["variables"].(map[string]any)
		writeJSON(t, w, map[string]any{"data": map[string]any{}})
	}))
	f := PriorityField{ProjectID: "proj", ItemID: "item", FieldID: "field"}
	if err := api.SetPriority(context.Background(), f, "o0"); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}
	want := map[string]any{"project": "proj", "item": "item", "field": "field", "option": "o0"}
	for k, v := range want {
		if vars[k] != v {
			t.Errorf("%s = %v, want %v", k, vars[k], v)
		}
	}
}

func TestBoardPriorityField(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		board := func(id string, field any) map[string]any {
			return map[string]any{"project": map[string]any{"id": id, "field": field}}
		}
		writeJSON(t, w, map[string]any{"data": map[string]any{"repository": map[string]any{"issues": map[string]any{"nodes": []any{
			map[string]any{"projectItems": map[string]any{"nodes": []any{board("no-field", map[string]any{})}}},
			map[string]any{"projectItems": map[string]any{"nodes": []any{board("proj-b", map[string]any{
				"id": "field-b", "options": []any{map[string]any{"id": "o2", "name": "P2", "color": "YELLOW"}},
			})}}},
		}}}}})
	}))

	got, err := api.BoardPriorityField(context.Background(), testSlug, "me")
	if err != nil || got == nil {
		t.Fatalf("got %v, %v", got, err)
	}
	if got.ProjectID != "proj-b" || got.FieldID != "field-b" || got.ItemID != "" || len(got.Options) != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestBoardPriorityFieldNone(t *testing.T) {
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"data": map[string]any{"repository": map[string]any{"issues": map[string]any{"nodes": []any{}}}}})
	}))
	if got, err := api.BoardPriorityField(context.Background(), testSlug, "me"); err != nil || got != nil {
		t.Errorf("got %v, %v; want nil, nil", got, err)
	}
}

func TestAddToProject(t *testing.T) {
	var vars map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars = decodeBody(t, r)["variables"].(map[string]any)
		writeJSON(t, w, map[string]any{"data": map[string]any{"addProjectV2ItemById": map[string]any{"item": map[string]any{"id": "item-1"}}}})
	}))
	got, err := api.AddToProject(context.Background(), "proj", "node")
	if err != nil || got != "item-1" {
		t.Fatalf("got %q, %v", got, err)
	}
	if vars["project"] != "proj" || vars["content"] != "node" {
		t.Errorf("vars = %v", vars)
	}
}

// A board filter skips items on other boards.
func TestIssuePriorityFieldFiltersByProject(t *testing.T) {
	item := func(proj string) map[string]any {
		return map[string]any{"id": "item-" + proj, "project": map[string]any{"id": proj, "field": map[string]any{
			"id": "field-" + proj, "options": []any{map[string]any{"id": "o2", "name": "P2", "color": "YELLOW"}},
		}}}
	}
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"data": map[string]any{"repository": map[string]any{"issue": map[string]any{
			"projectItems": map[string]any{"nodes": []any{item("a"), item("b")}},
		}}}})
	}))

	if got, _ := api.IssuePriorityField(context.Background(), testSlug, 1, "b"); got == nil || got.ItemID != "item-b" {
		t.Errorf("filtered to b: got %+v", got)
	}
	if got, _ := api.IssuePriorityField(context.Background(), testSlug, 1, "a"); got == nil || got.ItemID != "item-a" {
		t.Errorf("filtered to a: got %+v", got)
	}
	if got, _ := api.IssuePriorityField(context.Background(), testSlug, 1, "c"); got != nil {
		t.Errorf("an issue not on board c should give nil, got %+v", got)
	}
}

func TestProjectPriorityField(t *testing.T) {
	var vars map[string]any
	reply := map[string]any{}
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars = decodeBody(t, r)["variables"].(map[string]any)
		writeJSON(t, w, map[string]any{"data": map[string]any{"repositoryOwner": map[string]any{"projectV2": reply}}})
	}))

	reply = map[string]any{"id": "proj", "closed": false, "field": map[string]any{
		"id": "field", "options": []any{map[string]any{"id": "o0", "name": "P0", "color": "RED"}},
	}}
	got, err := api.ProjectPriorityField(context.Background(), "EfficientComputer", 12)
	if err != nil || got == nil {
		t.Fatalf("got %v, %v", got, err)
	}
	if got.ProjectID != "proj" || got.FieldID != "field" || got.ItemID != "" || len(got.Options) != 1 {
		t.Errorf("got %+v", got)
	}
	if vars["owner"] != "EfficientComputer" || vars["number"] != float64(12) {
		t.Errorf("vars = %v", vars)
	}

	// Closed, or without the field: nothing to prioritize on.
	reply = map[string]any{"id": "proj", "closed": true, "field": reply["field"]}
	if got, _ := api.ProjectPriorityField(context.Background(), "o", 1); got != nil {
		t.Errorf("a closed board should give nil, got %+v", got)
	}
	reply = map[string]any{"id": "proj", "closed": false, "field": map[string]any{}}
	if got, _ := api.ProjectPriorityField(context.Background(), "o", 1); got != nil {
		t.Errorf("a board with no Priority field should give nil, got %+v", got)
	}
}

// Linked boards come first, each board once, and boards that are closed or
// have no Priority field are left out.
func TestCandidateBoards(t *testing.T) {
	board := func(id string, n int, title string, closed bool, field any) map[string]any {
		return map[string]any{"id": id, "number": n, "title": title, "closed": closed,
			"owner": map[string]any{"login": "Org"}, "field": field}
	}
	prio := map[string]any{"id": "f"}
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		onIssue := func(b map[string]any) any {
			return map[string]any{"projectItems": map[string]any{"nodes": []any{map[string]any{"project": b}}}}
		}
		writeJSON(t, w, map[string]any{"data": map[string]any{"repository": map[string]any{
			"projectsV2": map[string]any{"nodes": []any{
				board("linked", 11, "Linked", false, prio),
				board("dead", 3, "Closed", true, prio),
			}},
			"issues": map[string]any{"nodes": []any{
				onIssue(board("other", 12, "Other", false, prio)),
				onIssue(board("linked", 11, "Linked", false, prio)),
				onIssue(board("nofield", 4, "No field", false, map[string]any{})),
			}},
		}}})
	}))

	got, err := api.CandidateBoards(context.Background(), testSlug, "me")
	if err != nil {
		t.Fatalf("CandidateBoards: %v", err)
	}
	want := []Board{{"Org", 11, "Linked"}, {"Org", 12, "Other"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
