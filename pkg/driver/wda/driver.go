package wda

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// Driver implements core.Driver using WebDriverAgent for iOS.
type Driver struct {
	client *Client
	info   *core.PlatformInfo
	udid   string // Device UDID for simctl commands

	// Parent context for element-finding operations (nil = context.Background())
	ctx context.Context

	// App file path for clearState (uninstall+reinstall)
	appFile string

	// WDA alert action for real device permission handling ("accept", "dismiss", or "")
	alertAction string

	// Timeouts (0 = use defaults)
	findTimeout         int // ms, for required elements
	optionalFindTimeout int // ms, for optional elements

	// Typing speed (0 = use WDA default of 60 keys/sec)
	typingFrequency int

	// Selector validation dedup
	warnedFields map[string]bool

	// Crash-loop detection. When the app under test keeps dying immediately
	// after launch (debug builds, signing mismatch, runtime crash on startup)
	// we get a flood of "app not running" / "session lost" errors. Without
	// this gate, the runner would chew through the full flow-level timeout
	// retrying every step against a dead app.
	appDeathCount    int
	appDeathFirstAt  time.Time
	crashAbortReason string
}

// Crash-loop detection thresholds.
const (
	crashLoopThreshold  = 4               // N app-death errors → abort
	crashLoopTimeWindow = 6 * time.Second // ...within this window
)

// NewDriver creates a new WDA driver.
func NewDriver(client *Client, info *core.PlatformInfo, udid string) *Driver {
	return &Driver{
		client: client,
		info:   info,
		udid:   udid,
		// Default to "" so WDA's alerts monitor is NOT registered at session
		// creation. We rely on PrepareForFlow (core.FlowAware) to set this to
		// "accept"/"dismiss" if the flow has a launchApp step BEFORE
		// EnsureSession runs — see PrepareForFlow below.
		//
		// Rationale: WDA only registers the alerts monitor when
		// defaultAlertAction is in the session-creation capabilities (see
		// FBSessionCommands.m). Setting it later via /appium/settings just
		// changes the value and cannot start the monitor retroactively. The
		// previous "always accept" default ($COMMIT 2702939, Refs #64) fixed
		// permission-dialog auto-handling but also caused WDA to auto-dismiss
		// in-app confirmation alerts ("Discard changes?" etc.). Pre-scanning
		// the flow gives us the right monitor state without that regression
		// and matches upstream maestro's behavior (SystemPermissionHelper
		// short-circuits when no permissions are configured).
		alertAction:  "",
		warnedFields: make(map[string]bool),
	}
}

// PrepareForFlow implements core.FlowAware. Called once before EnsureSession,
// it scans the flow's steps for a LaunchAppStep and sets d.alertAction to the
// resolved value (so the WDA alerts monitor is registered with the right
// behavior at session creation). Flows without launchApp leave alertAction
// at "", which means no monitor is registered and in-app dialogs aren't
// auto-handled.
func (d *Driver) PrepareForFlow(steps []flow.Step) {
	for _, s := range steps {
		launchApp, ok := s.(*flow.LaunchAppStep)
		if !ok {
			continue
		}
		d.alertAction = resolveAlertAction(launchApp.Permissions)

		// Real-device safety net (#108): system permission dialogs are
		// auto-handled ONLY via WDA's defaultAlertAction, which needs a single
		// accept/dismiss. Mixed permissions (e.g. camera=allow, location=deny)
		// resolve to "" → no monitor → a permission prompt hangs the flow and the
		// un-dismissed SpringBoard alert can wedge later runs. The simulator
		// doesn't hit this (permissions are pre-granted via simctl). Warn so the
		// failure isn't silent. An explicit `unset` is an intentional opt-out and
		// stays silent.
		if d.alertAction == "" && d.info != nil && !d.info.IsSimulator &&
			len(launchApp.Permissions) > 0 && !hasAllValue(launchApp.Permissions, "unset") {
			logger.Warn("[wda] launchApp declares mixed permissions; on a real iOS device WDA can only auto-accept or auto-dismiss ALL permission dialogs, so they won't be auto-handled and may block the flow (and can wedge the device). Use permissions: { all: allow } (or { all: deny }), or add an explicit acceptAlert/dismissAlert step where the prompt appears.")
		}
		return
	}
	// No launchApp in this flow — leave alertAction at "" (no auto-handling).
}

