//go:build !windows

package devicelab_ios

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts xcodebuild in its own process group, so stopping the
// agent ends xcodebuild and the children it spawned together.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup ends cmd and its process group.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	_ = cmd.Process.Kill()
}

// killPidGroup ends the process group pid leads: a kept xcodebuild from an
// earlier run, known only by its pid.
func killPidGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
