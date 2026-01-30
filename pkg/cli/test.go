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
	"github.com/devicelab-dev/maestro-runner/pkg/device"
	appiumdriver "github.com/devicelab-dev/maestro-runner/pkg/driver/appium"
	"github.com/devicelab-dev/maestro-runner/pkg/driver/mock"
	uia2driver "github.com/devicelab-dev/maestro-runner/pkg/driver/uiautomator2"
	wdadriver "github.com/devicelab-dev/maestro-runner/pkg/driver/wda"
	"github.com/devicelab-dev/maestro-runner/pkg/executor"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
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

	// 4. Determine execution mode and devices
	needsParallel, deviceIDs, err := determineExecutionMode(cfg)
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
func determineExecutionMode(cfg *RunConfig) (needsParallel bool, deviceIDs []string, err error) {
	needsParallel = cfg.Parallel > 0 || len(cfg.Devices) > 1

	if needsParallel {
		if len(cfg.Devices) > 0 {
			deviceIDs = cfg.Devices
		} else if cfg.Parallel > 0 {
			deviceIDs, err = autoDetectDevices(cfg.Platform, cfg.Parallel)
			if err != nil {
				return false, nil, fmt.Errorf("failed to auto-detect devices: %w", err)
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
		Device: *deviceInfo,
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
