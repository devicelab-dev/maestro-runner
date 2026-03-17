package devicelab

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// ============================================================================
// Tap Commands
// ============================================================================

func (d *Driver) tapOn(step *flow.TapOnStep) *core.CommandResult {
	// Check if using Point WITHOUT selector (screen-relative tap)
	if step.Point != "" && step.Selector.IsEmpty() {
		return d.tapOnPointWithCoords(step.Point)
	}

	wasInput := d.consumeInputFlag()

	// Quick check: if previous step was input and keyboard is blocking, fail fast
	if result := d.checkKeyboardBlocking(wasInput, step.Selector); result != nil {
		return result
	}

	// For text-based and ID-based taps, use FindAndClick —
	// single atomic Java call: find node + coordinate click at center.
	// No stale nodes, no performAction, no parent walk-up.
	// Prevents stale coordinates caused by Compose recomposition between find and click.
	if (step.Selector.Text != "" || step.Selector.ID != "") && step.Point == "" && !step.Selector.HasRelativeSelector() {
		strategies, err := buildSelectors(step.Selector, 0)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to build selectors: %v", err))
		}
		timeout := d.calculateTimeout(step.IsOptional(), step.TimeoutMs)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		var lastErr error
		for {
			select {
			case <-ctx.Done():
				if lastErr != nil {
					return errorResult(fmt.Errorf("%s: %w", ctx.Err(), lastErr), fmt.Sprintf("Element not found: %v", lastErr))
				}
				return errorResult(ctx.Err(), fmt.Sprintf("Element not found: %v", ctx.Err()))
			default:
				for _, s := range strategies {
					elem, err := d.client.FindAndClick(s.Strategy, s.Value)
					if err == nil {
						info := &core.ElementInfo{
							Visible: true,
							Enabled: true,
						}
						if t, err := elem.Text(); err == nil {
							info.Text = t
						}
						if rect, err := elem.Rect(); err == nil {
							info.Bounds = core.Bounds{X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height}
						}
						return successResult("Tapped on element", info)
					}
					lastErr = err
				}
			}
		}
	}

	_, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}
	if info == nil {
		return errorResult(fmt.Errorf("nil element info"), "Element info is nil")
	}

	// If Point is specified WITH selector, tap at relative position within element bounds
	if step.Point != "" && info.Bounds.Width > 0 {
		x, y, parseErr := core.ParsePointCoords(step.Point, info.Bounds.Width, info.Bounds.Height)
		if parseErr != nil {
			return errorResult(parseErr, fmt.Sprintf("Invalid point coordinates: %v", parseErr))
		}
		x += info.Bounds.X
		y += info.Bounds.Y
		if err := d.client.Click(x, y); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to tap at relative point: %v", err))
		}
		return successResult(fmt.Sprintf("Tapped at relative point (%d, %d) on element", x, y), info)
	}

	// Always use coordinate-based tap (not accessibility performAction).
	// Coordinate taps simulate real touch events, which work reliably for
	// repeated taps on the same button and custom click handlers.
	x, y := info.Bounds.Center()
	if err := d.client.Click(x, y); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to tap at coordinates: %v", err))
	}

	return successResult("Tapped on element", info)
}

// tapOnPointWithCoords handles point-based tap with either percentage or absolute coordinates.
func (d *Driver) tapOnPointWithCoords(point string) *core.CommandResult {
	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get screen size: %v", err))
	}

	x, y, err := core.ParsePointCoords(point, width, height)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid point coordinates: %v", err))
	}

	if err := d.client.Click(x, y); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to tap at point: %v", err))
	}

	return successResult(fmt.Sprintf("Tapped at (%d, %d)", x, y), nil)
}

func (d *Driver) doubleTapOn(step *flow.DoubleTapOnStep) *core.CommandResult {
	wasInput := d.consumeInputFlag()

	if result := d.checkKeyboardBlocking(wasInput, step.Selector); result != nil {
		return result
	}

	_, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	x, y := info.Bounds.Center()
	if err := d.client.DoubleClick(x, y); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to double tap at coordinates: %v", err))
	}

	return successResult("Double tapped on element", info)
}

