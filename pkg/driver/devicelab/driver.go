// Package devicelab provides Android automation driver using the DeviceLab Android Driver.
// This is a WebSocket-based driver with on-device RPC for app lifecycle operations.
// Unlike the UIAutomator2 HTTP driver, element taps are always coordinate-based
// (FindAndClick) and app lifecycle uses on-device RPC instead of ADB shell.
package devicelab

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// ShellExecutor runs shell commands on a device.
// Implemented by device.AndroidDevice.
type ShellExecutor interface {
	Shell(cmd string) (string, error)
}

// DeviceLabClient defines the interface for DeviceLab driver operations.
// Implemented by maestro.Adapter. Includes RPC methods for app lifecycle
// that the UIA2 HTTP client does not provide.
type DeviceLabClient interface {
	// Element finding
	FindElement(strategy, selector string) (*uiautomator2.Element, error)
	FindAndClick(strategy, selector string) (*uiautomator2.Element, error)
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
	SendKeyActions(text string) error

	// Device state
	Screenshot() ([]byte, error)
	Source() (string, error)
	GetOrientation() (string, error)
	SetOrientation(orientation string) error
	GetClipboard() (string, error)
	SetClipboard(text string) error
	GetDeviceInfo() (*uiautomator2.DeviceInfo, error)

	// App lifecycle (RPC — no USB round-trips)
	LaunchApp(appID string, arguments map[string]interface{}) error
	ForceStop(appID string) error
	ClearAppData(appID string) error
	GrantPermissions(appID string, permissions []string) error

	// Settings
	SetAppiumSettings(settings map[string]interface{}) error

	// Settle detection
	WaitForSettle(timeoutMs, quietMs int) (bool, error)
}

// Driver implements core.Driver using the DeviceLab Android Driver.
type Driver struct {
	client DeviceLabClient
	info   *core.PlatformInfo
	device ShellExecutor // for ADB commands (fallback)

	// Parent context for element-finding operations (nil = context.Background())
	ctx context.Context

	// Timeouts (0 = use defaults)
	findTimeout         int // ms, for required elements
	optionalFindTimeout int // ms, for optional elements

	// Keyboard auto-dismiss: set after inputText/inputRandom, checked on next tap/assert
	lastStepWasInput bool

	// Cached values to avoid repeated ADB shell calls
	cachedAPILevel   int
	cachedActivities map[string]string // appID -> activity

	// CDP state from background push events (nil = not wired)
	cdpStateFunc func() *core.CDPInfo

	// WebView CDP connection manager (nil = not wired)
	webView        *webViewManager
	lastCDPScan    time.Time     // rate-limit ADB shell CDP scans
	lastCDPResult  *core.CDPInfo // cached result from last scan
	knownCDPType   string        // "browser" or "webview" — set from socket name, cleared on CDP down
}

// New creates a new DeviceLab driver.
func New(client DeviceLabClient, info *core.PlatformInfo, device ShellExecutor) *Driver {
	return &Driver{
		client: client,
		info:   info,
		device: device,
	}
}

// WaitForSettle delegates to the on-device agent's tree-comparison settle detection.
func (d *Driver) WaitForSettle(timeoutMs, quietMs int) (bool, error) {
	return d.client.WaitForSettle(timeoutMs, quietMs)
}

// SetCDPStateFunc sets the function used to retrieve real-time CDP socket state
// from the background monitor.
func (d *Driver) SetCDPStateFunc(fn func() *core.CDPInfo) {
	d.cdpStateFunc = fn
}

// SetWebViewForwarder enables WebView CDP support using the given ADB forwarder.
// When CDP sockets are detected, Rod will connect to the WebView for element finding.
func (d *Driver) SetWebViewForwarder(forwarder CDPForwarder) {
	d.webView = newWebViewManager(forwarder)
}

// Close cleans up resources (WebView CDP connection, ADB forwards).
func (d *Driver) Close() {
	if d.webView != nil {
		d.webView.disconnect()
	}
}

