package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	defaultFindTimeoutMs        = 17000
	defaultActionableTimeoutMs  = 2000 // brief window to wait for the actionable gate to pass post-find
	optionalFindTimeoutMs = 7000
	defaultViewportW      = 1280
	defaultViewportH      = 800
)

// Config holds browser driver configuration.
type Config struct {
	Headless    bool
	URL         string // Initial URL to navigate to
	ChromeBin   string // Path to Chrome binary (empty = auto-download)
	Browser     string // "chrome", "chromium", or path to binary (default: chromium)
	UserDataDir string // Persistent profile directory (empty = ephemeral profile)
	ViewportW   int
	ViewportH   int
}

// Driver implements core.Driver for desktop browser testing using Rod + CDP.
type Driver struct {
	browser *rod.Browser
	page    *rod.Page
	config  Config

	findTimeoutMs int
	clipboard     string
	viewportW     int
	viewportH     int

	// Dialog handling
	dialogCh chan *proto.PageJavascriptDialogOpening
	stopCh   chan struct{}

	// Tab management
	tabLabels map[string]*rod.Page // label → page mapping

	// Network interception
	networkMocks  []networkMock // active mock rules
	networkBlocks []string      // blocked URL patterns
	fetchEnabled  bool          // whether Fetch domain is enabled
	networkMu     sync.Mutex    // protects mocks/blocks

	// Network idle tracking (for waitForPageReady)
	network *networkTracker

	// Console capture
	consoleLogs []ConsoleEntry
	consoleMu   sync.Mutex

	// Selector validation dedup
	warnedFields map[string]bool

	closeOnce sync.Once
	closeErr  error
}

// New creates a new browser Driver.
func New(cfg Config) (*Driver, error) {
	if cfg.ViewportW == 0 {
		cfg.ViewportW = defaultViewportW
	}
	if cfg.ViewportH == 0 {
		cfg.ViewportH = defaultViewportH
	}

	// Cache downloaded browsers in ~/.maestro-runner/browsers/
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		launcher.DefaultBrowserDir = filepath.Join(homeDir, ".maestro-runner", "browsers")
	}

	// Resolve which browser binary to use.
	// Priority: ChromeBin (direct path) > Browser flag > default (chromium/download)
	chromeBin := resolveBrowserBin(cfg)

	// Use a clean profile with password manager disabled
	l := launcher.New().Headless(cfg.Headless).
		Set("no-first-run").
		Set("disable-default-apps").
		Set("disable-popup-blocking").
		Set("disable-translate").
		Set("disable-background-timer-throttling").
		Set("disable-component-update").
		Set("password-store", "basic")
	// Chrome's setuid sandbox relies on user namespaces, which CI runners
	// (e.g. GitHub Actions) restrict — causing the zygote to abort on launch.
	// Disable the sandbox only under CI or when explicitly requested.
	if os.Getenv("CI") != "" || os.Getenv("MAESTRO_NO_SANDBOX") != "" {
		l = l.Set("no-sandbox")
	}
	if cfg.UserDataDir != "" {
		// Persistent profile: cookies / localStorage / extensions survive
		// across runs. Caller is responsible for the directory's lifecycle.
		l = l.Set("user-data-dir", cfg.UserDataDir)
	}
	if chromeBin != "" {
		l = l.Bin(chromeBin)
	} else {
		// Using Rod's bundled Chromium — check if download is needed
		browserDir := launcher.DefaultBrowserDir
		if needsDownload(browserDir) {
			log.Printf("[browser] Downloading Chromium (first time only, subsequent runs will be faster)...")
		}
	}

	// Write Chrome preferences to disable password manager and breach detection
	if dataDir := l.Get("user-data-dir"); dataDir != "" {
		writeChromePref(dataDir)
	}

	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	// Connect with NoDefaultDevice so we control the viewport
	browser := rod.New().ControlURL(controlURL).NoDefaultDevice()
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to browser: %w", err)
	}

	// Create a new page
	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %w", err)
	}

	// Set viewport
	err = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  cfg.ViewportW,
		Height: cfg.ViewportH,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set viewport: %w", err)
	}

	// Inject JS helper (persists across navigations)
	_, err = page.EvalOnNewDocument(jsHelperCode)
	if err != nil {
		return nil, fmt.Errorf("failed to inject JS helper: %w", err)
	}

	d := &Driver{
		browser:       browser,
		page:          page,
		config:        cfg,
		findTimeoutMs: defaultFindTimeoutMs,
		viewportW:     cfg.ViewportW,
		viewportH:     cfg.ViewportH,
		dialogCh:      make(chan *proto.PageJavascriptDialogOpening, 10),
		stopCh:        make(chan struct{}),
		tabLabels:     make(map[string]*rod.Page),
		warnedFields:  make(map[string]bool),
	}

	// Start background handlers
	d.startDialogHandler()
	d.startConsoleHandler()
	d.setupNetworkTracking(page)

	// Navigate to initial URL if provided
	if cfg.URL != "" {
		if err := page.Navigate(cfg.URL); err != nil {
			return nil, fmt.Errorf("failed to navigate to %s: %w", cfg.URL, err)
		}
		page.MustWaitLoad()
		p := page.Timeout(5 * time.Second)
		_ = p.WaitDOMStable(300*time.Millisecond, 0)
		d.network.waitForIdle(5*time.Second, 500*time.Millisecond)
	}

	return d, nil
}

