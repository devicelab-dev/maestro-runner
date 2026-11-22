package devicelab_ios

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"howett.net/plist"
)

// sampleXctestrun is the shape scripts/build.sh --device writes (format 1,
// paths under __TESTROOT__), trimmed.
const sampleXctestrun = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>DevicelabIOSAgentUITests</key>
	<dict>
		<key>BundleIdentifiersForCrashReportEmphasis</key>
		<array>
			<string>dev.devicelab.agent</string>
			<string>dev.devicelab.agent.uitests</string>
		</array>
		<key>DefaultTestExecutionTimeAllowance</key>
		<integer>600</integer>
		<key>EnvironmentVariables</key>
		<dict>
			<key>OS_ACTIVITY_DT_MODE</key>
			<string>YES</string>
		</dict>
		<key>IsUITestBundle</key>
		<true/>
		<key>TestBundlePath</key>
		<string>__TESTHOST__/PlugIns/DevicelabIOSAgentUITests.xctest</string>
		<key>TestHostBundleIdentifier</key>
		<string>dev.devicelab.agent.uitests.xctrunner</string>
		<key>TestHostPath</key>
		<string>__TESTROOT__/DevicelabIOSAgentUITests-Runner.app</string>
		<key>UITargetAppPath</key>
		<string>__TESTROOT__/DevicelabIOSAgent.app</string>
	</dict>
	<key>__xctestrun_metadata__</key>
	<dict>
		<key>FormatVersion</key>
		<integer>1</integer>
	</dict>
