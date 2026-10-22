package wda

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

const reuseUDID = "0B16AFCF-3F0A-4F6F-BEA4-D33D8D7D0228"

// withKeptDir points the kept-WDA state and lock files at a temp dir.
func withKeptDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := keptDir
	keptDir = func() string { return dir }
	t.Cleanup(func() { keptDir = prev })
	return dir
}

// withProcessCommand makes processCommand report cmdline for any pid.
func withProcessCommand(t *testing.T, cmdline string) {
	t.Helper()
	prev := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "ps" {
			return exec.Command("echo", cmdline)
		}
		return prev(name, args...)
	}
	t.Cleanup(func() { execCommand = prev })
}

// freePort is a port nothing listens on.
func freePort(t *testing.T) uint16 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return uint16(port)
}

func xcodebuildCmdline(udid string) string {
	return "/Applications/Xcode.app/Contents/Developer/usr/bin/xcodebuild test-without-building -xctestrun /x/WebDriverAgentRunner.xctestrun -destination platform=iOS Simulator,arch=arm64,id=" + udid + " -derivedDataPath /x/DerivedData"
}

func TestReuseRefusal(t *testing.T) {
	port := PortFromUDID(reuseUDID)
	kept := keptWDA{Port: port, Pid: 4242, BuildDir: "/cache/sim-ios18.5-iphone-wda16", BuildHash: "abc"}
	ours := xcodebuildCmdline(reuseUDID)

	if why := reuseRefusal(kept, port, kept.BuildDir, kept.BuildHash, ours, reuseUDID); why != "" {
		t.Fatalf("a matching kept WDA was refused: %s", why)
	}
	cases := []struct {
		name                  string
		port                  uint16
		buildDir, hash, cmdln string
		want                  string
	}{
		{"other port", port + 1, kept.BuildDir, "abc", ours, "port"},
		{"xcodebuild gone", port, kept.BuildDir, "abc", "", "no longer running"},
		{"pid reused by something else", port, kept.BuildDir, "abc", "/usr/bin/python3 server.py", "no longer running"},
		{"another simulator's xcodebuild", port, kept.BuildDir, "abc", xcodebuildCmdline("11111111-2222-3333-4444-555555555555"), "no longer running"},
		{"other build dir", port, "/cache/sim-ios26.0-iphone-wda16", "abc", ours, "built in"},
		{"rebuilt in place", port, kept.BuildDir, "def", ours, "different build"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			why := reuseRefusal(kept, c.port, c.buildDir, c.hash, c.cmdln, reuseUDID)
			if !strings.Contains(why, c.want) {
				t.Fatalf("refusal = %q, want it to mention %q", why, c.want)
			}
		})
	}
}

func TestIsOurXcodebuild(t *testing.T) {
	if !isOurXcodebuild(xcodebuildCmdline(reuseUDID), reuseUDID) {
		t.Fatal("the xcodebuild Start launches was not recognised")
	}
	device := "xcodebuild test-without-building -xctestrun /x/a.xctestrun -destination platform=iOS,id=00008101-001C0C660A13001E"
	if !isOurXcodebuild(device, "00008101-001C0C660A13001E") {
		t.Fatal("a real device's xcodebuild was not recognised")
	}
	if isOurXcodebuild("xcodebuild build-for-testing -destination id="+reuseUDID, reuseUDID) {
		t.Fatal("a build was taken for a running WDA")
	}
	if isOurXcodebuild(xcodebuildCmdline(reuseUDID), "") {
		t.Fatal("an empty UDID matched")
	}
}

func TestStatusReady(t *testing.T) {
	ready := map[string]interface{}{"value": map[string]interface{}{"ready": true}}
	if !statusReady(ready) {
		t.Fatal("a ready status was refused")
	}
	for _, s := range []map[string]interface{}{
		{"value": map[string]interface{}{"ready": false}},
		{"value": map[string]interface{}{}},
		{"value": "ok"},
		{},
	} {
		if statusReady(s) {
			t.Fatalf("%v was taken as ready", s)
		}
	}
}

func TestKeptStateRoundTrip(t *testing.T) {
	withKeptDir(t)
	if _, ok := loadKept(reuseUDID); ok {
		t.Fatal("state found before any was saved")
	}
	want := keptWDA{Port: 8123, Pid: 77, BuildDir: "/b", BuildHash: "h"}
	if err := saveKept(reuseUDID, want); err != nil {
		t.Fatal(err)
	}
	got, ok := loadKept(reuseUDID)
	if !ok || got != want {
		t.Fatalf("loaded %+v %v, want %+v", got, ok, want)
	}
	removeKept(reuseUDID)
	if _, ok := loadKept(reuseUDID); ok {
		t.Fatal("state still found after removal")
	}
}

