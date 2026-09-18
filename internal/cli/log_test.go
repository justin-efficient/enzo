package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-efficient/enzo/internal/config"
)

// A log that cannot be written must warn, not fail the issue that was created.
func TestLogFailureDoesNotFailTheCommand(t *testing.T) {
	h := newReady(t)
	h.logErr = errBoom

	if err := New(context.Background(), h.env, []string{"a title"}); err != nil {
		t.Fatalf("New should still succeed when logging fails: %v", err)
	}
	if h.client.createCalls != 1 {
		t.Errorf("CreateIssue called %d times, want 1", h.client.createCalls)
	}
	if !strings.Contains(h.stderr.String(), "could not write the log") {
		t.Errorf("stderr should warn about the log:\n%s", h.stderr.String())
	}
	// The warning belongs on stderr, so piping stdout stays clean.
	if h.out() != "" {
		t.Errorf("stdout should stay empty, got:\n%s", h.out())
	}
}

// With nowhere to log, enzo says so and carries on.
func TestNoLogLocationWarnsOnly(t *testing.T) {
	h := newReady(t)
	h.setenv(nil) // no ENZO_LOG, no XDG_STATE_HOME, no HOME

	if err := New(context.Background(), h.env, []string{"a title"}); err != nil {
		t.Fatalf("New should still succeed: %v", err)
	}
	if len(h.logged) != 0 {
		t.Errorf("nothing should have been logged, got %v", h.logged)
	}
	if !strings.Contains(h.stderr.String(), "not logging") {
		t.Errorf("stderr should explain why:\n%s", h.stderr.String())
	}
}

func TestLogPathFromConfig(t *testing.T) {
	h := newHarness(t, defaultRemote)
	want := filepath.Join(h.root, "from-config.log")
	seedConfig(t, h, &config.Config{Token: "t", Log: want})

	if err := New(context.Background(), h.env, []string{"a title"}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if h.logPath != want {
		t.Errorf("logged to %q, want the configured path %q", h.logPath, want)
	}
}

func TestLogPathFromEnv(t *testing.T) {
	h := newReady(t)
	want := filepath.Join(h.root, "from-env.log")
	h.setenv(map[string]string{"ENZO_LOG": want})

	if err := New(context.Background(), h.env, []string{"a title"}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if h.logPath != want {
		t.Errorf("logged to %q, want %q", h.logPath, want)
	}
}

func TestLogPathConfigBeatsEnv(t *testing.T) {
	h := newHarness(t, defaultRemote)
	want := filepath.Join(h.root, "from-config.log")
	seedConfig(t, h, &config.Config{Token: "t", Log: want})
	h.setenv(map[string]string{"ENZO_LOG": filepath.Join(h.root, "from-env.log")})

	if err := New(context.Background(), h.env, []string{"a title"}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if h.logPath != want {
		t.Errorf("logged to %q, want the configured path to win", h.logPath)
	}
}

// Every created issue gets its own entry, so the log is a full record.
func TestLogRecordsEveryCreation(t *testing.T) {
	h := newReady(t)
	for _, title := range []string{"first", "second", "third"} {
		if err := New(context.Background(), h.env, []string{title}); err != nil {
			t.Fatalf("New(%q): %v", title, err)
		}
	}
	if len(h.logged) != 3 {
		t.Fatalf("logged %d entries, want 3: %v", len(h.logged), h.loggedActions())
	}
	for _, e := range h.logged {
		if e.Action != "created" {
			t.Errorf("action = %q, want created", e.Action)
		}
		if e.Repo != "justin-efficient/enzo" {
			t.Errorf("repo = %q", e.Repo)
		}
	}
}

// A sub-issue records both the creation and the link.
func TestLogRecordsCreationAndLink(t *testing.T) {
	h := newReady(t)
	if err := New(context.Background(), h.env, []string{"sub", "12", "a child"}); err != nil {
		t.Fatalf("New sub: %v", err)
	}
	if got := strings.Join(h.loggedActions(), ","); got != "created,linked" {
		t.Errorf("logged actions = %q, want %q", got, "created,linked")
	}
}

// Nothing is created, so nothing is logged.
func TestLogSilentWhenCanceled(t *testing.T) {
	h := newReady(t)
	h.env.Interactive = true
	h.draftErr = ErrCanceled

	if err := New(context.Background(), h.env, nil); err != ErrCanceled {
		t.Fatalf("New = %v, want ErrCanceled", err)
	}
	if len(h.logged) != 0 {
		t.Errorf("canceling should log nothing, got %v", h.logged)
	}
}
