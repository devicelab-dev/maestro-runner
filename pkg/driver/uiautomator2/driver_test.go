package uiautomator2

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// writeJSON encodes data as JSON to the response writer.
func writeJSON(w http.ResponseWriter, data interface{}) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// ============================================================================
// Mock UIA2Client
// ============================================================================

type MockUIA2Client struct {
	// Config
	findElementFunc    func(strategy, selector string) (*uiautomator2.Element, error)
	activeElementFunc  func() (*uiautomator2.Element, error)
	sourceFunc         func() (string, error)
	sendKeyActionsFunc func(text string) error

	// Tracking
	clickCalls          []struct{ X, Y int }
	doubleClickCalls    []struct{ X, Y int }
	longClickCalls      []struct{ X, Y, Duration int }
	scrollCalls         []uiautomator2.RectModel
	swipeCalls          []uiautomator2.RectModel
	pressKeyCalls       []int
	backCalls           int
	hideKeyboardCalls   int
	setClipboardCalls   []string
	setOrientationCalls []string

	// Return values
	screenshotData    []byte
	screenshotErr     error
	sourceData        string
	sourceErr         error
	orientationData   string
	orientationErr    error
	setOrientationErr error
	clipboardData     string
	clipboardErr      error
	clickErr          error
	doubleClickErr    error
	longClickErr      error
	scrollErr         error
	swipeErr          error
	pressKeyErr       error
	backErr           error
	hideKeyboardErr   error
	setClipboardErr   error
}

func (m *MockUIA2Client) FindElement(strategy, selector string) (*uiautomator2.Element, error) {
	if m.findElementFunc != nil {
		return m.findElementFunc(strategy, selector)
	}
	return nil, errors.New("element not found")
}

func (m *MockUIA2Client) ActiveElement() (*uiautomator2.Element, error) {
	if m.activeElementFunc != nil {
		return m.activeElementFunc()
	}
	return nil, errors.New("no active element")
}

func (m *MockUIA2Client) Click(x, y int) error {
	m.clickCalls = append(m.clickCalls, struct{ X, Y int }{x, y})
	return m.clickErr
}

func (m *MockUIA2Client) DoubleClick(x, y int) error {
	m.doubleClickCalls = append(m.doubleClickCalls, struct{ X, Y int }{x, y})
	return m.doubleClickErr
}

func (m *MockUIA2Client) DoubleClickElement(elementID string) error {
	return m.doubleClickErr
}

func (m *MockUIA2Client) LongClick(x, y, durationMs int) error {
	m.longClickCalls = append(m.longClickCalls, struct{ X, Y, Duration int }{x, y, durationMs})
	return m.longClickErr
}

func (m *MockUIA2Client) LongClickElement(elementID string, durationMs int) error {
	return m.longClickErr
}

func (m *MockUIA2Client) ScrollInArea(area uiautomator2.RectModel, direction string, percent float64, speed int) error {
	m.scrollCalls = append(m.scrollCalls, area)
	return m.scrollErr
}

func (m *MockUIA2Client) SwipeInArea(area uiautomator2.RectModel, direction string, percent float64, speed int) error {
	m.swipeCalls = append(m.swipeCalls, area)
	return m.swipeErr
}

func (m *MockUIA2Client) Back() error {
	m.backCalls++
	return m.backErr
}

func (m *MockUIA2Client) HideKeyboard() error {
	m.hideKeyboardCalls++
	return m.hideKeyboardErr
}

func (m *MockUIA2Client) PressKeyCode(keyCode int) error {
	m.pressKeyCalls = append(m.pressKeyCalls, keyCode)
	return m.pressKeyErr
}

func (m *MockUIA2Client) SendKeyActions(text string) error {
	if m.sendKeyActionsFunc != nil {
		return m.sendKeyActionsFunc(text)
	}
	return nil
}

func (m *MockUIA2Client) Screenshot() ([]byte, error) {
	return m.screenshotData, m.screenshotErr
}

func (m *MockUIA2Client) Source() (string, error) {
	if m.sourceFunc != nil {
		return m.sourceFunc()
	}
	return m.sourceData, m.sourceErr
}

func (m *MockUIA2Client) GetOrientation() (string, error) {
	return m.orientationData, m.orientationErr
}

func (m *MockUIA2Client) SetOrientation(orientation string) error {
	m.setOrientationCalls = append(m.setOrientationCalls, orientation)
	return m.setOrientationErr
}

func (m *MockUIA2Client) GetClipboard() (string, error) {
	return m.clipboardData, m.clipboardErr
}

func (m *MockUIA2Client) SetClipboard(text string) error {
	m.setClipboardCalls = append(m.setClipboardCalls, text)
	return m.setClipboardErr
}

func (m *MockUIA2Client) SetImplicitWait(timeout time.Duration) error {
	return nil
}

func (m *MockUIA2Client) GetDeviceInfo() (*uiautomator2.DeviceInfo, error) {
	return &uiautomator2.DeviceInfo{
		RealDisplaySize: "1080x2400",
	}, nil
}

func (m *MockUIA2Client) SetAppiumSettings(settings map[string]interface{}) error {
	return nil
}

// ============================================================================
// MockShellExecutor
// ============================================================================

type MockShellExecutor struct {
	commands []string
	response string
	err      error
}

func (m *MockShellExecutor) Shell(cmd string) (string, error) {
	m.commands = append(m.commands, cmd)
	return m.response, m.err
}

// ============================================================================
// Build Selectors Tests
// ============================================================================

func TestBuildSelectorsText(t *testing.T) {
	// Literal text uses textContains (not regex)
	sel := flow.Selector{Text: "Login"}
	strategies, err := buildSelectors(sel, 5000)
	if err != nil {
		t.Fatalf("buildSelectors failed: %v", err)
	}

	// Should have 2 strategies: text and description
	if len(strategies) != 2 {
		t.Errorf("expected 2 strategies, got %d", len(strategies))
	}

	// First should be text-based with textContains (for literal text)
	s := strategies[0]
	if !strings.Contains(s.Value, "textContains") {
		t.Errorf("expected textContains in selector, got: %s", s.Value)
	}
	if !strings.Contains(s.Value, `"Login"`) {
		t.Errorf("expected Login in selector, got: %s", s.Value)
	}
}

