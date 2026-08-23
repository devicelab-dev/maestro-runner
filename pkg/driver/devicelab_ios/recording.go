package devicelab_ios

import (
	"fmt"

	"github.com/devicelab-dev/maestro-runner/pkg/simulator"
)

// --record support (core.ScreenRecorder). Capture happens host-side via
// `simctl io recordVideo`, so it is simulator-only: the on-device runner has
// no screen-capture command, and physical devices have no host-side path.

// StartScreenRecording implements core.ScreenRecorder.
func (d *Driver) StartScreenRecording() error {
	if d.info == nil || !d.info.IsSimulator {
		return fmt.Errorf("screen recording is supported on simulators only — physical iOS devices have no host-side capture path")
	}
	if d.recording != nil {
		return fmt.Errorf("a recording is already in progress")
	}
	rec, err := simulator.StartRecording(d.udid)
	if err != nil {
		return err
	}
	d.recording = rec
	return nil
}

// StopScreenRecording implements core.ScreenRecorder.
func (d *Driver) StopScreenRecording(hostPath string) error {
	if d.recording == nil {
		return fmt.Errorf("no recording in progress")
	}
	rec := d.recording
	d.recording = nil
	return rec.Stop(hostPath)
}
