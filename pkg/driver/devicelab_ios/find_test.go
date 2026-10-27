package devicelab_ios

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func sc(nodes ...Node) *screen { return &screen{nodes: nodes, width: 400, height: 800} }

func TestPickPrefersExactCaseThenDeepest(t *testing.T) {
	outer := node(1, "Other", "Login", 0, 0, 400, 100)
	inner := node(2, "Button", "Login", 10, 10, 100, 40)
	inner.P = 1
	lower := node(3, "Button", "login", 0, 200, 100, 40)
	got, err := pick(sc(outer, lower, inner), flow.Selector{Text: "Login"})
	if err != nil || got.I != 2 {
		t.Fatalf("got %+v %v, want the deepest exact-case match", got, err)
	}
	got, _ = pick(sc(outer, lower, inner), flow.Selector{Text: "login"})
	if got.I != 3 {
		t.Fatalf("got %d, want the exact-case lowercase match", got.I)
	}
}

func TestPickIndex(t *testing.T) {
	s := sc(node(1, "Cell", "Row", 0, 0, 400, 50), node(2, "Cell", "Row", 0, 60, 400, 50), node(3, "Cell", "Row", 0, 120, 400, 50))
	for idx, want := range map[string]int{"0": 1, "2": 3, "-1": 3} {
		got, err := pick(s, flow.Selector{Text: "Row", Index: idx})
		if err != nil || got.I != want {
			t.Errorf("index %s: got %+v %v, want %d", idx, got, err, want)
		}
	}
	for _, bad := range []string{"5", "x", "-9"} {
		if _, err := pick(s, flow.Selector{Text: "Row", Index: bad}); err == nil {
			t.Errorf("index %s should fail", bad)
		}
	}
}

func TestMatchesSelfFields(t *testing.T) {
	n := node(1, "Switch", "Wi-Fi", 0, 0, 100, 30)
	n.ID = "com.app:id/wifi_toggle"
	n.Value = "1"
	n.Focused = true
	cases := []struct {
		sel  flow.Selector
		want bool
	}{
		{flow.Selector{ID: "wifi_toggle"}, true},
		{flow.Selector{ID: "WIFI.*"}, true},
		{flow.Selector{ID: "bluetooth"}, false},
		{flow.Selector{Text: "wi-fi"}, true},
		{flow.Selector{Checked: boolp(true)}, true},
		{flow.Selector{Checked: boolp(false)}, false},
		{flow.Selector{Enabled: boolp(false)}, false},
		{flow.Selector{Selected: boolp(true)}, false},
		{flow.Selector{Focused: boolp(true)}, true},
		{flow.Selector{Width: 103, Height: 28}, true},
		{flow.Selector{Width: 120}, false},
		{flow.Selector{Height: 50, Tolerance: 30}, true},
	}
	for _, c := range cases {
		if got := matchesSelf(n, c.sel); got != c.want {
			t.Errorf("%+v: got %v, want %v", c.sel, got, c.want)
		}
	}
}

// The #188 cases: text and ids match whole, as in Maestro.
func TestMatchesSelfWholeText(t *testing.T) {
	row := node(1, "Cell", "Talk · Open", 0, 0, 100, 30)
	row.ID = "com.app:id/login_button"
	cases := []struct {
		sel  flow.Selector
		want bool
	}{
		{flow.Selector{Text: "Open"}, false},
		{flow.Selector{Text: ".*Open.*"}, true},
		{flow.Selector{Text: "talk · open"}, true},
		{flow.Selector{ID: "login"}, false},
		{flow.Selector{ID: "login_button"}, true},
	}
	for _, c := range cases {
		if got := matchesSelf(row, c.sel); got != c.want {
			t.Errorf("%s: got %v, want %v", c.sel.Describe(), got, c.want)
		}
	}
}

func TestInvisibleNodesDoNotMatch(t *testing.T) {
	n := node(1, "Button", "Hidden", 0, 0, 10, 10)
	n.Vis = 0.05
	if _, err := pick(sc(n), flow.Selector{Text: "Hidden"}); err == nil {
		t.Fatal("5% visible should not count")
	}
}

