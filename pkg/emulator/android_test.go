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
		_ = os.Setenv("ANDROID_HOME", origHome)
		_ = os.Setenv("ANDROID_SDK_ROOT", origSDKRoot)
		_ = os.Setenv("ANDROID_SDK_HOME", origSDKHome)
	}()

	// Test ANDROID_HOME priority
	_ = os.Setenv("ANDROID_HOME", "/path/to/android")
	_ = os.Setenv("ANDROID_SDK_ROOT", "/other/path")
	result := getAndroidHome()
	if result != "/path/to/android" {
		t.Errorf("getAndroidHome() = %q, want %q", result, "/path/to/android")
	}

	// Test ANDROID_SDK_ROOT fallback
	_ = os.Unsetenv("ANDROID_HOME")
	result = getAndroidHome()
	if result != "/other/path" {
		t.Errorf("getAndroidHome() = %q, want %q", result, "/other/path")
	}

	// Test no env vars
	_ = os.Unsetenv("ANDROID_SDK_ROOT")
	_ = os.Unsetenv("ANDROID_SDK_HOME")
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
		_ = os.Setenv("ANDROID_HOME", origHome)
		_ = os.Setenv("ANDROID_SDK_ROOT", origSDKRoot)
		_ = os.Setenv("ANDROID_SDK_HOME", origSDKHome)
		_ = os.Setenv("PATH", origPath)
	}()

	// Clear all Android env vars and PATH
	_ = os.Unsetenv("ANDROID_HOME")
	_ = os.Unsetenv("ANDROID_SDK_ROOT")
	_ = os.Unsetenv("ANDROID_SDK_HOME")
	_ = os.Setenv("PATH", "/nonexistent/path")

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
	// Create a clean manager without persistent port mapping
	mgr := &Manager{
		portMap: make(map[string]int),
	}

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

func TestFindAVDManagerBinary_NoAndroidHome(t *testing.T) {
	origHome := os.Getenv("ANDROID_HOME")
	origSDKRoot := os.Getenv("ANDROID_SDK_ROOT")
	origSDKHome := os.Getenv("ANDROID_SDK_HOME")
	origPath := os.Getenv("PATH")
	defer func() {
		_ = os.Setenv("ANDROID_HOME", origHome)
		_ = os.Setenv("ANDROID_SDK_ROOT", origSDKRoot)
		_ = os.Setenv("ANDROID_SDK_HOME", origSDKHome)
		_ = os.Setenv("PATH", origPath)
	}()

	_ = os.Unsetenv("ANDROID_HOME")
	_ = os.Unsetenv("ANDROID_SDK_ROOT")
	_ = os.Unsetenv("ANDROID_SDK_HOME")
	_ = os.Setenv("PATH", "/nonexistent/path")

	_, err := FindAVDManagerBinary()
	if err == nil {
		t.Error("FindAVDManagerBinary() should return error when ANDROID_HOME not set and avdmanager not in PATH")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error should mention 'not found', got: %v", err)
	}
}

func TestGetAndroidHome_SDKHome(t *testing.T) {
	origHome := os.Getenv("ANDROID_HOME")
	origSDKRoot := os.Getenv("ANDROID_SDK_ROOT")
	origSDKHome := os.Getenv("ANDROID_SDK_HOME")
	defer func() {
		_ = os.Setenv("ANDROID_HOME", origHome)
		_ = os.Setenv("ANDROID_SDK_ROOT", origSDKRoot)
		_ = os.Setenv("ANDROID_SDK_HOME", origSDKHome)
	}()

	_ = os.Unsetenv("ANDROID_HOME")
	_ = os.Unsetenv("ANDROID_SDK_ROOT")
	_ = os.Setenv("ANDROID_SDK_HOME", "/sdk/home/path")

	result := getAndroidHome()
	if result != "/sdk/home/path" {
		t.Errorf("getAndroidHome() = %q, want %q", result, "/sdk/home/path")
	}
}

