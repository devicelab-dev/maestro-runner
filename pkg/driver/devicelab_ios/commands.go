package devicelab_ios

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// executeStep dispatches a maestro flow step to the right handler. The
// handlers translate maestro semantics into agent-device wire commands.
// Steps not covered fall through to an "unsupported" error so the flow
// runner surfaces the gap clearly.
func (d *Driver) executeStep(step flow.Step) *core.CommandResult {
	// Snapshot cache must be invalidated BEFORE any step that mutates
	// the screen — tap/type/swipe/back/hide-keyboard/erase-text/press-key
	// /launchApp/stopApp. Read-only steps (assertVisible, waitUntil,
	// waitForAnimation, takeScreenshot) can keep the cache.
	switch step.(type) {
	case *flow.LaunchAppStep, *flow.StopAppStep, *flow.TapOnStep,
		*flow.InputTextStep, *flow.PressKeyStep, *flow.EraseTextStep,
		*flow.BackStep, *flow.HideKeyboardStep,
		*flow.SwipeStep, *flow.ScrollStep, *flow.ScrollUntilVisibleStep,
		*flow.DoubleTapOnStep, *flow.LongPressOnStep:
		d.invalidateSnapshotCache()
	}
	switch s := step.(type) {
	case *flow.LaunchAppStep:
		return d.handleLaunchApp(s)
	case *flow.StopAppStep:
		return d.handleStopApp(s)
	case *flow.TapOnStep:
		return d.handleTapOn(s)
	case *flow.InputTextStep:
		return d.handleInputText(s)
	case *flow.AssertVisibleStep:
		return d.handleAssertVisible(s)
	case *flow.AssertNotVisibleStep:
		return d.handleAssertNotVisible(s)
	case *flow.TakeScreenshotStep:
		return d.handleTakeScreenshot(s)
	case *flow.PressKeyStep:
		return d.handlePressKey(s)
	case *flow.WaitForAnimationToEndStep:
		return d.handleWaitForAnimation(s)
	case *flow.EraseTextStep:
		return d.handleEraseText(s)
	case *flow.WaitUntilStep:
		return d.handleWaitUntil(s)
	case *flow.BackStep:
		return d.handleBack(s)
	case *flow.HideKeyboardStep:
		return d.handleHideKeyboard(s)
	case *flow.SwipeStep:
		return d.handleSwipe(s)
	case *flow.ScrollStep:
		return d.handleScroll(s)
	case *flow.ScrollUntilVisibleStep:
		return d.handleScrollUntilVisible(s)
	case *flow.DoubleTapOnStep:
		return d.handleDoubleTap(s)
	case *flow.LongPressOnStep:
		return d.handleLongPress(s)
	case *flow.OpenLinkStep:
		return d.handleOpenLink(s)
	case *flow.SetLocationStep:
		return d.handleSetLocation(s)
	default:
		return core.ErrorResult(
			fmt.Errorf("step %T not implemented in devicelab_ios driver yet", step),
			fmt.Sprintf("step %T not supported", step),
		)
	}
}

// ---------- lifecycle handlers ----------

// handleLaunchApp launches via `xcrun simctl launch` and remembers the
// bundle id. Agent-device's runner doesn't have an explicit launch command;
// it activates the bundle implicitly the first time it sees `appBundleId`
// on a subsequent command.
func (d *Driver) handleLaunchApp(s *flow.LaunchAppStep) *core.CommandResult {
	bid := strings.TrimSpace(s.AppID)
	if bid == "" {
		bid = d.appID
	}
	if bid == "" {
		return core.ErrorResult(fmt.Errorf("launchApp requires appId"), "missing appId")
	}
	d.appID = bid

	if s.StopApp != nil && *s.StopApp {
		_ = exec.Command("xcrun", "simctl", "terminate", d.udid, bid).Run()
	}

	args := []string{"simctl", "launch", "--terminate-running-process", d.udid, bid}
	args = append(args, flattenArguments(s.Arguments)...)
	cmd := exec.Command("xcrun", args...)
	for k, v := range s.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf("SIMCTL_CHILD_%s=%s", k, v))
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("simctl launch failed: %s", strings.TrimSpace(string(out))))
	}
	return core.SuccessResult(fmt.Sprintf("launched %s", bid), nil)
}

// handleStopApp terminates via simctl. Treat "not running" as success.
func (d *Driver) handleStopApp(s *flow.StopAppStep) *core.CommandResult {
	bid := d.appID
	if bid == "" {
		return core.ErrorResult(fmt.Errorf("stopApp requires an active app"), "no active app")
	}
	out, err := exec.Command("xcrun", "simctl", "terminate", d.udid, bid).CombinedOutput()
	if err != nil {
		body := strings.ToLower(strings.TrimSpace(string(out)))
		if !strings.Contains(body, "found nothing") && !strings.Contains(body, "no such process") {
			return core.ErrorResult(err, fmt.Sprintf("simctl terminate failed: %s", strings.TrimSpace(string(out))))
		}
	}
	return core.SuccessResult(fmt.Sprintf("stopped %s", bid), nil)
}

// ---------- interaction handlers ----------

