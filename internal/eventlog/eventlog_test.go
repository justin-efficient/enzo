package eventlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var when = time.Date(2026, 9, 18, 1, 23, 45, 0, time.UTC)

func TestEntryString(t *testing.T) {
	tests := []struct {
		name  string
		entry Entry
		want  string
	}{
		{
			name:  "created with a URL",
			entry: Entry{Time: when, Repo: "justin-efficient/enzo", Action: "created", Text: "#11 bork2", URL: "https://github.com/justin-efficient/enzo/issues/11"},
			want:  "2026-09-18T01:23:45Z justin-efficient/enzo created #11 bork2 https://github.com/justin-efficient/enzo/issues/11",
		},
		{
			name:  "no URL",
			entry: Entry{Time: when, Repo: "o/r", Action: "linked", Text: "#5 under #1"},
			want:  "2026-09-18T01:23:45Z o/r linked #5 under #1",
		},
		{
			name:  "no detail at all",
			entry: Entry{Time: when, Repo: "o/r", Action: "created"},
			want:  "2026-09-18T01:23:45Z o/r created",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entry.String(); got != tt.want {
				t.Errorf("String() =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

// Timestamps are normalised to UTC so lines sort and compare regardless of the
// machine's zone.
func TestEntryStringUsesUTC(t *testing.T) {
	zone := time.FixedZone("UTC+9", 9*60*60)
	e := Entry{Time: when.In(zone), Repo: "o/r", Action: "created"}
	if !strings.HasPrefix(e.String(), "2026-09-18T01:23:45Z") {
		t.Errorf("String() = %q, want the UTC instant", e.String())
	}
}

func TestAppendCreatesFileAndDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "enzo", FileName)

	if err := Append(path, Entry{Time: when, Repo: "o/r", Action: "created", Text: "#1 first"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	if !strings.Contains(string(b), "#1 first") {
		t.Errorf("log = %q", b)
	}
	if !strings.HasSuffix(string(b), "\n") {
		t.Errorf("each entry should end with a newline, got %q", b)
	}
}

func TestAppendAddsToExistingLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	for i, text := range []string{"#1 first", "#2 second", "#3 third"} {
		if err := Append(path, Entry{Time: when.Add(time.Duration(i) * time.Minute), Repo: "o/r", Action: "created", Text: text}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), b)
	}
	// Appending must not truncate what came before.
	for i, want := range []string{"#1 first", "#2 second", "#3 third"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want %q", i, lines[i], want)
		}
	}
}

// The log names private repositories, so it must not be world readable.
func TestAppendPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state", FileName)
	if err := Append(path, Entry{Time: when, Repo: "o/r", Action: "created"}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("log mode = %o, want 600", perm)
	}
	di, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("log directory mode = %o, want 700", perm)
	}
}

func TestAppendUnwritable(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocked")
	// A regular file where a directory needs to be.
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := Append(filepath.Join(blocker, FileName), Entry{Time: when, Action: "created"})
	if err == nil {
		t.Fatal("writing under a file should fail")
	}
	if !strings.Contains(err.Error(), "log") {
		t.Errorf("error %q should mention the log", err)
	}
}

func TestDefaultPath(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	tests := []struct {
		name       string
		configured string
		vars       map[string]string
		want       string
	}{
		{
			name:       "config wins",
			configured: "/custom/enzo.log",
			vars:       map[string]string{"ENZO_LOG": "/env/enzo.log", "HOME": "/home/j"},
			want:       "/custom/enzo.log",
		},
		{
			name: "env next",
			vars: map[string]string{"ENZO_LOG": "/env/enzo.log", "XDG_STATE_HOME": "/xdg", "HOME": "/home/j"},
			want: "/env/enzo.log",
		},
		{
			name: "xdg state home",
			vars: map[string]string{"XDG_STATE_HOME": "/xdg", "HOME": "/home/j"},
			want: "/xdg/enzo/enzo.log",
		},
		{
			name: "home fallback",
			vars: map[string]string{"HOME": "/home/j"},
			want: "/home/j/.local/state/enzo/enzo.log",
		},
		{
			name:       "whitespace is ignored",
			configured: "   ",
			vars:       map[string]string{"ENZO_LOG": "  ", "HOME": "/home/j"},
			want:       "/home/j/.local/state/enzo/enzo.log",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultPath(tt.configured, env(tt.vars))
			if err != nil {
				t.Fatalf("DefaultPath: %v", err)
			}
			if got != tt.want {
				t.Errorf("DefaultPath = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultPathWithNothingSet(t *testing.T) {
	_, err := DefaultPath("", func(string) string { return "" })
	if err == nil {
		t.Fatal("with no HOME there is nowhere to log; expected an error")
	}
}

// Round trip: what Append writes is what Entry.String produced.
func TestAppendWritesEntryString(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	e := Entry{Time: when, Repo: "justin-efficient/enzo", Action: "created", Text: "#11 bork2", URL: "https://github.com/justin-efficient/enzo/issues/11"}
	if err := Append(path, e); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if got := strings.TrimRight(string(b), "\n"); got != e.String() {
		t.Errorf("wrote %q, want %q", got, e.String())
	}
}
