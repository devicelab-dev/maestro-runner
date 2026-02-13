package wda

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockWDAServer creates a mock WDA server for testing
func mockWDAServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// jsonResponse writes a JSON response
func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// TestNewClient tests client creation
func TestNewClient(t *testing.T) {
	client := NewClient(8100)

	if client.baseURL != "http://localhost:8100" {
		t.Errorf("Expected baseURL 'http://localhost:8100', got '%s'", client.baseURL)
	}

	if client.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	}
}

// TestCreateSession tests session creation
func TestCreateSession(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/session" {
			t.Errorf("Expected POST /session, got %s %s", r.Method, r.URL.Path)
		}

		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{
				"sessionId": "test-session-123",
			},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
	}

	err := client.CreateSession("com.example.app", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if client.sessionID != "test-session-123" {
		t.Errorf("Expected sessionID 'test-session-123', got '%s'", client.sessionID)
	}
}

// TestCreateSessionAlternateFormat tests alternate session response format
func TestCreateSessionAlternateFormat(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{
			"sessionId": "alternate-session-456",
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
	}

	err := client.CreateSession("com.example.app", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if client.sessionID != "alternate-session-456" {
		t.Errorf("Expected sessionID 'alternate-session-456', got '%s'", client.sessionID)
	}
}

// TestCreateSessionWithAlertAction tests that alertAction is included in session caps
func TestCreateSessionWithAlertAction(t *testing.T) {
	var receivedBody map[string]interface{}
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/session" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedBody)
		}
		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{
				"sessionId": "test-session-alert",
			},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
	}

	err := client.CreateSession("com.example.app", "accept")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Verify defaultAlertAction was sent
	caps, _ := receivedBody["capabilities"].(map[string]interface{})
	alwaysMatch, _ := caps["alwaysMatch"].(map[string]interface{})
	if alwaysMatch["defaultAlertAction"] != "accept" {
		t.Errorf("Expected defaultAlertAction 'accept', got '%v'", alwaysMatch["defaultAlertAction"])
	}
}

// TestCreateSessionWithoutAlertAction tests that no alertAction omits the key
func TestCreateSessionWithoutAlertAction(t *testing.T) {
	var receivedBody map[string]interface{}
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/session" {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &receivedBody)
		}
		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{
				"sessionId": "test-session-no-alert",
			},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
	}

	err := client.CreateSession("com.example.app", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Verify defaultAlertAction was NOT sent
	caps, _ := receivedBody["capabilities"].(map[string]interface{})
	alwaysMatch, _ := caps["alwaysMatch"].(map[string]interface{})
	if _, exists := alwaysMatch["defaultAlertAction"]; exists {
		t.Error("Expected defaultAlertAction to be absent when alertAction is empty")
	}
}

