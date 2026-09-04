package wda

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/forward"
	"github.com/danielpaulus/go-ios/ios/instruments"
	"github.com/devicelab-dev/maestro-runner/pkg/config"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/simulator"
)

const (
	wdaBasePort  = uint16(8100)
	wdaPortRange = uint16(1000)
	buildTimeout = 10 * time.Minute
	// startupTimeout covers the full window from invoking
	// `xcodebuild test-without-building` to WDA's FBWebServer printing
	// "ServerURLHere->...". On CI macos-latest with Xcode 26.3 + iOS 26
	// simulator, the xcodebuild bootstrap + test-runtime spawn alone
	// consumes ~75 seconds before XCTest even prints "Running tests..."
	// (vs ~10s on a fast local Mac). At 90s the budget left for WDA's
	// own startup was only ~15s — too tight, flaked ~67% of soak runs.
	// Bumped to 600s to match upstream maestro's
	// MAESTRO_DRIVER_STARTUP_TIMEOUT — sized for the slowest CI path
	// we've seen (>250s under load). Local runs see "ServerURLHere->"
	// in <30s so they pay nothing for the larger budget.
	startupTimeout = 600 * time.Second
	// maxStartupAttempts caps the retry loop in Start. On CI macos-latest
	// xcodebuild test-without-building intermittently hangs after launch —
	// emits a few "[MT] IDERunDestination" lines then never opens WDA's
	// HTTP listener. Killing it and retrying clears the hang ~80% of the
	// time, so 4 attempts (1 initial + 3 retries) reduces the effective
	// startup-fail rate from the ~40% per-attempt baseline to a fraction
	// of a percent in theory. Each failed attempt waits the stall window
	// (60s) before giving up, so 4 attempts cap at ~4 minutes worst case.
	maxStartupAttempts = 4
	// stallDetectWindow is how long waitForStartup waits with no new log
	// output before declaring xcodebuild stalled. A healthy startup emits
	// new log lines every few seconds (xcodebuild bootstrap, XCTest init,
	// then WDA's "ServerURLHere->" marker). 60s of silence is reliable
	// hung-process evidence.
	stallDetectWindow = 60 * time.Second
)

// Runner handles building and running WDA on iOS devices.
type Runner struct {
	deviceUDID          string
	teamID              string
	wdaBundleID         string
	port                uint16
	wdaPath             string
	buildDir            string
	cmd                 *exec.Cmd
	logFile             *os.File
	portForwardListener io.Closer // Port forwarding for physical devices (go-ios)
	isSimulatorCache    bool      // Cached device type
}

// NewRunner creates a new WDA runner.
// The WDA port is derived from the device UDID so each simulator gets a
// deterministic, unique port without scanning.
func NewRunner(deviceUDID, teamID, wdaBundleID string) *Runner {
	return &Runner{
		deviceUDID:  deviceUDID,
		teamID:      teamID,
		wdaBundleID: wdaBundleID,
		port:        PortFromUDID(deviceUDID),
	}
}

// Port returns the WDA port allocated for this runner's device.
func (r *Runner) Port() uint16 {
	return r.port
}

// PortFromUDID derives a deterministic port from a device UDID.
// Uses the last UUID segment (12 fully random hex chars in UUID v4),
// parsed as an integer mod 1000, added to base port 8100.
// Range: 8100–9099.
// Exported for use by CLI to check device availability before starting.
func PortFromUDID(udid string) uint16 {
	seg := udid
	if idx := strings.LastIndex(udid, "-"); idx >= 0 {
		seg = udid[idx+1:]
	}
	// Bound to the last 12 hex chars before parsing. A standard UUID's final
	// segment is already exactly 12 chars, so UUID-derived ports are unchanged;
	// a legacy 40-char hyphenless UDID would otherwise overflow uint64 in
	// ParseUint and fall back to 8100 for every device — colliding in parallel
	// runs (#129).
	if len(seg) > 12 {
		seg = seg[len(seg)-12:]
	}
	val, err := strconv.ParseUint(seg, 16, 64)
	if err != nil {
		return wdaBasePort // fallback to 8100 if the tail isn't hex
	}
	return wdaBasePort + uint16(val%uint64(wdaPortRange))
}

