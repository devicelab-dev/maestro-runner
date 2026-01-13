// Package device provides Android device management via ADB.
package device

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// AndroidDevice manages an Android device connection via ADB.
type AndroidDevice struct {
	serial     string
	adbPath    string
	socketPath string // Unix socket path for UIAutomator2 (Linux/Mac)
	localPort  int    // TCP port for UIAutomator2 (Windows)
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
// If serial is empty, it uses the default connected device.
func New(serial string) (*AndroidDevice, error) {
	adbPath, err := findADB()
	if err != nil {
		return nil, err
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

// Serial returns the device serial number.
func (d *AndroidDevice) Serial() string {
	return d.serial
}

// Shell executes a shell command on the device.
func (d *AndroidDevice) Shell(cmd string) (string, error) {
	return d.adb("shell", cmd)
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

// IsInstalled checks if a package is installed.
func (d *AndroidDevice) IsInstalled(pkg string) bool {
	out, err := d.Shell("pm list packages " + pkg)
	if err != nil {
		return false
	}
	return strings.Contains(out, "package:"+pkg)
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
	cmdArgs := make([]string, 0, len(args)+2)
	if d.serial != "" {
		cmdArgs = append(cmdArgs, "-s", d.serial)
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command(d.adbPath, cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = stdout.String()
		}
		return "", fmt.Errorf("adb %s: %w: %s", strings.Join(args, " "), err, errMsg)
	}

	return stdout.String(), nil
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
	if path, err := exec.LookPath("adb"); err == nil {
		return path, nil
	}

	// Try ANDROID_HOME
	// Note: We could add more fallback paths here if needed
	return "", fmt.Errorf("adb not found in PATH; ensure Android SDK is installed")
}