// CDPState returns the latest CDP socket state from push events.
// Implements core.CDPStateProvider.
func (d *Driver) CDPState() *core.CDPInfo {
	if d.cdpStateFunc != nil {
		return d.cdpStateFunc()
	}
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
// Returns context.Background() if no context was set.
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

// SetWaitForIdleTimeout sets the wait for idle timeout.
func (d *Driver) SetWaitForIdleTimeout(ms int) error {
	if ms < 0 {
		ms = 0
	}
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
	case *flow.OpenNotificationsStep:
		result = d.openNotifications(s)

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

	// WebView JS execution
	case *flow.EvalWebViewScriptStep:
		result = d.evalWebViewScript(s)
	case *flow.RunWebViewScriptStep:
		result = d.runWebViewScript(s)

	// Media
	case *flow.TakeScreenshotStep:
		result = d.takeScreenshot(s)
	case *flow.StartRecordingStep:
		result = d.startRecording(s)
	case *flow.StopRecordingStep:
		result = d.stopRecording(s)
	case *flow.RemoveMediaStep:
		result = d.removeMedia(s)
	case *flow.AddMediaStep:
		result = d.addMedia(s)

	default:
		result = &core.CommandResult{
			Success: false,
			Error:   fmt.Errorf("unknown step type: %T", step),
			Message: fmt.Sprintf("Step type '%T' is not supported", step),
		}
	}

	// Track whether this step was an input step (for keyboard auto-dismiss on next tap/assert)
	switch step.(type) {
	case *flow.InputTextStep, *flow.InputRandomStep:
		d.lastStepWasInput = result.Success
	default:
		d.lastStepWasInput = false
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

// ============================================================================
// WebView CDP Connection
// ============================================================================

// ensureWebViewConnection checks CDP state and connects/disconnects Rod as needed.
// Called at the top of find loops to keep the WebView connection in sync.
func (d *Driver) ensureWebViewConnection() {
	if d.webView == nil {
		return
	}

	cdpInfo := d.getCDPInfo()
	if cdpInfo == nil || !cdpInfo.Available {
		// CDP socket gone — disconnect if connected
		if d.webView.isConnected() {
			logger.Info("[cdp:3-connection] CDP socket gone, disconnecting webview")
			d.webView.disconnect()
		}
		d.knownCDPType = "" // Clear browser mode when CDP goes away
		return
	}

	// Determine CDP type from socket name — set immediately so isBrowserMode()
	// works even before the Rod connection is established.
	cdpType := "webview"
	if strings.HasPrefix(cdpInfo.Socket, "chrome_devtools_remote") ||
		strings.HasPrefix(cdpInfo.Socket, "chromium_devtools_remote") {
		cdpType = "browser"
	}
	d.knownCDPType = cdpType

	// CDP socket available — connect if not already.
	// The on-device agent filters sockets intelligently:
	//   - webview_devtools_remote_* sockets are always reported
	//   - chrome_devtools_remote sockets are only reported when a browser is in the foreground
	// So we trust whatever socket the agent sends us.
	if !d.webView.isConnected() {
		logger.Info("[cdp:3-connection] CDP socket available, initiating connection to %s (type=%s)", cdpInfo.Socket, cdpType)
		if err := d.webView.connect(cdpInfo, cdpType); err != nil {
			logger.Info("[cdp:3-connection] connect failed: %v (socket=%s)", err, cdpInfo.Socket)
			return
		}
	}
}

// getCDPInfo returns the current CDP socket info.
// Tries the push-event tracker first, then falls back to an ADB shell scan.
func (d *Driver) getCDPInfo() *core.CDPInfo {
	// Try push-event tracker first (instant, no ADB call).
	// The on-device agent already filters chrome sockets by foreground app,
	// so we trust whatever socket it reports.
	if d.cdpStateFunc != nil {
		if info := d.cdpStateFunc(); info != nil {
			logger.Info("[cdp:2-source] detected via push event: socket=%s", info.Socket)
			return info
		}
	}

	// Fallback: scan /proc/net/unix via ADB shell
	// This handles the case where the push event was missed (e.g., browser was
	// already open when the WebSocket connection was established).
	result := d.scanCDPSocket()
	if result != nil {
		logger.Info("[cdp:2-source] detected via ADB scan fallback: socket=%s", result.Socket)
	}
	return result
}

// scanCDPSocket scans /proc/net/unix on the device for CDP sockets.
// Rate-limited to at most once per 500ms to avoid excessive ADB calls.
// Returns the cached result between scans.
func (d *Driver) scanCDPSocket() *core.CDPInfo {
	if d.device == nil {
		return nil
	}

	// Rate-limit: return cached result if scanned recently
	if time.Since(d.lastCDPScan) < 500*time.Millisecond {
		return d.lastCDPResult
	}
	d.lastCDPScan = time.Now()

	output, err := d.device.Shell("cat /proc/net/unix")
	if err != nil {
		d.lastCDPResult = nil
		return nil
	}

	// Find CDP sockets. Prefer webview sockets over browser sockets.
	// Browser sockets (chrome_devtools_remote) are only returned when a browser
	// is actually the foreground app, to avoid connecting to background Chrome.
	var browserSocket string
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "devtools_remote") {
			continue
		}
		atIdx := strings.Index(line, "@")
		if atIdx < 0 {
			continue
		}
		socket := strings.TrimSpace(line[atIdx+1:])
		// Prefer webview sockets (embedded in the app under test)
		if strings.HasPrefix(socket, "webview_devtools_remote") {
			d.lastCDPResult = &core.CDPInfo{Available: true, Socket: socket}
			return d.lastCDPResult
		}
		// Remember browser socket as fallback
		if strings.HasPrefix(socket, "chrome_devtools_remote") || strings.HasPrefix(socket, "chromium_devtools_remote") {
			browserSocket = socket
		}
	}
	// Use browser socket only if a browser is actually in the foreground.
	// This prevents connecting to background Chrome when testing a native app.
	if browserSocket != "" && d.isBrowserForeground() {
		d.lastCDPResult = &core.CDPInfo{Available: true, Socket: browserSocket}
		return d.lastCDPResult
	}

	d.lastCDPResult = nil
	return nil
}

