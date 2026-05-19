package core

import (
	"context"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Driver defines the interface for executing commands on a device.
// Implementations: Appium, Native, Detox, etc.
// The Runner handles flow logic; Driver just executes individual commands.
type Driver interface {
	// Execute runs a single step and returns the result
	Execute(step flow.Step) *CommandResult

	// Screenshot captures the current screen as PNG
	Screenshot() ([]byte, error)

	// Hierarchy captures the UI hierarchy as JSON
	Hierarchy() ([]byte, error)

	// GetState returns the current device/app state
	GetState() *StateSnapshot

	// GetPlatformInfo returns device/platform information
	GetPlatformInfo() *PlatformInfo

	// SetFindTimeout sets the default timeout (in ms) for finding elements.
	// This is used by commandTimeout in flow config.
	SetFindTimeout(ms int)

	// SetWaitForIdleTimeout sets the wait for idle timeout (in ms).
	// 0 = disabled, >0 = wait up to N ms for device to be idle.
	// This is used by waitForIdleTimeout in flow config.
	SetWaitForIdleTimeout(ms int) error

	// SetContext sets the parent context for element-finding operations.
	// When set, driver polling loops use this as the parent context so that
	// external cancellation (e.g. runFlow timeout) can interrupt them.
	SetContext(ctx context.Context)
}

// CommandResult represents the outcome of executing a single command
type CommandResult struct {
	// Core outcome
	Success  bool          `json:"success"`
	Error    error         `json:"-"`
	Duration time.Duration `json:"duration"`

	// Human-readable output
	Message string `json:"message,omitempty"`

	// Element information (for tap, assert, scroll, etc.)
	Element *ElementInfo `json:"element,omitempty"`

	// Generic data for command-specific results
	// Examples: clipboard text, extracted AI text, generated random value
	Data interface{} `json:"data,omitempty"`

	// Debug information (internal details, not for reporting)
	Debug interface{} `json:"-"`
}

// SuccessResult creates a successful command result.
func SuccessResult(msg string, elem *ElementInfo) *CommandResult {
	return &CommandResult{
		Success: true,
		Message: msg,
		Element: elem,
	}
}

// ErrorResult creates a failed command result.
func ErrorResult(err error, msg string) *CommandResult {
	if msg == "" && err != nil {
		msg = err.Error()
	}
	return &CommandResult{
		Success: false,
		Error:   err,
		Message: msg,
	}
}

// ElementInfo represents information about a UI element
type ElementInfo struct {
	ID                 string            `json:"id,omitempty"`
	Text               string            `json:"text,omitempty"`
	Bounds             Bounds            `json:"bounds"`
	Visible            bool              `json:"visible"`
	Enabled            bool              `json:"enabled"`
	Focused            bool              `json:"focused,omitempty"`
	Checked            bool              `json:"checked,omitempty"`
	Selected           bool              `json:"selected,omitempty"`
	Class              string            `json:"class,omitempty"`
	AccessibilityLabel string            `json:"accessibilityLabel,omitempty"`
	Attributes         map[string]string `json:"attributes,omitempty"`
	// MatchNote explains a non-default match path the driver took to find the
	// element (e.g. "rescued: container marked visible=false but hosts visible
	// descendants" on iOS for RN container testIDs). Surfaced in step messages
	// and the JSON report so users know they're relying on relaxed matching.
	MatchNote string `json:"matchNote,omitempty"`
}

// Bounds represents element position and size
type Bounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Center returns the center point of the bounds
func (b Bounds) Center() (int, int) {
	return b.X + b.Width/2, b.Y + b.Height/2
}

// Contains checks if a point is within the bounds
func (b Bounds) Contains(x, y int) bool {
	return x >= b.X && x < b.X+b.Width && y >= b.Y && y < b.Y+b.Height
}

// VisiblePercentage returns the fraction of the element's area that overlaps
// with the screen (0.0 to 1.0). Matches Maestro's filterOutOfBounds logic:
// elements with less than 10% visible are considered off-screen.
func (b Bounds) VisiblePercentage(screenWidth, screenHeight int) float64 {
	if b.Width == 0 && b.Height == 0 {
		return 0
	}
	// Full-screen overlay
	if b.X <= 0 && b.Y <= 0 && b.X+b.Width >= screenWidth && b.Y+b.Height >= screenHeight {
		return 1.0
	}
	visibleX := max(0, min(b.X+b.Width, screenWidth)-max(b.X, 0))
	visibleY := max(0, min(b.Y+b.Height, screenHeight)-max(b.Y, 0))
	totalArea := b.Width * b.Height
	if totalArea == 0 {
		return 0
	}
	return float64(visibleX*visibleY) / float64(totalArea)
}

// CenterInside checks if the center of inner bounds is inside outer bounds.
func (b Bounds) CenterInside(outer Bounds) bool {
	cx, cy := b.Center()
	return cx >= outer.X && cx <= outer.X+outer.Width &&
		cy >= outer.Y && cy <= outer.Y+outer.Height
}

// HasNonASCII checks if text contains non-ASCII characters.
func HasNonASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] > 127 {
			return true
		}
	}
	return false
}

