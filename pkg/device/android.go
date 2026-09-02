// Package device provides Android device management via ADB.
package device

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// AndroidDevice manages an Android device connection via ADB.
type AndroidDevice struct {
	serial     string
	adbPath    string
	socketPath string // Unix socket path for UIAutomator2 (Linux/Mac)
	localPort  int    // TCP port for UIAutomator2 (Windows)

	driverSocketPath string // Unix socket path for DeviceLab Android Driver (Linux/Mac)
	driverLocalPort  int    // TCP port for DeviceLab Android Driver (Windows)
}

// DeviceInfo contains basic device information.
type DeviceInfo struct {
	Serial     string
	Model      string
	SDK        string
	Brand      string
	IsEmulator bool
}

// New creates an AndroidDevice for the given serial.
// If serial is empty, it auto-detects the connected device.
func New(serial string) (*AndroidDevice, error) {
	adbPath, err := findADB()
	if err != nil {
		return nil, err
	}

	// Auto-detect serial if not provided
	if serial == "" {
		serial, err = detectDeviceSerial(adbPath)
		if err != nil {
			return nil, fmt.Errorf("no device specified and auto-detect failed: %w", err)
		}
	}

	d := &AndroidDevice{
		serial:  serial,
		adbPath: adbPath,
	}

	// Verify device is connected
	if err := d.waitForDevice(5 * time.Second); err != nil {
		return nil, fmt.Errorf("device not found: %w", err)
	}

	return d, nil
}

// NoDevicesError is returned when no devices are connected, with helpful suggestions.
type NoDevicesError struct {
	Message       string
	AvailableAVDs []string
	Suggestions   []string
}

func (e *NoDevicesError) Error() string {
	msg := e.Message + "\n\n"

	if len(e.AvailableAVDs) > 0 {
		msg += "Available AVDs:\n"
		for _, avd := range e.AvailableAVDs {
			msg += fmt.Sprintf("  - %s\n", avd)
		}
		msg += "\n"
	}

	if len(e.Suggestions) > 0 {
		msg += "Options:\n"
		for i, s := range e.Suggestions {
			msg += fmt.Sprintf("  %d. %s\n", i+1, s)
		}
	}

	return msg
}

// detectDeviceSerial finds the first connected device serial.
// If no devices found, returns NoDevicesError with helpful suggestions.
func detectDeviceSerial(adbPath string) (string, error) {
	cmd := execCommand(adbPath, "devices")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("adb devices failed: %w\nHint: Is adb server running? Try: adb start-server", err)
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == "device" {
			return parts[0], nil
		}
	}

	// No devices found - provide helpful error with AVD suggestions
	return "", buildNoDevicesError()
}

// buildNoDevicesError creates a helpful error message with available AVDs and suggestions.
func buildNoDevicesError() error {
	noDevErr := &NoDevicesError{
		Message: "No Android devices or emulators found",
	}

	// Try to list available AVDs (import emulator package to avoid circular dependency)
	// We'll use exec directly here to keep dependencies simple
	avds := listAvailableAVDs()
	noDevErr.AvailableAVDs = avds

	// Build suggestions based on available AVDs
	noDevErr.Suggestions = []string{
		"Connect a physical device via USB (enable USB debugging)",
	}

	if len(avds) > 0 {
		noDevErr.Suggestions = append(noDevErr.Suggestions,
			fmt.Sprintf("Start emulator manually: emulator -avd %s", avds[0]),
			"Auto-start first AVD: maestro-runner --auto-start-emulator <flow>",
			fmt.Sprintf("Start specific AVD: maestro-runner --start-emulator %s <flow>", avds[0]),
		)

		// If multiple AVDs, suggest parallel execution
		if len(avds) >= 2 {
			noDevErr.Suggestions = append(noDevErr.Suggestions,
				fmt.Sprintf("Run in parallel on %d emulators: maestro-runner --parallel %d --auto-start-emulator <flows>", len(avds), len(avds)),
			)
		}
	} else {
		noDevErr.Suggestions = append(noDevErr.Suggestions,
			"Create an AVD: avdmanager create avd --name <name> --package <system-image>",
			"Or create via Android Studio: Tools → Device Manager → Create Device",
		)
	}

	return noDevErr
}