func TestBuildSelectorsTextWithPeriod(t *testing.T) {
	// Text with period (domain name) uses textContains, not regex
	sel := flow.Selector{Text: "Join mastodon.social"}
	strategies, err := buildSelectors(sel, 5000)
	if err != nil {
		t.Fatalf("buildSelectors failed: %v", err)
	}

	// Should have 2 strategies: text and description
	if len(strategies) != 2 {
		t.Errorf("expected 2 strategies, got %d", len(strategies))
	}

	// Should use textContains, NOT textMatches
	s := strategies[0]
	if !strings.Contains(s.Value, "textContains") {
		t.Errorf("expected textContains in selector, got: %s", s.Value)
	}
	if strings.Contains(s.Value, "textMatches") {
		t.Errorf("should NOT use textMatches for literal text, got: %s", s.Value)
	}
	// The period should NOT be escaped (textContains doesn't use regex)
	if strings.Contains(s.Value, `\.`) {
		t.Errorf("period should not be escaped in textContains, got: %s", s.Value)
	}
	if !strings.Contains(s.Value, "mastodon.social") {
		t.Errorf("expected literal text 'mastodon.social' in selector, got: %s", s.Value)
	}
}

func TestLooksLikeRegex(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{".+@.+", true},                 // Email pattern
		{".*hello.*", true},             // Contains pattern
		{"[a-z]+", true},                // Character class
		{"^start", true},                // Anchor
		{"end$", true},                  // Anchor
		{"a|b", true},                   // Alternation
		{"a?b", true},                   // Optional
		{"a{2,3}", true},                // Quantifier
		{"(group)", true},               // Group
		{"Hello", false},                // Plain text
		{"Hello World", false},          // Plain text with space
		{"Hello_World", false},          // Plain text with underscore
		{"Login123", false},             // Alphanumeric
		{`\.escaped`, false},            // Escaped dot
		{"mastodon.social", false},      // Domain name - period is literal
		{"Join mastodon.social", false}, // Button text with domain
		{"user@example.com", false},     // Email address (literal)
		{"v1.2.3", false},               // Version number
		{"file.txt", false},             // Filename
		{"www.google.com", false},       // URL host
	}

	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			result := looksLikeRegex(tc.text)
			if result != tc.expected {
				t.Errorf("looksLikeRegex(%q) = %v, expected %v", tc.text, result, tc.expected)
			}
		})
	}
}

func TestBuildSelectorsRegexPattern(t *testing.T) {
	// Email regex pattern - uses textMatches
	sel := flow.Selector{Text: ".+@.+"}
	strategies, err := buildSelectors(sel, 5000)
	if err != nil {
		t.Fatalf("buildSelectors failed: %v", err)
	}

	// Should have 2 strategies: text and description
	if len(strategies) != 2 {
		t.Errorf("expected 2 strategies, got %d", len(strategies))
	}

	// First should be text-based with regex pattern preserved
	s := strategies[0]
	if !strings.Contains(s.Value, "textMatches") {
		t.Error("expected textMatches in selector")
	}
	if !strings.Contains(s.Value, ".+@.+") {
		t.Errorf("expected regex pattern '.+@.+' to be preserved, got: %s", s.Value)
	}
	if !strings.Contains(s.Value, "(?is)") {
		t.Error("expected case-insensitive flag in selector")
	}
}

func TestBuildSelectorsID(t *testing.T) {
	sel := flow.Selector{ID: "login_btn"}
	strategies, err := buildSelectors(sel, 17000)
	if err != nil {
		t.Fatalf("buildSelectors failed: %v", err)
	}

	if len(strategies) != 1 {
		t.Errorf("expected 1 strategy, got %d", len(strategies))
	}

	s := strategies[0]
	if !strings.Contains(s.Value, "resourceIdMatches") {
		t.Error("expected resourceIdMatches in selector")
	}
}

func TestBuildSelectorsWithStateFilters(t *testing.T) {
	enabled := true
	checked := false
	sel := flow.Selector{
		Text:    "Checkbox",
		Enabled: &enabled,
		Checked: &checked,
	}
	strategies, err := buildSelectors(sel, 5000)
	if err != nil {
		t.Fatalf("buildSelectors failed: %v", err)
	}

	s := strategies[0]
	if !strings.Contains(s.Value, ".enabled(true)") {
		t.Error("expected .enabled(true) in selector")
	}
	if !strings.Contains(s.Value, ".checked(false)") {
		t.Error("expected .checked(false) in selector")
	}
}

func TestBuildSelectorsEmpty(t *testing.T) {
	sel := flow.Selector{}
	_, err := buildSelectors(sel, 5000)
	if err == nil {
		t.Error("expected error for empty selector")
	}
}

func TestBuildSelectorsCSS(t *testing.T) {
	sel := flow.Selector{CSS: "android.widget.Button"}
	strategies, err := buildSelectors(sel, 5000)
	if err != nil {
		t.Fatalf("buildSelectors failed: %v", err)
	}

	if len(strategies) != 1 {
		t.Errorf("expected 1 strategy, got %d", len(strategies))
	}

	// CSS doesn't get waitForExists
	if strings.Contains(strategies[0].Value, "waitForExists") {
		t.Error("CSS selector should not have waitForExists")
	}
}

func TestBuildStateFilters(t *testing.T) {
	enabled := true
	selected := false
	focused := true
	checked := false

	sel := flow.Selector{
		Enabled:  &enabled,
		Selected: &selected,
		Focused:  &focused,
		Checked:  &checked,
	}

	filters := buildStateFilters(sel)

	if !strings.Contains(filters, ".enabled(true)") {
		t.Error("missing enabled filter")
	}
	if !strings.Contains(filters, ".selected(false)") {
		t.Error("missing selected filter")
	}
	if !strings.Contains(filters, ".focused(true)") {
		t.Error("missing focused filter")
	}
	if !strings.Contains(filters, ".checked(false)") {
		t.Error("missing checked filter")
	}
}

