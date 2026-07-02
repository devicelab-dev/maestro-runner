// Package devicelab provides Android automation driver using the DeviceLab Android Driver.
// This is a WebSocket-based driver with on-device RPC for app lifecycle operations.
// Unlike the UIAutomator2 HTTP driver, element taps are always coordinate-based
// (FindAndClick) and app lifecycle uses on-device RPC instead of ADB shell.
package devicelab

import (
	"context"
	"fmt"
	"os"
	"strconv"
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

	// TreeHash returns a hash of the current accessibility tree. Used by
	// lazy retry: capture pre-tap, compare after a failing assertion to
	// detect "screen unchanged → tap had no effect → retry the tap".
	TreeHash() (uint64, error)

	// FindFirstOf tries multiple (strategy, selector) pairs in a single
	// RPC, returning the first match. Avoids paying the tree-fetch cost
	// once per strategy. Pairs are passed flat: [s1, sel1, s2, sel2, ...].
	FindFirstOf(strategiesAndSelectors []string) (*uiautomator2.Element, error)

	// WaitForWindowUpdate blocks up to timeoutMs for a window-content-changed
	// event on the target package. Returns true if updated, false if window
	// stayed stable for the full timeout. Mirrors Maestro's isWindowUpdating.
	WaitForWindowUpdate(appID string, timeoutMs int) (bool, error)
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
	cachedScreenW    int               // physical display width  (from `wm size`)
	cachedScreenH    int               // physical display height (from `wm size`)

	// CDP state from background push events (nil = not wired)
	cdpStateFunc func() *core.CDPInfo

	// WebView CDP connection manager (nil = not wired)
	webView       *webViewManager
	lastCDPScan   time.Time     // rate-limit ADB shell CDP scans
	lastCDPResult *core.CDPInfo // cached result from last scan
	knownCDPType  string        // "browser" or "webview" — set from socket name, cleared on CDP down

	// Lazy retry state: each successful tap captures the pre-tap tree hash
	// + selector + time. If the NEXT element-based command can't find its
	// target within lazyRetryProbeMs, we check if the screen has changed
	// since the tap (hash unchanged → tap had no effect → re-tap, up to
	// lazyRetryMax times). Zero overhead on success paths.
	lastTapHash     uint64
	lastTapSelector flow.Selector
	lastTapTime     time.Time
	lastTapRetries  int
}

// Lazy retry tunables. Window: how long after a tap we still consider it
// "recent enough" to retry. Probe: how long we let the next command poll
// before doing the hash check. Max: retry cap per original tap.
const (
	lazyRetryWindow  = 2 * time.Second
	lazyRetryProbeMs = 500
	lazyRetryMax     = 2
)

// lazyRetryEnabled gates the host-side lazy tap-retry. OFF by default.
//
// The retry fires when "tree hash unchanged since the tap AND the tap target
// is still findable", treating that as "the tap had no effect, re-issue it".
// That predicate cannot distinguish a genuinely dropped tap from a successful
// tap whose effect is async (submit-then-navigate) or that merely disables the
// source button without changing the hash — so it can re-issue a tap across a
// navigation boundary and land on the next screen's CTA (issue #95). Disabled
// by default; set MAESTRO_DEVICELAB_LAZY_RETRY=1 to opt back in.
var lazyRetryEnabled = func() bool {
	v := os.Getenv("MAESTRO_DEVICELAB_LAZY_RETRY")
	return v == "1" || strings.EqualFold(v, "true")
}()

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

// tapHadEffect (kept as dead code) — element-presence post-tap check.
// Currently NOT called from the tap path. Was problematic because some
// apps (React Navigation showcase) have buttons that persist across
// multiple screens, producing false negatives (tap navigated but source
// still findable on the next screen).
//
//nolint:unused
func (d *Driver) tapHadEffect() bool {
	if d.lastTapTime.IsZero() {
		return true
	}
	for i := 0; i < tapVerifyAttempts; i++ {
		_, _, err := d.findElementFast(d.lastTapSelector, true, 50)
		if err != nil {
			return true
		}
		if i < tapVerifyAttempts-1 {
			time.Sleep(tapVerifyInterval)
		}
	}
	return false
}

