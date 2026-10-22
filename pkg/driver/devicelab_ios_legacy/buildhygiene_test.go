package devicelab_ios_legacy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseXcodeMajor(t *testing.T) {
	cases := map[string]int{
		"Xcode 26.2\nBuild version 17C52\n": 26,
		"Xcode 14.3.1\n":                    14,
		"garbage":                           0,
	}
	for in, want := range cases {
		if got := parseXcodeMajor(in); got != want {
			t.Errorf("parseXcodeMajor(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestTestWithoutBuildingArgs(t *testing.T) {
	got := strings.Join(testWithoutBuildingArgs("r.xctestrun", "dest", 26), " ")
	if !strings.HasSuffix(got, "-collect-test-diagnostics never") {
		t.Errorf("Xcode 26 args = %q, want diagnostics off", got)
	}
	if got := strings.Join(testWithoutBuildingArgs("r.xctestrun", "dest", 14), " "); strings.Contains(got, "diagnostics") {
		t.Errorf("Xcode 14 has no diagnostics flag, args = %q", got)
	}
}

// Each simulator gets its own xctestrun beside the built one, and the build
// lookup never mistakes a copy for the original.
func TestXctestrunPerSimulator(t *testing.T) {
	dir := t.TempDir()
	products := filepath.Join(dir, "Build/Products")
	if err := os.MkdirAll(products, 0o755); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(products, "Runner_iphonesimulator26.2-arm64.xctestrun")
	if err := os.WriteFile(base, []byte("plist"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := xctestrunForSimulator(base, "AAAA")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := xctestrunForSimulator(base, "BBBB")
	if a == b || filepath.Dir(a) != products {
		t.Errorf("copies %q and %q must differ and sit beside the original", a, b)
	}
	if data, _ := os.ReadFile(a); string(data) != "plist" {
		t.Errorf("copy content = %q", data)
	}
	if got, err := findXctestrun(dir); err != nil || got != base {
		t.Errorf("findXctestrun = %q, %v; want the original", got, err)
	}
	if err := os.Remove(base); err != nil {
		t.Fatal(err)
	}
	if _, err := findXctestrun(dir); err == nil {
		t.Error("only copies left, but a build was reported")
	}
	if _, err := xctestrunForSimulator(base, "CCCC"); err == nil {
		t.Error("copying a missing xctestrun succeeded")
	}
}

func TestLockFileSerialises(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slot.lock")
	unlock, err := lockFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan struct{})
	go func() {
		u, err := lockFile(path)
		if err == nil {
			u()
		}
		close(got)
	}()
	select {
	case <-got:
		t.Fatal("second lock taken while the first was held")
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("second lock not taken after unlock")
	}
	if _, err := lockFile(filepath.Join(path, "no", "such")); err == nil {
		t.Error("locking an impossible path succeeded")
	}
}

func TestHostAppInstalled(t *testing.T) {
	mkApp := func(plist string) string {
		app := filepath.Join(t.TempDir(), "Host.app")
		if err := os.MkdirAll(app, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(app, "Info.plist"), []byte(plist), 0o644); err != nil {
			t.Fatal(err)
		}
		return app
	}
	built := mkApp("v1")
	installed := mkApp("v1")
	f := &fakeSimctl{reply: func([]string) (string, bool) { return installed, false }}
	f.install(t)
	if !hostAppInstalled("SIM", "dev.devicelab.runner", built) {
		t.Error("the same build was not recognised")
	}
	if hostAppInstalled("SIM", "dev.devicelab.runner", mkApp("v2")) {
		t.Error("a different build was taken as installed")
	}
	if hostAppInstalled("SIM", "dev.devicelab.runner", filepath.Join(t.TempDir(), "missing.app")) {
		t.Error("a missing build was taken as installed")
	}
	missing := &fakeSimctl{reply: func([]string) (string, bool) { return "", true }}
	missing.install(t)
	if hostAppInstalled("SIM", "dev.devicelab.runner", built) {
		t.Error("an app not on the simulator was taken as installed")
	}
}

func TestApplySimulatorPrefs(t *testing.T) {
	f := &fakeSimctl{reply: func(args []string) (string, bool) {
		return "denied", args[4] == "com.apple.WebUI"
	}}
	f.install(t)
	ApplySimulatorPrefs("SIM")
	want := "spawn SIM defaults write com.apple.Accessibility ReduceMotionEnabled -int 1|" +
		"spawn SIM defaults write com.apple.WebUI AutoFillPasswords -int 0"
	if got := strings.Join(f.joined(), "|"); got != want {
		t.Errorf("calls = %q\nwant %q", got, want)
	}

	off := &fakeSimctl{}
	off.install(t)
	t.Setenv(SimPrefsEnv, "0")
	ApplySimulatorPrefs("SIM")
	if len(off.calls) != 0 {
		t.Errorf("opted out, still wrote %q", off.joined())
	}
}

// Xcode before 26 rejects arch= on a simulator destination (EAS pinned
// 1.1.15 on its Xcode 16 images for this), so the pin is gated on the version.
// Xcode 26 and later, and an unread version, keep it: Xcode 26's resolver
// lists a simulator under both arm64 and x86_64.
func TestSimulatorDestination(t *testing.T) {
	const udid = "SIM-UDID"
	cases := []struct {
		goarch string
		major  int
		want   string
	}{
		{"arm64", 14, "platform=iOS Simulator,id=SIM-UDID"},
		{"arm64", 15, "platform=iOS Simulator,id=SIM-UDID"},
		{"arm64", 16, "platform=iOS Simulator,id=SIM-UDID"},
		{"amd64", 16, "platform=iOS Simulator,id=SIM-UDID"},
		{"arm64", 25, "platform=iOS Simulator,id=SIM-UDID"},
		{"arm64", 26, "platform=iOS Simulator,arch=arm64,id=SIM-UDID"},
		{"amd64", 26, "platform=iOS Simulator,arch=x86_64,id=SIM-UDID"},
		{"arm64", 27, "platform=iOS Simulator,arch=arm64,id=SIM-UDID"},
		{"arm64", 0, "platform=iOS Simulator,arch=arm64,id=SIM-UDID"},
		{"amd64", 0, "platform=iOS Simulator,arch=x86_64,id=SIM-UDID"},
	}
	for _, c := range cases {
		if got := simulatorDestination(udid, c.goarch, c.major); got != c.want {
			t.Errorf("simulatorDestination(%s, Xcode %d) = %q, want %q", c.goarch, c.major, got, c.want)
		}
	}
}
