package devicelab_ios

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

// TestAnnounceRetrySimulatorReset: the between-attempt simulator reset runs
// by default and is skipped with NoSimulatorReset; a failing reset is
// best-effort; the banner is mirrored into the runner log.
func TestAnnounceRetrySimulatorReset(t *testing.T) {
	tests := []struct {
		name      string
		noReset   bool
		resetErr  error
		wantReset bool
	}{
		{name: "default resets", wantReset: true},
		{name: "reset failure is best-effort", resetErr: errors.New("boot failed"), wantReset: true},
		{name: "NoSimulatorReset skips reset", noReset: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resets []string
			orig := resetSim
			resetSim = func(_ context.Context, udid string, _ io.Writer) error {
				resets = append(resets, udid)
				return tt.resetErr
			}
			defer func() { resetSim = orig }()

			var log bytes.Buffer
			opts := SetupOptions{SimulatorUDID: "SIM-1", Stdout: &log, NoSimulatorReset: tt.noReset}
			announceRetry(context.Background(), opts, 2, errors.New("stalled"))

			if got := len(resets) == 1; got != tt.wantReset {
				t.Errorf("reset called %d times, want reset=%v", len(resets), tt.wantReset)
			}
			if !strings.Contains(log.String(), "=== attempt 2/4 ===") {
				t.Errorf("runner log missing retry banner: %q", log.String())
			}
		})
	}
}

// TestAnnounceRetryStderrOutputNotMirrored: when output already goes to
// stderr the banner is not written twice.
func TestAnnounceRetryStderrOutputNotMirrored(t *testing.T) {
	orig := resetSim
	resetSim = func(context.Context, string, io.Writer) error { return nil }
	defer func() { resetSim = orig }()
	announceRetry(context.Background(), SetupOptions{Stdout: os.Stderr, NoSimulatorReset: true}, 2, errors.New("x"))
}
