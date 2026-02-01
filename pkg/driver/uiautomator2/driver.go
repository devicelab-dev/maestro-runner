// Package uiautomator2 provides Android automation driver using UIAutomator2 server.
package uiautomator2

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// ShellExecutor runs shell commands on a device.
// Implemented by device.AndroidDevice.
type ShellExecutor interface {
	Shell(cmd string) (string, error)
}

// UIA2Client defines the interface for UIAutomator2 client operations.
// Implemented by uiautomator2.Client. Allows mocking in tests.
type UIA2Client interface {
	// Element finding
	FindElement(strategy, selector string) (*uiautomator2.Element, error)
	ActiveElement() (*uiautomator2.Element, error)

	// Timeouts
	SetImplicitWait(timeout time.Duration) error

	// Gestures
	Click(x, y int) error
	DoubleClick(x, y int) error
	DoubleClickElement(elementID string) error
	LongClick(x, y, durationMs int) error
	LongClickElement(elementID string, durationMs int) error
	ScrollInArea(area uiautomator2.RectModel, direction string, percent float64, speed int) error
	SwipeInArea(area uiautomator2.RectModel, direction string, percent float64, speed int) error

	// Navigation
	Back() error
	HideKeyboard() error
	PressKeyCode(keyCode int) error

	// Device state
	Screenshot() ([]byte, error)
	Source() (string, error)
	GetOrientation() (string, error)
	SetOrientation(orientation string) error
	GetClipboard() (string, error)
	SetClipboard(text string) error
	GetDeviceInfo() (*uiautomator2.DeviceInfo, error)

	// Settings
	SetAppiumSettings(settings map[string]interface{}) error
}

// Driver implements core.Driver using UIAutomator2.
type Driver struct {
	client UIA2Client
	info   *core.PlatformInfo
	device ShellExecutor // for ADB commands (launchApp, stopApp, clearState)

	// Timeouts (0 = use defaults)
	findTimeout         int // ms, for required elements
	optionalFindTimeout int // ms, for optional elements
}

// New creates a new UIAutomator2 driver.
func New(client UIA2Client, info *core.PlatformInfo, device ShellExecutor) *Driver {
	return &Driver{
		client: client,
		info:   info,
		device: device,
	}
}

// SetFindTimeout sets the timeout for finding required elements.
// Useful for testing with shorter timeouts.
func (d *Driver) SetFindTimeout(ms int) {
	d.findTimeout = ms
}

// SetOptionalFindTimeout sets the timeout for finding optional elements.
func (d *Driver) SetOptionalFindTimeout(ms int) {
	d.optionalFindTimeout = ms
}

// SetWaitForIdleTimeout sets the wait for idle timeout.
// 0 = disabled, >0 = wait up to N ms for device to be idle.
func (d *Driver) SetWaitForIdleTimeout(ms int) error {
	return d.client.SetAppiumSettings(map[string]interface{}{
		"waitForIdleTimeout": ms,
	})
}

// Execute runs a single step and returns the result.
func (d *Driver) Execute(step flow.Step) *core.CommandResult {
	start := time.Now()

	var result *core.CommandResult
	switch s := step.(type) {
	// Tap commands
	case *flow.TapOnStep:
		result = d.tapOn(s)
	case *flow.DoubleTapOnStep:
		result = d.doubleTapOn(s)
	case *flow.LongPressOnStep:
		result = d.longPressOn(s)
	case *flow.TapOnPointStep:
		result = d.tapOnPoint(s)

	// Assert commands
	case *flow.AssertVisibleStep:
		result = d.assertVisible(s)
	case *flow.AssertNotVisibleStep:
		result = d.assertNotVisible(s)

	// Input commands
	case *flow.InputTextStep:
		result = d.inputText(s)
	case *flow.EraseTextStep:
		result = d.eraseText(s)
	case *flow.HideKeyboardStep:
		result = d.hideKeyboard(s)
	case *flow.InputRandomStep:
		result = d.inputRandom(s)

	// Scroll/Swipe commands
	case *flow.ScrollStep:
		result = d.scroll(s)
	case *flow.ScrollUntilVisibleStep:
		result = d.scrollUntilVisible(s)
	case *flow.SwipeStep:
		result = d.swipe(s)

	// Navigation commands
	case *flow.BackStep:
		result = d.back(s)
	case *flow.PressKeyStep:
		result = d.pressKey(s)

	// App lifecycle
	case *flow.LaunchAppStep:
		result = d.launchApp(s)
	case *flow.StopAppStep:
		result = d.stopApp(s)
	case *flow.KillAppStep:
		result = d.killApp(s)
	case *flow.ClearStateStep:
		result = d.clearState(s)

	// Clipboard
	case *flow.CopyTextFromStep:
		result = d.copyTextFrom(s)
	case *flow.PasteTextStep:
		result = d.pasteText(s)
	case *flow.SetClipboardStep:
		result = d.setClipboard(s)

	// Device control
	case *flow.SetOrientationStep:
		result = d.setOrientation(s)
	case *flow.OpenLinkStep:
		result = d.openLink(s)
	case *flow.OpenBrowserStep:
		result = d.openBrowser(s)
	case *flow.SetLocationStep:
		result = d.setLocation(s)
	case *flow.SetAirplaneModeStep:
		result = d.setAirplaneMode(s)
	case *flow.ToggleAirplaneModeStep:
		result = d.toggleAirplaneMode(s)
	case *flow.TravelStep:
		result = d.travel(s)

	// Wait commands
	case *flow.WaitUntilStep:
		result = d.waitUntil(s)
	case *flow.WaitForAnimationToEndStep:
		result = d.waitForAnimationToEnd(s)

	// Media
	case *flow.TakeScreenshotStep:
		result = d.takeScreenshot(s)
	case *flow.StartRecordingStep:
		result = d.startRecording(s)
	case *flow.StopRecordingStep:
		result = d.stopRecording(s)
	case *flow.AddMediaStep:
		result = d.addMedia(s)

	default:
		result = &core.CommandResult{
			Success: false,
			Error:   fmt.Errorf("unknown step type: %T", step),
			Message: fmt.Sprintf("Step type '%T' is not supported", step),
		}
	}

	result.Duration = time.Since(start)
	return result
}

