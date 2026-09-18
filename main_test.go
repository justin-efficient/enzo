package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-efficient/enzo/internal/version"
)

// These tests build the real binary and run it as a subprocess, so they cover
// argument handling, exit codes and the wiring in main that the package-level
// tests cannot reach.

var (
	enzoBin string
	// sandboxLog keeps every test run's logging inside the temp directory. A
	// test that forgot to set ENZO_LOG would otherwise append to the real
	// user's state file.
	sandboxLog string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "enzo-e2e")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	sandboxLog = filepath.Join(dir, "sandbox.log")
	enzoBin = filepath.Join(dir, "enzo")
	build := exec.Command("go", "build", "-o", enzoBin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		panic("building enzo: " + err.Error() + "\n" + string(out))
	}
	os.Exit(m.Run())
}

// run executes enzo in dir and returns stdout, stderr and the exit code.
func run(t *testing.T, dir string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(enzoBin, args...)
	cmd.Dir = dir
	// The sandbox default comes first so a caller's own ENZO_LOG overrides it;
	// os/exec keeps the last value for a repeated key.
	cmd.Env = append(append(os.Environ(), "ENZO_LOG="+sandboxLog), env...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()

	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running enzo %v: %v", args, err)
	}
	return stdout.String(), stderr.String(), code
}

// logFileFor points ENZO_LOG at a fresh file and returns the path plus the env
// entry to pass to run.
func logFileFor(t *testing.T) (string, []string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "enzo.log")
	return path, []string{"ENZO_LOG=" + path}
}

// readLog returns the log's lines, or nil when it was never written.
func readLog(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	trimmed := strings.TrimRight(string(b), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func testRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
		{"remote", "add", "origin", "git@github.com:justin-efficient/enzo.git"},
		// origin's URL has to look like GitHub, because that is what the slug
		// is parsed from. insteadOf rewrites it to a bare repo beside it at
		// transport time, so no fetch or push in a test can leave the machine.
		{"config", "url." + filepath.Join(dir, "origin.git") + ".insteadOf",
			"git@github.com:justin-efficient/enzo.git"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	bare := exec.Command("git", "init", "-q", "--bare", filepath.Join(dir, "origin.git"))
	if out, err := bare.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	push := exec.Command("git", "push", "-q", "origin", "main")
	push.Dir = dir
	if out, err := push.CombinedOutput(); err != nil {
		t.Fatalf("seeding origin/main: %v\n%s", err, out)
	}
	return dir
}

// fakeGitHub serves the endpoints enzo uses, at the enterprise path prefix
// go-github adds for a custom host.
// created, linked and pulls record the write requests the fake server received.
var created, linked, pulls, closedPulls []map[string]any

// openPR, when set, is what the pull request listing reports.
var openPR map[string]any

func fakeGitHub(t *testing.T, issues []map[string]any) *httptest.Server {
	t.Helper()
	created, linked, pulls, closedPulls = nil, nil, nil, nil
	openPR = nil
	t.Cleanup(func() { openPR = nil })
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/justin-efficient/enzo", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"name": "enzo", "default_branch": "main"})
	})
	mux.HandleFunc("/api/v3/repos/justin-efficient/enzo/pulls/77", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		closedPulls = append(closedPulls, in)
		json.NewEncoder(w).Encode(map[string]any{"number": 77, "state": in["state"]})
	})
	mux.HandleFunc("/api/v3/repos/justin-efficient/enzo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			pulls = append(pulls, in)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{
				"number": 300, "title": in["title"], "draft": in["draft"], "state": "open",
				"html_url": "https://github.com/justin-efficient/enzo/pull/300",
				"head":     map[string]any{"ref": in["head"]},
				"base":     map[string]any{"ref": in["base"]},
			})
			return
		}
		if openPR != nil {
			json.NewEncoder(w).Encode([]any{openPR})
			return
		}
		// No pull request open on any branch yet.
		json.NewEncoder(w).Encode([]any{})
	})
	mux.HandleFunc("/api/v3/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer bad" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"login": "justin-efficient"})
	})
	mux.HandleFunc("/api/v3/repos/justin-efficient/enzo/issues", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			created = append(created, in)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{
				"id": 987654, "number": 57, "title": in["title"],
				"html_url": "https://github.com/justin-efficient/enzo/issues/57",
			})
			return
		}
		json.NewEncoder(w).Encode(issues)
	})
	mux.HandleFunc("/api/v3/repos/justin-efficient/enzo/issues/12", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": 1200, "number": 12, "title": "the parent"})
	})
	mux.HandleFunc("/api/v3/repos/justin-efficient/enzo/issues/12/sub_issues", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		linked = append(linked, in)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 987654, "number": 57})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestVersionFlag(t *testing.T) {
	want := version.Banner()
	for _, arg := range []string{"--version", "version"} {
		stdout, _, code := run(t, t.TempDir(), nil, arg)
		if code != 0 {
			t.Errorf("%s exited %d, want 0", arg, code)
		}
		if got := strings.TrimSpace(stdout); got != want {
			t.Errorf("%s printed %q, want %q", arg, got, want)
		}
	}
}

