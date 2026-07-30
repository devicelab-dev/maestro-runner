package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_SingleFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.yaml")

	content := `
appId: com.example.app
---
- tapOn: "Login"
- inputText: "username"
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(file)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case, got %d", len(result.TestCases))
	}
}

func TestValidate_Directory(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"flow1.yaml": `- tapOn: "Button1"`,
		"flow2.yaml": `- tapOn: "Button2"`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != 2 {
		t.Errorf("expected 2 test cases, got %d", len(result.TestCases))
	}
}

func TestValidate_RunFlowResolution(t *testing.T) {
	dir := t.TempDir()

	// Main flow references sub flow
	mainFlow := `
- tapOn: "Start"
- runFlow: subflow.yaml
- tapOn: "End"
`
	subFlow := `
- tapOn: "SubStep1"
- tapOn: "SubStep2"
`

	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte(mainFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "subflow.yaml"), []byte(subFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "main.yaml"))

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Only main.yaml is a test case; subflow.yaml is a dependency (validated but not in TestCases)
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_NestedRunFlow(t *testing.T) {
	dir := t.TempDir()

	// main -> sub1 -> sub2
	mainFlow := `- runFlow: sub1.yaml`
	sub1Flow := `- runFlow: sub2.yaml`
	sub2Flow := `- tapOn: "Deep"`

	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte(mainFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub1.yaml"), []byte(sub1Flow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub2.yaml"), []byte(sub2Flow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "main.yaml"))

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Only main.yaml is a test case; sub1.yaml and sub2.yaml are dependencies
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case, got %d", len(result.TestCases))
	}
}

func TestValidate_CircularDependency(t *testing.T) {
	dir := t.TempDir()

	// a -> b -> a (circular)
	flowA := `- runFlow: b.yaml`
	flowB := `- runFlow: a.yaml`

	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(flowA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(flowB), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "a.yaml"))

	if result.IsValid() {
		t.Error("expected circular dependency error")
	}

	// Check for circular dependency error message
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err.Error(), "circular dependency") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected circular dependency error, got: %v", result.Errors)
	}
}

func TestValidate_SelfReference(t *testing.T) {
	dir := t.TempDir()

	// Flow references itself
	flow := `- runFlow: self.yaml`

	if err := os.WriteFile(filepath.Join(dir, "self.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "self.yaml"))

	if result.IsValid() {
		t.Error("expected circular dependency error for self-reference")
	}
}

func TestValidate_MissingRunFlowFile(t *testing.T) {
	dir := t.TempDir()

	flow := `- runFlow: nonexistent.yaml`

	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "main.yaml"))

	if result.IsValid() {
		t.Error("expected error for missing runFlow file")
	}
}

func TestValidate_InvalidYAML(t *testing.T) {
	dir := t.TempDir()

	flow := `- tapOn: [invalid yaml`

	if err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "invalid.yaml"))

	if result.IsValid() {
		t.Error("expected parse error for invalid YAML")
	}
}

func TestValidate_TagFiltering(t *testing.T) {
	dir := t.TempDir()

	smokeFlow := `
appId: com.example
tags:
  - smoke
---
- tapOn: "Smoke"
`
	regressionFlow := `
appId: com.example
tags:
  - regression
---
- tapOn: "Regression"
`

	if err := os.WriteFile(filepath.Join(dir, "smoke.yaml"), []byte(smokeFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "regression.yaml"), []byte(regressionFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test include tags
	v := New([]string{"smoke"}, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case with smoke tag, got %d", len(result.TestCases))
	}

	// Test exclude tags
	v = New(nil, []string{"regression"})
	result = v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case without regression tag, got %d", len(result.TestCases))
	}
}

func TestValidate_NonExistentPath(t *testing.T) {
	v := New(nil, nil)
	result := v.Validate("/nonexistent/path")

	if result.IsValid() {
		t.Error("expected error for nonexistent path")
	}
}

func TestValidate_RunFlowInSubdir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subflows")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	mainFlow := `- runFlow: subflows/helper.yaml`
	helperFlow := `- tapOn: "Helper"`

	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte(mainFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "helper.yaml"), []byte(helperFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "main.yaml"))

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Only main.yaml is a test case; helper.yaml is a dependency
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case, got %d", len(result.TestCases))
	}
}

func TestValidate_RetryWithFile(t *testing.T) {
	dir := t.TempDir()

	mainFlow := `
- retry:
    maxRetries: "3"
    file: helper.yaml
`
	helperFlow := `- tapOn: "Retry helper"`

	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte(mainFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "helper.yaml"), []byte(helperFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "main.yaml"))

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Only main.yaml is a test case; helper.yaml is a dependency
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case, got %d", len(result.TestCases))
	}
}

func TestValidate_SharedDependency(t *testing.T) {
	dir := t.TempDir()

	// Both a and b reference shared
	flowA := `- runFlow: shared.yaml`
	flowB := `- runFlow: shared.yaml`
	sharedFlow := `- tapOn: "Shared"`

	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(flowA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(flowB), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shared.yaml"), []byte(sharedFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// All three are at top-level, so all are test cases
	if len(result.TestCases) != 3 {
		t.Errorf("expected 3 test cases, got %d", len(result.TestCases))
	}
}

func TestValidate_SubdirNotIncludedByDefault(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "helpers")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Top-level test case
	mainFlow := `- tapOn: "Main"`
	// Helper in subdirectory
	helperFlow := `- tapOn: "Helper"`

	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte(mainFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "helper.yaml"), []byte(helperFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Only top-level main.yaml is a test case; helpers/helper.yaml is not included
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case (top-level only), got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_ConfigYamlFlowPatterns(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "auth")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	// config.yaml specifies to include auth/* flows
	config := `flows:
  - auth/*
`
	authFlow := `- tapOn: "Login"`
	topFlow := `- tapOn: "Top"` // Should NOT be included

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "login.yaml"), []byte(authFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.yaml"), []byte(topFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Only auth/login.yaml matches the pattern
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case from auth/*, got %d: %v", len(result.TestCases), result.TestCases)
	}
	if !strings.HasSuffix(result.TestCases[0], "login.yaml") {
		t.Errorf("expected login.yaml, got %s", result.TestCases[0])
	}
}

func TestValidate_ConfigYamlExcludeTags(t *testing.T) {
	dir := t.TempDir()

	// config.yaml excludes wip tag
	config := `excludeTags:
  - wip
`
	readyFlow := `
appId: com.example
tags:
  - ready
---
- tapOn: "Ready"
`
	wipFlow := `
appId: com.example
tags:
  - wip
---
- tapOn: "WIP"
`

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ready.yaml"), []byte(readyFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wip.yaml"), []byte(wipFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Only ready.yaml (wip.yaml is excluded by config)
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case, got %d", len(result.TestCases))
	}
}

func TestValidate_RecursivePattern(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "nested", "deep")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	// config.yaml uses ** for recursive
	config := `flows:
  - "**"
`
	topFlow := `- tapOn: "Top"`
	deepFlow := `- tapOn: "Deep"`

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.yaml"), []byte(topFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "deep.yaml"), []byte(deepFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Both top.yaml and nested/deep/deep.yaml should be included
	if len(result.TestCases) != 2 {
		t.Errorf("expected 2 test cases with **, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestResult_IsValid(t *testing.T) {
	r := &Result{}
	if !r.IsValid() {
		t.Error("empty result should be valid")
	}

	r.Errors = append(r.Errors, &ValidationError{File: "test", Message: "error"})
	if r.IsValid() {
		t.Error("result with errors should not be valid")
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		File:    "test.yaml",
		Message: "something went wrong",
	}

	expected := "test.yaml: something went wrong"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestValidate_RecursivePatternWithSuffix(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "auth")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	// config.yaml uses **/ with suffix
	config := `flows:
  - "**/login*.yaml"
`
	loginFlow := `- tapOn: "Login"`
	otherFlow := `- tapOn: "Other"`

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "login_test.yaml"), []byte(loginFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "other.yaml"), []byte(otherFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Only login_test.yaml matches the pattern
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case matching **/login*.yaml, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_RepeatStep(t *testing.T) {
	dir := t.TempDir()

	flow := `
- repeat:
    times: "3"
    commands:
      - tapOn: "Button"
`
	if err := os.WriteFile(filepath.Join(dir, "repeat.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "repeat.yaml"))

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestValidate_RepeatWithRunFlow(t *testing.T) {
	dir := t.TempDir()

	mainFlow := `
- repeat:
    times: "2"
    commands:
      - runFlow: helper.yaml
`
	helperFlow := `- tapOn: "Helper"`

	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte(mainFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "helper.yaml"), []byte(helperFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "main.yaml"))

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestValidate_OnFlowStartHook(t *testing.T) {
	dir := t.TempDir()

	mainFlow := `
appId: com.example
onFlowStart:
  - runFlow: setup.yaml
---
- tapOn: "Start"
`
	setupFlow := `- launchApp: com.example`

	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte(mainFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "setup.yaml"), []byte(setupFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "main.yaml"))

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestValidate_OnFlowCompleteHook(t *testing.T) {
	dir := t.TempDir()

	mainFlow := `
appId: com.example
onFlowComplete:
  - runFlow: teardown.yaml
---
- tapOn: "Start"
`
	teardownFlow := `- stopApp: com.example`

	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte(mainFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "teardown.yaml"), []byte(teardownFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "main.yaml"))

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestValidate_NonYamlFilesIgnored(t *testing.T) {
	dir := t.TempDir()

	flow := `- tapOn: "Button"`
	readme := `# This is a README`

	if err := os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script.sh"), []byte("#!/bin/bash"), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Only test.yaml should be included
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_DependencyFirstThenTestCase(t *testing.T) {
	dir := t.TempDir()

	// a.yaml references shared.yaml, b.yaml is standalone
	// When validating directory, shared.yaml gets validated as dependency first,
	// then as a test case (since it's at top level)
	flowA := `- runFlow: shared.yaml`
	flowB := `- tapOn: "B"`
	sharedFlow := `- tapOn: "Shared"`

	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(flowA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(flowB), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shared.yaml"), []byte(sharedFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// All three should be test cases (all at top level)
	if len(result.TestCases) != 3 {
		t.Errorf("expected 3 test cases, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_ConfigYamlIncludeTags(t *testing.T) {
	dir := t.TempDir()

	config := `includeTags:
  - smoke
`
	smokeFlow := `
appId: com.example
tags:
  - smoke
---
- tapOn: "Smoke"
`
	regularFlow := `- tapOn: "Regular"`

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "smoke.yaml"), []byte(smokeFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "regular.yaml"), []byte(regularFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Only smoke.yaml has the smoke tag
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case with smoke tag, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_RunFlowWithInlineCommands(t *testing.T) {
	dir := t.TempDir()

	flow := `
- runFlow:
    commands:
      - tapOn: "Button"
      - inputText: "text"
`
	if err := os.WriteFile(filepath.Join(dir, "inline.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "inline.yaml"))

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestValidate_RetryWithInlineCommands(t *testing.T) {
	dir := t.TempDir()

	flow := `
- retry:
    maxRetries: "3"
    commands:
      - tapOn: "Flaky Button"
`
	if err := os.WriteFile(filepath.Join(dir, "retry.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "retry.yaml"))

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestValidate_SubdirPatternWithDirectory(t *testing.T) {
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// config.yaml uses subdir pattern that matches a directory containing flows
	config := `flows:
  - "auth/*"
`
	loginFlow := `- tapOn: "Login"`
	logoutFlow := `- tapOn: "Logout"`

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "login.yaml"), []byte(loginFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "logout.yaml"), []byte(logoutFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != 2 {
		t.Errorf("expected 2 test cases from auth/*, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_NestedSubdirPattern(t *testing.T) {
	dir := t.TempDir()
	nestedDir := filepath.Join(dir, "features", "auth")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// config.yaml uses nested subdir pattern
	config := `flows:
  - "features/auth/*"
`
	flow := `- tapOn: "Login"`

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "login.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_AbsoluteRunFlowPath(t *testing.T) {
	dir := t.TempDir()
	helperDir := t.TempDir() // Different directory for absolute path

	helperFlow := `- tapOn: "Helper"`
	helperPath := filepath.Join(helperDir, "helper.yaml")
	if err := os.WriteFile(helperPath, []byte(helperFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	// Main flow uses absolute path
	mainFlow := `- runFlow: ` + helperPath
	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte(mainFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(filepath.Join(dir, "main.yaml"))

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
}

func TestValidate_PatternMatchesDirectory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "flows")
	nestedDir := filepath.Join(subDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Pattern "flows/*" matches both file and directory
	config := `flows:
  - "flows/*"
`
	topFlow := `- tapOn: "Top"`
	nestedFlow := `- tapOn: "Nested"`

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "top.yaml"), []byte(topFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "nested.yaml"), []byte(nestedFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Only top.yaml, nested/ directory should be recursed into
	if len(result.TestCases) != 2 {
		t.Errorf("expected 2 test cases, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result for empty dir, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != 0 {
		t.Errorf("expected 0 test cases for empty dir, got %d", len(result.TestCases))
	}
}

func TestValidate_YmlExtension(t *testing.T) {
	dir := t.TempDir()

	flow := `- tapOn: "Button"`
	if err := os.WriteFile(filepath.Join(dir, "test.yml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case for .yml file, got %d", len(result.TestCases))
	}
}

func TestValidate_InvalidGlobPattern(t *testing.T) {
	dir := t.TempDir()

	// config.yaml with invalid glob pattern (unclosed bracket)
	config := `flows:
  - "[invalid"
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if result.IsValid() {
		t.Error("expected error for invalid glob pattern")
	}
}

func TestValidate_UnreadableDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "noperm")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	flow := `- tapOn: "Button"`
	if err := os.WriteFile(filepath.Join(subdir, "test.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	// config.yaml references the subdir
	config := `flows:
  - "noperm/*"
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	// Remove read permission on subdir (only works on Unix)
	if err := os.Chmod(subdir, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(subdir, 0o755) }()

	v := New(nil, nil)
	result := v.Validate(dir)

	// Should handle permission errors gracefully
	// Either returns error or empty results
	_ = result
}

func TestValidate_MultiplePatterns(t *testing.T) {
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")
	cartDir := filepath.Join(dir, "cart")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cartDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// config.yaml with multiple patterns
	config := `flows:
  - "auth/*"
  - "cart/*"
`
	authFlow := `- tapOn: "Login"`
	cartFlow := `- tapOn: "Add to cart"`

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "login.yaml"), []byte(authFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cartDir, "add.yaml"), []byte(cartFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != 2 {
		t.Errorf("expected 2 test cases from multiple patterns, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_DuplicatePatternMatches(t *testing.T) {
	dir := t.TempDir()

	// config.yaml with overlapping patterns
	config := `flows:
  - "*.yaml"
  - "test*.yaml"
`
	flow := `- tapOn: "Button"`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test_flow.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// Should deduplicate matches
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case (deduplicated), got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_WalkError(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "nested")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	// config.yaml with recursive pattern
	config := `flows:
  - "**"
`
	flow := `- tapOn: "Button"`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "test.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	// Remove permission on subdir to trigger walk error (Unix only)
	if err := os.Chmod(subdir, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(subdir, 0o755) }()

	v := New(nil, nil)
	result := v.Validate(dir)

	// Should handle walk errors
	_ = result
}

func TestValidate_SubdirPatternWithNestedDir(t *testing.T) {
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")
	nestedDir := filepath.Join(authDir, "helpers")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// config.yaml uses subdir pattern
	// "auth/*" matches files in auth/ AND directories, and recurses into matched directories
	config := `flows:
  - "auth/*"
`
	loginFlow := `- tapOn: "Login"`
	helperFlow := `- tapOn: "Helper"`

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "login.yaml"), []byte(loginFlow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "helper.yaml"), []byte(helperFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// auth/* matches login.yaml and helpers/ directory, which contains helper.yaml
	if len(result.TestCases) != 2 {
		t.Errorf("expected 2 test cases, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

func TestValidate_MatchPatternWithStatError(t *testing.T) {
	dir := t.TempDir()

	flow := `- tapOn: "Button"`
	flowPath := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(flowPath, []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != 1 {
		t.Errorf("expected 1 test case, got %d", len(result.TestCases))
	}
}

func TestValidate_GetTopLevelFlowsWithMixedContent(t *testing.T) {
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")
	nestedDir := filepath.Join(authDir, "subdir")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// config.yaml uses subdir/* pattern which triggers getTopLevelFlows
	// It matches files in auth/ and recurses into subdirs
	config := `flows:
  - "auth/*"
`
	flow1 := `- tapOn: "Flow1"`
	flow2 := `- tapOn: "Flow2"`
	nestedFlow := `- tapOn: "Nested"`
	readme := `# README` // Non-yaml file, should be skipped

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "flow1.yaml"), []byte(flow1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "flow2.yml"), []byte(flow2), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "nested.yaml"), []byte(nestedFlow), 0o644); err != nil {
		t.Fatal(err)
	}

	v := New(nil, nil)
	result := v.Validate(dir)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	// flow1.yaml, flow2.yml from auth/, and nested.yaml from subdir (via getTopLevelFlows)
	// README.md is skipped (not yaml)
	if len(result.TestCases) != 3 {
		t.Errorf("expected 3 test cases, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

// writeFlow is a shared helper for the recursive-pattern tests below.
// Creates any missing parent directories and writes a minimal-but-valid flow.
func writeFlow(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`- tapOn: "X"`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestValidate_RecursivePatternWithPrefix verifies that "**" is treated as
// recursive when it appears after a directory prefix, e.g. "tests/**/*.yaml".
// Previously only patterns starting with "**" or "**/" were expanded
// recursively; anything with a prefix fell through to filepath.Glob, which
// treats "**" like a single "*" and therefore matched only one directory
// level, silently skipping deeper flows.
func TestValidate_RecursivePatternWithPrefix(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		"tests/a.yaml",
		"tests/sub/b.yaml",
		"tests/sub/deep/c.yaml",
		"tests/sub/deep/more/d.yaml",
	}
	for _, p := range files {
		writeFlow(t, filepath.Join(dir, p))
	}
	// Sibling directory that must NOT be picked up.
	writeFlow(t, filepath.Join(dir, "other", "skip.yaml"))

	config := `flows:
  - "tests/**/*.yaml"
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	result := New(nil, nil).Validate(dir)

	if !result.IsValid() {
		t.Fatalf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != len(files) {
		t.Errorf("expected %d test cases from tests/**/*.yaml, got %d: %v",
			len(files), len(result.TestCases), result.TestCases)
	}
	for _, tc := range result.TestCases {
		if strings.Contains(filepath.ToSlash(tc), "/other/") {
			t.Errorf("test case leaked from sibling dir: %s", tc)
		}
	}
}

// TestValidate_RecursivePatternPrefixOnly verifies that "prefix/**" matches
// every flow file underneath the prefix at any depth, without requiring a
// filename suffix in the pattern.
func TestValidate_RecursivePatternPrefixOnly(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		"flows/a.yaml",
		"flows/sub/b.yaml",
		"flows/sub/deep/c.yaml",
	}
	for _, p := range files {
		writeFlow(t, filepath.Join(dir, p))
	}

	config := `flows:
  - "flows/**"
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	result := New(nil, nil).Validate(dir)

	if !result.IsValid() {
		t.Fatalf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != len(files) {
		t.Errorf("expected %d test cases from flows/**, got %d: %v",
			len(files), len(result.TestCases), result.TestCases)
	}
}

// TestValidate_RecursivePatternMultiSegmentSuffix verifies that a pattern
// like "a/**/c/*.yaml" walks recursively from dir/a and then matches only
// files whose trailing path is "c/<name>.yaml".
func TestValidate_RecursivePatternMultiSegmentSuffix(t *testing.T) {
	dir := t.TempDir()

	included := []string{
		"a/c/one.yaml",
		"a/x/c/two.yaml",
		"a/x/y/c/three.yaml",
	}
	excluded := []string{
		"a/d/one.yaml",
		"a/x/y/four.yaml",
	}
	for _, p := range append(included, excluded...) {
		writeFlow(t, filepath.Join(dir, p))
	}

	config := `flows:
  - "a/**/c/*.yaml"
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	result := New(nil, nil).Validate(dir)

	if !result.IsValid() {
		t.Fatalf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != len(included) {
		t.Errorf("expected %d test cases matching a/**/c/*.yaml, got %d: %v",
			len(included), len(result.TestCases), result.TestCases)
	}
	for _, tc := range result.TestCases {
		rel := filepath.ToSlash(tc)
		if !strings.Contains(rel, "/c/") {
			t.Errorf("unexpected match outside c/ suffix: %s", tc)
		}
	}
}

// TestValidate_RecursivePatternMissingPrefixDir verifies that a pattern whose
// prefix directory does not exist (e.g. "subflows/**/*.yaml" with no
// subflows/) is a silent no-match rather than a walk error.
func TestValidate_RecursivePatternMissingPrefixDir(t *testing.T) {
	dir := t.TempDir()

	// Only a top-level flow that should NOT match the prefixed pattern.
	writeFlow(t, filepath.Join(dir, "top.yaml"))

	config := `flows:
  - "subflows/**/*.yaml"
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	result := New(nil, nil).Validate(dir)

	if !result.IsValid() {
		t.Fatalf("expected valid result for missing prefix dir, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != 0 {
		t.Errorf("expected 0 test cases, got %d: %v", len(result.TestCases), result.TestCases)
	}
}

// TestValidate_RecursivePatternWildcardPrefix verifies that a shell-wildcard
// in the prefix (before "**") is expanded via filepath.Glob, so
// "flows-*/**/*.yaml" walks each matching directory.
func TestValidate_RecursivePatternWildcardPrefix(t *testing.T) {
	dir := t.TempDir()

	included := []string{
		"flows-alpha/a.yaml",
		"flows-alpha/sub/b.yaml",
		"flows-beta/c.yaml",
	}
	// Must NOT match — prefix wildcard "flows-*" doesn't include "other-dir".
	excluded := []string{
		"other-dir/skip.yaml",
	}
	for _, p := range append(included, excluded...) {
		writeFlow(t, filepath.Join(dir, p))
	}

	config := `flows:
  - "flows-*/**/*.yaml"
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	result := New(nil, nil).Validate(dir)

	if !result.IsValid() {
		t.Fatalf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.TestCases) != len(included) {
		t.Errorf("expected %d test cases matching flows-*/**/*.yaml, got %d: %v",
			len(included), len(result.TestCases), result.TestCases)
	}
	for _, tc := range result.TestCases {
		if strings.Contains(filepath.ToSlash(tc), "/other-dir/") {
			t.Errorf("test case leaked from sibling dir: %s", tc)
		}
	}
}
