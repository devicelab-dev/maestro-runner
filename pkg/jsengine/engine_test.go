package jsengine

import (
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	engine := New()
	defer engine.Close()

	if engine == nil {
		t.Fatal("expected engine to be created")
	}
	if engine.runtime == nil {
		t.Fatal("expected runtime to be initialized")
	}
}

func TestEval(t *testing.T) {
	engine := New()
	defer engine.Close()

	tests := []struct {
		name     string
		script   string
		expected interface{}
	}{
		{"simple number", "1 + 2", int64(3)},
		{"string concat", "'hello' + ' ' + 'world'", "hello world"},
		{"boolean", "true && false", false},
		{"null coalescing", "null ?? 'default'", "default"},
		{"array length", "[1, 2, 3].length", int64(3)},
		{"object property", "({name: 'test'}).name", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Eval(tt.script)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestSetVariable(t *testing.T) {
	engine := New()
	defer engine.Close()

	engine.SetVariable("username", "john")
	engine.SetVariable("count", 42)

	// Test string variable
	result, err := engine.EvalString("username")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "john" {
		t.Errorf("expected 'john', got %q", result)
	}

	// Test number variable
	result, err = engine.EvalString("count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "42" {
		t.Errorf("expected '42', got %q", result)
	}
}

func TestExpandVariables(t *testing.T) {
	engine := New()
	defer engine.Close()

	engine.SetVariable("name", "John")
	engine.SetVariable("age", 30)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple var", "Hello ${name}", "Hello John"},
		{"expression", "Age: ${age + 5}", "Age: 35"},
		{"multiple vars", "${name} is ${age}", "John is 30"},
		{"no vars", "plain text", "plain text"},
		{"string concat", "${name + ' Doe'}", "John Doe"},
		{"nested braces", "${({a: 1}).a}", "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.ExpandVariables(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestConsoleLog(t *testing.T) {
	engine := New()
	defer engine.Close()

	// Just make sure it doesn't panic
	err := engine.RunScript(`
		console.log("test message");
		console.error("error message");
		console.warn("warning message");
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetTimeout(t *testing.T) {
	engine := New()
	defer engine.Close()

	// Set up a flag that will be set by setTimeout
	engine.SetVariable("flag", false)

	err := engine.RunScript(`
		setTimeout(function() {
			flag = true;
		}, 50);
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for timeout to fire
	time.Sleep(100 * time.Millisecond)

	result, err := engine.Eval("flag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != true {
		t.Errorf("expected flag to be true after setTimeout, got %v", result)
	}
}

func TestClearTimeout(t *testing.T) {
	engine := New()
	defer engine.Close()

	engine.SetVariable("flag", false)

	err := engine.RunScript(`
		var id = setTimeout(function() {
			flag = true;
		}, 50);
		clearTimeout(id);
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait longer than the timeout would have been
	time.Sleep(100 * time.Millisecond)

	result, err := engine.Eval("flag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != false {
		t.Errorf("expected flag to still be false after clearTimeout, got %v", result)
	}
}

func TestSetInterval(t *testing.T) {
	engine := New()
	defer engine.Close()

	engine.SetVariable("counter", int64(0))

	err := engine.RunScript(`
		var id = setInterval(function() {
			counter = counter + 1;
		}, 30);

		// Store id for later clearInterval
		intervalId = id;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for a few intervals
	time.Sleep(100 * time.Millisecond)

	// Clear the interval
	engine.RunScript("clearInterval(intervalId)")

	result, err := engine.Eval("counter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have incremented a few times
	counter, ok := result.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T", result)
	}
	if counter < 2 {
		t.Errorf("expected counter >= 2, got %d", counter)
	}
}

func TestJSON(t *testing.T) {
	engine := New()
	defer engine.Close()

	err := engine.RunScript(`
		var data = json('{"name": "test", "value": 123}');
		parsedName = data.name;
		parsedValue = data.value;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	name, _ := engine.EvalString("parsedName")
	if name != "test" {
		t.Errorf("expected 'test', got %q", name)
	}

	value, _ := engine.EvalString("parsedValue")
	if value != "123" {
		t.Errorf("expected '123', got %q", value)
	}
}

func TestOutput(t *testing.T) {
	engine := New()
	defer engine.Close()

	err := engine.RunScript(`
		output.result = "success";
		output.count = 42;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := engine.GetOutput()
	if output["result"] != "success" {
		t.Errorf("expected output.result = 'success', got %v", output["result"])
	}
	if output["count"] != int64(42) {
		t.Errorf("expected output.count = 42, got %v", output["count"])
	}
}

func TestMaestroObject(t *testing.T) {
	engine := New()
	defer engine.Close()

	engine.SetCopiedText("copied value")
	engine.SetPlatform("android")

	// Test copiedText
	result, err := engine.EvalString("maestro.copiedText")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "copied value" {
		t.Errorf("expected 'copied value', got %q", result)
	}

	// Test platform
	result, err = engine.EvalString("maestro.platform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "android" {
		t.Errorf("expected 'android', got %q", result)
	}
}

func TestAsyncAwait(t *testing.T) {
	engine := New()
	defer engine.Close()

	// Test basic async/await syntax
	err := engine.RunScript(`
		async function asyncFunc() {
			return "async result";
		}

		var promise = asyncFunc();
		asyncResult = "pending";

		promise.then(function(result) {
			asyncResult = result;
		});
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give promise time to resolve
	time.Sleep(50 * time.Millisecond)

	result, err := engine.EvalString("asyncResult")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "async result" {
		t.Errorf("expected 'async result', got %q", result)
	}
}

func TestPromise(t *testing.T) {
	engine := New()
	defer engine.Close()

	err := engine.RunScript(`
		var promiseResult = "pending";

		new Promise(function(resolve, reject) {
			resolve("resolved value");
		}).then(function(value) {
			promiseResult = value;
		});
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give promise time to resolve
	time.Sleep(50 * time.Millisecond)

	result, err := engine.EvalString("promiseResult")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "resolved value" {
		t.Errorf("expected 'resolved value', got %q", result)
	}
}

func TestArrowFunctions(t *testing.T) {
	engine := New()
	defer engine.Close()

	result, err := engine.Eval(`
		const add = (a, b) => a + b;
		add(2, 3);
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != int64(5) {
		t.Errorf("expected 5, got %v", result)
	}
}

func TestTemplateLiterals(t *testing.T) {
	engine := New()
	defer engine.Close()

	engine.SetVariable("name", "World")

	result, err := engine.EvalString("`Hello, ${name}!`")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", result)
	}
}

func TestDestructuring(t *testing.T) {
	engine := New()
	defer engine.Close()

	err := engine.RunScript(`
		const {a, b} = {a: 1, b: 2};
		const [x, y] = [3, 4];
		destructured = a + b + x + y;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := engine.Eval("destructured")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != int64(10) {
		t.Errorf("expected 10, got %v", result)
	}
}

func TestHTTPModule(t *testing.T) {
	engine := New()
	defer engine.Close()

	// Just verify the http module exists and has methods
	result, err := engine.Eval("typeof http.get")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "function" {
		t.Errorf("expected http.get to be a function, got %v", result)
	}

	result, err = engine.Eval("typeof http.post")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "function" {
		t.Errorf("expected http.post to be a function, got %v", result)
	}
}

func TestRunScriptError(t *testing.T) {
	engine := New()
	defer engine.Close()

	err := engine.RunScript("invalid javascript {{{{")
	if err == nil {
		t.Error("expected error for invalid javascript")
	}
}

func TestEvalError(t *testing.T) {
	engine := New()
	defer engine.Close()

	_, err := engine.Eval("undefinedVariable.property")
	if err == nil {
		t.Error("expected error for undefined variable")
	}
}

func TestExpandVariablesWithError(t *testing.T) {
	engine := New()
	defer engine.Close()

	// Should not fail, just leave invalid expression as-is
	result, err := engine.ExpandVariables("Value: ${undefinedVar}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The expression evaluation fails, so it should continue
	if !strings.Contains(result, "Value:") {
		t.Errorf("expected result to contain 'Value:', got %q", result)
	}
}
