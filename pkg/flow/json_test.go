package flow

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestUnmarshalStep_TapOn(t *testing.T) {
	input := `{"type":"tapOn","selector":"Login","longPress":true,"timeout":5000}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	if step.Type() != StepTapOn {
		t.Fatalf("expected type %s, got %s", StepTapOn, step.Type())
	}
	tap := step.(*TapOnStep)
	if tap.Selector.Text != "Login" {
		t.Errorf("expected text 'Login', got %q", tap.Selector.Text)
	}
	if !tap.LongPress {
		t.Error("expected longPress=true")
	}
	if tap.TimeoutMs != 5000 {
		t.Errorf("expected timeout 5000, got %d", tap.TimeoutMs)
	}
}

func TestUnmarshalStep_TapOnByID(t *testing.T) {
	input := `{"type":"tapOn","selector":{"id":"btn_login"},"longPress":true}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	tap := step.(*TapOnStep)
	if tap.Selector.ID != "btn_login" {
		t.Errorf("expected id 'btn_login', got %q", tap.Selector.ID)
	}
	if !tap.LongPress {
		t.Error("expected longPress=true")
	}
}

func TestUnmarshalStep_InputText(t *testing.T) {
	input := `{"type":"inputText","text":"user@example.com"}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	if step.Type() != StepInputText {
		t.Fatalf("expected type %s, got %s", StepInputText, step.Type())
	}
	is := step.(*InputTextStep)
	if is.Text != "user@example.com" {
		t.Errorf("expected text 'user@example.com', got %q", is.Text)
	}
}

func TestUnmarshalStep_AssertVisible(t *testing.T) {
	input := `{"type":"assertVisible","selector":"Dashboard","timeout":10000}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	if step.Type() != StepAssertVisible {
		t.Fatalf("expected type %s, got %s", StepAssertVisible, step.Type())
	}
	av := step.(*AssertVisibleStep)
	if av.Selector.Text != "Dashboard" {
		t.Errorf("expected text 'Dashboard', got %q", av.Selector.Text)
	}
	if av.TimeoutMs != 10000 {
		t.Errorf("expected timeout 10000, got %d", av.TimeoutMs)
	}
}

func TestUnmarshalStep_LaunchApp(t *testing.T) {
	input := `{"type":"launchApp","appId":"com.example.app","clearState":true}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	la := step.(*LaunchAppStep)
	if la.AppID != "com.example.app" {
		t.Errorf("expected appId 'com.example.app', got %q", la.AppID)
	}
	if !la.ClearState {
		t.Error("expected clearState=true")
	}
}

func TestUnmarshalStep_Swipe(t *testing.T) {
	input := `{"type":"swipe","direction":"UP","duration":400}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	sw := step.(*SwipeStep)
	if sw.Direction != "UP" {
		t.Errorf("expected direction UP, got %q", sw.Direction)
	}
	if sw.Duration != 400 {
		t.Errorf("expected duration 400, got %d", sw.Duration)
	}
}

func TestUnmarshalStep_Back(t *testing.T) {
	input := `{"type":"back"}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	if step.Type() != StepBack {
		t.Fatalf("expected type %s, got %s", StepBack, step.Type())
	}
}

func TestUnmarshalStep_PressKey(t *testing.T) {
	input := `{"type":"pressKey","key":"ENTER"}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	pk := step.(*PressKeyStep)
	if pk.Key != "ENTER" {
		t.Errorf("expected key 'ENTER', got %q", pk.Key)
	}
}

func TestUnmarshalStep_EraseText(t *testing.T) {
	input := `{"type":"eraseText","charactersToErase":5}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	et := step.(*EraseTextStep)
	if et.Characters != 5 {
		t.Errorf("expected characters 5, got %d", et.Characters)
	}
}

func TestUnmarshalStep_MissingType(t *testing.T) {
	input := `{"text":"Login"}`
	_, err := UnmarshalStep([]byte(input))
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestUnmarshalStep_Optional(t *testing.T) {
	input := `{"type":"tapOn","selector":"OK","optional":true}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	if !step.IsOptional() {
		t.Error("expected optional=true")
	}
}

func TestUnmarshalStep_StopApp(t *testing.T) {
	input := `{"type":"stopApp","appId":"com.example.app"}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	sa := step.(*StopAppStep)
	if sa.AppID != "com.example.app" {
		t.Errorf("expected appId 'com.example.app', got %q", sa.AppID)
	}
}

func TestUnmarshalStep_Scroll(t *testing.T) {
	input := `{"type":"scroll"}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("UnmarshalStep error: %v", err)
	}
	if step.Type() != StepScroll {
		t.Fatalf("expected type %s, got %s", StepScroll, step.Type())
	}
}

