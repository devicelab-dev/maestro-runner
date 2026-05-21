package devicelab

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// makeElement is a tiny helper for building ParsedElement test fixtures.
func makeElement(x, y, w, h, depth int) *ParsedElement {
	return &ParsedElement{
		Bounds: core.Bounds{X: x, Y: y, Width: w, Height: h},
		Depth:  depth,
	}
}

// =============================================================================
// Pure predicates
// =============================================================================

func TestWithinTolerance(t *testing.T) {
	cases := []struct {
		actual, expected, tolerance int
		want                        bool
	}{
		{100, 100, 0, true},  // exact
		{100, 102, 2, true},  // diff exactly tolerance
		{100, 103, 2, false}, // diff over
		{102, 100, 2, true},  // negative diff handled
		{103, 100, 2, false},
	}
	for _, c := range cases {
		if got := withinTolerance(c.actual, c.expected, c.tolerance); got != c.want {
			t.Errorf("withinTolerance(%d,%d,%d) = %v, want %v",
				c.actual, c.expected, c.tolerance, got, c.want)
		}
	}
}

func TestMatchesID(t *testing.T) {
	cases := []struct {
		pattern, id string
		want        bool
	}{
		{"button", "com.app:id/button", true},               // substring fallback
		{"^com\\.app", "com.app:id/foo", true},              // regex
		{"^com\\.app", "com.example:id/foo", false},         // regex no match
		{"((", "com.app:id/((stuff", true},                  // invalid regex → substring fallback
		{"((", "no match here", false},                      // invalid regex + no substring
		{"\\d+", "foo123bar", true},                         // regex with digits
	}
	for _, c := range cases {
		if got := matchesID(c.pattern, c.id); got != c.want {
			t.Errorf("matchesID(%q,%q) = %v, want %v", c.pattern, c.id, got, c.want)
		}
	}
}

func TestMatchesText(t *testing.T) {
	cases := []struct {
		name                          string
		pattern, text, content, hint  string
		want                          bool
	}{
		{"literal text match", "Login", "Login Button", "", "", true},
		{"literal case-insensitive", "LOGIN", "login button", "", "", true},
		{"literal content-desc", "submit", "", "Submit form", "", true},
		{"literal hint", "search", "", "", "Search...", true},
		{"literal no match", "logout", "Login", "", "", false},
		// Regex pattern (looksLikeRegex returns true for ^, $, \d, etc.)
		{"regex match text", "^Login$", "Login", "", "", true},
		{"regex no match", "^Login$", "Logout", "", "", false},
		{"regex match content-desc", "^Submit$", "", "Submit", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := matchesText(c.pattern, c.text, c.content, c.hint); got != c.want {
				t.Errorf("matchesText(%q,%q,%q,%q) = %v, want %v",
					c.pattern, c.text, c.content, c.hint, got, c.want)
			}
		})
	}
}

func TestIsInside(t *testing.T) {
	outer := core.Bounds{X: 0, Y: 0, Width: 100, Height: 100}
	cases := []struct {
		name  string
		inner core.Bounds
		want  bool
	}{
		{"fully inside", core.Bounds{X: 10, Y: 10, Width: 50, Height: 50}, true},
		{"flush with outer", core.Bounds{X: 0, Y: 0, Width: 100, Height: 100}, true},
		{"extends right", core.Bounds{X: 50, Y: 50, Width: 60, Height: 40}, false},
		{"starts left of outer", core.Bounds{X: -1, Y: 10, Width: 10, Height: 10}, false},
		{"extends below", core.Bounds{X: 10, Y: 90, Width: 10, Height: 20}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isInside(c.inner, outer); got != c.want {
				t.Errorf("isInside(%v, %v) = %v, want %v", c.inner, outer, got, c.want)
			}
		})
	}
}

// =============================================================================
// Position-based filters
// =============================================================================

func TestFilterBelow(t *testing.T) {
	anchor := makeElement(0, 100, 50, 50, 0) // bottom = 150
	elements := []*ParsedElement{
		makeElement(0, 50, 10, 10, 0),   // above anchor
		makeElement(0, 150, 10, 10, 0),  // flush at anchor bottom — below
		makeElement(0, 200, 10, 10, 0),  // further below
		makeElement(0, 130, 10, 10, 0),  // overlaps anchor — not strictly below
	}
	got := FilterBelow(elements, anchor)
	if len(got) != 2 {
		t.Fatalf("expected 2 elements below, got %d", len(got))
	}
	// Sorted by Y ascending (closest first).
	if got[0].Bounds.Y != 150 {
		t.Errorf("expected closest element first (Y=150), got %d", got[0].Bounds.Y)
	}
}

