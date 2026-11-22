package device

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// ShellTimeout kills an adb call that runs past its deadline and says so;
// one that finishes in time returns its output.
func TestShellTimeout(t *testing.T) {
	t.Cleanup(func() { execCommand = exec.Command })
	d := &AndroidDevice{serial: "x", adbPath: "adb"}

	execCommand = func(name string, args ...string) *exec.Cmd { return exec.Command("sh", "-c", "echo hi") }
	if out, err := d.ShellTimeout("echo hi", time.Second); err != nil || strings.TrimSpace(out) != "hi" {
		t.Fatalf("fast command: out=%q err=%v", out, err)
	}

	execCommand = func(name string, args ...string) *exec.Cmd { return exec.Command("sleep", "5") }
	start := time.Now()
	_, err := d.ShellTimeout("hang", 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("hung command: err=%v, want a timeout", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("timeout took %v, want about 200ms", time.Since(start))
	}
}

// withADBTimeout shortens the default adb limit for a test.
func withADBTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := adbTimeout
	adbTimeout = d
	t.Cleanup(func() { adbTimeout = prev })
}

// A plain adb call (Shell, forward, get-state...) that hangs is killed at
// adbTimeout, and the error names the adb command that timed out.
func TestADB_DefaultTimeoutNamesTheCommand(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) *exec.Cmd { return exec.Command("sleep", "5") })
	withADBTimeout(t, 200*time.Millisecond)
	d := &AndroidDevice{serial: "x", adbPath: "adb"}

	start := time.Now()
	_, err := d.Shell("getprop ro.product.model")
	if err == nil || !strings.Contains(err.Error(), "adb shell getprop ro.product.model: timed out after 200ms") {
		t.Fatalf("err = %v, want a timeout that names the adb command", err)
	}
	if !errors.Is(err, errTimedOut) {
		t.Errorf("err = %v, want it to wrap errTimedOut", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("timeout took %v, want about 200ms", time.Since(start))
	}
}

// A failing adb call still reports stderr, not a timeout.
func TestADB_FailureKeepsStderr(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "echo 'device offline' >&2; exit 1")
	})
	d := &AndroidDevice{serial: "x", adbPath: "adb"}

	_, err := d.Shell("ls")
	if err == nil || !strings.Contains(err.Error(), "device offline") || errors.Is(err, errTimedOut) {
		t.Fatalf("err = %v, want the stderr and no timeout", err)
	}
}

// install, push and pull move files of any size, so they get the longer
// transfer limit, not adbTimeout.
func TestADB_TransfersUseTheLongerLimit(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) *exec.Cmd { return exec.Command("sleep", "0.3") })
	withADBTimeout(t, 50*time.Millisecond)
	d := &AndroidDevice{serial: "x", adbPath: "adb"}

	if err := d.Install("/tmp/app.apk"); err != nil {
		t.Errorf("Install: %v, want no timeout", err)
	}
	if err := d.Push("/tmp/a", "/sdcard/a"); err != nil {
		t.Errorf("Push: %v, want no timeout", err)
	}
	if err := d.Pull("/sdcard/a", "/tmp/a"); err != nil {
		t.Errorf("Pull: %v, want no timeout", err)
	}
	if _, err := d.Shell("ls"); !errors.Is(err, errTimedOut) {
		t.Errorf("Shell: %v, want a timeout", err)
	}
}

// adb devices (device discovery) is bounded too.
func TestListDevices_Timeout(t *testing.T) {
	withFakeExec(t, func(name string, args ...string) *exec.Cmd { return exec.Command("sleep", "5") })
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/adb", nil })
	withADBTimeout(t, 100*time.Millisecond)

	if _, err := ListDevices(); err == nil || !strings.Contains(err.Error(), "adb devices: timed out") {
		t.Fatalf("err = %v, want an adb devices timeout", err)
	}
}

// runTimeout with no limit runs the command to the end.
func TestRunTimeout_NoLimit(t *testing.T) {
	if err := runTimeout(exec.Command("true"), 0, "true"); err != nil {
		t.Fatalf("err = %v", err)
	}
}
