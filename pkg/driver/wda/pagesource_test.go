package wda

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Sample iOS page source XML for testing
const sampleIOSPageSource = `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" label="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeWindow type="XCUIElementTypeWindow" enabled="true" visible="true" x="0" y="0" width="390" height="844">
      <XCUIElementTypeOther type="XCUIElementTypeOther" enabled="true" visible="true" x="0" y="0" width="390" height="844">
        <XCUIElementTypeButton type="XCUIElementTypeButton" name="loginButton" label="Login" enabled="true" visible="true" x="50" y="100" width="290" height="50"/>
        <XCUIElementTypeTextField type="XCUIElementTypeTextField" name="emailField" label="Email" placeholderValue="Enter email" enabled="true" visible="true" x="50" y="200" width="290" height="44"/>
        <XCUIElementTypeSecureTextField type="XCUIElementTypeSecureTextField" name="passwordField" label="Password" enabled="true" visible="true" x="50" y="260" width="290" height="44"/>
        <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" label="Welcome to the app" enabled="true" visible="true" x="50" y="320" width="290" height="30"/>
        <XCUIElementTypeButton type="XCUIElementTypeButton" name="settingsButton" label="Settings" enabled="false" visible="true" x="50" y="400" width="100" height="40"/>
        <XCUIElementTypeSwitch type="XCUIElementTypeSwitch" name="notifySwitch" label="Notifications" enabled="true" visible="true" selected="true" x="250" y="400" width="60" height="40"/>
      </XCUIElementTypeOther>
    </XCUIElementTypeWindow>
  </XCUIElementTypeApplication>
</AppiumAUT>`

// TestParsePageSource tests parsing iOS page source XML
func TestParsePageSource(t *testing.T) {
	elements, err := ParsePageSource(sampleIOSPageSource)
	if err != nil {
		t.Fatalf("ParsePageSource failed: %v", err)
	}

	if len(elements) == 0 {
		t.Fatal("Expected elements, got none")
	}

	// Find login button
	var loginButton *ParsedElement
	for _, elem := range elements {
		if elem.Name == "loginButton" {
			loginButton = elem
			break
		}
	}

	if loginButton == nil {
		t.Fatal("Login button not found")
	}

	if loginButton.Label != "Login" {
		t.Errorf("Expected label 'Login', got '%s'", loginButton.Label)
	}

	if loginButton.Bounds.X != 50 || loginButton.Bounds.Y != 100 {
		t.Errorf("Expected bounds (50, 100), got (%d, %d)", loginButton.Bounds.X, loginButton.Bounds.Y)
	}

	if loginButton.Bounds.Width != 290 || loginButton.Bounds.Height != 50 {
		t.Errorf("Expected size (290, 50), got (%d, %d)", loginButton.Bounds.Width, loginButton.Bounds.Height)
	}
}

// TestParsePageSourceInvalidXML tests parsing invalid XML
func TestParsePageSourceInvalidXML(t *testing.T) {
	_, err := ParsePageSource("<invalid xml")
	if err == nil {
		t.Error("Expected error for invalid XML")
	}
}

// TestParsePageSourceEmptyXML tests parsing empty content
func TestParsePageSourceEmptyXML(t *testing.T) {
	_, err := ParsePageSource("")
	if err == nil {
		t.Error("Expected error for empty XML")
	}
}

// TestFilterBySelector tests filtering elements by selector
func TestFilterBySelector(t *testing.T) {
	elements, _ := ParsePageSource(sampleIOSPageSource)

	// Filter by text
	sel := flow.Selector{Text: "Login"}
	filtered := FilterBySelector(elements, sel)
	if len(filtered) != 1 {
		t.Errorf("Expected 1 element with text 'Login', got %d", len(filtered))
	}

	// Filter by ID
	sel = flow.Selector{ID: "emailField"}
	filtered = FilterBySelector(elements, sel)
	if len(filtered) != 1 {
		t.Errorf("Expected 1 element with id 'emailField', got %d", len(filtered))
	}

	// Filter by placeholder
	sel = flow.Selector{Text: "Enter email"}
	filtered = FilterBySelector(elements, sel)
	if len(filtered) != 1 {
		t.Errorf("Expected 1 element with placeholder 'Enter email', got %d", len(filtered))
	}
}

