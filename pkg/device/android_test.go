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
	if err := device.Forward(port, 6790); err != nil {
		t.Logf("Forward setup: %v", err)
	}

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
	apkPath := "../../drivers/android/settings_apk-debug.apk"
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
	if err := device.InstallUIAutomator2("../../drivers/android"); err != nil {
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
	t.Parallel()
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
	_, _ = device.Shell("exit 1")
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
	if err := device.InstallUIAutomator2("../../drivers/android"); err != nil {
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
	if err := device.InstallUIAutomator2("../../drivers/android"); err != nil {
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
	apkPath, err := findAPK("../../drivers/android", "appium-uiautomator2-server-v*.apk")
	if err != nil {
		t.Fatalf("findAPK failed: %v", err)
	}
	if apkPath == "" {
		t.Error("expected non-empty APK path")
	}

	// Test with non-existent pattern
	_, err = findAPK("../../drivers/android", "nonexistent-*.apk")
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

func TestNoDevicesError(t *testing.T) {
	// Test NoDevicesError formatting
	err := &NoDevicesError{
		Message:       "No Android devices or emulators found",
		AvailableAVDs: []string{"Pixel_7_API_33", "Pixel_6_API_31", "Nexus_5_API_29"},
		Suggestions: []string{
			"Connect a physical device via USB",
			"Start emulator: emulator -avd Pixel_7_API_33",
			"Auto-start: maestro-runner --auto-start-emulator flow.yaml",
		},
	}

	errMsg := err.Error()

	// Check message contains key parts
	if !strings.Contains(errMsg, "No Android devices or emulators found") {
		t.Error("Error message should contain main message")
	}

	// Check AVDs are listed
	if !strings.Contains(errMsg, "Pixel_7_API_33") {
		t.Error("Error message should list AVDs")
	}

	// Check suggestions are listed
	if !strings.Contains(errMsg, "Connect a physical device") {
		t.Error("Error message should contain suggestions")
	}

	// Check options header
	if !strings.Contains(errMsg, "Options:") {
		t.Error("Error message should have Options header")
	}
}

func TestNoDevicesError_WithParallelSuggestion(t *testing.T) {
	// Test that parallel suggestion appears when multiple AVDs available
	err := buildNoDevicesError()

	errMsg := err.Error()

	// Should contain basic error components
	if !strings.Contains(errMsg, "No Android devices") {
		t.Error("Should contain base error message")
	}

	if !strings.Contains(errMsg, "Options:") {
		t.Error("Should contain options header")
	}

	// If AVDs are available, check suggestions
	// This is environment-dependent so we just verify it doesn't panic
	t.Logf("Error message:\n%s", errMsg)
}

// =============================================================================
// GetAppVersionAndBuild — dumpsys parsing (#144)
// =============================================================================

// TestGetAppVersionAndBuild covers the dumpsys shapes this has to survive. The
// version code shares its line with other fields, which is the detail most
// likely to be parsed wrong — taking the rest of the line would report
// "10009107 minSdk=24 targetSdk=34" as the build number.
func TestGetAppVersionAndBuild(t *testing.T) {
	tests := []struct {
		name        string
		dumpsys     string
		wantVersion string
		wantBuild   string
	}{
		{
			name: "both present, build shares its line",
			dumpsys: "    versionCode=10009107 minSdk=24 targetSdk=34\n" +
				"    versionName=1.16.0\n",
			wantVersion: "1.16.0",
			wantBuild:   "10009107",
		},
		{
			name:        "version name only",
			dumpsys:     "    versionName=2.2.0\n",
			wantVersion: "2.2.0",
			wantBuild:   "",
		},
		{
			name:        "version code only",
			dumpsys:     "    versionCode=42 minSdk=21\n",
			wantVersion: "",
			wantBuild:   "42",
		},
		// Split APKs list a base and a per-split code; the first is the app's.
		{
			name: "first version code wins",
			dumpsys: "    versionCode=100 minSdk=24\n" +
				"    versionName=1.0.0\n" +
				"    versionCode=200 minSdk=24\n",
			wantVersion: "1.0.0",
			wantBuild:   "100",
		},
		{
			name:        "version name containing spaces is kept whole",
			dumpsys:     "    versionName=1.16.0 beta 3\n",
			wantVersion: "1.16.0 beta 3",
			wantBuild:   "",
		},
		{
			name:        "empty value does not panic",
			dumpsys:     "    versionCode=\n    versionName=\n",
			wantVersion: "",
			wantBuild:   "",
		},
		{
			name:        "unrelated output",
			dumpsys:     "Unable to find package\n",
			wantVersion: "",
			wantBuild:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFakeExec(t, fakeExec(tt.dumpsys))
			d := &AndroidDevice{serial: "test", adbPath: "adb"}

			version, build := d.GetAppVersionAndBuild("com.example.app")
			if version != tt.wantVersion {
				t.Errorf("version = %q, want %q", version, tt.wantVersion)
			}
			if build != tt.wantBuild {
				t.Errorf("build = %q, want %q", build, tt.wantBuild)
			}
			// The older single-value accessor has to keep behaving.
			if got := d.GetAppVersion("com.example.app"); got != tt.wantVersion {
				t.Errorf("GetAppVersion() = %q, want %q", got, tt.wantVersion)
			}
		})
	}
}

func TestGetAppVersionAndBuildEmptyPackage(t *testing.T) {
	d := &AndroidDevice{serial: "test", adbPath: "adb"}
	version, build := d.GetAppVersionAndBuild("")
	if version != "" || build != "" {
		t.Errorf("expected empty results for an empty package, got %q / %q", version, build)
	}
}
