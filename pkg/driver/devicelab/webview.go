package devicelab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
)

// CDPForwarder handles ADB socket forwarding for CDP connections.
type CDPForwarder interface {
	ForwardToAbstractSocket(localSocketPath, remoteSocketName string) error
	ForwardTCPToAbstractSocket(localPort int, remoteSocketName string) error
	RemoveSocketForward(socketPath string) error
	RemoveTCPForward(localPort int) error
	CDPSocketPath() string
}

// errConnectionDead signals that the CDP WebSocket connection is broken and needs cleanup.
var errConnectionDead = fmt.Errorf("CDP connection dead")

// webViewManager manages the Rod/CDP connection to an Android WebView.
// It connects when a CDP socket becomes available and disconnects when it goes away.
type webViewManager struct {
	mu         sync.RWMutex
	browser    *rod.Browser
	page       *rod.Page
	cdpType    string // "webview"
	socketPath string // local forwarded socket path
	network    *webViewNetworkTracker

	forwarder CDPForwarder
}

func newWebViewManager(forwarder CDPForwarder) *webViewManager {
	return &webViewManager{
		forwarder: forwarder,
	}
}

// connect establishes a Rod connection to the WebView's CDP socket.
// Called when CDPTracker reports a socket is available.
func (m *webViewManager) connect(cdpInfo *core.CDPInfo, cdpType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Already connected to same socket
	if m.page != nil && m.cdpType == cdpType {
		return nil
	}

	// Disconnect previous connection if any
	m.disconnectLocked()

	// Chrome browser on Android: use HTTP /json + page-level WebSocket
	// (browser-level Target.getTargets is unsupported on many mobile Chrome versions)
	if cdpType == "browser" {
		return m.connectBrowserViaHTTP(cdpInfo, cdpType)
	}

	return m.connectViaUnixSocket(cdpInfo, cdpType)
}

// connectViaUnixSocket uses ADB Unix socket forwarding with a custom dialer.
func (m *webViewManager) connectViaUnixSocket(cdpInfo *core.CDPInfo, cdpType string) error {
	socketPath := m.forwarder.CDPSocketPath()

	// Step 4: ADB socket forwarding (local unix socket → device abstract socket)
	logger.Info("[cdp:4-forward] setting up ADB forward: local=%s → device=%s", socketPath, cdpInfo.Socket)
	if err := m.forwarder.ForwardToAbstractSocket(socketPath, cdpInfo.Socket); err != nil {
		logger.Info("[cdp:4-forward] ADB forward failed: %v", err)
		return fmt.Errorf("failed to forward CDP socket: %w", err)
	}
	logger.Info("[cdp:4-forward] ADB forward established: local=%s → device=%s", socketPath, cdpInfo.Socket)

	// Step 5: CDP WebSocket connection via unix socket
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer connectCancel()

	ws := &cdp.WebSocket{
		Dialer: &unixDialer{socketPath: socketPath},
	}
	logger.Info("[cdp:5-websocket] connecting CDP WebSocket via unix socket: %s", socketPath)
	if err := ws.Connect(connectCtx, "ws://localhost/devtools/browser", nil); err != nil {
		logger.Info("[cdp:5-websocket] CDP WebSocket connection failed: %v", err)
		_ = m.forwarder.RemoveSocketForward(socketPath)
		os.Remove(socketPath)
		return fmt.Errorf("failed to connect CDP WebSocket: %w", err)
	}
	logger.Info("[cdp:5-websocket] CDP WebSocket connected successfully")

	// Step 6: Rod browser client + page acquisition (bounded by timeout)
	logger.Info("[cdp:6-browser] creating Rod browser client")
	client := cdp.New().Start(ws)

	browser := rod.New().Client(client).NoDefaultDevice()
	// Use a temporary timeout for Connect + Pages — the browser object itself is long-lived
	browserTimeout := browser.Timeout(10 * time.Second)
	if err := browserTimeout.Connect(); err != nil {
		logger.Info("[cdp:6-browser] Rod browser connection failed: %v", err)
		_ = m.forwarder.RemoveSocketForward(socketPath)
		os.Remove(socketPath)
		return fmt.Errorf("failed to connect Rod browser: %w", err)
	}

	pages, err := browserTimeout.Pages()
	if err != nil || len(pages) == 0 {
		logger.Info("[cdp:6-browser] no pages found in WebView (err=%v)", err)
		browser.Close()
		_ = m.forwarder.RemoveSocketForward(socketPath)
		os.Remove(socketPath)
		return fmt.Errorf("no pages found in WebView")
	}

	page := pages.First()
	pageInfo, _ := page.Info()
	pageURL := ""
	if pageInfo != nil {
		pageURL = pageInfo.URL
	}
	logger.Info("[cdp:6-browser] Rod browser connected, found %d page(s), active page: %s", len(pages), pageURL)

	// Step 7: JS helper injection + ready
	logger.Info("[cdp:7-ready] injecting JS helper into WebView")
	if _, err := page.EvalOnNewDocument(webViewJSHelper); err != nil {
		logger.Warn("[cdp:7-ready] failed to inject JS helper for future navigations: %v", err)
	}
	if _, err := page.Evaluate(rod.Eval(webViewJSHelper)); err != nil {
		logger.Info("[cdp:7-ready] failed to inject JS helper into current page: %v", err)
	}

	m.browser = browser
	m.page = page
	m.cdpType = cdpType
	m.socketPath = socketPath

	// Enable network idle tracking for WebView navigations
	m.setupNetworkTracking(page)

	logger.Info("[cdp:7-ready] WebView CDP connection ready — type=%s socket=%s page=%s", cdpType, cdpInfo.Socket, pageURL)
	return nil
}

