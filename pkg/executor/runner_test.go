package executor

import (
	"bytes"
	"context"
	"fmt"
	"image"
	imagecolor "image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
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

func (m *mockDriver) SetFindTimeout(ms int) {
	// no-op for mock
}

func (m *mockDriver) SetWaitForIdleTimeout(ms int) error {
	// no-op for mock
	return nil
}

func (m *mockDriver) SetContext(ctx context.Context) {
	// no-op for mock
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

	// Test result with error and message — message wraps the underlying
	// cause so debugging surfaces the original error reason without losing
	// the friendly wrapper.
	result = &core.CommandResult{
		Success: false,
		Error:   &testError{msg: "element not found"},
		Message: "Could not find login button",
	}
	got := commandResultToError(result)
	if got == nil {
		t.Fatal("commandResultToError() = nil, want error")
	}
	want := "Could not find login button (cause: element not found)"
	if got.Message != want {
		t.Errorf("Message = %q, want %q", got.Message, want)
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

func TestRunner_RunFlowStep_WhenFalse_RunsElseSteps(t *testing.T) {
	tmpDir := t.TempDir()

	var tapTargets []string
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if _, ok := step.(*flow.AssertVisibleStep); ok {
				// When condition: not visible → falls to else branch.
				return &core.CommandResult{Success: false}
			}
			if t, ok := step.(*flow.TapOnStep); ok {
				tapTargets = append(tapTargets, t.Selector.Text)
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

	mainTap := &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}}
	mainTap.Selector.Text = "MainBranch"
	elseTap := &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}}
	elseTap.Selector.Text = "ElseBranch"

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "RunFlow Else Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					When: &flow.Condition{
						Visible: &flow.Selector{Text: "Logout"},
					},
					Steps:     []flow.Step{mainTap},
					ElseSteps: []flow.Step{elseTap},
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
	if len(tapTargets) != 1 || tapTargets[0] != "ElseBranch" {
		t.Errorf("expected only ElseBranch tap, got %v", tapTargets)
	}
}

func TestRunner_RunFlowStep_WhenTrue_RunsMainBranch(t *testing.T) {
	tmpDir := t.TempDir()

	var tapTargets []string
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if _, ok := step.(*flow.AssertVisibleStep); ok {
				return &core.CommandResult{Success: true} // when condition met
			}
			if t, ok := step.(*flow.TapOnStep); ok {
				tapTargets = append(tapTargets, t.Selector.Text)
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

	mainTap := &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}}
	mainTap.Selector.Text = "MainBranch"
	elseTap := &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}}
	elseTap.Selector.Text = "ElseBranch"

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "RunFlow When True Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep:  flow.BaseStep{StepType: flow.StepRunFlow},
					When:      &flow.Condition{Visible: &flow.Selector{Text: "Logout"}},
					Steps:     []flow.Step{mainTap},
					ElseSteps: []flow.Step{elseTap},
				},
			},
		},
	}

	if _, err := runner.Run(context.Background(), flows); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(tapTargets) != 1 || tapTargets[0] != "MainBranch" {
		t.Errorf("expected only MainBranch tap, got %v", tapTargets)
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

func TestRunner_RunFlowStep_Timeout(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			// Each step takes 100ms
			time.Sleep(100 * time.Millisecond)
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
	})

	// runFlow with 250ms timeout containing 5 steps (each takes 100ms)
	// Should only complete ~2 steps before timeout
	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "RunFlow Timeout Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow, TimeoutMs: 250},
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
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

	// Should fail due to timeout
	if result.Status != report.StatusFailed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusFailed)
	}

	// Should NOT have executed all 5 steps
	if execCount >= 5 {
		t.Errorf("execCount = %d, want < 5 (timeout should stop execution)", execCount)
	}
}

func TestRunner_RunFlowStep_TimeoutNotExceeded(t *testing.T) {
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

	// runFlow with generous timeout — all steps should complete
	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "RunFlow No Timeout Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow, TimeoutMs: 5000},
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
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

	if execCount != 3 {
		t.Errorf("execCount = %d, want 3", execCount)
	}
}

