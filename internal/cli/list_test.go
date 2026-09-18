package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/justin-efficient/enzo/internal/config"
	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/ui"
)

func seedConfig(t *testing.T, h *harness, c *config.Config) {
	t.Helper()
	if err := config.Save(h.root, c); err != nil {
		t.Fatal(err)
	}
}

var listIssues = []ghclient.Issue{
	{Number: 12, Title: "fix the thing", URL: "https://github.com/justin-efficient/enzo/issues/12"},
	{Number: 34, Title: "add the other thing", URL: "https://github.com/justin-efficient/enzo/issues/34"},
}

func TestListPlainOutput(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues

	if err := List(context.Background(), h.env, []string{"--plain"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, want := range []string{"#12", "fix the thing", "#34", "add the other thing"} {
		if !strings.Contains(h.out(), want) {
			t.Errorf("output missing %q:\n%s", want, h.out())
		}
	}
	if h.pickCalls != 0 {
		t.Error("--plain should not open the picker")
	}
}

// Without a terminal, list prints rather than trying to draw a TUI.
func TestListNonInteractiveFallsBackToPlain(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.pickCalls != 0 {
		t.Error("the picker should not run without a terminal")
	}
	if !strings.Contains(h.out(), "#12") {
		t.Errorf("issues should still be printed:\n%s", h.out())
	}
}

func TestListEmptyPlain(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})

	if err := List(context.Background(), h.env, []string{"--plain"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(h.out(), "no open issues assigned to you") {
		t.Errorf("empty list should say so:\n%s", h.out())
	}
}

func TestListQueriesTheRightRepoAndUser(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.login = "someone-else"

	if err := List(context.Background(), h.env, []string{"--plain"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := h.client.gotSlug.String(); got != "justin-efficient/enzo" {
		t.Errorf("queried %q, want justin-efficient/enzo", got)
	}
	if h.client.gotLogin != "someone-else" {
		t.Errorf("queried for login %q, want the authenticated user", h.client.gotLogin)
	}
}

func TestListPassesIssuesToPicker(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues
	h.env.Interactive = true
	h.picked = ui.Result{Action: ui.ActionCancel}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.pickCalls != 1 {
		t.Fatalf("picker ran %d times, want 1", h.pickCalls)
	}
	if h.pickedRepo != "justin-efficient/enzo" {
		t.Errorf("picker titled %q", h.pickedRepo)
	}
	if len(h.pickedIssues) != 2 {
		t.Errorf("picker got %d issues, want 2", len(h.pickedIssues))
	}
}

func TestListPickerOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		result   ui.Result
		wantOut  []string
		emptyOut bool
	}{
		{
			name:     "cancel prints nothing",
			result:   ui.Result{Action: ui.ActionCancel},
			emptyOut: true,
		},
		{
			name:     "new creates an issue without printing",
			result:   ui.Result{Action: ui.ActionNew},
			emptyOut: true,
		},
		{
			name:    "picking an issue starts it",
			result:  ui.Result{Action: ui.ActionGrab, Issue: listIssues[0]},
			wantOut: []string{"#12", "fix the thing", "justin-efficient/12-fix-the-thing", "pull/300"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, defaultRemote)
			seedConfig(t, h, &config.Config{Token: "t"})
			h.client.issues = listIssues
			h.env.Interactive = true
			h.picked = tt.result
			h.drafted = ui.Draft{Title: "a brand new issue"}

			if err := List(context.Background(), h.env, nil); err != nil {
				t.Fatalf("List: %v", err)
			}
			if tt.emptyOut && h.out() != "" {
				t.Errorf("expected no output, got:\n%s", h.out())
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(h.out(), want) {
					t.Errorf("output missing %q:\n%s", want, h.out())
				}
			}
		})
	}
}

func TestListTokenSources(t *testing.T) {
	tests := []struct {
		name  string
		cfg   *config.Config
		vars  map[string]string
		want  string
		fails bool
	}{
		{"from config", &config.Config{Token: "cfg"}, nil, "cfg", false},
		{"from ENZO_TOKEN", nil, map[string]string{"ENZO_TOKEN": "env"}, "env", false},
		{"from GITHUB_TOKEN", nil, map[string]string{"GITHUB_TOKEN": "gh"}, "gh", false},
		{"config beats env", &config.Config{Token: "cfg"}, map[string]string{"GITHUB_TOKEN": "gh"}, "cfg", false},
		{"nothing at all", nil, nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, defaultRemote)
			if tt.cfg != nil {
				seedConfig(t, h, tt.cfg)
			}
			h.setenv(tt.vars)

			err := List(context.Background(), h.env, []string{"--plain"})
			if tt.fails {
				requireErrorContains(t, err, "enzo setup")
				return
			}
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if h.gotToken != tt.want {
				t.Errorf("used token %q, want %q", h.gotToken, tt.want)
			}
		})
	}
}

