package uiautomator2

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	elem, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		err = d.notFoundOrCrash(err)
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

	// If duration is set, hold the press for that long (also covers longPress: true via tapOn).
	if step.DurationMs > 0 || step.LongPress {
		duration := step.DurationMs
		if duration <= 0 {
			duration = 1000
		}
		x, y := info.Bounds.Center()
		if err := d.client.LongClick(x, y, duration); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to press for %dms: %v", duration, err))
		}
		return successResult("Pressed on element", info)
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

// tapOnPointWithCoords handles point-based tap with either percentage ("85%, 50%") or absolute ("123, 456") coordinates.
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

func (d *Driver) dragAndDrop(step *flow.DragAndDropStep) *core.CommandResult {
	fromX, fromY, info, err := d.resolveGesturePoint(step.From, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("dragAndDrop: from: %v", err))
	}
	toX, toY, _, err := d.resolveGesturePoint(step.To, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("dragAndDrop: to: %v", err))
	}

	if err := d.client.DragAndDrop(fromX, fromY, toX, toY, step.HoldDuration, step.Duration); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to drag: %v", err))
	}
	return successResult(fmt.Sprintf("Dragged from (%d, %d) to (%d, %d)", fromX, fromY, toX, toY), info)
}

// resolveGesturePoint turns a drag endpoint — a bare point, a selector, or a
// selector plus a point inside it — into screen coordinates. The same
// percentage/absolute rules as tapOn apply: a bare point is relative to the
// screen, a point on a selector is relative to the matched element's bounds,
// and a selector alone resolves to the element's center.
func (d *Driver) resolveGesturePoint(sel flow.Selector, optional bool, stepTimeoutMs int) (int, int, *core.ElementInfo, error) {
	if sel.IsEmpty() {
		width, height, err := d.screenSize()
		if err != nil {
			return 0, 0, nil, err
		}
		x, y, err := core.ParsePointCoords(sel.Point, width, height)
		return x, y, nil, err
	}

	_, info, err := d.findElementForTap(sel, optional, stepTimeoutMs)
	if err != nil {
		return 0, 0, nil, d.notFoundOrCrash(err)
	}
	if info == nil {
		return 0, 0, nil, fmt.Errorf("nil element info")
	}
	x, y, err := core.PointInBounds(sel.Point, info.Bounds)
	return x, y, info, err
}

func (d *Driver) doubleTapOn(step *flow.DoubleTapOnStep) *core.CommandResult {
	wasInput := d.consumeInputFlag()

	if result := d.checkKeyboardBlocking(wasInput, step.Selector); result != nil {
		return result
	}

	elem, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		err = d.notFoundOrCrash(err)
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	// For relative selectors elem is nil but we have bounds. An explicit
	// `point:` also has to go through coordinates, since the element-scoped
	// gesture always lands on the centre.
	if elem == nil || step.Selector.Point != "" {
		x, y, perr := core.PointInBounds(step.Selector.Point, info.Bounds)
		if perr != nil {
			return errorResult(perr, fmt.Sprintf("Invalid point coordinates: %v", perr))
		}
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
	wasInput := d.consumeInputFlag()

	if result := d.checkKeyboardBlocking(wasInput, step.Selector); result != nil {
		return result
	}

	elem, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		err = d.notFoundOrCrash(err)
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	duration := step.DurationMs
	if duration <= 0 {
		duration = 1000 // default 1 second
	}

	// For relative selectors elem is nil but we have bounds; an explicit
	// `point:` likewise needs coordinates rather than the element-scoped press.
	if elem == nil || step.Selector.Point != "" {
		x, y, perr := core.PointInBounds(step.Selector.Point, info.Bounds)
		if perr != nil {
			return errorResult(perr, fmt.Sprintf("Invalid point coordinates: %v", perr))
		}
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

	// Check if using Point field (e.g., "85%, 50%" or "123, 456")
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

	if step.DurationMs > 0 || step.LongPress {
		duration := step.DurationMs
		if duration <= 0 {
			duration = 1000
		}
		if err := d.client.LongClick(x, y, duration); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to press at point for %dms: %v", duration, err))
		}
		return successResult(fmt.Sprintf("Pressed at (%d, %d)", x, y), nil)
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

	// A count assertion needs every match, not the first one — route to the
	// page-source path, which is the only enumerator we have.
	if step.Count != "" {
		return d.assertVisibleCount(step)
	}

	// Use findElementFast - only need to check element exists (1 HTTP call vs 3)
	_, info, err := d.findElementFast(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		err = d.notFoundOrCrash(err)
		return errorResult(err, fmt.Sprintf("Element not visible: %v", err))
	}

	// info.Visible is already set by findElementFast
	if info != nil && info.Visible {
		return successResult("Element is visible", info)
	}

	return errorResult(fmt.Errorf("element not visible"), "Element exists but is not visible")
}

// assertVisibleCount asserts that the selector matches exactly the requested
// number of visible elements. Polls until the count is right or the assert
// timeout runs out, reporting the last observed count on failure.
func (d *Driver) assertVisibleCount(step *flow.AssertVisibleStep) *core.CommandResult {
	want, _, err := step.ExpectedCount()
	if err != nil {
		return errorResult(err, err.Error())
	}
	if step.Selector.HasRelativeSelector() {
		err := fmt.Errorf("count cannot be combined with relative selectors (childOf/below/...)")
		return errorResult(err, err.Error())
	}

	timeout := d.calculateTimeout(step.IsOptional(), step.TimeoutMs)
	ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
	defer cancel()

	desc := step.Selector.Describe()
	observed := -1
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if observed < 0 {
				if lastErr != nil {
					return errorResult(lastErr, fmt.Sprintf("Failed to count matches of '%s': %v", desc, lastErr))
				}
				return errorResult(ctx.Err(), fmt.Sprintf("Expected %d visible matches of '%s' but no observation completed: %v", want, desc, ctx.Err()))
			}
			err := fmt.Errorf("expected %d visible matches of '%s', last observed %d", want, desc, observed)
			return errorResult(err, err.Error())
		default:
			n, err := d.countVisibleMatches(step.Selector)
			if err != nil {
				lastErr = err
				continue // page-source round-trip is the natural rate limit
			}
			observed = n
			if n == want {
				return successResult(fmt.Sprintf("%d elements visible", n), nil)
			}
		}
	}
}

