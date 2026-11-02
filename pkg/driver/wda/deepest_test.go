package wda

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// A search field and its magnifier icon share an id side by side; the icon
// comes first in tree order but takes no input (#259).
func TestDeepestOfPrefersTheControl(t *testing.T) {
	icon := &core.ElementInfo{Class: "XCUIElementTypeImage", Bounds: core.Bounds{X: 34, Y: 147, Width: 17, Height: 18}}
	field := &core.ElementInfo{Class: "XCUIElementTypeTextField", Bounds: core.Bounds{X: 60, Y: 144, Width: 310, Height: 23}}
	if got := deepestOf([]*core.ElementInfo{icon, field}); got != field {
		t.Errorf("picked %s, want the text field", got.Class)
	}
}

// With no control among them, the first innermost element in tree order.
func TestDeepestOfKeepsTreeOrderWithoutControls(t *testing.T) {
	a := &core.ElementInfo{Class: "XCUIElementTypeImage", Bounds: core.Bounds{X: 0, Y: 0, Width: 10, Height: 10}}
	b := &core.ElementInfo{Class: "XCUIElementTypeStaticText", Bounds: core.Bounds{X: 20, Y: 0, Width: 10, Height: 10}}
	if got := deepestOf([]*core.ElementInfo{a, b}); got != a {
		t.Errorf("picked %s, want the first", got.Class)
	}
}

// A control that holds another match is still skipped for the one inside it.
func TestDeepestOfSkipsAnOuterControl(t *testing.T) {
	row := &core.ElementInfo{Class: "XCUIElementTypeButton", Bounds: core.Bounds{X: 0, Y: 0, Width: 400, Height: 44}}
	inner := &core.ElementInfo{Class: "XCUIElementTypeSwitch", Bounds: core.Bounds{X: 340, Y: 6, Width: 51, Height: 31}}
	if got := deepestOf([]*core.ElementInfo{row, inner}); got != inner {
		t.Errorf("picked %s, want the switch inside the row", got.Class)
	}
}
