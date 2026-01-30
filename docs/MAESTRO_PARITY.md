# Maestro Feature Parity Status

This document tracks maestro-runner's implementation status against Maestro's commands.

**Last Updated:** 2026-01-22 (Deep dive of Maestro source code)
**Source Files Analyzed:**
- `maestro-orchestra-models/src/main/java/maestro/orchestra/Commands.kt`
- `maestro-orchestra/src/main/java/maestro/orchestra/yaml/YamlFluentCommand.kt`
- `maestro-orchestra-models/src/main/java/maestro/orchestra/ElementSelector.kt`
- All YAML command definition files

---

## Implementation Status Legend

| Status | Meaning |
|--------|---------|
| ✅ | Fully implemented |
| ⚠️ | Partially implemented (missing parameters) |
| ❌ | Not implemented |
| 🚫 | Not planned (AI features, etc.) |

---

## 1. Tap & Gesture Commands

### TapOnElementCommand (from Commands.kt)

```kotlin
data class TapOnElementCommand(
    val selector: ElementSelector,
    val retryIfNoChange: Boolean? = null,      // Retry tap if UI doesn't change
    val waitUntilVisible: Boolean? = null,     // Wait for element before tap
    val longPress: Boolean? = null,            // Long press instead of tap
    val repeat: TapRepeat? = null,             // Repeat count + delay
    val waitToSettleTimeoutMs: Int? = null,    // Wait for UI to settle (max 30000ms)
    val relativePoint: String? = null,         // Element-relative tap point "50%,50%"
    val label: String? = null,
    val optional: Boolean = false,
)

companion object {
    const val DEFAULT_REPEAT_DELAY = 100L
    const val MAX_TIMEOUT_WAIT_TO_SETTLE_MS = 30000
}
```

### TapOnPointV2Command (from Commands.kt)

```kotlin
data class TapOnPointV2Command(
    val point: String,                         // "x,y" or "x%,y%"
    val retryIfNoChange: Boolean? = null,
    val longPress: Boolean? = null,
    val repeat: TapRepeat? = null,
    val waitToSettleTimeoutMs: Int? = null,
    val label: String? = null,
    val optional: Boolean = false,
)
```

### SwipeCommand (from Commands.kt)

```kotlin
data class SwipeCommand(
    val direction: SwipeDirection? = null,     // UP, DOWN, LEFT, RIGHT
    val startPoint: Point? = null,             // Absolute start
    val endPoint: Point? = null,               // Absolute end
    val elementSelector: ElementSelector? = null, // Swipe ON element
    val startRelative: String? = null,         // Relative "50%,90%"
    val endRelative: String? = null,           // Relative "50%,10%"
    val duration: Long = 400L,                 // Swipe duration ms
    val waitToSettleTimeoutMs: Int? = null,
    val label: String? = null,
    val optional: Boolean = false,
)
```

### ScrollUntilVisibleCommand (from Commands.kt)

```kotlin
data class ScrollUntilVisibleCommand(
    val selector: ElementSelector,
    val direction: ScrollDirection,
    val scrollDuration: String = "40",         // Speed (0-100), not ms!
    val visibilityPercentage: Int = 100,       // 0-100% visibility required
    val timeout: String = "20000",             // Timeout in ms
    val waitToSettleTimeoutMs: Int? = null,
    val centerElement: Boolean = false,        // Center element after found
    val label: String? = null,
    val optional: Boolean = false,
)
```

