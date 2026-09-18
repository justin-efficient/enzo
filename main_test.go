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
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// fakeGitHub serves the endpoints enzo uses, at the enterprise path prefix
// go-github adds for a custom host.
// created and linked record the write requests the fake server received.
var created, linked []map[string]any

func fakeGitHub(t *testing.T, issues []map[string]any) *httptest.Server {
	t.Helper()
	created, linked = nil, nil
	mux := http.NewServeMux()
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
	for _, arg := range []string{"--version", "version"} {
		stdout, _, code := run(t, t.TempDir(), nil, arg)
		if code != 0 {
			t.Errorf("%s exited %d, want 0", arg, code)
		}
		if !strings.HasPrefix(stdout, "enzo ") {
			t.Errorf("%s printed %q, want a version line", arg, stdout)
		}
	}
}

func TestVersionIsSetByLdflags(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "enzo")
	build := exec.Command("go", "build", "-ldflags", "-X main.version=1.2.3", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "enzo 1.2.3" {
		t.Errorf("version = %q, want %q", got, "enzo 1.2.3")
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
	if created[0]["body"] != "with detail" {
		t.Errorf("body = %v", created[0]["body"])
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

func TestHelpMentionsNew(t *testing.T) {
	stdout, _, code := run(t, t.TempDir(), nil, "help")
	if code != 0 {
		t.Fatalf("help exited %d", code)
	}
	if !strings.Contains(stdout, "enzo new") {
		t.Errorf("help should list new:\n%s", stdout)
	}
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
	// No body was given, so none should be sent.
	if _, ok := created[0]["body"]; ok {
		t.Errorf("body should be omitted, got %v", created[0]["body"])
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
