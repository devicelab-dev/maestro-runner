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

// realDriverRejects behaves like the real drivers for the steps the executor
// is meant to handle itself: it has no case for them and fails.
func realDriverRejects(seen *[]flow.Step) *mockDriver {
	return &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			*seen = append(*seen, step)
			switch step.(type) {
			case *flow.RunShellStep, *flow.PasteTextStep:
				return &core.CommandResult{Success: false, Message: "Step type '" + string(step.Type()) + "' is not supported"}
			case *flow.CopyTextFromStep:
				return &core.CommandResult{Success: true, Data: "copied-123"}
			}
			return &core.CommandResult{Success: true}
		},
	}
}

func runOneFlow(t *testing.T, driver *mockDriver, f flow.Flow) *RunResult {
	t.Helper()
	runner := New(driver, RunnerConfig{
		OutputDir: t.TempDir(),
		Artifacts: ArtifactNever,
		Device:    report.Device{ID: "test", Platform: "android"},
	})
	result, err := runner.Run(context.Background(), []flow.Flow{f})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return result
}

func runShellTouching(path string) *flow.RunShellStep {
	return &flow.RunShellStep{
		BaseStep: flow.BaseStep{StepType: flow.StepRunShell},
		Command:  "touch " + path,
	}
}

// #174: runShell worked at the top level and failed inside a runFlow,
// because only the top-level dispatcher handled it.
func TestRunShellInsideInlineRunFlow(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	var seen []flow.Step

	result := runOneFlow(t, realDriverRejects(&seen), flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "nested runShell"},
		Steps: []flow.Step{
			&flow.RunFlowStep{
				BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
				Steps:    []flow.Step{runShellTouching(marker)},
			},
		},
	})

	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want passed", result.Status)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the nested runShell command did not run: %v", err)
	}
	for _, s := range seen {
		if _, ok := s.(*flow.RunShellStep); ok {
			t.Error("runShell reached the driver instead of running on the host")
		}
	}
}

func TestRunShellInsideSubflowFile(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	sub := filepath.Join(dir, "sub.yaml")
	if err := os.WriteFile(sub, []byte("appId: com.example.app\n---\n- runShell: touch "+marker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var seen []flow.Step

	result := runOneFlow(t, realDriverRejects(&seen), flow.Flow{
		SourcePath: filepath.Join(dir, "main.yaml"),
		Config:     flow.Config{Name: "subflow runShell"},
		Steps: []flow.Step{
			&flow.RunFlowStep{BaseStep: flow.BaseStep{StepType: flow.StepRunFlow}, File: "sub.yaml"},
		},
	})

	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want passed", result.Status)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the subflow's runShell command did not run: %v", err)
	}
}

// pasteText pastes what copyTextFrom saved. Nested, it used to fall through
// to the driver and paste the device clipboard instead.
func TestPasteTextInsideRunFlowUsesCopiedText(t *testing.T) {
	var seen []flow.Step

	result := runOneFlow(t, realDriverRejects(&seen), flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "nested pasteText"},
		Steps: []flow.Step{
			&flow.CopyTextFromStep{BaseStep: flow.BaseStep{StepType: flow.StepCopyTextFrom}},
			&flow.RunFlowStep{
				BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
				Steps:    []flow.Step{&flow.PasteTextStep{BaseStep: flow.BaseStep{StepType: flow.StepPasteText}}},
			},
		},
	})

	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want passed", result.Status)
	}
	typed := ""
	for _, s := range seen {
		if in, ok := s.(*flow.InputTextStep); ok {
			typed = in.Text
		}
		if _, ok := s.(*flow.PasteTextStep); ok {
			t.Error("nested pasteText reached the driver instead of typing the copied text")
		}
	}
	if typed != "copied-123" {
		t.Errorf("typed %q, want the text copyTextFrom saved", typed)
	}
}

