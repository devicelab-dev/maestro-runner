package devicelab_ios

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// minVisible is Maestro's rule: an element less than 10% on screen does not
// count as present.
const minVisible = 0.1

// screen is one lookup's view: the candidates (or the whole tree, for
// relative selectors), and the viewport they were measured against.
type screen struct {
	nodes      []Node
	width      float64
	height     float64
	snapshotID int
	truncated  bool
}

// errNotFound means the selector matched nothing on screen.
var errNotFound = errors.New("element not found")

// lookup asks the agent once. Plain selectors go through `find` (coarse match
// on the device, candidates only); relative selectors need their anchors and
// the parent links, so they take the whole on-screen tree.
func (d *Driver) lookup(sel flow.Selector, alerts bool) (*screen, error) {
	var resp *Response
	var err error
	if hasRelative(sel) || sel.Width > 0 || sel.Height > 0 {
		resp, err = d.call("snapshot", &Args{VisibleOnly: true, Alerts: alerts})
	} else {
		args := &Args{MinVisible: minVisible, Alerts: alerts}
		if sel.Text != "" {
			args.Needles = core.LiteralNeedles(sel.Text)
		}
		if sel.ID != "" && !core.LooksLikeRegex(sel.ID) && !strings.Contains(sel.ID, ".") {
			args.IDNeedle = strings.ToLower(sel.ID)
		}
		resp, err = d.call("find", args)
	}
	if err != nil {
		return nil, err
	}
	p := resp.payload()
	return &screen{nodes: p.Nodes, width: p.ScreenW, height: p.ScreenH, snapshotID: resp.SnapshotID, truncated: p.Truncated}, nil
}

// findElement polls until sel matches an on-screen element or the budget runs
// out. timeoutMs 0 takes the driver's default for required or optional steps.
func (d *Driver) findElement(sel flow.Selector, optional bool, timeoutMs int) (*Node, *screen, error) {
	budget := d.findTimeout
	if optional {
		budget = d.optionalTimeout
	}
	if timeoutMs > 0 {
		budget = time.Duration(timeoutMs) * time.Millisecond
	}
	start := time.Now()
	deadline := start.Add(budget)
	var lastErr error
	for attempt := 0; ; attempt++ {
		// A system alert (a permission prompt) belongs to SpringBoard, not the
		// app; include it after the first miss.
		sc, err := d.lookup(sel, attempt > 0)
		if err == nil {
			if node, merr := pick(sc, sel); merr == nil {
				logger.Debug("[devicelab-ios] found %s → %s %q vis=%.2f bounds %v (attempt %d)", describe(sel),
					node.Type, firstNonEmpty(node.Label, node.Value, node.Placeholder, node.ID), node.Vis, bounds(*node), attempt)
				d.foundLate = attempt > 0
				return node, sc, nil
			} else {
				lastErr = merr
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) || d.context().Err() != nil {
			if lastErr == nil {
				lastErr = errNotFound
			}
			return nil, nil, fmt.Errorf("%s: %w", describe(sel), lastErr)
		}
		// A permission prompt may be what hides the element.
		if d.answerAlertOnMiss(start) {
			d.afterAlertTap()
			continue
		}
		time.Sleep(pollInterval)
	}
}

// pick applies the selector to one lookup and chooses the element Maestro
// would: of the matches, those in the selector's own case first, then the
// deepest (one that contains another match yields to it), then tree order,
// then `index`.
func pick(sc *screen, sel flow.Selector) (*Node, error) {
	matches, err := matchAll(sc, sel)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, errNotFound
	}
	if sel.Index != "" {
		// Maestro drops the matches that contain another match (deepest
		// wins) before applying the index. Indexing the raw list picked a
		// switch row (label and control share a value) over the switch inside
		// it, and the tap on the row's centre hit the label: DDG's
		// Application Lock never turned on.
		matches = innermost(matches)
		// Maestro then counts the matches by their place on screen, top to
		// bottom, then left to right, not in tree order (#254): a tab bar
		// early in the tree is at the bottom of the screen.
		matches = core.SortByPosition(matches, nodeBounds)
		i, err := strconv.Atoi(strings.TrimSpace(sel.Index))
		if err != nil {
			return nil, fmt.Errorf("index %q is not a number", sel.Index)
		}
		if i < 0 {
			i += len(matches)
		}
		if i < 0 || i >= len(matches) {
			return nil, fmt.Errorf("index %d out of range: %d match(es)", i, len(matches))
		}
		return &matches[i], nil
	}
	return &deepest(matches, sel)[0], nil
}