func TestBuildStateFiltersEmpty(t *testing.T) {
	sel := flow.Selector{}
	filters := buildStateFilters(sel)
	if filters != "" {
		t.Errorf("expected empty string, got %q", filters)
	}
}

func TestEscapeUiAutomator(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{`with "quotes"`, `with \"quotes\"`},
		{"with\nnewline", `with\nnewline`},
		{"with\ttab", `with\ttab`},
		{`back\slash`, `back\\slash`},
		{"regex.*chars", `regex\.\*chars`},
		{"parens()", `parens\(\)`},
		{"brackets[]", `brackets\[\]`},
		{"$special^", `\$special\^`},
		{"pipe|or", `pipe\|or`},
		{"plus+", `plus\+`},
		{"question?", `question\?`},
		{"braces{}", `braces\{\}`},
	}

	for _, tt := range tests {
		got := escapeUIAutomator(tt.input)
		if got != tt.expected {
			t.Errorf("escapeUIAutomator(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ============================================================================
// Helper Tests
// ============================================================================

func TestSuccessResult(t *testing.T) {
	result := successResult("Test passed", nil)
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.Message != "Test passed" {
		t.Errorf("expected message 'Test passed', got %q", result.Message)
	}
	if result.Error != nil {
		t.Error("expected no error")
	}
}

func TestErrorResult(t *testing.T) {
	result := errorResult(nil, "Test failed")
	if result.Success {
		t.Error("expected success=false")
	}
	if result.Message != "Test failed" {
		t.Errorf("expected message 'Test failed', got %q", result.Message)
	}
}

func TestDefaultTimeouts(t *testing.T) {
	if DefaultFindTimeout != 17000 {
		t.Errorf("expected DefaultFindTimeout=17000, got %d", DefaultFindTimeout)
	}
	if OptionalFindTimeout != 7000 {
		t.Errorf("expected OptionalFindTimeout=7000, got %d", OptionalFindTimeout)
	}
}

func TestLocatorStrategy(t *testing.T) {
	ls := LocatorStrategy{
		Strategy: "uiautomator",
		Value:    "new UiSelector().text(\"test\")",
	}
	if ls.Strategy != "uiautomator" {
		t.Error("strategy not set correctly")
	}
	if ls.Value == "" {
		t.Error("value should not be empty")
	}
}

// ============================================================================
// Driver Tests
// ============================================================================

func TestDriverNew(t *testing.T) {
	mock := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, mock)

	if driver == nil {
		t.Fatal("expected driver, got nil")
	}
	if driver.device != mock {
		t.Error("device not set correctly")
	}
}

func TestDriverGetPlatformInfo(t *testing.T) {
	info := &core.PlatformInfo{
		Platform: "android",
		DeviceID: "emulator-5554",
	}
	driver := New(&MockUIA2Client{}, info, nil)

	got := driver.GetPlatformInfo()
	if got != info {
		t.Error("GetPlatformInfo returned wrong info")
	}
}

func TestExecuteUnknownStep(t *testing.T) {
	driver := New(&MockUIA2Client{}, nil, nil)
	step := &flow.UnsupportedStep{}

	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure for unknown step")
	}
	if result.Error == nil {
		t.Error("expected error for unknown step")
	}
}

func TestExecuteTapOnPoint(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.TapOnPointStep{X: 100, Y: 200}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(client.clickCalls) != 1 {
		t.Errorf("expected 1 click call, got %d", len(client.clickCalls))
	}
	if client.clickCalls[0].X != 100 || client.clickCalls[0].Y != 200 {
		t.Error("click coordinates don't match")
	}
}

func TestExecuteTapOnPointError(t *testing.T) {
	client := &MockUIA2Client{clickErr: errors.New("click failed")}
	driver := New(client, nil, nil)

	step := &flow.TapOnPointStep{X: 100, Y: 200}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure")
	}
}

func TestExecuteBack(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.BackStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if client.backCalls != 1 {
		t.Errorf("expected 1 back call, got %d", client.backCalls)
	}
}

func TestExecuteBackError(t *testing.T) {
	client := &MockUIA2Client{backErr: errors.New("back failed")}
	driver := New(client, nil, nil)

	step := &flow.BackStep{}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure")
	}
}

func TestExecutePressKey(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.PressKeyStep{Key: "enter"}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(client.pressKeyCalls) != 1 {
		t.Errorf("expected 1 pressKey call, got %d", len(client.pressKeyCalls))
	}
	if client.pressKeyCalls[0] != 66 { // enter = 66
		t.Errorf("expected keyCode 66, got %d", client.pressKeyCalls[0])
	}
}

func TestExecutePressKeyError(t *testing.T) {
	client := &MockUIA2Client{pressKeyErr: errors.New("key failed")}
	driver := New(client, nil, nil)

	step := &flow.PressKeyStep{Key: "enter"}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure")
	}
}

func TestExecuteScroll(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.ScrollStep{Direction: "down"}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(client.scrollCalls) != 1 {
		t.Errorf("expected 1 scroll call, got %d", len(client.scrollCalls))
	}
}

func TestExecuteScrollError(t *testing.T) {
	client := &MockUIA2Client{scrollErr: errors.New("scroll failed")}
	driver := New(client, nil, nil)

	step := &flow.ScrollStep{Direction: "down"}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure")
	}
}

func TestExecuteScrollDefaultDirection(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.ScrollStep{Direction: ""} // empty = default down
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

func TestExecuteSwipe(t *testing.T) {
	client := &MockUIA2Client{}
	shell := &MockShellExecutor{}
	driver := New(client, nil, shell)

	step := &flow.SwipeStep{Direction: "up"}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	// When no scrollable element found, swipe uses shell command
	if len(shell.commands) != 1 {
		t.Errorf("expected 1 shell command, got %d", len(shell.commands))
	}
}

func TestExecuteSwipeError(t *testing.T) {
	client := &MockUIA2Client{}
	shell := &MockShellExecutor{err: errors.New("swipe failed")}
	driver := New(client, nil, shell)

	step := &flow.SwipeStep{Direction: "up"}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure")
	}
}