// Screenshot captures the current screen as PNG.
func (d *Driver) Screenshot() ([]byte, error) {
	return d.client.Screenshot()
}

// Hierarchy captures the UI hierarchy as XML.
func (d *Driver) Hierarchy() ([]byte, error) {
	source, err := d.client.Source()
	if err != nil {
		return nil, err
	}
	return []byte(source), nil
}

// GetState returns the current device/app state.
func (d *Driver) GetState() *core.StateSnapshot {
	state := &core.StateSnapshot{}

	if orientation, err := d.client.GetOrientation(); err == nil {
		state.Orientation = strings.ToLower(orientation)
	}

	if clipboard, err := d.client.GetClipboard(); err == nil {
		state.ClipboardText = clipboard
	}

	return state
}

// GetPlatformInfo returns device/platform information.
func (d *Driver) GetPlatformInfo() *core.PlatformInfo {
	return d.info
}

// findElement finds an element using a selector with client-side polling.
// Tries multiple locator strategies in order until one succeeds.
// For relative selectors or regex patterns, uses page source parsing.
// If stepTimeoutMs > 0, uses that; otherwise uses 17s for required, 7s for optional.
// Returns full element info including text and bounds (3 HTTP calls).
func (d *Driver) findElement(sel flow.Selector, optional bool, stepTimeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	return d.findElementWithOptions(sel, optional, stepTimeoutMs, false, false)
}

// findElementFast finds an element with minimal HTTP calls (1 call).
// Use for visibility checks where we only need to know element exists.
func (d *Driver) findElementFast(sel flow.Selector, optional bool, stepTimeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	return d.findElementWithOptions(sel, optional, stepTimeoutMs, false, true)
}

// findElementForTap finds an element for tap commands, prioritizing clickable elements.
// When multiple elements match (e.g., "Login" title and "Login" button), prefers the clickable one.
// Strategy for text-based selectors:
//  1. Try UiAutomator with .clickable(true) - fast if element itself is clickable
//  2. If fails, check if text exists at all
//  3. If text exists but not clickable → page source with clickable parent lookup
//  4. If text doesn't exist → keep polling
//
// This handles React Native pattern where text nodes aren't clickable but parent containers are.
func (d *Driver) findElementForTap(sel flow.Selector, optional bool, stepTimeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	// For relative selectors (below, above, etc.), use page source which handles them correctly
	if sel.HasRelativeSelector() {
		timeout := d.calculateTimeout(optional, stepTimeoutMs)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return d.findElementRelativeWithContext(ctx, sel)
	}

	// For ID-based selectors, use standard UiAutomator approach (IDs are usually unique)
	if sel.ID != "" {
		return d.findElementWithOptions(sel, optional, stepTimeoutMs, true, false)
	}

	// For text-based selectors, use smart fallback strategy
	if sel.Text != "" {
		timeout := d.calculateTimeout(optional, stepTimeoutMs)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		return d.findElementForTapWithContext(ctx, sel)
	}

	// For other selectors, use standard approach
	return d.findElementWithOptions(sel, optional, stepTimeoutMs, true, false)
}

