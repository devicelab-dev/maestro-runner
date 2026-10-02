package devicelab_ios

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/config"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// Bundle ids of the prebuilt agent.
const (
	runnerBundleID = "dev.devicelab.agent.uitests.xctrunner"
	hostBundleID   = "dev.devicelab.agent"
	runnerAppName  = "DevicelabIOSAgentUITests-Runner.app"
	hostAppName    = "DevicelabIOSAgent.app"
	testTargetName = "DevicelabIOSAgentUITests"
)

// Launch modes.
const (
	ModeAuto       = "auto"       // simctl launch, xcodebuild if it does not come up
	ModeSimctl     = "simctl"     // simctl launch only
	ModeXcodebuild = "xcodebuild" // xcodebuild test-without-building only
)

// AgentOptions configures StartAgent.
type AgentOptions struct {
	UDID string
	// Dir holds the prebuilt agent (the apps, agent.xctestrun, manifest.json).
	// Empty means the bundled one (drivers/ios/devicelab-ios-agent/simulator).
	Dir string
	// Mode is ModeAuto (default), ModeSimctl or ModeXcodebuild; the
	// DEVICELAB_IOS_AGENT_LAUNCH environment variable overrides it.
	Mode string
	// ReadyTimeout bounds how long a launch may take to answer status.
	ReadyTimeout time.Duration
	// TeamID is the Apple development team the agent is signed with on a
	// real device (StartDeviceAgent); simulators do not use it.
	TeamID string
}

// Agent is a running agent on one simulator.
type Agent struct {
	opts    AgentOptions
	port    int
	mode    string
	version string

	// device is set for a real iPhone (StartDeviceAgent), with the bundle
	// ids the agent was signed as and the usbmux port forward.
	device bool
	ids    bundleIDs

	// keptPid is the xcodebuild of an agent an earlier run left running and
	// this one re-attached to, stopped if the agent has to be restarted.
	keptPid int

	mu       sync.Mutex
	xcodeCmd *exec.Cmd
	logFile  *os.File
	forward  io.Closer
}

// Port is the agent's port on 127.0.0.1.
func (a *Agent) Port() int { return a.port }

// Mode is how the agent was started.
func (a *Agent) Mode() string { return a.mode }

// BundledAgentDir is where maestro-runner ships the prebuilt agent.
func BundledAgentDir() string {
	return filepath.Join(config.GetDriversDir("ios"), "devicelab-ios-agent", "simulator")
}

// manifest is the prebuilt agent's manifest.json.
type manifest struct {
	Version  string `json:"version"`
	Protocol string `json:"protocol"`
}

func readManifest(dir string) (manifest, error) {
	var m manifest
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, fmt.Errorf("no prebuilt agent at %s: %w", dir, err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("bad agent manifest in %s: %w", dir, err)
	}
	if !strings.HasPrefix(m.Protocol, ProtocolMajor+".") && m.Protocol != ProtocolMajor {
		return m, fmt.Errorf("agent in %s speaks protocol %s; this maestro-runner speaks %s.x", dir, m.Protocol, ProtocolMajor)
	}
	return m, nil
}

// stateDir holds per-simulator agent state (installed version, port).
func stateDir(udid string) string {
	return filepath.Join(config.GetCacheDir(), "devicelab-ios-agent", udid)
}

type agentState struct {
	Port    int    `json:"port"`
	Version string `json:"version"`
	// Pid is the xcodebuild the agent runs under, when it was started that
	// way: a later run that finds the agent stuck stops it by this.
	Pid int `json:"pid,omitempty"`
}

func loadState(udid string) (agentState, bool) {
	var s agentState
	raw, err := os.ReadFile(filepath.Join(stateDir(udid), "agent.json"))
	if err != nil || json.Unmarshal(raw, &s) != nil || s.Port <= 0 {
		return s, false
	}
	return s, true
}

