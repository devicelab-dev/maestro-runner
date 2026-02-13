package device

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// ============================================================
// Tests for pidPathFor
// ============================================================

func TestPidPathFor(t *testing.T) {
	tests := []struct {
		name       string
		socketPath string
		expected   string
	}{
		{
			name:       "standard socket path",
			socketPath: "/tmp/uia2-emulator-5554.sock",
			expected:   "/tmp/uia2-emulator-5554.pid",
		},
		{
			name:       "socket with different directory",
			socketPath: "/var/run/uia2-device123.sock",
			expected:   "/var/run/uia2-device123.pid",
		},
		{
			name:       "no extension",
			socketPath: "/tmp/uia2-emulator-5554",
			expected:   "/tmp/uia2-emulator-5554.pid",
		},
		{
			name:       "double extension",
			socketPath: "/tmp/uia2-device.backup.sock",
			expected:   "/tmp/uia2-device.backup.pid",
		},
		{
			name:       "just filename",
			socketPath: "socket.sock",
			expected:   "socket.pid",
		},
		{
			name:       "empty string",
			socketPath: "",
			expected:   ".pid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pidPathFor(tt.socketPath)
			if result != tt.expected {
				t.Errorf("pidPathFor(%q) = %q, want %q", tt.socketPath, result, tt.expected)
			}
		})
	}
}

// ============================================================
// Tests for IsOwnerAlive
// ============================================================

func TestIsOwnerAlive_NoPidFile(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")

	// No PID file exists -> should return false
	if IsOwnerAlive(socketPath) {
		t.Error("IsOwnerAlive should return false when PID file does not exist")
	}
}

func TestIsOwnerAlive_CurrentProcess(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	pidPath := pidPathFor(socketPath)

	// Write current process PID
	pid := os.Getpid()
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	// Current process is alive -> should return true
	if !IsOwnerAlive(socketPath) {
		t.Error("IsOwnerAlive should return true for current process PID")
	}
}

func TestIsOwnerAlive_DeadProcess(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	pidPath := pidPathFor(socketPath)

	// Write a PID that almost certainly does not exist
	// Use a very high PID number that is unlikely to be in use
	if err := os.WriteFile(pidPath, []byte("99999999"), 0644); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	// Dead process -> should return false
	if IsOwnerAlive(socketPath) {
		t.Error("IsOwnerAlive should return false for dead process PID")
	}
}

func TestIsOwnerAlive_InvalidPidContent(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	pidPath := pidPathFor(socketPath)

	// Write non-numeric content
	if err := os.WriteFile(pidPath, []byte("not-a-pid"), 0644); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	// Invalid PID content -> should return false
	if IsOwnerAlive(socketPath) {
		t.Error("IsOwnerAlive should return false for non-numeric PID file content")
	}
}

func TestIsOwnerAlive_EmptyPidFile(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	pidPath := pidPathFor(socketPath)

	// Write empty file
	if err := os.WriteFile(pidPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	// Empty PID file -> should return false
	if IsOwnerAlive(socketPath) {
		t.Error("IsOwnerAlive should return false for empty PID file")
	}
}

func TestIsOwnerAlive_PidWithWhitespace(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	pidPath := pidPathFor(socketPath)

	// Write PID with leading/trailing whitespace and newline
	pid := os.Getpid()
	if err := os.WriteFile(pidPath, []byte("  "+strconv.Itoa(pid)+"\n"), 0644); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	// Should handle whitespace gracefully and find the live process
	if !IsOwnerAlive(socketPath) {
		t.Error("IsOwnerAlive should return true when PID has surrounding whitespace")
	}
}

func TestIsOwnerAlive_NegativePid(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	pidPath := pidPathFor(socketPath)

	// Write negative PID
	if err := os.WriteFile(pidPath, []byte("-1"), 0644); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	// Negative PID -> FindProcess may succeed but Signal(0) should fail
	// The behavior varies by OS, but it should not return true for our use case
	result := IsOwnerAlive(socketPath)
	// On most systems, PID -1 refers to all processes, so Signal(0) might succeed
	// for root but fail for non-root. We just verify it does not panic.
	_ = result
}

func TestIsOwnerAlive_ZeroPid(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	pidPath := pidPathFor(socketPath)

	// Write PID 0
	if err := os.WriteFile(pidPath, []byte("0"), 0644); err != nil {
		t.Fatalf("failed to write PID file: %v", err)
	}

	// PID 0 -> should not panic; behavior depends on OS
	result := IsOwnerAlive(socketPath)
	_ = result // Just verifying no panic
}

// ============================================================
// Tests for findAPK
// ============================================================

func TestFindAPK_NoMatch(t *testing.T) {
	dir := t.TempDir()
	_, err := findAPK(dir, "nonexistent-*.apk")
	if err == nil {
		t.Error("expected error for no matching APK")
	}
}

func TestFindAPK_MatchesFile(t *testing.T) {
	dir := t.TempDir()
	apkPath := filepath.Join(dir, "test-v1.0.apk")
	if err := os.WriteFile(apkPath, []byte("fake apk"), 0644); err != nil {
		t.Fatalf("failed to create test APK: %v", err)
	}

	result, err := findAPK(dir, "test-v*.apk")
	if err != nil {
		t.Fatalf("findAPK failed: %v", err)
	}
	if result != apkPath {
		t.Errorf("findAPK returned %q, want %q", result, apkPath)
	}
}

// ============================================================
// Tests for checkHealthViaSocket and checkHealthViaTCP
// ============================================================

func TestCheckHealthViaSocket_NonExistent(t *testing.T) {
	result := checkHealthViaSocket("/tmp/nonexistent-health-check.sock")
	if result {
		t.Error("expected false for non-existent socket")
	}
}

func TestCheckHealthViaTCP_InvalidPort(t *testing.T) {
	result := checkHealthViaTCP(59998)
	if result {
		t.Error("expected false for port with no server")
	}
}
