package cli

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/device"
	"github.com/devicelab-dev/maestro-runner/pkg/emulator"
	"github.com/devicelab-dev/maestro-runner/pkg/executor"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
	"github.com/devicelab-dev/maestro-runner/pkg/simulator"
	"github.com/urfave/cli/v2"
)

// mockDriver implements core.Driver for testing helpers that take a Driver.
type mockDriver struct {
	platformInfo *core.PlatformInfo
}

func (m *mockDriver) Execute(flow.Step) *core.CommandResult { return &core.CommandResult{} }
func (m *mockDriver) Screenshot() ([]byte, error)           { return nil, nil }
func (m *mockDriver) Hierarchy() ([]byte, error)            { return nil, nil }
func (m *mockDriver) GetState() *core.StateSnapshot         { return nil }
func (m *mockDriver) GetPlatformInfo() *core.PlatformInfo   { return m.platformInfo }
func (m *mockDriver) SetFindTimeout(int)                    {}
func (m *mockDriver) SetWaitForIdleTimeout(int) error       { return nil }

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
	defer func() { _ = ln.Close() }()

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
	_ = ln.Close()

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
	_ = os.Remove(socketPath) // Ensure it doesn't exist

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
	_ = os.Remove(socketPath) // Clean up if exists

	// Create a listener on the socket
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to create socket listener: %v", err)
	}
	defer func() { _ = ln.Close() }()
	defer func() { _ = os.Remove(socketPath) }()

	// Socket should be in use
	if !isSocketInUse(socketPath) {
		t.Error("expected active socket to be in use")
	}
}

func TestIsSocketInUse_StaleSocket(t *testing.T) {
	socketPath := "/tmp/test-socket-stale-" + time.Now().Format("20060102150405") + ".sock"
	_ = os.Remove(socketPath) // Clean up if exists

	// Create a socket file without a listener (stale)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to create socket: %v", err)
	}
	_ = ln.Close() // Close immediately to make it stale

	// Give OS time to clean up
	time.Sleep(10 * time.Millisecond)

	// Stale socket should be detected as not in use (and cleaned up)
	if isSocketInUse(socketPath) {
		t.Error("expected stale socket to not be in use")
	}

	// Socket file should be removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("expected stale socket file to be removed")
		_ = os.Remove(socketPath) // Clean up
	}
}

// Tests for bootTimeout helper

func TestBootTimeout_Default(t *testing.T) {
	cfg := &RunConfig{BootTimeout: 0}
	timeout := bootTimeout(cfg)
	if timeout != 180*time.Second {
		t.Errorf("bootTimeout(0) = %v, want 180s", timeout)
	}
}

func TestBootTimeout_Custom(t *testing.T) {
	cfg := &RunConfig{BootTimeout: 60}
	timeout := bootTimeout(cfg)
	if timeout != 60*time.Second {
		t.Errorf("bootTimeout(60) = %v, want 60s", timeout)
	}
}

func TestBootTimeout_Large(t *testing.T) {
	cfg := &RunConfig{BootTimeout: 300}
	timeout := bootTimeout(cfg)
	if timeout != 300*time.Second {
		t.Errorf("bootTimeout(300) = %v, want 300s", timeout)
	}
}

// Tests for getFirstDevice helper

func TestGetFirstDevice_WithDevices(t *testing.T) {
	cfg := &RunConfig{Devices: []string{"emulator-5554", "emulator-5556"}}
	result := getFirstDevice(cfg)
	if result != "emulator-5554" {
		t.Errorf("getFirstDevice() = %q, want %q", result, "emulator-5554")
	}
}

func TestGetFirstDevice_NoDevices(t *testing.T) {
	cfg := &RunConfig{Devices: nil}
	result := getFirstDevice(cfg)
	if result != "" {
		t.Errorf("getFirstDevice() = %q, want empty string", result)
	}
}

func TestGetFirstDevice_EmptySlice(t *testing.T) {
	cfg := &RunConfig{Devices: []string{}}
	result := getFirstDevice(cfg)
	if result != "" {
		t.Errorf("getFirstDevice() = %q, want empty string", result)
	}
}

// Tests for parseDevices

