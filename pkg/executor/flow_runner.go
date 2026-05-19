package executor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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
	// Effective wait-for-idle timeout (0 = disabled, used to skip settle)
	waitForIdleTimeout int
	// Active runFlow timeout label (e.g. "3s") for enriching sub-step errors
	runFlowTimeout string
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

	// Set parent context on driver so element-finding respects cancellation
	fr.driver.SetContext(fr.ctx)

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
	// Expand the appId first so that `appId: ${APP_ID}` resolves using CLI -e values
	if fr.flow.Config.AppID != "" {
		expanded := fr.script.ExpandVariables(fr.flow.Config.AppID)
		fr.flow.Config.AppID = expanded
	}
	if appID := fr.flow.Config.EffectiveAppID(); appID != "" {
		fr.script.SetVariable("APP_ID", appID)
	}
	// Expand flow config env values to support ${VAR || "default"} syntax
	for k, v := range fr.flow.Config.Env {
		fr.script.SetVariable(k, fr.script.ExpandVariables(v))
	}

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
	fr.waitForIdleTimeout = waitForIdleTimeout
	if err := fr.driver.SetWaitForIdleTimeout(waitForIdleTimeout); err != nil {
		// Log warning but continue - some drivers don't support this
		_ = err // ignore error, just continue
	}

	// Apply typingFrequency with priority: Flow config > CLI flag > Default (0 = WDA default 60)
	if configurer, ok := core.Unwrap(fr.driver).(core.TypingFrequencyConfigurer); ok {
		typingFrequency := fr.config.TypingFrequency
		if fr.flow.Config.TypingFrequency != nil {
			typingFrequency = *fr.flow.Config.TypingFrequency
		}
		if typingFrequency > 0 {
			_ = configurer.SetTypingFrequency(typingFrequency)
		}
	}

	// Ensure a WDA session exists before execution starts.
	// If launchApp runs later, it reuses this session and updates settings.
	// Use Unwrap to reach through wrapper layers (e.g. FlutterDriver).
	if ensurer, ok := core.Unwrap(fr.driver).(core.SessionEnsurer); ok {
		appID := fr.flow.Config.EffectiveAppID()
		if appID != "" {
			if err := ensurer.EnsureSession(appID); err != nil {
				logger.Warn("failed to ensure session: %v", err)
			}
		}
	}

	// Notify flow start
	flowName := fr.detail.Name
	flowFile := filepath.Base(fr.flow.SourcePath)
	if fr.config.OnFlowStart != nil {
		fr.config.OnFlowStart(fr.flowIdx, fr.totalFlows, flowName, flowFile)
	}

	// Mark flow as started
	fr.flowWriter.Start()

	// Reset any console / page-error noise captured before the first step
	// ran. The CDP browser driver subscribes to Runtime events during
	// construction and the initial navigation to cfg.URL fires events
	// before the user's flow begins — we don't want those polluting the
	// flow's report or double-counting alongside the launchApp navigation.
	// No-op for drivers that don't implement consoleLogReporter.
	resetConsoleLogs(fr.driver)

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
				errMsg := fmt.Sprintf("onFlowStart failed: %v", result.Error)
				fr.flowWriter.End(report.StatusFailed, errMsg)
				if fr.config.OnFlowEnd != nil {
					fr.config.OnFlowEnd(flowName, false, time.Since(flowStart).Milliseconds(), errMsg)
				}
				return FlowResult{
					ID:           fr.detail.ID,
					Name:         fr.detail.Name,
					SourceFile:   fr.flow.SourcePath,
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

	// Pull console / page error entries from the driver (web only).
	// Any driver that exposes ConsoleLogReport() — the CDP browser driver
	// does today, others return nothing — surfaces its captured entries
	// into the flow report so users see JS errors without writing an
	// explicit `assertNoJSErrors` / `getConsoleLogs` step.
	consoleLogs := collectConsoleLogs(fr.driver)
	if len(consoleLogs) > 0 {
		fr.flowWriter.SetConsoleLogs(consoleLogs)
	}

	// Opt-in stricter mode: fail the flow if the captured console contains
	// any error / exception entries. Equivalent to running `assertNoJSErrors`
	// at flow end, without requiring the user to add the step explicitly.
	// Only takes effect when the flow's `failOnConsoleError: true` config is
	// set AND the flow didn't already fail for another reason (we don't want
	// to flip status from failed back to a different failure shape).
	if fr.flow.Config.FailOnConsoleError && flowStatus != report.StatusFailed {
		if msg := jsErrorSummary(consoleLogs); msg != "" {
			flowStatus = report.StatusFailed
			flowError = msg
		}
	}

	// Mark flow as complete. Pass the flow-level error so the report picks
	// up failures that didn't originate from a command (e.g.
	// failOnConsoleError or runFlow timeout) — End() falls back to this
	// when no command-level error is present.
	fr.flowWriter.End(flowStatus, flowError)

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
		SourceFile:   fr.flow.SourcePath,
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

	// App lifecycle steps - inject flow's appId/url if not specified
	case *flow.LaunchAppStep:
		if s.AppID == "" {
			s.AppID = fr.flow.Config.EffectiveAppID()
		}
		result = fr.driver.Execute(step)
	case *flow.StopAppStep:
		if s.AppID == "" {
			s.AppID = fr.flow.Config.EffectiveAppID()
		}
		result = fr.driver.Execute(step)
	case *flow.KillAppStep:
		if s.AppID == "" {
			s.AppID = fr.flow.Config.EffectiveAppID()
		}
		result = fr.driver.Execute(step)
	case *flow.ClearStateStep:
		if s.AppID == "" {
			s.AppID = fr.flow.Config.EffectiveAppID()
		}
		result = fr.driver.Execute(step)

	// EvalBrowserScript - execute JS in browser, store output variable
	case *flow.EvalBrowserScriptStep:
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}

	// RunBrowserScript - execute JS file in browser, store output variable
	case *flow.RunBrowserScriptStep:
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}

	// EvalWebViewScript - execute JS in mobile WebView, store output variable
	case *flow.EvalWebViewScriptStep:
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}

	// RunWebViewScript - execute JS file in mobile WebView, store output variable
	case *flow.RunWebViewScriptStep:
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}

	// GetConsoleLogs - execute and store output variable
	case *flow.GetConsoleLogsStep:
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}

	// GetCookies - execute and store output variable
	case *flow.GetCookiesStep:
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}

	// WaitForRequest - execute and store output variable (request body)
	case *flow.WaitForRequestStep:
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}

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

	// Tap steps - apply repeat/delay/retry/settle options
	case *flow.TapOnStep, *flow.DoubleTapOnStep, *flow.LongPressOnStep:
		opts, _ := extractTapOptions(step)
		result = fr.executeTapWithOptions(step, opts)

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
		// Enrich error with WebView/CDP context
		if errorInfo != nil {
			cdpAvailable := false
			if provider, ok := fr.driver.(core.CDPStateProvider); ok {
				if cdp := provider.CDPState(); cdp != nil && cdp.Available {
					enrichErrorWithCDP(errorInfo, cdp)
					cdpAvailable = true
				}
			}
			// If CDP is not available, do an on-demand WebView check.
			// This is ~30ms (accessibility tree scan) — acceptable on failure.
			if !cdpAvailable {
				if detector, ok := fr.driver.(core.WebViewDetector); ok {
					if wv, err := detector.DetectWebView(); err == nil && wv != nil {
						enrichErrorWithWebView(errorInfo, wv)
					}
				}
			}
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
	hasWhile := step.While.Visible != nil || step.While.NotVisible != nil || step.While.Script != ""

	defaultTimes := 1
	if hasWhile && step.Times == "" {
		defaultTimes = 1000 // Max iterations for while loops without explicit times
	}
	times := fr.script.ParseInt(step.Times, defaultTimes)
	if times <= 0 {
		times = 1
	}

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

		// Brief settle delay for while loops — gives the UI/accessibility tree
		// time to update after actions before re-checking the condition
		if hasWhile {
			time.Sleep(300 * time.Millisecond)
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
	// Check when condition. When false, prefer the else branch over skipping
	// if one was provided.
	if step.When != nil {
		if !fr.script.CheckCondition(fr.ctx, *step.When, fr.driver) {
			if step.ElseFile != "" || len(step.ElseSteps) > 0 {
				return fr.executeRunFlowElse(step)
			}
			return &core.CommandResult{
				Success: true,
				Message: "Skipped (when condition not met)",
			}
		}
	}

	// Apply runFlow timeout if specified
	if step.TimeoutMs > 0 {
		timeout := time.Duration(step.TimeoutMs) * time.Millisecond
		ctx, cancel := context.WithTimeout(fr.ctx, timeout)
		defer cancel()
		origCtx := fr.ctx
		origTimeout := fr.runFlowTimeout
		fr.ctx = ctx
		fr.runFlowTimeout = timeout.String()
		fr.driver.SetContext(ctx)
		defer func() {
			fr.ctx = origCtx
			fr.runFlowTimeout = origTimeout
			fr.driver.SetContext(origCtx)
		}()
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

	// Format timeout limit for error messages
	timeoutLimit := ""
	if step.TimeoutMs > 0 {
		timeoutLimit = (time.Duration(step.TimeoutMs) * time.Millisecond).String()
	}

	// Execute inline steps if present
	if len(step.Steps) > 0 {
		var lastStep string
		for _, nestedStep := range step.Steps {
			if fr.ctx.Err() != nil {
				msg := fmt.Sprintf("runFlow timed out (%s timeout) — last completed: %s", timeoutLimit, lastStep)
				return &core.CommandResult{
					Success: false,
					Error:   fmt.Errorf("%s", msg),
					Message: msg,
				}
			}
			lastStep = nestedStep.Describe()
			result := fr.executeNestedStep(nestedStep)
			if !result.Success && !nestedStep.IsOptional() {
				return fr.wrapRunFlowTimeout(result, step, lastStep, timeoutLimit)
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

	result := fr.executeSubFlow(*subFlow)
	if !result.Success {
		return fr.wrapRunFlowTimeout(result, step, "", timeoutLimit)
	}
	return result
}

// executeRunFlowElse runs the fallback branch of a runFlow step (the `else:`
// or `elseCommands:` block) when the `when:` condition evaluated false.
// Mirrors the body of executeRunFlow for the else inputs — same depth bump,
// env handling, timeout propagation, and sub-flow loading semantics.
func (fr *FlowRunner) executeRunFlowElse(step *flow.RunFlowStep) *core.CommandResult {
	// Apply runFlow timeout if specified (same as main branch).
	if step.TimeoutMs > 0 {
		timeout := time.Duration(step.TimeoutMs) * time.Millisecond
		ctx, cancel := context.WithTimeout(fr.ctx, timeout)
		defer cancel()
		origCtx := fr.ctx
		origTimeout := fr.runFlowTimeout
		fr.ctx = ctx
		fr.runFlowTimeout = timeout.String()
		fr.driver.SetContext(ctx)
		defer func() {
			fr.ctx = origCtx
			fr.runFlowTimeout = origTimeout
			fr.driver.SetContext(origCtx)
		}()
	}

	if fr.config.OnNestedFlowStart != nil && step.ElseFile != "" {
		fr.config.OnNestedFlowStart(fr.depth+1, "Run "+step.ElseFile+" (else)")
	}

	fr.depth++
	defer func() { fr.depth-- }()

	defer fr.script.withEnvVars(step.Env)()

	timeoutLimit := ""
	if step.TimeoutMs > 0 {
		timeoutLimit = (time.Duration(step.TimeoutMs) * time.Millisecond).String()
	}

	// Inline else steps take precedence over ElseFile when both are set.
	if len(step.ElseSteps) > 0 {
		var lastStep string
		for _, nestedStep := range step.ElseSteps {
			if fr.ctx.Err() != nil {
				msg := fmt.Sprintf("runFlow else timed out (%s timeout) — last completed: %s", timeoutLimit, lastStep)
				return &core.CommandResult{
					Success: false,
					Error:   fmt.Errorf("%s", msg),
					Message: msg,
				}
			}
			lastStep = nestedStep.Describe()
			result := fr.executeNestedStep(nestedStep)
			if !result.Success && !nestedStep.IsOptional() {
				return fr.wrapRunFlowTimeout(result, step, lastStep, timeoutLimit)
			}
		}
		return &core.CommandResult{
			Success: true,
			Message: "Else branch completed",
		}
	}

	if step.ElseFile == "" {
		return &core.CommandResult{
			Success: false,
			Error:   fmt.Errorf("runFlow else: no fallback file or steps specified"),
			Message: "runFlow else requires file or inline steps",
		}
	}

	filePath := fr.script.ResolvePath(step.ElseFile)
	subFlow, err := flow.ParseFile(filePath)
	if err != nil {
		return &core.CommandResult{
			Success: false,
			Error:   err,
			Message: fmt.Sprintf("Failed to parse else flow file: %s", filePath),
		}
	}

	result := fr.executeSubFlow(*subFlow)
	if !result.Success {
		return fr.wrapRunFlowTimeout(result, step, "", timeoutLimit)
	}
	return result
}

// wrapRunFlowTimeout replaces cryptic context errors with a clear timeout message
// when a runFlow step fails due to its timeout expiring.
func (fr *FlowRunner) wrapRunFlowTimeout(result *core.CommandResult, step *flow.RunFlowStep, lastStep, timeoutLimit string) *core.CommandResult {
	if timeoutLimit == "" || fr.ctx.Err() == nil {
		return result // not a timeout — return original error
	}

	// Build informative message: include timeout value, flow file, and what was running
	var msg string
	if step.File != "" {
		msg = fmt.Sprintf("runFlow '%s' timed out (%s timeout)", step.File, timeoutLimit)
	} else {
		msg = fmt.Sprintf("runFlow timed out (%s timeout)", timeoutLimit)
	}
	if lastStep != "" {
		msg += fmt.Sprintf(" while executing: %s", lastStep)
	}

	return &core.CommandResult{
		Success: false,
		Error:   fmt.Errorf("%s", msg),
		Message: msg,
	}
}

// enrichTimeoutError replaces cryptic "context deadline exceeded" in sub-step errors
// with a message that references the active runFlow timeout.
func (fr *FlowRunner) enrichTimeoutError(result *core.CommandResult) *core.CommandResult {
	timeout := fr.runFlowTimeout
	enriched := *result // shallow copy

	reason := fmt.Sprintf("runFlow timeout (%s) exceeded", timeout)

	if enriched.Error != nil {
		orig := enriched.Error.Error()
		// Replace the cryptic Go context error with timeout context
		cleaned := strings.ReplaceAll(orig, "context deadline exceeded", reason)
		if cleaned == orig {
			// No replacement happened — append the timeout info
			cleaned = orig + " (" + reason + ")"
		}
		enriched.Error = fmt.Errorf("%s", cleaned)
	}

	if enriched.Message != "" {
		enriched.Message = strings.ReplaceAll(enriched.Message, "context deadline exceeded", reason)
	}

	return &enriched
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
		fr.script.ExpandStep(step)
		result = fr.executeRunFlow(s)

	// App lifecycle steps - inject flow's appId if not specified.
	// Mirrors executeStep so hooks (onFlowStart/onFlowComplete) and other nested
	// invocations resolve the default appId the same way as top-level steps.
	case *flow.LaunchAppStep:
		if s.AppID == "" {
			s.AppID = fr.flow.Config.EffectiveAppID()
		}
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
	case *flow.StopAppStep:
		if s.AppID == "" {
			s.AppID = fr.flow.Config.EffectiveAppID()
		}
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
	case *flow.KillAppStep:
		if s.AppID == "" {
			s.AppID = fr.flow.Config.EffectiveAppID()
		}
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
	case *flow.ClearStateStep:
		if s.AppID == "" {
			s.AppID = fr.flow.Config.EffectiveAppID()
		}
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
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
	case *flow.EvalBrowserScriptStep:
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}
	case *flow.RunBrowserScriptStep:
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}
	case *flow.EvalWebViewScriptStep:
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}
	case *flow.RunWebViewScriptStep:
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}
	case *flow.GetConsoleLogsStep:
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}
	case *flow.GetCookiesStep:
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
			}
		}
	case *flow.WaitForRequestStep:
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
		if result.Success && s.Output != "" {
			if val, ok := result.Data.(string); ok {
				fr.script.SetVariable(s.Output, val)
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
	case *flow.TapOnStep, *flow.DoubleTapOnStep, *flow.LongPressOnStep:
		fr.script.ExpandStep(step)
		opts, _ := extractTapOptions(step)
		result = fr.executeTapWithOptions(step, opts)

	default:
		// Expand variables before driver execution
		fr.script.ExpandStep(step)
		result = fr.driver.Execute(step)
	}

	duration := time.Since(start).Milliseconds()

	// Enrich sub-step errors with runFlow timeout context
	if !result.Success && fr.runFlowTimeout != "" && fr.ctx.Err() != nil {
		result = fr.enrichTimeoutError(result)
	}

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
	var lastStepDesc string
	for _, step := range subFlow.Steps {
		if fr.ctx.Err() != nil {
			msg := "Sub-flow cancelled"
			if lastStepDesc != "" {
				msg = fmt.Sprintf("Sub-flow cancelled after: %s", lastStepDesc)
			}
			return &core.CommandResult{
				Success: false,
				Error:   fr.ctx.Err(),
				Message: msg,
			}
		}
		lastStepDesc = step.Describe()

		// Inject subflow's appId/url into app lifecycle steps (same as executeStep does for main flow)
		switch s := step.(type) {
		case *flow.LaunchAppStep:
			if s.AppID == "" {
				s.AppID = subFlow.Config.EffectiveAppID()
			}
		case *flow.StopAppStep:
			if s.AppID == "" {
				s.AppID = subFlow.Config.EffectiveAppID()
			}
		case *flow.KillAppStep:
			if s.AppID == "" {
				s.AppID = subFlow.Config.EffectiveAppID()
			}
		case *flow.ClearStateStep:
			if s.AppID == "" {
				s.AppID = subFlow.Config.EffectiveAppID()
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

// consoleLogReporter is a duck-typed interface drivers can optionally
// implement to surface browser console / page error entries into the flow
// report. The CDP browser driver implements it (see
// pkg/driver/browser/cdp). Mobile / native drivers don't, and that's fine
// — the type assertion fails and we leave ConsoleLogs nil.
//
// ClearConsoleLogReport is called at the start of each top-level flow so
// noise captured during driver construction (initial navigation before any
// user step ran) doesn't pollute the flow's report.
type consoleLogReporter interface {
	ConsoleLogReport() []report.ConsoleLog
	ClearConsoleLogReport()
}

// collectConsoleLogs extracts console entries from a driver if it
// implements consoleLogReporter, returning nil otherwise.
func collectConsoleLogs(d core.Driver) []report.ConsoleLog {
	provider, ok := core.Unwrap(d).(consoleLogReporter)
	if !ok {
		return nil
	}
	return provider.ConsoleLogReport()
}

// resetConsoleLogs clears the driver's captured-console buffer if it
// supports it. Called at flow start so pre-flow events (driver
// construction's initial navigation) are discarded — only events the
// flow's own steps trigger are included in the per-flow report.
func resetConsoleLogs(d core.Driver) {
	if provider, ok := core.Unwrap(d).(consoleLogReporter); ok {
		provider.ClearConsoleLogReport()
	}
}

// jsErrorSummary returns a human-readable string listing the error- and
// exception-level entries in a captured console buffer, or "" if there are
// none. Used by the failOnConsoleError flow config to produce the failure
// message. Mirrors what `assertNoJSErrors` produces.
func jsErrorSummary(logs []report.ConsoleLog) string {
	if len(logs) == 0 {
		return ""
	}
	var errs []string
	for _, e := range logs {
		if e.Level == "error" || e.Level == "exception" {
			errs = append(errs, fmt.Sprintf("[%s] %s", e.Level, e.Message))
		}
	}
	if len(errs) == 0 {
		return ""
	}
	return fmt.Sprintf("failOnConsoleError: %d JS error(s) detected:\n%s",
		len(errs), strings.Join(errs, "\n"))
}
