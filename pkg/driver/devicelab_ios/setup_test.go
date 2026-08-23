package devicelab_ios

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- #118: fast-exiting xcodebuild must surface its real error, not "stalled" ---

func TestExitedBeforeReadyError_XcodebuildErrorIsPermanent(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "runner.log")
	content := "noise\nxcodebuild: error: Unable to find a device matching the provided destination specifier\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	err := exitedBeforeReadyError(logPath, fmt.Errorf("exit status 70"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Unable to find a device") {
		t.Errorf("expected the real xcodebuild error to surface, got: %v", err)
	}
	var perm *permanentStartupError
	if !errors.As(err, &perm) {
		t.Errorf("expected permanentStartupError (skips retries), got %T: %v", err, err)
	}
}

func TestExitedBeforeReadyError_CrashIsRetryable(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "runner.log")
	if err := os.WriteFile(logPath, []byte("some unrelated output\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := exitedBeforeReadyError(logPath, fmt.Errorf("signal: killed"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Errorf("expected exit-status error, got: %v", err)
	}
	var perm *permanentStartupError
	if errors.As(err, &perm) {
		t.Errorf("crash exits should stay retryable, got permanent error: %v", err)
	}
}

func TestExitedBeforeReadyError_NoLogPath(t *testing.T) {
	err := exitedBeforeReadyError("", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 0") {
		t.Errorf("expected status-0 description, got: %v", err)
	}
}
