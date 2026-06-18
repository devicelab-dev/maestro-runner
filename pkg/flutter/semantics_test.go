// SPDX-FileCopyrightText: 2024–2026 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package flutter

import (
	"strings"
	"testing"
)

const testDump = `SemanticsNode#0
 │ Rect.fromLTRB(0.0, 0.0, 411.4, 890.3)
 │ scaled by 2.6x
 │ flags: scopesRoute
 │ actions: tap
 │
 ├─SemanticsNode#1
 │ │ Rect.fromLTRB(0.0, 0.0, 411.4, 890.3)
 │ │ flags: scopesRoute
 │ │
 │ ├─SemanticsNode#2
 │ │   Rect.fromLTRB(16.0, 100.0, 395.4, 140.0)
 │ │   label: "Welcome to TestHive"
 │ │   identifier: "welcome_title"
 │ │
 │ ├─SemanticsNode#3
 │ │   Rect.fromLTRB(16.0, 160.0, 395.4, 200.0)
 │ │   label: "Enter your email"
 │ │   hint: "Email address"
 │ │   identifier: "email_field"
 │ │   actions: tap, longPress
 │ │
 │ └─SemanticsNode#4
 │     Rect.fromLTRB(16.0, 220.0, 395.4, 268.0)
 │     label: "Login"
 │     identifier: "login_button"
 │     flags: isButton, isFocusable
 │     actions: tap
 │
 └─SemanticsNode#5
     Rect.fromLTRB(0.0, 800.0, 411.4, 890.3)
     label: "Footer"
     value: "v1.0.0"
`

func TestParseSemanticsTree(t *testing.T) {
	root, pixelRatio, err := ParseSemanticsTree(testDump)
	if err != nil {
		t.Fatalf("ParseSemanticsTree: %v", err)
	}

	if pixelRatio != 2.6 {
		t.Errorf("pixelRatio = %v, want 2.6", pixelRatio)
	}

	if root.ID != 0 {
		t.Errorf("root.ID = %d, want 0", root.ID)
	}

	if len(root.Children) != 2 {
		t.Fatalf("root has %d children, want 2", len(root.Children))
	}

	node1 := root.Children[0]
	if node1.ID != 1 {
		t.Errorf("node1.ID = %d, want 1", node1.ID)
	}
	if len(node1.Children) != 3 {
		t.Fatalf("node1 has %d children, want 3", len(node1.Children))
	}

	// Check node #2 (welcome title)
	node2 := node1.Children[0]
	if node2.ID != 2 {
		t.Errorf("node2.ID = %d, want 2", node2.ID)
	}
	if node2.Label != "Welcome to TestHive" {
		t.Errorf("node2.Label = %q, want %q", node2.Label, "Welcome to TestHive")
	}
	if node2.Identifier != "welcome_title" {
		t.Errorf("node2.Identifier = %q, want %q", node2.Identifier, "welcome_title")
	}

	// Check node #3 (email field)
	node3 := node1.Children[1]
	if node3.ID != 3 {
		t.Errorf("node3.ID = %d, want 3", node3.ID)
	}
	if node3.Label != "Enter your email" {
		t.Errorf("node3.Label = %q", node3.Label)
	}
	if node3.Hint != "Email address" {
		t.Errorf("node3.Hint = %q", node3.Hint)
	}
	if node3.Identifier != "email_field" {
		t.Errorf("node3.Identifier = %q", node3.Identifier)
	}
	if len(node3.Actions) != 2 {
		t.Errorf("node3.Actions = %v, want [tap longPress]", node3.Actions)
	}

	// Check node #4 (login button)
	node4 := node1.Children[2]
	if node4.ID != 4 {
		t.Errorf("node4.ID = %d, want 4", node4.ID)
	}
	if node4.Label != "Login" {
		t.Errorf("node4.Label = %q", node4.Label)
	}
	if len(node4.Flags) != 2 {
		t.Errorf("node4.Flags = %v, want [isButton isFocusable]", node4.Flags)
	}

	// Check node #5 (footer)
	node5 := root.Children[1]
	if node5.ID != 5 {
		t.Errorf("node5.ID = %d, want 5", node5.ID)
	}
	if node5.Value != "v1.0.0" {
		t.Errorf("node5.Value = %q, want %q", node5.Value, "v1.0.0")
	}
}