// TestDeleteSession tests session deletion
func TestDeleteSession(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || !strings.Contains(r.URL.Path, "/session/") {
			t.Errorf("Expected DELETE /session/*, got %s %s", r.Method, r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.DeleteSession()
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	if client.sessionID != "" {
		t.Errorf("Expected sessionID to be cleared, got '%s'", client.sessionID)
	}
}

// TestDeleteSessionNoSession tests deleting when no session exists
func TestDeleteSessionNoSession(t *testing.T) {
	client := &Client{
		httpClient: http.DefaultClient,
	}

	err := client.DeleteSession()
	if err != nil {
		t.Errorf("DeleteSession should not fail when no session: %v", err)
	}
}

// TestHasSession tests session existence check
func TestHasSession(t *testing.T) {
	client := &Client{}

	if client.HasSession() {
		t.Error("Expected HasSession to return false initially")
	}

	client.sessionID = "test-session"
	if !client.HasSession() {
		t.Error("Expected HasSession to return true with session")
	}
}

// TestSessionID tests session ID getter
func TestSessionID(t *testing.T) {
	client := &Client{sessionID: "my-session"}

	if client.SessionID() != "my-session" {
		t.Errorf("Expected 'my-session', got '%s'", client.SessionID())
	}
}

// TestStatus tests status endpoint
func TestStatus(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("Expected /status, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{
				"state": "idle",
			},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
	}

	status, err := client.Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if status == nil {
		t.Error("Expected status response")
	}
}

// TestLaunchApp tests app launching
func TestLaunchApp(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/wda/apps/launch") {
			t.Errorf("Expected /wda/apps/launch, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.LaunchApp("com.example.app")
	if err != nil {
		t.Fatalf("LaunchApp failed: %v", err)
	}
}

// TestTerminateApp tests app termination
func TestTerminateApp(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/wda/apps/terminate") {
			t.Errorf("Expected /wda/apps/terminate, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.TerminateApp("com.example.app")
	if err != nil {
		t.Fatalf("TerminateApp failed: %v", err)
	}
}

// TestActivateApp tests app activation
func TestActivateApp(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/wda/apps/activate") {
			t.Errorf("Expected /wda/apps/activate, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.ActivateApp("com.example.app")
	if err != nil {
		t.Fatalf("ActivateApp failed: %v", err)
	}
}

// TestTap tests tap action
func TestTap(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/wda/tap") {
			t.Errorf("Expected /wda/tap, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.Tap(100.0, 200.0)
	if err != nil {
		t.Fatalf("Tap failed: %v", err)
	}
}

// TestDoubleTap tests double tap action
func TestDoubleTap(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/wda/doubleTap") {
			t.Errorf("Expected /wda/doubleTap, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.DoubleTap(100.0, 200.0)
	if err != nil {
		t.Fatalf("DoubleTap failed: %v", err)
	}
}

// TestLongPress tests long press action
func TestLongPress(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/wda/touchAndHold") {
			t.Errorf("Expected /wda/touchAndHold, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.LongPress(100.0, 200.0, 1.0)
	if err != nil {
		t.Fatalf("LongPress failed: %v", err)
	}
}

// TestSwipe tests swipe action
func TestSwipe(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/wda/dragfromtoforduration") {
			t.Errorf("Expected /wda/dragfromtoforduration, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.Swipe(100.0, 200.0, 100.0, 500.0, 0.3)
	if err != nil {
		t.Fatalf("Swipe failed: %v", err)
	}
}

// TestSendKeys tests text input
func TestSendKeys(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/wda/keys") {
			t.Errorf("Expected /wda/keys, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.SendKeys("hello world")
	if err != nil {
		t.Fatalf("SendKeys failed: %v", err)
	}
}

// TestElementSendKeys tests element-specific text input
func TestElementSendKeys(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element/elem123/value") {
			t.Errorf("Expected /element/elem123/value, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.ElementSendKeys("elem123", "hello")
	if err != nil {
		t.Fatalf("ElementSendKeys failed: %v", err)
	}
}

// TestElementClear tests element clearing
func TestElementClear(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element/elem123/clear") {
			t.Errorf("Expected /element/elem123/clear, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.ElementClear("elem123")
	if err != nil {
		t.Fatalf("ElementClear failed: %v", err)
	}
}

// TestScreenshot tests screenshot capture
func TestScreenshot(t *testing.T) {
	expectedData := []byte("fake png data")
	encoded := base64.StdEncoding.EncodeToString(expectedData)

	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/screenshot") {
			t.Errorf("Expected /screenshot, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"value": encoded})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	data, err := client.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot failed: %v", err)
	}

	if string(data) != string(expectedData) {
		t.Errorf("Expected %v, got %v", expectedData, data)
	}
}

// TestScreenshotInvalidResponse tests screenshot with invalid response
func TestScreenshotInvalidResponse(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{"value": 123}) // Not a string
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.Screenshot()
	if err == nil {
		t.Error("Expected error for invalid screenshot response")
	}
}

// TestSource tests page source retrieval
func TestSource(t *testing.T) {
	expectedSource := "<AppiumAUT><XCUIElementTypeWindow/></AppiumAUT>"

	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/source") {
			t.Errorf("Expected /source, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"value": expectedSource})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	source, err := client.Source()
	if err != nil {
		t.Fatalf("Source failed: %v", err)
	}

	if source != expectedSource {
		t.Errorf("Expected '%s', got '%s'", expectedSource, source)
	}
}

// TestSourceInvalidResponse tests source with invalid response
func TestSourceInvalidResponse(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{"value": 123})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.Source()
	if err == nil {
		t.Error("Expected error for invalid source response")
	}
}

// TestWindowSize tests window size retrieval
func TestWindowSize(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/window/size") {
			t.Errorf("Expected /window/size, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{
				"width":  390.0,
				"height": 844.0,
			},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	width, height, err := client.WindowSize()
	if err != nil {
		t.Fatalf("WindowSize failed: %v", err)
	}

	if width != 390 || height != 844 {
		t.Errorf("Expected (390, 844), got (%d, %d)", width, height)
	}
}

// TestWindowSizeInvalidResponse tests window size with invalid response
func TestWindowSizeInvalidResponse(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{"value": "invalid"})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, _, err := client.WindowSize()
	if err == nil {
		t.Error("Expected error for invalid window size response")
	}
}

// TestPressButton tests button press
func TestPressButton(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/wda/pressButton") {
			t.Errorf("Expected /wda/pressButton, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.PressButton("home")
	if err != nil {
		t.Fatalf("PressButton failed: %v", err)
	}
}

// TestHome tests home button press
func TestHome(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.Home()
	if err != nil {
		t.Fatalf("Home failed: %v", err)
	}
}

// TestLockUnlock tests lock/unlock
func TestLockUnlock(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	if err := client.Lock(); err != nil {
		t.Fatalf("Lock failed: %v", err)
	}

	if err := client.Unlock(); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}
}

// TestAcceptAlertClient tests accepting a system alert via client
func TestAcceptAlertClient(t *testing.T) {
	var called bool
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/alert/accept") && r.Method == "POST" {
			called = true
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	if err := client.AcceptAlert(); err != nil {
		t.Fatalf("AcceptAlert failed: %v", err)
	}
	if !called {
		t.Error("Expected /alert/accept to be called")
	}
}

// TestDismissAlertClient tests dismissing a system alert via client
func TestDismissAlertClient(t *testing.T) {
	var called bool
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/alert/dismiss") && r.Method == "POST" {
			called = true
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	if err := client.DismissAlert(); err != nil {
		t.Fatalf("DismissAlert failed: %v", err)
	}
	if !called {
		t.Error("Expected /alert/dismiss to be called")
	}
}

// TestAcceptAlertClientError tests AcceptAlert when server returns an error
func TestAcceptAlertClientError(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/alert/accept") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "no such alert",
					"message": "An attempt was made to operate on a modal dialog when one was not open",
				},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.AcceptAlert()
	if err == nil {
		t.Error("Expected error when no alert is present")
	}
}

// TestDismissAlertClientError tests DismissAlert when server returns an error
func TestDismissAlertClientError(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/alert/dismiss") {
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{
					"error":   "no such alert",
					"message": "No alert open",
				},
			})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.DismissAlert()
	if err == nil {
		t.Error("Expected error when no alert is present")
	}
}

// TestGetOrientation tests orientation retrieval
func TestGetOrientation(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{"value": "PORTRAIT"})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	orientation, err := client.GetOrientation()
	if err != nil {
		t.Fatalf("GetOrientation failed: %v", err)
	}

	if orientation != "PORTRAIT" {
		t.Errorf("Expected 'PORTRAIT', got '%s'", orientation)
	}
}

// TestGetOrientationInvalidResponse tests orientation with invalid response
func TestGetOrientationInvalidResponse(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{"value": 123})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.GetOrientation()
	if err == nil {
		t.Error("Expected error for invalid orientation response")
	}
}

// TestSetOrientation tests orientation setting
func TestSetOrientation(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.SetOrientation("LANDSCAPE")
	if err != nil {
		t.Fatalf("SetOrientation failed: %v", err)
	}
}

// TestDeepLink tests deep link opening
func TestDeepLink(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/url") {
			t.Errorf("Expected /url, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.DeepLink("myapp://page")
	if err != nil {
		t.Fatalf("DeepLink failed: %v", err)
	}
}

// TestFindElement tests element finding
func TestFindElement(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element") {
			t.Errorf("Expected /element, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{
				"ELEMENT": "elem123",
			},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	elemID, err := client.FindElement("class chain", "**/XCUIElementTypeButton")
	if err != nil {
		t.Fatalf("FindElement failed: %v", err)
	}

	if elemID != "elem123" {
		t.Errorf("Expected 'elem123', got '%s'", elemID)
	}
}

// TestFindElementW3CFormat tests W3C format element finding
func TestFindElementW3CFormat(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{
				"element-6066-11e4-a52e-4f735466cecf": "w3c-elem-456",
			},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	elemID, err := client.FindElement("predicate string", "name == 'test'")
	if err != nil {
		t.Fatalf("FindElement failed: %v", err)
	}

	if elemID != "w3c-elem-456" {
		t.Errorf("Expected 'w3c-elem-456', got '%s'", elemID)
	}
}

// TestFindElementNotFound tests element not found
func TestFindElementNotFound(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.FindElement("class chain", "**/XCUIElementTypeNotExist")
	if err == nil {
		t.Error("Expected error for element not found")
	}
}

// TestFindElements tests multiple element finding
func TestFindElements(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{
			"value": []interface{}{
				map[string]interface{}{"ELEMENT": "elem1"},
				map[string]interface{}{"ELEMENT": "elem2"},
				map[string]interface{}{"ELEMENT": "elem3"},
			},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	elemIDs, err := client.FindElements("class chain", "**/XCUIElementTypeButton")
	if err != nil {
		t.Fatalf("FindElements failed: %v", err)
	}

	if len(elemIDs) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(elemIDs))
	}
}

// TestElementClick tests element clicking
func TestElementClick(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element/elem123/click") {
			t.Errorf("Expected /element/elem123/click, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	err := client.ElementClick("elem123")
	if err != nil {
		t.Fatalf("ElementClick failed: %v", err)
	}
}

// TestElementText tests element text retrieval
func TestElementText(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{"value": "Button Text"})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	text, err := client.ElementText("elem123")
	if err != nil {
		t.Fatalf("ElementText failed: %v", err)
	}

	if text != "Button Text" {
		t.Errorf("Expected 'Button Text', got '%s'", text)
	}
}

// TestElementDisplayed tests element visibility check
func TestElementDisplayed(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{"value": true})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	displayed, err := client.ElementDisplayed("elem123")
	if err != nil {
		t.Fatalf("ElementDisplayed failed: %v", err)
	}

	if !displayed {
		t.Error("Expected displayed=true")
	}
}

// TestElementRect tests element bounds retrieval
func TestElementRect(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{
				"x":      50.0,
				"y":      100.0,
				"width":  200.0,
				"height": 44.0,
			},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	x, y, width, height, err := client.ElementRect("elem123")
	if err != nil {
		t.Fatalf("ElementRect failed: %v", err)
	}

	if x != 50 || y != 100 || width != 200 || height != 44 {
		t.Errorf("Expected (50, 100, 200, 44), got (%d, %d, %d, %d)", x, y, width, height)
	}
}

// TestGetActiveElement tests active element retrieval
func TestGetActiveElement(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/element/active") {
			t.Errorf("Expected /element/active, got %s", r.URL.Path)
		}
		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{
				"ELEMENT": "active-elem-789",
			},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	elemID, err := client.GetActiveElement()
	if err != nil {
		t.Fatalf("GetActiveElement failed: %v", err)
	}

	if elemID != "active-elem-789" {
		t.Errorf("Expected 'active-elem-789', got '%s'", elemID)
	}
}

// TestGetActiveElementNotFound tests no active element
func TestGetActiveElementNotFound(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.GetActiveElement()
	if err == nil {
		t.Error("Expected error for no active element")
	}
}

// TestSessionPath tests session path building
func TestSessionPath(t *testing.T) {
	client := &Client{sessionID: "test-session"}

	path := client.sessionPath("/screenshot")
	if path != "/session/test-session/screenshot" {
		t.Errorf("Expected '/session/test-session/screenshot', got '%s'", path)
	}

	// Without session
	client.sessionID = ""
	path = client.sessionPath("/status")
	if path != "/status" {
		t.Errorf("Expected '/status', got '%s'", path)
	}
}

// TestWDAError tests WDA error handling
func TestWDAError(t *testing.T) {
	server := mockWDAServer(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]interface{}{
			"value": map[string]interface{}{
				"error":   "no such element",
				"message": "Element not found using xpath",
			},
		})
	})
	defer server.Close()

	client := &Client{
		baseURL:    server.URL,
		httpClient: http.DefaultClient,
		sessionID:  "test-session",
	}

	_, err := client.FindElement("xpath", "//invalid")
	if err == nil {
		t.Error("Expected error for WDA error response")
	}

	if !strings.Contains(err.Error(), "WDA error") {
		t.Errorf("Expected WDA error message, got: %v", err)
	}
}

// TestBase64Decode tests base64 decoding
func TestBase64Decode(t *testing.T) {
	original := "Hello World"
	encoded := base64.StdEncoding.EncodeToString([]byte(original))

	decoded, err := base64Decode(encoded)
	if err != nil {
		t.Fatalf("base64Decode failed: %v", err)
	}

	if string(decoded) != original {
		t.Errorf("Expected '%s', got '%s'", original, string(decoded))
	}
}

// TestBase64DecodeInvalid tests invalid base64
func TestBase64DecodeInvalid(t *testing.T) {
	_, err := base64Decode("not valid base64!!!")
	if err == nil {
		t.Error("Expected error for invalid base64")
	}
}
