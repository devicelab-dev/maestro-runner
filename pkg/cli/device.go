package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/device"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
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
	Description: `Print the view hierarchy of the connected device as a normalized JSON tree.

Output is consistent across drivers (Android, iOS, web) so it can be piped
to jq or diffed between drivers. Use --compact for a flat, greppable view
and --find to filter to elements matching a substring.

Examples:
  maestro-runner hierarchy
  maestro-runner hierarchy --device emulator-5554
  maestro-runner hierarchy --compact
  maestro-runner hierarchy --find "Sign in"`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "compact",
			Usage: "Flat one-line-per-element output instead of a JSON tree",
		},
		&cli.StringFlag{
			Name:  "find",
			Usage: "Only show elements whose type/id/text contains this substring (case-insensitive)",
		},
		&cli.StringFlag{
			Name:  "screenshot",
			Usage: "Also capture a screenshot to this path, from the same session",
		},
		&cli.StringFlag{
			Name:  "format",
			Usage: "Output format: tree (normalized, the default) or maestro (Maestro's `maestro hierarchy` JSON; the default when run as `maestro`)",
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
	runDevice := c.String("device")
	compact := c.Bool("compact")
	find := c.String("find")
	format, err := hierarchyFormat(c.String("format"))
	if err != nil {
		return err
	}

	// Status goes to stderr (logger) so stdout carries only the hierarchy —
	// keeps `maestro-runner hierarchy | jq` / `> tree.json` clean.
	if runDevice != "" {
		logger.Info("Capturing hierarchy from device: %s", runDevice)
	}

	cfg, err := buildDeviceRunConfig(c)
	if err != nil {
		return err
	}

	// A test running on the device answers for it (Maestro allows
	// `maestro hierarchy` next to a running test, and tools call it
	// mid-flow).
	if !strings.EqualFold(cfg.Platform, "ios") && !strings.EqualFold(cfg.Platform, "web") {
		if share := liveHierarchyShare(runDevice); share != "" {
			return hierarchyFromRunningTest(share, c.String("screenshot"), format, compact, find)
		}
	}

	// Driver setup and teardown print progress to stdout; redirect that to
	// stderr so stdout carries only the hierarchy and stays pipe-clean
	// (hierarchy | jq / > tree.json).
	realStdout := os.Stdout
	os.Stdout = os.Stderr

	driver, cleanup, err := CreateDriver(cfg)
	if err != nil {
		os.Stdout = realStdout
		// Surface NoDevicesError directly so its helpful message isn't buried;
		// otherwise wrap. Either way return the error so the command exits
		// non-zero (matches `test`) instead of silently succeeding.
		var noDevErr *device.NoDevicesError
		if errors.As(err, &noDevErr) {
			return noDevErr
		}
		return fmt.Errorf("failed to create driver: %w", err)
	}
	defer func() { os.Stdout = os.Stderr; cleanup(); os.Stdout = realStdout }()

	raw, err := driver.Hierarchy()
	if err != nil {
		os.Stdout = realStdout
		return fmt.Errorf("failed to get hierarchy: %w", err)
	}

	// Capture the screenshot inside the same session, before teardown. Two
	// separate invocations would pay driver startup twice and — worse — could
	// straddle a UI change, leaving the tree and the image disagreeing about
	// what was on screen.
	if shotPath := c.String("screenshot"); shotPath != "" {
		if serr := captureHierarchyScreenshot(driver, shotPath); serr != nil {
			os.Stdout = realStdout
			return serr
		}
		// stderr, so stdout stays pipe-clean for the tree itself.
		logger.Info("Screenshot written to %s", shotPath)
	}
	os.Stdout = realStdout

	// Normalize the driver's platform-specific output (Android/iOS XML or the
	// devicelab JSON) into one consistent tree, or Maestro's, then render.
	return printHierarchy(raw, format, compact, find)
}