// A repo with no .enzo file but a token in the environment should still work.
func TestListWorksWithoutConfigFile(t *testing.T) {
	h := newHarness(t, defaultRemote)
	h.setenv(map[string]string{"GITHUB_TOKEN": "gh"})

	if err := List(context.Background(), h.env, []string{"--plain"}); err != nil {
		t.Fatalf("List: %v", err)
	}
}

func TestListUsesConfiguredHost(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t", Host: "https://ghe.internal/api/v3/"})

	if err := List(context.Background(), h.env, []string{"--plain"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.gotHost != "https://ghe.internal/api/v3/" {
		t.Errorf("host = %q, want the configured enterprise URL", h.gotHost)
	}
}

func TestListMalformedConfig(t *testing.T) {
	h := newHarness(t, defaultRemote)
	if err := os.WriteFile(config.Path(h.root), []byte("{oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	requireErrorContains(t, List(context.Background(), h.env, []string{"--plain"}), config.FileName)
}

func TestListPropagatesAPIErrors(t *testing.T) {
	t.Run("viewer fails", func(t *testing.T) {
		h := newHarness(t, defaultRemote)
		seedConfig(t, h, &config.Config{Token: "t"})
		h.client.loginErr = errBoom
		requireErrorContains(t, List(context.Background(), h.env, []string{"--plain"}), "boom")
	})
	t.Run("issue listing fails", func(t *testing.T) {
		h := newHarness(t, defaultRemote)
		seedConfig(t, h, &config.Config{Token: "t"})
		h.client.issuesErr = errBoom
		requireErrorContains(t, List(context.Background(), h.env, []string{"--plain"}), "boom")
	})
	t.Run("picker fails", func(t *testing.T) {
		h := newHarness(t, defaultRemote)
		seedConfig(t, h, &config.Config{Token: "t"})
		h.env.Interactive = true
		h.pickErr = errBoom
		requireErrorContains(t, List(context.Background(), h.env, nil), "boom")
	})
}

func TestListOutsideAGitRepo(t *testing.T) {
	h := newHarness(t, defaultRemote)
	h.env.Dir = os.TempDir()
	if err := List(context.Background(), h.env, []string{"--plain"}); err == nil {
		t.Error("List outside a repo should fail")
	}
}

func TestListWithoutOrigin(t *testing.T) {
	h := newHarness(t, "")
	seedConfig(t, h, &config.Config{Token: "t"})
	requireErrorContains(t, List(context.Background(), h.env, []string{"--plain"}), "origin")
}

func TestListRejectsUnknownFlag(t *testing.T) {
	h := newHarness(t, defaultRemote)
	if err := List(context.Background(), h.env, []string{"--nope"}); err == nil {
		t.Error("an unknown flag should be an error")
	}
}

// Titles line up even when issue numbers differ in width.
func TestListPlainAligns(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = []ghclient.Issue{
		{Number: 7, Title: "alpha"},
		{Number: 112, Title: "beta"},
	}
	if err := List(context.Background(), h.env, []string{"--plain"}); err != nil {
		t.Fatalf("List: %v", err)
	}

	var cols []int
	for _, line := range strings.Split(strings.TrimSpace(h.out()), "\n") {
		for _, title := range []string{"alpha", "beta"} {
			if i := strings.Index(line, title); i >= 0 {
				cols = append(cols, i)
			}
		}
	}
	if len(cols) != 2 {
		t.Fatalf("found %d rows, want 2:\n%s", len(cols), h.out())
	}
	if cols[0] != cols[1] {
		t.Errorf("titles start at columns %v, want them aligned:\n%s", cols, h.out())
	}
}

// nestedIssues mimics the API's newest-first order, so children arrive before
// their parent.
func nestedIssues() []ghclient.Issue {
	return []ghclient.Issue{
		{ID: 500, Number: 5, Title: "a child", ParentRepo: "justin-efficient/enzo", ParentNumber: 1},
		{ID: 400, Number: 4, Title: "unrelated"},
		{ID: 100, Number: 1, Title: "the parent"},
	}
}

func TestListPlainNestsSubIssues(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = nestedIssues()

	if err := List(context.Background(), h.env, []string{"--plain"}); err != nil {
		t.Fatalf("List: %v", err)
	}

	lines := strings.Split(strings.TrimRight(h.out(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), h.out())
	}
	// Roots keep their order; the child follows its parent.
	for i, want := range []string{"#4", "#1", "#5"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want %s:\n%s", i, lines[i], want, h.out())
		}
	}
	// The child is indented; the roots are not.
	if strings.HasPrefix(lines[0], " ") || strings.HasPrefix(lines[1], " ") {
		t.Errorf("roots should not be indented:\n%s", h.out())
	}
	if !strings.HasPrefix(lines[2], "  #5") {
		t.Errorf("the child should be indented two spaces, got %q", lines[2])
	}
}

func TestListPlainLeavesFlatListsAlone(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues

	if err := List(context.Background(), h.env, []string{"--plain"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(h.out()), "\n") {
		if strings.HasPrefix(line, " ") {
			t.Errorf("nothing should be indented when no issue has a parent: %q", line)
		}
	}
}

// The picker receives the raw list and arranges it itself, so every issue must
// still reach it.
func TestListPassesNestedIssuesToPicker(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = nestedIssues()
	h.env.Interactive = true
	h.picked = ui.Result{Action: ui.ActionCancel}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(h.pickedIssues) != 3 {
		t.Errorf("picker got %d issues, want all 3", len(h.pickedIssues))
	}
}

// Creating an issue from the "new" row returns to the list rather than exiting.
func TestListReturnsToListAfterCreating(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "a brand new issue"}
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNew},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}

	if h.client.createCalls != 1 {
		t.Errorf("CreateIssue called %d times, want 1", h.client.createCalls)
	}
	if h.pickCalls != 2 {
		t.Errorf("picker ran %d times, want 2 — the list should come back", h.pickCalls)
	}
	if e := h.findLogged(t, "created"); !strings.Contains(e.Text, "a brand new issue") {
		t.Errorf("logged text = %q", e.Text)
	}
	// Nothing is printed, so the redrawn list stays clean.
	if h.out() != "" {
		t.Errorf("creating from the list should write nothing to stdout, got:\n%s", h.out())
	}
}

// enzo keeps no issue state: it waits for GitHub's listing to catch up, then
// shows whatever GitHub reports.
func TestListWaitsForTheNewIssueToBeListed(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues
	h.client.lagPolls = 3 // GitHub needs three polls before it lists the issue
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "a brand new issue"}
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNew},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}

	if h.awaitCalls != 1 {
		t.Fatalf("waited %d times, want 1", h.awaitCalls)
	}
	if h.awaitPolls <= h.client.lagPolls {
		t.Errorf("polled %d times, want more than the %d-poll lag", h.awaitPolls, h.client.lagPolls)
	}

	// pickedIssues is what the picker was given after the wait.
	var found bool
	for _, iss := range h.pickedIssues {
		if iss.Number == 99 {
			found = true
		}
	}
	if !found {
		t.Errorf("the new issue is missing from the list after waiting: %+v", h.pickedIssues)
	}
}