func TestRelativeSelectors(t *testing.T) {
	list := node(1, "Table", "", 0, 100, 400, 600)
	header := node(2, "StaticText", "Email", 0, 100, 400, 30)
	header.P = 1
	field := node(3, "TextField", "", 0, 140, 400, 40)
	field.P = 1
	field.Placeholder = "you@example.com"
	far := node(4, "TextField", "", 0, 600, 400, 40)
	far.P = 1
	left := node(5, "Button", "L", 0, 700, 50, 50)
	right := node(6, "Button", "R", 300, 700, 50, 50)
	s := sc(list, header, field, far, left, right)

	cases := []struct {
		name string
		sel  flow.Selector
		want int
	}{
		{"below nearest", flow.Selector{Below: &flow.Selector{Text: "Email"}, Text: ".*"}, 3},
		{"above", flow.Selector{Above: &flow.Selector{Text: "you@.*"}, Text: "Email"}, 2},
		{"rightOf", flow.Selector{RightOf: &flow.Selector{Text: "L"}, Text: "R"}, 6},
		{"leftOf", flow.Selector{LeftOf: &flow.Selector{Text: "R"}, Text: "L"}, 5},
		{"childOf", flow.Selector{ChildOf: &flow.Selector{ID: ""}, Text: "Email"}, -1},
		{"insideOf", flow.Selector{InsideOf: &flow.Selector{Text: "Email"}, Text: "Email"}, 2},
	}
	for _, c := range cases {
		got, err := pick(s, c.sel)
		if c.want < 0 {
			if err == nil {
				t.Errorf("%s: want error", c.name)
			}
			continue
		}
		if err != nil || got.I != c.want {
			t.Errorf("%s: got %+v %v, want %d", c.name, got, err, c.want)
		}
	}

	list.ID = "form"
	s = sc(list, header, field, far, left, right)
	got, err := pick(s, flow.Selector{ChildOf: &flow.Selector{ID: "form"}, Text: "Email"})
	if err != nil || got.I != 2 {
		t.Fatalf("childOf: %+v %v", got, err)
	}
	got, err = pick(s, flow.Selector{ID: "form", ContainsChild: &flow.Selector{Text: "Email"}})
	if err != nil || got.I != 1 {
		t.Fatalf("containsChild: %+v %v", got, err)
	}
	got, err = pick(s, flow.Selector{ID: "form", ContainsDescendants: []*flow.Selector{{Text: "you@.*"}, nil}})
	if err != nil || got.I != 1 {
		t.Fatalf("containsDescendants: %+v %v", got, err)
	}
	if _, err := pick(s, flow.Selector{ID: "form", ContainsChild: &flow.Selector{Text: "nope"}}); err == nil {
		t.Fatal("containsChild miss should fail")
	}
}

func TestLookupUsesSnapshotForRelative(t *testing.T) {
	d, fa, _ := newTestDriver(t, screenOf(node(1, "Button", "A", 0, 0, 10, 10)))
	_, _ = d.lookup(flow.Selector{Text: "A", Below: &flow.Selector{Text: "B"}}, false)
	_, _ = d.lookup(flow.Selector{Width: 10}, false)
	_, _ = d.lookup(flow.Selector{ID: "submit_btn"}, false)
	_, _ = d.lookup(flow.Selector{ID: "com.app.submit"}, false)
	if s := fa.sent("snapshot"); len(s) != 2 || !s[0].VisibleOnly {
		t.Fatalf("snapshots = %+v", s)
	}
	f := fa.sent("find")
	if len(f) != 2 || f[0].IDNeedle != "submit_btn" || f[1].IDNeedle != "" {
		t.Fatalf("finds = %+v", f)
	}
}

func TestToElementInfoAndDescribe(t *testing.T) {
	n := node(1, "Button", "Save", 1, 2, 3, 4)
	n.ID = "save"
	info := toElementInfo(&n)
	if info.Text != "Save" || info.Bounds.Width != 3 || !info.Visible || info.Class != "Button" {
		t.Fatalf("info = %+v", info)
	}
	if toElementInfo(nil) != nil {
		t.Fatal("nil")
	}
	if describe(flow.Selector{}) != "element" || describe(flow.Selector{Text: "a", ID: "b", Index: "1", Below: &flow.Selector{}}) == "" {
		t.Fatal("describe")
	}
	n.Title, n.Value = "", "v"
	if elementText(n) != "v" {
		t.Fatal("value before label")
	}
}

