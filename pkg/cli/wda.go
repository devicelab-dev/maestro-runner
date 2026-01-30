package cli

import (
	"fmt"
	"os"
	"path/filepath"

	wdadriver "github.com/devicelab-dev/maestro-runner/pkg/driver/wda"
	"github.com/urfave/cli/v2"
)

var wdaCommand = &cli.Command{
	Name:  "wda",
	Usage: "Manage WebDriverAgent for iOS testing",
	Description: `Manage the bundled WebDriverAgent used for iOS device automation.

Examples:
  # Show current WDA version
  maestro-runner wda version

  # Check if a newer version is available
  maestro-runner wda update --check

  # Update to the latest version
  maestro-runner wda update

  # Update to a specific version
  maestro-runner wda update 11.0.0`,
	Subcommands: []*cli.Command{
		{
			Name:  "version",
			Usage: "Show current WebDriverAgent version",
			Action: func(c *cli.Context) error {
				version, err := wdadriver.GetLocalWDAVersion()
				if err != nil {
					if !wdadriver.IsWDAInstalled() {
						fmt.Println("WebDriverAgent is not installed")
						return nil
					}
					return fmt.Errorf("failed to get version: %w", err)
				}
				fmt.Printf("WebDriverAgent v%s\n", version)
				return nil
			},
		},
		{
			Name:      "update",
			Usage:     "Update WebDriverAgent to latest or specific version",
			ArgsUsage: "[version]",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:  "check",
					Usage: "Only check if update is available, don't install",
				},
			},
			Action: func(c *cli.Context) error {
				// --check flag: only check for updates
				if c.Bool("check") {
					return checkWDAUpdate()
				}

				// If version argument provided, download that specific version
				if c.NArg() > 0 {
					version := c.Args().First()
					return updateWDAToVersion(version)
				}

				// No argument: update to latest
				return updateWDAToLatest()
			},
		},
	},
	// Default action when running 'maestro-runner wda' without subcommand
	Action: func(c *cli.Context) error {
		return cli.ShowSubcommandHelp(c)
	},
}

// checkWDAUpdate checks if a newer WDA version is available.
func checkWDAUpdate() error {
	fmt.Println("Checking for updates...")

	local, latest, updateAvailable, err := wdadriver.CheckWDAUpdate()
	if err != nil {
		return err
	}

	fmt.Printf("Current version: %s\n", local)
	fmt.Printf("Latest version:  %s\n", latest)

	if updateAvailable {
		fmt.Printf("\n%sUpdate available!%s Run 'maestro-runner wda update' to update.\n",
			color(colorGreen), color(colorReset))
	} else {
		fmt.Println("\nYou're up to date!")
	}

	return nil
}

// updateWDAToLatest updates WDA to the latest version.
func updateWDAToLatest() error {
	// Check current version first
	local, err := wdadriver.GetLocalWDAVersion()
	if err != nil && wdadriver.IsWDAInstalled() {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	latest, err := wdadriver.GetLatestWDAVersion()
	if err != nil {
		return fmt.Errorf("failed to get latest version: %w", err)
	}

	if local == latest {
		fmt.Printf("Already at latest version (v%s)\n", latest)
		return nil
	}

	if local != "" {
		fmt.Printf("Updating from v%s to v%s...\n", local, latest)
	}

	if err := wdadriver.UpdateWDA(); err != nil {
		return err
	}

	return clearWDACache()
}

// updateWDAToVersion updates WDA to a specific version.
func updateWDAToVersion(version string) error {
	local, err := wdadriver.GetLocalWDAVersion()
	if err == nil && local == version {
		fmt.Printf("Already at version v%s\n", version)
		return nil
	}

	if local != "" {
		fmt.Printf("Updating from v%s to v%s...\n", local, version)
	} else {
		fmt.Printf("Installing WebDriverAgent v%s...\n", version)
	}

	if err := wdadriver.DownloadWDA(version); err != nil {
		return err
	}

	return clearWDACache()
}

// clearWDACache clears the WDA build cache after an update.
func clearWDACache() error {
	fmt.Println("\nClearing WDA build cache...")
	if err := clearWDABuildCache(); err != nil {
		fmt.Printf("Warning: failed to clear build cache: %v\n", err)
		fmt.Println("You may need to manually clear ~/.maestro-runner/cache/wda-builds/")
	} else {
		fmt.Println("Build cache cleared")
	}
	return nil
}

// clearWDABuildCache removes cached WDA builds so they get rebuilt with the new version.
func clearWDABuildCache() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cachePath := filepath.Join(home, ".maestro-runner", "cache", "wda-builds")

	// Check if cache exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return nil // No cache to clear
	}

	return os.RemoveAll(cachePath)
}
