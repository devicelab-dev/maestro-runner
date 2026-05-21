package appium

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// Tap commands

func (d *Driver) tapOn(step *flow.TapOnStep) *core.CommandResult {
	// Check if using Point WITHOUT selector (screen-relative tap)
	if step.Point != "" && step.Selector.IsEmpty() {
		w, h := d.client.ScreenSize()
		x, y, err := core.ParsePointCoords(step.Point, w, h)
		if err != nil {
			return errorResult(err, "Invalid point coordinates")
		}
		if err := d.client.Tap(x, y); err != nil {
			return errorResult(err, "Failed to tap at point")
		}
		return successResult(fmt.Sprintf("Tapped at (%d, %d)", x, y), nil)
	}

	timeout := time.Duration(step.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = d.getFindTimeout()
	}

	// Use findElementForTap which prioritizes clickable elements
	info, err := d.findElementForTap(step.Selector, timeout)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %s", step.Selector.Describe()))
	}

	cx, cy := info.Bounds.Center()

	// If duration is set (or longPress: true), hold the press for that long.
	if step.DurationMs > 0 || step.LongPress {
		duration := step.DurationMs
		if duration <= 0 {
			duration = 1000
		}
		if err := d.client.LongPress(cx, cy, duration); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to press for %dms", duration))
		}
		return successResult(fmt.Sprintf("Pressed on element for %dms", duration), info)
	}

	// On iOS, store the element ID so inputText can use ElementSendKeys
	// to atomically focus + type (bypasses keyboard focus timing issues).
	if d.platform == "ios" && info.ID != "" {
		d.lastTappedElementID = info.ID
		// Use ClickElement (POST /element/{id}/click) instead of coordinate tap.
		// Coordinate taps via W3C pointer actions are unreliable on iOS: they can miss
		// if the keyboard is animating, or if the element is partially obscured.
		// ClickElement calls [XCUIElement tap] directly via WDA.
		if err := d.client.ClickElement(info.ID); err != nil {
			return errorResult(err, "Failed to tap")
		}
		return successResult(fmt.Sprintf("Tapped on element '%s'", info.ID), info)
	}

	if err := d.client.Tap(cx, cy); err != nil {
		return errorResult(err, "Failed to tap")
	}

	return successResult(fmt.Sprintf("Tapped on element at (%d, %d)", cx, cy), info)
}

func (d *Driver) doubleTapOn(step *flow.DoubleTapOnStep) *core.CommandResult {
	timeout := time.Duration(step.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = d.getFindTimeout()
	}

	// Use findElementForTap which prioritizes clickable elements
	info, err := d.findElementForTap(step.Selector, timeout)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %s", step.Selector.Describe()))
	}

	cx, cy := info.Bounds.Center()
	if err := d.client.DoubleTap(cx, cy); err != nil {
		return errorResult(err, "Failed to double tap")
	}

	return successResult(fmt.Sprintf("Double tapped on element at (%d, %d)", cx, cy), info)
}

func (d *Driver) longPressOn(step *flow.LongPressOnStep) *core.CommandResult {
	timeout := time.Duration(step.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = d.getFindTimeout()
	}

	// Use findElementForTap which prioritizes clickable elements
	info, err := d.findElementForTap(step.Selector, timeout)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %s", step.Selector.Describe()))
	}

	duration := step.DurationMs
	if duration <= 0 {
		duration = 1000 // Default 1 second for long press
	}

	cx, cy := info.Bounds.Center()
	if err := d.client.LongPress(cx, cy, duration); err != nil {
		return errorResult(err, "Failed to long press")
	}

	return successResult(fmt.Sprintf("Long pressed on element for %dms", duration), info)
}

