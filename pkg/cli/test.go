package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/cloud"
	"github.com/devicelab-dev/maestro-runner/pkg/config"
	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/device"
	appiumdriver "github.com/devicelab-dev/maestro-runner/pkg/driver/appium"
	cdpdriver "github.com/devicelab-dev/maestro-runner/pkg/driver/browser/cdp"
	"github.com/devicelab-dev/maestro-runner/pkg/driver/mock"
	wdadriver "github.com/devicelab-dev/maestro-runner/pkg/driver/wda"
	"github.com/devicelab-dev/maestro-runner/pkg/emulator"
	"github.com/devicelab-dev/maestro-runner/pkg/executor"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
	"github.com/devicelab-dev/maestro-runner/pkg/simulator"
	"github.com/devicelab-dev/maestro-runner/pkg/validator"
	"github.com/urfave/cli/v2"
)

// activeCleanup holds the current driver cleanup function so the signal handler
// can clean up resources (sockets, adb forwards, sessions) before os.Exit.
var activeCleanup func()
var cleanupMu sync.Mutex

var testCommand = &cli.Command{
	Name:      "test",
	Usage:     "Run Maestro flows on a device",
	ArgsUsage: "<flow-file-or-folder>...",
	Description: `Run one or more Maestro flow files on a connected device.

Reports are generated in the output directory:
  - Default: ./reports/<timestamp>/
  - With --output: <output>/<timestamp>/
  - With --output and --flatten: <output>/ (no timestamp subfolder)

Examples:
 Basic usage
  maestro-runner test flow.yaml
  maestro-runner test flows/
  maestro-runner test login.yaml checkout.yaml

  # With environment variables
  maestro-runner test -e USER=test -e PASS=secret flows/

  # With tag filtering
  maestro-runner test --include-tags smoke flows/

  # With Appium and capabilities file
  maestro-runner --driver appium --caps caps.json test flow.yaml

  # With cloud provider (BrowserStack/SauceLabs/LambdaTest/TestingBot)
  maestro-runner --driver appium --appium-url "https://hub.provider.com/wd/hub" --caps cloud.json test flow.yaml

  # Custom output directory
  maestro-runner test --output ./my-reports --flatten flows/`,
	Flags: []cli.Flag{
		// Configuration
		&cli.StringFlag{
			Name:  "config",
			Usage: "Path to workspace config.yaml",
		},

		// Environment variables
		&cli.StringSliceFlag{
			Name:    "env",
			Aliases: []string{"e"},
			Usage:   "Environment variables (KEY=VALUE)",
		},
		&cli.StringFlag{
			Name:    "env-file",
			Usage:   "Path to .env file (KEY=VALUE per line, # comments, single/double quoting)",
			EnvVars: []string{"MAESTRO_ENV_FILE"},
		},

		// Tag filtering
		&cli.StringSliceFlag{
			Name:  "include-tags",
			Usage: "Only include flows with these tags",
		},
		&cli.StringSliceFlag{
			Name:  "exclude-tags",
			Usage: "Exclude flows with these tags",
		},

		// Output directory
		&cli.StringFlag{
			Name:  "output",
			Usage: "Output directory for reports (default: ./reports)",
		},
		&cli.BoolFlag{
			Name:  "flatten",
			Usage: "Don't create timestamp subfolder (requires --output)",
		},

		// Parallelization
		&cli.IntFlag{
			Name:  "parallel",
			Usage: "Run tests in parallel on N devices (auto-selects available devices)",
		},

		// Execution modes
		&cli.BoolFlag{
			Name:    "continuous",
			Aliases: []string{"c"},
			Usage:   "Enable continuous mode for single flow",
		},

		// Web options
		&cli.BoolFlag{
			Name:  "headed",
			Usage: "Show browser window (web only, default is headless)",
		},
		&cli.StringFlag{
			Name:    "browser",
			Usage:   "Browser to use: chrome, chromium, or path to binary (web only)",
			EnvVars: []string{"MAESTRO_BROWSER"},
		},
		&cli.StringFlag{
			Name:    "user-data-dir",
			Usage:   "Chrome user-data-dir for a persistent profile across runs (web only). Cookies / localStorage / extensions are reused.",
			EnvVars: []string{"MAESTRO_USER_DATA_DIR"},
		},

		// Driver settings
		&cli.IntFlag{
			Name:    "wait-for-idle-timeout",
			Usage:   "Wait for device idle in ms (0 = disabled, default 200)",
			Value:   200,
			EnvVars: []string{"MAESTRO_WAIT_FOR_IDLE_TIMEOUT"},
		},
		&cli.IntFlag{
			Name:    "condition-timeout",
			Usage:   "Default timeout in ms for when:/while: condition checks (default 1000). Override per condition with `timeout:`.",
			Value:   1000,
			EnvVars: []string{"MAESTRO_CONDITION_TIMEOUT"},
		},
		&cli.IntFlag{
			Name:    "typing-frequency",
			Usage:   "WDA typing speed in keys/sec (default 30). Lower values help React Native apps.",
			Value:   30,
			EnvVars: []string{"MAESTRO_TYPING_FREQUENCY"},
		},
		&cli.BoolFlag{
			Name:  "no-flutter-fallback",
			Usage: "Disable automatic Flutter VM Service fallback",
		},
		&cli.BoolFlag{
			Name:  "android-tcp-forward",
			Usage: "Force TCP-to-TCP adb forward for Android drivers (auto-enabled when $DEVICEFARM_DEVICE_UDID is set; needed on AWS Device Farm and similar sandboxes that block localfilesystem:/localabstract: forwards)",
		},
		&cli.StringFlag{
			Name:  "artifacts",
			Usage: "When to capture screenshots/hierarchy: on-failure (default), always, never",
			Value: "on-failure",
		},

		// Emulator management flags (start-emulator, auto-start-emulator,
		// shutdown-after, boot-timeout) are global flags defined in cli.go.

	},
	Action: runTest,
}

func parseArtifactMode(s string) executor.ArtifactMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "always":
		return executor.ArtifactAlways
	case "never":
		return executor.ArtifactNever
	default:
		return executor.ArtifactOnFailure
	}
}

// parseDevices parses the --device flag value into a slice of device UDIDs.
// Returns nil if no devices specified (triggers auto-detection later).
func parseDevices(deviceFlag string) []string {
	if deviceFlag != "" {
		// Split comma-separated devices
		devices := strings.Split(deviceFlag, ",")
		for i, d := range devices {
			devices[i] = strings.TrimSpace(d)
		}
		return devices
	}

	// --parallel without --device means auto-detect
	// Return nil to trigger auto-detection in executeTest
	return nil
}

// buildDeviceReport creates a report.Device from driver platform info.
func buildDeviceReport(driver core.Driver) report.Device {
	pi := driver.GetPlatformInfo()
	return report.Device{
		ID:          pi.DeviceID,
		Platform:    pi.Platform,
		Name:        pi.DeviceName,
		OSVersion:   pi.OSVersion,
		IsSimulator: pi.IsSimulator,
	}
}

// buildAppReport creates a report.App from driver platform info.
func buildAppReport(driver core.Driver) report.App {
	pi := driver.GetPlatformInfo()
	return report.App{
		ID:      pi.AppID,
		Version: pi.AppVersion,
	}
}

// resolveDriverName returns the driver name for reports based on config and platform.
func resolveDriverName(cfg *RunConfig, platform string) string {
	driverName := strings.ToLower(cfg.Driver)
	switch strings.ToLower(platform) {
	case "web":
		if driverName == "" {
			driverName = "cdp"
		}
	case "ios":
		if driverName == "" || driverName == "uiautomator2" {
			driverName = "wda"
		}
	case "mock":
		driverName = "mock"
	default: // android or empty
		if driverName == "" || driverName == "uiautomator2" {
			driverName = "uiautomator2"
		}
	}
	return driverName
}

// bootTimeout returns the configured emulator boot timeout, defaulting to 180 seconds.
func bootTimeout(cfg *RunConfig) time.Duration {
	timeout := time.Duration(cfg.BootTimeout) * time.Second
	if timeout == 0 {
		timeout = 180 * time.Second
	}
	return timeout
}

// handleEmulatorStartup starts Android emulators if requested via CLI flags.
// Handles two cases:
// 1. --start-emulator: Explicitly start a specific AVD
// 2. --auto-start-emulator: Start an emulator if no devices are found
func handleEmulatorStartup(cfg *RunConfig, mgr *emulator.Manager) error {
	// Only handle Android emulators
	if cfg.Platform != "" && cfg.Platform != "android" {
		return nil
	}

	timeout := bootTimeout(cfg)

	// Case 1: Explicit --start-emulator flag
	if cfg.StartEmulator != "" {
		fmt.Printf("  %s⏳ Starting emulator: %s%s\n", color(colorCyan), cfg.StartEmulator, color(colorReset))
		logger.Info("Starting emulator: %s (timeout: %v)", cfg.StartEmulator, timeout)

		serial, err := mgr.Start(cfg.StartEmulator, timeout)
		if err != nil {
			return fmt.Errorf("failed to start emulator %s: %w", cfg.StartEmulator, err)
		}

		fmt.Printf("  %s✓ Emulator started: %s%s\n", color(colorGreen), serial, color(colorReset))
		logger.Info("Emulator started successfully: %s", serial)

		// Add to device list if not already specified
		if len(cfg.Devices) == 0 {
			cfg.Devices = []string{serial}
			logger.Info("Added emulator to device list: %s", serial)
		}

		return nil
	}

	// Case 2: Auto-start if --auto-start-emulator and no devices
	// Skip if --parallel is set — determineExecutionMode handles parallel emulator starts
	if cfg.AutoStartEmulator && cfg.Parallel <= 0 {
		// Check if we already have devices
		devices, _ := device.ListDevices()
		if len(devices) > 0 {
			logger.Info("Devices already available, skipping auto-start")
			return nil
		}

		// No devices found - start an emulator
		logger.Info("No devices found, auto-starting emulator...")
		fmt.Printf("  %s⏳ No devices found, auto-starting emulator...%s\n", color(colorCyan), color(colorReset))

		// Find first available AVD
		avds, err := emulator.ListAVDs()
		if err != nil {
			return fmt.Errorf("failed to list AVDs: %w", err)
		}
		if len(avds) == 0 {
			return fmt.Errorf("no devices and no AVDs available; create an AVD with: avdmanager create avd")
		}

		// Start the first AVD
		avdName := avds[0].Name
		logger.Info("Starting AVD: %s", avdName)
		fmt.Printf("  %s⏳ Starting AVD: %s%s\n", color(colorCyan), avdName, color(colorReset))

		serial, err := mgr.Start(avdName, timeout)
		if err != nil {
			return fmt.Errorf("failed to auto-start emulator %s: %w", avdName, err)
		}

		fmt.Printf("  %s✓ Emulator started: %s%s\n", color(colorGreen), serial, color(colorReset))
		logger.Info("Emulator auto-started successfully: %s", serial)

		// Add to device list
		cfg.Devices = []string{serial}
		logger.Info("Added auto-started emulator to device list: %s", serial)
	}

	return nil
}