// cdpTarget represents a Chrome DevTools Protocol target from /json endpoint.
type cdpTarget struct {
	ID                 string `json:"id"`
	Type               string `json:"type"`
	Title              string `json:"title"`
	URL                string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// browserCDPClient wraps a cdp.Client for page-level Chrome Android connections.
// Chrome on Android doesn't support Target.* commands at the browser level,
// so we connect at the page level (/devtools/page/<id>) and intercept
// Target.setDiscoverTargets and Target.attachToTarget to fake success.
// All other commands pass through to the page directly (no session ID needed).
type browserCDPClient struct {
	*cdp.Client
}

func (c *browserCDPClient) Call(ctx context.Context, sessionID, method string, params interface{}) ([]byte, error) {
	switch method {
	case "Target.setDiscoverTargets":
		return []byte(`{}`), nil
	case "Target.attachToTarget":
		// Return empty session ID — on a page-level connection, commands go directly
		return []byte(`{"sessionId":""}`), nil
	}
	return c.Client.Call(ctx, sessionID, method, params)
}

// connectBrowserViaHTTP connects to Chrome browser using HTTP /json for page discovery.
// Chrome on Android doesn't reliably support Target.setDiscoverTargets/getTargets at the
// browser level, so we discover pages via the HTTP /json endpoint, then connect at the
// browser level with a wrapped CDP client and use PageFromTarget with the known target ID.
func (m *webViewManager) connectBrowserViaHTTP(cdpInfo *core.CDPInfo, cdpType string) error {
	socketPath := m.forwarder.CDPSocketPath()

	// Step 4: ADB socket forwarding
	logger.Info("[cdp:4-forward] setting up ADB forward: local=%s → device=%s", socketPath, cdpInfo.Socket)
	if err := m.forwarder.ForwardToAbstractSocket(socketPath, cdpInfo.Socket); err != nil {
		logger.Info("[cdp:4-forward] ADB forward failed: %v", err)
		return fmt.Errorf("failed to forward CDP socket: %w", err)
	}
	logger.Info("[cdp:4-forward] ADB forward established: local=%s → device=%s", socketPath, cdpInfo.Socket)

	// Step 5: HTTP GET /json to discover page targets
	logger.Info("[cdp:5-discover] fetching page targets via HTTP /json")
	targets, err := m.fetchCDPTargets(socketPath)
	if err != nil {
		logger.Info("[cdp:5-discover] failed to fetch targets: %v", err)
		_ = m.forwarder.RemoveSocketForward(socketPath)
		os.Remove(socketPath)
		return fmt.Errorf("failed to fetch CDP targets: %w", err)
	}

	// Find the most recently created page target (highest ID = newest tab).
	// Chrome returns targets in focus order, but after openLink there's a race
	// where the old tab may still be focused. Using the highest ID is more reliable.
	var pageTarget *cdpTarget
	for i := range targets {
		if targets[i].Type == "page" {
			if pageTarget == nil || targets[i].ID > pageTarget.ID {
				pageTarget = &targets[i]
			}
		}
	}
	if pageTarget == nil {
		logger.Info("[cdp:5-discover] no page targets found in %d targets", len(targets))
		_ = m.forwarder.RemoveSocketForward(socketPath)
		os.Remove(socketPath)
		return fmt.Errorf("no page targets found")
	}
	logger.Info("[cdp:5-discover] found page target: id=%s title=%q url=%s", pageTarget.ID, pageTarget.Title, pageTarget.URL)

	// Close all other tabs to prevent tab accumulation and stale connections.
	// Each openLink creates a new Chrome tab; without cleanup, tabs pile up indefinitely.
	m.closeOtherTabs(socketPath, targets, pageTarget.ID)

	// Step 6: Connect WebSocket to PAGE-level endpoint (not browser level)
	// Chrome on Android doesn't support Target.* commands at browser level.
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer connectCancel()

	pageWSPath := fmt.Sprintf("ws://localhost/devtools/page/%s", pageTarget.ID)
	ws := &cdp.WebSocket{
		Dialer: &unixDialer{socketPath: socketPath},
	}
	logger.Info("[cdp:6-websocket] connecting CDP WebSocket to page: %s", pageWSPath)
	if err := ws.Connect(connectCtx, pageWSPath, nil); err != nil {
		logger.Info("[cdp:6-websocket] page WebSocket connection failed: %v", err)
		_ = m.forwarder.RemoveSocketForward(socketPath)
		os.Remove(socketPath)
		return fmt.Errorf("failed to connect page WebSocket: %w", err)
	}
	logger.Info("[cdp:6-websocket] page WebSocket connected successfully")

	// Step 7: Create Rod browser+page via wrapped CDP client
	// The wrapper intercepts Target.* commands (unsupported on page-level connections)
	// and returns fake responses so Rod's Connect() and PageFromTarget() succeed.
	logger.Info("[cdp:7-browser] creating Rod browser+page (page-level CDP)")
	rawClient := cdp.New().Start(ws)
	wrappedClient := &browserCDPClient{Client: rawClient}
	browser := rod.New().Client(wrappedClient).NoDefaultDevice()

	if err := browser.Connect(); err != nil {
		logger.Info("[cdp:7-browser] Rod browser connection failed: %v", err)
		_ = m.forwarder.RemoveSocketForward(socketPath)
		os.Remove(socketPath)
		return fmt.Errorf("failed to connect Rod browser: %w", err)
	}

	targetID := proto.TargetTargetID(pageTarget.ID)
	page, err := browser.PageFromTarget(targetID)
	if err != nil {
		logger.Info("[cdp:7-browser] PageFromTarget failed: %v", err)
		browser.Close()
		_ = m.forwarder.RemoveSocketForward(socketPath)
		os.Remove(socketPath)
		return fmt.Errorf("failed to create Rod page: %w", err)
	}
	logger.Info("[cdp:7-browser] Rod page created successfully")

	// Step 8: Inject browser JS helper (dialog overrides + element finding + visibility + polling).
	// Separate from webViewJSHelper (embedded WebViews) and desktop jsHelperCode.
	logger.Info("[cdp:8-ready] injecting browser JS helper")
	if _, err := page.EvalOnNewDocument(browserJSHelper); err != nil {
		logger.Warn("[cdp:8-ready] failed to inject browser JS for future navigations: %v", err)
	}
	if _, err := page.Evaluate(rod.Eval(browserJSHelper)); err != nil {
		logger.Info("[cdp:8-ready] failed to inject browser JS into current page: %v", err)
	}

	m.browser = browser
	m.page = page
	m.cdpType = cdpType
	m.socketPath = socketPath

	// Enable network idle tracking for browser navigations
	m.setupNetworkTracking(page)

	logger.Info("[cdp:8-ready] Browser CDP connection ready — type=%s socket=%s page=%s", cdpType, cdpInfo.Socket, pageTarget.URL)
	return nil
}

// fetchCDPTargets fetches the list of CDP targets via HTTP /json over a Unix socket.
func (m *webViewManager) fetchCDPTargets(socketPath string) ([]cdpTarget, error) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	resp, err := httpClient.Get("http://localhost/json")
	if err != nil {
		return nil, fmt.Errorf("HTTP GET /json failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var targets []cdpTarget
	if err := json.Unmarshal(body, &targets); err != nil {
		return nil, fmt.Errorf("failed to parse targets JSON: %w", err)
	}

	return targets, nil
}

// closeOtherTabs closes all Chrome tabs except the one we're connecting to.
// Uses HTTP /json/close/<id> via the forwarded Unix socket. Best-effort — errors are logged but ignored.
func (m *webViewManager) closeOtherTabs(socketPath string, targets []cdpTarget, keepID string) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	httpClient := &http.Client{Transport: transport, Timeout: 2 * time.Second}

	closed := 0
	for _, t := range targets {
		if t.ID == keepID || t.Type != "page" {
			continue
		}
		resp, err := httpClient.Get(fmt.Sprintf("http://localhost/json/close/%s", t.ID))
		if err != nil {
			logger.Debug("[cdp:5-cleanup] failed to close tab %s: %v", t.ID, err)
			continue
		}
		resp.Body.Close()
		closed++
	}
	if closed > 0 {
		logger.Info("[cdp:5-cleanup] closed %d old tab(s), keeping %s", closed, keepID)
	}
}

// disconnect closes the Rod connection and cleans up ADB forwarding.
func (m *webViewManager) disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disconnectLocked()
}

