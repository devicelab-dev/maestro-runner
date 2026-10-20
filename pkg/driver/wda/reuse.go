package wda

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/config"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// --reuse-wda: a run started with it leaves its WebDriverAgent running when it
// ends, and records which one in a state file per device. The next run with
// the flag on that device drives the recorded WDA, with a session of its own,
// when it is still the xcodebuild this runner started, from the same build,
// on the same port, and /status says it is ready. Otherwise the recorded WDA
// is stopped and a new one started. A lock file per device, held for the
// whole run, keeps two runs from driving one WDA at once.

// keptDir holds the state and lock files of kept WDAs; tests replace it.
var keptDir = func() string {
	return filepath.Join(config.GetCacheDir(), "wda-reuse")
}

// keptWDA is what a run that kept its WDA records for the next one.
type keptWDA struct {
	Port uint16 `json:"port"`
	// Pid is the xcodebuild WDA runs under, leader of its own process group.
	Pid int `json:"pid"`
	// BuildDir and BuildHash name the build it was started from: the cache
	// directory, and a hash of the test bundle's executable in it.
	BuildDir  string `json:"buildDir"`
	BuildHash string `json:"buildHash"`
}

func keptStatePath(udid string) string { return filepath.Join(keptDir(), udid+".json") }
func keptLockPath(udid string) string  { return filepath.Join(keptDir(), udid+".lock") }

func loadKept(udid string) (keptWDA, bool) {
	var k keptWDA
	raw, err := os.ReadFile(keptStatePath(udid))
	if err != nil || json.Unmarshal(raw, &k) != nil || k.Pid <= 0 || k.Port == 0 {
		return keptWDA{}, false
	}
	return k, true
}

