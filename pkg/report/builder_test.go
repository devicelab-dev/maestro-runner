package report

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestBuildSkeleton(t *testing.T) {
	flows := []flow.Flow{
		{
			SourcePath: "flows/login.yaml",
			Config: flow.Config{
				Name: "Login Flow",
				Tags: []string{"smoke", "auth"},
			},
			Steps: []flow.Step{
				&flow.LaunchAppStep{
					BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp},
				},
				&flow.TapOnStep{
					BaseStep: flow.BaseStep{StepType: flow.StepTapOn},
					Selector: flow.Selector{ID: "login_button"},
				},
			},
		},
		{
			SourcePath: "flows/checkout.yaml",
			Config:     flow.Config{},
			Steps: []flow.Step{
				&flow.LaunchAppStep{
					BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp},
				},
			},
		},
	}

	cfg := BuilderConfig{
		OutputDir:     "/tmp/report",
		Device:        Device{ID: "emulator-5554", Platform: "android"},
		App:           App{ID: "com.example.app"},
		RunnerVersion: "0.1.0",
		DriverName:    "appium",
	}

	index, flowDetails, err := BuildSkeleton(flows, cfg)
	if err != nil {
		t.Fatalf("BuildSkeleton() error = %v", err)
	}

	// Check index
	if index.Version != Version {
		t.Errorf("index.Version = %q, want %q", index.Version, Version)
	}
	if index.Status != StatusPending {
		t.Errorf("index.Status = %q, want %q", index.Status, StatusPending)
	}
	if len(index.Flows) != 2 {
		t.Errorf("len(index.Flows) = %d, want 2", len(index.Flows))
	}

	// Check first flow entry
	if index.Flows[0].ID != "flow-000" {
		t.Errorf("index.Flows[0].ID = %q, want %q", index.Flows[0].ID, "flow-000")
	}
	if index.Flows[0].Name != "Login Flow" {
		t.Errorf("index.Flows[0].Name = %q, want %q", index.Flows[0].Name, "Login Flow")
	}
	if index.Flows[0].Commands.Total != 2 {
		t.Errorf("index.Flows[0].Commands.Total = %d, want 2", index.Flows[0].Commands.Total)
	}

	// Check second flow uses filename when no name configured
	if index.Flows[1].Name != "checkout" {
		t.Errorf("index.Flows[1].Name = %q, want %q", index.Flows[1].Name, "checkout")
	}

	// Check flow details
	if len(flowDetails) != 2 {
		t.Errorf("len(flowDetails) = %d, want 2", len(flowDetails))
	}
	if len(flowDetails[0].Commands) != 2 {
		t.Errorf("len(flowDetails[0].Commands) = %d, want 2", len(flowDetails[0].Commands))
	}
	if flowDetails[0].Commands[0].Status != StatusPending {
		t.Errorf("flowDetails[0].Commands[0].Status = %q, want %q", flowDetails[0].Commands[0].Status, StatusPending)
	}

	// Check summary
	if index.Summary.Total != 2 {
		t.Errorf("index.Summary.Total = %d, want 2", index.Summary.Total)
	}
	if index.Summary.Pending != 2 {
		t.Errorf("index.Summary.Pending = %d, want 2", index.Summary.Pending)
	}
}

