package device

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// DeviceLab Android Driver package names.
const (
	DeviceLabDriverServer = "dev.devicelab.driver.android"
	DeviceLabDriverTest   = "dev.devicelab.driver.android.test"
	DeviceLabDriverPort   = 6791
)

// DeviceLabDriverConfig holds configuration for the DeviceLab Android Driver.
type DeviceLabDriverConfig struct {
	SocketPath string        // Unix socket path (Linux/Mac only)
	LocalPort  int           // TCP port (Windows only)
	DevicePort int           // Port on device (default: 6791)
	Timeout    time.Duration // Startup timeout (default: 30s)
}

// DefaultDeviceLabDriverConfig returns default configuration.
func DefaultDeviceLabDriverConfig() DeviceLabDriverConfig {
	return DeviceLabDriverConfig{
		DevicePort: DeviceLabDriverPort,
		Timeout:    30 * time.Second,
	}
}

// DeviceLabDriverSocketPath returns the default socket path for the DeviceLab Android Driver.
func (d *AndroidDevice) DeviceLabDriverSocketPath() string {
	return fmt.Sprintf("/tmp/devicelab-driver-%s.sock", d.serial)
}

// StartDeviceLabDriver starts the DeviceLab Android Driver on the device.
func (d *AndroidDevice) StartDeviceLabDriver(cfg DeviceLabDriverConfig) error {
	// Check if driver APKs are installed
	if !d.IsInstalled(DeviceLabDriverServer) {
		return fmt.Errorf("DeviceLab Android Driver not installed: %s", DeviceLabDriverServer)
	}
	if !d.IsInstalled(DeviceLabDriverTest) {
		return fmt.Errorf("DeviceLab Android Driver test APK not installed: %s", DeviceLabDriverTest)
	}

	// Pre-flight: check for conflicting UiAutomation holders (e.g. Appium)
	if err := d.checkUiAutomationConflict(); err != nil {
		return err
	}

	// Stop any existing instance
	if err := d.StopDeviceLabDriver(); err != nil {
		logger.Warn("failed to stop existing DeviceLab Android Driver instance: %v", err)
	}

	// Set up forwarding based on OS
	if runtime.GOOS == "windows" {
		if err := d.setupDeviceLabTCPForward(cfg); err != nil {
			return err
		}
	} else {
		if err := d.setupDeviceLabSocketForward(cfg); err != nil {
			return err
		}
	}

	// Start instrumentation to a temp file so we can read crash output
	instrumentCmd := fmt.Sprintf(
		"nohup am instrument -w "+
			"%s/%s "+
			"> /data/local/tmp/devicelab-driver.log 2>&1 &",
		DeviceLabDriverTest,
		DeviceLabDriverServer+".DeviceLabDriverRunner",
	)
	if _, err := d.Shell(instrumentCmd); err != nil {
		return fmt.Errorf("failed to start DeviceLab Android Driver instrumentation: %w", err)
	}

	// Quick check: give it 2 seconds then verify it didn't crash immediately
	time.Sleep(2 * time.Second)
	if crashed, reason := d.checkDriverCrashed(); crashed {
		if stopErr := d.StopDeviceLabDriver(); stopErr != nil {
			logger.Warn("failed to stop DeviceLab Android Driver after crash: %v", stopErr)
		}
		return fmt.Errorf("DeviceLab driver crashed on startup: %s", reason)
	}

	// Wait for driver to be ready
	if err := d.waitForDeviceLabDriverReady(cfg.Timeout); err != nil {
		// Read crash log for diagnostics
		if _, reason := d.checkDriverCrashed(); reason != "" {
			err = fmt.Errorf("%w\nDriver output: %s", err, reason)
		}
		// Slow-infra diagnostics: surface the runtime state that's most
		// useful for explaining why the health check missed a running
		// driver — adb forward list (is our local→device forward live?)
		// and the device's listening sockets (did the driver actually
		// bind its port?). Both hypotheses for the AWS Device Farm
		// failure mode in #76. Best-effort: ignore command errors and
		// just include whatever output we can grab.
		err = d.appendDriverDiagnostics(err)
		if stopErr := d.StopDeviceLabDriver(); stopErr != nil {
			logger.Warn("failed to stop DeviceLab Android Driver after startup timeout: %v", stopErr)
		}
		return err
	}

	return nil
}

