package device

import (
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// skipIfNoDevice skips the test if no device is connected.
func skipIfNoDevice(t *testing.T) {
	t.Helper()
	cmd := exec.Command("adb", "devices")
	out, err := cmd.Output()
	if err != nil {
		t.Skip("adb not available")
	}
	lines := strings.Split(string(out), "\n")
	deviceCount := 0
	for _, line := range lines {
		if strings.Contains(line, "\tdevice") {
			deviceCount++
		}
	}
	if deviceCount == 0 {
		t.Skip("no device connected")
	}
}

func TestListDevices_Real(t *testing.T) {
	skipIfNoDevice(t)

	devices, err := ListDevices()
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}

	if len(devices) == 0 {
		t.Fatal("expected at least one device")
	}

	// Check first device has valid fields
	d := devices[0]
	if d.Serial == "" {
		t.Error("device serial is empty")
	}
	if d.State != "device" {
		t.Errorf("expected state 'device', got %s", d.State)
	}
}

func TestFirstAvailable_Real(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	if device.Serial() == "" {
		t.Error("device serial is empty")
	}
}

func TestAndroidDevice_New(t *testing.T) {
	skipIfNoDevice(t)

	devices, _ := ListDevices()
	if len(devices) == 0 {
		t.Skip("no device")
	}

	device, err := New(devices[0].Serial)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if device.Serial() != devices[0].Serial {
		t.Errorf("expected serial %s, got %s", devices[0].Serial, device.Serial())
	}
}

func TestAndroidDevice_Shell(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// Run a simple shell command
	out, err := device.Shell("echo hello")
	if err != nil {
		t.Fatalf("Shell failed: %v", err)
	}

	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", out)
	}
}

func TestAndroidDevice_Info(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	info, err := device.Info()
	if err != nil {
		t.Fatalf("Info failed: %v", err)
	}

	if info.Serial == "" {
		t.Error("info.Serial is empty")
	}
	if info.Model == "" {
		t.Error("info.Model is empty")
	}
	if info.SDK == "" {
		t.Error("info.SDK is empty")
	}

	t.Logf("Device: %s %s (SDK %s)", info.Brand, info.Model, info.SDK)
}

func TestAndroidDevice_IsInstalled(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// Check for a package that should exist on all Android devices
	if !device.IsInstalled("com.android.settings") {
		t.Error("expected com.android.settings to be installed")
	}

	// Check for a package that shouldn't exist
	if device.IsInstalled("com.nonexistent.package.xyz123") {
		t.Error("unexpected package found")
	}
}

func TestAndroidDevice_Forward(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// Find a free port
	port, err := findFreePort(7001, 7100)
	if err != nil {
		t.Fatalf("findFreePort failed: %v", err)
	}

	// Create forward
	if err := device.Forward(port, 6790); err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	// Clean up
	if err := device.RemoveForward(port); err != nil {
		t.Errorf("RemoveForward failed: %v", err)
	}
}

func TestAndroidDevice_SocketPath(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	socketPath := device.DefaultSocketPath()
	expected := "/tmp/uia2-" + device.Serial() + ".sock"

	if socketPath != expected {
		t.Errorf("expected %s, got %s", expected, socketPath)
	}

	// SocketPath() should be empty before StartUIAutomator2
	if device.SocketPath() != "" {
		t.Errorf("expected empty SocketPath, got %s", device.SocketPath())
	}

	// LocalPort() should be 0 before StartUIAutomator2
	if device.LocalPort() != 0 {
		t.Errorf("expected LocalPort 0, got %d", device.LocalPort())
	}
}

func TestAndroidDevice_ForwardSocket(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	socketPath := "/tmp/test-uia2-" + device.Serial() + ".sock"

	// Create socket forward
	if err := device.ForwardSocket(socketPath, 6790); err != nil {
		t.Fatalf("ForwardSocket failed: %v", err)
	}

	// Clean up
	if err := device.RemoveSocketForward(socketPath); err != nil {
		t.Errorf("RemoveSocketForward failed: %v", err)
	}
}

func TestAndroidDevice_RemoveAllForwards(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// Create a forward first
	port, _ := findFreePort(7101, 7200)
	device.Forward(port, 6790)

	// Remove all
	if err := device.RemoveAllForwards(); err != nil {
		t.Errorf("RemoveAllForwards failed: %v", err)
	}
}

func TestAndroidDevice_UIAutomator2NotInstalled(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// Check if UIAutomator2 is installed
	serverInstalled := device.IsInstalled(UIAutomator2Server)
	testInstalled := device.IsInstalled(UIAutomator2Test)

	t.Logf("UIAutomator2 Server installed: %v", serverInstalled)
	t.Logf("UIAutomator2 Test installed: %v", testInstalled)

	// If not installed, StartUIAutomator2 should fail
	if !serverInstalled || !testInstalled {
		cfg := DefaultUIAutomator2Config()
		err := device.StartUIAutomator2(cfg)
		if err == nil {
			t.Error("expected error when UIAutomator2 not installed")
		}
	}
}

func TestAndroidDevice_StopUIAutomator2(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// StopUIAutomator2 should not error even if not running
	if err := device.StopUIAutomator2(); err != nil {
		t.Errorf("StopUIAutomator2 failed: %v", err)
	}
}

func TestAndroidDevice_IsUIAutomator2Running(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// Should return false when not started
	if device.IsUIAutomator2Running() {
		t.Error("expected IsUIAutomator2Running to be false")
	}
}

