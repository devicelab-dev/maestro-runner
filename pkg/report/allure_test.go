package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helper to write a test report to disk.
func writeTestReport(t *testing.T, tmpDir string, index *Index, flows []FlowDetail) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755); err != nil {
		t.Fatalf("mkdir flows: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index); err != nil {
		t.Fatalf("write index: %v", err)
	}
	for i, flow := range flows {
		name := index.Flows[i].DataFile
		if err := atomicWriteJSON(filepath.Join(tmpDir, name), flow); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestGenerateAllurePassedFlow(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(5 * time.Second)
	d := int64(5000)
	cmdDur := int64(2500)

	index := &Index{
		Version:       "1.0.0",
		Status:        StatusPassed,
		StartTime:     now,
		EndTime:       &endTime,
		LastUpdated:   now,
		Device:        Device{ID: "emulator-5554", Name: "Pixel 6", Platform: "android", OSVersion: "13"},
		App:           App{ID: "com.example.app"},
		MaestroRunner: RunnerInfo{Version: "0.2.0", Driver: "uiautomator2"},
		Summary:       Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{
				Index: 0, ID: "flow-000", Name: "Login Test",
				SourceFile: "flows/login.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed, Duration: &d,
				StartTime: &now, EndTime: &endTime,
				Tags:     []string{"smoke"},
				Commands: CommandSummary{Total: 1, Passed: 1},
			},
		},
	}

	flow0 := FlowDetail{
		ID: "flow-000", Name: "Login Test", StartTime: now, Duration: &d,
		Commands: []Command{
			{ID: "cmd-000", Type: "launchApp", Label: "Launch app", Status: StatusPassed, Duration: &cmdDur,
				StartTime: &now, EndTime: &endTime},
		},
	}

	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	if err := GenerateAllure(tmpDir); err != nil {
		t.Fatalf("GenerateAllure: %v", err)
	}

	// Verify result file exists
	resultPath := filepath.Join(tmpDir, "allure-results", "flow-000-result.json")
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}

	var result AllureResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if result.UUID != "flow-000" {
		t.Errorf("UUID = %q, want flow-000", result.UUID)
	}
	if result.Name != "Login Test" {
		t.Errorf("Name = %q, want Login Test", result.Name)
	}
	if result.Status != "passed" {
		t.Errorf("Status = %q, want passed", result.Status)
	}
	if result.Stage != "finished" {
		t.Errorf("Stage = %q, want finished", result.Stage)
	}
	if result.Start == 0 {
		t.Error("Start should not be 0")
	}
	if result.Stop == 0 {
		t.Error("Stop should not be 0")
	}

	// Check steps
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}
	if result.Steps[0].Name != "launchApp: Launch app" {
		t.Errorf("step name = %q, want launchApp: Launch app", result.Steps[0].Name)
	}
	if result.Steps[0].Status != "passed" {
		t.Errorf("step status = %q, want passed", result.Steps[0].Status)
	}
}