func (d *Driver) tapOnPoint(step *flow.TapOnPointStep) *core.CommandResult {
	w, h := d.client.ScreenSize()

	x := step.X
	y := step.Y

	// Handle Point field (percentage or absolute coordinates)
	if step.Point != "" {
		var err error
		x, y, err = core.ParsePointCoords(step.Point, w, h)
		if err != nil {
			return errorResult(err, "Invalid point coordinates")
		}
	}

	if step.DurationMs > 0 || step.LongPress {
		duration := step.DurationMs
		if duration <= 0 {
			duration = 1000
		}
		if err := d.client.LongPress(x, y, duration); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to press at point for %dms", duration))
		}
		return successResult(fmt.Sprintf("Pressed at (%d, %d)", x, y), nil)
	}

	if err := d.client.Tap(x, y); err != nil {
		return errorResult(err, "Failed to tap")
	}

	return successResult(fmt.Sprintf("Tapped at (%d, %d)", x, y), nil)
}

// Swipe and scroll

func (d *Driver) swipe(step *flow.SwipeStep) *core.CommandResult {
	w, h := d.client.ScreenSize()

	// Coordinate-based swipe
	if step.Start != "" && step.End != "" {
		startXPct, startYPct, err := parsePercentageCoords(step.Start)
		if err != nil {
			return errorResult(err, "Invalid start coordinates")
		}
		endXPct, endYPct, err := parsePercentageCoords(step.End)
		if err != nil {
			return errorResult(err, "Invalid end coordinates")
		}

		startX := int(float64(w) * startXPct)
		startY := int(float64(h) * startYPct)
		endX := int(float64(w) * endXPct)
		endY := int(float64(h) * endYPct)

		duration := step.Duration
		if duration <= 0 {
			duration = 300
		}

		if err := d.client.Swipe(startX, startY, endX, endY, duration); err != nil {
			return errorResult(err, "Failed to swipe")
		}
		return successResult(fmt.Sprintf("Swiped from (%d,%d) to (%d,%d)", startX, startY, endX, endY), nil)
	}

	// Absolute coordinates
	if step.StartX > 0 || step.StartY > 0 || step.EndX > 0 || step.EndY > 0 {
		duration := step.Duration
		if duration <= 0 {
			duration = 300
		}
		if err := d.client.Swipe(step.StartX, step.StartY, step.EndX, step.EndY, duration); err != nil {
			return errorResult(err, "Failed to swipe")
		}
		return successResult(fmt.Sprintf("Swiped from (%d,%d) to (%d,%d)", step.StartX, step.StartY, step.EndX, step.EndY), nil)
	}

	// Direction-based swipe
	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "up"
	}

	// Swipe coordinates match Maestro behavior:
	// UP:    50%,50% → 50%,10%
	// DOWN:  50%,20% → 50%,90%
	// LEFT:  90%,50% → 10%,50%
	// RIGHT: 10%,50% → 90%,50%
	var startX, startY, endX, endY int

	switch direction {
	case "up":
		startX, startY = w*50/100, h*50/100
		endX, endY = w*50/100, h*10/100
	case "down":
		startX, startY = w*50/100, h*20/100
		endX, endY = w*50/100, h*90/100
	case "left":
		startX, startY = w*90/100, h*50/100
		endX, endY = w*10/100, h*50/100
	case "right":
		startX, startY = w*10/100, h*50/100
		endX, endY = w*90/100, h*50/100
	default:
		return errorResult(fmt.Errorf("invalid direction: %s", direction), "")
	}

	if err := d.client.Swipe(startX, startY, endX, endY, 500); err != nil {
		return errorResult(err, "Failed to swipe")
	}

	return successResult(fmt.Sprintf("Swiped %s", direction), nil)
}

func (d *Driver) scroll(step *flow.ScrollStep) *core.CommandResult {
	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "down"
	}

	w, h := d.client.ScreenSize()
	centerX := w / 2
	var startY, endY int

	switch direction {
	case "down":
		startY = h * 2 / 3
		endY = h / 3
	case "up":
		startY = h / 3
		endY = h * 2 / 3
	default:
		return errorResult(fmt.Errorf("invalid scroll direction: %s", direction), "")
	}

	if err := d.client.Swipe(centerX, startY, centerX, endY, 500); err != nil {
		return errorResult(err, "Failed to scroll")
	}

	return successResult(fmt.Sprintf("Scrolled %s", direction), nil)
}