func TestParseDevices_SingleDevice(t *testing.T) {
	devices := parseDevices("emulator-5554")
	if len(devices) != 1 || devices[0] != "emulator-5554" {
		t.Errorf("parseDevices single device = %v, want [emulator-5554]", devices)
	}
}

func TestParseDevices_MultipleDevices(t *testing.T) {
	devices := parseDevices("emulator-5554, emulator-5556")
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if devices[0] != "emulator-5554" {
		t.Errorf("devices[0] = %q, want %q", devices[0], "emulator-5554")
	}
	if devices[1] != "emulator-5556" {
		t.Errorf("devices[1] = %q, want %q", devices[1], "emulator-5556")
	}
}

func TestParseDevices_EmptyFlag(t *testing.T) {
	devices := parseDevices("")
	if devices != nil {
		t.Errorf("parseDevices empty flag = %v, want nil", devices)
	}
}

func TestParseDevices_NoFlagsSet(t *testing.T) {
	devices := parseDevices("")
	if devices != nil {
		t.Errorf("parseDevices no flags = %v, want nil", devices)
	}
}

func TestParseDevices_WhitespaceHandling(t *testing.T) {
	devices := parseDevices("  device1  ,  device2  ")
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if devices[0] != "device1" {
		t.Errorf("devices[0] = %q, want %q", devices[0], "device1")
	}
	if devices[1] != "device2" {
		t.Errorf("devices[1] = %q, want %q", devices[1], "device2")
	}
}

// Tests for resolveDriverName

func TestResolveDriverName(t *testing.T) {
	tests := []struct {
		name     string
		driver   string
		platform string
		expected string
	}{
		{"default android", "", "android", "uiautomator2"},
		{"default ios", "", "ios", "wda"},
		{"explicit uiautomator2 android", "uiautomator2", "android", "uiautomator2"},
		{"explicit uiautomator2 ios overrides to wda", "uiautomator2", "ios", "wda"},
		{"appium android", "appium", "android", "appium"},
		{"appium ios", "appium", "ios", "appium"},
		{"mock platform", "", "mock", "mock"},
		{"case insensitive ios", "", "iOS", "wda"},
		{"case insensitive appium", "Appium", "android", "appium"},
		{"empty both", "", "", "uiautomator2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RunConfig{Driver: tt.driver}
			result := resolveDriverName(cfg, tt.platform)
			if result != tt.expected {
				t.Errorf("resolveDriverName(driver=%q, platform=%q) = %q, want %q",
					tt.driver, tt.platform, result, tt.expected)
			}
		})
	}
}

// Tests for buildDeviceReport and buildAppReport

func TestBuildDeviceReport(t *testing.T) {
	driver := &mockDriver{
		platformInfo: &core.PlatformInfo{
			DeviceID:    "test-device-123",
			Platform:    "android",
			DeviceName:  "Pixel 7",
			OSVersion:   "33",
			IsSimulator: false,
		},
	}

	d := buildDeviceReport(driver)
	if d.ID != "test-device-123" {
		t.Errorf("ID = %q, want %q", d.ID, "test-device-123")
	}
	if d.Platform != "android" {
		t.Errorf("Platform = %q, want %q", d.Platform, "android")
	}
	if d.Name != "Pixel 7" {
		t.Errorf("Name = %q, want %q", d.Name, "Pixel 7")
	}
	if d.OSVersion != "33" {
		t.Errorf("OSVersion = %q, want %q", d.OSVersion, "33")
	}
	if d.IsSimulator {
		t.Error("IsSimulator should be false")
	}
}

func TestBuildAppReport(t *testing.T) {
	driver := &mockDriver{
		platformInfo: &core.PlatformInfo{
			AppID:      "com.example.app",
			AppVersion: "2.1.0",
		},
	}

	a := buildAppReport(driver)
	if a.ID != "com.example.app" {
		t.Errorf("ID = %q, want %q", a.ID, "com.example.app")
	}
	if a.Version != "2.1.0" {
		t.Errorf("Version = %q, want %q", a.Version, "2.1.0")
	}
}

// Test executeTest with ShutdownAfter flag

