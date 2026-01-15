package jsengine

import (
	"fmt"
	"strings"
	"sync"
	"time"

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
	e.runtime.Set("json", e.jsonFunc())

	// HTTP module
	e.runtime.Set("http", e.httpModule())

	// Output object (for storing values to pass back to flow)
	e.runtime.Set("output", e.output)

	// Maestro object
	e.runtime.Set("maestro", e.maestroObject())
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
	console.Set("log", makeConsoleFunc(""))
	console.Set("error", makeConsoleFunc("ERROR:"))
	console.Set("warn", makeConsoleFunc("WARN:"))
	e.runtime.Set("console", console)
}

// setupTimers adds setTimeout, setInterval, clearTimeout, clearInterval
func (e *Engine) setupTimers() {
	// setTimeout
	e.runtime.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
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
	})

	// clearTimeout
	e.runtime.Set("clearTimeout", func(call goja.FunctionCall) goja.Value {
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
	})

	// setInterval
	e.runtime.Set("setInterval", func(call goja.FunctionCall) goja.Value {
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
	})

	// clearInterval
	e.runtime.Set("clearInterval", func(call goja.FunctionCall) goja.Value {
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
	})
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
	obj.DefineAccessorProperty("copiedText", e.runtime.ToValue(func() string {
		return e.copiedText
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	// maestro.platform - current platform (android/ios)
	obj.DefineAccessorProperty("platform", e.runtime.ToValue(func() string {
		return e.platform
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

	return obj
}

// SetVariable sets a variable accessible in JS as a global
func (e *Engine) SetVariable(name string, value interface{}) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.variables[name] = value
	e.runtime.Set(name, value)
}

// SetVariables sets multiple variables
func (e *Engine) SetVariables(vars map[string]interface{}) {
	for k, v := range vars {
		e.SetVariable(k, v)
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
			e.runtime.Set(name, goja.Undefined())
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

		// Evaluate expression
		value, err := e.EvalString(expr)
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