</dict>
</plist>`

func decodeXctestrun(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var doc map[string]interface{}
	if _, err := plist.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	target, ok := doc[testTargetName].(map[string]interface{})
	if !ok {
		t.Fatalf("no %s target in %s", testTargetName, raw)
	}
	return target
}

func TestRenderDeviceXctestrunInjectsPort(t *testing.T) {
	out, err := renderDeviceXctestrun([]byte(sampleXctestrun), 22517, agentBundleIDs(""))
	if err != nil {
		t.Fatal(err)
	}
	target := decodeXctestrun(t, out)
	env := target["EnvironmentVariables"].(map[string]interface{})
	if env["DL_AGENT_PORT"] != "22517" || env["OS_ACTIVITY_DT_MODE"] != "YES" {
		t.Fatalf("EnvironmentVariables = %v", env)
	}
	testingEnv := target["TestingEnvironmentVariables"].(map[string]interface{})
	if testingEnv["DL_AGENT_PORT"] != "22517" {
		t.Fatalf("TestingEnvironmentVariables = %v (created when missing)", testingEnv)
	}
	if target["TestHostBundleIdentifier"] != runnerBundleID {
		t.Fatalf("the runner id should stay the agent's: %v", target["TestHostBundleIdentifier"])
	}
	if target["TestHostPath"] != "__TESTROOT__/DevicelabIOSAgentUITests-Runner.app" {
		t.Fatalf("paths should stay relative to __TESTROOT__: %v", target["TestHostPath"])
	}
	s := string(out)
	if !strings.Contains(s, "<integer>600</integer>") || !strings.Contains(s, "<true/>") {
		t.Fatalf("value types should survive:\n%s", s)
	}
	if meta := string(out); !strings.Contains(meta, "__xctestrun_metadata__") {
		t.Fatal("metadata dropped")
	}
}

func TestRenderDeviceXctestrunCustomBundleID(t *testing.T) {
	out, err := renderDeviceXctestrun([]byte(sampleXctestrun), 22100, agentBundleIDs("com.jane.agent"))
	if err != nil {
		t.Fatal(err)
	}
	target := decodeXctestrun(t, out)
	if target["TestHostBundleIdentifier"] != "com.jane.agent.uitests.xctrunner" {
		t.Fatalf("TestHostBundleIdentifier = %v", target["TestHostBundleIdentifier"])
	}
	emph := target["BundleIdentifiersForCrashReportEmphasis"].([]interface{})
	if len(emph) != 2 || emph[0] != "com.jane.agent" || emph[1] != "com.jane.agent.uitests" {
		t.Fatalf("crash emphasis = %v", emph)
	}
}

func TestRenderDeviceXctestrunRejectsBadInput(t *testing.T) {
	if _, err := renderDeviceXctestrun([]byte("junk"), 1, agentBundleIDs("")); err == nil {
		t.Error("garbage should not render")
	}
	empty := `<plist version="1.0"><dict><key>__xctestrun_metadata__</key><dict/></dict></plist>`
	if _, err := renderDeviceXctestrun([]byte(empty), 1, agentBundleIDs("")); err == nil {
		t.Error("an xctestrun without a test target should be rejected")
	}
}

func TestWriteDeviceXctestrun(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.xctestrun"), []byte(sampleXctestrun), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := writeDeviceXctestrun(dir, 22333, agentBundleIDs(""))
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "agent-22333.xctestrun") {
		t.Fatalf("path = %s (must sit next to the products: __TESTROOT__ is its folder)", path)
	}
	raw, _ := os.ReadFile(path)
	env := decodeXctestrun(t, raw)["EnvironmentVariables"].(map[string]interface{})
	if env["DL_AGENT_PORT"] != "22333" {
		t.Fatalf("port = %v", env["DL_AGENT_PORT"])
	}
	if _, err := writeDeviceXctestrun(t.TempDir(), 1, agentBundleIDs("")); err == nil {
		t.Fatal("a missing template should fail")
	}
}

func TestBundledDeviceXctestrunRenders(t *testing.T) {
	path := filepath.Join("..", "..", "..", "drivers", "ios", "devicelab-ios-agent", "device", "agent.xctestrun")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skip("no bundled device agent in this checkout")
	}
	out, err := renderDeviceXctestrun(raw, 22444, agentBundleIDs(""))
	if err != nil {
		t.Fatal(err)
	}
	target := decodeXctestrun(t, out)
	if target["EnvironmentVariables"].(map[string]interface{})["DL_AGENT_PORT"] != "22444" {
		t.Fatal("port not set in the bundled template")
	}
	if strings.Contains(string(out), "iphonesimulator") || strings.Contains(string(out), "Release-iphoneos") {
		t.Fatal("the device template should name products under __TESTROOT__ directly")
	}
}

func TestCheckDeviceLog(t *testing.T) {
	cases := []struct {
		log       string
		want      string // "" = keep waiting
		permanent bool
	}{
		{"Writing result bundle\nTesting started\n", "", false},
		{"The application could not be launched because the Developer App Certificate is not trusted.", "VPN & Device Management", true},
		{"Unable to launch dev.devicelab.agent.uitests.xctrunner because it has an invalid code signature, inadequate entitlements or its profile has not been explicitly trusted by the user.", "Trust", true},
		{"error: Developer Mode disabled", "Developer Mode", true},
		{"The device is locked.", "unlock", true},
		{"ERROR: 0xe8008015 A valid provisioning profile for this executable was not found.", "re-sign", true},
		{"2026-09-29 xcodebuild[1:2] xcodebuild: error: Unable to find a device matching the provided destination specifier", "xcodebuild: error:", true},
		{"Testing failed:\n\tDevicelabIOSAgentUITests-Runner (123) encountered an error", "test session failed", false},
	}
	for _, c := range cases {
		err := checkDeviceLog(c.log)
		if c.want == "" {
			if err != nil {
				t.Errorf("checkDeviceLog(%q) = %v, want nil", c.log, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("checkDeviceLog(%q) = %v, want it to mention %q", c.log, err, c.want)
			continue
		}
		var perm *permanentStartError
		if errors.As(err, &perm) != c.permanent {
			t.Errorf("checkDeviceLog(%q): permanent = %v, want %v", c.log, !c.permanent, c.permanent)
		}
		if wrapped := withLog(err, "/tmp/x.log"); errors.As(wrapped, &perm) != c.permanent || !strings.Contains(wrapped.Error(), "/tmp/x.log") {
			t.Errorf("withLog changed %q: %v", c.log, wrapped)
		}
	}
}

// ---------- the driver on a real device ----------

// newDeviceDriver is newTestDriver switched to a real device, with devicectl
// recorded instead of run.
func newDeviceDriver(t *testing.T, appFile string, handler func(string, Args) (*Response, error)) (*Driver, *fakeAgent, *simctlLog, *[]string) {
	t.Helper()
	d, fa, sl := newTestDriver(t, handler)
	d.info = &core.PlatformInfo{Platform: "ios", IsSimulator: false, ScreenWidth: 400, ScreenHeight: 800}
	d.SetRealDevice(appFile)
	var devicectl []string
	orig := runDevicectl
	runDevicectl = func(args ...string) (string, error) {
		devicectl = append(devicectl, strings.Join(args, " "))
		return "", nil
	}
	t.Cleanup(func() { runDevicectl = orig })
	return d, fa, sl, &devicectl
}

func TestDeviceLaunchAppGoesThroughTheAgent(t *testing.T) {
	d, fa, sl, dc := newDeviceDriver(t, "/apps/My.app", nil)
	launched := fakeLaunch(t, nil)
	res := d.Execute(&flow.LaunchAppStep{
		AppID:         "com.example",
		ClearState:    true,
		ClearKeychain: true,
		Permissions:   map[string]string{"camera": "allow"},
		Arguments:     map[string]any{"b": 2, "a": "x"},
		Environment:   map[string]string{"MODE": "test"},
	})
	if !res.Success {
		t.Fatal(res.Message)
	}
	if len(sl.calls) != 0 || len(*launched) != 0 {
		t.Fatalf("no simctl on a device: %v %v", sl.calls, *launched)
	}
	want := []string{
		"device uninstall app --device SIM-1 com.example",
		"device install app --device SIM-1 /apps/My.app",
	}
	if strings.Join(*dc, "|") != strings.Join(want, "|") {
		t.Fatalf("devicectl = %v", *dc)
	}
	apps := fa.sent("app")
	if len(apps) < 2 {
		t.Fatalf("app calls = %+v", apps)
	}
	last := apps[len(apps)-1]
	if last.Action != "launch" || last.BundleID != "com.example" || strings.Join(last.Arguments, " ") != "-a x -b 2" || last.Environment["MODE"] != "test" {
		t.Fatalf("launch = %+v", last)
	}
	if prev := apps[len(apps)-2]; prev.Action != "terminate" {
		t.Fatalf("the app should be stopped before the launch: %+v", prev)
	}
	if d.appID != "com.example" {
		t.Fatal("launchApp should set the app id")
	}
}

func TestDeviceLaunchAppFailure(t *testing.T) {
	d, _, _, _ := newDeviceDriver(t, "", func(cmd string, a Args) (*Response, error) {
		if cmd == "app" && a.Action == "launch" {
			return nil, &AgentError{Code: "EXCEPTION", Message: "no such app"}
		}
		return ok(&Payload{}), nil
	})
	if res := d.Execute(&flow.LaunchAppStep{AppID: "com.example"}); res.Success || !strings.Contains(res.Message, "no such app") {
		t.Fatalf("res = %+v", res)
	}
}

func TestDeviceClearStateNeedsAppFile(t *testing.T) {
	d, _, _, dc := newDeviceDriver(t, "", nil)
	res := d.Execute(&flow.ClearStateStep{AppID: "com.example"})
	if res.Success || !strings.Contains(res.Message, "--app-file") {
		t.Fatalf("res = %+v", res)
	}
	if len(*dc) != 0 {
		t.Fatalf("nothing should be uninstalled without an app to reinstall: %v", *dc)
	}
}

func TestDeviceStopAppAndOpenLink(t *testing.T) {
	d, fa, sl, _ := newDeviceDriver(t, "", nil)
	if res := d.Execute(&flow.StopAppStep{AppID: "com.example"}); !res.Success {
		t.Fatal(res.Message)
	}
	if apps := fa.sent("app"); len(apps) != 1 || apps[0].Action != "terminate" || apps[0].BundleID != "com.example" {
		t.Fatalf("app calls = %+v", apps)
	}
	if res := d.Execute(&flow.OpenLinkStep{Link: "myapp://home"}); !res.Success {
		t.Fatal(res.Message)
	}
	if dev := fa.sent("device"); len(dev) != 1 || dev[0].Action != "openURL" || dev[0].Value != "myapp://home" {
		t.Fatalf("device calls = %+v", dev)
	}
	if len(sl.calls) != 0 {
		t.Fatalf("no simctl on a device: %v", sl.calls)
	}
}

func TestDeviceUnavailableSteps(t *testing.T) {
	d, _, sl, _ := newDeviceDriver(t, "", nil)
	steps := []flow.Step{
		&flow.ClearKeychainStep{},
		&flow.SetPermissionsStep{AppID: "com.example", Permissions: map[string]string{"camera": "allow"}},
		&flow.SetLocationStep{Latitude: "1", Longitude: "2"},
		&flow.AddMediaStep{Files: []string{"device_launch_test.go"}},
		&flow.SetClipboardStep{Text: "x"},
	}
	for _, s := range steps {
		res := d.Execute(s)
		if res.Success || !strings.Contains(res.Message, "real iPhone") {
			t.Errorf("%s: %+v", s.Type(), res)
		}
	}
	if len(sl.calls) != 0 {
		t.Fatalf("no simctl on a device: %v", sl.calls)
	}
	if err := d.StartScreenRecording(); err == nil || !strings.Contains(err.Error(), "real iPhone") {
		t.Fatalf("recording: %v", err)
	}
}

// stopApp: false leaves a running app running: XCTest's launch would restart
// it, so the agent brings it to the front instead.
func TestDeviceLaunchAppStopAppFalseActivatesRunningApp(t *testing.T) {
	for _, tc := range []struct {
		state, want string
	}{
		{"foreground", "activate"},
		{"background", "activate"},
		{"notRunning", "launch"},
	} {
		d, fa, _, _ := newDeviceDriver(t, "", func(cmd string, a Args) (*Response, error) {
			if cmd == "app" && a.Action == "state" {
				return ok(&Payload{AppState: tc.state}), nil
			}
			return ok(&Payload{}), nil
		})
		res := d.Execute(&flow.LaunchAppStep{AppID: "com.example", StopApp: boolp(false)})
		if !res.Success {
			t.Fatal(res.Message)
		}
		var actions []string
		for _, a := range fa.sent("app") {
			actions = append(actions, a.Action)
		}
		if got := strings.Join(actions, ","); got != "state,"+tc.want {
			t.Fatalf("%s: app actions = %s, want state,%s", tc.state, got, tc.want)
		}
	}
}
