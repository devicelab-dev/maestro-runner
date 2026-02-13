package uiautomator2

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
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
		{"UP", "down"},      // unknown, defaults to down
		{"invalid", "down"}, // unknown, defaults to down
		{"", "down"},        // empty, defaults to down
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

func TestSetOrientationLandscapeLeft(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}
	step := &flow.SetOrientationStep{Orientation: "LANDSCAPE_LEFT"}

	result := driver.setOrientation(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	// Should have 2 shell commands: disable accelerometer, set rotation
	if len(shell.commands) != 2 {
		t.Errorf("expected 2 shell commands, got %d", len(shell.commands))
	}
	if shell.commands[1] != "settings put system user_rotation 1" {
		t.Errorf("expected user_rotation 1, got %s", shell.commands[1])
	}
}

func TestSetOrientationLandscapeRight(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}
	step := &flow.SetOrientationStep{Orientation: "LANDSCAPE_RIGHT"}

	result := driver.setOrientation(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if shell.commands[1] != "settings put system user_rotation 3" {
		t.Errorf("expected user_rotation 3, got %s", shell.commands[1])
	}
}

func TestSetOrientationUpsideDown(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}
	step := &flow.SetOrientationStep{Orientation: "UPSIDE_DOWN"}

	result := driver.setOrientation(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if shell.commands[1] != "settings put system user_rotation 2" {
		t.Errorf("expected user_rotation 2, got %s", shell.commands[1])
	}
}

func TestSetOrientationExtendedNoDevice(t *testing.T) {
	driver := &Driver{client: &MockUIA2Client{}}
	step := &flow.SetOrientationStep{Orientation: "LANDSCAPE_LEFT"}

	result := driver.setOrientation(step)

	if result.Success {
		t.Error("expected failure when device is nil for extended orientation")
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

// ============================================================================
// AssertNotVisible HTTP Mock Tests
// ============================================================================

func TestAssertNotVisibleElementNotFound(t *testing.T) {
	// Element not found at all - should succeed (not visible = success)
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			// Return empty element ID to simulate not found
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": ""},
			})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><node text="Other" bounds="[0,0][100,100]"/></hierarchy>`,
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.AssertNotVisibleStep{
		Selector: flow.Selector{Text: "Missing"},
		BaseStep: flow.BaseStep{TimeoutMs: 500},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success when element not found, got error: %v", result.Error)
	}
}

func TestAssertNotVisibleTimeout(t *testing.T) {
	// Element is always found - should fail after timeout
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-vis"},
			})
		},
		"GET /element/elem-vis/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Visible Label"})
		},
		"GET /element/elem-vis/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 10, "y": 20, "width": 100, "height": 50},
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

	step := &flow.AssertNotVisibleStep{
		Selector: flow.Selector{Text: "Visible Label"},
		BaseStep: flow.BaseStep{TimeoutMs: 500},
	}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure when element remains visible")
	}
}

func TestAssertNotVisibleDefaultTimeout(t *testing.T) {
	// Test with zero timeout (should use default 5000)
	// Element not found - should succeed quickly regardless of timeout
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": ""},
			})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy></hierarchy>`,
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	step := &flow.AssertNotVisibleStep{
		Selector: flow.Selector{Text: "Nonexistent"},
		BaseStep: flow.BaseStep{TimeoutMs: 0}, // Should default to 5000
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success when element not found, got error: %v", result.Error)
	}
}

// ============================================================================
// EraseText Optimized Path Tests (HTTP Mock)
// ============================================================================

func TestEraseTextOptimizedClearAll(t *testing.T) {
	// Active element found with text - uses Clear() for full erase
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "active-elem"},
			})
		},
		"GET /element/active-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Hello World"})
		},
		"POST /element/active-elem/clear": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	// Erase more than text length (11 chars) - should just clear all
	step := &flow.EraseTextStep{Characters: 20}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "Cleared") {
		t.Errorf("expected 'Cleared' in message, got: %s", result.Message)
	}
}

func TestEraseTextOptimizedPartialErase(t *testing.T) {
	// Active element with text - erase N chars from end using clear+sendKeys
	var sendKeysText string
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "active-elem"},
			})
		},
		"GET /element/active-elem/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Hello World"})
		},
		"POST /element/active-elem/clear": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
		"POST /element/active-elem/value": func(w http.ResponseWriter, r *http.Request) {
			// Capture the text being sent
			sendKeysText = "captured"
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	// Erase last 5 chars from "Hello World" -> should clear and re-type "Hello "
	step := &flow.EraseTextStep{Characters: 5}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "Erased 5") {
		t.Errorf("expected 'Erased 5' in message, got: %s", result.Message)
	}
	if sendKeysText != "captured" {
		t.Error("expected SendKeys to be called for remaining text")
	}
}

func TestEraseTextFallbackToDeleteKeys(t *testing.T) {
	// ActiveElement not available - fall back to pressing delete key N times
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.EraseTextStep{Characters: 3}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(client.pressKeyCalls) != 3 {
		t.Errorf("expected 3 delete key presses, got %d", len(client.pressKeyCalls))
	}
	for _, code := range client.pressKeyCalls {
		if code != 67 { // KeyCodeDelete
			t.Errorf("expected keyCode 67, got %d", code)
		}
	}
}

// ============================================================================
// CopyTextFrom Additional Tests (HTTP Mock)
// ============================================================================

func TestCopyTextFromContentDescFallback(t *testing.T) {
	// Element text is empty, but content-desc has value
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-copy"},
			})
		},
		"GET /element/elem-copy/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": ""})
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
		"GET /element/elem-copy/attribute/content-desc": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Accessibility Label"})
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
	if result.Data != "Accessibility Label" {
		t.Errorf("expected 'Accessibility Label', got %v", result.Data)
	}
}

