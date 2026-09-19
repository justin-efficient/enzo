package ghclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// enzo is a REST client everywhere it can be. GraphQL is here because two of
// the things `enzo finish` needs have no REST equivalent at all:
//
//   - Taking a pull request out of draft. `PATCH /pulls/{n}` with
//     "draft": false answers 200 and changes nothing — it does not fail, it
//     ignores the field. Only markPullRequestReadyForReview works.
//   - Asking whether review is required. Over REST that means reading branch
//     protection, which needs admin rights the author of a pull request
//     usually does not have. reviewDecision needs only read access.
//
// See docs/decisions/0006-finish-needs-graphql.md.

// graphqlURL derives the GraphQL endpoint from the REST base URL, which is
// where the host and any GitHub Enterprise path already live.
//
// github.com serves GraphQL from /graphql beside the REST root. Enterprise
// serves it from /api/graphql, a sibling of the /api/v3/ REST root rather than
// a child of it.
func (a *API) graphqlURL() string {
	base := a.gh.BaseURL()
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	if i := strings.LastIndex(base, "/api/v3/"); i >= 0 {
		return base[:i] + "/api/graphql"
	}
	return base + "graphql"
}

// graphqlError is one entry of a GraphQL error array.
type graphqlError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// graphql posts query with vars and decodes the "data" object into out.
//
// A GraphQL failure arrives as HTTP 200 with an "errors" array, so the status
// code alone proves nothing and the body has to be read either way.
func (a *API) graphql(ctx context.Context, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.graphqlURL(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// The client carries the token, so authentication comes with it.
	resp, err := a.gh.Client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("the token was rejected; run `enzo setup` to store a new one")
		case http.StatusForbidden:
			return fmt.Errorf("the token lacks permission or is rate limited")
		}
		return fmt.Errorf("GitHub answered %s", resp.Status)
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []graphqlError  `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("GitHub sent a reply that is not JSON: %w", err)
	}
	if len(envelope.Errors) > 0 {
		msgs := make([]string, 0, len(envelope.Errors))
		for _, e := range envelope.Errors {
			msgs = append(msgs, e.Message)
		}
		return fmt.Errorf("%s", strings.Join(msgs, "; "))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("GitHub sent a reply enzo could not read: %w", err)
	}
	return nil
}