func TestRunner_RunFlowStep_InlineStepFailure(t *testing.T) {
	tmpDir := t.TempDir()

	execCount := 0
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			// Second step fails
			if execCount == 2 {
				return &core.CommandResult{Success: false, Error: fmt.Errorf("step failed")}
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
			Config:     flow.Config{Name: "Inline Failure Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					Steps: []flow.Step{
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}},
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}}, // fails
						&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}}, // should not run
					},
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

	// Third step should not execute
	if execCount != 2 {
		t.Errorf("execCount = %d, want 2", execCount)
	}
}

func TestRunner_RunFlowStep_ExternalFileWithCallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a sub-flow file
	subFlowContent := `appId: com.test
---
- tapOn: "OK"
`
	subFlowPath := filepath.Join(tmpDir, "sub.yaml")
	if err := os.WriteFile(subFlowPath, []byte(subFlowContent), 0644); err != nil {
		t.Fatalf("Failed to write sub-flow: %v", err)
	}

	nestedFlowStarted := false
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
		OnNestedFlowStart: func(depth int, name string) {
			nestedFlowStarted = true
		},
	})

	flows := []flow.Flow{
		{
			SourcePath: filepath.Join(tmpDir, "main.yaml"),
			Config:     flow.Config{Name: "Callback Test"},
			Steps: []flow.Step{
				&flow.RunFlowStep{
					BaseStep: flow.BaseStep{StepType: flow.StepRunFlow},
					File:     "sub.yaml",
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

	if !nestedFlowStarted {
		t.Error("OnNestedFlowStart callback was not called")
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
	if err := os.WriteFile(subFlowPath, []byte(subFlowContent), 0o644); err != nil {
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
	if err := os.WriteFile(subFlowPath, []byte(subFlowContent), 0o644); err != nil {
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
	if err := os.WriteFile(subFlowPath, []byte(subFlowContent), 0o644); err != nil {
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
	if err := os.WriteFile(subFlowPath, []byte(subFlowContent), 0o644); err != nil {
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

// ===========================================
// TakeScreenshotStep Tests
// ===========================================

func TestRunner_TakeScreenshotStep_Success(t *testing.T) {
	tmpDir := t.TempDir()

	screenshotData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if _, ok := step.(*flow.TakeScreenshotStep); ok {
				return &core.CommandResult{
					Success: true,
					Data:    screenshotData,
				}
			}
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
		App:         report.App{ID: "com.test"},
	})

	flows := []flow.Flow{
		{
			SourcePath: filepath.Join(tmpDir, "test.yaml"),
			Config:     flow.Config{Name: "Screenshot Test"},
			Steps: []flow.Step{
				&flow.TakeScreenshotStep{
					BaseStep: flow.BaseStep{StepType: flow.StepTakeScreenshot},
					Path:     "my-screenshot.png",
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

	// Check that screenshot file was saved
	screenshotPath := filepath.Join(tmpDir, "assets", "flow-000", "cmd-000-my-screenshot.png")
	if _, err := os.Stat(screenshotPath); err != nil {
		t.Errorf("screenshot file not created at %s: %v", screenshotPath, err)
	}

	requestedPath := filepath.Join(tmpDir, "my-screenshot.png")
	if data, err := os.ReadFile(requestedPath); err != nil {
		t.Errorf("requested screenshot file not created at %s: %v", requestedPath, err)
	} else if !bytes.Equal(data, screenshotData) {
		t.Error("requested screenshot file does not contain captured data")
	}
}

func TestRunner_TakeScreenshotStep_EmptyName(t *testing.T) {
	tmpDir := t.TempDir()

	screenshotData := []byte{0x89, 0x50, 0x4E, 0x47}
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if _, ok := step.(*flow.TakeScreenshotStep); ok {
				return &core.CommandResult{
					Success: true,
					Data:    screenshotData,
				}
			}
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
		App:         report.App{ID: "com.test"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Screenshot Empty Name"},
			Steps: []flow.Step{
				&flow.TakeScreenshotStep{
					BaseStep: flow.BaseStep{StepType: flow.StepTakeScreenshot},
					Path:     "", // Empty name should default to screenshot.png
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

	// Check that screenshot file was saved with default name
	screenshotPath := filepath.Join(tmpDir, "assets", "flow-000", "cmd-000-screenshot.png")
	if _, err := os.Stat(screenshotPath); err != nil {
		t.Errorf("screenshot file not created at %s: %v", screenshotPath, err)
	}
}

func TestRunner_TakeScreenshotStep_DriverFailure(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if _, ok := step.(*flow.TakeScreenshotStep); ok {
				return &core.CommandResult{
					Success: false,
					Error:   &testError{msg: "screenshot failed"},
				}
			}
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
		App:         report.App{ID: "com.test"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Screenshot Fail Test"},
			Steps: []flow.Step{
				&flow.TakeScreenshotStep{
					BaseStep: flow.BaseStep{StepType: flow.StepTakeScreenshot},
					Path:     "test.png",
				},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Step failure should fail the flow
	if result.Status != report.StatusFailed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusFailed)
	}
}

func TestRunner_TakeScreenshotStep_NoData(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if _, ok := step.(*flow.TakeScreenshotStep); ok {
				// Success but no data returned (Data is nil)
				return &core.CommandResult{
					Success: true,
					Data:    nil,
				}
			}
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
		App:         report.App{ID: "com.test"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Screenshot No Data"},
			Steps: []flow.Step{
				&flow.TakeScreenshotStep{
					BaseStep: flow.BaseStep{StepType: flow.StepTakeScreenshot},
					Path:     "test.png",
				},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should still pass, just no screenshot saved
	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusPassed)
	}
}

func TestRunner_TakeScreenshotStep_EmptyData(t *testing.T) {
	tmpDir := t.TempDir()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if _, ok := step.(*flow.TakeScreenshotStep); ok {
				return &core.CommandResult{
					Success: true,
					Data:    []byte{}, // Empty data
				}
			}
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir:   tmpDir,
		Parallelism: 0,
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
		App:         report.App{ID: "com.test"},
	})

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Screenshot Empty Data"},
			Steps: []flow.Step{
				&flow.TakeScreenshotStep{
					BaseStep: flow.BaseStep{StepType: flow.StepTakeScreenshot},
					Path:     "test.png",
				},
			},
		},
	}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should still pass, just no screenshot saved due to empty data
	if result.Status != report.StatusPassed {
		t.Errorf("Status = %v, want %v", result.Status, report.StatusPassed)
	}
}

func TestRunner_AssertScreenshotStep(t *testing.T) {
	reference := encodeTestPNG(t, []imagecolor.RGBA{
		{R: 255, A: 255},
		{G: 255, A: 255},
	})
	withinTolerance := encodeTestPNG(t, []imagecolor.RGBA{
		{R: 240, A: 255},
		{G: 240, A: 255},
	})
	mismatch := encodeTestPNG(t, []imagecolor.RGBA{
		{B: 255, A: 255},
		{G: 255, A: 255},
	})

	tests := []struct {
		name           string
		captured       []byte
		expectedStatus report.Status
	}{
		{
			name:           "image within Maestro color tolerance passes",
			captured:       withinTolerance,
			expectedStatus: report.StatusPassed,
		},
		{
			name:           "image below threshold fails",
			captured:       mismatch,
			expectedStatus: report.StatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmpDir, "reference.png"), reference, 0o644); err != nil {
				t.Fatalf("write reference image: %v", err)
			}

			driver := &mockDriver{
				executeFunc: func(step flow.Step) *core.CommandResult {
					if _, ok := step.(*flow.AssertScreenshotStep); ok {
						return &core.CommandResult{Success: true, Data: tt.captured}
					}
					return &core.CommandResult{Success: true}
				},
			}
			runner := New(driver, RunnerConfig{
				OutputDir:   filepath.Join(tmpDir, "output"),
				Artifacts:   ArtifactNever,
				Device:      report.Device{ID: "test", Platform: "android"},
				App:         report.App{ID: "com.test"},
				Parallelism: 0,
			})
			flows := []flow.Flow{{
				SourcePath: filepath.Join(tmpDir, "test.yaml"),
				Config:     flow.Config{Name: "Assert Screenshot Test"},
				Steps: []flow.Step{&flow.AssertScreenshotStep{
					BaseStep:            flow.BaseStep{StepType: flow.StepAssertScreenshot},
					Path:                "reference",
					ThresholdPercentage: 100,
				}},
			}}

			result, err := runner.Run(context.Background(), flows)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result.Status != tt.expectedStatus {
				t.Errorf("Status = %v, want %v", result.Status, tt.expectedStatus)
			}

			diffPath := filepath.Join(tmpDir, "reference_diff.png")
			if tt.expectedStatus == report.StatusFailed {
				if _, err := os.Stat(diffPath); err != nil {
					t.Errorf("expected diff image at %s: %v", diffPath, err)
				}
			} else if _, err := os.Stat(diffPath); err == nil {
				t.Errorf("unexpected diff image written at %s", diffPath)
			}
		})
	}
}

// TestRunner_AssertScreenshotStep_NearMissMessage covers #138: a screenshot
// that misses a 100% threshold by a few pixels used to report "100.00% match
// (threshold: 100.00%)", which reads like a runner bug. The message must widen
// precision and name the differing pixel count.
func TestRunner_AssertScreenshotStep_NearMissMessage(t *testing.T) {
	tmpDir := t.TempDir()

	base := imagecolor.RGBA{R: 100, G: 100, B: 100, A: 255}
	reference := encodeTestFilledPNG(t, 200, 150, base, nil)
	// One pixel outside Maestro's color tolerance: 1/30000 → 99.996667% match,
	// which rounds to "100.00" at the old two decimals.
	captured := encodeTestFilledPNG(t, 200, 150, base, &imagecolor.RGBA{R: 250, G: 100, B: 100, A: 255})

	if err := os.WriteFile(filepath.Join(tmpDir, "reference.png"), reference, 0o644); err != nil {
		t.Fatalf("write reference image: %v", err)
	}

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if _, ok := step.(*flow.AssertScreenshotStep); ok {
				return &core.CommandResult{Success: true, Data: captured}
			}
			return &core.CommandResult{Success: true}
		},
	}
	runner := New(driver, RunnerConfig{
		OutputDir:   filepath.Join(tmpDir, "output"),
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
		App:         report.App{ID: "com.test"},
		Parallelism: 0,
	})
	flows := []flow.Flow{{
		SourcePath: filepath.Join(tmpDir, "test.yaml"),
		Config:     flow.Config{Name: "Assert Screenshot Near Miss"},
		Steps: []flow.Step{&flow.AssertScreenshotStep{
			BaseStep:            flow.BaseStep{StepType: flow.StepAssertScreenshot},
			Path:                "reference",
			ThresholdPercentage: 100,
		}},
	}}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != report.StatusFailed {
		t.Fatalf("Status = %v, want %v", result.Status, report.StatusFailed)
	}

	msg := result.FlowResults[0].Error
	for _, want := range []string{"99.997%", "threshold: 100.000%", "1 of 30000 pixels differ"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}

func TestRunner_AssertScreenshotStep_SeedsMissingBaseline(t *testing.T) {
	tmpDir := t.TempDir()
	captured := encodeTestPNG(t, []imagecolor.RGBA{
		{R: 10, A: 255},
		{G: 20, A: 255},
	})

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if _, ok := step.(*flow.AssertScreenshotStep); ok {
				return &core.CommandResult{Success: true, Data: captured}
			}
			return &core.CommandResult{Success: true}
		},
	}
	runner := New(driver, RunnerConfig{
		OutputDir:   filepath.Join(tmpDir, "output"),
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
		App:         report.App{ID: "com.test"},
		Parallelism: 0,
	})
	flows := []flow.Flow{{
		SourcePath: filepath.Join(tmpDir, "test.yaml"),
		Config:     flow.Config{Name: "Seed Baseline Test"},
		Steps: []flow.Step{&flow.AssertScreenshotStep{
			BaseStep:            flow.BaseStep{StepType: flow.StepAssertScreenshot},
			Path:                "baselines/new-screen",
			ThresholdPercentage: 100,
		}},
	}}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != report.StatusPassed {
		t.Fatalf("Status = %v, want %v", result.Status, report.StatusPassed)
	}

	baselinePath := filepath.Join(tmpDir, "baselines", "new-screen.png")
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("expected baseline at %s: %v", baselinePath, err)
	}
	if !bytes.Equal(data, captured) {
		t.Error("seeded baseline does not match captured screenshot")
	}
}

func TestRunner_AssertScreenshotStep_UpdateScreenshotsOverwritesBaseline(t *testing.T) {
	tmpDir := t.TempDir()
	oldBaseline := encodeTestPNG(t, []imagecolor.RGBA{
		{R: 255, A: 255},
		{G: 255, A: 255},
	})
	updated := encodeTestPNG(t, []imagecolor.RGBA{
		{B: 255, A: 255},
		{G: 255, A: 255},
	})
	baselinePath := filepath.Join(tmpDir, "reference.png")
	if err := os.WriteFile(baselinePath, oldBaseline, 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if _, ok := step.(*flow.AssertScreenshotStep); ok {
				return &core.CommandResult{Success: true, Data: updated}
			}
			return &core.CommandResult{Success: true}
		},
	}
	runner := New(driver, RunnerConfig{
		OutputDir:         filepath.Join(tmpDir, "output"),
		Artifacts:         ArtifactNever,
		UpdateScreenshots: true,
		Device:            report.Device{ID: "test", Platform: "android"},
		App:               report.App{ID: "com.test"},
		Parallelism:       0,
	})
	flows := []flow.Flow{{
		SourcePath: filepath.Join(tmpDir, "test.yaml"),
		Config:     flow.Config{Name: "Update Baseline Test"},
		Steps: []flow.Step{&flow.AssertScreenshotStep{
			BaseStep:            flow.BaseStep{StepType: flow.StepAssertScreenshot},
			Path:                "reference",
			ThresholdPercentage: 100,
		}},
	}}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != report.StatusPassed {
		t.Fatalf("Status = %v, want %v", result.Status, report.StatusPassed)
	}

	data, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read updated baseline: %v", err)
	}
	if !bytes.Equal(data, updated) {
		t.Error("baseline was not overwritten by --update-screenshots")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "reference_diff.png")); err == nil {
		t.Error("unexpected diff image when updating screenshots")
	}
}

