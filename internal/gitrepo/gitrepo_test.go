package gitrepo

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseRemote(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   Slug
	}{
		{"scp ssh", "git@github.com:justin-efficient/enzo.git", Slug{"justin-efficient", "enzo"}},
		{"scp ssh no suffix", "git@github.com:justin-efficient/enzo", Slug{"justin-efficient", "enzo"}},
		{"ssh scheme", "ssh://git@github.com/justin-efficient/enzo.git", Slug{"justin-efficient", "enzo"}},
		{"ssh scheme with port", "ssh://git@github.com:22/justin-efficient/enzo.git", Slug{"justin-efficient", "enzo"}},
		{"https", "https://github.com/justin-efficient/enzo.git", Slug{"justin-efficient", "enzo"}},
		{"https no suffix", "https://github.com/justin-efficient/enzo", Slug{"justin-efficient", "enzo"}},
		{"https trailing slash", "https://github.com/justin-efficient/enzo/", Slug{"justin-efficient", "enzo"}},
		{"https with credentials", "https://user:ghp_secret@github.com/justin-efficient/enzo.git", Slug{"justin-efficient", "enzo"}},
		{"git scheme", "git://github.com/justin-efficient/enzo.git", Slug{"justin-efficient", "enzo"}},
		{"surrounding whitespace", "  git@github.com:justin-efficient/enzo.git\n", Slug{"justin-efficient", "enzo"}},
		{"enterprise nested path", "https://ghe.internal/scm/justin-efficient/enzo.git", Slug{"justin-efficient", "enzo"}},
		{"repo name containing dot", "git@github.com:acme/enzo.tools.git", Slug{"acme", "enzo.tools"}},
		{"dashes and dots in owner", "git@github.com:a.b-c/d_e.git", Slug{"a.b-c", "d_e"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRemote(tt.remote)
			if err != nil {
				t.Fatalf("ParseRemote(%q) returned error: %v", tt.remote, err)
			}
			if got != tt.want {
				t.Errorf("ParseRemote(%q) = %v, want %v", tt.remote, got, tt.want)
			}
		})
	}
}

func TestParseRemoteErrors(t *testing.T) {
	for _, remote := range []string{"", "   ", "github.com", "https://github.com/", "https://github.com/onlyowner", "git@github.com:"} {
		t.Run(remote, func(t *testing.T) {
			if got, err := ParseRemote(remote); err == nil {
				t.Errorf("ParseRemote(%q) = %v, want an error", remote, got)
			}
		})
	}
}

func TestSlugString(t *testing.T) {
	if got := (Slug{"a", "b"}).String(); got != "a/b" {
		t.Errorf("String() = %q, want %q", got, "a/b")
	}
	if !(Slug{}).IsZero() {
		t.Error("empty Slug should report IsZero")
	}
	if (Slug{"a", "b"}).IsZero() {
		t.Error("populated Slug should not report IsZero")
	}
}

// initRepo builds a throwaway git repo so the exec-backed helpers get real
// coverage rather than a stub.
func initRepo(t *testing.T, remote string) string {
	t.Helper()
	dir := t.TempDir()
	// macOS temp dirs are symlinks; resolve so paths compare equal.
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if remote != "" {
		cmd := exec.Command("git", "remote", "add", "origin", remote)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git remote add: %v\n%s", err, out)
		}
	}
	return dir
}

func TestRoot(t *testing.T) {
	dir := initRepo(t, "")
	sub := filepath.Join(dir, "a", "b")
	if err := exec.Command("mkdir", "-p", sub).Run(); err != nil {
		t.Fatal(err)
	}

	for _, start := range []string{dir, sub} {
		got, err := Root(start)
		if err != nil {
			t.Fatalf("Root(%q): %v", start, err)
		}
		if got != dir {
			t.Errorf("Root(%q) = %q, want %q", start, got, dir)
		}
	}
}

func TestRootOutsideRepo(t *testing.T) {
	// t.TempDir is not a git repo, but it could sit under one on a dev box;
	// only assert the error when git agrees there is no repo.
	dir := t.TempDir()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		t.Skip("temp dir is inside a git repo on this machine")
	}
	if _, err := Root(dir); err != ErrNotARepo {
		t.Errorf("Root outside a repo = %v, want ErrNotARepo", err)
	}
}

func TestOriginSlug(t *testing.T) {
	dir := initRepo(t, "git@github.com:justin-efficient/enzo.git")
	got, err := OriginSlug(dir)
	if err != nil {
		t.Fatalf("OriginSlug: %v", err)
	}
	if want := (Slug{"justin-efficient", "enzo"}); got != want {
		t.Errorf("OriginSlug = %v, want %v", got, want)
	}
}

func TestOriginSlugNoRemote(t *testing.T) {
	dir := initRepo(t, "")
	if _, err := OriginSlug(dir); err == nil {
		t.Error("OriginSlug with no origin should fail")
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := initRepo(t, "")
	got, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if got != "main" {
		t.Errorf("CurrentBranch = %q, want %q", got, "main")
	}
}
