package emulator

import (
	"os/exec"
	"sync"
	"time"
)

// AVDInfo represents an Android Virtual Device
type AVDInfo struct {
	Name       string // AVD name (e.g., "Pixel_7_API_33")
	Device     string // Device type (e.g., "pixel_7")
	Target     string // API level (e.g., "android-33")
	Path       string // AVD directory path
	SDKVersion string // SDK version string
	IsRunning  bool   // Whether emulator is currently running
}

// EmulatorInstance tracks a running emulator started by maestro-runner
type EmulatorInstance struct {
	AVDName      string        // AVD name
	Serial       string        // Device serial (e.g., "emulator-5554")
	ConsolePort  int           // Console port (even: 5554, 5556, 5558...)
	ADBPort      int           // ADB port (odd: console + 1)
	Process      *exec.Cmd     // Emulator process
	StartedBy    string        // "maestro-runner" or "external"
	BootStart    time.Time     // Boot start time
	BootDuration time.Duration // Total boot duration
}

// Manager manages emulator lifecycle and tracks started emulators
type Manager struct {
	started sync.Map       // serial -> *EmulatorInstance (thread-safe)
	portMap map[string]int // AVD name -> console port (persistent mapping)
	mu      sync.Mutex     // Protects portMap
}

// BootStatus represents emulator boot state
type BootStatus struct {
	StateReady     bool // adb get-state == "device"
	BootCompleted  bool // sys.boot_completed == "1"
	SettingsReady  bool // settings list global succeeds
	PackageManager bool // pm get-max-users succeeds
}

// IsFullyReady returns true if all boot checks passed
func (bs *BootStatus) IsFullyReady() bool {
	return bs.StateReady && bs.BootCompleted && bs.SettingsReady && bs.PackageManager
}
