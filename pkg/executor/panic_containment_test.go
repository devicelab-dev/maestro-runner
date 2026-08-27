package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// panickingDriver stands in for a driver that dereferences nil deep inside a
// dependency — the shape of #149, where go-rod panicked on a browser whose
// event observable was never initialised.
type panickingDriver struct{ core.Driver }

func (panickingDriver) Execute(flow.Step) *core.CommandResult {
	panic("nil pointer dereference in a dependency")
}

// A panic must cost the flow that caused it, not the process. Under --parallel
// an uncontained panic kills every flow in flight and every flow not yet
// scheduled, so one flaky screen loses a whole suite.
func TestExecuteFlowContainsADriverPanic(t *testing.T) {
	r := &Runner{driver: panickingDriver{}, config: RunnerConfig{}}
	f := flow.Flow{
		SourcePath: "crash.yaml",
		Config:     flow.Config{Name: "crashing flow"},
		Steps:      []flow.Step{&flow.BackStep{}},
	}
	detail := &report.FlowDetail{}

	// The assertion is as much that this returns at all as what it returns:
	// before the fix the panic escaped and took the test binary with it.
	result := r.executeFlow(context.Background(), f, detail, nil, 0, 1)

	if result.Status != report.StatusFailed {
		t.Errorf("status = %v, want failed", result.Status)
	}
	if !strings.Contains(result.Error, "driver panic") {
		t.Errorf("error %q should name the panic", result.Error)
	}
	if result.Name != "crashing flow" {
		t.Errorf("name = %q, want the flow's name so the report identifies it", result.Name)
	}
}
