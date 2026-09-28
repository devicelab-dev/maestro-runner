package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

func tapText(text string, optional bool) *flow.TapOnStep {
	return &flow.TapOnStep{
		BaseStep: flow.BaseStep{StepType: flow.StepTapOn, Optional: optional},
		Selector: flow.Selector{Text: text},
	}
}

// hookRun is what runHookFlow saw: the flow's result, every tap in order, and
// how many taps had run when the flow was reported as finished.
type hookRun struct {
	result      FlowResult
	taps        []string
	tapsAtEnd   int
	passedAtEnd bool
}

// runHookFlow runs one flow on a driver whose taps on "missing" fail and every
// other step passes.
func runHookFlow(t *testing.T, config flow.Config, body ...flow.Step) hookRun {
	t.Helper()
	var run hookRun
	driver := &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		if tap, ok := step.(*flow.TapOnStep); ok {
			run.taps = append(run.taps, tap.Selector.Text)
			if tap.Selector.Text == "missing" {
				return &core.CommandResult{Success: false, Error: &testError{msg: "missing is not on screen"}}
			}
		}
		return &core.CommandResult{Success: true}
	}}
	config.Name = "hooks"
	runner := New(driver, RunnerConfig{
		OutputDir: t.TempDir(),
		Artifacts: ArtifactNever,
		Device:    report.Device{ID: "test", Platform: "ios"},
		OnFlowEnd: func(_ string, passed bool, _ int64, _ string) {
			run.tapsAtEnd = len(run.taps)
			run.passedAtEnd = passed
		},
	})
	result, err := runner.Run(context.Background(), []flow.Flow{{SourcePath: "test.yaml", Config: config, Steps: body}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	run.result = result.FlowResults[0]
	return run
}

// Maestro fails a flow that passed when its onFlowComplete hook fails
// (Orchestra.kt:237-268). The runner ignored the hook, and ran it only after
// the flow had been reported.
func TestOnFlowComplete_FailingHookFailsAPassingFlow(t *testing.T) {
	run := runHookFlow(t, flow.Config{OnFlowComplete: []flow.Step{tapText("missing", false)}}, tapText("body", false))

	if run.result.Status != report.StatusFailed {
		t.Fatalf("status = %v, want failed", run.result.Status)
	}
	if !strings.Contains(run.result.Error, "onFlowComplete failed") {
		t.Errorf("error = %q, want it to name the onFlowComplete hook", run.result.Error)
	}
	if run.tapsAtEnd != 2 || run.passedAtEnd {
		t.Errorf("the flow was reported (passed=%v) after %d of 2 taps: the hook must run first", run.passedAtEnd, run.tapsAtEnd)
	}
}

func TestOnFlowComplete_OptionalHookStepMayFail(t *testing.T) {
	run := runHookFlow(t, flow.Config{OnFlowComplete: []flow.Step{tapText("missing", true), tapText("after", false)}}, tapText("body", false))

	if run.result.Status != report.StatusPassed {
		t.Errorf("status = %v, want passed: the failing hook step is optional", run.result.Status)
	}
	if strings.Join(run.taps, ",") != "body,missing,after" {
		t.Errorf("taps = %v, want the hook to carry on past its optional step", run.taps)
	}
}

// Maestro's executeCommands stops at the hook's first failing step.
func TestOnFlowComplete_StopsAtItsFirstFailure(t *testing.T) {
	run := runHookFlow(t, flow.Config{OnFlowComplete: []flow.Step{tapText("missing", false), tapText("after", false)}}, tapText("body", false))

	if strings.Join(run.taps, ",") != "body,missing" {
		t.Errorf("taps = %v, want the hook to stop at its failing step", run.taps)
	}
}

// A flow that already failed keeps its own error, and the hook still runs.
func TestOnFlowComplete_BodyFailureKeepsItsError(t *testing.T) {
	run := runHookFlow(t, flow.Config{OnFlowComplete: []flow.Step{tapText("cleanup", false)}}, tapText("missing", false))

	if run.result.Status != report.StatusFailed {
		t.Fatalf("status = %v, want failed", run.result.Status)
	}
	if strings.Contains(run.result.Error, "onFlowComplete") || !strings.Contains(run.result.Error, "missing is not on screen") {
		t.Errorf("error = %q, want the body's failure", run.result.Error)
	}
	if strings.Join(run.taps, ",") != "missing,cleanup" || run.tapsAtEnd != 2 {
		t.Errorf("taps = %v (%d before the report), want the hook to run before the flow is reported", run.taps, run.tapsAtEnd)
	}
}

// When onFlowStart fails, Maestro skips the body, still runs onFlowComplete,
// and fails the flow with the onFlowStart error.
func TestOnFlowStart_FailureStillRunsOnFlowCompleteFirst(t *testing.T) {
	run := runHookFlow(t, flow.Config{
		OnFlowStart:    []flow.Step{tapText("missing", false)},
		OnFlowComplete: []flow.Step{tapText("cleanup", false)},
	}, tapText("body", false))

	if run.result.Status != report.StatusFailed || !strings.Contains(run.result.Error, "onFlowStart failed") {
		t.Fatalf("status = %v, error = %q: want failed by onFlowStart", run.result.Status, run.result.Error)
	}
	if strings.Join(run.taps, ",") != "missing,cleanup" {
		t.Errorf("taps = %v, want the body skipped and onFlowComplete run", run.taps)
	}
	if run.tapsAtEnd != 2 {
		t.Errorf("the flow was reported after %d of 2 taps: onFlowComplete must run first", run.tapsAtEnd)
	}
}

// A step that panics still leaves onFlowComplete to run, as Maestro runs the
// hook in a finally.
func TestOnFlowComplete_RunsAfterAPanic(t *testing.T) {
	var taps []string
	driver := &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		tap, ok := step.(*flow.TapOnStep)
		if ok && tap.Selector.Text == "body" {
			panic("nil pointer dereference in a dependency")
		}
		if ok {
			taps = append(taps, tap.Selector.Text)
		}
		return &core.CommandResult{Success: true}
	}}
	result := runOneFlow(t, driver, flow.Flow{
		SourcePath: "crash.yaml",
		Config:     flow.Config{Name: "crashing flow", OnFlowComplete: []flow.Step{tapText("cleanup", false)}},
		Steps:      []flow.Step{tapText("body", false)},
	})

	if result.Status != report.StatusFailed {
		t.Errorf("status = %v, want failed", result.Status)
	}
	if strings.Join(taps, ",") != "cleanup" {
		t.Errorf("taps = %v, want the onFlowComplete tap to have run", taps)
	}
}

