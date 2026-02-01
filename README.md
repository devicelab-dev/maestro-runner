<div align="center">

# maestro-runner

---

**Fast mobile UI test automation for Android, iOS, React Native, Flutter & Expo**
<br>
*Open-source Maestro alternative — single binary, no JVM. 100% free, no features behind a paywall.*
<br>
*Supports real iOS devices, simulators, emulators, and cloud providers.*

![3.6x faster](https://img.shields.io/badge/3.6x_faster-3a9d5c?style=for-the-badge) ![14x less memory](https://img.shields.io/badge/14x_less_memory-3a9d5c?style=for-the-badge)

[![license](https://img.shields.io/badge/license-Apache_2.0-blue.svg?style=for-the-badge)](LICENSE)
[![by](https://img.shields.io/badge/by-DeviceLab.dev-17a2b8.svg?style=for-the-badge)](https://devicelab.dev)

[![CI](https://github.com/devicelab-dev/maestro-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/devicelab-dev/maestro-runner/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/devicelab-dev/maestro-runner/branch/main/graph/badge.svg)](https://codecov.io/gh/devicelab-dev/maestro-runner)
[![Go Report Card](https://goreportcard.com/badge/github.com/devicelab-dev/maestro-runner?v=2)](https://goreportcard.com/report/github.com/devicelab-dev/maestro-runner)

<b><a href="https://open.devicelab.dev/maestro-runner">Documentation</a></b> | <b><a href="#install">Get Started</a></b> | <b><a href="https://open.devicelab.dev/maestro-runner/docs/cli-reference">CLI Reference</a></b> | <b><a href="https://open.devicelab.dev/maestro-runner/docs/flow-commands">Flow Commands</a></b> | <b><a href="CONTRIBUTING.md">Contributing</a></b>

</div>

---

- Runs Maestro YAML flows on real devices, emulators, and simulators
- Supports Android (UIAutomator2), iOS (WebDriverAgent), and cloud (Appium)
- Built-in parallel execution, HTML/JUnit/Allure reports, and JavaScript scripting
- Addresses [78% of the top 100 most-discussed open issues](docs/maestro-issues-analysis.md) on Maestro's GitHub

## Install

```bash
curl -fsSL https://open.devicelab.dev/install/maestro-runner | bash
```

## Run Tests

```bash
maestro-runner test flow.yaml                                           # Android (default)
maestro-runner --platform ios test flow.yaml                            # iOS
maestro-runner --app-file app.apk test flows/                           # Install app and run
maestro-runner --driver appium --appium-url <server-url> test flow.yaml # Appium
maestro-runner test --parallel 3 flows/                                 # Parallel on 3 devices
```

## Key Features

- **Zero migration** — Runs your existing Maestro YAML flows as-is, no changes needed
- **Real iOS device testing** — Supports physical iOS devices, not just simulators
- **Cloud testing** — BrowserStack, Sauce Labs, LambdaTest via Appium driver
- **React Native & Flutter** — Smart element finding for RN testIDs and Flutter semantics
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
| **WDA (WebDriverAgent)** | iOS | Auto-selected with `--platform ios`. Supports simulators and physical devices. |
| **Appium** | Android & iOS | `--driver appium`. For cloud testing providers and existing Appium infrastructure. |

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

## Requirements

- **Android testing:** `adb` (Android SDK Platform-Tools)
- **iOS testing:** Xcode command-line tools (`xcrun`)
- **Cloud & Appium testing:** Appium 2.x or 3.x — works with local Appium servers and cloud providers (BrowserStack, Sauce Labs, LambdaTest)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 — see [LICENSE](LICENSE).

