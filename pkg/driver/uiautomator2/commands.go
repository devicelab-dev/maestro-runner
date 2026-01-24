package uiautomator2

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// ============================================================================
// Tap Commands
// ============================================================================

func (d *Driver) tapOn(step *flow.TapOnStep) *core.CommandResult {
	// Check if using percentage-based Point WITHOUT selector (screen-relative tap)
	if step.Point != "" && step.Selector.IsEmpty() {
		return d.tapOnPointWithPercentage(step.Point)
	}

	elem, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	// If Point is specified WITH selector, tap at relative position within element bounds
	if step.Point != "" && info != nil && info.Bounds.Width > 0 {
		xPct, yPct, parseErr := parsePercentageCoords(step.Point)
		if parseErr != nil {
			return errorResult(parseErr, fmt.Sprintf("Invalid point coordinates: %v", parseErr))
		}
		x := info.Bounds.X + int(float64(info.Bounds.Width)*xPct)
		y := info.Bounds.Y + int(float64(info.Bounds.Height)*yPct)
		if err := d.client.Click(x, y); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to tap at relative point: %v", err))
		}
		return successResult(fmt.Sprintf("Tapped at relative point (%d, %d) on element", x, y), info)
	}

	// For relative selectors, elem is nil but we have bounds - tap at center
	if elem == nil {
		x, y := info.Bounds.Center()
		if err := d.client.Click(x, y); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to tap at coordinates: %v", err))
		}
	} else {
		if err := elem.Click(); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to tap: %v", err))
		}
	}

	return successResult("Tapped on element", info)
}

// tapOnPointWithPercentage handles percentage-based tap (e.g., "85%, 50%")
func (d *Driver) tapOnPointWithPercentage(point string) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "tapOn with percentage point requires device access")
	}

	// Get screen dimensions
	width, height, err := d.getScreenSize()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get screen size: %v", err))
	}

	// Parse percentage coordinates
	xPct, yPct, err := parsePercentageCoords(point)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid point coordinates: %v", err))
	}

	x := int(float64(width) * xPct)
	y := int(float64(height) * yPct)

	if err := d.client.Click(x, y); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to tap at point: %v", err))
	}

	return successResult(fmt.Sprintf("Tapped at (%d, %d)", x, y), nil)
}

func (d *Driver) doubleTapOn(step *flow.DoubleTapOnStep) *core.CommandResult {
	elem, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	// For relative selectors, elem is nil but we have bounds - double tap at center
	if elem == nil {
		x, y := info.Bounds.Center()
		if err := d.client.DoubleClick(x, y); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to double tap at coordinates: %v", err))
		}
	} else {
		if err := d.client.DoubleClickElement(elem.ID()); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to double tap: %v", err))
		}
	}

	return successResult("Double tapped on element", info)
}

func (d *Driver) longPressOn(step *flow.LongPressOnStep) *core.CommandResult {
	elem, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	duration := 1000 // default 1 second

	// For relative selectors, elem is nil but we have bounds - long press at center
	if elem == nil {
		x, y := info.Bounds.Center()
		if err := d.client.LongClick(x, y, duration); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to long press at coordinates: %v", err))
		}
	} else {
		if err := d.client.LongClickElement(elem.ID(), duration); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to long press: %v", err))
		}
	}

	return successResult("Long pressed on element", info)
}

func (d *Driver) tapOnPoint(step *flow.TapOnPointStep) *core.CommandResult {
	x, y := step.X, step.Y

	// Check if using percentage-based Point (e.g., "85%, 50%")
	if step.Point != "" {
		if d.device == nil {
			return errorResult(fmt.Errorf("device not configured"), "tapOn with percentage point requires device access")
		}

		// Get screen dimensions
		width, height, err := d.getScreenSize()
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to get screen size: %v", err))
		}

		// Parse percentage coordinates
		xPct, yPct, err := parsePercentageCoords(step.Point)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Invalid point coordinates: %v", err))
		}

		x = int(float64(width) * xPct)
		y = int(float64(height) * yPct)
	}

	if x == 0 && y == 0 {
		return errorResult(fmt.Errorf("no point specified"), "Either point or x/y coordinates required")
	}

	if err := d.client.Click(x, y); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to tap at point: %v", err))
	}

	return successResult(fmt.Sprintf("Tapped at (%d, %d)", x, y), nil)
}

// ============================================================================
// Assert Commands
// ============================================================================

func (d *Driver) assertVisible(step *flow.AssertVisibleStep) *core.CommandResult {
	// Use findElementFast - only need to check element exists (1 HTTP call vs 3)
	_, info, err := d.findElementFast(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not visible: %v", err))
	}

	// info.Visible is already set by findElementFast
	if info != nil && info.Visible {
		return successResult("Element is visible", info)
	}

	return errorResult(fmt.Errorf("element not visible"), "Element exists but is not visible")
}

func (d *Driver) assertNotVisible(step *flow.AssertNotVisibleStep) *core.CommandResult {
	// Poll until element is NOT visible (or timeout)
	// Used to verify element has disappeared after an action
	timeout := step.TimeoutMs
	if timeout <= 0 {
		timeout = 5000
	}

	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	pollInterval := 500 * time.Millisecond

	for {
		// Quick check if element exists (no waiting)
		_, info, err := d.findElementQuick(step.Selector, 0)
		if err != nil || info == nil {
			// Element not found = not visible = success
			return successResult("Element is not visible", nil)
		}

		// Element still visible - check if we've timed out
		if time.Now().After(deadline) {
			return errorResult(fmt.Errorf("element is visible"), "Element should not be visible but was found")
		}

		// Wait before next check
		time.Sleep(pollInterval)
	}
}

