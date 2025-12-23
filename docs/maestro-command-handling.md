# Maestro Command Handling Deep Dive

This document analyzes how Maestro handles commands internally, based on reading the actual Maestro source code.

## Architecture Overview

```
┌───────────────────────────────────────────────────────────────┐
│                    Orchestra (orchestrator)                    │
│  - Translates YAML commands to method calls                    │
│  - Handles retries, conditionals, scripts                      │
│  - Manages command lifecycle (start/complete/fail)             │
│  Location: maestro-orchestra/Orchestra.kt                      │
└───────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌───────────────────────────────────────────────────────────────┐
│                    Maestro Client                              │
│  - Device-agnostic operations                                  │
│  - Tap with retry logic, element finding with timeout          │
│  - waitForAppToSettle, waitForAnimationToEnd                   │
│  Location: maestro-client/Maestro.kt                           │
└───────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌─────────────────────────┐     ┌─────────────────────────┐
│     AndroidDriver       │     │       IOSDriver         │
│  - gRPC (port 7001)     │     │  - HTTP REST API        │
│  - APK on device        │     │  - XCTest runner        │
│  - ADB shell commands   │     │  - Device bridge        │
└─────────────────────────┘     └─────────────────────────┘
```

## Key Source Files

| File | Purpose |
|------|---------|
| `maestro-client/Driver.kt` | Driver interface (30+ methods) |
| `maestro-client/Maestro.kt` | Client with retry/wait logic |
| `maestro-orchestra/Orchestra.kt` | Command execution (40+ types) |
| `maestro-client/drivers/AndroidDriver.kt` | Android: gRPC + ADB |
| `maestro-client/drivers/IOSDriver.kt` | iOS: HTTP via XCTest |
| `maestro-client/Filters.kt` | Element matching filters |
| `maestro-client/utils/ScreenshotUtils.kt` | Wait/settle utilities |

---

## Element Finding

### Filter Types (Filters.kt)

```kotlin
// Basic filters
textMatches(regex)     // matches text, hintText, accessibilityText
idMatches(regex)       // matches resource-id
sizeMatches(w, h)      // matches element dimensions
enabled(bool)          // filter by enabled state
selected(bool)         // filter by selected state
checked(bool)          // filter by checked state
focused(bool)          // filter by focused state

// Relative filters
below(filter)          // elements below matching element
above(filter)          // elements above matching element
leftOf(filter)         // elements to the left
rightOf(filter)        // elements to the right
containsChild(element) // parent contains specific child
containsDescendants(filters) // has matching descendants

// Special filters
deepestMatchingElement(filter)  // finds deepest node (avoids containers)
clickableFirst()                // prioritize clickable elements
index(idx)                      // select by index (supports negative)
```

### Element Finding with Timeout (Maestro.kt)

```kotlin
fun findElementWithTimeout(timeoutMs: Long, filter: ElementFilter): FindElementResult? {
    return MaestroTimer.withTimeout(timeoutMs) {
        val hierarchy = viewHierarchy()
        val nodes = hierarchy.aggregate()  // flatten tree to list
        filter(nodes).firstOrNull()?.toUiElement()
    }
}
```

**Key insight**: Maestro fetches the FULL hierarchy and filters locally, rather than querying for specific elements.

---

## Wait Strategies

### 1. Wait for App to Settle (ScreenshotUtils.kt:38-74)

Compares consecutive view hierarchies until stable:

```kotlin
fun waitForAppToSettle(initialHierarchy: ViewHierarchy?, driver: Driver): ViewHierarchy {
    var latestHierarchy = initialHierarchy ?: viewHierarchy(driver)

    repeat(10) {  // Max 10 iterations
        val hierarchyAfter = viewHierarchy(driver)

        if (latestHierarchy == hierarchyAfter) {
            // Also check is-loading attribute
            val isLoading = latestHierarchy.root.attributes
                .getOrDefault("is-loading", "false").toBoolean()
            if (!isLoading) {
                return hierarchyAfter  // UI has settled
            }
        }

        latestHierarchy = hierarchyAfter
        sleep(200)  // 200ms between checks
    }

    return latestHierarchy  // Return last hierarchy even if not settled
}
```

### 2. Wait Until Screen Is Static (ScreenshotUtils.kt:76-96)

Compares screenshots using image diff:

