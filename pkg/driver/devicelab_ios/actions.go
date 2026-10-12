package devicelab_ios

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// ---------- geometry ----------

// screenSize is the viewport in points: from PlatformInfo, else the last
// lookup.
func (d *Driver) screenSize(sc *screen) (int, int) {
	if sc != nil && sc.width > 0 && sc.height > 0 {
		return int(sc.width), int(sc.height)
	}
	if d.info != nil && d.info.ScreenWidth > 0 && d.info.ScreenHeight > 0 {
		return d.info.ScreenWidth, d.info.ScreenHeight
	}
	return 0, 0
}

// tapPoint is the centre of the node's on-screen part, so a tap never lands
// off the display.
func tapPoint(n Node, w, h float64) (float64, float64, bool) {
	left, top := math.Max(n.X, 0), math.Max(n.Y, 0)
	right, bottom := n.X+n.W, n.Y+n.H
	if w > 0 {
		right = math.Min(right, w)
	}
	if h > 0 {
		bottom = math.Min(bottom, h)
	}
	if right <= left || bottom <= top {
		return 0, 0, false
	}
	return (left + right) / 2, (top + bottom) / 2, true
}

func (d *Driver) act(kind string, x, y float64, extra *Args) error {
	args := &Args{Kind: kind, X: f64(x), Y: f64(y)}
	if extra != nil {
		args.X2, args.Y2, args.HoldMs, args.MoveMs, args.RestMs = extra.X2, extra.Y2, extra.HoldMs, extra.MoveMs, extra.RestMs
	}
	_, err := d.call("act", args)
	return err
}

// ---------- taps ----------

func (d *Driver) tapOn(s *flow.TapOnStep) *core.CommandResult {
	if s.Selector.IsEmpty() && s.Point != "" {
		return d.tapOnPoint(&flow.TapOnPointStep{Point: s.Point, LongPress: s.LongPress, DurationMs: s.DurationMs})
	}
	kind, hold := "tap", 0
	if s.LongPress || s.DurationMs > 0 {
		kind, hold = "longPress", s.DurationMs
	}
	if s.Point != "" {
		return d.tapRelative(s.Selector, s.Point, s.Optional, s.TimeoutMs, kind, hold)
	}
	return d.tapSelector(s.Selector, s.Optional, s.TimeoutMs, kind, hold)
}

// tapSelector finds the element and taps (or double-taps, long-presses) it.
func (d *Driver) tapSelector(sel flow.Selector, optional bool, timeoutMs int, kind string, holdMs int) *core.CommandResult {
	node, sc, err := d.findElement(sel, optional, timeoutMs)
	if err != nil {
		return notFound(err, optional, "tap")
	}
	if d.foundLate {
		// The element appeared while we polled, so the screen was still
		// changing. Maestro settles right before every tap: do the same, then
		// aim where the element is now. A button on a page still loading took
		// the tap and did nothing (DDG's address-bar spoofing tests).
		d.settle(defaultSettleTimeout)
		again, sc2, err := d.findElement(sel, optional, settledRefindMs)
		if err != nil {
			// Gone once the screen settled: tapping where it was would hit
			// whatever is there now.
			return notFound(err, optional, "tap")
		}
		node, sc = again, sc2
	}
	x, y, ok := tapPoint(*node, sc.width, sc.height)
	if !ok {
		err := fmt.Errorf("%s is not on screen", describe(sel))
		return core.ErrorResult(err, err.Error())
	}
	var extra *Args
	if kind == "longPress" {
		ms := float64(holdMs)
		if ms <= 0 {
			ms = 1000
		}
		extra = &Args{HoldMs: f64(ms)}
	}
	logger.Debug("[devicelab-ios] %s %s → %s %q at (%.0f,%.0f) bounds %v", kind, describe(sel),
		node.Type, firstNonEmpty(node.Label, node.Value, node.Placeholder, node.ID), x, y, bounds(*node))
	if err := d.act(kind, x, y, extra); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("%s failed: %v", kind, err))
	}
	if kind == "tap" {
		d.lastTap = &tapAt{x: x, y: y}
	}
	return core.SuccessResult(fmt.Sprintf("%s on %s", kind, describe(sel)), toElementInfo(node))
}

