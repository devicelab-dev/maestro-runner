//go:build !windows

package wda

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// setProcessGroup puts xcodebuild in its own process group, so it outlives
// this process (Ctrl-C in a terminal signals the terminal's group) and can be
// stopped together with the children it spawned.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killPidGroup ends the process group pid leads.
func killPidGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func tryLockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func unlockFile(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// watchSimulatorXcodebuild starts a detached watcher that stops the
// xcodebuild process group pid leads once the simulator is no longer booted:
// xcodebuild does not exit when its simulator shuts down, and a kept WDA must
// not outlive it. The watcher ends by itself when xcodebuild does.
func watchSimulatorXcodebuild(udid string, pid int) {
	script := fmt.Sprintf(`while kill -0 %[1]d 2>/dev/null && xcrun simctl list devices booted | grep -q %[2]q; do sleep 20; done
kill -TERM -%[1]d 2>/dev/null; sleep 5; kill -KILL -%[1]d 2>/dev/null; exit 0`, pid, udid)
	w := exec.Command("/bin/sh", "-c", script)
	setProcessGroup(w)
	if err := w.Start(); err != nil {
		logger.Warn("[wda] could not watch the kept WDA (pid %d): %v", pid, err)
		return
	}
	go func() { _ = w.Wait() }()
}
