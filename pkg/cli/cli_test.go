package cli

import (
	"os"
	"strings"
	"testing"

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
		ShardSplit:  2,
		ShardAll:    0,
		Continuous:  true,
		Headless:    false,
		Platform:    "ios",
		Device:      "iPhone-15",
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
		Platform:  "ios",
		Device:    "iPhone-15",
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

	err := app.Run([]string{"test-app", "test", flowFile})
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
		"-p", "ios",
		"--device", "iPhone-15",
		"--verbose",
		"--app-file", "app.ipa",
		"test",
		"-e", "USER=test",
		"-e", "PASS=secret",
		"--include-tags", "smoke",
		"--exclude-tags", "wip",
		"--output", dir + "/reports",
		"--shard-split", "2",
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

	// Note: flags must come before positional args
	err := app.Run([]string{
		"test-app", "test",
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
