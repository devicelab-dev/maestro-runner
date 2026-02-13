// Package wda provides WebDriverAgent client for iOS automation.
package wda

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// Client is an HTTP client for WebDriverAgent.
type Client struct {
	baseURL    string
	sessionID  string
	httpClient *http.Client
}

// NewClient creates a new WDA client.
func NewClient(port uint16) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://localhost:%d", port),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Session management

// CreateSession creates a new WDA session.
// If alertAction is non-empty ("accept" or "dismiss"), it sets defaultAlertAction
// in the session capabilities, enabling WDA's auto alert handling for permission dialogs.
func (c *Client) CreateSession(bundleID string, alertAction string) error {
	alwaysMatch := map[string]interface{}{
		"bundleId":                bundleID,
		"shouldWaitForQuiescence": false,
		"waitForIdleTimeout":      0,
		"shouldUseTestManagerForVisibilityDetection": false,
	}
	if alertAction != "" {
		alwaysMatch["defaultAlertAction"] = alertAction
	}
	caps := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"alwaysMatch": alwaysMatch,
		},
	}

	resp, err := c.post("/session", caps)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Extract session ID
	if value, ok := resp["value"].(map[string]interface{}); ok {
		if sessionID, ok := value["sessionId"].(string); ok {
			c.sessionID = sessionID
		}
	}
	if c.sessionID == "" {
		if sessionID, ok := resp["sessionId"].(string); ok {
			c.sessionID = sessionID
		}
	}

	return nil
}

// UpdateSettings updates WDA session settings.
// Supports keys like shouldWaitForQuiescence, waitForIdleTimeout, etc.
func (c *Client) UpdateSettings(settings map[string]interface{}) error {
	_, err := c.post(c.sessionPath("/appium/settings"), map[string]interface{}{
		"settings": settings,
	})
	return err
}

// DisableQuiescence disables WDA's wait-for-quiescence behavior.
// This prevents the XCTest "-[XCUIApplicationProcess waitForQuiescenceIncludingAnimationsIdle:]"
// crash on certain Xcode/iOS version combinations.
func (c *Client) DisableQuiescence() error {
	return c.UpdateSettings(map[string]interface{}{
		"shouldWaitForQuiescence": false,
		"waitForIdleTimeout":      0,
	})
}

// DeleteSession ends the current session.
func (c *Client) DeleteSession() error {
	if c.sessionID == "" {
		return nil
	}
	_, err := c.delete(fmt.Sprintf("/session/%s", c.sessionID))
	c.sessionID = ""
	return err
}

// HasSession returns true if a session is active.
func (c *Client) HasSession() bool {
	return c.sessionID != ""
}

// SessionID returns the current session ID.
func (c *Client) SessionID() string {
	return c.sessionID
}

// Status returns WDA status.
func (c *Client) Status() (map[string]interface{}, error) {
	return c.get("/status")
}

// App management

// LaunchApp launches an app by bundle ID.
func (c *Client) LaunchApp(bundleID string) error {
	return c.LaunchAppWithArgs(bundleID, nil, nil)
}

// LaunchAppWithArgs launches an app with optional arguments and environment variables.
func (c *Client) LaunchAppWithArgs(bundleID string, arguments []string, environment map[string]string) error {
	body := map[string]interface{}{
		"bundleId": bundleID,
	}
	if len(arguments) > 0 {
		body["arguments"] = arguments
	}
	if len(environment) > 0 {
		body["environment"] = environment
	}
	_, err := c.post(c.sessionPath("/wda/apps/launch"), body)
	return err
}

// TerminateApp terminates an app by bundle ID.
func (c *Client) TerminateApp(bundleID string) error {
	_, err := c.post(c.sessionPath("/wda/apps/terminate"), map[string]interface{}{
		"bundleId": bundleID,
	})
	return err
}

// ActivateApp brings an app to foreground.
func (c *Client) ActivateApp(bundleID string) error {
	_, err := c.post(c.sessionPath("/wda/apps/activate"), map[string]interface{}{
		"bundleId": bundleID,
	})
	return err
}

// Touch actions

// Tap performs a tap at coordinates.
func (c *Client) Tap(x, y float64) error {
	_, err := c.post(c.sessionPath("/wda/tap"), map[string]interface{}{
		"x": x,
		"y": y,
	})
	return err
}

// DoubleTap performs a double tap at coordinates.
func (c *Client) DoubleTap(x, y float64) error {
	_, err := c.post(c.sessionPath("/wda/doubleTap"), map[string]interface{}{
		"x": x,
		"y": y,
	})
	return err
}

// LongPress performs a long press at coordinates.
func (c *Client) LongPress(x, y float64, durationSec float64) error {
	_, err := c.post(c.sessionPath("/wda/touchAndHold"), map[string]interface{}{
		"x":        x,
		"y":        y,
		"duration": durationSec,
	})
	return err
}

// Swipe performs a swipe gesture.
func (c *Client) Swipe(fromX, fromY, toX, toY float64, durationSec float64) error {
	_, err := c.post(c.sessionPath("/wda/dragfromtoforduration"), map[string]interface{}{
		"fromX":    fromX,
		"fromY":    fromY,
		"toX":      toX,
		"toY":      toY,
		"duration": durationSec,
	})
	return err
}

