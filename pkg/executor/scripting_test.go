package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestNewScriptEngine(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	if se == nil {
		t.Fatal("NewScriptEngine() returned nil")
	}
	if se.js == nil {
		t.Error("js engine not initialized")
	}
	if se.variables == nil {
		t.Error("variables map not initialized")
	}
}

func TestScriptEngine_SetVariable(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("USERNAME", "john")
	se.SetVariable("COUNT", "42")

	if got := se.GetVariable("USERNAME"); got != "john" {
		t.Errorf("GetVariable(USERNAME) = %q, want %q", got, "john")
	}
	if got := se.GetVariable("COUNT"); got != "42" {
		t.Errorf("GetVariable(COUNT) = %q, want %q", got, "42")
	}
}

func TestScriptEngine_SetVariables(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariables(map[string]string{
		"A": "1",
		"B": "2",
	})

	if got := se.GetVariable("A"); got != "1" {
		t.Errorf("GetVariable(A) = %q, want %q", got, "1")
	}
	if got := se.GetVariable("B"); got != "2" {
		t.Errorf("GetVariable(B) = %q, want %q", got, "2")
	}
}

func TestScriptEngine_SetPlatform(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetPlatform("android")
	// Just verify no panic - platform is set in JS engine
}

func TestScriptEngine_SetCopiedText(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetCopiedText("copied text")
	// Just verify no panic - copiedText is set in JS engine
}

