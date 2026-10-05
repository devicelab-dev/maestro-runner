package telemetry

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// clearEnv unsets the opt-out, debug and CI variables for one test.
func clearEnv(t *testing.T) {
	for _, k := range []string{"GITHUB_REPOSITORY_ID", "CI_PROJECT_ID", "BITRISE_APP_SLUG", "BUILDKITE_PIPELINE_ID", "CIRCLE_PROJECT_ID",
		"DO_NOT_TRACK", "MAESTRO_RUNNER_NO_TELEMETRY", "MAESTRO_RUNNER_TELEMETRY_DEBUG",
		"MAESTRO_RUNNER_TELEMETRY_URL", "CI", "GITHUB_ACTIONS", "GITLAB_CI", "BITRISE_IO", "CIRCLECI", "BUILDKITE",
		"JENKINS_URL", "TF_BUILD", "TRAVIS", "CODEBUILD_BUILD_ID", "TEAMCITY_VERSION", "EAS_BUILD", "CM_BUILD_ID"} {
		if v, ok := os.LookupEnv(k); ok {
			t.Cleanup(func() { os.Setenv(k, v) })
			os.Unsetenv(k)
		}
	}
}

func TestEnabledHonoursOptOuts(t *testing.T) {
	clearEnv(t)
	if !Enabled() {
		t.Fatal("telemetry off with no opt-out set")
	}
	for _, c := range []struct {
		env, val string
		on       bool
	}{
		{"DO_NOT_TRACK", "1", false},
		{"DO_NOT_TRACK", "true", false},
		{"DO_NOT_TRACK", "0", true},
		{"DO_NOT_TRACK", "false", true},
		{"MAESTRO_RUNNER_NO_TELEMETRY", "yes", false},
	} {
		t.Setenv(c.env, c.val)
		if got := Enabled(); got != c.on {
			t.Errorf("%s=%s: Enabled() = %v, want %v", c.env, c.val, got, c.on)
		}
		os.Unsetenv(c.env)
	}
}

func TestDetectCI(t *testing.T) {
	clearEnv(t)
	if got := DetectCI(); got != "none" {
		t.Fatalf("no CI env: %q, want none", got)
	}
	t.Setenv("GITHUB_ACTIONS", "true")
	if got := DetectCI(); got != "github-actions" {
		t.Errorf("GitHub Actions: %q", got)
	}
	os.Unsetenv("GITHUB_ACTIONS")
	t.Setenv("CI", "1")
	if got := DetectCI(); got != "other" {
		t.Errorf("generic CI: %q, want other", got)
	}
}

// gitRepo makes a repository with one commit in a new directory.
func gitRepo(t *testing.T) (string, func(dir string, args ...string) string) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	run := func(dir string, args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	repo := t.TempDir()
	run(repo, "init", "-q")
	run(repo, "commit", "-q", "--allow-empty", "-m", "first")
	return repo, run
}

// A full clone with no remote is identified by its first commit, the same
// from any of its directories; outside git there is no id. The id is a
// hash, not the commit hash itself.
func TestProjectIDFirstCommit(t *testing.T) {
	clearEnv(t)
	repo, run := gitRepo(t)
	sub := filepath.Join(repo, "flows", "auth")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	id := ProjectID(repo)
	if len(id) != 64 || ProjectID(sub) != id {
		t.Fatalf("id %q / from a subdirectory %q", id, ProjectID(sub))
	}
	if strings.Contains(id, run(repo, "rev-list", "--max-parents=0", "HEAD")) {
		t.Error("project id carries the commit hash itself")
	}
	if got := ProjectID(t.TempDir()); got != "" {
		t.Errorf("outside git: %q, want empty", got)
	}
}