func TestFilterAbove(t *testing.T) {
	anchor := makeElement(0, 100, 50, 50, 0) // top = 100
	elements := []*ParsedElement{
		makeElement(0, 50, 10, 10, 0),  // bottom=60, above
		makeElement(0, 0, 10, 100, 0),  // bottom=100, flush with anchor top
		makeElement(0, 110, 10, 10, 0), // not above
	}
	got := FilterAbove(elements, anchor)
	if len(got) != 2 {
		t.Fatalf("expected 2 elements above, got %d", len(got))
	}
	// Closest first means highest bottom.
	if got[0].Bounds.Y+got[0].Bounds.Height != 100 {
		t.Errorf("expected closest (bottom=100) first, got %d", got[0].Bounds.Y+got[0].Bounds.Height)
	}
}

func TestFilterLeftOf(t *testing.T) {
	anchor := makeElement(100, 0, 50, 10, 0) // left = 100
	elements := []*ParsedElement{
		makeElement(0, 0, 50, 10, 0),  // right=50, left of anchor
		makeElement(60, 0, 40, 10, 0), // right=100, flush
		makeElement(150, 0, 10, 10, 0),
	}
	got := FilterLeftOf(elements, anchor)
	if len(got) != 2 {
		t.Fatalf("expected 2 elements left of anchor, got %d", len(got))
	}
	if got[0].Bounds.X+got[0].Bounds.Width != 100 {
		t.Errorf("closest right edge should be 100, got %d", got[0].Bounds.X+got[0].Bounds.Width)
	}
}

func TestFilterRightOf(t *testing.T) {
	anchor := makeElement(100, 0, 50, 10, 0) // right = 150
	elements := []*ParsedElement{
		makeElement(150, 0, 10, 10, 0),
		makeElement(200, 0, 10, 10, 0),
		makeElement(50, 0, 10, 10, 0), // left of
	}
	got := FilterRightOf(elements, anchor)
	if len(got) != 2 {
		t.Fatalf("expected 2 elements right of anchor, got %d", len(got))
	}
	if got[0].Bounds.X != 150 {
		t.Errorf("closest left edge should be 150, got %d", got[0].Bounds.X)
	}
}

func TestFilterChildOf(t *testing.T) {
	anchor := makeElement(0, 0, 100, 100, 0)
	elements := []*ParsedElement{
		makeElement(10, 10, 80, 80, 1), // inside
		makeElement(0, 0, 100, 100, 1), // flush — inside
		makeElement(-1, 0, 10, 10, 1),  // extends left — not inside
	}
	got := FilterChildOf(elements, anchor)
	if len(got) != 2 {
		t.Errorf("expected 2 children, got %d", len(got))
	}
}

func TestFilterContainsChild(t *testing.T) {
	// Child of width 10x10 at (50,50)
	anchor := makeElement(50, 50, 10, 10, 1)
	elements := []*ParsedElement{
		makeElement(0, 0, 100, 100, 0), // anchor inside this — contains
		makeElement(0, 0, 30, 30, 0),   // anchor not inside this
	}
	got := FilterContainsChild(elements, anchor)
	if len(got) != 1 {
		t.Errorf("expected 1 container, got %d", len(got))
	}
}

// =============================================================================
// Deepest element selector
// =============================================================================

func TestDeepestMatchingElement(t *testing.T) {
	if e := DeepestMatchingElement(nil); e != nil {
		t.Error("DeepestMatchingElement(nil) should return nil")
	}
	if e := DeepestMatchingElement([]*ParsedElement{}); e != nil {
		t.Error("DeepestMatchingElement(empty) should return nil")
	}
	elems := []*ParsedElement{
		makeElement(0, 0, 10, 10, 1),
		makeElement(0, 0, 10, 10, 5), // deepest
		makeElement(0, 0, 10, 10, 3),
	}
	got := DeepestMatchingElement(elems)
	if got == nil || got.Depth != 5 {
		t.Errorf("expected depth 5, got %+v", got)
	}
}

// =============================================================================
// containsAllDescendants
// =============================================================================

