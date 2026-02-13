package executor

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
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
	script      *ScriptEngine
	depth       int // Nesting depth for runFlow reporting
	flowIdx     int // Current flow index (0-based)
	totalFlows  int // Total number of flows
	// Step counters
	stepsPassed  int
	stepsFailed  int
	stepsSkipped int
	// Sub-command tracking for compound steps (runFlow, repeat, retry)
	subCommands []report.Command
}

// Run executes the flow and returns the result.
func (fr *FlowRunner) Run() FlowResult {
	flowStart := time.Now()

	logger.Info("=== Starting flow: %s ===", fr.detail.Name)
	logger.Info("Flow file: %s", fr.flow.SourcePath)
	logger.Info("Total steps: %d", len(fr.flow.Steps))

	// Create flow writer for this flow's updates
	fr.flowWriter = report.NewFlowWriter(fr.detail, fr.config.OutputDir, fr.indexWriter)

	// Initialize script engine
	fr.script = NewScriptEngine()
	defer fr.script.Close()

	// Import system environment variables
	fr.script.ImportSystemEnv()

	// Apply CLI environment variables (from -e flags)
	// These take precedence over system env, but flow-level env takes precedence over these
	fr.script.SetVariables(fr.config.Env)

	// Set flow directory for relative path resolution
	if fr.flow.SourcePath != "" {
		fr.script.SetFlowDir(filepath.Dir(fr.flow.SourcePath))
	}

	// Set platform in JS engine
	if info := fr.driver.GetPlatformInfo(); info != nil {
		fr.script.SetPlatform(info.Platform)
	}

	// Apply flow config variables (takes precedence over CLI env)
	if fr.flow.Config.AppID != "" {
		fr.script.SetVariable("APP_ID", fr.flow.Config.AppID)
	}
	fr.script.SetVariables(fr.flow.Config.Env)

	// Apply commandTimeout if specified - overrides driver's default find timeout
	if fr.flow.Config.CommandTimeout > 0 {
		fr.driver.SetFindTimeout(fr.flow.Config.CommandTimeout)
	}

	// Apply waitForIdleTimeout with priority:
	// Flow config > CLI flag > Workspace config > Cap file > Default (5000ms)
	// fr.config.WaitForIdleTimeout already has CLI > Workspace > Cap > Default applied
	// Here we apply flow-level override if specified
	waitForIdleTimeout := fr.config.WaitForIdleTimeout
	if fr.flow.Config.WaitForIdleTimeout != nil {
		waitForIdleTimeout = *fr.flow.Config.WaitForIdleTimeout // flow override (highest priority)
	}
	if err := fr.driver.SetWaitForIdleTimeout(waitForIdleTimeout); err != nil {
		// Log warning but continue - some drivers don't support this
		_ = err // ignore error, just continue
	}

	// Notify flow start
	flowName := fr.detail.Name
	flowFile := filepath.Base(fr.flow.SourcePath)
	if fr.config.OnFlowStart != nil {
		fr.config.OnFlowStart(fr.flowIdx, fr.totalFlows, flowName, flowFile)
	}

	// Mark flow as started
	fr.flowWriter.Start()

	// Execute all steps
	flowStatus := report.StatusPassed
	var flowError string

	// Execute onFlowComplete in defer (runs even on failure)
	defer func() {
		if len(fr.flow.Config.OnFlowComplete) > 0 {
			for _, step := range fr.flow.Config.OnFlowComplete {
				fr.executeNestedStep(step) // Ignore failures in cleanup
			}
		}
	}()

	// Execute onFlowStart hooks
	if len(fr.flow.Config.OnFlowStart) > 0 {
		for _, step := range fr.flow.Config.OnFlowStart {
			result := fr.executeNestedStep(step)
			if !result.Success && !step.IsOptional() {
				// onFlowStart failed - fail the flow
				fr.flowWriter.End(report.StatusFailed)
				errMsg := fmt.Sprintf("onFlowStart failed: %v", result.Error)
				if fr.config.OnFlowEnd != nil {
					fr.config.OnFlowEnd(flowName, false, time.Since(flowStart).Milliseconds(), errMsg)
				}
				return FlowResult{
					ID:           fr.detail.ID,
					Name:         fr.detail.Name,
					Status:       report.StatusFailed,
					Duration:     time.Since(flowStart).Milliseconds(),
					Error:        errMsg,
					StepsTotal:   fr.stepsPassed + fr.stepsFailed + fr.stepsSkipped,
					StepsPassed:  fr.stepsPassed,
					StepsFailed:  fr.stepsFailed,
					StepsSkipped: fr.stepsSkipped,
				}
			}
		}
	}

	for i, step := range fr.flow.Steps {
		// Check context cancellation
		if fr.ctx.Err() != nil {
			fr.flowWriter.SkipRemainingCommands(i)
			flowStatus = report.StatusSkipped
			flowError = "execution cancelled"
			break
		}

		// Execute step
		stepStatus, stepError, stepDuration := fr.executeStep(i, step)

		// Notify step complete
		if fr.config.OnStepComplete != nil {
			fr.config.OnStepComplete(i, step.Describe(), stepStatus == report.StatusPassed, stepDuration, stepError)
		}

		// Track step counts (compound steps like runFlow/repeat/retry don't count themselves,
		// their sub-steps are counted individually in executeNestedStep)
		isCompoundStep := false
		switch step.(type) {
		case *flow.RepeatStep, *flow.RetryStep, *flow.RunFlowStep:
			isCompoundStep = true
		}
		if !isCompoundStep {
			switch stepStatus {
			case report.StatusPassed:
				fr.stepsPassed++
			case report.StatusFailed:
				fr.stepsFailed++
			case report.StatusSkipped:
				fr.stepsSkipped++
			}
		}

		// Handle step result
		if stepStatus == report.StatusFailed {
			if step.IsOptional() {
				// Optional step failure doesn't fail flow
				continue
			}
			// Required step failed - skip remaining and fail flow
			fr.flowWriter.SkipRemainingCommands(i + 1)
			// Count remaining non-compound steps as skipped
			for j := i + 1; j < len(fr.flow.Steps); j++ {
				switch fr.flow.Steps[j].(type) {
				case *flow.RepeatStep, *flow.RetryStep, *flow.RunFlowStep:
					// Compound steps don't count themselves
				default:
					fr.stepsSkipped++
				}
			}
			flowStatus = report.StatusFailed
			flowError = stepError
			break
		}
	}

	// Mark flow as complete
	fr.flowWriter.End(flowStatus)

	// Calculate duration
	flowDuration := time.Since(flowStart).Milliseconds()

	// Notify flow end
	if fr.config.OnFlowEnd != nil {
		fr.config.OnFlowEnd(flowName, flowStatus == report.StatusPassed, flowDuration, flowError)
	}

	logger.Info("=== Flow completed: %s (status: %s, duration: %dms, passed: %d, failed: %d, skipped: %d) ===",
		flowName, flowStatus, flowDuration, fr.stepsPassed, fr.stepsFailed, fr.stepsSkipped)

	return FlowResult{
		ID:           fr.detail.ID,
		Name:         fr.detail.Name,
		Status:       flowStatus,
		Duration:     flowDuration,
		Error:        flowError,
		StepsTotal:   fr.stepsPassed + fr.stepsFailed + fr.stepsSkipped,
		StepsPassed:  fr.stepsPassed,
		StepsFailed:  fr.stepsFailed,
		StepsSkipped: fr.stepsSkipped,
	}
}