func saveState(udid string, s agentState) {
	dir := stateDir(udid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	raw, _ := json.Marshal(s)
	_ = os.WriteFile(filepath.Join(dir, "agent.json"), raw, 0o644)
}

// PortFor derives a stable port for a simulator from its UDID, in
// 22100–22899, clear of WDA's 8100–9099.
func PortFor(udid string) int {
	tail := udid
	if i := strings.LastIndex(udid, "-"); i >= 0 {
		tail = udid[i+1:]
	}
	if len(tail) > 12 {
		tail = tail[len(tail)-12:]
	}
	n, err := strconv.ParseUint(tail, 16, 64)
	if err != nil {
		return 22100
	}
	return 22100 + int(n%800)
}

// portFree reports whether nothing listens on 127.0.0.1:port or [::1]:port.
func portFree(port int) bool {
	for _, addr := range []string{fmt.Sprintf("127.0.0.1:%d", port), fmt.Sprintf("[::1]:%d", port)} {
		c, err := net.DialTimeout("tcp", addr, 150*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return false
		}
	}
	return true
}

// StartAgent returns a client for a live agent on the simulator: the one
// already running there when it answers with this build's version, else a
// freshly launched one.
func StartAgent(ctx context.Context, opts AgentOptions) (*Agent, *Client, error) {
	if opts.Dir == "" {
		if env := os.Getenv("DEVICELAB_IOS_AGENT_DIR"); env != "" {
			opts.Dir = env
		} else {
			opts.Dir = BundledAgentDir()
		}
	}
	if env := os.Getenv("DEVICELAB_IOS_AGENT_LAUNCH"); env != "" {
		opts.Mode = env
	}
	if opts.Mode == "" {
		opts.Mode = ModeAuto
	}
	if opts.ReadyTimeout <= 0 {
		opts.ReadyTimeout = 120 * time.Second
	}
	m, err := readManifest(opts.Dir)
	if err != nil {
		return nil, nil, err
	}
	a := &Agent{opts: opts, version: m.Version}

	// Re-attach to an agent a previous run left running, if it still does
	// real work: a stuck agent answers status but not a snapshot, and
	// re-attaching to one failed every later flow (RNTester iOS on CI).
	if s, ok := loadState(opts.UDID); ok && s.Version == m.Version {
		c := NewClient(s.Port)
		if a.alive(ctx, c, 2*time.Second) && a.working(ctx, c, reattachProbeWait) {
			a.port, a.mode, a.keptPid = s.Port, "reattached", s.Pid
			logger.Info("[devicelab-ios] re-attached to agent %s on port %d", m.Version, s.Port)
			c.SetReviver(a.revive)
			return a, c, nil
		}
		logger.Info("[devicelab-ios] agent on port %d left by an earlier run does not answer; starting a new one", s.Port)
		killPidGroup(s.Pid)
		_ = os.Remove(filepath.Join(stateDir(opts.UDID), "agent.json"))
	}

	if err := a.install(ctx, m.Version); err != nil {
		return nil, nil, err
	}
	c, err := a.launch(ctx)
	if err != nil {
		return nil, nil, err
	}
	c.SetReviver(a.revive)
	return a, c, nil
}

// alive reports whether the agent at c answers status within wait.
// reattachProbeWait bounds the snapshot a kept agent must answer before a
// run re-attaches to it.
var reattachProbeWait = 10 * time.Second

// working reports whether the agent answers a real request, a one-node
// snapshot, within wait.
func (a *Agent) working(ctx context.Context, c *Client, wait time.Duration) bool {
	callCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	resp, err := c.Call(callCtx, "snapshot", &Args{MaxNodes: 1})
	return err == nil && resp.OK
}

func (a *Agent) alive(ctx context.Context, c *Client, wait time.Duration) bool {
	callCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	resp, err := c.Call(callCtx, "status", nil)
	return err == nil && resp.OK
}

// install puts the agent apps on the simulator unless this version is there.
func (a *Agent) install(ctx context.Context, version string) error {
	marker := filepath.Join(stateDir(a.opts.UDID), "installed")
	if raw, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(raw)) == version {
		return nil
	}
	for _, app := range []string{hostAppName, runnerAppName} {
		if out, err := simctl(ctx, 3*time.Minute, "install", a.opts.UDID, filepath.Join(a.opts.Dir, app)); err != nil {
			return fmt.Errorf("install %s: %v: %s", app, err, strings.TrimSpace(out))
		}
	}
	_ = os.MkdirAll(filepath.Dir(marker), 0o755)
	_ = os.WriteFile(marker, []byte(version), 0o644)
	return nil
}