// settledRefindMs bounds the second lookup after a late find's settle.
const settledRefindMs = 1000

// tapAt is a point a step tapped.
type tapAt struct{ x, y float64 }

// tapRelative taps a point given relative to the element ("50%, 90%").
func (d *Driver) tapRelative(sel flow.Selector, point string, optional bool, timeoutMs int, kind string, holdMs int) *core.CommandResult {
	node, _, err := d.findElement(sel, optional, timeoutMs)
	if err != nil {
		return notFound(err, optional, "tap")
	}
	x, y, err := core.PointInBounds(point, bounds(*node))
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("invalid point %q: %v", point, err))
	}
	var extra *Args
	if kind == "longPress" {
		extra = &Args{HoldMs: f64(math.Max(float64(holdMs), 1000))}
	}
	if err := d.act(kind, float64(x), float64(y), extra); err != nil {
		return core.ErrorResult(err, err.Error())
	}
	return core.SuccessResult(fmt.Sprintf("%s at %s on %s", kind, point, describe(sel)), toElementInfo(node))
}

func (d *Driver) tapOnPoint(s *flow.TapOnPointStep) *core.CommandResult {
	x, y := float64(s.X), float64(s.Y)
	if s.Point != "" {
		w, h := d.screenSize(nil)
		px, py, err := core.ParsePointCoords(s.Point, w, h)
		if err != nil {
			return core.ErrorResult(err, fmt.Sprintf("invalid point %q: %v", s.Point, err))
		}
		x, y = float64(px), float64(py)
	}
	kind := "tap"
	var extra *Args
	if s.LongPress || s.DurationMs > 0 {
		kind = "longPress"
		extra = &Args{HoldMs: f64(math.Max(float64(s.DurationMs), 1000))}
	}
	if err := d.act(kind, x, y, extra); err != nil {
		return core.ErrorResult(err, err.Error())
	}
	return core.SuccessResult(fmt.Sprintf("%s at (%.0f, %.0f)", kind, x, y), nil)
}

// notFound turns a failed lookup into a result: success for an optional step.
func notFound(err error, optional bool, what string) *core.CommandResult {
	if optional {
		return core.SuccessResult(fmt.Sprintf("optional %s skipped: %v", what, err), nil)
	}
	return core.ErrorResult(err, fmt.Sprintf("Element not found: %v", err))
}

// ---------- swipes and scrolls ----------

// swipeTimes are Maestro's swipe shape (EventRecord.addSwipeEvent): move in
// 0.1s, then rest for the whole duration before lifting (a rest stops the
// fling). Resting 0.1s less kept more fling: at speed 80 RNTester's list
// carried a row past the top of the screen between two lookups where
// Maestro's stopped with it in view.
func swipeTimes(durationMs int) *Args {
	if durationMs <= 0 {
		durationMs = 400
	}
	return &Args{HoldMs: f64(0), MoveMs: f64(100), RestMs: f64(float64(durationMs))}
}

func (d *Driver) swipeBetween(x1, y1, x2, y2 int, durationMs int) error {
	extra := swipeTimes(durationMs)
	extra.X2, extra.Y2 = f64(float64(x2)), f64(float64(y2))
	return d.act("swipe", float64(x1), float64(y1), extra)
}

