package executor

import (
	"context"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// findTimeoutDriver records every SetFindTimeout call.
type findTimeoutDriver struct {
	mockDriver
	calls []int
}

func (d *findTimeoutDriver) SetFindTimeout(ms int) { d.calls = append(d.calls, ms) }

func TestEffectiveFindTimeout(t *testing.T) {
	tests := []struct {
		name   string
		flowMs int
		runMs  int
		wantMs int
	}{
		{"neither set keeps driver default", 0, 0, 0},
		{"run-wide flag", 0, 3000, 3000},
		{"flow commandTimeout only", 5000, 0, 5000},
		{"flow commandTimeout wins over flag", 5000, 3000, 5000},
		{"negative flag ignored", 0, -1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveFindTimeout(tt.flowMs, tt.runMs); got != tt.wantMs {
				t.Errorf("effectiveFindTimeout(%d, %d) = %d, want %d", tt.flowMs, tt.runMs, got, tt.wantMs)
			}
		})
	}
}

func TestRunner_FindTimeoutAppliedPerFlow(t *testing.T) {
	driver := &findTimeoutDriver{}
	runner := New(driver, RunnerConfig{
		OutputDir:   t.TempDir(),
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
		App:         report.App{ID: "com.test"},
		DriverName:  "mock",
		FindTimeout: 3000,
	})
	step := func() []flow.Step {
		return []flow.Step{&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}}}
	}
	flows := []flow.Flow{
		{SourcePath: "a.yaml", Config: flow.Config{Name: "A", CommandTimeout: 8000}, Steps: step()},
		// B has no commandTimeout: it must get the run-wide value back, not A's.
		{SourcePath: "b.yaml", Config: flow.Config{Name: "B"}, Steps: step()},
	}
	if _, err := runner.Run(context.Background(), flows); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(driver.calls) != 2 || driver.calls[0] != 8000 || driver.calls[1] != 3000 {
		t.Errorf("SetFindTimeout calls = %v, want [8000 3000]", driver.calls)
	}
}

func TestRunner_NoFindTimeoutLeavesDriverDefault(t *testing.T) {
	driver := &findTimeoutDriver{}
	runner := New(driver, RunnerConfig{
		OutputDir:  t.TempDir(),
		Artifacts:  ArtifactNever,
		Device:     report.Device{ID: "test", Platform: "android"},
		App:        report.App{ID: "com.test"},
		DriverName: "mock",
	})
	flows := []flow.Flow{{SourcePath: "a.yaml", Config: flow.Config{Name: "A"},
		Steps: []flow.Step{&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}}}}}
	if _, err := runner.Run(context.Background(), flows); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(driver.calls) != 0 {
		t.Errorf("SetFindTimeout calls = %v, want none", driver.calls)
	}
}