// nodeBounds is a node's frame in whole points, for core.SortByPosition.
func nodeBounds(n Node) core.Bounds {
	return core.Bounds{X: int(n.X), Y: int(n.Y), Width: int(n.W), Height: int(n.H)}
}

// matchAll returns every node the selector matches, in tree order.
func matchAll(sc *screen, sel flow.Selector) ([]Node, error) {
	var out []Node
	for _, n := range sc.nodes {
		if n.Vis >= minVisible && matchesSelf(n, sel) {
			out = append(out, n)
		}
	}
	if !hasRelative(sel) {
		return out, nil
	}
	return applyRelative(sc, sel, out)
}

// matchesSelf checks the selector's own fields (not relative ones).
func matchesSelf(n Node, sel flow.Selector) bool {
	if sel.Text != "" && !core.MatchSelectorText(sel.Text, texts(n)...) {
		return false
	}
	if sel.ID != "" && !core.MatchSelectorID(sel.ID, n.ID) {
		return false
	}
	if sel.Width > 0 || sel.Height > 0 {
		tol := sel.Tolerance
		if tol == 0 {
			tol = 5
		}
		if sel.Width > 0 && abs(int(n.W)-sel.Width) > tol {
			return false
		}
		if sel.Height > 0 && abs(int(n.H)-sel.Height) > tol {
			return false
		}
	}
	if sel.Enabled != nil && n.Enabled != *sel.Enabled {
		return false
	}
	if sel.Selected != nil && n.Selected != *sel.Selected {
		return false
	}
	if sel.Focused != nil && n.Focused != *sel.Focused {
		return false
	}
	if sel.Checked != nil && isChecked(n) != *sel.Checked {
		return false
	}
	return true
}

// texts are the values a text selector matches on iOS: title/value (Maestro's
// text), placeholder (hintText) and label (accessibilityText).
func texts(n Node) []string {
	return []string{n.Title, n.Value, n.Placeholder, n.Label}
}

// isChecked reads a switch's or checkbox's state from its value.
func isChecked(n Node) bool {
	switch strings.ToLower(n.Value) {
	case "1", "true", "on", "yes":
		return true
	}
	return n.Selected
}

// deepest orders matches the way Maestro chooses: exact case first, then
// matches containing no other match (the deepest), then tree order.
func deepest(matches []Node, sel flow.Selector) []Node {
	type ranked struct {
		n       Node
		exact   bool
		outer   bool
		control bool
		pos     int
	}
	rs := make([]ranked, len(matches))
	for i, m := range matches {
		rs[i] = ranked{n: m, pos: i}
		if sel.Text != "" {
			rs[i].exact = core.MatchSelectorTextExactCase(sel.Text, texts(m)...)
		}
		rs[i].outer = containsOther(matches, i)
		rs[i].control = isControl(m.Type)
	}
	sort.SliceStable(rs, func(a, b int) bool {
		if rs[a].exact != rs[b].exact {
			return rs[a].exact
		}
		if rs[a].outer != rs[b].outer {
			return !rs[a].outer
		}
		// Of siblings that match alike, a control comes before an image or
		// a label: TestHive's search icon and its text field share the id
		// "search-bar", the icon comes first, and a tap on it never focused
		// the field. WDA takes the first match XCUITest reports displayed,
		// which is the field.
		if rs[a].control != rs[b].control {
			return rs[a].control
		}
		return rs[a].pos < rs[b].pos
	})
	out := make([]Node, len(rs))
	for i, r := range rs {
		out[i] = r.n
	}
	return out
}

