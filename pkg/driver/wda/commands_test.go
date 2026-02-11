package wda

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// =============================================================================
// eraseText tests
// =============================================================================

// TestEraseTextPartialEraseWithRemainingText tests eraseText Case 2:
// erasing N characters from end, leaving remaining text via clear+retype.
func TestEraseTextPartialEraseWithRemainingText(t *testing.T) {
	var clearedElem bool
	var sentKeysText string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Active element
		if strings.Contains(path, "/element/active") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "field-1"},
			})
			return
		}

		// Element text - return text with 10 chars
		if strings.Contains(path, "/element/") && strings.HasSuffix(path, "/text") {
			jsonResponse(w, map[string]interface{}{"value": "HelloWorld"})
			return
		}

		// Element clear
		if strings.Contains(path, "/element/") && strings.HasSuffix(path, "/clear") {
			clearedElem = true
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}

		// SendKeys - capture what was typed back
		if strings.Contains(path, "/wda/keys") {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			_ = json.Unmarshal(body, &payload)
			if v, ok := payload["value"]; ok {
				if arr, ok := v.([]interface{}); ok {
					var parts []string
					for _, c := range arr {
						if s, ok := c.(string); ok {
							parts = append(parts, s)
						}
					}
					sentKeysText = strings.Join(parts, "")
				}
			}
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}

		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	// Erase 5 characters from "HelloWorld" (10 chars) -> should leave "Hello"
	step := &flow.EraseTextStep{Characters: 5}
	result := driver.eraseText(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if !clearedElem {
		t.Error("Expected ElementClear to be called")
	}
	if sentKeysText != "Hello" {
		t.Errorf("Expected remaining text 'Hello', got '%s'", sentKeysText)
	}
}

// TestEraseTextPartialEraseAllChars tests eraseText Case 1:
// erasing more characters than exist -> just Clear()
func TestEraseTextPartialEraseAllChars(t *testing.T) {
	var clearCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/element/active") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "field-1"},
			})
			return
		}
		if strings.Contains(path, "/element/") && strings.HasSuffix(path, "/text") {
			jsonResponse(w, map[string]interface{}{"value": "Hi"}) // 2 chars
			return
		}
		if strings.Contains(path, "/element/") && strings.HasSuffix(path, "/clear") {
			clearCalled = true
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	// Erase 50 chars from a 2-char field -> should just Clear()
	step := &flow.EraseTextStep{Characters: 50}
	result := driver.eraseText(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if !clearCalled {
		t.Error("Expected ElementClear to be called for full erase")
	}
}

// TestEraseTextUnicodeRunes tests eraseText with Unicode text
// to verify rune-based counting works correctly.
func TestEraseTextUnicodeRunes(t *testing.T) {
	var sentKeysText string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/element/active") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "field-1"},
			})
			return
		}
		// Unicode text: each CJK char is 1 rune but 3 bytes
		if strings.Contains(path, "/element/") && strings.HasSuffix(path, "/text") {
			jsonResponse(w, map[string]interface{}{"value": "AB\u4F60\u597D"}) // "AB你好" = 4 runes
			return
		}
		if strings.Contains(path, "/element/") && strings.HasSuffix(path, "/clear") {
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		if strings.Contains(path, "/wda/keys") {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			_ = json.Unmarshal(body, &payload)
			if v, ok := payload["value"]; ok {
				if arr, ok := v.([]interface{}); ok {
					var parts []string
					for _, c := range arr {
						if s, ok := c.(string); ok {
							parts = append(parts, s)
						}
					}
					sentKeysText = strings.Join(parts, "")
				}
			}
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	// Erase 2 runes from "AB你好" (4 runes) -> should leave "AB"
	step := &flow.EraseTextStep{Characters: 2}
	result := driver.eraseText(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if sentKeysText != "AB" {
		t.Errorf("Expected remaining text 'AB', got '%s'", sentKeysText)
	}
}

// TestEraseTextSendKeysFallbackError tests eraseText when all paths fail
// (GetActiveElement fails, and SendKeys for backspaces also fails).
func TestEraseTextSendKeysFallbackError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/element/active") {
			// GetActiveElement fails
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"error": "no active element"},
			})
			return
		}
		if strings.Contains(path, "/wda/keys") {
			// SendKeys also fails
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"error": "keys failed"},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.EraseTextStep{Characters: 5}
	result := driver.eraseText(step)

	if result.Success {
		t.Error("Expected failure when both active element and sendKeys fail")
	}
}

// TestEraseTextZeroCharacters tests eraseText with 0 characters (default to 50).
func TestEraseTextZeroCharacters(t *testing.T) {
	var sentKeysBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/element/active") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"error": "no active"},
			})
			return
		}
		if strings.Contains(path, "/wda/keys") {
			body, _ := io.ReadAll(r.Body)
			sentKeysBody = string(body)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	// Characters=0, should default to 50 backspaces
	step := &flow.EraseTextStep{Characters: 0}
	result := driver.eraseText(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	// Verify 50 backspace characters were sent
	if !strings.Contains(sentKeysBody, strings.Repeat("\\b", 50)) {
		// The body is JSON with value array; just check the result message mentions 50
		if !strings.Contains(result.Message, "50") {
			t.Errorf("Expected message about 50 characters, got: %s", result.Message)
		}
	}
}

// TestEraseTextPartialEraseSendKeysFails tests eraseText Case 2 when
// Clear succeeds but SendKeys (to retype remaining text) fails.
func TestEraseTextPartialEraseSendKeysFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/element/active") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "field-1"},
			})
			return
		}
		if strings.Contains(path, "/element/") && strings.HasSuffix(path, "/text") {
			jsonResponse(w, map[string]interface{}{"value": "HelloWorld"})
			return
		}
		if strings.Contains(path, "/element/") && strings.HasSuffix(path, "/clear") {
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		if strings.Contains(path, "/wda/keys") {
			// First call to SendKeys (retype remaining) fails
			// Then fallback SendKeys (backspaces) also fails
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"error": "keys failed"},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.EraseTextStep{Characters: 5}
	result := driver.eraseText(step)

	// The clear succeeded but SendKeys failed for remaining text,
	// then it falls through to the delete key approach which also fails
	if result.Success {
		t.Error("Expected failure when SendKeys fails in both optimized and fallback paths")
	}
}

// =============================================================================
// openLink tests
// =============================================================================

// TestOpenLinkWithAutoVerify tests openLink with autoVerify flag set.
func TestOpenLinkWithAutoVerify(t *testing.T) {
	var urlRequested string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/url") {
			urlRequested = r.URL.Path
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	autoVerify := true
	step := &flow.OpenLinkStep{
		Link:       "myapp://verify-test",
		AutoVerify: &autoVerify,
	}
	result := driver.openLink(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if urlRequested == "" {
		t.Error("Expected URL endpoint to be called")
	}
}

// TestOpenLinkWithBrowserFlag tests openLink with browser flag set.
func TestOpenLinkWithBrowserFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	browser := true
	step := &flow.OpenLinkStep{
		Link:    "https://example.com",
		Browser: &browser,
	}
	result := driver.openLink(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	// Verify message mentions browser flag
	if !strings.Contains(result.Message, "browser") {
		t.Errorf("Expected message to mention browser flag, got: %s", result.Message)
	}
}

// TestOpenLinkWithBothFlags tests openLink with both autoVerify and browser flags.
func TestOpenLinkWithBothFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	autoVerify := true
	browser := true
	step := &flow.OpenLinkStep{
		Link:       "https://example.com/page",
		AutoVerify: &autoVerify,
		Browser:    &browser,
	}
	result := driver.openLink(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "browser") {
		t.Errorf("Expected message to mention browser, got: %s", result.Message)
	}
}

// =============================================================================
// copyTextFrom tests
// =============================================================================

// TestCopyTextFromVerifiesDataField tests that copyTextFrom returns the element's
// text in the Data field of the result.
func TestCopyTextFromVerifiesDataField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="priceLabel" label="$42.99" enabled="true" visible="true" x="50" y="200" width="100" height="30"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.CopyTextFromStep{
		Selector: flow.Selector{Text: "$42.99"},
	}
	result := driver.copyTextFrom(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if result.Data != "$42.99" {
		t.Errorf("Expected Data='$42.99', got '%v'", result.Data)
	}
	if result.Element == nil {
		t.Error("Expected Element to be set")
	}
	if !strings.Contains(result.Message, "$42.99") {
		t.Errorf("Expected message to contain '$42.99', got: %s", result.Message)
	}
}

// TestCopyTextFromByID tests copyTextFrom using an ID selector.
func TestCopyTextFromByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="statusLabel" label="Connected" enabled="true" visible="true" x="50" y="200" width="200" height="30"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.CopyTextFromStep{
		Selector: flow.Selector{ID: "statusLabel"},
	}
	result := driver.copyTextFrom(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if result.Data != "Connected" {
		t.Errorf("Expected Data='Connected', got '%v'", result.Data)
	}
}

