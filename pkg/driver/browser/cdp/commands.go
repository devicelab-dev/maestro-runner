package cdp

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// tapOn taps on an element. Rod's Click() handles scroll+stable+interactable+enabled.
func (d *Driver) tapOn(step *flow.TapOnStep) *core.CommandResult {
	elem, info, err := d.findElement(step.Selector, isOptional(step.Selector.Optional), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to find element %s", step.Selector.DescribeQuoted()))
	}

	// Handle <option> elements: select the option via its parent <select> instead of clicking
	tag, _ := elem.Eval(`() => this.tagName.toLowerCase()`)
	if tag != nil && tag.Value.Str() == "option" {
		_, err := elem.Eval(`() => {
			this.selected = true;
			var select = this.closest('select');
			if (select) {
				select.value = this.value;
				select.dispatchEvent(new Event('change', {bubbles: true}));
			}
		}`)
		if err != nil {
			return errorResult(err, "Failed to select option")
		}
		return successResult(fmt.Sprintf("Selected option %s", step.Selector.DescribeQuoted()), info)
	}

	// Iframe / shadow-root branch: when the element lives inside an iframe,
	// Rod's Click() uses iframe-LOCAL coordinates while CDP Input.dispatchMouseEvent
	// operates in TOP-FRAME viewport coordinates — clicks land at the wrong place
	// and report success silently. Route these through the coord-translated
	// dispatch path which also runs Playwright-style hit-target verification.
	// Top-frame elements (including those inside top-frame shadow roots) keep
	// the existing path — getBoundingClientRect() is already in top-frame
	// coords for them, so Rod's Click() is correct. (Issues #71/#72 acting layer.)
	inIframe, _ := elem.Eval(`() => window.__maestro._isInIframe(this)`)
	if inIframe != nil && inIframe.Value.Bool() {
		return d.tapOnCrossRoot(elem, info, step)
	}

	if err := elem.Click(proto.InputMouseButtonLeft, 1); err != nil {
		// Fallback: use JS click (handles elements that can't receive CDP input events)
		if _, jsErr := elem.Eval(`() => this.click()`); jsErr != nil {
			return errorResult(err, "Failed to tap on element")
		}
	}

	return successResult(fmt.Sprintf("Tapped on %s", step.Selector.DescribeQuoted()), info)
}

// tapOnCrossRoot dispatches a tap against an element nested inside an iframe
// (or iframe + open shadow root). Translates the element's coordinate from
// iframe-local to top-frame viewport coordinates, runs Playwright-style
// hit-target verification before and after dispatch, and uses page-level
// Mouse to send a trusted Input.dispatchMouseEvent at the translated point.
//
// Ports microsoft/playwright `_checkFrameIsHitTarget` (server/dom.ts) and
// `setupHitTargetInterceptor` (injected/injectedScript.ts) into the maestro
// CDP driver. The coord-translation walk is in jshelper.js
// (`topFrameClickPoint`); pre-flight + interceptor lives in
// `setupHitTargetInterceptor`. (Issues #71/#72 acting layer.)
func (d *Driver) tapOnCrossRoot(elem *rod.Element, info *core.ElementInfo, step *flow.TapOnStep) *core.CommandResult {
	desc := step.Selector.DescribeQuoted()

	// Step 1: compute top-frame click point. The JS walks up the frame chain,
	// adding each iframe's box.left/top + content-area inset (border+padding)
	// at every level, ending with a coord in the top-frame viewport.
	// Throws on transformed-iframe ancestors (Playwright bails on transforms
	// rather than computing through DOMMatrix; we follow that decision).
	ptRes, err := elem.Eval(`() => {
		var p = window.__maestro.topFrameClickPoint(this);
		return [p.x, p.y];
	}`)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Cross-root click setup failed for %s: %v", desc, err))
	}
	arr := ptRes.Value.Arr()
	if len(arr) != 2 {
		return errorResult(
			fmt.Errorf("topFrameClickPoint returned %d values", len(arr)),
			fmt.Sprintf("Failed to compute cross-root click point for %s", desc))
	}
	x := arr[0].Num()
	y := arr[1].Num()

	// Step 2: pre-flight expectHitTarget + install trusted-event interceptor.
	// Pre-flight runs elementsFromPoint per shadow root and walks slot/host
	// chain to verify the click point would land on our target. If something
	// occludes (modal overlay, hidden iframe, etc.), we get back an
	// {error: hitTargetDescription} and bail before dispatch.
	setupRes, err := elem.Eval(
		`(x, y) => window.__maestro.setupHitTargetInterceptor(this, {x: x, y: y})`,
		x, y)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set up hit-target interceptor for %s", desc))
	}
	if setupRes.Value.Has("error") {
		errMsg := setupRes.Value.Get("error").Str()
		return errorResult(
			fmt.Errorf("hit-target pre-flight: %s blocks %s", errMsg, desc),
			fmt.Sprintf("Click on %s blocked by overlay (%s)", desc, errMsg))
	}
	if !setupRes.Value.Has("token") {
		return errorResult(
			fmt.Errorf("interceptor returned no token"),
			fmt.Sprintf("Failed to set up hit-target interceptor for %s", desc))
	}
	token := setupRes.Value.Get("token").Int()

	// Defensive cleanup. pollHitTargetResult also frees the token on capture,
	// but if we error out before polling, this prevents a leaked listener.
	defer func() {
		_, _ = d.page.Eval(`(t) => window.__maestro.disposeHitTargetInterceptor(t)`, token)
	}()

	// Step 3: dispatch via top-frame mouse. Produces a trusted
	// pointerdown/mousedown/click sequence; the JS interceptor captures the
	// actual clientX/Y at event-fire time and runs expectHitTarget against
	// that point. We do NOT use elem.Click() here — that's the buggy path
	// for cross-root elements.
	mouse := d.page.Mouse
	if err := mouse.MoveTo(proto.NewPoint(x, y)); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to move mouse for %s", desc))
	}
	if err := mouse.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to dispatch click for %s", desc))
	}

	// Step 4: poll for verify result. Chromium delivers trusted events
	// synchronously during Mouse.Click, so the first poll is usually
	// decisive. Brief retry window absorbs scheduler jitter on slower
	// machines / CI.
	for i := 0; i < 5; i++ {
		pollRes, pollErr := d.page.Eval(`(t) => window.__maestro.pollHitTargetResult(t)`, token)
		if pollErr != nil {
			return errorResult(pollErr, fmt.Sprintf("Failed to poll hit-target result for %s", desc))
		}
		v := pollRes.Value
		if v.Has("hitTargetDescription") {
			hd := v.Get("hitTargetDescription").Str()
			return errorResult(
				fmt.Errorf("click did not reach %s — landed on %s", desc, hd),
				fmt.Sprintf("Click on %s did not reach the target (landed on %s)", desc, hd))
		}
		switch v.Str() {
		case "pending":
			time.Sleep(20 * time.Millisecond)
			continue
		case "done":
			return successResult(fmt.Sprintf("Tapped on %s", desc), info)
		default:
			// Unrecognized payload — be permissive (don't manufacture
			// failures from unknown shapes), but the success path still
			// returns the standard ok result.
			return successResult(fmt.Sprintf("Tapped on %s", desc), info)
		}
	}
	return errorResult(
		fmt.Errorf("hit-target verify did not capture trusted event within timeout"),
		fmt.Sprintf("Click on %s dispatched but verification timed out", desc))
}

// doubleTapOn double-clicks an element.
func (d *Driver) doubleTapOn(step *flow.DoubleTapOnStep) *core.CommandResult {
	elem, info, err := d.findElement(step.Selector, isOptional(step.Selector.Optional), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to find element %s", step.Selector.DescribeQuoted()))
	}

	if err := elem.Click(proto.InputMouseButtonLeft, 2); err != nil {
		return errorResult(err, "Failed to double tap on element")
	}

	return successResult(fmt.Sprintf("Double tapped on %s", step.Selector.DescribeQuoted()), info)
}