// launch starts the agent on a free port, by simctl launch and, failing that
// (auto mode), by xcodebuild test-without-building.
func (a *Agent) launch(ctx context.Context) (*Client, error) {
	if a.device {
		return a.launchDevice(ctx)
	}
	port := PortFor(a.opts.UDID)
	for i := 0; i < 50 && !portFree(port); i++ {
		port++
	}
	a.port = port
	c := NewClient(port)
	var errs []string
	// In auto mode a simulator where simctl launch once failed goes straight
	// to xcodebuild, instead of paying the simctl wait on every run.
	noSimctl := filepath.Join(stateDir(a.opts.UDID), "simctl-failed-"+a.version)
	skipSimctl := a.opts.Mode == ModeAuto && pathExists(noSimctl)
	if skipSimctl {
		// On the console too: in CI the log file is often not kept, and this
		// is why every run then pays a full agent start.
		reason, _ := os.ReadFile(noSimctl)
		fmt.Fprintf(os.Stderr, "  ⚠ simctl launch failed on this simulator before (%s); starting the agent with xcodebuild\n",
			strings.TrimSpace(oneLine(string(reason))))
	}
	if (a.opts.Mode == ModeAuto && !skipSimctl) || a.opts.Mode == ModeSimctl {
		if err := a.launchSimctl(ctx, c); err == nil {
			a.mode = ModeSimctl
			saveState(a.opts.UDID, agentState{Port: port, Version: a.version})
			return c, nil
		} else {
			errs = append(errs, "simctl launch: "+err.Error())
			logger.Info("[devicelab-ios] simctl launch did not bring the agent up (%v)", err)
			fmt.Fprintf(os.Stderr, "  ⚠ simctl launch did not bring the agent up (%s); trying xcodebuild\n", oneLine(err.Error()))
			if a.opts.Mode == ModeAuto {
				_ = os.MkdirAll(filepath.Dir(noSimctl), 0o755)
				_ = os.WriteFile(noSimctl, []byte(err.Error()), 0o644)
			}
		}
	}
	if a.opts.Mode == ModeAuto || a.opts.Mode == ModeXcodebuild {
		if err := a.launchXcodebuild(ctx, c); err == nil {
			a.mode = ModeXcodebuild
			pid := 0
			a.mu.Lock()
			if a.xcodeCmd != nil && a.xcodeCmd.Process != nil {
				pid = a.xcodeCmd.Process.Pid
			}
			a.mu.Unlock()
			saveState(a.opts.UDID, agentState{Port: port, Version: a.version, Pid: pid})
			return c, nil
		} else {
			errs = append(errs, "xcodebuild: "+err.Error())
		}
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("unknown agent launch mode %q", a.opts.Mode)
	}
	return nil, fmt.Errorf("agent did not start: %s", strings.Join(errs, "; "))
}

func (a *Agent) launchSimctl(ctx context.Context, c *Client) error {
	_, _ = simctl(ctx, 15*time.Second, "terminate", a.opts.UDID, runnerBundleID)
	cmd := exec.CommandContext(ctx, "xcrun", "simctl", "launch", a.opts.UDID, runnerBundleID)
	cmd.Env = append(os.Environ(), fmt.Sprintf("SIMCTL_CHILD_DL_AGENT_PORT=%d", a.port))
	if os.Getenv("SIMCTL_CHILD_"+snapshotDepthVar) == "" {
		cmd.Env = append(cmd.Env, "SIMCTL_CHILD_"+snapshotDepthVar+"="+agentSnapshotMaxDepth())
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	wait := a.opts.ReadyTimeout
	if a.opts.Mode == ModeAuto && wait > 30*time.Second {
		wait = 30 * time.Second // the fallback still has its own budget
	}
	return a.waitReady(ctx, c, wait, nil)
}

func (a *Agent) launchXcodebuild(ctx context.Context, c *Client) error {
	xctestrun, err := a.xctestrunFor(a.port)
	if err != nil {
		return err
	}
	logDir := stateDir(a.opts.UDID)
	_ = os.MkdirAll(logDir, 0o755)
	logFile, err := os.Create(filepath.Join(logDir, "xcodebuild.log"))
	if err != nil {
		return err
	}
	cmd := exec.Command("xcodebuild", "test-without-building",
		"-xctestrun", xctestrun,
		"-destination", "id="+a.opts.UDID,
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
	if err := a.waitReady(ctx, c, a.opts.ReadyTimeout, exited); err != nil {
		a.stopXcodebuild()
		return fmt.Errorf("%v (log: %s)", err, logFile.Name())
	}
	return nil
}

// xctestrunFor writes a copy of agent.xctestrun next to the apps with the
// port set, one per port so parallel simulators never share a file.
func (a *Agent) xctestrunFor(port int) (string, error) {
	src := filepath.Join(a.opts.Dir, "agent.xctestrun")
	dst := filepath.Join(a.opts.Dir, fmt.Sprintf("agent-%d.xctestrun", port))
	raw, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		return "", err
	}
	for _, key := range []string{"EnvironmentVariables", "TestingEnvironmentVariables"} {
		path := fmt.Sprintf("%s.%s.DL_AGENT_PORT", testTargetName, key)
		if out, err := exec.Command("plutil", "-replace", path, "-string", strconv.Itoa(port), dst).CombinedOutput(); err != nil {
			return "", fmt.Errorf("set port in %s: %v: %s", dst, err, strings.TrimSpace(string(out)))
		}
		path = fmt.Sprintf("%s.%s.%s", testTargetName, key, snapshotDepthVar)
		if out, err := exec.Command("plutil", "-replace", path, "-string", agentSnapshotMaxDepth(), dst).CombinedOutput(); err != nil {
			return "", fmt.Errorf("set snapshot depth in %s: %v: %s", dst, err, strings.TrimSpace(string(out)))
		}
	}
	return dst, nil
}

func (a *Agent) waitReady(ctx context.Context, c *Client, wait time.Duration, exited <-chan error) error {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if a.alive(ctx, c, 2*time.Second) {
			return nil
		}
		select {
		case err := <-exited:
			return fmt.Errorf("runner exited before it answered: %v", err)
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("agent did not answer on port %d within %s", a.port, wait)
}

// revive restarts a dead agent (the Client's Reviver).
func (a *Agent) revive(ctx context.Context) (int, error) {
	logger.Info("[devicelab-ios] agent on port %d stopped answering; restarting it", a.port)
	a.stopXcodebuild()
	if a.keptPid > 0 {
		killPidGroup(a.keptPid) // the xcodebuild an earlier run left
		a.keptPid = 0
	}
	if _, err := a.launch(ctx); err != nil {
		return 0, err
	}
	return a.port, nil
}

// snapshotDepthVar is the agent's accessibility-snapshot depth setting.
const snapshotDepthVar = "DL_AGENT_SNAPSHOT_MAX_DEPTH"

// agentSnapshotMaxDepth is the snapshot depth the agent is started with. With
// a cap above 62, XCTest returns no elements at all for a screen nested deeper
// than 62 levels, so a deep screen came back blank at the agent's own default
// of 100 (#171, measured on iOS 27). 62 is the deepest cap that still returns
// it, and is the same as 100 for any screen 100 worked on. Set
// DL_AGENT_SNAPSHOT_MAX_DEPTH (or its SIMCTL_CHILD_ form) to override.
func agentSnapshotMaxDepth() string {
	for _, name := range []string{snapshotDepthVar, "SIMCTL_CHILD_" + snapshotDepthVar} {
		if v := os.Getenv(name); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return strconv.Itoa(n)
			}
		}
	}
	return "62"
}