// tapHadEffectViaWindowUpdate uses Maestro's isWindowUpdating signal —
// asks the agent to subscribe to AccessibilityEvent stream and wait up
// to windowUpdateTimeoutMs for a window content-changed event on the
// app's package. Returns true if any event fires (tap caused something
// in the window), false if nothing fires (tap likely eaten).
//
// Currently NOT called from the tap path (commented out). Wired up so
// callers can opt in once we know it doesn't fire false positives /
// negatives on the React Navigation showcase suite.
//
//nolint:unused
func (d *Driver) tapHadEffectViaWindowUpdate(appID string) bool {
	if appID == "" {
		return true // no app context — assume effect (avoid false retry)
	}
	updated, err := d.client.WaitForWindowUpdate(appID, windowUpdateTimeoutMs)
	if err != nil {
		return true // can't verify — assume effect
	}
	return updated
}

const (
	tapVerifyAttempts     = 3 //nolint:unused
	tapVerifyInterval     = 30 * time.Millisecond //nolint:unused
	windowUpdateTimeoutMs = 500 //nolint:unused
)

// recordTap captures the current tree hash so a later failing assertion can
// detect "tap had no effect" (hash unchanged) and retry. Must be called
// BEFORE the click fires. Resets the retry counter to 0 — this is a new
// original tap, not a re-attempt.
func (d *Driver) recordTap(sel flow.Selector) {
	if !lazyRetryEnabled {
		return // lazy retry disabled — skip the per-tap TreeHash round-trip
	}
	hash, err := d.client.TreeHash()
	if err != nil {
		// If we can't read the hash, just clear state — lazy retry will skip.
		logger.Info("[devicelab] recordTap CLEAR (TreeHash error): %v on %s", err, sel.Describe())
		d.lastTapHash = 0
		d.lastTapTime = time.Time{}
		return
	}
	d.lastTapHash = hash
	d.lastTapSelector = sel
	d.lastTapTime = time.Now()
	d.lastTapRetries = 0
	logger.Info("[devicelab] recordTap SET hash=%x for %s", hash, sel.Describe())
}

// maybeLazyRetryTap is called from element-based commands (assertVisible,
// extendedWaitUntil, inputText — NOT assertNotVisible) after they've polled
// for lazyRetryProbeMs without finding the target. It checks if:
//   - a tap happened recently (within lazyRetryWindow)
//   - the tree hash hasn't changed since that tap (screen unchanged)
//   - we haven't exceeded lazyRetryMax for this tap
//
// If all true, it re-issues the prior tap and returns true so the caller
// knows to keep polling. Returns false otherwise (caller continues its
// normal failure path).
func (d *Driver) maybeLazyRetryTap() bool {
	if !lazyRetryEnabled {
		return false // lazy retry disabled by default (issue #95)
	}
	if d.lastTapTime.IsZero() {
		logger.Info("[devicelab] lazy retry skip: no recent tap recorded")
		return false
	}
	if time.Since(d.lastTapTime) > lazyRetryWindow {
		logger.Info("[devicelab] lazy retry skip: tap was %v ago (>%v)", time.Since(d.lastTapTime), lazyRetryWindow)
		return false
	}
	if d.lastTapRetries >= lazyRetryMax {
		logger.Info("[devicelab] lazy retry skip: hit max retries (%d)", d.lastTapRetries)
		return false
	}

	// Gate: the screen must be UNCHANGED since the tap. recordTap captured the
	// tree hash immediately before the click; if a fresh hash now differs, the
	// tap demonstrably caused something — an error message rendered, an async
	// submit resolved, a dialog opened, a partial transition began. Re-tapping
	// in that state is wrong: e.g. a failed-login flow keeps the login button
	// on screen (so the element-presence check below would wrongly fire) while
	// the "Invalid credentials" error renders — that render flips the hash, so
	// we correctly treat the original tap as effective and skip the retry.
	// A genuinely missed tap (fired at phantom off-screen bounds, nothing hit)
	// produces no ripple and leaves the hash identical, so real misses still
	// retry. On hash-read failure we fall through to the element check rather
	// than skip, preserving prior behaviour.
	if curHash, hashErr := d.client.TreeHash(); hashErr == nil && curHash != d.lastTapHash {
		logger.Info("[devicelab] lazy retry skip: tree hash changed since tap (%x→%x) — tap had effect", d.lastTapHash, curHash)
		return false
	}

	// Secondary signal: is the tap's target element STILL findable? If yes,
	// the tap clearly didn't navigate away from it → tap had no effect →
	// retry. Combined with the hash gate above this is precise: same screen
	// (hash unchanged) AND source button still present → the tap was eaten.
	// Animations, ripples, focus changes etc. do not remove the original
	// button — only a real navigation does.
	_, _, findErr := d.findElementFast(d.lastTapSelector, true, 50)
	if findErr != nil {
		// Source element gone → tap navigated → don't retry.
		logger.Info("[devicelab] lazy retry skip: tap target %s no longer findable — tap had effect", d.lastTapSelector.Describe())
		return false
	}
	logger.Info("[devicelab] lazy retry triggered: tap target %s still findable → tap had no effect", d.lastTapSelector.Describe())

	// Re-issue the tap via FindAndClick. Reset the timer so further probes
	// keep counting from this re-attempt, not the original.
	for _, s := range buildClickableOrAllStrategies(d.lastTapSelector) {
		if _, err := d.client.FindAndClick(s.Strategy, s.Value); err == nil {
			d.lastTapRetries++
			d.lastTapTime = time.Now()
			return true
		}
	}
	return false
}