func (d *Driver) longPressOn(step *flow.LongPressOnStep) *core.CommandResult {
	wasInput := d.consumeInputFlag()

	if result := d.checkKeyboardBlocking(wasInput, step.Selector); result != nil {
		return result
	}

	_, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	duration := 1000 // default 1 second

	x, y := info.Bounds.Center()
	if err := d.client.LongClick(x, y, duration); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to long press at coordinates: %v", err))
	}

	return successResult("Long pressed on element", info)
}

func (d *Driver) tapOnPoint(step *flow.TapOnPointStep) *core.CommandResult {
	x, y := step.X, step.Y

	if step.Point != "" {
		width, height, err := d.screenSize()
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to get screen size: %v", err))
		}

		x, y, err = core.ParsePointCoords(step.Point, width, height)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Invalid point coordinates: %v", err))
		}
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
	wasInput := d.consumeInputFlag()

	if result := d.checkKeyboardBlocking(wasInput, step.Selector); result != nil {
		return result
	}

	_, info, err := d.findElementFast(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not visible: %v", err))
	}

	if info != nil && info.Visible {
		return successResult("Element is visible", info)
	}

	return errorResult(fmt.Errorf("element not visible"), "Element exists but is not visible")
}

func (d *Driver) assertNotVisible(step *flow.AssertNotVisibleStep) *core.CommandResult {
	timeout := step.TimeoutMs
	if timeout <= 0 {
		timeout = 5000
	}

	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	pollInterval := 500 * time.Millisecond

	for {
		_, info, err := d.findElementQuick(step.Selector, 0)
		if err != nil || info == nil {
			return successResult("Element is not visible", nil)
		}

		if time.Now().After(deadline) {
			return errorResult(fmt.Errorf("element is visible"), "Element should not be visible but was found")
		}

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

	unicodeWarning := ""
	if core.HasNonASCII(text) {
		unicodeWarning = " (warning: non-ASCII characters may not input correctly)"
	}

	if step.KeyPress {
		if err := d.client.SendKeyActions(text); err != nil {
			return errorResult(err, "Failed to input text via key press")
		}
		return successResult(fmt.Sprintf("Entered text (keyPress): %s%s", text, unicodeWarning), nil)
	}

	if !step.Selector.IsEmpty() {
		elem, _, err := d.findElement(step.Selector, step.IsOptional(), step.TimeoutMs)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Element not found: %v", err))
		}
		if err := elem.SendKeys(text); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to input text: %v", err))
		}
	} else {
		active, err := d.client.ActiveElement()
		if err != nil {
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
			// Fallback to SendKeyActions when SendKeys fails (e.g., stale element
			// reference from Compose recomposition after tapping an EditText).
			logger.Warn("SendKeys failed (%v), falling back to SendKeyActions", err)
			if keyErr := d.client.SendKeyActions(text); keyErr != nil {
				return errorResult(err, fmt.Sprintf("Failed to input text: %v (SendKeyActions fallback also failed: %v)", err, keyErr))
			}
			return successResult(fmt.Sprintf("Entered text (fallback): %s%s", text, unicodeWarning), nil)
		}
	}

	return successResult(fmt.Sprintf("Entered text: %s%s", text, unicodeWarning), nil)
}

func (d *Driver) eraseText(step *flow.EraseTextStep) *core.CommandResult {
	chars := step.Characters
	if chars <= 0 {
		chars = 50
	}

	active, err := d.client.ActiveElement()
	if err == nil {
		currentText, textErr := active.Text()
		if textErr == nil {
			textLen := len([]rune(currentText))

			if chars >= textLen || textLen == 0 {
				if clearErr := active.Clear(); clearErr == nil {
					return successResult(fmt.Sprintf("Cleared %d characters", textLen), nil)
				}
			} else {
				runes := []rune(currentText)
				remaining := string(runes[:textLen-chars])

				if clearErr := active.Clear(); clearErr == nil {
					if remaining != "" {
						if sendErr := active.SendKeys(remaining); sendErr == nil {
							return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
						}
					} else {
						return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
					}
				}
			}
		}
	}

	for i := 0; i < chars; i++ {
		if err := d.client.PressKeyCode(uiautomator2.KeyCodeDelete); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to erase text: %v", err))
		}
	}

	return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
}