// longPressOn performs a long press (mouse down, hold 1s, mouse up).
func (d *Driver) longPressOn(step *flow.LongPressOnStep) *core.CommandResult {
	elem, info, err := d.findElement(step.Selector, isOptional(step.Selector.Optional), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to find element %s", step.Selector.DescribeQuoted()))
	}

	// Scroll into view and wait for interactable
	pt, err := elem.WaitInteractable()
	if err != nil {
		return errorResult(err, "Element not interactable for long press")
	}

	mouse := d.page.Mouse
	if err := mouse.MoveTo(*pt); err != nil {
		return errorResult(err, "Failed to move mouse")
	}
	if err := mouse.Down(proto.InputMouseButtonLeft, 1); err != nil {
		return errorResult(err, "Failed to mouse down")
	}
	time.Sleep(1 * time.Second)
	if err := mouse.Up(proto.InputMouseButtonLeft, 1); err != nil {
		return errorResult(err, "Failed to mouse up")
	}

	return successResult(fmt.Sprintf("Long pressed on %s", step.Selector.DescribeQuoted()), info)
}

// tapOnPoint taps at specific coordinates.
func (d *Driver) tapOnPoint(step *flow.TapOnPointStep) *core.CommandResult {
	x, y := step.X, step.Y

	// Handle percentage-based point
	if step.Point != "" {
		px, py, err := parsePercentageCoords(step.Point)
		if err != nil {
			return errorResult(err, "Failed to parse point coordinates")
		}
		x = int(px * float64(d.viewportW))
		y = int(py * float64(d.viewportH))
	}

	mouse := d.page.Mouse
	pt := proto.NewPoint(float64(x), float64(y))
	if err := mouse.MoveTo(pt); err != nil {
		return errorResult(err, "Failed to move mouse")
	}
	if err := mouse.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return errorResult(err, "Failed to click at point")
	}

	return successResult(fmt.Sprintf("Tapped at (%d, %d)", x, y), nil)
}

// assertVisible asserts that an element is visible.
// Uses the JS helper's visibility check for plain selectors (RAF-based polling
// in the browser, faster than CDP round-trips). Falls back to the Go-side
// finder for selectors the JS fast path can't handle correctly:
//   - state filters (Enabled / Checked / Focused / Selected) — the Go finder
//     applies these via matchesStateFilters; the JS waitForVisible path does not.
//   - Nth > 0 — the Go finder respects index/out-of-range; the JS path just
//     checks "any visible".
//   - Role selectors — implicit ARIA roles (e.g. <a> as "link") need the CDP
//     accessibility tree, which only the Go path uses.
func (d *Driver) assertVisible(step *flow.AssertVisibleStep) *core.CommandResult {
	timeoutMs := step.TimeoutMs
	if timeoutMs == 0 {
		timeoutMs = 5000
	}
	desc := step.Selector.DescribeQuoted()

	// Track unsupported-field warnings even when we take the JS fast path
	// (findElement records them but the fast path bypasses findElement).
	d.recordUnsupportedFields(&step.Selector)

	selectorType, selectorValue := jsSelectorTypeValue(step.Selector)
	if selectorType != "" && !needsGoFinder(step.Selector) {
		// Use RAF-based JS polling: consistent with waitUntil visibility checks
		result, err := d.page.Timeout(time.Duration(timeoutMs+1000) * time.Millisecond).Evaluate(
			rod.Eval(`(type, value, timeout) => window.__maestro.waitForVisible(type, value, timeout)`,
				selectorType, selectorValue, timeoutMs).ByPromise(),
		)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Element %s is not visible", desc))
		}
		if result.Value.Bool() {
			return successResult(fmt.Sprintf("Element %s is visible", desc), nil)
		}
		return errorResult(
			fmt.Errorf("element is not visible"),
			fmt.Sprintf("Element %s is not visible", desc),
		)
	}

	// Fallback: Rod-based element find with visibility check
	_, info, err := d.findElement(step.Selector, isOptional(step.Selector.Optional), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element %s is not visible", desc))
	}
	if !info.Visible {
		return errorResult(
			fmt.Errorf("element exists but is not visible"),
			fmt.Sprintf("Element %s exists but is not visible", desc),
		)
	}
	return successResult(fmt.Sprintf("Element %s is visible", desc), info)
}

// needsGoFinder reports whether a selector has features the JS fast-path
// (waitForVisible) does not implement, so the caller must route through the
// Go-side findElement instead. Keep this in sync with the Go finder's
// capabilities.
func needsGoFinder(sel flow.Selector) bool {
	if sel.Enabled != nil || sel.Checked != nil || sel.Focused != nil || sel.Selected != nil {
		return true
	}
	if sel.EffectiveNth() > 0 {
		return true
	}
	if sel.Role != "" {
		return true
	}
	return false
}

// recordUnsupportedFields registers any web-unsupported selector fields in
// d.warnedFields so consumers (and tests) can detect them, and logs each new
// field once. Callers that bypass findElement still need to call this.
func (d *Driver) recordUnsupportedFields(sel *flow.Selector) {
	unsupported := flow.CheckUnsupportedFields(sel, "web")
	for _, field := range unsupported {
		if !d.warnedFields[field] {
			d.warnedFields[field] = true
			log.Printf("[browser] warning: %q is not supported on web — will be ignored", field)
		}
	}
}

// assertNotVisible asserts that an element is NOT visible.
// Uses RAF-based polling in the browser for fast resolution (~16ms) instead of
// CDP round-trips with 200ms polling.
func (d *Driver) assertNotVisible(step *flow.AssertNotVisibleStep) *core.CommandResult {
	timeoutMs := step.TimeoutMs
	if timeoutMs == 0 {
		timeoutMs = 5000
	}

	selectorType, selectorValue := jsSelectorTypeValue(step.Selector)
	desc := step.Selector.DescribeQuoted()

	// Use the JS RAF-based polling: runs inside the browser, no CDP round-trips.
	// Resolves within ~16ms of element disappearing.
	// ByPromise() tells Rod to await the JS Promise before returning.
	result, err := d.page.Timeout(time.Duration(timeoutMs+1000) * time.Millisecond).Evaluate(
		rod.Eval(`(type, value, timeout) => window.__maestro.waitForNotVisible(type, value, timeout)`,
			selectorType, selectorValue, timeoutMs).ByPromise(),
	)
	if err != nil {
		// JS evaluation failed (e.g. page navigated) — element is gone
		return successResult(fmt.Sprintf("Element %s is not visible", desc), nil)
	}

	if result.Value.Bool() {
		return successResult(fmt.Sprintf("Element %s is not visible", desc), nil)
	}

	return errorResult(
		fmt.Errorf("element is still visible after %dms", timeoutMs),
		fmt.Sprintf("Element %s is still visible", desc),
	)
}

// jsSelectorTypeValue extracts the selector type and value for use with the
// browser-side __maestro JS helper functions.
func jsSelectorTypeValue(sel flow.Selector) (string, string) {
	switch {
	case sel.CSS != "":
		return "css", sel.CSS
	case sel.TestID != "":
		return "testId", sel.TestID
	case sel.Name != "":
		return "name", sel.Name
	case sel.Placeholder != "":
		return "placeholder", sel.Placeholder
	case sel.Href != "":
		return "href", sel.Href
	case sel.Alt != "":
		return "alt", sel.Alt
	case sel.Title != "":
		return "title", sel.Title
	case sel.Role != "":
		return "role", sel.Role
	case sel.ID != "":
		return "id", sel.ID
	case sel.TextRegex != "":
		return "textRegex", sel.TextRegex
	case sel.TextContains != "":
		return "textContains", sel.TextContains
	case sel.Text != "":
		if looksLikeRegex(sel.Text) {
			return "textRegex", sel.Text
		}
		return "text", sel.Text
	default:
		return "text", ""
	}
}