// Input

// SendKeys types text.
func (c *Client) SendKeys(text string) error {
	_, err := c.post(c.sessionPath("/wda/keys"), map[string]interface{}{
		"value": strings.Split(text, ""),
	})
	return err
}

// ElementSendKeys types text into an element.
func (c *Client) ElementSendKeys(elementID, text string) error {
	_, err := c.post(c.sessionPath(fmt.Sprintf("/element/%s/value", elementID)), map[string]interface{}{
		"value": strings.Split(text, ""),
	})
	return err
}

// ElementClear clears an element's text.
func (c *Client) ElementClear(elementID string) error {
	_, err := c.post(c.sessionPath(fmt.Sprintf("/element/%s/clear", elementID)), nil)
	return err
}

// Screen

// Screenshot captures the screen as PNG.
func (c *Client) Screenshot() ([]byte, error) {
	resp, err := c.get(c.sessionPath("/screenshot"))
	if err != nil {
		return nil, err
	}

	if value, ok := resp["value"].(string); ok {
		return base64Decode(value)
	}
	return nil, fmt.Errorf("invalid screenshot response")
}

// Source returns the UI hierarchy as XML.
func (c *Client) Source() (string, error) {
	resp, err := c.get(c.sessionPath("/source"))
	if err != nil {
		return "", err
	}

	if value, ok := resp["value"].(string); ok {
		return value, nil
	}
	return "", fmt.Errorf("invalid source response")
}

// WindowSize returns the screen dimensions.
func (c *Client) WindowSize() (width, height int, err error) {
	resp, err := c.get(c.sessionPath("/window/size"))
	if err != nil {
		return 0, 0, err
	}

	if value, ok := resp["value"].(map[string]interface{}); ok {
		if w, ok := value["width"].(float64); ok {
			width = int(w)
		}
		if h, ok := value["height"].(float64); ok {
			height = int(h)
		}
		return width, height, nil
	}
	return 0, 0, fmt.Errorf("invalid window size response")
}

// Device control

// PressButton presses a hardware button (home, volumeUp, volumeDown).
func (c *Client) PressButton(button string) error {
	_, err := c.post(c.sessionPath("/wda/pressButton"), map[string]interface{}{
		"name": button,
	})
	return err
}

// Home presses the home button.
func (c *Client) Home() error {
	return c.PressButton("home")
}

// Lock locks the device.
func (c *Client) Lock() error {
	_, err := c.post(c.sessionPath("/wda/lock"), nil)
	return err
}

// Unlock unlocks the device.
func (c *Client) Unlock() error {
	_, err := c.post(c.sessionPath("/wda/unlock"), nil)
	return err
}

// AcceptAlert accepts the current system alert (taps Allow/OK).
func (c *Client) AcceptAlert() error {
	_, err := c.post(c.sessionPath("/alert/accept"), nil)
	return err
}

// DismissAlert dismisses the current system alert (taps Don't Allow/Cancel).
func (c *Client) DismissAlert() error {
	_, err := c.post(c.sessionPath("/alert/dismiss"), nil)
	return err
}

// GetOrientation returns the current orientation.
func (c *Client) GetOrientation() (string, error) {
	resp, err := c.get(c.sessionPath("/orientation"))
	if err != nil {
		return "", err
	}
	if value, ok := resp["value"].(string); ok {
		return value, nil
	}
	return "", fmt.Errorf("invalid orientation response")
}

// SetOrientation sets the device orientation.
func (c *Client) SetOrientation(orientation string) error {
	_, err := c.post(c.sessionPath("/orientation"), map[string]interface{}{
		"orientation": orientation,
	})
	return err
}

// DeepLink opens a URL or deep link on the device.
// Works for both simulator and real device via WDA.
func (c *Client) DeepLink(url string) error {
	_, err := c.post(c.sessionPath("/url"), map[string]interface{}{
		"url": url,
	})
	return err
}

// Element finding

// FindElement finds a single element.
func (c *Client) FindElement(using, value string) (string, error) {
	resp, err := c.post(c.sessionPath("/element"), map[string]interface{}{
		"using": using,
		"value": value,
	})
	if err != nil {
		return "", err
	}

	if val, ok := resp["value"].(map[string]interface{}); ok {
		if elem, ok := val["ELEMENT"].(string); ok {
			return elem, nil
		}
		// W3C format
		for k, v := range val {
			if str, ok := v.(string); ok && k != "error" {
				return str, nil
			}
		}
	}
	return "", fmt.Errorf("element not found")
}

// FindElements finds multiple elements.
func (c *Client) FindElements(using, value string) ([]string, error) {
	resp, err := c.post(c.sessionPath("/elements"), map[string]interface{}{
		"using": using,
		"value": value,
	})
	if err != nil {
		return nil, err
	}

	var elements []string
	if val, ok := resp["value"].([]interface{}); ok {
		for _, elem := range val {
			if m, ok := elem.(map[string]interface{}); ok {
				if id, ok := m["ELEMENT"].(string); ok {
					elements = append(elements, id)
				} else {
					// W3C format
					for _, v := range m {
						if str, ok := v.(string); ok {
							elements = append(elements, str)
							break
						}
					}
				}
			}
		}
	}
	return elements, nil
}

