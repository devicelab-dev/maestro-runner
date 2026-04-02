# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **`--persist` flag** (`MAESTRO_PERSIST` env var) — keeps WebDriverAgent running between invocations for faster repeated test runs. On startup, reuses an existing healthy WDA session (leaving the app alive); on exit, leaves WDA running instead of tearing it down. The xcodebuild process is detached into its own process group so it survives `maestro-runner` exiting.
  ```bash
  # First run: builds and launches WDA normally
  maestro-runner --persist --platform ios test flow.yaml

  # Subsequent runs: reuses existing WDA session (~30-60s faster)
  maestro-runner --persist --platform ios test flow.yaml
  ```

## [1.1.12] - 2026-04-22

### Added
- **Tap options** — `repeat`, `delay`, `retryTapIfNoChange`, and `waitToSettleTimeoutMs` now
  honored during execution on all drivers (uiautomator2, wda, devicelab, appium, cdp).
  Implemented at the executor layer, zero driver-side changes.
  ([#52](https://github.com/devicelab-dev/maestro-runner/issues/52), [#53](https://github.com/devicelab-dev/maestro-runner/pull/53))
  ```yaml
  - tapOn:
      id: "login-button"
      repeat: 3
      delay: 500
      retryTapIfNoChange: true
      waitToSettleTimeoutMs: 2000
  ```
- **runFlow timeout** — `timeout:` parameter on `runFlow` steps with context propagation
  into driver polling loops. Element-finding cancels immediately on expiry, and failures
  are classified as `TIMEOUT` in reports. Ref
  [#29](https://github.com/devicelab-dev/maestro-runner/issues/29), thanks to
  [@maraujop](https://github.com/maraujop) for the suggestion.
  ```yaml
  - runFlow:
      file: common/login.yaml
      timeout: 5000
      env:
        username: devicelab
  ```
- **Cloud Provider lifecycle hooks** — `Provider` interface now exposes `OnRunStart`,
  `OnFlowStart`, and `OnFlowEnd` alongside the existing `ExtractMeta` and `ReportResult`.
  Cloud integrations can update dashboards live per-flow instead of only at run end.
  Sauce Labs ships with no-op placeholders for the new hooks.
- **UI.waitForSettle RPC** — on-device tree-comparison settle detection on the DeviceLab
  Android driver, used as an auto-settle before `inputText` / `eraseText` to avoid key
  events firing mid-transition.
- **Clickable-ancestor promotion** — when a DeviceLab tap matches text on a non-clickable
  descendant (e.g. `"Sign In"` TextView inside a clickable login-button `ViewGroup`), the
  agent now walks up to the nearest clickable ancestor.
- **hintText matching** — `hintContains` / `hintMatches` UiSelector extensions on the
  DeviceLab driver match an `EditText`'s `android:hint` placeholder. Lets
  `tapOn: "Email"` find an empty email field by its hint.
- **Case-insensitive text matching on Android** — `textContains` / `descriptionContains`
  now fall back to case-insensitive match when case-sensitive fails, fixing Android dialog
  buttons where `textAllCaps` displays `"CANCEL"` but the view hierarchy text is
  `"Cancel"`. Reported by [@satya164](https://github.com/satya164).
- **Appium parallel execution** — run flows across N Appium sessions concurrently. Each
  session connects to the same Appium URL; the server allocates devices.
  ([#47](https://github.com/devicelab-dev/maestro-runner/pull/47))
- **`--wda-bundle-id` flag** — custom WebDriverAgent bundle identifier for signing
  scenarios where the default bundle id isn't usable.
  ([#48](https://github.com/devicelab-dev/maestro-runner/pull/48))
- **Device info in Appium reports** — device info and session ID now surface in console
  output and JUnit/Allure reports for Appium runs.

### Changed
- **Simpler `inputText` without selector** — DeviceLab and UIAutomator2 drivers now send
  key events directly via `SendKeyActions` instead of attempting
  `findFocused` / `ActiveElement` fallbacks. Matches Maestro's "type into whatever the OS
  has focused" behavior.
- Updated DeviceLab Android driver APK to ship `UI.waitForSettle`, clickable-ancestor
  promotion, and hintText predicate support.
- Appium parallel session count is capped at the number of flows (prints a warning
  when parallel count exceeds flow count).

### Fixed
- **iOS install hang on iOS 17+ / iOS 26** — prefer `xcrun devicectl device install app`
  over the legacy `go-ios` zipconduit path on real devices. Both paths now run under a
  3-minute context timeout so a stuck install surfaces as an error instead of an infinite
  spinner. Escape hatch via `MAESTRO_RUNNER_IOS_INSTALLER=zipconduit|devicectl`. Fixes
  [#54](https://github.com/devicelab-dev/maestro-runner/issues/54), thanks to
  [@ptmkenny](https://github.com/ptmkenny) for the clear repro.
- **`clearKeychain` on iOS** — standalone `clearKeychain` step and
  `launchApp { clearKeychain: true }` both now work. Previously the step erred with
  `Step type '*flow.ClearKeychainStep' is not supported on iOS`, and the `launchApp`
  flag was a silent no-op (users stayed logged in). On simulators runs
  `xcrun simctl keychain <udid> reset`; on real devices returns a clear unsupported
  message pointing to `clearState` as the alternative. Fixes
  [#57](https://github.com/devicelab-dev/maestro-runner/issues/57), thanks to
  [@ross-aker](https://github.com/ross-aker) for reporting.
- **Swipe `LEFT` / `RIGHT` on Android** — use screen coordinates directly instead of the
  previous element-relative computation that misbehaved.
- **`when: { true: <expr> }` silently always-true** — the `true:` field wasn't parsed
  (YAML tag bound to the internal `scriptCondition` name instead), so conditions were
  ignored and commands always ran. Fixes
  [#60](https://github.com/devicelab-dev/maestro-runner/issues/60), reported by
  [@satya164](https://github.com/satya164) and
  [@kavithamahesh](https://github.com/kavithamahesh).
- **Env var default syntax** — `${VAR || "default"}` and `${VAR ?? "fallback"}` now
  resolve correctly. Undefined JS variables auto-define as `undefined` on
  `ReferenceError`, matching Maestro's GraalJS Proxy behavior. Fixes
  [#49](https://github.com/devicelab-dev/maestro-runner/issues/49),
  [#50](https://github.com/devicelab-dev/maestro-runner/issues/50).

### Contributors

[@ptmkenny](https://github.com/ptmkenny)
1. Reported the iOS install hang on iOS 17+/26 with a clear repro ([#54](https://github.com/devicelab-dev/maestro-runner/issues/54))

[@ross-aker](https://github.com/ross-aker)
1. Reported `clearKeychain` not working on iOS Simulator ([#57](https://github.com/devicelab-dev/maestro-runner/issues/57))

[@satya164](https://github.com/satya164)
1. Reported Android dialog `textAllCaps` case mismatch (`CANCEL` vs `Cancel`)
2. Reported `when: { true: <expr> }` parsing bug (duplicated by [#60](https://github.com/devicelab-dev/maestro-runner/issues/60))

[@kavithamahesh](https://github.com/kavithamahesh)
1. Reported `when.true` condition ignored ([#60](https://github.com/devicelab-dev/maestro-runner/issues/60))

[@maraujop](https://github.com/maraujop)
1. Suggested `runFlow` timeout ([#29](https://github.com/devicelab-dev/maestro-runner/issues/29))

## [1.1.1] - 2026-04-06

### Added
- **Cloud provider abstraction** — automatic detection and result reporting for cloud device providers (Sauce Labs, BrowserStack, LambdaTest, etc.) when using the Appium driver. Test pass/fail status, flow results, and metadata are reported to the provider after the run completes. Based on [@eyaly](https://github.com/eyaly)'s Sauce Labs integration ([#43](https://github.com/devicelab-dev/maestro-runner/pull/43), [#45](https://github.com/devicelab-dev/maestro-runner/pull/45))
  ```bash
  # Sauce Labs — automatically detected from the Appium URL
  maestro-runner --driver appium --appium-url "https://ondemand.us-west-1.saucelabs.com/wd/hub" \
    --caps caps.json test flows/
  ```
- **Source file path in FlowResult** — each flow result now includes the path to the source YAML file, used by cloud providers and report consumers

### Changed
- Updated DeviceLab Android driver APK with latest on-device agent
- Airplane mode commands now use `cmd connectivity airplane-mode enable/disable` (Android 11+) instead of the legacy `settings put global airplane_mode_on` approach

### Fixed
- **CDP `waitForPageReady` crash** — replaced panicking `MustWaitLoad()` with error-handling `WaitLoad()` in the browser CDP driver, preventing test run crashes on pages with deeply nested object references
- Removed unused `freePort()` function from DeviceLab WebView driver
- Removed unused regex variables (`reLabel`, `reHint`, `reValue`) from Flutter semantics parser
- Tightened variable scope in Flutter widget tree parser

### Contributors

[@eyaly](https://github.com/eyaly)
1. Implemented original Sauce Labs pass/fail reporting integration ([#43](https://github.com/devicelab-dev/maestro-runner/pull/43)), which formed the basis for the cloud provider abstraction in [#45](https://github.com/devicelab-dev/maestro-runner/pull/45)

## [1.1.0] - 2026-03-25

### Added
- **WebView CDP support for Android** — the DeviceLab driver now connects to WebViews via Chrome DevTools Protocol for element finding and JavaScript execution, instead of relying solely on the native UiAutomator accessibility tree
  ```bash
  # Automatic — when a WebView is detected, CDP is used transparently
  maestro-runner --driver devicelab test webview-flow.yaml
  ```
- **Chrome browser CDP on Android** — the DeviceLab driver can now automate Chrome browser on Android devices via CDP, enabling web testing on real Android devices
- **`evalWebViewScript` command** — execute inline JavaScript in a mobile WebView via CDP. Returns the result as a string, optionally stored in an output variable
  ```yaml
  # Inline script
  - evalWebViewScript: "return document.title"

  # With output variable
  - evalWebViewScript:
      script: "return document.querySelector('#price').textContent"
      output: price

  # Use the result
  - assertTrue: ${price == '$7.50'}
  ```
- **`runWebViewScript` command** — load and execute a JavaScript file in a mobile WebView via CDP. Supports environment variables injected as `window.__env`
  ```yaml
  # Simple file execution
  - runWebViewScript: scripts/extract-data.js

  # With environment variables and output
  - runWebViewScript:
      file: scripts/validate-cart.js
      env:
        EXPECTED_TOTAL: "29.99"
      output: validationResult
  ```
- **Network idle detection and DOM stability waits** — after navigations (in both browser and WebView contexts), maestro-runner now waits for network idle and DOM stability before proceeding, reducing flakiness on pages with async loading
- **CDP RAF-based visibility polling** — browser commands now use `requestAnimationFrame`-based polling for element visibility, improving reliability for dynamically rendered content
- **CDP `<select>` option support** — `tapOn` with option elements now correctly selects the option via JavaScript instead of attempting a click
- **CDP JS click fallback** — when a native click fails on a browser element, falls back to JavaScript `.click()` for better reliability with overlapping elements

### Changed
- Default WDA swipe duration changed from 300ms to 100ms for faster, more responsive swipe gestures on iOS
- JavaScript helper code extracted from Go string literals into dedicated embedded `.js` files for easier maintenance ([#37](https://github.com/devicelab-dev/maestro-runner/pull/37))

### Fixed
- **Swipe coordinates now match Maestro behavior** across all drivers (UIAutomator2, DeviceLab, WDA, Appium) — previously, swipe start/end positions differed from Maestro's implementation
- **`assertNotVisible` now correctly polls for disappearance** instead of polling for appearance — previously, the command would pass immediately if the element wasn't visible, without waiting for it to disappear after an action
- **Filter out-of-bounds elements from page source searches** — elements with coordinates outside the visible screen bounds are now excluded from search results, preventing false matches on off-screen elements ([#39](https://github.com/devicelab-dev/maestro-runner/issues/39))
- **Text node attribute error** — fixed `TypeError: this.getAttribute is not a function` when browser CDP encounters text nodes that don't have HTML attributes ([#35](https://github.com/devicelab-dev/maestro-runner/issues/35), [#36](https://github.com/devicelab-dev/maestro-runner/pull/36))
- **iOS WDA session lifecycle** — improved driver reliability with better session creation, cleanup, and error recovery
- **`--team-id` no longer required for auto-detected simulators** — when a booted simulator is auto-detected, `--team-id` is automatically skipped since simulators don't need code signing
  ```bash
  # Before: required --team-id even when simulator is already booted
  # Now: just works
  maestro-runner --platform ios test flow.yaml
  ```
- **Flutter reconnection** — skip retries for non-Flutter apps instead of wasting time on connection attempts. Non-Flutter apps now pay zero retry cost
- **WebView CDP forwarder** — wired `SetWebViewForwarder` in the DeviceLab driver, which was never connected — elements were previously found only via native UiAutomator accessibility tree even when a WebView was present
- **hideKeyboard reliability** — on-device agent now uses `KEYCODE_ESCAPE` first (keyboard-only, no navigation side-effects), falls back to `KEYCODE_BACK` if needed. Retries up to 3 times with keyboard visibility polling
- **In-WebView navigation** — when visibility check fails during in-WebView page navigation (JS context destroyed), refreshes page reference and retries instead of skipping CDP entirely
- **CDP text match filtering** — text-based visibility checks (`text`, `textContains`, `textRegex`) now filter to the deepest matching element, preventing false positives from ancestor elements whose `textContent` includes hidden children's text

### Contributors

[@tmahesh](https://github.com/tmahesh)
1. Fixed text node attribute error in browser CDP ([#36](https://github.com/devicelab-dev/maestro-runner/pull/36))
2. Refactored JS helper code into embedded files ([#37](https://github.com/devicelab-dev/maestro-runner/pull/37))

[@mahesh-e27](https://github.com/mahesh-e27)
1. Reported text node attribute bug in browser CDP ([#35](https://github.com/devicelab-dev/maestro-runner/issues/35))

[@sircharleswatson](https://github.com/sircharleswatson)
1. Reported `assertVisible` passing for off-screen text in browser ([#39](https://github.com/devicelab-dev/maestro-runner/issues/39))

[@satishs22](https://github.com/satishs22)
1. Reported `tapOn` timeout issue on Android emulator ([#25](https://github.com/devicelab-dev/maestro-runner/issues/25))

[@chrisjin-swipe](https://github.com/chrisjin-swipe)
1. Reported `inputText` character skipping on Android ([#32](https://github.com/devicelab-dev/maestro-runner/issues/32))

## [1.0.9] - 2026-03-11

### Added
- **Desktop browser testing** — new `--platform web` with built-in CDP driver for Chrome/Chromium. Headless by default, `--headed` for visible browser. Supports parallel browser execution
  ```bash
  maestro-runner --platform web test flow.yaml
  maestro-runner --platform web --headed --browser chrome test flow.yaml
  maestro-runner --platform web test --parallel 3 flows/
  ```
- **Browser-specific commands** — `evalBrowserScript`, `setCookies`, `getCookies`, `saveAuthState`, `loadAuthState`, `openTab`, `switchTab`, `closeTab`, `mockNetwork`, `blockNetwork`, `setNetworkConditions`, `waitForRequest`, `clearNetworkMocks`, `uploadFile`, `waitForDownload`, `grantPermissions`, `resetPermissions`, `getConsoleLogs`, `clearConsoleLogs`, `assertNoJSErrors`, `runBrowserScript`
- **Browser selectors** — `css` and `xpath` selectors for web elements, in addition to `text` and `id`
  ```yaml
  - tapOn:
      css: "button.submit"
  - inputText:
      id: "username"
      text: "hello"
  ```
- `--no-app-install` flag — skip app installation even if `--app-file` is provided. Useful when the app is already installed
  ```bash
  maestro-runner --no-app-install --app-file app.apk test flow.yaml
  ```
- `--no-driver-install` flag — skip driver installation (UIAutomator2, WDA, DeviceLab). Useful when drivers are already installed on the device
  ```bash
  maestro-runner --no-driver-install test flow.yaml
  ```
- Flutter VM Service fallback for element finding — when the native driver (WDA/UIAutomator2) can't find a Flutter element, automatically discovers the Dart VM Service and searches the semantics/widget trees in parallel. Works on Android and iOS simulators. Non-Flutter apps pay only one log read on first miss, then fully bypassed. Disable with `--no-flutter-fallback`
- Flutter widget tree cross-reference — when semantics tree search fails, falls back to widget tree analysis (hint text, identifiers, suffix icons) and cross-references with semantics nodes for coordinates
- DeviceLab Android driver — WebSocket-based on-device automation with bounds stabilization for animated elements and special character handling. ~2x faster than UIAutomator2
  ```bash
  maestro-runner --driver devicelab --platform android test flow.yaml
  ```
- `setAirplaneMode` and `toggleAirplaneMode` commands for iOS (WDA) — automates the Settings app to toggle airplane mode on real devices. Supports both mapping and scalar syntax
  ```yaml
  # Mapping syntax
  - setAirplaneMode:
      enabled: true

  # Scalar syntax
  - setAirplaneMode: enabled
  - setAirplaneMode: disabled

  # Toggle (flips current state)
  - toggleAirplaneMode
  ```
- `maxTypingFrequency` support for WDA (iOS) — configurable typing speed via `--typing-frequency` flag. Default: 30 keys/sec (WDA default is 60). Useful for React Native apps where the JS bridge can't keep up at full speed
  ```bash
  maestro-runner --typing-frequency 15 test flow.yaml
  ```
  ```yaml
  # Or set per-flow in YAML config section:
  appId: com.example.app
  typingFrequency: 20
  ---
  - inputText: "hello world"
  ```
- `maxScrolls` and `timeout` fields wired up in `scrollUntilVisible` for all 4 drivers — previously parsed but ignored, now each driver uses dual-condition loop (max scrolls AND timeout)
  ```yaml
  - scrollUntilVisible:
      element:
        text: "Sign Out"
      direction: "down"
      maxScrolls: 5
      timeout: 10000
  ```
- On-failure WebView detection with CDP-aware error enrichment — background CDP socket monitor with push event architecture
- Regex pattern support for ID selectors across all drivers — use regex patterns like wildcards, alternation, and character classes in `id` selectors
  ```yaml
  # Wildcard
  - tapOn:
      id: "username-.*"

  # Alternation
  - assertVisible:
      id: "(username|email)-input"

  # Suffix anchor
  - tapOn:
      id: "login.*-button$"
  ```
- `repeat` with `while` condition now loops correctly instead of executing only once. Supports configurable timeout for the condition check
  ```yaml
  - repeat:
      while:
        visible: "Delete"
        timeout: 2000    # ms to wait before declaring element gone
      commands:
        - tapOn: "Delete"
  ```
- Cloud Providers section in README with TestingBot setup guide

### Fixed
- iOS simulator no longer requires `--team-id` — simulators don't need code signing, so the validation now only enforces `--team-id` for real devices
  ```bash
  # Before: required --team-id even for simulators
  # Now: just works
  maestro-runner --platform ios --start-simulator <UDID> test flow.yaml
  ```
- `runFlow: when` conditions with variable expressions (e.g., `${output.element.id}`) were never expanded, causing conditions to always evaluate as false and silently skip conditional blocks
- iOS real device: `acceptAlertButtonSelector` matched "Don't Allow" instead of "Allow" — `CONTAINS[c] 'Allow'` matched both buttons, causing WDA to reject permission dialogs. Changed to `BEGINSWITH[c] 'Allow'` with `OK` fallback for older iOS versions
- `AllocatePort` was ignoring existing port allocations and `assertCondition` had duplicate `timeout` yaml tag
- `repeat` with `while` condition executed only once instead of looping
- `repeat-while` condition check timeout reduced from 17s to 7s default
- Implicit wait warning resolved by using Appium settings endpoint
- `assertVisible` optional timeout and optimized tap element finding
- WDA `launchApp` optimized: parallel permissions and removed sleeps
- Element finding consolidated: single query with prefetched element name, merged WDA session settings into single HTTP call
- Android `setAirplaneMode`/`toggleAirplaneMode` failed with `SecurityException: Permission Denial` on Android 7+ — `am broadcast` requires system-level permissions. Now uses `cmd connectivity airplane-mode` on Android 11+ (no root needed), with `settings put` + broadcast fallback for older versions ([#27](https://github.com/devicelab-dev/maestro-runner/issues/27))

### Contributors

[@gdealmeida1885](https://github.com/gdealmeida1885)
1. Fixed variable expansion in `runFlow` `when` conditions ([#10](https://github.com/devicelab-dev/maestro-runner/pull/10))

[@maggialejandro](https://github.com/maggialejandro)
1. Fixed `acceptAlertButtonSelector` matching "Don't Allow" instead of "Allow" ([#24](https://github.com/devicelab-dev/maestro-runner/pull/24))

[@7ammer](https://github.com/7ammer)
1. Reported `repeat` with `while` condition executing only once ([#23](https://github.com/devicelab-dev/maestro-runner/issues/23))
2. Reported implicit wait warning with Appium settings endpoint

[@wrench7](https://github.com/wrench7)
1. Reported `setAirplaneMode` scalar syntax parsing issue ([#27](https://github.com/devicelab-dev/maestro-runner/issues/27))
2. Reported `setAirplaneMode` broadcast permission denied on Android 7+ ([#27](https://github.com/devicelab-dev/maestro-runner/issues/27))

[@AkashRajvanshi](https://github.com/AkashRajvanshi)
1. Reported regex pattern support for ID selectors ([#22](https://github.com/devicelab-dev/maestro-runner/issues/22))

[@jochen-testingbot](https://github.com/jochen-testingbot)
1. Added TestingBot cloud provider documentation ([#20](https://github.com/devicelab-dev/maestro-runner/pull/20))

## [1.0.7] - 2026-02-20

### Added
- Appium driver: `newSession` option for `launchApp` — creates a fresh Appium session, useful when `clearState` fails on real iOS devices (`mobile: clearApp` unsupported). On iOS real devices with `newSession: true`, `clearState` is skipped since a fresh session already provides clean state ([#14](https://github.com/devicelab-dev/maestro-runner/issues/14))
  ```yaml
  - launchApp:
      appId: com.example.app
      newSession: true
  ```
- Bundled UIAutomator2 server upgraded from v9.9.0 to v9.11.1 with new LaunchApp endpoint (`getLaunchIntentForPackage` + `startActivity`)
- Android: classify error types in report (`element_not_found`, `timeout`, `assertion`, `keyboard_covering`, etc.) for better debugging
- Android: detect keyboard covering elements after `inputText`/`inputRandom` — when the soft keyboard covers a target element, taps land on the keyboard instead of the element. Now detects this with a clear error message suggesting `- hideKeyboard`
- Auto-create iOS simulators when not enough shutdown simulators exist for `--parallel` — created simulators are automatically deleted on shutdown
- Parallel device selection: in-use detection via WDA port check (iOS) and socket check (Android) to skip devices already claimed by another maestro-runner instance

### Fixed
- iOS real device: `clearState` no longer kills WDA connection — replaced `go-ios` (`installationproxy`/`zipconduit` over usbmuxd) with `xcrun devicectl` (over Apple's `remoted` daemon), which doesn't interfere with USB port forwarding
- Android: `scroll` and `scrollUntilVisible` direction was inverted — `scroll down` was scrolling up because `/appium/gestures/scroll` already uses scroll semantics, no inversion needed ([#9](https://github.com/devicelab-dev/maestro-runner/issues/9))
- Android: `launchApp` failed with "No apps can perform this action" on certain devices — `resolve-activity` was called without `-a android.intent.action.MAIN -c android.intent.category.LAUNCHER` flags. New three-tier launch strategy: (1) UIAutomator2 server `getLaunchIntentForPackage()` on-device, (2) shell fallback with proper flags + `dumpsys` parsing + API-level-aware `am start`, (3) monkey fallback ([#15](https://github.com/devicelab-dev/maestro-runner/issues/15))
- Android: server APK install now checks version and handles signing conflicts (uninstall + reinstall when version mismatches)
- `index` selector was ignored in simple (non-relative) selectors — `tapOn: text: X, index: 1` always tapped the first match because native driver APIs return only a single element. Now selectors with a non-zero `index` route through page source parsing, which returns all matches and picks the Nth one
- `-e` env variables were not expanding in flow config `appId` — `appId: ${APP_ID}` with `-e APP_ID=com.myapp` sent the literal `${APP_ID}` to adb. Now expands using `ExpandVariables()` before setting as a variable ([#12](https://github.com/devicelab-dev/maestro-runner/issues/12))
- Parallel device selection: devices are now filtered by platform (excludes tvOS/watchOS/xrOS) and in-use devices are skipped ([#11](https://github.com/devicelab-dev/maestro-runner/issues/11))
- Android: emulator port allocation skipped ports occupied by running emulators
- CLI: flags must come before flow paths in command examples

### Contributors

[@ditzdragos](https://github.com/ditzdragos)
1. Reported `launchApp` "No apps can perform this action" on Android ([#15](https://github.com/devicelab-dev/maestro-runner/issues/15))

[@popatre](https://github.com/popatre)
1. Reported `clearState` failing on real iOS devices via Appium ([#14](https://github.com/devicelab-dev/maestro-runner/issues/14))

[@hyry2024](https://github.com/hyry2024)
1. Reported `-e` env variables not expanding in flow config `appId` ([#12](https://github.com/devicelab-dev/maestro-runner/issues/12))

[@DouweBos](https://github.com/DouweBos)
1. Reported parallel device selection issues — non-iOS simulators selected and in-use devices not skipped ([#11](https://github.com/devicelab-dev/maestro-runner/issues/11))

[@janfreund](https://github.com/janfreund)
1. Reported scroll direction inversion with video evidence ([#9](https://github.com/devicelab-dev/maestro-runner/issues/9))

[@SuperRoach](https://github.com/SuperRoach)
1. Reported keyboard covering elements after input steps on Android
2. Reported `index` selector being ignored in simple selectors

## [1.0.6] - 2026-02-17

### Fixed
- iOS WDA: off-screen elements no longer returned by `findElement` — `assertVisible`, `tapOn`, `scrollUntilVisible`, and all element commands now correctly reject elements not visible in the viewport
- iOS WDA: `scrollUntilVisible` no longer skips scrolling when the target element exists in the accessibility tree but is off-screen
- iOS WDA: `scrollUntilVisible` direction matching is now case-insensitive (e.g., `direction: "DOWN"` works)
- iOS WDA: `waitForIdleTimeout` now works on iOS via WDA quiescence
- `when: platform` condition was ignored in `runFlow` blocks ([#8](https://github.com/devicelab-dev/maestro-runner/issues/8))

### Contributors

[@janfreund](https://github.com/janfreund)
1. Reported `scrollUntilVisible` and element visibility issues on iOS ([#9](https://github.com/devicelab-dev/maestro-runner/issues/9))

[@kavithamahesh](https://github.com/kavithamahesh)
1. Reported `when: platform` condition being ignored ([#8](https://github.com/devicelab-dev/maestro-runner/issues/8))

## [1.0.5] - 2026-02-16

### Added
- `tapOn: point` now supports absolute pixel coordinates (e.g., `point: "286, 819"`) in addition to percentages
- Coordinate validation: negative values, out-of-bounds pixels, and percentage range (0-100%) are all rejected with clear error messages
- Screen size cached at session startup instead of fetching on every tap/swipe/scroll
- `launchApp: environment` for passing environment variables via WDA `launchEnvironment`

### Changed
- Extracted shared helpers (`ParsePointCoords`, `ParsePercentageCoords`, `RandomString`, `SuccessResult`, etc.) from drivers into `pkg/core`
- Removed hardcoded 1080x1920 screen size fallback in UIAutomator2 scroll/swipe

### Fixed
- `launchApp: arguments` silently failed on real iOS devices — early return after session creation, unpopulated env map, activate vs launch, missing variable expansion
- Removed unused AI flags (`--analyze`, `--api-url`, `--api-key`)

### Contributors

[@mahesh-e27](https://github.com/mahesh-e27)
1. Reported `tapOn: point` not supporting absolute pixel coordinates ([#6](https://github.com/devicelab-dev/maestro-runner/issues/6))
2. Spotted unused AI flags (`--analyze`, `--api-url`, `--api-key`)

[@majdukovic](https://github.com/majdukovic)
1. Reported `launchApp: arguments` not working on real iOS devices ([#7](https://github.com/devicelab-dev/maestro-runner/issues/7))

## [1.0.4] - 2026-02-13

### Added
- `keyPress` option for character-by-character text input on Android
- Stale socket cleanup on force-stop (Ctrl+C / kill -9) with PID-based locking

### Fixed
- iOS Appium driver: element finding and tap reliability (use `label` instead of `content-desc` for accessibility)
- iOS Appium driver: `pressKey` command support
- iOS Appium driver: `tapOn` and `inputText` reliability improvements
- iOS Appium driver: skip `--app-file` and `--team-id` pre-checks (not needed for Appium)
- iOS Appium driver: skip `clearState` on real devices (`mobile: clearApp` only works on simulators)
- iOS WDA driver: auto-alert handling on simulators (accept/dismiss permission dialogs)
- `takeScreenshot` command now correctly saves PNG files
- GitHub star link in HTML report
- All `errcheck` violations fixed with proper error logging

### Contributors

[@SuperRoach](https://github.com/SuperRoach)
1. Suggested the `keyPress` feature for character-by-character input
2. Suggested the `--team-id` pre-check for WDA driver
3. Reported the `takeScreenshot` bug

[u/Healthy_Carpet_26](https://www.reddit.com/user/Healthy_Carpet_26/)
1. Reported the stale socket issue on force-stop (Ctrl+C)

[@kavithamahesh](https://github.com/kavithamahesh)
1. Reported iOS element finding issue — `label` instead of `content-desc` ([#3](https://github.com/devicelab-dev/maestro-runner/issues/3))
2. Reported `pressKey` not working for iOS on Saucelabs ([#4](https://github.com/devicelab-dev/maestro-runner/issues/4))

[@janfreund](https://github.com/janfreund)
1. Reported clearState and iOS permission dialog handling issues ([#2](https://github.com/devicelab-dev/maestro-runner/issues/2))

## [0.1.0] - 2026-01-27

### Added
- CLI with `validate` and `run` commands
- Configuration loading from `config.yaml`
- YAML flow parser with support for all Maestro commands
- Flow validator with dependency resolution
- Tag-based test filtering (include/exclude)
- UIAutomator2 driver with native element waiting
- Appium driver with per-flow sessions and capabilities file support
- WDA driver for iOS via WebDriverAgent
- JavaScript scripting engine (`evalScript`, `assertTrue`, `runScript`)
- Regex pattern matching for element selectors (`assertVisible`, `copyTextFrom`)
- Coordinate-based swipe and percentage-based tap support
- Nested relative selector support
- Step-level and command-level configurable timeouts
- Context-based timeout management
- Configurable `waitForIdleTimeout` for UIAutomator2
- `inputRandom` with DataType support
- JSON report output with real-time updates
- HTML report generator with sub-command expansion for `runFlow`, `repeat`, `retry`
- Clickable element prioritization for Appium

### Fixed
- JS `evalScript` and `assertTrue` parsing for Maestro `${...}` syntax
- Step counting accuracy in reports
- Appium driver regex matching
