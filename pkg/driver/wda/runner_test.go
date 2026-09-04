// SPDX-FileCopyrightText: 2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package wda

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Tests for PortFromUDID function

func TestPortFromUDID_StandardUUID(t *testing.T) {
	// Standard iOS simulator UDID format: XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX
	// The last segment (12 hex chars) is used for port calculation
	udid := "12345678-1234-1234-1234-ABCDEF123456"

	port := PortFromUDID(udid)

	// Verify port is in expected range (8100-9099)
	if port < wdaBasePort || port >= wdaBasePort+wdaPortRange {
		t.Errorf("port %d outside expected range %d-%d", port, wdaBasePort, wdaBasePort+wdaPortRange-1)
	}
}

func TestPortFromUDID_Deterministic(t *testing.T) {
	// Same UDID should always produce same port
	udid := "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"

	port1 := PortFromUDID(udid)
	port2 := PortFromUDID(udid)

	if port1 != port2 {
		t.Errorf("same UDID produced different ports: %d vs %d", port1, port2)
	}
}

func TestPortFromUDID_DifferentUDIDs(t *testing.T) {
	// Different UDIDs should (usually) produce different ports
	// Note: Due to modulo, collisions are possible but unlikely for random UUIDs
	udid1 := "12345678-1234-1234-1234-000000000001"
	udid2 := "12345678-1234-1234-1234-000000000002"

	port1 := PortFromUDID(udid1)
	port2 := PortFromUDID(udid2)

	// These specific UDIDs should produce different ports
	if port1 == port2 {
		t.Errorf("different UDIDs produced same port: %d", port1)
	}
}

func TestPortFromUDID_NoHyphen(t *testing.T) {
	// UDID without hyphens should use entire string
	udid := "ABCDEF123456"

	port := PortFromUDID(udid)

	// Should still produce valid port in range
	if port < wdaBasePort || port >= wdaBasePort+wdaPortRange {
		t.Errorf("port %d outside expected range", port)
	}
}

func TestPortFromUDID_InvalidHex(t *testing.T) {
	// Non-hex UDID should fallback to base port
	udid := "invalid-udid-with-ZZZZZZZZZZZZ"

	port := PortFromUDID(udid)

	// Should fallback to base port 8100
	if port != wdaBasePort {
		t.Errorf("expected fallback to base port %d, got %d", wdaBasePort, port)
	}
}

func TestPortFromUDID_EmptyString(t *testing.T) {
	// Empty string should fallback to base port
	port := PortFromUDID("")

	if port != wdaBasePort {
		t.Errorf("expected fallback to base port %d, got %d", wdaBasePort, port)
	}
}

func TestPortFromUDID_PortRange(t *testing.T) {
	// Test several UDIDs to verify ports are always in valid range
	testUDIDs := []string{
		"00000000-0000-0000-0000-000000000000",
		"FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF",
		"12345678-ABCD-EF01-2345-6789ABCDEF01",
		"A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
	}

	for _, udid := range testUDIDs {
		port := PortFromUDID(udid)
		if port < wdaBasePort || port >= wdaBasePort+wdaPortRange {
			t.Errorf("UDID %q produced out-of-range port %d", udid, port)
		}
	}
}

func TestPortFromUDID_LastSegmentUsed(t *testing.T) {
	// Verify that only the last segment affects the port
	// These should produce different ports because last segments differ
	udid1 := "SAME-SAME-SAME-SAME-000000000001"
	udid2 := "SAME-SAME-SAME-SAME-000000000002"

	port1 := PortFromUDID(udid1)
	port2 := PortFromUDID(udid2)

	if port1 == port2 {
		t.Error("different last segments should produce different ports")
	}
}

func TestPortFromUDID_SameLastSegment(t *testing.T) {
	// UDIDs with same last segment should produce same port
	udid1 := "AAAA-BBBB-CCCC-DDDD-123456789ABC"
	udid2 := "XXXX-YYYY-ZZZZ-WWWW-123456789ABC"

	port1 := PortFromUDID(udid1)
	port2 := PortFromUDID(udid2)

	if port1 != port2 {
		t.Errorf("same last segment should produce same port, got %d vs %d", port1, port2)
	}
}

// Tests for NewRunner

func TestNewRunner_SetsPort(t *testing.T) {
	udid := "12345678-1234-1234-1234-ABCDEF123456"
	teamID := "ABC123DEF"

	runner := NewRunner(udid, teamID, "")

	expectedPort := PortFromUDID(udid)
	if runner.Port() != expectedPort {
		t.Errorf("expected port %d, got %d", expectedPort, runner.Port())
	}
}

func TestNewRunner_StoresUDID(t *testing.T) {
	udid := "test-udid-1234"
	teamID := "TEAM123"

	runner := NewRunner(udid, teamID, "")

	if runner.deviceUDID != udid {
		t.Errorf("expected deviceUDID %q, got %q", udid, runner.deviceUDID)
	}
}

func TestNewRunner_StoresTeamID(t *testing.T) {
	udid := "test-udid-1234"
	teamID := "TEAM456"

	runner := NewRunner(udid, teamID, "")

	if runner.teamID != teamID {
		t.Errorf("expected teamID %q, got %q", teamID, runner.teamID)
	}
}

