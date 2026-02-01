package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateHTML(t *testing.T) {
	// Create temp directory with test report
	tmpDir := t.TempDir()

	// Create test index
	now := time.Now()
	duration := int64(5000)
	index := &Index{
		Version:     "1.0.0",
		UpdateSeq:   1,
		Status:      StatusPassed,
		StartTime:   now,
		LastUpdated: now,
		Device: Device{
			ID:          "emulator-5554",
			Name:        "Pixel 6",
			Platform:    "android",
			OSVersion:   "14",
			IsSimulator: true,
		},
		App: App{
			ID:      "com.example.app",
			Name:    "TestApp",
			Version: "1.0.0",
		},
		MaestroRunner: RunnerInfo{
			Version: "0.1.0",
			Driver:  "uiautomator2",
		},
		Summary: Summary{
			Total:  1,
			Passed: 1,
		},
		Flows: []FlowEntry{
			{
				Index:      0,
				ID:         "flow-000",
				Name:       "Login Test",
				SourceFile: "flows/login.yaml",
				DataFile:   "flows/flow-000.json",
				AssetsDir:  "assets/flow-000",
				Status:     StatusPassed,
				Duration:   &duration,
				Commands: CommandSummary{
					Total:  2,
					Passed: 2,
				},
			},
		},
	}

	// Create test flow detail
	cmdDuration := int64(2500)
	flowDetail := FlowDetail{
		ID:         "flow-000",
		Name:       "Login Test",
		SourceFile: "flows/login.yaml",
		StartTime:  now,
		Duration:   &duration,
		Commands: []Command{
			{
				ID:       "cmd-000",
				Index:    0,
				Type:     "launchApp",
				YAML:     "- launchApp",
				Status:   StatusPassed,
				Duration: &cmdDuration,
			},
			{
				ID:       "cmd-001",
				Index:    1,
				Type:     "tapOn",
				YAML:     "- tapOn:\n    id: \"login_button\"",
				Status:   StatusPassed,
				Duration: &cmdDuration,
				Params: &CommandParams{
					Selector: &Selector{
						Type:  "id",
						Value: "login_button",
					},
				},
				Element: &Element{
					Found: true,
					ID:    "login_button",
					Class: "android.widget.Button",
					Bounds: &Bounds{
						X: 100, Y: 200, Width: 200, Height: 50,
					},
				},
			},
		},
	}

	// Write report files
	if err := os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755); err != nil {
		t.Fatalf("create flows dir: %v", err)
	}

	if err := atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index); err != nil {
		t.Fatalf("write index: %v", err)
	}

	if err := atomicWriteJSON(filepath.Join(tmpDir, "flows", "flow-000.json"), flowDetail); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	// Generate HTML
	outputPath := filepath.Join(tmpDir, "report.html")
	err := GenerateHTML(tmpDir, HTMLConfig{
		OutputPath: outputPath,
		Title:      "Test Report",
	})
	if err != nil {
		t.Fatalf("GenerateHTML: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("report.html not created")
	}

	// Read and verify content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read html: %v", err)
	}

	html := string(content)

	// Check for essential elements
	checks := []string{
		"<!DOCTYPE html>",
		"<title>Test Report</title>",
		"Login Test",
		"launchApp",
		"tapOn",
		"login_button",
		"Pixel 6",
		"android",
		"uiautomator2",
		"passed",
	}

	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("HTML missing expected content: %s", check)
		}
	}
}

