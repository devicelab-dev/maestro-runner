package executor

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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

// shapeOf is a report's commands as type:status, with their sub-commands in
// brackets.
func shapeOf(cs []report.Command) string {
	var out []string
	for _, c := range cs {
		s := c.Type + ":" + string(c.Status)
		if len(c.SubCommands) > 0 {
			s += "[" + shapeOf(c.SubCommands) + "]"
		}
		out = append(out, s)
	}
	return strings.Join(out, ",")
}

// waitForSubs reads the flow's file until its first command's sub-commands
// have the shape want, for at most within; it returns the last shape read.
func waitForSubs(path, want string, within time.Duration) (string, error) {
	deadline := time.Now().Add(within)
	for {
		d, err := report.ReadFlowDetail(path)
		got := ""
		if err == nil && len(d.Commands) > 0 {
			got = shapeOf(d.Commands[0].SubCommands)
		}
		if got == want || time.Now().After(deadline) {
			return got, err
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func liveTap(id string) *flow.TapOnStep {
	return &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}, Selector: flow.Selector{ID: id}}
}

func liveRunFlow(steps ...flow.Step) *flow.RunFlowStep {
	return &flow.RunFlowStep{BaseStep: flow.BaseStep{StepType: flow.StepRunFlow}, Steps: steps}
}

// A runFlow's steps are in the report while they run, each as it starts and
// as it ends, the nested runFlow's too: they used to arrive with its end.
// The file is written at most every report.LiveWriteInterval, so each step
// waits for its state to reach the file.
func TestRunFlowStepsAreReportedAsTheyRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flows", "flow-000.json")
	want := map[string]string{
		"a": "tapOn:running",
		"b": "tapOn:passed,runFlow:running[tapOn:running]",
		"c": "tapOn:passed,runFlow:passed[tapOn:passed],tapOn:running",
	}
	seen := map[string]string{} // a tap's id → its runFlow's steps in the report as it ran
	driver := &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		if tap, ok := step.(*flow.TapOnStep); ok {
			got, err := waitForSubs(path, want[tap.Selector.ID], 5*time.Second)
			if err != nil {
				t.Errorf("read the report while %s runs: %v", tap.Selector.ID, err)
			}
			seen[tap.Selector.ID] = got
		}
		return &core.CommandResult{Success: true}
	}}
	runner := New(driver, RunnerConfig{
		OutputDir: dir,
		Artifacts: ArtifactNever,
		Device:    report.Device{ID: "test", Platform: "android"},
	})
	f := flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "live steps"},
		Steps:      []flow.Step{liveRunFlow(liveTap("a"), liveRunFlow(liveTap("b")), liveTap("c"))},
	}
	if _, err := runner.Run(context.Background(), []flow.Flow{f}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for id, w := range want {
		if seen[id] != w {
			t.Errorf("while %s ran the report had %q, want %q", id, seen[id], w)
		}
	}
	d, err := report.ReadFlowDetail(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, w := shapeOf(d.Commands[0].SubCommands), "tapOn:passed,runFlow:passed[tapOn:passed],tapOn:passed"; got != w {
		t.Errorf("at the end the report has %q, want %q", got, w)
	}
}

// Steps that end quickly before one that runs long are in the file while it
// runs, a runFlow that ended among them too: the last state is written once
// the interval is up, not only at the next step. And the top-level runFlow's
// end is in the file at once, for the step after it.
func TestQuickStepsReachTheFileWhileALongStepRuns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flows", "flow-000.json")
	var whileLong, atNext string
	var waited time.Duration
	driver := &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		tap, ok := step.(*flow.TapOnStep)
		if !ok {
			return &core.CommandResult{Success: true}
		}
		switch tap.Selector.ID {
		case "long":
			start := time.Now()
			got, err := waitForSubs(path, "runFlow:passed[tapOn:passed],tapOn:running", 5*time.Second)
			if err != nil {
				t.Errorf("read the report while the long step runs: %v", err)
			}
			whileLong, waited = got, time.Since(start)
		case "next":
			d, err := report.ReadFlowDetail(path)
			if err != nil {
				t.Errorf("read the report at the next step: %v", err)
				break
			}
			atNext = string(d.Commands[0].Status) + "[" + shapeOf(d.Commands[0].SubCommands) + "]"
		}
		return &core.CommandResult{Success: true}
	}}
	runner := New(driver, RunnerConfig{
		OutputDir: dir,
		Artifacts: ArtifactNever,
		Device:    report.Device{ID: "test", Platform: "android"},
	})
	f := flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "quick then long"},
		Steps:      []flow.Step{liveRunFlow(liveRunFlow(liveTap("quick")), liveTap("long")), liveTap("next")},
	}
	if _, err := runner.Run(context.Background(), []flow.Flow{f}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if w := "runFlow:passed[tapOn:passed],tapOn:running"; whileLong != w {
		t.Errorf("while the long step ran the report had %q after %v, want %q", whileLong, waited, w)
	}
	if w := "passed[runFlow:passed[tapOn:passed],tapOn:passed]"; atNext != w {
		t.Errorf("at the next step the runFlow was %q, want %q", atNext, w)
	}
}

