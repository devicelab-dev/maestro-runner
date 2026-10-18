package wda

import (
	"os"
	"strings"
)

// hostedFrames reports whether page-source frames inside a view hosted by another process are
// moved onto the screen's coordinates (MAESTRO_WDA_HOSTED_FRAMES set to anything but "", "0"
// or "false").
//
// On iOS 27 XCUITest reports the elements of a remotely hosted view in that view's own
// coordinate space. Apple Health's authorization sheet sits 48 points down on an iPhone 11 and
// every element inside it reports a frame 48 points too high, so a tap at an element's centre
// lands on whatever is drawn 48 points above it: "Select All 12 Topics" was tapped on the
// sheet's subtitle, and nothing was selected. A coordinate tap, a tap from a WDA rect and
// XCUITest's own element click all missed the same way, because all three use that frame.
//
// The page source shows where such a space starts: a child at (0,0) with exactly its parent's
// size. An element in the screen's coordinates that fills its parent reports the parent's
// origin instead. With this switch set, ParsePageSource adds the parent's origin to that child
// and to everything under it; under a parent at the origin that adds nothing.
//
// A web view is left alone. Its content reports screen coordinates already, under the same
// shape: Google's sign-in sheet (ASWebAuthenticationSession) is a WebView at (0,48) whose
// same-size child sits at (0,0), with every link in it where the screen draws it. Rebased, a tap
// on the chooser's account landed 48 points lower, on "Use another account".
//
// Three limits. Only lookups through the page source see the correction: a selector WDA answers
// with a query of its own (plain text, or an id that means itself) still gets WDA's rect, so a
// flow reaches a hosted view with a regex text or an id with its dots escaped. And a table cell
// that is off screen reports its children's frames relative to the cell, which no rule here can
// tell from a frame on the screen, so a flow scrolls the cell into view before it reads them.
// And the shape can occur by chance: a scroll view not at the origin, scrolled by exactly its
// own origin and with content of exactly its size, looks like a hosted space and is moved. So
// the switch is off by default.
func hostedFrames() bool {
	v := strings.TrimSpace(os.Getenv("MAESTRO_WDA_HOSTED_FRAMES"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// rebaseHostedFrames adds (offsetX, offsetY) to elem's frame, the offset its coordinate space
// needs, and walks its children, starting a new offset wherever a child begins a hosted space.
func rebaseHostedFrames(elem *ParsedElement, offsetX, offsetY int) {
	elem.Bounds.X += offsetX
	elem.Bounds.Y += offsetY
	for _, child := range elem.Children {
		childX, childY := offsetX, offsetY
		if startsHostedSpace(elem, child) {
			childX, childY = elem.Bounds.X, elem.Bounds.Y
		}
		rebaseHostedFrames(child, childX, childY)
	}
}

// startsHostedSpace reports whether child restarts the coordinates: it is at (0,0) with its
// parent's size, and the parent is not a web view (see hostedFrames). Zero-size elements never
// do.
func startsHostedSpace(parent, child *ParsedElement) bool {
	if parent.Type == "XCUIElementTypeWebView" {
		return false
	}
	c := child.Bounds
	return c.X == 0 && c.Y == 0 && c.Width > 0 && c.Height > 0 &&
		c.Width == parent.Bounds.Width && c.Height == parent.Bounds.Height
}