func (m *webViewManager) disconnectLocked() {
	if m.browser != nil {
		logger.Info("[cdp:disconnect] closing Rod browser (type=%s, socket=%s)", m.cdpType, m.socketPath)
		m.browser.Close()
		m.browser = nil
	}
	m.page = nil
	m.network = nil
	if m.socketPath != "" {
		logger.Info("[cdp:disconnect] removing ADB forward and cleaning up: %s", m.socketPath)
		_ = m.forwarder.RemoveSocketForward(m.socketPath)
		os.Remove(m.socketPath)
		m.socketPath = ""
	}
	m.cdpType = ""
}

// cleanup tears down the entire CDP connection — Rod browser, ADB forward, local socket.
// Called when we detect the connection is dead (WebSocket broken, WebView destroyed, etc.)
// so that ensureWebViewConnection can reconnect on the next find loop iteration.
func (m *webViewManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	socketPath := m.socketPath
	cdpType := m.cdpType

	if m.browser != nil {
		logger.Info("[cdp:cleanup] closing Rod browser (type=%s)", cdpType)
		m.browser.Close()
		m.browser = nil
	}
	m.page = nil
	m.network = nil

	if socketPath != "" {
		logger.Info("[cdp:cleanup] removing ADB forward: %s", socketPath)
		_ = m.forwarder.RemoveSocketForward(socketPath)
		logger.Info("[cdp:cleanup] removing local socket file: %s", socketPath)
		os.Remove(socketPath)
		m.socketPath = ""
	}
	m.cdpType = ""

	logger.Info("[cdp:cleanup] full cleanup done — ready for reconnect")
}