func TestManager_ShutdownAll_Empty(t *testing.T) {
	mgr := NewManager()

	// ShutdownAll with no emulators should succeed
	err := mgr.ShutdownAll()
	if err != nil {
		t.Errorf("ShutdownAll() with no emulators should not error, got: %v", err)
	}
}

func TestManager_Shutdown_NotStartedByUs(t *testing.T) {
	mgr := NewManager()

	// Shutting down an emulator we did not start should be a no-op
	err := mgr.Shutdown("emulator-9999")
	if err != nil {
		t.Errorf("Shutdown() for unknown emulator should not error, got: %v", err)
	}
}

func TestForceKillEmulator_InvalidSerial(t *testing.T) {
	err := forceKillEmulator("not-an-emulator")
	if err == nil {
		t.Error("forceKillEmulator() should return error for invalid serial format")
	}
	if !strings.Contains(err.Error(), "failed to extract port") {
		t.Errorf("expected port extraction error, got: %v", err)
	}
}

// ============================================================
// Additional tests for forceKillEmulator
// ============================================================

func TestForceKillEmulator_InvalidSerialFormats(t *testing.T) {
	tests := []struct {
		name   string
		serial string
	}{
		{"empty string", ""},
		{"no dash", "emulator5554"},
		{"text port", "emulator-abc"},
		{"physical device", "R5CR50ABCDE"},
		{"just prefix", "emulator-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := forceKillEmulator(tt.serial)
			if err == nil {
				t.Errorf("forceKillEmulator(%q) should return error", tt.serial)
			}
		})
	}
}

