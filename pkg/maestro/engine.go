package maestro

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"
)

// Opt-in agent features for hosts other than the flow runner (an AI agent
// engine): window info in snapshots, the foreground app, a launch that waits
// until the app is in front, settle and hash over chosen windows, and one
// call that observes the screen after an action.
//
// The flow runner calls none of these, and the existing Adapter methods send
// exactly what they sent before. Each feature needs an agent that lists its
// capability in Session.create (see SessionResult.Has).

// Capability names an agent lists in Session.create.
const (
	CapSnapshotWindowInfo      = "snapshot.windowInfo"
	CapSnapshotIncludeKeyboard = "snapshot.includeKeyboard"
	CapSnapshotVisibleBounds   = "snapshot.visibleBounds"
	CapDeviceGetForeground     = "device.getForeground"
	CapLaunchWaitForForeground = "launch.waitForForeground"
	CapSettleScope             = "settle.scope"
	CapSettleResult            = "settle.result"
	CapTreeHashScope           = "treeHash.scope"
	CapTreeHashLayout          = "treeHash.layout"
	CapUIObserve               = "ui.observe"
)

// Scopes for WaitForSettleWith, TreeHashWith and Observe.
const (
	ScopeActive = "active" // the active window only, as WaitForSettle and TreeHash
	ScopeApp    = "app"    // every application window, with dialogs and popups; no keyboard or system windows
	ScopeAll    = "all"    // every window but the keyboard
)

// Hash modes for TreeHashWith.
const (
	ModeContent = "content" // text and content-desc, as TreeHash
	ModeLayout  = "layout"  // class, text, content-desc and bounds, as the settle signature
)

// SnapshotOptions are UI.snapshot's opt-in params.
type SnapshotOptions struct {
	WaitForIdleMs int
	// WindowInfo adds window attributes to each window root (window-id,
	// window-type, window-layer, window-active, window-focused, window-title,
	// package) and width, height, foreground-package, keyboard and
	// keyboard-top to <hierarchy>. It also turns on visible-bounds.
	WindowInfo bool
	// IncludeKeyboard dumps the keyboard window too.
	IncludeKeyboard bool
	// VisibleBounds adds visible-bounds where a node is clipped by its window
	// or a scrolling ancestor.
	VisibleBounds bool
}

// SnapshotWith returns UI.snapshot's XML with the opt-in params. Without any
// of them it is Snapshot.
func (a *Adapter) SnapshotWith(opts SnapshotOptions) (string, error) {
	params := map[string]interface{}{"waitForIdleMs": opts.WaitForIdleMs}
	if opts.WindowInfo {
		params["windowInfo"] = true
	}
	if opts.IncludeKeyboard {
		params["includeKeyboard"] = true
	}
	if opts.VisibleBounds {
		params["visibleBounds"] = true
	}
	resp, err := a.client.Call("UI.snapshot", params)
	if err != nil {
		return "", err
	}
	var result SourceResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parse snapshot result: %w", err)
	}
	return result.XML, nil
}

// Foreground is what is in front: Device.getForeground, and the state a
// launch with LaunchAppWait found.
type Foreground struct {
	Package        string `json:"package"`
	WindowID       int    `json:"windowId"`
	KeyguardLocked bool   `json:"keyguardLocked"`
	ScreenOn       bool   `json:"screenOn"`
}

// GetForeground reads the foreground app's package, keyguard and screen
// state on the device, with no shell call.
func (a *Adapter) GetForeground() (*Foreground, error) {
	resp, err := a.client.Call("Device.getForeground", nil)
	if err != nil {
		return nil, err
	}
	var fg Foreground
	if err := json.Unmarshal(resp.Result, &fg); err != nil {
		return nil, fmt.Errorf("parse getForeground result: %w", err)
	}
	return &fg, nil
}

// LaunchResult is a successful LaunchAppWait.
type LaunchResult struct {
	Foreground string `json:"foreground"`
	WindowID   int    `json:"windowId"`
	WaitedMs   int64  `json:"waitedMs"`
}