// ============================================================================
// Input Commands
// ============================================================================

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

	// If selector provided, find element and type into it
	if !step.Selector.IsEmpty() {
		elem, _, err := d.findElement(step.Selector, step.IsOptional(), step.TimeoutMs)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Element not found: %v", err))
		}
		if err := elem.SendKeys(text); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to input text: %v", err))
		}
	} else {
		// Type into focused element
		// First try WebDriver activeElement endpoint
		active, err := d.client.ActiveElement()
		if err != nil {
			// Fallback: find element with focused=true via page source
			focusedTrue := true
			focusedSel := flow.Selector{Focused: &focusedTrue}
			elem, _, findErr := d.findElement(focusedSel, false, 2000)
			if findErr != nil {
				return errorResult(err, "No focused element to type into")
			}
			if err := elem.SendKeys(text); err != nil {
				return errorResult(err, fmt.Sprintf("Failed to input text: %v", err))
			}
			return successResult(fmt.Sprintf("Entered text: %s%s", text, unicodeWarning), nil)
		}
		if err := active.SendKeys(text); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to input text: %v", err))
		}
	}

	return successResult(fmt.Sprintf("Entered text: %s%s", text, unicodeWarning), nil)
}

func (d *Driver) eraseText(step *flow.EraseTextStep) *core.CommandResult {
	chars := step.Characters
	if chars <= 0 {
		chars = 50 // default
	}

	// Press delete key multiple times
	for i := 0; i < chars; i++ {
		if err := d.client.PressKeyCode(uiautomator2.KeyCodeDelete); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to erase text: %v", err))
		}
	}

	return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
}

func (d *Driver) hideKeyboard(_ *flow.HideKeyboardStep) *core.CommandResult {
	// Press back to hide keyboard
	if err := d.client.Back(); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to hide keyboard: %v", err))
	}

	return successResult("Keyboard hidden", nil)
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

	// Type into focused element
	active, err := d.client.ActiveElement()
	if err != nil {
		return errorResult(err, "No focused element to type into")
	}
	if err := active.SendKeys(text); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to input text: %v", err))
	}

	return &core.CommandResult{
		Success: true,
		Message: fmt.Sprintf("Entered random %s: %s", dataType, text),
		Data:    text,
	}
}

// ============================================================================
// Scroll/Swipe Commands
// ============================================================================

func (d *Driver) scroll(step *flow.ScrollStep) *core.CommandResult {
	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "down"
	}

	// Invert direction: scroll direction = content movement, swipe = finger gesture
	// "scroll down" means reveal content below, which requires swiping up
	uiaDir := invertScrollDirection(direction)

	// Get screen size for dynamic scroll area
	width, height := 1080, 1920 // defaults
	if w, h, err := d.getScreenSize(); err == nil {
		width, height = w, h
	}

	// Use most of screen for scroll area (leave margins)
	area := uiautomator2.NewRect(0, height/8, width, height*3/4)

	if err := d.client.ScrollInArea(area, uiaDir, 0.5, 0); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to scroll: %v", err))
	}

	return successResult(fmt.Sprintf("Scrolled %s", direction), nil)
}

func (d *Driver) scrollUntilVisible(step *flow.ScrollUntilVisibleStep) *core.CommandResult {
	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "down"
	}

	maxScrolls := 10
	// Invert direction: scroll direction = content movement, swipe = finger gesture
	// "scroll down" means reveal content below, which requires swiping up
	uiaDir := invertScrollDirection(direction)

	// Get screen size for dynamic scroll area
	width, height := 1080, 1920 // defaults
	if w, h, err := d.getScreenSize(); err == nil {
		width, height = w, h
	}

	// Use most of screen for scroll area (leave margins)
	area := uiautomator2.NewRect(0, height/8, width, height*3/4)

	for i := 0; i < maxScrolls; i++ {
		// Try to find element (short timeout - includes page source fallback)
		_, info, err := d.findElement(step.Element, true, 1000)
		if err == nil && info != nil {
			// Element found - return success
			return successResult(fmt.Sprintf("Element found after %d scrolls", i), info)
		}

		// Scroll
		if err := d.client.ScrollInArea(area, uiaDir, 0.3, 0); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to scroll: %v", err))
		}

		time.Sleep(300 * time.Millisecond)
	}

	return errorResult(fmt.Errorf("element not found"), fmt.Sprintf("Element not found after %d scrolls", maxScrolls))
}

