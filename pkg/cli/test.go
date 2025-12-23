package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/driver/mock"
	"github.com/devicelab-dev/maestro-runner/pkg/executor"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
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
  maestro-runner test flow.yaml
  maestro-runner test flows/
  maestro-runner test login.yaml checkout.yaml
  maestro-runner test flows/ -e USER=test -e PASS=secret
  maestro-runner test flows/ --include-tags smoke
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
			Name:  "shard-split",
			Usage: "Split tests across N devices",
		},
		&cli.IntFlag{
			Name:  "shard-all",
			Usage: "Run all tests on N devices",
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
	ShardSplit int
	ShardAll   int

	// Execution
	Continuous bool
	Headless   bool

	// Device
	Platform string
	Device   string
	Verbose  bool
	AppFile  string
}

func runTest(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("at least one flow file or folder is required")
	}

	// Parse environment variables
	env := parseEnvVars(c.StringSlice("env"))

	// Resolve output directory
	outputDir, err := resolveOutputDir(c.String("output"), c.Bool("flatten"))
	if err != nil {
		return err
	}

	// Build run configuration
	cfg := &RunConfig{
		FlowPaths:   c.Args().Slice(),
		ConfigPath:  c.String("config"),
		Env:         env,
		IncludeTags: c.StringSlice("include-tags"),
		ExcludeTags: c.StringSlice("exclude-tags"),
		OutputDir:   outputDir,
		ShardSplit:  c.Int("shard-split"),
		ShardAll:    c.Int("shard-all"),
		Continuous:  c.Bool("continuous"),
		Headless:    c.Bool("headless"),
		Platform:    c.String("platform"),
		Device:      c.String("device"),
		Verbose:     c.Bool("verbose"),
		AppFile:     c.String("app-file"),
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
	// 1. Validate all flows
	v := validator.New(cfg.IncludeTags, cfg.ExcludeTags)
	var allTestCases []string
	var allErrors []error

	for _, path := range cfg.FlowPaths {
		result := v.Validate(path)
		allTestCases = append(allTestCases, result.TestCases...)
		allErrors = append(allErrors, result.Errors...)
	}

	// Report validation errors
	if len(allErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Validation errors:\n")
		for _, err := range allErrors {
			fmt.Fprintf(os.Stderr, "  - %v\n", err)
		}
		return fmt.Errorf("validation failed with %d error(s)", len(allErrors))
	}

	if len(allTestCases) == 0 {
		return fmt.Errorf("no test flows found")
	}

	fmt.Printf("Found %d test flow(s)\n", len(allTestCases))

	// 2. Parse all validated flows
	var flows []flow.Flow
	for _, path := range allTestCases {
		f, err := flow.ParseFile(path)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}
		flows = append(flows, *f)
	}

	// 3. Create driver (mock for now)
	driver := mock.New(mock.Config{
		Platform: cfg.Platform,
		DeviceID: cfg.Device,
	})

	// 4. Create and run executor
	runner := executor.New(driver, executor.RunnerConfig{
		OutputDir:   cfg.OutputDir,
		Parallelism: 0, // Sequential for now
		Artifacts:   executor.ArtifactOnFailure,
		Device: report.Device{
			ID:       driver.GetPlatformInfo().DeviceID,
			Platform: driver.GetPlatformInfo().Platform,
			Name:     driver.GetPlatformInfo().DeviceName,
		},
		App: report.App{
			ID: cfg.AppFile,
		},
		RunnerVersion: "0.1.0",
		DriverName:    "mock",
	})

	fmt.Printf("Running tests...\n")
	fmt.Printf("Output: %s\n\n", cfg.OutputDir)

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// 5. Print summary
	printSummary(result)

	// Return error if any flows failed
	if result.Status != report.StatusPassed {
		return fmt.Errorf("test run failed: %d/%d flows failed", result.FailedFlows, result.TotalFlows)
	}

	return nil
}

func printSummary(result *executor.RunResult) {
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println("TEST RESULTS")
	fmt.Println("=" + strings.Repeat("=", 59))

	// Print each flow result
	for _, fr := range result.FlowResults {
		status := "PASS"
		if fr.Status == report.StatusFailed {
			status = "FAIL"
		} else if fr.Status == report.StatusSkipped {
			status = "SKIP"
		}
		fmt.Printf("  [%s] %s (%dms)\n", status, fr.Name, fr.Duration)
		if fr.Error != "" {
			fmt.Printf("         Error: %s\n", fr.Error)
		}
	}

	fmt.Println("-" + strings.Repeat("-", 59))

	// Print totals
	fmt.Printf("Total:  %d flows\n", result.TotalFlows)
	fmt.Printf("Passed: %d\n", result.PassedFlows)
	fmt.Printf("Failed: %d\n", result.FailedFlows)
	if result.SkippedFlows > 0 {
		fmt.Printf("Skipped: %d\n", result.SkippedFlows)
	}
	fmt.Printf("Duration: %dms\n", result.Duration)
	fmt.Println("=" + strings.Repeat("=", 59))

	// Overall status
	if result.Status == report.StatusPassed {
		fmt.Println("Status: PASSED")
	} else {
		fmt.Println("Status: FAILED")
	}
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