// hierarchyFromRunningTest prints the hierarchy (and writes the screenshot)
// that a running test returns for its device.
func hierarchyFromRunningTest(share, shotPath, format string, compact bool, find string) error {
	logger.Info("Device is in use by a running test; asking it for the hierarchy (%s)", share)
	raw, err := askHierarchyShare(share, "hierarchy")
	if err != nil {
		return fmt.Errorf("failed to get hierarchy: %w", err)
	}
	if shotPath != "" {
		data, err := askHierarchyShare(share, "screenshot")
		if err != nil {
			return fmt.Errorf("failed to take screenshot: %w", err)
		}
		if err := writeScreenshotFile(shotPath, data); err != nil {
			return err
		}
		logger.Info("Screenshot written to %s", shotPath)
	}
	return printHierarchy(raw, format, compact, find)
}

// printHierarchy renders raw in the chosen format: --compact and --find are
// the normalized views, and override --format.
func printHierarchy(raw []byte, format string, compact bool, find string) error {
	var out string
	var err error
	if format == "maestro" && !compact && find == "" {
		out, err = formatMaestroHierarchy(raw)
	} else {
		out, err = formatHierarchy(raw, compact, find)
	}
	if err != nil {
		return fmt.Errorf("format hierarchy: %w", err)
	}
	fmt.Println(out)
	return nil
}

// captureHierarchyScreenshot writes a screenshot from an already-open driver
// session to path, creating parent directories as needed.
func captureHierarchyScreenshot(driver core.Driver, path string) error {
	data, err := driver.Screenshot()
	if err != nil {
		return fmt.Errorf("failed to capture screenshot: %w", err)
	}
	return writeScreenshotFile(path, data)
}

// writeScreenshotFile writes screenshot data to path, creating parent
// directories as needed.
func writeScreenshotFile(path string, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("screenshot capture returned no image data")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return fmt.Errorf("create screenshot directory: %w", mkErr)
		}
	}
	if wErr := os.WriteFile(path, data, 0o644); wErr != nil {
		return fmt.Errorf("write screenshot %s: %w", path, wErr)
	}
	return nil
}

// buildDeviceRunConfig assembles the RunConfig for the subcommands that open a
// driver session but do not run a flow — hierarchy, screenshot. Global flags may
// be given before the subcommand or after it, so each is read from the current
// context first and the parent's second.
func buildDeviceRunConfig(c *cli.Context) (*RunConfig, error) {
	getString := func(name string) string { return globalString(c, name) }
	getInt := func(name string) int {
		if c.IsSet(name) {
			return c.Int(name)
		}
		if lineage := c.Lineage(); len(lineage) > 1 && lineage[1] != nil {
			return lineage[1].Int(name)
		}
		return c.Int(name)
	}
	getBool := func(name string) bool {
		if c.IsSet(name) {
			return c.Bool(name)
		}
		if lineage := c.Lineage(); len(lineage) > 1 && lineage[1] != nil {
			return lineage[1].Bool(name)
		}
		return c.Bool(name)
	}

	capsFile := getString("caps")
	var caps map[string]interface{}
	if capsFile != "" {
		var err error
		caps, err = loadCapabilities(capsFile)
		if err != nil {
			return nil, err
		}
	}

	return &RunConfig{
		Headed:             getBool("headed"),
		Browser:            getString("browser"),
		UserDataDir:        getString("user-data-dir"),
		WindowSize:         getString("window-size"),
		Platform:           getString("platform"),
		Devices:            parseDevices(getString("device")),
		Driver:             getString("driver"),
		AppiumURL:          getString("appium-url"),
		AppiumSessionFile:  getString("appium-session-file"),
		CapsFile:           capsFile,
		Capabilities:       caps,
		TeamID:             getString("team-id"),
		WDABundleID:        getString("wda-bundle-id"),
		StartEmulator:      getString("start-emulator"),
		StartSimulator:     getString("start-simulator"),
		AutoStartEmulator:  getBool("auto-start-emulator"),
		BootTimeout:        getInt("boot-timeout"),
		DriverStartTimeout: getInt("driver-start-timeout"),
		NoDriverInstall:    getBool("no-driver-install"),
		NoFlutterFallback:  getBool("no-flutter-fallback"),
		AndroidTCPForward:  getBool("android-tcp-forward"),
	}, nil
}