// LaunchError is LaunchAppWait's failure when the device was locked
// (Code "screen_locked", nothing launched) or the app did not come to the
// front in time (Code "not_foreground").
type LaunchError struct {
	Code           string
	Message        string
	Foreground     string `json:"foreground"`
	KeyguardLocked bool   `json:"keyguardLocked"`
	ScreenOn       bool   `json:"screenOn"`
	// TopWindowTitle names what covers the app, such as a USB or permission
	// dialog (its title, else its package). Set for not_foreground.
	TopWindowTitle string `json:"topWindowTitle"`
}

func (e *LaunchError) Error() string { return e.Code + ": " + e.Message }

// LaunchAppWait launches appID and waits up to timeout until it is the
// foreground app with a window that has content. A locked or dark screen
// fails at once with a *LaunchError (screen_locked), and an app that does not
// come to the front fails with a *LaunchError (not_foreground) saying what is
// in front instead.
func (a *Adapter) LaunchAppWait(appID string, arguments map[string]interface{}, timeout time.Duration) (*LaunchResult, error) {
	params := map[string]interface{}{
		"appId":               appID,
		"waitForForegroundMs": timeout.Milliseconds(),
	}
	if len(arguments) > 0 {
		params["arguments"] = arguments
	}
	resp, err := a.client.CallWithTimeout("Device.launchApp", params, timeout+defaultCallTimeout)
	if err != nil {
		return nil, asLaunchError(err)
	}
	var result LaunchResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse launchApp result: %w", err)
	}
	return &result, nil
}

func asLaunchError(err error) error {
	ep, ok := err.(*ErrorPayload)
	if !ok || (ep.Code != "screen_locked" && ep.Code != "not_foreground") {
		return err
	}
	le := &LaunchError{Code: ep.Code, Message: ep.Message}
	if len(ep.Data) > 0 {
		_ = json.Unmarshal(ep.Data, le)
	}
	return le
}

// SettleOptions are UI.waitForSettle's opt-in params.
type SettleOptions struct {
	TimeoutMs int
	QuietMs   int
	// Scope is ScopeActive, ScopeApp or ScopeAll; empty means ScopeActive.
	Scope string
	// IgnoreTextChanges leaves text and content-desc out of the settle
	// signature, so a clock or ticker does not hold it up.
	IgnoreTextChanges bool
}

// SettleResult is what a scoped settle reports.
type SettleResult struct {
	Settled   bool  `json:"settled"`
	ElapsedMs int64 `json:"elapsedMs"`
	Reads     int   `json:"reads"`
	// LastChangeMs is when, from the start, the tree last changed; 0 when it
	// never did.
	LastChangeMs int64 `json:"lastChangeMs"`
	// Hash is the final layout hash of the scope.
	Hash int64 `json:"hash"`
}

// WaitForSettleWith waits for the windows in scope to settle and says how it
// went. WaitForSettle is unchanged.
func (a *Adapter) WaitForSettleWith(opts SettleOptions) (*SettleResult, error) {
	scope := opts.Scope
	if scope == "" {
		scope = ScopeActive
	}
	params := map[string]interface{}{
		"timeout": opts.TimeoutMs,
		"quiet":   opts.QuietMs,
		"scope":   scope,
	}
	if opts.IgnoreTextChanges {
		params["ignoreTextChanges"] = true
	}
	resp, err := a.client.CallWithTimeout("UI.waitForSettle", params,
		time.Duration(opts.TimeoutMs)*time.Millisecond+defaultCallTimeout)
	if err != nil {
		return nil, err
	}
	var result SettleResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse waitForSettle result: %w", err)
	}
	return &result, nil
}

