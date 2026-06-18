// SPDX-FileCopyrightText: 2024–2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package flutter

import (
	"testing"
)

const testWidgetDump = `[root](renderObject: RenderView#12345)
└WidgetsApp(state: _WidgetsAppState#abcde)
 └MaterialApp(state: _MaterialAppState#fghij)
  └Scaffold(state: ScaffoldState#klmno)
   └Column(direction: vertical)
    ├TextField(controller: TextEditingController#111, decoration: InputDecoration(labelText: "Email", hintText: "Enter your email"))
    │└EditableText(...)
    ├TextField(controller: TextEditingController#222, decoration: InputDecoration(labelText: "Username", hintText: "Enter your username"))
    │└EditableText(...)
    ├Semantics(identifier: "card_subtitle", container: false)
    │└Text("Card Subtitle", textAlign: start)
    ├Semantics(identifier: "card_description", container: false)
    │└Text("This is a longer description inside the card. Its resource-id may not be visible to Maestro.", textAlign: start)
    ├TextField(controller: TextEditingController#333, decoration: InputDecoration(labelText: "Password", hintText: "Enter password", suffix: Semantics(identifier: "toggle_password_visibility", label: "Toggle password visibility", child: GestureDetector(child: Icon(Icons.visibility_off)))))
    │└EditableText(...)
    └TextField(controller: TextEditingController#444, decoration: InputDecoration(labelText: "Search", suffixIcon: Semantics(identifier: "clear_search_btn", label: "Clear search", child: IconButton(icon: Icon(Icons.clear)))))
     └EditableText(...)
`

func TestSearchWidgetTreeForText_HintText(t *testing.T) {
	match := SearchWidgetTreeForText(testWidgetDump, "Enter your email")
	if match == nil {
		t.Fatal("expected match for hintText")
	}
	if match.MatchType != "hintText" {
		t.Errorf("MatchType = %q, want %q", match.MatchType, "hintText")
	}
	if match.LabelText != "Email" {
		t.Errorf("LabelText = %q, want %q", match.LabelText, "Email")
	}
	if !match.IsTextField {
		t.Error("expected IsTextField = true")
	}
}

func TestSearchWidgetTreeForText_HintTextPartial(t *testing.T) {
	match := SearchWidgetTreeForText(testWidgetDump, "Enter your username")
	if match == nil {
		t.Fatal("expected match")
	}
	if match.LabelText != "Username" {
		t.Errorf("LabelText = %q, want %q", match.LabelText, "Username")
	}
}

func TestSearchWidgetTreeForText_NotFound(t *testing.T) {
	match := SearchWidgetTreeForText(testWidgetDump, "nonexistent text")
	if match != nil {
		t.Errorf("expected nil, got %+v", match)
	}
}

func TestSearchWidgetTreeForText_EmptyInputs(t *testing.T) {
	if SearchWidgetTreeForText("", "test") != nil {
		t.Error("expected nil for empty dump")
	}
	if SearchWidgetTreeForText(testWidgetDump, "") != nil {
		t.Error("expected nil for empty search text")
	}
}

func TestSearchWidgetTreeForID_WithNearbyText(t *testing.T) {
	match := SearchWidgetTreeForID(testWidgetDump, "card_subtitle")
	if match == nil {
		t.Fatal("expected match for identifier")
	}
	if match.MatchType != "identifier" {
		t.Errorf("MatchType = %q, want %q", match.MatchType, "identifier")
	}
	if match.NearbyText != "Card Subtitle" {
		t.Errorf("NearbyText = %q, want %q", match.NearbyText, "Card Subtitle")
	}
}

func TestSearchWidgetTreeForID_SuffixDetected(t *testing.T) {
	match := SearchWidgetTreeForID(testWidgetDump, "toggle_password_visibility")
	if match == nil {
		t.Fatal("expected match for identifier")
	}
	if !match.IsSuffix {
		t.Error("expected IsSuffix = true for suffix: Semantics identifier")
	}
	// Should find labelText "Password" from backward context
	if match.LabelText != "Password" {
		t.Errorf("LabelText = %q, want %q", match.LabelText, "Password")
	}
}

func TestSearchWidgetTreeForID_SuffixIconDetected(t *testing.T) {
	match := SearchWidgetTreeForID(testWidgetDump, "clear_search_btn")
	if match == nil {
		t.Fatal("expected match for identifier")
	}
	if !match.IsSuffix {
		t.Error("expected IsSuffix = true for suffixIcon: Semantics identifier")
	}
}

func TestSearchWidgetTreeForID_NonSuffix(t *testing.T) {
	match := SearchWidgetTreeForID(testWidgetDump, "card_subtitle")
	if match == nil {
		t.Fatal("expected match")
	}
	if match.IsSuffix {
		t.Error("expected IsSuffix = false for non-suffix identifier")
	}
}

func TestSearchWidgetTreeForID_NotFound(t *testing.T) {
	match := SearchWidgetTreeForID(testWidgetDump, "nonexistent_id")
	if match != nil {
		t.Errorf("expected nil, got %+v", match)
	}
}

