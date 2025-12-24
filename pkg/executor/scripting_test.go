package executor

import (
	"context"
	"os"
	"path/filepath"
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
		{"$USERNAME", "USER", "admin", "$USERNAME"}, // Should NOT match
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

func TestScriptEngine_RunScript_Error(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	err := se.RunScript("invalid javascript {{{{", nil)
	if err == nil {
		t.Error("RunScript() with invalid JS should return error")
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

func TestScriptEngine_GetOutput(t *testing.T) {
	se := NewScriptEngine()
	defer se.Close()

	se.RunScript("output.key1 = 'value1'; output.key2 = 123", nil)

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
	err := os.WriteFile(scriptPath, []byte("output.fromFile = 'yes'"), 0644)
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