func TestScriptEngine_ExpandVariables_JSExpression(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("name", "John")
	se.SetVariable("age", "30")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple var", "Hello ${name}", "Hello John"},
		{"expression", "Age: ${age}", "Age: 30"},
		{"math", "Result: ${1 + 2}", "Result: 3"},
		{"no vars", "plain text", "plain text"},
		{"multiple", "${name} is ${age}", "John is 30"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := se.ExpandVariables(tt.input)
			if got != tt.expected {
				t.Errorf("ExpandVariables(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestScriptEngine_ExpandVariables_DollarVar(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("USER", "admin")
	se.SetVariable("USERNAME", "john")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "Hello $USER", "Hello admin"},
		{"longer first", "Hello $USERNAME", "Hello john"},
		{"end of string", "User: $USER", "User: admin"},
		{"multiple", "$USER and $USERNAME", "admin and john"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := se.ExpandVariables(tt.input)
			if got != tt.expected {
				t.Errorf("ExpandVariables(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExpandDollarVar(t *testing.T) {
	tests := []struct {
		text     string
		name     string
		value    string
		expected string
	}{
		{"Hello $USER", "USER", "admin", "Hello admin"},
		{"$USER", "USER", "admin", "admin"},
		{"$USER!", "USER", "admin", "admin!"},
		{"$USERNAME", "USER", "admin", "$USERNAME"},   // Should NOT match
		{"$USER_NAME", "USER", "admin", "$USER_NAME"}, // Should NOT match
	}

	for _, tt := range tests {
		got := expandDollarVar(tt.text, tt.name, tt.value)
		if got != tt.expected {
			t.Errorf("expandDollarVar(%q, %q, %q) = %q, want %q",
				tt.text, tt.name, tt.value, got, tt.expected)
		}
	}
}

func TestScriptEngine_RunScript(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// Run simple script that sets output
	err := se.RunScript("output.result = 'success'; output.count = 42", nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}

	// Check output was synced to variables
	if got := se.GetVariable("result"); got != "success" {
		t.Errorf("result = %q, want %q", got, "success")
	}
	if got := se.GetVariable("count"); got != "42" {
		t.Errorf("count = %q, want %q", got, "42")
	}
}

func TestScriptEngine_RunScript_WithEnv(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	err := se.RunScript("output.msg = PREFIX + '_test'", map[string]string{
		"PREFIX": "hello",
	})
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}

	if got := se.GetVariable("msg"); got != "hello_test" {
		t.Errorf("msg = %q, want %q", got, "hello_test")
	}
}

// TestScriptEngine_RunScript_EnvValuesAreExpanded reproduces #107: env values
// passed to runScript must be variable-expanded (like defineVariables/runFlow
// env), so an exec string referencing an outer variable and prior output
// resolves before the script reads it — instead of leaking literal ${...}.
func TestScriptEngine_RunScript_EnvValuesAreExpanded(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// Outer variable, as if injected by a runFlow env.
	se.SetVariable("MOCK_FILE", "Login.json")
	// Prior runScript discovered a port and stored it in output.
	if err := se.RunScript("output.mock_port = 8888", nil); err != nil {
		t.Fatalf("seed script: %v", err)
	}

	// Second runScript: env value interpolates both the outer variable and
	// the prior output. The script copies exec to output so we can assert it.
	err := se.RunScript("output.cmd = exec", map[string]string{
		"exec": "mockoon-cli start --data ../mocks/${MOCK_FILE} --port ${output.mock_port}",
	})
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}

	want := "mockoon-cli start --data ../mocks/Login.json --port 8888"
	if got := se.GetVariable("cmd"); got != want {
		t.Errorf("expanded exec = %q, want %q (literal ${...} leaked = bug)", got, want)
	}
}

func TestScriptEngine_RunScript_Error(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	err := se.RunScript("invalid javascript {{{{", nil)
	if err == nil {
		t.Error("RunScript() with invalid JS should return error")
	}
}

// --- Regression tests for issue #110: fast when/while condition checks ---

// captureVisibleTimeout runs a `when: visible:` check through CheckCondition and
// returns the TimeoutMs the engine handed the driver for the visibility lookup.
func captureVisibleTimeout(t *testing.T, se *ScriptEngine, cond flow.Condition) int {
	t.Helper()
	got := -1
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if vs, ok := step.(*flow.AssertVisibleStep); ok {
				got = vs.TimeoutMs
			}
			return &core.CommandResult{Success: false} // not visible → condition false
		},
	}
	se.CheckCondition(context.Background(), cond, driver)
	return got
}

// TestCheckCondition_FastDefaultTimeout reproduces #110: an unmet `when:`
// condition must use the short default, not the driver's 7s optional-find
// timeout (passing 0 would defer to that).
func TestCheckCondition_FastDefaultTimeout(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	got := captureVisibleTimeout(t, se, flow.Condition{Visible: &flow.Selector{Text: "Nope"}})
	if got != defaultConditionTimeoutMs {
		t.Errorf("when-condition timeout = %d, want fast default %d (0 would defer to the 7s optional-find timeout)", got, defaultConditionTimeoutMs)
	}
}

// TestCheckCondition_PerConditionTimeoutWins verifies an explicit per-condition
// timeout still overrides the default.
func TestCheckCondition_PerConditionTimeoutWins(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	got := captureVisibleTimeout(t, se, flow.Condition{Visible: &flow.Selector{Text: "Nope"}, Timeout: 250})
	if got != 250 {
		t.Errorf("per-condition timeout = %d, want 250", got)
	}
}

// TestSetConditionTimeout verifies the global override and that a non-positive
// value is ignored (keeps the existing default).
func TestSetConditionTimeout(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetConditionTimeout(2500)
	if got := captureVisibleTimeout(t, se, flow.Condition{Visible: &flow.Selector{Text: "Nope"}}); got != 2500 {
		t.Errorf("after SetConditionTimeout(2500), timeout = %d, want 2500", got)
	}
	se.SetConditionTimeout(0) // ignored
	if got := captureVisibleTimeout(t, se, flow.Condition{Visible: &flow.Selector{Text: "Nope"}}); got != 2500 {
		t.Errorf("SetConditionTimeout(0) should be ignored; timeout = %d, want 2500", got)
	}
}

// --- Regression tests for issue #109: script env behavior vs Maestro ---

// TestScriptEngine_RunScript_EnvDoesNotLeak reproduces #109: an env var set on
// one runScript must NOT persist into the next runScript (Maestro gives each
// script a fresh env scope). Before the fix, env was set as a sticky global.
func TestScriptEngine_RunScript_EnvDoesNotLeak(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// First script provides returnValue via env.
	if err := se.RunScript("output.first = returnValue", map[string]string{
		"returnValue": "mock_port",
	}); err != nil {
		t.Fatalf("first RunScript: %v", err)
	}
	if got := se.GetVariable("first"); got != "mock_port" {
		t.Fatalf("first = %q, want %q", got, "mock_port")
	}

	// Second script does NOT provide returnValue. It must read as undefined,
	// not the leaked "mock_port" from the previous run.
	if err := se.RunScript(
		`output.second = (typeof returnValue === 'undefined') ? "GONE" : returnValue`, nil,
	); err != nil {
		t.Fatalf("second RunScript: %v", err)
	}
	if got := se.GetVariable("second"); got != "GONE" {
		t.Errorf("returnValue leaked across runScript calls: second = %q, want %q", got, "GONE")
	}
}

// TestScriptEngine_RunScript_EnvRestoresPriorValue ensures a pre-existing
// variable shadowed by a runScript env value is restored afterward, not unset.
func TestScriptEngine_RunScript_EnvRestoresPriorValue(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("TOKEN", "original")
	if err := se.RunScript("output.seen = TOKEN", map[string]string{"TOKEN": "override"}); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if got := se.GetVariable("seen"); got != "override" {
		t.Errorf("inside script TOKEN = %q, want %q", got, "override")
	}
	if got := se.GetVariable("TOKEN"); got != "original" {
		t.Errorf("after script TOKEN = %q, want restored %q", got, "original")
	}
}

// TestScriptEngine_RunScript_UndeclaredEnvIsUndefined reproduces #109: a script
// reusable across scenarios references optional env vars that aren't always
// provided. An unprovided one must read as undefined (so `x || default` and
// `typeof x` work), matching Maestro — not throw ReferenceError. This is the
// reporter's shell_client.js pattern, with `async` omitted from env.
func TestScriptEngine_RunScript_UndeclaredEnvIsUndefined(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	script := `
var is_async = false;
if (typeof async !== 'undefined') { is_async = async; }
output.body = JSON.stringify({ command: exec, async: async || false });
`
	if err := se.RunScript(script, map[string]string{"exec": "echo hi"}); err != nil {
		t.Fatalf("unprovided env var must not throw, got: %v", err)
	}
	want := `{"command":"echo hi","async":false}`
	if got := se.GetVariable("body"); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// TestScriptEngine_RunScript_LocalFunctionNotClobbered guards that the
// undeclared-safety pre-definition doesn't shadow a function the script itself
// declares (a local declaration must win over the predefined-undefined global).
func TestScriptEngine_RunScript_LocalFunctionNotClobbered(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	script := `
function double(n) { return n * 2; }
output.result = double(21);
`
	if err := se.RunScript(script, nil); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if got := se.GetVariable("result"); got != "42" {
		t.Errorf("result = %q, want %q (local function clobbered by pre-define?)", got, "42")
	}
}

// --- Regression tests for issue #70: output mutations + scope reuse ---

// TestScriptEngine_RunScript_OutputArrayPushPersists guards Bug B from #70:
// `output.list.push(...)` mutations must persist across reads, both within
// the same script and across consecutive RunScript calls. Before the fix,
// the `output` global was a Goja proxy over the Go map and exported nested
// values as fresh wrappers each read, silently dropping in-place mutations.
func TestScriptEngine_RunScript_OutputArrayPushPersists(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// Script 1: initialise + push within same script. With the bug, the
	// final output.list would be []; with the fix it must be ["a"].
	if err := se.RunScript(`output.list = []; output.list.push("a"); output.firstLen = output.list.length`, nil); err != nil {
		t.Fatalf("init RunScript: %v", err)
	}
	if got := se.GetVariable("firstLen"); got != "1" {
		t.Errorf("after init, output.list.length = %q, want \"1\" (push within script must persist)", got)
	}

	// Script 2: push from a separate RunScript call. Must see the existing
	// element and append; after this output.list should be ["a","b"].
	if err := se.RunScript(`output.list.push("b"); output.secondLen = output.list.length`, nil); err != nil {
		t.Fatalf("push RunScript: %v", err)
	}
	if got := se.GetVariable("secondLen"); got != "2" {
		t.Errorf("after second push, output.list.length = %q, want \"2\" (push across scripts must persist)", got)
	}

	out := se.GetOutput()
	list, ok := out["list"].([]interface{})
	if !ok {
		t.Fatalf("output.list type = %T, want []interface{}", out["list"])
	}
	if len(list) != 2 || list[0] != "a" || list[1] != "b" {
		t.Errorf("output.list = %v, want [a b]", list)
	}
}

// TestScriptEngine_RunScript_OutputNestedObjectMutation guards the same Bug B
// for object-shaped values: `output.user.name = "x"` must persist.
func TestScriptEngine_RunScript_OutputNestedObjectMutation(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	if err := se.RunScript(`output.user = {}; output.user.name = "Sina"; output.user.count = 1`, nil); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if err := se.RunScript(`output.user.count = output.user.count + 1`, nil); err != nil {
		t.Fatalf("RunScript inc: %v", err)
	}

	out := se.GetOutput()
	user, ok := out["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("output.user type = %T, want map[string]interface{}", out["user"])
	}
	if user["name"] != "Sina" {
		t.Errorf("output.user.name = %v, want Sina", user["name"])
	}
	// Goja exports JS numbers as int64 when they have no fractional part.
	if got := user["count"]; got != int64(2) && got != float64(2) {
		t.Errorf("output.user.count = %v (%T), want 2", got, got)
	}
}

// TestScriptEngine_RunScript_OutputWholeArrayReassign locks in that the
// existing whole-value-reassign workaround keeps working — users who already
// rely on it shouldn't see any behavior change.
func TestScriptEngine_RunScript_OutputWholeArrayReassign(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	if err := se.RunScript(`output.items = [1, 2, 3]`, nil); err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	out := se.GetOutput()
	items, ok := out["items"].([]interface{})
	if !ok || len(items) != 3 {
		t.Fatalf("output.items = %v, want [1 2 3]", out["items"])
	}
}

// TestScriptEngine_RunScript_RepeatedConstNoCollision guards Bug A from #70:
// running the same script twice (or two scripts that both declare a top-
// level `const word`) must not collide. Before the fix, Goja's shared global
// scope produced `SyntaxError: Identifier 'word' has already been declared`
// on the second invocation. The IIFE wrap moves user declarations into a
// per-invocation function scope.
func TestScriptEngine_RunScript_RepeatedConstNoCollision(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	const userScript = `const word = "hello"; output.last = word`

	if err := se.RunScript(userScript, nil); err != nil {
		t.Fatalf("first RunScript: %v", err)
	}
	if err := se.RunScript(userScript, nil); err != nil {
		t.Fatalf("second RunScript collision: %v (issue #70 Bug A regression)", err)
	}
	if got := se.GetVariable("last"); got != "hello" {
		t.Errorf("output.last = %q, want \"hello\"", got)
	}
}

// TestScriptEngine_RunScript_TopLevelLetAlsoIsolated covers `let` for the
// same reason as `const` — the IIFE wrap should isolate it too.
func TestScriptEngine_RunScript_TopLevelLetAlsoIsolated(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	const userScript = `let counter = 0; counter = counter + 1; output.tick = counter`
	for i := 0; i < 3; i++ {
		if err := se.RunScript(userScript, nil); err != nil {
			t.Fatalf("RunScript iteration %d: %v", i, err)
		}
	}
	// counter is function-scoped per invocation, so each call sees fresh 0,
	// increments to 1. output.tick should be "1" after the loop.
	if got := se.GetVariable("tick"); got != "1" {
		t.Errorf("output.tick = %q, want \"1\" (per-invocation scope)", got)
	}
}

func TestScriptEngine_EvalCondition(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("count", "5")

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{"true literal", "true", true},
		{"false literal", "false", false},
		{"comparison true", "count > 3", true},
		{"comparison false", "count > 10", false},
		{"equality", "count == 5", true},
		{"string true", "'true'", true},
		{"string other", "'yes'", false},
		{"empty string", "''", false},
		{"number non-zero", "42", true},
		{"number zero", "0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := se.EvalCondition(tt.script)
			if err != nil {
				t.Fatalf("EvalCondition() error = %v", err)
			}
			if got != tt.expected {
				t.Errorf("EvalCondition(%q) = %v, want %v", tt.script, got, tt.expected)
			}
		})
	}
}

