package uiautomator2

import (
	"fmt"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// --record support (core.ScreenRecorder).
//
// screenrecord writes to the device filesystem and caps a single clip at its
// default three minutes; a longer flow keeps its first three minutes. The file
// is pulled to the host and removed from the device on stop.

const recordingDevicePath = "/sdcard/maestro-runner-recording.mp4"

// StartScreenRecording implements core.ScreenRecorder.
func (d *Driver) StartScreenRecording() error {
	if d.device == nil {
		return fmt.Errorf("screen recording requires device access")
	}
	// A recorder left over from an interrupted run would hold the file open.
	_, _ = d.device.Shell("pkill -INT screenrecord")
	_, _ = d.device.Shell("rm -f " + recordingDevicePath)

	// The stdio redirects are load-bearing: a backgrounded child that inherits
	// the adb shell's stdout either holds the session open or dies with it.
	if _, err := d.device.Shell(fmt.Sprintf("screenrecord %s </dev/null >/dev/null 2>&1 &", recordingDevicePath)); err != nil {
		return fmt.Errorf("start screenrecord: %w", err)
	}
	// screenrecord exits straight away on an unwritable path or unsupported
	// resolution — confirm it is actually running before reporting success.
	time.Sleep(300 * time.Millisecond)
	if out, err := d.device.Shell("pgrep screenrecord"); err != nil || strings.TrimSpace(out) == "" {
		return fmt.Errorf("screenrecord did not start")
	}
	return nil
}

// StopScreenRecording implements core.ScreenRecorder.
func (d *Driver) StopScreenRecording(hostPath string) error {
	if d.device == nil {
		return fmt.Errorf("screen recording requires device access")
	}
	if _, err := d.device.Shell("pkill -INT screenrecord"); err != nil {
		logger.Warn("stop screenrecord: %v", err)
	}
	// The MP4 index is written as the process exits; pulling earlier yields an
	// unplayable file.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := d.device.Shell("pgrep screenrecord")
		if strings.TrimSpace(out) == "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	puller, ok := d.device.(interface {
		Pull(remotePath, localPath string) error
	})
	if !ok {
		return fmt.Errorf("device does not support pulling files")
	}
	if err := puller.Pull(recordingDevicePath, hostPath); err != nil {
		return fmt.Errorf("pull recording: %w", err)
	}
	_, _ = d.device.Shell("rm -f " + recordingDevicePath)
	return nil
}