// handleDeviceStartup routes to the appropriate platform startup handler.
// Also catches mismatched flags (e.g., --start-emulator with --platform ios).
func handleDeviceStartup(cfg *RunConfig, emulatorMgr *emulator.Manager, simulatorMgr *simulator.Manager) error {
	platform := strings.ToLower(cfg.Platform)

	// Web platform doesn't need device startup
	if platform == "web" {
		return nil
	}

	// Catch mismatched flags and suggest the right one
	if platform == "ios" && cfg.StartEmulator != "" {
		return fmt.Errorf("--start-emulator is for Android, but --platform is ios\n\n" +
			"For iOS, use:\n" +
			"  --start-simulator <name>     Start an iOS simulator (e.g., \"iPhone 15 Pro\")\n" +
			"  --auto-start-emulator        Auto-start a simulator if none found\n\n" +
			"Tip: If you're coming from Maestro CLI (start-device), use:\n" +
			"  --start-simulator for iOS, --start-emulator for Android")
	}
	if (platform == "android" || platform == "") && cfg.StartSimulator != "" {
		return fmt.Errorf("--start-simulator is for iOS, but --platform is %s\n\n"+
			"For Android, use:\n"+
			"  --start-emulator <avd>       Start an Android emulator (e.g., Pixel_7_API_33)\n"+
			"  --auto-start-emulator        Auto-start an emulator if none found\n\n"+
			"Tip: If you're coming from Maestro CLI (start-device), use:\n"+
			"  --start-simulator for iOS, --start-emulator for Android", platform)
	}

	// iOS simulator startup
	if platform == "ios" || cfg.StartSimulator != "" {
		return handleSimulatorStartup(cfg, simulatorMgr)
	}

	// Android emulator startup (existing logic)
	return handleEmulatorStartup(cfg, emulatorMgr)
}

// handleSimulatorStartup starts iOS simulators if requested via CLI flags.
// Handles two cases:
// 1. --start-simulator: Explicitly start a named or UDID simulator
// 2. --auto-start-emulator with --platform ios: Start a simulator if none booted
func handleSimulatorStartup(cfg *RunConfig, mgr *simulator.Manager) error {
	timeout := bootTimeout(cfg)

	// Case 1: Explicit --start-simulator flag
	if cfg.StartSimulator != "" {
		fmt.Printf("  %s⏳ Starting simulator: %s%s\n", color(colorCyan), cfg.StartSimulator, color(colorReset))
		logger.Info("Starting simulator: %s (timeout: %v)", cfg.StartSimulator, timeout)

		udid, err := mgr.StartByName(cfg.StartSimulator, timeout)
		if err != nil {
			return fmt.Errorf("failed to start simulator %s: %w", cfg.StartSimulator, err)
		}

		fmt.Printf("  %s✓ Simulator started: %s%s\n", color(colorGreen), udid, color(colorReset))
		logger.Info("Simulator started successfully: %s", udid)

		if len(cfg.Devices) == 0 {
			cfg.Devices = []string{udid}
			logger.Info("Added simulator to device list: %s", udid)
		}

		return nil
	}

	// Case 2: Auto-start if --auto-start-emulator and platform is iOS
	// Skip if --parallel is set — determineExecutionMode handles parallel starts
	if cfg.AutoStartEmulator && cfg.Parallel <= 0 {
		// Check if we already have a booted simulator
		udid, _ := findBootedSimulator()
		if udid != "" {
			logger.Info("Simulator already booted, skipping auto-start")
			return nil
		}

		// No booted simulators — find an iOS one to start
		logger.Info("No booted iOS simulators found, auto-starting...")
		fmt.Printf("  %s⏳ No iOS simulators found, auto-starting...%s\n", color(colorCyan), color(colorReset))

		shutdownSims, err := simulator.ListShutdownIOSSimulators()
		if err == nil && len(shutdownSims) > 0 {
			// Boot an existing shutdown simulator
			target := shutdownSims[0]
			logger.Info("Starting simulator: %s (%s)", target.Name, target.UDID)
			fmt.Printf("  %s⏳ Starting simulator: %s%s\n", color(colorCyan), target.Name, color(colorReset))

			udid, err = mgr.Start(target.UDID, timeout)
			if err != nil {
				return fmt.Errorf("failed to auto-start simulator %s: %w", target.Name, err)
			}
		} else {
			// No shutdown simulators available — create a new one
			logger.Info("No shutdown iOS simulators, creating a new one...")

			iosRuntime, err := simulator.LatestIOSRuntime()
			if err != nil {
				return fmt.Errorf("no iOS simulators available and cannot create one: %w", err)
			}

			deviceType := iosRuntime.DeviceTypes[0]
			simName := "maestro-runner-1"
			fmt.Printf("  %s⏳ Creating simulator: %s (iOS %s)%s\n", color(colorCyan), simName, iosRuntime.Version, color(colorReset))

			udid, err = mgr.CreateAndStart(simName, deviceType, iosRuntime.Identifier, timeout)
			if err != nil {
				return fmt.Errorf("failed to create simulator: %w", err)
			}
		}

		fmt.Printf("  %s✓ Simulator started: %s%s\n", color(colorGreen), udid, color(colorReset))
		logger.Info("Simulator auto-started successfully: %s", udid)

		cfg.Devices = []string{udid}
		logger.Info("Added auto-started simulator to device list: %s", udid)
	}

	return nil
}

// RunConfig holds the complete test run configuration.
type RunConfig struct {
	// Paths
	FlowPaths  []string
	ConfigPath string

	// Environment
	Env map[string]string

	// Filtering
	IncludeTags []string
	ExcludeTags []string

	// Output
	OutputDir string // Final resolved output directory

	// Parallelization
	Parallel int // Number of devices to use (0 = single device mode)

	// Execution
	Continuous  bool
	Headed      bool   // Show browser window (web only, default is headless)
	Browser     string // chrome, chromium, or path to binary (web only)
	UserDataDir string // Persistent Chrome profile directory (web only)

	// Device
	Platform string
	Devices  []string // Device UDIDs (can be comma-separated or multiple from --parallel)
	Verbose  bool
	AppFile  string // App binary to install before testing
	AppID    string // App bundle ID or package name

	// Driver
	Driver    string // uiautomator2, appium
	AppiumURL string // Appium server URL
	// AppiumSessionFile, when set, makes maestro-runner publish live Appium
	// session info (sessionId + appiumUrl per device) to this JSON file via the
	// session-export provider, so external tools can attach without polling
	// reports. Runs alongside any detected cloud provider. See issue #91.
	AppiumSessionFile string
	CapsFile          string                 // Appium capabilities JSON file path
	Capabilities      map[string]interface{} // Parsed Appium capabilities

	// Driver settings
	WaitForIdleTimeout int    // Wait for device idle in ms (0 = disabled, default 200)
	ConditionTimeout   int    // Default timeout (ms) for when:/while: condition checks (default 1000)
	TypingFrequency    int    // WDA typing frequency in keys/sec (0 = use WDA default of 60)
	TeamID             string // Apple Development Team ID for WDA code signing
	WDABundleID        string // Custom WDA bundle identifier

	// Emulator/Simulator management
	StartEmulator     string // AVD name to start (e.g., Pixel_7_API_33)
	StartSimulator    string // iOS simulator name/UDID to start (e.g., "iPhone 15 Pro")
	AutoStartEmulator bool   // Auto-start an emulator/simulator if no devices found
	ShutdownAfter     bool   // Shutdown emulators/simulators started by maestro-runner after tests
	BootTimeout       int    // Device boot timeout in seconds

	// Driver start
	DriverStartTimeout int // Override driver start timeout in seconds (0 = use driver default — 30s Android, 90s iOS WDA)

	// Install control
	NoAppInstall    bool // Skip app installation
	NoDriverInstall bool // Skip driver installation

	// Flutter
	NoFlutterFallback bool // Disable automatic Flutter VM Service fallback

	// Android driver — TCP forward override
	// Forces TCP-to-TCP adb forward instead of the Unix-socket forward
	// we use on Linux/Mac. Auto-enabled when $DEVICEFARM_DEVICE_UDID is
	// set (AWS Device Farm). Use the flag to opt in manually for other
	// sandboxed environments that block localfilesystem:/localabstract:
	// forwards.
	AndroidTCPForward bool

	// Artifacts
	Artifacts executor.ArtifactMode // When to capture screenshots/hierarchy

	// Cloud provider (detected from AppiumURL, nil if not a cloud provider)
	CloudProvider cloud.Provider
	CloudMeta     map[string]string
}

func hyperlink(url, text string) string {
	if colorsEnabled {
		return "\x1b]8;;" + url + "\x07" + text + "\x1b]8;;\x07"
	}
	return text
}

func printBanner() {
	deviceLabLink := hyperlink("https://devicelab.dev", color(colorCyan)+"DeviceLab.dev"+color(colorReset))
	githubLink := hyperlink("https://github.com/devicelab-dev/maestro-runner", "Star us on GitHub")

	fmt.Println()
	fmt.Printf("  maestro-runner %s - by %s\n", Version, deviceLabLink)
	fmt.Println("  Fast, lightweight Maestro test runner")
	fmt.Printf("  %s\n", githubLink)
	fmt.Println()
}

func printFooter() {
	deviceLabLink := hyperlink("https://devicelab.dev", color(colorCyan)+"DeviceLab.dev"+color(colorReset))

	fmt.Println()
	fmt.Printf("  Built by %s - Turn Your Devices Into a Distributed Device Lab\n", deviceLabLink)
	fmt.Println()
}

