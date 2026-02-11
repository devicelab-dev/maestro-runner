package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_SimpleFlow(t *testing.T) {
	yaml := `
- tapOn: "Login"
- inputText: "username"
- tapOn:
    id: submit-btn
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(flow.Steps))
	}

	// Check first step
	tap, ok := flow.Steps[0].(*TapOnStep)
	if !ok {
		t.Fatalf("expected TapOnStep, got %T", flow.Steps[0])
	}
	if tap.Selector.Text != "Login" {
		t.Errorf("expected text=Login, got %q", tap.Selector.Text)
	}

	// Check second step
	input, ok := flow.Steps[1].(*InputTextStep)
	if !ok {
		t.Fatalf("expected InputTextStep, got %T", flow.Steps[1])
	}
	if input.Text != "username" {
		t.Errorf("expected text=username, got %q", input.Text)
	}

	// Check third step
	tap2, ok := flow.Steps[2].(*TapOnStep)
	if !ok {
		t.Fatalf("expected TapOnStep, got %T", flow.Steps[2])
	}
	if tap2.Selector.ID != "submit-btn" {
		t.Errorf("expected id=submit-btn, got %q", tap2.Selector.ID)
	}
}

func TestParse_WithConfig(t *testing.T) {
	yaml := `
appId: com.example.app
name: Login Test
tags:
  - smoke
  - login
env:
  USERNAME: testuser
timeout: 30000
---
- launchApp: com.example.app
- tapOn: "Login"
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if flow.Config.AppID != "com.example.app" {
		t.Errorf("expected appId=com.example.app, got %q", flow.Config.AppID)
	}
	if flow.Config.Name != "Login Test" {
		t.Errorf("expected name=Login Test, got %q", flow.Config.Name)
	}
	if len(flow.Config.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(flow.Config.Tags))
	}
	if flow.Config.Env["USERNAME"] != "testuser" {
		t.Errorf("expected env.USERNAME=testuser, got %q", flow.Config.Env["USERNAME"])
	}
	if flow.Config.Timeout != 30000 {
		t.Errorf("expected timeout=30000, got %d", flow.Config.Timeout)
	}
	if len(flow.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(flow.Steps))
	}
}

func TestParse_AllStepTypes(t *testing.T) {
	testCases := []struct {
		name     string
		yaml     string
		stepType StepType
	}{
		{"tapOn scalar", `- tapOn: "Login"`, StepTapOn},
		{"tapOn mapping", `- tapOn: {id: btn}`, StepTapOn},
		{"doubleTapOn", `- doubleTapOn: "Button"`, StepDoubleTapOn},
		{"longPressOn", `- longPressOn: "Button"`, StepLongPressOn},
		{"tapOnPoint", `- tapOnPoint: {x: 100, y: 200}`, StepTapOnPoint},
		{"swipe scalar", `- swipe: UP`, StepSwipe},
		{"swipe mapping", `- swipe: {direction: DOWN}`, StepSwipe},
		{"scroll", `- scroll: DOWN`, StepScroll},
		{"scrollUntilVisible", `- scrollUntilVisible: "End"`, StepScrollUntilVisible},
		{"back", `- back:`, StepBack},
		{"hideKeyboard", `- hideKeyboard:`, StepHideKeyboard},
		{"acceptAlert", `- acceptAlert:`, StepAcceptAlert},
		{"dismissAlert", `- dismissAlert:`, StepDismissAlert},
		{"inputText scalar", `- inputText: "hello"`, StepInputText},
		{"inputText mapping", `- inputText: {text: hello}`, StepInputText},
		{"inputRandom", `- inputRandom: EMAIL`, StepInputRandom},
		{"inputRandomEmail", `- inputRandomEmail`, StepInputRandom},
		{"inputRandomNumber", `- inputRandomNumber`, StepInputRandom},
		{"inputRandomPersonName", `- inputRandomPersonName`, StepInputRandom},
		{"inputRandomText", `- inputRandomText`, StepInputRandom},
		{"eraseText scalar", `- eraseText: 5`, StepEraseText},
		{"eraseText mapping", `- eraseText: {characters: 10}`, StepEraseText},
		{"copyTextFrom", `- copyTextFrom: "Label"`, StepCopyTextFrom},
		{"pasteText", `- pasteText:`, StepPasteText},
		{"assertVisible", `- assertVisible: "Success"`, StepAssertVisible},
		{"assertNotVisible", `- assertNotVisible: "Error"`, StepAssertNotVisible},
		{"assertTrue", `- assertTrue: "1 === 1"`, StepAssertTrue},
		{"assertCondition", `- assertCondition: {scriptCondition: "x > 0"}`, StepAssertCondition},
		{"assertNoDefectsWithAI", `- assertNoDefectsWithAI: {}`, StepAssertNoDefectsWithAI},
		{"assertWithAI", `- assertWithAI: "Button visible"`, StepAssertWithAI},
		{"extractTextWithAI", `- extractTextWithAI: {query: "price", variable: p}`, StepExtractTextWithAI},
		{"extendedWaitUntil", `- extendedWaitUntil: {visible: {text: "Ready"}}`, StepWaitUntil},
		{"launchApp scalar", `- launchApp: com.example.app`, StepLaunchApp},
		{"launchApp mapping", `- launchApp: {appId: com.app}`, StepLaunchApp},
		{"stopApp", `- stopApp: com.example.app`, StepStopApp},
		{"killApp", `- killApp: com.example.app`, StepKillApp},
		{"clearState", `- clearState: com.example.app`, StepClearState},
		{"clearKeychain", `- clearKeychain:`, StepClearKeychain},
		{"setLocation", `- setLocation: {latitude: "37.7", longitude: "-122.4"}`, StepSetLocation},
		{"setOrientation scalar", `- setOrientation: LANDSCAPE`, StepSetOrientation},
		{"setOrientation mapping", `- setOrientation: {orientation: PORTRAIT}`, StepSetOrientation},
		{"setAirplaneMode", `- setAirplaneMode: {enabled: true}`, StepSetAirplaneMode},
		{"toggleAirplaneMode", `- toggleAirplaneMode:`, StepToggleAirplaneMode},
		{"travel", `- travel: {points: ["0,0"], speed: 50}`, StepTravel},
		{"openLink scalar", `- openLink: "https://example.com"`, StepOpenLink},
		{"openLink mapping", `- openLink: {link: "https://example.com"}`, StepOpenLink},
		{"openBrowser scalar", `- openBrowser: "https://example.com"`, StepOpenBrowser},
		{"openBrowser mapping", `- openBrowser: {url: "https://example.com"}`, StepOpenBrowser},
		{"runScript scalar", `- runScript: "console.log('hi')"`, StepRunScript},
		{"runScript mapping", `- runScript: {script: "x=1"}`, StepRunScript},
		{"evalScript", `- evalScript: "output.result = 42"`, StepEvalScript},
		{"takeScreenshot", `- takeScreenshot: "screen.png"`, StepTakeScreenshot},
		{"startRecording", `- startRecording: "video.mp4"`, StepStartRecording},
		{"stopRecording", `- stopRecording: "video.mp4"`, StepStopRecording},
		{"addMedia", `- addMedia: {files: ["img.png"]}`, StepAddMedia},
		{"pressKey", `- pressKey: ENTER`, StepPressKey},
		{"waitForAnimationToEnd", `- waitForAnimationToEnd: {}`, StepWaitForAnimationToEnd},
		{"defineVariables", `- defineVariables: {VAR1: value1}`, StepDefineVariables},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			flow, err := Parse([]byte(tc.yaml), "test.yaml")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(flow.Steps) != 1 {
				t.Fatalf("expected 1 step, got %d", len(flow.Steps))
			}
			if flow.Steps[0].Type() != tc.stepType {
				t.Errorf("expected type %v, got %v", tc.stepType, flow.Steps[0].Type())
			}
		})
	}
}

