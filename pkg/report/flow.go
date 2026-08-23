package report

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// FlowWriter writes updates for a single flow.
// Each flow goroutine has its own FlowWriter - no locking needed.
type FlowWriter struct {
	flow      *FlowDetail
	path      string
	assetsDir string
	index     *IndexWriter
}

// NewFlowWriter creates a new FlowWriter for a flow.
func NewFlowWriter(flowDetail *FlowDetail, outputDir string, index *IndexWriter) *FlowWriter {
	flowPath := filepath.Join(outputDir, "flows", flowDetail.ID+".json")
	assetsDir := filepath.Join(outputDir, "assets", flowDetail.ID)

	// Ensure assets directory exists
	if err := ensureDir(assetsDir); err != nil {
		logger.Warn("failed to create assets directory %s: %v", assetsDir, err)
	}

	return &FlowWriter{
		flow:      flowDetail,
		path:      flowPath,
		assetsDir: assetsDir,
		index:     index,
	}
}

// Start marks the flow as started.
func (w *FlowWriter) Start() {
	now := time.Now()
	w.flow.StartTime = now

	w.flush()
	w.updateIndex(StatusRunning, &now, nil, nil)
}

// CommandStart marks a command as started.
func (w *FlowWriter) CommandStart(cmdIndex int) {
	if cmdIndex < 0 || cmdIndex >= len(w.flow.Commands) {
		return
	}

	now := time.Now()
	cmd := &w.flow.Commands[cmdIndex]
	cmd.Status = StatusRunning
	cmd.StartTime = &now

	w.flush()
	w.updateIndexProgress()
}

// CommandEnd marks a command as complete.
func (w *FlowWriter) CommandEnd(cmdIndex int, status Status, element *Element, err *Error, artifacts CommandArtifacts) {
	w.CommandEndWithSubs(cmdIndex, status, element, err, artifacts, nil)
}

// CommandEndWithSubs marks a command as complete with optional sub-commands.
func (w *FlowWriter) CommandEndWithSubs(cmdIndex int, status Status, element *Element, err *Error, artifacts CommandArtifacts, subCommands []Command) {
	if cmdIndex < 0 || cmdIndex >= len(w.flow.Commands) {
		return
	}

	now := time.Now()
	cmd := &w.flow.Commands[cmdIndex]
	cmd.Status = status
	cmd.EndTime = &now

	if cmd.StartTime != nil {
		duration := now.Sub(*cmd.StartTime).Milliseconds()
		cmd.Duration = &duration
	}

	cmd.Element = element
	cmd.Error = err
	cmd.Artifacts = artifacts
	cmd.SubCommands = subCommands

	w.flush()
	w.updateIndexProgress()
}

// End marks the flow as complete. flowLevelError, when non-empty, is used
// as the failure message when no individual command recorded one — covers
// flow-level failures like failOnConsoleError.
func (w *FlowWriter) End(status Status, flowLevelError ...string) {
	now := time.Now()
	w.flow.EndTime = &now

	var duration int64
	if !w.flow.StartTime.IsZero() {
		duration = now.Sub(w.flow.StartTime).Milliseconds()
		w.flow.Duration = &duration
	}

	// Summarise per-step latency now that every command has its duration, so
	// the shape of a slow run is visible in the report rather than only its
	// total. Omitted when nothing ran (a fully skipped flow has no data).
	if latency := ComputeStepLatency(w.flow.Commands); latency.Count > 0 {
		w.flow.StepLatency = &latency
	}

	w.flush()

	var errMsg *string
	if status == StatusFailed {
		// Find first error
		for _, cmd := range w.flow.Commands {
			if cmd.Error != nil {
				errMsg = &cmd.Error.Message
				break
			}
		}
		// Fall back to flow-level error (no command failed but the flow did
		// — e.g. failOnConsoleError).
		if errMsg == nil && len(flowLevelError) > 0 && flowLevelError[0] != "" {
			fle := flowLevelError[0]
			errMsg = &fle
		}
	}

	w.updateIndex(status, nil, &now, &duration)
	if errMsg != nil {
		w.index.UpdateFlow(w.flow.ID, &FlowUpdate{
			Status:   status,
			EndTime:  &now,
			Duration: &duration,
			Commands: w.commandSummary(),
			Error:    errMsg,
			Device:   w.flow.Device, // Include actual device
		})
	}
}

// SetFlowArtifacts sets flow-level artifacts (video, logs).
func (w *FlowWriter) SetFlowArtifacts(artifacts FlowArtifacts) {
	w.flow.Artifacts = artifacts
	w.flush()
}

// SetConsoleLogs records browser console / page error entries captured during
// the flow. Web flows only — mobile / native drivers leave this nil.
func (w *FlowWriter) SetConsoleLogs(logs []ConsoleLog) {
	w.flow.ConsoleLogs = logs
	w.flush()
}

