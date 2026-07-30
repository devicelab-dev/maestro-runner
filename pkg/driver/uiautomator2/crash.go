package uiautomator2

import (
	"fmt"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// appTerminationError returns a descriptive error when the app-under-test's
// process is no longer running — i.e. it crashed or was killed mid-flow. It
// returns nil when the app is alive, when there's no known app id, or when the
// device isn't available (never manufactures a failure). Callers use it to turn
// a post-crash "element not found" into a clear "app crashed" message.
//
// Cheap and only meant to run on a step that already failed: one `pidof` check,
// and a logcat read only when the process is confirmed gone.
func (d *Driver) appTerminationError() error {
	if d.device == nil || d.currentAppID == "" {
		return nil
	}
	// `|| true` forces exit 0 so pidof's own "not found" exit status (1) doesn't
	// look like a shell failure — that way a non-nil err means a genuine device/
	// adb problem (return nil, can't tell) while empty output means the process
	// is simply gone.
	out, err := d.device.Shell("pidof " + d.currentAppID + " || true")
	if err != nil {
		return nil // genuine device/shell failure — don't invent a crash
	}
	if strings.TrimSpace(out) != "" {
		return nil // process alive
	}

	// Process is gone. Look for a crash/ANR in the crash buffer to explain why.
	summary := "app '" + d.currentAppID + "' is no longer running (crashed or was terminated during the flow)"
	if logcat, lerr := d.device.Shell("logcat -d -b crash -t 400"); lerr == nil {
		if s, found := core.AndroidCrashSummary(logcat, d.currentAppID); found {
			summary = d.currentAppID + ": " + s
		}
	}
	return fmt.Errorf("%s", summary)
}

// notFoundOrCrash returns a crash/termination error when the app died mid-flow,
// otherwise the original "not found" error. Wrap a required-element lookup
// failure with this so a crash surfaces as "app crashed" rather than a
// misleading "element not found".
func (d *Driver) notFoundOrCrash(orig error) error {
	if termErr := d.appTerminationError(); termErr != nil {
		return termErr
	}
	return orig
}