func TestGenerateHTMLWithError(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now()
	duration := int64(30000)
	index := &Index{
		Version:     "1.0.0",
		Status:      StatusFailed,
		StartTime:   now,
		LastUpdated: now,
		Device: Device{
			ID:       "emulator-5554",
			Name:     "Pixel 6",
			Platform: "android",
		},
		App: App{ID: "com.example.app"},
		MaestroRunner: RunnerInfo{
			Version: "0.1.0",
			Driver:  "uiautomator2",
		},
		Summary: Summary{
			Total:  1,
			Failed: 1,
		},
		Flows: []FlowEntry{
			{
				Index:      0,
				ID:         "flow-000",
				Name:       "Login Test",
				SourceFile: "flows/login.yaml",
				DataFile:   "flows/flow-000.json",
				Status:     StatusFailed,
				Duration:   &duration,
				Commands: CommandSummary{
					Total:  1,
					Failed: 1,
				},
			},
		},
	}

	flowDetail := FlowDetail{
		ID:        "flow-000",
		Name:      "Login Test",
		StartTime: now,
		Duration:  &duration,
		Commands: []Command{
			{
				ID:       "cmd-000",
				Index:    0,
				Type:     "assertVisible",
				YAML:     "- assertVisible:\n    text: \"Welcome\"",
				Status:   StatusFailed,
				Duration: &duration,
				Error: &Error{
					Type:       "element_not_found",
					Message:    "Element with text 'Welcome' not found within 30000ms",
					Details:    "Searched for: text='Welcome'",
					Suggestion: "Check if the element text changed or if page loaded correctly",
				},
			},
		},
	}

	if err := os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755); err != nil {
		t.Fatalf("failed to create flows directory: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index); err != nil {
		t.Fatalf("failed to write report.json: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "flows", "flow-000.json"), flowDetail); err != nil {
		t.Fatalf("failed to write flow detail: %v", err)
	}

	outputPath := filepath.Join(tmpDir, "report.html")
	err := GenerateHTML(tmpDir, HTMLConfig{OutputPath: outputPath})
	if err != nil {
		t.Fatalf("GenerateHTML: %v", err)
	}

	content, _ := os.ReadFile(outputPath)
	html := string(content)

	// Check error content is present
	checks := []string{
		"element_not_found",
		"Element with text 'Welcome' not found",
		"Check if the element text changed",
		"failed",
	}

	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("HTML missing error content: %s", check)
		}
	}
}

func TestGenerateHTMLDefaultOutput(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now()
	index := &Index{
		Version:       "1.0.0",
		Status:        StatusPassed,
		StartTime:     now,
		LastUpdated:   now,
		Device:        Device{ID: "test", Name: "Test", Platform: "android"},
		App:           App{ID: "com.test"},
		MaestroRunner: RunnerInfo{Version: "0.1.0", Driver: "test"},
		Summary:       Summary{Total: 0},
		Flows:         []FlowEntry{},
	}

	if err := os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755); err != nil {
		t.Fatalf("failed to create flows directory: %v", err)
	}
	if err := atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index); err != nil {
		t.Fatalf("failed to write report.json: %v", err)
	}

	// Generate with no output path - should use default
	err := GenerateHTML(tmpDir, HTMLConfig{})
	if err != nil {
		t.Fatalf("GenerateHTML: %v", err)
	}

	// Check default output path
	defaultPath := filepath.Join(tmpDir, "report.html")
	if _, err := os.Stat(defaultPath); os.IsNotExist(err) {
		t.Error("expected report.html at default path")
	}
}

func TestGenerateHTMLReadError(t *testing.T) {
	tmpDir := t.TempDir()

	// No report.json - should fail
	err := GenerateHTML(tmpDir, HTMLConfig{})
	if err == nil {
		t.Error("expected error when report.json missing")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		ms       *int64
		expected string
	}{
		{nil, "-"},
		{ptr(int64(500)), "500ms"},
		{ptr(int64(1500)), "1.5s"},
		{ptr(int64(5000)), "5.0s"},
		{ptr(int64(65000)), "1m 5s"},
		{ptr(int64(120000)), "2m 0s"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.ms)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %s, want %s", tt.ms, result, tt.expected)
		}
	}
}

func ptr(i int64) *int64 {
	return &i
}

