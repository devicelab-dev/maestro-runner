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
	workers     []DeviceWorker
	config      RunnerConfig
	outputMutex sync.Mutex
}

// Terminal color codes for parallel output
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorGray   = "\033[90m"
	colorCyan   = "\033[36m"
)

func color(c string) string {
	return c
}

// formatDeviceLabel creates a short device label for event logs
func formatDeviceLabel(device *report.Device) string {
	if device == nil {
		return "Unknown"
	}
	// For event logs, just show device name
	return device.Name
}

// formatDuration formats milliseconds as human-readable duration
func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	seconds := float64(ms) / 1000.0
	if seconds < 60 {
		return fmt.Sprintf("%.1fs", seconds)
	}
	minutes := int(seconds / 60)
	secs := int(seconds) % 60
	return fmt.Sprintf("%dm%ds", minutes, secs)
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

			// Capture device info for this worker
			platformInfo := w.Driver.GetPlatformInfo()
			deviceInfo := &report.Device{
				ID:          platformInfo.DeviceID,
				Name:        platformInfo.DeviceName,
				Platform:    platformInfo.Platform,
				OSVersion:   platformInfo.OSVersion,
				IsSimulator: platformInfo.IsSimulator,
			}

			// Create device-specific config with device info set
			workerConfig := pr.config
			workerConfig.DeviceInfo = deviceInfo

			// Create device-specific callbacks that include device info in output
			deviceLabel := formatDeviceLabel(deviceInfo)

			// Store flow info for OnFlowEnd callback
			var currentFlowIdx int
			var currentTotalFlows int
			var currentFlowFile string

			workerConfig.OnFlowStart = func(flowIdx, totalFlows int, name, file string) {
				// Store for OnFlowEnd
				currentFlowIdx = flowIdx
				currentTotalFlows = totalFlows
				currentFlowFile = file

				pr.outputMutex.Lock()
				defer pr.outputMutex.Unlock()
				fmt.Printf("[%d/%d] %s (%s) - %s⚡ Started%s on %s\n",
					flowIdx+1, totalFlows, name, file, color(colorCyan), color(colorReset), deviceLabel)
			}

			workerConfig.OnFlowEnd = func(name string, passed bool, durationMs int64, errMsg string) {
				pr.outputMutex.Lock()
				defer pr.outputMutex.Unlock()

				status := "✓ Passed"
				statusColor := color(colorGreen)
				if !passed {
					status = "✗ Failed"
					statusColor = color(colorRed)
				}

				fmt.Printf("[%d/%d] %s (%s) - %s%s%s on %s (%s)\n",
					currentFlowIdx+1, currentTotalFlows, name, currentFlowFile,
					statusColor, status, color(colorReset), deviceLabel, formatDuration(durationMs))

				if !passed && errMsg != "" {
					fmt.Printf("  Error: %s\n", errMsg)
				}
			}

			// Suppress detailed command output during parallel execution
			workerConfig.OnStepComplete = func(idx int, desc string, passed bool, durationMs int64, errMsg string) {}
			workerConfig.OnNestedStep = func(depth int, desc string, passed bool, durationMs int64, errMsg string) {}
			workerConfig.OnNestedFlowStart = func(depth int, desc string) {}

			// Create runner for this worker with device-specific config
			runner := &Runner{
				config: workerConfig,
				driver: w.Driver,
			}

			// Process flows from queue
			for item := range workQueue {
				// Update flow detail with actual device
				flowDetails[item.index].Device = deviceInfo

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

	// Cleanup all workers after tests complete
	// This ensures cleanup happens synchronously after all work is done
	for i := range pr.workers {
		pr.workers[i].Cleanup()
	}
	// Give cleanup a moment to complete (socket/port release)
	time.Sleep(100 * time.Millisecond)

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