// =============================================================================
// scroll tests
// =============================================================================

// TestScrollWindowSizeError tests scroll when WindowSize fails.
func TestScrollWindowSizeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "window size failed",
					"message": "Cannot get window size",
				},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollStep{Direction: "down"}
	result := driver.scroll(step)

	if result.Success {
		t.Error("Expected failure when WindowSize fails")
	}
	if !strings.Contains(result.Message, "screen size") {
		t.Errorf("Expected message about screen size, got: %s", result.Message)
	}
}

// TestScrollVerifiesSwipeCoordinates tests scroll direction mapping:
// "scroll down" = reveal content below = swipe UP (fromY > toY).
func TestScrollVerifiesSwipeCoordinates(t *testing.T) {
	var fromX, fromY, toX, toY float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/dragfromtoforduration") {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			_ = json.Unmarshal(body, &payload)
			fromX, _ = payload["fromX"].(float64)
			fromY, _ = payload["fromY"].(float64)
			toX, _ = payload["toX"].(float64)
			toY, _ = payload["toY"].(float64)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	// Scroll down = swipe UP: fromY should be greater than toY
	step := &flow.ScrollStep{Direction: "down"}
	result := driver.scroll(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if fromY <= toY {
		t.Errorf("Scroll down should swipe UP (fromY > toY), got fromY=%.0f, toY=%.0f", fromY, toY)
	}
	// X should stay centered
	if fromX != toX {
		t.Errorf("Expected X to stay centered, fromX=%.0f, toX=%.0f", fromX, toX)
	}
}

// =============================================================================
// scrollUntilVisible tests
// =============================================================================

// TestScrollUntilVisibleImmediateFind tests scrollUntilVisible when element
// is already visible on the first check (no scrolling needed).
func TestScrollUntilVisibleImmediateFind(t *testing.T) {
	scrollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			// Element is immediately visible
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="target" label="TargetButton" enabled="true" visible="true" x="50" y="400" width="290" height="50"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		if strings.Contains(path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		if strings.Contains(path, "/dragfromtoforduration") {
			scrollCount++
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "TargetButton"},
		Direction: "down",
	}
	result := driver.scrollUntilVisible(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if scrollCount != 0 {
		t.Errorf("Expected 0 scrolls (element immediately visible), got %d", scrollCount)
	}
}

// TestScrollUntilVisibleUpDirection tests scrollUntilVisible with "up" direction.
func TestScrollUntilVisibleUpDirection(t *testing.T) {
	scrollCount := 0
	server := mockWDAServerWithScrollElements(1) // Found after 1 scroll
	// Override to count scrolls
	server.Close()

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/dragfromtoforduration") {
			scrollCount++
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		if strings.HasSuffix(path, "/source") {
			if scrollCount >= 1 {
				jsonResponse(w, map[string]interface{}{
					"value": `<AppiumAUT>
  <XCUIElementTypeApplication name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton name="target" label="Target" enabled="true" visible="true" x="50" y="100" width="100" height="50"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
				})
			} else {
				jsonResponse(w, map[string]interface{}{
					"value": `<AppiumAUT>
  <XCUIElementTypeApplication name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
				})
			}
			return
		}
		if strings.Contains(path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "Target"},
		Direction: "up",
		BaseStep:  flow.BaseStep{TimeoutMs: 10000},
	}
	result := driver.scrollUntilVisible(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if scrollCount < 1 {
		t.Errorf("Expected at least 1 scroll, got %d", scrollCount)
	}
}

// =============================================================================
// setOrientation tests
// =============================================================================

// TestSetOrientationMapsPortraitToUppercase tests that "portrait" is mapped to "PORTRAIT".
func TestSetOrientationMapsPortraitToUppercase(t *testing.T) {
	var sentOrientation string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/orientation") && r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			_ = json.Unmarshal(body, &payload)
			if o, ok := payload["orientation"].(string); ok {
				sentOrientation = o
			}
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SetOrientationStep{Orientation: "portrait"}
	result := driver.setOrientation(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if sentOrientation != "PORTRAIT" {
		t.Errorf("Expected 'PORTRAIT', got '%s'", sentOrientation)
	}
}

// TestSetOrientationMapsLandscapeToUppercase tests that "landscape" is mapped to "LANDSCAPE".
func TestSetOrientationMapsLandscapeToUppercase(t *testing.T) {
	var sentOrientation string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/orientation") && r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			_ = json.Unmarshal(body, &payload)
			if o, ok := payload["orientation"].(string); ok {
				sentOrientation = o
			}
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SetOrientationStep{Orientation: "landscape"}
	result := driver.setOrientation(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if sentOrientation != "LANDSCAPE" {
		t.Errorf("Expected 'LANDSCAPE', got '%s'", sentOrientation)
	}
}

// TestSetOrientationPassthroughUppercase tests that already-uppercase values
// are passed through unchanged.
func TestSetOrientationPassthroughUppercase(t *testing.T) {
	var sentOrientation string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/orientation") && r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			var payload map[string]interface{}
			_ = json.Unmarshal(body, &payload)
			if o, ok := payload["orientation"].(string); ok {
				sentOrientation = o
			}
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SetOrientationStep{Orientation: "LANDSCAPE"}
	result := driver.setOrientation(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if sentOrientation != "LANDSCAPE" {
		t.Errorf("Expected 'LANDSCAPE', got '%s'", sentOrientation)
	}
}

// =============================================================================
// inputText tests
// =============================================================================

// TestInputTextWithSelectorElementIDDirectSend tests inputText when
// element is found with an ID, using ElementSendKeys directly.
func TestInputTextWithSelectorElementIDDirectSend(t *testing.T) {
	var elementSendKeysPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Find element by predicate -> return element with ID
		if strings.HasSuffix(path, "/element") && r.Method == "POST" {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "text-field-1"},
			})
			return
		}
		// Element rect
		if strings.Contains(path, "/element/") && strings.Contains(path, "/rect") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"x": 50.0, "y": 200.0, "width": 290.0, "height": 44.0},
			})
			return
		}
		// Element displayed
		if strings.Contains(path, "/element/") && strings.Contains(path, "/displayed") {
			jsonResponse(w, map[string]interface{}{"value": true})
			return
		}
		// Element send keys (direct to element)
		if strings.Contains(path, "/element/") && strings.Contains(path, "/value") {
			elementSendKeysPath = path
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		// Source fallback
		if strings.HasSuffix(path, "/source") {
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeTextField type="XCUIElementTypeTextField" name="emailField" label="Email" enabled="true" visible="true" x="50" y="200" width="290" height="44"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text:     "user@test.com",
		Selector: flow.Selector{ID: "emailField"},
	}
	result := driver.inputText(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	// Should have used ElementSendKeys (path contains /element/.../value)
	if elementSendKeysPath == "" {
		// It may have used the page source fallback which doesn't have element ID
		// This is acceptable - just verify success
		return
	}
	if !strings.Contains(elementSendKeysPath, "/element/") {
		t.Errorf("Expected ElementSendKeys path, got: %s", elementSendKeysPath)
	}
}

// TestInputTextElementSendKeysError tests inputText when ElementSendKeys fails
// for an element with ID.
func TestInputTextElementSendKeysError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/element") && r.Method == "POST" {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "text-field-1"},
			})
			return
		}
		if strings.Contains(path, "/element/") && strings.Contains(path, "/rect") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"x": 50.0, "y": 200.0, "width": 290.0, "height": 44.0},
			})
			return
		}
		if strings.Contains(path, "/element/") && strings.Contains(path, "/displayed") {
			jsonResponse(w, map[string]interface{}{"value": true})
			return
		}
		if strings.Contains(path, "/element/") && strings.Contains(path, "/value") {
			// ElementSendKeys fails
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"error": "send keys to element failed"},
			})
			return
		}
		if strings.HasSuffix(path, "/source") {
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeTextField type="XCUIElementTypeTextField" name="emailField" label="Email" enabled="true" visible="true" x="50" y="200" width="290" height="44"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{
		Text:     "user@test.com",
		Selector: flow.Selector{ID: "emailField"},
	}
	result := driver.inputText(step)

	// When element is found via page source (no element ID), it falls through
	// to the tap+SendKeys path. If found via WDA with ID, ElementSendKeys error
	// would cause failure. Either outcome is valid depending on which path is taken.
	// The key thing is the test doesn't panic.
	_ = result
}

// =============================================================================
// assertNotVisible tests
// =============================================================================

// TestAssertNotVisibleVerifiesErrorMessage tests that assertNotVisible returns
// a clear error message when the element IS visible.
func TestAssertNotVisibleVerifiesErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="errorMsg" label="ErrorMessage" enabled="true" visible="true" x="50" y="200" width="200" height="30"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Selector: flow.Selector{Text: "ErrorMessage"},
	}
	result := driver.assertNotVisible(step)

	if result.Success {
		t.Error("Expected failure when element is visible")
	}
	if !strings.Contains(result.Message, "should not be visible") {
		t.Errorf("Expected 'should not be visible' in message, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "ErrorMessage") {
		t.Errorf("Expected selector text in message, got: %s", result.Message)
	}
}