func (d *Driver) swipe(s *flow.SwipeStep) *core.CommandResult {
	w, h := d.screenSize(nil)
	duration := s.Duration
	if duration <= 0 && s.Speed > 0 {
		duration = core.ScrollSpeedToDurationMs(s.Speed)
	}
	var x1, y1, x2, y2 int
	var err error
	switch {
	case s.Start != "" && s.End != "":
		if x1, y1, err = core.ParsePointCoords(s.Start, w, h); err == nil {
			x2, y2, err = core.ParsePointCoords(s.End, w, h)
		}
	case s.StartX != 0 || s.StartY != 0 || s.EndX != 0 || s.EndY != 0:
		x1, y1, x2, y2 = s.StartX, s.StartY, s.EndX, s.EndY
	case s.Selector != nil && !s.Selector.IsEmpty():
		dir, derr := core.NormalizeSwipeDirection(s.Direction)
		if derr != nil {
			return core.ErrorResult(derr, derr.Error())
		}
		node, _, ferr := d.findElement(*s.Selector, s.Optional, s.TimeoutMs)
		if ferr != nil {
			return notFound(ferr, s.Optional, "swipe")
		}
		x1, y1, x2, y2, err = core.SwipeCoordsForElement(dir, bounds(*node), w, h, s.Distance, s.Selector.Point)
	default:
		dir, derr := core.NormalizeSwipeDirection(s.Direction)
		if derr != nil {
			return core.ErrorResult(derr, derr.Error())
		}
		x1, y1, x2, y2, err = core.DirectionSwipeScreenCoords(dir, w, h, s.Distance)
	}
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("swipe: %v", err))
	}
	if err := d.swipeBetween(x1, y1, x2, y2, duration); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("swipe failed: %v", err))
	}
	return core.SuccessResult(fmt.Sprintf("swiped (%d,%d)→(%d,%d)", x1, y1, x2, y2), nil)
}

// scrollCoords is Maestro's scroll: from the centre to 10% from the edge,
// moving the content in the named direction.
func scrollCoords(direction string, w, h int) (int, int, int, int, error) {
	cx, cy := w/2, h/2
	switch strings.ToLower(direction) {
	case "", "down":
		return cx, cy, cx, h / 10, nil
	case "up":
		return cx, cy, cx, h * 9 / 10, nil
	case "left":
		return cx, cy, w * 9 / 10, cy, nil
	case "right":
		return cx, cy, w / 10, cy, nil
	}
	return 0, 0, 0, 0, fmt.Errorf("invalid scroll direction %q", direction)
}

// scrollDurationMs is Maestro's DEFAULT_SCROLL_DURATION (speed 40).
const scrollDurationMs = 601

func (d *Driver) scroll(direction string, speed int) *core.CommandResult {
	w, h := d.screenSize(nil)
	x1, y1, x2, y2, err := scrollCoords(direction, w, h)
	if err != nil {
		return core.ErrorResult(err, err.Error())
	}
	// Maestro's default scroll lasts 601ms (a 0.1s move, then still). With
	// 400ms the list kept more fling: the same centre-to-10% drag moved DDG's
	// debug list 682pt against Maestro's 488, past the row a centred
	// scrollUntilVisible was after and under the navigation bar.
	if err := d.swipeBetween(x1, y1, x2, y2, core.ScrollDurationOrDefault(speed, scrollDurationMs)); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("scroll failed: %v", err))
	}
	return core.SuccessResult("scrolled "+strings.ToLower(direction), nil)
}

// scrollUntilVisible: one lookup per round (on-screen matches only), then a
// swipe and an on-device settle. The timeout is the budget, as in Maestro;
// maxScrolls caps it only when the flow sets it. A screen that stops moving
// (the settle's screen hash repeating) ends the search early.
// scrollSettleTimeout bounds the wait after each scrollUntilVisible swipe. A
// list keeps drifting for about 2s after a fling (DDG settings), and waiting
// it out bought nothing: the next swipe stops the drift, the step that acts
// on the element settles first, and a drifting screen never reads as "no
// progress".
const scrollSettleTimeout = time.Second