// TestUnmarshalStep_SupportedStepTypes pins the protocol contract: every step
// type the JSON protocol advertises must decode to its documented concrete
// flow.Step implementation. Adding a step type here without teaching
// unmarshalStepByType about it will fail, which is the point — the server must
// not silently accept a step it cannot execute.
func TestUnmarshalStep_SupportedStepTypes(t *testing.T) {
	tests := []struct {
		stepType StepType
		want     Step
	}{
		{StepTapOn, (*TapOnStep)(nil)},
		{StepDoubleTapOn, (*DoubleTapOnStep)(nil)},
		{StepLongPressOn, (*LongPressOnStep)(nil)},
		{StepTapOnPoint, (*TapOnPointStep)(nil)},
		{StepSwipe, (*SwipeStep)(nil)},
		{StepScroll, (*ScrollStep)(nil)},
		{StepScrollUntilVisible, (*ScrollUntilVisibleStep)(nil)},
		{StepBack, (*BackStep)(nil)},
		{StepHideKeyboard, (*HideKeyboardStep)(nil)},
		{StepAcceptAlert, (*AcceptAlertStep)(nil)},
		{StepIsKeyboardVisible, (*IsKeyboardVisibleStep)(nil)},
		{StepDismissAlert, (*DismissAlertStep)(nil)},
		{StepInputText, (*InputTextStep)(nil)},
		{StepInputRandom, (*InputRandomStep)(nil)},
		{StepEraseText, (*EraseTextStep)(nil)},
		{StepCopyTextFrom, (*CopyTextFromStep)(nil)},
		{StepPasteText, (*PasteTextStep)(nil)},
		{StepSetClipboard, (*SetClipboardStep)(nil)},
		{StepAssertVisible, (*AssertVisibleStep)(nil)},
		{StepAssertNotVisible, (*AssertNotVisibleStep)(nil)},
		{StepAssertTrue, (*AssertTrueStep)(nil)},
		{StepAssertCondition, (*AssertConditionStep)(nil)},
		{StepLaunchApp, (*LaunchAppStep)(nil)},
		{StepStopApp, (*StopAppStep)(nil)},
		{StepKillApp, (*KillAppStep)(nil)},
		{StepClearState, (*ClearStateStep)(nil)},
		{StepClearKeychain, (*ClearKeychainStep)(nil)},
		{StepSetPermissions, (*SetPermissionsStep)(nil)},
		{StepSetLocation, (*SetLocationStep)(nil)},
		{StepSetOrientation, (*SetOrientationStep)(nil)},
		{StepOpenLink, (*OpenLinkStep)(nil)},
		{StepOpenBrowser, (*OpenBrowserStep)(nil)},
		{StepPressKey, (*PressKeyStep)(nil)},
		{StepSleep, (*SleepStep)(nil)},
		{StepWaitForAnimationToEnd, (*WaitForAnimationToEndStep)(nil)},
		{StepTakeScreenshot, (*TakeScreenshotStep)(nil)},
	}

	for _, tt := range tests {
		t.Run(string(tt.stepType), func(t *testing.T) {
			body := `{"type":"` + string(tt.stepType) + `"}`
			step, err := UnmarshalStep([]byte(body))
			if err != nil {
				t.Fatalf("UnmarshalStep(%s): %v", body, err)
			}
			if step.Type() != tt.stepType {
				t.Errorf("Type() = %q, want %q", step.Type(), tt.stepType)
			}
			if got, want := reflect.TypeOf(step), reflect.TypeOf(tt.want); got != want {
				t.Errorf("concrete type = %v, want %v", got, want)
			}
		})
	}
}

// TestUnmarshalStep_UnsupportedStepTypeIsAnError asserts that step types the
// JSON protocol does not implement produce a clear, non-panicking error naming
// the offending type. Clients rely on this to distinguish "I sent a step the
// server cannot run" from a transport failure.
func TestUnmarshalStep_UnsupportedStepTypeIsAnError(t *testing.T) {
	unsupported := []StepType{
		StepOpenNotifications,
		StepInputRandomPersonName,
		StepInputRandomText,
		StepWaitUntil,
		StepSetDarkMode,
		StepAssertDarkMode,
		StepEvalBrowserScript,
		StepOpenTab,
		StepAssertScreenshot,
		StepStartRecording,
		StepAddMedia,
		StepDefineVariables,
	}

	for _, stepType := range unsupported {
		t.Run(string(stepType), func(t *testing.T) {
			body := `{"type":"` + string(stepType) + `"}`
			step, err := UnmarshalStep([]byte(body))
			if err == nil {
				t.Fatalf("expected an error for unsupported step type %q, got step %v", stepType, step)
			}
			if !strings.Contains(err.Error(), string(stepType)) {
				t.Errorf("error %q should name the unsupported step type %q", err, stepType)
			}
		})
	}
}

