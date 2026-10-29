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

// scriptedPauser answers each pause with the next action and runs onPause
// first, so a test can act on the paused device.
type scriptedPauser struct {
	actions []PauseAction
	onPause func(p *PausedStep)
	paused  []PausedStep
}

func (s *scriptedPauser) Pause(_ context.Context, p *PausedStep) PauseAction {
	s.paused = append(s.paused, *p)
	if s.onPause != nil {
		s.onPause(p)
	}
	if len(s.actions) == 0 {
		return PauseContinue
	}
	a := s.actions[0]
	s.actions = s.actions[1:]
	return a
}

// pauseDriver fails taps on "missing" until fixed is set; every other step passes.
type pauseDriver struct {
	taps  []string
	fixed bool
}

func (d *pauseDriver) driver() *mockDriver {
	return &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		if tap, ok := step.(*flow.TapOnStep); ok {
			d.taps = append(d.taps, tap.Selector.Text)
			if tap.Selector.Text == "fix" {
				d.fixed = true
			}
			if tap.Selector.Text == "missing" && !d.fixed {
				return &core.CommandResult{Success: false, Error: &testError{msg: "missing is not on screen"}}
			}
		}
		return &core.CommandResult{Success: true}
	}}
}

func runPauseFlow(t *testing.T, pauser FailurePauser, d *pauseDriver, steps ...flow.Step) (FlowResult, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := RunnerConfig{
		OutputDir: dir,
		Artifacts: ArtifactNever,
		Device:    report.Device{ID: "test", Platform: "android"},
	}
	if pauser != nil {
		cfg.Pauser = pauser
	}
	result, err := New(d.driver(), cfg).Run(context.Background(), []flow.Flow{{
		SourcePath: "pause.yaml",
		Config:     flow.Config{Name: "pause"},
		Steps:      steps,
	}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return result.FlowResults[0], dir
}

func readCommands(t *testing.T, dir string) []report.Command {
	t.Helper()
	detail, err := report.ReadFlowDetail(filepath.Join(dir, "flows", "flow-000.json"))
	if err != nil {
		t.Fatalf("ReadFlowDetail: %v", err)
	}
	return detail.Commands
}

func TestPause_ContinueFailsTheFlowAsWithoutPauser(t *testing.T) {
	steps := func() []flow.Step {
		return []flow.Step{tapText("a", false), tapText("missing", false), tapText("b", false)}
	}
	without, dirWithout := runPauseFlow(t, nil, &pauseDriver{}, steps()...)
	pauser := &scriptedPauser{actions: []PauseAction{PauseContinue}}
	with, dirWith := runPauseFlow(t, pauser, &pauseDriver{}, steps()...)

	if len(pauser.paused) != 1 {
		t.Fatalf("paused %d times, want 1", len(pauser.paused))
	}
	p := pauser.paused[0]
	if p.FlowName != "pause" || p.StepIndex != 1 || p.StepCount != 3 || p.Attempt != 1 ||
		!strings.Contains(p.Error, "missing is not on screen") || !strings.Contains(p.Step, "missing") {
		t.Errorf("paused step = %+v", p)
	}

	if with.Status != without.Status || with.Error != without.Error ||
		with.StepsPassed != without.StepsPassed || with.StepsFailed != without.StepsFailed ||
		with.StepsSkipped != without.StepsSkipped || with.StepsTotal != without.StepsTotal {
		t.Errorf("result with pause = %+v, without = %+v", with, without)
	}
	cw, cwo := readCommands(t, dirWith), readCommands(t, dirWithout)
	for i := range cwo {
		if cw[i].Status != cwo[i].Status {
			t.Errorf("command %d status = %s, without pause %s", i, cw[i].Status, cwo[i].Status)
		}
	}
}

func TestPause_RetryAfterFixPassesTheFlow(t *testing.T) {
	d := &pauseDriver{}
	pauser := &scriptedPauser{actions: []PauseAction{PauseRetry}}
	pauser.onPause = func(p *PausedStep) {
		out, err := p.Run(`tapOn: "fix"`)
		if err != nil || len(out) != 1 || !out[0].Passed {
			t.Errorf("Run() = %+v, %v", out, err)
		}
	}
	res, dir := runPauseFlow(t, pauser, d, tapText("a", false), tapText("missing", false), tapText("b", false))

	if res.Status != report.StatusPassed {
		t.Fatalf("status = %s (%s), want passed", res.Status, res.Error)
	}
	// The probe tap is not a flow step; the failed attempt is replaced.
	if res.StepsPassed != 3 || res.StepsFailed != 0 || res.StepsSkipped != 0 || res.StepsTotal != 3 {
		t.Errorf("counts = %+v, want 3 passed of 3", res)
	}
	want := []string{"a", "missing", "fix", "missing", "b"}
	if strings.Join(d.taps, ",") != strings.Join(want, ",") {
		t.Errorf("taps = %v, want %v", d.taps, want)
	}
	cmds := readCommands(t, dir)
	if len(cmds) != 3 {
		t.Fatalf("report has %d commands, want 3", len(cmds))
	}
	for i, c := range cmds {
		if c.Status != report.StatusPassed || c.Error != nil {
			t.Errorf("command %d = %s %+v, want passed with no error", i, c.Status, c.Error)
		}
	}
}

func TestPause_RetryThatFailsPausesAgain(t *testing.T) {
	pauser := &scriptedPauser{actions: []PauseAction{PauseRetry, PauseContinue}}
	res, _ := runPauseFlow(t, pauser, &pauseDriver{}, tapText("missing", false), tapText("b", false))

	if len(pauser.paused) != 2 || pauser.paused[1].Attempt != 2 {
		t.Fatalf("pauses = %+v, want 2 (attempt 2 last)", pauser.paused)
	}
	if res.Status != report.StatusFailed || res.StepsFailed != 1 || res.StepsSkipped != 1 {
		t.Errorf("result = %+v, want failed with 1 failed and 1 skipped", res)
	}
}

func TestPause_NotOnOptionalOrPassingSteps(t *testing.T) {
	pauser := &scriptedPauser{}
	res, _ := runPauseFlow(t, pauser, &pauseDriver{}, tapText("missing", true), tapText("b", false))
	if len(pauser.paused) != 0 {
		t.Errorf("paused on an optional step: %+v", pauser.paused)
	}
	if res.Status != report.StatusPassed {
		t.Errorf("status = %s, want passed", res.Status)
	}
}

func TestPause_ProbeStopsAtFirstFailureAndReportsIt(t *testing.T) {
	d := &pauseDriver{}
	var out []StepOutcome
	pauser := &scriptedPauser{onPause: func(p *PausedStep) {
		var err error
		out, err = p.Run("- tapOn: \"missing\"\n- tapOn: \"never\"\n")
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if _, err := p.Run("tapOn: [unclosed"); err == nil {
			t.Error("Run() of invalid YAML returned no error")
		}
		if _, err := p.Run("   "); err == nil {
			t.Error("Run() of nothing returned no error")
		}
		if shot, err := p.Screenshot(); err != nil || len(shot) == 0 {
			t.Errorf("Screenshot() = %d bytes, %v", len(shot), err)
		}
		if h, err := p.Hierarchy(); err != nil || len(h) == 0 {
			t.Errorf("Hierarchy() = %d bytes, %v", len(h), err)
		}
	}}
	res, _ := runPauseFlow(t, pauser, d, tapText("missing", false))

	if len(out) != 1 || out[0].Passed || !strings.Contains(out[0].Error, "missing is not on screen") {
		t.Errorf("outcomes = %+v, want one failed tap on missing", out)
	}
	for _, tap := range d.taps {
		if tap == "never" {
			t.Error("a step after the failed probe step ran")
		}
	}
	// Probe failures don't count: one failed step, the flow's own.
	if res.StepsFailed != 1 || res.StepsTotal != 1 {
		t.Errorf("counts = %+v, want 1 failed of 1", res)
	}
}

func TestPause_CancelledRunDoesNotPause(t *testing.T) {
	pauser := &scriptedPauser{}
	ctx, cancel := context.WithCancel(context.Background())
	driver := &mockDriver{executeFunc: func(flow.Step) *core.CommandResult {
		cancel()
		return &core.CommandResult{Success: false, Error: &testError{msg: "boom"}}
	}}
	_, err := New(driver, RunnerConfig{OutputDir: t.TempDir(), Artifacts: ArtifactNever, Pauser: pauser}).
		Run(ctx, []flow.Flow{{SourcePath: "c.yaml", Config: flow.Config{Name: "c"}, Steps: []flow.Step{tapText("x", false)}}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(pauser.paused) != 0 {
		t.Errorf("paused on a cancelled run")
	}
}

func TestNewParallelRunner_DropsPauser(t *testing.T) {
	pr := NewParallelRunner(nil, RunnerConfig{Pauser: &scriptedPauser{}})
	if pr.config.Pauser != nil {
		t.Error("parallel runner kept the pauser")
	}
}
