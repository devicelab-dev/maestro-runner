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
	"github.com/devicelab-dev/maestro-runner/pkg/simulator"
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

	media, documents := core.SplitMediaDocuments(s.Files)

	if d.info != nil && d.info.IsSimulator {
		if len(media) > 0 {
			args := append([]string{"simctl", "addmedia", d.udid}, media...)
			out, err := exec.Command("xcrun", args...).CombinedOutput()
			if err != nil {
				return core.ErrorResult(fmt.Errorf("simctl addmedia failed: %w", err),
					fmt.Sprintf("Failed to add media: %v: %s", err, strings.TrimSpace(string(out))))
			}
		}
		// Documents go to the simulator's "On My iPhone" storage, which every
		// app's document picker browses (#167).
		if len(documents) > 0 {
			if err := simulator.AddDocuments(d.udid, documents); err != nil {
				return core.ErrorResult(err, fmt.Sprintf("Failed to add documents: %v", err))
			}
		}
		return core.SuccessResult(fmt.Sprintf("Added %d media file(s) to the simulator", len(s.Files)), nil)
	}

	// Real device — PhotoKit reaches the Photos library and nothing else.
	// There is no host-side or on-device path into Files storage for a
	// document, so say so rather than fail inside the runner.
	if len(documents) > 0 {
		err := fmt.Errorf("documents cannot be added to a physical iPhone: PhotoKit covers photos and videos only, " +
			"and Apple exposes no path into Files storage; addMedia supports documents on the iOS simulator only")
		return core.ErrorResult(err, err.Error())
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
