package core

import "testing"

func TestVisibleFraction(t *testing.T) {
	screenW, screenH := 1000, 2000
	cases := []struct {
		name string
		b    Bounds
		want float64
	}{
		{"fully visible", Bounds{X: 100, Y: 100, Width: 200, Height: 200}, 1.0},
		{"half below the fold", Bounds{X: 0, Y: 1900, Width: 100, Height: 200}, 0.5},
		{"sliver peeking over the fold", Bounds{X: 0, Y: 1990, Width: 100, Height: 200}, 0.05},
		{"fully off screen", Bounds{X: 0, Y: 2000, Width: 100, Height: 200}, 0},
		{"negative origin, half on", Bounds{X: -100, Y: 0, Width: 200, Height: 100}, 0.5},
		{"zero size", Bounds{X: 10, Y: 10, Width: 0, Height: 0}, 0},
	}
	for _, c := range cases {
		got := VisibleFraction(c.b, screenW, screenH)
		if got < c.want-0.001 || got > c.want+0.001 {
			t.Errorf("%s: VisibleFraction = %v, want %v", c.name, got, c.want)
		}
	}

	if VisibleFraction(Bounds{X: 0, Y: 0, Width: 10, Height: 10}, 0, 0) != 0 {
		t.Error("unknown screen size must report 0, not visible")
	}
}

func TestMeetsVisibility(t *testing.T) {
	screenW, screenH := 1000, 2000
	half := Bounds{X: 0, Y: 1900, Width: 100, Height: 200} // 50% visible
	full := Bounds{X: 0, Y: 0, Width: 100, Height: 100}

	if MeetsVisibility(half, screenW, screenH, 0) {
		t.Error("unset percentage must default to fully-visible and reject 50%")
	}
	if !MeetsVisibility(full, screenW, screenH, 0) {
		t.Error("fully visible element must pass the default requirement")
	}
	if !MeetsVisibility(half, screenW, screenH, 50) {
		t.Error("50% visible must satisfy visibilityPercentage: 50")
	}
	if MeetsVisibility(half, screenW, screenH, 51) {
		t.Error("50% visible must not satisfy visibilityPercentage: 51")
	}
	if MeetsVisibility(half, screenW, screenH, 150) {
		t.Error("out-of-range percentage must fall back to the fully-visible default")
	}
}

func TestClippedAtScrollEdge(t *testing.T) {
	// The container from the report: a ScrollView whose visible area ends at
	// the bottom tab bar (y 2083).
	container := Bounds{X: 42, Y: 300, Width: 996, Height: 1783} // 300..2083

	tests := []struct {
		name      string
		b         Bounds
		direction string
		want      bool
	}{
		// The reported sliver: 9px tall, ending exactly on the container edge.
		{"sliver at the fold, scrolling down", Bounds{X: 42, Y: 2074, Width: 996, Height: 9}, "down", true},
		// Same button once the list has settled — clear of the edge.
		{"settled element clear of the edge", Bounds{X: 42, Y: 1915, Width: 996, Height: 126}, "down", false},
		// The first visible row is flush with the container TOP. Scrolling
		// down, that is the trailing edge: treating it as clipped would spend
		// a scroll dragging a fully visible element away, possibly off screen.
		{"flush with the trailing edge is not suspicious", Bounds{X: 42, Y: 300, Width: 996, Height: 126}, "down", false},
		// Scrolling up, the leading edge is the top, so the same rect is.
		{"flush with the top, scrolling up", Bounds{X: 42, Y: 300, Width: 996, Height: 126}, "up", true},
		// Full-width rows share the container's left and right edges. Those
		// must never register on a vertical scroll or almost every row would.
		{"full-width row on a vertical scroll", Bounds{X: 42, Y: 800, Width: 996, Height: 126}, "down", false},
		{"flush right, scrolling right", Bounds{X: 800, Y: 800, Width: 238, Height: 126}, "right", true},
		{"flush left, scrolling left", Bounds{X: 42, Y: 800, Width: 200, Height: 126}, "left", true},
		// A pixel of rounding still counts; a real gap does not.
		{"one pixel short still counts", Bounds{X: 42, Y: 2074, Width: 996, Height: 8}, "down", true},
		{"ten pixels short is a real gap", Bounds{X: 42, Y: 2000, Width: 996, Height: 73}, "down", false},
		{"zero-area element", Bounds{X: 42, Y: 2083, Width: 996, Height: 0}, "down", false},
		{"unknown direction", Bounds{X: 42, Y: 2074, Width: 996, Height: 9}, "sideways", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClippedAtScrollEdge(tt.b, container, tt.direction, 2); got != tt.want {
				t.Errorf("ClippedAtScrollEdge(%+v, %q) = %v, want %v", tt.b, tt.direction, got, tt.want)
			}
		})
	}
}

// TestVisibleFractionCannotSeeContainerClipping documents why the helper above
// has to exist: the arithmetic is handed a pre-clipped rect and cannot tell.
func TestVisibleFractionCannotSeeContainerClipping(t *testing.T) {
	sliver := Bounds{X: 42, Y: 2074, Width: 996, Height: 9}
	if got := VisibleFraction(sliver, 1080, 2400); got != 1 {
		t.Fatalf("expected the clipped sliver to still score 1.0, got %v", got)
	}
	if !MeetsVisibility(sliver, 1080, 2400, 100) {
		t.Error("expected MeetsVisibility to accept it — this is the gap ClippedAtScrollEdge covers")
	}
}
