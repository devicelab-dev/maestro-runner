## Use maestro-runner as a Go library

maestro-runner is a Go module, not only a CLI. You can import it and drive a device, run Maestro flows, and read structured results directly from your own Go program — no separate binary to install, no shelling out, no protocol version skew. The device runner is embedded in the module and version-locked to the exact commit you pin, so a build that compiles is a build that runs.

Use this when you are building a service, a test orchestrator, or a tool that needs to run flows itself: a device farm scheduler, a CI runner, a dashboard that executes flows on demand.

For clients that are not written in Go, drive the same device runner over HTTP instead: `maestro-runner server` exposes sessions, JSON steps, screenshots, and the UI hierarchy with no Go dependency — see [rest-api.md](rest-api.md).

## Install

```bash
go get github.com/devicelab-dev/maestro-runner
```

The module requires Go 1.25 or newer. Running iOS flows still needs the platform toolchain on the host (Xcode and a booted simulator, or a connected device); Android needs the Android SDK platform-tools. That is the same requirement as the CLI — the library does not remove the need for the underlying device tooling, it removes the need for a separate maestro-runner install.

## The short path: one driver, many flows

The intended entry point is `cli.CreateDriver`, which builds and starts the driver once, and `cli.ExecuteFlowWithDriver`, which runs a single flow on that driver with full output — console progress, a JSON report, and a screenshot on failure. Reuse the one driver across every flow so the device session and the running runner are shared, exactly as a `test` run over a directory would.

```go
package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/cli"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func main() {
	cfg := &cli.RunConfig{
		Platform: "ios",          // "ios", "android", or "web"
		Driver:   "wda",          // see the driver list below
		Devices:  []string{"00008101-001C0C660A13001E"},
		AppFile:  "/path/to/App.app",
		AppID:    "com.example.app",
		TeamID:   "A3RCAA2YAX",   // Apple Development Team ID (iOS/WDA only)
	}

	driver, cleanup, err := cli.CreateDriver(cfg)
	if err != nil {
		log.Fatalf("create driver: %v", err)
	}
	defer cleanup() // stops the on-device runner / emulator session

	flowPaths := []string{"flows/login.yaml", "flows/checkout.yaml"}
	for _, path := range flowPaths {
		f, err := flow.ParseFile(path)
		if err != nil {
			log.Fatalf("parse %s: %v", path, err)
		}

		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		cfg.OutputDir = "./reports/" + name // one report directory per flow

		result, err := cli.ExecuteFlowWithDriver(driver, cfg, *f)
		if err != nil {
			log.Fatalf("run %s: %v", name, err)
		}
		fmt.Printf("%s: %d/%d flows passed\n", name, result.PassedFlows, result.TotalFlows)
	}
}
```

`cleanup` is not optional. It stops the on-device runner and, when maestro-runner started an emulator or simulator for you, shuts it down. Defer it as soon as `CreateDriver` succeeds.

## Choosing a driver

`Platform` and `Driver` together select how the device is driven. The valid `Driver` values per platform match the CLI's `--driver` flag:

- **Android:** `devicelab` (default since v1.1.28), `uiautomator2`, or `appium`.
- **iOS:** `devicelab` (default since v1.1.28: the prebuilt DeviceLab iOS agent, simulators and real iPhones; `TeamID` only on a real iPhone) or `wda` (WebDriverAgent; needs `TeamID` set, on simulator as well as device). `devicelab-legacy` selects the previous DeviceLab runner, kept for comparison.
- **Web:** leave `Driver` empty; set `Headed`, `Browser`, and `WindowSize` on the config instead.

For Appium, set `AppiumURL` and either `CapsFile` or a parsed `Capabilities` map.

## The config

`cli.RunConfig` is the same struct the CLI fills from its flags, so anything the CLI can do is reachable here. The fields you will most often set:

| Field | Purpose |
|---|---|
| `Platform`, `Driver` | select the device backend (above) |
| `Devices` | one or more device UDIDs / serials |
| `AppFile`, `AppID` | app binary to install and its bundle id / package. `AppFile` may be a local path or an http(s) URL (downloaded and cached; a `.zip` is unpacked to the `.app`/`.ipa` inside; set `AppFileSHA256` to verify it). `AppID` is optional: when left empty the runner takes it from the first flow in the suite that declares an `appId` (or a web `url`), so a suite may lead with a setup flow that declares none. |
| `TeamID` | Apple Development Team ID (iOS) |
| `OutputDir` | where the JSON, HTML, JUnit, and Allure reports are written |
| `Env` | variables exposed to `${...}` in flows |
| `IncludeTags`, `ExcludeTags` | filter which flows in a suite run |
| `WaitForIdleTimeout`, `StepDelay`, `TypingFrequency` | timing knobs |
| `StartSimulator`, `StartEmulator`, `AutoStartEmulator`, `ShutdownAfter` | boot a device for the run and tear it down after |

Set `OutputDir` per flow (as in the example) if you want one report directory per flow; set it once if you want them to share a directory.

## Reading the result

`ExecuteFlowWithDriver` returns an `*executor.RunResult`:

```go
import "github.com/devicelab-dev/maestro-runner/pkg/report"

result, _ := cli.ExecuteFlowWithDriver(driver, cfg, *f)

if result.Status == report.StatusPassed {
	// every flow passed
}
fmt.Printf("passed=%d failed=%d skipped=%d total=%d took=%dms\n",
	result.PassedFlows, result.FailedFlows, result.SkippedFlows,
	result.TotalFlows, result.Duration)

for _, fr := range result.FlowResults {
	fmt.Printf("  %s: %s\n", fr.ID, fr.Status)
}
```

`RunResult.Status` is `report.StatusPassed` or `report.StatusFailed`; `FlowResults` carries the per-flow outcome. The same JSON, HTML, JUnit, and Allure reports the CLI writes are produced under `OutputDir`, so an existing report pipeline keeps working unchanged.

## Parsing flows from other sources

`flow.ParseFile(path)` reads a YAML file. If your flows live in memory — generated, fetched from a database, received over an API — use `flow.Parse(data, sourcePath)` instead; `sourcePath` is used only for relative `runFlow` resolution and error messages, and may be empty.

```go
f, err := flow.Parse(yamlBytes, "")
```

A parsed flow exposes `f.SourcePath`, `f.Config` (the `appId`, `name`, and tags from the YAML header), and `f.Steps`. The flow's declared name is `f.Config.Name`; it is whatever the YAML `name:` says, and empty when the flow does not set one, so derive report directory names from the file path as the example above does.

## Lower level: the executor directly

`ExecuteFlowWithDriver` is a convenience wrapper that also wires console output and the cloud hooks. If you want to run a batch of flows with no console printing and assemble reporting yourself, construct the runner directly:

```go
import (
	"context"

	"github.com/devicelab-dev/maestro-runner/pkg/executor"
)

runner := executor.New(driver, executor.RunnerConfig{
	OutputDir: "./reports",
	Env:       cfg.Env,
	// OnFlowStart / OnStepComplete / OnFlowEnd callbacks let you stream progress
})
result, err := runner.Run(context.Background(), []flow.Flow{*f})
```

`RunnerConfig` exposes `OnFlowStart`, `OnStepComplete`, `OnNestedStep`, and `OnFlowEnd` callbacks, which is how you stream live progress into your own UI or logs instead of the terminal.

## Version locking

Because the device runner ships inside the module, the runner your program uses is the one pinned in your `go.mod` — there is no globally installed maestro-runner to drift out of sync, and no protocol mismatch between the host code and the on-device runner. Upgrading is a `go get -u` and a rebuild.

## API stability

The library surface used above — `cli.CreateDriver`, `cli.ExecuteFlowWithDriver`, `cli.RunConfig`, `flow.Parse` / `flow.ParseFile`, `executor.New` and `executor.RunnerConfig`, and the `pkg/driver/*` setup functions — is the supported embedding API. Treat everything else as internal and subject to change.