```kotlin
fun waitUntilScreenIsStatic(timeoutMs: Long, threshold: Double, driver: Driver): Boolean {
    return MaestroTimer.retryUntilTrue(timeoutMs) {
        val startScreenshot = takeScreenshot(driver)
        val endScreenshot = takeScreenshot(driver)

        val imageDiff = ImageComparison(startScreenshot, endScreenshot)
            .compareImages()
            .differencePercent

        return imageDiff <= threshold  // Default: 0.005 (0.5%)
    }
}
```

### 3. iOS-specific: isScreenStatic (IOSDriver.kt)

iOS has native animation detection:

```kotlin
override fun waitForAppToSettle(...): ViewHierarchy? {
    // First try native iOS animation detection
    val didFinishOnTime = waitUntilScreenIsStatic(SCREEN_SETTLE_TIMEOUT_MS)

    // Fallback to hierarchy comparison if needed
    if (didFinishOnTime) null
    else ScreenshotUtils.waitForAppToSettle(initialHierarchy, this, timeoutMs)
}
```

---

## Tap Implementation

### Two Modes (Maestro.kt:270-384)

```kotlin
fun tap(x: Int, y: Int, retryIfNoChange: Boolean, longPress: Boolean, ...) {
    if (Capability.FAST_HIERARCHY in cachedDeviceInfo.capabilities) {
        hierarchyBasedTap(x, y, retryIfNoChange, longPress, ...)  // Android
    } else {
        screenshotBasedTap(x, y, retryIfNoChange, longPress, ...)  // iOS
    }
}
```

### Hierarchy-Based Tap (Android)

```kotlin
private fun hierarchyBasedTap(x: Int, y: Int, retryIfNoChange: Boolean, ...) {
    val hierarchyBefore = viewHierarchy()

    performTap(x, y, longPress)

    val hierarchyAfter = waitForAppToSettle(hierarchyBefore, appId, waitToSettleTimeoutMs)

    if (retryIfNoChange && hierarchyBefore == hierarchyAfter) {
        // Hierarchy unchanged - check screenshots as fallback
        val screenshotBefore = ScreenshotUtils.tryTakingScreenshot(driver)

        performTap(x, y, longPress)  // Retry tap

        val screenshotAfter = ScreenshotUtils.tryTakingScreenshot(driver)

        if (screenshotsAreDifferent(screenshotBefore, screenshotAfter)) {
            waitForAppToSettle(hierarchyAfter, appId, waitToSettleTimeoutMs)
        }
    }
}
```

### Screenshot-Based Tap (iOS)

```kotlin
private fun screenshotBasedTap(x: Int, y: Int, retryIfNoChange: Boolean, ...) {
    val screenshotBefore = ScreenshotUtils.tryTakingScreenshot(driver)

    performTap(x, y, longPress)
    waitForAppToSettle(null, appId, waitToSettleTimeoutMs)

    if (retryIfNoChange) {
        val screenshotAfter = ScreenshotUtils.tryTakingScreenshot(driver)

        if (!screenshotsAreDifferent(screenshotBefore, screenshotAfter)) {
            performTap(x, y, longPress)  // Retry
            waitForAppToSettle(null, appId, waitToSettleTimeoutMs)
        }
    }
}
```

---

## Android Driver (AndroidDriver.kt)

### Communication

- **Protocol**: gRPC over TCP port 7001
- **Server**: APK installed on device (`dev.mobile.maestro`, `dev.mobile.maestro.test`)
- **Startup**: `am instrument -w ... dev.mobile.maestro.test/androidx.test.runner.AndroidJUnitRunner`

### Operations

| Operation | Implementation |
|-----------|---------------|
| `tap(x,y)` | gRPC: `blockingStub.tap(tapRequest{x,y})` |
| `longPress(x,y)` | ADB: `input swipe x y x y 3000` |
| `inputText(text)` | gRPC: `blockingStub.inputText(inputTextRequest{text})` |
| `screenshot` | gRPC: `blockingStub.screenshot()` → bytes |
| `viewHierarchy` | gRPC: `blockingStub.viewHierarchy()` → XML → TreeNode |
| `swipe` | ADB: `input swipe startX startY endX endY durationMs` |
| `pressKey(code)` | ADB: `input keyevent <keycode>` |
| `launchApp` | gRPC: `blockingStub.launchApp(launchAppRequest{appId})` |
| `stopApp` | ADB: `am force-stop <appId>` |
| `clearAppState` | ADB: `pm clear <appId>` |
| `eraseText(n)` | gRPC: `blockingStub.eraseAllText(eraseAllTextRequest{n})` |