// TestFilterBySelectorStateFilters tests filtering by enabled/selected states
func TestFilterBySelectorStateFilters(t *testing.T) {
	elements, _ := ParsePageSource(sampleIOSPageSource)

	// Filter by enabled=false
	enabledFalse := false
	sel := flow.Selector{Enabled: &enabledFalse}
	filtered := FilterBySelector(elements, sel)
	if len(filtered) != 1 {
		t.Errorf("Expected 1 disabled element, got %d", len(filtered))
	}
	if filtered[0].Name != "settingsButton" {
		t.Errorf("Expected settings button, got %s", filtered[0].Name)
	}

	// Filter by selected=true
	selectedTrue := true
	sel = flow.Selector{Selected: &selectedTrue}
	filtered = FilterBySelector(elements, sel)
	if len(filtered) != 1 {
		t.Errorf("Expected 1 selected element, got %d", len(filtered))
	}
	if filtered[0].Name != "notifySwitch" {
		t.Errorf("Expected notify switch, got %s", filtered[0].Name)
	}
}

// TestFilterBySelectorWithSize tests filtering by size with tolerance
func TestFilterBySelectorWithSize(t *testing.T) {
	elements, _ := ParsePageSource(sampleIOSPageSource)

	// Filter by width with default tolerance
	sel := flow.Selector{Width: 292} // 290 +/- 5
	filtered := FilterBySelector(elements, sel)
	if len(filtered) == 0 {
		t.Error("Expected elements matching width within tolerance")
	}

	// Filter by exact size
	sel = flow.Selector{Width: 290, Height: 50, Tolerance: 0}
	filtered = FilterBySelector(elements, sel)
	if len(filtered) != 1 {
		t.Errorf("Expected 1 element with size 290x50, got %d", len(filtered))
	}
}

// TestFilterBySelectorCaseInsensitive tests case-insensitive text matching
func TestFilterBySelectorCaseInsensitive(t *testing.T) {
	elements, _ := ParsePageSource(sampleIOSPageSource)

	sel := flow.Selector{Text: "login"}
	filtered := FilterBySelector(elements, sel)
	if len(filtered) != 1 {
		t.Errorf("Expected 1 element with text 'login' (case insensitive), got %d", len(filtered))
	}
}

// TestFilterBySelectorRegex tests regex pattern matching
func TestFilterBySelectorRegex(t *testing.T) {
	elements, _ := ParsePageSource(sampleIOSPageSource)

	// Regex pattern
	sel := flow.Selector{Text: "Log.*"}
	filtered := FilterBySelector(elements, sel)
	if len(filtered) != 1 {
		t.Errorf("Expected 1 element matching 'Log.*', got %d", len(filtered))
	}

	// More complex regex
	sel = flow.Selector{Text: ".*Button"}
	filtered = FilterBySelector(elements, sel)
	// Should match loginButton, settingsButton names
	if len(filtered) < 2 {
		t.Errorf("Expected at least 2 elements matching '.*Button', got %d", len(filtered))
	}
}

// TestFilterBelow tests filtering elements below an anchor
func TestFilterBelow(t *testing.T) {
	elements := []*ParsedElement{
		{Label: "Top", Bounds: core.Bounds{X: 100, Y: 50, Width: 100, Height: 30}},
		{Label: "Middle", Bounds: core.Bounds{X: 100, Y: 100, Width: 100, Height: 30}},
		{Label: "Bottom", Bounds: core.Bounds{X: 100, Y: 200, Width: 100, Height: 30}},
	}

	anchor := elements[0] // Top element
	filtered := FilterBelow(elements, anchor)

	if len(filtered) != 2 {
		t.Errorf("Expected 2 elements below anchor, got %d", len(filtered))
	}

	// First element should be closest (Middle)
	if filtered[0].Label != "Middle" {
		t.Errorf("Expected Middle first, got %s", filtered[0].Label)
	}
}

