package wda

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// Tap commands

func (d *Driver) tapOn(step *flow.TapOnStep) *core.CommandResult {
	// Check if using Point WITHOUT selector (screen-relative tap)
	if step.Point != "" && step.Selector.IsEmpty() {
		return d.tapOnPointWithCoords(step.Point)
	}

	// Handle keyboard key names — iOS keyboard buttons aren't reliably findable via WDA
	if step.Selector.Text != "" {
		if keyChar := iosKeyboardKey(step.Selector.Text); keyChar != "" {
			if err := d.client.SendKeys(keyChar, 0); err != nil {
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
		px, py, parseErr := core.ParsePointCoords(step.Point, info.Bounds.Width, info.Bounds.Height)
		if parseErr != nil {
			return errorResult(parseErr, "Invalid point coordinates")
		}
		x := float64(info.Bounds.X + px)
		y := float64(info.Bounds.Y + py)
		if err := d.client.Tap(x, y); err != nil {
			return errorResult(err, "Tap at relative point failed")
		}
		return successResult(fmt.Sprintf("Tapped at relative point (%.0f, %.0f) on element", x, y), info)
	}

	// If duration is set (or longPress: true), hold the press for that long.
	if step.DurationMs > 0 || step.LongPress {
		durationSec := float64(step.DurationMs) / 1000.0
		if durationSec <= 0 {
			durationSec = 1.0
		}
		x := float64(info.Bounds.X + info.Bounds.Width/2)
		y := float64(info.Bounds.Y + info.Bounds.Height/2)
		if err := d.client.LongPress(x, y, durationSec); err != nil {
			return errorResult(err, fmt.Sprintf("Press for %.2fs failed", durationSec))
		}
		return successResult("Pressed element", info)
	}

	// Determine if element is a text field (needs focus verification)
	isTextField := strings.Contains(info.Class, "TextField")

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

// tapOnPointWithCoords handles point-based tap with either percentage ("85%, 50%") or absolute ("123, 456") coordinates.
func (d *Driver) tapOnPointWithCoords(point string) *core.CommandResult {
	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}

	x, y, err := core.ParsePointCoords(point, width, height)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid point coordinates: %s", point))
	}

	if err := d.client.Tap(float64(x), float64(y)); err != nil {
		return errorResult(err, "Tap at point failed")
	}

	return successResult(fmt.Sprintf("Tapped at (%d, %d)", x, y), nil)
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

	duration := float64(step.DurationMs) / 1000.0
	if duration <= 0 {
		duration = 1.0 // default 1 second
	}

	if err := d.client.LongPress(x, y, duration); err != nil {
		return errorResult(err, "Long press failed")
	}

	return successResult("Long pressed element", info)
}

func (d *Driver) tapOnPoint(step *flow.TapOnPointStep) *core.CommandResult {
	var x, y float64

	// Handle Point field (percentage or absolute coordinates)
	if step.Point != "" {
		width, height, err := d.screenSize()
		if err != nil {
			return errorResult(err, "Failed to get screen size")
		}
		px, py, err := core.ParsePointCoords(step.Point, width, height)
		if err != nil {
			return errorResult(err, "Invalid point format")
		}
		x = float64(px)
		y = float64(py)
	} else {
		x = float64(step.X)
		y = float64(step.Y)
	}

	if step.DurationMs > 0 || step.LongPress {
		durationSec := float64(step.DurationMs) / 1000.0
		if durationSec <= 0 {
			durationSec = 1.0
		}
		if err := d.client.LongPress(x, y, durationSec); err != nil {
			return errorResult(err, fmt.Sprintf("Press at point for %.2fs failed", durationSec))
		}
		return successResult(fmt.Sprintf("Pressed at (%.0f, %.0f)", x, y), nil)
	}

	if err := d.client.Tap(x, y); err != nil {
		return errorResult(err, "Tap on point failed")
	}

	return successResult(fmt.Sprintf("Tapped at (%.0f, %.0f)", x, y), nil)
}

// Assert commands

func (d *Driver) assertVisible(step *flow.AssertVisibleStep) *core.CommandResult {
	info, err := d.findElement(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not visible: %s", selectorDesc(step.Selector)))
	}

	msg := "Element is visible"
	if info != nil && info.MatchNote != "" {
		msg = "Element is visible (" + info.MatchNote + ")"
	}
	return successResult(msg, info)
}

func (d *Driver) assertNotVisible(step *flow.AssertNotVisibleStep) *core.CommandResult {
	// Poll with quick checks, waiting for element to disappear.
	// Each check is a single lookup (no retries). If element is not found
	// at any point, we pass immediately. If still visible at timeout, fail.
	timeoutMs := step.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	pollInterval := 500 * time.Millisecond

	for {
		info, err := d.findElementOnce(step.Selector)
		if err != nil || info == nil {
			return successResult("Element is not visible", nil)
		}

		if time.Now().After(deadline) {
			return errorResult(fmt.Errorf("element is visible"), fmt.Sprintf("Element should not be visible: %s", selectorDesc(step.Selector)))
		}

		time.Sleep(pollInterval)
	}
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
			if err := d.client.ElementSendKeys(info.ID, text, d.typingFrequency); err != nil {
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

	if err := d.client.SendKeys(text, d.typingFrequency); err != nil {
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
						if sendErr := d.client.SendKeys(remaining, d.typingFrequency); sendErr == nil {
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
	if err := d.client.SendKeys(deleteStr, d.typingFrequency); err != nil {
		return errorResult(err, "Erase text failed")
	}

	return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
}

func (d *Driver) hideKeyboard(step *flow.HideKeyboardStep) *core.CommandResult {
	// iOS: tap outside to dismiss keyboard, or press Done button
	// Try pressing the "return" key (ignore error - keyboard might not be visible)
	_ = d.client.SendKeys("\n", 0)

	return successResult("Attempted to hide keyboard", nil)
}

func (d *Driver) acceptAlert(step *flow.AcceptAlertStep) *core.CommandResult {
	return d.waitForAlert(step.TimeoutMs, true)
}

func (d *Driver) dismissAlert(step *flow.DismissAlertStep) *core.CommandResult {
	return d.waitForAlert(step.TimeoutMs, false)
}

// waitForAlert polls for a system alert and accepts/dismisses it.
// If no alert appears within the timeout, succeeds silently.
func (d *Driver) waitForAlert(timeoutMs int, accept bool) *core.CommandResult {
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
	defer cancel()

	action := "accept"
	if !accept {
		action = "dismiss"
	}

	for {
		select {
		case <-ctx.Done():
			return successResult(fmt.Sprintf("No alert to %s", action), nil)
		default:
			var err error
			if accept {
				err = d.client.AcceptAlert()
			} else {
				err = d.client.DismissAlert()
			}
			if err == nil {
				return successResult(fmt.Sprintf("Alert %sed", action), nil)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
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

	if err := d.client.SendKeys(text, d.typingFrequency); err != nil {
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
	width, height, err := d.screenSize()
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
	dir := strings.ToLower(step.Direction)
	switch dir {
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
	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "down"
	}

	maxScrolls := 20
	if step.MaxScrolls > 0 {
		maxScrolls = step.MaxScrolls
	}
	timeout := 30 * time.Second
	if step.TimeoutMs > 0 {
		timeout = time.Duration(step.TimeoutMs) * time.Millisecond
	}
	deadline := time.Now().Add(timeout)

	for i := 0; i < maxScrolls && time.Now().Before(deadline); i++ {
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
	width, height, err := d.screenSize()
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

		// Swipe coordinates match Maestro iOS behavior:
		// LEFT:  90%→10% of width,  centered vertically
		// RIGHT: 10%→90% of width,  centered vertically
		// UP:    centered horizontally, 90%→10% of height
		// DOWN:  centered horizontally, 20%→90% of height
		dir := strings.ToLower(step.Direction)
		switch dir {
		case "up":
			fromX = areaX + areaW*0.5
			fromY = areaY + areaH*0.9
			toX = areaX + areaW*0.5
			toY = areaY + areaH*0.1
		case "down":
			fromX = areaX + areaW*0.5
			fromY = areaY + areaH*0.2
			toX = areaX + areaW*0.5
			toY = areaY + areaH*0.9
		case "left":
			fromX = areaX + areaW*0.9
			fromY = areaY + areaH*0.5
			toX = areaX + areaW*0.1
			toY = areaY + areaH*0.5
		case "right":
			fromX = areaX + areaW*0.1
			fromY = areaY + areaH*0.5
			toX = areaX + areaW*0.9
			toY = areaY + areaH*0.5
		default:
			return errorResult(fmt.Errorf("invalid direction: %s", step.Direction), "Invalid swipe direction")
		}
	}

	duration := 0.1
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
			if err := d.client.SendKeys(keyChar, 0); err != nil {
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

	// Clear state and apply permissions
	// On simulator: overlap uninstall+install with permission resets for speed
	permissions := step.Permissions
	if d.udid != "" && len(permissions) == 0 {
		permissions = map[string]string{"all": "allow"}
	}
	needPerms := d.udid != "" && d.info.IsSimulator && !hasAllValue(permissions, "unset")

	if step.ClearState {
		_ = d.terminateApp(bundleID)

		if needPerms {
			// Run uninstall+install and permission resets concurrently
			allPerms := getIOSPermissions()
			var wg sync.WaitGroup
			var clearResult *core.CommandResult

			wg.Add(1)
			go func() {
				defer wg.Done()
				clearResult = d.clearAppState(bundleID)
			}()

			wg.Add(len(allPerms))
			for _, perm := range allPerms {
				go func(p string) {
					defer wg.Done()
					_ = d.resetIOSPermission(bundleID, p)
				}(perm)
			}
			wg.Wait()

			if clearResult != nil && !clearResult.Success {
				return clearResult
			}

			// Grant permissions after install completes
			var applyList []struct{ perm, action string }
			for name, value := range permissions {
				lower := strings.ToLower(value)
				if lower != "allow" && lower != "deny" {
					continue
				}
				if strings.ToLower(name) == "all" {
					for _, perm := range allPerms {
						applyList = append(applyList, struct{ perm, action string }{perm, lower})
					}
				} else {
					for _, perm := range resolveIOSPermissionShortcut(name) {
						applyList = append(applyList, struct{ perm, action string }{perm, lower})
					}
				}
			}
			wg.Add(len(applyList))
			for _, item := range applyList {
				go func(p, a string) {
					defer wg.Done()
					_ = d.applyIOSPermission(bundleID, p, a)
				}(item.perm, item.action)
			}
			wg.Wait()
		} else {
			if result := d.clearAppState(bundleID); !result.Success {
				return result
			}
		}
	} else if needPerms {
		allPerms := getIOSPermissions()
		var wg sync.WaitGroup
		wg.Add(len(allPerms))
		for _, perm := range allPerms {
			go func(p string) {
				defer wg.Done()
				_ = d.resetIOSPermission(bundleID, p)
			}(perm)
		}
		wg.Wait()

		var applyList []struct{ perm, action string }
		for name, value := range permissions {
			lower := strings.ToLower(value)
			if lower != "allow" && lower != "deny" {
				continue
			}
			if strings.ToLower(name) == "all" {
				for _, perm := range allPerms {
					applyList = append(applyList, struct{ perm, action string }{perm, lower})
				}
			} else {
				for _, perm := range resolveIOSPermissionShortcut(name) {
					applyList = append(applyList, struct{ perm, action string }{perm, lower})
				}
			}
		}
		wg.Add(len(applyList))
		for _, item := range applyList {
			go func(p, a string) {
				defer wg.Done()
				_ = d.applyIOSPermission(bundleID, p, a)
			}(item.perm, item.action)
		}
		wg.Wait()
	}

	// Reset simulator keychain if requested — after clearState so a reinstall
	// can't race, before launch so the app starts with a clean keychain.
	// No-op with a warning on real devices (keychain can't be reset there).
	if step.ClearKeychain {
		if r := d.resetKeychain(); !r.Success {
			logger.Warn("launchApp: clearKeychain skipped: %s", r.Message)
		}
	}

	if d.udid != "" {
		d.alertAction = resolveAlertAction(permissions)
	}

	// Convert arguments map to iOS launch arguments format
	var launchArgs []string
	launchEnv := make(map[string]string)

	// Populate launchEnv from environment field
	for key, value := range step.Environment {
		launchEnv[key] = value
	}

	// Populate launchArgs and launchEnv from arguments field
	if len(step.Arguments) > 0 {
		for key, value := range step.Arguments {
			var strVal string
			switch v := value.(type) {
			case string:
				strVal = v
			case bool:
				if v {
					strVal = "true"
				} else {
					strVal = "false"
				}
			default:
				strVal = fmt.Sprint(v)
			}
			launchArgs = append(launchArgs, fmt.Sprintf("-%s", key), strVal)
			// Also set as environment variable so the app can read via ProcessInfo.environment
			launchEnv[key] = strVal
		}
	}

	hasArgs := len(launchArgs) > 0 || len(launchEnv) > 0

	// If no session exists, create one (which also launches the app)
	newSession := false
	if !d.client.HasSession() {
		if err := d.client.CreateSession(bundleID, d.alertAction); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to create session for app: %s", bundleID))
		}
		newSession = true
	}

	// Always update settings — ensures alert config is correct even when
	// reusing a session from a previous flow with different permissions.
	sessionSettings := map[string]interface{}{
		"shouldWaitForQuiescence": false,
		"waitForIdleTimeout":      0,
		"defaultAlertAction":      d.alertAction,
	}
	if d.alertAction == "accept" {
		sessionSettings["acceptAlertButtonSelector"] = "**/XCUIElementTypeButton[`label BEGINSWITH[c] 'Allow' OR label ==[c] 'OK'`]"
	} else if d.alertAction == "dismiss" {
		sessionSettings["dismissAlertButtonSelector"] = "**/XCUIElementTypeButton[`label CONTAINS[c] 'Don't Allow' OR label CONTAINS[c] 'Dont Allow'`]"
	}
	_ = d.client.UpdateSettings(sessionSettings)

	// If we just created a session and no args needed, we're done
	// (CreateSession already launched the app)
	if newSession && !hasArgs {
		return successResult(fmt.Sprintf("Launched app: %s", bundleID), nil)
	}

	// Terminate the app first so WDA calls launch (not activate),
	// which is required for arguments/environment to take effect
	if hasArgs {
		_ = d.client.TerminateApp(bundleID)
	}

	// Launch the app (with or without args)
	if err := d.client.LaunchAppWithArgs(bundleID, launchArgs, launchEnv); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to launch app: %s", bundleID))
	}

	return successResult(fmt.Sprintf("Launched app: %s", bundleID), nil)
}

func (d *Driver) stopApp(step *flow.StopAppStep) *core.CommandResult {
	bundleID := step.AppID
	if bundleID == "" {
		return errorResult(fmt.Errorf("bundleID required"), "Bundle ID is required for stopApp")
	}

	if err := d.terminateApp(bundleID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to stop app: %s", bundleID))
	}

	return successResult(fmt.Sprintf("Stopped app: %s", bundleID), nil)
}

func (d *Driver) killApp(step *flow.KillAppStep) *core.CommandResult {
	bundleID := step.AppID
	if bundleID == "" {
		return errorResult(fmt.Errorf("bundleID required"), "Bundle ID is required for killApp")
	}

	if err := d.terminateApp(bundleID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to kill app: %s", bundleID))
	}

	return successResult(fmt.Sprintf("Killed app: %s", bundleID), nil)
}

// terminateApp terminates an app via WDA session if available, otherwise falls back
// to xcrun simctl terminate (simulators) or devicectl (real devices).
// This handles the case where stopApp/killApp is called before any launchApp
// (e.g. "- stopApp" as the first step in a flow).
func (d *Driver) terminateApp(bundleID string) error {
	if d.client.HasSession() {
		return d.client.TerminateApp(bundleID)
	}

	// No WDA session — fall back to device-level terminate
	if d.info != nil && d.info.IsSimulator {
		cmd := exec.Command("xcrun", "simctl", "terminate", d.udid, bundleID)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Ignore errors — app might not be running
			logger.Debug("simctl terminate %s: %v: %s", bundleID, err, string(output))
		}
		return nil
	}

	// Real device without session — nothing we can do, succeed silently.
	// The next launchApp will create a session and handle it.
	logger.Debug("no WDA session for terminateApp on real device, skipping")
	return nil
}

func (d *Driver) clearState(step *flow.ClearStateStep) *core.CommandResult {
	bundleID := step.AppID
	if bundleID == "" {
		return errorResult(fmt.Errorf("bundleID required"), "Bundle ID is required for clearState")
	}

	// Terminate app first
	_ = d.terminateApp(bundleID)

	return d.clearAppState(bundleID)
}

// clearAppState uninstalls and reinstalls an app to clear its state.
//   - Simulator: auto-discovers the installed .app via `simctl get_app_container`
//     when --app-file isn't provided (or when it points inside the live sim
//     container, which would be deleted by the uninstall step). Matches
//     Maestro CLI's auto-stage behavior so flows like `- clearState` work
//     without any extra flags.
//   - Real device: --app-file is still required (Apple seals the .app on
//     device, no public API to extract it back to the host).
func (d *Driver) clearAppState(bundleID string) *core.CommandResult {
	if d.info.IsSimulator {
		return d.clearAppStateSimulator(bundleID)
	}
	if d.appFile == "" {
		return errorResult(fmt.Errorf("clearState on real iOS devices requires --app-file"),
			"clearState on real iOS devices requires --app-file — the installed bundle "+
				"isn't reachable from the host. On simulators no flag is needed.\n"+
				"Usage: maestro-runner --app-file <path-to-ipa-or-app> --platform ios test <flow-files>")
	}
	return d.clearAppStateDevice(bundleID)
}

func (d *Driver) clearAppStateSimulator(bundleID string) *core.CommandResult {
	appFile, cleanup, err := d.resolveSimAppFile(bundleID)
	if err != nil {
		return errorResult(err, fmt.Sprintf("clearState: %v", err))
	}
	if cleanup != nil {
		defer cleanup()
	}

	cmd := exec.Command("xcrun", "simctl", "uninstall", d.udid, bundleID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errorResult(fmt.Errorf("simctl uninstall failed: %w: %s", err, string(output)),
			"Failed to uninstall app on simulator")
	}

	cmd = exec.Command("xcrun", "simctl", "install", d.udid, appFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errorResult(fmt.Errorf("simctl install failed: %w: %s", err, string(output)),
			"Failed to reinstall app on simulator")
	}

	return successResult(fmt.Sprintf("Cleared state for %s (uninstall+reinstall)", bundleID), nil)
}

// resolveSimAppFile returns the .app path to feed into `simctl install` after
// uninstall, plus an optional cleanup closure for any temp staging directory.
//
// The selection logic:
//  1. --app-file points to an external location (not inside this sim's
//     container) → use it as-is, no staging. Same speed as before.
//  2. --app-file is empty OR points inside /CoreSimulator/Devices/<udid>/ →
//     discover the installed .app via `simctl get_app_container <udid> <id>`
//     and stage a copy to a temp dir. The uninstall step would otherwise
//     delete the path before install reads it.
//
// Staging uses APFS clone (`cp -c`) — sim container and `os.MkdirTemp` both
// land on the same APFS volume, so the copy is effectively zero-cost. Falls
// back to `cp -R` if clone isn't supported.
func (d *Driver) resolveSimAppFile(bundleID string) (string, func(), error) {
	containerMarker := fmt.Sprintf("/CoreSimulator/Devices/%s/", d.udid)
	pointsAtLiveContainer := d.appFile != "" && strings.Contains(d.appFile, containerMarker)

	if d.appFile != "" && !pointsAtLiveContainer {
		return d.appFile, nil, nil
	}

	installed, err := d.simulatorInstalledAppPath(bundleID)
	if err != nil {
		if d.appFile == "" {
			return "", nil, fmt.Errorf("could not locate installed app for %s on simulator: %w (provide --app-file or install the app first)", bundleID, err)
		}
		// Fall back to the user-supplied --app-file even though it looked
		// like a container path; let `simctl install` produce the actual
		// error if it really is unreachable.
		return d.appFile, nil, nil
	}

	tmpDir, err := os.MkdirTemp("", "maestro-runner-clearstate-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir for clearState staging: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	staged := filepath.Join(tmpDir, filepath.Base(installed))
	if err := stageAppBundle(installed, staged); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage app bundle for clearState: %w", err)
	}
	return staged, cleanup, nil
}

// simulatorInstalledAppPath returns the .app bundle path for the installed
// app, via `xcrun simctl get_app_container <udid> <bundleID>`. Returns an
// error when the app isn't installed or simctl fails.
func (d *Driver) simulatorInstalledAppPath(bundleID string) (string, error) {
	cmd := exec.Command("xcrun", "simctl", "get_app_container", d.udid, bundleID)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("simctl get_app_container: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("simctl returned empty container path")
	}
	return path, nil
}

// stageAppBundle copies a .app bundle from src to dst, preferring APFS
// clone-on-write (cp -c) for near-zero-cost staging on modern macOS. Falls
// back to a plain recursive copy if clone isn't supported (e.g. cross-volume).
func stageAppBundle(src, dst string) error {
	if err := exec.Command("cp", "-c", "-R", src, dst).Run(); err == nil {
		return nil
	}
	if out, err := exec.Command("cp", "-R", src, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("cp -R %s %s: %w: %s", src, dst, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// clearKeychain handles the standalone clearKeychain step. Unsupported on
// real devices (iOS keychain is sandboxed and can't be reset via public API).
func (d *Driver) clearKeychain(_ *flow.ClearKeychainStep) *core.CommandResult {
	return d.resetKeychain()
}

// resetKeychain runs `xcrun simctl keychain <udid> reset` on the simulator.
// Shared by the standalone step and the launchApp clearKeychain: true option.
func (d *Driver) resetKeychain() *core.CommandResult {
	if !d.info.IsSimulator {
		return &core.CommandResult{
			Success: false,
			Error:   fmt.Errorf("clearKeychain is not supported on real iOS devices"),
			Message: "clearKeychain requires an iOS Simulator — the iOS keychain is sandboxed on real devices and cannot be reset programmatically. Use clearState to reinstall the app, which drops its keychain entries.",
		}
	}
	if d.udid == "" {
		return errorResult(fmt.Errorf("simulator UDID required"), "clearKeychain requires a booted simulator")
	}
	cmd := exec.Command("xcrun", "simctl", "keychain", d.udid, "reset")
	if output, err := cmd.CombinedOutput(); err != nil {
		return errorResult(fmt.Errorf("simctl keychain reset failed: %w: %s", err, string(output)),
			"Failed to reset simulator keychain")
	}
	return successResult("Simulator keychain reset", nil)
}

func (d *Driver) clearAppStateDevice(bundleID string) *core.CommandResult {
	// Uninstall via xcrun devicectl (uses remoted, doesn't disrupt usbmuxd port forwarding)
	cmd := exec.Command("xcrun", "devicectl", "device", "uninstall", "app",
		"--device", d.udid, bundleID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errorResult(fmt.Errorf("devicectl uninstall failed: %w: %s", err, string(output)),
			"Failed to uninstall app on device")
	}

	// Reinstall via xcrun devicectl
	cmd = exec.Command("xcrun", "devicectl", "device", "install", "app",
		"--device", d.udid, d.appFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errorResult(fmt.Errorf("devicectl install failed: %w: %s", err, string(output)),
			"Failed to reinstall app on device")
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

	if err := d.openURL(link); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open link: %s", link))
	}

	// If autoVerify is enabled, wait briefly for page load
	if step.AutoVerify != nil && *step.AutoVerify {
		time.Sleep(2 * time.Second)
	}

	msg := fmt.Sprintf("Opened link: %s", link)
	if step.Browser != nil && *step.Browser {
		msg += " (browser flag set, but the system default handler is used)"
	}
	return successResult(msg, nil)
}

// openURL dispatches a URL to the iOS device.
//
//   - Simulators: shell out to `xcrun simctl openurl <udid> <url>`. Bypasses
//     WDA entirely so deep links keep working even when the user-installed
//     WDA build (e.g. via `maestro-runner wda update` on a different version)
//     has dropped or relocated the `/url` route. Matches Maestro CLI's
//     behaviour and avoids the version-coupling between maestro-runner and
//     the WebDriverAgent endpoint shape.
//   - Real devices: continue using the WDA `/url` POST. simctl doesn't reach
//     real devices, and there's no equivalent host-side primitive for
//     deep-link delivery.
func (d *Driver) openURL(url string) error {
	if d.info != nil && d.info.IsSimulator && d.udid != "" {
		cmd := exec.Command("xcrun", "simctl", "openurl", d.udid, url)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("simctl openurl: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	return d.client.DeepLink(url)
}

func (d *Driver) openBrowser(step *flow.OpenBrowserStep) *core.CommandResult {
	url := step.URL
	if url == "" {
		return errorResult(fmt.Errorf("no URL specified"), "No URL to open")
	}

	if err := d.openURL(url); err != nil {
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

	ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
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

func (d *Driver) waitForAnimationToEnd(step *flow.WaitForAnimationToEndStep) *core.CommandResult {
	timeoutMs := step.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 15000
	}
	const threshold = 0.005

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	start := time.Now()
	for time.Now().Before(deadline) {
		prev, err := d.client.Screenshot()
		if err != nil {
			return errorResult(err, fmt.Sprintf("Screenshot failed: %v", err))
		}
		curr, err := d.client.Screenshot()
		if err != nil {
			return errorResult(err, fmt.Sprintf("Screenshot failed: %v", err))
		}
		diff := core.ImageDifference(prev, curr)
		if diff <= threshold {
			elapsed := time.Since(start)
			return successResult(fmt.Sprintf("Animation ended (%.1f%% diff, %dms)", diff*100, elapsed.Milliseconds()), nil)
		}
	}

	return successResult(fmt.Sprintf("Animation did not settle within %dms — continuing", timeoutMs), nil)
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
	return core.RandomString(length)
}

func randomEmail() string {
	return core.RandomEmail()
}

func randomNumber(length int) string {
	return core.RandomNumber(length)
}

func randomPersonName() string {
	return core.RandomPersonName()
}

func parsePercentageCoords(coord string) (float64, float64, error) {
	return core.ParsePercentageCoords(coord)
}

// Airplane mode

// findAirplaneModeSwitch activates Settings and finds the Airplane Mode switch row.
// Returns the element ID and rect. Retries up to 5 times (1s apart) for Settings load time.
// The row element's `name` is "com.apple.settings.airplaneMode", `label` is "Airplane Mode".
// The actual toggle widget is a child at the right edge of the row.
func (d *Driver) findAirplaneModeSwitch() (elemID string, x, y, w, h int, err error) {
	if activateErr := d.client.ActivateApp("com.apple.Preferences"); activateErr != nil {
		return "", 0, 0, 0, 0, fmt.Errorf("failed to open Settings: %w", activateErr)
	}
	time.Sleep(500 * time.Millisecond)

	var findErr error
	for attempt := 0; attempt < 5; attempt++ {
		elemID, findErr = d.client.FindElement("predicate string",
			"type == 'XCUIElementTypeSwitch' AND label == 'Airplane Mode'")
		if findErr == nil && elemID != "" {
			x, y, w, h, err = d.client.ElementRect(elemID)
			if err != nil {
				return "", 0, 0, 0, 0, fmt.Errorf("failed to get switch rect: %w", err)
			}
			return elemID, x, y, w, h, nil
		}
		logger.Debug("findAirplaneModeSwitch: attempt %d - not found: %v", attempt+1, findErr)
		time.Sleep(1 * time.Second)
	}
	return "", 0, 0, 0, 0, findErr
}

// tapAirplaneModeToggle taps the actual toggle widget on the right side of the
// Airplane Mode row. The FindElement matches the full-width row (x=16, w=343),
// but the physical toggle is a child at the far right (~x=282, w=63).
// ElementClick hits the row center (the label text) which doesn't flip the switch.
func (d *Driver) tapAirplaneModeToggle(x, y, w, h int) error {
	// Tap the right portion of the row where the toggle widget lives
	tapX := float64(x+w) - 30.0
	tapY := float64(y) + float64(h)/2.0
	return d.client.Tap(tapX, tapY)
}

func (d *Driver) setAirplaneMode(step *flow.SetAirplaneModeStep) *core.CommandResult {
	if d.info != nil && d.info.IsSimulator {
		return successResult("setAirplaneMode: simulators don't have airplane mode (skipped)", nil)
	}

	elemID, x, y, w, h, findErr := d.findAirplaneModeSwitch()
	if findErr != nil || elemID == "" {
		return errorResult(findErr, "Failed to find Airplane Mode switch in Settings")
	}

	value, err := d.client.ElementAttribute(elemID, "value")
	if err != nil {
		return errorResult(err, "Failed to read Airplane Mode switch state")
	}

	isOn := value == "1"
	if isOn == step.Enabled {
		state := "enabled"
		if !step.Enabled {
			state = "disabled"
		}
		return successResult(fmt.Sprintf("Airplane mode already %s", state), nil)
	}

	if err := d.tapAirplaneModeToggle(x, y, w, h); err != nil {
		return errorResult(err, "Failed to toggle Airplane Mode switch")
	}

	state := "enabled"
	if !step.Enabled {
		state = "disabled"
	}
	return successResult(fmt.Sprintf("Airplane mode %s", state), nil)
}

func (d *Driver) toggleAirplaneMode(_ *flow.ToggleAirplaneModeStep) *core.CommandResult {
	if d.info != nil && d.info.IsSimulator {
		return successResult("toggleAirplaneMode: simulators don't have airplane mode (skipped)", nil)
	}

	_, x, y, w, h, findErr := d.findAirplaneModeSwitch()
	if findErr != nil {
		return errorResult(findErr, "Failed to find Airplane Mode switch in Settings")
	}

	if err := d.tapAirplaneModeToggle(x, y, w, h); err != nil {
		return errorResult(err, "Failed to toggle Airplane Mode switch")
	}

	return successResult("Toggled airplane mode", nil)
}

// setPermissions sets app permissions.
// On simulators, uses xcrun simctl privacy. On real devices, relies on WDA's
// defaultAlertAction set at session creation time.
func (d *Driver) setPermissions(step *flow.SetPermissionsStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		return errorResult(fmt.Errorf("no appId specified"), "No app ID for permissions")
	}

	if d.udid == "" {
		return &core.CommandResult{
			Success: true,
			Message: "setPermissions skipped (no UDID)",
		}
	}

	if len(step.Permissions) == 0 {
		return errorResult(fmt.Errorf("no permissions specified"), "No permissions to set")
	}

	// Real device: permissions are handled by WDA's defaultAlertAction at session creation
	if !d.info.IsSimulator {
		action := resolveAlertAction(step.Permissions)
		if action == "" {
			logger.Warn("Mixed permissions not supported on real iOS devices — permission dialogs must be handled manually")
		}
		return &core.CommandResult{
			Success: true,
			Message: "setPermissions on real device: handled by WDA alert monitor",
		}
	}

	// Simulator: same logic as launchApp —
	//   "unset" → do nothing (hands off)
	//   otherwise → reset all, then grant only "allow" ones
	if hasAllValue(step.Permissions, "unset") {
		return &core.CommandResult{
			Success: true,
			Message: "setPermissions: unset — no permissions changed",
		}
	}

	// Reset all to clean slate
	for _, perm := range getIOSPermissions() {
		_ = d.resetIOSPermission(appID, perm)
	}

	// Apply allow/deny permissions; unspecified stay as "not determined"
	var applied, errors []string
	for name, value := range step.Permissions {
		lower := strings.ToLower(value)
		if lower != "allow" && lower != "deny" {
			continue
		}
		if strings.ToLower(name) == "all" {
			for _, perm := range getIOSPermissions() {
				if err := d.applyIOSPermission(appID, perm, lower); err != nil {
					errors = append(errors, fmt.Sprintf("%s: %v", perm, err))
				} else {
					applied = append(applied, perm)
				}
			}
		} else {
			for _, perm := range resolveIOSPermissionShortcut(name) {
				if err := d.applyIOSPermission(appID, perm, lower); err != nil {
					errors = append(errors, fmt.Sprintf("%s: %v", perm, err))
				} else {
					applied = append(applied, perm)
				}
			}
		}
	}

	msg := fmt.Sprintf("Permissions set: %d applied, all others reset", len(applied))
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

// hasAllValue checks if all permission values match the given value.
func hasAllValue(permissions map[string]string, value string) bool {
	for _, v := range permissions {
		if strings.ToLower(v) != value {
			return false
		}
	}
	return len(permissions) > 0
}

// resetIOSPermission resets a single permission to "not determined" using xcrun simctl privacy.
func (d *Driver) resetIOSPermission(appID, permission string) error {
	cmd := exec.Command("xcrun", "simctl", "privacy", d.udid, "reset", permission, appID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// resolveAlertAction determines the WDA defaultAlertAction from a permission map.
// Returns "accept" for all-allow, "dismiss" for all-deny, "" for mixed.
func resolveAlertAction(permissions map[string]string) string {
	if len(permissions) == 0 {
		return "accept"
	}

	// Check for "all" key
	if val, ok := permissions["all"]; ok && len(permissions) == 1 {
		switch strings.ToLower(val) {
		case "allow":
			return "accept"
		case "deny":
			return "dismiss"
		}
	}

	// Check if all values are the same
	var lastVal string
	for _, v := range permissions {
		lower := strings.ToLower(v)
		if lastVal == "" {
			lastVal = lower
		} else if lastVal != lower {
			return "" // Mixed permissions
		}
	}
	switch lastVal {
	case "allow":
		return "accept"
	case "deny":
		return "dismiss"
	default:
		return ""
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