func TestExecuteTest_WithShutdownAfter(t *testing.T) {
	dir := t.TempDir()
	flowFile := dir + "/test.yaml"
	if err := os.WriteFile(flowFile, []byte(`- tapOn: "Button"`), 0o644); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	cfg := &RunConfig{
		FlowPaths:     []string{flowFile},
		OutputDir:     dir + "/reports",
		Platform:      "mock",
		Devices:       []string{"test-device"},
		ShutdownAfter: true,
	}

	err := executeTest(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// Test color function

func TestColor_Enabled(t *testing.T) {
	oldEnabled := colorsEnabled
	defer func() { colorsEnabled = oldEnabled }()

	colorsEnabled = true
	result := color(colorGreen)
	if result != colorGreen {
		t.Errorf("color(colorGreen) with colors enabled = %q, want %q", result, colorGreen)
	}
}

func TestColor_Disabled(t *testing.T) {
	oldEnabled := colorsEnabled
	defer func() { colorsEnabled = oldEnabled }()

	colorsEnabled = false
	result := color(colorGreen)
	if result != "" {
		t.Errorf("color(colorGreen) with colors disabled = %q, want empty string", result)
	}
}

// ============================================================
// Tests for enhanceNoDevicesError
// ============================================================

func TestEnhanceNoDevicesError_BasicAutoStart(t *testing.T) {
	// Save and restore os.Args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"maestro-runner", "test", "flow.yaml"}

	noDevErr := &device.NoDevicesError{
		Message: "No Android devices or emulators found",
		Suggestions: []string{
			"Auto-start first AVD: maestro-runner --auto-start-emulator <flow>",
		},
	}
	cfg := &RunConfig{}

	enhanceNoDevicesError(noDevErr, cfg)

	expected := "Auto-start first AVD: maestro-runner --auto-start-emulator test flow.yaml"
	if noDevErr.Suggestions[0] != expected {
		t.Errorf("enhanceNoDevicesError auto-start suggestion:\n  got:  %q\n  want: %q", noDevErr.Suggestions[0], expected)
	}
}

func TestEnhanceNoDevicesError_StartEmulatorWithAVD(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"maestro-runner", "--platform", "android", "test", "flows/"}

	noDevErr := &device.NoDevicesError{
		Message: "No Android devices or emulators found",
		Suggestions: []string{
			"Start specific AVD: maestro-runner --start-emulator Pixel_7_API_33 <flow>",
		},
	}
	cfg := &RunConfig{}

	enhanceNoDevicesError(noDevErr, cfg)

	expected := "Start specific AVD: maestro-runner --platform android --start-emulator Pixel_7_API_33 test flows/"
	if noDevErr.Suggestions[0] != expected {
		t.Errorf("enhanceNoDevicesError start-emulator suggestion:\n  got:  %q\n  want: %q", noDevErr.Suggestions[0], expected)
	}
}

func TestEnhanceNoDevicesError_ParallelSuggestion(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"maestro-runner", "test", "flows/"}

	noDevErr := &device.NoDevicesError{
		Message: "No Android devices or emulators found",
		Suggestions: []string{
			"Run in parallel on 2 emulators: maestro-runner --parallel 2 --auto-start-emulator <flows>",
		},
	}
	cfg := &RunConfig{}

	enhanceNoDevicesError(noDevErr, cfg)

	expected := "Run in parallel on 2 emulators: maestro-runner --parallel 2 --auto-start-emulator test flows/"
	if noDevErr.Suggestions[0] != expected {
		t.Errorf("enhanceNoDevicesError parallel suggestion:\n  got:  %q\n  want: %q", noDevErr.Suggestions[0], expected)
	}
}

func TestEnhanceNoDevicesError_NoTestSubcommand(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// When there is no "test" subcommand in args
	os.Args = []string{"maestro-runner", "flow.yaml"}

	noDevErr := &device.NoDevicesError{
		Message: "No Android devices or emulators found",
		Suggestions: []string{
			"Auto-start first AVD: maestro-runner --auto-start-emulator <flow>",
		},
	}
	cfg := &RunConfig{}

	enhanceNoDevicesError(noDevErr, cfg)

	// When no "test" found, globalPart = entire args joined, testPart = ""
	expected := "Auto-start first AVD: maestro-runner flow.yaml --auto-start-emulator"
	if noDevErr.Suggestions[0] != expected {
		t.Errorf("enhanceNoDevicesError no-test suggestion:\n  got:  %q\n  want: %q", noDevErr.Suggestions[0], expected)
	}
}

