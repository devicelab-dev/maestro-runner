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

// defaultConditionTimeoutMs is the budget for a `when:`/`while:` condition's
// visible/notVisible check when the condition (or its selector) sets no explicit
// timeout. It is deliberately short: a condition that isn't met should resolve
// quickly rather than blocking on the 7s optional-find timeout. Vanilla Maestro
// is effectively fast here too (it shrinks the budget by time already elapsed
// since the last interaction). A present element still resolves immediately;
// this only bounds how long an unmet condition waits. Tunable via
// SetConditionTimeout / the --condition-timeout flag, and overridable per
// condition with `timeout:`. (#110)
const defaultConditionTimeoutMs = 1000

// ScriptEngine handles JavaScript execution and variable management.
type ScriptEngine struct {
	js                 *jsengine.Engine
	variables          map[string]string
	flowDir            string // Directory of current flow (for resolving relative paths)
	conditionTimeoutMs int    // default timeout for when/while condition checks
}

// NewScriptEngine creates a new script engine.
func NewScriptEngine() *ScriptEngine {
	return &ScriptEngine{
		js:                 jsengine.New(),
		variables:          make(map[string]string),
		conditionTimeoutMs: defaultConditionTimeoutMs,
	}
}

// SetConditionTimeout overrides the default timeout (ms) used for `when:`/
// `while:` condition checks that don't specify their own. A non-positive value
// is ignored (keeps the current default).
func (se *ScriptEngine) SetConditionTimeout(ms int) {
	if ms > 0 {
		se.conditionTimeoutMs = ms
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

// UnsetVariable removes a variable from both the Go map and the JS engine.
func (se *ScriptEngine) UnsetVariable(name string) {
	delete(se.variables, name)
	se.js.UnsetVariable(name)
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
//
// The user's script body runs inside an IIFE so that top-level `const`/`let`/
// `var`/`function` declarations are function-scoped to this invocation. This
// matches Maestro CLI semantics (each runScript gets a fresh scope) and
// avoids the `SyntaxError: Identifier 'foo' has already been declared`
// collision that hits any flow that runs the same script — or two scripts
// declaring the same name — more than once (issue #70). State that should
// outlive a single runScript call still goes through the global `output`
// bag, exactly as documented.
func (se *ScriptEngine) RunScript(script string, env map[string]string) error {
	// Expand variables in script
	script = se.ExpandVariables(script)

	// Apply env variables for the duration of THIS script only, expanded so
	// values like "mockoon-cli start --port ${output.port}" resolve before the
	// script reads them (#107). Scoped via save/restore so a value set here
	// doesn't leak into later runScript calls — matching Maestro's per-script
	// env scope (#109). restore() reverts each var to its prior value, or
	// unsets it entirely if it didn't exist before this run.
	restore := se.applyScopedEnv(env)
	defer restore()

	// Pre-define any referenced-but-undeclared identifier as undefined so the
	// script can use optional env vars via `someVar || default` or
	// `typeof someVar` without a ReferenceError — matching Maestro, where
	// undeclared variables evaluate to undefined rather than throwing (#109).
	// DefineUndefinedIfMissing is conservative (it skips real globals/builtins
	// and already-defined vars), and a local declaration in the script shadows
	// the predefined undefined, so this can't clobber the script's own
	// functions or variables.
	for _, name := range referencedIdentifiers(script) {
		se.js.DefineUndefinedIfMissing(name)
	}

	// Execute the user script inside an IIFE for fresh-scope semantics.
	// The leading newline preserves source-line numbers in error messages.
	wrapped := "(function(){\n" + script + "\n})()"
	if err := se.js.RunScript(wrapped); err != nil {
		return err
	}

	// Sync output back to variables
	se.SyncOutputToVariables()
	return nil
}

// applyScopedEnv sets env vars (with their values variable-expanded) for one
// script run and returns a restore func. Each var is reverted to its prior
// value afterward — or unset if it had none — so runScript env never persists
// into a subsequent script.
func (se *ScriptEngine) applyScopedEnv(env map[string]string) func() {
	type prev struct {
		val     string
		existed bool
	}
	saved := make(map[string]prev, len(env))
	for k := range env {
		v, ok := se.variables[k]
		saved[k] = prev{val: v, existed: ok}
	}
	for k, v := range env {
		se.SetVariable(k, se.ExpandVariables(v))
	}
	return func() {
		for k, p := range saved {
			if p.existed {
				se.SetVariable(k, p.val)
			} else {
				se.UnsetVariable(k)
			}
		}
	}
}

// jsIdentifierPattern matches JavaScript identifiers. The scan is deliberately
// liberal (it also matches property names and tokens inside strings) — that's
// harmless because DefineUndefinedIfMissing only ever defines names that aren't
// already real globals or declared variables.
var jsIdentifierPattern = regexp.MustCompile(`[A-Za-z_$][A-Za-z0-9_$]*`)

// jsHardKeywords are reserved words that can never be a variable reference, so
// there's no point predefining them. Contextual keywords that ARE valid
// identifiers (async, await, let, of, yield, from, …) are intentionally NOT
// here — e.g. a script may use `async` as an optional env var name.
var jsHardKeywords = map[string]bool{
	"var": true, "const": true, "function": true, "return": true,
	"if": true, "else": true, "for": true, "while": true, "do": true,
	"switch": true, "case": true, "default": true, "break": true,
	"continue": true, "new": true, "delete": true, "typeof": true,
	"instanceof": true, "in": true, "void": true, "this": true,
	"true": true, "false": true, "null": true, "undefined": true,
	"try": true, "catch": true, "finally": true, "throw": true,
	"class": true, "extends": true, "super": true, "import": true,
	"export": true,
}

// referencedIdentifiers returns the distinct identifier-like tokens in a script
// worth predefining as undefined (keywords removed). See RunScript for why
// over-matching here is safe.
func referencedIdentifiers(script string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, name := range jsIdentifierPattern.FindAllString(script, -1) {
		if jsHardKeywords[name] || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
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
		// Assertion: keep the driver's full optional-find wait (fallback 0).
		visibleStep.TimeoutMs = conditionTimeout(cond, cond.Visible, 0)
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
		notVisibleStep.TimeoutMs = conditionTimeout(cond, cond.NotVisible, 0)
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
	// Check platform (first — no device call needed)
	if cond.Platform != "" {
		if info := driver.GetPlatformInfo(); info != nil {
			if !strings.EqualFold(cond.Platform, info.Platform) {
				return false
			}
		}
	}

	// Check visible
	if cond.Visible != nil {
		visibleStep := &flow.AssertVisibleStep{Selector: *cond.Visible}
		// when/while: an unmet condition should fail fast (#110).
		visibleStep.TimeoutMs = conditionTimeout(cond, cond.Visible, se.conditionTimeoutMs)
		visibleStep.Optional = true
		result := driver.Execute(visibleStep)
		if !result.Success {
			return false
		}
	}

	// Check notVisible
	if cond.NotVisible != nil {
		notVisibleStep := &flow.AssertNotVisibleStep{Selector: *cond.NotVisible}
		notVisibleStep.TimeoutMs = conditionTimeout(cond, cond.NotVisible, se.conditionTimeoutMs)
		notVisibleStep.Optional = true
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

// conditionTimeout returns the timeout to use for a condition check.
// Priority: 1) Condition.Timeout, 2) Selector.Timeout, 3) the caller's fallback.
// A fallback of 0 lets the driver apply its OptionalFindTimeout (7s) — used for
// assertCondition, where an asserted element should be waited for. The when/
// while path passes the short conditionTimeoutMs so an unmet condition fails
// fast instead of blocking 7s (#110).
func conditionTimeout(cond flow.Condition, sel *flow.Selector, fallback int) int {
	if cond.Timeout > 0 {
		return cond.Timeout
	}
	if sel != nil && sel.Timeout > 0 {
		return sel.Timeout
	}
	return fallback
}

// withEnvVars applies environment variables and returns a restore function.
// Values are expanded through ExpandVariables to support ${VAR || "default"} syntax.
func (se *ScriptEngine) withEnvVars(env map[string]string) func() {
	oldVars := make(map[string]string)
	for k, v := range env {
		oldVars[k] = se.GetVariable(k)
		se.SetVariable(k, se.ExpandVariables(v))
	}
	return func() {
		for k, v := range oldVars {
			se.SetVariable(k, v)
		}
	}
}

// parseBoolExpr converts the resolved value of an `enabled:` argument into a
// boolean. Accepts "true"/"false", "enabled"/"disabled", "on"/"off",
// "yes"/"no", "1"/"0" (case-insensitive); anything else is treated as false.
func parseBoolExpr(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "enabled", "on", "yes", "1":
		return true
	}
	return false
}

// ParseInt parses an integer from string, supporting variable expansion.
// Invalid or empty input silently yields defaultVal. Callers that need to
// reject malformed input (e.g. `times: abc`) should use ParseIntStrict.
func (se *ScriptEngine) ParseInt(s string, defaultVal int) int {
	val, _ := se.ParseIntStrict(s, defaultVal)
	return val
}

// ParseIntStrict parses an integer from string with variable expansion and
// "10_000" grouping. An empty value (field unspecified) yields defaultVal with
// no error; a non-empty value that is not a valid integer yields defaultVal and
// an error, so callers can surface a clear message instead of silently falling
// back to the default.
func (se *ScriptEngine) ParseIntStrict(s string, defaultVal int) (int, error) {
	expanded := strings.ReplaceAll(se.ExpandVariables(s), "_", "")
	if strings.TrimSpace(expanded) == "" {
		return defaultVal, nil
	}
	val, err := strconv.Atoi(expanded)
	if err != nil {
		return defaultVal, fmt.Errorf("%q is not a valid integer", strings.TrimSpace(expanded))
	}
	return val, nil
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
		s.Direction = se.ExpandVariables(s.Direction)
	case *flow.SetAirplaneModeStep:
		if str, ok := s.EnabledRaw.(string); ok {
			s.Enabled = parseBoolExpr(se.ExpandVariables(str))
		}
	case *flow.CopyTextFromStep:
		s.Selector = *se.expandSelector(&s.Selector)
	case *flow.LaunchAppStep:
		s.AppID = se.ExpandVariables(s.AppID)
		for k, v := range s.Arguments {
			if str, ok := v.(string); ok {
				s.Arguments[k] = se.ExpandVariables(str)
			}
		}
		for k, v := range s.Environment {
			s.Environment[k] = se.ExpandVariables(v)
		}
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
	case *flow.RunFlowStep:
		s.File = se.ExpandVariables(s.File)
		s.ElseFile = se.ExpandVariables(s.ElseFile)
		if s.When != nil {
			se.ExpandCondition(s.When)
		}
		for k, v := range s.Env {
			s.Env[k] = se.ExpandVariables(v)
		}
	}
}

// ExpandCondition expands variables in a condition's selector and string
// fields, in place. Used for `runFlow: when:` and `repeat: while:` — the
// latter re-expands every iteration so a loop body that mutates an
// interpolated variable (e.g. ${output.i}) is reflected in the next check.
func (se *ScriptEngine) ExpandCondition(cond *flow.Condition) {
	if cond == nil {
		return
	}
	if cond.Visible != nil {
		cond.Visible = se.expandSelector(cond.Visible)
	}
	if cond.NotVisible != nil {
		cond.NotVisible = se.expandSelector(cond.NotVisible)
	}
	if cond.Script != "" {
		cond.Script = se.ExpandVariables(cond.Script)
	}
	if cond.Platform != "" {
		cond.Platform = se.ExpandVariables(cond.Platform)
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
