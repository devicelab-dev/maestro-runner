package appium

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestParseAndroidPageSource(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,1920]" class="android.widget.FrameLayout" enabled="true" displayed="true">
    <android.widget.TextView text="Hello World" resource-id="com.example:id/title" bounds="[100,200][400,250]" enabled="true" clickable="true" />
    <android.widget.Button text="Click Me" content-desc="Submit button" bounds="[100,300][400,380]" enabled="true" clickable="true" />
    <android.widget.EditText hint="Enter name" bounds="[100,400][400,480]" enabled="true" focused="true" />
  </android.widget.FrameLayout>
</hierarchy>`

	elements, platform, err := ParsePageSource(xml)
	if err != nil {
		t.Fatalf("ParsePageSource failed: %v", err)
	}

	if platform != "android" {
		t.Errorf("Expected platform 'android', got '%s'", platform)
	}

	if len(elements) != 4 {
		t.Errorf("Expected 4 elements, got %d", len(elements))
	}

	// Check TextView
	var textView *ParsedElement
	for _, e := range elements {
		if e.Text == "Hello World" {
			textView = e
			break
		}
	}
	if textView == nil {
		t.Fatal("TextView not found")
	}
	if textView.ResourceID != "com.example:id/title" {
		t.Errorf("Expected resource-id 'com.example:id/title', got '%s'", textView.ResourceID)
	}
	if textView.Bounds.X != 100 || textView.Bounds.Y != 200 {
		t.Errorf("Unexpected bounds: %+v", textView.Bounds)
	}

	// Check Button with content-desc
	var button *ParsedElement
	for _, e := range elements {
		if e.ContentDesc == "Submit button" {
			button = e
			break
		}
	}
	if button == nil {
		t.Fatal("Button with content-desc not found")
	}

	// Check EditText with hint
	var editText *ParsedElement
	for _, e := range elements {
		if e.HintText == "Enter name" {
			editText = e
			break
		}
	}
	if editText == nil {
		t.Fatal("EditText with hint not found")
	}
	if !editText.Focused {
		t.Error("EditText should be focused")
	}
}

func TestParseIOSPageSource(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" label="Test App" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="submitBtn" label="Submit" enabled="true" visible="true" x="100" y="200" width="100" height="44" />
    <XCUIElementTypeTextField type="XCUIElementTypeTextField" name="emailField" label="" value="test@example.com" placeholderValue="Enter email" enabled="true" visible="true" x="50" y="300" width="300" height="44" />
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" label="Welcome" enabled="true" visible="true" x="50" y="100" width="200" height="30" />
  </XCUIElementTypeApplication>
</AppiumAUT>`

	elements, platform, err := ParsePageSource(xml)
	if err != nil {
		t.Fatalf("ParsePageSource failed: %v", err)
	}

	if platform != "ios" {
		t.Errorf("Expected platform 'ios', got '%s'", platform)
	}

	if len(elements) != 4 {
		t.Errorf("Expected 4 elements, got %d", len(elements))
	}

	// Check Button
	var button *ParsedElement
	for _, e := range elements {
		if e.Name == "submitBtn" {
			button = e
			break
		}
	}
	if button == nil {
		t.Fatal("Button not found")
	}
	if button.Label != "Submit" {
		t.Errorf("Expected label 'Submit', got '%s'", button.Label)
	}
	if button.Type != "XCUIElementTypeButton" {
		t.Errorf("Expected type 'XCUIElementTypeButton', got '%s'", button.Type)
	}

	// Check TextField
	var textField *ParsedElement
	for _, e := range elements {
		if e.Name == "emailField" {
			textField = e
			break
		}
	}
	if textField == nil {
		t.Fatal("TextField not found")
	}
	if textField.Value != "test@example.com" {
		t.Errorf("Expected value 'test@example.com', got '%s'", textField.Value)
	}
	if textField.PlaceholderValue != "Enter email" {
		t.Errorf("Expected placeholder 'Enter email', got '%s'", textField.PlaceholderValue)
	}
}

