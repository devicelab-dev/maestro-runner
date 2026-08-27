package appium

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// =============================================================================
// Pure function tests
// =============================================================================

func TestRandomString(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"length 1", 1},
		{"length 5", 5},
		{"length 10", 10},
		{"length 20", 20},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := randomString(tc.length)
			if len(result) != tc.length {
				t.Fatalf("expected length %d, got %d", tc.length, len(result))
			}
			for _, c := range result {
				if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
					t.Fatalf("unexpected character %q in random string", c)
				}
			}
		})
	}
}

func TestRandomStringUniqueness(t *testing.T) {
	// Generate several strings and verify they are not all identical
	results := make(map[string]bool)
	for i := 0; i < 5; i++ {
		results[randomString(8)] = true
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 unique strings out of 5, got %d", len(results))
	}
}

func TestRandomEmail(t *testing.T) {
	email := randomEmail()
	if !strings.Contains(email, "@") {
		t.Fatalf("expected @ in email, got %q", email)
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		t.Fatalf("invalid email format: %s", email)
	}
	validDomains := map[string]bool{"example.com": true, "test.com": true, "mail.com": true}
	if !validDomains[parts[1]] {
		t.Fatalf("unexpected domain in email: %s", email)
	}
}

func TestRandomNumber(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"length 1", 1},
		{"length 5", 5},
		{"length 10", 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := randomNumber(tc.length)
			if len(result) != tc.length {
				t.Fatalf("expected length %d, got %d", tc.length, len(result))
			}
			for _, c := range result {
				if !unicode.IsDigit(c) {
					t.Fatalf("expected only digits, got %q", c)
				}
			}
		})
	}
}

func TestRandomPersonName(t *testing.T) {
	name := randomPersonName()
	parts := strings.SplitN(name, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected first and last name separated by space, got %q", name)
	}

	validFirstNames := map[string]bool{
		"John": true, "Jane": true, "Michael": true, "Emily": true, "David": true,
		"Sarah": true, "James": true, "Emma": true, "Robert": true, "Olivia": true,
	}
	validLastNames := map[string]bool{
		"Smith": true, "Johnson": true, "Williams": true, "Brown": true, "Jones": true,
		"Garcia": true, "Miller": true, "Davis": true, "Rodriguez": true, "Martinez": true,
	}

	if !validFirstNames[parts[0]] {
		t.Fatalf("unexpected first name %q", parts[0])
	}
	if !validLastNames[parts[1]] {
		t.Fatalf("unexpected last name %q", parts[1])
	}
}

func TestEscapeIOSPredicateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no special chars", "hello", "hello"},
		{"double quote", `say "hello"`, `say \"hello\"`},
		{"backslash", `path\to\file`, `path\\to\\file`},
		{"both quotes and backslash", `"path\name"`, `\"path\\name\"`},
		{"empty string", "", ""},
		{"only double quotes", `""`, `\"\"`},
		{"only backslashes", `\\`, `\\\\`},
		{"mixed content", `label CONTAINS "test\value"`, `label CONTAINS \"test\\value\"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := escapeIOSPredicateString(tc.input)
			if result != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestGetAllPermissions(t *testing.T) {
	perms := getAllPermissions()
	if len(perms) == 0 {
		t.Fatalf("expected non-empty permissions list")
	}

	// Verify all entries start with "android.permission."
	for _, perm := range perms {
		if !strings.HasPrefix(perm, "android.permission.") {
			t.Fatalf("expected permission to start with 'android.permission.', got %q", perm)
		}
	}

	// Check for some well-known permissions
	permSet := make(map[string]bool)
	for _, p := range perms {
		permSet[p] = true
	}
	expected := []string{
		"android.permission.CAMERA",
		"android.permission.ACCESS_FINE_LOCATION",
		"android.permission.RECORD_AUDIO",
	}
	for _, e := range expected {
		if !permSet[e] {
			t.Fatalf("expected permission %q to be in the list", e)
		}
	}
}

// =============================================================================
// newSession / RestartSession tests
// =============================================================================

func TestLaunchAppNewSessionAndroid(t *testing.T) {
	sessionCreateCount := 0
	sessionDeleteCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if path == "/session" && r.Method == "POST" {
			sessionCreateCount++
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId": "new-session",
					"capabilities": map[string]interface{}{
						"platformName": "Android",
					},
				},
			})
			return
		}
		if strings.HasPrefix(path, "/session/") && r.Method == "DELETE" {
			sessionDeleteCount++
			writeJSON(w, map[string]interface{}{"value": nil})
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
	driver.capabilities = map[string]interface{}{
		"platformName":      "Android",
		"appium:appPackage": "com.example.app",
	}

	step := &flow.LaunchAppStep{
		AppID:      "com.example.app",
		NewSession: true,
		ClearState: true,
	}
	result := driver.launchApp(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v - %s", result.Error, result.Message)
	}
	if sessionDeleteCount != 1 {
		t.Fatalf("expected 1 session delete (Disconnect), got %d", sessionDeleteCount)
	}
	if sessionCreateCount != 1 {
		t.Fatalf("expected 1 session create (Connect), got %d", sessionCreateCount)
	}
	// ClearState should still be true on Android
	if !step.ClearState {
		t.Fatal("expected ClearState to remain true on Android")
	}
}

func TestLaunchAppNewSessionIOSRealDevice(t *testing.T) {
	sessionCreateCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if path == "/session" && r.Method == "POST" {
			sessionCreateCount++
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId": "new-session",
					"capabilities": map[string]interface{}{
						"platformName": "iOS",
						"isRealDevice": true,
					},
				},
			})
			return
		}
		if strings.Contains(path, "/window/rect") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0, "x": 0.0, "y": 0.0},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	driver := createTestAppiumDriver(server)
	driver.platform = "ios"
	driver.client.platform = "ios"
	driver.client.isRealDevice = true
	driver.capabilities = map[string]interface{}{
		"platformName":    "iOS",
		"appium:bundleId": "com.example.app",
	}

	step := &flow.LaunchAppStep{
		AppID:      "com.example.app",
		NewSession: true,
		ClearState: true,
	}
	result := driver.launchApp(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v - %s", result.Error, result.Message)
	}
	if sessionCreateCount != 1 {
		t.Fatalf("expected 1 session create, got %d", sessionCreateCount)
	}
	// ClearState should be set to false on iOS real device
	if step.ClearState {
		t.Fatal("expected ClearState to be false on iOS real device with newSession")
	}
}

func TestLaunchAppNewSessionIOSSimulatorIgnored(t *testing.T) {
	sessionCreateCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if path == "/session" && r.Method == "POST" {
			sessionCreateCount++
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId":    "test-session",
					"capabilities": map[string]interface{}{"platformName": "iOS"},
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	driver := createTestAppiumDriver(server)
	driver.platform = "ios"
	driver.client.platform = "ios"
	driver.client.isRealDevice = false // simulator
	driver.capabilities = map[string]interface{}{
		"platformName": "iOS",
	}

	step := &flow.LaunchAppStep{
		AppID:      "com.example.app",
		NewSession: true,
	}
	result := driver.launchApp(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v - %s", result.Error, result.Message)
	}
	// No session should have been created (newSession ignored on simulator)
	if sessionCreateCount != 0 {
		t.Fatalf("expected 0 session creates on iOS simulator, got %d", sessionCreateCount)
	}
}

func TestLaunchAppNewSessionFalseDefault(t *testing.T) {
	sessionCreateCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if path == "/session" && r.Method == "POST" {
			sessionCreateCount++
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId":    "test-session",
					"capabilities": map[string]interface{}{"platformName": "Android"},
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	driver := createTestAppiumDriver(server)

	step := &flow.LaunchAppStep{
		AppID: "com.example.app",
		// NewSession defaults to false
	}
	result := driver.launchApp(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v - %s", result.Error, result.Message)
	}
	// No session should have been created
	if sessionCreateCount != 0 {
		t.Fatalf("expected 0 session creates when newSession=false, got %d", sessionCreateCount)
	}
}

func TestRestartSessionDisconnectFailsStillConnects(t *testing.T) {
	connectCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Disconnect fails
		if strings.HasPrefix(path, "/session/") && r.Method == "DELETE" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "disconnect failed",
					"message": "Session not found",
				},
			})
			return
		}
		// Connect succeeds
		if path == "/session" && r.Method == "POST" {
			connectCalled = true
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId":    "new-session",
					"capabilities": map[string]interface{}{"platformName": "Android"},
				},
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
	driver.capabilities = map[string]interface{}{"platformName": "Android"}

	err := driver.RestartSession()
	if err != nil {
		t.Fatalf("expected success even when Disconnect fails, got: %v", err)
	}
	if !connectCalled {
		t.Fatal("expected Connect to be called even after Disconnect failure")
	}
}

func TestRestartSessionConnectFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if path == "/session" && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "session create failed",
					"message": "Cannot create session",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	driver := createTestAppiumDriver(server)
	driver.capabilities = map[string]interface{}{"platformName": "Android"}

	err := driver.RestartSession()
	if err == nil {
		t.Fatal("expected error when Connect fails")
	}
	if !strings.Contains(err.Error(), "failed to create new session") {
		t.Fatalf("expected 'failed to create new session' error, got: %v", err)
	}
}

func TestRestartSessionResetsState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if path == "/session" && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId":    "new-session",
					"capabilities": map[string]interface{}{"platformName": "Android"},
				},
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
	driver.capabilities = map[string]interface{}{"platformName": "Android"}
	driver.waitForIdleTimeoutSet = true
	driver.lastTappedElementID = "some-element"

	err := driver.RestartSession()
	if err != nil {
		t.Fatalf("RestartSession failed: %v", err)
	}
	if driver.waitForIdleTimeoutSet {
		t.Error("expected waitForIdleTimeoutSet to be reset to false")
	}
	if driver.lastTappedElementID != "" {
		t.Error("expected lastTappedElementID to be reset to empty")
	}
}

func TestDeepCopyCaps(t *testing.T) {
	original := map[string]interface{}{
		"platformName":      "Android",
		"appium:appPackage": "com.example.app",
		"appium:settings": map[string]interface{}{
			"waitForIdleTimeout": 0,
		},
	}

	copied := deepCopyCaps(original)

	// Modify the copy
	copied["appium:autoLaunch"] = false
	if settings, ok := copied["appium:settings"].(map[string]interface{}); ok {
		settings["waitForIdleTimeout"] = 100
	}

	// Original should be unmodified
	if _, exists := original["appium:autoLaunch"]; exists {
		t.Error("original should not have appium:autoLaunch after modifying copy")
	}
	if settings, ok := original["appium:settings"].(map[string]interface{}); ok {
		if val, ok := settings["waitForIdleTimeout"].(int); !ok || val != 0 {
			t.Errorf("original settings should be unmodified, got waitForIdleTimeout=%v", settings["waitForIdleTimeout"])
		}
	}
}

func TestDeepCopyCapsNil(t *testing.T) {
	result := deepCopyCaps(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

// =============================================================================
// Driver method tests using mock servers
// =============================================================================

func TestSetFindTimeout(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// Default timeout
	if driver.getFindTimeout() != DefaultFindTimeout {
		t.Fatalf("expected default find timeout %v, got %v", DefaultFindTimeout, driver.getFindTimeout())
	}

	// Set custom timeout
	driver.SetFindTimeout(5000)
	if driver.getFindTimeout() != 5*time.Second {
		t.Fatalf("expected find timeout 5s, got %v", driver.getFindTimeout())
	}

	// Set another value
	driver.SetFindTimeout(500)
	if driver.getFindTimeout() != 500*time.Millisecond {
		t.Fatalf("expected find timeout 500ms, got %v", driver.getFindTimeout())
	}
}

func TestSetWaitForIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// First call should make HTTP request
	err := driver.SetWaitForIdleTimeout(100)
	if err != nil {
		t.Fatalf("SetWaitForIdleTimeout failed: %v", err)
	}
	if !driver.waitForIdleTimeoutSet {
		t.Fatalf("expected waitForIdleTimeoutSet to be true")
	}
	if driver.currentWaitForIdleTimeout != 100 {
		t.Fatalf("expected currentWaitForIdleTimeout 100, got %d", driver.currentWaitForIdleTimeout)
	}

	// Second call with same value should be a no-op (cached)
	err = driver.SetWaitForIdleTimeout(100)
	if err != nil {
		t.Fatalf("SetWaitForIdleTimeout cached call failed: %v", err)
	}

	// Call with different value should make HTTP request
	err = driver.SetWaitForIdleTimeout(200)
	if err != nil {
		t.Fatalf("SetWaitForIdleTimeout with new value failed: %v", err)
	}
	if driver.currentWaitForIdleTimeout != 200 {
		t.Fatalf("expected currentWaitForIdleTimeout 200, got %d", driver.currentWaitForIdleTimeout)
	}
}

func TestSetWaitForIdleTimeoutError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/appium/settings") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "settings failed",
					"message": "Failed to set settings",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	err := driver.SetWaitForIdleTimeout(100)
	if err == nil {
		t.Fatalf("expected error from SetWaitForIdleTimeout")
	}
	// Should NOT have updated internal state on error
	if driver.waitForIdleTimeoutSet {
		t.Fatalf("expected waitForIdleTimeoutSet to remain false on error")
	}
}

func TestSetClipboard(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SetClipboardStep{Text: "hello clipboard"}
	result := driver.setClipboard(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "hello clipboard") {
		t.Fatalf("expected message to contain text, got %q", result.Message)
	}
}

func TestSetClipboardEmptyText(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SetClipboardStep{Text: ""}
	result := driver.setClipboard(step)

	if result.Success {
		t.Fatalf("expected failure for empty text")
	}
}

func TestSetClipboardError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/appium/device/set_clipboard") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "clipboard failed",
					"message": "Failed to set clipboard",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SetClipboardStep{Text: "test"}
	result := driver.setClipboard(step)

	if result.Success {
		t.Fatalf("expected failure when SetClipboard errors")
	}
}

func TestWaitForAnimationToEnd(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// Mock server returns identical "fake-png-data" on every /screenshot call,
	// so bytes.Equal fast-path fires and the screen is immediately "static".
	step := &flow.WaitForAnimationToEndStep{BaseStep: flow.BaseStep{TimeoutMs: 1000}}
	result := driver.waitForAnimationToEnd(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
}

func TestWaitUntilVisible(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	sel := &flow.Selector{Text: "Login"}
	step := &flow.WaitUntilStep{
		BaseStep: flow.BaseStep{TimeoutMs: 3000},
		Visible:  sel,
	}
	result := driver.waitUntil(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v - %s", result.Error, result.Message)
	}
}

func TestWaitUntilVisibleTimeout(t *testing.T) {
	// Server returns empty hierarchy so element is never found
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><android.widget.FrameLayout bounds="[0,0][1080,2340]"/></hierarchy>`,
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/element") && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "no such element",
					"message": "Element not found",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	sel := &flow.Selector{Text: "NonExistent"}
	step := &flow.WaitUntilStep{
		BaseStep: flow.BaseStep{TimeoutMs: 200},
		Visible:  sel,
	}
	result := driver.waitUntil(step)

	if result.Success {
		t.Fatalf("expected timeout failure for non-existent element")
	}
}

