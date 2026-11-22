package appium

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// zeroAreaServer answers every element query with one element whose rect is
// 0x0 at (100,200) for the first zeroReads rect reads, then 200x100 there.
// It counts the /actions a tap sends.
func zeroAreaServer(t *testing.T, platform string, zeroReads int32) (*Driver, *int32) {
	t.Helper()
	var rectReads, actions int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/actions"):
			atomic.AddInt32(&actions, 1)
			writeJSON(w, map[string]interface{}{"value": nil})
		case strings.HasSuffix(p, "/element") && r.Method == http.MethodPost:
			writeJSON(w, map[string]interface{}{"value": map[string]interface{}{w3cElementKey: "dot"}})
		case strings.HasSuffix(p, "/rect"):
			size := 0
			if atomic.AddInt32(&rectReads, 1) > zeroReads {
				size = 100
			}
			writeJSON(w, map[string]interface{}{"value": map[string]interface{}{"x": 100, "y": 200, "width": size * 2, "height": size}})
		case strings.HasSuffix(p, "/displayed"):
			writeJSON(w, map[string]interface{}{"value": true})
		default:
			writeJSON(w, map[string]interface{}{"value": ""})
		}
	}))
	t.Cleanup(server.Close)
	d := createTestAppiumDriver(server)
	d.platform = platform
	d.client.platform = platform
	return d, &actions
}

// A 0x0 element has only its origin to tap, a point on whatever else is drawn
// there. The tap is refused rather than sent there.
func TestTapOnRefusesAZeroAreaElement(t *testing.T) {
	d, actions := zeroAreaServer(t, "android", 1<<30)
	res := d.tapOn(&flow.TapOnStep{Selector: flow.Selector{ID: "dot"}, BaseStep: flow.BaseStep{TimeoutMs: 300}})
	if res.Success {
		t.Fatal("tapOn succeeded on a 0x0 element")
	}
	if n := atomic.LoadInt32(actions); n != 0 {
		t.Errorf("sent %d tap actions, want none", n)
	}
	if res.Error == nil || !strings.Contains(res.Error.Error(), "no area") {
		t.Errorf("error = %v, want it to say the element has no area", res.Error)
	}
}

// The lookup keeps polling, so an element laid out a moment later is tapped
// at its centre.
func TestTapOnWaitsForAZeroAreaElementToBeLaidOut(t *testing.T) {
	d, actions := zeroAreaServer(t, "android", 1)
	res := d.tapOn(&flow.TapOnStep{Selector: flow.Selector{ID: "dot"}, BaseStep: flow.BaseStep{TimeoutMs: 2000}})
	if !res.Success {
		t.Fatalf("tapOn failed: %s", res.Message)
	}
	if n := atomic.LoadInt32(actions); n != 1 {
		t.Errorf("sent %d tap actions, want 1", n)
	}
	if res.Element == nil || res.Element.Bounds.Width != 200 {
		t.Errorf("tapped %+v, want the laid-out 200x100 element", res.Element)
	}
}

func TestDoubleTapAndLongPressRefuseAZeroAreaElement(t *testing.T) {
	d, actions := zeroAreaServer(t, "android", 1<<30)
	base := flow.BaseStep{TimeoutMs: 300}
	if res := d.doubleTapOn(&flow.DoubleTapOnStep{Selector: flow.Selector{ID: "dot"}, BaseStep: base}); res.Success {
		t.Error("doubleTapOn succeeded on a 0x0 element")
	}
	if res := d.longPressOn(&flow.LongPressOnStep{Selector: flow.Selector{ID: "dot"}, BaseStep: base}); res.Success {
		t.Error("longPressOn succeeded on a 0x0 element")
	}
	if n := atomic.LoadInt32(actions); n != 0 {
		t.Errorf("sent %d actions, want none", n)
	}
}
