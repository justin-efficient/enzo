package ui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/justin-efficient/enzo/internal/ghclient"
)

func typeInto(t *testing.T, m DraftForm, s string) DraftForm {
	t.Helper()
	var model tea.Model = m
	for _, r := range s {
		model, _ = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	f, ok := model.(DraftForm)
	if !ok {
		t.Fatalf("Update returned %T, want DraftForm", model)
	}
	return f
}

func press(t *testing.T, m DraftForm, keys ...tea.KeyPressMsg) DraftForm {
	t.Helper()
	var model tea.Model = m
	for _, k := range keys {
		model, _ = model.Update(k)
	}
	return model.(DraftForm)
}

var (
	keyTab    = tea.KeyPressMsg{Code: tea.KeyTab}
	keyEnter  = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc    = tea.KeyPressMsg{Code: tea.KeyEscape}
	keySubmit = tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}
)

func TestDraftFormCollectsTitleAndBody(t *testing.T) {
	m := typeInto(t, NewDraftForm("o/r", nil, PlainStyles()), "a new thing")
	m = press(t, m, keyTab)
	m = typeInto(t, m, "some detail")
	m = press(t, m, keySubmit)

	if m.Canceled() {
		t.Fatal("submitting should not cancel")
	}
	got := m.Draft()
	if got.Title != "a new thing" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Body != "some detail" {
		t.Errorf("Body = %q", got.Body)
	}
}

// Enter moves on from the title rather than submitting, so a stray return key
// cannot open a bodyless issue.
func TestDraftFormEnterAdvancesFromTitle(t *testing.T) {
	m := typeInto(t, NewDraftForm("o/r", nil, PlainStyles()), "a title")
	m = press(t, m, keyEnter)
	if !m.onBody {
		t.Error("enter on the title should move focus to the body")
	}
	if m.done {
		t.Error("enter on the title should not submit the form")
	}
}

// Inside the body, enter is an ordinary newline.
func TestDraftFormEnterInBodyIsANewline(t *testing.T) {
	m := typeInto(t, NewDraftForm("o/r", nil, PlainStyles()), "a title")
	m = press(t, m, keyTab)
	m = typeInto(t, m, "line one")
	m = press(t, m, keyEnter)
	m = typeInto(t, m, "line two")
	m = press(t, m, keySubmit)

	if got := m.Draft().Body; !strings.Contains(got, "line one") || !strings.Contains(got, "line two") {
		t.Errorf("Body = %q, want both lines", got)
	}
	if m.Draft().Title != "a title" {
		t.Errorf("Title = %q, should be untouched", m.Draft().Title)
	}
}

func TestDraftFormTabCyclesFields(t *testing.T) {
	m := NewDraftForm("o/r", nil, PlainStyles())
	if m.onBody {
		t.Fatal("the form should open on the title")
	}
	m = press(t, m, keyTab)
	if !m.onBody {
		t.Error("tab should move to the body")
	}
	m = press(t, m, keyTab)
	if m.onBody {
		t.Error("tab should move back to the title")
	}
}

// A title is required; submitting without one must not resolve the form.
func TestDraftFormRefusesEmptyTitle(t *testing.T) {
	m := press(t, NewDraftForm("o/r", nil, PlainStyles()), keySubmit)
	if m.done {
		t.Error("submitting with no title should not complete the form")
	}
	if m.Draft().Title != "" {
		t.Errorf("Title = %q, want empty", m.Draft().Title)
	}
}

func TestDraftFormRefusesWhitespaceTitle(t *testing.T) {
	m := typeInto(t, NewDraftForm("o/r", nil, PlainStyles()), "   ")
	m = press(t, m, keySubmit)
	if m.done {
		t.Error("a whitespace-only title should not submit")
	}
}

func TestDraftFormTrimsWhitespace(t *testing.T) {
	m := typeInto(t, NewDraftForm("o/r", nil, PlainStyles()), "  a title  ")
	m = press(t, m, keySubmit)
	if got := m.Draft().Title; got != "a title" {
		t.Errorf("Title = %q, want it trimmed", got)
	}
}

func TestDraftFormCancel(t *testing.T) {
	for name, k := range map[string]tea.KeyPressMsg{
		"esc":    keyEsc,
		"ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
	} {
		t.Run(name, func(t *testing.T) {
			m := typeInto(t, NewDraftForm("o/r", nil, PlainStyles()), "a title")
			m = press(t, m, k)
			if !m.Canceled() {
				t.Errorf("%s should cancel", name)
			}
			if m.Draft().Title != "" {
				t.Errorf("canceling should yield no draft, got %q", m.Draft().Title)
			}
		})
	}
}

func TestDraftFormShowsRepo(t *testing.T) {
	out := NewDraftForm("justin-efficient/enzo", nil, PlainStyles()).View().Content
	for _, want := range []string{"new issue in justin-efficient/enzo", "title", "body", "esc cancel"} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing %q:\n%s", want, out)
		}
	}
}