// buildClickableOrAllStrategies returns the strategy chain used for a tap.
// Mirrors what tapOn's fast path does: clickable-first for text, fall back
// to non-clickable. Pure helper so maybeLazyRetryTap can re-issue the tap.
func buildClickableOrAllStrategies(sel flow.Selector) []LocatorStrategy {
	clickable, _ := buildClickableOnlyStrategies(sel)
	all, _ := buildSelectors(sel, 0)
	return append(clickable, all...)
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

// physicalScreenSize returns the full physical display size (e.g. 1080x2340 from
// `wm size`), cached for the session. Unlike screenSize() — which returns the
// on-device-reported USABLE size (it can exclude the status bar, e.g. 1080x2204) —
// this matches the coordinate space of the accessibility hierarchy (whose root spans
// the full display), so it is the correct reference for on-screen geometry checks
// against element bounds. Returns ok=false if it cannot be determined.
func (d *Driver) physicalScreenSize() (int, int, bool) {
	if d.cachedScreenW > 0 && d.cachedScreenH > 0 {
		return d.cachedScreenW, d.cachedScreenH, true
	}
	if d.device == nil {
		return 0, 0, false
	}
	out, err := d.device.Shell("wm size")
	if err != nil {
		return 0, 0, false
	}
	w, h := parseWmSize(out)
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	d.cachedScreenW, d.cachedScreenH = w, h
	return w, h, true
}

// tappableScreenSize returns the screen size used to validate element bounds: the
// full physical display (same coordinate space as the accessibility hierarchy that
// produced the bounds), falling back to the on-device-reported size when the
// physical size is unavailable.
func (d *Driver) tappableScreenSize() (int, int, error) {
	if w, h, ok := d.physicalScreenSize(); ok {
		return w, h, nil
	}
	return d.screenSize()
}

// parseWmSize extracts width x height from `wm size` output. It prefers an
// "Override size" line (the effective display size when one is set) over the
// "Physical size" line.
func parseWmSize(out string) (int, int) {
	var pw, ph, ow, oh int
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Physical size:"):
			pw, ph = parseWxH(strings.TrimPrefix(line, "Physical size:"))
		case strings.HasPrefix(line, "Override size:"):
			ow, oh = parseWxH(strings.TrimPrefix(line, "Override size:"))
		}
	}
	if ow > 0 && oh > 0 {
		return ow, oh
	}
	return pw, ph
}

