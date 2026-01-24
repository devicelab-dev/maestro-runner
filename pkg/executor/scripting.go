package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/jsengine"
)

// envVarPattern matches ALL_CAPS identifiers that look like env variables
var envVarPattern = regexp.MustCompile(`\b([A-Z][A-Z0-9_]{2,})\b`)

// ScriptEngine handles JavaScript execution and variable management.
type ScriptEngine struct {
	js        *jsengine.Engine
	variables map[string]string
	flowDir   string // Directory of current flow (for resolving relative paths)
}

// NewScriptEngine creates a new script engine.
func NewScriptEngine() *ScriptEngine {
	return &ScriptEngine{
		js:        jsengine.New(),
		variables: make(map[string]string),
	}
}

// Close cleans up the script engine.
func (se *ScriptEngine) Close() {
	if se.js != nil {
		se.js.Close()
	}
}

// SetFlowDir sets the current flow directory for relative path resolution.
func (se *ScriptEngine) SetFlowDir(dir string) {
	se.flowDir = dir
}

// SetVariable sets a variable in both Go map and JS engine.
func (se *ScriptEngine) SetVariable(name, value string) {
	se.variables[name] = value
	se.js.SetVariable(name, value)
}

// SetVariables sets multiple variables.
func (se *ScriptEngine) SetVariables(vars map[string]string) {
	for k, v := range vars {
		se.SetVariable(k, v)
	}
}

// ImportSystemEnv imports system environment variables into the script engine.
// Only imports variables matching the pattern (uppercase with underscores).
func (se *ScriptEngine) ImportSystemEnv() {
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			name := parts[0]
			value := parts[1]
			// Import if it matches env var pattern (uppercase like THING, MY_VAR)
			if envVarPattern.MatchString(name) {
				se.SetVariable(name, value)
			}
		}
	}
}

// GetVariable returns a variable value.
func (se *ScriptEngine) GetVariable(name string) string {
	return se.variables[name]
}

// SetPlatform sets the platform in the JS engine.
func (se *ScriptEngine) SetPlatform(platform string) {
	se.js.SetPlatform(platform)
}

// SetCopiedText sets the copied text in the JS engine.
func (se *ScriptEngine) SetCopiedText(text string) {
	se.js.SetCopiedText(text)
}

// GetCopiedText returns the stored copied text.
func (se *ScriptEngine) GetCopiedText() string {
	return se.js.GetCopiedText()
}

// GetOutput returns the JS output variables.
func (se *ScriptEngine) GetOutput() map[string]interface{} {
	return se.js.GetOutput()
}

// SyncOutputToVariables copies JS output back to variables.
func (se *ScriptEngine) SyncOutputToVariables() {
	for k, v := range se.js.GetOutput() {
		se.SetVariable(k, fmt.Sprintf("%v", v))
	}
}

// ExpandVariables expands ${expr} and $VAR syntax in text.
func (se *ScriptEngine) ExpandVariables(text string) string {
	// First pass: JS engine for ${expression} syntax
	result, err := se.js.ExpandVariables(text)
	if err == nil {
		text = result
	}

	// Second pass: expand $VAR syntax (without braces)
	// Sort by length (longest first) to avoid partial matches
	names := make([]string, 0, len(se.variables))
	for name := range se.variables {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return len(names[i]) > len(names[j])
	})

	for _, name := range names {
		value := se.variables[name]
		text = expandDollarVar(text, name, value)
	}

	return text
}

// expandDollarVar replaces $VAR with value, checking word boundaries.
func expandDollarVar(text, name, value string) string {
	pattern := "$" + name
	idx := 0
	for {
		pos := strings.Index(text[idx:], pattern)
		if pos == -1 {
			break
		}
		pos += idx

		// Check if followed by alphanumeric (would be different variable)
		endPos := pos + len(pattern)
		if endPos < len(text) {
			next := text[endPos]
			if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') ||
				(next >= '0' && next <= '9') || next == '_' {
				idx = endPos
				continue
			}
		}

		// Replace
		text = text[:pos] + value + text[endPos:]
		idx = pos + len(value)
	}
	return text
}