// rodPage returns the current Rod page, or nil if not connected.
func (m *webViewManager) rodPage() *rod.Page {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.page
}

// webViewType returns "browser", "webview", or "" if not connected.
func (m *webViewManager) webViewType() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cdpType
}

// isConnected returns true if Rod is connected to a WebView.
func (m *webViewManager) isConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.page != nil
}

// refreshPage re-acquires the active page from the browser connection.
// Returns errConnectionDead if the browser connection is broken.
func (m *webViewManager) refreshPage() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.browser == nil {
		return errConnectionDead
	}

	browser := m.browser.Timeout(cdpCallTimeout)
	pages, err := browser.Pages()
	if err != nil {
		// browser.Pages() failing means the CDP WebSocket is dead or unresponsive
		return errConnectionDead
	}
	if len(pages) == 0 {
		return fmt.Errorf("no pages found after refresh")
	}

	m.page = pages.First()

	// Re-inject JS helper into the new page context
	page := m.page.Timeout(cdpCallTimeout)
	if _, err := page.Evaluate(rod.Eval(webViewJSHelper)); err != nil {
		logger.Debug("[webview] failed to inject JS helper after page refresh: %v", err)
	}

	return nil
}

// visibilityCheckTimeout is a short timeout for the document.visibilityState check.
// Must be fast — if the WebView can't answer this in 500ms, it's unresponsive.
const visibilityCheckTimeout = 500 * time.Millisecond

// isWebViewVisible checks if the WebView is actually visible on screen
// by evaluating document.visibilityState via CDP. Returns false if hidden,
// unresponsive, or the connection is dead.
func (m *webViewManager) isWebViewVisible() bool {
	page := m.rodPage()
	if page == nil {
		return false
	}

	result, err := page.Timeout(visibilityCheckTimeout).Eval(`() => document.visibilityState`)
	if err != nil {
		// Timeout or connection dead — treat as not visible
		logger.Info("[cdp:visibility] check failed (unresponsive or dead): %v", err)
		return false
	}

	state := result.Value.Str()
	if state != "visible" {
		logger.Info("[cdp:visibility] WebView not visible (state=%s), skipping CDP", state)
		return false
	}
	return true
}

// findWebOnce performs a single attempt to find an element via Rod/CDP.
// Skips CDP entirely if the WebView is not visible (background tab, fragment detached).
// If the connection is dead (WebSocket broken), it cleans up everything
// (Rod browser, ADB forward, local socket file) so the next ensureWebViewConnection
// call can reconnect cleanly.
func (m *webViewManager) findWebOnce(sel flow.Selector) (core.Element, error) {
	// Browser mode: skip visibility check and page refresh — we're connected
	// directly at the page level and Chrome's visibilityState may report "hidden"
	// even when the page is visible (background tab issue).
	if m.webViewType() == "browser" {
		return m.findWebOnceBrowser(sel)
	}

	// Quick visibility gate — if WebView is hidden (tab switched, fragment detached),
	// skip all CDP work and let the caller fall through to native immediately.
	if !m.isWebViewVisible() {
		// Visibility check can fail during in-WebView navigation (JS context destroyed).
		// Try refreshing the page reference and check again before giving up on CDP.
		if refreshErr := m.refreshPage(); refreshErr != nil {
			if refreshErr == errConnectionDead {
				m.cleanup()
				return nil, errConnectionDead
			}
			return nil, fmt.Errorf("webview not visible")
		}
		if !m.isWebViewVisible() {
			return nil, fmt.Errorf("webview not visible")
		}
	}

	// Wait briefly for network idle — if XHR/fetch requests are still in-flight
	// after navigation, give them a moment to complete before looking for elements.
	if t := m.getNetworkTracker(); t != nil {
		t.waitForIdle(1*time.Second, 500*time.Millisecond)
	}

	elem, err := m.findWebOnceInternal(sel)
	if err == nil {
		return elem, nil
	}

	logger.Debug("[webview] findWebOnce failed: %v, refreshing page...", err)

	if refreshErr := m.refreshPage(); refreshErr != nil {
		if refreshErr == errConnectionDead {
			logger.Info("[cdp:cleanup] connection dead detected during find (find err: %v), cleaning up", err)
			m.cleanup()
			return nil, errConnectionDead
		}
		logger.Debug("[webview] refreshPage failed: %v", refreshErr)
		return nil, err
	}

	// Wait for page to be ready after refresh — handles in-WebView navigation
	// where the page URL changed and the new DOM is still loading.
	m.waitForPageReady()

	return m.findWebOnceInternal(sel)
}

