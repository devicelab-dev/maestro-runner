package executor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// A runFlow inside a retry is in the report with its own steps. It used to
// drop out, with them, because the nested list was restored after it was
// appended to.
func TestRunFlowInsideRetryIsReported(t *testing.T) {
	var seen []flow.Step
	tap := func(id string) *flow.TapOnStep {
		return &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}, Selector: flow.Selector{ID: id}}
	}
	dir := t.TempDir()
	runner := New(realDriverRejects(&seen), RunnerConfig{
		OutputDir: dir,
		Artifacts: ArtifactNever,
		Device:    report.Device{ID: "test", Platform: "android"},
	})
	f := flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "retry with runFlow"},
		Steps: []flow.Step{
			&flow.RetryStep{
				BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
				MaxRetries: "1",
				Steps: []flow.Step{
					tap("before"),
					&flow.RunFlowStep{
						BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
						Steps:    []flow.Step{tap("inside")},
					},
					tap("after"),
				},
			},
		},
	}
	if _, err := runner.Run(context.Background(), []flow.Flow{f}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	detail, err := report.ReadFlowDetail(filepath.Join(dir, "flows", "flow-000.json"))
	if err != nil {
		t.Fatal(err)
	}
	retry := detail.Commands[0]
	var types []string
	for _, c := range retry.SubCommands {
		types = append(types, c.Type)
	}
	if len(retry.SubCommands) != 3 || retry.SubCommands[1].Type != string(flow.StepRunFlow) {
		t.Fatalf("retry's steps = %v, want [tapOn runFlow tapOn]", types)
	}
	if inner := retry.SubCommands[1].SubCommands; len(inner) != 1 || inner[0].Type != string(flow.StepTapOn) {
		t.Errorf("runFlow's steps = %v, want one tapOn", inner)
	}
}

// console.log output is kept on the step whose script printed it, nested or
// not (#192).
func TestConsoleLogsAreReportedPerStep(t *testing.T) {
	var seen []flow.Step
	eval := func(script string) *flow.EvalScriptStep {
		return &flow.EvalScriptStep{BaseStep: flow.BaseStep{StepType: flow.StepEvalScript}, Script: script}
	}
	dir := t.TempDir()
	runner := New(realDriverRejects(&seen), RunnerConfig{
		OutputDir: dir,
		Artifacts: ArtifactNever,
		Device:    report.Device{ID: "test", Platform: "android"},
	})
	f := flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "console"},
		Steps: []flow.Step{
			eval(`${console.log("top")}`),
			&flow.RunFlowStep{
				BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
				Steps:    []flow.Step{eval(`${console.log("nested")}`)},
			},
			&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}, Selector: flow.Selector{ID: "x"}},
		},
	}
	if _, err := runner.Run(context.Background(), []flow.Flow{f}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	detail, err := report.ReadFlowDetail(filepath.Join(dir, "flows", "flow-000.json"))
	if err != nil {
		t.Fatal(err)
	}
	cmds := detail.Commands
	if got := cmds[0].Logs; len(got) != 1 || got[0] != "top" {
		t.Errorf("top-level step logs = %q, want [top]", got)
	}
	if sub := cmds[1].SubCommands; len(sub) != 1 || len(sub[0].Logs) != 1 || sub[0].Logs[0] != "nested" {
		t.Errorf("nested step = %+v, want logs [nested]", sub)
	}
	if len(cmds[1].Logs) != 0 || len(cmds[2].Logs) != 0 {
		t.Errorf("runFlow logs %q, tapOn logs %q: want none", cmds[1].Logs, cmds[2].Logs)
	}
}

// A runFlow's steps are in the report while they run, each as it starts and
// as it ends, the nested runFlow's too: they used to arrive with its end.
func TestRunFlowStepsAreReportedAsTheyRun(t *testing.T) {
	dir := t.TempDir()
	var shape func(cs []report.Command) string
	shape = func(cs []report.Command) string {
		var out []string
		for _, c := range cs {
			s := c.Type + ":" + string(c.Status)
			if len(c.SubCommands) > 0 {
				s += "[" + shape(c.SubCommands) + "]"
			}
			out = append(out, s)
		}
		return strings.Join(out, ",")
	}
	seen := map[string]string{} // a tap's id → its runFlow's steps in the report as it ran
	driver := &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		if tap, ok := step.(*flow.TapOnStep); ok {
			d, err := report.ReadFlowDetail(filepath.Join(dir, "flows", "flow-000.json"))
			if err != nil {
				t.Errorf("read the report while %s runs: %v", tap.Selector.ID, err)
			} else {
				seen[tap.Selector.ID] = shape(d.Commands[0].SubCommands)
			}
		}
		return &core.CommandResult{Success: true}
	}}
	tap := func(id string) *flow.TapOnStep {
		return &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}, Selector: flow.Selector{ID: id}}
	}
	runner := New(driver, RunnerConfig{
		OutputDir: dir,
		Artifacts: ArtifactNever,
		Device:    report.Device{ID: "test", Platform: "android"},
	})
	f := flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "live steps"},
		Steps: []flow.Step{
			&flow.RunFlowStep{
				BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
				Steps: []flow.Step{
					tap("a"),
					&flow.RunFlowStep{BaseStep: flow.BaseStep{StepType: flow.StepRunFlow}, Steps: []flow.Step{tap("b")}},
					tap("c"),
				},
			},
		},
	}
	if _, err := runner.Run(context.Background(), []flow.Flow{f}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := map[string]string{
		"a": "tapOn:running",
		"b": "tapOn:passed,runFlow:running[tapOn:running]",
		"c": "tapOn:passed,runFlow:passed[tapOn:passed],tapOn:running",
	}
	for id, w := range want {
		if seen[id] != w {
			t.Errorf("while %s ran the report had %q, want %q", id, seen[id], w)
		}
	}
	d, err := report.ReadFlowDetail(filepath.Join(dir, "flows", "flow-000.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got, w := shape(d.Commands[0].SubCommands), "tapOn:passed,runFlow:passed[tapOn:passed],tapOn:passed"; got != w {
		t.Errorf("at the end the report has %q, want %q", got, w)
	}
}