// findElementForTapWithContext implements the smart tap element finding strategy.
// Tries clickable UiAutomator first, falls back to page source if text exists but isn't clickable.
func (d *Driver) findElementForTapWithContext(ctx context.Context, sel flow.Selector) (*uiautomator2.Element, *core.ElementInfo, error) {
	// Build clickable-only strategies
	clickableStrategies, err := buildClickableOnlyStrategies(sel)
	if err != nil {
		return nil, nil, err
	}

	// Build text-exists strategies (without clickable filter)
	textExistsStrategies, err := buildSelectors(sel, 0)
	if err != nil {
		return nil, nil, err
	}

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, nil, fmt.Errorf("%s: %w", ctx.Err(), lastErr)
			}
			return nil, nil, fmt.Errorf("element '%s' not found: %w", sel.Describe(), ctx.Err())
		default:
			// Step 1: Try clickable strategies first (fast path)
			elem, info, err := d.tryFindElement(clickableStrategies)
			if err == nil {
				return elem, info, nil
			}

			// Step 2: Check if text exists at all (via UiAutomator)
			_, _, existsErr := d.tryFindElementFast(textExistsStrategies)
			if existsErr != nil {
				// Text not found via UiAutomator - try page source as fallback
				// (handles hint text, content-desc, etc. that UiAutomator misses)
				_, info, err = d.findElementByPageSourceOnce(sel)
				if err == nil {
					return nil, info, nil
				}
				// Still not found - keep polling
				lastErr = existsErr
				continue
			}

			// Step 3: Text exists but not clickable → use page source with parent lookup
			_, info, err = d.findElementByPageSourceOnce(sel)
			if err == nil {
				return nil, info, nil
			}
			lastErr = err
		}
	}
}

// buildClickableOnlyStrategies builds UiAutomator strategies that only match clickable elements.
func buildClickableOnlyStrategies(sel flow.Selector) ([]LocatorStrategy, error) {
	var strategies []LocatorStrategy
	stateFilters := buildStateFilters(sel)

	if sel.Text != "" {
		if looksLikeRegex(sel.Text) {
			pattern := "(?is)" + escapeUIAutomatorString(sel.Text)
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().textMatches("` + pattern + `").clickable(true)` + stateFilters,
			})
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().descriptionMatches("` + pattern + `").clickable(true)` + stateFilters,
			})
		} else {
			escaped := escapeUIAutomatorString(sel.Text)
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().textContains("` + escaped + `").clickable(true)` + stateFilters,
			})
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().descriptionContains("` + escaped + `").clickable(true)` + stateFilters,
			})
		}
	}

	if len(strategies) == 0 {
		return nil, fmt.Errorf("no text selector specified")
	}

	return strategies, nil
}

// findElementWithOptions is the internal implementation with clickable preference option.
// Set fastMode=true for visibility checks (1 HTTP call), false for full info (3 HTTP calls).
func (d *Driver) findElementWithOptions(sel flow.Selector, optional bool, stepTimeoutMs int, preferClickable bool, fastMode bool) (*uiautomator2.Element, *core.ElementInfo, error) {
	timeout := d.calculateTimeout(optional, stepTimeoutMs)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return d.findElementWithContext(ctx, sel, preferClickable, fastMode)
}

// findElementWithContext finds an element using context for deadline management.
// Single polling loop - context controls the timeout.
// Set fastMode=true for visibility checks (1 HTTP call), false for full info (3 HTTP calls).
func (d *Driver) findElementWithContext(ctx context.Context, sel flow.Selector, preferClickable bool, fastMode bool) (*uiautomator2.Element, *core.ElementInfo, error) {
	// Handle relative selectors via page source (position calculation required)
	if sel.HasRelativeSelector() {
		return d.findElementRelativeWithContext(ctx, sel)
	}

	// Handle size selectors via page source (bounds calculation required)
	if sel.Width > 0 || sel.Height > 0 {
		return d.findElementByPageSourceWithContext(ctx, sel)
	}

	// Build strategies for UiAutomator
	var strategies []LocatorStrategy
	var err error
	if preferClickable {
		strategies, err = buildSelectorsForTap(sel, 0)
	} else {
		strategies, err = buildSelectors(sel, 0)
	}
	if err != nil {
		return nil, nil, err
	}

	// Single polling loop with context deadline
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			// UiAutomator strategies exhausted - try page source ONCE as final fallback
			if sel.Text != "" {
				_, info, err := d.findElementByPageSourceOnce(sel)
				if err == nil {
					return nil, info, nil
				}
			}
			if lastErr != nil {
				return nil, nil, fmt.Errorf("%s: %w", ctx.Err(), lastErr)
			}
			return nil, nil, fmt.Errorf("element '%s' not found: %w", sel.Describe(), ctx.Err())
		default:
			// Try UiAutomator strategies
			var elem *uiautomator2.Element
			var info *core.ElementInfo
			var err error
			if fastMode {
				elem, info, err = d.tryFindElementFast(strategies)
			} else {
				elem, info, err = d.tryFindElement(strategies)
			}
			if err == nil {
				return elem, info, nil
			}
			lastErr = err
			// HTTP round-trip (~100ms) is natural rate limit, no sleep needed
		}
	}
}