// startDialogHandler starts a background goroutine to capture and auto-dismiss dialog events.
// Native JS dialogs (alert/confirm/prompt) block all CDP communication until handled.
// Uses Rod's EachEvent pattern — the goroutine blocks until the browser closes.
func (d *Driver) startDialogHandler() {
	go d.page.EachEvent(func(e *proto.PageJavascriptDialogOpening) bool {
		// Capture event for explicit acceptAlert/dismissAlert steps
		select {
		case d.dialogCh <- e:
		default:
			// Channel full — drop oldest
			select {
			case <-d.dialogCh:
			default:
			}
			d.dialogCh <- e
		}

		// Auto-accept the dialog to unblock CDP communication.
		// Tests can still use acceptAlert/dismissAlert for explicit handling.
		err := proto.PageHandleJavaScriptDialog{Accept: true, PromptText: ""}.Call(d.page)
		if err != nil {
			log.Printf("[browser] failed to auto-dismiss dialog (%s): %v", e.Type, err)
		}

		return false // keep listening
	})()
}

// writeChromePref writes a Chrome Preferences file that disables password manager
// and breach detection popups. Must be called before Launch().
func writeChromePref(userDataDir string) {
	defaultDir := filepath.Join(userDataDir, "Default")
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		return
	}
	prefs := `{
  "credentials_enable_service": false,
  "profile": {
    "password_manager_enabled": false,
    "password_manager_leak_detection": false
  },
  "password_manager": {
    "leak_detection": false
  }
}`
	_ = os.WriteFile(filepath.Join(defaultDir, "Preferences"), []byte(prefs), 0o644)
}

// detectChrome returns the path to an installed Chrome/Chromium binary, or empty string.
func detectChrome() string {
	var paths []string
	switch runtime.GOOS {
	case "darwin":
		paths = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "linux":
		paths = []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		}
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// resolveBrowserBin determines which browser binary to use based on config.
// Returns empty string to let Rod download/use its bundled Chromium.
func resolveBrowserBin(cfg Config) string {
	// Explicit ChromeBin takes highest priority (programmatic use)
	if cfg.ChromeBin != "" {
		return cfg.ChromeBin
	}

	browser := strings.TrimSpace(cfg.Browser)
	switch strings.ToLower(browser) {
	case "chrome":
		// User explicitly requested installed Chrome
		if path := detectChrome(); path != "" {
			log.Printf("[browser] Using installed Chrome: %s", path)
			return path
		}
		log.Printf("[browser] Chrome not found, falling back to Chromium")
		return "" // fall back to Rod download
	case "", "chromium":
		// Default: use Rod's bundled Chromium (download if needed)
		return ""
	default:
		// Treat as a custom path to a browser binary
		if _, err := os.Stat(browser); err == nil {
			log.Printf("[browser] Using custom browser: %s", browser)
			return browser
		}
		log.Printf("[browser] Browser binary not found at %s, falling back to Chromium", browser)
		return ""
	}
}

// EnsureBrowser ensures the browser binary is available, downloading Chromium
// if needed. Call this once before creating multiple parallel drivers to avoid
// N simultaneous downloads on first run.
func EnsureBrowser(cfg Config) error {
	if resolveBrowserBin(cfg) != "" {
		return nil // using a local binary, no download needed
	}

	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		launcher.DefaultBrowserDir = filepath.Join(homeDir, ".maestro-runner", "browsers")
	}

	b := launcher.NewBrowser()
	b.RootDir = launcher.DefaultBrowserDir

	if needsDownload(b.RootDir) {
		log.Printf("[browser] Downloading Chromium (first time only, subsequent runs will be faster)...")
	}

	_, err := b.Get()
	return err
}

