# devicelab-ios-runner — Wire Protocol v1

Internal HTTP+JSON protocol between maestro-runner's Go consumer (`pkg/driver/devicelab_ios/`) and the on-device XCUITest runner.

**Status:** v1 draft, no consumers yet. Free to evolve.
**Owners:** maestro-runner team.
**Stability:** owned outright. The protocol bumps on breaking change; see [Versioning](#versioning).

## Design rules

These are non-negotiable. If a change conflicts with one of these, the change is wrong.

1. **Stateless app binding.** Every command carries `appBundleId`. Runner caches `XCUIApplication(bundleIdentifier:)` until it changes. No session lifecycle.
2. **No element IDs / element handles.** Operations target either coordinates or fresh selectors. Eliminates stale-element bugs and WebDriver session state entirely.
3. **Full hierarchy, no filtering.** Snapshot returns every node with every accessibility attribute. No `interactiveOnly` / `compact` / occlusion / element caps. Flow selectors run on the Go side against the full tree.
4. **Inline binary payloads.** Screenshots and recordings return base64 bytes in the JSON envelope. No file-on-device + fetch-back round trips.
5. **Caller-chosen port.** Go side picks the port and injects via env var. Runner never picks a random port.
6. **Both path and direction gestures.** Swipe / scroll accept either `(x, y, x2, y2, durationMs)` or `(direction, percent)`.
7. **Every command times out.** Hard 30s ceiling enforced server-side. Hung commands cannot wedge the runner.

## Transport

| Field | Value |
|---|---|
| Protocol | HTTP/1.1 over TCP |
| Host | `127.0.0.1` (simulator) or tunneled device port |
| Port | Caller-injected via `DEVICELAB_IOS_RUNNER_PORT` env var |
| Path | `POST /command` |
| Content-Type | `application/json; charset=utf-8` |
| Max request size | 4 MB (covers any reasonable `type` text payload) |
| Connection | Keep-alive supported; runner is single-threaded so requests serialize |

The runner also exposes `GET /health` (no JSON body, returns `200` once ready) for the Go side to await readiness without sending a command.

## Port allocation

- Go side picks a free ephemeral port (`net.Listen("tcp", "127.0.0.1:0")`, immediately close, reuse port number).
- Inject the port into the runner via the `.xctestrun` `EnvironmentVariables.DEVICELAB_IOS_RUNNER_PORT` before invoking `xcodebuild test-without-building`.
- For real devices: same port on device, plus `iproxy <hostPort>:<devicePort>` (or equivalent via `pymobiledevice3`) so `127.0.0.1:<hostPort>` reaches the runner.

## Request envelope

```jsonc
{
  "command": "<commandType>",
  "appBundleId": "<bundle-id>",          // required for app-targeted commands
  // ... command-specific fields
}
```

Unknown fields are ignored. Missing required fields produce `INVALID_ARGUMENT`.

## Response envelope

```jsonc
{
  "ok": true,
  "data": { /* command-specific */ }
}
```

```jsonc
{
  "ok": false,
  "error": {
    "code": "ELEMENT_NOT_FOUND",
    "message": "human-readable",
    "details": { /* optional structured context */ }
  }
}
```

`data` is always an object (never an array or scalar). `error` is present iff `ok === false`.

### Error codes

| Code | When |
|---|---|
| `INVALID_ARGUMENT` | Missing/invalid request field |
| `UNSUPPORTED_OPERATION` | Command exists but is not available in this build/platform |
| `APP_NOT_RUNNING` | `appBundleId` not foregrounded and could not be activated |
| `ELEMENT_NOT_FOUND` | Selector returned zero matches |
| `ELEMENT_NOT_INTERACTABLE` | Element matched but is not visible / enabled / hittable |
| `ALERT_PRESENT` | A system alert blocked the operation |
| `TIMEOUT` | Server-side 30s ceiling exceeded |
| `XCUI_EXCEPTION` | Underlying XCUITest threw an ObjC exception (retried once before this) |
| `RUNNER_INTERNAL` | Bug in the runner |

## Versioning

- Protocol version is `1`. Single integer. Bumped on any breaking change to existing command shapes or response semantics.
- `GET /version` returns `{"protocolVersion": 1, "runnerBuild": "<sha-or-tag>"}`.
- The Go client asserts on connect that `protocolVersion` matches the version it was built against. Mismatch = hard error, no version negotiation.
- Adding new optional fields or new commands does not bump the version.

## Command surface

### Lifecycle

| Command | Inputs | Returns | Notes |
|---|---|---|---|
| `appLaunch` | `appBundleId`, `arguments[]?`, `environment{}?` | `{ pid, launched: true }` | `XCUIApplication(bundleIdentifier:).launch()`. Replaces cached `currentApp`. |
| `appActivate` | `appBundleId` | `{ pid, activated: true }` | Foreground without relaunch. |
| `appTerminate` | `appBundleId` | `{ terminated: true }` | |
| `appState` | `appBundleId` | `{ state: "running"|"notRunning"|"runningBackground" }` | |
| `appCurrent` | (none) | `{ appBundleId?, pid? }` | Returns currently foregrounded app. |
| `shutdown` | (none) | `{ shuttingDown: true }` | Runner exits cleanly. |
| `ping` | (none) | `{ uptimeMs }` | Round-trip baseline. |

### Snapshot

| Command | Inputs | Returns |
|---|---|---|
| `snapshot` | `appBundleId?` | `{ nodes: [SnapshotNode] }` |
| `screenSize` | `appBundleId?` | `{ width, height, scale }` |

`snapshot` always returns the full tree. If `appBundleId` is omitted, snapshots the currently foregrounded app via `XCUIApplication()`. **No filtering, no caps, no occlusion computation.**

#### SnapshotNode

Flat array; tree reconstructed via `parentIndex`. Every node carries every available accessibility field — Go-side selectors decide what to match on.

```jsonc
{
  "index": 12,
  "parentIndex": 5,        // omitted for root
  "depth": 3,
  "type": "XCUIElementTypeButton",   // raw XCUIElementType string
  "identifier": "login-button",      // .identifier — accessibility ID
  "label": "Sign In",                // .label
  "value": "",                       // .value (e.g. text field contents)
  "placeholderValue": "",            // .placeholderValue
  "rect": { "x": 32.0, "y": 624.6, "width": 338.0, "height": 50.0 },
  "enabled": false,
  "displayed": true,                 // visible in the viewport (rect inside screen bounds, not occluded by parent clipping)
  "selected": false,
  "focused": false,
  "hittable": true                   // .isHittable
}
```

**Deltas from agent-device's runner:** adds `identifier`, `value`, `placeholderValue`, `selected`, `focused`, `displayed`. Drops `interactiveOnly`/`compact`/occlusion logic.

### Find

These are conveniences when the Go side already knows what it wants and does not need the full snapshot. They return one or more `SnapshotNode`s. **No element ID is retained** — the returned bounds are the caller's only handle.

| Command | Inputs | Returns |
|---|---|---|
| `findByIdentifier` | `appBundleId`, `identifier` | `{ found: bool, node?: SnapshotNode }` |
| `findByLabel` | `appBundleId`, `label`, `exact?: bool` | `{ found: bool, node?: SnapshotNode }` |
| `findByPredicate` | `appBundleId`, `predicate` | `{ found: bool, nodes: [SnapshotNode] }` |
| `findByClassChain` | `appBundleId`, `query` | `{ found: bool, nodes: [SnapshotNode] }` |

Selector strategies match WDA's: predicate strings and class chains map directly to `XCUIElement.matching(NSPredicate(...))` and the XCUIElementQuery class-chain syntax.

### Gestures

All gestures accept `appBundleId` so the runner can target the right `XCUIApplication` for coordinate-space resolution.

| Command | Inputs | Returns |
|---|---|---|
| `tap` | `x, y` | `{ gestureStartMs, gestureEndMs, referenceWidth, referenceHeight }` |
| `doubleTap` | `x, y` | gesture timings |
| `longPress` | `x, y, durationMs` | gesture timings |
| `swipe` | EITHER `x, y, x2, y2, durationMs` OR `direction, percent` | gesture timings + `path: [{x,y},...]` |
| `scroll` | EITHER coordinate path OR `direction, percent` (same as swipe but with slower default duration) | gesture timings |
| `drag` | `x, y, x2, y2, durationMs, holdMs?` | gesture timings + path |
| `pinch` | `x, y, scale, durationMs` | gesture timings |
| `pressButton` | `button: "home"|"lock"|"volumeUp"|"volumeDown"|"appSwitcher"` | `{ pressed: true }` |
| `rotate` | `orientation: "portrait"|"landscapeLeft"|"landscapeRight"|"portraitUpsideDown"` | `{ orientation }` |

**Delta from agent-device:** swipe + scroll accept coordinate paths AND direction. drag, pinch, pressButton, rotate kept; tapSeries / dragSeries / interactionFrame dropped (LLM-shaped, not needed for flows).

### Text

| Command | Inputs | Returns |
|---|---|---|
| `type` | `text`, `typingFrequency?` | `{ typed }` |
| `eraseText` | `count` (number of chars to delete) | `{ erased }` |
| `keyPress` | `key: "enter"|"tab"|"backspace"|"escape"` | `{ pressed }` |
| `clipboardGet` | (none) | `{ text }` |
| `clipboardSet` | `text` | `{ set: true }` |

`type` sends keys to the currently focused element (caller is responsible for tapping to focus first — same as WDA today).

### Alerts

| Command | Inputs | Returns |
|---|---|---|
| `alertVisible` | (none) | `{ visible: bool, button1Label?, button2Label? }` |
| `alertAccept` | (none) | `{ dismissed: true }` |
| `alertDismiss` | (none) | `{ dismissed: true }` |

### URLs

| Command | Inputs | Returns |
|---|---|---|
| `openURL` | `url` | `{ opened: true }` |

Uses `XCUIDevice.shared.system.open(URL)` (iOS 15+) or SpringBoard tap path as fallback.

### Settings (runtime)

| Command | Inputs | Returns |
|---|---|---|
| `setIdleTimeout` | `timeoutMs` (0 disables) | `{ set }` |
| `setTypingFrequency` | `freq` (chars/sec) | `{ set }` |
| `setFindTimeout` | `timeoutMs` | `{ set }` |

Cover `core.SetWaitForIdleTimeout`, `TypingFrequencyConfigurer`, `core.SetFindTimeout`.

### Recording (Phase 5+; not in v1 minimum)

| Command | Inputs | Returns |
|---|---|---|
| `recordStart` | `outPath` (host path), `fps?`, `quality?` | `{ recording: true }` |
| `recordStop` | (none) | `{ bytes: <base64> }` or `{ outPath }` |

Deferred — `core.Driver` doesn't expose recording today; can add in Phase 6.

## Out of scope for v1

These exist in maestro-runner today but stay on the **Go side** via `xcrun simctl` / `xcrun devicectl`, not in the runner:

- App install / uninstall (`simctl install` / `devicectl install`)
- Clear app data (uninstall + reinstall)
- Permission grants (`simctl privacy`, `devicectl set-permission`)
- Reset keychain (`simctl keychain reset`)
- Airplane mode (open Settings app, find toggle, tap — uses regular runner commands)
- Reading app container files

This split mirrors how the current `pkg/driver/wda/` and `pkg/driver/devicelab/` (Android) drivers do these via shell calls, not in-process.

## Deltas from agent-device's protocol (with rationale)

| Item | agent-device | devicelab-ios-runner | Why |
|---|---|---|---|
| Element refs | `@e1`/`@e2` computed on TS side, used in tap | None | Stale-element risk; no benefit for flows that re-snapshot anyway |
| Snapshot filtering | `interactiveOnly`, `compact`, occlusion | None | LLM-shaped; flows need full tree for selector matching |
| Snapshot caps | 600 elements / 2 MB | None | Caps cause silent flow failures when the screen is busy |
| Screenshot return | File path on simulator | Inline base64 bytes | One round trip instead of two; works the same on real devices |
| Swipe | Direction only | Path OR direction | maestro-runner flows often use coordinate swipes for trackpads/sliders |
| Snapshot fields | `index/depth/type/label/rect/hittable/enabled` | + `identifier`, `value`, `placeholderValue`, `selected`, `focused`, `displayed` | Required by `id=` / `value=` / focus-aware selectors |
| Port allocation | Random if env unset | Always caller-injected | Real-device tunneling needs the port before launch |
| `readText` | Point-based (`x, y` required) | Use `findByIdentifier` or `snapshot` and read `label`/`value` directly | Single way to get text |
| App lifecycle | TS-side only (`simctl launch`) | Either runner (via `XCUIApplication.launch()`) or Go-side (`simctl`) | Runner-side is faster for warm flows; Go-side stays available |
| Recording | First-class command | Deferred to Phase 5+ | Not in `core.Driver` today |
| Protocol stability | "Maintainer document, can change" | Versioned, breaking changes bump version | We are the only consumer; cheap to be strict |

## Open questions to revisit before Phase 3 implementation

- **`type` without a tap-to-focus first.** WDA's `SendKeys` works against the global keyboard. Should we require a focused element, or accept silent no-op? Current call sites all tap first — keep WDA semantics.
- **`displayed` semantics.** WDA's `isDisplayed` differs from XCUI's `isHittable`. We compute `displayed` as "rect intersects screen bounds AND parent clip path", `hittable` as raw `.isHittable`. Validate with TestHive flows.
- **Predicate string compatibility.** WDA supports a specific NSPredicate dialect via Appium. We should support the same dialect so existing flow selectors keep working — verify against `pkg/driver/wda/driver.go:findElementByWDA`.
- **`scroll` vs `swipe` duration defaults.** WDA scrolls slower than it swipes. Per react-navigation PR #13027, swipe duration tuning is touchy. Pick defaults that match upstream Maestro out of the gate.
