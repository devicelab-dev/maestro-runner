package devicelab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
)

// ============================================================================
// Tap Commands
// ============================================================================

func (d *Driver) tapOn(step *flow.TapOnStep) *core.CommandResult {
	// Check if using Point WITHOUT selector (screen-relative tap)
	if step.Point != "" && step.Selector.IsEmpty() {
		return d.tapOnPointWithCoords(step.Point)
	}

	wasInput := d.consumeInputFlag()

	// Quick check: if previous step was input and keyboard is blocking, fail fast
	if result := d.checkKeyboardBlocking(wasInput, step.Selector); result != nil {
		return result
	}

	// For text- AND id-based taps, use FindAndClick —
	// single atomic Java call: find node + coordinate click at center.
	// No stale nodes, no performAction, no parent walk-up. Routing id
	// selectors through this path gives them the same tree-signature
	// settle that text selectors get (agent's findAndClick → findElement
	// with settle=true), so id taps don't fire mid-animation.
	if (step.Selector.Text != "" || step.Selector.ID != "") && step.Point == "" && !step.Selector.HasRelativeSelector() {
		// Browser mode: find via CDP and click via CDP
		if d.isBrowserMode() {
			return d.tapOnBrowser(step)
		}

		// Clickable-first: prefer buttons over labels. The agent promotes a text
		// match to its nearest clickable ancestor when .clickable(true) is set,
		// so "tapOn: SIGN IN" hits the clickable login-button ViewGroup even
		// when the text lives on a non-clickable TextView child.
		clickableStrategies, _ := buildClickableOnlyStrategies(step.Selector)
		allStrategies, err := buildSelectors(step.Selector, 0)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to build selectors: %v", err))
		}
		strategies := append(clickableStrategies, allStrategies...)
		timeout := d.calculateTimeout(step.IsOptional(), step.TimeoutMs)
		ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
		defer cancel()

		// Read once: cached after the first call, and the agent needs it on
		// every attempt. Zero on failure, which tells the agent to click
		// unconditionally — the pre-#162 behaviour.
		guardW, guardH, guardErr := d.tappableScreenSize()
		if guardErr != nil {
			guardW, guardH = 0, 0
		}
		// Hit-testing costs one tree walk per attempt and only ever turns a
		// tap that would have been swallowed into a re-poll. Off switch for a
		// tree pathological enough that the walk is not worth it.
		hitTest := os.Getenv("MAESTRO_DISABLE_HIT_TEST") == ""

		var lastErr error
		for {
			select {
			case <-ctx.Done():
				if lastErr != nil {
					return errorResult(fmt.Errorf("%s: %w", ctx.Err(), lastErr), fmt.Sprintf("Element not found: %v", lastErr))
				}
				return errorResult(ctx.Err(), fmt.Sprintf("Element not found: %v", ctx.Err()))
			default:
				d.ensureWebViewConnection()

				// Re-check: browser mode may have been detected after ensureWebViewConnection
				if d.isBrowserMode() {
					return d.tapOnBrowser(step)
				}

				for _, s := range strategies {
					// Capture the pre-tap tree hash so a later failing
					// assertion can detect "tap had no effect" and retry.
					d.recordTap(step.Selector)

					// Hand the agent the screen size so it can reject an
					// untappable rect BEFORE injecting the tap. Previously the
					// check below ran on a tap that had already landed: a
					// clipped rect's centre sits outside the element, and with
					// a bottom tab bar that centre is a tab, so the "rejected"
					// tap navigated and desynced the flow (#162).
					elem, clicked, blockedBy, err := d.client.FindAndClickChecked(s.Strategy, s.Value, guardW, guardH, hitTest)
					if err == nil {
						info := &core.ElementInfo{
							Visible: true,
							Enabled: true,
						}
						if t, err := elem.Text(); err == nil {
							info.Text = t
						}
						rectOK := false
						if rect, err := elem.Rect(); err == nil {
							info.Bounds = core.Bounds{X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height}
							rectOK = true
						}
						logger.Info("[devicelab] FindAndClick hit for %s via %s=%s: bounds=[%d,%d][%d,%d] (w=%d h=%d) center=(%d,%d)",
							step.Selector.Describe(), s.Strategy, s.Value,
							info.Bounds.X, info.Bounds.Y,
							info.Bounds.X+info.Bounds.Width, info.Bounds.Y+info.Bounds.Height,
							info.Bounds.Width, info.Bounds.Height,
							info.Bounds.X+info.Bounds.Width/2, info.Bounds.Y+info.Bounds.Height/2)

						// #94: reject a tap whose rect is malformed (non-positive
						// width/height) or whose centre lies off-screen, and keep
						// polling. The agent's id-find path applies no on-screen
						// filter, so a just-opened bottom sheet's first laid-out
						// frame yields a clipped rect (top>bottom) and FindAndClick
						// injects the tap off-screen — a no-op that leaves the flow
						// desynced. A settled frame a moment later taps the real
						// target. (Mirrors the assert-side viewport check from #39.)
						// The agent declined to tap. Nothing was injected,
						// so just keep polling for a settled frame.
						if !clicked {
							// blockedBy is set when the hit test found something
							// over the point; otherwise the rect itself was bad.
							if blockedBy != "" {
								logger.Info("[devicelab] tap skipped before injection for %s: point covered by %s — re-polling",
									step.Selector.Describe(), blockedBy)
								lastErr = fmt.Errorf("tap point is covered by %s", blockedBy)
							} else {
								logger.Info("[devicelab] tap skipped before injection (untappable rect) for %s: w=%d h=%d center=(%d,%d) screen=%dx%d — re-polling",
									step.Selector.Describe(), info.Bounds.Width, info.Bounds.Height,
									info.Bounds.X+info.Bounds.Width/2, info.Bounds.Y+info.Bounds.Height/2, guardW, guardH)
								lastErr = fmt.Errorf("element rect not tappable (w=%d h=%d center=(%d,%d) screen=%dx%d)",
									info.Bounds.Width, info.Bounds.Height,
									info.Bounds.X+info.Bounds.Width/2, info.Bounds.Y+info.Bounds.Height/2, guardW, guardH)
							}
							time.Sleep(50 * time.Millisecond)
							break
						}

						// Fallback for an agent predating the guard above: it
						// has already clicked, so this only stops a second tap.
						if rectOK {
							// Validate against the FULL physical display (same coordinate
							// space as info.Bounds, which come from the accessibility
							// hierarchy). screenSize() can report the USABLE height (minus
							// the status bar), which wrongly condemns on-screen bottom
							// buttons/FABs whose centre sits in the bottom band.
							if sw, sh, serr := d.tappableScreenSize(); serr == nil && !boundsTappable(info.Bounds, sw, sh) {
								logger.Info("[devicelab] tap rejected (off-screen/malformed rect) for %s: w=%d h=%d center=(%d,%d) screen=%dx%d — re-polling",
									step.Selector.Describe(), info.Bounds.Width, info.Bounds.Height,
									info.Bounds.X+info.Bounds.Width/2, info.Bounds.Y+info.Bounds.Height/2, sw, sh)
								lastErr = fmt.Errorf("element rect not tappable (w=%d h=%d center=(%d,%d) screen=%dx%d)",
									info.Bounds.Width, info.Bounds.Height,
									info.Bounds.X+info.Bounds.Width/2, info.Bounds.Y+info.Bounds.Height/2, sw, sh)
								time.Sleep(50 * time.Millisecond)
								break
							}
						}

						// Post-tap verification candidates (all wired but
						// NOT called — empirically none reliably distinguish
						// "tap had effect" from "tap fired ripple only" on
						// the React Navigation showcase app):
						//   d.tapHadEffectViaWindowUpdate("")  // Maestro's isWindowUpdating
						//   d.tapHadEffect()                   // element-presence check
						// Both produce false positives (ripples count) and
						// false negatives (buttons persist across screens).
						return successResult("Tapped on element", info)
					}
					lastErr = err
				}
			}
		}
	}

	// Browser mode: all selectors go through CDP find + CDP click
	if d.isBrowserMode() {
		return d.tapOnBrowser(step)
	}

	_, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		err = d.notFoundOrCrash(err)
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}
	if info == nil {
		return errorResult(fmt.Errorf("nil element info"), "Element info is nil")
	}

	logger.Info("[devicelab] tap target for %s: bounds=[%d,%d][%d,%d] (w=%d h=%d)",
		step.Selector.Describe(),
		info.Bounds.X, info.Bounds.Y,
		info.Bounds.X+info.Bounds.Width, info.Bounds.Y+info.Bounds.Height,
		info.Bounds.Width, info.Bounds.Height)

	// If Point is specified WITH selector, tap at relative position within element bounds
	if step.Point != "" && info.Bounds.Width > 0 {
		x, y, parseErr := core.ParsePointCoords(step.Point, info.Bounds.Width, info.Bounds.Height)
		if parseErr != nil {
			return errorResult(parseErr, fmt.Sprintf("Invalid point coordinates: %v", parseErr))
		}
		x += info.Bounds.X
		y += info.Bounds.Y
		if bad := d.guardTapInjection(step.Selector.Describe(), info.Bounds, x, y); bad != nil {
			return bad
		}
		if err := d.client.Click(x, y); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to tap at relative point: %v", err))
		}
		return successResult(fmt.Sprintf("Tapped at relative point (%d, %d) on element", x, y), info)
	}

	x, y := info.Bounds.Center()
	if bad := d.guardTapInjection(step.Selector.Describe(), info.Bounds, x, y); bad != nil {
		return bad
	}

	// If duration is set (or longPress: true), hold the press for that long.
	if step.DurationMs > 0 || step.LongPress {
		duration := step.DurationMs
		if duration <= 0 {
			duration = 1000
		}
		if err := d.client.LongClick(x, y, duration); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to press for %dms: %v", duration, err))
		}
		return successResult("Pressed on element", info)
	}

	// Always use coordinate-based tap (not accessibility performAction).
	// Coordinate taps simulate real touch events, which work reliably for
	// repeated taps on the same button and custom click handlers.
	if err := d.client.Click(x, y); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to tap at coordinates: %v", err))
	}

	return successResult("Tapped on element", info)
}

// tapPointInjectable reports whether a resolved tap point may be injected: the
// element must be a real rectangle, and the point must land on the display.
//
// Both halves matter and catch different things. The centre of a clipped rect
// (top > bottom, so a negative height) can still be on-screen — in #162 it was
// (540,2109) on a 1080x2400 display — so the on-screen test alone would pass
// it; only the malformed-rect test rejects it. Conversely a well-formed rect
// scrolled off the bottom needs the on-screen test.
func tapPointInjectable(b core.Bounds, x, y, screenW, screenH int) bool {
	if b.Width <= 0 || b.Height <= 0 {
		return false
	}
	return x >= 0 && x < screenW && y >= 0 && y < screenH
}