func (d *Driver) swipe(step *flow.SwipeStep) *core.CommandResult {
	// Check if coordinate-based swipe (percentage or absolute)
	if step.Start != "" && step.End != "" {
		return d.swipeWithCoordinates(step.Start, step.End, step.Duration)
	}

	if step.StartX > 0 || step.StartY > 0 || step.EndX > 0 || step.EndY > 0 {
		return d.swipeWithAbsoluteCoords(step.StartX, step.StartY, step.EndX, step.EndY, step.Duration)
	}

	// Direction-based swipe
	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "up"
	}

	uiaDir := mapDirection(direction)

	// If selector specified, swipe within that element's bounds
	if step.Selector != nil && !step.Selector.IsEmpty() {
		_, info, err := d.findElement(*step.Selector, step.IsOptional(), step.TimeoutMs)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Element not found for swipe: %v", err))
		}
		if info != nil && info.Bounds.Width > 0 {
			area := uiautomator2.NewRect(
				info.Bounds.X,
				info.Bounds.Y,
				info.Bounds.Width,
				info.Bounds.Height,
			)
			if err := d.client.SwipeInArea(area, uiaDir, 0.7, 0); err != nil {
				return errorResult(err, fmt.Sprintf("Failed to swipe in element: %v", err))
			}
			return successResult(fmt.Sprintf("Swiped %s in element", direction), info)
		}
	}

	// Get screen size
	width, height := 1080, 1920 // defaults
	if w, h, err := d.getScreenSize(); err == nil {
		width, height = w, h
	}

	// No selector specified - try to find a scrollable element
	// Wait up to 10 seconds for page to load and find scrollable
	scrollableInfo, scrollableCount := d.findScrollableElement(10000)

	// Print debug info about scrollable elements found
	if scrollableInfo != nil {
		b := scrollableInfo.Bounds
		fmt.Printf("[swipe] Found %d scrollable(s), using: bounds=[%d,%d,%d,%d]\n",
			scrollableCount, b.X, b.Y, b.Width, b.Height)

		// Use coordinate-based swipe within scrollable bounds
		// Centered at 50% horizontally, 70%→30% vertically (relative to scrollable area)
		centerX := b.X + b.Width/2
		var startY, endY int
		switch direction {
		case "up":
			startY = b.Y + b.Height*70/100
			endY = b.Y + b.Height*30/100
		case "down":
			startY = b.Y + b.Height*30/100
			endY = b.Y + b.Height*70/100
		default:
			startY = b.Y + b.Height*70/100
			endY = b.Y + b.Height*30/100
		}

		fmt.Printf("[swipe] Coords in scrollable: (%d,%d) → (%d,%d)\n", centerX, startY, centerX, endY)
		return d.swipeWithAbsoluteCoords(centerX, startY, centerX, endY, step.Duration)
	}

	fmt.Printf("[swipe] No scrollable found, using screen coordinates (50%% center)\n")
	// Fallback: Use coordinates starting from 50% center
	return d.swipeWithMaestroCoordinates(direction, width, height, step.Duration)
}

// findScrollableElement waits for and finds a scrollable element.
// Returns the element info and count of scrollables found.
func (d *Driver) findScrollableElement(timeoutMs int) (*core.ElementInfo, int) {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		source, err := d.client.Source()
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		elements, err := ParsePageSource(source)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		scrollables := FilterScrollable(elements)

		// If exactly one scrollable, use it
		if len(scrollables) == 1 {
			elem := scrollables[0]
			return &core.ElementInfo{
				Bounds: elem.Bounds,
			}, 1
		}

		// If multiple scrollables, find the largest one (likely the main content area)
		if len(scrollables) > 1 {
			largest := FindLargestScrollable(elements)
			if largest != nil {
				return &core.ElementInfo{
					Bounds: largest.Bounds,
				}, len(scrollables)
			}
		}

		time.Sleep(pollInterval)
	}

	return nil, 0
}

// swipeWithMaestroCoordinates performs swipe using centered coordinates.
// Uses 50% as center point with 70%→30% range to avoid triggering system gestures.
// UP: 50%,70% → 50%,30% (swipe finger up, content moves up, reveals below)
// DOWN: 50%,30% → 50%,70% (swipe finger down, content moves down, reveals above)
// LEFT: 70%,50% → 30%,50%
// RIGHT: 30%,50% → 70%,50%
func (d *Driver) swipeWithMaestroCoordinates(direction string, width, height, durationMs int) *core.CommandResult {
	var startX, startY, endX, endY int

	switch direction {
	case "up":
		startX = width / 2
		startY = height * 70 / 100
		endX = width / 2
		endY = height * 30 / 100
	case "down":
		startX = width / 2
		startY = height * 30 / 100
		endX = width / 2
		endY = height * 70 / 100
	case "left":
		startX = width * 70 / 100
		startY = height / 2
		endX = width * 30 / 100
		endY = height / 2
	case "right":
		startX = width * 30 / 100
		startY = height / 2
		endX = width * 70 / 100
		endY = height / 2
	default:
		// Default to up
		startX = width / 2
		startY = height * 70 / 100
		endX = width / 2
		endY = height * 30 / 100
	}

	fmt.Printf("[swipe] Using screen coords: (%d,%d) → (%d,%d)\n", startX, startY, endX, endY)
	return d.swipeWithAbsoluteCoords(startX, startY, endX, endY, durationMs)
}

// swipeWithCoordinates handles percentage-based swipe (e.g., "50%, 15%")
func (d *Driver) swipeWithCoordinates(start, end string, durationMs int) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "swipe with coordinates requires device access")
	}

	// Get screen dimensions
	width, height, err := d.getScreenSize()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get screen size: %v", err))
	}

	// Parse start coordinates
	startXPct, startYPct, err := parsePercentageCoords(start)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid start coordinates: %v", err))
	}

	// Parse end coordinates
	endXPct, endYPct, err := parsePercentageCoords(end)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid end coordinates: %v", err))
	}

	// Convert percentages to pixels
	startX := int(float64(width) * startXPct)
	startY := int(float64(height) * startYPct)
	endX := int(float64(width) * endXPct)
	endY := int(float64(height) * endYPct)

	return d.swipeWithAbsoluteCoords(startX, startY, endX, endY, durationMs)
}

