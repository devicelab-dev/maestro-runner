package appium

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// writeJSON encodes data as JSON to the response writer.
func writeJSON(w http.ResponseWriter, data interface{}) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func TestClient_Connect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session" && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId": "test-session-123",
					"capabilities": map[string]interface{}{
						"platformName":    "Android",
						"platformVersion": "14",
					},
				},
			})
			return
		}
		if r.URL.Path == "/session/test-session-123/window/rect" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"width":  1080.0,
					"height": 1920.0,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.Connect(map[string]interface{}{
		"platformName": "Android",
	})
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if client.sessionID != "test-session-123" {
		t.Errorf("Expected sessionID 'test-session-123', got '%s'", client.sessionID)
	}

	if client.platform != "android" {
		t.Errorf("Expected platform 'android', got '%s'", client.platform)
	}

	w, h := client.ScreenSize()
	if w != 1080 || h != 1920 {
		t.Errorf("Expected screen size 1080x1920, got %dx%d", w, h)
	}
}

// TestClient_Connect_SkipsGrantLoopWhenAutoGrantSet verifies the
// 1.1.15+ optimisation: when the caller passes
// `appium:autoGrantPermissions: true`, we skip our own pm grant loop
// so the session boots cleanly on hosts (e.g. Sauce) that disable
// `adb_shell`. Counterpart test below proves the loop still runs
// without the cap.
func TestClient_Connect_SkipsGrantLoopWhenAutoGrantSet(t *testing.T) {
	scriptCalls := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/session" && r.Method == "POST":
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId": "ag-session",
					"capabilities": map[string]interface{}{
						"platformName": "Android",
					},
				},
			})
		case r.URL.Path == "/session/ag-session/window/rect":
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
		case r.URL.Path == "/session/ag-session/execute/sync":
			var body struct {
				Script string `json:"script"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			scriptCalls[body.Script]++
			writeJSON(w, map[string]interface{}{"value": nil})
		case r.URL.Path == "/session/ag-session/appium/device/start_activity":
			writeJSON(w, map[string]interface{}{"value": nil})
		default:
			writeJSON(w, map[string]interface{}{"value": nil})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.Connect(map[string]interface{}{
		"platformName":                  "Android",
		"appium:appPackage":             "com.test.app",
		"appium:appActivity":            ".MainActivity",
		"appium:noReset":                false,
		"appium:autoGrantPermissions":   true,
	})
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if scriptCalls["mobile: shell"] != 0 {
		t.Errorf("autoGrantPermissions=true should skip the pm grant loop; got %d mobile: shell calls", scriptCalls["mobile: shell"])
	}
}

// TestClient_Connect_RunsGrantLoopWithoutAutoGrant verifies that the
// legacy `mobile: shell pm grant` loop still fires for callers who
// haven't opted into `appium:autoGrantPermissions: true`.
func TestClient_Connect_RunsGrantLoopWithoutAutoGrant(t *testing.T) {
	scriptCalls := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/session" && r.Method == "POST":
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId":    "ng-session",
					"capabilities": map[string]interface{}{"platformName": "Android"},
				},
			})
		case r.URL.Path == "/session/ng-session/window/rect":
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
		case r.URL.Path == "/session/ng-session/execute/sync":
			var body struct {
				Script string `json:"script"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			scriptCalls[body.Script]++
			writeJSON(w, map[string]interface{}{"value": nil})
		default:
			writeJSON(w, map[string]interface{}{"value": nil})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.Connect(map[string]interface{}{
		"platformName":      "Android",
		"appium:appPackage": "com.test.app",
		"appium:noReset":    false,
	})
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Expect one mobile: shell per known runtime permission.
	want := len(getAllPermissions())
	if scriptCalls["mobile: shell"] != want {
		t.Errorf("expected %d mobile: shell pm grant calls; got %d", want, scriptCalls["mobile: shell"])
	}
}

