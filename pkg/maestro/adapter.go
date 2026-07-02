package maestro

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// Adapter implements the UIA2Client interface from pkg/driver/uiautomator2
// by translating each method to a WebSocket RPC call to the DeviceLab Android Driver.
type Adapter struct {
	client *Client
}

// NewAdapter creates an adapter backed by the given WebSocket client.
func NewAdapter(client *Client) *Adapter {
	return &Adapter{client: client}
}

// --- Element finding ---

// FindElement finds a single element on the device.
// Returns a cached element with text and bounds pre-populated.
func (a *Adapter) FindElement(strategy, selector string) (*uiautomator2.Element, error) {
	resp, err := a.client.Call("UI.findElement", map[string]interface{}{
		"strategy": strategy,
		"selector": selector,
	})
	if err != nil {
		return nil, err
	}

	var result ElementResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse findElement result: %w", err)
	}

	elem := uiautomator2.NewCachedElement(
		result.ElementID,
		result.Text,
		uiautomator2.ElementRect{
			X:      result.Bounds.X,
			Y:      result.Bounds.Y,
			Width:  result.Bounds.Width,
			Height: result.Bounds.Height,
		},
	)
	a.wireElementActions(elem, result.ElementID)
	return elem, nil
}

// ActiveElement returns the currently focused element.
func (a *Adapter) ActiveElement() (*uiautomator2.Element, error) {
	resp, err := a.client.Call("UI.activeElement", nil)
	if err != nil {
		return nil, err
	}

	var result ElementResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse activeElement result: %w", err)
	}

	elem := uiautomator2.NewCachedElement(
		result.ElementID,
		result.Text,
		uiautomator2.ElementRect{
			X:      result.Bounds.X,
			Y:      result.Bounds.Y,
			Width:  result.Bounds.Width,
			Height: result.Bounds.Height,
		},
	)
	a.wireElementActions(elem, result.ElementID)
	return elem, nil
}

// SendKeysToActive finds the active element and sends text in a single RPC.
func (a *Adapter) SendKeysToActive(text string) error {
	_, err := a.client.Call("Input.sendKeysToActive", map[string]interface{}{
		"text": text,
	})
	return err
}

// wireElementActions sets Click/Clear/SendKeys callbacks on a cached element
// to route through the WebSocket driver.
func (a *Adapter) wireElementActions(elem *uiautomator2.Element, elementID string) {
	elem.SetClickFunc(func() error {
		_, err := a.client.Call("Gesture.click", map[string]interface{}{
			"elementId": elementID,
		})
		return err
	})
	elem.SetClearFunc(func() error {
		_, err := a.client.Call("Input.clearElement", map[string]interface{}{
			"elementId": elementID,
		})
		return err
	})
	elem.SetSendKeysFunc(func(text string) error {
		_, err := a.client.Call("Input.sendKeys", map[string]interface{}{
			"elementId": elementID,
			"text":      text,
		})
		return err
	})
}

// FindFirstOf tries multiple (strategy, selector) pairs in a single RPC,
// returning the first match (in input order). Much cheaper than calling
// FindElement N times since the agent only fetches the accessibility tree
// once (~80-120ms saved per skipped call).
//
// Pairs are passed as flat alternating slices to avoid coupling this package
// to a caller-specific struct type.
func (a *Adapter) FindFirstOf(strategiesAndSelectors []string) (*uiautomator2.Element, error) {
	if len(strategiesAndSelectors)%2 != 0 {
		return nil, fmt.Errorf("findFirstOf: pairs must alternate (strategy, selector)")
	}
	pairs := len(strategiesAndSelectors) / 2
	payload := make([]map[string]string, pairs)
	for i := 0; i < pairs; i++ {
		payload[i] = map[string]string{
			"strategy": strategiesAndSelectors[i*2],
			"selector": strategiesAndSelectors[i*2+1],
		}
	}
	resp, err := a.client.Call("UI.findFirstOf", map[string]interface{}{
		"strategies": payload,
	})
	if err != nil {
		return nil, err
	}
	var result ElementResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse findFirstOf result: %w", err)
	}
	elem := uiautomator2.NewCachedElement(
		result.ElementID,
		result.Text,
		uiautomator2.ElementRect{
			X:      result.Bounds.X,
			Y:      result.Bounds.Y,
			Width:  result.Bounds.Width,
			Height: result.Bounds.Height,
		},
	)
	a.wireElementActions(elem, result.ElementID)
	return elem, nil
}

