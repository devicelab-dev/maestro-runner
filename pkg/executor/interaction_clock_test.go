package executor

import (
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Only steps that may change the app restart the when: budget clock, as in
// Maestro; asserts, waits and a runFlow its condition skipped leave it.
func TestMarkInteraction(t *testing.T) {
	past := time.Now().Add(-5 * time.Second)
	for _, tc := range []struct {
		name    string
		step    flow.Step
		skipped bool
		marks   bool
	}{
		{"tap", &flow.TapOnStep{}, false, true},
		{"inputText", &flow.InputTextStep{}, false, true},
		{"assertVisible", &flow.AssertVisibleStep{}, false, false},
		{"extendedWaitUntil", &flow.WaitUntilStep{}, false, false},
		{"retry", &flow.RetryStep{}, false, false},
		{"runFlow ran", &flow.RunFlowStep{}, false, true},
		{"runFlow skipped", &flow.RunFlowStep{}, true, false},
		{"runScript ran", &flow.RunScriptStep{}, false, true},
		{"runScript skipped", &flow.RunScriptStep{}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fr := &FlowRunner{script: NewScriptEngine(), conditionSkipped: tc.skipped}
			fr.script.lastInteraction = past
			fr.markInteraction(tc.step)
			if marked := fr.script.lastInteraction.After(past); marked != tc.marks {
				t.Errorf("marked = %v, want %v", marked, tc.marks)
			}
			if fr.conditionSkipped {
				t.Error("conditionSkipped not consumed")
			}
		})
	}
}
