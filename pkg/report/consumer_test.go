package report

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestReport(t *testing.T) string {
	tmpDir := t.TempDir()

	// Create directory structure
	os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755)
	os.MkdirAll(filepath.Join(tmpDir, "assets", "flow-000"), 0o755)

	// Create index
	index := &Index{
		Version:   Version,
		Status:    StatusRunning,
		UpdateSeq: 1,
		Flows: []FlowEntry{
			{
				ID:        "flow-000",
				Name:      "Test Flow",
				Status:    StatusRunning,
				DataFile:  "flows/flow-000.json",
				UpdateSeq: 1,
			},
		},
		Summary: Summary{Total: 1, Running: 1},
	}
	atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index)

	// Create flow detail
	flow := &FlowDetail{
		ID:   "flow-000",
		Name: "Test Flow",
		Commands: []Command{
			{Index: 0, Type: "launchApp", Status: StatusPassed},
			{Index: 1, Type: "tapOn", Status: StatusRunning},
		},
	}
	atomicWriteJSON(filepath.Join(tmpDir, "flows", "flow-000.json"), flow)

	return tmpDir
}

func TestNewConsumer(t *testing.T) {
	c := NewConsumer("/tmp/report")

	if c.reportDir != "/tmp/report" {
		t.Errorf("reportDir = %q, want %q", c.reportDir, "/tmp/report")
	}
	if c.lastFlowSeq == nil {
		t.Error("lastFlowSeq not initialized")
	}
}

func TestConsumer_Poll_FirstTime(t *testing.T) {
	tmpDir := setupTestReport(t)
	c := NewConsumer(tmpDir)

	changed, index, err := c.Poll()
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	if index == nil {
		t.Fatal("index is nil")
	}
	if len(changed) != 1 {
		t.Errorf("len(changed) = %d, want 1", len(changed))
	}
	if changed[0] != "flow-000" {
		t.Errorf("changed[0] = %q, want %q", changed[0], "flow-000")
	}
}

func TestConsumer_Poll_NoChange(t *testing.T) {
	tmpDir := setupTestReport(t)
	c := NewConsumer(tmpDir)

	// First poll
	c.Poll()

	// Second poll with no changes
	changed, index, err := c.Poll()
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	if index == nil {
		t.Fatal("index is nil")
	}
	if len(changed) != 0 {
		t.Errorf("len(changed) = %d, want 0", len(changed))
	}
}

func TestConsumer_Poll_WithChange(t *testing.T) {
	tmpDir := setupTestReport(t)
	c := NewConsumer(tmpDir)

	// First poll
	c.Poll()

	// Update the index
	index, _ := ReadIndex(filepath.Join(tmpDir, "report.json"))
	index.UpdateSeq = 2
	index.Flows[0].UpdateSeq = 2
	index.Flows[0].Status = StatusPassed
	atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index)

	// Second poll should detect change
	changed, _, err := c.Poll()
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	if len(changed) != 1 {
		t.Errorf("len(changed) = %d, want 1", len(changed))
	}
}

func TestConsumer_ReadIndex(t *testing.T) {
	tmpDir := setupTestReport(t)
	c := NewConsumer(tmpDir)

	index, err := c.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}

	if index.Version != Version {
		t.Errorf("Version = %q, want %q", index.Version, Version)
	}
}

func TestConsumer_ReadFlow(t *testing.T) {
	tmpDir := setupTestReport(t)
	c := NewConsumer(tmpDir)

	flow, err := c.ReadFlow("flow-000")
	if err != nil {
		t.Fatalf("ReadFlow() error = %v", err)
	}

	if flow.ID != "flow-000" {
		t.Errorf("ID = %q, want %q", flow.ID, "flow-000")
	}
	if len(flow.Commands) != 2 {
		t.Errorf("len(Commands) = %d, want 2", len(flow.Commands))
	}
}

func TestConsumer_Reset(t *testing.T) {
	tmpDir := setupTestReport(t)
	c := NewConsumer(tmpDir)

	// First poll
	c.Poll()

	// Reset
	c.Reset()

	if c.lastGlobalSeq != 0 {
		t.Errorf("lastGlobalSeq = %d, want 0", c.lastGlobalSeq)
	}
	if len(c.lastFlowSeq) != 0 {
		t.Errorf("len(lastFlowSeq) = %d, want 0", len(c.lastFlowSeq))
	}

	// Poll should return changes again
	changed, _, _ := c.Poll()
	if len(changed) != 1 {
		t.Errorf("len(changed) = %d, want 1", len(changed))
	}
}

func TestReadReport(t *testing.T) {
	tmpDir := setupTestReport(t)

	index, flows, err := ReadReport(tmpDir)
	if err != nil {
		t.Fatalf("ReadReport() error = %v", err)
	}

	if index == nil {
		t.Fatal("index is nil")
	}
	if len(flows) != 1 {
		t.Fatalf("len(flows) = %d, want 1", len(flows))
	}
	if flows[0].ID != "flow-000" {
		t.Errorf("flows[0].ID = %q, want %q", flows[0].ID, "flow-000")
	}
}