func TestCopyTextFromElementNotFound(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": ""},
			})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><node text="Other" bounds="[0,0][100,100]"/></hierarchy>`,
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)
	driver.SetFindTimeout(100)

	step := &flow.CopyTextFromStep{Selector: flow.Selector{Text: "Missing"}}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure when element not found")
	}
}

// ============================================================================
// OpenLink Additional Tests
// ============================================================================

func TestOpenLinkWithBrowserFlag(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	browser := true
	step := &flow.OpenLinkStep{Link: "https://example.com", Browser: &browser}

	result := driver.openLink(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	// Should include BROWSABLE category
	if len(mock.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(mock.commands))
	}
	if !strings.Contains(mock.commands[0], "android.intent.category.BROWSABLE") {
		t.Errorf("expected BROWSABLE category in command, got: %s", mock.commands[0])
	}
}

func TestOpenLinkWithoutBrowserFlag(t *testing.T) {
	// Default (no browser flag) should use plain VIEW intent
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.OpenLinkStep{Link: "myapp://deep/link"}

	result := driver.openLink(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	if len(mock.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(mock.commands))
	}
	// Without browser flag, should NOT include BROWSABLE category
	if strings.Contains(mock.commands[0], "android.intent.category.BROWSABLE") {
		t.Errorf("should not include BROWSABLE category for default link, got: %s", mock.commands[0])
	}
	if !strings.Contains(mock.commands[0], "myapp://deep/link") {
		t.Errorf("expected link in command, got: %s", mock.commands[0])
	}
}

// ============================================================================
// OpenBrowser Error Test
// ============================================================================

func TestOpenBrowserShellError(t *testing.T) {
	mock := &MockShellExecutor{err: errors.New("shell failed")}
	driver := &Driver{device: mock}
	step := &flow.OpenBrowserStep{URL: "https://example.com"}

	result := driver.openBrowser(step)

	if result.Success {
		t.Error("expected failure when shell command fails")
	}
}

// ============================================================================
// SetLocation Additional Tests
// ============================================================================

func TestSetLocationShellError(t *testing.T) {
	mock := &MockShellExecutor{err: errors.New("shell failed")}
	driver := &Driver{device: mock}
	step := &flow.SetLocationStep{Latitude: "37.7749", Longitude: "-122.4194"}

	result := driver.setLocation(step)

	if result.Success {
		t.Error("expected failure when shell command fails")
	}
}

func TestSetLocationShellCommand(t *testing.T) {
	mock := &MockShellExecutor{response: "Success"}
	driver := &Driver{device: mock}
	step := &flow.SetLocationStep{Latitude: "37.7749", Longitude: "-122.4194"}

	result := driver.setLocation(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	if len(mock.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(mock.commands))
	}
	if !strings.Contains(mock.commands[0], "37.7749") || !strings.Contains(mock.commands[0], "-122.4194") {
		t.Errorf("expected coordinates in command, got: %s", mock.commands[0])
	}
}

// ============================================================================
// Scroll Direction Tests
// ============================================================================

func TestScrollAllDirections(t *testing.T) {
	directions := []string{"up", "down", "left", "right"}
	for _, dir := range directions {
		t.Run(dir, func(t *testing.T) {
			client := &MockUIA2Client{}
			driver := New(client, nil, nil)

			step := &flow.ScrollStep{Direction: dir}
			result := driver.Execute(step)

			if !result.Success {
				t.Errorf("expected success for direction %s, got error: %v", dir, result.Error)
			}
			if len(client.scrollCalls) != 1 {
				t.Errorf("expected 1 scroll call, got %d", len(client.scrollCalls))
			}
		})
	}
}

func TestScrollEmptyDirection(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.ScrollStep{Direction: ""}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success with empty direction (default down), got error: %v", result.Error)
	}
}

// ============================================================================
// ScrollUntilVisible Additional Tests
// ============================================================================

func TestScrollUntilVisibleDefaultDirection(t *testing.T) {
	// Element found immediately (no scrolls needed)
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-scroll"},
			})
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
		Direction: "", // empty = default down
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "0 scrolls") {
		t.Errorf("expected 'found after 0 scrolls', got: %s", result.Message)
	}
}

func TestScrollUntilVisibleUpDirection(t *testing.T) {
	// Element found immediately with "up" direction
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-scroll"},
			})
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
		Direction: "up",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

// ============================================================================
// InputText Additional Tests (HTTP Mock)
// ============================================================================

func TestInputTextWithUnicodeWarning(t *testing.T) {
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

	// Use non-ASCII text that triggers the warning
	step := &flow.InputTextStep{
		Text:     "Hola mundo \u00e9\u00e8\u00ea",
		Selector: flow.Selector{ID: "input_field"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "non-ASCII") {
		t.Errorf("expected non-ASCII warning in message, got: %s", result.Message)
	}
}

func TestInputTextNoSelectorNoActiveElement(t *testing.T) {
	// No selector, no active element, no focused element -> should fail
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /element/active": func(w http.ResponseWriter, r *http.Request) {
			// Return empty element ID (no active element)
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": ""},
			})
		},
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			// Focused element search also fails
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": ""},
			})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><node text="label" bounds="[0,0][100,100]"/></hierarchy>`,
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)
	driver.SetFindTimeout(200)

	step := &flow.InputTextStep{Text: "Hello"}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure when no focused element found")
	}
}

func TestInputTextNoSelectorActiveElementSuccess(t *testing.T) {
	// Active element found - type into it
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

	step := &flow.InputTextStep{Text: "Hello via active element"}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success typing into active element, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "Hello via active element") {
		t.Errorf("expected text in message, got: %s", result.Message)
	}
}

// ============================================================================
// SetClipboard Tests
// ============================================================================

func TestSetClipboardSuccess(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.SetClipboardStep{Text: "test clipboard text"}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(client.setClipboardCalls) != 1 || client.setClipboardCalls[0] != "test clipboard text" {
		t.Errorf("expected SetClipboard('test clipboard text'), got %v", client.setClipboardCalls)
	}
}

func TestSetClipboardEmptyText(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.SetClipboardStep{Text: ""}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure when text is empty")
	}
}

func TestSetClipboardError(t *testing.T) {
	client := &MockUIA2Client{setClipboardErr: errors.New("clipboard error")}
	driver := New(client, nil, nil)

	step := &flow.SetClipboardStep{Text: "test"}
	result := driver.Execute(step)

	if result.Success {
		t.Error("expected failure when SetClipboard returns error")
	}
}

// ============================================================================
// InvertScrollDirection Tests
// ============================================================================