// handleTapOn resolves the maestro selector locally via snapshot+filter
// (so maestro's full selector richness — regex, state, substring — works),
// then issues a coordinate tap. This matches agent-device's pattern: the
// `tap` command takes either selectorKey/selectorValue or x/y, and the
// coordinate path is faster + avoids re-resolving on the runner side.
func (d *Driver) handleTapOn(s *flow.TapOnStep) *core.CommandResult {
	if s.Point != "" {
		return core.ErrorResult(fmt.Errorf("tapOn with point not yet implemented"), "point-tap unsupported")
	}

	// Fast path: for simple id-or-text selectors, ask the runner to find
	// and tap atomically. Walks the full XCUI tree once, applies the same
	// liberal matching + prefer-editable-inputs ranking as the Go-side does,
	// taps at the matched element's center. On miss the runner returns the
	// walked snapshot so we fall through to the existing snapshot+filter
	// path below (which handles complex selectors: relative, indexed, etc.).
	if key, value, ok := simpleSelectorKeyValue(s.Selector); ok {
		if hit, err := d.tryTapBySelector(key, value); err == nil && hit != nil {
			d.lastTappedIdentifier = hit.Identifier
			d.lastTappedX, d.lastTappedY = hit.X, hit.Y
			d.lastTapHasCoords = true
			return core.SuccessResult(fmt.Sprintf("tapped: %s", describeSelector(s.Selector)), nil)
		}
		// Fall through to snapshot path on miss/error.
	}

	node, err := d.findElement(s.Selector, s.IsOptional(), s.TimeoutMs)
	if err != nil {
		return core.ErrorResult(err, "tapOn: "+err.Error())
	}
	if node == nil {
		return core.ErrorResult(fmt.Errorf("element not found"), "element not found")
	}
	cx, cy := centerOf(node)
	ctx, cancel := d.callTimeout()
	defer cancel()
	if _, err := d.client.Call(ctx, Command{
		Command:     CmdTap,
		AppBundleID: d.appID,
		X:           ptrFloat(cx),
		Y:           ptrFloat(cy),
	}); err != nil {
		return core.ErrorResult(err, "tap failed: "+err.Error())
	}
	// Remember the tap location so a following inputText (with no inline
	// selector — Maestro's tapOn+inputText idiom) can pass the same coords
	// into `type`. The runner's textInputAt(x, y) uses geometric containment
	// instead of the `hasKeyboardFocus == 1` predicate, which can throw an
	// NSException for SecureTextField. This is agent-device's PR #298 fix.
	if node.Identifier != "" {
		d.lastTappedIdentifier = node.Identifier
	} else {
		d.lastTappedIdentifier = ""
	}
	d.lastTappedX, d.lastTappedY = cx, cy
	d.lastTapHasCoords = true
	return core.SuccessResult("tapped", toElementInfo(node))
}

// handleInputText routes text through the runner's `type` command. When the
// preceding step was a tap, the saved coordinates are passed so the runner
// resolves the editable element via textInputAt (PR #298). With no recent
// tap, falls through to focused-element typing.
func (d *Driver) handleInputText(s *flow.InputTextStep) *core.CommandResult {
	ctx, cancel := d.callTimeout()
	defer cancel()

	// Inline selector path — resolve locally, tap centre, then type.
	if !selectorIsEmpty(s.Selector) {
		node, err := d.findElement(s.Selector, s.IsOptional(), s.TimeoutMs)
		if err != nil {
			return core.ErrorResult(err, "inputText: "+err.Error())
		}
		if node == nil {
			return core.ErrorResult(fmt.Errorf("element not found"), "element not found")
		}
		cx, cy := centerOf(node)
		// Tap to focus first.
		if _, err := d.client.Call(ctx, Command{
			Command:     CmdTap,
			AppBundleID: d.appID,
			X:           ptrFloat(cx),
			Y:           ptrFloat(cy),
		}); err != nil {
			return core.ErrorResult(err, "focus tap failed: "+err.Error())
		}
		typeCmd := Command{
			Command:     CmdType,
			AppBundleID: d.appID,
			Text:        s.Text,
			X:           ptrFloat(cx),
			Y:           ptrFloat(cy),
		}
		// Pass identifier hint so the runner can resolve the target via
		// XCTest's query DSL (predicate match) instead of walking the
		// whole descendant tree via textInputAt. Saves ~1.4s per call.
		if node.Identifier != "" {
			typeCmd.SelectorKey = "id"
			typeCmd.SelectorValue = node.Identifier
		}
		if _, err := d.client.Call(ctx, typeCmd); err != nil {
			return core.ErrorResult(err, "inputText failed: "+err.Error())
		}
		return core.SuccessResult("typed", toElementInfo(node))
	}

	// No inline selector — Maestro's tapOn+inputText idiom.
	cmd := Command{
		Command:     CmdType,
		AppBundleID: d.appID,
		Text:        s.Text,
	}
	if d.lastTapHasCoords {
		cmd.X = ptrFloat(d.lastTappedX)
		cmd.Y = ptrFloat(d.lastTappedY)
	}
	if d.lastTappedIdentifier != "" {
		cmd.SelectorKey = "id"
		cmd.SelectorValue = d.lastTappedIdentifier
	}
	if _, err := d.client.Call(ctx, cmd); err != nil {
		return core.ErrorResult(err, "inputText failed: "+err.Error())
	}
	return core.SuccessResult("typed", nil)
}

func (d *Driver) handleAssertVisible(s *flow.AssertVisibleStep) *core.CommandResult {
	// IsOptional is honoured by findElement (short poll budget +
	// returns nil-nil on absence). We always surface "not visible" as an
	// ErrorResult: the flow_runner skips optional failures without
	// stopping the flow, and CheckCondition correctly interprets
	// Success=false as "condition not met". Returning SuccessResult on
	// optional+not-found would make `when: visible: ...` always evaluate
	// true regardless of actual visibility.
	optional := s.IsOptional()
	node, err := d.findElement(s.Selector, optional, s.TimeoutMs)
	if err != nil {
		return core.ErrorResult(err, "assertVisible: "+err.Error())
	}
	if node == nil || !isDisplayed(node) {
		return core.ErrorResult(fmt.Errorf("element not visible"), "element not visible")
	}
	return core.SuccessResult("visible", toElementInfo(node))
}

