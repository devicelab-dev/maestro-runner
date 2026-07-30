package core

import (
	"fmt"
	"strings"
)

// NormalizeSwipeDirection lowercases a YAML `direction:` value, defaults
// empty to "up" (Maestro's default), and rejects anything that isn't a
// cardinal direction so flow typos surface as errors instead of a silent
// full-screen guess.
func NormalizeSwipeDirection(s string) (string, error) {
	d := strings.ToLower(s)
	if d == "" {
		d = "up"
	}
	switch d {
	case "up", "down", "left", "right":
		return d, nil
	}
	return "", fmt.Errorf("invalid swipe direction: %q", s)
}

// SwipeCoordsInBounds returns absolute start/end coordinates for a
// direction-based swipe anchored on an element. The swipe starts inside
// the element (so the touch is captured by that element) and ends past
// the opposite edge (so drag-based targets like native sliders reach
// their extreme value). This matches classic Maestro's semantics on the
// two common use cases:
//   - scroll containers: touch starts inside and moves outward, scrolling
//     the container by the drag distance
//   - drag targets (sliders, drag handles): the release position past
//     the edge pins the drag target to its extreme in that direction
//
// Coordinates are clamped to the screen: negatives to 0, and — when
// screenW/screenH are known (> 0) — overshoot past the far edge to the
// last on-screen pixel, so elements flush against any screen edge still
// produce injectable coordinates.
func SwipeCoordsInBounds(direction string, b Bounds, screenW, screenH int) (startX, startY, endX, endY int, err error) {
	clamp := func(v, max int) int {
		if v < 0 {
			return 0
		}
		if max > 0 && v > max-1 {
			return max - 1
		}
		return v
	}
	pctX := func(p int) int { return clamp(b.X+b.Width*p/100, screenW) }
	pctY := func(p int) int { return clamp(b.Y+b.Height*p/100, screenH) }
	switch direction {
	case "up":
		return pctX(50), pctY(90), pctX(50), pctY(-10), nil
	case "down":
		return pctX(50), pctY(10), pctX(50), pctY(110), nil
	case "left":
		return pctX(90), pctY(50), pctX(-10), pctY(50), nil
	case "right":
		return pctX(10), pctY(50), pctX(110), pctY(50), nil
	default:
		return 0, 0, 0, 0, fmt.Errorf("invalid swipe direction: %q", direction)
	}
}