// countVisibleMatches reads the page source once and counts displayed matches,
// applying the same off-screen filter as the single-match page-source path.
func (d *Driver) countVisibleMatches(sel flow.Selector) (int, error) {
	pageSource, err := d.client.Source()
	if err != nil {
		return 0, fmt.Errorf("failed to get page source: %w", err)
	}

	allElements, err := ParsePageSource(pageSource)
	if err != nil {
		return 0, fmt.Errorf("failed to parse page source: %w", err)
	}

	if w, h, err := d.screenSize(); err == nil {
		allElements = FilterOutOfBounds(allElements, w, h)
	}

	return CountDisplayedMatches(allElements, sel), nil
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

	// keyPress mode: simulate real key presses via W3C Actions API.
	// This triggers TextWatcher/onTextChanged per character (unlike setText injection).
	if step.KeyPress {
		// Resolve and read the focused field first: after typing, "unchanged"
		// is the only thing that separates a hint from a lost keystroke.
		target, before := d.focusedFieldBefore()
		if err := d.client.SendKeyActions(text); err != nil {
			return errorResult(err, "Failed to input text via key press")
		}
		// Per-character key events are the path that loses characters when the
		// app janks — the whole reason this verification exists.
		note := core.ConfirmTypedText(target, text, before, logger.Warn)
		return successResult(fmt.Sprintf("Entered text (keyPress): %s%s%s", text, unicodeWarning, note), nil)
	}

	// If selector provided, find element and type into it
	var typedInto core.TextField
	var beforeText string
	if !step.Selector.IsEmpty() {
		elem, _, err := d.findElement(step.Selector, step.IsOptional(), step.TimeoutMs)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Element not found: %v", err))
		}
		before, _ := elem.Text()
		if err := elem.SendKeys(text); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to input text: %v", err))
		}
		typedInto, beforeText = core.TextFieldFuncs(elem.Text, elem.SendKeys, elem.Clear), before
	} else {
		// No selector — prefer element-scoped typing into the focused
		// element (POST /element/{id}/value). Blind key events can silently
		// miss WebView DOM inputs on some devices (#122 — same wire
		// mechanism failing over Appium on cloud farms); element send-keys
		// routes through accessibility ACTION_SET_TEXT, which Chrome
		// translates into a real DOM value change, and the UiAutomator2
		// server appends rather than replaces, preserving type-into-focused
		// semantics. Fall back to key events when nothing has focus.
		//
		// History: acea0c7 removed an ActiveElement path here because it
		// dragged a fragile focused=true selector-search fallback with it.
		// This reintroduction is a single ActiveElement round-trip with a
		// plain key-events fallback — no selector search.
		typed := false
		if active, err := d.client.ActiveElement(); err == nil && active != nil {
			before, _ := active.Text()
			if err := active.SendKeys(text); err == nil {
				typed = true
				typedInto, beforeText = core.TextFieldFuncs(active.Text, active.SendKeys, active.Clear), before
			}
		}
		if !typed {
			if err := d.client.SendKeyActions(text); err != nil {
				return errorResult(err, fmt.Sprintf("Failed to input text: %v", err))
			}
		}
	}

	note := core.ConfirmTypedText(typedInto, text, beforeText, logger.Warn)
	return successResult(fmt.Sprintf("Entered text: %s%s%s", text, unicodeWarning, note), nil)
}

// focusedFieldBefore resolves the element that key events will reach and reads
// it, so the value can be compared once typing is done.
func (d *Driver) focusedFieldBefore() (core.TextField, string) {
	active, err := d.client.ActiveElement()
	if err != nil || active == nil {
		return nil, ""
	}
	before, _ := active.Text()
	return core.TextFieldFuncs(active.Text, active.SendKeys, active.Clear), before
}