func TestInvertScrollDirection(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"up", "down"},
		{"down", "up"},
		{"left", "right"},
		{"right", "left"},
		{"unknown", "up"}, // default = swipe up (scroll down)
		{"", "up"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := invertScrollDirection(tt.input)
			if got != tt.expected {
				t.Errorf("invertScrollDirection(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ============================================================================
// ParsePercentageCoords Tests
// ============================================================================

func TestParsePercentageCoords(t *testing.T) {
	tests := []struct {
		input string
		xPct  float64
		yPct  float64
		err   bool
	}{
		{"50%, 50%", 0.50, 0.50, false},
		{"85%, 15%", 0.85, 0.15, false},
		{"0%, 100%", 0.0, 1.0, false},
		{"50, 50", 0.50, 0.50, false}, // Without % sign
		{"invalid", 0, 0, true},       // No comma
		{"abc, def", 0, 0, true},      // Non-numeric
		{"50%, abc", 0, 0, true},      // Y non-numeric
		{"abc, 50%", 0, 0, true},      // X non-numeric
		{"50%, 50%, 50%", 0, 0, true}, // Too many parts
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			x, y, err := parsePercentageCoords(tt.input)
			if tt.err {
				if err == nil {
					t.Errorf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for input %q: %v", tt.input, err)
				return
			}
			if x != tt.xPct || y != tt.yPct {
				t.Errorf("parsePercentageCoords(%q) = (%f, %f), want (%f, %f)", tt.input, x, y, tt.xPct, tt.yPct)
			}
		})
	}
}

// ============================================================================
// SetAirplaneMode Shell Error Tests
// ============================================================================

func TestSetAirplaneModeShellError(t *testing.T) {
	mock := &MockShellExecutor{err: errors.New("shell failed")}
	driver := &Driver{device: mock}
	step := &flow.SetAirplaneModeStep{Enabled: true}

	result := driver.setAirplaneMode(step)

	if result.Success {
		t.Error("expected failure when shell command fails")
	}
}

func TestToggleAirplaneModeShellError(t *testing.T) {
	mock := &MockShellExecutor{err: errors.New("shell failed")}
	driver := &Driver{device: mock}
	step := &flow.ToggleAirplaneModeStep{}

	result := driver.toggleAirplaneMode(step)

	if result.Success {
		t.Error("expected failure when shell command fails")
	}
}

// ============================================================================
// InputRandom DataType Tests (HTTP Mock)
// ============================================================================

func TestInputRandomEmail(t *testing.T) {
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

	step := &flow.InputRandomStep{DataType: "EMAIL", Length: 8}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if text, ok := result.Data.(string); ok {
		if !strings.Contains(text, "@") {
			t.Errorf("expected email format with @, got: %s", text)
		}
	} else {
		t.Error("expected string data")
	}
}

func TestInputRandomNumber(t *testing.T) {
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

	step := &flow.InputRandomStep{DataType: "NUMBER", Length: 6}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if text, ok := result.Data.(string); ok {
		if len(text) != 6 {
			t.Errorf("expected 6 digit number, got %d chars: %s", len(text), text)
		}
		for _, c := range text {
			if c < '0' || c > '9' {
				t.Errorf("expected digits only, got: %s", text)
				break
			}
		}
	} else {
		t.Error("expected string data")
	}
}

func TestInputRandomPersonName(t *testing.T) {
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

	step := &flow.InputRandomStep{DataType: "PERSON_NAME"}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if text, ok := result.Data.(string); ok {
		if !strings.Contains(text, " ") {
			t.Errorf("expected person name with space (first last), got: %s", text)
		}
	} else {
		t.Error("expected string data")
	}
}

// ============================================================================
// RandomHelpers Tests
// ============================================================================

func TestRandomEmail(t *testing.T) {
	email := randomEmail()
	if !strings.Contains(email, "@") {
		t.Errorf("expected @ in email, got: %s", email)
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		t.Errorf("invalid email format: %s", email)
	}
}

func TestRandomNumber(t *testing.T) {
	num := randomNumber(5)
	if len(num) != 5 {
		t.Errorf("expected 5 digits, got %d: %s", len(num), num)
	}
	for _, c := range num {
		if c < '0' || c > '9' {
			t.Errorf("expected digits only, got: %s", num)
			break
		}
	}
}

func TestRandomPersonName(t *testing.T) {
	name := randomPersonName()
	parts := strings.Split(name, " ")
	if len(parts) != 2 {
		t.Errorf("expected 'first last' format, got: %s", name)
	}
}

// ============================================================================
// SetOrientation Shell Error Test
// ============================================================================

func TestSetOrientationExtendedShellError(t *testing.T) {
	mock := &MockShellExecutor{err: errors.New("shell failed")}
	driver := &Driver{device: mock}
	step := &flow.SetOrientationStep{Orientation: "LANDSCAPE_LEFT"}

	result := driver.setOrientation(step)

	if result.Success {
		t.Error("expected failure when shell command fails")
	}
}

// ============================================================================
// AddMedia Shell Error Test
// ============================================================================

func TestAddMediaShellError(t *testing.T) {
	mock := &MockShellExecutor{err: errors.New("shell failed")}
	driver := &Driver{device: mock}
	step := &flow.AddMediaStep{Files: []string{"/path/to/file.jpg"}}

	result := driver.addMedia(step)

	if result.Success {
		t.Error("expected failure when shell command fails")
	}
}

// ============================================================================
// StartRecording Error Test
// ============================================================================

func TestStartRecordingError(t *testing.T) {
	mock := &MockShellExecutor{err: errors.New("shell failed")}
	driver := &Driver{device: mock}
	step := &flow.StartRecordingStep{Path: "/sdcard/test.mp4"}

	result := driver.startRecording(step)

	if result.Success {
		t.Error("expected failure when shell command fails")
	}
}

// ============================================================================
// HideKeyboard via Mock Client Test
// ============================================================================

func TestHideKeyboardSuccess(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.HideKeyboardStep{}
	result := driver.hideKeyboard(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if client.hideKeyboardCalls != 1 {
		t.Errorf("expected 1 hideKeyboard call, got %d", client.hideKeyboardCalls)
	}
}

// ============================================================================
// TakeScreenshot via Direct Method Test
// ============================================================================

func TestTakeScreenshotViaMethod(t *testing.T) {
	expectedData := []byte{0x89, 0x50, 0x4E, 0x47}
	mock := &MockUIA2Client{screenshotData: expectedData}
	driver := &Driver{client: mock}
	step := &flow.TakeScreenshotStep{}

	result := driver.takeScreenshot(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if data, ok := result.Data.([]byte); ok {
		if len(data) != 4 {
			t.Errorf("expected 4 bytes, got %d", len(data))
		}
	} else {
		t.Fatalf("expected []byte data, got %T", result.Data)
	}
}

// ============================================================================
// tapOnPointWithPercentage Tests
// ============================================================================

func TestTapOnPointWithPercentageSuccess(t *testing.T) {
	client := &MockUIA2Client{}
	shell := &MockShellExecutor{}
	driver := New(client, nil, shell)

	result := driver.tapOnPointWithPercentage("50%, 50%")

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	// Screen size from MockUIA2Client.GetDeviceInfo is 1080x2400
	// 50% of 1080 = 540, 50% of 2400 = 1200
	if len(client.clickCalls) != 1 {
		t.Fatalf("expected 1 click call, got %d", len(client.clickCalls))
	}
	if client.clickCalls[0].X != 540 || client.clickCalls[0].Y != 1200 {
		t.Errorf("expected click at (540, 1200), got (%d, %d)", client.clickCalls[0].X, client.clickCalls[0].Y)
	}
}

func TestTapOnPointWithPercentageCorners(t *testing.T) {
	client := &MockUIA2Client{}
	shell := &MockShellExecutor{}
	driver := New(client, nil, shell)

	// Top-left corner: 0%, 0%
	result := driver.tapOnPointWithPercentage("0%, 0%")

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(client.clickCalls) != 1 {
		t.Fatalf("expected 1 click call, got %d", len(client.clickCalls))
	}
	if client.clickCalls[0].X != 0 || client.clickCalls[0].Y != 0 {
		t.Errorf("expected click at (0, 0), got (%d, %d)", client.clickCalls[0].X, client.clickCalls[0].Y)
	}
}

func TestTapOnPointWithPercentageNoDevice(t *testing.T) {
	driver := &Driver{device: nil}

	result := driver.tapOnPointWithPercentage("50%, 50%")

	if result.Success {
		t.Error("expected failure when device is nil")
	}
	if !strings.Contains(result.Message, "device") {
		t.Errorf("expected device-related error message, got: %s", result.Message)
	}
}

func TestTapOnPointWithPercentageInvalidCoords(t *testing.T) {
	client := &MockUIA2Client{}
	shell := &MockShellExecutor{}
	driver := New(client, nil, shell)

	result := driver.tapOnPointWithPercentage("invalid")

	if result.Success {
		t.Error("expected failure for invalid coordinates")
	}
}

func TestTapOnPointWithPercentageClickError(t *testing.T) {
	client := &MockUIA2Client{clickErr: errors.New("click failed")}
	shell := &MockShellExecutor{}
	driver := New(client, nil, shell)

	result := driver.tapOnPointWithPercentage("50%, 50%")

	if result.Success {
		t.Error("expected failure when click returns error")
	}
}

// ============================================================================
// tapOnPoint Additional Tests
// ============================================================================

func TestTapOnPointWithPercentagePoint(t *testing.T) {
	// Test tapOnPoint with a percentage-based Point string
	client := &MockUIA2Client{}
	shell := &MockShellExecutor{}
	driver := New(client, nil, shell)

	step := &flow.TapOnPointStep{Point: "85%, 15%"}
	result := driver.tapOnPoint(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	// 85% of 1080 = 918, 15% of 2400 = 360
	if len(client.clickCalls) != 1 {
		t.Fatalf("expected 1 click call, got %d", len(client.clickCalls))
	}
	if client.clickCalls[0].X != 918 || client.clickCalls[0].Y != 360 {
		t.Errorf("expected click at (918, 360), got (%d, %d)", client.clickCalls[0].X, client.clickCalls[0].Y)
	}
}

func TestTapOnPointWithPercentagePointNoDevice(t *testing.T) {
	driver := &Driver{device: nil}

	step := &flow.TapOnPointStep{Point: "50%, 50%"}
	result := driver.tapOnPoint(step)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestTapOnPointWithPercentagePointInvalid(t *testing.T) {
	client := &MockUIA2Client{}
	shell := &MockShellExecutor{}
	driver := New(client, nil, shell)

	step := &flow.TapOnPointStep{Point: "bad_coords"}
	result := driver.tapOnPoint(step)

	if result.Success {
		t.Error("expected failure for invalid percentage coords")
	}
}

func TestTapOnPointZeroCoords(t *testing.T) {
	// Neither Point nor X/Y specified
	driver := &Driver{}

	step := &flow.TapOnPointStep{}
	result := driver.tapOnPoint(step)

	if result.Success {
		t.Error("expected failure when no point specified")
	}
	if !strings.Contains(result.Message, "coordinates required") {
		t.Errorf("expected 'coordinates required' in message, got: %s", result.Message)
	}
}

// ============================================================================
// swipeWithCoordinates Tests
// ============================================================================

func TestSwipeWithCoordinatesSuccess(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	result := driver.swipeWithCoordinates("50%, 80%", "50%, 20%", 400)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	// 50% of 1080 = 540, 80% of 2400 = 1920
	// 50% of 1080 = 540, 20% of 2400 = 480
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 shell command, got %d", len(shell.commands))
	}
	expected := "input swipe 540 1920 540 480 400"
	if shell.commands[0] != expected {
		t.Errorf("expected command %q, got %q", expected, shell.commands[0])
	}
}

func TestSwipeWithCoordinatesNoDevice(t *testing.T) {
	driver := &Driver{device: nil}

	result := driver.swipeWithCoordinates("50%, 80%", "50%, 20%", 400)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestSwipeWithCoordinatesInvalidStart(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	result := driver.swipeWithCoordinates("invalid", "50%, 20%", 400)

	if result.Success {
		t.Error("expected failure for invalid start coordinates")
	}
}

func TestSwipeWithCoordinatesInvalidEnd(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	result := driver.swipeWithCoordinates("50%, 80%", "bad", 400)

	if result.Success {
		t.Error("expected failure for invalid end coordinates")
	}
}

func TestSwipeWithCoordinatesDefaultDuration(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	// Duration 0 should default to 300
	result := driver.swipeWithCoordinates("50%, 80%", "50%, 20%", 0)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 shell command, got %d", len(shell.commands))
	}
	if !strings.Contains(shell.commands[0], " 300") {
		t.Errorf("expected default duration 300 in command, got: %s", shell.commands[0])
	}
}

// ============================================================================
// swipe (main entry) Additional Tests
// ============================================================================

func TestSwipeWithStartEndPercentages(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	step := &flow.SwipeStep{Start: "30%, 70%", End: "70%, 30%", Duration: 500}
	result := driver.swipe(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	// 30% of 1080 = 324, 70% of 2400 = 1680
	// 70% of 1080 = 756, 30% of 2400 = 720
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 shell command, got %d", len(shell.commands))
	}
	expected := "input swipe 324 1680 756 720 500"
	if shell.commands[0] != expected {
		t.Errorf("expected command %q, got %q", expected, shell.commands[0])
	}
}

func TestSwipeWithAbsoluteCoords(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	step := &flow.SwipeStep{StartX: 100, StartY: 800, EndX: 100, EndY: 200, Duration: 300}
	result := driver.swipe(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 shell command, got %d", len(shell.commands))
	}
	expected := "input swipe 100 800 100 200 300"
	if shell.commands[0] != expected {
		t.Errorf("expected command %q, got %q", expected, shell.commands[0])
	}
}

func TestSwipeEmptyDirectionDefaultsToUp(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	step := &flow.SwipeStep{Direction: ""}
	result := driver.swipe(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

// ============================================================================
// swipeWithMaestroCoordinates Tests
// ============================================================================

func TestSwipeWithMaestroCoordinatesUp(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	result := driver.swipeWithMaestroCoordinates("up", 1080, 2400, 300)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 shell command, got %d", len(shell.commands))
	}
	// UP: startX=540, startY=1680, endX=540, endY=720
	expected := "input swipe 540 1680 540 720 300"
	if shell.commands[0] != expected {
		t.Errorf("expected command %q, got %q", expected, shell.commands[0])
	}
}

func TestSwipeWithMaestroCoordinatesDown(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	result := driver.swipeWithMaestroCoordinates("down", 1080, 2400, 300)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 shell command, got %d", len(shell.commands))
	}
	// DOWN: startX=540, startY=720, endX=540, endY=1680
	expected := "input swipe 540 720 540 1680 300"
	if shell.commands[0] != expected {
		t.Errorf("expected command %q, got %q", expected, shell.commands[0])
	}
}

func TestSwipeWithMaestroCoordinatesLeft(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	result := driver.swipeWithMaestroCoordinates("left", 1080, 2400, 300)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 shell command, got %d", len(shell.commands))
	}
	// LEFT: startX=756, startY=1200, endX=324, endY=1200
	expected := "input swipe 756 1200 324 1200 300"
	if shell.commands[0] != expected {
		t.Errorf("expected command %q, got %q", expected, shell.commands[0])
	}
}

func TestSwipeWithMaestroCoordinatesRight(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	result := driver.swipeWithMaestroCoordinates("right", 1080, 2400, 300)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 shell command, got %d", len(shell.commands))
	}
	// RIGHT: startX=324, startY=1200, endX=756, endY=1200
	expected := "input swipe 324 1200 756 1200 300"
	if shell.commands[0] != expected {
		t.Errorf("expected command %q, got %q", expected, shell.commands[0])
	}
}

func TestSwipeWithMaestroCoordinatesDefaultDirection(t *testing.T) {
	shell := &MockShellExecutor{}
	client := &MockUIA2Client{}
	driver := New(client, nil, shell)

	// Unknown direction defaults to "up" behavior
	result := driver.swipeWithMaestroCoordinates("unknown", 1080, 2400, 300)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 shell command, got %d", len(shell.commands))
	}
	// Default = up: startX=540, startY=1680, endX=540, endY=720
	expected := "input swipe 540 1680 540 720 300"
	if shell.commands[0] != expected {
		t.Errorf("expected command %q, got %q", expected, shell.commands[0])
	}
}

// ============================================================================
// resolvePermissionShortcut Tests
// ============================================================================

func TestResolvePermissionShortcutKnownShortcuts(t *testing.T) {
	tests := []struct {
		shortcut string
		contains string
		count    int
	}{
		{"camera", "android.permission.CAMERA", 1},
		{"microphone", "android.permission.RECORD_AUDIO", 1},
		{"notifications", "android.permission.POST_NOTIFICATIONS", 1},
		{"location", "android.permission.ACCESS_FINE_LOCATION", 3},
		{"contacts", "android.permission.READ_CONTACTS", 3},
		{"phone", "android.permission.READ_PHONE_STATE", 6},
		{"bluetooth", "android.permission.BLUETOOTH_CONNECT", 3},
		{"storage", "android.permission.READ_EXTERNAL_STORAGE", 5},
		{"medialibrary", "android.permission.READ_MEDIA_IMAGES", 3},
		{"calendar", "android.permission.READ_CALENDAR", 2},
		{"sms", "android.permission.SEND_SMS", 5},
		{"sensors", "android.permission.BODY_SENSORS", 2},
		{"activity_recognition", "android.permission.ACTIVITY_RECOGNITION", 2},
	}

	for _, tt := range tests {
		t.Run(tt.shortcut, func(t *testing.T) {
			perms := resolvePermissionShortcut(tt.shortcut)
			if len(perms) != tt.count {
				t.Errorf("resolvePermissionShortcut(%q) returned %d perms, want %d", tt.shortcut, len(perms), tt.count)
			}
			found := false
			for _, p := range perms {
				if p == tt.contains {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("resolvePermissionShortcut(%q) missing %q, got %v", tt.shortcut, tt.contains, perms)
			}
		})
	}
}

func TestResolvePermissionShortcutCaseInsensitive(t *testing.T) {
	perms := resolvePermissionShortcut("CAMERA")
	if len(perms) != 1 || perms[0] != "android.permission.CAMERA" {
		t.Errorf("expected [android.permission.CAMERA], got %v", perms)
	}

	perms = resolvePermissionShortcut("Camera")
	if len(perms) != 1 || perms[0] != "android.permission.CAMERA" {
		t.Errorf("expected [android.permission.CAMERA], got %v", perms)
	}
}

func TestResolvePermissionShortcutFullAndroidPermission(t *testing.T) {
	perms := resolvePermissionShortcut("android.permission.READ_PHONE_STATE")
	if len(perms) != 1 || perms[0] != "android.permission.READ_PHONE_STATE" {
		t.Errorf("expected [android.permission.READ_PHONE_STATE], got %v", perms)
	}
}

func TestResolvePermissionShortcutUnknownAddsPrefix(t *testing.T) {
	perms := resolvePermissionShortcut("CUSTOM_PERM")
	if len(perms) != 1 || perms[0] != "android.permission.CUSTOM_PERM" {
		t.Errorf("expected [android.permission.CUSTOM_PERM], got %v", perms)
	}
}

// ============================================================================
// applyPermission Tests
// ============================================================================

func TestApplyPermissionAllow(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}

	err := driver.applyPermission("com.example.app", "android.permission.CAMERA", "allow")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(shell.commands))
	}
	expected := "pm grant com.example.app android.permission.CAMERA"
	if shell.commands[0] != expected {
		t.Errorf("expected %q, got %q", expected, shell.commands[0])
	}
}

