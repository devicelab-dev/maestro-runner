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
