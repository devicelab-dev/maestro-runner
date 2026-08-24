package uiautomator2

import "testing"

// A scroll confined to a container must stay inside it. A gesture that starts
// or ends outside the container is delivered to whatever is there instead —
// usually the parent list, which then scrolls the wrong thing.
func TestScrollPointsStayInsideTheRect(t *testing.T) {
	const x, y, w, h = 100, 600, 800, 400

	for _, direction := range []string{"up", "down", "left", "right"} {
		t.Run(direction, func(t *testing.T) {
			fromX, fromY, toX, toY := scrollPointsInRect(direction, x, y, w, h, 0.5)
			for _, p := range []struct {
				name string
				px   int
				py   int
			}{{"from", fromX, fromY}, {"to", toX, toY}} {
				if p.px < x || p.px > x+w {
					t.Errorf("%s x=%d outside [%d,%d]", p.name, p.px, x, x+w)
				}
				if p.py < y || p.py > y+h {
					t.Errorf("%s y=%d outside [%d,%d]", p.name, p.py, y, y+h)
				}
			}
		})
	}
}

func TestScrollPointsDirection(t *testing.T) {
	const x, y, w, h = 0, 0, 1000, 1000

	// "down" reveals content below, which means dragging the finger upwards.
	_, fromY, _, toY := scrollPointsInRect("down", x, y, w, h, 0.5)
	if fromY <= toY {
		t.Errorf("down should drag upward: from y=%d to y=%d", fromY, toY)
	}

	_, fromY, _, toY = scrollPointsInRect("up", x, y, w, h, 0.5)
	if fromY >= toY {
		t.Errorf("up should drag downward: from y=%d to y=%d", fromY, toY)
	}

	fromX, _, toX, _ := scrollPointsInRect("right", x, y, w, h, 0.5)
	if fromX <= toX {
		t.Errorf("right should drag leftward: from x=%d to x=%d", fromX, toX)
	}

	// An unrecognised direction behaves as "down" rather than producing a
	// zero-length swipe that silently does nothing.
	downFrom, _, downTo, _ := scrollPointsInRect("down", x, y, w, h, 0.5)
	oddFrom, _, oddTo, _ := scrollPointsInRect("sideways", x, y, w, h, 0.5)
	if oddFrom != downFrom || oddTo != downTo {
		t.Error("an unknown direction should fall back to down")
	}
}

func TestScrollPointsAreCentredInTheContainer(t *testing.T) {
	// The gesture belongs in the middle of the container, not the middle of
	// the screen — that is the whole point of scrolling `from:` something.
	fromX, _, toX, _ := scrollPointsInRect("down", 100, 600, 800, 400, 0.5)
	if fromX != 500 || toX != 500 {
		t.Errorf("expected the swipe on the container's centre line x=500, got from=%d to=%d", fromX, toX)
	}
}
