package wda

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

func TestIsAppDeathError(t *testing.T) {
	cases := []struct {
		name string
		r    *core.CommandResult
		want bool
	}{
		{"nil result", nil, false},
		{"empty result", &core.CommandResult{}, false},
		{"app not in foreground", &core.CommandResult{Message: "Application is not in foreground state"}, true},
		{"app not running", &core.CommandResult{Message: "Application is not running"}, true},
		{"session lost", &core.CommandResult{Error: fmt.Errorf("session does not exist")}, true},
		{"invalid session", &core.CommandResult{Error: fmt.Errorf("invalid session id")}, true},
		{"connection refused", &core.CommandResult{Message: "post failed: connection refused"}, true},
		{"unrelated error", &core.CommandResult{Message: "element not found: id=submit"}, false},
		{"successful result", &core.CommandResult{Success: true, Message: "Tapped on element"}, false},
		{"case insensitive", &core.CommandResult{Message: "APPLICATION DIED unexpectedly"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAppDeathError(tc.r); got != tc.want {
				t.Errorf("isAppDeathError = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTrackCrashLoop_TripsAfterThreshold(t *testing.T) {
	d := &Driver{}

	// First N-1 deaths shouldn't trip.
	for i := 0; i < crashLoopThreshold-1; i++ {
		d.trackCrashLoop(&core.CommandResult{
			Error:   fmt.Errorf("Application is not running"),
			Message: "Application is not running",
		})
	}
	if d.crashAbortReason != "" {
		t.Fatalf("crashAbortReason set early (after %d failures, threshold is %d)",
			crashLoopThreshold-1, crashLoopThreshold)
	}

	// Nth death trips the breaker.
	d.trackCrashLoop(&core.CommandResult{
		Error:   fmt.Errorf("Application is not running"),
		Message: "Application is not running",
	})
	if d.crashAbortReason == "" {
		t.Fatalf("expected crashAbortReason to be set after %d failures", crashLoopThreshold)
	}
	if !strings.Contains(d.crashAbortReason, "crashing on launch") {
		t.Errorf("reason missing expected diagnosis text: %s", d.crashAbortReason)
	}
	if !strings.Contains(d.crashAbortReason, "flutter build ios --release") {
		t.Errorf("reason missing Flutter rebuild hint: %s", d.crashAbortReason)
	}
}

func TestTrackCrashLoop_SuccessResetsCounter(t *testing.T) {
	d := &Driver{}

	for i := 0; i < crashLoopThreshold-1; i++ {
		d.trackCrashLoop(&core.CommandResult{
			Error:   fmt.Errorf("session does not exist"),
			Message: "session does not exist",
		})
	}
	if d.appDeathCount == 0 {
		t.Fatal("expected appDeathCount > 0 after deaths")
	}

	// A successful step → counter resets.
	d.trackCrashLoop(&core.CommandResult{Success: true})
	if d.appDeathCount != 0 {
		t.Errorf("expected appDeathCount=0 after success, got %d", d.appDeathCount)
	}
	if d.crashAbortReason != "" {
		t.Errorf("crashAbortReason should be empty after recovery: %s", d.crashAbortReason)
	}
}

func TestTrackCrashLoop_WindowExpires(t *testing.T) {
	d := &Driver{}
	d.trackCrashLoop(&core.CommandResult{
		Error:   fmt.Errorf("Application is not running"),
		Message: "Application is not running",
	})
	first := d.appDeathFirstAt

	// Simulate the time window expiring.
	d.appDeathFirstAt = time.Now().Add(-2 * crashLoopTimeWindow)

	d.trackCrashLoop(&core.CommandResult{
		Error:   fmt.Errorf("Application is not running"),
		Message: "Application is not running",
	})
	if d.appDeathCount != 1 {
		t.Errorf("expected counter to reset after window expiry, got %d", d.appDeathCount)
	}
	if d.appDeathFirstAt.Equal(first) {
		t.Error("expected appDeathFirstAt to be updated")
	}
}

func TestTrackCrashLoop_UnrelatedErrorsDoNotCount(t *testing.T) {
	d := &Driver{}
	for i := 0; i < crashLoopThreshold*2; i++ {
		d.trackCrashLoop(&core.CommandResult{
			Error:   fmt.Errorf("element not found: text='Login'"),
			Message: "element not found: text='Login'",
		})
	}
	if d.crashAbortReason != "" {
		t.Errorf("unrelated errors should not trip crash-loop; got reason: %s", d.crashAbortReason)
	}
}
