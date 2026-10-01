package devicelab_ios

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/forward"
	"github.com/danielpaulus/go-ios/ios/instruments"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"howett.net/plist"
)

// ModeDevice is how an agent on a real iPhone is started: xcodebuild
// test-without-building with the re-signed agent, the port forwarded over
// usbmux.
const ModeDevice = "device"

const (
	// deviceStartAttempts caps the start retries. xcodebuild on a device
	// sometimes hangs before the runner starts (the WDA path sees the same);
	// a kill and retry usually clears it.
	deviceStartAttempts = 3
	// deviceStallWindow is how long xcodebuild may print nothing before the
	// attempt counts as hung. Longer than the WDA path's 60s: installing the
	// two apps on a device is silent and can take that long on its own.
	deviceStallWindow = 120 * time.Second
)

// StartDeviceAgent returns a client for the agent on a real iPhone. The
// prebuilt device agent is re-signed for opts.TeamID once per agent version,
// team and device (see device_signing.go); each run then starts it with
// xcodebuild and forwards its port to 127.0.0.1. An agent xcodebuild runs
// cannot outlive this process, so there is no re-attaching.
func StartDeviceAgent(ctx context.Context, opts AgentOptions) (*Agent, *Client, error) {
	if err := RequireTeamID(opts.TeamID); err != nil {
		return nil, nil, err
	}
	opts.TeamID = strings.ToUpper(strings.TrimSpace(opts.TeamID))
	if opts.Dir == "" {
		if env := os.Getenv(DeviceDirEnv); env != "" {
			opts.Dir = env
		} else {
			opts.Dir = BundledDeviceAgentDir()
		}
	}
	if opts.ReadyTimeout <= 0 {
		opts.ReadyTimeout = 120 * time.Second
	}
	m, err := readManifest(opts.Dir)
	if err != nil {
		return nil, nil, err
	}
	ids := agentBundleIDs(os.Getenv(BundleIDEnv))
	signer := &deviceSigner{
		udid: opts.UDID, team: opts.TeamID, version: m.Version, ids: ids,
		agentDir: opts.Dir, stubDir: bundledSigningStubDir(),
		logf: func(format string, args ...any) { logger.Info("[devicelab-ios] "+format, args...) },
	}
	signed, err := signer.signedDir(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("sign the agent for device %s: %w", opts.UDID, err)
	}
	opts.Dir = signed
	a := &Agent{opts: opts, version: m.Version, device: true, ids: ids}
	c, err := a.launch(ctx)
	if err != nil {
		return nil, nil, err
	}
	c.SetReviver(a.revive)
	return a, c, nil
}

// launchDevice starts the agent on the device, retrying a start that hangs
// or fails in a way a retry can fix. The port and its forward are chosen once
// and kept across restarts.
func (a *Agent) launchDevice(ctx context.Context) (*Client, error) {
	if a.port == 0 {
		port := PortFor(a.opts.UDID)
		for i := 0; i < 50 && !portFree(port); i++ {
			port++
		}
		a.port = port
	}
	if err := a.startForward(); err != nil {
		return nil, err
	}
	c := NewClient(a.port)
	var lastErr error
	for attempt := 1; attempt <= deviceStartAttempts; attempt++ {
		if attempt > 1 {
			logger.Info("[devicelab-ios] agent start %d/%d failed (%v); retrying", attempt-1, deviceStartAttempts, lastErr)
			// Killing xcodebuild does not end the runner on the phone; a
			// runner left over would race the next session for the device.
			terminateDeviceRunner(a.opts.UDID)
		}
		err := a.startDeviceOnce(ctx, c, attempt)
		if err == nil {
			a.mode = ModeDevice
			return c, nil
		}
		lastErr = err
		var perm *permanentStartError
		if errors.As(err, &perm) || ctx.Err() != nil {
			break
		}
	}
	a.closeForward()
	return nil, fmt.Errorf("agent did not start on device %s: %w", a.opts.UDID, lastErr)
}