func TestSearchWidgetTreeForID_EmptyInputs(t *testing.T) {
	if SearchWidgetTreeForID("", "test") != nil {
		t.Error("expected nil for empty dump")
	}
	if SearchWidgetTreeForID(testWidgetDump, "") != nil {
		t.Error("expected nil for empty search id")
	}
}

func TestCrossReference_HintText(t *testing.T) {
	// Build a semantics tree with TextFields
	root := &SemanticsNode{
		ID: 0,
		Children: []*SemanticsNode{
			{
				ID:    1,
				Label: "Email",
				Flags: []string{"isTextField", "hasEnabledState", "isEnabled"},
				Rect:  Rect{Left: 24.0, Top: 213.5, Right: 368.7, Bottom: 269.5},
			},
			{
				ID:    2,
				Label: "Username",
				Flags: []string{"isTextField", "hasEnabledState", "isEnabled"},
				Rect:  Rect{Left: 24.0, Top: 285.5, Right: 368.7, Bottom: 333.5},
			},
		},
	}

	match := &WidgetTreeMatch{
		MatchType:   "hintText",
		LabelText:   "Email",
		IsTextField: true,
	}

	nodes := match.CrossReferenceWithSemantics(root)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].ID != 1 {
		t.Errorf("node ID = %d, want 1", nodes[0].ID)
	}
}

func TestCrossReference_Identifier(t *testing.T) {
	// Build a semantics tree where "Card Subtitle" text is in a merged label
	root := &SemanticsNode{
		ID: 0,
		Children: []*SemanticsNode{
			{
				ID:         1,
				Identifier: "card_title",
				Label:      "Card Title\nCard Subtitle\nA longer description.",
				Rect:       Rect{Left: 0.0, Top: 0.0, Right: 360.7, Bottom: 212.0},
			},
		},
	}

	match := &WidgetTreeMatch{
		MatchType:  "identifier",
		NearbyText: "Card Subtitle",
	}

	nodes := match.CrossReferenceWithSemantics(root)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].ID != 1 {
		t.Errorf("node ID = %d, want 1", nodes[0].ID)
	}
}

func TestCrossReference_IdentifierTextField(t *testing.T) {
	// Suffix icon identifier → cross-reference via TextField labelText
	root := &SemanticsNode{
		ID: 0,
		Children: []*SemanticsNode{
			{
				ID:    1,
				Label: "Password",
				Flags: []string{"isTextField", "hasEnabledState", "isEnabled", "isObscured"},
				Rect:  Rect{Left: 24.0, Top: 193.5, Right: 368.7, Bottom: 256.0},
			},
		},
	}

	match := &WidgetTreeMatch{
		MatchType:   "identifier",
		LabelText:   "Password",
		IsTextField: true,
	}

	nodes := match.CrossReferenceWithSemantics(root)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].ID != 1 {
		t.Errorf("node ID = %d, want 1", nodes[0].ID)
	}
}

func TestCrossReference_IdentifierWithNewlineInLabel(t *testing.T) {
	// Simulates the card_description case: nearby text is one line but
	// the semantics label has newlines from dump wrapping.
	root := &SemanticsNode{
		ID: 0,
		Children: []*SemanticsNode{
			{
				ID:         1,
				Identifier: "card_title",
				// Label has newlines inserted by semantics dump wrapping
				Label: "Card Title\nCard Subtitle\nThis is a longer description inside the card. Its resource-id\nmay not be visible to Maestro.",
				Rect:  Rect{Left: 0.0, Top: 0.0, Right: 360.7, Bottom: 212.0},
			},
		},
	}

	match := &WidgetTreeMatch{
		MatchType: "identifier",
		// Widget tree has the full text without wrapping newlines
		NearbyText: "This is a longer description inside the card. Its resource-id may not be visible to Maestro.",
	}

	nodes := match.CrossReferenceWithSemantics(root)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].ID != 1 {
		t.Errorf("node ID = %d, want 1", nodes[0].ID)
	}
}

func TestContainsNormalized(t *testing.T) {
	tests := []struct {
		haystack string
		needle   string
		want     bool
	}{
		{"hello world", "hello world", true},
		{"hello\nworld", "hello world", true},
		{"This is a\nlonger text", "a longer text", true},
		{"no match here", "something else", false},
		{"multi\n  space\n  text", "multi space text", true},
	}
	for _, tt := range tests {
		got := containsNormalized(tt.haystack, tt.needle)
		if got != tt.want {
			t.Errorf("containsNormalized(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
		}
	}
}

func TestCrossReference_NilInputs(t *testing.T) {
	root := &SemanticsNode{ID: 0}
	var nilMatch *WidgetTreeMatch

	if nodes := nilMatch.CrossReferenceWithSemantics(root); len(nodes) != 0 {
		t.Errorf("expected empty for nil match, got %d", len(nodes))
	}

	match := &WidgetTreeMatch{MatchType: "hintText", LabelText: "Test"}
	if nodes := match.CrossReferenceWithSemantics(nil); len(nodes) != 0 {
		t.Errorf("expected empty for nil root, got %d", len(nodes))
	}
}
