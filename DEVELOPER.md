# Developer Guide

How the code is organized and how to extend it.

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│    YAML     │────▶│   Executor  │────▶│   Report    │
│   Parser    │     │   (Driver)  │     │  Generator  │
└─────────────┘     └─────────────┘     └─────────────┘
     flow/         core/ + driver/         report/
```

1. **YAML Parser** (`pkg/flow`) — Parses Maestro flow files into typed step structures
2. **Executor** (`pkg/executor`) — Orchestrates flow execution, delegates commands to Driver implementations
3. **Report** (`pkg/report`) — Generates test reports (JSON, HTML)

## Packages

| Package | Purpose |
|---------|---------|
| `pkg/cli` | CLI commands and argument parsing |
| `pkg/config` | Configuration file loading (`config.yaml`) |
| `pkg/core` | Core types: Driver interface, CommandResult, Status, Artifacts |
| `pkg/device` | Android device management via ADB |
| `pkg/driver/appium` | Appium driver (Android/iOS, local and cloud) |
| `pkg/driver/uiautomator2` | UIAutomator2 driver (Android, direct) |
| `pkg/driver/wda` | WebDriverAgent driver (iOS) |
| `pkg/driver/mock` | Mock driver for testing |
| `pkg/executor` | Flow runner — orchestrates step execution and callbacks |
| `pkg/flow` | YAML parsing, Step types, Selectors |
| `pkg/jsengine` | JavaScript evaluation engine (evalScript, assertTrue) |
| `pkg/report` | JSON and HTML report generation |
| `pkg/uiautomator2` | UIAutomator2 HTTP protocol client |
| `pkg/validator` | Pre-execution flow validation and tag filtering |

## Key Interfaces

### Driver (`pkg/core/driver.go`)

The abstraction all backends implement:

```go
type Driver interface {
    Execute(step flow.Step) *CommandResult
    Screenshot() ([]byte, error)
    Hierarchy() ([]byte, error)
    GetState() *StateSnapshot
    GetPlatformInfo() *PlatformInfo
    SetFindTimeout(ms int)
    SetWaitForIdleTimeout(ms int) error
}
```

### Step (`pkg/flow/step.go`)

All flow steps implement:

```go
type Step interface {
    Type() StepType
    IsOptional() bool
    Label() string
    Describe() string
}
```

## How to Add a New Driver

### 1. Create the driver package

```
pkg/driver/mydriver/
├── driver.go       # Driver implementation
├── driver_test.go  # Tests
└── commands.go     # Command implementations
```

### 2. Implement the Driver interface

```go
package mydriver

import (
    "github.com/devicelab-dev/maestro-runner/pkg/core"
    "github.com/devicelab-dev/maestro-runner/pkg/flow"
)

type Driver struct {
    findTimeout int
}

func (d *Driver) Execute(step flow.Step) *core.CommandResult {
    switch s := step.(type) {
    case *flow.TapOnStep:
        return d.executeTap(s)
    case *flow.InputTextStep:
        return d.executeInputText(s)
    default:
        return &core.CommandResult{
            Success: false,
            Error:   fmt.Errorf("unsupported step: %s", step.Type()),
        }
    }
}

func (d *Driver) Screenshot() ([]byte, error)       { /* capture screenshot */ }
func (d *Driver) Hierarchy() ([]byte, error)         { /* capture UI hierarchy */ }
func (d *Driver) GetState() *core.StateSnapshot      { /* return current state */ }
func (d *Driver) GetPlatformInfo() *core.PlatformInfo { /* return platform info */ }
func (d *Driver) SetFindTimeout(ms int)              { d.findTimeout = ms }
func (d *Driver) SetWaitForIdleTimeout(ms int) error  { return nil }
```

### 3. Register the driver

Wire it up in `pkg/cli/test.go` where drivers are selected based on the `--driver` flag.

## How to Add a New Step Type

### 1. Add the step type constant in `pkg/flow/step.go`

```go
const (
    StepMyNewCommand StepType = "myNewCommand"
)
```

### 2. Create the step struct

```go
type MyNewCommandStep struct {
    BaseStep `yaml:",inline"`
    Target   string `yaml:"target"`
}

func (s *MyNewCommandStep) Describe() string {
    return fmt.Sprintf("myNewCommand on %s", s.Target)
}
```

### 3. Add parsing in `pkg/flow/parser.go`

Add a case to `parseStep()` for the new command key.

### 4. Add driver handling

In each driver's `Execute()` method, add a `case *flow.MyNewCommandStep` branch.

### 5. Add tests

- Parser test in `pkg/flow/parser_test.go`
- Driver test in `pkg/driver/<driver>/driver_test.go`

## Result Model

```
SuiteResult
├── Flows []FlowResult
│   ├── PlatformInfo
│   ├── OnFlowStart []StepResult
│   ├── Steps []StepResult
│   │   ├── Status (passed/failed/skipped/warned/errored)
│   │   ├── Duration
│   │   ├── Error
│   │   ├── Attachments (screenshots, hierarchy)
│   │   └── SubFlowResult (for runFlow steps)
│   └── OnFlowComplete []StepResult
└── Summary (passed/failed/skipped counts)
```

## Status Values

| Status | Meaning |
|--------|---------|
| `pending` | Not yet executed |
| `running` | Currently executing |
| `passed` | Completed successfully |
| `failed` | Assertion/expectation failed |
| `errored` | Unexpected error occurred |
| `skipped` | Skipped (condition not met) |
| `warned` | Passed with warnings |

## Speed Gate

`tools/speedgate` fails when a command type got slower. It reads the reports of
repeated runs of one suite (same device, same driver), takes the median duration
of each command type over every passed step (container steps such as `repeat`
count their children, not themselves), and compares it with
`benchmarks/speed-baseline.json`. A type regresses when its median is slower by
more than **both** `--threshold-pct` (default 20) and `--threshold-ms` (default
50). Types with fewer than `--min-samples` (default 3) passed steps are not
judged. Exit status: 0 no regression, 1 regression, 2 bad input or no baseline.

The baseline is written by hand from real runs, since it needs a device:

```bash
make build
for i in 1 2 3; do
  maestro-runner --device <SERIAL> test --output /tmp/sg/$i \
    /Users/omnarayan/work/temp/TestHive/maestro-tests/auth-suite.yaml
done
make speedgate REPORTS="/tmp/sg/1 /tmp/sg/2 /tmp/sg/3" WRITE=1   # write the baseline
make speedgate REPORTS="/tmp/sg/1 /tmp/sg/2 /tmp/sg/3"           # compare against it
```

A baseline holds for the device, driver and suite it came from; the tool refuses
to pool reports of different drivers or platforms, and warns when the baseline's
differ from the reports'. Say which device and suite in the commit that adds or
updates the baseline. Run it directly for other paths or thresholds:
`go run ./tools/speedgate --baseline <file> --threshold-pct 30 <report-dir>...`.
