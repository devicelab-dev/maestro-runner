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

	os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755)
	atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index)
	atomicWriteJSON(filepath.Join(tmpDir, "flows", "flow-000.json"), flowDetail)

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

	os.MkdirAll(filepath.Join(tmpDir, "flows"), 0o755)
	atomicWriteJSON(filepath.Join(tmpDir, "report.json"), index)

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
	os.WriteFile(pngPath, pngData, 0o644)

	result = loadAsBase64(pngPath)
	if !strings.HasPrefix(result, "data:image/png;base64,") {
		t.Errorf("expected base64 PNG, got: %s", result[:50])
	}

	// Test JPEG
	jpgPath := filepath.Join(tmpDir, "test.jpg")
	os.WriteFile(jpgPath, []byte{0xFF, 0xD8, 0xFF}, 0o644)
	result = loadAsBase64(jpgPath)
	if !strings.HasPrefix(result, "data:image/jpeg;base64,") {
		t.Error("expected base64 JPEG")
	}
}
