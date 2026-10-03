package executor

import (
	"context"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// Maestro reads a condition value as false only when it is blank, "false" in
// any case, "undefined", "null" or zero (Orchestra.kt:1030-1052).
func TestMaestroTruthy(t *testing.T) {
	for value, want := range map[string]bool{
		"":          false,
		"   ":       false,
		"false":     false,
		"FALSE":     false,
		"undefined": false,
		"null":      false,
		"0":         false,
		"0.0":       false,
		"-0":        false,
		" 0 ":       false,
		"true":      true,
		"True":      true,
		"abc":       true,
		"yes":       true,
		"1":         true,
		"-1":        true,
		"NaN":       true,
		"Null":      true, // the null check is case-sensitive
		"UNDEFINED": true,
	} {
		if got := maestroTruthy(value); got != want {
			t.Errorf("maestroTruthy(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestIsWholeExpression(t *testing.T) {
	for text, want := range map[string]bool{
		"${a}":          true,
		" ${a == 1} ":   true,
		"${ {a: 1}.a }": true,
		"${a} > ${b}":   false,
		"${a}-x":        false,
		"x ${a}":        false,
		"a":             false,
		"${a":           false,
	} {
		if got := isWholeExpression(text); got != want {
			t.Errorf("isWholeExpression(%q) = %v, want %v", text, got, want)
		}
	}
}

// assertTrue on a string that is not "true" failed.
func TestAssertTrue_StringValueIsTrueAsInMaestro(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	se.SetVariable("name", "abc")

	for script, want := range map[string]bool{
		"${name}":           true,
		"${'yes'}":          true,
		"${'false'}":        false,
		"${''}":             false,
		"${'0'}":            false,
		"${notDeclared}":    false,
		"${notDeclared||1}": true,
	} {
		result := se.ExecuteAssertTrue(&flow.AssertTrueStep{Script: script})
		if result.Success != want {
			t.Errorf("assertTrue %s passed=%v, want %v (%s)", script, result.Success, want, result.Message)
		}
	}
}

// A when: condition went through two readings: its ${...} was expanded to text,
// and that text then ran as JavaScript. So a value like "abc" was a
// ReferenceError, "YES" an undefined name, and an unset variable expanded to
// "", which counted as no condition at all and ran the branch.
func TestWhenCondition_ValueIsReadAsInMaestro(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	se.SetVariable("name", "abc")
	se.SetVariable("FLAG", "YES")
	se.SetVariable("url", "https://example.com/a b")
	se.SetVariable("count", "1")

	for script, want := range map[string]bool{
		"${name}":             true,
		"${FLAG}":             true,
		"${url}":              true,
		"${UNSET_FLAG}":       false,
		"${notDeclared}":      false,
		"${notDeclared||'x'}": true,
		"${name == 'abc'}":    true,
		"${count > 3}":        false,
		"${count} > 0":        true, // text around the ${...}: still run as JS, as before
	} {
		cond := flow.Condition{Script: script}
		se.ExpandCondition(&cond)
		if got := se.CheckCondition(context.Background(), cond, &mockDriver{}); got != want {
			t.Errorf("when: true: %s = %v, want %v", script, got, want)
		}
	}
}

func TestRunFlow_WhenUnsetVariableSkipsTheBranch(t *testing.T) {
	var taps []string
	driver := &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		if tap, ok := step.(*flow.TapOnStep); ok {
			taps = append(taps, tap.Selector.Text)
		}
		return &core.CommandResult{Success: true}
	}}
	branch := func(script, tap string) *flow.RunFlowStep {
		return &flow.RunFlowStep{
			BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
			When:     &flow.Condition{Script: script},
			Steps: []flow.Step{&flow.TapOnStep{
				BaseStep: flow.BaseStep{StepType: flow.StepTapOn},
				Selector: flow.Selector{Text: tap},
			}},
		}
	}
	result := runOneFlow(t, driver, flow.Flow{
		SourcePath: "test.yaml",
		Config:     flow.Config{Name: "when true", Env: map[string]string{"NAME": "abc"}},
		Steps: []flow.Step{
			branch("${UNSET_FLAG}", "unset"),
			branch("${NAME}", "named"),
		},
	})
	if result.Status != report.StatusPassed {
		t.Fatalf("status = %v, want passed", result.Status)
	}
	if len(taps) != 1 || taps[0] != "named" {
		t.Errorf("taps = %v, want only the branch whose variable is set", taps)
	}
}
