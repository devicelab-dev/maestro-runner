package core

import (
	"math"
	"strings"
)

// iOS software keyboard geometry, shared by the iOS drivers (WDA, DeviceLab,
// DeviceLab legacy): where a tap may land when the keyboard is up, and where
// the drag that dismisses it may start.

// IOSElementType returns an XCUIElementType name without its prefix:
// "XCUIElementTypeButton" and "Button" both give "Button".
func IOSElementType(t string) string {
	return strings.TrimPrefix(t, "XCUIElementType")
}

// IsIOSKeyboardType reports whether an element of type t is the keyboard or
// one of its keys.
func IsIOSKeyboardType(t string) bool {
	switch IOSElementType(t) {
	case "Keyboard", "Key":
		return true
	}
	return false
}

// IsIOSTextInputType reports whether tapping an element of type t raises the
// keyboard.
func IsIOSTextInputType(t string) bool {
	switch IOSElementType(t) {
	case "TextField", "SecureTextField", "SearchField", "TextView":
		return true
	}
	return false
}

// ScreenNode is one element of a screen's hierarchy, as KeyboardDragStart
// reads it.
type ScreenNode struct {
	// Type is the XCUIElementType name, with or without its prefix.
	Type   string
	Bounds Bounds
	// Leaf is true when the node has no children.
	Leaf bool
}

// controlClearance is how far, in points, the keyboard-dismissing drag starts
// from any control: a control's touch area runs past its frame, and the
// drag's first points are still a tap on it.
const controlClearance = 8

var (
	// dragControlTypes are types a touch presses, toggles or focuses, and
	// the bars, where a drag never reaches the content.
	dragControlTypes = map[string]bool{
		"Button": true, "Link": true, "TextField": true, "SecureTextField": true, "SearchField": true,
		"TextView": true, "Switch": true, "Toggle": true, "CheckBox": true, "Slider": true,
		"SegmentedControl": true, "PickerWheel": true, "MenuItem": true, "Key": true, "Keyboard": true,
		"NavigationBar": true, "TabBar": true, "Toolbar": true,
	}
	// dragScrollTypes scroll, so a short drag on them reads as the touch
	// that sends the keyboard away.
	dragScrollTypes = map[string]bool{"ScrollView": true, "Table": true, "CollectionView": true, "WebView": true}
	// dragScreenTypes span the screen and hold everything else.
	dragScreenTypes = map[string]bool{"Application": true, "Window": true}
	// dragContentTypes are leaf types a touch does nothing to.
	dragContentTypes = map[string]bool{"StaticText": true, "Image": true}
)

// KeyboardDragStart is where the keyboard-dismissing drag starts (Maestro's
// iOS hideKeyboard: two short drags from the middle of the screen): the point
// nearest the centre of the screen, above the keyboard, that no control
// covers, inside a scroll view when the screen has one (the scroll view is
// what turns the drag into a dismissal). Maestro starts at the exact centre,
// where a 50 ms drag still presses the control under it.
//
// A control is a node of a control type, or a leaf that is not plain text or
// an image (a React Native Pressable with no role is an unnamed leaf) and
// covers at most half the screen (a flattened tree shows its content as one
// big leaf). With no such point it returns the centre when a scroll view
// holds it (a scroll view cancels the touches of the controls in it once it
// scrolls), else ok is false. nodes are the on-screen nodes.
func KeyboardDragStart(nodes []ScreenNode, screenW, screenH, keyboardTop int) (x, y int, ok bool) {
	var controls, scrollers []Bounds
	merged := screenW * screenH / 2
	for _, n := range nodes {
		t := IOSElementType(n.Type)
		if dragScreenTypes[t] {
			continue
		}
		b := n.Bounds
		switch {
		case dragScrollTypes[t]:
			scrollers = append(scrollers, b)
		case dragControlTypes[t] || (n.Leaf && !dragContentTypes[t] && b.Width*b.Height <= merged):
			controls = append(controls, Bounds{
				X: b.X - controlClearance, Y: b.Y - controlClearance,
				Width: b.Width + 2*controlClearance, Height: b.Height + 2*controlClearance,
			})
		}
	}
	cx, cy := float64(screenW)/2, float64(screenH)/2
	// The status bar, the home indicator and the system edge gestures live
	// in the outer tenth of each edge.
	left, right, top := float64(screenW)*0.1, float64(screenW)*0.9, float64(screenH)*0.1
	bottom := math.Min(float64(screenH)*0.9, float64(keyboardTop-controlClearance))
	step := math.Min(float64(screenW), float64(screenH)) / 40
	if step <= 0 || bottom <= top {
		return 0, 0, false
	}
	inside := func(rs []Bounds, px, py float64) bool {
		for _, r := range rs {
			if px >= float64(r.X) && px < float64(r.X+r.Width) && py >= float64(r.Y) && py < float64(r.Y+r.Height) {
				return true
			}
		}
		return false
	}
	best, found := math.Inf(1), false
	var bx, by float64
	// Anchored on the centre, so an empty screen starts where it always did.
	for py := cy - math.Floor((cy-top)/step)*step; py <= bottom; py += step {
		for px := cx - math.Floor((cx-left)/step)*step; px <= right; px += step {
			if (len(scrollers) == 0 || inside(scrollers, px, py)) && !inside(controls, px, py) {
				if d := math.Hypot(px-cx, py-cy); d < best {
					best, bx, by, found = d, px, py, true
				}
			}
		}
	}
	if found {
		return int(math.Round(bx)), int(math.Round(by)), true
	}
	if cy < float64(keyboardTop) && inside(scrollers, cx, cy) {
		return int(cx), int(cy), true
	}
	return 0, 0, false
}
