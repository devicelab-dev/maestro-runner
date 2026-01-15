package uiautomator2

import (
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
	PressKeyCode(keyCode int) error

	// Device state
	Screenshot() ([]byte, error)
	Source() (string, error)
	GetOrientation() (string, error)
	SetOrientation(orientation string) error
	GetClipboard() (string, error)
	SetClipboard(text string) error
	GetDeviceInfo() (*uiautomator2.DeviceInfo, error)
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
func (d *Driver) findElement(sel flow.Selector, optional bool, stepTimeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
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
	timeout := time.Duration(timeoutMs) * time.Millisecond

	// Handle relative selectors via page source (position calculation required)
	if sel.HasRelativeSelector() {
		return d.findElementRelative(sel, int(timeout.Milliseconds()))
	}

	// Handle size selectors via page source (bounds calculation required)
	if sel.Width > 0 || sel.Height > 0 {
		return d.findElementByPageSource(sel, int(timeout.Milliseconds()))
	}

	// All other selectors (text, id, state filters) use UiAutomator directly
	// including regex patterns via textMatches()/descriptionMatches()
	strategies, err := buildSelectors(sel, int(timeout.Milliseconds()))
	if err != nil {
		return nil, nil, err
	}

	// Client-side polling - UIAutomator2 server doesn't reliably respect implicit wait
	// No sleep between retries - HTTP round-trip (~100ms) is the natural rate limit
	deadline := time.Now().Add(timeout)
	var lastErr error

	for {
		// Try UiAutomator strategies first
		elem, info, err := d.tryFindElement(strategies)
		if err == nil {
			return elem, info, nil
		}
		lastErr = err

		// For text-based selectors, also try page source matching as fallback
		// This catches hint text and other attributes UiAutomator doesn't expose directly
		if sel.Text != "" {
			_, info, err := d.findElementByPageSourceOnce(sel)
			if err == nil {
				return nil, info, nil
			}
		}

		// Check if we've exceeded timeout
		if time.Now().After(deadline) {
			break
		}
	}

	// All strategies failed after timeout
	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, fmt.Errorf("element not found after %v", timeout)
}

// findElementQuick finds an element without polling (single attempt).
// Used by waitUntil which has its own polling loop, and assertNotVisible.
// If timeoutMs is 0, uses QuickFindTimeout (1s) as default.
func (d *Driver) findElementQuick(sel flow.Selector, timeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	if timeoutMs <= 0 {
		timeoutMs = QuickFindTimeout
	}

	if sel.HasRelativeSelector() {
		return d.findElementRelative(sel, timeoutMs)
	}

	if sel.Width > 0 || sel.Height > 0 {
		return d.findElementByPageSource(sel, timeoutMs)
	}

	strategies, err := buildSelectors(sel, timeoutMs)
	if err != nil {
		return nil, nil, err
	}

	return d.tryFindElement(strategies)
}

// tryFindElement attempts to find element using given strategies (single attempt).
func (d *Driver) tryFindElement(strategies []LocatorStrategy) (*uiautomator2.Element, *core.ElementInfo, error) {
	var lastErr error
	for _, s := range strategies {
		elem, err := d.client.FindElement(s.Strategy, s.Value)
		if err != nil {
			lastErr = err
			continue
		}

		// Found element - build info
		info := &core.ElementInfo{
			ID: elem.ID(),
		}

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

		if displayed, err := elem.IsDisplayed(); err == nil {
			info.Visible = displayed
		}

		if enabled, err := elem.IsEnabled(); err == nil {
			info.Enabled = enabled
		}

		return elem, info, nil
	}

	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, fmt.Errorf("element not found")
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
	default:
		return candidates
	}
}

// findElementRelative handles relative selectors (below, above, leftOf, rightOf, childOf, containsChild, containsDescendants).
// Uses page source XML parsing to find elements by position with polling/retry.
// When multiple anchor elements exist, tries each anchor to find a valid match.
func (d *Driver) findElementRelative(sel flow.Selector, timeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = DefaultFindTimeout
	}
	deadline := time.Now().Add(timeout)

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

	var lastErr error
	var candidates []*ParsedElement

	for {
		// Get page source
		pageSource, err := d.client.Source()
		if err != nil {
			lastErr = fmt.Errorf("failed to get page source: %w", err)
			if time.Now().After(deadline) {
				return nil, nil, lastErr
			}
			continue
		}

		// Parse all elements
		allElements, err := ParsePageSource(pageSource)
		if err != nil {
			lastErr = fmt.Errorf("failed to parse page source: %w", err)
			if time.Now().After(deadline) {
				return nil, nil, lastErr
			}
			continue
		}

		// Filter by base selector to get target candidates
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
					// Convert ElementInfo bounds back to a ParsedElement for filtering
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
		// This handles cases where multiple elements have the same text (e.g., multiple "Login" elements)
		var matchingCandidates []*ParsedElement
		if len(anchors) > 0 {
			for _, anchor := range anchors {
				filtered := applyRelativeFilter(candidates, anchor, filterType)
				if len(filtered) > 0 {
					matchingCandidates = filtered
					break // Found matches with this anchor
				}
			}
			candidates = matchingCandidates
		} else if anchorSelector != nil {
			// Anchor selector specified but no anchors found
			lastErr = fmt.Errorf("anchor element not found")
			if time.Now().After(deadline) {
				return nil, nil, lastErr
			}
			continue
		}

		// Apply containsDescendants filter
		if len(sel.ContainsDescendants) > 0 {
			candidates = FilterContainsDescendants(candidates, allElements, sel.ContainsDescendants)
		}

		if len(candidates) > 0 {
			// Found matching elements
			break
		}

		lastErr = fmt.Errorf("no elements match relative criteria")
		if time.Now().After(deadline) {
			return nil, nil, lastErr
		}
		// Continue polling
	}

	if len(candidates) == 0 {
		return nil, nil, fmt.Errorf("no elements match relative criteria")
	}

	// Step 6: Prioritize clickable elements
	candidates = SortClickableFirst(candidates)

	// Step 7: Apply index if specified, otherwise use deepest matching element
	var selected *ParsedElement
	if sel.Index != "" {
		idx := 0
		if i, err := strconv.Atoi(sel.Index); err == nil {
			if i < 0 {
				i = len(candidates) + i // negative index
			}
			if i >= 0 && i < len(candidates) {
				idx = i
			}
		}
		selected = candidates[idx]
	} else {
		// Default: use deepest matching element to avoid containers
		selected = DeepestMatchingElement(candidates)
	}

	// Return element info (no WebDriver element ID, but we have bounds for tap)
	info := &core.ElementInfo{
		Text:    selected.Text,
		Bounds:  selected.Bounds,
		Enabled: selected.Enabled,
		Visible: selected.Displayed,
	}

	// For relative finds, we don't have WebDriver element - return nil element
	// Caller should use bounds for tap
	return nil, info, nil
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

	info := &core.ElementInfo{
		Text:    selected.Text,
		Bounds:  selected.Bounds,
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

		info := &core.ElementInfo{
			Text: selected.Text,
			Bounds: core.Bounds{
				X:      selected.Bounds.X,
				Y:      selected.Bounds.Y,
				Width:  selected.Bounds.Width,
				Height: selected.Bounds.Height,
			},
			Enabled: selected.Enabled,
			Visible: selected.Displayed,
		}
		return nil, info, nil
	}

	return nil, nil, fmt.Errorf("no elements match selector")
}

