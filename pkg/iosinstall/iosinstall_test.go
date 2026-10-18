package iosinstall

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeTools replaces devicectl and go-ios for one test and records the calls.
type fakeTools struct {
	available    bool
	devicectlErr error
	goiosErr     error
	devicectl    [][]string
	goios        []string
	deadline     bool
}

func (f *fakeTools) install(t *testing.T) {
	t.Helper()
	oldAvail, oldRun, oldInst, oldUninst := devicectlAvailable, runDevicectl, goiosInstall, goiosUninstall
	t.Cleanup(func() {
		devicectlAvailable, runDevicectl, goiosInstall, goiosUninstall = oldAvail, oldRun, oldInst, oldUninst
	})
	devicectlAvailable = func() bool { return f.available }
	runDevicectl = func(ctx context.Context, args ...string) ([]byte, error) {
		_, f.deadline = ctx.Deadline()
		f.devicectl = append(f.devicectl, args)
		return []byte("devicectl output"), f.devicectlErr
	}
	goiosInstall = func(udid, appPath string) error {
		f.goios = append(f.goios, "install "+udid+" "+appPath)
		return f.goiosErr
	}
	goiosUninstall = func(udid, bundleID string) error {
		f.goios = append(f.goios, "uninstall "+udid+" "+bundleID)
		return f.goiosErr
	}
}

func TestInstallUsesDevicectlWhenItWorks(t *testing.T) {
	t.Setenv(installerEnv, "")
	f := &fakeTools{available: true}
	f.install(t)
	path, err := Install("UDID", "/a/App.app")
	if err != nil || path != Devicectl {
		t.Fatalf("Install = %q, %v; want devicectl, nil", path, err)
	}
	if got := strings.Join(f.devicectl[0], " "); got != "device install app --device UDID /a/App.app" {
		t.Errorf("devicectl args = %q", got)
	}
	if len(f.goios) != 0 {
		t.Errorf("go-ios was called: %v", f.goios)
	}
	if !f.deadline {
		t.Error("the devicectl call had no time limit")
	}
}

// Without Xcode on the host (a Linux machine the phone is plugged into)
// devicectl is missing and go-ios does the work.
func TestInstallWithoutDevicectlUsesGoIOS(t *testing.T) {
	t.Setenv(installerEnv, "")
	f := &fakeTools{available: false}
	f.install(t)
	path, err := Uninstall("UDID", "com.example")
	if err != nil || path != GoIOS {
		t.Fatalf("Uninstall = %q, %v; want go-ios, nil", path, err)
	}
	if len(f.devicectl) != 0 || len(f.goios) != 1 || f.goios[0] != "uninstall UDID com.example" {
		t.Errorf("devicectl calls %v, go-ios calls %v", f.devicectl, f.goios)
	}
}

// A devicectl that is installed but fails (no CoreDevice pairing, a locked
// phone) falls back to go-ios, for uninstall as for install.
func TestDevicectlFailureFallsBackToGoIOS(t *testing.T) {
	t.Setenv(installerEnv, "")
	for name, op := range map[string]func() (Path, error){
		"install":   func() (Path, error) { return Install("UDID", "/a/App.app") },
		"uninstall": func() (Path, error) { return Uninstall("UDID", "com.example") },
	} {
		f := &fakeTools{available: true, devicectlErr: errors.New("exit status 1")}
		f.install(t)
		path, err := op()
		if err != nil || path != GoIOS || len(f.devicectl) != 1 || len(f.goios) != 1 {
			t.Errorf("%s: path %q, err %v, devicectl calls %d, go-ios calls %d; want go-ios after one devicectl try",
				name, path, err, len(f.devicectl), len(f.goios))
		}
	}
}

