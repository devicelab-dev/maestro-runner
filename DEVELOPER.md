# Developer Guide

This guide explains the architecture of maestro-runner and how to extend it.

## Architecture Overview

maestro-runner follows a 3-part design:

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│    YAML     │────▶│   Executor  │────▶│   Report    │
│   Parser    │     │   (Driver)  │     │  Generator  │
└─────────────┘     └─────────────┘     └─────────────┘
     flow/              core/              report/
```

1. **YAML Parser** (`pkg/flow`) - Parses Maestro flow files into typed step structures
2. **Executor** (`pkg/core`, `pkg/executor`) - Executes steps via Driver implementations
3. **Report** (`pkg/report`) - Generates test reports (JUnit, JSON)

## Package Overview

| Package | Purpose |
|---------|---------|
| `pkg/cli` | Command-line interface and argument parsing |
| `pkg/config` | Configuration file loading (`config.yaml`) |
| `pkg/core` | Execution model: Driver, Result, Status, Artifacts |
| `pkg/executor` | Driver implementations (Appium, Native, Detox) |
| `pkg/flow` | YAML parsing, Step types, Selectors |
| `pkg/report` | Test report generation |
| `pkg/validator` | Pre-execution flow validation |

## Key Interfaces

### Driver Interface

The `Driver` interface (`pkg/core/driver.go`) is the abstraction for executing commands on devices:

```go
type Driver interface {
    // Execute runs a single step and returns the result
    Execute(step flow.Step) *CommandResult

    // Screenshot captures the current screen as PNG
    Screenshot() ([]byte, error)

    // Hierarchy captures the UI hierarchy as JSON
    Hierarchy() ([]byte, error)

    // GetState returns the current device/app state
    GetState() *StateSnapshot

    // GetPlatformInfo returns device/platform information
    GetPlatformInfo() *PlatformInfo
}
```

### Step Interface

All flow steps implement the `Step` interface (`pkg/flow/step.go`):

```go
type Step interface {
    Type() StepType
    IsOptional() bool
    Label() string
    Describe() string
}
```

## How to Add a New Driver

To add support for a new execution backend (e.g., Detox, Espresso):

### 1. Create the driver package

```
pkg/executor/detox/
├── driver.go       # Driver implementation
├── driver_test.go  # Tests
├── commands.go     # Command implementations
└── session.go      # Session management
```

### 2. Implement the Driver interface

```go
package detox

import (
    "github.com/devicelab-dev/maestro-runner/pkg/core"
    "github.com/devicelab-dev/maestro-runner/pkg/flow"
)

type Driver struct {
    // driver state
}

func New(config Config) (*Driver, error) {
    // initialize driver
}

func (d *Driver) Execute(step flow.Step) *core.CommandResult {
    switch s := step.(type) {
    case *flow.TapOnStep:
        return d.executeTap(s)
    case *flow.InputTextStep:
        return d.executeInputText(s)
    // ... handle other step types
    default:
        return &core.CommandResult{
            Success: false,
            Error:   fmt.Errorf("unsupported step: %s", step.Type()),
        }
    }
}

func (d *Driver) Screenshot() ([]byte, error) {
    // capture screenshot
}

func (d *Driver) Hierarchy() ([]byte, error) {
    // capture UI hierarchy
}

func (d *Driver) GetState() *core.StateSnapshot {
    // return current state
}

func (d *Driver) GetPlatformInfo() *core.PlatformInfo {
    // return platform info
}
```

### 3. Register the driver

Add the driver to the executor factory (when implemented).

## How to Add a New Step Type

To add support for a new Maestro command:

### 1. Add the step type constant

In `pkg/flow/step.go`:

```go
const (
    // ... existing types
    StepMyNewCommand StepType = "myNewCommand"
)
```

### 2. Create the step struct

```go
type MyNewCommandStep struct {
    BaseStep `yaml:",inline"`
    // Step-specific fields
    Target   string `yaml:"target"`
    Duration int    `yaml:"duration"`
}

func (s *MyNewCommandStep) Describe() string {
    return fmt.Sprintf("myNewCommand on %s", s.Target)
}
```

### 3. Add parsing logic

In `pkg/flow/parser.go`, add to `parseStep()`:

```go
case "myNewCommand":
    step := &MyNewCommandStep{
        BaseStep: BaseStep{StepType: StepMyNewCommand},
    }
    if err := mapstructure.Decode(value, step); err != nil {
        return nil, err
    }
    return step, nil
```

### 4. Add driver implementation

In each driver, add handling for the new step type in `Execute()`.

### 5. Add tests

- Parser test in `pkg/flow/parser_test.go`
- Driver test in `pkg/executor/<driver>/<driver>_test.go`

## How to Add a New Report Format

### 1. Create the reporter

In `pkg/report/`:

```go
type HTMLReporter struct {
    outputPath string
}

func (r *HTMLReporter) Generate(result *core.SuiteResult) error {
    // generate HTML report
}
```

### 2. Register the format

Add to the report factory/CLI options.

## Result Model

The execution produces a hierarchy of results:

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

## Design Principles

1. **Executor-agnostic** - Appium, Native, Detox are equal implementations
2. **Configurable timeouts** - Flow, step, and command level
3. **Small, focused components** - No god classes
4. **Independent parts** - Changes in one don't affect others
5. **KISS and DRY** - Keep it simple, don't repeat yourself

## Common Tasks

### Debug flow parsing

```go
f, err := flow.ParseFile("test.yaml")
if err != nil {
    log.Fatal(err)
}
for _, step := range f.Steps {
    fmt.Printf("%s: %s\n", step.Type(), step.Describe())
}
```

### Validate flows programmatically

```go
v := validator.New(includeTags, excludeTags)
result := v.Validate("./tests/")
if !result.IsValid() {
    for _, err := range result.Errors {
        fmt.Println(err)
    }
}
```

## Questions?

Open an issue or check existing discussions.
