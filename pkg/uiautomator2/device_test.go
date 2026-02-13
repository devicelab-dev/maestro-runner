package uiautomator2

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestBack(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/back") {
			t.Errorf("expected /back suffix, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	err := client.Back()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPressKeyCode(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/device/press_keycode") {
			t.Errorf("expected /appium/device/press_keycode, got %s", r.URL.Path)
		}

		var req KeyCodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.KeyCode != KeyCodeHome {
			t.Errorf("expected keycode %d, got %d", KeyCodeHome, req.KeyCode)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	err := client.PressKeyCode(KeyCodeHome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLongPressKeyCode(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/device/long_press_keycode") {
			t.Errorf("expected /appium/device/long_press_keycode, got %s", r.URL.Path)
		}

		var req KeyCodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.KeyCode != KeyCodePower {
			t.Errorf("expected keycode %d, got %d", KeyCodePower, req.KeyCode)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	err := client.LongPressKeyCode(KeyCodePower)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenNotifications(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/device/open_notifications") {
			t.Errorf("expected /appium/device/open_notifications, got %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	err := client.OpenNotifications()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetClipboard(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/device/get_clipboard") {
			t.Errorf("expected /appium/device/get_clipboard, got %s", r.URL.Path)
		}
		// Base64 encoded "clipboard text"
		encoded := base64.StdEncoding.EncodeToString([]byte("clipboard text"))
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": encoded,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	text, err := client.GetClipboard()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "clipboard text" {
		t.Errorf("expected 'clipboard text', got %s", text)
	}
}

func TestGetClipboardNotBase64(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": "plain text not base64!!!",
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	text, err := client.GetClipboard()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "plain text not base64!!!" {
		t.Errorf("expected 'plain text not base64!!!', got %s", text)
	}
}

func TestGetClipboardEmpty(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": nil,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	text, err := client.GetClipboard()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "" {
		t.Errorf("expected empty, got %s", text)
	}
}

func TestSetClipboard(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/device/set_clipboard") {
			t.Errorf("expected /appium/device/set_clipboard, got %s", r.URL.Path)
		}

		var req ClipboardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ContentType != "plaintext" {
			t.Errorf("expected plaintext, got %s", req.ContentType)
		}
		decoded, _ := base64.StdEncoding.DecodeString(req.Content)
		if string(decoded) != "new clipboard" {
			t.Errorf("expected 'new clipboard', got %s", string(decoded))
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	err := client.SetClipboard("new clipboard")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetDeviceInfo(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/device/info") {
			t.Errorf("expected /appium/device/info, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"androidId":       "abc123",
				"manufacturer":    "Google",
				"model":           "Pixel 6",
				"brand":           "google",
				"apiVersion":      "33",
				"platformVersion": "13",
				"carrierName":     "T-Mobile",
				"realDisplaySize": "1080x2400",
				"displayDensity":  420,
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	info, err := client.GetDeviceInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Manufacturer != "Google" {
		t.Errorf("expected Google, got %s", info.Manufacturer)
	}
	if info.Model != "Pixel 6" {
		t.Errorf("expected Pixel 6, got %s", info.Model)
	}
	if info.DisplayDensity != 420 {
		t.Errorf("expected 420, got %d", info.DisplayDensity)
	}
}

func TestGetBatteryInfo(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/device/battery_info") {
			t.Errorf("expected /appium/device/battery_info, got %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"level":  0.85,
				"status": 2,
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	info, err := client.GetBatteryInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Level != 0.85 {
		t.Errorf("expected 0.85, got %f", info.Level)
	}
	if info.Status != 2 {
		t.Errorf("expected 2, got %d", info.Status)
	}
}

func TestScreenshot(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/screenshot") {
			t.Errorf("expected /screenshot suffix, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		// Base64 encoded PNG header
		pngBytes := []byte{0x89, 0x50, 0x4E, 0x47}
		encoded := base64.StdEncoding.EncodeToString(pngBytes)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": encoded,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	data, err := client.Screenshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 4 || data[0] != 0x89 {
		t.Errorf("unexpected screenshot data: %v", data)
	}
}

func TestScreenshotInvalidResponse(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": 12345,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	_, err := client.Screenshot()
	if err == nil {
		t.Error("expected error for invalid response")
	}
}

func TestSource(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/source") {
			t.Errorf("expected /source suffix, got %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": "<hierarchy><node/></hierarchy>",
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	source, err := client.Source()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(source, "<hierarchy>") {
		t.Errorf("expected XML hierarchy, got %s", source)
	}
}

func TestGetOrientation(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/orientation") {
			t.Errorf("expected /orientation suffix, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": "PORTRAIT",
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	orientation, err := client.GetOrientation()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orientation != "PORTRAIT" {
		t.Errorf("expected PORTRAIT, got %s", orientation)
	}
}

func TestSetOrientation(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/orientation") {
			t.Errorf("expected /orientation suffix, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req OrientationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Orientation != "LANDSCAPE" {
			t.Errorf("expected LANDSCAPE, got %s", req.Orientation)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	err := client.SetOrientation("LANDSCAPE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAlertText(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/alert/text") {
			t.Errorf("expected /alert/text, got %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": "Are you sure?",
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	text, err := client.GetAlertText()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Are you sure?" {
		t.Errorf("expected 'Are you sure?', got %s", text)
	}
}

func TestAcceptAlert(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/alert/accept") {
			t.Errorf("expected /alert/accept, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	err := client.AcceptAlert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDismissAlert(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/alert/dismiss") {
			t.Errorf("expected /alert/dismiss, got %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	err := client.DismissAlert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetSettings(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/settings") {
			t.Errorf("expected /appium/settings, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"waitForIdleTimeout":     10000,
				"waitForSelectorTimeout": 5000,
			},
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	settings, err := client.GetSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings["waitForIdleTimeout"] != float64(10000) {
		t.Errorf("expected 10000, got %v", settings["waitForIdleTimeout"])
	}
}

func TestUpdateSettings(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/settings") {
			t.Errorf("expected /appium/settings, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req SettingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Settings["waitForIdleTimeout"] != float64(5000) {
			t.Errorf("expected 5000, got %v", req.Settings["waitForIdleTimeout"])
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	err := client.UpdateSettings(map[string]interface{}{
		"waitForIdleTimeout": float64(5000),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetClipboardUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	_, err := client.GetClipboard()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetDeviceInfoUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	_, err := client.GetDeviceInfo()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetBatteryInfoUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	_, err := client.GetBatteryInfo()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestScreenshotUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	_, err := client.Screenshot()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSourceUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	_, err := client.Source()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetOrientationUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	_, err := client.GetOrientation()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetAlertTextUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	_, err := client.GetAlertText()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetSettingsUnmarshalError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("invalid json")); err != nil {
			return
		}
	})
	defer server.Close()

	_, err := client.GetSettings()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetDeviceInfoRequestError(t *testing.T) {
	client := newErrorTestClient()
	_, err := client.GetDeviceInfo()
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetBatteryInfoRequestError(t *testing.T) {
	client := newErrorTestClient()
	_, err := client.GetBatteryInfo()
	if err == nil {
		t.Error("expected error")
	}
}

func TestScreenshotRequestError(t *testing.T) {
	client := newErrorTestClient()
	_, err := client.Screenshot()
	if err == nil {
		t.Error("expected error")
	}
}

func TestSourceRequestError(t *testing.T) {
	client := newErrorTestClient()
	_, err := client.Source()
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetOrientationRequestError(t *testing.T) {
	client := newErrorTestClient()
	_, err := client.GetOrientation()
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetAlertTextRequestError(t *testing.T) {
	client := newErrorTestClient()
	_, err := client.GetAlertText()
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetSettingsRequestError(t *testing.T) {
	client := newErrorTestClient()
	_, err := client.GetSettings()
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetClipboardRequestError(t *testing.T) {
	client := newErrorTestClient()
	_, err := client.GetClipboard()
	if err == nil {
		t.Error("expected error")
	}
}

func TestSendKeyActions(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/actions") {
			t.Errorf("expected /actions suffix, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		actions, ok := req["actions"].([]interface{})
		if !ok || len(actions) != 1 {
			t.Errorf("expected 1 action source, got %v", req["actions"])
		}

		actionSource, ok := actions[0].(map[string]interface{})
		if !ok {
			t.Fatal("expected action source to be a map")
		}
		if actionSource["type"] != "key" {
			t.Errorf("expected type 'key', got %v", actionSource["type"])
		}
		if actionSource["id"] != "keyboard" {
			t.Errorf("expected id 'keyboard', got %v", actionSource["id"])
		}

		keyActions, ok := actionSource["actions"].([]interface{})
		if !ok {
			t.Fatal("expected key actions to be a list")
		}
		// "Hi" = 2 chars * 2 (keyDown + keyUp) = 4 actions
		if len(keyActions) != 4 {
			t.Errorf("expected 4 key actions for 'Hi', got %d", len(keyActions))
		}

		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	err := client.SendKeyActions("Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendKeyActionsEmpty(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		actions := req["actions"].([]interface{})
		actionSource := actions[0].(map[string]interface{})
		keyActions := actionSource["actions"]

		// Empty string = nil actions (no keyDown/keyUp pairs)
		if keyActions != nil {
			t.Errorf("expected nil key actions for empty string, got %v", keyActions)
		}

		if err := json.NewEncoder(w).Encode(map[string]interface{}{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	defer server.Close()

	err := client.SendKeyActions("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendKeyActionsRequestError(t *testing.T) {
	client := newErrorTestClient()
	err := client.SendKeyActions("test")
	if err == nil {
		t.Error("expected error")
	}
}

func TestSendKeyActionsServerError(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"error":   "unknown error",
				"message": "server error",
			},
		}); err != nil {
			return
		}
	})
	defer server.Close()

	err := client.SendKeyActions("test")
	if err == nil {
		t.Error("expected error on server error")
	}
}