// pasteText after setClipboard types the text setClipboard set, as Maestro
// does, at the top level and inside a runFlow. It used to read the device
// clipboard, which Android 10+ returns empty to the UiAutomator2 server (#202).
func TestPasteTextAfterSetClipboardTypesItsText(t *testing.T) {
	setClip := func(text string) *flow.SetClipboardStep {
		return &flow.SetClipboardStep{BaseStep: flow.BaseStep{StepType: flow.StepSetClipboard}, Text: text}
	}
	paste := func() *flow.PasteTextStep {
		return &flow.PasteTextStep{BaseStep: flow.BaseStep{StepType: flow.StepPasteText}}
	}
	var seen []flow.Step

	result := runOneFlow(t, realDriverRejects(&seen), flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "setClipboard then pasteText"},
		Steps: []flow.Step{
			setClip("4893010215"),
			paste(),
			&flow.RunFlowStep{
				BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
				Steps:    []flow.Step{setClip("nested-42"), paste()},
			},
		},
	})

	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want passed", result.Status)
	}
	var typed []string
	for _, s := range seen {
		if in, ok := s.(*flow.InputTextStep); ok {
			typed = append(typed, in.Text)
		}
		if _, ok := s.(*flow.PasteTextStep); ok {
			t.Error("pasteText reached the driver instead of typing the setClipboard text")
		}
	}
	if strings.Join(typed, ",") != "4893010215,nested-42" {
		t.Errorf("typed %q, want each setClipboard's text", typed)
	}
}

// A step inside repeat expands ${...} afresh on every pass. Expansion used to
// rewrite the shared step, so the first pass's value was baked in and every
// later pass typed it again (duckduckgo/Android's autofill suite).
func TestRepeatExpandsStepsEachPass(t *testing.T) {
	var typed []string
	driver := &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		if in, ok := step.(*flow.InputTextStep); ok {
			typed = append(typed, in.Text)
		}
		return &core.CommandResult{Success: true}
	}}
	result := runOneFlow(t, driver, flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "repeat expands"},
		Steps: []flow.Step{
			&flow.EvalScriptStep{BaseStep: flow.BaseStep{StepType: flow.StepEvalScript}, Script: "${output.d = ['a','b','c']; output.i = 0}"},
			&flow.RepeatStep{
				BaseStep: flow.BaseStep{StepType: flow.StepRepeat},
				Times:    "3",
				Steps: []flow.Step{
					&flow.InputTextStep{BaseStep: flow.BaseStep{StepType: flow.StepInputText}, Text: "${output.d[output.i]}"},
					&flow.EvalScriptStep{BaseStep: flow.BaseStep{StepType: flow.StepEvalScript}, Script: "${output.i++}"},
				},
			},
		},
	})
	if result.Status != report.StatusPassed {
		t.Fatalf("Status = %v", result.Status)
	}
	if got := strings.Join(typed, ","); got != "a,b,c" {
		t.Errorf("typed %q, want a,b,c", got)
	}
}

// A runFlow condition inside a retry is expanded afresh on every attempt, so
// it sees the output an earlier attempt changed (#176).
func TestRetryReevaluatesRunFlowCondition(t *testing.T) {
	cond := &flow.Condition{Script: "${output.attempt > 0}"}
	result := runOneFlow(t, &mockDriver{}, flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "retry condition"},
		Steps: []flow.Step{
			&flow.EvalScriptStep{BaseStep: flow.BaseStep{StepType: flow.StepEvalScript}, Script: "${output.attempt = 0; output.recoveryRan = false}"},
			&flow.RetryStep{
				BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
				MaxRetries: "2",
				Steps: []flow.Step{
					&flow.RunFlowStep{
						BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
						When:     cond,
						Steps: []flow.Step{
							&flow.EvalScriptStep{BaseStep: flow.BaseStep{StepType: flow.StepEvalScript}, Script: "${output.recoveryRan = true}"},
						},
					},
					&flow.EvalScriptStep{BaseStep: flow.BaseStep{StepType: flow.StepEvalScript}, Script: "${output.attempt += 1}"},
					&flow.AssertTrueStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertTrue}, Script: "${output.recoveryRan}"},
				},
			},
		},
	})
	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want passed: the second attempt should run the recovery branch", result.Status)
	}
	if cond.Script != "${output.attempt > 0}" {
		t.Errorf("condition rewritten in the flow: %q", cond.Script)
	}
}