func TestParse_RepeatStep(t *testing.T) {
	yaml := `
- repeat:
    times: "3"
    commands:
      - tapOn: "Next"
      - swipe: LEFT
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	repeat, ok := flow.Steps[0].(*RepeatStep)
	if !ok {
		t.Fatalf("expected RepeatStep, got %T", flow.Steps[0])
	}
	if repeat.Times != "3" {
		t.Errorf("expected times=3, got %q", repeat.Times)
	}
	if len(repeat.Steps) != 2 {
		t.Errorf("expected 2 nested steps, got %d", len(repeat.Steps))
	}
}

func TestParse_RepeatWithWhile(t *testing.T) {
	yaml := `
- repeat:
    while:
      visible:
        text: "More"
    commands:
      - tapOn: "Load More"
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	repeat, ok := flow.Steps[0].(*RepeatStep)
	if !ok {
		t.Fatalf("expected RepeatStep, got %T", flow.Steps[0])
	}
	if repeat.While.Visible == nil {
		t.Error("expected while.visible to be set")
	}
	if repeat.While.Visible.Text != "More" {
		t.Errorf("expected while.visible.text=More, got %q", repeat.While.Visible.Text)
	}
}

func TestParse_RetryStep(t *testing.T) {
	yaml := `
- retry:
    maxRetries: "3"
    commands:
      - tapOn: "Submit"
      - assertVisible: "Success"
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retry, ok := flow.Steps[0].(*RetryStep)
	if !ok {
		t.Fatalf("expected RetryStep, got %T", flow.Steps[0])
	}
	if retry.MaxRetries != "3" {
		t.Errorf("expected maxRetries=3, got %q", retry.MaxRetries)
	}
	if len(retry.Steps) != 2 {
		t.Errorf("expected 2 nested steps, got %d", len(retry.Steps))
	}
}

func TestParse_RetryWithFile(t *testing.T) {
	yaml := `
- retry:
    maxRetries: "2"
    file: "subflow.yaml"
    env:
      MODE: test
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retry, ok := flow.Steps[0].(*RetryStep)
	if !ok {
		t.Fatalf("expected RetryStep, got %T", flow.Steps[0])
	}
	if retry.File != "subflow.yaml" {
		t.Errorf("expected file=subflow.yaml, got %q", retry.File)
	}
	if retry.Env["MODE"] != "test" {
		t.Errorf("expected env.MODE=test, got %q", retry.Env["MODE"])
	}
}

func TestParse_InputRandomShorthands(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		expectedType string
	}{
		{"inputRandomEmail", "- inputRandomEmail", "EMAIL"},
		{"inputRandomNumber", "- inputRandomNumber", "NUMBER"},
		{"inputRandomPersonName", "- inputRandomPersonName", "PERSON_NAME"},
		{"inputRandomText", "- inputRandomText", "TEXT"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flow, err := Parse([]byte(tc.yaml), "test.yaml")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(flow.Steps) != 1 {
				t.Fatalf("expected 1 step, got %d", len(flow.Steps))
			}
			step, ok := flow.Steps[0].(*InputRandomStep)
			if !ok {
				t.Fatalf("expected InputRandomStep, got %T", flow.Steps[0])
			}
			if step.DataType != tc.expectedType {
				t.Errorf("expected DataType=%s, got %s", tc.expectedType, step.DataType)
			}
		})
	}
}

