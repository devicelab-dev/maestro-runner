# UIAutomator2 Server API Reference

This document describes the HTTP REST API exposed by `io.appium.uiautomator2.server`.

**Source:** [appium-uiautomator2-server](https://github.com/appium/appium-uiautomator2-server)

## Overview

The UIAutomator2 server runs on Android devices and provides HTTP endpoints for UI automation. It uses Google's UIAutomator V2 API under the hood.

**Default Port:** 6790 (on device)

**Architecture:**
```
┌─────────────┐      ADB Forward       ┌─────────────────────────┐
│   Client    │ ───────────────────────│  UIAutomator2 Server    │
│  (Go code)  │   localhost:socket     │  (on Android device)    │
│             │   or localhost:port    │  Port 6790              │
└─────────────┘                        └─────────────────────────┘
```

## Session Management

All operations (except `/status`) require a session. Create one first.

### Create Session

```
POST /session
```

**Request:**
```json
{
  "capabilities": {
    "platformName": "Android",
    "deviceName": "device"
  }
}
```

**Response:**
```json
{
  "sessionId": "abc123",
  "value": {
    "capabilities": { ... }
  }
}
```

### Get Session Info

```
GET /session/:sessionId
```

### Delete Session

```
DELETE /session/:sessionId
```

### Server Status

```
GET /status
```

**Response:**
```json
{
  "value": {
    "ready": true,
    "message": "UiAutomator2 Server is ready"
  }
}
```

---

## Element Finding

### Locator Strategies

| Strategy | Description | Example |
|----------|-------------|---------|
| `id` | Resource ID | `com.app:id/button` |
| `accessibilityId` | Content description | `Login Button` |
| `xpath` | XPath expression | `//android.widget.Button[@text='Login']` |
| `className` | Class name | `android.widget.Button` |
| `text` | Text content | `Login` |
| `androidUiAutomator` | UiSelector expression | `new UiSelector().text("Login")` |

### Find Single Element

```
POST /session/:sessionId/element
```

**Request:**
```json
{
  "strategy": "id",
  "selector": "com.app:id/login_button",
  "context": ""
}
```

- `strategy` - Locator strategy (see table above)
- `selector` - Value to match
- `context` - (Optional) Parent element ID for scoped search

**Response:**
```json
{
  "sessionId": "abc123",
  "value": {
    "ELEMENT": "element-id-123"
  }
}
```

### Find Multiple Elements

```
POST /session/:sessionId/elements
```

Same request format as above.

**Response:**
```json
{
  "sessionId": "abc123",
  "value": [
    { "ELEMENT": "element-id-1" },
    { "ELEMENT": "element-id-2" }
  ]
}
```

### Get Active (Focused) Element

```
GET /session/:sessionId/element/active
```

---

## Element Operations

All element operations use the element ID from find results.

### Click Element

```
POST /session/:sessionId/element/:elementId/click
```

No request body needed.

### Clear Text

```
POST /session/:sessionId/element/:elementId/clear
```

### Input Text

```
POST /session/:sessionId/element/:elementId/value
```

**Request:**
```json
{
  "text": "Hello World"
}
```

### Get Element Text

```
GET /session/:sessionId/element/:elementId/text
```

**Response:**
```json
{
  "sessionId": "abc123",
  "value": "Button Text"
}
```

### Get Element Attribute

```
GET /session/:sessionId/element/:elementId/attribute/:name
```

Common attributes: `text`, `class`, `package`, `content-desc`, `resource-id`, `checkable`, `checked`, `clickable`, `enabled`, `focusable`, `focused`, `scrollable`, `selected`, `displayed`

### Get Element Bounds

```
GET /session/:sessionId/element/:elementId/rect
```

**Response:**
```json
{
  "sessionId": "abc123",
  "value": {
    "x": 100,
    "y": 200,
    "width": 150,
    "height": 50
  }
}
```

### Get Element Screenshot

```
GET /session/:sessionId/element/:elementId/screenshot
```

**Response:** Base64-encoded PNG in `value` field.

---

## Gestures

### Tap at Coordinates

```
POST /session/:sessionId/appium/gestures/click
```

**Request:**
```json
{
  "origin": { "ELEMENT": "element-id" },
  "offset": { "x": 0, "y": 0 }
}
```

Or by coordinates (no element):
```json
{
  "locator": null,
  "offset": { "x": 500, "y": 800 }
}
```

### Long Press

```
POST /session/:sessionId/appium/gestures/long_click
```

**Request:**
```json
{
  "origin": { "ELEMENT": "element-id" },
  "duration": 1000
}
```

### Double Tap

```
POST /session/:sessionId/appium/gestures/double_click
```

Same format as click.

### Swipe

```
POST /session/:sessionId/appium/gestures/swipe
```

**Request:**
```json
{
  "origin": { "ELEMENT": "element-id" },
  "direction": "up",
  "percent": 0.5,
  "speed": 2500
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `origin` | ElementModel | No* | Element to swipe from |
| `area` | RectModel | No* | Rectangle area for swipe |
| `direction` | String | Yes | `up`, `down`, `left`, `right` |
| `percent` | Float | Yes | Swipe distance (0.0 - 1.0) |
| `speed` | Integer | No | Swipe speed in pixels/sec |

*Either `origin` or `area` must be provided.

### Scroll

```
POST /session/:sessionId/appium/gestures/scroll
```

**Request:**
```json
{
  "origin": { "ELEMENT": "scrollable-element-id" },
  "direction": "down",
  "percent": 0.5
}
```

### Drag

```
POST /session/:sessionId/appium/gestures/drag
```

**Request:**
```json
{
  "origin": { "ELEMENT": "element-id" },
  "endX": 500,
  "endY": 800,
  "speed": 2500
}
```

### Pinch Open/Close

```
POST /session/:sessionId/appium/gestures/pinch_open
POST /session/:sessionId/appium/gestures/pinch_close
```

**Request:**
```json
{
  "origin": { "ELEMENT": "element-id" },
  "percent": 0.5,
  "speed": 2500
}
```

---

## Device Operations

### Press Back

```
POST /session/:sessionId/back
```

### Press Key Code

```
POST /session/:sessionId/appium/device/press_keycode
```

**Request:**
```json
{
  "keycode": 4
}
```

Common key codes:
| Key | Code |
|-----|------|
| BACK | 4 |
| HOME | 3 |
| ENTER | 66 |
| DELETE/BACKSPACE | 67 |
| VOLUME_UP | 24 |
| VOLUME_DOWN | 25 |
| POWER | 26 |

### Long Press Key Code

```
POST /session/:sessionId/appium/device/long_press_keycode
```

Same format as press_keycode.

### Open Notifications

```
POST /session/:sessionId/appium/device/open_notifications
```

### Get/Set Clipboard

```
POST /session/:sessionId/appium/device/get_clipboard
```

**Response:**
```json
{
  "value": "clipboard text (base64 encoded)"
}
```

```
POST /session/:sessionId/appium/device/set_clipboard
```

**Request:**
```json
{
  "content": "text to copy (base64 encoded)",
  "contentType": "plaintext"
}
```

### Get Device Info

```
GET /session/:sessionId/appium/device/info
```

**Response:**
```json
{
  "value": {
    "androidId": "...",
    "manufacturer": "Google",
    "model": "Pixel 4a",
    "brand": "google",
    "apiVersion": "33",
    "platformVersion": "13",
    ...
  }
}
```

### Get Battery Info

```
GET /session/:sessionId/appium/device/battery_info
```

---

## Screen Operations

### Screenshot

```
GET /session/:sessionId/screenshot
```

**Response:**
```json
{
  "value": "<base64-encoded-png>"
}
```

### Get UI Hierarchy (Source)

```
GET /session/:sessionId/source
```

**Response:**
```json
{
  "value": "<?xml version='1.0'?>..."
}
```

Returns XML hierarchy of current screen.

### Get/Set Orientation

```
GET /session/:sessionId/orientation

POST /session/:sessionId/orientation
```

**Request (POST):**
```json
{
  "orientation": "LANDSCAPE"
}
```

Values: `PORTRAIT`, `LANDSCAPE`

---

## Alert Handling

### Get Alert Text

```
GET /session/:sessionId/alert/text
```

### Accept Alert

```
POST /session/:sessionId/alert/accept
```

### Dismiss Alert

```
POST /session/:sessionId/alert/dismiss
```

---

## Settings

### Get Settings

```
GET /session/:sessionId/appium/settings
```

### Update Settings

```
POST /session/:sessionId/appium/settings
```

**Request:**
```json
{
  "settings": {
    "waitForIdleTimeout": 10000,
    "waitForSelectorTimeout": 10000,
    "actionAcknowledgmentTimeout": 3000
  }
}
```

---

## Error Responses

All errors follow this format:

```json
{
  "sessionId": "abc123",
  "value": {
    "error": "no such element",
    "message": "An element could not be located on the page using the given search parameters"
  }
}
```

| Error | HTTP Status | Description |
|-------|-------------|-------------|
| `no such element` | 404 | Element not found |
| `stale element reference` | 404 | Element no longer in DOM |
| `invalid argument` | 400 | Bad request parameters |
| `unknown error` | 500 | Server error |

---

## References

- [appium-uiautomator2-server (GitHub)](https://github.com/appium/appium-uiautomator2-server)
- [appium-uiautomator2-driver (GitHub)](https://github.com/appium/appium-uiautomator2-driver)
- [Android KeyEvent Codes](https://developer.android.com/reference/android/view/KeyEvent)
