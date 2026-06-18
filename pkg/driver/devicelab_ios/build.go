// SPDX-FileCopyrightText: 2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package devicelab_ios

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/config"
)

// GetRunnerSourcePath returns the path where the devicelab-ios-runner
// Xcode project is vendored. Releases ship the source under
// drivers/ios/DevicelabIOSRunner/; same convention as the WDA driver.
func GetRunnerSourcePath() string {
	return filepath.Join(config.GetDriversDir("ios"), "DevicelabIOSRunner")
}

// GetRunnerBuildCacheDir returns the cache directory for build artifacts
// keyed by simulator iOS version. Different iOS versions produce different
// .xctestrun files (linker output references the SDK version), so each gets
// its own cache slot. Mirrors the WDA cache-by-sim-version pattern.
func GetRunnerBuildCacheDir(simulatorUDID string) (string, error) {
	iosVersion, err := simulatorOSVersion(simulatorUDID)
	if err != nil {
		return "", err
	}
	configName := fmt.Sprintf("sim-ios%s", iosVersion)
	return filepath.Join(config.GetCacheDir(), "devicelab-ios-runner-builds", configName), nil
}

// EnsureBuilt returns the artifacts directory for the given simulator,
// building the runner if no cached build exists. The returned path is
// laid out as expected by Setup:
//
//	<artifactsDir>/Build/Products/*.xctestrun
//	<artifactsDir>/Build/Products/Debug-iphonesimulator/DevicelabIOSRunner.app
//	<artifactsDir>/Build/Products/Debug-iphonesimulator/DevicelabIOSRunnerUITests-Runner.app
//
// First build takes ~30-60s on M-series Macs. Subsequent runs reuse the
// cache and return in milliseconds.
func EnsureBuilt(ctx context.Context, simulatorUDID string) (string, error) {
	sourcePath := GetRunnerSourcePath()
	projectPath := filepath.Join(sourcePath, "DevicelabIOSRunner.xcodeproj")
	if _, err := os.Stat(projectPath); err != nil {
		return "", fmt.Errorf(
			"devicelab-ios-runner source not found at %s — bundled runner missing from install.\n"+
				"Reinstall maestro-runner.",
			sourcePath,
		)
	}

	cacheDir, err := GetRunnerBuildCacheDir(simulatorUDID)
	if err != nil {
		return "", fmt.Errorf("resolve build cache dir: %w", err)
	}

	if _, err := findXctestrun(cacheDir); err == nil {
		// Cached build exists.
		return cacheDir, nil
	}

	if err := os.MkdirAll(filepath.Join(cacheDir, "logs"), 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	fmt.Println("\n  ⏳ Building devicelab-ios-runner for the first time...")
	fmt.Println("     ~30-60s on Apple Silicon. Subsequent runs reuse the cache.")
	fmt.Println()

	logPath := filepath.Join(cacheDir, "logs", "build.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return "", fmt.Errorf("create build log: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	buildCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	args := []string{
		"build-for-testing",
		"-project", projectPath,
		"-scheme", "DevicelabIOSRunnerUITests",
		"-destination", "generic/platform=iOS Simulator",
		"-derivedDataPath", cacheDir,
		"COMPILER_INDEX_STORE_ENABLE=NO",
		"ENABLE_CODE_COVERAGE=NO",
		"CODE_SIGNING_ALLOWED=NO",
	}
	cmd := exec.CommandContext(buildCtx, "xcodebuild", args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("xcodebuild failed:\n%s\n\nFull log: %s",
			tailFile(logPath, 30), logPath)
	}

	if _, err := findXctestrun(cacheDir); err != nil {
		return "", fmt.Errorf("build succeeded but no .xctestrun under %s: %w", cacheDir, err)
	}

	fmt.Println("  ✓ devicelab-ios-runner built")
	return cacheDir, nil
}

// simulatorOSVersion returns the iOS runtime version (e.g. "26.2") for a
// booted simulator. Used to key the build cache.
func simulatorOSVersion(udid string) (string, error) {
	cmd := exec.Command("xcrun", "simctl", "list", "devices", "-j")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("simctl list: %w", err)
	}
	version, err := extractRuntimeVersion(out, udid)
	if err != nil {
		return "", err
	}
	return version, nil
}

// tailFile returns the last n lines of a file (or the whole file if shorter).
func tailFile(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("(failed to read log: %v)", err)
	}
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) <= n {
		return string(data)
	}
	return string(bytes.Join(lines[len(lines)-n:], []byte("\n")))
}
