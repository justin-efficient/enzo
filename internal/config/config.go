// Package config reads and writes the per-repo .enzo file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileName is the config file enzo keeps at the repository root.
const FileName = ".enzo"

// ErrNotFound means the repo has no .enzo file yet; run `enzo setup`.
var ErrNotFound = errors.New("no .enzo file found")

// Config is the contents of a .enzo file.
type Config struct {
	// Token is a GitHub personal access token with `repo` scope.
	Token string `json:"token"`
	// Host is the API host for GitHub Enterprise. Empty means github.com.
	Host string `json:"host,omitempty"`
	// DefaultReviewers are attached by `enzo review`.
	DefaultReviewers []string `json:"default_reviewers,omitempty"`
	// Log is where enzo records what it creates. Empty uses the default
	// state directory.
	Log string `json:"log,omitempty"`
}

// Path returns the .enzo path for a repository root.
func Path(root string) string { return filepath.Join(root, FileName) }

// Load reads the .enzo file at the repository root.
func Load(root string) (*Config, error) {
	b, err := os.ReadFile(Path(root))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", FileName, err)
	}
	return &c, nil
}

// Save writes the .enzo file with owner-only permissions; it holds a token.
func Save(root string, c *Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(Path(root), b, 0o600)
}

// Exists reports whether the repo already has a .enzo file.
func Exists(root string) bool {
	_, err := os.Stat(Path(root))
	return err == nil
}

// ResolveToken returns the first token available from the config file,
// $ENZO_TOKEN, or $GITHUB_TOKEN. c may be nil.
func ResolveToken(c *Config, getenv func(string) string) string {
	if c != nil && strings.TrimSpace(c.Token) != "" {
		return strings.TrimSpace(c.Token)
	}
	for _, k := range []string{"ENZO_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// EnsureIgnored appends .enzo to the repo's .gitignore if it is not already
// covered. It reports whether the file was modified.
func EnsureIgnored(root string) (bool, error) {
	path := filepath.Join(root, ".gitignore")
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		switch strings.TrimSpace(line) {
		case FileName, "/" + FileName:
			return false, nil
		}
	}
	body := string(b)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += "\n# enzo credentials\n" + FileName + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