// inputText types text into an element. Rod's Input() handles focus+waitEnabled+waitWritable+events.
func (d *Driver) inputText(step *flow.InputTextStep) *core.CommandResult {
	if !step.Selector.IsEmpty() {
		elem, _, err := d.findElement(step.Selector, isOptional(step.Selector.Optional), step.TimeoutMs)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to find element %s", step.Selector.DescribeQuoted()))
		}
		if err := elem.Input(step.Text); err != nil {
			return errorResult(err, "Failed to input text")
		}
	} else {
		// Type into the currently focused element via keyboard
		if err := d.page.Keyboard.Type([]input.Key(convertToKeys(step.Text))...); err != nil {
			// Fallback: use InsertText for non-typeable characters
			if err := d.page.InsertText(step.Text); err != nil {
				return errorResult(err, "Failed to input text")
			}
		}
	}

	return successResult(fmt.Sprintf("Entered text: %s", step.Text), nil)
}

// eraseText erases characters. Sends Ctrl+A then Backspace, or N backspaces.
func (d *Driver) eraseText(step *flow.EraseTextStep) *core.CommandResult {
	chars := step.Characters
	if chars == 0 {
		chars = 50
	}

	kb := d.page.Keyboard
	for i := 0; i < chars; i++ {
		if err := kb.Type(input.Backspace); err != nil {
			return errorResult(err, "Failed to erase text")
		}
	}

	return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
}

// hideKeyboard is a no-op on web (no virtual keyboard).
func (d *Driver) hideKeyboard(step *flow.HideKeyboardStep) *core.CommandResult {
	return successResult("hideKeyboard is a no-op on web platform", nil)
}

// inputRandom generates and inputs random text.
func (d *Driver) inputRandom(step *flow.InputRandomStep) *core.CommandResult {
	length := step.Length
	if length == 0 {
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
	default: // TEXT
		text = randomString(length)
	}

	if err := d.page.InsertText(text); err != nil {
		return errorResult(err, "Failed to input random text")
	}

	result := successResult(fmt.Sprintf("Entered random text: %s", text), nil)
	result.Data = text
	return result
}

// viewportCenter returns the center point of the viewport.
func (d *Driver) viewportCenter() proto.Point {
	return proto.NewPoint(float64(d.viewportW)/2, float64(d.viewportH)/2)
}

// scroll scrolls the page in the given direction.
func (d *Driver) scroll(step *flow.ScrollStep) *core.CommandResult {
	dir := strings.ToLower(step.Direction)
	if dir == "" {
		dir = "down"
	}

	var dx, dy float64
	switch dir {
	case "down":
		dy = 300
	case "up":
		dy = -300
	case "left":
		dx = -300
	case "right":
		dx = 300
	}

	mouse := d.page.Mouse
	if err := mouse.MoveTo(d.viewportCenter()); err != nil {
		return errorResult(err, "Failed to move mouse for scroll")
	}
	if err := mouse.Scroll(dx, dy, 0); err != nil {
		return errorResult(err, "Failed to scroll")
	}

	return successResult(fmt.Sprintf("Scrolled %s", dir), nil)
}

// scrollUntilVisible scrolls until an element is visible.
func (d *Driver) scrollUntilVisible(step *flow.ScrollUntilVisibleStep) *core.CommandResult {
	dir := strings.ToLower(step.Direction)
	if dir == "" {
		dir = "down"
	}
	maxScrolls := 10

	var dy float64
	switch dir {
	case "down":
		dy = 300
	case "up":
		dy = -300
	}

	center := d.viewportCenter()
	for i := 0; i < maxScrolls; i++ {
		_, info, err := d.findElementOnce(step.Element)
		if err == nil && info != nil && info.Visible {
			return successResult(
				fmt.Sprintf("Element visible after %d scrolls", i),
				info,
			)
		}

		mouse := d.page.Mouse
		if err := mouse.MoveTo(center); err != nil {
			log.Printf("[browser] scrollUntilVisible: MoveTo failed: %v", err)
		}
		if err := mouse.Scroll(0, dy, 0); err != nil {
			return errorResult(err, "Failed to scroll")
		}
		time.Sleep(300 * time.Millisecond)
	}

	return errorResult(
		fmt.Errorf("element not visible after %d scrolls", maxScrolls),
		fmt.Sprintf("Element %s not visible after scrolling", step.Element.DescribeQuoted()),
	)
}

// swipe performs a swipe gesture using mouse drag.
func (d *Driver) swipe(step *flow.SwipeStep) *core.CommandResult {
	dir := strings.ToLower(step.Direction)
	center := d.viewportCenter()
	cx, cy := center.X, center.Y

	var startX, startY, endX, endY float64
	switch dir {
	case "up":
		startX, startY = cx, cy*1.4
		endX, endY = cx, cy*0.6
	case "down":
		startX, startY = cx, cy*0.6
		endX, endY = cx, cy*1.4
	case "left":
		startX, startY = cx*1.4, cy
		endX, endY = cx*0.6, cy
	case "right":
		startX, startY = cx*0.6, cy
		endX, endY = cx*1.4, cy
	default:
		return errorResult(fmt.Errorf("unsupported swipe direction: %s", dir), "Invalid swipe direction")
	}

	mouse := d.page.Mouse
	startPt := proto.NewPoint(startX, startY)
	if err := mouse.MoveTo(startPt); err != nil {
		return errorResult(err, "Failed to move mouse for swipe")
	}
	if err := mouse.Down(proto.InputMouseButtonLeft, 1); err != nil {
		return errorResult(err, "Failed to mouse down for swipe")
	}
	endPt := proto.NewPoint(endX, endY)
	if err := mouse.MoveTo(endPt); err != nil {
		return errorResult(err, "Failed to drag for swipe")
	}
	if err := mouse.Up(proto.InputMouseButtonLeft, 1); err != nil {
		return errorResult(err, "Failed to mouse up for swipe")
	}

	return successResult(fmt.Sprintf("Swiped %s", dir), nil)
}

// waitForPageReady waits for the page to finish loading and DOM to stabilize.
// Used after navigations to handle SPAs that render content after the load event.
func (d *Driver) waitForPageReady() {
	_ = d.page.WaitLoad()
	p := d.page.Timeout(5 * time.Second)
	_ = p.WaitDOMStable(300*time.Millisecond, 0)
	if d.network != nil {
		d.network.waitForIdle(5*time.Second, 500*time.Millisecond)
	}
}

// back navigates back in browser history.
func (d *Driver) back(step *flow.BackStep) *core.CommandResult {
	if err := d.page.NavigateBack(); err != nil {
		return errorResult(err, "Failed to navigate back")
	}
	d.waitForPageReady()
	return successResult("Navigated back", nil)
}

// pressKey presses a keyboard key.
func (d *Driver) pressKey(step *flow.PressKeyStep) *core.CommandResult {
	key := mapKey(step.Key)
	if key == 0 {
		return errorResult(fmt.Errorf("unknown key: %s", step.Key), "Unknown key")
	}

	if err := d.page.Keyboard.Type(key); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to press key: %s", step.Key))
	}

	return successResult(fmt.Sprintf("Pressed key: %s", step.Key), nil)
}

// launchApp navigates to the app URL.
func (d *Driver) launchApp(step *flow.LaunchAppStep) *core.CommandResult {
	url := step.AppID
	if url == "" {
		return errorResult(fmt.Errorf("no URL specified"), "No URL specified for launchApp")
	}

	if step.ClearState {
		d.clearAllState()
	}

	if err := d.page.Navigate(url); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to navigate to %s", url))
	}
	d.waitForPageReady()

	return successResult(fmt.Sprintf("Navigated to %s", url), nil)
}