// A tagged build names its tag. The Makefile sets this flag; if the symbol
// path ever drifts from the package, ldflags fails silently and every release
// binary claims to be whatever is in source.
func TestVersionIsSetByLdflags(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "enzo")
	build := exec.Command("go", "build",
		"-ldflags", "-X github.com/justin-efficient/enzo/internal/version.Version=1.2.3",
		"-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(out)), "🚘 enzo v1.2.3"; got != want {
		t.Errorf("version = %q, want %q", got, want)
	}
}

// The Makefile is what ships releases, so the flag it builds has to be the one
// that lands. A typo there is invisible until someone reads a version string.
func TestMakefileOverridesTheVersion(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "enzo")
	build := exec.Command("make", "build", "BIN="+bin, "VERSION=9.9.9")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("make build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(out)), "🚘 enzo v9.9.9"; got != want {
		t.Errorf("version = %q, want %q", got, want)
	}
}

func TestHelpExitsZero(t *testing.T) {
	stdout, _, code := run(t, t.TempDir(), nil, "help")
	if code != 0 {
		t.Errorf("help exited %d, want 0", code)
	}
	if !strings.Contains(stdout, "enzo setup") {
		t.Errorf("help output:\n%s", stdout)
	}
}

func TestUnknownCommandExitsNonZero(t *testing.T) {
	_, stderr, code := run(t, t.TempDir(), nil, "frobnicate")
	if code == 0 {
		t.Error("an unknown command should exit non-zero")
	}
	if !strings.Contains(stderr, "frobnicate") {
		t.Errorf("stderr should name the bad command:\n%s", stderr)
	}
}

func TestOutsideGitRepoExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Run(); err == nil {
		t.Skip("temp dir is inside a git repo on this machine")
	}
	_, stderr, code := run(t, dir, nil, "list", "--plain")
	if code == 0 {
		t.Error("running outside a repo should exit non-zero")
	}
	if !strings.Contains(stderr, "enzo:") {
		t.Errorf("errors should be prefixed for the user:\n%s", stderr)
	}
}