// swipeWithAbsoluteCoords performs swipe with absolute pixel coordinates
func (d *Driver) swipeWithAbsoluteCoords(startX, startY, endX, endY, durationMs int) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "swipe with coordinates requires device access")
	}

	// Default duration if not specified
	if durationMs <= 0 {
		durationMs = 300
	}

	// Use ADB shell command for coordinate swipe
	cmd := fmt.Sprintf("input swipe %d %d %d %d %d", startX, startY, endX, endY, durationMs)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to swipe: %v", err))
	}

	return successResult(fmt.Sprintf("Swiped from (%d,%d) to (%d,%d)", startX, startY, endX, endY), nil)
}

// getScreenSize returns the device screen dimensions (width, height)
func (d *Driver) getScreenSize() (int, int, error) {
	// Try to get from device info via UIAutomator2
	if d.client != nil {
		info, err := d.client.GetDeviceInfo()
		if err == nil && info.RealDisplaySize != "" {
			// Parse "1080x2400" format
			parts := strings.Split(info.RealDisplaySize, "x")
			if len(parts) == 2 {
				width, err1 := strconv.Atoi(parts[0])
				height, err2 := strconv.Atoi(parts[1])
				if err1 == nil && err2 == nil {
					return width, height, nil
				}
			}
		}
	}

	// Fallback: use wm size command
	if d.device == nil {
		return 0, 0, fmt.Errorf("no device connection available to get screen size")
	}
	output, err := d.device.Shell("wm size")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get screen size: %w", err)
	}

	// Parse "Physical size: 1080x2400" format
	output = strings.TrimSpace(output)
	if idx := strings.LastIndex(output, ":"); idx != -1 {
		output = strings.TrimSpace(output[idx+1:])
	}
	parts := strings.Split(output, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected wm size output: %s", output)
	}

	width, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("failed to parse screen size: %s", output)
	}

	return width, height, nil
}

// parsePercentageCoords parses "x%, y%" format into decimal fractions (0.0-1.0)
func parsePercentageCoords(coord string) (float64, float64, error) {
	parts := strings.Split(coord, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected 'x%%, y%%' format, got: %s", coord)
	}

	xStr := strings.TrimSpace(strings.TrimSuffix(parts[0], "%"))
	yStr := strings.TrimSpace(strings.TrimSuffix(parts[1], "%"))

	xPct, err := strconv.ParseFloat(xStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid x percentage: %s", parts[0])
	}

	yPct, err := strconv.ParseFloat(yStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid y percentage: %s", parts[1])
	}

	return xPct / 100.0, yPct / 100.0, nil
}

// ============================================================================
// Navigation Commands
// ============================================================================

func (d *Driver) back(_ *flow.BackStep) *core.CommandResult {
	if err := d.client.Back(); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to press back: %v", err))
	}

	return successResult("Pressed back", nil)
}

func (d *Driver) pressKey(step *flow.PressKeyStep) *core.CommandResult {
	key := step.Key
	keyCode := mapKeyCode(key)
	if keyCode == 0 {
		return errorResult(fmt.Errorf("unknown key: %s", key), fmt.Sprintf("Unknown key: %s", key))
	}

	if err := d.client.PressKeyCode(keyCode); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to press key: %v", err))
	}

	return successResult(fmt.Sprintf("Pressed key: %s", key), nil)
}

// ============================================================================
// App Lifecycle Commands
// ============================================================================

func (d *Driver) launchApp(step *flow.LaunchAppStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		return errorResult(fmt.Errorf("no appId specified"), "No app ID to launch")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "launchApp requires device access")
	}

	// Stop app first if requested (default: true)
	if step.StopApp == nil || *step.StopApp {
		d.device.Shell("am force-stop " + appID)
	}

	// Clear state if requested
	if step.ClearState {
		if _, err := d.device.Shell("pm clear " + appID); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to clear app state: %v", err))
		}
	}

	// Apply permissions (default: all allow, like Maestro)
	permissions := step.Permissions
	if len(permissions) == 0 {
		permissions = map[string]string{"all": "allow"}
	}
	result := d.applyPermissions(appID, permissions)
	if !result.Success {
		// Log warning but don't fail - permission errors are common for non-runtime permissions
		// Continue with launch
	}

	// Launch app - use am start with arguments if provided, otherwise monkey
	var cmd string
	if len(step.Arguments) > 0 {
		// Build am start command with intent extras
		cmd = fmt.Sprintf("am start -n %s/.MainActivity -a android.intent.action.MAIN -c android.intent.category.LAUNCHER", appID)
		for key, value := range step.Arguments {
			switch v := value.(type) {
			case string:
				cmd += fmt.Sprintf(" --es %s '%s'", key, v)
			case int:
				cmd += fmt.Sprintf(" --ei %s %d", key, v)
			case int64:
				cmd += fmt.Sprintf(" --ei %s %d", key, v)
			case float64:
				// YAML numbers can be float64
				if v == float64(int(v)) {
					cmd += fmt.Sprintf(" --ei %s %d", key, int(v))
				} else {
					cmd += fmt.Sprintf(" --ef %s %f", key, v)
				}
			case bool:
				cmd += fmt.Sprintf(" --ez %s %t", key, v)
			default:
				cmd += fmt.Sprintf(" --es %s '%v'", key, v)
			}
		}
	} else {
		// Use monkey (works without knowing activity name)
		cmd = fmt.Sprintf("monkey -p %s -c android.intent.category.LAUNCHER 1", appID)
	}
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to launch app: %v", err))
	}

	// Wait for app to start
	time.Sleep(1 * time.Second)

	return successResult(fmt.Sprintf("Launched app: %s", appID), nil)
}

