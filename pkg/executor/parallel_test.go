package executor

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

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