func TestWaitUntilNotVisible(t *testing.T) {
	// Server returns empty hierarchy so element is not found (which means not visible)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><android.widget.FrameLayout bounds="[0,0][1080,2340]"/></hierarchy>`,
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/element") && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "no such element",
					"message": "Element not found",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	sel := &flow.Selector{Text: "GoneElement"}
	step := &flow.WaitUntilStep{
		BaseStep:   flow.BaseStep{TimeoutMs: 3000},
		NotVisible: sel,
	}
	result := driver.waitUntil(step)

	if !result.Success {
		t.Fatalf("expected success (element not visible), got error: %v", result.Error)
	}
}

func TestWaitUntilNotVisibleTimeout(t *testing.T) {
	// Element is always visible so NotVisible should timeout
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	sel := &flow.Selector{Text: "Login"}
	step := &flow.WaitUntilStep{
		BaseStep:   flow.BaseStep{TimeoutMs: 200},
		NotVisible: sel,
	}
	result := driver.waitUntil(step)

	if result.Success {
		t.Fatalf("expected timeout failure when element is still visible")
	}
}

func TestWaitUntilDefaultTimeout(t *testing.T) {
	// Server returns empty hierarchy, TimeoutMs is 0 -> defaults to 30s
	// We test with a short-lived element to not wait 30s
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			writeJSON(w, map[string]interface{}{
				"value": `<hierarchy><android.widget.FrameLayout bounds="[0,0][1080,2340]"/></hierarchy>`,
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/element") && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "no such element",
					"message": "Element not found",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// NotVisible with non-existent element and no timeout -> should succeed quickly
	sel := &flow.Selector{Text: "Gone"}
	step := &flow.WaitUntilStep{
		NotVisible: sel,
	}
	result := driver.waitUntil(step)

	if !result.Success {
		t.Fatalf("expected success for not visible, got error: %v", result.Error)
	}
}

func TestKillApp(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.KillAppStep{AppID: "com.test.app"}
	result := driver.killApp(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "com.test.app") {
		t.Fatalf("expected message to contain app ID, got %q", result.Message)
	}
}

func TestKillAppDefaultAppID(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.KillAppStep{} // No AppID - should use driver.appID
	result := driver.killApp(step)

	if !result.Success {
		t.Fatalf("expected success with default appID, got error: %v", result.Error)
	}
}

func TestKillAppEmptyID(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)
	driver.appID = ""

	step := &flow.KillAppStep{}
	result := driver.killApp(step)

	if result.Success {
		t.Fatalf("expected failure for empty app ID")
	}
}

func TestKillAppError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/appium/device/terminate_app") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "terminate failed",
					"message": "App not found",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.KillAppStep{AppID: "com.bad.app"}
	result := driver.killApp(step)

	if result.Success {
		t.Fatalf("expected failure when terminate fails")
	}
}

