package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestBounds_Center(t *testing.T) {
	tests := []struct {
		bounds    Bounds
		expectedX int
		expectedY int
	}{
		{Bounds{X: 0, Y: 0, Width: 100, Height: 100}, 50, 50},
		{Bounds{X: 10, Y: 20, Width: 100, Height: 200}, 60, 120},
		{Bounds{X: 0, Y: 0, Width: 0, Height: 0}, 0, 0},
	}

	for _, tt := range tests {
		x, y := tt.bounds.Center()
		if x != tt.expectedX || y != tt.expectedY {
			t.Errorf("Bounds%+v.Center() = (%d, %d), want (%d, %d)",
				tt.bounds, x, y, tt.expectedX, tt.expectedY)
		}
	}
}

func TestBounds_Contains(t *testing.T) {
	bounds := Bounds{X: 10, Y: 10, Width: 100, Height: 100}

	tests := []struct {
		x, y     int
		expected bool
	}{
		{50, 50, true},    // Center
		{10, 10, true},    // Top-left corner
		{109, 109, true},  // Just inside bottom-right
		{110, 110, false}, // Exactly at boundary (exclusive)
		{0, 0, false},     // Outside
		{200, 200, false}, // Far outside
	}

	for _, tt := range tests {
		if got := bounds.Contains(tt.x, tt.y); got != tt.expected {
			t.Errorf("Bounds.Contains(%d, %d) = %v, want %v", tt.x, tt.y, got, tt.expected)
		}
	}
}

func TestCommandResult_Fields(t *testing.T) {
	result := CommandResult{
		Success:  true,
		Duration: 100 * time.Millisecond,
		Message:  "Tapped on button",
		Element: &ElementInfo{
			ID:      "btn-submit",
			Text:    "Submit",
			Visible: true,
			Enabled: true,
		},
	}

	if !result.Success {
		t.Error("Success should be true")
	}
	if result.Duration != 100*time.Millisecond {
		t.Errorf("Duration = %v, want 100ms", result.Duration)
	}
	if result.Element == nil {
		t.Error("Element should not be nil")
	}
	if result.Element.ID != "btn-submit" {
		t.Errorf("Element.ID = %s, want btn-submit", result.Element.ID)
	}
}

func TestElementInfo_Fields(t *testing.T) {
	elem := ElementInfo{
		ID:                 "elem-1",
		Text:               "Hello",
		Bounds:             Bounds{X: 10, Y: 20, Width: 100, Height: 50},
		Visible:            true,
		Enabled:            true,
		Focused:            false,
		Checked:            true,
		Selected:           false,
		Class:              "android.widget.Button",
		AccessibilityLabel: "Submit Button",
		Attributes: map[string]string{
			"resource-id": "com.app:id/submit",
		},
	}

	if elem.ID != "elem-1" {
		t.Errorf("ID = %s, want elem-1", elem.ID)
	}
	if elem.Bounds.Width != 100 {
		t.Errorf("Bounds.Width = %d, want 100", elem.Bounds.Width)
	}
	if !elem.Visible {
		t.Error("Visible should be true")
	}
	if !elem.Checked {
		t.Error("Checked should be true")
	}
	if elem.Attributes["resource-id"] != "com.app:id/submit" {
		t.Errorf("Attributes[resource-id] = %s, want com.app:id/submit", elem.Attributes["resource-id"])
	}
}

func TestStateSnapshot_Fields(t *testing.T) {
	state := StateSnapshot{
		AppState:        "foreground",
		Orientation:     "portrait",
		KeyboardVisible: true,
		FocusedElement: &ElementInfo{
			ID: "input-email",
		},
		ClipboardText:   "copied text",
		CurrentActivity: "com.app.MainActivity",
		CurrentScreen:   "LoginScreen",
	}

	if state.AppState != "foreground" {
		t.Errorf("AppState = %s, want foreground", state.AppState)
	}
	if !state.KeyboardVisible {
		t.Error("KeyboardVisible should be true")
	}
	if state.FocusedElement == nil {
		t.Error("FocusedElement should not be nil")
	}
	if state.ClipboardText != "copied text" {
		t.Errorf("ClipboardText = %s, want 'copied text'", state.ClipboardText)
	}
}

func TestPlatformInfo_Fields(t *testing.T) {
	info := PlatformInfo{
		Platform:     "android",
		OSVersion:    "14",
		DeviceName:   "Pixel 8",
		DeviceID:     "emulator-5554",
		IsSimulator:  true,
		ScreenWidth:  1080,
		ScreenHeight: 2400,
		AppID:        "com.example.app",
		AppVersion:   "1.2.3",
	}

	if info.Platform != "android" {
		t.Errorf("Platform = %s, want android", info.Platform)
	}
	if !info.IsSimulator {
		t.Error("IsSimulator should be true")
	}
	if info.ScreenWidth != 1080 {
		t.Errorf("ScreenWidth = %d, want 1080", info.ScreenWidth)
	}
}

