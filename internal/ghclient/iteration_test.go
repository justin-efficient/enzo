package ghclient

import (
	"context"
	"net/http"
	"testing"
)

func iterationResponse(field any, item any) map[string]any {
	return map[string]any{"data": map[string]any{
		"project": map[string]any{"field": field},
		"item":    item,
	}}
}

func TestBoardIterationField(t *testing.T) {
	var vars map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars = decodeBody(t, r)["variables"].(map[string]any)
		writeJSON(t, w, iterationResponse(
			map[string]any{"id": "IF", "configuration": map[string]any{"iterations": []any{
				map[string]any{"id": "i1", "title": "Iteration 64", "startDate": "2026-10-19", "duration": 21},
				map[string]any{"id": "i2", "title": "Iteration 65", "startDate": "2026-11-09", "duration": 21},
			}}},
			map[string]any{"fieldValueByName": map[string]any{"iterationId": "i2"}},
		))
	}))

	got, err := api.BoardIterationField(context.Background(), "proj", "item")
	if err != nil {
		t.Fatalf("BoardIterationField: %v", err)
	}
	if got == nil || got.FieldID != "IF" || got.Current != "i2" || len(got.Options) != 2 {
		t.Fatalf("got %+v", got)
	}
	o := got.Options[0]
	if o.ID != "i1" || o.Title != "Iteration 64" || o.Start.Format("2006-01-02") != "2026-10-19" || o.End().Format("2006-01-02") != "2026-11-08" {
		t.Errorf("option = %+v, end %v", o, o.End())
	}
	if vars["project"] != "proj" || vars["item"] != "item" || vars["withItem"] != true {
		t.Errorf("vars = %v", vars)
	}
}

// An issue not on the board yet has no item to read a value from.
func TestBoardIterationFieldWithoutItem(t *testing.T) {
	var vars map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars = decodeBody(t, r)["variables"].(map[string]any)
		writeJSON(t, w, iterationResponse(
			map[string]any{"id": "IF", "configuration": map[string]any{"iterations": []any{
				map[string]any{"id": "i1", "title": "Iteration 64", "startDate": "2026-10-19", "duration": 21},
			}}}, nil))
	}))
	got, err := api.BoardIterationField(context.Background(), "proj", "")
	if err != nil || got == nil || got.Current != "" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if vars["withItem"] != false {
		t.Errorf("withItem = %v, want false", vars["withItem"])
	}
}

func TestBoardIterationFieldNone(t *testing.T) {
	for name, field := range map[string]any{
		"no field":      map[string]any{},
		"no iterations": map[string]any{"id": "IF", "configuration": map[string]any{"iterations": []any{}}},
	} {
		t.Run(name, func(t *testing.T) {
			api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, iterationResponse(field, nil))
			}))
			got, err := api.BoardIterationField(context.Background(), "proj", "")
			if err != nil || got != nil {
				t.Errorf("got %v, %v; want nil, nil", got, err)
			}
		})
	}
}

func TestSetIteration(t *testing.T) {
	var vars map[string]any
	api := newTestAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars = decodeBody(t, r)["variables"].(map[string]any)
		writeJSON(t, w, map[string]any{"data": map[string]any{}})
	}))
	if err := api.SetIteration(context.Background(), "proj", "item", IterationField{FieldID: "IF"}, "i2"); err != nil {
		t.Fatalf("SetIteration: %v", err)
	}
	want := map[string]any{"project": "proj", "item": "item", "field": "IF", "iteration": "i2"}
	for k, v := range want {
		if vars[k] != v {
			t.Errorf("%s = %v, want %v", k, vars[k], v)
		}
	}
}