// knownBrowserPackages is the set of Android packages that are browsers.
// Matches the on-device Java agent's browser detection logic.
var knownBrowserPackages = map[string]bool{
	"com.android.chrome":        true,
	"com.chrome.beta":           true,
	"com.chrome.dev":            true,
	"com.chrome.canary":         true,
	"org.chromium.chrome":       true,
	"app.vanadium.browser":      true,
	"org.mozilla.firefox":       true,
	"org.mozilla.firefox_beta":  true,
	"com.opera.browser":         true,
	"com.opera.mini.native":     true,
	"com.brave.browser":         true,
	"com.microsoft.emmx":        true,
	"com.vivaldi.browser":       true,
	"com.sec.android.app.sbrowser": true,
}

// isBrowserForeground checks if the foreground app is a known browser.
// Used by scanCDPSocket to avoid connecting to background Chrome.
func (d *Driver) isBrowserForeground() bool {
	if d.device == nil {
		return false
	}
	// dumpsys window displays (SDK 29+) — cheapest way to get foreground activity
	output, err := d.device.Shell("dumpsys activity activities | grep topResumedActivity")
	if err != nil || output == "" {
		return false
	}
	// Output format: topResumedActivity=ActivityRecord{... u0 com.package.name/... t123}
	for pkg := range knownBrowserPackages {
		if strings.Contains(output, pkg) {
			return true
		}
	}
	return false
}

// findFocused returns the currently focused element as a core.Element.
// Tries Rod first (`:focus` selector), then native ActiveElement().
func (d *Driver) findFocused() (core.Element, error) {
	d.ensureWebViewConnection()

	// Try Rod first
	if d.webView != nil && d.webView.isConnected() {
		if elem, err := d.webView.findFocusedWeb(); err == nil {
			return elem, nil
		}
	}

	// Try native
	if d.webView == nil || d.webView.webViewType() != "browser" {
		active, err := d.client.ActiveElement()
		if err == nil {
			info := &core.ElementInfo{Visible: true, Enabled: true}
			if text, textErr := active.Text(); textErr == nil {
				info.Text = text
			}
			if rect, rectErr := active.Rect(); rectErr == nil {
				info.Bounds = core.Bounds{X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height}
			}
			return &NativeElement{elem: active, client: d.client, info: info}, nil
		}
	}

	return nil, fmt.Errorf("no focused element")
}

