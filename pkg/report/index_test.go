package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewIndexWriter(t *testing.T) {
	tmpDir := t.TempDir()

	index := &Index{
		Version: Version,
		Status:  StatusPending,
		Flows: []FlowEntry{
			{ID: "flow-000", Name: "Test Flow", Status: StatusPending},
		},
	}

	w := NewIndexWriter(tmpDir, index)
	defer w.Close()

	if w.path != filepath.Join(tmpDir, "report.json") {
		t.Errorf("path = %q, want %q", w.path, filepath.Join(tmpDir, "report.json"))
	}
	if w.index != index {
		t.Error("index not stored correctly")
	}
}

func TestIndexWriter_Start(t *testing.T) {
	tmpDir := t.TempDir()

	index := &Index{
		Version: Version,
		Status:  StatusPending,
		Flows:   []FlowEntry{{ID: "flow-000", Status: StatusPending}},
	}

	w := NewIndexWriter(tmpDir, index)
	defer w.Close()

	before := time.Now()
	w.Start()
	after := time.Now()

	if index.Status != StatusRunning {
		t.Errorf("Status = %q, want %q", index.Status, StatusRunning)
	}
	if index.StartTime.Before(before) || index.StartTime.After(after) {
		t.Errorf("StartTime not set correctly")
	}
	if index.UpdateSeq < 1 {
		t.Errorf("UpdateSeq = %d, want >= 1", index.UpdateSeq)
	}

	// Check file was written
	if _, err := os.Stat(filepath.Join(tmpDir, "report.json")); err != nil {
		t.Errorf("report.json not created: %v", err)
	}
}

func TestIndexWriter_UpdateFlow_Terminal(t *testing.T) {
	tmpDir := t.TempDir()

	index := &Index{
		Version: Version,
		Status:  StatusRunning,
		Flows: []FlowEntry{
			{ID: "flow-000", Status: StatusPending},
			{ID: "flow-001", Status: StatusPending},
		},
	}

	w := NewIndexWriter(tmpDir, index)
	defer w.Close()

	// Terminal update should flush immediately
	now := time.Now()
	duration := int64(1000)
	w.UpdateFlow("flow-000", &FlowUpdate{
		Status:   StatusPassed,
		EndTime:  &now,
		Duration: &duration,
		Commands: CommandSummary{Total: 5, Passed: 5},
	})

	// Give a tiny bit of time for async operations
	time.Sleep(10 * time.Millisecond)

	// Read back and verify
	readIndex, err := ReadIndex(filepath.Join(tmpDir, "report.json"))
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}

	if readIndex.Flows[0].Status != StatusPassed {
		t.Errorf("Flows[0].Status = %q, want %q", readIndex.Flows[0].Status, StatusPassed)
	}
	if readIndex.Summary.Passed != 1 {
		t.Errorf("Summary.Passed = %d, want 1", readIndex.Summary.Passed)
	}
}

func TestIndexWriter_UpdateFlow_Progress(t *testing.T) {
	tmpDir := t.TempDir()

	index := &Index{
		Version: Version,
		Status:  StatusRunning,
		Flows: []FlowEntry{
			{ID: "flow-000", Status: StatusPending},
		},
	}

	w := NewIndexWriter(tmpDir, index)
	defer w.Close()

	// Progress update should be debounced
	w.UpdateFlow("flow-000", &FlowUpdate{
		Status:   StatusRunning,
		Commands: CommandSummary{Total: 5, Running: 1, Pending: 4},
	})

	// Wait for debounce timer (100ms + buffer)
	time.Sleep(150 * time.Millisecond)

	// Read back and verify
	readIndex, err := ReadIndex(filepath.Join(tmpDir, "report.json"))
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}

	if readIndex.Flows[0].Status != StatusRunning {
		t.Errorf("Flows[0].Status = %q, want %q", readIndex.Flows[0].Status, StatusRunning)
	}
}

func TestIndexWriter_End(t *testing.T) {
	tmpDir := t.TempDir()

	index := &Index{
		Version:   Version,
		Status:    StatusRunning,
		StartTime: time.Now(),
		Flows: []FlowEntry{
			{ID: "flow-000", Status: StatusPassed},
			{ID: "flow-001", Status: StatusPassed},
		},
	}

	w := NewIndexWriter(tmpDir, index)
	defer w.Close()

	w.End()

	if index.Status != StatusPassed {
		t.Errorf("Status = %q, want %q", index.Status, StatusPassed)
	}
	if index.EndTime == nil {
		t.Error("EndTime not set")
	}
}