func (d *Driver) scrollUntilVisible(step *flow.ScrollUntilVisibleStep) *core.CommandResult {
	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "down"
	}

	timeout := time.Duration(step.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	deadline := time.Now().Add(timeout)
	maxScrolls := 20
	if step.MaxScrolls > 0 {
		maxScrolls = step.MaxScrolls
	}

	for i := 0; i < maxScrolls && time.Now().Before(deadline); i++ {
		if err := d.parentContext().Err(); err != nil {
			return errorResult(fmt.Errorf("scroll cancelled: %w", err), "")
		}

		// Check if element is visible
		info, err := d.findElement(step.Element, 1*time.Second)
		if err == nil && info != nil {
			return successResult("Element found", info)
		}

		// Scroll
		d.scroll(&flow.ScrollStep{Direction: direction})
		time.Sleep(300 * time.Millisecond)
	}

	return errorResult(fmt.Errorf("element not found after scrolling"), "")
}

// Text input

func (d *Driver) inputText(step *flow.InputTextStep) *core.CommandResult {
	text := step.Text

	if d.platform == "ios" {
		// On iOS, use ElementSendKeys (POST /element/{id}/value) which internally
		// calls WDA's fb_typeText. This atomically handles focus (taps the element
		// if not focused) and types text — no dependency on prior keyboard state.
		elemID := d.lastTappedElementID
		if elemID == "" {
			// No element ID from tapOn (e.g., element found via page source parsing).
			// Find the currently focused element by polling for hasKeyboardFocus.
			elemID = d.findFocusedElementID()
		}

		if elemID != "" {
			d.waitForKeyboardFocus(elemID)
			if err := d.client.ElementSendKeys(elemID, text); err != nil {
				return errorResult(err, "Failed to input text")
			}
		} else {
			// Final fallback: use "mobile: keys" which types into currently focused element
			_, err := d.client.ExecuteMobile("keys", map[string]interface{}{
				"keys": strings.Split(text, ""),
			})
			if err != nil {
				return errorResult(err, "Failed to input text")
			}
		}
	} else {
		if err := d.client.SendKeys(text); err != nil {
			return errorResult(err, "Failed to input text")
		}
	}

	return successResult(fmt.Sprintf("Input text: %s", text), nil)
}

