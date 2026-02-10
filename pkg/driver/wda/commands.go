package wda

import (
	"context"
	"fmt"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"
	"time"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/installationproxy"
	"github.com/danielpaulus/go-ios/ios/zipconduit"
	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Tap commands

func (d *Driver) tapOn(step *flow.TapOnStep) *core.CommandResult {
	// Check if using percentage-based Point WITHOUT selector (screen-relative tap)
	if step.Point != "" && step.Selector.IsEmpty() {
		return d.tapOnPointWithPercentage(step.Point)
	}

	// Handle keyboard key names — iOS keyboard buttons aren't reliably findable via WDA
	if step.Selector.Text != "" {
		if keyChar := iosKeyboardKey(step.Selector.Text); keyChar != "" {
			if err := d.client.SendKeys(keyChar); err != nil {
				return errorResult(err, fmt.Sprintf("Failed to send key: %s", step.Selector.Text))
			}
			return successResult(fmt.Sprintf("Pressed keyboard key: %s", step.Selector.Text), nil)
		}
	}

	info, err := d.findElementForTap(step.Selector, step.Optional, step.TimeoutMs)
	if err != nil {
		if step.Optional {
			return successResult("Optional element not found, skipping tap", nil)
		}
		return errorResult(err, fmt.Sprintf("Element not found: %s", selectorDesc(step.Selector)))
	}

	// If Point is specified WITH selector, tap at relative position within element bounds
	if step.Point != "" && info != nil && info.Bounds.Width > 0 {
		xPct, yPct, parseErr := parsePercentageCoords(step.Point)
		if parseErr != nil {
			return errorResult(parseErr, "Invalid point coordinates")
		}
		x := float64(info.Bounds.X) + float64(info.Bounds.Width)*xPct
		y := float64(info.Bounds.Y) + float64(info.Bounds.Height)*yPct
		if err := d.client.Tap(x, y); err != nil {
			return errorResult(err, "Tap at relative point failed")
		}
		return successResult(fmt.Sprintf("Tapped at relative point (%.0f, %.0f) on element", x, y), info)
	}

	// Determine if element is a text field (needs focus verification)
	isTextField := false
	if info.ID != "" {
		if name, err := d.client.ElementName(info.ID); err == nil {
			isTextField = strings.Contains(name, "TextField")
		}
	}

	// Strategy: ElementClick first (WDA's internal element targeting handles z-order),
	// then coordinate tap as fallback. For text fields, verify focus after each attempt
	// because ElementClick can return success without actually focusing the field.
	tapped := false
	if info.ID != "" {
		if err := d.client.ElementClick(info.ID); err == nil {
			tapped = true
			if isTextField {
				time.Sleep(100 * time.Millisecond)
				if _, err := d.client.GetActiveElement(); err != nil {
					tapped = false // No focus — retry with coordinate tap
				}
			}
		}
	}

	if !tapped {
		x := float64(info.Bounds.X + info.Bounds.Width/2)
		y := float64(info.Bounds.Y + info.Bounds.Height/2)
		if err := d.client.Tap(x, y); err != nil {
			return errorResult(err, "Tap failed")
		}
	}

	return successResult("Tapped element", info)
}

// tapOnPointWithPercentage handles percentage-based tap (e.g., "85%, 50%")
func (d *Driver) tapOnPointWithPercentage(point string) *core.CommandResult {
	width, height, err := d.client.WindowSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}

	xPct, yPct, err := parsePercentageCoords(point)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid point coordinates: %s", point))
	}

	x := float64(width) * xPct
	y := float64(height) * yPct

	if err := d.client.Tap(x, y); err != nil {
		return errorResult(err, "Tap at point failed")
	}

	return successResult(fmt.Sprintf("Tapped at (%.0f, %.0f)", x, y), nil)
}

func (d *Driver) doubleTapOn(step *flow.DoubleTapOnStep) *core.CommandResult {
	info, err := d.findElementForTap(step.Selector, false, step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %s", selectorDesc(step.Selector)))
	}

	x := float64(info.Bounds.X + info.Bounds.Width/2)
	y := float64(info.Bounds.Y + info.Bounds.Height/2)

	if err := d.client.DoubleTap(x, y); err != nil {
		return errorResult(err, "Double tap failed")
	}

	return successResult("Double tapped element", info)
}