// guardTapInjection returns a failing result when a tap must not be injected,
// or nil to proceed.
//
// The coordinate paths resolve (x, y) here and inject them directly, so the
// agent-side guard in findAndClick never sees them — the check has to happen
// here or nowhere. Injecting anyway is worse than failing: a clipped rect's
// point lands outside the element, and on a screen with a bottom tab bar that
// is a tab, so the tap navigates and every later step runs on the wrong
// screen (#162).
//
// Unlike the findAndClick path this does not re-poll for a settled frame: the
// element has already been resolved by a find that polls, and these paths
// (point:, relative, index, duration) have no surrounding retry loop to hook
// into. Failing with the rect in the message is the honest outcome — the flow
// stops where the problem is instead of drifting.
//
// A screen size we cannot read disables the check rather than blocking a tap
// that would otherwise have worked.
func (d *Driver) guardTapInjection(desc string, b core.Bounds, x, y int) *core.CommandResult {
	sw, sh, err := d.tappableScreenSize()
	if err != nil || sw <= 0 || sh <= 0 {
		return nil
	}
	if tapPointInjectable(b, x, y, sw, sh) {
		return nil
	}
	logger.Info("[devicelab] tap skipped before injection for %s: bounds w=%d h=%d point=(%d,%d) screen=%dx%d",
		desc, b.Width, b.Height, x, y, sw, sh)
	e := fmt.Errorf("element rect not tappable (w=%d h=%d point=(%d,%d) screen=%dx%d)",
		b.Width, b.Height, x, y, sw, sh)
	return errorResult(e, fmt.Sprintf("Element not tappable: %v", e))
}

// boundsTappable reports whether b is a real on-screen rectangle whose centre
// can be tapped. It rejects a malformed rect (non-positive width/height — e.g.
// a clipped first-frame rect with top>bottom) and one whose centre falls
// outside the display. Used to avoid injecting a lost off-screen tap (#94).
func boundsTappable(b core.Bounds, screenW, screenH int) bool {
	if b.Width <= 0 || b.Height <= 0 {
		return false
	}
	cx := b.X + b.Width/2
	cy := b.Y + b.Height/2
	return cx >= 0 && cx < screenW && cy >= 0 && cy < screenH
}

// tapOnBrowser handles tapOn entirely via CDP for Chrome browser mode.
func (d *Driver) tapOnBrowser(step *flow.TapOnStep) *core.CommandResult {
	timeout := d.calculateTimeout(step.IsOptional(), step.TimeoutMs)
	deadline := time.Now().Add(timeout)

	var lastErr error
	for time.Now().Before(deadline) {
		d.ensureWebViewConnection()
		webElem, err := d.webView.findWebOnce(step.Selector)
		if err == nil {
			we, ok := webElem.(*WebElement)
			if !ok {
				return errorResult(fmt.Errorf("unexpected element type"), "Failed to tap via CDP")
			}
			if clickErr := we.Click(); clickErr != nil {
				return errorResult(clickErr, "Failed to tap via CDP")
			}
			return successResult("Tapped on element", webElem.Info())
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		return errorResult(fmt.Errorf("timeout: %w", lastErr), fmt.Sprintf("Element not found: %v", lastErr))
	}
	return errorResult(fmt.Errorf("element not found"), "Element not found")
}

// tapOnPointWithCoords handles point-based tap with either percentage or absolute coordinates.
func (d *Driver) tapOnPointWithCoords(point string) *core.CommandResult {
	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get screen size: %v", err))
	}

	x, y, err := core.ParsePointCoords(point, width, height)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid point coordinates: %v", err))
	}

	if err := d.client.Click(x, y); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to tap at point: %v", err))
	}

	return successResult(fmt.Sprintf("Tapped at (%d, %d)", x, y), nil)
}

func (d *Driver) dragAndDrop(step *flow.DragAndDropStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "dragAndDrop requires device access")
	}

	fromX, fromY, info, err := d.resolveGesturePoint(step.From, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("dragAndDrop: from: %v", err))
	}
	toX, toY, _, err := d.resolveGesturePoint(step.To, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("dragAndDrop: to: %v", err))
	}

	// The agent has no drag RPC, so the gesture goes through Android's own
	// `input draganddrop`, which long-presses (the system long-press timeout)
	// before moving — the lift reorder UIs need. That timeout is fixed by the
	// system: a custom holdDuration beyond it is not controllable on this
	// driver. The trailing argument is the move duration.
	cmd := fmt.Sprintf("input draganddrop %d %d %d %d %d", fromX, fromY, toX, toY, step.Duration)
	if out, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to drag (input draganddrop needs Android 12+): %v — %s", err, out))
	}
	return successResult(fmt.Sprintf("Dragged from (%d, %d) to (%d, %d)", fromX, fromY, toX, toY), info)
}

// resolveGesturePoint turns a drag endpoint — a bare point, a selector, or a
// selector plus a point inside it — into screen coordinates, following the
// same percentage/absolute rules as tapOn.
func (d *Driver) resolveGesturePoint(sel flow.Selector, optional bool, stepTimeoutMs int) (int, int, *core.ElementInfo, error) {
	if sel.IsEmpty() {
		width, height, err := d.screenSize()
		if err != nil {
			return 0, 0, nil, err
		}
		x, y, err := core.ParsePointCoords(sel.Point, width, height)
		return x, y, nil, err
	}

	_, info, err := d.findElementForTap(sel, optional, stepTimeoutMs)
	if err != nil {
		return 0, 0, nil, err
	}
	if info == nil {
		return 0, 0, nil, fmt.Errorf("nil element info")
	}
	x, y, err := core.PointInBounds(sel.Point, info.Bounds)
	return x, y, info, err
}

func (d *Driver) doubleTapOn(step *flow.DoubleTapOnStep) *core.CommandResult {
	wasInput := d.consumeInputFlag()

	if result := d.checkKeyboardBlocking(wasInput, step.Selector); result != nil {
		return result
	}

	_, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		err = d.notFoundOrCrash(err)
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	x, y, perr := core.PointInBounds(step.Selector.Point, info.Bounds)
	if perr != nil {
		return errorResult(perr, fmt.Sprintf("Invalid point coordinates: %v", perr))
	}
	if bad := d.guardTapInjection(step.Selector.Describe(), info.Bounds, x, y); bad != nil {
		return bad
	}
	if err := d.client.DoubleClick(x, y); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to double tap at coordinates: %v", err))
	}

	return successResult("Double tapped on element", info)
}

func (d *Driver) longPressOn(step *flow.LongPressOnStep) *core.CommandResult {
	wasInput := d.consumeInputFlag()

	if result := d.checkKeyboardBlocking(wasInput, step.Selector); result != nil {
		return result
	}

	_, info, err := d.findElementForTap(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		err = d.notFoundOrCrash(err)
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	duration := step.DurationMs
	if duration <= 0 {
		duration = 1000 // default 1 second
	}

	x, y, perr := core.PointInBounds(step.Selector.Point, info.Bounds)
	if perr != nil {
		return errorResult(perr, fmt.Sprintf("Invalid point coordinates: %v", perr))
	}
	if bad := d.guardTapInjection(step.Selector.Describe(), info.Bounds, x, y); bad != nil {
		return bad
	}
	if err := d.client.LongClick(x, y, duration); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to long press at coordinates: %v", err))
	}

	return successResult("Long pressed on element", info)
}

func (d *Driver) tapOnPoint(step *flow.TapOnPointStep) *core.CommandResult {
	x, y := step.X, step.Y

	if step.Point != "" {
		width, height, err := d.screenSize()
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to get screen size: %v", err))
		}

		x, y, err = core.ParsePointCoords(step.Point, width, height)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Invalid point coordinates: %v", err))
		}
	}

	if x == 0 && y == 0 {
		return errorResult(fmt.Errorf("no point specified"), "Either point or x/y coordinates required")
	}

	if step.DurationMs > 0 || step.LongPress {
		duration := step.DurationMs
		if duration <= 0 {
			duration = 1000
		}
		if err := d.client.LongClick(x, y, duration); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to press at point for %dms: %v", duration, err))
		}
		return successResult(fmt.Sprintf("Pressed at (%d, %d)", x, y), nil)
	}

	if err := d.client.Click(x, y); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to tap at point: %v", err))
	}

	return successResult(fmt.Sprintf("Tapped at (%d, %d)", x, y), nil)
}

// ============================================================================
// Assert Commands
// ============================================================================

func (d *Driver) assertVisible(step *flow.AssertVisibleStep) *core.CommandResult {
	wasInput := d.consumeInputFlag()

	if result := d.checkKeyboardBlocking(wasInput, step.Selector); result != nil {
		return result
	}

	// Browser mode: use JS RAF-based polling (60fps in-browser, single CDP call).
	if d.isBrowserMode() && d.webView != nil && d.webView.isConnected() {
		if step.Count != "" {
			err := fmt.Errorf("count is not supported in browser mode")
			return errorResult(err, "assertVisible count: not supported in browser mode")
		}
		timeout := step.TimeoutMs
		if timeout <= 0 {
			timeout = 5000
		}
		return d.assertVisibleBrowser(step.Selector, timeout)
	}

	// A count assertion needs every match, not the first one — route to the
	// page-source path, which is the only enumerator we have.
	if step.Count != "" {
		return d.assertVisibleCount(step)
	}

	_, info, err := d.findElementFastWithLazyRetry(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		err = d.notFoundOrCrash(err)
		return errorResult(err, fmt.Sprintf("Element not visible: %v", err))
	}

	if info != nil && info.Visible {
		return successResult("Element is visible", info)
	}

	return errorResult(fmt.Errorf("element not visible"), "Element exists but is not visible")
}

// assertVisibleCount asserts that the selector matches exactly the requested
// number of visible elements. Polls until the count is right or the assert
// timeout runs out, reporting the last observed count on failure.
func (d *Driver) assertVisibleCount(step *flow.AssertVisibleStep) *core.CommandResult {
	want, _, err := step.ExpectedCount()
	if err != nil {
		return errorResult(err, err.Error())
	}
	if step.Selector.HasRelativeSelector() {
		err := fmt.Errorf("count cannot be combined with relative selectors (childOf/below/...)")
		return errorResult(err, err.Error())
	}

	timeout := d.calculateTimeout(step.IsOptional(), step.TimeoutMs)
	ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
	defer cancel()

	desc := step.Selector.Describe()
	observed := -1
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if observed < 0 {
				if lastErr != nil {
					return errorResult(lastErr, fmt.Sprintf("Failed to count matches of '%s': %v", desc, lastErr))
				}
				return errorResult(ctx.Err(), fmt.Sprintf("Expected %d visible matches of '%s' but no observation completed: %v", want, desc, ctx.Err()))
			}
			err := fmt.Errorf("expected %d visible matches of '%s', last observed %d", want, desc, observed)
			return errorResult(err, err.Error())
		default:
			n, err := d.countVisibleMatches(step.Selector)
			if err != nil {
				lastErr = err
				continue // page-source round-trip is the natural rate limit
			}
			observed = n
			if n == want {
				return successResult(fmt.Sprintf("%d elements visible", n), nil)
			}
		}
	}
}