// needsDownload checks if the browser cache directory is empty or missing.
func needsDownload(browserDir string) bool {
	entries, err := os.ReadDir(browserDir)
	if err != nil {
		return true // directory doesn't exist
	}
	return len(entries) == 0
}

// Close shuts down the browser. Safe to call multiple times.
func (d *Driver) Close() error {
	d.closeOnce.Do(func() {
		close(d.stopCh)
		if d.browser != nil {
			d.closeErr = d.browser.Close()
		}
	})
	return d.closeErr
}

// networkTracker tracks in-flight network requests via CDP Network domain events.
// Used to detect network idle state after navigations (SPAs with AJAX/fetch).
type networkTracker struct {
	mu       sync.Mutex
	inflight map[proto.NetworkRequestID]struct{}
	lastIdle time.Time
}

func newNetworkTracker() *networkTracker {
	return &networkTracker{
		inflight: make(map[proto.NetworkRequestID]struct{}),
		lastIdle: time.Now(),
	}
}

func (t *networkTracker) onRequest(id proto.NetworkRequestID, url string, resourceType proto.NetworkResourceType) {
	// Skip WebSocket connections (never fire loadingFinished) and data: URLs
	if resourceType == proto.NetworkResourceTypeWebSocket ||
		resourceType == proto.NetworkResourceTypeEventSource ||
		strings.HasPrefix(url, "data:") {
		return
	}
	t.mu.Lock()
	t.inflight[id] = struct{}{}
	t.mu.Unlock()
}

func (t *networkTracker) onComplete(id proto.NetworkRequestID) {
	t.mu.Lock()
	if _, ok := t.inflight[id]; ok {
		delete(t.inflight, id)
		if len(t.inflight) == 0 {
			t.lastIdle = time.Now()
		}
	}
	t.mu.Unlock()
}

