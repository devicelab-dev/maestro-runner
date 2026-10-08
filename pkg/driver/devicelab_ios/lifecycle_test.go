package devicelab_ios

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// fakeLaunch replaces simctlEnv for one test and records the launch.
func fakeLaunch(t *testing.T, fail error) *[]string {
	t.Helper()
	var got []string
	orig := simctlEnv
	simctlEnv = func(env []string, args ...string) (string, error) {
		got = append([]string{}, args...)
		for _, e := range env {
			if strings.HasPrefix(e, "SIMCTL_CHILD_") {
				got = append(got, e)
			}
		}
		return "", fail
	}
	t.Cleanup(func() { simctlEnv = orig })
	return &got
}

func TestLaunchAppOrder(t *testing.T) {
	d, _, sl := newTestDriver(t, nil)
	launched := fakeLaunch(t, nil)
	res := d.Execute(&flow.LaunchAppStep{
		AppID:         "com.example",
		ClearKeychain: true,
		Permissions:   map[string]string{"camera": "deny", "location": "unset"},
		Arguments:     map[string]any{"b": 2, "a": "x"},
		Environment:   map[string]string{"MODE": "test"},
	})
	if !res.Success {
		t.Fatal(res.Message)
	}
	want := []string{
		"privacy SIM-1 reset all com.example",
		"privacy SIM-1 revoke camera com.example",
		"keychain SIM-1 reset",
		"terminate SIM-1 com.example",
	}
	if strings.Join(sl.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("simctl calls:\n%s\nwant:\n%s", strings.Join(sl.calls, "\n"), strings.Join(want, "\n"))
	}
	if strings.Join(*launched, " ") != "launch --terminate-running-process SIM-1 com.example -a x -b 2 SIMCTL_CHILD_MODE=test" {
		t.Fatalf("launch = %v", *launched)
	}
	if d.appID != "com.example" {
		t.Fatal("launchApp should set the app id")
	}
}

func TestLaunchAppDefaultsAndErrors(t *testing.T) {
	d, _, sl := newTestDriver(t, nil)
	fakeLaunch(t, errBoom)
	if res := d.Execute(&flow.LaunchAppStep{}); res.Success {
		t.Fatal("no app id should fail")
	}
	res := d.Execute(&flow.LaunchAppStep{AppID: "com.x", StopApp: boolp(false)})
	if res.Success {
		t.Fatal("launch error should fail")
	}
	if !sl.has("privacy SIM-1 grant all com.x") {
		t.Fatalf("default all:allow missing: %v", sl.calls)
	}
	// A reset would lose grants that simctl cannot give back (contacts, notifications).
	if sl.has("privacy SIM-1 reset") {
		t.Fatalf("default all:allow must not reset permissions: %v", sl.calls)
	}
	if sl.has("terminate") {
		t.Fatal("stopApp: false must not terminate")
	}
}

func TestStopApp(t *testing.T) {
	d, _, sl := newTestDriver(t, nil)
	if res := d.Execute(&flow.StopAppStep{}); res.Success {
		t.Fatal("no app should fail")
	}
	d.SetAppID("com.x")
	sl.fail["terminate"] = errors.New("found nothing to terminate")
	if res := d.Execute(&flow.KillAppStep{}); !res.Success {
		t.Fatal("not running counts as stopped")
	}
	sl.fail["terminate"] = errBoom
	if res := d.Execute(&flow.StopAppStep{AppID: "com.y"}); res.Success {
		t.Fatal("real error should fail")
	}
}