// checkUiAutomationConflict kills any process holding a UiAutomation connection.
// Only one UiAutomation connection is allowed at a time — a second attempt crashes
// with "UiAutomationService already registered".
// This stops all known instrumentation holders (Appium, Maestro, etc.) and also
// kills any active instrumentation reported by the system.
func (d *AndroidDevice) checkUiAutomationConflict() error {
	// Known packages that hold UiAutomation connections
	knownConflicts := []string{
		"io.appium.uiautomator2.server",
		"io.appium.uiautomator2.server.test",
		"dev.mobile.maestro",
		"dev.mobile.maestro.test",
		"com.example.maestro.orientation",
	}

	output, _ := d.Shell("ps -A")
	for _, pkg := range knownConflicts {
		if strings.Contains(output, pkg) {
			logger.Info("Stopping %s to avoid UiAutomation conflict", pkg)
			d.Shell("am force-stop " + pkg)
		}
	}

	// Also kill any active instrumentation — catches unknown holders
	instrOutput, _ := d.Shell("cmd activity get-current-instrumentation")
	if instrOutput != "" && !strings.Contains(instrOutput, "No active") {
		// Format: "package/runner (target=package)"
		for _, line := range strings.Split(instrOutput, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.Contains(line, DeviceLabDriverTest) {
				continue // skip our own driver
			}
			// Extract package name before the "/"
			if idx := strings.Index(line, "/"); idx > 0 {
				pkg := line[:idx]
				logger.Info("Stopping active instrumentation: %s", pkg)
				d.Shell("am force-stop " + pkg)
			}
		}
	}

	time.Sleep(500 * time.Millisecond)
	return nil
}

// checkDriverCrashed checks if the driver process exited and returns the reason.
func (d *AndroidDevice) checkDriverCrashed() (crashed bool, reason string) {
	output, _ := d.Shell("ps -A")
	if strings.Contains(output, DeviceLabDriverServer) {
		return false, "" // still running
	}

	// Process not found — read the log for crash info
	logOutput, err := d.Shell("cat /data/local/tmp/devicelab-driver.log")
	if err != nil || logOutput == "" {
		return true, "process exited (no log output)"
	}
	// Trim to last 500 chars for readability
	if len(logOutput) > 500 {
		logOutput = logOutput[len(logOutput)-500:]
	}
	return true, strings.TrimSpace(logOutput)
}

// setupDeviceLabSocketForward sets up Unix socket forwarding for the DeviceLab Android Driver.
func (d *AndroidDevice) setupDeviceLabSocketForward(cfg DeviceLabDriverConfig) error {
	socketPath := cfg.SocketPath
	if socketPath == "" {
		socketPath = d.DeviceLabDriverSocketPath()
	}

	// Check for existing socket file
	if _, err := os.Stat(socketPath); err == nil {
		if IsOwnerAlive(socketPath) {
			return fmt.Errorf("device %s already in use by DeviceLab Android Driver (socket %s is active)", d.Serial(), socketPath)
		}
		// Stale socket — clean up
		logger.Info("Removing stale DeviceLab Android Driver socket for device %s: %s", d.Serial(), socketPath)
		if err := d.RemoveSocketForward(socketPath); err != nil {
			logger.Debug("failed to remove stale socket forward %s: %v", socketPath, err)
		}
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			logger.Debug("failed to remove stale socket file %s: %v", socketPath, err)
		}
		os.Remove(pidPathFor(socketPath))
	}

	if err := d.ForwardSocket(socketPath, cfg.DevicePort); err != nil {
		return fmt.Errorf("DeviceLab Android Driver socket forward failed: %w", err)
	}
	d.driverSocketPath = socketPath

	// Write PID file
	if err := os.WriteFile(pidPathFor(socketPath), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		logger.Warn("failed to write DeviceLab Android Driver PID file: %v", err)
	}

	return nil
}

// setupDeviceLabTCPForward sets up TCP port forwarding for the DeviceLab Android Driver (Windows).
func (d *AndroidDevice) setupDeviceLabTCPForward(cfg DeviceLabDriverConfig) error {
	localPort := cfg.LocalPort
	if localPort == 0 {
		port, err := findFreePort(portRangeStart, portRangeEnd)
		if err != nil {
			return err
		}
		localPort = port
	}

	if err := d.Forward(localPort, cfg.DevicePort); err != nil {
		return fmt.Errorf("DeviceLab Android Driver port forward failed: %w", err)
	}
	d.driverLocalPort = localPort
	return nil
}