// encodeTestFilledPNG builds a w×h PNG filled with fill, optionally setting
// pixel (0,0) to a different color — enough to exercise near-100% matches.
func encodeTestFilledPNG(t *testing.T, w, h int, fill imagecolor.RGBA, corner *imagecolor.RGBA) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
	if corner != nil {
		img.SetRGBA(0, 0, *corner)
	}

	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	return data.Bytes()
}

func encodeTestPNG(t *testing.T, pixels []imagecolor.RGBA) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, len(pixels), 1))
	for x, pixel := range pixels {
		img.SetRGBA(x, 0, pixel)
	}

	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	return data.Bytes()
}

// TestRunner_StepPlatformGate verifies a step restricted to another platform is
// skipped (its driver Execute is never called) while unrestricted and
// matching-platform steps run. Maestro #1353.
func TestRunner_StepPlatformGate(t *testing.T) {
	tmpDir := t.TempDir()

	var executed []flow.StepType
	driver := &mockDriver{
		platformFunc: func() *core.PlatformInfo { return &core.PlatformInfo{Platform: "android", DeviceID: "d"} },
		executeFunc: func(step flow.Step) *core.CommandResult {
			executed = append(executed, step.Type())
			return &core.CommandResult{Success: true}
		},
	}

	runner := New(driver, RunnerConfig{
		OutputDir: tmpDir, Artifacts: ArtifactNever,
		Device: report.Device{ID: "d", Platform: "android"}, App: report.App{ID: "com.test"},
		RunnerVersion: "1.0.0", DriverName: "mock",
	})

	flows := []flow.Flow{{
		SourcePath: "t.yaml", Config: flow.Config{Name: "gate"},
		Steps: []flow.Step{
			&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}},
			&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn, Platform: "iOS"}},   // skipped on android
			&flow.BackStep{BaseStep: flow.BaseStep{StepType: flow.StepBack, Platform: "Android"}}, // runs on android
			&flow.AssertVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertVisible}},    // no gate, runs
		},
	}}

	if _, err := runner.Run(context.Background(), flows); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, e := range executed {
		if e == flow.StepTapOn {
			t.Errorf("iOS-gated tapOn should have been skipped on android, but it executed")
		}
	}
	hasBack, hasAssert := false, false
	for _, e := range executed {
		if e == flow.StepBack {
			hasBack = true
		}
		if e == flow.StepAssertVisible {
			hasAssert = true
		}
	}
	if !hasBack {
		t.Error("android-gated back should have run on android")
	}
	if !hasAssert {
		t.Error("ungated assertVisible should have run")
	}
}