func TestInputRandom(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	tests := []struct {
		name     string
		dataType string
		length   int
	}{
		{"email", "EMAIL", 0},
		{"number", "NUMBER", 5},
		{"person_name", "PERSON_NAME", 0},
		{"default text", "", 8},
		{"text with length", "TEXT", 12},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			step := &flow.InputRandomStep{
				DataType: tc.dataType,
				Length:   tc.length,
			}
			result := driver.inputRandom(step)

			if !result.Success {
				t.Fatalf("expected success, got error: %v", result.Error)
			}
			if result.Data == nil {
				t.Fatalf("expected Data to be set with generated text")
			}
		})
	}
}

func TestInputRandomDefaultLength(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// Length <= 0 should default to 10
	step := &flow.InputRandomStep{
		DataType: "TEXT",
		Length:   0,
	}
	result := driver.inputRandom(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
}

func TestInputRandomError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// SendKeys tries /actions first, then /appium/element/active/value
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

	step := &flow.InputRandomStep{DataType: "EMAIL"}
	result := driver.inputRandom(step)

	if result.Success {
		t.Fatalf("expected failure when SendKeys fails")
	}
}

func TestTakeScreenshot(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.TakeScreenshotStep{}
	result := driver.takeScreenshot(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Data == nil {
		t.Fatalf("expected screenshot data to be set")
	}
}

func TestTakeScreenshotError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/screenshot") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "screenshot failed",
					"message": "Failed to capture",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.TakeScreenshotStep{}
	result := driver.takeScreenshot(step)

	if result.Success {
		t.Fatalf("expected failure when screenshot fails")
	}
}

