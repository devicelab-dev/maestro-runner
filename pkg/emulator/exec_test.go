package emulator

import (
	"os/exec"
	"testing"
	"time"
)

func fakeExec(stdout string) func(name string, args ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		return exec.Command("printf", "%s", stdout)
	}
}

func fakeExecError() func(name string, args ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}
}

// fakeExecPerCmd dispatches stdout by command-args prefix match.
// First match wins. Use the FIRST argument (the binary name like "adb") plus
// the first sub-arg if you need to disambiguate. Unmatched calls run "false".
func fakeExecPerCmd(cases map[string]string) func(name string, args ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		key := name
		if len(args) > 0 {
			key = name + " " + args[0]
		}
		if out, ok := cases[key]; ok {
			return exec.Command("printf", "%s", out)
		}
		// Also try a 2-arg key (e.g., "adb -s") for finer-grained dispatch.
		if len(args) >= 2 {
			key2 := name + " " + args[0] + " " + args[1]
			if out, ok := cases[key2]; ok {
				return exec.Command("printf", "%s", out)
			}
		}
		// 3-arg key
		if len(args) >= 3 {
			key3 := name + " " + args[0] + " " + args[1] + " " + args[2]
			if out, ok := cases[key3]; ok {
				return exec.Command("printf", "%s", out)
			}
			// Also "adb shell getprop"
			key3alt := name + " " + args[2] + " " + args[3]
			_ = key3alt
		}
		// adb -s SERIAL shell <subcommand> ...
		if len(args) >= 4 && args[0] == "-s" && args[2] == "shell" {
			key4 := name + " shell " + args[3]
			if out, ok := cases[key4]; ok {
				return exec.Command("printf", "%s", out)
			}
		}
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
// ListAvailableAVDs
// =============================================================================

func TestListAVDs_Success(t *testing.T) {
	withFakeExec(t, fakeExec("Pixel_4a_API_33\nNexus_5_API_28\n\n"))
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/" + s, nil })

	avds, err := ListAVDs()
	if err != nil {
		t.Fatalf("ListAvailableAVDs: %v", err)
	}
	if len(avds) != 2 {
		t.Errorf("expected 2 AVDs, got %d (%+v)", len(avds), avds)
	}
}

func TestListAVDs_NoEmulatorBinary(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "", exec.ErrNotFound })
	_, err := ListAVDs()
	if err == nil {
		t.Error("expected error when emulator binary missing")
	}
}

func TestListAVDs_ExecError(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/emulator", nil })
	withFakeExec(t, fakeExecError())
	_, err := ListAVDs()
	if err == nil {
		t.Error("expected error when emulator -list-avds fails")
	}
}

func TestListAVDs_Empty(t *testing.T) {
	withFakeExec(t, fakeExec(""))
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/local/bin/" + s, nil })
	avds, err := ListAVDs()
	if err != nil {
		t.Fatalf("ListAvailableAVDs: %v", err)
	}
	if len(avds) != 0 {
		t.Errorf("expected no AVDs, got %v", avds)
	}
}

// =============================================================================
// RunningEmulatorPorts
// =============================================================================

func TestRunningEmulatorPorts(t *testing.T) {
	withFakeExec(t, fakeExec(
		"List of devices attached\nemulator-5554\tdevice\nemulator-5556\tdevice\nABC123\tdevice\n",
	))
	ports := RunningEmulatorPorts()
	if len(ports) != 2 {
		t.Errorf("expected 2 emulator ports, got %v", ports)
	}
}

func TestRunningEmulatorPorts_AdbError(t *testing.T) {
	withFakeExec(t, fakeExecError())
	if ports := RunningEmulatorPorts(); ports != nil {
		t.Errorf("expected nil ports on adb error, got %v", ports)
	}
}

// =============================================================================
// CheckBootStatus
// =============================================================================

func TestCheckBootStatus_Ready(t *testing.T) {
	// All three checks need to succeed. Use a dispatcher.
	withFakeExec(t, func(name string, args ...string) *exec.Cmd {
		// args: -s <serial> [get-state | shell ...]
		if len(args) >= 3 && args[2] == "get-state" {
			return exec.Command("printf", "%s", "device")
		}
		if len(args) >= 4 && args[2] == "shell" && args[3] == "getprop" {
			return exec.Command("printf", "%s", "1")
		}
		if len(args) >= 4 && args[2] == "shell" && args[3] == "settings" {
			return exec.Command("printf", "%s", "system")
		}
		if len(args) >= 4 && args[2] == "shell" && args[3] == "pm" {
			return exec.Command("printf", "%s", "4")
		}
		return exec.Command("false")
	})

	status, err := CheckBootStatus("emulator-5554")
	if err != nil {
		t.Fatalf("CheckBootStatus: %v", err)
	}
	if !status.StateReady {
		t.Error("StateReady should be true")
	}
	if !status.BootCompleted {
		t.Error("BootCompleted should be true")
	}
	if !status.SettingsReady {
		t.Error("SettingsReady should be true")
	}
}

func TestCheckBootStatus_DeviceNotReady(t *testing.T) {
	// get-state returns "offline" → StateReady=false, short-circuits early.
	withFakeExec(t, fakeExec("offline"))
	status, err := CheckBootStatus("emulator-5554")
	if err != nil {
		t.Fatalf("CheckBootStatus: %v", err)
	}
	if status.StateReady {
		t.Error("StateReady should be false when get-state != 'device'")
	}
}

func TestCheckBootStatus_GetStateErrors(t *testing.T) {
	withFakeExec(t, fakeExecError())
	status, err := CheckBootStatus("emulator-5554")
	if err != nil {
		t.Errorf("CheckBootStatus should not return error on get-state failure, got %v", err)
	}
	if status.StateReady {
		t.Error("StateReady should be false when get-state errors")
	}
}

// =============================================================================
// WaitForDeviceState
// =============================================================================

func TestWaitForDeviceState_AlreadyDevice(t *testing.T) {
	withFakeExec(t, fakeExec("device"))
	if err := WaitForDeviceState("emulator-5554", 100*time.Millisecond); err != nil {
		t.Errorf("WaitForDeviceState: %v", err)
	}
}

func TestWaitForDeviceState_Timeout(t *testing.T) {
	withFakeExec(t, fakeExec("offline"))
	if err := WaitForDeviceState("emulator-5554", 100*time.Millisecond); err == nil {
		t.Error("expected timeout error when state never matches")
	}
}
