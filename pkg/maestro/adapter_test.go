package maestro

import (
	"encoding/json"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// adapterWithMock creates an Adapter backed by a mock WS server.
// The handler inspects each request and returns appropriate responses.
func adapterWithMock(t *testing.T, handler wsHandler) (*Adapter, func()) {
	t.Helper()
	server := newMockWSServer(t, handler)
	client := tcpClientFromServer(t, server)
	adapter := NewAdapter(client)
	cleanup := func() {
		client.Close()
		server.Close()
	}
	return adapter, cleanup
}

func TestAdapterFindElement(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "UI.findElement" {
			t.Errorf("expected UI.findElement, got %s", req.Method)
		}
		return ElementResult{
			ElementID: "e1",
			Text:      "Login",
			Bounds:    BoundsResult{X: 100, Y: 200, Width: 200, Height: 48},
			Displayed: true,
			Enabled:   true,
		}
	})
	defer cleanup()

	elem, err := adapter.FindElement("id", "login_btn")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elem.ID() != "e1" {
		t.Errorf("expected e1, got %s", elem.ID())
	}

	// Verify cached text
	text, err := elem.Text()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Login" {
		t.Errorf("expected 'Login', got %s", text)
	}

	// Verify cached rect
	rect, err := elem.Rect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rect.X != 100 || rect.Y != 200 || rect.Width != 200 || rect.Height != 48 {
		t.Errorf("unexpected rect: %+v", rect)
	}
}

func TestAdapterFindElementError(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		return &ErrorPayload{Code: "not_found", Message: "element not found"}
	})
	defer cleanup()

	_, err := adapter.FindElement("id", "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAdapterActiveElement(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "UI.activeElement" {
			t.Errorf("expected UI.activeElement, got %s", req.Method)
		}
		return ElementResult{
			ElementID: "active1",
			Text:      "Input Field",
			Bounds:    BoundsResult{X: 0, Y: 0, Width: 100, Height: 50},
		}
	})
	defer cleanup()

	elem, err := adapter.ActiveElement()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elem.ID() != "active1" {
		t.Errorf("expected active1, got %s", elem.ID())
	}
}

