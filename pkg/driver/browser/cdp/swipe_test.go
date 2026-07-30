package cdp

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// TestViewportSwipeCoords locks in the full-viewport drag segment: the
// central 40% band around the viewport center.
func TestViewportSwipeCoords(t *testing.T) {
	cx, cy := 500.0, 400.0 // 1000x800 viewport
	cases := []struct {
		dir            string
		sx, sy, ex, ey float64
	}{
		{"up", 500, 560, 500, 240},
		{"down", 500, 240, 500, 560},
		{"left", 700, 400, 300, 400},
		{"right", 300, 400, 700, 400},
	}
	for _, c := range cases {
		t.Run(c.dir, func(t *testing.T) {
			sx, sy, ex, ey := viewportSwipeCoords(c.dir, cx, cy)
			if sx != c.sx || sy != c.sy || ex != c.ex || ey != c.ey {
				t.Errorf("viewportSwipeCoords(%q) = (%v,%v)->(%v,%v), want (%v,%v)->(%v,%v)",
					c.dir, sx, sy, ex, ey, c.sx, c.sy, c.ex, c.ey)
			}
		})
	}
}

// TestElementSwipeCoords locks in the element-anchored drag segment: 90% →
// 10% along the swipe axis inside the element's box (no overshoot — web drag
// handlers without pointer capture stop tracking outside their hit area).
func TestElementSwipeCoords(t *testing.T) {
	b := core.Bounds{X: 100, Y: 200, Width: 300, Height: 80}
	cases := []struct {
		dir            string
		sx, sy, ex, ey float64
	}{
		{"up", 250, 272, 250, 208},
		{"down", 250, 208, 250, 272},
		{"left", 370, 240, 130, 240},
		{"right", 130, 240, 370, 240},
	}
	for _, c := range cases {
		t.Run(c.dir, func(t *testing.T) {
			sx, sy, ex, ey := elementSwipeCoords(c.dir, b)
			if sx != c.sx || sy != c.sy || ex != c.ex || ey != c.ey {
				t.Errorf("elementSwipeCoords(%q) = (%v,%v)->(%v,%v), want (%v,%v)->(%v,%v)",
					c.dir, sx, sy, ex, ey, c.sx, c.sy, c.ex, c.ey)
			}
		})
	}
}
