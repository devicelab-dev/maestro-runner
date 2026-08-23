package devicelab_ios

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// handleAddMedia adds photos/videos to the device's Photos library.
//
//   - Simulator: `xcrun simctl addmedia` writes real PHAssets host-side.
//   - Real device: there is no host-side path to the Photos DB (Apple
//     restriction), so we stream each file to the on-device runner, which adds
//     it via PhotoKit (PHAssetCreationRequest). This is what lets maestro-runner
//     support addMedia on real iOS devices — which upstream Maestro does not.
func (d *Driver) handleAddMedia(s *flow.AddMediaStep) *core.CommandResult {
	if err := core.ValidateMediaFiles(s.Files); err != nil {
		return core.ErrorResult(err, err.Error())
	}
	for _, f := range s.Files {
		if _, err := os.Stat(f); err != nil {
			return core.ErrorResult(err, fmt.Sprintf("Media file not found: %s", f))
		}
	}

	if d.info != nil && d.info.IsSimulator {
		args := append([]string{"simctl", "addmedia", d.udid}, s.Files...)
		out, err := exec.Command("xcrun", args...).CombinedOutput()
		if err != nil {
			return core.ErrorResult(fmt.Errorf("simctl addmedia failed: %w", err),
				fmt.Sprintf("Failed to add media: %v: %s", err, strings.TrimSpace(string(out))))
		}
		return core.SuccessResult(fmt.Sprintf("Added %d media file(s) to the simulator", len(s.Files)), nil)
	}

	// Real device — add via the on-device runner's PhotoKit path.
	ctx, cancel := d.callTimeout()
	defer cancel()
	for _, f := range s.Files {
		data, err := os.ReadFile(f)
		if err != nil {
			return core.ErrorResult(err, fmt.Sprintf("Failed to read media file %s: %v", f, err))
		}
		mime, _ := core.MediaMIMEType(f)
		if _, err := d.client.Call(ctx, Command{
			Command:   CmdAddMedia,
			MediaName: filepath.Base(f),
			MimeType:  mime,
			MediaData: base64.StdEncoding.EncodeToString(data),
		}); err != nil {
			return core.ErrorResult(err, fmt.Sprintf("Failed to add media %s: %v", filepath.Base(f), err))
		}
	}
	return core.SuccessResult(fmt.Sprintf("Added %d media file(s) via on-device PhotoKit", len(s.Files)), nil)
}