// EnsureSession creates a WDA session if one doesn't exist.
// Called by the flow runner before execution starts when the flow has no launchApp step.
// If launchApp runs later, it will replace this session.
func (d *Driver) EnsureSession(appID string) error {
	if d.client.HasSession() {
		return nil
	}
	if err := d.client.CreateSession(appID, d.alertAction); err != nil {
		return fmt.Errorf("failed to create WDA session: %w", err)
	}
	// Disable quiescence to prevent XCTest crashes
	_ = d.client.UpdateSettings(map[string]interface{}{
		"shouldWaitForQuiescence": false,
		"waitForIdleTimeout":      0,
	})
	return nil
}

// screenSize returns cached screen dimensions from PlatformInfo.
func (d *Driver) screenSize() (int, int, error) {
	if d.info != nil && d.info.ScreenWidth > 0 && d.info.ScreenHeight > 0 {
		return d.info.ScreenWidth, d.info.ScreenHeight, nil
	}
	return 0, 0, fmt.Errorf("screen dimensions not available")
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

// SetFindTimeout sets the timeout for finding required elements.
func (d *Driver) SetFindTimeout(ms int) {
	d.findTimeout = ms
}

// SetOptionalFindTimeout sets the timeout for finding optional elements.
func (d *Driver) SetOptionalFindTimeout(ms int) {
	d.optionalFindTimeout = ms
}

// SetAppFile sets the app file path used for clearState (uninstall+reinstall).
func (d *Driver) SetAppFile(path string) {
	d.appFile = path
}

// SetWaitForIdleTimeout sets the wait for idle timeout.
// Quiescence is disabled by default on iOS because it can cause XCTest crashes
// on apps with continuous animations. It is only enabled when the user explicitly
// sets a timeout > 200ms (the CLI default), indicating they want idle waiting.
// Negative values and 0 disable quiescence. Values 1-200 are a no-op (keep session default).
// Values > 200 enable quiescence — minimum effective value is 200ms.
func (d *Driver) SetWaitForIdleTimeout(ms int) error {
	if ms > 200 {
		return d.client.UpdateSettings(map[string]interface{}{
			"shouldWaitForQuiescence": true,
			"waitForIdleTimeout":      ms,
		})
	}
	if ms <= 0 {
		return d.client.DisableQuiescence()
	}
	// ms 1-200 (default range): keep quiescence disabled (session default)
	return nil
}

// SetTypingFrequency sets the WDA typing speed in keys/sec.
// The value is stored and passed per-request via the frequency parameter
// on SendKeys/ElementSendKeys calls. 0 means use WDA default (60 keys/sec).
func (d *Driver) SetTypingFrequency(freq int) error {
	d.typingFrequency = freq
	return nil
}

// Element finding timeouts (milliseconds).
const (
	DefaultFindTimeout  = 12000 // 12 seconds for required elements
	OptionalFindTimeout = 7000  // 7 seconds for optional elements
	QuickFindTimeout    = 1000  // 1 second for quick checks
)

// Execute runs a single step and returns the result.
func (d *Driver) Execute(step flow.Step) *core.CommandResult {
	start := time.Now()

	// If we've already detected a crash-loop on this driver, every
	// subsequent step short-circuits with the same actionable error.
	// Avoids spamming the user with the same diagnosis on every step.
	if d.crashAbortReason != "" {
		return &core.CommandResult{
			Success:  false,
			Error:    fmt.Errorf("%s", d.crashAbortReason),
			Message:  d.crashAbortReason,
			Duration: time.Since(start),
		}
	}

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
	case *flow.AcceptAlertStep:
		result = d.acceptAlert(s)
	case *flow.DismissAlertStep:
		result = d.dismissAlert(s)
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
	case *flow.SetLocationStep:
		result = d.setLocation(s)
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

	// Airplane mode
	case *flow.SetAirplaneModeStep:
		result = d.setAirplaneMode(s)
	case *flow.ToggleAirplaneModeStep:
		result = d.toggleAirplaneMode(s)

	// Permissions
	case *flow.SetPermissionsStep:
		result = d.setPermissions(s)

	// Keychain
	case *flow.ClearKeychainStep:
		result = d.clearKeychain(s)

	default:
		result = &core.CommandResult{
			Success: false,
			Error:   fmt.Errorf("unknown step type: %T", step),
			Message: fmt.Sprintf("Step type '%T' is not supported on iOS", step),
		}
	}

	result.Duration = time.Since(start)
	d.trackCrashLoop(result)
	return result
}