// TestFilterAbove tests filtering elements above an anchor
func TestFilterAbove(t *testing.T) {
	elements := []*ParsedElement{
		{Label: "Top", Bounds: core.Bounds{X: 100, Y: 50, Width: 100, Height: 30}},
		{Label: "Middle", Bounds: core.Bounds{X: 100, Y: 100, Width: 100, Height: 30}},
		{Label: "Bottom", Bounds: core.Bounds{X: 100, Y: 200, Width: 100, Height: 30}},
	}

	anchor := elements[2] // Bottom element
	filtered := FilterAbove(elements, anchor)

	if len(filtered) != 2 {
		t.Errorf("Expected 2 elements above anchor, got %d", len(filtered))
	}

	// First element should be closest (Middle)
	if filtered[0].Label != "Middle" {
		t.Errorf("Expected Middle first, got %s", filtered[0].Label)
	}
}

// TestFilterLeftOf tests filtering elements left of an anchor
func TestFilterLeftOf(t *testing.T) {
	elements := []*ParsedElement{
		{Label: "Left", Bounds: core.Bounds{X: 50, Y: 100, Width: 40, Height: 30}},
		{Label: "Center", Bounds: core.Bounds{X: 150, Y: 100, Width: 40, Height: 30}},
		{Label: "Right", Bounds: core.Bounds{X: 250, Y: 100, Width: 40, Height: 30}},
	}

	anchor := elements[2] // Right element
	filtered := FilterLeftOf(elements, anchor)

	if len(filtered) != 2 {
		t.Errorf("Expected 2 elements left of anchor, got %d", len(filtered))
	}

	// First element should be closest (Center)
	if filtered[0].Label != "Center" {
		t.Errorf("Expected Center first, got %s", filtered[0].Label)
	}
}

// TestFilterRightOf tests filtering elements right of an anchor
func TestFilterRightOf(t *testing.T) {
	elements := []*ParsedElement{
		{Label: "Left", Bounds: core.Bounds{X: 50, Y: 100, Width: 40, Height: 30}},
		{Label: "Center", Bounds: core.Bounds{X: 150, Y: 100, Width: 40, Height: 30}},
		{Label: "Right", Bounds: core.Bounds{X: 250, Y: 100, Width: 40, Height: 30}},
	}

	anchor := elements[0] // Left element
	filtered := FilterRightOf(elements, anchor)

	if len(filtered) != 2 {
		t.Errorf("Expected 2 elements right of anchor, got %d", len(filtered))
	}

	// First element should be closest (Center)
	if filtered[0].Label != "Center" {
		t.Errorf("Expected Center first, got %s", filtered[0].Label)
	}
}

// TestFilterChildOf tests filtering elements inside a parent
func TestFilterChildOf(t *testing.T) {
	elements := []*ParsedElement{
		{Label: "Parent", Bounds: core.Bounds{X: 0, Y: 0, Width: 200, Height: 200}},
		{Label: "Inside", Bounds: core.Bounds{X: 10, Y: 10, Width: 50, Height: 50}},
		{Label: "Outside", Bounds: core.Bounds{X: 250, Y: 250, Width: 50, Height: 50}},
	}

	anchor := elements[0] // Parent
	filtered := FilterChildOf(elements, anchor)

	if len(filtered) != 2 { // Parent itself and Inside
		t.Errorf("Expected 2 elements inside parent, got %d", len(filtered))
	}
}

// TestFilterContainsChild tests filtering elements that contain an anchor
func TestFilterContainsChild(t *testing.T) {
	elements := []*ParsedElement{
		{Label: "Outer", Bounds: core.Bounds{X: 0, Y: 0, Width: 300, Height: 300}},
		{Label: "Inner", Bounds: core.Bounds{X: 50, Y: 50, Width: 100, Height: 100}},
		{Label: "Outside", Bounds: core.Bounds{X: 400, Y: 400, Width: 50, Height: 50}},
	}

	anchor := elements[1] // Inner
	filtered := FilterContainsChild(elements, anchor)

	if len(filtered) != 2 { // Outer and Inner itself
		t.Errorf("Expected 2 elements containing anchor, got %d", len(filtered))
	}
}