func TestContainsAllDescendants(t *testing.T) {
	parent := makeElement(0, 0, 100, 100, 0)
	innerA := &ParsedElement{
		Text:   "Submit",
		Bounds: core.Bounds{X: 10, Y: 10, Width: 50, Height: 20},
	}
	innerB := &ParsedElement{
		ResourceID: "com.app:id/username",
		Bounds:     core.Bounds{X: 10, Y: 50, Width: 50, Height: 20},
	}
	outsideC := &ParsedElement{
		Text:   "Submit",
		Bounds: core.Bounds{X: 200, Y: 200, Width: 50, Height: 20},
	}
	allElems := []*ParsedElement{parent, innerA, innerB, outsideC}

	// Both required descendants are inside parent → true
	desc := []*flow.Selector{
		{Text: "Submit"},
		{ID: "username"},
	}
	if !containsAllDescendants(parent, allElems, desc) {
		t.Error("parent should contain both required descendants")
	}

	// One descendant is outside parent → false
	descMissing := []*flow.Selector{
		{Text: "Submit"},
		{Text: "DoesNotExist"},
	}
	if containsAllDescendants(parent, allElems, descMissing) {
		t.Error("parent missing 'DoesNotExist' should return false")
	}
}

// =============================================================================
// Sort helpers (table-driven)
// =============================================================================

func TestSortByDistanceY(t *testing.T) {
	elems := []*ParsedElement{
		makeElement(0, 200, 10, 10, 0),
		makeElement(0, 50, 10, 10, 0),
		makeElement(0, 100, 10, 10, 0),
	}
	sortByDistanceY(elems, 50)
	wantOrder := []int{50, 100, 200}
	for i, w := range wantOrder {
		if elems[i].Bounds.Y != w {
			t.Errorf("sortByDistanceY[%d].Y = %d, want %d", i, elems[i].Bounds.Y, w)
		}
	}
}

func TestSortByDistanceYReverse(t *testing.T) {
	// Reverse: sort by (refY - elem.Bottom), closest-to-anchor first means
	// highest bottom first (i.e. element nearest above the anchor).
	elems := []*ParsedElement{
		makeElement(0, 0, 10, 50, 0),   // bottom = 50
		makeElement(0, 0, 10, 90, 0),   // bottom = 90 — closest
		makeElement(0, 0, 10, 10, 0),   // bottom = 10 — farthest
	}
	sortByDistanceYReverse(elems, 100)
	if elems[0].Bounds.Height != 90 {
		t.Errorf("closest bottom should be 90, got %d", elems[0].Bounds.Height)
	}
}

func TestSortByDistanceX(t *testing.T) {
	elems := []*ParsedElement{
		makeElement(200, 0, 10, 10, 0),
		makeElement(50, 0, 10, 10, 0),
		makeElement(100, 0, 10, 10, 0),
	}
	sortByDistanceX(elems, 50)
	wantOrder := []int{50, 100, 200}
	for i, w := range wantOrder {
		if elems[i].Bounds.X != w {
			t.Errorf("sortByDistanceX[%d].X = %d, want %d", i, elems[i].Bounds.X, w)
		}
	}
}

func TestSortByDistanceXReverse(t *testing.T) {
	elems := []*ParsedElement{
		makeElement(0, 0, 50, 10, 0),  // right = 50
		makeElement(0, 0, 90, 10, 0),  // right = 90 — closest to anchorLeft=100
		makeElement(0, 0, 10, 10, 0),  // right = 10
	}
	sortByDistanceXReverse(elems, 100)
	if elems[0].Bounds.Width != 90 {
		t.Errorf("closest right=90 should be first, got %d", elems[0].Bounds.Width)
	}
}

// =============================================================================
// FilterInsideOf — uses center containment, not full-bounds
// =============================================================================

func TestFilterInsideOf(t *testing.T) {
	anchor := makeElement(0, 0, 100, 100, 0)
	elems := []*ParsedElement{
		makeElement(40, 40, 10, 10, 0),   // center (45,45) inside
		makeElement(150, 150, 10, 10, 0), // center way outside
	}
	got := FilterInsideOf(elems, anchor)
	if len(got) != 1 || got[0].Bounds.X != 40 {
		t.Errorf("expected single inside element at (40,40), got %v", got)
	}
}

// =============================================================================
// FilterContainsDescendants
// =============================================================================

