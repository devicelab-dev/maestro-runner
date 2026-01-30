package emulator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// FindEmulatorBinary locates the Android emulator binary
func FindEmulatorBinary() (string, error) {
	// Try ANDROID_HOME/emulator/emulator first (new layout)
	androidHome := getAndroidHome()
	if androidHome != "" {
		emulatorPath := filepath.Join(androidHome, "emulator", "emulator")
		if _, err := os.Stat(emulatorPath); err == nil {
			return emulatorPath, nil
		}

		// Try ANDROID_HOME/tools/emulator (old layout)
		emulatorPath = filepath.Join(androidHome, "tools", "emulator")
		if _, err := os.Stat(emulatorPath); err == nil {
			return emulatorPath, nil
		}
	}

	// Try PATH
	if path, err := exec.LookPath("emulator"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("emulator binary not found. Set ANDROID_HOME or add emulator to PATH")
}

// FindAVDManagerBinary locates the avdmanager binary
func FindAVDManagerBinary() (string, error) {
	androidHome := getAndroidHome()
	if androidHome != "" {
		// Try cmdline-tools/latest/bin/avdmanager (new layout)
		avdPath := filepath.Join(androidHome, "cmdline-tools", "latest", "bin", "avdmanager")
		if _, err := os.Stat(avdPath); err == nil {
			return avdPath, nil
		}

		// Try tools/bin/avdmanager (old layout)
		avdPath = filepath.Join(androidHome, "tools", "bin", "avdmanager")
		if _, err := os.Stat(avdPath); err == nil {
			return avdPath, nil
		}
	}

	// Try PATH
	if path, err := exec.LookPath("avdmanager"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("avdmanager not found. Set ANDROID_HOME or add avdmanager to PATH")
}

// getAndroidHome returns ANDROID_HOME environment variable
func getAndroidHome() string {
	// Try multiple env vars
	if home := os.Getenv("ANDROID_HOME"); home != "" {
		return home
	}
	if home := os.Getenv("ANDROID_SDK_ROOT"); home != "" {
		return home
	}
	if home := os.Getenv("ANDROID_SDK_HOME"); home != "" {
		return home
	}
	return ""
}

// ListAVDs returns all available Android Virtual Devices
func ListAVDs() ([]AVDInfo, error) {
	// Find emulator binary to list AVDs
	emulatorPath, err := FindEmulatorBinary()
	if err != nil {
		return nil, err
	}

	// Run emulator -list-avds
	cmd := exec.Command(emulatorPath, "-list-avds")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list AVDs: %w", err)
	}

	// Parse output (one AVD name per line)
	var avds []AVDInfo
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		avds = append(avds, AVDInfo{
			Name:      line,
			IsRunning: false, // Will be checked separately
		})
	}

	logger.Debug("Found %d AVDs: %v", len(avds), avds)
	return avds, nil
}

// IsEmulator checks if a device serial is an emulator
func IsEmulator(serial string) bool {
	return strings.HasPrefix(serial, "emulator-")
}

// CheckBootStatus checks all boot conditions for an emulator
// Implements devicelab's 3-stage verification
func CheckBootStatus(serial string) (*BootStatus, error) {
	status := &BootStatus{}

	// Stage 1: Check device state
	stateCmd := exec.Command("adb", "-s", serial, "get-state")
	stateOut, err := stateCmd.Output()
	status.StateReady = (err == nil && strings.TrimSpace(string(stateOut)) == "device")

	if !status.StateReady {
		return status, nil // Not ready yet
	}

	// Stage 2: Check boot completed property
	bootCmd := exec.Command("adb", "-s", serial, "shell", "getprop", "sys.boot_completed")
	bootOut, err := bootCmd.Output()
	status.BootCompleted = (err == nil && strings.TrimSpace(string(bootOut)) == "1")

	// Stage 3: Check service readiness
	// Check settings service
	settingsCmd := exec.Command("adb", "-s", serial, "shell", "settings", "list", "global")
	_, err = settingsCmd.Output()
	status.SettingsReady = (err == nil)

	// Check package manager
	pmCmd := exec.Command("adb", "-s", serial, "shell", "pm", "get-max-users")
	_, err = pmCmd.Output()
	status.PackageManager = (err == nil)

	return status, nil
}

// WaitForBootComplete waits for emulator to be fully booted
// Implements devicelab's 3-stage verification with polling
func WaitForBootComplete(serial string, timeout time.Duration) error {
	logger.Info("Waiting for emulator boot complete: %s", serial)
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		status, err := CheckBootStatus(serial)
		if err != nil {
			logger.Debug("Boot check error: %v", err)
			<-ticker.C
			continue
		}

		// Log progress
		logger.Debug("Boot status for %s: state=%v, boot=%v, settings=%v, pm=%v",
			serial, status.StateReady, status.BootCompleted, status.SettingsReady, status.PackageManager)

		if status.IsFullyReady() {
			logger.Info("Emulator fully booted: %s", serial)
			return nil
		}

		<-ticker.C
	}

	// Timeout - return partial status
	status, _ := CheckBootStatus(serial)
	return fmt.Errorf("emulator boot timeout after %v (state:%v boot:%v settings:%v pm:%v)",
		timeout, status.StateReady, status.BootCompleted, status.SettingsReady, status.PackageManager)
}

