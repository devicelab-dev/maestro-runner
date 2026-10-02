package devicelab_ios

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/simulator"
)

// Everything outside the app's UI goes through simctl on the host: it is
// faster than the agent and works while the app is not running.

// fastClearStateEnv opts into clearing an app by emptying its data container
// instead of reinstalling it: much faster for a large app, but the keychain,
// App Group containers and permissions survive. Maestro reinstalls, so that
// stays the default.
const fastClearStateEnv = "DEVICELAB_IOS_FAST_CLEARSTATE"

// SimPrefsEnv turns the simulator preferences off when set to "0".
const SimPrefsEnv = "DEVICELAB_IOS_SIM_PREFS"

// simctlStdin builds `xcrun simctl pbcopy <udid>` fed with text; tests
// replace it.
var simctlStdin = func(udid, text string) *exec.Cmd {
	cmd := exec.Command("xcrun", "simctl", "pbcopy", udid)
	cmd.Stdin = strings.NewReader(text)
	return cmd
}

// simctlEnv runs `xcrun simctl args…` with extra environment; tests replace
// it.
var simctlEnv = func(env []string, args ...string) (string, error) {
	cmd := exec.Command("xcrun", append([]string{"simctl"}, args...)...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("simctl %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (d *Driver) bundleOr(appID string) (string, error) {
	bid := strings.TrimSpace(appID)
	if bid == "" {
		bid = d.appID
	}
	if bid == "" {
		return "", fmt.Errorf("no appId: set appId in the flow header or on the step")
	}
	return bid, nil
}

// ---------- launch and stop ----------

// launchApp follows Maestro's order: clear state, permissions (all:allow by
// default), keychain, stop, then launch with arguments and environment.
func (d *Driver) launchApp(s *flow.LaunchAppStep) *core.CommandResult {
	bid, err := d.bundleOr(s.AppID)
	if err != nil {
		return core.ErrorResult(err, err.Error())
	}
	d.appID = bid
	if d.realDevice {
		return d.launchAppOnDevice(s, bid)
	}
	if s.ClearState {
		if res := d.clearState(bid); !res.Success {
			return res
		}
	}
	d.applyLaunchPermissions(bid, s.Permissions)
	if s.ClearKeychain {
		if _, err := d.runSimctl("keychain", d.udid, "reset"); err != nil {
			logger.Warn("launchApp: clearKeychain skipped: %v", err)
		}
	}
	if s.StopApp == nil || *s.StopApp {
		_ = d.terminate(bid)
	}
	args := []string{"launch", "--terminate-running-process", d.udid, bid}
	args = append(args, flattenArguments(s.Arguments)...)
	if _, err := simctlEnv(launchEnv(s.Environment), args...); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("launchApp failed: %v", err))
	}
	return core.SuccessResult("launched "+bid, nil)
}

// launchEnv is the host's environment (xcrun needs PATH, DEVELOPER_DIR) plus
// the app's variables under the SIMCTL_CHILD_ prefix simctl forwards.
func launchEnv(appEnv map[string]string) []string {
	env := os.Environ()
	keys := make([]string, 0, len(appEnv))
	for k := range appEnv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, "SIMCTL_CHILD_"+k+"="+appEnv[k])
	}
	return env
}

// flattenArguments turns launch arguments into a command line the way
// Maestro's iOS driver does (IOSLaunchArguments.kt): a boolean keeps its key
// as written (`autoclear-ui-test true`), any other value gets a `-` prefix
// unless it has one (`-cartValue 3`, a UserDefaults argument). Apps test
// flags with ProcessInfo.arguments.contains("key"); a dash on a boolean key
// hid it (DDG's autoclear-ui-test never took effect). Keys are sorted so the
// command line is stable.
func flattenArguments(args map[string]any) []string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(args)*2)
	for _, k := range keys {
		v := args[k]
		key := k
		if _, isBool := v.(bool); !isBool && !strings.HasPrefix(k, "-") {
			key = "-" + k
		}
		out = append(out, key, fmt.Sprintf("%v", v))
	}
	return out
}

// terminate stops an app; one that is not running counts as stopped.
func (d *Driver) terminate(bid string) error {
	if d.realDevice {
		return d.terminateOnDevice(bid)
	}
	out, err := d.runSimctl("terminate", d.udid, bid)
	if err == nil {
		return nil
	}
	body := strings.ToLower(out + " " + err.Error())
	if strings.Contains(body, "found nothing") || strings.Contains(body, "no such process") {
		return nil
	}
	return err
}

