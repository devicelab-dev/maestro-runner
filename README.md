# maestro-runner

A fast, Go-based test runner for [Maestro](https://maestro.mobile.dev/) YAML flows with pluggable driver backends.

## Why maestro-runner?

Maestro is a great format for writing mobile UI tests, but its runner has architectural limitations that hurt real-world usage:

- **Hardcoded ports** — Android gRPC on port 7001, making parallel execution on the same machine impossible
- **Hardcoded timeouts** — No way to configure wait durations at the flow, step, or command level
- **Flaky text input** — Character-by-character key presses that drop or mangle text
- **No cloud provider support** — Can't run on BrowserStack, Sauce Labs, or LambdaTest out of the box
- **Monolithic architecture** — A 1500-line orchestrator class that's hard to extend

maestro-runner is a clean-room reimplementation that keeps the Maestro YAML format but replaces the execution engine with a pluggable, configurable architecture. Write your tests in the same YAML you already know, then run them on any backend.

## Features

- **Maestro YAML compatible** — Parses and runs standard Maestro flow files (39 command types)
- **Multiple drivers** — UIAutomator2 (Android, default), Appium (Android/iOS + cloud), WDA (iOS)
- **Configurable timeouts** — At flow, step, and command levels
- **Cloud-ready** — Works with BrowserStack, Sauce Labs, LambdaTest via Appium
- **Tag-based filtering** — Include/exclude flows by tag
- **Rich reports** — JSON with real-time updates and interactive HTML reports
- **JavaScript scripting** — `evalScript`, `assertTrue`, `runScript` with full JS engine
- **Regex selectors** — Pattern matching for element text and assertions
- **Environment variables** — Pass config via CLI flags, YAML, or config file

## Installation

```bash
go install github.com/devicelab-dev/maestro-runner@latest
```

Or build from source:

```bash
git clone https://github.com/devicelab-dev/maestro-runner.git
cd maestro-runner
make build
```

### Requirements

- Go 1.22+
- **UIAutomator2 driver:** `adb` (Android SDK Platform-Tools)
- **Appium driver:** Appium server 2.x (`npm i -g appium`)
- **WDA driver:** Xcode command-line tools (`xcrun`)

## Quick Start

### 1. Write a flow

Create `login.yaml`:

```yaml
appId: com.example.app
---
- launchApp: com.example.app
- tapOn: "Email"
- inputText: "test@example.com"
- tapOn: "Password"
- inputText: "secret123"
- tapOn: "Login"
- assertVisible: "Welcome"
```

### 2. Run it

```bash
# Android (UIAutomator2 — default)
maestro-runner test login.yaml

# iOS
maestro-runner test login.yaml --platform ios

# Via Appium
maestro-runner --driver appium test login.yaml

# Run an entire folder
maestro-runner test flows/
```

### 3. View the report

Reports are written to `./reports/<timestamp>/`:

```
reports/
└── 2026-01-27_15-04-05/
    ├── report.json       # Machine-readable results
    ├── report.html       # Interactive HTML report
    ├── flows/
    │   └── flow-000.json # Per-flow command details
    └── assets/
        └── flow-000/     # Screenshots, hierarchy dumps, logs
```

## Writing Flows

Flows are YAML files with an optional config header (first YAML document) and a list of steps.

### Flow config header

```yaml
appId: com.example.app
name: Login Flow
tags:
  - smoke
  - critical
env:
  TEST_USER: demo
timeout: 60000              # Flow timeout (ms)
commandTimeout: 10000       # Default per-command timeout (ms)
waitForIdleTimeout: 3000    # Device idle wait (ms), 0 to disable
---
# Steps follow
- launchApp: com.example.app
- tapOn: "Login"
```

### Commands

#### Navigation and interaction

```yaml
- tapOn: "Login"                    # Tap element by text
- tapOn:
    id: btn_submit                  # Tap by resource/accessibility ID
- doubleTapOn: "Item"
- longPressOn: "Delete"
- tapOnPoint:                       # Tap at coordinates
    x: 200
    y: 400
- swipe:                            # Swipe gesture
    direction: UP                   # UP, DOWN, LEFT, RIGHT
    duration: 500
- scroll                            # Scroll down
- scrollUntilVisible:
    element:
      text: "Load More"
    direction: DOWN
- back                              # Press back button
- hideKeyboard
```

#### Text input

```yaml
- inputText: "hello world"
- inputText:
    text: "hello"
    selector:
      id: search_field
- eraseText: 5                      # Erase 5 characters
- copyTextFrom:
    text: "Price"                   # Copy text from element
- pasteText
- inputRandom:
    type: EMAIL                     # EMAIL, TEXT, NUMBER, PERSON_NAME
    length: 10
```

#### Assertions

```yaml
- assertVisible: "Welcome"
- assertNotVisible: "Error"
- assertTrue: "${output.status == 'ok'}"  # JavaScript condition
- assertVisible:
    text: "Order #\\d+"             # Regex pattern
- assertCondition:
    visible:
      text: "Dashboard"
    timeout: 15000                  # Custom timeout for this step
```

#### App lifecycle

```yaml
- launchApp: com.example.app
- launchApp:
    appId: com.example.app
    clearState: true                # Wipe app data before launch
    clearKeychain: true             # iOS: clear keychain
    permissions:
      notifications: allow
      location: allow
- stopApp: com.example.app
- clearState: com.example.app
```

#### JavaScript

```yaml
- evalScript: "output.counter = 1"
- assertTrue: "${output.counter == 1}"
- runScript: scripts/setup.js
```

#### Flow control

```yaml
- repeat:
    times: 3
    steps:
      - tapOn: "Next"
      - assertVisible: "Page"

- retry:
    maxRetries: 2
    steps:
      - tapOn: "Submit"
      - assertVisible: "Success"

- runFlow: other-flow.yaml          # Run another flow file
- runFlow:
    when:
      visible: "Login Screen"
    steps:                          # Or inline steps
      - tapOn: "Login"
```

#### Device control

```yaml
- setLocation:
    latitude: 37.7749
    longitude: -122.4194
- openLink: "myapp://deep/link"
- pressKey: HOME
- takeScreenshot: screenshot_name
```

### Selectors

Elements can be found by text, ID, or a combination:

```yaml
# Simple text match
- tapOn: "Login"

# By ID
- tapOn:
    id: btn_login

# Regex match
- tapOn:
    text: "Order #\\d+"

# Relative positioning
- tapOn:
    text: "Edit"
    below:
      text: "Username"

# Multiple constraints
- tapOn:
    text: "Submit"
    enabled: true
    index: "0"
```

Relative selector directions: `below`, `above`, `leftOf`, `rightOf`, `containsChild`, `containsDescendants`.

### Environment variables

```yaml
# In flow header
env:
  BASE_URL: https://api.example.com

# In steps — ${VAR} syntax
- inputText: "${TEST_USER}"
- openLink: "${BASE_URL}/login"
```

Pass from the CLI:

```bash
maestro-runner test flows/ -e USER=test -e PASS=secret
```

Or in `config.yaml`:

```yaml
env:
  USER: test
  PASS: secret
```

### Optional steps and labels

```yaml
- assertVisible:
    text: "Cookie banner"
    optional: true                  # Won't fail the flow if missing
    label: "Dismiss cookie banner"  # Custom label in reports
```

## Drivers

### UIAutomator2 (default)

Direct connection to Android devices via UIAutomator2. No external server needed.

```bash
maestro-runner test flow.yaml
maestro-runner test flow.yaml --device emulator-5554
```

Automatically installs UIAutomator2 APKs from `./apks/` if present.

### Appium

Connects to an Appium 2.x server. Supports local devices and cloud providers.

```bash
# Local
appium &
maestro-runner --driver appium test flow.yaml

# With capabilities file
maestro-runner --driver appium --caps caps.json test flow.yaml
```

#### Capabilities file

```json
{
  "platformName": "Android",
  "appium:automationName": "UiAutomator2",
  "appium:deviceName": "emulator-5554",
  "appium:app": "/path/to/app.apk"
}
```

CLI flags override capabilities: `--platform` overrides `platformName`, `--device` overrides `appium:deviceName`, `--app-file` overrides `appium:app`.

#### Cloud providers

BrowserStack:

```bash
maestro-runner --driver appium \
  --appium-url "https://hub-cloud.browserstack.com/wd/hub" \
  --caps browserstack.json \
  test flow.yaml
```

Sauce Labs:

```bash
maestro-runner --driver appium \
  --appium-url "https://ondemand.us-west-1.saucelabs.com:443/wd/hub" \
  --caps saucelabs.json \
  test flow.yaml
```

LambdaTest:

```bash
maestro-runner --driver appium \
  --appium-url "https://mobile-hub.lambdatest.com/wd/hub" \
  --caps lambdatest.json \
  test flow.yaml
```

See [Appium capabilities docs](https://appium.io/docs/en/latest/guides/caps/) for the full list of options.

### WDA (iOS)

Uses WebDriverAgent for iOS simulators.

```bash
maestro-runner --platform ios test flow.yaml
maestro-runner --platform ios --device "iPhone 15" test flow.yaml
```

## Configuration

Create `config.yaml` in your test directory for shared settings:

```yaml
# Flow selection
flows:
  - "**/*.yaml"

# Tag filtering
includeTags:
  - smoke
excludeTags:
  - wip

# Environment
env:
  API_URL: https://staging.example.com

# Timeouts
waitForIdleTimeout: 3000    # ms, 0 to disable
```

**Priority** (highest wins): CLI flags > config.yaml > capabilities file > defaults.

## CLI Reference

### Global flags

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--platform, -p` | `MAESTRO_PLATFORM` | `android` | Target platform: `android`, `ios` |
| `--device, --udid` | `MAESTRO_DEVICE` | auto-detect | Device ID (comma-separated for multiple) |
| `--driver, -d` | `MAESTRO_DRIVER` | `uiautomator2` | Driver: `uiautomator2`, `appium` |
| `--appium-url` | `APPIUM_URL` | `http://127.0.0.1:4723` | Appium server URL |
| `--caps` | `APPIUM_CAPS` | | Path to Appium capabilities JSON |
| `--app-file` | `MAESTRO_APP_FILE` | | App binary to install before testing |
| `--verbose` | `MAESTRO_VERBOSE` | `false` | Enable verbose logging |
| `--no-ansi` | | `false` | Disable colored output |

### `test` command

```bash
maestro-runner test [flags] <flow-or-folder>...
```

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `config.yaml` | Path to workspace config |
| `--env, -e` | | Environment variable `KEY=VALUE` (repeatable) |
| `--include-tags` | | Only run flows matching these tags |
| `--exclude-tags` | | Skip flows matching these tags |
| `--output` | `./reports` | Report output directory |
| `--flatten` | `false` | Don't create timestamp subfolder |
| `--continuous, -c` | `false` | Continuous mode (re-run on change) |
| `--wait-for-idle-timeout` | `5000` | Device idle wait in ms (0 to disable) |

## Architecture

```
YAML Parser ──> Executor ──> Report Generator
  (pkg/flow)    (pkg/executor)   (pkg/report)
                    │
        ┌───────────┼───────────┐
        │           │           │
  UIAutomator2    Appium       WDA
```

Each part is independent. See [DEVELOPER.md](DEVELOPER.md) for details on adding new drivers, commands, or report formats.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