func TestBuildSkeleton_ExtractParams(t *testing.T) {
	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Steps: []flow.Step{
				&flow.TapOnStep{
					BaseStep: flow.BaseStep{StepType: flow.StepTapOn},
					Selector: flow.Selector{ID: "btn"},
				},
				&flow.InputTextStep{
					BaseStep: flow.BaseStep{StepType: flow.StepInputText},
					Text:     "hello",
				},
				&flow.SwipeStep{
					BaseStep:  flow.BaseStep{StepType: flow.StepSwipe},
					Direction: "UP",
				},
				&flow.AssertVisibleStep{
					BaseStep: flow.BaseStep{StepType: flow.StepAssertVisible, TimeoutMs: 5000},
					Selector: flow.Selector{Text: "Welcome"},
				},
			},
		},
	}

	cfg := BuilderConfig{
		Device: Device{ID: "test"},
		App:    App{ID: "test"},
	}

	_, flowDetails, err := BuildSkeleton(flows, cfg)
	if err != nil {
		t.Fatalf("BuildSkeleton() error = %v", err)
	}

	commands := flowDetails[0].Commands

	// Check tapOn has selector
	if commands[0].Params == nil || commands[0].Params.Selector == nil {
		t.Error("commands[0] should have selector params")
	} else if commands[0].Params.Selector.Type != "id" {
		t.Errorf("commands[0].Params.Selector.Type = %q, want %q", commands[0].Params.Selector.Type, "id")
	}

	// Check inputText has text
	if commands[1].Params == nil || commands[1].Params.Text != "hello" {
		t.Error("commands[1] should have text param")
	}

	// Check swipe has direction
	if commands[2].Params == nil || commands[2].Params.Direction != "UP" {
		t.Error("commands[2] should have direction param")
	}

	// Check assertVisible has timeout
	if commands[3].Params == nil || commands[3].Params.Timeout != 5000 {
		t.Errorf("commands[3].Params.Timeout = %d, want 5000", commands[3].Params.Timeout)
	}
}

func TestWriteSkeleton(t *testing.T) {
	tmpDir := t.TempDir()

	flows := []flow.Flow{
		{
			SourcePath: "test.yaml",
			Config:     flow.Config{Name: "Test"},
			Steps: []flow.Step{
				&flow.LaunchAppStep{
					BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp},
				},
			},
		},
	}

	cfg := BuilderConfig{
		OutputDir: tmpDir,
		Device:    Device{ID: "test", Platform: "android"},
		App:       App{ID: "com.test"},
	}

	index, flowDetails, err := BuildSkeleton(flows, cfg)
	if err != nil {
		t.Fatalf("BuildSkeleton() error = %v", err)
	}

	err = WriteSkeleton(tmpDir, index, flowDetails)
	if err != nil {
		t.Fatalf("WriteSkeleton() error = %v", err)
	}

	// Check files exist
	if _, err := os.Stat(filepath.Join(tmpDir, "report.json")); err != nil {
		t.Errorf("report.json not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "flows", "flow-000.json")); err != nil {
		t.Errorf("flow-000.json not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "assets", "flow-000")); err != nil {
		t.Errorf("assets/flow-000 directory not created: %v", err)
	}

	// Read back and verify
	readIndex, err := ReadIndex(filepath.Join(tmpDir, "report.json"))
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}
	if readIndex.Status != StatusPending {
		t.Errorf("readIndex.Status = %q, want %q", readIndex.Status, StatusPending)
	}
}