func TestParse_RunFlowScalar(t *testing.T) {
	yaml := `- runFlow: "login.yaml"`

	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runFlow, ok := flow.Steps[0].(*RunFlowStep)
	if !ok {
		t.Fatalf("expected RunFlowStep, got %T", flow.Steps[0])
	}
	if runFlow.File != "login.yaml" {
		t.Errorf("expected file=login.yaml, got %q", runFlow.File)
	}
}

func TestParse_RunFlowWithInlineSteps(t *testing.T) {
	yaml := `
- runFlow:
    commands:
      - tapOn: "Login"
      - inputText: "user"
    when:
      visible:
        text: "Welcome"
    env:
      MODE: test
    optional: true
    label: "login flow"
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runFlow, ok := flow.Steps[0].(*RunFlowStep)
	if !ok {
		t.Fatalf("expected RunFlowStep, got %T", flow.Steps[0])
	}
	if len(runFlow.Steps) != 2 {
		t.Errorf("expected 2 inline steps, got %d", len(runFlow.Steps))
	}
	if runFlow.When == nil || runFlow.When.Visible == nil {
		t.Error("expected when.visible to be set")
	}
	if runFlow.Env["MODE"] != "test" {
		t.Errorf("expected env.MODE=test, got %q", runFlow.Env["MODE"])
	}
	if !runFlow.Optional {
		t.Error("expected optional=true")
	}
	if runFlow.StepLabel != "login flow" {
		t.Errorf("expected label=login flow, got %q", runFlow.StepLabel)
	}
}

func TestParse_NestedRepeat(t *testing.T) {
	yaml := `
- repeat:
    times: "2"
    commands:
      - repeat:
          times: "3"
          commands:
            - tapOn: "Item"
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outer, ok := flow.Steps[0].(*RepeatStep)
	if !ok {
		t.Fatalf("expected RepeatStep, got %T", flow.Steps[0])
	}

	inner, ok := outer.Steps[0].(*RepeatStep)
	if !ok {
		t.Fatalf("expected nested RepeatStep, got %T", outer.Steps[0])
	}
	if inner.Times != "3" {
		t.Errorf("expected inner times=3, got %q", inner.Times)
	}
}

func TestParse_DefineVariables(t *testing.T) {
	yaml := `
- defineVariables:
    USERNAME: testuser
    PASSWORD: secret123
    COUNT: "5"
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defVars, ok := flow.Steps[0].(*DefineVariablesStep)
	if !ok {
		t.Fatalf("expected DefineVariablesStep, got %T", flow.Steps[0])
	}
	if defVars.Env["USERNAME"] != "testuser" {
		t.Errorf("expected USERNAME=testuser, got %q", defVars.Env["USERNAME"])
	}
	if defVars.Env["PASSWORD"] != "secret123" {
		t.Errorf("expected PASSWORD=secret123, got %q", defVars.Env["PASSWORD"])
	}
	if defVars.Env["COUNT"] != "5" {
		t.Errorf("expected COUNT=5, got %q", defVars.Env["COUNT"])
	}
}

func TestParse_EmptyFlow(t *testing.T) {
	yaml := ""
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for empty flow")
	}
	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if parseErr.Message != "empty flow file" {
		t.Errorf("expected 'empty flow file' error, got %q", parseErr.Message)
	}
}

func TestParse_InvalidStep(t *testing.T) {
	yaml := `- notAValidStep: value`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid step")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	yaml := `
- tapOn: [invalid
  yaml: structure
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParse_StepNotMapping(t *testing.T) {
	yaml := `- "just a string"`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for non-mapping step")
	}
	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected ParseError, got %T", err)
	}
	// Unknown scalar step names should give "unknown step type" error
	if !strings.Contains(parseErr.Message, "unknown step type") {
		t.Errorf("expected 'unknown step type' error, got %q", parseErr.Message)
	}
}

func TestParse_ScalarStep(t *testing.T) {
	// Steps without colon (like Maestro allows)
	yaml := `- waitForAnimationToEnd`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flow.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(flow.Steps))
	}
	if flow.Steps[0].Type() != StepWaitForAnimationToEnd {
		t.Errorf("expected waitForAnimationToEnd, got %s", flow.Steps[0].Type())
	}
}

