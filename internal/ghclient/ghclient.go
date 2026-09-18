// Package ghclient wraps the GitHub API surface enzo needs behind a small
// interface, so commands can be tested without a network.
package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v92/github"

	"github.com/justin-efficient/enzo/internal/gitrepo"
)

// Issue is the subset of a GitHub issue enzo displays.
type Issue struct {
	// ID is GitHub's database id. Sub-issue links are made by id, not number.
	ID        int64
	Number    int
	Title     string
	State     string
	URL       string
	Labels    []string
	UpdatedAt time.Time

	// ParentRepo and ParentNumber identify the issue this one hangs under,
	// taken from parent_issue_url. ParentNumber is 0 when there is no parent.
	ParentRepo   string
	ParentNumber int
}

// HasParent reports whether the issue is a sub-issue of another.
func (i Issue) HasParent() bool { return i.ParentNumber > 0 }

// NewIssue describes an issue to create.
type NewIssue struct {
	Title    string
	Body     string
	Assignee string
}

// Client is the GitHub surface enzo depends on.
type Client interface {
	// Viewer returns the login of the authenticated user.
	Viewer(ctx context.Context) (string, error)
	// AssignedIssues lists open issues in slug assigned to login, newest
	// activity first. Pull requests are excluded.
	AssignedIssues(ctx context.Context, slug gitrepo.Slug, login string) ([]Issue, error)
	// OpenIssues lists every open issue in slug, newest activity first. Pull
	// requests are excluded. It is what `enzo new sub` offers as parents.
	OpenIssues(ctx context.Context, slug gitrepo.Slug) ([]Issue, error)
	// Issue fetches a single issue by number.
	Issue(ctx context.Context, slug gitrepo.Slug, number int) (Issue, error)
	// CreateIssue opens a new issue and returns it.
	CreateIssue(ctx context.Context, slug gitrepo.Slug, in NewIssue) (Issue, error)
	// LinkSubIssue makes child a sub-issue of parent.
	LinkSubIssue(ctx context.Context, slug gitrepo.Slug, parentNumber int, childID int64) error
}

// API is the go-github backed implementation of Client.
type API struct{ gh *github.Client }

var _ Client = (*API)(nil)

// New returns a Client authenticated with token. host is the GitHub Enterprise
// base URL; empty means github.com.
func New(token, host string) (*API, error) {
	opts := []github.ClientOptionsFunc{github.WithAuthToken(token)}
	if h := strings.TrimSpace(host); h != "" {
		opts = append(opts, github.WithEnterpriseURLs(h, h))
	}
	c, err := github.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("could not build a GitHub client: %w", err)
	}
	return &API{gh: c}, nil
}

// NewWithHTTPClient builds an API against an arbitrary base URL. Tests use it
// to point the client at an httptest server.
func NewWithHTTPClient(baseURL string, hc *http.Client) (*API, error) {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	c, err := github.NewClient(
		github.WithHTTPClient(hc),
		github.WithEnterpriseURLs(baseURL, baseURL),
	)
	if err != nil {
		return nil, err
	}
	return &API{gh: c}, nil
}

// Viewer returns the authenticated user's login.
func (a *API) Viewer(ctx context.Context) (string, error) {
	u, _, err := a.gh.Users.Get(ctx, "")
	if err != nil {
		return "", wrap(err, "could not identify the authenticated user")
	}
	if u.GetLogin() == "" {
		return "", fmt.Errorf("GitHub returned a user with no login")
	}
	return u.GetLogin(), nil
}

// AssignedIssues lists open issues assigned to login, excluding pull requests.
func (a *API) AssignedIssues(ctx context.Context, slug gitrepo.Slug, login string) ([]Issue, error) {
	return a.listIssues(ctx, slug, login)
}

// OpenIssues lists every open issue, excluding pull requests.
func (a *API) OpenIssues(ctx context.Context, slug gitrepo.Slug) ([]Issue, error) {
	return a.listIssues(ctx, slug, "")
}

