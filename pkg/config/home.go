// SPDX-FileCopyrightText: 2024 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package config

import (
	"os"
	"path/filepath"
	"sync"
)

const envHome = "MAESTRO_RUNNER_HOME"

var (
	homeOnce sync.Once
	homeDir  string
)

// GetHome returns the maestro-runner home directory.
//
// Resolution order:
//  1. $MAESTRO_RUNNER_HOME environment variable
//  2. Parent of the binary's directory (if binary is in <home>/bin/)
//  3. Current working directory (development fallback)
func GetHome() string {
	homeOnce.Do(func() {
		homeDir = resolveHome()
	})
	return homeDir
}

// GetCacheDir returns <home>/cache.
func GetCacheDir() string {
	return filepath.Join(GetHome(), "cache")
}

// GetDriversDir returns <home>/drivers/<platform>.
func GetDriversDir(platform string) string {
	return filepath.Join(GetHome(), "drivers", platform)
}

func resolveHome() string {
	// 1. Environment variable
	if env := os.Getenv(envHome); env != "" {
		return env
	}

	// 2. Binary-relative: if binary is at <home>/bin/maestro-runner, use <home>
	if execPath, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
			execPath = resolved
		}
		binDir := filepath.Dir(execPath)
		if filepath.Base(binDir) == "bin" {
			return filepath.Dir(binDir)
		}
	}

	// 3. Current working directory
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}

	return "."
}

// ResetHome resets the cached home directory (for testing).
func ResetHome() {
	homeOnce = sync.Once{}
	homeDir = ""
}
