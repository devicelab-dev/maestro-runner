package device

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// UIAutomator2 package names
const (
	UIAutomator2Server = "io.appium.uiautomator2.server"
	UIAutomator2Test   = "io.appium.uiautomator2.server.test"
	AppiumSettings     = "io.appium.settings"
)

// Port range for TCP forwarding (Windows)
const (
	portRangeStart = 6001
	portRangeEnd   = 7001
)

// UIAutomator2Config holds configuration for the UIAutomator2 server.
type UIAutomator2Config struct {
	SocketPath string        // Unix socket path (Linux/Mac only, default: /tmp/uia2-<serial>.sock)
	LocalPort  int           // TCP port (Windows only, default: auto-find free port)
	DevicePort int           // Port on device (default: 6790)
	Timeout    time.Duration // Startup timeout (default: 30s)
}

// DefaultUIAutomator2Config returns default configuration.
func DefaultUIAutomator2Config() UIAutomator2Config {
	return UIAutomator2Config{
		DevicePort: 6790,
		Timeout:    30 * time.Second,
	}
}

// StartUIAutomator2 starts the UIAutomator2 server on the device.
func (d *AndroidDevice) StartUIAutomator2(cfg UIAutomator2Config) error {
	// Check if server APKs are installed
	if !d.IsInstalled(UIAutomator2Server) {
		return fmt.Errorf("UIAutomator2 server not installed: %s", UIAutomator2Server)
	}
	if !d.IsInstalled(UIAutomator2Test) {
		return fmt.Errorf("UIAutomator2 test APK not installed: %s", UIAutomator2Test)
	}

	// Stop any existing instance
	if err := d.StopUIAutomator2(); err != nil {
		logger.Warn("failed to stop existing UIAutomator2 instance: %v", err)
	}

	// Set up forwarding based on OS
	if runtime.GOOS == "windows" {
		if err := d.setupTCPForward(cfg); err != nil {
			return err
		}
	} else {
		if err := d.setupSocketForward(cfg); err != nil {
			return err
		}
	}

	// Start instrumentation in background using nohup
	// Note: We use nohup and redirect output to /dev/null to properly background the process
	instrumentCmd := fmt.Sprintf(
		"nohup am instrument -w -e disableAnalytics true "+
			"%s/androidx.test.runner.AndroidJUnitRunner "+
			"> /dev/null 2>&1 &",
		UIAutomator2Test,
	)
	if _, err := d.Shell(instrumentCmd); err != nil {
		return fmt.Errorf("failed to start instrumentation: %w", err)
	}

	// Wait for server to be ready
	if err := d.waitForUIAutomator2Ready(cfg.Timeout); err != nil {
		if stopErr := d.StopUIAutomator2(); stopErr != nil {
			logger.Warn("failed to stop UIAutomator2 after startup timeout: %v", stopErr)
		}
		return err
	}

	return nil
}

// setupSocketForward sets up Unix socket forwarding (Linux/Mac).
func (d *AndroidDevice) setupSocketForward(cfg UIAutomator2Config) error {
	socketPath := cfg.SocketPath
	if socketPath == "" {
		socketPath = d.DefaultSocketPath()
	}

	// Remove stale socket file
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		logger.Debug("failed to remove stale socket %s: %v", socketPath, err)
	}

	if err := d.ForwardSocket(socketPath, cfg.DevicePort); err != nil {
		return fmt.Errorf("socket forward failed: %w", err)
	}
	d.socketPath = socketPath
	return nil
}

// setupTCPForward sets up TCP port forwarding (Windows).
func (d *AndroidDevice) setupTCPForward(cfg UIAutomator2Config) error {
	localPort := cfg.LocalPort
	if localPort == 0 {
		port, err := findFreePort(portRangeStart, portRangeEnd)
		if err != nil {
			return err
		}
		localPort = port
	}

	if err := d.Forward(localPort, cfg.DevicePort); err != nil {
		return fmt.Errorf("port forward failed: %w", err)
	}
	d.localPort = localPort
	return nil
}

// findFreePort finds a free TCP port in the given range.
func findFreePort(start, end int) (int, error) {
	for port := start; port <= end; port++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			if closeErr := ln.Close(); closeErr != nil {
				logger.Warn("failed to close listener on port %d: %v", port, closeErr)
			}
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port found in range %d-%d", start, end)
}