// StopDeviceLabDriver stops the DeviceLab Android Driver.
func (d *AndroidDevice) StopDeviceLabDriver() error {
	// Force stop packages
	if _, err := d.Shell("am force-stop " + DeviceLabDriverServer); err != nil {
		logger.Warn("failed to force-stop %s: %v", DeviceLabDriverServer, err)
	}
	if _, err := d.Shell("am force-stop " + DeviceLabDriverTest); err != nil {
		logger.Warn("failed to force-stop %s: %v", DeviceLabDriverTest, err)
	}

	time.Sleep(300 * time.Millisecond)

	// Clean up socket (Linux/Mac)
	if d.driverSocketPath != "" {
		if err := d.RemoveSocketForward(d.driverSocketPath); err != nil {
			logger.Warn("failed to remove DeviceLab Android Driver socket forward for %s: %v", d.driverSocketPath, err)
		}
		if err := os.Remove(d.driverSocketPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("failed to remove DeviceLab Android Driver socket file %s: %v", d.driverSocketPath, err)
		}
		os.Remove(pidPathFor(d.driverSocketPath))
		d.driverSocketPath = ""
	}
	// Clean up default socket path
	defaultSocket := d.DeviceLabDriverSocketPath()
	if err := d.RemoveSocketForward(defaultSocket); err != nil {
		logger.Warn("failed to remove default DeviceLab Android Driver socket forward for %s: %v", defaultSocket, err)
	}
	if err := os.Remove(defaultSocket); err != nil && !os.IsNotExist(err) {
		logger.Warn("failed to remove default DeviceLab Android Driver socket file %s: %v", defaultSocket, err)
	}
	os.Remove(pidPathFor(defaultSocket))

	// Clean up port forward (Windows)
	if d.driverLocalPort != 0 {
		if err := d.RemoveForward(d.driverLocalPort); err != nil {
			logger.Warn("failed to remove DeviceLab Android Driver port forward for port %d: %v", d.driverLocalPort, err)
		}
		d.driverLocalPort = 0
	}

	// Remove any adb forward for the device port
	if _, err := d.adb("forward", "--remove", fmt.Sprintf("tcp:%d", DeviceLabDriverPort)); err != nil {
		logger.Warn("failed to remove adb forward for tcp:%d: %v", DeviceLabDriverPort, err)
	}

	return nil
}

// IsDeviceLabDriverRunning checks if the DeviceLab Android Driver is responding.
func (d *AndroidDevice) IsDeviceLabDriverRunning() bool {
	return d.checkDeviceLabHealth()
}

// DeviceLabDriverSocket returns the current DeviceLab Android Driver socket path.
func (d *AndroidDevice) DeviceLabDriverSocket() string {
	return d.driverSocketPath
}

// DeviceLabDriverPort returns the current DeviceLab Android Driver TCP port.
func (d *AndroidDevice) DeviceLabDriverLocalPort() int {
	return d.driverLocalPort
}

// waitForDeviceLabDriverReady waits for the driver to be ready.
func (d *AndroidDevice) waitForDeviceLabDriverReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if d.checkDeviceLabHealth() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("DeviceLab Android Driver not ready after %v", timeout)
}

// checkDeviceLabHealth checks if the DeviceLab Android Driver is responding.
// Uses TCP connect since the driver uses WebSocket (not HTTP /status).
func (d *AndroidDevice) checkDeviceLabHealth() bool {
	if d.driverSocketPath != "" {
		return checkDeviceLabHealthViaSocket(d.driverSocketPath)
	}
	if d.driverLocalPort != 0 {
		return checkDeviceLabHealthViaTCP(d.driverLocalPort)
	}
	return false
}

// checkDeviceLabHealthViaSocket checks health via Unix socket.
// Sends a WebSocket handshake to verify end-to-end connectivity
// (bare connect succeeds immediately since ADB creates the socket on forward).
func checkDeviceLabHealthViaSocket(socketPath string) bool {
	return checkDeviceLabHandshake("unix", socketPath)
}

// checkDeviceLabHealthViaTCP checks health via TCP connect.
func checkDeviceLabHealthViaTCP(port int) bool {
	return checkDeviceLabHandshake("tcp", fmt.Sprintf("127.0.0.1:%d", port))
}