func (d *Driver) scrollUntilVisible(s *flow.ScrollUntilVisibleStep) *core.CommandResult {
	if !s.From.IsEmpty() {
		err := fmt.Errorf("scrollUntilVisible `from:` is not supported on this driver yet")
		return core.ErrorResult(err, err.Error()+" — it currently works on the uiautomator2 driver")
	}
	var progress core.ScrollProgress
	timeout := 30 * time.Second
	if s.TimeoutMs > 0 {
		timeout = time.Duration(s.TimeoutMs) * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	maxScrolls := math.MaxInt
	if s.MaxScrolls > 0 {
		maxScrolls = s.MaxScrolls
	}
	direction := s.Direction
	if direction == "" {
		direction = "down"
	}
	// centerElement, as in Maestro: an element on screen keeps the scroll
	// going until it is near the middle, for at most maxCenterScrolls extra
	// scrolls (the list may end first). DDG's "Passwords" menu item sat under
	// the home indicator at the bottom edge, and a tap there never reached it.
	const maxCenterScrolls = 4
	centerScrolls := 0
	var shown *Node // on screen and visible enough, though not yet centred
	for i := 0; ; i++ {
		sc, err := d.lookup(s.Element, i > 0)
		if err == nil {
			if node, perr := pick(sc, s.Element); perr == nil {
				w, h := d.screenSize(sc)
				b := bounds(*node)
				logger.Debug("[devicelab-ios] scrollUntilVisible round %d: %s at %v on %dx%d", i, describe(s.Element), b, w, h)
				if s.CenterElement && core.VisibleFraction(b, w, h) > 0.1 && centerScrolls <= maxCenterScrolls {
					if core.NearScreenCenter(b, w, h, direction) {
						return core.SuccessResult("element centred after scrolling", toElementInfo(node))
					}
					centerScrolls++
					if core.MeetsVisibility(b, w, h, s.VisibilityPercentage) {
						shown = node
					}
				} else if core.MeetsVisibility(b, w, h, s.VisibilityPercentage) {
					return core.SuccessResult("element visible after scrolling", toElementInfo(node))
				}
			}
		}
		if i >= maxScrolls || time.Now().After(deadline) || d.context().Err() != nil {
			break
		}
		if res := d.scroll(direction, s.Speed); !res.Success {
			return res
		}
		if resp, err := d.call("settle", &Args{TimeoutMs: float64(scrollSettleTimeout.Milliseconds())}); err == nil {
			if sig := resp.payload().ScreenHash; sig != "" && progress.Observe(sig) {
				if shown != nil {
					return core.SuccessResult("element visible at the end of the content", toElementInfo(shown))
				}
				err := fmt.Errorf("%s not visible: scrolling %s made no progress after %d scrolls (end of content?)",
					describe(s.Element), direction, i+1)
				return core.ErrorResult(err, err.Error())
			}
		}
	}
	err := fmt.Errorf("%s not visible after scrolling %s", describe(s.Element), direction)
	return core.ErrorResult(err, err.Error())
}

func (d *Driver) dragAndDrop(s *flow.DragAndDropStep) *core.CommandResult {
	point := func(sel flow.Selector) (float64, float64, error) {
		if sel.IsEmpty() && sel.Point != "" {
			w, h := d.screenSize(nil)
			x, y, err := core.ParsePointCoords(sel.Point, w, h)
			return float64(x), float64(y), err
		}
		node, sc, err := d.findElement(sel, false, s.TimeoutMs)
		if err != nil {
			return 0, 0, err
		}
		x, y, ok := tapPoint(*node, sc.width, sc.height)
		if !ok {
			return 0, 0, fmt.Errorf("%s is not on screen", describe(sel))
		}
		return x, y, nil
	}
	x1, y1, err := point(s.From)
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("dragAndDrop from: %v", err))
	}
	x2, y2, err := point(s.To)
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("dragAndDrop to: %v", err))
	}
	hold, move := float64(s.HoldDuration), float64(s.Duration)
	if hold <= 0 {
		hold = 1000
	}
	if move <= 0 {
		move = 500
	}
	extra := &Args{X2: f64(x2), Y2: f64(y2), HoldMs: f64(hold), MoveMs: f64(move), RestMs: f64(250)}
	if err := d.act("drag", x1, y1, extra); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("drag failed: %v", err))
	}
	return core.SuccessResult("dragged", nil)
}