func TestEnhanceNoDevicesError_NoMatchingSuggestions(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"maestro-runner", "test", "flow.yaml"}

	noDevErr := &device.NoDevicesError{
		Message: "No Android devices or emulators found",
		Suggestions: []string{
			"Connect a physical device via USB (enable USB debugging)",
		},
	}
	cfg := &RunConfig{}

	enhanceNoDevicesError(noDevErr, cfg)

	// Suggestion that does not match any pattern should remain unchanged
	if noDevErr.Suggestions[0] != "Connect a physical device via USB (enable USB debugging)" {
		t.Errorf("non-matching suggestion should remain unchanged, got: %q", noDevErr.Suggestions[0])
	}
}

func TestEnhanceNoDevicesError_GlobalFlagsBeforeTest(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"maestro-runner", "--platform", "android", "--verbose", "test", "-e", "USER=test", "flow.yaml"}

	noDevErr := &device.NoDevicesError{
		Message: "No Android devices or emulators found",
		Suggestions: []string{
			"Auto-start first AVD: maestro-runner --auto-start-emulator <flow>",
		},
	}
	cfg := &RunConfig{}

	enhanceNoDevicesError(noDevErr, cfg)

	expected := "Auto-start first AVD: maestro-runner --platform android --verbose --auto-start-emulator test -e USER=test flow.yaml"
	if noDevErr.Suggestions[0] != expected {
		t.Errorf("enhanceNoDevicesError with global flags:\n  got:  %q\n  want: %q", noDevErr.Suggestions[0], expected)
	}
}

func TestEnhanceNoDevicesError_EmptySuggestions(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"maestro-runner", "test", "flow.yaml"}

	noDevErr := &device.NoDevicesError{
		Message:     "No Android devices or emulators found",
		Suggestions: []string{},
	}
	cfg := &RunConfig{}

	// Should not panic on empty suggestions
	enhanceNoDevicesError(noDevErr, cfg)

	if len(noDevErr.Suggestions) != 0 {
		t.Errorf("expected empty suggestions, got %d", len(noDevErr.Suggestions))
	}
}

// ============================================================
// Tests for bootTimeout (table-driven)
// ============================================================

