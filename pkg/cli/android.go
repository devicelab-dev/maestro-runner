package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/device"
	devicelabdriver "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab"
	uia2driver "github.com/devicelab-dev/maestro-runner/pkg/driver/uiautomator2"
	"github.com/devicelab-dev/maestro-runner/pkg/flutter"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/maestro"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// useAndroidTCPForward decides whether to force TCP-to-TCP adb forwards
// (instead of the default Linux/Mac unix-socket forward) for Android
// drivers. Returns true when either the explicit cfg flag is set, OR
// $DEVICEFARM_DEVICE_UDID is in the environment (AWS Device Farm test
// spec sets that on every run — its adb-proxy sandbox blocks
// localfilesystem:/localabstract: forwards but allows plain tcp:,
// see issue #83).
func useAndroidTCPForward(cfg *RunConfig) bool {
	if cfg != nil && cfg.AndroidTCPForward {
		return true
	}
	if os.Getenv("DEVICEFARM_DEVICE_UDID") != "" {
		return true
	}
	return false
}

// CreateAndroidDriver creates an Android driver based on cfg.Driver type.
// Exported for library use.
func CreateAndroidDriver(cfg *RunConfig) (core.Driver, func(), error) {
	driverType := strings.ToLower(cfg.Driver)
	if driverType == "" {
		driverType = "uiautomator2"
	}

	// 1. Connect to device
	deviceID := getFirstDevice(cfg)
	if deviceID != "" {
		printSetupStep(fmt.Sprintf("Connecting to device %s...", deviceID))
		logger.Info("Connecting to Android device: %s", deviceID)
	} else {
		printSetupStep("Connecting to device...")
		logger.Info("Auto-detecting Android device...")
	}
	dev, err := device.New(deviceID)
	if err != nil {
		logger.Error("Failed to connect to device: %v", err)

		// Enhance NoDevicesError with actual command context
		var noDevErr *device.NoDevicesError
		if errors.As(err, &noDevErr) {
			enhanceNoDevicesError(noDevErr, cfg)
		}

		return nil, nil, fmt.Errorf("connect to device: %w", err)
	}

	// Get device info for reporting
	info, err := dev.Info()
	if err != nil {
		logger.Error("Failed to get device info: %v", err)
		return nil, nil, fmt.Errorf("get device info: %w", err)
	}
	logger.Info("Device info: %s %s, SDK %s, Serial %s, Emulator: %v",
		info.Brand, info.Model, info.SDK, info.Serial, info.IsEmulator)
	printSetupSuccess(fmt.Sprintf("Connected to %s %s (SDK %s)", info.Brand, info.Model, info.SDK))

	// 2. Check if device is already in use
	// Do this BEFORE starting servers which would kill the other instance's server
	switch driverType {
	case "uiautomator2":
		socketPath := dev.DefaultSocketPath()
		if device.IsOwnerAlive(socketPath) {
			return nil, nil, fmt.Errorf("device %s is already in use\n"+
				"Another maestro-runner instance may be using this device.\n"+
				"Socket: %s\n"+
				"Hint: Wait for it to finish or use a different device", dev.Serial(), socketPath)
		}
	case "devicelab":
		socketPath := dev.DeviceLabDriverSocketPath()
		if device.IsOwnerAlive(socketPath) {
			return nil, nil, fmt.Errorf("device %s is already in use by DeviceLab driver\n"+
				"Another maestro-runner instance may be using this device.\n"+
				"Socket: %s\n"+
				"Hint: Wait for it to finish or use a different device", dev.Serial(), socketPath)
		}
	}

	// 3. Install app if specified
	if cfg.AppFile != "" && !cfg.NoAppInstall {
		printSetupStep(fmt.Sprintf("Installing app: %s", cfg.AppFile))
		logger.Info("Installing app: %s", cfg.AppFile)
		if err := dev.Install(cfg.AppFile); err != nil {
			logger.Error("App installation failed: %v", err)
			return nil, nil, fmt.Errorf("install app: %w", err)
		}
		logger.Info("App installed successfully")
		printSetupSuccess("App installed")
	}

	// 4. Create driver based on type
	var driver core.Driver
	var cleanup func()

	switch driverType {
	case "uiautomator2":
		driver, cleanup, err = createUIAutomator2Driver(cfg, dev, info)
	case "devicelab":
		driver, cleanup, err = createDeviceLabDriver(cfg, dev, info)
	case "appium":
		return createAppiumDriver(cfg)
	default:
		return nil, nil, fmt.Errorf("unsupported driver: %s (use uiautomator2, devicelab, or appium)", driverType)
	}
	if err != nil {
		return nil, nil, err
	}

	// 5. Wrap driver with Flutter VM Service fallback (lazy connection)
	if !cfg.NoFlutterFallback {
		socketPath := fmt.Sprintf("/tmp/%s-flutter.sock", dev.Serial())
		fw := flutter.Wrap(driver, nil, dev, cfg.AppID, socketPath)
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

// createUIAutomator2Driver creates a direct UIAutomator2 driver (no Appium server needed).
func createUIAutomator2Driver(cfg *RunConfig, dev *device.AndroidDevice, info device.DeviceInfo) (core.Driver, func(), error) {
	// 1. Check/install UIAutomator2 APKs
	if !cfg.NoDriverInstall && !dev.IsInstalled(device.UIAutomator2Server) {
		printSetupStep("Installing UIAutomator2 APKs...")
		apksDir, err := getDriversDir("android")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to locate drivers directory: %w", err)
		}
		if err := dev.InstallUIAutomator2(apksDir); err != nil {
			return nil, nil, fmt.Errorf("install UIAutomator2: %w", err)
		}
		printSetupSuccess("UIAutomator2 installed")
	}

	// 2. Start UIAutomator2 server
	printSetupStep("Starting UIAutomator2 server...")
	logger.Info("Starting UIAutomator2 server on device %s", dev.Serial())
	uia2Cfg := device.DefaultUIAutomator2Config()
	if cfg.DriverStartTimeout > 0 {
		uia2Cfg.Timeout = time.Duration(cfg.DriverStartTimeout) * time.Second
		logger.Info("Overriding UIAutomator2 start timeout to %v (--driver-start-timeout)", uia2Cfg.Timeout)
	}
	if useAndroidTCPForward(cfg) {
		uia2Cfg.TCPForward = true
		logger.Info("Android: using TCP-to-TCP adb forward (AWS DF / --android-tcp-forward)")
	}
	if err := dev.StartUIAutomator2(uia2Cfg); err != nil {
		logger.Error("Failed to start UIAutomator2: %v", err)
		return nil, nil, fmt.Errorf("start UIAutomator2: %w", err)
	}

	// Debug: Print socket/port info
	if dev.SocketPath() != "" {
		fmt.Printf("  → Socket: %s\n", dev.SocketPath())
	} else if dev.LocalPort() != 0 {
		fmt.Printf("  → Port: %d\n", dev.LocalPort())
	}

	// Verify server is actually responding
	if !dev.IsUIAutomator2Running() {
		return nil, nil, fmt.Errorf("UIAutomator2 server not responding after start")
	}
	printSetupSuccess("UIAutomator2 server started")

	// 3. Create client
	var client *uiautomator2.Client
	if dev.SocketPath() != "" {
		client = uiautomator2.NewClient(dev.SocketPath())
	} else {
		client = uiautomator2.NewClientTCP(dev.LocalPort())
	}

	// Set log path to report folder
	if cfg.OutputDir != "" {
		client.SetLogPath(filepath.Join(cfg.OutputDir, "client.log"))
	}

	// 4. Create session
	printSetupStep("Creating session...")
	logger.Info("Creating UIAutomator2 session with capabilities: Platform=Android, Device=%s", info.Model)
	caps := uiautomator2.Capabilities{
		PlatformName: "Android",
		DeviceName:   info.Model,
	}
	if err := client.CreateSession(caps); err != nil {
		logger.Error("Failed to create session: %v", err)
		if stopErr := dev.StopUIAutomator2(); stopErr != nil {
			logger.Warn("failed to stop UIAutomator2 after session failure: %v", stopErr)
		}
		return nil, nil, fmt.Errorf("create session: %w", err)
	}
	logger.Info("Session created successfully: %s", client.SessionID())
	printSetupSuccess("Session created")

	// Set waitForIdle timeout - configurable via --wait-for-idle-timeout or config.yaml
	// Default is 5000ms which balances speed and reliability
	// Set to 0 to disable (faster but may miss animations)
	//
	// enableMultiWindows=true makes the server's page source and element search
	// span ALL accessibility windows, not just the focused one. Without it,
	// content in a separate window — AlertDialogs, permission prompts, and
	// Material ExposedDropdownMenu / Spinner popups (ListPopupWindow) — is
	// invisible, so tapOn an item reports "no such element" even though it is on
	// screen (issue #93). Stock Maestro's instrumentation always traverses all
	// windows; this aligns the uiautomator2 driver with that behavior.
	if err := client.SetAppiumSettings(map[string]interface{}{
		"waitForIdleTimeout": cfg.WaitForIdleTimeout,
		"enableMultiWindows": true,
	}); err != nil {
		fmt.Printf("  %s⚠%s Warning: failed to set appium settings: %v\n", color(colorYellow), color(colorReset), err)
	}

	// Enable server-side element polling (implicitWait=100ms).
	// Each FindElement call polls on-device for up to 100ms before returning,
	// catching elements that appear mid-animation without extra round-trips.
	if err := client.SetImplicitWait(100 * time.Millisecond); err != nil {
		fmt.Printf("  %s⚠%s Warning: failed to set implicit wait: %v\n", color(colorYellow), color(colorReset), err)
	}

	// 5. Query app version from device if appId is known
	appVersion := ""
	if cfg.AppID != "" {
		appVersion = dev.GetAppVersion(cfg.AppID)
	}

	// 6. Get screen size
	var screenW, screenH int
	devInfo, err := client.GetDeviceInfo()
	if err == nil && devInfo.RealDisplaySize != "" {
		parts := strings.Split(devInfo.RealDisplaySize, "x")
		if len(parts) == 2 {
			screenW, _ = strconv.Atoi(parts[0])
			screenH, _ = strconv.Atoi(parts[1])
		}
	}
	if screenW == 0 || screenH == 0 {
		// Fallback: wm size
		if output, err := dev.Shell("wm size"); err == nil {
			output = strings.TrimSpace(output)
			if idx := strings.LastIndex(output, ":"); idx != -1 {
				output = strings.TrimSpace(output[idx+1:])
			}
			parts := strings.Split(output, "x")
			if len(parts) == 2 {
				screenW, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
				screenH, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		}
	}

	// 7. Create driver
	platformInfo := &core.PlatformInfo{
		Platform:     "android",
		DeviceID:     info.Serial,
		DeviceName:   fmt.Sprintf("%s %s", info.Brand, info.Model),
		OSVersion:    info.SDK,
		IsSimulator:  info.IsEmulator,
		ScreenWidth:  screenW,
		ScreenHeight: screenH,
		AppID:        cfg.AppID,
		AppVersion:   appVersion,
	}
	driver := uia2driver.New(client, platformInfo, dev)

	// Cleanup function (silent)
	cleanup := func() {
		if err := client.Close(); err != nil {
			logger.Debug("failed to close client during cleanup: %v", err)
		}
		if err := dev.StopUIAutomator2(); err != nil {
			logger.Warn("failed to stop UIAutomator2 during cleanup: %v", err)
		}
	}

	return driver, cleanup, nil
}

// autoDetectAndroidDevices finds up to N available Android devices that are not in use.
// Skips devices whose UIAutomator2 socket is already bound by another maestro-runner instance.
// Returns available devices (may be fewer than count) and an error only if zero found.
func autoDetectAndroidDevices(count int) ([]string, error) {
	// Use adb devices to list all connected devices
	cmd := exec.Command("adb", "devices")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list Android devices: %w", err)
	}

	// Parse output to find device serials
	var devices []string
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") || strings.HasPrefix(line, "*") {
			continue
		}
		// Line format: "serial\tdevice"
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == "device" {
			serial := parts[0]
			// Skip devices whose socket is already in use
			socketPath := fmt.Sprintf("/tmp/uia2-%s.sock", serial)
			if isSocketInUse(socketPath) {
				logger.Info("Skipping device %s: socket %s in use", serial, socketPath)
				continue
			}
			devices = append(devices, serial)
		}
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no available Android devices found")
	}

	// Return up to count devices
	if len(devices) > count {
		devices = devices[:count]
	}

	return devices, nil
}

