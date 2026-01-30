package emulator

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestIsEmulator(t *testing.T) {
	tests := []struct {
		name     string
		serial   string
		expected bool
	}{
		{"valid emulator", "emulator-5554", true},
		{"another emulator", "emulator-5556", true},
		{"physical device", "R5CR50ABCDE", false},
		{"empty serial", "", false},
		{"almost emulator", "emulator", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsEmulator(tt.serial)
			if result != tt.expected {
				t.Errorf("IsEmulator(%q) = %v, want %v", tt.serial, result, tt.expected)
			}
		})
	}
}

func TestGetAndroidHome(t *testing.T) {
	// Save original env vars
	origHome := os.Getenv("ANDROID_HOME")
	origSDKRoot := os.Getenv("ANDROID_SDK_ROOT")
	origSDKHome := os.Getenv("ANDROID_SDK_HOME")
	defer func() {
		os.Setenv("ANDROID_HOME", origHome)
		os.Setenv("ANDROID_SDK_ROOT", origSDKRoot)
		os.Setenv("ANDROID_SDK_HOME", origSDKHome)
	}()

	// Test ANDROID_HOME priority
	os.Setenv("ANDROID_HOME", "/path/to/android")
	os.Setenv("ANDROID_SDK_ROOT", "/other/path")
	result := getAndroidHome()
	if result != "/path/to/android" {
		t.Errorf("getAndroidHome() = %q, want %q", result, "/path/to/android")
	}

	// Test ANDROID_SDK_ROOT fallback
	os.Unsetenv("ANDROID_HOME")
	result = getAndroidHome()
	if result != "/other/path" {
		t.Errorf("getAndroidHome() = %q, want %q", result, "/other/path")
	}

	// Test no env vars
	os.Unsetenv("ANDROID_SDK_ROOT")
	os.Unsetenv("ANDROID_SDK_HOME")
	result = getAndroidHome()
	if result != "" {
		t.Errorf("getAndroidHome() = %q, want empty string", result)
	}
}

