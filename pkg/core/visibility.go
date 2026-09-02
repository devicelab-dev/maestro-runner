package core

// VisibleFraction reports how much of the element's area lies inside the
// viewport, as a fraction in [0, 1].
//
// This is the stop criterion scrollUntilVisible needs: "found in the tree"
// and "any pixel on screen" both accept an element still half-hidden behind
// a bottom bar or barely peeking over the fold, and the tap that follows
// lands wrong. A zero-area element reports 0 — there is nothing visible to
// stop for.
func VisibleFraction(b Bounds, screenW, screenH int) float64 {
	area := b.Width * b.Height
	if area <= 0 || screenW <= 0 || screenH <= 0 {
		return 0
	}

	left := max(b.X, 0)
	top := max(b.Y, 0)
	right := min(b.X+b.Width, screenW)
	bottom := min(b.Y+b.Height, screenH)

	if right <= left || bottom <= top {
		return 0
	}
	return float64((right-left)*(bottom-top)) / float64(area)
}

// MeetsVisibility reports whether the element satisfies a visibilityPercentage
// requirement (1-100). A percentage outside that range means the caller didn't
// set one and gets the default: fully visible. That default is deliberate —
// it is what scrollUntilVisible's contract promises, and a partially covered
// element is exactly the case the check exists to reject.
func MeetsVisibility(b Bounds, screenW, screenH, percentage int) bool {
	if percentage < 1 || percentage > 100 {
		percentage = 100
	}
	// Compare in integer percent to avoid 0.999999… missing 100 on floats.
	return int(VisibleFraction(b, screenW, screenH)*100+0.5) >= percentage
}

// ClippedAtScrollEdge reports whether b's leading edge in the scroll direction
// sits on container's matching edge, within tolerance pixels.
//
// VisibleFraction cannot see clipping by a scroll container. Android hands us
// bounds the hierarchy has already clipped, so a 9px sliver of a 126px button
// peeking over the fold arrives as a 9px rect wholly inside the screen and
// scores 100% — scrollUntilVisible stops, and the tap that follows lands on
// whatever sits under the fold. Only clipping by the screen edge is visible to
// the arithmetic, because there the reported rect still extends past the
// viewport.
//
// The shared edge is the signal: a rect the container truncated ends exactly
// where the container's visible area ends.
//
// Only the leading edge counts — the bottom when scrolling down, the top when
// scrolling up. Both edges would be far too eager: in a vertical list the first
// visible row is routinely flush with the container's top, and treating that as
// clipped would spend a scroll moving a perfectly visible element away, or off
// screen entirely. New content enters at the leading edge, so that is the only
// edge where a sliver can appear.
//
// Being flush does not prove truncation — the last row of a fully scrolled list
// is flush too. It only marks the rect as worth a second look; the caller
// decides by scrolling once and seeing whether the element grows.
func ClippedAtScrollEdge(b, container Bounds, direction string, tolerance int) bool {
	if b.Width <= 0 || b.Height <= 0 || container.Width <= 0 || container.Height <= 0 {
		return false
	}
	near := func(a, c int) bool {
		d := a - c
		if d < 0 {
			d = -d
		}
		return d <= tolerance
	}
	switch direction {
	case "down":
		return near(b.Y+b.Height, container.Y+container.Height)
	case "up":
		return near(b.Y, container.Y)
	case "right":
		return near(b.X+b.Width, container.X+container.Width)
	case "left":
		return near(b.X, container.X)
	default:
		return false
	}
}
