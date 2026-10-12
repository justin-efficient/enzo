package ghclient

import (
	"context"
	"fmt"

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

// PriorityOption is one choice of a board's Priority field.
type PriorityOption struct {
	ID    string
	Name  string
	Color string
}

// PriorityField is everything needed to read and change one issue's priority
// on one board: which board item it is, which field, and the field's choices.
// The choices belong to the board, so they differ between boards.
type PriorityField struct {
	ProjectID string
	ItemID    string
	FieldID   string
	Options   []PriorityOption
	// Current is the ID of the option set now, or empty when none is.
	Current string
}

const priorityFieldQuery = `
query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      projectItems(first: 10) {
        nodes {
          id
          project {
            id
            field(name: "Priority") {
              ... on ProjectV2SingleSelectField { id options { id name color } }
            }
          }
          fieldValueByName(name: "Priority") {
            ... on ProjectV2ItemFieldSingleSelectValue { optionId }
          }
        }
      }
    }
  }
}`

// IssuePriorityField finds the board the issue is on that has a Priority
// field, and returns nil when there is none. An issue on several such boards
// takes the first, matching Priorities. A non-empty projectID restricts the
// search to that board.
func (a *API) IssuePriorityField(ctx context.Context, slug gitrepo.Slug, number int, projectID string) (*PriorityField, error) {
	var data struct {
		Repository struct {
			Issue struct {
				ProjectItems struct {
					Nodes []struct {
						ID      string `json:"id"`
						Project struct {
							ID    string `json:"id"`
							Field *struct {
								ID      string `json:"id"`
								Options []struct {
									ID    string `json:"id"`
									Name  string `json:"name"`
									Color string `json:"color"`
								} `json:"options"`
							} `json:"field"`
						} `json:"project"`
						Value *struct {
							OptionID string `json:"optionId"`
						} `json:"fieldValueByName"`
					} `json:"nodes"`
				} `json:"projectItems"`
			} `json:"issue"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": slug.Owner, "name": slug.Name, "number": number}
	if err := a.graphql(ctx, priorityFieldQuery, vars, &data); err != nil {
		return nil, err
	}

	for _, item := range data.Repository.Issue.ProjectItems.Nodes {
		f := item.Project.Field
		// A board with no Priority field (or a field that is not a
		// single-select) comes back with no id.
		if f == nil || f.ID == "" || len(f.Options) == 0 {
			continue
		}
		if projectID != "" && item.Project.ID != projectID {
			continue
		}
		out := &PriorityField{ProjectID: item.Project.ID, ItemID: item.ID, FieldID: f.ID}
		for _, o := range f.Options {
			out.Options = append(out.Options, PriorityOption{ID: o.ID, Name: o.Name, Color: o.Color})
		}
		if item.Value != nil {
			out.Current = item.Value.OptionID
		}
		return out, nil
	}
	return nil, nil
}

const setPriorityMutation = `
mutation($project: ID!, $item: ID!, $field: ID!, $option: String!) {
  updateProjectV2ItemFieldValue(input: {
    projectId: $project, itemId: $item, fieldId: $field,
    value: {singleSelectOptionId: $option}
  }) { projectV2Item { id } }
}`

// SetPriority sets the issue's priority to optionID. It needs the `project`
// token scope, which is more than reading the field does.
func (a *API) SetPriority(ctx context.Context, f PriorityField, optionID string) error {
	vars := map[string]any{"project": f.ProjectID, "item": f.ItemID, "field": f.FieldID, "option": optionID}
	return a.graphql(ctx, setPriorityMutation, vars, nil)
}

const boardFieldQuery = `
query($owner: String!, $name: String!, $login: String!) {
  repository(owner: $owner, name: $name) {
    issues(first: 50, states: OPEN, filterBy: {assignee: $login}) {
      nodes {
        projectItems(first: 10) {
          nodes {
            project {
              id
              field(name: "Priority") {
                ... on ProjectV2SingleSelectField { id options { id name color } }
              }
            }
          }
        }
      }
    }
  }
}`

// BoardPriorityField finds the board a new issue should be prioritized on.
// A new issue is on no board yet, and a repository need not be linked to one,
// so the board is the first one with a Priority field that any issue assigned
// to login is on. The result has no ItemID: there is no item until the issue
// is added with AddToProject. It is nil when no such board turns up.
func (a *API) BoardPriorityField(ctx context.Context, slug gitrepo.Slug, login string) (*PriorityField, error) {
	var data struct {
		Repository struct {
			Issues struct {
				Nodes []struct {
					ProjectItems struct {
						Nodes []struct {
							Project struct {
								ID    string `json:"id"`
								Field *struct {
									ID      string `json:"id"`
									Options []struct {
										ID    string `json:"id"`
										Name  string `json:"name"`
										Color string `json:"color"`
									} `json:"options"`
								} `json:"field"`
							} `json:"project"`
						} `json:"nodes"`
					} `json:"projectItems"`
				} `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": slug.Owner, "name": slug.Name, "login": login}
	if err := a.graphql(ctx, boardFieldQuery, vars, &data); err != nil {
		return nil, err
	}

	for _, iss := range data.Repository.Issues.Nodes {
		for _, item := range iss.ProjectItems.Nodes {
			f := item.Project.Field
			if f == nil || f.ID == "" || len(f.Options) == 0 {
				continue
			}
			out := &PriorityField{ProjectID: item.Project.ID, FieldID: f.ID}
			for _, o := range f.Options {
				out.Options = append(out.Options, PriorityOption{ID: o.ID, Name: o.Name, Color: o.Color})
			}
			return out, nil
		}
	}
	return nil, nil
}

const addToProjectMutation = `
mutation($project: ID!, $content: ID!) {
  addProjectV2ItemById(input: {projectId: $project, contentId: $content}) {
    item { id }
  }
}`

// AddToProject puts the issue or pull request with node id contentID on a
// board and returns the board item. Adding something already on the board
// returns its existing item, so this is safe when the board auto-adds.
func (a *API) AddToProject(ctx context.Context, projectID, contentID string) (string, error) {
	var data struct {
		Add struct {
			Item struct {
				ID string `json:"id"`
			} `json:"item"`
		} `json:"addProjectV2ItemById"`
	}
	vars := map[string]any{"project": projectID, "content": contentID}
	if err := a.graphql(ctx, addToProjectMutation, vars, &data); err != nil {
		return "", err
	}
	if data.Add.Item.ID == "" {
		return "", fmt.Errorf("GitHub did not return the board item")
	}
	return data.Add.Item.ID, nil
}

// Board names a project board.
type Board struct {
	Owner  string
	Number int
	Title  string
}

const projectFieldQuery = `
query($owner: String!, $number: Int!) {
  repositoryOwner(login: $owner) {
    ... on ProjectV2Owner {
      projectV2(number: $number) {
        id
        closed
        field(name: "Priority") {
          ... on ProjectV2SingleSelectField { id options { id name color } }
        }
      }
    }
  }
}`

// ProjectPriorityField reads the Priority field of one board by owner and
// number. It has no ItemID. It is nil when the board does not exist, is
// closed, or has no Priority field.
func (a *API) ProjectPriorityField(ctx context.Context, owner string, number int) (*PriorityField, error) {
	var data struct {
		Owner struct {
			Project *struct {
				ID     string `json:"id"`
				Closed bool   `json:"closed"`
				Field  *struct {
					ID      string `json:"id"`
					Options []struct {
						ID    string `json:"id"`
						Name  string `json:"name"`
						Color string `json:"color"`
					} `json:"options"`
				} `json:"field"`
			} `json:"projectV2"`
		} `json:"repositoryOwner"`
	}
	vars := map[string]any{"owner": owner, "number": number}
	if err := a.graphql(ctx, projectFieldQuery, vars, &data); err != nil {
		return nil, err
	}

	p := data.Owner.Project
	if p == nil || p.Closed || p.Field == nil || p.Field.ID == "" || len(p.Field.Options) == 0 {
		return nil, nil
	}
	out := &PriorityField{ProjectID: p.ID, FieldID: p.Field.ID}
	for _, o := range p.Field.Options {
		out.Options = append(out.Options, PriorityOption{ID: o.ID, Name: o.Name, Color: o.Color})
	}
	return out, nil
}

const candidateBoardsQuery = `
fragment board on ProjectV2 {
  id number title closed
  owner { ... on Organization { login } ... on User { login } }
  field(name: "Priority") { ... on ProjectV2SingleSelectField { id } }
}
query($owner: String!, $name: String!, $login: String!) {
  repository(owner: $owner, name: $name) {
    projectsV2(first: 20) { nodes { ...board } }
    issues(first: 50, states: OPEN, filterBy: {assignee: $login}) {
      nodes { projectItems(first: 10) { nodes { project { ...board } } } }
    }
  }
}`

// CandidateBoards lists the open boards with a Priority field that a repo's
// priorities could live on: the boards linked to the repository first, then the
// ones any issue assigned to login is on. Each board appears once, in that
// order.
func (a *API) CandidateBoards(ctx context.Context, slug gitrepo.Slug, login string) ([]Board, error) {
	type board struct {
		ID     string `json:"id"`
		Number int    `json:"number"`
		Title  string `json:"title"`
		Closed bool   `json:"closed"`
		Owner  struct {
			Login string `json:"login"`
		} `json:"owner"`
		Field *struct {
			ID string `json:"id"`
		} `json:"field"`
	}
	var data struct {
		Repository struct {
			Projects struct {
				Nodes []board `json:"nodes"`
			} `json:"projectsV2"`
			Issues struct {
				Nodes []struct {
					ProjectItems struct {
						Nodes []struct {
							Project board `json:"project"`
						} `json:"nodes"`
					} `json:"projectItems"`
				} `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": slug.Owner, "name": slug.Name, "login": login}
	if err := a.graphql(ctx, candidateBoardsQuery, vars, &data); err != nil {
		return nil, err
	}

	all := append([]board{}, data.Repository.Projects.Nodes...)
	for _, iss := range data.Repository.Issues.Nodes {
		for _, item := range iss.ProjectItems.Nodes {
			all = append(all, item.Project)
		}
	}

	var out []Board
	seen := map[string]bool{}
	for _, b := range all {
		if b.Closed || b.Field == nil || b.Field.ID == "" || seen[b.ID] {
			continue
		}
		seen[b.ID] = true
		out = append(out, Board{Owner: b.Owner.Login, Number: b.Number, Title: b.Title})
	}
	return out, nil
}