// trackCrashLoop counts consecutive "app died on launch" failures and trips a
// circuit-breaker after several within a short window. The follow-up Execute
// calls then short-circuit with a clear error mentioning the most likely
// causes — saves the user from waiting out a flow-level timeout staring at
// "session lost / app not running" errors that all stem from the same root.
func (d *Driver) trackCrashLoop(result *core.CommandResult) {
	if d.crashAbortReason != "" {
		return // already aborted
	}
	if result.Success {
		// Any success resets the counter — the app is alive again.
		d.appDeathCount = 0
		return
	}
	if !isAppDeathError(result) {
		return
	}

	now := time.Now()
	if d.appDeathCount == 0 || now.Sub(d.appDeathFirstAt) > crashLoopTimeWindow {
		d.appDeathFirstAt = now
		d.appDeathCount = 1
		return
	}
	d.appDeathCount++
	if d.appDeathCount >= crashLoopThreshold {
		d.crashAbortReason = "Aborting: the app under test appears to be crashing on launch (" +
			fmt.Sprintf("%d 'app died' / 'session lost' errors in %.1fs).\n",
				d.appDeathCount, now.Sub(d.appDeathFirstAt).Seconds()) +
			"Common causes on iOS:\n" +
			"  • Flutter debug build — rebuild with `flutter build ios --release` or `--profile`.\n" +
			"  • Code-signing / provisioning mismatch — re-sign with the team ID passed via --team-id.\n" +
			"  • App crashes immediately on startup — check device logs with `idevicesyslog` (real device)\n" +
			"    or `xcrun simctl spawn booted log stream` (simulator).\n"
		log.Printf("[wda] %s", d.crashAbortReason)
	}
}

// isAppDeathError matches result messages / error texts that indicate the
// app under test is no longer running. Patterns drawn from real WDA failure
// modes observed in #38 and similar repros.
func isAppDeathError(result *core.CommandResult) bool {
	if result == nil {
		return false
	}
	combined := result.Message
	if result.Error != nil {
		combined += " " + result.Error.Error()
	}
	combined = strings.ToLower(combined)

	signals := []string{
		"application is not in foreground",
		"application is not running",
		"app is not running",
		"application died",
		"session does not exist",
		"session is not started",
		"invalid session id",
		"could not start app",
		"failed to launch",
		"no such session",
		"connection reset",
		"connection refused",
	}
	for _, s := range signals {
		if strings.Contains(combined, s) {
			return true
		}
	}
	return false
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
	// Warn about unsupported selector fields (once per field)
	if unsupported := flow.CheckUnsupportedFields(&sel, "ios"); len(unsupported) > 0 {
		for _, field := range unsupported {
			if !d.warnedFields[field] {
				d.warnedFields[field] = true
				log.Printf("[wda] warning: %q is not supported on ios — will be ignored", field)
			}
		}
	}

	timeout := d.calculateTimeout(optional, stepTimeoutMs)
	ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
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
			// Try WDA strategies first (skip for index selectors — WDA returns single match)
			if !sel.HasNonZeroIndex() {
				if info, err := d.findElementByWDA(sel); err == nil {
					return info, nil
				}
			}

			// Fallback to page source parsing
			if info, err := d.findElementByPageSourceOnce(sel); err == nil {
				return info, nil
			} else {
				lastErr = err
			}
			time.Sleep(50 * time.Millisecond)
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
		ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
		defer cancel()
		return d.findElementRelativeWithContext(ctx, sel)
	}

	// For index selectors, use standard findElement which routes to page source
	// (WDA native API returns single match, can't pick Nth)
	if sel.HasNonZeroIndex() {
		return d.findElement(sel, optional, stepTimeoutMs)
	}

	// For ID-based selectors, use standard findElement (IDs are usually unique)
	if sel.ID != "" {
		return d.findElement(sel, optional, stepTimeoutMs)
	}

	// For text-based selectors, use smart fallback strategy
	if sel.Text != "" {
		timeout := d.calculateTimeout(optional, stepTimeoutMs)
		ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
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

			// If exact predicate found element, try getElementInfo directly — avoids
			// a full page source fetch when the element is already identified.
			if exactElemID != "" {
				if info, err := d.getElementInfo(exactElemID); err == nil {
					return info, nil
				}
			}

			// Step 2b: Check if text exists via substring WDA predicate
			predicateBase := fmt.Sprintf("label CONTAINS[c] '%s' OR name CONTAINS[c] '%s' OR value CONTAINS[c] '%s'",
				sel.Text, sel.Text, sel.Text)
			predicate := "(" + predicateBase + ")" + stateFilter
			containsElemID, textExistsErr := d.client.FindElement("predicate string", predicate)

			if textExistsErr != nil {
				// Text not found via WDA at all - try page source as fallback
				info, psErr := d.findElementByPageSourceOnce(sel)
				if psErr == nil {
					return info, nil
				}
				// Still not found - keep polling. Surface both failures:
				// the page-source reason (with closest-text hints) is the
				// actionable one; hiding it behind the predicate error sent
				// issue #89's diagnosis down the wrong path.
				lastErr = fmt.Errorf("%w; page source: %v", textExistsErr, psErr)
				continue
			}

			// Step 3: Text exists but not in an interactive element → page source with parent lookup
			if info, err := d.findElementByPageSourceOnce(sel); err == nil {
				return info, nil
			}

			// Step 4: Page source failed (e.g. quiescence error) — use the contains predicate element.
			if containsElemID != "" {
				info, err := d.getElementInfo(containsElemID)
				if err == nil {
					return info, nil
				}
				lastErr = err
			}
		}
	}
}