func TestParseSemanticsTree_Empty(t *testing.T) {
	_, _, err := ParseSemanticsTree("")
	if err == nil {
		t.Error("expected error for empty dump")
	}
}

func TestParseSemanticsTree_NoNodes(t *testing.T) {
	_, _, err := ParseSemanticsTree("some text without nodes\nanother line\n")
	if err == nil {
		t.Error("expected error when no nodes found")
	}
}

func TestParseSemanticsTree_NoPixelRatio(t *testing.T) {
	dump := `SemanticsNode#0
 Rect.fromLTRB(0.0, 0.0, 400.0, 800.0)
 label: "Root"
`
	root, pixelRatio, err := ParseSemanticsTree(dump)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pixelRatio != 1.0 {
		t.Errorf("pixelRatio = %v, want 1.0 (default)", pixelRatio)
	}
	if root.Label != "Root" {
		t.Errorf("root.Label = %q, want %q", root.Label, "Root")
	}
}

func TestFindByIdentifier(t *testing.T) {
	root, _, _ := ParseSemanticsTree(testDump)

	results := FindByIdentifier(root, "email_field")
	if len(results) != 1 {
		t.Fatalf("FindByIdentifier returned %d results, want 1", len(results))
	}
	if results[0].ID != 3 {
		t.Errorf("found node ID = %d, want 3", results[0].ID)
	}

	// Non-existent identifier
	results = FindByIdentifier(root, "nonexistent")
	if len(results) != 0 {
		t.Errorf("FindByIdentifier returned %d results for nonexistent, want 0", len(results))
	}
}

func TestFindByLabel(t *testing.T) {
	root, _, _ := ParseSemanticsTree(testDump)

	results := FindByLabel(root, "Login")
	if len(results) != 1 {
		t.Fatalf("FindByLabel returned %d results, want 1", len(results))
	}
	if results[0].ID != 4 {
		t.Errorf("found node ID = %d, want 4", results[0].ID)
	}

	// Partial match
	results = FindByLabel(root, "Welcome")
	if len(results) != 1 {
		t.Fatalf("FindByLabel partial match returned %d results, want 1", len(results))
	}
	if results[0].ID != 2 {
		t.Errorf("found node ID = %d, want 2", results[0].ID)
	}
}

func TestFindByHint(t *testing.T) {
	root, _, _ := ParseSemanticsTree(testDump)

	results := FindByHint(root, "Email address")
	if len(results) != 1 {
		t.Fatalf("FindByHint returned %d results, want 1", len(results))
	}
	if results[0].ID != 3 {
		t.Errorf("found node ID = %d, want 3", results[0].ID)
	}

	// No match
	results = FindByHint(root, "nonexistent")
	if len(results) != 0 {
		t.Errorf("FindByHint returned %d results for nonexistent, want 0", len(results))
	}
}

func TestRectToBounds(t *testing.T) {
	r := Rect{Left: 16.0, Top: 100.0, Right: 395.4, Bottom: 140.0}
	b := r.ToBounds(2.6)

	// 16.0 * 2.6 = 41.6 → 41
	if b.X != 41 {
		t.Errorf("X = %d, want 41", b.X)
	}
	// 100.0 * 2.6 = 260
	if b.Y != 260 {
		t.Errorf("Y = %d, want 260", b.Y)
	}
	// (395.4 - 16.0) * 2.6 = 986.44 → 986
	if b.Width != 986 {
		t.Errorf("Width = %d, want 986", b.Width)
	}
	// (140.0 - 100.0) * 2.6 = 104
	if b.Height != 104 {
		t.Errorf("Height = %d, want 104", b.Height)
	}
}