func (d *Driver) eraseText(step *flow.EraseTextStep) *core.CommandResult {
	chars := step.Characters
	if chars <= 0 {
		chars = 50 // default
	}

	// Try optimized approach first (Clear or text replacement)
	// This is much faster than pressing delete key N times (3 HTTP calls vs N calls)
	active, err := d.client.ActiveElement()
	if err == nil {
		// Got active element - try to read its text
		currentText, textErr := active.Text()
		if textErr == nil {
			textLen := len([]rune(currentText)) // Use runes for proper Unicode handling

			// Case 1: Erase all text (or more than exists) - just Clear() in one shot
			if chars >= textLen || textLen == 0 {
				if clearErr := active.Clear(); clearErr == nil {
					return successResult(fmt.Sprintf("Cleared %d characters", textLen), nil)
				}
				// Clear failed, fall through to delete key approach
			} else {
				// Case 2: Erase N chars from end - use text replacement
				runes := []rune(currentText)
				remaining := string(runes[:textLen-chars])

				if clearErr := active.Clear(); clearErr == nil {
					if remaining != "" {
						if sendErr := active.SendKeys(remaining); sendErr == nil {
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
		// Text() failed (e.g., password field), fall through to delete key approach
	}
	// ActiveElement() failed, fall through to delete key approach

	// Fallback: Press delete key multiple times
	// This is slower (N HTTP calls) but works in edge cases:
	// - Can't find focused element
	// - Element doesn't support Clear() or Text()
	// - Password fields that don't expose text
	// - Custom input components
	for i := 0; i < chars; i++ {
		if err := d.client.PressKeyCode(uiautomator2.KeyCodeDelete); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to erase text: %v", err))
		}
	}

	return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
}

func (d *Driver) hideKeyboard(step *flow.HideKeyboardStep) *core.CommandResult {
	if !d.isInputShown() {
		return successResult("Keyboard not visible, skipped", nil)
	}

	strategy := strings.ToLower(strings.TrimSpace(step.Strategy))

	// If a specific strategy is requested, use only that one.
	switch strategy {
	case "appium":
		return d.hideKeyboardAppium()
	case "escape", "esc":
		return d.hideKeyboardEscape()
	case "back":
		return d.hideKeyboardBack()
	case "":
		// Try all strategies in order.
	default:
		return errorResult(nil, fmt.Sprintf("Unknown hideKeyboard strategy: %q (valid: appium, escape/esc, back)", step.Strategy))
	}

	// Try all strategies: Appium → ESCAPE → BACK
	if r := d.hideKeyboardAppium(); r.Success {
		return r
	}
	if r := d.hideKeyboardEscape(); r.Success {
		return r
	}
	return d.hideKeyboardBack()
}

func (d *Driver) hideKeyboardAppium() *core.CommandResult {
	_ = d.client.HideKeyboard()
	time.Sleep(500 * time.Millisecond)
	if !d.isInputShown() {
		return successResult("Keyboard hidden via Appium endpoint", nil)
	}
	return errorResult(nil, "Appium endpoint failed toa hide keyboard")
}

func (d *Driver) hideKeyboardEscape() *core.CommandResult {
	if d.device == nil {
		return errorResult(nil, "No device available for KEYCODE_ESCAPE")
	}
	_, _ = d.device.Shell("input keyevent 111")
	time.Sleep(500 * time.Millisecond)
	if !d.isInputShown() {
		return successResult("Keyboard hidden via KEYCODE_ESCAPE", nil)
	}
	return errorResult(nil, "KEYCODE_ESCAPE failed to hide keyboard")
}

func (d *Driver) hideKeyboardBack() *core.CommandResult {
	if d.device == nil {
		return errorResult(nil, "No device available for BACK key")
	}
	// Safe: when keyboard IS visible, BACK dismisses it without navigating
	_, _ = d.device.Shell("input keyevent 4")
	time.Sleep(500 * time.Millisecond)
	if !d.isInputShown() {
		return successResult("Keyboard hidden via BACK key", nil)
	}
	return errorResult(nil, "BACK key failed to hide keyboard")
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

	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}

	if err := d.performScroll(direction, width, height, step.Engine, 0.5); err != nil {
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

	// `from:` confines the gesture to one container. Resolved once, before the
	// loop: re-finding it on every iteration would cost a lookup per scroll,
	// and a container that moves while its own content scrolls is not a case
	// worth paying for.
	var container *core.Bounds
	if !step.From.IsEmpty() {
		_, info, findErr := d.findElement(step.From, false, step.TimeoutMs)
		if findErr != nil || info == nil {
			return errorResult(findErr, fmt.Sprintf("Scroll container not found: %s", step.From.Describe()))
		}
		bounds := info.Bounds
		container = &bounds
	}

	for i := 0; i < maxScrolls && time.Now().Before(deadline); i++ {
		// Try to find element (short timeout - includes page source fallback)
		_, info, err := d.findElement(step.Element, true, 1000)
		if err == nil && info != nil {
			// UIAutomator's view hierarchy can include items in a ScrollView
			// that are off-screen — and a match half-hidden behind a bottom
			// bar is no better, because the tap that follows lands wrong.
			// Stop only when the element meets the flow's visibility
			// requirement (default: fully inside the viewport).
			if core.MeetsVisibility(info.Bounds, width, height, step.VisibilityPercentage) {
				return successResult(fmt.Sprintf("Element found after %d scrolls", i), info)
			}
		} else if err != nil && !isElementNotFoundError(err) {
			// Real infrastructure failure — bail rather than silently looping.
			return errorResult(err, "Failed to find element")
		}

		scrollErr := error(nil)
		if container != nil {
			scrollErr = d.performScrollInRect(direction, *container, step.Engine, 0.3)
		} else {
			scrollErr = d.performScroll(direction, width, height, step.Engine, 0.3)
		}
		if scrollErr != nil {
			return errorResult(scrollErr, fmt.Sprintf("Failed to scroll: %v", scrollErr))
		}

		time.Sleep(300 * time.Millisecond)
	}

	return errorResult(fmt.Errorf("element not found"), fmt.Sprintf("Element not found after %d scrolls", maxScrolls))
}

// scrollDurationMs is the swipe duration (in ms) used for adb input swipe.
const scrollDurationMs = 300

const (
	defaultAnimationTimeoutMs = 15000
	defaultAnimationSleepMs   = 200   // pause between the two comparison screenshots
	screenshotDiffThreshold   = 0.005 // 0.5 % — default pixel-diff threshold
	screenshotRetryIntervalMs = 100   // outer loop retry interval
)

// performScroll dispatches a scroll gesture. Default ("" or "adb") uses adb
// input swipe (matches upstream Maestro and is the most reliable path across
// Android skins, including OneUI where /appium/gestures/scroll often no-ops).
// "agent" uses the existing UIA2-server Appium gesture path. ADB falls back
// to the Appium path (with a warning) when no shell executor is available.
// percent controls the swipe distance as a fraction of screen dimension —
// callers use ~0.5 for plain scroll and ~0.3 for scrollUntilVisible (which
// wants smaller steps to avoid overshooting the target).
func (d *Driver) performScroll(direction string, width, height int, engine string, percent float64) error {
	useAgent := strings.EqualFold(engine, "agent")
	if !useAgent {
		if d.device != nil {
			return d.scrollByAdb(direction, width, height, percent)
		}
		logger.Warn("scroll: ADB shell unavailable, falling back to Appium gesture (may be unreliable on some Android skins)")
	}
	area := uiautomator2.NewRect(0, height/8, width, height*3/4)
	return d.client.ScrollInArea(area, direction, percent, 0)
}

// performScrollInRect scrolls inside one container rather than the screen. The
// inset keeps the gesture off the container's own edges, where a swipe is as
// likely to be read by the parent list as by the container itself.
func (d *Driver) performScrollInRect(direction string, bounds core.Bounds, engine string, percent float64) error {
	inset := bounds.Height / 8
	x, y := bounds.X, bounds.Y+inset
	w, h := bounds.Width, bounds.Height-2*inset
	if h <= 0 {
		x, y, w, h = bounds.X, bounds.Y, bounds.Width, bounds.Height
	}

	useAgent := strings.EqualFold(engine, "agent")
	if !useAgent && d.device != nil {
		return d.scrollByAdbInRect(direction, x, y, w, h, percent)
	}
	return d.client.ScrollInArea(uiautomator2.NewRect(x, y, w, h), direction, percent, 0)
}

// scrollByAdb issues `adb shell input swipe` over the local shell executor.
// percent is the swipe distance as a fraction of the screen dimension along
// the scroll axis. Direction uses Maestro scroll semantics (what becomes
// visible — "down" reveals content below by swiping the finger UP).
func (d *Driver) scrollByAdb(direction string, screenWidth, screenHeight int, percent float64) error {
	return d.scrollByAdbInRect(direction, 0, 0, screenWidth, screenHeight, percent)
}

// scrollByAdbInRect is scrollByAdb over an arbitrary rectangle, so a scroll can
// be confined to one container rather than the whole screen. The gesture is
// centred in the rectangle and spans `percent` of its height or width.
func (d *Driver) scrollByAdbInRect(direction string, rectX, rectY, rectW, rectH int, percent float64) error {
	fromX, fromY, toX, toY := scrollPointsInRect(direction, rectX, rectY, rectW, rectH, percent)
	cmd := fmt.Sprintf("input swipe %d %d %d %d %d", fromX, fromY, toX, toY, scrollDurationMs)
	_, err := d.device.Shell(cmd)
	return err
}

// scrollPointsInRect returns the swipe endpoints for a scroll centred in a
// rectangle and spanning `percent` of it along the scroll axis. Pure, so the
// geometry can be checked without a device.
//
// Direction follows Maestro semantics — it names what becomes visible, so
// "down" reveals content below by dragging the finger up the screen.
func scrollPointsInRect(direction string, rectX, rectY, rectW, rectH int, percent float64) (fromX, fromY, toX, toY int) {
	centerX := rectX + rectW/2
	centerY := rectY + rectH/2
	halfV := int(float64(rectH) * percent / 2)
	halfH := int(float64(rectW) * percent / 2)
	switch direction {
	case "up":
		return centerX, centerY - halfV, centerX, centerY + halfV
	case "left":
		return centerX - halfH, centerY, centerX + halfH, centerY
	case "right":
		return centerX + halfH, centerY, centerX - halfH, centerY
	default: // "down" and anything unrecognised
		return centerX, centerY + halfV, centerX, centerY - halfV
	}
}

// isElementNotFoundError distinguishes expected "not on screen yet" lookups
// (which scrollUntilVisible should swallow and keep scrolling) from real
// infrastructure failures that should propagate immediately.
func isElementNotFoundError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, phrase := range []string{
		"not found",
		"no elements match",
		"no such element",
		"could not be located",
		"context deadline exceeded",
	} {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
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
	direction, err := core.NormalizeSwipeDirection(step.Direction)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid swipe direction: %s", step.Direction))
	}

	// If selector specified, derive swipe coordinates from the element's
	// bounds and route through the same ADB `input swipe` path used by
	// screen-percentage swipes. `SwipeInArea` (the previous path) does not
	// honor `step.Duration`, producing a fast flick — sufficient for scroll
	// containers but too fast for native drag targets (Compose sliders,
	// custom drag-handlers), which discard the gesture. Using the ADB path
	// with an element-derived start/end matches classic Maestro's semantics.
	if step.Selector != nil && !step.Selector.IsEmpty() {
		_, info, err := d.findElement(*step.Selector, step.IsOptional(), step.TimeoutMs)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Element not found for swipe: %v", err))
		}
		if info != nil && info.Bounds.Width > 0 {
			screenW, screenH, _ := d.screenSize() // (0,0) when unknown → far-edge clamp skipped
			startX, startY, endX, endY, err := core.SwipeCoordsForElement(
				direction, info.Bounds, screenW, screenH, step.Distance, step.Selector.Point)
			if err != nil {
				return errorResult(err, fmt.Sprintf("Invalid swipe direction: %s", step.Direction))
			}
			return d.swipeWithAbsoluteCoords(startX, startY, endX, endY, step.Duration)
		}
	}

	// No selector — use screen coordinates directly (matches Maestro behavior)
	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}
	// An explicit `distance:` controls how far the centered swipe travels.
	if step.Distance > 0 {
		sx, sy, ex, ey, derr := core.DirectionSwipeScreenCoords(direction, width, height, step.Distance)
		if derr != nil {
			return errorResult(derr, fmt.Sprintf("Invalid swipe direction: %s", step.Direction))
		}
		return d.swipeWithAbsoluteCoords(sx, sy, ex, ey, step.Duration)
	}
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

		// Valid page source with elements but no scrollables — no point waiting
		if len(elements) > 0 {
			return nil, 0
		}

		time.Sleep(pollInterval)
	}

	return nil, 0
}

