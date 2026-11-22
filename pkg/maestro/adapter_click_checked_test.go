package maestro

import (
	"encoding/json"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// ClickChecked sends the target's bounds as edges with the tap and returns
// the window the agent names in coveredBy.
func TestAdapterClickCheckedSendsTargetAndReadsCoveredBy(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		if req.Method != "Gesture.click" {
			t.Errorf("expected Gesture.click, got %s", req.Method)
		}
		raw, _ := json.Marshal(req.Params)
		var p map[string]float64
		_ = json.Unmarshal(raw, &p)
		want := map[string]float64{"x": 150, "y": 220, "targetLeft": 100, "targetTop": 200, "targetRight": 300, "targetBottom": 240}
		for k, v := range want {
			if p[k] != v {
				t.Errorf("%s = %v, want %v", k, p[k], v)
			}
		}
		return map[string]interface{}{"coveredBy": "keyboard window [0,1500][1080,2400] layer 2"}
	})
	defer cleanup()

	got, err := adapter.ClickChecked(150, 220, core.Bounds{X: 100, Y: 200, Width: 200, Height: 40})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "keyboard window [0,1500][1080,2400] layer 2" {
		t.Errorf("coveredBy = %q", got)
	}
}

// An agent that does not check (older build) answers an empty result: no
// window, no error.
func TestAdapterClickCheckedOlderAgent(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		return map[string]interface{}{}
	})
	defer cleanup()

	got, err := adapter.ClickChecked(1, 2, core.Bounds{Width: 10, Height: 10})
	if err != nil || got != "" {
		t.Errorf("got %q, %v; want empty, nil", got, err)
	}
}

// DoubleClickChecked and LongClickChecked send the same bounds on their own
// gestures, the long press with its duration.
func TestAdapterDoubleAndLongClickChecked(t *testing.T) {
	adapter, cleanup := adapterWithMock(t, func(req Request) interface{} {
		raw, _ := json.Marshal(req.Params)
		var p map[string]float64
		_ = json.Unmarshal(raw, &p)
		if p["targetRight"] != 110 || p["targetBottom"] != 220 {
			t.Errorf("%s: target edges %v", req.Method, p)
		}
		if req.Method == "Gesture.longClick" && p["durationMs"] != 800 {
			t.Errorf("durationMs = %v, want 800", p["durationMs"])
		}
		return map[string]interface{}{"coveredBy": req.Method}
	})
	defer cleanup()

	target := core.Bounds{X: 10, Y: 20, Width: 100, Height: 200}
	if got, err := adapter.DoubleClickChecked(50, 60, target); err != nil || got != "Gesture.doubleClick" {
		t.Errorf("DoubleClickChecked = %q, %v", got, err)
	}
	if got, err := adapter.LongClickChecked(50, 60, 800, target); err != nil || got != "Gesture.longClick" {
		t.Errorf("LongClickChecked = %q, %v", got, err)
	}
}
