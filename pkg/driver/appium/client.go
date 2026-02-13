// Package appium implements core.Driver using Appium server via W3C WebDriver protocol.
package appium

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// W3C WebDriver element identifier key (standard constant)
const w3cElementKey = "element-6066-11e4-a52e-4f735466cecf"

// Client handles HTTP communication with Appium server.
type Client struct {
	serverURL    string
	sessionID    string
	client       *http.Client
	platform     string // ios, android
	screenW      int
	screenH      int
	isRealDevice bool // true for physical devices, false for simulators
}

// NewClient creates a new Appium client.
func NewClient(serverURL string) *Client {
	return &Client{
		serverURL: strings.TrimSuffix(serverURL, "/"),
		client: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for install/screenshot
		},
	}
}

// Connect creates a new session with the given capabilities.
func (c *Client) Connect(capabilities map[string]interface{}) error {
	// For iOS: disable auto-launch to prevent double-launch (once by Appium session,
	// once by flow's launchApp). Double-launch causes duplicate permission alerts
	// that interfere with element focus during text input.
	if p, ok := capabilities["platformName"].(string); ok && strings.EqualFold(p, "ios") {
		capabilities["appium:autoLaunch"] = false
	}

	// For Android with clearState (noReset=false): disable auto-launch so we can
	// grant permissions via pm grant before the app starts (avoids permission popups).
	// When noReset is true (default), permissions persist across sessions so this isn't needed.
	var androidAppPackage, androidAppActivity string
	if p, ok := capabilities["platformName"].(string); ok && strings.EqualFold(p, "android") {
		if noReset, ok := capabilities["appium:noReset"].(bool); ok && !noReset {
			if pkg, ok := capabilities["appium:appPackage"].(string); ok && pkg != "" {
				androidAppPackage = pkg
				androidAppActivity, _ = capabilities["appium:appActivity"].(string)
				capabilities["appium:autoLaunch"] = false
			}
		}
	}

	body := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"alwaysMatch": capabilities,
		},
	}

	resp, err := c.post("/session", body)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	value, ok := resp["value"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid session response")
	}

	c.sessionID, _ = value["sessionId"].(string)
	if c.sessionID == "" {
		return fmt.Errorf("no session ID in response")
	}

	// Extract platform and device type from capabilities
	if caps, ok := value["capabilities"].(map[string]interface{}); ok {
		if platform, ok := caps["platformName"].(string); ok {
			c.platform = strings.ToLower(platform)
		}
		// Detect real device vs simulator from session response
		if isReal, ok := caps["isRealDevice"].(bool); ok {
			c.isRealDevice = isReal
		} else if udid, ok := capabilities["appium:udid"].(string); ok && udid != "" {
			// Simulator UDIDs are UUID format (8-4-4-4-12 hex with dashes)
			c.isRealDevice = !isUUIDFormat(udid)
		}
	}

	// Get screen size
	c.fetchScreenSize()

	// Configure UiAutomator2/XCUITest settings for faster element finding
	// Extract waitForIdleTimeout from appium:settings capability if provided
	waitForIdleTimeout := 0 // default to 0 (disabled) for backward compatibility
	if settings, ok := capabilities["appium:settings"].(map[string]interface{}); ok {
		if val, ok := settings["waitForIdleTimeout"].(int); ok {
			waitForIdleTimeout = val
		} else if val, ok := settings["waitForIdleTimeout"].(float64); ok {
			waitForIdleTimeout = int(val)
		}
	}

	if c.platform == "ios" {
		// iOS XCUITest settings:
		// - animationCoolOffTimeout: Don't wait for animations to finish (default 2s)
		if err := c.SetSettings(map[string]interface{}{
			"waitForIdleTimeout":      waitForIdleTimeout,
			"animationCoolOffTimeout": 0,
		}); err != nil {
			logger.Warn("failed to configure iOS XCUITest settings: %v", err)
		}
	} else {
		// Android UiAutomator2 settings:
		// - waitForSelectorTimeout: Don't add extra wait when finding elements (default 0)
		if err := c.SetSettings(map[string]interface{}{
			"waitForIdleTimeout":     waitForIdleTimeout,
			"waitForSelectorTimeout": 0,
		}); err != nil {
			logger.Warn("failed to configure Android UiAutomator2 settings: %v", err)
		}

		// Grant all permissions and launch app (autoLaunch was disabled above)
		if androidAppPackage != "" {
			for _, perm := range getAllPermissions() {
				// Ignore errors - permission might not be declared by the app
				if _, err := c.ExecuteMobile("shell", map[string]interface{}{
					"command": "pm",
					"args":    []string{"grant", androidAppPackage, perm},
				}); err != nil {
					logger.Debug("grant %s failed (expected if not declared): %v", perm, err)
				}
			}
			if androidAppActivity != "" {
				if _, err := c.ExecuteMobile("startActivity", map[string]interface{}{
					"appPackage":  androidAppPackage,
					"appActivity": androidAppActivity,
				}); err != nil {
					logger.Warn("failed to start activity %s/%s: %v", androidAppPackage, androidAppActivity, err)
				}
			} else {
				if err := c.LaunchApp(androidAppPackage); err != nil {
					logger.Warn("failed to launch app %s: %v", androidAppPackage, err)
				}
			}
		}
	}

	return nil
}

