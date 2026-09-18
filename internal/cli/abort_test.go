package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-efficient/enzo/internal/config"
	"github.com/justin-efficient/enzo/internal/ghclient"
)

// abortReady returns a harness standing on a started branch, with an open PR
// on it, and the confirmation phrase queued on stdin.
func abortReady(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.byNumber = map[int]ghclient.Issue{
		12: {Number: 12, Title: "fix the thing", State: "open"},
	}
	h.client.existingPR = &ghclient.PullRequest{
		Number: 77, Title: "fix the thing", State: "open", Draft: true,
		Head: "justin-efficient/12-fix-the-thing", Base: "main",
		URL: "https://github.com/justin-efficient/enzo/pull/77",
	}

	// Get onto a real started branch the way enzo would.
	mustGit(t, h.root, "switch", "-q", "-c", "justin-efficient/12-fix-the-thing")
	mustGit(t, h.root, "commit", "-q", "--allow-empty", "-m", "work")
	mustGit(t, h.root, "push", "-q", "-u", "origin", "justin-efficient/12-fix-the-thing")

	h.confirm(confirmPhrase)
	return h
}

func TestAbortDestroysTheAttempt(t *testing.T) {
	h := abortReady(t)

	if err := Abort(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Abort: %v", err)
	}

	if h.client.closePRCalls != 1 || h.client.closedPR != 77 {
		t.Errorf("closed PR #%d in %d calls, want #77 once", h.client.closedPR, h.client.closePRCalls)
	}
	if got := h.branch(t); got != "main" {
		t.Errorf("left on %q, want main", got)
	}
	if branches := mustGit(t, h.root, "branch", "--format=%(refname:short)"); strings.Contains(branches, "12-fix-the-thing") {
		t.Errorf("local branch survived:\n%s", branches)
	}
	if got := h.pushedBranches(t); len(got) != 0 {
		t.Errorf("origin still has %v", got)
	}
	// Aborting an attempt is not abandoning the work: the issue is left open.
	// ghclient.Client has no way to close an issue at all, so this is a
	// guarantee of the interface rather than of the command.
	e := h.findLogged(t, "aborted")
	if !strings.Contains(e.Text, "#77") || !strings.Contains(e.Text, "#12") {
		t.Errorf("logged %q, want both numbers", e.Text)
	}
	if !strings.Contains(h.out(), "Issue #12 : will remain open") {
		t.Errorf("output should say the issue survives:\n%s", h.out())
	}
}

// The phrase is the safety. Anything else leaves everything alone.
func TestAbortNeedsThePhrase(t *testing.T) {
	for _, typed := range []string{"y", "yes", "", "nukefromorbi", "NUKEFROMORBIT"} {
		t.Run("typed "+typed, func(t *testing.T) {
			h := abortReady(t)
			h.confirm(typed)

			err := Abort(context.Background(), h.env, nil)
			if err != ErrCanceled {
				t.Fatalf("Abort = %v, want ErrCanceled", err)
			}
			if h.client.closePRCalls != 0 {
				t.Error("the PR was closed without the phrase")
			}
			if got := h.branch(t); got != "justin-efficient/12-fix-the-thing" {
				t.Errorf("left on %q, want the branch untouched", got)
			}
			if got := h.pushedBranches(t); len(got) != 1 {
				t.Errorf("origin branches = %v, want the branch still there", got)
			}
		})
	}
}

// Nothing may be destroyed before the prompt has been answered.
func TestAbortWarnsBeforeAsking(t *testing.T) {
	h := abortReady(t)
	// Work that only exists here, and a dirty file on top of it.
	mustGit(t, h.root, "commit", "-q", "--allow-empty", "-m", "never pushed")
	if err := writeFile(h.root, "scratch.txt", "wip"); err != nil {
		t.Fatal(err)
	}

	if err := Abort(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Abort: %v", err)
	}

	out := h.out()
	for _, want := range []string{
		"aborting work in",
		"justin-efficient/12-fix-the-thing",
		"PR #77",
		"1 commit not on origin",
		// The count depends on what else the harness left lying around, so
		// only the warning itself is asserted here; plural() has its own test.
		"with uncommitted changes",
		// Each row's fate, not just its name: "deleted forever" against
		// "will be closed" is what tells the reader how far the abort goes.
		"will be deleted forever",
		"will be closed",
		confirmPhrase,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt is missing %q:\n%s", want, out)
		}
	}
	// The warnings have to come before the prompt, or they are decoration.
	if i, j := strings.Index(out, "not on origin"), strings.Index(out, "type "+confirmPhrase); i < 0 || j < i {
		t.Errorf("warnings should precede the prompt:\n%s", out)
	}
}