// StateSnapshot captures the current device/app state
type StateSnapshot struct {
	AppState        string       `json:"appState,omitempty"`        // foreground, background, not_running
	Orientation     string       `json:"orientation,omitempty"`     // portrait, landscape
	KeyboardVisible bool         `json:"keyboardVisible"`           // Is keyboard shown
	FocusedElement  *ElementInfo `json:"focusedElement,omitempty"`  // Currently focused element
	ClipboardText   string       `json:"clipboardText,omitempty"`   // Clipboard contents
	CurrentActivity string       `json:"currentActivity,omitempty"` // Android activity
	CurrentScreen   string       `json:"currentScreen,omitempty"`   // Screen identifier
}

// PlatformInfo contains device and platform details
type PlatformInfo struct {
	Platform     string `json:"platform"`               // ios, android
	OSVersion    string `json:"osVersion"`              // e.g., "17.0", "14"
	DeviceName   string `json:"deviceName"`             // e.g., "iPhone 15 Pro", "Pixel 8"
	DeviceID     string `json:"deviceId"`               // Unique device identifier
	IsSimulator  bool   `json:"isSimulator"`            // Simulator/emulator vs real device
	ScreenWidth  int    `json:"screenWidth,omitempty"`  // Screen width in pixels
	ScreenHeight int    `json:"screenHeight,omitempty"` // Screen height in pixels
	AppID        string `json:"appId,omitempty"`        // Bundle ID / Package name
	AppVersion   string `json:"appVersion,omitempty"`   // App version
}

// ExecutedBy indicates what component executed a step
type ExecutedBy string

// ExecutedBy values
const (
	ExecutedByDriver ExecutedBy = "driver" // Executed by the Driver (Appium, native, etc.)
	ExecutedByRunner ExecutedBy = "runner" // Executed by the Runner (JS, subflow, etc.)
)

// WebViewInfo describes the WebView/browser context detected on screen.
type WebViewInfo struct {
	Type        string // "webview" or "browser"
	PackageName string // e.g., "com.wdiodemoapp" or "com.android.chrome"
	ClassName   string // e.g., "android.webkit.WebView" (only for type=webview)
}

// CDPInfo describes the Chrome DevTools Protocol socket state.
type CDPInfo struct {
	Available bool   // CDP socket detected
	Socket    string // e.g., "webview_devtools_remote_12345"
}

// CDPStateProvider is an optional interface drivers can implement
// to provide real-time CDP socket state from background monitoring.
type CDPStateProvider interface {
	CDPState() *CDPInfo
}

// AppLifecycleManager is an optional interface drivers can implement
// to handle app lifecycle operations (force-stop, clear data) on-device
// instead of via ADB shell.
type AppLifecycleManager interface {
	ForceStop(appID string) error
	ClearAppData(appID string) error
}

// WebViewDetector is an optional interface drivers can implement
// to detect if the current screen contains a WebView or browser.
type WebViewDetector interface {
	DetectWebView() (*WebViewInfo, error)
}

// TypingFrequencyConfigurer is an optional interface drivers can implement
// to control typing speed. Currently only WDA (iOS) supports this.
type TypingFrequencyConfigurer interface {
	SetTypingFrequency(freq int) error
}

// SessionEnsurer is an optional interface drivers can implement to create
// a session before flow execution starts. This is needed when a flow has
// no launchApp step (e.g. stopApp → openLink pattern) so that WDA commands
// have an active session. If launchApp runs later, it replaces the session.
type SessionEnsurer interface {
	EnsureSession(appID string) error
}

// Unwrap returns the innermost driver, stripping any wrapper layers
// (e.g. FlutterDriver). Use this to access optional interfaces that
// wrappers may not forward.
func Unwrap(d Driver) Driver {
	for {
		if u, ok := d.(interface{ Inner() Driver }); ok {
			d = u.Inner()
		} else {
			return d
		}
	}
}

// LogEntry represents a single log message captured during execution
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`  // debug, info, warn, error
	Source    string    `json:"source"` // device, app, driver
	Message   string    `json:"message"`
}