func TestScriptEngine_EvalCondition_Error(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	_, err := se.EvalCondition("undefined_var.property")
	if err == nil {
		t.Error("EvalCondition() with invalid script should return error")
	}
}

func TestScriptEngine_ResolvePath(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// Without flow dir
	if got := se.ResolvePath("test.js"); got != "test.js" {
		t.Errorf("ResolvePath without flowDir = %q, want %q", got, "test.js")
	}

	// With absolute path
	if got := se.ResolvePath("/abs/path.js"); got != "/abs/path.js" {
		t.Errorf("ResolvePath with abs path = %q, want %q", got, "/abs/path.js")
	}

	// With flow dir
	se.SetFlowDir("/flows/login")
	if got := se.ResolvePath("helper.js"); got != "/flows/login/helper.js" {
		t.Errorf("ResolvePath with flowDir = %q, want %q", got, "/flows/login/helper.js")
	}
}

func TestScriptEngine_ParseInt(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("count", "5")

	tests := []struct {
		input    string
		defVal   int
		expected int
	}{
		{"10", 0, 10},
		{"${count}", 0, 5},
		{"10_000", 0, 10000},
		{"invalid", 99, 99},
		{"", 42, 42},
	}

	for _, tt := range tests {
		got := se.ParseInt(tt.input, tt.defVal)
		if got != tt.expected {
			t.Errorf("ParseInt(%q, %d) = %d, want %d", tt.input, tt.defVal, got, tt.expected)
		}
	}
}

func TestScriptEngine_ParseIntStrict(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	se.SetVariable("count", "5")

	tests := []struct {
		input   string
		defVal  int
		want    int
		wantErr bool
	}{
		{"10", 0, 10, false},
		{"${count}", 0, 5, false},
		{"10_000", 0, 10000, false},
		{"", 42, 42, false},   // unspecified → default, no error
		{"  ", 42, 42, false}, // whitespace-only → default, no error
		{"abc", 99, 99, true}, // garbage → default + error
		{"5s", 7, 7, true},    // trailing unit → error (not silently 5)
	}

	for _, tt := range tests {
		got, err := se.ParseIntStrict(tt.input, tt.defVal)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseIntStrict(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("ParseIntStrict(%q, %d) = %d, want %d", tt.input, tt.defVal, got, tt.want)
		}
	}
}

func TestExecuteRepeat_InvalidTimes(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	fr := &FlowRunner{ctx: context.Background(), driver: &mockDriver{}, script: se}

	result := fr.executeRepeat(&flow.RepeatStep{Times: "abc"})
	if result.Success {
		t.Error("expected failure for non-numeric times, got success")
	}
	if !strings.Contains(result.Message, "invalid 'times'") {
		t.Errorf("message = %q, want it to mention invalid 'times'", result.Message)
	}
}

func TestExecuteRetry_InvalidMaxRetries(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	fr := &FlowRunner{ctx: context.Background(), driver: &mockDriver{}, script: se}

	result := fr.executeRetry(&flow.RetryStep{MaxRetries: "five"})
	if result.Success {
		t.Error("expected failure for non-numeric maxRetries, got success")
	}
	if !strings.Contains(result.Message, "invalid 'maxRetries'") {
		t.Errorf("message = %q, want it to mention invalid 'maxRetries'", result.Message)
	}
}

func TestScriptEngine_withEnvVars(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("VAR1", "original1")
	se.SetVariable("VAR2", "original2")

	// Apply env vars and check they're set
	restore := se.withEnvVars(map[string]string{
		"VAR1": "new1",
		"VAR3": "new3",
	})

	if got := se.GetVariable("VAR1"); got != "new1" {
		t.Errorf("VAR1 after apply = %q, want %q", got, "new1")
	}
	if got := se.GetVariable("VAR3"); got != "new3" {
		t.Errorf("VAR3 after apply = %q, want %q", got, "new3")
	}

	// Restore and check original values
	restore()

	if got := se.GetVariable("VAR1"); got != "original1" {
		t.Errorf("VAR1 after restore = %q, want %q", got, "original1")
	}
	if got := se.GetVariable("VAR2"); got != "original2" {
		t.Errorf("VAR2 after restore = %q, want %q", got, "original2")
	}
}

func TestScriptEngine_withEnvVars_DefaultValues(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("CLI_VAR", "from_cli")

	// withEnvVars should expand ${VAR || "default"} syntax
	restore := se.withEnvVars(map[string]string{
		"APP_ID":  `${APP_ID || "com.example.default"}`,
		"USER_ID": `${CLI_VAR || "fallback"}`,
	})

	// APP_ID was undefined, should get default
	if got := se.GetVariable("APP_ID"); got != "com.example.default" {
		t.Errorf("APP_ID = %q, want %q", got, "com.example.default")
	}
	// CLI_VAR was defined, should get its value
	if got := se.GetVariable("USER_ID"); got != "from_cli" {
		t.Errorf("USER_ID = %q, want %q", got, "from_cli")
	}

	restore()
}

func TestScriptEngine_GetOutput(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	if err := se.RunScript("output.key1 = 'value1'; output.key2 = 123", nil); err != nil {
		t.Fatalf("failed to run script: %v", err)
	}

	out := se.GetOutput()
	if out["key1"] != "value1" {
		t.Errorf("output.key1 = %v, want %q", out["key1"], "value1")
	}
}

func TestScriptEngine_ExecuteDefineVariables(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	step := &flow.DefineVariablesStep{
		Env: map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		},
	}

	result := se.ExecuteDefineVariables(step)
	if !result.Success {
		t.Errorf("ExecuteDefineVariables() success = false, want true")
	}

	if got := se.GetVariable("VAR1"); got != "value1" {
		t.Errorf("VAR1 = %q, want %q", got, "value1")
	}
}

func TestScriptEngine_ExecuteRunScript(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	step := &flow.RunScriptStep{
		Script: "output.executed = true",
	}

	result := se.ExecuteRunScript(step)
	if !result.Success {
		t.Errorf("ExecuteRunScript() success = false, error = %v", result.Error)
	}
}