func (d *Driver) handleAssertNotVisible(s *flow.AssertNotVisibleStep) *core.CommandResult {
	nodes, err := d.snapshotMatching(s.Selector)
	if err != nil {
		return core.ErrorResult(err, "assertNotVisible: "+err.Error())
	}
	for i := range nodes {
		if isDisplayed(&nodes[i]) {
			return core.ErrorResult(fmt.Errorf("element unexpectedly visible"), "element visible")
		}
	}
	return core.SuccessResult("not visible", nil)
}

// handleTakeScreenshot uses simctl io booted screenshot for a host-side
// PNG capture. The runner's `screenshot` command writes to the sim's tmp
// container and returns a path, which would require an extra read. Going
// through simctl from the host is one round trip and gives us bytes
// directly.
func (d *Driver) handleTakeScreenshot(s *flow.TakeScreenshotStep) *core.CommandResult {
	png, err := d.captureScreenshot()
	if err != nil {
		return core.ErrorResult(err, "screenshot failed")
	}

	if s.CropOn != nil {
		node, findErr := d.findElement(*s.CropOn, false, 0)
		if findErr != nil || node == nil {
			return core.ErrorResult(findErr, fmt.Sprintf("cropOn: element not found: %v", findErr))
		}
		sw, sh := d.screenDims()
		if sw <= 0 || sh <= 0 {
			return core.ErrorResult(fmt.Errorf("screen dimensions unknown"), "cropOn requires screen dimensions")
		}
		bounds := core.Bounds{
			X:      int(node.Rect.X),
			Y:      int(node.Rect.Y),
			Width:  int(node.Rect.Width),
			Height: int(node.Rect.Height),
		}
		cropped, cropErr := core.CropScreenshot(png, bounds, sw, sh)
		if cropErr != nil {
			return core.ErrorResult(cropErr, fmt.Sprintf("cropOn: %v", cropErr))
		}
		png = cropped
	}

	res := core.SuccessResult("screenshot taken", nil)
	res.Data = png
	return res
}

// handlePressKey maps maestro press-key strings to the appropriate
// agent-device command. Only common keys are wired today; expand as flow
// authors hit gaps.
func (d *Driver) handlePressKey(s *flow.PressKeyStep) *core.CommandResult {
	ctx, cancel := d.callTimeout()
	defer cancel()
	var cmd Command
	switch strings.ToLower(s.Key) {
	case "back":
		cmd = Command{Command: CmdBack, AppBundleID: d.appID}
	case "home":
		cmd = Command{Command: CmdHome, AppBundleID: d.appID}
	case "enter", "return":
		// Type a newline through the focused element.
		cmd = Command{Command: CmdType, AppBundleID: d.appID, Text: "\n"}
	default:
		return core.ErrorResult(
			fmt.Errorf("pressKey %q not supported in devicelab_ios driver", s.Key),
			"pressKey unsupported",
		)
	}
	if _, err := d.client.Call(ctx, cmd); err != nil {
		return core.ErrorResult(err, "pressKey failed: "+err.Error())
	}
	return core.SuccessResult(fmt.Sprintf("pressed %s", s.Key), nil)
}

// handleWaitForAnimation delegates the entire poll loop to the runner via
// the `awaitIdle` command. The runner holds the previous screenshot
// in-process between iterations and compares each new capture against
// it. We pay ONE HTTP roundtrip per waitForAnimation step instead of one
// per iteration. The runner's loop returns as soon as two consecutive
// captures are within the 0.5% threshold, or on timeout (success either
// way — Maestro semantics: best-effort settle).
func (d *Driver) handleWaitForAnimation(s *flow.WaitForAnimationToEndStep) *core.CommandResult {
	timeoutMs := float64(s.TimeoutMs)
	if timeoutMs <= 0 {
		timeoutMs = 15000
	}
	threshold := 0.005
	// Use a callTimeout slightly larger than the wait timeout so HTTP
	// doesn't cut off the runner before its own deadline.
	bufferMs := timeoutMs + 5000
	ctx, cancel := context.WithTimeout(d.parentContext(), time.Duration(bufferMs)*time.Millisecond)
	defer cancel()
	_, err := d.client.Call(ctx, Command{
		Command:     CmdAwaitIdle,
		AppBundleID: d.appID,
		DurationMs:  &timeoutMs,
		Scale:       &threshold,
	})
	if err != nil {
		return core.ErrorResult(err, "awaitIdle failed: "+err.Error())
	}
	return core.SuccessResult("animation ended", nil)
}

// handleEraseText emits a `type` request with textEntryMode="replace"
// and an empty text payload — the runner sees replace mode, calls
// clearTextInput on the focused element (which uses element.typeText
// with backspaces and is much faster than app.typeText), then
// typeTextReliably's empty-text short-circuit returns before typing
// anything. We build the JSON manually here because the Command struct's
// `text` field is JSON `omitempty` (any other handler sending an empty
// string would mis-trigger text-based element matching on the runner).
func (d *Driver) handleEraseText(s *flow.EraseTextStep) *core.CommandResult {
	body := map[string]any{
		"command":       string(CmdType),
		"text":          "",
		"textEntryMode": "replace",
	}
	if d.appID != "" {
		body["appBundleId"] = d.appID
	}
	if d.lastTapHasCoords {
		body["x"] = d.lastTappedX
		body["y"] = d.lastTappedY
	}
	if d.lastTappedIdentifier != "" {
		body["selectorKey"] = "id"
		body["selectorValue"] = d.lastTappedIdentifier
	}
	ctx, cancel := d.callTimeout()
	defer cancel()
	if _, err := d.client.CallRaw(ctx, body); err != nil {
		return core.ErrorResult(err, "eraseText failed: "+err.Error())
	}
	return core.SuccessResult("erased", nil)
}

