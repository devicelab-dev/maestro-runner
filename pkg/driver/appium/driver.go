package appium

import (
	"fmt"
	"strconv"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// DefaultFindTimeout is the default timeout for element operations.
const DefaultFindTimeout = 10 * time.Second

// Driver implements core.Driver using Appium server.
type Driver struct {
	client      *Client
	platform    string        // detected from page source or capabilities
	appID       string        // current app ID
	findTimeout time.Duration // configurable timeout for finding elements
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
	case *flow.PressKeyStep:
		return d.pressKey(s)
	case *flow.ScrollUntilVisibleStep:
		return d.scrollUntilVisible(s)
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
func (d *Driver) findElementDirect(sel flow.Selector) (*core.ElementInfo, error) {
	// Try ID first
	if sel.ID != "" {
		if d.platform == "ios" {
			if elemID, err := d.client.FindElement("accessibility id", sel.ID); err == nil {
				return d.getElementInfo(elemID)
			}
		} else {
			// Android: try resource-id
			if elemID, err := d.client.FindElement("id", sel.ID); err == nil {
				return d.getElementInfo(elemID)
			}
		}
	}

	// Try text using XPath or platform-specific
	if sel.Text != "" {
		// Use page source parsing for complex text matching
		return d.findElementByPageSource(sel)
	}

	// Fallback to page source parsing
	return d.findElementByPageSource(sel)
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

	return elementToInfo(selected, platform), nil
}

// findElementRelative handles relative selectors (below, above, etc.)
func (d *Driver) findElementRelative(sel flow.Selector, timeout time.Duration) (*core.ElementInfo, error) {
	deadline := time.Now().Add(timeout)

	for {
		source, err := d.client.Source()
		if err != nil {
			if time.Now().After(deadline) {
				return nil, err
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}

		elements, platform, err := ParsePageSource(source)
		if err != nil {
			if time.Now().After(deadline) {
				return nil, err
			}
			continue
		}
		d.platform = platform

		info, err := d.findElementRelativeWithElements(sel, elements, platform)
		if err == nil && info != nil {
			return info, nil
		}

		if time.Now().After(deadline) {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("element not found")
		}
		time.Sleep(200 * time.Millisecond)
	}
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

	return elementToInfo(selected, platform), nil
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

	return &core.ElementInfo{
		ID:      elementID,
		Text:    text,
		Bounds:  core.Bounds{X: x, Y: y, Width: w, Height: h},
		Visible: displayed,
		Enabled: enabled,
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
