package core

import (
	"strings"
	"testing"
	"time"
)

func TestWaitForDisplayRotation_ReturnsOnceDisplayTurns(t *testing.T) {
	calls := 0
	shell := func(string) (string, error) {
		calls++
		if calls < 3 {
			return "  mCurrentOrientation=0\n", nil
		}
		return "  mCurrentOrientation=1\n", nil
	}
	if err := WaitForDisplayRotation(shell, 1, time.Second, time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected to poll until the third read, polled %d times", calls)
	}
}

func TestWaitForDisplayRotation_TimesOutWithObservedValue(t *testing.T) {
	shell := func(string) (string, error) { return "mCurrentOrientation=0", nil }
	err := WaitForDisplayRotation(shell, 3, 10*time.Millisecond, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "reports rotation 0, wanted 3") {
		t.Fatalf("expected a timeout naming the observed rotation, got %v", err)
	}
}

// A device whose dumpsys lacks the field cannot be observed; the flow must not
// be held for the full timeout on it.
func TestWaitForDisplayRotation_UnobservableReturnsImmediately(t *testing.T) {
	calls := 0
	shell := func(string) (string, error) { calls++; return "no such field here", nil }
	if err := WaitForDisplayRotation(shell, 1, time.Second, time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected a single read, got %d", calls)
	}
}

func TestWaitForDisplayRotation_ShellErrorSurfaces(t *testing.T) {
	shell := func(string) (string, error) { return "", errFake }
	if err := WaitForDisplayRotation(shell, 1, time.Second, time.Millisecond); err == nil {
		t.Fatal("expected the shell error to surface")
	}
}

type fakeErr struct{}

func (fakeErr) Error() string { return "fake" }

var errFake = fakeErr{}