func (d *Driver) hideKeyboard(_ *flow.HideKeyboardStep) *core.CommandResult {
	if err := d.client.HideKeyboard(); err != nil {
		return successResult("Hide keyboard (may not have been visible)", nil)
	}
	return successResult("Keyboard hidden", nil)
}

func (d *Driver) inputRandom(step *flow.InputRandomStep) *core.CommandResult {
	length := step.Length
	if length <= 0 {
		length = 10
	}

	var text string
	dataType := strings.ToUpper(step.DataType)
	switch dataType {
	case "EMAIL":
		text = core.RandomEmail()
	case "NUMBER":
		text = core.RandomNumber(length)
	case "PERSON_NAME":
		text = core.RandomPersonName()
	default:
		text = core.RandomString(length)
	}

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

	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}

	area := uiautomator2.NewRect(0, height/8, width, height*3/4)

	if err := d.client.ScrollInArea(area, direction, 0.5, 0); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to scroll: %v", err))
	}

	return successResult(fmt.Sprintf("Scrolled %s", direction), nil)
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

	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}

	area := uiautomator2.NewRect(0, height/8, width, height*3/4)

	for i := 0; i < maxScrolls && time.Now().Before(deadline); i++ {
		_, info, err := d.findElement(step.Element, true, 1000)
		if err == nil && info != nil {
			return successResult(fmt.Sprintf("Element found after %d scrolls", i), info)
		}

		if err := d.client.ScrollInArea(area, direction, 0.3, 0); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to scroll: %v", err))
		}

		time.Sleep(300 * time.Millisecond)
	}

	return errorResult(fmt.Errorf("element not found"), fmt.Sprintf("Element not found after %d scrolls", maxScrolls))
}

func (d *Driver) swipe(step *flow.SwipeStep) *core.CommandResult {
	if step.Start != "" && step.End != "" {
		return d.swipeWithCoordinates(step.Start, step.End, step.Duration)
	}

	if step.StartX > 0 || step.StartY > 0 || step.EndX > 0 || step.EndY > 0 {
		return d.swipeWithAbsoluteCoords(step.StartX, step.StartY, step.EndX, step.EndY, step.Duration)
	}

	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "up"
	}

	uiaDir := mapDirection(direction)

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

	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}

	scrollableInfo, scrollableCount := d.findScrollableElement(10000)

	if scrollableInfo != nil {
		b := scrollableInfo.Bounds
		fmt.Printf("[swipe] Found %d scrollable(s), using: bounds=[%d,%d,%d,%d]\n",
			scrollableCount, b.X, b.Y, b.Width, b.Height)

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
	return d.swipeWithMaestroCoordinates(direction, width, height, step.Duration)
}

// findScrollableElement waits for and finds a scrollable element.
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

		if len(scrollables) == 1 {
			elem := scrollables[0]
			return &core.ElementInfo{
				Bounds: elem.Bounds,
			}, 1
		}

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
		startX = width / 2
		startY = height * 70 / 100
		endX = width / 2
		endY = height * 30 / 100
	}

	fmt.Printf("[swipe] Using screen coords: (%d,%d) → (%d,%d)\n", startX, startY, endX, endY)
	return d.swipeWithAbsoluteCoords(startX, startY, endX, endY, durationMs)
}

func (d *Driver) swipeWithCoordinates(start, end string, durationMs int) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "swipe with coordinates requires device access")
	}

	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get screen size: %v", err))
	}

	startXPct, startYPct, err := core.ParsePercentageCoords(start)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid start coordinates: %v", err))
	}

	endXPct, endYPct, err := core.ParsePercentageCoords(end)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid end coordinates: %v", err))
	}

	startX := int(float64(width) * startXPct)
	startY := int(float64(height) * startYPct)
	endX := int(float64(width) * endXPct)
	endY := int(float64(height) * endYPct)

	return d.swipeWithAbsoluteCoords(startX, startY, endX, endY, durationMs)
}