func runTest(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("at least one flow file or folder is required")
	}

	// Print banner at start
	printBanner()

	// Check for updates in background (prints at end)
	startUpdateCheck()

	// Helper to get flag value from current or parent context
	// When run as subcommand, global flags are in parent context
	getString := func(name string) string {
		if c.IsSet(name) {
			return c.String(name)
		}
		if c.Lineage()[1] != nil {
			return c.Lineage()[1].String(name)
		}
		return c.String(name)
	}
	getInt := func(name string) int {
		if c.IsSet(name) {
			return c.Int(name)
		}
		if c.Lineage()[1] != nil {
			return c.Lineage()[1].Int(name)
		}
		return c.Int(name)
	}
	getBool := func(name string) bool {
		if c.IsSet(name) {
			return c.Bool(name)
		}
		if c.Lineage()[1] != nil {
			return c.Lineage()[1].Bool(name)
		}
		return c.Bool(name)
	}
	getStringSlice := func(name string) []string {
		if c.IsSet(name) {
			return c.StringSlice(name)
		}
		if c.Lineage()[1] != nil {
			return c.Lineage()[1].StringSlice(name)
		}
		return c.StringSlice(name)
	}

	// Parse environment variables
	env := parseEnvVars(getStringSlice("env"))

	// Resolve output directory
	outputDir, err := resolveOutputDir(getString("output"), getBool("flatten"))
	if err != nil {
		return err
	}

	// Load Appium capabilities if provided
	capsFile := getString("caps")
	var caps map[string]interface{}
	if capsFile != "" {
		var err error
		caps, err = loadCapabilities(capsFile)
		if err != nil {
			return err
		}
	}

	// Load workspace config if provided
	var workspaceConfig *config.Config
	configPath := getString("config")
	if configPath != "" {
		var err error
		workspaceConfig, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Load --env-file if specified. Values from the file slot between the
	// workspace config (lowest precedence) and -e CLI flags (highest).
	var envFileVars map[string]string
	if envFilePath := getString("env-file"); envFilePath != "" {
		var err error
		envFileVars, err = ParseEnvFile(envFilePath)
		if err != nil {
			return fmt.Errorf("env-file: %w", err)
		}
	}

	// Merge env variables. Precedence (lowest → highest):
	//   workspace config Env  <  --env-file values  <  -e CLI flag values
	mergedEnv := make(map[string]string)
	if workspaceConfig != nil {
		for k, v := range workspaceConfig.Env {
			mergedEnv[k] = v
		}
	}
	for k, v := range envFileVars {
		mergedEnv[k] = v // --env-file overrides workspace config
	}
	for k, v := range env {
		mergedEnv[k] = v // -e CLI overrides --env-file and workspace config
	}

	// Get appId from workspace config or will be extracted from flows later
	appID := ""
	if workspaceConfig != nil && workspaceConfig.AppID != "" {
		appID = workspaceConfig.AppID
	}

	// Build run configuration
	cfg := &RunConfig{
		FlowPaths:          c.Args().Slice(),
		ConfigPath:         configPath,
		Env:                mergedEnv,
		IncludeTags:        getStringSlice("include-tags"),
		ExcludeTags:        getStringSlice("exclude-tags"),
		OutputDir:          outputDir,
		Parallel:           getInt("parallel"),
		Continuous:         getBool("continuous"),
		Headed:             getBool("headed"),
		Browser:            getString("browser"),
		UserDataDir:        getString("user-data-dir"),
		Platform:           getString("platform"),
		Devices:            parseDevices(getString("device")),
		Verbose:            getBool("verbose"),
		AppFile:            getString("app-file"),
		AppID:              appID,
		Driver:             getString("driver"),
		AppiumURL:          getString("appium-url"),
		AppiumSessionFile:  getString("appium-session-file"),
		CapsFile:           capsFile,
		Capabilities:       caps,
		WaitForIdleTimeout: getInt("wait-for-idle-timeout"),
		ConditionTimeout:   getInt("condition-timeout"),
		TypingFrequency:    getInt("typing-frequency"),
		TeamID:             getString("team-id"),
		WDABundleID:        getString("wda-bundle-id"),
		StartEmulator:      getString("start-emulator"),
		StartSimulator:     getString("start-simulator"),
		AutoStartEmulator:  getBool("auto-start-emulator"),
		ShutdownAfter:      getBool("shutdown-after"),
		BootTimeout:        getInt("boot-timeout"),
		DriverStartTimeout: getInt("driver-start-timeout"),
		NoAppInstall:       getBool("no-app-install"),
		NoDriverInstall:    getBool("no-driver-install"),
		NoFlutterFallback:  getBool("no-flutter-fallback"),
		AndroidTCPForward:  getBool("android-tcp-forward"),
		Artifacts:          parseArtifactMode(getString("artifacts")),
	}

	// Apply waitForIdleTimeout with priority:
	// Flow config > CLI flag > Workspace config > Cap file > Default (5000ms)
	// (Flow config is handled in flow_runner.go)
	if !c.IsSet("wait-for-idle-timeout") {
		// CLI not explicitly set, check other sources
		if workspaceConfig != nil && workspaceConfig.WaitForIdleTimeout != 0 {
			// Use workspace config
			cfg.WaitForIdleTimeout = workspaceConfig.WaitForIdleTimeout
		} else if caps != nil {
			// Check caps file for waitForIdleTimeout
			if val, ok := caps["appium:waitForIdleTimeout"].(float64); ok {
				cfg.WaitForIdleTimeout = int(val)
			} else if val, ok := caps["waitForIdleTimeout"].(float64); ok {
				cfg.WaitForIdleTimeout = int(val)
			}
			// else: keep default 200ms from CLI flag
		}
	}

	return executeTest(cfg)
}

// resolveOutputDir determines the output directory based on flags.
// - No --output: ./reports/<timestamp>/
// - --output given: <output>/<timestamp>/
// - --output + --flatten: <output>/ (error if --output not given)
func resolveOutputDir(output string, flatten bool) (string, error) {
	if flatten && output == "" {
		return "", fmt.Errorf("--flatten requires --output to be specified")
	}

	baseDir := output
	if baseDir == "" {
		baseDir = "./reports"
	}

	if flatten {
		return filepath.Clean(baseDir), nil
	}

	// Create timestamp-based subfolder
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	return filepath.Join(baseDir, timestamp), nil
}

func executeTest(cfg *RunConfig) error {
	// 1. Create output directory
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// 2. Initialize logging
	logPath := filepath.Join(cfg.OutputDir, "maestro-runner.log")
	if err := logger.Init(logPath); err != nil {
		fmt.Printf("Warning: Failed to initialize logger: %v\n", err)
	}
	defer logger.Close()

	logger.Info("=== Test execution started ===")
	logger.Info("Output directory: %s", cfg.OutputDir)
	logger.Info("Platform: %s", cfg.Platform)
	logger.Info("Driver: %s", cfg.Driver)

	// 2.5. Initialize device lifecycle managers
	emulatorMgr := emulator.NewManager()
	simulatorMgr := simulator.NewManager()

	// Ensure emulators/simulators are cleaned up on normal exit
	defer func() {
		if cfg.ShutdownAfter {
			logger.Info("Shutting down devices started by maestro-runner...")
			if err := emulatorMgr.ShutdownAll(); err != nil {
				logger.Error("Failed to shutdown emulators: %v", err)
			}
			if err := simulatorMgr.ShutdownAll(); err != nil {
				logger.Error("Failed to shutdown simulators: %v", err)
			}
		}
	}()

	// Handle SIGINT/SIGTERM to clean up on Ctrl+C or kill
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("Received signal %v, cleaning up...", sig)
		fmt.Fprintf(os.Stderr, "\nReceived %v, cleaning up...\n", sig)

		// Clean up driver resources (socket, adb forward, session)
		cleanupMu.Lock()
		fn := activeCleanup
		cleanupMu.Unlock()
		if fn != nil {
			fn()
		}

		if cfg.ShutdownAfter {
			if err := emulatorMgr.ShutdownAll(); err != nil {
				logger.Error("Failed to shutdown emulators on signal: %v", err)
			}
			if err := simulatorMgr.ShutdownAll(); err != nil {
				logger.Error("Failed to shutdown simulators on signal: %v", err)
			}
		}
		os.Exit(1)
	}()
	defer signal.Stop(sigCh)

	// 3. Validate and parse flows
	flows, err := validateAndParseFlows(cfg)
	if err != nil {
		logger.Error("Flow validation failed: %v", err)
		return err
	}
	logger.Info("Validated %d flow(s)", len(flows))

	// Warn about unsupported selector fields for the target platform
	if cfg.Platform != "" {
		warnUnsupportedSelectors(flows, cfg.Platform)
	}

	// Pre-checks for iOS with direct WDA driver (not Appium).
	// Appium handles everything via capabilities — no --app-file or --team-id needed.
	if strings.EqualFold(cfg.Platform, "ios") && cfg.Driver != "appium" {
		// team-id is only required for real devices, not simulators.
		isSimTarget := iosTargetsSimulator(cfg)
		if cfg.TeamID == "" && !isSimTarget {
			return fmt.Errorf("iOS with WDA driver requires --team-id for code signing (real devices only)\n" +
				"Usage: maestro-runner --platform ios --team-id <APPLE_TEAM_ID> test <flow-files>\n" +
				"Note: --team-id is not required for simulators")
		}
		// clearState on iOS uninstalls + reinstalls. On real devices the host
		// can't reach the installed .app bundle, so --app-file is mandatory.
		// On simulators the runner auto-discovers the installed bundle via
		// `simctl get_app_container` and stages a copy before uninstall, so
		// --app-file is optional (matches Maestro CLI's behavior).
		if cfg.AppFile == "" && !isSimTarget && flowsUseClearState(flows) {
			return fmt.Errorf("clearState on real iOS devices requires --app-file — " +
				"the installed bundle isn't reachable from the host. On simulators no flag is needed.\n" +
				"Usage: maestro-runner --app-file <path-to-ipa-or-app> --platform ios test <flow-files>")
		}

		// Advisory only: surface Flutter debug-build pitfall before the run
		// starts. Standalone debug builds crash and produce a port-forward
		// flood — easy to misread as a maestro-runner bug.
		warnIfFlutterDebugBuild(cfg.AppFile)
	}

	// Extract appId/url from first flow if not in config
	if cfg.AppID == "" && len(flows) > 0 {
		cfg.AppID = flows[0].Config.EffectiveAppID()
	}

	// 3.5. Handle device startup (emulator or simulator, if requested)
	if err := handleDeviceStartup(cfg, emulatorMgr, simulatorMgr); err != nil {
		logger.Error("Device startup failed: %v", err)
		return err
	}

	// 4. Determine execution mode and devices
	needsParallel, deviceIDs, err := determineExecutionMode(cfg, emulatorMgr, simulatorMgr)
	if err != nil {
		logger.Error("Failed to determine execution mode: %v", err)
		return err
	}

	if needsParallel {
		logger.Info("Parallel execution mode: %d devices: %v", len(deviceIDs), deviceIDs)
	} else {
		logger.Info("Single device execution mode")
	}

	// 5. Execute flows
	logger.Info("Starting flow execution (parallel: %v, devices: %v)", needsParallel, deviceIDs)
	result, err := executeFlowsWithMode(cfg, flows, needsParallel, deviceIDs)
	if err != nil {
		logger.Error("Flow execution failed: %v", err)
		return err
	}
	logger.Info("Flow execution completed: %d passed, %d failed, %d skipped",
		result.PassedFlows, result.FailedFlows, result.SkippedFlows)

	// 6. Print unified output (works for both single and parallel)
	if err := printUnifiedOutput(cfg.OutputDir, result); err != nil {
		fmt.Printf("Warning: Failed to print unified output: %v\n", err)
		// Fallback to basic summary
		printSummary(result)
	}

	// 7. Generate and display reports
	logger.Info("Generating reports...")
	fmt.Println()
	fmt.Printf("  %s✓ Tests completed. Generating reports...%s\n", color(colorGreen), color(colorReset))
	fmt.Println()

	htmlPath := filepath.Join(cfg.OutputDir, "report.html")
	jsonPath := filepath.Join(cfg.OutputDir, "report.json")
	junitPath := filepath.Join(cfg.OutputDir, "junit-report.xml")

	htmlGenerated := true
	if err := report.GenerateHTML(cfg.OutputDir, report.HTMLConfig{
		OutputPath: htmlPath,
		Title:      "Test Report",
	}); err != nil {
		htmlGenerated = false
		fmt.Printf("  %s⚠%s Warning: failed to generate HTML report: %v\n", color(colorYellow), color(colorReset), err)
	}

	junitGenerated := true
	if err := report.GenerateJUnit(cfg.OutputDir); err != nil {
		junitGenerated = false
		fmt.Printf("  %s⚠%s Warning: failed to generate JUnit report: %v\n", color(colorYellow), color(colorReset), err)
	}

	allurePath := filepath.Join(cfg.OutputDir, "allure-results")
	allureGenerated := true
	if err := report.GenerateAllure(cfg.OutputDir); err != nil {
		allureGenerated = false
		fmt.Printf("  %s⚠%s Warning: failed to generate Allure report: %v\n", color(colorYellow), color(colorReset), err)
	}

	// Report result to cloud provider (if detected)
	if cfg.CloudProvider != nil {
		cloudResult := &cloud.TestResult{
			Passed:      result.Status == report.StatusPassed,
			Total:       result.TotalFlows,
			PassedCount: result.PassedFlows,
			FailedCount: result.FailedFlows,
			Duration:    result.Duration,
			OutputDir:   cfg.OutputDir,
		}
		for _, f := range result.FlowResults {
			cloudResult.Flows = append(cloudResult.Flows, cloud.FlowResult{
				Name:     f.Name,
				File:     f.SourceFile,
				Passed:   f.Status == report.StatusPassed,
				Duration: f.Duration,
				Error:    f.Error,
			})
		}
		if err := cfg.CloudProvider.ReportResult(cfg.AppiumURL, cfg.CloudMeta, cloudResult); err != nil {
			logger.Warn("%s result reporting failed: %v", cfg.CloudProvider.Name(), err)
		} else {
			logger.Info("%s job updated: passed=%v", cfg.CloudProvider.Name(), cloudResult.Passed)
		}
	}

	// Display reports section as a directory tree
	fmt.Printf("  %sReports:%s %s\n", color(colorBold), color(colorReset), cfg.OutputDir)
	fmt.Printf("    ├── report.json\n")
	if htmlGenerated {
		fmt.Printf("    ├── report.html\n")
	}
	if junitGenerated {
		fmt.Printf("    ├── junit-report.xml\n")
	}
	if allureGenerated {
		fmt.Printf("    └── allure-results/\n")
	}
	fmt.Println()
	fmt.Println("  Paths:")
	if htmlGenerated {
		fmt.Printf("    HTML:   %s\n", htmlPath)
	}
	fmt.Printf("    JSON:   %s\n", jsonPath)
	if junitGenerated {
		fmt.Printf("    JUnit:  %s\n", junitPath)
	}
	if allureGenerated {
		fmt.Printf("    Allure: %s\n", allurePath)
	}

	// 7. Print update notice if available
	printUpdateNotice()

	// 8. Print footer
	printFooter()

	// Exit with code 1 if any flows failed (summary already printed)
	if result.Status != report.StatusPassed {
		return cli.Exit("", 1)
	}

	return nil
}