// Build compiles WDA for the target device.
// Uses a persistent build cache directory specific to iOS version, device type, and team ID.
func (r *Runner) Build(ctx context.Context) error {
	wdaPath, err := GetWDAPath()
	if err != nil {
		return err
	}
	r.wdaPath = wdaPath

	// Get build cache directory specific to this configuration
	r.buildDir, err = r.getBuildCacheDir()
	if err != nil {
		return fmt.Errorf("failed to get build cache directory: %w", err)
	}

	if err := os.MkdirAll(r.buildDir, 0o755); err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(r.buildDir, "logs"), 0o755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Check if already built by looking for xctestrun file
	if _, err := r.findXctestrun(); err == nil {
		// Build exists - skip rebuilding
		fmt.Printf("  ✓ Using cached WebDriverAgent build (%s)\n", filepath.Base(r.buildDir))
		return nil
	}

	// Need to build
	fmt.Println("\n  ⏳ Building WebDriverAgent for the first time...")
	fmt.Println("     This may take 5-10 minutes depending on your machine.")
	fmt.Println("     Next time it will be much faster (cached builds are reused).")
	fmt.Println()

	logPath := filepath.Join(r.buildDir, "logs", "build.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer func() {
		if err := logFile.Close(); err != nil {
			logger.Warn("failed to close build log file: %v", err)
		}
	}()

	buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()

	projectPath := filepath.Join(r.wdaPath, "WebDriverAgent.xcodeproj")

	args := []string{
		"build-for-testing",
		"-project", projectPath,
		"-scheme", "WebDriverAgentRunner",
		"-destination", r.destination(),
		"-derivedDataPath", r.derivedDataPath(),
		"-allowProvisioningUpdates",
		fmt.Sprintf("DEVELOPMENT_TEAM=%s", r.teamID),
	}
	if r.wdaBundleID != "" {
		args = append(args, fmt.Sprintf("PRODUCT_BUNDLE_IDENTIFIER=%s", r.wdaBundleID))
	}
	cmd := exec.CommandContext(buildCtx, "xcodebuild", args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build failed:\n%s\n\nFull log: %s", tailLog(logPath, 20), logPath)
	}

	if _, err := r.findXctestrun(); err != nil {
		return err
	}

	fmt.Println("WebDriverAgent build complete")
	return nil
}

// Start runs WDA on the device. Wraps a per-attempt startOnce in a retry
// loop that detects xcodebuild stalls (no log output for stallDetectWindow)
// and kills + retries up to maxStartupAttempts times. Matches the
// equivalent retry logic in pkg/driver/devicelab_ios/setup.go.
func (r *Runner) Start(ctx context.Context) error {
	xctestrun, err := r.findXctestrun()
	if err != nil {
		return err
	}

	// Check if this is a simulator or physical device
	r.isSimulatorCache, _ = r.isSimulator()

	logPath := filepath.Join(r.buildDir, "logs", "runner.log")

	var lastErr error
	for attempt := 1; attempt <= maxStartupAttempts; attempt++ {
		if attempt > 1 {
			banner := fmt.Sprintf(
				"  ⚠ WDA startup failed on attempt %d/%d: %v",
				attempt-1, maxStartupAttempts, lastErr,
			)
			fmt.Fprintln(os.Stderr, banner)
			fmt.Fprintf(os.Stderr, "  ↻ Retrying (attempt %d/%d)...\n", attempt, maxStartupAttempts)
			// Mirror banner into the runner log so the failure artifact
			// captures the full retry history. Use append mode so we
			// don't lose the previous attempt's xcodebuild output.
			if f, ferr := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644); ferr == nil {
				fmt.Fprintln(f, banner)
				fmt.Fprintf(f, "=== attempt %d/%d ===\n", attempt, maxStartupAttempts)
				_ = f.Close()
			}
			// Reset the simulator before retrying. Killing xcodebuild alone
			// doesn't unwedge a stuck CoreSimulator daemon — if the sim is
			// in a bad state every xcodebuild retry hits the same wall.
			// shutdown+boot on the same UDID clears CoreSimulator without
			// losing installed apps. Sim-only — physical devices don't
			// have a simctl equivalent (and don't suffer this CI-runner
			// wedge pattern anyway).
			if r.isSimulatorCache {
				if rerr := resetSimulator(ctx, r.deviceUDID); rerr != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ simctl reset failed: %v (continuing anyway)\n", rerr)
				}
			} else {
				// A device had no recovery step at all, on the assumption it
				// could not get wedged. A WebDriverAgentRunner left running
				// from the previous attempt still owns the device, so every
				// retry raced the session it needed to replace.
				terminateDeviceWDA(r.deviceUDID)
			}
		}

		err := r.startOnce(ctx, xctestrun, logPath, attempt)
		if err == nil {
			if attempt > 1 {
				fmt.Fprintf(os.Stderr, "  ✓ WDA started on attempt %d/%d\n", attempt, maxStartupAttempts)
			}
			// For physical devices, forward the WDA port from device to localhost.
			if !r.isSimulatorCache {
				if pferr := r.startPortForward(); pferr != nil {
					r.Stop()
					return fmt.Errorf("failed to start port forwarding: %w", pferr)
				}
			}
			fmt.Println("WebDriverAgent started")
			return nil
		}
		// Deterministic configuration failures won't be fixed by retrying —
		// report the real error immediately instead of burning ~4 blind
		// attempts (#118).
		var perm *permanentStartupError
		if errors.As(err, &perm) {
			return fmt.Errorf("WDA failed to start: %w", err)
		}
		lastErr = err
	}
	return fmt.Errorf(
		"WDA failed to start after %d attempts: %w",
		maxStartupAttempts, lastErr,
	)
}