func (d *Driver) swipeWithAbsoluteCoords(startX, startY, endX, endY, durationMs int) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "swipe with coordinates requires device access")
	}

	if durationMs <= 0 {
		durationMs = 300
	}

	cmd := fmt.Sprintf("input swipe %d %d %d %d %d", startX, startY, endX, endY, durationMs)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to swipe: %v", err))
	}

	return successResult(fmt.Sprintf("Swiped from (%d,%d) to (%d,%d)", startX, startY, endX, endY), nil)
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
		return errorResult(fmt.Errorf("no appId specified"), "launchApp: no appId specified in flow")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "launchApp: no device connected — check ADB connection")
	}

	// 1. Clear state or force-stop via RPC (no USB round-trips)
	if step.ClearState {
		if err := d.client.ClearAppData(appID); err != nil {
			// RPC failed — fall back to shell
			logger.Warn("launchApp: RPC clearAppData failed for %s: %v — falling back to shell", appID, err)
			if _, shellErr := d.device.Shell("pm clear " + appID); shellErr != nil {
				return errorResult(shellErr, fmt.Sprintf("launchApp: failed to clear app state for '%s' — is the app installed?", appID))
			}
		}
	} else if step.StopApp == nil || *step.StopApp {
		if err := d.client.ForceStop(appID); err != nil {
			logger.Warn("launchApp: RPC forceStop failed for %s: %v — falling back to shell", appID, err)
			if _, shellErr := d.device.Shell("am force-stop " + appID); shellErr != nil {
				logger.Warn("failed to force-stop app %s: %v", appID, shellErr)
			}
		}
	}

	// 2. Permissions via RPC
	permissions := step.Permissions
	if len(permissions) == 0 {
		permissions = map[string]string{"all": "allow"}
	}
	var toGrant []string
	for name, value := range permissions {
		if strings.ToLower(value) != "allow" {
			continue
		}
		if strings.ToLower(name) == "all" {
			toGrant = append(toGrant, getAllPermissions()...)
		} else {
			toGrant = append(toGrant, resolvePermissionShortcut(name)...)
		}
	}
	if len(toGrant) > 0 {
		if err := d.client.GrantPermissions(appID, toGrant); err != nil {
			logger.Warn("launchApp: RPC grantPermissions failed for %s: %v — falling back to shell", appID, err)
			d.applyPermissions(appID, permissions)
		}
	}

	// 3. Launch via RPC
	var arguments map[string]interface{}
	if len(step.Arguments) > 0 {
		arguments = step.Arguments
	}
	if err := d.client.LaunchApp(appID, arguments); err != nil {
		// RPC launch failed — fall back to shell
		logger.Warn("launchApp: RPC launch failed for %s: %v — falling back to shell", appID, err)
		return d.launchAppViaShell(appID, arguments)
	}

	return successResult(fmt.Sprintf("Launched app: %s", appID), nil)
}