// The list always comes from GitHub, never from anything enzo remembered.
func TestListRereadsFromGitHub(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "a brand new issue"}
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNew},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.client.assignedCalls < 2 {
		t.Errorf("issues fetched %d times, want a fresh read after creating", h.client.assignedCalls)
	}
}

// The wait names the issue it is waiting for.
func TestListWaitMessageNamesTheIssue(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "a brand new issue"}
	h.pickSeq = []ui.Result{{Action: ui.ActionNew}, {Action: ui.ActionCancel}}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(h.awaitMessage, "#99") {
		t.Errorf("wait message = %q, want it to name #99", h.awaitMessage)
	}
}

// A sub-issue is not ready until the listing shows its parent link too, or it
// would flash up at the top level before settling under its parent.
func TestListWaitsForTheParentLinkToBeListed(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "a child"}
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNewSub, Issue: ghclient.Issue{ID: 1200, Number: 12, Title: "the parent"}},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}

	var created *ghclient.Issue
	for i, iss := range h.pickedIssues {
		if iss.Number == 99 {
			created = &h.pickedIssues[i]
		}
	}
	if created == nil {
		t.Fatalf("the new sub-issue is missing from the list: %+v", h.pickedIssues)
	}
	if created.ParentNumber != 12 {
		t.Errorf("ParentNumber = %d, want 12 — the wait should not end before the link is listed", created.ParentNumber)
	}
}