func TestBootTimeout_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected time.Duration
	}{
		{"zero defaults to 180s", 0, 180 * time.Second},
		{"custom 60s", 60, 60 * time.Second},
		{"custom 300s", 300, 300 * time.Second},
		{"custom 1s", 1, 1 * time.Second},
		{"large value 3600s", 3600, 3600 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RunConfig{BootTimeout: tt.input}
			result := bootTimeout(cfg)
			if result != tt.expected {
				t.Errorf("bootTimeout(%d) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// ============================================================
// Tests for handleEmulatorStartup
// ============================================================

func TestHandleEmulatorStartup_NonAndroidPlatform(t *testing.T) {
	// Should return nil immediately for non-android platforms
	cfg := &RunConfig{Platform: "ios"}
	mgr := emulator.NewManager()

	err := handleEmulatorStartup(cfg, mgr)
	if err != nil {
		t.Errorf("handleEmulatorStartup for ios should return nil, got: %v", err)
	}
}

func TestHandleEmulatorStartup_NoFlags(t *testing.T) {
	// When neither StartEmulator nor AutoStartEmulator are set
	cfg := &RunConfig{
		Platform:          "android",
		StartEmulator:     "",
		AutoStartEmulator: false,
	}
	mgr := emulator.NewManager()

	// Suppress stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	err := handleEmulatorStartup(cfg, mgr)
	if err != nil {
		t.Errorf("handleEmulatorStartup with no flags should return nil, got: %v", err)
	}
}

func TestHandleEmulatorStartup_EmptyPlatform(t *testing.T) {
	// Empty platform should be treated as Android (not skipped)
	cfg := &RunConfig{
		Platform:          "",
		StartEmulator:     "",
		AutoStartEmulator: false,
	}
	mgr := emulator.NewManager()

	err := handleEmulatorStartup(cfg, mgr)
	if err != nil {
		t.Errorf("handleEmulatorStartup with empty platform should return nil, got: %v", err)
	}
}

// ============================================================
// Tests for determineExecutionMode
// ============================================================

func TestDetermineExecutionMode_SingleDevice(t *testing.T) {
	// Suppress stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	cfg := &RunConfig{
		Parallel:  0,
		Devices:   nil,
		OutputDir: t.TempDir(),
	}
	mgr := emulator.NewManager()

	needsParallel, deviceIDs, err := determineExecutionMode(cfg, mgr, simulator.NewManager())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if needsParallel {
		t.Error("expected needsParallel=false for single device mode")
	}
	if len(deviceIDs) != 0 {
		t.Errorf("expected empty deviceIDs, got %v", deviceIDs)
	}
}

func TestDetermineExecutionMode_ExplicitDevices(t *testing.T) {
	// Suppress stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	cfg := &RunConfig{
		Parallel:  0,
		Devices:   []string{"emulator-5554", "emulator-5556"},
		OutputDir: t.TempDir(),
	}
	mgr := emulator.NewManager()

	needsParallel, deviceIDs, err := determineExecutionMode(cfg, mgr, simulator.NewManager())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !needsParallel {
		t.Error("expected needsParallel=true when multiple devices specified")
	}
	if len(deviceIDs) != 2 {
		t.Errorf("expected 2 deviceIDs, got %d", len(deviceIDs))
	}
}

func TestDetermineExecutionMode_ParallelWithoutAutoStart(t *testing.T) {
	// Suppress stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	cfg := &RunConfig{
		Parallel:          2,
		Devices:           nil,
		AutoStartEmulator: false,
		Platform:          "android",
		OutputDir:         t.TempDir(),
	}
	mgr := emulator.NewManager()

	// This should fail because no devices found and auto-start disabled
	_, _, err := determineExecutionMode(cfg, mgr, simulator.NewManager())
	if err == nil {
		t.Error("expected error when parallel requested with no devices and auto-start disabled")
	}
}

func TestDetermineExecutionMode_SingleExplicitDevice(t *testing.T) {
	// Single device in Devices slice, Parallel=0 => not parallel
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	cfg := &RunConfig{
		Parallel:  0,
		Devices:   []string{"emulator-5554"},
		OutputDir: t.TempDir(),
	}
	mgr := emulator.NewManager()

	needsParallel, _, err := determineExecutionMode(cfg, mgr, simulator.NewManager())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if needsParallel {
		t.Error("expected needsParallel=false for single explicit device")
	}
}

// ============================================================
// Tests for executeFlowsWithMode
// ============================================================

func TestExecuteFlowsWithMode_AppiumParallel(t *testing.T) {
	cfg := &RunConfig{
		Driver: "appium",
	}
	_, err := executeFlowsWithMode(cfg, nil, true, []string{"d1", "d2"})
	if err == nil {
		t.Error("expected error for parallel Appium execution")
	}
	if !strings.Contains(err.Error(), "parallel execution not yet supported for Appium") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================
// Tests for handleEmulatorStartup with StartEmulator set (error path)
// ============================================================

func TestHandleEmulatorStartup_StartEmulatorError(t *testing.T) {
	// Suppress stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	cfg := &RunConfig{
		Platform:      "android",
		StartEmulator: "NonExistent_AVD_12345",
		BootTimeout:   5,
	}
	mgr := emulator.NewManager()

	err := handleEmulatorStartup(cfg, mgr)
	// This will fail because the AVD does not exist (emulator binary may not be found)
	if err == nil {
		t.Error("expected error when starting nonexistent emulator")
	}
}

// ============================================================
// Tests for executeTest signal handling setup
// ============================================================

func TestExecuteTest_SignalSetup(t *testing.T) {
	// Verify executeTest sets up signal handling without crashing
	// We test this by running a successful mock test which goes through
	// the signal setup code path.
	dir := t.TempDir()
	flowFile := dir + "/test.yaml"
	if err := os.WriteFile(flowFile, []byte(`- tapOn: "Button"`), 0o644); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	cfg := &RunConfig{
		FlowPaths:     []string{flowFile},
		OutputDir:     dir + "/reports",
		Platform:      "mock",
		Devices:       []string{"test-device"},
		ShutdownAfter: false,
	}

	err := executeTest(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================
// Tests for printSummary (does not crash)
// ============================================================

func TestPrintSummary_NoCrash(t *testing.T) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	result := &executor.RunResult{
		TotalFlows:  2,
		PassedFlows: 1,
		FailedFlows: 1,
		Status:      report.StatusFailed,
		Duration:    5000,
		FlowResults: []executor.FlowResult{
			{
				Name:         "test-flow-1",
				Status:       report.StatusPassed,
				StepsTotal:   3,
				StepsPassed:  3,
				StepsFailed:  0,
				StepsSkipped: 0,
				Duration:     2000,
			},
			{
				Name:         "very-long-test-flow-name-that-exceeds-42-characters-for-truncation",
				Status:       report.StatusFailed,
				StepsTotal:   5,
				StepsPassed:  2,
				StepsFailed:  3,
				StepsSkipped: 0,
				Duration:     3000,
				Error:        "some error",
			},
		},
	}

	// Should not panic
	printSummary(result)
}

func TestPrintSummary_WithSkipped(t *testing.T) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	result := &executor.RunResult{
		TotalFlows:   1,
		PassedFlows:  0,
		FailedFlows:  0,
		SkippedFlows: 1,
		Status:       report.StatusPassed,
		Duration:     500,
		FlowResults: []executor.FlowResult{
			{
				Name:         "skipped-flow",
				Status:       report.StatusSkipped,
				StepsTotal:   0,
				StepsSkipped: 0,
				Duration:     0,
			},
		},
	}

	printSummary(result)
}

// ============================================================
// Tests for callback functions (no panics)
// ============================================================

func TestOnFlowStart_NoCrash(t *testing.T) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	onFlowStart(0, 5, "Login Flow", "login.yaml")
}

