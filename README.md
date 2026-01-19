# maestro-runner

A Go-based test runner for [Maestro](https://maestro.mobile.dev/) mobile UI testing flows.

## Features

- Parse and validate Maestro YAML flow files
- Execute flows on iOS and Android devices
- Multiple executor backends (Appium, Native, Detox)
- Configurable timeouts at flow, step, and command levels
- JUnit/JSON test reports
- Tag-based test filtering

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

## Quick Start

### Run tests on Android (UIAutomator2 - default)

```bash
maestro-runner test flow.yaml
maestro-runner test flows/ --device emulator-5554
```

### Run tests on iOS

```bash
maestro-runner test flow.yaml --platform ios --device "iPhone 15"
```

### With tag filtering

```bash
maestro-runner test flows/ --include-tags smoke --exclude-tags slow
```

### With environment variables

```bash
maestro-runner test flows/ -e USER=test -e PASS=secret
```

## Using Appium

### Local Appium Server

```bash
# Start Appium server first
appium

# Run with Appium driver
maestro-runner --driver appium test flow.yaml
maestro-runner --driver appium --device emulator-5554 test flow.yaml
```

### With Capabilities File

Create a capabilities JSON file for more control:

```json
{
  "platformName": "Android",
  "appium:automationName": "UiAutomator2",
  "appium:deviceName": "emulator-5554",
  "appium:app": "/path/to/app.apk",
  "appium:platformVersion": "14"
}
```

```bash
maestro-runner --driver appium --caps caps.json test flow.yaml
```

**Priority:** CLI flags override capabilities file values:
- `--platform` overrides `platformName`
- `--device` overrides `appium:deviceName`
- `--app-file` overrides `appium:app`

### Cloud Providers

#### BrowserStack

```json
{
  "platformName": "Android",
  "appium:automationName": "UiAutomator2",
  "appium:platformVersion": "13.0",
  "appium:deviceName": "Samsung Galaxy S23",
  "appium:app": "bs://app-id",
  "bstack:options": {
    "userName": "YOUR_USERNAME",
    "accessKey": "YOUR_ACCESS_KEY",
    "projectName": "My Project",
    "buildName": "Build 1"
  }
}
```

```bash
maestro-runner --driver appium \
  --appium-url "https://hub-cloud.browserstack.com/wd/hub" \
  --caps browserstack.json \
  test flow.yaml
```

#### Sauce Labs

```json
{
  "platformName": "Android",
  "appium:automationName": "UiAutomator2",
  "appium:platformVersion": "13",
  "appium:deviceName": "Google Pixel 7",
  "appium:app": "storage:filename=app.apk",
  "sauce:options": {
    "username": "YOUR_USERNAME",
    "accessKey": "YOUR_ACCESS_KEY",
    "name": "Test Run"
  }
}
```

```bash
maestro-runner --driver appium \
  --appium-url "https://ondemand.us-west-1.saucelabs.com:443/wd/hub" \
  --caps saucelabs.json \
  test flow.yaml
```

#### LambdaTest

```json
{
  "platformName": "Android",
  "appium:automationName": "UiAutomator2",
  "appium:platformVersion": "13",
  "appium:deviceName": "Pixel 7",
  "appium:app": "lt://APP_ID",
  "lt:options": {
    "username": "YOUR_USERNAME",
    "accessKey": "YOUR_ACCESS_KEY",
    "project": "My Project",
    "build": "Build 1"
  }
}
```

```bash
maestro-runner --driver appium \
  --appium-url "https://mobile-hub.lambdatest.com/wd/hub" \
  --caps lambdatest.json \
  test flow.yaml
```

## CLI Reference

### Global Flags

| Flag | Env Var | Description |
|------|---------|-------------|
| `--platform, -p` | `MAESTRO_PLATFORM` | Platform: ios, android |
| `--device, --udid` | `MAESTRO_DEVICE` | Device ID (comma-separated for multiple) |
| `--driver, -d` | `MAESTRO_DRIVER` | Driver: uiautomator2 (default), appium |
| `--appium-url` | `APPIUM_URL` | Appium server URL (default: http://127.0.0.1:4723) |
| `--caps` | `APPIUM_CAPS` | Path to Appium capabilities JSON file |
| `--app-file` | `MAESTRO_APP_FILE` | App binary (.apk, .app, .ipa) to install |
| `--verbose` | `MAESTRO_VERBOSE` | Enable verbose logging |

### Test Command Flags

| Flag | Description |
|------|-------------|
| `--env, -e` | Environment variables (KEY=VALUE, repeatable) |
| `--include-tags` | Only run flows with these tags |
| `--exclude-tags` | Skip flows with these tags |
| `--output` | Output directory for reports (default: ./reports) |
| `--flatten` | Don't create timestamp subfolder |
| `--continuous, -c` | Enable continuous mode |

## Configuration

Create a `config.yaml` in your test directory:

```yaml
flows:
  - "**/*.yaml"

includeTags:
  - smoke

excludeTags:
  - wip

appId: com.example.app
```

## Documentation

- [Developer Guide](DEVELOPER.md) - Architecture and how to extend
- [Contributing](CONTRIBUTING.md) - How to contribute
- [Changelog](CHANGELOG.md) - Version history

## Requirements

- Go 1.21+
- For Appium executor: Appium server 2.x
- For Native executor: Platform-specific tools (xcrun, adb)

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
