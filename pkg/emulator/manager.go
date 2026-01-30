package emulator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

const (
	startingPort     = 5554 // First emulator port
	maxRetryAttempts = 50   // Max port allocation attempts (devicelab pattern)
)

// NewManager creates a new emulator manager
func NewManager() *Manager {
	mgr := &Manager{
		portMap: make(map[string]int),
	}

	// Load persistent port mapping
	if err := mgr.loadPortMapping(); err != nil {
		logger.Debug("Could not load port mapping: %v", err)
	}

	return mgr
}

// Start starts an emulator and tracks it
func (m *Manager) Start(avdName string, timeout time.Duration) (string, error) {
	return m.StartWithRetry(avdName, timeout, maxRetryAttempts)
}

// StartWithRetry starts an emulator with port conflict retry logic
// Implements devicelab's retry pattern
func (m *Manager) StartWithRetry(avdName string, timeout time.Duration, maxAttempts int) (string, error) {
	// Get initial port
	port := m.AllocatePort(avdName)
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		logger.Info("Starting emulator attempt %d/%d: %s (port %d)", attempt, maxAttempts, avdName, port)

		// Try to start emulator
		startedSerial, cmd, err := StartEmulator(avdName, port, timeout)
		if err == nil {
			// Success! Track and save
			instance := &EmulatorInstance{
				AVDName:     avdName,
				Serial:      startedSerial,
				ConsolePort: port,
				ADBPort:     port + 1,
				Process:     cmd,
				StartedBy:   "maestro-runner",
				BootStart:   time.Now(),
			}
			m.started.Store(startedSerial, instance)

			// Save port mapping
			m.mu.Lock()
			m.portMap[avdName] = port
			m.mu.Unlock()
			m.savePortMapping()

			logger.Info("Emulator started and tracked: %s", startedSerial)
			return startedSerial, nil
		}

		lastErr = err

		// Check if we should retry (port conflict)
		if m.shouldRetryOnError(err) {
			logger.Warn("Port conflict detected, trying next port (attempt %d/%d)", attempt, maxAttempts)
			port = m.getNextPort(port)
			continue
		}

		// Non-port error - don't retry
		logger.Error("Emulator start failed (non-retriable): %v", err)
		return "", err
	}

	// All attempts exhausted
	return "", fmt.Errorf("failed to start emulator after %d attempts: %w", maxAttempts, lastErr)
}

// AllocatePort returns the port for an AVD (from mapping or next available)
func (m *Manager) AllocatePort(avdName string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we have a persistent mapping
	if port, exists := m.portMap[avdName]; exists {
		logger.Debug("Reusing persistent port %d for AVD %s", port, avdName)
		return port
	}

	// Find next available port
	nextPort := startingPort
	for _, port := range m.portMap {
		if port >= nextPort {
			nextPort = port + 2 // Always increment by 2 (even numbers)
		}
	}

	// Save allocation immediately to prevent race conditions
	m.portMap[avdName] = nextPort
	logger.Debug("Allocated new port %d for AVD %s", nextPort, avdName)
	return nextPort
}

// getNextPort returns the next emulator port (increment by 2)
func (m *Manager) getNextPort(currentPort int) int {
	return currentPort + 2
}

// shouldRetryOnError checks if error indicates port conflict
func (m *Manager) shouldRetryOnError(err error) bool {
	// Simple heuristic: if error mentions "port" or "address already in use"
	// TODO: Could be more sophisticated
	// For now, don't retry on errors (emulator doesn't return port conflicts clearly)
	// In practice, emulator will fail to start if port is in use, but error message varies
	_ = err // unused for now
	return false
}

// Shutdown shuts down an emulator if we started it
func (m *Manager) Shutdown(serial string) error {
	// Check if we started this emulator
	instance, exists := m.started.Load(serial)
	if !exists {
		logger.Debug("Emulator %s not started by us, skipping shutdown", serial)
		return nil
	}

	logger.Info("Shutting down emulator: %s", serial)

	// Shutdown the emulator
	if err := ShutdownEmulator(serial, 30*time.Second); err != nil {
		logger.Error("Failed to shutdown emulator %s: %v", serial, err)
		return err
	}

	// Remove from tracking
	m.started.Delete(serial)

	// Update boot duration if we have the instance
	if inst, ok := instance.(*EmulatorInstance); ok {
		inst.BootDuration = time.Since(inst.BootStart)
		logger.Debug("Emulator %s ran for %v", serial, inst.BootDuration)
	}

	return nil
}

// ShutdownAll shuts down all emulators started by us
func (m *Manager) ShutdownAll() error {
	logger.Info("Shutting down all tracked emulators")

	var errors []error
	m.started.Range(func(key, value interface{}) bool {
		serial := key.(string)
		if err := m.Shutdown(serial); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", serial, err))
		}
		return true // Continue iteration
	})

	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	return nil
}

// IsStartedByUs checks if we started this emulator
func (m *Manager) IsStartedByUs(serial string) bool {
	_, exists := m.started.Load(serial)
	return exists
}

// GetStartedEmulators returns list of all emulators we started
func (m *Manager) GetStartedEmulators() []string {
	var serials []string
	m.started.Range(func(key, value interface{}) bool {
		serials = append(serials, key.(string))
		return true
	})
	return serials
}

// loadPortMapping loads persistent port mapping from file
func (m *Manager) loadPortMapping() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	portMapFile := filepath.Join(homeDir, ".maestro-runner", "emulator-ports.json")
	data, err := os.ReadFile(portMapFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's OK
		}
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := json.Unmarshal(data, &m.portMap); err != nil {
		return err
	}

	logger.Debug("Loaded port mapping: %v", m.portMap)
	return nil
}

// savePortMapping saves persistent port mapping to file
func (m *Manager) savePortMapping() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	maestroDir := filepath.Join(homeDir, ".maestro-runner")
	if err := os.MkdirAll(maestroDir, 0755); err != nil {
		return err
	}

	portMapFile := filepath.Join(maestroDir, "emulator-ports.json")

	m.mu.Lock()
	data, err := json.MarshalIndent(m.portMap, "", "  ")
	m.mu.Unlock()

	if err != nil {
		return err
	}

	if err := os.WriteFile(portMapFile, data, 0644); err != nil {
		return err
	}

	logger.Debug("Saved port mapping to %s", portMapFile)
	return nil
}