// FindAndClick finds an element and clicks it in a single RPC call.
func (a *Adapter) FindAndClick(strategy, selector string) (*uiautomator2.Element, error) {
	resp, err := a.client.Call("Gesture.findAndClick", map[string]interface{}{
		"strategy": strategy,
		"selector": selector,
	})
	if err != nil {
		return nil, err
	}

	var result ElementResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse findAndClick result: %w", err)
	}

	elem := uiautomator2.NewCachedElement(
		result.ElementID,
		result.Text,
		uiautomator2.ElementRect{
			X:      result.Bounds.X,
			Y:      result.Bounds.Y,
			Width:  result.Bounds.Width,
			Height: result.Bounds.Height,
		},
	)
	a.wireElementActions(elem, result.ElementID)
	return elem, nil
}

// --- Timeouts ---

// SetImplicitWait sets the implicit wait timeout for element finding.
func (a *Adapter) SetImplicitWait(timeout time.Duration) error {
	_, err := a.client.Call("Settings.update", map[string]interface{}{
		"implicitWait": timeout.Milliseconds(),
	})
	return err
}

// --- Gestures ---

// Click taps at the given coordinates.
func (a *Adapter) Click(x, y int) error {
	_, err := a.client.Call("Gesture.click", map[string]interface{}{
		"x": x, "y": y,
	})
	return err
}

// DoubleClick double-taps at the given coordinates.
func (a *Adapter) DoubleClick(x, y int) error {
	_, err := a.client.Call("Gesture.doubleClick", map[string]interface{}{
		"x": x, "y": y,
	})
	return err
}

// DoubleClickElement double-taps on an element.
func (a *Adapter) DoubleClickElement(elementID string) error {
	_, err := a.client.Call("Gesture.doubleClick", map[string]interface{}{
		"elementId": elementID,
	})
	return err
}

// LongClick long-presses at the given coordinates.
func (a *Adapter) LongClick(x, y, durationMs int) error {
	_, err := a.client.Call("Gesture.longClick", map[string]interface{}{
		"x": x, "y": y, "durationMs": durationMs,
	})
	return err
}

// LongClickElement long-presses on an element.
func (a *Adapter) LongClickElement(elementID string, durationMs int) error {
	_, err := a.client.Call("Gesture.longClick", map[string]interface{}{
		"elementId": elementID, "durationMs": durationMs,
	})
	return err
}

// ScrollInArea performs a scroll gesture in a rectangular area.
func (a *Adapter) ScrollInArea(area uiautomator2.RectModel, direction string, percent float64, speed int) error {
	_, err := a.client.Call("Gesture.scroll", map[string]interface{}{
		"area": map[string]interface{}{
			"left": area.Left, "top": area.Top,
			"width": area.Width, "height": area.Height,
		},
		"direction": direction,
		"percent":   percent,
		"speed":     speed,
	})
	return err
}

// SwipeInArea performs a swipe gesture in a rectangular area.
func (a *Adapter) SwipeInArea(area uiautomator2.RectModel, direction string, percent float64, speed int) error {
	_, err := a.client.Call("Gesture.swipe", map[string]interface{}{
		"area": map[string]interface{}{
			"left": area.Left, "top": area.Top,
			"width": area.Width, "height": area.Height,
		},
		"direction": direction,
		"percent":   percent,
		"speed":     speed,
	})
	return err
}

// --- Navigation ---

// Back presses the back button.
func (a *Adapter) Back() error {
	_, err := a.client.Call("Input.pressKeyCode", map[string]interface{}{
		"keyCode": uiautomator2.KeyCodeBack,
	})
	return err
}