func TestExecuteSwipeDefaultDirection(t *testing.T) {
	client := &MockUIA2Client{}
	shell := &MockShellExecutor{}
	driver := New(client, nil, shell)

	step := &flow.SwipeStep{Direction: ""} // empty = default up
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

func TestExecuteHideKeyboard(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.HideKeyboardStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if client.hideKeyboardCalls != 1 {
		t.Errorf("expected 1 hideKeyboard call, got %d", client.hideKeyboardCalls)
	}
}

func TestExecuteEraseText(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.EraseTextStep{Characters: 5}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(client.pressKeyCalls) != 5 {
		t.Errorf("expected 5 key presses, got %d", len(client.pressKeyCalls))
	}
	// All should be delete key (67)
	for i, code := range client.pressKeyCalls {
		if code != 67 {
			t.Errorf("press %d: expected keyCode 67, got %d", i, code)
		}
	}
}

func TestExecuteEraseTextDefault(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.EraseTextStep{Characters: 0} // 0 = default 50
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(client.pressKeyCalls) != 50 {
		t.Errorf("expected 50 key presses (default), got %d", len(client.pressKeyCalls))
	}
}

func TestExecuteEraseTextError(t *testing.T) {
	client := &MockUIA2Client{pressKeyErr: errors.New("key failed")}
	driver := New(client, nil, nil)

	step := &flow.EraseTextStep{Characters: 5}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure")
	}
}

func TestScreenshot(t *testing.T) {
	client := &MockUIA2Client{
		screenshotData: []byte{0x89, 0x50, 0x4E, 0x47}, // PNG header
	}
	driver := New(client, nil, nil)

	data, err := driver.Screenshot()
	if err != nil {
		t.Errorf("Screenshot failed: %v", err)
	}
	if len(data) != 4 {
		t.Errorf("expected 4 bytes, got %d", len(data))
	}
}

func TestScreenshotError(t *testing.T) {
	client := &MockUIA2Client{
		screenshotErr: errors.New("screenshot failed"),
	}
	driver := New(client, nil, nil)

	_, err := driver.Screenshot()
	if err == nil {
		t.Error("expected error")
	}
}

func TestHierarchy(t *testing.T) {
	client := &MockUIA2Client{
		sourceData: "<hierarchy></hierarchy>",
	}
	driver := New(client, nil, nil)

	data, err := driver.Hierarchy()
	if err != nil {
		t.Errorf("Hierarchy failed: %v", err)
	}
	if string(data) != "<hierarchy></hierarchy>" {
		t.Errorf("unexpected hierarchy: %s", string(data))
	}
}

func TestHierarchyError(t *testing.T) {
	client := &MockUIA2Client{
		sourceErr: errors.New("source failed"),
	}
	driver := New(client, nil, nil)

	_, err := driver.Hierarchy()
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetState(t *testing.T) {
	client := &MockUIA2Client{
		orientationData: "PORTRAIT",
		clipboardData:   "test clipboard",
	}
	driver := New(client, nil, nil)

	state := driver.GetState()
	if state == nil {
		t.Fatal("expected state, got nil")
	}
	if state.Orientation != "portrait" {
		t.Errorf("expected portrait, got %s", state.Orientation)
	}
	if state.ClipboardText != "test clipboard" {
		t.Errorf("expected 'test clipboard', got %s", state.ClipboardText)
	}
}

func TestGetStateErrors(t *testing.T) {
	client := &MockUIA2Client{
		orientationErr: errors.New("orientation failed"),
		clipboardErr:   errors.New("clipboard failed"),
	}
	driver := New(client, nil, nil)

	state := driver.GetState()
	if state == nil {
		t.Fatal("expected state, got nil")
	}
	// Should have empty values on error but not fail
	if state.Orientation != "" {
		t.Errorf("expected empty orientation on error, got %s", state.Orientation)
	}
}

// ============================================================================
// Input Command Tests
// ============================================================================

func TestInputTextEmpty(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.InputTextStep{Text: ""}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure for empty text")
	}
}

func TestInputRandomNoActiveElement(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.InputRandomStep{Length: 10}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure when no active element")
	}
}

// ============================================================================
// Execute Switch Coverage
// ============================================================================

func TestExecuteAllStepTypes(t *testing.T) {
	// This test covers the Execute switch statement for all step types
	// Most will fail because they need findElement, but this covers the switch paths
	client := &MockUIA2Client{}
	shell := &MockShellExecutor{}
	driver := New(client, nil, shell)
	driver.SetFindTimeout(100)        // 100ms for fast test failure
	driver.SetOptionalFindTimeout(50) // 50ms for optional elements

	tests := []struct {
		name    string
		step    flow.Step
		wantErr bool
	}{
		{"TapOnStep", &flow.TapOnStep{Selector: flow.Selector{Text: "btn"}}, true},
		{"DoubleTapOnStep", &flow.DoubleTapOnStep{Selector: flow.Selector{Text: "btn"}}, true},
		{"LongPressOnStep", &flow.LongPressOnStep{Selector: flow.Selector{Text: "btn"}}, true},
		{"TapOnPointStep", &flow.TapOnPointStep{X: 10, Y: 20}, false},
		{"AssertVisibleStep", &flow.AssertVisibleStep{Selector: flow.Selector{Text: "btn"}}, true},
		{"AssertNotVisibleStep", &flow.AssertNotVisibleStep{Selector: flow.Selector{Text: "btn"}}, false}, // Not found = success
		{"InputTextStep_empty", &flow.InputTextStep{Text: ""}, true},
		{"EraseTextStep", &flow.EraseTextStep{Characters: 1}, false},
		{"HideKeyboardStep", &flow.HideKeyboardStep{}, false},
		{"InputRandomStep", &flow.InputRandomStep{Length: 5}, true},
		{"ScrollStep", &flow.ScrollStep{Direction: "down"}, false},
		{"ScrollUntilVisibleStep", &flow.ScrollUntilVisibleStep{Element: flow.Selector{Text: "btn"}}, true},
		{"SwipeStep", &flow.SwipeStep{Direction: "up"}, false},
		{"BackStep", &flow.BackStep{}, false},
		{"PressKeyStep", &flow.PressKeyStep{Key: "enter"}, false},
		{"CopyTextFromStep", &flow.CopyTextFromStep{Selector: flow.Selector{Text: "btn"}}, true},
		{"PasteTextStep", &flow.PasteTextStep{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := driver.Execute(tt.step)
			if tt.wantErr && result.Success {
				t.Errorf("expected failure for %s", tt.name)
			}
			if !tt.wantErr && !result.Success {
				t.Errorf("expected success for %s, got error: %v", tt.name, result.Error)
			}
		})
	}
}

