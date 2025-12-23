package executor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// mockDriver implements core.Driver for testing.
type mockDriver struct {
	executeFunc    func(step flow.Step) *core.CommandResult
	screenshotFunc func() ([]byte, error)
	hierarchyFunc  func() ([]byte, error)
	stateFunc      func() *core.StateSnapshot
	platformFunc   func() *core.PlatformInfo
}

func (m *mockDriver) Execute(step flow.Step) *core.CommandResult {
	if m.executeFunc != nil {
		return m.executeFunc(step)
	}
	return &core.CommandResult{Success: true, Duration: 100 * time.Millisecond}
}

func (m *mockDriver) Screenshot() ([]byte, error) {
	if m.screenshotFunc != nil {
		return m.screenshotFunc()
	}
	return []byte{0x89, 0x50, 0x4E, 0x47}, nil // PNG magic bytes
}

func (m *mockDriver) Hierarchy() ([]byte, error) {
	if m.hierarchyFunc != nil {
		return m.hierarchyFunc()
	}
	return []byte("<hierarchy/>"), nil
}

func (m *mockDriver) GetState() *core.StateSnapshot {
	if m.stateFunc != nil {
		return m.stateFunc()
	}
	return &core.StateSnapshot{AppState: "foreground"}
}

func (m *mockDriver) GetPlatformInfo() *core.PlatformInfo {
	if m.platformFunc != nil {
		return m.platformFunc()
	}
	return &core.PlatformInfo{Platform: "android", DeviceID: "test"}
}

func TestRunner_Run_AllPassed(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:     tmpDir,
		Parallelism:   0,
		Artifacts:     ArtifactNever,
		Device:        report.Device{ID: "test", Platform: "android"},
		App:           report.App{ID: "com.test"},
		RunnerVersion: "1.0.0",
		DriverName:    "mock",
	})

	flows := []flow.Flow{
		{
			SourcePath: "test1.yaml",
			Config:     flow.Config{Name: "Test Flow 1"},
			Steps: []flow.Step{
				&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}},
				&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
			},
		},
		{
			SourcePath: "test2.yaml",
			Config:     flow.Config{Name: "Test Flow 2"},
			Steps: []flow.Step{
				&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusPassed)
	}
	if result.TotalFlows != 2 {
		t.Errorf("TotalFlows = %d, want 2", result.TotalFlows)
	}
	if result.PassedFlows != 2 {
		t.Errorf("PassedFlows = %d, want 2", result.PassedFlows)
	}
	if result.FailedFlows != 0 {
		t.Errorf("FailedFlows = %d, want 0", result.FailedFlows)
	}
}

func TestRunner_Run_WithFailure(t *testing.T) {
	tmpDir := t.TempDir()

	stepCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			stepCount++
			if stepCount == 2 {
				return &core.CommandResult{
					Success: false,
					Error:   &testError{msg: "element not found"},
					Message: "Could not find element",
				}
			}
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:     tmpDir,
		Parallelism:   0,
		Artifacts:     ArtifactNever,
		Device:        report.Device{ID: "test"},
		App:           report.App{ID: "com.test"},
		RunnerVersion: "1.0.0",
		DriverName:    "mock",
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Test Flow"},
			Steps: []flow.Step{
				&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}},
				&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
				&flow.AssertVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertVisible}},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Status != report.StatusFailed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusFailed)
	}
	if result.FailedFlows != 1 {
		t.Errorf("FailedFlows = %d, want 1", result.FailedFlows)
	}

	// Third step should be skipped
	if stepCount != 2 {
		t.Errorf("stepCount = %d, want 2 (third step should be skipped)", stepCount)
	}
}

func TestRunner_Run_OptionalStepFailure(t *testing.T) {
	tmpDir := t.TempDir()

	stepCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			stepCount++
			if stepCount == 2 {
				return &core.CommandResult{
					Success: false,
					Error:   &testError{msg: "optional step failed"},
				}
			}
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:     tmpDir,
		Parallelism:   0,
		Artifacts:     ArtifactNever,
		Device:        report.Device{ID: "test"},
		App:           report.App{ID: "com.test"},
		RunnerVersion: "1.0.0",
		DriverName:    "mock",
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Test Flow"},
			Steps: []flow.Step{
				&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}},
				&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn, Optional: true}},
				&flow.AssertVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertVisible}},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Flow should still pass because the failing step was optional
	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusPassed)
	}

	// All three steps should execute
	if stepCount != 3 {
		t.Errorf("stepCount = %d, want 3", stepCount)
	}
}

