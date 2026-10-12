package ghclient

import (
	"context"

	"github.com/justin-efficient/enzo/internal/gitrepo"
)

// Priority is the value of an issue's "Priority" field on a Projects v2 board.
// GitHub has no priority of its own: it lives on the board item, as a
// single-select option whose name and color are whatever the team chose. See
// docs/decisions/0001-no-priority-column.md.
type Priority struct {
	Name string
	// Color is GitHub's option color: RED, ORANGE, YELLOW, GREEN, BLUE, PURPLE,
	// PINK or GRAY.
	Color string
}

const prioritiesQuery = `
query($owner: String!, $name: String!, $login: String!, $after: String) {
  repository(owner: $owner, name: $name) {
    issues(first: 100, after: $after, states: OPEN, filterBy: {assignee: $login}) {
      pageInfo { hasNextPage endCursor }
      nodes {
        number
        projectItems(first: 10) {
          nodes {
            fieldValueByName(name: "Priority") {
              ... on ProjectV2ItemFieldSingleSelectValue { name color }
            }
          }
        }
      }
    }
  }
}`

// Priorities maps issue number to priority for the open issues assigned to
// login. Issues on no board, or with no priority set, are absent.
//
// An issue on several boards takes the first priority GitHub lists for it.
func (a *API) Priorities(ctx context.Context, slug gitrepo.Slug, login string) (map[int]Priority, error) {
	out := map[int]Priority{}
	after := (*string)(nil)

	for {
		var data struct {
			Repository struct {
				Issues struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						Number       int `json:"number"`
						ProjectItems struct {
							Nodes []struct {
								// Null when the board has no such field, or the
								// field is not a single-select.
								FieldValue *struct {
									Name  string `json:"name"`
									Color string `json:"color"`
								} `json:"fieldValueByName"`
							} `json:"nodes"`
						} `json:"projectItems"`
					} `json:"nodes"`
				} `json:"issues"`
			} `json:"repository"`
		}

		vars := map[string]any{"owner": slug.Owner, "name": slug.Name, "login": login, "after": after}
		if err := a.graphql(ctx, prioritiesQuery, vars, &data); err != nil {
			return nil, err
		}

		issues := data.Repository.Issues
		for _, iss := range issues.Nodes {
			for _, item := range iss.ProjectItems.Nodes {
				if v := item.FieldValue; v != nil && v.Name != "" {
					out[iss.Number] = Priority{Name: v.Name, Color: v.Color}
					break
				}
			}
		}

		if !issues.PageInfo.HasNextPage {
			return out, nil
		}
		cursor := issues.PageInfo.EndCursor
		after = &cursor
	}
}