// swipeWithMaestroCoordinates performs swipe using centered coordinates.
// Swipe coordinates match Maestro Android behavior:
// UP:    50%,50% → 50%,10%
// DOWN:  50%,20% → 50%,90%
// LEFT:  90%,50% → 10%,50%
// RIGHT: 10%,50% → 90%,50%
func (d *Driver) swipeWithMaestroCoordinates(direction string, width, height, durationMs int) *core.CommandResult {
	var startX, startY, endX, endY int

	switch direction {
	case "up":
		startX = width * 50 / 100
		startY = height * 50 / 100
		endX = width * 50 / 100
		endY = height * 10 / 100
	case "down":
		startX = width * 50 / 100
		startY = height * 20 / 100
		endX = width * 50 / 100
		endY = height * 90 / 100
	case "left":
		startX = width * 90 / 100
		startY = height * 50 / 100
		endX = width * 10 / 100
		endY = height * 50 / 100
	case "right":
		startX = width * 10 / 100
		startY = height * 50 / 100
		endX = width * 90 / 100
		endY = height * 50 / 100
	default:
		startX = width * 50 / 100
		startY = height * 50 / 100
		endX = width * 50 / 100
		endY = height * 10 / 100
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
	width, height, err := d.screenSize()
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

// parsePercentageCoords parses "x%, y%" format into decimal fractions (0.0-1.0)
func parsePercentageCoords(coord string) (float64, float64, error) {
	return core.ParsePercentageCoords(coord)
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

func (d *Driver) openNotifications(_ *flow.OpenNotificationsStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("no shell executor"), "openNotifications requires shell access")
	}
	if _, err := d.device.Shell("cmd statusbar expand-notifications"); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open notification shade: %v", err))
	}
	return successResult("Opened notification shade", nil)
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
	d.currentAppID = appID // remember for mid-flow crash detection

	// Forget deaths recorded before this launch. exit-info persists across
	// runs, so without this a crash from an earlier run — or the force-stop
	// below — would be reported as this flow's. Clearing here means any entry
	// found later belongs to the app this launch started.
	d.clearExitHistory(appID)

	// Stop app first if requested (default: true)
	if step.StopApp == nil || *step.StopApp {
		if _, err := d.device.Shell("am force-stop " + appID); err != nil {
			logger.Warn("failed to force-stop app %s before launch: %v", appID, err)
		}
	}

	// Clear state if requested
	if step.ClearState {
		if _, err := d.device.Shell("pm clear " + appID); err != nil {
			return errorResult(err, fmt.Sprintf("launchApp: failed to clear app state for '%s' — is the app installed?", appID))
		}
	}

	// Apply permissions (default: all allow, like Maestro)
	permissions := step.Permissions
	if len(permissions) == 0 {
		permissions = map[string]string{"all": "allow"}
	}
	_ = d.applyPermissions(appID, permissions)

	// Convert arguments for the server API
	var arguments map[string]interface{}
	if len(step.Arguments) > 0 {
		arguments = step.Arguments
	}

	// Strategy 1: Use UIAutomator2 server endpoint (most reliable)
	// Calls PackageManager.getLaunchIntentForPackage() on-device — works on all OEMs/versions
	if d.client != nil {
		if err := d.client.LaunchApp(appID, arguments); err != nil {
			logger.Warn("launchApp via server failed for %s: %v — falling back to shell commands", appID, err)
		} else {
			time.Sleep(1 * time.Second)
			return successResult(fmt.Sprintf("Launched app: %s", appID), nil)
		}
	}

	// Strategy 2: Shell-based launch (fallback when server endpoint unavailable)
	return d.launchAppViaShell(appID, arguments)
}

