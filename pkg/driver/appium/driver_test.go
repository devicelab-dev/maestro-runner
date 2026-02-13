package appium

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// mockAppiumServerForDriver creates a comprehensive mock Appium server for driver testing
func mockAppiumServerForDriver() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Session creation
		if strings.HasSuffix(path, "/session") && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId": "test-session",
					"capabilities": map[string]interface{}{
						"platformName": "Android",
						"deviceScreenSize": map[string]interface{}{
							"width":  1080,
							"height": 2340,
						},
					},
				},
			})
			return
		}

		// Page source (Android format)
		if strings.HasSuffix(path, "/source") {
			writeJSON(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2340]">
    <android.widget.Button resource-id="com.app:id/loginBtn" text="Login" clickable="true" enabled="true" bounds="[100,200][400,280]"/>
    <android.widget.EditText resource-id="com.app:id/emailField" text="" content-desc="Email" enabled="true" bounds="[100,300][900,380]"/>
    <android.widget.TextView text="Welcome" enabled="true" bounds="[100,400][500,450]"/>
    <android.widget.Button text="Disabled" enabled="false" bounds="[100,500][300,550]"/>
  </android.widget.FrameLayout>
</hierarchy>`,
			})
			return
		}

		// Window/rect
		if strings.Contains(path, "/window/rect") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 2340.0, "x": 0.0, "y": 0.0},
			})
			return
		}

		// Screenshot
		if strings.Contains(path, "/screenshot") {
			writeJSON(w, map[string]interface{}{
				"value": base64.StdEncoding.EncodeToString([]byte("fake-png-data")),
			})
			return
		}

		// Orientation
		if strings.Contains(path, "/orientation") {
			if r.Method == "GET" {
				writeJSON(w, map[string]interface{}{"value": "PORTRAIT"})
			} else {
				writeJSON(w, map[string]interface{}{"value": nil})
			}
			return
		}

		// Element finding
		if strings.HasSuffix(path, "/element") && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"element-6066-11e4-a52e-4f735466cecf": "elem-123",
				},
			})
			return
		}

		// Active element
		if strings.Contains(path, "/element/active") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"element-6066-11e4-a52e-4f735466cecf": "active-elem",
				},
			})
			return
		}

		// Element rect
		if strings.Contains(path, "/rect") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"x": 100.0, "y": 200.0, "width": 300.0, "height": 80.0},
			})
			return
		}

		// Element text
		if strings.Contains(path, "/text") {
			writeJSON(w, map[string]interface{}{"value": "Login"})
			return
		}

		// Element displayed
		if strings.Contains(path, "/displayed") {
			writeJSON(w, map[string]interface{}{"value": true})
			return
		}

		// Element enabled
		if strings.Contains(path, "/enabled") {
			writeJSON(w, map[string]interface{}{"value": true})
			return
		}

		// Clipboard
		if strings.Contains(path, "/appium/device/get_clipboard") {
			writeJSON(w, map[string]interface{}{
				"value": base64.StdEncoding.EncodeToString([]byte("clipboard content")),
			})
			return
		}

		// Default success response
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
}

// createTestAppiumDriver creates a driver with mock server for testing
func createTestAppiumDriver(server *httptest.Server) *Driver {
	client := &Client{
		serverURL: server.URL,
		client:    http.DefaultClient,
		sessionID: "test-session",
		platform:  "android",
		screenW:   1080,
		screenH:   2340,
	}
	return &Driver{
		client:   client,
		platform: "android",
		appID:    "com.example.app",
	}
}

// TestNewDriver tests driver creation via NewDriver function
func TestNewDriverAppium(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]interface{}{
			"value": map[string]interface{}{
				"sessionId": "test-session",
				"capabilities": map[string]interface{}{
					"platformName": "Android",
				},
			},
		})
	}))
	defer server.Close()

	caps := map[string]interface{}{
		"appium:appPackage": "com.example.app",
	}

	driver, err := NewDriver(server.URL, caps)
	if err != nil {
		t.Fatalf("NewDriver failed: %v", err)
	}

	if driver == nil {
		t.Fatal("Expected driver to be created")
	}
}

// TestExecuteTapOn tests tap on element
func TestExecuteAppiumTapOn(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteDoubleTapOn tests double tap
func TestExecuteAppiumDoubleTapOn(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.DoubleTapOnStep{
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteLongPressOn tests long press
func TestExecuteAppiumLongPressOn(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.LongPressOnStep{
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteTapOnPoint tests tap on point
func TestExecuteAppiumTapOnPoint(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.TapOnPointStep{
		X: 100,
		Y: 200,
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteTapOnPointWithPercentage tests tap with percentage
func TestExecuteAppiumTapOnPointWithPercentage(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.TapOnPointStep{
		Point: "50%, 50%",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteSwipe tests swipe
func TestExecuteAppiumSwipe(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	directions := []string{"up", "down", "left", "right"}
	for _, dir := range directions {
		step := &flow.SwipeStep{Direction: dir}
		result := driver.Execute(step)

		if !result.Success {
			t.Errorf("Swipe %s failed: %v", dir, result.Error)
		}
	}
}

// TestExecuteSwipeWithCoordinates tests swipe with coordinates
func TestExecuteAppiumSwipeWithCoordinates(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SwipeStep{
		Start:    "50%, 80%",
		End:      "50%, 20%",
		Duration: 300,
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteSwipeWithPixelCoords tests swipe with pixel coordinates
func TestExecuteAppiumSwipeWithPixelCoords(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SwipeStep{
		StartX: 500,
		StartY: 1500,
		EndX:   500,
		EndY:   500,
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteScroll tests scroll
func TestExecuteAppiumScroll(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.ScrollStep{Direction: "down"}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteScrollUntilVisible tests scroll until visible
func TestExecuteAppiumScrollUntilVisible(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		BaseStep:  flow.BaseStep{TimeoutMs: 5000},
		Element:   flow.Selector{Text: "Login"},
		Direction: "down",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteInputText tests text input
func TestExecuteAppiumInputText(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.InputTextStep{
		Text: "hello@example.com",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteEraseText tests text erasing
func TestExecuteAppiumEraseText(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.EraseTextStep{
		Characters: 10,
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteAssertVisible tests visibility assertion
func TestExecuteAppiumAssertVisible(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteAssertNotVisible tests not visible assertion
func TestExecuteAppiumAssertNotVisible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/source") {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><android.widget.FrameLayout/></hierarchy>`,
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Selector: flow.Selector{Text: "NonExistent"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success for not visible: %v", result.Error)
	}
}