// Disconnect closes the session.
func (c *Client) Disconnect() error {
	if c.sessionID == "" {
		return nil
	}
	_, err := c.delete(c.sessionPath())
	c.sessionID = ""
	return err
}

// Platform returns the platform (ios/android).
func (c *Client) Platform() string {
	return c.platform
}

// ScreenSize returns the screen dimensions.
func (c *Client) ScreenSize() (int, int) {
	return c.screenW, c.screenH
}

func (c *Client) fetchScreenSize() {
	resp, err := c.get(c.sessionPath() + "/window/rect")
	if err != nil {
		return
	}
	if value, ok := resp["value"].(map[string]interface{}); ok {
		if w, ok := value["width"].(float64); ok {
			c.screenW = int(w)
		}
		if h, ok := value["height"].(float64); ok {
			c.screenH = int(h)
		}
	}
}

// Element Operations

// FindElement finds a single element.
func (c *Client) FindElement(strategy, value string) (string, error) {
	body := map[string]interface{}{
		"using": strategy,
		"value": value,
	}

	resp, err := c.post(c.sessionPath()+"/element", body)
	if err != nil {
		return "", err
	}

	elemValue, ok := resp["value"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("element not found")
	}

	// Check for error
	if errMsg, ok := elemValue["error"].(string); ok {
		return "", fmt.Errorf("%s", errMsg)
	}

	return extractElementID(elemValue), nil
}