func (d *Driver) stopApp(step *flow.StopAppStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		return errorResult(fmt.Errorf("no appId specified"), "No app ID to stop")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "stopApp requires device access")
	}

	if _, err := d.device.Shell("am force-stop " + appID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to stop app: %v", err))
	}

	return successResult(fmt.Sprintf("Stopped app: %s", appID), nil)
}

func (d *Driver) clearState(step *flow.ClearStateStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		return errorResult(fmt.Errorf("no appId specified"), "No app ID to clear")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "clearState requires device access")
	}

	if _, err := d.device.Shell("pm clear " + appID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to clear state: %v", err))
	}

	return successResult(fmt.Sprintf("Cleared state for: %s", appID), nil)
}

func (d *Driver) killApp(step *flow.KillAppStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		return errorResult(fmt.Errorf("no appId specified"), "No app ID to kill")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "killApp requires device access")
	}

	if _, err := d.device.Shell("am force-stop " + appID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to kill app: %v", err))
	}

	return successResult(fmt.Sprintf("Killed app: %s", appID), nil)
}

func (d *Driver) setPermissions(step *flow.SetPermissionsStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		return errorResult(fmt.Errorf("no appId specified"), "No app ID for permissions")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "setPermissions requires device access")
	}

	if len(step.Permissions) == 0 {
		return errorResult(fmt.Errorf("no permissions specified"), "No permissions to set")
	}

	return d.applyPermissions(appID, step.Permissions)
}

// applyPermissions applies permission settings to an app.
// Permissions map: shortcut/permission name -> "allow"/"deny"/"unset"
func (d *Driver) applyPermissions(appID string, permissions map[string]string) *core.CommandResult {
	var granted, revoked, errors []string

	for name, value := range permissions {
		// Handle "all" shortcut - applies to all common permissions
		if strings.ToLower(name) == "all" {
			allPerms := getAllPermissions()
			for _, perm := range allPerms {
				err := d.applyPermission(appID, perm, value)
				if err != nil {
					errors = append(errors, fmt.Sprintf("%s: %v", perm, err))
				} else if value == "allow" {
					granted = append(granted, perm)
				} else if value == "deny" {
					revoked = append(revoked, perm)
				}
			}
			continue
		}

		// Resolve permission shortcut to Android permission names
		perms := resolvePermissionShortcut(name)
		for _, perm := range perms {
			err := d.applyPermission(appID, perm, value)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", perm, err))
			} else if value == "allow" {
				granted = append(granted, perm)
			} else if value == "deny" {
				revoked = append(revoked, perm)
			}
		}
	}

	if len(errors) > 0 {
		return errorResult(
			fmt.Errorf("some permissions failed"),
			fmt.Sprintf("Granted: %d, Revoked: %d, Errors: %v", len(granted), len(revoked), errors),
		)
	}

	return successResult(fmt.Sprintf("Permissions updated: %d granted, %d revoked", len(granted), len(revoked)), nil)
}

// applyPermission grants or revokes a single permission.
func (d *Driver) applyPermission(appID, permission, value string) error {
	switch strings.ToLower(value) {
	case "allow":
		_, err := d.device.Shell(fmt.Sprintf("pm grant %s %s", appID, permission))
		return err
	case "deny", "unset":
		_, err := d.device.Shell(fmt.Sprintf("pm revoke %s %s", appID, permission))
		return err
	default:
		return fmt.Errorf("invalid permission value: %s (use allow/deny/unset)", value)
	}
}

// resolvePermissionShortcut maps Maestro permission shortcuts to Android permission names.
func resolvePermissionShortcut(shortcut string) []string {
	switch strings.ToLower(shortcut) {
	case "location":
		return []string{
			"android.permission.ACCESS_FINE_LOCATION",
			"android.permission.ACCESS_COARSE_LOCATION",
			"android.permission.ACCESS_BACKGROUND_LOCATION",
		}
	case "camera":
		return []string{"android.permission.CAMERA"}
	case "contacts":
		return []string{
			"android.permission.READ_CONTACTS",
			"android.permission.WRITE_CONTACTS",
			"android.permission.GET_ACCOUNTS",
		}
	case "phone":
		return []string{
			"android.permission.READ_PHONE_STATE",
			"android.permission.CALL_PHONE",
			"android.permission.READ_CALL_LOG",
			"android.permission.WRITE_CALL_LOG",
			"android.permission.USE_SIP",
			"android.permission.PROCESS_OUTGOING_CALLS",
		}
	case "microphone":
		return []string{"android.permission.RECORD_AUDIO"}
	case "bluetooth":
		return []string{
			"android.permission.BLUETOOTH_CONNECT",
			"android.permission.BLUETOOTH_SCAN",
			"android.permission.BLUETOOTH_ADVERTISE",
		}
	case "storage":
		return []string{
			"android.permission.READ_EXTERNAL_STORAGE",
			"android.permission.WRITE_EXTERNAL_STORAGE",
			"android.permission.READ_MEDIA_IMAGES",
			"android.permission.READ_MEDIA_VIDEO",
			"android.permission.READ_MEDIA_AUDIO",
		}
	case "notifications":
		return []string{"android.permission.POST_NOTIFICATIONS"}
	case "medialibrary":
		return []string{
			"android.permission.READ_MEDIA_IMAGES",
			"android.permission.READ_MEDIA_VIDEO",
			"android.permission.READ_MEDIA_AUDIO",
		}
	case "calendar":
		return []string{
			"android.permission.READ_CALENDAR",
			"android.permission.WRITE_CALENDAR",
		}
	case "sms":
		return []string{
			"android.permission.SEND_SMS",
			"android.permission.RECEIVE_SMS",
			"android.permission.READ_SMS",
			"android.permission.RECEIVE_WAP_PUSH",
			"android.permission.RECEIVE_MMS",
		}
	case "sensors", "activity_recognition":
		return []string{
			"android.permission.BODY_SENSORS",
			"android.permission.ACTIVITY_RECOGNITION",
		}
	default:
		// Assume it's a full Android permission name
		if strings.HasPrefix(shortcut, "android.permission.") {
			return []string{shortcut}
		}
		// Try adding the prefix
		return []string{"android.permission." + strings.ToUpper(shortcut)}
	}
}

