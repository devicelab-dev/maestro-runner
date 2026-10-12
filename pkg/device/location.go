package device

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/config"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// appiumSettingsLocationService mocks the device location. It adds mock gps and network
// providers (and Play services' fused provider when present) and sends the location again
// every couple of seconds until it is stopped, as Maestro's own driver does. An app that asks
// for a fresh fix, as most do, needs those repeated updates: a location set once is not a new
// fix, so the app goes on with the real location.
const appiumSettingsLocationService = AppiumSettings + "/.LocationService"

// ShellRunner runs a shell command on an Android device.
type ShellRunner interface {
	Shell(cmd string) (string, error)
}

// appiumSettingsInstaller is implemented by AndroidDevice, which can install the bundled
// Appium Settings APK. Drivers only hold a ShellRunner, and a test double has no installer.
type appiumSettingsInstaller interface {
	EnsureAppiumSettings() error
}

// SetMockLocation points the device's location at lat, lon through Appium Settings'
// LocationService, until it is set again or the service stops. Appium Settings is installed
// from the drivers directory first if it is missing.
func SetMockLocation(sh ShellRunner, lat, lon string) error {
	for _, v := range []string{lat, lon} {
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			return fmt.Errorf("invalid coordinate %q: %w", v, err)
		}
	}
	if inst, ok := sh.(appiumSettingsInstaller); ok {
		if err := inst.EnsureAppiumSettings(); err != nil {
			return err
		}
	}

	out, err := sh.Shell("appops set " + AppiumSettings + " android:mock_location allow")
	if err != nil {
		return fmt.Errorf("allow %s to mock the location: %w", AppiumSettings, err)
	}
	if amFailed(out) {
		return fmt.Errorf("allow %s to mock the location: %s", AppiumSettings, strings.TrimSpace(out))
	}

	args := fmt.Sprintf("--user 0 -n %s --es latitude %s --es longitude %s", appiumSettingsLocationService, lat, lon)
	out, err = sh.Shell("am start-foreground-service " + args)
	if err == nil && amFailed(out) {
		// Android 7 and older have no start-foreground-service.
		out, err = sh.Shell("am startservice " + args)
	}
	if err != nil {
		return fmt.Errorf("start %s: %w", appiumSettingsLocationService, err)
	}
	if amFailed(out) {
		return fmt.Errorf("start %s: %s", appiumSettingsLocationService, strings.TrimSpace(out))
	}
	return nil
}

// amFailed reports whether am or appops printed an error. Both exit 0 over adb shell on some
// Android versions, so the output is the only sign.
func amFailed(out string) bool {
	return strings.Contains(out, "Error") || strings.Contains(out, "Unknown command") ||
		strings.Contains(out, "Exception")
}

// EnsureAppiumSettings installs the bundled Appium Settings APK if it is not installed.
func (d *AndroidDevice) EnsureAppiumSettings() error {
	if d.IsInstalled(AppiumSettings) {
		return nil
	}
	apkPath, err := findAPK(config.GetDriversDir("android"), "settings_apk*.apk")
	if err != nil {
		return fmt.Errorf("%s is not installed and its APK was not found in the drivers directory: %w",
			AppiumSettings, err)
	}
	logger.Info("Installing Appium Settings (%s) to mock the location", apkPath)
	if err := d.Install(apkPath); err != nil {
		return fmt.Errorf("install %s: %w", AppiumSettings, err)
	}
	return nil
}
