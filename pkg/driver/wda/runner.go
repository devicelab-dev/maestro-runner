package wda

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	wdaBasePort    = uint16(8100)
	wdaPortRange   = uint16(1000)
	buildTimeout   = 10 * time.Minute
	startupTimeout = 90 * time.Second
)

// Runner handles building and running WDA on iOS devices.
type Runner struct {
	deviceUDID       string
	teamID           string
	port             uint16
	wdaPath          string
	buildDir         string
	cmd              *exec.Cmd
	logFile          *os.File
	iproxyCmd        *exec.Cmd // Port forwarding for physical devices
	isSimulatorCache bool      // Cached device type
}

// NewRunner creates a new WDA runner.
// The WDA port is derived from the device UDID so each simulator gets a
// deterministic, unique port without scanning.
func NewRunner(deviceUDID, teamID string) *Runner {
	return &Runner{
		deviceUDID: deviceUDID,
		teamID:     teamID,
		port:       PortFromUDID(deviceUDID),
	}
}

// Port returns the WDA port allocated for this runner's device.
func (r *Runner) Port() uint16 {
	return r.port
}

// PortFromUDID derives a deterministic port from a device UDID.
// Uses the last UUID segment (12 fully random hex chars in UUID v4),
// parsed as an integer mod 1000, added to base port 8100.
// Range: 8100–9099.
// Exported for use by CLI to check device availability before starting.
func PortFromUDID(udid string) uint16 {
	seg := udid
	if idx := strings.LastIndex(udid, "-"); idx >= 0 {
		seg = udid[idx+1:]
	}
	val, err := strconv.ParseUint(seg, 16, 64)
	if err != nil {
		return wdaBasePort // fallback to 8100 if UDID is not a standard UUID
	}
	return wdaBasePort + uint16(val%uint64(wdaPortRange))
}

// Build compiles WDA for the target device.
// Uses a persistent build cache directory specific to iOS version, device type, and team ID.
func (r *Runner) Build(ctx context.Context) error {
	wdaPath, err := GetWDAPath()
	if err != nil {
		return err
	}
	r.wdaPath = wdaPath

	// Get build cache directory specific to this configuration
	r.buildDir, err = r.getBuildCacheDir()
	if err != nil {
		return fmt.Errorf("failed to get build cache directory: %w", err)
	}

	os.MkdirAll(r.buildDir, 0755)
	os.MkdirAll(filepath.Join(r.buildDir, "logs"), 0755)

	// Check if already built by looking for xctestrun file
	if _, err := r.findXctestrun(); err == nil {
		// Build exists - skip rebuilding
		fmt.Printf("  ✓ Using cached WebDriverAgent build (%s)\n", filepath.Base(r.buildDir))
		return nil
	}

	// Need to build
	fmt.Println("\n  ⏳ Building WebDriverAgent for the first time...")
	fmt.Println("     This may take 5-10 minutes depending on your machine.")
	fmt.Println("     Next time it will be much faster (cached builds are reused).")
	fmt.Println()

	logPath := filepath.Join(r.buildDir, "logs", "build.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()

	buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()

	projectPath := filepath.Join(r.wdaPath, "WebDriverAgent.xcodeproj")

	cmd := exec.CommandContext(buildCtx, "xcodebuild",
		"build-for-testing",
		"-project", projectPath,
		"-scheme", "WebDriverAgentRunner",
		"-destination", r.destination(),
		"-derivedDataPath", r.derivedDataPath(),
		"-allowProvisioningUpdates",
		fmt.Sprintf("DEVELOPMENT_TEAM=%s", r.teamID),
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build failed:\n%s\n\nFull log: %s", tailLog(logPath, 20), logPath)
	}

	if _, err := r.findXctestrun(); err != nil {
		return err
	}

	fmt.Println("WebDriverAgent build complete")
	return nil
}

// Start runs WDA on the device.
func (r *Runner) Start(ctx context.Context) error {
	xctestrun, err := r.findXctestrun()
	if err != nil {
		return err
	}

	// Check if this is a simulator or physical device
	r.isSimulatorCache, _ = r.isSimulator()

	// Inject USE_PORT into the xctestrun plist so the WDA process picks it up.
	// Setting cmd.Env on xcodebuild does NOT propagate to the test runner;
	// the runner reads env vars from the xctestrun plist's EnvironmentVariables.
	if err := r.injectPort(xctestrun); err != nil {
		return fmt.Errorf("failed to set WDA port in xctestrun: %w", err)
	}

	logPath := filepath.Join(r.buildDir, "logs", "runner.log")
	r.logFile, err = os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	r.cmd = exec.CommandContext(ctx, "xcodebuild",
		"test-without-building",
		"-xctestrun", xctestrun,
		"-destination", r.destination(),
		"-derivedDataPath", r.derivedDataPath(),
	)
	r.cmd.Stdout = r.logFile
	r.cmd.Stderr = r.logFile

	fmt.Println("Starting WebDriverAgent...")

	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start WDA: %w", err)
	}

	if err := r.waitForStartup(logPath); err != nil {
		r.Stop()
		return err
	}

	// For physical devices, start iproxy to forward the port from device to localhost
	if !r.isSimulatorCache {
		if err := r.startIProxy(ctx); err != nil {
			r.Stop()
			return fmt.Errorf("failed to start port forwarding: %w", err)
		}
	}

	fmt.Println("WebDriverAgent started")
	return nil
}

