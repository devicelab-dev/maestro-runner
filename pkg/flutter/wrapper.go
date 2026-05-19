package flutter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

const (
	// innerPollTimeout is the inner driver timeout per poll iteration (ms).
	// Between each poll, we check if Flutter has found the element.
	innerPollTimeout = 1000

	// innerGraceTimeout is the inner driver timeout after Flutter finds the element (ms).
	// Gives the inner driver one last chance — the element might still be painting.
	innerGraceTimeout = 2000

	// defaultFindTimeout is the fallback when no timeout is configured.
	defaultFindTimeout = 17000
)

// FlutterDriver wraps an inner core.Driver and uses the Flutter VM Service
// in parallel to find elements that the inner driver cannot see.
//
// Parallel search: the inner driver polls in 1s increments while Flutter
// continuously searches in a background goroutine. Between each inner driver
// poll, we check if Flutter has found the element. If Flutter finds it, the
// inner driver gets one last 2s chance, then we use Flutter's coordinates.
type FlutterDriver struct {
	inner         core.Driver
	client        *VMServiceClient
	dev           DeviceExecutor
	appID         string
	socketPath    string          // Unix socket path for VM Service forwarding (Android)
	udid          string          // iOS simulator UDID (empty for Android)
	isIOS         bool            // true = iOS reconnection path
	attempted     bool            // true after first discovery attempt (avoids retrying every step)
	findTimeoutMs int             // current find timeout set by executor (0 = driver default)
	ctx           context.Context // Parent context for element-finding operations (runFlow timeout)
}

// Wrap creates a FlutterDriver that wraps the given inner driver.
// client may be nil — connection will be established lazily on first fallback.
// socketPath is the Unix socket used for adb forward (e.g., /tmp/<serial>-flutter.sock).
func Wrap(inner core.Driver, client *VMServiceClient, dev DeviceExecutor, appID, socketPath string) core.Driver {
	return &FlutterDriver{
		inner:      inner,
		client:     client,
		dev:        dev,
		appID:      appID,
		socketPath: socketPath,
	}
}

// WrapIOS creates a FlutterDriver for iOS simulators.
// client may be nil — connection will be established lazily on first fallback.
// No port forwarding needed — the VM Service listens on localhost directly.
func WrapIOS(inner core.Driver, client *VMServiceClient, udid, appID string) core.Driver {
	return &FlutterDriver{
		inner: inner,
		client: client,
		appID:  appID,
		udid:   udid,
		isIOS:  true,
	}
}