// launchAppViaShell launches an app using ADB shell commands.
func (d *Driver) launchAppViaShell(appID string, arguments map[string]interface{}) *core.CommandResult {
	apiLevel := d.getAPILevel()

	if apiLevel < 24 && len(arguments) == 0 {
		return d.launchWithMonkey(appID)
	}

	activity, err := d.resolveLauncherActivityCached(appID, apiLevel)
	if err != nil {
		if len(arguments) == 0 {
			logger.Warn("launchApp: activity resolution failed for %s: %v — trying monkey", appID, err)
			return d.launchWithMonkey(appID)
		}
		return errorResult(err, fmt.Sprintf(
			"launchApp: cannot find launcher activity for '%s' — %v. "+
				"Is the app installed? Check with: adb shell pm list packages | grep %s", appID, err, appID))
	}

	amCmd := "am start"
	if apiLevel >= 26 {
		amCmd = "am start-activity"
	}

	cmd := fmt.Sprintf("%s -W -n %s -a android.intent.action.MAIN -c android.intent.category.LAUNCHER -f 0x10200000",
		amCmd, activity)

	for key, value := range arguments {
		switch v := value.(type) {
		case string:
			cmd += fmt.Sprintf(" --es %s '%s'", key, v)
		case int:
			cmd += fmt.Sprintf(" --ei %s %d", key, v)
		case int64:
			cmd += fmt.Sprintf(" --ei %s %d", key, v)
		case float64:
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

	output, err := d.device.Shell(cmd)
	if err != nil || strings.Contains(output, "Error") {
		if strings.Contains(output, "does not exist") || strings.Contains(output, "ClassNotFoundException") {
			dotActivity := d.addDotPrefix(activity)
			if dotActivity != activity {
				logger.Info("launchApp: retrying with dot-prefixed activity: %s", dotActivity)
				retryCmd := strings.Replace(cmd, activity, dotActivity, 1)
				if output2, err2 := d.device.Shell(retryCmd); err2 == nil && !strings.Contains(output2, "Error") {
					return successResult(fmt.Sprintf("Launched app: %s", appID), nil)
				}
			}
		}

		if len(arguments) == 0 {
			logger.Warn("launchApp: am start failed for %s: %v — trying monkey", appID, err)
			return d.launchWithMonkey(appID)
		}
		errMsg := fmt.Sprintf("launchApp: '%s' failed for '%s' activity '%s'", amCmd, appID, activity)
		if err != nil {
			return errorResult(err, errMsg)
		}
		return errorResult(fmt.Errorf("am start returned error: %s", strings.TrimSpace(output)), errMsg)
	}

	return successResult(fmt.Sprintf("Launched app: %s", appID), nil)
}

// getAPILevel returns the device's Android API level, or 24 as a safe default.
func (d *Driver) getAPILevel() int {
	if d.cachedAPILevel > 0 {
		return d.cachedAPILevel
	}
	output, err := d.device.Shell("getprop ro.build.version.sdk")
	if err != nil {
		return 24
	}
	level, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 24
	}
	d.cachedAPILevel = level
	return level
}

// resolveLauncherActivityCached resolves the launcher activity with caching.
func (d *Driver) resolveLauncherActivityCached(appID string, apiLevel int) (string, error) {
	if d.cachedActivities != nil {
		if activity, ok := d.cachedActivities[appID]; ok {
			return activity, nil
		}
	}
	activity, err := d.resolveLauncherActivity(appID, apiLevel)
	if err != nil {
		return "", err
	}
	if d.cachedActivities == nil {
		d.cachedActivities = make(map[string]string)
	}
	d.cachedActivities[appID] = activity
	return activity, nil
}

// resolveLauncherActivity resolves the launcher activity for a package.
func (d *Driver) resolveLauncherActivity(appID string, apiLevel int) (string, error) {
	if apiLevel >= 24 {
		resolveCmd := fmt.Sprintf("cmd package resolve-activity --brief -a android.intent.action.MAIN -c android.intent.category.LAUNCHER %s | tail -n 1", appID)
		output, err := d.device.Shell(resolveCmd)
		if err == nil {
			activity := strings.TrimSpace(output)
			if activity != "" &&
				!strings.Contains(activity, "No activity found") &&
				!strings.Contains(activity, "ResolverActivity") &&
				strings.Contains(activity, "/") {
				return activity, nil
			}
		}
	}

	return d.resolveLauncherFromDumpsys(appID)
}

// launchWithMonkey launches an app using the monkey command.
func (d *Driver) launchWithMonkey(appID string) *core.CommandResult {
	monkeyCmd := fmt.Sprintf("monkey -p %s -c android.intent.category.LAUNCHER 1", appID)
	output, err := d.device.Shell(monkeyCmd)
	if err != nil || strings.Contains(output, "monkey aborted") {
		errMsg := fmt.Sprintf("launchApp: all launch methods failed for '%s'. "+
			"The app may not be installed or has no launcher activity. "+
			"Check with: adb shell pm list packages | grep %s", appID, appID)
		if err != nil {
			return errorResult(err, errMsg)
		}
		return errorResult(fmt.Errorf("monkey aborted — no launchable activity for %s", appID), errMsg)
	}
	return successResult(fmt.Sprintf("Launched app: %s", appID), nil)
}

// addDotPrefix converts "com.app/MainActivity" to "com.app/.MainActivity".
func (d *Driver) addDotPrefix(activity string) string {
	parts := strings.SplitN(activity, "/", 2)
	if len(parts) != 2 {
		return activity
	}
	activityName := parts[1]
	if strings.HasPrefix(activityName, ".") || strings.Contains(activityName, ".") {
		return activity
	}
	return parts[0] + "/." + activityName
}

// resolveLauncherFromDumpsys parses `dumpsys package` output to find the MAIN/LAUNCHER activity.
func (d *Driver) resolveLauncherFromDumpsys(appID string) (string, error) {
	output, err := d.device.Shell(fmt.Sprintf("dumpsys package %s", appID))
	if err != nil {
		return "", fmt.Errorf("dumpsys failed for %s: %w", appID, err)
	}

	lines := strings.Split(output, "\n")
	inFilter := false
	hasMain := false
	hasLauncher := false
	var currentActivity string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, appID) && strings.Contains(trimmed, "/") && strings.Contains(trimmed, "filter") {
			if inFilter && hasMain && hasLauncher && currentActivity != "" {
				return currentActivity, nil
			}
			inFilter = true
			hasMain = false
			hasLauncher = false
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				currentActivity = parts[0]
			}
			continue
		}

		if inFilter {
			if strings.Contains(trimmed, "android.intent.action.MAIN") {
				hasMain = true
			}
			if strings.Contains(trimmed, "android.intent.category.LAUNCHER") {
				hasLauncher = true
			}
			if trimmed == "" || (!strings.HasPrefix(trimmed, "Action:") &&
				!strings.HasPrefix(trimmed, "Category:") &&
				!strings.HasPrefix(trimmed, "\"")) {
				if hasMain && hasLauncher && currentActivity != "" {
					return currentActivity, nil
				}
				inFilter = false
			}
		}
	}

	if hasMain && hasLauncher && currentActivity != "" {
		return currentActivity, nil
	}

	return "", fmt.Errorf("no MAIN/LAUNCHER activity found in dumpsys for %s", appID)
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