func TestOnStepComplete_Passed(t *testing.T) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	onStepComplete(0, "tapOn: button", true, 100, "")
}

func TestOnStepComplete_Failed(t *testing.T) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	onStepComplete(0, "tapOn: button", false, 100, "element not found")
}

func TestOnStepComplete_Slow(t *testing.T) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	// Should show slow warning (>5000ms)
	onStepComplete(0, "tapOn: button", true, 6000, "")
}

func TestOnStepComplete_CompoundStepNotSlow(t *testing.T) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	// Compound steps (runFlow, repeat, retry) should not show slow warning
	onStepComplete(0, "runFlow: login", true, 10000, "")
	onStepComplete(1, "repeat: 3 times", true, 15000, "")
	onStepComplete(2, "retry: 2 times", true, 8000, "")
}

func TestOnNestedFlowStart_NoCrash(t *testing.T) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	onNestedFlowStart(0, "nested flow")
	onNestedFlowStart(1, "deeply nested flow")
}

func TestOnNestedStep_PassedAndFailed(t *testing.T) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	onNestedStep(0, "tapOn: nested button", true, 50, "")
	onNestedStep(0, "tapOn: nested button", false, 50, "element not found")
	// Slow nested step
	onNestedStep(1, "scrollDown", true, 6000, "")
}

func TestOnFlowEnd_PassedAndFailed(t *testing.T) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	onFlowEnd("Login Flow", true, 2000, "")
	onFlowEnd("Checkout Flow", false, 5000, "step failed")
}

// ============================================================
// Tests for exported library functions
// ============================================================

// TestCreateDriver_Mock tests CreateDriver with mock platform
func TestCreateDriver_Mock(t *testing.T) {
	cfg := &RunConfig{
		Platform: "mock",
		Devices:  []string{"mock-device"},
	}

	driver, cleanup, err := CreateDriver(cfg)
	if err != nil {
		t.Fatalf("CreateDriver failed: %v", err)
	}
	defer cleanup()

	if driver == nil {
		t.Fatal("expected driver to be non-nil")
	}

	info := driver.GetPlatformInfo()
	if info == nil {
		t.Fatal("expected platform info to be non-nil")
	}
}