// StopUIAutomator2 stops the UIAutomator2 server.
func (d *AndroidDevice) StopUIAutomator2() error {
	// Force stop both packages - this should kill the instrumentation runner
	if _, err := d.Shell("am force-stop " + UIAutomator2Server); err != nil {
		logger.Warn("failed to force-stop %s: %v", UIAutomator2Server, err)
	}
	if _, err := d.Shell("am force-stop " + UIAutomator2Test); err != nil {
		logger.Warn("failed to force-stop %s: %v", UIAutomator2Test, err)
	}

	// Give processes time to die
	time.Sleep(300 * time.Millisecond)

	// Clean up socket (Linux/Mac) - always try default path even if socketPath not set
	if d.socketPath != "" {
		if err := d.RemoveSocketForward(d.socketPath); err != nil {
			logger.Warn("failed to remove socket forward for %s: %v", d.socketPath, err)
		}
		if err := os.Remove(d.socketPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("failed to remove socket file %s: %v", d.socketPath, err)
		}
		d.socketPath = ""
	}
	// Also clean up default socket path (in case of stale from previous run)
	defaultSocket := d.DefaultSocketPath()
	if err := d.RemoveSocketForward(defaultSocket); err != nil {
		logger.Warn("failed to remove default socket forward for %s: %v", defaultSocket, err)
	}
	if err := os.Remove(defaultSocket); err != nil && !os.IsNotExist(err) {
		logger.Warn("failed to remove default socket file %s: %v", defaultSocket, err)
	}

	// Clean up port forward (Windows)
	if d.localPort != 0 {
		if err := d.RemoveForward(d.localPort); err != nil {
			logger.Warn("failed to remove port forward for port %d: %v", d.localPort, err)
		}
		d.localPort = 0
	}

	// Remove any adb forward for the device port (cleans up stale forwards)
	if _, err := d.adb("forward", "--remove", fmt.Sprintf("tcp:%d", 6790)); err != nil {
		logger.Warn("failed to remove adb forward for tcp:6790: %v", err)
	}

	return nil
}

// IsUIAutomator2Running checks if the UIAutomator2 server is responding.
func (d *AndroidDevice) IsUIAutomator2Running() bool {
	return d.checkHealth()
}

// waitForUIAutomator2Ready waits for the server to be ready.
func (d *AndroidDevice) waitForUIAutomator2Ready(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if d.checkHealth() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("UIAutomator2 server not ready after %v", timeout)
}

// checkHealth checks if UIAutomator2 is responding.
func (d *AndroidDevice) checkHealth() bool {
	if d.socketPath != "" {
		return checkHealthViaSocket(d.socketPath)
	}
	if d.localPort != 0 {
		return checkHealthViaTCP(d.localPort)
	}
	return false
}

// checkHealthViaSocket checks health via Unix socket (Linux/Mac).
func checkHealthViaSocket(socketPath string) bool {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 2 * time.Second,
	}
	return checkHealthWithClient(client, "http://localhost/wd/hub/status")
}

// checkHealthViaTCP checks health via TCP port (Windows).
func checkHealthViaTCP(port int) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	return checkHealthWithClient(client, fmt.Sprintf("http://127.0.0.1:%d/wd/hub/status", port))
}

// checkHealthWithClient performs health check using the given client and URL.
func checkHealthWithClient(client *http.Client, url string) bool {
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	if resp == nil {
		return false
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Debug("failed to close health check response body: %v", err)
		}
	}()
	return resp.StatusCode == http.StatusOK
}

// InstallUIAutomator2 installs UIAutomator2 APKs from the given directory.
func (d *AndroidDevice) InstallUIAutomator2(apksDir string) error {
	apks := []struct {
		pkg     string
		pattern string
	}{
		{UIAutomator2Server, "appium-uiautomator2-server-v*.apk"},
		{UIAutomator2Test, "appium-uiautomator2-server-debug-androidTest.apk"},
	}

	for _, apk := range apks {
		if d.IsInstalled(apk.pkg) {
			continue // Already installed
		}
		apkPath, err := findAPK(apksDir, apk.pattern)
		if err != nil {
			return fmt.Errorf("failed to find APK for %s: %w", apk.pkg, err)
		}
		if err := d.Install(apkPath); err != nil {
			return fmt.Errorf("failed to install %s: %w", apk.pkg, err)
		}
	}

	return nil
}

// findAPK finds an APK file matching the pattern in the given directory.
func findAPK(dir, pattern string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no APK found matching %s", pattern)
	}
	return matches[0], nil
}

// UninstallUIAutomator2 removes UIAutomator2 packages from the device.
func (d *AndroidDevice) UninstallUIAutomator2() error {
	packages := []string{UIAutomator2Server, UIAutomator2Test, AppiumSettings}
	var errs []string

	for _, pkg := range packages {
		if d.IsInstalled(pkg) {
			if err := d.Uninstall(pkg); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", pkg, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("uninstall errors: %s", strings.Join(errs, "; "))
	}
	return nil
}
