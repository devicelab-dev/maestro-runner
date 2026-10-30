package appium

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// After setOrientation the window size is read again, so the on-screen
// filter and percentage points use the landscape size (#256). The server
// reports the old size for a moment after the call returns.
func TestSetOrientationRereadsWindowSize(t *testing.T) {
	landscape, reads := false, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/orientation"):
			landscape = true
			writeJSON(w, map[string]interface{}{"value": nil})
		case strings.HasSuffix(r.URL.Path, "/window/rect"):
			reads++
			size := map[string]interface{}{"x": 0, "y": 0, "width": 1080, "height": 2340}
			if landscape && reads > 1 {
				size["width"], size["height"] = 2340, 1080
			}
			writeJSON(w, map[string]interface{}{"value": size})
		default:
			writeJSON(w, map[string]interface{}{"value": nil})
		}
	}))
	defer server.Close()
	d := createTestAppiumDriver(server)

	if res := d.setOrientation(&flow.SetOrientationStep{Orientation: "LANDSCAPE"}); !res.Success {
		t.Fatalf("setOrientation: %v", res.Error)
	}
	if w, h := d.client.ScreenSize(); w != 2340 || h != 1080 {
		t.Errorf("ScreenSize = %dx%d, want 2340x1080", w, h)
	}
	if reads < 2 {
		t.Errorf("window size read %d times, want it read until it turned", reads)
	}
	// An element right of the portrait width is on screen now.
	tan := &ParsedElement{Text: "tan"}
	tan.Bounds.X, tan.Bounds.Y, tan.Bounds.Width, tan.Bounds.Height = 2046, 597, 261, 131
	if got := d.onScreen([]*ParsedElement{tan}); len(got) != 1 {
		t.Error("an element at x=2046 in landscape was filtered out as off screen")
	}
}