func TestGrantPermissionsWithExplicitPermissions(t *testing.T) {
	grantedPerms := make(map[string]bool)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/execute/sync") {
			// Track which permissions were granted
			grantedPerms["called"] = true
			writeJSON(w, map[string]interface{}{
				"value": nil,
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	permissions := map[string]string{
		"android.permission.CAMERA":               "allow",
		"android.permission.ACCESS_FINE_LOCATION": "allow",
	}
	driver.grantPermissions("com.test.app", permissions)

	if !grantedPerms["called"] {
		t.Fatalf("expected grant permissions to be called")
	}
}

func TestGrantPermissionsWithAllPermissions(t *testing.T) {
	var scripts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/execute/sync") {
			var body struct {
				Script string `json:"script"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			scripts = append(scripts, body.Script)
			if body.Script == "mobile: getPermissions" {
				writeJSON(w, map[string]interface{}{
					"value": []interface{}{"android.permission.CAMERA"},
				})
				return
			}
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// nil permissions means "whatever the app declares"
	driver.grantPermissions("com.test.app", nil)

	// Read what the manifest declares, then grant that list in one call —
	// two requests, regardless of how many permissions exist. The old
	// behaviour shelled out once per entry in a hardcoded list of 32.
	want := []string{"mobile: getPermissions", "mobile: changePermissions"}
	if len(scripts) != len(want) {
		t.Fatalf("issued %v, want exactly %v", scripts, want)
	}
	for i := range want {
		if scripts[i] != want[i] {
			t.Fatalf("issued %v, want %v", scripts, want)
		}
	}
}

func TestGrantPermissionsWithEmptyMap(t *testing.T) {
	var scripts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/execute/sync") {
			var body struct {
				Script string `json:"script"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			scripts = append(scripts, body.Script)
			if body.Script == "mobile: getPermissions" {
				writeJSON(w, map[string]interface{}{
					"value": []interface{}{"android.permission.CAMERA"},
				})
				return
			}
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// An empty map is the same as nil: no explicit list was given.
	driver.grantPermissions("com.test.app", map[string]string{})

	// Read what the manifest declares, then grant that list in one call —
	// two requests, regardless of how many permissions exist. The old
	// behaviour shelled out once per entry in a hardcoded list of 32.
	want := []string{"mobile: getPermissions", "mobile: changePermissions"}
	if len(scripts) != len(want) {
		t.Fatalf("issued %v, want exactly %v", scripts, want)
	}
	for i := range want {
		if scripts[i] != want[i] {
			t.Fatalf("issued %v, want %v", scripts, want)
		}
	}
}

func TestGrantPermissionsErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/execute/sync") {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "grant failed",
					"message": "Permission not declared",
				},
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// Should not panic even when all grants fail
	permissions := map[string]string{
		"android.permission.CAMERA": "allow",
	}
	driver.grantPermissions("com.test.app", permissions)
}

// =============================================================================
// Execute step integration tests (via executeStep)
// =============================================================================

func TestExecuteWaitForAnimationToEnd(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.WaitForAnimationToEndStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Duration == 0 {
		t.Fatalf("expected non-zero duration from Execute")
	}
}

func TestExecuteKillApp(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.KillAppStep{AppID: "com.test.app"}
	result := driver.Execute(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
}

func TestExecuteInputRandom(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.InputRandomStep{DataType: "EMAIL"}
	result := driver.Execute(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
}

func TestExecuteTakeScreenshot(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.TakeScreenshotStep{}
	result := driver.Execute(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
}

func TestExecuteSetClipboard(t *testing.T) {
	server := mockAppiumServerForDriver()
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.SetClipboardStep{Text: "test clipboard"}
	result := driver.Execute(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
}

// =============================================================================
// TakeScreenshot data verification
// =============================================================================

func TestTakeScreenshotReturnsDecodedData(t *testing.T) {
	fakeImage := []byte("PNG-fake-image-bytes")
	encoded := base64.StdEncoding.EncodeToString(fakeImage)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/screenshot") {
			writeJSON(w, map[string]interface{}{
				"value": encoded,
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	step := &flow.TakeScreenshotStep{}
	result := driver.takeScreenshot(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	data, ok := result.Data.([]byte)
	if !ok {
		t.Fatalf("expected Data to be []byte, got %T", result.Data)
	}
	if string(data) != string(fakeImage) {
		t.Fatalf("expected decoded image data, got %q", string(data))
	}
}
