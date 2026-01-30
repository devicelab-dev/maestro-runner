# iOS Driver Improvement Plan (Complete)

## Current State
- Using WebDriverAgent (WDA) via HTTP API
- 3/4 auth tests passing
- Issues with text field focus and input reliability

---

## Maestro's Complete iOS Architecture

```
┌─────────────────────┐
│  Kotlin/Java Client │  (IOSDriver.kt)
└─────────┬───────────┘
          │ HTTP/JSON
┌─────────▼───────────┐
│  Swift HTTP Server  │  (XCTestHTTPServer.swift, Port 22087)
└─────────┬───────────┘
          │
┌─────────▼───────────┐
│   Route Handlers    │  (TouchRouteHandler, InputTextHandler, etc.)
└─────────┬───────────┘
          │
┌─────────▼───────────┐
│  Event Synthesis    │  (EventRecord, PointerEventPath)
└─────────┬───────────┘
          │
┌─────────▼───────────┐
│  Private XCTest API │  (XCPointerEventPath, XCSynthesizedEventRecord)
└─────────┬───────────┘
          │
┌─────────▼───────────┐
│  RunnerDaemonProxy  │  (_XCT_synthesizeEvent, _XCT_sendString)
└─────────────────────┘
```

---

## 1. clearState Implementation (CRITICAL)

### Maestro's Approach (Simulator)
```kotlin
// LocalSimulatorUtils.kt - Lines 276-285
fun clearAppState(deviceId: String, bundleId: String) {
    // 1. Terminate app
    terminateApp(deviceId, bundleId)

    // 2. Wait for termination
    Thread.sleep(500)

    // 3. Uninstall app completely
    exec("xcrun", "simctl", "uninstall", deviceId, bundleId)

    // 4. Reinstall fresh copy
    exec("xcrun", "simctl", "install", deviceId, appPath)
}
```

### clearKeychain (Simulator)
```bash
xcrun simctl keychain <deviceId> reset
```

### Our Implementation Plan
```go
// pkg/driver/wda/commands.go
func (d *Driver) clearState(step *flow.ClearStateStep) *core.CommandResult {
    bundleID := step.AppID

    // 1. Terminate app via WDA
    d.client.TerminateApp(bundleID)
    time.Sleep(500 * time.Millisecond)

    // 2. Uninstall via simctl
    exec.Command("xcrun", "simctl", "uninstall", d.deviceID, bundleID).Run()

    // 3. Reinstall (need app path from config)
    exec.Command("xcrun", "simctl", "install", d.deviceID, appPath).Run()

    // 4. Relaunch
    d.client.CreateSession(bundleID)

    return successResult("App state cleared", nil)
}
```

---

## 2. Text Input (CRITICAL FIX)

### Maestro's Workaround (TextInputHelper.swift)
```swift
static func inputText(_ text: String) async throws {
    // STEP 1: Type first character SLOW (speed 1)
    let firstChar = String(text.prefix(1))
    var eventPath = PointerEventPath.pathForTextInput()
    eventPath.type(text: firstChar, typingSpeed: 1)
    try await RunnerDaemonProxy().synthesize(eventRecord)

    // STEP 2: Wait 500ms (CRITICAL!)
    try await Task.sleep(nanoseconds: 500_000_000)

    // STEP 3: Type remaining characters FAST (speed 30)
    if text.count > 1 {
        let remaining = String(text.suffix(text.count - 1))
        eventPath2.type(text: remaining, typingSpeed: 30)
        try await RunnerDaemonProxy().synthesize(eventRecord2)
    }
}
```

**Why?** iOS keyboard listener events (autocorrection, hardware keyboard detection) often skip characters after the first when typing too fast.

### Our Implementation Plan
```go
// pkg/driver/wda/commands.go
func (d *Driver) inputText(step *flow.InputTextStep) *core.CommandResult {
    text := step.Text

    // Wait for keyboard to appear (poll for 1s)
    d.waitForKeyboard(1000)

    // Type first character
    if len(text) > 0 {
        d.client.SendKeys(text[:1])
    }

    // Wait 500ms (critical for iOS)
    time.Sleep(500 * time.Millisecond)

    // Type remaining characters
    if len(text) > 1 {
        d.client.SendKeys(text[1:])
    }

    return successResult(fmt.Sprintf("Entered text: %s", text), nil)
}

func (d *Driver) waitForKeyboard(timeoutMs int) bool {
    deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
    for time.Now().Before(deadline) {
        source, _ := d.client.Source()
        if strings.Contains(source, "XCUIElementTypeKeyboard") {
            return true
        }
        time.Sleep(200 * time.Millisecond)
    }
    return false
}
```

---

## 3. Wait for Animation/Screen Static

### Maestro's Approach (ScreenDiffHandler.swift)
```swift
func isScreenStatic() -> Bool {
    let screenshot1 = XCUIScreen.main.screenshot().pngRepresentation
    let hash1 = SHA256.hash(data: screenshot1)

    let screenshot2 = XCUIScreen.main.screenshot().pngRepresentation
    let hash2 = SHA256.hash(data: screenshot2)

    return hash1 == hash2
}
```