// TestFilterInsideOf tests filtering elements whose center is inside anchor
func TestFilterInsideOf(t *testing.T) {
	elements := []*ParsedElement{
		{Label: "CenterInside", Bounds: core.Bounds{X: 50, Y: 50, Width: 20, Height: 20}},      // center at (60, 60)
		{Label: "CenterOutside", Bounds: core.Bounds{X: 180, Y: 180, Width: 100, Height: 100}}, // center at (230, 230)
		{Label: "PartialOverlap", Bounds: core.Bounds{X: 80, Y: 80, Width: 100, Height: 100}},  // center at (130, 130), inside
	}

	anchor := &ParsedElement{Bounds: core.Bounds{X: 0, Y: 0, Width: 200, Height: 200}}
	filtered := FilterInsideOf(elements, anchor)

	if len(filtered) != 2 {
		t.Errorf("Expected 2 elements with center inside anchor, got %d", len(filtered))
	}
}

// TestFilterContainsDescendants tests filtering elements containing all descendants
func TestFilterContainsDescendants(t *testing.T) {
	elements := []*ParsedElement{
		{Label: "Container", Bounds: core.Bounds{X: 0, Y: 0, Width: 400, Height: 400}},
		{Label: "Child1", Bounds: core.Bounds{X: 10, Y: 10, Width: 50, Height: 50}},
		{Label: "Child2", Bounds: core.Bounds{X: 100, Y: 10, Width: 50, Height: 50}},
		{Label: "Outside", Bounds: core.Bounds{X: 500, Y: 500, Width: 50, Height: 50}},
	}

	descendants := []*flow.Selector{
		{Text: "Child1"},
		{Text: "Child2"},
	}

	filtered := FilterContainsDescendants(elements, elements, descendants)

	if len(filtered) != 1 {
		t.Errorf("Expected 1 container with both children, got %d", len(filtered))
	}
	if filtered[0].Label != "Container" {
		t.Errorf("Expected Container, got %s", filtered[0].Label)
	}
}

// TestDeepestMatchingElement tests selecting the deepest element
func TestDeepestMatchingElement(t *testing.T) {
	elements := []*ParsedElement{
		{Label: "Shallow", Depth: 1},
		{Label: "Deep", Depth: 5},
		{Label: "Medium", Depth: 3},
	}

	deepest := DeepestMatchingElement(elements)
	if deepest == nil {
		t.Fatal("Expected an element")
	}
	if deepest.Label != "Deep" {
		t.Errorf("Expected 'Deep', got '%s'", deepest.Label)
	}
}

// TestDeepestMatchingElementEmpty tests with empty list
func TestDeepestMatchingElementEmpty(t *testing.T) {
	deepest := DeepestMatchingElement(nil)
	if deepest != nil {
		t.Error("Expected nil for empty list")
	}
}

// TestLooksLikeRegex tests regex detection
func TestLooksLikeRegex(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"hello", false},
		{"hello.*world", true}, // .* is regex
		{"hello.+world", true}, // .+ is regex
		{"hello.?world", true}, // .? is regex
		{"hello.world", false}, // standalone period is NOT regex (domain-like)
		{"^start", true},
		{"end$", true},
		{"[abc]", true},
		{"a+b*c?", true},
		{"(group)", true},
		{"{3,5}", true},
		{"a|b", true},
		{"plain text", false},
		{"hello\\.", false},             // escaped dot
		{"mastodon.social", false},      // domain name
		{"Join mastodon.social", false}, // button text with domain
		{"v1.2.3", false},               // version number
		{"file.txt", false},             // filename
	}

	for _, tc := range tests {
		result := looksLikeRegex(tc.input)
		if result != tc.expected {
			t.Errorf("looksLikeRegex(%q) = %v, want %v", tc.input, result, tc.expected)
		}
	}
}

// TestContainsIgnoreCase tests case-insensitive contains
func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		s, substr string
		expected  bool
	}{
		{"Hello World", "world", true},
		{"Hello World", "HELLO", true},
		{"Hello World", "hello world", true},
		{"Hello World", "xyz", false},
		{"", "test", false},
		{"Test", "", true},
	}

	for _, tc := range tests {
		result := containsIgnoreCase(tc.s, tc.substr)
		if result != tc.expected {
			t.Errorf("containsIgnoreCase(%q, %q) = %v, want %v", tc.s, tc.substr, result, tc.expected)
		}
	}
}

