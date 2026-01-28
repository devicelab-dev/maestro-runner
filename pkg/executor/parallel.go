package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// DeviceWorker represents a single device worker that pulls from the queue.
type DeviceWorker struct {
	ID       int
	DeviceID string
	Driver   core.Driver
	Cleanup  func()
}

// workItem represents a flow and its index in the original flow list.
type workItem struct {
	flow  flow.Flow
	index int
}

// ParallelRunner coordinates parallel test execution across multiple devices.
type ParallelRunner struct {
	workers []DeviceWorker
	config  RunnerConfig
}

// NewParallelRunner creates a parallel runner with multiple device workers.
func NewParallelRunner(workers []DeviceWorker, config RunnerConfig) *ParallelRunner {
	return &ParallelRunner{
		workers: workers,
		config:  config,
	}
}

// Run executes flows in parallel using a work queue pattern.
// All workers pull from the same queue until all flows are complete.
func (pr *ParallelRunner) Run(ctx context.Context, flows []flow.Flow) (*RunResult, error) {
	if len(pr.workers) == 0 {
		return nil, fmt.Errorf("no workers available")
	}

	// Build shared report skeleton
	builderCfg := report.BuilderConfig{
		OutputDir:     pr.config.OutputDir,
		Device:        pr.config.Device,
		App:           pr.config.App,
		CI:            pr.config.CI,
		RunnerVersion: pr.config.RunnerVersion,
		DriverName:    pr.config.DriverName,
	}

	index, flowDetails, err := report.BuildSkeleton(flows, builderCfg)
	if err != nil {
		return nil, err
	}

	// Write initial skeleton to disk
	if err := report.WriteSkeleton(pr.config.OutputDir, index, flowDetails); err != nil {
		return nil, err
	}

	// Create index writer for coordinated updates
	indexWriter := report.NewIndexWriter(pr.config.OutputDir, index)
	defer indexWriter.Close()

	// Mark run as started and track wall clock time
	indexWriter.Start()
	startTime := time.Now()

	// Create work queue with flow indices
	workQueue := make(chan workItem, len(flows))
	for i, f := range flows {
		workQueue <- workItem{flow: f, index: i}
	}
	close(workQueue)

	// Results collection
	results := make([]FlowResult, len(flows))
	var resultsMu sync.Mutex
	var wg sync.WaitGroup

	totalFlows := len(flows)

	// Start workers
	for i := range pr.workers {
		wg.Add(1)
		worker := pr.workers[i]

		go func(w DeviceWorker) {
			defer wg.Done()
			defer w.Cleanup()

			// Create runner for this worker
			// Each worker uses its own driver but shares the report
			runner := &Runner{
				config: pr.config,
				driver: w.Driver,
			}

			// Process flows from queue
			for item := range workQueue {
				// Execute flow
				result := runner.executeFlow(ctx, item.flow, &flowDetails[item.index], indexWriter, item.index, totalFlows)

				// Store result
				resultsMu.Lock()
				results[item.index] = result
				resultsMu.Unlock()
			}
		}(worker)
	}

	// Wait for all workers to complete
	wg.Wait()

	// Calculate actual wall clock time
	wallClockDuration := time.Since(startTime).Milliseconds()

	// Mark run as complete
	indexWriter.End()

	// Build result using the same logic as single-device runner
	return pr.buildRunResult(results, wallClockDuration), nil
}

// buildRunResult aggregates flow results into a run result.
// For parallel execution, use wall clock duration instead of sum of flow durations.
func (pr *ParallelRunner) buildRunResult(flowResults []FlowResult, wallClockDuration int64) *RunResult {
	result := &RunResult{
		TotalFlows:  len(flowResults),
		FlowResults: flowResults,
		Duration:    wallClockDuration, // Use actual wall clock time for parallel execution
	}

	for _, fr := range flowResults {
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