func (d *Driver) longPressOn(step *flow.LongPressOnStep) *core.CommandResult {
	info, err := d.findElementForTap(step.Selector, false, step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %s", selectorDesc(step.Selector)))
	}

	x := float64(info.Bounds.X + info.Bounds.Width/2)
	y := float64(info.Bounds.Y + info.Bounds.Height/2)

	duration := 1.0 // default 1 second

	if err := d.client.LongPress(x, y, duration); err != nil {
		return errorResult(err, "Long press failed")
	}

	return successResult("Long pressed element", info)
}

func (d *Driver) tapOnPoint(step *flow.TapOnPointStep) *core.CommandResult {
	var x, y float64

	// Handle percentage-based coordinates via Point field
	if step.Point != "" {
		width, height, err := d.client.WindowSize()
		if err != nil {
			return errorResult(err, "Failed to get screen size")
		}
		pctX, pctY, err := parsePercentageCoords(step.Point)
		if err != nil {
			return errorResult(err, "Invalid point format")
		}
		x = float64(width) * pctX
		y = float64(height) * pctY
	} else {
		x = float64(step.X)
		y = float64(step.Y)
	}

	if err := d.client.Tap(x, y); err != nil {
		return errorResult(err, "Tap on point failed")
	}

	return successResult(fmt.Sprintf("Tapped at (%.0f, %.0f)", x, y), nil)
}

// Assert commands

func (d *Driver) assertVisible(step *flow.AssertVisibleStep) *core.CommandResult {
	info, err := d.findElement(step.Selector, false, step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not visible: %s", selectorDesc(step.Selector)))
	}

	return successResult("Element is visible", info)
}

func (d *Driver) assertNotVisible(step *flow.AssertNotVisibleStep) *core.CommandResult {
	// Poll to confirm element stays invisible
	// Default 5s aligns closer to Maestro's optionalLookupTimeoutMs (7s)
	timeoutMs := step.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}

	info, err := d.findElement(step.Selector, true, timeoutMs)
	if err != nil || info == nil {
		return successResult("Element is not visible", nil)
	}

	return errorResult(fmt.Errorf("element is visible"), fmt.Sprintf("Element should not be visible: %s", selectorDesc(step.Selector)))
}

// Input commands

func (d *Driver) inputText(step *flow.InputTextStep) *core.CommandResult {
	text := step.Text
	if text == "" {
		return errorResult(fmt.Errorf("no text specified"), "No text to input")
	}

	// Check for non-ASCII characters (may cause input issues on some devices)
	unicodeWarning := ""
	if core.HasNonASCII(text) {
		unicodeWarning = " (warning: non-ASCII characters may not input correctly)"
	}

	// If selector provided, find the element and type directly into it
	if !step.Selector.IsEmpty() {
		info, err := d.findElement(step.Selector, step.IsOptional(), step.TimeoutMs)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Element not found: %s", selectorDesc(step.Selector)))
		}
		// If we have element ID, send keys directly to the element
		if info.ID != "" {
			if err := d.client.ElementSendKeys(info.ID, text); err != nil {
				return errorResult(err, "Input text to element failed")
			}
			return successResult(fmt.Sprintf("Entered text: %s%s", text, unicodeWarning), info)
		}
		// Fallback: tap to focus first
		x := float64(info.Bounds.X + info.Bounds.Width/2)
		y := float64(info.Bounds.Y + info.Bounds.Height/2)
		if err := d.client.Tap(x, y); err != nil {
			return errorResult(err, "Failed to tap element before input")
		}
		time.Sleep(100 * time.Millisecond) // Wait for focus
	}

	// Wait for keyboard to be ready by confirming a text field is focused.
	// Poll GetActiveElement up to 1s (5 attempts, 200ms apart) similar to
	// original Maestro's InputTextRouteHandler.swift keyboard wait.
	for i := 0; i < 5; i++ {
		if elemID, err := d.client.GetActiveElement(); err == nil && elemID != "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err := d.client.SendKeys(text); err != nil {
		return errorResult(err, "Input text failed")
	}

	return successResult(fmt.Sprintf("Entered text: %s%s", text, unicodeWarning), nil)
}

