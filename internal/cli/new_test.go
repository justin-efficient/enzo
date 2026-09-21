package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-efficient/enzo/internal/config"
	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/ui"
)

// newReady returns a harness with a token in place, ready to run `enzo new`.
func newReady(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	return h
}

func TestNewCreatesIssue(t *testing.T) {
	h := newReady(t)

	if err := New(context.Background(), h.env, []string{"--title", "fix the thing"}); err != nil {
		t.Fatalf("New: %v", err)
	}

	if h.client.createCalls != 1 {
		t.Fatalf("CreateIssue called %d times, want 1", h.client.createCalls)
	}
	if got := h.client.gotNewIssue.Title; got != "fix the thing" {
		t.Errorf("title = %q", got)
	}
	// The README says "assign to me"; an unassigned issue would vanish from
	// `enzo list` immediately after being created.
	if got := h.client.gotNewIssue.Assignee; got != "justin-efficient" {
		t.Errorf("assignee = %q, want the authenticated user", got)
	}
	if got := h.client.gotSlug.String(); got != "justin-efficient/enzo" {
		t.Errorf("created in %q", got)
	}
	// The creation is both announced and recorded in the log.
	if !strings.Contains(h.out(), `created a new issue #99, "fix the thing"`) {
		t.Errorf("output should name the issue it opened:\n%s", h.out())
	}
	e := h.findLogged(t, "created")
	if e.Repo != "justin-efficient/enzo" {
		t.Errorf("logged repo = %q", e.Repo)
	}
	if !strings.Contains(e.Text, "#99") || !strings.Contains(e.Text, "fix the thing") {
		t.Errorf("logged text = %q, want the number and title", e.Text)
	}
	if e.URL != "https://github.com/justin-efficient/enzo/issues/99" {
		t.Errorf("logged URL = %q", e.URL)
	}
	if e.Time.IsZero() {
		t.Error("the entry should be timestamped")
	}
	if h.client.linkCalls != 0 {
		t.Error("a top-level issue should not be linked to a parent")
	}
}

func TestNewPassesBody(t *testing.T) {
	h := newReady(t)
	if err := New(context.Background(), h.env, []string{"--title", "t", "--body", "the details"}); err != nil {
		t.Fatalf("New: %v", err)
	}
	requireSignedBody(t, h.client.gotNewIssue.Body, "the details")
}

func TestNewPromptsWhenInteractive(t *testing.T) {
	h := newReady(t)
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "typed title", Body: "typed body"}

	if err := New(context.Background(), h.env, nil); err != nil {
		t.Fatalf("New: %v", err)
	}
	if h.draftCalls != 1 {
		t.Errorf("form shown %d times, want 1", h.draftCalls)
	}
	requireSignedBody(t, h.client.gotNewIssue.Body, "typed body")
	if h.client.gotNewIssue.Title != "typed title" {
		t.Errorf("created %+v, want the drafted title and body", h.client.gotNewIssue)
	}
	if h.draftedParent != nil {
		t.Error("a top-level issue should reach the form with no parent")
	}
}

func TestNewCanceledForm(t *testing.T) {
	h := newReady(t)
	h.env.Interactive = true
	h.draftErr = ErrCanceled

	if err := New(context.Background(), h.env, nil); err != ErrCanceled {
		t.Errorf("New = %v, want ErrCanceled", err)
	}
	if h.client.createCalls != 0 {
		t.Error("canceling must not create an issue")
	}
}

// An empty title from the form means the user backed out.
func TestNewEmptyTitleIsCanceled(t *testing.T) {
	h := newReady(t)
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "   "}

	if err := New(context.Background(), h.env, nil); err != ErrCanceled {
		t.Errorf("New = %v, want ErrCanceled", err)
	}
	if h.client.createCalls != 0 {
		t.Error("a blank title must not create an issue")
	}
}

func TestNewNonInteractiveNeedsTitle(t *testing.T) {
	h := newReady(t)
	requireErrorContains(t, New(context.Background(), h.env, nil), "--title")
	if h.client.createCalls != 0 {
		t.Error("no issue should be created without a title")
	}
}