func TestScriptEngine_ExecuteRunScript_File(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// Create temp script file
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.js")
	err := os.WriteFile(scriptPath, []byte("output.fromFile = 'yes'"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	se.SetFlowDir(tmpDir)

	step := &flow.RunScriptStep{
		Script: "test.js",
	}

	result := se.ExecuteRunScript(step)
	if !result.Success {
		t.Errorf("ExecuteRunScript() success = false, error = %v", result.Error)
	}

	if got := se.GetVariable("fromFile"); got != "yes" {
		t.Errorf("fromFile = %q, want %q", got, "yes")
	}
}

func TestScriptEngine_ExecuteRunScript_FileNotFound(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	step := &flow.RunScriptStep{
		Script: "nonexistent.js",
	}

	result := se.ExecuteRunScript(step)
	if result.Success {
		t.Error("ExecuteRunScript() with missing file should fail")
	}
}

func TestScriptEngine_ExecuteEvalScript(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	step := &flow.EvalScriptStep{
		Script: "output.evalResult = 1 + 2",
	}

	result := se.ExecuteEvalScript(step)
	if !result.Success {
		t.Errorf("ExecuteEvalScript() success = false, error = %v", result.Error)
	}
}

func TestScriptEngine_ExecuteEvalScript_Error(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	step := &flow.EvalScriptStep{
		Script: "invalid {{{{",
	}

	result := se.ExecuteEvalScript(step)
	if result.Success {
		t.Error("ExecuteEvalScript() with invalid script should fail")
	}
}

func TestScriptEngine_ExecuteAssertTrue(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("value", "5")

	tests := []struct {
		name    string
		script  string
		success bool
	}{
		{"true condition", "value > 3", true},
		{"false condition", "value > 10", false},
		{"literal true", "true", true},
		{"literal false", "false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &flow.AssertTrueStep{Script: tt.script}
			result := se.ExecuteAssertTrue(step)
			if result.Success != tt.success {
				t.Errorf("ExecuteAssertTrue(%q) success = %v, want %v",
					tt.script, result.Success, tt.success)
			}
		})
	}
}

func TestScriptEngine_ExecuteAssertCondition_Script(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("count", "10")

	driver := &mockDriver{}

	step := &flow.AssertConditionStep{
		Condition: flow.Condition{
			Script: "count > 5",
		},
	}

	result := se.ExecuteAssertCondition(context.Background(), step, driver)
	if !result.Success {
		t.Errorf("ExecuteAssertCondition() success = false, error = %v", result.Error)
	}
}

func TestScriptEngine_ExecuteAssertCondition_ScriptFail(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("count", "3")

	driver := &mockDriver{}

	step := &flow.AssertConditionStep{
		Condition: flow.Condition{
			Script: "count > 5",
		},
	}

	result := se.ExecuteAssertCondition(context.Background(), step, driver)
	if result.Success {
		t.Error("ExecuteAssertCondition() with false condition should fail")
	}
}

func TestScriptEngine_ExecuteAssertCondition_Platform(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{
		platformFunc: func() *core.PlatformInfo {
			return &core.PlatformInfo{Platform: "android"}
		},
	}

	// Matching platform
	step := &flow.AssertConditionStep{
		Condition: flow.Condition{
			Platform: "android",
		},
	}
	result := se.ExecuteAssertCondition(context.Background(), step, driver)
	if !result.Success {
		t.Error("ExecuteAssertCondition() with matching platform should pass")
	}

	// Non-matching platform (should skip/pass)
	step = &flow.AssertConditionStep{
		Condition: flow.Condition{
			Platform: "ios",
		},
	}
	result = se.ExecuteAssertCondition(context.Background(), step, driver)
	if !result.Success {
		t.Error("ExecuteAssertCondition() with non-matching platform should pass (skip)")
	}
}

func TestScriptEngine_ExecuteAssertCondition_Visible(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// Driver that returns success for visible check
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: true}
		},
	}

	step := &flow.AssertConditionStep{
		Condition: flow.Condition{
			Visible: &flow.Selector{Text: "Login"},
		},
	}

	result := se.ExecuteAssertCondition(context.Background(), step, driver)
	if !result.Success {
		t.Errorf("ExecuteAssertCondition() with visible success = false, error = %v", result.Error)
	}
}

func TestScriptEngine_ExecuteAssertCondition_VisibleFail(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// Driver that returns failure for visible check
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: false, Error: &testError{msg: "not found"}}
		},
	}

	step := &flow.AssertConditionStep{
		Condition: flow.Condition{
			Visible: &flow.Selector{Text: "Login"},
		},
	}

	result := se.ExecuteAssertCondition(context.Background(), step, driver)
	if result.Success {
		t.Error("ExecuteAssertCondition() with visible failure should fail")
	}
}

func TestScriptEngine_CheckCondition(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("flag", "true")

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: true}
		},
	}

	// Script condition
	cond := flow.Condition{Script: "flag == 'true'"}
	if !se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() with true script should return true")
	}

	// Visible condition
	cond = flow.Condition{Visible: &flow.Selector{Text: "Test"}}
	if !se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() with visible success should return true")
	}

	// Failed script condition
	cond = flow.Condition{Script: "false"}
	if se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() with false script should return false")
	}
}

// ===========================================
// extractJS tests
// ===========================================

func TestExtractJS(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain script", "true", "true"},
		{"wrapped expression", "${x > 5}", "x > 5"},
		{"wrapped with spaces", "  ${1 + 2}  ", "1 + 2"},
		{"no wrapping", "count > 3", "count > 3"},
		{"partial prefix", "${incomplete", "${incomplete"},
		{"partial suffix", "incomplete}", "incomplete}"},
		{"empty wrapped", "${}", ""},
		{"nested braces", "${obj = {a: 1}}", "obj = {a: 1}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJS(tt.input)
			if got != tt.expected {
				t.Errorf("extractJS(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ===========================================
// EvalCondition with ${...} wrapper tests
// ===========================================

func TestScriptEngine_EvalCondition_WithDollarBraceWrapper(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("count", "10")

	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{"wrapped true", "${true}", true},
		{"wrapped false", "${false}", false},
		{"wrapped comparison", "${count > 5}", true},
		{"wrapped math", "${1 + 1 == 2}", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := se.EvalCondition(tt.script)
			if err != nil {
				t.Fatalf("EvalCondition(%q) error = %v", tt.script, err)
			}
			if got != tt.expected {
				t.Errorf("EvalCondition(%q) = %v, want %v", tt.script, got, tt.expected)
			}
		})
	}
}

func TestScriptEngine_EvalCondition_WithDollarVarExpansion(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("MY_COUNT", "10")

	// $MY_COUNT expands to literal "10" in the script, so "10 > 5" evaluates to true
	got, err := se.EvalCondition("$MY_COUNT > 5")
	if err != nil {
		t.Fatalf("EvalCondition() error = %v", err)
	}
	if !got {
		t.Error("EvalCondition with $VAR expansion should return true")
	}
}

// ===========================================
// ExecuteDefineVariables edge cases
// ===========================================

func TestScriptEngine_ExecuteDefineVariables_WithExpansion(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("BASE_URL", "https://example.com")

	step := &flow.DefineVariablesStep{
		Env: map[string]string{
			"API_URL": "${BASE_URL}/api",
		},
	}

	result := se.ExecuteDefineVariables(step)
	if !result.Success {
		t.Errorf("ExecuteDefineVariables() success = false")
	}

	if got := se.GetVariable("API_URL"); got != "https://example.com/api" {
		t.Errorf("API_URL = %q, want %q", got, "https://example.com/api")
	}
}

func TestScriptEngine_ExecuteDefineVariables_EmptyEnv(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	step := &flow.DefineVariablesStep{
		Env: map[string]string{},
	}

	result := se.ExecuteDefineVariables(step)
	if !result.Success {
		t.Errorf("ExecuteDefineVariables() with empty env success = false")
	}
}

func TestScriptEngine_ExecuteDefineVariables_MessageFormat(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	step := &flow.DefineVariablesStep{
		Env: map[string]string{
			"A": "1",
			"B": "2",
			"C": "3",
		},
	}

	result := se.ExecuteDefineVariables(step)
	if result.Message != "Defined 3 variable(s)" {
		t.Errorf("Message = %q, want %q", result.Message, "Defined 3 variable(s)")
	}
}

// ===========================================
// ExecuteAssertTrue edge cases
// ===========================================

func TestScriptEngine_ExecuteAssertTrue_ErrorMessage(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	step := &flow.AssertTrueStep{Script: "false"}
	result := se.ExecuteAssertTrue(step)

	if result.Success {
		t.Error("ExecuteAssertTrue(false) should fail")
	}
	if result.Error == nil {
		t.Error("ExecuteAssertTrue(false) should set Error")
	}
	expected := "assertTrue failed: false"
	if result.Message != expected {
		t.Errorf("Message = %q, want %q", result.Message, expected)
	}
}

func TestScriptEngine_ExecuteAssertTrue_InvalidScript(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	step := &flow.AssertTrueStep{Script: "invalid {{{{"}
	result := se.ExecuteAssertTrue(step)

	if result.Success {
		t.Error("ExecuteAssertTrue with invalid script should fail")
	}
	if result.Error == nil {
		t.Error("ExecuteAssertTrue with invalid script should set Error")
	}
}

func TestScriptEngine_ExecuteAssertTrue_WithWrappedExpression(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("count", "10")

	step := &flow.AssertTrueStep{Script: "${count > 5}"}
	result := se.ExecuteAssertTrue(step)

	if !result.Success {
		t.Errorf("ExecuteAssertTrue(${count > 5}) success = false, error = %v", result.Error)
	}
}

// ===========================================
// ExpandStep tests
// ===========================================

func TestScriptEngine_ExpandStep_InputTextStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("USERNAME", "john")

	step := &flow.InputTextStep{
		Text:     "Hello ${USERNAME}",
		Selector: flow.Selector{Text: "${USERNAME}", ID: "input_${USERNAME}"},
	}

	se.ExpandStep(step)

	if step.Text != "Hello john" {
		t.Errorf("Text = %q, want %q", step.Text, "Hello john")
	}
	if step.Selector.Text != "john" {
		t.Errorf("Selector.Text = %q, want %q", step.Selector.Text, "john")
	}
	if step.Selector.ID != "input_john" {
		t.Errorf("Selector.ID = %q, want %q", step.Selector.ID, "input_john")
	}
}

func TestScriptEngine_ExpandStep_ScrollUntilVisibleDirection(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("DIR", "UP")

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "target"},
		Direction: "${DIR}",
	}

	se.ExpandStep(step)

	if step.Direction != "UP" {
		t.Errorf("Direction = %q, want %q", step.Direction, "UP")
	}
}