// A long repeat writes the flow's file at most every report.LiveWriteInterval
// while it runs, not at each of its steps' starts and ends: with 2 steps ×
// 1000 that was about 4,000 writes of a file that grows to 0.7 MB, a cost
// that grew with the square of the steps. The same for a repeat of a
// runFlow.
func TestLongRepeatWritesTheFlowFileAtMostEveryInterval(t *testing.T) {
	for _, tc := range []struct {
		name  string
		steps []flow.Step
		want  string // the repeat's first two steps in the report at the end
		count int    // its steps in the report at the end
	}{
		{"two taps", []flow.Step{liveTap("a"), liveTap("b")}, "tapOn:passed,tapOn:passed", 2000},
		{"a runFlow", []flow.Step{liveRunFlow(liveTap("a"))}, "runFlow:passed[tapOn:passed],runFlow:passed[tapOn:passed]", 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var w *report.FlowWriter
			orig := newFlowWriter
			newFlowWriter = func(d *report.FlowDetail, outputDir string, index *report.IndexWriter) *report.FlowWriter {
				w = orig(d, outputDir, index)
				return w
			}
			t.Cleanup(func() { newFlowWriter = orig })
			driver := &mockDriver{executeFunc: func(flow.Step) *core.CommandResult { return &core.CommandResult{Success: true} }}
			runner := New(driver, RunnerConfig{
				OutputDir: dir,
				Artifacts: ArtifactNever,
				Device:    report.Device{ID: "test", Platform: "android"},
			})
			f := flow.Flow{
				SourcePath: "test.yaml",
				Config:     flow.Config{Name: "long repeat"},
				Steps: []flow.Step{&flow.RepeatStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRepeat},
					Times:    strconv.Itoa(1000),
					Steps:    tc.steps,
				}},
			}
			start := time.Now()
			if _, err := runner.Run(context.Background(), []flow.Flow{f}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			took := time.Since(start)
			// The flow's own writes (its start, the repeat's start and end,
			// its end) and one live write per interval at most.
			if limit := 10 + int(took/report.LiveWriteInterval); w.Writes() > limit {
				t.Errorf("the flow's file was written %d times in %v, want at most %d", w.Writes(), took, limit)
			}
			d, err := report.ReadFlowDetail(filepath.Join(dir, "flows", "flow-000.json"))
			if err != nil {
				t.Fatal(err)
			}
			subs := d.Commands[0].SubCommands
			if len(subs) != tc.count || shapeOf(subs[:2]) != tc.want {
				t.Errorf("at the end the repeat has %d steps starting %q, want %d starting %q", len(subs), shapeOf(subs[:min(2, len(subs))]), tc.count, tc.want)
			}
		})
	}
}

// A step that panics skips the flow's end, so its writer is stopped when Run
// returns: a runFlow's steps left waiting for CommandSubs' timer are not
// written to the flow's file and the index after the run ended, nor is a
// list given later.
func TestNoLiveWriteAfterAStepPanics(t *testing.T) {
	dir := t.TempDir()
	var w *report.FlowWriter
	orig := newFlowWriter
	newFlowWriter = func(d *report.FlowDetail, outputDir string, index *report.IndexWriter) *report.FlowWriter {
		w = orig(d, outputDir, index)
		return w
	}
	t.Cleanup(func() { newFlowWriter = orig })
	driver := &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		if tap, ok := step.(*flow.TapOnStep); ok && tap.Selector.ID == "boom" {
			panic("nil pointer dereference in a dependency")
		}
		return &core.CommandResult{Success: true}
	}}
	runner := New(driver, RunnerConfig{
		OutputDir: dir,
		Artifacts: ArtifactNever,
		Device:    report.Device{ID: "test", Platform: "android"},
	})
	f := flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "panic in a runFlow"},
		Steps:      []flow.Step{liveRunFlow(liveTap("a"), liveTap("boom"))},
	}
	result, err := runner.Run(context.Background(), []flow.Flow{f})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != report.StatusFailed {
		t.Fatalf("status = %v, want failed", result.Status)
	}
	after := w.Writes()
	time.Sleep(2 * report.LiveWriteInterval)
	if got := w.Writes() - after; got != 0 {
		t.Errorf("the flow's file was written %d times after Run returned, want 0", got)
	}
	w.CommandSubs(0, func() []report.Command { return nil })
	time.Sleep(2 * report.LiveWriteInterval)
	if got := w.Writes() - after; got != 0 {
		t.Errorf("a list given after Run returned was written %d times, want 0", got)
	}
}
