package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitOut runs git in dir and returns its trimmed output.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestBranchExists(t *testing.T) {
	dir := initRepo(t, "")

	if BranchExists(dir, "nope") {
		t.Error("a branch that was never made should not exist")
	}
	if !BranchExists(dir, "main") {
		t.Error("main should exist")
	}
	// A tag of the same name is not a branch; a ref lookup that did not say
	// refs/heads would wrongly find it.
	gitOut(t, dir, "tag", "v1")
	if BranchExists(dir, "v1") {
		t.Error("a tag should not count as a branch")
	}
}

func TestCreateAndSwitchBranch(t *testing.T) {
	dir := initRepo(t, "")

	if err := CreateBranch(dir, "feature", ""); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if got := gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature" {
		t.Errorf("on branch %q, want feature", got)
	}

	if err := Switch(dir, "main"); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if got := gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("on branch %q, want main", got)
	}

	// Creating a branch twice is a mistake worth reporting, and git's own
	// message is the clearest thing to say.
	if err := CreateBranch(dir, "feature", ""); err == nil {
		t.Error("creating an existing branch should fail")
	} else if !strings.Contains(err.Error(), "feature") {
		t.Errorf("error %q should name the branch", err)
	}
}

// A branch can start somewhere other than HEAD, which is how `enzo start`
// cuts from the default branch rather than from whatever you were standing on.
func TestCreateBranchAtAStartPoint(t *testing.T) {
	dir := initRepo(t, "")
	base := gitOut(t, dir, "rev-parse", "HEAD")

	// Wander off onto some other work first.
	if err := CreateBranch(dir, "other", ""); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := CommitEmpty(dir, "unrelated work"); err != nil {
		t.Fatalf("CommitEmpty: %v", err)
	}

	if err := CreateBranch(dir, "feature", base); err != nil {
		t.Fatalf("CreateBranch at a start point: %v", err)
	}
	if got := gitOut(t, dir, "rev-parse", "HEAD"); got != base {
		t.Errorf("branch starts at %s, want the start point %s", got, base)
	}
	// The unrelated work must not have come along.
	n, err := CommitsAhead(dir, base, "HEAD")
	if err != nil {
		t.Fatalf("CommitsAhead: %v", err)
	}
	if n != 0 {
		t.Errorf("branch is %d commits past the start point, want 0", n)
	}
}

func TestCommitEmptyAndCommitsAhead(t *testing.T) {
	dir := initRepo(t, "")
	if err := CreateBranch(dir, "feature", ""); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	n, err := CommitsAhead(dir, "main", "HEAD")
	if err != nil {
		t.Fatalf("CommitsAhead: %v", err)
	}
	if n != 0 {
		t.Errorf("a fresh branch is %d commits ahead, want 0", n)
	}

	if err := CommitEmpty(dir, "start #12"); err != nil {
		t.Fatalf("CommitEmpty: %v", err)
	}
	n, err = CommitsAhead(dir, "main", "HEAD")
	if err != nil {
		t.Fatalf("CommitsAhead: %v", err)
	}
	if n != 1 {
		t.Errorf("after one commit the branch is %d ahead, want 1", n)
	}
	if got := gitOut(t, dir, "log", "-1", "--format=%s"); got != "start #12" {
		t.Errorf("commit subject = %q", got)
	}
}

// A base that does not resolve is an error, not a zero count — the caller has
// to be able to tell the difference.
func TestCommitsAheadUnknownBase(t *testing.T) {
	dir := initRepo(t, "")
	if _, err := CommitsAhead(dir, "no-such-branch", "HEAD"); err == nil {
		t.Fatal("expected an error for a base that does not exist")
	}
}

func TestRev(t *testing.T) {
	dir := initRepo(t, "")
	head := gitOut(t, dir, "rev-parse", "HEAD")

	for _, ref := range []string{"HEAD", "main", "refs/heads/main"} {
		got, err := Rev(dir, ref)
		if err != nil {
			t.Fatalf("Rev(%q): %v", ref, err)
		}
		if got != head {
			t.Errorf("Rev(%q) = %q, want %q", ref, got, head)
		}
	}
	if _, err := Rev(dir, "refs/heads/nope"); err == nil {
		t.Error("resolving a ref that does not exist should fail")
	}
}

func TestRefExists(t *testing.T) {
	dir := initRepo(t, "")
	for _, tt := range []struct {
		ref  string
		want bool
	}{
		{"refs/heads/main", true},
		{"refs/heads/nope", false},
		{"refs/remotes/origin/main", false},
		{"HEAD", true},
	} {
		if got := RefExists(dir, tt.ref); got != tt.want {
			t.Errorf("RefExists(%q) = %v, want %v", tt.ref, got, tt.want)
		}
	}
}

func TestPush(t *testing.T) {
	dir := initRepo(t, "")
	bare := filepath.Join(t.TempDir(), "origin.git")
	gitOut(t, "", "init", "-q", "--bare", bare)
	gitOut(t, dir, "remote", "add", "origin", bare)

	if err := CreateBranch(dir, "feature", ""); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := CommitEmpty(dir, "work"); err != nil {
		t.Fatalf("CommitEmpty: %v", err)
	}
	if err := Push(dir, "origin", "feature"); err != nil {
		t.Fatalf("Push: %v", err)
	}

	if got := gitOut(t, bare, "rev-parse", "--abbrev-ref", "feature"); got != "feature" {
		t.Errorf("origin has %q, want the pushed branch", got)
	}
	// -u is what lets `git push` on its own work afterwards.
	if got := gitOut(t, dir, "rev-parse", "--abbrev-ref", "feature@{upstream}"); got != "origin/feature" {
		t.Errorf("upstream = %q, want origin/feature", got)
	}

	// Pushing to a remote that is not there should say so rather than hang.
	if err := Push(dir, "nowhere", "feature"); err == nil {
		t.Error("pushing to an unknown remote should fail")
	}
}

