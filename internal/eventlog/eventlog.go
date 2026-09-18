// Package eventlog appends a line to a log file for each thing enzo creates,
// so there is a persistent record of work that stdout no longer carries.
package eventlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileName is the log file enzo writes inside its state directory.
const FileName = "enzo.log"

// Entry is one logged action.
type Entry struct {
	Time   time.Time
	Repo   string
	Action string
	Text   string
	URL    string
}

// String renders the entry as it appears in the file: an RFC 3339 timestamp,
// the repo, the action, and whatever detail the action carried.
func (e Entry) String() string {
	fields := []string{
		e.Time.UTC().Format(time.RFC3339),
		e.Repo,
		e.Action,
	}
	if e.Text != "" {
		fields = append(fields, e.Text)
	}
	if e.URL != "" {
		fields = append(fields, e.URL)
	}
	return strings.Join(fields, " ")
}

// DefaultPath is where enzo logs when nothing says otherwise:
// $ENZO_LOG, else $XDG_STATE_HOME/enzo/enzo.log, else ~/.local/state/enzo/enzo.log.
//
// configured is the path from the repo's .enzo file, and wins when set.
func DefaultPath(configured string, getenv func(string) string) (string, error) {
	if p := strings.TrimSpace(configured); p != "" {
		return p, nil
	}
	if p := strings.TrimSpace(getenv("ENZO_LOG")); p != "" {
		return p, nil
	}
	if dir := strings.TrimSpace(getenv("XDG_STATE_HOME")); dir != "" {
		return filepath.Join(dir, "enzo", FileName), nil
	}
	home := strings.TrimSpace(getenv("HOME"))
	if home == "" {
		return "", fmt.Errorf("cannot find a log location: neither $ENZO_LOG, $XDG_STATE_HOME nor $HOME is set")
	}
	return filepath.Join(home, ".local", "state", "enzo", FileName), nil
}

// Append writes one entry to the log at path, creating the directory and file
// if they are not there yet. The file is owner-only: it names private repos.
func Append(path string, e Entry) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("could not create the log directory %s: %w", dir, err)
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("could not open the log %s: %w", path, err)
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, e.String()); err != nil {
		return fmt.Errorf("could not write to the log %s: %w", path, err)
	}
	return nil
}
