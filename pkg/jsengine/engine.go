// Package jsengine provides JavaScript expression evaluation for Maestro flows.
package jsengine

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/dop251/goja"
)

// Engine wraps goja runtime with Maestro-compatible features
type Engine struct {
	runtime    *goja.Runtime
	variables  map[string]interface{}
	output     map[string]interface{}
	copiedText string
	platform   string
	timers     *timerRegistry
	mu         sync.Mutex
}

// timerRegistry manages setTimeout/setInterval timers
type timerRegistry struct {
	timers    map[int]*time.Timer
	tickers   map[int]*time.Ticker
	nextID    int
	mu        sync.Mutex
	stopChan  chan struct{}
	closeOnce sync.Once
}

func newTimerRegistry() *timerRegistry {
	return &timerRegistry{
		timers:   make(map[int]*time.Timer),
		tickers:  make(map[int]*time.Ticker),
		nextID:   1,
		stopChan: make(chan struct{}),
	}
}

// New creates a new JS engine instance
func New() *Engine {
	e := &Engine{
		runtime:   goja.New(),
		variables: make(map[string]interface{}),
		output:    make(map[string]interface{}),
		timers:    newTimerRegistry(),
	}

	e.setupBuiltins()
	return e
}

// setupBuiltins registers all built-in functions and objects
func (e *Engine) setupBuiltins() {
	// Console
	e.setupConsole()

	// Timers
	e.setupTimers()

	// JSON helper
	if err := e.runtime.Set("json", e.jsonFunc()); err != nil {
		logger.Warn("failed to set JS runtime global 'json': %v", err)
	}

	// HTTP module
	if err := e.runtime.Set("http", e.httpModule()); err != nil {
		logger.Warn("failed to set JS runtime global 'http': %v", err)
	}

	// Output object (for storing values to pass back to flow).
	// Use a JS-native object rather than a proxy over the Go-side map so that
	// nested mutations (e.g. `output.list.push("a")`) actually persist —
	// proxying a Go map exports nested values as fresh wrappers each read,
	// silently dropping in-place mutations (issue #70).
	if err := e.runtime.Set("output", e.runtime.NewObject()); err != nil {
		logger.Warn("failed to set JS runtime global 'output': %v", err)
	}

	// Maestro object
	if err := e.runtime.Set("maestro", e.maestroObject()); err != nil {
		logger.Warn("failed to set JS runtime global 'maestro': %v", err)
	}
}

// setupConsole adds console.log, console.error, etc.
func (e *Engine) setupConsole() {
	// Helper to create console methods
	makeConsoleFunc := func(prefix string) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			args := make([]interface{}, len(call.Arguments))
			for i, arg := range call.Arguments {
				args[i] = arg.Export()
			}
			if prefix != "" {
				fmt.Println(prefix, args)
			} else {
				fmt.Println(args...)
			}
			return goja.Undefined()
		}
	}

	console := e.runtime.NewObject()
	if err := console.Set("log", makeConsoleFunc("")); err != nil {
		logger.Warn("failed to set console.log: %v", err)
	}
	if err := console.Set("error", makeConsoleFunc("ERROR:")); err != nil {
		logger.Warn("failed to set console.error: %v", err)
	}
	if err := console.Set("warn", makeConsoleFunc("WARN:")); err != nil {
		logger.Warn("failed to set console.warn: %v", err)
	}
	if err := e.runtime.Set("console", console); err != nil {
		logger.Warn("failed to set JS runtime global 'console': %v", err)
	}
}