func (d *Driver) eraseText(step *flow.EraseTextStep) *core.CommandResult {
	chars := step.Characters
	if chars == 0 {
		chars = 50 // default
	}

	// Try optimized approach first (Clear or text replacement)
	// This is much faster than sending delete keys (3 HTTP calls vs N characters)
	elemID, err := d.client.GetActiveElement()
	if err == nil && elemID != "" {
		// Got active element - try to read its text
		currentText, textErr := d.client.ElementText(elemID)
		if textErr == nil {
			textLen := len([]rune(currentText)) // Use runes for proper Unicode handling

			// Case 1: Erase all text (or more than exists) - just Clear() in one shot
			if chars >= textLen || textLen == 0 {
				if clearErr := d.client.ElementClear(elemID); clearErr == nil {
					return successResult(fmt.Sprintf("Cleared %d characters", textLen), nil)
				}
				// Clear failed, fall through to delete key approach
			} else {
				// Case 2: Erase N chars from end - use text replacement
				runes := []rune(currentText)
				remaining := string(runes[:textLen-chars])

				if clearErr := d.client.ElementClear(elemID); clearErr == nil {
					if remaining != "" {
						if sendErr := d.client.SendKeys(remaining); sendErr == nil {
							return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
						}
						// SendKeys failed, fall through to delete key approach
					} else {
						// Remaining text is empty, Clear() already did the job
						return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
					}
				}
				// Clear failed, fall through to delete key approach
			}
		}
		// ElementText failed (e.g., secure text field), fall through to delete key approach
	}
	// GetActiveElement failed, fall through to delete key approach

	// Fallback: Send all delete keys in a single request
	// WDA supports sending multiple backspace characters at once
	deleteStr := strings.Repeat("\b", chars)
	if err := d.client.SendKeys(deleteStr); err != nil {
		return errorResult(err, "Erase text failed")
	}

	return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
}

func (d *Driver) hideKeyboard(step *flow.HideKeyboardStep) *core.CommandResult {
	// iOS: tap outside to dismiss keyboard, or press Done button
	// Try pressing the "return" key (ignore error - keyboard might not be visible)
	_ = d.client.SendKeys("\n")

	return successResult("Attempted to hide keyboard", nil)
}

func (d *Driver) inputRandom(step *flow.InputRandomStep) *core.CommandResult {
	length := step.Length
	if length <= 0 {
		length = 10 // default
	}

	// Generate random data based on DataType
	var text string
	dataType := strings.ToUpper(step.DataType)
	switch dataType {
	case "EMAIL":
		text = randomEmail()
	case "NUMBER":
		text = randomNumber(length)
	case "PERSON_NAME":
		text = randomPersonName()
	default: // "TEXT" or empty
		text = randomString(length)
	}

	if err := d.client.SendKeys(text); err != nil {
		return errorResult(err, "Input random text failed")
	}

	return &core.CommandResult{
		Success: true,
		Message: fmt.Sprintf("Entered random %s: %s", dataType, text),
		Data:    text,
	}
}

// Scroll/Swipe commands

func (d *Driver) scroll(step *flow.ScrollStep) *core.CommandResult {
	width, height, err := d.client.WindowSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}

	centerX := float64(width) / 2
	centerY := float64(height) / 2
	scrollDistance := float64(height) / 3

	// Scroll direction = content movement direction
	// "scroll down" means reveal content below, which requires swiping UP
	// Maestro: ScrollDirection.DOWN -> SwipeDirection.UP
	var fromX, fromY, toX, toY float64
	switch step.Direction {
	case "up":
		// Scroll up = reveal top content = swipe DOWN
		fromX, fromY = centerX, centerY-scrollDistance/2
		toX, toY = centerX, centerY+scrollDistance/2
	case "down":
		// Scroll down = reveal bottom content = swipe UP
		fromX, fromY = centerX, centerY+scrollDistance/2
		toX, toY = centerX, centerY-scrollDistance/2
	case "left":
		// Scroll left = reveal left content = swipe RIGHT
		fromX, fromY = centerX-scrollDistance/2, centerY
		toX, toY = centerX+scrollDistance/2, centerY
	case "right":
		// Scroll right = reveal right content = swipe LEFT
		fromX, fromY = centerX+scrollDistance/2, centerY
		toX, toY = centerX-scrollDistance/2, centerY
	default:
		return errorResult(fmt.Errorf("invalid direction: %s", step.Direction), "Invalid scroll direction")
	}

	if err := d.client.Swipe(fromX, fromY, toX, toY, 0.3); err != nil {
		return errorResult(err, "Scroll failed")
	}

	return successResult(fmt.Sprintf("Scrolled %s", step.Direction), nil)
}