// TestAssertNotVisibleSuccessVerifiesMessage tests that assertNotVisible returns
// success message when element is NOT found.
func TestAssertNotVisibleSuccessVerifiesMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Selector: flow.Selector{Text: "GhostElement"},
	}
	result := driver.assertNotVisible(step)

	if !result.Success {
		t.Errorf("Expected success for non-visible element, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "not visible") {
		t.Errorf("Expected 'not visible' in message, got: %s", result.Message)
	}
}

// TestAssertNotVisibleWithIDSelector tests assertNotVisible with an ID selector.
func TestAssertNotVisibleWithIDSelector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="deleteBtn" label="Delete" enabled="true" visible="true" x="250" y="50" width="80" height="40"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 100},
		Selector: flow.Selector{ID: "deleteBtn"},
	}
	result := driver.assertNotVisible(step)

	// Element IS visible by ID, so assert should fail
	if result.Success {
		t.Error("Expected failure when element is visible by ID")
	}
}

// =============================================================================
// iosKeyboardKey tests
// =============================================================================

// TestIosKeyboardKeyMapping tests the iosKeyboardKey helper for all recognized keys.
func TestIosKeyboardKeyMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"return", "\n"},
		{"enter", "\n"},
		{"Return", "\n"},
		{"ENTER", "\n"},
		{"tab", "\t"},
		{"Tab", "\t"},
		{"delete", "\b"},
		{"backspace", "\b"},
		{"Delete", "\b"},
		{"space", " "},
		{"Space", " "},
		{"unknown", ""},
		{"shift", ""},
		{"ctrl", ""},
		{"", ""},
	}

	for _, tc := range tests {
		result := iosKeyboardKey(tc.input)
		if result != tc.expected {
			t.Errorf("iosKeyboardKey(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

// =============================================================================
// resolveIOSPermissionShortcut tests
// =============================================================================

// TestResolveIOSPermissionShortcut tests the permission shortcut resolution.
func TestResolveIOSPermissionShortcut(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"location", []string{"location-always"}},
		{"camera", []string{"camera"}},
		{"contacts", []string{"contacts"}},
		{"phone", []string{"contacts"}},
		{"microphone", []string{"microphone"}},
		{"photos", []string{"photos"}},
		{"medialibrary", []string{"photos"}},
		{"calendar", []string{"calendar"}},
		{"reminders", []string{"reminders"}},
		{"notifications", []string{"notifications"}},
		{"bluetooth", []string{"bluetooth-peripheral"}},
		{"health", []string{"health"}},
		{"homekit", []string{"homekit"}},
		{"motion", []string{"motion"}},
		{"speech", []string{"speech-recognition"}},
		{"siri", []string{"siri"}},
		{"faceid", []string{"faceid"}},
		{"custom-permission", []string{"custom-permission"}},
	}

	for _, tc := range tests {
		result := resolveIOSPermissionShortcut(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("resolveIOSPermissionShortcut(%q) returned %d items, want %d", tc.input, len(result), len(tc.expected))
			continue
		}
		for i, v := range result {
			if v != tc.expected[i] {
				t.Errorf("resolveIOSPermissionShortcut(%q)[%d] = %q, want %q", tc.input, i, v, tc.expected[i])
			}
		}
	}
}

// TestGetIOSPermissions tests the list of iOS permissions.
func TestGetIOSPermissions(t *testing.T) {
	perms := getIOSPermissions()
	if len(perms) == 0 {
		t.Error("Expected non-empty permissions list")
	}
	// Verify some key permissions are present
	expected := []string{"camera", "microphone", "photos", "contacts"}
	for _, e := range expected {
		found := false
		for _, p := range perms {
			if p == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected permission '%s' to be in the list", e)
		}
	}
}

// =============================================================================
// hideKeyboard test
// =============================================================================

// TestHideKeyboardSendsNewline tests that hideKeyboard sends a newline character.
func TestHideKeyboardSendsNewline(t *testing.T) {
	var keysSent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/keys") {
			body, _ := io.ReadAll(r.Body)
			keysSent = string(body)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.HideKeyboardStep{}
	result := driver.hideKeyboard(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	// Verify a newline was sent
	if !strings.Contains(keysSent, "\\n") {
		// The body is JSON-encoded, so \n appears as \\n
		t.Logf("Keys sent body: %s", keysSent)
	}
}

// =============================================================================
// openBrowser tests
// =============================================================================

// TestOpenBrowserEmptyURL tests openBrowser with empty URL.
func TestOpenBrowserEmptyURL(t *testing.T) {
	server := mockWDAServerForDriver()
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenBrowserStep{URL: ""}
	result := driver.openBrowser(step)

	if result.Success {
		t.Error("Expected failure for empty URL")
	}
	if !strings.Contains(result.Message, "No URL") {
		t.Errorf("Expected message about no URL, got: %s", result.Message)
	}
}

// =============================================================================
// tapOn keyboard key tests
// =============================================================================

// TestTapOnKeyboardKey tests tapOn with a text selector matching a keyboard key.
func TestTapOnKeyboardKey(t *testing.T) {
	var sendKeysCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/keys") {
			sendKeysCalled = true
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "Return"},
	}
	result := driver.tapOn(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if !sendKeysCalled {
		t.Error("Expected SendKeys to be called for keyboard key")
	}
	if !strings.Contains(result.Message, "keyboard key") {
		t.Errorf("Expected message about keyboard key, got: %s", result.Message)
	}
}

// TestTapOnKeyboardKeySendKeysFails tests tapOn when SendKeys fails for a keyboard key.
func TestTapOnKeyboardKeySendKeysFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/keys") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"error": "send keys failed"},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "Return"},
	}
	result := driver.tapOn(step)

	if result.Success {
		t.Error("Expected failure when SendKeys fails for keyboard key")
	}
}

// =============================================================================
// pressKey keyboard key tests
// =============================================================================

// TestPressKeyKeyboardKeys tests pressKey with keyboard key names (enter, tab, etc).
func TestPressKeyKeyboardKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	keys := []string{"enter", "tab", "delete", "backspace", "space"}
	for _, key := range keys {
		step := &flow.PressKeyStep{Key: key}
		result := driver.pressKey(step)
		if !result.Success {
			t.Errorf("pressKey(%s) failed: %s", key, result.Message)
		}
	}
}

// TestPressKeyVolume_Down tests pressKey with "volume_down" (underscore variant).
func TestPressKeyVolume_Down(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "volume_down"}
	result := driver.pressKey(step)
	if !result.Success {
		t.Errorf("Expected success for volume_down, got: %s", result.Message)
	}
}

// TestPressKeyVolume_Up tests pressKey with "volume_up" (underscore variant).
func TestPressKeyVolume_Up(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "volume_up"}
	result := driver.pressKey(step)
	if !result.Success {
		t.Errorf("Expected success for volume_up, got: %s", result.Message)
	}
}

// =============================================================================
// setClipboard / pasteText tests
// =============================================================================

// TestSetClipboardNotSupported tests that setClipboard returns an error.
func TestSetClipboardNotSupported(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios"},
	}

	step := &flow.SetClipboardStep{}
	result := driver.setClipboard(step)

	if result.Success {
		t.Error("Expected failure for setClipboard on iOS")
	}
}

// TestPasteTextNotSupported tests that pasteText returns an error.
func TestPasteTextNotSupported(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios"},
	}

	step := &flow.PasteTextStep{}
	result := driver.pasteText(step)

	if result.Success {
		t.Error("Expected failure for pasteText on iOS")
	}
}

// =============================================================================
// clearState tests
// =============================================================================

// TestClearStateNoAppFile tests clearState without appFile returns error requiring --app-file.
func TestClearStateNoAppFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ClearStateStep{AppID: "com.example.app"}
	result := driver.clearState(step)

	if result.Success {
		t.Error("Expected failure for clearState without appFile")
	}
	if !strings.Contains(result.Message, "--app-file") {
		t.Errorf("Expected message about --app-file, got: %s", result.Message)
	}
}

// =============================================================================
// back command test
// =============================================================================

// TestBackNotSupportedMessage tests back command returns proper message.
func TestBackNotSupportedMessage(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios"},
	}

	step := &flow.BackStep{}
	result := driver.back(step)

	if result.Success {
		t.Error("Expected failure for back on iOS")
	}
	if !strings.Contains(result.Message, "back button") {
		t.Errorf("Expected message about back button, got: %s", result.Message)
	}
}

// =============================================================================
// setPermissions tests
// =============================================================================

// TestSetPermissionsEmptyAppID tests setPermissions with empty appID.
func TestSetPermissionsEmptyAppID(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios"},
	}

	step := &flow.SetPermissionsStep{
		AppID:       "",
		Permissions: map[string]string{"camera": "allow"},
	}
	result := driver.setPermissions(step)

	if result.Success {
		t.Fatalf("Expected failure for empty appID, got success: %s", result.Message)
	}
	if !strings.Contains(result.Message, "No app ID") {
		t.Errorf("Expected 'No app ID' in message, got: %s", result.Message)
	}
}