// executeStep executes a single step and updates the report.
// Returns status, error message, and duration in milliseconds.
func (fr *FlowRunner) executeStep(idx int, step flow.Step) (report.Status, string, int64) {
	stepStart := time.Now()

	logger.Debug("Executing step %d: %s", idx, step.Describe())

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

	// Expand variables in step before execution
	fr.script.ExpandStep(step)

	// Execute step - route to appropriate handler
	var result *core.CommandResult

	switch s := step.(type) {
	// JS/Scripting steps - handled by ScriptEngine
	case *flow.DefineVariablesStep:
		result = fr.script.ExecuteDefineVariables(s)
	case *flow.RunScriptStep:
		result = fr.script.ExecuteRunScript(s)
	case *flow.EvalScriptStep:
		result = fr.script.ExecuteEvalScript(s)
	case *flow.AssertTrueStep:
		result = fr.script.ExecuteAssertTrue(s)
	case *flow.AssertConditionStep:
		result = fr.script.ExecuteAssertCondition(fr.ctx, s, fr.driver)

	// Flow control steps - handled by FlowRunner
	// Clear sub-commands before compound step execution
	case *flow.RepeatStep:
		fr.subCommands = nil
		result = fr.executeRepeat(s)
	case *flow.RetryStep:
		fr.subCommands = nil
		result = fr.executeRetry(s)
	case *flow.RunFlowStep:
		fr.subCommands = nil
		result = fr.executeRunFlow(s)

	// App lifecycle steps - inject flow's appId if not specified
	case *flow.LaunchAppStep:
		if s.AppID == "" && fr.flow.Config.AppID != "" {
			s.AppID = fr.flow.Config.AppID
		}
		result = fr.driver.Execute(step)
	case *flow.StopAppStep:
		if s.AppID == "" && fr.flow.Config.AppID != "" {
			s.AppID = fr.flow.Config.AppID
		}
		result = fr.driver.Execute(step)
	case *flow.KillAppStep:
		if s.AppID == "" && fr.flow.Config.AppID != "" {
			s.AppID = fr.flow.Config.AppID
		}
		result = fr.driver.Execute(step)
	case *flow.ClearStateStep:
		if s.AppID == "" && fr.flow.Config.AppID != "" {
			s.AppID = fr.flow.Config.AppID
		}
		result = fr.driver.Execute(step)

	// CopyTextFrom - delegate to driver and sync copied text to script engine
	case *flow.CopyTextFromStep:
		fr.script.ExpandStep(step) // Expand variables in selector
		result = fr.driver.Execute(step)
		if result.Success && result.Data != nil {
			if text, ok := result.Data.(string); ok {
				fr.script.SetCopiedText(text)
			}
		}

	// TakeScreenshot - delegate to driver, then save the returned PNG data
	case *flow.TakeScreenshotStep:
		result = fr.driver.Execute(step)
		if result.Success {
			if data, ok := result.Data.([]byte); ok && len(data) > 0 {
				path, saveErr := fr.flowWriter.SaveNamedScreenshot(idx, s.Path, data)
				if saveErr != nil {
					logger.Warn("Failed to save screenshot: %v", saveErr)
				} else {
					artifacts.ScreenshotAfter = path
					result.Message = fmt.Sprintf("Screenshot saved: %s", filepath.Base(path))
				}
			}
		}

	// PasteText - use in-memory copiedText first, clipboard as fallback
	case *flow.PasteTextStep:
		text := fr.script.GetCopiedText()
		if text != "" {
			// Use stored copiedText (like Maestro does)
			inputStep := &flow.InputTextStep{Text: text}
			result = fr.driver.Execute(inputStep)
			if result.Success {
				result.Message = fmt.Sprintf("Pasted text: %s", text)
			}
		} else {
			// Fallback to clipboard
			result = fr.driver.Execute(step)
		}

	// All other steps - delegate to driver
	default:
		result = fr.driver.Execute(step)
	}

	stepDuration := time.Since(stepStart).Milliseconds()

	// Determine status and error
	var status report.Status
	var errorInfo *report.Error
	var errorMsg string

	if result.Success {
		status = report.StatusPassed
		logger.Debug("Step %d completed successfully (%dms): %s", idx, stepDuration, step.Describe())
	} else {
		status = report.StatusFailed
		errorInfo = commandResultToError(result)
		if errorInfo != nil {
			errorMsg = errorInfo.Message
		}
		logger.Error("Step %d failed (%dms): %s - Error: %s", idx, stepDuration, step.Describe(), errorMsg)
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

	// Update report - use CommandEndWithSubs for compound steps
	switch step.(type) {
	case *flow.RepeatStep, *flow.RetryStep, *flow.RunFlowStep:
		fr.flowWriter.CommandEndWithSubs(idx, status, element, errorInfo, artifacts, fr.subCommands)
		fr.subCommands = nil // Clear after use
	default:
		fr.flowWriter.CommandEnd(idx, status, element, errorInfo, artifacts)
	}

	return status, errorMsg, stepDuration
}

// executeRepeat handles repeat step execution.
func (fr *FlowRunner) executeRepeat(step *flow.RepeatStep) *core.CommandResult {
	times := fr.script.ParseInt(step.Times, 1)
	if times <= 0 {
		times = 1000 // Default max iterations for while loops
	}

	hasWhile := step.While.Visible != nil || step.While.NotVisible != nil || step.While.Script != ""

	for i := 0; i < times; i++ {
		// Check context
		if fr.ctx.Err() != nil {
			return &core.CommandResult{
				Success: false,
				Error:   fr.ctx.Err(),
				Message: "Repeat cancelled",
			}
		}

		// Check while condition
		if hasWhile {
			if !fr.script.CheckCondition(fr.ctx, step.While, fr.driver) {
				break // Condition no longer met
			}
		}

		// Execute nested steps
		for _, nestedStep := range step.Steps {
			result := fr.executeNestedStep(nestedStep)
			if !result.Success && !nestedStep.IsOptional() {
				return result
			}
		}
	}

	return &core.CommandResult{
		Success: true,
		Message: fmt.Sprintf("Repeat completed (%d iterations)", times),
	}
}

// executeRetry handles retry step execution.
func (fr *FlowRunner) executeRetry(step *flow.RetryStep) *core.CommandResult {
	maxRetries := fr.script.ParseInt(step.MaxRetries, 3)

	// Apply env variables with restore
	defer fr.script.withEnvVars(step.Env)()

	// If file is specified, load and execute that flow
	if step.File != "" && len(step.Steps) == 0 {
		filePath := fr.script.ResolvePath(step.File)
		subFlow, err := flow.ParseFile(filePath)
		if err != nil {
			return &core.CommandResult{
				Success: false,
				Error:   err,
				Message: fmt.Sprintf("Failed to parse flow file: %s", filePath),
			}
		}
		return fr.executeSubFlowWithRetry(*subFlow, maxRetries)
	}

	// Execute inline steps with retry
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if fr.ctx.Err() != nil {
			return &core.CommandResult{
				Success: false,
				Error:   fr.ctx.Err(),
				Message: "Retry cancelled",
			}
		}

		success := true
		for _, nestedStep := range step.Steps {
			result := fr.executeNestedStep(nestedStep)
			if !result.Success && !nestedStep.IsOptional() {
				lastErr = result.Error
				success = false
				break
			}
		}

		if success {
			return &core.CommandResult{
				Success: true,
				Message: fmt.Sprintf("Retry succeeded on attempt %d", attempt),
			}
		}
	}

	return &core.CommandResult{
		Success: false,
		Error:   lastErr,
		Message: fmt.Sprintf("Retry failed after %d attempts", maxRetries),
	}
}