// listed is the condition the spinner polls on.
func TestListed(t *testing.T) {
	parented := ghclient.Issue{Number: 99, ParentRepo: "o/r", ParentNumber: 12}
	tests := []struct {
		name   string
		issues []ghclient.Issue
		want   ghclient.Issue
		ready  bool
	}{
		{"absent", []ghclient.Issue{{Number: 1}}, ghclient.Issue{Number: 99}, false},
		{"present", []ghclient.Issue{{Number: 99}}, ghclient.Issue{Number: 99}, true},
		{"empty listing", nil, ghclient.Issue{Number: 99}, false},
		{"sub-issue without its link yet", []ghclient.Issue{{Number: 99}}, parented, false},
		{"sub-issue with the wrong parent", []ghclient.Issue{{Number: 99, ParentNumber: 7}}, parented, false},
		{"sub-issue fully listed", []ghclient.Issue{{Number: 99, ParentRepo: "o/r", ParentNumber: 12}}, parented, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := listed(tt.issues, tt.want); got != tt.ready {
				t.Errorf("listed() = %v, want %v", got, tt.ready)
			}
		})
	}
}

// Giving up on the wait still shows the list rather than failing.
func TestListSurvivesAWaitThatGivesUp(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "a brand new issue"}
	h.awaitGivesUp = true
	h.pickSeq = []ui.Result{{Action: ui.ActionNew}, {Action: ui.ActionCancel}}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List should not fail when the wait gives up: %v", err)
	}
	if h.pickCalls != 2 {
		t.Errorf("picker ran %d times, want 2", h.pickCalls)
	}
}

// An error while waiting is a real error worth reporting.
func TestListReportsWaitErrors(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "x"}
	h.awaitErr = errBoom
	h.pickSeq = []ui.Result{{Action: ui.ActionNew}, {Action: ui.ActionCancel}}

	requireErrorContains(t, List(context.Background(), h.env, nil), "boom")
}

// Nothing was created, so there is nothing to wait for.
func TestListDoesNotWaitAfterCanceling(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues
	h.env.Interactive = true
	h.draftErr = ErrCanceled
	h.pickSeq = []ui.Result{{Action: ui.ActionNew}, {Action: ui.ActionCancel}}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.awaitCalls != 0 {
		t.Errorf("waited %d times after canceling, want 0", h.awaitCalls)
	}
}

// Backing out of the form returns to the list too, having created nothing.
func TestListReturnsToListAfterCancelingTheForm(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues
	h.env.Interactive = true
	h.draftErr = ErrCanceled
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNew},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.client.createCalls != 0 {
		t.Error("canceling the form must not create an issue")
	}
	if h.pickCalls != 2 {
		t.Errorf("picker ran %d times, want 2 — canceling should return to the list", h.pickCalls)
	}
}

// Several issues can be created in one sitting.
func TestListCreatesRepeatedly(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "another one"}
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNew},
		{Action: ui.ActionNew},
		{Action: ui.ActionNew},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.client.createCalls != 3 {
		t.Errorf("CreateIssue called %d times, want 3", h.client.createCalls)
	}
}

// Esc on the list itself still exits.
func TestListCancelExits(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues
	h.env.Interactive = true
	h.picked = ui.Result{Action: ui.ActionCancel}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.pickCalls != 1 {
		t.Errorf("picker ran %d times, want 1 — esc should exit", h.pickCalls)
	}
}

