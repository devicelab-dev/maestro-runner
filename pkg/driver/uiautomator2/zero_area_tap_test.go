package uiautomator2

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// zeroAreaDriver serves one element, "dot", whose rect is 0x0 at (100,200)
// for the first zeroReads reads and 200x100 there after. It counts every tap
// that reaches the server: element clicks and coordinate gestures.
func zeroAreaDriver(t *testing.T, zeroReads int32) (*Driver, *int32) {
	t.Helper()
	var rectReads, taps int32
	tap := func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&taps, 1)
		writeJSON(w, map[string]interface{}{"value": nil})
	}
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": map[string]string{"ELEMENT": "dot"}})
		},
		"GET /element/dot/text": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": "Dot"})
		},
		"GET /element/dot/rect": func(w http.ResponseWriter, r *http.Request) {
			size := 0
			if atomic.AddInt32(&rectReads, 1) > zeroReads {
				size = 100
			}
			writeJSON(w, map[string]interface{}{"value": map[string]int{"x": 100, "y": 200, "width": size * 2, "height": size}})
		},
		// The page-source fallback sees the same 0x0 node, and drops it.
		"GET /source": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": `<hierarchy><node class="android.widget.Button" text="Dot" resource-id="com.app:id/dot" clickable="true" enabled="true" displayed="true" bounds="[100,200][100,200]"/></hierarchy>`})
		},
		"POST /element/dot/click":                 tap,
		"POST /appium/gestures/click":             tap,
		"POST /appium/gestures/double_click":      tap,
		"POST /appium/gestures/long_click":        tap,
		"POST /actions":                           tap,
		"GET /element/dot/attribute/displayed":    func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]interface{}{"value": "true"}) },
		"GET /element/dot/attribute/enabled":      func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]interface{}{"value": "true"}) },
		"GET /element/dot/attribute/clickable":    func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]interface{}{"value": "true"}) },
		"GET /element/dot/attribute/content-desc": func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]interface{}{"value": ""}) },
	})
	t.Cleanup(server.Close)
	return New(newMockHTTPClient(server.URL).Client, nil, nil), &taps
}

// UiAutomator taps the centre of an element's visible bounds, which for a 0x0
// node is its origin: a point on whatever else is drawn there. The tap is
// refused instead.
func TestTapOnRefusesAZeroAreaElement(t *testing.T) {
	for _, sel := range []flow.Selector{{ID: "dot"}, {Text: "Dot"}} {
		d, taps := zeroAreaDriver(t, 1<<30)
		res := d.tapOn(&flow.TapOnStep{Selector: sel, BaseStep: flow.BaseStep{TimeoutMs: 300}})
		if res.Success {
			t.Errorf("%s: tapOn succeeded on a 0x0 element", sel.Describe())
		}
		if n := atomic.LoadInt32(taps); n != 0 {
			t.Errorf("%s: sent %d taps, want none", sel.Describe(), n)
		}
	}
}

func TestTapOnZeroAreaErrorSaysWhy(t *testing.T) {
	d, _ := zeroAreaDriver(t, 1<<30)
	res := d.tapOn(&flow.TapOnStep{Selector: flow.Selector{ID: "dot"}, BaseStep: flow.BaseStep{TimeoutMs: 300}})
	if res.Error == nil || !strings.Contains(res.Error.Error(), "no area") {
		t.Errorf("error = %v, want it to say the element has no area", res.Error)
	}
}

// The lookup keeps polling, so an element laid out a moment later is tapped.
func TestTapOnWaitsForAZeroAreaElementToBeLaidOut(t *testing.T) {
	d, taps := zeroAreaDriver(t, 1)
	res := d.tapOn(&flow.TapOnStep{Selector: flow.Selector{ID: "dot"}, BaseStep: flow.BaseStep{TimeoutMs: 2000}})
	if !res.Success {
		t.Fatalf("tapOn failed: %s", res.Message)
	}
	if n := atomic.LoadInt32(taps); n != 1 {
		t.Errorf("sent %d taps, want 1", n)
	}
	if res.Element == nil || res.Element.Bounds.Width != 200 {
		t.Errorf("tapped %+v, want the laid-out 200x100 element", res.Element)
	}
}

func TestDoubleTapAndLongPressRefuseAZeroAreaElement(t *testing.T) {
	d, taps := zeroAreaDriver(t, 1<<30)
	base := flow.BaseStep{TimeoutMs: 300}
	if res := d.doubleTapOn(&flow.DoubleTapOnStep{Selector: flow.Selector{ID: "dot"}, BaseStep: base}); res.Success {
		t.Error("doubleTapOn succeeded on a 0x0 element")
	}
	if res := d.longPressOn(&flow.LongPressOnStep{Selector: flow.Selector{ID: "dot"}, BaseStep: base}); res.Success {
		t.Error("longPressOn succeeded on a 0x0 element")
	}
	if n := atomic.LoadInt32(taps); n != 0 {
		t.Errorf("sent %d taps, want none", n)
	}
}

// An element whose rect cannot be read is not refused: zero bounds from a
// failed read say nothing about its size.
func TestTapOnDoesNotRefuseAnUnreadRect(t *testing.T) {
	var taps int32
	server := setupMockServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"POST /element": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, map[string]interface{}{"value": map[string]string{"ELEMENT": "dot"}})
		},
		"GET /element/dot/rect": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": map[string]string{"error": "unknown error", "message": "boom"}})
		},
		"POST /element/dot/click": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&taps, 1)
			writeJSON(w, map[string]interface{}{"value": nil})
		},
	})
	defer server.Close()
	d := New(newMockHTTPClient(server.URL).Client, nil, nil)
	res := d.tapOn(&flow.TapOnStep{Selector: flow.Selector{ID: "dot"}, BaseStep: flow.BaseStep{TimeoutMs: 300}})
	if !res.Success || atomic.LoadInt32(&taps) != 1 {
		t.Errorf("success=%v taps=%d, want the element clicked once", res.Success, taps)
	}
}