// TestClient_ClearApp_AndroidPrefersClearAppNative verifies the
// 1.1.16 swap: Android ClearApp now uses native `mobile: clearApp`
// (no adb_shell required) and only falls back to `mobile: shell pm
// clear` when the native call fails.
func TestClient_ClearApp_AndroidPrefersClearAppNative(t *testing.T) {
	scriptCalls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session/sess/appium/device/terminate_app":
			writeJSON(w, map[string]interface{}{"value": nil})
		case "/session/sess/execute/sync":
			var body struct {
				Script string `json:"script"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			scriptCalls = append(scriptCalls, body.Script)
			writeJSON(w, map[string]interface{}{"value": nil})
		default:
			writeJSON(w, map[string]interface{}{"value": nil})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "sess"
	client.platform = "android"

	if err := client.ClearAppData("com.test.app"); err != nil {
		t.Fatalf("ClearApp failed: %v", err)
	}

	// Expect mobile: clearApp, NOT mobile: shell.
	if len(scriptCalls) != 1 || scriptCalls[0] != "mobile: clearApp" {
		t.Errorf("expected only mobile: clearApp call; got %v", scriptCalls)
	}
}

// TestClient_ClearApp_AndroidFallsBackToShellOnNativeFailure verifies
// the fallback path: when the Appium server doesn't implement
// `mobile: clearApp` (older versions), we fall back to the legacy
// `mobile: shell pm clear` so existing setups don't regress.
func TestClient_ClearApp_AndroidFallsBackToShellOnNativeFailure(t *testing.T) {
	scriptCalls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session/sess/appium/device/terminate_app":
			writeJSON(w, map[string]interface{}{"value": nil})
		case "/session/sess/execute/sync":
			var body struct {
				Script string `json:"script"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			scriptCalls = append(scriptCalls, body.Script)
			if body.Script == "mobile: clearApp" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				writeJSON(w, map[string]interface{}{"value": map[string]interface{}{
					"error":   "unknown method",
					"message": "mobile: clearApp not supported",
				}})
				return
			}
			writeJSON(w, map[string]interface{}{"value": nil})
		default:
			writeJSON(w, map[string]interface{}{"value": nil})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "sess"
	client.platform = "android"

	if err := client.ClearAppData("com.test.app"); err != nil {
		t.Fatalf("ClearApp failed: %v", err)
	}

	if len(scriptCalls) != 2 {
		t.Fatalf("expected 1 native attempt + 1 shell fallback (2 calls); got %d: %v", len(scriptCalls), scriptCalls)
	}
	if scriptCalls[0] != "mobile: clearApp" {
		t.Errorf("first call should be mobile: clearApp; got %q", scriptCalls[0])
	}
	if scriptCalls[1] != "mobile: shell" {
		t.Errorf("second call should be mobile: shell fallback; got %q", scriptCalls[1])
	}
}

func TestClient_Disconnect(t *testing.T) {
	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session" && r.Method == "DELETE" {
			deleteCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.Disconnect()
	if err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	if !deleteCalled {
		t.Error("DELETE /session was not called")
	}

	if client.sessionID != "" {
		t.Error("sessionID should be cleared after disconnect")
	}
}

func TestClient_FindElement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/element" && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"element-6066-11e4-a52e-4f735466cecf": "elem-123",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	elemID, err := client.FindElement("accessibility id", "myButton")
	if err != nil {
		t.Fatalf("FindElement failed: %v", err)
	}

	if elemID != "elem-123" {
		t.Errorf("Expected element ID 'elem-123', got '%s'", elemID)
	}
}

func TestClient_FindElements(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/elements" && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{
				"value": []interface{}{
					map[string]interface{}{"element-6066-11e4-a52e-4f735466cecf": "elem-1"},
					map[string]interface{}{"element-6066-11e4-a52e-4f735466cecf": "elem-2"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	ids, err := client.FindElements("xpath", "//button")
	if err != nil {
		t.Fatalf("FindElements failed: %v", err)
	}

	if len(ids) != 2 {
		t.Errorf("Expected 2 elements, got %d", len(ids))
	}
}

func TestClient_Tap(t *testing.T) {
	actionsCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/actions" && r.Method == "POST" {
			actionsCalled = true
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.Tap(100, 200)
	if err != nil {
		t.Fatalf("Tap failed: %v", err)
	}

	if !actionsCalled {
		t.Error("Actions endpoint was not called")
	}
}

func TestClient_DoubleTap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/actions" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.DoubleTap(100, 200)
	if err != nil {
		t.Fatalf("DoubleTap failed: %v", err)
	}
}

func TestClient_LongPress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/actions" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.LongPress(100, 200, 1000)
	if err != nil {
		t.Fatalf("LongPress failed: %v", err)
	}
}

func TestClient_Swipe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/actions" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.Swipe(100, 200, 100, 500, 300)
	if err != nil {
		t.Fatalf("Swipe failed: %v", err)
	}
}

func TestClient_SendKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/actions" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.SendKeys("hello")
	if err != nil {
		t.Fatalf("SendKeys failed: %v", err)
	}
}

