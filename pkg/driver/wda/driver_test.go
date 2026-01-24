package wda

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// mockWDAServerForDriver creates a comprehensive mock WDA server for driver testing
func mockWDAServerForDriver() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Session endpoints
		if strings.HasSuffix(path, "/session") && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"sessionId": "test-session"},
			})
			return
		}

		// Source endpoint
		if strings.HasSuffix(path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="loginBtn" label="Login" enabled="true" visible="true" x="50" y="100" width="290" height="50"/>
    <XCUIElementTypeTextField type="XCUIElementTypeTextField" name="emailField" label="Email" enabled="true" visible="true" x="50" y="200" width="290" height="44"/>
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="disabledBtn" label="Disabled" enabled="false" visible="true" x="50" y="300" width="100" height="40"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}

		// Window size
		if strings.Contains(path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}

		// Screenshot
		if strings.Contains(path, "/screenshot") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": base64.StdEncoding.EncodeToString([]byte("fake-png-data")),
			})
			return
		}

		// Orientation
		if strings.Contains(path, "/orientation") {
			if r.Method == "GET" {
				json.NewEncoder(w).Encode(map[string]interface{}{"value": "PORTRAIT"})
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
			}
			return
		}

		// Element finding
		if strings.HasSuffix(path, "/element") && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "elem-123"},
			})
			return
		}

		// Element active
		if strings.Contains(path, "/element/active") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "active-elem"},
			})
			return
		}

		// Element rect
		if strings.Contains(path, "/rect") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"x": 50.0, "y": 100.0, "width": 290.0, "height": 50.0},
			})
			return
		}

		// Element text
		if strings.Contains(path, "/text") {
			json.NewEncoder(w).Encode(map[string]interface{}{"value": "Login"})
			return
		}

		// Element displayed
		if strings.Contains(path, "/displayed") {
			json.NewEncoder(w).Encode(map[string]interface{}{"value": true})
			return
		}

		// Default success response for actions
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
}

// createTestDriver creates a driver with mock server for testing
func createTestDriver(server *httptest.Server) *Driver {
	// Extract port from server URL
	url := server.URL
	client := &Client{
		baseURL:    url,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}
	info := &core.PlatformInfo{
		Platform:     "ios",
		ScreenWidth:  390,
		ScreenHeight: 844,
	}
	return NewDriver(client, info, "")
}

// TestNewDriver tests driver creation
func TestNewDriver(t *testing.T) {
	client := &Client{}
	info := &core.PlatformInfo{Platform: "ios"}

	driver := NewDriver(client, info, "")
	if driver == nil {
		t.Fatal("Expected driver to be created")
	}
	if driver.client != client {
		t.Error("Client not set correctly")
	}
	if driver.info != info {
		t.Error("Info not set correctly")
	}
}

// TestSetFindTimeout tests timeout setting
func TestSetFindTimeout(t *testing.T) {
	driver := &Driver{}
	driver.SetFindTimeout(5000)
	if driver.findTimeout != 5000 {
		t.Errorf("Expected 5000, got %d", driver.findTimeout)
	}
}

// TestSetOptionalFindTimeout tests optional timeout setting
func TestSetOptionalFindTimeout(t *testing.T) {
	driver := &Driver{}
	driver.SetOptionalFindTimeout(3000)
	if driver.optionalFindTimeout != 3000 {
		t.Errorf("Expected 3000, got %d", driver.optionalFindTimeout)
	}
}

