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

func TestRotateSize(t *testing.T) {
	for r, want := range map[int][2]int{0: {1080, 2340}, 1: {2340, 1080}, 2: {1080, 2340}, 3: {2340, 1080}} {
		if w, h := RotateSize(1080, 2340, r); w != want[0] || h != want[1] {
			t.Errorf("RotateSize(1080, 2340, %d) = %dx%d, want %dx%d", r, w, h, want[0], want[1])
		}
	}
}

// LANDSCAPE is a quarter turn on a phone, but rotation 0 on a tablet whose
// natural orientation is landscape (#256).
func TestWaitForDisplayOrientation(t *testing.T) {
	rotation := func(r string) func(string) (string, error) {
		return func(string) (string, error) { return "mCurrentOrientation=" + r, nil }
	}
	if r, err := WaitForDisplayOrientation(rotation("1"), true, 1080, 2340, time.Second, time.Millisecond); err != nil || r != 1 {
		t.Errorf("phone turned to landscape: rotation %d err %v", r, err)
	}
	if r, err := WaitForDisplayOrientation(rotation("0"), true, 2560, 1600, time.Second, time.Millisecond); err != nil || r != 0 {
		t.Errorf("tablet at rest is landscape: rotation %d err %v", r, err)
	}
	// A display that never turns is not an error: the last rotation is kept.
	start := time.Now()
	if r, err := WaitForDisplayOrientation(rotation("0"), true, 1080, 2340, 20*time.Millisecond, time.Millisecond); err != nil || r != 0 || time.Since(start) < 20*time.Millisecond {
		t.Errorf("portrait-locked app: rotation %d err %v", r, err)
	}
	if r, _ := WaitForDisplayOrientation(func(string) (string, error) { return "", nil }, true, 1080, 2340, time.Second, time.Millisecond); r != -1 {
		t.Errorf("unobservable rotation = %d, want -1", r)
	}
	if r := DisplayRotation(rotation("3")); r != 3 {
		t.Errorf("DisplayRotation = %d, want 3", r)
	}
}

func TestParseWmSize(t *testing.T) {
	for out, want := range map[string][2]int{
		"Physical size: 1080x2340\n":                        {1080, 2340},
		"Physical size: 1080x2340\nOverride size: 720x1560": {720, 1560},
		"error": {0, 0},
	} {
		if w, h := ParseWmSize(out); w != want[0] || h != want[1] {
			t.Errorf("ParseWmSize(%q) = %dx%d, want %dx%d", out, w, h, want[0], want[1])
		}
	}
}