func TestClient_Screenshot(t *testing.T) {
	expectedData := []byte("fake-png-data")
	encoded := base64.StdEncoding.EncodeToString(expectedData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/screenshot" {
			writeJSON(w, map[string]interface{}{
				"value": encoded,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	data, err := client.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot failed: %v", err)
	}

	if string(data) != string(expectedData) {
		t.Errorf("Screenshot data mismatch")
	}
}

func TestClient_Source(t *testing.T) {
	expectedSource := "<hierarchy><element/></hierarchy>"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/source" {
			writeJSON(w, map[string]interface{}{
				"value": expectedSource,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	source, err := client.Source()
	if err != nil {
		t.Fatalf("Source failed: %v", err)
	}

	if source != expectedSource {
		t.Errorf("Expected source '%s', got '%s'", expectedSource, source)
	}
}

func TestClient_GetOrientation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/orientation" && r.Method == "GET" {
			writeJSON(w, map[string]interface{}{
				"value": "PORTRAIT",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	orientation, err := client.GetOrientation()
	if err != nil {
		t.Fatalf("GetOrientation failed: %v", err)
	}

	if orientation != "portrait" {
		t.Errorf("Expected 'portrait', got '%s'", orientation)
	}
}

func TestClient_SetOrientation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/orientation" && r.Method == "POST" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.SetOrientation("landscape")
	if err != nil {
		t.Fatalf("SetOrientation failed: %v", err)
	}
}

func TestClient_LaunchApp(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		appID    string
	}{
		{"Android", "android", "com.example.app"},
		{"iOS", "ios", "com.example.app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/session/test-session/appium/device/activate_app" {
					writeJSON(w, map[string]interface{}{"value": nil})
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			client := NewClient(server.URL)
			client.sessionID = "test-session"
			client.platform = tt.platform

			err := client.LaunchApp(tt.appID)
			if err != nil {
				t.Fatalf("LaunchApp failed: %v", err)
			}
		})
	}
}

func TestClient_TerminateApp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/appium/device/terminate_app" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"
	client.platform = "android"

	err := client.TerminateApp("com.example.app")
	if err != nil {
		t.Fatalf("TerminateApp failed: %v", err)
	}
}

func TestClient_Back(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/appium/device/press_keycode" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.Back()
	if err != nil {
		t.Fatalf("Back failed: %v", err)
	}
}

func TestClient_HideKeyboard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/appium/device/hide_keyboard" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.HideKeyboard()
	if err != nil {
		t.Fatalf("HideKeyboard failed: %v", err)
	}
}

func TestClient_GetClipboard(t *testing.T) {
	expectedText := "clipboard content"
	encoded := base64.StdEncoding.EncodeToString([]byte(expectedText))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/appium/device/get_clipboard" {
			writeJSON(w, map[string]interface{}{
				"value": encoded,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	text, err := client.GetClipboard()
	if err != nil {
		t.Fatalf("GetClipboard failed: %v", err)
	}

	if text != expectedText {
		t.Errorf("Expected '%s', got '%s'", expectedText, text)
	}
}

func TestClient_SetClipboard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/appium/device/set_clipboard" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.SetClipboard("test text")
	if err != nil {
		t.Fatalf("SetClipboard failed: %v", err)
	}
}

func TestClient_OpenURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/url" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.OpenURL("https://example.com")
	if err != nil {
		t.Fatalf("OpenURL failed: %v", err)
	}
}

func TestClient_SetLocation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/location" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.SetLocation(37.7749, -122.4194)
	if err != nil {
		t.Fatalf("SetLocation failed: %v", err)
	}
}

func TestClient_GetElementRect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/element/elem-1/rect" {
			writeJSON(w, map[string]interface{}{
				"value": map[string]interface{}{
					"x":      100.0,
					"y":      200.0,
					"width":  300.0,
					"height": 50.0,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	x, y, w, h, err := client.GetElementRect("elem-1")
	if err != nil {
		t.Fatalf("GetElementRect failed: %v", err)
	}

	if x != 100 || y != 200 || w != 300 || h != 50 {
		t.Errorf("Expected rect (100,200,300,50), got (%d,%d,%d,%d)", x, y, w, h)
	}
}

func TestClient_GetElementText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/element/elem-1/text" {
			writeJSON(w, map[string]interface{}{
				"value": "Hello World",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	text, err := client.GetElementText("elem-1")
	if err != nil {
		t.Fatalf("GetElementText failed: %v", err)
	}

	if text != "Hello World" {
		t.Errorf("Expected 'Hello World', got '%s'", text)
	}
}

func TestClient_ClickElement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/element/elem-1/click" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.ClickElement("elem-1")
	if err != nil {
		t.Fatalf("ClickElement failed: %v", err)
	}
}

func TestClient_ClearElement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/element/elem-1/clear" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.ClearElement("elem-1")
	if err != nil {
		t.Fatalf("ClearElement failed: %v", err)
	}
}

func TestClient_SetImplicitWait(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/test-session/timeouts" {
			writeJSON(w, map[string]interface{}{"value": nil})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	client.sessionID = "test-session"

	err := client.SetImplicitWait(10000)
	if err != nil {
		t.Fatalf("SetImplicitWait failed: %v", err)
	}
}

func TestExtractElementID(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
	}{
		{
			"W3C format",
			map[string]interface{}{"element-6066-11e4-a52e-4f735466cecf": "elem-123"},
			"elem-123",
		},
		{
			"Legacy format",
			map[string]interface{}{"ELEMENT": "elem-456"},
			"elem-456",
		},
		{
			"Empty",
			map[string]interface{}{},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractElementID(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