// Execute runs the step using parallel search: the inner driver polls in 1s increments
// while Flutter continuously searches in a background goroutine. Between each inner
// driver poll, we check if Flutter has found the element. This gives ~1s reaction time
// to whichever side finds the element first.
func (d *FlutterDriver) Execute(step flow.Step) *core.CommandResult {
	// After launchApp, the app restarts — invalidate connection but only
	// re-discover if the previous attempt succeeded (was actually Flutter).
	// If the app was already determined to be non-Flutter, don't retry.
	if _, ok := step.(*flow.LaunchAppStep); ok {
		if d.client != nil {
			d.client.Close()
			d.client = nil
			d.attempted = false // Was Flutter before — re-discover after restart
		}
		return d.inner.Execute(step)
	}

	// Non-element-finding steps → inner only
	if !isElementFindingStep(step) {
		return d.inner.Execute(step)
	}

	sel := extractSelector(step)
	if sel == nil || sel.IsEmpty() || (sel.Text == "" && sel.ID == "") {
		return d.inner.Execute(step)
	}

	// No Flutter available → inner only
	if d.client == nil && d.attempted {
		return d.inner.Execute(step)
	}

	// Optional steps: inner driver uses its own optionalFindTimeout internally
	// which ignores SetFindTimeout. Skip parallel loop for these.
	if step.IsOptional() {
		return d.inner.Execute(step)
	}

	effectiveTimeout := d.findTimeoutMs
	if effectiveTimeout <= 0 {
		effectiveTimeout = defaultFindTimeout
	}

	// --- Parallel search: inner driver (1s polls) + Flutter (continuous goroutine) ---

	// Start Flutter polling in background goroutine
	searchCtx, searchCancel := context.WithTimeout(d.parentContext(), time.Duration(effectiveTimeout)*time.Millisecond)
	flutterCh := make(chan *flutterSearchResult, 1)
	go d.flutterPollLoop(searchCtx, sel, flutterCh)

	// Inner driver polls in 1s increments, checking Flutter between each
	logger.Debug("Flutter: parallel search (text=%q, id=%q)", sel.Text, sel.ID)
	deadline := time.Now().Add(time.Duration(effectiveTimeout) * time.Millisecond)
	var result *core.CommandResult

	for time.Now().Before(deadline) {
		// Check if parent context (e.g. runFlow timeout) was cancelled
		if err := d.parentContext().Err(); err != nil {
			searchCancel()
			<-flutterCh
			d.inner.SetFindTimeout(d.findTimeoutMs)
			return core.ErrorResult(fmt.Errorf("element %q not found: %w", sel.Describe(), err), "")
		}

		d.inner.SetFindTimeout(innerPollTimeout)
		result = d.inner.Execute(step)

		if result.Success {
			searchCancel()
			<-flutterCh // drain — ensure goroutine exits before next Execute
			d.inner.SetFindTimeout(d.findTimeoutMs)
			return result
		}

		// Non-element-not-found error (network, crash) → stop immediately
		if !isElementNotFoundError(result) {
			searchCancel()
			<-flutterCh
			d.inner.SetFindTimeout(d.findTimeoutMs)
			return result
		}

		// Check if Flutter found the element
		select {
		case fr := <-flutterCh:
			d.inner.SetFindTimeout(d.findTimeoutMs)
			if fr == nil {
				// Flutter gave up (connection error, etc.) — continue inner-only
				searchCancel()
				// Run inner driver with remaining time
				remaining := int(time.Until(deadline).Milliseconds())
				if remaining > innerPollTimeout {
					d.inner.SetFindTimeout(remaining)
					result = d.inner.Execute(step)
					d.inner.SetFindTimeout(d.findTimeoutMs)
				}
				return result
			}
			// Flutter found it! Give inner driver one last chance (element might be painting)
			logger.Info("Flutter: found element (node #%d at %d,%d), giving inner driver last chance", fr.node.ID, fr.cx, fr.cy)
			searchCancel()
			d.inner.SetFindTimeout(innerGraceTimeout)
			result = d.inner.Execute(step)
			d.inner.SetFindTimeout(d.findTimeoutMs)
			if result.Success {
				return result
			}
			// Inner truly can't see it → use Flutter coordinates
			return d.executeFlutterResult(step, fr)
		default:
			// Flutter still searching, continue loop
		}
	}

	// Inner driver exhausted timeout — wait for Flutter's final answer
	searchCancel()
	fr := <-flutterCh
	d.inner.SetFindTimeout(d.findTimeoutMs)

	if fr != nil {
		logger.Info("Flutter: found element after inner timeout (node #%d at %d,%d)", fr.node.ID, fr.cx, fr.cy)
		return d.executeFlutterResult(step, fr)
	}

	// Both failed
	return result
}

// flutterPollLoop continuously searches the Flutter VM Service trees until the
// element is found, the context is cancelled, or the timeout expires.
// Runs in a goroutine — always writes exactly one result to ch before returning.
func (d *FlutterDriver) flutterPollLoop(ctx context.Context, sel *flow.Selector, ch chan<- *flutterSearchResult) {
	for {
		select {
		case <-ctx.Done():
			ch <- nil
			return
		default:
		}

		if fr := d.searchFlutterOnce(sel); fr != nil {
			ch <- fr
			return
		}
	}
}

// flutterSearchResult holds the element location found via Flutter VM Service.
type flutterSearchResult struct {
	node       *SemanticsNode
	bounds     core.Bounds
	cx, cy     int
	isSuffix   bool
	pixelRatio float64
}

// searchFlutterOnce does a single search attempt across semantics and widget trees.
func (d *FlutterDriver) searchFlutterOnce(sel *flow.Selector) *flutterSearchResult {
	semanticsDump, widgetDump, err := d.getFlutterTrees(true)
	if err != nil {
		logger.Debug("Flutter fallback: failed to get trees: %v", err)
		return nil
	}

	root, pixelRatio, err := ParseSemanticsTree(semanticsDump)
	if err != nil {
		logger.Debug("Flutter fallback: failed to parse semantics tree: %v", err)
		return nil
	}

	// iOS WDA uses points (logical pixels), not physical pixels.
	// Flutter's Rect is already in logical pixels, so skip the pixelRatio scaling.
	if d.isIOS {
		pixelRatio = 1.0
	}

	// Step 1: Search semantics tree directly
	nodes := searchSemanticsTree(root, sel)
	isSuffix := false

	// Step 2: If not found, try widget tree cross-reference
	if len(nodes) == 0 && widgetDump != "" {
		logger.Debug("Flutter fallback: searching widget tree (%d bytes)", len(widgetDump))
		var match *WidgetTreeMatch
		if sel.Text != "" {
			match = SearchWidgetTreeForText(widgetDump, sel.Text)
		}
		if match == nil && sel.ID != "" {
			match = SearchWidgetTreeForID(widgetDump, sel.ID)
		}
		if match != nil {
			logger.Debug("Flutter fallback: widget tree match (type=%s, label=%q, nearbyText=%q, suffix=%v)",
				match.MatchType, match.LabelText, match.NearbyText, match.IsSuffix)
			isSuffix = match.IsSuffix
			nodes = match.CrossReferenceWithSemantics(root)
		}
	}

	if len(nodes) == 0 {
		logger.Debug("Flutter fallback: no matching node (text=%q, id=%q)", sel.Text, sel.ID)
		return nil
	}

	node := nodes[0]
	bounds := node.Rect.ToBounds(pixelRatio)
	cx, cy := bounds.Center()

	return &flutterSearchResult{
		node:       node,
		bounds:     bounds,
		cx:         cx,
		cy:         cy,
		isSuffix:   isSuffix,
		pixelRatio: pixelRatio,
	}
}