func TestParse_MultilineScript(t *testing.T) {
	yaml := `
- runScript: |
    function test() {
      return true;
    }
    ---
    test();
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	script, ok := flow.Steps[0].(*RunScriptStep)
	if !ok {
		t.Fatalf("expected RunScriptStep, got %T", flow.Steps[0])
	}
	// The script should contain "---" as part of the multiline content
	if script.Script == "" {
		t.Error("expected non-empty script")
	}
}

func TestParseError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ParseError
		expected string
	}{
		{
			name:     "with line number",
			err:      ParseError{Path: "test.yaml", Line: 10, Message: "invalid syntax"},
			expected: "test.yaml:10: invalid syntax",
		},
		{
			name:     "without line number",
			err:      ParseError{Path: "test.yaml", Line: 0, Message: "file error"},
			expected: "test.yaml: file error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.expected {
				t.Errorf("Error()=%q, want %q", got, tc.expected)
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	// Create a temp file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	content := `- tapOn: "Login"`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	flow, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(flow.Steps))
	}
	if flow.SourcePath != path {
		t.Errorf("expected sourcePath=%q, got %q", path, flow.SourcePath)
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/flow.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	files := map[string]string{
		"flow1.yaml": `
appId: com.app1
tags:
  - smoke
---
- tapOn: "Login"
`,
		"flow2.yml": `
appId: com.app2
tags:
  - regression
---
- tapOn: "Register"
`,
		"not-yaml.txt": `This is not YAML`,
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	// Test without tag filters
	flows, err := ParseDirectory(dir, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flows) != 2 {
		t.Errorf("expected 2 flows, got %d", len(flows))
	}

	// Test with include tags
	flows, err = ParseDirectory(dir, []string{"smoke"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flows) != 1 {
		t.Errorf("expected 1 flow with smoke tag, got %d", len(flows))
	}

	// Test with exclude tags
	flows, err = ParseDirectory(dir, nil, []string{"regression"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flows) != 1 {
		t.Errorf("expected 1 flow without regression tag, got %d", len(flows))
	}
}

func TestParseDirectory_WithSubdirs(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subflows")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	content := `- tapOn: "Button"`
	if err := os.WriteFile(filepath.Join(dir, "main.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "sub.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	flows, err := ParseDirectory(dir, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flows) != 2 {
		t.Errorf("expected 2 flows (including subdir), got %d", len(flows))
	}
}

func TestParse_TapOnWithAllFields(t *testing.T) {
	yaml := `
- tapOn:
    text: "Submit"
    id: submit-btn
    enabled: true
    index: "2"
    below:
      id: header
    longPress: true
    repeat: 3
    delay: 100
    point: "50%, 50%"
    retryTapIfNoChange: true
    waitUntilVisible: true
    waitToSettleTimeoutMs: 500
    optional: true
    label: "submit button"
    timeout: 10000
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tap, ok := flow.Steps[0].(*TapOnStep)
	if !ok {
		t.Fatalf("expected TapOnStep, got %T", flow.Steps[0])
	}

	if tap.Selector.Text != "Submit" {
		t.Errorf("Selector.Text=%q, want Submit", tap.Selector.Text)
	}
	if tap.Selector.ID != "submit-btn" {
		t.Errorf("Selector.ID=%q, want submit-btn", tap.Selector.ID)
	}
	if tap.Selector.Enabled == nil || !*tap.Selector.Enabled {
		t.Error("expected Selector.Enabled=true")
	}
	if tap.Selector.Index != "2" {
		t.Errorf("Selector.Index=%q, want 2", tap.Selector.Index)
	}
	if tap.Selector.Below == nil || tap.Selector.Below.ID != "header" {
		t.Error("expected Selector.Below.ID=header")
	}
	if !tap.LongPress {
		t.Error("expected LongPress=true")
	}
	if tap.Repeat != 3 {
		t.Errorf("Repeat=%d, want 3", tap.Repeat)
	}
	if tap.DelayMs != 100 {
		t.Errorf("DelayMs=%d, want 100", tap.DelayMs)
	}
	if tap.Point != "50%, 50%" {
		t.Errorf("Point=%q, want 50%%, 50%%", tap.Point)
	}
	if tap.RetryTapIfNoChange == nil || !*tap.RetryTapIfNoChange {
		t.Error("expected RetryTapIfNoChange=true")
	}
	if tap.WaitUntilVisible == nil || !*tap.WaitUntilVisible {
		t.Error("expected WaitUntilVisible=true")
	}
	if tap.WaitToSettleTimeoutMs != 500 {
		t.Errorf("WaitToSettleTimeoutMs=%d, want 500", tap.WaitToSettleTimeoutMs)
	}
	if !tap.Optional {
		t.Error("expected Optional=true")
	}
	if tap.StepLabel != "submit button" {
		t.Errorf("StepLabel=%q, want submit button", tap.StepLabel)
	}
	if tap.TimeoutMs != 10000 {
		t.Errorf("TimeoutMs=%d, want 10000", tap.TimeoutMs)
	}
}

func TestParse_SwipeWithAllFields(t *testing.T) {
	yaml := `
- swipe:
    direction: UP
    start: "50%, 80%"
    end: "50%, 20%"
    startX: 100
    startY: 500
    endX: 100
    endY: 200
    duration: 300
    speed: 50
    waitToSettleTimeoutMs: 200
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	swipe, ok := flow.Steps[0].(*SwipeStep)
	if !ok {
		t.Fatalf("expected SwipeStep, got %T", flow.Steps[0])
	}

	if swipe.Direction != "UP" {
		t.Errorf("Direction=%q, want UP", swipe.Direction)
	}
	if swipe.Start != "50%, 80%" {
		t.Errorf("Start=%q, want 50%%, 80%%", swipe.Start)
	}
	if swipe.End != "50%, 20%" {
		t.Errorf("End=%q, want 50%%, 20%%", swipe.End)
	}
	if swipe.StartX != 100 || swipe.StartY != 500 {
		t.Errorf("StartX,StartY=%d,%d, want 100,500", swipe.StartX, swipe.StartY)
	}
	if swipe.EndX != 100 || swipe.EndY != 200 {
		t.Errorf("EndX,EndY=%d,%d, want 100,200", swipe.EndX, swipe.EndY)
	}
	if swipe.Duration != 300 {
		t.Errorf("Duration=%d, want 300", swipe.Duration)
	}
	if swipe.Speed != 50 {
		t.Errorf("Speed=%d, want 50", swipe.Speed)
	}
	if swipe.WaitToSettleTimeoutMs != 200 {
		t.Errorf("WaitToSettleTimeoutMs=%d, want 200", swipe.WaitToSettleTimeoutMs)
	}
}

func TestParse_ScrollUntilVisibleWithAllFields(t *testing.T) {
	yaml := `
- scrollUntilVisible:
    element:
      text: "End of list"
    direction: DOWN
    timeout: 30000
    maxScrolls: 20
    speed: 40
    visibilityPercentage: 80
    centerElement: true
    waitToSettleTimeoutMs: 100
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scroll, ok := flow.Steps[0].(*ScrollUntilVisibleStep)
	if !ok {
		t.Fatalf("expected ScrollUntilVisibleStep, got %T", flow.Steps[0])
	}

	if scroll.Element.Text != "End of list" {
		t.Errorf("Element.Text=%q, want End of list", scroll.Element.Text)
	}
	if scroll.Direction != "DOWN" {
		t.Errorf("Direction=%q, want DOWN", scroll.Direction)
	}
	if scroll.BaseStep.TimeoutMs != 30000 {
		t.Errorf("TimeoutMs=%d, want 30000", scroll.BaseStep.TimeoutMs)
	}
	if scroll.MaxScrolls != 20 {
		t.Errorf("MaxScrolls=%d, want 20", scroll.MaxScrolls)
	}
	if scroll.Speed != 40 {
		t.Errorf("Speed=%d, want 40", scroll.Speed)
	}
	if scroll.VisibilityPercentage != 80 {
		t.Errorf("VisibilityPercentage=%d, want 80", scroll.VisibilityPercentage)
	}
	if !scroll.CenterElement {
		t.Error("expected CenterElement=true")
	}
	if scroll.WaitToSettleTimeoutMs != 100 {
		t.Errorf("WaitToSettleTimeoutMs=%d, want 100", scroll.WaitToSettleTimeoutMs)
	}
}