func (d *Driver) handleBack(s *flow.BackStep) *core.CommandResult {
	ctx, cancel := d.callTimeout()
	defer cancel()
	if _, err := d.client.Call(ctx, Command{
		Command:     CmdBack,
		AppBundleID: d.appID,
	}); err != nil {
		return core.ErrorResult(err, "back failed: "+err.Error())
	}
	return core.SuccessResult("back", nil)
}

func (d *Driver) handleHideKeyboard(s *flow.HideKeyboardStep) *core.CommandResult {
	ctx, cancel := d.callTimeout()
	defer cancel()
	if _, err := d.client.Call(ctx, Command{
		Command:     CmdKeyboardDismiss,
		AppBundleID: d.appID,
	}); err != nil {
		// RN apps frequently have no native keyboard dismiss button and
		// don't respond to swipe-down on the keyboard view. agent-device
		// returns UNSUPPORTED_OPERATION in that case. Maestro flows that
		// call hideKeyboard expect best-effort — keyboard may still be
		// visible, but the next interaction (often a tap outside the
		// field) implicitly dismisses it. Don't fail the step here.
		if re, ok := IsRunnerError(err); ok && re.Code == ErrUnsupportedOperation {
			return core.SuccessResult("keyboard dismiss best-effort (no native control)", nil)
		}
		return core.ErrorResult(err, "hideKeyboard failed: "+err.Error())
	}
	return core.SuccessResult("keyboard dismissed", nil)
}

// handleSwipe routes maestro's SwipeStep to the runner's drag command.
// agent-device's `swipe` command only handles tvOS direction-based swipes;
// for iOS we always use `drag` with concrete coordinates and compute the
// from/to points from direction or percentage/absolute coords.
func (d *Driver) handleSwipe(s *flow.SwipeStep) *core.CommandResult {
	fromX, fromY, toX, toY, err := d.resolveSwipeCoords(s)
	if err != nil {
		return core.ErrorResult(err, "swipe: "+err.Error())
	}
	durationMs := float64(s.Duration)
	if durationMs <= 0 {
		durationMs = 200 // matches maestro upstream default
	}
	ctx, cancel := d.callTimeout()
	defer cancel()
	if _, err := d.client.Call(ctx, Command{
		Command:     CmdDrag,
		AppBundleID: d.appID,
		X:           ptrFloat(fromX),
		Y:           ptrFloat(fromY),
		X2:          ptrFloat(toX),
		Y2:          ptrFloat(toY),
		DurationMs:  &durationMs,
	}); err != nil {
		return core.ErrorResult(err, "swipe failed: "+err.Error())
	}
	return core.SuccessResult("swiped", nil)
}

// handleScroll converts maestro's ScrollStep to a swipe in the opposite
// direction (Maestro: "scroll down" = reveal bottom content = swipe up).
func (d *Driver) handleScroll(s *flow.ScrollStep) *core.CommandResult {
	w, h := d.screenDims()
	if w == 0 || h == 0 {
		return core.ErrorResult(fmt.Errorf("screen size not available"), "screen size unknown")
	}
	centerX := float64(w) / 2
	centerY := float64(h) / 2
	dist := float64(h) / 3
	var fromX, fromY, toX, toY float64
	switch strings.ToLower(s.Direction) {
	case "up":
		fromX, fromY = centerX, centerY-dist/2
		toX, toY = centerX, centerY+dist/2
	case "down":
		fromX, fromY = centerX, centerY+dist/2
		toX, toY = centerX, centerY-dist/2
	case "left":
		fromX, fromY = centerX-dist/2, centerY
		toX, toY = centerX+dist/2, centerY
	case "right":
		fromX, fromY = centerX+dist/2, centerY
		toX, toY = centerX-dist/2, centerY
	default:
		return core.ErrorResult(fmt.Errorf("invalid direction: %s", s.Direction), "invalid scroll direction")
	}
	durationMs := 300.0
	ctx, cancel := d.callTimeout()
	defer cancel()
	if _, err := d.client.Call(ctx, Command{
		Command:     CmdDrag,
		AppBundleID: d.appID,
		X:           ptrFloat(fromX),
		Y:           ptrFloat(fromY),
		X2:          ptrFloat(toX),
		Y2:          ptrFloat(toY),
		DurationMs:  &durationMs,
	}); err != nil {
		return core.ErrorResult(err, "scroll failed: "+err.Error())
	}
	return core.SuccessResult(fmt.Sprintf("scrolled %s", s.Direction), nil)
}

