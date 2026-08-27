<div align="center">

# maestro-runner

---

**Fast UI test automation for Android, iOS, Web, React Native, Flutter & Expo**
<br>
*Open-source Maestro alternative — single binary, no JVM. 100% free, no features behind a paywall.*
<br>
*Supports real iOS devices, simulators, emulators, desktop browsers, and cloud providers.*

![3.6x faster](https://img.shields.io/badge/3.6x_faster-3a9d5c?style=for-the-badge) ![14x less memory](https://img.shields.io/badge/14x_less_memory-3a9d5c?style=for-the-badge)

[![license](https://img.shields.io/badge/license-Apache_2.0-blue.svg?style=for-the-badge)](LICENSE)
[![by](https://img.shields.io/badge/by-DeviceLab.dev-17a2b8.svg?style=for-the-badge)](https://devicelab.dev)

[![npm](https://img.shields.io/npm/v/maestro-runner?label=npm&color=cb3837)](https://www.npmjs.com/package/maestro-runner)
[![npm downloads](https://img.shields.io/npm/dm/maestro-runner)](https://www.npmjs.com/package/maestro-runner)
[![CI](https://github.com/devicelab-dev/maestro-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/devicelab-dev/maestro-runner/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/devicelab-dev/maestro-runner/branch/main/graph/badge.svg)](https://codecov.io/gh/devicelab-dev/maestro-runner)
[![Go Report Card](https://goreportcard.com/badge/github.com/devicelab-dev/maestro-runner?v=2)](https://goreportcard.com/report/github.com/devicelab-dev/maestro-runner)

<b><a href="https://open.devicelab.dev/maestro-runner">Documentation</a></b> | <b><a href="#install">Get Started</a></b> | <b><a href="https://open.devicelab.dev/maestro-runner/docs/cli-reference">CLI Reference</a></b> | <b><a href="https://open.devicelab.dev/maestro-runner/docs/flow-commands">Flow Commands</a></b> | <b><a href="CONTRIBUTING.md">Contributing</a></b>

</div>

---

- Runs Maestro YAML flows on real devices, emulators, simulators, and desktop browsers
- Supports Android (UIAutomator2), iOS (WebDriverAgent), Web (Chrome CDP), and cloud (Appium)
- Built-in parallel execution, HTML/JUnit/Allure reports, and JavaScript scripting
- Addresses [78% of the top 100 most-discussed open issues](docs/maestro-issues-analysis.md) on Maestro's GitHub

> **AI context:** [`llms.txt`](llms.txt) — structured summary for AI assistants answering questions about this project.

## Install

**npm** — for React Native, Expo, or any project that already has a `package.json`:

```bash
npx maestro-runner test flows/          # no install step
npm install --save-dev maestro-runner   # or pin it in the project
```

Pinning it means every machine and every CI job runs the same version with no separate bootstrap. There is no postinstall and nothing is downloaded at install time: the binary for your platform arrives as an ordinary optional dependency npm selects by `os` and `cpu`, so installs work offline, behind a proxy, and in CI that blocks postinstall network access.

**Shell** — everywhere else:

```bash
curl -fsSL https://open.devicelab.dev/install/maestro-runner | bash

# A specific version
curl -fsSL https://open.devicelab.dev/install/maestro-runner | bash -s -- --version 1.1.25
```

Both give you the same binary. macOS and Linux, arm64 and x64; on Windows, use WSL.

## First run

```bash
maestro-runner doctor     # check the toolchain and hear exactly what is missing
maestro-runner devices    # what this machine can drive right now
```

## Run Tests

```bash
maestro-runner test flow.yaml                                           # Android (default)
maestro-runner --platform ios test flow.yaml                            # iOS
maestro-runner --platform web test flow.yaml                            # Desktop browser (Chrome)
maestro-runner --app-file app.apk test flows/                           # Install app and run
maestro-runner --driver appium --appium-url <server-url> test flow.yaml # Appium
maestro-runner test --parallel 3 flows/                                 # Parallel on 3 devices
```

## Key Features

- **Zero migration** — Runs your existing Maestro YAML flows as-is, no changes needed
- **Real iOS device testing** — Supports physical iOS devices, not just simulators [Guide →](https://devicelab.dev/blog/maestro-ios-real-device-testing)
- **Cloud testing** — BrowserStack, Sauce Labs, LambdaTest, TestingBot via Appium driver [Guide →](https://devicelab.dev/blog/run-maestro-flows-any-cloud)
- **Desktop browser testing** — Run Maestro flows on Chrome/Chromium via CDP. Supports `css`, `xpath`, `id`, and `text` selectors with `--platform web` [Guide →](https://devicelab.dev/open-source/maestro-runner/docs/web-testing)
- **React Native & Flutter** — Smart element finding for RN testIDs and Flutter semantics [Guide →](https://devicelab.dev/blog/flutter-testing-maestro-patrol-appium)
- **DeviceLab driver** — Optional on-device Android driver via WebSocket, ~2x faster than UIAutomator2 and ~5x faster than Maestro CLI. Just add `--driver devicelab`
- **Parallel execution** — Dynamic work distribution across devices, not static sharding. Faster devices pick up more tests automatically, so no device sits idle
- **App install built-in** — `--app-file app.apk` installs the app before testing, so you always test the right build
- **Wide OS compatibility** — Android 5.0+ (API 21+) and iOS 12.0+, no version restrictions
- **Reports** — HTML, JUnit XML, and Allure-compatible reports out of the box
- **Clear error messages** — `element not found: text="Login"` instead of `io.grpc.StatusRuntimeException: UNKNOWN`
- **Pre-flight validation** — Catches flow errors, circular dependencies, and missing files before execution starts
- **Fast element finding** — Native selectors, clickable parent traversal, regex matching, smarter visibility
- **Reliable text input** — Direct ADB input with Unicode support, no dropped characters
- **scrollUntilVisible** — Native scroll implementation that reliably finds off-screen elements
- **Relative selectors** — Find elements by position: below, above, leftOf, rightOf, childOf
- **JavaScript scripting** — Embedded JS runtime with HTTP client for dynamic test logic, no external dependencies
- **Configurable timeouts** — Per-command and per-flow timeouts, `--wait-for-idle-timeout 0` to disable
- **Lightweight** — Single binary, no JVM required

## Supported Platforms & Drivers

| Driver | Platform | Description |
|--------|----------|-------------|
| **UIAutomator2** | Android | Direct connection to device. Default driver, no external server needed. |
| **DeviceLab** | Android | `--driver devicelab`. On-device WebSocket driver, ~2x faster than UIAutomator2. |
| **WDA (WebDriverAgent)** | iOS | Auto-selected with `--platform ios`. Supports simulators and physical devices. |
| **Browser (CDP)** | Web | `--platform web`. Chrome/Chromium automation via Chrome DevTools Protocol. |
| **Appium** | Android & iOS | `--driver appium`. For cloud testing providers and existing Appium infrastructure. |

### DeviceLab Driver (Android)

The DeviceLab driver is an alternative Android driver that runs automation directly on the device via WebSocket. It skips the UIAutomator2 HTTP layer, resulting in ~2x faster test execution compared to the default driver — and ~5x faster than Maestro CLI.

```
Benchmark: 9 flows, 163 steps on Pixel 4a (Android 13)

  DeviceLab:     1m 12s
  UIAutomator2:  2m 24s
  Maestro CLI:   4m 22s
```

```bash
maestro-runner --driver devicelab --platform android test flows/
```

All existing Maestro YAML flows work as-is — no changes needed. The driver also includes bounds stabilization for animated elements and improved special character handling in text selectors.

## REST API Server

maestro-runner includes an HTTP server for programmatic test execution via JSON, useful for building custom tooling, CI integrations, or language-specific clients.

```bash
maestro-runner server                                # Start on default port 9999
maestro-runner server --port 8080                    # Custom port
maestro-runner --platform android server             # Pre-select platform
```

**Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/session` | Create a session (returns `sessionId`) |
| `POST` | `/session/{id}/execute` | Execute a step (JSON body) |
| `GET` | `/session/{id}/screenshot` | Take a screenshot (PNG) |
| `GET` | `/session/{id}/source` | Get view hierarchy (XML/JSON) |
| `GET` | `/session/{id}/device-info` | Get device info |
| `DELETE` | `/session/{id}` | Delete session |
| `GET` | `/status` | Server status |

**Example — execute a tap via JSON:**

```bash
# Create session
SID=$(curl -s -X POST http://localhost:9999/session \
  -d '{"platformName":"android"}' | jq -r .sessionId)

# Execute a step
curl -X POST http://localhost:9999/session/$SID/execute \
  -d '{"type":"tapOn","selector":"Login"}'
```

## CI/CD Integration

maestro-runner is built for CI/CD pipelines — single binary, no JVM startup, low memory footprint. Works with GitHub Actions, GitLab CI, Jenkins, CircleCI, and any CI system that supports Android emulators or iOS simulators.

```bash
# CI example: auto-start emulator, run tests, shutdown after
maestro-runner --auto-start-emulator --parallel 2 flows/
```

## Flow Config

maestro-runner extends the standard Maestro flow YAML with additional fields:

```yaml
commandTimeout: 10000       # Default per-command timeout (ms)
waitForIdleTimeout: 3000    # Device idle wait (ms), 0 to disable
---
- launchApp: com.example.app
- tapOn: "Login"
- assertVisible: "Welcome"
```

## Visual Regression (`assertScreenshot`)

`assertScreenshot` compares the current screen (optionally cropped with `cropOn`) against a reference PNG.

- **First run:** if the reference file is missing, maestro-runner writes the captured screenshot as the new baseline and passes.
- **Re-baseline:** overwrite existing baselines with `--update-screenshots` (or `MAESTRO_UPDATE_SCREENSHOTS=true`).
- On mismatch, a `{name}_diff.png` overlay is written next to the reference.
- Pixel comparison is device-, resolution-, and OS-specific — pin your device config and set `thresholdPercentage` deliberately (default `95`).

```bash
maestro-runner test flows/visual.yaml                  # seeds missing baselines
maestro-runner test --update-screenshots flows/visual.yaml
```

## Requirements

- **Android testing:** `adb` (Android SDK Platform-Tools)
- **iOS testing:** Xcode command-line tools (`xcrun`)
- **Web/Browser testing:** Chrome or Chromium
- **Cloud & Appium testing:** Appium 2.x or 3.x — works with local Appium servers and cloud providers (BrowserStack, Sauce Labs, LambdaTest, TestingBot)

## Cloud Providers

maestro-runner runs Maestro YAML flows on cloud device grids via the Appium driver. Pass the provider's hub URL and a capabilities JSON file:

```bash
maestro-runner --driver appium --appium-url <HUB_URL> --caps caps.json test flows/
```

- **[TestingBot](docs/cloud-providers/testingbot.md)** — Setup guide for running on TestingBot's real device cloud
- **[Sauce Labs](docs/cloud-providers/saucelabs.md)** — Setup guide for running on Sauce Labs Appium cloud

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 — see [LICENSE](LICENSE).