// executeRunFlow handles runFlow step execution.
func (fr *FlowRunner) executeRunFlow(step *flow.RunFlowStep) *core.CommandResult {
	// Check when condition
	if step.When != nil {
		if !fr.script.CheckCondition(fr.ctx, *step.When, fr.driver) {
			return &core.CommandResult{
				Success: true,
				Message: "Skipped (when condition not met)",
			}
		}
	}

	// Report nested flow start
	if fr.config.OnNestedFlowStart != nil && step.File != "" {
		fr.config.OnNestedFlowStart(fr.depth+1, "Run "+step.File)
	}

	// Increment depth for nested execution
	fr.depth++
	defer func() { fr.depth-- }()

	// Apply env variables with restore
	defer fr.script.withEnvVars(step.Env)()

	// Execute inline steps if present
	if len(step.Steps) > 0 {
		for _, nestedStep := range step.Steps {
			result := fr.executeNestedStep(nestedStep)
			if !result.Success && !nestedStep.IsOptional() {
				return result
			}
		}
		return &core.CommandResult{
			Success: true,
			Message: "Inline flow completed",
		}
	}

	// Load and execute external flow file
	if step.File == "" {
		return &core.CommandResult{
			Success: false,
			Error:   fmt.Errorf("no flow file or commands specified"),
			Message: "runFlow requires file or inline steps",
		}
	}

	filePath := fr.script.ResolvePath(step.File)
	subFlow, err := flow.ParseFile(filePath)
	if err != nil {
		return &core.CommandResult{
			Success: false,
			Error:   err,
			Message: fmt.Sprintf("Failed to parse flow file: %s", filePath),
		}
	}

	return fr.executeSubFlow(*subFlow)
}