// ============================================================================
// App Lifecycle with Shell Errors
// ============================================================================

func TestLaunchAppShellError(t *testing.T) {
	shell := &MockShellExecutor{err: errors.New("shell error")}
	driver := New(&MockUIA2Client{}, nil, shell)

	step := &flow.LaunchAppStep{AppID: "com.test", ClearState: true}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on shell error")
	}
}

func TestStopAppShellError(t *testing.T) {
	shell := &MockShellExecutor{err: errors.New("shell error")}
	driver := New(&MockUIA2Client{}, nil, shell)

	step := &flow.StopAppStep{AppID: "com.test"}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on shell error")
	}
}

func TestClearStateShellError(t *testing.T) {
	shell := &MockShellExecutor{err: errors.New("shell error")}
	driver := New(&MockUIA2Client{}, nil, shell)

	step := &flow.ClearStateStep{AppID: "com.test"}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on shell error")
	}
}

// ============================================================================
// Escape UiAutomator Edge Cases
// ============================================================================

func TestEscapeUiAutomatorCarriageReturn(t *testing.T) {
	got := escapeUIAutomator("with\rreturn")
	if got != `with\rreturn` {
		t.Errorf("expected 'with\\rreturn', got %q", got)
	}
}

// ============================================================================
// Hide Keyboard Error
// ============================================================================

func TestHideKeyboardError(t *testing.T) {
	// hideKeyboard swallows errors (keyboard may not be visible)
	client := &MockUIA2Client{hideKeyboardErr: errors.New("hide keyboard failed")}
	driver := New(client, nil, nil)

	step := &flow.HideKeyboardStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Error("expected success even when hideKeyboard fails (error is swallowed)")
	}
}

// ============================================================================
// Paste Text Error
// ============================================================================

func TestPasteTextClipboardError(t *testing.T) {
	client := &MockUIA2Client{clipboardErr: errors.New("clipboard error")}
	driver := New(client, nil, nil)

	step := &flow.PasteTextStep{}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure when clipboard read fails")
	}
}

// ============================================================================
// HTTP Mock Server Tests
// ============================================================================

// setupMockServer creates a test server that mimics UIAutomator2 responses.
func setupMockServer(t *testing.T, handlers map[string]func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Strip /session/xxx prefix if present
		if strings.HasPrefix(path, "/session/") {
			parts := strings.SplitN(path[9:], "/", 2) // Skip "/session/"
			if len(parts) > 1 {
				path = "/" + parts[1]
			} else {
				path = "/"
			}
		}

		if handler, ok := handlers[r.Method+" "+path]; ok {
			handler(w, r)
			return
		}

		// Handle timeouts endpoint (for implicit wait)
		if path == "/timeouts" && r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}

		// Default: element not found
		t.Logf("Unhandled request: %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]interface{}{
			"value": map[string]string{"ELEMENT": ""},
		})
	}))
}

// MockHTTPClient wraps a real uiautomator2.Client pointing to a test server.
type MockHTTPClient struct {
	*uiautomator2.Client
}

func newMockHTTPClient(serverURL string) *MockHTTPClient {
	// Extract port from test server URL (e.g., "http://127.0.0.1:12345")
	// NewClientTCP uses regular HTTP transport, not Unix socket
	port := 0
	if n, err := fmt.Sscanf(serverURL, "http://127.0.0.1:%d", &port); err != nil || n != 1 {
		// Try alternative format
		_, _ = fmt.Sscanf(serverURL, "http://localhost:%d", &port) // best-effort fallback parse
	}
	client := uiautomator2.NewClientTCP(port)
	// Override baseURL to match test server exactly
	client.SetBaseURL(serverURL)
	client.SetSession("test-session")
	return &MockHTTPClient{Client: client}
}

// ============================================================================
// Tests with HTTP Mock - Element Success Paths
// ============================================================================

func TestTapOnWithElement(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-123"},
			})
		},
		"POST /element/elem-123/click": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
		"GET /element/elem-123/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Button"})
		},
		"GET /element/elem-123/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-123/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-123/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{Selector: flow.Selector{Text: "Login"}}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if result.Element == nil {
		t.Error("expected element info")
	}
}

func TestTapOnClickError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-123"},
			})
		},
		"POST /element/elem-123/click": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "click failed"})
		},
		"GET /element/elem-123/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Button"})
		},
		"GET /element/elem-123/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-123/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-123/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{Selector: flow.Selector{Text: "Login"}}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on click error")
	}
}

func TestDoubleTapOnWithElement(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-456"},
			})
		},
		"POST /appium/gestures/double_click": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
		"GET /element/elem-456/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Button"})
		},
		"GET /element/elem-456/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-456/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-456/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.DoubleTapOnStep{Selector: flow.Selector{Text: "Button"}}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

func TestDoubleTapOnClickError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-456"},
			})
		},
		"POST /appium/gestures/double_click": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "double click failed"})
		},
		"GET /element/elem-456/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Button"})
		},
		"GET /element/elem-456/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-456/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-456/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.DoubleTapOnStep{Selector: flow.Selector{Text: "Button"}}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on double click error")
	}
}

func TestLongPressOnWithElement(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-789"},
			})
		},
		"POST /appium/gestures/long_click": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
		"GET /element/elem-789/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Button"})
		},
		"GET /element/elem-789/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-789/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-789/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.LongPressOnStep{Selector: flow.Selector{Text: "Button"}}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