// TestExecuteBack tests back button
func TestExecuteAppiumBack(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.BackStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteHideKeyboard tests keyboard hiding
func TestExecuteAppiumHideKeyboard(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.HideKeyboardStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteLaunchApp tests app launching
func TestExecuteAppiumLaunchApp(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.LaunchAppStep{
		AppID: "com.example.app",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteLaunchAppDefault tests launching with default app
func TestExecuteAppiumLaunchAppDefault(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.LaunchAppStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success with default app, got error: %v", result.Error)
	}
}

// TestExecuteStopApp tests app stopping
func TestExecuteAppiumStopApp(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.StopAppStep{
		AppID: "com.example.app",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteClearState tests state clearing
func TestExecuteAppiumClearState(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.ClearStateStep{
		AppID: "com.example.app",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteSetLocation tests location setting
func TestExecuteAppiumSetLocation(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SetLocationStep{
		Latitude:  "37.7749",
		Longitude: "-122.4194",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteSetLocationInvalid tests invalid location
func TestExecuteAppiumSetLocationInvalid(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SetLocationStep{
		Latitude:  "invalid",
		Longitude: "-122.4194",
	}
	result := driver.Execute(step)

	if result.Success {
		t.Error("Expected error for invalid latitude")
	}

	step = &flow.SetLocationStep{
		Latitude:  "37.7749",
		Longitude: "invalid",
	}
	result = driver.Execute(step)

	if result.Success {
		t.Error("Expected error for invalid longitude")
	}
}

// TestExecuteSetOrientation tests orientation setting
func TestExecuteAppiumSetOrientation(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SetOrientationStep{
		Orientation: "landscape",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteOpenLink tests link opening
func TestExecuteAppiumOpenLink(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.OpenLinkStep{
		Link: "https://example.com",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteCopyTextFrom tests text copying
func TestExecuteAppiumCopyTextFrom(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.CopyTextFromStep{
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecutePasteText tests text pasting
func TestExecuteAppiumPasteText(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.PasteTextStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecutePressKey tests key pressing
func TestExecuteAppiumPressKey(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	keys := []string{"back", "home", "enter", "backspace", "delete", "tab", "volume_up", "volume_down", "power"}
	for _, key := range keys {
		step := &flow.PressKeyStep{Key: key}
		result := driver.Execute(step)

		if !result.Success {
			t.Errorf("PressKey %s failed: %v", key, result.Error)
		}
	}
}

// TestExecutePressKeyUnknown tests unknown key
func TestExecuteAppiumPressKeyUnknown(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.PressKeyStep{Key: "unknown_key"}
	result := driver.Execute(step)

	if result.Success {
		t.Error("Expected error for unknown key")
	}
}

// TestPressKeyIOSKeyboard verifies iOS uses W3C key actions for keyboard keys
func TestPressKeyIOSKeyboard(t *testing.T) {
	var lastPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		lastPath = r.URL.Path
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	driver := createTestAppiumDriver(server)
	driver.platform = "ios"

	keys := []string{"enter", "return", "tab", "backspace", "delete", "space"}
	for _, key := range keys {
		step := &flow.PressKeyStep{Key: key}
		result := driver.Execute(step)
		if !result.Success {
			t.Errorf("iOS pressKey %s failed: %v", key, result.Error)
		}
		if !strings.Contains(lastPath, "/actions") {
			t.Errorf("iOS pressKey %s: expected W3C actions endpoint, got %s", key, lastPath)
		}
	}
}

// TestPressKeyIOSPhysicalButtons verifies iOS uses mobile:pressButton for physical buttons
func TestPressKeyIOSPhysicalButtons(t *testing.T) {
	var lastPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		lastPath = r.URL.Path
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	driver := createTestAppiumDriver(server)
	driver.platform = "ios"

	keys := []string{"home", "volume_up", "volume_down"}
	for _, key := range keys {
		step := &flow.PressKeyStep{Key: key}
		result := driver.Execute(step)
		if !result.Success {
			t.Errorf("iOS pressKey %s failed: %v", key, result.Error)
		}
		if !strings.Contains(lastPath, "/execute/sync") {
			t.Errorf("iOS pressKey %s: expected mobile:pressButton (execute/sync), got %s", key, lastPath)
		}
	}
}

// TestPressKeyIOSUnknown verifies iOS returns error for unknown keys
func TestPressKeyIOSUnknown(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)
	driver.platform = "ios"

	step := &flow.PressKeyStep{Key: "unknown_key"}
	result := driver.Execute(step)
	if result.Success {
		t.Error("Expected error for unknown key on iOS")
	}
}

// TestPressKeyAndroidStillUsesKeycode verifies Android still uses press_keycode
func TestPressKeyAndroidStillUsesKeycode(t *testing.T) {
	var lastPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		lastPath = r.URL.Path
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	driver := createTestAppiumDriver(server)
	driver.platform = "android"

	step := &flow.PressKeyStep{Key: "enter"}
	result := driver.Execute(step)
	if !result.Success {
		t.Errorf("Android pressKey enter failed: %v", result.Error)
	}
	if !strings.Contains(lastPath, "/press_keycode") {
		t.Errorf("Android pressKey enter: expected press_keycode, got %s", lastPath)
	}
}

// TestScreenshot tests screenshot capture
func TestAppiumScreenshot(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	data, err := driver.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected screenshot data")
	}
}

// TestHierarchy tests hierarchy retrieval
func TestAppiumHierarchy(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	data, err := driver.Hierarchy()
	if err != nil {
		t.Fatalf("Hierarchy failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected hierarchy data")
	}
}

// TestGetState tests state retrieval
func TestAppiumGetState(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	state := driver.GetState()
	if state == nil {
		t.Fatal("Expected state")
	}
}

// TestGetPlatformInfo tests platform info
func TestAppiumGetPlatformInfo(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	info := driver.GetPlatformInfo()
	if info == nil {
		t.Fatal("Expected platform info")
	}
	if info.Platform != "android" {
		t.Errorf("Expected 'android', got '%s'", info.Platform)
	}
}

// TestClose tests driver close
func TestAppiumClose(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	err := driver.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// TestFindElementDirect tests direct element finding
func TestFindElementDirect(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// Test with ID
	info, err := driver.findElementDirect(flow.Selector{ID: "loginBtn"})
	if err != nil {
		t.Errorf("findElementDirect with ID failed: %v", err)
	}
	if info == nil {
		t.Error("Expected element info")
	}

	// Test with text
	info, err = driver.findElementDirect(flow.Selector{Text: "Login"})
	if err != nil {
		t.Errorf("findElementDirect with text failed: %v", err)
	}
	if info == nil {
		t.Error("Expected element info")
	}
}

// TestFindElementByPageSource tests page source element finding
func TestFindElementByPageSource(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	info, err := driver.findElementByPageSource(flow.Selector{Text: "Login"})
	if err != nil {
		t.Errorf("findElementByPageSource failed: %v", err)
	}
	if info == nil {
		t.Error("Expected element info")
	}
}

// TestElementToInfo tests element to info conversion
func TestElementToInfo(t *testing.T) {
	elem := &ParsedElement{
		Text:        "Test Text",
		ContentDesc: "Description",
		ResourceID:  "com.app:id/test",
		ClassName:   "android.widget.Button",
		Bounds:      core.Bounds{X: 10, Y: 20, Width: 100, Height: 50},
		Enabled:     true,
		Displayed:   true,
	}

	info := elementToInfo(elem, "android")
	if info.Text != "Test Text" {
		t.Errorf("Expected 'Test Text', got '%s'", info.Text)
	}
	if info.ID != "com.app:id/test" {
		t.Errorf("Expected resource ID, got '%s'", info.ID)
	}

	// Test iOS
	elem = &ParsedElement{
		Label: "iOS Label",
		Name:  "iOS Name",
		Type:  "XCUIElementTypeButton",
	}
	info = elementToInfo(elem, "ios")
	if info.Text != "iOS Label" {
		t.Errorf("Expected 'iOS Label', got '%s'", info.Text)
	}
	if info.Class != "XCUIElementTypeButton" {
		t.Errorf("Expected type, got '%s'", info.Class)
	}
}

// TestSuccessResult tests success result creation
func TestAppiumSuccessResult(t *testing.T) {
	result := successResult("test", nil)
	if !result.Success {
		t.Error("Expected success")
	}
	if result.Message != "test" {
		t.Error("Message not set")
	}
}

// TestErrorResult tests error result creation
func TestAppiumErrorResult(t *testing.T) {
	result := errorResult(nil, "error msg")
	if result.Success {
		t.Error("Expected failure")
	}
	if result.Message != "error msg" {
		t.Error("Message not set")
	}
}

// TestParsePercentageCoords tests coordinate parsing
func TestAppiumParsePercentageCoords(t *testing.T) {
	x, y, err := parsePercentageCoords("50%, 75%")
	if err != nil {
		t.Fatalf("parsePercentageCoords failed: %v", err)
	}
	if x != 0.5 || y != 0.75 {
		t.Errorf("Expected (0.5, 0.75), got (%.2f, %.2f)", x, y)
	}

	// Invalid format
	_, _, err = parsePercentageCoords("invalid")
	if err == nil {
		t.Error("Expected error for invalid format")
	}
}

// TestGetRelativeFilter tests relative filter extraction
func TestAppiumGetRelativeFilter(t *testing.T) {
	anchor := &flow.Selector{Text: "anchor"}

	tests := []struct {
		sel          flow.Selector
		expectedType filterType
	}{
		{flow.Selector{Below: anchor}, filterBelow},
		{flow.Selector{Above: anchor}, filterAbove},
		{flow.Selector{LeftOf: anchor}, filterLeftOf},
		{flow.Selector{RightOf: anchor}, filterRightOf},
		{flow.Selector{ChildOf: anchor}, filterChildOf},
		{flow.Selector{ContainsChild: anchor}, filterContainsChild},
		{flow.Selector{InsideOf: anchor}, filterInsideOf},
		{flow.Selector{}, filterNone},
	}

	for _, tc := range tests {
		_, ft := getRelativeFilter(tc.sel)
		if ft != tc.expectedType {
			t.Errorf("Expected %v, got %v", tc.expectedType, ft)
		}
	}
}

// TestApplyRelativeFilter tests relative filter application
func TestAppiumApplyRelativeFilter(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 100, Y: 100, Width: 50, Height: 50}}
	candidates := []*ParsedElement{
		{Text: "Below", Bounds: core.Bounds{X: 100, Y: 200, Width: 50, Height: 50}},
		{Text: "Above", Bounds: core.Bounds{X: 100, Y: 20, Width: 50, Height: 30}},
	}

	result := applyRelativeFilter(candidates, anchor, filterBelow)
	if len(result) != 1 || result[0].Text != "Below" {
		t.Error("filterBelow failed")
	}

	result = applyRelativeFilter(candidates, anchor, filterAbove)
	if len(result) != 1 || result[0].Text != "Above" {
		t.Error("filterAbove failed")
	}
}

// =============================================================================
// Additional tests for uncovered functions
// =============================================================================

// mockAppiumServerForRelativeElements creates mock for relative selector testing
func mockAppiumServerForRelativeElements() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			writeJSON(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2340]">
    <android.widget.TextView text="Header" bounds="[100,50][500,100]"/>
    <android.widget.Button text="BelowButton" clickable="true" enabled="true" bounds="[100,150][400,200]"/>
    <android.widget.TextView text="LeftLabel" bounds="[50,300][150,350]"/>
    <android.widget.Button text="RightButton" clickable="true" enabled="true" bounds="[200,300][400,350]"/>
    <android.widget.FrameLayout bounds="[0,400][1080,800]">
      <android.widget.Button text="ChildButton" clickable="true" enabled="true" bounds="[100,500][300,550]"/>
    </android.widget.FrameLayout>
  </android.widget.FrameLayout>
</hierarchy>`,
			})
			return
		}

		if strings.Contains(path, "/window/rect") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 2340.0, "x": 0.0, "y": 0.0},
			})
			return
		}

		writeJSON(w, map[string]interface{}{"value": nil})
	}))
}

// TestFindElementRelativeBelow tests finding element below another
func TestAppiumFindElementRelativeBelow(t *testing.T) {
	server := mockAppiumServerForRelativeElements()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	sel := flow.Selector{
		Text:  "BelowButton",
		Below: &flow.Selector{Text: "Header"},
	}

	info, err := driver.findElementRelative(sel, 2*time.Second)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeAbove tests finding element above another
func TestAppiumFindElementRelativeAbove(t *testing.T) {
	server := mockAppiumServerForRelativeElements()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	sel := flow.Selector{
		Text:  "Header",
		Above: &flow.Selector{Text: "BelowButton"},
	}

	info, err := driver.findElementRelative(sel, 2*time.Second)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeRightOf tests finding element to the right
func TestAppiumFindElementRelativeRightOf(t *testing.T) {
	server := mockAppiumServerForRelativeElements()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	sel := flow.Selector{
		Text:    "RightButton",
		RightOf: &flow.Selector{Text: "LeftLabel"},
	}

	info, err := driver.findElementRelative(sel, 2*time.Second)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeLeftOf tests finding element to the left
func TestAppiumFindElementRelativeLeftOf(t *testing.T) {
	server := mockAppiumServerForRelativeElements()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	sel := flow.Selector{
		Text:   "LeftLabel",
		LeftOf: &flow.Selector{Text: "RightButton"},
	}

	info, err := driver.findElementRelative(sel, 2*time.Second)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeAnchorNotFound tests when anchor element not found
func TestAppiumFindElementRelativeAnchorNotFound(t *testing.T) {
	server := mockAppiumServerForRelativeElements()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	sel := flow.Selector{
		Text:  "BelowButton",
		Below: &flow.Selector{Text: "NonExistentAnchor"},
	}

	_, err := driver.findElementRelative(sel, 500*time.Millisecond)
	if err == nil {
		t.Error("Expected error when anchor not found")
	}
}

// TestFindElementRelativeNoMatch tests when no element matches
func TestAppiumFindElementRelativeNoMatch(t *testing.T) {
	server := mockAppiumServerForRelativeElements()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	sel := flow.Selector{
		Text:  "NonExistent",
		Below: &flow.Selector{Text: "Header"},
	}

	_, err := driver.findElementRelative(sel, 500*time.Millisecond)
	if err == nil {
		t.Error("Expected error when no element matches")
	}
}

// TestFindElementRelativeWithElementsSuccess tests findElementRelativeWithElements
func TestFindElementRelativeWithElementsSuccess(t *testing.T) {
	server := mockAppiumServerForRelativeElements()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// Get elements from page source
	source, _ := driver.client.Source()
	elements, platform, _ := ParsePageSource(source)

	sel := flow.Selector{
		Text:  "BelowButton",
		Below: &flow.Selector{Text: "Header"},
	}

	info, err := driver.findElementRelativeWithElements(sel, elements, platform)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeWithElementsIndex tests index selection
func TestFindElementRelativeWithElementsIndex(t *testing.T) {
	server := mockAppiumServerForRelativeElements()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	source, _ := driver.client.Source()
	elements, platform, _ := ParsePageSource(source)

	sel := flow.Selector{
		Index: "0",
	}

	info, err := driver.findElementRelativeWithElements(sel, elements, platform)
	if err != nil {
		t.Fatalf("Expected success with index, got: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeWithElementsNegativeIndex tests negative index
func TestFindElementRelativeWithElementsNegativeIndex(t *testing.T) {
	server := mockAppiumServerForRelativeElements()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	source, _ := driver.client.Source()
	elements, platform, _ := ParsePageSource(source)

	sel := flow.Selector{
		Index: "-1", // Last element
	}

	info, err := driver.findElementRelativeWithElements(sel, elements, platform)
	if err != nil {
		t.Fatalf("Expected success with negative index, got: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeWithElementsContainsDescendants tests containsDescendants
func TestFindElementRelativeWithElementsContainsDescendants(t *testing.T) {
	server := mockAppiumServerForRelativeElements()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	source, _ := driver.client.Source()
	elements, platform, _ := ParsePageSource(source)

	sel := flow.Selector{
		ContainsDescendants: []*flow.Selector{
			{Text: "ChildButton"},
		},
	}

	info, err := driver.findElementRelativeWithElements(sel, elements, platform)
	if err != nil {
		t.Fatalf("Expected success with containsDescendants, got: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeWithNestedRelative tests nested relative selector
func TestFindElementRelativeWithNestedRelative(t *testing.T) {
	server := mockAppiumServerForRelativeElements()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	source, _ := driver.client.Source()
	elements, platform, _ := ParsePageSource(source)

	// Find element below a relative anchor
	sel := flow.Selector{
		Text: "BelowButton",
		Below: &flow.Selector{
			Text: "Header",
		},
	}

	info, err := driver.findElementRelativeWithElements(sel, elements, platform)
	if err != nil {
		t.Fatalf("Expected success with nested relative, got: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestEraseTextWithActiveElement tests eraseText with active element
func TestAppiumEraseTextWithActiveElement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Active element
		if strings.Contains(path, "/element/active") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"element-6066-11e4-a52e-4f735466cecf": "active-elem",
				},
			})
			return
		}

		// Clear element
		if strings.Contains(path, "/clear") {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}

		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.EraseTextStep{Characters: 10}
	result := driver.eraseText(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestEraseTextWithDeleteKeys tests eraseText with delete keys fallback
func TestAppiumEraseTextWithDeleteKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Active element fails
		if strings.Contains(path, "/element/active") {
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}

		// Key press
		if strings.Contains(path, "/appium/device/press_keycode") {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}

		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.EraseTextStep{Characters: 5}
	result := driver.eraseText(step)

	if !result.Success {
		t.Errorf("Expected success with delete keys fallback, got: %s", result.Message)
	}
}

// TestEraseTextDefaultCharacters tests default character count
func TestAppiumEraseTextDefaultCharacters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.EraseTextStep{} // No characters set - should use default
	result := driver.eraseText(step)

	if !result.Success {
		t.Errorf("Expected success with default characters, got: %s", result.Message)
	}
}

// TestExecuteMobileSuccess tests ExecuteMobile command
func TestExecuteMobileSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/appium/execute_mobile") || strings.Contains(path, "/execute/sync") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"success": true},
			})
			return
		}

		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	result, err := driver.client.ExecuteMobile("mobile: scroll", map[string]interface{}{
		"direction": "down",
	})
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected result")
	}
}

// TestExecuteMobileError tests ExecuteMobile error handling with malformed JSON
func TestExecuteMobileError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return malformed JSON to trigger parse error
		if _, err := w.Write([]byte("not valid json")); err != nil {
			return
		}
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	_, err := driver.client.ExecuteMobile("scroll", nil)
	if err == nil {
		t.Error("Expected error for malformed response")
	}
}

// TestApplyRelativeFilterLeftOf tests leftOf filter
func TestAppiumApplyRelativeFilterLeftOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 200, Y: 100, Width: 50, Height: 50}}
	candidates := []*ParsedElement{
		{Text: "Left", Bounds: core.Bounds{X: 50, Y: 100, Width: 50, Height: 50}},
		{Text: "Right", Bounds: core.Bounds{X: 300, Y: 100, Width: 50, Height: 50}},
	}

	result := applyRelativeFilter(candidates, anchor, filterLeftOf)
	if len(result) != 1 || result[0].Text != "Left" {
		t.Error("filterLeftOf failed")
	}
}

// TestApplyRelativeFilterRightOf tests rightOf filter
func TestAppiumApplyRelativeFilterRightOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 50, Y: 100, Width: 50, Height: 50}}
	candidates := []*ParsedElement{
		{Text: "Left", Bounds: core.Bounds{X: 10, Y: 100, Width: 30, Height: 50}},
		{Text: "Right", Bounds: core.Bounds{X: 200, Y: 100, Width: 50, Height: 50}},
	}

	result := applyRelativeFilter(candidates, anchor, filterRightOf)
	if len(result) != 1 || result[0].Text != "Right" {
		t.Error("filterRightOf failed")
	}
}

// TestApplyRelativeFilterChildOf tests childOf filter
func TestAppiumApplyRelativeFilterChildOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 0, Y: 0, Width: 400, Height: 400}}
	candidates := []*ParsedElement{
		{Text: "Inside", Bounds: core.Bounds{X: 50, Y: 50, Width: 100, Height: 100}},
		{Text: "Outside", Bounds: core.Bounds{X: 500, Y: 500, Width: 100, Height: 100}},
	}

	result := applyRelativeFilter(candidates, anchor, filterChildOf)
	if len(result) != 1 || result[0].Text != "Inside" {
		t.Error("filterChildOf failed")
	}
}

// TestApplyRelativeFilterContainsChild tests containsChild filter
func TestAppiumApplyRelativeFilterContainsChild(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 50, Y: 50, Width: 100, Height: 100}}
	candidates := []*ParsedElement{
		{Text: "Container", Bounds: core.Bounds{X: 0, Y: 0, Width: 400, Height: 400}},
		{Text: "Other", Bounds: core.Bounds{X: 500, Y: 500, Width: 100, Height: 100}},
	}

	result := applyRelativeFilter(candidates, anchor, filterContainsChild)
	if len(result) != 1 || result[0].Text != "Container" {
		t.Error("filterContainsChild failed")
	}
}

// TestApplyRelativeFilterInsideOf tests insideOf filter
func TestAppiumApplyRelativeFilterInsideOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 0, Y: 0, Width: 400, Height: 400}}
	candidates := []*ParsedElement{
		{Text: "Inside", Bounds: core.Bounds{X: 50, Y: 50, Width: 100, Height: 100}},
		{Text: "Outside", Bounds: core.Bounds{X: 500, Y: 500, Width: 100, Height: 100}},
	}

	result := applyRelativeFilter(candidates, anchor, filterInsideOf)
	if len(result) != 1 || result[0].Text != "Inside" {
		t.Error("filterInsideOf failed")
	}
}

// TestApplyRelativeFilterNone tests filterNone (returns all)
func TestAppiumApplyRelativeFilterNone(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 100, Y: 100, Width: 50, Height: 50}}
	candidates := []*ParsedElement{
		{Text: "One", Bounds: core.Bounds{X: 50, Y: 50, Width: 50, Height: 50}},
		{Text: "Two", Bounds: core.Bounds{X: 200, Y: 200, Width: 50, Height: 50}},
	}

	result := applyRelativeFilter(candidates, anchor, filterNone)
	if len(result) != 2 {
		t.Error("filterNone should return all candidates")
	}
}

// TestScrollUntilVisibleSuccess tests scrollUntilVisible
func TestAppiumScrollUntilVisibleSuccess(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			callCount++
			if callCount >= 2 {
				writeJSON(w, map[string]interface{}{
					"value": `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2340]">
    <android.widget.Button text="TargetButton" clickable="true" enabled="true" bounds="[100,500][300,550]"/>
  </android.widget.FrameLayout>
</hierarchy>`,
				})
			} else {
				writeJSON(w, map[string]interface{}{
					"value": `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2340]">
    <android.widget.TextView text="Other" bounds="[100,100][300,150]"/>
  </android.widget.FrameLayout>
</hierarchy>`,
				})
			}
			return
		}

		if strings.Contains(path, "/window/rect") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 2340.0, "x": 0.0, "y": 0.0},
			})
			return
		}

		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "TargetButton"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 10000},
	}

	result := driver.scrollUntilVisible(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestScrollUntilVisibleNotFound tests scrollUntilVisible when element not found
func TestAppiumScrollUntilVisibleNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			writeJSON(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2340]">
    <android.widget.TextView text="Other" bounds="[100,100][300,150]"/>
  </android.widget.FrameLayout>
</hierarchy>`,
			})
			return
		}

		if strings.Contains(path, "/window/rect") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 2340.0, "x": 0.0, "y": 0.0},
			})
			return
		}

		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "NonExistent"},
		Direction: "down",
		BaseStep:  flow.BaseStep{TimeoutMs: 1000},
	}

	result := driver.scrollUntilVisible(step)
	if result.Success {
		t.Error("Expected failure when element not found")
	}
}

// TestFindElementDirectSuccess tests direct element finding
func TestFindElementDirectSuccess(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	info, err := driver.findElementDirect(flow.Selector{ID: "loginBtn"})
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestGetElementInfo tests getElementInfo
func TestAppiumGetElementInfo(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	info, err := driver.getElementInfo("elem-123")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
	if info.Bounds.Width == 0 {
		t.Error("Expected valid bounds")
	}
}

// TestGetElementInfoAndroidAttribute verifies Android uses content-desc attribute
func TestGetElementInfoAndroidAttribute(t *testing.T) {
	var requestedAttr string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		if strings.Contains(path, "/attribute/") {
			requestedAttr = path[strings.LastIndex(path, "/attribute/")+len("/attribute/"):]
			writeJSON(w, map[string]interface{}{"value": "Submit button"})
			return
		}
		if strings.Contains(path, "/rect") {
			writeJSON(w, map[string]interface{}{"value": map[string]interface{}{"x": 10.0, "y": 20.0, "width": 100.0, "height": 40.0}})
			return
		}
		if strings.Contains(path, "/text") {
			writeJSON(w, map[string]interface{}{"value": "Submit"})
			return
		}
		if strings.Contains(path, "/displayed") {
			writeJSON(w, map[string]interface{}{"value": true})
			return
		}
		if strings.Contains(path, "/enabled") {
			writeJSON(w, map[string]interface{}{"value": true})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	driver := createTestAppiumDriver(server)
	driver.platform = "android"

	info, err := driver.getElementInfo("elem-1")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if requestedAttr != "content-desc" {
		t.Errorf("Expected Android to request 'content-desc', got '%s'", requestedAttr)
	}
	if info.AccessibilityLabel != "Submit button" {
		t.Errorf("Expected AccessibilityLabel 'Submit button', got '%s'", info.AccessibilityLabel)
	}
}

// TestGetElementInfoIOSAttribute verifies iOS uses label attribute instead of content-desc
func TestGetElementInfoIOSAttribute(t *testing.T) {
	var requestedAttr string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		if strings.Contains(path, "/attribute/") {
			requestedAttr = path[strings.LastIndex(path, "/attribute/")+len("/attribute/"):]
			writeJSON(w, map[string]interface{}{"value": "Submit button"})
			return
		}
		if strings.Contains(path, "/rect") {
			writeJSON(w, map[string]interface{}{"value": map[string]interface{}{"x": 10.0, "y": 20.0, "width": 100.0, "height": 40.0}})
			return
		}
		if strings.Contains(path, "/text") {
			writeJSON(w, map[string]interface{}{"value": "Submit"})
			return
		}
		if strings.Contains(path, "/displayed") {
			writeJSON(w, map[string]interface{}{"value": true})
			return
		}
		if strings.Contains(path, "/enabled") {
			writeJSON(w, map[string]interface{}{"value": true})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	driver := createTestAppiumDriver(server)
	driver.platform = "ios"

	info, err := driver.getElementInfo("elem-1")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if requestedAttr != "label" {
		t.Errorf("Expected iOS to request 'label', got '%s'", requestedAttr)
	}
	if info.AccessibilityLabel != "Submit button" {
		t.Errorf("Expected AccessibilityLabel 'Submit button', got '%s'", info.AccessibilityLabel)
	}
}

// TestElementToInfo tests elementToInfo conversion
func TestAppiumElementToInfo(t *testing.T) {
	elem := &ParsedElement{
		Text:      "Test",
		Bounds:    core.Bounds{X: 100, Y: 200, Width: 300, Height: 80},
		Enabled:   true,
		Displayed: true,
		Clickable: true,
	}

	info := elementToInfo(elem, "android")
	if info == nil {
		t.Fatal("Expected element info")
	}
	if info.Text != "Test" {
		t.Errorf("Expected text 'Test', got '%s'", info.Text)
	}
	if info.Bounds.X != 100 {
		t.Errorf("Expected bounds.X 100, got %d", info.Bounds.X)
	}
}

// TestStopAppEmptyID tests stopApp with empty app ID
func TestStopAppEmptyID(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)
	driver.appID = "" // Clear the default appID

	step := &flow.StopAppStep{AppID: ""}
	result := driver.stopApp(step)

	// Should use driver's appID as fallback, which is empty - should fail
	if result.Success {
		t.Error("Expected failure for empty app ID")
	}
}

// TestStopAppError tests stopApp when terminate fails
func TestStopAppError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/appium/device/terminate_app") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "terminate failed",
					"message": "Failed to terminate app",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.StopAppStep{AppID: "com.test.app"}
	result := driver.stopApp(step)

	if result.Success {
		t.Error("Expected failure when terminate fails")
	}
}

// TestClearStateEmptyID tests clearState with empty app ID
func TestClearStateEmptyID(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)
	driver.appID = "" // Clear the default appID

	step := &flow.ClearStateStep{AppID: ""}
	result := driver.clearState(step)

	// Should use driver's appID as fallback, which is empty - should fail
	if result.Success {
		t.Error("Expected failure for empty app ID")
	}
}

// TestClearStateError tests clearState when clear fails
func TestClearStateError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// ClearAppData uses /execute/sync with mobile: clearApp
		if strings.Contains(r.URL.Path, "/execute/sync") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "clear failed",
					"message": "Failed to clear app data",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.ClearStateStep{AppID: "com.test.app"}
	result := driver.clearState(step)

	if result.Success {
		t.Error("Expected failure when clear fails")
	}
}