// waitForKeyboardFocus polls until the element has keyboard focus or timeout.
// This handles the async gap between a coordinate tap and keyboard readiness.
func (d *Driver) waitForKeyboardFocus(elementID string) {
	for i := 0; i < 10; i++ {
		val, err := d.client.GetElementAttribute(elementID, "hasKeyboardFocus")
		if err == nil && val == "true" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// findFocusedElementID finds the element with keyboard focus on iOS.
// Polls up to 1s for an element with hasKeyboardFocus == true.
func (d *Driver) findFocusedElementID() string {
	for i := 0; i < 10; i++ {
		if id, err := d.client.FindElement("-ios predicate string", "hasKeyboardFocus == true"); err == nil && id != "" {
			return id
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ""
}

func (d *Driver) eraseText(step *flow.EraseTextStep) *core.CommandResult {
	chars := step.Characters
	if chars <= 0 {
		chars = 50 // Default
	}

	// Try optimized approach first (Clear or text replacement)
	// This is much faster than pressing delete key N times (3 HTTP calls vs N calls)
	activeElemID, err := d.client.GetActiveElement()
	if err == nil && activeElemID != "" {
		// Got active element - try to read its text
		currentText, textErr := d.client.GetElementText(activeElemID)
		if textErr == nil {
			textLen := len([]rune(currentText)) // Use runes for proper Unicode handling

			// Case 1: Erase all text (or more than exists) - just Clear() in one shot
			if chars >= textLen || textLen == 0 {
				if clearErr := d.client.ClearElement(activeElemID); clearErr == nil {
					return successResult(fmt.Sprintf("Cleared %d characters", textLen), nil)
				}
				// Clear failed, fall through to delete key approach
			} else {
				// Case 2: Erase N chars from end - use text replacement
				runes := []rune(currentText)
				remaining := string(runes[:textLen-chars])

				if clearErr := d.client.ClearElement(activeElemID); clearErr == nil {
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
		// GetElementText failed (e.g., password field), fall through to delete key approach
	}
	// GetActiveElement failed, fall through to delete key approach

	// Fallback: Press delete key multiple times
	// This is slower (N HTTP calls) but works in edge cases:
	// - Can't find focused element
	// - Element doesn't support Clear() or Text()
	// - Password fields that don't expose text
	// - Custom input components
	for i := 0; i < chars; i++ {
		if err := d.client.PressKeyCode(67); err != nil { // Android KEYCODE_DEL
			logger.Warn("failed to press delete key on iteration %d: %v", i, err)
		}
	}

	return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
}

// Assertions

func (d *Driver) assertVisible(step *flow.AssertVisibleStep) *core.CommandResult {
	timeout := time.Duration(step.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = d.getFindTimeout()
	}

	info, err := d.findElement(step.Selector, timeout)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not visible: %s", step.Selector.Describe()))
	}

	return successResult(fmt.Sprintf("Element is visible: %s", step.Selector.Describe()), info)
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
			return successResult(fmt.Sprintf("Element is not visible: %s", step.Selector.Describe()), nil)
		}

		if time.Now().After(deadline) {
			return errorResult(fmt.Errorf("element is visible when it should not be"), fmt.Sprintf("Element should not be visible: %s", step.Selector.Describe()))
		}

		time.Sleep(pollInterval)
	}
}

// Navigation

func (d *Driver) back(step *flow.BackStep) *core.CommandResult {
	if err := d.client.Back(); err != nil {
		return errorResult(err, "Failed to press back")
	}
	return successResult("Pressed back", nil)
}

func (d *Driver) hideKeyboard(step *flow.HideKeyboardStep) *core.CommandResult {
	if err := d.client.HideKeyboard(); err != nil {
		// Don't fail - keyboard may not be visible
		return successResult("Hide keyboard (may not have been visible)", nil)
	}
	return successResult("Hid keyboard", nil)
}

// App management

func (d *Driver) launchApp(step *flow.LaunchAppStep) *core.CommandResult {
	// Handle newSession (Appium only)
	if step.NewSession {
		if d.platform == "ios" && !d.client.IsRealDevice() {
			// iOS simulator: no benefit from session restart
			logger.Info("newSession ignored on iOS simulator")
		} else {
			if err := d.RestartSession(); err != nil {
				return errorResult(err, "Failed to create new Appium session")
			}
			// On iOS real device, skip clearState — fresh session is already clean
			if d.platform == "ios" {
				step.ClearState = false
			}
		}
	}

	appID := step.AppID
	if appID == "" {
		appID = d.appID
	}

	if appID == "" {
		return errorResult(fmt.Errorf("no app ID specified"), "")
	}

	// Stop app first if requested (default: true)
	if step.StopApp == nil || *step.StopApp {
		if err := d.client.TerminateApp(appID); err != nil {
			logger.Warn("failed to stop app %s before relaunch: %v", appID, err)
		}
	}

	// Clear state if requested
	if step.ClearState {
		if err := d.client.ClearAppData(appID); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to clear app state: %s", appID))
		}

		// Grant permissions after clearing state (pm clear resets permissions).
		// Use flow-specified permissions if provided, otherwise grant all.
		if d.client.Platform() == "android" {
			d.grantPermissions(appID, step.Permissions)
		}
	}

	if err := d.client.LaunchApp(appID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to launch app: %s", appID))
	}

	return successResult(fmt.Sprintf("Launched app: %s", appID), nil)
}

func (d *Driver) stopApp(step *flow.StopAppStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		appID = d.appID
	}

	if appID == "" {
		return errorResult(fmt.Errorf("no app ID specified"), "")
	}

	if err := d.client.TerminateApp(appID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to stop app: %s", appID))
	}

	return successResult(fmt.Sprintf("Stopped app: %s", appID), nil)
}

func (d *Driver) clearState(step *flow.ClearStateStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		appID = d.appID
	}

	if appID == "" {
		return errorResult(fmt.Errorf("no app ID specified"), "")
	}

	if err := d.client.ClearAppData(appID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to clear app state: %s", appID))
	}

	return successResult(fmt.Sprintf("Cleared app state: %s", appID), nil)
}

// Device control

func (d *Driver) setLocation(step *flow.SetLocationStep) *core.CommandResult {
	lat, err := strconv.ParseFloat(step.Latitude, 64)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid latitude: %s", step.Latitude))
	}

	lon, err := strconv.ParseFloat(step.Longitude, 64)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid longitude: %s", step.Longitude))
	}

	if err := d.client.SetLocation(lat, lon); err != nil {
		return errorResult(err, "Failed to set location")
	}
	return successResult(fmt.Sprintf("Set location to (%.6f, %.6f)", lat, lon), nil)
}