// countVisibleMatches reads the page source once and counts visible matches.
func (d *Driver) countVisibleMatches(sel flow.Selector) (int, error) {
	pageSource, err := d.client.Source()
	if err != nil {
		return 0, fmt.Errorf("failed to get page source: %w", err)
	}

	allElements, err := ParsePageSource(pageSource)
	if err != nil {
		return 0, fmt.Errorf("failed to parse page source: %w", err)
	}

	return CountDisplayedMatches(allElements, sel), nil
}

// assertVisibleBrowser uses the injected __maestro.waitForVisible() JS helper.
// RAF-based polling runs inside the browser at ~60fps — resolves within ~16ms of
// element appearing, with a single CDP roundtrip.
func (d *Driver) assertVisibleBrowser(sel flow.Selector, timeoutMs int) *core.CommandResult {
	selectorType, selectorValue := browserSelectorTypeValue(sel)
	desc := sel.DescribeQuoted()

	page := d.webView.rodPage()
	if page == nil {
		return errorResult(fmt.Errorf("no CDP connection"), fmt.Sprintf("Element %s not visible: no CDP", desc))
	}

	result, err := page.Timeout(time.Duration(timeoutMs+1000) * time.Millisecond).Evaluate(
		rod.Eval(`(type, value, timeout) => window.__maestro.waitForVisible(type, value, timeout)`,
			selectorType, selectorValue, timeoutMs).ByPromise(),
	)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element %s not visible: %v", desc, err))
	}

	if result.Value.Bool() {
		return successResult(fmt.Sprintf("Element %s is visible", desc), nil)
	}

	return errorResult(
		fmt.Errorf("element not visible within %dms", timeoutMs),
		fmt.Sprintf("Element %s not visible within %dms", desc, timeoutMs),
	)
}

func (d *Driver) assertNotVisible(step *flow.AssertNotVisibleStep) *core.CommandResult {
	timeout := step.TimeoutMs
	if timeout <= 0 {
		timeout = 5000
	}

	// Browser mode: use JS RAF-based polling (60fps in-browser, single CDP call).
	if d.isBrowserMode() && d.webView != nil && d.webView.isConnected() {
		return d.assertNotVisibleBrowser(step.Selector, timeout)
	}

	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	pollInterval := 500 * time.Millisecond

	for {
		_, info, err := d.findElementQuick(step.Selector, 0)
		if err != nil || info == nil {
			return successResult("Element is not visible", nil)
		}

		if time.Now().After(deadline) {
			return errorResult(fmt.Errorf("element is visible"), "Element should not be visible but was found")
		}

		time.Sleep(pollInterval)
	}
}

// assertNotVisibleBrowser uses the injected __maestro.waitForNotVisible() JS helper.
// RAF-based polling runs inside the browser at ~60fps — resolves within ~16ms of
// element disappearing, with a single CDP roundtrip.
func (d *Driver) assertNotVisibleBrowser(sel flow.Selector, timeoutMs int) *core.CommandResult {
	selectorType, selectorValue := browserSelectorTypeValue(sel)
	desc := sel.DescribeQuoted()

	page := d.webView.rodPage()
	if page == nil {
		// Fallback to native polling
		return nil
	}

	result, err := page.Timeout(time.Duration(timeoutMs+1000) * time.Millisecond).Evaluate(
		rod.Eval(`(type, value, timeout) => window.__maestro.waitForNotVisible(type, value, timeout)`,
			selectorType, selectorValue, timeoutMs).ByPromise(),
	)
	if err != nil {
		// JS evaluation failed (e.g. page navigated) — element is gone
		return successResult(fmt.Sprintf("Element %s is not visible", desc), nil)
	}

	if result.Value.Bool() {
		return successResult(fmt.Sprintf("Element %s is not visible", desc), nil)
	}

	return errorResult(
		fmt.Errorf("element is still visible after %dms", timeoutMs),
		fmt.Sprintf("Element %s is still visible", desc),
	)
}

// ============================================================================
// Input Commands
// ============================================================================

func (d *Driver) inputText(step *flow.InputTextStep) *core.CommandResult {
	text := step.Text
	if text == "" {
		return errorResult(fmt.Errorf("no text specified"), "No text to input")
	}

	unicodeWarning := ""
	if core.HasNonASCII(text) {
		unicodeWarning = " (warning: non-ASCII characters may not input correctly)"
	}

	// Browser mode: all input goes through CDP — native setText doesn't work for Chrome
	if d.isBrowserMode() {
		return d.inputTextBrowser(step, text, unicodeWarning)
	}

	if step.KeyPress {
		// Resolve and read the focused field first: after typing, "unchanged"
		// is the only thing that separates a hint from a lost keystroke.
		target, before := d.focusedFieldBefore()
		if err := d.client.SendKeyActions(text); err != nil {
			return errorResult(err, "Failed to input text via key press")
		}
		// Per-character key events are the path that loses characters when the
		// app janks — the reason this verification exists at all.
		note := core.ConfirmTypedText(target, text, before, logger.Warn)
		return successResult(fmt.Sprintf("Entered text (keyPress): %s%s%s", text, unicodeWarning, note), nil)
	}

	var typedInto core.TextField
	var beforeText string
	if !step.Selector.IsEmpty() {
		elem, _, err := d.findElementWithLazyRetry(step.Selector, step.IsOptional(), step.TimeoutMs)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Element not found: %v", err))
		}
		if elem != nil {
			before, _ := elem.Text()
			if err := elem.SendKeys(text); err != nil {
				// The element reference can go stale between the find and the
				// write — a Compose recomposition is enough — and the write is
				// then rejected for a field that is perfectly typeable. Key
				// events go to whatever holds focus, which after a tap is that
				// same field, so they get the text in without a second lookup.
				logger.Warn("inputText: send-keys failed (%v), falling back to key events", err)
				if keyErr := d.client.SendKeyActions(text); keyErr != nil {
					return errorResult(err, fmt.Sprintf("Failed to input text: %v (key events also failed: %v)", err, keyErr))
				}
			}
			typedInto, beforeText = core.TextFieldFuncs(elem.Text, elem.SendKeys, elem.Clear), before
		} else if d.webView != nil && d.webView.isConnected() {
			// Web element was found by Rod during polling — re-find for interaction
			webElem, webErr := d.webView.findWebOnce(step.Selector)
			if webErr != nil {
				return errorResult(webErr, "Web element found but cannot interact")
			}
			if inputErr := webElem.Input(text); inputErr != nil {
				return errorResult(inputErr, fmt.Sprintf("Failed to input text: %v", inputErr))
			}
		}
	} else {
		// No selector — type into whatever has focus. findFocused prefers
		// the WebView's DOM activeElement over the native ActiveElement,
		// which matters after a CDP tap: the DOM input has focus but blind
		// native key events never reach it, while the call still reports
		// success (#122). Element-scoped native typing likewise fixes
		// WebView inputs reached via a11y focus. Fall back to raw key
		// events when nothing reports focus.
		//
		// History: acea0c7 removed an ActiveElement path here because it
		// dragged a fragile focused=true selector-search fallback with it.
		// This reintroduction is a single findFocused round-trip with a
		// plain key-events fallback — no selector search.
		typed := false
		if focused, err := d.findFocused(); err == nil && focused != nil {
			before, _ := focused.Text()
			if err := focused.Input(text); err == nil {
				typed = true
				typedInto, beforeText = focused, before
			}
		}
		if !typed {
			if err := d.client.SendKeyActions(text); err != nil {
				return errorResult(err, fmt.Sprintf("Failed to input text: %v", err))
			}
		}
	}

	note := core.ConfirmTypedText(typedInto, text, beforeText, logger.Warn)
	return successResult(fmt.Sprintf("Entered text: %s%s%s", text, unicodeWarning, note), nil)
}

// focusedFieldBefore resolves the element that key events will reach and reads
// it, so the value can be compared once typing is done.
func (d *Driver) focusedFieldBefore() (core.TextField, string) {
	focused, err := d.findFocused()
	if err != nil || focused == nil {
		return nil, ""
	}
	before, _ := focused.Text()
	return focused, before
}

