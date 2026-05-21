package simulator

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

const simctlListJSON = `{
  "devices": {
    "com.apple.CoreSimulator.SimRuntime.iOS-18-6": [
      {
        "udid": "AAAA-1111",
        "name": "iPhone 16 Pro",
        "state": "Shutdown",
        "isAvailable": true
      },
      {
        "udid": "BBBB-2222",
        "name": "iPhone 16",
        "state": "Booted",
        "isAvailable": true
      },
      {
        "udid": "CCCC-3333",
        "name": "Unavailable Device",
        "state": "Shutdown",
        "isAvailable": false
      }
    ],
    "com.apple.CoreSimulator.SimRuntime.tvOS-18-2": [
      {
        "udid": "DDDD-4444",
        "name": "Apple TV",
        "state": "Shutdown",
        "isAvailable": true
      }
    ]
  }
}`

// =============================================================================
// FindSimctlBinary
// =============================================================================

func TestFindSimctlBinary_Found(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/bin/xcrun", nil })
	path, err := FindSimctlBinary()
	if err != nil {
		t.Fatalf("FindSimctlBinary: %v", err)
	}
	if path != "/usr/bin/xcrun" {
		t.Errorf("got %q", path)
	}
}

func TestFindSimctlBinary_NotFound(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "", exec.ErrNotFound })
	if _, err := FindSimctlBinary(); err == nil {
		t.Error("expected error when xcrun missing")
	}
}

// =============================================================================
// ListSimulators
// =============================================================================

func TestListSimulators_HappyPath(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/bin/xcrun", nil })
	withFakeExec(t, fakeExec(simctlListJSON))

	sims, err := ListSimulators()
	if err != nil {
		t.Fatalf("ListSimulators: %v", err)
	}
	// Only IsAvailable=true entries returned. 3 of 4 in the JSON.
	if len(sims) != 3 {
		t.Errorf("expected 3 available sims, got %d", len(sims))
	}
}

func TestListSimulators_ExecError(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/bin/xcrun", nil })
	withFakeExec(t, fakeExecError())
	if _, err := ListSimulators(); err == nil {
		t.Error("expected error on simctl failure")
	}
}

func TestListSimulators_InvalidJSON(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/bin/xcrun", nil })
	withFakeExec(t, fakeExec("not-json"))
	if _, err := ListSimulators(); err == nil {
		t.Error("expected JSON parse error")
	}
}

func TestListSimulators_NoSimctl(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "", exec.ErrNotFound })
	if _, err := ListSimulators(); err == nil {
		t.Error("expected error when xcrun missing")
	}
}

// =============================================================================
// ListIOSSimulators — filters tvOS out
// =============================================================================

func TestListIOSSimulators(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/bin/xcrun", nil })
	withFakeExec(t, fakeExec(simctlListJSON))

	sims, err := ListIOSSimulators()
	if err != nil {
		t.Fatalf("ListIOSSimulators: %v", err)
	}
	// 2 available iOS sims; tvOS excluded.
	if len(sims) != 2 {
		t.Errorf("expected 2 iOS sims (tvOS excluded), got %d", len(sims))
	}
}

// =============================================================================
// ListShutdownSimulators / ListShutdownIOSSimulators
// =============================================================================

func TestListShutdownSimulators(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/bin/xcrun", nil })
	withFakeExec(t, fakeExec(simctlListJSON))

	sims, err := ListShutdownSimulators()
	if err != nil {
		t.Fatalf("ListShutdownSimulators: %v", err)
	}
	// AAAA-1111 (iOS shutdown) + DDDD-4444 (tvOS shutdown) = 2 shutdowns.
	if len(sims) != 2 {
		t.Errorf("expected 2 shutdown sims, got %d", len(sims))
	}
}

func TestListShutdownIOSSimulators(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/bin/xcrun", nil })
	withFakeExec(t, fakeExec(simctlListJSON))

	sims, err := ListShutdownIOSSimulators()
	if err != nil {
		t.Fatalf("ListShutdownIOSSimulators: %v", err)
	}
	// Only AAAA-1111 (iOS shutdown).
	if len(sims) != 1 {
		t.Errorf("expected 1 iOS shutdown sim, got %d", len(sims))
	}
}

// =============================================================================
// IsSimulator
// =============================================================================

func TestIsSimulator_Match(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/bin/xcrun", nil })
	withFakeExec(t, fakeExec(simctlListJSON))
	if !IsSimulator("AAAA-1111") {
		t.Error("expected known UDID to match")
	}
}

func TestIsSimulator_NoMatch(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/bin/xcrun", nil })
	withFakeExec(t, fakeExec(simctlListJSON))
	if IsSimulator("not-a-real-udid") {
		t.Error("expected unknown UDID to NOT match")
	}
}

func TestIsSimulator_ListFails(t *testing.T) {
	withFakeLookPath(t, func(s string) (string, error) { return "/usr/bin/xcrun", nil })
	withFakeExec(t, fakeExecError())
	if IsSimulator("AAAA-1111") {
		t.Error("expected false when list fails")
	}
}

// =============================================================================
// ShutdownSimulator
// =============================================================================

func TestShutdownSimulator_Success(t *testing.T) {
	withFakeExec(t, fakeExec("")) // simctl shutdown returns empty on success
	if err := ShutdownSimulator("AAAA-1111", 5*time.Second); err != nil {
		t.Errorf("ShutdownSimulator: %v", err)
	}
}

// ShutdownSimulator is intentionally forgiving — when simctl shutdown
// fails it falls through to polling CheckBootStatus, and when that also
// errors, it treats the simulator as already shut down and returns nil.
// That makes a clean "error" path hard to exercise without more elaborate
// mocking (would need CheckBootStatus to succeed and report Booted=true
// throughout the deadline window). Skipping.