// The full path a new user walks: setup, then list.
func TestSetupThenList(t *testing.T) {
	srv := fakeGitHub(t, []map[string]any{
		{"number": 12, "title": "fix the thing", "state": "open"},
		{"number": 13, "title": "a pull request", "state": "open",
			"pull_request": map[string]any{"url": "x"}},
	})
	dir := testRepo(t)

	stdout, stderr, code := run(t, dir, nil, "setup", "--token", "ghp_good", "--host", srv.URL+"/")
	if code != 0 {
		t.Fatalf("setup exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "authenticated as justin-efficient") {
		t.Errorf("setup output:\n%s", stdout)
	}

	// The token file must exist, be private, and be ignored by git.
	info, err := os.Stat(filepath.Join(dir, ".enzo"))
	if err != nil {
		t.Fatalf(".enzo was not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf(".enzo mode = %o, want 600", perm)
	}
	status, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(status), ".enzo") {
		t.Errorf("git can still see .enzo:\n%s", status)
	}

	stdout, stderr, code = run(t, dir, nil, "list", "--plain")
	if code != 0 {
		t.Fatalf("list exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "#12") || !strings.Contains(stdout, "fix the thing") {
		t.Errorf("list output:\n%s", stdout)
	}
	if strings.Contains(stdout, "a pull request") {
		t.Errorf("list should not show pull requests:\n%s", stdout)
	}
}

func TestSetupRejectsBadCredentials(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)

	_, stderr, code := run(t, dir, nil, "setup", "--token", "bad", "--host", srv.URL+"/")
	if code == 0 {
		t.Error("setup with a rejected token should exit non-zero")
	}
	if !strings.Contains(stderr, "enzo setup") {
		t.Errorf("stderr should tell the user what to do:\n%s", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".enzo")); err == nil {
		t.Error("a rejected token must not be written to disk")
	}
}

func TestSetupReadsStdin(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)

	cmd := exec.Command(enzoBin, "setup", "--host", srv.URL+"/")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("ghp_piped\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("setup from stdin: %v\n%s", err, out)
	}

	b, err := os.ReadFile(filepath.Join(dir, ".enzo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "ghp_piped") {
		t.Errorf(".enzo = %s, want the piped token", b)
	}
}

func TestListUsesGitHubTokenEnvVar(t *testing.T) {
	srv := fakeGitHub(t, []map[string]any{{"number": 5, "title": "from env", "state": "open"}})
	dir := testRepo(t)

	// Write a host-only config so the client targets the fake server, and let
	// the token come from the environment.
	cfg := `{"host":"` + srv.URL + `/"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".enzo"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := run(t, dir, []string{"GITHUB_TOKEN=ghp_env"}, "list", "--plain")
	if code != 0 {
		t.Fatalf("list exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "from env") {
		t.Errorf("list output:\n%s", stdout)
	}
}

func TestListWithoutTokenExitsNonZero(t *testing.T) {
	dir := testRepo(t)
	_, stderr, code := run(t, dir, []string{"GITHUB_TOKEN=", "ENZO_TOKEN="}, "list", "--plain")
	if code == 0 {
		t.Error("list with no token should exit non-zero")
	}
	if !strings.Contains(stderr, "enzo setup") {
		t.Errorf("stderr should point at setup:\n%s", stderr)
	}
}

// Piping enzo's output must not drop into the TUI.
func TestBareListIsPlainWhenPiped(t *testing.T) {
	srv := fakeGitHub(t, []map[string]any{{"number": 7, "title": "piped", "state": "open"}})
	dir := testRepo(t)
	cfg := `{"token":"t","host":"` + srv.URL + `/"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".enzo"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := run(t, dir, nil, "list")
	if code != 0 {
		t.Fatalf("list exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "#7") {
		t.Errorf("piped list output:\n%s", stdout)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("piped output should carry no escape codes:\n%q", stdout)
	}
}

// configure writes a .enzo pointing at the fake server.
func configure(t *testing.T, dir, host string) {
	t.Helper()
	cfg := `{"token":"t","host":"` + host + `/"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".enzo"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNewCreatesAnIssue(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	logPath, logEnv := logFileFor(t)
	stdout, stderr, code := run(t, dir, logEnv, "new", "--title", "from the CLI", "--body", "with detail")
	if code != 0 {
		t.Fatalf("new exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	// Creating is silent; the record goes to the log.
	if stdout != "" {
		t.Errorf("stdout should be empty, got:\n%s", stdout)
	}
	lines := readLog(t, logPath)
	if len(lines) != 1 {
		t.Fatalf("log has %d lines, want 1:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for _, want := range []string{"justin-efficient/enzo", "created", "#57", "https://github.com/justin-efficient/enzo/issues/57"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("log line %q missing %q", lines[0], want)
		}
	}

	if len(created) != 1 {
		t.Fatalf("server saw %d creates, want 1", len(created))
	}
	if created[0]["title"] != "from the CLI" {
		t.Errorf("title = %v", created[0]["title"])
	}
	body, _ := created[0]["body"].(string)
	if !strings.HasPrefix(body, "with detail\n") || !strings.Contains(body, version.Credit()) {
		t.Errorf("body = %q, want what was written plus enzo's footer", body)
	}
	assignees, _ := created[0]["assignees"].([]any)
	if len(assignees) != 1 || assignees[0] != "justin-efficient" {
		t.Errorf("assignees = %v, want the authenticated user", created[0]["assignees"])
	}
	if len(linked) != 0 {
		t.Errorf("a top-level issue should not be linked: %v", linked)
	}
}

func TestNewSubLinksToParent(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	logPath, logEnv := logFileFor(t)
	stdout, stderr, code := run(t, dir, logEnv, "new", "sub", "12", "--title", "a child")
	if code != 0 {
		t.Fatalf("new sub exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty, got:\n%s", stdout)
	}
	lines := readLog(t, logPath)
	if len(lines) != 2 {
		t.Fatalf("log has %d lines, want a creation and a link:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[0], "created") {
		t.Errorf("first line should record the creation: %q", lines[0])
	}
	if !strings.Contains(lines[1], "linked") || !strings.Contains(lines[1], "#12") {
		t.Errorf("second line should record the link under #12: %q", lines[1])
	}

	if len(linked) != 1 {
		t.Fatalf("server saw %d links, want 1", len(linked))
	}
	// The child is addressed by database id, not issue number.
	if got, ok := linked[0]["sub_issue_id"].(float64); !ok || int64(got) != 987654 {
		t.Errorf("sub_issue_id = %v, want the created issue id 987654", linked[0]["sub_issue_id"])
	}
}

func TestNewWithoutTitleExitsNonZero(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	_, stderr, code := run(t, dir, nil, "new")
	if code == 0 {
		t.Error("new with no title and no terminal should exit non-zero")
	}
	if !strings.Contains(stderr, "--title") {
		t.Errorf("stderr should say how to supply a title:\n%s", stderr)
	}
	if len(created) != 0 {
		t.Errorf("nothing should have been created: %v", created)
	}
}

func TestNewSubWithoutParentExitsNonZero(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	_, stderr, code := run(t, dir, nil, "new", "sub", "--title", "a child")
	if code == 0 {
		t.Error("new sub with no parent and no terminal should exit non-zero")
	}
	if !strings.Contains(stderr, "parent issue number") {
		t.Errorf("stderr:\n%s", stderr)
	}
	if len(created) != 0 {
		t.Errorf("nothing should have been created: %v", created)
	}
}

func TestHelpMentionsCommands(t *testing.T) {
	stdout, _, code := run(t, t.TempDir(), nil, "help")
	if code != 0 {
		t.Fatalf("help exited %d", code)
	}
	for _, want := range []string{"enzo new", "enzo start"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help should list %q:\n%s", want, stdout)
		}
	}
}

// End to end: `enzo start 12` branches, pushes and drafts a pull request.
func TestStartBranchesAndDraftsAPR(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)
	logPath, env := logFileFor(t)

	stdout, stderr, code := run(t, dir, env, "start", "12")
	if code != 0 {
		t.Fatalf("start exited %d\nstdout:%s\nstderr:%s", code, stdout, stderr)
	}

	const branch = "justin-efficient/12-the-parent"
	if got := gitIn(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != branch {
		t.Errorf("on branch %q, want %q", got, branch)
	}
	// The branch reached origin, or GitHub would have nothing to open a PR on.
	if got := gitIn(t, filepath.Join(dir, "origin.git"), "rev-parse", "--abbrev-ref", branch); got != branch {
		t.Errorf("origin has %q, want the pushed branch", got)
	}

	if len(pulls) != 1 {
		t.Fatalf("opened %d pull requests, want 1: %v", len(pulls), pulls)
	}
	pr := pulls[0]
	if pr["draft"] != true {
		t.Errorf("draft = %v, want true", pr["draft"])
	}
	if pr["head"] != branch || pr["base"] != "main" {
		t.Errorf("head/base = %v/%v", pr["head"], pr["base"])
	}
	prBody, _ := pr["body"].(string)
	if !strings.Contains(prBody, "Closes #12") {
		t.Errorf("PR body = %q, want it to close the issue", prBody)
	}
	if !strings.Contains(prBody, version.Credit()) {
		t.Errorf("PR body = %q, want enzo's footer", prBody)
	}
	// #12 already existed, so start had no reason to open an issue.
	if len(created) != 0 {
		t.Errorf("nothing should have been created: %v", created)
	}
	if !strings.Contains(stdout, "pull/300") {
		t.Errorf("stdout should name the PR:\n%s", stdout)
	}

	lines := readLog(t, logPath)
	if len(lines) != 1 || !strings.Contains(lines[0], "drafted") {
		t.Errorf("log = %v, want one drafted entry", lines)
	}
}

// End to end: a title with no number opens the issue first.
func TestStartOpensAnIssueFromATitle(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	_, stderr, code := run(t, dir, nil, "start", "add a thing")
	if code != 0 {
		t.Fatalf("start exited %d: %s", code, stderr)
	}

	if len(created) != 1 || created[0]["title"] != "add a thing" {
		t.Fatalf("created = %v, want the issue", created)
	}
	// The fake opens #57, so that is what the branch and the PR are for.
	if got := gitIn(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "justin-efficient/57-add-a-thing" {
		t.Errorf("on branch %q", got)
	}
	if len(pulls) != 1 {
		t.Fatalf("opened %d pull requests, want 1", len(pulls))
	}
	prBody, _ := pulls[0]["body"].(string)
	if !strings.Contains(prBody, "Closes #57") || !strings.Contains(prBody, version.Credit()) {
		t.Errorf("PR body = %q, want it to close #57 and carry enzo's footer", prBody)
	}
}

func TestStartWithNothingToGoOnExitsNonZero(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	_, stderr, code := run(t, dir, nil, "start")
	if code == 0 {
		t.Error("start with no issue and no title should exit non-zero")
	}
	if !strings.Contains(stderr, "issue number") {
		t.Errorf("stderr should say what is missing:\n%s", stderr)
	}
	if len(created) != 0 || len(pulls) != 0 {
		t.Error("a rejected command should not reach GitHub")
	}
	if got := gitIn(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("left on branch %q, want main", got)
	}
}

// gitIn runs git in dir and returns its trimmed output.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// End to end: sub-issues come back nested under their parent.
func TestListNestsSubIssues(t *testing.T) {
	srv := fakeGitHub(t, []map[string]any{
		{"id": 500, "number": 5, "title": "a child", "state": "open",
			"parent_issue_url": "https://api.github.com/repos/justin-efficient/enzo/issues/1"},
		{"id": 400, "number": 4, "title": "unrelated", "state": "open"},
		{"id": 100, "number": 1, "title": "the parent", "state": "open"},
	})
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	stdout, stderr, code := run(t, dir, nil, "list", "--plain")
	if code != 0 {
		t.Fatalf("list exited %d\nstderr: %s", code, stderr)
	}

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), stdout)
	}
	for i, want := range []string{"#4", "#1", "#5"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want %s:\n%s", i, lines[i], want, stdout)
		}
	}
	if !strings.HasPrefix(lines[2], "  #5") {
		t.Errorf("the child should be indented, got %q", lines[2])
	}
	if strings.HasPrefix(lines[1], " ") {
		t.Errorf("the parent should not be indented, got %q", lines[1])
	}
}

func TestNewPositionalTitleEndToEnd(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	stdout, stderr, code := run(t, dir, nil, "new", "my new issue")
	if code != 0 {
		t.Fatalf("new exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if len(created) != 1 {
		t.Fatalf("server saw %d creates, want 1", len(created))
	}
	if created[0]["title"] != "my new issue" {
		t.Errorf("title = %v", created[0]["title"])
	}
	// No body was given, so the footer is the whole of it.
	if body, _ := created[0]["body"].(string); body != "*"+version.Credit()+"*" {
		t.Errorf("body = %q, want the footer alone", body)
	}
}

func TestNewSubPositionalTitleEndToEnd(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	stdout, stderr, code := run(t, dir, nil, "new", "sub", "12", "my new sub issue work")
	if code != 0 {
		t.Fatalf("new sub exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if len(created) != 1 || created[0]["title"] != "my new sub issue work" {
		t.Errorf("created = %v", created)
	}
	if len(linked) != 1 {
		t.Errorf("server saw %d links, want 1", len(linked))
	}
}

func TestNewUnquotedTitleEndToEnd(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	_, stderr, code := run(t, dir, nil, "new", "my", "new", "issue")
	if code == 0 {
		t.Error("an unquoted title should exit non-zero")
	}
	if !strings.Contains(stderr, "quote the title") {
		t.Errorf("stderr should show the fix:\n%s", stderr)
	}
	if len(created) != 0 {
		t.Errorf("nothing should have been created: %v", created)
	}
}

// Nothing was created, so nothing should be logged.
func TestFailedCreateLogsNothing(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)
	logPath, logEnv := logFileFor(t)

	_, _, code := run(t, dir, logEnv, "new", "my", "unquoted", "title")
	if code == 0 {
		t.Fatal("an unquoted title should exit non-zero")
	}
	if lines := readLog(t, logPath); len(lines) != 0 {
		t.Errorf("nothing should be logged, got:\n%s", strings.Join(lines, "\n"))
	}
}

// Repeated runs append rather than overwrite.
func TestLogAccumulatesAcrossRuns(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)
	logPath, logEnv := logFileFor(t)

	for _, title := range []string{"first", "second", "third"} {
		if _, stderr, code := run(t, dir, logEnv, "new", title); code != 0 {
			t.Fatalf("new %q exited %d: %s", title, code, stderr)
		}
	}
	lines := readLog(t, logPath)
	if len(lines) != 3 {
		t.Fatalf("log has %d lines, want 3:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for i, want := range []string{"first", "second", "third"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want %q", i, lines[i], want)
		}
	}
}

// The log names private repos, so the file it creates must be owner-only.
func TestLogFilePermissions(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)
	logPath := filepath.Join(t.TempDir(), "nested", "enzo.log")

	if _, stderr, code := run(t, dir, []string{"ENZO_LOG=" + logPath}, "new", "a title"); code != 0 {
		t.Fatalf("new exited %d: %s", code, stderr)
	}
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("the log was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("log mode = %o, want 600", perm)
	}
}

// Guard the guard: every subprocess must log inside the sandbox, so a test
// that forgets ENZO_LOG cannot append to the real user's state file.
func TestRunsAreSandboxedFromTheRealLog(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	before := readLog(t, sandboxLog)
	if _, stderr, code := run(t, dir, nil, "new", "sandbox check"); code != 0 {
		t.Fatalf("new exited %d: %s", code, stderr)
	}
	after := readLog(t, sandboxLog)
	if len(after) != len(before)+1 {
		t.Errorf("a run with no ENZO_LOG went somewhere else: sandbox had %d lines, now %d", len(before), len(after))
	}
}

// The banner is built in exactly one place. A hardcoded copy somewhere else
// would keep rendering the old number after the next version bump, and nothing
// would fail — so this fails instead.
func TestBannerIsNotHardcodedAnywhere(t *testing.T) {
	const literal = "🚘 enzo v"
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case info.IsDir() && (info.Name() == ".git" || info.Name() == "dist"):
			return filepath.SkipDir
		case info.IsDir() || !strings.HasSuffix(path, ".go"):
			return nil
		// version is where the banner is built, and tests are allowed to say
		// what they expect to see.
		case strings.HasPrefix(path, filepath.Join("internal", "version")):
			return nil
		case strings.HasSuffix(path, "_test.go"):
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), literal) {
			t.Errorf("%s hardcodes %q; call version.Banner() instead", path, literal)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the source: %v", err)
	}
}

// End to end: abort closes the PR and deletes the branch, here and on origin,
// and only after the phrase is typed.
func TestAbortEndToEnd(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)
	logPath, env := logFileFor(t)

	const branch = "justin-efficient/12-fix-the-thing"
	gitIn(t, dir, "switch", "-q", "-c", branch)
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "work")
	gitIn(t, dir, "push", "-q", "-u", "origin", branch)
	openPR = map[string]any{
		"number": 77, "title": "fix the thing", "state": "open", "draft": true,
		"html_url": "https://github.com/justin-efficient/enzo/pull/77",
		"head":     map[string]any{"ref": branch},
		"base":     map[string]any{"ref": "main"},
	}

	stdout, stderr, code := runStdin(t, dir, env, "nukefromorbit\n", "abort")
	if code != 0 {
		t.Fatalf("abort exited %d\nstdout:%s\nstderr:%s", code, stdout, stderr)
	}

	if len(closedPulls) != 1 || closedPulls[0]["state"] != "closed" {
		t.Errorf("closed = %v, want one PR closed", closedPulls)
	}
	if got := gitIn(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("left on %q, want main", got)
	}
	if out := gitIn(t, dir, "branch", "--format=%(refname:short)"); strings.Contains(out, "fix-the-thing") {
		t.Errorf("local branch survived:\n%s", out)
	}
	bare := filepath.Join(dir, "origin.git")
	if out := gitIn(t, bare, "for-each-ref", "--format=%(refname:short)", "refs/heads"); strings.Contains(out, "fix-the-thing") {
		t.Errorf("origin still has the branch:\n%s", out)
	}
	if lines := readLog(t, logPath); len(lines) != 1 || !strings.Contains(lines[0], "aborted") {
		t.Errorf("log = %v, want one aborted entry", lines)
	}
}

// Without the phrase, abort is a no-op and exits quietly.
func TestAbortWithoutThePhraseChangesNothing(t *testing.T) {
	srv := fakeGitHub(t, nil)
	dir := testRepo(t)
	configure(t, dir, srv.URL)

	const branch = "justin-efficient/12-fix-the-thing"
	gitIn(t, dir, "switch", "-q", "-c", branch)
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "work")
	gitIn(t, dir, "push", "-q", "-u", "origin", branch)

	stdout, _, code := runStdin(t, dir, nil, "no thanks\n", "abort")
	if code != 0 {
		t.Errorf("backing out should exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "nothing was touched") {
		t.Errorf("stdout should say nothing happened:\n%s", stdout)
	}
	if len(closedPulls) != 0 {
		t.Errorf("closed %v without the phrase", closedPulls)
	}
	if got := gitIn(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != branch {
		t.Errorf("left on %q, want the branch untouched", got)
	}
}

// runStdin is run with something on the command's stdin.
func runStdin(t *testing.T, dir string, env []string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(enzoBin, args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "ENZO_LOG="+sandboxLog), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()

	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running enzo %v: %v", args, err)
	}
	return stdout.String(), stderr.String(), code
}