### Our Implementation Plan
```go
// pkg/driver/wda/commands.go
func (d *Driver) waitForAnimationToEnd(step *flow.WaitForAnimationToEndStep) *core.CommandResult {
    timeout := step.Timeout
    if timeout == 0 {
        timeout = 3000
    }

    deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
    var lastHash string

    for time.Now().Before(deadline) {
        screenshot, _ := d.client.Screenshot()
        hash := sha256.Sum256(screenshot)
        currentHash := hex.EncodeToString(hash[:])

        if currentHash == lastHash {
            return successResult("Screen is static", nil)
        }
        lastHash = currentHash
        time.Sleep(100 * time.Millisecond)
    }

    return successResult("Animation timeout", nil)
}
```

---

## 4. Touch/Tap Events

### Maestro's Implementation (TouchRouteHandler.swift)
```swift
func handleTouch(x: Float, y: Float, duration: TimeInterval) {
    // 1. Transform coordinates for orientation
    let point = ScreenSizeHelper.orientationAwarePoint(x, y)

    // 2. Create event record
    let eventRecord = EventRecord(orientation: .portrait)
    eventRecord.addPointerTouchEvent(at: point, touchUpAfter: duration)

    // 3. Synthesize via daemon proxy
    RunnerDaemonProxy().synthesize(eventRecord)
}
```

### Our WDA Approach (Current - OK)
```go
func (c *Client) Tap(x, y float64) error {
    _, err := c.post(c.sessionPath("/wda/tap"), map[string]interface{}{
        "x": x,
        "y": y,
    })
    return err
}
```

**Note:** WDA handles this internally, should be fine.

---

## 5. Swipe/Scroll

### Maestro's Implementation
```swift
func handleSwipe(start: CGPoint, end: CGPoint, duration: TimeInterval) {
    let eventRecord = EventRecord(orientation: .portrait)
    eventRecord.addSwipeEvent(start: start, end: end, duration: duration)
    RunnerDaemonProxy().synthesize(eventRecord)
}

// EventRecord.swift
func addSwipeEvent(start: CGPoint, end: CGPoint, duration: TimeInterval) {
    let path = PointerEventPath.pathForTouch(at: start)
    path.offset += 0.1  // Initial touch duration
    path.moveTo(point: end)
    path.offset += duration
    path.liftUp()
    self.add(path)
}
```

### Our WDA Approach (Current - OK)
```go
func (c *Client) Swipe(fromX, fromY, toX, toY float64, durationSec float64) error {
    _, err := c.post(c.sessionPath("/wda/dragfromtoforduration"), ...)
    return err
}
```

---

## 6. Press Hardware Buttons

### Maestro's Implementation (PressButtonHandler.swift)
```swift
func pressButton(_ button: String) {
    switch button {
    case "home":
        XCUIDevice.shared.press(.home)
    case "lock":
        // Private API!
        XCUIDevice.shared.perform(NSSelectorFromString("pressLockButton"))
    }
}
```

### Our WDA Implementation
```go
func (c *Client) PressButton(button string) error {
    _, err := c.post(c.sessionPath("/wda/pressButton"), map[string]interface{}{
        "name": button,  // "home", "volumeUp", "volumeDown"
    })
    return err
}
```

**Note:** WDA supports home, volumeUp, volumeDown. Lock requires private API.

---

## 7. Press Keyboard Keys

### Maestro's Implementation (PressKeyHandler.swift)
```swift
let keyMap = [
    "delete": XCUIKeyboardKey.delete.rawValue,
    "return": XCUIKeyboardKey.return.rawValue,
    "enter": XCUIKeyboardKey.enter.rawValue,
    "tab": XCUIKeyboardKey.tab.rawValue,
    "space": XCUIKeyboardKey.space.rawValue,
    "escape": XCUIKeyboardKey.escape.rawValue,
]

func pressKey(_ key: String) {
    let path = PointerEventPath.pathForTextInput()
    path.type(text: keyMap[key], typingSpeed: 30)
    RunnerDaemonProxy().synthesize(eventRecord)
}
```

### Our Implementation Plan
```go
// Map key names to actual key characters
var iosKeyMap = map[string]string{
    "delete":    "\u007F",  // DEL
    "return":    "\n",
    "enter":     "\n",
    "tab":       "\t",
    "space":     " ",
    "escape":    "\u001B",  // ESC
    "backspace": "\u0008",  // BS
}

func (d *Driver) pressKey(step *flow.PressKeyStep) *core.CommandResult {
    key := strings.ToLower(step.Key)
    if char, ok := iosKeyMap[key]; ok {
        d.client.SendKeys(char)
        return successResult(fmt.Sprintf("Pressed %s", key), nil)
    }
    // Fallback: send as-is (for letter keys)
    d.client.SendKeys(key)
    return successResult(fmt.Sprintf("Pressed %s", key), nil)
}
```

---

## 8. Error Handling