// HideKeyboard hides the on-screen keyboard.
func (a *Adapter) HideKeyboard() error {
	_, err := a.client.Call("Input.hideKeyboard", nil)
	return err
}

// PressKeyCode presses a key by key code.
func (a *Adapter) PressKeyCode(keyCode int) error {
	_, err := a.client.Call("Input.pressKeyCode", map[string]interface{}{
		"keyCode": keyCode,
	})
	return err
}

// SendKeyActions sends text character-by-character via key events.
func (a *Adapter) SendKeyActions(text string) error {
	_, err := a.client.Call("Input.sendKeyActions", map[string]interface{}{
		"text": text,
	})
	return err
}

// --- Device state ---

// Screenshot captures the screen and returns image bytes.
func (a *Adapter) Screenshot() ([]byte, error) {
	resp, err := a.client.Call("UI.screenshot", nil)
	if err != nil {
		return nil, err
	}

	// Binary frame — raw image bytes, no base64 decoding needed
	if resp.BinaryData != nil {
		return resp.BinaryData, nil
	}

	// Fallback: JSON text frame with base64-encoded data
	var result ScreenshotResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse screenshot result: %w", err)
	}

	return base64.StdEncoding.DecodeString(result.Data)
}

// Source returns the UI hierarchy as XML.
func (a *Adapter) Source() (string, error) {
	resp, err := a.client.Call("UI.getSource", nil)
	if err != nil {
		return "", err
	}

	var result SourceResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parse source result: %w", err)
	}

	return result.XML, nil
}

// GetOrientation returns the current device orientation.
func (a *Adapter) GetOrientation() (string, error) {
	resp, err := a.client.Call("Device.getOrientation", nil)
	if err != nil {
		return "", err
	}

	var result OrientationResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parse orientation result: %w", err)
	}

	return result.Orientation, nil
}

// SetOrientation sets the device orientation.
func (a *Adapter) SetOrientation(orientation string) error {
	_, err := a.client.Call("Device.setOrientation", map[string]interface{}{
		"orientation": orientation,
	})
	return err
}

// GetClipboard returns the clipboard text.
func (a *Adapter) GetClipboard() (string, error) {
	resp, err := a.client.Call("Device.getClipboard", nil)
	if err != nil {
		return "", err
	}

	var result ClipboardResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parse clipboard result: %w", err)
	}

	return result.Text, nil
}

// SetClipboard sets the clipboard text.
func (a *Adapter) SetClipboard(text string) error {
	_, err := a.client.Call("Device.setClipboard", map[string]interface{}{
		"text": text,
	})
	return err
}

// GetDeviceInfo returns device information.
func (a *Adapter) GetDeviceInfo() (*uiautomator2.DeviceInfo, error) {
	resp, err := a.client.Call("Device.getInfo", nil)
	if err != nil {
		return nil, err
	}

	var result DeviceResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse device info result: %w", err)
	}

	return &uiautomator2.DeviceInfo{
		Manufacturer:    result.Manufacturer,
		Model:           result.Model,
		Brand:           result.Brand,
		APIVersion:      strconv.Itoa(result.SDK),
		PlatformVersion: result.PlatformVersion,
		RealDisplaySize: result.DisplaySize,
		DisplayDensity:  result.DisplayDensity,
	}, nil
}

// --- App lifecycle ---

// LaunchApp launches an app by package name.
func (a *Adapter) LaunchApp(appID string, arguments map[string]interface{}) error {
	params := map[string]interface{}{"appId": appID}
	if len(arguments) > 0 {
		params["arguments"] = arguments
	}
	_, err := a.client.Call("Device.launchApp", params)
	return err
}

// ForceStop force-stops an app on-device via the driver (no USB roundtrip).
func (a *Adapter) ForceStop(appID string) error {
	_, err := a.client.Call("Device.forceStop", map[string]interface{}{
		"appId": appID,
	})
	return err
}

