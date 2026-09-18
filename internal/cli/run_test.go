package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-efficient/enzo/internal/config"
	"github.com/justin-efficient/enzo/internal/version"
)

func TestRunDefaultsToList(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues

	if err := Run(context.Background(), h.env, nil); err != nil {
		t.Fatalf("Run with no args: %v", err)
	}
	if !strings.Contains(h.out(), "#12") {
		t.Errorf("bare `enzo` should list issues:\n%s", h.out())
	}
}

func TestRunDispatchesSetup(t *testing.T) {
	h := newHarness(t, defaultRemote)
	if err := Run(context.Background(), h.env, []string{"setup", "--token", "ghp_x"}); err != nil {
		t.Fatalf("Run setup: %v", err)
	}
	if !config.Exists(h.root) {
		t.Error("`enzo setup` did not write a config")
	}
}

func TestRunDispatchesList(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})
	h.client.issues = listIssues

	if err := Run(context.Background(), h.env, []string{"list", "--plain"}); err != nil {
		t.Fatalf("Run list: %v", err)
	}
	if !strings.Contains(h.out(), "fix the thing") {
		t.Errorf("`enzo list` should print issues:\n%s", h.out())
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			h := newHarness(t, defaultRemote)
			if err := Run(context.Background(), h.env, []string{arg}); err != nil {
				t.Fatalf("Run %s: %v", arg, err)
			}
			for _, want := range []string{"enzo setup", "enzo list"} {
				if !strings.Contains(h.out(), want) {
					t.Errorf("help should mention %q:\n%s", want, h.out())
				}
			}
		})
	}
}

func TestRunUnknownCommand(t *testing.T) {
	h := newHarness(t, defaultRemote)
	err := Run(context.Background(), h.env, []string{"frobnicate"})
	requireErrorContains(t, err, "frobnicate")
	if !strings.Contains(h.stderr.String(), "usage:") {
		t.Errorf("an unknown command should print usage to stderr:\n%s", h.stderr.String())
	}
	if h.out() != "" {
		t.Errorf("usage for an error belongs on stderr, not stdout:\n%s", h.out())
	}
}

// The usage text leads with the banner, so `enzo help` says which enzo this is.
func TestUsageLeadsWithTheVersion(t *testing.T) {
	if !strings.HasPrefix(Usage(), version.Banner()) {
		t.Errorf("usage should start with %q:\n%s", version.Banner(), Usage())
	}
	if !strings.Contains(Usage(), version.Version) {
		t.Errorf("usage should name the version %q", version.Version)
	}
}

// Every command named in the usage text must actually dispatch.
func TestUsageMatchesDispatch(t *testing.T) {
	for _, cmd := range []string{"setup", "list", "new", "start", "abort", "help"} {
		if !strings.Contains(Usage(), "enzo "+cmd) {
			t.Errorf("usage text does not mention %q", cmd)
		}
		h := newHarness(t, defaultRemote)
		err := Run(context.Background(), h.env, []string{cmd, "--help-probe-unknown"})
		if err != nil && strings.Contains(err.Error(), "unknown command") {
			t.Errorf("usage advertises %q but Run does not dispatch it", cmd)
		}
	}
}

func TestRunDispatchesNew(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})

	if err := Run(context.Background(), h.env, []string{"new", "--title", "via dispatch"}); err != nil {
		t.Fatalf("Run new: %v", err)
	}
	if h.client.gotNewIssue.Title != "via dispatch" {
		t.Errorf("created %q", h.client.gotNewIssue.Title)
	}
}

func TestRunDispatchesNewSub(t *testing.T) {
	h := newHarness(t, defaultRemote)
	seedConfig(t, h, &config.Config{Token: "t"})

	if err := Run(context.Background(), h.env, []string{"new", "sub", "12", "--title", "a child"}); err != nil {
		t.Fatalf("Run new sub: %v", err)
	}
	if h.client.linkedParent != 12 {
		t.Errorf("linked under #%d, want #12", h.client.linkedParent)
	}
}

func TestUsageMentionsNew(t *testing.T) {
	for _, want := range []string{"enzo new", "sub"} {
		if !strings.Contains(Usage(), want) {
			t.Errorf("usage should mention %q:\n%s", want, Usage())
		}
	}
}

// The footer is appended, never substituted: what was written has to survive
// intact, and the signature has to be visibly separate from it. The same
// function signs issue bodies and pull request bodies.
func TestSignedBody(t *testing.T) {
	tests := []struct {
		name    string
		written string
		want    string
	}{
		{
			"an empty body is the footer alone",
			"",
			"*" + version.Credit() + "*",
		},
		{
			"a body keeps what was written, under a rule",
			"the parser crashes on empty input",
			"the parser crashes on empty input\n\n---\n\n*" + version.Credit() + "*",
		},
		{
			"surrounding whitespace is trimmed before signing",
			"  \n detail \n\n ",
			"detail\n\n---\n\n*" + version.Credit() + "*",
		},
		{
			"a whitespace-only body is an empty one",
			"   \n\t ",
			"*" + version.Credit() + "*",
		},
		{
			"a pull request body is signed the same way",
			"Closes #12",
			"Closes #12\n\n---\n\n*" + version.Credit() + "*",
		},
		{
			"markdown in the body is left alone",
			"### steps\n\n1. run it\n2. watch it fall over",
			"### steps\n\n1. run it\n2. watch it fall over\n\n---\n\n*" + version.Credit() + "*",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := signedBody(tt.written); got != tt.want {
				t.Errorf("signedBody(%q) =\n%q\nwant\n%q", tt.written, got, tt.want)
			}
		})
	}
}