func TestBuildHTMLData(t *testing.T) {
	now := time.Now()
	endTime := now.Add(10 * time.Second)
	d1 := int64(5000)
	d2 := int64(10000)
	cmdDuration := int64(2500)

	index := &Index{
		Version:     Version,
		Status:      StatusFailed,
		StartTime:   now,
		EndTime:     &endTime,
		LastUpdated: now,
		Device: Device{
			ID:       "emulator-5554",
			Name:     "Pixel 7",
			Platform: "android",
		},
		App: App{ID: "com.example.app", Version: "2.0"},
		MaestroRunner: RunnerInfo{
			Version: "0.2.0",
			Driver:  "appium",
		},
		Summary: Summary{
			Total:  2,
			Passed: 1,
			Failed: 1,
		},
		Flows: []FlowEntry{
			{
				Index:    0,
				ID:       "flow-000",
				Name:     "Login",
				Status:   StatusPassed,
				Duration: &d1,
				Commands: CommandSummary{Total: 1, Passed: 1},
			},
			{
				Index:    1,
				ID:       "flow-001",
				Name:     "Checkout",
				Status:   StatusFailed,
				Duration: &d2,
				Commands: CommandSummary{Total: 1, Failed: 1},
			},
		},
	}

	flows := []FlowDetail{
		{
			ID:   "flow-000",
			Name: "Login",
			Commands: []Command{
				{
					ID:       "cmd-000",
					Type:     "launchApp",
					Status:   StatusPassed,
					Duration: &cmdDuration,
				},
			},
		},
		{
			ID:   "flow-001",
			Name: "Checkout",
			Commands: []Command{
				{
					ID:       "cmd-000",
					Type:     "tapOn",
					Status:   StatusFailed,
					Duration: &cmdDuration,
					Error: &Error{
						Type:    "element_not_found",
						Message: "button not found",
					},
				},
			},
		},
	}

	cfg := HTMLConfig{
		Title:     "Build Test Report",
		ReportDir: "/tmp/test-report",
	}

	data := buildHTMLData(index, flows, cfg)

	// Check title
	if data.Title != "Build Test Report" {
		t.Errorf("Title = %q, want %q", data.Title, "Build Test Report")
	}

	// Check pass rate
	expectedPassRate := 50.0
	if data.PassRate != expectedPassRate {
		t.Errorf("PassRate = %.1f, want %.1f", data.PassRate, expectedPassRate)
	}

	// Check max duration
	if data.MaxDuration != 10000 {
		t.Errorf("MaxDuration = %d, want 10000", data.MaxDuration)
	}

	// Check flows data
	if len(data.Flows) != 2 {
		t.Fatalf("len(Flows) = %d, want 2", len(data.Flows))
	}
	if data.Flows[0].StatusClass != "passed" {
		t.Errorf("Flows[0].StatusClass = %q, want %q", data.Flows[0].StatusClass, "passed")
	}
	if data.Flows[1].StatusClass != "failed" {
		t.Errorf("Flows[1].StatusClass = %q, want %q", data.Flows[1].StatusClass, "failed")
	}

	// Check duration percentage
	if data.Flows[0].DurationPct != 50.0 {
		t.Errorf("Flows[0].DurationPct = %.1f, want 50.0", data.Flows[0].DurationPct)
	}
	if data.Flows[1].DurationPct != 100.0 {
		t.Errorf("Flows[1].DurationPct = %.1f, want 100.0", data.Flows[1].DurationPct)
	}

	// Check status class map
	if data.StatusClass[StatusPassed] != "passed" {
		t.Errorf("StatusClass[passed] = %q, want %q", data.StatusClass[StatusPassed], "passed")
	}
	if data.StatusClass[StatusFailed] != "failed" {
		t.Errorf("StatusClass[failed] = %q, want %q", data.StatusClass[StatusFailed], "failed")
	}

	// Check JSON data is populated
	if len(data.JSONData) == 0 {
		t.Error("JSONData should not be empty")
	}

	// Check command HTML data
	if len(data.Flows[1].Commands) != 1 {
		t.Fatalf("len(Flows[1].Commands) = %d, want 1", len(data.Flows[1].Commands))
	}
	if data.Flows[1].Commands[0].StatusClass != "failed" {
		t.Errorf("Flows[1].Commands[0].StatusClass = %q, want %q", data.Flows[1].Commands[0].StatusClass, "failed")
	}
}

