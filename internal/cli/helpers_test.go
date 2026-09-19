package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-efficient/enzo/internal/eventlog"
	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/gitrepo"
	"github.com/justin-efficient/enzo/internal/ui"
	"github.com/justin-efficient/enzo/internal/version"
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

	// defaultBranch backs DefaultBranch; empty means "main".
	defaultBranch    string
	defaultBranchErr error

	// existingPR is what PullRequestForBranch finds; nil means none.
	existingPR    *ghclient.PullRequest
	prLookupErr   error
	prLookupCalls int
	gotPRBranch   string

	// createdPR is what CreatePullRequest returns; createPRErr overrides it.
	createdPR     ghclient.PullRequest
	createPRErr   error
	createPRCalls int
	gotNewPR      ghclient.NewPullRequest

	// closedPR records the pull request `enzo abort` closed.
	closedPR     int
	closePRErr   error
	closePRCalls int

	// What `enzo finish` does and sees.
	readyCalls    int
	readiedNodeID string
	readyErr      error
	readiness     ghclient.Readiness
	readinessErr  error
	mergedPR      int
	mergeCalls    int
	mergeErr      error

	viewerCalls   int
	createCalls   int
	assignedCalls int
	gotSlug       gitrepo.Slug
	gotLogin      string
}

func (f *fakeClient) DefaultBranch(context.Context, gitrepo.Slug) (string, error) {
	if f.defaultBranchErr != nil {
		return "", f.defaultBranchErr
	}
	if f.defaultBranch != "" {
		return f.defaultBranch, nil
	}
	return "main", nil
}

func (f *fakeClient) PullRequestForBranch(_ context.Context, _ gitrepo.Slug, branch string) (*ghclient.PullRequest, error) {
	f.prLookupCalls++
	f.gotPRBranch = branch
	if f.prLookupErr != nil {
		return nil, f.prLookupErr
	}
	return f.existingPR, nil
}

func (f *fakeClient) CreatePullRequest(_ context.Context, slug gitrepo.Slug, in ghclient.NewPullRequest) (ghclient.PullRequest, error) {
	f.createPRCalls++
	f.gotSlug, f.gotNewPR = slug, in
	if f.createPRErr != nil {
		return ghclient.PullRequest{}, f.createPRErr
	}
	out := f.createdPR
	if out.Number == 0 {
		out = ghclient.PullRequest{
			Number: 300,
			Title:  in.Title,
			State:  "open",
			Draft:  in.Draft,
			Head:   in.Head,
			Base:   in.Base,
			URL:    "https://github.com/justin-efficient/enzo/pull/300",
		}
	}
	return out, nil
}

func (f *fakeClient) ClosePullRequest(_ context.Context, _ gitrepo.Slug, number int) error {
	f.closePRCalls++
	f.closedPR = number
	return f.closePRErr
}

func (f *fakeClient) MarkReadyForReview(_ context.Context, nodeID string) error {
	f.readyCalls++
	f.readiedNodeID = nodeID
	return f.readyErr
}

func (f *fakeClient) Readiness(_ context.Context, _ gitrepo.Slug, _ int) (ghclient.Readiness, error) {
	return f.readiness, f.readinessErr
}

func (f *fakeClient) MergePullRequest(_ context.Context, _ gitrepo.Slug, number int) error {
	f.mergeCalls++
	f.mergedPR = number
	return f.mergeErr
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
	env  Env
	root string
	// bare is the local repository origin pushes to, so a test push never
	// leaves the machine.
	bare   string
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
		mustGit(t, dir, args...)
	}
	bare := ""
	if remote != "" {
		mustGit(t, dir, "remote", "add", "origin", remote)
		// origin's URL has to stay a GitHub one — it is what the slug is
		// parsed from — but nothing in this suite may reach GitHub. insteadOf
		// rewrites it to a local bare repo at transport time, so fetch and
		// push are both real and both local, while `git config --get
		// remote.origin.url` still reads back the GitHub URL.
		bare = filepath.Join(t.TempDir(), "origin.git")
		mustGit(t, "", "init", "-q", "--bare", bare)
		mustGit(t, dir, "config", "url."+bare+".insteadOf", remote)
		// origin needs a base branch to fetch, the same as a real one.
		mustGit(t, dir, "push", "-q", "origin", "main")
	}

	defaultEnv := map[string]string{"ENZO_LOG": filepath.Join(dir, "enzo.log")}

	h := &harness{
		root:   dir,
		bare:   bare,
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

// mustGit runs a git command in dir, failing the test if it does not succeed.
// An empty dir means the process working directory, for `git init --bare`.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
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

// requireSignedBody fails unless the issue body is what the user wrote, signed
// with enzo's footer. Both halves are checked: a footer that ate the body
// would pass either one alone.
func requireSignedBody(t *testing.T, got, written string) {
	t.Helper()
	if !strings.Contains(got, version.Credit()) {
		t.Errorf("body = %q, want it signed with %q", got, version.Credit())
	}
	if written == "" {
		if got != "*"+version.Credit()+"*" {
			t.Errorf("body = %q, want the footer alone", got)
		}
		return
	}
	if !strings.HasPrefix(got, written+"\n") {
		t.Errorf("body = %q, want it to open with %q", got, written)
	}
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

// confirm queues what the user types at a confirmation prompt.
func (h *harness) confirm(answer string) {
	h.env.Stdin = strings.NewReader(answer + "\n")
}

// writeFile drops a file into the worktree, to dirty it.
func writeFile(dir, name, body string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
}

// branch returns the branch the worktree is on.
func (h *harness) branch(t *testing.T) string {
	t.Helper()
	return mustGit(t, h.root, "rev-parse", "--abbrev-ref", "HEAD")
}

// pushedBranches lists what reached origin beyond "main", which newHarness
// seeds so there is a base branch to fetch.
func (h *harness) pushedBranches(t *testing.T) []string {
	t.Helper()
	out := mustGit(t, h.bare, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	var branches []string
	for _, b := range strings.Split(out, "\n") {
		if b != "" && b != "main" {
			branches = append(branches, b)
		}
	}
	return branches
}
