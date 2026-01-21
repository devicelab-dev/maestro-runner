package wda

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	WDAPort        = uint16(8100)
	buildTimeout   = 10 * time.Minute
	startupTimeout = 90 * time.Second
)

// Runner handles building and running WDA on iOS devices.
type Runner struct {
	deviceUDID string
	teamID     string
	wdaPath    string
	buildDir   string
	cmd        *exec.Cmd
	logFile    *os.File
}

// NewRunner creates a new WDA runner.
func NewRunner(deviceUDID, teamID string) *Runner {
	return &Runner{
		deviceUDID: deviceUDID,
		teamID:     teamID,
	}
}

// Build compiles WDA for the target device.
func (r *Runner) Build(ctx context.Context) error {
	wdaPath, err := GetWDAPath()
	if err != nil {
		return err
	}
	r.wdaPath = wdaPath

	// Create temp build directory
	r.buildDir, err = os.MkdirTemp("", "wda-build-*")
	if err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}

	os.MkdirAll(filepath.Join(r.buildDir, "logs"), 0755)

	logPath := filepath.Join(r.buildDir, "logs", "build.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()

	buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()

	projectPath := filepath.Join(r.wdaPath, "WebDriverAgent.xcodeproj")

	cmd := exec.CommandContext(buildCtx, "xcodebuild",
		"build-for-testing",
		"-project", projectPath,
		"-scheme", "WebDriverAgentRunner",
		"-destination", r.destination(),
		"-derivedDataPath", r.derivedDataPath(),
		"-allowProvisioningUpdates",
		fmt.Sprintf("DEVELOPMENT_TEAM=%s", r.teamID),
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	fmt.Println("Building WebDriverAgent (up to 10 min)...")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build failed:\n%s\n\nFull log: %s", tailLog(logPath, 20), logPath)
	}

	if _, err := r.findXctestrun(); err != nil {
		return err
	}

	fmt.Println("WebDriverAgent build complete")
	return nil
}

// Start runs WDA on the device.
func (r *Runner) Start(ctx context.Context) error {
	xctestrun, err := r.findXctestrun()
	if err != nil {
		return err
	}

	logPath := filepath.Join(r.buildDir, "logs", "runner.log")
	r.logFile, err = os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	r.cmd = exec.CommandContext(ctx, "xcodebuild",
		"test-without-building",
		"-xctestrun", xctestrun,
		"-destination", r.destination(),
		"-derivedDataPath", r.derivedDataPath(),
	)
	r.cmd.Stdout = r.logFile
	r.cmd.Stderr = r.logFile

	fmt.Println("Starting WebDriverAgent...")

	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start WDA: %w", err)
	}

	if err := r.waitForStartup(logPath); err != nil {
		r.Stop()
		return err
	}

	fmt.Println("WebDriverAgent started")
	return nil
}

// Stop terminates the running WDA.
func (r *Runner) Stop() {
	if r.cmd != nil && r.cmd.Process != nil {
		r.cmd.Process.Kill()
		r.cmd = nil
	}
	if r.logFile != nil {
		r.logFile.Close()
		r.logFile = nil
	}
}

// Cleanup stops WDA and removes build artifacts.
func (r *Runner) Cleanup() {
	r.Stop()
	if r.buildDir != "" {
		os.RemoveAll(r.buildDir)
		r.buildDir = ""
	}
}

func (r *Runner) destination() string {
	return fmt.Sprintf("id=%s", r.deviceUDID)
}

func (r *Runner) derivedDataPath() string {
	return filepath.Join(r.buildDir, "DerivedData")
}

func (r *Runner) findXctestrun() (string, error) {
	pattern := filepath.Join(r.derivedDataPath(), "Build", "Products", "*.xctestrun")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return "", fmt.Errorf("no xctestrun file found in %s", filepath.Dir(pattern))
	}
	return matches[0], nil
}

func (r *Runner) waitForStartup(logPath string) error {
	timeout := time.After(startupTimeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			content, err := os.ReadFile(logPath)
			if err != nil {
				continue
			}
			if err := r.checkLog(string(content), logPath); err != errNotReady {
				return err
			}
		case <-timeout:
			return fmt.Errorf("WDA startup timeout (90s):\n%s\n\nFull log: %s", tailLog(logPath, 20), logPath)
		}
	}
}

var errNotReady = fmt.Errorf("not ready")

func (r *Runner) checkLog(log, logPath string) error {
	// Success indicators
	if strings.Contains(log, "ServerURLHere") || strings.Contains(log, "WebDriverAgent") && strings.Contains(log, "started") {
		return nil
	}

	// Known errors
	if strings.Contains(log, "Developer App Certificate is not trusted") {
		return fmt.Errorf("certificate not trusted - trust it in Settings > General > VPN & Device Management")
	}
	if strings.Contains(log, "Code Sign error") {
		return fmt.Errorf("code signing failed - check your DEVELOPMENT_TEAM and provisioning profiles")
	}
	if strings.Contains(log, "Testing failed:") {
		return fmt.Errorf("WDA failed:\n%s\n\nFull log: %s", tailLog(logPath, 20), logPath)
	}

	return errNotReady
}

func tailLog(path string, lines int) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("(could not read log: %s)", err)
	}
	allLines := strings.Split(string(content), "\n")
	if len(allLines) <= lines {
		return string(content)
	}
	return strings.Join(allLines[len(allLines)-lines:], "\n")
}
