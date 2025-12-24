package executor

import (
	"context"
	"os"
	"path/filepath"
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

// ===========================================
// Flow Control Handler Tests
// ===========================================

func TestRunner_RepeatStep_FixedTimes(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Repeat Test"},
			Steps: []flow.Step{
				&flow.RepeatStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRepeat},
					Times:    "3",
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
					},
				},
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
	// Should execute 3 times
	if execCount != 3 {
		t.Errorf("execCount = %d, want 3", execCount)
	}
}

func TestRunner_RepeatStep_WhileCondition(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			// Simulate element disappearing after 3 iterations
			if _, ok := step.(*flow.AssertVisibleStep); ok {
				if execCount <= 3 {
					return &core.CommandResult{Success: true}
				}
				return &core.CommandResult{Success: false}
			}
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "While Test"},
			Steps: []flow.Step{
				&flow.RepeatStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRepeat},
					While: flow.Condition{
						Visible: &flow.Selector{Text: "Loading"},
					},
					Steps: []flow.Step{
						&flow.BackStep{BaseStep: flow.BaseStep{StepType: flow.StepBack}},
					},
				},
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

func TestRunner_RepeatStep_NestedStepFailure(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			if execCount == 2 {
				return &core.CommandResult{Success: false, Error: &testError{msg: "nested fail"}}
			}
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Repeat Fail Test"},
			Steps: []flow.Step{
				&flow.RepeatStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRepeat},
					Times:    "5",
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
					},
				},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should fail because nested step failed
	if result.Status != report.StatusFailed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusFailed)
	}
}

func TestRunner_RetryStep_Success(t *testing.T) {
	tmpDir := t.TempDir()

	attemptCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			attemptCount++
			// Succeed on third attempt
			if attemptCount == 3 {
				return &core.CommandResult{Success: true}
			}
			return &core.CommandResult{Success: false, Error: &testError{msg: "not yet"}}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Retry Test"},
			Steps: []flow.Step{
				&flow.RetryStep{
					BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
					MaxRetries: "5",
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
					},
				},
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

func TestRunner_RetryStep_Exhausted(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: false, Error: &testError{msg: "always fails"}}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Retry Fail Test"},
			Steps: []flow.Step{
				&flow.RetryStep{
					BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
					MaxRetries: "3",
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
					},
				},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should fail after exhausting retries
	if result.Status != report.StatusFailed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusFailed)
	}
}

func TestRunner_RetryStep_WithEnv(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Retry Env Test"},
			Steps: []flow.Step{
				&flow.RetryStep{
					BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
					MaxRetries: "2",
					Env: map[string]string{
						"RETRY_VAR": "value",
					},
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
					},
				},
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

func TestRunner_RunFlowStep_InlineSteps(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "RunFlow Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
						&flow.SwipeStep{BaseStep: flow.BaseStep{StepType: flow.StepSwipe}},
					},
				},
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
	if execCount != 2 {
		t.Errorf("execCount = %d, want 2", execCount)
	}
}

func TestRunner_RunFlowStep_WhenCondition(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			// First call is condition check (AssertVisible)
			if _, ok := step.(*flow.AssertVisibleStep); ok {
				return &core.CommandResult{Success: false} // Condition not met
			}
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "RunFlow When Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					When: &flow.Condition{
						Visible: &flow.Selector{Text: "Login"},
					},
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
					},
				},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should pass but skip execution due to when condition
	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusPassed)
	}
	// Only one call for condition check, inner steps skipped
	if execCount != 1 {
		t.Errorf("execCount = %d, want 1", execCount)
	}
}

func TestRunner_RunFlowStep_NoFileOrSteps(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "RunFlow Empty Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					// No file, no steps
				},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should fail - no file or steps
	if result.Status != report.StatusFailed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusFailed)
	}
}

