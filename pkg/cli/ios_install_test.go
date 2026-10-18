package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/iosinstall"
)

func stubInstallOnDevice(t *testing.T, path iosinstall.Path, err error) {
	t.Helper()
	old := installOnDevice
	t.Cleanup(func() { installOnDevice = old })
	installOnDevice = func(udid, appPath string) (iosinstall.Path, error) { return path, err }
}

func TestInstallIOSAppOnADevice(t *testing.T) {
	stubInstallOnDevice(t, iosinstall.Devicectl, nil)
	if err := installIOSApp("UDID", "/a/App.app", false); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestInstallIOSAppReportsAFailedInstall(t *testing.T) {
	stubInstallOnDevice(t, iosinstall.GoIOS, errors.New("go-ios install failed: device locked"))
	err := installIOSApp("UDID", "/a/App.app", false)
	if err == nil || !strings.Contains(err.Error(), "device locked") {
		t.Fatalf("err = %v, want the install error", err)
	}
	if strings.Contains(err.Error(), "try: xcrun devicectl") {
		t.Errorf("err = %v; the devicectl hint is for a go-ios timeout only", err)
	}
}

// A go-ios install that timed out suggests trying devicectl by hand, as the
// CLI did before.
func TestInstallIOSAppTimeoutSuggestsDevicectl(t *testing.T) {
	stubInstallOnDevice(t, iosinstall.GoIOS, fmt.Errorf("go-ios install failed: %w after 3m0s", iosinstall.ErrTimeout))
	err := installIOSApp("UDID", "/a/App.app", false)
	if err == nil || !strings.Contains(err.Error(), "try: xcrun devicectl device install app --device UDID /a/App.app") {
		t.Fatalf("err = %v, want the devicectl hint", err)
	}
}
