package core

import "testing"

func TestIOSElementTypes(t *testing.T) {
	if got := IOSElementType("XCUIElementTypeButton"); got != "Button" {
		t.Errorf("IOSElementType = %q", got)
	}
	for typ, want := range map[string]bool{"XCUIElementTypeKey": true, "Keyboard": true, "Button": false, "": false} {
		if got := IsIOSKeyboardType(typ); got != want {
			t.Errorf("IsIOSKeyboardType(%q) = %v", typ, got)
		}
	}
	for typ, want := range map[string]bool{"XCUIElementTypeSecureTextField": true, "TextView": true, "SearchField": true, "StaticText": false} {
		if got := IsIOSTextInputType(typ); got != want {
			t.Errorf("IsIOSTextInputType(%q) = %v", typ, got)
		}
	}
}

func TestKeyboardDragStart(t *testing.T) {
	const w, h, kbTop = 400, 800, 500

	t.Run("empty screen starts at the centre", func(t *testing.T) {
		x, y, ok := KeyboardDragStart(nil, w, h, kbTop)
		if !ok || x != 200 || y != 400 {
			t.Errorf("got (%d,%d) ok=%v, want (200,400)", x, y, ok)
		}
	})

	t.Run("screen types are not controls", func(t *testing.T) {
		nodes := []ScreenNode{
			{Type: "XCUIElementTypeApplication", Bounds: Bounds{0, 0, w, h}},
			{Type: "XCUIElementTypeWindow", Bounds: Bounds{0, 0, w, h}, Leaf: true},
		}
		x, y, ok := KeyboardDragStart(nodes, w, h, kbTop)
		if !ok || x != 200 || y != 400 {
			t.Errorf("got (%d,%d) ok=%v, want (200,400)", x, y, ok)
		}
	})

	t.Run("a button at the centre moves the start off it", func(t *testing.T) {
		btn := Bounds{X: 150, Y: 380, Width: 100, Height: 40}
		x, y, ok := KeyboardDragStart([]ScreenNode{{Type: "XCUIElementTypeButton", Bounds: btn, Leaf: true}}, w, h, kbTop)
		if !ok {
			t.Fatal("no start")
		}
		cleared := Bounds{X: btn.X - 8, Y: btn.Y - 8, Width: btn.Width + 16, Height: btn.Height + 16}
		if cleared.Contains(x, y) {
			t.Errorf("start (%d,%d) is within 8pt of the button", x, y)
		}
		if y >= kbTop {
			t.Errorf("start (%d,%d) is on the keyboard", x, y)
		}
	})

	t.Run("an unnamed leaf counts as a control, text does not", func(t *testing.T) {
		leaf := Bounds{X: 100, Y: 300, Width: 200, Height: 200}
		x, y, _ := KeyboardDragStart([]ScreenNode{{Type: "Other", Bounds: leaf, Leaf: true}}, w, h, kbTop)
		if leaf.Contains(x, y) {
			t.Errorf("start (%d,%d) is on the pressable leaf", x, y)
		}
		x, y, _ = KeyboardDragStart([]ScreenNode{{Type: "StaticText", Bounds: leaf, Leaf: true}}, w, h, kbTop)
		if x != 200 || y != 400 {
			t.Errorf("text moved the start to (%d,%d)", x, y)
		}
	})

	t.Run("a leaf over half the screen is content, not a control", func(t *testing.T) {
		big := Bounds{X: 0, Y: 0, Width: w, Height: h}
		x, y, ok := KeyboardDragStart([]ScreenNode{{Type: "Other", Bounds: big, Leaf: true}}, w, h, kbTop)
		if !ok || x != 200 || y != 400 {
			t.Errorf("got (%d,%d) ok=%v, want the centre", x, y, ok)
		}
	})

	t.Run("start stays inside the scroll view", func(t *testing.T) {
		scroll := Bounds{X: 0, Y: 100, Width: w, Height: 200}
		x, y, ok := KeyboardDragStart([]ScreenNode{{Type: "XCUIElementTypeScrollView", Bounds: scroll}}, w, h, kbTop)
		if !ok || !scroll.Contains(x, y) {
			t.Errorf("start (%d,%d) ok=%v is outside the scroll view", x, y, ok)
		}
	})

	t.Run("start stays above the keyboard", func(t *testing.T) {
		x, y, ok := KeyboardDragStart(nil, w, h, 300)
		if !ok || y > 300-8 {
			t.Errorf("start (%d,%d) ok=%v is not clear of the keyboard", x, y, ok)
		}
	})

	t.Run("controls everywhere and no scroll view: no start", func(t *testing.T) {
		_, _, ok := KeyboardDragStart([]ScreenNode{{Type: "Button", Bounds: Bounds{0, 0, w, h}}}, w, h, kbTop)
		if ok {
			t.Error("expected no start")
		}
	})

	t.Run("controls everywhere inside a scroll view: the centre", func(t *testing.T) {
		nodes := []ScreenNode{
			{Type: "ScrollView", Bounds: Bounds{0, 0, w, h}},
			{Type: "Button", Bounds: Bounds{0, 0, w, h}, Leaf: true},
		}
		x, y, ok := KeyboardDragStart(nodes, w, h, kbTop)
		if !ok || x != 200 || y != 400 {
			t.Errorf("got (%d,%d) ok=%v, want the centre", x, y, ok)
		}
	})

	t.Run("keyboard above the usable band: no start", func(t *testing.T) {
		if _, _, ok := KeyboardDragStart(nil, w, h, 50); ok {
			t.Error("expected no start")
		}
		if _, _, ok := KeyboardDragStart(nil, 0, 0, 0); ok {
			t.Error("expected no start without a screen")
		}
	})
}
