package devicelab

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
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