// TestScrollUp tests scroll with up direction
func TestScrollUp(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.ScrollStep{Direction: "up"}
	result := driver.scroll(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestScrollInvalidDirection tests scroll with invalid direction
func TestScrollInvalidDirection(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.ScrollStep{Direction: "invalid"}
	result := driver.scroll(step)

	if result.Success {
		t.Error("Expected failure for invalid direction")
	}
}

// TestBackError tests back when it fails
func TestBackError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Back uses PressKeyCode(4) which posts to /appium/device/press_keycode
		if strings.Contains(r.URL.Path, "/press_keycode") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "back failed",
					"message": "Failed to press back",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.BackStep{}
	result := driver.back(step)

	if result.Success {
		t.Error("Expected failure when back fails")
	}
}

// TestHideKeyboardError tests hideKeyboard - should not fail even on error
func TestHideKeyboardError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/hide_keyboard") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "hide keyboard failed",
					"message": "Keyboard not visible",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.HideKeyboardStep{}
	result := driver.hideKeyboard(step)

	// hideKeyboard should succeed even on error (keyboard may not be visible)
	if !result.Success {
		t.Errorf("Expected success (keyboard may not be visible), got: %s", result.Message)
	}
}

// TestOpenLinkError tests openLink when it fails
func TestOpenLinkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/url") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "open link failed",
					"message": "Failed to open URL",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.OpenLinkStep{Link: "https://example.com"}
	result := driver.openLink(step)

	if result.Success {
		t.Error("Expected failure when openLink fails")
	}
}