func (d *Driver) scrollUntilVisible(step *flow.ScrollUntilVisibleStep) *core.CommandResult {
	direction := step.Direction
	if direction == "" {
		direction = "down"
	}

	maxScrolls := 10
	if step.TimeoutMs > 0 {
		maxScrolls = step.TimeoutMs / 1000 // rough estimate
	}

	for i := 0; i < maxScrolls; i++ {
		// Check if element is visible (includes page source fallback)
		info, err := d.findElement(step.Element, true, 1000)
		if err == nil && info != nil {
			return successResult("Element found after scrolling", info)
		}

		// Scroll
		scrollStep := &flow.ScrollStep{Direction: direction}
		result := d.scroll(scrollStep)
		if !result.Success {
			return result
		}

		time.Sleep(300 * time.Millisecond) // Wait for scroll animation
	}

	return errorResult(fmt.Errorf("element not found after scrolling"), fmt.Sprintf("Element not found: %s", selectorDesc(step.Element)))
}

func (d *Driver) swipe(step *flow.SwipeStep) *core.CommandResult {
	width, height, err := d.client.WindowSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}

	var fromX, fromY, toX, toY float64

	// Handle coordinate-based swipe
	if step.Start != "" && step.End != "" {
		startX, startY, err := parsePercentageCoords(step.Start)
		if err != nil {
			return errorResult(err, "Invalid start coordinates")
		}
		endX, endY, err := parsePercentageCoords(step.End)
		if err != nil {
			return errorResult(err, "Invalid end coordinates")
		}

		fromX = float64(width) * startX
		fromY = float64(height) * startY
		toX = float64(width) * endX
		toY = float64(height) * endY
	} else if step.StartX > 0 || step.StartY > 0 {
		// Direct pixel coordinates
		fromX = float64(step.StartX)
		fromY = float64(step.StartY)
		toX = float64(step.EndX)
		toY = float64(step.EndY)
	} else {
		// Direction-based swipe
		var areaX, areaY, areaW, areaH float64
		areaX, areaY = 0, 0
		areaW, areaH = float64(width), float64(height)

		// If selector specified, swipe within that element's bounds
		if step.Selector != nil && !step.Selector.IsEmpty() {
			info, err := d.findElement(*step.Selector, false, step.TimeoutMs)
			if err != nil {
				return errorResult(err, fmt.Sprintf("Element not found for swipe: %s", step.Selector.Describe()))
			}
			if info != nil && info.Bounds.Width > 0 {
				areaX = float64(info.Bounds.X)
				areaY = float64(info.Bounds.Y)
				areaW = float64(info.Bounds.Width)
				areaH = float64(info.Bounds.Height)
			}
		}

		centerX := areaX + areaW/2
		centerY := areaY + areaH/2
		swipeDistance := areaH / 3

		switch step.Direction {
		case "up":
			fromX, fromY = centerX, centerY+swipeDistance/2
			toX, toY = centerX, centerY-swipeDistance/2
		case "down":
			fromX, fromY = centerX, centerY-swipeDistance/2
			toX, toY = centerX, centerY+swipeDistance/2
		case "left":
			swipeDistance = areaW / 3
			fromX, fromY = centerX+swipeDistance/2, centerY
			toX, toY = centerX-swipeDistance/2, centerY
		case "right":
			swipeDistance = areaW / 3
			fromX, fromY = centerX-swipeDistance/2, centerY
			toX, toY = centerX+swipeDistance/2, centerY
		default:
			return errorResult(fmt.Errorf("invalid direction: %s", step.Direction), "Invalid swipe direction")
		}
	}

	duration := 0.3
	if step.Duration > 0 {
		duration = float64(step.Duration) / 1000.0
	}

	if err := d.client.Swipe(fromX, fromY, toX, toY, duration); err != nil {
		return errorResult(err, "Swipe failed")
	}

	return successResult("Swipe completed", nil)
}

// Navigation commands

func (d *Driver) back(step *flow.BackStep) *core.CommandResult {
	// iOS doesn't have a hardware back button
	// Could try to find a back button in the UI
	return errorResult(fmt.Errorf("back not supported on iOS"), "iOS doesn't have a back button")
}

