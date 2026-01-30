// Package cli provides the command-line interface for maestro-runner.
package cli

import (
	"fmt"
	"os"
	"runtime"

	"github.com/urfave/cli/v2"
)

// Build info — set at build time via -ldflags.
var (
	Version   = "1.0.1"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// GlobalFlags are available to all commands.
var GlobalFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "platform",
		Aliases: []string{"p"},
		Usage:   "Platform to run on (ios, android, web)",
		EnvVars: []string{"MAESTRO_PLATFORM"},
	},
	&cli.StringFlag{
		Name:    "device",
		Aliases: []string{"udid"},
		Usage:   "Device ID to run on (can be comma-separated)",
		EnvVars: []string{"MAESTRO_DEVICE"},
	},
	&cli.StringFlag{
		Name:    "driver",
		Aliases: []string{"d"},
		Usage:   "Driver to use (uiautomator2, appium)",
		Value:   "uiautomator2",
		EnvVars: []string{"MAESTRO_DRIVER"},
	},
	&cli.StringFlag{
		Name:    "appium-url",
		Usage:   "Appium server URL (for appium driver)",
		Value:   "http://127.0.0.1:4723",
		EnvVars: []string{"APPIUM_URL"},
	},
	&cli.StringFlag{
		Name:    "caps",
		Usage:   "Path to Appium capabilities JSON file",
		EnvVars: []string{"APPIUM_CAPS"},
	},
	&cli.BoolFlag{
		Name:    "verbose",
		Usage:   "Enable verbose logging",
		EnvVars: []string{"MAESTRO_VERBOSE"},
	},
	&cli.StringFlag{
		Name:    "app-file",
		Usage:   "App binary (.apk, .app, .ipa) to install before testing",
		EnvVars: []string{"MAESTRO_APP_FILE"},
	},
	&cli.BoolFlag{
		Name:  "no-ansi",
		Usage: "Disable ANSI colors",
	},
	&cli.StringFlag{
		Name:    "team-id",
		Usage:   "Apple Development Team ID for WDA code signing (iOS)",
		EnvVars: []string{"MAESTRO_TEAM_ID", "DEVELOPMENT_TEAM"},
	},
	&cli.StringFlag{
		Name:    "start-emulator",
		Usage:   "Start emulator with AVD name (e.g., Pixel_7_API_33)",
		EnvVars: []string{"MAESTRO_START_EMULATOR"},
	},
	&cli.BoolFlag{
		Name:    "auto-start-emulator",
		Usage:   "Auto-start an emulator if no devices found",
		EnvVars: []string{"MAESTRO_AUTO_START_EMULATOR"},
	},
	&cli.BoolFlag{
		Name:    "shutdown-after",
		Value:   true,
		Usage:   "Shutdown emulators started by maestro-runner after tests",
		EnvVars: []string{"MAESTRO_SHUTDOWN_AFTER"},
	},
	&cli.IntFlag{
		Name:  "boot-timeout",
		Value: 180,
		Usage: "Emulator boot timeout in seconds",
	},
}

// Execute runs the CLI.
func Execute() {
	// Merge global flags and test command flags for root-level execution
	allFlags := append(GlobalFlags, testCommand.Flags...)

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("maestro-runner %s\n", c.App.Version)
		fmt.Printf("  Commit:  %s\n", Commit)
		fmt.Printf("  Built:   %s\n", BuildDate)
		fmt.Printf("  Go:      %s\n", runtime.Version())
		fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	}

	app := &cli.App{
		Name:      "maestro-runner",
		Usage:     "Maestro test runner for mobile and web apps",
		Version:   Version,
		ArgsUsage: "<flow-file-or-folder>...",
		Description: `Maestro Runner executes Maestro flow files for automated testing
of iOS, Android, and web applications.

Examples:
  # Run with default UIAutomator2 driver
  maestro-runner flow.yaml
  maestro-runner flows/ -e USER=test

  # Run with Appium driver
  maestro-runner --driver appium flow.yaml
  maestro-runner --driver appium --caps caps.json flow.yaml

  # Run on cloud providers
  maestro-runner --driver appium --appium-url "https://your-cloud-hub/wd/hub" --caps caps.json flow.yaml

  # Run in parallel on multiple devices
  maestro-runner --platform android --parallel 2 flows/`,
		Flags:  allFlags,
		Action: testCommand.Action,
		// Keep test command for backward compatibility
		Commands: []*cli.Command{
			testCommand,
			wdaCommand,
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
