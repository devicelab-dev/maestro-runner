// SPDX-FileCopyrightText: 2024–2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package simulator

import (
	"sync"
	"time"
)

// SimulatorDevice represents an available simulator from simctl list.
type SimulatorDevice struct {
	Name        string // e.g., "iPhone 15 Pro"
	UDID        string // e.g., "A1B2C3D4-E5F6-..."
	Runtime     string // e.g., "com.apple.CoreSimulator.SimRuntime.iOS-17-2"
	Platform    string // e.g., "iOS", "tvOS", "watchOS", "xrOS"
	OSVersion   string // e.g., "17.2" (extracted from Runtime)
	State       string // "Shutdown", "Booted", etc.
	IsAvailable bool
}

// SimulatorInstance tracks a simulator booted by maestro-runner.
type SimulatorInstance struct {
	UDID         string        // Simulator UDID
	Name         string        // Simulator name (e.g., "iPhone 15 Pro")
	StartedBy    string        // "maestro-runner"
	CreatedByUs  bool          // true if we created this simulator (should delete on cleanup)
	BootStart    time.Time     // When boot was initiated
	BootDuration time.Duration // Total boot duration
}

// BootStatus represents simulator boot state.
type BootStatus struct {
	Booted bool // state == "Booted" from simctl list
}

// IsReady returns true if the simulator is fully booted.
func (bs *BootStatus) IsReady() bool {
	return bs.Booted
}

// Manager manages iOS simulator lifecycle and tracks started simulators.
type Manager struct {
	started sync.Map // UDID -> *SimulatorInstance (thread-safe)
}