func makeApp(t *testing.T) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), "My.app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Info.plist"), []byte("plist"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "My"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestClearStateReinstallsFromCachedCopy(t *testing.T) {
	d, _, sl := newTestDriver(t, nil)
	defer d.Close()
	app := makeApp(t)
	sl.answers["get_app_container SIM-1 com.x app"] = app + "\n"
	if res := d.Execute(&flow.ClearStateStep{AppID: "com.x"}); !res.Success {
		t.Fatal(res.Message)
	}
	first := d.stagedApps["com.x"].path
	if !pathExists(filepath.Join(first, "Info.plist")) || !sl.has("install SIM-1 "+first) || !sl.has("uninstall SIM-1 com.x") {
		t.Fatalf("calls = %v", sl.calls)
	}
	// Same build: the copy is reused.
	if res := d.Execute(&flow.ClearStateStep{AppID: "com.x"}); !res.Success || d.stagedApps["com.x"].path != first {
		t.Fatal("copy should be reused")
	}
	// A new build: a fresh copy.
	if err := os.WriteFile(filepath.Join(app, "My"), []byte("binary v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if res := d.Execute(&flow.ClearStateStep{AppID: "com.x"}); !res.Success || d.stagedApps["com.x"].path == first {
		t.Fatal("new build should be staged again")
	}
	if pathExists(first) {
		t.Fatal("old copy should be removed")
	}
}

func TestClearStateErrors(t *testing.T) {
	d, _, sl := newTestDriver(t, nil)
	if res := d.Execute(&flow.ClearStateStep{}); res.Success {
		t.Fatal("no app id")
	}
	if res := d.Execute(&flow.ClearStateStep{AppID: "com.none"}); res.Success {
		t.Fatal("not installed")
	}
	sl.answers["get_app_container SIM-1 com.x app"] = makeApp(t)
	sl.fail["uninstall"] = errBoom
	if res := d.Execute(&flow.ClearStateStep{AppID: "com.x"}); res.Success {
		t.Fatal("uninstall error")
	}
	delete(sl.fail, "uninstall")
	sl.fail["install"] = errBoom
	if res := d.Execute(&flow.ClearStateStep{AppID: "com.x"}); res.Success {
		t.Fatal("install error")
	}
	d.Close()
}

func TestFastClearState(t *testing.T) {
	t.Setenv(fastClearStateEnv, "1")
	d, _, sl := newTestDriver(t, nil)
	data := t.TempDir()
	for _, f := range []string{containerMetadata, "Documents/db.sqlite", "Library/Preferences/p.plist"} {
		p := filepath.Join(data, f)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("x"), 0o644)
	}
	sl.answers["get_app_container SIM-1 com.x data"] = data
	if res := d.Execute(&flow.ClearStateStep{AppID: "com.x"}); !res.Success {
		t.Fatal(res.Message)
	}
	if !pathExists(filepath.Join(data, containerMetadata)) || pathExists(filepath.Join(data, "Documents/db.sqlite")) ||
		!pathExists(filepath.Join(data, "tmp")) {
		t.Fatal("wipe kept the wrong things")
	}
	if !sl.has("privacy SIM-1 reset all com.x") || sl.has("uninstall") {
		t.Fatalf("calls = %v", sl.calls)
	}
	if res := d.Execute(&flow.ClearStateStep{AppID: "com.none"}); res.Success {
		t.Fatal("missing container should fail")
	}
	if err := wipeDataContainer(filepath.Join(data, "missing")); err == nil {
		t.Fatal("missing dir should fail")
	}
}

func TestKeychainAndPermissions(t *testing.T) {
	d, _, sl := newTestDriver(t, nil)
	d.SetAppID("com.x")
	if res := d.Execute(&flow.ClearKeychainStep{}); !res.Success {
		t.Fatal(res.Message)
	}
	sl.fail["keychain"] = errBoom
	if res := d.Execute(&flow.ClearKeychainStep{}); res.Success {
		t.Fatal("keychain error should fail")
	}
	if res := d.Execute(&flow.SetPermissionsStep{Permissions: map[string]string{"photos": "allow", "notifications": "allow"}}); !res.Success {
		t.Fatal(res.Message)
	}
	if !sl.has("privacy SIM-1 grant photos com.x") {
		t.Fatalf("calls = %v", sl.calls)
	}
	if res := d.Execute(&flow.SetPermissionsStep{}); res.Success {
		t.Fatal("empty permissions should fail")
	}
	sl.fail["privacy"] = errBoom
	if res := d.Execute(&flow.SetPermissionsStep{Permissions: map[string]string{"camera": "allow"}}); res.Success {
		t.Fatal("privacy error should fail")
	}
	d.SetAppID("")
	if res := d.Execute(&flow.SetPermissionsStep{Permissions: map[string]string{"camera": "allow"}}); res.Success {
		t.Fatal("no app id should fail")
	}
}

func TestOpenLinkAndLocation(t *testing.T) {
	d, fa, sl := newTestDriver(t, nil)
	if res := d.Execute(&flow.OpenLinkStep{Link: "app://home"}); !res.Success || !sl.has("openurl SIM-1 app://home") {
		t.Fatalf("res = %+v calls = %v", res, sl.calls)
	}
	if len(fa.sent("settle")) != 1 {
		t.Fatal("openLink should settle")
	}
	if res := d.Execute(&flow.OpenBrowserStep{}); res.Success {
		t.Fatal("empty link should fail")
	}
	if res := d.Execute(&flow.SetLocationStep{Latitude: "52.5", Longitude: "13.4"}); !res.Success || !sl.has("location SIM-1 set 52.500000,13.400000") {
		t.Fatalf("res = %+v calls = %v", res, sl.calls)
	}
	for _, s := range []*flow.SetLocationStep{{Latitude: "x", Longitude: "1"}, {Latitude: "1", Longitude: "y"}} {
		if res := d.Execute(s); res.Success {
			t.Fatalf("%+v should fail", s)
		}
	}
	sl.fail["openurl"] = errBoom
	sl.fail["location"] = errBoom
	if res := d.Execute(&flow.OpenLinkStep{Link: "x://"}); res.Success {
		t.Fatal("openurl error")
	}
	if res := d.Execute(&flow.SetLocationStep{Latitude: "1", Longitude: "1"}); res.Success {
		t.Fatal("location error")
	}
}

func TestOrientationAndDarkMode(t *testing.T) {
	appearance := "light"
	d, fa, _ := newTestDriver(t, func(cmd string, a Args) (*Response, error) {
		if cmd != "device" {
			return ok(nil), nil
		}
		if a.Action == "orientation" {
			return ok(&Payload{Orientation: "landscape"}), nil
		}
		if a.Value != "" && a.Value != "stuck" {
			appearance = a.Value
		}
		return ok(&Payload{Appearance: appearance}), nil
	})
	if res := d.Execute(&flow.SetOrientationStep{Orientation: "LANDSCAPE_LEFT"}); !res.Success {
		t.Fatal(res.Message)
	}
	if v := fa.sent("device")[0].Value; v != "landscape_left" {
		t.Fatalf("orientation value = %q", v)
	}
	if res := d.Execute(&flow.AssertLightModeStep{}); !res.Success {
		t.Fatal(res.Message)
	}
	if res := d.Execute(&flow.ToggleDarkModeStep{}); !res.Success || appearance != "dark" {
		t.Fatalf("toggle: %+v %s", res, appearance)
	}
	if res := d.Execute(&flow.AssertDarkModeStep{}); !res.Success {
		t.Fatal(res.Message)
	}
	if res := d.Execute(&flow.AssertLightModeStep{}); res.Success {
		t.Fatal("light assert in dark mode should fail")
	}
	if res := d.Execute(&flow.SetDarkModeStep{Enabled: false}); !res.Success || appearance != "light" {
		t.Fatal("set light")
	}
}

func TestDarkModeErrors(t *testing.T) {
	d, _, _ := newTestDriver(t, func(cmd string, a Args) (*Response, error) {
		if a.Action == "orientation" {
			return nil, errBoom
		}
		return ok(&Payload{}), nil // no appearance
	})
	if res := d.Execute(&flow.SetOrientationStep{Orientation: "portrait"}); res.Success {
		t.Fatal("orientation error")
	}
	for _, s := range []flow.Step{&flow.SetDarkModeStep{Enabled: true}, &flow.ToggleDarkModeStep{}, &flow.AssertDarkModeStep{}} {
		if res := d.Execute(s); res.Success {
			t.Errorf("%T with no appearance should fail", s)
		}
	}
	d2, _, _ := newTestDriver(t, func(string, Args) (*Response, error) { return ok(&Payload{Appearance: "light"}), nil })
	if res := d2.Execute(&flow.SetDarkModeStep{Enabled: true}); res.Success || !strings.Contains(res.Message, "light mode") {
		t.Fatalf("set that did not apply should fail: %+v", res)
	}
}

func TestAddMedia(t *testing.T) {
	d, _, sl := newTestDriver(t, nil)
	if res := d.Execute(&flow.AddMediaStep{Files: []string{"/nope/x.png"}}); res.Success {
		t.Fatal("missing file should fail")
	}
	img := filepath.Join(t.TempDir(), "a.png")
	_ = os.WriteFile(img, []byte("x"), 0o644)
	if res := d.Execute(&flow.AddMediaStep{Files: []string{img}}); !res.Success || !sl.has("addmedia SIM-1 "+img) {
		t.Fatalf("res = %+v calls = %v", res, sl.calls)
	}
	sl.fail["addmedia"] = errBoom
	if res := d.Execute(&flow.AddMediaStep{Files: []string{img}}); res.Success {
		t.Fatal("addmedia error")
	}
	if res := d.Execute(&flow.AddMediaStep{}); res.Success {
		t.Fatal("no files should fail")
	}
}

func TestApplySimulatorPrefsDisabled(t *testing.T) {
	t.Setenv(SimPrefsEnv, "0")
	ApplySimulatorPrefs("SIM-1") // must not run simctl
	ApplySimulatorPrefs("")
}

func TestLaunchEnvAndArgs(t *testing.T) {
	env := launchEnv(map[string]string{"B": "2", "A": "1"})
	n := len(env)
	if env[n-2] != "SIMCTL_CHILD_A=1" || env[n-1] != "SIMCTL_CHILD_B=2" {
		t.Fatalf("env tail = %v", env[n-2:])
	}
	if len(flattenArguments(nil)) != 0 {
		t.Fatal("nil args")
	}
}

// Launch arguments follow Maestro's iOS rule: a boolean keeps its key
// as written, another value gets a "-" prefix unless it already has one.
func TestFlattenArgumentsLikeMaestro(t *testing.T) {
	got := flattenArguments(map[string]any{
		"autoclear-ui-test": true,
		"cartValue":         3,
		"-cartColor":        "Orange",
		"isOnboarding":      "true", // a string, e.g. from ${ENV}
	})
	want := []string{"-cartColor", "Orange", "autoclear-ui-test", "true", "-cartValue", "3", "-isOnboarding", "true"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