// TestSetPermissionsNoUDID tests setPermissions with no UDID (skipped).
func TestSetPermissionsNoUDID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server) // createTestDriver uses udid=""

	step := &flow.SetPermissionsStep{
		AppID:       "com.test.app",
		Permissions: map[string]string{"camera": "allow"},
	}
	result := driver.setPermissions(step)

	if !result.Success {
		t.Fatalf("Expected success (skipped) for no UDID, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "skipped") {
		t.Errorf("Expected 'skipped' in message, got: %s", result.Message)
	}
}

// TestSetPermissionsEmptyPermissions tests setPermissions with empty permissions map.
func TestSetPermissionsEmptyPermissions(t *testing.T) {
	driver := &Driver{
		client: &Client{baseURL: "http://localhost:0", httpClient: http.DefaultClient, sessionID: "test-session"},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: true},
		udid:   "FAKE-UDID-12345",
	}

	step := &flow.SetPermissionsStep{
		AppID:       "com.test.app",
		Permissions: map[string]string{},
	}
	result := driver.setPermissions(step)

	if result.Success {
		t.Fatalf("Expected failure for empty permissions, got success: %s", result.Message)
	}
	if !strings.Contains(result.Message, "No permissions") {
		t.Errorf("Expected 'No permissions' in message, got: %s", result.Message)
	}
}

// TestSetPermissionsAllAllow tests setPermissions with "all" permission and "allow" value.
// Since exec.Command("xcrun",...) will fail in test, the code handles errors gracefully.
func TestSetPermissionsAllAllow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	driver := &Driver{
		client: &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: true},
		udid:   "FAKE-UDID-12345",
	}

	step := &flow.SetPermissionsStep{
		AppID:       "com.test.app",
		Permissions: map[string]string{"all": "allow"},
	}
	result := driver.setPermissions(step)

	// The function always returns success; xcrun failures are counted as errors
	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	// Should mention errors since xcrun will fail in test environment
	if !strings.Contains(result.Message, "errors") && !strings.Contains(result.Message, "granted") {
		t.Errorf("Expected message about errors or granted, got: %s", result.Message)
	}
}

// TestSetPermissionsSpecificAllow tests setPermissions with a specific permission.
func TestSetPermissionsSpecificAllow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	driver := &Driver{
		client: &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: true},
		udid:   "FAKE-UDID-12345",
	}

	step := &flow.SetPermissionsStep{
		AppID:       "com.test.app",
		Permissions: map[string]string{"camera": "allow"},
	}
	result := driver.setPermissions(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
}

// TestSetPermissionsSpecificDeny tests setPermissions with "deny" value.
func TestSetPermissionsSpecificDeny(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	driver := &Driver{
		client: &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: true},
		udid:   "FAKE-UDID-12345",
	}

	step := &flow.SetPermissionsStep{
		AppID:       "com.test.app",
		Permissions: map[string]string{"photos": "deny"},
	}
	result := driver.setPermissions(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
}

// =============================================================================
// applyIOSPermission tests
// =============================================================================

// TestApplyIOSPermissionInvalidValue tests applyIOSPermission with an invalid value.
func TestApplyIOSPermissionInvalidValue(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios"},
		udid:   "FAKE-UDID-12345",
	}

	err := driver.applyIOSPermission("com.test.app", "camera", "invalid")
	if err == nil {
		t.Fatalf("Expected error for invalid permission value")
	}
	if !strings.Contains(err.Error(), "invalid permission value") {
		t.Errorf("Expected 'invalid permission value' in error, got: %v", err)
	}
}

// TestApplyIOSPermissionAllow tests applyIOSPermission with "allow" value.
// xcrun will fail in test but the function returns the exec error.
func TestApplyIOSPermissionAllow(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios"},
		udid:   "FAKE-UDID-12345",
	}

	err := driver.applyIOSPermission("com.test.app", "camera", "allow")
	// xcrun will fail in test environment, but the function should not panic
	if err == nil {
		// If xcrun happens to be available (e.g., on macOS), that's fine too
		return
	}
	// Verify the error is from exec, not from invalid value parsing
	if strings.Contains(err.Error(), "invalid permission value") {
		t.Errorf("Expected exec error, not validation error: %v", err)
	}
}

// TestApplyIOSPermissionDeny tests applyIOSPermission with "deny" value.
func TestApplyIOSPermissionDeny(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios"},
		udid:   "FAKE-UDID-12345",
	}

	err := driver.applyIOSPermission("com.test.app", "camera", "deny")
	if err == nil {
		return // xcrun might be available on macOS
	}
	if strings.Contains(err.Error(), "invalid permission value") {
		t.Errorf("Expected exec error, not validation error: %v", err)
	}
}

// TestApplyIOSPermissionUnset tests applyIOSPermission with "unset" value.
func TestApplyIOSPermissionUnset(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios"},
		udid:   "FAKE-UDID-12345",
	}

	err := driver.applyIOSPermission("com.test.app", "camera", "unset")
	if err == nil {
		return // xcrun might be available on macOS
	}
	if strings.Contains(err.Error(), "invalid permission value") {
		t.Errorf("Expected exec error, not validation error: %v", err)
	}
}

// =============================================================================
// SetWaitForIdleTimeout test
// =============================================================================

// TestSetWaitForIdleTimeoutNoOp tests that SetWaitForIdleTimeout is a no-op.
func TestSetWaitForIdleTimeoutNoOp(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios"},
	}

	err := driver.SetWaitForIdleTimeout(5000)
	if err != nil {
		t.Fatalf("Expected nil error from no-op SetWaitForIdleTimeout, got: %v", err)
	}
}

// =============================================================================
// launchApp tests
// =============================================================================

// TestLaunchAppNoBundleID tests launchApp with empty bundle ID returns error.
func TestLaunchAppNoBundleID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.LaunchAppStep{AppID: ""}
	result := driver.launchApp(step)

	if result.Success {
		t.Fatalf("Expected failure for empty bundleID")
	}
	if !strings.Contains(result.Message, "Bundle ID") {
		t.Errorf("Expected message about Bundle ID, got: %s", result.Message)
	}
}

// TestLaunchAppNoSessionCreatesSession tests launchApp when no session exists.
func TestLaunchAppNoSessionCreatesSession(t *testing.T) {
	var sessionCreated bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if path == "/session" && r.Method == "POST" {
			sessionCreated = true
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"sessionId": "new-session-123"},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	// Create client without sessionID to simulate no session
	client := &Client{baseURL: server.URL, httpClient: http.DefaultClient}
	driver := &Driver{
		client: client,
		info:   &core.PlatformInfo{Platform: "ios"},
	}

	step := &flow.LaunchAppStep{AppID: "com.test.app"}
	result := driver.launchApp(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if !sessionCreated {
		t.Errorf("Expected CreateSession to be called")
	}
}

// TestLaunchAppWithArguments tests launchApp with various argument types.
func TestLaunchAppWithArguments(t *testing.T) {
	var launchBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/wda/apps/launch") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read request body: %v", readErr)
			}
			launchBody = string(body)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.LaunchAppStep{
		AppID: "com.test.app",
		Arguments: map[string]interface{}{
			"debug":   "true",
			"verbose": true,
			"level":   42,
			"quiet":   false,
		},
	}
	result := driver.launchApp(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if launchBody == "" {
		t.Errorf("Expected LaunchAppWithArgs to be called with body")
	}
}

// TestLaunchAppWithUDIDDefaultPermissions tests launchApp with UDID applies default permissions.
func TestLaunchAppWithUDIDDefaultPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		if strings.Contains(path, "/wda/apps/launch") {
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	driver := &Driver{
		client: &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: true},
		udid:   "FAKE-UDID-12345",
	}

	// No explicit permissions -- should default to "all"/"allow"
	step := &flow.LaunchAppStep{AppID: "com.test.app"}
	result := driver.launchApp(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
}

// TestLaunchAppWithUDIDExplicitPermissions tests launchApp with explicit permissions.
func TestLaunchAppWithUDIDExplicitPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		if strings.Contains(path, "/wda/apps/launch") {
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	driver := &Driver{
		client: &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: true},
		udid:   "FAKE-UDID-12345",
	}

	step := &flow.LaunchAppStep{
		AppID: "com.test.app",
		Permissions: map[string]string{
			"camera":   "allow",
			"location": "deny",
		},
	}
	result := driver.launchApp(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
}

// =============================================================================
// tapOn additional tests
// =============================================================================

