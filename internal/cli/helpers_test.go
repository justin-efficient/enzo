package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/justin-efficient/enzo/internal/eventlog"
	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/gitrepo"
	"github.com/justin-efficient/enzo/internal/ui"
)

// fakeClient stands in for GitHub and records what the commands asked for.
type fakeClient struct {
	login     string
	loginErr  error
	issues    []ghclient.Issue
	issuesErr error

	// openIssues backs OpenIssues; nil falls back to issues.
	openIssues []ghclient.Issue
	openErr    error

	// created is the issue CreateIssue returns; createErr overrides it.
	created     ghclient.Issue
	createErr   error
	gotNewIssue ghclient.NewIssue

	// lagPolls simulates GitHub's listing lag: after a create, this many
	// AssignedIssues calls still come back without the new issue.
	lagPolls    int
	lastCreated *ghclient.Issue
	pollsSince  int

	// byNumber backs Issue lookups; a miss returns issueErr or a default.
	byNumber map[int]ghclient.Issue
	issueErr error

	linkErr      error
	linkedParent int
	linkedChild  int64
	linkCalls    int

	viewerCalls   int
	createCalls   int
	assignedCalls int
	gotSlug       gitrepo.Slug
	gotLogin      string
}

func (f *fakeClient) Viewer(context.Context) (string, error) {
	f.viewerCalls++
	return f.login, f.loginErr
}

func (f *fakeClient) AssignedIssues(_ context.Context, slug gitrepo.Slug, login string) ([]ghclient.Issue, error) {
	f.assignedCalls++
	f.gotSlug, f.gotLogin = slug, login
	if f.issuesErr != nil {
		return nil, f.issuesErr
	}
	if f.lastCreated == nil {
		return f.issues, nil
	}
	// GitHub lists a new issue only once its index has caught up.
	f.pollsSince++
	if f.pollsSince <= f.lagPolls {
		return f.issues, nil
	}
	return append([]ghclient.Issue{*f.lastCreated}, f.issues...), nil
}

func (f *fakeClient) OpenIssues(_ context.Context, slug gitrepo.Slug) ([]ghclient.Issue, error) {
	f.gotSlug = slug
	if f.openErr != nil {
		return nil, f.openErr
	}
	if f.openIssues != nil {
		return f.openIssues, nil
	}
	return f.issues, nil
}

func (f *fakeClient) Issue(_ context.Context, _ gitrepo.Slug, number int) (ghclient.Issue, error) {
	if f.issueErr != nil {
		return ghclient.Issue{}, f.issueErr
	}
	if iss, ok := f.byNumber[number]; ok {
		return iss, nil
	}
	return ghclient.Issue{ID: int64(number * 1000), Number: number, Title: fmt.Sprintf("issue %d", number)}, nil
}

func (f *fakeClient) CreateIssue(_ context.Context, slug gitrepo.Slug, in ghclient.NewIssue) (ghclient.Issue, error) {
	f.createCalls++
	f.gotSlug, f.gotNewIssue = slug, in
	if f.createErr != nil {
		return ghclient.Issue{}, f.createErr
	}
	out := f.created
	if out.Number == 0 {
		out = ghclient.Issue{ID: 9001, Number: 99, Title: in.Title, URL: "https://github.com/justin-efficient/enzo/issues/99"}
	}
	f.lastCreated, f.pollsSince = &out, 0
	return out, nil
}

func (f *fakeClient) LinkSubIssue(_ context.Context, slug gitrepo.Slug, parent int, child int64) error {
	f.linkCalls++
	f.linkedParent, f.linkedChild = parent, child
	if f.linkErr != nil {
		return f.linkErr
	}
	// The listing reflects the link once it catches up, same as the issue.
	if f.lastCreated != nil {
		f.lastCreated.ParentRepo, f.lastCreated.ParentNumber = slug.String(), parent
	}
	return nil
}

// harness is a command under test plus everything it wrote.
type harness struct {
	env    Env
	root   string
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	client *fakeClient

	// gotToken and gotHost record what NewClient was built with.
	gotToken string
	gotHost  string
	// newClientErr, when set, makes NewClient fail.
	newClientErr error
	// picked is what the fake picker returns on its first call. Later calls
	// cancel, so a looping caller always terminates.
	picked ui.Result
	// pickSeq, when set, is returned one entry per call before that fallback.
	pickSeq []ui.Result
	// pickErr, when set, makes the picker fail.
	pickErr error
	// pickedIssues records what the picker was shown.
	pickedIssues []ghclient.Issue
	pickedRepo   string
	pickCalls    int
	// askToken is what the interactive prompt returns.
	askToken    string
	askTokenErr error
	askCalls    int

	// pickedParent is what the parent picker returns.
	pickedParent       ui.Result
	pickParentErr      error
	pickParentCalls    int
	pickedParentIssues []ghclient.Issue

	// logged collects the entries enzo recorded, and logPath the file it
	// would have written them to.
	logged  []eventlog.Entry
	logPath string
	logErr  error

	// awaitCalls counts how many times enzo waited for GitHub to catch up,
	// and awaitPolls how many polls that took in total.
	awaitCalls   int
	awaitPolls   int
	awaitMessage string
	awaitErr     error
	// awaitGivesUp stops the fake polling, standing in for a timeout.
	awaitGivesUp bool

	// drafted is what the issue form returns.
	drafted       ui.Draft
	draftErr      error
	draftCalls    int
	draftedParent *ghclient.Issue
}

