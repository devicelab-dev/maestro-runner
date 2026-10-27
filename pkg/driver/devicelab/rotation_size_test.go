package devicelab

import (
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// After setOrientation: LANDSCAPE the screen is 2340x1080, so a percentage
// point lands where it should and a point right of x=1080 is on screen (#256).
// The size read at session start, and `wm size`, stay portrait.
func TestScreenSizeFollowsSetOrientation(t *testing.T) {
	shell := &mockShell{out: "Physical size: 1080x2340\nmCurrentOrientation=1\n"}
	d := New(&mockDeviceLabClient{}, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2340}, shell)

	if res := d.setOrientation(&flow.SetOrientationStep{Orientation: "LANDSCAPE"}); !res.Success {
		t.Fatalf("setOrientation: %v", res.Error)
	}
	if w, h, _ := d.screenSize(); w != 2340 || h != 1080 {
		t.Errorf("screenSize = %dx%d, want 2340x1080", w, h)
	}
	if w, h, _ := d.tappableScreenSize(); w != 2340 || h != 1080 {
		t.Errorf("tappableScreenSize = %dx%d, want 2340x1080", w, h)
	}
	w, h, _ := d.screenSize()
	if x, y, err := core.ParsePointCoords("50%, 40%", w, h); err != nil || x != 1170 || y != 432 {
		t.Errorf("50%%, 40%% = (%d, %d) %v, want (1170, 432)", x, y, err)
	}
	if res := d.tapOnPointWithCoords("1170, 432"); !res.Success {
		t.Errorf("a landscape point past x=1080 was refused: %v", res.Error)
	}

	shell.out = "Physical size: 1080x2340\nmCurrentOrientation=0\n"
	d.setOrientation(&flow.SetOrientationStep{Orientation: "PORTRAIT"})
	if w, h, _ := d.screenSize(); w != 1080 || h != 2340 {
		t.Errorf("back in portrait: screenSize = %dx%d, want 1080x2340", w, h)
	}
}

// A device that does not report its rotation is taken to have turned to the
// rotation just set; a device that reports another one (an app locked to
// portrait) keeps the size of the rotation it reports.
func TestScreenSizeWhenRotationNotReached(t *testing.T) {
	shell := &mockShell{out: "Physical size: 1080x2340\n"}
	d := New(&mockDeviceLabClient{}, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2340}, shell)
	d.setOrientation(&flow.SetOrientationStep{Orientation: "LANDSCAPE"})
	if w, h, _ := d.screenSize(); w != 2340 || h != 1080 {
		t.Errorf("unobservable: screenSize = %dx%d, want 2340x1080", w, h)
	}

	saved := rotationSettleTimeout
	rotationSettleTimeout = 10 * time.Millisecond
	t.Cleanup(func() { rotationSettleTimeout = saved })
	shell = &mockShell{out: "Physical size: 1080x2340\nmCurrentOrientation=0\n"}
	d = New(&mockDeviceLabClient{}, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2340}, shell)
	d.setOrientation(&flow.SetOrientationStep{Orientation: "LANDSCAPE"})
	if w, h, _ := d.screenSize(); w != 1080 || h != 2340 {
		t.Errorf("portrait-locked: screenSize = %dx%d, want 1080x2340", w, h)
	}
}
