// Package devicelab_ios provides the Go client for the devicelab-ios-runner
// XCUITest-based iOS driver. This package is the translation layer: it converts
// maestro-runner flow steps into the runner's wire commands and decodes the
// responses.
//
// The runner source that actually ships and gets built lives in this repo at
// drivers/ios/DevicelabIOSRunner. It began as a copy of
// callstackincubator/agent-device's ios-runner (MIT-licensed) and has since
// diverged with local additions — see ATTRIBUTION.md there, and PROTOCOL.md for
// the command surface. The standalone clone under support-tools is a separate,
// older checkout; editing it does not change what ships.
//
// The Swift wire protocol is defined by RunnerTests+Models.swift (Command +
// Response + DataPayload + SnapshotNode). The shapes here mirror that schema,
// so a field added on one side needs adding on the other.
package devicelab_ios

// CommandType matches agent-device's CommandType enum verbatim. See
// RunnerTests+Models.swift in the runner project.
type CommandType string

const (
	CmdTap CommandType = "tap"
	// CmdTapBySelector — local extension. The runner walks its full XCUI
	// tree, finds the best liberal match (substring + case-insensitive +
	// prefer-editable-inputs ranking, mirroring Go-side matchesSelector),
	// taps at its center, returns {ok:true} on hit or {ok:false, snapshot}
	// on miss so the Go side can fall back to the existing snapshot+filter
	// path. Used by handleTapOn for simple id/text selectors as a faster
	// + more reliable alternative to the two-trip snapshot+filter+tap.
	CmdTapBySelector    CommandType = "tapBySelector"
	CmdMouseClick       CommandType = "mouseClick"
	CmdTapSeries        CommandType = "tapSeries"
	CmdLongPress        CommandType = "longPress"
	CmdInteractionFrame CommandType = "interactionFrame"
	CmdDrag             CommandType = "drag"
	CmdDragSeries       CommandType = "dragSeries"
	CmdRemotePress      CommandType = "remotePress"
	CmdType             CommandType = "type"
	CmdSwipe            CommandType = "swipe"
	CmdFindText         CommandType = "findText"
	CmdQuerySelector    CommandType = "querySelector"
	CmdReadText         CommandType = "readText"
	CmdSnapshot         CommandType = "snapshot"
	CmdScreenshot       CommandType = "screenshot"
	CmdBack             CommandType = "back"
	CmdBackInApp        CommandType = "backInApp"
	CmdBackSystem       CommandType = "backSystem"
	CmdHome             CommandType = "home"
	CmdRotate           CommandType = "rotate"
	CmdAppSwitcher      CommandType = "appSwitcher"
	CmdKeyboardDismiss  CommandType = "keyboardDismiss"
	CmdAddMedia         CommandType = "addMedia"
	CmdAlert            CommandType = "alert"
	CmdPinch            CommandType = "pinch"
	CmdRecordStart      CommandType = "recordStart"
	CmdRecordStop       CommandType = "recordStop"
	CmdUptime           CommandType = "uptime"
	CmdShutdown         CommandType = "shutdown"
	// Local extension: take two screenshots in-process and return the
	// fraction of differing pixels. Used by waitForAnimationToEnd to
	// avoid two PNG roundtrips per iteration.
	CmdIdleCheck CommandType = "idleCheck"
	// Local extension: runner-side wait-for-idle loop. Eliminates the
	// HTTP RTT per poll iteration we were paying with idleCheck.
	CmdAwaitIdle CommandType = "awaitIdle"
	// Local extension: read / set the device's light-dark appearance
	// through XCUIDevice. `simctl ui appearance` covers only simulators,
	// so these are what make dark mode work on physical devices.
	CmdAppearance    CommandType = "appearance"
	CmdSetAppearance CommandType = "setAppearance"
	// CmdIdle — local extension: wait, capped by Command.TimeoutMs, for the
	// target app to go quiescent including animations (XCTest's own
	// pre/post-event check, run on demand). Read-only: the runner never
	// activates the app for it. Answers ResponseData.Idle and WaitedMs.
	CmdIdle CommandType = "idle"
)