func TestFilterContainsDescendants(t *testing.T) {
	parentA := makeElement(0, 0, 100, 100, 0)
	parentB := makeElement(200, 0, 100, 100, 0)
	innerInA := &ParsedElement{Text: "Submit", Bounds: core.Bounds{X: 10, Y: 10, Width: 20, Height: 20}}
	innerInB := &ParsedElement{Text: "Cancel", Bounds: core.Bounds{X: 210, Y: 10, Width: 20, Height: 20}}
	all := []*ParsedElement{parentA, parentB, innerInA, innerInB}

	desc := []*flow.Selector{{Text: "Submit"}}
	got := FilterContainsDescendants([]*ParsedElement{parentA, parentB}, all, desc)
	if len(got) != 1 || got[0] != parentA {
		t.Errorf("expected only parentA to contain 'Submit', got %v", got)
	}
}

// =============================================================================
// Keyboard helpers
// =============================================================================

func TestParseKeyboardFrame_NotShown(t *testing.T) {
	// Explicit isOnScreen=false should short-circuit to nil.
	if b := parseKeyboardFrame("blah isOnScreen=false blah"); b != nil {
		t.Errorf("expected nil for isOnScreen=false, got %+v", b)
	}
	// mViewVisibility=0x8 (GONE) should also short-circuit.
	if b := parseKeyboardFrame("blah mViewVisibility=0x8 blah"); b != nil {
		t.Errorf("expected nil for mViewVisibility=0x8, got %+v", b)
	}
	// No recognizable frame → nil.
	if b := parseKeyboardFrame("irrelevant text"); b != nil {
		t.Errorf("expected nil for unrecognized input, got %+v", b)
	}
}

func TestParseKeyboardFrame_TouchableRegion(t *testing.T) {
	// Regex is: `touchable region=SkRegion\(\((\d+),(\d+),(\d+),(\d+)\)\)`
	// Note the space between "touchable" and "region", and the lowercase r.
	out := "blah touchable region=SkRegion((0,1200,1080,2400)) blah"
	b := parseKeyboardFrame(out)
	if b == nil {
		t.Fatalf("expected bounds from touchable region")
	}
	if b.X != 0 || b.Y != 1200 || b.Width != 1080 || b.Height != 1200 {
		t.Errorf("unexpected bounds from touchableRegion: %+v", b)
	}
}

func TestBoundsFromMatches(t *testing.T) {
	// Positive
	b := boundsFromMatches([]string{"", "10", "20", "110", "220"})
	if b == nil || b.X != 10 || b.Y != 20 || b.Width != 100 || b.Height != 200 {
		t.Errorf("unexpected bounds: %+v", b)
	}

	// Zero / negative dimensions — should return nil
	if b := boundsFromMatches([]string{"", "10", "20", "10", "30"}); b != nil {
		t.Errorf("expected nil for zero-width bounds, got %+v", b)
	}
	if b := boundsFromMatches([]string{"", "10", "20", "30", "20"}); b != nil {
		t.Errorf("expected nil for zero-height bounds, got %+v", b)
	}
}

func TestTapWouldHitKeyboard(t *testing.T) {
	keyboard := core.Bounds{X: 0, Y: 1500, Width: 1080, Height: 900}
	// Element above keyboard (with margin)
	above := core.Bounds{X: 100, Y: 100, Width: 200, Height: 200}
	if tapWouldHitKeyboard(above, keyboard) {
		t.Error("element well above keyboard should not collide")
	}
	// Element overlapping keyboard
	overlap := core.Bounds{X: 100, Y: 1700, Width: 200, Height: 100}
	if !tapWouldHitKeyboard(overlap, keyboard) {
		t.Error("element overlapping keyboard should collide")
	}
}

func TestConsumeInputFlag(t *testing.T) {
	d := &Driver{lastStepWasInput: true}
	if !d.consumeInputFlag() {
		t.Error("first consume should return true")
	}
	if d.consumeInputFlag() {
		t.Error("flag should be reset after consume")
	}
}

// =============================================================================
// ParsePageSource — XML parser
// =============================================================================

func TestParsePageSource_HappyPath(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]" enabled="true" clickable="false" scrollable="false">
    <node class="android.widget.Button" text="Sign In" resource-id="com.app:id/btn" bounds="[100,200][300,260]" enabled="true" clickable="true" displayed="true"/>
    <node class="android.widget.ScrollView" bounds="[0,300][1080,2200]" scrollable="true" enabled="true"/>
  </node>
