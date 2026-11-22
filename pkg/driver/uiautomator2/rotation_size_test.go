package uiautomator2

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// After a quarter turn the screen size is turned too, so percentage points
// land where they should (#256); turning back restores it.
func TestScreenSizeFollowsSetOrientation(t *testing.T) {
	shell := &MockShellExecutor{response: "Physical size: 1080x2340\nmCurrentOrientation=1\n"}
	d := &Driver{device: shell, info: &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2340}}

	if res := d.setOrientation(&flow.SetOrientationStep{Orientation: "LANDSCAPE_LEFT"}); !res.Success {
		t.Fatalf("setOrientation: %v", res.Error)
	}
	w, h, _ := d.screenSize()
	if w != 2340 || h != 1080 {
		t.Fatalf("screenSize = %dx%d, want 2340x1080", w, h)
	}
	if x, y, err := core.ParsePointCoords("50%, 40%", w, h); err != nil || x != 1170 || y != 432 {
		t.Errorf("50%%, 40%% = (%d, %d) %v, want (1170, 432)", x, y, err)
	}

	shell.response = "Physical size: 1080x2340\nmCurrentOrientation=2\n"
	d.setOrientation(&flow.SetOrientationStep{Orientation: "UPSIDE_DOWN"})
	if w, h, _ := d.screenSize(); w != 1080 || h != 2340 {
		t.Errorf("upside down: screenSize = %dx%d, want 1080x2340", w, h)
	}
}
