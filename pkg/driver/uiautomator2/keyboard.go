package uiautomator2

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
	// Android <=12: "mFrame=[left,top][right,bottom]"
	mFrameRegex = regexp.MustCompile(`mFrame=\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)

	// Android 13+: "touchable region=SkRegion((left,top,right,bottom))"
	touchableRegionRegex = regexp.MustCompile(`touchable region=SkRegion\(\((\d+),(\d+),(\d+),(\d+)\)\)`)
)

// parseKeyboardFrame extracts keyboard bounds from "dumpsys window InputMethod" output.
// Supports both Android <=12 (mFrame=) and Android 13+ (touchable region + isOnScreen) formats.
// Returns nil if keyboard is not visible.
func parseKeyboardFrame(dumpsysOutput string) *core.Bounds {
	// Strategy 1: Android <=12 — look for mFrame=
	if matches := mFrameRegex.FindStringSubmatch(dumpsysOutput); matches != nil {
		return boundsFromMatches(matches)
	}

	// Strategy 2: Android 13+ — check isOnScreen + touchable region
	if !strings.Contains(dumpsysOutput, "isOnScreen=true") {
		return nil
	}

	if matches := touchableRegionRegex.FindStringSubmatch(dumpsysOutput); matches != nil {
		return boundsFromMatches(matches)
	}

	return nil
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
func (d *Driver) isKeyboardVisible() bool {
	return d.getKeyboardBounds() != nil
}

// waitKeyboardHidden polls (up to ~600ms) until the soft keyboard is no longer
// shown, allowing for the dismissal animation. Returns true once hidden. When
// there's no shell to inspect (d.device == nil) it reports hidden immediately —
// the caller can't verify, so it best-efforts the result.
func (d *Driver) waitKeyboardHidden() bool {
	if d.device == nil {
		return true
	}
	for i := 0; i < 6; i++ {
		if !d.isKeyboardVisible() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !d.isKeyboardVisible()
}

// tapWouldHitKeyboard returns true if a tap on the element's center would land
// on the keyboard area instead of the element.
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

// checkKeyboardBlocking checks if the keyboard overlaps the target element after an input step.
// UIA2 finds elements via the accessibility tree even when the keyboard covers them,
// but coordinate taps land on the keyboard overlay instead. This detects that case and
// fails fast with a helpful hint instead of silently tapping the keyboard.
// Returns nil if this check doesn't apply or element is not blocked — caller should proceed normally.
func (d *Driver) checkKeyboardBlocking(wasInput bool, sel flow.Selector) *core.CommandResult {
	if !wasInput {
		return nil
	}

	// Find element (UIA2 will find it even behind keyboard)
	_, info, err := d.findElementOnce(sel)
	if err != nil || info == nil {
		// Element genuinely not found — let caller do the full-timeout find
		return nil
	}

	// Element found — check if keyboard overlaps its bounds
	kbBounds := d.getKeyboardBounds()
	if kbBounds == nil {
		return nil
	}

	if tapWouldHitKeyboard(info.Bounds, *kbBounds) {
		_, cy := info.Bounds.Center()
		return errorResult(errKeyboardOpen,
			fmt.Sprintf("Element found but keyboard is covering it (keyboard top: %d, element center Y: %d) — add a `- hideKeyboard` step before this step",
				kbBounds.Y, cy))
	}

	return nil
}