</hierarchy>`
	elems, err := ParsePageSource(xml)
	if err != nil {
		t.Fatalf("ParsePageSource: %v", err)
	}
	if len(elems) == 0 {
		t.Fatal("expected at least one element")
	}

	// Find the button
	var btn *ParsedElement
	for _, e := range elems {
		if e.Text == "Sign In" {
			btn = e
			break
		}
	}
	if btn == nil {
		t.Fatal("missing Sign In button")
	}
	if !btn.Clickable {
		t.Error("button should be clickable")
	}
	if btn.ResourceID != "com.app:id/btn" {
		t.Errorf("resource-id: got %q", btn.ResourceID)
	}
	if btn.Bounds.Width != 200 || btn.Bounds.Height != 60 {
		t.Errorf("bounds: got %+v, want 200x60", btn.Bounds)
	}
}

func TestParsePageSource_UIAutomatorFormat(t *testing.T) {
	// Old UIAutomator dump format uses class names as element tags.
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy>
  <android.widget.FrameLayout bounds="[0,0][1080,2400]">
    <android.widget.TextView text="hello" bounds="[10,10][100,40]"/>
  </android.widget.FrameLayout>
</hierarchy>`
	elems, err := ParsePageSource(xml)
	if err != nil {
		t.Fatalf("ParsePageSource UIAutomator format: %v", err)
	}
	if len(elems) == 0 {
		t.Fatal("expected at least one element")
	}
}

func TestParsePageSource_InvalidXML(t *testing.T) {
	_, err := ParsePageSource("<not closed")
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestParsePageSource_Empty(t *testing.T) {
	_, err := ParsePageSource("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

// =============================================================================
// FilterScrollable / FindLargestScrollable
// =============================================================================

func TestFilterScrollable(t *testing.T) {
	elems := []*ParsedElement{
		{Scrollable: true, Bounds: core.Bounds{Width: 100, Height: 100}},
		{Scrollable: false, Bounds: core.Bounds{Width: 100, Height: 100}},
		{Scrollable: true, Bounds: core.Bounds{Width: 0, Height: 100}}, // zero width → skip
		{Scrollable: true, Bounds: core.Bounds{Width: 100, Height: 0}}, // zero height → skip
	}
	got := FilterScrollable(elems)
	if len(got) != 1 {
		t.Errorf("expected 1 scrollable element, got %d", len(got))
	}
}

func TestFindLargestScrollable(t *testing.T) {
	// No scrollables
	got := FindLargestScrollable([]*ParsedElement{
		{Scrollable: false, Bounds: core.Bounds{Width: 100, Height: 100}},
	})
	if got != nil {
		t.Error("expected nil when no scrollables present")
	}

	// Largest of multiple
	a := &ParsedElement{Scrollable: true, Bounds: core.Bounds{Width: 100, Height: 100}} // 10000
	b := &ParsedElement{Scrollable: true, Bounds: core.Bounds{Width: 200, Height: 200}} // 40000
	c := &ParsedElement{Scrollable: true, Bounds: core.Bounds{Width: 50, Height: 50}}   // 2500
	got = FindLargestScrollable([]*ParsedElement{a, b, c})
	if got != b {
		t.Errorf("expected largest=b (200x200), got %+v", got)
	}
}

// =============================================================================
// GetClickableElement
// =============================================================================

func TestGetClickableElement(t *testing.T) {
	// Nil → nil
	if e := GetClickableElement(nil); e != nil {
		t.Error("GetClickableElement(nil) should return nil")
	}

	// Self is clickable → return self
	self := &ParsedElement{Clickable: true}
	if e := GetClickableElement(self); e != self {
		t.Error("self-clickable should return itself")
	}

	// Walk up to find clickable parent
	grandparent := &ParsedElement{Clickable: true}
	parent := &ParsedElement{Clickable: false, Parent: grandparent}
	child := &ParsedElement{Clickable: false, Parent: parent}
	if e := GetClickableElement(child); e != grandparent {
		t.Errorf("expected grandparent, got %+v", e)
	}

	// No clickable ancestor — return original
	deadend := &ParsedElement{Clickable: false}
	if e := GetClickableElement(deadend); e != deadend {
		t.Error("no clickable parent → return original")
	}
}