// inputTextBrowser handles inputText entirely via CDP for Chrome browser mode.
// In browser mode, Selector.Text may be populated as a YAML parsing artifact
// (InputTextStep.Text and Selector.Text share the yaml:"text" key via inline embedding).
// We detect this and route to the focused-element path.
func (d *Driver) inputTextBrowser(step *flow.InputTextStep, text, unicodeWarning string) *core.CommandResult {
	// Detect YAML parsing artifact: Selector.Text == Text with no other selector fields.
	// This means "type into focused element", not "find element by text then type".
	hasSelectorArtifact := step.Selector.Text == step.Text &&
		step.Selector.ID == "" && step.Selector.CSS == "" &&
		step.Selector.TestID == "" && step.Selector.Name == "" &&
		step.Selector.Placeholder == ""
	selectorIsReal := !step.Selector.IsEmpty() && !hasSelectorArtifact

	if selectorIsReal {
		// Real selector: find element via CDP and type into it
		timeout := d.calculateTimeout(step.IsOptional(), step.TimeoutMs)
		deadline := time.Now().Add(timeout)
		var webElem core.Element
		var err error
		for time.Now().Before(deadline) {
			d.ensureWebViewConnection()
			if d.webView.isConnected() {
				webElem, err = d.webView.findWebOnce(step.Selector)
				if err == nil {
					break
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		if webElem == nil {
			if err != nil {
				return errorResult(err, fmt.Sprintf("Element not found via CDP: %v", err))
			}
			return errorResult(fmt.Errorf("no CDP connection"), "No CDP connection for element input")
		}
		if inputErr := webElem.Input(text); inputErr != nil {
			return errorResult(inputErr, fmt.Sprintf("Failed to input text via CDP: %v", inputErr))
		}
		return successResult(fmt.Sprintf("Entered text: %s%s", text, unicodeWarning), nil)
	}

	// No real selector: type into focused element via CDP keyboard
	// Wait for CDP connection if not yet established
	if !d.webView.isConnected() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			d.ensureWebViewConnection()
			if d.webView.isConnected() {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	page := d.webView.rodPage()
	if page == nil {
		return errorResult(fmt.Errorf("no CDP connection"), "No CDP connection for text input")
	}
	if err := page.InsertText(text); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to input text via CDP: %v", err))
	}
	return successResult(fmt.Sprintf("Entered text: %s%s", text, unicodeWarning), nil)
}

func (d *Driver) eraseText(step *flow.EraseTextStep) *core.CommandResult {
	chars := step.Characters
	if chars <= 0 {
		chars = 50
	}

	// Browser mode: use CDP keyboard for backspace
	if d.isBrowserMode() {
		return d.eraseTextBrowser(chars)
	}

	// Try using Element interface (supports both web and native)
	focused, err := d.findFocused()
	if err == nil {
		currentText, textErr := focused.Text()
		if textErr == nil {
			textLen := len([]rune(currentText))

			if chars >= textLen || textLen == 0 {
				if clearErr := focused.Clear(); clearErr == nil {
					return successResult(fmt.Sprintf("Cleared %d characters", textLen), nil)
				}
			} else {
				runes := []rune(currentText)
				remaining := string(runes[:textLen-chars])

				if clearErr := focused.Clear(); clearErr == nil {
					if remaining != "" {
						if sendErr := focused.Input(remaining); sendErr == nil {
							return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
						}
					} else {
						return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
					}
				}
			}
		}
	}

	// Fallback: delete key presses (native only)
	for i := 0; i < chars; i++ {
		if err := d.client.PressKeyCode(uiautomator2.KeyCodeDelete); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to erase text: %v", err))
		}
	}

	return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
}

// eraseTextBrowser handles eraseText via CDP keyboard backspace presses.
func (d *Driver) eraseTextBrowser(chars int) *core.CommandResult {
	// Wait for CDP connection if not yet established
	if !d.webView.isConnected() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			d.ensureWebViewConnection()
			if d.webView.isConnected() {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	page := d.webView.rodPage()
	if page == nil {
		return errorResult(fmt.Errorf("no CDP connection"), "No CDP connection for erase")
	}
	kb := page.Keyboard
	for i := 0; i < chars; i++ {
		if err := kb.Type(input.Backspace); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to erase text via CDP: %v", err))
		}
	}
	return successResult(fmt.Sprintf("Erased %d characters", chars), nil)
}

func (d *Driver) hideKeyboard(_ *flow.HideKeyboardStep) *core.CommandResult {
	// Retry up to 3 times. The on-device agent sends KEYCODE_ESCAPE, which is
	// keyboard-only, and deliberately never KEYCODE_BACK — Back navigates away
	// when the IME is not actually up. It also guards on a real IME window
	// (AccessibilityWindowInfo.TYPE_INPUT_METHOD) rather than guessing from the
	// node tree, so calling this with no keyboard showing is a no-op rather
	// than a stray back-navigation.
	for attempt := 0; attempt < 3; attempt++ {
		_ = d.client.HideKeyboard()

		// Wait for keyboard to actually disappear (animation ~300ms).
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			if !d.isKeyboardVisible() {
				return successResult("Keyboard hidden", nil)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	return successResult("Hide keyboard (may not have been visible)", nil)
}

func (d *Driver) inputRandom(step *flow.InputRandomStep) *core.CommandResult {
	length := step.Length
	if length <= 0 {
		length = 10
	}

	var text string
	dataType := strings.ToUpper(step.DataType)
	switch dataType {
	case "EMAIL":
		text = core.RandomEmail()
	case "NUMBER":
		text = core.RandomNumber(length)
	case "PERSON_NAME":
		text = core.RandomPersonName()
	default:
		text = core.RandomString(length)
	}

	focused, err := d.findFocused()
	if err != nil {
		return errorResult(err, "No focused element to type into")
	}
	if err := focused.Input(text); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to input text: %v", err))
	}

	return &core.CommandResult{
		Success: true,
		Message: fmt.Sprintf("Entered random %s: %s", dataType, text),
		Data:    text,
	}
}

// ============================================================================
// Scroll/Swipe Commands
// ============================================================================

func (d *Driver) scroll(step *flow.ScrollStep) *core.CommandResult {
	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "down"
	}

	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}

	if err := d.performScroll(direction, width, height, step.Engine, 0.5); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to scroll: %v", err))
	}
	return successResult(fmt.Sprintf("Scrolled %s", direction), nil)
}

// atScrollContainerEdge reports whether the element occupying b ends exactly on
// its nearest scrollable ancestor's leading edge — the shape of a rect the
// hierarchy clipped rather than a whole element that happens to be visible
// (#164).
//
// Costs one page-source fetch, and is only consulted once the geometry check
// has already passed, so a scroll that stops on an unambiguous match pays
// nothing. Any failure to establish the ancestry answers false: an
// unverifiable tree must not block a match that the geometry accepted.
func (d *Driver) atScrollContainerEdge(b core.Bounds, direction string) bool {
	src, err := d.client.Source()
	if err != nil {
		return false
	}
	elems, err := ParsePageSource(src)
	if err != nil {
		return false
	}
	for _, e := range elems {
		if e.Bounds != b {
			continue
		}
		for p := e.Parent; p != nil; p = p.Parent {
			if !p.Scrollable {
				continue
			}
			return core.ClippedAtScrollEdge(b, p.Bounds, direction, scrollEdgeTolerancePx)
		}
	}
	return false
}

// How far from the container edge still counts as flush. Rounding between the
// hierarchy's integer bounds and the container's own edge leaves a pixel or
// two; anything larger is a real gap.
const scrollEdgeTolerancePx = 2

func (d *Driver) scrollUntilVisible(step *flow.ScrollUntilVisibleStep) *core.CommandResult {
	// `from:` confines the scroll to a container. Only the UIAutomator2 driver
	// implements it so far; refusing here is better than silently scrolling the
	// whole screen and leaving the flow author to wonder why.
	if !step.From.IsEmpty() {
		return errorResult(fmt.Errorf("unsupported option"), "scrollUntilVisible `from:` is not supported on this driver yet — it currently works on the uiautomator2 driver")
	}

	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "down"
	}

	maxScrolls := 20
	if step.MaxScrolls > 0 {
		maxScrolls = step.MaxScrolls
	}
	timeout := 30 * time.Second
	if step.TimeoutMs > 0 {
		timeout = time.Duration(step.TimeoutMs) * time.Millisecond
	}
	deadline := time.Now().Add(timeout)

	// Use the FULL physical display (same coordinate space as the hierarchy bounds). An element
	// in the bottom system-bar band — e.g. the last nav-drawer item, centre y in
	// (usableHeight, physicalHeight] — is genuinely on screen and tappable (see boundsTappable),
	// so isElementOnScreen must NOT treat it as off-screen; otherwise scrollUntilVisible loops to
	// the scroll cap on a last item that is already shown. Falls back to screenSize().
	width, height, err := d.tappableScreenSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}

	// Height of a flush candidate awaiting confirmation, or -1 for none.
	pendingHeight := -1

	for i := 0; i < maxScrolls && time.Now().Before(deadline); i++ {
		_, info, err := d.findElement(step.Element, true, 1000)
		if err == nil && info != nil {
			// The DeviceLab agent returns matches from the full view hierarchy,
			// including items below the fold in a ScrollView — and a match
			// half-hidden at the screen edge is no better, because the tap that
			// follows lands wrong. Stop only when the element meets the flow's
			// visibility requirement (default: fully inside the viewport, which
			// here is the full physical display — see tappableScreenSize above).
			if core.MeetsVisibility(info.Bounds, width, height, step.VisibilityPercentage) {
				// The geometry says fully visible, but it is computed from a
				// rect the hierarchy may already have clipped to the scroll
				// container — a sliver at the fold scores 100% (#164). A rect
				// flush with the container's leading edge might be truncated,
				// so scroll once and look again: a sliver grows, an element
				// resting at the end of the list does not.
				if pendingHeight >= 0 && info.Bounds.Height <= pendingHeight {
					return successResult(fmt.Sprintf("Element found after %d scrolls", i), info)
				}
				if !d.atScrollContainerEdge(info.Bounds, direction) {
					return successResult(fmt.Sprintf("Element found after %d scrolls", i), info)
				}
				pendingHeight = info.Bounds.Height
			}
		} else if err != nil && !isElementNotFoundError(err) {
			// Infrastructure failure (dead session, connection refused, etc.):
			// surface immediately rather than silently looping through all scrolls.
			return errorResult(err, "Failed to find element")
		}

		if err := d.performScroll(direction, width, height, step.Engine, 0.3); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to scroll: %v", err))
		}

		time.Sleep(300 * time.Millisecond)
	}

	return errorResult(fmt.Errorf("element not found"), fmt.Sprintf("Element not found after %d scrolls", maxScrolls))
}

// performScroll dispatches a scroll gesture using the engine selected by the
// step. Default ("" or "adb") uses adb input swipe (matches upstream Maestro
// and is the most reliable path across Android skins). "agent" uses the
// on-device DeviceLab agent's MotionEvent injection. ADB falls back to the
// agent (with a warning) when no shell executor is available.
// percent controls the swipe distance as a fraction of screen dimension.
func (d *Driver) performScroll(direction string, width, height int, engine string, percent float64) error {
	useAgent := strings.EqualFold(engine, "agent")
	if !useAgent {
		if d.device != nil {
			return d.scrollByAdb(direction, width, height, percent)
		}
		logger.Warn("scroll: ADB shell unavailable, falling back to agent gesture (may be unreliable on some Android skins)")
	}
	area := uiautomator2.NewRect(0, height/8, width, height*3/4)
	return d.client.ScrollInArea(area, direction, percent, scrollDurationMs)
}

// scrollByAdb issues `adb shell input swipe` over the local shell executor.
// percent is the swipe distance as a fraction of the screen dimension along
// the scroll axis. Direction uses Maestro scroll semantics (what becomes
// visible — "down" reveals content below by swiping the finger UP).
func (d *Driver) scrollByAdb(direction string, screenWidth, screenHeight int, percent float64) error {
	centerX := screenWidth / 2
	centerY := screenHeight / 2
	halfV := int(float64(screenHeight) * percent / 2)
	halfH := int(float64(screenWidth) * percent / 2)
	var fromX, fromY, toX, toY int
	switch direction {
	case "up":
		fromX, fromY = centerX, centerY-halfV
		toX, toY = centerX, centerY+halfV
	case "down":
		fromX, fromY = centerX, centerY+halfV
		toX, toY = centerX, centerY-halfV
	case "left":
		fromX, fromY = centerX-halfH, centerY
		toX, toY = centerX+halfH, centerY
	case "right":
		fromX, fromY = centerX+halfH, centerY
		toX, toY = centerX-halfH, centerY
	default:
		fromX, fromY = centerX, centerY+halfV
		toX, toY = centerX, centerY-halfV
	}
	cmd := fmt.Sprintf("input swipe %d %d %d %d %d", fromX, fromY, toX, toY, scrollDurationMs)
	_, err := d.device.Shell(cmd)
	return err
}

// scrollDurationMs is the swipe duration (in ms) passed to the DeviceLab
// agent for scroll gestures. The agent uses it as the total MotionEvent
// duration; values that are too low (including 0) emit events too fast for
// Android's input pipeline to register as a scroll.
const scrollDurationMs = 300

// isElementNotFoundError distinguishes expected "not on screen yet" errors
// (which scrollUntilVisible should swallow and keep scrolling) from real
// infrastructure failures that should propagate immediately.
func isElementNotFoundError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, phrase := range []string{
		"not found",
		"no elements match",
		"no such element",
		"could not be located",
		"context deadline exceeded",
	} {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}

