package core

import "testing"

// TestSwipeCoordsInBounds locks in the element-anchored swipe coordinates
// (start inside the element, end past the opposite edge, clamped to the
// screen). These feed the drivers' absolute-coordinate swipe paths so a
// from:/selector swipe honours duration: (#114).
func TestSwipeCoordsInBounds(t *testing.T) {
	b := Bounds{X: 0, Y: 100, Width: 500, Height: 800}
	screenW, screenH := 1080, 2400
	cases := []struct {
		dir            string
		sx, sy, ex, ey int
	}{
		{"left", 450, 500, 0, 500},   // end clamps to 0 (element flush against left edge)
		{"right", 50, 500, 550, 500}, // end 10% past right edge
		{"up", 250, 820, 250, 20},    // end 10% above top (100 - 80)
		{"down", 250, 180, 250, 980}, // end 10% below bottom
	}
	for _, c := range cases {
		t.Run(c.dir, func(t *testing.T) {
			sx, sy, ex, ey, err := SwipeCoordsInBounds(c.dir, b, screenW, screenH)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sx != c.sx || sy != c.sy || ex != c.ex || ey != c.ey {
				t.Errorf("SwipeCoordsInBounds(%q) = (%d,%d)->(%d,%d), want (%d,%d)->(%d,%d)",
					c.dir, sx, sy, ex, ey, c.sx, c.sy, c.ex, c.ey)
			}
		})
	}
}

// TestSwipeCoordsInBounds_ClampsToScreen verifies overshoot past the far
// screen edge lands on the last on-screen pixel for elements flush against
// the right/bottom edges.
func TestSwipeCoordsInBounds_ClampsToScreen(t *testing.T) {
	screenW, screenH := 1000, 2000
	// Element flush against the right and bottom screen edges.
	b := Bounds{X: 500, Y: 1200, Width: 500, Height: 800}

	_, _, ex, _, err := SwipeCoordsInBounds("right", b, screenW, screenH)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ex != screenW-1 {
		t.Errorf("right swipe end X = %d, want clamp to %d", ex, screenW-1)
	}

	_, _, _, ey, err := SwipeCoordsInBounds("down", b, screenW, screenH)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ey != screenH-1 {
		t.Errorf("down swipe end Y = %d, want clamp to %d", ey, screenH-1)
	}
}

// TestSwipeCoordsInBounds_UnknownScreen verifies that unknown screen dims
// (0,0) skip the far-edge clamp but still clamp negatives to 0.
func TestSwipeCoordsInBounds_UnknownScreen(t *testing.T) {
	b := Bounds{X: 0, Y: 100, Width: 500, Height: 800}
	sx, sy, ex, ey, err := SwipeCoordsInBounds("right", b, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sx != 50 || sy != 500 || ex != 550 || ey != 500 {
		t.Errorf("got (%d,%d)->(%d,%d), want (50,500)->(550,500)", sx, sy, ex, ey)
	}

	_, _, ex, _, err = SwipeCoordsInBounds("left", b, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ex != 0 {
		t.Errorf("left swipe end X = %d, want negative overshoot clamped to 0", ex)
	}
}

func TestNormalizeSwipeDirection(t *testing.T) {
	good := map[string]string{"": "up", "up": "up", "DOWN": "down", "Left": "left", "right": "right"}
	for in, want := range good {
		got, err := NormalizeSwipeDirection(in)
		if err != nil || got != want {
			t.Errorf("NormalizeSwipeDirection(%q) = (%q, %v), want (%q, nil)", in, got, err, want)
		}
	}
	for _, in := range []string{"diagonal", "upp", "north"} {
		if _, err := NormalizeSwipeDirection(in); err == nil {
			t.Errorf("NormalizeSwipeDirection(%q) expected error, got nil", in)
		}
	}
}

func TestSwipeCoordsInBounds_InvalidDirection(t *testing.T) {
	for _, dir := range []string{"", "diagonal", "UP"} {
		if _, _, _, _, err := SwipeCoordsInBounds(dir, Bounds{Width: 10, Height: 10}, 100, 100); err == nil {
			t.Errorf("SwipeCoordsInBounds(%q) expected error, got nil", dir)
		}
	}
}
