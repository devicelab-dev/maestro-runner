package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func createTestFlowWriter(t *testing.T) (*FlowWriter, *IndexWriter, string) {
	tmpDir := t.TempDir()

	index := &Index{
		Version: Version,
		Status:  StatusRunning,
		Flows: []FlowEntry{
			{ID: "flow-000", Name: "Test Flow", Status: StatusPending},
		},
	}

	indexWriter := NewIndexWriter(tmpDir, index)

	flowDetail := &FlowDetail{
		ID:   "flow-000",
		Name: "Test Flow",
		Commands: []Command{
			{Index: 0, Type: "launchApp", Status: StatusPending},
			{Index: 1, Type: "tapOn", Status: StatusPending},
			{Index: 2, Type: "assertVisible", Status: StatusPending},
		},
	}

	// Create flows directory
	os.MkdirAll(filepath.Join(tmpDir, "flows"), 0755)

	flowWriter := NewFlowWriter(flowDetail, tmpDir, indexWriter)

	return flowWriter, indexWriter, tmpDir
}

func TestNewFlowWriter(t *testing.T) {
	fw, iw, tmpDir := createTestFlowWriter(t)
	defer iw.Close()

	if fw.flow.ID != "flow-000" {
		t.Errorf("flow.ID = %q, want %q", fw.flow.ID, "flow-000")
	}

	expectedPath := filepath.Join(tmpDir, "flows", "flow-000.json")
	if fw.path != expectedPath {
		t.Errorf("path = %q, want %q", fw.path, expectedPath)
	}

	expectedAssetsDir := filepath.Join(tmpDir, "assets", "flow-000")
	if fw.assetsDir != expectedAssetsDir {
		t.Errorf("assetsDir = %q, want %q", fw.assetsDir, expectedAssetsDir)
	}

	// Check assets directory was created
	if _, err := os.Stat(expectedAssetsDir); err != nil {
		t.Errorf("assets directory not created: %v", err)
	}
}

func TestFlowWriter_Start(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	before := time.Now()
	fw.Start()
	after := time.Now()

	if fw.flow.StartTime.Before(before) || fw.flow.StartTime.After(after) {
		t.Error("StartTime not set correctly")
	}
}

func TestFlowWriter_CommandStart(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	fw.Start()

	before := time.Now()
	fw.CommandStart(0)
	after := time.Now()

	cmd := fw.flow.Commands[0]
	if cmd.Status != StatusRunning {
		t.Errorf("cmd.Status = %q, want %q", cmd.Status, StatusRunning)
	}
	if cmd.StartTime == nil {
		t.Error("StartTime not set")
	} else if cmd.StartTime.Before(before) || cmd.StartTime.After(after) {
		t.Error("StartTime not in expected range")
	}
}

func TestFlowWriter_CommandStart_InvalidIndex(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	// Should not panic with invalid index
	fw.CommandStart(-1)
	fw.CommandStart(100)
}

func TestFlowWriter_CommandEnd(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	fw.Start()
	fw.CommandStart(0)

	time.Sleep(10 * time.Millisecond)

	element := &Element{Found: true, ID: "login_btn", Class: "Button"}
	artifacts := CommandArtifacts{
		ScreenshotBefore: "assets/flow-000/cmd-000-before.png",
	}

	fw.CommandEnd(0, StatusPassed, element, nil, artifacts)

	cmd := fw.flow.Commands[0]
	if cmd.Status != StatusPassed {
		t.Errorf("cmd.Status = %q, want %q", cmd.Status, StatusPassed)
	}
	if cmd.EndTime == nil {
		t.Error("EndTime not set")
	}
	if cmd.Duration == nil || *cmd.Duration < 10 {
		t.Error("Duration not calculated correctly")
	}
	if cmd.Element == nil || cmd.Element.ID != "login_btn" || !cmd.Element.Found {
		t.Error("Element not set correctly")
	}
	if cmd.Artifacts.ScreenshotBefore != "assets/flow-000/cmd-000-before.png" {
		t.Errorf("Artifacts not set correctly")
	}
}

func TestFlowWriter_CommandEnd_WithError(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	fw.Start()
	fw.CommandStart(0)

	err := &Error{
		Type:    "ElementNotFound",
		Message: "Could not find element with id 'login_btn'",
	}

	fw.CommandEnd(0, StatusFailed, nil, err, CommandArtifacts{})

	cmd := fw.flow.Commands[0]
	if cmd.Status != StatusFailed {
		t.Errorf("cmd.Status = %q, want %q", cmd.Status, StatusFailed)
	}
	if cmd.Error == nil {
		t.Error("Error not set")
	} else if cmd.Error.Type != "ElementNotFound" {
		t.Errorf("Error.Type = %q, want %q", cmd.Error.Type, "ElementNotFound")
	}
}

func TestFlowWriter_End(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	fw.Start()
	fw.CommandStart(0)
	fw.CommandEnd(0, StatusPassed, nil, nil, CommandArtifacts{})
	fw.CommandStart(1)
	fw.CommandEnd(1, StatusPassed, nil, nil, CommandArtifacts{})
	fw.CommandStart(2)
	fw.CommandEnd(2, StatusPassed, nil, nil, CommandArtifacts{})

	fw.End(StatusPassed)

	if fw.flow.EndTime == nil {
		t.Error("EndTime not set")
	}
	if fw.flow.Duration == nil {
		t.Error("Duration not set")
	}
}

