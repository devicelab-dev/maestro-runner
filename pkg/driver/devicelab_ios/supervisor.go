package devicelab_ios

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// maxRelaunchesPerWindow bounds how many times the supervisor relaunches a
// dead runner before giving up, counting relaunches until the runner has
// stayed up for relaunchWindow. The bound exists for the
// deterministic-crash case: a command that kills the runner every time (the
// keyboard-query XCTest abort we hit on TestHive) would otherwise relaunch
// forever. read-only commands are replayed after a relaunch and could re-crash;
// actions are not replayed, so in practice one crasher costs at most this many
// relaunches, then later (different) commands proceed on the live runner.
const maxRelaunchesPerWindow = 5

// relaunchWindow resets the relaunch counter once the runner has been healthy
// for this long — measured from the last relaunch, not from a fixed window
// start — so an occasional crash over a long suite does not exhaust the
// budget that a burst of deterministic crashes is meant to cap, while a crash
// loop spread just over a fixed window boundary cannot dodge it.
const relaunchWindow = 2 * time.Minute

// Supervisor keeps a devicelab runner alive across mid-session crashes. It
// owns the current process handle and hands the Client a reviver that
// relaunches on a transport failure. One supervisor per Setup; commands to a
// devicelab runner are serial, so its locking only guards the rare concurrent
// failure, not a hot path.
type Supervisor struct {
	opts      SetupOptions
	xctestrun string
	logPath   string
	// start launches one runner process; startOnce in production, a fake
	// in tests.
	start func(ctx context.Context, opts SetupOptions, xctestrun, logPath string) (*Client, *RunnerHandle, error)

	mu           sync.Mutex
	handle       atomic.Pointer[RunnerHandle]
	stopping     atomic.Bool
	relaunches   int
	lastRelaunch time.Time
	// warn receives relaunch diagnostics; nil means os.Stderr.
	warn io.Writer
}

// newSupervisor builds the supervisor for a freshly started runner, points
// the handle's Stop() at it, and installs the reviver on the client. It is
// wired inside Setup; nothing else needs to call it.
func newSupervisor(opts SetupOptions, xctestrun, logPath string, client *Client, handle *RunnerHandle) *Supervisor {
	s := &Supervisor{opts: opts, xctestrun: xctestrun, logPath: logPath, start: startOnce}
	s.handle.Store(handle)
	handle.sup = s
	client.SetReviver(s.revive)
	return s
}

// revive relaunches the runner after a transport failure and returns the port
// the new process listens on. failedPort is the port the failing call used:
// if the live handle is already on a different port, another failed call has
// relaunched and this one simply re-points, so we do not relaunch twice.
//
// The relaunch runs under its own RelaunchTimeout derived from ctx: callers
// may pass a deadline-less context (context.WithoutCancel), and without a
// bound a wedged xcodebuild would hold s.mu — and every queued caller — for
// the full ReadyTimeout.
func (s *Supervisor) revive(ctx context.Context, failedPort int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopping.Load() {
		return 0, fmt.Errorf("devicelab runner is shutting down")
	}
	if cur := s.handle.Load(); cur != nil && cur.port != failedPort {
		// A concurrent (or immediately prior) failed call already relaunched.
		return cur.port, nil
	}

	if err := s.takeRelaunchBudget(time.Now()); err != nil {
		return 0, err
	}

	if cur := s.handle.Load(); cur != nil {
		if err := cur.stopProcess(); err != nil {
			// Relaunch anyway: the new runner gets a fresh port, and the
			// warning is the only trace of a leaked xcodebuild.
			_, _ = fmt.Fprintf(s.warnOut(), "  ⚠ could not stop the dead devicelab runner: %v\n", err)
		}
	}

	_, _ = fmt.Fprintf(s.warnOut(), "  ↻ devicelab runner died mid-session — relaunching (%d/%d)\n",
		s.relaunches, maxRelaunchesPerWindow)

	relaunchCtx, cancel := context.WithTimeout(ctx, s.relaunchTimeout())
	defer cancel()
	_, handle, err := s.start(relaunchCtx, s.opts, s.xctestrun, s.logPath)
	if err != nil {
		return 0, fmt.Errorf("devicelab runner relaunch failed: %w", err)
	}
	s.handle.Store(handle)
	return handle.port, nil
}

// stop marks the supervisor shutting down (so a racing revive gives up) and
// terminates the live process.
func (s *Supervisor) stop() error {
	s.stopping.Store(true)
	if cur := s.handle.Load(); cur != nil {
		return cur.stopProcess()
	}
	return nil
}

// relaunchTimeout is opts.RelaunchTimeout, or DefaultRelaunchTimeout when unset.
func (s *Supervisor) relaunchTimeout() time.Duration {
	if s.opts.RelaunchTimeout > 0 {
		return s.opts.RelaunchTimeout
	}
	return DefaultRelaunchTimeout
}

// takeRelaunchBudget charges one relaunch against the budget, or refuses when
// maxRelaunchesPerWindow relaunches happened without the runner then staying
// up for relaunchWindow. Caller holds s.mu.
func (s *Supervisor) takeRelaunchBudget(now time.Time) error {
	if !s.lastRelaunch.IsZero() && now.Sub(s.lastRelaunch) > relaunchWindow {
		s.relaunches = 0 // healthy since the last relaunch: fresh budget
	}
	if s.relaunches >= maxRelaunchesPerWindow {
		return fmt.Errorf(
			"devicelab runner relaunch limit reached (%d relaunches without %s of uptime) — the runner keeps dying, likely a deterministic crash",
			maxRelaunchesPerWindow, relaunchWindow,
		)
	}
	s.relaunches++
	s.lastRelaunch = now
	return nil
}

// warnOut is where relaunch diagnostics go (s.warn, default os.Stderr).
func (s *Supervisor) warnOut() io.Writer {
	if s.warn != nil {
		return s.warn
	}
	return os.Stderr
}
