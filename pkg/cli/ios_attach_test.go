package cli

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/urfave/cli/v2"

	wdadriver "github.com/devicelab-dev/maestro-runner/pkg/driver/wda"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// attachUDID is a made-up UDID; the CLI derives the WDA port from it.
const attachUDID = "00000000-0000000000A77AC4"

func fakeDeviceInfo(t *testing.T) {
	t.Helper()
	old := getIOSDeviceInfoFn
	getIOSDeviceInfoFn = func(string) (*iosDeviceInfo, error) {
		return &iosDeviceInfo{Name: "iPhone", OSVersion: "27.0"}, nil
	}
	t.Cleanup(func() { getIOSDeviceInfoFn = old })
}

// fakeWDA serves a minimal WebDriverAgent on the device's WDA port and records
// each request as "METHOD /path".
type fakeWDA struct {
	mu    sync.Mutex
	asked []string
}

func (f *fakeWDA) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asked...)
}

func startFakeWDA(t *testing.T) *fakeWDA {
	t.Helper()
	port := wdadriver.PortFromUDID(attachUDID)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Skipf("port %d is taken here: %v", port, err)
	}
	f := &fakeWDA{}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.asked = append(f.asked, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/status":
			_, _ = w.Write([]byte(`{"value":{"ready":true},"sessionId":null}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_, _ = w.Write([]byte(`{"value":{"sessionId":"attached-1","capabilities":{}},"sessionId":"attached-1"}`))
		default:
			_, _ = w.Write([]byte(`{"value":{"width":414,"height":896}}`))
		}
	}), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return f
}

// With --wda-attach the CLI uses the WDA already answering on the device's
// port: the bound port is not "device in use", nothing is built or started
// (xcodebuild is not on a CI machine, so a build would fail the call), and
// /status is the first thing asked.
func TestCreateIOSDriverAttachesToTheWDAOnTheDevicePort(t *testing.T) {
	fakeDeviceInfo(t)
	f := startFakeWDA(t)

	driver, cleanup, err := CreateIOSDriver(&RunConfig{Devices: []string{attachUDID}, Driver: "wda", NoAppInstall: true, AppID: "ai.example.app", WDAAttach: true})
	if err != nil {
		t.Fatalf("CreateIOSDriver: %v", err)
	}
	if driver == nil || cleanup == nil {
		t.Fatal("no driver or no cleanup")
	}
	cleanup()
	if asked := f.requests(); len(asked) == 0 || asked[0] != "GET /status" {
		t.Fatalf("the WDA was asked %q, want GET /status first", asked)
	}
}

// The attached WDA belongs to whatever started it: cleanup ends the run's
// session and leaves the WDA running.
func TestAttachedWDACleanupEndsOnlyTheSession(t *testing.T) {
	fakeDeviceInfo(t)
	f := startFakeWDA(t)

	driver, cleanup, err := CreateIOSDriver(&RunConfig{Devices: []string{attachUDID}, Driver: "wda", NoAppInstall: true, AppID: "ai.example.app", WDAAttach: true})
	if err != nil {
		t.Fatalf("CreateIOSDriver: %v", err)
	}
	_ = driver.Execute(&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}, AppID: "ai.example.app"})
	if !hasRequest(f.requests(), "POST /session") {
		t.Skipf("launchApp created no session here (%q), so there is nothing to end", f.requests())
	}
	cleanup()
	asked := f.requests()
	if !hasRequest(asked, "DELETE /session/attached-1") {
		t.Fatalf("cleanup did not end the session: %q", asked)
	}
	for _, r := range asked {
		if strings.HasPrefix(r, "DELETE") && r != "DELETE /session/attached-1" {
			t.Errorf("cleanup asked %q; only the session may be ended", r)
		}
	}
}

func hasRequest(asked []string, want string) bool {
	for _, r := range asked {
		if r == want {
			return true
		}
	}
	return false
}

func TestCreateIOSDriverAttachFailsWhenNoWDAAnswers(t *testing.T) {
	fakeDeviceInfo(t)
	old := wdadriver.AttachTimeout
	wdadriver.AttachTimeout = 200 * time.Millisecond
	t.Cleanup(func() { wdadriver.AttachTimeout = old })
	port := wdadriver.PortFromUDID(attachUDID)
	if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err != nil {
		t.Skipf("port %d is taken here: %v", port, err)
	} else {
		_ = ln.Close() // nothing listens there now
	}

	_, _, err := CreateIOSDriver(&RunConfig{Devices: []string{attachUDID}, Driver: "wda", NoAppInstall: true, AppID: "ai.example.app", WDAAttach: true})
	if err == nil {
		t.Fatal("CreateIOSDriver attached with no WDA answering")
	}
	if !strings.Contains(err.Error(), "--wda-attach") || !strings.Contains(err.Error(), fmt.Sprint(port)) {
		t.Fatalf("the error does not say the attach found no WDA on port %d: %v", port, err)
	}
}

// --parallel checks every device before it starts any. With --wda-attach the
// attached WDA's port is bound on purpose, so it is not "device in use" there
// either. Without it, it is.
func TestCheckDeviceAvailableAttachIgnoresTheBoundWDAPort(t *testing.T) {
	port := wdadriver.PortFromUDID(attachUDID)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Skipf("port %d is taken here: %v", port, err)
	}
	defer ln.Close()

	if err := checkDeviceAvailable(attachUDID, "ios", false); err == nil {
		t.Fatalf("without --wda-attach a bound port %d is not reported as in use", port)
	}
	if err := checkDeviceAvailable(attachUDID, "ios", true); err != nil {
		t.Fatalf("with --wda-attach the attached WDA's port reads as in use: %v", err)
	}
	if err := validateDevicesAvailable([]string{attachUDID}, "ios", true); err != nil {
		t.Fatalf("with --wda-attach --parallel refuses the device: %v", err)
	}
}

// --wda-attach is a WDA option: other iOS drivers ignore it.
func TestWDAAttachOnlyWithTheWDADriver(t *testing.T) {
	for driver, want := range map[string]bool{"wda": true, "WDA": true, "": false, "devicelab": false, "devicelab-legacy": false} {
		if got := wdaAttach(&RunConfig{WDAAttach: true, Driver: driver}); got != want {
			t.Errorf("wdaAttach with --driver %q = %v, want %v", driver, got, want)
		}
	}
	if wdaAttach(&RunConfig{Driver: "wda"}) {
		t.Error("wdaAttach without the flag = true")
	}
}

// A real device needs --team-id to sign WDA, unless the run attaches to a WDA
// something else signed; attaching names its device.
func TestCheckIOSSigning(t *testing.T) {
	cases := []struct {
		name    string
		cfg     RunConfig
		wantErr string
	}{
		{"real device, no team id", RunConfig{Driver: "wda", Devices: []string{attachUDID}}, "--team-id"},
		{"attach, device named, no team id", RunConfig{Driver: "wda", WDAAttach: true, Devices: []string{attachUDID}}, ""},
		{"attach, no device", RunConfig{Driver: "wda", WDAAttach: true}, "--device"},
		{"attach flag with another driver", RunConfig{Driver: "devicelab", WDAAttach: true, Devices: []string{attachUDID}}, "--team-id"},
	}
	for _, tc := range cases {
		err := checkIOSSigning(&tc.cfg, false)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: err = %v, want one naming %s", tc.name, err, tc.wantErr)
		}
	}
	if err := checkIOSSigning(&RunConfig{Driver: "wda"}, true); err != nil {
		t.Errorf("a simulator needs no team id: %v", err)
	}
}

// Auto-detection skips a device whose WDA port is bound, so attaching without
// --device says what is missing instead of "no device found".
func TestCreateIOSDriverAttachNeedsADevice(t *testing.T) {
	_, _, err := CreateIOSDriver(&RunConfig{Driver: "wda", WDAAttach: true, NoAppInstall: true})
	if err == nil || !strings.Contains(err.Error(), "--wda-attach needs the device named with --device") {
		t.Fatalf("err = %v, want the --wda-attach one asking for --device", err)
	}
}

func TestGlobalFlags_WDAAttach(t *testing.T) {
	for _, f := range GlobalFlags {
		if b, ok := f.(*cli.BoolFlag); ok && b.Name == "wda-attach" {
			for _, e := range b.EnvVars {
				if e == "MAESTRO_WDA_ATTACH" {
					return
				}
			}
			t.Fatalf("--wda-attach has env vars %v, want MAESTRO_WDA_ATTACH", b.EnvVars)
		}
	}
	t.Fatal("no --wda-attach flag in GlobalFlags")
}

// The flag and its env var reach RunConfig through buildDeviceRunConfig.
func TestBuildDeviceRunConfigReadsWDAAttach(t *testing.T) {
	run := func(t *testing.T, args ...string) bool {
		t.Helper()
		var got bool
		app := &cli.App{Flags: GlobalFlags, Action: func(c *cli.Context) error {
			cfg, err := buildDeviceRunConfig(c)
			if err != nil {
				return err
			}
			got = cfg.WDAAttach
			return nil
		}}
		if err := app.Run(append([]string{"maestro-runner"}, args...)); err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		return got
	}
	t.Setenv("MAESTRO_WDA_ATTACH", "")
	if run(t) {
		t.Error("WDAAttach set with neither the flag nor the env var")
	}
	if !run(t, "--wda-attach") {
		t.Error("--wda-attach did not set WDAAttach")
	}
	t.Setenv("MAESTRO_WDA_ATTACH", "1")
	if !run(t) {
		t.Error("MAESTRO_WDA_ATTACH=1 did not set WDAAttach")
	}
}