// A CI checkout is a shallow clone that only knows its latest commit: its id
// must not change with every commit, and must match the full clone of the
// same repository, whatever form the remote address takes.
func TestProjectIDShallowClone(t *testing.T) {
	clearEnv(t)
	repo, run := gitRepo(t)
	run(repo, "remote", "add", "origin", "git@github.com:Owner/Repo.git")
	full := ProjectID(repo)

	shallow := func() string {
		dir := filepath.Join(t.TempDir(), "c")
		run(t.TempDir(), "clone", "-q", "--depth", "1", "file://"+repo, dir)
		if run(dir, "rev-parse", "--is-shallow-repository") != "true" {
			t.Fatal("clone is not shallow")
		}
		run(dir, "remote", "set-url", "origin", "https://x-access-token:secret@github.com/owner/repo.git")
		return ProjectID(dir)
	}
	first := shallow()
	run(repo, "commit", "-q", "--allow-empty", "-m", "second")
	second := shallow()
	if first == "" || first != second {
		t.Errorf("shallow clones of two commits: %q and %q, want one stable id", first, second)
	}
	if first != full {
		t.Errorf("shallow clone %q, full clone %q: want the same project", first, full)
	}

	// A shallow clone with no remote has nothing stable: no id (not the
	// latest commit).
	dir := filepath.Join(t.TempDir(), "c")
	run(t.TempDir(), "clone", "-q", "--depth", "1", "file://"+repo, dir)
	run(dir, "remote", "remove", "origin")
	if got := ProjectID(dir); got != "" {
		t.Errorf("shallow clone without remote: %q, want empty", got)
	}
	t.Setenv("GITHUB_REPOSITORY_ID", "12345")
	if got := ProjectID(dir); got == "" {
		t.Error("shallow clone on GitHub Actions: want the repository id to give a project id")
	}
}

func TestNormalizeRemote(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:Owner/Repo.git":                        "github.com/owner/repo",
		"https://github.com/owner/repo":                        "github.com/owner/repo",
		"https://github.com/owner/repo.git/":                   "github.com/owner/repo",
		"https://x-access-token:T0K3N@github.com/owner/repo":   "github.com/owner/repo",
		"ssh://git@gitlab.example.com:2222/group/sub/repo.git": "gitlab.example.com/group/sub/repo",
		"/home/me/repos/app":                                   "",
		"./app":                                                "",
		"":                                                     "",
	} {
		if got := normalizeRemote(in); got != want {
			t.Errorf("normalizeRemote(%q) = %q, want %q", in, got, want)
		}
	}
}

