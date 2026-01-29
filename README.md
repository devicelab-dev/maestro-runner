# maestro-runner

**3.6x faster** · **14x less memory** · **single binary, no JVM** · your existing [Maestro](https://maestro.mobile.dev/) YAML files work as-is — with the features and fixes Maestro hasn't delivered.

## Quick Start

### Install

```bash
go install github.com/devicelab-dev/maestro-runner@latest
```

### Run

```bash
maestro-runner flow.yaml                              # Android (default)
maestro-runner flow.yaml --platform ios               # iOS
maestro-runner --driver appium flow.yaml              # Appium (local or cloud)
```

## Why maestro-runner?

If you've hit any of these, maestro-runner fixes them:

- **`inputText` drops characters** → direct ADB input, reliable Unicode support
- **Tests are slow** → native element selectors, no polling, configurable idle timeouts
- **Can't configure timeouts** → per-command and per-flow, `--wait-for-idle-timeout 0` to disable
- **Can't run parallel** → dynamic ports, multiple instances on one machine
- **JVM eats memory in CI** → ~21 MB Go binary vs ~289 MB JVM
- **No cloud support** → BrowserStack, Sauce Labs, LambdaTest via Appium
- **Elements not found reliably** → clickable parent traversal, native `textMatches()` regex, smarter visibility checks

Addresses [78% of the top 100 most-discussed open issues](docs/maestro-issues-analysis.md) on Maestro's GitHub.

### Requirements

- Go 1.22+
- **UIAutomator2 driver:** `adb` (Android SDK Platform-Tools)
- **Appium driver:** Appium server 2.x (`npm i -g appium`)
- **WDA driver:** Xcode command-line tools (`xcrun`)

## Flow Config

maestro-runner adds two fields to the standard Maestro flow config header that Maestro doesn't support:

```yaml
commandTimeout: 10000       # Default per-command timeout (ms)
waitForIdleTimeout: 3000    # Device idle wait (ms), 0 to disable
---
- launchApp: com.example.app
- tapOn: "Login"
```

| Field | Description |
|-------|-------------|
| `commandTimeout` | Override the default element-find timeout for all commands in this flow (ms) |
| `waitForIdleTimeout` | Override the device idle wait for this flow (ms, `0` to disable) |

## Drivers

### UIAutomator2 (default)

Direct connection to Android devices via UIAutomator2. No external server needed.

```bash
maestro-runner flow.yaml
maestro-runner flow.yaml --device emulator-5554
```

Automatically installs UIAutomator2 APKs from `./apks/` if present.

### Appium

Connects to an Appium 2.x server. Supports local devices and cloud providers.

```bash
# Local
appium &
maestro-runner --driver appium flow.yaml

# With capabilities file
maestro-runner --driver appium --caps caps.json flow.yaml
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

```bash
maestro-runner --driver appium \
  --appium-url "https://your-cloud-hub/wd/hub" \
  --caps caps.json \
  flow.yaml
```

See [Appium capabilities docs](https://appium.io/docs/en/latest/guides/caps/) for the full list of options.

### WDA (iOS)

Uses WebDriverAgent for iOS simulators.

```bash
maestro-runner --platform ios flow.yaml
maestro-runner --platform ios --device "iPhone 15" flow.yaml
```

## CLI Flags (maestro-runner specific)

| Flag | Default | Description |
|------|---------|-------------|
| `--driver, -d` | `uiautomator2` | Driver: `uiautomator2`, `appium` |
| `--appium-url` | `http://127.0.0.1:4723` | Appium server URL |
| `--caps` | | Path to Appium capabilities JSON |
| `--app-file` | | App binary to install before testing |
| `--wait-for-idle-timeout` | `5000` | Device idle wait in ms (0 to disable) |
| `--team-id` | | Apple Development Team ID for WDA code signing (iOS) |

All standard Maestro flags (`--platform`, `--device`, `--env`, `--include-tags`, `--exclude-tags`, etc.) are also supported. Run `maestro-runner --help` for the full list.

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
