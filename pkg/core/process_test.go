package core

import (
	"testing"
	"time"
)

func TestWaitForProcessExit(t *testing.T) {
	calls := 0
	shell := func(string) (string, error) {
		calls++
		if calls < 3 {
			return "12345\n", nil
		}
		return "", nil
	}
	if !WaitForProcessExit(shell, "screenrecord", time.Second, time.Millisecond) {
		t.Fatal("expected the process to be reported gone on the third read")
	}
	if calls != 3 {
		t.Errorf("polled %d times, want 3", calls)
	}
	stuck := func(string) (string, error) { return "12345", nil }
	if WaitForProcessExit(stuck, "screenrecord", 5*time.Millisecond, time.Millisecond) {
		t.Fatal("a process that never exits should time out as false")
	}
}