func saveKept(udid string, k keptWDA) error {
	if err := os.MkdirAll(keptDir(), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(k)
	if err != nil {
		return err
	}
	return os.WriteFile(keptStatePath(udid), raw, 0o644)
}

func removeKept(udid string) {
	if err := os.Remove(keptStatePath(udid)); err != nil && !os.IsNotExist(err) {
		logger.Debug("[wda] remove kept WDA state: %v", err)
	}
}

// lockDevice takes the device's run lock without waiting. The returned
// function lets go of it; so does the process ending.
func lockDevice(udid string) (func(), error) {
	if err := os.MkdirAll(keptDir(), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(keptLockPath(udid), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := tryLockFile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("another maestro-runner run with --reuse-wda is using device %s", udid)
	}
	return func() {
		unlockFile(f)
		_ = f.Close()
	}, nil
}

// KeptIdle reports whether an earlier --reuse-wda run left a WDA for udid
// that no run is using now. Its port is bound, but a run may take it over:
// with --reuse-wda by reusing it, without by stopping it (StopKept).
func KeptIdle(udid string) bool {
	if _, ok := loadKept(udid); !ok {
		return false
	}
	unlock, err := lockDevice(udid)
	if err != nil {
		return false
	}
	unlock()
	return true
}

// StopKept stops the WDA an earlier --reuse-wda run left for udid, when no
// run is using it: a run without the flag starts and stops its own WDA, as
// before the flag existed.
func StopKept(udid string) {
	unlock, err := lockDevice(udid)
	if err != nil {
		return
	}
	defer unlock()
	if k, ok := loadKept(udid); ok {
		logger.Info("[wda] stopping the WebDriverAgent an earlier --reuse-wda run left on port %d (pid %d)", k.Port, k.Pid)
		discardKept(udid, k)
	}
}

// discardKept stops a recorded WDA, if its pid is still the xcodebuild that
// started it, forgets it, and waits briefly for its port to come free.
func discardKept(udid string, k keptWDA) {
	if isOurXcodebuild(processCommand(k.Pid), udid) {
		killPidGroup(k.Pid)
	}
	removeKept(udid)
	waitPortFree(k.Port, 5*time.Second)
}

// KeepRunning makes this run leave the WDA it starts or reuses running for
// the next run (--reuse-wda). It takes the device's run lock, which Release
// and Cleanup let go of, and fails when another run holds it.
func (r *Runner) KeepRunning() error {
	unlock, err := lockDevice(r.deviceUDID)
	if err != nil {
		return err
	}
	r.keep = true
	r.unlock = unlock
	return nil
}

// Reattach drives the WDA an earlier --reuse-wda run left for this device,
// when it can be reused; Build must have run. On an error the recorded WDA,
// if any, has been stopped, and the caller starts a new one with Start.
func (r *Runner) Reattach(ctx context.Context) error {
	k, ok := loadKept(r.deviceUDID)
	if !ok {
		return errors.New("no WebDriverAgent left running by an earlier --reuse-wda run")
	}
	hash, err := r.buildHash()
	if err != nil {
		discardKept(r.deviceUDID, k)
		return err
	}
	cmdline := processCommand(k.Pid)
	if why := reuseRefusal(k, r.port, r.buildDir, hash, cmdline, r.deviceUDID); why != "" {
		discardKept(r.deviceUDID, k)
		return errors.New(why)
	}
	r.isSimulatorCache, _ = r.isSimulator()
	if !r.isSimulatorCache {
		// The forward to a device lived in the run that started WDA.
		if err := r.startPortForward(); err != nil {
			discardKept(r.deviceUDID, k)
			return fmt.Errorf("port forward to the kept WebDriverAgent: %w", err)
		}
	}
	if err := probeReady(ctx, r.port); err != nil {
		r.closePortForward()
		discardKept(r.deviceUDID, k)
		return err
	}
	logger.Info("[wda] reusing the WebDriverAgent on port %d (xcodebuild pid %d)", r.port, k.Pid)
	return nil
}

// Release ends this run's hold on WDA and leaves it running for the next
// --reuse-wda run: xcodebuild runs in its own process group, so it outlives
// this process. The caller ends its own session first.
func (r *Runner) Release() {
	r.closePortForward()
	if r.cmd != nil && r.cmd.Process != nil && r.isSimulatorCache {
		watchSimulatorXcodebuild(r.deviceUDID, r.cmd.Process.Pid)
	}
	r.cmd = nil
	if r.logFile != nil {
		_ = r.logFile.Close() // xcodebuild keeps its own descriptor
		r.logFile = nil
	}
	r.releaseLock()
}

func (r *Runner) releaseLock() {
	if r.unlock != nil {
		r.unlock()
		r.unlock = nil
	}
}

// recordKept writes the state a later --reuse-wda run reattaches by, for the
// WDA this run just started.
func (r *Runner) recordKept() {
	if r.cmd == nil || r.cmd.Process == nil {
		return
	}
	hash, err := r.buildHash()
	if err != nil {
		logger.Warn("[wda] --reuse-wda: cannot identify the WDA build, so the next run starts its own: %v", err)
		return
	}
	k := keptWDA{Port: r.port, Pid: r.cmd.Process.Pid, BuildDir: r.buildDir, BuildHash: hash}
	if err := saveKept(r.deviceUDID, k); err != nil {
		logger.Warn("[wda] --reuse-wda: cannot record the WDA for the next run: %v", err)
	}
}

// reuseRefusal says why a recorded WDA cannot be reused by a run that would
// start WDA from buildDir (whose test bundle hashes to buildHash) on port, or
// "" when it can. cmdline is the recorded pid's command line now.
func reuseRefusal(k keptWDA, port uint16, buildDir, buildHash, cmdline, udid string) string {
	switch {
	case k.Port != port:
		return fmt.Sprintf("the kept WebDriverAgent listens on port %d, this device's is %d", k.Port, port)
	case !isOurXcodebuild(cmdline, udid):
		return fmt.Sprintf("the kept WebDriverAgent's xcodebuild (pid %d) is no longer running", k.Pid)
	case k.BuildDir != buildDir:
		return fmt.Sprintf("the kept WebDriverAgent was built in %s, this run uses %s", k.BuildDir, buildDir)
	case k.BuildHash != buildHash:
		return "the kept WebDriverAgent is a different build from the cached one"
	}
	return ""
}

// isOurXcodebuild reports whether cmdline is an xcodebuild running WDA's
// tests for the device udid, as Start launches it.
func isOurXcodebuild(cmdline, udid string) bool {
	return udid != "" &&
		strings.Contains(cmdline, "xcodebuild") &&
		strings.Contains(cmdline, "test-without-building") &&
		strings.Contains(cmdline, "id="+udid)
}

// processCommand is pid's command line, "" when there is no such process.
func processCommand(pid int) string {
	if pid <= 0 {
		return ""
	}
	out, err := execCommand("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// testBundleExecutables is where a build's WDA test bundle executable is, by
// platform directory (Debug-iphonesimulator, Debug-iphoneos).
const testBundleExecutables = "Build/Products/*/WebDriverAgentRunner-Runner.app/PlugIns/WebDriverAgentRunner.xctest/WebDriverAgentRunner"

// buildHash is a SHA-256 of the cached build's WDA test bundle executable.
func (r *Runner) buildHash() (string, error) {
	matches, _ := filepath.Glob(filepath.Join(r.derivedDataPath(), testBundleExecutables))
	if len(matches) == 0 {
		return "", fmt.Errorf("no WDA test bundle in %s", r.derivedDataPath())
	}
	f, err := os.Open(matches[0])
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// probeReady asks WDA on port for /status once, within attachProbeTimeout.
func probeReady(ctx context.Context, port uint16) error {
	c := NewClient(port)
	c.httpClient = &http.Client{Timeout: attachProbeTimeout, Transport: c.httpClient.Transport}
	status, err := c.Status()
	if err != nil {
		return fmt.Errorf("the kept WebDriverAgent on port %d does not answer /status: %w", port, err)
	}
	if !statusReady(status) {
		return fmt.Errorf("the kept WebDriverAgent on port %d is not ready", port)
	}
	return ctx.Err()
}

// statusReady reports whether a /status response says WDA is ready.
func statusReady(status map[string]interface{}) bool {
	value, ok := status["value"].(map[string]interface{})
	if !ok {
		return false
	}
	ready, ok := value["ready"].(bool)
	return ok && ready
}

// waitPortFree waits up to d for nothing to listen on 127.0.0.1:port.
func waitPortFree(port uint16, d time.Duration) {
	deadline := time.Now().Add(d)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Now().Before(deadline) {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			_ = ln.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (r *Runner) closePortForward() {
	if r.portForwardListener != nil {
		if err := r.portForwardListener.Close(); err != nil {
			logger.Warn("failed to close port forward listener: %v", err)
		}
		r.portForwardListener = nil
	}
}
