package devicelab_ios

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// keyboardAgent is a 400x800 screen whose keyboard (top 500) hides on the
// hideAfter-th "keyboard hide" (never with 0); button is the screen's node.
type keyboardAgent struct {
	mu        sync.Mutex
	up        bool
	hideAfter int
	hides     int
	button    Node
	extra     []Node
}

func (k *keyboardAgent) handle(cmd string, a Args) (*Response, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	switch cmd {
	case "find", "snapshot":
		nodes := append([]Node{k.button}, k.extra...)
		return tree(nodes...), nil
	case "keyboard":
		if a.Action == "hide" {
			k.hides++
			if k.hideAfter > 0 && k.hides >= k.hideAfter {
				k.up = false
			}
		}
		if !k.up {
			return ok(&Payload{}), nil
		}
		return ok(&Payload{Visible: true, FrameX: 0, FrameY: 500, FrameW: 400, FrameH: 300}), nil
	}
	return ok(&Payload{}), nil
}

func tapLabel(d *Driver, label string) *core.CommandResult {
	return d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: label}})
}

func TestTapNoKeyboardQueryBeforeTyping(t *testing.T) {
	k := &keyboardAgent{up: true, button: node(1, "Button", "Submit", 20, 700, 360, 50)}
	d, fa, _ := newTestDriver(t, k.handle)
	if r := tapLabel(d, "Submit"); !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	if n := len(fa.sent("keyboard")); n != 0 {
		t.Errorf("keyboard asked %d times, want 0", n)
	}
}

func TestTapUnderKeyboardHidesItFirst(t *testing.T) {
	k := &keyboardAgent{up: true, hideAfter: 1, button: node(1, "Button", "Submit", 20, 700, 360, 50)}
	d, fa, _ := newTestDriver(t, k.handle)
	d.keyboardMayBeUp = true
	if r := tapLabel(d, "Submit"); !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	if k.hides != 1 {
		t.Errorf("hides = %d, want 1", k.hides)
	}
	acts := fa.sent("act")
	if len(acts) != 1 || *acts[0].X != 200 || *acts[0].Y != 725 {
		t.Fatalf("acts = %+v, want one tap at the button's centre", acts)
	}
	if d.keyboardMayBeUp {
		t.Error("keyboardMayBeUp still set after the keyboard went")
	}
}

func TestTapUnderKeyboardAimsAboveIt(t *testing.T) {
	k := &keyboardAgent{up: true, button: node(1, "Button", "Submit", 20, 460, 360, 100)}
	d, fa, _ := newTestDriver(t, k.handle)
	d.keyboardMayBeUp = true
	if r := tapLabel(d, "Submit"); !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	acts := fa.sent("act")
	if len(acts) != 1 || *acts[0].Y != 480 {
		t.Fatalf("acts = %+v, want one tap at y=480", acts)
	}
}

func TestTapUnderKeyboardFailsWhenCovered(t *testing.T) {
	k := &keyboardAgent{up: true, button: node(1, "Button", "Submit", 20, 700, 360, 50)}
	d, fa, _ := newTestDriver(t, k.handle)
	d.keyboardMayBeUp = true
	r := tapLabel(d, "Submit")
	if r.Success || !errors.Is(r.Error, core.ErrUnderKeyboard) || !strings.Contains(r.Message, "under the keyboard") {
		t.Fatalf("result %v %v %q, want ErrUnderKeyboard", r.Success, r.Error, r.Message)
	}
	if len(fa.sent("act")) != 0 {
		t.Error("tapped despite the keyboard")
	}
}

// The keyboard's own Done button is tapped where it is.
func TestTapKeyboardButtonIsNotHidden(t *testing.T) {
	kb := node(1, "Keyboard", "", 0, 500, 400, 300)
	done := node(2, "Button", "Done", 300, 750, 96, 42)
	done.P = 1
	k := &keyboardAgent{up: true, button: done, extra: []Node{kb}}
	d, fa, _ := newTestDriver(t, k.handle)
	d.keyboardMayBeUp = true
	if r := tapLabel(d, "Done"); !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	if k.hides != 0 || len(fa.sent("act")) != 1 {
		t.Errorf("hides %d acts %d, want the button tapped in place", k.hides, len(fa.sent("act")))
	}
}

// Typing sets the check; a tap that finds no keyboard clears it.
func TestKeyboardCheckFollowsTyping(t *testing.T) {
	k := &keyboardAgent{button: node(1, "Button", "Submit", 20, 700, 360, 50)}
	d, fa, _ := newTestDriver(t, k.handle)
	d.Execute(&flow.InputTextStep{Text: "x"})
	if !d.keyboardMayBeUp {
		t.Fatal("inputText did not set keyboardMayBeUp")
	}
	tapLabel(d, "Submit")
	tapLabel(d, "Submit")
	if n := len(fa.sent("keyboard")); n != 1 || d.keyboardMayBeUp {
		t.Errorf("keyboard asked %d times, keyboardMayBeUp %v; want one check, then none", n, d.keyboardMayBeUp)
	}
}