func TestParseSemanticsTree_MultilineLabel(t *testing.T) {
	dump := `SemanticsNode#0
 │ Rect.fromLTRB(0.0, 0.0, 1080.0, 2274.0)
 │ scaled by 2.8x
 │
 └─SemanticsNode#1
   │ Rect.fromLTRB(16.0, 185.5, 376.7, 397.5)
   │ identifier: "card_wrapper"
   │
   └─SemanticsNode#2
     │ Rect.fromLTRB(0.0, 0.0, 360.7, 212.0)
     │ identifier: "card_title"
     │ label:
     │   "Card Title
     │   Card Subtitle
     │   This is a longer description inside the card."
     │ textDirection: ltr
     │
     └─SemanticsNode#3
         Rect.fromLTRB(176.1, 144.0, 265.3, 192.0)
         label: "Action"
`
	root, _, err := ParseSemanticsTree(dump)
	if err != nil {
		t.Fatalf("ParseSemanticsTree: %v", err)
	}

	// Node #2 should have the full multiline label
	node2 := root.Children[0].Children[0]
	if node2.ID != 2 {
		t.Fatalf("expected node #2, got #%d", node2.ID)
	}
	if node2.Identifier != "card_title" {
		t.Errorf("identifier = %q, want %q", node2.Identifier, "card_title")
	}

	// The label should contain all three lines
	if !strings.Contains(node2.Label, "Card Title") {
		t.Errorf("label missing 'Card Title': %q", node2.Label)
	}
	if !strings.Contains(node2.Label, "Card Subtitle") {
		t.Errorf("label missing 'Card Subtitle': %q", node2.Label)
	}
	if !strings.Contains(node2.Label, "longer description") {
		t.Errorf("label missing 'longer description': %q", node2.Label)
	}

	// Node #3 should still parse correctly after the multiline label
	node3 := node2.Children[0]
	if node3.ID != 3 {
		t.Fatalf("expected node #3, got #%d", node3.ID)
	}
	if node3.Label != "Action" {
		t.Errorf("node3.Label = %q, want %q", node3.Label, "Action")
	}
}

func TestParseSemanticsTree_AccuratePixelRatio(t *testing.T) {
	// Real Flutter dump: Node #0 has physical pixels, Node #1 has logical pixels.
	// The "scaled by 2.8x" is rounded; actual ratio is 1080/392.7 = 2.75.
	dump := `SemanticsNode#0
 │ Rect.fromLTRB(0.0, 0.0, 1080.0, 2274.0)
 │
 └─SemanticsNode#1
   │ Rect.fromLTRB(0.0, 0.0, 392.7, 826.9) scaled by 2.8x
   │ textDirection: ltr
   │
   └─SemanticsNode#2
       Rect.fromLTRB(24.0, 193.5, 368.7, 256.0)
       label: "Password"
`
	_, pixelRatio, err := ParseSemanticsTree(dump)
	if err != nil {
		t.Fatalf("ParseSemanticsTree: %v", err)
	}

	// Should compute accurate ratio from Node #0 / Node #1 dimensions
	expected := 1080.0 / 392.7
	if pixelRatio < expected-0.01 || pixelRatio > expected+0.01 {
		t.Errorf("pixelRatio = %v, want ~%v (not rounded 2.8)", pixelRatio, expected)
	}
}

func TestParseSemanticsTree_PixelRatioFallback(t *testing.T) {
	// When Node #0 and Node #1 have the same rect (test data),
	// should fall back to the "scaled by" annotation.
	_, pixelRatio, err := ParseSemanticsTree(testDump)
	if err != nil {
		t.Fatalf("ParseSemanticsTree: %v", err)
	}
	if pixelRatio != 2.6 {
		t.Errorf("pixelRatio = %v, want 2.6 (from annotation)", pixelRatio)
	}
}

func TestHasFlag(t *testing.T) {
	node := &SemanticsNode{
		Flags: []string{"isTextField", "hasEnabledState", "isEnabled"},
	}
	if !HasFlag(node, "isTextField") {
		t.Error("expected HasFlag('isTextField') = true")
	}
	if HasFlag(node, "isButton") {
		t.Error("expected HasFlag('isButton') = false")
	}
}

func TestRectToBounds_PixelRatio1(t *testing.T) {
	r := Rect{Left: 10.0, Top: 20.0, Right: 110.0, Bottom: 70.0}
	b := r.ToBounds(1.0)

	if b.X != 10 || b.Y != 20 || b.Width != 100 || b.Height != 50 {
		t.Errorf("bounds = %+v, want {X:10 Y:20 Width:100 Height:50}", b)
	}
}