// findInteractiveElementByWDA tries WDA queries for interactive element types in parallel.
func (d *Driver) findInteractiveElementByWDA(sel flow.Selector, stateFilter string) (*core.ElementInfo, error) {
	type queryResult struct {
		elemID string
		err    error
		prio   int // lower = higher priority (TextField > SecureTextField > Button > fallback predicate)
	}

	textFieldChain := fmt.Sprintf("**/XCUIElementTypeTextField[`(label CONTAINS[c] '%s' OR value CONTAINS[c] '%s' OR placeholderValue CONTAINS[c] '%s')%s`]", sel.Text, sel.Text, sel.Text, stateFilter)
	secureFieldChain := fmt.Sprintf("**/XCUIElementTypeSecureTextField[`(label CONTAINS[c] '%s' OR value CONTAINS[c] '%s' OR placeholderValue CONTAINS[c] '%s')%s`]", sel.Text, sel.Text, sel.Text, stateFilter)
	buttonChain := fmt.Sprintf("**/XCUIElementTypeButton[`(label ==[c] '%s' OR name ==[c] '%s')%s`]", sel.Text, sel.Text, stateFilter)

	// Fallback: combined predicate for all interactive types.
	// Class chain can fail due to quiescence while predicate queries may succeed.
	fallbackPred := fmt.Sprintf(
		"((type == 'XCUIElementTypeTextField' OR type == 'XCUIElementTypeSecureTextField' OR type == 'XCUIElementTypeSearchField') AND (label CONTAINS[c] '%s' OR value CONTAINS[c] '%s')) OR (type == 'XCUIElementTypeButton' AND (label ==[c] '%s' OR name ==[c] '%s'))",
		sel.Text, sel.Text, sel.Text, sel.Text,
	)
	if stateFilter != "" {
		fallbackPred = fmt.Sprintf("(%s)%s", fallbackPred, stateFilter)
	}

	queries := []struct {
		strategy string
		value    string
		prio     int
	}{
		{"class chain", textFieldChain, 0},
		{"class chain", secureFieldChain, 1},
		{"class chain", buttonChain, 2},
		{"predicate string", fallbackPred, 3},
	}

	ch := make(chan queryResult, len(queries))
	for _, q := range queries {
		go func(strategy, value string, prio int) {
			elemID, err := d.client.FindElement(strategy, value)
			if err != nil || elemID == "" {
				ch <- queryResult{"", err, prio}
				return
			}
			ch <- queryResult{elemID, nil, prio}
		}(q.strategy, q.value, q.prio)
	}

	var bestID string
	bestPrio := len(queries) // higher than any valid prio
	for i := 0; i < len(queries); i++ {
		r := <-ch
		if r.err == nil && r.elemID != "" && r.prio < bestPrio {
			bestID = r.elemID
			bestPrio = r.prio
		}
	}

	if bestID != "" {
		return d.getElementInfo(bestID)
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

	// Handle index selectors via page source (need all matches to pick Nth)
	if sel.HasNonZeroIndex() {
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

// filterVisibleOrHostingVisible keeps candidates WDA reports visible="true",
// plus candidates marked visible="false" that nonetheless host at least one
// visible descendant. The second class covers RN container testIDs and other
// wrapper views that XCUITest can't classify as accessible but which clearly
// contain visible content. Hidden-but-still-mounted screens (all descendants
// invisible) are correctly excluded.
//nolint:unused
func filterVisibleOrHostingVisible(candidates []*ParsedElement) []*ParsedElement {
	out := candidates[:0]
	for _, c := range candidates {
		if c.Displayed || HasVisibleDescendant(c) {
			out = append(out, c)
		}
	}
	return out
}

// rescueNote returns a non-empty match note when elem was accepted only
// because it hosts visible descendants (XCUITest marked the container itself
// visible="false"). Empty for normal matches.
func rescueNote(elem *ParsedElement) string {
	if elem.Displayed {
		return ""
	}
	return "XCUITest reported visible=false but bounds within screen viewport — accepted"
}

// selectorLog returns a compact string describing a selector for log lines.
func selectorLog(sel flow.Selector) string {
	if sel.ID != "" {
		return fmt.Sprintf("id=%q", sel.ID)
	}
	if sel.Text != "" {
		return fmt.Sprintf("text=%q", sel.Text)
	}
	return sel.Describe()
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
// Used primarily by assertions — tries generic predicate first since most asserts
// target StaticText/labels, not TextFields. Tap actions use findElementForTap instead.
func (d *Driver) findElementByWDA(sel flow.Selector) (*core.ElementInfo, error) {
	stateFilter := buildStateFilter(sel)

	// Try class chain for accessibility ID
	if sel.ID != "" {
		// Use CONTAINS for literal IDs, MATCHES for regex patterns
		op := "CONTAINS"
		if looksLikeRegex(sel.ID) {
			op = "MATCHES"
		}
		query := fmt.Sprintf("**/XCUIElementTypeAny[`name %s '%s'%s`]", op, sel.ID, stateFilter)
		elemID, err := d.client.FindElement("class chain", query)
		if err == nil && elemID != "" {
			return d.getElementInfo(elemID)
		}
	}

	if sel.Text != "" {
		// Try generic predicate first — most assertions target StaticText/labels,
		// so this avoids 3 wasted type-specific queries (TextField, SecureTextField, Button)
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
// Fetches text, rect, displayed status, and element name in parallel for speed.
//
// Visibility policy: XCUITest's `displayed` flag is unreliable on React Native
// testID-bearing wrappers — it returns false even when the visible content
// paints at the same bounds via a sibling node. Rather than trust the flag,
// we apply a single bounds-based rule that works for tap / type / assertVisible
// alike:
//   - displayed=true → accept (XCUITest agrees, no override needed)
//   - displayed=false AND bounds are within the screen viewport (>=10% visible)
//     → accept, set MatchNote noting we overrode XCUITest's opinion based on bounds
//   - displayed=false AND bounds are off-screen → reject (genuinely not visible)
func (d *Driver) getElementInfo(elemID string) (*core.ElementInfo, error) {
	info := &core.ElementInfo{
		ID:      elemID,
		Enabled: true, // WDA doesn't have separate enabled check
	}

	var (
		text      string
		elemName  string
		x, y, w, h int
		displayed bool
		textErr, rectErr, dispErr, nameErr error
		wg sync.WaitGroup
	)

	wg.Add(4)
	go func() {
		defer wg.Done()
		text, textErr = d.client.ElementText(elemID)
	}()
	go func() {
		defer wg.Done()
		x, y, w, h, rectErr = d.client.ElementRect(elemID)
	}()
	go func() {
		defer wg.Done()
		displayed, dispErr = d.client.ElementDisplayed(elemID)
	}()
	go func() {
		defer wg.Done()
		elemName, nameErr = d.client.ElementName(elemID)
	}()
	wg.Wait()

	if textErr == nil {
		info.Text = text
	}
	if rectErr == nil {
		info.Bounds = core.Bounds{X: x, Y: y, Width: w, Height: h}
	}
	if nameErr == nil {
		info.Class = elemName
	}
	info.Visible = dispErr == nil && displayed

	if dispErr == nil && !displayed {
		// XCUITest says off-screen. Check bounds geometrically — if they're
		// inside the viewport, override XCUITest and accept the element with
		// a MatchNote so the report records the override.
		if rectErr != nil {
			return nil, fmt.Errorf("element exists but is not visible on screen (no bounds)")
		}
		screenW, screenH, sErr := d.screenSize()
		if sErr != nil {
			return nil, fmt.Errorf("element exists but is not visible on screen (screen size unavailable)")
		}
		if info.Bounds.VisiblePercentage(screenW, screenH) < 0.1 {
			return nil, fmt.Errorf("element exists but is not visible on screen (bounds outside viewport)")
		}
		info.MatchNote = fmt.Sprintf(
			"XCUITest displayed=false but bounds (%d,%d %dx%d) within viewport %dx%d — proceeding",
			info.Bounds.X, info.Bounds.Y, info.Bounds.Width, info.Bounds.Height, screenW, screenH,
		)
		logger.Info("[wda] elemID=%s — %s", elemID, info.MatchNote)
	}

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

	// Filter out off-screen elements before resolving relative selectors
	if w, h, err := d.screenSize(); err == nil {
		allElements = FilterOutOfBounds(allElements, w, h)
	}

	return d.resolveRelativeSelector(sel, allElements)
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

	// Bounds-based visibility (FilterOutOfBounds already applied above);
	// XCUITest's `visible="false"` is unreliable on RN testID wrappers.
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no elements match selector")
	}

	// Prioritize clickable/interactive elements
	candidates = SortClickableFirst(candidates)

	selected := SelectByIndex(candidates, sel.Index)

	info := &core.ElementInfo{
		Text:    selected.Label,
		Bounds:  selected.Bounds,
		Enabled: selected.Enabled,
		Visible: selected.Displayed,
	}
	if note := rescueNote(selected); note != "" {
		info.MatchNote = note
		logger.Info("[wda] %s: %s", selectorLog(sel), note)
	}
	return info, nil
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

	// Filter out off-screen elements — page source XML includes elements
	// from the full accessibility tree, not just the visible viewport.
	if w, h, err := d.screenSize(); err == nil {
		allElements = FilterOutOfBounds(allElements, w, h)
	}

	candidates := FilterBySelector(allElements, sel)

	// XCUITest's `visible="false"` is unreliable on React Native testID-bearing
	// wrappers — it can return false even when bounds host visible sibling
	// content. We rely on bounds (already enforced by FilterOutOfBounds above)
	// rather than XCUITest's opinion. When accepting a visible="false" element
	// we tag a MatchNote so the report records the override (see rescueNote).
	if len(candidates) == 0 {
		if sel.Text != "" {
			if closest := ClosestTexts(allElements, sel.Text, 3); len(closest) > 0 {
				return nil, fmt.Errorf("no elements match selector; closest on-screen texts: %s", strings.Join(closest, ", "))
			}
		}
		return nil, fmt.Errorf("no elements match selector")
	}

	// Prioritize clickable/interactive elements
	candidates = SortClickableFirst(candidates)

	selected := SelectByIndex(candidates, sel.Index)

	// If element isn't a clickable type, try to find a clickable parent
	// This handles patterns where text labels aren't interactive but their containers are
	clickableElem := GetClickableElement(selected)

	info := &core.ElementInfo{
		Text:    selected.Label,
		Bounds:  clickableElem.Bounds,
		Enabled: selected.Enabled,
		Visible: selected.Displayed,
	}
	if note := rescueNote(selected); note != "" {
		info.MatchNote = note
		logger.Info("[wda] %s: %s", selectorLog(sel), note)
	}
	return info, nil
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
	return core.SuccessResult(msg, elem)
}

func errorResult(err error, msg string) *core.CommandResult {
	return core.ErrorResult(err, msg)
}
