package devicelab_ios

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// On a physical iPhone there is no simctl: the agent launches, stops and
// opens URLs (XCUIApplication, XCUIDevice), devicectl reinstalls for
// clearState, and what only a simulator can do is reported as unavailable.

// devicectlTimeout bounds one devicectl install or uninstall.
const devicectlTimeout = 5 * time.Minute

// runDevicectl runs `xcrun devicectl args…`; tests replace it.
var runDevicectl = func(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), devicectlTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xcrun", append([]string{"devicectl"}, args...)...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("devicectl %s: %v: %s", strings.Join(args[:min(3, len(args))], " "), err, lastLines(string(out), 5))
	}
	return string(out), nil
}

// SetRealDevice switches the driver to a physical iPhone. appFile is the
// --app-file clearState reinstalls from (the installed bundle cannot be read
// back from a device); empty makes clearState fail with that explanation.
func (d *Driver) SetRealDevice(appFile string) {
	d.realDevice = true
	d.appFile = appFile
	// The WebKit inspector check reads the simulator's socket.
	d.openWeb = nil
}

func errOnDevice(what string) error {
	return fmt.Errorf("%s is not available on a real iPhone with --driver devicelab", what)
}

// launchAppOnDevice follows launchApp's order with what a device allows:
// clear state, stop, launch through the agent with arguments and
// environment. Permissions and the keychain have no host-side control on a
// device, so a flow that sets them gets a warning, not a failure.
func (d *Driver) launchAppOnDevice(s *flow.LaunchAppStep, bid string) *core.CommandResult {
	if s.ClearState {
		if res := d.clearStateOnDevice(bid); !res.Success {
			return res
		}
	}
	if len(s.Permissions) > 0 {
		logger.Warn("launchApp: permissions are not applied on a real iPhone; the app asks for them at runtime")
	}
	if s.ClearKeychain {
		logger.Warn("launchApp: clearKeychain skipped: %v", errOnDevice("clearKeychain"))
	}
	args := &Args{Action: "launch", BundleID: bid, Arguments: flattenArguments(s.Arguments), Environment: s.Environment}
	if keepsRunningApp(s) {
		// XCTest's launch restarts a running app; `stopApp: false` brings it
		// to the front instead, as Maestro does.
		if d.appRunningOnDevice(bid) {
			args = &Args{Action: "activate", BundleID: bid}
		}
	} else {
		_ = d.terminateOnDevice(bid)
	}
	if _, err := d.call("app", args); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("launchApp failed: %v", err))
	}
	return core.SuccessResult("launched "+bid, nil)
}

// appRunningOnDevice reports whether the agent sees the app running, in front
// or in the background. An unanswered read counts as not running.
func (d *Driver) appRunningOnDevice(bid string) bool {
	resp, err := d.call("app", &Args{Action: "state", BundleID: bid})
	if err != nil {
		return false
	}
	switch resp.payload().AppState {
	case "foreground", "background":
		return true
	}
	return false
}

// terminateOnDevice stops an app through the agent; one that is not running
// counts as stopped.
func (d *Driver) terminateOnDevice(bid string) error {
	_, err := d.call("app", &Args{Action: "terminate", BundleID: bid})
	return err
}

// clearStateOnDevice uninstalls the app and installs --app-file again, as
// the WDA driver does on a device.
func (d *Driver) clearStateOnDevice(bid string) *core.CommandResult {
	if d.appFile == "" {
		err := fmt.Errorf("clearState on real iOS devices requires --app-file")
		return core.ErrorResult(err, "clearState on real iOS devices requires --app-file — the installed bundle "+
			"isn't reachable from the host. On simulators no flag is needed.\n"+
			"Usage: maestro-runner --app-file <path-to-ipa-or-app> --platform ios test <flow-files>")
	}
	_ = d.terminateOnDevice(bid)
	if _, err := runDevicectl("device", "uninstall", "app", "--device", d.udid, bid); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("clearState failed: uninstall: %v", err))
	}
	if _, err := runDevicectl("device", "install", "app", "--device", d.udid, d.appFile); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("clearState failed: reinstall: %v", err))
	}
	return core.SuccessResult("cleared state for "+bid, nil)
}