// getFlutterTrees fetches the semantics tree (and optionally widget tree).
// If the client is broken, attempts reconnection once.
func (d *FlutterDriver) getFlutterTrees(needWidgetTree bool) (semanticsDump, widgetDump string, err error) {
	if d.client == nil {
		if err := d.tryReconnect(); err != nil {
			return "", "", err
		}
		if d.client == nil {
			return "", "", fmt.Errorf("no flutter client")
		}
	}

	semanticsDump, err = d.client.GetSemanticsTree()
	if err != nil {
		// Connection might be broken — try reconnecting once
		logger.Debug("Flutter fallback: semantics tree error, reconnecting: %v", err)
		if reconnErr := d.tryReconnect(); reconnErr != nil {
			return "", "", fmt.Errorf("connection lost: %w", err)
		}
		if d.client == nil {
			return "", "", fmt.Errorf("reconnection produced nil client")
		}
		semanticsDump, err = d.client.GetSemanticsTree()
		if err != nil {
			return "", "", err
		}
	}

	if needWidgetTree {
		widgetDump, _ = d.client.GetWidgetTree() // best-effort
	}

	return semanticsDump, widgetDump, nil
}

// tryReconnect attempts to re-establish the VM Service connection.
func (d *FlutterDriver) tryReconnect() error {
	if d.client != nil {
		d.client.Close()
		d.client = nil
	}

	if d.isIOS {
		return d.tryReconnectIOS()
	}
	return d.tryReconnectAndroid()
}

// tryReconnectAndroid discovers and connects via Unix socket (adb forward).
func (d *FlutterDriver) tryReconnectAndroid() error {
	d.attempted = true // Mark as attempted even on failure to avoid retrying every step

	if d.dev == nil {
		return fmt.Errorf("no device executor")
	}

	token, err := DiscoverVMService(d.dev, d.appID, d.socketPath)
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}
	if token == "" {
		return fmt.Errorf("no Flutter VM service found")
	}

	client, err := ConnectUnix(d.socketPath, token)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	d.client = client
	logger.Info("Flutter VM service reconnected (Android)")
	return nil
}

// tryReconnectIOS discovers and connects via direct TCP (simulator localhost).
func (d *FlutterDriver) tryReconnectIOS() error {
	d.attempted = true // Mark as attempted even on failure to avoid retrying every step

	wsURL, err := DiscoverVMServiceIOS(d.udid)
	if err != nil {
		return fmt.Errorf("iOS discovery failed: %w", err)
	}
	if wsURL == "" {
		return fmt.Errorf("no Flutter VM service found")
	}

	client, err := Connect(wsURL)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	d.client = client
	logger.Info("Flutter VM service reconnected (iOS)")
	return nil
}

