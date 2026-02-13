package flow

import "testing"

func TestBaseStep_Type(t *testing.T) {
	b := BaseStep{StepType: StepTapOn}
	if got := b.Type(); got != StepTapOn {
		t.Errorf("Type()=%v, want %v", got, StepTapOn)
	}
}

func TestBaseStep_IsOptional(t *testing.T) {
	tests := []struct {
		name     string
		optional bool
		expected bool
	}{
		{"not optional", false, false},
		{"optional", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := BaseStep{Optional: tt.optional}
			if got := b.IsOptional(); got != tt.expected {
				t.Errorf("IsOptional()=%v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBaseStep_Label(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		expected string
	}{
		{"empty label", "", ""},
		{"with label", "login step", "login step"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := BaseStep{StepLabel: tt.label}
			if got := b.Label(); got != tt.expected {
				t.Errorf("Label()=%q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBaseStep_Describe(t *testing.T) {
	tests := []struct {
		name     string
		stepType StepType
		expected string
	}{
		{"tapOn", StepTapOn, "tapOn"},
		{"swipe", StepSwipe, "swipe"},
		{"assertVisible", StepAssertVisible, "assertVisible"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := BaseStep{StepType: tt.stepType}
			if got := b.Describe(); got != tt.expected {
				t.Errorf("Describe()=%q, want %q", got, tt.expected)
			}
		})
	}
}

func TestUnsupportedStep_Describe(t *testing.T) {
	s := UnsupportedStep{
		BaseStep: BaseStep{StepType: "unknownCommand"},
		Reason:   "not implemented",
	}

	expected := "unknownCommand (unsupported: not implemented)"
	if got := s.Describe(); got != expected {
		t.Errorf("Describe()=%q, want %q", got, expected)
	}
}

func TestStepInterface(t *testing.T) {
	// Verify all step types implement the Step interface
	steps := []Step{
		&TapOnStep{BaseStep: BaseStep{StepType: StepTapOn}},
		&DoubleTapOnStep{BaseStep: BaseStep{StepType: StepDoubleTapOn}},
		&LongPressOnStep{BaseStep: BaseStep{StepType: StepLongPressOn}},
		&TapOnPointStep{BaseStep: BaseStep{StepType: StepTapOnPoint}},
		&SwipeStep{BaseStep: BaseStep{StepType: StepSwipe}},
		&ScrollStep{BaseStep: BaseStep{StepType: StepScroll}},
		&ScrollUntilVisibleStep{BaseStep: BaseStep{StepType: StepScrollUntilVisible}},
		&BackStep{BaseStep: BaseStep{StepType: StepBack}},
		&HideKeyboardStep{BaseStep: BaseStep{StepType: StepHideKeyboard}},
		&AcceptAlertStep{BaseStep: BaseStep{StepType: StepAcceptAlert}},
		&DismissAlertStep{BaseStep: BaseStep{StepType: StepDismissAlert}},
		&InputTextStep{BaseStep: BaseStep{StepType: StepInputText}},
		&InputRandomStep{BaseStep: BaseStep{StepType: StepInputRandom}},
		&EraseTextStep{BaseStep: BaseStep{StepType: StepEraseText}},
		&CopyTextFromStep{BaseStep: BaseStep{StepType: StepCopyTextFrom}},
		&PasteTextStep{BaseStep: BaseStep{StepType: StepPasteText}},
		&SetClipboardStep{BaseStep: BaseStep{StepType: StepSetClipboard}},
		&AssertVisibleStep{BaseStep: BaseStep{StepType: StepAssertVisible}},
		&AssertNotVisibleStep{BaseStep: BaseStep{StepType: StepAssertNotVisible}},
		&AssertTrueStep{BaseStep: BaseStep{StepType: StepAssertTrue}},
		&AssertConditionStep{BaseStep: BaseStep{StepType: StepAssertCondition}},
		&AssertNoDefectsWithAIStep{BaseStep: BaseStep{StepType: StepAssertNoDefectsWithAI}},
		&AssertWithAIStep{BaseStep: BaseStep{StepType: StepAssertWithAI}},
		&ExtractTextWithAIStep{BaseStep: BaseStep{StepType: StepExtractTextWithAI}},
		&WaitUntilStep{BaseStep: BaseStep{StepType: StepWaitUntil}},
		&LaunchAppStep{BaseStep: BaseStep{StepType: StepLaunchApp}},
		&StopAppStep{BaseStep: BaseStep{StepType: StepStopApp}},
		&KillAppStep{BaseStep: BaseStep{StepType: StepKillApp}},
		&ClearStateStep{BaseStep: BaseStep{StepType: StepClearState}},
		&ClearKeychainStep{BaseStep: BaseStep{StepType: StepClearKeychain}},
		&SetPermissionsStep{BaseStep: BaseStep{StepType: StepSetPermissions}},
		&SetLocationStep{BaseStep: BaseStep{StepType: StepSetLocation}},
		&SetOrientationStep{BaseStep: BaseStep{StepType: StepSetOrientation}},
		&SetAirplaneModeStep{BaseStep: BaseStep{StepType: StepSetAirplaneMode}},
		&ToggleAirplaneModeStep{BaseStep: BaseStep{StepType: StepToggleAirplaneMode}},
		&TravelStep{BaseStep: BaseStep{StepType: StepTravel}},
		&OpenLinkStep{BaseStep: BaseStep{StepType: StepOpenLink}},
		&OpenBrowserStep{BaseStep: BaseStep{StepType: StepOpenBrowser}},
		&RepeatStep{BaseStep: BaseStep{StepType: StepRepeat}},
		&RetryStep{BaseStep: BaseStep{StepType: StepRetry}},
		&RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}},
		&RunScriptStep{BaseStep: BaseStep{StepType: StepRunScript}},
		&EvalScriptStep{BaseStep: BaseStep{StepType: StepEvalScript}},
		&TakeScreenshotStep{BaseStep: BaseStep{StepType: StepTakeScreenshot}},
		&StartRecordingStep{BaseStep: BaseStep{StepType: StepStartRecording}},
		&StopRecordingStep{BaseStep: BaseStep{StepType: StepStopRecording}},
		&AddMediaStep{BaseStep: BaseStep{StepType: StepAddMedia}},
		&PressKeyStep{BaseStep: BaseStep{StepType: StepPressKey}},
		&WaitForAnimationToEndStep{BaseStep: BaseStep{StepType: StepWaitForAnimationToEnd}},
		&DefineVariablesStep{BaseStep: BaseStep{StepType: StepDefineVariables}},
		&UnsupportedStep{BaseStep: BaseStep{StepType: "unknown"}, Reason: "test"},
	}

	for _, step := range steps {
		// Verify interface methods are callable
		_ = step.Type()
		_ = step.IsOptional()
		_ = step.Label()
		_ = step.Describe()
	}

	if len(steps) == 0 {
		t.Error("expected at least one step type")
	}
}

func TestTapOnStep_Fields(t *testing.T) {
	boolTrue := true
	s := TapOnStep{
		BaseStep:              BaseStep{StepType: StepTapOn, Optional: true, StepLabel: "tap", TimeoutMs: 5000},
		Selector:              Selector{Text: "Login"},
		LongPress:             true,
		Repeat:                2,
		DelayMs:               100,
		Point:                 "50%, 50%",
		RetryTapIfNoChange:    &boolTrue,
		WaitUntilVisible:      &boolTrue,
		WaitToSettleTimeoutMs: 500,
	}

	if s.Type() != StepTapOn {
		t.Errorf("Type()=%v, want %v", s.Type(), StepTapOn)
	}
	if !s.IsOptional() {
		t.Error("expected optional=true")
	}
	if s.Label() != "tap" {
		t.Errorf("Label()=%q, want tap", s.Label())
	}
	if s.Selector.Text != "Login" {
		t.Errorf("Selector.Text=%q, want Login", s.Selector.Text)
	}
	if !s.LongPress {
		t.Error("expected LongPress=true")
	}
	if s.Repeat != 2 {
		t.Errorf("Repeat=%d, want 2", s.Repeat)
	}
	if s.DelayMs != 100 {
		t.Errorf("DelayMs=%d, want 100", s.DelayMs)
	}
	if s.Point != "50%, 50%" {
		t.Errorf("Point=%q, want 50%%, 50%%", s.Point)
	}
	if s.RetryTapIfNoChange == nil || !*s.RetryTapIfNoChange {
		t.Error("expected RetryTapIfNoChange=true")
	}
	if s.WaitUntilVisible == nil || !*s.WaitUntilVisible {
		t.Error("expected WaitUntilVisible=true")
	}
	if s.WaitToSettleTimeoutMs != 500 {
		t.Errorf("WaitToSettleTimeoutMs=%d, want 500", s.WaitToSettleTimeoutMs)
	}
}

func TestSwipeStep_Fields(t *testing.T) {
	s := SwipeStep{
		BaseStep:              BaseStep{StepType: StepSwipe},
		Direction:             "UP",
		Selector:              &Selector{ID: "list"},
		Start:                 "50%, 80%",
		End:                   "50%, 20%",
		StartX:                100,
		StartY:                500,
		EndX:                  100,
		EndY:                  200,
		Duration:              300,
		Speed:                 50,
		WaitToSettleTimeoutMs: 200,
	}

	if s.Direction != "UP" {
		t.Errorf("Direction=%q, want UP", s.Direction)
	}
	if s.Selector == nil || s.Selector.ID != "list" {
		t.Error("expected Selector.ID=list")
	}
	if s.Start != "50%, 80%" {
		t.Errorf("Start=%q, want 50%%, 80%%", s.Start)
	}
	if s.End != "50%, 20%" {
		t.Errorf("End=%q, want 50%%, 20%%", s.End)
	}
	if s.StartX != 100 || s.StartY != 500 {
		t.Errorf("StartX,StartY=%d,%d, want 100,500", s.StartX, s.StartY)
	}
	if s.EndX != 100 || s.EndY != 200 {
		t.Errorf("EndX,EndY=%d,%d, want 100,200", s.EndX, s.EndY)
	}
	if s.Duration != 300 {
		t.Errorf("Duration=%d, want 300", s.Duration)
	}
	if s.Speed != 50 {
		t.Errorf("Speed=%d, want 50", s.Speed)
	}
	if s.WaitToSettleTimeoutMs != 200 {
		t.Errorf("WaitToSettleTimeoutMs=%d, want 200", s.WaitToSettleTimeoutMs)
	}
}

func TestScrollUntilVisibleStep_Fields(t *testing.T) {
	s := ScrollUntilVisibleStep{
		BaseStep:              BaseStep{StepType: StepScrollUntilVisible, TimeoutMs: 10000},
		Element:               Selector{Text: "End of list"},
		Direction:             "DOWN",
		MaxScrolls:            20,
		Speed:                 40,
		VisibilityPercentage:  80,
		CenterElement:         true,
		WaitToSettleTimeoutMs: 100,
	}

	if s.Element.Text != "End of list" {
		t.Errorf("Element.Text=%q, want End of list", s.Element.Text)
	}
	if s.Direction != "DOWN" {
		t.Errorf("Direction=%q, want DOWN", s.Direction)
	}
	if s.MaxScrolls != 20 {
		t.Errorf("MaxScrolls=%d, want 20", s.MaxScrolls)
	}
	if s.Speed != 40 {
		t.Errorf("Speed=%d, want 40", s.Speed)
	}
	if s.VisibilityPercentage != 80 {
		t.Errorf("VisibilityPercentage=%d, want 80", s.VisibilityPercentage)
	}
	if !s.CenterElement {
		t.Error("expected CenterElement=true")
	}
}

func TestLaunchAppStep_Fields(t *testing.T) {
	boolFalse := false
	s := LaunchAppStep{
		BaseStep:      BaseStep{StepType: StepLaunchApp},
		AppID:         "com.example.app",
		ClearState:    true,
		ClearKeychain: true,
		StopApp:       &boolFalse,
		Permissions:   map[string]string{"camera": "allow", "location": "deny"},
		Arguments:     map[string]any{"debug": true, "env": "test"},
	}

	if s.AppID != "com.example.app" {
		t.Errorf("AppID=%q, want com.example.app", s.AppID)
	}
	if !s.ClearState {
		t.Error("expected ClearState=true")
	}
	if !s.ClearKeychain {
		t.Error("expected ClearKeychain=true")
	}
	if s.StopApp == nil || *s.StopApp {
		t.Error("expected StopApp=false")
	}
	if len(s.Permissions) != 2 {
		t.Errorf("len(Permissions)=%d, want 2", len(s.Permissions))
	}
	if len(s.Arguments) != 2 {
		t.Errorf("len(Arguments)=%d, want 2", len(s.Arguments))
	}
}

func TestSetLocationStep_Fields(t *testing.T) {
	s := SetLocationStep{
		BaseStep:  BaseStep{StepType: StepSetLocation},
		Latitude:  "37.7749",
		Longitude: "-122.4194",
	}

	if s.Latitude != "37.7749" {
		t.Errorf("Latitude=%q, want 37.7749", s.Latitude)
	}
	if s.Longitude != "-122.4194" {
		t.Errorf("Longitude=%q, want -122.4194", s.Longitude)
	}
}

func TestRepeatStep_Fields(t *testing.T) {
	s := RepeatStep{
		BaseStep: BaseStep{StepType: StepRepeat, Optional: true, StepLabel: "repeat block"},
		Times:    "5",
		While:    Condition{Script: "counter < 10"},
		Steps: []Step{
			&TapOnStep{BaseStep: BaseStep{StepType: StepTapOn}, Selector: Selector{Text: "Next"}},
		},
	}

	if s.Times != "5" {
		t.Errorf("Times=%q, want 5", s.Times)
	}
	if s.While.Script != "counter < 10" {
		t.Errorf("While.Script=%q, want counter < 10", s.While.Script)
	}
	if len(s.Steps) != 1 {
		t.Errorf("len(Steps)=%d, want 1", len(s.Steps))
	}
}

func TestRetryStep_Fields(t *testing.T) {
	s := RetryStep{
		BaseStep:   BaseStep{StepType: StepRetry},
		MaxRetries: "3",
		File:       "retry-flow.yaml",
		Env:        map[string]string{"RETRY": "true"},
		Steps: []Step{
			&TapOnStep{BaseStep: BaseStep{StepType: StepTapOn}},
		},
	}

	if s.MaxRetries != "3" {
		t.Errorf("MaxRetries=%q, want 3", s.MaxRetries)
	}
	if s.File != "retry-flow.yaml" {
		t.Errorf("File=%q, want retry-flow.yaml", s.File)
	}
	if len(s.Env) != 1 {
		t.Errorf("len(Env)=%d, want 1", len(s.Env))
	}
	if len(s.Steps) != 1 {
		t.Errorf("len(Steps)=%d, want 1", len(s.Steps))
	}
}

func TestCondition_Fields(t *testing.T) {
	c := Condition{
		Visible:    &Selector{Text: "Success"},
		NotVisible: &Selector{ID: "error"},
		Script:     "result === true",
		Platform:   "Android",
	}

	if c.Visible == nil || c.Visible.Text != "Success" {
		t.Error("expected Visible.Text=Success")
	}
	if c.NotVisible == nil || c.NotVisible.ID != "error" {
		t.Error("expected NotVisible.ID=error")
	}
	if c.Script != "result === true" {
		t.Errorf("Script=%q, want result === true", c.Script)
	}
	if c.Platform != "Android" {
		t.Errorf("Platform=%q, want Android", c.Platform)
	}
}

func TestAISteps_Fields(t *testing.T) {
	t.Run("AssertWithAIStep", func(t *testing.T) {
		s := AssertWithAIStep{
			BaseStep:  BaseStep{StepType: StepAssertWithAI},
			Assertion: "The login button should be visible",
		}
		if s.Assertion != "The login button should be visible" {
			t.Errorf("Assertion=%q, want The login button should be visible", s.Assertion)
		}
	})

	t.Run("ExtractTextWithAIStep", func(t *testing.T) {
		s := ExtractTextWithAIStep{
			BaseStep: BaseStep{StepType: StepExtractTextWithAI},
			Query:    "What is the total price?",
			Variable: "totalPrice",
		}
		if s.Query != "What is the total price?" {
			t.Errorf("Query=%q, want What is the total price?", s.Query)
		}
		if s.Variable != "totalPrice" {
			t.Errorf("Variable=%q, want totalPrice", s.Variable)
		}
	})
}

func TestOpenLinkStep_Fields(t *testing.T) {
	boolTrue := true
	boolFalse := false
	s := OpenLinkStep{
		BaseStep:   BaseStep{StepType: StepOpenLink},
		Link:       "https://example.com",
		AutoVerify: &boolTrue,
		Browser:    &boolFalse,
	}

	if s.Link != "https://example.com" {
		t.Errorf("Link=%q, want https://example.com", s.Link)
	}
	if s.AutoVerify == nil || !*s.AutoVerify {
		t.Error("expected AutoVerify=true")
	}
	if s.Browser == nil || *s.Browser {
		t.Error("expected Browser=false")
	}
}

func TestRunScriptStep_ScriptPath(t *testing.T) {
	tests := []struct {
		name     string
		step     RunScriptStep
		expected string
	}{
		{
			name: "file field takes precedence",
			step: RunScriptStep{
				BaseStep: BaseStep{StepType: StepRunScript},
				Script:   "inline-script.js",
				File:     "scripts/run.sh",
			},
			expected: "scripts/run.sh",
		},
		{
			name: "falls back to script field",
			step: RunScriptStep{
				BaseStep: BaseStep{StepType: StepRunScript},
				Script:   "scripts/test.js",
			},
			expected: "scripts/test.js",
		},
		{
			name: "both empty returns empty",
			step: RunScriptStep{
				BaseStep: BaseStep{StepType: StepRunScript},
			},
			expected: "",
		},
		{
			name: "file only",
			step: RunScriptStep{
				BaseStep: BaseStep{StepType: StepRunScript},
				File:     "run-tests.sh",
			},
			expected: "run-tests.sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.step.ScriptPath()
			if got != tt.expected {
				t.Errorf("ScriptPath() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTapOnStep_Describe(t *testing.T) {
	s := TapOnStep{
		BaseStep: BaseStep{StepType: StepTapOn},
		Selector: Selector{Text: "Login"},
	}
	expected := `tapOn: text="Login"`
	if got := s.Describe(); got != expected {
		t.Errorf("Describe() = %q, want %q", got, expected)
	}
}

func TestDoubleTapOnStep_Describe(t *testing.T) {
	s := DoubleTapOnStep{
		BaseStep: BaseStep{StepType: StepDoubleTapOn},
		Selector: Selector{ID: "btn"},
	}
	expected := `doubleTapOn: id="btn"`
	if got := s.Describe(); got != expected {
		t.Errorf("Describe() = %q, want %q", got, expected)
	}
}

func TestLongPressOnStep_Describe(t *testing.T) {
	s := LongPressOnStep{
		BaseStep: BaseStep{StepType: StepLongPressOn},
		Selector: Selector{CSS: ".menu-item"},
	}
	expected := `longPressOn: css=".menu-item"`
	if got := s.Describe(); got != expected {
		t.Errorf("Describe() = %q, want %q", got, expected)
	}
}

func TestAssertVisibleStep_Describe(t *testing.T) {
	s := AssertVisibleStep{
		BaseStep: BaseStep{StepType: StepAssertVisible},
		Selector: Selector{Text: "Welcome"},
	}
	expected := `assertVisible: text="Welcome"`
	if got := s.Describe(); got != expected {
		t.Errorf("Describe() = %q, want %q", got, expected)
	}
}

func TestAssertNotVisibleStep_Describe(t *testing.T) {
	s := AssertNotVisibleStep{
		BaseStep: BaseStep{StepType: StepAssertNotVisible},
		Selector: Selector{ID: "error"},
	}
	expected := `assertNotVisible: id="error"`
	if got := s.Describe(); got != expected {
		t.Errorf("Describe() = %q, want %q", got, expected)
	}
}

func TestInputTextStep_Describe(t *testing.T) {
	s := InputTextStep{
		BaseStep: BaseStep{StepType: StepInputText},
		Text:     "user@example.com",
	}
	expected := `inputText: "user@example.com"`
	if got := s.Describe(); got != expected {
		t.Errorf("Describe() = %q, want %q", got, expected)
	}
}

func TestLaunchAppStep_Describe(t *testing.T) {
	tests := []struct {
		name       string
		clearState bool
		expected   string
	}{
		{
			name:       "without clearState",
			clearState: false,
			expected:   "launchApp",
		},
		{
			name:       "with clearState",
			clearState: true,
			expected:   "launchApp (clearState)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := LaunchAppStep{
				BaseStep:   BaseStep{StepType: StepLaunchApp},
				ClearState: tt.clearState,
			}
			if got := s.Describe(); got != tt.expected {
				t.Errorf("Describe() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestWaitUntilStep_Describe(t *testing.T) {
	tests := []struct {
		name     string
		step     WaitUntilStep
		expected string
	}{
		{
			name: "with visible",
			step: WaitUntilStep{
				BaseStep: BaseStep{StepType: StepWaitUntil},
				Visible:  &Selector{Text: "Done"},
			},
			expected: `extendedWaitUntil: visible text="Done"`,
		},
		{
			name: "with notVisible",
			step: WaitUntilStep{
				BaseStep:   BaseStep{StepType: StepWaitUntil},
				NotVisible: &Selector{ID: "spinner"},
			},
			expected: `extendedWaitUntil: notVisible id="spinner"`,
		},
		{
			name: "neither visible nor notVisible",
			step: WaitUntilStep{
				BaseStep: BaseStep{StepType: StepWaitUntil},
			},
			expected: "extendedWaitUntil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.step.Describe(); got != tt.expected {
				t.Errorf("Describe() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestScrollUntilVisibleStep_Describe(t *testing.T) {
	s := ScrollUntilVisibleStep{
		BaseStep: BaseStep{StepType: StepScrollUntilVisible},
		Element:  Selector{Text: "End of list"},
	}
	expected := `scrollUntilVisible: text="End of list"`
	if got := s.Describe(); got != expected {
		t.Errorf("Describe() = %q, want %q", got, expected)
	}
}

func TestCopyTextFromStep_Describe(t *testing.T) {
	s := CopyTextFromStep{
		BaseStep: BaseStep{StepType: StepCopyTextFrom},
		Selector: Selector{ID: "price_label"},
	}
	expected := `copyTextFrom: id="price_label"`
	if got := s.Describe(); got != expected {
		t.Errorf("Describe() = %q, want %q", got, expected)
	}
}

func TestRunFlowStep_Describe(t *testing.T) {
	tests := []struct {
		name     string
		step     RunFlowStep
		expected string
	}{
		{
			name: "with file",
			step: RunFlowStep{
				BaseStep: BaseStep{StepType: StepRunFlow},
				File:     "login.yaml",
			},
			expected: "runFlow: login.yaml",
		},
		{
			name: "without file",
			step: RunFlowStep{
				BaseStep: BaseStep{StepType: StepRunFlow},
			},
			expected: "runFlow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.step.Describe(); got != tt.expected {
				t.Errorf("Describe() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestPressKeyStep_Describe(t *testing.T) {
	s := PressKeyStep{
		BaseStep: BaseStep{StepType: StepPressKey},
		Key:      "Enter",
	}
	expected := "pressKey: Enter"
	if got := s.Describe(); got != expected {
		t.Errorf("Describe() = %q, want %q", got, expected)
	}
}

func TestSwipeStep_Describe(t *testing.T) {
	tests := []struct {
		name     string
		step     SwipeStep
		expected string
	}{
		{
			name: "with direction",
			step: SwipeStep{
				BaseStep:  BaseStep{StepType: StepSwipe},
				Direction: "UP",
			},
			expected: "swipe: UP",
		},
		{
			name: "without direction",
			step: SwipeStep{
				BaseStep: BaseStep{StepType: StepSwipe},
			},
			expected: "swipe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.step.Describe(); got != tt.expected {
				t.Errorf("Describe() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestScrollStep_Describe(t *testing.T) {
	tests := []struct {
		name     string
		step     ScrollStep
		expected string
	}{
		{
			name: "with direction",
			step: ScrollStep{
				BaseStep:  BaseStep{StepType: StepScroll},
				Direction: "DOWN",
			},
			expected: "scroll: DOWN",
		},
		{
			name: "without direction",
			step: ScrollStep{
				BaseStep: BaseStep{StepType: StepScroll},
			},
			expected: "scroll",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.step.Describe(); got != tt.expected {
				t.Errorf("Describe() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSetPermissionsStep_Describe(t *testing.T) {
	s := SetPermissionsStep{
		BaseStep:    BaseStep{StepType: StepSetPermissions},
		Permissions: map[string]string{"camera": "allow"},
	}
	expected := "setPermissions"
	if got := s.Describe(); got != expected {
		t.Errorf("Describe() = %q, want %q", got, expected)
	}
}

func TestStepTypeConstants(t *testing.T) {
	// Verify step type constants have correct values
	expectedTypes := map[StepType]string{
		StepTapOn:                 "tapOn",
		StepDoubleTapOn:           "doubleTapOn",
		StepLongPressOn:           "longPressOn",
		StepTapOnPoint:            "tapOnPoint",
		StepSwipe:                 "swipe",
		StepScroll:                "scroll",
		StepScrollUntilVisible:    "scrollUntilVisible",
		StepBack:                  "back",
		StepHideKeyboard:          "hideKeyboard",
		StepAcceptAlert:           "acceptAlert",
		StepDismissAlert:          "dismissAlert",
		StepInputText:             "inputText",
		StepInputRandom:           "inputRandom",
		StepEraseText:             "eraseText",
		StepCopyTextFrom:          "copyTextFrom",
		StepPasteText:             "pasteText",
		StepSetClipboard:          "setClipboard",
		StepAssertVisible:         "assertVisible",
		StepAssertNotVisible:      "assertNotVisible",
		StepAssertTrue:            "assertTrue",
		StepAssertCondition:       "assertCondition",
		StepAssertNoDefectsWithAI: "assertNoDefectsWithAI",
		StepAssertWithAI:          "assertWithAI",
		StepExtractTextWithAI:     "extractTextWithAI",
		StepWaitUntil:             "extendedWaitUntil",
		StepLaunchApp:             "launchApp",
		StepStopApp:               "stopApp",
		StepKillApp:               "killApp",
		StepClearState:            "clearState",
		StepClearKeychain:         "clearKeychain",
		StepSetPermissions:        "setPermissions",
		StepSetLocation:           "setLocation",
		StepSetOrientation:        "setOrientation",
		StepSetAirplaneMode:       "setAirplaneMode",
		StepToggleAirplaneMode:    "toggleAirplaneMode",
		StepTravel:                "travel",
		StepOpenLink:              "openLink",
		StepOpenBrowser:           "openBrowser",
		StepRepeat:                "repeat",
		StepRetry:                 "retry",
		StepRunFlow:               "runFlow",
		StepRunScript:             "runScript",
		StepEvalScript:            "evalScript",
		StepTakeScreenshot:        "takeScreenshot",
		StepStartRecording:        "startRecording",
		StepStopRecording:         "stopRecording",
		StepAddMedia:              "addMedia",
		StepPressKey:              "pressKey",
		StepWaitForAnimationToEnd: "waitForAnimationToEnd",
		StepDefineVariables:       "defineVariables",
	}

	for stepType, expected := range expectedTypes {
		if string(stepType) != expected {
			t.Errorf("StepType %q != %q", stepType, expected)
		}
	}
}
