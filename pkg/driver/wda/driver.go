package wda

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Driver implements core.Driver using WebDriverAgent for iOS.
type Driver struct {
	client *Client
	info   *core.PlatformInfo
	udid   string // Device UDID for simctl commands

	// Timeouts (0 = use defaults)
	findTimeout         int // ms, for required elements
	optionalFindTimeout int // ms, for optional elements
}

// NewDriver creates a new WDA driver.
func NewDriver(client *Client, info *core.PlatformInfo, udid string) *Driver {
	return &Driver{
		client: client,
		info:   info,
		udid:   udid,
	}
}

// SetFindTimeout sets the timeout for finding required elements.
func (d *Driver) SetFindTimeout(ms int) {
	d.findTimeout = ms
}

// SetOptionalFindTimeout sets the timeout for finding optional elements.
func (d *Driver) SetOptionalFindTimeout(ms int) {
	d.optionalFindTimeout = ms
}

// SetWaitForIdleTimeout sets the wait for idle timeout.
// Note: This is a no-op for iOS/WDA as idle timeout is not applicable.
func (d *Driver) SetWaitForIdleTimeout(ms int) error {
	// iOS/WDA does not support waitForIdleTimeout
	return nil
}

// Element finding timeouts (milliseconds).
const (
	DefaultFindTimeout  = 17000 // 17 seconds for required elements
	OptionalFindTimeout = 7000  // 7 seconds for optional elements
	QuickFindTimeout    = 1000  // 1 second for quick checks
)

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

	// Wait commands
	case *flow.WaitUntilStep:
		result = d.waitUntil(s)
	case *flow.WaitForAnimationToEndStep:
		result = d.waitForAnimationToEnd(s)

	// Media
	case *flow.TakeScreenshotStep:
		result = d.takeScreenshot(s)

	// Permissions
	case *flow.SetPermissionsStep:
		result = d.setPermissions(s)

	default:
		result = &core.CommandResult{
			Success: false,
			Error:   fmt.Errorf("unknown step type: %T", step),
			Message: fmt.Sprintf("Step type '%T' is not supported on iOS", step),
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

	return state
}

// GetPlatformInfo returns device/platform information.
func (d *Driver) GetPlatformInfo() *core.PlatformInfo {
	return d.info
}

// findElement finds an element using a selector with polling.
func (d *Driver) findElement(sel flow.Selector, optional bool, stepTimeoutMs int) (*core.ElementInfo, error) {
	timeout := d.calculateTimeout(optional, stepTimeoutMs)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return d.findElementWithContext(ctx, sel)
}

// findElementWithContext finds an element using context for deadline management.
func (d *Driver) findElementWithContext(ctx context.Context, sel flow.Selector) (*core.ElementInfo, error) {
	// Handle relative selectors via page source
	if sel.HasRelativeSelector() {
		return d.findElementRelativeWithContext(ctx, sel)
	}

	// All other selectors - try WDA strategies with page source fallback
	var lastErr error

	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("%s: %w", ctx.Err(), lastErr)
			}
			return nil, fmt.Errorf("element '%s' not found: %w", sel.Describe(), ctx.Err())
		default:
			// Try WDA strategies first
			if info, err := d.findElementByWDA(sel); err == nil {
				return info, nil
			} else {
				lastErr = err
			}

			// Fallback to page source parsing
			if info, err := d.findElementByPageSourceOnce(sel); err == nil {
				return info, nil
			} else {
				lastErr = err
			}
			// HTTP round-trip is natural rate limit, no sleep needed
		}
	}
}

// findElementForTap finds an element using a strategy optimized for tap actions.
// For text selectors, it tries interactive element types first (TextField, SecureTextField, Button),
// then falls back to generic text matching with clickable parent lookup via page source.
func (d *Driver) findElementForTap(sel flow.Selector, optional bool, stepTimeoutMs int) (*core.ElementInfo, error) {
	// For relative selectors, use page source which handles them correctly
	if sel.HasRelativeSelector() {
		timeout := d.calculateTimeout(optional, stepTimeoutMs)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return d.findElementRelativeWithContext(ctx, sel)
	}

	// For ID-based selectors, use standard findElement (IDs are usually unique)
	if sel.ID != "" {
		return d.findElement(sel, optional, stepTimeoutMs)
	}

	// For text-based selectors, use smart fallback strategy
	if sel.Text != "" {
		timeout := d.calculateTimeout(optional, stepTimeoutMs)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return d.findElementForTapWithContext(ctx, sel)
	}

	// For other selectors, use standard approach
	return d.findElement(sel, optional, stepTimeoutMs)
}