// RunScript executes a JavaScript script.
func (se *ScriptEngine) RunScript(script string, env map[string]string) error {
	// Expand variables in script
	script = se.ExpandVariables(script)

	// Apply env variables
	for k, v := range env {
		se.SetVariable(k, v)
	}

	// Pre-define potential env variables as undefined to avoid ReferenceError.
	// This matches Maestro's behavior where undefined variables are falsy rather than errors.
	matches := envVarPattern.FindAllString(script, -1)
	for _, name := range matches {
		se.js.DefineUndefinedIfMissing(name)
	}

	// Execute script
	if err := se.js.RunScript(script); err != nil {
		return err
	}

	// Sync output back to variables
	se.SyncOutputToVariables()
	return nil
}

// EvalCondition evaluates a script condition and returns true/false.
func (se *ScriptEngine) EvalCondition(script string) (bool, error) {
	// Extract JS from ${...} wrapper if present
	script = extractJS(script)
	// Expand any remaining $VAR style variables
	script = se.expandDollarVars(script)

	// Pre-define potential env variables as undefined to avoid ReferenceError
	matches := envVarPattern.FindAllString(script, -1)
	for _, name := range matches {
		se.js.DefineUndefinedIfMissing(name)
	}

	result, err := se.js.Eval(script)
	if err != nil {
		return false, err
	}

	// Convert result to boolean
	switch v := result.(type) {
	case bool:
		return v, nil
	case string:
		return v == "true", nil
	case int64:
		return v != 0, nil
	case float64:
		return v != 0, nil
	default:
		return result != nil, nil
	}
}

// ResolvePath resolves a relative path against the flow directory.
func (se *ScriptEngine) ResolvePath(path string) string {
	if filepath.IsAbs(path) || se.flowDir == "" {
		return path
	}
	return filepath.Join(se.flowDir, path)
}

// ============================================
// Step Execution Helpers
// ============================================

// ExecuteDefineVariables handles defineVariables step.
func (se *ScriptEngine) ExecuteDefineVariables(step *flow.DefineVariablesStep) *core.CommandResult {
	for k, v := range step.Env {
		se.SetVariable(k, se.ExpandVariables(v))
	}
	return &core.CommandResult{
		Success: true,
		Message: fmt.Sprintf("Defined %d variable(s)", len(step.Env)),
	}
}

// ExecuteRunScript handles runScript step.
func (se *ScriptEngine) ExecuteRunScript(step *flow.RunScriptStep) *core.CommandResult {
	script := step.ScriptPath()

	// Check if it's a file path (ends with .js)
	if strings.HasSuffix(script, ".js") {
		filePath := se.ResolvePath(script)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return &core.CommandResult{
				Success: false,
				Error:   err,
				Message: fmt.Sprintf("Cannot read script file: %s", filePath),
			}
		}
		script = string(content)
	}

	if err := se.RunScript(script, step.Env); err != nil {
		return &core.CommandResult{
			Success: false,
			Error:   err,
			Message: fmt.Sprintf("Script execution failed: %v", err),
		}
	}

	return &core.CommandResult{
		Success: true,
		Message: "Script executed successfully",
	}
}

// ExecuteEvalScript handles evalScript step.
func (se *ScriptEngine) ExecuteEvalScript(step *flow.EvalScriptStep) *core.CommandResult {
	script := extractJS(step.Script)
	if err := se.js.RunScript(script); err != nil {
		return &core.CommandResult{
			Success: false,
			Error:   err,
			Message: fmt.Sprintf("Eval failed: %v", err),
		}
	}

	// Sync output back to variables
	se.SyncOutputToVariables()

	return &core.CommandResult{
		Success: true,
		Message: "Eval completed",
	}
}

// extractJS extracts JavaScript from ${...} wrapper if present.
// Maestro uses ${...} syntax to indicate JavaScript expressions.
func extractJS(script string) string {
	script = strings.TrimSpace(script)
	if strings.HasPrefix(script, "${") && strings.HasSuffix(script, "}") {
		return script[2 : len(script)-1]
	}
	return script
}

