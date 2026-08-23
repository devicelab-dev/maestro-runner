# Runner source attribution

All `RunnerTests*.swift` files in this directory and the
`RunnerObjCExceptionCatcher.{h,m}` pair are **verbatim copies** of files from
[callstackincubator/agent-device](https://github.com/callstackincubator/agent-device)
(`ios-runner/AgentDeviceRunner/AgentDeviceRunnerUITests/`), cloned at the v0.15.x
series (early 2026). Licence: MIT (see upstream `LICENSE`).

The only intentional local edits are:

1. `RunnerTests+Environment.swift` accepts `DEVICELAB_IOS_RUNNER_PORT` as an
   alias of `AGENT_DEVICE_RUNNER_PORT` so the maestro-runner Go driver does
   not need to use the upstream variable name.
2. `Bridging-Header.h` (this project's bridging header) imports
   `RunnerObjCExceptionCatcher.h`.
3. `RunnerTests+Models.swift`, `RunnerTests+Snapshot.swift`,
   `RunnerTests+SystemModal.swift`, `RunnerTests+Interaction.swift`:
   `SnapshotNode` carries a new optional `placeholderValue` field, populated
   from the XCUIElementSnapshot's placeholder. Upstream omits this because
   their LLM-driven consumer sees aggregated labels; maestro-runner YAML
   flows often match on placeholder text (e.g. RN TextInput
   placeholder="Username"), so we expose it.
4. `RunnerTests+Interaction.swift` `typeIntoCurrentTarget`: when the
   resolved target is a `.secureTextField`, route typing through
   `DLSendSyntheticTyping` (private XCSynthesizedEventRecord +
   XCPointerEventPath path, same as WebDriverAgent's FBTypeText). Upstream
   uses `app.typeText` exclusively. RN SecureTextField on iOS Simulator
   presents the keyboard but leaves `hasKeyboardFocus == false`, so
   `app.typeText` silently drops characters; the synthetic-event path
   bypasses keyboard-focus checks entirely. Non-secure fields still use
   `app.typeText`. Failure falls back to `app.typeText` so we never lose
   the upstream path entirely.

   Files added to support this:
   - `SyntheticTyping.h` / `SyntheticTyping.m` — ObjC bridge
   - `PrivateHeaders/XCTest/{XCSynthesizedEventRecord.h,
     XCPointerEventPath.h, XCUIDevice.h}` — class-dumped private
     framework headers used by the bridge
   - `Bridging-Header.h` imports `SyntheticTyping.h`

5. `RunnerTests+Interaction.swift` `focusTextInputForTextEntry`: accepts
   an optional `identifier` hint. When present, resolves the editable
   target via `descendants(matching:.any).matching(predicate: identifier == …)`
   (XCTest query DSL — uses an indexed lookup) instead of upstream's
   `textInputAt(x,y)` which enumerates every element via
   `allElementsBoundByIndex`. Saves ~1.4s per `type` call on dense
   screens. Falls back to upstream's x/y path when the identifier is
   absent or doesn't resolve to a text input.
   Call site in `RunnerTests+CommandExecution.swift` `executeTypeCommand`
   passes `command.selectorValue` as the hint when `command.selectorKey
   == "id"`.

6. `RunnerTests+Models.swift` `DataPayload`: added optional
   `pngBase64` field. `RunnerTests+CommandExecution.swift` `.screenshot`
   case now encodes the PNG inline (base64) and returns it in
   `pngBase64` instead of writing to the sim's tmp directory and
   returning the path in `message`. Upstream's tmp-file approach is for
   their TS daemon that mounts the sim container; maestro-runner needs
   the bytes on the host without a second simctl roundtrip.

7. `RunnerTests+Interaction.swift` `typeIntoCurrentTarget` for
   SecureTextField: `DLSendSyntheticTyping` typing speed raised from
   60 → 240 chars/sec (matches WDA's `FBTypeText` default). RN inputs
   accept this rate without dropping characters.

The wire-protocol contract (`RunnerTests+Models.swift` Command/Response shapes,
HTTP/JSON transport in `RunnerTests+Transport.swift`, dispatch behaviour in
`RunnerTests+CommandExecution.swift`) is **agent-device's contract** — the
maestro-runner Go driver in `pkg/driver/devicelab_ios/` translates maestro flow
steps into commands of that shape.

When pulling in upstream changes, re-copy the files and re-apply only the two
edits above.