func TestFilterBySelector_Android(t *testing.T) {
	elements := []*ParsedElement{
		{Text: "Hello", ResourceID: "id/hello", Enabled: true},
		{Text: "World", ResourceID: "id/world", Enabled: false},
		{Text: "Hello World", ResourceID: "id/greeting", Enabled: true},
		{ContentDesc: "Hello button", ResourceID: "id/btn", Enabled: true},
	}

	tests := []struct {
		name     string
		selector flow.Selector
		expected int
	}{
		{"by text exact", flow.Selector{Text: "Hello"}, 3}, // matches Hello, Hello World, Hello button
		{"by text contains", flow.Selector{Text: "World"}, 2},
		{"by ID", flow.Selector{ID: "id/hello"}, 1},
		{"by ID partial", flow.Selector{ID: "id/"}, 4},
		{"by enabled true", flow.Selector{Enabled: boolPtr(true)}, 3},
		{"by enabled false", flow.Selector{Enabled: boolPtr(false)}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterBySelector(elements, tt.selector, "android")
			if len(result) != tt.expected {
				t.Errorf("Expected %d elements, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestFilterBySelector_iOS(t *testing.T) {
	elements := []*ParsedElement{
		{Label: "Submit", Name: "submitBtn", Enabled: true},
		{Label: "Cancel", Name: "cancelBtn", Enabled: true},
		{Label: "Submit Order", Name: "orderBtn", Enabled: false},
		{Value: "Submit value", Name: "valueField", Enabled: true},
	}

	tests := []struct {
		name     string
		selector flow.Selector
		expected int
	}{
		{"by text (label)", flow.Selector{Text: "Submit"}, 3},
		{"by ID (name)", flow.Selector{ID: "submitBtn"}, 1},
		{"by ID partial", flow.Selector{ID: "Btn"}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterBySelector(elements, tt.selector, "ios")
			if len(result) != tt.expected {
				t.Errorf("Expected %d elements, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestFilterBySelector_Regex(t *testing.T) {
	elements := []*ParsedElement{
		{Text: "Price: $10.00"},
		{Text: "Price: $25.50"},
		{Text: "Total: $100"},
		{Text: "No price here"},
	}

	sel := flow.Selector{Text: `Price: \$\d+`}
	result := FilterBySelector(elements, sel, "android")

	if len(result) != 2 {
		t.Errorf("Expected 2 elements matching regex, got %d", len(result))
	}
}

func TestFilterBySelector_Size(t *testing.T) {
	elements := []*ParsedElement{
		{Bounds: core.Bounds{Width: 100, Height: 50}},
		{Bounds: core.Bounds{Width: 102, Height: 48}},
		{Bounds: core.Bounds{Width: 200, Height: 100}},
	}

	sel := flow.Selector{Width: 100, Height: 50, Tolerance: 5}
	result := FilterBySelector(elements, sel, "android")

	if len(result) != 2 {
		t.Errorf("Expected 2 elements within tolerance, got %d", len(result))
	}
}

func TestFilterBelow(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 100, Y: 100, Width: 200, Height: 50}}
	elements := []*ParsedElement{
		{Text: "Above", Bounds: core.Bounds{X: 100, Y: 50, Width: 200, Height: 40}},
		{Text: "Below1", Bounds: core.Bounds{X: 100, Y: 160, Width: 200, Height: 40}},
		{Text: "Below2", Bounds: core.Bounds{X: 100, Y: 220, Width: 200, Height: 40}},
		{Text: "Overlapping", Bounds: core.Bounds{X: 100, Y: 140, Width: 200, Height: 40}},
	}

	result := FilterBelow(elements, anchor)

	if len(result) != 2 {
		t.Errorf("Expected 2 elements below, got %d", len(result))
	}
	if result[0].Text != "Below1" {
		t.Errorf("Expected 'Below1' first (closest), got '%s'", result[0].Text)
	}
}

func TestFilterAbove(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 100, Y: 200, Width: 200, Height: 50}}
	elements := []*ParsedElement{
		{Text: "Above1", Bounds: core.Bounds{X: 100, Y: 50, Width: 200, Height: 40}},
		{Text: "Above2", Bounds: core.Bounds{X: 100, Y: 100, Width: 200, Height: 40}},
		{Text: "Below", Bounds: core.Bounds{X: 100, Y: 260, Width: 200, Height: 40}},
	}

	result := FilterAbove(elements, anchor)

	if len(result) != 2 {
		t.Errorf("Expected 2 elements above, got %d", len(result))
	}
	if result[0].Text != "Above2" {
		t.Errorf("Expected 'Above2' first (closest), got '%s'", result[0].Text)
	}
}

func TestFilterLeftOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 200, Y: 100, Width: 100, Height: 50}}
	elements := []*ParsedElement{
		{Text: "Left1", Bounds: core.Bounds{X: 50, Y: 100, Width: 50, Height: 50}},
		{Text: "Left2", Bounds: core.Bounds{X: 120, Y: 100, Width: 50, Height: 50}},
		{Text: "Right", Bounds: core.Bounds{X: 320, Y: 100, Width: 50, Height: 50}},
	}

	result := FilterLeftOf(elements, anchor)

	if len(result) != 2 {
		t.Errorf("Expected 2 elements left, got %d", len(result))
	}
	if result[0].Text != "Left2" {
		t.Errorf("Expected 'Left2' first (closest), got '%s'", result[0].Text)
	}
}

func TestFilterRightOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 100, Y: 100, Width: 100, Height: 50}}
	elements := []*ParsedElement{
		{Text: "Left", Bounds: core.Bounds{X: 20, Y: 100, Width: 50, Height: 50}},
		{Text: "Right1", Bounds: core.Bounds{X: 220, Y: 100, Width: 50, Height: 50}},
		{Text: "Right2", Bounds: core.Bounds{X: 300, Y: 100, Width: 50, Height: 50}},
	}

	result := FilterRightOf(elements, anchor)

	if len(result) != 2 {
		t.Errorf("Expected 2 elements right, got %d", len(result))
	}
	if result[0].Text != "Right1" {
		t.Errorf("Expected 'Right1' first (closest), got '%s'", result[0].Text)
	}
}

func TestFilterChildOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 100, Y: 100, Width: 200, Height: 200}}
	elements := []*ParsedElement{
		{Text: "Inside", Bounds: core.Bounds{X: 120, Y: 120, Width: 50, Height: 50}},
		{Text: "Outside", Bounds: core.Bounds{X: 50, Y: 50, Width: 30, Height: 30}},
		{Text: "PartiallyInside", Bounds: core.Bounds{X: 250, Y: 150, Width: 100, Height: 50}},
	}

	result := FilterChildOf(elements, anchor)

	if len(result) != 1 {
		t.Errorf("Expected 1 element inside, got %d", len(result))
	}
	if result[0].Text != "Inside" {
		t.Errorf("Expected 'Inside', got '%s'", result[0].Text)
	}
}

func TestFilterContainsChild(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 150, Y: 150, Width: 50, Height: 50}}
	elements := []*ParsedElement{
		{Text: "Container", Bounds: core.Bounds{X: 100, Y: 100, Width: 200, Height: 200}},
		{Text: "SmallBox", Bounds: core.Bounds{X: 140, Y: 140, Width: 60, Height: 60}},
		{Text: "Outside", Bounds: core.Bounds{X: 400, Y: 400, Width: 100, Height: 100}},
	}

	result := FilterContainsChild(elements, anchor)

	if len(result) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(result))
	}
}