// startDeviceOnce runs one xcodebuild and waits for the agent to answer.
func (a *Agent) startDeviceOnce(ctx context.Context, c *Client, attempt int) error {
	xctestrun, err := writeDeviceXctestrun(a.opts.Dir, a.port, a.ids)
	if err != nil {
		return &permanentStartError{err}
	}
	logDir := stateDir(a.opts.UDID)
	_ = os.MkdirAll(logDir, 0o755)
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if attempt == 1 {
		flags |= os.O_TRUNC
	}
	logPath := filepath.Join(logDir, "xcodebuild.log")
	logFile, err := os.OpenFile(logPath, flags, 0o644)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logFile, "=== attempt %d/%d, port %d ===\n", attempt, deviceStartAttempts, a.port)
	cmd := exec.Command("xcodebuild", "test-without-building",
		"-xctestrun", xctestrun,
		"-destination", "platform=iOS,id="+a.opts.UDID,
		"-collect-test-diagnostics", "never")
	cmd.Stdout, cmd.Stderr = logFile, logFile
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	a.mu.Lock()
	a.xcodeCmd, a.logFile = cmd, logFile
	a.mu.Unlock()
	if err := a.waitDeviceReady(ctx, c, logPath, exited); err != nil {
		a.stopXcodebuild()
		return err
	}
	return nil
}

// waitDeviceReady polls status until the agent answers, xcodebuild exits,
// its log names a failure, or it goes quiet for deviceStallWindow.
func (a *Agent) waitDeviceReady(ctx context.Context, c *Client, logPath string, exited <-chan error) error {
	deadline := time.Now().Add(a.opts.ReadyTimeout)
	lastSize, lastActivity := int64(-1), time.Now()
	for time.Now().Before(deadline) {
		if a.alive(ctx, c, 2*time.Second) {
			return nil
		}
		raw, _ := os.ReadFile(logPath)
		if err := checkDeviceLog(string(raw)); err != nil {
			return withLog(err, logPath)
		}
		if size := int64(len(raw)); size != lastSize {
			lastSize, lastActivity = size, time.Now()
		} else if time.Since(lastActivity) > deviceStallWindow {
			return fmt.Errorf("xcodebuild printed nothing for %s (log: %s)", deviceStallWindow, logPath)
		}
		select {
		case err := <-exited:
			raw, _ := os.ReadFile(logPath)
			if cerr := checkDeviceLog(string(raw)); cerr != nil {
				return withLog(cerr, logPath)
			}
			return fmt.Errorf("xcodebuild exited (%v) before the agent answered:\n%s\n(log: %s)", err, lastLines(string(raw), 15), logPath)
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("agent did not answer on port %d within %s (log: %s)", a.port, a.opts.ReadyTimeout, logPath)
}

// permanentStartError is a start failure a retry cannot fix.
type permanentStartError struct{ err error }

func (e *permanentStartError) Error() string { return e.err.Error() }
func (e *permanentStartError) Unwrap() error { return e.err }

func withLog(err error, logPath string) error {
	wrapped := fmt.Errorf("%w (log: %s)", err, logPath)
	var perm *permanentStartError
	if errors.As(err, &perm) {
		return &permanentStartError{wrapped}
	}
	return wrapped
}

// checkDeviceLog reads xcodebuild's log for a failure; nil means keep
// waiting. The ones only the user can fix on the phone are permanent.
func checkDeviceLog(log string) error {
	l := strings.ToLower(log)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(l, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("developer app certificate is not trusted", "has not been explicitly trusted by the user", "untrusted developer"):
		return &permanentStartError{errors.New("the iPhone does not trust your developer certificate yet: on the device open " +
			"Settings > General > VPN & Device Management, tap your Apple Development certificate, tap Trust, then run again")}
	case has("developer mode disabled", "developer mode is disabled", "developer mode is not enabled", "enable developer mode"):
		return &permanentStartError{errors.New("the iPhone has Developer Mode off: turn it on in Settings > Privacy & Security > Developer Mode (the phone restarts), then run again")}
	case has("device is locked", "is passcode protected", "unlock the device", "unlock your device"):
		return &permanentStartError{errors.New("the iPhone is locked: unlock it and keep it unlocked while the agent starts")}
	case has("a valid provisioning profile for this executable was not found", "no valid provisioning profile", "0xe8008015", "0xe800801c", "invalid code signature", "0xe8008001"):
		return &permanentStartError{errors.New("the iPhone rejected the agent's signature or profile: delete " +
			"~/.maestro-runner/cache/devicelab-ios-agent/device-signed to re-sign it, and check --team-id")}
	}
	if line := firstLine(log, "xcodebuild: error:"); line != "" {
		return &permanentStartError{errors.New(line)}
	}
	if strings.Contains(log, "Testing failed:") {
		return fmt.Errorf("the agent's test session failed:\n%s", lastLines(log, 15))
	}
	return nil
}

func firstLine(log, substr string) string {
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, substr) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// writeDeviceXctestrun writes agent-<port>.xctestrun next to the signed
// products (__TESTROOT__ is the file's own directory).
func writeDeviceXctestrun(dir string, port int, ids bundleIDs) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "agent.xctestrun"))
	if err != nil {
		return "", fmt.Errorf("read the agent's xctestrun: %w", err)
	}
	out, err := renderDeviceXctestrun(raw, port, ids)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, fmt.Sprintf("agent-%d.xctestrun", port))
	if err := os.WriteFile(dst, out, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

// renderDeviceXctestrun sets DL_AGENT_PORT in every test target's
// environment (the way the WDA path sets USE_PORT) and, for custom bundle
// ids, the runner's bundle id.
func renderDeviceXctestrun(raw []byte, port int, ids bundleIDs) ([]byte, error) {
	var doc map[string]interface{}
	if _, err := plist.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("bad xctestrun: %w", err)
	}
	targets := 0
	for key, v := range doc {
		target, ok := v.(map[string]interface{})
		if !ok || key == "__xctestrun_metadata__" {
			continue
		}
		targets++
		for _, envKey := range []string{"EnvironmentVariables", "TestingEnvironmentVariables"} {
			env, ok := target[envKey].(map[string]interface{})
			if !ok {
				env = map[string]interface{}{}
				target[envKey] = env
			}
			env["DL_AGENT_PORT"] = strconv.Itoa(port)
			env[snapshotDepthVar] = agentSnapshotMaxDepth()
		}
		if ids.custom() {
			target["TestHostBundleIdentifier"] = ids.runner
			target["BundleIdentifiersForCrashReportEmphasis"] = []interface{}{ids.host, ids.test}
		}
	}
	if targets == 0 {
		return nil, fmt.Errorf("xctestrun has no test target")
	}
	return plist.MarshalIndent(doc, plist.XMLFormat, "\t")
}