// setupTimers adds setTimeout, setInterval, clearTimeout, clearInterval
func (e *Engine) setupTimers() {
	// setTimeout
	if err := e.runtime.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(e.runtime.NewTypeError("setTimeout requires 2 arguments"))
		}

		callback, ok := goja.AssertFunction(call.Arguments[0])
		if !ok {
			panic(e.runtime.NewTypeError("first argument must be a function"))
		}

		delay := call.Arguments[1].ToInteger()

		e.timers.mu.Lock()
		id := e.timers.nextID
		e.timers.nextID++

		timer := time.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
			e.mu.Lock()
			defer e.mu.Unlock()

			// Call the callback
			_, err := callback(goja.Undefined())
			if err != nil {
				fmt.Printf("setTimeout callback error: %v\n", err)
			}

			// Clean up
			e.timers.mu.Lock()
			delete(e.timers.timers, id)
			e.timers.mu.Unlock()
		})

		e.timers.timers[id] = timer
		e.timers.mu.Unlock()

		return e.runtime.ToValue(id)
	}); err != nil {
		logger.Warn("failed to set JS runtime global 'setTimeout': %v", err)
	}

	// clearTimeout
	if err := e.runtime.Set("clearTimeout", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}

		id := int(call.Arguments[0].ToInteger())

		e.timers.mu.Lock()
		if timer, ok := e.timers.timers[id]; ok {
			timer.Stop()
			delete(e.timers.timers, id)
		}
		e.timers.mu.Unlock()

		return goja.Undefined()
	}); err != nil {
		logger.Warn("failed to set JS runtime global 'clearTimeout': %v", err)
	}

	// setInterval
	if err := e.runtime.Set("setInterval", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(e.runtime.NewTypeError("setInterval requires 2 arguments"))
		}

		callback, ok := goja.AssertFunction(call.Arguments[0])
		if !ok {
			panic(e.runtime.NewTypeError("first argument must be a function"))
		}

		interval := call.Arguments[1].ToInteger()

		e.timers.mu.Lock()
		id := e.timers.nextID
		e.timers.nextID++

		ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
		e.timers.tickers[id] = ticker
		e.timers.mu.Unlock()

		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-e.timers.stopChan:
					return
				case <-ticker.C:
					e.mu.Lock()
					_, err := callback(goja.Undefined())
					if err != nil {
						fmt.Printf("setInterval callback error: %v\n", err)
					}
					e.mu.Unlock()
				}
			}
		}()

		return e.runtime.ToValue(id)
	}); err != nil {
		logger.Warn("failed to set JS runtime global 'setInterval': %v", err)
	}

	// clearInterval
	if err := e.runtime.Set("clearInterval", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}

		id := int(call.Arguments[0].ToInteger())

		e.timers.mu.Lock()
		if ticker, ok := e.timers.tickers[id]; ok {
			ticker.Stop()
			delete(e.timers.tickers, id)
		}
		e.timers.mu.Unlock()

		return goja.Undefined()
	}); err != nil {
		logger.Warn("failed to set JS runtime global 'clearInterval': %v", err)
	}
}

// jsonFunc returns the json() helper function
func (e *Engine) jsonFunc() func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(e.runtime.NewTypeError("json requires 1 argument"))
		}

		str := call.Arguments[0].String()

		// Parse JSON string and return JS object
		result, err := e.runtime.RunString(fmt.Sprintf("JSON.parse(%q)", str))
		if err != nil {
			panic(e.runtime.NewTypeError(fmt.Sprintf("invalid JSON: %v", err)))
		}

		return result
	}
}