func (d *Driver) swipe(step *flow.SwipeStep) *core.CommandResult {
	if step.Start != "" && step.End != "" {
		return d.swipeWithCoordinates(step.Start, step.End, step.Duration)
	}

	if step.StartX > 0 || step.StartY > 0 || step.EndX > 0 || step.EndY > 0 {
		return d.swipeWithAbsoluteCoords(step.StartX, step.StartY, step.EndX, step.EndY, step.Duration)
	}

	direction, err := core.NormalizeSwipeDirection(step.Direction)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid swipe direction: %s", step.Direction))
	}

	// If a from:/selector element is specified, derive swipe coordinates from
	// the element's bounds and route through the same ADB `input swipe` path
	// used by screen-percentage swipes. `SwipeInArea` (the previous path) does
	// not honor `step.Duration`, producing a fast flick — too fast for native
	// drag targets (sliders, drag handles), which discard the gesture. This
	// mirrors the uiautomator2 fix for #114.
	if step.Selector != nil && !step.Selector.IsEmpty() {
		_, info, err := d.findElement(*step.Selector, step.IsOptional(), step.TimeoutMs)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Element not found for swipe: %v", err))
		}
		if info != nil && info.Bounds.Width > 0 {
			screenW, screenH, _ := d.screenSize() // (0,0) when unknown → far-edge clamp skipped
			startX, startY, endX, endY, err := core.SwipeCoordsForElement(
				direction, info.Bounds, screenW, screenH, step.Distance, step.Selector.Point)
			if err != nil {
				return errorResult(err, fmt.Sprintf("Invalid swipe direction: %s", step.Direction))
			}
			return d.swipeWithAbsoluteCoords(startX, startY, endX, endY, step.Duration)
		}
	}

	// No selector — use screen coordinates directly (matches Maestro behavior)
	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, "Failed to get screen size")
	}
	// An explicit `distance:` controls how far the centered swipe travels.
	if step.Distance > 0 {
		sx, sy, ex, ey, derr := core.DirectionSwipeScreenCoords(direction, width, height, step.Distance)
		if derr != nil {
			return errorResult(derr, fmt.Sprintf("Invalid swipe direction: %s", step.Direction))
		}
		return d.swipeWithAbsoluteCoords(sx, sy, ex, ey, step.Duration)
	}
	return d.swipeWithMaestroCoordinates(direction, width, height, step.Duration)
}

// findScrollableElement waits for and finds a scrollable element.
func (d *Driver) findScrollableElement(timeoutMs int) (*core.ElementInfo, int) {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		source, err := d.client.Source()
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		elements, err := ParsePageSource(source)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		scrollables := FilterScrollable(elements)

		if len(scrollables) == 1 {
			elem := scrollables[0]
			return &core.ElementInfo{
				Bounds: elem.Bounds,
			}, 1
		}

		if len(scrollables) > 1 {
			largest := FindLargestScrollable(elements)
			if largest != nil {
				return &core.ElementInfo{
					Bounds: largest.Bounds,
				}, len(scrollables)
			}
		}

		time.Sleep(pollInterval)
	}

	return nil, 0
}

// Swipe coordinates match Maestro Android behavior:
// UP:    50%,50% → 50%,10%
// DOWN:  50%,20% → 50%,90%
// LEFT:  90%,50% → 10%,50%
// RIGHT: 10%,50% → 90%,50%
func (d *Driver) swipeWithMaestroCoordinates(direction string, width, height, durationMs int) *core.CommandResult {
	var startX, startY, endX, endY int

	switch direction {
	case "up":
		startX = width * 50 / 100
		startY = height * 50 / 100
		endX = width * 50 / 100
		endY = height * 10 / 100
	case "down":
		startX = width * 50 / 100
		startY = height * 20 / 100
		endX = width * 50 / 100
		endY = height * 90 / 100
	case "left":
		startX = width * 90 / 100
		startY = height * 50 / 100
		endX = width * 10 / 100
		endY = height * 50 / 100
	case "right":
		startX = width * 10 / 100
		startY = height * 50 / 100
		endX = width * 90 / 100
		endY = height * 50 / 100
	default:
		startX = width * 50 / 100
		startY = height * 50 / 100
		endX = width * 50 / 100
		endY = height * 10 / 100
	}

	fmt.Printf("[swipe] Using screen coords: (%d,%d) → (%d,%d)\n", startX, startY, endX, endY)
	return d.swipeWithAbsoluteCoords(startX, startY, endX, endY, durationMs)
}

func (d *Driver) swipeWithCoordinates(start, end string, durationMs int) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "swipe with coordinates requires device access")
	}

	width, height, err := d.screenSize()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get screen size: %v", err))
	}

	startXPct, startYPct, err := core.ParsePercentageCoords(start)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid start coordinates: %v", err))
	}

	endXPct, endYPct, err := core.ParsePercentageCoords(end)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Invalid end coordinates: %v", err))
	}

	startX := int(float64(width) * startXPct)
	startY := int(float64(height) * startYPct)
	endX := int(float64(width) * endXPct)
	endY := int(float64(height) * endYPct)

	return d.swipeWithAbsoluteCoords(startX, startY, endX, endY, durationMs)
}

// swipeWithAbsoluteCoords runs a swipe between two screen points.
//
// Prefers the agent's in-process injection over `adb shell input swipe`.
// The shell command always lifts the pointer at speed, so the view flings and
// the distance scrolled depends on momentum computed from timings that shift
// with machine load — the same flow then scrolls differently locally and on CI
// (#141). The agent primes the touch slop and holds the pointer still before
// lifting, neither of which `input swipe` can express. ADB stays as the
// fallback for when the agent isn't reachable.
func (d *Driver) swipeWithAbsoluteCoords(startX, startY, endX, endY, durationMs int) *core.CommandResult {
	if durationMs <= 0 {
		durationMs = 300
	}

	if d.client != nil {
		if err := d.client.SwipeCoords(startX, startY, endX, endY, durationMs); err == nil {
			return successResult(fmt.Sprintf("Swiped from (%d,%d) to (%d,%d)", startX, startY, endX, endY), nil)
		} else if d.device == nil {
			return errorResult(err, fmt.Sprintf("Failed to swipe: %v", err))
		} else {
			logger.Warn("[devicelab] agent swipe failed, falling back to adb: %v", err)
		}
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "swipe with coordinates requires device access")
	}

	cmd := fmt.Sprintf("input swipe %d %d %d %d %d", startX, startY, endX, endY, durationMs)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to swipe: %v", err))
	}

	return successResult(fmt.Sprintf("Swiped from (%d,%d) to (%d,%d)", startX, startY, endX, endY), nil)
}

// ============================================================================
// Navigation Commands
// ============================================================================

func (d *Driver) back(_ *flow.BackStep) *core.CommandResult {
	if err := d.client.Back(); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to press back: %v", err))
	}

	return successResult("Pressed back", nil)
}

func (d *Driver) pressKey(step *flow.PressKeyStep) *core.CommandResult {
	key := step.Key
	keyCode := mapKeyCode(key)
	if keyCode == 0 {
		return errorResult(fmt.Errorf("unknown key: %s", key), fmt.Sprintf("Unknown key: %s", key))
	}

	if err := d.client.PressKeyCode(keyCode); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to press key: %v", err))
	}

	return successResult(fmt.Sprintf("Pressed key: %s", key), nil)
}

func (d *Driver) openNotifications(_ *flow.OpenNotificationsStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("no shell executor"), "openNotifications requires shell access")
	}
	if _, err := d.device.Shell("cmd statusbar expand-notifications"); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open notification shade: %v", err))
	}
	return successResult("Opened notification shade", nil)
}

// ============================================================================
// App Lifecycle Commands
// ============================================================================

func (d *Driver) launchApp(step *flow.LaunchAppStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		return errorResult(fmt.Errorf("no appId specified"), "launchApp: no appId specified in flow")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "launchApp: no device connected — check ADB connection")
	}
	d.currentAppID = appID // remember for mid-flow crash detection

	// Forget deaths recorded before this launch: exit-info persists across
	// runs, so an earlier crash — or the force-stop below — would otherwise be
	// reported as this flow's.
	d.clearExitHistory(appID)

	// 1. Clear state or force-stop via RPC (no USB round-trips)
	if step.ClearState {
		if err := d.client.ClearAppData(appID); err != nil {
			// RPC failed — fall back to shell
			logger.Warn("launchApp: RPC clearAppData failed for %s: %v — falling back to shell", appID, err)
			if _, shellErr := d.device.Shell("pm clear " + appID); shellErr != nil {
				return errorResult(shellErr, fmt.Sprintf("launchApp: failed to clear app state for '%s' — is the app installed?", appID))
			}
		}
	} else if step.StopApp == nil || *step.StopApp {
		if err := d.client.ForceStop(appID); err != nil {
			logger.Warn("launchApp: RPC forceStop failed for %s: %v — falling back to shell", appID, err)
			if _, shellErr := d.device.Shell("am force-stop " + appID); shellErr != nil {
				logger.Warn("failed to force-stop app %s: %v", appID, shellErr)
			}
		}
	}

	// 2. Permissions via RPC
	permissions := step.Permissions
	if len(permissions) == 0 {
		permissions = map[string]string{"all": "allow"}
	}
	var toGrant []string
	for name, value := range permissions {
		if strings.ToLower(value) != "allow" {
			continue
		}
		if strings.ToLower(name) == "all" {
			toGrant = append(toGrant, getAllPermissions()...)
		} else {
			toGrant = append(toGrant, resolvePermissionShortcut(name)...)
		}
	}
	if len(toGrant) > 0 {
		if err := d.client.GrantPermissions(appID, toGrant); err != nil {
			logger.Warn("launchApp: RPC grantPermissions failed for %s: %v — falling back to shell", appID, err)
			d.applyPermissions(appID, permissions)
		}
	}

	// 3. Launch via RPC
	var arguments map[string]interface{}
	if len(step.Arguments) > 0 {
		arguments = step.Arguments
	}
	if err := d.client.LaunchApp(appID, arguments); err != nil {
		// RPC launch failed — fall back to shell
		logger.Warn("launchApp: RPC launch failed for %s: %v — falling back to shell", appID, err)
		return d.launchAppViaShell(appID, arguments)
	}

	return successResult(fmt.Sprintf("Launched app: %s", appID), nil)
}