// applyPermissions applies permission settings to an app.
func (d *Driver) applyPermissions(appID string, permissions map[string]string) *core.CommandResult {
	var toGrant, toRevoke []string

	for name, value := range permissions {
		var perms []string
		if strings.ToLower(name) == "all" {
			perms = getAllPermissions()
		} else {
			perms = resolvePermissionShortcut(name)
		}

		switch strings.ToLower(value) {
		case "allow":
			toGrant = append(toGrant, perms...)
		case "deny", "unset":
			toRevoke = append(toRevoke, perms...)
		}
	}

	if len(toGrant) > 0 {
		var parts []string
		for _, perm := range toGrant {
			parts = append(parts, fmt.Sprintf("pm grant %s %s 2>/dev/null", appID, perm))
		}
		_, _ = d.device.Shell(strings.Join(parts, "; "))
	}

	if len(toRevoke) > 0 {
		var parts []string
		for _, perm := range toRevoke {
			parts = append(parts, fmt.Sprintf("pm revoke %s %s 2>/dev/null", appID, perm))
		}
		_, _ = d.device.Shell(strings.Join(parts, "; "))
	}

	return successResult(fmt.Sprintf("Permissions updated: %d granted, %d revoked", len(toGrant), len(toRevoke)), nil)
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
		if strings.HasPrefix(shortcut, "android.permission.") {
			return []string{shortcut}
		}
		return []string{"android.permission." + strings.ToUpper(shortcut)}
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
		"android.permission.RECORD_AUDIO",
		"android.permission.READ_EXTERNAL_STORAGE",
		"android.permission.WRITE_EXTERNAL_STORAGE",
		"android.permission.READ_MEDIA_IMAGES",
		"android.permission.READ_MEDIA_VIDEO",
		"android.permission.READ_MEDIA_AUDIO",
		"android.permission.READ_CALENDAR",
		"android.permission.WRITE_CALENDAR",
		"android.permission.SEND_SMS",
		"android.permission.RECEIVE_SMS",
		"android.permission.READ_SMS",
		"android.permission.POST_NOTIFICATIONS",
		"android.permission.BLUETOOTH_CONNECT",
		"android.permission.BLUETOOTH_SCAN",
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
		if text == "" {
			if desc, descErr := elem.Attribute("content-desc"); descErr == nil && desc != "" {
				text = desc
			}
		}
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to get text: %v", err))
		}
	} else if info != nil {
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

	if orientation == "PORTRAIT" || orientation == "LANDSCAPE" {
		if err := d.client.SetOrientation(orientation); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to set orientation: %v", err))
		}
		return successResult(fmt.Sprintf("Set orientation to %s", orientation), nil)
	}

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

	if _, err := d.device.Shell("settings put system accelerometer_rotation 0"); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to disable accelerometer rotation: %v", err))
	}

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

	var cmd string
	if step.Browser != nil && *step.Browser {
		cmd = fmt.Sprintf("am start -a android.intent.action.VIEW -c android.intent.category.BROWSABLE -d '%s'", link)
	} else {
		cmd = fmt.Sprintf("am start -a android.intent.action.VIEW -d '%s'", link)
	}

	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open link: %v", err))
	}

	if step.AutoVerify != nil && *step.AutoVerify {
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

	for _, file := range step.Files {
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

	if _, err := d.device.Shell("pkill -INT screenrecord"); err != nil {
		logger.Warn("failed to stop screenrecord process: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	return successResult("Stopped recording", nil)
}

// ============================================================================
// Wait Commands
// ============================================================================

func (d *Driver) waitUntil(step *flow.WaitUntilStep) *core.CommandResult {
	timeout := 30 * time.Second
	if step.TimeoutMs > 0 {
		timeout = time.Duration(step.TimeoutMs) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
				_, info, err := d.findElementOnce(*step.Visible)
				if err == nil && info != nil {
					return successResult("Element is now visible", info)
				}
			} else {
				_, info, err := d.findElementOnce(*step.NotVisible)
				if err != nil || info == nil {
					return successResult("Element is no longer visible", nil)
				}
			}
		}
	}
}

func (d *Driver) waitForAnimationToEnd(_ *flow.WaitForAnimationToEndStep) *core.CommandResult {
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

	cmd := fmt.Sprintf("am broadcast -a android.intent.action.MOCK_LOCATION --ef lat %s --ef lon %s", lat, lon)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set location: %v", err))
	}

	return successResult(fmt.Sprintf("Set location to %s, %s", lat, lon), nil)
}