// isBrowserMode returns true if we know the CDP target is a Chrome browser (not an embedded WebView).
// Set from socket name when CDP socket is detected — does NOT require an active connection,
// so native strategies are skipped even while Rod is still connecting.
func (d *Driver) isBrowserMode() bool {
	return d.knownCDPType == "browser"
}

// ============================================================================
// Element Finding
// ============================================================================

// findElement finds an element using a selector with client-side polling.
// Tries multiple locator strategies in order until one succeeds.
// For relative selectors or regex patterns, uses page source parsing.
// Returns full element info including text and bounds (3 HTTP calls).
func (d *Driver) findElement(sel flow.Selector, optional bool, stepTimeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	return d.findElementWithOptions(sel, optional, stepTimeoutMs, false, false)
}

// findElementFast finds an element with minimal HTTP calls (1 call).
// Use for visibility checks where we only need to know element exists.
func (d *Driver) findElementFast(sel flow.Selector, optional bool, stepTimeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	return d.findElementWithOptions(sel, optional, stepTimeoutMs, false, true)
}

// findElementForTap finds an element for tap commands.
// DeviceLab behavior: non-clickable first (fastest match), then clickable fallback.
// No page source fallback for text-based selectors (UiAutomator strategies only).
func (d *Driver) findElementForTap(sel flow.Selector, optional bool, stepTimeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	// For relative selectors (below, above, etc.), use page source which handles them correctly
	if sel.HasRelativeSelector() {
		timeout := d.calculateTimeout(optional, stepTimeoutMs)
		ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
		defer cancel()
		return d.findElementRelativeWithContext(ctx, sel)
	}

	// Index selectors need page source (native APIs return single match)
	if sel.HasNonZeroIndex() {
		return d.findElementWithOptions(sel, optional, stepTimeoutMs, false, false)
	}

	// For ID-based selectors: skip clickable preference (IDs are unique, no disambiguation needed)
	if sel.ID != "" {
		return d.findElementWithOptions(sel, optional, stepTimeoutMs, false, false)
	}

	// For text-based selectors: use UiAutomator only (no page source fallback)
	// Non-clickable strategies first, then clickable fallback
	if sel.Text != "" {
		timeout := d.calculateTimeout(optional, stepTimeoutMs)
		ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
		defer cancel()
		return d.findElementDirectWithContext(ctx, sel)
	}

	// For other selectors, use standard approach
	return d.findElementWithOptions(sel, optional, stepTimeoutMs, false, false)
}