// findElementForTapWithContext implements the smart tap element finding strategy.
// Tries interactive WDA queries first (TextField, SecureTextField, Button), then falls back
// to generic predicate to check if text exists, and finally page source with clickable parent lookup.
func (d *Driver) findElementForTapWithContext(ctx context.Context, sel flow.Selector) (*core.ElementInfo, error) {
	stateFilter := buildStateFilter(sel)
	var lastErr error

	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("%s: %w", ctx.Err(), lastErr)
			}
			return nil, fmt.Errorf("element '%s' not found: %w", sel.Describe(), ctx.Err())
		default:
			// Step 1: Try interactive element types first (TextField, SecureTextField, Button)
			if info, err := d.findInteractiveElementByWDA(sel, stateFilter); err == nil {
				return info, nil
			}

			// Step 2a: Try exact-match predicate first.
			// This prevents "Password" from matching "Forgot Password?" etc.
			exactPredicate := fmt.Sprintf("(label == '%s' OR name == '%s' OR value == '%s')%s",
				sel.Text, sel.Text, sel.Text, stateFilter)
			exactElemID, _ := d.client.FindElement("predicate string", exactPredicate)

			// Step 2b: Check if text exists via substring WDA predicate
			predicateBase := fmt.Sprintf("label CONTAINS[c] '%s' OR name CONTAINS[c] '%s' OR value CONTAINS[c] '%s'",
				sel.Text, sel.Text, sel.Text)
			predicate := "(" + predicateBase + ")" + stateFilter
			containsElemID, textExistsErr := d.client.FindElement("predicate string", predicate)

			// Prefer exact match element for fallback; use contains match otherwise
			fallbackElemID := exactElemID
			if fallbackElemID == "" {
				fallbackElemID = containsElemID
			}

			if textExistsErr != nil && exactElemID == "" {
				// Text not found via WDA at all - try page source as fallback
				if info, err := d.findElementByPageSourceOnce(sel); err == nil {
					return info, nil
				}
				// Still not found - keep polling
				lastErr = textExistsErr
				continue
			}

			// Step 3: Text exists but not in an interactive element → page source with parent lookup
			if info, err := d.findElementByPageSourceOnce(sel); err == nil {
				return info, nil
			}

			// Step 4: Page source failed (e.g. quiescence error) — use the best predicate element.
			// Exact match is preferred to avoid "Password" hitting "Forgot Password?" etc.
			if fallbackElemID != "" {
				if info, err := d.getElementInfo(fallbackElemID); err == nil {
					return info, nil
				} else {
					lastErr = err
				}
			}
		}
	}
}

// findInteractiveElementByWDA tries WDA queries for interactive element types only.
func (d *Driver) findInteractiveElementByWDA(sel flow.Selector, stateFilter string) (*core.ElementInfo, error) {
	// Try class chain queries first (support placeholderValue attribute)
	textFieldChain := fmt.Sprintf("**/XCUIElementTypeTextField[`(label CONTAINS[c] '%s' OR value CONTAINS[c] '%s' OR placeholderValue CONTAINS[c] '%s')%s`]", sel.Text, sel.Text, sel.Text, stateFilter)
	if elemID, err := d.client.FindElement("class chain", textFieldChain); err == nil && elemID != "" {
		return d.getElementInfo(elemID)
	}

	secureFieldChain := fmt.Sprintf("**/XCUIElementTypeSecureTextField[`(label CONTAINS[c] '%s' OR value CONTAINS[c] '%s' OR placeholderValue CONTAINS[c] '%s')%s`]", sel.Text, sel.Text, sel.Text, stateFilter)
	if elemID, err := d.client.FindElement("class chain", secureFieldChain); err == nil && elemID != "" {
		return d.getElementInfo(elemID)
	}

	// Button queries use exact match (==[c]) to avoid "Password" matching "Forgot Password?" etc.
	buttonChain := fmt.Sprintf("**/XCUIElementTypeButton[`(label ==[c] '%s' OR name ==[c] '%s')%s`]", sel.Text, sel.Text, stateFilter)
	if elemID, err := d.client.FindElement("class chain", buttonChain); err == nil && elemID != "" {
		return d.getElementInfo(elemID)
	}

	// Fallback: try type-filtered predicate queries.
	// Class chain queries can fail due to quiescence while predicate queries may succeed.
	// Note: placeholderValue is not reliably available in predicate queries, so we use label/value only.
	textMatchCond := fmt.Sprintf("(label CONTAINS[c] '%s' OR value CONTAINS[c] '%s')", sel.Text, sel.Text)
	for _, elemType := range []string{"XCUIElementTypeTextField", "XCUIElementTypeSecureTextField", "XCUIElementTypeSearchField"} {
		pred := fmt.Sprintf("type == '%s' AND %s", elemType, textMatchCond)
		if elemID, err := d.client.FindElement("predicate string", pred); err == nil && elemID != "" {
			return d.getElementInfo(elemID)
		}
	}
	buttonPred := fmt.Sprintf("type == 'XCUIElementTypeButton' AND (label ==[c] '%s' OR name ==[c] '%s')", sel.Text, sel.Text)
	if elemID, err := d.client.FindElement("predicate string", buttonPred); err == nil && elemID != "" {
		return d.getElementInfo(elemID)
	}

	return nil, fmt.Errorf("no interactive element found via WDA")
}