func TestBootStatus_IsFullyReady(t *testing.T) {
	tests := []struct {
		name     string
		status   BootStatus
		expected bool
	}{
		{
			name: "all ready",
			status: BootStatus{
				StateReady:     true,
				BootCompleted:  true,
				SettingsReady:  true,
				PackageManager: true,
			},
			expected: true,
		},
		{
			name: "missing state",
			status: BootStatus{
				StateReady:     false,
				BootCompleted:  true,
				SettingsReady:  true,
				PackageManager: true,
			},
			expected: false,
		},
		{
			name: "missing boot",
			status: BootStatus{
				StateReady:     true,
				BootCompleted:  false,
				SettingsReady:  true,
				PackageManager: true,
			},
			expected: false,
		},
		{
			name: "missing settings",
			status: BootStatus{
				StateReady:     true,
				BootCompleted:  true,
				SettingsReady:  false,
				PackageManager: true,
			},
			expected: false,
		},
		{
			name: "missing package manager",
			status: BootStatus{
				StateReady:     true,
				BootCompleted:  true,
				SettingsReady:  true,
				PackageManager: false,
			},
			expected: false,
		},
		{
			name:     "all false",
			status:   BootStatus{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.IsFullyReady()
			if result != tt.expected {
				t.Errorf("IsFullyReady() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFindEmulatorBinary_NoAndroidHome(t *testing.T) {
	// Save original env vars
	origHome := os.Getenv("ANDROID_HOME")
	origSDKRoot := os.Getenv("ANDROID_SDK_ROOT")
	origSDKHome := os.Getenv("ANDROID_SDK_HOME")
	origPath := os.Getenv("PATH")
	defer func() {
		os.Setenv("ANDROID_HOME", origHome)
		os.Setenv("ANDROID_SDK_ROOT", origSDKRoot)
		os.Setenv("ANDROID_SDK_HOME", origSDKHome)
		os.Setenv("PATH", origPath)
	}()

	// Clear all Android env vars and PATH
	os.Unsetenv("ANDROID_HOME")
	os.Unsetenv("ANDROID_SDK_ROOT")
	os.Unsetenv("ANDROID_SDK_HOME")
	os.Setenv("PATH", "/nonexistent/path")

	_, err := FindEmulatorBinary()
	if err == nil {
		t.Error("FindEmulatorBinary() should return error when ANDROID_HOME not set and emulator not in PATH")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error should mention 'not found', got: %v", err)
	}
}

func TestListAVDs_Integration(t *testing.T) {
	// This test only runs if ANDROID_HOME is set
	if os.Getenv("ANDROID_HOME") == "" {
		t.Skip("ANDROID_HOME not set, skipping integration test")
	}

	// Try to find emulator
	_, err := FindEmulatorBinary()
	if err != nil {
		t.Skipf("Emulator binary not found: %v", err)
	}

	// List AVDs
	avds, err := ListAVDs()
	if err != nil {
		t.Fatalf("ListAVDs() failed: %v", err)
	}

	// We might have 0 AVDs on CI, that's OK
	t.Logf("Found %d AVDs", len(avds))
	for _, avd := range avds {
		if avd.Name == "" {
			t.Error("AVD name should not be empty")
		}
	}
}

func TestManager_AllocatePort(t *testing.T) {
	mgr := NewManager()

	// First allocation should start at 5554
	port1 := mgr.AllocatePort("test-avd-1")
	if port1 != 5554 {
		t.Errorf("First allocation = %d, want 5554", port1)
	}

	// Same AVD should get same port
	port1Again := mgr.AllocatePort("test-avd-1")
	if port1Again != port1 {
		t.Errorf("Same AVD should get same port: got %d, want %d", port1Again, port1)
	}

	// Different AVD should get next port
	port2 := mgr.AllocatePort("test-avd-2")
	if port2 != 5556 {
		t.Errorf("Second AVD allocation = %d, want 5556", port2)
	}

	// Third AVD
	port3 := mgr.AllocatePort("test-avd-3")
	if port3 != 5558 {
		t.Errorf("Third AVD allocation = %d, want 5558", port3)
	}
}

func TestManager_GetNextPort(t *testing.T) {
	mgr := NewManager()

	tests := []struct {
		current  int
		expected int
	}{
		{5554, 5556},
		{5556, 5558},
		{5600, 5602},
	}

	for _, tt := range tests {
		result := mgr.getNextPort(tt.current)
		if result != tt.expected {
			t.Errorf("getNextPort(%d) = %d, want %d", tt.current, result, tt.expected)
		}
	}
}

func TestManager_IsStartedByUs(t *testing.T) {
	mgr := NewManager()

	// Initially no emulators
	if mgr.IsStartedByUs("emulator-5554") {
		t.Error("Should return false for unknown emulator")
	}

	// Add an emulator
	instance := &EmulatorInstance{
		AVDName:     "test-avd",
		Serial:      "emulator-5554",
		ConsolePort: 5554,
		ADBPort:     5555,
		StartedBy:   "maestro-runner",
		BootStart:   time.Now(),
	}
	mgr.started.Store("emulator-5554", instance)

	// Now should return true
	if !mgr.IsStartedByUs("emulator-5554") {
		t.Error("Should return true for tracked emulator")
	}

	// Different serial should be false
	if mgr.IsStartedByUs("emulator-5556") {
		t.Error("Should return false for different serial")
	}
}

func TestManager_GetStartedEmulators(t *testing.T) {
	mgr := NewManager()

	// Initially empty
	emulators := mgr.GetStartedEmulators()
	if len(emulators) != 0 {
		t.Errorf("Expected 0 emulators, got %d", len(emulators))
	}

	// Add some emulators
	serials := []string{"emulator-5554", "emulator-5556", "emulator-5558"}
	for i, serial := range serials {
		instance := &EmulatorInstance{
			AVDName:     "test-avd-" + serial,
			Serial:      serial,
			ConsolePort: 5554 + i*2,
			ADBPort:     5555 + i*2,
			StartedBy:   "maestro-runner",
			BootStart:   time.Now(),
		}
		mgr.started.Store(serial, instance)
	}

	// Get all started emulators
	emulators = mgr.GetStartedEmulators()
	if len(emulators) != len(serials) {
		t.Errorf("Expected %d emulators, got %d", len(serials), len(emulators))
	}

	// Check all serials are present
	found := make(map[string]bool)
	for _, serial := range emulators {
		found[serial] = true
	}
	for _, serial := range serials {
		if !found[serial] {
			t.Errorf("Missing serial %s in result", serial)
		}
	}
}

func TestManager_ShouldRetryOnError(t *testing.T) {
	mgr := NewManager()

	// Currently always returns false
	err := os.ErrNotExist
	if mgr.shouldRetryOnError(err) {
		t.Error("shouldRetryOnError should return false (not implemented yet)")
	}
}
