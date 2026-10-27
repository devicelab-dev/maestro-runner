package devicelab_ios_legacy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// fakeSimctl records every simctl call. reply picks each call's stdout and
// whether it fails; nil answers every call with empty success.
type fakeSimctl struct {
	mu    sync.Mutex
	calls [][]string
	cmds  []*exec.Cmd
	reply func(args []string) (out string, fail bool)
}

func (f *fakeSimctl) install(t *testing.T) {
	t.Helper()
	orig := simctlCommand
	simctlCommand = func(args ...string) *exec.Cmd {
		f.mu.Lock()
		f.calls = append(f.calls, args)
		f.mu.Unlock()
		out, fail := "", false
		if f.reply != nil {
			out, fail = f.reply(args)
		}
		code := "0"
		if fail {
			code = "1"
		}
		cmd := exec.Command("sh", "-c", `printf '%s' "$1"; exit "$2"`, "sh", out, code)
		f.mu.Lock()
		f.cmds = append(f.cmds, cmd) // kept so a test can read the Env the caller set
		f.mu.Unlock()
		return cmd
	}
	t.Cleanup(func() { simctlCommand = orig })
}

func (f *fakeSimctl) joined() []string {
	var out []string
	for _, c := range f.calls {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

func simDriver() *Driver {
	return &Driver{udid: "SIM", info: &core.PlatformInfo{IsSimulator: true}}
}

func TestLaunchAppDefaultsToAllowAll(t *testing.T) {
	f := &fakeSimctl{}
	f.install(t)
	d := simDriver()
	res := d.handleLaunchApp(&flow.LaunchAppStep{AppID: "com.x"})
	if !res.Success {
		t.Fatalf("launch failed: %s", res.Message)
	}
	want := []string{
		"privacy SIM reset all com.x",
		"privacy SIM grant all com.x",
		"launch --terminate-running-process SIM com.x",
	}
	if got := f.joined(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %q\nwant %q", got, want)
	}
}

func TestLaunchAppStopAppFalseKeepsRunningApp(t *testing.T) {
	f := &fakeSimctl{}
	f.install(t)
	d := simDriver()
	stop := false
	res := d.handleLaunchApp(&flow.LaunchAppStep{AppID: "com.x", StopApp: &stop})
	if !res.Success {
		t.Fatalf("launch failed: %s", res.Message)
	}
	got := f.joined()
	if last := got[len(got)-1]; last != "launch SIM com.x" {
		t.Errorf("launch = %q, want no --terminate-running-process", last)
	}
	for _, c := range got {
		if strings.HasPrefix(c, "terminate") {
			t.Errorf("stopApp: false must not terminate: %q", got)
		}
	}
}

func TestLaunchAppPermissionsKeychainAndEnv(t *testing.T) {
	f := &fakeSimctl{reply: func(args []string) (string, bool) {
		return "", args[0] == "privacy" && args[2] == "grant" && args[3] == "camera"
	}}
	f.install(t)
	d := simDriver()
	res := d.handleLaunchApp(&flow.LaunchAppStep{
		AppID:         "com.x",
		ClearKeychain: true,
		Permissions:   map[string]string{"camera": "allow", "microphone": "deny", "photos": "unset", "notifications": "allow"},
		Environment:   map[string]string{"B": "2", "A": "1"},
	})
	if !res.Success {
		t.Fatalf("a failed permission must not fail the launch: %s", res.Message)
	}
	want := []string{
		"privacy SIM reset all com.x",
		"privacy SIM grant camera com.x",
		"privacy SIM revoke microphone com.x",
		"keychain SIM reset",
		"launch --terminate-running-process SIM com.x",
	}
	if got := f.joined(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %q\nwant %q", got, want)
	}
	launch := f.cmds[len(f.cmds)-1]
	env := strings.Join(launch.Env, "\n")
	if !strings.Contains(env, "SIMCTL_CHILD_A=1\nSIMCTL_CHILD_B=2") {
		t.Errorf("app env missing or unordered: %q", launch.Env)
	}
	if !strings.Contains(env, "PATH=") {
		t.Error("launch env dropped the host PATH")
	}
}

func TestLaunchAppOnDeviceSkipsPermissions(t *testing.T) {
	f := &fakeSimctl{}
	f.install(t)
	d := &Driver{udid: "DEV", info: &core.PlatformInfo{}}
	d.handleLaunchApp(&flow.LaunchAppStep{AppID: "com.x"})
	for _, c := range f.joined() {
		if strings.HasPrefix(c, "privacy") {
			t.Errorf("real device got %q", c)
		}
	}
}

func TestLaunchEnvNilWithoutAppEnv(t *testing.T) {
	if launchEnv(nil) != nil {
		t.Error("no app env must keep the default environment")
	}
}

func TestKillAppAndStopApp(t *testing.T) {
	f := &fakeSimctl{reply: func(args []string) (string, bool) {
		switch args[2] {
		case "gone":
			return "found nothing to terminate", true
		case "broken":
			return "boom", true
		}
		return "", false
	}}
	f.install(t)
	d := simDriver()
	d.appID = "com.x"
	if res := d.handleKillApp(&flow.KillAppStep{}); !res.Success || f.joined()[0] != "terminate SIM com.x" {
		t.Errorf("killApp = %+v, calls %q", res, f.joined())
	}
	if res := d.handleKillApp(&flow.KillAppStep{AppID: "gone"}); !res.Success {
		t.Errorf("killing a stopped app must pass: %+v", res)
	}
	if res := d.handleKillApp(&flow.KillAppStep{AppID: "broken"}); res.Success {
		t.Error("a terminate failure passed")
	}
	if res := (&Driver{}).handleKillApp(&flow.KillAppStep{}); res.Success {
		t.Error("killApp with no app passed")
	}
	if res := d.handleStopApp(&flow.StopAppStep{}); !res.Success {
		t.Errorf("stopApp = %+v", res)
	}
	res := d.executeStep(&flow.KillAppStep{})
	if !res.Success {
		t.Errorf("killApp is not dispatched: %+v", res)
	}
}

func TestClearKeychain(t *testing.T) {
	f := &fakeSimctl{}
	f.install(t)
	if res := simDriver().executeStep(&flow.ClearKeychainStep{}); !res.Success {
		t.Errorf("clearKeychain = %+v", res)
	}
	if res := (&Driver{info: &core.PlatformInfo{}}).handleClearKeychain(); res.Success {
		t.Error("clearKeychain passed on a real device")
	}
	fail := &fakeSimctl{reply: func([]string) (string, bool) { return "no", true }}
	fail.install(t)
	if res := simDriver().handleClearKeychain(); res.Success {
		t.Error("a keychain reset failure passed")
	}
}

func TestSetPermissionsUsesActiveApp(t *testing.T) {
	f := &fakeSimctl{}
	f.install(t)
	d := simDriver()
	d.appID = "com.x"
	res := d.handleSetPermissions(&flow.SetPermissionsStep{Permissions: map[string]string{"location": "inuse", "camera": "allow"}})
	if !res.Success {
		t.Fatalf("setPermissions = %+v", res)
	}
	want := "privacy SIM grant camera com.x|privacy SIM grant location com.x"
	if got := strings.Join(f.joined(), "|"); got != want {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

func TestClearStateReinstallsFromCachedCopy(t *testing.T) {
	app := filepath.Join(t.TempDir(), "X.app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Info.plist"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeSimctl{reply: func(args []string) (string, bool) {
		if args[0] == "get_app_container" {
			return app, false
		}
		return "", false
	}}
	f.install(t)
	d := simDriver()
	defer d.removeStagedApps()
	for i := 0; i < 2; i++ {
		if res := d.handleClearState("com.x"); !res.Success {
			t.Fatalf("clearState = %+v", res)
		}
	}
	if len(d.stagedApps) != 1 {
		t.Fatalf("staged copies = %d, want 1", len(d.stagedApps))
	}
	first := d.stagedApps["com.x"].path
	if err := os.WriteFile(filepath.Join(app, "Info.plist"), []byte("v2 build"), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := d.handleClearState("com.x"); !res.Success {
		t.Fatalf("clearState = %+v", res)
	}
	if d.stagedApps["com.x"].path == first || pathExists(first) {
		t.Error("a new build was not restaged, or the old copy was kept")
	}
	var installs int
	for _, c := range f.joined() {
		if strings.HasPrefix(c, "install SIM ") {
			installs++
		}
	}
	if installs != 3 {
		t.Errorf("installs = %d, want 3", installs)
	}
}

func TestClearStateFastWipe(t *testing.T) {
	data := t.TempDir()
	for _, p := range []string{"Documents/a.db", "Library/Preferences/x.plist", containerMetadata} {
		full := filepath.Join(data, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeSimctl{reply: func(args []string) (string, bool) {
		if args[0] == "get_app_container" {
			return data, false
		}
		return "", false
	}}
	f.install(t)
	t.Setenv(fastClearStateEnv, "1")
	if res := simDriver().handleClearState("com.x"); !res.Success {
		t.Fatalf("clearState = %+v", res)
	}
	if pathExists(filepath.Join(data, "Documents/a.db")) || pathExists(filepath.Join(data, "Library/Preferences/x.plist")) {
		t.Error("app data survived the wipe")
	}
	if !pathExists(filepath.Join(data, containerMetadata)) || !pathExists(filepath.Join(data, "tmp")) {
		t.Error("container metadata or standard directories missing")
	}
	for _, c := range f.joined() {
		if strings.HasPrefix(c, "uninstall") || strings.HasPrefix(c, "install") {
			t.Errorf("fast clearState reinstalled: %q", c)
		}
	}
}

func TestClearStateErrors(t *testing.T) {
	f := &fakeSimctl{reply: func([]string) (string, bool) { return "", true }}
	f.install(t)
	if res := (&Driver{}).handleClearState(""); res.Success {
		t.Error("clearState without an app passed")
	}
	if res := (&Driver{info: &core.PlatformInfo{}}).handleClearState("com.x"); res.Success {
		t.Error("clearState passed on a real device")
	}
	if res := simDriver().handleClearState("com.x"); res.Success {
		t.Error("clearState of a missing app passed")
	}
	t.Setenv(fastClearStateEnv, "1")
	if res := simDriver().handleClearState("com.x"); res.Success {
		t.Error("fast clearState of a missing app passed")
	}
}
