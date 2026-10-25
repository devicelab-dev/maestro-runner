package devicelab_ios_legacy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// fastClearStateEnv opts into clearing an app by emptying its data container
// instead of reinstalling it. Much faster for a large app, but narrower: the
// keychain, App Group containers and permissions survive, which reinstalling
// resets. Maestro reinstalls, so that stays the default.
const fastClearStateEnv = "DEVICELAB_IOS_FAST_CLEARSTATE"

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
	clear := d.clearStateSimulator
	if os.Getenv(fastClearStateEnv) == "1" {
		clear = d.clearDataContainer
	}
	if err := clear(bundleID); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("clearState failed: %v", err))
	}
	d.appID = bundleID
	return core.SuccessResult(fmt.Sprintf("cleared state for %s", bundleID), nil)
}

func (d *Driver) clearStateSimulator(bundleID string) error {
	// Terminate first so the reinstall isn't racing a live process.
	_ = d.simctl("terminate", d.udid, bundleID).Run()

	staged, err := d.stagedAppBundle(bundleID)
	if err != nil {
		return err
	}
	if out, err := d.simctl("uninstall", d.udid, bundleID).CombinedOutput(); err != nil {
		return fmt.Errorf("uninstall: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := d.simctl("install", d.udid, staged).CombinedOutput(); err != nil {
		return fmt.Errorf("reinstall: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// stagedAppBundle returns a copy of the installed .app, made once per bundle
// for the driver's lifetime: the uninstall deletes the original, and copying
// a large app on every clearState (DuckDuckGo's is ~700MB) dominated it. The
// copy is refreshed when the installed bundle's stamp changes — a new build
// installed mid-run.
func (d *Driver) stagedAppBundle(bundleID string) (string, error) {
	out, err := d.simctl("get_app_container", d.udid, bundleID, "app").Output()
	if err != nil {
		return "", fmt.Errorf("app %s not installed on the simulator: %w", bundleID, err)
	}
	appPath := strings.TrimSpace(string(out))
	if appPath == "" {
		return "", fmt.Errorf("could not locate the installed .app for %s", bundleID)
	}
	stamp := bundleStamp(appPath)
	if d.stagedApps == nil {
		d.stagedApps = map[string]stagedApp{}
	}
	if s, ok := d.stagedApps[bundleID]; ok && s.stamp == stamp && pathExists(s.path) {
		return s.path, nil
	}

	dir, err := os.MkdirTemp("", "dlios-clearstate-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	staged := filepath.Join(dir, filepath.Base(appPath))
	if out, err := exec.Command("cp", "-R", appPath, staged).CombinedOutput(); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("stage app bundle: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if old, ok := d.stagedApps[bundleID]; ok {
		_ = os.RemoveAll(filepath.Dir(old.path))
	}
	d.stagedApps[bundleID] = stagedApp{path: staged, stamp: stamp}
	return staged, nil
}

// stagedApp is one cached copy of an installed app bundle.
type stagedApp struct {
	path  string
	stamp string
}

// bundleStamp identifies a bundle's build by content, not time — the
// reinstalled copy gets new timestamps but must still match: a hash of the
// Info.plist and of each top-level file's name and size (the executable's
// size moves with nearly every build).
func bundleStamp(appPath string) string {
	plist, err := os.ReadFile(filepath.Join(appPath, "Info.plist"))
	if err != nil {
		return ""
	}
	h := sha256.New()
	h.Write(plist)
	entries, _ := os.ReadDir(appPath)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() {
			fmt.Fprintf(h, "\x00%s:%d", e.Name(), info.Size())
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// removeStagedApps deletes the cached bundle copies.
func (d *Driver) removeStagedApps() {
	for id, s := range d.stagedApps {
		_ = os.RemoveAll(filepath.Dir(s.path))
		delete(d.stagedApps, id)
	}
}

// containerMetadata is the file iOS keeps in every data container to know
// whose it is; it must survive a wipe.
const containerMetadata = ".com.apple.mobile_container_manager.metadata.plist"

// clearDataContainer is the fast clearState: stop the app, restart the
// preferences daemon, empty its data container (keeping the container's own
// metadata), recreate the standard directories, and reset its permissions —
// no uninstall, no reinstall.
func (d *Driver) clearDataContainer(bundleID string) error {
	_ = d.simctl("terminate", d.udid, bundleID).Run()
	out, err := d.simctl("get_app_container", d.udid, bundleID, "data").Output()
	if err != nil {
		return fmt.Errorf("app %s not installed on the simulator: %w", bundleID, err)
	}
	container := strings.TrimSpace(string(out))
	if container == "" {
		return fmt.Errorf("could not locate the data container for %s", bundleID)
	}
	if err := d.restartPrefsDaemon(); err != nil {
		return err
	}
	if err := wipeDataContainer(container); err != nil {
		return err
	}
	_ = d.simctl("privacy", d.udid, "reset", "all", bundleID).Run()
	return nil
}

// wipeDataContainer empties container except its metadata file, then puts
// back the directories an app expects to find.
func wipeDataContainer(container string) error {
	entries, err := os.ReadDir(container)
	if err != nil {
		return fmt.Errorf("read data container: %w", err)
	}
	for _, e := range entries {
		if e.Name() == containerMetadata {
			continue
		}
		if err := os.RemoveAll(filepath.Join(container, e.Name())); err != nil {
			return fmt.Errorf("clear %s: %w", e.Name(), err)
		}
	}
	for _, sub := range []string{"Documents", "Library/Caches", "Library/Preferences", "tmp"} {
		if err := os.MkdirAll(filepath.Join(container, sub), 0o755); err != nil {
			return fmt.Errorf("recreate %s: %w", sub, err)
		}
	}
	return nil
}

// cfprefsdServices name the simulator's preferences daemon for launchctl:
// the user/foreground form current runtimes ask for, then the system form
// for a runtime that does not know the first.
var cfprefsdServices = []string{
	"user/foreground/com.apple.cfprefsd.xpc.daemon",
	"system/com.apple.cfprefsd.xpc.daemon",
}

// restartPrefsDaemon kills the simulator's cfprefsd; launchd starts it again
// on demand with an empty cache. The daemon serves an app's UserDefaults from
// memory and writes them to disk later, so emptying the container alone left
// them in place for the next launch. SIGKILL, not a stop: a daemon on its way
// out flushes its pending writes. Called with the app stopped and before the
// wipe, so no deferred write can land in the emptied container. A preference
// another app has not flushed yet is lost — the price of the fast path.
func (d *Driver) restartPrefsDaemon() error {
	var errs []string
	for _, svc := range cfprefsdServices {
		out, err := d.simctl(prefsKillArgs(d.udid, svc)...).CombinedOutput()
		if err == nil || prefsDaemonNotRunning(string(out)) {
			return nil
		}
		errs = append(errs, fmt.Sprintf("%v: %s", err, strings.TrimSpace(string(out))))
	}
	return fmt.Errorf("restart the simulator's preferences daemon (UserDefaults would survive the clear): %s",
		strings.Join(errs, "; "))
}

// prefsKillArgs is the simctl command that SIGKILLs service in the simulator.
func prefsKillArgs(udid, service string) []string {
	return []string{"spawn", udid, "launchctl", "kill", "SIGKILL", service}
}

// prefsDaemonNotRunning reports launchctl refusing because the daemon is not
// running: it caches nothing, which is all the restart is for.
func prefsDaemonNotRunning(out string) bool {
	return strings.Contains(strings.ToLower(out), "no such process")
}