// TestExecuteTapOn tests tap on element
func TestExecuteTapOn(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteTapOnWithPoint tests tap with percentage coordinates
func TestExecuteTapOnWithPoint(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Point: "50%, 50%",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteDoubleTapOn tests double tap
func TestExecuteDoubleTapOn(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.DoubleTapOnStep{
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteLongPressOn tests long press
func TestExecuteLongPressOn(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.LongPressOnStep{
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteTapOnPoint tests tap on point
func TestExecuteTapOnPoint(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

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
func TestExecuteTapOnPointWithPercentage(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnPointStep{
		Point: "25%, 75%",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteAssertVisible tests visibility assertion
func TestExecuteAssertVisible(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteAssertNotVisible tests not visible assertion
func TestExecuteAssertNotVisible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/source") {
			// Return empty source so element is not found
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<AppiumAUT><XCUIElementTypeApplication/></AppiumAUT>`,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/element") {
			// Element not found
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "no such element",
					"message": "not found",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Selector: flow.Selector{Text: "NonExistent"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success for non-visible element, got error: %v", result.Error)
	}
}

// TestExecuteInputText tests text input
func TestExecuteInputText(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text: "hello@example.com",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteInputTextWithSelector tests input with selector
func TestExecuteInputTextWithSelector(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text:     "test@test.com",
		Selector: flow.Selector{Text: "Email"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteInputTextEmpty tests empty text input
func TestExecuteInputTextEmpty(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text: "",
	}
	result := driver.Execute(step)

	if result.Success {
		t.Error("Expected error for empty text")
	}
}

// TestExecuteEraseText tests text erasing
func TestExecuteEraseText(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.EraseTextStep{
		Characters: 10,
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteHideKeyboard tests keyboard hiding
func TestExecuteHideKeyboard(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.HideKeyboardStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteInputRandom tests random input
func TestExecuteInputRandom(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	testCases := []struct {
		dataType string
	}{
		{"TEXT"},
		{"EMAIL"},
		{"NUMBER"},
		{"PERSON_NAME"},
	}

	for _, tc := range testCases {
		step := &flow.InputRandomStep{
			DataType: tc.dataType,
			Length:   10,
		}
		result := driver.Execute(step)

		if !result.Success {
			t.Errorf("InputRandom %s failed: %v", tc.dataType, result.Error)
		}
	}
}

// TestExecuteScroll tests scroll command
func TestExecuteScroll(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	directions := []string{"up", "down", "left", "right"}
	for _, dir := range directions {
		step := &flow.ScrollStep{Direction: dir}
		result := driver.Execute(step)

		if !result.Success {
			t.Errorf("Scroll %s failed: %v", dir, result.Error)
		}
	}
}

// TestExecuteScrollInvalidDirection tests invalid scroll direction
func TestExecuteScrollInvalidDirection(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollStep{Direction: "diagonal"}
	result := driver.Execute(step)

	if result.Success {
		t.Error("Expected error for invalid direction")
	}
}

// TestExecuteSwipe tests swipe command
func TestExecuteSwipe(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "up"}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteSwipeWithCoordinates tests swipe with percentage coordinates
func TestExecuteSwipeWithCoordinates(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{
		Start:    "50%, 80%",
		End:      "50%, 20%",
		Duration: 500,
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteSwipeWithPixelCoords tests swipe with pixel coordinates
func TestExecuteSwipeWithPixelCoords(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{
		StartX: 100,
		StartY: 500,
		EndX:   100,
		EndY:   200,
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteBack tests back command (should fail on iOS)
func TestExecuteBack(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.BackStep{}
	result := driver.Execute(step)

	// iOS doesn't support back button
	if result.Success {
		t.Error("Expected failure for back on iOS")
	}
}

// TestExecutePressKey tests key press
func TestExecutePressKey(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	keys := []string{"home", "volumeUp", "volumeDown"}
	for _, key := range keys {
		step := &flow.PressKeyStep{Key: key}
		result := driver.Execute(step)

		if !result.Success {
			t.Errorf("PressKey %s failed: %v", key, result.Error)
		}
	}
}

// TestExecutePressKeyUnknown tests unknown key
func TestExecutePressKeyUnknown(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "unknown"}
	result := driver.Execute(step)

	if result.Success {
		t.Error("Expected error for unknown key")
	}
}

// TestExecuteLaunchApp tests app launching
func TestExecuteLaunchApp(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.LaunchAppStep{
		AppID: "com.example.app",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteLaunchAppNoID tests launch without app ID
func TestExecuteLaunchAppNoID(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.LaunchAppStep{}
	result := driver.Execute(step)

	if result.Success {
		t.Error("Expected error for missing app ID")
	}
}

// TestExecuteStopApp tests app stopping
func TestExecuteStopApp(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.StopAppStep{
		AppID: "com.example.app",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteKillApp tests app killing
func TestExecuteKillApp(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.KillAppStep{
		AppID: "com.example.app",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteClearState tests clear state (not fully supported on iOS)
func TestExecuteClearState(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ClearStateStep{
		AppID: "com.example.app",
	}
	result := driver.Execute(step)

	// iOS doesn't fully support clear state
	if result.Success {
		t.Error("Expected failure for clearState on iOS")
	}
}

// TestExecuteCopyTextFrom tests text copying
func TestExecuteCopyTextFrom(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.CopyTextFromStep{
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecutePasteText tests text pasting (not supported on iOS via WDA)
func TestExecutePasteText(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PasteTextStep{}
	result := driver.Execute(step)

	// WDA doesn't support paste
	if result.Success {
		t.Error("Expected failure for pasteText on iOS")
	}
}

// TestExecuteSetOrientation tests orientation setting
func TestExecuteSetOrientation(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	orientations := []string{"portrait", "landscape"}
	for _, orient := range orientations {
		step := &flow.SetOrientationStep{Orientation: orient}
		result := driver.Execute(step)

		if !result.Success {
			t.Errorf("SetOrientation %s failed: %v", orient, result.Error)
		}
	}
}

// TestExecuteOpenLink tests link opening
func TestExecuteOpenLink(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenLinkStep{
		Link: "myapp://page/123",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteOpenLinkEmpty tests empty link
func TestExecuteOpenLinkEmpty(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenLinkStep{}
	result := driver.Execute(step)

	if result.Success {
		t.Error("Expected error for empty link")
	}
}

// TestExecuteOpenBrowser tests browser opening
func TestExecuteOpenBrowser(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenBrowserStep{
		URL: "https://example.com",
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteTakeScreenshot tests screenshot capture
func TestExecuteTakeScreenshot(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TakeScreenshotStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteWaitForAnimationToEnd tests animation wait
func TestExecuteWaitForAnimationToEnd(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.WaitForAnimationToEndStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestDriverScreenshot tests screenshot via driver
func TestDriverScreenshot(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	data, err := driver.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected screenshot data")
	}
}

// TestHierarchy tests hierarchy retrieval
func TestHierarchy(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	data, err := driver.Hierarchy()
	if err != nil {
		t.Fatalf("Hierarchy failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected hierarchy data")
	}
}

// TestGetState tests state retrieval
func TestGetState(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	state := driver.GetState()
	if state == nil {
		t.Fatal("Expected state")
	}

	if state.Orientation != "portrait" {
		t.Errorf("Expected 'portrait', got '%s'", state.Orientation)
	}
}

// TestGetPlatformInfo tests platform info retrieval
func TestGetPlatformInfo(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	info := driver.GetPlatformInfo()
	if info == nil {
		t.Fatal("Expected platform info")
	}

	if info.Platform != "ios" {
		t.Errorf("Expected 'ios', got '%s'", info.Platform)
	}
}

// TestBuildStateFilter tests state filter building
func TestBuildStateFilter(t *testing.T) {
	enabledTrue := true
	enabledFalse := false
	selectedTrue := true
	focusedTrue := true

	tests := []struct {
		sel      flow.Selector
		contains []string
	}{
		{flow.Selector{Enabled: &enabledTrue}, []string{"enabled == true"}},
		{flow.Selector{Enabled: &enabledFalse}, []string{"enabled == false"}},
		{flow.Selector{Selected: &selectedTrue}, []string{"selected == true"}},
		{flow.Selector{Focused: &focusedTrue}, []string{"hasFocus == true"}},
		{flow.Selector{Enabled: &enabledTrue, Selected: &selectedTrue}, []string{"enabled == true", "selected == true"}},
	}

	for _, tc := range tests {
		result := buildStateFilter(tc.sel)
		for _, expected := range tc.contains {
			if !strings.Contains(result, expected) {
				t.Errorf("Expected '%s' in '%s'", expected, result)
			}
		}
	}
}

// TestBuildStateFilterEmpty tests empty state filter
func TestBuildStateFilterEmpty(t *testing.T) {
	result := buildStateFilter(flow.Selector{})
	if result != "" {
		t.Errorf("Expected empty string, got '%s'", result)
	}
}

// TestGetRelativeFilter tests relative filter extraction
func TestGetRelativeFilter(t *testing.T) {
	below := &flow.Selector{Text: "anchor"}
	above := &flow.Selector{Text: "anchor"}
	leftOf := &flow.Selector{Text: "anchor"}
	rightOf := &flow.Selector{Text: "anchor"}
	childOf := &flow.Selector{Text: "anchor"}
	containsChild := &flow.Selector{Text: "anchor"}
	insideOf := &flow.Selector{Text: "anchor"}

	tests := []struct {
		sel          flow.Selector
		expectedType relativeFilterType
	}{
		{flow.Selector{Below: below}, filterBelow},
		{flow.Selector{Above: above}, filterAbove},
		{flow.Selector{LeftOf: leftOf}, filterLeftOf},
		{flow.Selector{RightOf: rightOf}, filterRightOf},
		{flow.Selector{ChildOf: childOf}, filterChildOf},
		{flow.Selector{ContainsChild: containsChild}, filterContainsChild},
		{flow.Selector{InsideOf: insideOf}, filterInsideOf},
		{flow.Selector{Text: "noRelative"}, filterNone},
	}

	for _, tc := range tests {
		_, ft := getRelativeFilter(tc.sel)
		if ft != tc.expectedType {
			t.Errorf("Expected filter type %v, got %v", tc.expectedType, ft)
		}
	}
}

// TestApplyRelativeFilter tests filter application
func TestApplyRelativeFilter(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 100, Y: 100, Width: 50, Height: 50}}
	candidates := []*ParsedElement{
		{Label: "Below", Bounds: core.Bounds{X: 100, Y: 200, Width: 50, Height: 50}},
		{Label: "Above", Bounds: core.Bounds{X: 100, Y: 20, Width: 50, Height: 30}},
	}

	// Test filterBelow
	result := applyRelativeFilter(candidates, anchor, filterBelow)
	if len(result) != 1 || result[0].Label != "Below" {
		t.Error("filterBelow failed")
	}

	// Test filterAbove
	result = applyRelativeFilter(candidates, anchor, filterAbove)
	if len(result) != 1 || result[0].Label != "Above" {
		t.Error("filterAbove failed")
	}

	// Test filterNone (returns all)
	result = applyRelativeFilter(candidates, anchor, filterNone)
	if len(result) != 2 {
		t.Error("filterNone should return all candidates")
	}
}

// TestSelectorDesc tests selector description
func TestSelectorDesc(t *testing.T) {
	tests := []struct {
		sel      flow.Selector
		expected string
	}{
		{flow.Selector{Text: "Login"}, "text='Login'"},
		{flow.Selector{ID: "btnLogin"}, "id='btnLogin'"},
		{flow.Selector{}, "selector"},
	}

	for _, tc := range tests {
		result := selectorDesc(tc.sel)
		if result != tc.expected {
			t.Errorf("Expected '%s', got '%s'", tc.expected, result)
		}
	}
}

// TestParsePercentageCoords tests percentage coordinate parsing
func TestParsePercentageCoords(t *testing.T) {
	tests := []struct {
		input string
		x, y  float64
		err   bool
	}{
		{"50%, 50%", 0.5, 0.5, false},
		{"0%, 100%", 0.0, 1.0, false},
		{"25%,75%", 0.25, 0.75, false},
		{"invalid", 0, 0, true},
		{"50%", 0, 0, true},
		{"abc%, 50%", 0, 0, true},
		{"50%, xyz%", 0, 0, true},
	}

	for _, tc := range tests {
		x, y, err := parsePercentageCoords(tc.input)
		if tc.err {
			if err == nil {
				t.Errorf("Expected error for '%s'", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for '%s': %v", tc.input, err)
			}
			if x != tc.x || y != tc.y {
				t.Errorf("For '%s': expected (%.2f, %.2f), got (%.2f, %.2f)", tc.input, tc.x, tc.y, x, y)
			}
		}
	}
}

// TestRandomHelpers tests random generation helpers
func TestRandomHelpers(t *testing.T) {
	// Test randomString
	s := randomString(10)
	if len(s) != 10 {
		t.Errorf("Expected length 10, got %d", len(s))
	}

	// Test randomEmail
	email := randomEmail()
	if !strings.Contains(email, "@") {
		t.Errorf("Expected email format, got '%s'", email)
	}

	// Test randomNumber
	num := randomNumber(5)
	if len(num) != 5 {
		t.Errorf("Expected length 5, got %d", len(num))
	}
	for _, c := range num {
		if c < '0' || c > '9' {
			t.Errorf("Expected digit, got '%c'", c)
		}
	}

	// Test randomPersonName
	name := randomPersonName()
	if !strings.Contains(name, " ") {
		t.Errorf("Expected first and last name, got '%s'", name)
	}
}

// TestSuccessResult tests success result creation
func TestSuccessResult(t *testing.T) {
	elem := &core.ElementInfo{Text: "Test"}
	result := successResult("test message", elem)

	if !result.Success {
		t.Error("Expected success=true")
	}
	if result.Message != "test message" {
		t.Errorf("Expected 'test message', got '%s'", result.Message)
	}
	if result.Element != elem {
		t.Error("Element not set correctly")
	}
}

// TestErrorResult tests error result creation
func TestErrorResult(t *testing.T) {
	result := errorResult(nil, "error message")

	if result.Success {
		t.Error("Expected success=false")
	}
	if result.Message != "error message" {
		t.Errorf("Expected 'error message', got '%s'", result.Message)
	}
}

// =============================================================================
// Additional tests for uncovered functions
// =============================================================================

// mockWDAServerWithScrollElements creates a mock server that returns different elements on each call
func mockWDAServerWithScrollElements(foundAfterScrolls int) *httptest.Server {
	scrollCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Track swipe/drag calls (used by scroll)
		if strings.Contains(path, "/wda/dragfromtoforduration") && r.Method == "POST" {
			scrollCount++
			json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
			return
		}

		// Source - return target element after enough scrolls
		if strings.HasSuffix(path, "/source") {
			// First source call is before any scroll, subsequent calls happen after scroll
			effectiveScrolls := scrollCount
			if effectiveScrolls >= foundAfterScrolls {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="targetBtn" label="TargetButton" enabled="true" visible="true" x="50" y="400" width="290" height="50"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
				})
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" label="Other" enabled="true" visible="true" x="50" y="100" width="100" height="30"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
				})
			}
			return
		}

		// Window size
		if strings.Contains(path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
}

// TestScrollUntilVisibleElementFound tests scrollUntilVisible when element is found after scrolls
func TestScrollUntilVisibleElementFound(t *testing.T) {
	server := mockWDAServerWithScrollElements(2) // Element found after 2 scrolls
	defer server.Close()
	driver := createTestDriver(server)

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

// TestScrollUntilVisibleElementNotFound tests scrollUntilVisible when element is not found
func TestScrollUntilVisibleElementNotFound(t *testing.T) {
	server := mockWDAServerWithScrollElements(100) // Element never found
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "NonExistent"},
		Direction: "up",
		BaseStep:  flow.BaseStep{TimeoutMs: 1000}, // Short timeout
	}

	result := driver.scrollUntilVisible(step)
	if result.Success {
		t.Error("Expected failure when element not found")
	}
}

// TestScrollUntilVisibleDefaultDirection tests default direction
func TestScrollUntilVisibleDefaultDirection(t *testing.T) {
	server := mockWDAServerWithScrollElements(0)
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element: flow.Selector{Text: "TargetButton"},
		// Direction not set - should default to "down"
	}

	result := driver.scrollUntilVisible(step)
	if !result.Success {
		t.Errorf("Expected success with default direction, got: %s", result.Message)
	}
}

// mockWDAServerForWaitUntil creates a mock for waitUntil testing
func mockWDAServerForWaitUntil(visibleAfterMs int) *httptest.Server {
	startTime := time.Now()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			elapsed := time.Since(startTime).Milliseconds()
			if int(elapsed) >= visibleAfterMs {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="waitTarget" label="WaitTarget" enabled="true" visible="true" x="50" y="200" width="100" height="50"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
				})
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
				})
			}
			return
		}

		if strings.Contains(path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
}

// TestWaitUntilVisibleSuccess tests waitUntil with visible condition that succeeds
func TestWaitUntilVisibleSuccess(t *testing.T) {
	server := mockWDAServerForWaitUntil(50) // Element visible after 50ms
	defer server.Close()
	driver := createTestDriver(server)

	visibleSel := flow.Selector{Text: "WaitTarget"}
	step := &flow.WaitUntilStep{
		Visible:  &visibleSel,
		BaseStep: flow.BaseStep{TimeoutMs: 5000},
	}

	result := driver.waitUntil(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestWaitUntilVisibleTimeout tests waitUntil with visible condition that times out
func TestWaitUntilVisibleTimeout(t *testing.T) {
	server := mockWDAServerForWaitUntil(10000) // Element visible after 10s
	defer server.Close()
	driver := createTestDriver(server)

	visibleSel := flow.Selector{Text: "WaitTarget"}
	step := &flow.WaitUntilStep{
		Visible:  &visibleSel,
		BaseStep: flow.BaseStep{TimeoutMs: 200}, // Short timeout
	}

	result := driver.waitUntil(step)
	if result.Success {
		t.Error("Expected timeout failure")
	}
}

// TestWaitUntilNotVisibleSuccess tests waitUntil with notVisible condition
func TestWaitUntilNotVisibleSuccess(t *testing.T) {
	// Create a server where element disappears immediately
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			// Element not present
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}

		if strings.Contains(path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	notVisibleSel := flow.Selector{Text: "DisappearingElement"}
	step := &flow.WaitUntilStep{
		NotVisible: &notVisibleSel,
		BaseStep:   flow.BaseStep{TimeoutMs: 2000},
	}

	result := driver.waitUntil(step)
	if !result.Success {
		t.Errorf("Expected success for notVisible, got: %s", result.Message)
	}
}

// TestWaitUntilDefaultTimeout tests default timeout
func TestWaitUntilDefaultTimeout(t *testing.T) {
	server := mockWDAServerForWaitUntil(50)
	defer server.Close()
	driver := createTestDriver(server)

	visibleSel := flow.Selector{Text: "WaitTarget"}
	step := &flow.WaitUntilStep{
		Visible: &visibleSel,
		// No TimeoutMs set - should use default
	}

	result := driver.waitUntil(step)
	if !result.Success {
		t.Errorf("Expected success with default timeout, got: %s", result.Message)
	}
}

// mockWDAServerForRelativeElements creates mock for relative selector testing
func mockWDAServerForRelativeElements() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="header" label="Header" enabled="true" visible="true" x="50" y="50" width="290" height="30"/>
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="belowBtn" label="BelowButton" enabled="true" visible="true" x="50" y="100" width="290" height="50"/>
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="leftLabel" label="LeftLabel" enabled="true" visible="true" x="20" y="200" width="60" height="30"/>
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="rightBtn" label="RightButton" enabled="true" visible="true" x="100" y="200" width="100" height="30"/>
    <XCUIElementTypeOther type="XCUIElementTypeOther" name="container" enabled="true" visible="true" x="0" y="300" width="390" height="200">
      <XCUIElementTypeButton type="XCUIElementTypeButton" name="childBtn" label="ChildButton" enabled="true" visible="true" x="50" y="350" width="100" height="50"/>
    </XCUIElementTypeOther>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}

		if strings.Contains(path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
}

// TestFindElementRelativeBelow tests finding element below another
func TestFindElementRelativeBelow(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{
		Text:  "BelowButton",
		Below: &flow.Selector{Text: "Header"},
	}

	info, err := driver.findElementRelative(sel, 2000)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeAbove tests finding element above another
func TestFindElementRelativeAbove(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{
		Text:  "Header",
		Above: &flow.Selector{Text: "BelowButton"},
	}

	info, err := driver.findElementRelative(sel, 2000)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeRightOf tests finding element to the right of another
func TestFindElementRelativeRightOf(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{
		Text:    "RightButton",
		RightOf: &flow.Selector{Text: "LeftLabel"},
	}

	info, err := driver.findElementRelative(sel, 2000)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeLeftOf tests finding element to the left of another
func TestFindElementRelativeLeftOf(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{
		Text:   "LeftLabel",
		LeftOf: &flow.Selector{Text: "RightButton"},
	}

	info, err := driver.findElementRelative(sel, 2000)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeAnchorNotFound tests when anchor element not found
func TestFindElementRelativeAnchorNotFound(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{
		Text:  "BelowButton",
		Below: &flow.Selector{Text: "NonExistentAnchor"},
	}

	_, err := driver.findElementRelative(sel, 500)
	if err == nil {
		t.Error("Expected error when anchor not found")
	}
}

// TestFindElementRelativeNoMatch tests when no element matches relative criteria
func TestFindElementRelativeNoMatch(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{
		Text:  "NonExistent",
		Below: &flow.Selector{Text: "Header"},
	}

	_, err := driver.findElementRelative(sel, 500)
	if err == nil {
		t.Error("Expected error when no element matches")
	}
}

// TestResolveRelativeSelectorWithIndex tests index selection
func TestResolveRelativeSelectorWithIndex(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	// Get page source and parse
	source, _ := driver.client.Source()
	elements, _ := ParsePageSource(source)

	sel := flow.Selector{
		Index: "0",
	}

	info, err := driver.resolveRelativeSelector(sel, elements)
	if err != nil {
		t.Fatalf("Expected success with index, got: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestResolveRelativeSelectorNegativeIndex tests negative index
func TestResolveRelativeSelectorNegativeIndex(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	source, _ := driver.client.Source()
	elements, _ := ParsePageSource(source)

	sel := flow.Selector{
		Index: "-1", // Last element
	}

	info, err := driver.resolveRelativeSelector(sel, elements)
	if err != nil {
		t.Fatalf("Expected success with negative index, got: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestResolveRelativeSelectorContainsDescendants tests containsDescendants
func TestResolveRelativeSelectorContainsDescendants(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	source, _ := driver.client.Source()
	elements, _ := ParsePageSource(source)

	sel := flow.Selector{
		ContainsDescendants: []*flow.Selector{
			{Text: "ChildButton"},
		},
	}

	info, err := driver.resolveRelativeSelector(sel, elements)
	if err != nil {
		t.Fatalf("Expected success with containsDescendants, got: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestEraseTextWithActiveElement tests eraseText with active element
func TestEraseTextWithActiveElement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Active element
		if strings.Contains(path, "/element/active") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "active-elem"},
			})
			return
		}

		// Element clear
		if strings.Contains(path, "/clear") {
			json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.EraseTextStep{Characters: 10}
	result := driver.eraseText(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestEraseTextWithDeleteKeys tests eraseText when clear fails
func TestEraseTextWithDeleteKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Active element fails
		if strings.Contains(path, "/element/active") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"status": 1, "message": "No active element"})
			return
		}

		// Keys endpoint
		if strings.Contains(path, "/keys") {
			json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.EraseTextStep{Characters: 5}
	result := driver.eraseText(step)

	if !result.Success {
		t.Errorf("Expected success with delete keys fallback, got: %s", result.Message)
	}
}

// TestEraseTextDefaultCharacters tests default character count
func TestEraseTextDefaultCharacters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.EraseTextStep{} // No characters set - should use default
	result := driver.eraseText(step)

	if !result.Success {
		t.Errorf("Expected success with default characters, got: %s", result.Message)
	}
}

// TestTapOnWithRetry tests tapOn with element not found initially
func TestTapOnWithRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			callCount++
			if callCount >= 2 {
				// Return element on second call
				json.NewEncoder(w).Encode(map[string]interface{}{
					"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="retryBtn" label="RetryButton" enabled="true" visible="true" x="50" y="100" width="100" height="50"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
				})
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
				})
			}
			return
		}

		if strings.Contains(path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "RetryButton"},
		BaseStep: flow.BaseStep{TimeoutMs: 5000},
	}

	result := driver.tapOn(step)
	if !result.Success {
		t.Errorf("Expected success after retry, got: %s", result.Message)
	}
}

// TestTapOnElementNotFound tests tapOn when element never found
func TestTapOnElementNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}

		if strings.Contains(path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "NonExistent"},
		BaseStep: flow.BaseStep{TimeoutMs: 500},
	}

	result := driver.tapOn(step)
	if result.Success {
		t.Error("Expected failure when element not found")
	}
}

// TestTapOnWithRelativeSelector tests tapOn with relative selector
func TestTapOnWithRelativeSelector(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:  "BelowButton",
			Below: &flow.Selector{Text: "Header"},
		},
		BaseStep: flow.BaseStep{TimeoutMs: 2000},
	}

	result := driver.tapOn(step)
	if !result.Success {
		t.Errorf("Expected success with relative selector, got: %s", result.Message)
	}
}

// TestFindElementQuickSuccess tests quick element finding
func TestFindElementQuickSuccess(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	info, err := driver.findElementQuick(flow.Selector{Text: "Login"}, 1000)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementQuickNotFound tests quick element finding when not found
func TestFindElementQuickNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	_, err := driver.findElementQuick(flow.Selector{Text: "NonExistent"}, 200)
	if err == nil {
		t.Error("Expected error when element not found")
	}
}

// TestFindElementByPageSourceOnce tests single page source search
func TestFindElementByPageSourceOnceSuccess(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	info, err := driver.findElementByPageSourceOnce(flow.Selector{Text: "Login"})
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementByPageSourceOnceNotFound tests single page source search when not found
func TestFindElementByPageSourceOnceNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	_, err := driver.findElementByPageSourceOnce(flow.Selector{Text: "NonExistent"})
	if err == nil {
		t.Error("Expected error when element not found")
	}
}

// TestApplyRelativeFilterLeftOf tests leftOf filter
func TestApplyRelativeFilterLeftOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 200, Y: 100, Width: 50, Height: 50}}
	candidates := []*ParsedElement{
		{Label: "Left", Bounds: core.Bounds{X: 50, Y: 100, Width: 50, Height: 50}},
		{Label: "Right", Bounds: core.Bounds{X: 300, Y: 100, Width: 50, Height: 50}},
	}

	result := applyRelativeFilter(candidates, anchor, filterLeftOf)
	if len(result) != 1 || result[0].Label != "Left" {
		t.Error("filterLeftOf failed")
	}
}

// TestApplyRelativeFilterRightOf tests rightOf filter
func TestApplyRelativeFilterRightOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 50, Y: 100, Width: 50, Height: 50}}
	candidates := []*ParsedElement{
		{Label: "Left", Bounds: core.Bounds{X: 10, Y: 100, Width: 30, Height: 50}},
		{Label: "Right", Bounds: core.Bounds{X: 200, Y: 100, Width: 50, Height: 50}},
	}

	result := applyRelativeFilter(candidates, anchor, filterRightOf)
	if len(result) != 1 || result[0].Label != "Right" {
		t.Error("filterRightOf failed")
	}
}

// TestApplyRelativeFilterChildOf tests childOf filter
func TestApplyRelativeFilterChildOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 0, Y: 0, Width: 400, Height: 400}}
	candidates := []*ParsedElement{
		{Label: "Inside", Bounds: core.Bounds{X: 50, Y: 50, Width: 100, Height: 100}},
		{Label: "Outside", Bounds: core.Bounds{X: 500, Y: 500, Width: 100, Height: 100}},
	}

	result := applyRelativeFilter(candidates, anchor, filterChildOf)
	if len(result) != 1 || result[0].Label != "Inside" {
		t.Error("filterChildOf failed")
	}
}

// TestApplyRelativeFilterContainsChild tests containsChild filter
func TestApplyRelativeFilterContainsChild(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 50, Y: 50, Width: 100, Height: 100}}
	candidates := []*ParsedElement{
		{Label: "Container", Bounds: core.Bounds{X: 0, Y: 0, Width: 400, Height: 400}},
		{Label: "Other", Bounds: core.Bounds{X: 500, Y: 500, Width: 100, Height: 100}},
	}

	result := applyRelativeFilter(candidates, anchor, filterContainsChild)
	if len(result) != 1 || result[0].Label != "Container" {
		t.Error("filterContainsChild failed")
	}
}

// TestApplyRelativeFilterInsideOf tests insideOf filter
func TestApplyRelativeFilterInsideOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 0, Y: 0, Width: 400, Height: 400}}
	candidates := []*ParsedElement{
		{Label: "Inside", Bounds: core.Bounds{X: 50, Y: 50, Width: 100, Height: 100}},
		{Label: "Outside", Bounds: core.Bounds{X: 500, Y: 500, Width: 100, Height: 100}},
	}

	result := applyRelativeFilter(candidates, anchor, filterInsideOf)
	if len(result) != 1 || result[0].Label != "Inside" {
		t.Error("filterInsideOf failed")
	}
}

// TestFindElementWithWDAStrategy tests WDA-native element finding
func TestFindElementWithWDAStrategy(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	// This should try WDA strategy first
	info, err := driver.findElement(flow.Selector{ID: "loginBtn"}, false, 2000)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementWithPageSource tests page source fallback
func TestFindElementWithPageSource(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	// Text matching requires page source
	info, err := driver.findElement(flow.Selector{Text: "Login"}, false, 2000)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestInputTextWithSelector tests inputText when finding element first
func TestInputTextWithSelector(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text:     "test@email.com",
		Selector: flow.Selector{Text: "Email"},
	}

	result := driver.inputText(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestInputTextWithoutSelector tests inputText without selector
func TestInputTextWithoutSelector(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text: "simple text",
	}

	result := driver.inputText(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestInputTextEmptyText tests inputText with empty text
func TestInputTextEmptyText(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text: "",
	}

	result := driver.inputText(step)
	// Empty text should fail - WDA implementation rejects empty text
	if result.Success {
		t.Errorf("Expected failure for empty text, got success")
	}
}

// TestLaunchAppWithSession tests launchApp when session already exists
func TestLaunchAppWithSession(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.LaunchAppStep{
		AppID: "com.test.app",
	}

	result := driver.launchApp(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestLaunchAppNoAppID tests launchApp without app ID
func TestLaunchAppNoAppID(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.LaunchAppStep{} // No AppID

	result := driver.launchApp(step)
	if result.Success {
		t.Error("Expected failure without app ID")
	}
}

// TestStopAppSuccess tests stopApp
func TestStopAppSuccess(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.StopAppStep{
		AppID: "com.test.app",
	}

	result := driver.stopApp(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestStopAppNoAppID tests stopApp without app ID
func TestStopAppNoAppID(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.StopAppStep{} // No AppID

	result := driver.stopApp(step)
	if result.Success {
		t.Error("Expected failure without app ID")
	}
}

// TestKillAppSuccess tests killApp
func TestKillAppSuccess(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.KillAppStep{
		AppID: "com.test.app",
	}

	result := driver.killApp(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestClearStateNoAppID tests clearState without app ID
func TestClearStateNoAppID(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ClearStateStep{} // No AppID

	result := driver.clearState(step)
	if result.Success {
		t.Error("Expected failure without app ID")
	}
}

// TestSwipeWithStartEnd tests swipe with start/end coordinates
func TestSwipeWithStartEnd(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{
		Start:    "50%, 70%",
		End:      "50%, 30%",
		Duration: 500,
	}

	result := driver.swipe(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestSwipeWithAbsoluteCoords tests swipe with absolute coordinates
func TestSwipeWithAbsoluteCoords(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{
		StartX:   100,
		StartY:   500,
		EndX:     100,
		EndY:     200,
		Duration: 300,
	}

	result := driver.swipe(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestSwipeDirectionLeft tests swipe left
func TestSwipeDirectionLeft(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "left"}
	result := driver.swipe(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestSwipeDirectionRight tests swipe right
func TestSwipeDirectionRight(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "right"}
	result := driver.swipe(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestSwipeInvalidDirection tests swipe with invalid direction
func TestSwipeInvalidDirection(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "invalid"}
	result := driver.swipe(step)
	if result.Success {
		t.Error("Expected failure for invalid direction")
	}
}

// TestPressKeyHome tests pressing home key
func TestPressKeyHome(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "home"}
	result := driver.pressKey(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestPressKeyVolumeUp tests pressing volume up
func TestPressKeyVolumeUp(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "volumeUp"}
	result := driver.pressKey(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestPressKeyVolumeDown tests pressing volume down
func TestPressKeyVolumeDown(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "volumeDown"}
	result := driver.pressKey(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestPressKeyUnknown tests pressing unknown key
func TestPressKeyUnknown(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "unknown"}
	result := driver.pressKey(step)
	if result.Success {
		t.Error("Expected failure for unknown key")
	}
}

// TestOpenLinkSuccess tests openLink
func TestOpenLinkSuccess(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenLinkStep{Link: "https://example.com"}
	result := driver.openLink(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestOpenLinkNoLink tests openLink without link
func TestOpenLinkNoLink(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenLinkStep{} // No link
	result := driver.openLink(step)
	if result.Success {
		t.Error("Expected failure without link")
	}
}

// TestOpenBrowserSuccess tests openBrowser
func TestOpenBrowserSuccess(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenBrowserStep{URL: "https://example.com"}
	result := driver.openBrowser(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestOpenBrowserNoURL tests openBrowser without URL
func TestOpenBrowserNoURL(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenBrowserStep{} // No URL
	result := driver.openBrowser(step)
	if result.Success {
		t.Error("Expected failure without URL")
	}
}

// TestBackCommand tests back command (iOS doesn't support)
func TestBackCommand(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.BackStep{}
	result := driver.back(step)
	if result.Success {
		t.Error("Expected failure - back not supported on iOS")
	}
}

// TestCopyTextFromSuccess tests copyTextFrom
func TestCopyTextFromSuccess(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.CopyTextFromStep{
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.copyTextFrom(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestSetOrientationPortrait tests setting portrait orientation
func TestSetOrientationPortrait(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SetOrientationStep{Orientation: "portrait"}
	result := driver.setOrientation(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestSetOrientationLandscape tests setting landscape orientation
func TestSetOrientationLandscape(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SetOrientationStep{Orientation: "landscape"}
	result := driver.setOrientation(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestTakeScreenshotSuccess tests takeScreenshot
func TestTakeScreenshotSuccess(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TakeScreenshotStep{Path: "/tmp/test.png"}
	result := driver.takeScreenshot(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestWaitForAnimationToEnd tests waitForAnimationToEnd
func TestWaitForAnimationToEnd(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.WaitForAnimationToEndStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
	}
	result := driver.waitForAnimationToEnd(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestInputTextWithUnicode tests inputText with non-ASCII characters
func TestInputTextWithUnicode(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text: "Hello 你好 🎉",
	}

	result := driver.inputText(step)
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	// Should contain unicode warning
	if !strings.Contains(result.Message, "non-ASCII") {
		t.Error("Expected non-ASCII warning in message")
	}
}

// TestInputTextSelectorNotFound tests inputText when selector fails to find element
func TestInputTextSelectorNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Source endpoint - return empty page
		if strings.HasSuffix(path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}

		// Default
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}
	info := &core.PlatformInfo{Platform: "ios", ScreenWidth: 390, ScreenHeight: 844}
	driver := NewDriver(client, info, "")
	driver.SetFindTimeout(100) // Short timeout

	step := &flow.InputTextStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Text:     "test",
		Selector: flow.Selector{Text: "NonExistent"},
	}

	result := driver.inputText(step)
	if result.Success {
		t.Error("Expected failure when selector doesn't find element")
	}
}

// TestInputTextWithSelectorNoElementID tests inputText with element that has no ID (tap fallback)
func TestInputTextWithSelectorNoElementID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Source endpoint
		if strings.HasSuffix(path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeTextField type="XCUIElementTypeTextField" label="Email" enabled="true" visible="true" x="50" y="200" width="290" height="44"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}

		// Tap endpoint
		if strings.Contains(path, "/wda/tap") {
			json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
			return
		}

		// Send keys endpoint
		if strings.Contains(path, "/wda/keys") {
			json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
			return
		}

		// Default
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text:     "test@example.com",
		Selector: flow.Selector{Text: "Email"},
	}

	result := driver.inputText(step)
	if !result.Success {
		t.Errorf("Expected success with tap fallback, got: %s", result.Message)
	}
}

// TestLaunchAppEmptyBundleID tests launchApp with empty bundle ID
func TestLaunchAppEmptyBundleID(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.LaunchAppStep{AppID: ""}
	result := driver.launchApp(step)

	if result.Success {
		t.Error("Expected failure for empty bundle ID")
	}
	if !strings.Contains(result.Message, "Bundle ID") {
		t.Errorf("Expected bundle ID error message, got: %s", result.Message)
	}
}

// TestLaunchAppWithoutSession tests launchApp when no session exists
func TestLaunchAppWithoutSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Create session endpoint
		if strings.HasSuffix(path, "/session") && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"sessionId": "new-session"},
			})
			return
		}

		// Default
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	// Create driver without session
	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "", // No session
	}
	info := &core.PlatformInfo{Platform: "ios", ScreenWidth: 390, ScreenHeight: 844}
	driver := NewDriver(client, info, "")

	step := &flow.LaunchAppStep{AppID: "com.test.app"}
	result := driver.launchApp(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestLaunchAppCreateSessionFails tests launchApp when CreateSession fails
func TestLaunchAppCreateSessionFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Create session endpoint - return WDA error format
		if strings.HasSuffix(path, "/session") && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "session not created",
					"message": "Session creation failed",
				},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	// Create driver without session
	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "", // No session
	}
	info := &core.PlatformInfo{Platform: "ios", ScreenWidth: 390, ScreenHeight: 844}
	driver := NewDriver(client, info, "")

	step := &flow.LaunchAppStep{AppID: "com.test.app"}
	result := driver.launchApp(step)

	if result.Success {
		t.Error("Expected failure when CreateSession fails")
	}
}

// TestLaunchAppExistingSessionFails tests launchApp when LaunchApp fails with existing session
func TestLaunchAppExistingSessionFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Launch app endpoint - return WDA error format
		if strings.Contains(path, "/wda/apps/launch") && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "app launch failed",
					"message": "Failed to launch app",
				},
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.LaunchAppStep{AppID: "com.test.app"}
	result := driver.launchApp(step)

	if result.Success {
		t.Error("Expected failure when LaunchApp fails")
	}
}

// TestFindElementQuickWithError tests findElementQuick error handling
func TestFindElementQuickWithError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Source endpoint - return error
		if strings.HasSuffix(path, "/source") {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "Source failed"})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	_, err := driver.findElementQuick(flow.Selector{Text: "Test"}, 100)
	if err == nil {
		t.Error("Expected error when source fails")
	}
}

// TestAssertNotVisibleWhenElementExists tests assertNotVisible when element exists
func TestAssertNotVisibleWhenElementExists(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Selector: flow.Selector{Text: "Login"},
	}
	result := driver.assertNotVisible(step)

	if result.Success {
		t.Error("Expected failure when element is visible")
	}
}

// TestStopAppEmptyBundleID tests stopApp with empty bundle ID
func TestStopAppEmptyBundleID(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.StopAppStep{AppID: ""}
	result := driver.stopApp(step)

	if result.Success {
		t.Error("Expected failure for empty bundle ID")
	}
}

// TestStopAppFails tests stopApp when terminate fails
func TestStopAppFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/apps/terminate") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "terminate failed",
					"message": "Failed to terminate app",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.StopAppStep{AppID: "com.test.app"}
	result := driver.stopApp(step)

	if result.Success {
		t.Error("Expected failure when terminate fails")
	}
}

// TestKillAppEmptyBundleID tests killApp with empty bundle ID
func TestKillAppEmptyBundleID(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.KillAppStep{AppID: ""}
	result := driver.killApp(step)

	if result.Success {
		t.Error("Expected failure for empty bundle ID")
	}
}

// TestClearStateEmptyBundleID tests clearState with empty bundle ID
func TestClearStateEmptyBundleID(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ClearStateStep{AppID: ""}
	result := driver.clearState(step)

	if result.Success {
		t.Error("Expected failure for empty bundle ID")
	}
}

// TestDoubleTapOnNotFound tests doubleTapOn when element not found
func TestDoubleTapOnNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"}
	driver := NewDriver(client, &core.PlatformInfo{Platform: "ios", ScreenWidth: 390, ScreenHeight: 844}, "")
	driver.SetFindTimeout(100)

	step := &flow.DoubleTapOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Selector: flow.Selector{Text: "NonExistent"},
	}
	result := driver.doubleTapOn(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
}

// TestLongPressOnNotFound tests longPressOn when element not found
func TestLongPressOnNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"}
	driver := NewDriver(client, &core.PlatformInfo{Platform: "ios", ScreenWidth: 390, ScreenHeight: 844}, "")
	driver.SetFindTimeout(100)

	step := &flow.LongPressOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Selector: flow.Selector{Text: "NonExistent"},
	}
	result := driver.longPressOn(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
}

// TestCopyTextFromEmptyText tests copyTextFrom when element has empty text
func TestCopyTextFromEmptyText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			// Element with no label (text) - only name for matching
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeImage type="XCUIElementTypeImage" name="noTextImage" enabled="true" visible="true" x="50" y="100" width="100" height="50"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	// Select by ID - element exists but has no label (text)
	// The function returns success even with empty text
	step := &flow.CopyTextFromStep{
		Selector: flow.Selector{ID: "noTextImage"},
	}
	result := driver.copyTextFrom(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	// Data should be empty since element has no label
	if result.Data != "" {
		t.Errorf("Expected empty text, got: %s", result.Data)
	}
}

// TestTakeScreenshotError tests takeScreenshot when screenshot fails
func TestTakeScreenshotError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/screenshot") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "screenshot failed",
					"message": "Failed to capture screenshot",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TakeScreenshotStep{Path: "/tmp/test.png"}
	result := driver.takeScreenshot(step)

	if result.Success {
		t.Error("Expected failure when screenshot fails")
	}
}

// TestOpenLinkError tests openLink when deep link fails
func TestOpenLinkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/url") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "open url failed",
					"message": "Failed to open URL",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenLinkStep{Link: "myapp://test"}
	result := driver.openLink(step)

	if result.Success {
		t.Error("Expected failure when openLink fails")
	}
}

// TestOpenBrowserError tests openBrowser when deep link fails
func TestOpenBrowserError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// openBrowser uses DeepLink which calls /url endpoint
		if strings.HasSuffix(r.URL.Path, "/url") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "open url failed",
					"message": "Failed to open URL",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenBrowserStep{URL: "https://example.com"}
	result := driver.openBrowser(step)

	if result.Success {
		t.Error("Expected failure when openBrowser fails")
	}
}

// TestSetOrientationError tests setOrientation when it fails
func TestSetOrientationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/orientation") && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "orientation failed",
					"message": "Failed to set orientation",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SetOrientationStep{Orientation: "landscape"}
	result := driver.setOrientation(step)

	if result.Success {
		t.Error("Expected failure when setOrientation fails")
	}
}

// TestSwipeWithCoordinateError tests swipe when the action fails
func TestSwipeWithCoordinateError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/dragfromtoforduration") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "swipe failed",
					"message": "Failed to perform swipe",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Start: "50%,50%", End: "50%,20%"}
	result := driver.swipe(step)

	if result.Success {
		t.Error("Expected failure when swipe fails")
	}
}

// TestAssertVisibleNotFound tests assertVisible when element not found
func TestAssertVisibleNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"}
	driver := NewDriver(client, &core.PlatformInfo{Platform: "ios", ScreenWidth: 390, ScreenHeight: 844}, "")
	driver.SetFindTimeout(100)

	step := &flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Selector: flow.Selector{Text: "NonExistent"},
	}
	result := driver.assertVisible(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
}

// TestInputRandomName tests inputRandom with Name type
func TestInputRandomName(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputRandomStep{
		DataType: "Name",
	}
	result := driver.inputRandom(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestInputRandomNumber tests inputRandom with Number type
func TestInputRandomNumber(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputRandomStep{
		DataType: "Number",
		Length:   5,
	}
	result := driver.inputRandom(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestInputRandomPersonName tests inputRandom with Person_Name type
func TestInputRandomPersonName(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputRandomStep{
		DataType: "Person_Name",
	}
	result := driver.inputRandom(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
}

// TestInputRandomUnknownTypeDefaultsToText tests inputRandom with unknown type defaults to text
func TestInputRandomUnknownTypeDefaultsToText(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputRandomStep{
		DataType: "Unknown",
		Length:   8,
	}
	result := driver.inputRandom(step)

	// Unknown types default to TEXT, so it should succeed
	if !result.Success {
		t.Errorf("Expected success (defaults to TEXT), got: %s", result.Message)
	}
}

// TestClientFindElements tests FindElements client method
func TestClientFindElements(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/elements") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": []interface{}{
					map[string]interface{}{"ELEMENT": "elem-1"},
					map[string]interface{}{"ELEMENT": "elem-2"},
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	ids, err := client.FindElements("class name", "XCUIElementTypeButton")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("Expected 2 elements, got %d", len(ids))
	}
}

// TestClientFindElementsEmpty tests FindElements with empty result
func TestClientFindElementsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/elements") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": []interface{}{},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	ids, err := client.FindElements("class name", "XCUIElementTypeButton")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("Expected 0 elements, got %d", len(ids))
	}
}

// TestClientElementTextError tests ElementText error path
func TestClientElementTextError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/text") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "text failed",
					"message": "Failed to get text",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.ElementText("elem-123")
	if err == nil {
		t.Error("Expected error when ElementText fails")
	}
}

// TestClientElementDisplayedError tests ElementDisplayed error path
func TestClientElementDisplayedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/displayed") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "displayed failed",
					"message": "Failed to check displayed",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.ElementDisplayed("elem-123")
	if err == nil {
		t.Error("Expected error when ElementDisplayed fails")
	}
}

// TestTapOnPointWithPercentageError tests tapOnPointWithPercentage with invalid percentage
func TestTapOnPointWithPercentageInvalidX(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnPointStep{Point: "invalid%, 50%"}
	result := driver.tapOnPoint(step)

	if result.Success {
		t.Error("Expected failure for invalid x percentage")
	}
}

// TestTapOnPointWithPercentageInvalidY tests tapOnPointWithPercentage with invalid y percentage
func TestTapOnPointWithPercentageInvalidY(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnPointStep{Point: "50%, invalid%"}
	result := driver.tapOnPoint(step)

	if result.Success {
		t.Error("Expected failure for invalid y percentage")
	}
}

// TestCopyTextFromElementHasText tests copyTextFrom success with element text
func TestCopyTextFromElementHasText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0"?>
<AppiumAUT>
  <XCUIElementTypeButton name="testBtn" label="CopyMe" x="10" y="20" width="100" height="50"/>
</AppiumAUT>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.CopyTextFromStep{
		Selector: flow.Selector{Text: "CopyMe"},
	}
	result := driver.copyTextFrom(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if result.Data != "CopyMe" {
		t.Errorf("Expected data 'CopyMe', got '%v'", result.Data)
	}
}

// TestSwipeStartEndCoords tests swipe with start/end percentage strings
func TestSwipeWDAStartEndCoords(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

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
func TestSwipeWDAStartError(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

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
func TestSwipeWDAEndError(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

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
func TestSwipeWDAInvalidDirection(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "invalid"}
	result := driver.swipe(step)

	if result.Success {
		t.Error("Expected failure for invalid direction")
	}
}

// TestSwipeAbsoluteCoords tests swipe with absolute pixel coordinates
func TestSwipeWDAAbsoluteCoords(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

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

// TestHierarchyError tests Hierarchy when source fails
func TestHierarchyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "source failed",
					"message": "Failed to get source",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	_, err := driver.Hierarchy()
	if err == nil {
		t.Error("Expected error when Hierarchy fails")
	}
}

// TestSourceError tests Source client method when it fails
func TestSourceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "source failed",
					"message": "Failed to get source",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.Source()
	if err == nil {
		t.Error("Expected error when Source fails")
	}
}

// TestGetOrientationError tests GetOrientation when it fails
func TestGetOrientationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/orientation") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "orientation failed",
					"message": "Failed to get orientation",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.GetOrientation()
	if err == nil {
		t.Error("Expected error when GetOrientation fails")
	}
}

// TestGetActiveElementError tests GetActiveElement when it fails
func TestGetActiveElementError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/element/active") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "active element failed",
					"message": "No active element",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.GetActiveElement()
	if err == nil {
		t.Error("Expected error when GetActiveElement fails")
	}
}

// TestTapOnError tests tapOn when tap fails
func TestTapOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0"?>
<AppiumAUT>
  <XCUIElementTypeButton name="btn" label="Button" x="10" y="20" width="100" height="50"/>
</AppiumAUT>`,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/tap") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "tap failed",
					"message": "Failed to tap",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "Button"},
	}
	result := driver.tapOn(step)

	if result.Success {
		t.Error("Expected failure when tap fails")
	}
}

// TestPressKeyHomeError tests pressKey when Home() fails
func TestPressKeyHomeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/pressButton") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "home failed",
					"message": "Failed to go home",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "home"}
	result := driver.pressKey(step)

	if result.Success {
		t.Error("Expected failure when Home fails")
	}
}

// TestPressKeyVolumeUpError tests pressKey when volumeUp fails
func TestPressKeyVolumeUpError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/pressButton") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "button failed",
					"message": "Failed to press button",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "volumeUp"}
	result := driver.pressKey(step)

	if result.Success {
		t.Error("Expected failure when volumeUp fails")
	}
}

// TestFindElementsError tests FindElements client method error
func TestFindElementsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/elements") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "find elements failed",
					"message": "No elements found",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.FindElements("class name", "XCUIElementTypeButton")
	if err == nil {
		t.Error("Expected error when FindElements fails")
	}
}

// TestCopyTextFromNotFound tests copyTextFrom when element not found
func TestCopyTextFromNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0"?>
<AppiumAUT>
  <XCUIElementTypeButton name="btn" label="Button" x="10" y="20" width="100" height="50"/>
</AppiumAUT>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.CopyTextFromStep{
		Selector: flow.Selector{Text: "NonExistent"},
	}
	result := driver.copyTextFrom(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
}

// TestTapOnNotFoundError tests tapOn when element not found
func TestTapOnNotFoundError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0"?>
<AppiumAUT>
  <XCUIElementTypeButton name="btn" label="Button" x="10" y="20" width="100" height="50"/>
</AppiumAUT>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Selector: flow.Selector{Text: "NonExistent"},
	}
	result := driver.tapOn(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
}

// TestDoubleTapOnError tests doubleTapOn when doubleTap fails
func TestDoubleTapOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0"?>
<AppiumAUT>
  <XCUIElementTypeButton name="btn" label="Button" x="10" y="20" width="100" height="50"/>
</AppiumAUT>`,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/doubleTap") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "doubleTap failed",
					"message": "Failed to double tap",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.DoubleTapOnStep{
		Selector: flow.Selector{Text: "Button"},
	}
	result := driver.doubleTapOn(step)

	if result.Success {
		t.Error("Expected failure when doubleTap fails")
	}
}

// TestLongPressOnError tests longPressOn when longPress fails
func TestLongPressOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0"?>
<AppiumAUT>
  <XCUIElementTypeButton name="btn" label="Button" x="10" y="20" width="100" height="50"/>
</AppiumAUT>`,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/touchAndHold") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "longPress failed",
					"message": "Failed to long press",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.LongPressOnStep{
		Selector: flow.Selector{Text: "Button"},
	}
	result := driver.longPressOn(step)

	if result.Success {
		t.Error("Expected failure when longPress fails")
	}
}