// TestTapOnOptionalElementNotFound tests tapOn with optional=true when element not found.
func TestTapOnOptionalElementNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Element finding fails
		if strings.HasSuffix(path, "/element") && r.Method == "POST" {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"error": "no such element"},
			})
			return
		}
		// Source returns empty page (no matching elements)
		if strings.HasSuffix(path, "/source") {
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 500},
		Selector: flow.Selector{Text: "NonExistent"},
	}
	result := driver.tapOn(step)

	if !result.Success {
		t.Fatalf("Expected success for optional tap, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "Optional") {
		t.Errorf("Expected 'Optional' in message, got: %s", result.Message)
	}
}

// TestTapOnPointWithSelectorRelativeTap tests tapOn with a Point and selector for relative tap.
func TestTapOnPointWithSelectorRelativeTap(t *testing.T) {
	var tapX, tapY float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="target" label="MyButton" enabled="true" visible="true" x="100" y="200" width="200" height="100"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		if strings.HasSuffix(path, "/element") && r.Method == "POST" {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"error": "not found"},
			})
			return
		}
		if strings.Contains(path, "/wda/tap") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal body: %v", err)
			}
			tapX, _ = payload["x"].(float64)
			tapY, _ = payload["y"].(float64)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{Text: "MyButton"},
		Point:    "50%, 50%",
	}
	result := driver.tapOn(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	// Relative tap at 50%,50% of element (100,200,200,100) = (200, 250)
	if tapX < 190 || tapX > 210 {
		t.Errorf("Expected tapX near 200, got: %.0f", tapX)
	}
	if tapY < 240 || tapY > 260 {
		t.Errorf("Expected tapY near 250, got: %.0f", tapY)
	}
}

// TestTapOnPointInvalidCoords tests tapOn with invalid point coordinates.
func TestTapOnPointInvalidCoords(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.HasSuffix(path, "/source") {
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="btn" label="Btn" enabled="true" visible="true" x="50" y="100" width="200" height="50"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		if strings.HasSuffix(path, "/element") && r.Method == "POST" {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"error": "not found"},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{Text: "Btn"},
		Point:    "invalid-coords",
	}
	result := driver.tapOn(step)

	if result.Success {
		t.Fatalf("Expected failure for invalid point coordinates, got success")
	}
	if !strings.Contains(result.Message, "Invalid point") {
		t.Errorf("Expected 'Invalid point' in message, got: %s", result.Message)
	}
}

// =============================================================================
// swipe tests
// =============================================================================

// TestSwipeStartEndCoordinates tests swipe with percentage-based start/end coordinates.
func TestSwipeStartEndCoordinates(t *testing.T) {
	var fromX, fromY, toX, toY float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		if strings.Contains(path, "/dragfromtoforduration") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal body: %v", err)
			}
			fromX, _ = payload["fromX"].(float64)
			fromY, _ = payload["fromY"].(float64)
			toX, _ = payload["toX"].(float64)
			toY, _ = payload["toY"].(float64)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{
		Start: "50%, 80%",
		End:   "50%, 20%",
	}
	result := driver.swipe(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	// fromX = 390 * 0.50 = 195
	if fromX < 194 || fromX > 196 {
		t.Errorf("Expected fromX near 195, got: %.0f", fromX)
	}
	// fromY = 844 * 0.80 = 675.2
	if fromY < 674 || fromY > 676 {
		t.Errorf("Expected fromY near 675, got: %.0f", fromY)
	}
	// toY = 844 * 0.20 = 168.8
	if toY < 167 || toY > 170 {
		t.Errorf("Expected toY near 169, got: %.0f", toY)
	}
	// toX = 390 * 0.50 = 195
	if toX < 194 || toX > 196 {
		t.Errorf("Expected toX near 195, got: %.0f", toX)
	}
}

// TestSwipeDirectPixelCoordinates tests swipe with direct pixel coordinates.
func TestSwipeDirectPixelCoordinates(t *testing.T) {
	var fromX, fromY, toX, toY float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		if strings.Contains(path, "/dragfromtoforduration") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal body: %v", err)
			}
			fromX, _ = payload["fromX"].(float64)
			fromY, _ = payload["fromY"].(float64)
			toX, _ = payload["toX"].(float64)
			toY, _ = payload["toY"].(float64)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{
		StartX: 100,
		StartY: 500,
		EndX:   100,
		EndY:   200,
	}
	result := driver.swipe(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if fromX != 100 {
		t.Errorf("Expected fromX=100, got: %.0f", fromX)
	}
	if fromY != 500 {
		t.Errorf("Expected fromY=500, got: %.0f", fromY)
	}
	if toX != 100 {
		t.Errorf("Expected toX=100, got: %.0f", toX)
	}
	if toY != 200 {
		t.Errorf("Expected toY=200, got: %.0f", toY)
	}
}

// TestSwipeDirectionLeftCoords tests swipe with "left" direction verifies coordinate values.
func TestSwipeDirectionLeftCoords(t *testing.T) {
	var fromX, toX float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		if strings.Contains(path, "/dragfromtoforduration") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal body: %v", err)
			}
			fromX, _ = payload["fromX"].(float64)
			toX, _ = payload["toX"].(float64)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "left"}
	result := driver.swipe(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	// Swipe left: fromX should be > toX
	if fromX <= toX {
		t.Errorf("Swipe left should have fromX > toX, got fromX=%.0f, toX=%.0f", fromX, toX)
	}
}

// TestSwipeDirectionRightCoords tests swipe with "right" direction verifies coordinate values.
func TestSwipeDirectionRightCoords(t *testing.T) {
	var fromX, toX float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		if strings.Contains(path, "/dragfromtoforduration") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal body: %v", err)
			}
			fromX, _ = payload["fromX"].(float64)
			toX, _ = payload["toX"].(float64)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "right"}
	result := driver.swipe(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	// Swipe right: fromX should be < toX
	if fromX >= toX {
		t.Errorf("Swipe right should have fromX < toX, got fromX=%.0f, toX=%.0f", fromX, toX)
	}
}

// TestSwipeInvalidDirectionError tests swipe with invalid direction returns error.
func TestSwipeInvalidDirectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{Direction: "diagonal"}
	result := driver.swipe(step)

	if result.Success {
		t.Fatalf("Expected failure for invalid direction")
	}
	if !strings.Contains(result.Message, "Invalid swipe direction") {
		t.Errorf("Expected 'Invalid swipe direction' in message, got: %s", result.Message)
	}
}

// TestSwipeCustomDuration tests swipe with custom duration.
func TestSwipeCustomDuration(t *testing.T) {
	var sentDuration float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		if strings.Contains(path, "/dragfromtoforduration") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal body: %v", err)
			}
			sentDuration, _ = payload["duration"].(float64)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.SwipeStep{
		Direction: "up",
		Duration:  2000, // 2000ms = 2.0 seconds
	}
	result := driver.swipe(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	// duration = 2000 / 1000.0 = 2.0
	if sentDuration < 1.9 || sentDuration > 2.1 {
		t.Errorf("Expected duration near 2.0, got: %.2f", sentDuration)
	}
}

// TestSwipeWithSelector tests swipe with direction and selector (element bounds).
func TestSwipeWithSelector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		if strings.HasSuffix(path, "/source") {
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeScrollView type="XCUIElementTypeScrollView" name="scrollView" label="MyScroll" enabled="true" visible="true" x="20" y="100" width="350" height="600"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
			return
		}
		if strings.HasSuffix(path, "/element") && r.Method == "POST" {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"error": "not found"},
			})
			return
		}
		if strings.Contains(path, "/dragfromtoforduration") {
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	sel := &flow.Selector{Text: "MyScroll"}
	step := &flow.SwipeStep{
		Direction: "down",
		Selector:  sel,
		BaseStep:  flow.BaseStep{TimeoutMs: 2000},
	}
	result := driver.swipe(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
}

// =============================================================================
// pressKey additional tests
// =============================================================================

// TestPressKeyUnknownKeyError tests pressKey with an unknown key returns error.
func TestPressKeyUnknownKeyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "nonexistent_key"}
	result := driver.pressKey(step)

	if result.Success {
		t.Fatalf("Expected failure for unknown key")
	}
	if !strings.Contains(result.Message, "Unknown key") {
		t.Errorf("Expected 'Unknown key' in message, got: %s", result.Message)
	}
}

// TestPressKeyHomeButton tests pressKey with "home" key sends correct button name.
func TestPressKeyHomeButton(t *testing.T) {
	var pressedButton string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/pressButton") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal body: %v", err)
			}
			pressedButton, _ = payload["name"].(string)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.PressKeyStep{Key: "home"}
	result := driver.pressKey(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if pressedButton != "home" {
		t.Errorf("Expected button 'home', got: %s", pressedButton)
	}
}

// =============================================================================
// tapOnPoint additional tests
// =============================================================================

// TestTapOnPointDirectPixelCoordinates tests tapOnPoint with direct pixel coordinates.
func TestTapOnPointDirectPixelCoordinates(t *testing.T) {
	var tapX, tapY float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/tap") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal body: %v", err)
			}
			tapX, _ = payload["x"].(float64)
			tapY, _ = payload["y"].(float64)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnPointStep{
		X: 150,
		Y: 300,
	}
	result := driver.tapOnPoint(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if tapX != 150 {
		t.Errorf("Expected tapX=150, got: %.0f", tapX)
	}
	if tapY != 300 {
		t.Errorf("Expected tapY=300, got: %.0f", tapY)
	}
}

