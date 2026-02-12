package appium

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// DefaultFindTimeout is the default timeout for element operations.
const DefaultFindTimeout = 10 * time.Second

// Driver implements core.Driver using Appium server.
type Driver struct {
	client                    *Client
	platform                  string        // detected from page source or capabilities
	appID                     string        // current app ID
	findTimeout               time.Duration // configurable timeout for finding elements
	currentWaitForIdleTimeout int           // track current value to skip redundant calls
	waitForIdleTimeoutSet     bool          // whether waitForIdleTimeout has been set
}

// NewDriver creates a new Appium driver.
func NewDriver(serverURL string, capabilities map[string]interface{}) (*Driver, error) {
	client := NewClient(serverURL)

	if err := client.Connect(capabilities); err != nil {
		return nil, err
	}

	d := &Driver{
		client:   client,
		platform: client.Platform(),
	}

	// Extract app ID from capabilities
	if appID, ok := capabilities["appium:appPackage"].(string); ok {
		d.appID = appID
	} else if appID, ok := capabilities["appium:bundleId"].(string); ok {
		d.appID = appID
	}

	// Track waitForIdleTimeout if set via appium:settings capability
	if settings, ok := capabilities["appium:settings"].(map[string]interface{}); ok {
		if val, ok := settings["waitForIdleTimeout"].(int); ok {
			d.currentWaitForIdleTimeout = val
			d.waitForIdleTimeoutSet = true
		} else if val, ok := settings["waitForIdleTimeout"].(float64); ok {
			d.currentWaitForIdleTimeout = int(val)
			d.waitForIdleTimeoutSet = true
		}
	}

	return d, nil
}

// Close disconnects from Appium server.
func (d *Driver) Close() error {
	return d.client.Disconnect()
}

// Execute implements core.Driver.
func (d *Driver) Execute(step flow.Step) *core.CommandResult {
	start := time.Now()
	result := d.executeStep(step)
	result.Duration = time.Since(start)
	return result
}

func (d *Driver) executeStep(step flow.Step) *core.CommandResult {
	switch s := step.(type) {
	case *flow.TapOnStep:
		return d.tapOn(s)
	case *flow.DoubleTapOnStep:
		return d.doubleTapOn(s)
	case *flow.LongPressOnStep:
		return d.longPressOn(s)
	case *flow.TapOnPointStep:
		return d.tapOnPoint(s)
	case *flow.SwipeStep:
		return d.swipe(s)
	case *flow.ScrollStep:
		return d.scroll(s)
	case *flow.InputTextStep:
		return d.inputText(s)
	case *flow.EraseTextStep:
		return d.eraseText(s)
	case *flow.AssertVisibleStep:
		return d.assertVisible(s)
	case *flow.AssertNotVisibleStep:
		return d.assertNotVisible(s)
	case *flow.BackStep:
		return d.back(s)
	case *flow.HideKeyboardStep:
		return d.hideKeyboard(s)
	case *flow.LaunchAppStep:
		return d.launchApp(s)
	case *flow.StopAppStep:
		return d.stopApp(s)
	case *flow.ClearStateStep:
		return d.clearState(s)
	case *flow.SetLocationStep:
		return d.setLocation(s)
	case *flow.SetOrientationStep:
		return d.setOrientation(s)
	case *flow.OpenLinkStep:
		return d.openLink(s)
	case *flow.CopyTextFromStep:
		return d.copyTextFrom(s)
	case *flow.PasteTextStep:
		return d.pasteText(s)
	case *flow.SetClipboardStep:
		return d.setClipboard(s)
	case *flow.PressKeyStep:
		return d.pressKey(s)
	case *flow.ScrollUntilVisibleStep:
		return d.scrollUntilVisible(s)
	case *flow.WaitForAnimationToEndStep:
		return d.waitForAnimationToEnd(s)
	case *flow.WaitUntilStep:
		return d.waitUntil(s)
	case *flow.KillAppStep:
		return d.killApp(s)
	case *flow.InputRandomStep:
		return d.inputRandom(s)
	case *flow.TakeScreenshotStep:
		return d.takeScreenshot(s)
	default:
		return errorResult(fmt.Errorf("unsupported step type: %T", step), "")
	}
}