### Maestro's Error Classification (XCTestDriverClient.kt)
```kotlin
val appCrashPatterns = listOf(
    "Lost connection to the application.*",
    "Application [a-zA-Z0-9.]+ is not running",
    "Error getting main window kAXErrorCannotComplete",
    "Error getting main window.*"
)

when {
    response.code == 408 -> throw OperationTimeout
    response.code in 400..499 -> throw BadRequest
    appCrashPatterns.any { body.matches(it) } -> throw AppCrash
}
```

### Our Implementation Plan
```go
// pkg/driver/wda/client.go
func (c *Client) parseResponse(resp *http.Response) (map[string]interface{}, error) {
    // ... existing code ...

    // Check for app crash patterns
    if c.isAppCrash(body) {
        return nil, fmt.Errorf("app crashed: %s", body)
    }

    // Check for timeout
    if resp.StatusCode == 408 {
        return nil, fmt.Errorf("operation timeout")
    }
}

func (c *Client) isAppCrash(body string) bool {
    patterns := []string{
        "Lost connection to the application",
        "is not running",
        "kAXErrorCannotComplete",
    }
    for _, p := range patterns {
        if strings.Contains(body, p) {
            return true
        }
    }
    return false
}
```

---

## 9. Screenshot

### Maestro (Direct XCTest)
```swift
let screenshot = XCUIScreen.main.screenshot()
let png = screenshot.pngRepresentation
// or compressed:
let jpeg = screenshot.image.jpegData(compressionQuality: 0.5)
```

### Our WDA Approach (Current - OK)
```go
func (c *Client) Screenshot() ([]byte, error) {
    resp, _ := c.get(c.sessionPath("/screenshot"))
    return base64Decode(resp["value"].(string))
}
```

---

## 10. Orientation Handling

### Maestro's Coordinate Transform (ScreenSizeHelper.swift)
```swift
func orientationAwarePoint(width: Float, height: Float, point: CGPoint) -> CGPoint {
    switch orientation {
    case .portrait:
        return point
    case .portraitUpsideDown:
        return CGPoint(x: width - point.x, y: height - point.y)
    case .landscapeLeft:
        return CGPoint(x: width - point.y, y: point.x)
    case .landscapeRight:
        return CGPoint(x: point.y, y: height - point.x)
    }
}
```

### Our Implementation Plan
```go
func (d *Driver) orientationAwarePoint(x, y float64) (float64, float64) {
    orientation, _ := d.client.GetOrientation()
    width, height, _ := d.client.WindowSize()

    switch orientation {
    case "PORTRAIT":
        return x, y
    case "UIA_DEVICE_ORIENTATION_PORTRAIT_UPSIDEDOWN":
        return float64(width) - x, float64(height) - y
    case "LANDSCAPE":
        return float64(width) - y, x
    case "UIA_DEVICE_ORIENTATION_LANDSCAPERIGHT":
        return y, float64(height) - x
    default:
        return x, y
    }
}
```

---

## Implementation Priority

### Phase 1: Critical Fixes (Do Now)
| Task | Impact | Effort |
|------|--------|--------|
| Split text input (first char slow + 500ms delay) | HIGH | LOW |
| Wait for keyboard before typing | HIGH | LOW |
| Add placeholderValue to element search | MEDIUM | LOW |

### Phase 2: clearState (Next)
| Task | Impact | Effort |
|------|--------|--------|
| Implement clearState via simctl uninstall/install | HIGH | MEDIUM |
| Store app path in driver config | MEDIUM | LOW |

### Phase 3: Robustness
| Task | Impact | Effort |
|------|--------|--------|
| Screen static detection for waitForAnimation | MEDIUM | MEDIUM |
| Better error classification | MEDIUM | LOW |
| Orientation-aware coordinates | LOW | MEDIUM |

### Phase 4: Polish
| Task | Impact | Effort |
|------|--------|--------|
| Fix DRY in findElementQuick | LOW | LOW |
| Add keyboard key mapping | LOW | LOW |
| Unit tests | LOW | HIGH |

---

## Quick Reference: WDA vs Maestro

| Feature | WDA | Maestro XCTest |
|---------|-----|----------------|
| Text Input | SendKeys (fast) | Split typing with delay |
| Element Find | Predicate/ClassChain | Full AX tree + attributes |
| Touch | /wda/tap | XCPointerEventPath |
| clearState | Not native | simctl uninstall/install |
| Keyboard Wait | Not built-in | Poll for 1s with 200ms interval |
| Screen Static | Not native | SHA256 hash comparison |
| Error Detection | HTTP status | Pattern matching on body |

---

## Files to Modify

1. `pkg/driver/wda/commands.go`
   - inputText: Add split typing + delay
   - clearState: Implement via simctl
   - waitForAnimationToEnd: Add hash comparison
   - pressKey: Add key mapping

2. `pkg/driver/wda/driver.go`
   - Add waitForKeyboard helper
   - Fix DRY in findElementQuick
   - Add orientation transform

3. `pkg/driver/wda/client.go`
   - Add error classification
   - Add isAppCrash detection

4. `pkg/driver/wda/pagesource.go`
   - Parse placeholderValue attribute
   - Parse hasFocus attribute