func TestAndroidDevice_StartUIAutomator2(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// Check if UIAutomator2 is installed
	if !device.IsInstalled(UIAutomator2Server) || !device.IsInstalled(UIAutomator2Test) {
		t.Skip("UIAutomator2 not installed")
	}

	cfg := DefaultUIAutomator2Config()
	cfg.Timeout = 60 * time.Second // Give more time for startup

	// Start UIAutomator2
	if err := device.StartUIAutomator2(cfg); err != nil {
		t.Fatalf("StartUIAutomator2 failed: %v", err)
	}

	// Should be running now
	if !device.IsUIAutomator2Running() {
		t.Error("expected IsUIAutomator2Running to be true")
	}

	// SocketPath should be set (on Linux/Mac)
	if device.SocketPath() == "" && device.LocalPort() == 0 {
		t.Error("expected either SocketPath or LocalPort to be set")
	}

	t.Logf("UIAutomator2 running - SocketPath: %s, LocalPort: %d", device.SocketPath(), device.LocalPort())

	// Stop UIAutomator2
	if err := device.StopUIAutomator2(); err != nil {
		t.Errorf("StopUIAutomator2 failed: %v", err)
	}

	// Should not be running now
	if device.IsUIAutomator2Running() {
		t.Error("expected IsUIAutomator2Running to be false after stop")
	}
}

func TestAndroidDevice_Install_Uninstall(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// Use settings APK for testing (less critical than UIAutomator2)
	apkPath := "../../apks/settings_apk-debug.apk"
	pkg := AppiumSettings

	// Uninstall first if installed
	if device.IsInstalled(pkg) {
		if err := device.Uninstall(pkg); err != nil {
			t.Logf("Uninstall warning: %v", err)
		}
	}

	// Verify not installed
	if device.IsInstalled(pkg) {
		t.Fatal("package should not be installed")
	}

	// Install
	if err := device.Install(apkPath); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify installed
	if !device.IsInstalled(pkg) {
		t.Error("package should be installed after Install")
	}

	// Uninstall
	if err := device.Uninstall(pkg); err != nil {
		t.Errorf("Uninstall failed: %v", err)
	}

	// Verify not installed
	if device.IsInstalled(pkg) {
		t.Error("package should not be installed after Uninstall")
	}
}

func TestAndroidDevice_InstallUIAutomator2(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// This should be a no-op if already installed
	if err := device.InstallUIAutomator2("../../apks"); err != nil {
		t.Errorf("InstallUIAutomator2 failed: %v", err)
	}

	// Verify both packages are installed
	if !device.IsInstalled(UIAutomator2Server) {
		t.Error("UIAutomator2Server should be installed")
	}
	if !device.IsInstalled(UIAutomator2Test) {
		t.Error("UIAutomator2Test should be installed")
	}
}

func TestAndroidDevice_New_InvalidSerial(t *testing.T) {
	// Test with invalid serial - should timeout
	_, err := New("invalid-device-serial-xyz")
	if err == nil {
		t.Error("expected error for invalid serial")
	}
}

func TestAndroidDevice_adb_Error(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// Run invalid shell command
	_, err = device.Shell("exit 1")
	// This might or might not error depending on shell behavior
	// Just ensure it doesn't panic
}

func TestAndroidDevice_UninstallUIAutomator2(t *testing.T) {
	skipIfNoDevice(t)

	device, err := FirstAvailable()
	if err != nil {
		t.Fatalf("FirstAvailable failed: %v", err)
	}

	// First ensure UIAutomator2 is installed
	if err := device.InstallUIAutomator2("../../apks"); err != nil {
		t.Fatalf("InstallUIAutomator2 failed: %v", err)
	}

	// Uninstall
	if err := device.UninstallUIAutomator2(); err != nil {
		t.Errorf("UninstallUIAutomator2 failed: %v", err)
	}

	// Verify uninstalled
	if device.IsInstalled(UIAutomator2Server) {
		t.Error("UIAutomator2Server should be uninstalled")
	}
	if device.IsInstalled(UIAutomator2Test) {
		t.Error("UIAutomator2Test should be uninstalled")
	}

	// Reinstall for other tests
	if err := device.InstallUIAutomator2("../../apks"); err != nil {
		t.Fatalf("Reinstall failed: %v", err)
	}
}

func TestCheckHealthViaTCP(t *testing.T) {
	// Test with invalid port - should return false
	result := checkHealthViaTCP(59999)
	if result {
		t.Error("expected false for invalid port")
	}
}

func TestFindAPK(t *testing.T) {
	// Test finding APK in apks directory
	apkPath, err := findAPK("../../apks", "appium-uiautomator2-server-v*.apk")
	if err != nil {
		t.Fatalf("findAPK failed: %v", err)
	}
	if apkPath == "" {
		t.Error("expected non-empty APK path")
	}

	// Test with non-existent pattern
	_, err = findAPK("../../apks", "nonexistent-*.apk")
	if err == nil {
		t.Error("expected error for non-existent pattern")
	}
}

func TestCheckHealthWithClient(t *testing.T) {
	// Test with nil-safe behavior - invalid URL should return false
	client := &http.Client{Timeout: 100 * time.Millisecond}
	result := checkHealthWithClient(client, "http://127.0.0.1:59998/invalid")
	if result {
		t.Error("expected false for invalid endpoint")
	}
}
