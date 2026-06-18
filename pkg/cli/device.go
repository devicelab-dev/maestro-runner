// SPDX-FileCopyrightText: 2025 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package cli

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var startDeviceCommand = &cli.Command{
	Name:  "start-device",
	Usage: "Start or create an iOS Simulator or Android Emulator",
	Description: `Start or create a device similar to ones used in cloud testing.
Requires --platform global flag (before command).

Examples:
  maestro-runner -p ios start-device --os-version 17
  maestro-runner -p android start-device --os-version 33
  maestro-runner -p ios start-device --device-locale de_DE`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "os-version",
			Usage: "OS version (iOS: 16, 17, 18; Android: 28-33)",
		},
		&cli.StringFlag{
			Name:  "device-locale",
			Usage: "Device locale (e.g., de_DE)",
		},
		&cli.BoolFlag{
			Name:  "force-create",
			Usage: "Override existing device",
		},
	},
	Action: runStartDevice,
}

var hierarchyCommand = &cli.Command{
	Name:  "hierarchy",
	Usage: "Print the view hierarchy of the connected device",
	Description: `Print out the view hierarchy of the connected device in JSON or CSV format.

Examples:
  maestro-runner hierarchy
  maestro-runner hierarchy --compact
  maestro-runner hierarchy --device emulator-5554`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "compact",
			Usage: "Output in CSV format",
		},
	},
	Action: runHierarchy,
}

func runStartDevice(c *cli.Context) error {
	platform := c.String("platform") // Global flag
	if platform == "" {
		return fmt.Errorf("--platform is required (ios or android)")
	}

	osVersion := c.String("os-version")
	locale := c.String("device-locale")
	forceCreate := c.Bool("force-create")

	// TODO: Implement device creation
	fmt.Println("Start device command received:")
	fmt.Printf("  Platform: %s\n", platform)
	if osVersion != "" {
		fmt.Printf("  OS Version: %s\n", osVersion)
	}
	if locale != "" {
		fmt.Printf("  Locale: %s\n", locale)
	}
	if forceCreate {
		fmt.Println("  Force create: true")
	}

	fmt.Println("\n[Not yet implemented - will create/start device]")
	return nil
}

func runHierarchy(c *cli.Context) error {
	device := c.String("device")
	compact := c.Bool("compact")

	// TODO: Implement hierarchy dump
	fmt.Println("Hierarchy command received:")
	if device != "" {
		fmt.Printf("  Device: %s\n", device)
	}
	if compact {
		fmt.Println("  Format: CSV")
	} else {
		fmt.Println("  Format: JSON")
	}

	fmt.Println("\n[Not yet implemented - will dump view hierarchy]")
	return nil
}