func TestRunner_Port(t *testing.T) {
	runner := &Runner{port: 8150}

	if runner.Port() != 8150 {
		t.Errorf("expected Port() to return 8150, got %d", runner.Port())
	}
}

func TestRunner_Destination(t *testing.T) {
	// Fake UDID — isSimulator() runs `simctl get_app_container` which won't
	// match → we get the real-device branch ("platform=iOS,id=<udid>").
	// Pinning platform explicitly is the workaround for the Xcode 26 / iOS 26
	// destination-ambiguity bug that stalls test-without-building (the
	// resolver returns both arm64 and x86_64 entries for a sim UDID, picks
	// the wrong one, and testmanagerd never spawns the test bundle).
	runner := &Runner{deviceUDID: "my-device-udid"}
	dest := runner.destination()
	expected := "platform=iOS,id=my-device-udid"

	if dest != expected {
		t.Errorf("expected destination %q, got %q", expected, dest)
	}
}

// --- #118: fast-exiting xcodebuild must surface its real error, not "stalled" ---

func TestWaitForStartup_FastExitSurfacesRealError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "runner.log")
	logContent := "xcodebuild: error: Unable to find a device matching the provided destination specifier\n"
	if err := os.WriteFile(logPath, []byte(logContent), 0o644); err != nil {
		t.Fatal(err)
	}

	exit := make(chan error, 1)
	exit <- fmt.Errorf("exit status 70")

	r := &Runner{}
	err := r.waitForStartup(logPath, exit)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Unable to find a device") {
		t.Errorf("expected the real xcodebuild error to surface, got: %v", err)
	}
	var perm *permanentStartupError
	if !errors.As(err, &perm) {
		t.Errorf("expected permanentStartupError (skips retries), got %T: %v", err, err)
	}
}

func TestWaitForStartup_FastExitWithoutKnownErrorIsRetryable(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "runner.log")
	if err := os.WriteFile(logPath, []byte("some unrelated output\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	exit := make(chan error, 1)
	exit <- fmt.Errorf("signal: killed")

	r := &Runner{}
	err := r.waitForStartup(logPath, exit)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Errorf("expected exit-status error, got: %v", err)
	}
	var perm *permanentStartupError
	if errors.As(err, &perm) {
		t.Errorf("crash exits should stay retryable, got permanent error: %v", err)
	}
}

func TestWaitForStartup_ExitAfterReadyMarkerStillFails(t *testing.T) {
	// WDA runs inside xcodebuild: if the process exited, a ready marker in
	// the log must not be treated as success.
	logPath := filepath.Join(t.TempDir(), "runner.log")
	if err := os.WriteFile(logPath, []byte("ServerURLHere->http://127.0.0.1:8100\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	exit := make(chan error, 1)
	exit <- nil // exited with status 0

	r := &Runner{}
	err := r.waitForStartup(logPath, exit)
	if err == nil {
		t.Fatal("expected error when process exited, got nil")
	}
}

func TestCheckLog_XcodebuildErrorIsPermanent(t *testing.T) {
	r := &Runner{}
	err := r.checkLog("blah\nxcodebuild: error: Unable to find a device matching X\nmore", "/tmp/x.log")
	var perm *permanentStartupError
	if !errors.As(err, &perm) {
		t.Fatalf("expected permanentStartupError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "Unable to find a device matching X") {
		t.Errorf("expected error line surfaced, got: %v", err)
	}
}

// TestPortFromUDID_Legacy40CharUDID verifies that legacy 40-character
// hyphenless UDIDs get distinct in-range ports instead of all overflowing
// uint64 and falling back to 8100 (#129).
func TestPortFromUDID_Legacy40CharUDID(t *testing.T) {
	// Two real-shaped 40-hex-char UDIDs differing only in the tail.
	a := "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4"
	b := "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3e5"

	pa, pb := PortFromUDID(a), PortFromUDID(b)

	for _, p := range []uint16{pa, pb} {
		if p < wdaBasePort || p >= wdaBasePort+wdaPortRange {
			t.Errorf("legacy UDID produced out-of-range port %d", p)
		}
	}
	if pa == wdaBasePort && pb == wdaBasePort {
		t.Errorf("both legacy UDIDs fell back to base port %d — the #129 bug", wdaBasePort)
	}
	if pa == pb {
		t.Errorf("distinct legacy UDIDs collided on port %d", pa)
	}
}

// TestPortFromUDID_UUIDPortUnchanged pins that the bounded-tail change does not
// shift ports for standard UUIDs (their final segment is already 12 chars).
func TestPortFromUDID_UUIDPortUnchanged(t *testing.T) {
	udid := "00008101-001C0C660A13001E"
	seg := "001C0C660A13001E"[len("001C0C660A13001E")-12:] // last 12 of the final group
	val, _ := strconv.ParseUint(seg, 16, 64)
	want := wdaBasePort + uint16(val%uint64(wdaPortRange))
	if got := PortFromUDID(udid); got != want {
		t.Errorf("UUID port changed: got %d, want %d", got, want)
	}
}