// findWebOnceBrowser performs element finding for Chrome browser mode.
// Uses the injected __maestro.findVisible() JS helper for a single CDP roundtrip
// with proper visibility checks. Falls back to findWebOnceInternal (AX tree + CSS).
// On error, checks if connection is dead and cleans up.
func (m *webViewManager) findWebOnceBrowser(sel flow.Selector) (core.Element, error) {
	page := m.rodPage()
	if page == nil {
		return nil, fmt.Errorf("no CDP connection")
	}

	// Wait briefly for network idle before element find
	if t := m.getNetworkTracker(); t != nil {
		t.waitForIdle(1*time.Second, 500*time.Millisecond)
	}

	// Try JS helper first — single CDP roundtrip with visibility check.
	selectorType, selectorValue := browserSelectorTypeValue(sel)
	if selectorType != "" {
		timedPage := page.Timeout(cdpCallTimeout)
		obj, err := timedPage.Evaluate(
			rod.Eval(`(type, value) => window.__maestro.findVisible(type, value)`,
				selectorType, selectorValue).ByObject(),
		)
		if err == nil {
			elem, err := timedPage.ElementFromObject(obj)
			if err == nil {
				info := webElementInfo(elem)
				return &WebElement{elem: elem, info: info}, nil
			}
		}
		logger.Debug("[cdp:browser] JS findVisible miss: %s — %v", sel.Describe(), err)
	}

	// Fallback to AX tree + CSS selectors (e.g. role-based queries)
	elem, err := m.findWebOnceInternal(sel)
	if err == nil {
		return elem, nil
	}
	logger.Debug("[cdp:browser] findWebOnceInternal miss: %s — %v", sel.Describe(), err)

	// Check if the error is a connection issue by trying a simple eval
	if _, evalErr := page.Timeout(500 * time.Millisecond).Eval(`() => true`); evalErr != nil {
		logger.Info("[cdp:browser] connection dead detected: %v", evalErr)
		m.cleanup()
		return nil, errConnectionDead
	}

	return nil, err
}

// browserSelectorTypeValue maps a flow.Selector to (type, value) for the
// browser-side __maestro JS helper. Returns ("", "") for unsupported selectors.
func browserSelectorTypeValue(sel flow.Selector) (string, string) {
	switch {
	case sel.CSS != "":
		return "css", sel.CSS
	case sel.TestID != "":
		return "testId", sel.TestID
	case sel.Name != "":
		return "name", sel.Name
	case sel.Placeholder != "":
		return "placeholder", sel.Placeholder
	case sel.Href != "":
		return "href", sel.Href
	case sel.Alt != "":
		return "alt", sel.Alt
	case sel.Title != "":
		return "title", sel.Title
	case sel.Role != "":
		return "role", sel.Role
	case sel.ID != "":
		return "id", sel.ID
	case sel.TextRegex != "":
		return "textRegex", sel.TextRegex
	case sel.TextContains != "":
		return "textContains", sel.TextContains
	case sel.Text != "":
		return "text", sel.Text
	default:
		return "", ""
	}
}

