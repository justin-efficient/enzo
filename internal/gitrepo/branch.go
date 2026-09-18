package gitrepo

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// BranchExists reports whether a local branch of that name exists.
func BranchExists(dir, branch string) bool {
	return RefExists(dir, "refs/heads/"+branch)
}

// RefExists reports whether ref resolves in the repository.
func RefExists(dir, ref string) bool {
	_, err := run(dir, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}

// CreateBranch creates branch at startPoint and checks it out. An empty
// startPoint means HEAD, the same as `git switch -c` on its own.
//
// Uncommitted work comes along, as it always does with `git switch`; when it
// cannot, git refuses and says so rather than discarding anything.
func CreateBranch(dir, branch, startPoint string) error {
	args := []string{"switch", "-c", branch}
	if startPoint != "" {
		args = append(args, startPoint)
	}
	return do(dir, args...)
}

// Rev resolves a ref to the commit it points at.
func Rev(dir, ref string) (string, error) {
	out, err := run(dir, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("cannot resolve %s", ref)
	}
	return out, nil
}

// Switch checks out an existing branch.
func Switch(dir, branch string) error {
	return do(dir, "switch", branch)
}

// CommitEmpty records a commit that changes nothing, so a branch has something
// to open a pull request with.
func CommitEmpty(dir, message string) error {
	return do(dir, "commit", "--allow-empty", "-m", message)
}

// Push sends branch to remote and sets it as the upstream.
func Push(dir, remote, branch string) error {
	return do(dir, "push", "-u", remote, branch)
}

// FetchBranch reads a branch from remote and returns the commit it points at.
// It touches neither the worktree nor any local branch — only FETCH_HEAD.
//
// Anything that compares against a base branch has to go through here. A
// refs/remotes/<remote>/<branch> on disk is only as fresh as the last fetch,
// and a stale one makes a branch look ahead of a base that already contains it.
func FetchBranch(dir, remote, branch string) (string, error) {
	if err := do(dir, "fetch", "--quiet", remote, branch); err != nil {
		return "", err
	}
	out, err := run(dir, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return "", fmt.Errorf("fetched %s from %s but could not read FETCH_HEAD", branch, remote)
	}
	return out, nil
}

// CommitsAhead counts the commits on head that base does not have.
func CommitsAhead(dir, base, head string) (int, error) {
	out, err := run(dir, "rev-list", "--count", base+".."+head)
	if err != nil {
		return 0, fmt.Errorf("cannot compare %s with %s", head, base)
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("unexpected rev-list output %q", out)
	}
	return n, nil
}

// do runs a git command for its effect, reporting what git said when it fails.
// Unlike run it keeps stderr, which is where git explains itself.
func do(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
}

// DeleteBranch removes a local branch, whether or not it was merged. The
// caller must not be standing on it.
func DeleteBranch(dir, branch string) error {
	return do(dir, "branch", "-D", branch)
}

// DeleteRemoteBranch removes a branch from remote.
func DeleteRemoteBranch(dir, remote, branch string) error {
	return do(dir, "push", remote, "--delete", branch)
}

// RemoteBranchExists asks the remote directly, rather than trusting a
// remote-tracking ref that is only as fresh as the last fetch.
func RemoteBranchExists(dir, remote, branch string) bool {
	out, err := run(dir, "ls-remote", "--heads", remote, "refs/heads/"+branch)
	return err == nil && strings.TrimSpace(out) != ""
}

// Unpushed counts the commits on branch that remote does not have.
//
// When the remote knows the branch, that is everything past its remote tip,
// freshly fetched. When it does not, the branch has never been pushed at all,
// and everything past base is at risk — which is why base has to be given.
func Unpushed(dir, remote, branch, base string) (int, error) {
	if !RemoteBranchExists(dir, remote, branch) {
		return CommitsAhead(dir, base, branch)
	}
	rev, err := FetchBranch(dir, remote, branch)
	if err != nil {
		return 0, err
	}
	return CommitsAhead(dir, rev, branch)
}

// DirtyFiles lists the paths with uncommitted changes, staged or not,
// including files git does not track.
func DirtyFiles(dir string) ([]string, error) {
	// Untrimmed: `git status --porcelain` puts two status columns before each
	// path, and the first is a space for a change that is not staged. Trimming
	// the output would eat it and take a character off the first path.
	out, err := output(dir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 3 {
			paths = append(paths, line[3:])
		}
	}
	return paths, nil
}

// output runs git and returns stdout with only the trailing newline removed.
// Anything parsing fixed columns needs this rather than run.
func output(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}