// TestCreateDriver_MockAutoDetect tests CreateDriver with mock platform and no device specified
func TestCreateDriver_MockAutoDetect(t *testing.T) {
	cfg := &RunConfig{
		Platform: "mock",
		Devices:  nil, // Auto-detect
	}

	driver, cleanup, err := CreateDriver(cfg)
	if err != nil {
		t.Fatalf("CreateDriver failed: %v", err)
	}
	defer cleanup()

	if driver == nil {
		t.Fatal("expected driver to be non-nil")
	}
}

// TestCreateDriver_UnsupportedPlatform tests CreateDriver with unsupported platform
func TestCreateDriver_UnsupportedPlatform(t *testing.T) {
	cfg := &RunConfig{
		Platform: "unsupported",
	}

	_, _, err := CreateDriver(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported platform")
	}
	if !strings.Contains(err.Error(), "unsupported platform") {
		t.Errorf("expected 'unsupported platform' error, got: %v", err)
	}
}

// TestExecuteFlowWithDriver_Mock tests ExecuteFlowWithDriver with mock driver
func TestExecuteFlowWithDriver_Mock(t *testing.T) {
	dir := t.TempDir()

	// Create a test flow file
	flowFile := dir + "/test_flow.yaml"
	flowContent := `- tapOn: "Button"`
	if err := os.WriteFile(flowFile, []byte(flowContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Suppress stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	// Create driver
	cfg := &RunConfig{
		Platform:  "mock",
		Devices:   []string{"mock-device"},
		OutputDir: dir + "/reports",
	}

	driver, cleanup, err := CreateDriver(cfg)
	if err != nil {
		t.Fatalf("CreateDriver failed: %v", err)
	}
	defer cleanup()

	// Parse flow
	f, err := flow.ParseFile(flowFile)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Execute flow
	result, err := ExecuteFlowWithDriver(driver, cfg, *f)
	if err != nil {
		t.Fatalf("ExecuteFlowWithDriver failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be non-nil")
	}
	if result.TotalFlows != 1 {
		t.Errorf("expected TotalFlows=1, got %d", result.TotalFlows)
	}
	if len(result.FlowResults) != 1 {
		t.Errorf("expected 1 FlowResult, got %d", len(result.FlowResults))
	}
}

// TestExecuteFlowWithDriver_MultipleFlows tests running multiple flows with same driver
func TestExecuteFlowWithDriver_MultipleFlows(t *testing.T) {
	dir := t.TempDir()

	// Create test flow files
	flow1 := dir + "/flow1.yaml"
	flow2 := dir + "/flow2.yaml"
	if err := os.WriteFile(flow1, []byte(`- tapOn: "Button1"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(flow2, []byte(`- tapOn: "Button2"`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Suppress stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	// Create driver ONCE
	cfg := &RunConfig{
		Platform:  "mock",
		Devices:   []string{"mock-device"},
		OutputDir: dir + "/reports",
	}

	driver, cleanup, err := CreateDriver(cfg)
	if err != nil {
		t.Fatalf("CreateDriver failed: %v", err)
	}
	defer cleanup()

	// Run first flow
	f1, _ := flow.ParseFile(flow1)
	cfg.OutputDir = dir + "/reports/flow1"
	result1, err := ExecuteFlowWithDriver(driver, cfg, *f1)
	if err != nil {
		t.Fatalf("ExecuteFlowWithDriver flow1 failed: %v", err)
	}
	if result1.TotalFlows != 1 {
		t.Errorf("flow1: expected TotalFlows=1, got %d", result1.TotalFlows)
	}

	// Run second flow with SAME driver (session reuse)
	f2, _ := flow.ParseFile(flow2)
	cfg.OutputDir = dir + "/reports/flow2"
	result2, err := ExecuteFlowWithDriver(driver, cfg, *f2)
	if err != nil {
		t.Fatalf("ExecuteFlowWithDriver flow2 failed: %v", err)
	}
	if result2.TotalFlows != 1 {
		t.Errorf("flow2: expected TotalFlows=1, got %d", result2.TotalFlows)
	}
}

// TestExecuteFlowWithDriver_OutputPath tests that output goes to correct path
func TestExecuteFlowWithDriver_OutputPath(t *testing.T) {
	dir := t.TempDir()

	flowFile := dir + "/test_flow.yaml"
	if err := os.WriteFile(flowFile, []byte(`- tapOn: "Button"`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Suppress stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	cfg := &RunConfig{
		Platform:  "mock",
		Devices:   []string{"mock-device"},
		OutputDir: dir + "/custom_reports",
	}

	driver, cleanup, err := CreateDriver(cfg)
	if err != nil {
		t.Fatalf("CreateDriver failed: %v", err)
	}
	defer cleanup()

	f, _ := flow.ParseFile(flowFile)
	_, err = ExecuteFlowWithDriver(driver, cfg, *f)
	if err != nil {
		t.Fatalf("ExecuteFlowWithDriver failed: %v", err)
	}

	// Check that report.json was created in the output directory
	reportPath := dir + "/custom_reports/report.json"
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Errorf("expected report.json at %s", reportPath)
	}
}

// TestExecuteFlowWithDriver_WithEnv tests environment variables are passed
func TestExecuteFlowWithDriver_WithEnv(t *testing.T) {
	dir := t.TempDir()

	flowFile := dir + "/test_flow.yaml"
	if err := os.WriteFile(flowFile, []byte(`- tapOn: "Button"`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Suppress stdout
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	cfg := &RunConfig{
		Platform:  "mock",
		Devices:   []string{"mock-device"},
		OutputDir: dir + "/reports",
		Env: map[string]string{
			"TEST_USER": "testuser",
			"TEST_PASS": "testpass",
		},
	}

	driver, cleanup, err := CreateDriver(cfg)
	if err != nil {
		t.Fatalf("CreateDriver failed: %v", err)
	}
	defer cleanup()

	f, _ := flow.ParseFile(flowFile)
	result, err := ExecuteFlowWithDriver(driver, cfg, *f)
	if err != nil {
		t.Fatalf("ExecuteFlowWithDriver failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be non-nil")
	}
}

// TestCreateDriver_WithDeviceID tests CreateDriver with specific device ID
func TestCreateDriver_WithDeviceID(t *testing.T) {
	cfg := &RunConfig{
		Platform: "mock",
		Devices:  []string{"specific-device-id"},
	}

	driver, cleanup, err := CreateDriver(cfg)
	if err != nil {
		t.Fatalf("CreateDriver failed: %v", err)
	}
	defer cleanup()

	info := driver.GetPlatformInfo()
	if info.DeviceID != "specific-device-id" {
		t.Errorf("expected DeviceID='specific-device-id', got %q", info.DeviceID)
	}
}

func TestFlowsUseClearState(t *testing.T) {
	tests := []struct {
		name   string
		flows  []flow.Flow
		expect bool
	}{
		{
			name:   "no flows",
			flows:  nil,
			expect: false,
		},
		{
			name: "no clearState",
			flows: []flow.Flow{{
				Steps: []flow.Step{&flow.TapOnStep{}},
			}},
			expect: false,
		},
		{
			name: "standalone clearState step",
			flows: []flow.Flow{{
				Steps: []flow.Step{&flow.ClearStateStep{AppID: "com.test"}},
			}},
			expect: true,
		},
		{
			name: "launchApp with clearState true",
			flows: []flow.Flow{{
				Steps: []flow.Step{&flow.LaunchAppStep{AppID: "com.test", ClearState: true}},
			}},
			expect: true,
		},
		{
			name: "launchApp without clearState",
			flows: []flow.Flow{{
				Steps: []flow.Step{&flow.LaunchAppStep{AppID: "com.test", ClearState: false}},
			}},
			expect: false,
		},
		{
			name: "clearState in onFlowStart",
			flows: []flow.Flow{{
				Config: flow.Config{
					OnFlowStart: []flow.Step{&flow.ClearStateStep{AppID: "com.test"}},
				},
			}},
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flowsUseClearState(tt.flows)
			if got != tt.expect {
				t.Errorf("flowsUseClearState() = %v, want %v", got, tt.expect)
			}
		})
	}
}
