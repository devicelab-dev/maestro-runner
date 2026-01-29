package cli

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v2"
)

func TestResolveOutputDir_Default(t *testing.T) {
	dir, err := resolveOutputDir("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(dir, "reports/") {
		t.Errorf("expected dir to start with reports/, got %s", dir)
	}
	// Should have timestamp subfolder
	parts := strings.Split(dir, "/")
	if len(parts) != 2 {
		t.Errorf("expected reports/<timestamp>, got %s", dir)
	}
}

func TestResolveOutputDir_CustomOutput(t *testing.T) {
	dir, err := resolveOutputDir("./my-reports", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(dir, "my-reports/") {
		t.Errorf("expected dir to start with my-reports/, got %s", dir)
	}
}

func TestResolveOutputDir_Flatten(t *testing.T) {
	dir, err := resolveOutputDir("./my-reports", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dir != "my-reports" {
		t.Errorf("expected my-reports, got %s", dir)
	}
}

func TestResolveOutputDir_FlattenWithoutOutput(t *testing.T) {
	_, err := resolveOutputDir("", true)
	if err == nil {
		t.Error("expected error when flatten is used without output")
	}

	if !strings.Contains(err.Error(), "--flatten requires --output") {
		t.Errorf("expected error about --flatten requiring --output, got: %v", err)
	}
}

func TestParseEnvVars_Valid(t *testing.T) {
	envs := []string{"USER=test", "PASS=secret", "EMPTY="}
	result := parseEnvVars(envs)

	if result["USER"] != "test" {
		t.Errorf("expected USER=test, got %s", result["USER"])
	}
	if result["PASS"] != "secret" {
		t.Errorf("expected PASS=secret, got %s", result["PASS"])
	}
	if result["EMPTY"] != "" {
		t.Errorf("expected EMPTY='', got %s", result["EMPTY"])
	}
}

func TestParseEnvVars_ValueWithEquals(t *testing.T) {
	envs := []string{"URL=http://example.com?foo=bar"}
	result := parseEnvVars(envs)

	if result["URL"] != "http://example.com?foo=bar" {
		t.Errorf("expected URL with equals in value, got %s", result["URL"])
	}
}

func TestParseEnvVars_InvalidFormat(t *testing.T) {
	envs := []string{"NOEQUALS"}
	result := parseEnvVars(envs)

	// Should be ignored
	if _, ok := result["NOEQUALS"]; ok {
		t.Error("expected NOEQUALS to be ignored")
	}
}

func TestParseEnvVars_Empty(t *testing.T) {
	result := parseEnvVars(nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}

	result = parseEnvVars([]string{})
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestRunConfig_Struct(t *testing.T) {
	cfg := &RunConfig{
		FlowPaths:   []string{"flow1.yaml", "flow2.yaml"},
		ConfigPath:  "config.yaml",
		Env:         map[string]string{"USER": "test"},
		IncludeTags: []string{"smoke"},
		ExcludeTags: []string{"wip"},
		OutputDir:   "./reports/test",
		Parallel:    2,
		Continuous:  true,
		Headless:    false,
		Platform:    "ios",
		Devices:     []string{"iPhone-15"},
		Verbose:     true,
		AppFile:     "app.ipa",
	}

	if len(cfg.FlowPaths) != 2 {
		t.Errorf("expected 2 flow paths, got %d", len(cfg.FlowPaths))
	}
	if cfg.Platform != "ios" {
		t.Errorf("expected platform ios, got %s", cfg.Platform)
	}
}

func TestGlobalFlags(t *testing.T) {
	if len(GlobalFlags) == 0 {
		t.Error("expected GlobalFlags to be defined")
	}

	// Check that required flags are present
	flagNames := make(map[string]bool)
	for _, f := range GlobalFlags {
		for _, name := range f.Names() {
			flagNames[name] = true
		}
	}

	requiredFlags := []string{"platform", "p", "device", "verbose", "app-file"}
	for _, name := range requiredFlags {
		if !flagNames[name] {
			t.Errorf("expected flag %q to be defined", name)
		}
	}
}

func TestTestCommand_NoArgs(t *testing.T) {
	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{testCommand},
	}

	// Test command requires at least one flow file
	err := app.Run([]string{"test-app", "test"})
	if err == nil {
		t.Error("expected error when no flow files provided")
	}
}

func TestStartDeviceCommand_NoPlatform(t *testing.T) {
	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{startDeviceCommand},
	}

	// Capture stdout to suppress output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	// start-device requires platform
	err := app.Run([]string{"test-app", "start-device"})
	if err == nil {
		t.Error("expected error when platform not provided")
	}
	if err != nil && !strings.Contains(err.Error(), "--platform is required") {
		t.Errorf("expected platform required error, got: %v", err)
	}
}

func TestHierarchyCommand(t *testing.T) {
	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{hierarchyCommand},
	}

	// Capture stdout to suppress output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	// hierarchy should work without args (not yet implemented, just prints)
	err := app.Run([]string{"test-app", "hierarchy"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHierarchyCommand_WithCompact(t *testing.T) {
	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{hierarchyCommand},
	}

	// Capture stdout to suppress output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	err := app.Run([]string{"test-app", "hierarchy", "--compact"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartDeviceCommand_WithPlatform(t *testing.T) {
	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{startDeviceCommand},
	}

	// Capture stdout to suppress output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	// With platform flag before command
	err := app.Run([]string{"test-app", "-p", "ios", "start-device"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartDeviceCommand_AllFlags(t *testing.T) {
	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{startDeviceCommand},
	}

	// Capture stdout to suppress output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	err := app.Run([]string{
		"test-app", "-p", "android", "start-device",
		"--os-version", "33",
		"--device-locale", "de_DE",
		"--force-create",
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecuteTest(t *testing.T) {
	// Create a temp directory with a test flow
	dir := t.TempDir()
	flowFile := dir + "/test.yaml"
	if err := os.WriteFile(flowFile, []byte(`- tapOn: "Button"`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture stdout to suppress output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	cfg := &RunConfig{
		FlowPaths: []string{flowFile},
		OutputDir: dir + "/reports",
		Platform:  "mock",
		Devices:   []string{"test-device"},
	}

	err := executeTest(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTestCommand_WithFlowFile(t *testing.T) {
	dir := t.TempDir()
	flowFile := dir + "/test.yaml"
	if err := os.WriteFile(flowFile, []byte(`- tapOn: "Button"`), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{testCommand},
	}

	// Capture stdout to suppress output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	err := app.Run([]string{"test-app", "-p", "mock", "test", flowFile})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTestCommand_WithAllFlags(t *testing.T) {
	dir := t.TempDir()
	flowFile := dir + "/test.yaml"
	// Flow with smoke tag to match include-tags filter
	flowContent := `tags:
  - smoke
---
- tapOn: "Button"`
	if err := os.WriteFile(flowFile, []byte(flowContent), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{testCommand},
	}

	// Capture stdout to suppress output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	// Note: global flags before command, command flags before positional args
	err := app.Run([]string{
		"test-app",
		"-p", "mock",
		"--device", "mock-device",
		"--verbose",
		"--app-file", "app.ipa",
		"test",
		"-e", "USER=test",
		"-e", "PASS=secret",
		"--include-tags", "smoke",
		"--exclude-tags", "wip",
		"--output", dir + "/reports",
		"--continuous",
		flowFile,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTestCommand_FlattenWithOutput(t *testing.T) {
	dir := t.TempDir()
	flowFile := dir + "/test.yaml"
	if err := os.WriteFile(flowFile, []byte(`- tapOn: "Button"`), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{testCommand},
	}

	// Capture stdout to suppress output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	// Note: global flags before command, command flags before positional args
	err := app.Run([]string{
		"test-app", "-p", "mock", "test",
		"--output", dir + "/reports",
		"--flatten",
		flowFile,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTestCommand_FlattenWithoutOutput(t *testing.T) {
	dir := t.TempDir()
	flowFile := dir + "/test.yaml"
	if err := os.WriteFile(flowFile, []byte(`- tapOn: "Button"`), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{testCommand},
	}

	// --flatten without --output should error
	// Note: flags must come before positional args
	err := app.Run([]string{
		"test-app", "test", "--flatten", flowFile,
	})
	if err == nil {
		t.Error("expected error when --flatten used without --output")
	}
}

func TestHierarchyCommand_WithDevice(t *testing.T) {
	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{hierarchyCommand},
	}

	// Capture stdout to suppress output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	err := app.Run([]string{"test-app", "--device", "emulator-5554", "hierarchy"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		ms       int64
		expected string
	}{
		{0, "0ms"},
		{50, "50ms"},
		{500, "500ms"},
		{999, "999ms"},
		{1000, "1.0s"},
		{1500, "1.5s"},
		{2126, "2.1s"},
		{10500, "10.5s"},
		{59999, "60.0s"},
		{60000, "1m 0s"},
		{61000, "1m 1s"},
		{90000, "1m 30s"},
		{120000, "2m 0s"},
		{125000, "2m 5s"},
	}

	for _, tc := range tests {
		result := formatDuration(tc.ms)
		if result != tc.expected {
			t.Errorf("formatDuration(%d) = %q, expected %q", tc.ms, result, tc.expected)
		}
	}
}

// Tests for loadCapabilities function

func TestLoadCapabilities_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	capsFile := dir + "/caps.json"
	capsContent := `{
		"platformName": "Android",
		"appium:automationName": "UiAutomator2",
		"appium:deviceName": "emulator-5554",
		"appium:app": "/path/to/app.apk"
	}`
	if err := os.WriteFile(capsFile, []byte(capsContent), 0o644); err != nil {
		t.Fatal(err)
	}

	caps, err := loadCapabilities(capsFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if caps["platformName"] != "Android" {
		t.Errorf("expected platformName=Android, got %v", caps["platformName"])
	}
	if caps["appium:automationName"] != "UiAutomator2" {
		t.Errorf("expected appium:automationName=UiAutomator2, got %v", caps["appium:automationName"])
	}
	if caps["appium:deviceName"] != "emulator-5554" {
		t.Errorf("expected appium:deviceName=emulator-5554, got %v", caps["appium:deviceName"])
	}
	if caps["appium:app"] != "/path/to/app.apk" {
		t.Errorf("expected appium:app=/path/to/app.apk, got %v", caps["appium:app"])
	}
}

func TestLoadCapabilities_WithCloudOptions(t *testing.T) {
	dir := t.TempDir()
	capsFile := dir + "/bstack.json"
	capsContent := `{
		"platformName": "Android",
		"appium:automationName": "UiAutomator2",
		"bstack:options": {
			"userName": "testuser",
			"accessKey": "testkey",
			"projectName": "Test Project"
		}
	}`
	if err := os.WriteFile(capsFile, []byte(capsContent), 0o644); err != nil {
		t.Fatal(err)
	}

	caps, err := loadCapabilities(capsFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if caps["platformName"] != "Android" {
		t.Errorf("expected platformName=Android, got %v", caps["platformName"])
	}

	bstackOpts, ok := caps["bstack:options"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected bstack:options to be a map, got %T", caps["bstack:options"])
	}
	if bstackOpts["userName"] != "testuser" {
		t.Errorf("expected userName=testuser, got %v", bstackOpts["userName"])
	}
	if bstackOpts["accessKey"] != "testkey" {
		t.Errorf("expected accessKey=testkey, got %v", bstackOpts["accessKey"])
	}
}

func TestLoadCapabilities_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	capsFile := dir + "/invalid.json"
	if err := os.WriteFile(capsFile, []byte(`{invalid json}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadCapabilities(capsFile)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse caps JSON") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestLoadCapabilities_FileNotFound(t *testing.T) {
	_, err := loadCapabilities("/nonexistent/caps.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "failed to read caps file") {
		t.Errorf("expected read error, got: %v", err)
	}
}

func TestLoadCapabilities_EmptyJSON(t *testing.T) {
	dir := t.TempDir()
	capsFile := dir + "/empty.json"
	if err := os.WriteFile(capsFile, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	caps, err := loadCapabilities(capsFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(caps) != 0 {
		t.Errorf("expected empty map, got %v", caps)
	}
}

// Test --caps flag is defined in GlobalFlags

func TestGlobalFlags_CapsFlag(t *testing.T) {
	flagNames := make(map[string]bool)
	for _, f := range GlobalFlags {
		for _, name := range f.Names() {
			flagNames[name] = true
		}
	}

	if !flagNames["caps"] {
		t.Error("expected --caps flag to be defined in GlobalFlags")
	}
}

// Test RunConfig with Capabilities

func TestRunConfig_WithCapabilities(t *testing.T) {
	caps := map[string]interface{}{
		"platformName":          "Android",
		"appium:automationName": "UiAutomator2",
	}

	cfg := &RunConfig{
		FlowPaths:    []string{"flow.yaml"},
		Platform:     "android",
		Devices:      []string{"emulator-5554"},
		Driver:       "appium",
		AppiumURL:    "http://localhost:4723",
		CapsFile:     "caps.json",
		Capabilities: caps,
	}

	if cfg.CapsFile != "caps.json" {
		t.Errorf("expected CapsFile=caps.json, got %s", cfg.CapsFile)
	}
	if cfg.Capabilities["platformName"] != "Android" {
		t.Errorf("expected platformName=Android, got %v", cfg.Capabilities["platformName"])
	}
}

// Test --caps flag parsing in test command

func TestTestCommand_WithCapsFlag(t *testing.T) {
	dir := t.TempDir()

	// Create flow file
	flowFile := dir + "/test.yaml"
	if err := os.WriteFile(flowFile, []byte(`- tapOn: "Button"`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create caps file
	capsFile := dir + "/caps.json"
	capsContent := `{"platformName": "Android", "appium:automationName": "UiAutomator2"}`
	if err := os.WriteFile(capsFile, []byte(capsContent), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{testCommand},
	}

	// Capture stdout to suppress output
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	// Test with --caps flag (using mock platform to avoid real driver creation)
	err := app.Run([]string{
		"test-app",
		"-p", "mock",
		"--caps", capsFile,
		"test",
		flowFile,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTestCommand_WithInvalidCapsFile(t *testing.T) {
	dir := t.TempDir()

	// Create flow file
	flowFile := dir + "/test.yaml"
	if err := os.WriteFile(flowFile, []byte(`- tapOn: "Button"`), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{testCommand},
	}

	// Test with nonexistent caps file
	err := app.Run([]string{
		"test-app",
		"-p", "mock",
		"--caps", "/nonexistent/caps.json",
		"test",
		flowFile,
	})
	if err == nil {
		t.Error("expected error for nonexistent caps file")
	}
}

// Tests for isPortInUse function

func TestIsPortInUse_AvailablePort(t *testing.T) {
	// Port 0 means the OS will assign an available port
	// We test with a high port that's very unlikely to be in use
	// Use a random high port in ephemeral range
	port := uint16(49152 + (time.Now().UnixNano() % 1000))

	// First check - port should be free
	inUse := isPortInUse(port)
	if inUse {
		t.Skipf("port %d already in use, skipping test", port)
	}
}

func TestIsPortInUse_PortInUse(t *testing.T) {
	// Start a listener on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer ln.Close()

	// Get the port that was assigned
	addr := ln.Addr().(*net.TCPAddr)
	port := uint16(addr.Port)

	// Now isPortInUse should return true
	inUse := isPortInUse(port)
	if !inUse {
		t.Errorf("expected port %d to be in use", port)
	}
}

func TestIsPortInUse_PortBecomesAvailable(t *testing.T) {
	// Start a listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	addr := ln.Addr().(*net.TCPAddr)
	port := uint16(addr.Port)

	// Port should be in use
	if !isPortInUse(port) {
		t.Error("expected port to be in use while listener is active")
	}

	// Close the listener
	ln.Close()

	// Give the OS a moment to release the port
	time.Sleep(10 * time.Millisecond)

	// Port should now be available
	if isPortInUse(port) {
		t.Skipf("port %d still in use after close (OS may have TIME_WAIT), skipping", port)
	}
}

// Tests for isSocketInUse function

func TestIsSocketInUse_NonExistentSocket(t *testing.T) {
	socketPath := "/tmp/test-socket-nonexistent.sock"
	os.Remove(socketPath) // Ensure it doesn't exist

	if isSocketInUse(socketPath) {
		t.Error("expected non-existent socket to not be in use")
	}
}

func TestIsSocketInUse_EmptyPath(t *testing.T) {
	if isSocketInUse("") {
		t.Error("expected empty socket path to not be in use")
	}
}

func TestIsSocketInUse_ActiveSocket(t *testing.T) {
	socketPath := "/tmp/test-socket-active-" + time.Now().Format("20060102150405") + ".sock"
	os.Remove(socketPath) // Clean up if exists

	// Create a listener on the socket
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to create socket listener: %v", err)
	}
	defer ln.Close()
	defer os.Remove(socketPath)

	// Socket should be in use
	if !isSocketInUse(socketPath) {
		t.Error("expected active socket to be in use")
	}
}

func TestIsSocketInUse_StaleSocket(t *testing.T) {
	socketPath := "/tmp/test-socket-stale-" + time.Now().Format("20060102150405") + ".sock"
	os.Remove(socketPath) // Clean up if exists

	// Create a socket file without a listener (stale)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to create socket: %v", err)
	}
	ln.Close() // Close immediately to make it stale

	// Give OS time to clean up
	time.Sleep(10 * time.Millisecond)

	// Stale socket should be detected as not in use (and cleaned up)
	if isSocketInUse(socketPath) {
		t.Error("expected stale socket to not be in use")
	}

	// Socket file should be removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("expected stale socket file to be removed")
		os.Remove(socketPath) // Clean up
	}
}