// listAvailableAVDs returns list of AVD names using emulator binary.
// Returns empty slice if emulator not found or command fails.
func listAvailableAVDs() []string {
	// Try to find emulator binary
	emulatorPath := findEmulatorBinary()
	if emulatorPath == "" {
		return nil
	}

	// Run emulator -list-avds
	cmd := execCommand(emulatorPath, "-list-avds")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Parse output (one AVD per line)
	var avds []string
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			avds = append(avds, line)
		}
	}

	return avds
}

// findEmulatorBinary locates the emulator binary, returns empty string if not found.
func findEmulatorBinary() string {
	// Try ANDROID_HOME/emulator/emulator
	androidHome := os.Getenv("ANDROID_HOME")
	if androidHome == "" {
		androidHome = os.Getenv("ANDROID_SDK_ROOT")
	}
	if androidHome == "" {
		androidHome = os.Getenv("ANDROID_SDK_HOME")
	}

	if androidHome != "" {
		emulatorPath := filepath.Join(androidHome, "emulator", "emulator")
		if _, err := os.Stat(emulatorPath); err == nil {
			return emulatorPath
		}
	}

	// Try PATH
	if path, err := execLookPath("emulator"); err == nil {
		return path
	}

	return ""
}

// Serial returns the device serial number.
func (d *AndroidDevice) Serial() string {
	return d.serial
}

// Shell executes a shell command on the device.
func (d *AndroidDevice) Shell(cmd string) (string, error) {
	return d.adb("shell", cmd)
}

// Screenshot captures the physical display through ADB.
func (d *AndroidDevice) Screenshot() ([]byte, error) {
	return d.adbOutput("exec-out", "screencap", "-p")
}

// Install installs an APK on the device.
func (d *AndroidDevice) Install(apkPath string) error {
	_, err := d.adb("install", "-r", "-g", apkPath)
	return err
}

// Uninstall removes a package from the device.
func (d *AndroidDevice) Uninstall(pkg string) error {
	_, err := d.adb("uninstall", pkg)
	return err
}

// Push copies a local file to the device via `adb push`.
func (d *AndroidDevice) Push(localPath, remotePath string) error {
	_, err := d.adb("push", localPath, remotePath)
	return err
}

// Pull copies a file from the device to the host.
func (d *AndroidDevice) Pull(remotePath, localPath string) error {
	_, err := d.adb("pull", remotePath, localPath)
	return err
}

// GetAppVersion returns the version name of an installed app.
// Returns empty string if app is not installed or version cannot be determined.
func (d *AndroidDevice) GetAppVersion(packageName string) string {
	version, _ := d.GetAppVersionAndBuild(packageName)
	return version
}

// GetAppVersionAndBuild returns an installed app's version name and version
// code — what a user calls the version and the build number.
//
// One release version covers many CI builds, so the version name alone does not
// identify which binary a run used. Both come out of the same dumpsys output,
// so reading them together costs one shell call rather than two.
//
// Either value is empty when the app is not installed or it cannot be parsed.
func (d *AndroidDevice) GetAppVersionAndBuild(packageName string) (version, build string) {
	if packageName == "" {
		return "", ""
	}

	// Matching on "version" alone keeps this to a single grep with nothing to
	// quote, and the parsing below only accepts the two keys that matter.
	out, err := d.Shell(fmt.Sprintf("dumpsys package %s | grep version", core.ShellQuote(packageName)))
	if err != nil {
		return "", ""
	}

	// Parse output lines: "    versionName=2.2.0" and
	// "    versionCode=10009107 minSdk=24 targetSdk=34" — the version code
	// shares its line with other fields, so it stops at the first space.
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case version == "" && strings.HasPrefix(line, "versionName="):
			version = strings.TrimSpace(strings.TrimPrefix(line, "versionName="))
		case build == "" && strings.HasPrefix(line, "versionCode="):
			if fields := strings.Fields(strings.TrimPrefix(line, "versionCode=")); len(fields) > 0 {
				build = fields[0]
			}
		}
	}

	return version, build
}