func (d *Driver) setOrientation(step *flow.SetOrientationStep) *core.CommandResult {
	orientation := strings.ToLower(step.Orientation)
	if err := d.client.SetOrientation(orientation); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set orientation: %s", orientation))
	}
	return successResult(fmt.Sprintf("Set orientation to %s", orientation), nil)
}

func (d *Driver) openLink(step *flow.OpenLinkStep) *core.CommandResult {
	// Note: Appium's OpenURL opens in the default handler
	// browser parameter would require mobile: shell on Android or Safari automation on iOS
	// For now, we use the standard Appium approach which respects system defaults

	if err := d.client.OpenURL(step.Link); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open link: %s", step.Link))
	}

	// If autoVerify is enabled, wait briefly for page load
	if step.AutoVerify != nil && *step.AutoVerify {
		time.Sleep(2 * time.Second)
	}

	msg := fmt.Sprintf("Opened link: %s", step.Link)
	if step.Browser != nil && *step.Browser {
		msg += " (browser flag set, but Appium uses system default handler)"
	}
	return successResult(msg, nil)
}

// Clipboard

func (d *Driver) copyTextFrom(step *flow.CopyTextFromStep) *core.CommandResult {
	info, err := d.findElement(step.Selector, d.getFindTimeout())
	if err != nil {
		return errorResult(err, "Element not found for copyTextFrom")
	}

	// Get text, falling back to AccessibilityLabel if empty
	text := info.Text
	if text == "" && info.AccessibilityLabel != "" {
		text = info.AccessibilityLabel
	}
	if text == "" {
		return errorResult(fmt.Errorf("element has no text"), "")
	}

	// Don't push to device clipboard — Appium 3.x UIA2 returns 404 for
	// /appium/device/set_clipboard, and the executor already keeps the
	// copied text in memory (script.SetCopiedText) for pasteText to reuse.
	result := successResult(fmt.Sprintf("Copied text: '%s' (len=%d)", text, len(text)), info)
	result.Data = text
	return result
}

func (d *Driver) pasteText(step *flow.PasteTextStep) *core.CommandResult {
	text, err := d.client.GetClipboard()
	if err != nil {
		return errorResult(err, "Failed to get clipboard")
	}

	if err := d.client.SendKeys(text); err != nil {
		return errorResult(err, "Failed to paste text")
	}

	return successResult(fmt.Sprintf("Pasted text: %s", text), nil)
}

func (d *Driver) setClipboard(step *flow.SetClipboardStep) *core.CommandResult {
	if step.Text == "" {
		return errorResult(fmt.Errorf("no text specified"), "setClipboard requires text")
	}

	if err := d.client.SetClipboard(step.Text); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set clipboard: %v", err))
	}

	return successResult(fmt.Sprintf("Set clipboard to: %s", step.Text), nil)
}

// Keys

func (d *Driver) pressKey(step *flow.PressKeyStep) *core.CommandResult {
	key := strings.ToLower(step.Key)

	if d.platform == "ios" {
		return d.pressKeyIOS(key)
	}
	return d.pressKeyAndroid(key)
}

