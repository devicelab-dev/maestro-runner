package executor

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// An airplane mode step's message ("already enabled", "skipped" on a
// simulator) is kept for the report; other steps' messages and failures
// are not (#252).
func TestOutcomeMessage(t *testing.T) {
	ok := core.SuccessResult("Airplane mode already enabled", nil)
	cases := []struct {
		name   string
		step   flow.Step
		result *core.CommandResult
		want   string
	}{
		{"set", &flow.SetAirplaneModeStep{Enabled: true}, ok, "Airplane mode already enabled"},
		{"toggle", &flow.ToggleAirplaneModeStep{}, core.SuccessResult("Toggled airplane mode (enabled)", nil), "Toggled airplane mode (enabled)"},
		{"other step", &flow.TapOnStep{}, core.SuccessResult("Tapped", nil), ""},
		{"failed", &flow.SetAirplaneModeStep{}, &core.CommandResult{Success: false, Message: "boom"}, ""},
		{"nil", &flow.SetAirplaneModeStep{}, nil, ""},
	}
	for _, c := range cases {
		if got := outcomeMessage(c.step, c.result); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
