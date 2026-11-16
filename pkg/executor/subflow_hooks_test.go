package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// runFiles writes files to a temp dir, parses main, runs it with cfg against
// a driver that records each step it gets, and returns the result, the steps
// the driver got and the report folder. The driver fails a tapOn on
// "Missing", and one on "Flaky" the first time only.
func runFiles(t *testing.T, files map[string]string, main string, cfg RunnerConfig) (*RunResult, []flow.Step, string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := flow.ParseFile(filepath.Join(dir, main))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var seen []flow.Step
	flaky := 0
	driver := &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		seen = append(seen, step)
		if tap, ok := step.(*flow.TapOnStep); ok {
			switch tap.Selector.Text {
			case "Missing":
				return &core.CommandResult{Success: false, Error: errors.New("element not found"), Message: "Element not found: Missing"}
			case "Flaky":
				if flaky++; flaky == 1 {
					return &core.CommandResult{Success: false, Error: errors.New("element not found"), Message: "Element not found: Flaky"}
				}
			}
		}
		return &core.CommandResult{Success: true}
	}}
	cfg.OutputDir = filepath.Join(dir, "report")
	cfg.Artifacts = ArtifactNever
	cfg.Device = report.Device{ID: "test", Platform: "android"}
	result, err := New(driver, cfg).Run(context.Background(), []flow.Flow{*f})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return result, seen, cfg.OutputDir
}

// typed lists the taps and typed texts the driver got, in order.
func typed(steps []flow.Step) []string {
	var out []string
	for _, step := range steps {
		switch s := step.(type) {
		case *flow.TapOnStep:
			out = append(out, "tapOn "+s.Selector.Text)
		case *flow.InputTextStep:
			out = append(out, "inputText "+s.Text)
		}
	}
	return out
}

// hooked is a flow file with its own onFlowStart and onFlowComplete around
// steps.
func hooked(start, steps, complete string) string {
	return "appId: x\nonFlowStart:\n" + start + "onFlowComplete:\n" + complete + "---\n" + steps
}

// A file reached by runFlow or retry runs its own onFlowStart before its
// steps and its onFlowComplete after them, as Maestro's runSubFlow runs them
// (Orchestra.kt); only its env: was applied. A failing onFlowStart skips the
// steps, failing steps still run onFlowComplete, and a failing hook fails the
// runFlow. Maestro also skips the steps when onFlowStart changed nothing on
// screen (an assertion, a failed optional step): here they run.
func TestRunFlowFileRunsItsOwnHooks(t *testing.T) {
	start := "  - tapOn: part start\n"
	complete := "  - tapOn: part end\n"
	for _, c := range []struct {
		name, main, part string
		want             report.Status
		seen             string
		error            string
	}{
		{"runFlow, twice", "- runFlow: part.yaml\n- runFlow: part.yaml\n- tapOn: after\n", hooked(start, "- tapOn: inside\n", complete),
			report.StatusPassed, "tapOn part start, tapOn inside, tapOn part end, tapOn part start, tapOn inside, tapOn part end, tapOn after", ""},
		{"retry", "- retry:\n    maxRetries: 1\n    file: part.yaml\n- tapOn: after\n", hooked(start, "- tapOn: inside\n", complete),
			report.StatusPassed, "tapOn part start, tapOn inside, tapOn part end, tapOn after", ""},
		{"a retry runs the hooks again after a failed attempt", "- retry:\n    maxRetries: 2\n    file: part.yaml\n- tapOn: after\n",
			hooked(start, "- tapOn: Flaky\n", complete), report.StatusPassed,
			"tapOn part start, tapOn Flaky, tapOn part end, tapOn part start, tapOn Flaky, tapOn part end, tapOn after", ""},
		{"a failing step still runs onFlowComplete", "- runFlow: part.yaml\n- tapOn: after\n", hooked(start, "- tapOn: Missing\n- tapOn: skipped\n", complete),
			report.StatusFailed, "tapOn part start, tapOn Missing, tapOn part end", "Missing"},
		{"a failing onFlowStart skips the steps", "- runFlow: part.yaml\n- tapOn: after\n", hooked("  - tapOn: Missing\n", "- tapOn: inside\n", complete),
			report.StatusFailed, "tapOn Missing, tapOn part end", "onFlowStart failed"},
		{"a failing onFlowComplete fails the runFlow", "- runFlow: part.yaml\n- tapOn: after\n", hooked(start, "- tapOn: inside\n", "  - tapOn: Missing\n"),
			report.StatusFailed, "tapOn part start, tapOn inside, tapOn Missing", "onFlowComplete failed"},
		{"an optional hook step that fails lets the steps run", "- runFlow: part.yaml\n- tapOn: after\n",
			hooked("  - tapOn:\n      text: Missing\n      optional: true\n", "- tapOn: inside\n", complete), report.StatusPassed,
			"tapOn Missing, tapOn inside, tapOn part end, tapOn after", ""},
		{"a runFlow whose when: is false runs no hook", "- runFlow:\n    file: part.yaml\n    when:\n      true: ${false}\n- tapOn: after\n",
			hooked(start, "- tapOn: inside\n", complete), report.StatusPassed, "tapOn after", ""},
		{"inline commands have no hooks", "- runFlow:\n    commands:\n      - tapOn: inline\n- tapOn: after\n", "",
			report.StatusPassed, "tapOn inline, tapOn after", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			files := map[string]string{"main.yaml": "appId: x\n---\n" + c.main}
			if c.part != "" {
				files["part.yaml"] = c.part
			}
			result, seen, _ := runFiles(t, files, "main.yaml", RunnerConfig{})
			if result.Status != c.want {
				t.Fatalf("status = %s, want %s: %+v", result.Status, c.want, result.FlowResults)
			}
			if got := strings.Join(typed(seen), ", "); got != c.seen {
				t.Errorf("the driver got\n  %s\nwant\n  %s", got, c.seen)
			}
			if c.error != "" && !strings.Contains(result.FlowResults[0].Error, c.error) {
				t.Errorf("error = %q, want it to name %q", result.FlowResults[0].Error, c.error)
			}
		})
	}
}
