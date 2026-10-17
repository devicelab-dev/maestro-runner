package wda

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// fakeDeviceApps replaces devicectl and go-ios for one test and records what was asked of them.
type fakeDeviceApps struct {
	calls        []string
	devicectl    bool
	uninstallErr error
	installErr   error
}

func (f *fakeDeviceApps) install(t *testing.T) {
	t.Helper()
	oldAvail, oldRun, oldUn, oldIn := devicectlAvailable, devicectlRun, goiosUninstall, goiosInstall
	t.Cleanup(func() {
		devicectlAvailable, devicectlRun, goiosUninstall, goiosInstall = oldAvail, oldRun, oldUn, oldIn
	})
	devicectlAvailable = func() bool { return f.devicectl }
	devicectlRun = func(args ...string) ([]byte, error) {
		f.calls = append(f.calls, "devicectl "+strings.Join(args, " "))
		return nil, nil
	}
	goiosUninstall = func(udid, bundleID string) error {
		f.calls = append(f.calls, "go-ios uninstall "+udid+" "+bundleID)
		return f.uninstallErr
	}
	goiosInstall = func(udid, appFile string) error {
		f.calls = append(f.calls, "go-ios install "+udid+" "+appFile)
		return f.installErr
	}
}

func deviceDriver() *Driver {
	return &Driver{udid: "UDID-1", appFile: "/builds/app.ipa", info: &core.PlatformInfo{Platform: "ios"}}
}

// Without devicectl (no Xcode on the host), clearState goes through go-ios, uninstall first.
func TestClearStateOnARealDeviceWithoutXcodeUsesGoIOS(t *testing.T) {
	f := &fakeDeviceApps{devicectl: false}
	f.install(t)

	result := deviceDriver().clearAppStateDevice("ai.example.app")
	if !result.Success {
		t.Fatalf("clearState failed: %s (%v)", result.Message, result.Error)
	}
	want := []string{"go-ios uninstall UDID-1 ai.example.app", "go-ios install UDID-1 /builds/app.ipa"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %q, want %q", f.calls, want)
	}
	if !strings.Contains(result.Message, "go-ios") {
		t.Errorf("the message does not say go-ios did it: %q", result.Message)
	}
}

func TestClearStateWithoutXcodeStopsWhenTheUninstallFails(t *testing.T) {
	f := &fakeDeviceApps{devicectl: false, uninstallErr: errors.New("proxy said no")}
	f.install(t)

	result := deviceDriver().clearAppStateDevice("ai.example.app")
	if result.Success {
		t.Fatal("clearState passed although the uninstall failed")
	}
	if !strings.Contains(result.Error.Error(), "go-ios uninstall failed: proxy said no") {
		t.Errorf("error = %v", result.Error)
	}
	if len(f.calls) != 1 {
		t.Errorf("installed after a failed uninstall: %q", f.calls)
	}
}

func TestClearStateWithoutXcodeReportsAFailedInstall(t *testing.T) {
	f := &fakeDeviceApps{devicectl: false, installErr: errors.New("conduit timed out")}
	f.install(t)

	result := deviceDriver().clearAppStateDevice("ai.example.app")
	if result.Success {
		t.Fatal("clearState passed although the install failed")
	}
	if !strings.Contains(result.Error.Error(), "go-ios install failed: conduit timed out") {
		t.Errorf("error = %v", result.Error)
	}
}

// With devicectl the Mac path is unchanged, and go-ios is never touched.
func TestClearStateWithXcodeStillUsesDevicectl(t *testing.T) {
	f := &fakeDeviceApps{devicectl: true}
	f.install(t)

	result := deviceDriver().clearAppStateDevice("ai.example.app")
	if !result.Success {
		t.Fatalf("clearState failed: %s (%v)", result.Message, result.Error)
	}
	want := []string{
		"devicectl device uninstall app --device UDID-1 ai.example.app",
		"devicectl device install app --device UDID-1 /builds/app.ipa",
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %q, want %q", f.calls, want)
	}
}

// A call that never answers is abandoned at the timeout, and its connection is closed, which
// unblocks the call so its goroutine ends.
func TestAGoIOSCallThatNeverAnswersIsClosedAtTheTimeout(t *testing.T) {
	old := goiosAppTimeout
	goiosAppTimeout = 50 * time.Millisecond
	t.Cleanup(func() { goiosAppTimeout = old })

	closed := make(chan struct{})
	returned := make(chan struct{})
	err := withAppTimeout(func() error {
		<-closed // blocks like an install service that never answers, until its connection closes
		close(returned)
		return errors.New("use of closed network connection")
	}, func() { close(closed) })

	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("the abandoned call is still blocked: its connection was not closed")
	}
}