// stopApp navigates to about:blank.
func (d *Driver) stopApp(step *flow.StopAppStep) *core.CommandResult {
	return d.navigateBlank()
}

// killApp navigates to about:blank.
func (d *Driver) killApp(step *flow.KillAppStep) *core.CommandResult {
	return d.navigateBlank()
}

// navigateBlank navigates to about:blank (shared by stopApp/killApp).
func (d *Driver) navigateBlank() *core.CommandResult {
	if err := d.page.Navigate("about:blank"); err != nil {
		return errorResult(err, "Failed to navigate to about:blank")
	}
	return successResult("Navigated to about:blank", nil)
}

// clearState clears cookies, storage, and cache.
func (d *Driver) clearState(step *flow.ClearStateStep) *core.CommandResult {
	d.clearAllState()
	return successResult("Cleared browser state", nil)
}

// clearAllState clears cookies, local/session storage, and cache.
func (d *Driver) clearAllState() {
	if err := d.page.SetCookies(nil); err != nil {
		log.Printf("[browser] clearState: failed to clear cookies: %v", err)
	}

	d.page.MustEval(`() => {
		try { localStorage.clear(); } catch(e) {}
		try { sessionStorage.clear(); } catch(e) {}
	}`)

	if err := (proto.NetworkClearBrowserCache{}).Call(d.page); err != nil {
		log.Printf("[browser] clearState: failed to clear cache: %v", err)
	}
}

// copyTextFrom copies text from an element.
func (d *Driver) copyTextFrom(step *flow.CopyTextFromStep) *core.CommandResult {
	elem, info, err := d.findElement(step.Selector, isOptional(step.Selector.Optional), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to find element %s", step.Selector.DescribeQuoted()))
	}

	text, err := elem.Text()
	if err != nil {
		return errorResult(err, "Failed to get text from element")
	}

	d.clipboard = text

	result := successResult(fmt.Sprintf("Copied text: %s", text), info)
	result.Data = text
	return result
}

// pasteText pastes clipboard text into the focused element.
func (d *Driver) pasteText(step *flow.PasteTextStep) *core.CommandResult {
	if d.clipboard == "" {
		return errorResult(fmt.Errorf("clipboard is empty"), "Clipboard is empty")
	}

	if err := d.page.InsertText(d.clipboard); err != nil {
		return errorResult(err, "Failed to paste text")
	}

	return successResult(fmt.Sprintf("Pasted text: %s", d.clipboard), nil)
}

// setClipboard stores text in the driver's clipboard.
func (d *Driver) setClipboard(step *flow.SetClipboardStep) *core.CommandResult {
	d.clipboard = step.Text
	return successResult(fmt.Sprintf("Set clipboard: %s", step.Text), nil)
}

// setOrientation changes viewport dimensions to simulate orientation.
func (d *Driver) setOrientation(step *flow.SetOrientationStep) *core.CommandResult {
	switch strings.ToUpper(step.Orientation) {
	case "LANDSCAPE", "LANDSCAPE_LEFT", "LANDSCAPE_RIGHT":
		if d.viewportW < d.viewportH {
			d.viewportW, d.viewportH = d.viewportH, d.viewportW
		}
	default: // PORTRAIT
		if d.viewportW > d.viewportH {
			d.viewportW, d.viewportH = d.viewportH, d.viewportW
		}
	}

	err := d.page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  d.viewportW,
		Height: d.viewportH,
	})
	if err != nil {
		return errorResult(err, "Failed to set orientation")
	}

	return successResult(fmt.Sprintf("Set orientation: %s", step.Orientation), nil)
}

// openLink navigates to a URL.
func (d *Driver) openLink(step *flow.OpenLinkStep) *core.CommandResult {
	return d.navigateToURL(step.Link)
}

// openBrowser navigates to a URL.
func (d *Driver) openBrowser(step *flow.OpenBrowserStep) *core.CommandResult {
	return d.navigateToURL(step.URL)
}

// navigateToURL navigates to a URL and waits for load (shared by openLink/openBrowser).
func (d *Driver) navigateToURL(url string) *core.CommandResult {
	if err := d.page.Navigate(url); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open %s", url))
	}
	d.waitForPageReady()
	return successResult(fmt.Sprintf("Opened %s", url), nil)
}

// setLocation sets geolocation via Emulation CDP domain.
func (d *Driver) setLocation(step *flow.SetLocationStep) *core.CommandResult {
	lat, err := strconv.ParseFloat(step.Latitude, 64)
	if err != nil {
		return errorResult(err, "Invalid latitude")
	}
	lng, err := strconv.ParseFloat(step.Longitude, 64)
	if err != nil {
		return errorResult(err, "Invalid longitude")
	}

	// Grant geolocation permission
	if err := (proto.BrowserGrantPermissions{
		Permissions: []proto.BrowserPermissionType{proto.BrowserPermissionTypeGeolocation},
	}).Call(d.browser); err != nil {
		log.Printf("[browser] setLocation: failed to grant geolocation permission: %v", err)
	}

	accuracy := 100.0
	err = proto.EmulationSetGeolocationOverride{
		Latitude:  &lat,
		Longitude: &lng,
		Accuracy:  &accuracy,
	}.Call(d.page)
	if err != nil {
		return errorResult(err, "Failed to set geolocation")
	}

	return successResult(fmt.Sprintf("Set location: %s, %s", step.Latitude, step.Longitude), nil)
}

// waitUntil waits for an element to become visible or not visible.
// Uses RAF-based browser-side polling for fast resolution (~16ms per check)
// instead of CDP round-trips with 100ms intervals.
func (d *Driver) waitUntil(step *flow.WaitUntilStep) *core.CommandResult {
	timeoutMs := step.TimeoutMs
	if timeoutMs == 0 {
		timeoutMs = 30000
	}

	if step.Visible != nil {
		return d.waitUntilVisible(*step.Visible, timeoutMs)
	}
	if step.NotVisible != nil {
		return d.waitUntilNotVisible(*step.NotVisible, timeoutMs)
	}

	return errorResult(fmt.Errorf("no visible/notVisible condition"), "Wait condition missing")
}

