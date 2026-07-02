package devicelab_ios

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// SetupOptions configures runner launch.
type SetupOptions struct {
	// ArtifactsDir contains the built runner. Expected layout:
	//   <ArtifactsDir>/Build/Products/*.xctestrun
	//   <ArtifactsDir>/Build/Products/Debug-iphonesimulator/DevicelabIOSRunner.app
	//   <ArtifactsDir>/Build/Products/Debug-iphonesimulator/DevicelabIOSRunnerUITests-Runner.app
	// In dev: $HOME/.devicelab-ios-runner/derived. In shipped builds the
	// CLI will point at drivers/ios/devicelab-ios-runner under the release
	// bundle.
	ArtifactsDir string

	// SimulatorUDID is the booted iOS simulator's UDID. Required.
	SimulatorUDID string

	// HostBundleID identifies the placeholder app the runner is hosted by.
	// Default "dev.devicelab.runner". Used to verify install.
	HostBundleID string

	// Port the runner should listen on. If 0, we pick an ephemeral port.
	Port int

	// Stdout / Stderr for xcodebuild output. Default os.Stderr.
	Stdout io.Writer
	Stderr io.Writer

	// ReadyTimeout caps how long to wait for the runner to start listening.
	// Default 60s — XCUITest cold-starts the AccessibilityFramework which
	// can take 10-20s on slow machines.
	ReadyTimeout time.Duration
}

// RunnerHandle owns the running xcodebuild subprocess and the chosen port.
type RunnerHandle struct {
	cmd  *exec.Cmd
	port int
	host string
}

// Port returns the resolved listen port.
func (h *RunnerHandle) Port() int { return h.port }

// Host returns the host the runner is reachable on (always 127.0.0.1 for
// sim; would be tunneled for real device).
func (h *RunnerHandle) Host() string { return h.host }

// Stop terminates the runner subprocess. Caller typically also issues a
// `shutdown` command first to let the runner exit cleanly; this is the
// fallback.
func (h *RunnerHandle) Stop() error {
	if h == nil || h.cmd == nil || h.cmd.Process == nil {
		return nil
	}
	// Send SIGTERM first; force-kill after 5s.
	_ = h.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = h.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = h.cmd.Process.Kill()
		<-done
	}
	return nil
}

// maxStartupAttempts caps the retry loop in Setup. On CI macos-latest
// xcodebuild test-without-building intermittently hangs after launch —
// emits a few "[MT] IDERunDestination" lines then never opens the
// runner's HTTP listener. Killing it and retrying clears the hang ~80%
// of the time, so 4 attempts (1 initial + 3 retries) reduces the
// effective startup-fail rate from the ~20% per-attempt baseline to
// (0.2)^4 ≈ 0.16% in theory. Each failed attempt waits the stall
// detection window (60s) before giving up, so 4 attempts caps the
// total startup cost at roughly 4 minutes worst case.
const maxStartupAttempts = 4

// stallDetectWindow is how long awaitReady waits with no new log output
// before declaring xcodebuild stalled. 60s is well past the normal
// startup chatter — a healthy run emits new log lines every few seconds
// (test discovery, XCTest framework init, our XCTest output) — but well
// under the 5-minute ReadyTimeout so we fail fast on real hangs.
const stallDetectWindow = 60 * time.Second

