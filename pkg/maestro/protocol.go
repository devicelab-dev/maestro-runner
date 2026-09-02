// Package maestro provides a WebSocket-based client for the DeviceLab on-device driver.
// It implements the UIA2Client interface from pkg/driver/uiautomator2 using a
// bidirectional WebSocket protocol instead of HTTP, enabling batched operations
// and push events for reduced latency.
package maestro

import "encoding/json"

// Request is a message sent from host to device.
type Request struct {
	ID     int64       `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

// Response is a message sent from device to host, matched by request ID.
type Response struct {
	ID         int64           `json:"id"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      *ErrorPayload   `json:"error,omitempty"`
	BinaryData []byte          `json:"-"` // Raw binary payload (e.g., screenshot), not JSON-encoded
}

// Event is an unsolicited push message from device to host.
type Event struct {
	Event  string          `json:"event"`
	Params json.RawMessage `json:"params,omitempty"`
}

// ErrorPayload carries error details in a Response.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ErrorPayload) Error() string {
	return e.Code + ": " + e.Message
}

// rawMessage is used to detect whether an incoming JSON frame
// is a Response (has "id") or an Event (has "event").
type rawMessage struct {
	ID    *int64          `json:"id,omitempty"`
	Event string          `json:"event,omitempty"`
	Raw   json.RawMessage `json:"-"`
}

// ElementResult is the result returned by UI.findElement.
// It bundles element ID with commonly needed attributes to avoid extra round-trips.
type ElementResult struct {
	ElementID   string       `json:"elementId"`
	Text        string       `json:"text"`
	ContentDesc string       `json:"contentDesc"`
	ClassName   string       `json:"className"`
	ResourceID  string       `json:"resourceId"`
	Bounds      BoundsResult `json:"bounds"`
	Displayed   bool         `json:"displayed"`
	Enabled     bool         `json:"enabled"`
	Clickable   bool         `json:"clickable"`
	Selected    bool         `json:"selected"`

	// Clicked reports whether findAndClick actually injected the tap. It is a
	// pointer because absent and false mean different things: an agent built
	// before the #162 guard never sends the field and always clicked, so nil
	// must be read as "clicked", not as "skipped".
	Clicked *bool `json:"clicked"`

	// BlockedBy names what covered the tap point when the hit test refused the
	// tap ("keyboard window", `android.widget.Button "OK"`). Empty when the tap
	// was skipped on geometry alone, or not skipped at all.
	BlockedBy string `json:"blockedBy"`
}

// BoundsResult represents element bounds.
type BoundsResult struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// SessionResult is returned by Session.create.
type SessionResult struct {
	SessionID  string       `json:"sessionId"`
	DeviceInfo DeviceResult `json:"deviceInfo"`
}

// DeviceResult is returned by Device.getInfo and Session.create.
type DeviceResult struct {
	Model           string `json:"model"`
	Manufacturer    string `json:"manufacturer"`
	Brand           string `json:"brand"`
	SDK             int    `json:"sdk"`
	PlatformVersion string `json:"platformVersion"`
	DisplaySize     string `json:"displaySize"`
	DisplayDensity  int    `json:"displayDensity"`
}

// KeyboardInfo is returned by Device.getKeyboardInfo and pushed via Input.keyboardStateChanged.
type KeyboardInfo struct {
	Visible bool          `json:"visible"`
	Bounds  *BoundsResult `json:"bounds,omitempty"`
}

// SourceResult is returned by UI.getSource.
type SourceResult struct {
	XML string `json:"xml"`
}

// ScreenshotResult is returned by UI.screenshot.
type ScreenshotResult struct {
	Data string `json:"data"` // base64-encoded PNG
}

// OrientationResult is returned by Device.getOrientation.
type OrientationResult struct {
	Orientation string `json:"orientation"`
}

// ClipboardResult is returned by Device.getClipboard.
type ClipboardResult struct {
	Text string `json:"text"`
}