// stopApp serves stopApp and killApp: a simulator has no separate "killed by
// the system", as in the WDA driver.
func (d *Driver) stopApp(appID string) *core.CommandResult {
	bid, err := d.bundleOr(appID)
	if err != nil {
		return core.ErrorResult(err, err.Error())
	}
	if err := d.terminate(bid); err != nil {
		return core.ErrorResult(err, err.Error())
	}
	return core.SuccessResult("stopped "+bid, nil)
}

// ---------- state, keychain, permissions ----------

// clearState reinstalls the app from a cached copy of its bundle (Maestro's
// semantics), or empties its data container with DEVICELAB_IOS_FAST_CLEARSTATE=1.
func (d *Driver) clearState(appID string) *core.CommandResult {
	bid, err := d.bundleOr(appID)
	if err != nil {
		return core.ErrorResult(err, err.Error())
	}
	if d.realDevice {
		return d.clearStateOnDevice(bid)
	}
	clear := d.reinstall
	if os.Getenv(fastClearStateEnv) == "1" {
		clear = d.wipeData
	}
	if err := clear(bid); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("clearState failed: %v", err))
	}
	return core.SuccessResult("cleared state for "+bid, nil)
}

func (d *Driver) reinstall(bid string) error {
	_ = d.terminate(bid)
	staged, err := d.stagedAppBundle(bid)
	if err != nil {
		return err
	}
	if _, err := d.runSimctl("uninstall", d.udid, bid); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	if _, err := d.runSimctl("install", d.udid, staged); err != nil {
		return fmt.Errorf("reinstall: %w", err)
	}
	return nil
}

// stagedApp is one cached copy of an installed app bundle.
type stagedApp struct {
	path  string
	stamp string
}

// stagedAppBundle copies the installed .app once per build for the driver's
// lifetime: the uninstall deletes the original, and copying a large app on
// every clearState dominated it.
func (d *Driver) stagedAppBundle(bid string) (string, error) {
	out, err := d.runSimctl("get_app_container", d.udid, bid, "app")
	appPath := strings.TrimSpace(out)
	if err != nil || appPath == "" {
		return "", fmt.Errorf("app %s is not installed on the simulator", bid)
	}
	stamp := bundleStamp(appPath)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stagedApps == nil {
		d.stagedApps = map[string]stagedApp{}
	}
	if s, ok := d.stagedApps[bid]; ok && s.stamp == stamp && pathExists(s.path) {
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
	if old, ok := d.stagedApps[bid]; ok {
		_ = os.RemoveAll(filepath.Dir(old.path))
	}
	d.stagedApps[bid] = stagedApp{path: staged, stamp: stamp}
	return staged, nil
}

// bundleStamp identifies a build by content: the Info.plist plus each
// top-level file's name and size.
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
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, s := range d.stagedApps {
		_ = os.RemoveAll(filepath.Dir(s.path))
		delete(d.stagedApps, id)
	}
}

// containerMetadata is the file iOS keeps in every data container to know
// whose it is; it must survive a wipe.
const containerMetadata = ".com.apple.mobile_container_manager.metadata.plist"

// wipeData is the fast clearState: stop, empty the data container, reset
// permissions — no reinstall.
func (d *Driver) wipeData(bid string) error {
	_ = d.terminate(bid)
	out, err := d.runSimctl("get_app_container", d.udid, bid, "data")
	container := strings.TrimSpace(out)
	if err != nil || container == "" {
		return fmt.Errorf("app %s is not installed on the simulator", bid)
	}
	if err := wipeDataContainer(container); err != nil {
		return err
	}
	_, _ = d.runSimctl("privacy", d.udid, "reset", "all", bid)
	return nil
}

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

func (d *Driver) clearKeychain() *core.CommandResult {
	if d.realDevice {
		err := errOnDevice("clearKeychain")
		return core.ErrorResult(err, err.Error()+" (the keychain is sandboxed; clearState reinstalls the app, which drops its entries)")
	}
	if _, err := d.runSimctl("keychain", d.udid, "reset"); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("clearKeychain failed: %v", err))
	}
	return core.SuccessResult("keychain cleared", nil)
}

// applyLaunchPermissions resets the app's permissions, then applies the
// flow's (all:allow when it names none). Failures are logged: a permission
// simctl cannot set must not stop the launch.
func (d *Driver) applyLaunchPermissions(bid string, perms map[string]string) {
	if len(perms) == 0 {
		perms = map[string]string{"all": "allow"}
	}
	if _, err := d.runSimctl("privacy", d.udid, "reset", "all", bid); err != nil {
		logger.Warn("launchApp: permission reset failed: %v", err)
	}
	set := map[string]string{}
	for name, value := range perms {
		if !strings.EqualFold(strings.TrimSpace(value), "unset") {
			set[name] = value
		}
	}
	if _, failures := d.applyPermissions(bid, set); len(failures) > 0 {
		logger.Warn("launchApp: permissions not applied: %s", strings.Join(failures, "; "))
	}
}