func TestFilterInsideOf(t *testing.T) {
	anchor := &ParsedElement{Bounds: core.Bounds{X: 100, Y: 100, Width: 200, Height: 200}}
	elements := []*ParsedElement{
		{Text: "CenterInside", Bounds: core.Bounds{X: 150, Y: 150, Width: 100, Height: 100}},   // center at 200,200 - inside
		{Text: "CenterOutside", Bounds: core.Bounds{X: 350, Y: 350, Width: 100, Height: 100}},  // center at 400,400 - outside
		{Text: "EdgeCase", Bounds: core.Bounds{X: 50, Y: 150, Width: 100, Height: 100}},        // center at 100,200 - on edge (included)
	}

	result := FilterInsideOf(elements, anchor)

	// CenterInside: center(200,200) is inside [100,100]-[300,300] (uses <=)
	// EdgeCase: center(100,200) is on edge, included with <=
	// CenterOutside: center(400,400) is outside
	if len(result) != 2 {
		t.Errorf("Expected 2 elements with center inside, got %d", len(result))
	}
}

func TestDeepestMatchingElement(t *testing.T) {
	elements := []*ParsedElement{
		{Text: "Depth0", Depth: 0},
		{Text: "Depth2", Depth: 2},
		{Text: "Depth1", Depth: 1},
		{Text: "Depth3", Depth: 3},
	}

	result := DeepestMatchingElement(elements)

	if result.Text != "Depth3" {
		t.Errorf("Expected 'Depth3', got '%s'", result.Text)
	}
}

func TestSortClickableFirst(t *testing.T) {
	elements := []*ParsedElement{
		{Text: "NonClickable1", Clickable: false},
		{Text: "Clickable1", Clickable: true},
		{Text: "NonClickable2", Clickable: false},
		{Text: "Clickable2", Clickable: true},
	}

	result := SortClickableFirst(elements)

	if !result[0].Clickable || !result[1].Clickable {
		t.Error("Clickable elements should be first")
	}
	if result[2].Clickable || result[3].Clickable {
		t.Error("Non-clickable elements should be last")
	}
}

func TestParseBounds(t *testing.T) {
	tests := []struct {
		input    string
		expected core.Bounds
	}{
		{"[0,0][100,200]", core.Bounds{X: 0, Y: 0, Width: 100, Height: 200}},
		{"[50,100][150,250]", core.Bounds{X: 50, Y: 100, Width: 100, Height: 150}},
		{"invalid", core.Bounds{}},
	}

	for _, tt := range tests {
		result := parseBounds(tt.input)
		if result != tt.expected {
			t.Errorf("parseBounds(%s) = %+v, expected %+v", tt.input, result, tt.expected)
		}
	}
}

func TestMatchesText(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		texts    []string
		expected bool
	}{
		{"exact match", "Hello", []string{"Hello"}, true},
		{"contains", "ell", []string{"Hello"}, true},
		{"case insensitive", "HELLO", []string{"hello"}, true},
		{"no match", "xyz", []string{"Hello"}, false},
		{"regex match", "\\d+", []string{"Price: 100"}, true},
		{"regex no match", "^\\d+$", []string{"Price: 100"}, false},
		{"multiple texts", "World", []string{"Hello", "World"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesText(tt.pattern, tt.texts...)
			if result != tt.expected {
				t.Errorf("matchesText(%s, %v) = %v, expected %v", tt.pattern, tt.texts, result, tt.expected)
			}
		})
	}
}