// isControl reports whether an element type is one a user acts on: a field,
// a button or another control, as opposed to an image, a label or a container.
func isControl(t string) bool {
	switch strings.TrimPrefix(t, "XCUIElementType") {
	case "TextField", "SecureTextField", "SearchField", "TextView", "Button", "Switch", "Toggle",
		"Link", "Slider", "Stepper", "SegmentedControl", "Picker", "PickerWheel", "DatePicker":
		return true
	}
	return false
}

func contains(outer, inner Node) bool {
	return inner.X >= outer.X && inner.Y >= outer.Y &&
		inner.X+inner.W <= outer.X+outer.W && inner.Y+inner.H <= outer.Y+outer.H
}

func sameFrame(a, b Node) bool {
	return a.X == b.X && a.Y == b.Y && a.W == b.W && a.H == b.H
}

func hasRelative(sel flow.Selector) bool {
	return sel.Below != nil || sel.Above != nil || sel.LeftOf != nil || sel.RightOf != nil ||
		sel.ChildOf != nil || sel.ContainsChild != nil || len(sel.ContainsDescendants) > 0 || sel.InsideOf != nil
}

// applyRelative keeps the matches that satisfy every relative constraint
// against an anchor found in the same tree.
func applyRelative(sc *screen, sel flow.Selector, matches []Node) ([]Node, error) {
	anchor := func(a *flow.Selector) (*Node, error) {
		n, err := pick(sc, *a)
		if err != nil {
			return nil, fmt.Errorf("relative anchor %s: %w", describe(*a), err)
		}
		return n, nil
	}
	keep := matches
	filter := func(f func(Node) bool) {
		var next []Node
		for _, m := range keep {
			if f(m) {
				next = append(next, m)
			}
		}
		keep = next
	}
	type spatial struct {
		sel *flow.Selector
		ok  func(m, a Node) bool
	}
	for _, s := range []spatial{
		{sel.Below, func(m, a Node) bool { return m.Y >= a.Y+a.H }},
		{sel.Above, func(m, a Node) bool { return m.Y+m.H <= a.Y }},
		{sel.LeftOf, func(m, a Node) bool { return m.X+m.W <= a.X }},
		{sel.RightOf, func(m, a Node) bool { return m.X >= a.X+a.W }},
		{sel.InsideOf, func(m, a Node) bool {
			cx, cy := m.X+m.W/2, m.Y+m.H/2
			return cx >= a.X && cx <= a.X+a.W && cy >= a.Y && cy <= a.Y+a.H
		}},
	} {
		if s.sel == nil {
			continue
		}
		a, err := anchor(s.sel)
		if err != nil {
			return nil, err
		}
		filter(func(m Node) bool { return s.ok(m, *a) })
		// Nearest to the anchor first, as Maestro orders relative matches.
		sort.SliceStable(keep, func(i, j int) bool { return distance(keep[i], *a) < distance(keep[j], *a) })
	}
	if sel.ChildOf != nil {
		a, err := anchor(sel.ChildOf)
		if err != nil {
			return nil, err
		}
		filter(func(m Node) bool { return isDescendant(sc.nodes, m, a.I) })
	}
	if sel.ContainsChild != nil {
		filter(func(m Node) bool { return hasChildMatching(sc, m, *sel.ContainsChild, true) })
	}
	for _, desc := range sel.ContainsDescendants {
		if desc == nil {
			continue
		}
		dsel := *desc
		filter(func(m Node) bool { return hasChildMatching(sc, m, dsel, false) })
	}
	return keep, nil
}

func distance(m, a Node) float64 {
	dx := (m.X + m.W/2) - (a.X + a.W/2)
	dy := (m.Y + m.H/2) - (a.Y + a.H/2)
	return dx*dx + dy*dy
}