// calculateTimeout returns the appropriate timeout duration based on optional flag and step timeout.
func (d *Driver) calculateTimeout(optional bool, stepTimeoutMs int) time.Duration {
	var timeoutMs int
	if stepTimeoutMs > 0 {
		timeoutMs = stepTimeoutMs
	} else if optional {
		timeoutMs = OptionalFindTimeout
		if d.optionalFindTimeout > 0 {
			timeoutMs = d.optionalFindTimeout
		}
	} else {
		timeoutMs = DefaultFindTimeout
		if d.findTimeout > 0 {
			timeoutMs = d.findTimeout
		}
	}
	return time.Duration(timeoutMs) * time.Millisecond
}

// findElementOnce finds an element with a single attempt (no polling).
// Used by waitUntil which has its own polling loop with context.
func (d *Driver) findElementOnce(sel flow.Selector) (*uiautomator2.Element, *core.ElementInfo, error) {
	// Handle relative selectors with single page source fetch
	if sel.HasRelativeSelector() {
		return d.findElementRelativeOnce(sel)
	}

	// Handle size selectors with single page source fetch
	if sel.Width > 0 || sel.Height > 0 {
		return d.findElementByPageSourceOnce(sel)
	}

	strategies, err := buildSelectors(sel, 0)
	if err != nil {
		return nil, nil, err
	}

	// Single attempt with UiAutomator
	elem, info, err := d.tryFindElement(strategies)
	if err == nil {
		return elem, info, nil
	}

	// For text-based selectors, try page source as fallback
	if sel.Text != "" {
		_, info, err := d.findElementByPageSourceOnce(sel)
		if err == nil {
			return nil, info, nil
		}
	}

	return nil, nil, err
}

// findElementQuick finds an element without polling (single attempt).
// Deprecated: Use findElementOnce instead. Kept for backward compatibility.
func (d *Driver) findElementQuick(sel flow.Selector, timeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	return d.findElementOnce(sel)
}

// tryFindElementFast attempts to find element using given strategies (single attempt).
// Returns minimal info - just element ID and visible=true. No extra HTTP calls.
// Use this for visibility checks where we only need to know element exists.
func (d *Driver) tryFindElementFast(strategies []LocatorStrategy) (*uiautomator2.Element, *core.ElementInfo, error) {
	var lastErr error
	for _, s := range strategies {
		elem, err := d.client.FindElement(s.Strategy, s.Value)
		if err != nil {
			lastErr = err
			continue
		}

		// Found element - return minimal info (no extra HTTP calls)
		// UIAutomator2 by default only returns visible elements
		info := &core.ElementInfo{
			ID:      elem.ID(),
			Visible: true,
			Enabled: true,
		}
		return elem, info, nil
	}

	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, fmt.Errorf("element not found")
}

// tryFindElement attempts to find element using given strategies (single attempt).
// Returns full element info including text and bounds (3 HTTP calls total).
// Use tryFindElementFast for visibility checks where details aren't needed.
func (d *Driver) tryFindElement(strategies []LocatorStrategy) (*uiautomator2.Element, *core.ElementInfo, error) {
	elem, info, err := d.tryFindElementFast(strategies)
	if err != nil {
		return nil, nil, err
	}

	// Fetch additional details (2 more HTTP calls)
	if text, err := elem.Text(); err == nil {
		info.Text = text
	}

	if rect, err := elem.Rect(); err == nil {
		info.Bounds = core.Bounds{
			X:      rect.X,
			Y:      rect.Y,
			Width:  rect.Width,
			Height: rect.Height,
		}
	}

	return elem, info, nil
}

// relativeFilterType identifies which relative filter to apply
type relativeFilterType int

const (
	filterNone relativeFilterType = iota
	filterBelow
	filterAbove
	filterLeftOf
	filterRightOf
	filterChildOf
	filterContainsChild
	filterInsideOf
)

