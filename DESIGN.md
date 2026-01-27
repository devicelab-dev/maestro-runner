# Design

## Overview

maestro-runner is a test runner that executes [Maestro](https://maestro.mobile.dev/) YAML flow files on multiple backends. It keeps the Maestro YAML format but replaces the execution engine with a pluggable, configurable architecture.

## Architecture

```
┌──────────────┐       ┌──────────────┐       ┌──────────────┐
│     YAML     │──────▶│    Driver    │──────▶│    Report    │
│   (parser)   │       │  (contract)  │       │  (generator) │
└──────────────┘       └──────┬───────┘       └──────────────┘
                              │
              ┌───────────────┼───────────────┐
              │               │               │
         UIAutomator2      Appium           WDA

All drivers implement the same interface.
```

### Three independent parts

**1. YAML Parser** (`pkg/flow`) — Parses Maestro flow files into typed step structures. Changes here don't affect drivers.

**2. Driver** (`pkg/core`, `pkg/driver`) — Interface that all backends implement. Adding a new driver means implementing the interface — nothing else changes.

**3. Report** (`pkg/report`) — Consumes execution results and generates reports (JSON, HTML). Changes here don't affect drivers.

### Impact matrix

| Change | Parser | Driver | Report |
|--------|--------|--------|--------|
| Add new driver (e.g., Detox) | No change | New implementation | No change |
| Change YAML syntax | Change | No change | No change |
| Add report format | No change | No change | Change |
| New command (e.g., `doubleTap`) | Parse it | All implement | No change |

## Why not just use Maestro?

Maestro is a great YAML format for mobile UI tests, but the runner has architectural issues that limit real-world usage.

### Hardcoded ports

```kotlin
// AndroidDriver.kt
private const val DefaultDriverHostPort = 7001  // No parallel execution
```

Android uses gRPC on port 7001, iOS uses HTTP on port 22087. Both hardcoded. You can't run parallel tests on the same machine.

### Hardcoded timeouts

```kotlin
// Orchestra.kt
class Orchestra(
    private val lookupTimeoutMs: Long = 17000L,
    private val optionalLookupTimeoutMs: Long = 7000L,
)
```

No way to configure these per-flow or per-command. Feature requests for configurable timeouts have been open since 2022 (#423, #684, #1252).

### Flaky text input

Character-by-character input via `pressKeyCode()`. No Unicode support. Drops and mangles characters under load.

### Monolithic orchestrator

`Orchestra.kt` is 1500+ lines with a single method handling 50+ command types via a massive `when` block. `MaestroCommand.kt` uses 35+ nullable fields instead of a type hierarchy.

### Our approach

| Maestro limitation | maestro-runner |
|---|---|
| Hardcoded ports | Dynamic ports, parallel-ready |
| Hardcoded timeouts | Configurable per-flow and per-command |
| Character-by-character input | Appium Unicode IME / direct UIAutomator2 |
| No cloud support | BrowserStack, Sauce Labs, LambdaTest via Appium |
| 1500-line god class | Small, focused components |
| Tight coupling | Parser, Driver, Report are independent |

## Design principles

1. **Driver-agnostic** — UIAutomator2, Appium, WDA are equal implementations of the same interface
2. **Configurable** — Timeouts, idle waits, and driver settings at every level
3. **Small components** — No file over a few hundred lines, no god classes
4. **Independent parts** — Changes in one part don't cascade
5. **Cloud-native** — First-class support for remote device providers
