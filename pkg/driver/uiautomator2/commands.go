package uiautomator2

import (
	"fmt"
	"math/rand"
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
	elem, info, err := d.findElement(step.Selector, step.IsOptional())
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
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

func (d *Driver) doubleTapOn(step *flow.DoubleTapOnStep) *core.CommandResult {
	elem, info, err := d.findElement(step.Selector, step.IsOptional())
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
	elem, info, err := d.findElement(step.Selector, step.IsOptional())
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
	if err := d.client.Click(step.X, step.Y); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to tap at point: %v", err))
	}

	return successResult(fmt.Sprintf("Tapped at (%d, %d)", step.X, step.Y), nil)
}

// ============================================================================
// Assert Commands
// ============================================================================

func (d *Driver) assertVisible(step *flow.AssertVisibleStep) *core.CommandResult {
	elem, info, err := d.findElement(step.Selector, step.IsOptional())
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not visible: %v", err))
	}

	displayed, err := elem.IsDisplayed()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to check visibility: %v", err))
	}

	if !displayed {
		return errorResult(fmt.Errorf("element not visible"), "Element exists but is not visible")
	}

	return successResult("Element is visible", info)
}

func (d *Driver) assertNotVisible(step *flow.AssertNotVisibleStep) *core.CommandResult {
	_, _, err := d.findElement(step.Selector, step.IsOptional())
	if err != nil {
		// Element not found = not visible = success
		return successResult("Element is not visible", nil)
	}

	return errorResult(fmt.Errorf("element is visible"), "Element should not be visible but was found")
}

// ============================================================================
// Input Commands
// ============================================================================

func (d *Driver) inputText(step *flow.InputTextStep) *core.CommandResult {
	text := step.Text
	if text == "" {
		return errorResult(fmt.Errorf("no text specified"), "No text to input")
	}

	// If selector provided, find element and type into it
	if !step.Selector.IsEmpty() {
		elem, _, err := d.findElement(step.Selector, step.IsOptional())
		if err != nil {
			return errorResult(err, fmt.Sprintf("Element not found: %v", err))
		}
		if err := elem.SendKeys(text); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to input text: %v", err))
		}
	} else {
		// Type into focused element
		active, err := d.client.ActiveElement()
		if err != nil {
			return errorResult(err, "No focused element to type into")
		}
		if err := active.SendKeys(text); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to input text: %v", err))
		}
	}

	return successResult(fmt.Sprintf("Entered text: %s", text), nil)
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

	text := randomString(length)

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
		Message: fmt.Sprintf("Entered random text: %s", text),
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

	uiaDir := mapDirection(direction)
	area := uiautomator2.RectModel{X: 0, Y: 200, Width: 1080, Height: 1600}

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
	uiaDir := mapDirection(direction)
	area := uiautomator2.RectModel{X: 0, Y: 200, Width: 1080, Height: 1600}

	for i := 0; i < maxScrolls; i++ {
		// Try to find element (use short timeout since we're polling)
		elem, info, err := d.findElement(step.Selector, true)
		if err == nil {
			displayed, _ := elem.IsDisplayed()
			if displayed {
				return successResult(fmt.Sprintf("Element found after %d scrolls", i), info)
			}
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
	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "up"
	}

	uiaDir := mapDirection(direction)
	area := uiautomator2.RectModel{X: 0, Y: 200, Width: 1080, Height: 1600}

	if err := d.client.SwipeInArea(area, uiaDir, 0.7, 0); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to swipe: %v", err))
	}

	return successResult(fmt.Sprintf("Swiped %s", direction), nil)
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

	// Launch app using monkey (works without knowing activity name)
	// Alternative: am start -n appID/.MainActivity
	cmd := fmt.Sprintf("monkey -p %s -c android.intent.category.LAUNCHER 1", appID)
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

// ============================================================================
// Clipboard Commands
// ============================================================================

func (d *Driver) copyTextFrom(step *flow.CopyTextFromStep) *core.CommandResult {
	elem, info, err := d.findElement(step.Selector, step.IsOptional())
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	text, err := elem.Text()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get text: %v", err))
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

func randomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