func TestExtractFlowName(t *testing.T) {
	tests := []struct {
		name     string
		flow     flow.Flow
		expected string
	}{
		{
			name:     "uses config name",
			flow:     flow.Flow{Config: flow.Config{Name: "My Flow"}, SourcePath: "test.yaml"},
			expected: "My Flow",
		},
		{
			name:     "uses filename without extension",
			flow:     flow.Flow{SourcePath: "flows/login.yaml"},
			expected: "login",
		},
		{
			name:     "handles nested path",
			flow:     flow.Flow{SourcePath: "/path/to/checkout.yml"},
			expected: "checkout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFlowName(tt.flow)
			if got != tt.expected {
				t.Errorf("extractFlowName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGetBaseStep(t *testing.T) {
	tests := []struct {
		name      string
		step      flow.Step
		wantNil   bool
		wantTO    int // expected TimeoutMs if non-nil
	}{
		{
			name:    "TapOnStep",
			step:    &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn, TimeoutMs: 100}},
			wantNil: false,
			wantTO:  100,
		},
		{
			name:    "DoubleTapOnStep",
			step:    &flow.DoubleTapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepDoubleTapOn, TimeoutMs: 200}},
			wantNil: false,
			wantTO:  200,
		},
		{
			name:    "LongPressOnStep",
			step:    &flow.LongPressOnStep{BaseStep: flow.BaseStep{StepType: flow.StepLongPressOn, TimeoutMs: 300}},
			wantNil: false,
			wantTO:  300,
		},
		{
			name:    "AssertVisibleStep",
			step:    &flow.AssertVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertVisible, TimeoutMs: 400}},
			wantNil: false,
			wantTO:  400,
		},
		{
			name:    "AssertNotVisibleStep",
			step:    &flow.AssertNotVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertNotVisible, TimeoutMs: 500}},
			wantNil: false,
			wantTO:  500,
		},
		{
			name:    "InputTextStep",
			step:    &flow.InputTextStep{BaseStep: flow.BaseStep{StepType: flow.StepInputText, TimeoutMs: 600}},
			wantNil: false,
			wantTO:  600,
		},
		{
			name:    "SwipeStep",
			step:    &flow.SwipeStep{BaseStep: flow.BaseStep{StepType: flow.StepSwipe, TimeoutMs: 700}},
			wantNil: false,
			wantTO:  700,
		},
		{
			name:    "ScrollStep",
			step:    &flow.ScrollStep{BaseStep: flow.BaseStep{StepType: flow.StepScroll, TimeoutMs: 800}},
			wantNil: false,
			wantTO:  800,
		},
		{
			name:    "ScrollUntilVisibleStep",
			step:    &flow.ScrollUntilVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepScrollUntilVisible, TimeoutMs: 900}},
			wantNil: false,
			wantTO:  900,
		},
		{
			name:    "LaunchAppStep",
			step:    &flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp, TimeoutMs: 1000}},
			wantNil: false,
			wantTO:  1000,
		},
		{
			name:    "BackStep returns nil",
			step:    &flow.BackStep{BaseStep: flow.BaseStep{StepType: flow.StepBack, TimeoutMs: 50}},
			wantNil: true,
		},
		{
			name:    "HideKeyboardStep returns nil",
			step:    &flow.HideKeyboardStep{BaseStep: flow.BaseStep{StepType: flow.StepHideKeyboard}},
			wantNil: true,
		},
		{
			name:    "RunFlowStep returns nil",
			step:    &flow.RunFlowStep{BaseStep: flow.BaseStep{StepType: flow.StepRunFlow}},
			wantNil: true,
		},
		{
			name:    "RunScriptStep returns nil",
			step:    &flow.RunScriptStep{BaseStep: flow.BaseStep{StepType: flow.StepRunScript}},
			wantNil: true,
		},
		{
			name:    "UnsupportedStep returns nil",
			step:    &flow.UnsupportedStep{BaseStep: flow.BaseStep{StepType: "unknown"}, Reason: "test"},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getBaseStep(tt.step)
			if tt.wantNil {
				if got != nil {
					t.Errorf("getBaseStep() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("getBaseStep() = nil, want non-nil")
			}
			if got.TimeoutMs != tt.wantTO {
				t.Errorf("getBaseStep().TimeoutMs = %d, want %d", got.TimeoutMs, tt.wantTO)
			}
		})
	}
}