func TestRunner_Run_Parallel(t *testing.T) {
	tmpDir := t.TempDir()

	var mu sync.Mutex
	concurrent := 0
	maxConcurrent := 0

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			mu.Lock()
			concurrent++
			if concurrent > maxConcurrent {
				maxConcurrent = concurrent
			}
			mu.Unlock()

			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			concurrent--
			mu.Unlock()

			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:     tmpDir,
		Parallelism:   2, // Max 2 concurrent
		Artifacts:     ArtifactNever,
		Device:        report.Device{ID: "test"},
		App:           report.App{ID: "com.test"},
		RunnerVersion: "1.0.0",
		DriverName:    "mock",
	})

	// Create 4 flows
	flows := make([]flow.Flow, 4)
	for i := range flows {
		flows[i] = flow.Flow{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Test Flow"},
			Steps: []flow.Step{
				&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}},
			},
		}
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusPassed)
	}

	// Max concurrency should be limited to 2
	if maxConcurrent > 2 {
		t.Errorf("maxConcurrent = %d, want <= 2", maxConcurrent)
	}
}

func TestRunner_Run_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	stepCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			stepCount++
			time.Sleep(100 * time.Millisecond)
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:     tmpDir,
		Parallelism:   0,
		Artifacts:     ArtifactNever,
		Device:        report.Device{ID: "test"},
		App:           report.App{ID: "com.test"},
		RunnerVersion: "1.0.0",
		DriverName:    "mock",
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Steps: []flow.Step{
				&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}},
				&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
				&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	result, err := runner.Run(ctx, flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should have been cancelled/skipped
	if result.FlowResults[0].Status != report.StatusSkipped {
		t.Errorf("Flow status = %v, want %v", result.FlowResults[0].Status, report.StatusSkipped)
	}
}

// testError implements error interface for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestCommandResultToElement(t *testing.T) {
	// Test nil result
	if got := commandResultToElement(nil); got != nil {
		t.Errorf("commandResultToElement(nil) = %v, want nil", got)
	}

	// Test result with no element
	result := &core.CommandResult{Success: true}
	if got := commandResultToElement(result); got != nil {
		t.Errorf("commandResultToElement(no element) = %v, want nil", got)
	}

	// Test result with element
	result = &core.CommandResult{
		Success: true,
		Element: &core.ElementInfo{
			ID:    "btn_login",
			Text:  "Login",
			Class: "Button",
			Bounds: core.Bounds{
				X: 100, Y: 200, Width: 50, Height: 30,
			},
		},
	}
	got := commandResultToElement(result)
	if got == nil {
		t.Fatal("commandResultToElement() = nil, want element")
	}
	if !got.Found {
		t.Error("Found = false, want true")
	}
	if got.ID != "btn_login" {
		t.Errorf("ID = %q, want %q", got.ID, "btn_login")
	}
	if got.Bounds == nil || got.Bounds.X != 100 {
		t.Error("Bounds not set correctly")
	}
}

func TestCommandResultToError(t *testing.T) {
	// Test nil result
	if got := commandResultToError(nil); got != nil {
		t.Errorf("commandResultToError(nil) = %v, want nil", got)
	}

	// Test result with no error
	result := &core.CommandResult{Success: true}
	if got := commandResultToError(result); got != nil {
		t.Errorf("commandResultToError(no error) = %v, want nil", got)
	}

	// Test result with error and message
	result = &core.CommandResult{
		Success: false,
		Error:   &testError{msg: "element not found"},
		Message: "Could not find login button",
	}
	got := commandResultToError(result)
	if got == nil {
		t.Fatal("commandResultToError() = nil, want error")
	}
	if got.Message != "Could not find login button" {
		t.Errorf("Message = %q, want %q", got.Message, "Could not find login button")
	}
}

func TestRunner_Run_WithArtifacts(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{
				Success: true,
				Element: &core.ElementInfo{ID: "test", Bounds: core.Bounds{X: 0, Y: 0, Width: 100, Height: 50}},
			}
		},
		screenshotFunc: func() ([]byte, error) {
			return []byte{0x89, 0x50, 0x4E, 0x47}, nil
		},
		hierarchyFunc: func() ([]byte, error) {
			return []byte("<hierarchy/>"), nil
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:     tmpDir,
		Parallelism:   0,
		Artifacts:     ArtifactAlways,
		Device:        report.Device{ID: "test"},
		App:           report.App{ID: "com.test"},
		RunnerVersion: "1.0.0",
		DriverName:    "mock",
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Test"},
			Steps: []flow.Step{
				&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusPassed)
	}
}

func TestRunner_Run_ArtifactsOnFailure(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{
				Success: false,
				Error:   &testError{msg: "failed"},
			}
		},
		screenshotFunc: func() ([]byte, error) {
			return []byte{0x89, 0x50, 0x4E, 0x47}, nil
		},
		hierarchyFunc: func() ([]byte, error) {
			return []byte("<hierarchy/>"), nil
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:     tmpDir,
		Parallelism:   0,
		Artifacts:     ArtifactOnFailure,
		Device:        report.Device{ID: "test"},
		App:           report.App{ID: "com.test"},
		RunnerVersion: "1.0.0",
		DriverName:    "mock",
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Test"},
			Steps: []flow.Step{
				&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Status != report.StatusFailed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusFailed)
	}
}
