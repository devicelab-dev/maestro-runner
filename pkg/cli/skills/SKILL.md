---
name: maestro-runner
description: Write, run and debug Maestro YAML UI test flows with maestro-runner on Android devices and emulators, iOS simulators and iPhones, and desktop Chrome. Use when the user asks to create or fix a Maestro flow, run mobile UI tests, inspect a device's screen hierarchy, or read a maestro-runner report.
---
<!-- maestro-runner-skill-version: {{VERSION}} -->

# maestro-runner

maestro-runner is a single Go binary that runs Maestro YAML flows unchanged (no JVM).
It is not Maestro: it reads the same flow format with its own engine.
Docs: https://open.devicelab.dev/maestro-runner/docs

## Before a run

```bash
maestro-runner doctor      # what the toolchain is missing, and how to fix it
maestro-runner devices     # devices it can drive now (--json for scripts, --all for shut-down sims)
```

## Run flows

```bash
maestro-runner test flow.yaml                     # Android, the default platform
maestro-runner test flows/                        # every flow in a folder
maestro-runner --platform ios test flow.yaml      # iOS simulator or iPhone
maestro-runner --platform web test flow.yaml      # desktop Chrome (headless; --headed to watch)
maestro-runner --device <serial-or-udid> test flow.yaml
maestro-runner --app-file app.apk test flows/     # install the app first (.apk, .app, .ipa)
maestro-runner test -e USER=test -e PASS flows/   # flow variables; KEY alone takes it from the shell
maestro-runner test --include-tags smoke flows/
maestro-runner test --parallel 2 flows/           # spread flows over 2 devices
maestro-runner test --retries 1 flows/            # rerun failures in the same run
maestro-runner test --retry-failed flows/         # run only what failed last time
```

Global flags (`--platform`, `--device`, `--driver`, `--app-file`, `--team-id`) go before `test`.

Drivers (`--driver`): Android `devicelab` (default), `uiautomator2`, `appium`;
iOS `devicelab` (default), `wda`, `appium`; web `cdp`. Real iPhones need
`--team-id <Apple team id>` to sign the agent; simulators do not.
Cloud grids: `--driver appium --appium-url <hub> --caps caps.json`.

## Look at the screen

```bash
maestro-runner --platform android hierarchy --compact       # one element per line
maestro-runner --platform ios hierarchy --find "Sign in"    # only matching elements
maestro-runner --platform android screenshot out.png
maestro-runner lint flows/                                  # syntax check, no device
```

Read the hierarchy before writing selectors: use the `id` or `text` it shows, never guessed ones.

## Flow format

```yaml
appId: com.example.app
tags: [smoke]
---
- launchApp:
    clearState: true
- tapOn: "Sign in"                    # text selector: a regex matched against the whole text
- tapOn:
    id: "email"                       # resource-id, accessibilityIdentifier or React Native testID
- inputText: ${USER}
- tapOn: { id: "password" }
- inputText: ${PASS}
- hideKeyboard
- tapOn: { text: "Continue", below: { id: "password" } }
- assertVisible: "Welcome.*"
- scrollUntilVisible:
    element: { text: "Settings" }
    direction: DOWN
- runFlow: common/logout.yaml
```

Selectors: `text`, `id`, `index`, state (`enabled`, `checked`, `selected`, `focused`),
relative (`below`, `above`, `leftOf`, `rightOf`, `childOf`, `containsChild`),
`point: "50%, 90%"`. Web adds `css`, `testId`, `role`, `placeholder`, `href`.

Common commands: `launchApp`, `stopApp`, `clearState`, `tapOn`, `longPressOn`, `doubleTapOn`,
`inputText`, `eraseText`, `pressKey`, `back`, `hideKeyboard`, `swipe`, `scroll`,
`scrollUntilVisible`, `assertVisible`, `assertNotVisible`, `assertTrue`, `extendedWaitUntil`,
`waitForAnimationToEnd`, `copyTextFrom`, `pasteText`, `openLink`, `takeScreenshot`,
`assertScreenshot`, `runFlow`, `runScript`, `evalScript`, `repeat`, `retry`, `setPermissions`.
Conditions: `runFlow: { when: { visible: "X" }, file: f.yaml }` or `commands:` inline.

Timeouts: `commandTimeout` and `waitForIdleTimeout` (ms) in the flow header;
`--wait-for-idle-timeout 0` turns the idle wait off for animated screens.

## Reports

Each run writes to `./reports/<timestamp>/` (or `--output dir`, `--flatten` for no subfolder):
`report.json` (index), `flows/*.json` (per-step status, duration, errors), `report.html`,
`junit-report.xml`, `allure-results/`. On failure, read the failed step's `error` in the
flow JSON and its screenshot and hierarchy under `assets/` before changing the flow.

## Working rules

- Prefer `id` over `text` when the app sets ids; text changes with copy and locale.
- Do not add `- wait` or sleeps: steps already wait for their element up to the command timeout.
- `inputText` types into the focused field; tap the field first, `eraseText` to clear it.
- Keep secrets in `-e` / `--env-file`, never literally in a flow.
- A flow that passes once may be flaky: run it a few times before calling it fixed.