// TestTapOnPointError tests tapOnPoint when tap fails
func TestTapOnPointError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/tap") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "tap failed",
					"message": "Failed to tap",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnPointStep{X: 100, Y: 200}
	result := driver.tapOnPoint(step)

	if result.Success {
		t.Error("Expected failure when tap fails")
	}
}

// TestKillAppTerminateError tests killApp when terminate fails
func TestKillAppTerminateError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/apps/terminate") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "terminate failed",
					"message": "Failed to terminate app",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.KillAppStep{AppID: "com.test.app"}
	result := driver.killApp(step)

	if result.Success {
		t.Error("Expected failure when killApp fails")
	}
}

// TestMatchesTextInvalidRegexFallback tests that invalid regex falls back to contains
func TestMatchesTextInvalidRegexFallback(t *testing.T) {
	// Invalid regex pattern - unclosed bracket
	result := matchesText("[invalid", "test [invalid value")
	if !result {
		t.Error("Expected true for invalid regex fallback to contains")
	}

	// Invalid regex that doesn't contain the text
	result = matchesText("[invalid", "no match")
	if result {
		t.Error("Expected false when invalid regex doesn't match")
	}
}

// TestMatchesTextRegexStrippedNewline tests regex with stripped newlines
func TestMatchesTextRegexStrippedNewline(t *testing.T) {
	result := matchesText("test.*value", "test\nvalue")
	if !result {
		t.Error("Expected true for regex matching stripped newlines")
	}
}