func TestLockDeviceIsExclusive(t *testing.T) {
	withKeptDir(t)
	unlock, err := lockDevice(reuseUDID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockDevice(reuseUDID); err == nil {
		t.Fatal("a second run took a held device lock")
	}
	if _, err := lockDevice("11111111-2222-3333-4444-555555555555"); err != nil {
		t.Fatalf("another device's lock was refused: %v", err)
	}
	unlock()
	again, err := lockDevice(reuseUDID)
	if err != nil {
		t.Fatalf("the lock was not given back: %v", err)
	}
	again()
}

func TestKeptIdle(t *testing.T) {
	withKeptDir(t)
	if KeptIdle(reuseUDID) {
		t.Fatal("idle with nothing kept")
	}
	if err := saveKept(reuseUDID, keptWDA{Port: 8123, Pid: 77}); err != nil {
		t.Fatal(err)
	}
	if !KeptIdle(reuseUDID) {
		t.Fatal("a kept WDA no run holds was not idle")
	}
	unlock, err := lockDevice(reuseUDID)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if KeptIdle(reuseUDID) {
		t.Fatal("a kept WDA a run holds was idle")
	}
}

func TestKeepRunningLocksUntilRelease(t *testing.T) {
	withKeptDir(t)
	r := NewRunner(reuseUDID, "", "")
	if err := r.KeepRunning(); err != nil {
		t.Fatal(err)
	}
	if err := NewRunner(reuseUDID, "", "").KeepRunning(); err == nil {
		t.Fatal("two runs kept the same device")
	}
	r.Release()
	r.Release() // twice is harmless
	other := NewRunner(reuseUDID, "", "")
	if err := other.KeepRunning(); err != nil {
		t.Fatalf("the lock outlived Release: %v", err)
	}
	other.Cleanup()
}

func TestStopKeptForgetsAWDAThatIsNotOurs(t *testing.T) {
	withKeptDir(t)
	// The recorded pid now belongs to something else: it must not be
	// signalled, only forgotten.
	withProcessCommand(t, "/usr/bin/python3 server.py")
	if err := saveKept(reuseUDID, keptWDA{Port: freePort(t), Pid: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	StopKept(reuseUDID)
	if _, ok := loadKept(reuseUDID); ok {
		t.Fatal("state kept after StopKept")
	}
}

// fakeBuild lays out a cached build whose test bundle executable holds body.
func fakeBuild(t *testing.T, r *Runner, body string) {
	t.Helper()
	r.buildDir = t.TempDir()
	exe := filepath.Join(r.derivedDataPath(), "Build", "Products", "Debug-iphonesimulator",
		"WebDriverAgentRunner-Runner.app", "PlugIns", "WebDriverAgentRunner.xctest", "WebDriverAgentRunner")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildHashFollowsTheTestBundle(t *testing.T) {
	r := NewRunner(reuseUDID, "", "")
	fakeBuild(t, r, "one")
	h1, err := r.buildHash()
	if err != nil {
		t.Fatal(err)
	}
	fakeBuild(t, r, "two")
	h2, err := r.buildHash()
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 || len(h1) != 64 {
		t.Fatalf("hashes %q and %q", h1, h2)
	}
	r.buildDir = t.TempDir()
	if _, err := r.buildHash(); err == nil {
		t.Fatal("a build without a test bundle hashed")
	}
}

func TestReattachRefusesAnotherBuildAndForgetsIt(t *testing.T) {
	withKeptDir(t)
	withProcessCommand(t, "/usr/bin/python3 server.py") // never signalled
	r := NewRunner(reuseUDID, "", "")
	fakeBuild(t, r, "new build")
	r.port = freePort(t)
	if err := saveKept(reuseUDID, keptWDA{Port: r.port, Pid: os.Getpid(), BuildDir: r.buildDir, BuildHash: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Reattach(context.Background()); err == nil {
		t.Fatal("reattached to a WDA whose xcodebuild is gone")
	}
	if _, ok := loadKept(reuseUDID); ok {
		t.Fatal("a WDA that cannot be reused is still recorded")
	}
	if err := r.Reattach(context.Background()); err == nil || !strings.Contains(err.Error(), "no WebDriverAgent") {
		t.Fatalf("with nothing kept: %v", err)
	}
}

func TestProbeReady(t *testing.T) {
	var ready atomic.Bool
	ready.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ready.Load() {
			_, _ = w.Write([]byte(`{"value":{"ready":true,"message":"WebDriverAgent is ready to accept commands"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"value":{"ready":false}}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	p, _ := strconv.Atoi(u.Port())
	port := uint16(p)

	if err := probeReady(context.Background(), port); err != nil {
		t.Fatalf("a ready WDA was refused: %v", err)
	}
	ready.Store(false)
	if err := probeReady(context.Background(), port); err == nil {
		t.Fatal("an unready WDA was accepted")
	}
	if err := probeReady(context.Background(), freePort(t)); err == nil {
		t.Fatal("a port with no WDA was accepted")
	}
}