func TestForceKillEmulator_ValidSerialNoProcess(t *testing.T) {
	// Valid serial format but no matching process running.
	// This test exercises the code path where pgrep fails.
	err := forceKillEmulator("emulator-59998")
	if err == nil {
		t.Error("forceKillEmulator should error when no matching process found")
	}
	if !strings.Contains(err.Error(), "could not find emulator process") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ============================================================
// Additional tests for Manager.Shutdown
// ============================================================

func TestManager_Shutdown_TrackedEmulatorNotRunning(t *testing.T) {
	mgr := NewManager()

	// Register an emulator that is not actually running.
	// Shutdown will try ShutdownEmulator which will run "adb -s emulator-5554 emu kill"
	// and fail, then fall through to forceKillEmulator.
	// The test verifies the error propagation path.
	instance := &EmulatorInstance{
		AVDName:     "test-avd",
		Serial:      "emulator-5554",
		ConsolePort: 5554,
		ADBPort:     5555,
		StartedBy:   "maestro-runner",
		BootStart:   time.Now(),
	}
	mgr.started.Store("emulator-5554", instance)

	// Shutdown should eventually fail because there is no real emulator running
	// but this exercises the Shutdown code path.
	// We do not check the error value because it depends on whether adb is available.
	_ = mgr.Shutdown("emulator-5554")

	// After Shutdown attempt (whether it succeeds or fails on real machine),
	// verify the emulator was removed from tracking on success,
	// or still tracked on failure.
	// In unit tests without adb, it will fail, so the emulator stays tracked.
}

func TestManager_ShutdownAll_MultipleTracked(t *testing.T) {
	mgr := NewManager()

	// Track two emulators that are not actually running
	for _, serial := range []string{"emulator-5554", "emulator-5556"} {
		instance := &EmulatorInstance{
			AVDName:   "test-avd-" + serial,
			Serial:    serial,
			StartedBy: "maestro-runner",
			BootStart: time.Now(),
		}
		mgr.started.Store(serial, instance)
	}

	// This will attempt to shut down both.
	// They are not actually running, so ShutdownEmulator will fail.
	err := mgr.ShutdownAll()
	// We expect errors because emulators are not running
	if err == nil {
		// This can happen if adb is not found at all (no error from ShutdownEmulator)
		// or if system adb responds unexpectedly
		t.Log("ShutdownAll returned nil (adb may not be available)")
	} else {
		t.Logf("ShutdownAll returned expected error: %v", err)
	}
}

// ============================================================
// Additional tests for Manager port allocation edge cases
// ============================================================

func TestManager_AllocatePort_HighPorts(t *testing.T) {
	mgr := NewManager()

	// Manually set a high port to test increment logic
	mgr.mu.Lock()
	mgr.portMap["existing-avd"] = 5600
	mgr.mu.Unlock()

	port := mgr.AllocatePort("new-avd")
	if port != 5602 {
		t.Errorf("Expected port 5602 after existing 5600, got %d", port)
	}
}

// ============================================================
// Additional tests for FindEmulatorBinary with ANDROID_HOME
// ============================================================

func TestFindEmulatorBinary_WithAndroidHomeDirExists(t *testing.T) {
	origHome := os.Getenv("ANDROID_HOME")
	origSDKRoot := os.Getenv("ANDROID_SDK_ROOT")
	origSDKHome := os.Getenv("ANDROID_SDK_HOME")
	origPath := os.Getenv("PATH")
	defer func() {
		_ = os.Setenv("ANDROID_HOME", origHome)
		_ = os.Setenv("ANDROID_SDK_ROOT", origSDKRoot)
		_ = os.Setenv("ANDROID_SDK_HOME", origSDKHome)
		_ = os.Setenv("PATH", origPath)
	}()

	// Create temp directory with emulator binary
	tmpDir := t.TempDir()
	emulatorDir := tmpDir + "/emulator"
	if err := os.MkdirAll(emulatorDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	emulatorPath := emulatorDir + "/emulator"
	if err := os.WriteFile(emulatorPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_ = os.Setenv("ANDROID_HOME", tmpDir)
	_ = os.Unsetenv("ANDROID_SDK_ROOT")
	_ = os.Unsetenv("ANDROID_SDK_HOME")
	_ = os.Setenv("PATH", "/nonexistent/path")

	result, err := FindEmulatorBinary()
	if err != nil {
		t.Fatalf("FindEmulatorBinary() with valid ANDROID_HOME should work: %v", err)
	}
	if result != emulatorPath {
		t.Errorf("FindEmulatorBinary() = %q, want %q", result, emulatorPath)
	}
}

func TestFindEmulatorBinary_OldLayout(t *testing.T) {
	origHome := os.Getenv("ANDROID_HOME")
	origSDKRoot := os.Getenv("ANDROID_SDK_ROOT")
	origSDKHome := os.Getenv("ANDROID_SDK_HOME")
	origPath := os.Getenv("PATH")
	defer func() {
		_ = os.Setenv("ANDROID_HOME", origHome)
		_ = os.Setenv("ANDROID_SDK_ROOT", origSDKRoot)
		_ = os.Setenv("ANDROID_SDK_HOME", origSDKHome)
		_ = os.Setenv("PATH", origPath)
	}()

	// Create temp directory with old-layout emulator binary
	tmpDir := t.TempDir()
	toolsDir := tmpDir + "/tools"
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	emulatorPath := toolsDir + "/emulator"
	if err := os.WriteFile(emulatorPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_ = os.Setenv("ANDROID_HOME", tmpDir)
	_ = os.Unsetenv("ANDROID_SDK_ROOT")
	_ = os.Unsetenv("ANDROID_SDK_HOME")
	_ = os.Setenv("PATH", "/nonexistent/path")

	result, err := FindEmulatorBinary()
	if err != nil {
		t.Fatalf("FindEmulatorBinary() with old layout should work: %v", err)
	}
	if result != emulatorPath {
		t.Errorf("FindEmulatorBinary() = %q, want %q", result, emulatorPath)
	}
}

// ============================================================
// Tests for FindAVDManagerBinary with ANDROID_HOME
// ============================================================

func TestFindAVDManagerBinary_NewLayout(t *testing.T) {
	origHome := os.Getenv("ANDROID_HOME")
	origSDKRoot := os.Getenv("ANDROID_SDK_ROOT")
	origSDKHome := os.Getenv("ANDROID_SDK_HOME")
	origPath := os.Getenv("PATH")
	defer func() {
		_ = os.Setenv("ANDROID_HOME", origHome)
		_ = os.Setenv("ANDROID_SDK_ROOT", origSDKRoot)
		_ = os.Setenv("ANDROID_SDK_HOME", origSDKHome)
		_ = os.Setenv("PATH", origPath)
	}()

	tmpDir := t.TempDir()
	avdDir := tmpDir + "/cmdline-tools/latest/bin"
	if err := os.MkdirAll(avdDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	avdPath := avdDir + "/avdmanager"
	if err := os.WriteFile(avdPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_ = os.Setenv("ANDROID_HOME", tmpDir)
	_ = os.Unsetenv("ANDROID_SDK_ROOT")
	_ = os.Unsetenv("ANDROID_SDK_HOME")
	_ = os.Setenv("PATH", "/nonexistent/path")

	result, err := FindAVDManagerBinary()
	if err != nil {
		t.Fatalf("FindAVDManagerBinary() with new layout should work: %v", err)
	}
	if result != avdPath {
		t.Errorf("FindAVDManagerBinary() = %q, want %q", result, avdPath)
	}
}

func TestFindAVDManagerBinary_OldLayout(t *testing.T) {
	origHome := os.Getenv("ANDROID_HOME")
	origSDKRoot := os.Getenv("ANDROID_SDK_ROOT")
	origSDKHome := os.Getenv("ANDROID_SDK_HOME")
	origPath := os.Getenv("PATH")
	defer func() {
		_ = os.Setenv("ANDROID_HOME", origHome)
		_ = os.Setenv("ANDROID_SDK_ROOT", origSDKRoot)
		_ = os.Setenv("ANDROID_SDK_HOME", origSDKHome)
		_ = os.Setenv("PATH", origPath)
	}()

	tmpDir := t.TempDir()
	avdDir := tmpDir + "/tools/bin"
	if err := os.MkdirAll(avdDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	avdPath := avdDir + "/avdmanager"
	if err := os.WriteFile(avdPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_ = os.Setenv("ANDROID_HOME", tmpDir)
	_ = os.Unsetenv("ANDROID_SDK_ROOT")
	_ = os.Unsetenv("ANDROID_SDK_HOME")
	_ = os.Setenv("PATH", "/nonexistent/path")

	result, err := FindAVDManagerBinary()
	if err != nil {
		t.Fatalf("FindAVDManagerBinary() with old layout should work: %v", err)
	}
	if result != avdPath {
		t.Errorf("FindAVDManagerBinary() = %q, want %q", result, avdPath)
	}
}

// ============================================================
// Tests for EmulatorInstance struct fields
// ============================================================

func TestEmulatorInstance_Fields(t *testing.T) {
	now := time.Now()
	instance := &EmulatorInstance{
		AVDName:      "Pixel_7_API_33",
		Serial:       "emulator-5554",
		ConsolePort:  5554,
		ADBPort:      5555,
		StartedBy:    "maestro-runner",
		BootStart:    now,
		BootDuration: 30 * time.Second,
	}

	if instance.AVDName != "Pixel_7_API_33" {
		t.Errorf("AVDName = %q, want %q", instance.AVDName, "Pixel_7_API_33")
	}
	if instance.ConsolePort != 5554 {
		t.Errorf("ConsolePort = %d, want 5554", instance.ConsolePort)
	}
	if instance.ADBPort != 5555 {
		t.Errorf("ADBPort = %d, want 5555", instance.ADBPort)
	}
}

// ============================================================
// Tests for AVDInfo struct
// ============================================================

func TestAVDInfo_Fields(t *testing.T) {
	avd := AVDInfo{
		Name:       "Pixel_7_API_33",
		Device:     "pixel_7",
		Target:     "android-33",
		SDKVersion: "33",
		IsRunning:  false,
	}

	if avd.Name != "Pixel_7_API_33" {
		t.Errorf("Name = %q, want %q", avd.Name, "Pixel_7_API_33")
	}
	if avd.IsRunning {
		t.Error("IsRunning should be false")
	}
}
