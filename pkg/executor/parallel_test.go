package executor

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

func TestColor(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"reset", colorReset, colorReset},
		{"green", colorGreen, colorGreen},
		{"red", colorRed, colorRed},
		{"gray", colorGray, colorGray},
		{"cyan", colorCyan, colorCyan},
		{"empty string", "", ""},
		{"arbitrary string", "hello", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := color(tt.input)
			if got != tt.want {
				t.Errorf("color(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		ms       int64
		expected string
	}{
		{"zero ms", 0, "0ms"},
		{"small ms", 100, "100ms"},
		{"under 1s", 999, "999ms"},
		{"exactly 1s", 1000, "1.0s"},
		{"1.5s", 1500, "1.5s"},
		{"under 1min", 59000, "59.0s"},
		{"exactly 1min", 60000, "1m0s"},
		{"1min 30s", 90000, "1m30s"},
		{"2min 15s", 135000, "2m15s"},
		{"10min", 600000, "10m0s"},
		{"large value", 3661000, "61m1s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.ms)
			if got != tt.expected {
				t.Errorf("formatDuration(%d) = %q, want %q", tt.ms, got, tt.expected)
			}
		})
	}
}

func TestFormatDeviceLabel(t *testing.T) {
	tests := []struct {
		name     string
		device   *report.Device
		expected string
	}{
		{
			name:     "nil device",
			device:   nil,
			expected: "Unknown",
		},
		{
			name: "device with name",
			device: &report.Device{
				ID:   "emulator-5554",
				Name: "Pixel 6",
			},
			expected: "Pixel 6",
		},
		{
			name: "device with empty name",
			device: &report.Device{
				ID:   "emulator-5554",
				Name: "",
			},
			expected: "",
		},
		{
			name: "full device info",
			device: &report.Device{
				ID:          "ABC123",
				Name:        "iPhone 15 Pro",
				Platform:    "ios",
				OSVersion:   "17.0",
				IsSimulator: true,
			},
			expected: "iPhone 15 Pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDeviceLabel(tt.device)
			if got != tt.expected {
				t.Errorf("formatDeviceLabel() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildRunResult(t *testing.T) {
	pr := &ParallelRunner{}

	t.Run("all passed", func(t *testing.T) {
		results := []FlowResult{
			{Status: report.StatusPassed},
			{Status: report.StatusPassed},
		}
		got := pr.buildRunResult(results, 5000)

		if got.Status != report.StatusPassed {
			t.Errorf("Status = %v, want %v", got.Status, report.StatusPassed)
		}
		if got.TotalFlows != 2 {
			t.Errorf("TotalFlows = %d, want 2", got.TotalFlows)
		}
		if got.PassedFlows != 2 {
			t.Errorf("PassedFlows = %d, want 2", got.PassedFlows)
		}
		if got.FailedFlows != 0 {
			t.Errorf("FailedFlows = %d, want 0", got.FailedFlows)
		}
		if got.Duration != 5000 {
			t.Errorf("Duration = %d, want 5000", got.Duration)
		}
	})

	t.Run("with failures", func(t *testing.T) {
		results := []FlowResult{
			{Status: report.StatusPassed},
			{Status: report.StatusFailed},
			{Status: report.StatusPassed},
		}
		got := pr.buildRunResult(results, 10000)

		if got.Status != report.StatusFailed {
			t.Errorf("Status = %v, want %v", got.Status, report.StatusFailed)
		}
		if got.PassedFlows != 2 {
			t.Errorf("PassedFlows = %d, want 2", got.PassedFlows)
		}
		if got.FailedFlows != 1 {
			t.Errorf("FailedFlows = %d, want 1", got.FailedFlows)
		}
	})

	t.Run("with skipped", func(t *testing.T) {
		results := []FlowResult{
			{Status: report.StatusPassed},
			{Status: report.StatusSkipped},
		}
		got := pr.buildRunResult(results, 3000)

		if got.Status != report.StatusPassed {
			t.Errorf("Status = %v, want %v", got.Status, report.StatusPassed)
		}
		if got.SkippedFlows != 1 {
			t.Errorf("SkippedFlows = %d, want 1", got.SkippedFlows)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		results := []FlowResult{}
		got := pr.buildRunResult(results, 0)

		if got.TotalFlows != 0 {
			t.Errorf("TotalFlows = %d, want 0", got.TotalFlows)
		}
		if got.Status != report.StatusPassed {
			t.Errorf("Status = %v, want %v", got.Status, report.StatusPassed)
		}
	})

	t.Run("all failed", func(t *testing.T) {
		results := []FlowResult{
			{Status: report.StatusFailed},
			{Status: report.StatusFailed},
		}
		got := pr.buildRunResult(results, 2000)

		if got.Status != report.StatusFailed {
			t.Errorf("Status = %v, want %v", got.Status, report.StatusFailed)
		}
		if got.FailedFlows != 2 {
			t.Errorf("FailedFlows = %d, want 2", got.FailedFlows)
		}
	})
}

// TestParallelRunner_PerWorkerCloudHooksAndSessionTagging covers the wiring
// that powers cloud-provider per-job reporting in parallel mode:
//   - each FlowResult is tagged with the SessionID of the worker that ran it
//   - each worker's DeviceWorker.OnFlowStart / OnFlowEnd fires for the flows
//     that worker picked up (and only those)
func TestParallelRunner_PerWorkerCloudHooksAndSessionTagging(t *testing.T) {
	tmpDir := t.TempDir()

	// Two flows, two workers. The work queue is FIFO, so worker A grabs
	// whichever item it sees first; we don't assume an ordering — we only
	// assert each flow ends up tagged with one of the two session IDs and
	// that the per-worker hooks recorded the matching flow names.
	flows := []flow.Flow{
		{
			SourcePath: "flow_a.yaml",
			Config:     flow.Config{Name: "Flow A"},
			Steps: []flow.Step{
				&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}},
			},
		},
		{
			SourcePath: "flow_b.yaml",
			Config:     flow.Config{Name: "Flow B"},
			Steps: []flow.Step{
				&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}},
			},
		},
	}

	type hookCall struct {
		event string
		flow  string
	}
	var (
		mu             sync.Mutex
		callsBySession = map[string][]hookCall{}
	)
	record := func(sessionID, event, flowName string) {
		mu.Lock()
		defer mu.Unlock()
		callsBySession[sessionID] = append(callsBySession[sessionID], hookCall{event: event, flow: flowName})
	}

	driverA := &mockDriver{
		executeFunc:  func(step flow.Step) *core.CommandResult { return &core.CommandResult{Success: true} },
		platformFunc: func() *core.PlatformInfo { return &core.PlatformInfo{Platform: "ios", DeviceID: "dev-a"} },
	}
	driverB := &mockDriver{
		executeFunc:  func(step flow.Step) *core.CommandResult { return &core.CommandResult{Success: true} },
		platformFunc: func() *core.PlatformInfo { return &core.PlatformInfo{Platform: "ios", DeviceID: "dev-b"} },
	}

	const sessA = "session-A"
	const sessB = "session-B"

	workers := []DeviceWorker{
		{
			ID: 0, DeviceID: "dev-a", SessionID: sessA, Driver: driverA, Cleanup: func() {},
			OnFlowStart: func(_, _ int, name, _ string) { record(sessA, "start", name) },
			OnFlowEnd:   func(name string, _ bool, _ int64, _ string) { record(sessA, "end", name) },
		},
		{
			ID: 1, DeviceID: "dev-b", SessionID: sessB, Driver: driverB, Cleanup: func() {},
			OnFlowStart: func(_, _ int, name, _ string) { record(sessB, "start", name) },
			OnFlowEnd:   func(name string, _ bool, _ int64, _ string) { record(sessB, "end", name) },
		},
	}

	pr := NewParallelRunner(workers, RunnerConfig{
		OutputDir:     tmpDir,
		Artifacts:     ArtifactNever,
		Device:        report.Device{Name: "2 devices", Platform: "ios"},
		App:           report.App{ID: "com.test"},
		RunnerVersion: "test",
		DriverName:    "mock",
	})

	result, err := pr.Run(context.Background(), flows)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.TotalFlows != 2 {
		t.Fatalf("TotalFlows = %d, want 2", result.TotalFlows)
	}

	// Every flow result must carry the session ID of the worker that ran it.
	flowsBySession := map[string][]string{}
	for _, fr := range result.FlowResults {
		if fr.SessionID != sessA && fr.SessionID != sessB {
			t.Errorf("FlowResult %q has SessionID = %q, want %q or %q", fr.Name, fr.SessionID, sessA, sessB)
		}
		flowsBySession[fr.SessionID] = append(flowsBySession[fr.SessionID], fr.Name)
	}

	// Hook calls must match the flows attributed to each session, with both
	// start and end events recorded for every flow that worker ran.
	for _, sess := range []string{sessA, sessB} {
		ranOnSess := flowsBySession[sess]
		var startNames, endNames []string
		for _, c := range callsBySession[sess] {
			if c.event == "start" {
				startNames = append(startNames, c.flow)
			} else if c.event == "end" {
				endNames = append(endNames, c.flow)
			}
		}
		sort.Strings(ranOnSess)
		sort.Strings(startNames)
		sort.Strings(endNames)
		if !equalStringSlices(startNames, ranOnSess) {
			t.Errorf("session %s OnFlowStart names = %v, want %v", sess, startNames, ranOnSess)
		}
		if !equalStringSlices(endNames, ranOnSess) {
			t.Errorf("session %s OnFlowEnd names = %v, want %v", sess, endNames, ranOnSess)
		}
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
