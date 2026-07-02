package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/zipconduit"
	"github.com/devicelab-dev/maestro-runner/pkg/core"
	wdadriver "github.com/devicelab-dev/maestro-runner/pkg/driver/wda"
	"github.com/devicelab-dev/maestro-runner/pkg/flutter"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// installTimeout is the max time we wait for an iOS app install to complete.
// Big apps on slow USB can legitimately take a while; 3 minutes is a generous
// upper bound that still catches infinite hangs.
const installTimeout = 3 * time.Minute

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

// CreateIOSDriver creates an iOS driver using WebDriverAgent.
// Exported for library use.
func CreateIOSDriver(cfg *RunConfig) (core.Driver, func(), error) {
	// Phase 4 — devicelab driver branch. Mirrors --driver devicelab on Android.
	// Routes to the XCUITest-based runner (pkg/driver/devicelab_ios) instead
	// of WebDriverAgent. Simulator-only in Phase 4.
	if strings.EqualFold(cfg.Driver, "devicelab") {
		return createDevicelabIOSDriver(cfg)
	}

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
	if cfg.AppFile != "" && !cfg.NoAppInstall {
		printSetupStep(fmt.Sprintf("Installing app: %s", cfg.AppFile))
		logger.Info("Installing iOS app: %s to device %s (simulator=%v)", cfg.AppFile, udid, isSimulator)
		if err := installIOSApp(udid, cfg.AppFile, isSimulator); err != nil {
			logger.Error("iOS app installation failed: %v", err)
			return nil, nil, fmt.Errorf("install app failed: %w", err)
		}
		logger.Info("iOS app installed successfully")
		printSetupSuccess("App installed")
	}

	// 2. Verify bundled WDA is present (no auto-download — releases ship WDA)
	if !cfg.NoDriverInstall {
		printSetupStep("Checking WDA installation...")
		if _, err := wdadriver.Setup(); err != nil {
			return nil, nil, fmt.Errorf("WDA setup failed: %w", err)
		}
		printSetupSuccess("WDA installed")
	}

	// 3. Create WDA runner
	printSetupStep("Building WDA...")
	logger.Info("Building WDA for device %s (team ID: %s)", udid, cfg.TeamID)
	runner := wdadriver.NewRunner(udid, cfg.TeamID, cfg.WDABundleID)
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

	// 8. Get screen size
	var screenW, screenH int
	if w, h, err := client.WindowSize(); err == nil {
		screenW, screenH = w, h
	}

	platformInfo := &core.PlatformInfo{
		Platform:     "ios",
		OSVersion:    deviceInfo.OSVersion,
		DeviceName:   deviceInfo.Name,
		DeviceID:     udid,
		IsSimulator:  deviceInfo.IsSimulator,
		ScreenWidth:  screenW,
		ScreenHeight: screenH,
		AppID:        cfg.AppID,
		AppVersion:   appVersion,
	}

	// 9. Create driver
	wdaDrv := wdadriver.NewDriver(client, platformInfo, udid)
	wdaDrv.SetAppFile(cfg.AppFile)

	// Cleanup function
	cleanup := func() {
		runner.Cleanup()
	}

	var driver core.Driver = wdaDrv

	// 10. Wrap driver with Flutter VM Service fallback (simulator only)
	if !cfg.NoFlutterFallback && isSimulator {
		fw := flutter.WrapIOS(wdaDrv, nil, udid, cfg.AppID)
		driver = fw
		origCleanup := cleanup
		cleanup = func() {
			if fd, ok := fw.(*flutter.FlutterDriver); ok {
				fd.Close()
			}
			origCleanup()
		}
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

// iosTargetsSimulator reports whether an iOS WDA run will target a simulator
// rather than a real device, so --team-id (code signing) and --app-file aren't
// required. --auto-start-emulator and --start-simulator both imply simulators
// (you can't auto-create a real device), and crucially this is evaluated before
// any simulator is booted — so it must not depend on hasBootedSimulator() in
// those cases, which is the bug behind --auto-start-emulator erroring for
// simulators (#111). Otherwise fall back to the named device, or a
// currently-booted simulator.
func iosTargetsSimulator(cfg *RunConfig) bool {
	switch {
	case cfg.AutoStartEmulator, cfg.StartSimulator != "":
		return true
	case len(cfg.Devices) > 0:
		return isIOSSimulator(cfg.Devices[0])
	default:
		return hasBootedSimulator()
	}
}

// hasBootedSimulator returns true if any iOS simulator is currently booted.
func hasBootedSimulator() bool {
	_, err := findBootedSimulator()
	return err == nil
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

	for runtime, deviceList := range devices {
		// Only consider iOS simulators — skip tvOS, watchOS, visionOS
		if !strings.Contains(runtime, "iOS-") {
			continue
		}
		if list, ok := deviceList.([]interface{}); ok {
			for _, device := range list {
				if deviceMap, ok := device.(map[string]interface{}); ok {
					if udid, ok := deviceMap["udid"].(string); ok && udid != "" {
						// Skip simulators whose WDA port is already in use
						port := wdadriver.PortFromUDID(udid)
						if isPortInUse(port) {
							logger.Info("Skipping booted simulator %s: port %d in use", udid, port)
							continue
						}
						return udid, nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("no available booted iOS simulator found")
}

// findConnectedDevice finds a connected physical iOS device using go-ios.
func findConnectedDevice() (string, error) {
	list, err := goios.ListDevices()
	if err != nil {
		return "", fmt.Errorf("failed to list devices: %w", err)
	}

	for _, d := range list.DeviceList {
		serial := d.Properties.SerialNumber
		if serial != "" {
			return serial, nil
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

// getPhysicalDeviceInfo gets information about a physical iOS device using go-ios.
func getPhysicalDeviceInfo(udid string) (*iosDeviceInfo, error) {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return nil, fmt.Errorf("device %s not found: %w (is the device connected and trusted?)", udid, err)
	}

	values, err := goios.GetValues(entry)
	if err != nil {
		return nil, fmt.Errorf("failed to get device info: %w", err)
	}

	name := values.Value.DeviceName
	if name == "" {
		name = values.Value.ProductType
	}
	if name == "" {
		name = "iOS Device"
	}

	return &iosDeviceInfo{
		Name:        name,
		OSVersion:   values.Value.ProductVersion,
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
//
// Strategy for physical devices:
//  1. Prefer `xcrun devicectl device install app` (Apple's modern CoreDevice
//     installer, iOS 17+ friendly) — set MAESTRO_RUNNER_IOS_INSTALLER=zipconduit
//     to skip.
//  2. Fall back to go-ios zipconduit for older Xcode / macOS or on devicectl
//     failure. Both paths run with installTimeout so an unresponsive install
//     service surfaces as an error instead of hanging forever.
func installIOSApp(udid string, appPath string, isSimulator bool) error {
	if isSimulator {
		return installViaSimctl(udid, appPath)
	}

	installer := strings.ToLower(strings.TrimSpace(os.Getenv("MAESTRO_RUNNER_IOS_INSTALLER")))

	// Default: prefer devicectl, fall back to zipconduit.
	if installer != "zipconduit" && devicectlAvailable() {
		if err := installViaDevicectl(udid, appPath); err == nil {
			return nil
		} else if installer == "devicectl" {
			// User explicitly forced devicectl — propagate the error, don't silently fall back.
			return err
		} else {
			logger.Warn("devicectl install failed, falling back to zipconduit: %v", err)
		}
	}

	return installViaZipconduit(udid, appPath)
}

// installViaSimctl installs on a simulator. Wrapped in a timeout so a stuck
// simulator can't freeze the whole run.
func installViaSimctl(udid, appPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xcrun", "simctl", "install", udid, appPath).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("simctl install timed out after %v", installTimeout)
	}
	if err != nil {
		return fmt.Errorf("simctl install failed: %w\nOutput: %s", err, out)
	}
	return nil
}

// devicectlAvailable reports whether `xcrun devicectl` works on this host.
// Requires macOS 14 / Xcode 15+.
func devicectlAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := exec.CommandContext(ctx, "xcrun", "devicectl", "--version").Run()
	return err == nil
}

// installViaDevicectl uses Apple's modern CoreDevice installer.
// Supported on iOS 17+ real devices; also works on earlier iOS via Xcode 15+.
func installViaDevicectl(udid, appPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xcrun", "devicectl", "device", "install", "app",
		"--device", udid, appPath).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("devicectl install timed out after %v", installTimeout)
	}
	if err != nil {
		return fmt.Errorf("devicectl install failed: %w\nOutput: %s", err, string(out))
	}
	return nil
}

// installViaZipconduit uses the go-ios Go-native installer. Kept as a fallback
// for hosts without devicectl (older macOS / Xcode). Wrapped in a timeout —
// without it, SendFile can hang indefinitely when the install service accepts
// the connection but never acks completion (observed on iOS 26 / iPhone 13).
func installViaZipconduit(udid, appPath string) error {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return fmt.Errorf("device %s not found: %w", udid, err)
	}
	conn, err := zipconduit.New(entry)
	if err != nil {
		return fmt.Errorf("failed to connect to device install service: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- conn.SendFile(appPath) }()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to install app: %w", err)
		}
		return nil
	case <-time.After(installTimeout):
		return fmt.Errorf("app install timed out after %v (try: xcrun devicectl device install app --device %s %s)",
			installTimeout, udid, appPath)
	}
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

// autoDetectIOSDevices finds up to N available booted iOS simulators that are not in use.
// Excludes tvOS, watchOS, visionOS simulators and simulators whose WDA port is already bound.
// Returns available devices (may be fewer than count) and an error only if zero found.
func autoDetectIOSDevices(count int) ([]string, error) {
	out, err := runCommand("xcrun", "simctl", "list", "devices", "booted", "-j")
	if err != nil {
		return nil, fmt.Errorf("failed to list iOS devices: %w", err)
	}

	var data struct {
		Devices map[string][]struct {
			UDID string `json:"udid"`
		} `json:"devices"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil, fmt.Errorf("failed to parse simctl output: %w", err)
	}

	var devices []string
	for runtime, devList := range data.Devices {
		// Only consider iOS simulators
		if !strings.Contains(runtime, "iOS-") {
			continue
		}
		for _, dev := range devList {
			if dev.UDID == "" {
				continue
			}
			// Skip simulators whose WDA port is already in use
			port := wdadriver.PortFromUDID(dev.UDID)
			if isPortInUse(port) {
				logger.Info("Skipping booted simulator %s: port %d in use", dev.UDID, port)
				continue
			}
			devices = append(devices, dev.UDID)
		}
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no available booted iOS simulators found\nHint: Start %d simulator(s) or specify devices with --device", count)
	}

	// Return up to count devices
	if len(devices) > count {
		devices = devices[:count]
	}

	return devices, nil
}