// waitUntilVisible uses RAF-based JS polling to wait for an element to appear.
func (d *Driver) waitUntilVisible(sel flow.Selector, timeoutMs int) *core.CommandResult {
	selectorType, selectorValue := jsSelectorTypeValue(sel)
	desc := sel.DescribeQuoted()

	if selectorType != "" {
		result, err := d.page.Timeout(time.Duration(timeoutMs+1000) * time.Millisecond).Evaluate(
			rod.Eval(`(type, value, timeout) => window.__maestro.waitForVisible(type, value, timeout)`,
				selectorType, selectorValue, timeoutMs).ByPromise(),
		)
		if err != nil {
			return errorResult(
				fmt.Errorf("wait condition not met within %dms: %w", timeoutMs, err),
				fmt.Sprintf("Wait condition not met within %ds", timeoutMs/1000),
			)
		}
		if result.Value.Bool() {
			return successResult(fmt.Sprintf("Element %s is now visible", desc), nil)
		}
		return errorResult(
			fmt.Errorf("wait condition not met within %dms", timeoutMs),
			fmt.Sprintf("Wait condition not met within %ds", timeoutMs/1000),
		)
	}

	// Fallback for selector types not supported by JS helper: Go-side polling
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	for time.Now().Before(deadline) {
		_, info, err := d.findElementOnce(sel)
		if err == nil && info != nil && info.Visible {
			return successResult(fmt.Sprintf("Element %s is now visible", desc), info)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errorResult(
		fmt.Errorf("wait condition not met within %dms", timeoutMs),
		fmt.Sprintf("Wait condition not met within %ds", timeoutMs/1000),
	)
}

// waitUntilNotVisible uses RAF-based JS polling to wait for an element to disappear.
func (d *Driver) waitUntilNotVisible(sel flow.Selector, timeoutMs int) *core.CommandResult {
	selectorType, selectorValue := jsSelectorTypeValue(sel)
	desc := sel.DescribeQuoted()

	if selectorType != "" {
		result, err := d.page.Timeout(time.Duration(timeoutMs+1000) * time.Millisecond).Evaluate(
			rod.Eval(`(type, value, timeout) => window.__maestro.waitForNotVisible(type, value, timeout)`,
				selectorType, selectorValue, timeoutMs).ByPromise(),
		)
		if err != nil {
			// JS eval failed (page navigated, etc.) — element is gone
			return successResult(fmt.Sprintf("Element %s is no longer visible", desc), nil)
		}
		if result.Value.Bool() {
			return successResult(fmt.Sprintf("Element %s is no longer visible", desc), nil)
		}
		return errorResult(
			fmt.Errorf("element still visible after %dms", timeoutMs),
			fmt.Sprintf("Element %s is still visible", desc),
		)
	}

	// Fallback for selector types not supported by JS helper: Go-side polling
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	for time.Now().Before(deadline) {
		_, info, err := d.findElementOnce(sel)
		if err != nil || info == nil || !info.Visible {
			return successResult(fmt.Sprintf("Element %s is no longer visible", desc), nil)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errorResult(
		fmt.Errorf("element still visible after %dms", timeoutMs),
		fmt.Sprintf("Element %s is still visible", desc),
	)
}

// waitForAnimationToEnd waits for the DOM to stabilize.
func (d *Driver) waitForAnimationToEnd(step *flow.WaitForAnimationToEndStep) *core.CommandResult {
	p := d.page.Timeout(10 * time.Second)
	if err := p.WaitDOMStable(300*time.Millisecond, 0); err != nil {
		return errorResult(err, "DOM did not stabilize")
	}
	return successResult("Animation ended (DOM stable)", nil)
}

// takeScreenshot captures a full-page screenshot.
func (d *Driver) takeScreenshot(step *flow.TakeScreenshotStep) *core.CommandResult {
	data, err := d.page.Screenshot(true, nil)
	if err != nil {
		return errorResult(err, "Failed to take screenshot")
	}

	result := successResult("Screenshot taken", nil)
	result.Data = data
	return result
}

// acceptAlert accepts the currently showing dialog.
func (d *Driver) acceptAlert(step *flow.AcceptAlertStep) *core.CommandResult {
	return d.handleDialog(true)
}

// dismissAlert dismisses the currently showing dialog.
func (d *Driver) dismissAlert(step *flow.DismissAlertStep) *core.CommandResult {
	return d.handleDialog(false)
}

// handleDialog accepts or dismisses the current JS dialog (shared by acceptAlert/dismissAlert).
// Dialogs are auto-accepted by startDialogHandler to unblock CDP. If the dialog was already
// auto-handled, we drain the channel and succeed — the explicit step is still meaningful
// as documentation of intent.
func (d *Driver) handleDialog(accept bool) *core.CommandResult {
	err := proto.PageHandleJavaScriptDialog{Accept: accept}.Call(d.page)
	if err != nil {
		// Dialog may have been auto-handled — check if one was recently captured
		select {
		case <-d.dialogCh:
			// Dialog existed and was auto-accepted
		default:
			action := "accept"
			if !accept {
				action = "dismiss"
			}
			return errorResult(err, fmt.Sprintf("No alert to %s", action))
		}
	}
	if accept {
		return successResult("Accepted alert", nil)
	}
	return successResult("Dismissed alert", nil)
}

// --- Helper functions ---

// isOptional returns true if the Optional pointer is set and true.
func isOptional(opt *bool) bool {
	return opt != nil && *opt
}

// parsePercentageCoords parses "x%, y%" format coordinates.
func parsePercentageCoords(point string) (float64, float64, error) {
	parts := strings.Split(point, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid point format: %s", point)
	}

	xStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[0]), "%"))
	yStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[1]), "%"))

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

// mapKey maps a key name to a Rod input key.
func mapKey(name string) input.Key {
	switch strings.ToLower(name) {
	case "enter":
		return input.Enter
	case "back", "backspace", "delete":
		return input.Backspace
	case "tab":
		return input.Tab
	case "space":
		return input.Space
	case "escape", "esc":
		return input.Escape
	case "home":
		return input.Home
	case "end":
		return input.End
	case "dpad_up", "arrow_up", "up":
		return input.ArrowUp
	case "dpad_down", "arrow_down", "down":
		return input.ArrowDown
	case "dpad_left", "arrow_left", "left":
		return input.ArrowLeft
	case "dpad_right", "arrow_right", "right":
		return input.ArrowRight
	case "page_up":
		return input.PageUp
	case "page_down":
		return input.PageDown
	default:
		return 0
	}
}

// convertToKeys converts a string to input.Key slice for typing.
func convertToKeys(text string) []input.Key {
	var keys []input.Key
	for _, ch := range text {
		keys = append(keys, input.Key(ch))
	}
	return keys
}

// --- Random text generators ---

const alphanumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// cryptoRandIntn returns a cryptographically secure random int in [0, max).
func cryptoRandIntn(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		log.Printf("[browser] crypto/rand failed, using fallback: %v", err)
		return 0
	}
	return int(n.Int64())
}

func randomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = alphanumeric[cryptoRandIntn(len(alphanumeric))]
	}
	return string(b)
}

func randomNumber(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = '0' + byte(cryptoRandIntn(10))
	}
	return string(b)
}

func randomEmail() string {
	return randomString(8) + "@" + randomString(6) + ".com"
}

// evalBrowserScript executes JavaScript in the browser page context via CDP.
// Returns the script's return value as a string in result.Data.
func (d *Driver) evalBrowserScript(step *flow.EvalBrowserScriptStep) *core.CommandResult {
	if step.Script == "" {
		return errorResult(fmt.Errorf("evalBrowserScript: script is empty"), "")
	}

	// Pass as async arrow function — Rod wraps it via .apply(this, arguments)
	// and Page.Eval sets AwaitPromise=true, so await works inside the script.
	js := fmt.Sprintf("async () => { %s }", step.Script)

	obj, err := d.page.Eval(js)
	if err != nil {
		return errorResult(fmt.Errorf("evalBrowserScript: %w", err), "")
	}

	// Convert result to string for variable storage
	val := ""
	if obj != nil && obj.Value.Val() != nil {
		val = obj.Value.Str()
	}

	result := successResult("evalBrowserScript completed", nil)
	result.Data = val
	return result
}

// runBrowserScript loads a JS file and executes it in the browser page context.
func (d *Driver) runBrowserScript(step *flow.RunBrowserScriptStep) *core.CommandResult {
	if step.File == "" {
		return errorResult(fmt.Errorf("runBrowserScript: file is required"), "")
	}

	data, err := os.ReadFile(step.File) //#nosec G304 -- user-provided script file
	if err != nil {
		return errorResult(fmt.Errorf("runBrowserScript: %w", err), "")
	}

	// Inject env vars as window.__env before running the script
	var envSetup string
	if len(step.Env) > 0 {
		envJSON, _ := json.Marshal(step.Env)
		envSetup = fmt.Sprintf("window.__env = %s;\n", envJSON)
	}

	js := fmt.Sprintf("async () => { %s%s }", envSetup, string(data))

	obj, err := d.page.Eval(js)
	if err != nil {
		return errorResult(fmt.Errorf("runBrowserScript: %w", err), "")
	}

	val := ""
	if obj != nil && obj.Value.Val() != nil {
		val = obj.Value.Str()
	}

	result := successResult(fmt.Sprintf("Executed browser script: %s", filepath.Base(step.File)), nil)
	result.Data = val
	return result
}

