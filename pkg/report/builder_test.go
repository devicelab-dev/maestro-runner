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