// executeNestedStep executes a step without report tracking (for nested execution).
func (fr *FlowRunner) executeNestedStep(step flow.Step) *core.CommandResult {
	start := time.Now()
	var result *core.CommandResult

	// For nested compound steps, we need to track their sub-commands separately
	var nestedSubCommands []report.Command
	isCompoundStep := false
	switch step.(type) {
	case *flow.RepeatStep, *flow.RetryStep, *flow.RunFlowStep:
		isCompoundStep = true
		// Save parent's subCommands and start fresh for this nested compound step
		parentSubCommands := fr.subCommands
		fr.subCommands = nil
		defer func() {
			nestedSubCommands = fr.subCommands
			fr.subCommands = parentSubCommands
		}()
	}

	switch s := step.(type) {
	case *flow.DefineVariablesStep:
		result = fr.script.ExecuteDefineVariables(s)
	case *flow.RunScriptStep:
		result = fr.script.ExecuteRunScript(s)
	case *flow.EvalScriptStep:
		result = fr.script.ExecuteEvalScript(s)
	case *flow.AssertTrueStep:
		result = fr.script.ExecuteAssertTrue(s)
	case *flow.AssertConditionStep:
		result = fr.script.ExecuteAssertCondition(fr.ctx, s, fr.driver)
	case *flow.RepeatStep:
		result = fr.executeRepeat(s)
	case *flow.RetryStep:
		result = fr.executeRetry(s)
	case *flow.RunFlowStep:
		result = fr.executeRunFlow(s)
	case *flow.TakeScreenshotStep:
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		if result.Success {
			if data, ok := result.Data.([]byte); ok && len(data) > 0 {
				subIdx := len(fr.subCommands)
				path, saveErr := fr.flowWriter.SaveNamedScreenshot(subIdx, s.Path, data)
				if saveErr != nil {
					logger.Warn("Failed to save nested screenshot: %v", saveErr)
				} else {
					result.Message = fmt.Sprintf("Screenshot saved: %s", filepath.Base(path))
				}
			}
		}
	case *flow.CopyTextFromStep:
		// Expand variables before driver execution
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		// Sync copied text to script engine
		if result.Success && result.Data != nil {
			if text, ok := result.Data.(string); ok {
				fr.script.SetCopiedText(text)
			}
		}
	default:
		// Expand variables before driver execution
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
	}

	duration := time.Since(start).Milliseconds()

	// Track nested step counts (compound steps like runFlow/repeat/retry don't count themselves)
	if !isCompoundStep {
		if result.Success {
			fr.stepsPassed++
		} else {
			fr.stepsFailed++
		}
	}

	// Report nested step progress
	if fr.config.OnNestedStep != nil && fr.depth > 0 {
		errMsg := ""
		if !result.Success && result.Error != nil {
			errMsg = result.Error.Error()
		}
		fr.config.OnNestedStep(fr.depth, step.Describe(), result.Success, duration, errMsg)
	}

	// Add to parent's sub-commands for report
	status := report.StatusPassed
	if !result.Success {
		status = report.StatusFailed
	}

	now := time.Now()
	cmd := report.Command{
		ID:        fmt.Sprintf("sub-%d", len(fr.subCommands)),
		Index:     len(fr.subCommands),
		Type:      string(step.Type()),
		Label:     step.Label(),
		YAML:      step.Describe(),
		Status:    status,
		StartTime: &start,
		EndTime:   &now,
		Duration:  &duration,
	}

	// Add error info if failed
	if !result.Success && result.Error != nil {
		cmd.Error = &report.Error{
			Type:    "execution",
			Message: result.Error.Error(),
		}
	}

	// Add nested sub-commands for compound steps
	if isCompoundStep {
		cmd.SubCommands = nestedSubCommands
	}

	fr.subCommands = append(fr.subCommands, cmd)

	return result
}