// expandDollarVars expands $VAR syntax (without braces) using stored variables.
func (se *ScriptEngine) expandDollarVars(text string) string {
	// Sort by length (longest first) to avoid partial matches
	names := make([]string, 0, len(se.variables))
	for name := range se.variables {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return len(names[i]) > len(names[j])
	})

	for _, name := range names {
		value := se.variables[name]
		text = expandDollarVar(text, name, value)
	}

	return text
}

// ExecuteAssertTrue handles assertTrue step.
func (se *ScriptEngine) ExecuteAssertTrue(step *flow.AssertTrueStep) *core.CommandResult {
	result, err := se.EvalCondition(step.Script)
	if err != nil {
		return &core.CommandResult{
			Success: false,
			Error:   err,
			Message: fmt.Sprintf("Assertion evaluation failed: %v", err),
		}
	}

	if !result {
		return &core.CommandResult{
			Success: false,
			Error:   fmt.Errorf("assertion failed"),
			Message: fmt.Sprintf("assertTrue failed: %s", step.Script),
		}
	}

	return &core.CommandResult{
		Success: true,
		Message: "Assertion passed",
	}
}

// ExecuteAssertCondition handles assertCondition step.
func (se *ScriptEngine) ExecuteAssertCondition(ctx context.Context, step *flow.AssertConditionStep, driver core.Driver) *core.CommandResult {
	cond := step.Condition

	// Check platform condition
	if cond.Platform != "" {
		info := driver.GetPlatformInfo()
		if info != nil && !strings.EqualFold(info.Platform, cond.Platform) {
			// Skip on wrong platform (not a failure)
			return &core.CommandResult{
				Success: true,
				Message: fmt.Sprintf("Skipped on platform %s", info.Platform),
			}
		}
	}

	// Check visible condition
	if cond.Visible != nil {
		visibleStep := &flow.AssertVisibleStep{Selector: *cond.Visible}
		result := driver.Execute(visibleStep)
		if !result.Success {
			return &core.CommandResult{
				Success: false,
				Error:   fmt.Errorf("visible condition failed"),
				Message: "assertCondition: visible element not found",
			}
		}
	}

	// Check notVisible condition
	if cond.NotVisible != nil {
		notVisibleStep := &flow.AssertNotVisibleStep{Selector: *cond.NotVisible}
		result := driver.Execute(notVisibleStep)
		if !result.Success {
			return &core.CommandResult{
				Success: false,
				Error:   fmt.Errorf("notVisible condition failed"),
				Message: "assertCondition: element is still visible",
			}
		}
	}

	// Check script condition
	if cond.Script != "" {
		result, err := se.EvalCondition(cond.Script)
		if err != nil {
			return &core.CommandResult{
				Success: false,
				Error:   err,
				Message: fmt.Sprintf("Script condition evaluation failed: %v", err),
			}
		}
		if !result {
			return &core.CommandResult{
				Success: false,
				Error:   fmt.Errorf("script condition returned false"),
				Message: fmt.Sprintf("assertCondition: %s returned false", cond.Script),
			}
		}
	}

	return &core.CommandResult{
		Success: true,
		Message: "Condition passed",
	}
}

// CheckCondition evaluates a flow.Condition and returns true if met.
func (se *ScriptEngine) CheckCondition(ctx context.Context, cond flow.Condition, driver core.Driver) bool {
	// Check visible
	if cond.Visible != nil {
		visibleStep := &flow.AssertVisibleStep{Selector: *cond.Visible}
		result := driver.Execute(visibleStep)
		if !result.Success {
			return false
		}
	}

	// Check notVisible
	if cond.NotVisible != nil {
		notVisibleStep := &flow.AssertNotVisibleStep{Selector: *cond.NotVisible}
		result := driver.Execute(notVisibleStep)
		if !result.Success {
			return false
		}
	}

	// Check script condition
	if cond.Script != "" {
		result, err := se.EvalCondition(cond.Script)
		if err != nil || !result {
			return false
		}
	}

	return true
}

// withEnvVars applies environment variables and returns a restore function.
func (se *ScriptEngine) withEnvVars(env map[string]string) func() {
	oldVars := make(map[string]string)
	for k, v := range env {
		oldVars[k] = se.GetVariable(k)
		se.SetVariable(k, v)
	}
	return func() {
		for k, v := range oldVars {
			se.SetVariable(k, v)
		}
	}
}