func TestScriptEngine_ExpandStep_SetAirplaneModeInterpolation(t *testing.T) {
	cases := []struct {
		offlineVar string
		want       bool
	}{
		{"true", true},
		{"enabled", true},
		{"1", true},
		{"on", true},
		{"yes", true},
		{"false", false},
		{"disabled", false},
		{"0", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.offlineVar, func(t *testing.T) {
			se := NewScriptEngine()
			defer se.Close()

			se.SetVariable("OFFLINE", tc.offlineVar)
			step := &flow.SetAirplaneModeStep{EnabledRaw: "${OFFLINE}"}

			se.ExpandStep(step)

			if step.Enabled != tc.want {
				t.Errorf("OFFLINE=%q → Enabled=%v, want %v", tc.offlineVar, step.Enabled, tc.want)
			}
		})
	}
}

func TestScriptEngine_ExpandStep_TapOnStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("BUTTON", "Login")

	step := &flow.TapOnStep{
		Selector: flow.Selector{Text: "${BUTTON}"},
	}

	se.ExpandStep(step)

	if step.Selector.Text != "Login" {
		t.Errorf("Selector.Text = %q, want %q", step.Selector.Text, "Login")
	}
}

func TestScriptEngine_ExpandStep_DoubleTapOnStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("ELEM", "icon")

	step := &flow.DoubleTapOnStep{
		Selector: flow.Selector{Text: "${ELEM}"},
	}

	se.ExpandStep(step)

	if step.Selector.Text != "icon" {
		t.Errorf("Selector.Text = %q, want %q", step.Selector.Text, "icon")
	}
}

func TestScriptEngine_ExpandStep_LongPressOnStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("ITEM", "Delete")

	step := &flow.LongPressOnStep{
		Selector: flow.Selector{Text: "${ITEM}"},
	}

	se.ExpandStep(step)

	if step.Selector.Text != "Delete" {
		t.Errorf("Selector.Text = %q, want %q", step.Selector.Text, "Delete")
	}
}

func TestScriptEngine_ExpandStep_AssertVisibleStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("LABEL", "Welcome")

	step := &flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "${LABEL}"},
	}

	se.ExpandStep(step)

	if step.Selector.Text != "Welcome" {
		t.Errorf("Selector.Text = %q, want %q", step.Selector.Text, "Welcome")
	}
}

func TestScriptEngine_ExpandStep_AssertNotVisibleStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("MSG", "Error")

	step := &flow.AssertNotVisibleStep{
		Selector: flow.Selector{Text: "${MSG}"},
	}

	se.ExpandStep(step)

	if step.Selector.Text != "Error" {
		t.Errorf("Selector.Text = %q, want %q", step.Selector.Text, "Error")
	}
}

func TestScriptEngine_ExpandStep_LaunchAppStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("APP_ID", "com.example.app")

	step := &flow.LaunchAppStep{
		AppID: "${APP_ID}",
	}

	se.ExpandStep(step)

	if step.AppID != "com.example.app" {
		t.Errorf("AppID = %q, want %q", step.AppID, "com.example.app")
	}
}

func TestScriptEngine_ExpandStep_LaunchAppStep_Arguments(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("TOKEN", "abc123")
	se.SetVariable("MODE", "debug")

	step := &flow.LaunchAppStep{
		AppID: "com.example.app",
		Arguments: map[string]any{
			"auth_token": "${TOKEN}",
			"mode":       "${MODE}",
			"count":      42, // non-string should be left as-is
		},
	}

	se.ExpandStep(step)

	if step.Arguments["auth_token"] != "abc123" {
		t.Errorf("Arguments[auth_token] = %v, want %q", step.Arguments["auth_token"], "abc123")
	}
	if step.Arguments["mode"] != "debug" {
		t.Errorf("Arguments[mode] = %v, want %q", step.Arguments["mode"], "debug")
	}
	if step.Arguments["count"] != 42 {
		t.Errorf("Arguments[count] = %v, want 42", step.Arguments["count"])
	}
}

func TestScriptEngine_ExpandStep_LaunchAppStep_Environment(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("BASE_URL", "https://api.example.com")
	se.SetVariable("STAGE", "staging")

	step := &flow.LaunchAppStep{
		AppID: "com.example.app",
		Environment: map[string]string{
			"API_URL": "${BASE_URL}/v1",
			"ENV":     "${STAGE}",
		},
	}

	se.ExpandStep(step)

	if step.Environment["API_URL"] != "https://api.example.com/v1" {
		t.Errorf("Environment[API_URL] = %q, want %q", step.Environment["API_URL"], "https://api.example.com/v1")
	}
	if step.Environment["ENV"] != "staging" {
		t.Errorf("Environment[ENV] = %q, want %q", step.Environment["ENV"], "staging")
	}
}

func TestScriptEngine_ExpandStep_StopAppStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("APP_ID", "com.example.app")

	step := &flow.StopAppStep{
		AppID: "${APP_ID}",
	}

	se.ExpandStep(step)

	if step.AppID != "com.example.app" {
		t.Errorf("AppID = %q, want %q", step.AppID, "com.example.app")
	}
}

func TestScriptEngine_ExpandStep_KillAppStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("APP_ID", "com.example.app")

	step := &flow.KillAppStep{
		AppID: "${APP_ID}",
	}

	se.ExpandStep(step)

	if step.AppID != "com.example.app" {
		t.Errorf("AppID = %q, want %q", step.AppID, "com.example.app")
	}
}

func TestScriptEngine_ExpandStep_ClearStateStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("APP_ID", "com.example.app")

	step := &flow.ClearStateStep{
		AppID: "${APP_ID}",
	}

	se.ExpandStep(step)

	if step.AppID != "com.example.app" {
		t.Errorf("AppID = %q, want %q", step.AppID, "com.example.app")
	}
}

// TestScriptEngine_FlowConfigAppID_WithCLIEnv reproduces the bug where
// flow config `appId: ${APP_ID}` overwrites the CLI -e value with the literal.
// The fix: expand the config appId before setting it as a variable.
func TestScriptEngine_FlowConfigAppID_WithCLIEnv(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// Step 1: CLI -e sets APP_ID (simulates fr.script.SetVariables(fr.config.Env))
	se.SetVariable("APP_ID", "com.testhiveapp")

	// Step 2: Flow config has appId: ${APP_ID}
	// The fix: expand BEFORE setting as variable (simulates flow_runner.go:69-71)
	flowConfigAppID := "${APP_ID}"
	expanded := se.ExpandVariables(flowConfigAppID)
	se.SetVariable("APP_ID", expanded)

	// Verify the variable resolves to the CLI value, not the literal
	if expanded != "com.testhiveapp" {
		t.Errorf("ExpandVariables(\"${APP_ID}\") = %q, want %q", expanded, "com.testhiveapp")
	}

	// Step 3: Verify clearState step gets the expanded value
	step := &flow.ClearStateStep{AppID: ""}
	// Simulate: if step.AppID is empty, flow config appId is injected
	if step.AppID == "" {
		step.AppID = expanded // this is what flow_runner does after the fix
	}
	se.ExpandStep(step)

	if step.AppID != "com.testhiveapp" {
		t.Errorf("ClearStateStep.AppID = %q, want %q", step.AppID, "com.testhiveapp")
	}
}