// startForward forwards 127.0.0.1:port to the same port on the device over
// usbmux (go-ios), as the WDA path does.
func (a *Agent) startForward() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.forward != nil {
		return nil
	}
	entry, err := goios.GetDevice(a.opts.UDID)
	if err != nil {
		return fmt.Errorf("device %s not found over usbmux: %w", a.opts.UDID, err)
	}
	l, err := forward.Forward(entry, uint16(a.port), uint16(a.port))
	if err != nil {
		return fmt.Errorf("forward port %d to device %s: %w", a.port, a.opts.UDID, err)
	}
	a.forward = l
	return nil
}

func (a *Agent) closeForward() {
	a.mu.Lock()
	l := a.forward
	a.forward = nil
	a.mu.Unlock()
	if l != nil {
		if err := l.Close(); err != nil {
			logger.Debug("[devicelab-ios] close port forward: %v", err)
		}
	}
}

// terminateDeviceRunner kills an agent runner left running on the device.
// Best effort: this runs on the recovery path, so every failure is logged
// and swallowed.
func terminateDeviceRunner(udid string) {
	device, err := goios.GetDevice(udid)
	if err != nil {
		logger.Debug("[devicelab-ios] runner cleanup: no device %s: %v", udid, err)
		return
	}
	info, err := instruments.NewDeviceInfoService(device)
	if err != nil {
		logger.Debug("[devicelab-ios] runner cleanup: device info service: %v", err)
		return
	}
	defer info.Close()
	procs, err := info.ProcessList()
	if err != nil {
		logger.Debug("[devicelab-ios] runner cleanup: process list: %v", err)
		return
	}
	pc, err := instruments.NewProcessControl(device)
	if err != nil {
		logger.Debug("[devicelab-ios] runner cleanup: process control: %v", err)
		return
	}
	defer func() { _ = pc.Close() }()
	runnerName := strings.TrimSuffix(runnerAppName, ".app")
	for _, p := range procs {
		if !strings.Contains(p.Name, runnerName) {
			continue
		}
		if err := pc.KillProcess(p.Pid); err != nil {
			logger.Debug("[devicelab-ios] runner cleanup: kill %s (pid %d): %v", p.Name, p.Pid, err)
			continue
		}
		logger.Info("[devicelab-ios] terminated leftover %s (pid %d) on the device", p.Name, p.Pid)
	}
}