// getRelativeFilter returns the anchor selector and filter type from a selector
func getRelativeFilter(sel flow.Selector) (*flow.Selector, relativeFilterType) {
	switch {
	case sel.Below != nil:
		return sel.Below, filterBelow
	case sel.Above != nil:
		return sel.Above, filterAbove
	case sel.LeftOf != nil:
		return sel.LeftOf, filterLeftOf
	case sel.RightOf != nil:
		return sel.RightOf, filterRightOf
	case sel.ChildOf != nil:
		return sel.ChildOf, filterChildOf
	case sel.ContainsChild != nil:
		return sel.ContainsChild, filterContainsChild
	case sel.InsideOf != nil:
		return sel.InsideOf, filterInsideOf
	default:
		return nil, filterNone
	}
}

// applyRelativeFilter applies the appropriate position filter based on filter type
func applyRelativeFilter(candidates []*ParsedElement, anchor *ParsedElement, filterType relativeFilterType) []*ParsedElement {
	switch filterType {
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

// findElementRelativeWithContext handles relative selectors with context-based timeout.
// Uses page source XML parsing to find elements by position with polling controlled by context.
func (d *Driver) findElementRelativeWithContext(ctx context.Context, sel flow.Selector) (*uiautomator2.Element, *core.ElementInfo, error) {
	var lastErr error

	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, nil, fmt.Errorf("%s: %w", ctx.Err(), lastErr)
			}
			return nil, nil, fmt.Errorf("element '%s' not found: %w", sel.Describe(), ctx.Err())
		default:
			info, err := d.resolveRelativeSelector(sel)
			if err == nil {
				return nil, info, nil
			}
			lastErr = err
			// HTTP round-trip is natural rate limit, no sleep needed
		}
	}
}

// findElementRelativeOnce performs a single attempt to find element with relative selector.
// No polling - returns immediately whether found or not.
func (d *Driver) findElementRelativeOnce(sel flow.Selector) (*uiautomator2.Element, *core.ElementInfo, error) {
	info, err := d.resolveRelativeSelector(sel)
	if err != nil {
		return nil, nil, err
	}
	return nil, info, nil
}

// resolveRelativeSelector resolves a relative selector with a single page source fetch.
// This is the core logic extracted from findElementRelative for reuse.
func (d *Driver) resolveRelativeSelector(sel flow.Selector) (*core.ElementInfo, error) {
	// Get anchor selector and filter type
	anchorSelector, filterType := getRelativeFilter(sel)

	// Build base selector for filtering
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

	// Get page source
	pageSource, err := d.client.Source()
	if err != nil {
		return nil, fmt.Errorf("failed to get page source: %w", err)
	}

	// Parse all elements
	allElements, err := ParsePageSource(pageSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse page source: %w", err)
	}

	// Filter by base selector to get target candidates
	var candidates []*ParsedElement
	if baseSel.Text != "" || baseSel.ID != "" || baseSel.Width > 0 || baseSel.Height > 0 {
		candidates = FilterBySelector(allElements, baseSel)
	} else {
		candidates = allElements
	}

	// Find all anchor candidates from page source
	// If anchor selector itself has a relative selector, resolve it recursively
	var anchors []*ParsedElement
	if anchorSelector != nil {
		_, anchorFilterType := getRelativeFilter(*anchorSelector)
		if anchorFilterType != filterNone {
			// Anchor has nested relative selector - resolve recursively
			_, anchorInfo, err := d.findElementRelativeWithElements(*anchorSelector, allElements)
			if err == nil && anchorInfo != nil {
				anchors = []*ParsedElement{{
					Text:      anchorInfo.Text,
					Bounds:    anchorInfo.Bounds,
					Enabled:   anchorInfo.Enabled,
					Displayed: anchorInfo.Visible,
				}}
			}
		} else {
			// Simple anchor - use FilterBySelector
			anchors = FilterBySelector(allElements, *anchorSelector)
		}
	}

	// Try each anchor candidate to find matches
	var matchingCandidates []*ParsedElement
	if len(anchors) > 0 {
		for _, anchor := range anchors {
			filtered := applyRelativeFilter(candidates, anchor, filterType)
			if len(filtered) > 0 {
				matchingCandidates = filtered
				break
			}
		}
		candidates = matchingCandidates
	} else if anchorSelector != nil {
		return nil, fmt.Errorf("anchor element not found")
	}

	// Apply containsDescendants filter
	if len(sel.ContainsDescendants) > 0 {
		candidates = FilterContainsDescendants(candidates, allElements, sel.ContainsDescendants)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no elements match relative criteria")
	}

	// Prioritize clickable elements
	candidates = SortClickableFirst(candidates)

	// Apply index if specified, otherwise use deepest matching element
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

	// If element isn't clickable, try to find a clickable parent
	// This handles React Native pattern where text nodes aren't clickable but containers are
	clickableElem := GetClickableElement(selected)

	return &core.ElementInfo{
		Text:    selected.Text,
		Bounds:  clickableElem.Bounds,
		Enabled: selected.Enabled,
		Visible: selected.Displayed,
	}, nil
}

