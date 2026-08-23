package device

import (
	"os/exec"
	"strings"
	"testing"
)

// fakeExec returns a function suitable for assigning to `execCommand`. It
// ignores the original command and runs `printf "%s" stdout` so callers
// that read cmd.Stdout get a deterministic payload. Use t.Cleanup to
// restore the original.
func fakeExec(stdout string) func(name string, args ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		return exec.Command("printf", "%s", stdout)
	}
}

// fakeExecError returns an exec.Cmd that exits with code 1 — for testing
// the error-handling branches.
func fakeExecError() func(name string, args ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}
}

func withFakeExec(t *testing.T, fn func(name string, args ...string) *exec.Cmd) {
	t.Helper()
	prev := execCommand
	execCommand = fn
	t.Cleanup(func() { execCommand = prev })
}

func withFakeLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	prev := execLookPath
	execLookPath = fn
	t.Cleanup(func() { execLookPath = prev })
}

// =============================================================================
// ListDevices — discover.go via execCommand seam
// =============================================================================

func TestListDevices_Single(t *testing.T) {
	withFakeExec(t, fakeExec(
		"List of devices attached\nABC123\tdevice\n",
	))
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/adb", nil })

	devices, err := ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Serial != "ABC123" || devices[0].State != "device" || devices[0].Type != "device" {
		t.Errorf("device fields: %+v", devices[0])
	}
}

func TestListDevices_Emulator(t *testing.T) {
	withFakeExec(t, fakeExec(
		"List of devices attached\nemulator-5554\tdevice\n",
	))
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/adb", nil })

	devices, err := ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 || devices[0].Type != "emulator" {
		t.Errorf("expected emulator type, got %+v", devices)
	}
}

func TestListDevices_Multiple(t *testing.T) {
	withFakeExec(t, fakeExec(
		"List of devices attached\nABC123\tdevice\nemulator-5554\tdevice\nXYZ789\toffline\n",
	))
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/adb", nil })

	devices, err := ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}
}

func TestListDevices_Empty(t *testing.T) {
	withFakeExec(t, fakeExec("List of devices attached\n"))
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/adb", nil })

	devices, err := ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
}

func TestListDevices_AdbExecError(t *testing.T) {
	withFakeExec(t, fakeExecError())
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/adb", nil })

	if _, err := ListDevices(); err == nil {
		t.Error("expected error when adb command fails")
	}
}

// =============================================================================
// FirstAvailable
// =============================================================================

// TestFirstAvailable_DeviceFound is intentionally NOT tested: when a
// `device`-state entry is found, FirstAvailable calls New(serial) which
// runs real ADB to connect. That's an integration boundary, not unit-
// testable via the exec seam without further mocking of the connect
// path. The "no devices" and "all offline" cases below exercise the
// loop and the ListDevices wrapper, which is enough for coverage.

func TestFirstAvailable_NoDevices(t *testing.T) {
	withFakeExec(t, fakeExec("List of devices attached\n"))
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/adb", nil })

	_, err := FirstAvailable()
	if err == nil {
		t.Error("expected error when no devices available")
	}
}

func TestFirstAvailable_AllOffline(t *testing.T) {
	withFakeExec(t, fakeExec("List of devices attached\nABC123\toffline\n"))
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/adb", nil })

	_, err := FirstAvailable()
	if err == nil {
		t.Error("expected error when all devices are offline")
	}
}

// =============================================================================
// detectDeviceSerial (android.go)
// =============================================================================

func TestDetectDeviceSerial_Found(t *testing.T) {
	withFakeExec(t, fakeExec("List of devices attached\nABC123\tdevice\n"))
	serial, err := detectDeviceSerial("/usr/local/bin/adb")
	if err != nil {
		t.Fatalf("detectDeviceSerial: %v", err)
	}
	if serial != "ABC123" {
		t.Errorf("got %q", serial)
	}
}

func TestDetectDeviceSerial_NoDevices(t *testing.T) {
	withFakeExec(t, fakeExec("List of devices attached\n"))
	// Make AVD list also empty so the suggestion section is exercised.
	withFakeLookPath(t, func(s string) (string, error) { return "", exec.ErrNotFound })

	_, err := detectDeviceSerial("/usr/local/bin/adb")
	if err == nil {
		t.Fatal("expected NoDevicesError")
	}
	if !strings.Contains(err.Error(), "No Android devices") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDetectDeviceSerial_AdbExecError(t *testing.T) {
	withFakeExec(t, fakeExecError())
	_, err := detectDeviceSerial("/usr/local/bin/adb")
	if err == nil {
		t.Error("expected error when adb command fails")
	}
}

// =============================================================================
// listAvailableAVDs (android.go)
// =============================================================================

func TestListAvailableAVDs_Found(t *testing.T) {
	withFakeExec(t, fakeExec("Pixel_4a_API_33\nNexus_5_API_28\n"))
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/" + s, nil })

	avds := listAvailableAVDs()
	if len(avds) != 2 {
		t.Errorf("expected 2 AVDs, got %v", avds)
	}
}

func TestListAvailableAVDs_NoEmulatorBinary(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "", exec.ErrNotFound })
	avds := listAvailableAVDs()
	if len(avds) != 0 {
		t.Errorf("expected empty AVD list when emulator not on PATH, got %v", avds)
	}
}

func TestListAvailableAVDs_ExecError(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/" + s, nil })
	withFakeExec(t, fakeExecError())

	avds := listAvailableAVDs()
	if len(avds) != 0 {
		t.Errorf("expected empty AVD list on exec error, got %v", avds)
	}
}