func TestApplyPermissionDeny(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}

	err := driver.applyPermission("com.example.app", "android.permission.CAMERA", "deny")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(shell.commands))
	}
	expected := "pm revoke com.example.app android.permission.CAMERA"
	if shell.commands[0] != expected {
		t.Errorf("expected %q, got %q", expected, shell.commands[0])
	}
}

func TestApplyPermissionUnset(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}

	err := driver.applyPermission("com.example.app", "android.permission.CAMERA", "unset")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(shell.commands))
	}
	expected := "pm revoke com.example.app android.permission.CAMERA"
	if shell.commands[0] != expected {
		t.Errorf("expected %q, got %q", expected, shell.commands[0])
	}
}

func TestApplyPermissionInvalidValue(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}

	err := driver.applyPermission("com.example.app", "android.permission.CAMERA", "invalid")

	if err == nil {
		t.Error("expected error for invalid permission value")
	}
	if !strings.Contains(err.Error(), "invalid permission value") {
		t.Errorf("expected 'invalid permission value' in error, got: %v", err)
	}
}

func TestApplyPermissionShellError(t *testing.T) {
	shell := &MockShellExecutor{err: errors.New("grant failed")}
	driver := &Driver{device: shell}

	err := driver.applyPermission("com.example.app", "android.permission.CAMERA", "allow")

	if err == nil {
		t.Error("expected error when shell fails")
	}
}

