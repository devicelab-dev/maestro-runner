package devicelab_ios

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// handleClearState clears an app's data. iOS has no `pm clear` equivalent, so
// (matching WDA / Maestro) we uninstall and reinstall the app. On a simulator
// the installed .app is auto-discovered via `simctl get_app_container`; real
// devices seal the bundle and would need an explicit app file, which the
// devicelab iOS driver doesn't carry — those should use `--driver wda
// --app-file`.
//
// Without this, `launchApp: {clearState: true}` / `- clearState` silently did
// nothing on the devicelab iOS driver, contaminating "starts fresh" flows.
func (d *Driver) handleClearState(bundleID string) *core.CommandResult {
	if strings.TrimSpace(bundleID) == "" {
		bundleID = d.appID
	}
	if strings.TrimSpace(bundleID) == "" {
		return core.ErrorResult(fmt.Errorf("bundleID required"), "clearState requires an appId")
	}
	if d.info == nil || !d.info.IsSimulator {
		err := fmt.Errorf("clearState on real iOS devices is not supported by the devicelab driver")
		return core.ErrorResult(err, "clearState on real iOS devices needs the app bundle — use --driver wda --app-file")
	}
	if err := d.clearStateSimulator(bundleID); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("clearState failed: %v", err))
	}
	d.appID = bundleID
	return core.SuccessResult(fmt.Sprintf("cleared state for %s", bundleID), nil)
}

func (d *Driver) clearStateSimulator(bundleID string) error {
	// Terminate first so the reinstall isn't racing a live process.
	_ = exec.Command("xcrun", "simctl", "terminate", d.udid, bundleID).Run()

	out, err := exec.Command("xcrun", "simctl", "get_app_container", d.udid, bundleID, "app").Output()
	if err != nil {
		return fmt.Errorf("app %s not installed on the simulator: %w", bundleID, err)
	}
	appPath := strings.TrimSpace(string(out))
	if appPath == "" {
		return fmt.Errorf("could not locate the installed .app for %s", bundleID)
	}

	// Copy the bundle out first — the uninstall deletes the original.
	tmp, err := os.MkdirTemp("", "dlios-clearstate-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)
	staged := filepath.Join(tmp, filepath.Base(appPath))
	if out, err := exec.Command("cp", "-R", appPath, staged).CombinedOutput(); err != nil {
		return fmt.Errorf("stage app bundle: %v: %s", err, strings.TrimSpace(string(out)))
	}

	if out, err := exec.Command("xcrun", "simctl", "uninstall", d.udid, bundleID).CombinedOutput(); err != nil {
		return fmt.Errorf("uninstall: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("xcrun", "simctl", "install", d.udid, staged).CombinedOutput(); err != nil {
		return fmt.Errorf("reinstall: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
