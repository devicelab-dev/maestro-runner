package cdp

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// resolveDragEnd's point path never touches the page, so a bare Driver with a
// viewport is enough.

func TestResolveDragEnd_Point(t *testing.T) {
	d := &Driver{viewportW: 1280, viewportH: 720}

	x, y, info, err := d.resolveDragEnd(flow.Selector{Point: "50%, 50%"}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if x != 640 || y != 360 {
		t.Errorf("point = (%v, %v), want (640, 360)", x, y)
	}
	if info != nil {
		t.Error("a bare point resolves without an element")
	}

	x, y, _, err = d.resolveDragEnd(flow.Selector{Point: "100, 200"}, 1000)
	if err != nil || x != 100 || y != 200 {
		t.Errorf("absolute point = (%v, %v), err %v; want (100, 200)", x, y, err)
	}
}

func TestResolveDragEnd_InvalidPoint(t *testing.T) {
	d := &Driver{viewportW: 1280, viewportH: 720}

	if _, _, _, err := d.resolveDragEnd(flow.Selector{Point: "5000, 200"}, 1000); err == nil {
		t.Error("expected an error for a point outside the viewport")
	}
	if _, _, _, err := d.resolveDragEnd(flow.Selector{Point: "nonsense"}, 1000); err == nil {
		t.Error("expected an error for an unparseable point")
	}
}

func TestDragAndDrop_InvalidPointFailsBeforePageAccess(t *testing.T) {
	d := &Driver{viewportW: 1280, viewportH: 720}
	step := &flow.DragAndDropStep{
		From:         flow.Selector{Point: "150%, 50%"},
		To:           flow.Selector{Point: "50%, 10%"},
		HoldDuration: 100,
		Duration:     100,
	}
	result := d.dragAndDrop(step)
	if result.Success {
		t.Fatal("expected failure for an out-of-range source point")
	}
	if !strings.Contains(result.Message, "from") {
		t.Errorf("message should say which end failed, got %q", result.Message)
	}
}