// findElementRelativeWithElements resolves a relative selector using pre-parsed elements.
// Used for recursive resolution of nested relative selectors without refetching page source.
func (d *Driver) findElementRelativeWithElements(sel flow.Selector, allElements []*ParsedElement) (*uiautomator2.Element, *core.ElementInfo, error) {
	// Get anchor selector and filter type
	anchorSelector, filterType := getRelativeFilter(sel)

	// Build base selector for filtering (without the relative part)
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

	// Filter by base selector to get target candidates
	var candidates []*ParsedElement
	if baseSel.Text != "" || baseSel.ID != "" || baseSel.Width > 0 || baseSel.Height > 0 {
		candidates = FilterBySelector(allElements, baseSel)
	} else {
		candidates = allElements
	}

	// Find anchor elements - recursively resolve if anchor has its own relative selector
	var anchors []*ParsedElement
	if anchorSelector != nil {
		_, anchorFilterType := getRelativeFilter(*anchorSelector)
		if anchorFilterType != filterNone {
			// Anchor has nested relative selector - resolve recursively
			_, anchorInfo, err := d.findElementRelativeWithElements(*anchorSelector, allElements)
			if err == nil && anchorInfo != nil {
				anchors = []*ParsedElement{{
					Text:      anchorInfo.Text,
					Bounds:    anchorInfo.Bounds,
					Enabled:   anchorInfo.Enabled,
					Displayed: anchorInfo.Visible,
				}}
			}
		} else {
			// Simple anchor - use FilterBySelector
			anchors = FilterBySelector(allElements, *anchorSelector)
		}
	}

	// Try each anchor candidate to find matches
	var matchingCandidates []*ParsedElement
	if len(anchors) > 0 {
		for _, anchor := range anchors {
			filtered := applyRelativeFilter(candidates, anchor, filterType)
			if len(filtered) > 0 {
				matchingCandidates = filtered
				break
			}
		}
		candidates = matchingCandidates
	} else if anchorSelector != nil {
		return nil, nil, fmt.Errorf("anchor element not found")
	}

	// Apply containsDescendants filter
	if len(sel.ContainsDescendants) > 0 {
		candidates = FilterContainsDescendants(candidates, allElements, sel.ContainsDescendants)
	}

	if len(candidates) == 0 {
		return nil, nil, fmt.Errorf("no elements match relative criteria")
	}

	// Prioritize clickable elements
	candidates = SortClickableFirst(candidates)

	// Apply index if specified, otherwise use deepest matching element
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

	// If element isn't clickable, try to find a clickable parent
	// This handles React Native pattern where text nodes aren't clickable but containers are
	clickableElem := GetClickableElement(selected)

	info := &core.ElementInfo{
		Text:    selected.Text,
		Bounds:  clickableElem.Bounds,
		Enabled: selected.Enabled,
		Visible: selected.Displayed,
	}

	return nil, info, nil
}

// findElementByPageSourceOnce performs a single page source search without polling.
// Used as a fallback when UiAutomator selectors don't find the element (e.g., hint text).
func (d *Driver) findElementByPageSourceOnce(sel flow.Selector) (*uiautomator2.Element, *core.ElementInfo, error) {
	pageSource, err := d.client.Source()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get page source: %w", err)
	}

	allElements, err := ParsePageSource(pageSource)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse page source: %w", err)
	}

	candidates := FilterBySelector(allElements, sel)
	candidates = SortClickableFirst(candidates)

	if len(candidates) > 0 {
		selected := DeepestMatchingElement(candidates)
		if selected == nil {
			selected = candidates[0]
		}

		// If element isn't clickable, try to find a clickable parent
		// This handles React Native pattern where text nodes aren't clickable but containers are
		clickableElem := GetClickableElement(selected)

		info := &core.ElementInfo{
			Text: selected.Text,
			Bounds: core.Bounds{
				X:      clickableElem.Bounds.X,
				Y:      clickableElem.Bounds.Y,
				Width:  clickableElem.Bounds.Width,
				Height: clickableElem.Bounds.Height,
			},
			Enabled: selected.Enabled,
			Visible: selected.Displayed,
		}
		return nil, info, nil
	}

	return nil, nil, fmt.Errorf("no elements match selector")
}

