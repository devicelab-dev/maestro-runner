package core

import "testing"

func TestCenteringCheck(t *testing.T) {
	const w, h = 1000, 2000
	edge := Bounds{X: 0, Y: 1850, Width: 1000, Height: 150}
	centred := Bounds{X: 0, Y: 950, Width: 1000, Height: 100}

	var off Centering
	if decided, _ := off.Check(false, edge, "down", w, h); decided {
		t.Error("without centerElement the usual visibility check must decide")
	}

	var c Centering
	if decided, done := c.Check(true, centred, "down", w, h); !decided || !done {
		t.Errorf("centred element: decided=%v done=%v, want stop", decided, done)
	}
	for i := 0; i <= maxCenterScrolls; i++ {
		if decided, done := c.Check(true, edge, "down", w, h); !decided || done {
			t.Fatalf("scroll %d at the edge: decided=%v done=%v, want scroll again", i, decided, done)
		}
	}
	if decided, _ := c.Check(true, edge, "down", w, h); decided {
		t.Error("after the extra scrolls the element is accepted on visibility")
	}

	var barely Centering
	sliver := Bounds{X: 0, Y: 1995, Width: 1000, Height: 100} // 5% on screen
	if decided, _ := barely.Check(true, sliver, "down", w, h); decided {
		t.Error("a barely visible element is left to the usual check")
	}
}