// createDeviceLabDriver creates a DeviceLab Android Driver (WebSocket-based).
// Uses the same uiautomator2.Driver but with a WebSocket transport instead of HTTP.
func createDeviceLabDriver(cfg *RunConfig, dev *device.AndroidDevice, info device.DeviceInfo) (core.Driver, func(), error) {
	// 1. Check/install DeviceLab driver APKs (always check for updates)
	if !cfg.NoDriverInstall {
		apksDir, err := getDriversDir("android")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to locate drivers directory: %w", err)
		}
		if err := dev.InstallDeviceLabDriver(apksDir); err != nil {
			return nil, nil, fmt.Errorf("install DeviceLab driver: %w", err)
		}
	}

	// 2. Start DeviceLab Android Driver
	printSetupStep("Starting DeviceLab driver...")
	logger.Info("Starting DeviceLab driver on device %s", dev.Serial())
	driverCfg := device.DefaultDeviceLabDriverConfig()
	if cfg.DriverStartTimeout > 0 {
		driverCfg.Timeout = time.Duration(cfg.DriverStartTimeout) * time.Second
		logger.Info("Overriding DeviceLab driver start timeout to %v (--driver-start-timeout)", driverCfg.Timeout)
	}
	if useAndroidTCPForward(cfg) {
		driverCfg.TCPForward = true
		logger.Info("Android: using TCP-to-TCP adb forward (AWS DF / --android-tcp-forward)")
	}
	if err := dev.StartDeviceLabDriver(driverCfg); err != nil {
		logger.Error("Failed to start DeviceLab driver: %v", err)
		return nil, nil, fmt.Errorf("start DeviceLab driver: %w", err)
	}

	if dev.DeviceLabDriverSocket() != "" {
		fmt.Printf("  → Socket: %s\n", dev.DeviceLabDriverSocket())
	} else if dev.DeviceLabDriverLocalPort() != 0 {
		fmt.Printf("  → Port: %d\n", dev.DeviceLabDriverLocalPort())
	}

	if !dev.IsDeviceLabDriverRunning() {
		return nil, nil, fmt.Errorf("DeviceLab driver not responding after start")
	}
	printSetupSuccess("DeviceLab driver started")

	// 3. Create WebSocket client
	var wsClient *maestro.Client
	if dev.DeviceLabDriverSocket() != "" {
		wsClient = maestro.NewClient(dev.DeviceLabDriverSocket())
	} else {
		wsClient = maestro.NewClientTCP(dev.DeviceLabDriverLocalPort())
	}

	if cfg.OutputDir != "" {
		wsClient.SetLogPath(filepath.Join(cfg.OutputDir, "client.log"))
	}

	printSetupStep("Connecting WebSocket...")
	if err := wsClient.Connect(); err != nil {
		logger.Error("Failed to connect WebSocket: %v", err)
		if stopErr := dev.StopDeviceLabDriver(); stopErr != nil {
			logger.Warn("failed to stop DeviceLab driver after connect failure: %v", stopErr)
		}
		return nil, nil, fmt.Errorf("connect WebSocket: %w", err)
	}
	printSetupSuccess("WebSocket connected")

	// 4. Create adapter and session
	adapter := maestro.NewAdapter(wsClient)

	printSetupStep("Creating session...")
	logger.Info("Creating DeviceLab driver session")
	session, err := adapter.CreateSession()
	if err != nil {
		logger.Error("Failed to create session: %v", err)
		wsClient.Close()
		if stopErr := dev.StopDeviceLabDriver(); stopErr != nil {
			logger.Warn("failed to stop DeviceLab driver after session failure: %v", stopErr)
		}
		return nil, nil, fmt.Errorf("create session: %w", err)
	}
	logger.Info("Session created: %s", session.SessionID)
	printSetupSuccess("Session created")

	// Set waitForIdle timeout
	if err := adapter.SetAppiumSettings(map[string]interface{}{
		"waitForIdleTimeout": cfg.WaitForIdleTimeout,
	}); err != nil {
		fmt.Printf("  %s⚠%s Warning: failed to set driver settings: %v\n", color(colorYellow), color(colorReset), err)
	}

	// Enable server-side element polling (implicitWait=100ms).
	// Each FindElement call polls on-device for up to 100ms before returning,
	// catching elements that appear mid-animation without extra round-trips.
	if err := adapter.SetImplicitWait(100 * time.Millisecond); err != nil {
		fmt.Printf("  %s⚠%s Warning: failed to set implicit wait: %v\n", color(colorYellow), color(colorReset), err)
	}

	// 5. Query app version
	appVersion := ""
	if cfg.AppID != "" {
		appVersion = dev.GetAppVersion(cfg.AppID)
	}

	// 6. Get screen size from session device info
	var screenW, screenH int
	if session.DeviceInfo.DisplaySize != "" {
		parts := strings.Split(session.DeviceInfo.DisplaySize, "x")
		if len(parts) == 2 {
			screenW, _ = strconv.Atoi(parts[0])
			screenH, _ = strconv.Atoi(parts[1])
		}
	}
	if screenW == 0 || screenH == 0 {
		// Fallback: wm size
		if output, err := dev.Shell("wm size"); err == nil {
			output = strings.TrimSpace(output)
			if idx := strings.LastIndex(output, ":"); idx != -1 {
				output = strings.TrimSpace(output[idx+1:])
			}
			parts := strings.Split(output, "x")
			if len(parts) == 2 {
				screenW, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
				screenH, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		}
	}

	// 7. Create driver (DeviceLab-specific driver with RPC support)
	platformInfo := &core.PlatformInfo{
		Platform:     "android",
		DeviceID:     info.Serial,
		DeviceName:   fmt.Sprintf("%s %s", info.Brand, info.Model),
		OSVersion:    info.SDK,
		IsSimulator:  info.IsEmulator,
		ScreenWidth:  screenW,
		ScreenHeight: screenH,
		AppID:        cfg.AppID,
		AppVersion:   appVersion,
	}
	driver := devicelabdriver.New(adapter, platformInfo, dev)

	// Wire background CDP socket monitor (push events from Java driver, polls /proc/net/unix every 100ms)
	cdpTracker := maestro.NewCDPTracker(wsClient)
	driver.SetCDPStateFunc(func() *core.CDPInfo {
		state := cdpTracker.Latest()
		if state == nil {
			return nil
		}
		return &core.CDPInfo{
			Available: state.Available,
			Socket:    state.Socket,
		}
	})

	// Enable WebView CDP element finding via ADB socket forwarding
	driver.SetWebViewForwarder(dev)

	cleanup := func() {
		if err := adapter.DeleteSession(); err != nil {
			logger.Debug("failed to delete session during cleanup: %v", err)
		}
		if err := wsClient.Close(); err != nil {
			logger.Debug("failed to close WebSocket client during cleanup: %v", err)
		}
		if err := dev.StopDeviceLabDriver(); err != nil {
			logger.Warn("failed to stop DeviceLab driver during cleanup: %v", err)
		}
	}

	return driver, cleanup, nil
}