### View Hierarchy Format (Android)

XML with bounds as `[x1,y1][x2,y2]`:

```xml
<node index="0" text="Hello" resource-id="com.app:id/text"
      class="android.widget.TextView" bounds="[0,100][200,150]">
  <node .../>
</node>
```

---

## iOS Driver (IOSDriver.kt)

### Communication

- **Protocol**: HTTP REST via `iosDevice` bridge object
- **Server**: XCTest runner on device/simulator

### Operations

| Operation | Implementation |
|-----------|---------------|
| `tap(x,y)` | `iosDevice.tap(x, y)` |
| `longPress(x,y)` | `iosDevice.longPress(x, y, 3000)` |
| `inputText(text)` | `iosDevice.input(text)` |
| `screenshot` | `iosDevice.takeScreenshot(sink, compressed)` |
| `viewHierarchy` | `iosDevice.viewHierarchy()` → AXElement → TreeNode |
| `swipe` | `iosDevice.scroll(xStart, yStart, xEnd, yEnd, duration)` |
| `pressKey(code)` | `iosDevice.pressKey(name)` or `pressButton(name)` |
| `launchApp` | `iosDevice.launch(appId, args)` |
| `stopApp` | `iosDevice.stop(appId)` |
| `clearAppState` | `iosDevice.clearAppState(appId)` |
| `eraseText(n)` | `iosDevice.eraseText(n)` |

### View Hierarchy Format (iOS)

AXElement tree with separate bounds attributes:

```kotlin
data class AXElement(
    val label: String,           // accessibilityText
    val title: String?,          // text
    val value: String?,          // value
    val identifier: String,      // resource-id
    val frame: Frame,            // x, y, width, height
    val enabled: Boolean,
    val hasFocus: Boolean,
    val selected: Boolean,
    val children: List<AXElement>
)
```

---

## Command Execution (Orchestra.kt)

### Command Types

```kotlin
when (command) {
    // Tap commands
    is TapOnElementCommand -> tapOnElement(command, ...)
    is TapOnPointCommand -> tapOnPoint(command, ...)
    is TapOnPointV2Command -> tapOnPointV2Command(command)

    // Navigation
    is BackPressCommand -> backPressCommand()
    is HideKeyboardCommand -> hideKeyboardCommand()
    is ScrollCommand -> scrollVerticalCommand()
    is ScrollUntilVisibleCommand -> scrollUntilVisible(command)
    is SwipeCommand -> swipeCommand(command)

    // Input
    is InputTextCommand -> inputTextCommand(command)
    is InputRandomCommand -> inputTextRandomCommand(command)
    is EraseTextCommand -> eraseTextCommand(command)
    is PressKeyCommand -> pressKeyCommand(command)
    is CopyTextFromCommand -> copyTextFromCommand(command)
    is PasteTextCommand -> pasteText()

    // Assertions
    is AssertCommand -> assertCommand(command)
    is AssertConditionCommand -> assertConditionCommand(command)
    is AssertNoDefectsWithAICommand -> assertNoDefectsWithAICommand(...)
    is AssertWithAICommand -> assertWithAICommand(...)

    // App lifecycle
    is LaunchAppCommand -> launchAppCommand(command)
    is StopAppCommand -> stopAppCommand(command)
    is KillAppCommand -> killAppCommand(command)
    is ClearStateCommand -> clearAppStateCommand(command)
    is ClearKeychainCommand -> clearKeychainCommand()

    // Flow control
    is RunFlowCommand -> runFlowCommand(command, config)
    is RepeatCommand -> repeatCommand(command, ...)
    is RetryCommand -> retryCommand(command, config)

    // Device state
    is SetLocationCommand -> setLocationCommand(command)
    is SetOrientationCommand -> setOrientationCommand(command)
    is SetAirplaneModeCommand -> setAirplaneMode(command)
    is ToggleAirplaneModeCommand -> toggleAirplaneMode()

    // Media
    is TakeScreenshotCommand -> takeScreenshotCommand(command)
    is StartRecordingCommand -> startRecordingCommand(command)
    is StopRecordingCommand -> stopRecordingCommand()
    is AddMediaCommand -> addMediaCommand(command.mediaPaths)

    // Scripts
    is DefineVariablesCommand -> defineVariablesCommand(command)
    is RunScriptCommand -> runScriptCommand(command)
    is EvalScriptCommand -> evalScriptCommand(command)

    // Special
    is WaitForAnimationToEndCommand -> waitForAnimationToEndCommand(command)
    is OpenLinkCommand -> openLinkCommand(command, config)
    is TravelCommand -> travelCommand(command)
    is ExtractTextWithAICommand -> extractTextWithAICommand(...)
}
```