// startIProxy starts iproxy to forward the WDA port from a physical device to localhost.
func (r *Runner) startIProxy(ctx context.Context) error {
	portStr := strconv.Itoa(int(r.port))

	// iproxy LOCAL_PORT:DEVICE_PORT -u UDID
	r.iproxyCmd = exec.CommandContext(ctx, "iproxy", portStr+":"+portStr, "-u", r.deviceUDID)

	if err := r.iproxyCmd.Start(); err != nil {
		return fmt.Errorf("iproxy failed to start: %w\nHint: Install libimobiledevice with 'brew install libimobiledevice'", err)
	}

	// Give iproxy a moment to establish the connection
	time.Sleep(500 * time.Millisecond)

	return nil
}

// injectPort writes USE_PORT into the xctestrun plist's EnvironmentVariables
// so the WDA test runner process starts on the allocated port.
func (r *Runner) injectPort(xctestrunPath string) error {
	portStr := strconv.Itoa(int(r.port))

	// Convert plist to JSON for easy manipulation
	jsonData, err := exec.Command("plutil", "-convert", "json", "-o", "-", xctestrunPath).Output()
	if err != nil {
		return fmt.Errorf("failed to read xctestrun: %w", err)
	}

	var plist map[string]interface{}
	if err := json.Unmarshal(jsonData, &plist); err != nil {
		return fmt.Errorf("failed to parse xctestrun: %w", err)
	}

	// Handle format version 2 (TestConfigurations array)
	if configs, ok := plist["TestConfigurations"].([]interface{}); ok {
		for _, cfg := range configs {
			cfgMap, _ := cfg.(map[string]interface{})
			if cfgMap == nil {
				continue
			}
			targets, _ := cfgMap["TestTargets"].([]interface{})
			for _, tgt := range targets {
				setPortEnv(tgt, portStr)
			}
		}
	} else {
		// Format version 1: top-level keys are test targets
		for key, val := range plist {
			if key == "__xctestrun_metadata__" {
				continue
			}
			setPortEnv(val, portStr)
		}
	}

	result, err := json.MarshalIndent(plist, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize xctestrun: %w", err)
	}

	if err := os.WriteFile(xctestrunPath, result, 0644); err != nil {
		return fmt.Errorf("failed to write xctestrun: %w", err)
	}

	// Convert back to XML plist format
	if out, err := exec.Command("plutil", "-convert", "xml1", xctestrunPath).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to convert xctestrun to plist: %s: %w", out, err)
	}

	return nil
}

func setPortEnv(target interface{}, portStr string) {
	tgtMap, ok := target.(map[string]interface{})
	if !ok {
		return
	}
	env, ok := tgtMap["EnvironmentVariables"].(map[string]interface{})
	if !ok {
		env = make(map[string]interface{})
		tgtMap["EnvironmentVariables"] = env
	}
	env["USE_PORT"] = portStr
}

// Stop terminates the running WDA.
func (r *Runner) Stop() {
	// Stop iproxy if running (for physical devices)
	if r.iproxyCmd != nil && r.iproxyCmd.Process != nil {
		r.iproxyCmd.Process.Kill()
		r.iproxyCmd = nil
	}
	if r.cmd != nil && r.cmd.Process != nil {
		r.cmd.Process.Kill()
		r.cmd = nil
	}
	if r.logFile != nil {
		r.logFile.Close()
		r.logFile = nil
	}
}

// Cleanup stops WDA runner.
// Note: Build directory is now persistent and not removed to enable build reuse.
func (r *Runner) Cleanup() {
	r.Stop()
	// Build directory is persistent (in cache), don't remove it
}

// getBuildCacheDir returns the cache directory path for this specific configuration.
// Format: ~/.maestro-runner/cache/wda-builds/{config-name}/
// Examples:
//   - Simulator: sim-ios18.5-iphone/
//   - Real device: device-ios18.0-teamABC123/
func (r *Runner) getBuildCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Get device info
	isSimulator, err := r.isSimulator()
	if err != nil {
		return "", err
	}

	iosVersion, err := r.getIOSVersion()
	if err != nil {
		return "", err
	}

	// Generate config-specific directory name
	var configName string
	if isSimulator {
		// Simulator: sim-ios{version}-iphone
		configName = fmt.Sprintf("sim-ios%s-iphone", iosVersion)
	} else {
		// Real device: device-ios{version}-team{teamID}
		teamID := r.teamID
		if teamID == "" {
			teamID = "default"
		}
		configName = fmt.Sprintf("device-ios%s-team%s", iosVersion, teamID)
	}

	cacheDir := filepath.Join(home, ".maestro-runner", "cache", "wda-builds", configName)
	return cacheDir, nil
}

