// SPDX-FileCopyrightText: 2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package device

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
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
	LocalPort  int           // TCP port (Windows or TCPForward)
	DevicePort int           // Port on device (default: 6791)
	Timeout    time.Duration // Startup timeout (default: 30s)
	// TCPForward forces TCP-to-TCP forwarding (adb forward tcp:N tcp:M)
	// instead of the default Linux/Mac unix-socket forward. Required on
	// sandboxed environments like AWS Device Farm whose adb-proxy blocks
	// localfilesystem:/localabstract: forwards but allows plain tcp:.
	// Windows always uses TCP regardless of this flag. Auto-set from
	// $DEVICEFARM_DEVICE_UDID in the CLI layer.
	TCPForward bool
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
	if ok, err := d.CheckInstalled(DeviceLabDriverServer); !ok {
		return installCheckError("DeviceLab Android Driver", DeviceLabDriverServer, err)
	}
	if ok, err := d.CheckInstalled(DeviceLabDriverTest); !ok {
		return installCheckError("DeviceLab Android Driver test APK", DeviceLabDriverTest, err)
	}

	// Pre-flight: check for conflicting UiAutomation holders (e.g. Appium)
	if err := d.checkUiAutomationConflict(); err != nil {
		return err
	}

	// Stop any existing instance
	if err := d.StopDeviceLabDriver(); err != nil {
		logger.Warn("failed to stop existing DeviceLab Android Driver instance: %v", err)
	}

	// Set up forwarding. TCP path is mandatory on Windows and used on
	// Unix when cfg.TCPForward is set (e.g. AWS Device Farm — see #83).
	if runtime.GOOS == "windows" || cfg.TCPForward {
		if err := d.setupDeviceLabTCPForward(cfg); err != nil {
			return err
		}
	} else {
		if err := d.setupDeviceLabSocketForward(cfg); err != nil {
			return err
		}
	}

	// Start instrumentation to a fresh log file so we can read crash
	// output — and only this start's output (see deviceLabStartCommand).
	marker := newDriverLogMarker()
	if _, err := d.Shell(deviceLabStartCommand(marker)); err != nil {
		return fmt.Errorf("failed to start DeviceLab Android Driver instrumentation: %w", err)
	}

	// Wait for the driver to be ready, failing early if it reports a
	// crash. There is deliberately no fixed grace period before that
	// check: `am instrument` can take several seconds to appear in `ps`
	// on a cold start (dexopt) or a loaded device, and a timed check
	// against a variable startup kills drivers that were about to come
	// up — measured at roughly one start in ten on an emulator.
	if err := d.waitForDeviceLabDriverReady(cfg.Timeout, marker); err != nil {
		// Read crash log for diagnostics
		if reason := d.driverLogTail(marker); reason != "" {
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
			_, _ = d.Shell("am force-stop " + pkg)
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
				_, _ = d.Shell("am force-stop " + pkg)
			}
		}
	}

	time.Sleep(500 * time.Millisecond)
	return nil
}

// deviceLabDriverLog is where `am instrument` output for the driver goes.
// It lives on the device across runs and reboots, which is why every
// read of it is scoped to the current start by a marker.
const deviceLabDriverLog = "/data/local/tmp/devicelab-driver.log"

// driverLogTailLen bounds how much crash output is quoted in an error.
const driverLogTailLen = 500

// newDriverLogMarker returns a line unique to one driver start.
func newDriverLogMarker() string {
	return fmt.Sprintf("devicelab-driver-start %d", time.Now().UnixNano())
}

