package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/config"
	"github.com/devicelab-dev/maestro-runner/pkg/core"
	appiumdriver "github.com/devicelab-dev/maestro-runner/pkg/driver/appium"
	"github.com/devicelab-dev/maestro-runner/pkg/driver/mock"
	wdadriver "github.com/devicelab-dev/maestro-runner/pkg/driver/wda"
	"github.com/devicelab-dev/maestro-runner/pkg/emulator"
	"github.com/devicelab-dev/maestro-runner/pkg/executor"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
	"github.com/devicelab-dev/maestro-runner/pkg/validator"
	"github.com/urfave/cli/v2"
)

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
  maestro-runner test flows/ -e USER=test -e PASS=secret

  # With tag filtering
  maestro-runner test flows/ --include-tags smoke

  # With Appium and capabilities file
  maestro-runner --driver appium --caps caps.json test flow.yaml

  # With cloud provider (BrowserStack/SauceLabs/LambdaTest)
  maestro-runner --driver appium --appium-url "https://hub.provider.com/wd/hub" --caps cloud.json test flow.yaml

  # Custom output directory
  maestro-runner test flows/ --output ./my-reports --flatten`,
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
			Name:  "headless",
			Usage: "Run in headless mode (web only)",
		},

		// Driver settings
		&cli.IntFlag{
			Name:    "wait-for-idle-timeout",
			Usage:   "Wait for device idle in ms (0 = disabled, default 5000)",
			Value:   5000,
			EnvVars: []string{"MAESTRO_WAIT_FOR_IDLE_TIMEOUT"},
		},

		// AI options
		&cli.BoolFlag{
			Name:  "analyze",
			Usage: "Enhance output with AI insights",
		},
		&cli.StringFlag{
			Name:  "api-url",
			Usage: "API base URL",
			Value: "https://api.copilot.mobile.dev",
		},
		&cli.StringFlag{
			Name:  "api-key",
			Usage: "API key",
		},
	},
	Action: runTest,
}

// parseDevices parses device configuration from --device and --parallel flags.
// Returns a slice of device UDIDs:
// - If --device has comma-separated values: split and return them
// - If --parallel N without --device: return nil (auto-detect handled later)
// - If neither: return nil (single device mode, auto-detect)
func parseDevices(deviceFlag string, parallelCount int, platform string) []string {
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

// handleEmulatorStartup starts Android emulators if requested via CLI flags.
// Handles two cases:
// 1. --start-emulator: Explicitly start a specific AVD
// 2. --auto-start-emulator: Start an emulator if no devices are found
func handleEmulatorStartup(cfg *RunConfig, mgr *emulator.Manager) error {
	// Only handle Android emulators
	if cfg.Platform != "" && cfg.Platform != "android" {
		return nil
	}

	timeout := time.Duration(cfg.BootTimeout) * time.Second
	if timeout == 0 {
		timeout = 180 * time.Second // Default 3 minutes
	}

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
	if cfg.AutoStartEmulator {
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
			return fmt.Errorf("no devices and no AVDs available. Create an AVD with: avdmanager create avd ...")
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
	Continuous bool
	Headless   bool

	// Device
	Platform string
	Devices  []string // Device UDIDs (can be comma-separated or multiple from --parallel)
	Verbose  bool
	AppFile  string // App binary to install before testing
	AppID    string // App bundle ID or package name

	// Driver
	Driver       string                 // uiautomator2, appium
	AppiumURL    string                 // Appium server URL
	CapsFile     string                 // Appium capabilities JSON file path
	Capabilities map[string]interface{} // Parsed Appium capabilities

	// Driver settings
	WaitForIdleTimeout int    // Wait for device idle in ms (0 = disabled, default 5000)
	TeamID             string // Apple Development Team ID for WDA code signing

	// Emulator management
	StartEmulator     string // AVD name to start (e.g., Pixel_7_API_33)
	AutoStartEmulator bool   // Auto-start an emulator if no devices found
	ShutdownAfter     bool   // Shutdown emulators started by maestro-runner after tests
	BootTimeout       int    // Emulator boot timeout in seconds
}

func printBanner() {
	// Make DeviceLab.dev clickable and colored (cyan)
	// OSC 8 hyperlink format: ESC]8;;URL BEL TEXT ESC]8;; BEL
	deviceLabLink := "\x1b]8;;https://devicelab.dev\x07" + color(colorCyan) + "DeviceLab.dev" + color(colorReset) + "\x1b]8;;\x07"

	// Make GitHub link clickable
	githubLink := "\x1b]8;;https://github.com/devicelab-dev/maestro-runner\x07Star us on GitHub\x1b]8;;\x07"

	// Box width is 64 characters (between the ║ symbols)
	// Calculate padding for version line
	// Visible text: "  maestro-runner " + Version + " - by DeviceLab.dev"
	versionLineVisible := 16 + len(Version) + 20 // "  maestro-runner " + version + " - by DeviceLab.dev"
	versionPadding := strings.Repeat(" ", 64-versionLineVisible)

	// Calculate padding for GitHub line
	// Visible text: "  ⭐ Star us on GitHub"
	githubLineVisible := 21 // "  ⭐ " + "Star us on GitHub" (⭐ is 3 bytes but 1 visual char)
	githubPadding := strings.Repeat(" ", 64-githubLineVisible)

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  maestro-runner %s - by %s%s   ║\n", Version, deviceLabLink, versionPadding)
	fmt.Println("║  Fast, lightweight Maestro test runner                            ║")
	fmt.Printf("║  ⭐ %s%s  ║\n", githubLink, githubPadding)
	fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func printFooter() {
	// Make DeviceLab.dev clickable and colored (cyan)
	deviceLabLink := "\x1b]8;;https://devicelab.dev\x07" + color(colorCyan) + "DeviceLab.dev" + color(colorReset) + "\x1b]8;;\x07"

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Printf("║ Built by %s - Turn Your Devices Into a Distributed Device Lab ║\n", deviceLabLink)
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func runTest(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("at least one flow file or folder is required")
	}

	// Print banner at start
	printBanner()

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

	// Merge env variables: workspace config env + CLI env (CLI takes precedence)
	mergedEnv := make(map[string]string)
	if workspaceConfig != nil {
		for k, v := range workspaceConfig.Env {
			mergedEnv[k] = v
		}
	}
	for k, v := range env {
		mergedEnv[k] = v // CLI overrides workspace config
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
		Headless:           getBool("headless"),
		Platform:           getString("platform"),
		Devices:            parseDevices(getString("device"), getInt("parallel"), getString("platform")),
		Verbose:            getBool("verbose"),
		AppFile:            getString("app-file"),
		AppID:              appID,
		Driver:             getString("driver"),
		AppiumURL:          getString("appium-url"),
		CapsFile:           capsFile,
		Capabilities:       caps,
		WaitForIdleTimeout: getInt("wait-for-idle-timeout"),
		TeamID:             getString("team-id"),
		StartEmulator:      getString("start-emulator"),
		AutoStartEmulator:  getBool("auto-start-emulator"),
		ShutdownAfter:      getBool("shutdown-after"),
		BootTimeout:        getInt("boot-timeout"),
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
			// else: keep default 5000ms from CLI flag
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
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
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

	// 2.5. Initialize emulator manager (for Android)
	emulatorMgr := emulator.NewManager()
	defer func() {
		if cfg.ShutdownAfter {
			logger.Info("Shutting down emulators started by maestro-runner...")
			if err := emulatorMgr.ShutdownAll(); err != nil {
				logger.Error("Failed to shutdown emulators: %v", err)
			}
		}
	}()

	// 3. Validate and parse flows
	flows, err := validateAndParseFlows(cfg)
	if err != nil {
		logger.Error("Flow validation failed: %v", err)
		return err
	}
	logger.Info("Validated %d flow(s)", len(flows))

	// Extract appId from first flow if not in config
	if cfg.AppID == "" && len(flows) > 0 && flows[0].Config.AppID != "" {
		cfg.AppID = flows[0].Config.AppID
	}

	// 3.5. Handle emulator startup (if requested)
	if err := handleEmulatorStartup(cfg, emulatorMgr); err != nil {
		logger.Error("Emulator startup failed: %v", err)
		return err
	}

	// 4. Determine execution mode and devices
	needsParallel, deviceIDs, err := determineExecutionMode(cfg, emulatorMgr)
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
	fmt.Printf("  %s⏳ Generating reports...%s\n", color(colorCyan), color(colorReset))
	fmt.Println()

	htmlPath := filepath.Join(cfg.OutputDir, "report.html")
	jsonPath := filepath.Join(cfg.OutputDir, "report.json")

	htmlGenerated := true
	if err := report.GenerateHTML(cfg.OutputDir, report.HTMLConfig{
		OutputPath: htmlPath,
		Title:      "Test Report",
	}); err != nil {
		htmlGenerated = false
		fmt.Printf("  %s⚠%s Warning: failed to generate HTML report: %v\n", color(colorYellow), color(colorReset), err)
	}

	// Display reports section
	fmt.Println("  Reports:")
	if htmlGenerated {
		fmt.Printf("    HTML:   %s\n", htmlPath)
	}
	fmt.Printf("    JSON:   %s\n", jsonPath)

	// 7. Print footer
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

// determineExecutionMode decides whether to run in parallel and which devices to use.
// If --parallel N is specified with --auto-start-emulator, this will start additional
// emulators to reach N total devices.
func determineExecutionMode(cfg *RunConfig, emulatorMgr *emulator.Manager) (needsParallel bool, deviceIDs []string, err error) {
	needsParallel = cfg.Parallel > 0 || len(cfg.Devices) > 1

	if needsParallel {
		if len(cfg.Devices) > 0 {
			deviceIDs = cfg.Devices
		} else if cfg.Parallel > 0 {
			// Try to auto-detect existing devices
			existingDevices, detectErr := autoDetectDevices(cfg.Platform, cfg.Parallel)
			if detectErr != nil && !cfg.AutoStartEmulator {
				// No devices and auto-start disabled
				return false, nil, fmt.Errorf("failed to auto-detect devices: %w", detectErr)
			}

			// Start with existing devices (may be empty if none found)
			if detectErr == nil {
				deviceIDs = existingDevices
			}

			// Check if we need more devices
			needed := cfg.Parallel - len(deviceIDs)
			if needed > 0 && cfg.AutoStartEmulator && (cfg.Platform == "" || cfg.Platform == "android") {
				logger.Info("Need %d more devices for --parallel %d, starting emulators...", needed, cfg.Parallel)
				fmt.Printf("  %s⏳ Starting %d emulator(s) for parallel execution...%s\n", color(colorCyan), needed, color(colorReset))

				// List available AVDs
				avds, err := emulator.ListAVDs()
				if err != nil {
					return false, nil, fmt.Errorf("failed to list AVDs: %w", err)
				}
				if len(avds) == 0 {
					return false, nil, fmt.Errorf("need %d more devices but no AVDs available. Create AVDs with: avdmanager create avd ...", needed)
				}

				// Start emulators sequentially to avoid port conflicts
				timeout := time.Duration(cfg.BootTimeout) * time.Second
				if timeout == 0 {
					timeout = 180 * time.Second
				}

				for i := 0; i < needed && i < len(avds); i++ {
					avdName := avds[i].Name
					logger.Info("Starting emulator %d/%d: %s", i+1, needed, avdName)
					fmt.Printf("  %s⏳ Starting emulator %d/%d: %s%s\n", color(colorCyan), i+1, needed, avdName, color(colorReset))

					serial, err := emulatorMgr.Start(avdName, timeout)
					if err != nil {
						return false, nil, fmt.Errorf("failed to start emulator %s: %w", avdName, err)
					}

					deviceIDs = append(deviceIDs, serial)
					logger.Info("Emulator started: %s (%d/%d)", serial, i+1, needed)
					fmt.Printf("  %s✓ Emulator started: %s%s\n", color(colorGreen), serial, color(colorReset))
				}

				if len(deviceIDs) < cfg.Parallel {
					return false, nil, fmt.Errorf("could only start %d/%d devices (found %d AVDs)", len(deviceIDs), cfg.Parallel, len(avds))
				}
			} else if needed > 0 {
				// Need more devices but auto-start is disabled
				return false, nil, fmt.Errorf("--parallel %d requires %d devices but only found %d. Use --auto-start-emulator to start additional emulators", cfg.Parallel, cfg.Parallel, len(deviceIDs))
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
		if needsParallel {
			return nil, fmt.Errorf("parallel execution not yet supported for Appium driver")
		}
		return executeFlowsWithPerFlowSession(cfg, flows)
	}

	if needsParallel {
		return executeParallel(cfg, deviceIDs, flows)
	}

	return executeSingleDevice(cfg, flows)
}

// executeSingleDevice runs flows on a single device.
func executeSingleDevice(cfg *RunConfig, flows []flow.Flow) (*executor.RunResult, error) {
	logger.Info("Creating driver for single device execution")
	driver, cleanup, err := createDriver(cfg)
	if err != nil {
		logger.Error("Failed to create driver: %v", err)
		return nil, fmt.Errorf("failed to create driver: %w", err)
	}
	defer cleanup()

	logger.Info("Driver created: %s on %s", driver.GetPlatformInfo().Platform, driver.GetPlatformInfo().DeviceName)

	driverName := "uiautomator2"
	if cfg.Platform == "mock" {
		driverName = "mock"
	}

	deviceInfo := &report.Device{
		ID:          driver.GetPlatformInfo().DeviceID,
		Platform:    driver.GetPlatformInfo().Platform,
		Name:        driver.GetPlatformInfo().DeviceName,
		OSVersion:   driver.GetPlatformInfo().OSVersion,
		IsSimulator: driver.GetPlatformInfo().IsSimulator,
	}

	runner := executor.New(driver, executor.RunnerConfig{
		OutputDir:   cfg.OutputDir,
		Parallelism: 0,
		Artifacts:   executor.ArtifactOnFailure,
		Device:      *deviceInfo,
		App: report.App{
			ID:      driver.GetPlatformInfo().AppID,
			Version: driver.GetPlatformInfo().AppVersion,
		},
		RunnerVersion:      "0.1.0",
		DriverName:         driverName,
		Env:                cfg.Env,
		WaitForIdleTimeout: cfg.WaitForIdleTimeout,
		DeviceInfo:         deviceInfo,
		OnFlowStart:        onFlowStart,
		OnStepComplete:     onStepComplete,
		OnNestedStep:       onNestedStep,
		OnNestedFlowStart:  onNestedFlowStart,
		OnFlowEnd:          onFlowEnd,
	})

	return runner.Run(context.Background(), flows)
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

// flowUsesClearState checks if a flow uses clearState in any of its steps.
// This includes launchApp with clearState:true and standalone clearState steps.
func flowUsesClearState(f *flow.Flow) bool {
	return checkStepsForClearState(f.Steps)
}

// checkStepsForClearState recursively checks steps for clearState usage.
func checkStepsForClearState(steps []flow.Step) bool {
	for _, step := range steps {
		switch s := step.(type) {
		case *flow.LaunchAppStep:
			if s.ClearState {
				return true
			}
		case *flow.ClearStateStep:
			return true
		case *flow.RepeatStep:
			if checkStepsForClearState(s.Steps) {
				return true
			}
		case *flow.RetryStep:
			if checkStepsForClearState(s.Steps) {
				return true
			}
		case *flow.RunFlowStep:
			if checkStepsForClearState(s.Steps) {
				return true
			}
			// Note: We don't load external flow files here - that would be too expensive.
			// For external runFlow files, clearState will be handled at runtime.
		}
	}
	return false
}

// executeFlowsWithPerFlowSession runs each flow with its own Appium session.
// This allows per-flow capability configuration (e.g., noReset based on clearState usage).
func executeFlowsWithPerFlowSession(cfg *RunConfig, flows []flow.Flow) (*executor.RunResult, error) {
	results := make([]executor.FlowResult, len(flows))
	var totalDuration int64

	for i, f := range flows {
		// Analyze flow for clearState usage
		usesClearState := flowUsesClearState(&f)

		// Create per-flow capabilities
		flowCaps := cloneCapabilities(cfg.Capabilities)

		// Add noReset: false if flow uses clearState (reset app at session start)
		if usesClearState {
			flowCaps["appium:noReset"] = false
			fmt.Printf("  %s→%s Flow uses clearState, setting noReset=false\n", color(colorCyan), color(colorReset))
		}

		// Create driver for this flow
		flowCfg := *cfg
		flowCfg.Capabilities = flowCaps

		// Apply flow-level waitForIdleTimeout override if specified
		// Priority: Flow config > CLI flag > Workspace config > Cap file > Default
		if f.Config.WaitForIdleTimeout != nil {
			flowCfg.WaitForIdleTimeout = *f.Config.WaitForIdleTimeout
		}
		driver, cleanup, err := createAppiumDriver(&flowCfg)
		if err != nil {
			// Record failure and continue to next flow
			results[i] = executor.FlowResult{
				ID:     fmt.Sprintf("flow-%d", i),
				Name:   f.Config.Name,
				Status: report.StatusFailed,
				Error:  fmt.Sprintf("failed to create driver: %v", err),
			}
			continue
		}

		// Create executor for single flow
		runner := executor.New(driver, executor.RunnerConfig{
			OutputDir:   cfg.OutputDir,
			Parallelism: 0,
			Artifacts:   executor.ArtifactOnFailure,
			Device: report.Device{
				ID:          driver.GetPlatformInfo().DeviceID,
				Platform:    driver.GetPlatformInfo().Platform,
				Name:        driver.GetPlatformInfo().DeviceName,
				OSVersion:   driver.GetPlatformInfo().OSVersion,
				IsSimulator: driver.GetPlatformInfo().IsSimulator,
			},
			App: report.App{
				ID:      driver.GetPlatformInfo().AppID,
				Version: driver.GetPlatformInfo().AppVersion,
			},
			RunnerVersion:      "0.1.0",
			DriverName:         "appium",
			Env:                cfg.Env,
			WaitForIdleTimeout: cfg.WaitForIdleTimeout,
			OnFlowStart:        func(flowIdx, totalFlows int, name, file string) { onFlowStart(i, len(flows), name, file) },
			OnStepComplete:     onStepComplete,
			OnNestedStep:       onNestedStep,
			OnNestedFlowStart:  onNestedFlowStart,
			OnFlowEnd:          onFlowEnd,
		})

		// Run single flow
		result, err := runner.Run(context.Background(), []flow.Flow{f})
		cleanup() // Clean up session after flow

		if err != nil {
			results[i] = executor.FlowResult{
				ID:     fmt.Sprintf("flow-%d", i),
				Name:   f.Config.Name,
				Status: report.StatusFailed,
				Error:  err.Error(),
			}
		} else if len(result.FlowResults) > 0 {
			results[i] = result.FlowResults[0]
			totalDuration += result.Duration
		}
	}

	// Build aggregated result
	runResult := &executor.RunResult{
		TotalFlows:  len(flows),
		FlowResults: results,
		Duration:    totalDuration,
	}

	for _, fr := range results {
		switch fr.Status {
		case report.StatusPassed:
			runResult.PassedFlows++
		case report.StatusFailed:
			runResult.FailedFlows++
		case report.StatusSkipped:
			runResult.SkippedFlows++
		}
	}

	if runResult.FailedFlows > 0 {
		runResult.Status = report.StatusFailed
	} else {
		runResult.Status = report.StatusPassed
	}

	return runResult, nil
}

// cloneCapabilities creates a copy of capabilities map.
func cloneCapabilities(caps map[string]interface{}) map[string]interface{} {
	if caps == nil {
		return make(map[string]interface{})
	}
	cloned := make(map[string]interface{}, len(caps))
	for k, v := range caps {
		cloned[k] = v
	}
	return cloned
}

// createDriver creates the appropriate driver for the platform.
// Returns the driver, a cleanup function, and any error.
func createDriver(cfg *RunConfig) (core.Driver, func(), error) {
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
		return createAndroidDriver(cfg)
	case "ios":
		return createIOSDriver(cfg)
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
	logger.Info("Appium session created successfully: %s", driver.GetPlatformInfo().DeviceID)
	printSetupSuccess("Appium session created")

	// Cleanup function
	cleanup := func() {
		driver.Close()
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
	ln.Close()
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

// getFirstDevice returns the first device from config, or empty string if none.
func getFirstDevice(cfg *RunConfig) string {
	if len(cfg.Devices) > 0 {
		return cfg.Devices[0]
	}
	return ""
}

// getDriversDir returns the absolute path to the drivers subdirectory.
// This ensures the path works correctly even in parallel goroutines.
func getDriversDir(platform string) (string, error) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	// Build absolute path to drivers directory
	driversPath := filepath.Join(cwd, "drivers", platform)

	// Verify directory exists
	if _, err := os.Stat(driversPath); os.IsNotExist(err) {
		return "", fmt.Errorf("drivers directory not found: %s", driversPath)
	}

	return driversPath, nil
}

// isSocketInUse checks if a Unix socket is in use by attempting to connect to it.
// Used to detect if another maestro-runner instance is using the same Android device.
// On Windows, where sockets aren't used, this always returns false (TCP port check is used instead).
func isSocketInUse(socketPath string) bool {
	if socketPath == "" {
		return false
	}

	// Check if socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return false // socket file doesn't exist, so not in use
	}

	// Try to connect to the socket to verify it's actually active
	conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
	if err != nil {
		// Socket file exists but can't connect - might be stale
		// Try to remove it and consider it not in use
		os.Remove(socketPath)
		return false
	}
	conn.Close()
	return true // socket is active and in use
}

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

	// 1. Validate devices
	if err := validateDevicesAvailable(deviceIDs, platform); err != nil {
		return nil, err
	}

	// 2. Create workers
	workers, err := createDeviceWorkers(cfg, deviceIDs, platform)
	if err != nil {
		return nil, err
	}

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
			driver, cleanup, err = createIOSDriver(&deviceCfg)
		} else {
			deviceCfg.Platform = "android"
			driver, cleanup, err = createAndroidDriver(&deviceCfg)
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

// createParallelRunner builds the parallel runner with config.
func createParallelRunner(cfg *RunConfig, workers []executor.DeviceWorker, platform string) *executor.ParallelRunner {
	driverName := "uiautomator2"
	if platform == "ios" {
		driverName = "wda"
	} else if cfg.Platform == "mock" {
		driverName = "mock"
	}

	firstDriver := workers[0].Driver
	runnerConfig := executor.RunnerConfig{
		OutputDir:   cfg.OutputDir,
		Parallelism: 0,
		Artifacts:   executor.ArtifactOnFailure,
		Device: report.Device{
			ID:          firstDriver.GetPlatformInfo().DeviceID,
			Platform:    firstDriver.GetPlatformInfo().Platform,
			Name:        fmt.Sprintf("%d devices", len(workers)),
			OSVersion:   firstDriver.GetPlatformInfo().OSVersion,
			IsSimulator: firstDriver.GetPlatformInfo().IsSimulator,
		},
		App: report.App{
			ID:      firstDriver.GetPlatformInfo().AppID,
			Version: firstDriver.GetPlatformInfo().AppVersion,
		},
		RunnerVersion:      "0.1.0",
		DriverName:         driverName,
		Env:                cfg.Env,
		WaitForIdleTimeout: cfg.WaitForIdleTimeout,
		// Callbacks will be set per-worker in parallel.go with device info
	}

	return executor.NewParallelRunner(workers, runnerConfig)
}
