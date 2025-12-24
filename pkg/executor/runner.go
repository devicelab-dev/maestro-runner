// Package executor orchestrates flow execution, connecting drivers to reports.
package executor

import (
	"context"
	"sync"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// ArtifactMode determines when to capture screenshots/hierarchy.
type ArtifactMode int

const (
	// ArtifactOnFailure captures artifacts only when a step fails.
	ArtifactOnFailure ArtifactMode = iota
	// ArtifactAlways captures artifacts before and after every step.
	ArtifactAlways
	// ArtifactNever disables artifact capture.
	ArtifactNever
)

// RunnerConfig configures the test runner.
type RunnerConfig struct {
	OutputDir   string       // Report output directory
	Parallelism int          // Max concurrent flows (0 = sequential)
	StopOnFail  bool         // Stop all flows on first failure
	Retries     int          // Max retries per flow (0 = no retries)
	Artifacts   ArtifactMode // When to capture artifacts

	// Device/App info for reports
	Device report.Device
	App    report.App
	CI     *report.CI

	// Runner metadata
	RunnerVersion string
	DriverName    string

	// Live progress callbacks
	OnFlowStart       func(flowIdx, totalFlows int, name, file string)
	OnStepComplete    func(idx int, desc string, passed bool, durationMs int64, err string)
	OnNestedStep      func(depth int, desc string, passed bool, durationMs int64, err string)
	OnNestedFlowStart func(depth int, desc string)
	OnFlowEnd         func(name string, passed bool, durationMs int64)
}

// RunResult contains the outcome of a test run.
type RunResult struct {
	Status       report.Status
	TotalFlows   int
	PassedFlows  int
	FailedFlows  int
	SkippedFlows int
	Duration     int64 // Total duration in milliseconds
	FlowResults  []FlowResult
}

// FlowResult contains the outcome of a single flow execution.
type FlowResult struct {
	ID           string
	Name         string
	Status       report.Status
	Duration     int64
	Error        string
	StepsTotal   int
	StepsPassed  int
	StepsFailed  int
	StepsSkipped int
}

// Runner orchestrates flow execution.
type Runner struct {
	config RunnerConfig
	driver core.Driver
}

// New creates a new Runner.
func New(driver core.Driver, cfg RunnerConfig) *Runner {
	return &Runner{
		config: cfg,
		driver: driver,
	}
}

// Run executes all flows and generates reports.
func (r *Runner) Run(ctx context.Context, flows []flow.Flow) (*RunResult, error) {
	// Build report skeleton
	builderCfg := report.BuilderConfig{
		OutputDir:     r.config.OutputDir,
		Device:        r.config.Device,
		App:           r.config.App,
		CI:            r.config.CI,
		RunnerVersion: r.config.RunnerVersion,
		DriverName:    r.config.DriverName,
	}

	index, flowDetails, err := report.BuildSkeleton(flows, builderCfg)
	if err != nil {
		return nil, err
	}

	// Write initial skeleton to disk
	if err := report.WriteSkeleton(r.config.OutputDir, index, flowDetails); err != nil {
		return nil, err
	}

	// Create index writer for coordinated updates
	indexWriter := report.NewIndexWriter(r.config.OutputDir, index)
	defer indexWriter.Close()

	// Mark run as started
	indexWriter.Start()

	// Execute flows
	results := r.executeFlows(ctx, flows, flowDetails, indexWriter)

	// Mark run as complete
	indexWriter.End()

	// Build result
	return r.buildRunResult(results), nil
}

// executeFlows runs flows either sequentially or in parallel.
func (r *Runner) executeFlows(ctx context.Context, flows []flow.Flow, flowDetails []report.FlowDetail, indexWriter *report.IndexWriter) []FlowResult {
	results := make([]FlowResult, len(flows))

	totalFlows := len(flows)
	if r.config.Parallelism <= 0 {
		// Sequential execution
		for i := range flows {
			if ctx.Err() != nil {
				// Context cancelled, skip remaining
				results[i] = FlowResult{
					ID:     flowDetails[i].ID,
					Name:   flowDetails[i].Name,
					Status: report.StatusSkipped,
					Error:  "run cancelled",
				}
				continue
			}
			results[i] = r.executeFlow(ctx, flows[i], &flowDetails[i], indexWriter, i, totalFlows)
		}
	} else {
		// Parallel execution with semaphore
		sem := make(chan struct{}, r.config.Parallelism)
		var wg sync.WaitGroup
		var mu sync.Mutex
		stopAll := false

		for i := range flows {
			// Check if we should stop
			mu.Lock()
			shouldStop := stopAll
			mu.Unlock()
			if shouldStop || ctx.Err() != nil {
				results[i] = FlowResult{
					ID:     flowDetails[i].ID,
					Name:   flowDetails[i].Name,
					Status: report.StatusSkipped,
					Error:  "run stopped",
				}
				continue
			}

			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				sem <- struct{}{}        // Acquire
				defer func() { <-sem }() // Release

				result := r.executeFlow(ctx, flows[idx], &flowDetails[idx], indexWriter, idx, totalFlows)
				results[idx] = result

				// Check if we should stop all
				if r.config.StopOnFail && result.Status == report.StatusFailed {
					mu.Lock()
					stopAll = true
					mu.Unlock()
				}
			}(i)
		}
		wg.Wait()
	}

	return results
}

// executeFlow runs a single flow.
func (r *Runner) executeFlow(ctx context.Context, f flow.Flow, detail *report.FlowDetail, indexWriter *report.IndexWriter, flowIdx, totalFlows int) FlowResult {
	fr := &FlowRunner{
		ctx:         ctx,
		flow:        f,
		detail:      detail,
		driver:      r.driver,
		config:      r.config,
		indexWriter: indexWriter,
		flowIdx:     flowIdx,
		totalFlows:  totalFlows,
	}
	return fr.Run()
}

// buildRunResult aggregates flow results into a run result.
func (r *Runner) buildRunResult(flowResults []FlowResult) *RunResult {
	result := &RunResult{
		TotalFlows:  len(flowResults),
		FlowResults: flowResults,
	}

	for _, fr := range flowResults {
		result.Duration += fr.Duration
		switch fr.Status {
		case report.StatusPassed:
			result.PassedFlows++
		case report.StatusFailed:
			result.FailedFlows++
		case report.StatusSkipped:
			result.SkippedFlows++
		}
	}

	// Determine overall status
	if result.FailedFlows > 0 {
		result.Status = report.StatusFailed
	} else if result.PassedFlows == result.TotalFlows {
		result.Status = report.StatusPassed
	} else {
		result.Status = report.StatusPassed // All passed or skipped
	}

	return result
}
