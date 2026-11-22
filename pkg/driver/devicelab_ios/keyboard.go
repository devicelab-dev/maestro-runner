package devicelab_ios

import (
	"math"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// keyboardRefindMs bounds the lookup of a tap's target after the keyboard
// that covered it was hidden.
const keyboardRefindMs = 2000

// keyboardFrame returns the software keyboard's frame, nil when none is up.
func (d *Driver) keyboardFrame() (*core.Bounds, error) {
	resp, err := d.call("keyboard", &Args{Action: "state"})
	if err != nil {
		return nil, err
	}
	p := resp.payload()
	if !p.Visible || p.FrameW <= 0 || p.FrameH <= 0 {
		return nil, nil
	}
	return &core.Bounds{
		X: int(math.Round(p.FrameX)), Y: int(math.Round(p.FrameY)),
		Width: int(math.Round(p.FrameW)), Height: int(math.Round(p.FrameH)),
	}, nil
}

// aimClearOfKeyboard returns where to tap node, planned at (x, y), so the
// touch does not press a key: when the software keyboard covers that point
// it hides the keyboard and looks sel up again, then aims at the part of the
// element above the keyboard if it is still covered, else fails
// (core.KeyboardGuard). It asks for the keyboard only after a step that may
// have left it up. The node returned is the one last found.
//
// The agent knows the keyboard's frame at every lookup; were `find` to return
// it, every tap could be guarded with no extra call, a keyboard the app raised
// by itself included.
func (d *Driver) aimClearOfKeyboard(sel flow.Selector, node *Node, x, y float64) (float64, float64, *Node, error) {
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
		Hide:       func() { d.hideKeyboard() },
		Refind: func() (core.Bounds, float64, float64, bool) {
			fresh, fsc, err := d.findElement(sel, true, keyboardRefindMs)
			if err != nil || fresh == nil {
				return core.Bounds{}, 0, 0, false
			}
			fx, fy, ok := tapPoint(*fresh, fsc.width, fsc.height)
			if ok {
				node = fresh
			}
			return bounds(*fresh), fx, fy, ok
		},
	}
	tx, ty, _, err := guard.Aim(bounds(target), x, y)
	return tx, ty, node, err
}

// onKeyboard reports whether n belongs to the keyboard (a key, its return
// button): a node of the same type and frame in a snapshot that sits under
// the Keyboard. False when the snapshot cannot be read.
func (d *Driver) onKeyboard(n Node) bool {
	resp, err := d.call("snapshot", &Args{VisibleOnly: true})
	if err != nil {
		return false
	}
	nodes := resp.payload().Nodes
	byIndex := make(map[int]Node, len(nodes))
	for _, m := range nodes {
		byIndex[m.I] = m
	}
	for _, m := range nodes {
		if m.Type != n.Type || !sameFrame(m, n) {
			continue
		}
		for p, seen := m, 0; seen <= len(nodes); seen++ {
			if p.Type == "Keyboard" {
				return true
			}
			parent, ok := byIndex[p.P]
			if !ok || p.P == p.I {
				break
			}
			p = parent
		}
	}
	return false
}