// Picking an existing issue starts it, which is a terminal action: the picker
// does not come back.
func TestListGrabStarts(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues
	h.env.Interactive = true
	h.picked = ui.Result{Action: ui.ActionGrab, Issue: listIssues[0]}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.pickCalls != 1 {
		t.Errorf("picker ran %d times, want 1 — starting should exit", h.pickCalls)
	}
	// The row the user highlighted is the issue; re-reading it would be a
	// wasted call against an issue we already have.
	if h.client.createCalls != 0 {
		t.Errorf("starting an existing issue should not create one")
	}
	if h.client.createPRCalls != 1 {
		t.Fatalf("CreatePullRequest called %d times, want 1", h.client.createPRCalls)
	}
	if got := h.branch(t); got != "justin-efficient/12-fix-the-thing" {
		t.Errorf("left on branch %q", got)
	}
}

// A real failure while creating stops the loop rather than spinning on it.
func TestListStopsOnCreateFailure(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "doomed"}
	h.client.createErr = errBoom
	h.pickSeq = []ui.Result{{Action: ui.ActionNew}, {Action: ui.ActionNew}}

	requireErrorContains(t, List(context.Background(), h.env, nil), "boom")
	if h.pickCalls != 1 {
		t.Errorf("picker ran %d times, want 1 — a create failure should not loop", h.pickCalls)
	}
}

// Looping must not re-authenticate on every pass.
func TestListAuthenticatesOnce(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "x"}
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNew},
		{Action: ui.ActionNew},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.client.viewerCalls != 1 {
		t.Errorf("Viewer called %d times, want 1", h.client.viewerCalls)
	}
}

// ctrl+n on a highlighted issue creates a sub-issue of it and returns to the
// list.
func TestListCreatesSubIssueFromHighlightedRow(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "a child issue"}
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNewSub, Issue: ghclient.Issue{ID: 1200, Number: 12, Title: "the parent"}},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}

	if h.client.createCalls != 1 {
		t.Fatalf("CreateIssue called %d times, want 1", h.client.createCalls)
	}
	if h.client.gotNewIssue.Title != "a child issue" {
		t.Errorf("title = %q", h.client.gotNewIssue.Title)
	}
	if h.client.linkedParent != 12 {
		t.Errorf("linked under #%d, want the highlighted #12", h.client.linkedParent)
	}
	if h.client.linkedChild != 9001 {
		t.Errorf("linked child id %d, want the created issue's id", h.client.linkedChild)
	}
	if h.pickCalls != 2 {
		t.Errorf("picker ran %d times, want 2 — it should return to the list", h.pickCalls)
	}
}

// The highlighted row is already a full issue, so no lookup is needed.
func TestListSubIssueDoesNotRefetchTheParent(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "a child"}
	// Any Issue lookup would fail, proving none is made.
	h.client.issueErr = errBoom
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNewSub, Issue: ghclient.Issue{ID: 1200, Number: 12, Title: "the parent"}},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.client.createCalls != 1 {
		t.Errorf("CreateIssue called %d times, want 1", h.client.createCalls)
	}
}

// The form should say which issue the new one will hang under.
func TestListSubIssueTellsTheFormItsParent(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "a child"}
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNewSub, Issue: ghclient.Issue{ID: 1200, Number: 12, Title: "the parent"}},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.draftedParent == nil {
		t.Fatal("the form was not told about a parent")
	}
	if h.draftedParent.Number != 12 || h.draftedParent.Title != "the parent" {
		t.Errorf("form parent = %+v, want #12 the parent", h.draftedParent)
	}
}

// Backing out of a sub-issue returns to the list, creating nothing.
func TestListSubIssueCancelReturnsToList(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.env.Interactive = true
	h.draftErr = ErrCanceled
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNewSub, Issue: ghclient.Issue{ID: 1200, Number: 12}},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if h.client.createCalls != 0 {
		t.Error("canceling must not create an issue")
	}
	if h.client.linkCalls != 0 {
		t.Error("canceling must not link anything")
	}
	if h.pickCalls != 2 {
		t.Errorf("picker ran %d times, want 2", h.pickCalls)
	}
}

// Both the creation and the link are recorded.
func TestListSubIssueIsLogged(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "a child"}
	h.pickSeq = []ui.Result{
		{Action: ui.ActionNewSub, Issue: ghclient.Issue{ID: 1200, Number: 12}},
		{Action: ui.ActionCancel},
	}

	if err := List(context.Background(), h.env, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := strings.Join(h.loggedActions(), ","); got != "created,linked" {
		t.Errorf("logged actions = %q, want %q", got, "created,linked")
	}
	if h.out() != "" {
		t.Errorf("stdout should stay empty, got:\n%s", h.out())
	}
}
