package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/justin-efficient/enzo/internal/config"
)

func TestSetupWritesConfig(t *testing.T) {
	h := newHarness(t, defaultRemote)

	if err := Setup(context.Background(), h.env, []string{"--token", "ghp_abc"}); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	cfg, err := config.Load(h.root)
	if err != nil {
		t.Fatalf("loading the config Setup wrote: %v", err)
	}
	if cfg.Token != "ghp_abc" {
		t.Errorf("stored token = %q, want %q", cfg.Token, "ghp_abc")
	}
	if h.gotToken != "ghp_abc" {
		t.Errorf("validated with token %q, want %q", h.gotToken, "ghp_abc")
	}
	if h.client.viewerCalls != 1 {
		t.Errorf("Viewer called %d times, want 1", h.client.viewerCalls)
	}
	if !strings.Contains(h.out(), "authenticated: justin-efficient") {
		t.Errorf("output should confirm the identity:\n%s", h.out())
	}
}

// A bad token must not be written to disk.
func TestSetupRejectsBadToken(t *testing.T) {
	h := newHarness(t, defaultRemote)
	h.client.loginErr = errBoom

	err := Setup(context.Background(), h.env, []string{"--token", "ghp_bad"})
	requireErrorContains(t, err, "boom")

	if config.Exists(h.root) {
		t.Error("Setup wrote a config despite the token being rejected")
	}
}

func TestSetupAddsGitignoreEntry(t *testing.T) {
	h := newHarness(t, defaultRemote)
	if err := Setup(context.Background(), h.env, []string{"--token", "ghp_abc"}); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	b, err := os.ReadFile(h.root + "/.gitignore")
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(b), config.FileName) {
		t.Errorf(".gitignore should list %s:\n%s", config.FileName, b)
	}
	if !strings.Contains(h.out(), ".gitignore") {
		t.Errorf("output should mention the .gitignore change:\n%s", h.out())
	}
}

// The token file must not be committable by accident.
func TestSetupConfigIsNotTrackedByGit(t *testing.T) {
	h := newHarness(t, defaultRemote)
	if err := Setup(context.Background(), h.env, []string{"--token", "ghp_abc"}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	out := gitStatus(t, h.root)
	if strings.Contains(out, config.FileName) {
		t.Errorf("git still sees %s as untracked:\n%s", config.FileName, out)
	}
}

func TestSetupRefusesToClobber(t *testing.T) {
	h := newHarness(t, defaultRemote)
	if err := config.Save(h.root, &config.Config{Token: "old"}); err != nil {
		t.Fatal(err)
	}

	err := Setup(context.Background(), h.env, []string{"--token", "new"})
	requireErrorContains(t, err, "--force")

	cfg, _ := config.Load(h.root)
	if cfg.Token != "old" {
		t.Errorf("token = %q, want the original %q", cfg.Token, "old")
	}
}

func TestSetupForceReplaces(t *testing.T) {
	h := newHarness(t, defaultRemote)
	if err := config.Save(h.root, &config.Config{Token: "old", DefaultReviewers: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}

	if err := Setup(context.Background(), h.env, []string{"--token", "new", "--force"}); err != nil {
		t.Fatalf("Setup --force: %v", err)
	}

	cfg, _ := config.Load(h.root)
	if cfg.Token != "new" {
		t.Errorf("token = %q, want %q", cfg.Token, "new")
	}
	// Settings unrelated to the token must survive a re-setup.
	if strings.Join(cfg.DefaultReviewers, ",") != "alice" {
		t.Errorf("DefaultReviewers = %v, want [alice] to be preserved", cfg.DefaultReviewers)
	}
}

func TestSetupStoresHost(t *testing.T) {
	h := newHarness(t, defaultRemote)
	const host = "https://ghe.internal/api/v3/"

	if err := Setup(context.Background(), h.env, []string{"--token", "t", "--host", host}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if h.gotHost != host {
		t.Errorf("validated against host %q, want %q", h.gotHost, host)
	}
	cfg, _ := config.Load(h.root)
	if cfg.Host != host {
		t.Errorf("stored host = %q, want %q", cfg.Host, host)
	}
}

func TestSetupReadsTokenFromStdin(t *testing.T) {
	h := newHarness(t, defaultRemote)
	h.env.Stdin = bytes.NewBufferString("ghp_piped\n")

	if err := Setup(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	cfg, _ := config.Load(h.root)
	if cfg.Token != "ghp_piped" {
		t.Errorf("token = %q, want %q", cfg.Token, "ghp_piped")
	}
	if h.askCalls != 0 {
		t.Error("Setup should not prompt when stdin supplies a token")
	}
}

func TestSetupPromptsWhenInteractive(t *testing.T) {
	h := newHarness(t, defaultRemote)
	h.env.Interactive = true
	h.askToken = "ghp_typed"

	if err := Setup(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if h.askCalls != 1 {
		t.Errorf("prompt shown %d times, want 1", h.askCalls)
	}
	cfg, _ := config.Load(h.root)
	if cfg.Token != "ghp_typed" {
		t.Errorf("token = %q, want %q", cfg.Token, "ghp_typed")
	}
}

func TestSetupCanceledPrompt(t *testing.T) {
	h := newHarness(t, defaultRemote)
	h.env.Interactive = true
	h.askTokenErr = ErrCanceled

	if err := Setup(context.Background(), h.env, nil); err != ErrCanceled {
		t.Errorf("Setup = %v, want ErrCanceled", err)
	}
	if config.Exists(h.root) {
		t.Error("canceling should not write a config")
	}
}

func TestSetupEmptyTokenIsCanceled(t *testing.T) {
	h := newHarness(t, defaultRemote)
	h.env.Interactive = true
	h.askToken = ""

	if err := Setup(context.Background(), h.env, nil); err != ErrCanceled {
		t.Errorf("Setup with an empty token = %v, want ErrCanceled", err)
	}
	if config.Exists(h.root) {
		t.Error("an empty token should not be written")
	}
}

func TestSetupNoTokenAnywhere(t *testing.T) {
	h := newHarness(t, defaultRemote)
	h.env.Stdin = bytes.NewReader(nil) // non-interactive, nothing piped

	err := Setup(context.Background(), h.env, nil)
	requireErrorContains(t, err, "--token")
}

func TestSetupOutsideAGitRepo(t *testing.T) {
	h := newHarness(t, defaultRemote)
	h.env.Dir = os.TempDir()

	if err := Setup(context.Background(), h.env, []string{"--token", "t"}); err == nil {
		t.Error("Setup outside a repo should fail")
	}
}

func TestSetupWithoutOrigin(t *testing.T) {
	h := newHarness(t, "")
	err := Setup(context.Background(), h.env, []string{"--token", "t"})
	requireErrorContains(t, err, "origin")
}

func TestSetupClientConstructionFails(t *testing.T) {
	h := newHarness(t, defaultRemote)
	h.newClientErr = errBoom

	requireErrorContains(t, Setup(context.Background(), h.env, []string{"--token", "t"}), "boom")
	if config.Exists(h.root) {
		t.Error("no config should be written when the client cannot be built")
	}
}

func TestSetupRejectsUnknownFlag(t *testing.T) {
	h := newHarness(t, defaultRemote)
	if err := Setup(context.Background(), h.env, []string{"--nope"}); err == nil {
		t.Error("an unknown flag should be an error")
	}
}