// AddVideoTimestamp adds a video timestamp mapping.
func (w *FlowWriter) AddVideoTimestamp(cmdIndex int, videoTimeMs int64) {
	w.flow.Artifacts.VideoTimestamps = append(w.flow.Artifacts.VideoTimestamps, VideoTimestamp{
		CommandIndex: cmdIndex,
		VideoTimeMs:  videoTimeMs,
	})
	w.flush()
}

// SaveScreenshot saves a screenshot and returns the relative path.
func (w *FlowWriter) SaveScreenshot(cmdIndex int, timing string, data []byte) (string, error) {
	filename := fmt.Sprintf("cmd-%03d-%s.png", cmdIndex, timing)
	absPath := filepath.Join(w.assetsDir, filename)

	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return "", err
	}

	// Return relative path for JSON
	return filepath.Join("assets", w.flow.ID, filename), nil
}

// SaveNamedScreenshot saves a screenshot with a user-specified name, prefixed with cmd index.
// If name is empty, defaults to "screenshot.png".
func (w *FlowWriter) SaveNamedScreenshot(cmdIndex int, name string, data []byte) (string, error) {
	if name == "" {
		name = "screenshot.png"
	}
	filename := fmt.Sprintf("cmd-%03d-%s", cmdIndex, name)
	// Ensure .png extension
	if filepath.Ext(filename) == "" {
		filename += ".png"
	}
	absPath := filepath.Join(w.assetsDir, filename)

	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return "", err
	}

	return filepath.Join("assets", w.flow.ID, filename), nil
}

// SaveViewHierarchy saves view hierarchy and returns the relative path.
func (w *FlowWriter) SaveViewHierarchy(cmdIndex int, data []byte) (string, error) {
	filename := fmt.Sprintf("cmd-%03d-hierarchy.xml", cmdIndex)
	absPath := filepath.Join(w.assetsDir, filename)

	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return "", err
	}

	return filepath.Join("assets", w.flow.ID, filename), nil
}

// RecordingTarget returns the absolute path a flow screen recording should be
// written to. The file does not exist until a driver writes it.
func (w *FlowWriter) RecordingTarget() string {
	return filepath.Join(w.assetsDir, "recording.mp4")
}

// SetVideo records that a screen recording exists at RecordingTarget,
// storing the report-relative path alongside the other flow artifacts.
func (w *FlowWriter) SetVideo() {
	w.flow.Artifacts.Video = filepath.Join("assets", w.flow.ID, "recording.mp4")
	w.flush()
}

// SaveDeviceLog saves device log and returns the relative path.
func (w *FlowWriter) SaveDeviceLog(data []byte) (string, error) {
	filename := "device.log"
	absPath := filepath.Join(w.assetsDir, filename)

	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return "", err
	}

	return filepath.Join("assets", w.flow.ID, filename), nil
}

// GetFlowDetail returns the current flow detail (for reading).
func (w *FlowWriter) GetFlowDetail() *FlowDetail {
	return w.flow
}

// flush writes the flow detail to disk.
func (w *FlowWriter) flush() {
	if err := atomicWriteJSON(w.path, w.flow); err != nil {
		logger.Warn("failed to write flow detail to %s: %v", w.path, err)
	}
}

// updateIndex updates the index with current flow state.
func (w *FlowWriter) updateIndex(status Status, startTime, endTime *time.Time, duration *int64) {
	w.index.UpdateFlow(w.flow.ID, &FlowUpdate{
		Status:    status,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  duration,
		Commands:  w.commandSummary(),
		Device:    w.flow.Device, // Include actual device that ran this flow
	})
}

// updateIndexProgress updates the index with progress only.
func (w *FlowWriter) updateIndexProgress() {
	w.index.UpdateFlow(w.flow.ID, &FlowUpdate{
		Status:   StatusRunning,
		Commands: w.commandSummary(),
	})
}

// commandSummary computes command summary.
func (w *FlowWriter) commandSummary() CommandSummary {
	var s CommandSummary
	s.Total = len(w.flow.Commands)

	for i, cmd := range w.flow.Commands {
		switch cmd.Status {
		case StatusPassed:
			s.Passed++
		case StatusFailed:
			s.Failed++
		case StatusSkipped:
			s.Skipped++
		case StatusRunning:
			s.Running++
			idx := i
			s.Current = &idx
		case StatusPending:
			s.Pending++
		}
	}

	return s
}

// SkipRemainingCommands marks all pending commands as skipped.
// Called when a command fails and we need to skip the rest.
func (w *FlowWriter) SkipRemainingCommands(fromIndex int) {
	for i := fromIndex; i < len(w.flow.Commands); i++ {
		if w.flow.Commands[i].Status == StatusPending {
			w.flow.Commands[i].Status = StatusSkipped
		}
	}
	w.flush()
}