func TestParse_WaitUntilStep(t *testing.T) {
	yaml := `
- extendedWaitUntil:
    visible:
      text: "Ready"
    notVisible:
      id: loading
    timeout: 10000
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wait, ok := flow.Steps[0].(*WaitUntilStep)
	if !ok {
		t.Fatalf("expected WaitUntilStep, got %T", flow.Steps[0])
	}

	if wait.Visible == nil || wait.Visible.Text != "Ready" {
		t.Error("expected Visible.Text=Ready")
	}
	if wait.NotVisible == nil || wait.NotVisible.ID != "loading" {
		t.Error("expected NotVisible.ID=loading")
	}
}

func TestParse_AssertConditionStep(t *testing.T) {
	yaml := `
- assertCondition:
    visible:
      text: "Success"
    notVisible:
      text: "Error"
    scriptCondition: "result === true"
    platform: Android
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assert, ok := flow.Steps[0].(*AssertConditionStep)
	if !ok {
		t.Fatalf("expected AssertConditionStep, got %T", flow.Steps[0])
	}

	if assert.Condition.Visible == nil || assert.Condition.Visible.Text != "Success" {
		t.Error("expected Condition.Visible.Text=Success")
	}
	if assert.Condition.NotVisible == nil || assert.Condition.NotVisible.Text != "Error" {
		t.Error("expected Condition.NotVisible.Text=Error")
	}
	if assert.Condition.Script != "result === true" {
		t.Errorf("Condition.Script=%q, want result === true", assert.Condition.Script)
	}
	if assert.Condition.Platform != "Android" {
		t.Errorf("Condition.Platform=%q, want Android", assert.Condition.Platform)
	}
}

func TestIsStepType(t *testing.T) {
	validTypes := []string{
		"tapOn", "doubleTapOn", "longPressOn", "tapOnPoint", "swipe", "scroll",
		"scrollUntilVisible", "back", "hideKeyboard", "acceptAlert", "dismissAlert",
		"inputText", "inputRandom", "inputRandomEmail", "inputRandomNumber",
		"inputRandomPersonName", "inputRandomText",
		"eraseText", "copyTextFrom", "pasteText", "setClipboard", "assertVisible",
		"assertNotVisible", "assertTrue", "assertCondition", "assertNoDefectsWithAI",
		"assertWithAI", "extractTextWithAI", "extendedWaitUntil", "launchApp",
		"stopApp", "killApp", "clearState", "clearKeychain", "setPermissions",
		"setLocation", "setOrientation", "setAirplaneMode", "toggleAirplaneMode",
		"travel", "openLink", "openBrowser", "repeat", "retry", "runFlow",
		"runScript", "evalScript", "takeScreenshot", "startRecording", "stopRecording",
		"addMedia", "pressKey", "waitForAnimationToEnd", "defineVariables",
	}

	for _, st := range validTypes {
		if !isStepType(st) {
			t.Errorf("isStepType(%q)=false, want true", st)
		}
	}

	invalidTypes := []string{"invalidStep", "unknown", "foo", ""}
	for _, st := range invalidTypes {
		if isStepType(st) {
			t.Errorf("isStepType(%q)=true, want false", st)
		}
	}
}

func TestShouldIncludeFlow(t *testing.T) {
	tests := []struct {
		name        string
		flowTags    []string
		includeTags []string
		excludeTags []string
		expected    bool
	}{
		{
			name:        "no filters",
			flowTags:    []string{"smoke", "login"},
			includeTags: nil,
			excludeTags: nil,
			expected:    true,
		},
		{
			name:        "include match",
			flowTags:    []string{"smoke", "login"},
			includeTags: []string{"smoke"},
			excludeTags: nil,
			expected:    true,
		},
		{
			name:        "include no match",
			flowTags:    []string{"regression"},
			includeTags: []string{"smoke"},
			excludeTags: nil,
			expected:    false,
		},
		{
			name:        "exclude match",
			flowTags:    []string{"smoke", "slow"},
			includeTags: nil,
			excludeTags: []string{"slow"},
			expected:    false,
		},
		{
			name:        "exclude no match",
			flowTags:    []string{"smoke"},
			includeTags: nil,
			excludeTags: []string{"slow"},
			expected:    true,
		},
		{
			name:        "include and exclude",
			flowTags:    []string{"smoke", "slow"},
			includeTags: []string{"smoke"},
			excludeTags: []string{"slow"},
			expected:    false,
		},
		{
			name:        "empty flow tags with include filter",
			flowTags:    []string{},
			includeTags: []string{"smoke"},
			excludeTags: nil,
			expected:    false,
		},
		{
			name:        "empty flow tags no filter",
			flowTags:    []string{},
			includeTags: nil,
			excludeTags: nil,
			expected:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flow := &Flow{Config: Config{Tags: tc.flowTags}}
			got := ShouldIncludeFlow(flow, tc.includeTags, tc.excludeTags)
			if got != tc.expected {
				t.Errorf("ShouldIncludeFlow()=%v, want %v", got, tc.expected)
			}
		})
	}
}

