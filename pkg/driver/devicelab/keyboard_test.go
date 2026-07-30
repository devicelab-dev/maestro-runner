package devicelab

import (
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Per-sample verdict for the keyboard-blocking settle loop. Pinned with the real-world
// geometry that motivated it: an AlertDialog positive button (android:id/button1)
// reported at centerY=1627 on the first frame after typing (keyboard top 1560), then
// lifted to ~1400 once the window's SOFT_INPUT_ADJUST_RESIZE relayout settled — the
// guard must treat the first sample as "still covering" and the second as clear, and a
// dismissed keyboard (nil bounds) as never covering.

func elementCenteredAtY(cy int) core.Bounds {
	// Height 100 → center = Y + 50.
	return core.Bounds{X: 400, Y: cy - 50, Width: 200, Height: 100}
}

func TestKeyboardStillCovering(t *testing.T) {
	keyboard := &core.Bounds{X: 0, Y: 1560, Width: 1080, Height: 840}

	cases := []struct {
		name     string
		element  core.Bounds
		keyboard *core.Bounds
		want     bool
	}{
		{"covered on first frame after typing", elementCenteredAtY(1627), keyboard, true},
		{"clear after ADJUST_RESIZE relayout", elementCenteredAtY(1400), keyboard, false},
		{"keyboard dismissed mid-settle", elementCenteredAtY(1627), nil, false},
		// The 50px suggestion-strip margin: at keyboard.Y+margin is covered…
		{"at margin boundary", elementCenteredAtY(1610), keyboard, true},
		// …one pixel above the margin is still tappable.
		{"just above margin", elementCenteredAtY(1609), keyboard, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := keyboardStillCovering(tc.element, tc.keyboard)
			if got != tc.want {
				t.Errorf("keyboardStillCovering(%+v, %+v) = %v, want %v",
					tc.element, tc.keyboard, got, tc.want)
			}
		})
	}
}

// --- Settle loop -----------------------------------------------------------------------

// shrinkSettleWindow shortens the settle window so loop tests run in milliseconds,
// restoring the real value afterwards.
func shrinkSettleWindow(t *testing.T, d time.Duration) {
	t.Helper()
	orig := keyboardSettleWindow
	keyboardSettleWindow = d
	t.Cleanup(func() { keyboardSettleWindow = orig })
}

func TestSettleKeyboardBlocking_ElementNotFound(t *testing.T) {
	shrinkSettleWindow(t, 100*time.Millisecond)
	res := settleKeyboardBlocking(
		func() (*core.ElementInfo, bool) { return nil, false },
		func() *core.Bounds {
			t.Fatal("keyboardBounds must not be sampled when the element is missing")
			return nil
		},
	)
	if res != nil {
		t.Errorf("expected nil (caller does the full-timeout find), got %+v", res)
	}
}

func TestSettleKeyboardBlocking_ClearOnFirstSample(t *testing.T) {
	shrinkSettleWindow(t, 100*time.Millisecond)
	samples := 0
	res := settleKeyboardBlocking(
		func() (*core.ElementInfo, bool) {
			samples++
			return &core.ElementInfo{Bounds: elementCenteredAtY(1400)}, true
		},
		func() *core.Bounds { return &core.Bounds{X: 0, Y: 1560, Width: 1080, Height: 840} },
	)
	if res != nil {
		t.Errorf("element above the keyboard must not block, got %+v", res)
	}
	if samples != 1 {
		t.Errorf("clear-on-first-frame must return without re-sampling, sampled %d times", samples)
	}
}

func TestSettleKeyboardBlocking_RelayoutLiftsElementMidSettle(t *testing.T) {
	// The real-world repro: first frame still covered (centerY 1627 vs keyboard top
	// 1560), then the ADJUST_RESIZE relayout lifts the element clear (~1400).
	shrinkSettleWindow(t, 2*time.Second) // generous — the early exit must beat it
	samples := 0
	start := time.Now()
	res := settleKeyboardBlocking(
		func() (*core.ElementInfo, bool) {
			samples++
			if samples == 1 {
				return &core.ElementInfo{Bounds: elementCenteredAtY(1627)}, true
			}
			return &core.ElementInfo{Bounds: elementCenteredAtY(1400)}, true
		},
		func() *core.Bounds { return &core.Bounds{X: 0, Y: 1560, Width: 1080, Height: 840} },
	)
	if res != nil {
		t.Errorf("element lifted mid-settle must not block, got %+v", res)
	}
	if samples != 2 {
		t.Errorf("expected exactly 2 samples (covered, then clear), got %d", samples)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("early exit expected well before the settle window, took %v", elapsed)
	}
}

func TestSettleKeyboardBlocking_KeyboardDismissedMidSettle(t *testing.T) {
	shrinkSettleWindow(t, 2*time.Second)
	samples := 0
	res := settleKeyboardBlocking(
		func() (*core.ElementInfo, bool) {
			samples++
			return &core.ElementInfo{Bounds: elementCenteredAtY(1627)}, true
		},
		func() *core.Bounds {
			if samples == 1 {
				return &core.Bounds{X: 0, Y: 1560, Width: 1080, Height: 840}
			}
			return nil // keyboard gone
		},
	)
	if res != nil {
		t.Errorf("dismissed keyboard must not block, got %+v", res)
	}
}

func TestSettleKeyboardBlocking_PersistentOverlapFails(t *testing.T) {
	shrinkSettleWindow(t, 120*time.Millisecond) // ~3 samples at the 50ms cadence
	samples := 0
	res := settleKeyboardBlocking(
		func() (*core.ElementInfo, bool) {
			samples++
			return &core.ElementInfo{Bounds: elementCenteredAtY(1627)}, true
		},
		func() *core.Bounds { return &core.Bounds{X: 0, Y: 1560, Width: 1080, Height: 840} },
	)
	if res == nil {
		t.Fatal("persistent overlap through the whole settle window must fail")
	}
	if samples < 2 {
		t.Errorf("expected re-sampling before failing, sampled %d times", samples)
	}
	if !strings.Contains(res.Message, "keyboard top: 1560") ||
		!strings.Contains(res.Message, "element center Y: 1627") {
		t.Errorf("error must keep the actionable geometry hint, got %q", res.Message)
	}
}

func TestCheckKeyboardBlocking_SkipsWhenPreviousStepWasNotInput(t *testing.T) {
	d := &Driver{client: &mockDeviceLabClient{}}
	if res := d.checkKeyboardBlocking(false, flow.Selector{ID: "android:id/button1"}); res != nil {
		t.Errorf("check must not apply after a non-input step, got %+v", res)
	}
}

func TestCheckKeyboardBlocking_ElementNotFoundProceeds(t *testing.T) {
	// End-to-end through the driver: the mock client fails FindElement, so the
	// settle loop must bail out and let the caller do the full-timeout find.
	shrinkSettleWindow(t, 100*time.Millisecond)
	d := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, &mockShell{})
	if res := d.checkKeyboardBlocking(true, flow.Selector{ID: "android:id/button1"}); res != nil {
		t.Errorf("element not found must proceed to the normal find, got %+v", res)
	}
}