func (d *Driver) pressKeyAndroid(key string) *core.CommandResult {
	keyMap := map[string]int{
		"back":        4,
		"home":        3,
		"enter":       66,
		"backspace":   67,
		"delete":      112,
		"tab":         61,
		"volume_up":   24,
		"volume_down": 25,
		"power":       26,
	}

	if keycode, ok := keyMap[key]; ok {
		if err := d.client.PressKeyCode(keycode); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to press key: %s", key))
		}
		return successResult(fmt.Sprintf("Pressed key: %s", key), nil)
	}

	return errorResult(fmt.Errorf("unknown key: %s", key), "")
}

func (d *Driver) pressKeyIOS(key string) *core.CommandResult {
	// Physical buttons via mobile: pressButton
	switch key {
	case "home":
		if _, err := d.client.ExecuteMobile("pressButton", map[string]interface{}{"name": "home"}); err != nil {
			return errorResult(err, "Failed to press home")
		}
		return successResult("Pressed home", nil)
	case "volumeup", "volume_up":
		if _, err := d.client.ExecuteMobile("pressButton", map[string]interface{}{"name": "volumeUp"}); err != nil {
			return errorResult(err, "Failed to press volume up")
		}
		return successResult("Pressed volume up", nil)
	case "volumedown", "volume_down":
		if _, err := d.client.ExecuteMobile("pressButton", map[string]interface{}{"name": "volumeDown"}); err != nil {
			return errorResult(err, "Failed to press volume down")
		}
		return successResult("Pressed volume down", nil)
	}

	// Keyboard keys via W3C key actions (SendKeys)
	keyChar := iosKeyChar(key)
	if keyChar == "" {
		return errorResult(fmt.Errorf("unknown key: %s", key), "")
	}
	if err := d.client.SendKeys(keyChar); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to press key: %s", key))
	}
	return successResult(fmt.Sprintf("Pressed key: %s", key), nil)
}

// iosKeyChar maps key names to the character to send via W3C key actions.
func iosKeyChar(name string) string {
	switch name {
	case "enter", "return":
		return "\n"
	case "tab":
		return "\t"
	case "backspace", "delete":
		return "\b"
	case "space":
		return " "
	default:
		return ""
	}
}

// Helpers

// Wait commands

func (d *Driver) waitForAnimationToEnd(_ *flow.WaitForAnimationToEndStep) *core.CommandResult {
	// NOTE: waitForAnimationToEnd is not fully implemented.
	// Maestro uses screenshot comparison which is complex to implement correctly.
	// For now, we pass this step with a warning.
	return &core.CommandResult{
		Success: true,
		Message: "WARNING: waitForAnimationToEnd is not fully implemented - step passed without animation check",
	}
}

func (d *Driver) waitUntil(step *flow.WaitUntilStep) *core.CommandResult {
	// Use step timeout if specified, otherwise default to 30 seconds
	timeout := 30 * time.Second
	if step.TimeoutMs > 0 {
		timeout = time.Duration(step.TimeoutMs) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
	defer cancel()

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
				info, err := d.findElementOnce(*step.Visible)
				if err == nil && info != nil {
					return successResult("Element is now visible", info)
				}
			} else {
				info, err := d.findElementOnce(*step.NotVisible)
				if err != nil || info == nil {
					return successResult("Element is no longer visible", nil)
				}
			}
			// HTTP round-trip (~100ms) is natural rate limit, no sleep needed
		}
	}
}

func (d *Driver) killApp(step *flow.KillAppStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		appID = d.appID
	}

	if appID == "" {
		return errorResult(fmt.Errorf("no app ID specified"), "")
	}

	if err := d.client.TerminateApp(appID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to kill app: %s", appID))
	}

	return successResult(fmt.Sprintf("Killed app: %s", appID), nil)
}