// getConsoleLogs returns captured browser console logs as JSON.
func (d *Driver) getConsoleLogs() *core.CommandResult {
	logs := d.ConsoleLogs()

	data, err := json.Marshal(logs)
	if err != nil {
		return errorResult(fmt.Errorf("getConsoleLogs: %w", err), "")
	}

	result := successResult(fmt.Sprintf("Got %d console log(s)", len(logs)), nil)
	result.Data = string(data)
	return result
}

// clearConsoleLogs clears captured browser console logs.
func (d *Driver) clearConsoleLogs() *core.CommandResult {
	d.consoleMu.Lock()
	d.consoleLogs = nil
	d.consoleMu.Unlock()
	return successResult("Cleared console logs", nil)
}

// assertNoJSErrors asserts that no console errors or uncaught exceptions occurred.
func (d *Driver) assertNoJSErrors() *core.CommandResult {
	d.consoleMu.Lock()
	defer d.consoleMu.Unlock()

	var errors []string
	for _, entry := range d.consoleLogs {
		if entry.Level == "error" || entry.Level == "exception" {
			errors = append(errors, fmt.Sprintf("[%s] %s", entry.Level, entry.Message))
		}
	}

	if len(errors) > 0 {
		return errorResult(
			fmt.Errorf("%d JS error(s) detected", len(errors)),
			strings.Join(errors, "\n"),
		)
	}

	return successResult("No JS errors detected", nil)
}

// setCookies sets browser cookies via CDP.
func (d *Driver) setCookies(step *flow.SetCookiesStep) *core.CommandResult {
	if len(step.Cookies) == 0 {
		return errorResult(fmt.Errorf("setCookies: no cookies provided"), "")
	}

	params := make([]*proto.NetworkCookieParam, len(step.Cookies))
	for i, c := range step.Cookies {
		params[i] = &proto.NetworkCookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
		}
		if c.SameSite != "" {
			params[i].SameSite = proto.NetworkCookieSameSite(c.SameSite)
		}
		if c.Expires > 0 {
			params[i].Expires = proto.TimeSinceEpoch(c.Expires)
		}
	}

	err := proto.NetworkSetCookies{Cookies: params}.Call(d.page)
	if err != nil {
		return errorResult(fmt.Errorf("setCookies: %w", err), "")
	}

	return successResult(fmt.Sprintf("Set %d cookie(s)", len(step.Cookies)), nil)
}

// getCookies retrieves all browser cookies and returns them as JSON.
func (d *Driver) getCookies(step *flow.GetCookiesStep) *core.CommandResult {
	res, err := proto.NetworkGetCookies{}.Call(d.page)
	if err != nil {
		return errorResult(fmt.Errorf("getCookies: %w", err), "")
	}

	data, err := json.Marshal(res.Cookies)
	if err != nil {
		return errorResult(fmt.Errorf("getCookies: failed to marshal: %w", err), "")
	}

	result := successResult(fmt.Sprintf("Got %d cookie(s)", len(res.Cookies)), nil)
	result.Data = string(data)
	return result
}

// authState is the JSON structure for saveAuthState/loadAuthState.
type authState struct {
	Cookies        []*proto.NetworkCookie `json:"cookies"`
	LocalStorage   map[string]string      `json:"localStorage"`
	SessionStorage map[string]string      `json:"sessionStorage"`
}

// getStorageItems reads all key-value pairs from localStorage or sessionStorage.
func (d *Driver) getStorageItems(storageName string) map[string]string {
	js := fmt.Sprintf(`() => {
		const items = {};
		for (let i = 0; i < %s.length; i++) {
			const key = %s.key(i);
			items[key] = %s.getItem(key);
		}
		return JSON.stringify(items);
	}`, storageName, storageName, storageName)
	obj, err := d.page.Eval(js)
	items := map[string]string{}
	if err == nil && obj != nil && obj.Value.Str() != "" {
		_ = json.Unmarshal([]byte(obj.Value.Str()), &items)
	}
	return items
}

// setStorageItems writes key-value pairs into localStorage or sessionStorage.
func (d *Driver) setStorageItems(storageName string, items map[string]string) error {
	itemsJSON, _ := json.Marshal(items)
	js := fmt.Sprintf(`(items) => {
		const parsed = JSON.parse(items);
		for (const [key, value] of Object.entries(parsed)) {
			%s.setItem(key, value);
		}
	}`, storageName)
	_, err := d.page.Eval(js, string(itemsJSON))
	return err
}

// saveAuthState saves cookies + localStorage + sessionStorage to a JSON file.
func (d *Driver) saveAuthState(step *flow.SaveAuthStateStep) *core.CommandResult {
	if step.Path == "" {
		return errorResult(fmt.Errorf("saveAuthState: path is required"), "")
	}

	// Get cookies
	cookieRes, err := proto.NetworkGetCookies{}.Call(d.page)
	if err != nil {
		return errorResult(fmt.Errorf("saveAuthState: failed to get cookies: %w", err), "")
	}

	localStorage := d.getStorageItems("localStorage")
	sessionStorage := d.getStorageItems("sessionStorage")

	state := authState{
		Cookies:        cookieRes.Cookies,
		LocalStorage:   localStorage,
		SessionStorage: sessionStorage,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return errorResult(fmt.Errorf("saveAuthState: failed to marshal: %w", err), "")
	}

	if dir := filepath.Dir(step.Path); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return errorResult(fmt.Errorf("saveAuthState: failed to create directory: %w", err), "")
		}
	}

	if err := os.WriteFile(step.Path, data, 0o600); err != nil {
		return errorResult(fmt.Errorf("saveAuthState: failed to write file: %w", err), "")
	}

	return successResult(fmt.Sprintf("Saved auth state to %s (%d cookies, %d localStorage, %d sessionStorage)",
		step.Path, len(cookieRes.Cookies), len(localStorage), len(sessionStorage)), nil)
}

// loadAuthState loads cookies + localStorage + sessionStorage from a JSON file.
func (d *Driver) loadAuthState(step *flow.LoadAuthStateStep) *core.CommandResult {
	if step.Path == "" {
		return errorResult(fmt.Errorf("loadAuthState: path is required"), "")
	}

	data, err := os.ReadFile(step.Path)
	if err != nil {
		return errorResult(fmt.Errorf("loadAuthState: failed to read file: %w", err), "")
	}

	var state authState
	if err := json.Unmarshal(data, &state); err != nil {
		return errorResult(fmt.Errorf("loadAuthState: failed to parse: %w", err), "")
	}

	// Set cookies
	if len(state.Cookies) > 0 {
		params := proto.CookiesToParams(state.Cookies)
		if err := (proto.NetworkSetCookies{Cookies: params}).Call(d.page); err != nil {
			return errorResult(fmt.Errorf("loadAuthState: failed to set cookies: %w", err), "")
		}
	}

	// Set localStorage
	if len(state.LocalStorage) > 0 {
		if err := d.setStorageItems("localStorage", state.LocalStorage); err != nil {
			log.Printf("[browser] loadAuthState: failed to set localStorage: %v", err)
		}
	}

	// Set sessionStorage
	if len(state.SessionStorage) > 0 {
		if err := d.setStorageItems("sessionStorage", state.SessionStorage); err != nil {
			log.Printf("[browser] loadAuthState: failed to set sessionStorage: %v", err)
		}
	}

	return successResult(fmt.Sprintf("Loaded auth state from %s (%d cookies, %d localStorage, %d sessionStorage)",
		step.Path, len(state.Cookies), len(state.LocalStorage), len(state.SessionStorage)), nil)
}

// uploadFile sets files on a file input element.
func (d *Driver) uploadFile(step *flow.UploadFileStep) *core.CommandResult {
	paths := step.Paths
	if step.Path != "" {
		paths = append([]string{step.Path}, paths...)
	}
	if len(paths) == 0 {
		return errorResult(fmt.Errorf("uploadFile: no file path(s) provided"), "")
	}

	// Verify files exist
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			return errorResult(fmt.Errorf("uploadFile: file not found: %s", p), "")
		}
	}

	elem, info, err := d.findElement(step.Selector, false, step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to find file input %s", step.Selector.DescribeQuoted()))
	}

	if err := elem.SetFiles(paths); err != nil {
		return errorResult(fmt.Errorf("uploadFile: %w", err), "")
	}

	return successResult(fmt.Sprintf("Uploaded %d file(s) to %s", len(paths), step.Selector.DescribeQuoted()), info)
}