func TestInstallerEnv(t *testing.T) {
	t.Run("zipconduit skips devicectl", func(t *testing.T) {
		t.Setenv(installerEnv, "zipconduit")
		f := &fakeTools{available: true}
		f.install(t)
		if path, err := Install("UDID", "/a/App.app"); err != nil || path != GoIOS || len(f.devicectl) != 0 {
			t.Fatalf("path %q, err %v, devicectl calls %d", path, err, len(f.devicectl))
		}
	})
	t.Run("devicectl does not fall back", func(t *testing.T) {
		t.Setenv(installerEnv, "devicectl")
		f := &fakeTools{available: true, devicectlErr: errors.New("exit status 1")}
		f.install(t)
		path, err := Install("UDID", "/a/App.app")
		if err == nil || path != Devicectl || len(f.goios) != 0 {
			t.Fatalf("path %q, err %v, go-ios calls %d; want devicectl's error and no fallback", path, err, len(f.goios))
		}
		if !strings.Contains(err.Error(), "devicectl output") {
			t.Errorf("error %q should carry devicectl's output", err)
		}
	})
}

func TestGoIOSErrorIsReturned(t *testing.T) {
	t.Setenv(installerEnv, "")
	f := &fakeTools{available: false, goiosErr: errors.New("device locked")}
	f.install(t)
	if _, err := Install("UDID", "/a/App.app"); err == nil || !strings.Contains(err.Error(), "go-ios install failed: device locked") {
		t.Fatalf("err = %v", err)
	}
}

// A devicectl call that runs past the limit is reported as a timeout.
func TestDevicectlTimeout(t *testing.T) {
	t.Setenv(installerEnv, "devicectl")
	old := timeout
	timeout = 20 * time.Millisecond
	t.Cleanup(func() { timeout = old })
	f := &fakeTools{available: true}
	f.install(t)
	runDevicectl = func(ctx context.Context, args ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if _, err := Install("UDID", "/a/App.app"); !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

// A go-ios call that never answers is abandoned at the limit, and its
// connection is closed, which unblocks the call so its goroutine ends.
func TestGoIOSCallThatNeverAnswersIsClosedAtTheTimeout(t *testing.T) {
	old := timeout
	timeout = 20 * time.Millisecond
	t.Cleanup(func() { timeout = old })

	closed := make(chan struct{})
	returned := make(chan struct{})
	err := withTimeout(func() error {
		<-closed
		close(returned)
		return errors.New("use of closed connection")
	}, func() { close(closed) })
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("the blocked call did not return after its connection was closed")
	}
}

// A go-ios call's connection is closed once, whether the call returns or
// times out.
func TestRunClosingClosesTheConnectionOnce(t *testing.T) {
	old := timeout
	timeout = 20 * time.Millisecond
	t.Cleanup(func() { timeout = old })

	closes := 0
	if err := runClosing(func() error { return nil }, func() { closes++ }); err != nil || closes != 1 {
		t.Fatalf("after a call that returned: err %v, closed %d times; want nil and 1", err, closes)
	}

	closes = 0
	closed := make(chan struct{})
	err := runClosing(func() error { <-closed; return errors.New("use of closed connection") }, func() {
		closes++
		close(closed)
	})
	if !errors.Is(err, ErrTimeout) || closes != 1 {
		t.Fatalf("after a call that hung: err %v, closed %d times; want ErrTimeout and 1", err, closes)
	}
}

// devicectl asked for but not installed: go-ios does the work, and the error
// of a failed fallback names both tools when devicectl was tried.
func TestForcedDevicectlThatIsMissingUsesGoIOS(t *testing.T) {
	t.Setenv(installerEnv, "devicectl")
	f := &fakeTools{available: false}
	f.install(t)
	if path, err := Install("UDID", "/a/App.app"); err != nil || path != GoIOS {
		t.Fatalf("Install = %q, %v; want go-ios, nil", path, err)
	}
}

func TestBothToolsFailingNamesBoth(t *testing.T) {
	t.Setenv(installerEnv, "")
	f := &fakeTools{available: true, devicectlErr: errors.New("exit status 1"), goiosErr: errors.New("device locked")}
	f.install(t)
	_, err := Install("UDID", "/a/App.app")
	if err == nil || !strings.Contains(err.Error(), "device locked") || !strings.Contains(err.Error(), "devicectl output") {
		t.Fatalf("err = %v; want both go-ios's and devicectl's errors", err)
	}
}