func (d *Driver) pressKey(step *flow.PressKeyStep) *core.CommandResult {
	switch strings.ToLower(step.Key) {
	case "home":
		if err := d.client.Home(); err != nil {
			return errorResult(err, "Press home failed")
		}
	case "volumeup", "volume_up":
		if err := d.client.PressButton("volumeUp"); err != nil {
			return errorResult(err, "Press volume up failed")
		}
	case "volumedown", "volume_down":
		if err := d.client.PressButton("volumeDown"); err != nil {
			return errorResult(err, "Press volume down failed")
		}
	default:
		// Try keyboard key
		if keyChar := iosKeyboardKey(step.Key); keyChar != "" {
			if err := d.client.SendKeys(keyChar); err != nil {
				return errorResult(err, fmt.Sprintf("Press %s failed", step.Key))
			}
		} else {
			return errorResult(fmt.Errorf("unknown key: %s", step.Key), "Unknown key")
		}
	}

	return successResult(fmt.Sprintf("Pressed %s", step.Key), nil)
}

// iosKeyboardKey maps keyboard key names to the character to send via WDA SendKeys.
// Returns empty string if the name is not a recognized keyboard key.
func iosKeyboardKey(name string) string {
	switch strings.ToLower(name) {
	case "return", "enter":
		return "\n"
	case "tab":
		return "\t"
	case "delete", "backspace":
		return "\b"
	case "space":
		return " "
	default:
		return ""
	}
}

// App lifecycle

func (d *Driver) launchApp(step *flow.LaunchAppStep) *core.CommandResult {
	bundleID := step.AppID
	if bundleID == "" {
		return errorResult(fmt.Errorf("bundleID required"), "Bundle ID is required for launchApp")
	}

	// Clear state if requested (uninstall + reinstall)
	if step.ClearState {
		_ = d.client.TerminateApp(bundleID)
		if result := d.clearAppState(bundleID); !result.Success {
			return result
		}
	}

	// Apply permissions (default: all allow, like Maestro)
	// Only works on simulators with UDID
	if d.udid != "" {
		permissions := step.Permissions
		if len(permissions) == 0 {
			permissions = map[string]string{"all": "allow"}
		}
		for name, value := range permissions {
			if strings.ToLower(name) == "all" {
				for _, perm := range getIOSPermissions() {
					_ = d.applyIOSPermission(bundleID, perm, value)
				}
			} else {
				for _, perm := range resolveIOSPermissionShortcut(name) {
					_ = d.applyIOSPermission(bundleID, perm, value)
				}
			}
		}
	}

	// If no session exists, create one (which also launches the app)
	if !d.client.HasSession() {
		if err := d.client.CreateSession(bundleID); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to create session for app: %s", bundleID))
		}
		// Disable quiescence wait to prevent XCTest crashes on certain Xcode/iOS versions
		_ = d.client.DisableQuiescence()
		time.Sleep(time.Second) // Brief wait for app to start
		return successResult(fmt.Sprintf("Launched app: %s", bundleID), nil)
	}

	// Convert arguments map to iOS launch arguments format
	var launchArgs []string
	var launchEnv map[string]string
	if len(step.Arguments) > 0 {
		launchEnv = make(map[string]string)
		for key, value := range step.Arguments {
			// iOS arguments: pass as -key value pairs for command line args
			// or as environment variables
			switch v := value.(type) {
			case string:
				launchArgs = append(launchArgs, fmt.Sprintf("-%s", key), v)
			case bool:
				if v {
					launchArgs = append(launchArgs, fmt.Sprintf("-%s", key), "true")
				} else {
					launchArgs = append(launchArgs, fmt.Sprintf("-%s", key), "false")
				}
			default:
				launchArgs = append(launchArgs, fmt.Sprintf("-%s", key), fmt.Sprint(v))
			}
		}
	}

	// Session exists - use LaunchApp to launch/relaunch the app
	if err := d.client.LaunchAppWithArgs(bundleID, launchArgs, launchEnv); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to launch app: %s", bundleID))
	}

	time.Sleep(time.Second) // Brief wait for app to start

	return successResult(fmt.Sprintf("Launched app: %s", bundleID), nil)
}

func (d *Driver) stopApp(step *flow.StopAppStep) *core.CommandResult {
	bundleID := step.AppID
	if bundleID == "" {
		return errorResult(fmt.Errorf("bundleID required"), "Bundle ID is required for stopApp")
	}

	if err := d.client.TerminateApp(bundleID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to stop app: %s", bundleID))
	}

	return successResult(fmt.Sprintf("Stopped app: %s", bundleID), nil)
}

func (d *Driver) killApp(step *flow.KillAppStep) *core.CommandResult {
	bundleID := step.AppID
	if bundleID == "" {
		return errorResult(fmt.Errorf("bundleID required"), "Bundle ID is required for killApp")
	}

	if err := d.client.TerminateApp(bundleID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to kill app: %s", bundleID))
	}

	return successResult(fmt.Sprintf("Killed app: %s", bundleID), nil)
}