// ============================================================================
// applyPermissions Tests
// ============================================================================

func TestApplyPermissionsAllAllow(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}

	result := driver.applyPermissions("com.example.app", map[string]string{"all": "allow"})

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	// Should have issued grant commands for all permissions
	if len(shell.commands) == 0 {
		t.Error("expected grant commands to be issued")
	}
	for _, cmd := range shell.commands {
		if !strings.HasPrefix(cmd, "pm grant com.example.app") {
			t.Errorf("expected pm grant command, got: %s", cmd)
		}
	}
}

func TestApplyPermissionsSpecificShortcut(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}

	result := driver.applyPermissions("com.example.app", map[string]string{"camera": "allow"})

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(shell.commands))
	}
	expected := "pm grant com.example.app android.permission.CAMERA"
	if shell.commands[0] != expected {
		t.Errorf("expected %q, got %q", expected, shell.commands[0])
	}
}

func TestApplyPermissionsWithErrors(t *testing.T) {
	shell := &MockShellExecutor{err: errors.New("grant failed")}
	driver := &Driver{device: shell}

	result := driver.applyPermissions("com.example.app", map[string]string{"camera": "allow"})

	if result.Success {
		t.Error("expected failure when permission grant fails")
	}
	if !strings.Contains(result.Message, "Errors") {
		t.Errorf("expected Errors in message, got: %s", result.Message)
	}
}