// launchAppViaShell launches an app using ADB shell commands.
func (d *Driver) launchAppViaShell(appID string, arguments map[string]interface{}) *core.CommandResult {
	apiLevel := d.getAPILevel()

	if apiLevel < 24 && len(arguments) == 0 {
		return d.launchWithMonkey(appID)
	}

	activity, err := d.resolveLauncherActivityCached(appID, apiLevel)
	if err != nil {
		if len(arguments) == 0 {
			logger.Warn("launchApp: activity resolution failed for %s: %v — trying monkey", appID, err)
			return d.launchWithMonkey(appID)
		}
		return errorResult(err, fmt.Sprintf(
			"launchApp: cannot find launcher activity for '%s' — %v. "+
				"Is the app installed? Check with: adb shell pm list packages | grep %s", appID, err, appID))
	}

	amCmd := "am start"
	if apiLevel >= 26 {
		amCmd = "am start-activity"
	}

	cmd := fmt.Sprintf("%s -W -n %s -a android.intent.action.MAIN -c android.intent.category.LAUNCHER -f 0x10200000",
		amCmd, activity)

	// String values are quoted because they are free text from the flow — the
	// numeric and boolean cases render from typed Go values and cannot carry
	// shell syntax.
	for key, value := range arguments {
		k := core.ShellQuote(key)
		switch v := value.(type) {
		case string:
			cmd += fmt.Sprintf(" --es %s %s", k, core.ShellQuote(v))
		case int:
			cmd += fmt.Sprintf(" --ei %s %d", k, v)
		case int64:
			cmd += fmt.Sprintf(" --ei %s %d", k, v)
		case float64:
			if v == float64(int(v)) {
				cmd += fmt.Sprintf(" --ei %s %d", k, int(v))
			} else {
				cmd += fmt.Sprintf(" --ef %s %f", k, v)
			}
		case bool:
			cmd += fmt.Sprintf(" --ez %s %t", k, v)
		default:
			cmd += fmt.Sprintf(" --es %s %s", k, core.ShellQuote(fmt.Sprintf("%v", v)))
		}
	}

	output, err := d.device.Shell(cmd)
	if err != nil || strings.Contains(output, "Error") {
		if strings.Contains(output, "does not exist") || strings.Contains(output, "ClassNotFoundException") {
			dotActivity := d.addDotPrefix(activity)
			if dotActivity != activity {
				logger.Info("launchApp: retrying with dot-prefixed activity: %s", dotActivity)
				retryCmd := strings.Replace(cmd, activity, dotActivity, 1)
				if output2, err2 := d.device.Shell(retryCmd); err2 == nil && !strings.Contains(output2, "Error") {
					return successResult(fmt.Sprintf("Launched app: %s", appID), nil)
				}
			}
		}

		if len(arguments) == 0 {
			logger.Warn("launchApp: am start failed for %s: %v — trying monkey", appID, err)
			return d.launchWithMonkey(appID)
		}
		errMsg := fmt.Sprintf("launchApp: '%s' failed for '%s' activity '%s'", amCmd, appID, activity)
		if err != nil {
			return errorResult(err, errMsg)
		}
		return errorResult(fmt.Errorf("am start returned error: %s", strings.TrimSpace(output)), errMsg)
	}

	return successResult(fmt.Sprintf("Launched app: %s", appID), nil)
}

// getAPILevel returns the device's Android API level, or 24 as a safe default.
func (d *Driver) getAPILevel() int {
	if d.cachedAPILevel > 0 {
		return d.cachedAPILevel
	}
	output, err := d.device.Shell("getprop ro.build.version.sdk")
	if err != nil {
		return 24
	}
	level, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 24
	}
	d.cachedAPILevel = level
	return level
}

// resolveLauncherActivityCached resolves the launcher activity with caching.
func (d *Driver) resolveLauncherActivityCached(appID string, apiLevel int) (string, error) {
	if d.cachedActivities != nil {
		if activity, ok := d.cachedActivities[appID]; ok {
			return activity, nil
		}
	}
	activity, err := d.resolveLauncherActivity(appID, apiLevel)
	if err != nil {
		return "", err
	}
	if d.cachedActivities == nil {
		d.cachedActivities = make(map[string]string)
	}
	d.cachedActivities[appID] = activity
	return activity, nil
}

// resolveLauncherActivity resolves the launcher activity for a package.
func (d *Driver) resolveLauncherActivity(appID string, apiLevel int) (string, error) {
	if apiLevel >= 24 {
		resolveCmd := fmt.Sprintf("cmd package resolve-activity --brief -a android.intent.action.MAIN -c android.intent.category.LAUNCHER %s | tail -n 1", appID)
		output, err := d.device.Shell(resolveCmd)
		if err == nil {
			activity := strings.TrimSpace(output)
			if activity != "" &&
				!strings.Contains(activity, "No activity found") &&
				!strings.Contains(activity, "ResolverActivity") &&
				strings.Contains(activity, "/") {
				return activity, nil
			}
		}
	}

	return d.resolveLauncherFromDumpsys(appID)
}

// launchWithMonkey launches an app using the monkey command.
func (d *Driver) launchWithMonkey(appID string) *core.CommandResult {
	monkeyCmd := fmt.Sprintf("monkey -p %s -c android.intent.category.LAUNCHER 1", appID)
	output, err := d.device.Shell(monkeyCmd)
	if err != nil || strings.Contains(output, "monkey aborted") {
		errMsg := fmt.Sprintf("launchApp: all launch methods failed for '%s'. "+
			"The app may not be installed or has no launcher activity. "+
			"Check with: adb shell pm list packages | grep %s", appID, appID)
		if err != nil {
			return errorResult(err, errMsg)
		}
		return errorResult(fmt.Errorf("monkey aborted — no launchable activity for %s", appID), errMsg)
	}
	return successResult(fmt.Sprintf("Launched app: %s", appID), nil)
}

// addDotPrefix converts "com.app/MainActivity" to "com.app/.MainActivity".
func (d *Driver) addDotPrefix(activity string) string {
	parts := strings.SplitN(activity, "/", 2)
	if len(parts) != 2 {
		return activity
	}
	activityName := parts[1]
	if strings.HasPrefix(activityName, ".") || strings.Contains(activityName, ".") {
		return activity
	}
	return parts[0] + "/." + activityName
}

// resolveLauncherFromDumpsys parses `dumpsys package` output to find the MAIN/LAUNCHER activity.
func (d *Driver) resolveLauncherFromDumpsys(appID string) (string, error) {
	output, err := d.device.Shell(fmt.Sprintf("dumpsys package %s", appID))
	if err != nil {
		return "", fmt.Errorf("dumpsys failed for %s: %w", appID, err)
	}

	lines := strings.Split(output, "\n")
	inFilter := false
	hasMain := false
	hasLauncher := false
	var currentActivity string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, appID) && strings.Contains(trimmed, "/") && strings.Contains(trimmed, "filter") {
			if inFilter && hasMain && hasLauncher && currentActivity != "" {
				return currentActivity, nil
			}
			inFilter = true
			hasMain = false
			hasLauncher = false
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				currentActivity = parts[0]
			}
			continue
		}

		if inFilter {
			if strings.Contains(trimmed, "android.intent.action.MAIN") {
				hasMain = true
			}
			if strings.Contains(trimmed, "android.intent.category.LAUNCHER") {
				hasLauncher = true
			}
			if trimmed == "" || (!strings.HasPrefix(trimmed, "Action:") &&
				!strings.HasPrefix(trimmed, "Category:") &&
				!strings.HasPrefix(trimmed, "\"")) {
				if hasMain && hasLauncher && currentActivity != "" {
					return currentActivity, nil
				}
				inFilter = false
			}
		}
	}

	if hasMain && hasLauncher && currentActivity != "" {
		return currentActivity, nil
	}

	return "", fmt.Errorf("no MAIN/LAUNCHER activity found in dumpsys for %s", appID)
}

func (d *Driver) stopApp(step *flow.StopAppStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		return errorResult(fmt.Errorf("no appId specified"), "No app ID to stop")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "stopApp requires device access")
	}

	if _, err := d.device.Shell("am force-stop " + appID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to stop app: %v", err))
	}

	return successResult(fmt.Sprintf("Stopped app: %s", appID), nil)
}

func (d *Driver) clearState(step *flow.ClearStateStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		return errorResult(fmt.Errorf("no appId specified"), "No app ID to clear")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "clearState requires device access")
	}

	if _, err := d.device.Shell("pm clear " + appID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to clear state: %v", err))
	}

	return successResult(fmt.Sprintf("Cleared state for: %s", appID), nil)
}

func (d *Driver) killApp(step *flow.KillAppStep) *core.CommandResult {
	appID := step.AppID
	if appID == "" {
		return errorResult(fmt.Errorf("no appId specified"), "No app ID to kill")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "killApp requires device access")
	}

	if _, err := d.device.Shell("am force-stop " + appID); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to kill app: %v", err))
	}

	return successResult(fmt.Sprintf("Killed app: %s", appID), nil)
}

// applyPermissions applies permission settings to an app.
func (d *Driver) applyPermissions(appID string, permissions map[string]string) *core.CommandResult {
	var toGrant, toRevoke []string

	for name, value := range permissions {
		var perms []string
		if strings.ToLower(name) == "all" {
			perms = getAllPermissions()
		} else {
			perms = resolvePermissionShortcut(name)
		}

		switch strings.ToLower(value) {
		case "allow":
			toGrant = append(toGrant, perms...)
		case "deny", "unset":
			toRevoke = append(toRevoke, perms...)
		}
	}

	// Batched into one shell round trip, but no longer with stderr thrown away:
	// discarding it meant a permission the app never declared looked identical
	// to one that applied, and setPermissions reported success either way.
	run := func(verb string, perms []string) {
		if len(perms) == 0 {
			return
		}
		var parts []string
		for _, perm := range perms {
			parts = append(parts, fmt.Sprintf("pm %s %s %s", verb, appID, perm))
		}
		out, err := d.device.Shell(strings.Join(parts, "; "))
		if err == nil && !strings.Contains(out, "Exception") {
			return
		}
		if core.IsUndeclaredPermissionError(out) {
			logger.Warn("setPermissions: %s does not declare one or more of %v — skipping those", appID, perms)
			return
		}
		logger.Warn("setPermissions: pm %s reported: %s", verb, strings.TrimSpace(out))
	}
	run("grant", toGrant)
	run("revoke", toRevoke)

	return successResult(fmt.Sprintf("Permissions updated: %d granted, %d revoked", len(toGrant), len(toRevoke)), nil)
}

// resolvePermissionShortcut maps Maestro permission shortcuts to Android permission names.
// resolvePermissionShortcut maps a flow's permission name to Android permission
// strings. The table is shared with the other drivers in pkg/core — it used to
// be duplicated per driver, and the copies drifted (#148).
func resolvePermissionShortcut(shortcut string) []string {
	return core.AndroidPermissionShortcut(shortcut)
}

func getAllPermissions() []string {
	return []string{
		"android.permission.ACCESS_FINE_LOCATION",
		"android.permission.ACCESS_COARSE_LOCATION",
		"android.permission.ACCESS_BACKGROUND_LOCATION",
		"android.permission.CAMERA",
		"android.permission.READ_CONTACTS",
		"android.permission.WRITE_CONTACTS",
		"android.permission.GET_ACCOUNTS",
		"android.permission.READ_PHONE_STATE",
		"android.permission.CALL_PHONE",
		"android.permission.READ_CALL_LOG",
		"android.permission.WRITE_CALL_LOG",
		"android.permission.RECORD_AUDIO",
		"android.permission.READ_EXTERNAL_STORAGE",
		"android.permission.WRITE_EXTERNAL_STORAGE",
		"android.permission.READ_MEDIA_IMAGES",
		"android.permission.READ_MEDIA_VIDEO",
		"android.permission.READ_MEDIA_AUDIO",
		"android.permission.READ_CALENDAR",
		"android.permission.WRITE_CALENDAR",
		"android.permission.SEND_SMS",
		"android.permission.RECEIVE_SMS",
		"android.permission.READ_SMS",
		"android.permission.POST_NOTIFICATIONS",
		"android.permission.BLUETOOTH_CONNECT",
		"android.permission.BLUETOOTH_SCAN",
		"android.permission.BODY_SENSORS",
		"android.permission.ACTIVITY_RECOGNITION",
	}
}