// waitForIdle waits until no network requests are in-flight for quietPeriod.
// Returns true if idle was reached, false if timeout expired (proceeds anyway).
func (t *networkTracker) waitForIdle(timeout, quietPeriod time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		t.mu.Lock()
		idle := len(t.inflight) == 0
		idleSince := t.lastIdle
		t.mu.Unlock()
		if idle && time.Since(idleSince) >= quietPeriod {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// setupNetworkTracking enables the CDP Network domain and subscribes to request
// lifecycle events on the given page. Replaces any previous tracker.
func (d *Driver) setupNetworkTracking(page *rod.Page) {
	tracker := newNetworkTracker()
	d.network = tracker

	if err := (proto.NetworkEnable{}).Call(page); err != nil {
		log.Printf("[browser] failed to enable Network domain: %v", err)
		return
	}

	go page.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) bool {
			url := ""
			if e.Request != nil {
				url = e.Request.URL
			}
			tracker.onRequest(e.RequestID, url, e.Type)
			select {
			case <-d.stopCh:
				return true
			default:
				return false
			}
		},
		func(e *proto.NetworkLoadingFinished) bool {
			tracker.onComplete(e.RequestID)
			select {
			case <-d.stopCh:
				return true
			default:
				return false
			}
		},
		func(e *proto.NetworkLoadingFailed) bool {
			tracker.onComplete(e.RequestID)
			select {
			case <-d.stopCh:
				return true
			default:
				return false
			}
		},
	)()
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

	// App lifecycle (URL-based for browser)
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

	// Wait commands
	case *flow.WaitUntilStep:
		result = d.waitUntil(s)
	case *flow.WaitForAnimationToEndStep:
		result = d.waitForAnimationToEnd(s)

	// Media
	case *flow.TakeScreenshotStep:
		result = d.takeScreenshot(s)

	// Alert handling
	case *flow.AcceptAlertStep:
		result = d.acceptAlert(s)
	case *flow.DismissAlertStep:
		result = d.dismissAlert(s)

	// Browser scripting
	case *flow.EvalBrowserScriptStep:
		result = d.evalBrowserScript(s)
	case *flow.RunBrowserScriptStep:
		result = d.runBrowserScript(s)

	// Console capture
	case *flow.GetConsoleLogsStep:
		result = d.getConsoleLogs()
	case *flow.ClearConsoleLogsStep:
		result = d.clearConsoleLogs()
	case *flow.AssertNoJSErrorsStep:
		result = d.assertNoJSErrors()

	// Browser state (cookies, auth)
	case *flow.SetCookiesStep:
		result = d.setCookies(s)
	case *flow.GetCookiesStep:
		result = d.getCookies(s)
	case *flow.SaveAuthStateStep:
		result = d.saveAuthState(s)
	case *flow.LoadAuthStateStep:
		result = d.loadAuthState(s)

	// File & permissions
	case *flow.UploadFileStep:
		result = d.uploadFile(s)
	case *flow.WaitForDownloadStep:
		result = d.waitForDownload(s)
	case *flow.GrantPermissionsStep:
		result = d.grantPermissions(s)
	case *flow.ResetPermissionsStep:
		result = d.resetPermissions()

	// Tab management
	case *flow.OpenTabStep:
		result = d.openTab(s)
	case *flow.SwitchTabStep:
		result = d.switchTab(s)
	case *flow.CloseTabStep:
		result = d.closeTab()

	// Network interception
	case *flow.MockNetworkStep:
		result = d.mockNetwork(s)
	case *flow.BlockNetworkStep:
		result = d.blockNetwork(s)
	case *flow.SetNetworkConditionsStep:
		result = d.setNetworkConditions(s)
	case *flow.WaitForRequestStep:
		result = d.waitForRequest(s)
	case *flow.ClearNetworkMocksStep:
		result = d.clearNetworkMocks()

	// Unsupported — mobile-only or not applicable to web
	case *flow.SetAirplaneModeStep, *flow.ToggleAirplaneModeStep:
		result = unsupportedResult("airplane mode is not supported on web platform")
	case *flow.TravelStep:
		result = unsupportedResult("travel is not supported on web platform")
	case *flow.AddMediaStep:
		result = unsupportedResult("addMedia is not supported on web platform")
	case *flow.StartRecordingStep:
		result = unsupportedResult("startRecording is not supported on web platform")
	case *flow.StopRecordingStep:
		result = unsupportedResult("stopRecording is not supported on web platform")
	case *flow.ClearKeychainStep:
		result = unsupportedResult("clearKeychain is not supported on web platform")
	case *flow.SetPermissionsStep:
		result = unsupportedResult("setPermissions is not supported on web platform")
	case *flow.AssertNoDefectsWithAIStep:
		result = unsupportedResult("assertNoDefectsWithAI is not supported on web platform")
	case *flow.AssertWithAIStep:
		result = unsupportedResult("assertWithAI is not supported on web platform")
	case *flow.ExtractTextWithAIStep:
		result = unsupportedResult("extractTextWithAI is not supported on web platform")

	default:
		result = &core.CommandResult{
			Success: false,
			Error:   fmt.Errorf("unknown step type: %T", step),
			Message: fmt.Sprintf("Step type '%T' is not supported on web platform", step),
		}
	}

	result.Duration = time.Since(start)
	return result
}

// Screenshot captures the current page as PNG.
func (d *Driver) Screenshot() ([]byte, error) {
	return d.page.Screenshot(true, nil)
}

// Hierarchy returns the full accessibility tree as JSON.
func (d *Driver) Hierarchy() ([]byte, error) {
	result, err := proto.AccessibilityGetFullAXTree{}.Call(d.page)
	if err != nil {
		return nil, fmt.Errorf("failed to get AX tree: %w", err)
	}
	return json.Marshal(result.Nodes)
}

// GetState returns the current browser state.
func (d *Driver) GetState() *core.StateSnapshot {
	orientation := "portrait"
	if d.viewportW > d.viewportH {
		orientation = "landscape"
	}
	return &core.StateSnapshot{
		Orientation:   orientation,
		ClipboardText: d.clipboard,
	}
}

// GetPlatformInfo returns platform information.
func (d *Driver) GetPlatformInfo() *core.PlatformInfo {
	return &core.PlatformInfo{
		Platform:     "web",
		DeviceName:   "Chrome",
		IsSimulator:  true,
		ScreenWidth:  d.viewportW,
		ScreenHeight: d.viewportH,
	}
}