// validateAndParseFlows validates and parses all flow files.
func validateAndParseFlows(cfg *RunConfig) ([]flow.Flow, error) {
	v := validator.New(cfg.IncludeTags, cfg.ExcludeTags)
	var allTestCases []string
	var allErrors []error

	for _, path := range cfg.FlowPaths {
		result := v.Validate(path)
		allTestCases = append(allTestCases, result.TestCases...)
		allErrors = append(allErrors, result.Errors...)
	}

	if len(allErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Validation errors:\n")
		for _, err := range allErrors {
			fmt.Fprintf(os.Stderr, "  - %v\n", err)
		}
		return nil, fmt.Errorf("validation failed with %d error(s)", len(allErrors))
	}

	if len(allTestCases) == 0 {
		return nil, fmt.Errorf("no test flows found")
	}

	fmt.Printf("\n%sSetup%s\n", color(colorBold), color(colorReset))
	fmt.Println(strings.Repeat("─", 40))
	printSetupSuccess(fmt.Sprintf("Found %d test flow(s)", len(allTestCases)))

	var flows []flow.Flow
	for _, path := range allTestCases {
		f, err := flow.ParseFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", path, err)
		}
		flows = append(flows, *f)
	}

	return flows, nil
}

// warnUnsupportedSelectors walks all steps in all flows, checks each selector
// against the target platform's supported set, and prints warnings for any
// unsupported fields. Deduplicates so each field is warned about only once.
func warnUnsupportedSelectors(flows []flow.Flow, platform string) {
	warned := make(map[string]bool)
	var warnings []string

	for _, f := range flows {
		for _, step := range f.Steps {
			selectors := flow.ExtractSelectors(step)
			for _, sel := range selectors {
				unsupported := flow.CheckUnsupportedFields(sel, platform)
				for _, field := range unsupported {
					key := field + "+" + platform
					if !warned[key] {
						warned[key] = true
						warnings = append(warnings,
							fmt.Sprintf("  %s\u26a0%s  %q is not supported on %s — will be ignored (%s)",
								color(colorYellow), color(colorReset), field, platform, f.SourcePath))
					}
				}
			}
		}
	}

	if len(warnings) > 0 {
		fmt.Println()
		for _, w := range warnings {
			fmt.Println(w)
		}
		fmt.Println()
	}
}

// flowsUseClearState checks if any flow uses clearState (standalone or via launchApp).
func flowsUseClearState(flows []flow.Flow) bool {
	for _, f := range flows {
		for _, step := range f.Steps {
			switch s := step.(type) {
			case *flow.ClearStateStep:
				return true
			case *flow.LaunchAppStep:
				if s.ClearState {
					return true
				}
			}
		}
		// Also check onFlowStart hooks
		for _, step := range f.Config.OnFlowStart {
			switch s := step.(type) {
			case *flow.ClearStateStep:
				return true
			case *flow.LaunchAppStep:
				if s.ClearState {
					return true
				}
			}
		}
	}
	return false
}

// determineExecutionMode decides whether to run in parallel and which devices to use.
// If --parallel N is specified with --auto-start-emulator, this will start additional
// emulators to reach N total devices.
func determineExecutionMode(cfg *RunConfig, emulatorMgr *emulator.Manager, simulatorMgr *simulator.Manager) (needsParallel bool, deviceIDs []string, err error) {
	// Web platform: no device management needed, but support --parallel N
	if strings.ToLower(cfg.Platform) == "web" {
		if cfg.Parallel > 0 {
			ids := make([]string, cfg.Parallel)
			for i := range ids {
				ids[i] = fmt.Sprintf("browser-%d", i+1)
			}
			return true, ids, nil
		}
		return false, nil, nil
	}

	// Appium driver: no local device management, server allocates devices
	if strings.ToLower(cfg.Driver) == "appium" {
		if cfg.Parallel > 1 {
			ids := make([]string, cfg.Parallel)
			for i := range ids {
				ids[i] = fmt.Sprintf("appium-%d", i+1)
			}
			return true, ids, nil
		}
		return false, nil, nil
	}

	needsParallel = cfg.Parallel > 0 || len(cfg.Devices) > 1

	if needsParallel {
		if len(cfg.Devices) > 0 {
			deviceIDs = cfg.Devices
		} else if cfg.Parallel > 0 {
			// Try to auto-detect existing available devices (not in use)
			existingDevices, detectErr := autoDetectDevicesFn(cfg.Platform, cfg.Parallel)
			if detectErr != nil && !cfg.AutoStartEmulator {
				// No devices and auto-start disabled - show helpful error
				return false, nil, buildParallelDeviceError(cfg, 0)
			}

			// Start with existing devices (may be fewer than requested)
			if len(existingDevices) > 0 {
				deviceIDs = existingDevices
				logger.Info("Found %d available booted device(s)", len(deviceIDs))
			}

			// Check if we need more devices
			needed := cfg.Parallel - len(deviceIDs)
			if needed > 0 && cfg.AutoStartEmulator && (cfg.Platform == "" || cfg.Platform == "android") {
				logger.Info("Need %d more devices for --parallel %d, starting emulators...", needed, cfg.Parallel)

				// List available AVDs and filter out those already running
				avds, err := emulator.ListAVDs()
				if err != nil {
					return false, nil, fmt.Errorf("failed to list AVDs: %w", err)
				}
				// Filter out AVDs whose emulators are already running —
				// same AVD cannot run twice (lock conflict).
				runningNames := emulator.RunningAVDNames()
				var stoppedAVDs []emulator.AVDInfo
				for _, avd := range avds {
					if !runningNames[avd.Name] {
						stoppedAVDs = append(stoppedAVDs, avd)
					}
				}
				avds = stoppedAVDs
				if len(avds) == 0 {
					return false, nil, fmt.Errorf("need %d more device(s) but no stopped AVDs available\n"+
						"All %d AVD(s) are already running (%d available, %d in use).\n"+
						"Hint: create more AVDs with: avdmanager create avd",
						needed, len(runningNames), len(deviceIDs), len(runningNames)-len(deviceIDs))
				}
				if len(avds) < needed {
					fmt.Println()
					return false, nil, buildNotEnoughAVDsError(cfg, len(deviceIDs), avds)
				}

				// Now we know we have enough AVDs -- print progress
				fmt.Printf("  %s⏳ Starting %d emulator(s) for parallel execution...%s\n", color(colorCyan), needed, color(colorReset))

				// Start emulators sequentially to avoid port conflicts.
				timeout := bootTimeout(cfg)

				for i := 0; i < needed; i++ {
					avdName := avds[i].Name
					logger.Info("Starting emulator %d/%d: %s", i+1, needed, avdName)
					fmt.Printf("  %s⏳ Starting emulator %d/%d: %s%s\n", color(colorCyan), i+1, needed, avdName, color(colorReset))

					serial, err := emulatorMgr.Start(avdName, timeout)
					if err != nil {
						// Clean up already-started emulators before returning
						logger.Error("Emulator %s failed, cleaning up %d already-started emulator(s)", avdName, i)
						if shutdownErr := emulatorMgr.ShutdownAll(); shutdownErr != nil {
							logger.Error("Failed to cleanup emulators after partial start: %v", shutdownErr)
						}
						return false, nil, fmt.Errorf("failed to start emulator %s: %w", avdName, err)
					}

					deviceIDs = append(deviceIDs, serial)
					logger.Info("Emulator started: %s (%d/%d)", serial, i+1, needed)
					fmt.Printf("  %s✓ Emulator started: %s%s\n", color(colorGreen), serial, color(colorReset))
				}
			} else if needed > 0 && cfg.AutoStartEmulator && cfg.Platform == "ios" {
				// iOS simulator parallel startup
				logger.Info("Need %d more iOS simulators for --parallel %d", needed, cfg.Parallel)
				timeout := bootTimeout(cfg)

				// Phase 1: Boot existing shutdown iOS simulators
				shutdownSims, err := simulator.ListShutdownIOSSimulators()
				if err != nil {
					return false, nil, fmt.Errorf("failed to list simulators: %w", err)
				}

				bootable := len(shutdownSims)
				if bootable > needed {
					bootable = needed
				}

				if bootable > 0 {
					fmt.Printf("  %s⏳ Starting %d existing simulator(s)...%s\n", color(colorCyan), bootable, color(colorReset))
				}

				for i := 0; i < bootable; i++ {
					sim := shutdownSims[i]
					logger.Info("Starting simulator %d/%d: %s (%s)", i+1, bootable, sim.Name, sim.UDID)
					fmt.Printf("  %s⏳ Starting simulator %d/%d: %s%s\n", color(colorCyan), i+1, bootable, sim.Name, color(colorReset))

					udid, err := simulatorMgr.Start(sim.UDID, timeout)
					if err != nil {
						logger.Error("Simulator %s failed, cleaning up", sim.Name)
						if shutdownErr := simulatorMgr.ShutdownAll(); shutdownErr != nil {
							logger.Error("Failed to cleanup simulators after partial start: %v", shutdownErr)
						}
						return false, nil, fmt.Errorf("failed to start simulator %s: %w", sim.Name, err)
					}

					deviceIDs = append(deviceIDs, udid)
					logger.Info("Simulator started: %s (%d/%d)", sim.Name, i+1, bootable)
					fmt.Printf("  %s✓ Simulator started: %s (%s)%s\n", color(colorGreen), sim.Name, udid, color(colorReset))
				}

				// Phase 2: Create new simulators if still not enough
				stillNeeded := cfg.Parallel - len(deviceIDs)
				if stillNeeded > 0 {
					logger.Info("Still need %d more simulator(s), creating new ones", stillNeeded)

					iosRuntime, err := simulator.LatestIOSRuntime()
					if err != nil {
						if shutdownErr := simulatorMgr.ShutdownAll(); shutdownErr != nil {
							logger.Error("Failed to cleanup simulators: %v", shutdownErr)
						}
						return false, nil, fmt.Errorf("cannot create simulators: %w", err)
					}

					fmt.Printf("  %s⏳ Creating %d new simulator(s) (iOS %s)...%s\n", color(colorCyan), stillNeeded, iosRuntime.Version, color(colorReset))

					for i := 0; i < stillNeeded; i++ {
						// Pick a device type (cycle through available iPhones)
						deviceType := iosRuntime.DeviceTypes[i%len(iosRuntime.DeviceTypes)]
						simName := fmt.Sprintf("maestro-runner-%d", i+1)

						logger.Info("Creating simulator %d/%d: %s (%s)", i+1, stillNeeded, simName, deviceType)
						fmt.Printf("  %s⏳ Creating simulator %d/%d: %s%s\n", color(colorCyan), i+1, stillNeeded, simName, color(colorReset))

						udid, err := simulatorMgr.CreateAndStart(simName, deviceType, iosRuntime.Identifier, timeout)
						if err != nil {
							logger.Error("Failed to create simulator %d/%d, cleaning up", i+1, stillNeeded)
							if shutdownErr := simulatorMgr.ShutdownAll(); shutdownErr != nil {
								logger.Error("Failed to cleanup simulators: %v", shutdownErr)
							}
							return false, nil, fmt.Errorf("failed to create simulator: %w", err)
						}

						deviceIDs = append(deviceIDs, udid)
						logger.Info("Simulator created and started: %s (%d/%d)", udid, i+1, stillNeeded)
						fmt.Printf("  %s✓ Simulator created: %s (%s)%s\n", color(colorGreen), simName, udid, color(colorReset))
					}
				}
			} else if needed > 0 {
				// Need more devices but auto-start is disabled - build helpful error
				return false, nil, buildParallelDeviceError(cfg, len(deviceIDs))
			}
		}
		printSetupSuccess(fmt.Sprintf("Using %d device(s) for parallel execution", len(deviceIDs)))
		fmt.Println()
		fmt.Printf("  %sℹ Parallel Mode:%s\n", color(colorCyan), color(colorReset))
		fmt.Println("    During execution, only brief status updates will be shown to avoid")
		fmt.Println("    messy interleaved output. Detailed results will be displayed after")
		fmt.Println("    all tests complete.")
		fmt.Println()
	}

	printSetupSuccess(fmt.Sprintf("Report directory: %s", cfg.OutputDir))
	fmt.Printf("\n%sExecution%s\n", color(colorBold), color(colorReset))
	fmt.Println(strings.Repeat("─", 40))

	return needsParallel, deviceIDs, nil
}