func TestExecutedByConstants(t *testing.T) {
	if ExecutedByDriver != "driver" {
		t.Errorf("ExecutedByDriver = %s, want driver", ExecutedByDriver)
	}
	if ExecutedByRunner != "runner" {
		t.Errorf("ExecutedByRunner = %s, want runner", ExecutedByRunner)
	}
}

func TestSuccessResult(t *testing.T) {
	// With message and element
	elem := &ElementInfo{ID: "btn-1", Text: "OK"}
	result := SuccessResult("Tapped", elem)
	if !result.Success {
		t.Error("SuccessResult should set Success to true")
	}
	if result.Message != "Tapped" {
		t.Errorf("Message = %q, want %q", result.Message, "Tapped")
	}
	if result.Element != elem {
		t.Error("Element should match the provided element")
	}
	if result.Error != nil {
		t.Error("Error should be nil for success result")
	}

	// With nil element
	result2 := SuccessResult("Done", nil)
	if result2.Element != nil {
		t.Error("Element should be nil when nil passed")
	}
}

func TestErrorResult(t *testing.T) {
	// With error and message
	err := fmt.Errorf("not found")
	result := ErrorResult(err, "Element missing")
	if result.Success {
		t.Error("ErrorResult should set Success to false")
	}
	if result.Error != err {
		t.Error("Error should match the provided error")
	}
	if result.Message != "Element missing" {
		t.Errorf("Message = %q, want %q", result.Message, "Element missing")
	}

	// With error and empty message (should use err.Error())
	result2 := ErrorResult(err, "")
	if result2.Message != "not found" {
		t.Errorf("Message = %q, want %q (from err.Error())", result2.Message, "not found")
	}

	// With nil error and empty message
	result3 := ErrorResult(nil, "")
	if result3.Message != "" {
		t.Errorf("Message = %q, want empty", result3.Message)
	}

	// With nil error and custom message
	result4 := ErrorResult(nil, "custom msg")
	if result4.Message != "custom msg" {
		t.Errorf("Message = %q, want %q", result4.Message, "custom msg")
	}
}

func TestHasNonASCII(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"hello", false},
		{"Hello World 123!", false},
		{"", false},
		{"abc\t\n", false},
		{"\x7f", false},  // DEL is ASCII (127)
		{"\x80", true},   // first non-ASCII byte
		{"cafe\u0301", true}, // e with combining accent
		{"hello world", false},
	}

	for _, tt := range tests {
		got := HasNonASCII(tt.input)
		if got != tt.expected {
			t.Errorf("HasNonASCII(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestBounds_VisiblePercentage(t *testing.T) {
	screenW, screenH := 1080, 1920

	tests := []struct {
		name     string
		bounds   Bounds
		expected float64
	}{
		{"fully on-screen", Bounds{X: 100, Y: 100, Width: 200, Height: 200}, 1.0},
		{"fully off-screen right", Bounds{X: 1100, Y: 100, Width: 200, Height: 200}, 0.0},
		{"fully off-screen below", Bounds{X: 100, Y: 2000, Width: 200, Height: 200}, 0.0},
		{"fully off-screen left", Bounds{X: -300, Y: 100, Width: 200, Height: 200}, 0.0},
		{"fully off-screen above", Bounds{X: 100, Y: -300, Width: 200, Height: 200}, 0.0},
		{"half off-screen right", Bounds{X: 980, Y: 100, Width: 200, Height: 200}, 0.5},
		{"half off-screen bottom", Bounds{X: 100, Y: 1820, Width: 200, Height: 200}, 0.5},
		{"zero-size element", Bounds{X: 100, Y: 100, Width: 0, Height: 0}, 0.0},
		{"zero-width element", Bounds{X: 100, Y: 100, Width: 0, Height: 200}, 0.0},
		{"full-screen overlay", Bounds{X: 0, Y: 0, Width: 1080, Height: 1920}, 1.0},
		{"full-screen overlay with negative origin", Bounds{X: -10, Y: -10, Width: 1100, Height: 1940}, 1.0},
		{"exactly 10% visible", Bounds{X: 1060, Y: 100, Width: 200, Height: 200}, 0.10},
		{"barely visible (<10%)", Bounds{X: 1070, Y: 100, Width: 200, Height: 200}, 0.05},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.bounds.VisiblePercentage(screenW, screenH)
			diff := got - tt.expected
			if diff < -0.01 || diff > 0.01 {
				t.Errorf("Bounds%+v.VisiblePercentage(%d, %d) = %f, want %f",
					tt.bounds, screenW, screenH, got, tt.expected)
			}
		})
	}
}

