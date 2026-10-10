package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// retryFlows: "stable" passes, "flaky" fails its tap the first time only,
// "broken" always fails its assert.
func retryFlows() ([]flow.Flow, core.Driver) {
	var taps atomic.Int32
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			switch step.Type() {
			case flow.StepTapOn:
				if taps.Add(1) == 1 {
					return &core.CommandResult{Success: false, Error: &testError{msg: "not yet"}, Message: "not yet"}
				}
			case flow.StepAssertVisible:
				return &core.CommandResult{Success: false, Error: &testError{msg: "never"}, Message: "never"}
			}
			return &core.CommandResult{Success: true}
		},
	}
	launch := &flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}}
	flows := []flow.Flow{
		{SourcePath: "stable.yaml", Config: flow.Config{Name: "stable"}, Steps: []flow.Step{launch}},
		{SourcePath: "flaky.yaml", Config: flow.Config{Name: "flaky"}, Steps: []flow.Step{launch,
			&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}}}},
		{SourcePath: "broken.yaml", Config: flow.Config{Name: "broken"}, Steps: []flow.Step{launch,
			&flow.AssertVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertVisible}}}},
	}
	return flows, driver
}

func retryConfig(dir string, retries int) RunnerConfig {
	return RunnerConfig{
		OutputDir:     dir,
		Retries:       retries,
		Artifacts:     ArtifactOnFailure,
		Device:        report.Device{ID: "test"},
		App:           report.App{ID: "com.test"},
		RunnerVersion: "1.0.0",
		DriverName:    "mock",
	}
}

func checkRetryResult(t *testing.T, dir string, result *RunResult) {
	t.Helper()
	if result.PassedFlows != 2 || result.FailedFlows != 1 {
		t.Fatalf("passed=%d failed=%d, want 2 and 1", result.PassedFlows, result.FailedFlows)
	}
	want := map[string]struct {
		status   report.Status
		attempts int
	}{
		"stable": {report.StatusPassed, 1},
		"flaky":  {report.StatusPassed, 2},
		"broken": {report.StatusFailed, 3},
	}
	for _, fr := range result.FlowResults {
		w := want[fr.Name]
		if fr.Status != w.status || fr.Attempts != w.attempts {
			t.Errorf("%s: status=%s attempts=%d, want %s and %d", fr.Name, fr.Status, fr.Attempts, w.status, w.attempts)
		}
	}

	index, err := report.ReadIndex(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range index.Flows {
		w := want[f.Name]
		if f.Status != w.status {
			t.Errorf("report %s: status=%s, want %s", f.Name, f.Status, w.status)
		}
		if w.attempts == 1 {
			if len(f.AttemptHistory) != 0 {
				t.Errorf("report %s: history %+v, want none", f.Name, f.AttemptHistory)
			}
			continue
		}
		if f.Attempts != w.attempts || len(f.AttemptHistory) != w.attempts {
			t.Fatalf("report %s: attempts=%d history=%d, want %d", f.Name, f.Attempts, len(f.AttemptHistory), w.attempts)
		}
		first := f.AttemptHistory[0]
		if first.Status != report.StatusFailed || first.DataFile != filepath.Join("flows", f.ID+".attempt-1.json") {
			t.Errorf("report %s: first attempt %+v", f.Name, first)
		}
		if last := f.AttemptHistory[w.attempts-1]; last.Status != w.status || last.DataFile != f.DataFile {
			t.Errorf("report %s: last attempt %+v", f.Name, last)
		}
		// The failed first attempt kept its detail and its failure screenshot.
		archived, err := os.ReadFile(filepath.Join(dir, first.DataFile))
		if err != nil {
			t.Fatal(err)
		}
		assets := "assets/" + f.ID + ".attempt-1/"
		if !strings.Contains(string(archived), assets) || strings.Contains(string(archived), "assets/"+f.ID+"/") {
			t.Errorf("report %s: archived detail does not point at %s", f.Name, assets)
		}
		if entries, _ := os.ReadDir(filepath.Join(dir, "assets", f.ID+".attempt-1")); len(entries) == 0 {
			t.Errorf("report %s: first attempt's assets not kept", f.Name)
		}
		detail, err := report.ReadFlowDetail(filepath.Join(dir, f.DataFile))
		if err != nil {
			t.Fatal(err)
		}
		if detail.Commands[0].Status != report.StatusPassed {
			t.Errorf("report %s: final detail %+v", f.Name, detail.Commands[0])
		}
	}
}

func TestRunnerRetriesFailedFlows(t *testing.T) {
	dir := t.TempDir()
	flows, driver := retryFlows()
	result, err := New(driver, retryConfig(dir, 2)).Run(context.Background(), flows)
	if err != nil {
		t.Fatal(err)
	}
	checkRetryResult(t, dir, result)
}

func TestParallelRunnerRetriesFailedFlows(t *testing.T) {
	dir := t.TempDir()
	flows, driver := retryFlows()
	workers := []DeviceWorker{
		{ID: 0, DeviceID: "a", Driver: driver, Cleanup: func() {}},
		{ID: 1, DeviceID: "b", Driver: driver, Cleanup: func() {}},
	}
	result, err := NewParallelRunner(workers, retryConfig(dir, 2)).Run(context.Background(), flows)
	if err != nil {
		t.Fatal(err)
	}
	checkRetryResult(t, dir, result)
}

func TestRunnerWithoutRetriesRunsOnce(t *testing.T) {
	dir := t.TempDir()
	flows, driver := retryFlows()
	result, err := New(driver, retryConfig(dir, 0)).Run(context.Background(), flows)
	if err != nil {
		t.Fatal(err)
	}
	if result.FailedFlows != 2 {
		t.Fatalf("failed=%d, want 2", result.FailedFlows)
	}
	for _, fr := range result.FlowResults {
		if fr.Attempts != 0 {
			t.Errorf("%s: attempts=%d", fr.Name, fr.Attempts)
		}
	}
}