// executeFlowsWithMode executes flows using the appropriate execution mode.
func executeFlowsWithMode(cfg *RunConfig, flows []flow.Flow, needsParallel bool, deviceIDs []string) (*executor.RunResult, error) {
	driverType := strings.ToLower(cfg.Driver)

	if driverType == "appium" {
		count := cfg.Parallel
		if count <= 1 {
			return executeAppiumSingleSession(cfg, flows)
		}
		// Don't create more sessions than flows
		if count > len(flows) {
			fmt.Printf("  %s⚠%s --parallel %d but only %d flow(s), using %d session(s)\n",
				color(colorYellow), color(colorReset), count, len(flows), len(flows))
			count = len(flows)
		}
		return executeAppiumParallel(cfg, count, flows)
	}

	if needsParallel {
		return executeParallel(cfg, deviceIDs, flows)
	}

	return executeSingleDevice(cfg, flows)
}

// executeSingleDevice runs flows on a single device.
func executeSingleDevice(cfg *RunConfig, flows []flow.Flow) (*executor.RunResult, error) {
	logger.Info("Creating driver for single device execution")
	driver, cleanup, err := CreateDriver(cfg)
	if err != nil {
		logger.Error("Failed to create driver: %v", err)
		// Surface NoDevicesError directly so the helpful message isn't buried
		var noDevErr *device.NoDevicesError
		if errors.As(err, &noDevErr) {
			return nil, noDevErr
		}
		return nil, fmt.Errorf("failed to create driver: %w", err)
	}
	cleanupMu.Lock()
	activeCleanup = cleanup
	cleanupMu.Unlock()
	defer func() {
		cleanup()
		cleanupMu.Lock()
		activeCleanup = nil
		cleanupMu.Unlock()
	}()

	logger.Info("Driver created: %s on %s", driver.GetPlatformInfo().Platform, driver.GetPlatformInfo().DeviceName)

	driverName := resolveDriverName(cfg, cfg.Platform)
	deviceInfo := buildDeviceReport(driver)

	runner := executor.New(driver, executor.RunnerConfig{
		OutputDir:          cfg.OutputDir,
		Parallelism:        0,
		Artifacts:          cfg.Artifacts,
		Device:             deviceInfo,
		App:                buildAppReport(driver),
		RunnerVersion:      Version,
		DriverName:         driverName,
		Env:                cfg.Env,
		WaitForIdleTimeout: cfg.WaitForIdleTimeout,
		ConditionTimeout:   cfg.ConditionTimeout,
		TypingFrequency:    cfg.TypingFrequency,
		DeviceInfo:         &deviceInfo,
		OnFlowStart:        onFlowStartWithCloud(cfg),
		OnStepComplete:     onStepComplete,
		OnNestedStep:       onNestedStep,
		OnNestedFlowStart:  onNestedFlowStart,
		OnFlowEnd:          onFlowEndWithCloud(cfg),
	})

	notifyCloudRunStart(cfg, len(flows))
	return runner.Run(context.Background(), flows)
}

// ExecuteFlowWithDriver runs a single flow using an existing driver.
// Use this for library usage when you want to reuse the same session across multiple flows.
// Provides full output: console progress, JSON report, and screenshots on failure.
//
// Example usage:
//
//	driver, cleanup, _ := cli.CreateDriver(cfg)
//	defer cleanup()
//
//	for flowPath := range getFlows() {
//	    f, _ := flow.ParseFile(flowPath)
//	    cfg.OutputDir = "./reports/" + f.Name
//	    result, _ := cli.ExecuteFlowWithDriver(driver, cfg, *f)
//	}
func ExecuteFlowWithDriver(driver core.Driver, cfg *RunConfig, f flow.Flow) (*executor.RunResult, error) {
	driverName := resolveDriverName(cfg, cfg.Platform)
	deviceInfo := buildDeviceReport(driver)

	runner := executor.New(driver, executor.RunnerConfig{
		OutputDir:          cfg.OutputDir,
		Parallelism:        0,
		Artifacts:          cfg.Artifacts,
		Device:             deviceInfo,
		App:                buildAppReport(driver),
		RunnerVersion:      Version,
		DriverName:         driverName,
		Env:                cfg.Env,
		WaitForIdleTimeout: cfg.WaitForIdleTimeout,
		ConditionTimeout:   cfg.ConditionTimeout,
		TypingFrequency:    cfg.TypingFrequency,
		DeviceInfo:         &deviceInfo,
		OnFlowStart:        onFlowStartWithCloud(cfg),
		OnStepComplete:     onStepComplete,
		OnNestedStep:       onNestedStep,
		OnNestedFlowStart:  onNestedFlowStart,
		OnFlowEnd:          onFlowEndWithCloud(cfg),
	})

	notifyCloudRunStart(cfg, 1)
	return runner.Run(context.Background(), []flow.Flow{f})
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// Slow step threshold in milliseconds (5 seconds)
const slowThresholdMs = 5000

// colorsEnabled determines if ANSI colors should be used
var colorsEnabled = true

func init() {
	// Respect NO_COLOR environment variable
	if os.Getenv("NO_COLOR") != "" {
		colorsEnabled = false
		return
	}
	// Check if stdout is a terminal
	if fileInfo, err := os.Stdout.Stat(); err == nil {
		if (fileInfo.Mode() & os.ModeCharDevice) == 0 {
			colorsEnabled = false
		}
	}
}

// color returns the color code if colors are enabled, empty string otherwise
func color(c string) string {
	if colorsEnabled {
		return c
	}
	return ""
}

// Live progress callbacks
// Note: For unified output, we'll read DeviceInfo from the runner config
// This callback is used during execution but detailed device info is shown in summary
func onFlowStart(flowIdx, totalFlows int, name, file string) {
	fmt.Printf("\n  %s[%d/%d]%s %s%s%s (%s)\n",
		color(colorCyan), flowIdx+1, totalFlows, color(colorReset),
		color(colorBold), name, color(colorReset), file)
	fmt.Println(strings.Repeat("─", 60))
}

// notifyCloudRunStart fires CloudProvider.OnRunStart once before the first flow.
// Errors are logged and do not abort the run.
func notifyCloudRunStart(cfg *RunConfig, totalFlows int) {
	if cfg.CloudProvider == nil {
		return
	}
	if err := cfg.CloudProvider.OnRunStart(cfg.CloudMeta, totalFlows); err != nil {
		logger.Warn("%s OnRunStart failed: %v", cfg.CloudProvider.Name(), err)
	}
}

// onFlowStartWithCloud returns a flow-start callback that logs console progress
// and fires CloudProvider.OnFlowStart when a cloud provider is configured.
func onFlowStartWithCloud(cfg *RunConfig) func(flowIdx, totalFlows int, name, file string) {
	return func(flowIdx, totalFlows int, name, file string) {
		onFlowStart(flowIdx, totalFlows, name, file)
		if cfg.CloudProvider == nil {
			return
		}
		if err := cfg.CloudProvider.OnFlowStart(cfg.CloudMeta, flowIdx, totalFlows, name, file); err != nil {
			logger.Warn("%s OnFlowStart failed: %v", cfg.CloudProvider.Name(), err)
		}
	}
}

// onFlowEndWithCloud returns a flow-end callback that logs console progress
// and fires CloudProvider.OnFlowEnd when a cloud provider is configured.
func onFlowEndWithCloud(cfg *RunConfig) func(name string, passed bool, durationMs int64, errMsg string) {
	return func(name string, passed bool, durationMs int64, errMsg string) {
		onFlowEnd(name, passed, durationMs, errMsg)
		if cfg.CloudProvider == nil {
			return
		}
		fr := &cloud.FlowResult{
			Name:     name,
			Passed:   passed,
			Duration: durationMs,
			Error:    errMsg,
		}
		if err := cfg.CloudProvider.OnFlowEnd(cfg.CloudMeta, fr); err != nil {
			logger.Warn("%s OnFlowEnd failed: %v", cfg.CloudProvider.Name(), err)
		}
	}
}

func onStepComplete(idx int, desc string, passed bool, durationMs int64, errMsg string) {
	// Don't mark runFlow/repeat/retry as slow - they contain multiple steps
	isCompoundStep := strings.HasPrefix(desc, "runFlow:") ||
		strings.HasPrefix(desc, "repeat:") ||
		strings.HasPrefix(desc, "retry:")
	isSlow := durationMs >= slowThresholdMs && !isCompoundStep
	durStr := formatDuration(durationMs)

	if passed {
		symbol := "✓"
		symbolColor := color(colorGreen)
		durColor := ""
		if isSlow {
			durColor = color(colorYellow)
			symbol = "⚠"
			symbolColor = color(colorYellow)
		}
		fmt.Printf("    %s%s%s %s %s(%s)%s\n",
			symbolColor, symbol, color(colorReset), desc, durColor, durStr, color(colorReset))
	} else {
		fmt.Printf("    %s✗%s %s (%s)\n", color(colorRed), color(colorReset), desc, durStr)
		if errMsg != "" {
			fmt.Printf("      %s╰─%s %s\n", color(colorGray), color(colorReset), errMsg)
		}
	}
}

func onNestedFlowStart(depth int, desc string) {
	// Base indent (4 spaces) + 2 spaces per depth level
	indent := strings.Repeat("  ", 2+depth)
	fmt.Printf("%s%s▸%s %s\n", indent, color(colorCyan), color(colorReset), desc)
}

func onNestedStep(depth int, desc string, passed bool, durationMs int64, errMsg string) {
	// Base indent (4 spaces) + 2 spaces per depth level + 2 more for being inside the flow
	indent := strings.Repeat("  ", 2+depth+1)
	isSlow := durationMs >= slowThresholdMs
	durStr := formatDuration(durationMs)

	if passed {
		symbol := "✓"
		symbolColor := color(colorGreen)
		durColor := ""
		if isSlow {
			durColor = color(colorYellow)
			symbol = "⚠"
			symbolColor = color(colorYellow)
		}
		fmt.Printf("%s%s%s%s %s %s(%s)%s\n",
			indent, symbolColor, symbol, color(colorReset), desc, durColor, durStr, color(colorReset))
	} else {
		fmt.Printf("%s%s✗%s %s (%s)\n", indent, color(colorRed), color(colorReset), desc, durStr)
		if errMsg != "" {
			fmt.Printf("%s  %s╰─%s %s\n", indent, color(colorGray), color(colorReset), errMsg)
		}
	}
}

func onFlowEnd(name string, passed bool, durationMs int64, errMsg string) {
	if passed {
		fmt.Printf("%s✓ %s%s %s%s%s\n",
			color(colorGreen), color(colorReset), name, color(colorGray), formatDuration(durationMs), color(colorReset))
	} else {
		fmt.Printf("%s✗ %s%s %s%s%s\n",
			color(colorRed), color(colorReset), name, color(colorGray), formatDuration(durationMs), color(colorReset))
	}
}

func printSummary(result *executor.RunResult) {
	// Calculate totals
	totalSteps := 0
	passedSteps := 0
	failedSteps := 0
	skippedSteps := 0
	for _, fr := range result.FlowResults {
		totalSteps += fr.StepsTotal
		passedSteps += fr.StepsPassed
		failedSteps += fr.StepsFailed
		skippedSteps += fr.StepsSkipped
	}

	// Print step summary
	fmt.Println()
	if passedSteps > 0 {
		fmt.Printf("  %s%d steps passing%s (%s)\n", color(colorGreen), passedSteps, color(colorReset), formatDuration(result.Duration))
	}
	if failedSteps > 0 {
		fmt.Printf("  %s%d steps failing%s\n", color(colorRed), failedSteps, color(colorReset))
	}
	if skippedSteps > 0 {
		fmt.Printf("  %s%d steps skipped%s\n", color(colorCyan), skippedSteps, color(colorReset))
	}
	fmt.Println()

	// Print table
	tableWidth := 92
	fmt.Println(strings.Repeat("═", tableWidth))
	fmt.Printf("  %-42s %6s %7s %6s %6s %6s %10s\n", "Flow", "Status", "Steps", "Pass", "Fail", "Skip", "Duration")
	fmt.Println(strings.Repeat("─", tableWidth))

	// Print each flow result
	for _, fr := range result.FlowResults {
		var status string
		var statusColor string
		if fr.Status == report.StatusFailed {
			status = "✗ FAIL"
			statusColor = color(colorRed)
		} else if fr.Status == report.StatusSkipped {
			status = "- SKIP"
			statusColor = color(colorCyan)
		} else {
			status = "✓ PASS"
			statusColor = color(colorGreen)
		}

		// Truncate name if too long
		name := fr.Name
		if len(name) > 42 {
			name = name[:39] + "..."
		}

		fmt.Printf("  %-42s %s%6s%s %7d %6d %6d %6d %10s\n",
			name, statusColor, status, color(colorReset),
			fr.StepsTotal, fr.StepsPassed, fr.StepsFailed, fr.StepsSkipped,
			formatDuration(fr.Duration))
	}

	// Print totals row
	fmt.Println(strings.Repeat("─", tableWidth))
	statusStr := fmt.Sprintf("%d/%d", result.PassedFlows, result.TotalFlows)
	statusColor := color(colorGreen)
	if result.FailedFlows > 0 {
		statusColor = color(colorRed)
	}
	fmt.Printf("  %s%-42s%s %s%6s%s %7d %6d %6d %6d %10s\n",
		color(colorBold), "TOTAL", color(colorReset),
		statusColor, statusStr, color(colorReset),
		totalSteps, passedSteps, failedSteps, skippedSteps,
		formatDuration(result.Duration))
	fmt.Println(strings.Repeat("═", tableWidth))
}

// formatDuration formats milliseconds to a human-readable string.
// Shows milliseconds for values < 1s, seconds otherwise.
func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	mins := ms / 60000
	secs := (ms % 60000) / 1000
	return fmt.Sprintf("%dm %ds", mins, secs)
}

