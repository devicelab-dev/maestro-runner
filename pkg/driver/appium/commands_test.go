package appium

import (
	"encoding/base64"
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
	if !strings.HasSuffix(email, "@example.com") {
		t.Fatalf("expected email to end with @example.com, got %q", email)
	}
	localPart := strings.TrimSuffix(email, "@example.com")
	if len(localPart) != 8 {
		t.Fatalf("expected local part length 8, got %d", len(localPart))
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
	}
	validLastNames := map[string]bool{
		"Smith": true, "Johnson": true, "Williams": true, "Brown": true, "Jones": true,
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

	step := &flow.WaitForAnimationToEndStep{}
	result := driver.waitForAnimationToEnd(step)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "WARNING") {
		t.Fatalf("expected warning message, got %q", result.Message)
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
		"android.permission.CAMERA":              "allow",
		"android.permission.ACCESS_FINE_LOCATION": "allow",
	}
	driver.grantPermissions("com.test.app", permissions)

	if !grantedPerms["called"] {
		t.Fatalf("expected grant permissions to be called")
	}
}

func TestGrantPermissionsWithAllPermissions(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/execute/sync") {
			callCount++
			writeJSON(w, map[string]interface{}{
				"value": nil,
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// nil permissions should grant all permissions
	driver.grantPermissions("com.test.app", nil)

	expectedCount := len(getAllPermissions())
	if callCount != expectedCount {
		t.Fatalf("expected %d grant calls for all permissions, got %d", expectedCount, callCount)
	}
}

func TestGrantPermissionsWithEmptyMap(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/execute/sync") {
			callCount++
			writeJSON(w, map[string]interface{}{
				"value": nil,
			})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()
	driver := createTestAppiumDriver(server)

	// Empty map should grant all permissions (len(permissions) == 0)
	driver.grantPermissions("com.test.app", map[string]string{})

	expectedCount := len(getAllPermissions())
	if callCount != expectedCount {
		t.Fatalf("expected %d grant calls for all permissions with empty map, got %d", expectedCount, callCount)
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
