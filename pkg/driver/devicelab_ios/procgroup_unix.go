//go:build !windows

package devicelab_ios

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts xcodebuild in its own process group.
//
// Without it, signalling the runner's group reaches xcodebuild too, and a
// Ctrl-C aimed at a test run would tear down the build mid-flight rather than
// letting the runner stop it in order.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
