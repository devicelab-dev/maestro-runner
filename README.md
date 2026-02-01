# maestro-runner — Fast Mobile UI Test Automation for Android & iOS

A lightweight, open-source alternative to [Maestro](https://maestro.mobile.dev/) for mobile UI testing. Single Go binary — no JVM, no Maestro Cloud dependency. Runs your existing Maestro YAML flows as-is, **3.6x faster** with **14x less memory**.

Addresses [78% of the top 100 most-discussed open issues](docs/maestro-issues-analysis.md) on Maestro's GitHub.

## Install

```bash
go install github.com/devicelab-dev/maestro-runner@latest
```

Or download a pre-built binary from [releases](https://devicelab.dev/open-source/maestro-runner).

## Run Tests

```bash
maestro-runner flow.yaml                              # Android (default)
maestro-runner flow.yaml --platform ios               # iOS
maestro-runner flows/                                 # All flows in a directory
maestro-runner --driver appium flow.yaml              # Appium (local or cloud)
maestro-runner --parallel 3 flows/                    # Parallel on 3 devices
```

## Why Switch from Maestro?

| Problem with Maestro | maestro-runner fix |
|---|---|
| `inputText` drops characters | Direct ADB input, reliable Unicode support |
| Tests are slow | Native element selectors, no polling, configurable idle timeouts |
| Can't configure timeouts | Per-command and per-flow timeouts, `--wait-for-idle-timeout 0` to disable |
| No parallel test execution | Dynamic ports, multiple device instances on one machine |
| JVM eats memory in CI/CD | ~21 MB Go binary vs ~289 MB JVM footprint |
| No cloud provider support | BrowserStack, Sauce Labs, LambdaTest via Appium driver |
| Elements not found reliably | Clickable parent traversal, native regex matching, smarter visibility |

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

## Documentation

| Document | Description |
|----------|-------------|
| **[CLI Reference](docs/cli-reference.md)** | Commands, flags, environment variables, tag filtering, parallel execution, emulator & simulator management |
| **[Flow Commands](docs/flow-commands.md)** | Complete YAML command reference — selectors, tap & gesture, text input, assertions, app lifecycle, flow control, JavaScript scripting |
| **[Technical Approach](docs/technical-approach.md)** | Driver architecture, element finding strategy, UIAutomator2 & WDA server lifecycles, report system |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
