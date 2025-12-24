package uiautomator2

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

const sampleHierarchy = `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,0][1080,1920]" clickable="false" enabled="true">
    <node index="0" text="Login" resource-id="com.app:id/login_btn" class="android.widget.Button" bounds="[100,200][300,280]" clickable="true" enabled="true"/>
    <node index="1" text="Sign Up" resource-id="com.app:id/signup_btn" class="android.widget.Button" bounds="[100,300][300,380]" clickable="true" enabled="true"/>
    <node index="2" text="" resource-id="com.app:id/container" class="android.widget.LinearLayout" bounds="[0,400][1080,800]" clickable="false" enabled="true">
      <node index="0" text="Username" resource-id="com.app:id/label" class="android.widget.TextView" bounds="[50,420][200,460]" clickable="false" enabled="true"/>
      <node index="1" text="" resource-id="com.app:id/input" class="android.widget.EditText" bounds="[50,470][500,530]" clickable="true" enabled="true" focused="true"/>
    </node>
  </node>
</hierarchy>`

func TestParsePageSource(t *testing.T) {
	elements, err := ParsePageSource(sampleHierarchy)
	if err != nil {
		t.Fatalf("ParsePageSource failed: %v", err)
	}

	if len(elements) == 0 {
		t.Fatal("expected elements, got none")
	}

	// Should have 6 elements total (1 root + 3 children + 2 grandchildren)
	if len(elements) != 6 {
		t.Errorf("expected 6 elements, got %d", len(elements))
	}

	// Check first button
	var loginBtn *ParsedElement
	for _, e := range elements {
		if e.Text == "Login" {
			loginBtn = e
			break
		}
	}
	if loginBtn == nil {
		t.Fatal("Login button not found")
	}
	if loginBtn.ResourceID != "com.app:id/login_btn" {
		t.Errorf("expected resource-id com.app:id/login_btn, got %s", loginBtn.ResourceID)
	}
	if !loginBtn.Clickable {
		t.Error("expected Login button to be clickable")
	}
}

