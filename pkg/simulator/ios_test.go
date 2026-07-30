package simulator

import (
	"runtime"
	"testing"
	"time"
)

func TestSimulatorDevice_Fields(t *testing.T) {
	dev := SimulatorDevice{
		Name:        "iPhone 15 Pro",
		UDID:        "A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
		Runtime:     "com.apple.CoreSimulator.SimRuntime.iOS-17-2",
		OSVersion:   "17.2",
		State:       "Shutdown",
		IsAvailable: true,
	}

	if dev.Name != "iPhone 15 Pro" {
		t.Errorf("Name = %q, want %q", dev.Name, "iPhone 15 Pro")
	}
	if dev.UDID != "A1B2C3D4-E5F6-7890-ABCD-EF1234567890" {
		t.Errorf("UDID = %q, want %q", dev.UDID, "A1B2C3D4-E5F6-7890-ABCD-EF1234567890")
	}
	if dev.OSVersion != "17.2" {
		t.Errorf("OSVersion = %q, want %q", dev.OSVersion, "17.2")
	}
	if dev.State != "Shutdown" {
		t.Errorf("State = %q, want %q", dev.State, "Shutdown")
	}
	if !dev.IsAvailable {
		t.Error("IsAvailable = false, want true")
	}
}

func TestSimulatorInstance_Fields(t *testing.T) {
	now := time.Now()
	inst := SimulatorInstance{
		UDID:         "A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
		Name:         "iPhone 15 Pro",
		StartedBy:    "maestro-runner",
		BootStart:    now,
		BootDuration: 5 * time.Second,
	}

	if inst.UDID != "A1B2C3D4-E5F6-7890-ABCD-EF1234567890" {
		t.Errorf("UDID = %q", inst.UDID)
	}
	if inst.StartedBy != "maestro-runner" {
		t.Errorf("StartedBy = %q", inst.StartedBy)
	}
	if inst.BootDuration != 5*time.Second {
		t.Errorf("BootDuration = %v", inst.BootDuration)
	}
}

func TestBootStatus_IsReady(t *testing.T) {
	tests := []struct {
		name   string
		booted bool
		want   bool
	}{
		{"booted", true, true},
		{"not booted", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bs := &BootStatus{Booted: tt.booted}
			if got := bs.IsReady(); got != tt.want {
				t.Errorf("IsReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractOSVersion(t *testing.T) {
	tests := []struct {
		runtime string
		want    string
	}{
		{"com.apple.CoreSimulator.SimRuntime.iOS-17-2", "17.2"},
		{"com.apple.CoreSimulator.SimRuntime.iOS-18-0", "18.0"},
		{"com.apple.CoreSimulator.SimRuntime.iOS-16-4", "16.4"},
		{"com.apple.CoreSimulator.SimRuntime.watchOS-10-2", "10.2"},
		{"com.apple.CoreSimulator.SimRuntime.tvOS-17-0", "17.0"},
		{"com.apple.CoreSimulator.SimRuntime.xrOS-1-0", "1.0"},
		{"unknown-runtime", ""},
	}

	for _, tt := range tests {
		t.Run(tt.runtime, func(t *testing.T) {
			got := extractOSVersion(tt.runtime)
			if got != tt.want {
				t.Errorf("extractOSVersion(%q) = %q, want %q", tt.runtime, got, tt.want)
			}
		})
	}
}

func TestExtractPlatform(t *testing.T) {
	tests := []struct {
		runtime string
		want    string
	}{
		{"com.apple.CoreSimulator.SimRuntime.iOS-17-2", "iOS"},
		{"com.apple.CoreSimulator.SimRuntime.iOS-18-0", "iOS"},
		{"com.apple.CoreSimulator.SimRuntime.tvOS-17-0", "tvOS"},
		{"com.apple.CoreSimulator.SimRuntime.watchOS-10-2", "watchOS"},
		{"com.apple.CoreSimulator.SimRuntime.xrOS-1-0", "xrOS"},
		{"unknown-runtime", ""},
	}

	for _, tt := range tests {
		t.Run(tt.runtime, func(t *testing.T) {
			got := extractPlatform(tt.runtime)
			if got != tt.want {
				t.Errorf("extractPlatform(%q) = %q, want %q", tt.runtime, got, tt.want)
			}
		})
	}
}

func TestListSimulators_PopulatesPlatform(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("iOS simulator tests require macOS")
	}

	sims, err := ListSimulators()
	if err != nil {
		t.Fatalf("ListSimulators() error: %v", err)
	}

	for _, sim := range sims {
		if sim.Platform == "" {
			t.Errorf("SimulatorDevice %q (%s) has empty Platform (runtime: %s)", sim.Name, sim.UDID, sim.Runtime)
		}
	}
}

func TestListIOSSimulators_FiltersNonIOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("iOS simulator tests require macOS")
	}

	iosSims, err := ListIOSSimulators()
	if err != nil {
		t.Fatalf("ListIOSSimulators() error: %v", err)
	}

	for _, sim := range iosSims {
		if sim.Platform != "iOS" {
			t.Errorf("ListIOSSimulators() returned non-iOS sim: %q platform=%q runtime=%s", sim.Name, sim.Platform, sim.Runtime)
		}
	}

	// Verify it returns fewer or equal results compared to ListSimulators
	allSims, err := ListSimulators()
	if err != nil {
		t.Fatalf("ListSimulators() error: %v", err)
	}

	if len(iosSims) > len(allSims) {
		t.Errorf("ListIOSSimulators() returned %d sims but ListSimulators() returned %d", len(iosSims), len(allSims))
	}
}

func TestListShutdownIOSSimulators_FiltersNonIOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("iOS simulator tests require macOS")
	}

	sims, err := ListShutdownIOSSimulators()
	if err != nil {
		t.Fatalf("ListShutdownIOSSimulators() error: %v", err)
	}

	for _, sim := range sims {
		if sim.Platform != "iOS" {
			t.Errorf("ListShutdownIOSSimulators() returned non-iOS sim: %q platform=%q", sim.Name, sim.Platform)
		}
		if sim.State != "Shutdown" {
			t.Errorf("ListShutdownIOSSimulators() returned sim with state %q", sim.State)
		}
	}
}

