//go:build windows

package devicelab_ios

import "os/exec"

// setProcessGroup is a no-op on Windows, which has no process groups in the
// POSIX sense — syscall.SysProcAttr there carries no Setpgid field at all.
//
// Nothing is lost: this package drives xcodebuild against iOS simulators, so on
// Windows it never runs. It exists only so the package compiles, which is what
// `GOOS=windows go build ./...` needs in order to build the parts of
// maestro-runner that do work there.
func setProcessGroup(cmd *exec.Cmd) {}