// TreeHashWith hashes the windows in scope (empty: ScopeActive) in mode
// (empty: ModeContent). TreeHash is unchanged.
func (a *Adapter) TreeHashWith(scope, mode string) (uint64, error) {
	if scope == "" {
		scope = ScopeActive
	}
	if mode == "" {
		mode = ModeContent
	}
	resp, err := a.client.Call("UI.treeHash", map[string]interface{}{"scope": scope, "mode": mode})
	if err != nil {
		return 0, err
	}
	var result struct {
		Hash int64 `json:"hash"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return 0, fmt.Errorf("parse treeHash result: %w", err)
	}
	return uint64(result.Hash), nil
}

// ObserveOptions are UI.observe's params.
type ObserveOptions struct {
	// Settle, when set, settles before the snapshot is read.
	Settle          *SettleOptions
	IncludeKeyboard bool
	Screenshot      bool
	// ScreenshotMaxEdge caps the screenshot's long side (0: the agent's
	// default, 1024); ScreenshotQuality is the JPEG quality (0: 75).
	ScreenshotMaxEdge int
	ScreenshotQuality int
}

// ObserveResult is what UI.observe reports. XML has window info and visible
// bounds, as SnapshotWith with WindowInfo.
type ObserveResult struct {
	Settle     *SettleResult `json:"settle,omitempty"`
	XML        string        `json:"xml"`
	Foreground Foreground    `json:"foreground"`
	Keyboard   struct {
		Visible bool `json:"visible"`
		Top     int  `json:"top"`
	} `json:"keyboard"`
	HasScreenshot bool `json:"hasScreenshot"`
}

// Observe settles (when asked), then reads the snapshot, the foreground app,
// the keyboard and (when asked) a screenshot taken right after the tree, in
// one call. The image is nil without Screenshot.
func (a *Adapter) Observe(opts ObserveOptions) (*ObserveResult, []byte, error) {
	params := map[string]interface{}{}
	timeout := defaultCallTimeout
	if s := opts.Settle; s != nil {
		settle := map[string]interface{}{"timeoutMs": s.TimeoutMs, "quietMs": s.QuietMs}
		if s.Scope != "" {
			settle["scope"] = s.Scope
		}
		if s.IgnoreTextChanges {
			settle["ignoreTextChanges"] = true
		}
		params["settle"] = settle
		timeout += time.Duration(s.TimeoutMs) * time.Millisecond
	}
	if opts.IncludeKeyboard {
		params["includeKeyboard"] = true
	}
	if opts.Screenshot {
		params["screenshot"] = true
		if opts.ScreenshotMaxEdge > 0 {
			params["screenshotMaxEdge"] = opts.ScreenshotMaxEdge
		}
		if opts.ScreenshotQuality > 0 {
			params["screenshotQuality"] = opts.ScreenshotQuality
		}
	}
	resp, err := a.client.CallWithTimeout("UI.observe", params, timeout)
	if err != nil {
		return nil, nil, err
	}
	return parseObserve(resp)
}

// parseObserve reads UI.observe's reply: a JSON response, or with a
// screenshot one binary frame of [4-byte JSON length][JSON response][JPEG]
// after the request id.
func parseObserve(resp *Response) (*ObserveResult, []byte, error) {
	raw := resp.Result
	var image []byte
	if resp.BinaryData != nil {
		data := resp.BinaryData
		if len(data) < 4 {
			return nil, nil, fmt.Errorf("observe frame too short: %d bytes", len(data))
		}
		n := int(binary.BigEndian.Uint32(data[:4]))
		if n > len(data)-4 {
			return nil, nil, fmt.Errorf("observe frame JSON length %d exceeds frame (%d bytes)", n, len(data)-4)
		}
		var inner Response
		if err := json.Unmarshal(data[4:4+n], &inner); err != nil {
			return nil, nil, fmt.Errorf("parse observe frame: %w", err)
		}
		if inner.Error != nil {
			return nil, nil, inner.Error
		}
		raw = inner.Result
		image = data[4+n:]
	}
	var result ObserveResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, nil, fmt.Errorf("parse observe result: %w", err)
	}
	return &result, image, nil
}