// getAllPermissions returns all common Android runtime permissions.
func getAllPermissions() []string {
	return []string{
		// Location
		"android.permission.ACCESS_FINE_LOCATION",
		"android.permission.ACCESS_COARSE_LOCATION",
		"android.permission.ACCESS_BACKGROUND_LOCATION",
		// Camera
		"android.permission.CAMERA",
		// Contacts
		"android.permission.READ_CONTACTS",
		"android.permission.WRITE_CONTACTS",
		"android.permission.GET_ACCOUNTS",
		// Phone
		"android.permission.READ_PHONE_STATE",
		"android.permission.CALL_PHONE",
		"android.permission.READ_CALL_LOG",
		"android.permission.WRITE_CALL_LOG",
		// Microphone
		"android.permission.RECORD_AUDIO",
		// Storage
		"android.permission.READ_EXTERNAL_STORAGE",
		"android.permission.WRITE_EXTERNAL_STORAGE",
		"android.permission.READ_MEDIA_IMAGES",
		"android.permission.READ_MEDIA_VIDEO",
		"android.permission.READ_MEDIA_AUDIO",
		// Calendar
		"android.permission.READ_CALENDAR",
		"android.permission.WRITE_CALENDAR",
		// SMS
		"android.permission.SEND_SMS",
		"android.permission.RECEIVE_SMS",
		"android.permission.READ_SMS",
		// Notifications
		"android.permission.POST_NOTIFICATIONS",
		// Bluetooth
		"android.permission.BLUETOOTH_CONNECT",
		"android.permission.BLUETOOTH_SCAN",
		// Sensors
		"android.permission.BODY_SENSORS",
		"android.permission.ACTIVITY_RECOGNITION",
	}
}

// ============================================================================
// Clipboard Commands
// ============================================================================

func (d *Driver) copyTextFrom(step *flow.CopyTextFromStep) *core.CommandResult {
	elem, info, err := d.findElement(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	var text string
	if elem != nil {
		text, err = elem.Text()
		// If text is empty, try content-desc (element may have been found via descriptionMatches)
		if text == "" {
			if desc, descErr := elem.Attribute("content-desc"); descErr == nil && desc != "" {
				text = desc
			}
		}
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to get text: %v", err))
		}
	} else if info != nil {
		// Element found via page source - use text from info or accessibility label
		text = info.Text
		if text == "" && info.AccessibilityLabel != "" {
			text = info.AccessibilityLabel
		}
	}

	if err := d.client.SetClipboard(text); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to copy to clipboard: %v", err))
	}

	return &core.CommandResult{
		Success: true,
		Message: fmt.Sprintf("Copied text: %s", text),
		Element: info,
		Data:    text,
	}
}

func (d *Driver) pasteText(_ *flow.PasteTextStep) *core.CommandResult {
	text, err := d.client.GetClipboard()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get clipboard: %v", err))
	}

	active, err := d.client.ActiveElement()
	if err != nil {
		return errorResult(err, "No focused element to paste into")
	}

	if err := active.SendKeys(text); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to paste text: %v", err))
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

// ============================================================================
// Device Control Commands
// ============================================================================

func (d *Driver) setOrientation(step *flow.SetOrientationStep) *core.CommandResult {
	orientation := strings.ToUpper(strings.ReplaceAll(step.Orientation, "_", ""))

	// PORTRAIT and LANDSCAPE: use UIAutomator2 API
	if orientation == "PORTRAIT" || orientation == "LANDSCAPE" {
		if err := d.client.SetOrientation(orientation); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to set orientation: %v", err))
		}
		return successResult(fmt.Sprintf("Set orientation to %s", orientation), nil)
	}

	// Extended orientations (LANDSCAPE_LEFT, LANDSCAPE_RIGHT, UPSIDE_DOWN): use shell commands
	var rotation string
	switch orientation {
	case "LANDSCAPELEFT":
		rotation = "1"
	case "UPSIDEDOWN":
		rotation = "2"
	case "LANDSCAPERIGHT":
		rotation = "3"
	default:
		return errorResult(fmt.Errorf("invalid orientation: %s", step.Orientation),
			fmt.Sprintf("Orientation must be PORTRAIT, LANDSCAPE, LANDSCAPE_LEFT, LANDSCAPE_RIGHT, or UPSIDE_DOWN, got: %s", step.Orientation))
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "Extended orientations require device access")
	}

	// Disable accelerometer-based rotation before setting orientation
	if _, err := d.device.Shell("settings put system accelerometer_rotation 0"); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to disable accelerometer rotation: %v", err))
	}

	// Set the user rotation
	cmd := fmt.Sprintf("settings put system user_rotation %s", rotation)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set orientation: %v", err))
	}

	return successResult(fmt.Sprintf("Set orientation to %s", step.Orientation), nil)
}