// executeFlutterResult takes a Flutter search result and executes the action.
func (d *FlutterDriver) executeFlutterResult(step flow.Step, fr *flutterSearchResult) *core.CommandResult {
	cx, cy := fr.cx, fr.cy

	// Viewport check: Flutter's widget/semantics trees include lazy ListView
	// items that are laid out in the cache-extent buffer (just past the
	// visible viewport). Their reported coordinates may still fall within
	// the device screen bounds, but a tap there lands on the system nav bar
	// or on a different widget that is actually painted at that position.
	// Reject when the tap point is in the system-bar safe area or fully
	// outside the screen, with a clear error so the user knows to scroll.
	if !fr.isSuffix && shouldRejectAsOffscreen(d.inner.GetPlatformInfo(), cx, cy) {
		sel := extractSelector(step)
		describe := ""
		if sel != nil {
			describe = sel.Describe()
		}
		info := d.inner.GetPlatformInfo()
		sw, sh := 0, 0
		if info != nil {
			sw, sh = info.ScreenWidth, info.ScreenHeight
		}
		logger.Info("Flutter fallback: rejecting offscreen tap (node #%d at %d,%d on %dx%d screen)", fr.node.ID, cx, cy, sw, sh)
		return core.ErrorResult(
			fmt.Errorf("element %s located by Flutter VM service at (%d,%d) but that's outside the visible viewport of the %dx%d screen — element is likely in a lazy ListView cache buffer; use scrollUntilVisible to bring it into view first", describe, cx, cy, sw, sh),
			"",
		)
	}

	if fr.isSuffix {
		rightEdge := int(fr.node.Rect.Right * fr.pixelRatio)
		cx = rightEdge - int(12*fr.pixelRatio)

		// Pre-focus: tap center of the TextField so the suffix becomes tappable
		switch step.(type) {
		case *flow.TapOnStep, *flow.DoubleTapOnStep, *flow.LongPressOnStep:
			centerX, centerY := fr.bounds.Center()
			focusTap := &flow.TapOnPointStep{X: centerX, Y: centerY}
			focusTap.StepType = flow.StepTapOnPoint
			d.inner.Execute(focusTap)
			time.Sleep(500 * time.Millisecond)
		}

		logger.Info("Flutter fallback: suffix icon — targeting right edge (node #%d at %d,%d)", fr.node.ID, cx, cy)
	} else {
		logger.Info("Flutter fallback: executing at coordinates (node #%d at %d,%d)", fr.node.ID, cx, cy)
	}

	return d.executeWithCoordinates(step, fr.node, cx, cy, fr.bounds)
}

// shouldRejectAsOffscreen decides whether a Flutter-located tap point is
// likely outside the visible viewport (system-bar safe-area buffer, or fully
// off the screen). Permissive when screen size is unknown.
//
// Safe-area heuristic: top 3% (status bar / notch) and bottom 5% (nav bar /
// gesture area). Tappable content rarely lands here; when it does, the
// inner driver usually sees it directly without needing Flutter fallback.
func shouldRejectAsOffscreen(info *core.PlatformInfo, cx, cy int) bool {
	if info == nil || info.ScreenWidth <= 0 || info.ScreenHeight <= 0 {
		return false
	}
	if cx < 0 || cx >= info.ScreenWidth || cy < 0 || cy >= info.ScreenHeight {
		return true
	}
	topInset := info.ScreenHeight * 3 / 100
	bottomInset := info.ScreenHeight - info.ScreenHeight*5/100
	return cy < topInset || cy > bottomInset
}

// searchSemanticsTree searches the parsed semantics tree for matching nodes.
func searchSemanticsTree(root *SemanticsNode, sel *flow.Selector) []*SemanticsNode {
	var nodes []*SemanticsNode
	if sel.ID != "" {
		nodes = FindByIdentifier(root, sel.ID)
		if len(nodes) == 0 {
			nodes = FindByLabel(root, sel.ID)
		}
	}
	if len(nodes) == 0 && sel.Text != "" {
		nodes = FindByIdentifier(root, sel.Text)
		if len(nodes) == 0 {
			nodes = FindByLabel(root, sel.Text)
		}
		if len(nodes) == 0 {
			nodes = FindByHint(root, sel.Text)
		}
	}
	return nodes
}

// executeWithCoordinates dispatches the step using the element's coordinates.
func (d *FlutterDriver) executeWithCoordinates(step flow.Step, node *SemanticsNode, cx, cy int, bounds core.Bounds) *core.CommandResult {
	elemInfo := &core.ElementInfo{
		Text:   node.Label,
		Bounds: bounds,
		ID:     node.Identifier,
	}

	switch s := step.(type) {
	case *flow.TapOnStep:
		tapStep := &flow.TapOnPointStep{
			X:         cx,
			Y:         cy,
			LongPress: s.LongPress,
			Repeat:    s.Repeat,
		}
		tapStep.StepType = flow.StepTapOnPoint
		return d.inner.Execute(tapStep)

	case *flow.DoubleTapOnStep:
		tapStep := &flow.TapOnPointStep{
			X:      cx,
			Y:      cy,
			Repeat: 2,
		}
		tapStep.StepType = flow.StepTapOnPoint
		return d.inner.Execute(tapStep)

	case *flow.LongPressOnStep:
		tapStep := &flow.TapOnPointStep{
			X:         cx,
			Y:         cy,
			LongPress: true,
		}
		tapStep.StepType = flow.StepTapOnPoint
		return d.inner.Execute(tapStep)

	case *flow.AssertVisibleStep:
		return core.SuccessResult(
			fmt.Sprintf("Element found via Flutter VM service (node #%d)", node.ID),
			elemInfo,
		)

	case *flow.InputTextStep, *flow.CopyTextFromStep:
		tapStep := &flow.TapOnPointStep{X: cx, Y: cy}
		tapStep.StepType = flow.StepTapOnPoint
		tapResult := d.inner.Execute(tapStep)
		if !tapResult.Success {
			return tapResult
		}
		return d.inner.Execute(step)
	}

	// Unreachable: isElementFindingStep filters the same types handled above.
	return core.ErrorResult(fmt.Errorf("unsupported step type for Flutter fallback: %T", step), "")
}