// TestMatchesTextExactPatternMatch tests exact pattern match path
func TestMatchesTextExactPatternMatch(t *testing.T) {
	// This tests the path where pattern == text exactly
	result := matchesText("Test.*", "Test.*")
	if !result {
		t.Error("Expected true for exact pattern match")
	}
}

// TestFindElementQuickWithSize tests findElementQuick with width/height selector
func TestFindElementQuickWithSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0"?>
<AppiumAUT>
  <XCUIElementTypeButton name="btn" label="TestButton" x="10" y="10" width="100" height="50"/>
  <XCUIElementTypeButton name="btn2" label="OtherButton" x="10" y="100" width="200" height="100"/>
</AppiumAUT>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	// Find by width
	sel := flow.Selector{Width: 100, Height: 50, Tolerance: 5}
	info, err := driver.findElementQuick(sel, 1000)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementQuickWDAFallback tests findElementQuick when WDA fails
func TestFindElementQuickWDAFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// WDA element finds fail
		if strings.Contains(r.URL.Path, "/element") && r.Method == "POST" && !strings.Contains(r.URL.Path, "/elements") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error": "no such element",
				},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0"?>
<AppiumAUT>
  <XCUIElementTypeStaticText name="testID" label="Fallback Text" x="10" y="10" width="100" height="50"/>