func TestLongPressOnClickError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-789"},
			})
		},
		"POST /appium/gestures/long_click": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "long click failed"})
		},
		"GET /element/elem-789/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Button"})
		},
		"GET /element/elem-789/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-789/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-789/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.LongPressOnStep{Selector: flow.Selector{Text: "Button"}}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on long click error")
	}
}

func TestAssertVisibleWithElement(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-vis"},
			})
		},
		"GET /element/elem-vis/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Label"})
		},
		"GET /element/elem-vis/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-vis/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-vis/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.AssertVisibleStep{Selector: flow.Selector{Text: "Label"}}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

// TestAssertVisibleElementFound tests that assertVisible succeeds when element is found.
// Note: UIAutomator2 by default only returns visible elements, so we assume visible=true when found.
// We no longer call IsDisplayed() as it causes slow HTTP calls and UIAutomator handles visibility filtering.
func TestAssertVisibleElementFoundIsVisible(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-vis"},
			})
		},
		"GET /element/elem-vis/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Label"})
		},
		"GET /element/elem-vis/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		// Note: displayed/enabled endpoints no longer called - we assume visible=true when found
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.AssertVisibleStep{Selector: flow.Selector{Text: "Label"}}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success when element found (UIAutomator filters invisible), got: %v", result.Error)
	}
}

func TestAssertNotVisibleElementFound(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-found"},
			})
		},
		"GET /element/elem-found/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Label"})
		},
		"GET /element/elem-found/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-found/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-found/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.AssertNotVisibleStep{Selector: flow.Selector{Text: "Label"}}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure when element is found (should not be visible)")
	}
}

func TestInputTextWithElement(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-input"},
			})
		},
		"POST /element/elem-input/value": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
		"GET /element/elem-input/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": ""})
		},
		"GET /element/elem-input/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 200, "height": 40},
			})
		},
		"GET /element/elem-input/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-input/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.InputTextStep{
		Text:     "Hello World",
		Selector: flow.Selector{ID: "input_field"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

func TestInputTextSendKeysError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-input"},
			})
		},
		"POST /element/elem-input/value": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "send keys failed"})
		},
		"GET /element/elem-input/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": ""})
		},
		"GET /element/elem-input/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 200, "height": 40},
			})
		},
		"GET /element/elem-input/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-input/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.InputTextStep{
		Text:     "Hello World",
		Selector: flow.Selector{ID: "input_field"},
	}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on sendKeys error")
	}
}

func TestInputTextNoSelector(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "active-elem"},
			})
		},
		"POST /element/active-elem/value": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.InputTextStep{Text: "Hello"}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

func TestInputTextNoSelectorError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "active-elem"},
			})
		},
		"POST /element/active-elem/value": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "send keys failed"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.InputTextStep{Text: "Hello"}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on sendKeys error")
	}
}

func TestInputRandomWithActiveElement(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "active-elem"},
			})
		},
		"POST /element/active-elem/value": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.InputRandomStep{Length: 8}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if result.Data == nil {
		t.Error("expected random text in Data")
	}
}

func TestInputRandomDefaultLength(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "active-elem"},
			})
		},
		"POST /element/active-elem/value": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.InputRandomStep{Length: 0} // Default 10
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	// Data should be 10 chars
	if text, ok := result.Data.(string); ok {
		if len(text) != 10 {
			t.Errorf("expected 10 char string, got %d", len(text))
		}
	}
}

func TestInputRandomSendKeysError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "active-elem"},
			})
		},
		"POST /element/active-elem/value": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "error"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.InputRandomStep{Length: 5}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on SendKeys error")
	}
}

func TestCopyTextFromWithElement(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-copy"},
			})
		},
		"GET /element/elem-copy/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Copied text"})
		},
		"GET /element/elem-copy/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-copy/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-copy/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"POST /appium/device/set_clipboard": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.CopyTextFromStep{Selector: flow.Selector{Text: "Label"}}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if result.Data != "Copied text" {
		t.Errorf("expected 'Copied text', got %v", result.Data)
	}
}

func TestCopyTextFromTextError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-copy"},
			})
		},
		"GET /element/elem-copy/text": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "error"})
		},
		"GET /element/elem-copy/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-copy/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-copy/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.CopyTextFromStep{Selector: flow.Selector{Text: "Label"}}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on Text() error")
	}
}

func TestCopyTextFromClipboardError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-copy"},
			})
		},
		"GET /element/elem-copy/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Copied text"})
		},
		"GET /element/elem-copy/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-copy/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-copy/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"POST /appium/device/set_clipboard": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "clipboard error"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.CopyTextFromStep{Selector: flow.Selector{Text: "Label"}}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on SetClipboard error")
	}
}

func TestPasteTextWithActiveElement(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /appium/device/get_clipboard": func(w http.ResponseWriter, r *http.Request) {
			// Clipboard returns base64 encoded text
			writeJSON(w, map[string]interface{}{"value": "SGVsbG8gV29ybGQ="}) // "Hello World"
		},
		"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "active-elem"},
			})
		},
		"POST /element/active-elem/value": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.PasteTextStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

func TestPasteTextNoActiveElement(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /appium/device/get_clipboard": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "SGVsbG8="})
		},
		"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": ""},
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.PasteTextStep{}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure when no active element")
	}
}

func TestPasteTextSendKeysError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /appium/device/get_clipboard": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "SGVsbG8="})
		},
		"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "active-elem"},
			})
		},
		"POST /element/active-elem/value": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "send keys failed"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.PasteTextStep{}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on SendKeys error")
	}
}

func TestScrollUntilVisibleFound(t *testing.T) {
	callCount := 0
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			callCount++
			// Found on second attempt
			if callCount >= 2 {
				writeJSON(w, map[string]interface{}{
					"value": map[string]string{"ELEMENT": "elem-scroll"},
				})
			} else {
				writeJSON(w, map[string]interface{}{
					"value": map[string]string{"ELEMENT": ""},
				})
			}
		},
		"GET /element/elem-scroll/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Target"})
		},
		"GET /element/elem-scroll/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 200, "width": 50, "height": 30},
			})
		},
		"GET /element/elem-scroll/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-scroll/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"POST /appium/actions": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
		"GET /appium/device/info": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"realDisplaySize": "1080x2400"},
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "Target"},
		Direction: "down",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