// recordingDriver can record the screen; the recording is a file at the
// target the runner asks for.
type recordingDriver struct {
	*mockDriver
	target string
}

func (d *recordingDriver) StartScreenRecording() error { return nil }

func (d *recordingDriver) StopScreenRecording(target string) error {
	d.target = target
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte("mp4"), 0o644)
}

// A flow that fails in onFlowStart is a failed flow for --video on-failure
// too. Its status stayed "passed" there, so its recording was thrown away.
func TestOnFlowStart_FailureKeepsAnOnFailureRecording(t *testing.T) {
	driver := &recordingDriver{mockDriver: &mockDriver{executeFunc: func(flow.Step) *core.CommandResult {
		return &core.CommandResult{Success: false, Error: &testError{msg: "not there"}}
	}}}
	runner := New(driver, RunnerConfig{
		OutputDir:  t.TempDir(),
		Artifacts:  ArtifactNever,
		Device:     report.Device{ID: "test", Platform: "ios"},
		Record:     true,
		RecordMode: "on-failure",
	})
	result, err := runner.Run(context.Background(), []flow.Flow{{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "start fails", OnFlowStart: []flow.Step{tapText("missing", false)}},
		Steps:      []flow.Step{tapText("body", false)},
	}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != report.StatusFailed {
		t.Fatalf("status = %v, want failed", result.Status)
	}
	if driver.target == "" {
		t.Fatal("the recording was never stopped")
	}
	if _, err := os.Stat(driver.target); err != nil {
		t.Errorf("the failed flow's recording was discarded: %v", err)
	}
}