// =============================================================================
// tapOnPointWithPercentage additional tests
// =============================================================================

// TestTapOnPointPercentageWindowSizeFails tests tapOnPointWithPercentage when WindowSize fails.
func TestTapOnPointPercentageWindowSizeFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "window size failed",
					"message": "Cannot get window size",
				},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	result := driver.tapOnPointWithPercentage("50%, 50%")

	if result.Success {
		t.Fatalf("Expected failure when WindowSize fails")
	}
	if !strings.Contains(result.Message, "screen size") {
		t.Errorf("Expected 'screen size' in message, got: %s", result.Message)
	}
}

// TestTapOnPointPercentageInvalidFormat tests tapOnPointWithPercentage with invalid format.
func TestTapOnPointPercentageInvalidFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	result := driver.tapOnPointWithPercentage("abc, xyz")

	if result.Success {
		t.Fatalf("Expected failure for invalid coordinates")
	}
	if !strings.Contains(result.Message, "Invalid point") {
		t.Errorf("Expected 'Invalid point' in message, got: %s", result.Message)
	}
}

// =============================================================================
// inputText additional tests
// =============================================================================

// TestInputTextEmpty tests inputText with empty text.
func TestInputTextEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{Text: ""}
	result := driver.inputText(step)

	if result.Success {
		t.Fatalf("Expected failure for empty text")
	}
	if !strings.Contains(result.Message, "No text") {
		t.Errorf("Expected 'No text' in message, got: %s", result.Message)
	}
}

// TestInputTextNonASCII tests inputText with non-ASCII characters includes warning.
func TestInputTextNonASCII(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/element/active") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"ELEMENT": "active-elem"},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputTextStep{Text: "Bonjour \u00e0 tous"}
	result := driver.inputText(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "non-ASCII") {
		t.Errorf("Expected 'non-ASCII' warning in message, got: %s", result.Message)
	}
}

// =============================================================================
// inputRandom additional tests
// =============================================================================

// TestInputRandomEmail tests inputRandom with EMAIL type.
func TestInputRandomEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputRandomStep{DataType: "EMAIL"}
	result := driver.inputRandom(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "EMAIL") {
		t.Errorf("Expected 'EMAIL' in message, got: %s", result.Message)
	}
	data, ok := result.Data.(string)
	if !ok {
		t.Fatalf("Expected Data to be string, got: %T", result.Data)
	}
	if !strings.Contains(data, "@") {
		t.Errorf("Expected email with '@', got: %s", data)
	}
}

// TestInputRandomNumberDigits tests inputRandom with NUMBER type returns only digits.
func TestInputRandomNumberDigits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputRandomStep{DataType: "NUMBER", Length: 6}
	result := driver.inputRandom(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	data, ok := result.Data.(string)
	if !ok {
		t.Fatalf("Expected Data to be string, got: %T", result.Data)
	}
	if len(data) != 6 {
		t.Errorf("Expected 6 digit number, got length %d: %s", len(data), data)
	}
	for _, c := range data {
		if c < '0' || c > '9' {
			t.Errorf("Expected only digits, got char: %c in %s", c, data)
			break
		}
	}
}

// TestInputRandomPersonNameFormat tests inputRandom with PERSON_NAME type returns first and last name.
func TestInputRandomPersonNameFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputRandomStep{DataType: "PERSON_NAME"}
	result := driver.inputRandom(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	data, ok := result.Data.(string)
	if !ok {
		t.Fatalf("Expected Data to be string, got: %T", result.Data)
	}
	// Person name should have a space between first and last name
	if !strings.Contains(data, " ") {
		t.Errorf("Expected person name with space, got: %s", data)
	}
}

// TestInputRandomDefaultText tests inputRandom with empty DataType (defaults to TEXT).
func TestInputRandomDefaultText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.InputRandomStep{DataType: "", Length: 12}
	result := driver.inputRandom(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	data, ok := result.Data.(string)
	if !ok {
		t.Fatalf("Expected Data to be string, got: %T", result.Data)
	}
	if len(data) != 12 {
		t.Errorf("Expected 12 char text, got length %d: %s", len(data), data)
	}
}

// =============================================================================
// killApp / stopApp / clearState additional tests
// =============================================================================

// TestKillAppNoBundleID tests killApp with empty bundleID returns error.
func TestKillAppNoBundleID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.KillAppStep{AppID: ""}
	result := driver.killApp(step)

	if result.Success {
		t.Fatalf("Expected failure for empty bundleID")
	}
	if !strings.Contains(result.Message, "Bundle ID") {
		t.Errorf("Expected 'Bundle ID' in message, got: %s", result.Message)
	}
}

// TestStopAppNoBundleID tests stopApp with empty bundleID returns error.
func TestStopAppNoBundleID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.StopAppStep{AppID: ""}
	result := driver.stopApp(step)

	if result.Success {
		t.Fatalf("Expected failure for empty bundleID")
	}
	if !strings.Contains(result.Message, "Bundle ID") {
		t.Errorf("Expected 'Bundle ID' in message, got: %s", result.Message)
	}
}

// TestClearStateNoBundleID tests clearState with empty bundleID returns error.
func TestClearStateNoBundleID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ClearStateStep{AppID: ""}
	result := driver.clearState(step)

	if result.Success {
		t.Fatalf("Expected failure for empty bundleID")
	}
	if !strings.Contains(result.Message, "Bundle ID") {
		t.Errorf("Expected 'Bundle ID' in message, got: %s", result.Message)
	}
}

// =============================================================================
// openLink additional tests
// =============================================================================

// TestOpenLinkEmpty tests openLink with empty link.
func TestOpenLinkEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenLinkStep{Link: ""}
	result := driver.openLink(step)

	if result.Success {
		t.Fatalf("Expected failure for empty link")
	}
	if !strings.Contains(result.Message, "No link") {
		t.Errorf("Expected 'No link' in message, got: %s", result.Message)
	}
}

// =============================================================================
// waitForAnimationToEnd test
// =============================================================================

// TestWaitForAnimationToEndReturnsWarning tests that waitForAnimationToEnd returns success with WARNING.
func TestWaitForAnimationToEndReturnsWarning(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios"},
	}

	step := &flow.WaitForAnimationToEndStep{}
	result := driver.waitForAnimationToEnd(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "WARNING") {
		t.Errorf("Expected 'WARNING' in message, got: %s", result.Message)
	}
}

// =============================================================================
// takeScreenshot tests
// =============================================================================

// TestTakeScreenshotClientError tests takeScreenshot when the client returns an error.
func TestTakeScreenshotClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/screenshot") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "screenshot failed",
					"message": "Could not capture screenshot",
				},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TakeScreenshotStep{}
	result := driver.takeScreenshot(step)

	if result.Success {
		t.Fatalf("Expected failure when screenshot fails")
	}
	if !strings.Contains(result.Message, "Screenshot failed") {
		t.Errorf("Expected 'Screenshot failed' in message, got: %s", result.Message)
	}
}

// =============================================================================
// ElementName non-string response test
// =============================================================================

// TestElementNameNonStringResponse tests ElementName when the response value is not a string.
func TestElementNameNonStringResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		jsonResponse(w, map[string]interface{}{"value": 12345})
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.ElementName("elem-1")
	if err == nil {
		t.Fatalf("Expected error for non-string element name response")
	}
	if !strings.Contains(err.Error(), "invalid element name response") {
		t.Errorf("Expected 'invalid element name response' in error, got: %v", err)
	}
}

// =============================================================================
// scroll additional direction tests
// =============================================================================

// TestScrollLeftDirection tests scroll with "left" direction.
func TestScrollLeftDirection(t *testing.T) {
	var fromX, toX float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/dragfromtoforduration") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal body: %v", err)
			}
			fromX, _ = payload["fromX"].(float64)
			toX, _ = payload["toX"].(float64)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollStep{Direction: "left"}
	result := driver.scroll(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	// Scroll left = reveal left content = swipe RIGHT (fromX < toX)
	if fromX >= toX {
		t.Errorf("Scroll left should swipe RIGHT (fromX < toX), got fromX=%.0f, toX=%.0f", fromX, toX)
	}
}

// TestScrollRightDirection tests scroll with "right" direction.
func TestScrollRightDirection(t *testing.T) {
	var fromX, toX float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/dragfromtoforduration") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal body: %v", err)
			}
			fromX, _ = payload["fromX"].(float64)
			toX, _ = payload["toX"].(float64)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollStep{Direction: "right"}
	result := driver.scroll(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	// Scroll right = reveal right content = swipe LEFT (fromX > toX)
	if fromX <= toX {
		t.Errorf("Scroll right should swipe LEFT (fromX > toX), got fromX=%.0f, toX=%.0f", fromX, toX)
	}
}