// findElementByPageSourceWithContext finds an element using page source with context-based timeout.
func (d *Driver) findElementByPageSourceWithContext(ctx context.Context, sel flow.Selector) (*uiautomator2.Element, *core.ElementInfo, error) {
	var lastErr error

	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, nil, fmt.Errorf("%s: %w", ctx.Err(), lastErr)
			}
			return nil, nil, fmt.Errorf("element '%s' not found: %w", sel.Describe(), ctx.Err())
		default:
			info, err := d.findElementByPageSourceOnceInternal(sel)
			if err == nil {
				return nil, info, nil
			}
			lastErr = err
			// HTTP round-trip is natural rate limit, no sleep needed
		}
	}
}

// findElementByPageSourceOnceInternal performs a single page source search.
// Returns ElementInfo on success, error on failure.
func (d *Driver) findElementByPageSourceOnceInternal(sel flow.Selector) (*core.ElementInfo, error) {
	pageSource, err := d.client.Source()
	if err != nil {
		return nil, fmt.Errorf("failed to get page source: %w", err)
	}

	allElements, err := ParsePageSource(pageSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse page source: %w", err)
	}

	candidates := FilterBySelector(allElements, sel)
	candidates = SortClickableFirst(candidates)

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no elements match selector")
	}

	selected := DeepestMatchingElement(candidates)
	if selected == nil {
		selected = candidates[0]
	}

	// If element isn't clickable, try to find a clickable parent
	// This handles React Native pattern where text nodes aren't clickable but containers are
	clickableElem := GetClickableElement(selected)

	return &core.ElementInfo{
		Text: selected.Text,
		Bounds: core.Bounds{
			X:      clickableElem.Bounds.X,
			Y:      clickableElem.Bounds.Y,
			Width:  clickableElem.Bounds.Width,
			Height: clickableElem.Bounds.Height,
		},
		Enabled: selected.Enabled,
		Visible: selected.Displayed,
	}, nil
}

// LocatorStrategy represents a single locator strategy with its value.
type LocatorStrategy struct {
	Strategy string
	Value    string
}

// Element finding timeouts (milliseconds).
// Matches Maestro's defaults for compatibility.
const (
	DefaultFindTimeout  = 17000 // 17 seconds for required elements
	OptionalFindTimeout = 7000  // 7 seconds for optional elements
	QuickFindTimeout    = 1000  // 1 second for quick checks (assertNotVisible, waitUntil)
)

// buildSelectors converts a Maestro Selector to UIAutomator2 locator strategies.
// Returns multiple strategies to try in order (first match wins).
// Mimics Maestro's case-insensitive contains matching behavior.
// Note: Relative selectors are handled separately in findElementRelative.
// Note: Timeout/waiting is handled via polling in findElement, not in selectors.
func buildSelectors(sel flow.Selector, timeoutMs int) ([]LocatorStrategy, error) {
	return buildSelectorsWithOptions(sel, timeoutMs, false)
}

// buildSelectorsForTap builds selectors that prioritize clickable elements.
// Used for tap commands where we want to prefer buttons over labels.
func buildSelectorsForTap(sel flow.Selector, timeoutMs int) ([]LocatorStrategy, error) {
	return buildSelectorsWithOptions(sel, timeoutMs, true)
}