func TestAdapterClick(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Gesture.click" {
			t.Errorf("expected Gesture.click, got %s", req.Method)
		}
		params, _ := json.Marshal(req.Params)
		var p map[string]interface{}
		_ = json.Unmarshal(params, &p)
		if p["x"].(float64) != 100 || p["y"].(float64) != 200 {
			t.Errorf("expected x=100,y=200, got %v,%v", p["x"], p["y"])
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.Click(100, 200); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterDoubleClick(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Gesture.doubleClick" {
			t.Errorf("expected Gesture.doubleClick, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.DoubleClick(50, 60); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterDoubleClickElement(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Gesture.doubleClick" {
			t.Errorf("expected Gesture.doubleClick, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.DoubleClickElement("e1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterLongClick(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Gesture.longClick" {
			t.Errorf("expected Gesture.longClick, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.LongClick(100, 200, 500); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterLongClickElement(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Gesture.longClick" {
			t.Errorf("expected Gesture.longClick, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.LongClickElement("e1", 500); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterScrollInArea(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Gesture.scroll" {
			t.Errorf("expected Gesture.scroll, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	area := uiautomator2.RectModel{Left: 0, Top: 0, Width: 1080, Height: 1920}
	if err := adapter.ScrollInArea(area, "down", 0.5, 500); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterSwipeInArea(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Gesture.swipe" {
			t.Errorf("expected Gesture.swipe, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	area := uiautomator2.RectModel{Left: 0, Top: 0, Width: 1080, Height: 1920}
	if err := adapter.SwipeInArea(area, "left", 0.8, 300); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterBack(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Input.pressKeyCode" {
			t.Errorf("expected Input.pressKeyCode, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.Back(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterHideKeyboard(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Input.hideKeyboard" {
			t.Errorf("expected Input.hideKeyboard, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.HideKeyboard(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterPressKeyCode(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Input.pressKeyCode" {
			t.Errorf("expected Input.pressKeyCode, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.PressKeyCode(66); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterSendKeyActions(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Input.sendKeyActions" {
			t.Errorf("expected Input.sendKeyActions, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.SendKeyActions("hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterScreenshot(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "UI.screenshot" {
			t.Errorf("expected UI.screenshot, got %s", req.Method)
		}
		return ScreenshotResult{Data: "UE5H"} // base64("PNG")
	})
	defer cleanup()

	data, err := adapter.Screenshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "PNG" {
		t.Errorf("expected PNG, got %s", string(data))
	}
}

func TestAdapterSource(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "UI.getSource" {
			t.Errorf("expected UI.getSource, got %s", req.Method)
		}
		return SourceResult{XML: "<hierarchy></hierarchy>"}
	})
	defer cleanup()

	source, err := adapter.Source()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "<hierarchy></hierarchy>" {
		t.Errorf("expected hierarchy XML, got %s", source)
	}
}

func TestAdapterGetOrientation(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Device.getOrientation" {
			t.Errorf("expected Device.getOrientation, got %s", req.Method)
		}
		return OrientationResult{Orientation: "PORTRAIT"}
	})
	defer cleanup()

	orient, err := adapter.GetOrientation()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orient != "PORTRAIT" {
		t.Errorf("expected PORTRAIT, got %s", orient)
	}
}

func TestAdapterSetOrientation(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Device.setOrientation" {
			t.Errorf("expected Device.setOrientation, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.SetOrientation("LANDSCAPE"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterGetClipboard(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Device.getClipboard" {
			t.Errorf("expected Device.getClipboard, got %s", req.Method)
		}
		return ClipboardResult{Text: "clipboard content"}
	})
	defer cleanup()

	text, err := adapter.GetClipboard()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "clipboard content" {
		t.Errorf("expected 'clipboard content', got %s", text)
	}
}

func TestAdapterSetClipboard(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Device.setClipboard" {
			t.Errorf("expected Device.setClipboard, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.SetClipboard("new text"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterGetDeviceInfo(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Device.getInfo" {
			t.Errorf("expected Device.getInfo, got %s", req.Method)
		}
		return DeviceResult{
			Model:           "Pixel 8",
			Manufacturer:    "Google",
			Brand:           "google",
			SDK:             34,
			PlatformVersion: "14",
			DisplaySize:     "1080x2400",
			DisplayDensity:  420,
		}
	})
	defer cleanup()

	info, err := adapter.GetDeviceInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Model != "Pixel 8" {
		t.Errorf("expected 'Pixel 8', got %s", info.Model)
	}
	if info.RealDisplaySize != "1080x2400" {
		t.Errorf("expected '1080x2400', got %s", info.RealDisplaySize)
	}
	if info.APIVersion != "34" {
		t.Errorf("expected '34', got %s", info.APIVersion)
	}
}

func TestAdapterLaunchApp(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Device.launchApp" {
			t.Errorf("expected Device.launchApp, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.LaunchApp("com.example.app", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterLaunchAppWithArgs(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Device.launchApp" {
			t.Errorf("expected Device.launchApp, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	args := map[string]interface{}{"key": "value"}
	if err := adapter.LaunchApp("com.example.app", args); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterSetAppiumSettings(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Settings.update" {
			t.Errorf("expected Settings.update, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	settings := map[string]interface{}{"waitForIdleTimeout": 5000}
	if err := adapter.SetAppiumSettings(settings); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterSetImplicitWait(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Settings.update" {
			t.Errorf("expected Settings.update, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.SetImplicitWait(5 * 1000 * 1000 * 1000); err != nil { // 5s
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterCreateSession(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Session.create" {
			t.Errorf("expected Session.create, got %s", req.Method)
		}
		return SessionResult{
			SessionID: "session-123",
			DeviceInfo: DeviceResult{
				Model: "Pixel 8",
				SDK:   34,
			},
		}
	})
	defer cleanup()

	result, err := adapter.CreateSession()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionID != "session-123" {
		t.Errorf("expected session-123, got %s", result.SessionID)
	}
	if result.DeviceInfo.Model != "Pixel 8" {
		t.Errorf("expected 'Pixel 8', got %s", result.DeviceInfo.Model)
	}
}

func TestAdapterDeleteSession(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Session.delete" {
			t.Errorf("expected Session.delete, got %s", req.Method)
		}
		return map[string]interface{}{}
	})
	defer cleanup()

	if err := adapter.DeleteSession(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdapterStatus(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Session.status" {
			t.Errorf("expected Session.status, got %s", req.Method)
		}
		return map[string]interface{}{"ready": true}
	})
	defer cleanup()

	ready, err := adapter.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Error("expected ready=true")
	}
}
