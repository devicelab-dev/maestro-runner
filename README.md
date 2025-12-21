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

### Validate flows

```bash
maestro-runner validate ./tests/
```

### Run tests

```bash
maestro-runner run ./tests/ --platform ios --device "iPhone 15"
```

### With tag filtering

```bash
maestro-runner run ./tests/ --include-tags smoke --exclude-tags slow
```

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