// waitForDownload waits for a browser download to complete.
func (d *Driver) waitForDownload(step *flow.WaitForDownloadStep) *core.CommandResult {
	downloadDir := step.SaveTo
	if downloadDir == "" {
		downloadDir = os.TempDir()
	}
	if err := os.MkdirAll(downloadDir, 0o750); err != nil {
		return errorResult(fmt.Errorf("waitForDownload: failed to create directory: %w", err), "")
	}

	// Enable download behavior
	err := proto.BrowserSetDownloadBehavior{
		Behavior:      "allowAndName",
		DownloadPath:  downloadDir,
		EventsEnabled: true,
	}.Call(d.browser)
	if err != nil {
		return errorResult(fmt.Errorf("waitForDownload: failed to set download behavior: %w", err), "")
	}

	timeoutMs := step.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}

	// Wait for download to complete
	doneCh := make(chan string, 1)
	var filename string

	wait := d.browser.EachEvent(func(e *proto.BrowserDownloadWillBegin) {
		filename = e.SuggestedFilename
	}, func(e *proto.BrowserDownloadProgress) bool {
		if e.State == proto.BrowserDownloadProgressStateCompleted {
			// Rename from GUID to suggested filename
			src := filepath.Join(downloadDir, e.GUID)
			dst := filepath.Join(downloadDir, filename)
			if filename != "" {
				_ = os.Rename(src, dst)
			}
			doneCh <- filename
			return true // stop listening
		}
		if e.State == proto.BrowserDownloadProgressStateCanceled {
			doneCh <- ""
			return true
		}
		return false // keep listening
	})
	go wait()

	select {
	case name := <-doneCh:
		if name == "" {
			return errorResult(fmt.Errorf("waitForDownload: download was canceled"), "")
		}
		if step.AssertFilename != "" && name != step.AssertFilename {
			return errorResult(fmt.Errorf("waitForDownload: expected filename %q, got %q", step.AssertFilename, name), "")
		}
		msg := fmt.Sprintf("Downloaded %s to %s", name, downloadDir)
		result := successResult(msg, nil)
		result.Data = filepath.Join(downloadDir, name)
		return result
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return errorResult(fmt.Errorf("waitForDownload: timed out after %dms", timeoutMs), "")
	}
}

// grantPermissions grants browser permissions.
func (d *Driver) grantPermissions(step *flow.GrantPermissionsStep) *core.CommandResult {
	if len(step.Permissions) == 0 {
		return errorResult(fmt.Errorf("grantPermissions: no permissions provided"), "")
	}

	perms := make([]proto.BrowserPermissionType, len(step.Permissions))
	for i, p := range step.Permissions {
		perms[i] = proto.BrowserPermissionType(p)
	}

	req := proto.BrowserGrantPermissions{Permissions: perms}
	if step.Origin != "" {
		req.Origin = step.Origin
	}

	if err := req.Call(d.browser); err != nil {
		return errorResult(fmt.Errorf("grantPermissions: %w", err), "")
	}

	return successResult(fmt.Sprintf("Granted %d permission(s)", len(step.Permissions)), nil)
}

// resetPermissions resets all browser permissions.
func (d *Driver) resetPermissions() *core.CommandResult {
	if err := (proto.BrowserResetPermissions{}).Call(d.browser); err != nil {
		return errorResult(fmt.Errorf("resetPermissions: %w", err), "")
	}
	return successResult("Reset all permissions", nil)
}

// initPage sets viewport and injects JS helpers on a new page.
func (d *Driver) initPage(page *rod.Page) error {
	if err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  d.viewportW,
		Height: d.viewportH,
	}); err != nil {
		return err
	}
	_, err := page.EvalOnNewDocument(jsHelperCode)
	return err
}

// openTab opens a new browser tab.
func (d *Driver) openTab(step *flow.OpenTabStep) *core.CommandResult {
	page, err := d.browser.Page(proto.TargetCreateTarget{URL: step.URL})
	if err != nil {
		return errorResult(fmt.Errorf("openTab: %w", err), "")
	}

	if err := d.initPage(page); err != nil {
		return errorResult(fmt.Errorf("openTab: failed to init page: %w", err), "")
	}

	if step.URL != "" {
		page.MustWaitLoad()
		p := page.Timeout(5 * time.Second)
		_ = p.WaitDOMStable(300*time.Millisecond, 0)
	}

	d.setupNetworkTracking(page)

	if step.TabLabel != "" {
		d.tabLabels[step.TabLabel] = page
	}

	d.page = page
	return successResult(fmt.Sprintf("Opened new tab%s", labelSuffix(step.TabLabel)), nil)
}

// switchTab switches to another browser tab by label, index, or URL pattern.
func (d *Driver) switchTab(step *flow.SwitchTabStep) *core.CommandResult {
	// By label
	if step.TabLabel != "" {
		page, ok := d.tabLabels[step.TabLabel]
		if !ok {
			return errorResult(fmt.Errorf("switchTab: no tab with label %q", step.TabLabel), "")
		}
		d.page = page
		return successResult(fmt.Sprintf("Switched to tab %q", step.TabLabel), nil)
	}

	// Get all pages
	pages, err := d.browser.Pages()
	if err != nil {
		return errorResult(fmt.Errorf("switchTab: %w", err), "")
	}

	// By index
	if step.URL == "" {
		if step.Index < 0 || step.Index >= len(pages) {
			return errorResult(fmt.Errorf("switchTab: index %d out of range (have %d tabs)", step.Index, len(pages)), "")
		}
		d.page = pages[step.Index]
		return successResult(fmt.Sprintf("Switched to tab index %d", step.Index), nil)
	}

	// By URL pattern
	for _, p := range pages {
		info, err := p.Info()
		if err != nil {
			continue
		}
		if matchURLPattern(info.URL, step.URL) {
			d.page = p
			return successResult(fmt.Sprintf("Switched to tab matching %q", step.URL), nil)
		}
	}

	return errorResult(fmt.Errorf("switchTab: no tab matching URL pattern %q", step.URL), "")
}

// closeTab closes the current tab and switches to the previous one.
func (d *Driver) closeTab() *core.CommandResult {
	pages, err := d.browser.Pages()
	if err != nil {
		return errorResult(fmt.Errorf("closeTab: %w", err), "")
	}
	if len(pages) <= 1 {
		return errorResult(fmt.Errorf("closeTab: cannot close the last tab"), "")
	}

	current := d.page

	// Remove from labels
	for label, p := range d.tabLabels {
		if p == current {
			delete(d.tabLabels, label)
			break
		}
	}

	// Find another page to switch to
	for _, p := range pages {
		if p != current {
			d.page = p
			break
		}
	}

	if err := current.Close(); err != nil {
		return errorResult(fmt.Errorf("closeTab: %w", err), "")
	}

	return successResult("Closed tab", nil)
}

// matchURLPattern checks if a URL matches a simple glob pattern (supports *).
func matchURLPattern(url, pattern string) bool {
	if pattern == "" {
		return false
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return strings.Contains(url, pattern)
	}
	idx := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		pos := strings.Index(url[idx:], part)
		if pos < 0 {
			return false
		}
		idx += pos + len(part)
	}
	return true
}

func labelSuffix(label string) string {
	if label != "" {
		return fmt.Sprintf(" (label: %s)", label)
	}
	return ""
}

// ============================================
// Network Interception
// ============================================

