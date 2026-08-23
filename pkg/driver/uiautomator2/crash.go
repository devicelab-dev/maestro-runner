package uiautomator2

import (
	"fmt"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"

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

	// Process is gone. Explain why, most authoritative source first.
	summary := "app '" + d.currentAppID + "' is no longer running (crashed or was terminated during the flow)"
	if explanation := d.exitExplanation(); explanation != "" {
		summary = explanation
	} else if logcat, lerr := d.device.Shell("logcat -d -b crash -t 400"); lerr == nil {
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

// clearExitHistory forgets process deaths recorded before now, so a later
// lookup cannot attribute an earlier run's crash — or the runner's own
// force-stop — to this flow. Best-effort: the command is unavailable before
// API 30, and failing to clear is not worth failing a launch over.
func (d *Driver) clearExitHistory(appID string) {
	if d.device == nil || appID == "" {
		return
	}
	if _, err := d.device.Shell("cmd activity clear-exit-info " + core.ShellQuote(appID)); err != nil {
		logger.Debug("could not clear exit-info for %s: %v", appID, err)
	}
}

// exitExplanation asks the platform why the app's process went away.
//
// This is the only host-reachable source of LOW MEMORY: an lmkd kill leaves
// nothing in logcat, so without it an app killed for memory is
// indistinguishable from one that merely stopped. It also carries pss and rss
// as measured at the moment of death.
//
// Returns "" when there is nothing noteworthy, which includes the common case
// of the runner having stopped the app itself.
func (d *Driver) exitExplanation() string {
	if d.device == nil || d.currentAppID == "" {
		return ""
	}
	out, err := d.device.Shell("dumpsys activity exit-info " + core.ShellQuote(d.currentAppID))
	if err != nil {
		return ""
	}
	// A process can die more than once in a flow — crash, restart, killed
	// again — and the newest record is not always the most informative.
	if info, ok := core.MostSignificant(core.ParseAndroidExitInfo(out)); ok {
		return info.Summary()
	}
	return ""
}
