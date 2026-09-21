package browser

import (
	"runtime"
	"testing"
)

const url = "https://github.com/justin-efficient/enzo/issues/12"

// Every platform enzo builds for gets an opener, and the URL reaches it
// unmangled and last, where an opener expects it.
func TestCommand(t *testing.T) {
	tests := []struct {
		goos string
		want string
	}{
		{"darwin", "open"},
		{"windows", "rundll32"},
		{"linux", "xdg-open"},
		{"freebsd", "xdg-open"},
		{"openbsd", "xdg-open"},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			name, args := command(tt.goos, url)
			if name != tt.want {
				t.Errorf("command(%q) = %q, want %q", tt.goos, name, tt.want)
			}
			if len(args) == 0 || args[len(args)-1] != url {
				t.Errorf("args = %v, want the URL last", args)
			}
		})
	}
}

// An unknown platform names itself rather than silently doing nothing, so
// `enzo list` falls back to printing the URL.
func TestCommandUnknownPlatform(t *testing.T) {
	if name, _ := command("plan9", url); name != "" {
		t.Errorf("command(plan9) = %q, want no opener", name)
	}
}

// Whatever is running the suite must itself be openable, or `enzo list` would
// only ever print URLs here.
func TestThisPlatformHasAnOpener(t *testing.T) {
	if name, _ := command(runtime.GOOS, url); name == "" {
		t.Fatalf("no opener for the platform running the suite, %s", runtime.GOOS)
	}
}