// ---------- asserts and waits ----------

func (d *Driver) assertVisible(s *flow.AssertVisibleStep) *core.CommandResult {
	if s.Count != "" {
		return d.assertCount(s)
	}
	// Not found is a failure even when the step is optional: the executor
	// turns a failed optional step into a warning itself, and it checks a
	// `when: visible` condition with an optional assert, so answering
	// success here made every such condition true (DDG ran the "Agree and
	// Continue" block with no consent screen on it). Optional only shortens
	// the wait, as in the WDA driver.
	node, _, err := d.findElement(s.Selector, s.Optional, s.TimeoutMs)
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("Element not visible: %v", err))
	}
	return core.SuccessResult("visible: "+describe(s.Selector), toElementInfo(node))
}

// assertCount checks the number of on-screen matches ("3", ">=2", "<5").
func (d *Driver) assertCount(s *flow.AssertVisibleStep) *core.CommandResult {
	want := strings.TrimSpace(s.Count)
	op, num := "==", want
	for _, prefix := range []string{">=", "<=", ">", "<", "=="} {
		if strings.HasPrefix(want, prefix) {
			op, num = prefix, strings.TrimSpace(strings.TrimPrefix(want, prefix))
			break
		}
	}
	var n int
	if _, err := fmt.Sscan(num, &n); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("invalid count %q", s.Count))
	}
	ok := func(got int) bool {
		switch op {
		case ">=":
			return got >= n
		case "<=":
			return got <= n
		case ">":
			return got > n
		case "<":
			return got < n
		}
		return got == n
	}
	deadline := time.Now().Add(d.budget(s.Optional, s.TimeoutMs))
	got := 0
	for {
		if sc, err := d.lookup(s.Selector, true); err == nil {
			if matches, merr := matchAll(sc, s.Selector); merr == nil {
				got = len(matches)
				if ok(got) {
					return core.SuccessResult(fmt.Sprintf("%d match(es) for %s", got, describe(s.Selector)), nil)
				}
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}
	err := fmt.Errorf("expected %s %s, found %d", describe(s.Selector), s.Count, got)
	return core.ErrorResult(err, err.Error())
}

func (d *Driver) budget(optional bool, timeoutMs int) time.Duration {
	if timeoutMs > 0 {
		return time.Duration(timeoutMs) * time.Millisecond
	}
	if optional {
		return d.optionalTimeout
	}
	return d.findTimeout
}

// assertNotVisible passes as soon as a lookup finds no on-screen match.
func (d *Driver) assertNotVisible(sel flow.Selector, timeoutMs int) *core.CommandResult {
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	for {
		sc, err := d.lookup(sel, true)
		if err == nil {
			if _, perr := pick(sc, sel); perr != nil {
				return core.SuccessResult("not visible: "+describe(sel), nil)
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}
	err := fmt.Errorf("%s is still visible", describe(sel))
	return core.ErrorResult(err, err.Error())
}

func (d *Driver) waitUntil(s *flow.WaitUntilStep) *core.CommandResult {
	timeout := s.TimeoutMs
	if timeout <= 0 {
		timeout = int(d.findTimeout.Milliseconds())
	}
	if s.Visible != nil {
		node, _, err := d.findElement(*s.Visible, false, timeout)
		if err != nil {
			// Say how long it waited, as the other drivers do.
			return core.ErrorResult(err, fmt.Sprintf("%v (waited %v)", err, time.Duration(timeout)*time.Millisecond))
		}
		return core.SuccessResult("visible", toElementInfo(node))
	}
	if s.NotVisible != nil {
		return d.assertNotVisible(*s.NotVisible, timeout)
	}
	err := errors.New("extendedWaitUntil needs visible or notVisible")
	return core.ErrorResult(err, err.Error())
}

func (d *Driver) waitForAnimation(timeoutMs int) *core.CommandResult {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	resp, err := d.call("settle", &Args{TimeoutMs: float64(timeout.Milliseconds())})
	if err != nil {
		return core.ErrorResult(err, err.Error())
	}
	if p := resp.payload(); p.Settled != nil && !*p.Settled {
		return core.SuccessResult(fmt.Sprintf("screen still changing after %s", timeout), nil)
	}
	return core.SuccessResult("screen settled", nil)
}

// ---------- text and keys ----------

func (d *Driver) inputText(s *flow.InputTextStep) *core.CommandResult {
	if !s.Selector.IsEmpty() {
		if res := d.tapSelector(s.Selector, false, s.TimeoutMs, "tap", 0); !res.Success {
			return res
		}
		d.settle(defaultSettleTimeout)
	}
	if s.Text == "" {
		return core.SuccessResult("nothing to type", nil)
	}
	resp, err := d.call("type", &Args{Text: s.Text, Speed: d.typingSpeed})
	// Nothing had focus: the tap meant to focus the field did not take. That
	// is this step's own tap when it has a selector, else the step before's
	// (the usual "tapOn field, inputText" pair). Tap it once more and type.
	retry := d.prevTap
	if !s.Selector.IsEmpty() {
		retry = d.lastTap
	}
	if isNoFocus(err) && retry != nil {
		logger.Debug("[devicelab-ios] inputText: no field has focus; re-tapping (%.0f,%.0f)", retry.x, retry.y)
		if terr := d.act("tap", retry.x, retry.y, nil); terr == nil {
			d.settle(defaultSettleTimeout)
			resp, err = d.call("type", &Args{Text: s.Text, Speed: d.typingSpeed})
		}
	}
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("inputText failed: %v", err))
	}
	msg := fmt.Sprintf("typed %d character(s)", len([]rune(s.Text)))
	if p := resp.payload(); p.Verified != nil && !*p.Verified {
		msg += " (not confirmed in the field)"
	}
	return core.SuccessResult(msg, nil)
}

func (d *Driver) eraseText(count int) *core.CommandResult {
	if count <= 0 {
		count = 50
	}
	if _, err := d.call("type", &Args{Erase: count, Speed: d.typingSpeed}); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("eraseText failed: %v", err))
	}
	return core.SuccessResult(fmt.Sprintf("erased up to %d character(s)", count), nil)
}