// Screenshot implements core.Driver.
func (d *Driver) Screenshot() ([]byte, error) {
	return d.client.Screenshot()
}

// Hierarchy implements core.Driver.
func (d *Driver) Hierarchy() ([]byte, error) {
	source, err := d.client.Source()
	if err != nil {
		return nil, err
	}
	return []byte(source), nil
}

// GetState implements core.Driver.
func (d *Driver) GetState() *core.StateSnapshot {
	orientation, _ := d.client.GetOrientation()
	clipboard, _ := d.client.GetClipboard()

	return &core.StateSnapshot{
		Orientation:   orientation,
		ClipboardText: clipboard,
	}
}

// GetPlatformInfo implements core.Driver.
func (d *Driver) GetPlatformInfo() *core.PlatformInfo {
	w, h := d.client.ScreenSize()
	return &core.PlatformInfo{
		Platform:     d.platform,
		ScreenWidth:  w,
		ScreenHeight: h,
		AppID:        d.appID,
	}
}

// SetFindTimeout implements core.Driver.
// Sets the default timeout (in ms) for finding elements.
func (d *Driver) SetFindTimeout(ms int) {
	d.findTimeout = time.Duration(ms) * time.Millisecond
}

// SetWaitForIdleTimeout sets the wait for idle timeout.
// 0 = disabled, >0 = wait up to N ms for device to be idle.
// Skips the HTTP call if the value is already set (optimization for per-flow sessions).
func (d *Driver) SetWaitForIdleTimeout(ms int) error {
	if d.waitForIdleTimeoutSet && d.currentWaitForIdleTimeout == ms {
		return nil // already set, skip HTTP call
	}
	err := d.client.SetSettings(map[string]interface{}{
		"waitForIdleTimeout": ms,
	})
	if err == nil {
		d.currentWaitForIdleTimeout = ms
		d.waitForIdleTimeoutSet = true
	}
	return err
}

// getFindTimeout returns the configured timeout or the default.
func (d *Driver) getFindTimeout() time.Duration {
	if d.findTimeout > 0 {
		return d.findTimeout
	}
	return DefaultFindTimeout
}

// Element Finding