func TestBuildHTMLData_ZeroPassRate(t *testing.T) {
	now := time.Now()
	index := &Index{
		Version:     Version,
		Status:      StatusPending,
		StartTime:   now,
		LastUpdated: now,
		Device:      Device{ID: "test", Platform: "android"},
		App:         App{ID: "com.test"},
		MaestroRunner: RunnerInfo{
			Version: "0.1.0",
			Driver:  "test",
		},
		Summary: Summary{Total: 0},
		Flows:   []FlowEntry{},
	}

	data := buildHTMLData(index, []FlowDetail{}, HTMLConfig{Title: "Empty"})

	if data.PassRate != 0 {
		t.Errorf("PassRate = %.1f, want 0", data.PassRate)
	}
	if data.MaxDuration != 0 {
		t.Errorf("MaxDuration = %d, want 0", data.MaxDuration)
	}
}

func TestBuildHTMLData_WithScreenshots(t *testing.T) {
	now := time.Now()
	d := int64(1000)
	index := &Index{
		Version:     Version,
		Status:      StatusPassed,
		StartTime:   now,
		LastUpdated: now,
		Device:      Device{ID: "test", Platform: "ios"},
		App:         App{ID: "com.test"},
		MaestroRunner: RunnerInfo{
			Version: "0.1.0",
			Driver:  "test",
		},
		Summary: Summary{Total: 1, Passed: 1},
		Flows: []FlowEntry{
			{Index: 0, ID: "flow-000", Status: StatusPassed, Duration: &d, Commands: CommandSummary{Total: 1, Passed: 1}},
		},
	}

	flows := []FlowDetail{
		{
			ID: "flow-000",
			Commands: []Command{
				{
					ID:     "cmd-000",
					Type:   "tapOn",
					Status: StatusPassed,
					Artifacts: CommandArtifacts{
						ScreenshotBefore: "assets/flow-000/cmd-000-before.png",
						ScreenshotAfter:  "assets/flow-000/cmd-000-after.png",
					},
				},
			},
		},
	}

	// Without embed, paths are used directly
	data := buildHTMLData(index, flows, HTMLConfig{
		Title:       "Screenshot Test",
		EmbedAssets: false,
	})

	cmd := data.Flows[0].Commands[0]
	if !cmd.HasScreenshots {
		t.Error("expected HasScreenshots = true")
	}
	if cmd.ScreenshotBefore != "assets/flow-000/cmd-000-before.png" {
		t.Errorf("ScreenshotBefore = %q, want path", cmd.ScreenshotBefore)
	}
	if cmd.ScreenshotAfter != "assets/flow-000/cmd-000-after.png" {
		t.Errorf("ScreenshotAfter = %q, want path", cmd.ScreenshotAfter)
	}
}

func TestRenderHTML(t *testing.T) {
	data := HTMLData{
		Title:       "Render Test",
		GeneratedAt: "2025-01-01 12:00:00",
		Index: &Index{
			Version:     Version,
			Status:      StatusPassed,
			Device:      Device{ID: "test", Name: "TestDevice", Platform: "android"},
			App:         App{ID: "com.test"},
			MaestroRunner: RunnerInfo{Version: "0.1.0", Driver: "appium"},
			Summary:     Summary{Total: 1, Passed: 1},
			Flows:       []FlowEntry{},
		},
		Flows:         []FlowHTMLData{},
		TotalDuration: "5.0s",
		PassRate:      100.0,
		StatusClass: map[Status]string{
			StatusPassed:  "passed",
			StatusFailed:  "failed",
			StatusSkipped: "skipped",
			StatusRunning: "running",
			StatusPending: "pending",
		},
		JSONData: `{"index":{},"flows":[]}`,
	}

	html, err := renderHTML(data)
	if err != nil {
		t.Fatalf("renderHTML() error = %v", err)
	}

	// Verify HTML structure
	checks := []string{
		"<!DOCTYPE html>",
		"<title>Render Test</title>",
		"2025-01-01 12:00:00",
		"TestDevice",
		"android",
		"appium",
	}

	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("rendered HTML missing content: %q", check)
		}
	}
}

