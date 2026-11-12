package device

import (
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// execCommand is a package-level seam over exec.Command so tests can
// override subprocess execution without spawning real children. Production
// code MUST always call this variable, not exec.Command directly. Tests
// override it via t.Cleanup(func(){ execCommand = exec.Command }) — they
// do not run with t.Parallel() to avoid clobbering.
var execCommand = exec.Command

// execLookPath is the companion seam over exec.LookPath. Same rules.
var execLookPath = exec.LookPath

// adbTimeout bounds an adb call that has no limit of its own: shell
// commands, forwards, uninstall, get-state, devices, screencap. A device
// that stops answering otherwise blocks the run with no output. 60s is the
// limit Maestro gives its adb calls. A var so tests can shorten it.
var adbTimeout = 60 * time.Second

// adbTransferTimeout bounds adb install, push and pull, which move files
// whose size we don't control (a large APK over USB 2, a long recording).
var adbTransferTimeout = 10 * time.Minute

// errTimedOut is wrapped by the error of a command that ran past its limit.
var errTimedOut = errors.New("timed out")

// waitDelayAfterKill is how long Wait waits for a killed command's output
// pipes to close; a grandchild (an adb server it started) may hold them.
const waitDelayAfterKill = 2 * time.Second

// runTimeout runs cmd and kills it if it runs past timeout (0 = no limit).
// desc names the command in the timeout error, e.g. "adb shell getprop".
// It uses the execCommand seam rather than exec.CommandContext so tests can
// substitute the command.
func runTimeout(cmd *exec.Cmd, timeout time.Duration, desc string) error {
	if timeout <= 0 {
		return cmd.Run()
	}
	cmd.WaitDelay = waitDelayAfterKill
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("%s: %w after %v", desc, errTimedOut, timeout)
	}
}