func TestRecover_RunningFlow(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755)

	// Create index with running flow
	index := &Index{
		Version:   Version,
		Status:    StatusRunning,
		UpdateSeq: 1,
		Flows: []FlowEntry{
			{
				ID:       "flow-000",
				Status:   StatusRunning,
				DataFile: "flows/flow-000.json",
			},
		},
		Summary: Summary{Total: 1, Running: 1},
	}
	atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index)

	// Create flow with completed commands
	flow := &FlowDetail{
		ID: "flow-000",
		Commands: []Command{
			{Index: 0, Status: StatusPassed},
			{Index: 1, Status: StatusPassed},
		},
	}
	atomicWriteJSON(filepath.Join(tmpDir, "flows", "flow-000.json"), flow)

	// Recover
	err := Recover(tmpDir)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}

	// Check index was updated
	recovered, _ := ReadIndex(filepath.Join(tmpDir, "report.json"))
	if recovered.Flows[0].Status != StatusPassed {
		t.Errorf("Flows[0].Status = %q, want %q", recovered.Flows[0].Status, StatusPassed)
	}
	if recovered.Summary.Passed != 1 {
		t.Errorf("Summary.Passed = %d, want 1", recovered.Summary.Passed)
	}
}

func TestRecover_InterruptedFlow(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755)

	// Create index with running flow
	index := &Index{
		Version:   Version,
		Status:    StatusRunning,
		UpdateSeq: 1,
		Flows: []FlowEntry{
			{
				ID:       "flow-000",
				Status:   StatusRunning,
				DataFile: "flows/flow-000.json",
			},
		},
	}
	atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index)

	// Create flow with some commands still running
	flow := &FlowDetail{
		ID: "flow-000",
		Commands: []Command{
			{Index: 0, Status: StatusPassed},
			{Index: 1, Status: StatusRunning},
			{Index: 2, Status: StatusPending},
		},
	}
	atomicWriteJSON(filepath.Join(tmpDir, "flows", "flow-000.json"), flow)

	// Recover
	err := Recover(tmpDir)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}

	// Check index was updated
	recovered, _ := ReadIndex(filepath.Join(tmpDir, "report.json"))
	if recovered.Flows[0].Status != StatusFailed {
		t.Errorf("Flows[0].Status = %q, want %q", recovered.Flows[0].Status, StatusFailed)
	}
	if recovered.Flows[0].Error == nil || *recovered.Flows[0].Error != "Flow interrupted" {
		t.Errorf("Flows[0].Error = %v, want %q", recovered.Flows[0].Error, "Flow interrupted")
	}
}

func TestRecover_MissingFlowFile(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755)

	// Create index with running flow but no flow file
	index := &Index{
		Version:   Version,
		Status:    StatusRunning,
		UpdateSeq: 1,
		Flows: []FlowEntry{
			{
				ID:       "flow-000",
				Status:   StatusRunning,
				DataFile: "flows/flow-000.json",
			},
		},
	}
	atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index)

	// Recover (no flow file exists)
	err := Recover(tmpDir)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}

	// Check index was updated to failed
	recovered, _ := ReadIndex(filepath.Join(tmpDir, "report.json"))
	if recovered.Flows[0].Status != StatusFailed {
		t.Errorf("Flows[0].Status = %q, want %q", recovered.Flows[0].Status, StatusFailed)
	}
}

func TestRecover_NoChangesNeeded(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755)

	// Create index with completed flow
	index := &Index{
		Version:   Version,
		Status:    StatusPassed,
		UpdateSeq: 1,
		Flows: []FlowEntry{
			{
				ID:       "flow-000",
				Status:   StatusPassed,
				DataFile: "flows/flow-000.json",
			},
		},
		Summary: Summary{Total: 1, Passed: 1},
	}
	originalSeq := index.UpdateSeq
	atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index)

	// Recover should not change anything
	err := Recover(tmpDir)
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}

	recovered, _ := ReadIndex(filepath.Join(tmpDir, "report.json"))
	if recovered.UpdateSeq != originalSeq {
		t.Errorf("UpdateSeq changed from %d to %d", originalSeq, recovered.UpdateSeq)
	}
}

func TestInferStatus(t *testing.T) {
	tests := []struct {
		name     string
		commands []Command
		expected Status
	}{
		{
			name:     "empty commands",
			commands: []Command{},
			expected: StatusFailed,
		},
		{
			name: "all passed",
			commands: []Command{
				{Status: StatusPassed},
				{Status: StatusPassed},
			},
			expected: StatusPassed,
		},
		{
			name: "one failed",
			commands: []Command{
				{Status: StatusPassed},
				{Status: StatusFailed},
			},
			expected: StatusFailed,
		},
		{
			name: "incomplete",
			commands: []Command{
				{Status: StatusPassed},
				{Status: StatusPending},
			},
			expected: StatusRunning,
		},
		{
			name: "still running",
			commands: []Command{
				{Status: StatusPassed},
				{Status: StatusRunning},
			},
			expected: StatusRunning,
		},
		{
			name: "all skipped",
			commands: []Command{
				{Status: StatusSkipped},
				{Status: StatusSkipped},
			},
			expected: StatusRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferStatus(tt.commands)
			if got != tt.expected {
				t.Errorf("inferStatus() = %q, want %q", got, tt.expected)
			}
		})
	}
}
