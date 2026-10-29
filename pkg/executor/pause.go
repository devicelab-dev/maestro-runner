package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Pausing a flow at its failing step (#246).
//
// With RunnerConfig.Pauser set, a top-level step whose failure would fail the
// flow hands control to the pauser before anything else happens: the
// remaining steps are not yet skipped, onFlowComplete has not run, and the
// driver session, the app and the flow's variables are exactly as the failure
// left them. The pauser can look at the screen, run steps on it in the flow's
// context, and then either let the failure stand or have the step run again.
//
// Everything happens on the flow's own goroutine: the pauser is called from
// it and PausedStep's methods use the flow's driver from it, so nothing else
// drives the device at the same time.
//
// It does not pause on an optional step, on a cancelled run, in the
// onFlowStart / onFlowComplete hooks, or inside a runFlow / repeat / retry:
// a failure in there pauses on the compound step that it fails.

// PauseAction is what a paused flow does next.
type PauseAction int

const (
	// PauseContinue lets the failure stand: the step is failed and the flow
	// ends as it would have without the pause.
	PauseContinue PauseAction = iota
	// PauseRetry runs the failed step again. Its new result replaces the
	// failed one; when that fails too, the flow pauses again.
	PauseRetry
)

// FailurePauser decides what a flow does when a step fails. Pause blocks
// until the decision is made; it should return PauseContinue when ctx ends.
type FailurePauser interface {
	Pause(ctx context.Context, p *PausedStep) PauseAction
}

// PausedStep describes the step a flow paused on and gives access to the
// device it paused on. Its methods must be called from inside Pause only.
type PausedStep struct {
	FlowName  string
	FlowFile  string // path of the flow file
	StepIndex int    // 0-based index of the failed top-level step
	StepCount int    // number of top-level steps in the flow
	Step      string // the step as the console shows it
	Error     string // why it failed
	Attempt   int    // 1 on the first failure, 2 after one retry, ...

	fr *FlowRunner
}

// Screenshot captures the screen as it is now.
func (p *PausedStep) Screenshot() ([]byte, error) {
	return p.fr.driver.Screenshot()
}

// Hierarchy reads the driver's view hierarchy as it is now.
func (p *PausedStep) Hierarchy() ([]byte, error) {
	return p.fr.driver.Hierarchy()
}

// StepOutcome is the result of one step run with PausedStep.Run.
type StepOutcome struct {
	Step   string
	Passed bool
	Error  string
}

// Run runs YAML steps on the paused device in the flow's context: its
// variables, output.*, copiedText and appId. yamlSteps is a step list as in a
// flow file; a single step may leave out the leading "- ". Steps run in order
// and stop at the first one that fails and is not optional.
//
// These are probes, not part of the flow: they are not added to the report
// and not counted in the flow's steps.
func (p *PausedStep) Run(yamlSteps string) ([]StepOutcome, error) {
	steps, err := parsePauseSteps(yamlSteps, p.fr.flow.SourcePath)
	if err != nil {
		return nil, err
	}
	return p.fr.runProbeSteps(steps), nil
}

// parsePauseSteps parses the steps a person or agent typed at a pause.
func parsePauseSteps(text, sourcePath string) ([]flow.Step, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("no step given")
	}
	if !strings.HasPrefix(trimmed, "-") {
		trimmed = "- " + trimmed
	}
	f, err := flow.Parse([]byte(trimmed), sourcePath)
	if err != nil {
		return nil, err
	}
	if len(f.Steps) == 0 {
		return nil, fmt.Errorf("no step given")
	}
	return f.Steps, nil
}

// runProbeSteps runs steps for a pause without touching the flow's report or
// step counts.
func (fr *FlowRunner) runProbeSteps(steps []flow.Step) []StepOutcome {
	passed, failed, skipped := fr.stepsPassed, fr.stepsFailed, fr.stepsSkipped
	subCommands := fr.subCommands
	fr.subCommands = nil
	defer func() {
		fr.stepsPassed, fr.stepsFailed, fr.stepsSkipped = passed, failed, skipped
		fr.subCommands = subCommands
		// A runScript's console.log must not end up on the next command.
		fr.script.TakeConsoleLogs()
	}()

	var outcomes []StepOutcome
	for _, step := range steps {
		step = cloneForRun(step)
		describe, _ := fr.stepNames(step)
		var result *core.CommandResult
		if err := fr.script.ExpandStep(step); err != nil {
			result = expressionFailed(err)
		} else {
			result = fr.runNestedStep(step)
		}
		result = fr.maskTypedResult(step, result)
		fr.markInteraction(step)

		outcome := StepOutcome{Step: describe, Passed: result.Success}
		if !result.Success {
			if e := commandResultToError(result); e != nil {
				outcome.Error = e.Message
			} else {
				outcome.Error = "failed"
			}
		}
		outcomes = append(outcomes, outcome)
		if !result.Success && !optionalFailure(step, result) {
			break
		}
	}
	return outcomes
}

// pauseOnFailure asks the pauser what to do about a failed step. It reports
// whether to run the step again.
func (fr *FlowRunner) pauseOnFailure(idx int, desc, errMsg string, attempt int) bool {
	if fr.config.Pauser == nil || fr.ctx.Err() != nil {
		return false
	}
	p := &PausedStep{
		FlowName:  fr.detail.Name,
		FlowFile:  fr.flow.SourcePath,
		StepIndex: idx,
		StepCount: len(fr.flow.Steps),
		Step:      desc,
		Error:     errMsg,
		Attempt:   attempt,
		fr:        fr,
	}
	return fr.config.Pauser.Pause(fr.ctx, p) == PauseRetry && fr.ctx.Err() == nil
}
