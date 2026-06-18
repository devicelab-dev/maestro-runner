// SPDX-FileCopyrightText: 2024–2026 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package simulator

import (
	"fmt"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// NewManager creates a new simulator manager.
func NewManager() *Manager {
	return &Manager{}
}

// Start boots a simulator by UDID and tracks it.
func (m *Manager) Start(udid string, timeout time.Duration) (string, error) {
	logger.Info("Starting simulator: %s (timeout: %v)", udid, timeout)
	bootStart := time.Now()

	if err := BootSimulator(udid, timeout); err != nil {
		return "", fmt.Errorf("failed to boot simulator %s: %w", udid, err)
	}

	bootDuration := time.Since(bootStart)

	// Look up name for tracking
	name := udid
	sims, err := ListSimulators()
	if err == nil {
		for _, sim := range sims {
			if sim.UDID == udid {
				name = sim.Name
				break
			}
		}
	}

	instance := &SimulatorInstance{
		UDID:         udid,
		Name:         name,
		StartedBy:    "maestro-runner",
		BootStart:    bootStart,
		BootDuration: bootDuration,
	}
	m.started.Store(udid, instance)

	logger.Info("Simulator started and tracked: %s (%s, boot time: %v)", name, udid, bootDuration)
	return udid, nil
}

// StartByName finds a simulator by name and boots it.
// Picks the first matching shutdown simulator. Returns the UDID.
func (m *Manager) StartByName(name string, timeout time.Duration) (string, error) {
	sims, err := ListSimulators()
	if err != nil {
		return "", err
	}

	nameLower := strings.ToLower(name)
	for _, sim := range sims {
		if strings.ToLower(sim.Name) == nameLower || sim.UDID == name {
			if sim.State == "Booted" {
				// Already booted — just track it
				logger.Info("Simulator already booted: %s (%s)", sim.Name, sim.UDID)
				m.started.Store(sim.UDID, &SimulatorInstance{
					UDID:      sim.UDID,
					Name:      sim.Name,
					StartedBy: "maestro-runner",
					BootStart: time.Now(),
				})
				return sim.UDID, nil
			}
			return m.Start(sim.UDID, timeout)
		}
	}

	return "", fmt.Errorf("simulator not found: %s", name)
}

// CreateAndStart creates a new iOS simulator, boots it, and tracks it.
// The simulator will be deleted on shutdown.
func (m *Manager) CreateAndStart(name, deviceTypeID, runtimeID string, timeout time.Duration) (string, error) {
	logger.Info("Creating simulator: %s (%s, %s)", name, deviceTypeID, runtimeID)

	udid, err := CreateSimulator(name, deviceTypeID, runtimeID)
	if err != nil {
		return "", fmt.Errorf("failed to create simulator: %w", err)
	}

	bootStart := time.Now()
	if err := BootSimulator(udid, timeout); err != nil {
		// Clean up the created simulator on boot failure
		logger.Error("Boot failed for created simulator %s, deleting", udid)
		if delErr := DeleteSimulator(udid); delErr != nil {
			logger.Error("Failed to delete simulator %s after boot failure: %v", udid, delErr)
		}
		return "", fmt.Errorf("failed to boot simulator: %w", err)
	}

	instance := &SimulatorInstance{
		UDID:         udid,
		Name:         name,
		StartedBy:    "maestro-runner",
		CreatedByUs:  true,
		BootStart:    bootStart,
		BootDuration: time.Since(bootStart),
	}
	m.started.Store(udid, instance)

	logger.Info("Created and started simulator: %s (%s, boot time: %v)", name, udid, instance.BootDuration)
	return udid, nil
}

// Shutdown shuts down a simulator if we started it.
// If the simulator was created by us, it is also deleted.
func (m *Manager) Shutdown(udid string) error {
	instance, exists := m.started.Load(udid)
	if !exists {
		logger.Debug("Simulator %s not started by us, skipping shutdown", udid)
		return nil
	}

	logger.Info("Shutting down simulator: %s", udid)

	if err := ShutdownSimulator(udid, 30*time.Second); err != nil {
		logger.Error("Failed to shutdown simulator %s: %v", udid, err)
		return err
	}

	m.started.Delete(udid)

	if inst, ok := instance.(*SimulatorInstance); ok {
		inst.BootDuration = time.Since(inst.BootStart)
		logger.Debug("Simulator %s ran for %v", udid, inst.BootDuration)

		// Delete simulators we created
		if inst.CreatedByUs {
			logger.Info("Deleting created simulator: %s (%s)", inst.Name, udid)
			if err := DeleteSimulator(udid); err != nil {
				logger.Error("Failed to delete simulator %s: %v", udid, err)
			}
		}
	}

	return nil
}

// ShutdownAll shuts down all simulators started by us, in parallel.
func (m *Manager) ShutdownAll() error {
	logger.Info("Shutting down all tracked simulators")

	var udids []string
	m.started.Range(func(key, _ interface{}) bool {
		udids = append(udids, key.(string))
		return true
	})

	if len(udids) == 0 {
		return nil
	}

	errCh := make(chan error, len(udids))
	for _, udid := range udids {
		go func(u string) {
			errCh <- m.Shutdown(u)
		}(udid)
	}

	var errs []error
	for range udids {
		if err := <-errCh; err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during simulator shutdown: %v", errs)
	}

	return nil
}

// IsStartedByUs checks if we started this simulator.
func (m *Manager) IsStartedByUs(udid string) bool {
	_, exists := m.started.Load(udid)
	return exists
}

// GetStartedSimulators returns list of all simulators we started.
func (m *Manager) GetStartedSimulators() []string {
	var udids []string
	m.started.Range(func(key, _ interface{}) bool {
		udids = append(udids, key.(string))
		return true
	})
	return udids
}