// TestRunner_AssertScreenshotStep_ClearsStaleDiffOnSizeMismatch covers the
// second half of #138. When the captured crop differs in size from the
// baseline, the comparison never runs and therefore writes no diff image — so a
// _diff.png from an earlier run survives and shows the wrong thing. Users
// following the "check the diff image" hint saw a picture identical to the
// capture and concluded the runner was wrong about the failure.
func TestRunner_AssertScreenshotStep_ClearsStaleDiffOnSizeMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	base := imagecolor.RGBA{R: 10, G: 20, B: 30, A: 255}
	reference := encodeTestFilledPNG(t, 200, 150, base, nil)
	// One pixel taller — the cropOn drift the issue reports (498x65 vs 498x66).
	captured := encodeTestFilledPNG(t, 200, 151, base, nil)

	referencePath := filepath.Join(tmpDir, "reference.png")
	if err := os.WriteFile(referencePath, reference, 0o644); err != nil {
		t.Fatalf("write reference image: %v", err)
	}
	// A diff left behind by an earlier failing run.
	staleDiff := core.DiffScreenshotPath(referencePath)
	if err := os.WriteFile(staleDiff, reference, 0o644); err != nil {
		t.Fatalf("write stale diff: %v", err)
	}

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if _, ok := step.(*flow.AssertScreenshotStep); ok {
				return &core.CommandResult{Success: true, Data: captured}
			}
			return &core.CommandResult{Success: true}
		},
	}
	runner := New(driver, RunnerConfig{
		OutputDir:   filepath.Join(tmpDir, "output"),
		Artifacts:   ArtifactNever,
		Device:      report.Device{ID: "test", Platform: "android"},
		App:         report.App{ID: "com.test"},
		Parallelism: 0,
	})
	flows := []flow.Flow{{
		SourcePath: filepath.Join(tmpDir, "test.yaml"),
		Config:     flow.Config{Name: "Assert Screenshot Size Mismatch"},
		Steps: []flow.Step{&flow.AssertScreenshotStep{
			BaseStep:            flow.BaseStep{StepType: flow.StepAssertScreenshot},
			Path:                "reference",
			ThresholdPercentage: 100,
			CropOn:              &flow.Selector{ID: "editor-input"},
		}},
	}}

	result, err := runner.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != report.StatusFailed {
		t.Fatalf("Status = %v, want %v", result.Status, report.StatusFailed)
	}
	if _, statErr := os.Stat(staleDiff); !os.IsNotExist(statErr) {
		t.Errorf("stale diff %s must be removed so it can't contradict the error", staleDiff)
	}

	msg := result.FlowResults[0].Error
	if !strings.Contains(msg, "size mismatch") {
		t.Errorf("error should name the size mismatch, got %q", msg)
	}
	if !strings.Contains(msg, "cropOn element rendered at a different size") {
		t.Errorf("a cropOn assertion should explain why sizes drift, got %q", msg)
	}
}
