package simulator

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// FindSimctlBinary verifies that xcrun/simctl is available.
func FindSimctlBinary() (string, error) {
	path, err := execLookPath("xcrun")
	if err != nil {
		return "", fmt.Errorf("xcrun not found; install Xcode Command Line Tools: xcode-select --install")
	}
	return path, nil
}

// simctlDevicesOutput represents the JSON output from xcrun simctl list devices.
type simctlDevicesOutput struct {
	Devices map[string][]simctlDevice `json:"devices"`
}

type simctlDevice struct {
	Name        string `json:"name"`
	UDID        string `json:"udid"`
	State       string `json:"state"`
	IsAvailable bool   `json:"isAvailable"`
}

// ListSimulators returns all available iOS simulators.
func ListSimulators() ([]SimulatorDevice, error) {
	if _, err := FindSimctlBinary(); err != nil {
		return nil, err
	}

	cmd := execCommand("xcrun", "simctl", "list", "devices", "available", "-j")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list simulators: %w", err)
	}

	var data simctlDevicesOutput
	if err := json.Unmarshal(output, &data); err != nil {
		return nil, fmt.Errorf("failed to parse simctl output: %w", err)
	}

	var sims []SimulatorDevice
	for runtime, devices := range data.Devices {
		platform := extractPlatform(runtime)
		osVersion := extractOSVersion(runtime)
		for _, dev := range devices {
			if !dev.IsAvailable {
				continue
			}
			sims = append(sims, SimulatorDevice{
				Name:        dev.Name,
				UDID:        dev.UDID,
				Runtime:     runtime,
				Platform:    platform,
				OSVersion:   osVersion,
				State:       dev.State,
				IsAvailable: dev.IsAvailable,
			})
		}
	}

	logger.Debug("Found %d available simulators", len(sims))
	return sims, nil
}

// ListIOSSimulators returns available iOS simulators only (excludes tvOS, watchOS, visionOS).
func ListIOSSimulators() ([]SimulatorDevice, error) {
	sims, err := ListSimulators()
	if err != nil {
		return nil, err
	}

	var iosSims []SimulatorDevice
	for _, sim := range sims {
		if sim.Platform == "iOS" {
			iosSims = append(iosSims, sim)
		}
	}
	return iosSims, nil
}

// ListShutdownSimulators returns available simulators that are currently shut down.
func ListShutdownSimulators() ([]SimulatorDevice, error) {
	sims, err := ListSimulators()
	if err != nil {
		return nil, err
	}

	var shutdown []SimulatorDevice
	for _, sim := range sims {
		if sim.State == "Shutdown" {
			shutdown = append(shutdown, sim)
		}
	}
	return shutdown, nil
}

// ListShutdownIOSSimulators returns iOS simulators that are currently shut down.
func ListShutdownIOSSimulators() ([]SimulatorDevice, error) {
	sims, err := ListIOSSimulators()
	if err != nil {
		return nil, err
	}

	var shutdown []SimulatorDevice
	for _, sim := range sims {
		if sim.State == "Shutdown" {
			shutdown = append(shutdown, sim)
		}
	}
	return shutdown, nil
}

// IsSimulator checks if a UDID belongs to a known simulator.
func IsSimulator(udid string) bool {
	sims, err := ListSimulators()
	if err != nil {
		return false
	}
	for _, sim := range sims {
		if sim.UDID == udid {
			return true
		}
	}
	return false
}

// CheckBootStatus checks if a simulator is booted.
func CheckBootStatus(udid string) (*BootStatus, error) {
	sims, err := ListSimulators()
	if err != nil {
		return nil, err
	}
	for _, sim := range sims {
		if sim.UDID == udid {
			return &BootStatus{Booted: sim.State == "Booted"}, nil
		}
	}
	return nil, fmt.Errorf("simulator not found: %s", udid)
}

// WaitForBoot waits for a simulator to reach "Booted" state.
func WaitForBoot(udid string, timeout time.Duration) error {
	logger.Info("Waiting for simulator boot: %s", udid)
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		status, err := CheckBootStatus(udid)
		if err != nil {
			logger.Debug("Boot check error: %v", err)
			<-ticker.C
			continue
		}
		if status.IsReady() {
			logger.Info("Simulator booted: %s", udid)
			return nil
		}
		<-ticker.C
	}

	return fmt.Errorf("simulator boot timeout after %v", timeout)
}

// BootSimulator boots an iOS simulator and waits for it to be ready.
func BootSimulator(udid string, timeout time.Duration) error {
	logger.Info("Booting simulator: %s", udid)

	cmd := execCommand("xcrun", "simctl", "boot", udid)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Check if already booted
		if strings.Contains(string(output), "current state: Booted") {
			logger.Info("Simulator already booted: %s", udid)
			return nil
		}
		return fmt.Errorf("failed to boot simulator: %s", strings.TrimSpace(string(output)))
	}

	// Wait for boot to complete
	if err := WaitForBoot(udid, timeout); err != nil {
		return err
	}

	// Open the Simulator UI
	openCmd := execCommand("open", "-a", "Simulator")
	if err := openCmd.Run(); err != nil {
		logger.Debug("Failed to open Simulator app: %v", err)
	}

	return nil
}

