package uiautomator2

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Back presses the back button.
func (c *Client) Back() error {
	_, err := c.request("POST", c.sessionPath("/back"), nil)
	return err
}

// PressKeyCode presses a key by key code.
func (c *Client) PressKeyCode(keyCode int) error {
	req := KeyCodeRequest{KeyCode: keyCode}
	_, err := c.request("POST", c.sessionPath("/appium/device/press_keycode"), req)
	return err
}

// LongPressKeyCode long-presses a key.
func (c *Client) LongPressKeyCode(keyCode int) error {
	req := KeyCodeRequest{KeyCode: keyCode}
	_, err := c.request("POST", c.sessionPath("/appium/device/long_press_keycode"), req)
	return err
}

// OpenNotifications opens the notification shade.
func (c *Client) OpenNotifications() error {
	_, err := c.request("POST", c.sessionPath("/appium/device/open_notifications"), nil)
	return err
}

// GetClipboard returns the clipboard text.
func (c *Client) GetClipboard() (string, error) {
	data, err := c.request("POST", c.sessionPath("/appium/device/get_clipboard"), nil)
	if err != nil {
		return "", err
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	b64, ok := resp.Value.(string)
	if !ok {
		return "", nil
	}

	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return b64, nil // Return as-is if not base64
	}
	return string(decoded), nil
}

// SetClipboard sets the clipboard text.
func (c *Client) SetClipboard(text string) error {
	req := ClipboardRequest{
		Content:     base64.StdEncoding.EncodeToString([]byte(text)),
		ContentType: "plaintext",
	}
	_, err := c.request("POST", c.sessionPath("/appium/device/set_clipboard"), req)
	return err
}

// GetDeviceInfo returns device information.
func (c *Client) GetDeviceInfo() (*DeviceInfo, error) {
	data, err := c.request("GET", c.sessionPath("/appium/device/info"), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Value DeviceInfo `json:"value"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return &resp.Value, nil
}

// GetBatteryInfo returns battery information.
func (c *Client) GetBatteryInfo() (*BatteryInfo, error) {
	data, err := c.request("GET", c.sessionPath("/appium/device/battery_info"), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Value BatteryInfo `json:"value"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return &resp.Value, nil
}

// Screenshot captures the screen and returns PNG bytes.
func (c *Client) Screenshot() ([]byte, error) {
	data, err := c.request("GET", c.sessionPath("/screenshot"), nil)
	if err != nil {
		return nil, err
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	b64, ok := resp.Value.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected screenshot response")
	}

	return decodeBase64(b64)
}

// Source returns the UI hierarchy as XML.
func (c *Client) Source() (string, error) {
	data, err := c.request("GET", c.sessionPath("/source"), nil)
	if err != nil {
		return "", err
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	source, _ := resp.Value.(string)
	return source, nil
}

// GetOrientation returns the current orientation.
func (c *Client) GetOrientation() (string, error) {
	data, err := c.request("GET", c.sessionPath("/orientation"), nil)
	if err != nil {
		return "", err
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	orientation, _ := resp.Value.(string)
	return orientation, nil
}

// SetOrientation sets the orientation.
func (c *Client) SetOrientation(orientation string) error {
	req := OrientationRequest{Orientation: orientation}
	_, err := c.request("POST", c.sessionPath("/orientation"), req)
	return err
}

// GetAlertText returns the current alert text.
func (c *Client) GetAlertText() (string, error) {
	data, err := c.request("GET", c.sessionPath("/alert/text"), nil)
	if err != nil {
		return "", err
	}

	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	text, _ := resp.Value.(string)
	return text, nil
}

// AcceptAlert accepts the current alert.
func (c *Client) AcceptAlert() error {
	_, err := c.request("POST", c.sessionPath("/alert/accept"), nil)
	return err
}

// DismissAlert dismisses the current alert.
func (c *Client) DismissAlert() error {
	_, err := c.request("POST", c.sessionPath("/alert/dismiss"), nil)
	return err
}

// GetSettings returns the current settings.
func (c *Client) GetSettings() (map[string]interface{}, error) {
	data, err := c.request("GET", c.sessionPath("/appium/settings"), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Value map[string]interface{} `json:"value"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return resp.Value, nil
}

// UpdateSettings updates settings.
func (c *Client) UpdateSettings(settings map[string]interface{}) error {
	req := SettingsRequest{Settings: settings}
	_, err := c.request("POST", c.sessionPath("/appium/settings"), req)
	return err
}

// decodeBase64 decodes a base64 string to bytes.
func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