func (d *Driver) inputRandom(step *flow.InputRandomStep) *core.CommandResult {
	length := step.Length
	if length <= 0 {
		length = 10
	}

	var text string
	switch strings.ToUpper(step.DataType) {
	case "EMAIL":
		text = randomEmail()
	case "NUMBER":
		text = randomNumber(length)
	case "PERSON_NAME":
		text = randomPersonName()
	default:
		text = randomString(length)
	}

	if err := d.client.SendKeys(text); err != nil {
		return errorResult(err, "Failed to input random text")
	}

	result := successResult(fmt.Sprintf("Input random %s: %s", step.DataType, text), nil)
	result.Data = text
	return result
}

func (d *Driver) takeScreenshot(step *flow.TakeScreenshotStep) *core.CommandResult {
	data, err := d.client.Screenshot()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to take screenshot: %v", err))
	}

	return &core.CommandResult{
		Success: true,
		Message: "Screenshot captured",
		Data:    data,
	}
}

// Random data generators

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

// Helpers

func parsePercentageCoords(coord string) (float64, float64, error) {
	return core.ParsePercentageCoords(coord)
}

// grantPermissions grants permissions via mobile: shell pm grant.
// If the permissions map is provided, only those are granted (keys are permission names).
// If empty/nil, all common runtime permissions are granted.
//
// Note: `mobile: shell` requires Appium's `adb_shell` insecure feature to
// be enabled. On hosts that disable it (e.g. Sauce Labs by default), pass
// `appium:autoGrantPermissions: true` in caps so Appium grants declared
// permissions during install, and avoid using this step explicitly.
func (d *Driver) grantPermissions(appID string, permissions map[string]string) {
	if len(permissions) > 0 {
		for perm := range permissions {
			if _, err := d.client.ExecuteMobile("shell", map[string]interface{}{
				"command": "pm",
				"args":    []string{"grant", appID, perm},
			}); err != nil {
				logger.Warn("failed to grant permission %s to %s: %v", perm, appID, err)
			}
		}
		return
	}

	for _, perm := range getAllPermissions() {
		if _, err := d.client.ExecuteMobile("shell", map[string]interface{}{
			"command": "pm",
			"args":    []string{"grant", appID, perm},
		}); err != nil {
			logger.Warn("failed to grant permission %s to %s: %v", perm, appID, err)
		}
	}
}

// getAllPermissions returns all common Android runtime permissions.
func getAllPermissions() []string {
	return []string{
		"android.permission.ACCESS_FINE_LOCATION",
		"android.permission.ACCESS_COARSE_LOCATION",
		"android.permission.ACCESS_BACKGROUND_LOCATION",
		"android.permission.CAMERA",
		"android.permission.READ_CONTACTS",
		"android.permission.WRITE_CONTACTS",
		"android.permission.GET_ACCOUNTS",
		"android.permission.READ_PHONE_STATE",
		"android.permission.CALL_PHONE",
		"android.permission.READ_CALL_LOG",
		"android.permission.WRITE_CALL_LOG",
		"android.permission.USE_SIP",
		"android.permission.PROCESS_OUTGOING_CALLS",
		"android.permission.RECORD_AUDIO",
		"android.permission.BLUETOOTH_CONNECT",
		"android.permission.BLUETOOTH_SCAN",
		"android.permission.BLUETOOTH_ADVERTISE",
		"android.permission.READ_EXTERNAL_STORAGE",
		"android.permission.WRITE_EXTERNAL_STORAGE",
		"android.permission.READ_MEDIA_IMAGES",
		"android.permission.READ_MEDIA_VIDEO",
		"android.permission.READ_MEDIA_AUDIO",
		"android.permission.POST_NOTIFICATIONS",
		"android.permission.READ_CALENDAR",
		"android.permission.WRITE_CALENDAR",
		"android.permission.SEND_SMS",
		"android.permission.RECEIVE_SMS",
		"android.permission.READ_SMS",
		"android.permission.RECEIVE_WAP_PUSH",
		"android.permission.RECEIVE_MMS",
		"android.permission.BODY_SENSORS",
		"android.permission.ACTIVITY_RECOGNITION",
	}
}