// deviceLabStartCommand builds the shell command that launches the driver
// with its output captured in deviceLabDriverLog.
//
// The log must hold only this start's output, or a still-starting driver
// gets declared crashed on the strength of somebody else's words. Two
// writers other than this start can put text in the file:
//
//   - The previous run. Every run ends with a force-stop, whose `am
//     instrument -w` prints "INSTRUMENTATION_RESULT: shortMsg=Process
//     crashed." into the log, and that file survives reboots. A plain
//     `> log &` truncates it only once the backgrounded child gets
//     round to its redirect, which the host's first poll can beat.
//   - The previous run's `am` itself, if it is still alive. It prints
//     that same result asynchronously after the force-stop, through a
//     descriptor on the same inode at offset 0 — so a late write lands
//     in the new log after any truncation.
//
// Either way the host then sees "no driver process + crash text" and,
// for as long as a cold start keeps the new process out of `ps`
// (seconds of dexopt), calls it a crash. Removing the file first gives
// this start a new inode that no old descriptor points at, and the
// marker, written synchronously before the launch, lets the reader
// reject anything that is not this start's output (see driverOutput).
func deviceLabStartCommand(marker string) string {
	return fmt.Sprintf(
		"rm -f %[1]s; echo '%[2]s' > %[1]s; "+
			"nohup am instrument -w %[3]s/%[4]s >> %[1]s 2>&1 &",
		deviceLabDriverLog,
		marker,
		DeviceLabDriverTest,
		DeviceLabDriverServer+".DeviceLabDriverRunner",
	)
}