func TestScrollUntilVisibleScrollError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": ""},
			})
		},
		"POST /appium/actions": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "scroll error"})
		},
		"GET /appium/device/info": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"realDisplaySize": "1080x2400"},
			})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			// Return valid hierarchy without target element
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><node text="Other" bounds="[0,0][100,100]"/></hierarchy>`,
			})
		},
		"POST /appium/gestures/scroll": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "scroll error"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "Target"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 2000}, // Short timeout for test
	}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on scroll error")
	}
}

// ============================================================================
// Relative Selector Tests (uses HTTP mock for anchor + page source for target)
// ============================================================================

// setupRelativeSelectorServer creates HTTP mock with anchor element + page source.
func setupRelativeSelectorServer(t *testing.T, pageSource string, clickHandler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		// Anchor element finding
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Header"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 0, "width": 1080, "height": 100},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		// Page source for target finding
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		// Coordinate-based click
		"POST /appium/gestures/click": clickHandler,
	})
}

func TestTapOnRelativeSelectorBelow(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Header" bounds="[0,0][1080,100]" class="android.widget.TextView" />
    <node text="Button" bounds="[100,150][200,200]" class="android.widget.Button" clickable="true" />
</hierarchy>`

	var clickCalled bool
	server := setupRelativeSelectorServer(t, pageSource, func(w http.ResponseWriter, r *http.Request) {
		clickCalled = true
		writeJSON(w, map[string]interface{}{"value": nil})
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:  "Button",
			Below: &flow.Selector{Text: "Header"},
		},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !clickCalled {
		t.Error("expected click to be called")
	}
}