// FindElements finds multiple elements.
func (c *Client) FindElements(strategy, value string) ([]string, error) {
	body := map[string]interface{}{
		"using": strategy,
		"value": value,
	}

	resp, err := c.post(c.sessionPath()+"/elements", body)
	if err != nil {
		return nil, err
	}

	values, ok := resp["value"].([]interface{})
	if !ok {
		return nil, nil
	}

	var ids []string
	for _, v := range values {
		if elem, ok := v.(map[string]interface{}); ok {
			if id := extractElementID(elem); id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

// GetActiveElement returns the currently focused element.
func (c *Client) GetActiveElement() (string, error) {
	resp, err := c.post(c.sessionPath()+"/element/active", nil)
	if err != nil {
		return "", err
	}
	if value, ok := resp["value"].(map[string]interface{}); ok {
		return extractElementID(value), nil
	}
	return "", fmt.Errorf("no active element")
}

// ClickElement clicks an element using WebDriver standard endpoint.
func (c *Client) ClickElement(elementID string) error {
	_, err := c.post(c.elementPath(elementID)+"/click", nil)
	return err
}

// ElementSendKeys types text into a specific element.
func (c *Client) ElementSendKeys(elementID, text string) error {
	_, err := c.post(c.elementPath(elementID)+"/value", map[string]interface{}{
		"value": strings.Split(text, ""),
		"text":  text,
	})
	return err
}

// ClearElement clears an element's text.
func (c *Client) ClearElement(elementID string) error {
	_, err := c.post(c.elementPath(elementID)+"/clear", nil)
	return err
}

// GetElementText returns an element's text.
func (c *Client) GetElementText(elementID string) (string, error) {
	resp, err := c.get(c.elementPath(elementID) + "/text")
	if err != nil {
		return "", err
	}
	text, _ := resp["value"].(string)
	return text, nil
}

// GetElementAttribute returns an element's attribute value.
func (c *Client) GetElementAttribute(elementID, name string) (string, error) {
	resp, err := c.get(c.elementPath(elementID) + "/attribute/" + name)
	if err != nil {
		return "", err
	}
	value, _ := resp["value"].(string)
	return value, nil
}

// GetElementRect returns an element's position and size.
func (c *Client) GetElementRect(elementID string) (x, y, w, h int, err error) {
	resp, err := c.get(c.elementPath(elementID) + "/rect")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	value, ok := resp["value"].(map[string]interface{})
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("invalid rect response")
	}

	xf, _ := value["x"].(float64)
	yf, _ := value["y"].(float64)
	wf, _ := value["width"].(float64)
	hf, _ := value["height"].(float64)
	return int(xf), int(yf), int(wf), int(hf), nil
}

// IsElementDisplayed checks if element is visible.
func (c *Client) IsElementDisplayed(elementID string) (bool, error) {
	resp, err := c.get(c.elementPath(elementID) + "/displayed")
	if err != nil {
		return false, err
	}
	displayed, _ := resp["value"].(bool)
	return displayed, nil
}

// IsElementEnabled checks if element is enabled.
func (c *Client) IsElementEnabled(elementID string) (bool, error) {
	resp, err := c.get(c.elementPath(elementID) + "/enabled")
	if err != nil {
		return false, err
	}
	enabled, _ := resp["value"].(bool)
	return enabled, nil
}

// Touch/Gesture Operations (W3C Actions)

func (c *Client) performTouchAction(actions []map[string]interface{}) error {
	payload := []map[string]interface{}{
		{
			"type":       "pointer",
			"id":         "finger1",
			"parameters": map[string]interface{}{"pointerType": "touch"},
			"actions":    actions,
		},
	}
	_, err := c.post(c.sessionPath()+"/actions", map[string]interface{}{"actions": payload})
	return err
}

// Tap performs a tap at coordinates using W3C touch actions.
func (c *Client) Tap(x, y int) error {
	return c.performTouchAction([]map[string]interface{}{
		{"type": "pointerMove", "duration": 0, "x": x, "y": y, "origin": "viewport"},
		{"type": "pointerDown", "button": 0},
		{"type": "pause", "duration": 50},
		{"type": "pointerUp", "button": 0},
	})
}

// TapElement performs a tap on an element using W3C touch actions with element origin.
func (c *Client) TapElement(elementID string) error {
	return c.performTouchAction([]map[string]interface{}{
		{
			"type":     "pointerMove",
			"duration": 0,
			"x":        0,
			"y":        0,
			"origin":   map[string]interface{}{w3cElementKey: elementID},
		},
		{"type": "pointerDown", "button": 0},
		{"type": "pause", "duration": 50},
		{"type": "pointerUp", "button": 0},
	})
}

// DoubleTap performs a double tap at coordinates.
func (c *Client) DoubleTap(x, y int) error {
	return c.performTouchAction([]map[string]interface{}{
		{"type": "pointerMove", "duration": 0, "x": x, "y": y},
		{"type": "pointerDown", "button": 0},
		{"type": "pointerUp", "button": 0},
		{"type": "pause", "duration": 100},
		{"type": "pointerDown", "button": 0},
		{"type": "pointerUp", "button": 0},
	})
}

// LongPress performs a long press at coordinates.
func (c *Client) LongPress(x, y, durationMs int) error {
	return c.performTouchAction([]map[string]interface{}{
		{"type": "pointerMove", "duration": 0, "x": x, "y": y},
		{"type": "pointerDown", "button": 0},
		{"type": "pause", "duration": durationMs},
		{"type": "pointerUp", "button": 0},
	})
}

// Swipe performs a swipe gesture.
func (c *Client) Swipe(startX, startY, endX, endY, durationMs int) error {
	return c.performTouchAction([]map[string]interface{}{
		{"type": "pointerMove", "duration": 0, "x": startX, "y": startY},
		{"type": "pointerDown", "button": 0},
		{"type": "pointerMove", "duration": durationMs, "x": endX, "y": endY},
		{"type": "pointerUp", "button": 0},
	})
}

// Text Input

// SendKeys sends text to the active element.
func (c *Client) SendKeys(text string) error {
	// Build key actions
	var keyActions []map[string]interface{}
	for _, ch := range text {
		keyActions = append(keyActions,
			map[string]interface{}{"type": "keyDown", "value": string(ch)},
			map[string]interface{}{"type": "keyUp", "value": string(ch)},
		)
	}

	_, err := c.post(c.sessionPath()+"/actions", map[string]interface{}{
		"actions": []map[string]interface{}{
			{
				"type":    "key",
				"id":      "keyboard",
				"actions": keyActions,
			},
		},
	})
	if err != nil {
		// Fallback: Appium element value endpoint
		_, err = c.post(c.sessionPath()+"/appium/element/active/value", map[string]interface{}{
			"text": text,
		})
	}
	return err
}

// HideKeyboard hides the on-screen keyboard.
func (c *Client) HideKeyboard() error {
	_, err := c.post(c.sessionPath()+"/appium/device/hide_keyboard", nil)
	return err
}

// Navigation

// Back presses the back button.
func (c *Client) Back() error {
	return c.PressKeyCode(4) // Android KEYCODE_BACK
}

// PressKeyCode presses a key by keycode (Android).
func (c *Client) PressKeyCode(keycode int) error {
	_, err := c.post(c.sessionPath()+"/appium/device/press_keycode", map[string]interface{}{
		"keycode": keycode,
	})
	return err
}

// App Management

// LaunchApp activates an app.
func (c *Client) LaunchApp(appID string) error {
	body := make(map[string]interface{})
	if c.platform == "ios" {
		body["bundleId"] = appID
	} else {
		body["appId"] = appID
	}
	_, err := c.post(c.sessionPath()+"/appium/device/activate_app", body)
	return err
}

// TerminateApp terminates an app.
func (c *Client) TerminateApp(appID string) error {
	body := make(map[string]interface{})
	if c.platform == "ios" {
		body["bundleId"] = appID
	} else {
		body["appId"] = appID
	}
	_, err := c.post(c.sessionPath()+"/appium/device/terminate_app", body)
	return err
}

// ClearAppData clears app data.
func (c *Client) ClearAppData(appID string) error {
	if err := c.TerminateApp(appID); err != nil {
		logger.Warn("failed to terminate app %s before clearing data: %v", appID, err)
	}

	if c.platform == "ios" {
		if c.isRealDevice {
			// Real iOS devices: mobile: clearApp is not supported.
			// Just terminate (done above) — state won't be fully cleared.
			logger.Warn("clearState on real iOS device via Appium: app state cannot be fully cleared (mobile: clearApp only works on simulators)")
			return nil
		}
		// Simulator: use mobile: clearApp
		_, err := c.post(c.sessionPath()+"/execute/sync", map[string]interface{}{
			"script": "mobile: clearApp",
			"args":   []interface{}{map[string]interface{}{"bundleId": appID}},
		})
		return err
	}

	// Android: use mobile: shell to run pm clear (same as native driver)
	_, err := c.post(c.sessionPath()+"/execute/sync", map[string]interface{}{
		"script": "mobile: shell",
		"args": []interface{}{map[string]interface{}{
			"command": "pm",
			"args":    []string{"clear", appID},
		}},
	})
	return err
}

// Screen Operations

// Screenshot returns a screenshot as PNG bytes.
func (c *Client) Screenshot() ([]byte, error) {
	resp, err := c.get(c.sessionPath() + "/screenshot")
	if err != nil {
		return nil, err
	}
	encoded, ok := resp["value"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid screenshot response")
	}
	return base64.StdEncoding.DecodeString(encoded)
}

// Source returns the page source XML.
func (c *Client) Source() (string, error) {
	resp, err := c.get(c.sessionPath() + "/source")
	if err != nil {
		return "", err
	}
	source, _ := resp["value"].(string)
	return source, nil
}

// Orientation

// GetOrientation returns the current orientation.
func (c *Client) GetOrientation() (string, error) {
	resp, err := c.get(c.sessionPath() + "/orientation")
	if err != nil {
		return "", err
	}
	orientation, _ := resp["value"].(string)
	return strings.ToLower(orientation), nil
}

// SetOrientation sets the orientation.
func (c *Client) SetOrientation(orientation string) error {
	_, err := c.post(c.sessionPath()+"/orientation", map[string]interface{}{
		"orientation": strings.ToUpper(orientation),
	})
	return err
}

// Location

// SetLocation sets the device location.
func (c *Client) SetLocation(lat, lon float64) error {
	_, err := c.post(c.sessionPath()+"/location", map[string]interface{}{
		"location": map[string]interface{}{
			"latitude":  lat,
			"longitude": lon,
			"altitude":  0,
		},
	})
	return err
}

// Clipboard

// GetClipboard returns clipboard text.
func (c *Client) GetClipboard() (string, error) {
	resp, err := c.post(c.sessionPath()+"/appium/device/get_clipboard", map[string]interface{}{
		"contentType": "plaintext",
	})
	if err != nil {
		return "", err
	}
	encoded, _ := resp["value"].(string)
	decoded, _ := base64.StdEncoding.DecodeString(encoded)
	return string(decoded), nil
}

// SetClipboard sets clipboard text.
func (c *Client) SetClipboard(text string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	_, err := c.post(c.sessionPath()+"/appium/device/set_clipboard", map[string]interface{}{
		"content":     encoded,
		"contentType": "plaintext",
	})
	return err
}

// Deep Links

// OpenURL opens a URL.
func (c *Client) OpenURL(url string) error {
	_, err := c.post(c.sessionPath()+"/url", map[string]interface{}{
		"url": url,
	})
	return err
}

// Timeouts

// SetImplicitWait sets the implicit wait timeout.
func (c *Client) SetImplicitWait(timeout time.Duration) error {
	_, err := c.post(c.sessionPath()+"/timeouts", map[string]interface{}{
		"implicit": timeout.Milliseconds(),
	})
	return err
}

// SetSettings updates Appium driver settings.
// For Android UiAutomator2: waitForIdleTimeout, waitForSelectorTimeout
// For iOS XCUITest: snapshotMaxDepth, customSnapshotTimeout
func (c *Client) SetSettings(settings map[string]interface{}) error {
	_, err := c.post(c.sessionPath()+"/appium/settings", map[string]interface{}{
		"settings": settings,
	})
	return err
}

// ExecuteMobile executes a mobile: command.
func (c *Client) ExecuteMobile(command string, args map[string]interface{}) (interface{}, error) {
	resp, err := c.post(c.sessionPath()+"/execute/sync", map[string]interface{}{
		"script": "mobile: " + command,
		"args":   []interface{}{args},
	})
	if err != nil {
		return nil, err
	}
	return resp["value"], nil
}

// HTTP Helpers

func (c *Client) sessionPath() string {
	return "/session/" + c.sessionID
}

func (c *Client) elementPath(elementID string) string {
	return c.sessionPath() + "/element/" + elementID
}

func (c *Client) get(path string) (map[string]interface{}, error) {
	return c.request("GET", path, nil)
}

func (c *Client) post(path string, body interface{}) (map[string]interface{}, error) {
	return c.request("POST", path, body)
}

func (c *Client) delete(path string) (map[string]interface{}, error) {
	return c.request("DELETE", path, nil)
}

func (c *Client) request(method, path string, body interface{}) (map[string]interface{}, error) {
	start := time.Now()
	url := c.serverURL + path

	var bodyReader io.Reader
	bodyStr := ""
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
		bodyStr = string(jsonBody)
		if len(bodyStr) > 100 {
			bodyStr = bodyStr[:100] + "..."
		}
	}

	logger.Debug("Appium %s %s body=%s", method, path, bodyStr)

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		logger.Error("Appium %s %s failed (%dms): %v", method, path, duration, err)
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("nil response from server")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Debug("failed to close response body for %s %s: %v", method, path, err)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for WebDriver error
	if errValue, ok := result["value"].(map[string]interface{}); ok {
		if errMsg, ok := errValue["message"].(string); ok {
			if errType, ok := errValue["error"].(string); ok {
				logger.Error("Appium %s %s returned error (%dms): %s: %s", method, path, duration, errType, errMsg)
				return result, fmt.Errorf("%s: %s", errType, errMsg)
			}
		}
	}

	logger.Debug("Appium %s %s completed (%dms, status: %d)", method, path, duration, resp.StatusCode)
	return result, nil
}

func extractElementID(value map[string]interface{}) string {
	// W3C format
	if id, ok := value[w3cElementKey].(string); ok {
		return id
	}
	// Legacy format
	if id, ok := value["ELEMENT"].(string); ok {
		return id
	}
	return ""
}

// uuidRegex matches simulator UDIDs (e.g. "EB69B42A-4763-4A33-AF0F-CD233F721951").
var uuidRegex = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

// isUUIDFormat returns true if s matches the UUID format used by iOS simulators.
func isUUIDFormat(s string) bool {
	return uuidRegex.MatchString(s)
}
