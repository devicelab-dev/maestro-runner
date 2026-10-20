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
// Nothing inside a web view starts a space. Its content reports screen coordinates already,
// under the same shape: Google's sign-in sheet (ASWebAuthenticationSession) is a WebView at
// (0,48) whose same-size child sits at (0,0), with every link in it where the screen draws it.
// Rebased, a tap on the chooser's account landed 48 points lower, on "Use another account". The
// web view can be any ancestor, not only the parent: SFSafariViewController wraps its content
// in more views.
//
// With this switch set, every element lookup reads the page source, the regex way: WDA's own
// queries (a plain text or id selector) return WDA's rect, which is not corrected. That costs a
// page source per lookup, and a lookup the page source cannot answer has no WDA fallback; it
// keeps reading the page source until its timeout.
//
// --driver devicelab corrects these sheets itself and needs no switch.
//
// Two limits. A table cell that is off screen reports its children's frames relative to the
// cell, which no rule here can tell from a frame on the screen, so a flow scrolls the cell into
// view before it reads them. And the shape can occur by chance: a scroll view not at the origin, scrolled by exactly its
// own origin and with content of exactly its size, looks like a hosted space and is moved. So
// the switch is off by default.
func hostedFrames() bool {
	v := strings.TrimSpace(os.Getenv("MAESTRO_WDA_HOSTED_FRAMES"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// rebaseHostedFrames adds (offsetX, offsetY) to elem's frame, the offset its coordinate space
// needs, and walks its children, starting a new offset wherever a child begins a hosted space.
// inWebView is true under a web view, where no space starts.
func rebaseHostedFrames(elem *ParsedElement, offsetX, offsetY int, inWebView bool) {
	elem.Bounds.X += offsetX
	elem.Bounds.Y += offsetY
	inWebView = inWebView || elem.Type == "XCUIElementTypeWebView"
	for _, child := range elem.Children {
		childX, childY := offsetX, offsetY
		if !inWebView && startsHostedSpace(elem, child) {
			childX, childY = elem.Bounds.X, elem.Bounds.Y
		}
		rebaseHostedFrames(child, childX, childY, inWebView)
	}
}

// startsHostedSpace reports whether child restarts the coordinates: it is at (0,0) with its
// parent's size (see hostedFrames). Zero-size elements never do.
func startsHostedSpace(parent, child *ParsedElement) bool {
	c := child.Bounds
	return c.X == 0 && c.Y == 0 && c.Width > 0 && c.Height > 0 &&
		c.Width == parent.Bounds.Width && c.Height == parent.Bounds.Height
}