func (d *Driver) clearState(step *flow.ClearStateStep) *core.CommandResult {
	bundleID := step.AppID
	if bundleID == "" {
		return errorResult(fmt.Errorf("bundleID required"), "Bundle ID is required for clearState")
	}

	// Terminate app first
	_ = d.client.TerminateApp(bundleID)

	return d.clearAppState(bundleID)
}

// clearAppState uninstalls and reinstalls an app to clear its state.
// Requires --app-file for both simulators and real devices.
// On simulators, uses simctl. On physical devices, uses go-ios.
func (d *Driver) clearAppState(bundleID string) *core.CommandResult {
	if d.appFile == "" {
		return errorResult(fmt.Errorf("clearState on iOS requires --app-file for reinstall"),
			"clearState on iOS requires --app-file to reinstall the app after uninstalling")
	}

	if d.info.IsSimulator {
		return d.clearAppStateSimulator(bundleID)
	}
	return d.clearAppStateDevice(bundleID)
}

func (d *Driver) clearAppStateSimulator(bundleID string) *core.CommandResult {
	cmd := exec.Command("xcrun", "simctl", "uninstall", d.udid, bundleID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errorResult(fmt.Errorf("simctl uninstall failed: %w: %s", err, string(output)),
			"Failed to uninstall app on simulator")
	}

	cmd = exec.Command("xcrun", "simctl", "install", d.udid, d.appFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errorResult(fmt.Errorf("simctl install failed: %w: %s", err, string(output)),
			"Failed to reinstall app on simulator")
	}

	return successResult(fmt.Sprintf("Cleared state for %s (uninstall+reinstall)", bundleID), nil)
}

func (d *Driver) clearAppStateDevice(bundleID string) *core.CommandResult {
	entry, err := goios.GetDevice(d.udid)
	if err != nil {
		return errorResult(fmt.Errorf("device %s not found: %w", d.udid, err),
			"Failed to connect to device for uninstall")
	}

	conn, err := installationproxy.New(entry)
	if err != nil {
		return errorResult(fmt.Errorf("failed to connect to installation service: %w", err),
			"Failed to connect to device installation service")
	}
	defer conn.Close()

	if err := conn.Uninstall(bundleID); err != nil {
		return errorResult(fmt.Errorf("failed to uninstall %s: %w", bundleID, err),
			"Failed to uninstall app")
	}

	zcConn, err := zipconduit.New(entry)
	if err != nil {
		return errorResult(fmt.Errorf("failed to connect to install service: %w", err),
			"Failed to connect to device for reinstall")
	}
	if err := zcConn.SendFile(d.appFile); err != nil {
		return errorResult(fmt.Errorf("failed to reinstall app: %w", err),
			"Failed to reinstall app after uninstall")
	}

	return successResult(fmt.Sprintf("Cleared state for %s (uninstall+reinstall)", bundleID), nil)
}

// Clipboard

func (d *Driver) copyTextFrom(step *flow.CopyTextFromStep) *core.CommandResult {
	info, err := d.findElement(step.Selector, false, step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %s", selectorDesc(step.Selector)))
	}

	return &core.CommandResult{
		Success: true,
		Message: fmt.Sprintf("Copied text: %s", info.Text),
		Data:    info.Text,
		Element: info,
	}
}

func (d *Driver) pasteText(step *flow.PasteTextStep) *core.CommandResult {
	// iOS: Need to use clipboard API via simctl or device APIs
	// WDA doesn't directly support clipboard operations
	return errorResult(fmt.Errorf("pasteText not supported via WDA"), "Paste requires clipboard access")
}

func (d *Driver) setClipboard(step *flow.SetClipboardStep) *core.CommandResult {
	// iOS: WDA doesn't directly support clipboard operations
	// For simulators, could use: xcrun simctl pbcopy <booted|udid>
	// For real devices, would need a helper app
	return errorResult(fmt.Errorf("setClipboard not supported via WDA"),
		"iOS clipboard operations require simctl (simulator) or a helper app (device)")
}

// Device control

func (d *Driver) setOrientation(step *flow.SetOrientationStep) *core.CommandResult {
	orientation := step.Orientation
	switch orientation {
	case "portrait":
		orientation = "PORTRAIT"
	case "landscape":
		orientation = "LANDSCAPE"
	}

	if err := d.client.SetOrientation(orientation); err != nil {
		return errorResult(err, "Set orientation failed")
	}

	return successResult(fmt.Sprintf("Set orientation to %s", step.Orientation), nil)
}