// ClearAppData clears app data on-device via the driver (no USB roundtrip).
func (a *Adapter) ClearAppData(appID string) error {
	_, err := a.client.Call("Device.clearAppData", map[string]interface{}{
		"appId": appID,
	})
	return err
}

// GrantPermissions grants a list of Android permissions to an app on-device (no USB roundtrip).
func (a *Adapter) GrantPermissions(appID string, permissions []string) error {
	_, err := a.client.Call("Device.grantPermissions", map[string]interface{}{
		"appId":       appID,
		"permissions": permissions,
	})
	return err
}

// --- Settings ---

// SetAppiumSettings configures driver settings.
func (a *Adapter) SetAppiumSettings(settings map[string]interface{}) error {
	_, err := a.client.Call("Settings.update", settings)
	return err
}

// WaitForSettle waits for the UI to settle using accessibility event tracking.
// Returns true if settled within timeout, false if timed out.
func (a *Adapter) WaitForSettle(timeoutMs, quietMs int) (bool, error) {
	resp, err := a.client.Call("UI.waitForSettle", map[string]interface{}{
		"timeout": timeoutMs,
		"quiet":   quietMs,
	})
	if err != nil {
		return false, err
	}

	var result struct {
		Settled bool `json:"settled"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return false, err
	}
	return result.Settled, nil
}

// WaitForWindowUpdate asks the agent to wait for a window-content-changed
// accessibility event on the target package. Returns true if such an event
// fires within timeoutMs (window is animating / updating), false if no
// event fires (window is stable). Mirrors Maestro's isWindowUpdating.
func (a *Adapter) WaitForWindowUpdate(appID string, timeoutMs int) (bool, error) {
	resp, err := a.client.Call("UI.waitForWindowUpdate", map[string]interface{}{
		"appId":   appID,
		"timeout": timeoutMs,
	})
	if err != nil {
		return false, err
	}
	var result struct {
		Updated bool `json:"updated"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return false, err
	}
	return result.Updated, nil
}

// TreeHash returns a hash of the current accessibility tree. Used by lazy
// retry: capture before tap, compare after a failing assertion to detect
// "screen unchanged → tap had no effect → retry". Returned as uint64 but
// the wire type is int64 (Java's signed long) — we convert via bit pattern
// so values are comparable across calls.
func (a *Adapter) TreeHash() (uint64, error) {
	resp, err := a.client.Call("UI.treeHash", map[string]interface{}{})
	if err != nil {
		return 0, err
	}
	var result struct {
		Hash int64 `json:"hash"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return 0, err
	}
	return uint64(result.Hash), nil
}

// --- Session management ---

// CreateSession creates a session on the driver.
func (a *Adapter) CreateSession() (*SessionResult, error) {
	resp, err := a.client.Call("Session.create", map[string]interface{}{
		"capabilities": map[string]interface{}{
			"platformName": "Android",
		},
	})
	if err != nil {
		return nil, err
	}

	var result SessionResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse session result: %w", err)
	}

	return &result, nil
}

// DeleteSession ends the session.
func (a *Adapter) DeleteSession() error {
	_, err := a.client.Call("Session.delete", nil)
	return err
}

// DetectWebView detects if the current screen contains a WebView or browser.
func (a *Adapter) DetectWebView() (*core.WebViewInfo, error) {
	resp, err := a.client.Call("UI.detectWebView", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		HasWebView bool                   `json:"hasWebView"`
		Webview    map[string]interface{} `json:"webview"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}

	if !result.HasWebView {
		return nil, nil
	}

	info := &core.WebViewInfo{}
	if t, ok := result.Webview["type"].(string); ok {
		info.Type = t
	}
	if p, ok := result.Webview["packageName"].(string); ok {
		info.PackageName = p
	}
	if c, ok := result.Webview["className"].(string); ok {
		info.ClassName = c
	}
	return info, nil
}

// Status checks if the driver is ready.
func (a *Adapter) Status() (bool, error) {
	resp, err := a.client.Call("Session.status", nil)
	if err != nil {
		return false, err
	}

	var result struct {
		Ready bool `json:"ready"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return false, err
	}

	return result.Ready, nil
}
