package core

import "strings"

// VisibleFraction reports how much of the element's area lies inside the
// viewport, as a fraction in [0, 1].
//
// This is the stop criterion scrollUntilVisible needs: "found in the tree"
// and "any pixel on screen" both accept an element still half-hidden behind
// a bottom bar or barely peeking over the fold, and the tap that follows
// lands wrong. A zero-area element reports 0 — there is nothing visible to
// stop for.
func VisibleFraction(b Bounds, screenW, screenH int) float64 {
	return VisibleFractionWithin(b, Bounds{Width: screenW, Height: screenH})
}

// VisibleFractionWithin reports how much of the element's area lies inside
// area, as a fraction in [0, 1]. VisibleFraction is the case where area is the
// screen.
func VisibleFractionWithin(b, area Bounds) float64 {
	size := b.Width * b.Height
	if size <= 0 || area.Width <= 0 || area.Height <= 0 {
		return 0
	}

	left := max(b.X, area.X)
	top := max(b.Y, area.Y)
	right := min(b.X+b.Width, area.X+area.Width)
	bottom := min(b.Y+b.Height, area.Y+area.Height)

	if right <= left || bottom <= top {
		return 0
	}
	return float64((right-left)*(bottom-top)) / float64(size)
}

// MeetsVisibility reports whether the element satisfies a visibilityPercentage
// requirement (1-100). A percentage outside that range means the caller didn't
// set one and gets the default: fully visible. That default is deliberate —
// it is what scrollUntilVisible's contract promises, and a partially covered
// element is exactly the case the check exists to reject.
func MeetsVisibility(b Bounds, screenW, screenH, percentage int) bool {
	return MeetsVisibilityWithin(b, Bounds{Width: screenW, Height: screenH}, percentage)
}