// startOnce performs one launch+wait attempt. On stall (no log output
// for stallDetectWindow) it stops the xcodebuild subprocess and returns
// an error tagged for retry. Caller (Start) owns the retry decision.
func (r *Runner) startOnce(ctx context.Context, xctestrun, logPath string, attempt int) error {
	// Re-inject port each attempt — the xctestrun is edited in place, and
	// the previous attempt may have left a stale port if injection raced
	// with the killed subprocess.
	if err := r.injectPort(xctestrun); err != nil {
		return fmt.Errorf("failed to set WDA port in xctestrun: %w", err)
	}

	// Truncate on first attempt, append on retries so the log captures
	// the full history.
	flags := os.O_CREATE | os.O_WRONLY
	if attempt == 1 {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	logFile, err := os.OpenFile(logPath, flags, 0o644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	r.logFile = logFile

	r.cmd = exec.CommandContext(ctx, "xcodebuild",
		"test-without-building",
		"-xctestrun", xctestrun,
		"-destination", r.destination(),
		"-derivedDataPath", r.derivedDataPath(),
	)
	r.cmd.Stdout = r.logFile
	r.cmd.Stderr = r.logFile

	if attempt == 1 {
		fmt.Println("Starting WebDriverAgent...")
	}

	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start WDA: %w", err)
	}

	// Watch for the process exiting before WDA is ready. A fast-failing
	// xcodebuild (bad -destination, missing runtime, …) previously looked
	// identical to a hang: the log stopped growing and the stall detector
	// misreported it 60s later, burning blind retries (#118).
	cmd := r.cmd
	exitCh := make(chan error, 1)
	go func() { exitCh <- cmd.Wait() }()

	if err := r.waitForStartup(logPath, exitCh); err != nil {
		r.Stop()
		return err
	}
	return nil
}

// startPortForward uses go-ios to forward the WDA port from a physical device to localhost.
func (r *Runner) startPortForward() error {
	entry, err := goios.GetDevice(r.deviceUDID)
	if err != nil {
		return fmt.Errorf("device %s not found: %w", r.deviceUDID, err)
	}

	listener, err := forward.Forward(entry, r.port, r.port)
	if err != nil {
		return fmt.Errorf("port forward %d->%d failed: %w", r.port, r.port, err)
	}
	r.portForwardListener = listener

	// Give the forward a moment to establish
	time.Sleep(500 * time.Millisecond)

	return nil
}

// injectPort writes USE_PORT into the xctestrun plist's EnvironmentVariables
// so the WDA test runner process starts on the allocated port.
func (r *Runner) injectPort(xctestrunPath string) error {
	portStr := strconv.Itoa(int(r.port))

	// Convert plist to JSON for easy manipulation
	jsonData, err := exec.Command("plutil", "-convert", "json", "-o", "-", xctestrunPath).Output()
	if err != nil {
		return fmt.Errorf("failed to read xctestrun: %w", err)
	}

	var plist map[string]interface{}
	if err := json.Unmarshal(jsonData, &plist); err != nil {
		return fmt.Errorf("failed to parse xctestrun: %w", err)
	}

	// Handle format version 2 (TestConfigurations array)
	if configs, ok := plist["TestConfigurations"].([]interface{}); ok {
		for _, cfg := range configs {
			cfgMap, _ := cfg.(map[string]interface{})
			if cfgMap == nil {
				continue
			}
			targets, _ := cfgMap["TestTargets"].([]interface{})
			for _, tgt := range targets {
				setPortEnv(tgt, portStr)
			}
		}
	} else {
		// Format version 1: top-level keys are test targets
		for key, val := range plist {
			if key == "__xctestrun_metadata__" {
				continue
			}
			setPortEnv(val, portStr)
		}
	}

	result, err := json.MarshalIndent(plist, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize xctestrun: %w", err)
	}

	if err := os.WriteFile(xctestrunPath, result, 0o644); err != nil {
		return fmt.Errorf("failed to write xctestrun: %w", err)
	}

	// Convert back to XML plist format
	if out, err := exec.Command("plutil", "-convert", "xml1", xctestrunPath).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to convert xctestrun to plist: %s: %w", out, err)
	}

	return nil
}