// When creating a sub-issue the form should say what it will hang under.
func TestDraftFormShowsParent(t *testing.T) {
	parent := ghclient.Issue{ID: 100, Number: 12, Title: "the parent issue"}
	out := NewDraftForm("o/r", &parent, PlainStyles()).View().Content
	if !strings.Contains(out, "new sub-issue of #12") {
		t.Errorf("heading should name the parent:\n%s", out)
	}
	if !strings.Contains(out, "the parent issue") {
		t.Errorf("view should show the parent's title:\n%s", out)
	}
}

// The help line should tell you why submitting is not working.
func TestDraftFormHelpFlagsMissingTitle(t *testing.T) {
	m := NewDraftForm("o/r", nil, PlainStyles())
	if !strings.Contains(m.View().Content, "a title is required") {
		t.Errorf("empty form should say a title is required:\n%s", m.View().Content)
	}
	m = typeInto(t, m, "now it has one")
	if !strings.Contains(m.View().Content, "ctrl+n create") {
		t.Errorf("once titled, the form should offer to create:\n%s", m.View().Content)
	}
}

func TestDraftFormViewEmptyOnceDone(t *testing.T) {
	m := typeInto(t, NewDraftForm("o/r", nil, PlainStyles()), "x")
	m = press(t, m, keySubmit)
	if out := m.View().Content; out != "" {
		t.Errorf("view after submitting = %q, want empty", out)
	}
}

func TestDraftFormSubmitQuits(t *testing.T) {
	m := typeInto(t, NewDraftForm("o/r", nil, PlainStyles()), "x")
	_, cmd := m.Update(keySubmit)
	if cmd == nil {
		t.Fatal("submitting should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("submitting should quit, got %T", cmd())
	}
}

// Driven through a real terminal, end to end.
func TestDraftFormProgram(t *testing.T) {
	tm := teatest.NewTestModel(t, NewDraftForm("o/r", nil, PlainStyles()),
		teatest.WithInitialTermSize(80, 24))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "new issue in o/r")
	}, teatest.WithCheckInterval(10*time.Millisecond), teatest.WithDuration(5*time.Second))

	tm.Type("typed in a terminal")
	tm.Send(keyTab)
	tm.Type("with a body")
	tm.Send(keySubmit)

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(DraftForm)
	if final.Canceled() {
		t.Fatal("the form reported canceled")
	}
	if got := final.Draft().Title; got != "typed in a terminal" {
		t.Errorf("Title = %q", got)
	}
	if got := final.Draft().Body; got != "with a body" {
		t.Errorf("Body = %q", got)
	}
	if _, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(5*time.Second))); err != nil {
		t.Fatalf("reading final output: %v", err)
	}
}

func TestParentPickerProgram(t *testing.T) {
	tm := teatest.NewTestModel(t, NewParentPicker("o/r", parentCandidates(), PlainStyles()),
		teatest.WithInitialTermSize(80, 24))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "choose a parent issue")
	}, teatest.WithCheckInterval(10*time.Millisecond), teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(keyEnter)

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(Picker)
	if got := final.Result().Issue.Number; got != 2 {
		t.Errorf("selected #%d, want #2", got)
	}
}

// The form submits on ctrl+n, and ctrl+d no longer does anything special.
func TestDraftFormSubmitKey(t *testing.T) {
	m := typeInto(t, NewDraftForm("o/r", nil, PlainStyles()), "a title")

	stale := press(t, m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if stale.done {
		t.Error("ctrl+d should no longer submit the form")
	}

	submitted := press(t, m, tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	if !submitted.done {
		t.Error("ctrl+n should submit the form")
	}
	if got := submitted.Draft().Title; got != "a title" {
		t.Errorf("Title = %q", got)
	}
}

// ctrl+n submits from the body too, not just the title field.
func TestDraftFormSubmitsFromBody(t *testing.T) {
	m := typeInto(t, NewDraftForm("o/r", nil, PlainStyles()), "a title")
	m = press(t, m, keyTab)
	m = typeInto(t, m, "some body")
	m = press(t, m, keySubmit)

	if !m.done {
		t.Fatal("ctrl+n in the body should submit")
	}
	if m.Draft().Body != "some body" {
		t.Errorf("Body = %q", m.Draft().Body)
	}
}

func TestDraftFormHelpNamesTheSubmitKey(t *testing.T) {
	m := typeInto(t, NewDraftForm("o/r", nil, PlainStyles()), "a title")
	out := m.View().Content
	if !strings.Contains(out, "ctrl+n create") {
		t.Errorf("help should name ctrl+n:\n%s", out)
	}
	if strings.Contains(out, "ctrl+d") {
		t.Errorf("help should no longer mention ctrl+d:\n%s", out)
	}
}
