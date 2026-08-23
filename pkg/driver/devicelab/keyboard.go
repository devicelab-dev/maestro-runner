package devicelab

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Patterns for extracting keyboard bounds from "dumpsys window InputMethod".
var (
	// Android <=12: "mFrame=[left,top][right,bottom]" (not present on Android 13+)
	mFrameRegex = regexp.MustCompile(`mFrame=\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)

	// "touchable region=SkRegion((left,top,right,bottom))" — present on all versions
	// when the keyboard sets mTouchableInsets (stock keyboards do; some vendor keyboards don't).
	touchableRegionRegex = regexp.MustCompile(`touchable region=SkRegion\(\((\d+),(\d+),(\d+),(\d+)\)\)`)

	// "mGivenContentInsets=[left,top][right,bottom]" — tells us where keyboard content
	// starts within the InputMethod window. The top inset is the transparent gap above
	// the keyboard. Present on all versions.
	contentInsetsRegex = regexp.MustCompile(`mGivenContentInsets=\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)
)

// parseKeyboardFrame extracts keyboard bounds from "dumpsys window InputMethod" output.
// Returns nil if keyboard is not visible.
//
// Strategy order (verified against AOSP source for Android 10, 11, 13):
//  1. touchable region — most accurate, gives actual keyboard area.
//  2. mFrame + mGivenContentInsets — for vendor keyboards (Samsung, Xiaomi, etc.)
//     that don't set touchable insets. Content insets reveal where keyboard starts.
//  3. mFrame alone — only if the frame looks like a keyboard (not a full-screen window).
func parseKeyboardFrame(dumpsysOutput string) *core.Bounds {
	// isOnScreen= is present on all Android versions (10+). mViewVisibility=0x8 means GONE.
	if strings.Contains(dumpsysOutput, "isOnScreen=false") ||
		strings.Contains(dumpsysOutput, "mViewVisibility=0x8") {
		return nil
	}

	// Strategy 1: touchable region — the actual keyboard touchable area.
	// Printed when mTouchableInsets != 0, which stock keyboards set but some vendor keyboards don't.
	if matches := touchableRegionRegex.FindStringSubmatch(dumpsysOutput); matches != nil {
		return boundsFromMatches(matches)
	}

	// Strategy 2+3: mFrame-based fallback (Android <=12 only; Android 13+ uses Frames: format).
	frameMatches := mFrameRegex.FindStringSubmatch(dumpsysOutput)
	if frameMatches == nil {
		return nil
	}
	bounds := boundsFromMatches(frameMatches)
	if bounds == nil {
		return nil
	}

	// Strategy 2: adjust mFrame by content insets. mGivenContentInsets.top tells us how many
	// pixels from the window top are transparent (not keyboard). This handles vendor keyboards
	// that use a full-screen InputMethod window but report content insets correctly.
	if insetsMatches := contentInsetsRegex.FindStringSubmatch(dumpsysOutput); insetsMatches != nil {
		topInset, _ := strconv.Atoi(insetsMatches[2])
		if topInset > 0 {
			bounds.Y += topInset
			bounds.Height -= topInset
			if bounds.Height <= 0 {
				return nil
			}
			return bounds
		}
	}

	// Strategy 3: bare mFrame. Sanity check — a real keyboard is at most ~60% of screen height.
	// If the frame is taller, it's the full InputMethod window, not the keyboard.
	screenBottom := bounds.Y + bounds.Height
	if screenBottom > 0 && bounds.Height > screenBottom*6/10 {
		return nil
	}
	return bounds
}

// boundsFromMatches converts regex matches [_, left, top, right, bottom] to Bounds.
// Atoi errors are safe to ignore — the regex guarantees \d+ captures.
// Returns nil if the resulting area has zero or negative dimensions.
func boundsFromMatches(matches []string) *core.Bounds {
	left, _ := strconv.Atoi(matches[1])
	top, _ := strconv.Atoi(matches[2])
	right, _ := strconv.Atoi(matches[3])
	bottom, _ := strconv.Atoi(matches[4])

	width := right - left
	height := bottom - top

	if width <= 0 || height <= 0 {
		return nil
	}

	return &core.Bounds{
		X:      left,
		Y:      top,
		Width:  width,
		Height: height,
	}
}

// getKeyboardBounds returns the keyboard frame if visible, nil otherwise.
// Requires device (ShellExecutor) to be available.
func (d *Driver) getKeyboardBounds() *core.Bounds {
	if d.device == nil {
		return nil
	}

	output, err := d.device.Shell("dumpsys window InputMethod")
	if err != nil {
		return nil
	}

	if strings.Contains(output, "mInputShown=false") {
		return nil
	}

	return parseKeyboardFrame(output)
}

// isKeyboardVisible checks if the soft keyboard is currently shown using dumpsys.
//nolint:unused
func (d *Driver) isKeyboardVisible() bool {
	return d.getKeyboardBounds() != nil
}