func TestSplitYAMLDocuments(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{
			name:     "single document",
			content:  "- tapOn: Login",
			expected: 1,
		},
		{
			name:     "two documents",
			content:  "appId: com.app\n---\n- tapOn: Login",
			expected: 2,
		},
		{
			name: "multiline script with ---",
			content: `- runScript: |
    console.log("---")
    test()
`,
			expected: 1,
		},
		{
			name:     "empty content",
			content:  "",
			expected: 0,
		},
		{
			name:     "whitespace only",
			content:  "   \n  \n  ",
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := splitYAMLDocuments(tc.content)
			if len(parts) != tc.expected {
				t.Errorf("splitYAMLDocuments() returned %d parts, want %d", len(parts), tc.expected)
			}
		})
	}
}

// Test error paths for coverage

func TestParse_DecodeErrors(t *testing.T) {
	// Test decode errors for various step types
	testCases := []struct {
		name string
		yaml string
	}{
		{"tapOn invalid", `- tapOn: {text: [invalid]}`},
		{"doubleTapOn invalid", `- doubleTapOn: {id: [invalid]}`},
		{"longPressOn invalid", `- longPressOn: {id: [invalid]}`},
		{"tapOnPoint invalid", `- tapOnPoint: {x: "not a number"}`},
		{"swipe invalid", `- swipe: {direction: [invalid]}`},
		{"scroll invalid", `- scroll: {direction: [invalid]}`},
		{"scrollUntilVisible invalid", `- scrollUntilVisible: {element: [invalid]}`},
		{"inputText invalid", `- inputText: {text: [invalid]}`},
		{"inputRandom invalid", `- inputRandom: {type: [invalid]}`},
		{"eraseText invalid", `- eraseText: {characters: "not a number"}`},
		{"copyTextFrom invalid", `- copyTextFrom: {text: [invalid]}`},
		{"assertVisible invalid", `- assertVisible: {text: [invalid]}`},
		{"assertNotVisible invalid", `- assertNotVisible: {text: [invalid]}`},
		{"assertTrue invalid", `- assertTrue: {condition: [invalid]}`},
		{"assertCondition invalid", `- assertCondition: {visible: {text: [invalid]}}`},
		{"assertNoDefectsWithAI invalid", `- assertNoDefectsWithAI: [invalid]`},
		{"assertWithAI invalid", `- assertWithAI: {assertion: [invalid]}`},
		{"extractTextWithAI invalid", `- extractTextWithAI: {query: [invalid]}`},
		{"extendedWaitUntil invalid", `- extendedWaitUntil: {visible: {text: [invalid]}}`},
		{"launchApp invalid", `- launchApp: {appId: [invalid]}`},
		{"stopApp invalid", `- stopApp: {appId: [invalid]}`},
		{"killApp invalid", `- killApp: {appId: [invalid]}`},
		{"clearState invalid", `- clearState: {appId: [invalid]}`},
		{"setLocation invalid", `- setLocation: {latitude: [invalid]}`},
		{"setOrientation invalid", `- setOrientation: {orientation: [invalid]}`},
		{"setAirplaneMode invalid", `- setAirplaneMode: {enabled: "not a bool"}`},
		{"travel invalid", `- travel: {points: "not an array"}`},
		{"openLink invalid", `- openLink: {link: [invalid]}`},
		{"runScript invalid", `- runScript: {script: [invalid]}`},
		{"evalScript invalid", `- evalScript: {script: [invalid]}`},
		{"takeScreenshot invalid", `- takeScreenshot: {path: [invalid]}`},
		{"startRecording invalid", `- startRecording: {path: [invalid]}`},
		{"stopRecording invalid", `- stopRecording: {path: [invalid]}`},
		{"addMedia invalid", `- addMedia: {files: "not an array"}`},
		{"pressKey invalid", `- pressKey: {key: [invalid]}`},
		{"waitForAnimationToEnd invalid", `- waitForAnimationToEnd: [invalid]`},
		// defineVariables doesn't error on invalid input - it manually processes the map
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml), "test.yaml")
			if err == nil {
				t.Error("expected error for invalid YAML")
			}
		})
	}
}

func TestParse_RepeatStepDecodeError(t *testing.T) {
	yaml := `
- repeat:
    times: [invalid]
    commands:
      - tapOn: "Test"
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid repeat step")
	}
}

func TestParse_RepeatNestedStepError(t *testing.T) {
	yaml := `
- repeat:
    times: "3"
    commands:
      - invalidStep: value
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid nested step in repeat")
	}
}