// TestSelector_JSONRoundTrip covers the selector shapes clients actually send.
// A selector that survives marshal -> unmarshal unchanged is what lets the
// server hand the same selector to the driver that YAML would have produced.
func TestSelector_JSONRoundTrip(t *testing.T) {
	textChild := Selector{Text: "Password"}
	enabled := true
	selected := false
	nth := 2

	tests := []struct {
		name     string
		selector Selector
	}{
		{"text only", Selector{Text: "Login"}},
		{"id only", Selector{ID: "btn_login"}},
		{"text plus id", Selector{Text: "Login", ID: "btn_login"}},
		{"boolean state filter", Selector{Text: "Notify", Enabled: &enabled}},
		{"explicit false boolean", Selector{Text: "Notify", Selected: &selected}},
		{"nested relative selector", Selector{Text: "Login", ChildOf: &textChild}},
		{"size matching", Selector{Text: "Logo", Width: 100, Height: 50, Tolerance: 5}},
		{"css selector", Selector{CSS: "#submit"}},
		{"index", Selector{Text: "Row", Index: "2"}},
		{"traits", Selector{Text: "Row", Traits: "button,heading"}},
		{"web fields", Selector{Placeholder: "Email", Role: "button", TextContains: "log",
			Href: "/login", Alt: "Logo", Title: "Sign in", Name: "email",
			TestID: "login-btn", TextRegex: "^Log.*$", Nth: nth}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.selector)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got Selector
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal %s: %v", data, err)
			}
			if !reflect.DeepEqual(got, tt.selector) {
				t.Errorf("round-trip changed selector:\n got: %+v\nwant: %+v", got, tt.selector)
			}
		})
	}
}

// TestSelector_UnmarshalJSON_DocumentedKeys feeds the camelCase keys a client
// would naturally send and asserts each one lands on the intended field.
func TestSelector_UnmarshalJSON_DocumentedKeys(t *testing.T) {
	body := `{"text":"Login","id":"btn","enabled":true,"placeholder":"Email","role":"button",` +
		`"textContains":"log","href":"/login","alt":"Logo","title":"Sign in",` +
		`"name":"email","testId":"login-btn","textRegex":"^Log.*$","nth":2}`
	var s Selector
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := []struct {
		field string
		got   interface{}
		want  interface{}
	}{
		{"text", s.Text, "Login"},
		{"id", s.ID, "btn"},
		{"placeholder", s.Placeholder, "Email"},
		{"role", s.Role, "button"},
		{"textContains", s.TextContains, "log"},
		{"href", s.Href, "/login"},
		{"alt", s.Alt, "Logo"},
		{"title", s.Title, "Sign in"},
		{"name", s.Name, "email"},
		{"testId", s.TestID, "login-btn"},
		{"textRegex", s.TextRegex, "^Log.*$"},
		{"nth", s.Nth, 2},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("selector %s = %v, want %v", c.field, c.got, c.want)
		}
	}
	if s.Enabled == nil || !*s.Enabled {
		t.Errorf("enabled = %v, want true", s.Enabled)
	}
}

// TestSelector_UnmarshalJSON_Invalid asserts that a selector which is neither a
// string nor an object is reported as an error, so the server can answer 400
// instead of silently producing an empty selector that matches nothing.
// ("null" is excluded: Go treats JSON null as a no-op by design.)
func TestSelector_UnmarshalJSON_Invalid(t *testing.T) {
	for _, body := range []string{`123`, `true`, `["Login"]`} {
		t.Run(body, func(t *testing.T) {
			var s Selector
			if err := json.Unmarshal([]byte(body), &s); err == nil {
				t.Fatalf("expected error unmarshalling selector %s, got %+v", body, s)
			}
		})
	}
}

// --- Selector JSON ---

func TestSelector_MarshalJSON_TextOnly(t *testing.T) {
	s := Selector{Text: "Login"}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	// Text-only selector should marshal as plain string
	if string(data) != `"Login"` {
		t.Errorf("expected \"Login\", got %s", string(data))
	}
}

func TestSelector_MarshalJSON_WithID(t *testing.T) {
	s := Selector{Text: "Login", ID: "btn"}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	// Should be an object
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("expected JSON object, got: %s", string(data))
	}
	if m["text"] != "Login" {
		t.Errorf("expected text 'Login', got %v", m["text"])
	}
	if m["id"] != "btn" {
		t.Errorf("expected id 'btn', got %v", m["id"])
	}
}

func TestSelector_UnmarshalJSON_String(t *testing.T) {
	var s Selector
	if err := json.Unmarshal([]byte(`"Login"`), &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if s.Text != "Login" {
		t.Errorf("expected text 'Login', got %q", s.Text)
	}
}

// --- JSON round-trip: step type is preserved ---

func TestStepType_JSONRoundTrip(t *testing.T) {
	input := `{"type":"tapOn","selector":"Login"}`
	step, err := UnmarshalStep([]byte(input))
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if m["type"] != "tapOn" {
		t.Errorf("expected type 'tapOn' in JSON, got %v", m["type"])
	}
}