func (d *Driver) hideKeyboard() *core.CommandResult {
	resp, err := d.call("keyboard", &Args{Action: "hide"})
	if err != nil {
		return core.ErrorResult(err, err.Error())
	}
	// Maestro's iOS hideKeyboard tries its two swipes and moves on: a
	// keyboard some screens will not dismiss (TestHive's search field) is
	// not an error there, and the flow's next steps decide. Failing here
	// failed a flow Maestro and WDA pass.
	if resp.payload().Visible {
		logger.Info("[devicelab-ios] hideKeyboard: the keyboard is still visible after the dismiss swipes")
		return core.SuccessResult("keyboard still visible (not dismissible here)", nil)
	}
	return core.SuccessResult("keyboard hidden", nil)
}

func (d *Driver) pressKey(key string) *core.CommandResult {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "back":
		return d.back()
	case "volume up", "volume down", "power":
		err := fmt.Errorf("key %q is not available on the iOS simulator", key)
		return core.ErrorResult(err, err.Error())
	}
	if _, err := d.call("type", &Args{Key: k}); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("pressKey %s failed: %v", key, err))
	}
	return core.SuccessResult("pressed "+key, nil)
}

// back on iOS is the navigation bar's back button when there is one, else the
// edge swipe that pops a navigation stack.
func (d *Driver) back() *core.CommandResult {
	// UIKit's back button is identified "BackButton" and labelled with the
	// previous screen's title; other stacks label it "Back".
	for _, sel := range []flow.Selector{{ID: "^BackButton$"}, {Text: "(?i)back"}} {
		sc, err := d.lookup(sel, false)
		if err != nil {
			continue
		}
		for _, n := range sc.nodes {
			if n.Type == "Button" && n.Vis >= minVisible && n.Y < sc.height/4 && matchesSelf(n, sel) {
				x, y, _ := tapPoint(n, sc.width, sc.height)
				if err := d.act("tap", x, y, nil); err == nil {
					return core.SuccessResult("tapped Back", nil)
				}
			}
		}
	}
	_, h := d.screenSize(nil)
	extra := &Args{X2: f64(250), Y2: f64(float64(h) / 2), HoldMs: f64(0), MoveMs: f64(200), RestMs: f64(0)}
	if err := d.act("swipe", 2, float64(h)/2, extra); err != nil {
		return core.ErrorResult(err, err.Error())
	}
	return core.SuccessResult("back (edge swipe)", nil)
}