func TestLatestIOSRuntime_Integration(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("simctl requires macOS")
	}

	rt, err := LatestIOSRuntime()
	if err != nil {
		t.Fatalf("LatestIOSRuntime() error: %v", err)
	}

	if rt.Identifier == "" {
		t.Error("LatestIOSRuntime().Identifier is empty")
	}
	if rt.Version == "" {
		t.Error("LatestIOSRuntime().Version is empty")
	}
	if len(rt.DeviceTypes) == 0 {
		t.Error("LatestIOSRuntime().DeviceTypes is empty")
	}

	// All device types should be iPhone identifiers
	for _, dt := range rt.DeviceTypes {
		if dt == "" {
			t.Error("empty device type identifier")
		}
	}
}

func TestCreateAndDeleteSimulator_Integration(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("simctl requires macOS")
	}

	rt, err := LatestIOSRuntime()
	if err != nil {
		t.Fatalf("LatestIOSRuntime() error: %v", err)
	}

	// Create a simulator
	udid, err := CreateSimulator("maestro-runner-test-tmp", rt.DeviceTypes[0], rt.Identifier)
	if err != nil {
		t.Fatalf("CreateSimulator() error: %v", err)
	}
	if udid == "" {
		t.Fatal("CreateSimulator() returned empty UDID")
	}

	// Verify it appears in the list
	sims, err := ListIOSSimulators()
	if err != nil {
		t.Fatalf("ListIOSSimulators() error: %v", err)
	}
	found := false
	for _, sim := range sims {
		if sim.UDID == udid {
			found = true
			if sim.Name != "maestro-runner-test-tmp" {
				t.Errorf("created sim name = %q, want %q", sim.Name, "maestro-runner-test-tmp")
			}
			break
		}
	}
	if !found {
		t.Errorf("created simulator %s not found in ListIOSSimulators()", udid)
	}

	// Clean up — delete it
	if err := DeleteSimulator(udid); err != nil {
		t.Fatalf("DeleteSimulator() error: %v", err)
	}
}

func TestManager_NewManager(t *testing.T) {
	mgr := NewManager()
	if mgr == nil {
		t.Fatal("NewManager() returned nil")
	}
}