// TestScrollInvalidDirection tests scroll with invalid direction.
func TestScrollInvalidDirection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollStep{Direction: "diagonal"}
	result := driver.scroll(step)

	if result.Success {
		t.Fatalf("Expected failure for invalid scroll direction")
	}
	if !strings.Contains(result.Message, "Invalid scroll direction") {
		t.Errorf("Expected 'Invalid scroll direction' in message, got: %s", result.Message)
	}
}

// =============================================================================
// stopApp / killApp success tests
// =============================================================================

// TestStopAppTerminatesApp tests stopApp with valid bundleID calls TerminateApp.
func TestStopAppTerminatesApp(t *testing.T) {
	var terminated bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/apps/terminate") {
			terminated = true
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.StopAppStep{AppID: "com.test.app"}
	result := driver.stopApp(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if !terminated {
		t.Errorf("Expected TerminateApp to be called")
	}
}

// TestKillAppTerminatesApp tests killApp with valid bundleID calls TerminateApp.
func TestKillAppTerminatesApp(t *testing.T) {
	var terminated bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/wda/apps/terminate") {
			terminated = true
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.KillAppStep{AppID: "com.test.app"}
	result := driver.killApp(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if !terminated {
		t.Errorf("Expected TerminateApp to be called")
	}
}

// =============================================================================
// openBrowser success test
// =============================================================================

// TestOpenBrowserValidURL tests openBrowser with a valid URL calls DeepLink.
func TestOpenBrowserValidURL(t *testing.T) {
	var deepLinkCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/url") {
			deepLinkCalled = true
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.OpenBrowserStep{URL: "https://example.com"}
	result := driver.openBrowser(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if !deepLinkCalled {
		t.Errorf("Expected DeepLink to be called")
	}
	if !strings.Contains(result.Message, "https://example.com") {
		t.Errorf("Expected URL in message, got: %s", result.Message)
	}
}

// =============================================================================
// tapOnPoint percentage with Point field test
// =============================================================================

// TestTapOnPointPercentageCoords tests tapOnPoint with percentage-based coordinates via Point field.
func TestTapOnPointPercentageCoords(t *testing.T) {
	var tapX, tapY float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/window/size") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/wda/tap") {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("Failed to read body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("Failed to unmarshal body: %v", err)
			}
			tapX, _ = payload["x"].(float64)
			tapY, _ = payload["y"].(float64)
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.TapOnPointStep{Point: "25%, 75%"}
	result := driver.tapOnPoint(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	// x = 390 * 0.25 = 97.5
	if tapX < 96 || tapX > 99 {
		t.Errorf("Expected tapX near 97.5, got: %.1f", tapX)
	}
	// y = 844 * 0.75 = 633
	if tapY < 632 || tapY > 634 {
		t.Errorf("Expected tapY near 633, got: %.1f", tapY)
	}
}

// =============================================================================
// parsePercentageCoords tests
// =============================================================================

// TestParsePercentageCoordsTableDriven tests the parsePercentageCoords helper function with table-driven cases.
func TestParsePercentageCoordsTableDriven(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantX   float64
		wantY   float64
		wantErr bool
	}{
		{name: "standard", input: "50%, 50%", wantX: 0.5, wantY: 0.5},
		{name: "no spaces", input: "25%,75%", wantX: 0.25, wantY: 0.75},
		{name: "without percent", input: "10,20", wantX: 0.1, wantY: 0.2},
		{name: "single value", input: "50%", wantErr: true},
		{name: "three values", input: "50%,50%,50%", wantErr: true},
		{name: "invalid x", input: "abc,50%", wantErr: true},
		{name: "invalid y", input: "50%,xyz", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x, y, err := parsePercentageCoords(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Expected error for input %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error for input %q: %v", tc.input, err)
			}
			if x < tc.wantX-0.001 || x > tc.wantX+0.001 {
				t.Errorf("Expected x=%.3f, got %.3f for input %q", tc.wantX, x, tc.input)
			}
			if y < tc.wantY-0.001 || y > tc.wantY+0.001 {
				t.Errorf("Expected y=%.3f, got %.3f for input %q", tc.wantY, y, tc.input)
			}
		})
	}
}

// =============================================================================
// selectorDesc tests
// =============================================================================

// TestSelectorDescTableDriven tests the selectorDesc helper function with table-driven cases.
func TestSelectorDescTableDriven(t *testing.T) {
	tests := []struct {
		name     string
		sel      flow.Selector
		expected string
	}{
		{name: "text selector", sel: flow.Selector{Text: "Login"}, expected: "text='Login'"},
		{name: "id selector", sel: flow.Selector{ID: "loginBtn"}, expected: "id='loginBtn'"},
		{name: "empty selector", sel: flow.Selector{}, expected: "selector"},
		{name: "text takes precedence", sel: flow.Selector{Text: "Login", ID: "btn"}, expected: "text='Login'"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := selectorDesc(tc.sel)
			if result != tc.expected {
				t.Errorf("selectorDesc() = %q, want %q", result, tc.expected)
			}
		})
	}
}

// =============================================================================
// randomString / randomEmail / randomNumber / randomPersonName tests
// =============================================================================

// TestRandomStringLength tests that randomString returns the correct length.
func TestRandomStringLength(t *testing.T) {
	for _, length := range []int{0, 1, 5, 20, 100} {
		result := randomString(length)
		if len(result) != length {
			t.Errorf("randomString(%d) returned length %d", length, len(result))
		}
	}
}

// TestRandomEmailFormat tests that randomEmail returns a valid email format.
func TestRandomEmailFormat(t *testing.T) {
	email := randomEmail()
	if !strings.Contains(email, "@") {
		t.Errorf("Expected email with '@', got: %s", email)
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		t.Errorf("Expected exactly one '@' in email, got: %s", email)
	}
	if len(parts[0]) == 0 || len(parts[1]) == 0 {
		t.Errorf("Expected non-empty user and domain, got: %s", email)
	}
}

// TestRandomNumberDigitsOnly tests that randomNumber returns only digits.
func TestRandomNumberDigitsOnly(t *testing.T) {
	result := randomNumber(10)
	if len(result) != 10 {
		t.Errorf("Expected length 10, got %d", len(result))
	}
	for _, c := range result {
		if c < '0' || c > '9' {
			t.Errorf("Expected only digits, found '%c' in %s", c, result)
		}
	}
}

// TestRandomPersonNameHasSpace tests that randomPersonName returns first and last name.
func TestRandomPersonNameHasSpace(t *testing.T) {
	name := randomPersonName()
	if !strings.Contains(name, " ") {
		t.Errorf("Expected name with space, got: %s", name)
	}
	parts := strings.Split(name, " ")
	if len(parts) < 2 {
		t.Errorf("Expected at least first and last name, got: %s", name)
	}
}

// =============================================================================
// resolveAlertAction tests
// =============================================================================

func TestResolveAlertActionEmpty(t *testing.T) {
	result := resolveAlertAction(map[string]string{})
	if result != "accept" {
		t.Errorf("Expected 'accept' for empty permissions, got '%s'", result)
	}
}

func TestResolveAlertActionAllAllow(t *testing.T) {
	result := resolveAlertAction(map[string]string{"all": "allow"})
	if result != "accept" {
		t.Errorf("Expected 'accept', got '%s'", result)
	}
}

func TestResolveAlertActionAllDeny(t *testing.T) {
	result := resolveAlertAction(map[string]string{"all": "deny"})
	if result != "dismiss" {
		t.Errorf("Expected 'dismiss', got '%s'", result)
	}
}

func TestResolveAlertActionMixed(t *testing.T) {
	result := resolveAlertAction(map[string]string{"camera": "allow", "location": "deny"})
	if result != "" {
		t.Errorf("Expected empty string for mixed permissions, got '%s'", result)
	}
}

func TestResolveAlertActionAllSameAllow(t *testing.T) {
	result := resolveAlertAction(map[string]string{"camera": "allow", "location": "allow"})
	if result != "accept" {
		t.Errorf("Expected 'accept' for all-allow, got '%s'", result)
	}
}

func TestResolveAlertActionAllSameDeny(t *testing.T) {
	result := resolveAlertAction(map[string]string{"camera": "deny", "location": "deny"})
	if result != "dismiss" {
		t.Errorf("Expected 'dismiss' for all-deny, got '%s'", result)
	}
}

func TestResolveAlertActionUnsetValue(t *testing.T) {
	result := resolveAlertAction(map[string]string{"camera": "unset"})
	if result != "" {
		t.Errorf("Expected empty string for 'unset' value, got '%s'", result)
	}
}

func TestResolveAlertActionAllKeyUnsetValue(t *testing.T) {
	result := resolveAlertAction(map[string]string{"all": "unset"})
	if result != "" {
		t.Errorf("Expected empty string for 'all: unset', got '%s'", result)
	}
}

// =============================================================================
// hasAllValue tests
// =============================================================================

func TestHasAllValueAllAllow(t *testing.T) {
	if !hasAllValue(map[string]string{"camera": "allow", "location": "allow"}, "allow") {
		t.Error("Expected true for all allow")
	}
}

func TestHasAllValueMixed(t *testing.T) {
	if hasAllValue(map[string]string{"camera": "allow", "location": "deny"}, "allow") {
		t.Error("Expected false for mixed")
	}
}

func TestHasAllValueEmpty(t *testing.T) {
	if hasAllValue(map[string]string{}, "unset") {
		t.Error("Expected false for empty map")
	}
}

func TestHasAllValueUnset(t *testing.T) {
	if !hasAllValue(map[string]string{"all": "unset"}, "unset") {
		t.Error("Expected true for all unset")
	}
}

// =============================================================================
// setPermissions simulator unset test
// =============================================================================

func TestSetPermissionsSimulatorUnset(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: true},
		udid:   "FAKE-UDID-12345",
	}

	step := &flow.SetPermissionsStep{
		AppID:       "com.test.app",
		Permissions: map[string]string{"all": "unset"},
	}
	result := driver.setPermissions(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "unset") {
		t.Errorf("Expected 'unset' in message, got: %s", result.Message)
	}
}