// WaitForDeviceState waits for device to appear in adb devices
func WaitForDeviceState(serial string, timeout time.Duration) error {
	logger.Info("Waiting for device state: %s", serial)
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		cmd := exec.Command("adb", "-s", serial, "get-state")
		output, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(output)) == "device" {
			logger.Info("Device state ready: %s", serial)
			return nil
		}
		<-ticker.C
	}

	return fmt.Errorf("timeout waiting for device state after %v", timeout)
}

// StartEmulator boots an Android emulator and returns serial and process
func StartEmulator(avdName string, consolePort int, timeout time.Duration) (string, *exec.Cmd, error) {
	logger.Info("Starting emulator: %s on port %d", avdName, consolePort)
	bootStart := time.Now()

	// Find emulator binary
	emulatorPath, err := FindEmulatorBinary()
	if err != nil {
		return "", nil, err
	}

	// Build serial
	serial := fmt.Sprintf("emulator-%d", consolePort)

	// Start emulator process (devicelab flags + Maestro optimizations)
	cmd := exec.Command(emulatorPath,
		"-avd", avdName,
		"-port", fmt.Sprintf("%d", consolePort),
		"-netdelay", "none",
		"-netspeed", "full",
		"-no-boot-anim",
		"-no-snapshot-load", // devicelab: always fresh boot
		"-no-snapshot-save", // avoid save dialog on shutdown
	)

	logger.Debug("Emulator command: %s %v", emulatorPath, cmd.Args[1:])

	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("failed to start emulator process: %w", err)
	}

	logger.Info("Emulator process started (PID: %d)", cmd.Process.Pid)

	// Stage 1: Wait for device state (60s)
	if err := WaitForDeviceState(serial, 60*time.Second); err != nil {
		cmd.Process.Kill()
		cmd.Wait() //nolint:errcheck // best-effort reap after kill
		return "", nil, fmt.Errorf("device state check failed: %w", err)
	}

	// Stage 2 & 3: Wait for boot complete and services (remaining timeout)
	elapsed := time.Since(bootStart)
	remaining := timeout - elapsed
	if remaining < 30*time.Second {
		remaining = 30 * time.Second // Minimum 30s for boot complete
	}

	if err := WaitForBootComplete(serial, remaining); err != nil {
		cmd.Process.Kill()
		cmd.Wait() //nolint:errcheck // best-effort reap after kill
		return "", nil, err
	}

	bootDuration := time.Since(bootStart)
	logger.Info("Emulator boot completed in %v", bootDuration)

	// Warn if slow boot (>30s)
	if bootDuration > 30*time.Second {
		logger.Info("Slow emulator boot detected (%v). Consider using snapshots or a faster system.", bootDuration)
	}

	// Reap the child process in the background to prevent zombies.
	// cmd.Wait() collects the exit status so the OS can clean up the process entry.
	// Started after boot verification to avoid racing with Kill() in error paths above.
	go func() {
		if err := cmd.Wait(); err != nil {
			logger.Debug("Emulator process exited: %v", err)
		}
	}()

	return serial, cmd, nil
}

// ShutdownEmulator gracefully shuts down an emulator
// Implements devicelab's shutdown chain pattern
func ShutdownEmulator(serial string, timeout time.Duration) error {
	logger.Info("Shutting down emulator: %s", serial)

	// Step 1: Try adb emu kill
	cmd := exec.Command("adb", "-s", serial, "emu", "kill")
	if err := cmd.Run(); err != nil {
		logger.Warn("adb emu kill failed for %s: %v", serial, err)
	}

	// Step 2: Wait for shutdown (30s)
	deadline := time.Now().Add(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		// Check if device is gone
		checkCmd := exec.Command("adb", "-s", serial, "get-state")
		if _, err := checkCmd.Output(); err != nil {
			logger.Info("Emulator shutdown confirmed: %s", serial)
			return nil
		}
		<-ticker.C
	}

	// Step 3: Force kill by process (if timeout)
	logger.Warn("Emulator shutdown timeout, trying force kill: %s", serial)
	if err := forceKillEmulator(serial); err != nil {
		logger.Error("Force kill failed for %s: %v", serial, err)
		return fmt.Errorf("failed to shutdown emulator after %v: %w", timeout, err)
	}

	logger.Info("Emulator force killed: %s", serial)
	return nil
}

// forceKillEmulator force kills emulator process
func forceKillEmulator(serial string) error {
	// Extract port from serial
	var port int
	if _, err := fmt.Sscanf(serial, "emulator-%d", &port); err != nil {
		return fmt.Errorf("failed to extract port from serial %s: %w", serial, err)
	}

	// Find emulator process by port
	cmd := exec.Command("pgrep", "-f", fmt.Sprintf("emulator.*-port %d", port))
	output, err := cmd.Output()
	if err != nil {
		// Try alternative search
		cmd = exec.Command("pgrep", "-f", "qemu-system.*-avd")
		output, err = cmd.Output()
		if err != nil {
			return fmt.Errorf("could not find emulator process")
		}
	}

	pids := strings.Fields(strings.TrimSpace(string(output)))
	if len(pids) == 0 {
		return fmt.Errorf("no emulator process found for %s", serial)
	}

	// Kill all found processes
	for _, pid := range pids {
		// Try SIGTERM first
		killCmd := exec.Command("kill", "-TERM", pid)
		if err := killCmd.Run(); err != nil {
			// If TERM fails, use SIGKILL
			logger.Warn("SIGTERM failed for PID %s, using SIGKILL", pid)
			killCmd = exec.Command("kill", "-KILL", pid)
			if err := killCmd.Run(); err != nil {
				logger.Error("SIGKILL failed for PID %s: %v", pid, err)
			}
		}
	}

	return nil
}