// findElementDirectWithContext finds an element for tap using only UiAutomator strategies.
// Non-clickable strategies first (fastest match), then clickable fallback. No page source.
func (d *Driver) findElementDirectWithContext(ctx context.Context, sel flow.Selector) (*uiautomator2.Element, *core.ElementInfo, error) {
	// Clickable first: for tap commands, prefer buttons over labels.
	// General strategies as fallback when no clickable element matches.
	clickableStrategies, _ := buildClickableOnlyStrategies(sel)
	allStrategies, _ := buildSelectors(sel, 0)
	combined := append(clickableStrategies, allStrategies...)

	// When text triggers regex detection, also add literal textMatches
	// as fallback. Handles false positives like "alice@example.com (locked out)" where
	// parentheses trigger regex detection but the text is actually literal.
	if looksLikeRegex(sel.Text) {
		escaped := escapeUIAutomatorString(sel.Text)
		stateFilters := buildStateFilters(sel)
		pattern := `(?is).*\Q` + escaped + `\E.*`
		combined = append(combined,
			LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().textMatches("` + pattern + `")` + stateFilters,
			},
			LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().descriptionMatches("` + pattern + `")` + stateFilters,
			},
		)
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
			// Keep WebView connection in sync (handles late-appearing WebViews)
			d.ensureWebViewConnection()

			// Try Rod/CDP first (one shot) — if WebView is connected
			if d.webView != nil && d.webView.isConnected() {
				cdpStart := time.Now()
				webElem, err := d.webView.findWebOnce(sel)
				cdpDur := time.Since(cdpStart)
				if err == nil {
					logger.Info("[CDP] found element via CDP: %s (%v)", sel.Describe(), cdpDur)
					return nil, webElem.Info(), nil
				}
				logger.Debug("[CDP] findWebOnce miss: %s (%v)", sel.Describe(), cdpDur)
			}

			// Browser mode: skip native strategies — all content is web
			if d.isBrowserMode() {
				lastErr = fmt.Errorf("element '%s' not found via CDP", sel.Describe())
				time.Sleep(100 * time.Millisecond)
				continue
			}

			elem, info, err := d.tryFindElement(combined)
			if err == nil {
				return elem, info, nil
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
		escaped := escapeUIAutomatorString(sel.Text)
		// Case-sensitive text/description/hint first, then case-insensitive fallback.
		// hintContains is a DeviceLab-agent extension — matches EditText android:hint
		// placeholder so "tapOn: 'Email'" finds an empty field by its hint text.
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().textContains("` + escaped + `").clickable(true)` + stateFilters,
		})
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().descriptionContains("` + escaped + `").clickable(true)` + stateFilters,
		})
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().hintContains("` + escaped + `").clickable(true)` + stateFilters,
		})
		ciPattern := `(?is).*\Q` + escaped + `\E.*`
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().textMatches("` + ciPattern + `").clickable(true)` + stateFilters,
		})
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().descriptionMatches("` + ciPattern + `").clickable(true)` + stateFilters,
		})
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().hintMatches("` + ciPattern + `").clickable(true)` + stateFilters,
		})
		// Fall back to regex match (case-insensitive) for partial/pattern matches
		if looksLikeRegex(sel.Text) {
			regexEscaped := escapeUIAutomator(sel.Text)
			pattern := "(?is)" + regexEscaped
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().textMatches("` + pattern + `").clickable(true)` + stateFilters,
			})
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().descriptionMatches("` + pattern + `").clickable(true)` + stateFilters,
			})
		}
	}

	if len(strategies) == 0 {
		return nil, fmt.Errorf("no text selector specified")
	}

	return strategies, nil
}

// findElementWithOptions is the internal implementation with clickable preference option.
func (d *Driver) findElementWithOptions(sel flow.Selector, optional bool, stepTimeoutMs int, preferClickable bool, fastMode bool) (*uiautomator2.Element, *core.ElementInfo, error) {
	timeout := d.calculateTimeout(optional, stepTimeoutMs)
	ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
	defer cancel()

	return d.findElementWithContext(ctx, sel, preferClickable, fastMode)
}