func TestParse_RetryStepDecodeError(t *testing.T) {
	yaml := `
- retry:
    maxRetries: [invalid]
    commands:
      - tapOn: "Test"
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid retry step")
	}
}

func TestParse_RetryNestedStepError(t *testing.T) {
	yaml := `
- retry:
    maxRetries: "3"
    commands:
      - invalidStep: value
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid nested step in retry")
	}
}

func TestParse_RunFlowDecodeError(t *testing.T) {
	yaml := `
- runFlow:
    file: [invalid]
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid runFlow step")
	}
}

func TestParse_RunFlowNestedStepError(t *testing.T) {
	yaml := `
- runFlow:
    commands:
      - invalidStep: value
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid nested step in runFlow")
	}
}

func TestParse_ConfigError(t *testing.T) {
	yaml := `
appId: [not, valid, scalar]
---
- tapOn: "Login"
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestParse_StepsError(t *testing.T) {
	yaml := `
- tapOn: [invalid
  structure
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid steps YAML")
	}
}

func TestParse_ConfigWithStepsError(t *testing.T) {
	yaml := `
appId: com.example
---
- tapOn: [invalid
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid steps after config")
	}
}

func TestParseDirectory_WithInvalidFiles(t *testing.T) {
	dir := t.TempDir()

	// Create an invalid YAML file
	content := `- tapOn: [invalid
  yaml`
	if err := os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Should not error but skip invalid files
	flows, err := ParseDirectory(dir, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flows) != 0 {
		t.Errorf("expected 0 flows (invalid skipped), got %d", len(flows))
	}
}

func TestParseDirectory_NonExistent(t *testing.T) {
	_, err := ParseDirectory("/nonexistent/path", nil, nil)
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestSplitYAMLDocuments_MultilineEndsWithIndent(t *testing.T) {
	// Test case where multiline ends because next line has less indentation
	content := `
- runScript: |
    line1
    line2
- tapOn: "Button"
`
	parts := splitYAMLDocuments(content)
	if len(parts) != 1 {
		t.Errorf("expected 1 part, got %d", len(parts))
	}
}

func TestSplitYAMLDocuments_MultilineWithDocSeparator(t *testing.T) {
	// Test multiline content ending before document separator
	content := `
appId: com.app
---
- runScript: |
    test()
- tapOn: "Done"
`
	parts := splitYAMLDocuments(content)
	if len(parts) != 2 {
		t.Errorf("expected 2 parts, got %d", len(parts))
	}
}

func TestSplitYAMLDocuments_FoldedMultiline(t *testing.T) {
	// Test folded style (>) multiline
	content := `
- inputText: >
    This is a long
    folded string
- tapOn: "Submit"
`
	parts := splitYAMLDocuments(content)
	if len(parts) != 1 {
		t.Errorf("expected 1 part, got %d", len(parts))
	}
}

func TestSplitYAMLDocuments_LiteralWithChomping(t *testing.T) {
	// Test literal style with chomping indicator (|-)
	content := `
- runScript: |-
    no trailing newline
- tapOn: "Next"
`
	parts := splitYAMLDocuments(content)
	if len(parts) != 1 {
		t.Errorf("expected 1 part, got %d", len(parts))
	}
}

func TestSplitYAMLDocuments_FoldedWithChomping(t *testing.T) {
	// Test folded style with chomping indicator (>-)
	content := `
- inputText: >-
    no trailing newline
- tapOn: "Next"
`
	parts := splitYAMLDocuments(content)
	if len(parts) != 1 {
		t.Errorf("expected 1 part, got %d", len(parts))
	}
}

func TestParse_ConfigWithURL(t *testing.T) {
	yaml := `
url: https://example.com
name: Web Test
---
- tapOn: "Login"
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if flow.Config.URL != "https://example.com" {
		t.Errorf("expected url=https://example.com, got %q", flow.Config.URL)
	}
	if flow.Config.Name != "Web Test" {
		t.Errorf("expected name=Web Test, got %q", flow.Config.Name)
	}
}

func TestParse_OnFlowStart(t *testing.T) {
	yaml := `
appId: com.example.app
onFlowStart:
  - runScript: setup.js
  - clearState:
---
- tapOn: "Login"
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Config.OnFlowStart) != 2 {
		t.Fatalf("expected 2 onFlowStart steps, got %d", len(flow.Config.OnFlowStart))
	}

	// Check first hook step
	script, ok := flow.Config.OnFlowStart[0].(*RunScriptStep)
	if !ok {
		t.Fatalf("expected RunScriptStep, got %T", flow.Config.OnFlowStart[0])
	}
	if script.Script != "setup.js" {
		t.Errorf("expected script=setup.js, got %q", script.Script)
	}

	// Check second hook step
	_, ok = flow.Config.OnFlowStart[1].(*ClearStateStep)
	if !ok {
		t.Fatalf("expected ClearStateStep, got %T", flow.Config.OnFlowStart[1])
	}

	// Main steps should still work
	if len(flow.Steps) != 1 {
		t.Errorf("expected 1 main step, got %d", len(flow.Steps))
	}
}

func TestParse_OnFlowComplete(t *testing.T) {
	yaml := `
appId: com.example.app
onFlowComplete:
  - takeScreenshot: "final.png"
  - stopApp:
---
- tapOn: "Login"
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Config.OnFlowComplete) != 2 {
		t.Fatalf("expected 2 onFlowComplete steps, got %d", len(flow.Config.OnFlowComplete))
	}

	// Check first hook step
	screenshot, ok := flow.Config.OnFlowComplete[0].(*TakeScreenshotStep)
	if !ok {
		t.Fatalf("expected TakeScreenshotStep, got %T", flow.Config.OnFlowComplete[0])
	}
	if screenshot.Path != "final.png" {
		t.Errorf("expected path=final.png, got %q", screenshot.Path)
	}

	// Check second hook step
	_, ok = flow.Config.OnFlowComplete[1].(*StopAppStep)
	if !ok {
		t.Fatalf("expected StopAppStep, got %T", flow.Config.OnFlowComplete[1])
	}
}

