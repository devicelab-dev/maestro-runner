package executor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// slowStepDriver passes every step after sleeping for d and counts them.
func slowStepDriver(d time.Duration, count *int) *mockDriver {
	return &mockDriver{executeFunc: func(step flow.Step) *core.CommandResult {
		*count++
		time.Sleep(d)
		return &core.CommandResult{Success: true}
	}}
}

// A repeat with times and timeout stops once the timeout has passed, and
// passes.
func TestExecuteRepeat_TimeoutStopsTimes(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	var taps int
	fr := &FlowRunner{ctx: context.Background(), driver: slowStepDriver(50*time.Millisecond, &taps), script: se}

	start := time.Now()
	res := fr.executeRepeat(&flow.RepeatStep{
		Times:   "100",
		Timeout: "200",
		Steps:   []flow.Step{&flow.TapOnStep{Selector: flow.Selector{Text: "Next"}}},
	})
	if !res.Success {
		t.Fatalf("repeat failed: %v", res.Message)
	}
	if taps < 2 || taps > 6 {
		t.Errorf("taps = %d, want about 4 (200ms / 50ms)", taps)
	}
	if !strings.Contains(res.Message, "200ms timeout") {
		t.Errorf("message = %q, want it to say the timeout ended the loop", res.Message)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("repeat took %v", time.Since(start))
	}
}

// A while loop with a timeout and no times is not capped at
// maxWhileIterations; the timeout ends it.
func TestExecuteRepeat_TimeoutEndsWhile(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	var taps int
	fr := &FlowRunner{ctx: context.Background(), driver: slowStepDriver(0, &taps), script: se}

	res := fr.executeRepeat(&flow.RepeatStep{
		While:   flow.Condition{Script: "true"},
		Timeout: "700",
		Steps:   []flow.Step{&flow.TapOnStep{Selector: flow.Selector{Text: "Next"}}},
	})
	if !res.Success {
		t.Fatalf("repeat failed: %v", res.Message)
	}
	// Each while pass sleeps 300ms, so 700ms allows 3 passes.
	if taps < 2 || taps > 4 {
		t.Errorf("taps = %d, want about 3", taps)
	}
}

// The first iteration always runs, even with a timeout shorter than it.
func TestExecuteRepeat_TimeoutRunsFirstIteration(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	var taps int
	fr := &FlowRunner{ctx: context.Background(), driver: slowStepDriver(20*time.Millisecond, &taps), script: se}

	res := fr.executeRepeat(&flow.RepeatStep{
		Times:   "3",
		Timeout: "1",
		Steps:   []flow.Step{&flow.TapOnStep{}},
	})
	if !res.Success || taps != 1 {
		t.Errorf("success=%v taps=%d, want true, 1", res.Success, taps)
	}
}

// A timeout that does not run out leaves times in charge.
func TestExecuteRepeat_TimeoutNotReached(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	se.SetVariable("T", "60000")
	var taps int
	fr := &FlowRunner{ctx: context.Background(), driver: slowStepDriver(0, &taps), script: se}

	res := fr.executeRepeat(&flow.RepeatStep{Times: "3", Timeout: "${T}", Steps: []flow.Step{&flow.TapOnStep{}}})
	if !res.Success || taps != 3 {
		t.Errorf("success=%v taps=%d, want true, 3", res.Success, taps)
	}
	if res.Message != "Repeat completed (3 iterations)" {
		t.Errorf("message = %q", res.Message)
	}
}

func TestExecuteRepeat_InvalidTimeout(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	fr := &FlowRunner{ctx: context.Background(), driver: &mockDriver{}, script: se}

	for _, v := range []string{"soon", "-5"} {
		res := fr.executeRepeat(&flow.RepeatStep{Times: "2", Timeout: v})
		if res.Success || !strings.Contains(res.Message, "invalid 'timeout'") {
			t.Errorf("timeout %q: success=%v message=%q, want an invalid 'timeout' failure", v, res.Success, res.Message)
		}
	}
}

// The iteration count reports the passes that ran, not the times limit.
func TestExecuteRepeat_MessageCountsPassesRun(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	if err := se.RunScript("output.n = 0", nil); err != nil {
		t.Fatal(err)
	}
	fr := &FlowRunner{ctx: context.Background(), driver: &mockDriver{}, script: se}

	res := fr.executeRepeat(&flow.RepeatStep{
		Times: "10",
		While: flow.Condition{Script: "${output.n < 2}"},
		Steps: []flow.Step{&flow.EvalScriptStep{Script: "output.n = output.n + 1"}},
	})
	if !res.Success || res.Message != "Repeat completed (2 iterations)" {
		t.Errorf("success=%v message=%q, want 2 iterations", res.Success, res.Message)
	}
}
