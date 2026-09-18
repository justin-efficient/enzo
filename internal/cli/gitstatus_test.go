package cli

import (
	"os/exec"
	"testing"
)

// gitStatus returns `git status --porcelain` for a repo, used to assert that
// enzo does not leave secrets visible to git.
func gitStatus(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	return string(out)
}