// TestPasteTextError tests pasteText when it fails
func TestPasteTextError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/appium/device/get_clipboard") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "clipboard failed",
					"message": "Failed to get clipboard",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.PasteTextStep{}
	result := driver.pasteText(step)

	if result.Success {
		t.Error("Expected failure when pasteText fails")
	}
}

// TestCopyTextFromError tests copyTextFrom when element not found
func TestCopyTextFromError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><android.widget.FrameLayout bounds="[0,0][1080,2400]"/></hierarchy>`,
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"
	client.screenW = 1080
	client.screenH = 2400
	driver := &Driver{
		client:   client,
		platform: "android",
	}

	step := &flow.CopyTextFromStep{
		Selector: flow.Selector{Text: "NonExistent"},
	}
	result := driver.copyTextFrom(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
}

// TestCopyTextFromEmptyText tests copyTextFrom when element has no text
func TestCopyTextFromEmptyText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><android.widget.Button bounds="[0,0][100,50]" resource-id="emptyBtn"/></hierarchy>`,
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"
	client.screenW = 1080
	client.screenH = 2400
	driver := &Driver{
		client:   client,
		platform: "android",
	}

	step := &flow.CopyTextFromStep{
		Selector: flow.Selector{ID: "emptyBtn"},
	}
	result := driver.copyTextFrom(step)

	// copyTextFrom returns error if text is empty
	if result.Success {
		t.Error("Expected failure when element has no text")
	}
}