// handleScrollUntilVisible scrolls in the chosen direction until the
// target element appears or the budget runs out.
func (d *Driver) handleScrollUntilVisible(s *flow.ScrollUntilVisibleStep) *core.CommandResult {
	direction := strings.ToLower(s.Direction)
	if direction == "" {
		direction = "down"
	}
	maxScrolls := s.MaxScrolls
	if maxScrolls <= 0 {
		maxScrolls = 20
	}
	timeout := 30 * time.Second
	if s.TimeoutMs > 0 {
		timeout = time.Duration(s.TimeoutMs) * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for i := 0; i < maxScrolls && time.Now().Before(deadline); i++ {
		node, err := d.findElement(s.Element, true, 1000)
		if err == nil && node != nil && isDisplayed(node) {
			return core.SuccessResult("element found after scrolling", toElementInfo(node))
		}
		result := d.handleScroll(&flow.ScrollStep{Direction: direction})
		if !result.Success {
			return result
		}
		time.Sleep(300 * time.Millisecond)
	}
	return core.ErrorResult(
		fmt.Errorf("element not found after %d scrolls", maxScrolls),
		"scroll target not found",
	)
}

func (d *Driver) handleDoubleTap(s *flow.DoubleTapOnStep) *core.CommandResult {
	node, err := d.findElement(s.Selector, s.IsOptional(), s.TimeoutMs)
	if err != nil {
		return core.ErrorResult(err, "doubleTap: "+err.Error())
	}
	if node == nil {
		return core.ErrorResult(fmt.Errorf("element not found"), "element not found")
	}
	cx, cy := centerOf(node)
	ctx, cancel := d.callTimeout()
	defer cancel()
	if _, err := d.client.Call(ctx, Command{
		Command:     CmdTapSeries,
		AppBundleID: d.appID,
		X:           ptrFloat(cx),
		Y:           ptrFloat(cy),
		DoubleTap:   boolPtr(true),
	}); err != nil {
		return core.ErrorResult(err, "doubleTap failed: "+err.Error())
	}
	return core.SuccessResult("double-tapped", toElementInfo(node))
}

// handleOpenLink dispatches a URL to the simulator via `simctl openurl`,
// then waits for the target app to reach foreground state. The wait is
// important: simctl openurl returns immediately, but the deep-link launch
// is asynchronous. If the runner's next command (e.g. extendedWaitUntil's
// snapshot) arrives while the app is still in `notRunning` state, the
// runner's auto-activateTarget will call XCUIApplication.activate() —
// which launches the app fresh WITHOUT the deep-link URL, taking it to
// the home/initial screen instead of the linked route. Waiting here
// ensures the deep link's launch wins the race.
func (d *Driver) handleOpenLink(s *flow.OpenLinkStep) *core.CommandResult {
	if s.Link == "" {
		return core.ErrorResult(fmt.Errorf("no link specified"), "no link")
	}
	if out, err := exec.Command("xcrun", "simctl", "openurl", d.udid, s.Link).CombinedOutput(); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("openurl failed: %s", strings.TrimSpace(string(out))))
	}
	// Wait for the target bundle to be running. Poll `simctl spawn launchctl
	// list` for the bundle id. Bounded so a never-launching app still
	// surfaces a clear error later via the next assertion.
	if d.appID != "" {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			out, _ := exec.Command("xcrun", "simctl", "spawn", d.udid, "launchctl", "list").Output()
			if strings.Contains(string(out), d.appID) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	if s.AutoVerify != nil && *s.AutoVerify {
		time.Sleep(2 * time.Second)
	}
	return core.SuccessResult(fmt.Sprintf("opened link: %s", s.Link), nil)
}

// handleSetLocation sets the simulator's GPS location via `xcrun simctl
// location <udid> set <lat>,<lon>` — same mechanism Maestro uses
// (LocalSimulatorUtils.kt) and the parallel WDA driver in this repo.
//
// Real-device path: Apple's public tooling (devicectl / instruments / WDA)
// doesn't expose a way to override GPS on a real iPhone, so there is no
// supported way to do this. Returns an explicit error rather than silently
// no-op'ing — Maestro's own real-device implementation is a `TODO` stub
// for the same reason. App-level mocking is the practical workaround.
func (d *Driver) handleSetLocation(s *flow.SetLocationStep) *core.CommandResult {
	if s.Latitude == "" || s.Longitude == "" {
		return core.ErrorResult(
			fmt.Errorf("latitude and longitude required"),
			"setLocation requires both latitude and longitude",
		)
	}
	lat, err := strconv.ParseFloat(s.Latitude, 64)
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("Invalid latitude: %s", s.Latitude))
	}
	lon, err := strconv.ParseFloat(s.Longitude, 64)
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("Invalid longitude: %s", s.Longitude))
	}
	if d.info == nil || !d.info.IsSimulator {
		return core.ErrorResult(
			fmt.Errorf("setLocation not supported on iOS real devices"),
			"setLocation isn't supported on iOS real devices: Apple's public tooling "+
				"(devicectl / instruments / WDA) doesn't expose a way to override GPS on "+
				"a real device. Run the flow on an iOS simulator, or mock location at the app level.",
		)
	}
	if d.udid == "" {
		return core.ErrorResult(
			fmt.Errorf("device UDID not configured"),
			"setLocation requires a simulator UDID",
		)
	}
	cmd := exec.Command("xcrun", "simctl", "location", d.udid, "set", fmt.Sprintf("%f,%f", lat, lon))
	if out, err := cmd.CombinedOutput(); err != nil {
		return core.ErrorResult(
			fmt.Errorf("simctl location: %w: %s", err, strings.TrimSpace(string(out))),
			"Failed to set simulator location",
		)
	}
	return core.SuccessResult(fmt.Sprintf("Location set to (%f, %f) on simulator %s", lat, lon, d.udid), nil)
}

