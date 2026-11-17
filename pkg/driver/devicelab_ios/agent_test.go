package devicelab_ios

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestKeepPreviousLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xcodebuild.log")
	for i := 1; i <= 4; i++ {
		keepPreviousLog(path, 2)
		if err := os.WriteFile(path, []byte(strconv.Itoa(i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]string{"": "4", ".1": "3", ".2": "2"} {
		if got, _ := os.ReadFile(path + name); string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Error(".3 kept")
	}
}

// simctl's deadline holds when xcrun's output stays open after xcrun is
// killed, as it does when `simctl spawn` has handed it to a process in the
// simulator: the call returns soon after its deadline, not when that process
// ends.
func TestSimctlDeadlineHoldsWhenOutputStaysOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands in for xcrun")
	}
	dir := t.TempDir()
	// The child keeps the output open for 8 s after xcrun itself is killed.
	if err := os.WriteFile(filepath.Join(dir, "xcrun"), []byte("#!/bin/sh\nsleep 8 &\nsleep 8\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	start := time.Now()
	_, err := simctl(context.Background(), 200*time.Millisecond, "spawn", "SIM-1", "launchctl", "list")
	if err == nil {
		t.Fatal("a call past its deadline succeeded")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("simctl returned after %v; want soon after its 200ms deadline", elapsed)
	}
}