func setPortEnv(target interface{}, portStr string) {
	tgtMap, ok := target.(map[string]interface{})
	if !ok {
		return
	}
	env, ok := tgtMap["EnvironmentVariables"].(map[string]interface{})
	if !ok {
		env = make(map[string]interface{})
		tgtMap["EnvironmentVariables"] = env
	}
	env["USE_PORT"] = portStr
}

// Stop terminates the running WDA.
func (r *Runner) Stop() {
	// Stop port forwarding if running (for physical devices)
	if r.portForwardListener != nil {
		if err := r.portForwardListener.Close(); err != nil {
			logger.Warn("failed to close port forward listener: %v", err)
		}
		r.portForwardListener = nil
	}
	if r.cmd != nil && r.cmd.Process != nil {
		if err := r.cmd.Process.Kill(); err != nil {
			logger.Warn("failed to kill WDA process: %v", err)
		}
		r.cmd = nil
	}
	if r.logFile != nil {
		if err := r.logFile.Close(); err != nil {
			logger.Warn("failed to close WDA log file: %v", err)
		}
		r.logFile = nil
	}
}

// Cleanup stops WDA runner.
// Note: Build directory is now persistent and not removed to enable build reuse.
func (r *Runner) Cleanup() {
	r.Stop()
	// Build directory is persistent (in cache), don't remove it
}

// getBuildCacheDir returns the cache directory path for this specific configuration.
// Format: ~/.maestro-runner/cache/wda-builds/{config-name}/
// Examples:
//   - Simulator: sim-ios18.5-iphone/
//   - Real device: device-ios18.0-teamABC123/
func (r *Runner) getBuildCacheDir() (string, error) {
	// Get device info
	isSimulator, err := r.isSimulator()
	if err != nil {
		return "", err
	}

	iosVersion, err := r.getIOSVersion()
	if err != nil {
		return "", err
	}

	// Generate config-specific directory name
	var configName string
	if isSimulator {
		// Simulator: sim-ios{version}-iphone
		configName = fmt.Sprintf("sim-ios%s-iphone", iosVersion)
	} else {
		// Real device: device-ios{version}-team{teamID}
		teamID := r.teamID
		if teamID == "" {
			teamID = "default"
		}
		configName = fmt.Sprintf("device-ios%s-team%s", iosVersion, teamID)
	}
	if r.wdaBundleID != "" {
		configName += "-bundle" + r.wdaBundleID
	}

	cacheDir := filepath.Join(config.GetCacheDir(), "wda-builds", configName)
	return cacheDir, nil
}

