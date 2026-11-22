package core

import (
	"errors"
	"fmt"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// ErrUnderKeyboard means a tap's target sits under the on-screen keyboard and
// no part of it could be reached: the tap would press a key instead.
var ErrUnderKeyboard = errors.New("element is under the keyboard")

// minClearStrip is the least height, in points, of the part of an element
// above the keyboard that a tap is aimed at. A thinner sliver puts the touch
// on the keyboard's own edge.
const minClearStrip = 8

// keyboardCovers reports whether the point (x, y) lies on the keyboard.
func keyboardCovers(kb Bounds, x, y float64) bool {
	return kb.Width > 0 && kb.Height > 0 &&
		x >= float64(kb.X) && x < float64(kb.X+kb.Width) &&
		y >= float64(kb.Y) && y < float64(kb.Y+kb.Height)
}

// TapAboveKeyboard aims at the part of b above the keyboard's top edge,
// keeping x: the centre of that strip, clipped to the top of the screen. ok
// is false when less than minClearStrip of b shows above the keyboard.
func TapAboveKeyboard(b, kb Bounds, x float64) (float64, float64, bool) {
	top := max(b.Y, 0)
	bottom := min(b.Y+b.Height, kb.Y)
	if bottom-top < minClearStrip {
		return 0, 0, false
	}
	return x, float64(top+bottom) / 2, true
}

// KeyboardGuard keeps an element tap off the iOS software keyboard. XCUITest
// reports an element the keyboard covers as on screen, and a touch at its
// centre presses a key instead. Maestro taps there too.
//
// Each driver supplies its own ways to read the keyboard, hide it and look
// the element up again; Aim holds the order they are used in.
type KeyboardGuard struct {
	// Keyboard returns the keyboard's frame, nil when none is on screen.
	Keyboard func() (*Bounds, error)
	// OnKeyboard reports whether the target is part of the keyboard itself
	// (a key, its return button), which must be tapped where it is. It is
	// asked only when the tap point lies on the keyboard. Nil means no.
	OnKeyboard func() bool
	// Hide dismisses the keyboard, as the driver's hideKeyboard does.
	Hide func()
	// Refind looks the element up again once the keyboard is hidden and
	// returns its bounds and the point the driver would tap; ok is false
	// when it is no longer on screen.
	Refind func() (b Bounds, x, y float64, ok bool)
}

// Aim returns the point to tap for an element with bounds b that the driver
// would tap at (x, y). When the keyboard covers that point it hides the
// keyboard and looks the element up again; when the keyboard still covers
// it, it aims at the part of the element above the keyboard; when there is
// none, it fails with ErrUnderKeyboard. moved is true when the element was
// looked up again, so the caller reports the refound element. A keyboard
// that cannot be read is no reason to block a tap: the point stands.
func (g KeyboardGuard) Aim(b Bounds, x, y float64) (tx, ty float64, moved bool, err error) {
	kb, kerr := g.Keyboard()
	if kerr != nil || kb == nil || !keyboardCovers(*kb, x, y) {
		return x, y, false, nil
	}
	if g.OnKeyboard != nil && g.OnKeyboard() {
		return x, y, false, nil
	}
	firstTop := kb.Y
	g.Hide()
	nb, nx, ny, ok := g.Refind()
	if !ok {
		return 0, 0, true, fmt.Errorf("%w: its tap point (%.0f,%.0f) was on the keyboard (top %d), and after hiding the keyboard it is not on screen",
			ErrUnderKeyboard, x, y, firstTop)
	}
	kb, kerr = g.Keyboard()
	if kerr != nil || kb == nil || !keyboardCovers(*kb, nx, ny) {
		return nx, ny, true, nil
	}
	if ax, ay, ok := TapAboveKeyboard(nb, *kb, nx); ok {
		return ax, ay, true, nil
	}
	return 0, 0, true, fmt.Errorf("%w: its tap point (%.0f,%.0f) is on the keyboard (top %d), hideKeyboard did not dismiss it, and no part of the element shows above it",
		ErrUnderKeyboard, nx, ny, kb.Y)
}

// MayLeaveIOSKeyboardUp reports whether a step that ran may have left the
// software keyboard up: one that types, and a tap on a text input. The iOS
// drivers look for the keyboard before a tap only after such a step, so a
// tap costs no extra round trip until something has been typed. A keyboard
// an app raises on its own (a field focused on screen entry) is not seen
// until the flow types.
func MayLeaveIOSKeyboardUp(step flow.Step, result *CommandResult) bool {
	switch step.(type) {
	case *flow.InputTextStep, *flow.InputRandomStep, *flow.EraseTextStep, *flow.PasteTextStep, *flow.PressKeyStep:
		return true
	case *flow.TapOnStep, *flow.DoubleTapOnStep, *flow.LongPressOnStep:
		return result != nil && result.Element != nil && IsIOSTextInputType(result.Element.Class)
	}
	return false
}