// ShutdownSimulator gracefully shuts down a simulator.
func ShutdownSimulator(udid string, timeout time.Duration) error {
	logger.Info("Shutting down simulator: %s", udid)

	cmd := execCommand("xcrun", "simctl", "shutdown", udid)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Check if already shutdown
		if strings.Contains(string(output), "current state: Shutdown") {
			logger.Info("Simulator already shutdown: %s", udid)
			return nil
		}
		logger.Warn("simctl shutdown failed for %s: %s", udid, strings.TrimSpace(string(output)))
	}

	// Poll until shutdown confirmed
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		status, err := CheckBootStatus(udid)
		if err != nil || !status.Booted {
			logger.Info("Simulator shutdown confirmed: %s", udid)
			return nil
		}
		<-ticker.C
	}

	return fmt.Errorf("simulator shutdown timeout after %v", timeout)
}

// simctlRuntimesOutput represents the JSON output from xcrun simctl list runtimes.
type simctlRuntimesOutput struct {
	Runtimes []simctlRuntime `json:"runtimes"`
}

type simctlRuntime struct {
	Identifier          string                   `json:"identifier"`
	Version             string                   `json:"version"`
	IsAvailable         bool                     `json:"isAvailable"`
	SupportedDeviceTypes []simctlDeviceTypeEntry `json:"supportedDeviceTypes"`
}

type simctlDeviceTypeEntry struct {
	Name          string `json:"name"`
	Identifier    string `json:"identifier"`
	ProductFamily string `json:"productFamily"`
}

// IOSRuntime describes an available iOS runtime and its supported iPhone device types.
type IOSRuntime struct {
	Identifier  string   // e.g., "com.apple.CoreSimulator.SimRuntime.iOS-17-2"
	Version     string   // e.g., "17.2"
	DeviceTypes []string // iPhone device type identifiers
}

// LatestIOSRuntime returns the latest available iOS runtime with its supported iPhone device types.
func LatestIOSRuntime() (*IOSRuntime, error) {
	if _, err := FindSimctlBinary(); err != nil {
		return nil, err
	}

	cmd := execCommand("xcrun", "simctl", "list", "runtimes", "available", "-j")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list runtimes: %w", err)
	}

	var data simctlRuntimesOutput
	if err := json.Unmarshal(output, &data); err != nil {
		return nil, fmt.Errorf("failed to parse runtimes output: %w", err)
	}

	// Find the latest iOS runtime (last in list is typically newest)
	var best *simctlRuntime
	for i := range data.Runtimes {
		rt := &data.Runtimes[i]
		if !rt.IsAvailable || !strings.Contains(rt.Identifier, "SimRuntime.iOS-") {
			continue
		}
		best = rt
	}

	if best == nil {
		return nil, fmt.Errorf("no available iOS runtimes found")
	}

	// Collect iPhone device types
	var deviceTypes []string
	for _, dt := range best.SupportedDeviceTypes {
		if dt.ProductFamily == "iPhone" {
			deviceTypes = append(deviceTypes, dt.Identifier)
		}
	}

	if len(deviceTypes) == 0 {
		return nil, fmt.Errorf("no iPhone device types for runtime %s", best.Identifier)
	}

	return &IOSRuntime{
		Identifier:  best.Identifier,
		Version:     best.Version,
		DeviceTypes: deviceTypes,
	}, nil
}

// CreateSimulator creates a new iOS simulator and returns its UDID.
// Uses xcrun simctl create <name> <deviceTypeID> <runtimeID>.
func CreateSimulator(name, deviceTypeID, runtimeID string) (string, error) {
	if _, err := FindSimctlBinary(); err != nil {
		return "", err
	}

	cmd := execCommand("xcrun", "simctl", "create", name, deviceTypeID, runtimeID)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to create simulator: %w", err)
	}

	udid := strings.TrimSpace(string(output))
	if udid == "" {
		return "", fmt.Errorf("simctl create returned empty UDID")
	}

	logger.Info("Created simulator: %s (%s) [%s, %s]", name, udid, deviceTypeID, runtimeID)
	return udid, nil
}

// DeleteSimulator deletes a simulator by UDID.
func DeleteSimulator(udid string) error {
	if _, err := FindSimctlBinary(); err != nil {
		return err
	}

	cmd := execCommand("xcrun", "simctl", "delete", udid)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete simulator %s: %s", udid, strings.TrimSpace(string(output)))
	}

	logger.Info("Deleted simulator: %s", udid)
	return nil
}

// extractPlatform extracts the platform name from a runtime string.
// e.g., "com.apple.CoreSimulator.SimRuntime.iOS-17-2" → "iOS"
// e.g., "com.apple.CoreSimulator.SimRuntime.tvOS-17-0" → "tvOS"
func extractPlatform(runtime string) string {
	for _, p := range []string{"iOS", "tvOS", "watchOS", "xrOS"} {
		if strings.Contains(runtime, p+"-") {
			return p
		}
	}
	return ""
}

// extractOSVersion extracts version from runtime string.
// e.g., "com.apple.CoreSimulator.SimRuntime.iOS-17-2" → "17.2"
func extractOSVersion(runtime string) string {
	// Find "iOS-" prefix and extract version
	idx := strings.LastIndex(runtime, "iOS-")
	if idx == -1 {
		// Try other platforms (watchOS, tvOS, visionOS)
		for _, prefix := range []string{"watchOS-", "tvOS-", "xrOS-"} {
			idx = strings.LastIndex(runtime, prefix)
			if idx != -1 {
				version := runtime[idx+len(prefix):]
				return strings.ReplaceAll(version, "-", ".")
			}
		}
		return ""
	}
	version := runtime[idx+4:] // skip "iOS-"
	return strings.ReplaceAll(version, "-", ".")
}