func TestIndexWriter_End_WithFailure(t *testing.T) {
	tmpDir := t.TempDir()

	index := &Index{
		Version:   Version,
		Status:    StatusRunning,
		StartTime: time.Now(),
		Flows: []FlowEntry{
			{ID: "flow-000", Status: StatusPassed},
			{ID: "flow-001", Status: StatusFailed},
			{ID: "flow-002", Status: StatusPassed},
		},
	}

	w := NewIndexWriter(tmpDir, index)
	defer w.Close()

	w.End()

	if index.Status != StatusFailed {
		t.Errorf("Status = %q, want %q", index.Status, StatusFailed)
	}
}

func TestIndexWriter_GetIndex(t *testing.T) {
	tmpDir := t.TempDir()

	index := &Index{
		Version: Version,
		Status:  StatusPending,
	}

	w := NewIndexWriter(tmpDir, index)
	defer w.Close()

	got := w.GetIndex()
	if got != index {
		t.Error("GetIndex() did not return the same index")
	}
}

func TestIndexWriter_RecordAttempt(t *testing.T) {
	tmpDir := t.TempDir()

	index := &Index{
		Version: Version,
		Status:  StatusRunning,
		Flows: []FlowEntry{
			{ID: "flow-000", Status: StatusRunning},
		},
	}

	w := NewIndexWriter(tmpDir, index)
	defer w.Close()

	w.RecordAttempt("flow-000", 1, StatusFailed, 5000, "timeout", "flows/flow-000-attempt-1.json")

	if index.Flows[0].Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", index.Flows[0].Attempts)
	}
	if len(index.Flows[0].AttemptHistory) != 1 {
		t.Fatalf("AttemptHistory length = %d, want 1", len(index.Flows[0].AttemptHistory))
	}
	if index.Flows[0].AttemptHistory[0].Error != "timeout" {
		t.Errorf("AttemptHistory[0].Error = %q, want %q", index.Flows[0].AttemptHistory[0].Error, "timeout")
	}
}

func TestComputeRunStatus(t *testing.T) {
	tests := []struct {
		name     string
		flows    []FlowEntry
		expected Status
	}{
		{
			name:     "all passed",
			flows:    []FlowEntry{{Status: StatusPassed}, {Status: StatusPassed}},
			expected: StatusPassed,
		},
		{
			name:     "one failed",
			flows:    []FlowEntry{{Status: StatusPassed}, {Status: StatusFailed}},
			expected: StatusFailed,
		},
		{
			name:     "some pending",
			flows:    []FlowEntry{{Status: StatusPassed}, {Status: StatusPending}},
			expected: StatusRunning,
		},
		{
			name:     "some running",
			flows:    []FlowEntry{{Status: StatusPassed}, {Status: StatusRunning}},
			expected: StatusRunning,
		},
		{
			name:     "all skipped",
			flows:    []FlowEntry{{Status: StatusSkipped}, {Status: StatusSkipped}},
			expected: StatusPassed,
		},
		{
			name:     "mixed terminal",
			flows:    []FlowEntry{{Status: StatusPassed}, {Status: StatusSkipped}, {Status: StatusFailed}},
			expected: StatusFailed,
		},
	}

	tmpDir := t.TempDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := &Index{
				Version: Version,
				Status:  StatusRunning,
				Flows:   tt.flows,
			}
			w := NewIndexWriter(tmpDir, index)
			defer w.Close()

			got := w.computeRunStatus()
			if got != tt.expected {
				t.Errorf("computeRunStatus() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestComputeSummary(t *testing.T) {
	tmpDir := t.TempDir()

	index := &Index{
		Version: Version,
		Status:  StatusRunning,
		Flows: []FlowEntry{
			{Status: StatusPassed},
			{Status: StatusPassed},
			{Status: StatusFailed},
			{Status: StatusSkipped},
			{Status: StatusRunning},
			{Status: StatusPending},
		},
	}

	w := NewIndexWriter(tmpDir, index)
	defer w.Close()

	summary := w.computeSummary()

	if summary.Total != 6 {
		t.Errorf("Total = %d, want 6", summary.Total)
	}
	if summary.Passed != 2 {
		t.Errorf("Passed = %d, want 2", summary.Passed)
	}
	if summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1", summary.Failed)
	}
	if summary.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", summary.Skipped)
	}
	if summary.Running != 1 {
		t.Errorf("Running = %d, want 1", summary.Running)
	}
	if summary.Pending != 1 {
		t.Errorf("Pending = %d, want 1", summary.Pending)
	}
}