// Command is the wire request envelope. Mirrors the Swift Command struct
// field-for-field. All fields beyond `command` are optional; the runner
// ignores ones it doesn't need for the given command.
type Command struct {
	Command       CommandType `json:"command"`
	AppBundleID   string      `json:"appBundleId,omitempty"`
	Text          string      `json:"text,omitempty"`
	SelectorKey   string      `json:"selectorKey,omitempty"`
	SelectorValue string      `json:"selectorValue,omitempty"`
	DelayMs       *int        `json:"delayMs,omitempty"`
	TextEntryMode string      `json:"textEntryMode,omitempty"`
	ClearFirst    *bool       `json:"clearFirst,omitempty"`
	Action        string      `json:"action,omitempty"`
	X             *float64    `json:"x,omitempty"`
	Y             *float64    `json:"y,omitempty"`
	Button        string      `json:"button,omitempty"`
	RemoteButton  string      `json:"remoteButton,omitempty"`
	Count         *float64    `json:"count,omitempty"`
	IntervalMs    *float64    `json:"intervalMs,omitempty"`
	DoubleTap     *bool       `json:"doubleTap,omitempty"`
	PauseMs       *float64    `json:"pauseMs,omitempty"`
	Pattern       string      `json:"pattern,omitempty"`
	X2            *float64    `json:"x2,omitempty"`
	Y2            *float64    `json:"y2,omitempty"`
	DurationMs    *float64    `json:"durationMs,omitempty"`
	// MoveDurationMs — drag only: how long the movement takes. DurationMs is
	// the press-before-move hold on that command; older runners ignore this
	// field, so swipe/scroll keep their wire shape.
	MoveDurationMs  *float64 `json:"moveDurationMs,omitempty"`
	Direction       string   `json:"direction,omitempty"`
	Orientation     string   `json:"orientation,omitempty"`
	Scale           *float64 `json:"scale,omitempty"`
	OutPath         string   `json:"outPath,omitempty"`
	MediaName       string   `json:"mediaName,omitempty"`
	MimeType        string   `json:"mimeType,omitempty"`
	MediaData       string   `json:"mediaData,omitempty"` // base64-encoded file bytes (addMedia)
	Fps             *int     `json:"fps,omitempty"`
	Quality         *int     `json:"quality,omitempty"`
	InteractiveOnly *bool    `json:"interactiveOnly,omitempty"`
	Compact         *bool    `json:"compact,omitempty"`
	Depth           *int     `json:"depth,omitempty"`
	Scope           string   `json:"scope,omitempty"`
	Raw             *bool    `json:"raw,omitempty"`
	Fullscreen      *bool    `json:"fullscreen,omitempty"`
	Appearance      string   `json:"appearance,omitempty"` // "dark" or "light" (setAppearance)
	// TimeoutMs — idle only: the cap on the wait. A pointer so an explicit 0
	// ("answer now, do not wait") reaches the runner; nil means its default
	// of 1000ms. The runner clamps it to 10000ms.
	TimeoutMs *float64 `json:"timeoutMs,omitempty"`
}

// Response is the wire response envelope. The runner returns one of these
// for every request.
type Response struct {
	Ok    bool          `json:"ok"`
	Data  *ResponseData `json:"data,omitempty"`
	Error *ErrorPayload `json:"error,omitempty"`
}

// ResponseData is the union-shaped data payload. The field set depends on
// the command — the Swift side returns whichever fields are relevant for
// that command type.
type ResponseData struct {
	Message              string         `json:"message,omitempty"`
	Text                 string         `json:"text,omitempty"`
	Found                *bool          `json:"found,omitempty"`
	Items                []string       `json:"items,omitempty"`
	Nodes                []SnapshotNode `json:"nodes,omitempty"`
	Truncated            *bool          `json:"truncated,omitempty"`
	GestureStartUptimeMs *float64       `json:"gestureStartUptimeMs,omitempty"`
	GestureEndUptimeMs   *float64       `json:"gestureEndUptimeMs,omitempty"`
	X                    *float64       `json:"x,omitempty"`
	Y                    *float64       `json:"y,omitempty"`
	X2                   *float64       `json:"x2,omitempty"`
	Y2                   *float64       `json:"y2,omitempty"`
	ReferenceWidth       *float64       `json:"referenceWidth,omitempty"`
	ReferenceHeight      *float64       `json:"referenceHeight,omitempty"`
	CurrentUptimeMs      *float64       `json:"currentUptimeMs,omitempty"`
	Visible              *bool          `json:"visible,omitempty"`
	WasVisible           *bool          `json:"wasVisible,omitempty"`
	Dismissed            *bool          `json:"dismissed,omitempty"`
	Orientation          string         `json:"orientation,omitempty"`
	PngBase64            string         `json:"pngBase64,omitempty"`
	DiffFraction         *float64       `json:"diffFraction,omitempty"`
	// Identifier — runner-side extension. Set by tapBySelector so the Go
	// side can populate lastTappedIdentifier for the next inputText to
	// hint the type command at which element to focus.
	Identifier string `json:"identifier,omitempty"`
	// Appearance — "dark" or "light", from the appearance/setAppearance
	// commands. setAppearance reports what actually took effect rather
	// than echoing the request.
	Appearance string `json:"appearance,omitempty"`
	// AppState — the target app's XCUIApplication.state at snapshot time:
	// "runningForeground", "runningBackground", "runningBackgroundSuspended",
	// "notRunning" or "unknown". A snapshot is not an interaction command,
	// so the runner never auto-activates the app first; this is its true
	// state. The node tree cannot report it reliably — per-node hittable
	// oscillates while a screen animates into the background — so a caller
	// that needs to know the app is frontmost reads this, not the tree.
	AppState string `json:"appState,omitempty"`
	// Verified — type only: the runner's read-back of the field. true means
	// the value read back as typed, false means it did not (the call then
	// also fails with TEXT_ENTRY_MISMATCH, and this data still comes back
	// with the error). nil means the value could not be read (a secure
	// field, an element that no longer resolves), so the text was typed but
	// never checked — callers must not treat nil as success-verified.
	Verified *bool `json:"verified,omitempty"`
	// Repaired — type only: true when the first attempt read back wrong and
	// the runner cleared the field and typed again. A runner that predates
	// the field omits it (nil) on every response.
	Repaired *bool `json:"repaired,omitempty"`
	// Source — snapshot only: which reader produced Nodes, SnapshotSourceXCTest
	// or SnapshotSourcePrivateAX. The private accessibility reader is the
	// fallback for trees the public API cannot serialize (deep React Native
	// screens); it covers more of the tree and computes hittable from geometry
	// alone, so two snapshots from different sources are not comparable node
	// for node. Empty from a runner that predates the field.
	Source string `json:"source,omitempty"`
	// Idle — idle only: true only when XCTest saw the app's event loop idle
	// and its animations finish within the cap. false when the cap passed
	// first, the app is not in the foreground, TimeoutMs was 0, or the
	// runner's quiescence API is missing (Message says which). An unknown is
	// never reported as idle.
	Idle *bool `json:"idle,omitempty"`
	// WaitedMs — idle only: how long the runner actually waited. It can
	// exceed the cap when XCTest spindumps an app that failed to go idle.
	WaitedMs *float64 `json:"waitedMs,omitempty"`
}