// isDescendant reports whether n sits under the node at index ancestor.
func isDescendant(nodes []Node, n Node, ancestor int) bool {
	byIndex := make(map[int]Node, len(nodes))
	for _, x := range nodes {
		byIndex[x.I] = x
	}
	for p := n.P; p >= 0; {
		if p == ancestor {
			return true
		}
		parent, ok := byIndex[p]
		if !ok {
			return false
		}
		p = parent.P
	}
	return false
}

// hasChildMatching reports whether a direct child (or any descendant) of m
// matches sel.
func hasChildMatching(sc *screen, m Node, sel flow.Selector, directOnly bool) bool {
	for _, n := range sc.nodes {
		if !matchesSelf(n, sel) {
			continue
		}
		if directOnly && n.P == m.I {
			return true
		}
		if !directOnly && isDescendant(sc.nodes, n, m.I) {
			return true
		}
	}
	return false
}

// toElementInfo converts a node for results and reports.
func toElementInfo(n *Node) *core.ElementInfo {
	if n == nil {
		return nil
	}
	return &core.ElementInfo{
		ID:                 n.ID,
		Text:               elementText(*n),
		Bounds:             bounds(*n),
		Visible:            n.Vis > 0,
		Enabled:            n.Enabled,
		Focused:            n.Focused,
		Selected:           n.Selected,
		Checked:            isChecked(*n),
		Class:              n.Type,
		AccessibilityLabel: n.Label,
	}
}

// elementText is the text Maestro reads from an iOS element (copyTextFrom):
// title, else value, else placeholder, else label.
func elementText(n Node) string {
	return firstNonEmpty(n.Title, n.Value, n.Placeholder, n.Label)
}

// bounds converts the agent's point rect to whole points the way Maestro does
// (AXElement.boundsString): each edge is truncated and the size is the
// difference of the edges. Truncating the size itself lost a point whenever
// the element started on a fraction: y=100.33 with height 168.67 became 168
// here and 169 in Maestro, so every cropOn screenshot came out 3px (one point
// at 3x) shorter than a baseline Maestro took, and assertScreenshot failed on
// size alone.
func bounds(n Node) core.Bounds {
	x, y := int(n.X), int(n.Y)
	return core.Bounds{X: x, Y: y, Width: int(n.X+n.W) - x, Height: int(n.Y+n.H) - y}
}

func describe(sel flow.Selector) string {
	var parts []string
	if sel.Text != "" {
		parts = append(parts, fmt.Sprintf("text=%q", sel.Text))
	}
	if sel.ID != "" {
		parts = append(parts, fmt.Sprintf("id=%q", sel.ID))
	}
	if sel.Index != "" {
		parts = append(parts, "index="+sel.Index)
	}
	if hasRelative(sel) {
		parts = append(parts, "relative")
	}
	if len(parts) == 0 {
		return "element"
	}
	return strings.Join(parts, ", ")
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// innermost keeps, in tree order, the matches that contain no other match.
func innermost(matches []Node) []Node {
	out := make([]Node, 0, len(matches))
	for i, m := range matches {
		if !containsOther(matches, i) {
			out = append(out, m)
		}
	}
	return out
}

// containsOther reports whether matches[i] contains another match with a
// different frame, i.e. is an ancestor Maestro's deepest-match rule drops.
func containsOther(matches []Node, i int) bool {
	for j, o := range matches {
		if i != j && mayContain(matches[i], o) && !sameFrame(matches[i], o) {
			return true
		}
	}
	return false
}

// mayContain reports whether outer can be an ancestor of inner. Maestro's rule
// is tree ancestry, and a find returns candidates only, so the tree order
// (parents before children) rules out what it can and bounds decide the rest.
// A sibling drawn inside another is not its child: DDG's password Title field
// and the 16pt icon laid over it share an id, and bounds alone dropped the
// field for the icon, which takes no focus.
func mayContain(outer, inner Node) bool {
	if inner.P == outer.I && outer.I != inner.I {
		return true
	}
	if outer.I > inner.I || (inner.P >= 0 && inner.P == outer.P) {
		return false
	}
	return contains(outer, inner)
}