// ---------- clipboard ----------

func (d *Driver) copyTextFrom(s *flow.CopyTextFromStep) *core.CommandResult {
	node, _, err := d.findElement(s.Selector, s.Optional, s.TimeoutMs)
	if err != nil {
		return notFound(err, s.Optional, "copyTextFrom")
	}
	text := elementText(*node)
	return &core.CommandResult{Success: true, Message: "copied: " + text, Data: text, Element: toElementInfo(node)}
}

func (d *Driver) setClipboard(text string) *core.CommandResult {
	if d.realDevice {
		err := errOnDevice("setClipboard")
		return core.ErrorResult(err, err.Error())
	}
	cmd := simctlStdin(d.udid, text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return core.ErrorResult(err, fmt.Sprintf("setClipboard failed: %s", strings.TrimSpace(string(out))))
	}
	return core.SuccessResult("clipboard set", nil)
}

// pasteText types the simulator's clipboard (the executor pastes its own
// copied text first; this is the fallback).
func (d *Driver) pasteText() *core.CommandResult {
	if d.realDevice {
		err := errOnDevice("pasteText without a copyTextFrom first")
		return core.ErrorResult(err, err.Error())
	}
	out, err := d.runSimctl("pbpaste", d.udid)
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("pasteText: read clipboard failed: %v", err))
	}
	return d.inputText(&flow.InputTextStep{Text: out})
}

// ---------- screenshots and alerts ----------

func (d *Driver) takeScreenshot(crop *flow.Selector) *core.CommandResult {
	data, err := d.Screenshot()
	if err != nil {
		return core.ErrorResult(err, fmt.Sprintf("screenshot failed: %v", err))
	}
	if crop != nil && !crop.IsEmpty() {
		node, sc, ferr := d.findElement(*crop, false, 0)
		if ferr != nil {
			return core.ErrorResult(ferr, fmt.Sprintf("cropOn: %v", ferr))
		}
		w, h := d.screenSize(sc)
		cropped, cerr := core.CropScreenshot(data, bounds(*node), w, h)
		if cerr != nil {
			return core.ErrorResult(cerr, fmt.Sprintf("cropOn: %v", cerr))
		}
		data = cropped
	}
	return &core.CommandResult{Success: true, Message: "screenshot captured", Data: data}
}

// alert accepts or dismisses a system or in-app alert, waiting for one to
// appear up to the step's timeout (default 5s).
func (d *Driver) alert(action string, timeoutMs int) *core.CommandResult {
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	for {
		resp, err := d.call("alert", &Args{Action: action, App: d.appID})
		if err == nil && resp.payload().Present {
			return core.SuccessResult(fmt.Sprintf("%s alert: %s", action, resp.payload().Message), nil)
		}
		if time.Now().After(deadline) {
			return core.SuccessResult("no alert to "+action, nil)
		}
		time.Sleep(pollInterval)
	}
}

// isNoFocus reports the agent's "nothing has keyboard focus" answer to type.
func isNoFocus(err error) bool {
	var ae *AgentError
	return errors.As(err, &ae) && ae.Code == "NO_FOCUS"
}
