package wda

import (
	"fmt"
	"os"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// addMedia adds photos/videos to the target's Photos library.
//
// On the WDA driver this is simulator-only: `xcrun simctl addmedia` writes real
// PHAssets the app's picker can select. Real iOS devices have no host-side path
// to the Photos database (Apple restriction) — use the `devicelab` iOS driver,
// whose on-device runner adds media via PhotoKit. (#131)
func (d *Driver) addMedia(step *flow.AddMediaStep) *core.CommandResult {
	if err := core.ValidateMediaFiles(step.Files); err != nil {
		return errorResult(err, err.Error())
	}
	for _, f := range step.Files {
		if _, err := os.Stat(f); err != nil {
			return errorResult(err, fmt.Sprintf("Media file not found: %s", f))
		}
	}

	if d.info == nil || !d.info.IsSimulator {
		err := fmt.Errorf("addMedia is only supported on iOS simulators with the wda driver; " +
			"for real iOS devices use --driver devicelab (adds media on-device via PhotoKit)")
		return errorResult(err, err.Error())
	}

	args := append([]string{"simctl", "addmedia", d.udid}, step.Files...)
	out, err := execCommand("xcrun", args...).CombinedOutput()
	if err != nil {
		return errorResult(fmt.Errorf("simctl addmedia failed: %w", err),
			fmt.Sprintf("Failed to add media: %v: %s", err, string(out)))
	}
	return successResult(fmt.Sprintf("Added %d media file(s) to the simulator", len(step.Files)), nil)
}
