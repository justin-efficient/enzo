package cli

import (
	"context"
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
		"undrafted: PR #77",
		"mergeable: yes, clean",
		"review:    not required here",
		"checks:    all passed",
		"main:      passing at 0d752a6",
		"merged:    PR #77 into main",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}

	e := h.findLogged(t, "merged")
	if !strings.Contains(e.Text, "#77") || !strings.Contains(e.Text, "#12") {
		t.Errorf("logged %q, want both numbers", e.Text)
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
	if !strings.Contains(h.out(), "main:      failure at 0d752a6") {
		t.Errorf("output should report the base build:\n%s", h.out())
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