func TestParsePageSourceInvalidXML(t *testing.T) {
	_, err := ParsePageSource("not xml")
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestParseBounds(t *testing.T) {
	tests := []struct {
		input    string
		expected core.Bounds
	}{
		{"[0,0][100,200]", core.Bounds{X: 0, Y: 0, Width: 100, Height: 200}},
		{"[50,100][150,300]", core.Bounds{X: 50, Y: 100, Width: 100, Height: 200}},
		{"invalid", core.Bounds{}},
		{"[0,0]", core.Bounds{}},
	}

	for _, tt := range tests {
		got := parseBounds(tt.input)
		if got != tt.expected {
			t.Errorf("parseBounds(%q) = %+v, want %+v", tt.input, got, tt.expected)
		}
	}
}

func TestFilterBySelector(t *testing.T) {
	elements, _ := ParsePageSource(sampleHierarchy)

	// Filter by text
	sel := flow.Selector{Text: "Login"}
	result := FilterBySelector(elements, sel)
	if len(result) != 1 {
		t.Errorf("expected 1 element with text 'Login', got %d", len(result))
	}

	// Filter by ID (partial match)
	sel = flow.Selector{ID: "login_btn"}
	result = FilterBySelector(elements, sel)
	if len(result) != 1 {
		t.Errorf("expected 1 element with ID containing 'login_btn', got %d", len(result))
	}

	// Filter by text case-insensitive
	sel = flow.Selector{Text: "login"}
	result = FilterBySelector(elements, sel)
	if len(result) != 1 {
		t.Errorf("expected 1 element with text 'login' (case-insensitive), got %d", len(result))
	}
}

func TestFilterBySelectorSize(t *testing.T) {
	elements, _ := ParsePageSource(sampleHierarchy)

	// Login button is 200x80 [100,200][300,280]
	sel := flow.Selector{Width: 200, Height: 80}
	result := FilterBySelector(elements, sel)
	if len(result) != 2 { // Login and Sign Up buttons have same size
		t.Errorf("expected 2 elements with size 200x80, got %d", len(result))
	}

	// With tolerance
	sel = flow.Selector{Width: 195, Height: 75, Tolerance: 10}
	result = FilterBySelector(elements, sel)
	if len(result) != 2 {
		t.Errorf("expected 2 elements within tolerance, got %d", len(result))
	}
}

func TestFilterBySelectorState(t *testing.T) {
	elements, _ := ParsePageSource(sampleHierarchy)

	enabled := true
	sel := flow.Selector{Enabled: &enabled}
	result := FilterBySelector(elements, sel)
	if len(result) != 6 { // All elements are enabled
		t.Errorf("expected 6 enabled elements, got %d", len(result))
	}

	disabled := false
	sel = flow.Selector{Enabled: &disabled}
	result = FilterBySelector(elements, sel)
	if len(result) != 0 {
		t.Errorf("expected 0 disabled elements, got %d", len(result))
	}
}

func TestFilterBelow(t *testing.T) {
	elements, _ := ParsePageSource(sampleHierarchy)

	// Find Login button as anchor
	var anchor *ParsedElement
	for _, e := range elements {
		if e.Text == "Login" {
			anchor = e
			break
		}
	}

	result := FilterBelow(elements, anchor)

	// Sign Up and container elements are below Login
	if len(result) < 1 {
		t.Error("expected elements below Login button")
	}

	// First result should be closest (Sign Up)
	if result[0].Text != "Sign Up" {
		t.Errorf("expected Sign Up to be first below, got %s", result[0].Text)
	}
}

func TestFilterAbove(t *testing.T) {
	elements, _ := ParsePageSource(sampleHierarchy)

	// Find Sign Up button as anchor
	var anchor *ParsedElement
	for _, e := range elements {
		if e.Text == "Sign Up" {
			anchor = e
			break
		}
	}

	result := FilterAbove(elements, anchor)

	// Login button is above Sign Up
	found := false
	for _, e := range result {
		if e.Text == "Login" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Login button to be above Sign Up")
	}
}

func TestFilterChildOf(t *testing.T) {
	elements, _ := ParsePageSource(sampleHierarchy)

	// Find container as anchor
	var container *ParsedElement
	for _, e := range elements {
		if e.ResourceID == "com.app:id/container" {
			container = e
			break
		}
	}

	result := FilterChildOf(elements, container)

	// Username label and input are children of container
	if len(result) < 2 {
		t.Errorf("expected at least 2 children of container, got %d", len(result))
	}
}

func TestFilterContainsChild(t *testing.T) {
	elements, _ := ParsePageSource(sampleHierarchy)

	// Find Username label as anchor
	var label *ParsedElement
	for _, e := range elements {
		if e.Text == "Username" {
			label = e
			break
		}
	}

	result := FilterContainsChild(elements, label)

	// Container should contain Username label
	found := false
	for _, e := range result {
		if e.ResourceID == "com.app:id/container" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected container to be found as parent of Username")
	}
}

func TestFilterContainsDescendants(t *testing.T) {
	elements, _ := ParsePageSource(sampleHierarchy)

	// Find elements that contain both "Username" and an input field
	descendants := []*flow.Selector{
		{Text: "Username"},
		{ID: "input"},
	}

	result := FilterContainsDescendants(elements, elements, descendants)

	// Should find container and root
	if len(result) < 1 {
		t.Error("expected at least 1 element containing both descendants")
	}
}

func TestDeepestMatchingElement(t *testing.T) {
	elements, _ := ParsePageSource(sampleHierarchy)

	// Filter to get multiple matches
	sel := flow.Selector{ID: "com.app"}
	matches := FilterBySelector(elements, sel)

	deepest := DeepestMatchingElement(matches)
	if deepest == nil {
		t.Fatal("expected deepest element, got nil")
	}

	// Deepest should have highest depth
	for _, e := range matches {
		if e.Depth > deepest.Depth {
			t.Errorf("found element with depth %d > deepest depth %d", e.Depth, deepest.Depth)
		}
	}
}

func TestDeepestMatchingElementEmpty(t *testing.T) {
	result := DeepestMatchingElement(nil)
	if result != nil {
		t.Error("expected nil for empty slice")
	}
}

func TestSortClickableFirst(t *testing.T) {
	elements := []*ParsedElement{
		{Text: "A", Clickable: false},
		{Text: "B", Clickable: true},
		{Text: "C", Clickable: false},
		{Text: "D", Clickable: true},
	}

	result := SortClickableFirst(elements)

	// First two should be clickable
	if !result[0].Clickable || !result[1].Clickable {
		t.Error("expected clickable elements first")
	}
	if result[2].Clickable || result[3].Clickable {
		t.Error("expected non-clickable elements last")
	}
}

func TestWithinTolerance(t *testing.T) {
	tests := []struct {
		actual, expected, tolerance int
		want                        bool
	}{
		{100, 100, 5, true},
		{102, 100, 5, true},
		{98, 100, 5, true},
		{106, 100, 5, false},
		{94, 100, 5, false},
	}

	for _, tt := range tests {
		got := withinTolerance(tt.actual, tt.expected, tt.tolerance)
		if got != tt.want {
			t.Errorf("withinTolerance(%d, %d, %d) = %v, want %v",
				tt.actual, tt.expected, tt.tolerance, got, tt.want)
		}
	}
}

func TestMatchesSelector(t *testing.T) {
	elem := &ParsedElement{
		Text:        "Login Button",
		ContentDesc: "Sign in to your account",
		ResourceID:  "com.app:id/login",
		Enabled:     true,
		Selected:    false,
		Bounds:      core.Bounds{Width: 200, Height: 80},
	}

	tests := []struct {
		name string
		sel  flow.Selector
		want bool
	}{
		{"text match", flow.Selector{Text: "Login"}, true},
		{"text no match", flow.Selector{Text: "Signup"}, false},
		{"text in content-desc", flow.Selector{Text: "Sign in"}, true},
		{"id match", flow.Selector{ID: "login"}, true},
		{"id no match", flow.Selector{ID: "signup"}, false},
		{"size match", flow.Selector{Width: 200, Height: 80}, true},
		{"size no match", flow.Selector{Width: 300, Height: 80}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesSelector(elem, tt.sel)
			if got != tt.want {
				t.Errorf("matchesSelector() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesSelectorStateFilters(t *testing.T) {
	elem := &ParsedElement{
		Text:     "Checkbox",
		Enabled:  true,
		Selected: false,
		Focused:  true,
	}

	enabled := true
	if !matchesSelector(elem, flow.Selector{Enabled: &enabled}) {
		t.Error("expected match for enabled=true")
	}

	selected := true
	if matchesSelector(elem, flow.Selector{Selected: &selected}) {
		t.Error("expected no match for selected=true (elem is not selected)")
	}

	focused := true
	if !matchesSelector(elem, flow.Selector{Focused: &focused}) {
		t.Error("expected match for focused=true")
	}
}

func TestMatchesSelectorEmptyText(t *testing.T) {
	elem := &ParsedElement{
		Text: "",
	}

	// Empty selector should match
	if !matchesSelector(elem, flow.Selector{}) {
		t.Error("expected empty selector to match")
	}
}

func TestFilterLeftOf(t *testing.T) {
	// Create elements in a horizontal layout
	elements := []*ParsedElement{
		{Text: "Left", Bounds: core.Bounds{X: 0, Y: 100, Width: 100, Height: 50}},
		{Text: "Middle", Bounds: core.Bounds{X: 150, Y: 100, Width: 100, Height: 50}},
		{Text: "Right", Bounds: core.Bounds{X: 300, Y: 100, Width: 100, Height: 50}},
	}

	// Find elements left of Middle
	anchor := elements[1] // Middle
	result := FilterLeftOf(elements, anchor)

	if len(result) != 1 {
		t.Errorf("expected 1 element left of Middle, got %d", len(result))
	}
	if result[0].Text != "Left" {
		t.Errorf("expected 'Left', got %s", result[0].Text)
	}
}

func TestFilterRightOf(t *testing.T) {
	// Create elements in a horizontal layout
	elements := []*ParsedElement{
		{Text: "Left", Bounds: core.Bounds{X: 0, Y: 100, Width: 100, Height: 50}},
		{Text: "Middle", Bounds: core.Bounds{X: 150, Y: 100, Width: 100, Height: 50}},
		{Text: "Right", Bounds: core.Bounds{X: 300, Y: 100, Width: 100, Height: 50}},
	}

	// Find elements right of Middle
	anchor := elements[1] // Middle (ends at x=250)
	result := FilterRightOf(elements, anchor)

	if len(result) != 1 {
		t.Errorf("expected 1 element right of Middle, got %d", len(result))
	}
	if result[0].Text != "Right" {
		t.Errorf("expected 'Right', got %s", result[0].Text)
	}
}

func TestFilterLeftOfMultiple(t *testing.T) {
	// Multiple elements left - should sort by closest first
	elements := []*ParsedElement{
		{Text: "Far", Bounds: core.Bounds{X: 0, Y: 100, Width: 50, Height: 50}},
		{Text: "Near", Bounds: core.Bounds{X: 100, Y: 100, Width: 50, Height: 50}},
		{Text: "Anchor", Bounds: core.Bounds{X: 200, Y: 100, Width: 50, Height: 50}},
	}

	anchor := elements[2]
	result := FilterLeftOf(elements, anchor)

	if len(result) != 2 {
		t.Errorf("expected 2 elements, got %d", len(result))
	}
	// Nearest (highest right edge) should be first
	if result[0].Text != "Near" {
		t.Errorf("expected 'Near' first (closest), got %s", result[0].Text)
	}
}

func TestFilterRightOfMultiple(t *testing.T) {
	// Multiple elements right - should sort by closest first
	elements := []*ParsedElement{
		{Text: "Anchor", Bounds: core.Bounds{X: 0, Y: 100, Width: 50, Height: 50}},
		{Text: "Near", Bounds: core.Bounds{X: 100, Y: 100, Width: 50, Height: 50}},
		{Text: "Far", Bounds: core.Bounds{X: 200, Y: 100, Width: 50, Height: 50}},
	}

	anchor := elements[0]
	result := FilterRightOf(elements, anchor)

	if len(result) != 2 {
		t.Errorf("expected 2 elements, got %d", len(result))
	}
	// Nearest (lowest left edge) should be first
	if result[0].Text != "Near" {
		t.Errorf("expected 'Near' first (closest), got %s", result[0].Text)
	}
}

func TestFilterBelowMultiple(t *testing.T) {
	// Multiple elements below - should sort by closest first
	elements := []*ParsedElement{
		{Text: "Anchor", Bounds: core.Bounds{X: 100, Y: 0, Width: 50, Height: 50}},
		{Text: "Near", Bounds: core.Bounds{X: 100, Y: 100, Width: 50, Height: 50}},
		{Text: "Far", Bounds: core.Bounds{X: 100, Y: 200, Width: 50, Height: 50}},
	}

	anchor := elements[0]
	result := FilterBelow(elements, anchor)

	if len(result) != 2 {
		t.Errorf("expected 2 elements, got %d", len(result))
	}
	if result[0].Text != "Near" {
		t.Errorf("expected 'Near' first (closest), got %s", result[0].Text)
	}
}

func TestFilterAboveMultiple(t *testing.T) {
	// Multiple elements above - should sort by closest first
	elements := []*ParsedElement{
		{Text: "Far", Bounds: core.Bounds{X: 100, Y: 0, Width: 50, Height: 50}},
		{Text: "Near", Bounds: core.Bounds{X: 100, Y: 100, Width: 50, Height: 50}},
		{Text: "Anchor", Bounds: core.Bounds{X: 100, Y: 200, Width: 50, Height: 50}},
	}

	anchor := elements[2]
	result := FilterAbove(elements, anchor)

	if len(result) != 2 {
		t.Errorf("expected 2 elements, got %d", len(result))
	}
	if result[0].Text != "Near" {
		t.Errorf("expected 'Near' first (closest), got %s", result[0].Text)
	}
}

func TestFilterContainsDescendantsNoMatch(t *testing.T) {
	elements := []*ParsedElement{
		{Text: "Parent", Bounds: core.Bounds{X: 0, Y: 0, Width: 200, Height: 200}},
		{Text: "Child", Bounds: core.Bounds{X: 10, Y: 10, Width: 50, Height: 50}},
	}

	// Look for descendants that don't exist
	descendants := []*flow.Selector{
		{Text: "NonExistent"},
	}

	result := FilterContainsDescendants(elements, elements, descendants)
	if len(result) != 0 {
		t.Errorf("expected 0 elements, got %d", len(result))
	}
}

func TestIsInside(t *testing.T) {
	outer := core.Bounds{X: 0, Y: 0, Width: 100, Height: 100}

	tests := []struct {
		name  string
		inner core.Bounds
		want  bool
	}{
		{"fully inside", core.Bounds{X: 10, Y: 10, Width: 50, Height: 50}, true},
		{"same bounds", core.Bounds{X: 0, Y: 0, Width: 100, Height: 100}, true},
		{"left edge outside", core.Bounds{X: -10, Y: 10, Width: 50, Height: 50}, false},
		{"right edge outside", core.Bounds{X: 60, Y: 10, Width: 50, Height: 50}, false},
		{"top edge outside", core.Bounds{X: 10, Y: -10, Width: 50, Height: 50}, false},
		{"bottom edge outside", core.Bounds{X: 10, Y: 60, Width: 50, Height: 50}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInside(tt.inner, outer)
			if got != tt.want {
				t.Errorf("isInside() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSortByDistanceY(t *testing.T) {
	elements := []*ParsedElement{
		{Text: "Far", Bounds: core.Bounds{Y: 200}},
		{Text: "Near", Bounds: core.Bounds{Y: 110}},
		{Text: "Middle", Bounds: core.Bounds{Y: 150}},
	}

	sortByDistanceY(elements, 100)

	if elements[0].Text != "Near" {
		t.Errorf("expected 'Near' first, got %s", elements[0].Text)
	}
	if elements[1].Text != "Middle" {
		t.Errorf("expected 'Middle' second, got %s", elements[1].Text)
	}
	if elements[2].Text != "Far" {
		t.Errorf("expected 'Far' third, got %s", elements[2].Text)
	}
}

func TestSortByDistanceYReverse(t *testing.T) {
	elements := []*ParsedElement{
		{Text: "Far", Bounds: core.Bounds{Y: 0, Height: 50}},   // bottom at 50
		{Text: "Near", Bounds: core.Bounds{Y: 80, Height: 50}}, // bottom at 130
		{Text: "Middle", Bounds: core.Bounds{Y: 40, Height: 50}}, // bottom at 90
	}

	sortByDistanceYReverse(elements, 150)

	if elements[0].Text != "Near" {
		t.Errorf("expected 'Near' first (bottom at 130), got %s", elements[0].Text)
	}
}

func TestSortByDistanceX(t *testing.T) {
	elements := []*ParsedElement{
		{Text: "Far", Bounds: core.Bounds{X: 200}},
		{Text: "Near", Bounds: core.Bounds{X: 110}},
		{Text: "Middle", Bounds: core.Bounds{X: 150}},
	}

	sortByDistanceX(elements, 100)

	if elements[0].Text != "Near" {
		t.Errorf("expected 'Near' first, got %s", elements[0].Text)
	}
}

func TestSortByDistanceXReverse(t *testing.T) {
	elements := []*ParsedElement{
		{Text: "Far", Bounds: core.Bounds{X: 0, Width: 50}},
		{Text: "Near", Bounds: core.Bounds{X: 80, Width: 50}},
		{Text: "Middle", Bounds: core.Bounds{X: 40, Width: 50}},
	}

	sortByDistanceXReverse(elements, 150)

	if elements[0].Text != "Near" {
		t.Errorf("expected 'Near' first, got %s", elements[0].Text)
	}
}
