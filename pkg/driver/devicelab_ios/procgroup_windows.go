//go:build windows

package devicelab_ios

import "os/exec"

// The iOS driver never runs on Windows; these exist so the package compiles
// for `GOOS=windows go build ./...`.

func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func killPidGroup(pid int) {}