func TestLooksLikeRegex(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"hello", false},
		{"hello.*world", true},          // .* is regex
		{"hello.+world", true},          // .+ is regex
		{"hello.?world", true},          // .? is regex
		{"hello.world", false},          // standalone period is NOT regex (domain-like)
		{"[abc]", true},
		{"a+b", true},
		{"a?b", true},
		{"a|b", true},
		{"^start", true},
		{"end$", true},
		{"(group)", true},
		{`a\*b`, false},                 // escaped
		{"mastodon.social", false},      // domain name
		{"Join mastodon.social", false}, // button text with domain
		{"v1.2.3", false},               // version number
		{"file.txt", false},             // filename
	}

	for _, tt := range tests {
		result := looksLikeRegex(tt.input)
		if result != tt.expected {
			t.Errorf("looksLikeRegex(%s) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestFilterContainsDescendants(t *testing.T) {
	allElements := []*ParsedElement{
		{Text: "Container", Bounds: core.Bounds{X: 0, Y: 0, Width: 500, Height: 500}},
		{Text: "Child1", Bounds: core.Bounds{X: 50, Y: 50, Width: 100, Height: 50}},
		{Text: "Child2", Bounds: core.Bounds{X: 50, Y: 150, Width: 100, Height: 50}},
		{Text: "Outside", Bounds: core.Bounds{X: 600, Y: 600, Width: 100, Height: 50}},
	}

	containers := []*ParsedElement{allElements[0]}
	descendants := []*flow.Selector{
		{Text: "Child1"},
		{Text: "Child2"},
	}

	result := FilterContainsDescendants(containers, allElements, descendants, "android")

	if len(result) != 1 {
		t.Errorf("Expected 1 container with descendants, got %d", len(result))
	}

	// Test with missing descendant
	descendants2 := []*flow.Selector{
		{Text: "Child1"},
		{Text: "Missing"},
	}

	result2 := FilterContainsDescendants(containers, allElements, descendants2, "android")
	if len(result2) != 0 {
		t.Errorf("Expected 0 containers (missing descendant), got %d", len(result2))
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestGetClickableElement(t *testing.T) {
	// Build a hierarchy: GrandParent (clickable) -> Parent (not clickable) -> Child (not clickable)
	grandParent := &ParsedElement{
		Text:      "GrandParent",
		Clickable: true,
		Bounds:    core.Bounds{X: 0, Y: 0, Width: 500, Height: 500},
	}
	parent := &ParsedElement{
		Text:      "Parent",
		Clickable: false,
		Bounds:    core.Bounds{X: 50, Y: 50, Width: 400, Height: 400},
		Parent:    grandParent,
	}
	child := &ParsedElement{
		Text:      "Child",
		Clickable: false,
		Bounds:    core.Bounds{X: 100, Y: 100, Width: 200, Height: 100},
		Parent:    parent,
	}

	// Test: child is not clickable, should return grandparent
	result := GetClickableElement(child)
	if result != grandParent {
		t.Errorf("Expected grandParent (clickable), got %s", result.Text)
	}

	// Test: parent is not clickable, should return grandparent
	result = GetClickableElement(parent)
	if result != grandParent {
		t.Errorf("Expected grandParent (clickable), got %s", result.Text)
	}

	// Test: grandparent is clickable, should return itself
	result = GetClickableElement(grandParent)
	if result != grandParent {
		t.Errorf("Expected grandParent (itself), got %s", result.Text)
	}

	// Test: element with no clickable parent returns itself
	orphan := &ParsedElement{
		Text:      "Orphan",
		Clickable: false,
		Bounds:    core.Bounds{X: 0, Y: 0, Width: 100, Height: 50},
		Parent:    nil,
	}
	result = GetClickableElement(orphan)
	if result != orphan {
		t.Errorf("Expected orphan (no clickable parent), got %s", result.Text)
	}

	// Test: clickable element returns itself even with clickable parent
	clickableChild := &ParsedElement{
		Text:      "ClickableChild",
		Clickable: true,
		Bounds:    core.Bounds{X: 100, Y: 100, Width: 200, Height: 100},
		Parent:    grandParent,
	}
	result = GetClickableElement(clickableChild)
	if result != clickableChild {
		t.Errorf("Expected clickableChild (itself), got %s", result.Text)
	}

	// Test: nil element returns nil
	result = GetClickableElement(nil)
	if result != nil {
		t.Errorf("Expected nil for nil input, got %v", result)
	}
}