func TestDeleteBranch(t *testing.T) {
	dir := initRepo(t, "")
	if err := CreateBranch(dir, "doomed", ""); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := CommitEmpty(dir, "unmerged work"); err != nil {
		t.Fatalf("CommitEmpty: %v", err)
	}

	// git refuses to delete the branch you are standing on.
	if err := DeleteBranch(dir, "doomed"); err == nil {
		t.Error("deleting the checked out branch should fail")
	}

	if err := Switch(dir, "main"); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	// -D, not -d: an aborted branch is unmerged by definition.
	if err := DeleteBranch(dir, "doomed"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if BranchExists(dir, "doomed") {
		t.Error("the branch should be gone")
	}
}

func TestRemoteBranchLifecycle(t *testing.T) {
	dir := initRepo(t, "")
	bare := filepath.Join(t.TempDir(), "origin.git")
	gitOut(t, "", "init", "-q", "--bare", bare)
	gitOut(t, dir, "remote", "add", "origin", bare)
	gitOut(t, dir, "push", "-q", "origin", "main")

	if RemoteBranchExists(dir, "origin", "feature") {
		t.Error("origin should not have a branch nobody pushed")
	}

	if err := CreateBranch(dir, "feature", ""); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := CommitEmpty(dir, "work"); err != nil {
		t.Fatalf("CommitEmpty: %v", err)
	}

	// Never pushed, so everything past main is at risk.
	n, err := Unpushed(dir, "origin", "feature", "main")
	if err != nil {
		t.Fatalf("Unpushed: %v", err)
	}
	if n != 1 {
		t.Errorf("Unpushed = %d on a branch origin has never seen, want 1", n)
	}

	if err := Push(dir, "origin", "feature"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if !RemoteBranchExists(dir, "origin", "feature") {
		t.Error("origin should have the branch after a push")
	}
	if n, err := Unpushed(dir, "origin", "feature", "main"); err != nil || n != 0 {
		t.Errorf("Unpushed = %d, %v after pushing; want 0", n, err)
	}

	// A commit made after the push is unpushed again.
	if err := CommitEmpty(dir, "more work"); err != nil {
		t.Fatalf("CommitEmpty: %v", err)
	}
	if n, err := Unpushed(dir, "origin", "feature", "main"); err != nil || n != 1 {
		t.Errorf("Unpushed = %d, %v after a new commit; want 1", n, err)
	}

	if err := DeleteRemoteBranch(dir, "origin", "feature"); err != nil {
		t.Fatalf("DeleteRemoteBranch: %v", err)
	}
	if RemoteBranchExists(dir, "origin", "feature") {
		t.Error("the branch should be gone from origin")
	}
}

func TestDirtyFiles(t *testing.T) {
	dir := initRepo(t, "")

	got, err := DirtyFiles(dir)
	if err != nil {
		t.Fatalf("DirtyFiles: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a fresh repo reports %v dirty", got)
	}

	// Untracked counts: it is work that a branch switch may carry or block.
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = DirtyFiles(dir)
	if err != nil {
		t.Fatalf("DirtyFiles: %v", err)
	}
	if len(got) != 1 || got[0] != "new.txt" {
		t.Errorf("DirtyFiles = %v, want [new.txt]", got)
	}

	// So does a staged change to a tracked file.
	gitOut(t, dir, "add", "new.txt")
	gitOut(t, dir, "commit", "-q", "-m", "add new.txt")
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = DirtyFiles(dir)
	if err != nil {
		t.Fatalf("DirtyFiles: %v", err)
	}
	if len(got) != 1 || got[0] != "new.txt" {
		t.Errorf("DirtyFiles = %v, want [new.txt]", got)
	}
}

// DirtyTrackedFiles is the same reading with the untracked files left out.
func TestDirtyTrackedFiles(t *testing.T) {
	dir := initRepo(t, "")

	// A file git was never told about is not a tracked change.
	if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := DirtyTrackedFiles(dir)
	if err != nil {
		t.Fatalf("DirtyTrackedFiles: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("DirtyTrackedFiles = %v, want nothing for an untracked file", got)
	}
	// DirtyFiles still sees it, which is what `enzo abort` warns about.
	if all, err := DirtyFiles(dir); err != nil || len(all) != 1 {
		t.Errorf("DirtyFiles = %v (err %v), want [scratch.txt]", all, err)
	}

	// Once git tracks it, an edit counts.
	gitOut(t, dir, "add", "scratch.txt")
	gitOut(t, dir, "commit", "-q", "-m", "add scratch.txt")
	if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = DirtyTrackedFiles(dir)
	if err != nil {
		t.Fatalf("DirtyTrackedFiles: %v", err)
	}
	if len(got) != 1 || got[0] != "scratch.txt" {
		t.Errorf("DirtyTrackedFiles = %v, want [scratch.txt]", got)
	}

	// Staged but uncommitted counts too.
	gitOut(t, dir, "add", "scratch.txt")
	got, err = DirtyTrackedFiles(dir)
	if err != nil {
		t.Fatalf("DirtyTrackedFiles: %v", err)
	}
	if len(got) != 1 || got[0] != "scratch.txt" {
		t.Errorf("staged change: DirtyTrackedFiles = %v, want [scratch.txt]", got)
	}
}
