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

// runFlowYAML parses main.yaml (and any other files) in a temp dir, runs it
// against a driver that records each step it gets, and returns the result
// with what the driver saw.
func runFlowYAML(t *testing.T, files map[string]string) (*RunResult, []flow.Step) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := flow.ParseFile(filepath.Join(dir, "main.yaml"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var seen []flow.Step
	result := runOneFlow(t, realDriverRejects(&seen), *f)
	return result, seen
}

// A ${...} whose JavaScript throws fails its step with the error, as in
// Maestro, which evaluates a command's scripts as part of running it
// (Orchestra.kt executeCommands). It used to stay in the step as text: the
// flow typed "${output.user.email}", looked for that literal id, read the
// when: as false, or gave the sub-flow the text, and then failed somewhere
// else or passed. The step does nothing on the device, and optional: true
// does not make the failure a warning.
func TestExpressionThatThrowsFailsTheStep(t *testing.T) {
	for name, steps := range map[string]string{
		"inputText": `- inputText: ${output.user.email}`,
		"optional selector": `- tapOn:
    id: ${output.user.id}
    optional: true`,
		"runFlow when": `- runFlow:
    when:
      true: ${output.user.email}
    commands:
      - tapOn: inside`,
		"runFlow env": `- runFlow:
    env:
      EMAIL: ${output.user.email}
    commands:
      - tapOn: inside`,
		"a step inside a runFlow": `- runFlow:
    commands:
      - inputText: ${output.user.email}`,
		"an optional step inside a runFlow": `- runFlow:
    commands:
      - tapOn:
          id: ${output.user.id}
          optional: true`,
		"repeat while": `- repeat:
    while:
      true: ${output.user.email}
    commands:
      - tapOn: inside`,
		"repeat times": `- repeat:
    times: ${output.user.count}
    commands:
      - tapOn: inside`,
		"runFlow file's own env": `- runFlow: sub.yaml`,
		"runScript env": `- runScript:
    file: noop.js
    env:
      EMAIL: ${output.user.email}`,
	} {
		t.Run(name, func(t *testing.T) {
			result, seen := runFlowYAML(t, map[string]string{
				"main.yaml": "appId: x\n---\n" + steps + "\n- tapOn: after\n",
				"sub.yaml":  "appId: x\nenv:\n  EMAIL: ${output.user.email}\n---\n- tapOn: inside\n",
				"noop.js":   "output.ran = true\n",
			})
			if result.Status != report.StatusFailed {
				t.Fatalf("status = %s, want failed", result.Status)
			}
			if msg := result.FlowResults[0].Error; !strings.Contains(msg, "TypeError") {
				t.Errorf("error = %q, want the JavaScript TypeError", msg)
			}
			if len(seen) != 0 {
				t.Errorf("the driver got %d step(s), first %s; want none", len(seen), seen[0].Describe())
			}
		})
	}
}

// An undeclared name is not an error: it reads as undefined, in Maestro too.
func TestUndeclaredNameIsNotAnError(t *testing.T) {
	result, seen := runFlowYAML(t, map[string]string{"main.yaml": `appId: x
---
- inputText: ${notDeclared}
- runFlow:
    when:
      true: ${notDeclared}
    commands:
      - tapOn: inside
`})
	if result.Status != report.StatusPassed {
		t.Fatalf("status = %s, want passed: %+v", result.Status, result.FlowResults)
	}
	if len(seen) != 1 {
		t.Fatalf("the driver got %d steps, want only the inputText", len(seen))
	}
	if in, ok := seen[0].(*flow.InputTextStep); !ok || in.Text != "undefined" {
		t.Errorf("driver got %s, want inputText \"undefined\"", seen[0].Describe())
	}
}

// A when: that is one ${...} is judged by its value; one that throws is an
// error. Text with no ${...} is not evaluated by Maestro at all, so an error
// running it as JavaScript stays a false condition.
func TestCheckConditionReportsAThrowingExpression(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	met, err := se.CheckCondition(context.Background(), flow.Condition{Script: "${output.user.email}"}, &mockDriver{})
	if err == nil || !strings.Contains(err.Error(), "TypeError") || met {
		t.Errorf("${output.user.email}: met=%v err=%v, want the TypeError", met, err)
	}

	met, err = se.CheckCondition(context.Background(), flow.Condition{Script: "undefined_var.property"}, &mockDriver{})
	if err != nil || met {
		t.Errorf("text with no ${...}: met=%v err=%v, want false and no error", met, err)
	}
}

// The step keeps every field that expanded; the one that threw stays as
// written, for its report.
func TestExpandStepKeepsWhatExpanded(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	se.SetVariable("NAME", "Ana")

	step := &flow.InputTextStep{Text: "${output.user.email}", Selector: flow.Selector{ID: "${NAME}"}}
	err := se.ExpandStep(step)
	if err == nil || !strings.Contains(err.Error(), "${output.user.email}") {
		t.Fatalf("err = %v, want one naming ${output.user.email}", err)
	}
	if step.Text != "${output.user.email}" || step.Selector.ID != "Ana" {
		t.Errorf("text = %q, id = %q; want the text as written and the id expanded", step.Text, step.Selector.ID)
	}
}
