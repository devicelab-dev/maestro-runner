package core

import (
	"errors"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestTapAboveKeyboard(t *testing.T) {
	kb := Bounds{X: 0, Y: 500, Width: 400, Height: 300}
	// 40pt of the element above the keyboard: aim at its middle.
	if x, y, ok := TapAboveKeyboard(Bounds{X: 100, Y: 460, Width: 200, Height: 100}, kb, 200); !ok || x != 200 || y != 480 {
		t.Errorf("got (%.0f,%.0f) ok=%v, want (200,480)", x, y, ok)
	}
	// Clipped to the top of the screen.
	if _, y, ok := TapAboveKeyboard(Bounds{X: 0, Y: -100, Width: 400, Height: 700}, kb, 200); !ok || y != 250 {
		t.Errorf("got y=%.0f ok=%v, want 250", y, ok)
	}
	// A sliver under 8pt is not a target.
	if _, _, ok := TapAboveKeyboard(Bounds{X: 0, Y: 495, Width: 400, Height: 50}, kb, 200); ok {
		t.Error("a 5pt sliver should not be aimed at")
	}
	// Wholly under the keyboard.
	if _, _, ok := TapAboveKeyboard(Bounds{X: 0, Y: 600, Width: 400, Height: 50}, kb, 200); ok {
		t.Error("an element under the keyboard has nothing to aim at")
	}
}

// fakeKeyboard drives KeyboardGuard: the keyboard frames returned in turn
// (the last repeats), whether hiding was asked, and what a refind gives.
type fakeKeyboard struct {
	frames   []*Bounds
	err      error
	reads    int
	hidden   bool
	onKB     bool
	refound  Bounds
	refoundX float64
	refoundY float64
	gone     bool
}

func (f *fakeKeyboard) guard() KeyboardGuard {
	return KeyboardGuard{
		Keyboard: func() (*Bounds, error) {
			i := min(f.reads, len(f.frames)-1)
			f.reads++
			return f.frames[i], f.err
		},
		OnKeyboard: func() bool { return f.onKB },
		Hide:       func() { f.hidden = true },
		Refind: func() (Bounds, float64, float64, bool) {
			return f.refound, f.refoundX, f.refoundY, !f.gone
		},
	}
}

func TestKeyboardGuardAim(t *testing.T) {
	kb := &Bounds{X: 0, Y: 500, Width: 400, Height: 300}
	button := Bounds{X: 100, Y: 600, Width: 200, Height: 50} // centre (200,625), under kb

	t.Run("no keyboard: tap where planned", func(t *testing.T) {
		f := &fakeKeyboard{frames: []*Bounds{nil}}
		x, y, moved, err := f.guard().Aim(button, 200, 625)
		if err != nil || moved || x != 200 || y != 625 || f.hidden {
			t.Errorf("got (%.0f,%.0f) moved=%v err=%v hidden=%v", x, y, moved, err, f.hidden)
		}
	})

	t.Run("keyboard query fails: tap where planned", func(t *testing.T) {
		f := &fakeKeyboard{frames: []*Bounds{kb}, err: errors.New("boom")}
		x, y, _, err := f.guard().Aim(button, 200, 625)
		if err != nil || x != 200 || y != 625 || f.hidden {
			t.Errorf("got (%.0f,%.0f) err=%v hidden=%v", x, y, err, f.hidden)
		}
	})

	t.Run("point above the keyboard: untouched", func(t *testing.T) {
		f := &fakeKeyboard{frames: []*Bounds{kb}}
		x, y, moved, err := f.guard().Aim(Bounds{X: 100, Y: 100, Width: 200, Height: 50}, 200, 125)
		if err != nil || moved || x != 200 || y != 125 || f.hidden {
			t.Errorf("got (%.0f,%.0f) moved=%v err=%v hidden=%v", x, y, moved, err, f.hidden)
		}
	})

	t.Run("a key of the keyboard is tapped where it is", func(t *testing.T) {
		f := &fakeKeyboard{frames: []*Bounds{kb}, onKB: true}
		x, y, moved, err := f.guard().Aim(Bounds{X: 300, Y: 750, Width: 90, Height: 40}, 345, 770)
		if err != nil || moved || x != 345 || y != 770 || f.hidden {
			t.Errorf("got (%.0f,%.0f) moved=%v err=%v hidden=%v", x, y, moved, err, f.hidden)
		}
	})

	t.Run("covered: hide, refind, tap the element where it is now", func(t *testing.T) {
		f := &fakeKeyboard{frames: []*Bounds{kb, nil}, refound: button, refoundX: 200, refoundY: 625}
		x, y, moved, err := f.guard().Aim(button, 200, 625)
		if err != nil || !moved || !f.hidden || x != 200 || y != 625 {
			t.Errorf("got (%.0f,%.0f) moved=%v err=%v hidden=%v", x, y, moved, err, f.hidden)
		}
	})

	t.Run("keyboard stays but the element moved above it", func(t *testing.T) {
		f := &fakeKeyboard{frames: []*Bounds{kb}, refound: Bounds{X: 100, Y: 400, Width: 200, Height: 50}, refoundX: 200, refoundY: 425}
		x, y, _, err := f.guard().Aim(button, 200, 625)
		if err != nil || x != 200 || y != 425 {
			t.Errorf("got (%.0f,%.0f) err=%v", x, y, err)
		}
	})

	t.Run("keyboard stays: aim at the part above it", func(t *testing.T) {
		tall := Bounds{X: 100, Y: 450, Width: 200, Height: 100} // centre 500, on the keyboard top
		f := &fakeKeyboard{frames: []*Bounds{kb}, refound: tall, refoundX: 200, refoundY: 500}
		x, y, _, err := f.guard().Aim(tall, 200, 500)
		if err != nil || x != 200 || y != 475 {
			t.Errorf("got (%.0f,%.0f) err=%v, want (200,475)", x, y, err)
		}
	})

	t.Run("keyboard stays and covers all of it: fail", func(t *testing.T) {
		f := &fakeKeyboard{frames: []*Bounds{kb}, refound: button, refoundX: 200, refoundY: 625}
		_, _, _, err := f.guard().Aim(button, 200, 625)
		if !errors.Is(err, ErrUnderKeyboard) {
			t.Errorf("err = %v, want ErrUnderKeyboard", err)
		}
	})

	t.Run("gone once the keyboard is hidden: fail", func(t *testing.T) {
		f := &fakeKeyboard{frames: []*Bounds{kb, nil}, gone: true}
		_, _, moved, err := f.guard().Aim(button, 200, 625)
		if !errors.Is(err, ErrUnderKeyboard) || !moved {
			t.Errorf("err = %v moved=%v, want ErrUnderKeyboard", err, moved)
		}
	})

	t.Run("no OnKeyboard means not a key", func(t *testing.T) {
		f := &fakeKeyboard{frames: []*Bounds{kb, nil}, refound: button, refoundX: 200, refoundY: 625}
		g := f.guard()
		g.OnKeyboard = nil
		if _, _, _, err := g.Aim(button, 200, 625); err != nil || !f.hidden {
			t.Errorf("err=%v hidden=%v", err, f.hidden)
		}
	})
}

func TestMayLeaveIOSKeyboardUp(t *testing.T) {
	field := &CommandResult{Element: &ElementInfo{Class: "XCUIElementTypeSecureTextField"}}
	button := &CommandResult{Element: &ElementInfo{Class: "Button"}}
	cases := []struct {
		step   flow.Step
		result *CommandResult
		want   bool
	}{
		{&flow.InputTextStep{}, nil, true},
		{&flow.InputRandomStep{}, nil, true},
		{&flow.EraseTextStep{}, nil, true},
		{&flow.PasteTextStep{}, nil, true},
		{&flow.PressKeyStep{}, nil, true},
		{&flow.TapOnStep{}, field, true},
		{&flow.LongPressOnStep{}, field, true},
		{&flow.TapOnStep{}, button, false},
		{&flow.TapOnStep{}, &CommandResult{}, false},
		{&flow.TapOnStep{}, nil, false},
		{&flow.HideKeyboardStep{}, nil, false},
		{&flow.SwipeStep{}, nil, false},
	}
	for _, c := range cases {
		if got := MayLeaveIOSKeyboardUp(c.step, c.result); got != c.want {
			t.Errorf("%T: got %v, want %v", c.step, got, c.want)
		}
	}
}