// findElementByPageSource finds an element using page source parsing with polling.
// Used for regex pattern matching and selectors requiring page source analysis.
func (d *Driver) findElementByPageSource(sel flow.Selector, timeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	deadline := time.Now().Add(timeout)

	for {
		// Get page source
		pageSource, err := d.client.Source()
		if err != nil {
			if time.Now().After(deadline) {
				return nil, nil, fmt.Errorf("failed to get page source: %w", err)
			}
			continue
		}

		// Parse all elements
		allElements, err := ParsePageSource(pageSource)
		if err != nil {
			if time.Now().After(deadline) {
				return nil, nil, fmt.Errorf("failed to parse page source: %w", err)
			}
			continue
		}

		// Filter by selector (this now supports regex patterns)
		candidates := FilterBySelector(allElements, sel)

		// Prioritize clickable elements
		candidates = SortClickableFirst(candidates)

		if len(candidates) > 0 {
			selected := DeepestMatchingElement(candidates)
			if selected == nil {
				selected = candidates[0]
			}

			info := &core.ElementInfo{
				Text: selected.Text,
				Bounds: core.Bounds{
					X:      selected.Bounds.X,
					Y:      selected.Bounds.Y,
					Width:  selected.Bounds.Width,
					Height: selected.Bounds.Height,
				},
				Enabled: selected.Enabled,
				Visible: selected.Displayed,
			}

			// For page source finds, we don't have WebDriver element - return nil element
			// Caller should use bounds for tap
			return nil, info, nil
		}

		// Check if we've exceeded timeout
		if time.Now().After(deadline) {
			return nil, nil, fmt.Errorf("no elements match regex pattern: %s", sel.Text)
		}
	}
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
	var strategies []LocatorStrategy
	stateFilters := buildStateFilters(sel)

	// ID-based selector - use resourceIdMatches for partial matching
	if sel.ID != "" {
		escaped := escapeUiAutomator(sel.ID)
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUiAutomator,
			Value:    `new UiSelector().resourceIdMatches(".*` + escaped + `.*")` + stateFilters,
		})
	}

	// Text-based selector - supports both regex patterns and literal text
	if sel.Text != "" {
		pattern := textToRegexPattern(sel.Text)
		// Try text first
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUiAutomator,
			Value:    `new UiSelector().textMatches("` + pattern + `")` + stateFilters,
		})
		// Also try description (content-desc) for Flutter apps
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUiAutomator,
			Value:    `new UiSelector().descriptionMatches("` + pattern + `")` + stateFilters,
		})
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

// textToRegexPattern converts text to a regex pattern for UiSelector.
// If the text is a valid regex (contains regex metacharacters), use it as-is.
// Otherwise, escape it for literal matching with case-insensitive contains.
func textToRegexPattern(text string) string {
	// Check if text looks like a regex pattern (contains unescaped metacharacters)
	if looksLikeRegex(text) {
		// Use as regex - add case-insensitive flag
		return "(?is)" + escapeUiAutomatorString(text)
	}
	// Literal text - escape and wrap for contains matching
	escaped := escapeUiAutomator(text)
	return "(?is).*" + escaped + ".*"
}

// looksLikeRegex checks if text contains regex metacharacters that suggest it's a pattern.
// Common patterns: .+, .*, [a-z], ^, $, etc.
func looksLikeRegex(text string) bool {
	// Check for common regex patterns
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch c {
		case '.', '*', '+', '?', '^', '$', '[', ']', '(', ')', '{', '}', '|':
			// Check if it's escaped
			if i > 0 && text[i-1] == '\\' {
				continue
			}
			return true
		}
	}
	return false
}

// escapeUiAutomatorString escapes only the double quotes for UiAutomator string.
// Used when the text is already a regex pattern.
func escapeUiAutomatorString(s string) string {
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

// escapeUiAutomator escapes special characters for UiAutomator selector strings.
func escapeUiAutomator(s string) string {
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