// TestScriptEngine_FlowConfigAppID_Hardcoded verifies that a hardcoded appId
// in flow config still works and takes precedence over CLI -e.
func TestScriptEngine_FlowConfigAppID_Hardcoded(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// CLI -e sets APP_ID
	se.SetVariable("APP_ID", "com.from.cli")

	// Flow config has a hardcoded appId (no variable reference)
	flowConfigAppID := "com.hardcoded.app"
	expanded := se.ExpandVariables(flowConfigAppID)
	se.SetVariable("APP_ID", expanded)

	// Hardcoded value should take precedence
	if expanded != "com.hardcoded.app" {
		t.Errorf("expanded = %q, want %q", expanded, "com.hardcoded.app")
	}

	got := se.ExpandVariables("${APP_ID}")
	if got != "com.hardcoded.app" {
		t.Errorf("APP_ID = %q, want %q", got, "com.hardcoded.app")
	}
}

func TestScriptEngine_ExpandStep_OpenLinkStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("URL", "https://example.com")

	step := &flow.OpenLinkStep{
		Link: "${URL}/path",
	}

	se.ExpandStep(step)

	if step.Link != "https://example.com/path" {
		t.Errorf("Link = %q, want %q", step.Link, "https://example.com/path")
	}
}

func TestScriptEngine_ExpandStep_PressKeyStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("KEY_NAME", "Enter")

	step := &flow.PressKeyStep{
		Key: "${KEY_NAME}",
	}

	se.ExpandStep(step)

	if step.Key != "Enter" {
		t.Errorf("Key = %q, want %q", step.Key, "Enter")
	}
}

func TestScriptEngine_ExpandStep_WaitUntilStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("ELEM", "Loaded")

	step := &flow.WaitUntilStep{
		Visible:    &flow.Selector{Text: "${ELEM}"},
		NotVisible: &flow.Selector{Text: "Loading ${ELEM}"},
	}

	se.ExpandStep(step)

	if step.Visible.Text != "Loaded" {
		t.Errorf("Visible.Text = %q, want %q", step.Visible.Text, "Loaded")
	}
	if step.NotVisible.Text != "Loading Loaded" {
		t.Errorf("NotVisible.Text = %q, want %q", step.NotVisible.Text, "Loading Loaded")
	}
}

func TestScriptEngine_ExpandStep_WaitUntilStep_NilSelectors(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	step := &flow.WaitUntilStep{
		Visible:    nil,
		NotVisible: nil,
	}

	// Should not panic with nil selectors
	se.ExpandStep(step)

	if step.Visible != nil {
		t.Error("Visible should remain nil")
	}
	if step.NotVisible != nil {
		t.Error("NotVisible should remain nil")
	}
}

func TestScriptEngine_ExpandStep_ScrollUntilVisibleStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("TARGET", "Submit")

	step := &flow.ScrollUntilVisibleStep{
		Element: flow.Selector{Text: "${TARGET}"},
	}

	se.ExpandStep(step)

	if step.Element.Text != "Submit" {
		t.Errorf("Element.Text = %q, want %q", step.Element.Text, "Submit")
	}
}

func TestScriptEngine_ExpandStep_CopyTextFromStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("FIELD", "price_label")

	step := &flow.CopyTextFromStep{
		Selector: flow.Selector{ID: "${FIELD}"},
	}

	se.ExpandStep(step)

	if step.Selector.ID != "price_label" {
		t.Errorf("Selector.ID = %q, want %q", step.Selector.ID, "price_label")
	}
}

func TestScriptEngine_ExpandStep_UnhandledStepType(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// BackStep is not handled in ExpandStep switch - should not panic
	step := &flow.BackStep{}
	se.ExpandStep(step)
}

// ===========================================
// expandSelector with relative selectors
// ===========================================

func TestScriptEngine_ExpandSelector_WithRelativeSelectors(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("PARENT", "Container")
	se.SetVariable("CHILD", "Button")
	se.SetVariable("REF", "Header")

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:    "${CHILD}",
			ChildOf: &flow.Selector{Text: "${PARENT}"},
			Below:   &flow.Selector{Text: "${REF}"},
		},
	}

	se.ExpandStep(step)

	if step.Selector.Text != "Button" {
		t.Errorf("Selector.Text = %q, want %q", step.Selector.Text, "Button")
	}
	if step.Selector.ChildOf == nil {
		t.Fatal("ChildOf should not be nil")
	}
	if step.Selector.ChildOf.Text != "Container" {
		t.Errorf("ChildOf.Text = %q, want %q", step.Selector.ChildOf.Text, "Container")
	}
	if step.Selector.Below == nil {
		t.Fatal("Below should not be nil")
	}
	if step.Selector.Below.Text != "Header" {
		t.Errorf("Below.Text = %q, want %q", step.Selector.Below.Text, "Header")
	}
}

func TestScriptEngine_ExpandSelector_AllRelativeTypes(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("VAR", "expanded")

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:          "${VAR}",
			Above:         &flow.Selector{Text: "${VAR}"},
			LeftOf:        &flow.Selector{Text: "${VAR}"},
			RightOf:       &flow.Selector{Text: "${VAR}"},
			ContainsChild: &flow.Selector{Text: "${VAR}"},
			ContainsDescendants: []*flow.Selector{
				{Text: "${VAR}"},
				{ID: "${VAR}"},
			},
		},
	}

	se.ExpandStep(step)

	if step.Selector.Text != "expanded" {
		t.Errorf("Text = %q, want %q", step.Selector.Text, "expanded")
	}
	if step.Selector.Above.Text != "expanded" {
		t.Errorf("Above.Text = %q, want %q", step.Selector.Above.Text, "expanded")
	}
	if step.Selector.LeftOf.Text != "expanded" {
		t.Errorf("LeftOf.Text = %q, want %q", step.Selector.LeftOf.Text, "expanded")
	}
	if step.Selector.RightOf.Text != "expanded" {
		t.Errorf("RightOf.Text = %q, want %q", step.Selector.RightOf.Text, "expanded")
	}
	if step.Selector.ContainsChild.Text != "expanded" {
		t.Errorf("ContainsChild.Text = %q, want %q", step.Selector.ContainsChild.Text, "expanded")
	}
	if len(step.Selector.ContainsDescendants) != 2 {
		t.Fatalf("ContainsDescendants length = %d, want 2", len(step.Selector.ContainsDescendants))
	}
	if step.Selector.ContainsDescendants[0].Text != "expanded" {
		t.Errorf("ContainsDescendants[0].Text = %q, want %q", step.Selector.ContainsDescendants[0].Text, "expanded")
	}
	if step.Selector.ContainsDescendants[1].ID != "expanded" {
		t.Errorf("ContainsDescendants[1].ID = %q, want %q", step.Selector.ContainsDescendants[1].ID, "expanded")
	}
}