// With an index, matches that contain another match are dropped first, as
// Maestro does: a switch row and the switch inside it share the value "0",
// and index 0 must be the switch, not the row whose centre is its label.
func TestIndexAppliesToInnermostMatches(t *testing.T) {
	row := node(1, "Switch", "Application Lock", 20, 160, 353, 44)
	row.Value = "0"
	inner := node(2, "Switch", "", 296, 166, 57, 32)
	inner.Value = "0"
	other := node(3, "Switch", "Other", 20, 260, 353, 44)
	other.Value = "0"
	sc := &screen{nodes: []Node{row, inner, other}, width: 400, height: 800}
	got, err := pick(sc, flow.Selector{Text: "0", Index: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if got.I != 2 {
		t.Errorf("index 0 picked node %d, want the inner switch (2)", got.I)
	}
	got, err = pick(sc, flow.Selector{Text: "0", Index: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.I != 3 {
		t.Errorf("index 1 picked node %d, want the next innermost match (3)", got.I)
	}
}

func TestSiblingInsideAnotherIsNotItsChild(t *testing.T) {
	field := node(27, "TextField", "", 36, 191, 321, 21)
	field.P, field.ID = 25, "Field_PasswordName"
	icon := node(28, "Image", "", 341, 194, 16, 16)
	icon.P, icon.ID = 25, "Field_PasswordName"
	sc := &screen{nodes: []Node{field, icon}, width: 393, height: 852}
	got, err := pick(sc, flow.Selector{ID: "Field_PasswordName"})
	if err != nil {
		t.Fatal(err)
	}
	if got.I != 27 {
		t.Errorf("picked node %d, want the text field (27), first in tree order", got.I)
	}
}

func TestMayContain(t *testing.T) {
	parent := node(5, "Cell", "", 0, 0, 100, 100)
	child := node(6, "Button", "", 10, 10, 20, 20)
	child.P = 5
	grandchild := node(9, "Image", "", 12, 12, 5, 5)
	grandchild.P = 7
	later := node(20, "Other", "", 0, 0, 100, 100)
	for _, c := range []struct {
		name         string
		outer, inner Node
		want         bool
	}{
		{"direct parent", parent, child, true},
		{"deeper descendant by bounds", parent, grandchild, true},
		{"later in tree order", later, child, false},
		{"child does not contain parent", child, parent, false},
	} {
		if got := mayContain(c.outer, c.inner); got != c.want {
			t.Errorf("%s: mayContain = %v, want %v", c.name, got, c.want)
		}
	}
}

// Bounds come from truncated edges, as in Maestro, so a crop matches the size
// of a baseline Maestro took: y=100.33 + h=168.67 is 169 points tall, not 168.
func TestBoundsMatchMaestroEdges(t *testing.T) {
	for _, tc := range []struct {
		n    Node
		want core.Bounds
	}{
		{Node{X: 16, Y: 100.33, W: 370, H: 168.67}, core.Bounds{X: 16, Y: 100, Width: 370, Height: 169}},
		{Node{X: 16.5, Y: 200, W: 369.8, H: 40}, core.Bounds{X: 16, Y: 200, Width: 370, Height: 40}},
		{Node{X: 0, Y: 0, W: 402, H: 874}, core.Bounds{X: 0, Y: 0, Width: 402, Height: 874}},
		{Node{X: 10.9, Y: 10.9, W: 0.05, H: 0.05}, core.Bounds{X: 10, Y: 10, Width: 0, Height: 0}},
	} {
		if got := bounds(tc.n); got != tc.want {
			t.Errorf("bounds(%+v) = %+v, want %+v", tc.n, got, tc.want)
		}
	}
}

// TestHive's search icon and its text field are siblings with the same id,
// the icon first in tree order. A tap must take the field: a tap on the icon
// never focused it, and inputText failed with NO_FOCUS on every device.
func TestSameIDPrefersTheControl(t *testing.T) {
	icon := node(15, "Image", "Search", 34, 147, 16, 17)
	icon.ID = "search-bar"
	field := node(16, "TextField", "", 60, 144, 309, 22)
	field.ID = "search-bar"
	field.Placeholder = "Search frameworks..."
	got, err := pick(&screen{nodes: []Node{icon, field}, width: 402, height: 874}, flow.Selector{ID: "search-bar"})
	if err != nil {
		t.Fatal(err)
	}
	if got.I != 16 {
		t.Errorf("picked node %d (%s), want the text field (16)", got.I, got.Type)
	}
}

// TestIndexCountsByScreenPosition: with an index, matches are counted top to
// bottom, then left to right, as in Maestro, not in tree order (#254). The
// tab bar comes first in the tree but sits at the bottom of the screen.
func TestIndexCountsByScreenPosition(t *testing.T) {
	tabA := node(1, "Button", "Home", 0, 780, 130, 50)
	tabB := node(2, "Button", "Back", 130, 780, 130, 50)
	header := node(3, "StaticText", "Connections", 0, 100, 400, 30)
	summary := node(4, "StaticText", "Wi-Fi", 0, 140, 400, 30)
	devices := node(5, "StaticText", "Connected devices", 0, 200, 400, 30)
	s := sc(tabA, tabB, header, summary, devices)

	for idx, want := range map[string]int{"0": 4, "1": 5, "2": 1, "-1": 2} {
		got, err := pick(s, flow.Selector{Below: &flow.Selector{Text: "Connections"}, Text: ".+", Index: idx})
		if err != nil || got.I != want {
			t.Errorf("below, index %s: got %+v %v, want %d", idx, got, err, want)
		}
	}
	for idx, want := range map[string]int{"0": 3, "1": 4, "-1": 2} {
		got, err := pick(s, flow.Selector{Text: ".+", Index: idx})
		if err != nil || got.I != want {
			t.Errorf("plain, index %s: got %+v %v, want %d", idx, got, err, want)
		}
	}
}
