package appium

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// DefaultFindTimeout is the default timeout for element operations.
const DefaultFindTimeout = 12 * time.Second

// Driver implements core.Driver using Appium server.
type Driver struct {
	client                    *Client
	capabilities              map[string]interface{} // stored for session recreation (deep copy of original)
	platform                  string                 // detected from page source or capabilities
	appID                     string                 // current app ID
	ctx                       context.Context        // parent context for element-finding operations (nil = context.Background())
	findTimeout               time.Duration          // configurable timeout for finding elements
	currentWaitForIdleTimeout int                    // track current value to skip redundant calls
	waitForIdleTimeoutSet     bool                   // whether waitForIdleTimeout has been set
	lastTappedElementID       string                 // iOS: last element clicked via ClickElement, used by inputText
	warnedFields              map[string]bool
	appVersion                string // version name, supplied by the caller
	appBuild                  string // build number, supplied by the caller
}

// NewDriver creates a new Appium driver.
func NewDriver(serverURL string, capabilities map[string]interface{}) (*Driver, error) {
	// Deep-copy capabilities before Connect modifies them (adds autoLaunch, settings, etc.)
	savedCaps := deepCopyCaps(capabilities)

	client := NewClient(serverURL)

	if err := client.Connect(capabilities); err != nil {
		return nil, err
	}

	d := &Driver{
		client:       client,
		capabilities: savedCaps,
		platform:     client.Platform(),
		warnedFields: make(map[string]bool),
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

// SessionID returns the Appium/WebDriver session ID.
func (d *Driver) SessionID() string {
	return d.client.SessionID()
}

// SessionCaps returns the merged capabilities from the session creation response.
func (d *Driver) SessionCaps() map[string]interface{} {
	return d.client.SessionCaps()
}

// Keepalive sends a lightweight session-scoped command to reset the server's
// newCommandTimeout idle timer. Used while --parallel pre-creates all sessions
// so an early session isn't reaped by a cloud farm before its flow runs (#124).
func (d *Driver) Keepalive() error {
	return d.client.Keepalive()
}

// RestartSession closes the existing Appium session and creates a fresh one.
func (d *Driver) RestartSession() error {
	if err := d.client.Disconnect(); err != nil {
		logger.Warn("failed to disconnect existing session: %v", err)
	}
	// Deep-copy stored caps so Connect can mutate them freely
	caps := deepCopyCaps(d.capabilities)
	if err := d.client.Connect(caps); err != nil {
		return fmt.Errorf("failed to create new session: %w", err)
	}
	d.platform = d.client.Platform()
	// Reset cached state
	d.waitForIdleTimeoutSet = false
	d.lastTappedElementID = ""
	return nil
}

// forgetTappedElementUnlessTyping clears the element the last tapOn resolved,
// for every step that can move focus. inputText on iOS types into that element
// by id and WDA re-focuses it, so a stale id sent text into a field the flow
// had already left (or failed once it was gone). Text entry and read-only
// steps keep it; tapOn sets it again when it resolves a native element.
func (d *Driver) forgetTappedElementUnlessTyping(step flow.Step) {
	switch step.(type) {
	case *flow.InputTextStep, *flow.InputRandomStep, *flow.EraseTextStep, *flow.PasteTextStep,
		*flow.CopyTextFromStep, *flow.AssertVisibleStep, *flow.AssertNotVisibleStep,
		*flow.WaitUntilStep, *flow.TakeScreenshotStep, *flow.AssertScreenshotStep,
		*flow.WaitForAnimationToEndStep, *flow.WaitStep:
		return
	}
	d.lastTappedElementID = ""
}

// deepCopyCaps returns a deep copy of a capabilities map via JSON round-trip.
func deepCopyCaps(caps map[string]interface{}) map[string]interface{} {
	if caps == nil {
		return nil
	}
	data, err := json.Marshal(caps)
	if err != nil {
		// Fallback: shallow copy
		cp := make(map[string]interface{}, len(caps))
		for k, v := range caps {
			cp[k] = v
		}
		return cp
	}
	var cp map[string]interface{}
	if err := json.Unmarshal(data, &cp); err != nil {
		cp = make(map[string]interface{}, len(caps))
		for k, v := range caps {
			cp[k] = v
		}
	}
	return cp
}

// Execute implements core.Driver.
func (d *Driver) Execute(step flow.Step) *core.CommandResult {
	start := time.Now()
	result := d.executeStep(step)
	result.Duration = time.Since(start)
	return result
}

func (d *Driver) executeStep(step flow.Step) *core.CommandResult {
	d.forgetTappedElementUnlessTyping(step)
	switch s := step.(type) {
	case *flow.TapOnStep:
		return d.tapOn(s)
	case *flow.DoubleTapOnStep:
		return d.doubleTapOn(s)
	case *flow.LongPressOnStep:
		return d.longPressOn(s)
	case *flow.TapOnPointStep:
		return d.tapOnPoint(s)
	case *flow.DragAndDropStep:
		return d.dragAndDrop(s)
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
	case *flow.SetPermissionsStep:
		return d.setPermissions(s)
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
	case *flow.AssertScreenshotStep:
		return d.takeScreenshot(&flow.TakeScreenshotStep{CropOn: s.CropOn})
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
		DeviceName:   d.client.DeviceName(),
		DeviceID:     d.client.DeviceUDID(),
		OSVersion:    d.client.OSVersion(),
		IsSimulator:  !d.client.IsRealDevice(),
		ScreenWidth:  w,
		ScreenHeight: h,
		AppID:        d.appID,
		AppVersion:   d.appVersion,
		AppBuild:     d.appBuild,
	}
}

// SetAppInfo records the app's version name and build number for reports.
//
// Appium session capabilities do not carry either one, and this driver has no
// device access of its own to look them up — on a cloud device farm there is no
// local app bundle and no adb/simctl to ask. So the caller resolves them when it
// can and supplies them here, leaving them empty when it cannot.
func (d *Driver) SetAppInfo(version, build string) {
	d.appVersion = version
	d.appBuild = build
}

// SetContext sets the parent context for element-finding operations.
func (d *Driver) SetContext(ctx context.Context) {
	d.ctx = ctx
}

// parentContext returns the parent context for element-finding operations.
func (d *Driver) parentContext() context.Context {
	if d.ctx != nil {
		return d.ctx
	}
	return context.Background()
}

// SetFindTimeout implements core.Driver.
// Sets the default timeout (in ms) for finding elements.
func (d *Driver) SetFindTimeout(ms int) {
	d.findTimeout = time.Duration(ms) * time.Millisecond
}

// SetWaitForIdleTimeout sets the wait for idle timeout.
// 0 = disabled, >0 = wait up to N ms for device to be idle.
// Negative values are treated as 0 (disabled).
// Skips the HTTP call if the value is already set (optimization for per-flow sessions).
func (d *Driver) SetWaitForIdleTimeout(ms int) error {
	if ms < 0 {
		ms = 0
	}
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
	// Warn about unsupported selector fields (once per field)
	platform := d.platform
	if platform == "" {
		platform = "android"
	}
	if unsupported := flow.CheckUnsupportedFields(&sel, platform); len(unsupported) > 0 {
		for _, field := range unsupported {
			if !d.warnedFields[field] {
				d.warnedFields[field] = true
				log.Printf("[appium] warning: %q is not supported on %s — will be ignored", field, platform)
			}
		}
	}

	if timeout <= 0 {
		timeout = d.getFindTimeout()
	}

	// Check if selector has relative components
	if sel.HasRelativeSelector() {
		return d.findElementRelative(sel, timeout)
	}

	// Index selectors need page source (native APIs return single match)
	if sel.HasNonZeroIndex() {
		return d.findElementByPageSourceWithPolling(sel, timeout)
	}

	// Simple selector - try Appium's native find
	deadline := time.Now().Add(timeout)

	for {
		if err := d.parentContext().Err(); err != nil {
			return nil, fmt.Errorf("element '%s' not found: %w", sel.Describe(), err)
		}

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

	// Both an id and a text mean "the element with both", and none of the
	// queries below can express that: each returns on the first match for the
	// one attribute it knows about, so the id path answered and the text was
	// never read. An assertVisible with the right id and a wrong text passed
	// green, which is the dangerous direction (#157).
	//
	// Page source is where a selector's fields are all checked together — the
	// same route state filters take above, and for the same reason.
	if sel.ID != "" && sel.Text != "" {
		return d.findElementByPageSource(sel)
	}

	// Try ID first
	if sel.ID != "" {
		if d.platform == "ios" {
			if looksLikeRegex(sel.ID) {
				// Use predicate string with MATCHES for regex patterns: a
				// whole-string match, ignoring case, as Maestro's idMatches.
				escaped := escapeIOSPredicateString(sel.ID)
				predicate := fmt.Sprintf(`name MATCHES[c] "%s"`, escaped)
				if elemID, err := d.client.FindElement("-ios predicate string", predicate); err == nil && elemID != "" {
					return d.getElementInfo(elemID)
				}
			} else {
				if elemID, err := d.client.FindElement("accessibility id", sel.ID); err == nil {
					return d.getElementInfo(elemID)
				}
				// accessibility id is an exact, case-sensitive match, but the
				// page-source matcher ignores case, so its miss proves
				// nothing. name CONTAINS[c] accepts everything the matcher
				// does; if it finds nothing either, skip the source (#173).
				if iosIDIsLiteral(sel.ID) && d.iosNativelyAbsent(iosIDContainsPredicate(sel.ID)) {
					return nil, fmt.Errorf("element not found: %s", sel.Describe())
				}
			}
		} else {
			if looksLikeRegex(sel.ID) {
				// Regex ID: use page source (Appium's UiAutomator calls are slow when element absent)
				return d.findElementByPageSource(sel)
			}
			// Literal ID: the exact id first; the id strategy prefixes the app
			// package itself when the id has none. Then the whole id, or the
			// part after the package prefix, ignoring case, as Maestro's
			// idMatches; a substring query found "login_button" for
			// `id: login` (#188).
			if elemID, err := d.client.FindElement("id", sel.ID); err == nil && elemID != "" {
				return d.getElementInfo(elemID)
			}
			uiSelector := `new UiSelector().resourceIdMatches("` + escapeUIAutomatorString(core.UiAutomatorIDRegex(sel.ID)) + `")`
			if elemID, err := d.client.FindElement("-android uiautomator", uiSelector); err == nil && elemID != "" {
				return d.getElementInfo(elemID)
			}
		}
	}

	// Try text using native platform strategies (fast)
	if sel.Text != "" {
		if d.platform == "ios" {
			// iOS: -ios predicate string over what the element shows (label,
			// value), as the page-source matcher does; name is the
			// accessibility identifier, which only id: matches (#178). The
			// whole value must match, as in Maestro: this was CONTAINS[c],
			// which returned a "Talk · Open" row for "Open" (#188). Text with
			// regex syntax is left to the page-source matcher, as ICU's regex
			// dialect is not Go's.
			if core.IsPlainSelectorText(sel.Text) {
				if elemID, err := d.client.FindElement("-ios predicate string", iosWholeTextPredicate(sel.Text, "label", "value")); err == nil && elemID != "" {
					return d.getElementInfo(elemID)
				}
			}
			// The query above leaves out placeholderValue, which the
			// page-source matcher reads, and values that match only with
			// their line breaks read as spaces. An element holding every
			// word of the text in one of the three covers both; when none
			// does, the text is absent, and the source dump — 20s or more on
			// a large tree over a cloud endpoint — is skipped (#173).
			if !looksLikeRegex(sel.Text) {
				if probe := iosTextNeedlesPredicate(sel.Text); probe != "" && d.iosNativelyAbsent(probe) {
					return nil, fmt.Errorf("element not found: %s", sel.Describe())
				}
			}
		} else {
			// Android: use UiAutomator selectors (much faster than page source).
			// Every query matches the whole text or description, as Maestro
			// does (core.UiSelectorTextTiers); the textContains forms this
			// used found "Talk · Open" for "Open" (#188).
			//
			// Each query costs a round trip. While polling for an element
			// that has not appeared yet — the common case — every one of
			// them missed, repeatedly: measured on a Pixel 4a, 22 of 30 finds
			// in a single flow were misses burning 2.8s, more than the
			// successful finds cost.
			//
			// The case-insensitive forms (the last tier) find everything the
			// earlier tiers can, except a value that equals the selector only
			// literally ("$7.50", whose $ anchors as a regex), which the page
			// source finds. Probe with those two first and go to the page
			// source when they find nothing.
			tiers := core.UiSelectorTextTiers(sel.Text, false)
			probes := tiers[len(tiers)-1]
			textHit, textErr := d.client.FindElement("-android uiautomator", `new UiSelector()`+probes[0])
			descHit, descErr := "", error(nil)
			if textErr != nil || textHit == "" {
				descHit, descErr = d.client.FindElement("-android uiautomator", `new UiSelector()`+probes[1])
			}
			if (textErr != nil || textHit == "") && (descErr != nil || descHit == "") {
				// Nothing on screen matches by text or description.
				return d.findElementByPageSource(sel)
			}

			// Something matches. Now prefer the most specific form, since
			// several elements can qualify: equal to the selector, then
			// matching it in its own case (#151), text before description.
			for _, tier := range tiers[:len(tiers)-1] {
				for _, body := range tier {
					if elemID, err := d.client.FindElement("-android uiautomator", `new UiSelector()`+body); err == nil && elemID != "" {
						return d.getElementInfo(elemID)
					}
				}
			}

			// Only the case-insensitive form matched — use the probe's hit
			// rather than paying for the same lookup again.
			if textHit != "" {
				return d.getElementInfo(textHit)
			}
			if descHit != "" {
				return d.getElementInfo(descHit)
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
	elements = d.onScreen(elements)

	// Filter by selector
	candidates := FilterBySelector(elements, sel, platform)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no elements match selector")
	}

	// Prioritize clickable, then select by index or deepest
	candidates = SortClickableFirst(candidates)
	if err := core.IndexError(len(candidates), sel.Index); err != nil {
		return nil, err
	}
	selected := SelectByIndex(candidates, sel.Index)

	// If element isn't clickable, try to find a clickable parent
	// This handles React Native pattern where text nodes aren't clickable but containers are
	clickableElem := GetClickableElement(selected)

	return elementToInfoWithClickable(selected, clickableElem, platform), nil
}

// findElementByPageSourceWithPolling finds element by page source with deadline-based polling.
func (d *Driver) findElementByPageSourceWithPolling(sel flow.Selector, timeout time.Duration) (*core.ElementInfo, error) {
	deadline := time.Now().Add(timeout)
	for {
		if err := d.parentContext().Err(); err != nil {
			return nil, fmt.Errorf("element '%s' not found: %w", sel.Describe(), err)
		}

		info, err := d.findElementByPageSource(sel)
		if err == nil {
			return info, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(200 * time.Millisecond)
	}
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

	// Index selectors need page source (native APIs return single match)
	if sel.HasNonZeroIndex() {
		return d.findElementByPageSourceWithPolling(sel, timeout)
	}

	deadline := time.Now().Add(timeout)

	for {
		if err := d.parentContext().Err(); err != nil {
			return nil, fmt.Errorf("element '%s' not found: %w", sel.Describe(), err)
		}

		var info *core.ElementInfo
		var err error

		if needsFullSelectorMatch(sel) {
			// The text queries below know only the text, so an id, a state
			// filter or a size next to it was dropped and the first element
			// with that text was tapped (the #157 defect, on the tap path).
			// Page source checks every field together.
			info, err = d.findElementByPageSource(sel)
		} else if sel.Text != "" && d.platform == "ios" {
			// iOS: exact match first, then page source with clickable prioritization
			info, err = d.findElementForTapIOS(sel)
		} else if sel.Text != "" && d.platform != "ios" {
			// Android: clickable-first approach via UiAutomator
			info, err = d.findElementForTapDirect(sel)
		} else {
			// ID-based selectors: standard approach
			info, err = d.findElementDirect(sel)
		}

		if err == nil && info != nil {
			return info, nil
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("element not found: %s", sel.Describe())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// onScreen drops elements less than 10% inside the viewport. Page source lists
// the whole accessibility tree, off-screen rows included, so without this a
// below-the-fold element passed assertVisible and could win an index or a
// tap. Same rule as Maestro's filterOutOfBounds and the uiautomator2 and WDA
// drivers. Without a known screen size the list is returned unchanged.
func (d *Driver) onScreen(elements []*ParsedElement) []*ParsedElement {
	w, h := d.client.ScreenSize()
	if w <= 0 || h <= 0 {
		return elements
	}
	out := make([]*ParsedElement, 0, len(elements))
	for _, e := range elements {
		if e.Bounds.VisiblePercentage(w, h) >= 0.1 {
			out = append(out, e)
		}
	}
	return out
}

// needsFullSelectorMatch reports whether a text selector carries other fields
// (id, state filters, size) that the text-only native queries cannot express.
func needsFullSelectorMatch(sel flow.Selector) bool {
	if sel.Text == "" {
		return false
	}
	return sel.ID != "" || sel.Enabled != nil || sel.Selected != nil || sel.Focused != nil ||
		sel.Checked != nil || sel.Width > 0 || sel.Height > 0
}

// findElementForTapDirect finds element for tap, trying clickable first then fallback to page source.
//
// Every query matches the whole text or description, as Maestro does
// (core.UiSelectorTextTiers): equal to the selector, then as a regex in its
// own case, then ignoring case. The textContains passes used here tapped a
// clickable "Talk · Open" row for `tapOn: Open` (#188).
func (d *Driver) findElementForTapDirect(sel flow.Selector) (*core.ElementInfo, error) {
	tiers := core.UiSelectorTextTiers(sel.Text, false)
	if len(tiers) == 0 {
		return d.findElementByPageSource(sel)
	}

	// Step 1: Try clickable elements first, most specific tier first.
	for _, tier := range tiers {
		for _, body := range tier {
			if elemID, err := d.client.FindElement("-android uiautomator", `new UiSelector()`+body+`.clickable(true)`); err == nil && elemID != "" {
				return d.getElementInfo(elemID)
			}
		}
	}

	// Step 2: Check if the text exists at all (without clickable filter). The
	// last tier, ignoring case, finds everything the others can.
	var textExistsErr error = fmt.Errorf("element with text '%s' not found", sel.Text)
	for _, body := range tiers[len(tiers)-1] {
		if _, err := d.client.FindElement("-android uiautomator", `new UiSelector()`+body); err == nil {
			textExistsErr = nil
			break
		}
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

// findElementForTapIOS finds element for tap on iOS, prioritizing clickable elements.
// Step 1: Try a whole-text match via iOS predicate (avoids substring false
// positives, e.g., "Sign In" button vs "Sign in to continue" text), for text
// without regex syntax.
//
// Step 2: Fall back to page source which has clickable prioritization
//
//	(SortClickableFirst + GetClickableElement).
func (d *Driver) findElementForTapIOS(sel flow.Selector) (*core.ElementInfo, error) {
	// Step 1: Try the whole text (fast path — returns Appium element ID)
	if core.IsPlainSelectorText(sel.Text) {
		if elemID, err := d.client.FindElement("-ios predicate string", iosWholeTextPredicate(sel.Text, "label", "value")); err == nil && elemID != "" {
			return d.getElementInfo(elemID)
		}
	}

	// No whole match. If no element even holds the text's words, the page
	// source cannot find one either, so do not pay for it (#173).
	if !looksLikeRegex(sel.Text) {
		if probe := iosTextNeedlesPredicate(sel.Text); probe != "" && d.iosNativelyAbsent(probe) {
			return nil, fmt.Errorf("element not found: %s", sel.Describe())
		}
	}

	// Step 2: Page source with clickable prioritization
	return d.findElementByPageSource(sel)
}

// iosNativelyAbsent runs one iOS predicate query and reports whether Appium
// answered "no such element". Only that answer proves absence: any other
// error — an older WebDriverAgent that rejects an attribute name, a session
// problem — returns false, and the caller falls back to the page source as
// before.
//
// Each predicate passed here must accept every element the page-source
// matcher would, so that "absent" here means the matcher finds nothing too.
// On a large tree over a cloud endpoint, GET /source measured ~20s and at
// worst exceeded Appium's 60s limit, which ends the session (#173). A miss is
// the normal case for when: conditions and for every poll before an element
// appears, so skipping the dump for a proven absence matters.
func (d *Driver) iosNativelyAbsent(predicate string) bool {
	_, err := d.client.FindElement("-ios predicate string", predicate)
	return err != nil && strings.HasPrefix(err.Error(), "no such element")
}

// iosWholeTextPredicate matches any of attrs against a plain text selector
// (core.IsPlainSelectorText) as Maestro does: the whole value, ignoring case,
// with its dots matching any character. For such text ICU's MATCHES and Go's
// regexp agree.
func iosWholeTextPredicate(text string, attrs ...string) string {
	re := escapeIOSPredicateString("(?s)" + text)
	parts := make([]string, len(attrs))
	for i, attr := range attrs {
		parts[i] = fmt.Sprintf(`%s MATCHES[c] "%s"`, attr, re)
	}
	return strings.Join(parts, " OR ")
}

// iosTextNeedlesPredicate matches every element the page-source matcher
// accepts for a text selector: one whose label, value or placeholderValue
// holds every word the selector must match (core.LiteralNeedles), ignoring
// case. Words rather than the phrase, because a value can match with a line
// break where the selector has a space. Empty when the selector has no words
// to require, and the caller then cannot prove absence.
func iosTextNeedlesPredicate(text string) string {
	needles := core.LiteralNeedles(text)
	if len(needles) == 0 {
		return ""
	}
	var alts []string
	for _, attr := range []string{"label", "value", "placeholderValue"} {
		terms := make([]string, len(needles))
		for i, n := range needles {
			terms[i] = fmt.Sprintf(`%s CONTAINS[c] "%s"`, attr, escapeIOSPredicateString(n))
		}
		alts = append(alts, "("+strings.Join(terms, " AND ")+")")
	}
	return strings.Join(alts, " OR ")
}

// iosIDContainsPredicate matches every element the page-source matcher
// accepts for a literal id, which matches the whole name ignoring case: any
// name that contains it, ignoring case, is a superset.
func iosIDContainsPredicate(id string) string {
	return fmt.Sprintf(`name CONTAINS[c] "%s"`, escapeIOSPredicateString(id))
}

// iosIDIsLiteral reports whether id has no regex syntax, so that a name
// matching it must contain it. looksLikeRegex lets a standalone "." through
// as literal, but matchesID compiles the id as a regex, where "." matches any
// character — so an id with a dot can match names that CONTAINS would not,
// and is left to the page source.
func iosIDIsLiteral(id string) bool {
	return !looksLikeRegex(id) && !strings.Contains(id, ".")
}

// findElementRelative handles relative selectors (below, above, etc.)
// Deprecated: Use findElementRelativeWithContext for new code.
func (d *Driver) findElementRelative(sel flow.Selector, timeout time.Duration) (*core.ElementInfo, error) {
	ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
	defer cancel()
	return d.findElementRelativeWithContext(ctx, sel)
}

// findElementOnce finds an element with a single attempt (no polling).
// Used for quick checks like waitUntil where we poll externally.
func (d *Driver) findElementOnce(sel flow.Selector) (*core.ElementInfo, error) {
	if sel.HasRelativeSelector() {
		return d.findElementRelativeOnce(sel)
	}
	if sel.HasNonZeroIndex() {
		return d.findElementByPageSource(sel)
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

			info, err := d.findElementRelativeWithElements(sel, d.onScreen(elements), platform)
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

	return d.findElementRelativeWithElements(sel, d.onScreen(elements), platform)
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
	if sel.Index == "" && (filterType == filterBelow || filterType == filterAbove || filterType == filterLeftOf || filterType == filterRightOf) {
		// Directional filters sort candidates by distance. Pick the closest
		// (first) element to match Maestro's .firstOrNull() behavior.
		selected = candidates[0]
	} else {
		if err := core.IndexError(len(candidates), sel.Index); err != nil {
			return nil, err
		}
		selected = SelectByIndex(candidates, sel.Index)
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

// getElementInfo describes a found element.
//
// Every request here is a round trip, and on a real device each one costs about
// the same regardless of what it asks for — measured at ~25ms apiece on a Pixel
// 4a — so the call count is the whole cost, not the work per call. Against a
// cloud grid it is worse again. So this asks only for what something downstream
// actually reads:
//
//   - rect is needed by every gesture to derive coordinates, and reaches the report
//   - text is read by copyTextFrom and assertions, and reaches the report
//   - displayed is the visibility answer itself
//
// Enabled used to be fetched here and nothing ever read it: selectors that
// filter on element state are routed to the page-source path, which builds that
// field from XML, and the report keeps only id, text, class and bounds.
//
// The accessibility description is fetched by the one command that wants it
// (see accessibilityLabelOf) rather than on every lookup.
func (d *Driver) getElementInfo(elementID string) (*core.ElementInfo, error) {
	x, y, w, h, err := d.client.GetElementRect(elementID)
	if err != nil {
		return nil, err
	}

	text, _ := d.client.GetElementText(elementID)
	displayed, _ := d.client.IsElementDisplayed(elementID)

	return &core.ElementInfo{
		ID:      elementID,
		Text:    text,
		Bounds:  core.Bounds{X: x, Y: y, Width: w, Height: h},
		Visible: displayed,
	}, nil
}

// accessibilityLabelOf reads an element's accessibility description, which iOS
// exposes as "label" and Android as "content-desc". Only copyTextFrom needs it,
// as a fallback when an element carries no text, so it is fetched on demand
// instead of on every element lookup.
func (d *Driver) accessibilityLabelOf(elementID string) string {
	// Elements resolved from page source carry a resource id here rather than
	// an Appium element handle, and the server cannot look one up. Those
	// already fold the description into Text during parsing, so there is
	// nothing to fetch — skip rather than spend a round trip that can only fail.
	if elementID == "" || strings.Contains(elementID, ":id/") {
		return ""
	}
	attr := "content-desc"
	if d.platform == "ios" {
		attr = "label"
	}
	label, _ := d.client.GetElementAttribute(elementID, attr)
	return label
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
	return core.SuccessResult(msg, elem)
}

func errorResult(err error, msg string) *core.CommandResult {
	return core.ErrorResult(err, msg)
}