| Command | Status | Maestro Parameters | Our Status |
|---------|--------|-------------------|------------|
| `tapOn` | ⚠️ | selector, retryIfNoChange, waitUntilVisible, longPress, repeat, waitToSettleTimeoutMs, relativePoint, label, optional | Missing: retryIfNoChange, waitUntilVisible, waitToSettleTimeoutMs, relativePoint |
| `doubleTapOn` | ⚠️ | Same as tapOn (repeat.repeat=2) | OK via repeat param |
| `longPressOn` | ✅ | Same as tapOn (longPress=true) | OK |
| `tapOnPoint` | ⚠️ | point, retryIfNoChange, longPress, repeat, waitToSettleTimeoutMs, label, optional | Missing: waitToSettleTimeoutMs |
| `swipe` | ⚠️ | direction, startPoint, endPoint, elementSelector, startRelative, endRelative, duration, waitToSettleTimeoutMs | Missing: elementSelector (swipe on element), waitToSettleTimeoutMs |
| `scroll` | ✅ | Simple vertical scroll | OK |
| `scrollUntilVisible` | ⚠️ | selector, direction, scrollDuration (speed), visibilityPercentage, timeout, waitToSettleTimeoutMs, centerElement | Missing: visibilityPercentage, centerElement, waitToSettleTimeoutMs |
| `back` | ✅ | label, optional | OK |

---

## 2. Input Commands

### InputRandomCommand (from Commands.kt)

```kotlin
enum class InputRandomType {
    NUMBER,             // Random digits
    TEXT,               // Random text (uses Faker)
    TEXT_EMAIL_ADDRESS, // Random email
    TEXT_PERSON_NAME,   // Random person name
    TEXT_CITY_NAME,     // Random city name
    TEXT_COUNTRY_NAME,  // Random country name
    TEXT_COLOR,         // Random color name
}

data class InputRandomCommand(
    val inputType: InputRandomType? = InputRandomType.TEXT,
    val length: Int? = 8,   // Default length for TEXT and NUMBER
    val label: String? = null,
    val optional: Boolean = false,
)
```

