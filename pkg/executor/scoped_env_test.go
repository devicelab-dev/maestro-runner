package executor

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// A key a runFlow, retry or sub-flow env added is gone when it returns, as in
// Maestro, whose leaveEnvScope restores the env as it was (GraalJsEngine.kt:
// 223-238). It used to stay behind set to "".
func TestWithEnvVars_RestoreRemovesAddedKeys(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	se.SetVariable("KEPT", "before")

	restore, err := se.withEnvVars(map[string]string{"KEPT": "inside", "ADDED": "inside"})
	if err != nil {
		t.Fatal(err)
	}
	if se.GetVariable("ADDED") != "inside" || se.GetVariable("KEPT") != "inside" {
		t.Fatalf("env not applied: ADDED=%q KEPT=%q", se.GetVariable("ADDED"), se.GetVariable("KEPT"))
	}
	restore()

	if got := se.GetVariable("KEPT"); got != "before" {
		t.Errorf("KEPT = %q after restore, want its old value", got)
	}
	if _, ok := se.Variables()["ADDED"]; ok {
		t.Error("ADDED is still a variable after restore, want it removed")
	}
	if got, err := se.js.Eval("typeof ADDED"); err != nil || got != "undefined" {
		t.Errorf("typeof ADDED = %v (%v) after restore, want undefined", got, err)
	}
}

func TestRunFlowEnv_IsGoneAfterTheRunFlow(t *testing.T) {
	result := runOneFlow(t, &mockDriver{}, flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "scoped env"},
		Steps: []flow.Step{
			&flow.RunFlowStep{
				BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
				Env:      map[string]string{"SCOPED": "1"},
				Steps: []flow.Step{
					&flow.AssertTrueStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertTrue}, Script: "${SCOPED === '1'}"},
				},
			},
			&flow.AssertTrueStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertTrue}, Script: "${typeof SCOPED === 'undefined'}"},
		},
	})
	if result.Status != report.StatusPassed {
		t.Errorf("status = %v, want passed: SCOPED should be set inside the runFlow and undefined after it", result.Status)
	}
}