func TestScriptEngine_ExpandSelector_AllStringFields(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("VAL", "test")

	step := &flow.TapOnStep{
		Selector: flow.Selector{
			Text:   "${VAL}",
			ID:     "${VAL}_id",
			CSS:    ".${VAL}",
			Index:  "${VAL}",
			Traits: "${VAL}",
			Point:  "${VAL}",
			Start:  "${VAL}",
			End:    "${VAL}",
			Label:  "${VAL}_label",
		},
	}

	se.ExpandStep(step)

	if step.Selector.Text != "test" {
		t.Errorf("Text = %q, want %q", step.Selector.Text, "test")
	}
	if step.Selector.ID != "test_id" {
		t.Errorf("ID = %q, want %q", step.Selector.ID, "test_id")
	}
	if step.Selector.CSS != ".test" {
		t.Errorf("CSS = %q, want %q", step.Selector.CSS, ".test")
	}
	if step.Selector.Index != "test" {
		t.Errorf("Index = %q, want %q", step.Selector.Index, "test")
	}
	if step.Selector.Traits != "test" {
		t.Errorf("Traits = %q, want %q", step.Selector.Traits, "test")
	}
	if step.Selector.Point != "test" {
		t.Errorf("Point = %q, want %q", step.Selector.Point, "test")
	}
	if step.Selector.Start != "test" {
		t.Errorf("Start = %q, want %q", step.Selector.Start, "test")
	}
	if step.Selector.End != "test" {
		t.Errorf("End = %q, want %q", step.Selector.End, "test")
	}
	if step.Selector.Label != "test_label" {
		t.Errorf("Label = %q, want %q", step.Selector.Label, "test_label")
	}
}

// ===========================================
// ExpandVariables edge cases
// ===========================================

func TestScriptEngine_ExpandVariables_EmptyString(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	got := se.ExpandVariables("")
	if got != "" {
		t.Errorf("ExpandVariables(%q) = %q, want %q", "", got, "")
	}
}

func TestScriptEngine_ExpandVariables_NoVariables(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	input := "plain text without any variables"
	got := se.ExpandVariables(input)
	if got != input {
		t.Errorf("ExpandVariables(%q) = %q, want %q", input, got, input)
	}
}

func TestScriptEngine_ExpandVariables_MixedSyntax(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("NAME", "World")

	// Both ${...} and $VAR syntax in the same string
	got := se.ExpandVariables("Hello ${NAME} and $NAME")
	if got != "Hello World and World" {
		t.Errorf("ExpandVariables() = %q, want %q", got, "Hello World and World")
	}
}

// ===========================================
// GetCopiedText tests (0% coverage)
// ===========================================

func TestScriptEngine_GetCopiedText_Empty(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// Before setting anything, copied text should be empty
	got := se.GetCopiedText()
	if got != "" {
		t.Errorf("GetCopiedText() = %q, want empty string", got)
	}
}

func TestScriptEngine_GetCopiedText_AfterSet(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetCopiedText("hello clipboard")
	got := se.GetCopiedText()
	if got != "hello clipboard" {
		t.Errorf("GetCopiedText() = %q, want %q", got, "hello clipboard")
	}
}

func TestScriptEngine_GetCopiedText_Overwrite(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetCopiedText("first")
	se.SetCopiedText("second")
	got := se.GetCopiedText()
	if got != "second" {
		t.Errorf("GetCopiedText() = %q, want %q", got, "second")
	}
}

func TestScriptEngine_GetCopiedText_SpecialCharacters(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetCopiedText("line1\nline2\ttab")
	got := se.GetCopiedText()
	if got != "line1\nline2\ttab" {
		t.Errorf("GetCopiedText() = %q, want %q", got, "line1\nline2\ttab")
	}
}

// ===========================================
// CheckCondition uncovered branches
// ===========================================

func TestScriptEngine_CheckCondition_NotVisible_Success(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: true}
		},
	}

	cond := flow.Condition{NotVisible: &flow.Selector{Text: "Loading"}}
	if !se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() with notVisible success should return true")
	}
}

func TestScriptEngine_CheckCondition_NotVisible_Failure(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: false, Error: &testError{msg: "element is visible"}}
		},
	}

	cond := flow.Condition{NotVisible: &flow.Selector{Text: "Loading"}}
	if se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() with notVisible failure should return false")
	}
}

func TestScriptEngine_CheckCondition_Visible_Failure(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: false, Error: &testError{msg: "not found"}}
		},
	}

	cond := flow.Condition{Visible: &flow.Selector{Text: "Missing"}}
	if se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() with visible failure should return false")
	}
}

func TestScriptEngine_CheckCondition_ScriptError(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{}

	// Script that causes an evaluation error
	cond := flow.Condition{Script: "undefined_var.property"}
	if se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() with script error should return false")
	}
}

func TestScriptEngine_CheckCondition_EmptyCondition(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{}

	// Empty condition with no fields set should return true
	cond := flow.Condition{}
	if !se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() with empty condition should return true")
	}
}

func TestScriptEngine_CheckCondition_AllConditionsMet(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("flag", "true")

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: true}
		},
	}

	// Condition with visible, notVisible, and script all set
	cond := flow.Condition{
		Visible:    &flow.Selector{Text: "Present"},
		NotVisible: &flow.Selector{Text: "Absent"},
		Script:     "flag == 'true'",
	}
	if !se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() with all conditions met should return true")
	}
}

// ===========================================
// CheckCondition platform tests
// ===========================================

func TestScriptEngine_CheckCondition_PlatformMatch(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{
		platformFunc: func() *core.PlatformInfo {
			return &core.PlatformInfo{Platform: "android"}
		},
	}

	cond := flow.Condition{Platform: "Android"}
	if !se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() should return true when platform matches (case-insensitive)")
	}
}

func TestScriptEngine_CheckCondition_PlatformMismatch(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{
		platformFunc: func() *core.PlatformInfo {
			return &core.PlatformInfo{Platform: "ios"}
		},
	}

	cond := flow.Condition{Platform: "Android"}
	if se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() should return false when platform doesn't match")
	}
}

func TestScriptEngine_CheckCondition_PlatformNilInfo(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{
		platformFunc: func() *core.PlatformInfo {
			return nil
		},
	}

	// When PlatformInfo is nil, platform check is skipped (condition passes)
	cond := flow.Condition{Platform: "Android"}
	if !se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() should return true when PlatformInfo is nil")
	}
}

func TestScriptEngine_CheckCondition_PlatformWithVisible(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	executed := false
	driver := &mockDriver{
		platformFunc: func() *core.PlatformInfo {
			return &core.PlatformInfo{Platform: "ios"}
		},
		executeFunc: func(step flow.Step) *core.CommandResult {
			executed = true
			return &core.CommandResult{Success: true}
		},
	}

	// Platform doesn't match — visible check should NOT execute
	cond := flow.Condition{
		Platform: "Android",
		Visible:  &flow.Selector{Text: "Login"},
	}
	if se.CheckCondition(context.Background(), cond, driver) {
		t.Error("CheckCondition() should return false when platform doesn't match")
	}
	if executed {
		t.Error("visible check should not execute when platform already failed")
	}
}

// ===========================================
// ExecuteAssertCondition uncovered branches
// ===========================================

func TestScriptEngine_ExecuteAssertCondition_NotVisible_Success(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: true}
		},
	}

	step := &flow.AssertConditionStep{
		Condition: flow.Condition{
			NotVisible: &flow.Selector{Text: "Loading"},
		},
	}

	result := se.ExecuteAssertCondition(context.Background(), step, driver)
	if !result.Success {
		t.Errorf("ExecuteAssertCondition() with notVisible success = false, error = %v", result.Error)
	}
}

func TestScriptEngine_ExecuteAssertCondition_NotVisible_Failure(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: false, Error: &testError{msg: "still visible"}}
		},
	}

	step := &flow.AssertConditionStep{
		Condition: flow.Condition{
			NotVisible: &flow.Selector{Text: "Loading"},
		},
	}

	result := se.ExecuteAssertCondition(context.Background(), step, driver)
	if result.Success {
		t.Error("ExecuteAssertCondition() with notVisible failure should fail")
	}
	if result.Message != "assertCondition: element is still visible" {
		t.Errorf("Message = %q, want %q", result.Message, "assertCondition: element is still visible")
	}
}

func TestScriptEngine_ExecuteAssertCondition_ScriptError(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{}

	step := &flow.AssertConditionStep{
		Condition: flow.Condition{
			Script: "undefined_var.property",
		},
	}

	result := se.ExecuteAssertCondition(context.Background(), step, driver)
	if result.Success {
		t.Error("ExecuteAssertCondition() with script error should fail")
	}
	if result.Error == nil {
		t.Error("ExecuteAssertCondition() with script error should set Error")
	}
}

