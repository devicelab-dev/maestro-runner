package executor

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Every step type with a YAML `env:` map must resolve ${VAR} against the flow's
// scope. Missing one means the step receives six literal characters where a
// value was meant, and the failure surfaces wherever that value was consumed
// rather than where it was written.
func TestExpandStepExpandsEveryEnvMap(t *testing.T) {
	tests := []struct {
		name string
		step flow.Step
		read func(flow.Step) map[string]string
	}{
		{"runShell", &flow.RunShellStep{Command: "true", Env: map[string]string{"TOKEN": "${AUTH}"}},
			func(s flow.Step) map[string]string { return s.(*flow.RunShellStep).Env }},
		{"retry", &flow.RetryStep{Env: map[string]string{"TOKEN": "${AUTH}"}},
			func(s flow.Step) map[string]string { return s.(*flow.RetryStep).Env }},
		{"runFlow", &flow.RunFlowStep{Env: map[string]string{"TOKEN": "${AUTH}"}},
			func(s flow.Step) map[string]string { return s.(*flow.RunFlowStep).Env }},
		{"defineVariables", &flow.DefineVariablesStep{Env: map[string]string{"TOKEN": "${AUTH}"}},
			func(s flow.Step) map[string]string { return s.(*flow.DefineVariablesStep).Env }},
		{"runBrowserScript", &flow.RunBrowserScriptStep{Env: map[string]string{"TOKEN": "${AUTH}"}},
			func(s flow.Step) map[string]string { return s.(*flow.RunBrowserScriptStep).Env }},
		{"runWebViewScript", &flow.RunWebViewScriptStep{Env: map[string]string{"TOKEN": "${AUTH}"}},
			func(s flow.Step) map[string]string { return s.(*flow.RunWebViewScriptStep).Env }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			se := NewScriptEngine()
			defer se.Close()
			se.SetVariable("AUTH", "secret-token")

			se.ExpandStep(tt.step)

			if got := tt.read(tt.step)["TOKEN"]; got != "secret-token" {
				t.Errorf("env value is %q, want the expanded variable", got)
			}
		})
	}
}

// A runShell command is left alone on purpose: ${VAR} means the same thing to
// the shell, and expanding it here would blank the MAESTRO_* values the step
// exports for the command to read.
func TestExpandStepLeavesRunShellCommandAlone(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	se.SetVariable("AUTH", "secret-token")

	step := &flow.RunShellStep{Command: "echo ${MAESTRO_DEVICE_ID} ${AUTH}"}
	se.ExpandStep(step)

	if step.Command != "echo ${MAESTRO_DEVICE_ID} ${AUTH}" {
		t.Errorf("command was rewritten to %q; it should reach the shell verbatim", step.Command)
	}
}