// TestSetOrientationError tests setOrientation when it fails
func TestSetOrientationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/orientation") && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "orientation failed",
					"message": "Failed to set orientation",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SetOrientationStep{Orientation: "landscape"}
	result := driver.setOrientation(step)

	if result.Success {
		t.Error("Expected failure when setOrientation fails")
	}
}

// TestAssertNotVisibleWhenVisible tests assertNotVisible when element is visible
func TestAssertNotVisibleWhenVisible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><android.widget.Button text="Login" bounds="[0,0][100,50]"/></hierarchy>`,
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"
	client.screenW = 1080
	client.screenH = 2400
	driver := &Driver{
		client:   client,
		platform: "android",
	}

	step := &flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.assertNotVisible(step)

	if result.Success {
		t.Error("Expected failure when element is visible")
	}
}

// TestLaunchAppEmptyID tests launchApp with empty app ID
func TestLaunchAppEmptyID(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)
	driver.appID = "" // Clear the default appID

	step := &flow.LaunchAppStep{AppID: ""}
	result := driver.launchApp(step)

	// Should use driver's appID as fallback, which is empty - should fail
	if result.Success {
		t.Error("Expected failure for empty app ID")
	}
}

// TestLaunchAppError tests launchApp when it fails
func TestLaunchAppError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/appium/device/activate_app") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "activate failed",
					"message": "Failed to launch app",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.LaunchAppStep{AppID: "com.test.app"}
	result := driver.launchApp(step)

	if result.Success {
		t.Error("Expected failure when launchApp fails")
	}
}

// TestFindElementDirect tests findElementDirect with ID
func TestFindElementDirectID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/element") && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "elem-123"},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/rect") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"x": 50.0, "y": 100.0, "width": 200.0, "height": 50.0},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/text") {
			writeJSON(w, map[string]interface{}{"value": "Login"})
			return
		}
		if strings.Contains(r.URL.Path, "/displayed") {
			writeJSON(w, map[string]interface{}{"value": true})
			return
		}
		if strings.Contains(r.URL.Path, "/enabled") {
			writeJSON(w, map[string]interface{}{"value": true})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"
	client.screenW = 1080
	client.screenH = 2400
	driver := &Driver{
		client:   client,
		platform: "android",
	}

	info, err := driver.findElementDirect(flow.Selector{ID: "loginBtn"})
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestNewDriverWithBundleID tests NewDriver extracting bundleId
func TestNewDriverWithBundleID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/session") && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId": "test-session-123",
					"capabilities": map[string]interface{}{
						"platformName": "iOS",
					},
				},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/window/size") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	caps := map[string]interface{}{
		"platformName":      "iOS",
		"appium:bundleId":   "com.example.app",
		"appium:deviceName": "iPhone",
	}

	driver, err := NewDriver(server.URL, caps)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if driver.appID != "com.example.app" {
		t.Errorf("Expected appID 'com.example.app', got '%s'", driver.appID)
	}
}

// TestParsePercentageCoordsErrors tests parsePercentageCoords error cases
func TestParsePercentageCoordsErrors(t *testing.T) {
	// Invalid format
	_, _, err := parsePercentageCoords("invalid")
	if err == nil {
		t.Error("Expected error for invalid format")
	}

	// Invalid x coordinate
	_, _, err = parsePercentageCoords("abc%, 50%")
	if err == nil {
		t.Error("Expected error for invalid x coordinate")
	}

	// Invalid y coordinate
	_, _, err = parsePercentageCoords("50%, xyz%")
	if err == nil {
		t.Error("Expected error for invalid y coordinate")
	}
}

// TestScreenshotError tests Screenshot when it fails
func TestScreenshotError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/screenshot") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "screenshot failed",
					"message": "Failed to take screenshot",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	_, err := driver.Screenshot()
	if err == nil {
		t.Error("Expected error when screenshot fails")
	}
}

// TestScreenshotInvalidResponse tests Screenshot with invalid response
func TestScreenshotInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/screenshot") {
			// Return non-string value
			writeJSON(w, map[string]interface{}{
				"value": 12345,
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	_, err := driver.Screenshot()
	if err == nil {
		t.Error("Expected error for invalid screenshot response")
	}
}

// TestSetLocationError tests setLocation with invalid coordinates
func TestSetLocationErrorLatitude(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SetLocationStep{Latitude: "invalid", Longitude: "0.0"}
	result := driver.setLocation(step)

	if result.Success {
		t.Error("Expected failure for invalid latitude")
	}
}

// TestSetLocationErrorLongitude tests setLocation with invalid longitude
func TestSetLocationErrorLongitude(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SetLocationStep{Latitude: "0.0", Longitude: "invalid"}
	result := driver.setLocation(step)

	if result.Success {
		t.Error("Expected failure for invalid longitude")
	}
}

// TestSwipeAbsoluteCoords tests swipe with absolute coordinates
func TestSwipeAbsoluteCoords(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SwipeStep{
		StartX:   100,
		StartY:   200,
		EndX:     300,
		EndY:     400,
		Duration: 500,
	}
	result := driver.swipe(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestSwipeStartEndCoords tests swipe with start/end percentage strings
func TestSwipeStartEndCoords(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SwipeStep{
		Start: "50%, 80%",
		End:   "50%, 20%",
	}
	result := driver.swipe(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestSwipeStartError tests swipe with invalid start coordinates
func TestSwipeStartError(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SwipeStep{
		Start: "invalid",
		End:   "50%, 20%",
	}
	result := driver.swipe(step)

	if result.Success {
		t.Error("Expected failure for invalid start coordinates")
	}
}

// TestSwipeEndError tests swipe with invalid end coordinates
func TestSwipeEndError(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SwipeStep{
		Start: "50%, 80%",
		End:   "invalid",
	}
	result := driver.swipe(step)

	if result.Success {
		t.Error("Expected failure for invalid end coordinates")
	}
}

// TestSwipeInvalidDirection tests swipe with invalid direction
func TestSwipeInvalidDirection(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SwipeStep{Direction: "invalid"}
	result := driver.swipe(step)

	if result.Success {
		t.Error("Expected failure for invalid direction")
	}
}

// TestInputTextError tests inputText when SendKeys fails
func TestInputTextError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// SendKeys tries /actions first, then /appium/element/active/value as fallback
		if strings.Contains(r.URL.Path, "/actions") || strings.Contains(r.URL.Path, "/active/value") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "input failed",
					"message": "Failed to send keys",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.InputTextStep{Text: "test"}
	result := driver.inputText(step)

	if result.Success {
		t.Error("Expected failure when inputText fails")
	}
}

// TestInputTextIOSUsesMobileKeys verifies iOS uses "mobile: keys" instead of W3C key actions
func TestInputTextIOSUsesMobileKeys(t *testing.T) {
	var lastPath string
	var lastBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		lastPath = r.URL.Path
		if strings.Contains(r.URL.Path, "/execute/sync") {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &lastBody)
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	driver := createTestAppiumDriver(server)
	driver.platform = "ios"

	step := &flow.InputTextStep{Text: "hello"}
	result := driver.inputText(step)

	if !result.Success {
		t.Errorf("iOS inputText failed: %v", result.Error)
	}
	if !strings.Contains(lastPath, "/execute/sync") {
		t.Errorf("iOS inputText should use /execute/sync (mobile: keys), got %s", lastPath)
	}
	if script, ok := lastBody["script"].(string); !ok || script != "mobile: keys" {
		t.Errorf("iOS inputText should call 'mobile: keys', got %v", lastBody["script"])
	}
}

// TestInputTextAndroidStillUsesActions verifies Android still uses W3C key actions for inputText
func TestInputTextAndroidStillUsesActions(t *testing.T) {
	var lastPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		lastPath = r.URL.Path
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	driver := createTestAppiumDriver(server)
	// driver.platform is "android" by default

	step := &flow.InputTextStep{Text: "hello"}
	result := driver.inputText(step)

	if !result.Success {
		t.Errorf("Android inputText failed: %v", result.Error)
	}
	if !strings.Contains(lastPath, "/actions") {
		t.Errorf("Android inputText should use /actions, got %s", lastPath)
	}
}