func TestNewSubWithExplicitParent(t *testing.T) {
	h := newReady(t)
	h.client.byNumber = map[int]ghclient.Issue{
		12: {ID: 1200, Number: 12, Title: "the parent"},
	}

	if err := New(context.Background(), h.env, []string{"sub", "12", "--title", "a child"}); err != nil {
		t.Fatalf("New sub: %v", err)
	}

	if h.client.linkCalls != 1 {
		t.Fatalf("LinkSubIssue called %d times, want 1", h.client.linkCalls)
	}
	// The parent is addressed by number, the child by database id.
	if h.client.linkedParent != 12 {
		t.Errorf("linked under #%d, want #12", h.client.linkedParent)
	}
	if h.client.linkedChild != 9001 {
		t.Errorf("linked child id %d, want the created issue's id 9001", h.client.linkedChild)
	}
	// A sub-issue names its parent, or the nesting is invisible until the
	// next `enzo list`.
	if !strings.Contains(h.out(), "linked: under #12") {
		t.Errorf("output should name the parent:\n%s", h.out())
	}
	e := h.findLogged(t, "linked")
	if !strings.Contains(e.Text, "#12") {
		t.Errorf("logged link = %q, want it to name the parent #12", e.Text)
	}
	if h.pickParentCalls != 0 {
		t.Error("an explicit parent number should not open the picker")
	}
}

func TestNewSubAcceptsHashPrefix(t *testing.T) {
	h := newReady(t)
	if err := New(context.Background(), h.env, []string{"sub", "#12", "--title", "a child"}); err != nil {
		t.Fatalf("New sub #12: %v", err)
	}
	if h.client.linkedParent != 12 {
		t.Errorf("linked under #%d, want #12", h.client.linkedParent)
	}
}

func TestNewSubPicksParentInteractively(t *testing.T) {
	h := newReady(t)
	h.env.Interactive = true
	h.drafted = ui.Draft{Title: "a child"}
	h.client.openIssues = []ghclient.Issue{
		{ID: 100, Number: 1, Title: "candidate one"},
		{ID: 200, Number: 2, Title: "candidate two"},
	}
	h.pickedParent = ui.Result{Action: ui.ActionChoose, Issue: ghclient.Issue{ID: 200, Number: 2, Title: "candidate two"}}

	if err := New(context.Background(), h.env, []string{"sub"}); err != nil {
		t.Fatalf("New sub: %v", err)
	}

	if h.pickParentCalls != 1 {
		t.Fatalf("parent picker ran %d times, want 1", h.pickParentCalls)
	}
	if len(h.pickedParentIssues) != 2 {
		t.Errorf("picker offered %d issues, want 2", len(h.pickedParentIssues))
	}
	if h.client.linkedParent != 2 {
		t.Errorf("linked under #%d, want #2", h.client.linkedParent)
	}
	// The form should know what it is hanging under, so it can say so.
	if h.draftedParent == nil || h.draftedParent.Number != 2 {
		t.Errorf("form got parent %+v, want #2", h.draftedParent)
	}
}

func TestNewSubCanceledPicker(t *testing.T) {
	h := newReady(t)
	h.env.Interactive = true
	h.client.openIssues = []ghclient.Issue{{ID: 100, Number: 1, Title: "candidate"}}
	h.pickedParent = ui.Result{Action: ui.ActionCancel}

	if err := New(context.Background(), h.env, []string{"sub"}); err != ErrCanceled {
		t.Errorf("New = %v, want ErrCanceled", err)
	}
	if h.client.createCalls != 0 {
		t.Error("canceling the parent picker must not create an issue")
	}
}

func TestNewSubWithNoOpenIssues(t *testing.T) {
	h := newReady(t)
	h.env.Interactive = true
	h.client.openIssues = []ghclient.Issue{}

	requireErrorContains(t, New(context.Background(), h.env, []string{"sub"}), "no open issues")
	if h.client.createCalls != 0 {
		t.Error("no issue should be created when there is no parent to pick")
	}
}

