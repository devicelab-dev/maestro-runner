package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/device"
	uia2driver "github.com/devicelab-dev/maestro-runner/pkg/driver/uiautomator2"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// createAndroidDriver creates an Android driver based on cfg.Driver type.
func createAndroidDriver(cfg *RunConfig) (core.Driver, func(), error) {
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

	// 2. Check if device is already in use (for UIAutomator2 driver)
	// Do this BEFORE installing app to fail fast
	if driverType == "uiautomator2" {
		socketPath := dev.DefaultSocketPath()
		if isSocketInUse(socketPath) {
			return nil, nil, fmt.Errorf("device %s is already in use\n"+
				"Another maestro-runner instance may be using this device.\n"+
				"Socket: %s\n"+
				"Hint: Wait for it to finish or use a different device", dev.Serial(), socketPath)
		}
	}

	// 3. Install app if specified
	if cfg.AppFile != "" {
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
	switch driverType {
	case "uiautomator2":
		return createUIAutomator2Driver(cfg, dev, info)
	case "appium":
		return createAppiumDriver(cfg)
	default:
		return nil, nil, fmt.Errorf("unsupported driver: %s (use uiautomator2 or appium)", driverType)
	}
}

// createUIAutomator2Driver creates a direct UIAutomator2 driver (no Appium server needed).
func createUIAutomator2Driver(cfg *RunConfig, dev *device.AndroidDevice, info device.DeviceInfo) (core.Driver, func(), error) {
	// 1. Check/install UIAutomator2 APKs
	if !dev.IsInstalled(device.UIAutomator2Server) {
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
		dev.StopUIAutomator2()
		return nil, nil, fmt.Errorf("create session: %w", err)
	}
	logger.Info("Session created successfully: %s", client.SessionID())
	printSetupSuccess("Session created")

	// Set waitForIdle timeout - configurable via --wait-for-idle-timeout or config.yaml
	// Default is 5000ms which balances speed and reliability
	// Set to 0 to disable (faster but may miss animations)
	if err := client.SetAppiumSettings(map[string]interface{}{
		"waitForIdleTimeout": cfg.WaitForIdleTimeout,
	}); err != nil {
		fmt.Printf("  %s⚠%s Warning: failed to set appium settings: %v\n", color(colorYellow), color(colorReset), err)
	}

	// 5. Query app version from device if appId is known
	appVersion := ""
	if cfg.AppID != "" {
		appVersion = dev.GetAppVersion(cfg.AppID)
	}

	// 6. Create driver
	platformInfo := &core.PlatformInfo{
		Platform:    "android",
		DeviceID:    info.Serial,
		DeviceName:  fmt.Sprintf("%s %s", info.Brand, info.Model),
		OSVersion:   info.SDK,
		IsSimulator: info.IsEmulator,
		AppID:       cfg.AppID,
		AppVersion:  appVersion,
	}
	driver := uia2driver.New(client, platformInfo, dev)

	// Cleanup function (silent)
	cleanup := func() {
		client.Close()
		dev.StopUIAutomator2()
	}

	return driver, cleanup, nil
}

// autoDetectAndroidDevices finds N available Android devices.
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
			devices = append(devices, parts[0])
		}
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no Android devices found")
	}

	// Return up to count devices
	if len(devices) > count {
		devices = devices[:count]
	}

	return devices, nil
}
