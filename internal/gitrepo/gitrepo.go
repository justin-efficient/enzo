// Package gitrepo locates the enclosing git repository and identifies the
// GitHub repo it points at.
package gitrepo

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotARepo is returned when no enclosing git repository exists.
var ErrNotARepo = errors.New("not inside a git repository")

// Slug is a GitHub owner/repo pair.
type Slug struct {
	Owner string
	Name  string
}

func (s Slug) String() string { return s.Owner + "/" + s.Name }

// IsZero reports whether the slug is unset.
func (s Slug) IsZero() bool { return s.Owner == "" && s.Name == "" }

// Root returns the top level of the git worktree containing dir.
func Root(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotARepo
	}
	return filepath.Clean(out), nil
}

// OriginSlug returns the GitHub repo that origin points at.
func OriginSlug(dir string) (Slug, error) {
	out, err := run(dir, "config", "--get", "remote.origin.url")
	if err != nil {
		return Slug{}, errors.New("no 'origin' remote configured")
	}
	return ParseRemote(out)
}

// CurrentBranch returns the checked out branch name.
func CurrentBranch(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", ErrNotARepo
	}
	return out, nil
}

// ParseRemote extracts the owner/repo from a git remote URL. It accepts the
// scp-like SSH form, ssh://, https:// and git:// URLs, with or without a
// trailing ".git" or embedded credentials.
func ParseRemote(remote string) (Slug, error) {
	raw := strings.TrimSpace(remote)
	if raw == "" {
		return Slug{}, errors.New("empty remote URL")
	}

	path := raw
	switch {
	case strings.Contains(raw, "://"):
		// scheme://[user[:pass]@]host[:port]/owner/repo
		rest := raw[strings.Index(raw, "://")+3:]
		if i := strings.LastIndex(rest, "@"); i >= 0 {
			rest = rest[i+1:]
		}
		i := strings.Index(rest, "/")
		if i < 0 {
			return Slug{}, fmt.Errorf("remote %q has no path", raw)
		}
		path = rest[i+1:]
	case strings.Contains(raw, ":"):
		// [user@]host:owner/repo
		path = raw[strings.LastIndex(raw, ":")+1:]
	}

	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return Slug{}, fmt.Errorf("cannot find owner/repo in remote %q", raw)
	}
	// Keep the last two segments so nested GHE paths still resolve.
	owner, name := parts[len(parts)-2], parts[len(parts)-1]
	if owner == "" || name == "" {
		return Slug{}, fmt.Errorf("cannot find owner/repo in remote %q", raw)
	}
	return Slug{Owner: owner, Name: name}, nil
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