// IsInstalled checks if a package is installed.
func (d *AndroidDevice) IsInstalled(pkg string) bool {
	installed, _ := d.CheckInstalled(pkg)
	return installed
}

// CheckInstalled reports whether pkg is installed, and why the answer is
// negative when it is.
//
// The error matters as much as the bool. Reporting an adb failure as "not
// installed" points the user at the package when the fault is the connection:
// one report had a server that was installed, running, and answering HTTP,
// and still saw "not installed", because any Shell error became a false here
// (#163).
//
// Three queries, because one is not enough:
//
//   - `pm list packages <filter>` matches substrings both ways. Asking for
//     io.appium.uiautomator2.server also returns …server.test, so a device
//     carrying only the test APK looked like it had the server. Whole-line
//     comparison fixes that direction.
//   - `--user 0` covers a device whose shell user does not see a package
//     installed for user 0 — a work profile or a vendor container.
//   - the unfiltered listing is the last resort for a `pm` whose filter
//     argument behaves unexpectedly.
func (d *AndroidDevice) CheckInstalled(pkg string) (bool, error) {
	var lastErr error
	for _, query := range []string{
		"pm list packages " + pkg,
		"pm list packages --user 0 " + pkg,
		"pm list packages",
	} {
		out, err := d.Shell(query)
		if err != nil {
			lastErr = err
			continue
		}
		if packageListed(out, pkg) {
			return true, nil
		}
	}
	return false, lastErr
}

// packageListed reports whether pkg appears as a whole entry in `pm list
// packages` output.
//
// Entries read "package:<name>", or "package:<apk path>=<name>" when pm is
// given -f. Lines are trimmed because adb on Windows terminates them with
// \r\n, and a stray \r would defeat the comparison.
func packageListed(out, pkg string) bool {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "package:") {
			continue
		}
		entry := strings.TrimPrefix(line, "package:")
		if i := strings.LastIndex(entry, "="); i >= 0 {
			entry = entry[i+1:]
		}
		if entry == pkg {
			return true
		}
	}
	return false
}

// Forward creates a port forward from local to device.
func (d *AndroidDevice) Forward(localPort, remotePort int) error {
	_, err := d.adb("forward", fmt.Sprintf("tcp:%d", localPort), fmt.Sprintf("tcp:%d", remotePort))
	return err
}

// RemoveForward removes a port forward.
func (d *AndroidDevice) RemoveForward(localPort int) error {
	_, err := d.adb("forward", "--remove", fmt.Sprintf("tcp:%d", localPort))
	return err
}

// RemoveAllForwards removes all port forwards for this device.
func (d *AndroidDevice) RemoveAllForwards() error {
	_, err := d.adb("forward", "--remove-all")
	return err
}

// ForwardSocket forwards a Unix socket to a device TCP port.
func (d *AndroidDevice) ForwardSocket(socketPath string, remotePort int) error {
	_, err := d.adb("forward", fmt.Sprintf("localfilesystem:%s", socketPath), fmt.Sprintf("tcp:%d", remotePort))
	return err
}

// RemoveSocketForward removes a Unix socket forward.
func (d *AndroidDevice) RemoveSocketForward(socketPath string) error {
	_, err := d.adb("forward", "--remove", fmt.Sprintf("localfilesystem:%s", socketPath))
	return err
}

// ForwardToAbstractSocket forwards a local Unix socket to a device abstract socket.
// Used for CDP WebView connections: localfilesystem:/tmp/cdp-<serial>.sock → localabstract:<socket>
func (d *AndroidDevice) ForwardToAbstractSocket(localSocketPath, remoteSocketName string) error {
	_, err := d.adb("forward", fmt.Sprintf("localfilesystem:%s", localSocketPath), fmt.Sprintf("localabstract:%s", remoteSocketName))
	return err
}