// newHarness builds a temp git repo with an origin remote and an Env wired to
// fakes. Pass remote == "" for a repo with no origin.
func newHarness(t *testing.T, remote string) *harness {
	t.Helper()

	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if remote != "" {
		cmd := exec.Command("git", "remote", "add", "origin", remote)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git remote add: %v\n%s", err, out)
		}
	}

	defaultEnv := map[string]string{"ENZO_LOG": filepath.Join(dir, "enzo.log")}

	h := &harness{
		root:   dir,
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		client: &fakeClient{login: "justin-efficient"},
	}
	h.env = Env{
		Dir:         dir,
		Stdin:       bytes.NewReader(nil),
		Stdout:      h.stdout,
		Stderr:      h.stderr,
		Getenv:      func(k string) string { return defaultEnv[k] },
		Interactive: false,
		NewClient: func(token, host string) (ghclient.Client, error) {
			h.gotToken, h.gotHost = token, host
			if h.newClientErr != nil {
				return nil, h.newClientErr
			}
			return h.client, nil
		},
		AskToken: func(repo string) (string, error) {
			h.askCalls++
			return h.askToken, h.askTokenErr
		},
		Pick: func(repo string, issues []ghclient.Issue) (ui.Result, error) {
			h.pickCalls++
			h.pickedRepo, h.pickedIssues = repo, issues
			if h.pickErr != nil {
				return ui.Result{}, h.pickErr
			}
			return h.nextPick(), nil
		},
		PickParent: func(repo string, issues []ghclient.Issue) (ui.Result, error) {
			h.pickParentCalls++
			h.pickedParentIssues = issues
			return h.pickedParent, h.pickParentErr
		},
		AskDraft: func(repo string, parent *ghclient.Issue) (ui.Draft, error) {
			h.draftCalls++
			h.draftedParent = parent
			return h.drafted, h.draftErr
		},
		Await: func(message string, poll func() (bool, error)) error {
			h.awaitCalls++
			h.awaitMessage = message
			if h.awaitErr != nil {
				return h.awaitErr
			}
			if h.awaitGivesUp {
				return nil
			}
			// Poll for real, so the condition under test is exercised. The cap
			// keeps a condition that never holds from hanging the suite.
			for i := 0; i < 100; i++ {
				h.awaitPolls++
				ready, err := poll()
				if err != nil {
					return err
				}
				if ready {
					return nil
				}
			}
			return nil
		},
		Log: func(path string, e eventlog.Entry) error {
			h.logPath = path
			h.logged = append(h.logged, e)
			return h.logErr
		},
	}
	return h
}

// defaultRemote is the origin used by most tests.
const defaultRemote = "git@github.com:justin-efficient/enzo.git"

func (h *harness) setenv(vars map[string]string) {
	h.env.Getenv = func(k string) string { return vars[k] }
}

// nextPick hands back the scripted picker results in order, then cancels. The
// fallback keeps a looping `enzo list` from spinning forever in tests.
func (h *harness) nextPick() ui.Result {
	if n := h.pickCalls - 1; n < len(h.pickSeq) {
		return h.pickSeq[n]
	}
	if h.pickCalls == 1 && len(h.pickSeq) == 0 {
		return h.picked
	}
	return ui.Result{Action: ui.ActionCancel}
}

func (h *harness) out() string { return h.stdout.String() }

// loggedActions returns the actions recorded, in order.
func (h *harness) loggedActions() []string {
	out := make([]string, 0, len(h.logged))
	for _, e := range h.logged {
		out = append(out, e.Action)
	}
	return out
}

// findLogged returns the first entry with the given action.
func (h *harness) findLogged(t *testing.T, action string) eventlog.Entry {
	t.Helper()
	for _, e := range h.logged {
		if e.Action == action {
			return e
		}
	}
	t.Fatalf("no %q entry was logged; got %v", action, h.loggedActions())
	return eventlog.Entry{}
}

// requireErrorContains fails unless err mentions want.
func requireErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error mentioning %q, got nil", want)
	}
	if !contains(err.Error(), want) {
		t.Errorf("error %q should mention %q", err, want)
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

var errBoom = errors.New("boom")
