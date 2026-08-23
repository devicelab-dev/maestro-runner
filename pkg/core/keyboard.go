package core

import "time"

// SettleKeyboardBlocking re-samples element vs keyboard geometry until window
// elapses, returning whether a tap on the element is PERSISTENTLY blocked by
// the keyboard. IME-aware Android windows (SOFT_INPUT_ADJUST_RESIZE) relayout a
// few frames after the keyboard appears or after typing — on the first frame
// the target still reports covered bounds, then the window shrinks and the
// target rises above the keyboard. A single-shot check reads that stale first
// frame and rejects a perfectly tappable element; re-sampling only fails when
// the overlap survives the whole window (the true positive the guard exists
// for), and returns early the instant the element clears or the keyboard goes.
//
// The samplers are injected so each driver keeps its own geometry semantics
// (e.g. a suggestion-strip margin in `stillCovering`) and this loop stays a
// pure, shared, testable timing structure. Returns blocked=false immediately
// when the element isn't present (caller should proceed to its normal find).
// When blocked, kbTop/centerY are the last covered sample's geometry, for the
// caller's error message.
func SettleKeyboardBlocking(
	findElement func() (Bounds, bool),
	keyboardBounds func() *Bounds,
	stillCovering func(element Bounds, keyboard *Bounds) bool,
	window, poll time.Duration,
) (blocked bool, kbTop, centerY int) {
	deadline := time.Now().Add(window)
	// lastKbTop keeps -1 when the keyboard vanishes mid-settle and we never see
	// its top. lastCenterY needs no sentinel: every path that reads it assigns
	// it first, from the element sampled this iteration.
	lastKbTop := -1
	var lastCenterY int
	for {
		elem, present := findElement()
		if !present {
			return false, 0, 0
		}
		kb := keyboardBounds()
		if !stillCovering(elem, kb) {
			// Keyboard dismissed, or the window resized and the element now
			// sits above it — nothing blocks the tap.
			return false, 0, 0
		}
		_, cy := elem.Center()
		if kb != nil {
			lastKbTop = kb.Y
		}
		lastCenterY = cy

		if !time.Now().Before(deadline) {
			return true, lastKbTop, lastCenterY
		}
		time.Sleep(poll)
	}
}
