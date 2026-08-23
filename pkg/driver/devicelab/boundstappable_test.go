package devicelab

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// boundsTappable guards the tap path against injecting a lost off-screen tap
// (issue #94). Screen is 1080x2400 (the device in the bug report).
func TestBoundsTappable(t *testing.T) {
	const sw, sh = 1080, 2400
	cases := []struct {
		name string
		b    core.Bounds
		want bool
	}{
		{"settled on-screen rect", core.Bounds{X: 63, Y: 1033, Width: 954, Height: 158}, true},
		// The exact malformed first-frame rect from #94: [63,2836][1017,2400],
		// top>bottom → height -436, centre off a 2400px screen.
		{"issue #94 malformed rect", core.Bounds{X: 63, Y: 2836, Width: 954, Height: -436}, false},
		{"negative width", core.Bounds{X: 0, Y: 0, Width: -10, Height: 100}, false},
		{"zero size", core.Bounds{X: 100, Y: 100, Width: 0, Height: 0}, false},
		{"centre below screen", core.Bounds{X: 0, Y: 2350, Width: 100, Height: 200}, false},
		{"centre right of screen", core.Bounds{X: 2000, Y: 100, Width: 100, Height: 100}, false},
		{"top-left corner", core.Bounds{X: 0, Y: 0, Width: 2, Height: 2}, true},
		{"centre exactly at right edge", core.Bounds{X: 1078, Y: 100, Width: 4, Height: 100}, false}, // cx=1080 == sw
		{"full-width bottom button", core.Bounds{X: 0, Y: 2200, Width: 1080, Height: 150}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := boundsTappable(c.b, sw, sh); got != c.want {
				t.Errorf("boundsTappable(%+v) = %v, want %v", c.b, got, c.want)
			}
		})
	}
}
