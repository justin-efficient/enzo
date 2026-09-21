package version

import (
	"strconv"
	"strings"
	"testing"
)

// The version shipped in source is a plain MAJOR.MINOR.PATCH. A build from a
// tag may append git's description, which is why only the released value is
// checked here.
func TestVersionIsSemver(t *testing.T) {
	parts := strings.Split(Version, ".")
	if len(parts) != 3 {
		t.Fatalf("Version = %q, want MAJOR.MINOR.PATCH", Version)
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			t.Errorf("Version = %q: %q is not a number", Version, p)
		}
	}
}

func TestBanner(t *testing.T) {
	got := Banner()
	if !strings.HasPrefix(got, "🚘 enzo v") {
		t.Errorf("Banner() = %q, want it to start with the car and the name", got)
	}
	if !strings.HasSuffix(got, Version) {
		t.Errorf("Banner() = %q, want it to end with %q", got, Version)
	}
	// The "v" belongs to the banner, not to Version, or a tag override would
	// double it up.
	if strings.HasPrefix(Version, "v") {
		t.Errorf("Version = %q, want no leading v", Version)
	}
}

// Credit is what enzo leaves on things it made, so it has to name both the
// tool and the version — the point is answering "what put this here?". It
// credits the person, not the tool: enzo did the typing, somebody asked for it.
func TestCredit(t *testing.T) {
	got := Credit()
	if !strings.Contains(got, Banner()) {
		t.Errorf("Credit() = %q, want it to contain the banner %q", got, Banner())
	}
	if !strings.HasPrefix(got, "Created by a human with ") {
		t.Errorf("Credit() = %q, want it to credit the person who asked", got)
	}
}
