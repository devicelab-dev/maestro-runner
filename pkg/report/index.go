package report

import (
	"path/filepath"
	"sync"
	"time"
)

// IndexWriter provides thread-safe updates to the report index.
// Multiple flow goroutines can update the index concurrently.
type IndexWriter struct {
	mu        sync.Mutex
	outputDir string
	path      string
	index     *Index

	// Debouncing for progress updates
	pending   map[string]*FlowUpdate
	timer     *time.Timer
	immediate chan struct{}
	done      chan struct{}
}

// NewIndexWriter creates a new IndexWriter.
func NewIndexWriter(outputDir string, index *Index) *IndexWriter {
	w := &IndexWriter{
		outputDir: outputDir,
		path:      filepath.Join(outputDir, "report.json"),
		index:     index,
		pending:   make(map[string]*FlowUpdate),
		immediate: make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
	go w.flushLoop()
	return w
}

// Start marks the run as started.
func (w *IndexWriter) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	w.index.Status = StatusRunning
	w.index.StartTime = now
	w.index.LastUpdated = now
	w.index.UpdateSeq++

	w.flushLocked()
}

// UpdateFlow updates a flow entry in the index.
// Terminal states (passed/failed) flush immediately.
// Progress updates are debounced to reduce I/O.
func (w *IndexWriter) UpdateFlow(flowID string, update *FlowUpdate) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending[flowID] = update

	// Immediate flush for terminal states
	if update.Status.IsTerminal() {
		w.flushLocked()
		return
	}

	// Debounced flush for progress updates (100ms)
	if w.timer == nil {
		w.timer = time.AfterFunc(100*time.Millisecond, func() {
			w.flush()
		})
	}
}

// End marks the run as complete.
func (w *IndexWriter) End() {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	w.index.EndTime = &now
	w.index.LastUpdated = now
	w.index.Status = w.computeRunStatus()
	w.index.UpdateSeq++

	w.flushLocked()
}

// Close shuts down the IndexWriter and flushes any pending updates.
func (w *IndexWriter) Close() {
	close(w.done)
	w.flush()
}

// GetIndex returns the current index (for reading).
func (w *IndexWriter) GetIndex() *Index {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.index
}

// flushLoop handles immediate flush requests.
func (w *IndexWriter) flushLoop() {
	for {
		select {
		case <-w.immediate:
			w.flush()
		case <-w.done:
			return
		}
	}
}

// flush applies pending updates and writes to disk.
func (w *IndexWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushLocked()
}

// flushLocked flushes while holding the lock.
func (w *IndexWriter) flushLocked() {
	// Apply pending updates
	for flowID, update := range w.pending {
		w.applyUpdate(flowID, update)
	}
	w.pending = make(map[string]*FlowUpdate)

	// Update metadata
	w.index.UpdateSeq++
	w.index.LastUpdated = time.Now()
	w.index.Summary = w.computeSummary()

	// Stop debounce timer if running
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}

	// Atomic write JSON
	atomicWriteJSON(w.path, w.index)

	// Regenerate HTML for live file:// viewing
	GenerateHTML(w.outputDir, HTMLConfig{
		Title:     "Test Report",
		ReportDir: w.outputDir,
	})
}

// applyUpdate applies a FlowUpdate to the index.
func (w *IndexWriter) applyUpdate(flowID string, update *FlowUpdate) {
	for i := range w.index.Flows {
		if w.index.Flows[i].ID == flowID {
			f := &w.index.Flows[i]
			f.Status = update.Status
			if update.StartTime != nil {
				f.StartTime = update.StartTime
			}
			if update.EndTime != nil {
				f.EndTime = update.EndTime
			}
			if update.Duration != nil {
				f.Duration = update.Duration
			}
			f.Commands = update.Commands
			if update.Error != nil {
				f.Error = update.Error
			}
			f.UpdateSeq++
			now := time.Now()
			f.LastUpdated = &now
			break
		}
	}
}

// computeSummary calculates summary from flow statuses.
func (w *IndexWriter) computeSummary() Summary {
	var s Summary
	for _, f := range w.index.Flows {
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
	return s
}

// computeRunStatus determines overall run status from flows.
func (w *IndexWriter) computeRunStatus() Status {
	hasFailure := false
	allComplete := true

	for _, f := range w.index.Flows {
		if f.Status == StatusFailed {
			hasFailure = true
		}
		if !f.Status.IsTerminal() {
			allComplete = false
		}
	}

	if !allComplete {
		return StatusRunning
	}
	if hasFailure {
		return StatusFailed
	}
	return StatusPassed
}

// RecordAttempt records a retry attempt for a flow.
func (w *IndexWriter) RecordAttempt(flowID string, attempt int, status Status, duration int64, errMsg string, dataFile string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i := range w.index.Flows {
		if w.index.Flows[i].ID == flowID {
			f := &w.index.Flows[i]
			f.Attempts = attempt
			f.AttemptHistory = append(f.AttemptHistory, AttemptEntry{
				Attempt:  attempt,
				DataFile: dataFile,
				Status:   status,
				Duration: duration,
				Error:    errMsg,
			})
			break
		}
	}

	w.flushLocked()
}
