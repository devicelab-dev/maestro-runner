// Package uiautomator2 provides HTTP client for UIAutomator2 server.
package uiautomator2

// Response is the standard UIAutomator2 response format.
type Response struct {
	SessionID string      `json:"sessionId"`
	Value     interface{} `json:"value"`
}

// ErrorValue represents an error from UIAutomator2.
type ErrorValue struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Capabilities for session creation.
type Capabilities struct {
	PlatformName string `json:"platformName,omitempty"`
	DeviceName   string `json:"deviceName,omitempty"`
}

// SessionRequest for creating a session.
type SessionRequest struct {
	Capabilities Capabilities `json:"capabilities"`
}

// ElementModel represents an element reference.
type ElementModel struct {
	ELEMENT string `json:"ELEMENT"`
}

// FindElementRequest for finding elements.
type FindElementRequest struct {
	Strategy string `json:"strategy"`
	Selector string `json:"selector"`
	Context  string `json:"context,omitempty"`
}

// InputTextRequest for typing text.
type InputTextRequest struct {
	Text string `json:"text"`
}

// KeyCodeRequest for pressing keys.
type KeyCodeRequest struct {
	KeyCode  int `json:"keycode"`
	MetaKeys int `json:"metastate,omitempty"`
}

// PointModel represents coordinates.
type PointModel struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// RectModel represents a rectangle.
type RectModel struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// ClickRequest for tap gestures.
type ClickRequest struct {
	Origin *ElementModel `json:"origin,omitempty"`
	Offset *PointModel   `json:"offset,omitempty"`
}

// LongClickRequest for long press gestures.
type LongClickRequest struct {
	Origin   *ElementModel `json:"origin,omitempty"`
	Offset   *PointModel   `json:"offset,omitempty"`
	Duration int           `json:"duration,omitempty"` // milliseconds
}

// SwipeRequest for swipe gestures.
type SwipeRequest struct {
	Origin    *ElementModel `json:"origin,omitempty"`
	Area      *RectModel    `json:"area,omitempty"`
	Direction string        `json:"direction"` // up, down, left, right
	Percent   float64       `json:"percent"`   // 0.0 - 1.0
	Speed     int           `json:"speed,omitempty"`
}

// ScrollRequest for scroll gestures.
type ScrollRequest struct {
	Origin    *ElementModel `json:"origin,omitempty"`
	Area      *RectModel    `json:"area,omitempty"`
	Direction string        `json:"direction"`
	Percent   float64       `json:"percent"`
	Speed     int           `json:"speed,omitempty"`
}

// DragRequest for drag gestures.
type DragRequest struct {
	Origin *ElementModel `json:"origin,omitempty"`
	EndX   int           `json:"endX"`
	EndY   int           `json:"endY"`
	Speed  int           `json:"speed,omitempty"`
}

// OrientationRequest for setting orientation.
type OrientationRequest struct {
	Orientation string `json:"orientation"` // PORTRAIT, LANDSCAPE
}

// ClipboardRequest for setting clipboard.
type ClipboardRequest struct {
	Content     string `json:"content"`     // base64 encoded
	ContentType string `json:"contentType"` // plaintext
}

// SettingsRequest for updating settings.
type SettingsRequest struct {
	Settings map[string]interface{} `json:"settings"`
}

// PinchRequest for pinch gestures.
type PinchRequest struct {
	Origin  *ElementModel `json:"origin,omitempty"`
	Percent float64       `json:"percent"`
	Speed   int           `json:"speed,omitempty"`
}

// DeviceInfo from device info endpoint.
type DeviceInfo struct {
	AndroidID       string `json:"androidId"`
	Manufacturer    string `json:"manufacturer"`
	Model           string `json:"model"`
	Brand           string `json:"brand"`
	APIVersion      string `json:"apiVersion"`
	PlatformVersion string `json:"platformVersion"`
	CarrierName     string `json:"carrierName"`
	RealDisplaySize string `json:"realDisplaySize"`
	DisplayDensity  int    `json:"displayDensity"`
}

// BatteryInfo from battery info endpoint.
type BatteryInfo struct {
	Level  float64 `json:"level"`
	Status int     `json:"status"`
}

// Common Android key codes.
const (
	KeyCodeBack       = 4
	KeyCodeHome       = 3
	KeyCodeMenu       = 82
	KeyCodeEnter      = 66
	KeyCodeDelete     = 67
	KeyCodeVolumeUp   = 24
	KeyCodeVolumeDown = 25
	KeyCodePower      = 26
	KeyCodeCamera     = 27
	KeyCodeSearch     = 84
	KeyCodeTab        = 61
	KeyCodeSpace      = 62
	KeyCodeDpadUp     = 19
	KeyCodeDpadDown   = 20
	KeyCodeDpadLeft   = 21
	KeyCodeDpadRight  = 22
	KeyCodeDpadCenter = 23
)

// Locator strategies.
const (
	StrategyID              = "id"
	StrategyAccessibilityID = "accessibilityId"
	StrategyXPath           = "xpath"
	StrategyClassName       = "className"
	StrategyText            = "text"
	StrategyUiAutomator     = "androidUiAutomator"
)

// Swipe/scroll directions.
const (
	DirectionUp    = "up"
	DirectionDown  = "down"
	DirectionLeft  = "left"
	DirectionRight = "right"
)

// Orientations.
const (
	OrientationPortrait  = "PORTRAIT"
	OrientationLandscape = "LANDSCAPE"
)
