package executor

import (
	"context"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// FlowRunner executes a single flow.
type FlowRunner struct {
	ctx         context.Context
	flow        flow.Flow
	detail      *report.FlowDetail
	driver      core.Driver
	config      RunnerConfig
	indexWriter *report.IndexWriter
	flowWriter  *report.FlowWriter
}

// Run executes the flow and returns the result.
func (fr *FlowRunner) Run() FlowResult {
	// Create flow writer for this flow's updates
	fr.flowWriter = report.NewFlowWriter(fr.detail, fr.config.OutputDir, fr.indexWriter)

	// Mark flow as started
	fr.flowWriter.Start()

	// Execute all steps
	flowStatus := report.StatusPassed
	var flowError string

	for i, step := range fr.flow.Steps {
		// Check context cancellation
		if fr.ctx.Err() != nil {
			fr.flowWriter.SkipRemainingCommands(i)
			flowStatus = report.StatusSkipped
			flowError = "execution cancelled"
			break
		}

		// Execute step
		stepStatus, stepError := fr.executeStep(i, step)

		// Handle step result
		if stepStatus == report.StatusFailed {
			if step.IsOptional() {
				// Optional step failure doesn't fail flow
				continue
			}
			// Required step failed - skip remaining and fail flow
			fr.flowWriter.SkipRemainingCommands(i + 1)
			flowStatus = report.StatusFailed
			flowError = stepError
			break
		}
	}

	// Mark flow as complete
	fr.flowWriter.End(flowStatus)

	// Get duration from flow detail
	var duration int64
	if fr.detail.Duration != nil {
		duration = *fr.detail.Duration
	}

	return FlowResult{
		ID:       fr.detail.ID,
		Name:     fr.detail.Name,
		Status:   flowStatus,
		Duration: duration,
		Error:    flowError,
	}
}

// executeStep executes a single step and updates the report.
func (fr *FlowRunner) executeStep(idx int, step flow.Step) (report.Status, string) {
	// Mark step as started
	fr.flowWriter.CommandStart(idx)

	// Determine what artifacts to capture
	captureAlways := fr.config.Artifacts == ArtifactAlways
	captureOnFailure := fr.config.Artifacts == ArtifactOnFailure

	// Capture before screenshot if configured
	var artifacts report.CommandArtifacts
	if captureAlways {
		artifacts = fr.captureArtifacts(idx, "before")
	}

	// Execute step via driver
	result := fr.driver.Execute(step)

	// Determine status and error
	var status report.Status
	var errorInfo *report.Error
	var errorMsg string

	if result.Success {
		status = report.StatusPassed
	} else {
		status = report.StatusFailed
		errorInfo = commandResultToError(result)
		if errorInfo != nil {
			errorMsg = errorInfo.Message
		}
	}

	// Capture after screenshot (on failure or always)
	shouldCaptureAfter := captureAlways || (captureOnFailure && !result.Success)
	if shouldCaptureAfter {
		afterArtifacts := fr.captureArtifacts(idx, "after")
		artifacts.ScreenshotAfter = afterArtifacts.ScreenshotAfter
		artifacts.ViewHierarchy = afterArtifacts.ViewHierarchy
	}

	// Convert element info
	var element *report.Element
	if result.Element != nil {
		element = commandResultToElement(result)
	}

	// Update report
	fr.flowWriter.CommandEnd(idx, status, element, errorInfo, artifacts)

	return status, errorMsg
}

// captureArtifacts captures screenshots and hierarchy.
func (fr *FlowRunner) captureArtifacts(cmdIdx int, timing string) report.CommandArtifacts {
	var artifacts report.CommandArtifacts

	// Capture screenshot
	if data, err := fr.driver.Screenshot(); err == nil && len(data) > 0 {
		path, saveErr := fr.flowWriter.SaveScreenshot(cmdIdx, timing, data)
		if saveErr == nil {
			if timing == "before" {
				artifacts.ScreenshotBefore = path
			} else {
				artifacts.ScreenshotAfter = path
			}
		}
	}

	// Capture hierarchy on failure
	if timing == "after" {
		if data, err := fr.driver.Hierarchy(); err == nil && len(data) > 0 {
			path, saveErr := fr.flowWriter.SaveViewHierarchy(cmdIdx, data)
			if saveErr == nil {
				artifacts.ViewHierarchy = path
			}
		}
	}

	return artifacts
}