// Values of ResponseData.Source.
const (
	SnapshotSourceXCTest    = "xctest"
	SnapshotSourcePrivateAX = "privateAX"
)

// SnapshotNode mirrors the Swift wire model. Tree is reconstructed by
// reading ParentIndex back-references; the runner sends a flat slice.
// PlaceholderValue is a local extension on top of agent-device's schema —
// the runner populates it so YAML flows can match by placeholder text.
type SnapshotNode struct {
	Index              int          `json:"index"`
	Type               string       `json:"type"`
	Label              string       `json:"label,omitempty"`
	Identifier         string       `json:"identifier,omitempty"`
	Value              string       `json:"value,omitempty"`
	PlaceholderValue   string       `json:"placeholderValue,omitempty"`
	Rect               SnapshotRect `json:"rect"`
	Enabled            bool         `json:"enabled"`
	Focused            bool         `json:"focused,omitempty"`
	Selected           bool         `json:"selected,omitempty"`
	Hittable           bool         `json:"hittable"`
	Depth              int          `json:"depth"`
	ParentIndex        *int         `json:"parentIndex,omitempty"`
	HiddenContentAbove *bool        `json:"hiddenContentAbove,omitempty"`
	HiddenContentBelow *bool        `json:"hiddenContentBelow,omitempty"`
}

// SnapshotRect — element bounds in app coordinate space.
type SnapshotRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// ErrorPayload mirrors the Swift wire model.
type ErrorPayload struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// Error codes — emitted by the runner. Not all are present in agent-device's
// code; document the ones we see in practice for branching from Go.
const (
	ErrUnsupportedOperation = "UNSUPPORTED_OPERATION"
	ErrElementNotFound      = "ELEMENT_NOT_FOUND"
	ErrAmbiguousMatch       = "AMBIGUOUS_MATCH"
	ErrTextEntryMismatch    = "TEXT_ENTRY_MISMATCH"
	// ErrSnapshotFailed — no tree could be read (the query failed or timed
	// out, usually because the app is suspended or busy). The response data
	// still carries AppState. Older runners answered this case with ok and
	// an empty node list, indistinguishable from an empty screen.
	ErrSnapshotFailed = "SNAPSHOT_FAILED"
	// ErrNoTargetApp — a read-only command (snapshot, idle) named no app and
	// the runner could not resolve the frontmost one. Older runners launched
	// their own host app here, covering whatever was on screen.
	ErrNoTargetApp = "NO_TARGET_APP"
)

// isSnapshotFailure reports whether err is the runner saying it could not
// read a tree this time. Polling callers treat it like "not there yet" and
// keep polling; it only becomes the answer once their deadline passes.
func isSnapshotFailure(err error) bool {
	re, ok := IsRunnerError(err)
	return ok && re.Code == ErrSnapshotFailed
}