// findElement finds an element by selector with timeout.
func (d *Driver) findElement(sel flow.Selector, timeout time.Duration) (*core.ElementInfo, error) {
	if timeout <= 0 {
		timeout = d.getFindTimeout()
	}

	// Check if selector has relative components
	if sel.HasRelativeSelector() {
		return d.findElementRelative(sel, timeout)
	}

	// Simple selector - try Appium's native find
	deadline := time.Now().Add(timeout)

	for {
		info, err := d.findElementDirect(sel)
		if err == nil && info != nil {
			return info, nil
		}

		if time.Now().After(deadline) {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("element not found: %s", sel.Describe())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// findElementDirect finds element using Appium's native strategies.
// Uses UiAutomator selectors for Android (fast) instead of page source parsing (slow).
func (d *Driver) findElementDirect(sel flow.Selector) (*core.ElementInfo, error) {
	// If selector has state filters, use page source (UiAutomator doesn't support these)
	if sel.Enabled != nil || sel.Selected != nil || sel.Focused != nil || sel.Checked != nil {
		return d.findElementByPageSource(sel)
	}

	// Try ID first
	if sel.ID != "" {
		if d.platform == "ios" {
			if elemID, err := d.client.FindElement("accessibility id", sel.ID); err == nil {
				return d.getElementInfo(elemID)
			}
		} else {
			// Android: use UiAutomator for ID (faster than id strategy)
			escaped := escapeUIAutomatorString(sel.ID)
			uiSelector := fmt.Sprintf(`new UiSelector().resourceIdMatches(".*%s.*")`, escaped)
			if elemID, err := d.client.FindElement("-android uiautomator", uiSelector); err == nil && elemID != "" {
				return d.getElementInfo(elemID)
			}
			// Fallback to standard id strategy
			if elemID, err := d.client.FindElement("id", sel.ID); err == nil {
				return d.getElementInfo(elemID)
			}
		}
	}

	// Try text using native platform strategies (fast)
	if sel.Text != "" {
		if d.platform == "ios" {
			// iOS: use -ios predicate string
			escaped := escapeIOSPredicateString(sel.Text)
			predicate := fmt.Sprintf(`label CONTAINS[c] "%s" OR name CONTAINS[c] "%s"`, escaped, escaped)
			if elemID, err := d.client.FindElement("-ios predicate string", predicate); err == nil && elemID != "" {
				return d.getElementInfo(elemID)
			}
		} else {
			// Android: use UiAutomator selectors (much faster than page source)
			escaped := escapeUIAutomatorString(sel.Text)

			if looksLikeRegex(sel.Text) {
				// Use textMatches for regex patterns
				uiSelector := fmt.Sprintf(`new UiSelector().textMatches("%s")`, escaped)
				if elemID, err := d.client.FindElement("-android uiautomator", uiSelector); err == nil && elemID != "" {
					return d.getElementInfo(elemID)
				}
				// Try descriptionMatches
				uiSelector = fmt.Sprintf(`new UiSelector().descriptionMatches("%s")`, escaped)
				if elemID, err := d.client.FindElement("-android uiautomator", uiSelector); err == nil && elemID != "" {
					return d.getElementInfo(elemID)
				}
			} else {
				// Try exact text match first
				uiSelector := fmt.Sprintf(`new UiSelector().text("%s")`, escaped)
				if elemID, err := d.client.FindElement("-android uiautomator", uiSelector); err == nil && elemID != "" {
					return d.getElementInfo(elemID)
				}

				// Try textContains
				uiSelector = fmt.Sprintf(`new UiSelector().textContains("%s")`, escaped)
				if elemID, err := d.client.FindElement("-android uiautomator", uiSelector); err == nil && elemID != "" {
					return d.getElementInfo(elemID)
				}

				// Try description (content-desc)
				uiSelector = fmt.Sprintf(`new UiSelector().description("%s")`, escaped)
				if elemID, err := d.client.FindElement("-android uiautomator", uiSelector); err == nil && elemID != "" {
					return d.getElementInfo(elemID)
				}

				// Try descriptionContains
				uiSelector = fmt.Sprintf(`new UiSelector().descriptionContains("%s")`, escaped)
				if elemID, err := d.client.FindElement("-android uiautomator", uiSelector); err == nil && elemID != "" {
					return d.getElementInfo(elemID)
				}
			}
		}
	}

	// Fallback to page source parsing for complex selectors
	return d.findElementByPageSource(sel)
}

// escapeUIAutomatorString escapes only double quotes for UiAutomator string.
// Used when the text is already a regex pattern - backslashes are NOT escaped
// to preserve regex metacharacters like \d, \w, etc.
func escapeUIAutomatorString(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// escapeIOSPredicateString escapes quotes for iOS predicate string
func escapeIOSPredicateString(s string) string {
	var result string
	for _, c := range s {
		switch c {
		case '"':
			result += `\"`
		case '\\':
			result += `\\`
		default:
			result += string(c)
		}
	}
	return result
}

// findElementByPageSource finds element by parsing page source XML.
func (d *Driver) findElementByPageSource(sel flow.Selector) (*core.ElementInfo, error) {
	source, err := d.client.Source()
	if err != nil {
		return nil, err
	}

	elements, platform, err := ParsePageSource(source)
	if err != nil {
		return nil, err
	}
	d.platform = platform

	// Filter by selector
	candidates := FilterBySelector(elements, sel, platform)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no elements match selector")
	}

	// Prioritize clickable, then deepest
	candidates = SortClickableFirst(candidates)
	selected := DeepestMatchingElement(candidates)

	// If element isn't clickable, try to find a clickable parent
	// This handles React Native pattern where text nodes aren't clickable but containers are
	clickableElem := GetClickableElement(selected)

	return elementToInfoWithClickable(selected, clickableElem, platform), nil
}

// findElementForTap finds an element for tap commands, prioritizing clickable elements.
// When multiple elements match (e.g., "Login" title and "Login" button), prefers the clickable one.
// For Android with text-based selectors:
//  1. Try UiAutomator with .clickable(true) - fast if element itself is clickable
//  2. If text exists but not clickable → page source with clickable parent lookup
//
// This handles React Native pattern where text nodes aren't clickable but parent containers are.
func (d *Driver) findElementForTap(sel flow.Selector, timeout time.Duration) (*core.ElementInfo, error) {
	if timeout <= 0 {
		timeout = d.getFindTimeout()
	}

	// For relative selectors, use page source (position calculation required)
	if sel.HasRelativeSelector() {
		return d.findElementRelative(sel, timeout)
	}

	deadline := time.Now().Add(timeout)

	for {
		// Try clickable-first approach for text-based selectors on Android
		if sel.Text != "" && d.platform != "ios" {
			info, err := d.findElementForTapDirect(sel)
			if err == nil && info != nil {
				return info, nil
			}
		} else {
			// For ID-based or iOS selectors, use standard approach
			info, err := d.findElementDirect(sel)
			if err == nil && info != nil {
				return info, nil
			}
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("element not found: %s", sel.Describe())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// findElementForTapDirect finds element for tap, trying clickable first then fallback to page source.
func (d *Driver) findElementForTapDirect(sel flow.Selector) (*core.ElementInfo, error) {
	escaped := escapeUIAutomatorString(sel.Text)

	// Step 1: Try clickable elements first (fast path)
	// Try textContains with clickable filter
	uiSelector := fmt.Sprintf(`new UiSelector().textContains("%s").clickable(true)`, escaped)
	if elemID, err := d.client.FindElement("-android uiautomator", uiSelector); err == nil && elemID != "" {
		return d.getElementInfo(elemID)
	}

	// Try descriptionContains with clickable filter
	uiSelector = fmt.Sprintf(`new UiSelector().descriptionContains("%s").clickable(true)`, escaped)
	if elemID, err := d.client.FindElement("-android uiautomator", uiSelector); err == nil && elemID != "" {
		return d.getElementInfo(elemID)
	}

	// Step 2: Check if text exists at all (without clickable filter)
	uiSelector = fmt.Sprintf(`new UiSelector().textContains("%s")`, escaped)
	_, textExistsErr := d.client.FindElement("-android uiautomator", uiSelector)

	if textExistsErr != nil {
		// Also try description
		uiSelector = fmt.Sprintf(`new UiSelector().descriptionContains("%s")`, escaped)
		_, textExistsErr = d.client.FindElement("-android uiautomator", uiSelector)
	}

	if textExistsErr != nil {
		// Text not found via UiAutomator - try page source as fallback
		// (handles hint text, content-desc, etc. that UiAutomator misses)
		info, err := d.findElementByPageSource(sel)
		if err == nil {
			return info, nil
		}
		// Still not found - return error to trigger retry
		return nil, fmt.Errorf("element with text '%s' not found", sel.Text)
	}

	// Step 3: Text exists but not clickable → use page source with parent lookup
	return d.findElementByPageSource(sel)
}

// findElementRelative handles relative selectors (below, above, etc.)
// Deprecated: Use findElementRelativeWithContext for new code.
func (d *Driver) findElementRelative(sel flow.Selector, timeout time.Duration) (*core.ElementInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return d.findElementRelativeWithContext(ctx, sel)
}

// findElementOnce finds an element with a single attempt (no polling).
// Used for quick checks like waitUntil where we poll externally.
func (d *Driver) findElementOnce(sel flow.Selector) (*core.ElementInfo, error) {
	if sel.HasRelativeSelector() {
		return d.findElementRelativeOnce(sel)
	}
	return d.findElementDirect(sel)
}

// findElementRelativeWithContext handles relative selectors with context deadline.
func (d *Driver) findElementRelativeWithContext(ctx context.Context, sel flow.Selector) (*core.ElementInfo, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("element not found with relative selector")
		default:
			source, err := d.client.Source()
			if err != nil {
				continue // Retry on source fetch error
			}

			elements, platform, err := ParsePageSource(source)
			if err != nil {
				continue // Retry on parse error
			}
			d.platform = platform

			info, err := d.findElementRelativeWithElements(sel, elements, platform)
			if err == nil && info != nil {
				return info, nil
			}
			// HTTP round-trip is natural rate limit, no sleep needed
		}
	}
}

// findElementRelativeOnce performs a single attempt to find element with relative selector.
func (d *Driver) findElementRelativeOnce(sel flow.Selector) (*core.ElementInfo, error) {
	source, err := d.client.Source()
	if err != nil {
		return nil, err
	}

	elements, platform, err := ParsePageSource(source)
	if err != nil {
		return nil, err
	}
	d.platform = platform

	return d.findElementRelativeWithElements(sel, elements, platform)
}

func (d *Driver) findElementRelativeWithElements(sel flow.Selector, allElements []*ParsedElement, platform string) (*core.ElementInfo, error) {
	// Build base selector (without relative parts)
	baseSel := flow.Selector{
		Text:      sel.Text,
		ID:        sel.ID,
		Width:     sel.Width,
		Height:    sel.Height,
		Tolerance: sel.Tolerance,
		Enabled:   sel.Enabled,
		Selected:  sel.Selected,
		Focused:   sel.Focused,
		Checked:   sel.Checked,
	}

	// Get candidates
	var candidates []*ParsedElement
	if baseSel.Text != "" || baseSel.ID != "" || baseSel.Width > 0 || baseSel.Height > 0 {
		candidates = FilterBySelector(allElements, baseSel, platform)
	} else {
		candidates = allElements
	}

	// Get anchor and filter type
	anchorSelector, filterType := getRelativeFilter(sel)

	// Find anchors
	var anchors []*ParsedElement
	if anchorSelector != nil {
		// Check if anchor itself has relative selector
		_, anchorFilterType := getRelativeFilter(*anchorSelector)
		if anchorFilterType != filterNone {
			// Recursive resolution
			anchorInfo, err := d.findElementRelativeWithElements(*anchorSelector, allElements, platform)
			if err == nil && anchorInfo != nil {
				anchors = []*ParsedElement{{
					Bounds:    anchorInfo.Bounds,
					Enabled:   anchorInfo.Enabled,
					Displayed: anchorInfo.Visible,
				}}
			}
		} else {
			anchors = FilterBySelector(allElements, *anchorSelector, platform)
		}
	}

	// Apply relative filter
	if len(anchors) > 0 {
		var matched []*ParsedElement
		for _, anchor := range anchors {
			filtered := applyRelativeFilter(candidates, anchor, filterType)
			if len(filtered) > 0 {
				matched = filtered
				break
			}
		}
		candidates = matched
	} else if anchorSelector != nil {
		return nil, fmt.Errorf("anchor element not found")
	}

	// Apply containsDescendants
	if len(sel.ContainsDescendants) > 0 {
		candidates = FilterContainsDescendants(candidates, allElements, sel.ContainsDescendants, platform)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no elements match relative criteria")
	}

	// Prioritize and select
	candidates = SortClickableFirst(candidates)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates after sorting")
	}

	var selected *ParsedElement
	if sel.Index != "" {
		idx := 0
		if i, err := strconv.Atoi(sel.Index); err == nil {
			if i < 0 {
				i = len(candidates) + i
			}
			if i >= 0 && i < len(candidates) {
				idx = i
			}
		}
		selected = candidates[idx]
	} else {
		selected = DeepestMatchingElement(candidates)
	}

	if selected == nil {
		return nil, fmt.Errorf("no element selected")
	}

	// If element isn't clickable, try to find a clickable parent
	// This handles React Native pattern where text nodes aren't clickable but containers are
	clickableElem := GetClickableElement(selected)

	return elementToInfoWithClickable(selected, clickableElem, platform), nil
}

// Filter types
type filterType int

const (
	filterNone filterType = iota
	filterBelow
	filterAbove
	filterLeftOf
	filterRightOf
	filterChildOf
	filterContainsChild
	filterInsideOf
)

func getRelativeFilter(sel flow.Selector) (*flow.Selector, filterType) {
	if sel.Below != nil {
		return sel.Below, filterBelow
	}
	if sel.Above != nil {
		return sel.Above, filterAbove
	}
	if sel.LeftOf != nil {
		return sel.LeftOf, filterLeftOf
	}
	if sel.RightOf != nil {
		return sel.RightOf, filterRightOf
	}
	if sel.ChildOf != nil {
		return sel.ChildOf, filterChildOf
	}
	if sel.ContainsChild != nil {
		return sel.ContainsChild, filterContainsChild
	}
	if sel.InsideOf != nil {
		return sel.InsideOf, filterInsideOf
	}
	return nil, filterNone
}

func applyRelativeFilter(candidates []*ParsedElement, anchor *ParsedElement, ft filterType) []*ParsedElement {
	switch ft {
	case filterBelow:
		return FilterBelow(candidates, anchor)
	case filterAbove:
		return FilterAbove(candidates, anchor)
	case filterLeftOf:
		return FilterLeftOf(candidates, anchor)
	case filterRightOf:
		return FilterRightOf(candidates, anchor)
	case filterChildOf:
		return FilterChildOf(candidates, anchor)
	case filterContainsChild:
		return FilterContainsChild(candidates, anchor)
	case filterInsideOf:
		return FilterInsideOf(candidates, anchor)
	default:
		return candidates
	}
}

func (d *Driver) getElementInfo(elementID string) (*core.ElementInfo, error) {
	x, y, w, h, err := d.client.GetElementRect(elementID)
	if err != nil {
		return nil, err
	}

	text, _ := d.client.GetElementText(elementID)
	displayed, _ := d.client.IsElementDisplayed(elementID)
	enabled, _ := d.client.IsElementEnabled(elementID)

	// Get accessibility description - important for elements found via descriptionMatches
	// iOS uses "label" (accessibilityLabel), Android uses "content-desc"
	var accessibilityDesc string
	if d.platform == "ios" {
		accessibilityDesc, _ = d.client.GetElementAttribute(elementID, "label")
	} else {
		accessibilityDesc, _ = d.client.GetElementAttribute(elementID, "content-desc")
	}

	return &core.ElementInfo{
		ID:                 elementID,
		Text:               text,
		AccessibilityLabel: accessibilityDesc,
		Bounds:             core.Bounds{X: x, Y: y, Width: w, Height: h},
		Visible:            displayed,
		Enabled:            enabled,
	}, nil
}

func elementToInfo(elem *ParsedElement, platform string) *core.ElementInfo {
	info := &core.ElementInfo{
		Bounds:  elem.Bounds,
		Enabled: elem.Enabled,
		Visible: elem.Displayed,
	}

	if platform == "ios" {
		info.Text = elem.Label
		if info.Text == "" {
			info.Text = elem.Name
		}
		info.Class = elem.Type
	} else {
		info.Text = elem.Text
		if info.Text == "" {
			info.Text = elem.ContentDesc
		}
		info.ID = elem.ResourceID
		info.Class = elem.ClassName
	}

	return info
}

// elementToInfoWithClickable creates ElementInfo using bounds from clickable element.
// This allows tapping on the clickable parent while preserving the matched element's text.
func elementToInfoWithClickable(matched, clickable *ParsedElement, platform string) *core.ElementInfo {
	if matched == nil || clickable == nil {
		return nil
	}
	info := &core.ElementInfo{
		Bounds:  clickable.Bounds, // Use clickable element's bounds for tap
		Enabled: matched.Enabled,
		Visible: matched.Displayed,
	}

	if platform == "ios" {
		info.Text = matched.Label
		if info.Text == "" {
			info.Text = matched.Name
		}
		info.Class = matched.Type
	} else {
		info.Text = matched.Text
		if info.Text == "" {
			info.Text = matched.ContentDesc
		}
		info.ID = matched.ResourceID
		info.Class = matched.ClassName
	}

	return info
}

// Helper functions

func successResult(msg string, elem *core.ElementInfo) *core.CommandResult {
	return &core.CommandResult{
		Success: true,
		Message: msg,
		Element: elem,
	}
}

func errorResult(err error, msg string) *core.CommandResult {
	if msg == "" && err != nil {
		msg = err.Error()
	}
	return &core.CommandResult{
		Success: false,
		Error:   err,
		Message: msg,
	}
}
