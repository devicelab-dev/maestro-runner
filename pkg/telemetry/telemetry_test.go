package telemetry

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// clearEnv unsets the opt-out, debug and CI variables for one test.
func clearEnv(t *testing.T) {
	for _, k := range []string{
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

// The project id depends only on which flow files there are: not their
// order or duplicates. Adding or renaming a file changes it. It is a hash,
// not the names.
func TestProjectIDFromFlowPaths(t *testing.T) {
	id := ProjectID([]string{"login.yaml", "auth/signup.yaml"})
	if len(id) != 64 {
		t.Fatalf("id %q", id)
	}
	if got := ProjectID([]string{"auth/signup.yaml", "login.yaml", "login.yaml", ""}); got != id {
		t.Errorf("reordered with a duplicate: %q, want %q", got, id)
	}
	for _, other := range [][]string{
		{"login.yaml", "auth/signup.yaml", "checkout.yaml"}, // added
		{"login.yaml", "auth/register.yaml"},                // renamed
		{"login.yaml"},                                      // a subset
	} {
		if ProjectID(other) == id {
			t.Errorf("%v gave the same id", other)
		}
	}
	for _, none := range [][]string{nil, {}, {""}} {
		if got := ProjectID(none); got != "" {
			t.Errorf("ProjectID(%q) = %q, want empty", none, got)
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

// A run without --platform finds its platform from the device, after the run
// config is made: the event takes the platform and driver from the report.
func TestBuildPlatformFromReport(t *testing.T) {
	clearEnv(t)
	index, dir := writeReport(t)
	index.Device = report.Device{ID: "11171JEC200939", Platform: "android", OSVersion: "33"}
	index.MaestroRunner.Driver = "devicelab"
	ev := Build(Run{Version: "1.1.29", Command: "test", Devices: 1, Outcome: "completed"}, index, dir)
	if ev.Platform != "android" || ev.Driver != "devicelab" || ev.DeviceKind != "real" || ev.OSMajor != "android 33" {
		t.Errorf("platform %q driver %q device %q os %q; want android, devicelab, real, android 33",
			ev.Platform, ev.Driver, ev.DeviceKind, ev.OSMajor)
	}
	// A platform or driver the run config names is kept.
	ev = Build(Run{Version: "1.1.29", Command: "test", Platform: "android", Driver: "uiautomator2", Outcome: "completed"}, index, dir)
	if ev.Platform != "android" || ev.Driver != "uiautomator2" {
		t.Errorf("platform %q driver %q; want android, uiautomator2", ev.Platform, ev.Driver)
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

// A parallel run counts the devices that ran flows and reports a kind or OS
// version only when they agree.
func TestBuildMultiDevice(t *testing.T) {
	clearEnv(t)
	flow := func(d report.Device) report.FlowEntry {
		return report.FlowEntry{Status: report.StatusPassed, Attempts: 1, Device: &d}
	}
	emu35 := report.Device{ID: "emulator-5554", Platform: "android", OSVersion: "35", IsSimulator: true}
	emu35b := report.Device{ID: "emulator-5556", Platform: "android", OSVersion: "35", IsSimulator: true}
	real34 := report.Device{ID: "R5CT", Platform: "android", OSVersion: "34"}

	same := Build(Run{Version: "1.1.29", Command: "test", Platform: "android", Devices: 2, Outcome: "completed"},
		&report.Index{Flows: []report.FlowEntry{flow(emu35), flow(emu35b), flow(emu35)}}, "")
	if same.Devices != 2 || same.DeviceKind != "emulator" || same.OSMajor != "android 35" {
		t.Errorf("two alike emulators: devices %d kind %q os %q", same.Devices, same.DeviceKind, same.OSMajor)
	}

	mixed := Build(Run{Version: "1.1.29", Command: "test", Platform: "android", Devices: 2, Outcome: "completed"},
		&report.Index{Flows: []report.FlowEntry{flow(emu35), flow(real34)}}, "")
	if mixed.Devices != 2 || mixed.DeviceKind != "mixed" || mixed.OSMajor != "mixed" {
		t.Errorf("emulator + real phone: devices %d kind %q os %q", mixed.Devices, mixed.DeviceKind, mixed.OSMajor)
	}

	// A device whose OS version could not be read is not a different version.
	emuBlank := report.Device{ID: "emulator-5558", Platform: "android", IsSimulator: true}
	blank := Build(Run{Version: "1.1.29", Command: "test", Platform: "android", Devices: 2, Outcome: "completed"},
		&report.Index{Flows: []report.FlowEntry{flow(emu35), flow(emuBlank)}}, "")
	if blank.OSMajor != "android 35" {
		t.Errorf("one OS unreadable: os %q, want android 35", blank.OSMajor)
	}

	// Appium cloud sessions have no local device list: count the sessions.
	s1 := report.Device{Platform: "android", OSVersion: "14", SessionID: "a"}
	s2 := report.Device{Platform: "android", OSVersion: "14", SessionID: "b"}
	cloud := Build(Run{Version: "1.1.29", Command: "test", Platform: "android", Driver: "appium", Cloud: true, Devices: 1, Outcome: "completed"},
		&report.Index{Flows: []report.FlowEntry{flow(s1), flow(s2), flow(s1)}}, "")
	if cloud.Devices != 2 || cloud.DeviceKind != "cloud" || cloud.OSMajor != "android 14" {
		t.Errorf("cloud sessions: devices %d kind %q os %q", cloud.Devices, cloud.DeviceKind, cloud.OSMajor)
	}
}