// maestroObject returns the maestro global object
func (e *Engine) maestroObject() *goja.Object {
	obj := e.runtime.NewObject()

	// maestro.copiedText - text copied via copyTextFrom
	if err := obj.DefineAccessorProperty("copiedText", e.runtime.ToValue(func() string {
		return e.copiedText
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		logger.Warn("failed to define maestro.copiedText: %v", err)
	}

	// maestro.platform - current platform (android/ios)
	if err := obj.DefineAccessorProperty("platform", e.runtime.ToValue(func() string {
		return e.platform
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		logger.Warn("failed to define maestro.platform: %v", err)
	}

	return obj
}

// SetVariable sets a variable accessible in JS as a global
func (e *Engine) SetVariable(name string, value interface{}) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.variables[name] = value
	if err := e.runtime.Set(name, value); err != nil {
		logger.Warn("failed to set JS variable '%s': %v", name, err)
	}
}

// SetVariables sets multiple variables
func (e *Engine) SetVariables(vars map[string]interface{}) {
	for k, v := range vars {
		e.SetVariable(k, v)
	}
}

// UnsetVariable removes a global so it no longer resolves as a defined value.
// Used to tear down per-runScript env vars after the script returns, so they
// don't leak into later scripts (matches Maestro's per-script env scope).
func (e *Engine) UnsetVariable(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.variables, name)
	if err := e.runtime.GlobalObject().Delete(name); err != nil {
		// Fall back to undefined if the property can't be deleted.
		_ = e.runtime.Set(name, goja.Undefined())
	}
}

// SetCopiedText sets the copiedText value (from copyTextFrom command)
func (e *Engine) SetCopiedText(text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.copiedText = text
}

// GetCopiedText returns the stored copiedText value
func (e *Engine) GetCopiedText() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.copiedText
}

// SetPlatform sets the current platform
func (e *Engine) SetPlatform(platform string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.platform = platform
}

// GetOutput returns a copy of the output object (values set by scripts)
func (e *Engine) GetOutput() map[string]interface{} {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Export the output object from JS
	outputVal := e.runtime.Get("output")
	var source map[string]interface{}

	if outputVal != nil && !goja.IsUndefined(outputVal) {
		if m, ok := outputVal.Export().(map[string]interface{}); ok {
			source = m
		}
	}

	if source == nil {
		source = e.output
	}

	// Return a copy to prevent external modification
	result := make(map[string]interface{}, len(source))
	for k, v := range source {
		result[k] = v
	}
	return result
}

// Eval evaluates a JavaScript expression and returns the result
func (e *Engine) Eval(script string) (interface{}, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	result, err := e.runtime.RunString(script)
	if err != nil {
		return nil, fmt.Errorf("JS eval error: %w", err)
	}

	return result.Export(), nil
}

// EvalString evaluates a JavaScript expression and returns string result
func (e *Engine) EvalString(script string) (string, error) {
	result, err := e.Eval(script)
	if err != nil {
		return "", err
	}

	if result == nil {
		return "", nil
	}

	return fmt.Sprintf("%v", result), nil
}

// RunScript runs a JavaScript file/script
func (e *Engine) RunScript(script string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, err := e.runtime.RunString(script)
	if err != nil {
		return fmt.Errorf("JS runtime error: %w", err)
	}

	return nil
}

// DefineUndefinedIfMissing defines a variable as undefined if it's not already defined.
// This prevents ReferenceError when scripts reference variables that may not exist.
func (e *Engine) DefineUndefinedIfMissing(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check if already defined
	val := e.runtime.Get(name)
	if val == nil || goja.IsUndefined(val) {
		// Only set if not already defined (nil means not set at all)
		if _, exists := e.variables[name]; !exists {
			if err := e.runtime.Set(name, goja.Undefined()); err != nil {
				logger.Warn("failed to set JS variable '%s' to undefined: %v", name, err)
			}
		}
	}
}

// ExpandVariables expands ${...} expressions in a string using JS evaluation
func (e *Engine) ExpandVariables(text string) (string, error) {
	// Find all ${...} patterns and evaluate them
	result := text
	start := 0

	for {
		// Find ${
		idx := strings.Index(result[start:], "${")
		if idx == -1 {
			break
		}
		idx += start

		// Find matching }
		depth := 1
		end := idx + 2
		for end < len(result) && depth > 0 {
			if result[end] == '{' {
				depth++
			} else if result[end] == '}' {
				depth--
			}
			end++
		}

		if depth != 0 {
			// Unmatched brace, skip
			start = idx + 2
			continue
		}

		// Extract expression
		expr := result[idx+2 : end-1]

		// Evaluate expression, auto-defining undefined variables on ReferenceError
		value, err := e.evalWithUndefinedFallback(expr)
		if err != nil {
			// If evaluation fails, leave as-is or return error
			start = end
			continue
		}

		// Replace in result
		result = result[:idx] + value + result[end:]
		start = idx + len(value)
	}

	return result, nil
}

// evalWithUndefinedFallback evaluates a JS expression, automatically defining
// undefined variables to prevent ReferenceError. This matches Maestro's behavior
// where undeclared variables are treated as undefined (falsy) rather than errors.
// Supports patterns like: ${VAR || "default"}, ${VAR ?? "fallback"}
func (e *Engine) evalWithUndefinedFallback(expr string) (string, error) {
	const maxRetries = 10
	for i := 0; i < maxRetries; i++ {
		value, err := e.EvalString(expr)
		if err == nil {
			return value, nil
		}
		// Check if it's a ReferenceError for an undefined variable
		varName := extractUndefinedVarName(err.Error())
		if varName == "" {
			return "", err // Not a ReferenceError, return original error
		}
		e.DefineUndefinedIfMissing(varName)
	}
	return "", fmt.Errorf("too many undefined variables in expression: %s", expr)
}

// extractUndefinedVarName extracts the variable name from a goja ReferenceError.
// Example: "JS eval error: ReferenceError: APP_ID is not defined at <eval>:1:1(0)"
// Returns "APP_ID", or empty string if not a ReferenceError.
func extractUndefinedVarName(errMsg string) string {
	const prefix = "ReferenceError: "
	const suffix = " is not defined"
	idx := strings.Index(errMsg, prefix)
	if idx == -1 {
		return ""
	}
	after := errMsg[idx+len(prefix):]
	endIdx := strings.Index(after, suffix)
	if endIdx == -1 {
		return ""
	}
	return after[:endIdx]
}

// Close cleans up the engine (stops timers, etc.)
// Safe to call multiple times.
func (e *Engine) Close() {
	e.timers.closeOnce.Do(func() {
		e.timers.mu.Lock()
		defer e.timers.mu.Unlock()

		// Stop all timers
		for _, timer := range e.timers.timers {
			timer.Stop()
		}
		e.timers.timers = make(map[int]*time.Timer)

		// Stop all tickers
		for _, ticker := range e.timers.tickers {
			ticker.Stop()
		}
		e.timers.tickers = make(map[int]*time.Ticker)

		// Signal stop to goroutines
		close(e.timers.stopChan)
	})
}