// waitForPageReady waits for document.readyState to be "complete" or "interactive",
// then waits for network idle (no in-flight requests for 500ms).
// Bounded to avoid blocking the polling loop for too long.
func (m *webViewManager) waitForPageReady() {
	page := m.rodPage()
	if page == nil {
		return
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		result, err := page.Timeout(500 * time.Millisecond).Eval(`() => document.readyState`)
		if err == nil {
			state := result.Value.Str()
			if state == "complete" || state == "interactive" {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for network idle — XHR/fetch requests to complete after DOM ready
	if t := m.getNetworkTracker(); t != nil {
		t.waitForIdle(5*time.Second, 500*time.Millisecond)
	}
}

// cdpCallTimeout is the maximum time any single CDP find attempt can take.
// Prevents hangs when the WebView is suspended, doing heavy JS, or unresponsive.
const cdpCallTimeout = 3 * time.Second

// findWebOnceInternal performs a single attempt to find an element via Rod/CDP.
// All Rod calls are bounded by cdpCallTimeout — no CDP call can hang indefinitely.
func (m *webViewManager) findWebOnceInternal(sel flow.Selector) (core.Element, error) {
	page := m.rodPage()
	if page == nil {
		return nil, fmt.Errorf("no CDP connection")
	}

	// Wrap page with timeout — all downstream Rod calls inherit this deadline.
	// If the WebView is unresponsive (suspended, heavy JS, etc.), we bail out
	// after cdpCallTimeout and fall through to native UiAutomator.
	page = page.Timeout(cdpCallTimeout)

	switch {
	case sel.CSS != "":
		return m.findByCSS(page, sel)
	case sel.TestID != "":
		return m.findByCSSSelector(page, fmt.Sprintf("[data-testid=%q]", sel.TestID))
	case sel.Name != "":
		return m.findByCSSSelector(page, fmt.Sprintf("[name=%q]", sel.Name))
	case sel.Placeholder != "":
		return m.findByCSSSelector(page, fmt.Sprintf("[placeholder=%q]", sel.Placeholder))
	case sel.Href != "":
		return m.findByCSSSelector(page, fmt.Sprintf("[href*=%q]", sel.Href))
	case sel.Alt != "":
		return m.findByCSSSelector(page, fmt.Sprintf("[alt=%q]", sel.Alt))
	case sel.Title != "":
		return m.findByCSSSelector(page, fmt.Sprintf("[title=%q]", sel.Title))
	case sel.Role != "":
		return m.findByAXTree(page, sel.Text, sel.Role)
	case sel.ID != "":
		return m.findByID(page, sel.ID)
	case sel.Text != "":
		return m.findByText(page, sel.Text)
	case sel.TextContains != "":
		return m.findByTextContains(page, sel.TextContains)
	case sel.TextRegex != "":
		return m.findByTextRegex(page, sel.TextRegex)
	default:
		return nil, fmt.Errorf("no web-compatible selector")
	}
}

// findFocusedWeb returns the currently focused element in the WebView.
// Cleans up if the connection is dead.
func (m *webViewManager) findFocusedWeb() (core.Element, error) {
	page := m.rodPage()
	if page == nil {
		return nil, fmt.Errorf("no CDP connection")
	}

	page = page.Timeout(cdpCallTimeout)

	obj, err := page.Evaluate(rod.Eval(`() => {
		const el = document.activeElement;
		if (!el || el === document.body) return null;
		return el;
	}`).ByObject())
	if err != nil {
		// Check if connection is dead by trying refreshPage
		if refreshErr := m.refreshPage(); refreshErr == errConnectionDead {
			logger.Info("[cdp:cleanup] connection dead detected during findFocused, cleaning up")
			m.cleanup()
			return nil, errConnectionDead
		}
		return nil, fmt.Errorf("no focused web element: %w", err)
	}

	elem, err := page.ElementFromObject(obj)
	if err != nil {
		return nil, fmt.Errorf("focused element from object: %w", err)
	}

	visible, _ := elem.Visible()
	if !visible {
		return nil, fmt.Errorf("focused web element is not visible")
	}

	info := webElementInfo(elem)
	return &WebElement{elem: elem, info: info}, nil
}

// ---- Internal finders (one-shot, no polling) ----

func (m *webViewManager) findByCSS(page *rod.Page, sel flow.Selector) (core.Element, error) {
	p := page.Sleeper(rod.NotFoundSleeper)
	elem, err := p.Element(sel.CSS)
	if err != nil {
		return nil, fmt.Errorf("CSS '%s' not found: %w", sel.CSS, err)
	}
	visible, _ := elem.Visible()
	if !visible {
		return nil, fmt.Errorf("CSS '%s' found but not visible", sel.CSS)
	}
	info := webElementInfo(elem)
	return &WebElement{elem: elem, info: info}, nil
}

func (m *webViewManager) findByCSSSelector(page *rod.Page, css string) (core.Element, error) {
	p := page.Sleeper(rod.NotFoundSleeper)
	elem, err := p.Element(css)
	if err != nil {
		return nil, fmt.Errorf("selector '%s' not found: %w", css, err)
	}
	visible, _ := elem.Visible()
	if !visible {
		return nil, fmt.Errorf("selector '%s' found but not visible", css)
	}
	info := webElementInfo(elem)
	return &WebElement{elem: elem, info: info}, nil
}

func (m *webViewManager) findByID(page *rod.Page, id string) (core.Element, error) {
	selectors := []string{
		"#" + cssEscapeID(id),
		fmt.Sprintf("[data-testid=%q]", id),
		fmt.Sprintf("[id*=%q]", id),
		fmt.Sprintf("[name=%q]", id),
		fmt.Sprintf("[aria-label=%q]", id),
	}

	p := page.Sleeper(rod.NotFoundSleeper)
	for _, css := range selectors {
		elem, err := p.Element(css)
		if err != nil {
			continue
		}
		visible, _ := elem.Visible()
		if !visible {
			continue
		}
		info := webElementInfo(elem)
		return &WebElement{elem: elem, info: info}, nil
	}
	return nil, fmt.Errorf("element with id '%s' not found", id)
}

// axRolePriority defines the priority order for AX tree node roles.
// Lower value = higher priority. Clickable roles first, then input, then everything else.
var axRolePriority = map[string]int{
	// Clickable roles (highest priority)
	"button": 1, "link": 1, "menuitem": 1, "tab": 1, "checkbox": 1, "radio": 1,
	// Input roles
	"textbox": 2, "combobox": 2, "searchbox": 2, "spinbutton": 2,
}

func (m *webViewManager) findByText(page *rod.Page, text string) (core.Element, error) {
	// Single AX tree query (no role filter) — returns all nodes matching the name.
	// We prioritize by role on the Go side: clickable > input > any.
	if elem, err := m.findByAXTreePrioritized(page, text); err == nil {
		return elem, nil
	}

	// JS fallback for elements not in AX tree
	return m.findByJS(page, text)
}

// findByAXTreePrioritized queries the AX tree once without role filter,
// then picks the best visible node by role priority (clickable > input > other).
func (m *webViewManager) findByAXTreePrioritized(page *rod.Page, text string) (core.Element, error) {
	body, err := page.Sleeper(rod.NotFoundSleeper).Element("body")
	if err != nil {
		return nil, fmt.Errorf("failed to get body: %w", err)
	}

	query := &proto.AccessibilityQueryAXTree{
		ObjectID:       body.Object.ObjectID,
		AccessibleName: text,
	}
	result, err := query.Call(page)
	if err != nil {
		return nil, fmt.Errorf("AX tree query failed: %w", err)
	}

	// Resolve all visible nodes, track their roles
	type candidate struct {
		elem     *rod.Element
		priority int
	}
	var candidates []candidate

	for _, node := range result.Nodes {
		if node.BackendDOMNodeID == 0 {
			continue
		}
		resolve := &proto.DOMResolveNode{BackendNodeID: node.BackendDOMNodeID}
		remote, err := resolve.Call(page)
		if err != nil {
			continue
		}
		elem, err := page.ElementFromObject(remote.Object)
		if err != nil {
			continue
		}
		visible, _ := elem.Visible()
		if !visible {
			continue
		}

		// Determine priority from role
		pri := 3 // default: lowest priority
		if node.Role != nil {
			roleName := node.Role.Value.Str()
			if p, found := axRolePriority[roleName]; found {
				pri = p
			}
		}

		// Priority 1 (clickable) — return immediately, can't do better
		if pri == 1 {
			info := webElementInfo(elem)
			return &WebElement{elem: elem, info: info}, nil
		}
		candidates = append(candidates, candidate{elem: elem, priority: pri})
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no visible AX node for text '%s'", text)
	}

	// Pick the highest priority (lowest number) candidate
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.priority < best.priority {
			best = c
		}
	}
	info := webElementInfo(best.elem)
	return &WebElement{elem: best.elem, info: info}, nil
}

func (m *webViewManager) findByTextContains(page *rod.Page, text string) (core.Element, error) {
	jsCode := `(text) => {
		const all = document.querySelectorAll('*');
		for (const el of all) {
			if (el.children.length === 0 && el.textContent && el.textContent.includes(text)) {
				return el;
			}
		}
		return null;
	}`
	obj, err := page.Evaluate(rod.Eval(jsCode, text).ByObject())
	if err != nil {
		return nil, fmt.Errorf("textContains '%s' not found: %w", text, err)
	}
	elem, err := page.ElementFromObject(obj)
	if err != nil {
		return nil, fmt.Errorf("textContains '%s' element from object failed: %w", text, err)
	}
	visible, _ := elem.Visible()
	if !visible {
		return nil, fmt.Errorf("textContains '%s' found but not visible", text)
	}
	info := webElementInfo(elem)
	return &WebElement{elem: elem, info: info}, nil
}

func (m *webViewManager) findByTextRegex(page *rod.Page, pattern string) (core.Element, error) {
	jsCode := `(pattern) => {
		const re = new RegExp(pattern);
		const all = document.querySelectorAll('*');
		for (const el of all) {
			if (el.children.length === 0 && el.textContent && re.test(el.textContent)) {
				return el;
			}
		}
		return null;
	}`
	obj, err := page.Evaluate(rod.Eval(jsCode, pattern).ByObject())
	if err != nil {
		return nil, fmt.Errorf("textRegex '%s' not found: %w", pattern, err)
	}
	elem, err := page.ElementFromObject(obj)
	if err != nil {
		return nil, fmt.Errorf("textRegex '%s' element from object failed: %w", pattern, err)
	}
	visible, _ := elem.Visible()
	if !visible {
		return nil, fmt.Errorf("textRegex '%s' found but not visible", pattern)
	}
	info := webElementInfo(elem)
	return &WebElement{elem: elem, info: info}, nil
}

func (m *webViewManager) findByAXTree(page *rod.Page, text, role string) (core.Element, error) {
	body, err := page.Sleeper(rod.NotFoundSleeper).Element("body")
	if err != nil {
		return nil, fmt.Errorf("failed to get body: %w", err)
	}

	query := &proto.AccessibilityQueryAXTree{
		ObjectID:       body.Object.ObjectID,
		AccessibleName: text,
	}
	if role != "" {
		query.Role = role
	}

	result, err := query.Call(page)
	if err != nil {
		return nil, fmt.Errorf("AX tree query failed: %w", err)
	}

	for _, node := range result.Nodes {
		if node.BackendDOMNodeID == 0 {
			continue
		}
		resolve := &proto.DOMResolveNode{BackendNodeID: node.BackendDOMNodeID}
		remote, err := resolve.Call(page)
		if err != nil {
			continue
		}
		elem, err := page.ElementFromObject(remote.Object)
		if err != nil {
			continue
		}
		visible, _ := elem.Visible()
		if !visible {
			continue
		}
		info := webElementInfo(elem)
		return &WebElement{elem: elem, info: info}, nil
	}

	return nil, fmt.Errorf("no visible AX node for text '%s' role '%s'", text, role)
}

func (m *webViewManager) findByJS(page *rod.Page, text string) (core.Element, error) {
	obj, err := page.Evaluate(rod.Eval(`(text) => window.__maestro.findByText(text)`, text).ByObject())
	if err != nil {
		return nil, fmt.Errorf("JS findByText failed: %w", err)
	}
	elem, err := page.ElementFromObject(obj)
	if err != nil {
		return nil, fmt.Errorf("JS findByText element from object failed: %w", err)
	}
	info := webElementInfo(elem)
	return &WebElement{elem: elem, info: info}, nil
}

// cssEscapeID escapes an ID for CSS selector use.
func cssEscapeID(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_':
			b.WriteRune(c)
		default:
			b.WriteRune('\\')
			b.WriteRune(c)
		}
	}
	return b.String()
}