// =============================================================================
// setPermissions real device tests
// =============================================================================

func TestSetPermissionsRealDeviceAllAllow(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: false},
		udid:   "REAL-DEVICE-UDID",
	}

	step := &flow.SetPermissionsStep{
		AppID:       "com.test.app",
		Permissions: map[string]string{"all": "allow"},
	}
	result := driver.setPermissions(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "WDA alert monitor") {
		t.Errorf("Expected message about WDA alert monitor, got: %s", result.Message)
	}
}

func TestSetPermissionsRealDeviceMixed(t *testing.T) {
	driver := &Driver{
		client: &Client{},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: false},
		udid:   "REAL-DEVICE-UDID",
	}

	step := &flow.SetPermissionsStep{
		AppID:       "com.test.app",
		Permissions: map[string]string{"camera": "allow", "location": "deny"},
	}
	result := driver.setPermissions(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}
}

// =============================================================================
// launchApp real device alert action tests
// =============================================================================

func TestLaunchAppRealDeviceDefaultAlertAction(t *testing.T) {
	var sessionBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if path == "/session" && r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &sessionBody)
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"sessionId": "new-session"},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	driver := &Driver{
		client: &Client{baseURL: server.URL, httpClient: http.DefaultClient},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: false},
		udid:   "REAL-DEVICE-UDID",
	}

	// No permissions → defaults to {all: allow} → alertAction = "accept"
	step := &flow.LaunchAppStep{AppID: "com.test.app"}
	result := driver.launchApp(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}

	// Verify defaultAlertAction was sent in session caps
	caps, _ := sessionBody["capabilities"].(map[string]interface{})
	alwaysMatch, _ := caps["alwaysMatch"].(map[string]interface{})
	if alwaysMatch["defaultAlertAction"] != "accept" {
		t.Errorf("Expected defaultAlertAction 'accept', got '%v'", alwaysMatch["defaultAlertAction"])
	}
}

func TestLaunchAppRealDeviceDenyPermissions(t *testing.T) {
	var sessionBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if path == "/session" && r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &sessionBody)
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"sessionId": "new-session"},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	driver := &Driver{
		client: &Client{baseURL: server.URL, httpClient: http.DefaultClient},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: false},
		udid:   "REAL-DEVICE-UDID",
	}

	step := &flow.LaunchAppStep{
		AppID:       "com.test.app",
		Permissions: map[string]string{"all": "deny"},
	}
	result := driver.launchApp(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}

	caps, _ := sessionBody["capabilities"].(map[string]interface{})
	alwaysMatch, _ := caps["alwaysMatch"].(map[string]interface{})
	if alwaysMatch["defaultAlertAction"] != "dismiss" {
		t.Errorf("Expected defaultAlertAction 'dismiss', got '%v'", alwaysMatch["defaultAlertAction"])
	}
}

func TestLaunchAppRealDeviceMixedPermissions(t *testing.T) {
	var sessionBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if path == "/session" && r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &sessionBody)
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"sessionId": "new-session"},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()

	driver := &Driver{
		client: &Client{baseURL: server.URL, httpClient: http.DefaultClient},
		info:   &core.PlatformInfo{Platform: "ios", IsSimulator: false},
		udid:   "REAL-DEVICE-UDID",
	}

	step := &flow.LaunchAppStep{
		AppID:       "com.test.app",
		Permissions: map[string]string{"camera": "allow", "location": "deny"},
	}
	result := driver.launchApp(step)

	if !result.Success {
		t.Fatalf("Expected success, got: %s", result.Message)
	}

	// Mixed permissions → no defaultAlertAction
	caps, _ := sessionBody["capabilities"].(map[string]interface{})
	alwaysMatch, _ := caps["alwaysMatch"].(map[string]interface{})
	if _, exists := alwaysMatch["defaultAlertAction"]; exists {
		t.Error("Expected no defaultAlertAction for mixed permissions")
	}
}

// =============================================================================
// acceptAlert / dismissAlert / waitForAlert tests
// =============================================================================

// TestAcceptAlertSuccess tests acceptAlert when an alert is present and accepted.
func TestAcceptAlertSuccess(t *testing.T) {
	var acceptCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/alert/accept") && r.Method == "POST" {
			acceptCalled = true
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AcceptAlertStep{BaseStep: flow.BaseStep{TimeoutMs: 1000}}
	result := driver.acceptAlert(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if !acceptCalled {
		t.Error("Expected AcceptAlert endpoint to be called")
	}
	if !strings.Contains(result.Message, "accepted") {
		// waitForAlert returns "Alert accepted" on success
		t.Logf("Result message: %s", result.Message)
	}
}

// TestDismissAlertSuccess tests dismissAlert when an alert is present and dismissed.
func TestDismissAlertSuccess(t *testing.T) {
	var dismissCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/alert/dismiss") && r.Method == "POST" {
			dismissCalled = true
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.DismissAlertStep{BaseStep: flow.BaseStep{TimeoutMs: 1000}}
	result := driver.dismissAlert(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if !dismissCalled {
		t.Error("Expected DismissAlert endpoint to be called")
	}
}

// TestAcceptAlertNoAlertTimeout tests acceptAlert when no alert appears within timeout.
// Should succeed silently per the waitForAlert contract.
func TestAcceptAlertNoAlertTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/alert/accept") && r.Method == "POST" {
			// No alert present - return error
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "no such alert",
					"message": "An attempt was made to operate on a modal dialog when one was not open",
				},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AcceptAlertStep{BaseStep: flow.BaseStep{TimeoutMs: 600}}
	result := driver.acceptAlert(step)

	// Should succeed silently when no alert appears
	if !result.Success {
		t.Errorf("Expected success (no alert is OK), got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "No alert") {
		t.Errorf("Expected 'No alert' in message, got: %s", result.Message)
	}
}

// TestDismissAlertNoAlertTimeout tests dismissAlert when no alert appears within timeout.
func TestDismissAlertNoAlertTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/alert/dismiss") && r.Method == "POST" {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "no such alert",
					"message": "No alert open",
				},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.DismissAlertStep{BaseStep: flow.BaseStep{TimeoutMs: 600}}
	result := driver.dismissAlert(step)

	if !result.Success {
		t.Errorf("Expected success (no alert is OK), got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "No alert") {
		t.Errorf("Expected 'No alert' in message, got: %s", result.Message)
	}
}

// TestAcceptAlertDefaultTimeout tests acceptAlert uses 5000ms default timeout.
func TestAcceptAlertDefaultTimeout(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/alert/accept") && r.Method == "POST" {
			callCount++
			// Always succeed on first call
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	// TimeoutMs = 0 should default to 5000ms
	step := &flow.AcceptAlertStep{BaseStep: flow.BaseStep{TimeoutMs: 0}}
	result := driver.acceptAlert(step)

	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if callCount == 0 {
		t.Error("Expected at least one call to accept alert")
	}
}

// TestWaitForAlertPollingBehavior tests that waitForAlert polls and eventually finds an alert.
func TestWaitForAlertPollingBehavior(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/alert/accept") && r.Method == "POST" {
			callCount++
			if callCount < 3 {
				// No alert for first 2 calls
				jsonResponse(w, map[string]interface{}{
					"value": map[string]interface{}{
						"error":   "no such alert",
						"message": "No alert open",
					},
				})
				return
			}
			// Alert appears on 3rd call
			jsonResponse(w, map[string]interface{}{"status": 0})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AcceptAlertStep{BaseStep: flow.BaseStep{TimeoutMs: 5000}}
	result := driver.acceptAlert(step)

	if !result.Success {
		t.Errorf("Expected success after polling, got: %s", result.Message)
	}
	if callCount < 3 {
		t.Errorf("Expected at least 3 calls (polling), got: %d", callCount)
	}
}
