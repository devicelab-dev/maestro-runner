# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.1.25] - 2026-08-19

This release is about **gestures that behave like a user's finger**: a real `dragAndDrop` on every driver — press, hold until the item lifts, move slowly, settle, release — and a `scrollUntilVisible` that stops when the element is actually visible instead of one pixel in. Flutter apps go from barely drivable to ahead of WDA on the DeviceLab iOS driver, `--step-delay` slows any flow down for demos and animation-heavy apps, and JUnit reports carry test-tracking properties and failure artifacts into CI.

### Added
- **`dragAndDrop`** — long-press an element (or point) and drag it to another, the way reorder UIs expect. `from:`/`to:` each take a selector or a `point:`; `holdDuration` (ms, default 1000) is the press before movement, `duration` (ms, default 1000) the movement itself. Works on every driver: UIAutomator2 (W3C actions with precise hold and paced moves), DeviceLab Android (`input draganddrop`, Android 12+ — hold length comes from the system long-press timeout), WDA (`press(forDuration:thenDragTo:)` — XCUITest paces the move itself), DeviceLab iOS (runner drag with a new backward-compatible `moveDurationMs` field), web (paced CDP mouse sequence), and Appium (W3C actions).
  ```yaml
  - dragAndDrop:
      from:
        id: "item-3"
      to:
        point: "50%, 20%"
      holdDuration: 800
  ```
  Heads-up for web: pages using native HTML5 `draggable` ignore synthetic mouse events by design — mouse/touch/pointer-event drag implementations work.
- **`--step-delay <ms>`** — pace a pause between top-level steps, for demos and for apps whose animations outrun the assertions. Also `MAESTRO_STEP_DELAY` in the environment, and per-flow override with `stepDelay:` in the flow config.
- **Flow `properties:` in JUnit reports** — a flow's `properties:` map (Maestro-compatible syntax) now lands as `<property>` entries on its JUnit testcase, next to the standard file and device properties — so flows can carry test-tracking ids into CI ([#84](https://github.com/devicelab-dev/maestro-runner/issues/84)).
  ```yaml
  properties:
    testID: Test-1234
  ```
- **Faster parallel runs** — the work queue is filled longest-flow-first using durations from the previous run, instead of file order. A run cannot finish before its longest remaining flow does, so one slow flow late in the alphabet used to set the wall clock for every worker. Flows with no recorded time are weighted as the median. Best-effort: no previous run, or an unreadable report, falls back to file order.
- **A mid-flow app death is explained rather than blamed on the selector (Android)** — when the app under test dies, the runner now asks the platform why instead of reporting a bare "element not found". That covers the two cases logcat cannot see: a low-memory kill writes nothing at all, and a system resource kill writes nothing useful. The DeviceLab driver had no crash detection whatsoever and is now level with UIAutomator2.
- **Failure artifacts in JUnit reports** — failed testcases attach the failing step's screenshot, and any `--record` video, via the `[[ATTACHMENT|path]]` convention Jenkins-style tooling reads; paths are relative to the report directory. A green run attaches only the video.
- **Negation globs in `config.yaml`** — a `flows:` pattern prefixed with `!` subtracts the files it would otherwise have selected, so a workspace can take everything and carve out the fixtures. Exclusions reuse the inclusion matcher, so `!a/**` excludes exactly what `a/**` would have included.
  ```yaml
  flows:
    - "**"
    - "!fixtures/**"
  ```
- **DeviceLab iOS targets the app that's actually on screen** — a command with no `appId` used to activate the runner's own placeholder host app, hijacking whatever you were looking at with a blank screen, because XCUITest's public API can't address "the app in front". It now resolves the frontmost application through the active-application PIDs and targets it with no activation at all, falling back to the host app (and logging which guard missed) when any step is unavailable.
- **The DeviceLab iOS runner source ships inside the Go module** — library consumers previously built the runner from whatever was in `~/.maestro-runner/drivers/ios`, a hidden runtime dependency and a protocol-skew hazard where a pinned Go client could drive runner sources from a different version. The runner a consumer runs is now version-locked to the module it compiled against, with no maestro-runner installation needed. The CLI keeps using the installed directory — identical content, shared build cache.

### Fixed
- **`scrollUntilVisible` stopped on elements it shouldn't have** — every driver accepted any 1px viewport overlap (or mere presence) as "visible", and the documented `visibilityPercentage` knob was parsed but wired to nothing. The stop criterion is now a real visibility check on every driver: fully visible by default, honoring `visibilityPercentage` when set. Heads-up: flows that relied on a sliver of the element counting as visible may scroll one step further now. On DeviceLab iOS, frames that arrive pre-clipped to the viewport (Flutter semantics especially) additionally need to hold still across one extra scroll before being accepted, so a 12pt sliver of an 80pt row can't masquerade as fully visible.
- **Flutter apps were undrivable on the DeviceLab iOS driver** — four stacked causes, each fixed: the Flutter VM fallback was wired only into the WDA path; the driver implemented neither the coordinate-tap step the fallback delivers taps through nor `tapOn: point:`; the fallback's short find windows leaked into the runner's HTTP timeout and cut XCUITest calls off mid-flight; and scroll gestures released mid-motion, so iOS spent the next tap cancelling residual deceleration instead of activating anything — a silent no-op. Scrolls now hold still 250ms before lifting, the same dead-stop lesson the Android agent swipes learned. On the Flutter issue-repro suite the driver goes from 9/19 to 17/19 — ahead of WDA-with-fallback at 14/19; the two remaining failures reproduce open Flutter framework bugs and fail on every driver.