// resetSimulator shuts down then boots the given simulator. Used between
// retry attempts when xcodebuild stalls — a sim-daemon stuck in a bad
// state survives a plain xcodebuild kill, but a shutdown+boot cycle
// resets it without losing installed apps.
func resetSimulator(ctx context.Context, udid string) error {
	shutdownCmd := exec.CommandContext(ctx, "xcrun", "simctl", "shutdown", udid)
	if out, err := shutdownCmd.CombinedOutput(); err != nil {
		msg := strings.ToLower(strings.TrimSpace(string(out)))
		// Already shut down → fine.
		if !strings.Contains(msg, "shutdown") && !strings.Contains(msg, "current state:") {
			return fmt.Errorf("simctl shutdown: %w (%s)", err, msg)
		}
	}
	bootCmd := exec.CommandContext(ctx, "xcrun", "simctl", "boot", udid)
	if out, err := bootCmd.CombinedOutput(); err != nil {
		msg := strings.ToLower(strings.TrimSpace(string(out)))
		// Already booted → fine.
		if !strings.Contains(msg, "booted") && !strings.Contains(msg, "current state:") {
			return fmt.Errorf("simctl boot: %w (%s)", err, msg)
		}
	}
	// Block until boot completes; cap at 60s. Healthy sim boots in 5-15s.
	bootCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	_ = exec.CommandContext(bootCtx, "xcrun", "simctl", "bootstatus", udid, "-b").Run()
	return nil
}

// isSimulator checks if the device is a simulator.
func (r *Runner) isSimulator() (bool, error) {
	// Run simctl to check if this UDID is a simulator
	cmd := exec.Command("xcrun", "simctl", "list", "devices", "-j")
	output, err := cmd.Output()
	if err != nil {
		// If simctl fails, assume it might be a real device
		return false, nil
	}

	// Parse JSON to check if UDID exists in simulator list
	var data map[string]interface{}
	if err := json.Unmarshal(output, &data); err != nil {
		return false, err
	}

	devices, ok := data["devices"].(map[string]interface{})
	if !ok {
		return false, nil
	}

	// Check if our UDID appears in any runtime
	for _, deviceList := range devices {
		if list, ok := deviceList.([]interface{}); ok {
			for _, device := range list {
				if deviceMap, ok := device.(map[string]interface{}); ok {
					if udid, ok := deviceMap["udid"].(string); ok && udid == r.deviceUDID {
						return true, nil
					}
				}
			}
		}
	}

	return false, nil
}

