package mock

import (
	"context"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestNew_Defaults(t *testing.T) {
	d := New(Config{})
	if d.Config.Platform != "mock" {
		t.Errorf("default platform: got %q", d.Config.Platform)
	}
	if d.Config.DeviceID != "mock-device" {
		t.Errorf("default device id: got %q", d.Config.DeviceID)
	}
}

func TestNew_Overrides(t *testing.T) {
	d := New(Config{Platform: "android", DeviceID: "pixel"})
	if d.Config.Platform != "android" || d.Config.DeviceID != "pixel" {
		t.Errorf("overrides not preserved: %+v", d.Config)
	}
}

func tapStep() *flow.TapOnStep {
	return &flow.TapOnStep{
		BaseStep: flow.BaseStep{StepType: flow.StepTapOn},
		Selector: flow.Selector{Text: "btn"},
	}
}
func backStep() *flow.BackStep {
	return &flow.BackStep{BaseStep: flow.BaseStep{StepType: flow.StepBack}}
}

func TestExecute_SuccessTapStep(t *testing.T) {
	d := New(Config{})
	res := d.Execute(tapStep())
	if !res.Success {
		t.Fatalf("expected success, got: %v", res.Error)
	}
	if res.Element == nil {
		t.Error("tapOn step should produce an Element")
	}
	if res.Duration <= 0 {
		t.Errorf("expected non-zero duration, got %v", res.Duration)
	}
}

func TestExecute_SuccessNonElementStep(t *testing.T) {
	d := New(Config{})
	res := d.Execute(backStep())
	if !res.Success {
		t.Fatalf("expected success: %v", res.Error)
	}
	if res.Element != nil {
		t.Error("BackStep should not produce an Element")
	}
}

func TestExecute_FailOnStep(t *testing.T) {
	d := New(Config{FailOnStep: 2})
	if !d.Execute(backStep()).Success {
		t.Fatal("step 1 should succeed")
	}
	res := d.Execute(backStep())
	if res.Success {
		t.Fatal("step 2 should fail")
	}
	if res.Error == nil {
		t.Error("step 2 should have a non-nil error")
	}
	if !d.Execute(backStep()).Success {
		t.Error("step 3 should succeed (FailOnStep only matches step 2)")
	}
}

func TestExecute_StepDelay(t *testing.T) {
	d := New(Config{StepDelay: 5 * time.Millisecond})
	start := time.Now()
	d.Execute(backStep())
	if time.Since(start) < 5*time.Millisecond {
		t.Error("StepDelay was not honored")
	}
}

func TestScreenshot(t *testing.T) {
	d := New(Config{})
	data, err := d.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if len(data) < 50 {
		t.Errorf("expected PNG-sized data, got %d bytes", len(data))
	}
	// PNG magic bytes
	if data[0] != 0x89 || data[1] != 0x50 || data[2] != 0x4E || data[3] != 0x47 {
		t.Error("data is not a PNG (missing magic bytes)")
	}
}

func TestHierarchy(t *testing.T) {
	d := New(Config{})
	data, err := d.Hierarchy()
	if err != nil {
		t.Fatalf("Hierarchy: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty hierarchy")
	}
}

func TestGetState(t *testing.T) {
	d := New(Config{})
	state := d.GetState()
	if state == nil {
		t.Fatal("GetState returned nil")
	}
	if state.AppState != "foreground" || state.Orientation != "portrait" {
		t.Errorf("unexpected state: %+v", state)
	}
}

func TestGetPlatformInfo(t *testing.T) {
	d := New(Config{Platform: "android", DeviceID: "pixel-99"})
	info := d.GetPlatformInfo()
	if info == nil {
		t.Fatal("GetPlatformInfo returned nil")
	}
	if info.Platform != "android" || info.DeviceID != "pixel-99" {
		t.Errorf("info should reflect Config: %+v", info)
	}
	if !info.IsSimulator {
		t.Error("mock driver should be marked as simulator")
	}
	if info.ScreenWidth == 0 || info.ScreenHeight == 0 {
		t.Error("expected non-zero screen dimensions")
	}
}

func TestSetters_NoOps(t *testing.T) {
	d := New(Config{})
	// These are documented no-ops, but call them for coverage.
	d.SetFindTimeout(1234)
	if err := d.SetWaitForIdleTimeout(50); err != nil {
		t.Errorf("SetWaitForIdleTimeout should be no-op, got err %v", err)
	}
	d.SetContext(context.Background())
}

func TestNeedsElement(t *testing.T) {
	// step.Type() reads from BaseStep.StepType (the parser sets this).
	cases := []struct {
		step flow.Step
		want bool
	}{
		{&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}}, true},
		{&flow.DoubleTapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepDoubleTapOn}}, true},
		{&flow.LongPressOnStep{BaseStep: flow.BaseStep{StepType: flow.StepLongPressOn}}, true},
		{&flow.AssertVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertVisible}}, true},
		{&flow.ScrollUntilVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepScrollUntilVisible}}, true},
		{&flow.CopyTextFromStep{BaseStep: flow.BaseStep{StepType: flow.StepCopyTextFrom}}, true},
		{&flow.BackStep{BaseStep: flow.BaseStep{StepType: flow.StepBack}}, false},
		{&flow.HideKeyboardStep{BaseStep: flow.BaseStep{StepType: flow.StepHideKeyboard}}, false},
	}
	for _, c := range cases {
		if got := needsElement(c.step); got != c.want {
			t.Errorf("needsElement(%T) = %v, want %v", c.step, got, c.want)
		}
	}
}
