// Package devicelab_ios provides the Go client for the devicelab-ios-runner
// XCUITest-based iOS driver. The runner source lives at
// /Users/omnarayan/work/support-tools/devicelab-ios-runner and is a verbatim
// copy of callstackincubator/agent-device's ios-runner (MIT-licensed). This
// Go package is the translation layer: it converts maestro-runner flow steps
// into agent-device's wire commands and decodes the responses.
//
// The Swift wire protocol is defined by agent-device's RunnerTests+Models.swift
// (Command + Response + DataPayload + SnapshotNode). The shapes here mirror
// that schema 1:1.
package devicelab_ios

// CommandType matches agent-device's CommandType enum verbatim. See
// RunnerTests+Models.swift in the runner project.
type CommandType string

const (
	CmdTap              CommandType = "tap"
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
)

// Command is the wire request envelope. Mirrors the Swift Command struct
// field-for-field. All fields beyond `command` are optional; the runner
// ignores ones it doesn't need for the given command.
type Command struct {
	Command         CommandType `json:"command"`
	AppBundleID     string      `json:"appBundleId,omitempty"`
	Text            string      `json:"text,omitempty"`
	SelectorKey     string      `json:"selectorKey,omitempty"`
	SelectorValue   string      `json:"selectorValue,omitempty"`
	DelayMs         *int        `json:"delayMs,omitempty"`
	TextEntryMode   string      `json:"textEntryMode,omitempty"`
	ClearFirst      *bool       `json:"clearFirst,omitempty"`
	Action          string      `json:"action,omitempty"`
	X               *float64    `json:"x,omitempty"`
	Y               *float64    `json:"y,omitempty"`
	Button          string      `json:"button,omitempty"`
	RemoteButton    string      `json:"remoteButton,omitempty"`
	Count           *float64    `json:"count,omitempty"`
	IntervalMs      *float64    `json:"intervalMs,omitempty"`
	DoubleTap       *bool       `json:"doubleTap,omitempty"`
	PauseMs         *float64    `json:"pauseMs,omitempty"`
	Pattern         string      `json:"pattern,omitempty"`
	X2              *float64    `json:"x2,omitempty"`
	Y2              *float64    `json:"y2,omitempty"`
	DurationMs      *float64    `json:"durationMs,omitempty"`
	Direction       string      `json:"direction,omitempty"`
	Orientation     string      `json:"orientation,omitempty"`
	Scale           *float64    `json:"scale,omitempty"`
	OutPath         string      `json:"outPath,omitempty"`
	Fps             *int        `json:"fps,omitempty"`
	Quality         *int        `json:"quality,omitempty"`
	InteractiveOnly *bool       `json:"interactiveOnly,omitempty"`
	Compact         *bool       `json:"compact,omitempty"`
	Depth           *int        `json:"depth,omitempty"`
	Scope           string      `json:"scope,omitempty"`
	Raw             *bool       `json:"raw,omitempty"`
	Fullscreen      *bool       `json:"fullscreen,omitempty"`
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
}

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
)