func parseEnvVars(envs []string) map[string]string {
	result := make(map[string]string)
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

// loadCapabilities loads Appium capabilities from a JSON file.
func loadCapabilities(capsFile string) (map[string]interface{}, error) {
	data, err := os.ReadFile(capsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read caps file: %w", err)
	}

	var caps map[string]interface{}
	if err := json.Unmarshal(data, &caps); err != nil {
		return nil, fmt.Errorf("failed to parse caps JSON: %w", err)
	}
	return caps, nil
}

// executeAppiumSingleSession runs all flows using a single Appium session.
// clearState is handled in-session via terminate + pm clear (same as UIA2).
func executeAppiumSingleSession(cfg *RunConfig, flows []flow.Flow) (*executor.RunResult, error) {
	driver, cleanup, err := createAppiumDriver(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Appium driver: %w", err)
	}
	cleanupMu.Lock()
	activeCleanup = cleanup
	cleanupMu.Unlock()
	defer func() {
		cleanup()
		cleanupMu.Lock()
		activeCleanup = nil
		cleanupMu.Unlock()
	}()

	deviceInfo := buildDeviceReport(driver)

	runner := executor.New(driver, executor.RunnerConfig{
		OutputDir:          cfg.OutputDir,
		Parallelism:        0,
		Artifacts:          cfg.Artifacts,
		Device:             deviceInfo,
		App:                buildAppReport(driver),
		RunnerVersion:      Version,
		DriverName:         "appium",
		Env:                cfg.Env,
		WaitForIdleTimeout: cfg.WaitForIdleTimeout,
		ConditionTimeout:   cfg.ConditionTimeout,
		TypingFrequency:    cfg.TypingFrequency,
		DeviceInfo:         &deviceInfo,
		OnFlowStart:        onFlowStartWithCloud(cfg),
		OnStepComplete:     onStepComplete,
		OnNestedStep:       onNestedStep,
		OnNestedFlowStart:  onNestedFlowStart,
		OnFlowEnd:          onFlowEndWithCloud(cfg),
	})

	notifyCloudRunStart(cfg, len(flows))
	return runner.Run(context.Background(), flows)
}

// executeAppiumParallel runs flows across N Appium sessions in parallel.
// Each session hits the same Appium URL — the server allocates devices.
func executeAppiumParallel(cfg *RunConfig, count int, flows []flow.Flow) (*executor.RunResult, error) {
	workers, cloudMetas, err := createAppiumWorkers(cfg, count)
	if err != nil {
		return nil, fmt.Errorf("failed to create Appium workers: %w", err)
	}

	// Register all worker cleanups for signal handler
	allCleanup := func() {
		for _, w := range workers {
			w.Cleanup()
		}
	}
	cleanupMu.Lock()
	activeCleanup = allCleanup
	cleanupMu.Unlock()
	defer func() {
		cleanupMu.Lock()
		activeCleanup = nil
		cleanupMu.Unlock()
	}()

	platform := strings.ToLower(cfg.Platform)
	if platform == "" {
		platform = "android"
	}

	parallelRunner := createParallelRunner(cfg, workers, platform)
	result, err := parallelRunner.Run(context.Background(), flows)
	if err != nil {
		return nil, err
	}

	// Report to cloud providers per-worker (each session = separate cloud job).
	// Each worker filters down to only the flows it actually ran, so the cloud
	// dashboard's job page reflects that worker's slice of the run rather than
	// the run-wide totals.
	for i, cm := range cloudMetas {
		if cm.provider == nil {
			continue
		}
		workerSessionID := workers[i].SessionID

		var workerFlows []cloud.FlowResult
		var passedCount, failedCount int
		var durationMs int64
		for _, f := range result.FlowResults {
			if f.SessionID != workerSessionID {
				continue
			}
			workerFlows = append(workerFlows, cloud.FlowResult{
				Name:     f.Name,
				File:     f.SourceFile,
				Passed:   f.Status == report.StatusPassed,
				Duration: f.Duration,
				Error:    f.Error,
			})
			durationMs += f.Duration
			switch f.Status {
			case report.StatusPassed:
				passedCount++
			case report.StatusFailed:
				failedCount++
			}
		}
		cloudResult := &cloud.TestResult{
			Passed:      failedCount == 0 && len(workerFlows) > 0,
			Total:       len(workerFlows),
			PassedCount: passedCount,
			FailedCount: failedCount,
			Duration:    durationMs,
			OutputDir:   cfg.OutputDir,
			Flows:       workerFlows,
		}
		if err := cm.provider.ReportResult(cfg.AppiumURL, cm.meta, cloudResult); err != nil {
			logger.Warn("[appium-%d] %s result reporting failed: %v", i+1, cm.provider.Name(), err)
		} else {
			logger.Info("[appium-%d] %s job updated: passed=%v (flows=%d)", i+1, cm.provider.Name(), cloudResult.Passed, len(workerFlows))
		}
	}

	return result, nil
}

// CreateDriver creates the appropriate driver for the platform.
// Returns the driver, a cleanup function, and any error.
// Exported for library use - call once, reuse across multiple flows.
func CreateDriver(cfg *RunConfig) (core.Driver, func(), error) {
	platform := strings.ToLower(cfg.Platform)
	driverType := strings.ToLower(cfg.Driver)

	// Mock driver for testing
	if platform == "mock" || driverType == "mock" {
		driver := mock.New(mock.Config{
			Platform: cfg.Platform,
			DeviceID: getFirstDevice(cfg),
		})
		return driver, func() {}, nil
	}

	switch platform {
	case "android", "":
		return CreateAndroidDriver(cfg)
	case "ios":
		return CreateIOSDriver(cfg)
	case "web":
		return CreateWebDriver(cfg)
	default:
		return nil, nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}

// printSetupStep prints a setup step with spinner-style prefix
func printSetupStep(msg string) {
	fmt.Printf("  %s⏳%s %s\n", color(colorCyan), color(colorReset), msg)
}

// printSetupSuccess prints a success message for setup
func printSetupSuccess(msg string) {
	fmt.Printf("  %s✓%s %s\n", color(colorGreen), color(colorReset), msg)
}

// createAppiumDriver creates a driver that connects to an external Appium server.
// Uses capabilities from --caps file, with CLI flags taking precedence.
func createAppiumDriver(cfg *RunConfig) (core.Driver, func(), error) {
	printSetupStep(fmt.Sprintf("Connecting to Appium server: %s", cfg.AppiumURL))
	logger.Info("Creating Appium driver, server URL: %s", cfg.AppiumURL)

	// Start with capabilities from file (or empty map)
	caps := cfg.Capabilities
	if caps == nil {
		caps = make(map[string]interface{})
	}

	// CLI flags override caps file values
	deviceID := getFirstDevice(cfg)
	if cfg.Platform != "" {
		caps["platformName"] = cfg.Platform
	}
	if deviceID != "" {
		caps["appium:deviceName"] = deviceID
		caps["appium:udid"] = deviceID
	}
	if cfg.AppFile != "" {
		caps["appium:app"] = cfg.AppFile
	}

	// Set defaults if not provided
	if caps["platformName"] == nil {
		caps["platformName"] = "Android"
	}
	if caps["appium:automationName"] == nil {
		caps["appium:automationName"] = "UiAutomator2"
	}
	// Auto-grant permissions by default (user can override with false in caps file)
	if caps["appium:autoGrantPermissions"] == nil {
		caps["appium:autoGrantPermissions"] = true
	}

	// Add waitForIdleTimeout to capabilities for session creation
	// Priority: Flow config > CLI flag > Workspace config > Cap file > Default (5000ms)
	// (Flow config override is handled in flow_runner.go)
	// Using appium:settings capability to pass settings during session creation (saves one HTTP call)
	if caps["appium:settings"] == nil {
		caps["appium:settings"] = make(map[string]interface{})
	}
	if settings, ok := caps["appium:settings"].(map[string]interface{}); ok {
		settings["waitForIdleTimeout"] = cfg.WaitForIdleTimeout
	}

	printSetupStep("Creating Appium session...")
	logger.Info("Creating Appium session with capabilities: %v", caps)
	driver, err := appiumdriver.NewDriver(cfg.AppiumURL, caps)
	if err != nil {
		logger.Error("Failed to create Appium session: %v", err)
		return nil, nil, fmt.Errorf("create Appium session: %w", err)
	}
	info := driver.GetPlatformInfo()
	logger.Info("Appium session created successfully: %s", info.DeviceID)

	// Build the provider chain: detected cloud provider (if any) + the session
	// exporter (if --appium-session-file is set). Composite runs them all, so a
	// cloud provider and the session file work together. See issue #91.
	var providers []cloud.Provider
	if p := cloud.Detect(cfg.AppiumURL); p != nil {
		providers = append(providers, p)
	}
	if cfg.AppiumSessionFile != "" {
		providers = append(providers, cloud.NewSessionExporter(cfg.AppiumSessionFile))
	}
	if cp := cloud.Composite(providers...); cp != nil {
		cfg.CloudProvider = cp
		cfg.CloudMeta = make(map[string]string)
		cp.ExtractMeta(driver.SessionID(), driver.SessionCaps(), cfg.CloudMeta)
		cfg.CloudMeta[cloud.MetaAppiumURL] = strings.TrimSpace(cfg.AppiumURL)
		cfg.CloudMeta[cloud.MetaDeviceID] = info.DeviceID
		logger.Info("Cloud provider(s): %s", cp.Name())
	}

	// Print session details
	sessionDetail := fmt.Sprintf("Appium session created (session: %s", driver.SessionID())
	if info.DeviceName != "" {
		sessionDetail += fmt.Sprintf(", device: %s", info.DeviceName)
	}
	if info.OSVersion != "" {
		sessionDetail += fmt.Sprintf(", OS: %s %s", info.Platform, info.OSVersion)
	}
	sessionDetail += ")"
	printSetupSuccess(sessionDetail)

	// Cleanup function
	cleanup := func() {
		if err := driver.Close(); err != nil {
			logger.Debug("failed to close driver during cleanup: %v", err)
		}
	}

	return driver, cleanup, nil
}

// runCommand runs a command and returns stdout.
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// isPortInUse checks if a TCP port is already bound on localhost.
// Used to detect if another maestro-runner instance is using the same device.
func isPortInUse(port uint16) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return true // port in use
	}
	if err := ln.Close(); err != nil {
		logger.Debug("failed to close listener: %v", err)
	}
	return false
}