func TestGenerateAllureFailedFlow(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(3 * time.Second)
	d := int64(3000)
	cmdDur := int64(1500)
	errMsg := "Element not found: login_button"

	index := &Index{
		Version: "1.0.0", Status: StatusFailed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:        Device{ID: "emulator-5554", Name: "Pixel 6", Platform: "android"},
		App:           App{ID: "com.test"},
		MaestroRunner: RunnerInfo{Version: "0.2.0", Driver: "uiautomator2"},
		Summary:       Summary{Total: 1, Failed: 1},
		Flows: []FlowEntry{
			{
				Index: 0, ID: "flow-000", Name: "Checkout",
				SourceFile: "flows/checkout.yaml", DataFile: "flows/flow-000.json",
				Status: StatusFailed, Duration: &d, Error: &errMsg,
				StartTime: &now, EndTime: &endTime,
				Commands: CommandSummary{Total: 2, Passed: 1, Failed: 1},
			},
		},
	}

	flow0 := FlowDetail{
		ID: "flow-000", Name: "Checkout", StartTime: now, Duration: &d,
		Commands: []Command{
			{ID: "cmd-000", Type: "launchApp", Status: StatusPassed, Duration: &cmdDur,
				StartTime: &now, EndTime: &endTime},
			{ID: "cmd-001", Type: "assertVisible", Label: "Check button", Status: StatusFailed, Duration: &cmdDur,
				StartTime: &now, EndTime: &endTime,
				Error: &Error{Type: "element_not_found", Message: "Element not found: login_button"}},
		},
	}

	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	if err := GenerateAllure(tmpDir); err != nil {
		t.Fatalf("GenerateAllure: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "allure-results", "flow-000-result.json"))
	var result AllureResult
	json.Unmarshal(data, &result)

	if result.Status != "failed" {
		t.Errorf("Status = %q, want failed", result.Status)
	}
	if result.StatusDetails.Message != errMsg {
		t.Errorf("StatusDetails.Message = %q, want %q", result.StatusDetails.Message, errMsg)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}
	if result.Steps[1].Status != "failed" {
		t.Errorf("step[1] status = %q, want failed", result.Steps[1].Status)
	}
}

func TestGenerateAllureSkippedFlow(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(1 * time.Second)

	index := &Index{
		Version: "1.0.0", Status: StatusPassed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:  Device{ID: "test", Name: "iPhone 15", Platform: "ios"},
		App:     App{ID: "com.test"},
		Summary: Summary{Total: 1, Skipped: 1},
		Flows: []FlowEntry{
			{
				Index: 0, ID: "flow-000", Name: "Skipped Flow",
				SourceFile: "flows/skipped.yaml", DataFile: "flows/flow-000.json",
				Status: StatusSkipped,
			},
		},
	}

	flow0 := FlowDetail{ID: "flow-000", Name: "Skipped Flow", StartTime: now, Commands: []Command{}}

	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	if err := GenerateAllure(tmpDir); err != nil {
		t.Fatalf("GenerateAllure: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "allure-results", "flow-000-result.json"))
	var result AllureResult
	json.Unmarshal(data, &result)

	if result.Status != "skipped" {
		t.Errorf("Status = %q, want skipped", result.Status)
	}
	if len(result.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(result.Steps))
	}
}

func TestGenerateAllureMixedFlows(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(10 * time.Second)
	d1 := int64(5000)
	d2 := int64(3000)
	cmdDur := int64(2500)
	errMsg := "Tap failed"

	index := &Index{
		Version: "1.0.0", Status: StatusFailed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:        Device{ID: "emu-5554", Name: "Pixel 6", Platform: "android"},
		App:           App{ID: "com.test"},
		MaestroRunner: RunnerInfo{Version: "0.1.0", Driver: "uiautomator2"},
		Summary:       Summary{Total: 3, Passed: 1, Failed: 1, Skipped: 1},
		Flows: []FlowEntry{
			{Index: 0, ID: "flow-000", Name: "Login",
				SourceFile: "flows/login.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed, Duration: &d1, StartTime: &now, EndTime: &endTime,
				Commands: CommandSummary{Total: 1, Passed: 1}},
			{Index: 1, ID: "flow-001", Name: "Checkout",
				SourceFile: "flows/checkout.yaml", DataFile: "flows/flow-001.json",
				Status: StatusFailed, Duration: &d2, Error: &errMsg,
				StartTime: &now, EndTime: &endTime,
				Commands: CommandSummary{Total: 1, Failed: 1}},
			{Index: 2, ID: "flow-002", Name: "Settings",
				SourceFile: "flows/settings.yaml", DataFile: "flows/flow-002.json",
				Status: StatusSkipped},
		},
	}

	flows := []FlowDetail{
		{ID: "flow-000", Name: "Login", StartTime: now, Duration: &d1,
			Commands: []Command{{ID: "cmd-000", Type: "launchApp", Status: StatusPassed, Duration: &cmdDur}}},
		{ID: "flow-001", Name: "Checkout", StartTime: now, Duration: &d2,
			Commands: []Command{{ID: "cmd-000", Type: "tapOn", Status: StatusFailed, Duration: &cmdDur,
				Error: &Error{Message: "Tap failed"}}}},
		{ID: "flow-002", Name: "Settings", StartTime: now, Commands: []Command{}},
	}

	writeTestReport(t, tmpDir, index, flows)

	if err := GenerateAllure(tmpDir); err != nil {
		t.Fatalf("GenerateAllure: %v", err)
	}

	allureDir := filepath.Join(tmpDir, "allure-results")

	// Verify all 3 result files exist
	for _, id := range []string{"flow-000", "flow-001", "flow-002"} {
		if _, err := os.Stat(filepath.Join(allureDir, id+"-result.json")); err != nil {
			t.Errorf("missing result file for %s", id)
		}
	}

	// Check statuses
	for _, tc := range []struct {
		id     string
		status string
	}{
		{"flow-000", "passed"},
		{"flow-001", "failed"},
		{"flow-002", "skipped"},
	} {
		data, _ := os.ReadFile(filepath.Join(allureDir, tc.id+"-result.json"))
		var result AllureResult
		json.Unmarshal(data, &result)
		if result.Status != tc.status {
			t.Errorf("%s status = %q, want %q", tc.id, result.Status, tc.status)
		}
	}
}

func TestAllureNestedSteps(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(5 * time.Second)
	d := int64(5000)
	cmdDur := int64(2000)
	subDur := int64(1000)

	index := &Index{
		Version: "1.0.0", Status: StatusPassed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:  Device{ID: "test", Name: "Pixel 6", Platform: "android"},
		Summary: Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{Index: 0, ID: "flow-000", Name: "Nested Test",
				SourceFile: "flows/nested.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed, Duration: &d, StartTime: &now, EndTime: &endTime},
		},
	}

	flow0 := FlowDetail{
		ID: "flow-000", Name: "Nested Test", StartTime: now, Duration: &d,
		Commands: []Command{
			{ID: "cmd-000", Type: "runFlow", Label: "Login Sub", Status: StatusPassed, Duration: &cmdDur,
				StartTime: &now, EndTime: &endTime,
				SubCommands: []Command{
					{ID: "sub-000", Type: "launchApp", Status: StatusPassed, Duration: &subDur,
						StartTime: &now, EndTime: &endTime},
					{ID: "sub-001", Type: "tapOn", Label: "Tap login", Status: StatusPassed, Duration: &subDur,
						StartTime: &now, EndTime: &endTime},
				}},
		},
	}

	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	if err := GenerateAllure(tmpDir); err != nil {
		t.Fatalf("GenerateAllure: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "allure-results", "flow-000-result.json"))
	var result AllureResult
	json.Unmarshal(data, &result)

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 top step, got %d", len(result.Steps))
	}
	topStep := result.Steps[0]
	if topStep.Name != "runFlow: Login Sub" {
		t.Errorf("top step name = %q", topStep.Name)
	}
	if len(topStep.Steps) != 2 {
		t.Fatalf("expected 2 sub-steps, got %d", len(topStep.Steps))
	}
	if topStep.Steps[1].Name != "tapOn: Tap login" {
		t.Errorf("sub-step name = %q", topStep.Steps[1].Name)
	}
}

func TestAllureScreenshotAttachments(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(5 * time.Second)
	d := int64(5000)
	cmdDur := int64(2500)

	// Create fake screenshots
	assetsDir := filepath.Join(tmpDir, "assets", "flow-000")
	os.MkdirAll(assetsDir, 0o755)
	os.WriteFile(filepath.Join(assetsDir, "cmd-000-before.png"), []byte("fake-png-before"), 0o644)
	os.WriteFile(filepath.Join(assetsDir, "cmd-000-after.png"), []byte("fake-png-after"), 0o644)

	index := &Index{
		Version: "1.0.0", Status: StatusPassed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:  Device{ID: "test", Name: "Pixel 6", Platform: "android"},
		Summary: Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{Index: 0, ID: "flow-000", Name: "Screenshot Test",
				SourceFile: "flows/screenshots.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed, Duration: &d, StartTime: &now, EndTime: &endTime},
		},
	}

	flow0 := FlowDetail{
		ID: "flow-000", Name: "Screenshot Test", StartTime: now, Duration: &d,
		Commands: []Command{
			{ID: "cmd-000", Type: "tapOn", Label: "Tap button", Status: StatusPassed, Duration: &cmdDur,
				StartTime: &now, EndTime: &endTime,
				Artifacts: CommandArtifacts{
					ScreenshotBefore: "assets/flow-000/cmd-000-before.png",
					ScreenshotAfter:  "assets/flow-000/cmd-000-after.png",
				}},
		},
	}

	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	if err := GenerateAllure(tmpDir); err != nil {
		t.Fatalf("GenerateAllure: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "allure-results", "flow-000-result.json"))
	var result AllureResult
	json.Unmarshal(data, &result)

	// Flow-level attachments
	if len(result.Attachments) != 2 {
		t.Fatalf("expected 2 flow attachments, got %d", len(result.Attachments))
	}
	if result.Attachments[0].Source != "cmd-000-before.png" {
		t.Errorf("attachment[0] source = %q", result.Attachments[0].Source)
	}
	if result.Attachments[1].Source != "cmd-000-after.png" {
		t.Errorf("attachment[1] source = %q", result.Attachments[1].Source)
	}
	if result.Attachments[0].Type != "image/png" {
		t.Errorf("attachment type = %q", result.Attachments[0].Type)
	}

	// Step-level attachments
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}
	if len(result.Steps[0].Attachments) != 2 {
		t.Fatalf("expected 2 step attachments, got %d", len(result.Steps[0].Attachments))
	}
	if result.Steps[0].Attachments[0].Name != "Before" {
		t.Errorf("step attachment[0] name = %q, want Before", result.Steps[0].Attachments[0].Name)
	}
	if result.Steps[0].Attachments[1].Name != "After" {
		t.Errorf("step attachment[1] name = %q, want After", result.Steps[0].Attachments[1].Name)
	}
}

func TestAllureCopyAttachments(t *testing.T) {
	tmpDir := t.TempDir()
	reportDir := filepath.Join(tmpDir, "report")
	allureDir := filepath.Join(tmpDir, "allure-results")
	os.MkdirAll(allureDir, 0o755)

	// Create fake screenshot
	assetsDir := filepath.Join(reportDir, "assets", "flow-000")
	os.MkdirAll(assetsDir, 0o755)
	os.WriteFile(filepath.Join(assetsDir, "cmd-000-after.png"), []byte("image-data"), 0o644)

	flows := []FlowDetail{
		{ID: "flow-000", Commands: []Command{
			{ID: "cmd-000", Type: "tapOn", Artifacts: CommandArtifacts{
				ScreenshotAfter: "assets/flow-000/cmd-000-after.png",
			}},
		}},
	}

	copyAllureAttachments(reportDir, allureDir, flows)

	// Check file was copied
	copied, err := os.ReadFile(filepath.Join(allureDir, "cmd-000-after.png"))
	if err != nil {
		t.Fatalf("copied file not found: %v", err)
	}
	if string(copied) != "image-data" {
		t.Errorf("copied content = %q", string(copied))
	}
}

func TestAllureCategoriesJSON(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(1 * time.Second)

	index := &Index{
		Version: "1.0.0", Status: StatusPassed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:  Device{ID: "test", Name: "Test", Platform: "android"},
		Summary: Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{Index: 0, ID: "flow-000", Name: "Test",
				SourceFile: "test.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed},
		},
	}

	flow0 := FlowDetail{ID: "flow-000", Name: "Test", StartTime: now, Commands: []Command{}}
	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	if err := GenerateAllure(tmpDir); err != nil {
		t.Fatalf("GenerateAllure: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "allure-results", "categories.json"))
	if err != nil {
		t.Fatalf("read categories.json: %v", err)
	}

	var categories []AllureCategory
	if err := json.Unmarshal(data, &categories); err != nil {
		t.Fatalf("unmarshal categories: %v", err)
	}

	if len(categories) != 8 {
		t.Errorf("expected 8 categories, got %d", len(categories))
	}

	// Verify some specific categories
	found := map[string]bool{}
	for _, c := range categories {
		found[c.Name] = true
		if len(c.MatchedStatuses) != 1 || c.MatchedStatuses[0] != "failed" {
			t.Errorf("category %q should match only 'failed'", c.Name)
		}
	}
	for _, name := range []string{"Element Not Found", "Timeout", "Assertion Failed"} {
		if !found[name] {
			t.Errorf("missing category %q", name)
		}
	}
}

func TestAllureEnvironmentProperties(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(1 * time.Second)

	index := &Index{
		Version: "1.0.0", Status: StatusPassed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:        Device{ID: "emu-5554", Name: "Pixel 6", Platform: "android", OSVersion: "13"},
		App:           App{ID: "com.example.app"},
		MaestroRunner: RunnerInfo{Version: "0.3.0", Driver: "uiautomator2"},
		Summary:       Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{Index: 0, ID: "flow-000", Name: "Test",
				SourceFile: "test.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed},
		},
	}

	flow0 := FlowDetail{ID: "flow-000", Name: "Test", StartTime: now, Commands: []Command{}}
	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	if err := GenerateAllure(tmpDir); err != nil {
		t.Fatalf("GenerateAllure: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "allure-results", "environment.properties"))
	if err != nil {
		t.Fatalf("read environment.properties: %v", err)
	}
	content := string(data)

	checks := []string{
		"framework=maestro",
		"device.name=Pixel 6",
		"device.platform=android",
		"device.osVersion=13",
		"runner.version=0.3.0",
		"runner.driver=uiautomator2",
		"app.id=com.example.app",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("environment.properties missing: %s\nGot:\n%s", check, content)
		}
	}
}

func TestAllureExecutorJSON(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(1 * time.Second)

	index := &Index{
		Version: "1.0.0", Status: StatusPassed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:  Device{ID: "test", Name: "Test", Platform: "android"},
		Summary: Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{Index: 0, ID: "flow-000", Name: "Test",
				SourceFile: "test.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed},
		},
	}

	flow0 := FlowDetail{ID: "flow-000", Name: "Test", StartTime: now, Commands: []Command{}}
	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	if err := GenerateAllure(tmpDir); err != nil {
		t.Fatalf("GenerateAllure: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "allure-results", "executor.json"))
	if err != nil {
		t.Fatalf("read executor.json: %v", err)
	}

	var exec AllureExecutor
	if err := json.Unmarshal(data, &exec); err != nil {
		t.Fatalf("unmarshal executor: %v", err)
	}

	if exec.Name != "DeviceLab" {
		t.Errorf("executor name = %q, want DeviceLab", exec.Name)
	}
	if exec.Type != "devicelab" {
		t.Errorf("executor type = %q, want devicelab", exec.Type)
	}
	if exec.ReportURL != "https://devicelab.dev" {
		t.Errorf("executor reportUrl = %q", exec.ReportURL)
	}
	if exec.ReportName != "Powered by DeviceLab" {
		t.Errorf("executor reportName = %q", exec.ReportName)
	}
}

func TestAllureLabels(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(1 * time.Second)
	d := int64(1000)

	index := &Index{
		Version: "1.0.0", Status: StatusPassed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:  Device{ID: "emulator-5554", Name: "Pixel 6", Platform: "android"},
		Summary: Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{Index: 0, ID: "flow-000", Name: "Login Test",
				SourceFile: "flows/login.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed, Duration: &d, StartTime: &now, EndTime: &endTime,
				Tags: []string{"smoke", "login"}},
		},
	}

	flow0 := FlowDetail{ID: "flow-000", Name: "Login Test", StartTime: now, Commands: []Command{}}
	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	if err := GenerateAllure(tmpDir); err != nil {
		t.Fatalf("GenerateAllure: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "allure-results", "flow-000-result.json"))
	var result AllureResult
	json.Unmarshal(data, &result)

	labelMap := map[string][]string{}
	for _, l := range result.Labels {
		labelMap[l.Name] = append(labelMap[l.Name], l.Value)
	}

	if v := labelMap["suite"]; len(v) != 1 || v[0] != "Login Test" {
		t.Errorf("suite label = %v", v)
	}
	if v := labelMap["parentSuite"]; len(v) != 1 || v[0] != "login.yaml" {
		t.Errorf("parentSuite label = %v", v)
	}
	if v := labelMap["framework"]; len(v) != 1 || v[0] != "maestro" {
		t.Errorf("framework label = %v", v)
	}
	if v := labelMap["severity"]; len(v) != 1 || v[0] != "normal" {
		t.Errorf("severity label = %v", v)
	}
	if v := labelMap["host"]; len(v) != 1 || v[0] != "Pixel 6" {
		t.Errorf("host label = %v", v)
	}
	if v := labelMap["thread"]; len(v) != 1 || v[0] != "emulator-5554" {
		t.Errorf("thread label = %v", v)
	}
	if v := labelMap["tag"]; len(v) != 2 {
		t.Errorf("expected 2 tag labels, got %v", v)
	}
}

func TestAllureHistoryIdDeterministic(t *testing.T) {
	// Same input should produce same hash
	h1 := fnv32aHash("Login Test:flows/login.yaml")
	h2 := fnv32aHash("Login Test:flows/login.yaml")
	if h1 != h2 {
		t.Errorf("historyId not deterministic: %s != %s", h1, h2)
	}

	// Different input should produce different hash
	h3 := fnv32aHash("Checkout:flows/checkout.yaml")
	if h1 == h3 {
		t.Errorf("different inputs produced same hash: %s", h1)
	}

	// Should be 8-char hex
	if len(h1) != 8 {
		t.Errorf("hash length = %d, want 8", len(h1))
	}
}

func TestAllureStatusMapping(t *testing.T) {
	tests := []struct {
		input    Status
		expected string
	}{
		{StatusPassed, "passed"},
		{StatusFailed, "failed"},
		{StatusSkipped, "skipped"},
		{StatusRunning, "unknown"},
		{StatusPending, "unknown"},
	}
	for _, tt := range tests {
		got := mapAllureStatus(tt.input)
		if got != tt.expected {
			t.Errorf("mapAllureStatus(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestGenerateAllureReportMissing(t *testing.T) {
	tmpDir := t.TempDir()
	err := GenerateAllure(tmpDir)
	if err == nil {
		t.Error("expected error when report.json missing")
	}
}

func TestAllureStopTimeFallbackFromDuration(t *testing.T) {
	// When EndTime is nil but StartTime and Duration are set, stopMs should be StartTime+Duration.
	now := time.Now()
	d := int64(3000)

	entry := &FlowEntry{
		Index: 0, ID: "flow-000", Name: "Duration Fallback",
		SourceFile: "flows/test.yaml", DataFile: "flows/flow-000.json",
		Status: StatusPassed, Duration: &d, StartTime: &now,
		// EndTime intentionally nil
	}
	index := &Index{
		Device: Device{ID: "emu", Name: "Pixel", Platform: "android"},
	}

	result := buildAllureResult(entry, nil, index, 0)

	expectedStop := now.UnixMilli() + d
	if result.Stop != expectedStop {
		t.Errorf("Stop = %d, want %d (StartTime + Duration)", result.Stop, expectedStop)
	}
}

func TestAllureStepWithoutLabel(t *testing.T) {
	// When Label is empty, step name should be just the Type.
	cmd := Command{
		ID: "cmd-000", Type: "launchApp", Status: StatusPassed,
		StartTime: ptrTime(time.Now()), EndTime: ptrTime(time.Now().Add(time.Second)),
	}

	step := buildAllureStep(cmd)
	if step.Name != "launchApp" {
		t.Errorf("step name = %q, want launchApp (no label)", step.Name)
	}
}

func TestAllureStepDurationFallback(t *testing.T) {
	// When EndTime is nil but StartTime and Duration are set.
	now := time.Now()
	d := int64(1500)

	cmd := Command{
		ID: "cmd-000", Type: "tapOn", Status: StatusPassed,
		StartTime: &now, Duration: &d,
		// EndTime intentionally nil
	}

	step := buildAllureStep(cmd)
	expectedStop := now.UnixMilli() + d
	if step.Stop != expectedStop {
		t.Errorf("step Stop = %d, want %d", step.Stop, expectedStop)
	}
}

func TestCopyFileSourceMissing(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "out.png")

	// copyFile silently ignores missing source
	copyFile("/nonexistent/path/file.png", dst)

	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("dst should not exist when source is missing")
	}
}

func TestCopyFileDestUnwritable(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src.png")
	os.WriteFile(src, []byte("data"), 0o644)

	// Point dst to a path inside a non-existent directory
	dst := filepath.Join(tmpDir, "nodir", "subdir", "out.png")

	// copyFile silently ignores create errors
	copyFile(src, dst)

	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("dst should not exist when directory doesn't exist")
	}
}

func TestGenerateAllureUnwritableDir(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(1 * time.Second)

	index := &Index{
		Version: "1.0.0", Status: StatusPassed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:  Device{ID: "test", Name: "Test", Platform: "android"},
		Summary: Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{Index: 0, ID: "flow-000", Name: "Test",
				SourceFile: "test.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed},
		},
	}
	flow0 := FlowDetail{ID: "flow-000", Name: "Test", StartTime: now, Commands: []Command{}}
	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	// Make the report dir read-only so allure-results can't be created
	allureDir := filepath.Join(tmpDir, "allure-results")
	// Create a file where the directory should be, so MkdirAll fails
	os.WriteFile(allureDir, []byte("block"), 0o444)

	err := GenerateAllure(tmpDir)
	if err == nil {
		t.Error("expected error when allure-results dir cannot be created")
	}
}

func TestAllureEnvironmentMinimalFields(t *testing.T) {
	// Test with empty device/runner fields to cover the "skip empty" branches.
	tmpDir := t.TempDir()
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatal(err)
	}

	index := &Index{
		Device:        Device{}, // all fields empty
		App:           App{},
		MaestroRunner: RunnerInfo{},
	}

	if err := writeAllureEnvironment(tmpDir, index); err != nil {
		t.Fatalf("writeAllureEnvironment: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "environment.properties"))
	content := string(data)

	if content != "framework=maestro\n" {
		t.Errorf("expected only framework line, got:\n%s", content)
	}
}

func TestAllureCopyAttachmentsWithSubcommands(t *testing.T) {
	tmpDir := t.TempDir()
	reportDir := filepath.Join(tmpDir, "report")
	allureDir := filepath.Join(tmpDir, "allure-results")
	os.MkdirAll(allureDir, 0o755)

	// Create screenshots in nested paths
	assetsDir := filepath.Join(reportDir, "assets", "flow-000")
	os.MkdirAll(assetsDir, 0o755)
	os.WriteFile(filepath.Join(assetsDir, "cmd-000-before.png"), []byte("before"), 0o644)
	os.WriteFile(filepath.Join(assetsDir, "sub-000-after.png"), []byte("sub-after"), 0o644)

	flows := []FlowDetail{
		{ID: "flow-000", Commands: []Command{
			{ID: "cmd-000", Type: "runFlow", Artifacts: CommandArtifacts{
				ScreenshotBefore: "assets/flow-000/cmd-000-before.png",
			}, SubCommands: []Command{
				{ID: "sub-000", Type: "tapOn", Artifacts: CommandArtifacts{
					ScreenshotAfter: "assets/flow-000/sub-000-after.png",
				}},
			}},
		}},
	}

	copyAllureAttachments(reportDir, allureDir, flows)

	// Both files should be copied flat
	if _, err := os.Stat(filepath.Join(allureDir, "cmd-000-before.png")); err != nil {
		t.Error("parent screenshot not copied")
	}
	if _, err := os.Stat(filepath.Join(allureDir, "sub-000-after.png")); err != nil {
		t.Error("subcommand screenshot not copied")
	}
}

func TestWriteAllureCategoriesUnwritable(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a read-only directory
	roDir := filepath.Join(tmpDir, "readonly")
	os.MkdirAll(roDir, 0o755)
	os.Chmod(roDir, 0o444)
	defer os.Chmod(roDir, 0o755)

	err := writeAllureCategories(roDir)
	if err == nil {
		t.Error("expected error writing to read-only dir")
	}
}

func TestWriteAllureExecutorUnwritable(t *testing.T) {
	tmpDir := t.TempDir()
	roDir := filepath.Join(tmpDir, "readonly")
	os.MkdirAll(roDir, 0o755)
	os.Chmod(roDir, 0o444)
	defer os.Chmod(roDir, 0o755)

	err := writeAllureExecutor(roDir)
	if err == nil {
		t.Error("expected error writing to read-only dir")
	}
}

func TestWriteAllureEnvironmentUnwritable(t *testing.T) {
	tmpDir := t.TempDir()
	roDir := filepath.Join(tmpDir, "readonly")
	os.MkdirAll(roDir, 0o755)
	os.Chmod(roDir, 0o444)
	defer os.Chmod(roDir, 0o755)

	index := &Index{Device: Device{Name: "Test"}}
	err := writeAllureEnvironment(roDir, index)
	if err == nil {
		t.Error("expected error writing to read-only dir")
	}
}

func TestGenerateAllureResultWriteError(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(1 * time.Second)

	index := &Index{
		Version: "1.0.0", Status: StatusPassed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:  Device{ID: "test", Name: "Test", Platform: "android"},
		Summary: Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{Index: 0, ID: "flow-000", Name: "Test",
				SourceFile: "test.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed, StartTime: &now, EndTime: &endTime},
		},
	}
	flow0 := FlowDetail{ID: "flow-000", Name: "Test", StartTime: now, Commands: []Command{}}
	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	// Create allure-results as a read-only dir so writing result files fails
	allureDir := filepath.Join(tmpDir, "allure-results")
	os.MkdirAll(allureDir, 0o755)
	os.Chmod(allureDir, 0o444)
	defer os.Chmod(allureDir, 0o755)

	err := GenerateAllure(tmpDir)
	if err == nil {
		t.Error("expected error when allure-results is read-only")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestAllurePerFlowDevice(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()
	endTime := now.Add(1 * time.Second)
	d := int64(1000)

	flowDevice := &Device{ID: "device-abc", Name: "iPhone 15 Pro", Platform: "ios"}

	index := &Index{
		Version: "1.0.0", Status: StatusPassed,
		StartTime: now, EndTime: &endTime, LastUpdated: now,
		Device:  Device{ID: "default", Name: "Default Device", Platform: "android"},
		Summary: Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{Index: 0, ID: "flow-000", Name: "Test",
				SourceFile: "test.yaml", DataFile: "flows/flow-000.json",
				Status: StatusPassed, Duration: &d, StartTime: &now, EndTime: &endTime,
				Device: flowDevice},
		},
	}

	flow0 := FlowDetail{ID: "flow-000", Name: "Test", StartTime: now, Commands: []Command{}}
	writeTestReport(t, tmpDir, index, []FlowDetail{flow0})

	if err := GenerateAllure(tmpDir); err != nil {
		t.Fatalf("GenerateAllure: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "allure-results", "flow-000-result.json"))
	var result AllureResult
	json.Unmarshal(data, &result)

	labelMap := map[string]string{}
	for _, l := range result.Labels {
		labelMap[l.Name] = l.Value
	}

	if labelMap["host"] != "iPhone 15 Pro" {
		t.Errorf("host = %q, want iPhone 15 Pro (per-flow device)", labelMap["host"])
	}
	if labelMap["thread"] != "device-abc" {
		t.Errorf("thread = %q, want device-abc (per-flow device)", labelMap["thread"])
	}
}