// networkMock describes a single mock rule for intercepted requests.
type networkMock struct {
	URLPattern string
	Method     string // empty = match all methods
	Status     int
	Headers    map[string]string
	Body       string
}

// mockNetwork adds a mock rule and enables Fetch interception.
func (d *Driver) mockNetwork(step *flow.MockNetworkStep) *core.CommandResult {
	if step.URL == "" {
		return errorResult(fmt.Errorf("mockNetwork: url is required"), "")
	}

	mock := networkMock{
		URLPattern: step.URL,
		Method:     strings.ToUpper(step.Method),
		Status:     step.Response.Status,
		Headers:    step.Response.Headers,
		Body:       step.Response.Body,
	}
	if mock.Status == 0 {
		mock.Status = 200
	}

	d.networkMu.Lock()
	d.networkMocks = append(d.networkMocks, mock)
	d.networkMu.Unlock()

	if err := d.enableFetchInterception(); err != nil {
		return errorResult(fmt.Errorf("mockNetwork: %w", err), "")
	}

	return successResult(fmt.Sprintf("Mocked %s %s → %d", step.Method, step.URL, mock.Status), nil)
}

// blockNetwork adds URL patterns to block and enables Fetch interception.
func (d *Driver) blockNetwork(step *flow.BlockNetworkStep) *core.CommandResult {
	if len(step.Patterns) == 0 {
		return errorResult(fmt.Errorf("blockNetwork: no patterns provided"), "")
	}

	d.networkMu.Lock()
	d.networkBlocks = append(d.networkBlocks, step.Patterns...)
	d.networkMu.Unlock()

	if err := d.enableFetchInterception(); err != nil {
		return errorResult(fmt.Errorf("blockNetwork: %w", err), "")
	}

	return successResult(fmt.Sprintf("Blocking %d URL pattern(s)", len(step.Patterns)), nil)
}

// enableFetchInterception enables CDP Fetch domain and starts the interception handler.
// Safe to call multiple times — only enables once.
func (d *Driver) enableFetchInterception() error {
	d.networkMu.Lock()
	alreadyEnabled := d.fetchEnabled
	d.fetchEnabled = true
	d.networkMu.Unlock()

	if alreadyEnabled {
		return nil
	}

	// Enable Fetch domain — intercept all requests
	if err := (proto.FetchEnable{}).Call(d.page); err != nil {
		return fmt.Errorf("failed to enable Fetch domain: %w", err)
	}

	// Start background handler for intercepted requests
	go d.page.EachEvent(func(e *proto.FetchRequestPaused) bool {
		d.handleInterceptedRequest(e)
		select {
		case <-d.stopCh:
			return true // stop on driver close
		default:
			return false // keep listening
		}
	})()

	return nil
}

// handleInterceptedRequest processes an intercepted request against mocks and blocks.
func (d *Driver) handleInterceptedRequest(e *proto.FetchRequestPaused) {
	url := e.Request.URL
	method := e.Request.Method

	d.networkMu.Lock()
	mocks := make([]networkMock, len(d.networkMocks))
	copy(mocks, d.networkMocks)
	blocks := make([]string, len(d.networkBlocks))
	copy(blocks, d.networkBlocks)
	d.networkMu.Unlock()

	// Check blocks first
	for _, pattern := range blocks {
		if matchURLPattern(url, pattern) {
			_ = (proto.FetchFailRequest{
				RequestID:   e.RequestID,
				ErrorReason: proto.NetworkErrorReasonBlockedByClient,
			}).Call(d.page)
			return
		}
	}

	// Check mocks
	for _, mock := range mocks {
		if !matchURLPattern(url, mock.URLPattern) {
			continue
		}
		if mock.Method != "" && mock.Method != method {
			continue
		}

		// Build response headers
		var headers []*proto.FetchHeaderEntry
		for k, v := range mock.Headers {
			headers = append(headers, &proto.FetchHeaderEntry{Name: k, Value: v})
		}

		_ = (proto.FetchFulfillRequest{
			RequestID:       e.RequestID,
			ResponseCode:    mock.Status,
			ResponseHeaders: headers,
			Body:            []byte(mock.Body),
		}).Call(d.page)
		return
	}

	// No match — continue request normally
	_ = (proto.FetchContinueRequest{
		RequestID: e.RequestID,
	}).Call(d.page)
}

// setNetworkConditions emulates network throttling or offline mode.
func (d *Driver) setNetworkConditions(step *flow.SetNetworkConditionsStep) *core.CommandResult {
	// Convert KB/s to bytes/sec (-1 means no throttle)
	download := step.DownloadSpeed * 1024
	if step.DownloadSpeed <= 0 {
		download = -1
	}
	upload := step.UploadSpeed * 1024
	if step.UploadSpeed <= 0 {
		upload = -1
	}

	err := (proto.NetworkEmulateNetworkConditions{
		Offline:            step.Offline,
		Latency:            step.Latency,
		DownloadThroughput: download,
		UploadThroughput:   upload,
	}).Call(d.page)
	if err != nil {
		return errorResult(fmt.Errorf("setNetworkConditions: %w", err), "")
	}

	if step.Offline {
		return successResult("Set network to offline", nil)
	}
	return successResult(fmt.Sprintf("Set network conditions: latency=%.0fms, download=%.0fKB/s, upload=%.0fKB/s",
		step.Latency, step.DownloadSpeed, step.UploadSpeed), nil)
}

// waitForRequest waits for a matching network request to be made.
func (d *Driver) waitForRequest(step *flow.WaitForRequestStep) *core.CommandResult {
	if step.URL == "" {
		return errorResult(fmt.Errorf("waitForRequest: url is required"), "")
	}

	timeoutMs := step.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}

	// Enable network events
	if err := (proto.NetworkEnable{}).Call(d.page); err != nil {
		return errorResult(fmt.Errorf("waitForRequest: %w", err), "")
	}

	matchMethod := strings.ToUpper(step.Method)
	doneCh := make(chan string, 1)

	wait := d.page.EachEvent(func(e *proto.NetworkRequestWillBeSent) bool {
		if !matchURLPattern(e.Request.URL, step.URL) {
			return false
		}
		if matchMethod != "" && e.Request.Method != matchMethod {
			return false
		}
		doneCh <- e.Request.PostData
		return true // stop listening
	})
	go wait()

	select {
	case body := <-doneCh:
		result := successResult(fmt.Sprintf("Request matched: %s", step.URL), nil)
		result.Data = body
		return result
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return errorResult(fmt.Errorf("waitForRequest: no request matching %q within %dms", step.URL, timeoutMs), "")
	}
}

// clearNetworkMocks disables Fetch interception and clears all mocks and blocks.
func (d *Driver) clearNetworkMocks() *core.CommandResult {
	d.networkMu.Lock()
	d.networkMocks = nil
	d.networkBlocks = nil
	wasFetchEnabled := d.fetchEnabled
	d.fetchEnabled = false
	d.networkMu.Unlock()

	if wasFetchEnabled {
		if err := (proto.FetchDisable{}).Call(d.page); err != nil {
			log.Printf("[browser] clearNetworkMocks: failed to disable Fetch: %v", err)
		}
	}

	// Reset network conditions to default
	_ = (proto.NetworkEmulateNetworkConditions{
		Offline:            false,
		Latency:            0,
		DownloadThroughput: -1,
		UploadThroughput:   -1,
	}).Call(d.page)

	return successResult("Cleared all network mocks and conditions", nil)
}

var firstNames = []string{"Alice", "Bob", "Charlie", "Diana", "Eve", "Frank", "Grace", "Henry"}
var lastNames = []string{"Smith", "Johnson", "Brown", "Taylor", "Wilson", "Davis", "Clark", "Lewis"}

func randomPersonName() string {
	return firstNames[cryptoRandIntn(len(firstNames))] + " " + lastNames[cryptoRandIntn(len(lastNames))]
}
