package devicelab_ios

import (
	"fmt"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// Airplane mode (#252). iOS has no API for it, so on a real iPhone the
// driver works the switch in Settings, as the WDA driver does
// (pkg/driver/wda/commands.go), with two additions: it reads the switch again
// after the tap and fails the step if it did not move, and it brings the app
// that was in front back with `app activate` (XCUIApplication.activate: a
// running app comes forward as it was, it is not relaunched). Simulators have
// no airplane mode; the step is skipped there, as on WDA.

const (
	settingsBundle = "com.apple.Preferences"
	// airplaneModeID is the identifier of the Airplane Mode row's switch.
	airplaneModeID    = "com.apple.settings.airplaneMode"
	airplaneModeLabel = "Airplane Mode"
)

// Budgets for finding the switch after Settings comes forward and for the
// switch to show the new state after the tap; tests shorten them.
var (
	airplaneFindTimeout   = 6 * time.Second
	airplaneVerifyTimeout = 3 * time.Second
	airplanePoll          = 250 * time.Millisecond
)

// setAirplaneMode turns airplane mode on or off.
func (d *Driver) setAirplaneMode(enabled bool) *core.CommandResult {
	return d.airplaneMode("setAirplaneMode", &enabled)
}

// toggleAirplaneMode flips airplane mode.
func (d *Driver) toggleAirplaneMode() *core.CommandResult {
	return d.airplaneMode("toggleAirplaneMode", nil)
}

// airplaneMode sets the switch to *want, or flips it when want is nil.
func (d *Driver) airplaneMode(command string, want *bool) *core.CommandResult {
	if !d.realDevice {
		return core.SuccessResult(command+": simulators don't have airplane mode (skipped)", nil)
	}
	front := d.frontmostApp()
	res := d.workAirplaneSwitch(want)
	if err := d.returnToApp(front); err != nil {
		msg := fmt.Sprintf("%s; then bringing %s back to the front failed: %v", res.Message, front, err)
		if !res.Success {
			return core.ErrorResult(res.Error, msg)
		}
		return core.ErrorResult(err, msg)
	}
	return res
}

// workAirplaneSwitch opens Settings, reads the switch, taps it when it is not
// in the wanted state and waits for it to show that state.
func (d *Driver) workAirplaneSwitch(want *bool) *core.CommandResult {
	if _, err := d.call("app", &Args{Action: "activate", BundleID: settingsBundle}); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("Failed to open Settings: %v", err))
	}
	sw, on, err := d.findAirplaneSwitch(airplaneFindTimeout)
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("Failed to find the Airplane Mode switch in Settings: %v", err))
	}
	target := !on
	if want != nil {
		target = *want
	}
	if want != nil && on == target {
		return core.SuccessResult("Airplane mode already "+onOff(target), nil)
	}
	x, y := airplaneTapPoint(sw)
	if err := d.act("tap", x, y, nil); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("Failed to tap the Airplane Mode switch: %v", err))
	}
	if err := d.waitAirplaneState(target); err != nil {
		return core.ErrorResult(err, err.Error())
	}
	if want == nil {
		return core.SuccessResult("Toggled airplane mode ("+onOff(target)+")", nil)
	}
	return core.SuccessResult("Airplane mode "+onOff(target), nil)
}

// findAirplaneSwitch polls Settings for the switch until timeout and returns
// it with its state.
func (d *Driver) findAirplaneSwitch(timeout time.Duration) (Node, bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		sw, on, err := d.readAirplaneSwitch()
		if err == nil || time.Now().After(deadline) || d.context().Err() != nil {
			return sw, on, err
		}
		logger.Debug("[devicelab-ios] airplane mode switch: %v", err)
		time.Sleep(airplanePoll)
	}
}

// readAirplaneSwitch looks at Settings once.
func (d *Driver) readAirplaneSwitch() (Node, bool, error) {
	resp, err := d.call("snapshot", &Args{App: settingsBundle, VisibleOnly: true})
	if err != nil {
		return Node{}, false, err
	}
	for _, n := range resp.payload().Nodes {
		if n.Type != "Switch" || (n.ID != airplaneModeID && n.Label != airplaneModeLabel) {
			continue
		}
		on, err := switchOn(n.Value)
		return n, on, err
	}
	return Node{}, false, fmt.Errorf("no switch %q on the Settings screen", airplaneModeLabel)
}

// waitAirplaneState waits for the switch to show want.
func (d *Driver) waitAirplaneState(want bool) error {
	_, on, err := d.readAirplaneSwitch()
	deadline := time.Now().Add(airplaneVerifyTimeout)
	for (err != nil || on != want) && time.Now().Before(deadline) && d.context().Err() == nil {
		time.Sleep(airplanePoll)
		_, on, err = d.readAirplaneSwitch()
	}
	if err != nil {
		return fmt.Errorf("could not read the Airplane Mode switch after tapping it: %w", err)
	}
	if on != want {
		return fmt.Errorf("tapped the Airplane Mode switch but airplane mode is still %s", onOff(on))
	}
	return nil
}

// frontmostApp is the app in front before Settings is opened: the agent's
// frontmost bundle, else the app under test. SpringBoard and Settings itself
// are nothing to return to.
func (d *Driver) frontmostApp() string {
	front := ""
	if resp, err := d.call("snapshot", &Args{MaxNodes: 1}); err == nil {
		front = resp.payload().BundleID
	}
	switch front {
	case "":
		return d.appID
	case springboardBundle, settingsBundle:
		return ""
	}
	return front
}

// returnToApp brings bid back to the front without relaunching it.
func (d *Driver) returnToApp(bid string) error {
	if bid == "" {
		return nil
	}
	_, err := d.call("app", &Args{Action: "activate", BundleID: bid})
	return err
}

// airplaneTapPoint is on the toggle at the right end of the row: the switch's
// frame can be the whole row, and a tap on its label does not flip it.
func airplaneTapPoint(n Node) (float64, float64) {
	x := n.X + n.W/2
	if n.W > 80 {
		x = n.X + n.W - 30
	}
	return x, n.Y + n.H/2
}

// switchOn reads a switch's value.
func switchOn(value string) (bool, error) {
	switch value {
	case "1", "on", "true":
		return true, nil
	case "0", "off", "false":
		return false, nil
	}
	return false, fmt.Errorf("unexpected Airplane Mode switch value %q", value)
}

func onOff(on bool) string {
	if on {
		return "enabled"
	}
	return "disabled"
}