- **`point:` was silently ignored on `doubleTapOn` and `longPressOn`** by the Appium and web drivers, which tapped the element centre instead. Every driver now honours it, so a knob that worked on four drivers and was quietly dropped by two behaves the same everywhere. Fixing the web path surfaced a second dead option: web long press hardcoded a one-second hold and dropped `duration:` entirely.
- **`assertScreenshot` compared whatever frame arrived** — capturing mid-animation seeds a baseline nothing will match again, and the failure that follows looks exactly like a real visual regression. It now re-captures until two consecutive frames agree, the same settle the drivers already used for `waitForAnimationToEnd`. A screen that never settles (a spinner, a video, a caret) falls through to the last frame rather than hanging.
- **DeviceLab iOS rejected clipped frames by scrolling again** — iOS clamps a descendant's frame to its scroll container's visible bounds, so a sliver of a row far past the fold reads as fully visible and the tap that follows does nothing. Rather than spending an extra scroll to see whether a suspicious rect moves, the driver now checks the parent chain: a descendant claiming to be on screen beneath an ancestor that is wholly off it is a contradiction, found in one pass.
- **Web flow-header variables reached the browser unexpanded** — a flow declaring `url: ${BASE_URL}` sent the literal `${BASE_URL}` to Chromium, which rejected it as an invalid URL, so `--platform web` could not be driven from the environment at all. The header is now expanded through the same script engine, in the same precedence order, that expands steps — so `-e`, `--env-file` and workspace config all reach the initial navigation ([#145](https://github.com/devicelab-dev/maestro-runner/issues/145)). The header is now expanded once centrally, so Android and iOS stop silently dropping the app version from reports when a flow uses `appId: ${VAR}`, and an unset variable is named at startup instead of surfacing later as a bare "no URL specified".
- **A dead DeviceLab iOS runner left no trail** — mid-session runner deaths surfaced only as `connection refused`: `runner.log` was truncated on every start, the runner exited on the first listener failure, and nothing distinguished a requested shutdown from a spontaneous one. The listener now rebinds on failure (5 retries, 1s backoff) rather than exiting, since simulator network daemons crash-loop on some Xcode/runtime combinations; every deliberate exit logs its reason; `runner.log` rotates through three generations so the log explaining a death survives the burst of failed restarts that follows it; and an `xcodebuild` that exits mid-session without a Stop being requested now says so on stderr.

### Contributors

[@humuhimi](https://github.com/humuhimi)
1. Reported and fixed web flow-header variables not being expanded before browser launch ([#145](https://github.com/devicelab-dev/maestro-runner/issues/145), [#146](https://github.com/devicelab-dev/maestro-runner/pull/146))

[@zcsteele](https://github.com/zcsteele)
1. Requested custom properties in JUnit report files ([#84](https://github.com/devicelab-dev/maestro-runner/issues/84))

## [1.1.24] - 2026-08-16

This release is about **the loop after a failed run**: re-run only what failed with `--retry-failed`, watch what happened with `--record`, and assert list contents exactly with `assertVisible: count:`. Dark mode now works everywhere — web pages and physical iOS devices included — and reports identify the exact CI build they ran against.

### Added
- **`--retry-failed`** — re-run only the flows that failed in the previous run. Reads the last report under the same `--output` directory (flattened or timestamped layout); a run cut short counts its unfinished flows as failed, so nothing is silently dropped. A clean previous run exits 0 without touching a device; if none of the previous failures are in the current selection, the run errors instead of silently running nothing.
  ```bash
  maestro-runner test --output ./reports flows/                 # 2 of 40 fail
  maestro-runner test --output ./reports --retry-failed flows/  # runs just those 2
  ```
- **`--record`** — save a screen recording of every flow into its report assets, played inline in the HTML report. Android devices/emulators record on-device via `screenrecord` (3-minute clip cap per flow) and the file is pulled to the host; iOS simulators record host-side via `simctl`. Unsupported platforms (physical iOS, web, Appium) warn once and run without recording.
- **`assertVisible` with `count:`** — assert that a selector matches exactly N visible elements; fewer or more both fail, and the error reports the count actually observed. Counts through the same multi-match machinery `index:` selection uses on every driver, so count semantics never diverge from single-element matching. Supports `${VAR}`; rejects `0` (use `assertNotVisible`) and combining with `index:`.
  ```yaml
  - assertVisible:
      css: .cart-item
      count: 3
  ```
- **Dark mode on web** — `setDarkMode`/`toggleDarkMode`/`assertDarkMode`/`assertLightMode` now work on the browser driver by emulating `prefers-color-scheme` over CDP. Heads-up: headless Chromium defaults to dark, so set an explicit mode before asserting.
- **Dark mode on physical iOS devices** — `--driver devicelab_ios` now sets appearance through `XCUIDevice.appearance` (iOS 15+), which works on real hardware, replacing the simulator-only `simctl` path. The set is read back and the step fails if it didn't take effect.
- **App build number in reports** — reports now show the build behind the version (Android `versionCode`, iOS `CFBundleVersion`): `v1.16.0 (10009107)` in `report.json`, the HTML header and title, and Allure's environment. The devicelab_ios and local-Appium paths report the app version too, where they previously reported none ([#144](https://github.com/devicelab-dev/maestro-runner/issues/144)).

### Fixed
- **Flow text reaching the device shell is now shell-quoted** — an apostrophe in a URL (`openBrowser: "https://example.com/s?q=it's"`) was a device-side shell parse error, and `addMedia` paths carried no quoting at all, so a filename with a space silently registered nothing. Applied at all six sites across the Android drivers (`openBrowser`, `openLink`, `launchApp` arguments, `addMedia`).
- **WDA `inputText` rejected React Native fields that are accessibility-merged** — the 1.1.23 focus gate required an active element, but iOS collapses a `TextInput` inside an accessible container and publishes only the parent, so nothing reports keyboard focus even though typing works. The gate now also accepts a visible keyboard as evidence (iOS doesn't raise it unless something holds first responder), a failed element send-keys falls through to tap-and-type, and the failure message now says exactly what was observed ([#143](https://github.com/devicelab-dev/maestro-runner/issues/143)).
- **iOS text-entry verification now polls for the keystrokes to commit** — the type command can return before the app has applied the keys, so the 1.1.23 one-shot re-read could land on a field that had taken nothing yet and report misdirection that never happened.
- **`startRecording` never survived on real Android devices** — a backgrounded `screenrecord` inheriting the adb shell's stdio dies with the session, so recordings silently skipped. Both the new `--record` flag and the standalone `startRecording` step now detach correctly and verify the recorder is actually running.
- **Web runs were reported as driver `uiautomator2`** in reports — the `--driver` flag's Android default was leaking through; web reports now say `cdp`.

### Contributors

[@georgetarazi-swipejobs](https://github.com/georgetarazi-swipejobs)
1. Reported the WDA `inputText` focus-gate regression on accessibility-merged React Native fields ([#143](https://github.com/devicelab-dev/maestro-runner/issues/143))

[@whalemare](https://github.com/whalemare)
1. Requested the app build number alongside the version in reports ([#144](https://github.com/devicelab-dev/maestro-runner/issues/144))

## [1.1.23] - 2026-08-11

This release is about **silent failures** — steps that reported success while doing nothing, or while acting on the wrong element. Four separate commands were quietly discarding a parameter, taps could land on the soft keyboard and report success, and text could be typed into a field the flow never named. Alongside those: dark-mode control, a device-free `lint` command, and per-step latency in the report.

### Added
- **Dark mode control** — `setDarkMode`, `toggleDarkMode`, `assertDarkMode` and `assertLightMode`. Dark-mode bugs are visual and pair naturally with `assertScreenshot`, but there was no way to put a device into a known appearance or assert the one it is in. Android uses `cmd uimode night`; iOS simulators use `simctl ui appearance` on both the WDA and DeviceLab iOS drivers.
  ```yaml
  - setDarkMode: dark      # or: light, or {enabled: true}
  - assertDarkMode
  - toggleDarkMode
  - assertLightMode
  ```
  Physical iOS devices and web return an explicit error rather than a silent no-op — iOS exposes no appearance hook outside the simulator, and web dark mode is a different mechanism (CDP `prefers-color-scheme`).
- **`lint` subcommand** — parse flow files with the runner's own parser and report syntax errors without a device. Anything that would abort a run at startup is caught in milliseconds. Non-zero exit on failure, so it drops straight into a CI step.
  ```bash
  maestro-runner lint flows/
  ```
- **`hierarchy --screenshot <path>`** — capture a screenshot from the same session that produced the tree. Two separate invocations pay driver startup twice and can straddle a UI change, leaving a tree and an image that disagree about what was on screen.
- **Per-step latency in `report.json`** — each flow now carries `stepLatency` with `p50`/`p95`/`max`/`mean` and the slowest command type. A wall-clock total hides the shape of a slowdown; percentiles separate "one command became pathological" from "everything drifted", and CI can gate on them.
- **`point:` on `swipe: from:`** — choose where inside a scrollable the gesture starts. Independent of `distance:`, so a point re-aims a swipe without lengthening it.
  ```yaml
  - swipe:
      from: {id: editor-input, point: "20%, 50%"}
      direction: DOWN
      distance: 0.4
  ```

### Fixed
- **`assertScreenshot` reported "100.00% match is below threshold 100.00%"** — `%.2f` rounded 99.9967% up, so a genuine few-pixel difference read like a runner bug. The message now reports the differing/total pixel counts and widens precision until match and threshold no longer print identically: `99.997% match (threshold: 100.000%, 1 of 30000 pixels differ)`. No epsilon was added — `thresholdPercentage: 100` means zero differing pixels, matching Maestro, and exact matches already returned exactly 100 ([#138](https://github.com/devicelab-dev/maestro-runner/issues/138)).
- **`cropOn` crops changed size between runs** — origin and size were truncated independently, so with a driver that halves the screenshot an element of height 131 cropped to 65 at y=100 and 66 at y=101. The comparison then rejected the pair on size before looking at a pixel. Rounding origin and size separately makes crop dimensions depend only on the element's size. Stale `_diff.png` files are also removed on a size mismatch — previously the "check the diff image" hint pointed at an artifact from an earlier run that looked identical to the capture ([#138](https://github.com/devicelab-dev/maestro-runner/issues/138)).
- **Taps could land on the soft keyboard and report success** — the occlusion guard only ran when the *previous* step was an input step, so a keyboard raised by an `autoFocus` field or left up across navigation was never checked. DeviceLab also allowed a 50px margin below the reported keyboard top, but the IME consumes touches across its whole touchable region — the suggestion strip included. Measured on a Pixel 4a: the region starts at y=1428, a tap at y=1439 was swallowed while one at y=1414 focused the field. uiautomator2 never had that margin, which is why the failure was DeviceLab-only. Such taps now fail with the actionable `hideKeyboard` hint ([#139](https://github.com/devicelab-dev/maestro-runner/issues/139)).
- **`point:` was silently ignored on `doubleTapOn` and `longPressOn`** — the parser filled it, then every driver tapped the element's centre and discarded it. It matters most on a text editor: the centre is often blank space past the end of the content, and double-tapping blank space selects no word and opens no context menu. Fixed on all four drivers — Android verified on RNTester (a field whose text ends before the centre failed 3/3 and passes 3/3 aimed at `point: "10%, 50%"`), iOS verified on a simulator ([#140](https://github.com/devicelab-dev/maestro-runner/issues/140)).
- **`distance:` was ignored on `swipe: from:`** — honoured only for screen swipes, while the element branch always travelled the anchor's own size. Scrolling from a small anchor moved almost nothing: a 77px input produced a ~76px drag and no scroll at all ([#141](https://github.com/devicelab-dev/maestro-runner/issues/141)).
- **DeviceLab swipes scrolled a different distance on every run** — `adb shell input swipe` always lifts the pointer at speed, so the view flings, and fling momentum comes from event timings that shift with machine load. Measured spread over identical runs: 114px at the 300ms default, 22px at `duration: 1200`, still 14px at 6000ms. Swipes now go through the on-device agent, which spends the touch slop up front and holds the pointer still before lifting. Measured over 4 runs: 1053/991/1029/1055 via adb (64px spread) against 870 every time via the agent. ADB remains the fallback when the agent is unreachable ([#141](https://github.com/devicelab-dev/maestro-runner/issues/141)).
- **iOS `inputText` could type into the wrong element and report success** — the drivers tapped to focus and typed immediately with nothing checking the text arrived, the iOS shape of the keyPress misdirection above. Verifying focus up front is impossible — the iOS runner never populates `focused` in its snapshot (confirmed on a simulator). devicelab_ios now re-reads the target and checks its value moved; WDA now fails when nothing ever takes keyboard focus instead of sending keys anyway. Both are best-effort: with no target to re-read, verification stays silent rather than inventing a failure.
- **`copyTextFrom` returned empty for DeviceLab cached elements** — `Element.Attribute()` dereferenced a nil HTTP client for elements resolved from a hierarchy snapshot, which could crash the runner, and there was no fallback to the accessibility label the snapshot already carries.
- **CI had been red since 2026-07-21** — two `ineffassign` findings failed every lint run, gating Build and Release the whole time. golangci-lint is now pinned rather than tracking `latest`, which would have turned the build red on 61 pre-existing findings the moment it rolled to v2.

### Changed
- **Taps on keyboard-covered elements now fail** instead of landing on the keyboard. This is the intended fix, but it can surface new failures in flows that were quietly tapping the wrong thing — add `- hideKeyboard` or scroll the field into view.
- **DeviceLab swipes scroll further** than before, because less travel is lost to touch slop and the pacing is accurate. Re-record screenshot baselines taken after a swipe.
- **The bundled Android agent APK changed.** A runner-only update will not deliver the deterministic-swipe fix.

### Contributors

[@kacperzolkiewski](https://github.com/kacperzolkiewski)
1. `assertScreenshot` reporting "100.00% match is below threshold 100.00%", and `cropOn` crops differing by a pixel between runs ([#138](https://github.com/devicelab-dev/maestro-runner/issues/138))
2. `inputText` with `keyPress: true` entering partial or wrong text on Android ([#139](https://github.com/devicelab-dev/maestro-runner/issues/139))
3. `doubleTapOn` not opening the text-selection context menu ([#140](https://github.com/devicelab-dev/maestro-runner/issues/140))
4. `swipe` not scrolling reliably in a React Native input, especially on CI ([#141](https://github.com/devicelab-dev/maestro-runner/issues/141))

## [1.1.22] - 2026-07-31

The headline is the community-contributed **`hierarchy` subcommand** — dump the on-device view hierarchy, normalized to one JSON tree across every driver — and **`addMedia` working on all platforms**, including real iOS devices via on-device PhotoKit (a capability beyond stock Maestro). Alongside them: a `swipe` distance parameter, deeper iOS snapshots for React Native trees, and a batch of correctness fixes — escaped-metacharacter `text:` selectors, `${VAR}` expansion in device-control steps, `#`/Shift typing on DeviceLab Android, DeviceLab iOS `clearState`, and transient WebView/CDP retries.

### Added
- **`hierarchy` subcommand** — dump the current on-device view hierarchy for selector discovery and debugging. Drivers return platform-specific formats (Android UIAutomator XML, iOS WDA XML, DeviceLab flat-JSON); the command normalizes all of them to one consistent JSON tree, so output is stable and diffable across drivers. `--compact` prints a flat, greppable one-line-per-element listing; `--find <substr>` filters to elements whose type/id/text match; element states surface as `[disabled]`/`[checked]`/`[selected]`/`[focused]`. Stdout carries only the hierarchy (driver setup goes to stderr), so it pipes cleanly into `jq` or a file.
  ```bash
  maestro-runner --device <id> hierarchy --compact --find login
  ```
  Contributed by [@zcsteele](https://github.com/zcsteele) ([#134](https://github.com/devicelab-dev/maestro-runner/pull/134)).
- **`addMedia` on every platform** — inject photos/videos into the device gallery before a flow. Reimplemented per platform, each device-validated: Android DeviceLab (on-device agent MediaStore insert, `IS_PENDING` scoped-storage flow), Android UIAutomator2 (adb push + `content scan_file` registration), iOS Simulator (`xcrun simctl addmedia`), and **iOS real device** (on-device PhotoKit `PHAssetCreationRequest`). Real-device iOS is not available in stock Maestro. The previous implementation fired a deprecated `MEDIA_SCANNER_SCAN_FILE` broadcast on a host path and reported success without adding anything.
  ```yaml
  - addMedia:
      - assets/photo.png
      - assets/clip.mp4
  ```
- **`swipe` distance parameter** — a direction swipe accepts `distance:` for a centered, fixed-length gesture instead of the default edge-to-edge travel. Mirrors Maestro #949.
  ```yaml
  - swipe:
      direction: UP
      distance: 0.4   # fraction of the screen (0-1), centered; default 0.5
  ```
- **Deeper iOS snapshots** — override XCTest's clipped snapshot request params (`maxDepth`/`maxChildren`, `snapshotKeyHonorModalViews=0`) so deep React Native trees and modal-obscured content are captured, fixing missing elements on RN-heavy screens.

### Fixed
- **Android: a `text:` selector escaping only metacharacters never matched** — `looksLikeRegex` skipped any backslash-escaped character when classifying a pattern, so an escape-only regex like `example\.com` or `\$0.00` was treated as a literal string and the backslashes were matched verbatim — it could only hit an element whose text literally contained a backslash. Upstream Maestro treats `text:` as always-regex, where `\.` / `\$` is the normal way to match a literal `.` / `$`. An escaped metacharacter now classifies the whole pattern as a regex (`textMatches`), fixed across all four matcher paths (UIAutomator2, DeviceLab, Appium, WDA). Reported by [@nixit28](https://github.com/nixit28) ([#136](https://github.com/devicelab-dev/maestro-runner/issues/136)).
- **`${VAR}` was not expanded in several device-control steps** — variable expansion runs off a per-step allowlist, and `setOrientation`, `setLocation`, `setClipboard`, `swipe`, `scroll`, and `openBrowser` were missing from it, so `setOrientation: ${ORIENT}` reached the driver as the literal `${ORIENT}` and failed with `invalid orientation`. Those steps now expand their value fields (both shorthand and long form, top-level and inside `runFlow`). Reported by [@nixit28](https://github.com/nixit28) ([#137](https://github.com/devicelab-dev/maestro-runner/issues/137)).
- **`addMedia:` bare-sequence lists failed to parse** — a `addMedia:` given a plain YAML sequence of paths failed to unmarshal. Reported by [@kacperzolkiewski](https://github.com/kacperzolkiewski) ([#131](https://github.com/devicelab-dev/maestro-runner/issues/131)).
- **DeviceLab Android: `#` and other Shift characters were dropped when typing** — `inputText` synthesized key events without honoring the shifted layout, so characters requiring Shift (`#`, `$`, `@`, …) never landed. Typing now goes through `KeyCharacterMap`, turning each character into the correct key-event sequence. Reported by [@kacperzolkiewski](https://github.com/kacperzolkiewski) ([#132](https://github.com/devicelab-dev/maestro-runner/issues/132), also [#135](https://github.com/devicelab-dev/maestro-runner/issues/135)).
- **DeviceLab iOS: `clearState` was a silent no-op** — `clearState` (and `launchApp` with `clearState: true`) claimed success without resetting the app. It now stages the app container, uninstalls, and reinstalls on the simulator so state is genuinely cleared.
- **Web: transient CDP execution-context errors during element finding are retried** — a Chrome DevTools "execution context was destroyed" error thrown mid-navigation no longer fails the step; the finder retries once the context settles.
- **`${VAR}` in `assertScreenshot` threshold and permission values** — `thresholdPercentage` and `launchApp` / `setPermissions` permission entries now expand variables, and `assertScreenshot` baseline/diff paths that escape the workspace are rejected.

### Contributors

[@zcsteele](https://github.com/zcsteele)
1. Contributed the `hierarchy` subcommand ([#134](https://github.com/devicelab-dev/maestro-runner/pull/134))

[@kacperzolkiewski](https://github.com/kacperzolkiewski)
1. Reported the `addMedia` parse error ([#131](https://github.com/devicelab-dev/maestro-runner/issues/131))
2. Reported `#`/Shift characters being dropped on DeviceLab Android ([#132](https://github.com/devicelab-dev/maestro-runner/issues/132))

[@nixit28](https://github.com/nixit28)
1. Reported escaped-metacharacter `text:` selectors never matching ([#136](https://github.com/devicelab-dev/maestro-runner/issues/136))
2. Reported `${VAR}` not expanding in `setOrientation` ([#137](https://github.com/devicelab-dev/maestro-runner/issues/137))

## [1.1.21] - 2026-07-28

The headline is **`assertScreenshot`** — visual regression testing with a highlighted diff image — contributed by the community. Alongside it: two regressions from v1.1.20 fixed (the keyboard-blocking guard on both Android drivers), a correctness fix where iOS `id:` + `text:` silently degraded to an OR, clearer failures when an app crashes mid-flow, correct WDA ports for legacy iOS UDIDs in parallel runs, and step-level `platform:` conditions.

### Added
- **`assertScreenshot` — visual regression testing** — compare the screen (or a cropped element) against a reference PNG, failing when the match falls below a threshold. On mismatch it writes a `*__diff.png` with the changed regions boxed in red. The reference is created automatically on first run, and `--update-screenshots` re-baselines intentionally. Built entirely on the Go standard library — no new dependencies.
  ```yaml
  - assertScreenshot:
      path: screenshots/login
      thresholdPercentage: 95
      cropOn:
        id: login-form
  ```
  Contributed by [@kacperzolkiewski](https://github.com/kacperzolkiewski) ([#126](https://github.com/devicelab-dev/maestro-runner/pull/126)).
- **Step-level `platform:` conditions** — a step can be restricted to one platform and is skipped (not failed) elsewhere, so a single flow can carry platform-specific steps.
  ```yaml
  - tapOn:
      text: "Allow"
      platform: ios
  ```
- **App crash / termination is reported as such** — on Android, when the app-under-test dies mid-flow (crash, native SIGSEGV/SIGABRT, ANR, or kill), a following step now fails with `app '<pkg>' is no longer running (crashed or was terminated during the flow)` — with the crash cause pulled from logcat when available — instead of a misleading `element not found: context deadline exceeded`. UIAutomator2 driver.
- **Flutter web `id:` selectors** — `findByID` now also matches `flt-semantics-identifier`, so a Maestro `id:` targets Flutter web widgets (Flutter renders a widget's `Semantics` identifier as that attribute). Mirrors Maestro #3323.

### Fixed
- **iOS: `id:` + `text:` together silently degraded to an OR** — `assertVisible`/`tapOn` given both an `id` and `text` did not require them on the same element: the WDA finder returned as soon as it found an element with the given id (never checking text), and only if the id matched nothing did it fall back to a text-contains-anywhere query that ignored the id. So a wrong `text:` value passed green against the right id'd element (masking wrong displayed values), and a substring text could match a different element entirely. A combined `id` + `text` selector now requires both on one element (the DeviceLab iOS driver already did this; the bug was WDA-only). Reported by [@ahabshamaa](https://github.com/ahabshamaa) ([#130](https://github.com/devicelab-dev/maestro-runner/issues/130)).
- **Regression (v1.1.20): keyboard-blocking guard falsely rejected a tappable element** — the guard added in v1.1.20 sampled element and keyboard geometry once, immediately after an input step. Windows using `SOFT_INPUT_ADJUST_RESIZE` (e.g. a plain `AlertDialog` whose body scrolls) relayout a few frames after the IME appears: the target reports covered bounds on the first frame, then the window shrinks and it rises above the keyboard. The single-shot check read the stale first frame and failed a perfectly tappable element (e.g. submitting a dialog via `tapOn: android:id/button1` right after `inputText`). The check now re-samples every 50 ms for up to 2 s and fails only on a *persistent* overlap, returning immediately once the element clears — no latency on the happy path. Fixed on the DeviceLab driver by [@MarioRial22](https://github.com/MarioRial22) ([#127](https://github.com/devicelab-dev/maestro-runner/pull/127)) and extended to the default UIAutomator2 driver, with the settle loop shared in `pkg/core` so the two can't drift.
- **iOS: `id:` matched by substring, resolving to the wrong element** — a literal `id:` selector matched an accessibility id by substring, so `id: enriched-text` could resolve to `set-enriched-text-button` (a superset) purely by match order. Both iOS drivers now prefer an exact `id` match, falling back to substring only when no exact match exists — preserving lenient partial-id matching while fixing the ambiguous case. Surfaced through `assertScreenshot`'s `cropOn`. Reported by [@kacperzolkiewski](https://github.com/kacperzolkiewski) ([#128](https://github.com/devicelab-dev/maestro-runner/issues/128)).
- **iOS: legacy 40-character UDIDs all collided on WDA port 8100** — `PortFromUDID` parsed the whole segment after the last hyphen as a `uint64`. A legacy 40-character UDID has no hyphen, so all 40 hex chars were parsed, overflowed `uint64`, and fell back to port 8100 for *every* such device — so two of them in parallel both forwarded 8100 and the second failed with "address already in use". The port is now derived from the last 12 hex characters; a standard UUID's final group is already 12 chars, so UUID-derived ports are unchanged. Reported by [@eatbob](https://github.com/eatbob) ([#129](https://github.com/devicelab-dev/maestro-runner/issues/129)).
- **DeviceLab: a stalled WebView devtools socket slowed every command** — `ensureWebViewConnection` runs on every command while the WebView isn't connected, and a connect against a stalled/unreachable devtools endpoint spends its full timeout (~20 s). A single flaky WebView therefore added ~20 s to every step. A failed connect now backs off (5 s per socket) so it can't keep re-slowing commands; the command falls through to native finding meanwhile. Mirrors Maestro MA-4119.
- **WDA: recover from a transient `kAXErrorInvalidUIElement`** — a page-source snapshot taken while the accessibility tree is mutating can fail with a transient `kAXErrorInvalidUIElement` (-25202) that clears on the next attempt. `Source()` now retries once before surfacing it, so assertions and diagnostics don't fail on a momentary tree mutation. Mirrors Maestro #3430.

### Contributors
Thanks to everyone who shaped this release.

**Code contributions:**
- [@kacperzolkiewski](https://github.com/kacperzolkiewski) — `assertScreenshot` visual regression command ([#126](https://github.com/devicelab-dev/maestro-runner/pull/126)), and reported the iOS substring-id bug ([#128](https://github.com/devicelab-dev/maestro-runner/issues/128))
- [@MarioRial22](https://github.com/MarioRial22) — keyboard-blocking settle fix on the DeviceLab driver ([#127](https://github.com/devicelab-dev/maestro-runner/pull/127))

**Reported by:**
- [@ahabshamaa](https://github.com/ahabshamaa) — iOS `id:` + `text:` OR-instead-of-AND ([#130](https://github.com/devicelab-dev/maestro-runner/issues/130))
- [@eatbob](https://github.com/eatbob) — legacy 40-char UDID WDA port collision ([#129](https://github.com/devicelab-dev/maestro-runner/issues/129))

## [1.1.20] - 2026-07-16

A reporter-driven correctness release with two themes. First, **`swipe` with a `from:`/selector anchor now works consistently across every driver** — it targets the element's bounds and honours `duration:` on Android (uiautomator2, DeviceLab), iOS (WDA, DeviceLab), Appium, and the browser, so slider/drag-handle gestures land instead of registering as a fast flick. Second, a batch of environment and cloud fixes from real-world runs: iOS builds work on Intel Macs, the flow parser stops choking on arrow-comment headers and bare `scroll`, Android WebView form fields actually receive typed text, and `--parallel` Appium sessions survive on cloud device farms.

### Fixed
- **`swipe: from: <selector>` ignored the anchor element and `duration:`** — a direction swipe anchored on an element routed through a fixed-speed helper that neither derived coordinates from the element's bounds nor threaded `duration:`, producing a fast flick that native drag targets (sliders, drag handles) discard. Selector-anchored swipes now derive start/end from the element bounds and honour `duration:` across all drivers: uiautomator2 and DeviceLab (Android), WDA and DeviceLab (iOS), Appium, and the browser (CDP). The Android drivers share one `pkg/core` helper (with screen-edge clamping), the browser interpolates the drag over the requested duration so JS drag handlers track the pointer, and an invalid `direction:` now fails the step instead of silently swiping up. Reported and fixed for uiautomator2 by [@jsonITP](https://github.com/jsonITP) ([#114](https://github.com/devicelab-dev/maestro-runner/issues/114), [#115](https://github.com/devicelab-dev/maestro-runner/pull/115)).
- **`**` was only recursive at the start of a flow pattern** — `matchPattern` treated `**` as recursive only when the pattern was exactly `**` or began `**/`; anything else (`tests/**/*.yaml`, `auth/**`) fell through to `filepath.Glob`, which matches `**` like a single `*`, so each extra `**` matched exactly one more directory level and flows nested deeper were silently skipped. `**` is now recursive anywhere in a pattern (matching bash `globstar`, `doublestar`, `minimatch`), with a wildcard prefix (`flows-*/**/*.yaml`) expanded via glob and a missing prefix directory treated as a silent no-match. Reported and fixed by [@jsonITP](https://github.com/jsonITP) ([#116](https://github.com/devicelab-dev/maestro-runner/pull/116)).
- **iOS: every WDA build and start failed on Intel Macs** — the `xcodebuild -destination` specifier interpolated Go's `runtime.GOARCH` (`amd64`), but xcodebuild's vocabulary for Intel is `x86_64`, so no simulator ever matched and both the WDA build and start failed with "Unable to find a device matching the provided destination specifier". Apple Silicon was unaffected only because `arm64` is spelled the same in both vocabularies. The arch name is now translated (`amd64` → `x86_64`) on both the WDA and DeviceLab iOS paths; the explicit `arch=` pin stays (it prevents an Xcode 26 dual-destination stall). Reported by [@porluz](https://github.com/porluz) ([#117](https://github.com/devicelab-dev/maestro-runner/issues/117)).
- **iOS: a fast-failing WDA start was misreported as "stalled" and blindly retried** — the WDA/DeviceLab startup watcher only monitored the log file for growth, so a process that printed an error and exited looked identical to a hang: it reported "xcodebuild stalled (no log output for 1m0s)" 60s later and retried 4 times (~7 minutes) while the real one-line error sat in the log from the first second. Startup now watches the process itself, surfaces the actual error and log tail immediately on a fast exit, recognizes deterministic `xcodebuild: error:` failures and skips the retry loop, and includes a log tail in genuine stall messages. Reported by [@porluz](https://github.com/porluz) ([#118](https://github.com/devicelab-dev/maestro-runner/issues/118)).
- **Flow header comment ending in `->` failed to parse** — the YAML document splitter treated any line ending in `|` or `>` as the start of a block scalar, including comments, so a header comment like `# navigation: Library ->` flipped the splitter into multiline mode, swallowed the `---` separator, and failed with `cannot unmarshal !!map into []yaml.Node`. The block-scalar heuristic now fires only when the line's last token is a bare block indicator, so comments and plain values ending in `>` parse fine. Reported by [@porluz](https://github.com/porluz) ([#119](https://github.com/devicelab-dev/maestro-runner/issues/119)).
- **Bare `- scroll` (no arguments) was rejected on iOS** — Maestro's `scroll` with no direction scrolls down, and the Android drivers defaulted accordingly, but the WDA and DeviceLab iOS drivers hit their `default:` case and failed with "Invalid scroll direction". The default is now normalized to `down` at parse time, so every driver sees a concrete direction. Reported by [@porluz](https://github.com/porluz) ([#120](https://github.com/devicelab-dev/maestro-runner/issues/120)).
- **`--parallel` device-shortage hint listed Android AVDs for iOS runs** — when `--platform ios --parallel N` couldn't find enough devices, the remediation hint suggested Android AVDs and `emulator`/`avdmanager` commands. iOS runs now get simulator guidance instead: available shut-down simulators, the `--auto-start-emulator` variant of the command, and per-simulator `xcrun simctl boot` commands. Reported by [@porluz](https://github.com/porluz) ([#121](https://github.com/devicelab-dev/maestro-runner/issues/121)).
- **Android: `inputText` silently no-op'd into WebView form fields** — on the Android drivers, `inputText` with no selector sent blind key events to whatever the OS considered focused; for a WebView DOM input the keystrokes never reached the page, but the step still reported success, so forms submitted blank. All three Android drivers (Appium, uiautomator2, DeviceLab) now prefer element-scoped typing into the focused element — which routes through accessibility `ACTION_SET_TEXT` and reaches the DOM input — and fall back to key events only when nothing has focus. The Appium driver additionally honours an inline selector on `inputText`. Reported by [@devrchoi](https://github.com/devrchoi) ([#122](https://github.com/devicelab-dev/maestro-runner/issues/122)).
- **Appium `--parallel`: cloud farms reaped pre-created sessions before their flow ran** — in `--parallel N` all N sessions are created serially before any flow runs, so the first session idled ~(N−1)×creation_time before its first command; on cloud farms (e.g. SauceLabs RDC, ~35–40s/session) that idle exceeded the server's `newCommandTimeout` at N≥3–4 and the session was reaped, failing the first flow with "invalid session id". Each already-created session is now kept warm with a lightweight ping (a session-scoped GET that resets `newCommandTimeout`) on a 20s ticker until session creation finishes and execution begins. (maestro-runner forwards the caps file's `newCommandTimeout` verbatim; the effective ceiling is the farm's, so removing the pre-creation idle is the real fix.) Reported by [@devrchoi](https://github.com/devrchoi) ([#124](https://github.com/devicelab-dev/maestro-runner/issues/124)).

### Contributors
Thanks to everyone who shaped this release.

**Code contributions:**
- [@jsonITP](https://github.com/jsonITP) — selector-anchored swipe on uiautomator2 ([#115](https://github.com/devicelab-dev/maestro-runner/pull/115), reported in [#114](https://github.com/devicelab-dev/maestro-runner/issues/114)) and recursive `**` glob anywhere in flow patterns ([#116](https://github.com/devicelab-dev/maestro-runner/pull/116))

**Reported by:**
- [@porluz](https://github.com/porluz) — Intel-Mac iOS destination arch ([#117](https://github.com/devicelab-dev/maestro-runner/issues/117)), blind WDA stall retries ([#118](https://github.com/devicelab-dev/maestro-runner/issues/118)), arrow-comment header parsing ([#119](https://github.com/devicelab-dev/maestro-runner/issues/119)), bare `scroll` default ([#120](https://github.com/devicelab-dev/maestro-runner/issues/120)), iOS `--parallel` hints ([#121](https://github.com/devicelab-dev/maestro-runner/issues/121))
- [@devrchoi](https://github.com/devrchoi) — Android WebView `inputText` ([#122](https://github.com/devicelab-dev/maestro-runner/issues/122)), Appium `--parallel` session keepalive ([#124](https://github.com/devicelab-dev/maestro-runner/issues/124))

## [1.1.19] - 2026-07-01

A reporter-driven follow-up focused on executor/`runScript` parity with Maestro and iOS simulator ergonomics. Headlines: `runScript`'s JavaScript environment now matches Maestro (env vars are scoped per script and undeclared variables read as `undefined` instead of throwing), `when:`/`while:` condition checks resolve **fast by default** instead of blocking on the 7s optional-find timeout, and `--auto-start-emulator` finally works for iOS simulators. Plus an iOS runner build-cache correctness fix.

### Fixed
- **`runScript` env leaked across scripts; optional vars threw `ReferenceError`** — `runScript` env vars were applied as sticky globals that were never cleared, so a value set in one script bled into the next; and referencing an env var that wasn't provided this run threw `ReferenceError: X is not defined`. Env is now scoped to the single script run and restored afterward, and undeclared identifiers evaluate to `undefined` (matching Maestro's GraalJS behavior), so `someVar || default` and `typeof someVar` work for optional env vars. Real globals and the script's own declarations are untouched. Reported by [@rafaelnobrekz](https://github.com/rafaelnobrekz) ([#109](https://github.com/devicelab-dev/maestro-runner/issues/109)).
- **`--auto-start-emulator` errored for iOS simulators** — the iOS WDA pre-check decided simulator-vs-real-device before any simulator was started and didn't account for `--auto-start-emulator`, so `--platform ios --parallel 2 --auto-start-emulator` exited demanding `--team-id` before the (working) auto-start path could boot the simulators. `--auto-start-emulator` and `--start-simulator` now count as simulator targets on iOS (a real device can't be auto-created), so neither requires `--team-id` or `--app-file`. Reported by [@rafaelnobrekz](https://github.com/rafaelnobrekz) ([#111](https://github.com/devicelab-dev/maestro-runner/issues/111)).
- **DeviceLab iOS runner build cache could serve a stale build** — the cache keyed on the simulator iOS version alone, so a release shipping updated runner sources into the same `sim-ios<ver>` slot would reuse the previous build. The cache key now also includes a content hash of the vendored runner source, so it invalidates exactly when the runner changes and is reused otherwise.

### Changed
- **`when:`/`while:` condition checks are fast by default** — a `runFlow` `when:` (or `repeat` `while:`) condition with no explicit timeout fell through to the driver's 7s optional-find timeout, so every unmet condition blocked ~7s (a flow with several optional `when:` branches paid 7s each). These checks now use a short default timeout (1000ms); a present element still resolves immediately, only an unmet condition is bounded. Tunable globally with `--condition-timeout` (env `MAESTRO_CONDITION_TIMEOUT`) and still overridable per condition with `timeout:`. Reported by [@rafaelnobrekz](https://github.com/rafaelnobrekz) ([#110](https://github.com/devicelab-dev/maestro-runner/issues/110)).
- **Faster cold DeviceLab iOS runner builds** — the runner now builds against the concrete booted simulator (`-destination platform=iOS Simulator,id=<udid>`) instead of a generic destination, so `xcodebuild` skips re-planning an abstract device.

### Contributors
Thanks to everyone who reported issues that shaped this release:
- [@rafaelnobrekz](https://github.com/rafaelnobrekz) — `runScript` env scope + undeclared vars ([#109](https://github.com/devicelab-dev/maestro-runner/issues/109)), fast `when:`/`while:` condition checks ([#110](https://github.com/devicelab-dev/maestro-runner/issues/110)), iOS `--auto-start-emulator` ([#111](https://github.com/devicelab-dev/maestro-runner/issues/111))

## [1.1.18] - 2026-06-24

A reporter-driven correctness release. The headline theme is **tap and scroll geometry on Android** — taps and `scrollUntilVisible` no longer treat on-screen elements near the bottom edge (or with a momentarily inverted first-frame rect) as off-screen, so bottom-anchored buttons, tall-dialog actions, and below-the-fold list items resolve reliably. Alongside that: the **"Update available" banner now prints a working install URL**, iOS real-device permission dialogs arm correctly when `launchApp` lives in `onFlowStart`/`runFlow`, variable interpolation works in `repeat` `while:` conditions and `runScript` env, WDA failures surface the closest on-screen texts, and JUnit reports keep flow subdirectories in the `file` property.

### Fixed
- **Update banner printed a 404 install URL** — the "Update available" notice told users to run `curl ... https://open.devicelab.dev/maestro-runner/install`, but the path segments were swapped and that URL 404s. It now points at the working `https://open.devicelab.dev/install/maestro-runner`. Reported by [@George-Anton-Tarazi](https://github.com/George-Anton-Tarazi) ([#102](https://github.com/devicelab-dev/maestro-runner/issues/102)).
- **Android: taps rejected near the bottom edge of the display** — `boundsTappable` compared full-display element bounds against the on-device-reported *usable* height (e.g. `1080x2204` on a `2340`-tall screen), so `AlertDialog` buttons in tall dialogs and bottom-anchored FABs became untappable on the DeviceLab driver even though UIAutomator2 tapped them fine. Taps are now validated against the *physical* display size (`wm size`, cached), matching the coordinate space the accessibility hierarchy produces. Reported and fixed by [@MarioRial22](https://github.com/MarioRial22) ([#100](https://github.com/devicelab-dev/maestro-runner/issues/100), [#101](https://github.com/devicelab-dev/maestro-runner/issues/101)).
- **Android: `scrollUntilVisible` looped on elements already on screen** — `isElementOnScreen` used the usable height while element bounds are in full-display coords, so an item in the bottom system-bar band (e.g. the last nav-drawer item) was treated as off-screen and the scroll loop ran to its cap on a visible element. It now validates against the physical display, same as the tap guard.
- **Android: `scrollUntilVisible` short-circuited on a malformed rect** — `isElementOnScreen` rejected only zero-area bounds, not negative ones, so a clipped below-the-fold child reported with `top > bottom` (a negative-height rect) was accepted as "on screen" and the scroll never happened. Both the DeviceLab and UIAutomator2 drivers now treat non-positive width/height as off-screen, mirroring the tap-side guard.
- **Android: keyboard not dismissed when Appium `hide_keyboard` no-ops** — Appium's `/appium/device/hide_keyboard` returns success without closing the keyboard on some devices (notably several Samsung models), so the next coordinate tap hit the keyboard overlay. `hideKeyboard` now verifies via `dumpsys` and, only while the keyboard is confirmed still up, falls back to `KEYCODE_BACK` (gated so it can't trigger a stray back-navigation). Reported by [@satishs22](https://github.com/satishs22) ([#42](https://github.com/devicelab-dev/maestro-runner/issues/42)).
- **iOS real device: permission dialogs not armed from `onFlowStart`/`runFlow` `launchApp`** — `PrepareForFlow` scanned only the main flow body, so a `launchApp` with permissions declared in `onFlowStart` or a `runFlow` subflow never armed `defaultAlertAction` on a real device; system permission dialogs weren't auto-accepted and could wedge the device (diverging from the simulator). It now scans a flattened, execution-ordered view (`onFlowStart` + body, with `runFlow` subflows expanded and cycle-guarded), and warns when a real-device launch declares unsupported mixed permissions. Reported by [@seanadkinson](https://github.com/seanadkinson) ([#108](https://github.com/devicelab-dev/maestro-runner/issues/108)).
- **Variable interpolation in `repeat` `while:` conditions and `runScript` env** — `${output.*}`/`${VAR}` in a `repeat` `while:` selector are now expanded each iteration, and `runScript` env values are expanded before the script runs (matching `defineVariables`/`runFlow` and vanilla Maestro). `repeat` `times` and retry `maxRetries` now reject non-numeric values instead of silently defaulting to zero. Reported by [@pk1m](https://github.com/pk1m) ([#97](https://github.com/devicelab-dev/maestro-runner/issues/97)) and [@rafaelnobrekz](https://github.com/rafaelnobrekz) ([#107](https://github.com/devicelab-dev/maestro-runner/issues/107)).
- **WDA: surface closest on-screen texts on a missing text selector** — when a text selector isn't found, the page-source path now appends the closest on-screen texts (Dice-bigram ranked, quoted so PUA/invisible glyphs are visible) instead of masking the page-source error behind the WDA predicate error. Text matching also NFC-normalizes both sides so composed/decomposed accents match. Reported by [@HugoGresse](https://github.com/HugoGresse) ([#89](https://github.com/devicelab-dev/maestro-runner/issues/89)).
- **JUnit report flattened flow subdirectories** — the `file` property used `filepath.Base(SourceFile)`, collapsing e.g. `authentication/flow.yaml` to `flow.yaml` and breaking CI tools that locate the source by path. It is now reported relative to the working directory, preserving subdirectories. Reported by [@ceopaludetto](https://github.com/ceopaludetto) ([#96](https://github.com/devicelab-dev/maestro-runner/issues/96)).

### Changed
- **CI hardening** — migrated the deprecated `nhooyr.io/websocket` to `github.com/coder/websocket` (drop-in), cleared `golangci-lint` findings (`errcheck`, `gosimple`, `staticcheck`), and stabilized the headless Chromium browser tests on GitHub runners (disable the setuid sandbox under CI). No user-facing behavior change.

### Contributors
Thanks to everyone who shaped this release.

**Code contributions:**
- [@MarioRial22](https://github.com/MarioRial22) — Android tap validation against the physical display ([#101](https://github.com/devicelab-dev/maestro-runner/pull/101), reported in [#100](https://github.com/devicelab-dev/maestro-runner/issues/100)), `scrollUntilVisible` malformed-rect rejection ([#103](https://github.com/devicelab-dev/maestro-runner/pull/103)), and CI lint + browser-test stabilization ([#104](https://github.com/devicelab-dev/maestro-runner/pull/104))

**Reported by:**
- [@George-Anton-Tarazi](https://github.com/George-Anton-Tarazi) — broken update banner install URL ([#102](https://github.com/devicelab-dev/maestro-runner/issues/102))
- [@seanadkinson](https://github.com/seanadkinson) — iOS real-device permission dialogs from `onFlowStart`/`runFlow` ([#108](https://github.com/devicelab-dev/maestro-runner/issues/108))
- [@satishs22](https://github.com/satishs22) — Android keyboard dismissal ([#42](https://github.com/devicelab-dev/maestro-runner/issues/42))
- [@HugoGresse](https://github.com/HugoGresse) — WDA closest on-screen texts ([#89](https://github.com/devicelab-dev/maestro-runner/issues/89))
- [@pk1m](https://github.com/pk1m) — `repeat` `while:` interpolation ([#97](https://github.com/devicelab-dev/maestro-runner/issues/97))
- [@rafaelnobrekz](https://github.com/rafaelnobrekz) — `runScript` env interpolation ([#107](https://github.com/devicelab-dev/maestro-runner/issues/107))
- [@ceopaludetto](https://github.com/ceopaludetto) — JUnit subdirectory paths ([#96](https://github.com/devicelab-dev/maestro-runner/issues/96))

## [1.1.17] - 2026-06-12

A reporter-driven reliability release centred on Android tap/find correctness, plus a new Appium session-export hook. Headlines: elements living in a **separate window** — `AlertDialog`s, runtime-permission prompts, drawers, and Material dropdown/spinner popups — are now found instead of reported missing; taps can no longer fire **off-screen** from a malformed first-frame rect; the Android lazy tap-retry is **disabled by default** (it could re-tap across a navigation boundary onto the next screen); and live Appium sessions can be published to a well-known file for external tools.

### Added
- **`--appium-session-file <path>`** (env `MAESTRO_APPIUM_SESSION_FILE`) — publishes the live Appium session(s) (`sessionId` + `appiumUrl` per device) to a single JSON file so external tools can attach without polling report artifacts. Off by default. One entry per device (parallel runs share one file, no clobbering), new-session-per-flow updates in place, and the file is rewritten atomically (temp + rename) so readers never see a partial file. Requested by [@ssharma007-dev](https://github.com/ssharma007-dev) ([#91](https://github.com/devicelab-dev/maestro-runner/issues/91)).
  ```bash
  maestro-runner --driver appium --appium-session-file /tmp/sessions.json test flows/
  ```

### Fixed
- **DeviceLab Android: find elements inside dialogs / permission prompts / drawers** — the on-device agent searched only the focused window, so a control rendered in a separate accessibility window (e.g. an `AlertDialog`'s **OK**/**Discard** button, a runtime-permission prompt, or a drawer) was reported "not found" even though it was on screen. The agent now searches every window (topmost first) when the focused window misses. Bundled agent APK refreshed. Reported by [@simon-kuzin](https://github.com/simon-kuzin) ([#90](https://github.com/devicelab-dev/maestro-runner/issues/90)).
- **uiautomator2: dropdown / spinner popup items not in the hierarchy** — the default driver only exposed the focused window, so items in a Material `ExposedDropdownMenu`, a `Spinner`, or any `ListPopupWindow` (and `AlertDialog`s / permission prompts) were invisible and `tapOn` failed with "no such element" even with the popup on screen. maestro-runner now enables the server's `enableMultiWindows` setting, matching stock Maestro's all-windows traversal. Reported by [@ConorGarry](https://github.com/ConorGarry) ([#93](https://github.com/devicelab-dev/maestro-runner/issues/93)).
- **DeviceLab Android: off-screen tap from a malformed first-frame rect** — `FindAndClick` took its tap point from whatever rect the find returned; a just-opened bottom sheet's first laid-out frame could yield a clipped rect (top > bottom, negative height) or one translated below the viewport, so the tap fired off-screen and was lost, desyncing the flow. The tap path now rejects a non-positive-width/height or off-screen-centre rect and keeps polling for a settled frame (mirroring the assert-side viewport check). Reported by [@laiskajoonas](https://github.com/laiskajoonas) ([#94](https://github.com/devicelab-dev/maestro-runner/issues/94)).

### Changed
- **DeviceLab Android: lazy tap-retry disabled by default** — the lazy retry re-issued a tap when "the tree hash was unchanged since the tap and the target was still findable", treating that as "the tap had no effect". That predicate cannot distinguish a dropped tap from a successful one whose effect is async (submit-then-navigate) or that merely disables the source button, so it could re-issue a tap across a navigation boundary and land on the next screen's CTA. It is now off by default; opt back in with `MAESTRO_DEVICELAB_LAZY_RETRY=1`. Reported by [@laiskajoonas](https://github.com/laiskajoonas) ([#95](https://github.com/devicelab-dev/maestro-runner/issues/95)).

## [1.1.16] - 2026-05-31

Another reporter-driven reliability + parity release, with a notable new capability: an **experimental native iOS DeviceLab driver**. Headlines: `takeScreenshot` gains Maestro-compatible `cropOn` cropping across every driver, a new `--artifacts` flag controls when screenshots/hierarchy are captured, `setLocation` now works on iOS simulators, Android DeviceLab tap reliability on React Native navigation jumped from ~20/38 to 37/38 on the React Navigation example suite, and the iOS startup path is far more resilient under CI load.

### Added
- **`takeScreenshot` `cropOn` selector (all drivers)** — pass a selector under `cropOn` to crop the screenshot to the matched element's bounds instead of the whole screen. Same YAML shape as Maestro, so existing flows are portable. Element bounds are scaled to the captured image resolution (e.g. the DeviceLab Android agent downscales frames) before cropping, and the input image format is preserved. Reported by [@TheUltDev](https://github.com/TheUltDev) ([#88](https://github.com/devicelab-dev/maestro-runner/issues/88)).
  ```yaml
  - takeScreenshot:
      path: "login-button"
      cropOn:
        id: "login-button"
  ```
- **`--artifacts {always|on-failure|never}` flag** — controls when per-step screenshots and the UI hierarchy are captured. `on-failure` (default) keeps the previous behaviour; `always` captures before/after every step for visual debugging; `never` disables capture for the fastest, smallest reports.
- **iOS DeviceLab driver (experimental)** — a native XCUITest-based iOS driver, invoked with `--driver devicelab --platform ios`. The runner is vendored as source and built with Xcode on first run (cached per iOS version/device type afterwards), mirroring WDA. Passes the TestHive auth suite; still maturing versus WDA on complex React Native navigation, so **WDA remains the default and recommended iOS driver**.
- **`setLocation` on iOS simulators** — routes through `xcrun simctl location <udid> set <lat>,<lon>` (the same mechanism Maestro uses), on both the WDA and DeviceLab iOS drivers. Real iOS devices return an explicit "unsupported" error (Apple exposes no public GPS-override API for physical devices). Reported by [@HugoGresse](https://github.com/HugoGresse) ([#82](https://github.com/devicelab-dev/maestro-runner/issues/82)).
- **`--android-tcp-forward` flag** — forces TCP-to-TCP `adb forward` for the Android drivers, for sandboxed environments that block `localfilesystem:`/`localabstract:` forwards. Auto-enabled when `$DEVICEFARM_DEVICE_UDID` is present, fixing "server not ready" failures on AWS Device Farm. Reported by [@pk1m](https://github.com/pk1m) ([#83](https://github.com/devicelab-dev/maestro-runner/issues/83)).

### Changed
- **Android DeviceLab tap reliability on React Native navigation** — pre-tap settle is now applied to *all* tap selectors (ID-based taps used to bypass the settle path and could fire at mid-animation/off-screen bounds), plus a lazy-retry on `assertVisible`/`inputText` that re-issues a tap when the prior tap clearly had no effect. Took the React Navigation example E2E suite from ~20/38 to a steady 37/38.
- **Lazy-retry gated on tree-hash unchanged** — the lazy-retry now skips when the screen changed since the tap (e.g. a failed-login error rendering), eliminating a wasted retry window on flows whose tapped control legitimately persists. Cuts ~2s off negative-path flows (TestHive Invalid Password 9.5s → 7.4s) with no loss of the navigation reliability gains (37/38 unchanged).
- **iOS startup resilience under CI** — startup timeout raised to 600s and the simulator is now shutdown/booted between retry attempts to clear a wedged CoreSimulator daemon; both WDA and the iOS DeviceLab driver gained stall-detection that auto-retries a hung `xcodebuild` instead of waiting out the full timeout.
- **Appium honours user-set `appium:autoLaunch`** — the driver only forces `autoLaunch=false` when the caller hasn't specified it, so launch-time capabilities like `appium:processArguments` (e.g. `DYLD_INSERT_LIBRARIES` for Applitools NML) take effect again. Reported by [@kavithamahesh](https://github.com/kavithamahesh) ([#86](https://github.com/devicelab-dev/maestro-runner/issues/86)).
- **iOS alert handling default** — `alertAction` now defaults to empty (was implicitly "accept"); flows that don't configure permissions keep in-app alerts interactable, while explicit permission config still auto-accepts. Reported by [@j-ezeh](https://github.com/j-ezeh) ([#64](https://github.com/devicelab-dev/maestro-runner/issues/64)).
- **Appium driver is friendlier to locked-down hosts** (e.g. Sauce Labs) where local filesystem/port access is restricted.

### Fixed
- **Android file-picker taps (API 31+)** — the bundled DeviceLab agent was refreshed so the brief DOWN→UP touch duration is applied on all API levels, restoring the non-zero touch needed to dispatch open-document intents from `RecyclerView` file-picker items on Android 12+. Reported by [@LandonPatmore](https://github.com/LandonPatmore) ([#87](https://github.com/devicelab-dev/maestro-runner/issues/87)).
- **Android DeviceLab `displayed=false` filtering** — elements reported as not-visible-to-user are now skipped to match Maestro's pass-through behaviour (fixes false "element exists but is not visible" and material-top-tabs cases).
- **Case-insensitive regex selectors over-escaped** — `text: '.*For You.*'` was being treated as a literal string; regex metacharacters are no longer escaped in the `textMatches`/`descriptionMatches` fallback.

### Contributors
Thanks to everyone who reported issues that shaped this release:
- [@TheUltDev](https://github.com/TheUltDev) — `takeScreenshot` `cropOn` ([#88](https://github.com/devicelab-dev/maestro-runner/issues/88))
- [@HugoGresse](https://github.com/HugoGresse) — iOS `setLocation` ([#82](https://github.com/devicelab-dev/maestro-runner/issues/82))
- [@LandonPatmore](https://github.com/LandonPatmore) — Android file-picker taps ([#87](https://github.com/devicelab-dev/maestro-runner/issues/87))
- [@kavithamahesh](https://github.com/kavithamahesh) — Appium `processArguments` ([#86](https://github.com/devicelab-dev/maestro-runner/issues/86))
- [@pk1m](https://github.com/pk1m) — AWS Device Farm support ([#83](https://github.com/devicelab-dev/maestro-runner/issues/83))
- [@j-ezeh](https://github.com/j-ezeh) — iOS alert handling ([#64](https://github.com/devicelab-dev/maestro-runner/issues/64))

## [1.1.15] - 2026-05-19

A broad reliability + ergonomics release driven mostly by real-user reports across iOS, Android, Flutter and web. Highlights: assertVisible now recognises React Native container testIDs on iOS, Android scroll is rewired to `adb input swipe` for cross-skin reliability (OneUI in particular), `waitForAnimationToEnd` actually polls instead of returning 0 ms, web tap is gated by a Playwright-style actionability check, and browser console errors auto-surface in the flow report.

### Added
- **Web actionability gate before `tapOn` / `doubleTapOn` / `longPressOn` / `inputText`** — Playwright-style auto-wait MVP. After find-element and before dispatch, the runner now waits up to 2 s polling at 50 ms for the element to be enabled in three orthogonal senses: `HTMLElement.disabled !== true`, `aria-disabled !== "true"`, and `pointer-events !== "none"`. (Visibility is enforced upstream by the finder cascade — see the `findByAXTree` notes below.) Catches the common "looks tappable, isn't yet" flakes in modals, multi-step forms, and submit buttons that flip enabled on `change`. Stable-bounding-box polling is the next slice. When the gate times out, the error message now reports the specific rejection reason (e.g. `last rejection: pointer-events-none`).
- **Web: browser console errors + uncaught JS exceptions auto-surface in the flow report** — the CDP driver was already capturing `console.log/warn/error/info` and `Runtime.exceptionThrown` events, but they were only visible if the flow explicitly called `getConsoleLogs` / `assertNoJSErrors`. Now every flow gets a collapsed "Browser console" section in `report.html` (and a `consoleLogs` array in the per-flow JSON) with counts, colour coding, and full entries. Mobile drivers are unaffected.
- **Web: `failOnConsoleError` flow config** — opt-in stricter mode that fails the flow when any captured console error (or uncaught exception) fires during the run. Off by default.
- **`--user-data-dir` flag for persistent Chrome profile** (`MAESTRO_USER_DATA_DIR`) — reuse cookies, localStorage, sessionStorage, and installed extensions across runs. Speeds up auth-heavy CI suites (log in once, reuse across flows) and supports flows that depend on installed extensions. Default unset → existing ephemeral-profile behaviour.
- **`--env-file` flag for `.env`-style environment loading** — loads `KEY=VALUE` pairs (with single/double quoting, `#` comments, blank-line skipping) into the flow runtime. Slots between workspace `Env:` block and `-e CLI` overrides, so precedence is workspace < env-file < `-e`. Lets CI keep secrets out of flow YAML.
- **`--driver-start-timeout <seconds>` flag** (`MAESTRO_DRIVER_START_TIMEOUT`) — overrides the 30 s hard-coded driver-start timeout for UIA2 / DeviceLab Android drivers. AWS Device Farm low-end Samsung devices take ~60–80 s for cold-path APK install + dex2oat + JVM warmup; the runner force-stopped them at +30 s every time. Default 0 keeps the existing 30 s behaviour. Reported by [@pk1m](https://github.com/pk1m) ([#76](https://github.com/devicelab-dev/maestro-runner/issues/76)).
- **`runFlow` with `when:` gets an `else:` branch** (parity fix) — three interchangeable YAML shapes (else as file, else as inline `commands:`, else inheriting parent file). Cleans up branching auth setups (run sign-in if not logged in, otherwise run the signed-in path) without a second top-level conditional.
- **`tapOn` / `longPressOn` / `tapOnPoint` accept `duration:` (ms)** (parity fix) — routes through each driver's long-press path. `tapOn.longPress: true` now works on UIA2 / DeviceLab / Appium too (was WDA-only), defaulting to 1000 ms. `longPressOn.duration` is also configurable (was hardcoded 1 s).
- **`openNotifications` step (Android)** (parity fix) — pulls down the notification shade via `cmd statusbar expand-notifications`. Dispatched by UIA2 + DeviceLab; no-op on iOS.
- **`removeMedia` step (Android)** (parity fix) — clears the MediaStore index for deterministic test setup. Symmetric with `addMedia`. Tries the modular provider first, falls back to legacy.
- **`scrollUntilVisible.direction` and `setAirplaneMode.enabled` support `${VAR}` interpolation** (parity fix) — values resolve at execute time, so the same flow YAML works across environments.
- **Pre-flight warning when `--app-file` looks like a Flutter debug build** — scans the `.app` bundle for `Frameworks/App.framework/flutter_assets/kernel_blob.bin` (the Dart kernel snapshot, present in debug, absent in release/profile AOT). Prints a yellow startup warning pointing at `flutter build ios --release/--profile`. Advisory only — unusual setups with a live `flutter run` daemon reachable from the test host can still succeed.
- **WDA crash-loop circuit breaker** — when the same client connects + dies repeatedly with no productive request in between, the runner now bails with a clear error instead of letting the retry storm fill the logs. Drove this through a real iPad Flutter crash that previously surfaced as silent log flooding. Reported by [@divan](https://github.com/divan) ([#38](https://github.com/devicelab-dev/maestro-runner/issues/38)).

### Fixed
- **iOS `assertVisible` by `id` for React Native container testIDs** (parity fix) — `assertVisible: { id: ... }` failed against `<View testID="…">` containers on both iOS simulator and real device. WDA's page-source filter rejected any element XCUITest reports as `visible="false"`, including RN wrapper views that have no own visual content but host visible children. Maestro CLI never consults that attribute, so the same flow worked on CLI. Added a phased visibility check: prefer `visible="true"` matches; fall back to `visible="false"` candidates only when they host at least one visible descendant — recovers RN container testIDs while still rejecting hidden-but-still-mounted screens. When the rescue path matches, the step result records `matchNote` in `report.json` and the step message becomes `Element is visible (matched via visible descendant …)`. Reported by [@AlonG-Papaya](https://github.com/AlonG-Papaya) ([#80](https://github.com/devicelab-dev/maestro-runner/issues/80)).
- **Android `scroll` / `scrollUntilVisible` on Samsung OneUI** (parity fix) — three compounding bugs caused `scrollUntilVisible` to either short-circuit without scrolling or report `Element not found after 20 scrolls` while the viewport never moved:
  1. `scrollUntilVisible` declared success when the target only existed in the off-screen portion of the view hierarchy. Now verifies the matched element actually overlaps the viewport.
  2. Both Android drivers routed scroll through gesture APIs that silently no-op on several Android skins (`/appium/gestures/scroll` on OneUI for the `uiautomator2` driver; the on-device agent's MotionEvent injection with zero-ms duration and inverted direction for the `devicelab` driver). The default scroll backend is now `adb input swipe` for both drivers — the same OS-level path you'd get from an `adb shell input swipe` call by hand. The agent itself was also corrected — `scroll` now uses scroll semantics (direction = what gets revealed), `swipe` keeps touch semantics (direction = finger motion), and `speed <= 0` is clamped to 300 ms. Bundled APK rebuilt.
  3. Infrastructure errors during element lookup (dead session, connection refused) were silently counted as "not found yet" and made failures surface as `Element not found after 20 scrolls`. Real errors now propagate immediately.

  The old gesture path is still available per step for users who need it:
  ```yaml
  - scrollUntilVisible:
      element: { id: "give feedback" }
      direction: DOWN
      engine: agent      # opt out of the default adb swipe
  ```

  Verified on a Samsung Galaxy M16 (OneUI, Android 14). Reported by [@George-Anton-Tarazi](https://github.com/George-Anton-Tarazi) ([#81](https://github.com/devicelab-dev/maestro-runner/issues/81)), with prior investigation in [#28](https://github.com/devicelab-dev/maestro-runner/pull/28) by [@maggialejandro](https://github.com/maggialejandro).
- **`waitForAnimationToEnd` actually waits** (parity fix) — the UIA2 / DeviceLab / WDA implementations were stubs that returned success in 0 ms (and logged "WARNING: not fully implemented"), making the step a no-op gate. The configured `timeout:` field was parsed but discarded. The step now polls two consecutive screenshots, computes the fraction of differing pixels, returns success once ≤ 0.5 % differ (i.e. screen is static), and respects `timeout:` everywhere (default 15 s). On timeout it soft-returns success so a never-settling animation doesn't block the surrounding flow. Web CDP path now honours the user-supplied timeout instead of a hardcoded 10 s.
- **Silent wrong-element tap for lazy ListView items on Android Flutter** (parity fix) — `tapOn: { id: "X" }` against an item in a `ListView`'s cache-extent buffer (laid out but not in the visible viewport) silently dispatched a coordinate tap at the cache item's bounds, which often fell inside the status / nav-bar safe area on top of an unrelated widget. Tests "passed" against the wrong target. The Flutter VM service path now rejects taps whose target lies in the top 3 % status bar or bottom 5 % nav / gesture area (or fully off-screen) and returns a clear error pointing at `scrollUntilVisible` as the fix.
- **Duplicate console events in per-flow report** — when `cfg.URL` was set, the CDP driver pre-navigated to that URL during construction, so console events from that load fired *before* the user's flow started. The flow's first `launchApp` re-navigated to the same URL and fired the same events again, producing duplicates (8 entries for 4 distinct events in the verified repro). The runner now resets the console buffer at flow start; mobile / native drivers that don't implement the reset interface are unaffected.
- **Web `tapOn` resolving to non-Element nodes (#text, `<title>`)** — on SPAs that put route labels into `document.title` (e.g. saucedemo, demoblaze), the AX-tree finder's accessible-name search returned a backend handle for the document title; the actionability gate rejected it but the find cascade had already committed to that element. Same shape for accessible names derived from a single child text node (`<button>Back to products</button>`) — the AX tree returned the `#text` node, which Rod's `Click()` can't dispatch on. `findByAXTree` now (a) skips non-renderable tags (`<title>`, `<script>`, etc.) and (b) walks up to the parent Element when the resolved handle isn't itself an Element. Surfaced by saucedemo and demoblaze regression flows.

### Contributors
- [@AlonG-Papaya](https://github.com/AlonG-Papaya) — reported [#80](https://github.com/devicelab-dev/maestro-runner/issues/80) (iOS RN container testID)
- [@George-Anton-Tarazi](https://github.com/George-Anton-Tarazi) — reported [#81](https://github.com/devicelab-dev/maestro-runner/issues/81) (Android scroll on OneUI)
- [@maggialejandro](https://github.com/maggialejandro) — prior investigation of the Android scroll path in [#28](https://github.com/devicelab-dev/maestro-runner/pull/28)
- [@pk1m](https://github.com/pk1m) — reported [#76](https://github.com/devicelab-dev/maestro-runner/issues/76) (driver-start timeout on AWS Device Farm)
- [@divan](https://github.com/divan) — reported [#38](https://github.com/devicelab-dev/maestro-runner/issues/38) (Flutter debug build crash loop on iPad)

## [1.1.14] - 2026-05-12

This release closes out the Flutter Web testing story. v1.1.13 fixed the *finding* layer (selectors traverse same-origin iframes, `index` is a first-class web selector). v1.1.14 completes it: selectors also pierce open shadow roots, `tapOn` dispatches at correct top-frame viewport coordinates when the target lives inside an iframe (with hit-target verification), the same path extends to `doubleTapOn` / `longPressOn` / `scrollUntilVisible`, visibility checks intersect iframe content viewports, and `tapOn` handles Flutter Web's `<flutter-view>` pointer-router glass pane that consumes trusted events before any third-party listener can observe them. A real Flutter Web user — [@richjun](https://github.com/richjun) — drove most of this with two substantial PRs ([#73](https://github.com/devicelab-dev/maestro-runner/pull/73), [#74](https://github.com/devicelab-dev/maestro-runner/pull/74)) and two issue reports ([#71](https://github.com/devicelab-dev/maestro-runner/issues/71), [#72](https://github.com/devicelab-dev/maestro-runner/issues/72)).

### Added
- **Selectors pierce open shadow roots on web** — `text` / CSS / `id` / attribute /
  role finders, plus the visibility and wait helpers, now recurse through
  every same-origin `<iframe>` *and* every open `shadowRoot` reachable from
  them. Flutter Web mounts its accessibility tree inside an open shadow root
  attached to `<flt-glass-pane>`, so `tapOn: "Close"` against a Flutter Web
  semantics node now resolves to the actual element. Closed shadow roots
  remain unreachable (same constraint every WebDriver-class tool has — no
  fix possible without privileged access). Reported by
  [@richjun](https://github.com/richjun) ([#71](https://github.com/devicelab-dev/maestro-runner/issues/71)).
- **`tapOn text + index` enumerates across iframes / shadow roots** —
  completes the [#67](https://github.com/devicelab-dev/maestro-runner/issues/67) fix from 1.1.13.
  Previously the resolver enumerated matches only within the top frame, so
  asking for index 1 when matches 0..N-1 lived in the top frame and the
  real target lived in an iframe silently re-tapped the in-range top-frame
  match — green test, wrong button. Now walks every same-origin root via
  `_collectRoots()`, sorts by document order, and indexes deterministically.
  Out-of-range returns a precise error with the actual match count instead
  of falling back. Reported by [@richjun](https://github.com/richjun)
  ([#72](https://github.com/devicelab-dev/maestro-runner/issues/72)).
- **`tapOn` dispatches at top-frame coordinates for iframe-nested targets** —
  Rod's `Element.Click()` used iframe-LOCAL viewport coordinates from
  `getBoundingClientRect()`; CDP `Input.dispatchMouseEvent` operates in
  TOP-FRAME viewport coordinates. The click landed at the wrong place and
  `tapOn` reported success silently. Now ports Playwright's
  `_checkFrameIsHitTarget` walk: from the target outward, adds each
  ancestor `<iframe>` element's box plus its content-area inset (border +
  padding) to convert iframe-local → top-frame viewport coordinates.
  Hit-target verification runs as both static pre-flight (rejects
  occluded / wrong-element clicks before dispatch) and post-click trusted-
  event capture (verifies the click landed on the target's frame tree).
  Contributed by [@richjun](https://github.com/richjun) in
  [#73](https://github.com/devicelab-dev/maestro-runner/pull/73).
- **`doubleTapOn` / `longPressOn` / `scrollUntilVisible` inherit the
  iframe-coord-translated path** — same root cause as `tapOn` had. Now
  routed through a shared `dispatchCrossRoot` helper. `scrollUntilVisible`
  for iframe-nested targets calls native `Element.scrollIntoView()` inside
  the element's own document (the previous page-level `Mouse.Scroll` only
  scrolled the outer document and never reached iframe content).
- **Visibility check intersects iframe content viewport** —
  `_isElementVisible` used to do intrinsic-only checks (computed style +
  `getBoundingClientRect()` dimensions) and reported elements scrolled or
  clipped outside their iframe's content viewport as "visible." This made
  `assertVisible` / `waitForVisible` / `extendedWaitUntil` silently pass
  on iframe-clipped elements, and made `scrollUntilVisible`'s loop exit
  on iteration 0 (the new `scrollIntoView` branch was unreachable in
  practice). Now walks the iframe ancestor chain at each level,
  intersecting with the iframe's content viewport. Empty intersection
  returns false; surviving rect is translated to parent coordinates and
  rechecked. Top-frame "below the fold" elements stay visible — only
  iframe clipping is added.
- **`tapOn` into Flutter Web semantics** — three orthogonal fixes for
  Flutter Web targets. `findBySearch` now rejects non-tappable text
  containers (`<script>` / `<style>` / `<template>` / etc.) because CDP
  `DOM.performSearch` matches against serialized HTML and Flutter Web
  pages whose JS source contains the button label as a string literal
  silently returned the `<script>` element. The hit-target pre-flight
  and post-click verifier both accept the Flutter `<flutter-view>` glass-
  pane occlusion case (target + topmost hit both inside `<flutter-view>`);
  Flutter intercepts trusted pointer events at the document/glass-pane
  capture layer and routes them through its own internal pointer router
  for semantics dispatch, so the verifier's one-shot listener never fires
  and a strict same-element walk-up always reports false occlusion. Non-
  Flutter occlusion (overlay div, modal, genuine z-stack) continues to
  fail-fast — the Occluded and Transformed regression tests still reject.
  Contributed by [@richjun](https://github.com/richjun) in
  [#74](https://github.com/devicelab-dev/maestro-runner/pull/74).

### Fixed
- **`runScript` per-call scope + persistent `output` mutations** — two
  related bugs. (a) top-level `const` / `let` / `function` declarations
  collided across `runScript` calls because the JS engine reused a single
  Goja runtime's global scope, surfacing as
  `SyntaxError: Identifier 'word' has already been declared` on the second
  invocation. Each `runScript` now executes inside an IIFE so top-level
  declarations are function-scoped to that invocation. (b) Mutations like
  `output.list.push(x)` did not persist across `runScript` calls because
  the `output` proxy returned a snapshot Go map per call — only whole-
  value reassignment (`output.list = [...]`) survived. The `output` bag
  is now a Goja-native `Object` shared across invocations so mutations
  persist. Reported by [@Sina-KH](https://github.com/Sina-KH)
  ([#70](https://github.com/devicelab-dev/maestro-runner/issues/70)).
- **iOS `openLink` on simulator** — `POST /session/<sid>/url` on
  WebDriverAgent v12+ returns `Unhandled endpoint: /url`. Users who ran
  `maestro-runner wda update` and got the newer WDA hit a hard failure
  on every `openLink` step, blocking Expo dev client flows where deep
  linking loads the JS bundle from Metro. Bypassed entirely on
  simulators by shelling out to `xcrun simctl openurl <udid> <url>` —
  same primitive Maestro CLI uses, faster, no WDA version coupling.
  Real iOS devices keep the existing WDA `/url` path (`simctl` can't
  reach them). Reported by [@jongbelegen](https://github.com/jongbelegen)
  ([#68](https://github.com/devicelab-dev/maestro-runner/issues/68)).
- **iOS `clearState` on simulator no longer requires `--app-file`** —
  the runner needs to uninstall + reinstall the app to wipe its data
  container (Apple doesn't expose a "clear data only" API). Previously
  failed with either `clearState on iOS requires --app-file` (no
  `--app-file`) or `lstat ... No such file or directory` (if
  `--app-file` pointed inside the live sim container, which the
  uninstall deleted before install could read it). Now auto-discovers
  the installed `.app` via `xcrun simctl get_app_container` and copies
  it to a temp directory before the uninstall — same approach Maestro
  CLI uses (`LocalSimulatorUtils.kt#reinstallApp`). Reported by
  [@jongbelegen](https://github.com/jongbelegen)
  ([#69](https://github.com/devicelab-dev/maestro-runner/issues/69)).

### Contributors

[@richjun](https://github.com/richjun)
1. Reported selectors not piercing shadow DOM ([#71](https://github.com/devicelab-dev/maestro-runner/issues/71))
2. Reported `tapOn text+index` not spanning iframes ([#72](https://github.com/devicelab-dev/maestro-runner/issues/72))
3. Contributed iframe + shadow-root coord-translated `tapOn` with hit-target verification ([#73](https://github.com/devicelab-dev/maestro-runner/pull/73))
4. Contributed Flutter Web semantics support — finder rejection, pre-flight and post-click glass-pane concession ([#74](https://github.com/devicelab-dev/maestro-runner/pull/74))

[@Sina-KH](https://github.com/Sina-KH)
1. Reported `runScript` top-level declaration collisions and non-persistent `output` mutations ([#70](https://github.com/devicelab-dev/maestro-runner/issues/70))

[@jongbelegen](https://github.com/jongbelegen)
1. Reported iOS `openLink` failing on simulator after WDA upgrade ([#68](https://github.com/devicelab-dev/maestro-runner/issues/68))
2. Reported iOS `clearState` on simulator failing without / with `--app-file` ([#69](https://github.com/devicelab-dev/maestro-runner/issues/69))

## [1.1.13] - 2026-05-05

### Added
- **Same-origin iframe traversal on web** — text/CSS/ID/attribute selectors now
  walk into same-origin `<iframe>` content (e.g. Flutter Web embedded under a
  host page). Cross-origin / OOPIF iframes are still skipped, but the
  not-found error now surfaces a clear `(skipped N cross-origin iframes — full
  OOPIF support not implemented yet)` hint so users debugging a missing
  selector can tell the cause is frame isolation, not a typo. Reported by
  [@richjun](https://github.com/richjun) ([#65](https://github.com/devicelab-dev/maestro-runner/issues/65)).
- **Mobile-style `index` selector on web** — `tapOn: { text: "Help", index: 1 }`
  now picks the second match instead of being silently dropped as
  unsupported. The web finder accepts both `index` (string, mobile-style) and
  `nth` (int) via a single `EffectiveNth()` helper, so the same flow YAML
  works across Android, iOS, and web. Reported by
  [@richjun](https://github.com/richjun) ([#67](https://github.com/devicelab-dev/maestro-runner/issues/67)).
- **Sauce Labs job context per flow** — the runner now posts
  `sauce:context` to Sauce on every flow start so jobs surface the YAML
  basename in the Sauce UI, and renames empty / "Default Appium Test" jobs
  on completion using the first flow's filename. Real-device caps without
  `appium:jobUuid` fall back to VMS + session id so REST status updates
  still target the right job. Contributed by
  [@eyaly](https://github.com/eyaly) ([#66](https://github.com/devicelab-dev/maestro-runner/pull/66)).

### Fixed
- **`onFlowStart` hook with default `appId`** — `launchApp` (and other app
  lifecycle steps) inside `onFlowStart` / `onFlowComplete` now resolve the
  flow's default `appId` the same way as top-level steps. Previously the
  hook ran with an empty `AppID`, causing a silent no-op on Android. Fixes
  [#62](https://github.com/devicelab-dev/maestro-runner/issues/62), reported
  by [@zcsteele](https://github.com/zcsteele).
- **`copyTextFrom` on Appium 3.x** — stop pushing the captured text to the
  device clipboard via `POST /appium/device/set_clipboard`, which Appium 3
  returns 404 for. The runner already keeps the value in memory (matching
  Maestro's design) so `pasteText` continues to work. Fixes
  [#61](https://github.com/devicelab-dev/maestro-runner/issues/61), reported
  by [@kavithamahesh](https://github.com/kavithamahesh).
- **iOS permission dialogs blocking real-device flows** — WDA's alerts
  monitor only registers when `defaultAlertAction` is in the session-creation
  capabilities; the runner now defaults to `accept` so notification (and
  other) permission dialogs auto-dismiss out of the box. Fixes
  [#64](https://github.com/devicelab-dev/maestro-runner/issues/64), reported
  by [@j-ezeh](https://github.com/j-ezeh).
- **assertVisible silently wrong for state filters / nth / role** — the JS
  fast path bypassed several capabilities the Go finder already implemented,
  so selectors with `enabled` / `checked` / `focused` / `nth` / `role` /
  ID-cascade hit the fast path and produced wrong answers. Centralised
  routing now sends those selectors to the Go finder; the JS path's `id`
  case also runs the same `data-testid` / `name` / `aria-label` cascade.

### Contributors

[@richjun](https://github.com/richjun)
1. Reported same-origin iframe selector failures with Flutter Web ([#65](https://github.com/devicelab-dev/maestro-runner/issues/65))
2. Reported `index` selector being silently dropped on web ([#67](https://github.com/devicelab-dev/maestro-runner/issues/67))

[@zcsteele](https://github.com/zcsteele)
1. Reported `onFlowStart` hook unable to reference default `appId` ([#62](https://github.com/devicelab-dev/maestro-runner/issues/62))

[@kavithamahesh](https://github.com/kavithamahesh)
1. Reported `copyTextFrom` failing on Appium 3.x with 404 ([#61](https://github.com/devicelab-dev/maestro-runner/issues/61))

[@j-ezeh](https://github.com/j-ezeh)
1. Reported iOS permission dialogs not auto-accepted on real devices ([#64](https://github.com/devicelab-dev/maestro-runner/issues/64))

[@eyaly](https://github.com/eyaly)
1. Improved Sauce Labs job naming + per-flow context ([#66](https://github.com/devicelab-dev/maestro-runner/pull/66))

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
- `sleep` command — pause execution for a given number of milliseconds. Supports scalar (`- sleep: 500`) and mapping syntax
- `isKeyboardVisible` command — query whether the soft keyboard is currently shown. Returns result in `CommandResult.Data` (boolean). Available via YAML, JSON, and REST API
- `hideKeyboard` strategy field — specify `strategy: appium|escape|esc|back` to force a specific dismissal method instead of trying all three
- `KeyboardVisible` field added to `StateSnapshot` for richer state introspection
- REST API server (`maestro-runner server`) — session-based HTTP server for executing Maestro steps via JSON instead of YAML flow files. Supports session management, screenshots, view hierarchy, and device info. Configurable port via `--port` flag or `MAESTRO_SERVER_PORT` env var
  ```bash
  maestro-runner --platform android server --port 9999
  ```
- JSON step unmarshaling (`pkg/flow/json.go`) — all step types can now be deserialized from JSON, enabling the REST API execute endpoint
- JSON struct tags on all flow step types and Selector for proper serialization/deserialization
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