func (d *Driver) openLink(step *flow.OpenLinkStep) *core.CommandResult {
	link := step.Link
	if link == "" {
		return errorResult(fmt.Errorf("no link specified"), "No link to open")
	}

	// Use WDA deep link - works for both simulator and real device
	// Note: browser parameter would require launching Safari explicitly
	// WDA's DeepLink uses the system handler which respects app associations
	if err := d.client.DeepLink(link); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open link: %s", link))
	}

	// If autoVerify is enabled, wait briefly for page load
	if step.AutoVerify != nil && *step.AutoVerify {
		time.Sleep(2 * time.Second)
	}

	msg := fmt.Sprintf("Opened link: %s", link)
	if step.Browser != nil && *step.Browser {
		msg += " (browser flag set, but WDA uses system default handler)"
	}
	return successResult(msg, nil)
}

func (d *Driver) openBrowser(step *flow.OpenBrowserStep) *core.CommandResult {
	url := step.URL
	if url == "" {
		return errorResult(fmt.Errorf("no URL specified"), "No URL to open")
	}

	// Use WDA deep link - opens in Safari for http/https URLs
	if err := d.client.DeepLink(url); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open browser: %s", url))
	}

	return successResult(fmt.Sprintf("Opened browser: %s", url), nil)
}

// Wait commands

func (d *Driver) waitUntil(step *flow.WaitUntilStep) *core.CommandResult {
	timeoutMs := step.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = DefaultFindTimeout
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Determine selector for error messages
	var selector *flow.Selector
	waitingForVisible := step.Visible != nil
	if waitingForVisible {
		selector = step.Visible
	} else {
		selector = step.NotVisible
	}

	for {
		select {
		case <-ctx.Done():
			// Clean, clear error message with timeout value
			if waitingForVisible {
				return errorResult(
					context.DeadlineExceeded,
					fmt.Sprintf("Element '%s' not visible within %v", selector.Describe(), timeout),
				)
			}
			return errorResult(
				context.DeadlineExceeded,
				fmt.Sprintf("Element '%s' still visible after %v", selector.Describe(), timeout),
			)
		default:
			if waitingForVisible {
				// Single attempt - context controls overall timeout
				info, err := d.findElementOnce(*step.Visible)
				if err == nil && info != nil {
					return successResult("Element became visible", info)
				}
			} else {
				// Single attempt for not visible check
				info, err := d.findElementOnce(*step.NotVisible)
				if err != nil || info == nil {
					return successResult("Element became not visible", nil)
				}
			}
			// HTTP round-trip (~100ms) is natural rate limit, no sleep needed
		}
	}
}

func (d *Driver) waitForAnimationToEnd(_ *flow.WaitForAnimationToEndStep) *core.CommandResult {
	// NOTE: waitForAnimationToEnd is not fully implemented.
	// Maestro uses screenshot comparison which is complex to implement correctly.
	// For now, we pass this step with a warning.
	return &core.CommandResult{
		Success: true,
		Message: "WARNING: waitForAnimationToEnd is not fully implemented - step passed without animation check",
	}
}

// Media

func (d *Driver) takeScreenshot(step *flow.TakeScreenshotStep) *core.CommandResult {
	data, err := d.client.Screenshot()
	if err != nil {
		return errorResult(err, "Screenshot failed")
	}

	return &core.CommandResult{
		Success: true,
		Message: "Screenshot captured",
		Data:    data,
	}
}

// Helper functions

func selectorDesc(sel flow.Selector) string {
	if sel.Text != "" {
		return fmt.Sprintf("text='%s'", sel.Text)
	}
	if sel.ID != "" {
		return fmt.Sprintf("id='%s'", sel.ID)
	}
	return "selector"
}

func randomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

func randomEmail() string {
	user := randomString(8)
	domains := []string{"example.com", "test.com", "mail.com"}
	domain := domains[rand.Intn(len(domains))]
	return user + "@" + domain
}

func randomNumber(length int) string {
	const digits = "0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = digits[rand.Intn(len(digits))]
	}
	return string(b)
}

func randomPersonName() string {
	firstNames := []string{"John", "Jane", "Michael", "Emily", "David", "Sarah", "James", "Emma", "Robert", "Olivia"}
	lastNames := []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis", "Rodriguez", "Martinez"}
	return firstNames[rand.Intn(len(firstNames))] + " " + lastNames[rand.Intn(len(lastNames))]
}