func TestFlowWriter_End_WithFailure(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	fw.Start()
	fw.CommandStart(0)
	fw.CommandEnd(0, StatusFailed, nil, &Error{Message: "Test error"}, CommandArtifacts{})

	fw.End(StatusFailed)

	// Wait for index update
	time.Sleep(50 * time.Millisecond)

	index := iw.GetIndex()
	if index.Flows[0].Status != StatusFailed {
		t.Errorf("index flow status = %q, want %q", index.Flows[0].Status, StatusFailed)
	}
}

func TestFlowWriter_SetFlowArtifacts(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	artifacts := FlowArtifacts{
		Video:     "assets/flow-000/video.mp4",
		DeviceLog: "assets/flow-000/device.log",
	}

	fw.SetFlowArtifacts(artifacts)

	if fw.flow.Artifacts.Video != "assets/flow-000/video.mp4" {
		t.Errorf("Video = %q, want %q", fw.flow.Artifacts.Video, "assets/flow-000/video.mp4")
	}
	if fw.flow.Artifacts.DeviceLog != "assets/flow-000/device.log" {
		t.Errorf("DeviceLog = %q, want %q", fw.flow.Artifacts.DeviceLog, "assets/flow-000/device.log")
	}
}

func TestFlowWriter_AddVideoTimestamp(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	fw.AddVideoTimestamp(0, 1000)
	fw.AddVideoTimestamp(1, 2500)

	if len(fw.flow.Artifacts.VideoTimestamps) != 2 {
		t.Fatalf("VideoTimestamps length = %d, want 2", len(fw.flow.Artifacts.VideoTimestamps))
	}
	if fw.flow.Artifacts.VideoTimestamps[0].CommandIndex != 0 {
		t.Errorf("VideoTimestamps[0].CommandIndex = %d, want 0", fw.flow.Artifacts.VideoTimestamps[0].CommandIndex)
	}
	if fw.flow.Artifacts.VideoTimestamps[0].VideoTimeMs != 1000 {
		t.Errorf("VideoTimestamps[0].VideoTimeMs = %d, want 1000", fw.flow.Artifacts.VideoTimestamps[0].VideoTimeMs)
	}
}

func TestFlowWriter_SaveScreenshot(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	// Fake PNG data
	data := []byte{0x89, 0x50, 0x4E, 0x47}

	path, err := fw.SaveScreenshot(0, "before", data)
	if err != nil {
		t.Fatalf("SaveScreenshot() error = %v", err)
	}

	expected := filepath.Join("assets", "flow-000", "cmd-000-before.png")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}

	// Check file exists
	absPath := filepath.Join(fw.assetsDir, "cmd-000-before.png")
	if _, err := os.Stat(absPath); err != nil {
		t.Errorf("screenshot file not created: %v", err)
	}
}

func TestFlowWriter_SaveViewHierarchy(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	data := []byte("<hierarchy><node /></hierarchy>")

	path, err := fw.SaveViewHierarchy(0, data)
	if err != nil {
		t.Fatalf("SaveViewHierarchy() error = %v", err)
	}

	expected := filepath.Join("assets", "flow-000", "cmd-000-hierarchy.xml")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}

	// Check file exists
	absPath := filepath.Join(fw.assetsDir, "cmd-000-hierarchy.xml")
	if _, err := os.Stat(absPath); err != nil {
		t.Errorf("hierarchy file not created: %v", err)
	}
}

func TestFlowWriter_SaveDeviceLog(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	data := []byte("log line 1\nlog line 2\n")

	path, err := fw.SaveDeviceLog(data)
	if err != nil {
		t.Fatalf("SaveDeviceLog() error = %v", err)
	}

	expected := filepath.Join("assets", "flow-000", "device.log")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}

	// Check file exists
	absPath := filepath.Join(fw.assetsDir, "device.log")
	if _, err := os.Stat(absPath); err != nil {
		t.Errorf("log file not created: %v", err)
	}
}

func TestFlowWriter_GetFlowDetail(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	detail := fw.GetFlowDetail()
	if detail.ID != "flow-000" {
		t.Errorf("ID = %q, want %q", detail.ID, "flow-000")
	}
}

func TestFlowWriter_SkipRemainingCommands(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	fw.Start()
	fw.CommandStart(0)
	fw.CommandEnd(0, StatusFailed, nil, &Error{Message: "failed"}, CommandArtifacts{})

	fw.SkipRemainingCommands(1)

	if fw.flow.Commands[1].Status != StatusSkipped {
		t.Errorf("Commands[1].Status = %q, want %q", fw.flow.Commands[1].Status, StatusSkipped)
	}
	if fw.flow.Commands[2].Status != StatusSkipped {
		t.Errorf("Commands[2].Status = %q, want %q", fw.flow.Commands[2].Status, StatusSkipped)
	}
}

func TestFlowWriter_commandSummary(t *testing.T) {
	fw, iw, _ := createTestFlowWriter(t)
	defer iw.Close()

	fw.flow.Commands[0].Status = StatusPassed
	fw.flow.Commands[1].Status = StatusRunning
	fw.flow.Commands[2].Status = StatusPending

	summary := fw.commandSummary()

	if summary.Total != 3 {
		t.Errorf("Total = %d, want 3", summary.Total)
	}
	if summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1", summary.Passed)
	}
	if summary.Running != 1 {
		t.Errorf("Running = %d, want 1", summary.Running)
	}
	if summary.Pending != 1 {
		t.Errorf("Pending = %d, want 1", summary.Pending)
	}
	if summary.Current == nil || *summary.Current != 1 {
		t.Errorf("Current = %v, want 1", summary.Current)
	}
}