// Close closes the VM Service client if connected.
func (d *FlutterDriver) Close() {
	d.client.Close()
	d.client = nil
}

// isElementFindingStep returns true for step types that involve finding an element.
func isElementFindingStep(step flow.Step) bool {
	switch step.(type) {
	case *flow.TapOnStep, *flow.DoubleTapOnStep, *flow.LongPressOnStep,
		*flow.AssertVisibleStep, *flow.InputTextStep, *flow.CopyTextFromStep:
		return true
	}
	return false
}

// isElementNotFoundError checks if a command result indicates an element-not-found error.
func isElementNotFoundError(result *core.CommandResult) bool {
	if result.Error == nil && result.Message == "" {
		return false
	}

	check := func(s string) bool {
		s = strings.ToLower(s)
		return strings.Contains(s, "not found") ||
			strings.Contains(s, "not visible") ||
			strings.Contains(s, "no such element") ||
			strings.Contains(s, "could not be located")
	}

	if result.Error != nil && check(result.Error.Error()) {
		return true
	}
	return result.Message != "" && check(result.Message)
}

// extractSelector gets the selector from element-finding steps.
func extractSelector(step flow.Step) *flow.Selector {
	switch s := step.(type) {
	case *flow.TapOnStep:
		return &s.Selector
	case *flow.DoubleTapOnStep:
		return &s.Selector
	case *flow.LongPressOnStep:
		return &s.Selector
	case *flow.AssertVisibleStep:
		return &s.Selector
	case *flow.InputTextStep:
		return &s.Selector
	case *flow.CopyTextFromStep:
		return &s.Selector
	}
	return nil
}

// --- Pass-through methods ---

func (d *FlutterDriver) Screenshot() ([]byte, error)       { return d.inner.Screenshot() }
func (d *FlutterDriver) Hierarchy() ([]byte, error)         { return d.inner.Hierarchy() }
func (d *FlutterDriver) GetState() *core.StateSnapshot      { return d.inner.GetState() }
func (d *FlutterDriver) GetPlatformInfo() *core.PlatformInfo { return d.inner.GetPlatformInfo() }
func (d *FlutterDriver) SetFindTimeout(ms int) {
	d.findTimeoutMs = ms
	d.inner.SetFindTimeout(ms)
}
func (d *FlutterDriver) SetWaitForIdleTimeout(ms int) error {
	return d.inner.SetWaitForIdleTimeout(ms)
}
func (d *FlutterDriver) SetContext(ctx context.Context) {
	d.ctx = ctx
	d.inner.SetContext(ctx)
}

func (d *FlutterDriver) parentContext() context.Context {
	if d.ctx != nil {
		return d.ctx
	}
	return context.Background()
}

// --- Optional interface forwarding ---

func (d *FlutterDriver) CDPState() *core.CDPInfo {
	if p, ok := d.inner.(core.CDPStateProvider); ok {
		return p.CDPState()
	}
	return nil
}

func (d *FlutterDriver) ForceStop(appID string) error {
	if m, ok := d.inner.(core.AppLifecycleManager); ok {
		return m.ForceStop(appID)
	}
	return fmt.Errorf("inner driver does not support ForceStop")
}

func (d *FlutterDriver) ClearAppData(appID string) error {
	if m, ok := d.inner.(core.AppLifecycleManager); ok {
		return m.ClearAppData(appID)
	}
	return fmt.Errorf("inner driver does not support ClearAppData")
}

func (d *FlutterDriver) DetectWebView() (*core.WebViewInfo, error) {
	if w, ok := d.inner.(core.WebViewDetector); ok {
		return w.DetectWebView()
	}
	return nil, fmt.Errorf("inner driver does not support DetectWebView")
}

// Inner returns the underlying driver for optional interface access.
func (d *FlutterDriver) Inner() core.Driver { return d.inner }