</AppiumAUT>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{ID: "testID"}
	info, err := driver.findElementQuick(sel, 1000)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info from page source fallback")
	}
}

// TestResolveRelativeSelectorAllElements tests when no base selector props
func TestResolveRelativeSelectorAllElements(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	source, _ := driver.client.Source()
	elements, _ := ParsePageSource(source)

	// Selector with only relative filter, no text/ID
	sel := flow.Selector{
		Below: &flow.Selector{Text: "Header"},
	}

	info, err := driver.resolveRelativeSelector(sel, elements)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementRelativeParseError tests findElementRelative when parse fails
func TestFindElementRelativeParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": "invalid xml <not closed",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{
		Text:  "Test",
		Below: &flow.Selector{Text: "Header"},
	}

	_, err := driver.findElementRelative(sel, 100)
	if err == nil {
		t.Error("Expected error when XML parse fails")
	}
}

// TestFindElementRelativeSourceError tests findElementRelative when source fails
func TestFindElementRelativeSourceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "source error",
					"message": "Failed to get source",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{
		Text:  "Test",
		Below: &flow.Selector{Text: "Header"},
	}

	_, err := driver.findElementRelative(sel, 100)
	if err == nil {
		t.Error("Expected error when source fails")
	}
}

// TestFindElementsClientMethod tests client FindElements method
func TestFindElementsClientMethod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/elements") && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": []map[string]interface{}{
					{"ELEMENT": "elem1"},
					{"ELEMENT": "elem2"},
					{"ELEMENT": "elem3"},
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	elements, err := client.FindElements("class chain", "**/XCUIElementTypeButton")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(elements) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(elements))
	}
}