// checkDeviceLabHandshake verifies the driver is responding by sending a
// WebSocket upgrade request and checking for the 101 response.
func checkDeviceLabHandshake(network, address string) bool {
	conn, err := net.DialTimeout(network, address, 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(2 * time.Second))

	// Send a minimal WebSocket upgrade request
	handshake := "GET / HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := conn.Write([]byte(handshake)); err != nil {
		return false
	}

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return false
	}

	// Check for HTTP 101 Switching Protocols
	return strings.Contains(string(buf[:n]), "101")
}

// InstallDeviceLabDriver installs DeviceLab Android Driver APKs from the given directory.
func (d *AndroidDevice) InstallDeviceLabDriver(apksDir string) error {
	apks := []struct {
		pkg     string
		pattern string
	}{
		{DeviceLabDriverServer, "devicelab-android-driver.apk"},
		{DeviceLabDriverTest, "devicelab-android-driver-test*.apk"},
	}

	for _, apk := range apks {
		apkPath, err := findAPK(apksDir, apk.pattern)
		if err != nil {
			return fmt.Errorf("failed to find APK for %s: %w", apk.pkg, err)
		}

		if d.IsInstalled(apk.pkg) {
			// Check version
			if apk.pkg == DeviceLabDriverServer {
				installedVersion := d.GetAppVersion(apk.pkg)
				bundledVersion := extractVersionFromFilename(apkPath)
				if installedVersion != "" && bundledVersion != "" && installedVersion == bundledVersion {
					continue
				}
				logger.Info("DeviceLab Android Driver version mismatch: installed=%s, bundled=%s — upgrading",
					installedVersion, bundledVersion)
			} else {
				continue
			}

			_ = d.Uninstall(apk.pkg)
			if apk.pkg == DeviceLabDriverServer {
				_ = d.Uninstall(DeviceLabDriverTest)
			}
		}

		if err := d.Install(apkPath); err != nil {
			return fmt.Errorf("failed to install %s: %w", apk.pkg, err)
		}
	}

	return nil
}

// UninstallDeviceLabDriver removes DeviceLab Android Driver packages from the device.
func (d *AndroidDevice) UninstallDeviceLabDriver() error {
	packages := []string{DeviceLabDriverServer, DeviceLabDriverTest}
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

// appendDriverDiagnostics enriches a driver-not-ready error with two
// pieces of runtime state that explain the common slow-infra failure
// modes:
//
//   * `adb forward --list` — verifies our local→device port/socket
//     forward is established. If it's missing or pointing at a different
//     port, the health check's net.Dial("tcp", 127.0.0.1:N) is failing
//     because there's nothing on the local end (not because the driver
//     is down).
//   * `cat /proc/net/tcp` filtered to our port — verifies the driver
//     process actually bound its WebSocket port on the device. If it
//     didn't, the driver is still warming up (JVM, classpath, etc.) and
//     a longer --driver-start-timeout helps.
//
// Both calls are best-effort. If they fail (locked-down farm, ADB
// proxy stripping forward subcommand, etc.) we just include whatever
// partial output we got. Never override the original error message.
func (d *AndroidDevice) appendDriverDiagnostics(origErr error) error {
	var diag strings.Builder

	forwards, ferr := d.adb("forward", "--list")
	diag.WriteString("\n\n[diagnostics] adb forward --list:")
	if ferr != nil {
		diag.WriteString(fmt.Sprintf(" (failed: %v)", ferr))
	} else if forwards == "" {
		diag.WriteString(" (empty — no active forwards)")
	} else {
		diag.WriteString("\n" + indentDiag(forwards))
	}

	// Driver port in hex for /proc/net/tcp matching.
	portHex := fmt.Sprintf("%04X", DeviceLabDriverPort)
	listening, lerr := d.Shell(fmt.Sprintf("cat /proc/net/tcp | grep ' %s '", portHex))
	diag.WriteString(fmt.Sprintf("\n[diagnostics] /proc/net/tcp entries for port %d (%s):", DeviceLabDriverPort, portHex))
	if lerr != nil || strings.TrimSpace(listening) == "" {
		diag.WriteString(" (none — driver did NOT bind the port; likely still warming up)")
	} else {
		diag.WriteString("\n" + indentDiag(listening))
	}

	return fmt.Errorf("%w%s", origErr, diag.String())
}

// indentDiag prefixes each line with two spaces so the diagnostic block
// reads visually distinct from the surrounding error.
func indentDiag(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}