// launchAppViaShell launches an app using ADB shell commands.
// Mirrors Appium's approach: detect API level, resolve activity, launch with proper flags.
func (d *Driver) launchAppViaShell(appID string, arguments map[string]interface{}) *core.CommandResult {
	apiLevel := d.getAPILevel()

	// API < 24: monkey for simple launches (resolve-activity doesn't exist)
	if apiLevel < 24 && len(arguments) == 0 {
		return d.launchWithMonkey(appID)
	}

	// Resolve launcher activity
	activity, err := d.resolveLauncherActivity(appID, apiLevel)
	if err != nil {
		// Final fallback: monkey for simple launches on any API level
		if len(arguments) == 0 {
			logger.Warn("launchApp: activity resolution failed for %s: %v — trying monkey", appID, err)
			return d.launchWithMonkey(appID)
		}
		return errorResult(err, fmt.Sprintf(
			"launchApp: cannot find launcher activity for '%s' — %v. "+
				"Is the app installed? Check with: adb shell pm list packages | grep %s", appID, err, appID))
	}

	// Build am start / am start-activity command
	// API >= 26 uses am start-activity, older uses am start (matches Appium)
	amCmd := "am start"
	if apiLevel >= 26 {
		amCmd = "am start-activity"
	}

	cmd := fmt.Sprintf("%s -W -n %s -a android.intent.action.MAIN -c android.intent.category.LAUNCHER -f 0x10200000",
		amCmd, activity)

	// Add intent extras for arguments. String values are quoted because they
	// are free text from the flow — the numeric and boolean cases render from
	// typed Go values and cannot carry shell syntax.
	for key, value := range arguments {
		k := core.ShellQuote(key)
		switch v := value.(type) {
		case string:
			cmd += fmt.Sprintf(" --es %s %s", k, core.ShellQuote(v))
		case int:
			cmd += fmt.Sprintf(" --ei %s %d", k, v)
		case int64:
			cmd += fmt.Sprintf(" --ei %s %d", k, v)
		case float64:
			if v == float64(int(v)) {
				cmd += fmt.Sprintf(" --ei %s %d", k, int(v))
			} else {
				cmd += fmt.Sprintf(" --ef %s %f", k, v)
			}
		case bool:
			cmd += fmt.Sprintf(" --ez %s %t", k, v)
		default:
			cmd += fmt.Sprintf(" --es %s %s", k, core.ShellQuote(fmt.Sprintf("%v", v)))
		}
	}

	output, err := d.device.Shell(cmd)
	if err != nil || strings.Contains(output, "Error") {
		// Retry with dot prefix if activity class not found (Appium does this)
		// e.g., "com.app/MainActivity" → "com.app/.MainActivity"
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

		// Fall back to monkey for no-args case
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
	output, err := d.device.Shell("getprop ro.build.version.sdk")
	if err != nil {
		return 24
	}
	level, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 24
	}
	return level
}

