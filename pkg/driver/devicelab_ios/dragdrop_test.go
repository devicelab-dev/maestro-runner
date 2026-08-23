package devicelab_ios

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// gestureServer serves a fake runner that records every drag command's wire
// payload and answers all other commands with the nodes from nodesFn, which
// receives the number of drags performed so far — letting a test move an
// element "after" each scroll.
type gestureServer struct {
	mu    sync.Mutex
	drags []map[string]any
}

func (g *gestureServer) dragCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.drags)
}

func (g *gestureServer) lastDrag(t *testing.T) map[string]any {
	t.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.drags) == 0 {
		t.Fatal("no drag command reached the runner")
	}
	return g.drags[len(g.drags)-1]
}

func newGestureDriver(t *testing.T, info *core.PlatformInfo, nodesFn func(dragsSoFar int) []SnapshotNode) (*Driver, *gestureServer) {
	t.Helper()
	g := &gestureServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cmd map[string]any
		if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
			t.Errorf("decode command: %v", err)
		}

		g.mu.Lock()
		if cmd["command"] == "drag" {
			g.drags = append(g.drags, cmd)
			g.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"data": map[string]any{"message": "dragged"},
			})
			return
		}
		drags := len(g.drags)
		g.mu.Unlock()

		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"data": map[string]any{"nodes": nodesFn(drags)},
		})
	}))
	t.Cleanup(srv.Close)

	client := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	return NewDriver(client, info, "test-udid", nil), g
}

func TestDragAndDrop_SelectorToPoint(t *testing.T) {
	info := &core.PlatformInfo{ScreenWidth: 400, ScreenHeight: 800}
	d, g := newGestureDriver(t, info, func(int) []SnapshotNode {
		return []SnapshotNode{{
			Type:       "Cell",
			Identifier: "item-3",
			Rect:       SnapshotRect{X: 40, Y: 100, Width: 80, Height: 40},
		}}
	})

	res := d.handleDragAndDrop(&flow.DragAndDropStep{
		From:         flow.Selector{ID: "item-3"},
		To:           flow.Selector{Point: "50%, 25%"},
		HoldDuration: 800,
		Duration:     1200,
	})
	if !res.Success {
		t.Fatalf("dragAndDrop failed: %s", res.Message)
	}

	drag := g.lastDrag(t)
	want := map[string]float64{
		"x": 80, "y": 120, // center of the matched element
		"x2": 200, "y2": 200, // 50% × 400, 25% × 800
		"durationMs":     800,
		"moveDurationMs": 1200,
	}
	for k, v := range want {
		if got, _ := drag[k].(float64); got != v {
			t.Errorf("%s = %v, want %v", k, drag[k], v)
		}
	}
}

func TestDragAndDrop_PointOnlyEndpoints(t *testing.T) {
	info := &core.PlatformInfo{ScreenWidth: 400, ScreenHeight: 800}
	d, g := newGestureDriver(t, info, func(int) []SnapshotNode { return nil })

	res := d.handleDragAndDrop(&flow.DragAndDropStep{
		From:         flow.Selector{Point: "10%, 50%"},
		To:           flow.Selector{Point: "300, 100"},
		HoldDuration: 1000,
		Duration:     1000,
	})
	if !res.Success {
		t.Fatalf("dragAndDrop failed: %s", res.Message)
	}

	drag := g.lastDrag(t)
	for k, v := range map[string]float64{"x": 40, "y": 400, "x2": 300, "y2": 100} {
		if got, _ := drag[k].(float64); got != v {
			t.Errorf("%s = %v, want %v", k, drag[k], v)
		}
	}
}

func TestDragAndDrop_MissingSourceFails(t *testing.T) {
	info := &core.PlatformInfo{ScreenWidth: 400, ScreenHeight: 800}
	d, g := newGestureDriver(t, info, func(int) []SnapshotNode { return nil })

	res := d.handleDragAndDrop(&flow.DragAndDropStep{
		BaseStep: flow.BaseStep{TimeoutMs: 200},
		From:     flow.Selector{ID: "nope"},
		To:       flow.Selector{Point: "50%, 50%"},
	})
	if res.Success {
		t.Fatal("expected failure when the source element is missing")
	}
	if g.dragCount() != 0 {
		t.Error("no drag should be attempted when resolution fails")
	}
}

func TestScrollUntilVisible_KeepsScrollingPastPartialVisibility(t *testing.T) {
	info := &core.PlatformInfo{ScreenWidth: 400, ScreenHeight: 800}
	// Before any scroll the target is half below the fold; after one scroll
	// it is fully on screen.
	d, _ := newGestureDriver(t, info, func(drags int) []SnapshotNode {
		y := 780.0 // 50% visible on an 800pt screen
		if drags >= 1 {
			y = 600
		}
		return []SnapshotNode{{
			Type:       "Cell",
			Identifier: "target",
			Rect:       SnapshotRect{X: 0, Y: y, Width: 100, Height: 40},
		}}
	})

	res := d.executeStep(&flow.ScrollUntilVisibleStep{
		BaseStep: flow.BaseStep{StepType: flow.StepScrollUntilVisible},
		Element:  flow.Selector{ID: "target"},
	})
	if !res.Success {
		t.Fatalf("scrollUntilVisible failed: %s", res.Message)
	}
	if res.Element == nil || res.Element.Bounds.Y != 600 {
		t.Errorf("expected the fully-visible position, got %+v", res.Element)
	}
}

func TestScrollUntilVisible_HonorsVisibilityPercentage(t *testing.T) {
	info := &core.PlatformInfo{ScreenWidth: 400, ScreenHeight: 800}
	d, g := newGestureDriver(t, info, func(int) []SnapshotNode {
		return []SnapshotNode{{
			Type:       "Cell",
			Identifier: "target",
			Rect:       SnapshotRect{X: 0, Y: 780, Width: 100, Height: 40}, // 50% visible
		}}
	})

	res := d.executeStep(&flow.ScrollUntilVisibleStep{
		BaseStep:             flow.BaseStep{StepType: flow.StepScrollUntilVisible},
		Element:              flow.Selector{ID: "target"},
		VisibilityPercentage: 50,
	})
	if !res.Success {
		t.Fatalf("50%% visible must satisfy visibilityPercentage: 50, got: %s", res.Message)
	}
	if g.dragCount() != 0 {
		t.Errorf("no scroll should be needed, performed %d", g.dragCount())
	}
}

func TestScrollUntilVisible_UnknownScreenKeepsOldBehavior(t *testing.T) {
	// No PlatformInfo → screen size unknown → the visibility gate must not
	// turn every match into an endless scroll.
	d, g := newGestureDriver(t, nil, func(int) []SnapshotNode {
		return []SnapshotNode{{
			Type:       "Cell",
			Identifier: "target",
			Rect:       SnapshotRect{X: 0, Y: 780, Width: 100, Height: 40},
		}}
	})

	res := d.executeStep(&flow.ScrollUntilVisibleStep{
		BaseStep: flow.BaseStep{StepType: flow.StepScrollUntilVisible},
		Element:  flow.Selector{ID: "target"},
	})
	if !res.Success {
		t.Fatalf("expected old accept-on-find behavior, got: %s", res.Message)
	}
	if g.dragCount() != 0 {
		t.Errorf("no scroll should happen, performed %d", g.dragCount())
	}
}
