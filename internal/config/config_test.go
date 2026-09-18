package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	want := &Config{
		Token:            "ghp_example",
		Host:             "https://ghe.internal/api/v3/",
		DefaultReviewers: []string{"alice", "bob"},
	}
	if err := Save(root, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Token != want.Token || got.Host != want.Host {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
	if strings.Join(got.DefaultReviewers, ",") != "alice,bob" {
		t.Errorf("DefaultReviewers = %v, want [alice bob]", got.DefaultReviewers)
	}
}

// The file holds a token, so it must not be group or world readable.
func TestSavePermissions(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, &Config{Token: "secret"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

func TestLoadMissing(t *testing.T) {
	if _, err := Load(t.TempDir()); !errors.Is(err, ErrNotFound) {
		t.Errorf("Load on empty dir = %v, want ErrNotFound", err)
	}
}

func TestLoadMalformed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(Path(root), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(root)
	if err == nil {
		t.Fatal("Load on malformed JSON should fail")
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("malformed JSON should not report ErrNotFound")
	}
	if !strings.Contains(err.Error(), FileName) {
		t.Errorf("error %q should name %s", err, FileName)
	}
}

func TestExists(t *testing.T) {
	root := t.TempDir()
	if Exists(root) {
		t.Error("Exists should be false before Save")
	}
	if err := Save(root, &Config{Token: "x"}); err != nil {
		t.Fatal(err)
	}
	if !Exists(root) {
		t.Error("Exists should be true after Save")
	}
}

func TestResolveToken(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	tests := []struct {
		name string
		cfg  *Config
		vars map[string]string
		want string
	}{
		{"config wins", &Config{Token: "from-config"}, map[string]string{"ENZO_TOKEN": "env", "GITHUB_TOKEN": "gh"}, "from-config"},
		{"enzo token next", &Config{}, map[string]string{"ENZO_TOKEN": "env", "GITHUB_TOKEN": "gh"}, "env"},
		{"github token last", &Config{}, map[string]string{"GITHUB_TOKEN": "gh"}, "gh"},
		{"nil config falls through", nil, map[string]string{"GITHUB_TOKEN": "gh"}, "gh"},
		{"nothing anywhere", nil, nil, ""},
		{"whitespace config is empty", &Config{Token: "   "}, map[string]string{"GITHUB_TOKEN": "gh"}, "gh"},
		{"whitespace is trimmed", &Config{Token: "  tok\n"}, nil, "tok"},
		{"whitespace env is skipped", nil, map[string]string{"ENZO_TOKEN": "  ", "GITHUB_TOKEN": "gh"}, "gh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveToken(tt.cfg, env(tt.vars)); got != tt.want {
				t.Errorf("ResolveToken = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnsureIgnored(t *testing.T) {
	tests := []struct {
		name      string
		existing  string
		wantAdded bool
	}{
		{"no gitignore at all", "", true},
		{"gitignore without enzo", "*.out\nvendor/\n", true},
		{"missing trailing newline", "*.out", true},
		{"already listed", "*.out\n.enzo\n", false},
		{"listed as rooted path", "/.enzo\n", false},
		{"listed with stray whitespace", "  .enzo  \n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".gitignore")
			if tt.existing != "" {
				if err := os.WriteFile(path, []byte(tt.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			added, err := EnsureIgnored(root)
			if err != nil {
				t.Fatalf("EnsureIgnored: %v", err)
			}
			if added != tt.wantAdded {
				t.Errorf("added = %v, want %v", added, tt.wantAdded)
			}

			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading .gitignore: %v", err)
			}
			if !ignores(string(b)) {
				t.Errorf(".gitignore does not ignore %s:\n%s", FileName, b)
			}
			// Anything that was already there must survive.
			for _, line := range strings.Split(tt.existing, "\n") {
				if line = strings.TrimSpace(line); line != "" && !strings.Contains(string(b), line) {
					t.Errorf("EnsureIgnored dropped existing line %q", line)
				}
			}
		})
	}
}

// TestEnsureIgnoredIsIdempotent guards against appending the entry twice.
func TestEnsureIgnoredIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := EnsureIgnored(root); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	added, err := EnsureIgnored(root)
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Error("second EnsureIgnored should report no change")
	}
	second, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if string(first) != string(second) {
		t.Errorf("second call rewrote the file:\n%s\n---\n%s", first, second)
	}
	if n := strings.Count(string(second), FileName); n != 1 {
		t.Errorf("%s appears %d times, want 1", FileName, n)
	}
}

func ignores(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if s := strings.TrimSpace(line); s == FileName || s == "/"+FileName {
			return true
		}
	}
	return false
}
