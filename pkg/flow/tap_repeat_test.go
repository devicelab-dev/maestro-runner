package flow

import "testing"

// Only the taps of a repeat: carry a place in it; the first does not skip
// the settle before it and the last does not skip the wait after it.
func TestRepeatTapPlace(t *testing.T) {
	tap := func(i, n int) Step { return &TapOnStep{RepeatIndex: i, RepeatCount: n} }
	cases := []struct {
		step                   Step
		afterFirst, beforeLast bool
	}{
		{&TapOnStep{}, false, false},       // a single tap
		{tap(0, 1), false, false},          // repeat: 1
		{tap(0, 3), false, true},           // first of three
		{tap(1, 3), true, true},            // middle
		{tap(2, 3), true, false},           // last
		{&DoubleTapOnStep{}, false, false}, // not a tapOn
		{&TapOnPointStep{}, false, false},
	}
	for _, c := range cases {
		if got := RepeatTapAfterFirst(c.step); got != c.afterFirst {
			t.Errorf("RepeatTapAfterFirst(%+v) = %v, want %v", c.step, got, c.afterFirst)
		}
		if got := RepeatTapBeforeLast(c.step); got != c.beforeLast {
			t.Errorf("RepeatTapBeforeLast(%+v) = %v, want %v", c.step, got, c.beforeLast)
		}
	}
}