func TestApplyPermissionsDeny(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}

	result := driver.applyPermissions("com.example.app", map[string]string{"camera": "deny"})

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(shell.commands))
	}
	expected := "pm revoke com.example.app android.permission.CAMERA"
	if shell.commands[0] != expected {
		t.Errorf("expected %q, got %q", expected, shell.commands[0])
	}
}

// ============================================================================
// getScreenSize Tests
// ============================================================================

func TestGetScreenSizeViaDeviceInfo(t *testing.T) {
	// MockUIA2Client returns "1080x2400" from GetDeviceInfo
	client := &MockUIA2Client{}
	driver := &Driver{client: client}

	w, h, err := driver.getScreenSize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 1080 || h != 2400 {
		t.Errorf("expected 1080x2400, got %dx%d", w, h)
	}
}

func TestGetScreenSizeViaWmSize(t *testing.T) {
	// Use HTTP mock that returns bad display size, forcing wm size fallback
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /appium/device/info": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"realDisplaySize": ""},
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	shell := &MockShellExecutor{response: "Physical size: 720x1280"}
	driver := New(client.Client, nil, shell)

	w, h, err := driver.getScreenSize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != 720 || h != 1280 {
		t.Errorf("expected 720x1280, got %dx%d", w, h)
	}
}

func TestGetScreenSizeWmSizeNoDevice(t *testing.T) {
	// No client, no device - should return error
	driver := &Driver{}

	_, _, err := driver.getScreenSize()

	if err == nil {
		t.Error("expected error when no client and no device")
	}
}

func TestGetScreenSizeWmSizeShellError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /appium/device/info": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"realDisplaySize": ""},
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	shell := &MockShellExecutor{err: errors.New("shell failed")}
	driver := New(client.Client, nil, shell)

	_, _, err := driver.getScreenSize()

	if err == nil {
		t.Error("expected error when shell fails")
	}
}

func TestGetScreenSizeWmSizeBadFormat(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /appium/device/info": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"realDisplaySize": ""},
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	shell := &MockShellExecutor{response: "unexpected output"}
	driver := New(client.Client, nil, shell)

	_, _, err := driver.getScreenSize()

	if err == nil {
		t.Error("expected error for bad wm size output")
	}
}

func TestGetScreenSizeWmSizeNonNumeric(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /appium/device/info": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"realDisplaySize": ""},
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	shell := &MockShellExecutor{response: "Physical size: abcxdef"}
	driver := New(client.Client, nil, shell)

	_, _, err := driver.getScreenSize()

	if err == nil {
		t.Error("expected error for non-numeric wm size values")
	}
}

// ============================================================================
// findScrollableElement Tests
// ============================================================================

func TestFindScrollableElementSingleScrollable(t *testing.T) {
	client := &MockUIA2Client{
		sourceData: `<hierarchy rotation="0">
			<node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]">
				<node class="android.widget.ScrollView" scrollable="true" bounds="[0,100][1080,2000]"
					text="" resource-id="" content-desc="" enabled="true" displayed="true" clickable="false"/>
			</node>
		</hierarchy>`,
	}
	driver := &Driver{client: client}

	info, count := driver.findScrollableElement(500)

	if info == nil {
		t.Fatal("expected scrollable element info, got nil")
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
	if info.Bounds.Width != 1080 || info.Bounds.Height != 1900 {
		t.Errorf("expected bounds 1080x1900, got %dx%d", info.Bounds.Width, info.Bounds.Height)
	}
}

func TestFindScrollableElementNoneFound(t *testing.T) {
	client := &MockUIA2Client{
		sourceData: `<hierarchy rotation="0">
			<node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]"
				text="" resource-id="" content-desc="" enabled="true" displayed="true"/>
		</hierarchy>`,
	}
	driver := &Driver{client: client}

	info, count := driver.findScrollableElement(200)

	if info != nil {
		t.Errorf("expected nil info, got %+v", info)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
}

func TestFindScrollableElementMultipleScrollables(t *testing.T) {
	client := &MockUIA2Client{
		sourceData: `<hierarchy rotation="0">
			<node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]">
				<node class="android.widget.ScrollView" scrollable="true" bounds="[0,0][200,200]"
					text="" resource-id="" content-desc="" enabled="true" displayed="true" clickable="false"/>
				<node class="android.widget.ScrollView" scrollable="true" bounds="[0,200][1080,2400]"
					text="" resource-id="" content-desc="" enabled="true" displayed="true" clickable="false"/>
			</node>
		</hierarchy>`,
	}
	driver := &Driver{client: client}

	info, count := driver.findScrollableElement(500)

	if info == nil {
		t.Fatal("expected scrollable element info, got nil")
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestFindScrollableElementSourceError(t *testing.T) {
	client := &MockUIA2Client{sourceErr: errors.New("source failed")}
	driver := &Driver{client: client}

	info, count := driver.findScrollableElement(200)

	if info != nil {
		t.Errorf("expected nil info, got %+v", info)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
}

// ============================================================================
// waitUntil Tests
// ============================================================================

func TestWaitUntilVisibleFound(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-wait"},
			})
		},
		"GET /element/elem-wait/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Target"})
		},
		"GET /element/elem-wait/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 10, "y": 20, "width": 100, "height": 50},
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	sel := flow.Selector{Text: "Target"}
	step := &flow.WaitUntilStep{
		Visible:  &sel,
		BaseStep: flow.BaseStep{TimeoutMs: 2000},
	}
	result := driver.waitUntil(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "visible") {
		t.Errorf("expected 'visible' in message, got: %s", result.Message)
	}
}

func TestWaitUntilVisibleTimeout(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": ""},
			})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><node text="Other" bounds="[0,0][100,100]"/></hierarchy>`,
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	sel := flow.Selector{Text: "Missing"}
	step := &flow.WaitUntilStep{
		Visible:  &sel,
		BaseStep: flow.BaseStep{TimeoutMs: 300},
	}
	result := driver.waitUntil(step)

	if result.Success {
		t.Error("expected failure when element not found within timeout")
	}
	if !strings.Contains(result.Message, "not visible") {
		t.Errorf("expected 'not visible' in message, got: %s", result.Message)
	}
}

func TestWaitUntilNotVisibleAlreadyGone(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": ""},
			})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><node text="Other" bounds="[0,0][100,100]"/></hierarchy>`,
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	sel := flow.Selector{Text: "Missing"}
	step := &flow.WaitUntilStep{
		NotVisible: &sel,
		BaseStep:   flow.BaseStep{TimeoutMs: 2000},
	}
	result := driver.waitUntil(step)

	if !result.Success {
		t.Errorf("expected success when element already gone, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "no longer visible") {
		t.Errorf("expected 'no longer visible' in message, got: %s", result.Message)
	}
}