// TestWithinTolerance tests tolerance checking
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
		{100, 100, 0, true},
		{101, 100, 0, false},
	}

	for _, tc := range tests {
		result := withinTolerance(tc.actual, tc.expected, tc.tolerance)
		if result != tc.want {
			t.Errorf("withinTolerance(%d, %d, %d) = %v, want %v", tc.actual, tc.expected, tc.tolerance, result, tc.want)
		}
	}
}

// TestFlattenElement tests flattening element tree
func TestFlattenElement(t *testing.T) {
	root := &ParsedElement{
		Label: "Root",
		Children: []*ParsedElement{
			{
				Label: "Child1",
				Children: []*ParsedElement{
					{Label: "GrandChild1"},
				},
			},
			{Label: "Child2"},
		},
	}

	flat := flattenElement(root, 0)

	if len(flat) != 4 {
		t.Errorf("Expected 4 flattened elements, got %d", len(flat))
	}

	// Check depths
	expected := map[string]int{
		"Root":        0,
		"Child1":      1,
		"GrandChild1": 2,
		"Child2":      1,
	}

	for _, elem := range flat {
		expectedDepth, ok := expected[elem.Label]
		if !ok {
			t.Errorf("Unexpected element: %s", elem.Label)
			continue
		}
		if elem.Depth != expectedDepth {
			t.Errorf("Element %s has depth %d, want %d", elem.Label, elem.Depth, expectedDepth)
		}
	}
}

func TestMatchesID(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		id      string
		want    bool
	}{
		{"literal contains", "login", "loginButton", true},
		{"literal no match", "signup", "loginButton", false},
		{"regex match", "login.*Button", "loginButton", true},
		{"regex no match", "^signup", "loginButton", false},
		{"wildcard match", "item_.*", "item_abc", true},
		{"regex digit match", "item_\\d+", "item_123", true},
		{"invalid regex fallback", "[invalid", "test[invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesID(tt.pattern, tt.id)
			if got != tt.want {
				t.Errorf("matchesID(%q, %q) = %v, want %v", tt.pattern, tt.id, got, tt.want)
			}
		})
	}
}

func TestFilterBySelectorRegexID(t *testing.T) {
	elements := []*ParsedElement{
		{Name: "loginButton", Label: "Login"},
		{Name: "settingsButton", Label: "Settings"},
		{Name: "logoutButton", Label: "Logout"},
		{Name: "profileView", Label: "Profile"},
	}

	// Regex ID should match multiple buttons
	sel := flow.Selector{ID: ".*Button"}
	filtered := FilterBySelector(elements, sel)
	if len(filtered) != 3 {
		t.Errorf("Expected 3 elements matching '.*Button', got %d", len(filtered))
	}

	// More specific regex
	sel = flow.Selector{ID: "log.*Button"}
	filtered = FilterBySelector(elements, sel)
	if len(filtered) != 2 {
		t.Errorf("Expected 2 elements matching 'log.*Button', got %d", len(filtered))
	}
}

// TestMatchesTextWithNewlines tests text matching with newlines
func TestMatchesTextWithNewlines(t *testing.T) {
	// Test that newlines are handled in regex matching
	result := matchesText("Hello.*World", "Hello\nWorld", "", "")
	if !result {
		t.Error("Expected match with newline replaced")
	}

	// Exact match should work
	result = matchesText("Hello\nWorld", "Hello\nWorld", "", "")
	if !result {
		t.Error("Expected exact match with newline")
	}
}

