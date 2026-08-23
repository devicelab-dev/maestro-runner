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