// writeReport writes an index of three flows (one passed first time, one
// passed on retry, one failed on an element_not_found step) and the failed
// flow's detail file.
func writeReport(t *testing.T) (*report.Index, string) {
	dir := t.TempDir()
	detail := report.FlowDetail{Commands: []report.Command{
		{Type: "launchApp", Status: report.StatusPassed},
		{Type: "runFlow", Status: report.StatusFailed, SubCommands: []report.Command{
			{Type: "tapOn", Status: report.StatusFailed, Error: &report.Error{Type: "element_not_found", Message: "Element 'Log in as bob@example.com' not found"}},
		}},
	}}
	b, _ := json.Marshal(detail)
	if err := os.MkdirAll(filepath.Join(dir, "flows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "flows", "f3.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return &report.Index{
		Device: report.Device{ID: "emulator-5554", Platform: "android", OSVersion: "15", Name: "Pixel of Bob"},
		App:    report.App{ID: "com.secret.app"},
		Flows: []report.FlowEntry{
			{Name: "login", SourceFile: "/Users/bob/flows/login.yaml", Status: report.StatusPassed, Attempts: 1, Commands: report.CommandSummary{Total: 10}},
			{Name: "checkout", Status: report.StatusPassed, Attempts: 2, Commands: report.CommandSummary{Total: 20}},
			{Name: "profile", Status: report.StatusFailed, Attempts: 3, DataFile: "flows/f3.json", Commands: report.CommandSummary{Total: 5}},
		},
	}, dir
}

func TestBuildFromReport(t *testing.T) {
	clearEnv(t)
	index, dir := writeReport(t)
	ev := Build(Run{
		Version: "1.1.28.2", Command: "test", FlagNames: []string{"retries", "parallel", "retries", "env"},
		Platform: "android", Driver: "devicelab", Devices: 1, Outcome: "completed", Duration: 612400 * time.Millisecond,
	}, index, dir)

	if ev.FlowsTotal != 3 || ev.FlowsPassed != 2 || ev.FlowsFailed != 1 || ev.FlowsPassedOnRetry != 1 {
		t.Errorf("flows %d total, %d passed, %d failed, %d on retry; want 3, 2, 1, 1",
			ev.FlowsTotal, ev.FlowsPassed, ev.FlowsFailed, ev.FlowsPassedOnRetry)
	}
	if ev.StepsTotal != 35 {
		t.Errorf("steps %d, want 35", ev.StepsTotal)
	}
	if ev.FailureKinds["element_not_found"] != 1 || len(ev.FailureKinds) != 1 {
		t.Errorf("failure kinds %v, want element_not_found:1", ev.FailureKinds)
	}
	if ev.DeviceKind != "emulator" || ev.OSMajor != "android 15" || ev.DurationS != 612 || ev.CI != "none" {
		t.Errorf("device %q os %q duration %d ci %q", ev.DeviceKind, ev.OSMajor, ev.DurationS, ev.CI)
	}
	if strings.Join(ev.Flags, ",") != "--env,--parallel,--retries" {
		t.Errorf("flags %v", ev.Flags)
	}
}

// Nothing from the run's content reaches the payload: no flow, file, app or
// device names, no error message, no ids.
func TestPayloadCarriesNoContent(t *testing.T) {
	clearEnv(t)
	index, dir := writeReport(t)
	ev := Build(Run{Version: "1.1.28.2", Command: "test", Platform: "android", Outcome: "completed"}, index, dir)
	b, _ := json.Marshal(ev)
	for _, secret := range []string{"login", "checkout", "profile", "/Users/bob", "bob@example.com", "com.secret.app", "Pixel of Bob", "emulator-5554", "Element '"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("payload contains %q: %s", secret, b)
		}
	}
	var fields map[string]any
	_ = json.Unmarshal(b, &fields)
	allowed := map[string]bool{"runner_version": true, "command": true, "flags": true, "project_id": true, "ci": true,
		"host_os": true, "host_arch": true, "platform": true, "driver": true, "device_kind": true, "os_major": true,
		"devices": true, "flows_total": true, "flows_passed": true, "flows_failed": true, "flows_passed_on_retry": true,
		"steps_total": true, "failure_kinds": true, "run_outcome": true, "duration_s": true}
	for k := range fields {
		if !allowed[k] {
			t.Errorf("unexpected field %q", k)
		}
	}
}

// A flag given with its value never carries the value; a version that is
// not a release becomes "dev"; an unknown driver becomes "other".
func TestValuesAreReducedToTokens(t *testing.T) {
	if got := flagNames([]string{"env=PASSWORD=x", "device", "--Bad Flag", "p", "platform"}); strings.Join(got, ",") != "--device,--platform" {
		t.Errorf("flagNames: %v", got)
	}
	if versionToken("v1.1.28-31-g6904d0f-dirty") != "dev" || versionToken("1.1.28.2") != "1.1.28.2" {
		t.Error("versionToken")
	}
	if oneOf("my-driver", "devicelab") != "other" {
		t.Error("oneOf")
	}
}

func TestSendPostsJSONAndDebugDoesNot(t *testing.T) {
	clearEnv(t)
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s", r.Method, r.Header.Get("Content-Type"))
		}
		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Setenv("MAESTRO_RUNNER_TELEMETRY_URL", srv.URL)

	Send(Event{RunnerVersion: "1.1.28.2", Command: "test", RunOutcome: "completed"})
	if !strings.Contains(string(got), `"runner_version":"1.1.28.2"`) {
		t.Fatalf("server got %s", got)
	}

	got = nil
	t.Setenv("MAESTRO_RUNNER_TELEMETRY_DEBUG", "1")
	Send(Event{RunnerVersion: "1.1.28.2", Command: "test"})
	if got != nil {
		t.Error("debug mode sent the event")
	}
}

// A slow or dead endpoint costs at most the send timeout.
func TestSendIsBounded(t *testing.T) {
	clearEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()
	t.Setenv("MAESTRO_RUNNER_TELEMETRY_URL", srv.URL)
	start := time.Now()
	Send(Event{RunnerVersion: "1.1.28.2", Command: "test"})
	if took := time.Since(start); took > sendTimeout+time.Second {
		t.Errorf("Send took %v, want about %v at most", took, sendTimeout)
	}
}
