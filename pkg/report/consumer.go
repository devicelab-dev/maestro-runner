package report

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Consumer reads report files and tracks changes.
// Used by HTML generators and other report consumers.
type Consumer struct {
	reportDir     string
	lastGlobalSeq uint64
	lastFlowSeq   map[string]uint64
}

// NewConsumer creates a new report consumer.
func NewConsumer(reportDir string) *Consumer {
	return &Consumer{
		reportDir:   reportDir,
		lastFlowSeq: make(map[string]uint64),
	}
}

// Poll checks for changes and returns changed flow IDs.
// Returns nil if no changes detected.
func (c *Consumer) Poll() (changed []string, index *Index, err error) {
	index, err = c.ReadIndex()
	if err != nil {
		return nil, nil, err
	}

	// No global changes
	if index.UpdateSeq <= c.lastGlobalSeq {
		return nil, index, nil
	}
	c.lastGlobalSeq = index.UpdateSeq

	// Find changed flows
	for _, f := range index.Flows {
		if f.UpdateSeq > c.lastFlowSeq[f.ID] {
			changed = append(changed, f.ID)
			c.lastFlowSeq[f.ID] = f.UpdateSeq
		}
	}

	return changed, index, nil
}

// ReadIndex reads the main index file.
func (c *Consumer) ReadIndex() (*Index, error) {
	path := filepath.Join(c.reportDir, "report.json")
	return ReadIndex(path)
}

// ReadFlow reads a flow detail file.
func (c *Consumer) ReadFlow(flowID string) (*FlowDetail, error) {
	path := filepath.Join(c.reportDir, "flows", flowID+".json")
	return ReadFlowDetail(path)
}

// Reset resets the consumer's change tracking state.
func (c *Consumer) Reset() {
	c.lastGlobalSeq = 0
	c.lastFlowSeq = make(map[string]uint64)
}

// ============================================================================
// STANDALONE READ FUNCTIONS
// ============================================================================

// ReadIndex reads an index file from the given path.
func ReadIndex(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}

	return &index, nil
}

// ReadFlowDetail reads a flow detail file from the given path.
func ReadFlowDetail(path string) (*FlowDetail, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var flow FlowDetail
	if err := json.Unmarshal(data, &flow); err != nil {
		return nil, err
	}

	return &flow, nil
}

// ReadReport reads the complete report (index + all flows).
func ReadReport(reportDir string) (*Index, []FlowDetail, error) {
	index, err := ReadIndex(filepath.Join(reportDir, "report.json"))
	if err != nil {
		return nil, nil, err
	}

	flows := make([]FlowDetail, len(index.Flows))
	for i, entry := range index.Flows {
		flowPath := filepath.Join(reportDir, entry.DataFile)
		flow, err := ReadFlowDetail(flowPath)
		if err != nil {
			return nil, nil, err
		}
		flows[i] = *flow
	}

	return index, flows, nil
}

// ============================================================================
// RECOVERY
// ============================================================================

// Recover recovers from incomplete state (e.g., after crash).
// It checks for flows that were left in "running" state and marks them appropriately.
func Recover(reportDir string) error {
	indexPath := filepath.Join(reportDir, "report.json")
	index, err := ReadIndex(indexPath)
	if err != nil {
		return err
	}

	changed := false
	for i := range index.Flows {
		f := &index.Flows[i]
		if f.Status == StatusRunning {
			// Check flow file for actual state
			flowPath := filepath.Join(reportDir, f.DataFile)
			flow, err := ReadFlowDetail(flowPath)
			if err != nil {
				// Flow file missing or corrupt - mark as failed
				f.Status = StatusFailed
				errMsg := "Flow interrupted"
				f.Error = &errMsg
				changed = true
				continue
			}

			// Infer status from commands
			status := inferStatus(flow.Commands)
			if status != StatusRunning {
				f.Status = status
				changed = true
			} else {
				// Still running = interrupted
				f.Status = StatusFailed
				errMsg := "Flow interrupted"
				f.Error = &errMsg
				changed = true
			}
		}
	}

	if changed {
		// Recompute summary
		var s Summary
		for _, f := range index.Flows {
			s.Total++
			switch f.Status {
			case StatusPassed:
				s.Passed++
			case StatusFailed:
				s.Failed++
			case StatusSkipped:
				s.Skipped++
			case StatusRunning:
				s.Running++
			case StatusPending:
				s.Pending++
			}
		}
		index.Summary = s

		// Update run status
		if s.Failed > 0 {
			index.Status = StatusFailed
		} else if s.Running > 0 || s.Pending > 0 {
			index.Status = StatusRunning
		} else {
			index.Status = StatusPassed
		}

		index.UpdateSeq++
		return atomicWriteJSON(indexPath, index)
	}

	return nil
}

// inferStatus infers flow status from command statuses.
func inferStatus(commands []Command) Status {
	if len(commands) == 0 {
		return StatusFailed
	}

	allPassed := true
	for _, c := range commands {
		if c.Status == StatusFailed {
			return StatusFailed
		}
		if c.Status != StatusPassed {
			allPassed = false
		}
	}

	if allPassed {
		return StatusPassed
	}

	// Has non-passed, non-failed commands = incomplete
	return StatusRunning
}
