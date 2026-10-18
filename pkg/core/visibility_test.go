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

func TestVisibleFractionWithin(t *testing.T) {
	area := Bounds{X: 100, Y: 200, Width: 400, Height: 600} // 100..500 x 200..800
	tests := []struct {
		name    string
		b, area Bounds
		want    float64
	}{
		{"inside", Bounds{X: 150, Y: 300, Width: 100, Height: 100}, area, 1},
		{"half past the bottom", Bounds{X: 150, Y: 750, Width: 100, Height: 100}, area, 0.5},
		{"a quarter past the left", Bounds{X: 75, Y: 300, Width: 100, Height: 100}, area, 0.75},
		{"outside", Bounds{X: 600, Y: 300, Width: 100, Height: 100}, area, 0},
		{"zero-size element", Bounds{X: 150, Y: 300}, area, 0},
		{"zero-size area", Bounds{X: 150, Y: 300, Width: 100, Height: 100}, Bounds{X: 100, Y: 200}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VisibleFractionWithin(tt.b, tt.area); got != tt.want {
				t.Errorf("VisibleFractionWithin(%+v, %+v) = %v, want %v", tt.b, tt.area, got, tt.want)
			}
		})
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

func TestHangsPastScrollEdge(t *testing.T) {
	// A bottom sheet's list, ending above a 2400px screen's gesture bar.
	list := Bounds{X: 63, Y: 63, Width: 954, Height: 2274} // 63..2337
	row := func(top, bottom int) Bounds { return Bounds{X: 63, Y: top, Width: 954, Height: bottom - top} }
	tests := []struct {
		name       string
		b, list    Bounds
		direction  string
		percentage int
		want       bool
	}{
		// The reported row: 42 of its 105px inside the list.
		{"past the bottom, scrolling down", row(2295, 2400), list, "down", 0, true},
		{"inside the list", row(1959, 2274), list, "down", 0, false},
		{"2px past is within tolerance", row(2239, 2339), list, "down", 0, false},
		{"3px past is not", row(2240, 2340), list, "down", 0, true},
		{"40% inside meets visibilityPercentage: 40", row(2295, 2400), list, "down", 40, false},
		// Scrolling up can't bring in a row past the bottom; it would move it out.
		{"past the bottom, scrolling up", row(2295, 2400), list, "up", 0, false},
		{"past the top, scrolling up", row(20, 125), list, "up", 0, true},
		{"past the top, scrolling down", row(20, 125), list, "down", 0, false},
		// A vertical scroll can't fix a sideways overhang.
		{"past the right, scrolling down", Bounds{X: 900, Y: 500, Width: 200, Height: 100}, list, "down", 0, false},
		{"past the right, scrolling right", Bounds{X: 900, Y: 500, Width: 200, Height: 100}, list, "right", 0, true},
		{"past the left, scrolling left", Bounds{X: 20, Y: 500, Width: 200, Height: 100}, list, "left", 0, true},
		{"taller than its list", row(30, 2400), list, "down", 0, false},
		{"zero-size list", row(2295, 2400), Bounds{X: 63, Y: 63}, "down", 0, false},
		{"wholly past the bottom, on screen", row(2345, 2390), list, "down", 0, true},
		{"beside its list", Bounds{X: 1020, Y: 2295, Width: 50, Height: 105}, list, "down", 0, false},
		{"no direction", row(2295, 2400), list, "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HangsPastScrollEdge(tt.b, tt.list, tt.direction, tt.percentage, 2); got != tt.want {
				t.Errorf("HangsPastScrollEdge(%+v, %+v, %q) = %v, want %v", tt.b, tt.list, tt.direction, got, tt.want)
			}
		})
	}
}

func TestScrollSpeedToDurationMs(t *testing.T) {
	// Values taken from upstream's speedToDuration so a flow written against
	// Maestro scrolls the same distance per swipe here.
	tests := []struct{ speed, want int }{
		{1, 991},  // the value in the bug report — a deliberate, slow drag
		{40, 601}, // Maestro's default
		{50, 501},
		{95, 51},  // last speed above the floor
		{96, 50},  // upstream says 41ms; floored — see MinSwipeDurationMs
		{100, 50}, // upstream says 1ms, which Android reads as a tap
		{0, 0},    // absent — caller keeps its own default, see ScrollDurationOrDefault
		{-5, 601}, // out of range → default, never an error
		{250, 601},
	}
	for _, tt := range tests {
		if got := ScrollSpeedToDurationMs(tt.speed); got != tt.want {
			t.Errorf("ScrollSpeedToDurationMs(%d) = %d, want %d", tt.speed, got, tt.want)
		}
	}
}

// A flow that never mentions speed must keep the driver's existing duration.
// Adopting upstream's 601ms default would change fling velocity, and with it
// how far every existing scroll travels.
func TestScrollDurationOrDefault(t *testing.T) {
	if got := ScrollDurationOrDefault(0, 300); got != 300 {
		t.Errorf("absent speed should keep the driver default 300, got %d", got)
	}
	if got := ScrollDurationOrDefault(0, 500); got != 500 {
		t.Errorf("absent speed should keep the driver default 500, got %d", got)
	}
	if got := ScrollDurationOrDefault(1, 300); got != 991 {
		t.Errorf("explicit speed 1 should win over the default, got %d", got)
	}
	if got := ScrollDurationOrDefault(40, 300); got != 601 {
		t.Errorf("explicit speed 40 should be upstream's 601, got %d", got)
	}
}

func TestNearScreenCenter(t *testing.T) {
	// DDG's browsing menu: "Passwords" at the bottom edge of an 852pt screen.
	bottom := Bounds{X: 84, Y: 823, Width: 82, Height: 20}
	middle := Bounds{X: 84, Y: 335, Width: 82, Height: 20}
	top := Bounds{X: 84, Y: 20, Width: 82, Height: 20}
	for _, c := range []struct {
		name string
		b    Bounds
		dir  string
		want bool
	}{
		{"down, at the bottom", bottom, "down", false},
		{"down, near the middle", middle, "DOWN", true},
		{"down is the default", bottom, "", false},
		{"up, at the top", top, "up", false},
		{"up, near the middle", middle, "up", true},
		{"right, far left", Bounds{X: 0, Y: 400, Width: 20, Height: 20}, "right", true},
		{"right, far right", Bounds{X: 370, Y: 400, Width: 20, Height: 20}, "right", false},
		{"left, far left", Bounds{X: 0, Y: 400, Width: 20, Height: 20}, "left", false},
		{"left, far right", Bounds{X: 370, Y: 400, Width: 20, Height: 20}, "left", true},
	} {
		if got := NearScreenCenter(c.b, 393, 852, c.dir); got != c.want {
			t.Errorf("%s: NearScreenCenter = %v, want %v", c.name, got, c.want)
		}
	}
}
