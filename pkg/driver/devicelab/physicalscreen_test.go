package devicelab

import (
	"errors"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

func TestParseWmSize(t *testing.T) {
	cases := []struct {
		name         string
		out          string
		wantW, wantH int
	}{
		{"physical only", "Physical size: 1080x2340\n", 1080, 2340},
		{"override preferred over physical", "Physical size: 1080x2340\nOverride size: 1080x2160\n", 1080, 2160},
		{"crlf and surrounding spaces", "  Physical size: 720x1280  \r\n", 720, 1280},
		{"garbage", "error: could not access display", 0, 0},
		{"empty", "", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, h := parseWmSize(c.out)
			if w != c.wantW || h != c.wantH {
				t.Errorf("parseWmSize(%q) = %dx%d, want %dx%d", c.out, w, h, c.wantW, c.wantH)
			}
		})
	}
}

// TestTappableScreenSize_UsesPhysicalNotUsable is the regression for the off-screen
// tap bug: the on-device driver reports the USABLE height (2204 = 2340 - 136px status
// bar), but element bounds come from the accessibility hierarchy in full-display
// coords. A bottom-anchored AlertDialog button at centre y=2219 is on screen
// (2219 < 2340) yet would be rejected against the usable height (2219 >= 2204). The
// tap guard must use the physical display size.
func TestTappableScreenSize_UsesPhysicalNotUsable(t *testing.T) {
	shell := &mockShell{out: "Physical size: 1080x2340\n"}
	d := New(&mockDeviceLabClient{}, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2204}, shell)

	sw, sh, err := d.tappableScreenSize()
	if err != nil {
		t.Fatalf("tappableScreenSize: %v", err)
	}
	if sw != 1080 || sh != 2340 {
		t.Fatalf("tappableScreenSize = %dx%d, want 1080x2340 (physical, not usable 2204)", sw, sh)
	}

	// Bottom dialog button: bounds [764,2153][976,2285], centre (870,2219).
	bottomBtn := core.Bounds{X: 764, Y: 2153, Width: 212, Height: 132}
	if !boundsTappable(bottomBtn, sw, sh) {
		t.Errorf("bottom button (centre y=2219) should be tappable on a 2340-tall display")
	}
	// Sanity: against the (wrong) usable height it is rejected — that was the bug.
	if boundsTappable(bottomBtn, 1080, 2204) {
		t.Errorf("centre y=2219 must be rejected against usable height 2204 (bug repro)")
	}
}

func TestTappableScreenSize_FallsBackToReported(t *testing.T) {
	// Physical size unavailable (shell errors) → fall back to PlatformInfo.
	shell := &mockShell{err: errors.New("no shell")}
	d := New(&mockDeviceLabClient{}, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2204}, shell)
	sw, sh, err := d.tappableScreenSize()
	if err != nil || sw != 1080 || sh != 2204 {
		t.Fatalf("fallback tappableScreenSize = %dx%d err=%v, want 1080x2204", sw, sh, err)
	}
}

// TestIsElementOnScreen_BottomBand is the scroll-side analogue of the tap fix: the last
// nav-drawer item ("Configuration") sits in the bottom system-bar band — bounds [61,2208][709,2340]
// on a 1080x2340 display whose USABLE height is 2204. It is genuinely on screen and tappable, so
// isElementOnScreen must accept it when given the PHYSICAL height; otherwise scrollUntilVisible
// loops to the scroll cap on an item that is already shown. scrollUntilVisible now sources the
// height from tappableScreenSize() (physical) for exactly this reason.
func TestIsElementOnScreen_BottomBand(t *testing.T) {
	lastDrawerItem := &core.ElementInfo{Bounds: core.Bounds{X: 61, Y: 2208, Width: 648, Height: 132}}
	if isElementOnScreen(lastDrawerItem, 1080, 2204) {
		t.Errorf("sanity: y=2208 must be off-screen against usable height 2204 (the bug)")
	}
	if !isElementOnScreen(lastDrawerItem, 1080, 2340) {
		t.Errorf("last drawer item (y=2208..2340) must be on-screen against physical height 2340")
	}
}

func TestPhysicalScreenSize_CachesWmSize(t *testing.T) {
	shell := &mockShell{out: "Physical size: 1080x2340\n"}
	d := New(&mockDeviceLabClient{}, &core.PlatformInfo{}, shell)
	for i := 0; i < 3; i++ {
		if w, h, ok := d.physicalScreenSize(); !ok || w != 1080 || h != 2340 {
			t.Fatalf("physicalScreenSize() = %dx%d ok=%v, want 1080x2340 true", w, h, ok)
		}
	}
	if len(shell.commands) != 1 || shell.commands[0] != "wm size" {
		t.Errorf("expected exactly one `wm size` shell call (cached), got %v", shell.commands)
	}
}