func TestScriptEngine_ExecuteAssertCondition_PlatformNilInfo(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// Driver that returns nil platform info
	driver := &mockDriver{
		platformFunc: func() *core.PlatformInfo {
			return nil
		},
	}

	step := &flow.AssertConditionStep{
		Condition: flow.Condition{
			Platform: "android",
		},
	}

	// When info is nil, the platform check is skipped (not a skip condition)
	// and it falls through to the end, returning "Condition passed"
	result := se.ExecuteAssertCondition(context.Background(), step, driver)
	if !result.Success {
		t.Errorf("ExecuteAssertCondition() with nil platform info should pass, error = %v", result.Error)
	}
}

func TestScriptEngine_ExecuteAssertCondition_EmptyCondition(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	driver := &mockDriver{}

	step := &flow.AssertConditionStep{
		Condition: flow.Condition{},
	}

	result := se.ExecuteAssertCondition(context.Background(), step, driver)
	if !result.Success {
		t.Error("ExecuteAssertCondition() with empty condition should pass")
	}
	if result.Message != "Condition passed" {
		t.Errorf("Message = %q, want %q", result.Message, "Condition passed")
	}
}

// ===========================================
// EvalCondition additional branch coverage
// ===========================================

func TestScriptEngine_EvalCondition_Float64NonZero(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// 3.14 returns float64 from JS
	got, err := se.EvalCondition("3.14")
	if err != nil {
		t.Fatalf("EvalCondition() error = %v", err)
	}
	if !got {
		t.Error("EvalCondition(3.14) should return true for non-zero float")
	}
}

func TestScriptEngine_EvalCondition_Float64Zero(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// 0.0 returns float64 from JS
	got, err := se.EvalCondition("0.0")
	if err != nil {
		t.Fatalf("EvalCondition() error = %v", err)
	}
	if got {
		t.Error("EvalCondition(0.0) should return false for zero float")
	}
}

func TestScriptEngine_EvalCondition_NullResult(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// null in JS should return false (default case, result == nil)
	got, err := se.EvalCondition("null")
	if err != nil {
		t.Fatalf("EvalCondition() error = %v", err)
	}
	if got {
		t.Error("EvalCondition(null) should return false")
	}
}

func TestScriptEngine_EvalCondition_UndefinedVariable(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	// Undefined env var should be pre-defined as undefined (falsy)
	got, err := se.EvalCondition("SOME_UNDEFINED_VAR")
	if err != nil {
		t.Fatalf("EvalCondition() error = %v", err)
	}
	if got {
		t.Error("EvalCondition(SOME_UNDEFINED_VAR) should return false for undefined variable")
	}
}

// ===========================================
// ExpandStep: RunFlowStep
// ===========================================

func TestScriptEngine_ExpandStep_RunFlowStep(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("FLOW_FILE", "auth.yaml")
	se.SetVariable("BUTTON_ID", "profile_button")
	se.SetVariable("LABEL_TEXT", "Welcome")
	se.SetVariable("ENV_VAL", "production")

	step := &flow.RunFlowStep{
		File: "${FLOW_FILE}",
		When: &flow.Condition{
			Visible:    &flow.Selector{ID: "${BUTTON_ID}"},
			NotVisible: &flow.Selector{Text: "${LABEL_TEXT}"},
			Script:     "${BUTTON_ID} !== undefined",
			Platform:   "${ENV_VAL}",
		},
		Env: map[string]string{
			"MODE": "${ENV_VAL}",
		},
	}

	se.ExpandStep(step)

	if step.File != "auth.yaml" {
		t.Errorf("File = %q, want %q", step.File, "auth.yaml")
	}
	if step.When.Visible.ID != "profile_button" {
		t.Errorf("When.Visible.ID = %q, want %q", step.When.Visible.ID, "profile_button")
	}
	if step.When.NotVisible.Text != "Welcome" {
		t.Errorf("When.NotVisible.Text = %q, want %q", step.When.NotVisible.Text, "Welcome")
	}
	if step.When.Script != "profile_button !== undefined" {
		t.Errorf("When.Script = %q, want %q", step.When.Script, "profile_button !== undefined")
	}
	if step.When.Platform != "production" {
		t.Errorf("When.Platform = %q, want %q", step.When.Platform, "production")
	}
	if step.Env["MODE"] != "production" {
		t.Errorf("Env[MODE] = %q, want %q", step.Env["MODE"], "production")
	}
}

func TestScriptEngine_ExpandStep_RunFlowStep_NilWhen(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("FILE", "test.yaml")

	// RunFlowStep with no When condition should not panic
	step := &flow.RunFlowStep{
		File: "${FILE}",
	}

	se.ExpandStep(step)

	if step.File != "test.yaml" {
		t.Errorf("File = %q, want %q", step.File, "test.yaml")
	}
}

func TestScriptEngine_ExpandCondition(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.SetVariable("IDX", "3")
	se.SetVariable("LABEL", "Done")
	se.SetVariable("PLAT", "ios")

	cond := &flow.Condition{
		Visible:    &flow.Selector{ID: "tab-${IDX}"},
		NotVisible: &flow.Selector{Text: "${LABEL}"},
		Script:     "${IDX} > 0",
		Platform:   "${PLAT}",
	}

	se.ExpandCondition(cond)

	if cond.Visible.ID != "tab-3" {
		t.Errorf("Visible.ID = %q, want %q", cond.Visible.ID, "tab-3")
	}
	if cond.NotVisible.Text != "Done" {
		t.Errorf("NotVisible.Text = %q, want %q", cond.NotVisible.Text, "Done")
	}
	if cond.Script != "3 > 0" {
		t.Errorf("Script = %q, want %q", cond.Script, "3 > 0")
	}
	if cond.Platform != "ios" {
		t.Errorf("Platform = %q, want %q", cond.Platform, "ios")
	}
}

func TestScriptEngine_ExpandCondition_Nil(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	se.ExpandCondition(nil) // must not panic
}

// TestExecuteRepeat_WhileInterpolation reproduces issue #97: a `while:`
// condition that interpolates a variable mutated by the loop body must be
// re-expanded with the current value each iteration. The loop body bumps
// output.i, and the while selector is id=tab-${output.i}; the driver reports
// tab-1 and tab-2 visible but not tab-3, so the loop must run exactly twice
// and query the freshly-interpolated id each pass — never the literal template.
func TestExecuteRepeat_WhileInterpolation(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()
	if err := se.RunScript("output.i = 1", nil); err != nil {
		t.Fatalf("seed script failed: %v", err)
	}

	var queriedIDs []string
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			if av, ok := step.(*flow.AssertVisibleStep); ok {
				queriedIDs = append(queriedIDs, av.Selector.ID)
				// tab-1 and tab-2 are "visible"; tab-3 is not.
				visible := av.Selector.ID == "tab-1" || av.Selector.ID == "tab-2"
				return &core.CommandResult{Success: visible}
			}
			return &core.CommandResult{Success: true}
		},
	}

	fr := &FlowRunner{ctx: context.Background(), driver: driver, script: se}

	step := &flow.RepeatStep{
		Times: "10",
		While: flow.Condition{Visible: &flow.Selector{ID: "tab-${output.i}"}},
		Steps: []flow.Step{
			&flow.EvalScriptStep{Script: "output.i = output.i + 1"},
		},
	}

	result := fr.executeRepeat(step)
	if !result.Success {
		t.Fatalf("executeRepeat failed: %v", result.Error)
	}

	// Loop runs while tab-1 (i=1) and tab-2 (i=2) are visible, stops at tab-3.
	want := []string{"tab-1", "tab-2", "tab-3"}
	if len(queriedIDs) != len(want) {
		t.Fatalf("queried IDs = %v, want %v", queriedIDs, want)
	}
	for i, id := range want {
		if queriedIDs[i] != id {
			t.Errorf("query %d = %q, want %q (literal template leaked = re-expansion broken)", i, queriedIDs[i], id)
		}
	}
}
