package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/devicelab-dev/maestro-runner/pkg/device"
	"github.com/urfave/cli/v2"
)

var screenshotCommand = &cli.Command{
	Name:      "screenshot",
	Usage:     "Capture a screenshot of the connected device",
	ArgsUsage: "[output.png]",
	Description: `Capture what is on screen right now and write it to a PNG.

Takes the same device and driver flags as a run, so it works against whatever
` + "`maestro-runner test`" + ` would have targeted. Writes to screenshot.png unless another
path is given; pass - to write the image to stdout for piping.

Examples:
  maestro-runner screenshot
  maestro-runner screenshot login-screen.png
  maestro-runner --device emulator-5554 screenshot
  maestro-runner screenshot - | open -f -a Preview`,
	Action: runScreenshot,
}

func runScreenshot(c *cli.Context) error {
	outPath := c.Args().First()
	if outPath == "" {
		outPath = "screenshot.png"
	}

	cfg, err := buildDeviceRunConfig(c)
	if err != nil {
		return err
	}

	// Driver setup prints progress to stdout; send that to stderr so `screenshot -`
	// can pipe the image itself.
	realStdout := os.Stdout
	os.Stdout = os.Stderr

	driver, cleanup, err := CreateDriver(cfg)
	if err != nil {
		os.Stdout = realStdout
		// Surface NoDevicesError directly — its suggestions are the useful part.
		var noDevErr *device.NoDevicesError
		if errors.As(err, &noDevErr) {
			return noDevErr
		}
		return fmt.Errorf("failed to create driver: %w", err)
	}
	defer func() { os.Stdout = os.Stderr; cleanup(); os.Stdout = realStdout }()

	if outPath == "-" {
		data, shotErr := driver.Screenshot()
		if shotErr != nil {
			os.Stdout = realStdout
			return fmt.Errorf("failed to capture screenshot: %w", shotErr)
		}
		os.Stdout = realStdout
		_, wErr := os.Stdout.Write(data)
		return wErr
	}

	if err := captureHierarchyScreenshot(driver, outPath); err != nil {
		os.Stdout = realStdout
		return err
	}
	os.Stdout = realStdout
	fmt.Fprintf(os.Stderr, "Screenshot written to %s\n", outPath)
	return nil
}
