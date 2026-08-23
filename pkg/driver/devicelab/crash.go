package devicelab

import (
	"fmt"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// A crash mid-flow used to surface here as a bare "element not found", which
// sends people looking for a selector bug that does not exist. The uiautomator2
// driver has explained this for a while; this brings the DeviceLab driver level,
// because a diagnosis that depends on which driver you picked is worse than none.

// clearExitHistory forgets deaths recorded before now, so a later lookup cannot
// attribute an earlier run's crash — or the force-stop a launch issues itself —
// to this flow. Best-effort: unavailable before API 30, and not worth failing a
// launch over.
func (d *Driver) clearExitHistory(appID string) {
	if d.device == nil || appID == "" {
		return
	}
	if _, err := d.device.Shell("cmd activity clear-exit-info " + core.ShellQuote(appID)); err != nil {
		logger.Debug("could not clear exit-info for %s: %v", appID, err)
	}
}

// appTerminationError returns a descriptive error when the app under test is no
// longer running, and nil when it is alive, unknown, or the device cannot be
// reached — it never manufactures a failure.
func (d *Driver) appTerminationError() error {
	if d.device == nil || d.currentAppID == "" {
		return nil
	}
	// `|| true` keeps pidof's "not found" exit status from looking like a shell
	// failure: a real error means we cannot tell, empty output means gone.
	out, err := d.device.Shell("pidof " + core.ShellQuote(d.currentAppID) + " || true")
	if err != nil {
		return nil
	}
	if strings.TrimSpace(out) != "" {
		return nil // alive
	}

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

// exitExplanation asks the platform why the process went away. This is the only
// host-reachable source of LOW MEMORY — an lmkd kill leaves nothing in logcat.
func (d *Driver) exitExplanation() string {
	if d.device == nil || d.currentAppID == "" {
		return ""
	}
	out, err := d.device.Shell("dumpsys activity exit-info " + core.ShellQuote(d.currentAppID))
	if err != nil {
		return ""
	}
	// A process can die more than once in a flow, and the newest record is not
	// always the most informative.
	if info, ok := core.MostSignificant(core.ParseAndroidExitInfo(out)); ok {
		return info.Summary()
	}
	return ""
}

// notFoundOrCrash returns a termination error when the app died mid-flow,
// otherwise the original lookup error.
func (d *Driver) notFoundOrCrash(orig error) error {
	if termErr := d.appTerminationError(); termErr != nil {
		return termErr
	}
	return orig
}