func TestNewSubNonInteractiveNeedsNumber(t *testing.T) {
	h := newReady(t)
	requireErrorContains(t, New(context.Background(), h.env, []string{"sub", "--title", "t"}), "parent issue number")
}

func TestNewSubRejectsBadNumbers(t *testing.T) {
	for _, arg := range []string{"abc", "12x", "0", "-3"} {
		t.Run(arg, func(t *testing.T) {
			h := newReady(t)
			if err := New(context.Background(), h.env, []string{"sub", arg, "--title", "t"}); err == nil {
				t.Errorf("`enzo new sub %s` should fail", arg)
			}
			if h.client.createCalls != 0 {
				t.Error("a bad parent must not create an issue")
			}
		})
	}
}

// A parent that does not exist must stop us before anything is created.
func TestNewSubUnknownParent(t *testing.T) {
	h := newReady(t)
	h.client.issueErr = errBoom

	requireErrorContains(t, New(context.Background(), h.env, []string{"sub", "404", "--title", "t"}), "boom")
	if h.client.createCalls != 0 {
		t.Error("an unresolvable parent must not create an issue")
	}
}

// If linking fails the issue still exists, so the error must say so rather
// than leaving the user thinking nothing happened.
func TestNewSubLinkFailureReportsTheOrphan(t *testing.T) {
	h := newReady(t)
	h.client.linkErr = errBoom

	err := New(context.Background(), h.env, []string{"sub", "12", "--title", "a child"})
	requireErrorContains(t, err, "created #99")
	requireErrorContains(t, err, "#12")
	// The issue exists, so the log must show it even though linking failed.
	if e := h.findLogged(t, "created"); !strings.Contains(e.Text, "#99") {
		t.Errorf("logged text = %q", e.Text)
	}
	for _, a := range h.loggedActions() {
		if a == "linked" {
			t.Error("a failed link must not be logged as linked")
		}
	}
}

func TestNewCreateFailure(t *testing.T) {
	h := newReady(t)
	h.client.createErr = errBoom
	requireErrorContains(t, New(context.Background(), h.env, []string{"--title", "t"}), "boom")
}

func TestNewNeedsToken(t *testing.T) {
	h := newHarness(t, defaultRemote)
	requireErrorContains(t, New(context.Background(), h.env, []string{"--title", "t"}), "enzo setup")
}

func TestNewWithoutOrigin(t *testing.T) {
	h := newHarness(t, "")
	seedConfig(t, h, &config.Config{Token: "t"})
	requireErrorContains(t, New(context.Background(), h.env, []string{"--title", "t"}), "origin")
}

func TestNewRejectsUnknownFlag(t *testing.T) {
	h := newReady(t)
	if err := New(context.Background(), h.env, []string{"--nope"}); err == nil {
		t.Error("an unknown flag should be an error")
	}
}

func TestParseSubArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantSub    bool
		wantParent int
		wantRest   []string
		wantErr    bool
	}{
		{"nothing", nil, false, 0, nil, false},
		{"flags only", []string{"--title", "t"}, false, 0, []string{"--title", "t"}, false},
		{"sub alone", []string{"sub"}, true, 0, []string{}, false},
		{"sub with number", []string{"sub", "12"}, true, 12, []string{}, false},
		{"sub with a title", []string{"sub", "my new sub issue work"}, true, 0, []string{"my new sub issue work"}, false},
		{"sub with number and title", []string{"sub", "12", "a child"}, true, 12, []string{"a child"}, false},
		{"positional title alone", []string{"my new issue"}, false, 0, []string{"my new issue"}, false},
		{"sub with hash number", []string{"sub", "#12"}, true, 12, []string{}, false},
		{"sub then flags", []string{"sub", "--title", "t"}, true, 0, []string{"--title", "t"}, false},
		{"sub number then flags", []string{"sub", "12", "--title", "t"}, true, 12, []string{"--title", "t"}, false},
		{"title that looks like a word", []string{"--title", "sub"}, false, 0, []string{"--title", "sub"}, false},
		{"a non-number is a title, not an error", []string{"sub", "abc"}, true, 0, []string{"abc"}, false},
		{"zero", []string{"sub", "0"}, false, 0, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, parent, rest, err := parseSubArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSubArgs(%v) should fail", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSubArgs(%v): %v", tt.args, err)
			}
			if sub != tt.wantSub {
				t.Errorf("sub = %v, want %v", sub, tt.wantSub)
			}
			if parent != tt.wantParent {
				t.Errorf("parent = %d, want %d", parent, tt.wantParent)
			}
			if strings.Join(rest, " ") != strings.Join(tt.wantRest, " ") {
				t.Errorf("rest = %v, want %v", rest, tt.wantRest)
			}
		})
	}
}

