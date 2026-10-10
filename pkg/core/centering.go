package core

// maxCenterScrolls is how many extra scrolls centerElement may take to bring
// a visible element to the middle, as in Maestro; after them the element is
// accepted on visibility alone (the end of a list cannot be scrolled further).
const maxCenterScrolls = 4

// Centering applies scrollUntilVisible's centerElement, as Maestro does:
// once the element is more than 10% visible, keep scrolling until its centre
// is within a fifth of the screen past the middle, in the direction of the
// scroll, for up to maxCenterScrolls more scrolls. Without centerElement the
// step stopped as soon as the element showed at the edge, so content the flow
// expected beside it (the next list item) was never on screen.
type Centering struct {
	tries int
}

// Check reports for one look at the element whether centering decides the
// step: decided is false when the step does not ask for it, or the element
// is barely visible, or the extra scrolls are used up, and the usual
// visibility check applies. When decided, done says stop (true) or scroll
// again (false).
func (c *Centering) Check(centerElement bool, b Bounds, direction string, screenW, screenH int) (decided, done bool) {
	if !centerElement || c.tries > maxCenterScrolls || VisibleFraction(b, screenW, screenH) <= 0.1 {
		return false, false
	}
	if NearScreenCenter(b, screenW, screenH, direction) {
		return true, true
	}
	c.tries++
	return true, false
}