func TestTapOnRelativeSelectorClickError(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Header" bounds="[0,0][1080,100]" class="android.widget.TextView" />
    <node text="Button" bounds="[100,150][200,200]" class="android.widget.Button" />
</hierarchy>`

	server := setupRelativeSelectorServer(t, pageSource, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]interface{}{"value": "click failed"})
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:  "Button",
			Below: &flow.Selector{Text: "Header"},
		},
	}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on click error")
	}
}

func TestDoubleTapOnRelativeSelector(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Header" bounds="[0,0][1080,100]" class="android.widget.TextView" />
    <node text="Button" bounds="[100,150][200,200]" class="android.widget.Button" />
</hierarchy>`

	var doubleClickCalled bool
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Header"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 0, "width": 1080, "height": 100},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/double_click": func(w http.ResponseWriter, r *http.Request) {
			doubleClickCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.DoubleTapOnStep{
		Selector: flow.Selector{
			Text:  "Button",
			Below: &flow.Selector{Text: "Header"},
		},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !doubleClickCalled {
		t.Error("expected double click to be called")
	}
}

func TestDoubleTapOnRelativeSelectorError(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Header" bounds="[0,0][1080,100]" class="android.widget.TextView" />
    <node text="Button" bounds="[100,150][200,200]" class="android.widget.Button" />
</hierarchy>`

	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Header"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 0, "width": 1080, "height": 100},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/double_click": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "double click failed"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.DoubleTapOnStep{
		Selector: flow.Selector{
			Text:  "Button",
			Below: &flow.Selector{Text: "Header"},
		},
	}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on double click error")
	}
}

func TestLongPressOnRelativeSelectorSuccess(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Header" bounds="[0,0][1080,100]" class="android.widget.TextView" />
    <node text="Button" bounds="[100,150][200,200]" class="android.widget.Button" />
</hierarchy>`

	var longClickCalled bool
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Header"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 0, "width": 1080, "height": 100},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/long_click": func(w http.ResponseWriter, r *http.Request) {
			longClickCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.LongPressOnStep{
		Selector: flow.Selector{
			Text:  "Button",
			Below: &flow.Selector{Text: "Header"},
		},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !longClickCalled {
		t.Error("expected long click to be called")
	}
}

func TestLongPressOnRelativeSelectorError(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Header" bounds="[0,0][1080,100]" class="android.widget.TextView" />
    <node text="Button" bounds="[100,150][200,200]" class="android.widget.Button" />
</hierarchy>`

	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Header"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 0, "width": 1080, "height": 100},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/long_click": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "long click failed"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.LongPressOnStep{
		Selector: flow.Selector{
			Text:  "Button",
			Below: &flow.Selector{Text: "Header"},
		},
	}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on long click error")
	}
}

// Additional relative selector tests for coverage
func TestTapOnRelativeSelectorAbove(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Button" bounds="[100,50][200,100]" class="android.widget.Button" clickable="true" />
    <node text="Footer" bounds="[0,150][1080,200]" class="android.widget.TextView" />
</hierarchy>`

	var clickCalled bool
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Footer"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 150, "width": 1080, "height": 50},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/click": func(w http.ResponseWriter, r *http.Request) {
			clickCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:  "Button",
			Above: &flow.Selector{Text: "Footer"},
		},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !clickCalled {
		t.Error("expected click to be called")
	}
}

func TestTapOnRelativeSelectorLeftOf(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Label" bounds="[50,100][150,150]" class="android.widget.TextView" clickable="true" />
    <node text="Input" bounds="[200,100][400,150]" class="android.widget.EditText" />
</hierarchy>`

	var clickCalled bool
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Input"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 200, "y": 100, "width": 200, "height": 50},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/click": func(w http.ResponseWriter, r *http.Request) {
			clickCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:   "Label",
			LeftOf: &flow.Selector{Text: "Input"},
		},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !clickCalled {
		t.Error("expected click to be called")
	}
}

func TestTapOnRelativeSelectorRightOf(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Label" bounds="[50,100][150,150]" class="android.widget.TextView" />
    <node text="Value" bounds="[200,100][400,150]" class="android.widget.TextView" clickable="true" />
</hierarchy>`

	var clickCalled bool
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Label"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 50, "y": 100, "width": 100, "height": 50},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/click": func(w http.ResponseWriter, r *http.Request) {
			clickCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:    "Value",
			RightOf: &flow.Selector{Text: "Label"},
		},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !clickCalled {
		t.Error("expected click to be called")
	}
}

func TestTapOnRelativeSelectorChildOf(t *testing.T) {
	// Page source must contain an element matching the ChildOf selector (ID "container")
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="" resource-id="container" bounds="[0,0][1080,500]" class="android.widget.LinearLayout">
        <node text="ChildButton" bounds="[100,100][200,150]" class="android.widget.Button" clickable="true" />
    </node>
</hierarchy>`

	var clickCalled bool
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/click": func(w http.ResponseWriter, r *http.Request) {
			clickCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:    "ChildButton",
			ChildOf: &flow.Selector{ID: "container"},
		},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !clickCalled {
		t.Error("expected click to be called")
	}
}

func TestTapOnRelativeSelectorContainsChild(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="" bounds="[0,0][1080,500]" class="android.widget.LinearLayout" clickable="true">
        <node text="ChildText" bounds="[100,100][200,150]" class="android.widget.TextView" />
    </node>
</hierarchy>`

	var clickCalled bool
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "ChildText"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 100, "y": 100, "width": 100, "height": 50},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/click": func(w http.ResponseWriter, r *http.Request) {
			clickCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			ContainsChild: &flow.Selector{Text: "ChildText"},
		},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !clickCalled {
		t.Error("expected click to be called")
	}
}

func TestTapOnRelativeSelectorContainsDescendants(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="" bounds="[0,0][1080,500]" class="android.widget.LinearLayout" clickable="true">
        <node text="Title" bounds="[100,50][500,100]" class="android.widget.TextView" />
        <node text="Description" bounds="[100,100][500,150]" class="android.widget.TextView" />
    </node>
</hierarchy>`

	var clickCalled bool
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/click": func(w http.ResponseWriter, r *http.Request) {
			clickCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			ContainsDescendants: []*flow.Selector{
				{Text: "Title"},
				{Text: "Description"},
			},
		},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !clickCalled {
		t.Error("expected click to be called")
	}
}

func TestRelativeSelectorWithIndex(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Header" bounds="[0,0][1080,100]" class="android.widget.TextView" />
    <node text="Item" bounds="[100,150][200,200]" class="android.widget.Button" clickable="true" />
    <node text="Item" bounds="[100,250][200,300]" class="android.widget.Button" clickable="true" />
    <node text="Item" bounds="[100,350][200,400]" class="android.widget.Button" clickable="true" />
</hierarchy>`

	var clickCalled bool
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Header"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 0, "width": 1080, "height": 100},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/click": func(w http.ResponseWriter, r *http.Request) {
			clickCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	// Select the second Item (index 1)
	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:  "Item",
			Below: &flow.Selector{Text: "Header"},
			Index: "1",
		},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !clickCalled {
		t.Error("expected click to be called")
	}
}

func TestRelativeSelectorWithNegativeIndex(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Header" bounds="[0,0][1080,100]" class="android.widget.TextView" />
    <node text="Item" bounds="[100,150][200,200]" class="android.widget.Button" clickable="true" />
    <node text="Item" bounds="[100,250][200,300]" class="android.widget.Button" clickable="true" />
    <node text="Item" bounds="[100,350][200,400]" class="android.widget.Button" clickable="true" />
</hierarchy>`

	var clickCalled bool
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Header"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 0, "width": 1080, "height": 100},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
		"POST /appium/gestures/click": func(w http.ResponseWriter, r *http.Request) {
			clickCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	// Select the last Item (index -1)
	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:  "Item",
			Below: &flow.Selector{Text: "Header"},
			Index: "-1",
		},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !clickCalled {
		t.Error("expected click to be called")
	}
}

func TestRelativeSelectorNoMatch(t *testing.T) {
	pageSource := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
    <node text="Header" bounds="[0,0][1080,100]" class="android.widget.TextView" />
</hierarchy>`

	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Header"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 0, "width": 1080, "height": 100},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": pageSource})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	// No element with text "Button" below Header
	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:  "Button",
			Below: &flow.Selector{Text: "Header"},
		},
	}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure when no elements match")
	}
}

func TestRelativeSelectorPageSourceError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Header"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 0, "width": 1080, "height": 100},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "source error"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:  "Button",
			Below: &flow.Selector{Text: "Header"},
		},
	}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on page source error")
	}
}

func TestRelativeSelectorParseError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "anchor-elem"},
			})
		},
		"GET /element/anchor-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Header"})
		},
		"GET /element/anchor-elem/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 0, "width": 1080, "height": 100},
			})
		},
		"GET /element/anchor-elem/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/anchor-elem/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "<invalid xml"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:  "Button",
			Below: &flow.Selector{Text: "Header"},
		},
	}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure on parse error")
	}
}

// ============================================================================
// FindElement Edge Cases
// ============================================================================

func TestFindElementMultipleStrategies(t *testing.T) {
	// First strategy fails, second succeeds
	callCount := 0
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			callCount++
			// Fail text strategy, succeed on description
			if callCount >= 2 {
				writeJSON(w, map[string]interface{}{
					"value": map[string]string{"ELEMENT": "elem-multi"},
				})
			} else {
				writeJSON(w, map[string]interface{}{
					"value": map[string]string{"ELEMENT": ""},
				})
			}
		},
		"GET /element/elem-multi/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Found"})
		},
		"GET /element/elem-multi/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 10, "y": 20, "width": 100, "height": 50},
			})
		},
		"GET /element/elem-multi/attribute/displayed": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"GET /element/elem-multi/attribute/enabled": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "true"})
		},
		"POST /element/elem-multi/click": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	// Text selector tries textMatches and descriptionMatches
	step := &flow.TapOnStep{Selector: flow.Selector{Text: "Button"}}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success on second strategy, got error: %v", result.Error)
	}
}

// Note: App lifecycle tests are in commands_test.go