// tapWouldHitKeyboard returns true if a tap on the element's center would land
// on the keyboard area instead of the element.
//
// This used to allow a 50px margin below the reported keyboard top, on the
// theory that the touchable region includes a suggestion strip above the real
// keys and elements barely overlapping it stay tappable. Measurement says
// otherwise: on a Pixel 4a the IME reports `touchable region=SkRegion((0,1428,
// 1080,2340))`, and a tap at y=1439 — inside the region but within the old
// margin — was swallowed by the keyboard, leaving the field unfocused while the
// step reported success (#139). The touchable region is exactly the area the IME
// window consumes touches in; the suggestion strip consumes them just as the
// keys do. All three parse strategies already normalise to the keyboard's real
// area (touchable region directly, or mFrame corrected by content insets), so
// adding slack on top of that double-counts.
func tapWouldHitKeyboard(element, keyboard core.Bounds) bool {
	_, cy := element.Center()
	return cy >= keyboard.Y
}

// consumeInputFlag checks and resets the lastStepWasInput flag.
// Returns true if the previous step was an input step.
func (d *Driver) consumeInputFlag() bool {
	was := d.lastStepWasInput
	d.lastStepWasInput = false
	return was
}

var errKeyboardOpen = fmt.Errorf("keyboard is open — add a `- hideKeyboard` step before this step")

// keyboardSettleWindow bounds how long checkKeyboardBlocking re-samples geometry before
// declaring the element covered. Windows with SOFT_INPUT_ADJUST_RESIZE (e.g. a plain
// AlertDialog whose body is a ScrollView) relayout a few frames after the IME appears or
// after typing: on the first frame the target still reports covered bounds, then the
// window shrinks and the target rises above the keyboard. A single-shot check reads that
// stale first frame and rejects a perfectly tappable element. Var (not const) so tests
// can shrink it.
var keyboardSettleWindow = 2 * time.Second

// keyboardSettlePoll is the re-sample cadence while waiting for the geometry to settle.
const keyboardSettlePoll = 50 * time.Millisecond

// keyboardStillCovering is the per-sample verdict: true only when the keyboard is
// visible AND a tap on the element's center would land on it.
func keyboardStillCovering(element core.Bounds, keyboard *core.Bounds) bool {
	return keyboard != nil && tapWouldHitKeyboard(element, *keyboard)
}

// checkKeyboardBlocking checks if the keyboard overlaps the target element.
// UIA2 finds elements via the accessibility tree even when the keyboard covers them,
// but coordinate taps land on the keyboard overlay instead. This detects that case and
// fails with a helpful hint instead of silently tapping the keyboard.
// Returns nil if this check doesn't apply or element is not blocked — caller should proceed normally.
//
// The keyboard is not only up because the previous step typed: a field with
// autoFocus raises the IME on screen entry, and the IME survives navigation.
// Gating solely on wasInput let those cases through, so the coordinate tap
// landed on the keyboard, the step still reported success, and the following
// `inputText` — which injects global key events — typed into whatever element
// actually held focus. That silent misdirection is #139. So when the previous
// step wasn't an input, fall back to asking whether the keyboard is up at all.
//
// Ordering matters for cost: `wasInput` short-circuits, so the tap-after-typing
// path pays exactly what it did before. Only taps that previously skipped the
// check outright spend a dumpsys, and only to discover the keyboard is down.
func (d *Driver) checkKeyboardBlocking(wasInput bool, sel flow.Selector) *core.CommandResult {
	if !wasInput && d.getKeyboardBounds() == nil {
		return nil
	}

	return settleKeyboardBlocking(
		func() (*core.ElementInfo, bool) {
			// Find element (UIA2 will find it even behind keyboard)
			_, info, err := d.findElementOnce(sel)
			if err != nil || info == nil {
				// Element genuinely not found — let caller do the full-timeout find
				return nil, false
			}
			return info, true
		},
		d.getKeyboardBounds,
	)
}

// settleKeyboardBlocking wraps the shared core settle loop (core.SettleKeyboardBlocking)
// with this driver's ElementInfo sampler, verdict (keyboardStillCovering, which includes
// the suggestion-strip margin), and error. The loop itself is shared with the uiautomator2
// driver so the two can't drift on timing behavior.
func settleKeyboardBlocking(findElement func() (*core.ElementInfo, bool),
	keyboardBounds func() *core.Bounds) *core.CommandResult {
	blocked, kbTop, centerY := core.SettleKeyboardBlocking(
		func() (core.Bounds, bool) {
			info, ok := findElement()
			if !ok {
				return core.Bounds{}, false
			}
			return info.Bounds, true
		},
		keyboardBounds,
		keyboardStillCovering,
		keyboardSettleWindow, keyboardSettlePoll,
	)
	if !blocked {
		return nil
	}
	return errorResult(errKeyboardOpen,
		fmt.Sprintf("Element found but keyboard is covering it (keyboard top: %d, element center Y: %d) — add a `- hideKeyboard` step before this step",
			kbTop, centerY))
}