// applyAirplaneMode sets airplane mode on/off using the best available method.
// Android 11+ (API 30+): "cmd connectivity airplane-mode" works without root.
// Older Android: "settings put global" + broadcast fallback.
func (d *Driver) applyAirplaneMode(enable bool) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "setAirplaneMode requires device access")
	}

	mode := "disable"
	status := "disabled"
	if enable {
		mode = "enable"
		status = "enabled"
	}

	// Try "cmd connectivity airplane-mode" first (Android 11+ / API 30+)
	cmdStr := fmt.Sprintf("cmd connectivity airplane-mode %s", mode)
	out, err := d.device.Shell(cmdStr)
	if err == nil && !strings.Contains(out, "Unknown command") {
		return successResult(fmt.Sprintf("Airplane mode %s", status), nil)
	}

	// Fallback: settings put + broadcast (older Android)
	value := "0"
	if enable {
		value = "1"
	}
	settingsCmd := fmt.Sprintf("settings put global airplane_mode_on %s", value)
	if _, err := d.device.Shell(settingsCmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set airplane mode: %v", err))
	}

	// Broadcast may fail on Android 7+ without root — warn but don't fail
	broadcastCmd := "am broadcast -a android.intent.action.AIRPLANE_MODE"
	if _, err := d.device.Shell(broadcastCmd); err != nil {
		logger.Warn("airplane mode broadcast failed (expected on Android 7+ without root): %v", err)
	}

	return successResult(fmt.Sprintf("Airplane mode %s", status), nil)
}

func (d *Driver) setAirplaneMode(step *flow.SetAirplaneModeStep) *core.CommandResult {
	return d.applyAirplaneMode(step.Enabled)
}

func (d *Driver) toggleAirplaneMode(_ *flow.ToggleAirplaneModeStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "toggleAirplaneMode requires device access")
	}

	output, err := d.device.Shell("settings get global airplane_mode_on")
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get airplane mode: %v", err))
	}

	enable := strings.TrimSpace(output) != "1"
	return d.applyAirplaneMode(enable)
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
		speed = 50
	}

	for _, point := range step.Points {
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
