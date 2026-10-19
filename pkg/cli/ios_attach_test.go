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

// attachUDID is a made-up UDID; the CLI derives the device's WDA port from it.
const attachUDID = "00000000-0000000000A77AC4"

func fakeDeviceInfo(t *testing.T) {
	t.Helper()
	old := getIOSDeviceInfoFn
	getIOSDeviceInfoFn = func(string) (*iosDeviceInfo, error) {
		return &iosDeviceInfo{Name: "iPhone", OSVersion: "27.0"}, nil
	}
	t.Cleanup(func() { getIOSDeviceInfoFn = old })
}

// fakeWDA serves a minimal WebDriverAgent and records each request as
// "METHOD /path". hangDelete makes DELETE never answer.
type fakeWDA struct {
	mu         sync.Mutex
	asked      []string
	hangDelete bool
	url        string
}

func (f *fakeWDA) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asked...)
}

// startFakeWDA starts f on addr ("127.0.0.1:0" for any free port) and sets
// f.url.
func startFakeWDA(t *testing.T, f *fakeWDA, addr string) *fakeWDA {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("%s is taken here: %v", addr, err)
	}
	release := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.asked = append(f.asked, r.Method+" "+r.URL.Path)
		hang := f.hangDelete && r.Method == http.MethodDelete
		f.mu.Unlock()
		if hang {
			<-release
			return
		}
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
	t.Cleanup(func() { close(release); _ = srv.Close() })
	f.url = "http://" + ln.Addr().String()
	return f
}

func devicePortAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", wdadriver.PortFromUDID(attachUDID))
}

// With --wda-url the CLI drives the WDA at that URL, wherever it listens, and
// builds and starts nothing (xcodebuild is not on a CI machine, so a build
// would fail the call); /status is the first thing asked.
func TestCreateIOSDriverDrivesTheWDAAtTheURL(t *testing.T) {
	fakeDeviceInfo(t)
	f := startFakeWDA(t, &fakeWDA{}, "127.0.0.1:0")
	if strings.HasSuffix(f.url, fmt.Sprintf(":%d", wdadriver.PortFromUDID(attachUDID))) {
		t.Skip("the free port happens to be the device's WDA port")
	}

	driver, cleanup, err := CreateIOSDriver(&RunConfig{Devices: []string{attachUDID}, Driver: "wda", NoAppInstall: true, AppID: "ai.example.app", WDAURL: f.url})
	if err != nil {
		t.Fatalf("CreateIOSDriver: %v", err)
	}
	if driver == nil || cleanup == nil {
		t.Fatal("no driver or no cleanup")
	}
	cleanup()
	if asked := f.requests(); len(asked) == 0 || asked[0] != "GET /status" {
		t.Fatalf("the WDA at %s was asked %q, want GET /status first", f.url, asked)
	}
}

// A --wda-url on the device's own WDA port binds that port on purpose, so it
// is not "device in use". A --wda-url anywhere else leaves the check in place:
// a bound device port there is another run on the same device.
func TestCreateIOSDriverInUseCheckWithAWDAURL(t *testing.T) {
	fakeDeviceInfo(t)
	onDevicePort := startFakeWDA(t, &fakeWDA{}, devicePortAddr())
	if _, cleanup, err := CreateIOSDriver(&RunConfig{Devices: []string{attachUDID}, Driver: "wda", NoAppInstall: true, AppID: "ai.example.app", WDAURL: onDevicePort.url}); err != nil {
		t.Fatalf("a --wda-url on the device's own port: %v", err)
	} else {
		cleanup()
	}

	elsewhere := startFakeWDA(t, &fakeWDA{}, "127.0.0.1:0")
	_, _, err := CreateIOSDriver(&RunConfig{Devices: []string{attachUDID}, Driver: "wda", NoAppInstall: true, AppID: "ai.example.app", WDAURL: elsewhere.url})
	if err == nil || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("err = %v; with the device's port bound and --wda-url elsewhere, want \"in use\"", err)
	}
}

func TestURLIsLocalPort(t *testing.T) {
	for u, want := range map[string]bool{
		"http://127.0.0.1:8690": true,
		"http://localhost:8690": true,
		"http://[::1]:8690":     true,
		"http://127.0.0.1:8100": false,
		"http://10.0.0.5:8690":  false,
		"":                      false,
	} {
		if got := urlIsLocalPort(u, 8690); got != want {
			t.Errorf("urlIsLocalPort(%q, 8690) = %v, want %v", u, got, want)
		}
	}
}