### Example: TapOnElement

```kotlin
private fun tapOnElement(command: TapOnElementCommand, ...): Boolean {
    // 1. Find element with timeout
    val result = findElement(command.selector, optional = command.optional)

    // 2. Handle relative point if specified
    if (command.relativePoint != null) {
        val tapPoint = calculateElementRelativePoint(result.element, relativePoint)
        maestro.tap(x = tapPoint.x, y = tapPoint.y, ...)
    } else {
        // 3. Default: tap at element center
        maestro.tap(
            element = result.element,
            initialHierarchy = result.hierarchy,
            retryIfNoChange = command.retryIfNoChange ?: false,
            waitUntilVisible = command.waitUntilVisible ?: false,
            longPress = command.longPress ?: false,
            tapRepeat = command.repeat,
            waitToSettleTimeoutMs = command.waitToSettleTimeoutMs,
        )
    }
    return true
}
```

### Example: LaunchApp

```kotlin
private fun launchAppCommand(command: LaunchAppCommand): Boolean {
    // 1. Clear keychain if requested
    if (command.clearKeychain == true) {
        maestro.clearKeychain()
    }

    // 2. Clear app state if requested
    if (command.clearState == true) {
        maestro.clearAppState(command.appId)
    }

    // 3. Set permissions (default: allow all)
    val permissions = command.permissions ?: mapOf("all" to "allow")
    maestro.setPermissions(command.appId, permissions)

    // 4. Launch the app
    maestro.launchApp(
        appId = command.appId,
        launchArguments = command.launchArguments ?: emptyMap(),
        stopIfRunning = command.stopApp ?: true
    )

    return true
}
```

---

## Constants & Thresholds

```kotlin
// Orchestra.kt
lookupTimeoutMs = 17000L              // Element find timeout
optionalLookupTimeoutMs = 7000L       // Optional element timeout

// Maestro.kt
SCREENSHOT_DIFF_THRESHOLD = 0.005     // 0.5% for screen comparison
ANIMATION_TIMEOUT_MS = 15000L         // Animation wait timeout

// AndroidDriver.kt
SERVER_LAUNCH_TIMEOUT_MS = 15000L     // Driver startup timeout
WINDOW_UPDATE_TIMEOUT_MS = 750        // Window settle check

// IOSDriver.kt
SCREEN_SETTLE_TIMEOUT_MS = 3000L      // iOS animation settle
WARNING_MAX_DEPTH = 61                // Hierarchy depth warning

// ScreenshotUtils.kt
MAX_ITERATIONS = 10                   // Wait for settle iterations
SLEEP_BETWEEN_ITERATIONS = 200ms      // Wait between checks
```

---

## Maestro vs Appium Comparison

| Aspect | Maestro | Appium |
|--------|---------|--------|
| **Protocol** | gRPC (Android), HTTP (iOS) | W3C WebDriver |
| **Element Finding** | Fetch full hierarchy → filter locally | Server-side locator strategies |
| **Wait Strategy** | Compare hierarchies/screenshots | Implicit/Explicit waits |
| **Animation Wait** | Screenshot diff (0.5% threshold) | No built-in (manual polling) |
| **Tap** | Direct coordinates to driver | W3C Actions API |
| **Hierarchy** | Full tree fetched, filtered locally | Query returns specific elements |
| **Timeout Default** | 17s / 7s (optional) | 0s (configurable) |
| **Retry on No Change** | Built-in with screenshot fallback | Manual implementation |

---

## Implications for maestro-runner

1. **Element Finding**: Implement local filtering on full hierarchy (like Maestro)
2. **Wait Strategy**: Use hierarchy comparison as primary, screenshot as fallback
3. **Tap Logic**: Include retry-if-no-change with screenshot verification
4. **Timeouts**: Use 17s default, 7s for optional
5. **Animation**: Compare consecutive screenshots with 0.5% threshold
6. **Driver Interface**: Keep it simple - tap, swipe, inputText, screenshot, hierarchy