// SetFindTimeout sets the default timeout for finding elements.
func (d *Driver) SetFindTimeout(ms int) {
	if ms > 0 {
		d.findTimeoutMs = ms
	}
}

// SetWaitForIdleTimeout is a no-op for browser since Rod handles waits differently.
func (d *Driver) SetWaitForIdleTimeout(ms int) error {
	return nil
}

// SetContext is a no-op for browser — Rod waits use their own timeouts.
func (d *Driver) SetContext(ctx context.Context) {}

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

// unsupportedResult creates an error result for unsupported commands.
func unsupportedResult(msg string) *core.CommandResult {
	return &core.CommandResult{
		Success: false,
		Error:   fmt.Errorf("%s", msg),
		Message: msg,
	}
}

// ConsoleEntry represents a captured browser console message or JS exception.
type ConsoleEntry struct {
	Level   string `json:"level"`   // "log", "warn", "error", "info", "exception"
	Message string `json:"message"` // Console message text
}

// ConsoleLogs returns all captured console entries.
func (d *Driver) ConsoleLogs() []ConsoleEntry {
	d.consoleMu.Lock()
	defer d.consoleMu.Unlock()
	out := make([]ConsoleEntry, len(d.consoleLogs))
	copy(out, d.consoleLogs)
	return out
}

// ConsoleLogReport returns the captured console entries shaped for the
// per-flow report. Implements the executor's consoleLogReporter interface so
// the flow runner can auto-surface JS errors without requiring users to
// invoke `getConsoleLogs` explicitly. Web flows only — other drivers don't
// implement this method, and the executor's type assertion returns nil for
// them.
func (d *Driver) ConsoleLogReport() []report.ConsoleLog {
	d.consoleMu.Lock()
	defer d.consoleMu.Unlock()
	if len(d.consoleLogs) == 0 {
		return nil
	}
	out := make([]report.ConsoleLog, len(d.consoleLogs))
	for i, e := range d.consoleLogs {
		out[i] = report.ConsoleLog{Level: e.Level, Message: e.Message}
	}
	return out
}

// ClearConsoleLogReport resets the captured-console buffer. Called by the
// flow runner at the start of each top-level flow so pre-flow noise (events
// captured during driver construction's initial navigation, before the
// user's first step ran) doesn't pollute the per-flow report — and so
// per-flow auto-surface counts match what the user's flow actually
// triggered. Same buffer the existing `clearConsoleLogs` flow step writes
// to; this method exists separately so the executor can call it via the
// consoleLogReporter interface without needing to dispatch a flow command.
func (d *Driver) ClearConsoleLogReport() {
	d.consoleMu.Lock()
	defer d.consoleMu.Unlock()
	d.consoleLogs = nil
}

// startConsoleHandler starts a background goroutine to capture console.log/error/warn
// and uncaught JS exceptions via the CDP Runtime domain.
func (d *Driver) startConsoleHandler() {
	if err := (proto.RuntimeEnable{}).Call(d.page); err != nil {
		log.Printf("[browser] failed to enable Runtime domain: %v", err)
		return
	}

	go d.page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) bool {
		level := string(e.Type)
		msg := formatConsoleArgs(e.Args)

		d.consoleMu.Lock()
		d.consoleLogs = append(d.consoleLogs, ConsoleEntry{Level: level, Message: msg})
		d.consoleMu.Unlock()

		select {
		case <-d.stopCh:
			return true
		default:
			return false
		}
	}, func(e *proto.RuntimeExceptionThrown) bool {
		msg := e.ExceptionDetails.Text
		if e.ExceptionDetails.Exception != nil {
			if desc := e.ExceptionDetails.Exception.Description; desc != "" {
				msg = desc
			}
		}

		d.consoleMu.Lock()
		d.consoleLogs = append(d.consoleLogs, ConsoleEntry{Level: "exception", Message: msg})
		d.consoleMu.Unlock()

		select {
		case <-d.stopCh:
			return true
		default:
			return false
		}
	})()
}

// formatConsoleArgs converts RuntimeRemoteObject args to a readable string.
func formatConsoleArgs(args []*proto.RuntimeRemoteObject) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		switch {
		case arg.Value.Str() != "":
			parts = append(parts, arg.Value.Str())
		case arg.Description != "":
			parts = append(parts, arg.Description)
		default:
			parts = append(parts, arg.Value.String())
		}
	}
	return strings.Join(parts, " ")
}