// calculateTimeout returns the appropriate timeout duration.
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
func (d *Driver) findElementOnce(sel flow.Selector) (*core.ElementInfo, error) {
	if sel.HasRelativeSelector() {
		return d.findElementRelativeOnce(sel)
	}

	if sel.Width > 0 || sel.Height > 0 {
		return d.findElementByPageSourceOnce(sel)
	}

	// Single attempt with WDA
	if info, err := d.findElementByWDA(sel); err == nil {
		return info, nil
	}

	return d.findElementByPageSourceOnce(sel)
}

// findElementQuick finds an element without polling (single attempt).
// Deprecated: Use findElementOnce instead.
func (d *Driver) findElementQuick(sel flow.Selector, timeoutMs int) (*core.ElementInfo, error) {
	return d.findElementOnce(sel)
}

// buildStateFilter builds WDA predicate conditions for state filters.
// Returns empty string if no state filters are set.
func buildStateFilter(sel flow.Selector) string {
	var conditions []string
	if sel.Enabled != nil {
		if *sel.Enabled {
			conditions = append(conditions, "enabled == true")
		} else {
			conditions = append(conditions, "enabled == false")
		}
	}
	if sel.Selected != nil {
		if *sel.Selected {
			conditions = append(conditions, "selected == true")
		} else {
			conditions = append(conditions, "selected == false")
		}
	}
	if sel.Focused != nil {
		if *sel.Focused {
			conditions = append(conditions, "hasFocus == true")
		} else {
			conditions = append(conditions, "hasFocus == false")
		}
	}
	if len(conditions) == 0 {
		return ""
	}
	return " AND " + strings.Join(conditions, " AND ")
}

// findElementByWDA attempts to find an element using WDA strategies (single attempt).
func (d *Driver) findElementByWDA(sel flow.Selector) (*core.ElementInfo, error) {
	stateFilter := buildStateFilter(sel)

	// Try class chain for accessibility ID
	if sel.ID != "" {
		query := fmt.Sprintf("**/XCUIElementTypeAny[`name CONTAINS '%s'%s`]", sel.ID, stateFilter)
		elemID, err := d.client.FindElement("class chain", query)
		if err == nil && elemID != "" {
			return d.getElementInfo(elemID)
		}
	}

	if sel.Text != "" {
		// Try TextField with matching placeholder/value
		textFieldChain := fmt.Sprintf("**/XCUIElementTypeTextField[`(label CONTAINS[c] '%s' OR value CONTAINS[c] '%s' OR placeholderValue CONTAINS[c] '%s')%s`]", sel.Text, sel.Text, sel.Text, stateFilter)
		if elemID, err := d.client.FindElement("class chain", textFieldChain); err == nil && elemID != "" {
			return d.getElementInfo(elemID)
		}

		// Try SecureTextField
		secureFieldChain := fmt.Sprintf("**/XCUIElementTypeSecureTextField[`(label CONTAINS[c] '%s' OR value CONTAINS[c] '%s' OR placeholderValue CONTAINS[c] '%s')%s`]", sel.Text, sel.Text, sel.Text, stateFilter)
		if elemID, err := d.client.FindElement("class chain", secureFieldChain); err == nil && elemID != "" {
			return d.getElementInfo(elemID)
		}

		// Try Button
		buttonChain := fmt.Sprintf("**/XCUIElementTypeButton[`(label CONTAINS[c] '%s' OR name CONTAINS[c] '%s')%s`]", sel.Text, sel.Text, stateFilter)
		if elemID, err := d.client.FindElement("class chain", buttonChain); err == nil && elemID != "" {
			return d.getElementInfo(elemID)
		}

		// Try any element with matching text
		predicateBase := fmt.Sprintf("label CONTAINS[c] '%s' OR name CONTAINS[c] '%s' OR value CONTAINS[c] '%s'",
			sel.Text, sel.Text, sel.Text)
		predicate := "(" + predicateBase + ")" + stateFilter
		if elemID, err := d.client.FindElement("predicate string", predicate); err == nil && elemID != "" {
			return d.getElementInfo(elemID)
		}
	}

	return nil, fmt.Errorf("element not found via WDA")
}

