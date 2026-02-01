<div align="center">

# maestro-runner

---

**Fast mobile UI test automation for Android & iOS**
<br>
*Open-source Maestro alternative — single binary, no JVM. 100% free, no features behind a paywall.*

![3.6x faster](https://img.shields.io/badge/3.6x_faster-brightgreen?style=for-the-badge) ![14x less memory](https://img.shields.io/badge/14x_less_memory-brightgreen?style=for-the-badge)

[![license](https://img.shields.io/badge/license-Apache_2.0-blue.svg?style=for-the-badge)](LICENSE)
[![by](https://img.shields.io/badge/by-DeviceLab.dev-17a2b8.svg?style=for-the-badge)](https://devicelab.dev)

[![CI](https://github.com/devicelab-dev/maestro-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/devicelab-dev/maestro-runner/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/devicelab-dev/maestro-runner/branch/main/graph/badge.svg)](https://codecov.io/gh/devicelab-dev/maestro-runner)
[![Go Report Card](https://goreportcard.com/badge/github.com/devicelab-dev/maestro-runner)](https://goreportcard.com/report/github.com/devicelab-dev/maestro-runner)

<b><a href="https://devicelab.dev/open-source/maestro-runner">Documentation</a></b> | <b><a href="#install">Get Started</a></b> | <b><a href="docs/cli-reference.md">CLI Reference</a></b> | <b><a href="docs/flow-commands.md">Flow Commands</a></b> | <b><a href="CONTRIBUTING.md">Contributing</a></b>

</div>

---

- Runs Maestro YAML flows on real devices, emulators, and simulators
- Supports Android (UIAutomator2), iOS (WebDriverAgent), and cloud (Appium)
- Built-in parallel execution, HTML/JUnit/Allure reports, and JavaScript scripting
- Addresses [78% of the top 100 most-discussed open issues](docs/maestro-issues-analysis.md) on Maestro's GitHub

## Install

```bash
go install github.com/devicelab-dev/maestro-runner@latest
```

Or download a pre-built binary from [releases](https://devicelab.dev/open-source/maestro-runner).

## Run Tests

```bash
maestro-runner flow.yaml                              # Android (default)
maestro-runner --platform ios flow.yaml               # iOS
maestro-runner flows/                                 # All flows in a directory
maestro-runner --driver appium flow.yaml              # Appium (local or cloud)
maestro-runner --parallel 3 flows/                    # Parallel on 3 devices
```

## Key Features

- **Reliable text input** — Direct ADB input with Unicode support, no dropped characters
- **Fast element finding** — Native selectors, clickable parent traversal, regex matching, smarter visibility
- **Configurable timeouts** — Per-command and per-flow timeouts, `--wait-for-idle-timeout 0` to disable
- **Parallel execution** — Dynamic ports, multiple device instances on one machine
- **Lightweight** — ~21 MB binary vs ~289 MB JVM footprint
- **Cloud testing** — BrowserStack, Sauce Labs, LambdaTest via Appium driver
- **Reports** — HTML, JUnit XML, and Allure-compatible reports out of the box

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
- **Cloud testing:** Appium server 2.x (`npm i -g appium`)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 — see [LICENSE](LICENSE).

