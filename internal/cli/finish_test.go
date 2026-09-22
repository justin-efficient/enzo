package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-efficient/enzo/internal/config"
	"github.com/justin-efficient/enzo/internal/ghclient"
)

// finishReady returns a harness standing on a started branch, with a draft
// pull request on it and nothing standing in the way of a merge.
func finishReady(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.byNumber = map[int]ghclient.Issue{
		12: {Number: 12, Title: "fix the thing", State: "open"},
	}
	h.client.existingPR = &ghclient.PullRequest{
		Number: 77, Title: "fix the thing", State: "open", Draft: true,
		NodeID: "PR_node77",
		Head:   "justin-efficient/12-fix-the-thing", Base: "main",
		URL: "https://github.com/justin-efficient/enzo/pull/77",
	}
	h.client.readiness = ghclient.Readiness{
		Mergeable: "MERGEABLE", MergeState: "CLEAN",
		Checks:     "SUCCESS",
		BaseBranch: "main", BaseHead: "0d752a6", BaseChecks: "SUCCESS",
	}

	mustGit(t, h.root, "switch", "-q", "-c", "justin-efficient/12-fix-the-thing")
	mustGit(t, h.root, "commit", "-q", "--allow-empty", "-m", "work")
	mustGit(t, h.root, "push", "-q", "-u", "origin", "justin-efficient/12-fix-the-thing")
	return h
}