// driverOutput returns what this start's `am instrument` wrote to the log,
// i.e. everything after the marker line. ours is false when the log does
// not begin with this start's marker: that content is not evidence about
// this driver, whatever it says.
func driverOutput(logOutput, marker string) (output string, ours bool) {
	first, rest, _ := strings.Cut(logOutput, "\n")
	if strings.TrimSpace(first) != marker {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// driverLogTailOf trims crash output to its tail, where the error is.
func driverLogTailOf(output string) string {
	if len(output) > driverLogTailLen {
		output = output[len(output)-driverLogTailLen:]
	}
	return strings.TrimSpace(output)
}

// driverState is what the device can tell us about the driver process.
type driverState int

const (
	// driverRunning: the process is in the process table.
	driverRunning driverState = iota
	// driverStarting: no process yet, and nothing in the log from this
	// start — which is not evidence of anything. `am instrument -w`
	// writes nothing until it finishes, so an absent process with no
	// output of its own is indistinguishable from one that has simply
	// not started yet.
	driverStarting
	// driverFailed: no process, and this start's log explains why.
	driverFailed
)

// classifyDriverState decides what the device's process table and driver
// log mean for the start identified by marker. Kept pure so the decision
// is testable without a device.
//
// The distinction that matters is between failed and starting: calling a
// slow-but-healthy start a crash is what made driver startup flaky. Only
// output this start wrote counts as a failure; an empty, unreadable or
// foreign (stale) log means the driver is still starting.
func classifyDriverState(psOutput, logOutput, marker string, logErr error) (driverState, string) {
	if strings.Contains(psOutput, DeviceLabDriverServer) {
		return driverRunning, ""
	}
	if logErr != nil {
		return driverStarting, ""
	}
	output, ours := driverOutput(logOutput, marker)
	if !ours || output == "" {
		return driverStarting, ""
	}
	return driverFailed, driverLogTailOf(output)
}

// readDriverLog reads the driver log. stderr is discarded so an adb
// without exit-status propagation cannot pass "No such file" off as
// driver output.
func (d *AndroidDevice) readDriverLog() (string, error) {
	return d.Shell("cat " + deviceLabDriverLog + " 2>/dev/null")
}

// checkDriverState reads the device and classifies the driver process.
func (d *AndroidDevice) checkDriverState(marker string) (driverState, string) {
	psOutput, _ := d.Shell("ps -A")
	logOutput, logErr := d.readDriverLog()
	return classifyDriverState(psOutput, logOutput, marker, logErr)
}

// driverLogTail returns what this start wrote to the driver log, for
// diagnostics on a startup timeout. Empty when there is nothing to report.
func (d *AndroidDevice) driverLogTail(marker string) string {
	logOutput, err := d.readDriverLog()
	if err != nil {
		return ""
	}
	output, _ := driverOutput(logOutput, marker)
	return driverLogTailOf(output)
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

// waitForDeviceLabDriverReady waits for the driver started with marker to
// be ready, returning early only when that start has reported a failure.
func (d *AndroidDevice) waitForDeviceLabDriverReady(timeout time.Duration, marker string) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if d.checkDeviceLabHealth() {
			return nil
		}
		// A driver that has actually failed says so in its log; stop
		// waiting for it rather than burning the whole timeout.
		if state, reason := d.checkDriverState(marker); state == driverFailed {
			return fmt.Errorf("DeviceLab driver crashed on startup: %s", reason)
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

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

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
//
// An APK already on the device is kept only when it is byte-for-byte the
// bundled one (see installedAPKMatches). Version metadata cannot decide this:
// the bundled files carry no version in their names, and every driver build
// ships versionName 1.0.0 / versionCode 1, so a rebuilt driver looks identical
// to the one it replaces. Comparing content skips the uninstall+install on
// every session start while still replacing any genuinely different build.
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
			if d.installedAPKMatches(apk.pkg, apkPath) {
				continue
			}
			logger.Info("DeviceLab Android Driver %s differs from bundled %s — reinstalling",
				apk.pkg, filepath.Base(apkPath))

			// Uninstall first to handle signing key conflicts. The server and
			// test APKs must share a signature, so replacing the server also
			// drops the test APK; the next iteration then installs it fresh.
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

// installedAPKMatches reports whether pkg is installed from exactly the bytes
// of the local apkPath.
//
// `adb install` stores the APK unmodified as the package's base.apk, so the
// SHA-256 of that file on the device equals the local file's hash when — and
// only when — the same build is installed. Any failure to read either side
// (no sha256sum on an old device, unexpected pm output) reports false, which
// falls back to reinstalling: slower, never wrong.
func (d *AndroidDevice) installedAPKMatches(pkg, apkPath string) bool {
	want, err := fileSHA256(apkPath)
	if err != nil {
		return false
	}
	got := d.installedAPKSHA256(pkg)
	return got != "" && got == want
}

// installedAPKSHA256 returns the hex SHA-256 of pkg's installed base APK, or
// "" when it cannot be determined.
func (d *AndroidDevice) installedAPKSHA256(pkg string) string {
	out, err := d.Shell("pm path " + core.ShellQuote(pkg))
	if err != nil {
		return ""
	}
	path := baseAPKPath(out)
	if path == "" {
		return ""
	}
	out, err = d.Shell("sha256sum " + core.ShellQuote(path))
	if err != nil {
		return ""
	}
	return parseSHA256Sum(out)
}

// baseAPKPath picks the base APK from `pm path` output ("package:<path>" per
// line). Split installs list several files; the base is the one named
// base.apk, and a single-line answer is taken as the base whatever its name.
func baseAPKPath(pmOut string) string {
	var paths []string
	for _, line := range strings.Split(pmOut, "\n") {
		line = strings.TrimSpace(line)
		if p, ok := strings.CutPrefix(line, "package:"); ok && p != "" {
			paths = append(paths, p)
		}
	}
	for _, p := range paths {
		if filepath.Base(p) == "base.apk" {
			return p
		}
	}
	if len(paths) == 1 {
		return paths[0]
	}
	return ""
}

// parseSHA256Sum extracts the digest from `sha256sum` output
// ("<hex>  <path>"), returning "" unless it is a well-formed SHA-256 hex digest.
func parseSHA256Sum(out string) string {
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}
	digest := strings.ToLower(fields[0])
	if len(digest) != sha256.Size*2 {
		return ""
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return ""
	}
	return digest
}

// fileSHA256 returns the lowercase hex SHA-256 of a local file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
//   - `adb forward --list` — verifies our local→device port/socket
//     forward is established. If it's missing or pointing at a different
//     port, the health check's net.Dial("tcp", 127.0.0.1:N) is failing
//     because there's nothing on the local end (not because the driver
//     is down).
//   - `cat /proc/net/tcp` filtered to our port — verifies the driver
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