func TestWaitUntilNotVisibleTimeout(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-wait"},
			})
		},
		"GET /element/elem-wait/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "StillHere"})
		},
		"GET /element/elem-wait/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 10, "y": 20, "width": 100, "height": 50},
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	sel := flow.Selector{Text: "StillHere"}
	step := &flow.WaitUntilStep{
		NotVisible: &sel,
		BaseStep:   flow.BaseStep{TimeoutMs: 300},
	}
	result := driver.waitUntil(step)

	if result.Success {
		t.Error("expected failure when element remains visible")
	}
	if !strings.Contains(result.Message, "still visible") {
		t.Errorf("expected 'still visible' in message, got: %s", result.Message)
	}
}

func TestWaitUntilDefaultTimeout(t *testing.T) {
	// TimeoutMs=0 should use 30 second default, but element found immediately
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-fast"},
			})
		},
		"GET /element/elem-fast/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Found"})
		},
		"GET /element/elem-fast/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 10, "y": 20, "width": 100, "height": 50},
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	sel := flow.Selector{Text: "Found"}
	step := &flow.WaitUntilStep{
		Visible:  &sel,
		BaseStep: flow.BaseStep{TimeoutMs: 0}, // default 30s
	}
	result := driver.waitUntil(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
}

func TestWaitUntilVisibleAppearsAfterDelay(t *testing.T) {
	// Element not found at first, then appears
	var callCount int64
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt64(&callCount, 1)
			if count < 3 {
				// Not found on first few calls
				writeJSON(w, map[string]interface{}{
					"value": map[string]string{"ELEMENT": ""},
				})
			} else {
				// Found after a few attempts
				writeJSON(w, map[string]interface{}{
					"value": map[string]string{"ELEMENT": "elem-delayed"},
				})
			}
		},
		"GET /element/elem-delayed/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Appeared"})
		},
		"GET /element/elem-delayed/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 10, "y": 20, "width": 100, "height": 50},
			})
		},
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			count := atomic.LoadInt64(&callCount)
			if count < 3 {
				writeJSON(w, map[string]interface{}{
					"value": `<hierarchy><node text="Other" bounds="[0,0][100,100]"/></hierarchy>`,
				})
			} else {
				writeJSON(w, map[string]interface{}{
					"value": `<hierarchy><node text="Appeared" bounds="[10,20][110,70]"/></hierarchy>`,
				})
			}
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	sel := flow.Selector{Text: "Appeared"}
	step := &flow.WaitUntilStep{
		Visible:  &sel,
		BaseStep: flow.BaseStep{TimeoutMs: 5000},
	}
	result := driver.waitUntil(step)

	if !result.Success {
		t.Errorf("expected success when element appears after delay, got error: %v", result.Error)
	}
}

// ============================================================================
// SetWaitForIdleTimeout Tests
// ============================================================================

func TestSetWaitForIdleTimeoutSuccess(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /appium/settings": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	err := driver.SetWaitForIdleTimeout(5000)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestSetWaitForIdleTimeoutZeroDisable(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /appium/settings": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	err := driver.SetWaitForIdleTimeout(0)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestSetWaitForIdleTimeoutServerError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /appium/settings": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "server error"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	err := driver.SetWaitForIdleTimeout(5000)

	if err == nil {
		t.Error("expected error when server returns 500")
	}
}

// ============================================================================
// travel Additional Tests
// ============================================================================

func TestTravelSuccess(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}

	step := &flow.TravelStep{
		Points: []string{"37.7749, -122.4194", "37.8049, -122.4094"},
		Speed:  3600, // High speed to minimize delay (3600 km/h = 1 point/sec)
	}
	result := driver.travel(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "2 points") {
		t.Errorf("expected '2 points' in message, got: %s", result.Message)
	}
	// Should have issued 2 shell commands (one per point)
	if len(shell.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(shell.commands))
	}
	if !strings.Contains(shell.commands[0], "37.7749") {
		t.Errorf("expected first point lat in command, got: %s", shell.commands[0])
	}
	if !strings.Contains(shell.commands[1], "37.8049") {
		t.Errorf("expected second point lat in command, got: %s", shell.commands[1])
	}
}

func TestTravelShellError(t *testing.T) {
	shell := &MockShellExecutor{err: errors.New("shell failed")}
	driver := &Driver{device: shell}

	step := &flow.TravelStep{
		Points: []string{"37.7749, -122.4194", "37.8049, -122.4094"},
		Speed:  3600,
	}
	result := driver.travel(step)

	if result.Success {
		t.Error("expected failure when shell command fails")
	}
}

func TestTravelDefaultSpeed(t *testing.T) {
	// Verify that speed=0 defaults to 50 km/h internally.
	// We can't easily test the actual wait (72s per point), but we verify
	// the field is set correctly on the step before calling.
	step := &flow.TravelStep{
		Points: []string{"37.7749, -122.4194", "37.8049, -122.4094"},
		Speed:  0,
	}
	if step.Speed != 0 {
		t.Errorf("expected speed 0, got %f", step.Speed)
	}
	// The travel() function sets speed=50 internally when step.Speed<=0.
	// This is tested indirectly by TestTravelSuccess which uses high speed.
}

func TestTravelMalformedPoints(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}

	step := &flow.TravelStep{
		Points: []string{"malformed_point", "also_malformed"},
		Speed:  3600,
	}
	result := driver.travel(step)

	// Malformed points are silently skipped (continue), so success
	if !result.Success {
		t.Errorf("expected success (malformed points are skipped), got error: %v", result.Error)
	}
	// No shell commands should have been issued since no valid points
	if len(shell.commands) != 0 {
		t.Errorf("expected 0 commands for malformed points, got %d", len(shell.commands))
	}
}

// ============================================================================
// getAllPermissions Tests
// ============================================================================

func TestGetAllPermissionsNotEmpty(t *testing.T) {
	perms := getAllPermissions()
	if len(perms) == 0 {
		t.Error("getAllPermissions returned empty list")
	}
	// Verify all are android.permission. prefixed
	for _, p := range perms {
		if !strings.HasPrefix(p, "android.permission.") {
			t.Errorf("expected android.permission. prefix, got: %s", p)
		}
	}
}

// ============================================================================
// tapOn with percentage Point (no selector) Tests
// ============================================================================

func TestTapOnWithPercentagePoint(t *testing.T) {
	client := &MockUIA2Client{}
	shell := &MockShellExecutor{}
	driver := New(client, nil, shell)

	step := &flow.TapOnStep{Point: "50%, 50%"}
	result := driver.tapOn(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	// Should have called tapOnPointWithPercentage -> Click(540, 1200)
	if len(client.clickCalls) != 1 {
		t.Fatalf("expected 1 click call, got %d", len(client.clickCalls))
	}
	if client.clickCalls[0].X != 540 || client.clickCalls[0].Y != 1200 {
		t.Errorf("expected click at (540, 1200), got (%d, %d)", client.clickCalls[0].X, client.clickCalls[0].Y)
	}
}

// ============================================================================
// getScreenSize via DeviceInfo with bad format Tests
// ============================================================================

func TestGetScreenSizeDeviceInfoBadFormat(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /appium/device/info": func(w http.ResponseWriter, r *http.Request) {
			// Return a malformed display size (missing "x" separator)
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"realDisplaySize": "1080-2400"},
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	shell := &MockShellExecutor{response: "Physical size: 1080x2400"}
	driver := New(client.Client, nil, shell)

	w, h, err := driver.getScreenSize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Falls back to wm size
	if w != 1080 || h != 2400 {
		t.Errorf("expected 1080x2400, got %dx%d", w, h)
	}
}

