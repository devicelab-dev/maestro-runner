package devicelab_ios_legacy

import (
	"math"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// keyboardRefindMs bounds the lookup of a tap's target after the keyboard
// that covered it was hidden.
const keyboardRefindMs = 2000

// keyboardFrame returns the software keyboard's frame from the screen's
// snapshot (the one the lookup just read, while it is fresh), nil when the
// keyboard is not on screen.
func (d *Driver) keyboardFrame() (*core.Bounds, error) {
	nodes, err := d.fetchSnapshot()
	if err != nil {
		return nil, err
	}
	for i := range nodes {
		if nodes[i].Type == "Keyboard" && nodes[i].Rect.Width > 0 && nodes[i].Rect.Height > 0 {
			r := nodes[i].Rect
			return &core.Bounds{
				X: int(math.Round(r.X)), Y: int(math.Round(r.Y)),
				Width: int(math.Round(r.Width)), Height: int(math.Round(r.Height)),
			}, nil
		}
	}
	return nil, nil
}

// aimClearOfKeyboard returns where to tap node, planned at (x, y), so the
// touch does not press a key: when the software keyboard covers that point
// it hides the keyboard and looks sel up again, then aims at the part of the
// element above the keyboard if it is still covered, else fails
// (core.KeyboardGuard). It applies only after a step that may have left the
// keyboard up. The node returned is the one last found.
func (d *Driver) aimClearOfKeyboard(sel flow.Selector, node *SnapshotNode, x, y float64) (float64, float64, *SnapshotNode, error) {
	if !d.keyboardMayBeUp || core.IsIOSKeyboardType(node.Type) {
		return x, y, node, nil
	}
	target := *node
	guard := core.KeyboardGuard{
		Keyboard: func() (*core.Bounds, error) {
			kb, err := d.keyboardFrame()
			if err == nil && kb == nil {
				d.keyboardMayBeUp = false
			}
			return kb, err
		},
		OnKeyboard: func() bool { return d.onKeyboard(target) },
		Hide: func() {
			d.handleHideKeyboard(&flow.HideKeyboardStep{})
			d.invalidateSnapshotCache()
		},
		Refind: func() (core.Bounds, float64, float64, bool) {
			fresh, err := d.findElement(sel, true, keyboardRefindMs)
			if err != nil || fresh == nil {
				return core.Bounds{}, 0, 0, false
			}
			node = fresh
			fx, fy := centerOf(fresh)
			return snapshotBounds(fresh), fx, fy, true
		},
	}
	tx, ty, _, err := guard.Aim(snapshotBounds(&target), x, y)
	return tx, ty, node, err
}

// onKeyboard reports whether n belongs to the keyboard (a key, its return
// button): a node of the same type and rect in the snapshot that sits under
// the Keyboard.
func (d *Driver) onKeyboard(n SnapshotNode) bool {
	nodes, err := d.fetchSnapshot()
	if err != nil {
		return false
	}
	for i := range nodes {
		if nodes[i].Type != n.Type || nodes[i].Rect != n.Rect {
			continue
		}
		for p, seen := &nodes[i], 0; p != nil && seen <= len(nodes); seen++ {
			if p.Type == "Keyboard" {
				return true
			}
			if p.ParentIndex == nil || *p.ParentIndex < 0 || *p.ParentIndex >= len(nodes) {
				break
			}
			p = &nodes[*p.ParentIndex]
		}
	}
	return false
}
