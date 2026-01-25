package core

import (
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

// LogEntry represents a single log message captured during execution
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`  // debug, info, warn, error
	Source    string    `json:"source"` // device, app, driver
	Message   string    `json:"message"`
}