// TestParsePageSourceWithElementAttributes tests parsing various attributes
func TestParsePageSourceWithElementAttributes(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeTextField name="myField" label="My Field" value="Current Value" placeholderValue="Placeholder" enabled="true" visible="true" selected="false" focused="true" x="10" y="20" width="200" height="44"/>
</AppiumAUT>`

	elements, err := ParsePageSource(xmlData)
	if err != nil {
		t.Fatalf("ParsePageSource failed: %v", err)
	}

	if len(elements) != 1 {
		t.Fatalf("Expected 1 element, got %d", len(elements))
	}

	elem := elements[0]
	if elem.Name != "myField" {
		t.Errorf("Name: got %s, want myField", elem.Name)
	}
	if elem.Label != "My Field" {
		t.Errorf("Label: got %s, want 'My Field'", elem.Label)
	}
	if elem.Value != "Current Value" {
		t.Errorf("Value: got %s, want 'Current Value'", elem.Value)
	}
	if elem.PlaceholderValue != "Placeholder" {
		t.Errorf("PlaceholderValue: got %s, want 'Placeholder'", elem.PlaceholderValue)
	}
	if !elem.Enabled {
		t.Error("Expected enabled=true")
	}
	if !elem.Displayed {
		t.Error("Expected visible=true")
	}
	if elem.Selected {
		t.Error("Expected selected=false")
	}
	if !elem.Focused {
		t.Error("Expected focused=true")
	}
}

func TestSelectByIndex(t *testing.T) {
	candidates := []*ParsedElement{
		{Label: "First", Depth: 1},
		{Label: "Second", Depth: 3}, // deepest
		{Label: "Third", Depth: 2},
	}

	tests := []struct {
		name     string
		index    string
		expected string
	}{
		{"no index returns deepest", "", "Second"},
		{"index 0 returns first", "0", "First"},
		{"index 1 returns second", "1", "Second"},
		{"index 2 returns third", "2", "Third"},
		{"index -1 returns last", "-1", "Third"},
		{"index -2 returns second", "-2", "Second"},
		{"out of range defaults to first", "99", "First"},
		{"negative out of range defaults to first", "-99", "First"},
		{"non-numeric defaults to first", "abc", "First"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectByIndex(candidates, tt.index)
			if got.Label != tt.expected {
				t.Errorf("SelectByIndex(index=%q) = %q, want %q", tt.index, got.Label, tt.expected)
			}
		})
	}
}

func TestFilterOutOfBounds(t *testing.T) {
	screenW, screenH := 390, 844

	elements := []*ParsedElement{
		{Label: "on-screen", Bounds: core.Bounds{X: 50, Y: 100, Width: 200, Height: 50}},
		{Label: "off-screen-right", Bounds: core.Bounds{X: 500, Y: 100, Width: 200, Height: 50}},
		{Label: "off-screen-below", Bounds: core.Bounds{X: 50, Y: 900, Width: 200, Height: 50}},
		{Label: "partially-visible", Bounds: core.Bounds{X: 300, Y: 100, Width: 200, Height: 50}},  // 90/200 = 45% visible
		{Label: "barely-off", Bounds: core.Bounds{X: 380, Y: 100, Width: 200, Height: 50}},          // 10/200 = 5% visible → filtered
		{Label: "full-screen", Bounds: core.Bounds{X: 0, Y: 0, Width: 390, Height: 844}},
	}

	result := FilterOutOfBounds(elements, screenW, screenH)

	expected := map[string]bool{
		"on-screen":         true,
		"partially-visible": true,
		"full-screen":       true,
	}

	if len(result) != len(expected) {
		t.Errorf("FilterOutOfBounds returned %d elements, want %d", len(result), len(expected))
		for _, e := range result {
			t.Logf("  kept: %s (visible=%.2f)", e.Label, e.Bounds.VisiblePercentage(screenW, screenH))
		}
		return
	}

	for _, e := range result {
		if !expected[e.Label] {
			t.Errorf("unexpected element kept: %s", e.Label)
		}
	}
}

// TestHasVisibleDescendant covers the rescue heuristic for RN container
// testIDs vs hidden-but-still-mounted screens.
func TestHasVisibleDescendant(t *testing.T) {
	// RN-style container: parent visible=false, child visible=true
	rnChild := &ParsedElement{Name: "child", Displayed: true}
	rnContainer := &ParsedElement{Name: "rn-container", Displayed: false, Children: []*ParsedElement{rnChild}}
	rnChild.Parent = rnContainer

	if !HasVisibleDescendant(rnContainer) {
		t.Error("RN container with visible child should be rescuable")
	}

	// Hidden screen: container visible=false, all descendants visible=false
	hiddenChild := &ParsedElement{Name: "login-button", Displayed: false}
	hiddenInner := &ParsedElement{Name: "form", Displayed: false, Children: []*ParsedElement{hiddenChild}}
	hiddenChild.Parent = hiddenInner
	hiddenContainer := &ParsedElement{Name: "login-screen", Displayed: false, Children: []*ParsedElement{hiddenInner}}
	hiddenInner.Parent = hiddenContainer

	if HasVisibleDescendant(hiddenContainer) {
		t.Error("hidden screen with all-invisible descendants must NOT be rescuable")
	}
	if HasVisibleDescendant(hiddenChild) {
		t.Error("leaf invisible element must NOT be rescuable")
	}

	// Deep nesting: visible descendant several levels down still rescues
	leaf := &ParsedElement{Name: "deep-text", Displayed: true}
	mid := &ParsedElement{Name: "mid", Displayed: false, Children: []*ParsedElement{leaf}}
	root := &ParsedElement{Name: "root", Displayed: false, Children: []*ParsedElement{mid}}
	if !HasVisibleDescendant(root) {
		t.Error("deep visible descendant should rescue ancestor")
	}
}

// TestFilterBySelectorPUAGlyph reproduces issue #89: selectors containing
// Private Use Area icon-font glyphs (e.g. U+F0142 from Material Design Icons)
// must match host-side against the page-source label.
func TestFilterBySelectorPUAGlyph(t *testing.T) {
	const puaPageSource = `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="PlantNet" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeWindow type="XCUIElementTypeWindow" enabled="true" visible="true" x="0" y="0" width="390" height="844">
      <XCUIElementTypeButton type="XCUIElementTypeButton" name="Map, ` + "\U000F0142" + `" label="Map, ` + "\U000F0142" + `" enabled="true" visible="true" x="12" y="429" width="378" height="80"/>
      <XCUIElementTypeButton type="XCUIElementTypeButton" label="NORTHERN AMERICA (10), ` + "\U000F0140" + `" enabled="true" visible="true" x="12" y="520" width="378" height="80"/>
    </XCUIElementTypeWindow>
  </XCUIElementTypeApplication>
</AppiumAUT>`

	elements, err := ParsePageSource(puaPageSource)
	if err != nil {
		t.Fatalf("ParsePageSource failed on PUA glyph XML: %v", err)
	}

	sel := flow.Selector{Text: "Map, \U000F0142"}
	filtered := FilterBySelector(elements, sel)
	if len(filtered) != 1 {
		t.Errorf("expected 1 element matching PUA selector, got %d", len(filtered))
	}

	// Parenthesised text triggers the regex path; PUA glyph must survive it.
	sel = flow.Selector{Text: "NORTHERN AMERICA (10), \U000F0140"}
	filtered = FilterBySelector(elements, sel)
	if len(filtered) != 1 {
		t.Errorf("expected 1 element matching parenthesised PUA selector, got %d", len(filtered))
	}
}

// TestMatchesTextNFCNormalization verifies composed (NFC) and decomposed
// (NFD) accent forms match each other in both directions (issue #89 —
// French labels with selector text generated by JS can disagree on form).
func TestMatchesTextNFCNormalization(t *testing.T) {
	nfc := "États-Unis"                  // U+00C9
	nfd := "États-Unis"            // E + combining acute
	if nfc == nfd {
		t.Fatal("test fixture invalid: NFC and NFD forms compare equal")
	}
	if !matchesText(nfc, nfd) {
		t.Error("NFC pattern should match NFD text")
	}
	if !matchesText(nfd, nfc) {
		t.Error("NFD pattern should match NFC text")
	}
}

// TestClosestTexts verifies near-miss ranking used in "not found" errors.
func TestClosestTexts(t *testing.T) {
	elements := []*ParsedElement{
		{Label: "Carte, \U000F0142"},
		{Label: "Recommandation, Automatiquement avec votre GPS, \U000F0142"},
		{Label: "Réglages"},
		{Label: ""},
	}

	closest := ClosestTexts(elements, "Map, \U000F0142", 3)
	if len(closest) == 0 {
		t.Fatal("expected at least one close text")
	}
	// "Carte, <glyph>" shares ", " and " <glyph>" bigrams and is shortest —
	// it must rank first.
	if closest[0] != `"Carte, \U000f0142"` {
		t.Errorf("expected Carte row ranked first, got %q", closest[0])
	}

	// No overlap at all → no suggestions.
	none := ClosestTexts([]*ParsedElement{{Label: "xyz"}}, "Map", 3)
	if len(none) != 0 {
		t.Errorf("expected no suggestions for disjoint texts, got %v", none)
	}
}