// A branch enzo did not name is not enzo's to destroy.
func TestAbortRefusesForeignBranches(t *testing.T) {
	for _, branch := range []string{"main", "some-feature", "someone-else/12-theirs", "justin-efficient/not-a-number"} {
		t.Run(branch, func(t *testing.T) {
			h := abortReady(t)
			if branch != "main" {
				mustGit(t, h.root, "switch", "-q", "-c", branch)
			} else {
				mustGit(t, h.root, "switch", "-q", "main")
			}

			err := Abort(context.Background(), h.env, nil)
			if err == nil {
				t.Fatal("expected a refusal")
			}
			if h.client.closePRCalls != 0 {
				t.Error("nothing should have been closed")
			}
			if got := h.branch(t); got != branch {
				t.Errorf("left on %q, want %q", got, branch)
			}
		})
	}
}

// A branch with no pull request is still a branch worth deleting.
func TestAbortWithoutAPullRequest(t *testing.T) {
	h := abortReady(t)
	h.client.existingPR = nil

	if err := Abort(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if h.client.closePRCalls != 0 {
		t.Error("there was no PR to close")
	}
	if !strings.Contains(h.out(), "no open pull request") {
		t.Errorf("the prompt should say there is no PR:\n%s", h.out())
	}
	if got := h.branch(t); got != "main" {
		t.Errorf("left on %q, want main", got)
	}
	if got := h.pushedBranches(t); len(got) != 0 {
		t.Errorf("origin still has %v", got)
	}
}

func TestAbortTakesNoArguments(t *testing.T) {
	h := abortReady(t)
	requireErrorContains(t, Abort(context.Background(), h.env, []string{"12"}), "takes no arguments")
	if h.client.closePRCalls != 0 {
		t.Error("nothing should have happened")
	}
}

func TestParseBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   int
		ok     bool
	}{
		{"justin-efficient/12-fix-the-thing", 12, true},
		{"justin-efficient/7", 7, true},
		{"justin-efficient/12-fix-the-thing-with-numbers-9", 12, true},
		{"justin-efficient/0-zero", 0, false},
		{"justin-efficient/not-a-number", 0, false},
		{"justin-efficient/", 0, false},
		{"someone-else/12-theirs", 0, false},
		{"main", 0, false},
		{"12-fix-the-thing", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			got, ok := parseBranch("justin-efficient", tt.branch)
			if got != tt.want || ok != tt.ok {
				t.Errorf("parseBranch(%q) = %d, %v; want %d, %v", tt.branch, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// Every branch enzo names has to read back as the issue it was named for.
func TestParseBranchInvertsBranchName(t *testing.T) {
	for _, tt := range []struct {
		number int
		title  string
	}{
		{1, "fix it"},
		{42, "a much longer title that will certainly be cut short somewhere"},
		{9999, ""},
		{7, "!!!"},
		{5, "123 numbers up front"},
	} {
		branch := branchName("justin-efficient", tt.number, tt.title)
		got, ok := parseBranch("justin-efficient", branch)
		if !ok || got != tt.number {
			t.Errorf("parseBranch(%q) = %d, %v; want %d, true", branch, got, ok, tt.number)
		}
	}
}

func TestPlural(t *testing.T) {
	for _, tt := range []struct {
		n    int
		want string
	}{
		{1, "1 commit"},
		{2, "2 commits"},
		{0, "0 commits"},
	} {
		if got := plural(tt.n, "commit"); got != tt.want {
			t.Errorf("plural(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