func TestExtractSelector(t *testing.T) {
	tests := []struct {
		name     string
		step     flow.Step
		wantNil  bool
		wantType string
		wantVal  string
	}{
		{
			name:     "TapOnStep with ID",
			step:     &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}, Selector: flow.Selector{ID: "btn"}},
			wantType: "id",
			wantVal:  "btn",
		},
		{
			name:     "DoubleTapOnStep with text",
			step:     &flow.DoubleTapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepDoubleTapOn}, Selector: flow.Selector{Text: "Submit"}},
			wantType: "text",
			wantVal:  "Submit",
		},
		{
			name:     "LongPressOnStep with CSS",
			step:     &flow.LongPressOnStep{BaseStep: flow.BaseStep{StepType: flow.StepLongPressOn}, Selector: flow.Selector{CSS: ".item"}},
			wantType: "css",
			wantVal:  ".item",
		},
		{
			name:     "AssertVisibleStep with text",
			step:     &flow.AssertVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertVisible}, Selector: flow.Selector{Text: "Welcome"}},
			wantType: "text",
			wantVal:  "Welcome",
		},
		{
			name:     "AssertNotVisibleStep with ID",
			step:     &flow.AssertNotVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertNotVisible}, Selector: flow.Selector{ID: "error_msg"}},
			wantType: "id",
			wantVal:  "error_msg",
		},
		{
			name:     "ScrollUntilVisibleStep uses Element field",
			step:     &flow.ScrollUntilVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepScrollUntilVisible}, Element: flow.Selector{Text: "End"}},
			wantType: "text",
			wantVal:  "End",
		},
		{
			name:     "InputTextStep with selector",
			step:     &flow.InputTextStep{BaseStep: flow.BaseStep{StepType: flow.StepInputText}, Selector: flow.Selector{ID: "input_field"}},
			wantType: "id",
			wantVal:  "input_field",
		},
		{
			name:     "CopyTextFromStep with text",
			step:     &flow.CopyTextFromStep{BaseStep: flow.BaseStep{StepType: flow.StepCopyTextFrom}, Selector: flow.Selector{Text: "Price"}},
			wantType: "text",
			wantVal:  "Price",
		},
		{
			name:    "TapOnStep with empty selector",
			step:    &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}, Selector: flow.Selector{}},
			wantNil: true,
		},
		{
			name:    "BackStep has no selector",
			step:    &flow.BackStep{BaseStep: flow.BaseStep{StepType: flow.StepBack}},
			wantNil: true,
		},
		{
			name:    "SwipeStep has no selector via extractSelector",
			step:    &flow.SwipeStep{BaseStep: flow.BaseStep{StepType: flow.StepSwipe}, Direction: "UP"},
			wantNil: true,
		},
		{
			name:    "LaunchAppStep has no selector",
			step:    &flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSelector(tt.step)
			if tt.wantNil {
				if got != nil {
					t.Errorf("extractSelector() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("extractSelector() = nil, want non-nil")
			}
			if got.Type != tt.wantType {
				t.Errorf("extractSelector().Type = %q, want %q", got.Type, tt.wantType)
			}
			if got.Value != tt.wantVal {
				t.Errorf("extractSelector().Value = %q, want %q", got.Value, tt.wantVal)
			}
		})
	}
}