// ============================================================================
// Clipboard Commands
// ============================================================================

func (d *Driver) copyTextFrom(step *flow.CopyTextFromStep) *core.CommandResult {
	elem, info, err := d.findElement(step.Selector, step.IsOptional(), step.TimeoutMs)
	if err != nil {
		return errorResult(err, fmt.Sprintf("Element not found: %v", err))
	}

	var text string
	if elem != nil {
		text, err = elem.Text()
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to get text: %v", err))
		}
		if text == "" {
			if desc, descErr := elem.Attribute("content-desc"); descErr == nil && desc != "" {
				text = desc
			}
		}
		// Cached elements can't serve content-desc over the wire; the
		// hierarchy snapshot already carries it.
		if text == "" && info != nil {
			text = info.AccessibilityLabel
		}
	} else if info != nil {
		text = info.Text
		if text == "" && info.AccessibilityLabel != "" {
			text = info.AccessibilityLabel
		}
	}

	if err := d.client.SetClipboard(text); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to copy to clipboard: %v", err))
	}

	return &core.CommandResult{
		Success: true,
		Message: fmt.Sprintf("Copied text: %s", text),
		Element: info,
		Data:    text,
	}
}

func (d *Driver) pasteText(_ *flow.PasteTextStep) *core.CommandResult {
	text, err := d.client.GetClipboard()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get clipboard: %v", err))
	}

	focused, err := d.findFocused()
	if err != nil {
		return errorResult(err, "No focused element to paste into")
	}

	if err := focused.Input(text); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to paste text: %v", err))
	}

	return successResult(fmt.Sprintf("Pasted text: %s", text), nil)
}

func (d *Driver) setClipboard(step *flow.SetClipboardStep) *core.CommandResult {
	if step.Text == "" {
		return errorResult(fmt.Errorf("no text specified"), "setClipboard requires text")
	}

	if err := d.client.SetClipboard(step.Text); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set clipboard: %v", err))
	}

	return successResult(fmt.Sprintf("Set clipboard to: %s", step.Text), nil)
}

// ============================================================================
// Device Control Commands
// ============================================================================

func (d *Driver) setOrientation(step *flow.SetOrientationStep) *core.CommandResult {
	orientation := strings.ToUpper(strings.ReplaceAll(step.Orientation, "_", ""))

	if orientation == "PORTRAIT" || orientation == "LANDSCAPE" {
		if err := d.client.SetOrientation(orientation); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to set orientation: %v", err))
		}
		return successResult(fmt.Sprintf("Set orientation to %s", orientation), nil)
	}

	var rotation string
	switch orientation {
	case "LANDSCAPELEFT":
		rotation = "1"
	case "UPSIDEDOWN":
		rotation = "2"
	case "LANDSCAPERIGHT":
		rotation = "3"
	default:
		return errorResult(fmt.Errorf("invalid orientation: %s", step.Orientation),
			fmt.Sprintf("Orientation must be PORTRAIT, LANDSCAPE, LANDSCAPE_LEFT, LANDSCAPE_RIGHT, or UPSIDE_DOWN, got: %s", step.Orientation))
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "Extended orientations require device access")
	}

	if _, err := d.device.Shell("settings put system accelerometer_rotation 0"); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to disable accelerometer rotation: %v", err))
	}

	cmd := fmt.Sprintf("settings put system user_rotation %s", rotation)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set orientation: %v", err))
	}

	return successResult(fmt.Sprintf("Set orientation to %s", step.Orientation), nil)
}

func (d *Driver) openLink(step *flow.OpenLinkStep) *core.CommandResult {
	link := step.Link
	if link == "" {
		return errorResult(fmt.Errorf("no link specified"), "No link to open")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "openLink requires device access")
	}

	quoted := core.ShellQuote(link)
	var cmd string
	if step.Browser != nil && *step.Browser {
		cmd = fmt.Sprintf("am start -a android.intent.action.VIEW -c android.intent.category.BROWSABLE -d %s", quoted)
	} else {
		cmd = fmt.Sprintf("am start -a android.intent.action.VIEW -d %s", quoted)
	}

	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open link: %v", err))
	}

	// In browser mode, openLink opens a new Chrome tab. The old CDP page connection
	// is now stale (points to the previous tab). Disconnect so ensureWebViewConnection()
	// reconnects to the new page via HTTP /json on the next operation.
	if d.isBrowserMode() && d.webView != nil {
		logger.Info("[browser] openLink: disconnecting CDP to reconnect to new tab")
		d.webView.disconnect()
		// Keep knownCDPType — we're still in browser mode, just need a fresh page
		// Give Chrome a moment to register the new tab
		time.Sleep(500 * time.Millisecond)
	}

	if step.AutoVerify != nil && *step.AutoVerify {
		time.Sleep(2 * time.Second)
	}

	return successResult(fmt.Sprintf("Opened link: %s", link), nil)
}

// ============================================================================
// Media Commands
// ============================================================================

func (d *Driver) takeScreenshot(step *flow.TakeScreenshotStep) *core.CommandResult {
	data, err := d.client.Screenshot()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to take screenshot: %v", err))
	}

	if step.CropOn != nil {
		_, info, findErr := d.findElement(*step.CropOn, false, 0)
		if findErr != nil || info == nil {
			return errorResult(findErr, fmt.Sprintf("cropOn: element not found: %v", findErr))
		}
		sw, sh, dimErr := d.screenSize()
		if dimErr != nil {
			return errorResult(dimErr, "cropOn requires screen dimensions")
		}
		cropped, cropErr := core.CropScreenshot(data, info.Bounds, sw, sh)
		if cropErr != nil {
			return errorResult(cropErr, fmt.Sprintf("cropOn: %v", cropErr))
		}
		data = cropped
	}

	return &core.CommandResult{
		Success: true,
		Message: "Screenshot captured",
		Data:    data,
	}
}

func (d *Driver) openBrowser(step *flow.OpenBrowserStep) *core.CommandResult {
	url := step.URL
	if url == "" {
		return errorResult(fmt.Errorf("no URL specified"), "No URL to open")
	}

	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "openBrowser requires device access")
	}

	cmd := fmt.Sprintf("am start -a android.intent.action.VIEW -d %s", core.ShellQuote(url))
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to open browser: %v", err))
	}

	return successResult(fmt.Sprintf("Opened browser: %s", url), nil)
}

func (d *Driver) addMedia(step *flow.AddMediaStep) *core.CommandResult {
	if err := core.ValidateMediaFiles(step.Files); err != nil {
		return errorResult(err, err.Error())
	}

	// Stream each file's bytes to the on-device agent, which inserts it into
	// MediaStore (ContentResolver, IS_PENDING flow) so the app's picker/gallery
	// can select it. This is the correct path on API 29+ scoped storage — the
	// old MEDIA_SCANNER_SCAN_FILE broadcast is deprecated and doesn't register
	// media for the modern photo picker.
	for _, file := range step.Files {
		data, err := os.ReadFile(file)
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to read media file %s: %v", file, err))
		}
		mime, _ := core.MediaMIMEType(file)
		if err := d.client.AddMedia(filepath.Base(file), mime, data); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to add media %s: %v", filepath.Base(file), err))
		}
	}

	return successResult(fmt.Sprintf("Added %d media file(s)", len(step.Files)), nil)
}

func (d *Driver) removeMedia(_ *flow.RemoveMediaStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "removeMedia requires device access")
	}

	var lastErr error
	cleared := false
	for _, pkg := range []string{
		"com.google.android.providers.media.module",
		"com.android.providers.media",
	} {
		if _, err := d.device.Shell("pm clear " + pkg); err == nil {
			cleared = true
		} else {
			lastErr = err
		}
	}
	if !cleared {
		return errorResult(lastErr, fmt.Sprintf("Failed to clear media providers: %v", lastErr))
	}

	return successResult("Cleared MediaStore index", nil)
}

func (d *Driver) startRecording(step *flow.StartRecordingStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "startRecording requires device access")
	}

	path := step.Path
	if path == "" {
		path = "/sdcard/recording.mp4"
	}

	cmd := fmt.Sprintf("screenrecord %s </dev/null >/dev/null 2>&1 &", path)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to start recording: %v", err))
	}

	return &core.CommandResult{
		Success: true,
		Message: fmt.Sprintf("Started recording to %s", path),
		Data:    path,
	}
}

func (d *Driver) stopRecording(_ *flow.StopRecordingStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "stopRecording requires device access")
	}

	if _, err := d.device.Shell("pkill -INT screenrecord"); err != nil {
		logger.Warn("failed to stop screenrecord process: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	return successResult("Stopped recording", nil)
}

// ============================================================================
// Wait Commands
// ============================================================================

func (d *Driver) waitUntil(step *flow.WaitUntilStep) *core.CommandResult {
	timeoutMs := 30000
	if step.TimeoutMs > 0 {
		timeoutMs = step.TimeoutMs
	}

	var selector *flow.Selector
	waitingForVisible := step.Visible != nil
	if waitingForVisible {
		selector = step.Visible
	} else {
		selector = step.NotVisible
	}

	// Browser mode: use JS RAF-based polling (60fps in-browser, single CDP call).
	if d.isBrowserMode() && d.webView != nil && d.webView.isConnected() {
		if waitingForVisible {
			return d.assertVisibleBrowser(*selector, timeoutMs)
		}
		return d.assertNotVisibleBrowser(*selector, timeoutMs)
	}

	timeout := time.Duration(timeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(d.parentContext(), timeout)
	defer cancel()

	// extendedWaitUntil intentionally does NOT use lazy retry: the user
	// explicitly asked for a long wait, so re-tapping the previous step's
	// target would corrupt state for persistent buttons (e.g. Preload
	// Details which stays on screen but triggers an async update — we
	// saw this break Native Stack - Preload Flow with triple-taps).
	for {
		select {
		case <-ctx.Done():
			if waitingForVisible {
				return errorResult(
					context.DeadlineExceeded,
					fmt.Sprintf("Element '%s' not visible within %v", selector.Describe(), timeout),
				)
			}
			return errorResult(
				context.DeadlineExceeded,
				fmt.Sprintf("Element '%s' still visible after %v", selector.Describe(), timeout),
			)
		default:
			if waitingForVisible {
				_, info, err := d.findElementOnce(*step.Visible)
				if err == nil && info != nil {
					return successResult("Element is now visible", info)
				}
			} else {
				_, info, err := d.findElementOnce(*step.NotVisible)
				if err != nil || info == nil {
					return successResult("Element is no longer visible", nil)
				}
			}
		}
	}
}

func (d *Driver) waitForAnimationToEnd(step *flow.WaitForAnimationToEndStep) *core.CommandResult {
	timeoutMs := step.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 15000
	}
	const threshold = 0.005

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	start := time.Now()
	for time.Now().Before(deadline) {
		prev, err := d.client.Screenshot()
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to take screenshot: %v", err))
		}
		curr, err := d.client.Screenshot()
		if err != nil {
			return errorResult(err, fmt.Sprintf("Failed to take screenshot: %v", err))
		}
		diff := core.ImageDifference(prev, curr)
		if diff <= threshold {
			elapsed := time.Since(start)
			return successResult(fmt.Sprintf("Animation ended (%.1f%% diff, %dms)", diff*100, elapsed.Milliseconds()), nil)
		}
	}

	return successResult(fmt.Sprintf("Animation did not settle within %dms — continuing", timeoutMs), nil)
}

