package core

import (
	"testing"
	"time"
)

// coversByTop is a test verdict: covered when the element center is at/below the keyboard top.
func coversByTop(el Bounds, kb *Bounds) bool {
	if kb == nil {
		return false
	}
	_, cy := el.Center()
	return cy >= kb.Y
}

func TestSettleKeyboardBlocking(t *testing.T) {
	fastWindow, fastPoll := 200*time.Millisecond, 10*time.Millisecond

	t.Run("element not present -> not blocked", func(t *testing.T) {
		blocked, _, _ := SettleKeyboardBlocking(
			func() (Bounds, bool) { return Bounds{}, false },
			func() *Bounds { return &Bounds{Y: 100} },
			coversByTop, fastWindow, fastPoll)
		if blocked {
			t.Error("absent element should not be blocked")
		}
	})

	t.Run("clear on first sample -> not blocked, no wait", func(t *testing.T) {
		// element center (y=50) is above keyboard top (y=100) => clear
		blocked, _, _ := SettleKeyboardBlocking(
			func() (Bounds, bool) { return Bounds{Y: 0, Height: 100}, true },
			func() *Bounds { return &Bounds{Y: 200} },
			coversByTop, fastWindow, fastPoll)
		if blocked {
			t.Error("element above keyboard should not be blocked")
		}
	})

	t.Run("relayout lifts element mid-settle -> not blocked", func(t *testing.T) {
		n := 0
		blocked, _, _ := SettleKeyboardBlocking(
			func() (Bounds, bool) {
				n++
				if n <= 2 {
					return Bounds{Y: 1600, Height: 60}, true // covered on first frames
				}
				return Bounds{Y: 1300, Height: 60}, true // lifted above keyboard
			},
			func() *Bounds { return &Bounds{Y: 1500} },
			coversByTop, fastWindow, fastPoll)
		if blocked {
			t.Error("element that clears after relayout should not be blocked")
		}
	})

	t.Run("persistent overlap -> blocked with last geometry", func(t *testing.T) {
		blocked, kbTop, centerY := SettleKeyboardBlocking(
			func() (Bounds, bool) { return Bounds{Y: 1600, Height: 60}, true },
			func() *Bounds { return &Bounds{Y: 1500} },
			coversByTop, fastWindow, fastPoll)
		if !blocked {
			t.Fatal("persistently covered element should be blocked")
		}
		if kbTop != 1500 || centerY != 1630 {
			t.Errorf("geometry = (kbTop %d, centerY %d), want (1500, 1630)", kbTop, centerY)
		}
	})

	t.Run("keyboard dismissed mid-settle -> not blocked", func(t *testing.T) {
		n := 0
		blocked, _, _ := SettleKeyboardBlocking(
			func() (Bounds, bool) { return Bounds{Y: 1600, Height: 60}, true },
			func() *Bounds {
				n++
				if n <= 2 {
					return &Bounds{Y: 1500}
				}
				return nil // keyboard gone
			},
			coversByTop, fastWindow, fastPoll)
		if blocked {
			t.Error("dismissed keyboard should unblock the element")
		}
	})
}