// Setup launches the runner on the simulator. Returns a Client wired to
// the chosen port and a Handle for shutdown. On error, any partial state
// is rolled back. Wraps a per-attempt startOnce in a retry loop that
// detects xcodebuild stalls (no log output for stallDetectWindow) and
// kills + retries up to maxStartupAttempts times.
func Setup(ctx context.Context, opts SetupOptions) (*Client, *RunnerHandle, error) {
	if opts.ArtifactsDir == "" {
		return nil, nil, errors.New("ArtifactsDir is required")
	}
	if opts.SimulatorUDID == "" {
		return nil, nil, errors.New("SimulatorUDID is required")
	}
	if opts.HostBundleID == "" {
		opts.HostBundleID = "dev.devicelab.runner"
	}

	// Route xcodebuild output to a per-build log file so the runner's
	// `t = X.Xs Find the Window…` chatter doesn't drown the user's
	// console, AND so the retry loop can monitor the log file for
	// stall detection. Callers can override via opts.Stdout/Stderr
	// if they want it inline (e.g. for debugging) — in that case we
	// skip stall detection (caller's writers aren't easily monitorable).
	var logPath string
	if opts.Stdout == nil || opts.Stderr == nil {
		logsDir := filepath.Join(opts.ArtifactsDir, "logs")
		_ = os.MkdirAll(logsDir, 0o755)
		logPath = filepath.Join(logsDir, "runner.log")
		logFile, err := os.Create(logPath)
		if err != nil {
			// Fall back to stderr — better some output than blocking startup.
			if opts.Stdout == nil {
				opts.Stdout = os.Stderr
			}
			if opts.Stderr == nil {
				opts.Stderr = os.Stderr
			}
			logPath = ""
		} else {
			if opts.Stdout == nil {
				opts.Stdout = logFile
			}
			if opts.Stderr == nil {
				opts.Stderr = logFile
			}
		}
	}
	if opts.ReadyTimeout == 0 {
		opts.ReadyTimeout = 600 * time.Second
	}

	xctestrun, err := findXctestrun(opts.ArtifactsDir)
	if err != nil {
		return nil, nil, err
	}

	hostAppPath := filepath.Join(opts.ArtifactsDir, "Build/Products/Debug-iphonesimulator/DevicelabIOSRunner.app")
	if err := simctlInstall(ctx, opts.SimulatorUDID, hostAppPath); err != nil {
		return nil, nil, fmt.Errorf("install host app: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= maxStartupAttempts; attempt++ {
		if attempt > 1 {
			// User-visible + log retry banner.
			banner := fmt.Sprintf(
				"  ⚠ devicelab runner stalled on attempt %d/%d: %v",
				attempt-1, maxStartupAttempts, lastErr,
			)
			fmt.Fprintln(os.Stderr, banner)
			fmt.Fprintf(os.Stderr, "  ↻ Retrying (attempt %d/%d)...\n", attempt, maxStartupAttempts)
			// Mirror into the runner log so the artifact captures the full
			// retry history (logFile may be closed if we hit the fallback
			// branch above; guard before writing).
			if opts.Stdout != os.Stderr {
				fmt.Fprintln(opts.Stdout, banner)
				fmt.Fprintf(opts.Stdout, "=== attempt %d/%d ===\n", attempt, maxStartupAttempts)
			}
			// Reset the simulator before retrying. Killing xcodebuild
			// alone doesn't unwedge a stuck CoreSimulator daemon — if
			// the sim itself is in a bad state, every xcodebuild retry
			// hits the same wall. A shutdown+boot cycle on the same
			// UDID clears CoreSimulator process state without losing
			// installed apps (those live in the sim's data container).
			if rerr := resetSimulator(ctx, opts.SimulatorUDID, opts.Stdout); rerr != nil {
				// Best-effort: log and continue. If reset fails the
				// retry attempt will reveal whether the sim is still
				// usable.
				fmt.Fprintf(os.Stderr, "  ⚠ simctl reset failed: %v (continuing anyway)\n", rerr)
			}
		}

		client, handle, err := startOnce(ctx, opts, xctestrun, logPath)
		if err == nil {
			if attempt > 1 {
				fmt.Fprintf(os.Stderr, "  ✓ Runner started on attempt %d/%d\n", attempt, maxStartupAttempts)
			}
			return client, handle, nil
		}
		lastErr = err
	}
	return nil, nil, fmt.Errorf(
		"runner not ready after %d attempts: %w",
		maxStartupAttempts, lastErr,
	)
}

// startOnce performs one attempt at launching xcodebuild + waiting for
// the runner's HTTP listener. On stall (no log output for
// stallDetectWindow) it kills xcodebuild and returns an error tagged
// for retry. On other errors it also kills + returns. Caller (Setup)
// owns the retry decision.
func startOnce(ctx context.Context, opts SetupOptions, xctestrun, logPath string) (*Client, *RunnerHandle, error) {
	// Fresh port per attempt — the previous attempt's killed XCTest may
	// still hold the old port in TIME_WAIT on the simulator side.
	port, err := pickEphemeralPort()
	if err != nil {
		return nil, nil, fmt.Errorf("pick port: %w", err)
	}
	if err := injectPortIntoXctestrun(xctestrun, port); err != nil {
		return nil, nil, fmt.Errorf("inject port: %w", err)
	}

	// Pin arch in the destination string. On Xcode 26 + iOS 26 simulators,
	// xcodebuild's destination resolver returns BOTH arm64 and x86_64
	// entries for the same UDID and warns "Using the first of multiple
	// matching destinations". When the resolver picks ambiguously it can
	// hang for minutes — observed as ~40% startup-fail rate on CI before
	// this pin (same root cause as the WDA fix in 822a511).
	destination := fmt.Sprintf(
		"platform=iOS Simulator,arch=%s,id=%s",
		runtime.GOARCH, opts.SimulatorUDID,
	)
	cmd := exec.Command(
		"xcodebuild",
		"test-without-building",
		"-xctestrun", xctestrun,
		"-destination", destination,
	)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start xcodebuild: %w", err)
	}

	handle := &RunnerHandle{cmd: cmd, port: port, host: "127.0.0.1"}
	client := NewClient(handle.host, port)

	if err := awaitReady(ctx, client, opts.ReadyTimeout, logPath); err != nil {
		_ = handle.Stop()
		return nil, nil, err
	}

	// Pre-warm XCTest's accessibility framework + screenshot path so the
	// first test step doesn't pay the cold-start cost (typically ~1-2s
	// for the first descendants() walk and the first XCUIScreen capture).
	// Best-effort: ignore errors, the actual test will surface real ones.
	warmCtx, warmCancel := context.WithTimeout(ctx, 5*time.Second)
	_, _ = client.Call(warmCtx, Command{Command: CmdScreenshot})
	_, _ = client.Call(warmCtx, Command{Command: CmdSnapshot})
	warmCancel()

	return client, handle, nil
}