// TestFindElementsEmptyResult tests client FindElements with empty result
func TestFindElementsEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/elements") && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": []map[string]interface{}{},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	elements, err := client.FindElements("class chain", "**/XCUIElementTypeButton")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(elements) != 0 {
		t.Errorf("Expected 0 elements, got %d", len(elements))
	}
}

// TestFindElementsError tests client FindElements when error occurs
func TestFindElementsErrorPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"error":   "no such element",
				"message": "Elements not found",
			},
		})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	_, err := client.FindElements("class chain", "**/XCUIElementTypeButton")
	if err == nil {
		t.Error("Expected error when FindElements fails")
	}
}

// TestContainsDescendantsFilter tests containsDescendants filter path
func TestContainsDescendantsFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0"?>
<AppiumAUT>
  <XCUIElementTypeOther name="container" x="0" y="0" width="400" height="400">
    <XCUIElementTypeButton name="btn" label="ChildButton" x="10" y="10" width="100" height="50"/>
  </XCUIElementTypeOther>
  <XCUIElementTypeOther name="other" x="0" y="500" width="400" height="100"/>
</AppiumAUT>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	source, _ := driver.client.Source()
	elements, _ := ParsePageSource(source)

	sel := flow.Selector{
		ContainsDescendants: []*flow.Selector{
			{Text: "ChildButton"},
		},
	}

	info, err := driver.resolveRelativeSelector(sel, elements)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element containing descendant")
	}
}

// TestSortByDistanceYDirect tests sortByDistanceY function
func TestSortByDistanceYDirect(t *testing.T) {
	elements := []*ParsedElement{
		{Bounds: core.Bounds{Y: 100}},
		{Bounds: core.Bounds{Y: 50}},
		{Bounds: core.Bounds{Y: 200}},
	}

	anchorY := 60

	// sortByDistanceY sorts in place
	sortByDistanceY(elements, anchorY)

	if len(elements) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(elements))
	}
	// Should be sorted by distance from anchorY=60
	// Element at Y=50 is closest (dist=-10), then Y=100 (dist=40), then Y=200 (dist=140)
	if elements[0].Bounds.Y != 50 {
		t.Errorf("Expected first element Y=50, got Y=%d", elements[0].Bounds.Y)
	}
}

// TestSortByDistanceXDirect tests sortByDistanceX function
func TestSortByDistanceXDirect(t *testing.T) {
	elements := []*ParsedElement{
		{Bounds: core.Bounds{X: 100}},
		{Bounds: core.Bounds{X: 50}},
		{Bounds: core.Bounds{X: 200}},
	}

	anchorX := 60

	// sortByDistanceX sorts in place
	sortByDistanceX(elements, anchorX)

	if len(elements) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(elements))
	}
	// Should be sorted by distance from anchorX=60
	// Element at X=50 is closest (dist=-10), then X=100 (dist=40), then X=200 (dist=140)
	if elements[0].Bounds.X != 50 {
		t.Errorf("Expected first element X=50, got X=%d", elements[0].Bounds.X)
	}
}

// TestBuildStateFilterDisabled tests buildStateFilter with enabled=false
func TestBuildStateFilterDisabled(t *testing.T) {
	enabled := false
	sel := flow.Selector{Enabled: &enabled}
	result := buildStateFilter(sel)
	if result != " AND enabled == false" {
		t.Errorf("Expected enabled == false, got: %s", result)
	}
}

// TestBuildStateFilterSelected tests buildStateFilter with selected
func TestBuildStateFilterSelected(t *testing.T) {
	selected := true
	sel := flow.Selector{Selected: &selected}
	result := buildStateFilter(sel)
	if result != " AND selected == true" {
		t.Errorf("Expected selected == true, got: %s", result)
	}

	selected = false
	sel = flow.Selector{Selected: &selected}
	result = buildStateFilter(sel)
	if result != " AND selected == false" {
		t.Errorf("Expected selected == false, got: %s", result)
	}
}

// TestBuildStateFilterFocused tests buildStateFilter with focused
func TestBuildStateFilterFocused(t *testing.T) {
	focused := true
	sel := flow.Selector{Focused: &focused}
	result := buildStateFilter(sel)
	if result != " AND hasFocus == true" {
		t.Errorf("Expected hasFocus == true, got: %s", result)
	}

	focused = false
	sel = flow.Selector{Focused: &focused}
	result = buildStateFilter(sel)
	if result != " AND hasFocus == false" {
		t.Errorf("Expected hasFocus == false, got: %s", result)
	}
}

// TestClientDeleteMethod tests delete client method
func TestClientDeleteMethod(t *testing.T) {
	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "DELETE" {
			deleteCalled = true
			json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	_, err := client.delete("/test")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !deleteCalled {
		t.Error("Expected DELETE method to be called")
	}
}

// TestClientDeleteError tests delete method when error occurs
func TestClientDeleteError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"error":   "delete failed",
				"message": "Failed to delete",
			},
		})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	_, err := client.delete("/test")
	if err == nil {
		t.Error("Expected error when delete fails")
	}
}

// TestWaitForAnimationToEndErrorPath tests waitForAnimationToEnd error path
func TestWaitForAnimationToEndErrorPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/screen") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "screen error",
					"message": "Failed to get screen",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	// Should not error even when screenshot fails
	step := &flow.WaitForAnimationToEndStep{BaseStep: flow.BaseStep{TimeoutMs: 100}}
	driver.waitForAnimationToEnd(step)
}