// ForwardTCPToAbstractSocket forwards a local TCP port to a device abstract socket.
// Used for browser CDP connections: tcp:<port> → localabstract:<socket>
func (d *AndroidDevice) ForwardTCPToAbstractSocket(localPort int, remoteSocketName string) error {
	_, err := d.adb("forward", fmt.Sprintf("tcp:%d", localPort), fmt.Sprintf("localabstract:%s", remoteSocketName))
	return err
}

// RemoveTCPForward removes a TCP port forward.
func (d *AndroidDevice) RemoveTCPForward(localPort int) error {
	_, err := d.adb("forward", "--remove", fmt.Sprintf("tcp:%d", localPort))
	return err
}

// CDPSocketPath returns the local Unix socket path for CDP WebView forwarding.
func (d *AndroidDevice) CDPSocketPath() string {
	return fmt.Sprintf("/tmp/cdp-%s.sock", d.serial)
}

// DefaultSocketPath returns the default Unix socket path for this device.
func (d *AndroidDevice) DefaultSocketPath() string {
	return fmt.Sprintf("/tmp/uia2-%s.sock", d.serial)
}

// SocketPath returns the current UIAutomator2 socket path (empty if not started or on Windows).
func (d *AndroidDevice) SocketPath() string {
	return d.socketPath
}

// LocalPort returns the current UIAutomator2 TCP port (0 if not started or on Linux/Mac).
func (d *AndroidDevice) LocalPort() int {
	return d.localPort
}

// Info returns device information.
func (d *AndroidDevice) Info() (DeviceInfo, error) {
	info := DeviceInfo{Serial: d.serial}

	if model, err := d.Shell("getprop ro.product.model"); err == nil {
		info.Model = strings.TrimSpace(model)
	}
	if sdk, err := d.Shell("getprop ro.build.version.sdk"); err == nil {
		info.SDK = strings.TrimSpace(sdk)
	}
	if brand, err := d.Shell("getprop ro.product.brand"); err == nil {
		info.Brand = strings.TrimSpace(brand)
	}

	// Check if emulator
	chars, _ := d.Shell("getprop ro.kernel.qemu")
	info.IsEmulator = strings.TrimSpace(chars) == "1"

	return info, nil
}

// adb executes an ADB command.
func (d *AndroidDevice) adb(args ...string) (string, error) {
	out, err := d.adbOutput(args...)
	return string(out), err
}

func (d *AndroidDevice) adbOutput(args ...string) ([]byte, error) {
	cmdArgs := make([]string, 0, len(args)+2)
	if d.serial != "" {
		cmdArgs = append(cmdArgs, "-s", d.serial)
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := execCommand(d.adbPath, cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = stdout.String()
		}
		return nil, fmt.Errorf("adb %s: %w: %s", strings.Join(args, " "), err, errMsg)
	}

	return stdout.Bytes(), nil
}

// waitForDevice waits for the device to be available.
func (d *AndroidDevice) waitForDevice(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if d.isConnected() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for device %s", d.serial)
}

// isConnected checks if the device is connected.
func (d *AndroidDevice) isConnected() bool {
	out, err := d.adb("get-state")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "device"
}

// findADB locates the ADB binary.
func findADB() (string, error) {
	// Try PATH first
	if path, err := execLookPath("adb"); err == nil {
		return path, nil
	}

	// Try ANDROID_HOME
	// Note: We could add more fallback paths here if needed
	return "", fmt.Errorf("adb not found in PATH; ensure Android SDK is installed")
}

// installCheckError turns a failed package check into a message that names the
// real fault. An adb failure is not a missing package, and saying so sends
// people to reinstall something that is already there (#163).
func installCheckError(what, pkg string, err error) error {
	if err != nil {
		return fmt.Errorf("could not determine whether %s (%s) is installed — adb failed: %w", what, pkg, err)
	}
	return fmt.Errorf("%s not installed: %s", what, pkg)
}
