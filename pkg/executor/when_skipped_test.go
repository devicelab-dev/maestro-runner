package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// A runFlow or runScript whose when: is false is skipped, and the report says
// so, at the top level and inside a runFlow, as Maestro's does (its
// CommandSkipped). It used to say passed, like a step that ran.
func TestWhenFalseIsReportedSkipped(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("set.js", "output.runs = (output.runs || 0) + 1;\n")
	write("sub.yaml", `appId: x
---
- tapOn: Sub
`)
	write("main.yaml", `appId: x
---
- evalScript: ${output.mode = 'local'}
- runScript:
    file: set.js
    when:
      true: ${output.mode == 'ci'}
- runFlow:
    file: sub.yaml
    when:
      true: ${output.mode == 'ci'}
- runFlow:
    commands:
      - runScript:
          file: set.js
          when:
            true: ${output.mode == 'ci'}
      - runFlow:
          when:
            true: ${output.mode == 'ci'}
          commands:
            - tapOn: Inline
      - runScript:
          file: set.js
- runScript:
    file: set.js
    when:
      true: ${output.mode == 'local'}
`)
	f, err := flow.ParseFile(filepath.Join(dir, "main.yaml"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := t.TempDir()
	var seen []flow.Step
	runner := New(realDriverRejects(&seen), RunnerConfig{
		OutputDir: out,
		Artifacts: ArtifactNever,
		Device:    report.Device{ID: "test", Platform: "android"},
	})
	result, err := runner.Run(context.Background(), []flow.Flow{*f})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != report.StatusPassed {
		t.Fatalf("status = %s, want passed: %+v", result.Status, result.FlowResults)
	}
	if len(seen) != 0 {
		t.Errorf("the device got %d steps, want none (both taps are skipped)", len(seen))
	}
	detail, err := report.ReadFlowDetail(filepath.Join(out, "flows", "flow-000.json"))
	if err != nil {
		t.Fatal(err)
	}
	var walk func([]report.Command) string
	walk = func(cmds []report.Command) string {
		var s []string
		for _, c := range cmds {
			e := string(c.Status)
			if len(c.SubCommands) > 0 {
				e += "[" + walk(c.SubCommands) + "]"
			}
			s = append(s, e)
		}
		return strings.Join(s, ",")
	}
	got := walk(detail.Commands)
	want := "passed,skipped,skipped,passed[skipped,skipped,passed],passed"
	if got != want {
		t.Errorf("statuses = %s, want %s", got, want)
	}
	// Two scripts skipped; the runFlows do not count themselves.
	if fr := result.FlowResults[0]; fr.StepsSkipped != 2 || fr.StepsPassed != 3 {
		t.Errorf("steps passed %d, skipped %d; want 3 and 2", fr.StepsPassed, fr.StepsSkipped)
	}
}
