package wda

import (
	"errors"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/iosinstall"
)

// fakeDeviceApps replaces iosinstall for one test and records the calls.
type fakeDeviceApps struct {
	calls        []string
	uninstallErr error
	installErr   error
	path         iosinstall.Path
}

func (f *fakeDeviceApps) install(t *testing.T) {
	t.Helper()
	oldUninstall, oldInstall := deviceUninstall, deviceInstall
	t.Cleanup(func() { deviceUninstall, deviceInstall = oldUninstall, oldInstall })
	deviceUninstall = func(udid, bundleID string) (iosinstall.Path, error) {
		f.calls = append(f.calls, "uninstall "+udid+" "+bundleID)
		return f.path, f.uninstallErr
	}
	deviceInstall = func(udid, appFile string) (iosinstall.Path, error) {
		f.calls = append(f.calls, "install "+udid+" "+appFile)
		return f.path, f.installErr
	}
}

func realDeviceDriver() *Driver {
	return &Driver{
		info:    &core.PlatformInfo{Platform: "ios", IsSimulator: false},
		udid:    "DEVICE-1",
		appFile: "/apps/App.ipa",
	}
}

// clearState on a real device uninstalls, then reinstalls, and says which
// tool did it.
func TestClearStateOnARealDeviceUninstallsThenReinstalls(t *testing.T) {
	f := &fakeDeviceApps{path: iosinstall.GoIOS}
	f.install(t)
	res := realDeviceDriver().clearAppStateDevice("com.example")
	if !res.Success {
		t.Fatalf("clearState failed: %s", res.Message)
	}
	want := []string{"uninstall DEVICE-1 com.example", "install DEVICE-1 /apps/App.ipa"}
	if strings.Join(f.calls, "; ") != strings.Join(want, "; ") {
		t.Errorf("calls = %v, want %v", f.calls, want)
	}
	if !strings.Contains(res.Message, "uninstall with go-ios, reinstall with go-ios") {
		t.Errorf("message %q should name the tool", res.Message)
	}
}

func TestClearStateStopsWhenTheUninstallFails(t *testing.T) {
	f := &fakeDeviceApps{path: iosinstall.GoIOS, uninstallErr: errors.New("go-ios uninstall failed: locked")}
	f.install(t)
	res := realDeviceDriver().clearAppStateDevice("com.example")
	if res.Success || len(f.calls) != 1 {
		t.Fatalf("success %v after calls %v; want a failure and no install", res.Success, f.calls)
	}
	if !strings.Contains(res.Message, "uninstall") {
		t.Errorf("message %q should say the uninstall failed", res.Message)
	}
}

func TestClearStateReportsAFailedReinstall(t *testing.T) {
	f := &fakeDeviceApps{path: iosinstall.Devicectl, installErr: errors.New("devicectl install failed")}
	f.install(t)
	res := realDeviceDriver().clearAppStateDevice("com.example")
	if res.Success || len(f.calls) != 2 {
		t.Fatalf("success %v after calls %v; want a failure after both calls", res.Success, f.calls)
	}
	if !strings.Contains(res.Message, "reinstall") {
		t.Errorf("message %q should say the reinstall failed", res.Message)
	}
}