// checkDeviceAvailable checks if a device is available and not in use.
// Returns error if device is busy or not found.
func checkDeviceAvailable(deviceID, platform string) error {
	if platform == "ios" {
		// Check iOS device/simulator availability via port
		port := wdadriver.PortFromUDID(deviceID)
		if isPortInUse(port) {
			return fmt.Errorf("device is in use (port %d already bound)", port)
		}
	} else {
		// Check Android device availability via socket
		// For emulators, deviceID is like "emulator-5554"
		// For physical devices, it's the serial number
		socketPath := fmt.Sprintf("/tmp/uia2-%s.sock", deviceID)
		if isSocketInUse(socketPath) {
			return fmt.Errorf("device is in use (socket %s already bound)", socketPath)
		}

		// Also verify device is connected via adb
		cmd := exec.Command("adb", "-s", deviceID, "get-state")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("device not found or not connected")
		}
		if strings.TrimSpace(string(output)) != "device" {
			return fmt.Errorf("device state is not 'device': %s", strings.TrimSpace(string(output)))
		}
	}
	return nil
}

// enhanceNoDevicesError enhances error suggestions with actual command context.
// Emulator flags are global flags, so they must appear before the "test" subcommand.
func enhanceNoDevicesError(noDevErr *device.NoDevicesError, cfg *RunConfig) {
	// Split os.Args into: binary + global flags vs "test" subcommand + its args.
	// Example: maestro-runner --platform android test flow.yaml
	//   → globalPart = "maestro-runner --platform android"
	//   → testPart   = "test flow.yaml"
	args := os.Args
	globalPart := args[0] // at minimum the binary name
	testPart := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "test" {
			globalPart = strings.Join(args[:i], " ")
			testPart = " " + strings.Join(args[i:], " ")
			break
		}
	}
	// If no "test" subcommand found, treat entire command as global
	if testPart == "" {
		globalPart = strings.Join(args, " ")
	}

	logger.Debug("Enhancing NoDevicesError: global=%q test=%q", globalPart, testPart)

	// Build suggestions inserting flags between global part and test part
	for i, suggestion := range noDevErr.Suggestions {
		if strings.Contains(suggestion, "--auto-start-emulator <flow>") {
			noDevErr.Suggestions[i] = strings.ReplaceAll(suggestion,
				"maestro-runner --auto-start-emulator <flow>",
				globalPart+" --auto-start-emulator"+testPart)
		} else if strings.Contains(suggestion, "--start-emulator") && strings.Contains(suggestion, "<flow>") {
			parts := strings.Fields(suggestion)
			var avdName string
			for j, part := range parts {
				if part == "--start-emulator" && j+1 < len(parts) {
					avdName = parts[j+1]
					break
				}
			}
			noDevErr.Suggestions[i] = strings.ReplaceAll(suggestion,
				"maestro-runner --start-emulator "+avdName+" <flow>",
				globalPart+" --start-emulator "+avdName+testPart)
		} else if strings.Contains(suggestion, "--parallel") && strings.Contains(suggestion, "<flows>") {
			parts := strings.Fields(suggestion)
			var parallelCount string
			for j, part := range parts {
				if part == "--parallel" && j+1 < len(parts) {
					parallelCount = parts[j+1]
					break
				}
			}
			noDevErr.Suggestions[i] = strings.ReplaceAll(suggestion,
				"maestro-runner --parallel "+parallelCount+" --auto-start-emulator <flows>",
				globalPart+" --parallel "+parallelCount+" --auto-start-emulator"+testPart)
		}
	}

	logger.Debug("Enhanced suggestions count: %d", len(noDevErr.Suggestions))
	for i, s := range noDevErr.Suggestions {
		logger.Debug("  Suggestion %d: %s", i+1, s)
	}
}

// buildParallelDeviceError creates a helpful error when --parallel needs more devices.
func buildParallelDeviceError(cfg *RunConfig, found int) error {
	msg := fmt.Sprintf("--parallel %d requires %d devices but only found %d\n", cfg.Parallel, cfg.Parallel, found)

	// Try to list available AVDs
	avds, _ := emulator.ListAVDs()
	if len(avds) > 0 {
		msg += "\nAvailable AVDs:\n"
		for _, avd := range avds {
			msg += fmt.Sprintf("  - %s\n", avd.Name)
		}
	}

	msg += "\nOptions:\n"
	optNum := 1

	if found > 0 {
		msg += fmt.Sprintf("  %d. Connect %d more device(s) via USB\n", optNum, cfg.Parallel-found)
		optNum++
	} else {
		msg += fmt.Sprintf("  %d. Connect %d device(s) via USB (enable USB debugging)\n", optNum, cfg.Parallel)
		optNum++
	}

	if len(avds) > 0 {
		// Build the actual command the user would run
		args := os.Args
		globalPart := args[0]
		testPart := ""
		for i := 1; i < len(args); i++ {
			if args[i] == "test" {
				globalPart = strings.Join(args[:i], " ")
				testPart = " " + strings.Join(args[i:], " ")
				break
			}
		}
		if testPart == "" {
			globalPart = strings.Join(args, " ")
		}

		msg += fmt.Sprintf("  %d. Auto-start emulators: %s --auto-start-emulator%s\n", optNum, globalPart, testPart)
		optNum++

		msg += fmt.Sprintf("  %d. Start emulators manually:\n", optNum)
		limit := cfg.Parallel
		if limit > len(avds) {
			limit = len(avds)
		}
		for i := 0; i < limit; i++ {
			msg += fmt.Sprintf("     emulator -avd %s &\n", avds[i].Name)
		}
		if cfg.Parallel > len(avds) {
			msg += fmt.Sprintf("     (need %d more AVD(s) — create with: avdmanager create avd)\n", cfg.Parallel-len(avds))
		}
	} else {
		msg += fmt.Sprintf("  %d. Create AVDs: avdmanager create avd --name <name> --package <system-image>\n", optNum)
	}

	return fmt.Errorf("%s", msg)
}

// buildNotEnoughAVDsError creates a helpful error when --parallel N requires more
// unique AVDs than are available. Each parallel emulator needs its own AVD because
// Android locks the AVD directory at boot, preventing the same AVD from running twice.
func buildNotEnoughAVDsError(cfg *RunConfig, existingDevices int, avds []emulator.AVDInfo) error {
	needed := cfg.Parallel - existingDevices
	msg := fmt.Sprintf("--parallel %d needs %d emulator(s) but only %d AVD(s) available\n", cfg.Parallel, needed, len(avds))
	msg += "\nEach emulator requires a unique AVD (Android locks the AVD directory at boot).\n"

	msg += "\nAvailable AVDs:\n"
	for _, avd := range avds {
		msg += fmt.Sprintf("  - %s\n", avd.Name)
	}

	msg += "\nOptions:\n"
	optNum := 1

	// Option: connect physical devices
	shortfall := needed - len(avds)
	msg += fmt.Sprintf("  %d. Connect %d more device(s) via USB (enable USB debugging)\n", optNum, shortfall)
	optNum++

	// Option: create more AVDs
	msg += fmt.Sprintf("  %d. Create %d more AVD(s):\n", optNum, shortfall)
	for i := len(avds); i < needed; i++ {
		msg += fmt.Sprintf("     avdmanager create avd --name Device_%d --package \"system-images;android-34;google_apis;x86_64\"\n", i+1)
	}

	return fmt.Errorf("%s", msg)
}