// TestTapOnWithID tests tapOn with ID selector
func TestTapOnWithID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/element") && strings.Contains(r.URL.Path, "/rect") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"x": 50, "y": 100, "width": 100, "height": 50,
				},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/elements") || (strings.Contains(r.URL.Path, "/element") && r.Method == "POST") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "elem1"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{ID: "myButton"},
	}
	result := driver.tapOn(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestTapOnWithPoint tests tapOn with percentage-based Point
func TestTapOnWithPoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"width":  1080,
					"height": 2340,
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Point: "50%, 50%",
	}
	result := driver.tapOn(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestTapOnOptionalNotFound tests tapOn with optional element not found
func TestTapOnOptionalNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/element") || strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error": "no such element",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100, Optional: true},
		Selector: flow.Selector{Text: "NonExistent"},
	}
	result := driver.tapOn(step)

	// Optional element not found should succeed
	if !result.Success {
		t.Errorf("Expected success for optional, got error: %v", result.Error)
	}
}

// TestTapOnWithElementIDClickFallback tests tapOn with element ID that fails click
func TestTapOnWithElementIDClickFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/element") && strings.Contains(r.URL.Path, "/click") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error": "click failed",
				},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/element") && strings.Contains(r.URL.Path, "/rect") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"x": 50, "y": 100, "width": 100, "height": 50,
				},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/element") && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "elem1"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{ID: "myButton"},
	}
	result := driver.tapOn(step)

	if !result.Success {
		t.Errorf("Expected success with fallback, got error: %v", result.Error)
	}
}

// TestTapOnPointWithPercentageTapFails tests tapOnPointWithPercentage when tap fails
func TestTapOnPointWithPercentageTapFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"width":  1080,
					"height": 2340,
				},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/tap") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error": "tap failed",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	result := driver.tapOnPointWithPercentage("50%, 50%")

	if result.Success {
		t.Error("Expected failure when tap fails")
	}
}

// TestFindElementsW3CFormat tests client FindElements with W3C format
func TestFindElementsW3CFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/elements") && r.Method == "POST" {
			// W3C format - uses UUID key instead of ELEMENT
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": []map[string]interface{}{
					{"element-6066-11e4-a52e-4f735466cecf": "elem1"},
					{"element-6066-11e4-a52e-4f735466cecf": "elem2"},
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	elements, err := client.FindElements("class chain", "**/XCUIElementTypeButton")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(elements) != 2 {
		t.Errorf("Expected 2 elements, got %d", len(elements))
	}
}

// TestGetClientMethod tests get client method error path
func TestGetClientMethodError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"error":   "get failed",
				"message": "Failed to get",
			},
		})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	_, err := client.get("/test")
	if err == nil {
		t.Error("Expected error when get fails")
	}
}

// TestPostClientMethod tests post client method
func TestPostClientMethodSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"result": "success",
			},
		})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	resp, err := client.post("/test", map[string]interface{}{"key": "value"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Error("Expected response")
	}
}

// TestSelectorDescWithText tests selectorDesc with text
func TestSelectorDescWithText(t *testing.T) {
	sel := flow.Selector{Text: "Hello"}
	desc := selectorDesc(sel)
	if desc != "text='Hello'" {
		t.Errorf("Expected text='Hello', got: %s", desc)
	}
}

// TestSelectorDescWithID tests selectorDesc with ID
func TestSelectorDescWithID(t *testing.T) {
	sel := flow.Selector{ID: "myID"}
	desc := selectorDesc(sel)
	if desc != "id='myID'" {
		t.Errorf("Expected id='myID', got: %s", desc)
	}
}

// TestSelectorDescEmpty tests selectorDesc with empty selector
func TestSelectorDescEmpty(t *testing.T) {
	sel := flow.Selector{}
	desc := selectorDesc(sel)
	if desc != "selector" {
		t.Errorf("Expected 'selector', got: %s", desc)
	}
}

// TestAssertNotVisibleOptional tests assertNotVisible with optional flag
func TestAssertNotVisibleOptional(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<?xml version="1.0"?>
<AppiumAUT>
  <XCUIElementTypeButton name="btn" label="FoundButton" x="10" y="10" width="100" height="50"/>
</AppiumAUT>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100, Optional: true},
		Selector: flow.Selector{Text: "FoundButton"},
	}
	result := driver.assertNotVisible(step)

	// Element found but optional - still fails because assertNotVisible means element should NOT be there
	if result.Success {
		t.Error("Expected failure when element is found (assertNotVisible)")
	}
}

// TestInputTextAppendMode tests inputText with append mode
func TestInputTextAppendMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text: "Hello World",
	}
	result := driver.inputText(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestParseResponseInvalidJSON tests parseResponse with invalid JSON
func TestParseResponseInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	_, err := client.get("/test")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestParseResponseWDAErrorNoMessage tests WDA error without message
func TestParseResponseWDAErrorNoMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"error": "simple error",
			},
		})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	_, err := client.get("/test")
	if err == nil {
		t.Error("Expected error")
	}
	if !strings.Contains(err.Error(), "simple error") {
		t.Errorf("Expected error to contain 'simple error', got: %s", err)
	}
}

// TestTapOnPointWithPercentageWindowSizeError tests when WindowSize fails
func TestTapOnPointWithPercentageWindowSizeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"error": "window size failed",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	result := driver.tapOnPointWithPercentage("50%, 50%")

	if result.Success {
		t.Error("Expected failure when WindowSize fails")
	}
}

// TestFindElementQuickWithRelative tests findElementQuick with relative selector
func TestFindElementQuickWithRelative(t *testing.T) {
	server := mockWDAServerForRelativeElements()
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{
		Text:  "BelowButton",
		Below: &flow.Selector{Text: "Header"},
	}
	info, err := driver.findElementQuick(sel, 1000)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementByWDAWithText tests findElementByWDA with text selector
func TestFindElementByWDAWithText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/element") && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "elem1"},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/rect") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"x": 10, "y": 20, "width": 100, "height": 50,
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{Text: "TestText"}
	info, err := driver.findElementByWDA(sel)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("Expected element info")
	}
}

// TestFindElementTimeout tests findElement with timeout
func TestFindElementTimeoutPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"error": "not found",
			},
		})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{Text: "NonExistent"}
	_, err := driver.findElement(sel, false, 100)

	if err == nil {
		t.Error("Expected error when element not found after timeout")
	}
}

// TestPostWithNilBody tests post method with nil body
func TestPostWithNilBody(t *testing.T) {
	postCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			postCalled = true
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	_, err := client.post("/test", nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !postCalled {
		t.Error("Expected POST to be called")
	}
}

// TestFindElementByPageSourceOnceParseError tests findElementByPageSourceOnce when parse fails
func TestFindElementByPageSourceOnceParseErrorPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": "invalid xml <not closed",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	sel := flow.Selector{Text: "Test"}
	_, err := driver.findElementByPageSourceOnce(sel)

	if err == nil {
		t.Error("Expected error when parse fails")
	}
}

// TestInputTextTapError tests inputText when tap for focus fails
func TestInputTextTapError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<XCUIElementTypeApplication>
					<XCUIElementTypeTextField label="InputField" x="10" y="20" width="200" height="40" enabled="true" visible="true"/>
				</XCUIElementTypeApplication>`,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/wda/tap") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"error": "tap failed"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text:     "test",
		Selector: flow.Selector{Text: "InputField"},
	}
	result := driver.inputText(step)

	if result.Success {
		t.Error("Expected error when tap fails")
	}
}

// TestAssertNotVisibleWithTimeout tests assertNotVisible with custom timeout
func TestAssertNotVisibleWithTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return error to indicate element not found
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{"error": "not found"},
		})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertNotVisibleStep{
		Selector: flow.Selector{Text: "SomeText"},
		BaseStep: flow.BaseStep{TimeoutMs: 500},
	}
	result := driver.assertNotVisible(step)

	if !result.Success {
		t.Errorf("Expected success for not visible element: %v", result.Error)
	}
}

// TestSwipeWithPixelCoordinates tests swipe with direct pixel coordinates
func TestSwipeWithPixelCoordinates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{
		StartX: 100,
		StartY: 200,
		EndX:   300,
		EndY:   400,
	}
	result := driver.swipe(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestSwipeWithDuration tests swipe with custom duration
func TestSwipeWithDuration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{
		Direction: "up",
		Duration:  500,
	}
	result := driver.swipe(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestScrollLeft tests scroll with left direction
func TestScrollLeft(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollStep{Direction: "left"}
	result := driver.scroll(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestScrollRight tests scroll with right direction
func TestScrollRight(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollStep{Direction: "right"}
	result := driver.scroll(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestSwipeLeft tests swipe with left direction
func TestSwipeLeft(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "left"}
	result := driver.swipe(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestSwipeRight tests swipe with right direction
func TestSwipeRight(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "right"}
	result := driver.swipe(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestSwipeDown tests swipe with down direction
func TestSwipeDown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "down"}
	result := driver.swipe(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestTapOnCoordinateFallbackError tests tapOn when both click and tap fail
func TestTapOnCoordinateFallbackError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<XCUIElementTypeApplication>
					<XCUIElementTypeButton name="btn1" label="Button" x="10" y="20" width="100" height="50" enabled="true" visible="true"/>
				</XCUIElementTypeApplication>`,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/element") && strings.Contains(r.URL.Path, "/click") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"error": "click failed"},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/wda/tap") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"error": "tap failed"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Selector: flow.Selector{ID: "btn1"},
	}
	result := driver.tapOn(step)

	if result.Success {
		t.Error("Expected error when both click and tap fail")
	}
}

// TestScrollDownDirection tests scroll with down direction
func TestScrollDownDirection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollStep{Direction: "down"}
	result := driver.scroll(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestInputTextSendKeysError tests inputText when sendKeys fails
func TestInputTextSendKeysError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/keys") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"error": "keys failed"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{Text: "test"}
	result := driver.inputText(step)

	if result.Success {
		t.Error("Expected error when sendKeys fails")
	}
}

// TestInputRandomDefaultLength tests inputRandom with default length
func TestInputRandomDefaultLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	// Length is 0, should default to 10
	step := &flow.InputRandomStep{DataType: "TEXT", Length: 0}
	result := driver.inputRandom(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
	// Verify the result data contains 10 characters (default length)
	if resultStr, ok := result.Data.(string); ok {
		if len(resultStr) != 10 {
			t.Errorf("Expected 10 characters (default), got %d", len(resultStr))
		}
	}
}

// TestInputRandomError tests inputRandom when sendKeys fails
func TestInputRandomError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{"error": "send keys failed"},
		})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputRandomStep{DataType: "TEXT", Length: 5}
	result := driver.inputRandom(step)

	if result.Success {
		t.Error("Expected error when sendKeys fails")
	}
}

// TestScrollSwipeError tests scroll when swipe fails
func TestScrollSwipeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/dragfromtoforduration") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"error": "swipe failed"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollStep{Direction: "up"}
	result := driver.scroll(step)

	if result.Success {
		t.Error("Expected error when swipe fails")
	}
}