**Maestro uses [Datafaker](https://github.com/datafaker-net/datafaker) library:**
- `faker.number().randomNumber(length)` for NUMBER
- `faker.text().text(length)` for TEXT
- `faker.internet().emailAddress()` for EMAIL
- `faker.name().name()` for PERSON_NAME
- `faker.address().cityName()` for CITY_NAME
- `faker.address().country()` for COUNTRY_NAME
- `faker.color().name()` for COLOR

| Command | Status | Maestro Parameters | Our Status |
|---------|--------|-------------------|------------|
| `inputText` | ✅ | text string with ${var} support | OK |
| `inputRandom` | ✅ | inputType, length (default: 8), label, optional | OK |
| `inputRandomEmail` | ✅ | label, optional | OK |
| `inputRandomNumber` | ✅ | length (default: 8), label, optional | OK |
| `inputRandomPersonName` | ✅ | label, optional | OK |
| `inputRandomText` | ✅ | length (default: 8), label, optional | OK |
| `inputRandomCityName` | ❌ | label, optional | Not implemented |
| `inputRandomCountryName` | ❌ | label, optional | Not implemented |
| `inputRandomColorName` | ❌ | label, optional | Not implemented |
| `eraseText` | ✅ | charactersToErase (null = all), label, optional | OK |
| `hideKeyboard` | ✅ | label, optional | OK |
| `pressKey` | ✅ | code (KeyCode enum), label, optional | OK |

---

## 3. Clipboard Commands

| Command | Status | Notes |
|---------|--------|-------|
| `copyTextFrom` | ✅ | Copy text from element to clipboard |
| `pasteText` | ✅ | Paste from clipboard |
| `setClipboard` | ✅ | Set clipboard to specific text |

---

## 4. Assertion Commands

### AssertConditionCommand (from Commands.kt)

```kotlin
data class AssertConditionCommand(
    val condition: Condition,        // visible, notVisible, or true
    val timeout: String? = null,     // Timeout in ms (as string for variable support)
    val label: String? = null,
    val optional: Boolean = false,
)

data class Condition(
    val visible: ElementSelector? = null,
    val notVisible: ElementSelector? = null,
    val scriptCondition: String? = null,  // JavaScript expression
)
```

### YamlAssertTrue (from YamlAssertTrue.kt)

```kotlin
data class YamlAssertTrue(
    val condition: String? = null,   // JS expression: ${output.foo == "bar"}
    val label: String? = null,
    val optional: Boolean = false,
)

// Supports multiple input types:
// - String: direct condition
// - Int, Long, Boolean, Float, Double: auto-converted to string
// - Map: {condition: "...", label: "...", optional: true}
```

| Command | Status | Notes |
|---------|--------|-------|
| `assertVisible` | ✅ | With text, id, regex, enabled state, timeout |
| `assertNotVisible` | ✅ | Optimized with fast timeout |
| `assertTrue` | ✅ | JavaScript condition evaluation |
| `assertCondition` | ❌ | Unified condition assertion (deprecated) |
| `assertNoDefectsWithAI` | 🚫 | AI feature - not planned |
| `assertWithAI` | 🚫 | AI feature - not planned |
| `extractTextWithAI` | 🚫 | AI feature - not planned |

---

## 5. App Lifecycle Commands

### LaunchAppCommand (from Commands.kt)

```kotlin
data class LaunchAppCommand(
    val appId: String,
    val clearState: Boolean? = null,              // Clear app data before launch
    val clearKeychain: Boolean? = null,           // iOS: clear keychain
    val stopApp: Boolean? = null,                 // Stop app before launch
    val permissions: Map<String, String>? = null, // Permission grants
    val launchArguments: Map<String, Any>? = null, // Launch arguments
    val label: String? = null,
    val optional: Boolean = false,
)
```

### YamlLaunchApp (from YamlLaunchApp.kt)

```kotlin
data class YamlLaunchApp(
    val appId: String?,        // Package ID or Bundle ID (alias: 'url')
    val clearState: Boolean?,   // Clear app data
    val clearKeychain: Boolean?, // iOS keychain clear
    val stopApp: Boolean?,      // Stop before launch
    val permissions: Map<String, String>?, // Permission map
    val arguments: Map<String, Any>?,  // Launch arguments
    val label: String? = null,
    val optional: Boolean = false,
)
```

| Command | Status | Notes |
|---------|--------|-------|
| `launchApp` | ✅ | With permissions (default: all allow), clearState, stopApp |
| `launchApp.arguments` | ❌ | Launch arguments not implemented |
| `stopApp` | ✅ | Graceful stop |
| `killApp` | ✅ | Force stop |
| `clearState` | ✅ | Clear app data |
| `clearKeychain` | ❌ | iOS only - not implemented |
| `setPermissions` | ✅ | Standalone permission setting |

### Permission System (Implemented)

```yaml
# Default behavior (Maestro-compatible)
- launchApp: com.example.app  # Grants ALL permissions by default

# Explicit permissions
- launchApp:
    appId: com.example.app
    permissions:
      camera: allow
      location: allow
      microphone: deny

# Standalone permission setting
- setPermissions:
    appId: com.example.app
    permissions:
      all: allow

# Permission shortcuts supported:
# location, camera, contacts, phone, microphone, bluetooth,
# storage, notifications, medialibrary, calendar, sms
```

**Driver Implementation:**
| Driver | Method |
|--------|--------|
| UIAutomator2 | `adb shell pm grant/revoke` |
| WDA (iOS) | `xcrun simctl privacy grant/revoke` |
| Appium | `mobile: shell` → `pm grant/revoke` |

---

## 6. Device Control Commands

| Command | Status | Notes |
|---------|--------|-------|
| `setLocation` | ✅ | Set GPS coordinates |
| `setOrientation` | ✅ | PORTRAIT, LANDSCAPE |
| `setAirplaneMode` | ✅ | Enable/disable |
| `toggleAirplaneMode` | ✅ | Toggle current state |
| `travel` | ✅ | Simulate travel between points |

---

## 7. Navigation Commands

| Command | Status | Notes |
|---------|--------|-------|
| `openLink` | ✅ | Open URL/deep link |
| `openBrowser` | ✅ | Open URL in browser |

---

## 8. Wait Commands

| Command | Status | Notes |
|---------|--------|-------|
| `extendedWaitUntil` | ✅ | Wait for condition with timeout |
| `waitForAnimationToEnd` | ✅ | Wait for UI to settle |

---

## 9. Flow Control Commands

### YamlCondition (from YamlCondition.kt)

```kotlin
data class YamlCondition(
    val platform: Platform? = null,      // 'iOS' or 'Android'
    val visible: YamlElementSelectorUnion? = null,
    val notVisible: YamlElementSelectorUnion? = null,
    val `true`: String? = null,          // JavaScript expression
    val label: String? = null,
    val optional: Boolean = false,
)
```

### YamlRepeatCommand (from YamlRepeatCommand.kt)

```kotlin
data class YamlRepeatCommand(
    val times: String? = null,           // Repeat count (or infinite if null)
    val `while`: YamlCondition? = null,  // Continue while condition is true
    val commands: List<YamlFluentCommand>,
    val label: String? = null,
    val optional: Boolean = false,
)
```

### YamlRunFlow (from YamlRunFlow.kt)

```kotlin
data class YamlRunFlow(
    val file: String? = null,            // Subflow file path
    val `when`: YamlCondition? = null,   // Conditional execution
    val env: Map<String, String> = emptyMap(), // Override env vars
    val commands: List<YamlFluentCommand>? = null, // Inline commands
    val label: String? = null,
    val optional: Boolean = false,
)
```

### YamlRetryCommand (from YamlRetry.kt)

```kotlin
data class YamlRetryCommand(
    val maxRetries: String? = null,      // Number of retry attempts
    val file: String? = null,            // Subflow file (optional)
    val commands: List<YamlFluentCommand>? = null, // Inline commands
    val env: Map<String, String> = emptyMap(),
    val label: String? = null,
    val optional: Boolean = false,
)
```

| Command | Status | Notes |
|---------|--------|-------|
| `runFlow` | ✅ | Run subflow with file path or inline commands |
| `runFlow.when` | ✅ | Conditional execution |
| `runFlow.env` | ✅ | Override env vars |
| `runFlow.commands` | ✅ | Inline commands |
| `repeat` | ✅ | Repeat steps N times |
| `repeat.while` | ✅ | Repeat while condition |
| `retry` | ✅ | Retry on failure with maxRetries |
| `retry.file` | ✅ | Retry subflow file |
| `runScript` | ✅ | Run JavaScript file |
| `evalScript` | ✅ | Evaluate JS expression |
| `defineVariables` | ✅ | Define flow variables |

---

## 10. Media Commands

| Command | Status | Notes |
|---------|--------|-------|
| `takeScreenshot` | ✅ | Save screenshot to file |
| `startRecording` | ✅ | Start video recording |
| `stopRecording` | ✅ | Stop and save recording |
| `addMedia` | ✅ | Add media files to device |

---

## 11. AI Commands (Not Planned)

| Command | Status | Notes |
|---------|--------|-------|
| `assertNoDefectsWithAI` | 🚫 | Requires Maestro Cloud |
| `assertWithAI` | 🚫 | Requires Maestro Cloud |
| `extractTextWithAI` | 🚫 | Requires Maestro Cloud |

---

## 12. Selector Features

### ElementSelector (from ElementSelector.kt)

```kotlin
data class ElementSelector(
    val textRegex: String? = null,              // Text matching (regex)
    val idRegex: String? = null,                // ID matching (regex)
    val size: SizeSelector? = null,             // Size-based matching
    val below: ElementSelector? = null,         // Relative: below another
    val above: ElementSelector? = null,         // Relative: above another
    val leftOf: ElementSelector? = null,        // Relative: left of another
    val rightOf: ElementSelector? = null,       // Relative: right of another
    val containsChild: ElementSelector? = null, // Contains child element
    val containsDescendants: List<ElementSelector>? = null,
    val traits: List<ElementTrait>? = null,     // Element traits
    val index: String? = null,                  // Index selection (supports negative)
    val enabled: Boolean? = null,               // Enabled state
    val optional: Boolean = false,              // Don't fail if not found
    val selected: Boolean? = null,              // Selected state
    val checked: Boolean? = null,               // Checked state
    val focused: Boolean? = null,               // Focused state
    val childOf: ElementSelector? = null,       // Is child of another
    val css: String? = null,                    // CSS selector (web only)
)

data class SizeSelector(
    val width: Int? = null,
    val height: Int? = null,
    val tolerance: Int? = null,  // Allowed deviation in pixels
)
```

### YamlElementSelector (from YamlElementSelector.kt)

Additional YAML-only fields (not in ElementSelector model):
```kotlin
val retryTapIfNoChange: Boolean? = null,    // Retry if tap doesn't change UI
val waitUntilVisible: Boolean? = null,      // Wait for element before action
val point: String? = null,                  // Relative tap point "50%,50%"
val repeat: Int? = null,                    // Repeat count
val delay: Int? = null,                     // Delay between repeats
val waitToSettleTimeoutMs: Int? = null,     // Wait for UI settle
val label: String? = null,                  // Custom label for selector
val insideOf: YamlElementSelectorUnion? = null, // Alias for childOf
```

### ElementTrait (from ElementTrait.kt)

```kotlin
enum class ElementTrait(val description: String) {
    TEXT("Has text"),           // Element has text content
    SQUARE("Is square"),        // Element is square shaped
    LONG_TEXT("Has long text"), // Element has long text content
}
```

| Feature | Status | Notes |
|---------|--------|-------|
| `text` (textRegex) | ✅ | Exact or regex match |
| `id` (idRegex) | ✅ | Resource ID match (regex) |
| `index` | ✅ | Select by index (supports negative) |
| `enabled` | ✅ | Filter by enabled state |
| `selected` | ✅ | Filter by selected state |
| `checked` | ✅ | Filter by checked state |
| `focused` | ✅ | Filter by focused state |
| `optional` | ✅ | Don't fail if not found |
| `childOf` | ✅ | Element is child of another |
| `containsChild` | ✅ | Element contains child |
| `containsDescendants` | ✅ | Element contains descendants |
| `below` | ✅ | Element below another |
| `above` | ✅ | Element above another |
| `leftOf` | ✅ | Element left of another |
| `rightOf` | ✅ | Element right of another |
| `size` (width/height/tolerance) | ❌ | Size-based matching |
| `traits` | ❌ | TEXT, SQUARE, LONG_TEXT traits |
| `css` | ❌ | CSS selector (web views only) |
| `point` (relativePoint) | ❌ | Element-relative tap "50%,50%" |
| `insideOf` | ✅ | Alias for childOf |

---

## 13. Flow Configuration

### YamlConfig (from YamlConfig.kt)

```kotlin
data class YamlConfig(
    val name: String?,                           // Flow name
    val appId: String?,                          // Package/Bundle ID (or URL)
    val url: String?,                            // Web app URL
    val tags: List<String>? = emptyList(),       // Tags for filtering
    val env: Map<String, String> = emptyMap(),   // Environment variables
    val onFlowStart: YamlOnFlowStart?,           // Commands before flow
    val onFlowComplete: YamlOnFlowComplete?,     // Commands after flow (even on error)
    val properties: Map<String, String> = emptyMap(), // Custom properties
    val ext: MutableMap<String, Any?> = mutableMapOf(), // Extension config
)
```

### onFlowStart / onFlowComplete

```yaml
appId: com.example.app
onFlowStart:
  - launchApp
  - assertVisible: "Welcome"

onFlowComplete:
  - takeScreenshot: "final_state"
  - stopApp
```

**Important:** `onFlowComplete` runs even if the flow fails.

### Extension Config (ext)

```yaml
appId: com.example.app
ext:
  jsEngine: graal              # or 'rhino' (default)
  androidWebViewHierarchy: devtools  # Chrome DevTools for WebView
```

| Feature | Status | Maestro Support | Our Status |
|---------|--------|-----------------|------------|
| `appId` in frontmatter | ✅ | Package/Bundle ID | OK |
| `url` in frontmatter | ❌ | Web app URL | Not implemented |
| `name` in frontmatter | ✅ | Flow name | OK |
| `tags` | ✅ | Tag filtering | OK |
| `env` variables | ✅ | ${VAR} substitution | OK |
| `onFlowStart` | ❌ | Commands before main flow | Not implemented |
| `onFlowComplete` | ❌ | Commands after flow (even on error) | Not implemented |
| `ext.jsEngine` | ❌ | graal or rhino | Not implemented |
| `ext.androidWebViewHierarchy` | ❌ | devtools mode | Not implemented |
| `properties` | ❌ | Custom key-value pairs | Not implemented |

---

## 14. CLI Features

| Feature | Status | Notes |
|---------|--------|-------|
| `--platform` | ✅ | ios, android |
| `--device` | ✅ | Device UDID |
| `--driver` | ✅ | uiautomator2, appium, wda |
| `--appium-url` | ✅ | Appium server URL |
| `--env` | ✅ | Environment variables |
| `--output` | ✅ | Report output directory |
| `--include-tags` | ✅ | Run flows with tags |
| `--exclude-tags` | ✅ | Skip flows with tags |
| `--caps` | ✅ | Appium capabilities file |
| `--app-file` | ✅ | Install APK/IPA before run |

---

## Missing Features Roadmap

### Priority 1 - Core Command Parameters (High Impact)

| Feature | Command | Complexity | Notes |
|---------|---------|------------|-------|
| `waitToSettleTimeoutMs` | tapOn, swipe, scroll | Low | Wait for UI settlement (max 30000ms) |
| `retryIfNoChange` | tapOn, tapOnPoint | Medium | Retry if UI doesn't change |
| `waitUntilVisible` | tapOn | Low | Wait for element before action |
| `relativePoint` | tapOn | Low | Element-relative tap "50%,50%" |
| `elementSelector` | swipe | Medium | Swipe on specific element |
| `visibilityPercentage` | scrollUntilVisible | Low | 0-100% visibility threshold |
| `centerElement` | scrollUntilVisible | Low | Center element after found |

### Priority 2 - Flow Configuration

| Feature | Complexity | Notes |
|---------|------------|-------|
| `onFlowStart` | Medium | Commands before main flow |
| `onFlowComplete` | Medium | Commands after flow (even on error) |
| `launchArguments` | Low | Pass arguments to app launch |
| `clearKeychain` | Low | iOS keychain clearing |
| `url` config | Low | Web app testing support |

### Priority 3 - Input Variants (Use Datafaker)

| Feature | Complexity | Notes |
|---------|------------|-------|
| `inputRandomCityName` | Low | `faker.address().cityName()` |
| `inputRandomCountryName` | Low | `faker.address().country()` |
| `inputRandomColorName` | Low | `faker.color().name()` |

### Priority 4 - Advanced Selectors

| Feature | Complexity | Notes |
|---------|------------|-------|
| `size` (width/height/tolerance) | Low | Size-based element matching |
| `traits` | Medium | TEXT, SQUARE, LONG_TEXT |
| `css` | Medium | CSS selector (web views only) |

### Priority 5 - Configuration

| Feature | Complexity | Notes |
|---------|------------|-------|
| `ext.jsEngine` | Low | graal or rhino |
| `ext.androidWebViewHierarchy` | Medium | Chrome DevTools for WebView |
| `properties` | Low | Custom key-value pairs |

### Priority 6 (Nice to Have)

| Feature | Complexity | Notes |
|---------|------------|-------|
| Parallel device execution | High | Run on multiple devices |
| Screenshot comparison | Medium | Visual regression testing |
| Maestro Studio compatibility | High | Interactive test builder |

---

## Maestro Default Values (Reference)

| Parameter | Default | Max | Notes |
|-----------|---------|-----|-------|
| `lookupTimeoutMs` | 17000ms | - | Element finding timeout |
| `optionalLookupTimeoutMs` | 7000ms | - | Optional element timeout |
| `waitToSettleTimeoutMs` | none | 30000ms | UI settlement wait |
| Swipe duration | 400ms | - | Default swipe animation |
| scrollUntilVisible speed | 40 | 100 | Speed units (not ms) |
| scrollUntilVisible timeout | 20000ms | - | Scroll timeout |
| inputRandom length | 8 | - | Default random string length |
| repeat delay | 100ms | - | Between tap repeats |

### Speed to Duration Conversion (scrollUntilVisible)
```
duration_ms = 1000 * (100 - speed) / 100 + 1
speed=40  → ~600ms
speed=90  → ~100ms
```

---

## Hidden/Undocumented Maestro Features

Based on source code analysis:

1. **Element-relative tapping**: `tapOn` supports `relativePoint: "50%,50%"` for tapping relative to element center
2. **Inline flow commands**: `runFlow.commands` for inline steps without separate file
3. **Conditional flows**:
   - `when.platform: iOS` - Platform-specific execution
   - `when.visible` - Execute if element visible
   - `when.notVisible` - Execute if element not visible
   - `when.true` - JavaScript condition
4. **Infinite repeat**: `repeat.while` without `times` repeats until condition is false
5. **AssertTrue type coercion**: Supports String, Int, Long, Boolean, Float, Double, or Map input
6. **Element traits**: Limited to TEXT, SQUARE, LONG_TEXT (not UI traits like "button")
7. **Percentage swipe**: Both start and end must include `%` for relative coordinates
8. **Speed to Duration**: scrollUntilVisible speed (0-100) converts to: `duration = 1000 * (100 - speed) / 100 + 1`
9. **Index negative**: Selector `index: "-1"` selects last matching element
10. **InsideOf alias**: `insideOf` is an alias for `childOf` selector
11. **URL as appId**: Web flows can use `url:` instead of `appId:` in config

---

## Implementation Statistics

| Category | Maestro Total | Implemented | Partial | Missing |
|----------|---------------|-------------|---------|---------|
| Interaction | 6 | 5 | 3 | 0 |
| Input | 10 | 7 | 0 | 3 |
| Clipboard | 3 | 3 | 0 | 0 |
| Assertions | 5 | 2 | 1 | 2 (AI) |
| App Management | 6 | 5 | 0 | 1 |
| Device Control | 5 | 5 | 0 | 0 |
| Navigation | 4 | 4 | 0 | 0 |
| Wait | 2 | 2 | 0 | 0 |
| Flow Control | 3 | 3 | 0 | 0 |
| Media | 3 | 3 | 0 | 0 |
| **TOTAL** | **47** | **39** | **4** | **4** |

**Parity: ~83% fully implemented, ~91% with partial**

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 0.1.0 | - | Initial implementation |
| 0.2.0 | - | Added setPermissions, default all:allow in launchApp |
| 0.3.0 | - | Deep dive parity analysis |
| 0.4.0 | 2026-01-22 | Comprehensive source code deep dive with command definitions |

### Changes in 0.4.0

- Added complete command structures from `Commands.kt`
- Documented all ElementSelector properties with source code
- Added YamlElementSelector extra fields (YAML-only)
- Documented ElementTrait enum values
- Added YamlCondition, YamlRepeatCommand, YamlRunFlow, YamlRetryCommand structures
- Updated YamlConfig with onFlowStart/onFlowComplete details
- Added LaunchAppCommand parameters including `launchArguments`
- Documented InputRandomType enum (7 types using Datafaker library)
- Updated roadmap with specific command parameters
- Added 11 hidden/undocumented features from source analysis