func TestRenderHTML_WithFlows(t *testing.T) {
	d := int64(3000)
	data := HTMLData{
		Title:       "Flow Render Test",
		GeneratedAt: "2025-01-01 12:00:00",
		Index: &Index{
			Version:     Version,
			Status:      StatusPassed,
			Device:      Device{ID: "test", Name: "Device", Platform: "android"},
			App:         App{ID: "com.test"},
			MaestroRunner: RunnerInfo{Version: "0.1.0", Driver: "test"},
			Summary:     Summary{Total: 1, Passed: 1},
			Flows: []FlowEntry{
				{Index: 0, ID: "flow-000", Name: "TestFlow", Status: StatusPassed, Duration: &d, Commands: CommandSummary{Total: 1, Passed: 1}},
			},
		},
		Flows: []FlowHTMLData{
			{
				FlowDetail: FlowDetail{
					ID:   "flow-000",
					Name: "TestFlow",
					Tags: []string{"smoke"},
				},
				StatusClass: "passed",
				DurationStr: "3.0s",
				DurationMs:  3000,
				DurationPct: 100.0,
				Commands: []CommandHTMLData{
					{
						Command:     Command{ID: "cmd-000", Type: "launchApp", Status: StatusPassed},
						StatusClass: "passed",
						DurationStr: "3.0s",
					},
				},
			},
		},
		TotalDuration: "3.0s",
		PassRate:      100.0,
		MaxDuration:   3000,
		StatusClass: map[Status]string{
			StatusPassed:  "passed",
			StatusFailed:  "failed",
			StatusSkipped: "skipped",
			StatusRunning: "running",
			StatusPending: "pending",
		},
		JSONData: `{"index":{},"flows":[]}`,
	}

	html, err := renderHTML(data)
	if err != nil {
		t.Fatalf("renderHTML() error = %v", err)
	}

	checks := []string{
		"TestFlow",
		"smoke",
		"Flow Render Test",
	}
	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("rendered HTML missing content: %q", check)
		}
	}
}

func TestLoadAsBase64(t *testing.T) {
	// Test with non-existent file
	result := loadAsBase64("/nonexistent/file.png")
	if result != "" {
		t.Error("expected empty string for non-existent file")
	}

	// Test with actual file
	tmpDir := t.TempDir()
	pngPath := filepath.Join(tmpDir, "test.png")
	// Minimal PNG (1x1 transparent pixel)
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	}
	if err := os.WriteFile(pngPath, pngData, 0o644); err != nil {
		t.Fatalf("failed to write PNG file: %v", err)
	}

	result = loadAsBase64(pngPath)
	if !strings.HasPrefix(result, "data:image/png;base64,") {
		t.Errorf("expected base64 PNG, got: %s", result[:50])
	}

	// Test JPEG
	jpgPath := filepath.Join(tmpDir, "test.jpg")
	if err := os.WriteFile(jpgPath, []byte{0xFF, 0xD8, 0xFF}, 0o644); err != nil {
		t.Fatalf("failed to write JPEG file: %v", err)
	}
	result = loadAsBase64(jpgPath)
	if !strings.HasPrefix(result, "data:image/jpeg;base64,") {
		t.Error("expected base64 JPEG")
	}
}
