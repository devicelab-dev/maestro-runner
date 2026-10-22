//go:build windows

package wda

import (
	"os"
	"os/exec"
)

// WDA never runs on Windows; these exist so the package compiles for
// `GOOS=windows go build ./...`.

func setProcessGroup(cmd *exec.Cmd) {}

func killPidGroup(pid int) {}

func tryLockFile(f *os.File) error { return nil }

func unlockFile(f *os.File) {}

func watchSimulatorXcodebuild(udid string, pid int) {}
