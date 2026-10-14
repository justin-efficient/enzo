package ghclient

import (
	"context"
	"time"
)

// IterationOption is one upcoming or current iteration of a board's Iteration
// field.
type IterationOption struct {
	ID    string
	Title string
	// Start is the first day; Days is how many it lasts.
	Start time.Time
	Days  int
}

// End is the last day of the iteration.
func (o IterationOption) End() time.Time { return o.Start.AddDate(0, 0, o.Days-1) }

// IterationField is a board's Iteration field and one item's value for it.
// Finished iterations are not offered: GitHub lists them separately.
type IterationField struct {
	FieldID string
	Options []IterationOption
	// Current is the ID of the iteration set now, or empty when none is. It may
	// name a finished iteration, which is not among Options.
	Current string
}

const iterationQuery = `
query($project: ID!, $item: ID!, $withItem: Boolean!) {
  project: node(id: $project) {
    ... on ProjectV2 {
      field(name: "Iteration") {
        ... on ProjectV2IterationField {
          id
          configuration { iterations { id title startDate duration } }
        }
      }
    }
  }
  item: node(id: $item) @include(if: $withItem) {
    ... on ProjectV2Item {
      fieldValueByName(name: "Iteration") {
        ... on ProjectV2ItemFieldIterationValue { iterationId }
      }
    }
  }
}`

// BoardIterationField reads a board's Iteration field. itemID may be empty for
// an issue not on the board yet, which leaves Current empty. It returns nil
// when the board has no Iteration field or no iterations to offer.
func (a *API) BoardIterationField(ctx context.Context, projectID, itemID string) (*IterationField, error) {
	var data struct {
		Project struct {
			Field *struct {
				ID            string `json:"id"`
				Configuration struct {
					Iterations []struct {
						ID       string `json:"id"`
						Title    string `json:"title"`
						Start    string `json:"startDate"`
						Duration int    `json:"duration"`
					} `json:"iterations"`
				} `json:"configuration"`
			} `json:"field"`
		} `json:"project"`
		Item *struct {
			Value *struct {
				IterationID string `json:"iterationId"`
			} `json:"fieldValueByName"`
		} `json:"item"`
	}
	vars := map[string]any{"project": projectID, "item": itemID, "withItem": itemID != ""}
	if err := a.graphql(ctx, iterationQuery, vars, &data); err != nil {
		return nil, err
	}

	f := data.Project.Field
	// A board with no Iteration field (or one of another type) has no id.
	if f == nil || f.ID == "" {
		return nil, nil
	}
	out := &IterationField{FieldID: f.ID}
	for _, it := range f.Configuration.Iterations {
		start, err := time.Parse(time.DateOnly, it.Start)
		if err != nil {
			continue
		}
		out.Options = append(out.Options, IterationOption{ID: it.ID, Title: it.Title, Start: start, Days: it.Duration})
	}
	if len(out.Options) == 0 {
		return nil, nil
	}
	if data.Item != nil && data.Item.Value != nil {
		out.Current = data.Item.Value.IterationID
	}
	return out, nil
}

const setIterationMutation = `
mutation($project: ID!, $item: ID!, $field: ID!, $iteration: String!) {
  updateProjectV2ItemFieldValue(input: {
    projectId: $project, itemId: $item, fieldId: $field,
    value: {iterationId: $iteration}
  }) { projectV2Item { id } }
}`

// SetIteration moves the board item into an iteration. Like SetPriority it
// needs the `project` token scope.
func (a *API) SetIteration(ctx context.Context, projectID, itemID string, f IterationField, iterationID string) error {
	vars := map[string]any{"project": projectID, "item": itemID, "field": f.FieldID, "iteration": iterationID}
	return a.graphql(ctx, setIterationMutation, vars, nil)
}