// ElementClick clicks an element.
func (c *Client) ElementClick(elementID string) error {
	_, err := c.post(c.sessionPath(fmt.Sprintf("/element/%s/click", elementID)), nil)
	return err
}

// ElementName returns the element's tag name (e.g., "XCUIElementTypeTextField").
func (c *Client) ElementName(elementID string) (string, error) {
	resp, err := c.get(c.sessionPath(fmt.Sprintf("/element/%s/name", elementID)))
	if err != nil {
		return "", err
	}
	if value, ok := resp["value"].(string); ok {
		return value, nil
	}
	return "", fmt.Errorf("invalid element name response")
}

// ElementText returns an element's text.
func (c *Client) ElementText(elementID string) (string, error) {
	resp, err := c.get(c.sessionPath(fmt.Sprintf("/element/%s/text", elementID)))
	if err != nil {
		return "", err
	}
	if value, ok := resp["value"].(string); ok {
		return value, nil
	}
	return "", nil
}

// ElementDisplayed checks if an element is visible.
func (c *Client) ElementDisplayed(elementID string) (bool, error) {
	resp, err := c.get(c.sessionPath(fmt.Sprintf("/element/%s/displayed", elementID)))
	if err != nil {
		return false, err
	}
	if value, ok := resp["value"].(bool); ok {
		return value, nil
	}
	return false, nil
}

// ElementRect returns an element's bounds.
func (c *Client) ElementRect(elementID string) (x, y, width, height int, err error) {
	resp, err := c.get(c.sessionPath(fmt.Sprintf("/element/%s/rect", elementID)))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if value, ok := resp["value"].(map[string]interface{}); ok {
		if v, ok := value["x"].(float64); ok {
			x = int(v)
		}
		if v, ok := value["y"].(float64); ok {
			y = int(v)
		}
		if v, ok := value["width"].(float64); ok {
			width = int(v)
		}
		if v, ok := value["height"].(float64); ok {
			height = int(v)
		}
	}
	return x, y, width, height, nil
}

// GetActiveElement returns the currently focused element ID.
func (c *Client) GetActiveElement() (string, error) {
	resp, err := c.get(c.sessionPath("/element/active"))
	if err != nil {
		return "", err
	}
	if value, ok := resp["value"].(map[string]interface{}); ok {
		// WDA returns element ID as ELEMENT key
		if elemID, ok := value["ELEMENT"].(string); ok {
			return elemID, nil
		}
	}
	return "", fmt.Errorf("no active element")
}

// HTTP helpers

func (c *Client) sessionPath(path string) string {
	if c.sessionID != "" {
		return fmt.Sprintf("/session/%s%s", c.sessionID, path)
	}
	return path
}

func (c *Client) get(path string) (map[string]interface{}, error) {
	start := time.Now()
	logger.Debug("WDA GET %s", path)

	resp, err := c.httpClient.Get(c.baseURL + path)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		logger.Error("WDA GET %s failed (%dms): %v", path, duration, err)
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Debug("failed to close response body for GET %s: %v", path, err)
		}
	}()

	logger.Debug("WDA GET %s completed (%dms, status: %d)", path, duration, resp.StatusCode)
	return c.parseResponse(resp)
}

func (c *Client) post(path string, body interface{}) (map[string]interface{}, error) {
	start := time.Now()
	var reqBody io.Reader
	bodyStr := ""
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
		bodyStr = string(data)
		if len(bodyStr) > 100 {
			bodyStr = bodyStr[:100] + "..."
		}
	}

	logger.Debug("WDA POST %s body=%s", path, bodyStr)

	resp, err := c.httpClient.Post(c.baseURL+path, "application/json", reqBody)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		logger.Error("WDA POST %s failed (%dms): %v", path, duration, err)
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Debug("failed to close response body for POST %s: %v", path, err)
		}
	}()

	logger.Debug("WDA POST %s completed (%dms, status: %d)", path, duration, resp.StatusCode)
	return c.parseResponse(resp)
}

func (c *Client) delete(path string) (map[string]interface{}, error) {
	start := time.Now()
	logger.Debug("WDA DELETE %s", path)

	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		logger.Error("WDA DELETE %s failed (%dms): %v", path, duration, err)
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Debug("failed to close response body for DELETE %s: %v", path, err)
		}
	}()
	return c.parseResponse(resp)
}

func (c *Client) parseResponse(resp *http.Response) (map[string]interface{}, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (body: %s)", err, string(body))
	}

	// Check for WDA error
	if value, ok := result["value"].(map[string]interface{}); ok {
		if errMsg, ok := value["error"].(string); ok {
			message := errMsg
			if msg, ok := value["message"].(string); ok {
				message = msg
			}
			return nil, fmt.Errorf("WDA error: %s", message)
		}
	}

	return result, nil
}

func base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