// buildSelectorsWithOptions builds selectors with optional clickable-first prioritization.
func buildSelectorsWithOptions(sel flow.Selector, timeoutMs int, preferClickable bool) ([]LocatorStrategy, error) {
	var strategies []LocatorStrategy
	stateFilters := buildStateFilters(sel)

	// ID-based selector - use resourceIdMatches for partial matching
	if sel.ID != "" {
		escaped := escapeUIAutomator(sel.ID)
		if preferClickable {
			// Try clickable first for tap commands
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().resourceIdMatches(".*` + escaped + `.*").clickable(true)` + stateFilters,
			})
		}
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().resourceIdMatches(".*` + escaped + `.*")` + stateFilters,
		})
	}

	// Text-based selector - use textContains for literal text, textMatches for regex
	if sel.Text != "" {
		if looksLikeRegex(sel.Text) {
			// Use textMatches for regex patterns (case-insensitive)
			pattern := "(?is)" + escapeUIAutomatorString(sel.Text)
			if preferClickable {
				strategies = append(strategies, LocatorStrategy{
					Strategy: uiautomator2.StrategyUIAutomator,
					Value:    `new UiSelector().textMatches("` + pattern + `").clickable(true)` + stateFilters,
				})
				strategies = append(strategies, LocatorStrategy{
					Strategy: uiautomator2.StrategyUIAutomator,
					Value:    `new UiSelector().descriptionMatches("` + pattern + `").clickable(true)` + stateFilters,
				})
			}
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().textMatches("` + pattern + `")` + stateFilters,
			})
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().descriptionMatches("` + pattern + `")` + stateFilters,
			})
		} else {
			// Use textContains for literal text (case-insensitive by default)
			// Escape only quotes for the string value
			escaped := escapeUIAutomatorString(sel.Text)
			if preferClickable {
				strategies = append(strategies, LocatorStrategy{
					Strategy: uiautomator2.StrategyUIAutomator,
					Value:    `new UiSelector().textContains("` + escaped + `").clickable(true)` + stateFilters,
				})
				strategies = append(strategies, LocatorStrategy{
					Strategy: uiautomator2.StrategyUIAutomator,
					Value:    `new UiSelector().descriptionContains("` + escaped + `").clickable(true)` + stateFilters,
				})
			}
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().textContains("` + escaped + `")` + stateFilters,
			})
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().descriptionContains("` + escaped + `")` + stateFilters,
			})
		}
	}

	// CSS selector for web views (no native wait support)
	if sel.CSS != "" {
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyClassName,
			Value:    sel.CSS,
		})
	}

	if len(strategies) == 0 {
		return nil, fmt.Errorf("no selector specified")
	}

	return strategies, nil
}

// looksLikeRegex checks if text contains regex metacharacters that suggest it's a pattern.
// Common patterns: .+, .*, [a-z], ^, $, etc.
// A standalone period (like in "mastodon.social") is NOT treated as regex.
func looksLikeRegex(text string) bool {
	// Check for common regex patterns
	for i := 0; i < len(text); i++ {
		c := text[i]
		// Check if it's escaped
		if i > 0 && text[i-1] == '\\' {
			continue
		}
		switch c {
		case '.':
			// Only treat '.' as regex if followed by a quantifier (*, +, ?)
			// This allows "mastodon.social" to be treated as literal text
			if i+1 < len(text) {
				next := text[i+1]
				if next == '*' || next == '+' || next == '?' {
					return true
				}
			}
		case '*', '+', '?', '[', ']', '{', '}', '|', '(', ')':
			return true
		case '^':
			// ^ at start is common in regex, but at end it's likely literal
			if i == 0 {
				return true
			}
		case '$':
			// $ at end is common in regex (end anchor), but at start it's likely literal (currency)
			if i == len(text)-1 {
				return true
			}
		}
	}
	return false
}

// escapeUIAutomatorString escapes only the double quotes for UiAutomator string.
// Used when the text is already a regex pattern.
func escapeUIAutomatorString(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// buildStateFilters returns UiSelector chain for state filters.
// e.g., ".enabled(true).checked(false)"
func buildStateFilters(sel flow.Selector) string {
	var filters strings.Builder

	if sel.Enabled != nil {
		filters.WriteString(fmt.Sprintf(".enabled(%t)", *sel.Enabled))
	}
	if sel.Selected != nil {
		filters.WriteString(fmt.Sprintf(".selected(%t)", *sel.Selected))
	}
	if sel.Checked != nil {
		filters.WriteString(fmt.Sprintf(".checked(%t)", *sel.Checked))
	}
	if sel.Focused != nil {
		filters.WriteString(fmt.Sprintf(".focused(%t)", *sel.Focused))
	}

	return filters.String()
}

// escapeUIAutomator escapes special characters for UiAutomator selector strings.
func escapeUIAutomator(s string) string {
	var result strings.Builder
	result.Grow(len(s) * 2)

	for _, c := range s {
		switch c {
		case '"':
			result.WriteString(`\"`)
		case '\\':
			result.WriteString(`\\`)
		case '\n':
			result.WriteString(`\n`)
		case '\r':
			result.WriteString(`\r`)
		case '\t':
			result.WriteString(`\t`)
		// Regex special characters
		case '$', '^', '.', '*', '+', '?', '(', ')', '[', ']', '{', '}', '|':
			result.WriteRune('\\')
			result.WriteRune(c)
		default:
			result.WriteRune(c)
		}
	}
	return result.String()
}

// successResult creates a success result.
func successResult(msg string, elem *core.ElementInfo) *core.CommandResult {
	return &core.CommandResult{
		Success: true,
		Message: msg,
		Element: elem,
	}
}

// errorResult creates an error result.
func errorResult(err error, msg string) *core.CommandResult {
	return &core.CommandResult{
		Success: false,
		Error:   err,
		Message: msg,
	}
}