func TestNewPositionalTitle(t *testing.T) {
	h := newReady(t)

	if err := New(context.Background(), h.env, []string{"my new issue"}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := h.client.gotNewIssue.Title; got != "my new issue" {
		t.Errorf("title = %q, want the positional argument", got)
	}
	// A title given on the command line means no form, even in a terminal.
	if h.draftCalls != 0 {
		t.Error("a positional title should not open the form")
	}
}

// A title alone is enough; the body is enzo's footer and nothing else.
func TestNewPositionalTitleLeavesBodyToTheFooter(t *testing.T) {
	h := newReady(t)
	h.env.Interactive = true

	if err := New(context.Background(), h.env, []string{"my new issue"}); err != nil {
		t.Fatalf("New: %v", err)
	}
	requireSignedBody(t, h.client.gotNewIssue.Body, "")
	if h.draftCalls != 0 {
		t.Error("enzo should not prompt when the title is already known")
	}
	if h.client.createCalls != 1 {
		t.Errorf("CreateIssue called %d times, want 1", h.client.createCalls)
	}
}

func TestNewPositionalTitleWithBodyFlag(t *testing.T) {
	h := newReady(t)
	if err := New(context.Background(), h.env, []string{"my new issue", "--body", "detail"}); err != nil {
		t.Fatalf("New: %v", err)
	}
	requireSignedBody(t, h.client.gotNewIssue.Body, "detail")
	if h.client.gotNewIssue.Title != "my new issue" {
		t.Errorf("created %+v", h.client.gotNewIssue)
	}
}

// The title that motivated this: `enzo new sub "..."` must read as a title,
// not as a malformed parent number.
func TestNewSubPositionalTitle(t *testing.T) {
	h := newReady(t)
	h.env.Interactive = true
	h.client.openIssues = []ghclient.Issue{{ID: 100, Number: 1, Title: "a parent"}}
	h.pickedParent = ui.Result{Action: ui.ActionChoose, Issue: ghclient.Issue{ID: 100, Number: 1}}

	if err := New(context.Background(), h.env, []string{"sub", "my new sub issue work"}); err != nil {
		t.Fatalf("New sub: %v", err)
	}
	if got := h.client.gotNewIssue.Title; got != "my new sub issue work" {
		t.Errorf("title = %q", got)
	}
	if h.pickParentCalls != 1 {
		t.Errorf("parent picker ran %d times, want 1", h.pickParentCalls)
	}
	if h.client.linkedParent != 1 {
		t.Errorf("linked under #%d, want #1", h.client.linkedParent)
	}
	if h.draftCalls != 0 {
		t.Error("a positional title should not open the form")
	}
}

func TestNewSubParentAndPositionalTitle(t *testing.T) {
	h := newReady(t)
	if err := New(context.Background(), h.env, []string{"sub", "12", "a child issue"}); err != nil {
		t.Fatalf("New sub: %v", err)
	}
	if got := h.client.gotNewIssue.Title; got != "a child issue" {
		t.Errorf("title = %q", got)
	}
	if h.client.linkedParent != 12 {
		t.Errorf("linked under #%d, want #12", h.client.linkedParent)
	}
}

func TestNewPositionalTitleIsTrimmed(t *testing.T) {
	h := newReady(t)
	if err := New(context.Background(), h.env, []string{"  spaced out  "}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := h.client.gotNewIssue.Title; got != "spaced out" {
		t.Errorf("title = %q, want it trimmed", got)
	}
}

// An unquoted title is the likely mistake, so the error should show the fix.
func TestNewUnquotedTitleIsRejected(t *testing.T) {
	h := newReady(t)
	err := New(context.Background(), h.env, []string{"my", "new", "issue"})
	requireErrorContains(t, err, "quote the title")
	requireErrorContains(t, err, `"my new issue"`)
	if h.client.createCalls != 0 {
		t.Error("an ambiguous title must not create an issue")
	}
}

func TestNewRejectsTitleGivenTwice(t *testing.T) {
	h := newReady(t)
	err := New(context.Background(), h.env, []string{"positional", "--title", "flagged"})
	requireErrorContains(t, err, "use one")
	if h.client.createCalls != 0 {
		t.Error("conflicting titles must not create an issue")
	}
}

func TestAsIssueNumber(t *testing.T) {
	tests := []struct {
		in     string
		want   int
		wantOK bool
	}{
		{"12", 12, true},
		{"#12", 12, true},
		{"1", 1, true},
		{"0", 0, true},
		{"my new sub issue work", 0, false},
		{"12 things", 0, false},
		{"12x", 0, false},
		{"", 0, false},
		{"#", 0, false},
		{"#abc", 0, false},
		{"-3", 0, false},
		{"--title", 0, false},
		{"1.5", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := asIssueNumber(tt.in)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("asIssueNumber(%q) = (%d, %v), want (%d, %v)", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestTitleFrom(t *testing.T) {
	tests := []struct {
		name       string
		positional []string
		flag       string
		want       string
		wantErr    string
	}{
		{"neither", nil, "", "", ""},
		{"flag only", nil, "flagged", "flagged", ""},
		{"positional only", []string{"positional"}, "", "positional", ""},
		{"positional trimmed", []string{"  spaced  "}, "", "spaced", ""},
		{"flag trimmed", nil, "  spaced  ", "spaced", ""},
		{"both", []string{"a"}, "b", "", "use one"},
		{"unquoted", []string{"a", "b"}, "", "", "quote the title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := titleFrom(tt.positional, tt.flag)
			if tt.wantErr != "" {
				requireErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("titleFrom: %v", err)
			}
			if got != tt.want {
				t.Errorf("titleFrom = %q, want %q", got, tt.want)
			}
		})
	}
}

// Flags must work on either side of the title.
func TestNewTitleAndFlagOrdering(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"title then flag", []string{"my new issue", "--body", "detail"}},
		{"flag then title", []string{"--body", "detail", "my new issue"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newReady(t)
			if err := New(context.Background(), h.env, tt.args); err != nil {
				t.Fatalf("New(%v): %v", tt.args, err)
			}
			if got := h.client.gotNewIssue.Title; got != "my new issue" {
				t.Errorf("title = %q", got)
			}
			requireSignedBody(t, h.client.gotNewIssue.Body, "detail")
		})
	}
}

func TestSplitLeadingPositional(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantPositional []string
		wantFlags      []string
	}{
		{"nothing", nil, []string{}, []string{}},
		{"title only", []string{"t"}, []string{"t"}, []string{}},
		{"flags only", []string{"--body", "b"}, []string{}, []string{"--body", "b"}},
		{"title then flags", []string{"t", "--body", "b"}, []string{"t"}, []string{"--body", "b"}},
		{"several words then flags", []string{"a", "b", "--body", "c"}, []string{"a", "b"}, []string{"--body", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos, flags := splitLeadingPositional(tt.args)
			if strings.Join(pos, "|") != strings.Join(tt.wantPositional, "|") {
				t.Errorf("positional = %v, want %v", pos, tt.wantPositional)
			}
			if strings.Join(flags, "|") != strings.Join(tt.wantFlags, "|") {
				t.Errorf("flags = %v, want %v", flags, tt.wantFlags)
			}
		})
	}
}