func TestManager_IsStartedByUs(t *testing.T) {
	mgr := NewManager()

	// Not started — should return false
	if mgr.IsStartedByUs("fake-udid") {
		t.Error("IsStartedByUs should return false for unknown UDID")
	}

	// Manually store an instance
	mgr.started.Store("test-udid", &SimulatorInstance{
		UDID:      "test-udid",
		Name:      "iPhone 15",
		StartedBy: "maestro-runner",
	})

	if !mgr.IsStartedByUs("test-udid") {
		t.Error("IsStartedByUs should return true for tracked UDID")
	}
}

func TestManager_GetStartedSimulators(t *testing.T) {
	mgr := NewManager()

	// Empty
	if got := mgr.GetStartedSimulators(); len(got) != 0 {
		t.Errorf("GetStartedSimulators() = %v, want empty", got)
	}

	// Add two
	mgr.started.Store("udid-1", &SimulatorInstance{UDID: "udid-1"})
	mgr.started.Store("udid-2", &SimulatorInstance{UDID: "udid-2"})

	got := mgr.GetStartedSimulators()
	if len(got) != 2 {
		t.Errorf("GetStartedSimulators() returned %d, want 2", len(got))
	}
}

func TestManager_ShutdownAll_Empty(t *testing.T) {
	mgr := NewManager()
	if err := mgr.ShutdownAll(); err != nil {
		t.Errorf("ShutdownAll() on empty manager = %v, want nil", err)
	}
}

func TestManager_Shutdown_NotStartedByUs(t *testing.T) {
	mgr := NewManager()
	// Shutdown of unknown UDID is a no-op
	if err := mgr.Shutdown("unknown-udid"); err != nil {
		t.Errorf("Shutdown(unknown) = %v, want nil", err)
	}
}

// Integration tests — require macOS with Xcode

func TestListSimulators_Integration(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("iOS simulator tests require macOS")
	}

	sims, err := ListSimulators()
	if err != nil {
		t.Fatalf("ListSimulators() error: %v", err)
	}

	if len(sims) == 0 {
		t.Skip("No simulators available")
	}

	// Verify fields are populated
	for _, sim := range sims {
		if sim.Name == "" {
			t.Error("SimulatorDevice.Name is empty")
		}
		if sim.UDID == "" {
			t.Error("SimulatorDevice.UDID is empty")
		}
		if sim.State == "" {
			t.Error("SimulatorDevice.State is empty")
		}
	}
}

func TestFindSimctlBinary_Integration(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("simctl requires macOS")
	}

	path, err := FindSimctlBinary()
	if err != nil {
		t.Fatalf("FindSimctlBinary() error: %v", err)
	}
	if path == "" {
		t.Error("FindSimctlBinary() returned empty path")
	}
}

func TestListShutdownSimulators_Integration(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("iOS simulator tests require macOS")
	}

	sims, err := ListShutdownSimulators()
	if err != nil {
		t.Fatalf("ListShutdownSimulators() error: %v", err)
	}

	// All returned should be shutdown
	for _, sim := range sims {
		if sim.State != "Shutdown" {
			t.Errorf("ListShutdownSimulators() returned sim with state %q", sim.State)
		}
	}
}

func TestCheckBootStatus_Integration(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("iOS simulator tests require macOS")
	}

	sims, err := ListSimulators()
	if err != nil || len(sims) == 0 {
		t.Skip("No simulators available")
	}

	// Check status of first simulator
	status, err := CheckBootStatus(sims[0].UDID)
	if err != nil {
		t.Fatalf("CheckBootStatus() error: %v", err)
	}

	// Status should match the state
	expected := sims[0].State == "Booted"
	if status.Booted != expected {
		t.Errorf("CheckBootStatus().Booted = %v, expected %v (state: %s)", status.Booted, expected, sims[0].State)
	}
}

func TestCheckBootStatus_UnknownUDID(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("simctl requires macOS")
	}

	_, err := CheckBootStatus("nonexistent-udid-12345")
	if err == nil {
		t.Error("CheckBootStatus(unknown) should return error")
	}
}

// TestXcodebuildArch verifies Go's GOARCH names are translated into
// xcodebuild's destination vocabulary (#117: amd64 → x86_64 on Intel Macs).
func TestXcodebuildArch(t *testing.T) {
	cases := map[string]string{
		"amd64": "x86_64",
		"arm64": "arm64",
	}
	for in, want := range cases {
		if got := XcodebuildArch(in); got != want {
			t.Errorf("XcodebuildArch(%q) = %q, want %q", in, got, want)
		}
	}
}