// MeetsVisibilityWithin is MeetsVisibility against area instead of the screen.
func MeetsVisibilityWithin(b, area Bounds, percentage int) bool {
	if percentage < 1 || percentage > 100 {
		percentage = 100
	}
	// Compare in integer percent to avoid 0.999999… missing 100 on floats.
	return int(VisibleFractionWithin(b, area)*100+0.5) >= percentage
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

// HangsPastScrollEdge reports whether b runs past container's leading edge in
// the scroll direction (the bottom when scrolling down, the top when scrolling
// up), by more than tolerance pixels and more than the visibility percentage
// allows.
//
// VisibleFraction measures against the screen, and the hierarchy doesn't
// always clip a child to its scroll container: a bottom sheet's last row was
// reported at [63,2295][1017,2400] on a 2400px screen while the sheet's list
// ended at 2337, above the gesture bar. Against the screen the row is wholly
// visible; inside the list 40% of it shows, and a tap on its centre (y 2347)
// lands on the gesture bar. Scrolling on in the same direction brings it in.
//
// Only the leading edge counts, as in ClippedAtScrollEdge: a scroll can't
// fix an overhang on any other side, and scrolling the other way would move
// the element further out. An element longer than its container along the
// scroll axis never fits, and a container with no area, or one b doesn't
// overlap across the scroll axis, isn't measured; those report false, as
// before this check. A match wholly past the leading edge is measured: it is
// the same bug at its worst.
func HangsPastScrollEdge(b, container Bounds, direction string, percentage, tolerance int) bool {
	if container.Width <= 0 || container.Height <= 0 {
		return false
	}
	acrossX := b.X < container.X+container.Width && b.X+b.Width > container.X
	acrossY := b.Y < container.Y+container.Height && b.Y+b.Height > container.Y
	// The part of the screen before the container's leading edge, widened by
	// tolerance: the element is measured against that edge alone.
	before := b
	switch direction {
	case "down":
		if !acrossX || b.Height > container.Height {
			return false
		}
		before.Height = container.Y + container.Height + tolerance - b.Y
	case "up":
		if !acrossX || b.Height > container.Height {
			return false
		}
		before.Y = container.Y - tolerance
		before.Height = b.Y + b.Height - before.Y
	case "right":
		if !acrossY || b.Width > container.Width {
			return false
		}
		before.Width = container.X + container.Width + tolerance - b.X
	case "left":
		if !acrossY || b.Width > container.Width {
			return false
		}
		before.X = container.X - tolerance
		before.Width = b.X + b.Width - before.X
	default:
		return false
	}
	return !MeetsVisibilityWithin(b, before, percentage)
}

// ScrollSpeedToDurationMs converts a Maestro `speed:` value into the swipe
// duration one scroll gesture should take.
//
// Maestro's number is inverted — it is a speed, and the gesture is expressed as
// a duration — so it computes (Commands.kt, speedToDuration):
//
//	duration = 1000 * (100 - speed) / 100 + 1
//
// speed 1 → 991ms (a slow, deliberate drag), the default 40 → 601ms, speed 100
// → 1ms. The formula is reproduced rather than approximated because a flow
// written against Maestro has to scroll the same distance per swipe here; an
// "equivalent" curve would make the same flow behave differently, which is the
// whole thing we promise not to do.
//
// Out-of-range values fall back to the default rather than failing: upstream
// clamps a negative duration to its default, and a flow should not die over a
// speed of 250.
//
// The result is floored at MinSwipeDurationMs. Upstream hands speed 100's 1ms
// straight to `input swipe`, and Android emits no MOVE events for a swipe that
// short — just a DOWN and an UP at different points. A React Native list read
// that as a press on the row under the finger and opened it, on every run, on
// both native drivers (Pixel 4a, TestHive). Nothing is lost by the floor:
// Android caps fling velocity, and 20ms, 50ms and 100ms swipes over the same
// 1000px all landed on the same row, while 300ms landed two rows short. So
// speeds 96-100 behave like 95 here, and scroll instead of tapping.
func ScrollSpeedToDurationMs(speed int) int {
	const defaultDurationMs = 601 // upstream's DEFAULT_SCROLL_DURATION, speed 40
	if speed < 0 || speed > 100 {
		return defaultDurationMs
	}
	if speed == 0 {
		// Unset. Callers must not substitute upstream's default here: each
		// driver already had its own scroll duration (300ms, 500ms, 0.3s), and
		// swipe duration sets fling velocity on Android — a longer swipe over
		// the same distance flings less far. Silently moving every existing
		// flow to 601ms would change how far every scroll travels, which is the
		// complaint in #141. ScrollDurationOrDefault keeps the driver's value.
		return 0
	}
	d := 1000*(100-speed)/100 + 1
	if d < 0 {
		return defaultDurationMs
	}
	if d < MinSwipeDurationMs {
		return MinSwipeDurationMs
	}
	return d
}

// MinSwipeDurationMs is the shortest swipe a `speed:` can ask for. Below it
// Android injects no intermediate MOVE events and a touch-handling framework
// may take the gesture for a tap; see ScrollSpeedToDurationMs.
const MinSwipeDurationMs = 50

// ScrollDurationOrDefault returns the duration a `speed:` asks for, or
// driverDefaultMs when the field is absent.
//
// Deliberately not upstream's 601ms default: adopting that would change the
// swipe duration of every existing scroll, and on Android duration sets fling
// velocity, so the distance each scroll travels would move too. A flow that
// never mentions speed keeps behaving exactly as it did.
func ScrollDurationOrDefault(speed, driverDefaultMs int) int {
	if d := ScrollSpeedToDurationMs(speed); d > 0 {
		return d
	}
	return driverDefaultMs
}

// NearScreenCenter is scrollUntilVisible's centerElement test, as in Maestro:
// the element's centre lies in the half of the screen the content scrolls
// towards, plus a fifth of the screen. Scrolling down (content moving up)
// wants the centre above the middle plus that margin.
func NearScreenCenter(b Bounds, screenW, screenH int, scrollDirection string) bool {
	cx, cy := b.X+b.Width/2, b.Y+b.Height/2
	switch strings.ToLower(scrollDirection) {
	case "up":
		return cy > screenH/2-screenH/5
	case "left":
		return cx > screenW/2-screenW/5
	case "right":
		return cx < screenW/2+screenW/5
	default:
		return cy < screenH/2+screenH/5
	}
}