// ============================================================================
// Location Commands
// ============================================================================

func (d *Driver) setLocation(step *flow.SetLocationStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "setLocation requires device access")
	}

	lat := step.Latitude
	lon := step.Longitude
	if lat == "" || lon == "" {
		return errorResult(fmt.Errorf("latitude and longitude required"), "Missing coordinates")
	}

	cmd := fmt.Sprintf("am broadcast -a android.intent.action.MOCK_LOCATION --ef lat %s --ef lon %s", lat, lon)
	if _, err := d.device.Shell(cmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set location: %v", err))
	}

	return successResult(fmt.Sprintf("Set location to %s, %s", lat, lon), nil)
}

// applyAirplaneMode sets airplane mode on/off using the best available method.
// Android 11+ (API 30+): "cmd connectivity airplane-mode" works without root.
// Older Android: "settings put global" + broadcast fallback.
func (d *Driver) applyAirplaneMode(enable bool) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "setAirplaneMode requires device access")
	}

	mode := "disable"
	status := "disabled"
	if enable {
		mode = "enable"
		status = "enabled"
	}

	// Try "cmd connectivity airplane-mode" first (Android 11+ / API 30+)
	cmdStr := fmt.Sprintf("cmd connectivity airplane-mode %s", mode)
	out, err := d.device.Shell(cmdStr)
	if err == nil && !strings.Contains(out, "Unknown command") {
		return successResult(fmt.Sprintf("Airplane mode %s", status), nil)
	}

	// Fallback: settings put + broadcast (older Android)
	value := "0"
	if enable {
		value = "1"
	}
	settingsCmd := fmt.Sprintf("settings put global airplane_mode_on %s", value)
	if _, err := d.device.Shell(settingsCmd); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set airplane mode: %v", err))
	}

	// Broadcast may fail on Android 7+ without root — warn but don't fail
	broadcastCmd := "am broadcast -a android.intent.action.AIRPLANE_MODE"
	if _, err := d.device.Shell(broadcastCmd); err != nil {
		logger.Warn("airplane mode broadcast failed (expected on Android 7+ without root): %v", err)
	}

	return successResult(fmt.Sprintf("Airplane mode %s", status), nil)
}

func (d *Driver) setAirplaneMode(step *flow.SetAirplaneModeStep) *core.CommandResult {
	return d.applyAirplaneMode(step.Enabled)
}

func (d *Driver) toggleAirplaneMode(_ *flow.ToggleAirplaneModeStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "toggleAirplaneMode requires device access")
	}

	output, err := d.device.Shell("settings get global airplane_mode_on")
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to get airplane mode: %v", err))
	}

	enable := strings.TrimSpace(output) != "1"
	return d.applyAirplaneMode(enable)
}

func (d *Driver) travel(step *flow.TravelStep) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "travel requires device access")
	}

	if len(step.Points) < 2 {
		return errorResult(fmt.Errorf("at least 2 points required"), "Travel requires at least 2 waypoints")
	}

	speed := step.Speed
	if speed <= 0 {
		speed = 50
	}

	for _, point := range step.Points {
		parts := strings.Split(point, ",")
		if len(parts) != 2 {
			continue
		}
		lat := strings.TrimSpace(parts[0])
		lon := strings.TrimSpace(parts[1])

		cmd := fmt.Sprintf("am broadcast -a android.intent.action.MOCK_LOCATION --ef lat %s --ef lon %s", lat, lon)
		if _, err := d.device.Shell(cmd); err != nil {
			return errorResult(err, fmt.Sprintf("Failed to set location during travel: %v", err))
		}

		delay := time.Duration(3600/speed) * time.Second
		time.Sleep(delay)
	}

	return successResult(fmt.Sprintf("Traveled through %d points", len(step.Points)), nil)
}

// ============================================================================
// Helpers
// ============================================================================

func mapDirection(dir string) string {
	switch dir {
	case "up":
		return uiautomator2.DirectionUp
	case "down":
		return uiautomator2.DirectionDown
	case "left":
		return uiautomator2.DirectionLeft
	case "right":
		return uiautomator2.DirectionRight
	default:
		return uiautomator2.DirectionDown
	}
}

func mapKeyCode(key string) int {
	switch strings.ToLower(key) {
	case "enter":
		return uiautomator2.KeyCodeEnter
	case "back":
		return uiautomator2.KeyCodeBack
	case "home":
		return uiautomator2.KeyCodeHome
	case "menu":
		return uiautomator2.KeyCodeMenu
	case "delete", "backspace":
		return uiautomator2.KeyCodeDelete
	case "tab":
		return uiautomator2.KeyCodeTab
	case "space":
		return uiautomator2.KeyCodeSpace
	case "volume_up":
		return uiautomator2.KeyCodeVolumeUp
	case "volume_down":
		return uiautomator2.KeyCodeVolumeDown
	case "power":
		return uiautomator2.KeyCodePower
	case "camera":
		return uiautomator2.KeyCodeCamera
	case "search":
		return uiautomator2.KeyCodeSearch
	case "dpad_up":
		return uiautomator2.KeyCodeDpadUp
	case "dpad_down":
		return uiautomator2.KeyCodeDpadDown
	case "dpad_left":
		return uiautomator2.KeyCodeDpadLeft
	case "dpad_right":
		return uiautomator2.KeyCodeDpadRight
	case "dpad_center":
		return uiautomator2.KeyCodeDpadCenter
	default:
		return 0
	}
}

// evalWebViewScript executes JavaScript in the mobile WebView via CDP.
func (d *Driver) evalWebViewScript(step *flow.EvalWebViewScriptStep) *core.CommandResult {
	if step.Script == "" {
		return &core.CommandResult{Success: false, Error: fmt.Errorf("evalWebViewScript: script is empty"), Message: "Script is empty"}
	}

	page := d.webView.rodPage()
	if page == nil {
		return &core.CommandResult{Success: false, Error: fmt.Errorf("evalWebViewScript: no WebView CDP connection"), Message: "No WebView CDP connection — is the WebView visible?"}
	}

	js := fmt.Sprintf("async () => { %s }", step.Script)
	obj, err := page.Timeout(10 * time.Second).Eval(js)
	if err != nil {
		return &core.CommandResult{Success: false, Error: fmt.Errorf("evalWebViewScript: %w", err), Message: fmt.Sprintf("JS execution failed: %v", err)}
	}

	val := ""
	if obj != nil && obj.Value.Val() != nil {
		val = obj.Value.Str()
	}

	result := &core.CommandResult{Success: true, Message: "evalWebViewScript completed"}
	result.Data = val
	return result
}

// runWebViewScript loads a JS file and executes it in the mobile WebView via CDP.
func (d *Driver) runWebViewScript(step *flow.RunWebViewScriptStep) *core.CommandResult {
	if step.File == "" {
		return &core.CommandResult{Success: false, Error: fmt.Errorf("runWebViewScript: file is required"), Message: "File is required"}
	}

	data, err := os.ReadFile(step.File) //#nosec G304 -- user-provided script file
	if err != nil {
		return &core.CommandResult{Success: false, Error: fmt.Errorf("runWebViewScript: %w", err), Message: fmt.Sprintf("Failed to read file: %v", err)}
	}

	page := d.webView.rodPage()
	if page == nil {
		return &core.CommandResult{Success: false, Error: fmt.Errorf("runWebViewScript: no WebView CDP connection"), Message: "No WebView CDP connection — is the WebView visible?"}
	}

	var envSetup string
	if len(step.Env) > 0 {
		envJSON, _ := json.Marshal(step.Env)
		envSetup = fmt.Sprintf("window.__env = %s;\n", envJSON)
	}

	js := fmt.Sprintf("async () => { %s%s }", envSetup, string(data))
	obj, err := page.Timeout(10 * time.Second).Eval(js)
	if err != nil {
		return &core.CommandResult{Success: false, Error: fmt.Errorf("runWebViewScript: %w", err), Message: fmt.Sprintf("JS execution failed: %v", err)}
	}

	val := ""
	if obj != nil && obj.Value.Val() != nil {
		val = obj.Value.Str()
	}

	result := &core.CommandResult{Success: true, Message: "runWebViewScript completed"}
	result.Data = val
	return result
}

// ============================================================================
// Dark mode (Maestro #2507)
// ============================================================================

func (d *Driver) setDarkMode(step *flow.SetDarkModeStep) *core.CommandResult {
	return d.applyDarkMode(step.Enabled)
}

func (d *Driver) toggleDarkMode(_ *flow.ToggleDarkModeStep) *core.CommandResult {
	current, err := d.currentDarkMode()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to read dark mode: %v", err))
	}
	return d.applyDarkMode(!current)
}

func (d *Driver) assertDarkMode(_ *flow.AssertDarkModeStep) *core.CommandResult {
	return d.assertDarkModeIs(true)
}

func (d *Driver) assertLightMode(_ *flow.AssertLightModeStep) *core.CommandResult {
	return d.assertDarkModeIs(false)
}

func (d *Driver) assertDarkModeIs(want bool) *core.CommandResult {
	got, err := d.currentDarkMode()
	if err != nil {
		return errorResult(err, fmt.Sprintf("Failed to read dark mode: %v", err))
	}
	if got != want {
		assertErr := core.DarkModeAssertionError(want, got)
		return errorResult(assertErr, assertErr.Error())
	}
	return successResult(fmt.Sprintf("Device is in %s mode", core.DarkModeStateName(want)), nil)
}

// currentDarkMode reports whether the system UI is currently dark.
func (d *Driver) currentDarkMode() (bool, error) {
	if d.device == nil {
		return false, fmt.Errorf("device not configured")
	}
	output, err := d.device.Shell(core.AndroidDarkModeQuery)
	if err != nil {
		return false, err
	}
	return core.ParseAndroidNightMode(output)
}

func (d *Driver) applyDarkMode(enabled bool) *core.CommandResult {
	if d.device == nil {
		return errorResult(fmt.Errorf("device not configured"), "setDarkMode requires device access")
	}
	if _, err := d.device.Shell(core.AndroidDarkModeCommand(enabled)); err != nil {
		return errorResult(err, fmt.Sprintf("Failed to set dark mode: %v", err))
	}
	return successResult(fmt.Sprintf("Set %s mode", core.DarkModeStateName(enabled)), nil)
}