// getFirstDevice returns the first device from config, or empty string if none.
func getFirstDevice(cfg *RunConfig) string {
	if len(cfg.Devices) > 0 {
		return cfg.Devices[0]
	}
	return ""
}

// getDriversDir returns the absolute path to the drivers subdirectory.
// Uses MAESTRO_RUNNER_HOME resolution (env var → binary parent → cwd).
func getDriversDir(platform string) (string, error) {
	driversPath := config.GetDriversDir(platform)
	if _, err := os.Stat(driversPath); os.IsNotExist(err) {
		return "", fmt.Errorf("drivers directory not found: %s\n"+
			"  MAESTRO_RUNNER_HOME: %s\n"+
			"  Hint: Set MAESTRO_RUNNER_HOME or place drivers/ in the maestro-runner home directory",
			driversPath, config.GetHome())
	}
	return driversPath, nil
}

// isSocketInUse checks if a Unix socket is actively owned by a running process.
// Uses PID file to distinguish a live owner from a stale socket left by a crashed process.
func isSocketInUse(socketPath string) bool {
	if socketPath == "" {
		return false
	}

	// Check if socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return false
	}

	// Check if the owning process is still alive via PID file
	return device.IsOwnerAlive(socketPath)
}

// autoDetectDevicesFn is the device-discovery function used by
// determineExecutionMode. Tests override it to make discovery hermetic.
var autoDetectDevicesFn = autoDetectDevices

// autoDetectDevices finds N available devices for the specified platform.
// Returns device IDs that can be used for parallel execution.
func autoDetectDevices(platform string, count int) ([]string, error) {
	if count <= 0 {
		return nil, fmt.Errorf("device count must be positive")
	}

	platform = strings.ToLower(platform)
	switch platform {
	case "android", "":
		return autoDetectAndroidDevices(count)
	case "ios":
		return autoDetectIOSDevices(count)
	default:
		return nil, fmt.Errorf("unsupported platform for auto-detection: %s", platform)
	}
}

// executeParallel runs tests in parallel across multiple devices.
func executeParallel(cfg *RunConfig, deviceIDs []string, flows []flow.Flow) (*executor.RunResult, error) {
	if len(deviceIDs) == 0 {
		return nil, fmt.Errorf("no devices available for parallel execution")
	}

	platform := strings.ToLower(cfg.Platform)
	if platform == "" {
		platform = "android"
	}

	// 1. Validate devices (skip for web — no physical devices)
	if platform != "web" {
		if err := validateDevicesAvailable(deviceIDs, platform); err != nil {
			return nil, err
		}
	}

	// 2. Create workers
	var workers []executor.DeviceWorker
	var err error
	if platform == "web" {
		workers, err = createBrowserWorkers(cfg, len(deviceIDs))
	} else {
		workers, err = createDeviceWorkers(cfg, deviceIDs, platform)
	}
	if err != nil {
		return nil, err
	}

	// Register all worker cleanups for signal handler
	allCleanup := func() {
		for _, w := range workers {
			w.Cleanup()
		}
	}
	cleanupMu.Lock()
	activeCleanup = allCleanup
	cleanupMu.Unlock()
	defer func() {
		cleanupMu.Lock()
		activeCleanup = nil
		cleanupMu.Unlock()
	}()

	// 3. Run parallel
	parallelRunner := createParallelRunner(cfg, workers, platform)
	return parallelRunner.Run(context.Background(), flows)
}

// validateDevicesAvailable checks all devices before starting initialization.
func validateDevicesAvailable(deviceIDs []string, platform string) error {
	printSetupStep(fmt.Sprintf("Checking availability of %d device(s)...", len(deviceIDs)))

	var unavailableDevices []string
	for i, deviceID := range deviceIDs {
		if err := checkDeviceAvailable(deviceID, platform); err != nil {
			unavailableDevices = append(unavailableDevices,
				fmt.Sprintf("  Device %d/%d (%s): %v", i+1, len(deviceIDs), deviceID, err))
		}
	}

	if len(unavailableDevices) > 0 {
		return fmt.Errorf("%d device(s) not available:\n%s\n\nAll devices must be available to start parallel execution",
			len(unavailableDevices), strings.Join(unavailableDevices, "\n"))
	}

	printSetupSuccess(fmt.Sprintf("All %d device(s) available", len(deviceIDs)))
	return nil
}

// createDeviceWorkers creates a worker for each device.
func createDeviceWorkers(cfg *RunConfig, deviceIDs []string, platform string) ([]executor.DeviceWorker, error) {
	var workers []executor.DeviceWorker
	var cleanups []func()

	cleanupAll := func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}

	for i, deviceID := range deviceIDs {
		printSetupStep(fmt.Sprintf("[Device %d/%d] Connecting to %s...", i+1, len(deviceIDs), deviceID))

		deviceCfg := *cfg
		deviceCfg.Devices = []string{deviceID}

		var driver core.Driver
		var cleanup func()
		var err error

		if platform == "ios" {
			deviceCfg.Platform = "ios"
			driver, cleanup, err = CreateIOSDriver(&deviceCfg)
		} else {
			deviceCfg.Platform = "android"
			driver, cleanup, err = CreateAndroidDriver(&deviceCfg)
		}

		if err != nil {
			cleanupAll()
			return nil, fmt.Errorf("failed to create driver for device %s: %w", deviceID, err)
		}

		workers = append(workers, executor.DeviceWorker{
			ID:       i,
			DeviceID: deviceID,
			Driver:   driver,
			Cleanup:  cleanup,
		})
		cleanups = append(cleanups, cleanup)
	}

	return workers, nil
}

// createBrowserWorkers creates N browser driver workers for parallel web execution.
// Each worker gets its own browser process with a separate profile for full isolation.
func createBrowserWorkers(cfg *RunConfig, count int) ([]executor.DeviceWorker, error) {
	// Ensure browser binary is downloaded once before launching N workers
	if err := cdpdriver.EnsureBrowser(cdpdriver.Config{Browser: cfg.Browser}); err != nil {
		return nil, fmt.Errorf("failed to ensure browser binary: %w", err)
	}

	var workers []executor.DeviceWorker
	var cleanups []func()

	cleanupAll := func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}

	for i := 0; i < count; i++ {
		workerID := fmt.Sprintf("browser-%d", i+1)
		printSetupStep(fmt.Sprintf("[%s] Launching browser...", workerID))

		driver, cleanup, err := CreateWebDriver(cfg)
		if err != nil {
			cleanupAll()
			return nil, fmt.Errorf("failed to launch %s: %w", workerID, err)
		}

		workers = append(workers, executor.DeviceWorker{
			ID:       i,
			DeviceID: workerID,
			Driver:   driver,
			Cleanup:  cleanup,
		})
		cleanups = append(cleanups, cleanup)
	}

	return workers, nil
}

// appiumWorkerMeta holds per-worker cloud provider state for Appium parallel execution.
type appiumWorkerMeta struct {
	provider cloud.Provider
	meta     map[string]string
}

// createAppiumWorkers creates N Appium session workers against the same server URL.
// Each session is independent — the Appium server (local or cloud) allocates devices.
func createAppiumWorkers(cfg *RunConfig, count int) ([]executor.DeviceWorker, []appiumWorkerMeta, error) {
	var workers []executor.DeviceWorker
	var cloudMetas []appiumWorkerMeta
	var cleanups []func()

	cleanupAll := func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}

	// Preserve the caller's Devices so we can swap in per-worker UDIDs and
	// restore them at the end. When the user supplies len(Devices) >= count,
	// we round-robin so each Appium session targets a distinct device. When
	// fewer devices are configured (e.g. cloud mode where Appium picks), every
	// session runs against the same caps.
	originalDevices := cfg.Devices

	for i := 0; i < count; i++ {
		workerID := fmt.Sprintf("appium-%d", i+1)
		printSetupStep(fmt.Sprintf("[%s] Creating Appium session...", workerID))

		if len(originalDevices) >= count {
			cfg.Devices = []string{originalDevices[i]}
		} else {
			cfg.Devices = originalDevices
		}

		driver, cleanup, err := createAppiumDriver(cfg)
		if err != nil {
			cfg.Devices = originalDevices
			logger.Warn("Failed to create session for %s: %v", workerID, err)
			cleanupAll()
			return nil, nil, fmt.Errorf("failed to create %s: %w", workerID, err)
		}

		// Extract session ID for parallel output
		var sessionID string
		if appDrv, ok := driver.(*appiumdriver.Driver); ok {
			sessionID = appDrv.SessionID()
		}

		// Capture per-worker cloud metadata (each session = separate cloud job).
		// Bound the closures below to *this* iteration's provider/meta.
		workerProvider := cfg.CloudProvider
		workerMeta := cfg.CloudMeta
		workerName := workerID

		var onFlowStart func(flowIdx, totalFlows int, name, file string)
		var onFlowEnd func(name string, passed bool, durationMs int64, errMsg string)
		if workerProvider != nil {
			onFlowStart = func(flowIdx, totalFlows int, name, file string) {
				if err := workerProvider.OnFlowStart(workerMeta, flowIdx, totalFlows, name, file); err != nil {
					logger.Warn("[%s] %s OnFlowStart failed: %v", workerName, workerProvider.Name(), err)
				}
			}
			onFlowEnd = func(name string, passed bool, durationMs int64, errMsg string) {
				fr := &cloud.FlowResult{
					Name:     name,
					Passed:   passed,
					Duration: durationMs,
					Error:    errMsg,
				}
				if err := workerProvider.OnFlowEnd(workerMeta, fr); err != nil {
					logger.Warn("[%s] %s OnFlowEnd failed: %v", workerName, workerProvider.Name(), err)
				}
			}
		}

		workers = append(workers, executor.DeviceWorker{
			ID:          i,
			DeviceID:    workerID,
			SessionID:   sessionID,
			Driver:      driver,
			Cleanup:     cleanup,
			OnFlowStart: onFlowStart,
			OnFlowEnd:   onFlowEnd,
		})
		cleanups = append(cleanups, cleanup)

		cloudMetas = append(cloudMetas, appiumWorkerMeta{
			provider: workerProvider,
			meta:     workerMeta,
		})
		// Reset for next worker so createAppiumDriver detects fresh
		cfg.CloudProvider = nil
		cfg.CloudMeta = nil
	}

	cfg.Devices = originalDevices

	return workers, cloudMetas, nil
}

// createParallelRunner builds the parallel runner with config.
func createParallelRunner(cfg *RunConfig, workers []executor.DeviceWorker, platform string) *executor.ParallelRunner {
	driverName := resolveDriverName(cfg, platform)

	firstDriver := workers[0].Driver
	deviceInfo := buildDeviceReport(firstDriver)
	deviceInfo.Name = fmt.Sprintf("%d devices", len(workers))
	runnerConfig := executor.RunnerConfig{
		OutputDir:          cfg.OutputDir,
		Parallelism:        0,
		Artifacts:          cfg.Artifacts,
		Device:             deviceInfo,
		App:                buildAppReport(firstDriver),
		RunnerVersion:      Version,
		DriverName:         driverName,
		Env:                cfg.Env,
		WaitForIdleTimeout: cfg.WaitForIdleTimeout,
		ConditionTimeout:   cfg.ConditionTimeout,
		TypingFrequency:    cfg.TypingFrequency,
		// Callbacks will be set per-worker in parallel.go with device info
	}

	return executor.NewParallelRunner(workers, runnerConfig)
}