// findXctestrun locates the .xctestrun file under <artifactsDir>/Build/Products/.
// Filename varies with arch + iOS version, so we glob.
func findXctestrun(artifactsDir string) (string, error) {
	pattern := filepath.Join(artifactsDir, "Build/Products/*.xctestrun")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no .xctestrun found under %s", pattern)
	}
	return matches[0], nil
}

// injectPortIntoXctestrun edits the xctestrun's nested
// :TestConfigurations:0:TestTargets:0:EnvironmentVariables:DEVICELAB_IOS_RUNNER_PORT
// path so the runner picks up our chosen port at launch.
func injectPortIntoXctestrun(path string, port int) error {
	const key = ":TestConfigurations:0:TestTargets:0:EnvironmentVariables:DEVICELAB_IOS_RUNNER_PORT"
	// Try Add first; if already present, fall through to Set.
	add := exec.Command("/usr/libexec/PlistBuddy",
		"-c", fmt.Sprintf("Add %s string %d", key, port),
		path,
	)
	if err := add.Run(); err == nil {
		return nil
	}
	set := exec.Command("/usr/libexec/PlistBuddy",
		"-c", fmt.Sprintf("Set %s %d", key, port),
		path,
	)
	if out, err := set.CombinedOutput(); err != nil {
		return fmt.Errorf("PlistBuddy set: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resetSimulator shuts down then boots the given simulator. Used between
// retry attempts when xcodebuild stalls — a sim-daemon stuck in a bad
// state survives a plain xcodebuild kill, but a shutdown+boot cycle
// resets it without losing installed apps (those live in the data
// container, not the sim runtime state).
//
// Best-effort: returns an error if either step fails but doesn't fight
// the caller — the next startup attempt will reveal whether the sim is
// usable again.
func resetSimulator(ctx context.Context, udid string, logOut io.Writer) error {
	if logOut != nil {
		fmt.Fprintf(logOut, "  ⟳ Resetting simulator %s...\n", udid)
	}
	shutdownCmd := exec.CommandContext(ctx, "xcrun", "simctl", "shutdown", udid)
	if out, err := shutdownCmd.CombinedOutput(); err != nil {
		msg := strings.ToLower(strings.TrimSpace(string(out)))
		// "current state shutdown" / "unable to lookup in current state:
		// shutdown" is fine — already off, nothing to do.
		if !strings.Contains(msg, "shutdown") && !strings.Contains(msg, "current state:") {
			return fmt.Errorf("simctl shutdown: %w (%s)", err, msg)
		}
	}
	bootCmd := exec.CommandContext(ctx, "xcrun", "simctl", "boot", udid)
	if out, err := bootCmd.CombinedOutput(); err != nil {
		msg := strings.ToLower(strings.TrimSpace(string(out)))
		// "Booted" / "current state: booted" is fine — already on.
		if !strings.Contains(msg, "booted") && !strings.Contains(msg, "current state:") {
			return fmt.Errorf("simctl boot: %w (%s)", err, msg)
		}
	}
	// Wait for the sim to be fully ready. `bootstatus -b` blocks until
	// boot completes. Cap at 60s — a healthy sim boots in 5-15s.
	bootCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	_ = exec.CommandContext(bootCtx, "xcrun", "simctl", "bootstatus", udid, "-b").Run()
	return nil
}

// simctlInstall calls `xcrun simctl install <udid> <appPath>`. Reinstalls
// if the app is already present; simctl handles that gracefully.
func simctlInstall(ctx context.Context, udid, appPath string) error {
	if _, err := os.Stat(appPath); err != nil {
		return fmt.Errorf("app not found: %s", appPath)
	}
	cmd := exec.CommandContext(ctx, "xcrun", "simctl", "install", udid, appPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("simctl install: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// awaitReady polls the runner with `uptime` until it answers or the
// deadline passes. Backoff is short (200ms) since the runner usually comes
// up within 10-15s of cold start. agent-device's transport doesn't expose
// a separate /health endpoint, so we use the lightest real command.
//
// In parallel it watches the runner.log file for stall detection. xcodebuild
// emits steady output during a healthy startup (test discovery, framework
// init, our XCTest output). When it hangs the log goes silent. If we see
// no log growth for stallDetectWindow, return errStalled — the caller (Setup)
// kills xcodebuild and retries. Passing logPath="" disables stall detection
// (used when callers route output to their own writers).
func awaitReady(parent context.Context, c *Client, timeout time.Duration, logPath string) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	deadline := time.Now().Add(timeout)
	stallCheckEnabled := logPath != ""
	var lastLogSize int64 = -1
	lastLogActivity := time.Now()

	for {
		probeCtx, probeCancel := context.WithTimeout(ctx, 2*time.Second)
		err := c.Ping(probeCtx)
		probeCancel()
		if err == nil {
			return nil
		}

		// Stall detection: if the log file isn't growing, xcodebuild is
		// hung (most commonly waiting on something Xcode 26 + iOS 26 sim
		// doesn't resolve). Bail early so the caller can kill + retry
		// instead of waiting the full ReadyTimeout.
		if stallCheckEnabled {
			if fi, statErr := os.Stat(logPath); statErr == nil {
				if fi.Size() != lastLogSize {
					lastLogSize = fi.Size()
					lastLogActivity = time.Now()
				} else if time.Since(lastLogActivity) > stallDetectWindow {
					return fmt.Errorf(
						"xcodebuild stalled (no log output for %v; log: %s)",
						stallDetectWindow.Round(time.Second), logPath,
					)
				}
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %s: last error: %v", timeout, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// pickEphemeralPort asks the OS for a free port, closes the socket, and
// returns the port number. There is a small race where the OS could
// reassign the same port before xcodebuild claims it, but in practice this
// is fine because the runner is the only thing competing for ephemeral
// ports in this process and xcodebuild claims it within seconds.
func pickEphemeralPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// GracefulShutdown sends a `shutdown` command to the runner, then waits for
// the subprocess to exit. Falls back to SIGTERM after 5s.
func GracefulShutdown(ctx context.Context, c *Client, h *RunnerHandle) error {
	if c != nil {
		_, _ = c.Call(ctx, Command{Command: CmdShutdown})
	}
	return h.Stop()
}
