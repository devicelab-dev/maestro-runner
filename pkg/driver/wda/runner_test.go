package wda

import (
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

	runner := NewRunner(udid, teamID)

	expectedPort := PortFromUDID(udid)
	if runner.Port() != expectedPort {
		t.Errorf("expected port %d, got %d", expectedPort, runner.Port())
	}
}

func TestNewRunner_StoresUDID(t *testing.T) {
	udid := "test-udid-1234"
	teamID := "TEAM123"

	runner := NewRunner(udid, teamID)

	if runner.deviceUDID != udid {
		t.Errorf("expected deviceUDID %q, got %q", udid, runner.deviceUDID)
	}
}

func TestNewRunner_StoresTeamID(t *testing.T) {
	udid := "test-udid-1234"
	teamID := "TEAM456"

	runner := NewRunner(udid, teamID)

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
	runner := &Runner{deviceUDID: "my-device-udid"}

	dest := runner.destination()
	expected := "id=my-device-udid"

	if dest != expected {
		t.Errorf("expected destination %q, got %q", expected, dest)
	}
}
