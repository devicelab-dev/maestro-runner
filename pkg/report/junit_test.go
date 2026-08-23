package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateJUnit(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now()
	endTime := now.Add(10 * time.Second)
	d1 := int64(5000)
	d2 := int64(3000)
	cmdDuration := int64(2500)

	index := &Index{
		Version:     "1.0.0",
		Status:      StatusPassed,
		StartTime:   now,
		EndTime:     &endTime,
		LastUpdated: now,
		Device: Device{
			ID:       "emulator-5554",
			Name:     "Pixel 6",
			Platform: "android",
		},
		App:           App{ID: "com.example.app"},
		MaestroRunner: RunnerInfo{Version: "0.1.0", Driver: "uiautomator2"},
		Summary: Summary{
			Total:  2,
			Passed: 2,
		},
		Flows: []FlowEntry{
			{
				Index:      0,
				ID:         "flow-000",
				Name:       "Login Test",
				SourceFile: "flows/login.yaml",
				DataFile:   "flows/flow-000.json",
				Status:     StatusPassed,
				Duration:   &d1,
				Commands:   CommandSummary{Total: 1, Passed: 1},
			},
			{
				Index:      1,
				ID:         "flow-001",
				Name:       "Signup Test",
				SourceFile: "flows/signup.yaml",
				DataFile:   "flows/flow-001.json",
				Status:     StatusPassed,
				Duration:   &d2,
				Commands:   CommandSummary{Total: 1, Passed: 1},
			},
		},
	}

	flow0 := FlowDetail{
		ID:        "flow-000",
		Name:      "Login Test",
		StartTime: now,
		Duration:  &d1,
		Commands: []Command{
			{ID: "cmd-000", Type: "launchApp", Status: StatusPassed, Duration: &cmdDuration},
		},
	}
	flow1 := FlowDetail{
		ID:        "flow-001",
		Name:      "Signup Test",
		StartTime: now,
		Duration:  &d2,
		Commands: []Command{
			{ID: "cmd-000", Type: "tapOn", Status: StatusPassed, Duration: &cmdDuration},
		},
	}

	// Write report files
	if err := os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "flows", "flow-000.json"), flow0); err != nil {
		t.Fatalf("write flow-000: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "flows", "flow-001.json"), flow1); err != nil {
		t.Fatalf("write flow-001: %v", err)
	}

	// Generate JUnit
	if err := GenerateJUnit(tmpDir); err != nil {
		t.Fatalf("GenerateJUnit: %v", err)
	}

	// Verify file exists
	junitPath := filepath.Join(tmpDir, "junit-report.xml")
	content, err := os.ReadFile(junitPath)
	if err != nil {
		t.Fatalf("read junit xml: %v", err)
	}

	xml := string(content)

	checks := []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<testsuites tests="2" failures="0" skipped="0" errors="0"`,
		`<testsuite name="maestro-runner" tests="2" failures="0" skipped="0"`,
		`<testcase name="Login Test" classname="Login Test" time="5.000"`,
		`<testcase name="Signup Test" classname="Signup Test" time="3.000"`,
		`<property name="file" value="flows/login.yaml"/>`,
		`<property name="file" value="flows/signup.yaml"/>`,
		`<property name="device.name" value="Pixel 6"/>`,
		`<property name="device.platform" value="android"/>`,
		`</testsuites>`,
	}

	for _, check := range checks {
		if !strings.Contains(xml, check) {
			t.Errorf("JUnit XML missing: %s", check)
		}
	}

	// Passed tests should not have <failure> or <skipped>
	if strings.Contains(xml, "<failure") {
		t.Error("passed tests should not contain <failure>")
	}
	if strings.Contains(xml, "<skipped") {
		t.Error("passed tests should not contain <skipped>")
	}
}

// TestJUnitFilePropertyKeepsSubdirectories is the #96 regression: the JUnit
// `file` property must keep the flow's subdirectory (e.g. authentication/flow.yaml)
// instead of being flattened to the bare filename. It mirrors the reported setup
// — flows discovered under a workspace dir, so SourceFile is an absolute path and
// the runner is invoked from that dir — by chdir'ing into a temp workspace with a
// real flow file.
func TestJUnitFilePropertyKeepsSubdirectories(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	ws := t.TempDir() // workspace = config.yaml dir
	if err := os.MkdirAll(filepath.Join(ws, "authentication"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "authentication", "flow.yaml"), []byte("- launchApp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(ws); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	end := now.Add(time.Second)
	d := int64(1000)
	index := &Index{
		StartTime: now,
		EndTime:   &end,
		Device:    Device{Name: "Pixel 9", Platform: "android"},
		Summary:   Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{{
			Index: 0, ID: "flow-000", Name: "Sign In",
			SourceFile: filepath.Join(ws, "authentication", "flow.yaml"), // absolute, as flow discovery produces
			DataFile:   "flows/flow-000.json",
			Status:     StatusPassed, Duration: &d,
			Commands: CommandSummary{Total: 1, Passed: 1},
		}},
	}
	if err := os.MkdirAll(filepath.Join(ws, "flows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteJSON(filepath.Join(ws, "report.json"), index); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteJSON(filepath.Join(ws, "flows", "flow-000.json"),
		FlowDetail{ID: "flow-000", Name: "Sign In", StartTime: now, Duration: &d}); err != nil {
		t.Fatal(err)
	}

	if err := GenerateJUnit(ws); err != nil {
		t.Fatalf("GenerateJUnit: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(ws, "junit-report.xml"))
	if err != nil {
		t.Fatal(err)
	}
	xml := string(content)

	if !strings.Contains(xml, `<property name="file" value="authentication/flow.yaml"/>`) {
		t.Errorf("expected file property to keep the subdirectory; got:\n%s", xml)
	}
	if strings.Contains(xml, `<property name="file" value="flow.yaml"/>`) {
		t.Error("file property was flattened to the bare filename (the #96 bug)")
	}
}

// TestRelSourceFile covers the path-relativization helper directly, from a
// chdir'd temp working dir with real files (so symlink resolution is consistent).
func TestRelSourceFile(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "a.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if got := relSourceFile(filepath.Join(dir, "sub", "a.yaml")); got != "sub/a.yaml" {
		t.Errorf("absolute under cwd: got %q, want %q", got, "sub/a.yaml")
	}
	if got := relSourceFile("sub/a.yaml"); got != "sub/a.yaml" {
		t.Errorf("relative path: got %q, want %q", got, "sub/a.yaml")
	}
	if got := relSourceFile(""); got != "" {
		t.Errorf("empty: got %q, want empty", got)
	}
	// Outside the working tree: keep the original rather than emitting "../..".
	outside := filepath.Join(filepath.Dir(orig), "elsewhere", "c.yaml")
	if got := relSourceFile(outside); got != outside {
		t.Errorf("outside cwd: got %q, want original %q", got, outside)
	}
}

func TestGenerateJUnitWithFailedFlows(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now()
	endTime := now.Add(5 * time.Second)
	d := int64(5000)
	cmdDuration := int64(2500)
	errMsg := "Element not found"

	index := &Index{
		Version:       "1.0.0",
		Status:        StatusFailed,
		StartTime:     now,
		EndTime:       &endTime,
		LastUpdated:   now,
		Device:        Device{ID: "test", Name: "Pixel 7", Platform: "android"},
		App:           App{ID: "com.test"},
		MaestroRunner: RunnerInfo{Version: "0.1.0", Driver: "uiautomator2"},
		Summary: Summary{
			Total:  1,
			Failed: 1,
		},
		Flows: []FlowEntry{
			{
				Index:      0,
				ID:         "flow-000",
				Name:       "Checkout",
				SourceFile: "flows/checkout.yaml",
				DataFile:   "flows/flow-000.json",
				Status:     StatusFailed,
				Duration:   &d,
				Error:      &errMsg,
				Commands:   CommandSummary{Total: 2, Passed: 1, Failed: 1},
			},
		},
	}

	flow0 := FlowDetail{
		ID:        "flow-000",
		Name:      "Checkout",
		StartTime: now,
		Duration:  &d,
		Commands: []Command{
			{ID: "cmd-000", Type: "launchApp", Status: StatusPassed, Duration: &cmdDuration},
			{
				ID:       "cmd-001",
				Type:     "assertVisible",
				Label:    "Verify checkout button",
				Status:   StatusFailed,
				Duration: &cmdDuration,
				Error: &Error{
					Type:    "element_not_found",
					Message: "Element not found",
				},
			},
		},
	}

	if err := os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "flows", "flow-000.json"), flow0); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	if err := GenerateJUnit(tmpDir); err != nil {
		t.Fatalf("GenerateJUnit: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "junit-report.xml"))
	xml := string(content)

	checks := []string{
		`failures="1"`,
		`<failure message="Element not found" type="AssertionError">Verify checkout button</failure>`,
	}
	for _, check := range checks {
		if !strings.Contains(xml, check) {
			t.Errorf("JUnit XML missing: %s\nGot:\n%s", check, xml)
		}
	}
}

func TestGenerateJUnitWithSkippedFlows(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now()
	endTime := now.Add(3 * time.Second)

	index := &Index{
		Version:       "1.0.0",
		Status:        StatusPassed,
		StartTime:     now,
		EndTime:       &endTime,
		LastUpdated:   now,
		Device:        Device{ID: "test", Name: "iPhone 15", Platform: "ios"},
		App:           App{ID: "com.test"},
		MaestroRunner: RunnerInfo{Version: "0.1.0", Driver: "xctest"},
		Summary: Summary{
			Total:   1,
			Skipped: 1,
		},
		Flows: []FlowEntry{
			{
				Index:      0,
				ID:         "flow-000",
				Name:       "Skipped Flow",
				SourceFile: "flows/skipped.yaml",
				DataFile:   "flows/flow-000.json",
				Status:     StatusSkipped,
				Commands:   CommandSummary{Total: 0},
			},
		},
	}

	flow0 := FlowDetail{
		ID:        "flow-000",
		Name:      "Skipped Flow",
		StartTime: now,
		Commands:  []Command{},
	}

	if err := os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "flows", "flow-000.json"), flow0); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	if err := GenerateJUnit(tmpDir); err != nil {
		t.Fatalf("GenerateJUnit: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "junit-report.xml"))
	xml := string(content)

	if !strings.Contains(xml, `skipped="1"`) {
		t.Error("missing skipped count")
	}
	if !strings.Contains(xml, "<skipped/>") {
		t.Error("missing <skipped/> element")
	}
	if !strings.Contains(xml, `<property name="device.platform" value="ios"/>`) {
		t.Error("missing ios platform property")
	}
}

func TestGenerateJUnitMixedResults(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now()
	endTime := now.Add(15 * time.Second)
	d1 := int64(5000)
	d2 := int64(3000)
	cmdDuration := int64(2500)
	errMsg := "Tap failed"

	index := &Index{
		Version:       "1.0.0",
		Status:        StatusFailed,
		StartTime:     now,
		EndTime:       &endTime,
		LastUpdated:   now,
		Device:        Device{ID: "emu-5554", Name: "Pixel 6", Platform: "android"},
		App:           App{ID: "com.test"},
		MaestroRunner: RunnerInfo{Version: "0.1.0", Driver: "uiautomator2"},
		Summary: Summary{
			Total:   3,
			Passed:  1,
			Failed:  1,
			Skipped: 1,
		},
		Flows: []FlowEntry{
			{
				Index: 0, ID: "flow-000", Name: "Login",
				SourceFile: "flows/login.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed, Duration: &d1,
				Commands: CommandSummary{Total: 1, Passed: 1},
			},
			{
				Index: 1, ID: "flow-001", Name: "Checkout",
				SourceFile: "flows/checkout.yaml", DataFile: "flows/flow-001.json",
				Status: StatusFailed, Duration: &d2, Error: &errMsg,
				Commands: CommandSummary{Total: 1, Failed: 1},
			},
			{
				Index: 2, ID: "flow-002", Name: "Settings",
				SourceFile: "flows/settings.yaml", DataFile: "flows/flow-002.json",
				Status:   StatusSkipped,
				Commands: CommandSummary{Total: 0},
			},
		},
	}

	flow0 := FlowDetail{
		ID: "flow-000", Name: "Login", StartTime: now, Duration: &d1,
		Commands: []Command{{ID: "cmd-000", Type: "launchApp", Status: StatusPassed, Duration: &cmdDuration}},
	}
	flow1 := FlowDetail{
		ID: "flow-001", Name: "Checkout", StartTime: now, Duration: &d2,
		Commands: []Command{{
			ID: "cmd-000", Type: "tapOn", Status: StatusFailed, Duration: &cmdDuration,
			Error: &Error{Type: "element_not_found", Message: "Tap failed"},
		}},
	}
	flow2 := FlowDetail{ID: "flow-002", Name: "Settings", StartTime: now, Commands: []Command{}}

	if err := os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index); err != nil {
		t.Fatalf("write index: %v", err)
	}
	for _, pair := range []struct {
		name string
		flow FlowDetail
	}{
		{"flow-000.json", flow0},
		{"flow-001.json", flow1},
		{"flow-002.json", flow2},
	} {
		if err := atomicWriteJSON(filepath.Join(tmpDir, "flows", pair.name), pair.flow); err != nil {
			t.Fatalf("write %s: %v", pair.name, err)
		}
	}

	if err := GenerateJUnit(tmpDir); err != nil {
		t.Fatalf("GenerateJUnit: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmpDir, "junit-report.xml"))
	xml := string(content)

	checks := []string{
		`tests="3" failures="1" skipped="1"`,
		`<testcase name="Login"`,
		`<testcase name="Checkout"`,
		`<testcase name="Settings"`,
		`<failure message="Tap failed" type="ElementInteractionError">tapOn</failure>`,
		`<skipped/>`,
	}
	for _, check := range checks {
		if !strings.Contains(xml, check) {
			t.Errorf("JUnit XML missing: %s\nGot:\n%s", check, xml)
		}
	}
}

func TestXMLEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"a & b", "a &amp; b"},
		{"a < b", "a &lt; b"},
		{"a > b", "a &gt; b"},
		{`a "b" c`, "a &quot;b&quot; c"},
		{"a 'b' c", "a &apos;b&apos; c"},
		{`<flow name="test & 'verify'">`, "&lt;flow name=&quot;test &amp; &apos;verify&apos;&quot;&gt;"},
	}

	for _, tt := range tests {
		got := xmlEscape(tt.input)
		if got != tt.expected {
			t.Errorf("xmlEscape(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestXMLEscapeInFlowNames(t *testing.T) {
	now := time.Now()
	endTime := now.Add(1 * time.Second)
	d := int64(1000)

	index := &Index{
		Version:   "1.0.0",
		Status:    StatusPassed,
		StartTime: now,
		EndTime:   &endTime,
		Device:    Device{ID: "test", Name: "Test <Device>", Platform: "android"},
		Summary:   Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{
				Index: 0, ID: "flow-000",
				Name:       `Login & "Signup" <test>`,
				SourceFile: "flows/login & signup.yaml",
				DataFile:   "flows/flow-000.json",
				Status:     StatusPassed, Duration: &d,
			},
		},
	}

	flows := []FlowDetail{
		{ID: "flow-000", Name: `Login & "Signup" <test>`, Commands: []Command{}},
	}

	xml := buildJUnitXML(index, flows)

	checks := []string{
		`name="Login &amp; &quot;Signup&quot; &lt;test&gt;"`,
		`value="Test &lt;Device&gt;"`,
		`value="flows/login &amp; signup.yaml"`,
	}
	for _, check := range checks {
		if !strings.Contains(xml, check) {
			t.Errorf("XML missing escaped content: %s\nGot:\n%s", check, xml)
		}
	}
}

func TestDeviceProperties(t *testing.T) {
	now := time.Now()
	endTime := now.Add(1 * time.Second)
	d := int64(1000)

	// Test with per-flow device
	flowDevice := &Device{
		ID:       "device-abc",
		Name:     "iPhone 15 Pro",
		Platform: "ios",
	}

	index := &Index{
		Version:   "1.0.0",
		Status:    StatusPassed,
		StartTime: now,
		EndTime:   &endTime,
		Device:    Device{ID: "default", Name: "Default Device", Platform: "android"},
		Summary:   Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{
				Index: 0, ID: "flow-000", Name: "Test",
				SourceFile: "test.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed, Duration: &d,
				Device: flowDevice,
			},
		},
	}

	flows := []FlowDetail{
		{ID: "flow-000", Name: "Test", Commands: []Command{}},
	}

	xml := buildJUnitXML(index, flows)

	// Should use the per-flow device, not the index-level device
	if !strings.Contains(xml, `value="iPhone 15 Pro"`) {
		t.Error("expected per-flow device name")
	}
	if !strings.Contains(xml, `value="ios"`) {
		t.Error("expected per-flow device platform")
	}
	if !strings.Contains(xml, `value="device-abc"`) {
		t.Error("expected per-flow device ID")
	}
	if strings.Contains(xml, `value="Default Device"`) {
		t.Error("should not use index-level device when flow device is present")
	}
}

func TestDevicePropertiesFallback(t *testing.T) {
	now := time.Now()
	endTime := now.Add(1 * time.Second)
	d := int64(1000)

	index := &Index{
		Version:   "1.0.0",
		Status:    StatusPassed,
		StartTime: now,
		EndTime:   &endTime,
		Device:    Device{ID: "emu-5554", Name: "Pixel 6", Platform: "android"},
		Summary:   Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{
				Index: 0, ID: "flow-000", Name: "Test",
				SourceFile: "test.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed, Duration: &d,
				// No Device set — should fall back to index Device
			},
		},
	}

	flows := []FlowDetail{
		{ID: "flow-000", Name: "Test", Commands: []Command{}},
	}

	xml := buildJUnitXML(index, flows)

	if !strings.Contains(xml, `value="Pixel 6"`) {
		t.Error("expected index-level device name as fallback")
	}
	if !strings.Contains(xml, `value="android"`) {
		t.Error("expected index-level device platform as fallback")
	}
}

func TestFailureTypeMapping(t *testing.T) {
	tests := []struct {
		cmdType  string
		expected string
	}{
		{"assertVisible", "AssertionError"},
		{"assertNotVisible", "AssertionError"},
		{"tapOn", "ElementInteractionError"},
		{"doubleTapOn", "ElementInteractionError"},
		{"longPressOn", "ElementInteractionError"},
		{"inputText", "InputError"},
		{"eraseText", "InputError"},
		{"launchApp", "AppLifecycleError"},
		{"stopApp", "AppLifecycleError"},
		{"runFlow", "SubflowError"},
		{"runScript", "SubflowError"},
		{"scroll", "ScrollError"},
		{"swipe", "ScrollError"},
		{"scrollUntilVisible", "ScrollError"},
		{"someUnknownType", "TestError"},
	}

	for _, tt := range tests {
		got := mapCommandTypeToFailure(tt.cmdType)
		if got != tt.expected {
			t.Errorf("mapCommandTypeToFailure(%q) = %q, want %q", tt.cmdType, got, tt.expected)
		}
	}
}

func TestFindFailedCommand(t *testing.T) {
	passed := Command{ID: "cmd-0", Type: "launchApp", Status: StatusPassed}
	failed := Command{ID: "cmd-1", Type: "assertVisible", Status: StatusFailed, Label: "Check welcome"}

	// Simple case
	cmd := findFailedCommand([]Command{passed, failed})
	if cmd == nil || cmd.ID != "cmd-1" {
		t.Errorf("expected cmd-1, got %v", cmd)
	}

	// No failures
	cmd = findFailedCommand([]Command{passed})
	if cmd != nil {
		t.Error("expected nil for no failures")
	}

	// Nested sub-commands
	parent := Command{
		ID:     "cmd-2",
		Type:   "runFlow",
		Status: StatusFailed,
		SubCommands: []Command{
			{ID: "sub-0", Type: "tapOn", Status: StatusPassed},
			{ID: "sub-1", Type: "inputText", Status: StatusFailed, Label: "Enter email"},
		},
	}
	cmd = findFailedCommand([]Command{passed, parent})
	if cmd == nil || cmd.ID != "sub-1" {
		t.Errorf("expected sub-1 (nested failure), got %v", cmd)
	}
}

func TestGenerateJUnitReadError(t *testing.T) {
	tmpDir := t.TempDir()
	err := GenerateJUnit(tmpDir)
	if err == nil {
		t.Error("expected error when report.json missing")
	}
}

func TestBuildJUnitXMLNoEndTime(t *testing.T) {
	now := time.Now()
	index := &Index{
		Version:   "1.0.0",
		Status:    StatusRunning,
		StartTime: now,
		Device:    Device{ID: "test", Name: "Test", Platform: "android"},
		Summary:   Summary{Total: 1},
		Flows: []FlowEntry{
			{
				Index: 0, ID: "flow-000", Name: "Test",
				SourceFile: "test.yaml", DataFile: "flows/flow-000.json",
				Status: StatusRunning,
			},
		},
	}

	flows := []FlowDetail{
		{ID: "flow-000", Name: "Test", Commands: []Command{}},
	}

	xml := buildJUnitXML(index, flows)

	// With no EndTime, total time should be 0
	if !strings.Contains(xml, `time="0.000"`) {
		t.Errorf("expected time=0.000 when no end time\nGot:\n%s", xml)
	}
}

func TestJUnit_CustomPropertiesAndAttachments(t *testing.T) {
	errMsg := "boom"
	dur := int64(1200)
	index := &Index{
		StartTime: time.Now(),
		Summary:   Summary{Total: 2, Failed: 1},
		Device:    Device{Name: "Pixel", ID: "abc", Platform: "android"},
		Flows: []FlowEntry{
			{Name: "green", SourceFile: "a.yaml", Status: StatusPassed, Duration: &dur},
			{Name: "red", SourceFile: "b.yaml", Status: StatusFailed, Duration: &dur, Error: &errMsg},
		},
	}
	flows := []FlowDetail{
		{
			Name:       "green",
			Properties: map[string]string{"testID": "Test-1234", "component": "auth"},
			Artifacts:  FlowArtifacts{Video: "assets/flow-000/recording.mp4"},
		},
		{
			Name: "red",
			Commands: []Command{{
				Type:      "tapOn",
				Status:    StatusFailed,
				Artifacts: CommandArtifacts{ScreenshotAfter: "assets/flow-001/cmd-000-after.png"},
			}},
		},
	}

	xml := buildJUnitXML(index, flows)

	for _, want := range []string{
		`<property name="testID" value="Test-1234"/>`,
		`<property name="component" value="auth"/>`,
		`[[ATTACHMENT|assets/flow-000/recording.mp4]]`,
		`[[ATTACHMENT|assets/flow-001/cmd-000-after.png]]`,
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("missing %q in:\n%s", want, xml)
		}
	}
	// Custom keys sorted: component before testID
	if strings.Index(xml, "component") > strings.Index(xml, "testID") {
		t.Error("custom properties should be emitted in sorted key order")
	}
	// A passing flow must not attach step screenshots
	if strings.Count(xml, "[[ATTACHMENT|") != 2 {
		t.Errorf("expected exactly 2 attachments, got:\n%s", xml)
	}
}