func TestExtractParams(t *testing.T) {
	tests := []struct {
		name    string
		step    flow.Step
		wantNil bool
		check   func(t *testing.T, p *CommandParams)
	}{
		{
			name:    "step with no params returns nil",
			step:    &flow.BackStep{BaseStep: flow.BaseStep{StepType: flow.StepBack}},
			wantNil: true,
		},
		{
			name: "tapOn with selector",
			step: &flow.TapOnStep{
				BaseStep: flow.BaseStep{StepType: flow.StepTapOn},
				Selector: flow.Selector{ID: "btn"},
			},
			check: func(t *testing.T, p *CommandParams) {
				if p.Selector == nil {
					t.Fatal("expected Selector")
				}
				if p.Selector.Type != "id" || p.Selector.Value != "btn" {
					t.Errorf("Selector = {%q, %q}, want {id, btn}", p.Selector.Type, p.Selector.Value)
				}
			},
		},
		{
			name: "inputText with text and selector",
			step: &flow.InputTextStep{
				BaseStep: flow.BaseStep{StepType: flow.StepInputText},
				Text:     "hello world",
				Selector: flow.Selector{ID: "field"},
			},
			check: func(t *testing.T, p *CommandParams) {
				if p.Text != "hello world" {
					t.Errorf("Text = %q, want %q", p.Text, "hello world")
				}
				if p.Selector == nil || p.Selector.Type != "id" {
					t.Error("expected selector with type=id")
				}
			},
		},
		{
			name: "inputText with empty text but selector",
			step: &flow.InputTextStep{
				BaseStep: flow.BaseStep{StepType: flow.StepInputText},
				Text:     "",
				Selector: flow.Selector{ID: "field"},
			},
			check: func(t *testing.T, p *CommandParams) {
				if p.Text != "" {
					t.Errorf("Text = %q, want empty", p.Text)
				}
				if p.Selector == nil {
					t.Error("expected selector")
				}
			},
		},
		{
			name: "swipe with direction",
			step: &flow.SwipeStep{
				BaseStep:  flow.BaseStep{StepType: flow.StepSwipe},
				Direction: "DOWN",
			},
			check: func(t *testing.T, p *CommandParams) {
				if p.Direction != "DOWN" {
					t.Errorf("Direction = %q, want %q", p.Direction, "DOWN")
				}
			},
		},
		{
			name: "scroll with direction",
			step: &flow.ScrollStep{
				BaseStep:  flow.BaseStep{StepType: flow.StepScroll},
				Direction: "LEFT",
			},
			check: func(t *testing.T, p *CommandParams) {
				if p.Direction != "LEFT" {
					t.Errorf("Direction = %q, want %q", p.Direction, "LEFT")
				}
			},
		},
		{
			name: "scrollUntilVisible with direction and element",
			step: &flow.ScrollUntilVisibleStep{
				BaseStep:  flow.BaseStep{StepType: flow.StepScrollUntilVisible},
				Direction: "RIGHT",
				Element:   flow.Selector{Text: "Footer"},
			},
			check: func(t *testing.T, p *CommandParams) {
				if p.Direction != "RIGHT" {
					t.Errorf("Direction = %q, want %q", p.Direction, "RIGHT")
				}
				if p.Selector == nil || p.Selector.Value != "Footer" {
					t.Error("expected selector with value=Footer")
				}
			},
		},
		{
			name: "step with timeout only",
			step: &flow.LaunchAppStep{
				BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp, TimeoutMs: 30000},
			},
			check: func(t *testing.T, p *CommandParams) {
				if p.Timeout != 30000 {
					t.Errorf("Timeout = %d, want 30000", p.Timeout)
				}
			},
		},
		{
			name: "swipe with no direction returns nil",
			step: &flow.SwipeStep{
				BaseStep: flow.BaseStep{StepType: flow.StepSwipe},
			},
			wantNil: true,
		},
		{
			name: "scroll with no direction returns nil",
			step: &flow.ScrollStep{
				BaseStep: flow.BaseStep{StepType: flow.StepScroll},
			},
			wantNil: true,
		},
		{
			name: "selector with optional flag",
			step: &flow.TapOnStep{
				BaseStep: flow.BaseStep{StepType: flow.StepTapOn},
				Selector: flow.Selector{ID: "btn", Optional: boolPtr(true)},
			},
			check: func(t *testing.T, p *CommandParams) {
				if p.Selector == nil {
					t.Fatal("expected Selector")
				}
				if !p.Selector.Optional {
					t.Error("expected Selector.Optional = true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractParams(tt.step)
			if tt.wantNil {
				if got != nil {
					t.Errorf("extractParams() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("extractParams() = nil, want non-nil")
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestWriteSkeleton_MultipleFlows(t *testing.T) {
	tmpDir := t.TempDir()

	flows := []flow.Flow{
		{
			SourcePath: "test1.yaml",
			Config:     flow.Config{Name: "Test One"},
			Steps: []flow.Step{
				&flow.LaunchAppStep{BaseStep: flow.BaseStep{StepType: flow.StepLaunchApp}},
				&flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn}, Selector: flow.Selector{ID: "btn"}},
			},
		},
		{
			SourcePath: "test2.yaml",
			Config:     flow.Config{Name: "Test Two"},
			Steps: []flow.Step{
				&flow.AssertVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertVisible}, Selector: flow.Selector{Text: "Hello"}},
			},
		},
	}

	cfg := BuilderConfig{
		OutputDir: tmpDir,
		Device:    Device{ID: "test", Platform: "android"},
		App:       App{ID: "com.test"},
	}

	index, flowDetails, err := BuildSkeleton(flows, cfg)
	if err != nil {
		t.Fatalf("BuildSkeleton() error = %v", err)
	}

	err = WriteSkeleton(tmpDir, index, flowDetails)
	if err != nil {
		t.Fatalf("WriteSkeleton() error = %v", err)
	}

	// Check all files exist
	for _, path := range []string{
		filepath.Join(tmpDir, "report.json"),
		filepath.Join(tmpDir, "report.html"),
		filepath.Join(tmpDir, "flows", "flow-000.json"),
		filepath.Join(tmpDir, "flows", "flow-001.json"),
		filepath.Join(tmpDir, "assets", "flow-000"),
		filepath.Join(tmpDir, "assets", "flow-001"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected path %q to exist: %v", path, err)
		}
	}

	// Read back and verify structure
	readIndex, err := ReadIndex(filepath.Join(tmpDir, "report.json"))
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}
	if len(readIndex.Flows) != 2 {
		t.Errorf("len(readIndex.Flows) = %d, want 2", len(readIndex.Flows))
	}
	if readIndex.Flows[0].Commands.Total != 2 {
		t.Errorf("Flows[0].Commands.Total = %d, want 2", readIndex.Flows[0].Commands.Total)
	}
	if readIndex.Flows[1].Commands.Total != 1 {
		t.Errorf("Flows[1].Commands.Total = %d, want 1", readIndex.Flows[1].Commands.Total)
	}
}

func TestWriteSkeleton_EmptyFlows(t *testing.T) {
	tmpDir := t.TempDir()

	index := &Index{
		Version: Version,
		Status:  StatusPending,
		Device:  Device{ID: "test", Platform: "android"},
		App:     App{ID: "com.test"},
		Summary: Summary{},
		Flows:   []FlowEntry{},
	}

	err := WriteSkeleton(tmpDir, index, []FlowDetail{})
	if err != nil {
		t.Fatalf("WriteSkeleton() error = %v", err)
	}

	// report.json should still exist
	if _, err := os.Stat(filepath.Join(tmpDir, "report.json")); err != nil {
		t.Errorf("report.json not created: %v", err)
	}
	// report.html should still exist
	if _, err := os.Stat(filepath.Join(tmpDir, "report.html")); err != nil {
		t.Errorf("report.html not created: %v", err)
	}
}

func TestConvertSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector *flow.Selector
		expected *Selector
	}{
		{
			name:     "nil selector",
			selector: nil,
			expected: nil,
		},
		{
			name:     "id selector",
			selector: &flow.Selector{ID: "btn"},
			expected: &Selector{Type: "id", Value: "btn"},
		},
		{
			name:     "text selector",
			selector: &flow.Selector{Text: "Click me"},
			expected: &Selector{Type: "text", Value: "Click me"},
		},
		{
			name:     "css selector",
			selector: &flow.Selector{CSS: ".button"},
			expected: &Selector{Type: "css", Value: ".button"},
		},
		{
			name:     "empty selector",
			selector: &flow.Selector{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertSelector(tt.selector)
			if tt.expected == nil {
				if got != nil {
					t.Errorf("convertSelector() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("convertSelector() = nil, want %v", tt.expected)
				return
			}
			if got.Type != tt.expected.Type || got.Value != tt.expected.Value {
				t.Errorf("convertSelector() = {%q, %q}, want {%q, %q}",
					got.Type, got.Value, tt.expected.Type, tt.expected.Value)
			}
		})
	}
}
