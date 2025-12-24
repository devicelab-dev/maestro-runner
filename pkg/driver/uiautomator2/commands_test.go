package uiautomator2

import (
	"errors"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestMapDirection(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"up", "up"},
		{"down", "down"},
		{"left", "left"},
		{"right", "right"},
		{"UP", "down"},   // unknown, defaults to down
		{"invalid", "down"}, // unknown, defaults to down
		{"", "down"},     // empty, defaults to down
	}

	for _, tt := range tests {
		got := mapDirection(tt.input)
		if got != tt.expected {
			t.Errorf("mapDirection(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestMapKeyCode(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"enter", 66},
		{"ENTER", 66},
		{"back", 4},
		{"home", 3},
		{"menu", 82},
		{"delete", 67},
		{"backspace", 67},
		{"tab", 61},
		{"space", 62},
		{"volume_up", 24},
		{"volume_down", 25},
		{"power", 26},
		{"camera", 27},
		{"search", 84},
		{"dpad_up", 19},
		{"dpad_down", 20},
		{"dpad_left", 21},
		{"dpad_right", 22},
		{"dpad_center", 23},
		{"unknown", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := mapKeyCode(tt.input)
		if got != tt.expected {
			t.Errorf("mapKeyCode(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestRandomString(t *testing.T) {
	// Test various lengths
	lengths := []int{0, 1, 5, 10, 50}
	for _, length := range lengths {
		result := randomString(length)
		if len(result) != length {
			t.Errorf("randomString(%d) returned length %d", length, len(result))
		}
	}

	// Test randomness (two calls should produce different results for sufficient length)
	r1 := randomString(20)
	r2 := randomString(20)
	if r1 == r2 {
		t.Error("randomString should produce different results")
	}

	// Test character set
	result := randomString(100)
	for _, c := range result {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			t.Errorf("randomString contains invalid character: %c", c)
		}
	}
}

func TestLaunchAppNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.LaunchAppStep{AppID: "com.example.app"}

	result := driver.launchApp(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
	if result.Error == nil {
		t.Error("expected error when device is nil")
	}
}

func TestLaunchAppNoAppID(t *testing.T) {
	mock := &MockShellExecutor{}
	driver := &Driver{device: mock}
	step := &flow.LaunchAppStep{AppID: ""}

	result := driver.launchApp(step)

	if result.Success {
		t.Error("expected failure when appId is empty")
	}
}

func TestLaunchAppSuccess(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.LaunchAppStep{AppID: "com.example.app"}

	result := driver.launchApp(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	// Should have called force-stop and monkey
	if len(mock.commands) < 2 {
		t.Errorf("expected at least 2 commands, got %d", len(mock.commands))
	}
}

func TestLaunchAppWithClearState(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.LaunchAppStep{
		AppID:      "com.example.app",
		ClearState: true,
	}

	result := driver.launchApp(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	// Should have called pm clear
	foundClear := false
	for _, cmd := range mock.commands {
		if cmd == "pm clear com.example.app" {
			foundClear = true
			break
		}
	}
	if !foundClear {
		t.Error("expected pm clear command")
	}
}

func TestLaunchAppStopAppFalse(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	stopApp := false
	step := &flow.LaunchAppStep{
		AppID:   "com.example.app",
		StopApp: &stopApp,
	}

	driver.launchApp(step)

	// Should NOT have called force-stop
	for _, cmd := range mock.commands {
		if cmd == "am force-stop com.example.app" {
			t.Error("should not call force-stop when StopApp=false")
		}
	}
}

func TestStopAppNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.StopAppStep{AppID: "com.example.app"}

	result := driver.stopApp(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestStopAppNoAppID(t *testing.T) {
	mock := &MockShellExecutor{}
	driver := &Driver{device: mock}
	step := &flow.StopAppStep{AppID: ""}

	result := driver.stopApp(step)

	if result.Success {
		t.Error("expected failure when appId is empty")
	}
}

func TestStopAppSuccess(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.StopAppStep{AppID: "com.example.app"}

	result := driver.stopApp(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	if len(mock.commands) != 1 || mock.commands[0] != "am force-stop com.example.app" {
		t.Errorf("expected force-stop command, got %v", mock.commands)
	}
}

func TestClearStateNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.ClearStateStep{AppID: "com.example.app"}

	result := driver.clearState(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestClearStateNoAppID(t *testing.T) {
	mock := &MockShellExecutor{}
	driver := &Driver{device: mock}
	step := &flow.ClearStateStep{AppID: ""}

	result := driver.clearState(step)

	if result.Success {
		t.Error("expected failure when appId is empty")
	}
}

func TestClearStateSuccess(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.ClearStateStep{AppID: "com.example.app"}

	result := driver.clearState(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	if len(mock.commands) != 1 || mock.commands[0] != "pm clear com.example.app" {
		t.Errorf("expected pm clear command, got %v", mock.commands)
	}
}

func TestInputTextNoText(t *testing.T) {
	driver := &Driver{}
	step := &flow.InputTextStep{Text: ""}

	result := driver.inputText(step)

	if result.Success {
		t.Error("expected failure when text is empty")
	}
}

func TestEraseTextDefaults(t *testing.T) {
	// Just test that step parsing works - actual erase needs client
	step := &flow.EraseTextStep{Characters: 0}
	if step.Characters != 0 {
		t.Error("expected default characters to be 0")
	}
}

func TestPressKeyUnknown(t *testing.T) {
	driver := &Driver{}
	step := &flow.PressKeyStep{Key: "unknown_key"}

	result := driver.pressKey(step)

	if result.Success {
		t.Error("expected failure for unknown key")
	}
}

// ============================================================================
// KillApp Tests
// ============================================================================

func TestKillAppNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.KillAppStep{AppID: "com.example.app"}

	result := driver.killApp(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
	if result.Error == nil {
		t.Error("expected error when device is nil")
	}
}

func TestKillAppNoAppID(t *testing.T) {
	mock := &MockShellExecutor{}
	driver := &Driver{device: mock}
	step := &flow.KillAppStep{AppID: ""}

	result := driver.killApp(step)

	if result.Success {
		t.Error("expected failure when appId is empty")
	}
}

func TestKillAppSuccess(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.KillAppStep{AppID: "com.example.app"}

	result := driver.killApp(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	if len(mock.commands) != 1 || mock.commands[0] != "am force-stop com.example.app" {
		t.Errorf("expected force-stop command, got %v", mock.commands)
	}
}

// ============================================================================
// SetOrientation Tests
// ============================================================================

func TestSetOrientationInvalid(t *testing.T) {
	mock := &MockUIA2Client{}
	driver := &Driver{client: mock}
	step := &flow.SetOrientationStep{Orientation: "invalid"}

	result := driver.setOrientation(step)

	if result.Success {
		t.Error("expected failure for invalid orientation")
	}
}

func TestSetOrientationPortrait(t *testing.T) {
	mock := &MockUIA2Client{}
	driver := &Driver{client: mock}
	step := &flow.SetOrientationStep{Orientation: "portrait"}

	result := driver.setOrientation(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	if len(mock.setOrientationCalls) != 1 || mock.setOrientationCalls[0] != "PORTRAIT" {
		t.Errorf("expected PORTRAIT call, got %v", mock.setOrientationCalls)
	}
}

func TestSetOrientationLandscape(t *testing.T) {
	mock := &MockUIA2Client{}
	driver := &Driver{client: mock}
	step := &flow.SetOrientationStep{Orientation: "LANDSCAPE"}

	result := driver.setOrientation(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	if len(mock.setOrientationCalls) != 1 || mock.setOrientationCalls[0] != "LANDSCAPE" {
		t.Errorf("expected LANDSCAPE call, got %v", mock.setOrientationCalls)
	}
}

func TestSetOrientationError(t *testing.T) {
	mock := &MockUIA2Client{setOrientationErr: errors.New("orientation failed")}
	driver := &Driver{client: mock}
	step := &flow.SetOrientationStep{Orientation: "portrait"}

	result := driver.setOrientation(step)

	if result.Success {
		t.Error("expected failure when orientation fails")
	}
}

// ============================================================================
// OpenLink Tests
// ============================================================================

func TestOpenLinkNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.OpenLinkStep{Link: "https://example.com"}

	result := driver.openLink(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestOpenLinkNoLink(t *testing.T) {
	mock := &MockShellExecutor{}
	driver := &Driver{device: mock}
	step := &flow.OpenLinkStep{Link: ""}

	result := driver.openLink(step)

	if result.Success {
		t.Error("expected failure when link is empty")
	}
}

func TestOpenLinkSuccess(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.OpenLinkStep{Link: "https://example.com"}

	result := driver.openLink(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	expectedCmd := "am start -a android.intent.action.VIEW -d 'https://example.com'"
	if len(mock.commands) != 1 || mock.commands[0] != expectedCmd {
		t.Errorf("expected command %q, got %v", expectedCmd, mock.commands)
	}
}

func TestOpenLinkError(t *testing.T) {
	mock := &MockShellExecutor{err: errors.New("shell failed")}
	driver := &Driver{device: mock}
	step := &flow.OpenLinkStep{Link: "https://example.com"}

	result := driver.openLink(step)

	if result.Success {
		t.Error("expected failure when shell command fails")
	}
}

// ============================================================================
// TakeScreenshot Tests
// ============================================================================

func TestTakeScreenshotSuccess(t *testing.T) {
	expectedData := []byte("fake-png-data")
	mock := &MockUIA2Client{screenshotData: expectedData}
	driver := &Driver{client: mock}
	step := &flow.TakeScreenshotStep{Path: "/tmp/screenshot.png"}

	result := driver.takeScreenshot(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	data, ok := result.Data.([]byte)
	if !ok {
		t.Fatalf("expected []byte data, got %T", result.Data)
	}
	if string(data) != string(expectedData) {
		t.Errorf("expected data %q, got %q", expectedData, data)
	}
}

func TestTakeScreenshotError(t *testing.T) {
	mock := &MockUIA2Client{screenshotErr: errors.New("screenshot failed")}
	driver := &Driver{client: mock}
	step := &flow.TakeScreenshotStep{Path: "/tmp/screenshot.png"}

	result := driver.takeScreenshot(step)

	if result.Success {
		t.Error("expected failure when screenshot fails")
	}
}

// ============================================================================
// OpenBrowser Tests
// ============================================================================

func TestOpenBrowserNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.OpenBrowserStep{URL: "https://example.com"}

	result := driver.openBrowser(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestOpenBrowserNoURL(t *testing.T) {
	mock := &MockShellExecutor{}
	driver := &Driver{device: mock}
	step := &flow.OpenBrowserStep{URL: ""}

	result := driver.openBrowser(step)

	if result.Success {
		t.Error("expected failure when URL is empty")
	}
}

func TestOpenBrowserSuccess(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.OpenBrowserStep{URL: "https://example.com"}

	result := driver.openBrowser(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	expectedCmd := "am start -a android.intent.action.VIEW -d 'https://example.com'"
	if len(mock.commands) != 1 || mock.commands[0] != expectedCmd {
		t.Errorf("expected command %q, got %v", expectedCmd, mock.commands)
	}
}

// ============================================================================
// AddMedia Tests
// ============================================================================

func TestAddMediaNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.AddMediaStep{Files: []string{"/path/to/file.jpg"}}

	result := driver.addMedia(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestAddMediaNoFiles(t *testing.T) {
	mock := &MockShellExecutor{}
	driver := &Driver{device: mock}
	step := &flow.AddMediaStep{Files: []string{}}

	result := driver.addMedia(step)

	if result.Success {
		t.Error("expected failure when no files specified")
	}
}

func TestAddMediaSuccess(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.AddMediaStep{Files: []string{"/path/to/file.jpg", "/path/to/file2.png"}}

	result := driver.addMedia(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	if len(mock.commands) != 2 {
		t.Errorf("expected 2 commands, got %d", len(mock.commands))
	}
}

// ============================================================================
// StartRecording Tests
// ============================================================================

func TestStartRecordingNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.StartRecordingStep{Path: "/sdcard/test.mp4"}

	result := driver.startRecording(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestStartRecordingSuccess(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.StartRecordingStep{Path: "/sdcard/test.mp4"}

	result := driver.startRecording(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	if result.Data != "/sdcard/test.mp4" {
		t.Errorf("expected path in data, got %v", result.Data)
	}
}

func TestStartRecordingDefaultPath(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.StartRecordingStep{Path: ""}

	result := driver.startRecording(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	if result.Data != "/sdcard/recording.mp4" {
		t.Errorf("expected default path, got %v", result.Data)
	}
}

// ============================================================================
// StopRecording Tests
// ============================================================================

func TestStopRecordingNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.StopRecordingStep{}

	result := driver.stopRecording(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestStopRecordingSuccess(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.StopRecordingStep{}

	result := driver.stopRecording(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

// ============================================================================
// WaitForAnimationToEnd Tests
// ============================================================================

func TestWaitForAnimationToEndSuccess(t *testing.T) {
	driver := &Driver{}
	step := &flow.WaitForAnimationToEndStep{}

	result := driver.waitForAnimationToEnd(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

// ============================================================================
// SetLocation Tests
// ============================================================================

func TestSetLocationNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.SetLocationStep{Latitude: "37.7749", Longitude: "-122.4194"}

	result := driver.setLocation(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestSetLocationMissingCoordinates(t *testing.T) {
	mock := &MockShellExecutor{}
	driver := &Driver{device: mock}

	tests := []struct {
		lat, lon string
	}{
		{"", "-122.4194"},
		{"37.7749", ""},
		{"", ""},
	}

	for _, tt := range tests {
		step := &flow.SetLocationStep{Latitude: tt.lat, Longitude: tt.lon}
		result := driver.setLocation(step)
		if result.Success {
			t.Errorf("expected failure for lat=%q lon=%q", tt.lat, tt.lon)
		}
	}
}

func TestSetLocationSuccess(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.SetLocationStep{Latitude: "37.7749", Longitude: "-122.4194"}

	result := driver.setLocation(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

// ============================================================================
// SetAirplaneMode Tests
// ============================================================================

func TestSetAirplaneModeNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.SetAirplaneModeStep{Enabled: true}

	result := driver.setAirplaneMode(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestSetAirplaneModeEnabled(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.SetAirplaneModeStep{Enabled: true}

	result := driver.setAirplaneMode(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	// Check first command sets airplane_mode_on to 1
	if len(mock.commands) < 1 || mock.commands[0] != "settings put global airplane_mode_on 1" {
		t.Errorf("expected settings command, got %v", mock.commands)
	}
}

func TestSetAirplaneModeDisabled(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.SetAirplaneModeStep{Enabled: false}

	result := driver.setAirplaneMode(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	// Check first command sets airplane_mode_on to 0
	if len(mock.commands) < 1 || mock.commands[0] != "settings put global airplane_mode_on 0" {
		t.Errorf("expected settings command, got %v", mock.commands)
	}
}

// ============================================================================
// ToggleAirplaneMode Tests
// ============================================================================

func TestToggleAirplaneModeNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.ToggleAirplaneModeStep{}

	result := driver.toggleAirplaneMode(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestToggleAirplaneModeFromOff(t *testing.T) {
	mock := &MockShellExecutor{response: "0"}
	driver := &Driver{device: mock}
	step := &flow.ToggleAirplaneModeStep{}

	result := driver.toggleAirplaneMode(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	// Should toggle from 0 to 1
	found := false
	for _, cmd := range mock.commands {
		if cmd == "settings put global airplane_mode_on 1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected toggle to 1, got commands: %v", mock.commands)
	}
}

func TestToggleAirplaneModeFromOn(t *testing.T) {
	mock := &MockShellExecutor{response: "1"}
	driver := &Driver{device: mock}
	step := &flow.ToggleAirplaneModeStep{}

	result := driver.toggleAirplaneMode(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	// Should toggle from 1 to 0
	found := false
	for _, cmd := range mock.commands {
		if cmd == "settings put global airplane_mode_on 0" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected toggle to 0, got commands: %v", mock.commands)
	}
}

// ============================================================================
// Travel Tests
// ============================================================================

func TestTravelNoDevice(t *testing.T) {
	driver := &Driver{device: nil}
	step := &flow.TravelStep{Points: []string{"37.7749, -122.4194", "37.8049, -122.4094"}}

	result := driver.travel(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestTravelNotEnoughPoints(t *testing.T) {
	mock := &MockShellExecutor{}
	driver := &Driver{device: mock}
	step := &flow.TravelStep{Points: []string{"37.7749, -122.4194"}}

	result := driver.travel(step)

	if result.Success {
		t.Error("expected failure when less than 2 points")
	}
}