func (d *Driver) handleLongPress(s *flow.LongPressOnStep) *core.CommandResult {
	node, err := d.findElement(s.Selector, s.IsOptional(), s.TimeoutMs)
	if err != nil {
		return core.ErrorResult(err, "longPress: "+err.Error())
	}
	if node == nil {
		return core.ErrorResult(fmt.Errorf("element not found"), "element not found")
	}
	cx, cy := centerOf(node)
	durationMs := 800.0
	if s.DurationMs > 0 {
		durationMs = float64(s.DurationMs)
	}
	ctx, cancel := d.callTimeout()
	defer cancel()
	if _, err := d.client.Call(ctx, Command{
		Command:     CmdLongPress,
		AppBundleID: d.appID,
		X:           ptrFloat(cx),
		Y:           ptrFloat(cy),
		DurationMs:  &durationMs,
	}); err != nil {
		return core.ErrorResult(err, "longPress failed: "+err.Error())
	}
	return core.SuccessResult("long-pressed", toElementInfo(node))
}

// resolveSwipeCoords converts a SwipeStep's direction OR start/end into
// concrete from/to coords. Order of precedence matches WDA: Start/End
// percentages first, then absolute StartX/Y/EndX/Y, then direction +
// optional selector for the swipe area.
func (d *Driver) resolveSwipeCoords(s *flow.SwipeStep) (float64, float64, float64, float64, error) {
	w, h := d.screenDims()
	if (s.Start != "" && s.End != "") {
		sx, sy, err := parsePercentageCoords(s.Start)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		ex, ey, err := parsePercentageCoords(s.End)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		return float64(w) * sx, float64(h) * sy, float64(w) * ex, float64(h) * ey, nil
	}
	if s.StartX > 0 || s.StartY > 0 || s.EndX > 0 || s.EndY > 0 {
		return float64(s.StartX), float64(s.StartY), float64(s.EndX), float64(s.EndY), nil
	}
	// Direction-based, optionally within a selector's area.
	areaX, areaY := 0.0, 0.0
	areaW, areaH := float64(w), float64(h)
	if s.Selector != nil && !s.Selector.IsEmpty() {
		node, err := d.findElement(*s.Selector, false, s.TimeoutMs)
		if err == nil && node != nil && node.Rect.Width > 0 {
			areaX = node.Rect.X
			areaY = node.Rect.Y
			areaW = node.Rect.Width
			areaH = node.Rect.Height
		}
	}
	switch strings.ToLower(s.Direction) {
	case "up":
		return areaX + areaW*0.5, areaY + areaH*0.9, areaX + areaW*0.5, areaY + areaH*0.1, nil
	case "down":
		return areaX + areaW*0.5, areaY + areaH*0.2, areaX + areaW*0.5, areaY + areaH*0.9, nil
	case "left":
		return areaX + areaW*0.9, areaY + areaH*0.5, areaX + areaW*0.1, areaY + areaH*0.5, nil
	case "right":
		return areaX + areaW*0.1, areaY + areaH*0.5, areaX + areaW*0.9, areaY + areaH*0.5, nil
	default:
		return 0, 0, 0, 0, fmt.Errorf("invalid swipe direction: %q", s.Direction)
	}
}

func (d *Driver) screenDims() (int, int) {
	if d.info != nil {
		return d.info.ScreenWidth, d.info.ScreenHeight
	}
	return 0, 0
}

