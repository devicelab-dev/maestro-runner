// Package cli provides the command-line interface for maestro-runner.
package cli

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

// Version is set at build time.
var Version = "dev"

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
	&cli.IntFlag{
		Name:  "driver-host-port",
		Usage: "AndroidDriver host port",
		Value: 7001,
	},
	&cli.BoolFlag{
		Name:  "no-ansi",
		Usage: "Disable ANSI colors",
	},
}

// Execute runs the CLI.
func Execute() {
	app := &cli.App{
		Name:    "maestro-runner",
		Usage:   "Maestro test runner for mobile and web apps",
		Version: Version,
		Description: `Maestro Runner executes Maestro flow files for automated testing
of iOS, Android, and web applications.

Examples:
  maestro-runner test flow.yaml
  maestro-runner test flows/ -e USER=test
  maestro-runner start-device --platform ios`,
		Flags: GlobalFlags,
		Commands: []*cli.Command{
			testCommand,
			startDeviceCommand,
			hierarchyCommand,
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