// Release ends the run's hold on the agent, which stays up for the next run
// to re-attach to, however it was started: xcodebuild runs in its own
// process group, so it outlives this process as a simctl-launched agent
// does. Stopping it here made a harness that runs one process per flow
// (React Native's iOS E2E) restart the agent for every flow when simctl
// launch did not work on the machine. DEVICELAB_IOS_AGENT_KEEP=0 stops it.
func (a *Agent) Release(ctx context.Context, c *Client) {
	if os.Getenv("DEVICELAB_IOS_AGENT_KEEP") == "0" {
		a.Stop(ctx, c)
		return
	}
	a.mu.Lock()
	cmd, logFile := a.xcodeCmd, a.logFile
	a.xcodeCmd, a.logFile = nil, nil
	a.mu.Unlock()
	if logFile != nil {
		_ = logFile.Close() // xcodebuild keeps its own descriptor
	}
	if cmd != nil && cmd.Process != nil {
		watchKeptAgent(a.opts.UDID, cmd.Process.Pid)
	}
}

// watchKeptAgent is watchXcodebuild; tests replace it.
var watchKeptAgent = watchXcodebuild

// watchXcodebuild starts a detached watcher that stops the xcodebuild
// process group pid leads once the simulator is no longer booted: xcodebuild
// does not exit when its simulator shuts down, and a kept agent must not
// outlive it. The watcher ends by itself when xcodebuild does.
func watchXcodebuild(udid string, pid int) {
	script := fmt.Sprintf(`while kill -0 %[1]d 2>/dev/null && xcrun simctl list devices booted | grep -q %[2]q; do sleep 20; done
kill -TERM -%[1]d 2>/dev/null; sleep 5; kill -KILL -%[1]d 2>/dev/null; exit 0`, pid, udid)
	w := exec.Command("/bin/sh", "-c", script)
	setProcessGroup(w)
	if err := w.Start(); err != nil {
		logger.Warn("[devicelab-ios] could not watch the kept agent (pid %d): %v", pid, err)
		return
	}
	go func() { _ = w.Wait() }()
}

// Stop asks the agent to exit and ends an xcodebuild it was started by.
func (a *Agent) Stop(ctx context.Context, c *Client) {
	if c != nil {
		callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_, _ = c.Call(callCtx, "shutdown", nil)
		cancel()
	}
	a.stopXcodebuild()
	a.closeForward()
	_ = os.Remove(filepath.Join(stateDir(a.opts.UDID), "agent.json"))
}

func (a *Agent) stopXcodebuild() {
	a.mu.Lock()
	cmd, logFile := a.xcodeCmd, a.logFile
	a.xcodeCmd, a.logFile = nil, nil
	a.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		killProcessGroup(cmd)
	}
	if logFile != nil {
		_ = logFile.Close()
	}
}

// simctl runs `xcrun simctl args…` bounded by timeout.
func simctl(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(c, "xcrun", append([]string{"simctl"}, args...)...).CombinedOutput()
	return string(out), err
}

// oneLine is s up to its first line break, for a one-line console note.
func oneLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