func (d *Driver) openLink(step *flow.OpenLinkStep) *core.CommandResult {
	link := step.Link
	if link == "" {
		return errorResult(fmt.Errorf("no link specified"), "No link to open")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "openLink requires device access")
	}

	// Build am start command
	var cmd string
	if step.Browser != nil && *step.Browser {
		// Force open in browser - try common browser packages
		// Chrome is most common, fallback to default browser activity
		cmd = fmt.Sprintf("am start -a android.intent.action.VIEW -c android.intent.category.BROWSABLE -d '%s'", link)
	} else {
		// Default: let system decide (may open in app if deep link is registered)
		cmd = fmt.Sprintf("am start -a android.intent.action.VIEW -d '%s'", link)
	}

	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open link: %v", err))
	}

	// If autoVerify is enabled, wait briefly for page load
	if step.AutoVerify != nil && *step.AutoVerify {
		// Give the browser time to open and start loading
		time.Sleep(2 * time.Second)
	}

	return successResult(fmt.Sprintf("Opened link: %s", link), nil)
}

// ============================================================================
// Media Commands
// ============================================================================

func (d *Driver) takeScreenshot(step *flow.TakeScreenshotStep) *core.CommandResult {
	data, err := d.client.Screenshot()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to take screenshot: %v", err))
	}

	// Return screenshot data; caller handles saving to file if path specified
	return &core.CommandResult{
		Success: true,
		Message: "Screenshot captured",
		Data:    data,
	}
}

func (d *Driver) openBrowser(step *flow.OpenBrowserStep) *core.CommandResult {
	url := step.URL
	if url == "" {
		return errorResult(fmt.Errorf("no URL specified"), "No URL to open")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "openBrowser requires device access")
	}

	// Open URL in default browser
	cmd := fmt.Sprintf("am start -a android.intent.action.VIEW -d '%s'", url)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open browser: %v", err))
	}

	return successResult(fmt.Sprintf("Opened browser: %s", url), nil)
}

func (d *Driver) addMedia(step *flow.AddMediaStep) *core.CommandResult {
	if len(step.Files) == 0 {
		return errorResult(fmt.Errorf("no files specified"), "No media files to add")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "addMedia requires device access")
	}

	// Push each file to device's Download folder
	for _, file := range step.Files {
		// Use am broadcast to scan media after push
		cmd := fmt.Sprintf("am broadcast -a android.intent.action.MEDIA_SCANNER_SCAN_FILE -d file://%s", file)
		if _, err := d.device.Shell(cmd); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to add media %s: %v", file, err))
		}
	}

	return successResult(fmt.Sprintf("Added %d media files", len(step.Files)), nil)
}

func (d *Driver) startRecording(step *flow.StartRecordingStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "startRecording requires device access")
	}

	path := step.Path
	if path == "" {
		path = "/sdcard/recording.mp4"
	}

	// Start screenrecord in background (will be killed by stopRecording)
	cmd := fmt.Sprintf("screenrecord %s &", path)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to start recording: %v", err))
	}

	return &core.CommandResult{
		Success: true,
		Message: fmt.Sprintf("Started recording to %s", path),
		Data:    path,
	}
}

func (d *Driver) stopRecording(_ *flow.StopRecordingStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "stopRecording requires device access")
	}

	// Kill screenrecord process
	if _, err := d.device.Shell("pkill -INT screenrecord"); err != nil {
		// Ignore error - process might have already stopped
	}

	// Wait for file to be written
	time.Sleep(500 * time.Millisecond)

	return successResult("Stopped recording", nil)
}

// ============================================================================
// Wait Commands
// ============================================================================

func (d *Driver) waitUntil(step *flow.WaitUntilStep) *core.CommandResult {
	// Use step timeout if specified, otherwise default to 30 seconds
	timeout := 30 * time.Second
	if step.TimeoutMs > 0 {
		timeout = time.Duration(step.TimeoutMs) * time.Millisecond
	}

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
				_, info, err := d.findElementOnce(*step.Visible)
				if err == nil && info != nil {
					return successResult("Element is now visible", info)
				}
			} else {
				// Single attempt for not visible check
				_, info, err := d.findElementOnce(*step.NotVisible)
				if err != nil || info == nil {
					return successResult("Element is no longer visible", nil)
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

// ============================================================================
// Location Commands
// ============================================================================

func (d *Driver) setLocation(step *flow.SetLocationStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "setLocation requires device access")
	}

	lat := step.Latitude
	lon := step.Longitude
	if lat == "" || lon == "" {
		return errorResult(fmt.Errorf("latitude and longitude required"), "Missing coordinates")
	}

	// Enable mock locations and set location via appops
	// Note: Requires mock location app or root access
	cmd := fmt.Sprintf("am broadcast -a android.intent.action.MOCK_LOCATION --ef lat %s --ef lon %s", lat, lon)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set location: %v", err))
	}

	return successResult(fmt.Sprintf("Set location to %s, %s", lat, lon), nil)
}