func TestRunner_DefineVariablesStep(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Define Variables Test"},
			Steps: []flow.Step{
				&flow.DefineVariablesStep{
					BaseStep: flow.BaseStep{StepType: flow.StepDefineVariables},
					Env: map[string]string{
						"USER": "testuser",
						"PASS": "testpass",
					},
				},
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

func TestRunner_RunScriptStep(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Run Script Test"},
			Steps: []flow.Step{
				&flow.RunScriptStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunScript},
					Script:   "output.value = 42",
				},
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

func TestRunner_EvalScriptStep(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Eval Script Test"},
			Steps: []flow.Step{
				&flow.EvalScriptStep{
					BaseStep: flow.BaseStep{StepType: flow.StepEvalScript},
					Script:   "var x = 1 + 2;",
				},
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

func TestRunner_AssertTrueStep(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Assert True Test"},
			Steps: []flow.Step{
				&flow.AssertTrueStep{
					BaseStep: flow.BaseStep{StepType: flow.StepAssertTrue},
					Script:   "1 + 1 == 2",
				},
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

func TestRunner_AssertConditionStep(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Assert Condition Test"},
			Steps: []flow.Step{
				&flow.AssertConditionStep{
					BaseStep: flow.BaseStep{StepType: flow.StepAssertCondition},
					Condition: flow.Condition{
						Script: "true",
					},
				},
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

func TestRunner_RepeatStep_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			time.Sleep(50 * time.Millisecond)
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Repeat Cancel Test"},
			Steps: []flow.Step{
				&flow.RepeatStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRepeat},
					Times:    "100",
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
					},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := runner.Run(ctx, flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should be failed or skipped due to cancellation
	if result.Status == report.StatusPassed {
		t.Errorf("Status should not be passed after cancellation")
	}
	// Should have executed less than 100 times
	if execCount >= 100 {
		t.Errorf("execCount = %d, should be less than 100", execCount)
	}
}

func TestRunner_RunFlowStep_ExternalFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an external flow file
	subFlowContent := `appId: com.test
name: Sub Flow
---
- launchApp:
- tapOn:
    text: "Login"
`
	subFlowPath := filepath.Join(tmpDir, "subflow.yaml")
	if err := os.WriteFile(subFlowPath, []byte(subFlowContent), 0644); err != nil {
		t.Fatalf("Failed to write subflow: %v", err)
	}

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: filepath.Join(tmpDir, "main.yaml"),
			Config:     flow.Config{Name: "Main Flow"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					File:     "subflow.yaml",
				},
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
	// Sub-flow has 2 steps
	if execCount != 2 {
		t.Errorf("execCount = %d, want 2", execCount)
	}
}

func TestRunner_RunFlowStep_ExternalFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: filepath.Join(tmpDir, "main.yaml"),
			Config:     flow.Config{Name: "Main Flow"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					File:     "nonexistent.yaml",
				},
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

func TestRunner_RetryStep_ExternalFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an external flow file
	subFlowContent := `appId: com.test
name: Sub Flow
---
- tapOn:
    text: OK
`
	subFlowPath := filepath.Join(tmpDir, "retry_flow.yaml")
	if err := os.WriteFile(subFlowPath, []byte(subFlowContent), 0644); err != nil {
		t.Fatalf("Failed to write subflow: %v", err)
	}

	attemptCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			attemptCount++
			// Succeed on second attempt
			if attemptCount >= 2 {
				return &core.CommandResult{Success: true}
			}
			return &core.CommandResult{Success: false, Error: &testError{msg: "not yet"}}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: filepath.Join(tmpDir, "main.yaml"),
			Config:     flow.Config{Name: "Retry External Test"},
			Steps: []flow.Step{
				&flow.RetryStep{
					BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
					MaxRetries: "3",
					File:       "retry_flow.yaml",
				},
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

func TestRunner_RetryStep_ExternalFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: filepath.Join(tmpDir, "main.yaml"),
			Config:     flow.Config{Name: "Retry External Fail Test"},
			Steps: []flow.Step{
				&flow.RetryStep{
					BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
					MaxRetries: "2",
					File:       "nonexistent.yaml",
				},
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

func TestRunner_NestedFlowControl(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	// Test nested repeat inside runFlow
	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Nested Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					Steps: []flow.Step{
						&flow.RepeatStep{
							BaseStep: flow.BaseStep{StepType: flow.StepRepeat},
							Times:    "2",
							Steps: []flow.Step{
								&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
							},
						},
					},
				},
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
	// Repeat 2 times
	if execCount != 2 {
		t.Errorf("execCount = %d, want 2", execCount)
	}
}

func TestRunner_RetryStep_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			time.Sleep(50 * time.Millisecond)
			return &core.CommandResult{Success: false, Error: &testError{msg: "fail"}}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Retry Cancel Test"},
			Steps: []flow.Step{
				&flow.RetryStep{
					BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
					MaxRetries: "100",
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
					},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := runner.Run(ctx, flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should be failed due to cancellation
	if result.Status == report.StatusPassed {
		t.Errorf("Status should not be passed after cancellation")
	}
	// Should have executed less than 100 times
	if execCount >= 100 {
		t.Errorf("execCount = %d, should be less than 100", execCount)
	}
}

// ===========================================
// Nested Step Type Tests (executeNestedStep coverage)
// ===========================================

func TestRunner_NestedDefineVariables(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Nested DefineVariables Test"},
			Steps: []flow.Step{
				&flow.RepeatStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRepeat},
					Times:    "2",
					Steps: []flow.Step{
						&flow.DefineVariablesStep{
							BaseStep: flow.BaseStep{StepType: flow.StepDefineVariables},
							Env:      map[string]string{"VAR": "value"},
						},
					},
				},
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

func TestRunner_NestedRunScript(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Nested RunScript Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					Steps: []flow.Step{
						&flow.RunScriptStep{
							BaseStep: flow.BaseStep{StepType: flow.StepRunScript},
							Script:   "output.x = 1",
						},
					},
				},
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

func TestRunner_NestedEvalScript(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Nested EvalScript Test"},
			Steps: []flow.Step{
				&flow.RetryStep{
					BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
					MaxRetries: "1",
					Steps: []flow.Step{
						&flow.EvalScriptStep{
							BaseStep: flow.BaseStep{StepType: flow.StepEvalScript},
							Script:   "var y = 2",
						},
					},
				},
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

func TestRunner_NestedAssertTrue(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Nested AssertTrue Test"},
			Steps: []flow.Step{
				&flow.RepeatStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRepeat},
					Times:    "1",
					Steps: []flow.Step{
						&flow.AssertTrueStep{
							BaseStep: flow.BaseStep{StepType: flow.StepAssertTrue},
							Script:   "1 + 1 == 2",
						},
					},
				},
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

func TestRunner_NestedAssertCondition(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Nested AssertCondition Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					Steps: []flow.Step{
						&flow.AssertConditionStep{
							BaseStep:  flow.BaseStep{StepType: flow.StepAssertCondition},
							Condition: flow.Condition{Script: "true"},
						},
					},
				},
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

func TestRunner_NestedRetry(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			// Fail first, succeed second
			if execCount == 1 {
				return &core.CommandResult{Success: false, Error: &testError{msg: "fail"}}
			}
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Nested Retry Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					Steps: []flow.Step{
						&flow.RetryStep{
							BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
							MaxRetries: "3",
							Steps: []flow.Step{
								&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
							},
						},
					},
				},
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

func TestRunner_NestedRunFlow(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Nested RunFlow Test"},
			Steps: []flow.Step{
				&flow.RepeatStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRepeat},
					Times:    "2",
					Steps: []flow.Step{
						&flow.RunFlowStep{
							BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
							Steps: []flow.Step{
								&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
							},
						},
					},
				},
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
	// RunFlow with 1 tap, repeated 2 times
	if execCount != 2 {
		t.Errorf("execCount = %d, want 2", execCount)
	}
}

func TestRunner_RetryStep_ExternalFile_Exhausted(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an external flow file
	subFlowContent := `appId: com.test
name: Sub Flow
---
- tapOn:
    text: OK
`
	subFlowPath := filepath.Join(tmpDir, "retry_flow.yaml")
	if err := os.WriteFile(subFlowPath, []byte(subFlowContent), 0644); err != nil {
		t.Fatalf("Failed to write subflow: %v", err)
	}

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: false, Error: &testError{msg: "always fails"}}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: filepath.Join(tmpDir, "main.yaml"),
			Config:     flow.Config{Name: "Retry External Exhausted Test"},
			Steps: []flow.Step{
				&flow.RetryStep{
					BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
					MaxRetries: "2",
					File:       "retry_flow.yaml",
				},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should fail after exhausting retries
	if result.Status != report.StatusFailed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusFailed)
	}
}

func TestRunner_RetryStep_ExternalFile_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an external flow file
	subFlowContent := `appId: com.test
name: Sub Flow
---
- tapOn:
    text: OK
`
	subFlowPath := filepath.Join(tmpDir, "retry_flow.yaml")
	if err := os.WriteFile(subFlowPath, []byte(subFlowContent), 0644); err != nil {
		t.Fatalf("Failed to write subflow: %v", err)
	}

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			time.Sleep(50 * time.Millisecond)
			return &core.CommandResult{Success: false, Error: &testError{msg: "fails"}}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: filepath.Join(tmpDir, "main.yaml"),
			Config:     flow.Config{Name: "Retry External Cancel Test"},
			Steps: []flow.Step{
				&flow.RetryStep{
					BaseStep:   flow.BaseStep{StepType: flow.StepRetry},
					MaxRetries: "100",
					File:       "retry_flow.yaml",
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := runner.Run(ctx, flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should fail due to cancellation
	if result.Status == report.StatusPassed {
		t.Errorf("Status should not be passed after cancellation")
	}
}

func TestRunner_NestedOptionalStepFailure(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: false, Error: &testError{msg: "fail"}}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Nested Optional Test"},
			Steps: []flow.Step{
				&flow.RepeatStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRepeat},
					Times:    "1",
					Steps: []flow.Step{
						&flow.TapOnStep{
							BaseStep: flow.BaseStep{
								StepType: flow.StepTapOn,
								Optional: true,
							},
						},
					},
				},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should pass because nested step is optional
	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusPassed)
	}
}