// getIOSVersion returns the iOS version of the device.
func (r *Runner) getIOSVersion() (string, error) {
	// Try simctl first (for simulators)
	cmd := exec.Command("xcrun", "simctl", "list", "devices", "-j")
	output, err := cmd.Output()
	if err == nil {
		var data map[string]interface{}
		if err := json.Unmarshal(output, &data); err == nil {
			devices, ok := data["devices"].(map[string]interface{})
			if ok {
				for runtime, deviceList := range devices {
					if list, ok := deviceList.([]interface{}); ok {
						for _, device := range list {
							if deviceMap, ok := device.(map[string]interface{}); ok {
								if udid, ok := deviceMap["udid"].(string); ok && udid == r.deviceUDID {
									// Extract iOS version from runtime string
									// Example: "com.apple.CoreSimulator.SimRuntime.iOS-18-5" -> "18.5"
									parts := strings.Split(runtime, ".")
									if len(parts) > 0 {
										lastPart := parts[len(parts)-1]
										// iOS-18-5 -> 18.5
										version := strings.TrimPrefix(lastPart, "iOS-")
										version = strings.ReplaceAll(version, "-", ".")
										return version, nil
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// For real devices, use go-ios to query the device
	entry, err := goios.GetDevice(r.deviceUDID)
	if err == nil {
		if values, err := goios.GetValues(entry); err == nil && values.Value.ProductVersion != "" {
			return values.Value.ProductVersion, nil
		}
	}

	// Fallback: use a generic version identifier
	return "unknown", nil
}

func (r *Runner) destination() string {
	// On Xcode 26 + iOS 26 simulators, xcodebuild's destination resolver
	// returns BOTH arm64 and x86_64 entries for the same UDID and warns
	// "Using the first of multiple matching destinations". The cached
	// xctestrun is built arm64-only on Apple Silicon hosts, so picking
	// the x86_64 variant leaves testmanagerd never spawning the test
	// bundle and `xcodebuild test-without-building` stalls past 90s
	// without ever emitting ServerURLHere. Pin platform + arch explicitly
	// so the resolver gets a single concrete destination.
	isSim, _ := r.isSimulator()
	if isSim {
		return fmt.Sprintf("platform=iOS Simulator,arch=%s,id=%s", simulator.XcodebuildArch(runtime.GOARCH), r.deviceUDID)
	}
	// A physical device has the same ambiguity: the resolver lists both arm64
	// and arm64e for one iPhone and warns "Using the first of multiple
	// matching destinations". The pin above was simulator-only, so devices
	// kept the coin flip — and a wrong pick stalls identically, which is what
	// "xcodebuild stalled (no log output)" on a real device looks like.
	//
	// arm64 is the slice WDA is built as. MAESTRO_WDA_DEST_ARCH overrides it,
	// and setting it to "any" drops the pin entirely, so a device whose
	// resolver disagrees can be recovered without a new binary.
	arch := os.Getenv("MAESTRO_WDA_DEST_ARCH")
	if arch == "" {
		arch = "arm64"
	}
	if strings.EqualFold(arch, "any") {
		return fmt.Sprintf("platform=iOS,id=%s", r.deviceUDID)
	}
	return fmt.Sprintf("platform=iOS,arch=%s,id=%s", arch, r.deviceUDID)
}

// wdaRunnerBundleID is the XCTest runner installed on the device by the WDA
// build. Its process is what survives a host-side `xcodebuild` kill.
const wdaRunnerBundleID = "WebDriverAgentRunner-Runner"

// terminateDeviceWDA kills a WebDriverAgentRunner left running on a physical
// device.
//
// Killing xcodebuild on the host does not end the XCTest session on the phone:
// the runner app stays resident holding the port, and the next
// `test-without-building` attempts to start a second session while the first
// still owns the device. That is the shape of "a few runs succeed, then every
// run stalls" — nothing is cleaned up between them, so the failure is
// cumulative rather than transient, and retrying without clearing it hits the
// same wall every time.
//
// Best-effort by design: every failure here is logged and swallowed. This runs
// on the recovery path, where the run is already failing, and a device that
// will not answer the instruments channel must not turn a retryable stall into
// a hard error.
func terminateDeviceWDA(udid string) {
	device, err := goios.GetDevice(udid)
	if err != nil {
		logger.Debug("device WDA cleanup: no device %s: %v", udid, err)
		return
	}
	info, err := instruments.NewDeviceInfoService(device)
	if err != nil {
		logger.Debug("device WDA cleanup: device info service: %v", err)
		return
	}
	defer info.Close()

	procs, err := info.ProcessList()
	if err != nil {
		logger.Debug("device WDA cleanup: process list: %v", err)
		return
	}

	pc, err := instruments.NewProcessControl(device)
	if err != nil {
		logger.Debug("device WDA cleanup: process control: %v", err)
		return
	}
	defer func() { _ = pc.Close() }()

	for _, proc := range procs {
		if !strings.Contains(proc.Name, "WebDriverAgentRunner") {
			continue
		}
		if err := pc.KillProcess(proc.Pid); err != nil {
			logger.Debug("device WDA cleanup: kill %s (pid %d): %v", proc.Name, proc.Pid, err)
			continue
		}
		logger.Info("[wda] terminated leftover %s (pid %d) on device", proc.Name, proc.Pid)
	}
}

func (r *Runner) derivedDataPath() string {
	return filepath.Join(r.buildDir, "DerivedData")
}

func (r *Runner) findXctestrun() (string, error) {
	pattern := filepath.Join(r.derivedDataPath(), "Build", "Products", "*.xctestrun")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return "", fmt.Errorf("no xctestrun file found in %s", filepath.Dir(pattern))
	}
	return matches[0], nil
}

func (r *Runner) waitForStartup(logPath string, exit <-chan error) error {
	timeout := time.After(startupTimeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Stall detection — if the log file isn't growing, xcodebuild is hung
	// (most commonly waiting on the Xcode 26 + iOS 26 sim destination
	// resolver). Bail early so the caller can kill + retry instead of
	// waiting the full 300s startupTimeout.
	var lastLogSize int64 = -1
	lastLogActivity := time.Now()

	for {
		select {
		case werr := <-exit:
			// xcodebuild exited before WDA became ready — WDA runs inside
			// the xcodebuild process, so this is always a failure even if
			// the log shows the ready marker. Report the real error from
			// the log instead of misdiagnosing the ensuing silence as a
			// stall (#118). checkLog may classify it as permanent
			// (`xcodebuild: error:`), which skips retries.
			content, _ := os.ReadFile(logPath)
			if cerr := r.checkLog(string(content), logPath); cerr != errNotReady && cerr != nil {
				return cerr
			}
			status := "exited unexpectedly (status 0)"
			if werr != nil {
				status = fmt.Sprintf("exited: %v", werr)
			}
			return fmt.Errorf(
				"xcodebuild %s before WDA became ready:\n%s\n\nFull log: %s",
				status, tailLog(logPath, 20), logPath,
			)
		case <-ticker.C:
			content, err := os.ReadFile(logPath)
			if err != nil {
				continue
			}
			if err := r.checkLog(string(content), logPath); err != errNotReady {
				return err
			}
			// Did the file grow since last tick? If not for stallDetectWindow,
			// xcodebuild is stalled — surface as a retryable error.
			size := int64(len(content))
			if size != lastLogSize {
				lastLogSize = size
				lastLogActivity = time.Now()
			} else if time.Since(lastLogActivity) > stallDetectWindow {
				return fmt.Errorf(
					"xcodebuild stalled (no log output for %v):\n%s\n\nFull log: %s",
					stallDetectWindow.Round(time.Second), tailLog(logPath, 20), logPath,
				)
			}
		case <-timeout:
			return fmt.Errorf("WDA startup timeout (%s):\n%s\n\nFull log: %s", startupTimeout, tailLog(logPath, 20), logPath)
		}
	}
}

var errNotReady = fmt.Errorf("not ready")

func (r *Runner) checkLog(log, logPath string) error {
	// Success indicators
	if strings.Contains(log, "ServerURLHere") || strings.Contains(log, "WebDriverAgent") && strings.Contains(log, "started") {
		return nil
	}

	// Known errors
	if strings.Contains(log, "Developer App Certificate is not trusted") {
		return fmt.Errorf("certificate not trusted - trust it in Settings > General > VPN & Device Management")
	}
	if strings.Contains(log, "Code Sign error") {
		return fmt.Errorf("code signing failed - check your DEVELOPMENT_TEAM and provisioning profiles")
	}
	// Generic xcodebuild errors (bad -destination, missing runtime, …)
	// are deterministic configuration failures: surface the actual error
	// line and skip the retry loop (#118).
	if line := firstLineContaining(log, "xcodebuild: error:"); line != "" {
		return &permanentStartupError{fmt.Errorf("%s\n\nFull log: %s", line, logPath)}
	}
	if strings.Contains(log, "Testing failed:") {
		return fmt.Errorf("WDA failed:\n%s\n\nFull log: %s", tailLog(logPath, 20), logPath)
	}

	return errNotReady
}

// permanentStartupError marks startup failures that retrying cannot fix
// (bad -destination, missing simulator runtime, …). Start stops the retry
// loop as soon as it sees one instead of burning further attempts (#118).
type permanentStartupError struct{ err error }

func (e *permanentStartupError) Error() string { return e.err.Error() }
func (e *permanentStartupError) Unwrap() error { return e.err }

// firstLineContaining returns the first log line containing substr,
// trimmed, or "" when absent.
func firstLineContaining(log, substr string) string {
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, substr) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func tailLog(path string, lines int) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("(could not read log: %s)", err)
	}
	allLines := strings.Split(string(content), "\n")
	if len(allLines) <= lines {
		return string(content)
	}
	return strings.Join(allLines[len(allLines)-lines:], "\n")
}