// ParseInt parses an integer from string, supporting variable expansion.
func (se *ScriptEngine) ParseInt(s string, defaultVal int) int {
	s = se.ExpandVariables(s)
	s = strings.ReplaceAll(s, "_", "") // Support 10_000 format
	if val, err := strconv.Atoi(s); err == nil {
		return val
	}
	return defaultVal
}

// ExpandStep expands variables in all string fields of a step.
// Note: This modifies the step in place. For steps used in loops,
// the parser creates fresh instances each iteration.
func (se *ScriptEngine) ExpandStep(step flow.Step) {
	switch s := step.(type) {
	case *flow.InputTextStep:
		s.Text = se.ExpandVariables(s.Text)
		s.Selector = *se.expandSelector(&s.Selector)
	case *flow.TapOnStep:
		s.Selector = *se.expandSelector(&s.Selector)
	case *flow.DoubleTapOnStep:
		s.Selector = *se.expandSelector(&s.Selector)
	case *flow.LongPressOnStep:
		s.Selector = *se.expandSelector(&s.Selector)
	case *flow.AssertVisibleStep:
		s.Selector = *se.expandSelector(&s.Selector)
	case *flow.AssertNotVisibleStep:
		s.Selector = *se.expandSelector(&s.Selector)
	case *flow.WaitUntilStep:
		if s.Visible != nil {
			s.Visible = se.expandSelector(s.Visible)
		}
		if s.NotVisible != nil {
			s.NotVisible = se.expandSelector(s.NotVisible)
		}
	case *flow.ScrollUntilVisibleStep:
		s.Element = *se.expandSelector(&s.Element)
	case *flow.CopyTextFromStep:
		s.Selector = *se.expandSelector(&s.Selector)
	case *flow.LaunchAppStep:
		s.AppID = se.ExpandVariables(s.AppID)
	case *flow.StopAppStep:
		s.AppID = se.ExpandVariables(s.AppID)
	case *flow.KillAppStep:
		s.AppID = se.ExpandVariables(s.AppID)
	case *flow.ClearStateStep:
		s.AppID = se.ExpandVariables(s.AppID)
	case *flow.OpenLinkStep:
		s.Link = se.ExpandVariables(s.Link)
	case *flow.PressKeyStep:
		s.Key = se.ExpandVariables(s.Key)
	}
}

// expandSelector expands variables in selector fields and returns a copy.
func (se *ScriptEngine) expandSelector(sel *flow.Selector) *flow.Selector {
	if sel == nil {
		return nil
	}
	// Create a copy to avoid modifying the original
	expanded := *sel
	expanded.Text = se.ExpandVariables(expanded.Text)
	expanded.ID = se.ExpandVariables(expanded.ID)
	expanded.CSS = se.ExpandVariables(expanded.CSS)
	expanded.Index = se.ExpandVariables(expanded.Index)
	expanded.Traits = se.ExpandVariables(expanded.Traits)
	expanded.Point = se.ExpandVariables(expanded.Point)
	expanded.Start = se.ExpandVariables(expanded.Start)
	expanded.End = se.ExpandVariables(expanded.End)
	expanded.Label = se.ExpandVariables(expanded.Label)

	// Expand relative selectors recursively
	expanded.ChildOf = se.expandSelector(sel.ChildOf)
	expanded.Below = se.expandSelector(sel.Below)
	expanded.Above = se.expandSelector(sel.Above)
	expanded.LeftOf = se.expandSelector(sel.LeftOf)
	expanded.RightOf = se.expandSelector(sel.RightOf)
	expanded.ContainsChild = se.expandSelector(sel.ContainsChild)
	if len(sel.ContainsDescendants) > 0 {
		expanded.ContainsDescendants = make([]*flow.Selector, len(sel.ContainsDescendants))
		for i, child := range sel.ContainsDescendants {
			expanded.ContainsDescendants[i] = se.expandSelector(child)
		}
	}
	return &expanded
}