func TestParse_BothLifecycleHooks(t *testing.T) {
	yaml := `
appId: com.example.app
onFlowStart:
  - launchApp: com.example.app
onFlowComplete:
  - clearState:
---
- tapOn: "Button"
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Config.OnFlowStart) != 1 {
		t.Errorf("expected 1 onFlowStart step, got %d", len(flow.Config.OnFlowStart))
	}
	if len(flow.Config.OnFlowComplete) != 1 {
		t.Errorf("expected 1 onFlowComplete step, got %d", len(flow.Config.OnFlowComplete))
	}
}

func TestParse_OpenBrowserStep(t *testing.T) {
	yaml := `
- openBrowser: "https://example.com"
`
	flow, err := Parse([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(flow.Steps))
	}

	browser, ok := flow.Steps[0].(*OpenBrowserStep)
	if !ok {
		t.Fatalf("expected OpenBrowserStep, got %T", flow.Steps[0])
	}
	if browser.URL != "https://example.com" {
		t.Errorf("expected url=https://example.com, got %q", browser.URL)
	}
	if browser.Type() != StepOpenBrowser {
		t.Errorf("expected type=openBrowser, got %v", browser.Type())
	}
}

func TestParse_InvalidOnFlowStartStep(t *testing.T) {
	yaml := `
appId: com.example
onFlowStart:
  - invalidStep
---
- tapOn: "Button"
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid onFlowStart step")
	}
}

func TestParse_InvalidOnFlowCompleteStep(t *testing.T) {
	yaml := `
appId: com.example
onFlowComplete:
  - invalidStep
---
- tapOn: "Button"
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid onFlowComplete step")
	}
}

func TestParse_InvalidConfigYAML(t *testing.T) {
	yaml := `
appId: [invalid yaml
---
- tapOn: "Button"
`
	_, err := Parse([]byte(yaml), "test.yaml")
	if err == nil {
		t.Error("expected error for invalid config YAML")
	}
}

func TestFlow_IsSuite(t *testing.T) {
	tests := []struct {
		name     string
		flow     Flow
		expected bool
	}{
		{
			name:     "empty flow",
			flow:     Flow{},
			expected: false,
		},
		{
			name: "single test step",
			flow: Flow{
				Steps: []Step{
					&TapOnStep{BaseStep: BaseStep{StepType: StepTapOn}},
				},
			},
			expected: false,
		},
		{
			name: "mixed steps",
			flow: Flow{
				Steps: []Step{
					&LaunchAppStep{BaseStep: BaseStep{StepType: StepLaunchApp}},
					&RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}, File: "test.yaml"},
					&TapOnStep{BaseStep: BaseStep{StepType: StepTapOn}},
				},
			},
			expected: false,
		},
		{
			name: "single runFlow with file - not a suite",
			flow: Flow{
				Steps: []Step{
					&RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}, File: "test.yaml"},
				},
			},
			expected: false, // Need at least 2 runFlows
		},
		{
			name: "single runFlow inline - not a suite",
			flow: Flow{
				Steps: []Step{
					&RunFlowStep{
						BaseStep: BaseStep{StepType: StepRunFlow},
						Steps:    []Step{&TapOnStep{BaseStep: BaseStep{StepType: StepTapOn}}},
					},
				},
			},
			expected: false,
		},
		{
			name: "two runFlows with files - is a suite",
			flow: Flow{
				Steps: []Step{
					&RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}, File: "login.yaml"},
					&RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}, File: "checkout.yaml"},
				},
			},
			expected: true,
		},
		{
			name: "three runFlows with files - is a suite",
			flow: Flow{
				Steps: []Step{
					&RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}, File: "login.yaml"},
					&RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}, File: "checkout.yaml"},
					&RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}, File: "profile.yaml"},
				},
			},
			expected: true,
		},
		{
			name: "two runFlows but one is inline - not a suite",
			flow: Flow{
				Steps: []Step{
					&RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}, File: "login.yaml"},
					&RunFlowStep{
						BaseStep: BaseStep{StepType: StepRunFlow},
						Steps:    []Step{&TapOnStep{BaseStep: BaseStep{StepType: StepTapOn}}},
					},
				},
			},
			expected: false, // Only 1 runFlow with file
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.flow.IsSuite()
			if got != tt.expected {
				t.Errorf("IsSuite() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFlow_GetTestCases(t *testing.T) {
	// Non-suite should return nil
	nonSuite := Flow{
		Steps: []Step{
			&TapOnStep{BaseStep: BaseStep{StepType: StepTapOn}},
		},
	}
	if nonSuite.GetTestCases() != nil {
		t.Error("GetTestCases() should return nil for non-suite")
	}

	// Suite should return runFlow steps
	suite := Flow{
		Steps: []Step{
			&RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}, File: "login.yaml"},
			&RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}, File: "checkout.yaml"},
		},
	}
	testCases := suite.GetTestCases()
	if len(testCases) != 2 {
		t.Errorf("GetTestCases() = %d, want 2", len(testCases))
	}
	if testCases[0].File != "login.yaml" {
		t.Errorf("testCases[0].File = %q, want 'login.yaml'", testCases[0].File)
	}
	if testCases[1].File != "checkout.yaml" {
		t.Errorf("testCases[1].File = %q, want 'checkout.yaml'", testCases[1].File)
	}
}
