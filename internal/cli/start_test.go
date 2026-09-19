package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/justin-efficient/enzo/internal/config"
	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/version"
)

// startReady returns a harness with a token in place and #12 on GitHub.
func startReady(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.byNumber = map[int]ghclient.Issue{
		12: {ID: 1200, Number: 12, Title: "fix the thing", State: "open",
			URL: "https://github.com/justin-efficient/enzo/issues/12"},
	}
	return h
}

func TestStartExistingIssue(t *testing.T) {
	h := startReady(t)

	if err := Start(context.Background(), h.env, []string{"12"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The issue already exists, so nothing should be opened.
	if h.client.createCalls != 0 {
		t.Errorf("CreateIssue called %d times, want 0", h.client.createCalls)
	}

	const want = "justin-efficient/12-fix-the-thing"
	if got := h.branch(t); got != want {
		t.Errorf("on branch %q, want %q", got, want)
	}
	if got := h.pushedBranches(t); len(got) != 1 || got[0] != want {
		t.Errorf("pushed %v, want [%s]", got, want)
	}

	pr := h.client.gotNewPR
	if pr.Head != want {
		t.Errorf("PR head = %q", pr.Head)
	}
	if pr.Base != "main" {
		t.Errorf("PR base = %q, want the repo default branch", pr.Base)
	}
	if !pr.Draft {
		t.Error("the PR should be a draft")
	}
	if pr.Title != "fix the thing" {
		t.Errorf("PR title = %q, want the issue's", pr.Title)
	}
	// The closing keyword is the link between the PR and the issue, and the
	// footer says what opened it — the same signature an issue body gets.
	if want := signedBody("Closes #12"); pr.Body != want {
		t.Errorf("PR body =\n%q\nwant\n%q", pr.Body, want)
	}

	e := h.findLogged(t, "drafted")
	if !strings.Contains(e.Text, "#300") || !strings.Contains(e.Text, "#12") {
		t.Errorf("logged text = %q, want both numbers", e.Text)
	}
	// The verbs say what enzo did: it made both the branch and the PR.
	if !strings.Contains(h.out(), "created: justin-efficient/12-fix-the-thing") {
		t.Errorf("output should say it created the branch:\n%s", h.out())
	}
	if !strings.Contains(h.out(), "drafted: PR #300") {
		t.Errorf("output should say it drafted the PR:\n%s", h.out())
	}
	if !strings.Contains(h.out(), "pull/300") {
		t.Errorf("output should name the PR:\n%s", h.out())
	}
}

// A branch with nothing on it cannot be a pull request, so enzo puts an empty
// commit on it. See docs/decisions/0004-draft-pr-needs-a-commit.md.
func TestStartCommitsSoThePRCanExist(t *testing.T) {
	h := startReady(t)

	if err := Start(context.Background(), h.env, []string{"12"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := mustGit(t, h.root, "rev-list", "--count", "main..HEAD"); got != "1" {
		t.Fatalf("%s commits ahead of main, want 1", got)
	}
	// The subject names what made the commit, so it is not a mystery later.
	if subject := mustGit(t, h.root, "log", "-1", "--format=%s"); subject != version.Credit() {
		t.Errorf("commit subject = %q, want enzo to sign it", subject)
	}
	// Empty is the point: the commit exists to give GitHub a diff to hang a
	// pull request on, not to change anything.
	if diff := mustGit(t, h.root, "diff", "--name-only", "HEAD~1", "HEAD"); diff != "" {
		t.Errorf("the commit should be empty, it touched:\n%s", diff)
	}
}

// A branch that already has work on it needs no commit of enzo's.
func TestStartDoesNotCommitOnTopOfWork(t *testing.T) {
	h := startReady(t)
	const branch = "justin-efficient/12-fix-the-thing"
	mustGit(t, h.root, "switch", "-c", branch)
	mustGit(t, h.root, "commit", "-q", "--allow-empty", "-m", "real work")
	mustGit(t, h.root, "switch", "main")

	if err := Start(context.Background(), h.env, []string{"12"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := mustGit(t, h.root, "rev-list", "--count", "main..HEAD"); got != "1" {
		t.Errorf("%s commits ahead of main, want the 1 that was already there", got)
	}
	if subject := mustGit(t, h.root, "log", "-1", "--format=%s"); subject != "real work" {
		t.Errorf("tip commit = %q, want the work already on the branch", subject)
	}
	if got := h.branch(t); got != branch {
		t.Errorf("on branch %q, want the existing %q", got, branch)
	}
}

// The branch enzo starts can already be contained in the base without any
// local ref saying so: origin/main is only as fresh as the last fetch, and a
// merged pull request moves main underneath it. Comparing against the stale
// ref reads as "1 commit ahead", skips the empty commit, and GitHub rejects
// the pull request with "No commits between main and <branch>".
func TestStartFetchesTheBaseBeforeDeciding(t *testing.T) {
	h := startReady(t)

	before := mustGit(t, h.root, "rev-parse", "HEAD")
	// Work on this branch that has since been merged into main...
	mustGit(t, h.root, "commit", "-q", "--allow-empty", "-m", "work that got merged")
	mustGit(t, h.root, "push", "-q", "origin", "main")
	// ...while the local remote-tracking ref stays where it was, as it does
	// until something fetches.
	mustGit(t, h.root, "update-ref", "refs/remotes/origin/main", before)

	if mustGit(t, h.root, "rev-parse", "refs/remotes/origin/main") != before {
		t.Fatal("the setup is meant to leave origin/main stale")
	}
	// Against the stale ref the branch looks ahead, which is the trap.
	if got := mustGit(t, h.root, "rev-list", "--count", "refs/remotes/origin/main..HEAD"); got != "1" {
		t.Fatalf("stale comparison says %s commits ahead, want the misleading 1", got)
	}

	if err := Start(context.Background(), h.env, []string{"12"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The branch has to carry something main does not, or the PR is a 422.
	ahead := mustGit(t, h.root, "rev-list", "--count", "origin/main..HEAD")
	if ahead == "0" {
		t.Errorf("branch is %s commits ahead of the real main; GitHub would refuse the PR", ahead)
	}
	if subject := mustGit(t, h.root, "log", "-1", "--format=%s"); subject != version.Credit() {
		t.Errorf("tip commit = %q, want enzo's empty commit", subject)
	}
	if h.client.createPRCalls != 1 {
		t.Errorf("CreatePullRequest called %d times, want 1", h.client.createPRCalls)
	}
}

// A base that cannot be fetched falls back to what is on disk rather than
// failing: offline there is nothing better to consult.
func TestStartWorksWithAnUnreachableOrigin(t *testing.T) {
	h := startReady(t)
	// origin still rewrites to the bare repo, but the bare repo is gone.
	if err := os.RemoveAll(h.bare); err != nil {
		t.Fatal(err)
	}

	err := Start(context.Background(), h.env, []string{"12"})
	// The push cannot succeed either, so the command fails — but it must fail
	// on the push, having already decided an empty commit was needed.
	if err == nil {
		t.Fatal("expected the push to fail against an unreachable origin")
	}
	if subject := mustGit(t, h.root, "log", "-1", "--format=%s"); subject != version.Credit() {
		t.Errorf("tip commit = %q, want enzo to have committed despite the failed fetch", subject)
	}
}

// Work for one issue must not arrive carrying work for another. A new branch
// starts from origin's default branch, whatever you were standing on.
func TestStartBranchesFromMainNotFromHEAD(t *testing.T) {
	h := startReady(t)
	base := mustGit(t, h.root, "rev-parse", "HEAD")

	// Stand on someone else's unfinished work.
	mustGit(t, h.root, "switch", "-q", "-c", "some-other-feature")
	mustGit(t, h.root, "commit", "-q", "--allow-empty", "-m", "unrelated work in progress")

	if err := Start(context.Background(), h.env, []string{"12"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := h.branch(t); got != "justin-efficient/12-fix-the-thing" {
		t.Fatalf("on branch %q", got)
	}
	// One commit past main: enzo's own, and nothing from the other branch.
	if got := mustGit(t, h.root, "rev-list", "--count", base+"..HEAD"); got != "1" {
		t.Errorf("branch is %s commits past main, want 1 — the other branch came along", got)
	}
	if got := mustGit(t, h.root, "log", "-1", "--format=%s", "HEAD~1"); got == "unrelated work in progress" {
		t.Error("the new branch was cut from the feature branch, not from main")
	}
	if subject := mustGit(t, h.root, "log", "-1", "--format=%s"); subject != version.Credit() {
		t.Errorf("tip commit = %q", subject)
	}
}

// A branch that already exists is switched to as it is. enzo does not move
// someone's existing branch onto main behind their back.
func TestStartDoesNotRebaseAnExistingBranch(t *testing.T) {
	h := startReady(t)
	const branch = "justin-efficient/12-fix-the-thing"
	base := mustGit(t, h.root, "rev-parse", "HEAD")

	// The branch exists, cut from somewhere else, with real work on it.
	mustGit(t, h.root, "switch", "-q", "-c", "elsewhere")
	mustGit(t, h.root, "commit", "-q", "--allow-empty", "-m", "groundwork")
	mustGit(t, h.root, "switch", "-q", "-c", branch)
	mustGit(t, h.root, "commit", "-q", "--allow-empty", "-m", "real work")
	tip := mustGit(t, h.root, "rev-parse", "HEAD")
	mustGit(t, h.root, "switch", "-q", "main")

	if err := Start(context.Background(), h.env, []string{"12"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if got := mustGit(t, h.root, "rev-parse", "HEAD"); got != tip {
		t.Errorf("branch tip moved to %s, want it left at %s", got, tip)
	}
	if got := mustGit(t, h.root, "rev-list", "--count", base+"..HEAD"); got != "2" {
		t.Errorf("branch is %s commits past main, want the 2 already on it", got)
	}
}

func TestStartOpensTheIssueWhenGivenATitle(t *testing.T) {
	h := startReady(t)

	if err := Start(context.Background(), h.env, []string{"fix the thing"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if h.client.createCalls != 1 {
		t.Fatalf("CreateIssue called %d times, want 1", h.client.createCalls)
	}
	if got := h.client.gotNewIssue.Title; got != "fix the thing" {
		t.Errorf("issue title = %q", got)
	}
	if got := h.client.gotNewIssue.Assignee; got != "justin-efficient" {
		t.Errorf("assignee = %q, want the authenticated user", got)
	}
	// The branch is named after the issue that was just opened, #99.
	if got := h.branch(t); got != "justin-efficient/99-fix-the-thing" {
		t.Errorf("on branch %q", got)
	}
	if want := signedBody("Closes #99"); h.client.gotNewPR.Body != want {
		t.Errorf("PR body =\n%q\nwant\n%q", h.client.gotNewPR.Body, want)
	}
	if actions := h.loggedActions(); len(actions) != 2 {
		t.Errorf("logged %v, want the issue and the PR", actions)
	}
}

func TestStartBodyFlagGoesToTheIssue(t *testing.T) {
	h := startReady(t)

	if err := Start(context.Background(), h.env, []string{"fix it", "--body", "the detail"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	requireSignedBody(t, h.client.gotNewIssue.Body, "the detail")
}

// Running start twice should settle rather than open a second pull request.
func TestStartIsIdempotent(t *testing.T) {
	h := startReady(t)
	// The state the first `enzo start` would have left: the branch exists
	// here, and the pull request is open on GitHub.
	mustGit(t, h.root, "switch", "-c", "justin-efficient/12-fix-the-thing")
	mustGit(t, h.root, "switch", "main")
	h.client.existingPR = &ghclient.PullRequest{
		Number: 77, Title: "fix the thing", State: "open", Draft: true,
		Head: "justin-efficient/12-fix-the-thing", Base: "main",
		URL: "https://github.com/justin-efficient/enzo/pull/77",
	}

	if err := Start(context.Background(), h.env, []string{"12"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if h.client.createPRCalls != 0 {
		t.Errorf("CreatePullRequest called %d times, want 0 — one is already open", h.client.createPRCalls)
	}
	// Getting you onto the branch is still the point of the command.
	if got := h.branch(t); got != "justin-efficient/12-fix-the-thing" {
		t.Errorf("on branch %q", got)
	}
	// Nothing was made this time, and the verbs have to say so rather than
	// claiming the work of the first `enzo start` over again.
	if !strings.Contains(h.out(), "switched to: justin-efficient/12-fix-the-thing") {
		t.Errorf("output should say it switched to the branch:\n%s", h.out())
	}
	if !strings.Contains(h.out(), "found:") || strings.Contains(h.out(), "drafted:") {
		t.Errorf("output should say it found the PR, not drafted one:\n%s", h.out())
	}
	if !strings.Contains(h.out(), "pull/77") {
		t.Errorf("output should name the PR that is already open:\n%s", h.out())
	}
	if len(h.logged) != 0 {
		t.Errorf("nothing was created, so nothing should be logged; got %v", h.loggedActions())
	}
}

// A failure that was going to happen anyway should not leave you on a new
// branch wondering what enzo did.
func TestStartStaysPutWhenGitHubFails(t *testing.T) {
	for _, tt := range []struct {
		name string
		set  func(*fakeClient)
	}{
		{"the issue cannot be read", func(c *fakeClient) { c.issueErr = errBoom }},
		{"the default branch cannot be read", func(c *fakeClient) { c.defaultBranchErr = errBoom }},
		{"the PR lookup fails", func(c *fakeClient) { c.prLookupErr = errBoom }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := startReady(t)
			tt.set(h.client)

			requireErrorContains(t, Start(context.Background(), h.env, []string{"12"}), "boom")

			if got := h.branch(t); got != "main" {
				t.Errorf("left on branch %q, want main", got)
			}
			if got := h.pushedBranches(t); len(got) != 0 {
				t.Errorf("pushed %v, want nothing", got)
			}
		})
	}
}

func TestStartArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"nothing to go on", nil, "give an issue number"},
		{"a number and a title", []string{"12", "fix it"}, "already has a title"},
		{"a number and --title", []string{"12", "--title", "fix it"}, "already has a title"},
		{"issue zero", []string{"0"}, "issue numbers start at 1"},
		{"an unquoted title", []string{"fix", "the", "thing"}, "quote the title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := startReady(t)
			requireErrorContains(t, Start(context.Background(), h.env, tt.args), tt.want)

			// Nothing should have happened on either side.
			if h.client.createCalls != 0 || h.client.createPRCalls != 0 {
				t.Error("a rejected command should not reach GitHub")
			}
			if got := h.branch(t); got != "main" {
				t.Errorf("left on branch %q, want main", got)
			}
		})
	}
}

// "#12" is how issues are written; it should work where "12" does.
func TestStartAcceptsHashPrefixedNumbers(t *testing.T) {
	h := startReady(t)

	if err := Start(context.Background(), h.env, []string{"#12"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := h.branch(t); got != "justin-efficient/12-fix-the-thing" {
		t.Errorf("on branch %q", got)
	}
}

// Starting a closed issue is allowed — you may be reopening work — but it is
// surprising enough to say out loud.
func TestStartWarnsOnAClosedIssue(t *testing.T) {
	h := startReady(t)
	h.client.byNumber = map[int]ghclient.Issue{
		12: {Number: 12, Title: "fix the thing", State: "closed"},
	}

	if err := Start(context.Background(), h.env, []string{"12"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !strings.Contains(h.stderr.String(), "closed") {
		t.Errorf("stderr should mention the state:\n%s", h.stderr.String())
	}
	if h.client.createPRCalls != 1 {
		t.Error("the warning should not stop the command")
	}
}

func TestBranchName(t *testing.T) {
	tests := []struct {
		name   string
		number int
		title  string
		want   string
	}{
		{"words become dashes", 12, "fix the parser crash", "justin/12-fix-the-parser-crash"},
		{"case is flattened", 3, "Fix The Thing", "justin/3-fix-the-thing"},
		{"punctuation collapses", 7, "fix: the *thing*, again!", "justin/7-fix-the-thing-again"},
		{"leading and trailing junk goes", 8, "  --wat--  ", "justin/8-wat"},
		{"digits are kept", 9, "bump go 1.26", "justin/9-bump-go-1-26"},
		{"non-ascii drops out", 10, "café ☕ time", "justin/10-caf-time"},
		{"a title of only punctuation leaves the number", 11, "!!!", "justin/11"},
		{"an empty title leaves the number", 13, "", "justin/13"},
		{
			"a long title is cut on a word boundary",
			14,
			"this is an extremely long issue title that nobody would ever want to type out in full",
			"justin/14-this-is-an-extremely-long-issue-title-that",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := branchName("justin", tt.number, tt.title); got != tt.want {
				t.Errorf("branchName(%d, %q) = %q, want %q", tt.number, tt.title, got, tt.want)
			}
		})
	}
}

// The slug is bounded, so a branch name stays typeable however long the title.
func TestBranchNameLength(t *testing.T) {
	long := strings.Repeat("averylongwordwithnobreaks", 10)
	got := branchName("justin", 1, long)
	if len(got) > len("justin/1-")+maxSlug {
		t.Errorf("branchName = %q (%d chars), want at most %d", got, len(got), len("justin/1-")+maxSlug)
	}
}
