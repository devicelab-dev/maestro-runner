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