// applyPermissions sets each permission with `simctl privacy`, one service
// per call, in a stable order. Names iOS gives the host no control over
// (notifications, faceid) are logged and skipped.
func (d *Driver) applyPermissions(bid string, perms map[string]string) (applied int, failures []string) {
	names := make([]string, 0, len(perms))
	for name := range perms {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := perms[name]
		services := core.IOSPrivacyServices(name)
		if len(services) == 0 {
			logger.Warn("permissions: iOS has no host-side control over %q — skipping", name)
			continue
		}
		for _, service := range services {
			action, resolved, ok := core.IOSPrivacyAction(service, value)
			if !ok {
				logger.Warn("permissions: ignoring unsupported value %q for permission %q", value, name)
				continue
			}
			if _, err := d.runSimctl("privacy", d.udid, action, resolved, bid); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", resolved, err))
				continue
			}
			applied++
		}
	}
	return applied, failures
}

func (d *Driver) setPermissions(s *flow.SetPermissionsStep) *core.CommandResult {
	bid, err := d.bundleOr(s.AppID)
	if err != nil {
		return core.ErrorResult(err, err.Error())
	}
	if len(s.Permissions) == 0 {
		err := fmt.Errorf("setPermissions needs at least one permission")
		return core.ErrorResult(err, err.Error())
	}
	if d.realDevice {
		err := errOnDevice("setPermissions")
		return core.ErrorResult(err, err.Error())
	}
	applied, failures := d.applyPermissions(bid, s.Permissions)
	if len(failures) > 0 {
		err := fmt.Errorf("some permissions failed: %s", strings.Join(failures, "; "))
		return core.ErrorResult(err, fmt.Sprintf("Permissions: %d applied, %v", applied, err))
	}
	return core.SuccessResult(fmt.Sprintf("Permissions updated: %d", applied), nil)
}

// ---------- device ----------

// openLink opens a URL with simctl. The launch it causes is asynchronous, so
// the next step's settle waits for it; autoVerify adds Maestro's grace time.
func (d *Driver) openLink(link string, autoVerify *bool) *core.CommandResult {
	if strings.TrimSpace(link) == "" {
		err := fmt.Errorf("no link specified")
		return core.ErrorResult(err, err.Error())
	}
	if d.realDevice {
		if _, err := d.call("device", &Args{Action: "openURL", Value: link}); err != nil {
			return core.ErrorResult(err, fmt.Sprintf("openLink failed: %v", err))
		}
	} else if _, err := d.runSimctl("openurl", d.udid, link); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("openLink failed: %v", err))
	}
	if autoVerify != nil && *autoVerify {
		time.Sleep(2 * time.Second)
	}
	d.settle(defaultSettleTimeout)
	return core.SuccessResult("opened link: "+link, nil)
}

func (d *Driver) setLocation(lat, lon string) *core.CommandResult {
	la, err := strconv.ParseFloat(strings.TrimSpace(lat), 64)
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("invalid latitude %q", lat))
	}
	lo, err := strconv.ParseFloat(strings.TrimSpace(lon), 64)
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("invalid longitude %q", lon))
	}
	if d.realDevice {
		err := errOnDevice("setLocation")
		return core.ErrorResult(err, err.Error())
	}
	if _, err := d.runSimctl("location", d.udid, "set", fmt.Sprintf("%f,%f", la, lo)); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("setLocation failed: %v", err))
	}
	return core.SuccessResult(fmt.Sprintf("location set to %f,%f", la, lo), nil)
}

func (d *Driver) setOrientation(orientation string) *core.CommandResult {
	value := strings.ToLower(strings.TrimSpace(orientation))
	resp, err := d.call("device", &Args{Action: "orientation", Value: value})
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("setOrientation failed: %v", err))
	}
	d.settle(defaultSettleTimeout)
	return core.SuccessResult("orientation "+resp.payload().Orientation, nil)
}

func (d *Driver) appearance(value string) (bool, error) {
	resp, err := d.call("device", &Args{Action: "appearance", Value: value})
	if err != nil {
		return false, err
	}
	got := resp.payload().Appearance
	if got == "" {
		return false, fmt.Errorf("agent returned no appearance")
	}
	return core.ParseIOSAppearance(got)
}

