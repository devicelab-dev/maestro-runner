package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	wdadriver "github.com/devicelab-dev/maestro-runner/pkg/driver/wda"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// simulatorInfo holds iOS simulator information.
type simulatorInfo struct {
	Name      string
	OSVersion string
	State     string
}

// iosDeviceInfo holds iOS device information (simulator or physical).
type iosDeviceInfo struct {
	Name        string
	OSVersion   string
	IsSimulator bool
}

// createIOSDriver creates an iOS driver using WebDriverAgent.
func createIOSDriver(cfg *RunConfig) (core.Driver, func(), error) {
	udid := getFirstDevice(cfg)

	if udid == "" {
		// Try to find booted simulator or connected physical device
		printSetupStep("Finding iOS device...")
		logger.Info("Auto-detecting iOS device (simulator or physical)...")
		var err error
		udid, err = findIOSDevice()
		if err != nil {
			logger.Error("No iOS device found")
			return nil, nil, fmt.Errorf("no device found\n" +
				"Hint: Specify a device with --device <UDID>, start a simulator, or connect a physical device")
		}
		logger.Info("Found iOS device: %s", udid)
		printSetupSuccess(fmt.Sprintf("Found device: %s", udid))
	} else {
		logger.Info("Using specified iOS device: %s", udid)
	}

	// Check if device port is already in use (another instance using this device)
	port := wdadriver.PortFromUDID(udid)
	if isPortInUse(port) {
		return nil, nil, fmt.Errorf("device %s is in use (port %d already bound)\n"+
			"Another maestro-runner instance may be using this device.\n"+
			"Hint: Wait for it to finish or use a different device with --device <UDID>", udid, port)
	}

	// 0. Detect device type (simulator vs physical)
	isSimulator := isIOSSimulator(udid)
	if isSimulator {
		logger.Info("Device %s is a simulator", udid)
	} else {
		logger.Info("Device %s is a physical device", udid)
	}

	// 1. Install app if specified
	if cfg.AppFile != "" {
		printSetupStep(fmt.Sprintf("Installing app: %s", cfg.AppFile))
		logger.Info("Installing iOS app: %s to device %s (simulator=%v)", cfg.AppFile, udid, isSimulator)
		if err := installIOSApp(udid, cfg.AppFile, isSimulator); err != nil {
			logger.Error("iOS app installation failed: %v", err)
			return nil, nil, fmt.Errorf("install app failed: %w", err)
		}
		logger.Info("iOS app installed successfully")
		printSetupSuccess("App installed")
	}

	// 2. Check if WDA is installed
	printSetupStep("Checking WDA installation...")
	if !wdadriver.IsWDAInstalled() {
		printSetupStep("Downloading WDA...")
		if _, err := wdadriver.Setup(); err != nil {
			return nil, nil, fmt.Errorf("WDA setup failed: %w", err)
		}
		printSetupSuccess("WDA installed")
	} else {
		printSetupSuccess("WDA already installed")
	}

	// 3. Create WDA runner
	printSetupStep("Building WDA...")
	logger.Info("Building WDA for device %s (team ID: %s)", udid, cfg.TeamID)
	runner := wdadriver.NewRunner(udid, cfg.TeamID)
	ctx := context.Background()

	if err := runner.Build(ctx); err != nil {
		logger.Error("WDA build failed: %v", err)
		return nil, nil, fmt.Errorf("WDA build failed: %w", err)
	}
	logger.Info("WDA build completed successfully")
	printSetupSuccess("WDA built")

	// 4. Start WDA
	printSetupStep("Starting WDA...")
	logger.Info("Starting WDA on device %s (port: %d)", udid, runner.Port())
	if err := runner.Start(ctx); err != nil {
		logger.Error("WDA start failed: %v", err)
		runner.Cleanup()
		return nil, nil, fmt.Errorf("WDA start failed: %w", err)
	}
	logger.Info("WDA started successfully on port %d", runner.Port())
	printSetupSuccess("WDA started")

	// 5. Create WDA client
	printSetupSuccess(fmt.Sprintf("WDA port: %d", runner.Port()))
	client := wdadriver.NewClient(runner.Port())

	// 6. Get device info
	deviceInfo, err := getIOSDeviceInfo(udid)
	if err != nil {
		runner.Cleanup()
		return nil, nil, fmt.Errorf("get device info: %w", err)
	}

	// 7. Query app version if appId is known (only works for simulators)
	appVersion := ""
	if cfg.AppID != "" && isSimulator {
		appVersion = getIOSAppVersion(udid, cfg.AppID)
	}

	platformInfo := &core.PlatformInfo{
		Platform:    "ios",
		OSVersion:   deviceInfo.OSVersion,
		DeviceName:  deviceInfo.Name,
		DeviceID:    udid,
		IsSimulator: deviceInfo.IsSimulator,
		AppID:       cfg.AppID,
		AppVersion:  appVersion,
	}

	// 8. Create driver
	driver := wdadriver.NewDriver(client, platformInfo, udid)

	// Cleanup function
	cleanup := func() {
		runner.Cleanup()
	}

	return driver, cleanup, nil
}