// resolveLauncherActivity resolves the launcher activity for a package.
// Tries: cmd package resolve-activity → dumpsys package parsing.
func (d *Driver) resolveLauncherActivity(appID string, apiLevel int) (string, error) {
	// Strategy 1: cmd package resolve-activity (API >= 24)
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

	// Strategy 2: dumpsys package parsing
	return d.resolveLauncherFromDumpsys(appID)
}

// launchWithMonkey launches an app using the monkey command.
// Universally reliable for simple launches (no arguments) on all Android versions.
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
// Some apps declare activities without the leading dot; am start requires it.
func (d *Driver) addDotPrefix(activity string) string {
	parts := strings.SplitN(activity, "/", 2)
	if len(parts) != 2 {
		return activity
	}
	activityName := parts[1]
	if strings.HasPrefix(activityName, ".") || strings.Contains(activityName, ".") {
		return activity // Already has dot prefix or is fully qualified
	}
	return parts[0] + "/." + activityName
}

// resolveLauncherFromDumpsys parses `dumpsys package` output to find the MAIN/LAUNCHER activity.
// Used as fallback when `cmd package resolve-activity` is unavailable (some OEMs strip it).
func (d *Driver) resolveLauncherFromDumpsys(appID string) (string, error) {
	output, err := d.device.Shell(fmt.Sprintf("dumpsys package %s", appID))
	if err != nil {
		return "", fmt.Errorf("dumpsys failed for %s: %w", appID, err)
	}

	// Look for MAIN/LAUNCHER activity in intent filter blocks.
	// The format has activity lines like "com.example.app/.MainActivity filter abc123"
	// followed by Action/Category lines within the filter block.
	lines := strings.Split(output, "\n")
	inFilter := false
	hasMain := false
	hasLauncher := false
	var currentActivity string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Match lines like "com.example.app/.MainActivity filter abc123"
		if strings.HasPrefix(trimmed, appID) && strings.Contains(trimmed, "/") && strings.Contains(trimmed, "filter") {
			// Check previous block before resetting
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

	// Check final block
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
// Permissions map: shortcut/permission name -> "allow"/"deny"/"unset"
func (d *Driver) applyPermissions(appID string, permissions map[string]string) *core.CommandResult {
	var granted, revoked, errors []string

	for name, value := range permissions {
		// Handle "all" shortcut - applies to all common permissions
		if strings.ToLower(name) == "all" {
			allPerms := getAllPermissions()
			for _, perm := range allPerms {
				err := d.applyPermission(appID, perm, value)
				if err != nil && core.IsUndeclaredPermissionError(err.Error()) {
					// "all" always names permissions a given app never wanted.
					// One line each would bury the run; the explicit branch
					// below is where a flow author asked for something specific
					// and deserves to hear it did not apply.
					logger.Debug("permissions: %s does not declare %s", appID, perm)
				} else if err != nil {
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
			if err != nil && core.IsUndeclaredPermissionError(err.Error()) {
				logger.Warn("setPermissions: %s does not declare %s — skipping", appID, perm)
			} else if err != nil {
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
// resolvePermissionShortcut maps a flow's permission name to Android permission
// strings. The table is shared with the other drivers in pkg/core — it used to
// be duplicated per driver, and the copies drifted (#148).
func resolvePermissionShortcut(shortcut string) []string {
	return core.AndroidPermissionShortcut(shortcut)
}

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
	quoted := core.ShellQuote(link)
	var cmd string
	if step.Browser != nil && *step.Browser {
		// Force open in browser - try common browser packages
		// Chrome is most common, fallback to default browser activity
		cmd = fmt.Sprintf("am start -a android.intent.action.VIEW -c android.intent.category.BROWSABLE -d %s", quoted)
	} else {
		// Default: let system decide (may open in app if deep link is registered)
		cmd = fmt.Sprintf("am start -a android.intent.action.VIEW -d %s", quoted)
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
	data, err := d.Screenshot()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to take screenshot: %v", err))
	}

	if step.CropOn != nil {
		_, info, findErr := d.findElement(*step.CropOn, false, 0)
		if findErr != nil || info == nil {
			return errorResult(findErr, fmt.Sprintf("cropOn: element not found: %v", findErr))
		}
		sw, sh, dimErr := d.screenSize()
		if dimErr != nil {
			return errorResult(dimErr, "cropOn requires screen dimensions")
		}
		cropped, cropErr := core.CropScreenshot(data, info.Bounds, sw, sh)
		if cropErr != nil {
			return errorResult(cropErr, fmt.Sprintf("cropOn: %v", cropErr))
		}
		data = cropped
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
	cmd := fmt.Sprintf("am start -a android.intent.action.VIEW -d %s", core.ShellQuote(url))
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open browser: %v", err))
	}

	return successResult(fmt.Sprintf("Opened browser: %s", url), nil)
}

func (d *Driver) addMedia(step *flow.AddMediaStep) *core.CommandResult {
	if err := core.ValidateMediaFiles(step.Files); err != nil {
		return errorResult(err, err.Error())
	}
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "addMedia requires device access")
	}
	pusher, ok := d.device.(interface {
		Push(local, remote string) error
	})
	if !ok {
		return errorResult(fmt.Errorf("device does not support file push"), "addMedia requires adb push support")
	}

	for _, file := range step.Files {
		if _, err := os.Stat(file); err != nil {
			return errorResult(err, fmt.Sprintf("Media file not found: %s", file))
		}
		destDir := "/sdcard/Pictures/MaestroRunner"
		if core.IsVideoMedia(file) {
			destDir = "/sdcard/Movies/MaestroRunner"
		}
		if _, err := d.device.Shell("mkdir -p " + core.ShellQuote(destDir)); err != nil {
			return errorResult(err, "Failed to create media directory")
		}
		remote := destDir + "/" + filepath.Base(file)
		if err := pusher.Push(file, remote); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to push media %s: %v", filepath.Base(file), err))
		}
		// Register the pushed file with MediaStore so the picker/gallery sees
		// it. The MEDIA_SCANNER_SCAN_FILE broadcast is deprecated and unreliable
		// on API 29+, so scan the file directly via `content`; fall back to the
		// broadcast on older devices where the method is unavailable.
		scan := fmt.Sprintf("content call --uri content://media --method scan_file --arg %s", core.ShellQuote(remote))
		if _, err := d.device.Shell(scan); err != nil {
			_, _ = d.device.Shell(fmt.Sprintf("am broadcast -a android.intent.action.MEDIA_SCANNER_SCAN_FILE -d %s",
				core.ShellQuote("file://"+remote)))
		}
	}

	return successResult(fmt.Sprintf("Added %d media file(s)", len(step.Files)), nil)
}

func (d *Driver) removeMedia(_ *flow.RemoveMediaStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "removeMedia requires device access")
	}

	// Clear the MediaStore index. The package name differs by Android version —
	// try the modular provider first, then the legacy one. We swallow individual
	// errors and only fail when both attempts fail, because devices have one or
	// the other depending on version.
	var lastErr error
	cleared := false
	for _, pkg := range []string{
		"com.google.android.providers.media.module",
		"com.android.providers.media",
	} {
		if _, err := d.device.Shell("pm clear " + pkg); err == nil {
			cleared = true
		} else {
			lastErr = err
		}
	}
	if !cleared {
		return errorResult(lastErr, fmt.Sprintf("Failed to clear media providers: %v", lastErr))
	}

	return successResult("Cleared MediaStore index", nil)
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
	cmd := fmt.Sprintf("screenrecord %s </dev/null >/dev/null 2>&1 &", path)
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

	// Kill screenrecord process (may have already stopped)
	if _, err := d.device.Shell("pkill -INT screenrecord"); err != nil {
		logger.Warn("failed to stop screenrecord process: %v", err)
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

func (d *Driver) waitForAnimationToEnd(step *flow.WaitForAnimationToEndStep) *core.CommandResult {
	timeoutMs := step.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultAnimationTimeoutMs
	}
	sleepMs := step.SleepMs
	if sleepMs <= 0 {
		sleepMs = defaultAnimationSleepMs
	}
	threshold := step.Threshold
	if threshold <= 0 {
		threshold = screenshotDiffThreshold
	}

	res := core.WaitForScreenStatic(
		func() ([]byte, error) { return d.client.Screenshot() },
		time.Duration(timeoutMs)*time.Millisecond,
		time.Duration(sleepMs)*time.Millisecond,
		time.Duration(screenshotRetryIntervalMs)*time.Millisecond,
		threshold,
	)

	if res.Settled {
		return successResult(
			fmt.Sprintf("Animation ended (%.1f%% diff, %dms)", res.Diffs[len(res.Diffs)-1]*100, res.Elapsed.Milliseconds()),
			nil,
		)
	}
	return &core.CommandResult{
		Success: false,
		Message: fmt.Sprintf(
			"Timed out after %dms (%d iteration(s)) waiting for screen to become static; diffs=%s threshold=%.4f",
			timeoutMs, res.Iterations, formatAnimationDiffs(res.Diffs), threshold,
		),
	}
}

// formatAnimationDiffs formats a slice of diff values as "[0.000764 0.000821 ...]"
func formatAnimationDiffs(diffs []float64) string {
	if len(diffs) == 0 {
		return "[]"
	}
	parts := make([]string, len(diffs))
	for i, d := range diffs {
		parts[i] = fmt.Sprintf("%.6f", d)
	}
	return "[" + strings.Join(parts, " ") + "]"
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

// ============================================================================
// Dark mode (Maestro #2507)
// ============================================================================

func (d *Driver) setDarkMode(step *flow.SetDarkModeStep) *core.CommandResult {
	return d.applyDarkMode(step.Enabled)
}

func (d *Driver) toggleDarkMode(_ *flow.ToggleDarkModeStep) *core.CommandResult {
	current, err := d.currentDarkMode()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to read dark mode: %v", err))
	}
	return d.applyDarkMode(!current)
}

func (d *Driver) assertDarkMode(_ *flow.AssertDarkModeStep) *core.CommandResult {
	return d.assertDarkModeIs(true)
}

func (d *Driver) assertLightMode(_ *flow.AssertLightModeStep) *core.CommandResult {
	return d.assertDarkModeIs(false)
}

func (d *Driver) assertDarkModeIs(want bool) *core.CommandResult {
	got, err := d.currentDarkMode()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to read dark mode: %v", err))
	}
	if got != want {
		assertErr := core.DarkModeAssertionError(want, got)
		return errorResult(assertErr, assertErr.Error())
	}
	return successResult(fmt.Sprintf("Device is in %s mode", core.DarkModeStateName(want)), nil)
}

// currentDarkMode reports whether the system UI is currently dark.
func (d *Driver) currentDarkMode() (bool, error) {
	if d.device == nil {
		return false, fmt.Errorf("device not configured")
	}
	output, err := d.device.Shell(core.AndroidDarkModeQuery)
	if err != nil {
		return false, err
	}
	return core.ParseAndroidNightMode(output)
}

func (d *Driver) applyDarkMode(enabled bool) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "setDarkMode requires device access")
	}
	if _, err := d.device.Shell(core.AndroidDarkModeCommand(enabled)); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set dark mode: %v", err))
	}
	return successResult(fmt.Sprintf("Set %s mode", core.DarkModeStateName(enabled)), nil)
}