func (d *Driver) setDarkMode(dark bool) *core.CommandResult {
	got, err := d.appearance(core.IOSAppearanceValue(dark))
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("Failed to set dark mode: %v", err))
	}
	if got != dark {
		err := fmt.Errorf("requested %s mode but the device is in %s mode",
			core.DarkModeStateName(dark), core.DarkModeStateName(got))
		return core.ErrorResult(err, err.Error())
	}
	return core.SuccessResult(fmt.Sprintf("Set %s mode", core.DarkModeStateName(dark)), nil)
}

func (d *Driver) toggleDarkMode() *core.CommandResult {
	current, err := d.appearance("")
	if err != nil {
		return core.ErrorResult(err, err.Error())
	}
	return d.setDarkMode(!current)
}

func (d *Driver) assertDarkMode(want bool) *core.CommandResult {
	got, err := d.appearance("")
	if err != nil {
		return core.ErrorResult(err, err.Error())
	}
	if got != want {
		aerr := core.DarkModeAssertionError(want, got)
		return core.ErrorResult(aerr, aerr.Error())
	}
	return core.SuccessResult(fmt.Sprintf("Device is in %s mode", core.DarkModeStateName(want)), nil)
}

// addMedia puts photos and videos in the Photos library (simctl addmedia)
// and documents in "On My iPhone" storage.
func (d *Driver) addMedia(files []string) *core.CommandResult {
	if d.realDevice {
		err := errOnDevice("addMedia")
		return core.ErrorResult(err, err.Error())
	}
	if err := core.ValidateMediaFiles(files); err != nil {
		return core.ErrorResult(err, err.Error())
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			return core.ErrorResult(err, fmt.Sprintf("Media file not found: %s", f))
		}
	}
	media, documents := core.SplitMediaDocuments(files)
	if len(media) > 0 {
		if err := d.addMediaToPhotos(media); err != nil {
			return core.ErrorResult(err, fmt.Sprintf("Failed to add media: %v", err))
		}
	}
	if len(documents) > 0 {
		if err := simulator.AddDocuments(d.udid, documents); err != nil {
			return core.ErrorResult(err, fmt.Sprintf("Failed to add documents: %v", err))
		}
	}
	return core.SuccessResult(fmt.Sprintf("Added %d media file(s) to the simulator", len(files)), nil)
}

// addMediaWaits bound `simctl addmedia`. On GitHub's macOS runners it hung
// until the 5-minute simctl limit on every attempt of a flow, while on a new
// local simulator it took 3s the first time and no time after: the first
// import into the Photos library pays a setup cost. The first attempt gets a
// minute, a second one longer, then the step fails saying so.
var addMediaWaits = []time.Duration{60 * time.Second, 120 * time.Second}

func (d *Driver) addMediaToPhotos(media []string) error {
	var err error
	for i, wait := range addMediaWaits {
		if _, err = d.runSimctlWithin(wait, append([]string{"addmedia", d.udid}, media...)...); err == nil {
			return nil
		}
		logger.Warn("simctl addmedia attempt %d of %d failed after up to %s: %v", i+1, len(addMediaWaits), wait, err)
	}
	return fmt.Errorf("simctl addmedia did not finish (%d attempts, up to %s): %w",
		len(addMediaWaits), addMediaWaits[len(addMediaWaits)-1], err)
}

// simPrefs are the settings Appium applies to a simulator for automation;
// they stay in the simulator after the run.
var simPrefs = []struct{ domain, key, kind, value, why string }{
	{"com.apple.Accessibility", "ReduceMotionEnabled", "-int", "1", "Reduce Motion on"},
	// Appium's key; not yet checked on an iOS 26 simulator (an unknown key
	// is inert).
	{"com.apple.WebUI", "AutoFillPasswords", "-int", "0", "password AutoFill off"},
}

// ApplySimulatorPrefs writes simPrefs into a booted simulator. Set
// DEVICELAB_IOS_SIM_PREFS=0 to leave it as it is. Failures are logged.
func ApplySimulatorPrefs(udid string) {
	if os.Getenv(SimPrefsEnv) == "0" || udid == "" {
		return
	}
	var applied []string
	for _, p := range simPrefs {
		if _, err := simctl(context.Background(), 30*time.Second, "spawn", udid, "defaults", "write", p.domain, p.key, p.kind, p.value); err != nil {
			logger.Warn("simulator pref %s.%s not set: %v", p.domain, p.key, err)
			continue
		}
		applied = append(applied, p.why)
	}
	if len(applied) > 0 {
		logger.Info("Simulator prefs: %s (%s=0 to skip)", strings.Join(applied, ", "), SimPrefsEnv)
	}
}