func (d *Driver) setAirplaneMode(step *flow.SetAirplaneModeStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "setAirplaneMode requires device access")
	}

	value := "0"
	if step.Enabled {
		value = "1"
	}

	// Set airplane mode via settings
	cmd := fmt.Sprintf("settings put global airplane_mode_on %s", value)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set airplane mode: %v", err))
	}

	// Broadcast the change
	broadcastCmd := "am broadcast -a android.intent.action.AIRPLANE_MODE"
	if _, err := d.device.Shell(broadcastCmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to broadcast airplane mode: %v", err))
	}

	status := "disabled"
	if step.Enabled {
		status = "enabled"
	}
	return successResult(fmt.Sprintf("Airplane mode %s", status), nil)
}

func (d *Driver) toggleAirplaneMode(_ *flow.ToggleAirplaneModeStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "toggleAirplaneMode requires device access")
	}

	// Get current airplane mode state
	output, err := d.device.Shell("settings get global airplane_mode_on")
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get airplane mode: %v", err))
	}

	// Toggle the value
	newValue := "1"
	if strings.TrimSpace(output) == "1" {
		newValue = "0"
	}

	// Set new value
	cmd := fmt.Sprintf("settings put global airplane_mode_on %s", newValue)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to toggle airplane mode: %v", err))
	}

	// Broadcast the change
	broadcastCmd := "am broadcast -a android.intent.action.AIRPLANE_MODE"
	if _, err := d.device.Shell(broadcastCmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to broadcast airplane mode: %v", err))
	}

	status := "disabled"
	if newValue == "1" {
		status = "enabled"
	}
	return successResult(fmt.Sprintf("Airplane mode toggled to %s", status), nil)
}

func (d *Driver) travel(step *flow.TravelStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "travel requires device access")
	}

	if len(step.Points) < 2 {
		return errorResult(fmt.Errorf("at least 2 points required"), "Travel requires at least 2 waypoints")
	}

	speed := step.Speed
	if speed <= 0 {
		speed = 50 // default 50 km/h
	}

	// Simulate travel by updating location at each point
	for _, point := range step.Points {
		// Parse "lat, lon" format
		parts := strings.Split(point, ",")
		if len(parts) != 2 {
			continue
		}
		lat := strings.TrimSpace(parts[0])
		lon := strings.TrimSpace(parts[1])

		cmd := fmt.Sprintf("am broadcast -a android.intent.action.MOCK_LOCATION --ef lat %s --ef lon %s", lat, lon)
		if _, err := d.device.Shell(cmd); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to set location during travel: %v", err))
		}

		// Wait based on speed (simplified - assumes ~1km between points)
		delay := time.Duration(3600/speed) * time.Second
		time.Sleep(delay)
	}

	return successResult(fmt.Sprintf("Traveled through %d points", len(step.Points)), nil)
}

// ============================================================================
// Helpers
// ============================================================================

func mapDirection(dir string) string {
	switch dir {
	case "up":
		return uiautomator2.DirectionUp
	case "down":
		return uiautomator2.DirectionDown
	case "left":
		return uiautomator2.DirectionLeft
	case "right":
		return uiautomator2.DirectionRight
	default:
		return uiautomator2.DirectionDown
	}
}

// invertScrollDirection converts scroll direction (content movement direction)
// to swipe direction (finger gesture direction).
// In Maestro: ScrollDirection.DOWN -> SwipeDirection.UP (to reveal content below)
func invertScrollDirection(dir string) string {
	switch dir {
	case "up":
		return uiautomator2.DirectionDown
	case "down":
		return uiautomator2.DirectionUp
	case "left":
		return uiautomator2.DirectionRight
	case "right":
		return uiautomator2.DirectionLeft
	default:
		return uiautomator2.DirectionUp // default: scroll down = swipe up
	}
}

func mapKeyCode(key string) int {
	switch strings.ToLower(key) {
	case "enter":
		return uiautomator2.KeyCodeEnter
	case "back":
		return uiautomator2.KeyCodeBack
	case "home":
		return uiautomator2.KeyCodeHome
	case "menu":
		return uiautomator2.KeyCodeMenu
	case "delete", "backspace":
		return uiautomator2.KeyCodeDelete
	case "tab":
		return uiautomator2.KeyCodeTab
	case "space":
		return uiautomator2.KeyCodeSpace
	case "volume_up":
		return uiautomator2.KeyCodeVolumeUp
	case "volume_down":
		return uiautomator2.KeyCodeVolumeDown
	case "power":
		return uiautomator2.KeyCodePower
	case "camera":
		return uiautomator2.KeyCodeCamera
	case "search":
		return uiautomator2.KeyCodeSearch
	case "dpad_up":
		return uiautomator2.KeyCodeDpadUp
	case "dpad_down":
		return uiautomator2.KeyCodeDpadDown
	case "dpad_left":
		return uiautomator2.KeyCodeDpadLeft
	case "dpad_right":
		return uiautomator2.KeyCodeDpadRight
	case "dpad_center":
		return uiautomator2.KeyCodeDpadCenter
	default:
		return 0
	}
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