// The WDA belongs to whatever started it: cleanup ends the run's session and
// leaves the WDA running.
func TestAttachedWDACleanupEndsOnlyTheSession(t *testing.T) {
	fakeDeviceInfo(t)
	f := startFakeWDA(t, &fakeWDA{}, "127.0.0.1:0")

	driver, cleanup, err := CreateIOSDriver(&RunConfig{Devices: []string{attachUDID}, Driver: "wda", NoAppInstall: true, AppID: "ai.example.app", WDAURL: f.url})
	if err != nil {
		t.Fatalf("CreateIOSDriver: %v", err)
	}
	_ = driver.Execute(&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}, AppID: "ai.example.app"})
	if !hasRequest(f.requests(), "POST /session") {
		t.Fatalf("launchApp created no session (%q), so the test cannot check that cleanup ends it", f.requests())
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

// Stopping a run cannot wait the client's minute on a WDA that no longer
// answers: ending the session has its own limit.
func TestAttachedWDACleanupIsBounded(t *testing.T) {
	fakeDeviceInfo(t)
	old := sessionEndTimeout
	sessionEndTimeout = 100 * time.Millisecond
	t.Cleanup(func() { sessionEndTimeout = old })
	f := startFakeWDA(t, &fakeWDA{hangDelete: true}, "127.0.0.1:0")

	driver, cleanup, err := CreateIOSDriver(&RunConfig{Devices: []string{attachUDID}, Driver: "wda", NoAppInstall: true, AppID: "ai.example.app", WDAURL: f.url})
	if err != nil {
		t.Fatalf("CreateIOSDriver: %v", err)
	}
	_ = driver.Execute(&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}, AppID: "ai.example.app"})
	start := time.Now()
	cleanup()
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("cleanup took %v against a WDA that never answers DELETE, want about 100ms", took)
	}
	if !hasRequest(f.requests(), "DELETE /session/attached-1") {
		t.Fatalf("cleanup did not try to end the session: %q", f.requests())
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
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + ln.Addr().String()
	_ = ln.Close() // nothing listens there now

	_, _, err = CreateIOSDriver(&RunConfig{Devices: []string{attachUDID}, Driver: "wda", NoAppInstall: true, AppID: "ai.example.app", WDAURL: url})
	if err == nil {
		t.Fatal("CreateIOSDriver attached with no WDA answering")
	}
	if !strings.Contains(err.Error(), "--wda-url") || !strings.Contains(err.Error(), url) {
		t.Fatalf("the error does not say no WDA answered at %s: %v", url, err)
	}
}

// --wda-url is a WDA option: other iOS drivers ignore it.
func TestWDAURLOnlyWithTheWDADriver(t *testing.T) {
	const u = "http://127.0.0.1:8100"
	for driver, want := range map[string]string{"wda": u, "WDA": u, "": "", "devicelab": "", "devicelab-legacy": ""} {
		if got := wdaURL(&RunConfig{WDAURL: u, Driver: driver}); got != want {
			t.Errorf("wdaURL with --driver %q = %q, want %q", driver, got, want)
		}
	}
	if got := wdaURL(&RunConfig{Driver: "wda"}); got != "" {
		t.Errorf("wdaURL without the flag = %q", got)
	}
}

// A real device needs --team-id to sign WDA, unless the run drives a WDA
// something else signed; that run names its device.
func TestCheckIOSSigning(t *testing.T) {
	const u = "http://127.0.0.1:8100"
	cases := []struct {
		name    string
		cfg     RunConfig
		wantErr string
	}{
		{"real device, no team id", RunConfig{Driver: "wda", Devices: []string{attachUDID}}, "--team-id"},
		{"--wda-url, device named, no team id", RunConfig{Driver: "wda", WDAURL: u, Devices: []string{attachUDID}}, ""},
		{"--wda-url, no device", RunConfig{Driver: "wda", WDAURL: u}, "--device"},
		{"--wda-url, two devices", RunConfig{Driver: "wda", WDAURL: u, Devices: []string{attachUDID, "00000000-0000000000B88BD5"}}, "one --device"},
		{"--wda-url with another driver", RunConfig{Driver: "devicelab", WDAURL: u, Devices: []string{attachUDID}}, "--team-id"},
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

// Auto-detection skips a device whose WDA port is bound, so --wda-url without
// --device says what is missing instead of "no device found".
func TestCreateIOSDriverWDAURLNeedsADevice(t *testing.T) {
	_, _, err := CreateIOSDriver(&RunConfig{Driver: "wda", WDAURL: "http://127.0.0.1:8100", NoAppInstall: true})
	if err == nil || !strings.Contains(err.Error(), "--wda-url needs the device named with --device") {
		t.Fatalf("err = %v, want the --wda-url one asking for --device", err)
	}
}

func TestCreateIOSDriverRejectsABadWDAURL(t *testing.T) {
	fakeDeviceInfo(t)
	_, _, err := CreateIOSDriver(&RunConfig{Devices: []string{attachUDID}, Driver: "wda", NoAppInstall: true, WDAURL: "127.0.0.1:8100"})
	if err == nil || !strings.Contains(err.Error(), "--wda-url") || !strings.Contains(err.Error(), "http://host:port") {
		t.Fatalf("err = %v, want one saying the URL needs http://host:port", err)
	}
}

func TestGlobalFlags_WDAURL(t *testing.T) {
	for _, f := range GlobalFlags {
		if sf, ok := f.(*cli.StringFlag); ok && sf.Name == "wda-url" {
			for _, e := range sf.EnvVars {
				if e == "MAESTRO_WDA_URL" {
					return
				}
			}
			t.Fatalf("--wda-url has env vars %v, want MAESTRO_WDA_URL", sf.EnvVars)
		}
	}
	t.Fatal("no --wda-url flag in GlobalFlags")
}

// The flag and its env var reach RunConfig through buildDeviceRunConfig.
func TestBuildDeviceRunConfigReadsWDAURL(t *testing.T) {
	run := func(t *testing.T, args ...string) string {
		t.Helper()
		var got string
		app := &cli.App{Flags: GlobalFlags, Action: func(c *cli.Context) error {
			cfg, err := buildDeviceRunConfig(c)
			if err != nil {
				return err
			}
			got = cfg.WDAURL
			return nil
		}}
		if err := app.Run(append([]string{"maestro-runner"}, args...)); err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		return got
	}
	t.Setenv("MAESTRO_WDA_URL", "")
	if got := run(t); got != "" {
		t.Errorf("WDAURL = %q with neither the flag nor the env var", got)
	}
	if got := run(t, "--wda-url", "http://127.0.0.1:8100"); got != "http://127.0.0.1:8100" {
		t.Errorf("--wda-url gave WDAURL %q", got)
	}
	t.Setenv("MAESTRO_WDA_URL", "http://10.0.0.5:8200")
	if got := run(t); got != "http://10.0.0.5:8200" {
		t.Errorf("MAESTRO_WDA_URL gave WDAURL %q", got)
	}
}