// isSimulator checks if the device is a simulator.
func (r *Runner) isSimulator() (bool, error) {
	// Run simctl to check if this UDID is a simulator
	cmd := exec.Command("xcrun", "simctl", "list", "devices", "-j")
	output, err := cmd.Output()
	if err != nil {
		// If simctl fails, assume it might be a real device
		return false, nil
	}

	// Parse JSON to check if UDID exists in simulator list
	var data map[string]interface{}
	if err := json.Unmarshal(output, &data); err != nil {
		return false, err
	}

	devices, ok := data["devices"].(map[string]interface{})
	if !ok {
		return false, nil
	}

	// Check if our UDID appears in any runtime
	for _, deviceList := range devices {
		if list, ok := deviceList.([]interface{}); ok {
			for _, device := range list {
				if deviceMap, ok := device.(map[string]interface{}); ok {
					if udid, ok := deviceMap["udid"].(string); ok && udid == r.deviceUDID {
						return true, nil
					}
				}
			}
		}
	}

	return false, nil
}

// getIOSVersion returns the iOS version of the device.
func (r *Runner) getIOSVersion() (string, error) {
	// Try simctl first (for simulators)
	cmd := exec.Command("xcrun", "simctl", "list", "devices", "-j")
	output, err := cmd.Output()
	if err == nil {
		var data map[string]interface{}
		if err := json.Unmarshal(output, &data); err == nil {
			devices, ok := data["devices"].(map[string]interface{})
			if ok {
				for runtime, deviceList := range devices {
					if list, ok := deviceList.([]interface{}); ok {
						for _, device := range list {
							if deviceMap, ok := device.(map[string]interface{}); ok {
								if udid, ok := deviceMap["udid"].(string); ok && udid == r.deviceUDID {
									// Extract iOS version from runtime string
									// Example: "com.apple.CoreSimulator.SimRuntime.iOS-18-5" -> "18.5"
									parts := strings.Split(runtime, ".")
									if len(parts) > 0 {
										lastPart := parts[len(parts)-1]
										// iOS-18-5 -> 18.5
										version := strings.TrimPrefix(lastPart, "iOS-")
										version = strings.ReplaceAll(version, "-", ".")
										return version, nil
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// For real devices, use ideviceinfo (requires libimobiledevice)
	// Fallback to a generic version if not available
	cmd = exec.Command("ideviceinfo", "-u", r.deviceUDID, "-k", "ProductVersion")
	output, err = cmd.Output()
	if err == nil {
		version := strings.TrimSpace(string(output))
		if version != "" {
			return version, nil
		}
	}

	// Fallback: use a generic version identifier
	return "unknown", nil
}

func (r *Runner) destination() string {
	return fmt.Sprintf("id=%s", r.deviceUDID)
}

func (r *Runner) derivedDataPath() string {
	return filepath.Join(r.buildDir, "DerivedData")
}

func (r *Runner) findXctestrun() (string, error) {
	pattern := filepath.Join(r.derivedDataPath(), "Build", "Products", "*.xctestrun")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return "", fmt.Errorf("no xctestrun file found in %s", filepath.Dir(pattern))
	}
	return matches[0], nil
}

func (r *Runner) waitForStartup(logPath string) error {
	timeout := time.After(startupTimeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			content, err := os.ReadFile(logPath)
			if err != nil {
				continue
			}
			if err := r.checkLog(string(content), logPath); err != errNotReady {
				return err
			}
		case <-timeout:
			return fmt.Errorf("WDA startup timeout (90s):\n%s\n\nFull log: %s", tailLog(logPath, 20), logPath)
		}
	}
}

var errNotReady = fmt.Errorf("not ready")

func (r *Runner) checkLog(log, logPath string) error {
	// Success indicators
	if strings.Contains(log, "ServerURLHere") || strings.Contains(log, "WebDriverAgent") && strings.Contains(log, "started") {
		return nil
	}

	// Known errors
	if strings.Contains(log, "Developer App Certificate is not trusted") {
		return fmt.Errorf("certificate not trusted - trust it in Settings > General > VPN & Device Management")
	}
	if strings.Contains(log, "Code Sign error") {
		return fmt.Errorf("code signing failed - check your DEVELOPMENT_TEAM and provisioning profiles")
	}
	if strings.Contains(log, "Testing failed:") {
		return fmt.Errorf("WDA failed:\n%s\n\nFull log: %s", tailLog(logPath, 20), logPath)
	}

	return errNotReady
}

func tailLog(path string, lines int) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("(could not read log: %s)", err)
	}
	allLines := strings.Split(string(content), "\n")
	if len(allLines) <= lines {
		return string(content)
	}
	return strings.Join(allLines[len(allLines)-lines:], "\n")
}