func parsePercentageCoords(coord string) (float64, float64, error) {
	// Parse "50%, 50%" format
	coord = strings.ReplaceAll(coord, " ", "")
	parts := strings.Split(coord, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid coordinate format: %s", coord)
	}

	xStr := strings.TrimSuffix(parts[0], "%")
	yStr := strings.TrimSuffix(parts[1], "%")

	x, err := strconv.ParseFloat(xStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid x coordinate: %s", parts[0])
	}

	y, err := strconv.ParseFloat(yStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid y coordinate: %s", parts[1])
	}

	return x / 100.0, y / 100.0, nil
}

// setPermissions sets app permissions using xcrun simctl privacy (iOS simulator only).
func (d *Driver) setPermissions(step *flow.SetPermissionsStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		return errorResult(fmt.Errorf("no appId specified"), "No app ID for permissions")
	}

	if d.udid == "" {
		return &core.CommandResult{
			Success: true,
			Message: "setPermissions skipped (no UDID - permissions require simulator)",
		}
	}

	if len(step.Permissions) == 0 {
		return errorResult(fmt.Errorf("no permissions specified"), "No permissions to set")
	}

	var granted, revoked, errors []string

	for name, value := range step.Permissions {
		if strings.ToLower(name) == "all" {
			// Apply to all common iOS permissions
			allPerms := getIOSPermissions()
			for _, perm := range allPerms {
				if err := d.applyIOSPermission(appID, perm, value); err != nil {
					errors = append(errors, fmt.Sprintf("%s: %v", perm, err))
				} else if strings.ToLower(value) == "allow" {
					granted = append(granted, perm)
				} else {
					revoked = append(revoked, perm)
				}
			}
			continue
		}

		// Map shortcut to iOS permission service name
		perms := resolveIOSPermissionShortcut(name)
		for _, perm := range perms {
			if err := d.applyIOSPermission(appID, perm, value); err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", perm, err))
			} else if strings.ToLower(value) == "allow" {
				granted = append(granted, perm)
			} else {
				revoked = append(revoked, perm)
			}
		}
	}

	msg := fmt.Sprintf("Permissions set: %d granted, %d revoked", len(granted), len(revoked))
	if len(errors) > 0 {
		msg += fmt.Sprintf(", %d errors", len(errors))
	}

	return &core.CommandResult{
		Success: true,
		Message: msg,
	}
}

// applyIOSPermission grants or revokes a single permission using xcrun simctl privacy.
func (d *Driver) applyIOSPermission(appID, permission, value string) error {
	var action string
	switch strings.ToLower(value) {
	case "allow":
		action = "grant"
	case "deny":
		action = "revoke"
	case "unset":
		action = "reset"
	default:
		return fmt.Errorf("invalid permission value: %s (use allow/deny/unset)", value)
	}

	// xcrun simctl privacy <device> <action> <service> <bundle-id>
	cmd := exec.Command("xcrun", "simctl", "privacy", d.udid, action, permission, appID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// resolveIOSPermissionShortcut maps shortcut names to iOS privacy service names.
func resolveIOSPermissionShortcut(shortcut string) []string {
	switch strings.ToLower(shortcut) {
	case "location", "location-always":
		return []string{"location-always"}
	case "camera":
		return []string{"camera"}
	case "contacts":
		return []string{"contacts"}
	case "phone":
		return []string{"contacts"} // iOS doesn't have separate phone permission
	case "microphone":
		return []string{"microphone"}
	case "photos", "medialibrary":
		return []string{"photos"}
	case "calendar":
		return []string{"calendar"}
	case "reminders":
		return []string{"reminders"}
	case "notifications":
		return []string{"notifications"}
	case "bluetooth":
		return []string{"bluetooth-peripheral"}
	case "health":
		return []string{"health"}
	case "homekit":
		return []string{"homekit"}
	case "motion":
		return []string{"motion"}
	case "speech":
		return []string{"speech-recognition"}
	case "siri":
		return []string{"siri"}
	case "faceid":
		return []string{"faceid"}
	default:
		// Assume it's already a valid service name
		return []string{shortcut}
	}
}

// getIOSPermissions returns all common iOS privacy services.
func getIOSPermissions() []string {
	return []string{
		"location-always",
		"camera",
		"microphone",
		"photos",
		"contacts",
		"calendar",
		"reminders",
		"notifications",
	}
}
