package appium

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Tap commands

func (d *Driver) tapOn(step *flow.TapOnStep) *core.CommandResult {
	timeout := time.Duration(step.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = d.getFindTimeout()
	}

	info, err := d.findElement(step.Selector, timeout)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %s", step.Selector.Describe()))
	}

	cx, cy := info.Bounds.Center()
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

	info, err := d.findElement(step.Selector, timeout)
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

	info, err := d.findElement(step.Selector, timeout)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %s", step.Selector.Describe()))
	}

	duration := 1000 // Default 1 second for long press

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

	// Handle "50%, 50%" format in Point field
	if step.Point != "" {
		xPct, yPct, err := parsePercentageCoords(step.Point)
		if err != nil {
			return errorResult(err, "Invalid point coordinates")
		}
		x = int(float64(w) * xPct)
		y = int(float64(h) * yPct)
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

	centerX := w / 2
	centerY := h / 2
	var startX, startY, endX, endY int

	switch direction {
	case "up":
		startX, startY = centerX, h*2/3
		endX, endY = centerX, h/3
	case "down":
		startX, startY = centerX, h/3
		endX, endY = centerX, h*2/3
	case "left":
		startX, startY = w*2/3, centerY
		endX, endY = w/3, centerY
	case "right":
		startX, startY = w/3, centerY
		endX, endY = w*2/3, centerY
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

	for i := 0; i < maxScrolls && time.Now().Before(deadline); i++ {
		// Check if element is visible
		info, err := d.findElement(step.Selector, 1*time.Second)
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

	if err := d.client.SendKeys(text); err != nil {
		return errorResult(err, "Failed to input text")
	}

	return successResult(fmt.Sprintf("Input text: %s", text), nil)
}

func (d *Driver) eraseText(step *flow.EraseTextStep) *core.CommandResult {
	// Try to clear active element
	if elemID, err := d.client.GetActiveElement(); err == nil && elemID != "" {
		if err := d.client.ClearElement(elemID); err == nil {
			return successResult("Cleared text from active element", nil)
		}
	}

	// Fallback: send delete keys
	chars := step.Characters
	if chars <= 0 {
		chars = 50 // Default
	}

	for i := 0; i < chars; i++ {
		d.client.PressKeyCode(67) // Android KEYCODE_DEL
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
	timeout := time.Duration(step.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 2 * time.Second // Shorter timeout for not visible
	}

	// Element should NOT be found
	_, err := d.findElement(step.Selector, timeout)
	if err == nil {
		return errorResult(fmt.Errorf("element is visible when it should not be"), fmt.Sprintf("Element should not be visible: %s", step.Selector.Describe()))
	}

	return successResult(fmt.Sprintf("Element is not visible: %s", step.Selector.Describe()), nil)
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
	appID := step.AppID
	if appID == "" {
		appID = d.appID
	}

	if appID == "" {
		return errorResult(fmt.Errorf("no app ID specified"), "")
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
	if err := d.client.OpenURL(step.Link); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open link: %s", step.Link))
	}
	return successResult(fmt.Sprintf("Opened link: %s", step.Link), nil)
}

// Clipboard

func (d *Driver) copyTextFrom(step *flow.CopyTextFromStep) *core.CommandResult {
	info, err := d.findElement(step.Selector, d.getFindTimeout())
	if err != nil {
		return errorResult(err, "Element not found for copyTextFrom")
	}

	text := info.Text
	if text == "" {
		return errorResult(fmt.Errorf("element has no text"), "")
	}

	if err := d.client.SetClipboard(text); err != nil {
		return errorResult(err, "Failed to set clipboard")
	}

	result := successResult(fmt.Sprintf("Copied text: %s", text), info)
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

// Keys

func (d *Driver) pressKey(step *flow.PressKeyStep) *core.CommandResult {
	key := strings.ToLower(step.Key)

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

// Helpers

func parsePercentageCoords(coord string) (float64, float64, error) {
	// Parse "50%, 15%" format
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

	return x / 100, y / 100, nil
}