// getElementInfo gets element info from WDA element ID.
func (d *Driver) getElementInfo(elemID string) (*core.ElementInfo, error) {
	info := &core.ElementInfo{
		ID: elemID,
	}

	if text, err := d.client.ElementText(elemID); err == nil {
		info.Text = text
	}

	if x, y, w, h, err := d.client.ElementRect(elemID); err == nil {
		info.Bounds = core.Bounds{X: x, Y: y, Width: w, Height: h}
	}

	if displayed, err := d.client.ElementDisplayed(elemID); err == nil {
		info.Visible = displayed
	}

	info.Enabled = true // WDA doesn't have separate enabled check

	return info, nil
}

// findElementRelativeWithContext handles relative selectors with context-based timeout.
func (d *Driver) findElementRelativeWithContext(ctx context.Context, sel flow.Selector) (*core.ElementInfo, error) {
	var lastErr error

	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("%s: %w", ctx.Err(), lastErr)
			}
			return nil, fmt.Errorf("element '%s' not found: %w", sel.Describe(), ctx.Err())
		default:
			info, err := d.findElementRelativeOnce(sel)
			if err == nil {
				return info, nil
			}
			lastErr = err
			// HTTP round-trip is natural rate limit, no sleep needed
		}
	}
}

// findElementRelativeOnce performs a single attempt to find element with relative selector.
func (d *Driver) findElementRelativeOnce(sel flow.Selector) (*core.ElementInfo, error) {
	pageSource, err := d.client.Source()
	if err != nil {
		return nil, fmt.Errorf("failed to get page source: %w", err)
	}

	allElements, err := ParsePageSource(pageSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse page source: %w", err)
	}

	return d.resolveRelativeSelector(sel, allElements)
}

// findElementRelative handles relative selectors using page source with polling.
// Deprecated: Use findElementRelativeWithContext for new code.
func (d *Driver) findElementRelative(sel flow.Selector, timeoutMs int) (*core.ElementInfo, error) {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return d.findElementRelativeWithContext(ctx, sel)
}

// resolveRelativeSelector resolves a relative selector against parsed elements.
func (d *Driver) resolveRelativeSelector(sel flow.Selector, allElements []*ParsedElement) (*core.ElementInfo, error) {
	// Build base selector
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
		candidates = FilterBySelector(allElements, baseSel)
	} else {
		candidates = allElements
	}

	// Apply relative filters
	anchorSelector, filterType := getRelativeFilter(sel)
	if anchorSelector != nil {
		anchors := FilterBySelector(allElements, *anchorSelector)
		if len(anchors) == 0 {
			return nil, fmt.Errorf("anchor element not found")
		}

		var matchingCandidates []*ParsedElement
		for _, anchor := range anchors {
			filtered := applyRelativeFilter(candidates, anchor, filterType)
			if len(filtered) > 0 {
				matchingCandidates = filtered
				break
			}
		}
		candidates = matchingCandidates
	}

	// Apply containsDescendants filter
	if len(sel.ContainsDescendants) > 0 {
		candidates = FilterContainsDescendants(candidates, allElements, sel.ContainsDescendants)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no elements match selector")
	}

	// Prioritize clickable/interactive elements
	candidates = SortClickableFirst(candidates)

	// Select element
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

	return &core.ElementInfo{
		Text:    selected.Label,
		Bounds:  selected.Bounds,
		Enabled: selected.Enabled,
		Visible: selected.Displayed,
	}, nil
}

// findElementByPageSourceOnce performs a single page source search.
func (d *Driver) findElementByPageSourceOnce(sel flow.Selector) (*core.ElementInfo, error) {
	pageSource, err := d.client.Source()
	if err != nil {
		return nil, err
	}

	allElements, err := ParsePageSource(pageSource)
	if err != nil {
		return nil, err
	}

	candidates := FilterBySelector(allElements, sel)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no elements match selector")
	}

	// Prioritize clickable/interactive elements
	candidates = SortClickableFirst(candidates)

	selected := DeepestMatchingElement(candidates)
	if selected == nil {
		selected = candidates[0]
	}

	// If element isn't a clickable type, try to find a clickable parent
	// This handles patterns where text labels aren't interactive but their containers are
	clickableElem := GetClickableElement(selected)

	return &core.ElementInfo{
		Text:    selected.Label,
		Bounds:  clickableElem.Bounds,
		Enabled: selected.Enabled,
		Visible: selected.Displayed,
	}, nil
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

// applyRelativeFilter applies the appropriate position filter
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