func TestBounds_CenterInside(t *testing.T) {
	outer := Bounds{X: 0, Y: 0, Width: 100, Height: 100}

	tests := []struct {
		name     string
		inner    Bounds
		expected bool
	}{
		{"centered inside", Bounds{X: 25, Y: 25, Width: 50, Height: 50}, true},
		{"at origin", Bounds{X: 0, Y: 0, Width: 10, Height: 10}, true},
		{"at edge", Bounds{X: 90, Y: 90, Width: 10, Height: 10}, true},
		{"center outside right", Bounds{X: 200, Y: 50, Width: 20, Height: 20}, false},
		{"center outside below", Bounds{X: 50, Y: 200, Width: 20, Height: 20}, false},
		{"zero-size at center", Bounds{X: 50, Y: 50, Width: 0, Height: 0}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.inner.CenterInside(outer)
			if got != tt.expected {
				t.Errorf("Bounds%+v.CenterInside(%+v) = %v, want %v", tt.inner, outer, got, tt.expected)
			}
		})
	}
}

// --- Unwrap tests ---

// stubDriver is a minimal Driver for testing Unwrap.
type stubDriver struct{}

func (s *stubDriver) Execute(step flow.Step) *CommandResult { return nil }
func (s *stubDriver) Screenshot() ([]byte, error)           { return nil, nil }
func (s *stubDriver) Hierarchy() ([]byte, error)            { return nil, nil }
func (s *stubDriver) GetState() *StateSnapshot              { return nil }
func (s *stubDriver) GetPlatformInfo() *PlatformInfo        { return nil }
func (s *stubDriver) SetFindTimeout(ms int)                 {}
func (s *stubDriver) SetWaitForIdleTimeout(ms int) error    { return nil }
func (s *stubDriver) SetContext(ctx context.Context)        {}

// wrappingDriver wraps another driver and implements Inner().
type wrappingDriver struct {
	inner Driver
}

func (w *wrappingDriver) Execute(step flow.Step) *CommandResult { return nil }
func (w *wrappingDriver) Screenshot() ([]byte, error)           { return nil, nil }
func (w *wrappingDriver) Hierarchy() ([]byte, error)            { return nil, nil }
func (w *wrappingDriver) GetState() *StateSnapshot              { return nil }
func (w *wrappingDriver) GetPlatformInfo() *PlatformInfo        { return nil }
func (w *wrappingDriver) SetFindTimeout(ms int)                 {}
func (w *wrappingDriver) SetWaitForIdleTimeout(ms int) error    { return nil }
func (w *wrappingDriver) SetContext(ctx context.Context)        {}
func (w *wrappingDriver) Inner() Driver                         { return w.inner }

func TestUnwrap_NoWrapper(t *testing.T) {
	d := &stubDriver{}
	got := Unwrap(d)
	if got != d {
		t.Error("Unwrap on non-wrapper should return the same driver")
	}
}

func TestUnwrap_SingleLayer(t *testing.T) {
	inner := &stubDriver{}
	wrapper := &wrappingDriver{inner: inner}
	got := Unwrap(wrapper)
	if got != inner {
		t.Error("Unwrap should return the inner driver from a single wrapper")
	}
}

func TestUnwrap_DoubleLayer(t *testing.T) {
	innermost := &stubDriver{}
	mid := &wrappingDriver{inner: innermost}
	outer := &wrappingDriver{inner: mid}
	got := Unwrap(outer)
	if got != innermost {
		t.Error("Unwrap should unwrap through multiple layers to the innermost driver")
	}
}

// sessionEnsurerDriver is a driver that also implements SessionEnsurer.
type sessionEnsurerDriver struct {
	stubDriver
	ensureCalled bool
	lastAppID    string
}

func (s *sessionEnsurerDriver) EnsureSession(appID string) error {
	s.ensureCalled = true
	s.lastAppID = appID
	return nil
}

func TestUnwrap_ExposesSessionEnsurer(t *testing.T) {
	inner := &sessionEnsurerDriver{}
	wrapper := &wrappingDriver{inner: inner}

	unwrapped := Unwrap(wrapper)
	ensurer, ok := unwrapped.(SessionEnsurer)
	if !ok {
		t.Fatal("Unwrap should expose SessionEnsurer from inner driver")
	}

	if err := ensurer.EnsureSession("com.example.app"); err != nil {
		t.Fatalf("EnsureSession returned error: %v", err)
	}
	if !inner.ensureCalled {
		t.Error("EnsureSession was not called on the inner driver")
	}
	if inner.lastAppID != "com.example.app" {
		t.Errorf("EnsureSession appID = %q, want %q", inner.lastAppID, "com.example.app")
	}
}

func TestLogEntry_Fields(t *testing.T) {
	now := time.Now()
	entry := LogEntry{
		Timestamp: now,
		Level:     "error",
		Source:    "device",
		Message:   "App crashed",
	}

	if entry.Timestamp != now {
		t.Error("Timestamp mismatch")
	}
	if entry.Level != "error" {
		t.Errorf("Level = %s, want error", entry.Level)
	}
	if entry.Source != "device" {
		t.Errorf("Source = %s, want device", entry.Source)
	}
	if entry.Message != "App crashed" {
		t.Errorf("Message = %s, want 'App crashed'", entry.Message)
	}
}