// TestSwipeInvalidEndCoords tests swipe with invalid end coordinates
func TestSwipeInvalidEndCoords(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{
		Start: "50%, 50%",
		End:   "invalid",
	}
	result := driver.swipe(step)

	if result.Success {
		t.Error("Expected error for invalid end coordinates")
	}
}

// TestSwipeError tests swipe when drag fails
func TestSwipeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/dragfromtoforduration") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"error": "drag failed"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "up"}
	result := driver.swipe(step)

	if result.Success {
		t.Error("Expected error when drag fails")
	}
}

// TestScrollUntilVisibleScrollFails tests scrollUntilVisible when scroll fails
func TestScrollUntilVisibleScrollFails(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if strings.HasSuffix(r.URL.Path, "/source") {
			// Element not found
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<XCUIElementTypeApplication></XCUIElementTypeApplication>`,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/dragfromtoforduration") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"error": "scroll failed"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element:  flow.Selector{Text: "NotFound"},
		BaseStep: flow.BaseStep{TimeoutMs: 100},
	}
	result := driver.scrollUntilVisible(step)

	if result.Success {
		t.Error("Expected error when scroll fails")
	}
}

// TestExecuteUnknownStepType tests Execute with an unknown step type
func TestExecuteUnknownStepType(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	// Create a custom step type that is not supported
	type customStep struct {
		flow.BaseStep
	}
	step := &customStep{}
	result := driver.Execute(step)

	if result.Success {
		t.Error("Expected failure for unknown step type")
	}
	if result.Error == nil {
		t.Error("Expected error to be set")
	}
}

// TestWaitForAnimationToEndDefaultTimeout tests waitForAnimationToEnd returns warning
func TestWaitForAnimationToEndDefaultTimeout(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.WaitForAnimationToEndStep{
		BaseStep: flow.BaseStep{TimeoutMs: 0},
	}
	result := driver.waitForAnimationToEnd(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
	// waitForAnimationToEnd is a placeholder - verify warning message
	if result.Message == "" {
		t.Error("Expected warning message")
	}
}

// TestGetMethodNetworkError tests get method when network fails
func TestGetMethodNetworkError(t *testing.T) {
	// Create a client with invalid URL
	client := &Client{
		baseURL:    "http://localhost:1", // Invalid port
		httpClient: &http.Client{Timeout: 100 * time.Millisecond},
	}

	_, err := client.get("/test")
	if err == nil {
		t.Error("Expected network error")
	}
}

// TestPostMethodNetworkError tests post method when network fails
func TestPostMethodNetworkError(t *testing.T) {
	// Create a client with invalid URL
	client := &Client{
		baseURL:    "http://localhost:1", // Invalid port
		httpClient: &http.Client{Timeout: 100 * time.Millisecond},
	}

	_, err := client.post("/test", map[string]string{"key": "value"})
	if err == nil {
		t.Error("Expected network error")
	}
}

// TestDeleteMethodNetworkError tests delete method when network fails
func TestDeleteMethodNetworkError(t *testing.T) {
	// Create a client with invalid URL
	client := &Client{
		baseURL:    "http://localhost:1", // Invalid port
		httpClient: &http.Client{Timeout: 100 * time.Millisecond},
	}

	_, err := client.delete("/test")
	if err == nil {
		t.Error("Expected network error")
	}
}

// TestEraseTextElementClearFallback tests eraseText when ElementClear fails
func TestEraseTextElementClearFallback(t *testing.T) {
	deleteKeyCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/element/active") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "elem-123"},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/clear") {
			// ElementClear fails
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"error": "clear failed"},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/keys") {
			// Delete keys - count calls
			deleteKeyCount++
			json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.EraseTextStep{Characters: 5}
	result := driver.eraseText(step)

	if !result.Success {
		t.Errorf("Expected success with fallback, got error: %v", result.Error)
	}
	if deleteKeyCount < 1 {
		t.Error("Expected delete keys fallback to be used")
	}
}

// TestScrollUntilVisibleMaxScrolls tests scrollUntilVisible hitting max scrolls
func TestScrollUntilVisibleMaxScrolls(t *testing.T) {
	scrollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			// Element never found
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<XCUIElementTypeApplication></XCUIElementTypeApplication>`,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/dragfromtoforduration") {
			scrollCount++
			json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element:    flow.Selector{Text: "NotFound"},
		MaxScrolls: 3, // Limit max scrolls
		BaseStep:   flow.BaseStep{TimeoutMs: 60000},
	}
	result := driver.scrollUntilVisible(step)

	if result.Success {
		t.Error("Expected failure when element not found after max scrolls")
	}
	if scrollCount < 3 {
		t.Errorf("Expected at least 3 scrolls, got %d", scrollCount)
	}
}

// TestTapOnWithSelectorParseError tests tapOn when page source parsing fails
func TestTapOnWithSelectorParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			// Return invalid XML
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `not valid xml`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "Login"},
		BaseStep: flow.BaseStep{TimeoutMs: 100},
	}
	result := driver.tapOn(step)

	if result.Success {
		t.Error("Expected failure when XML parsing fails")
	}
}

// TestFindElementRelativeWithNonExistentAnchor tests findElementRelative with anchor that doesn't match
func TestFindElementRelativeWithNonExistentAnchor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<XCUIElementTypeApplication>
					<XCUIElementTypeButton label="Target" x="50" y="200" width="100" height="40" enabled="true" visible="true"/>
				</XCUIElementTypeApplication>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	anchorSel := &flow.Selector{Text: "NonExistentAnchor"}
	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:  "Target",
			Below: anchorSel,
		},
		BaseStep: flow.BaseStep{TimeoutMs: 100},
	}
	result := driver.tapOn(step)

	if result.Success {
		t.Error("Expected failure when anchor not found")
	}
}

// TestInputTextWithUnicodeChars tests inputText with non-ASCII characters
func TestInputTextWithUnicodeChars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{Text: "こんにちは"} // Japanese characters
	result := driver.inputText(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "warning") {
		t.Error("Expected warning about non-ASCII characters")
	}
}

// TestScrollUpDirection tests scroll with up direction
func TestScrollUpDirection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollStep{Direction: "up"}
	result := driver.scroll(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestElementRectNetworkError tests ElementRect when network fails
func TestElementRectNetworkError(t *testing.T) {
	// Create a client with invalid URL
	client := &Client{
		baseURL:    "http://localhost:1", // Invalid port
		sessionID:  "test-session",
		httpClient: &http.Client{Timeout: 100 * time.Millisecond},
	}

	_, _, _, _, err := client.ElementRect("elem-1")
	if err == nil {
		t.Error("Expected network error")
	}
}

// TestElementRectParsesValues tests ElementRect parsing of values
func TestElementRectParsesValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/rect") {
			// Return incomplete rect data - only x and y
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"x": 10.0,
					"y": 20.0,
					// width and height missing - should be 0
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		sessionID:  "test-session",
		httpClient: http.DefaultClient,
	}

	x, y, w, h, err := client.ElementRect("elem-1")
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if x != 10 || y != 20 {
		t.Errorf("Expected x=10, y=20, got x=%d, y=%d", x, y)
	}
	if w != 0 || h != 0 {
		t.Errorf("Expected width=0, height=0, got w=%d, h=%d", w, h)
	}
}

// TestFindElementWithCustomFindTimeout tests findElement with custom findTimeout
func TestFindElementWithCustomFindTimeout(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)
	driver.findTimeout = 500 // Set custom timeout

	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "Login"},
		BaseStep: flow.BaseStep{}, // No step timeout - use driver's
	}
	result := driver.tapOn(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestFindElementWithCustomOptionalFindTimeout tests findElement with optional and custom optionalFindTimeout
func TestFindElementWithCustomOptionalFindTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<XCUIElementTypeApplication></XCUIElementTypeApplication>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)
	driver.optionalFindTimeout = 100 // Set custom optional timeout

	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "NotFound"},
		BaseStep: flow.BaseStep{Optional: true}, // Optional - use optionalFindTimeout
	}
	result := driver.tapOn(step)

	// Should succeed because optional element not found is acceptable
	if !result.Success {
		t.Errorf("Expected success for optional element, got error: %v", result.Error)
	}
}

// TestAssertNotVisibleDefaultTimeout tests assertNotVisible with default timeout
func TestAssertNotVisibleDefaultTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<XCUIElementTypeApplication></XCUIElementTypeApplication>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertNotVisibleStep{
		Selector: flow.Selector{Text: "NotFound"},
		BaseStep: flow.BaseStep{TimeoutMs: 0}, // Default timeout
	}
	result := driver.assertNotVisible(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestScrollUntilVisibleWithTimeoutMs tests scrollUntilVisible using TimeoutMs for maxScrolls
func TestScrollUntilVisibleWithTimeoutMs(t *testing.T) {
	scrollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<XCUIElementTypeApplication></XCUIElementTypeApplication>`,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/window/size") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/dragfromtoforduration") {
			scrollCount++
			json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element:  flow.Selector{Text: "NotFound"},
		BaseStep: flow.BaseStep{TimeoutMs: 2000}, // 2 seconds = ~2 scrolls
	}
	result := driver.scrollUntilVisible(step)

	if result.Success {
		t.Error("Expected failure when element not found")
	}
	if scrollCount < 1 {
		t.Errorf("Expected at least 1 scroll, got %d", scrollCount)
	}
}

// TestExecuteScrollUntilVisibleStep tests Execute with ScrollUntilVisibleStep
func TestExecuteScrollUntilVisibleStep(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<XCUIElementTypeApplication>
					<XCUIElementTypeButton label="Target" x="50" y="100" width="100" height="40" enabled="true" visible="true"/>
				</XCUIElementTypeApplication>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element: flow.Selector{Text: "Target"},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestExecuteWaitUntilStep tests Execute with WaitUntilStep
func TestExecuteWaitUntilStep(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<XCUIElementTypeApplication>
					<XCUIElementTypeButton label="Target" x="50" y="100" width="100" height="40" enabled="true" visible="true"/>
				</XCUIElementTypeApplication>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.WaitUntilStep{
		Visible:  &flow.Selector{Text: "Target"},
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
	}
	result := driver.Execute(step)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

// TestPressKeyHomeWhenHomeFails tests pressKey home when Home() fails
func TestPressKeyHomeWhenHomeFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/pressButton") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"error": "button press failed"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "home"}
	result := driver.pressKey(step)

	if result.Success {
		t.Error("Expected failure when home fails")
	}
}

// TestFindElementByWDAWithIDNotFound tests findElementByWDA with ID when not found
func TestFindElementByWDAWithIDNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/elements") {
			// Return empty array - no elements found
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": []interface{}{},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/source") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": `<XCUIElementTypeApplication></XCUIElementTypeApplication>`,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Selector: flow.Selector{ID: "nonexistentID"},
		BaseStep: flow.BaseStep{TimeoutMs: 100},
	}
	result := driver.tapOn(step)

	if result.Success {
		t.Error("Expected failure when ID not found")
	}
}