// listIssues walks every page of open issues, optionally filtered by assignee.
func (a *API) listIssues(ctx context.Context, slug gitrepo.Slug, assignee string) ([]Issue, error) {
	opts := &github.IssueListByRepoOptions{
		State:       "open",
		Assignee:    assignee,
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var out []Issue
	for {
		page, resp, err := a.gh.Issues.ListByRepo(ctx, slug.Owner, slug.Name, opts)
		if err != nil {
			return nil, wrap(err, fmt.Sprintf("could not list issues for %s", slug))
		}
		for _, iss := range page {
			if iss.IsPullRequest() {
				continue
			}
			out = append(out, convert(iss))
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.ListOptions.Page = resp.NextPage
	}
	return out, nil
}

func convert(iss *github.Issue) Issue {
	labels := make([]string, 0, len(iss.Labels))
	for _, l := range iss.Labels {
		labels = append(labels, l.GetName())
	}
	parentRepo, parentNumber := parseIssueURL(iss.GetParentIssueURL())
	return Issue{
		ID:        iss.GetID(),
		Number:    iss.GetNumber(),
		Title:     iss.GetTitle(),
		State:     iss.GetState(),
		URL:       iss.GetHTMLURL(),
		Labels:    labels,
		UpdatedAt: iss.GetUpdatedAt().Time,

		ParentRepo:   parentRepo,
		ParentNumber: parentNumber,
	}
}

// parseIssueURL pulls "owner/repo" and the issue number out of an API issue
// URL such as https://api.github.com/repos/o/r/issues/12. It returns a zero
// number when the URL is empty or does not have that shape.
func parseIssueURL(u string) (repo string, number int) {
	if u == "" {
		return "", 0
	}
	parts := strings.Split(strings.Trim(u, "/"), "/")
	// Expect ... /repos/{owner}/{repo}/issues/{number}
	if len(parts) < 5 {
		return "", 0
	}
	tail := parts[len(parts)-5:]
	if tail[0] != "repos" || tail[3] != "issues" {
		return "", 0
	}
	n, err := strconv.Atoi(tail[4])
	if err != nil || n <= 0 {
		return "", 0
	}
	return tail[1] + "/" + tail[2], n
}

// Issue fetches one issue by number.
func (a *API) Issue(ctx context.Context, slug gitrepo.Slug, number int) (Issue, error) {
	iss, _, err := a.gh.Issues.Get(ctx, slug.Owner, slug.Name, number)
	if err != nil {
		return Issue{}, wrap(err, fmt.Sprintf("could not read %s#%d", slug, number))
	}
	if iss.IsPullRequest() {
		return Issue{}, fmt.Errorf("%s#%d is a pull request, not an issue", slug, number)
	}
	return convert(iss), nil
}

// CreateIssue opens a new issue.
func (a *API) CreateIssue(ctx context.Context, slug gitrepo.Slug, in NewIssue) (Issue, error) {
	req := github.CreateIssueRequest{Title: in.Title}
	if in.Body != "" {
		req.Body = github.Ptr(in.Body)
	}
	if in.Assignee != "" {
		req.Assignees = []string{in.Assignee}
	}

	iss, _, err := a.gh.Issues.Create(ctx, slug.Owner, slug.Name, req)
	if err != nil {
		return Issue{}, wrap(err, fmt.Sprintf("could not create an issue in %s", slug))
	}
	return convert(iss), nil
}

// LinkSubIssue makes the issue with id childID a sub-issue of parentNumber.
func (a *API) LinkSubIssue(ctx context.Context, slug gitrepo.Slug, parentNumber int, childID int64) error {
	_, _, err := a.gh.SubIssue.Add(ctx, slug.Owner, slug.Name, int64(parentNumber),
		github.SubIssueRequest{SubIssueID: childID})
	if err != nil {
		return wrap(err, fmt.Sprintf("could not link the new issue under %s#%d", slug, parentNumber))
	}
	return nil
}

// wrap turns GitHub's error types into messages a CLI user can act on.
func wrap(err error, what string) error {
	var errResp *github.ErrorResponse
	if ok := asErrorResponse(err, &errResp); ok && errResp.Response != nil {
		switch errResp.Response.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("%s: the token was rejected; run `enzo setup` to store a new one", what)
		case http.StatusForbidden:
			return fmt.Errorf("%s: the token lacks permission or is rate limited", what)
		case http.StatusNotFound:
			return fmt.Errorf("%s: not found, or the token cannot see it", what)
		}
	}
	return fmt.Errorf("%s: %w", what, err)
}