func parsePercentageCoords(s string) (float64, float64, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected 'x%%, y%%' got %q", s)
	}
	x, err := parsePercent(parts[0])
	if err != nil {
		return 0, 0, err
	}
	y, err := parsePercent(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

func parsePercent(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	var n float64
	if _, err := fmt.Sscanf(s, "%f", &n); err != nil {
		return 0, fmt.Errorf("bad percent %q: %w", s, err)
	}
	return n / 100, nil
}

func boolPtr(b bool) *bool { return &b }

func (d *Driver) handleWaitUntil(s *flow.WaitUntilStep) *core.CommandResult {
	timeout := time.Duration(s.TimeoutMs) * time.Millisecond
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		if s.Visible != nil {
			nodes, err := d.snapshotMatching(*s.Visible)
			if err == nil {
				for i := range nodes {
					if isDisplayed(&nodes[i]) {
						return core.SuccessResult("visible", toElementInfo(&nodes[i]))
					}
				}
			}
		}
		if s.NotVisible != nil {
			nodes, err := d.snapshotMatching(*s.NotVisible)
			if err != nil {
				return core.ErrorResult(err, "snapshot failed")
			}
			anyVisible := false
			for i := range nodes {
				if isDisplayed(&nodes[i]) {
					anyVisible = true
					break
				}
			}
			if !anyVisible {
				return core.SuccessResult("not visible", nil)
			}
		}
		if !time.Now().Before(deadline) {
			return core.ErrorResult(fmt.Errorf("extendedWaitUntil timed out after %s", timeout),
				"wait timed out")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// ---------- shared building blocks ----------

// Default timeouts when no per-step or driver-level override is set.
const (
	defaultFindTimeoutMs         = 10_000
	defaultOptionalFindTimeoutMs = 2_000
)

// simpleSelectorKeyValue extracts (key, value) for selectors that can be
// resolved via the runner's CmdQuerySelector — currently pure-id or pure-text
// with no state filters and no regex metacharacters. Returns ok=false for
// anything else; those callers must fall back to snapshot+filter.
func simpleSelectorKeyValue(sel flow.Selector) (key, value string, ok bool) {
	if sel.CSS != "" || sel.Width != 0 || sel.Height != 0 {
		return "", "", false
	}
	if sel.Enabled != nil || sel.Selected != nil || sel.Focused != nil {
		return "", "", false
	}
	if sel.ID != "" && sel.Text == "" {
		if looksLikeRegex(sel.ID) {
			return "", "", false
		}
		return "id", sel.ID, true
	}
	if sel.Text != "" && sel.ID == "" {
		if looksLikeRegex(sel.Text) {
			return "", "", false
		}
		return "text", sel.Text, true
	}
	return "", "", false
}

// resolveByQuerySelector asks the runner for the element matching the
// given selectorKey/selectorValue directly via XCUIElementQuery, skipping
// the Go-side snapshot+filter round trip. This catches elements that the
// snapshot's flat tree misses on deep React Native hierarchies — XCUIElementQuery
// uses XCTest's in-process query DSL which doesn't depend on snapshot completeness.
// Returns (nil, nil) if the element isn't currently present or the selector
// matched multiple elements ambiguously.
func (d *Driver) resolveByQuerySelector(key, value string) (*SnapshotNode, error) {
	ctx, cancel := d.callTimeout()
	defer cancel()
	data, err := d.client.Call(ctx, Command{
		Command:       CmdQuerySelector,
		AppBundleID:   d.appID,
		SelectorKey:   key,
		SelectorValue: value,
	})
	if err != nil {
		if re, ok := IsRunnerError(err); ok && re.Code == ErrAmbiguousMatch {
			return nil, nil
		}
		return nil, err
	}
	if data == nil || data.Found == nil || !*data.Found || len(data.Nodes) == 0 {
		return nil, nil
	}
	node := data.Nodes[0]
	return &node, nil
}

// tapBySelectorHit summarises a successful tapBySelector response. The
// runner echoes back the tap coordinates and the matched element's
// identifier so the caller can populate d.lastTapped{X,Y,Identifier} —
// inputText reads those to hint the runner at which editable element to
// focus.
type tapBySelectorHit struct {
	X          float64
	Y          float64
	Identifier string
}

// tryTapBySelector asks the runner to find+tap atomically via the
// `tapBySelector` command. Returns the tap-coordinates + matched
// identifier on success, nil on miss (caller falls back to
// snapshot+filter+tap path), or an error for transport problems.
// The runner uses the same liberal matching + prefer-editable-inputs
// ranking as Go-side matchesSelector, so a hit here means we'd have hit
// on the snapshot path too — just in one round-trip instead of two.
func (d *Driver) tryTapBySelector(key, value string) (*tapBySelectorHit, error) {
	ctx, cancel := d.callTimeout()
	defer cancel()
	data, err := d.client.Call(ctx, Command{
		Command:       CmdTapBySelector,
		AppBundleID:   d.appID,
		SelectorKey:   key,
		SelectorValue: value,
	})
	if err != nil {
		return nil, err
	}
	// On miss the runner returns ok:false with walked snapshot nodes in
	// data.Nodes — that's how it signals "no match found" without losing
	// the work it did walking the tree. We treat that as a miss (nil) so
	// the caller falls through to the slower path.
	if data == nil || data.X == nil || data.Y == nil {
		return nil, nil
	}
	return &tapBySelectorHit{
		X:          *data.X,
		Y:          *data.Y,
		Identifier: data.Identifier,
	}, nil
}

// findElement resolves a selector to a single node. Polls until the
// applicable timeout if nothing matches immediately. For optional
// elements (where absence is acceptable) the poll budget is much shorter
// by default — looking for an optional element shouldn't block the flow
// for the full required-element timeout.
func (d *Driver) findElement(sel flow.Selector, optional bool, stepTimeoutMs int) (*SnapshotNode, error) {
	var timeout int
	switch {
	case stepTimeoutMs > 0:
		timeout = stepTimeoutMs
	case optional && d.optionalFindTimeout > 0:
		timeout = d.optionalFindTimeout
	case optional:
		timeout = defaultOptionalFindTimeoutMs
	case d.findTimeout > 0:
		timeout = d.findTimeout
	default:
		timeout = defaultFindTimeoutMs
	}

	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)

	qsKey, qsValue, qsOK := simpleSelectorKeyValue(sel)
	firstPass := true

	for {
		if !firstPass {
			// Force a fresh snapshot for each retry — the 200ms cache TTL
			// otherwise serves stale data across consecutive iterations,
			// pinning us to a pre-navigation tree.
			d.invalidateSnapshotCache()
		}
		firstPass = false

		// Strategy 1: snapshot dump + local filter.
		nodes, err := d.snapshotMatching(sel)
		if err != nil {
			return nil, err
		}
		if len(nodes) > 0 {
			return selectByIndex(nodesToPtrs(nodes), sel.Index), nil
		}

		// Strategy 2: on-device querySelector via XCUIElementQuery.
		// Catches elements that the snapshot's flat tree misses on deep RN
		// hierarchies (the snapshot truncates / omits some nested .other
		// containers; XCUI's query DSL can still find children inside them).
		if qsOK {
			if node, qerr := d.resolveByQuerySelector(qsKey, qsValue); qerr == nil && node != nil {
				return node, nil
			}
		}

		if !time.Now().Before(deadline) {
			if optional {
				return nil, nil
			}
			return nil, fmt.Errorf("element not found: %s", describeSelector(sel))
		}
		// No explicit sleep — strategies 1 and 2 each do real HTTP round-trips
		// (~100-200ms snapshot + ~50ms query) which paces the loop naturally.
	}
}

// snapshotMatching returns nodes matching the selector. Reuses the most
// recent snapshot when it's <snapshotCacheTTL old. Interaction steps
// (tap/type/swipe/back/launchApp/stopApp) invalidate the cache up-front
// in executeStep so the cache never serves stale screen state.
const snapshotCacheTTL = 200 * time.Millisecond

func (d *Driver) snapshotMatching(sel flow.Selector) ([]SnapshotNode, error) {
	nodes, err := d.fetchSnapshot()
	if err != nil {
		return nil, err
	}
	var hits []SnapshotNode
	for i := range nodes {
		if matchesSelector(&nodes[i], sel) {
			hits = append(hits, nodes[i])
		}
	}
	// Always prefer editable inputs / interactive controls when multiple
	// nodes match. Applies to id-selectors too: RN TextInputs often share
	// the testID with a sibling Image (e.g. a search icon), and we want
	// the actual TextField, not the icon.
	if len(hits) > 1 {
		hits = preferEditableInputs(hits)
	}
	return hits, nil
}

func (d *Driver) fetchSnapshot() ([]SnapshotNode, error) {
	if d.snapshotCache != nil && time.Since(d.snapshotCacheTime) < snapshotCacheTTL {
		return d.snapshotCache, nil
	}
	ctx, cancel := d.callTimeout()
	defer cancel()
	// Default snapshot (no depth limit, no compact). The tighter options
	// regressed React Native apps where the tree is deeply nested and the
	// text selectors target StaticText whose parents are .other wrappers
	// that compact mode drops.
	data, err := d.client.Call(ctx, Command{
		Command:     CmdSnapshot,
		AppBundleID: d.appID,
	})
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	d.snapshotCache = data.Nodes
	d.snapshotCacheTime = time.Now()
	return data.Nodes, nil
}

func (d *Driver) invalidateSnapshotCache() {
	d.snapshotCache = nil
	d.snapshotCacheTime = time.Time{}
}

// preferEditableInputs reorders text-matched candidates so the most
// actionable element wins: editable inputs first, then interactive
// controls (Button, Link, Cell, MenuItem, Switch, CheckBox), then
// everything else. Otherwise raw snapshot order returns labels like
// "Sign in to continue" before the actual "Sign In" Button.
func preferEditableInputs(in []SnapshotNode) []SnapshotNode {
	var inputs, controls, others []SnapshotNode
	for _, n := range in {
		switch n.Type {
		case "TextField", "SecureTextField", "SearchField", "TextView":
			inputs = append(inputs, n)
		case "Button", "Link", "Cell", "MenuItem", "Switch", "CheckBox":
			controls = append(controls, n)
		default:
			others = append(others, n)
		}
	}
	if len(inputs) == 0 && len(controls) == 0 {
		return in
	}
	out := make([]SnapshotNode, 0, len(in))
	out = append(out, inputs...)
	out = append(out, controls...)
	out = append(out, others...)
	return out
}

// captureScreenshot asks the runner for a PNG of the device screen. The
// runner takes XCUIScreen.main.screenshot() in-test and returns the PNG
// base64-encoded inline (one HTTP roundtrip; no subprocess fork). Previously
// this went through `xcrun simctl io ... screenshot -` which adds a fork +
// stdin pipe per call — measurable cost when waitForAnimationToEnd polls
// every 250ms.
func (d *Driver) captureScreenshot() ([]byte, error) {
	ctx, cancel := d.callTimeout()
	defer cancel()
	data, err := d.client.Call(ctx, Command{
		Command:     CmdScreenshot,
		AppBundleID: d.appID,
	})
	if err != nil {
		return nil, fmt.Errorf("runner screenshot: %w", err)
	}
	if data == nil || data.PngBase64 == "" {
		return nil, fmt.Errorf("runner returned empty screenshot")
	}
	return base64.StdEncoding.DecodeString(data.PngBase64)
}

// hierarchyJSON returns the flat snapshot list as JSON for the reporter.
func (d *Driver) hierarchyJSON() ([]byte, error) {
	ctx, cancel := d.callTimeout()
	defer cancel()
	data, err := d.client.Call(ctx, Command{
		Command:     CmdSnapshot,
		AppBundleID: d.appID,
	})
	if err != nil {
		return nil, err
	}
	if data == nil {
		return []byte("{\"nodes\":[]}"), nil
	}
	return json.Marshal(map[string]any{"nodes": data.Nodes})
}

// ---------- utilities ----------

func centerOf(n *SnapshotNode) (float64, float64) {
	return n.Rect.X + n.Rect.Width/2, n.Rect.Y + n.Rect.Height/2
}

// isDisplayed approximates Maestro's "displayed" predicate. agent-device's
// snapshot doesn't carry an explicit displayed flag. We treat any element
// with a non-empty rect as displayed — hittable is too strict (agent-device
// marks labels inside alerts as non-hittable because the alert's frame
// covers their centre, but they're clearly on screen and assertVisible
// should accept them).
func isDisplayed(n *SnapshotNode) bool {
	return n != nil && n.Rect.Width > 0 && n.Rect.Height > 0
}

func nodesToPtrs(in []SnapshotNode) []*SnapshotNode {
	out := make([]*SnapshotNode, len(in))
	for i := range in {
		out[i] = &in[i]
	}
	return out
}

func ptrFloat(f float64) *float64 { return &f }

func selectorIsEmpty(s flow.Selector) bool {
	return s.Text == "" && s.ID == "" && s.CSS == "" && s.Width == 0 && s.Height == 0
}

func describeSelector(s flow.Selector) string {
	parts := []string{}
	if s.Text != "" {
		parts = append(parts, "text="+s.Text)
	}
	if s.ID != "" {
		parts = append(parts, "id="+s.ID)
	}
	if len(parts) == 0 {
		return "<empty selector>"
	}
	return strings.Join(parts, ", ")
}

func flattenArguments(args map[string]any) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args)*2)
	for k, v := range args {
		out = append(out, "-"+k, fmt.Sprintf("%v", v))
	}
	return out
}

func bytesEqual(a, b []byte) bool { //nolint:unused
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var _ = context.Background