// parseWxH parses "1080x2340" (tolerating surrounding spaces) into width, height.
func parseWxH(s string) (int, int) {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, 'x')
	if i <= 0 {
		return 0, 0
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(s[:i]))
	h, err2 := strconv.Atoi(strings.TrimSpace(s[i+1:]))
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return w, h
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
	"com.android.chrome":           true,
	"com.chrome.beta":              true,
	"com.chrome.dev":               true,
	"com.chrome.canary":            true,
	"org.chromium.chrome":          true,
	"app.vanadium.browser":         true,
	"org.mozilla.firefox":          true,
	"org.mozilla.firefox_beta":     true,
	"com.opera.browser":            true,
	"com.opera.mini.native":        true,
	"com.brave.browser":            true,
	"com.microsoft.emmx":           true,
	"com.vivaldi.browser":          true,
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

// findElementFastWithLazyRetry wraps findElementFast with lazy retry of a
// preceding tap. Tries the find for lazyRetryProbeMs first; if not found,
// asks maybeLazyRetryTap whether to re-issue the prior tap; loops up to
// lazyRetryMax times. Falls through to the standard timeout if hash has
// changed (real "element not visible") or if no recent tap is recorded.
func (d *Driver) findElementFastWithLazyRetry(sel flow.Selector, optional bool, stepTimeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	logger.Info("[devicelab] findElementFastWithLazyRetry start for %s (lastTapTime zero=%v)", sel.Describe(), d.lastTapTime.IsZero())
	if elem, info, err := d.findElementFast(sel, optional, lazyRetryProbeMs); err == nil {
		return elem, info, nil
	}
	logger.Info("[devicelab] findElementFastWithLazyRetry first probe failed, calling maybeLazyRetryTap")
	for d.maybeLazyRetryTap() {
		logger.Info("[devicelab] lazy retry: re-issued tap on %s after assertion probe failed", d.lastTapSelector.Describe())
		if elem, info, err := d.findElementFast(sel, optional, lazyRetryProbeMs); err == nil {
			return elem, info, nil
		}
	}
	logger.Info("[devicelab] findElementFastWithLazyRetry falling through to full timeout for %s", sel.Describe())
	return d.findElementFast(sel, optional, stepTimeoutMs)
}

// findElementWithLazyRetry is the same wrapper for the standard (3-call)
// findElement path. Used by inputText, where the next step actually consumes
// the element (vs. assertVisible which just checks visibility).
func (d *Driver) findElementWithLazyRetry(sel flow.Selector, optional bool, stepTimeoutMs int) (*uiautomator2.Element, *core.ElementInfo, error) {
	if elem, info, err := d.findElement(sel, optional, lazyRetryProbeMs); err == nil {
		return elem, info, nil
	}
	for d.maybeLazyRetryTap() {
		logger.Info("[devicelab] lazy retry: re-issued tap on %s before inputText", d.lastTapSelector.Describe())
		if elem, info, err := d.findElement(sel, optional, lazyRetryProbeMs); err == nil {
			return elem, info, nil
		}
	}
	return d.findElement(sel, optional, stepTimeoutMs)
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
		// (Tried exact-match-first à la Maestro but it broke too many flows
		// where users expect substring matching for tab labels with counts.)
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
		// Fall back to regex match (case-insensitive) for partial/pattern matches.
		// looksLikeRegex(text) was true → text IS a regex. Pass it through
		// unmodified (only escape Java-string quotes); do NOT escape regex
		// metachars, or `.*For You.*` becomes `\.\*For You\.\*` which matches
		// the literal string ".*For You.*" instead of "anything around For You".
		if looksLikeRegex(sel.Text) {
			regexEscaped := escapeUIAutomatorString(sel.Text)
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
//
// We tried batching all strategies into UI.findFirstOf (~6× faster per call)
// but it regressed transient-element detection. The reason: 6 sequential
// per-strategy RPCs produce 6 fresh tree snapshots spread across ~900ms,
// which is what gives RN's brief mid-animation states multiple chances to
// be caught. The batched call (even with implicit-wait polling) produces
// fewer snapshot moments per call. The wire infrastructure (FindFirstOf)
// is left in place for future re-enabling when polling parity is sorted.
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
		// Fall back to regex match (case-insensitive). Text is already a regex
		// per looksLikeRegex — use it as-is; only escape Java-string quotes.
		// Escaping regex metachars here would defeat the regex (turns `.*` into
		// `\.\*` which matches the literal string ".*").
		if looksLikeRegex(sel.Text) {
			regexEscaped := escapeUIAutomatorString(sel.Text)
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