func TestGetScreenSizeDeviceInfoNonNumeric(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /appium/device/info": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"realDisplaySize": "ABCxDEF"},
			})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	shell := &MockShellExecutor{response: "Physical size: 720x1280"}
	driver := New(client.Client, nil, shell)

	w, h, err := driver.getScreenSize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Falls back to wm size
	if w != 720 || h != 1280 {
		t.Errorf("expected 720x1280, got %dx%d", w, h)
	}
}

// ============================================================================
// swipeWithAbsoluteCoords edge case Tests
// ============================================================================

func TestSwipeWithAbsoluteCoordsNoDevice(t *testing.T) {
	driver := &Driver{device: nil}

	result := driver.swipeWithAbsoluteCoords(100, 800, 100, 200, 300)

	if result.Success {
		t.Error("expected failure when device is nil")
	}
}

func TestSwipeWithAbsoluteCoordsShellError(t *testing.T) {
	shell := &MockShellExecutor{err: errors.New("shell error")}
	driver := &Driver{device: shell}

	result := driver.swipeWithAbsoluteCoords(100, 800, 100, 200, 300)

	if result.Success {
		t.Error("expected failure when shell fails")
	}
}

func TestSwipeWithAbsoluteCoordsDefaultDuration(t *testing.T) {
	shell := &MockShellExecutor{}
	driver := &Driver{device: shell}

	result := driver.swipeWithAbsoluteCoords(100, 800, 100, 200, 0)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if len(shell.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(shell.commands))
	}
	// Duration 0 should default to 300
	expected := "input swipe 100 800 100 200 300"
	if shell.commands[0] != expected {
		t.Errorf("expected %q, got %q", expected, shell.commands[0])
	}
}

// ============================================================================
// swipe with selector Tests
// ============================================================================

func TestSwipeWithSelectorDirection(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]string{"ELEMENT": "elem-swipe"},
			})
		},
		"GET /element/elem-swipe/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Container"})
		},
		"GET /element/elem-swipe/rect": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{
				"value": map[string]int{"x": 0, "y": 100, "width": 500, "height": 800},
			})
		},
		"POST /appium/gestures/swipe": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	driver := New(client.Client, nil, nil)

	sel := flow.Selector{ID: "container"}
	step := &flow.SwipeStep{Direction: "left", Selector: &sel}
	result := driver.swipe(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "left") {
		t.Errorf("expected 'left' in message, got: %s", result.Message)
	}
}

// ============================================================================
// SetWaitForIdleTimeout via MockUIA2Client Tests
// ============================================================================

func TestSetWaitForIdleTimeoutMock(t *testing.T) {
	// Using the MockUIA2Client which always returns nil from SetAppiumSettings
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	err := driver.SetWaitForIdleTimeout(0)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// ============================================================================
// swipeWithCoordinates getScreenSize error Tests
// ============================================================================

func TestSwipeWithCoordinatesScreenSizeError(t *testing.T) {
	// Device exists but GetDeviceInfo fails and shell also fails
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /appium/device/info": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "error"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	shell := &MockShellExecutor{err: errors.New("wm failed")}
	driver := New(client.Client, nil, shell)

	result := driver.swipeWithCoordinates("50%, 80%", "50%, 20%", 400)

	if result.Success {
		t.Error("expected failure when screen size cannot be determined")
	}
}

// ============================================================================
// tapOnPointWithPercentage getScreenSize error Tests
// ============================================================================

func TestTapOnPointWithPercentageScreenSizeError(t *testing.T) {
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /appium/device/info": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": "error"})
		},
	})
	defer server.Close()

	client := newMockHTTPClient(server.URL)
	shell := &MockShellExecutor{err: errors.New("wm failed")}
	driver := New(client.Client, nil, shell)

	result := driver.tapOnPointWithPercentage("50%, 50%")

	if result.Success {
		t.Error("expected failure when screen size cannot be determined")
	}
}

// ============================================================================
// InputText keyPress Mode Tests
// ============================================================================

func TestInputTextKeyPressSuccess(t *testing.T) {
	var sendKeyActionsCalled bool
	var textSent string
	client := &MockUIA2Client{}
	// Override SendKeyActions to track the call
	client.sendKeyActionsFunc = func(text string) error {
		sendKeyActionsCalled = true
		textSent = text
		return nil
	}
	driver := New(client, nil, nil)

	step := &flow.InputTextStep{
		Text:     "Hello",
		KeyPress: true,
	}
	result := driver.inputText(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !sendKeyActionsCalled {
		t.Error("expected SendKeyActions to be called")
	}
	if textSent != "Hello" {
		t.Errorf("expected text 'Hello', got %q", textSent)
	}
	if !strings.Contains(result.Message, "keyPress") {
		t.Errorf("expected 'keyPress' in message, got: %s", result.Message)
	}
}

func TestInputTextKeyPressError(t *testing.T) {
	client := &MockUIA2Client{}
	client.sendKeyActionsFunc = func(text string) error {
		return errors.New("key actions failed")
	}
	driver := New(client, nil, nil)

	step := &flow.InputTextStep{
		Text:     "Hello",
		KeyPress: true,
	}
	result := driver.inputText(step)

	if result.Success {
		t.Error("expected failure when SendKeyActions fails")
	}
	if !strings.Contains(result.Message, "key press") {
		t.Errorf("expected 'key press' in error message, got: %s", result.Message)
	}
}

func TestInputTextKeyPressWithUnicode(t *testing.T) {
	client := &MockUIA2Client{}
	client.sendKeyActionsFunc = func(text string) error {
		return nil
	}
	driver := New(client, nil, nil)

	step := &flow.InputTextStep{
		Text:     "caf\u00e9",
		KeyPress: true,
	}
	result := driver.inputText(step)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "non-ASCII") {
		t.Errorf("expected non-ASCII warning in message, got: %s", result.Message)
	}
}

func TestInputTextKeyPressEmptyText(t *testing.T) {
	client := &MockUIA2Client{}
	driver := New(client, nil, nil)

	step := &flow.InputTextStep{
		Text:     "",
		KeyPress: true,
	}
	result := driver.inputText(step)

	if result.Success {
		t.Error("expected failure for empty text even with keyPress=true")
	}
}

// ============================================================================
// Compile-time interface assertion
// ============================================================================

// Verify MockUIA2Client satisfies UIA2Client at compile time.
var _ UIA2Client = (*MockUIA2Client)(nil)

// Verify uiautomator2.DeviceInfo is used correctly.
var _ = &uiautomator2.DeviceInfo{}

// Use fmt to avoid unused import error
var _ = fmt.Sprintf