// unixDialer implements cdp.Dialer for Unix socket connections.
type unixDialer struct {
	socketPath string
}

func (d *unixDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, "unix", d.socketPath)
}

// webViewNetworkTracker tracks in-flight network requests via CDP Network domain events.
// Used to detect network idle state after in-WebView navigations (link taps, SPA route changes).
type webViewNetworkTracker struct {
	mu       sync.Mutex
	inflight map[proto.NetworkRequestID]struct{}
	lastIdle time.Time
}

func newWebViewNetworkTracker() *webViewNetworkTracker {
	return &webViewNetworkTracker{
		inflight: make(map[proto.NetworkRequestID]struct{}),
		lastIdle: time.Now(),
	}
}

func (t *webViewNetworkTracker) onRequest(id proto.NetworkRequestID, url string, resourceType proto.NetworkResourceType) {
	if resourceType == proto.NetworkResourceTypeWebSocket ||
		resourceType == proto.NetworkResourceTypeEventSource ||
		strings.HasPrefix(url, "data:") {
		return
	}
	t.mu.Lock()
	t.inflight[id] = struct{}{}
	t.mu.Unlock()
}

func (t *webViewNetworkTracker) onComplete(id proto.NetworkRequestID) {
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
func (t *webViewNetworkTracker) waitForIdle(timeout, quietPeriod time.Duration) bool {
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
// lifecycle events. Called after CDP connection is established.
// Must be called with m.mu held (called from connect methods).
func (m *webViewManager) setupNetworkTracking(page *rod.Page) {
	tracker := newWebViewNetworkTracker()
	m.network = tracker

	if err := (proto.NetworkEnable{}).Call(page); err != nil {
		logger.Debug("[cdp:network] failed to enable Network domain: %v", err)
		return
	}

	go page.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) {
			url := ""
			if e.Request != nil {
				url = e.Request.URL
			}
			tracker.onRequest(e.RequestID, url, e.Type)
		},
		func(e *proto.NetworkLoadingFinished) {
			tracker.onComplete(e.RequestID)
		},
		func(e *proto.NetworkLoadingFailed) {
			tracker.onComplete(e.RequestID)
		},
	)()
}

// getNetworkTracker returns the current network tracker, or nil.
func (m *webViewManager) getNetworkTracker() *webViewNetworkTracker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.network
}