func TestFinishUndraftsThenMerges(t *testing.T) {
	h := finishReady(t)

	if err := Finish(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// Out of draft before anything is read: a draft reports its merge state
	// and its review decision differently.
	if h.client.readyCalls != 1 || h.client.readiedNodeID != "PR_node77" {
		t.Errorf("undrafted %q in %d calls, want PR_node77 once", h.client.readiedNodeID, h.client.readyCalls)
	}
	if h.client.mergeCalls != 1 || h.client.mergedPR != 77 {
		t.Errorf("merged #%d in %d calls, want #77 once", h.client.mergedPR, h.client.mergeCalls)
	}

	out := h.out()
	for _, want := range []string{
		`finishing Issue #12 "fix the thing"`,
		markPass + " changes:   worktree is clean",
		markPass + " undrafted: PR #77",
		markPass + " mergeable: yes, clean",
		markPass + " review:    not required here",
		markPass + " checks:    all passed",
		markPass + " main:      passing at 0d752a6",
		markPass + " merged:    PR #77 into main",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}

	// The rows read in order, with mergeable last: the rows above it are the
	// reasons, and it is GitHub's answer.
	wantOrder := []string{"changes:", "undrafted:", "review:", "checks:", "main:", "mergeable:", "merged:"}
	at := -1
	for _, verb := range wantOrder {
		i := strings.Index(out, verb)
		if i < 0 {
			t.Fatalf("output is missing %q:\n%s", verb, out)
		}
		if i < at {
			t.Errorf("%q comes out of order:\n%s", verb, out)
		}
		at = i
	}

	e := h.findLogged(t, "merged")
	if !strings.Contains(e.Text, "#77") || !strings.Contains(e.Text, "#12") {
		t.Errorf("logged %q, want both numbers", e.Text)
	}
}

// Uncommitted work blocks the merge, but reports alongside everything else:
// one run names every reason rather than one reason at a time.
func TestFinishRefusesADirtyWorktree(t *testing.T) {
	h := finishReady(t)
	dirtyTracked(t, h, "tracked.txt")

	requireErrorContains(t, Finish(context.Background(), h.env, nil), "commit or stash")

	if h.client.mergeCalls != 0 {
		t.Errorf("merged in %d calls, want 0", h.client.mergeCalls)
	}

	out := h.out()
	for _, want := range []string{
		markFail + " changes:   1 file uncommitted: tracked.txt",
		markPass + " undrafted: PR #77",
		markPass + " mergeable: yes, clean",
		markPass + " checks:    all passed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "merged:") {
		t.Errorf("nothing was merged, so no merged row:\n%s", out)
	}
}

// Every reason is named in one go, dirty worktree included.
func TestFinishNamesEveryBlockerAtOnce(t *testing.T) {
	h := finishReady(t)
	dirtyTracked(t, h, "tracked.txt")
	h.client.readiness.Checks = "FAILURE"
	h.client.readiness.Failed = []string{"dist"}

	err := Finish(context.Background(), h.env, nil)
	for _, want := range []string{"1 file uncommitted", "checks failed"} {
		requireErrorContains(t, err, want)
	}
	if !strings.Contains(h.out(), markFail+" checks:    failed: dist") {
		t.Errorf("a failing check should be crossed:\n%s", h.out())
	}
}

// A file git was never told about is not work the pull request is missing —
// a scratch note, a build artefact — so it does not stand in the way.
func TestFinishIgnoresUntrackedFiles(t *testing.T) {
	h := finishReady(t)
	if err := os.WriteFile(filepath.Join(h.root, "scratch.txt"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Finish(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if h.client.mergeCalls != 1 {
		t.Errorf("merged in %d calls, want 1 — an untracked file must not block", h.client.mergeCalls)
	}
	if !strings.Contains(h.out(), markPass+" changes:   worktree is clean") {
		t.Errorf("an untracked file should leave the worktree clean:\n%s", h.out())
	}
}

// dirtyTracked commits a file and then edits it, leaving an uncommitted change
// to something git knows about. The harness repo has only empty commits in it,
// so there is nothing tracked to dirty otherwise.
func dirtyTracked(t *testing.T, h *harness, name string) {
	t.Helper()
	path := filepath.Join(h.root, name)
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, h.root, "add", name)
	mustGit(t, h.root, "commit", "-q", "-m", "add "+name)
	if err := os.WriteFile(path, []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Many dirty files are counted, not listed to the horizon.
func TestDescribeDirty(t *testing.T) {
	tests := []struct {
		paths []string
		want  string
	}{
		{[]string{"a.go"}, "1 file uncommitted: a.go"},
		{[]string{"a.go", "b.go"}, "2 files uncommitted: a.go, b.go"},
		{[]string{"a.go", "b.go", "c.go"}, "3 files uncommitted: a.go, b.go, c.go"},
		{[]string{"a.go", "b.go", "c.go", "d.go"}, "4 files uncommitted: a.go, b.go, c.go, and 1 more"},
		{[]string{"a", "b", "c", "d", "e"}, "5 files uncommitted: a, b, c, and 2 more"},
	}
	for _, tt := range tests {
		if got := describeDirty(tt.paths); got != tt.want {
			t.Errorf("describeDirty(%v) = %q, want %q", tt.paths, got, tt.want)
		}
	}
}

// A pull request already out of draft is not re-readied.
func TestFinishLeavesANonDraftAlone(t *testing.T) {
	h := finishReady(t)
	h.client.existingPR.Draft = false

	if err := Finish(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if h.client.readyCalls != 0 {
		t.Errorf("MarkReadyForReview called %d times on a PR that was not a draft", h.client.readyCalls)
	}
	if h.client.mergeCalls != 1 {
		t.Errorf("merged in %d calls, want 1", h.client.mergeCalls)
	}
}

// Nothing merges while something is outstanding, and the report says what.
func TestFinishRefusesAndSaysWhy(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*ghclient.Readiness)
		row   string
		why   string
	}{
		{"conflicts", func(r *ghclient.Readiness) {
			r.Mergeable = "CONFLICTING"
			r.MergeState = "DIRTY"
		}, "mergeable: no — conflicts with the base", "conflicts with the base"},

		{"mergeability not yet known", func(r *ghclient.Readiness) {
			r.Mergeable = "UNKNOWN"
		}, "mergeable: unknown", "has not finished working out"},

		{"review required", func(r *ghclient.Readiness) {
			r.ReviewDecision = "REVIEW_REQUIRED"
			r.Reviewers = []string{"someone", "a-team"}
		}, "waiting on someone, a-team", "review is required"},

		{"changes requested", func(r *ghclient.Readiness) {
			r.ReviewDecision = "CHANGES_REQUESTED"
		}, "review:    changes requested", "asked for changes"},

		{"checks failed", func(r *ghclient.Readiness) {
			r.Checks = "FAILURE"
			r.Failed = []string{"test"}
		}, "checks:    failed: test", "checks failed"},

		{"checks still running", func(r *ghclient.Readiness) {
			r.Checks = "PENDING"
			r.Pending = []string{"dist"}
		}, "checks:    still running: dist", "still running"},

		{"branch behind", func(r *ghclient.Readiness) {
			r.MergeState = "BEHIND"
		}, "mergeable: behind the base branch", "behind its base"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := finishReady(t)
			tt.setup(&h.client.readiness)

			err := Finish(context.Background(), h.env, nil)
			if err == nil {
				t.Fatal("Finish should refuse to merge")
			}
			if !strings.Contains(err.Error(), tt.why) {
				t.Errorf("error = %q, want it to mention %q", err, tt.why)
			}
			if h.client.mergeCalls != 0 {
				t.Error("it merged anyway")
			}
			// Refusing still reports every check, so one run names everything
			// that is wrong rather than one thing at a time.
			if !strings.Contains(h.out(), tt.row) {
				t.Errorf("output is missing %q:\n%s", tt.row, h.out())
			}
			for _, always := range []string{"mergeable:", "review:", "checks:", "main:"} {
				if !strings.Contains(h.out(), always) {
					t.Errorf("a refusal should still report %q:\n%s", always, h.out())
				}
			}
		})
	}
}

// A broken base branch is worth knowing about, but it is not yours to fix and
// it does not stop your work landing.
func TestFinishMergesOntoABrokenBase(t *testing.T) {
	h := finishReady(t)
	h.client.readiness.BaseChecks = "FAILURE"

	if err := Finish(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if h.client.mergeCalls != 1 {
		t.Errorf("merged in %d calls, want 1 — a red base is a warning, not a block", h.client.mergeCalls)
	}
	// The cross says main is red. It does not say your work is stuck: the
	// merge went through on the line above.
	if !strings.Contains(h.out(), markFail+" main:      failure at 0d752a6") {
		t.Errorf("output should cross the base build:\n%s", h.out())
	}
	if !strings.Contains(h.out(), markPass+" merged:    PR #77 into main") {
		t.Errorf("output should still report the merge:\n%s", h.out())
	}
}

// A base that is still building is not red, so it is not crossed.
func TestFinishDoesNotCrossABuildingBase(t *testing.T) {
	h := finishReady(t)
	h.client.readiness.BaseChecks = "PENDING"

	if err := Finish(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !strings.Contains(h.out(), markPass+" main:      building at 0d752a6") {
		t.Errorf("a building base should not be crossed:\n%s", h.out())
	}
}

// A repository with no CI at all must not read as "checks passed".
func TestFinishWithNoChecks(t *testing.T) {
	h := finishReady(t)
	h.client.readiness.Checks = ""
	h.client.readiness.BaseChecks = ""

	if err := Finish(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !strings.Contains(h.out(), "checks:    none on this commit") {
		t.Errorf("output should distinguish no checks from passing checks:\n%s", h.out())
	}
	if h.client.mergeCalls != 1 {
		t.Errorf("merged in %d calls, want 1", h.client.mergeCalls)
	}
}

// finish acts on where you are, like abort, and only on branches enzo named.
func TestFinishRefusesForeignBranches(t *testing.T) {
	for _, branch := range []string{"main", "some-feature", "someone-else/12-theirs"} {
		t.Run(branch, func(t *testing.T) {
			h := finishReady(t)
			mustGit(t, h.root, "switch", "-q", "-C", branch)

			err := Finish(context.Background(), h.env, nil)
			if err == nil {
				t.Fatalf("Finish on %q should be refused", branch)
			}
			if h.client.readyCalls != 0 || h.client.mergeCalls != 0 {
				t.Error("it touched a branch enzo did not start")
			}
		})
	}
}

func TestFinishTakesNoArguments(t *testing.T) {
	h := finishReady(t)
	if err := Finish(context.Background(), h.env, []string{"12"}); err == nil {
		t.Fatal("Finish should refuse arguments")
	}
	if h.client.mergeCalls != 0 {
		t.Error("it merged despite the bad arguments")
	}
}

// Without a pull request there is nothing to finish, and the message says what
// would make one.
func TestFinishWithoutAPullRequest(t *testing.T) {
	h := finishReady(t)
	h.client.existingPR = nil

	err := Finish(context.Background(), h.env, nil)
	if err == nil {
		t.Fatal("Finish should refuse when there is no pull request")
	}
	if !strings.Contains(err.Error(), "enzo start 12") {
		t.Errorf("error = %q, want it to point at `enzo start 12`", err)
	}
}