// findElementWithContext finds an element using context for deadline management.
func (d *Driver) findElementWithContext(ctx context.Context, sel flow.Selector, preferClickable bool, fastMode bool) (*uiautomator2.Element, *core.ElementInfo, error) {
	// Page source paths are native-only — skip in browser mode
	if !d.isBrowserMode() {
		// Handle relative selectors via page source (position calculation required)
		if sel.HasRelativeSelector() {
			return d.findElementRelativeWithContext(ctx, sel)
		}

		// Handle size selectors via page source (bounds calculation required)
		if sel.Width > 0 || sel.Height > 0 {
			return d.findElementByPageSourceWithContext(ctx, sel)
		}

		// Handle index selectors via page source (need all matches to pick Nth)
		if sel.HasNonZeroIndex() {
			return d.findElementByPageSourceWithContext(ctx, sel)
		}

		// Handle regex ID selectors via page source (UiAutomator calls are slow when element absent)
		if sel.ID != "" && looksLikeRegex(sel.ID) {
			return d.findElementByPageSourceWithContext(ctx, sel)
		}
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
			if !d.isBrowserMode() && sel.Text != "" {
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
			// Keep WebView connection in sync (handles late-appearing WebViews)
			d.ensureWebViewConnection()

			// Try Rod/CDP first (one shot, ~20ms) — if WebView is connected
			if d.webView != nil && d.webView.isConnected() {
				cdpStart := time.Now()
				webElem, err := d.webView.findWebOnce(sel)
				cdpDur := time.Since(cdpStart)
				if err == nil {
					logger.Info("[CDP] found element via CDP: %s (%v)", sel.Describe(), cdpDur)
					return nil, webElem.Info(), nil
				}
				logger.Debug("[CDP] findWebOnce miss: %s (%v)", sel.Describe(), cdpDur)
			}

			// Browser mode: skip native strategies — all content is web
			if d.isBrowserMode() {
				lastErr = fmt.Errorf("element '%s' not found via CDP", sel.Describe())
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Try native UiAutomator strategies
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
func (d *Driver) findElementOnce(sel flow.Selector) (*uiautomator2.Element, *core.ElementInfo, error) {
	// Keep WebView connection in sync
	d.ensureWebViewConnection()

	// Try Rod/CDP first (one shot) — if WebView is connected and not a special selector type
	if d.webView != nil && d.webView.isConnected() && !sel.HasRelativeSelector() {
		webElem, err := d.webView.findWebOnce(sel)
		if err == nil {
			logger.Info("[CDP] found element via CDP: %s", sel.Describe())
			return nil, webElem.Info(), nil
		}
		logger.Debug("[CDP] findWebOnce miss in findElementOnce: %s — %v", sel.Describe(), err)
	}

	// Browser mode: skip all native strategies — all content is web
	if d.isBrowserMode() {
		return nil, nil, fmt.Errorf("element '%s' not found via CDP", sel.Describe())
	}

	// Handle relative selectors with single page source fetch
	if sel.HasRelativeSelector() {
		return d.findElementRelativeOnce(sel)
	}

	// Handle size selectors with single page source fetch
	if sel.Width > 0 || sel.Height > 0 {
		return d.findElementByPageSourceOnce(sel)
	}

	// Handle index selectors with single page source fetch
	if sel.HasNonZeroIndex() {
		return d.findElementByPageSourceOnce(sel)
	}

	// Handle regex ID selectors with page source
	if sel.ID != "" && looksLikeRegex(sel.ID) {
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
func (d *Driver) findElementQuick(sel flow.Selector, timeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	return d.findElementOnce(sel)
}

// tryFindElementFast attempts to find element using given strategies (single attempt).
// Returns minimal info - just element ID and visible=true. No extra HTTP calls.
func (d *Driver) tryFindElementFast(strategies []LocatorStrategy) (*uiautomator2.Element, *core.ElementInfo, error) {
	var lastErr error
	for _, s := range strategies {
		elem, err := d.client.FindElement(s.Strategy, s.Value)
		if err != nil {
			lastErr = err
			continue
		}

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

// ============================================================================
// Relative Element Finding
// ============================================================================

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
		}
	}
}

// findElementRelativeOnce performs a single attempt to find element with relative selector.
func (d *Driver) findElementRelativeOnce(sel flow.Selector) (*uiautomator2.Element, *core.ElementInfo, error) {
	info, err := d.resolveRelativeSelector(sel)
	if err != nil {
		return nil, nil, err
	}
	return nil, info, nil
}

// resolveRelativeSelector resolves a relative selector with a single page source fetch.
func (d *Driver) resolveRelativeSelector(sel flow.Selector) (*core.ElementInfo, error) {
	anchorSelector, filterType := getRelativeFilter(sel)

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

	pageSource, err := d.client.Source()
	if err != nil {
		return nil, fmt.Errorf("failed to get page source: %w", err)
	}

	allElements, err := ParsePageSource(pageSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse page source: %w", err)
	}

	var candidates []*ParsedElement
	if baseSel.Text != "" || baseSel.ID != "" || baseSel.Width > 0 || baseSel.Height > 0 {
		candidates = FilterBySelector(allElements, baseSel)
	} else {
		candidates = allElements
	}

	var anchors []*ParsedElement
	if anchorSelector != nil {
		_, anchorFilterType := getRelativeFilter(*anchorSelector)
		if anchorFilterType != filterNone {
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
			anchors = FilterBySelector(allElements, *anchorSelector)
		}
	}

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

	if len(sel.ContainsDescendants) > 0 {
		candidates = FilterContainsDescendants(candidates, allElements, sel.ContainsDescendants)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no elements match relative criteria")
	}

	candidates = SortClickableFirst(candidates)

	selected := SelectByIndex(candidates, sel.Index)

	clickableElem := GetClickableElement(selected)

	return &core.ElementInfo{
		Text:    selected.Text,
		Bounds:  clickableElem.Bounds,
		Enabled: selected.Enabled,
		Visible: selected.Displayed,
	}, nil
}

// findElementRelativeWithElements resolves a relative selector using pre-parsed elements.
func (d *Driver) findElementRelativeWithElements(sel flow.Selector, allElements []*ParsedElement) (*uiautomator2.Element, *core.ElementInfo, error) {
	anchorSelector, filterType := getRelativeFilter(sel)

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

	var candidates []*ParsedElement
	if baseSel.Text != "" || baseSel.ID != "" || baseSel.Width > 0 || baseSel.Height > 0 {
		candidates = FilterBySelector(allElements, baseSel)
	} else {
		candidates = allElements
	}

	var anchors []*ParsedElement
	if anchorSelector != nil {
		_, anchorFilterType := getRelativeFilter(*anchorSelector)
		if anchorFilterType != filterNone {
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
			anchors = FilterBySelector(allElements, *anchorSelector)
		}
	}

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

	if len(sel.ContainsDescendants) > 0 {
		candidates = FilterContainsDescendants(candidates, allElements, sel.ContainsDescendants)
	}

	if len(candidates) == 0 {
		return nil, nil, fmt.Errorf("no elements match relative criteria")
	}

	candidates = SortClickableFirst(candidates)

	selected := SelectByIndex(candidates, sel.Index)

	clickableElem := GetClickableElement(selected)

	info := &core.ElementInfo{
		Text:    selected.Text,
		Bounds:  clickableElem.Bounds,
		Enabled: selected.Enabled,
		Visible: selected.Displayed,
	}

	return nil, info, nil
}

// ============================================================================
// Page Source Element Finding
// ============================================================================

// findElementByPageSourceOnce performs a single page source search without polling.
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

	if len(candidates) == 0 {
		return nil, nil, fmt.Errorf("no elements match selector")
	}

	selected := SelectByIndex(candidates, sel.Index)

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
		}
	}
}

// findElementByPageSourceOnceInternal performs a single page source search.
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

	selected := SelectByIndex(candidates, sel.Index)

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

// ============================================================================
// Selector Building
// ============================================================================

// LocatorStrategy represents a single locator strategy with its value.
type LocatorStrategy struct {
	Strategy string
	Value    string
}

// Element finding timeouts (milliseconds).
const (
	DefaultFindTimeout  = 12000 // 12 seconds for required elements
	OptionalFindTimeout = 7000  // 7 seconds for optional elements
	QuickFindTimeout    = 1000  // 1 second for quick checks
)

// buildSelectors converts a Maestro Selector to UIAutomator2 locator strategies.
func buildSelectors(sel flow.Selector, timeoutMs int) ([]LocatorStrategy, error) {
	return buildSelectorsWithOptions(sel, timeoutMs, false)
}

// buildSelectorsForTap builds selectors that prioritize clickable elements.
func buildSelectorsForTap(sel flow.Selector, timeoutMs int) ([]LocatorStrategy, error) {
	return buildSelectorsWithOptions(sel, timeoutMs, true)
}

// buildSelectorsWithOptions builds selectors with optional clickable-first prioritization.
func buildSelectorsWithOptions(sel flow.Selector, timeoutMs int, preferClickable bool) ([]LocatorStrategy, error) {
	var strategies []LocatorStrategy
	stateFilters := buildStateFilters(sel)

	// ID-based selector — exact match FIRST, substring fallback ONLY if exact
	// fails. Mirrors the parallel fix in pkg/driver/uiautomator2: substring-
	// only `resourceIdMatches(".*X.*")` triggered internal scrolling and
	// returned a wrong element when the target id wasn't in the rendered
	// tree (lazy ListView with target offscreen). Web driver does the same
	// cascade — exact → testid → substring → name → aria-label.
	if sel.ID != "" {
		escaped := escapeUIAutomatorString(sel.ID)
		if preferClickable {
			// Exact match — clickable first for tap commands.
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().resourceId("` + escaped + `").clickable(true)` + stateFilters,
			})
		}
		// Exact match — any element.
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().resourceId("` + escaped + `")` + stateFilters,
		})
		if preferClickable {
			// Substring fallback — clickable. Backward compat with users
			// relying on substring behaviour.
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().resourceIdMatches(".*` + escaped + `.*").clickable(true)` + stateFilters,
			})
		}
		// Substring fallback — any.
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().resourceIdMatches(".*` + escaped + `.*")` + stateFilters,
		})
	}

	// Text-based selector: case-sensitive first, case-insensitive fallback.
	// hintContains / hintMatches are DeviceLab-agent extensions — match EditText
	// android:hint placeholder so "tapOn: 'Email'" finds an empty field by hint.
	if sel.Text != "" {
		escaped := escapeUIAutomatorString(sel.Text)
		ciPattern := `(?is).*\Q` + escaped + `\E.*`
		if preferClickable {
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().textContains("` + escaped + `").clickable(true)` + stateFilters,
			})
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().descriptionContains("` + escaped + `").clickable(true)` + stateFilters,
			})
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().hintContains("` + escaped + `").clickable(true)` + stateFilters,
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
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().hintContains("` + escaped + `")` + stateFilters,
		})
		// Case-insensitive fallback
		if preferClickable {
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().textMatches("` + ciPattern + `").clickable(true)` + stateFilters,
			})
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().descriptionMatches("` + ciPattern + `").clickable(true)` + stateFilters,
			})
			strategies = append(strategies, LocatorStrategy{
				Strategy: uiautomator2.StrategyUIAutomator,
				Value:    `new UiSelector().hintMatches("` + ciPattern + `").clickable(true)` + stateFilters,
			})
		}
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().textMatches("` + ciPattern + `")` + stateFilters,
		})
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().descriptionMatches("` + ciPattern + `")` + stateFilters,
		})
		strategies = append(strategies, LocatorStrategy{
			Strategy: uiautomator2.StrategyUIAutomator,
			Value:    `new UiSelector().hintMatches("` + ciPattern + `")` + stateFilters,
		})
		// Fall back to regex match (case-insensitive) with proper escaping
		if looksLikeRegex(sel.Text) {
			regexEscaped := escapeUIAutomator(sel.Text)
			pattern := "(?is)" + regexEscaped
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
		}
	}

	// CSS selector for web views
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

// looksLikeRegex checks if text contains regex metacharacters.
func looksLikeRegex(text string) bool {
	for i := 0; i < len(text); i++ {
		c := text[i]
		if i > 0 && text[i-1] == '\\' {
			continue
		}
		switch c {
		case '.':
			if i+1 < len(text) {
				next := text[i+1]
				if next == '*' || next == '+' || next == '?' {
					return true
				}
			}
		case '*', '+', '?', '[', ']', '{', '}', '|', '(', ')':
			return true
		case '^':
			if i == 0 {
				return true
			}
		case '$':
			if i == len(text)-1 {
				return true
			}
		}
	}
	return false
}

// escapeUIAutomatorString escapes only the double quotes for UiAutomator string.
func escapeUIAutomatorString(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// buildStateFilters returns UiSelector chain for state filters.
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
	return core.SuccessResult(msg, elem)
}

func errorResult(err error, msg string) *core.CommandResult {
	return core.ErrorResult(err, msg)
}