// findIOSDevice finds an available iOS device (booted simulator or connected physical device).
// Prefers simulators over physical devices.
func findIOSDevice() (string, error) {
	// First, try to find a booted simulator
	udid, err := findBootedSimulator()
	if err == nil && udid != "" {
		return udid, nil
	}

	// No simulator found, try to find a connected physical device
	udid, err = findConnectedDevice()
	if err == nil && udid != "" {
		return udid, nil
	}

	return "", fmt.Errorf("no iOS device found (no booted simulator or connected physical device)")
}

// findBootedSimulator finds the UDID of a booted iOS simulator.
func findBootedSimulator() (string, error) {
	out, err := runCommand("xcrun", "simctl", "list", "devices", "booted", "-j")
	if err != nil {
		return "", err
	}

	// Parse JSON to find booted device
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return "", err
	}

	devices, ok := data["devices"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no devices in simctl output")
	}

	for _, deviceList := range devices {
		if list, ok := deviceList.([]interface{}); ok {
			for _, device := range list {
				if deviceMap, ok := device.(map[string]interface{}); ok {
					if udid, ok := deviceMap["udid"].(string); ok && udid != "" {
						return udid, nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("no booted simulator found")
}

// findConnectedDevice finds a connected physical iOS device using idevice_id.
func findConnectedDevice() (string, error) {
	out, err := runCommand("idevice_id", "-l")
	if err != nil {
		return "", fmt.Errorf("idevice_id failed: %w (is libimobiledevice installed?)", err)
	}

	// idevice_id -l outputs one UDID per line
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		udid := strings.TrimSpace(line)
		if udid != "" {
			return udid, nil
		}
	}

	return "", fmt.Errorf("no connected physical device found")
}

// isIOSSimulator checks if the given UDID is a simulator.
func isIOSSimulator(udid string) bool {
	cmd := exec.Command("xcrun", "simctl", "list", "devices", "-j")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	var data map[string]interface{}
	if err := json.Unmarshal(output, &data); err != nil {
		return false
	}

	devices, ok := data["devices"].(map[string]interface{})
	if !ok {
		return false
	}

	for _, deviceList := range devices {
		if list, ok := deviceList.([]interface{}); ok {
			for _, device := range list {
				if deviceMap, ok := device.(map[string]interface{}); ok {
					if deviceUDID, ok := deviceMap["udid"].(string); ok && deviceUDID == udid {
						return true
					}
				}
			}
		}
	}

	return false
}

// getPhysicalDeviceInfo gets information about a physical iOS device using ideviceinfo.
func getPhysicalDeviceInfo(udid string) (*iosDeviceInfo, error) {
	// Get device name
	nameOut, err := runCommand("ideviceinfo", "-u", udid, "-k", "DeviceName")
	if err != nil {
		return nil, fmt.Errorf("failed to get device name: %w (is the device connected and trusted?)", err)
	}
	name := strings.TrimSpace(nameOut)

	// Get iOS version
	versionOut, err := runCommand("ideviceinfo", "-u", udid, "-k", "ProductVersion")
	if err != nil {
		return nil, fmt.Errorf("failed to get device version: %w", err)
	}
	version := strings.TrimSpace(versionOut)

	return &iosDeviceInfo{
		Name:        name,
		OSVersion:   version,
		IsSimulator: false,
	}, nil
}

// getIOSDeviceInfo gets information about an iOS device (simulator or physical).
func getIOSDeviceInfo(udid string) (*iosDeviceInfo, error) {
	if isIOSSimulator(udid) {
		simInfo, err := getSimulatorInfo(udid)
		if err != nil {
			return nil, err
		}
		return &iosDeviceInfo{
			Name:        simInfo.Name,
			OSVersion:   simInfo.OSVersion,
			IsSimulator: true,
		}, nil
	}

	return getPhysicalDeviceInfo(udid)
}

// installIOSApp installs an app on an iOS device (simulator or physical).
func installIOSApp(udid string, appPath string, isSimulator bool) error {
	if isSimulator {
		out, err := runCommand("xcrun", "simctl", "install", udid, appPath)
		if err != nil {
			return fmt.Errorf("simctl install failed: %w\nOutput: %s", err, out)
		}
		return nil
	}

	// Physical device - use ios-deploy
	out, err := runCommand("ios-deploy", "--bundle", appPath, "--id", udid, "--no-wifi")
	if err != nil {
		return fmt.Errorf("ios-deploy failed: %w\nOutput: %s\nHint: Install ios-deploy with 'brew install ios-deploy'", err, out)
	}
	return nil
}

// getSimulatorInfo gets information about an iOS simulator.
func getSimulatorInfo(udid string) (*simulatorInfo, error) {
	out, err := runCommand("xcrun", "simctl", "list", "devices", "-j")
	if err != nil {
		return nil, err
	}

	// Parse JSON properly
	var data struct {
		Devices map[string][]struct {
			Name  string `json:"name"`
			UDID  string `json:"udid"`
			State string `json:"state"`
		} `json:"devices"`
	}

	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil, fmt.Errorf("failed to parse simctl output: %w", err)
	}

	// Search for the device by UDID
	for runtime, devices := range data.Devices {
		for _, device := range devices {
			if device.UDID == udid {
				// Extract iOS version from runtime string
				// Example: "com.apple.CoreSimulator.SimRuntime.iOS-26-1" -> "26.1"
				osVersion := extractIOSVersion(runtime)
				return &simulatorInfo{
					Name:      device.Name,
					OSVersion: osVersion,
					State:     device.State,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("simulator %s not found", udid)
}

// extractIOSVersion extracts the iOS version from a runtime string.
// Example: "com.apple.CoreSimulator.SimRuntime.iOS-26-1" -> "26.1"
func extractIOSVersion(runtime string) string {
	// Look for iOS version pattern
	parts := strings.Split(runtime, ".")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		if strings.HasPrefix(lastPart, "iOS-") {
			version := strings.TrimPrefix(lastPart, "iOS-")
			version = strings.ReplaceAll(version, "-", ".")
			return version
		}
	}
	return runtime
}

// getIOSAppVersion queries the iOS simulator for an app's version.
func getIOSAppVersion(udid, bundleID string) string {
	if bundleID == "" {
		return ""
	}

	// Get app container path
	out, err := runCommand("xcrun", "simctl", "get_app_container", udid, bundleID)
	if err != nil {
		return ""
	}

	appPath := strings.TrimSpace(out)
	if appPath == "" {
		return ""
	}

	// Read Info.plist from app bundle
	plistPath := filepath.Join(appPath, "Info.plist")
	version, err := runCommand("/usr/libexec/PlistBuddy", "-c", "Print CFBundleShortVersionString", plistPath)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(version)
}

// autoDetectIOSDevices finds N available iOS simulators/devices.
func autoDetectIOSDevices(count int) ([]string, error) {
	// List booted simulators
	out, err := runCommand("xcrun", "simctl", "list", "devices", "booted", "-j")
	if err != nil {
		return nil, fmt.Errorf("failed to list iOS devices: %w", err)
	}

	// Parse JSON to find booted device UDIDs
	var devices []string
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, `"udid"`) {
			// Extract UDID from "udid" : "XXXX-XXXX"
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				udid := strings.Trim(parts[1], ` ",`)
				if udid != "" {
					devices = append(devices, udid)
				}
			}
		}
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no booted iOS simulators found\nHint: Start %d simulator(s) or specify devices with --device", count)
	}

	// Return up to count devices
	if len(devices) > count {
		devices = devices[:count]
	}

	if len(devices) < count {
		return nil, fmt.Errorf("found %d booted simulators but need %d\nHint: Start more simulators or use --parallel %d", len(devices), count, len(devices))
	}

	return devices, nil
}