// executeSubFlow executes a sub-flow without separate report tracking.
func (fr *FlowRunner) executeSubFlow(subFlow flow.Flow) *core.CommandResult {
	// Save current flow dir
	prevDir := fr.script.flowDir
	if subFlow.SourcePath != "" {
		fr.script.SetFlowDir(filepath.Dir(subFlow.SourcePath))
	}
	defer func() { fr.script.flowDir = prevDir }()

	// Apply sub-flow env
	defer fr.script.withEnvVars(subFlow.Config.Env)()

	// Execute steps
	for _, step := range subFlow.Steps {
		if fr.ctx.Err() != nil {
			return &core.CommandResult{
				Success: false,
				Error:   fr.ctx.Err(),
				Message: "Sub-flow cancelled",
			}
		}

		// Inject subflow's appId into app lifecycle steps (same as executeStep does for main flow)
		switch s := step.(type) {
		case *flow.LaunchAppStep:
			if s.AppID == "" && subFlow.Config.AppID != "" {
				s.AppID = subFlow.Config.AppID
			}
		case *flow.StopAppStep:
			if s.AppID == "" && subFlow.Config.AppID != "" {
				s.AppID = subFlow.Config.AppID
			}
		case *flow.KillAppStep:
			if s.AppID == "" && subFlow.Config.AppID != "" {
				s.AppID = subFlow.Config.AppID
			}
		case *flow.ClearStateStep:
			if s.AppID == "" && subFlow.Config.AppID != "" {
				s.AppID = subFlow.Config.AppID
			}
		}

		result := fr.executeNestedStep(step)
		if !result.Success && !step.IsOptional() {
			return result
		}
	}

	return &core.CommandResult{
		Success: true,
		Message: fmt.Sprintf("Sub-flow '%s' completed", subFlow.Config.Name),
	}
}

// executeSubFlowWithRetry executes a sub-flow with retry logic.
func (fr *FlowRunner) executeSubFlowWithRetry(subFlow flow.Flow, maxRetries int) *core.CommandResult {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if fr.ctx.Err() != nil {
			return &core.CommandResult{
				Success: false,
				Error:   fr.ctx.Err(),
				Message: "Retry cancelled",
			}
		}

		result := fr.executeSubFlow(subFlow)
		if result.Success {
			return &core.CommandResult{
				Success: true,
				Message: fmt.Sprintf("Retry succeeded on attempt %d", attempt),
			}
		}
		lastErr = result.Error
	}

	return &core.CommandResult{
		Success: false,
		Error:   lastErr,
		Message: fmt.Sprintf("Retry failed after %d attempts", maxRetries),
	}
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
