package devicelab_ios_legacy

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// keyboardRunner is a fake runner whose screen holds nodes and, while up, a
// keyboard (top 500). keyboardDismiss hides it unless stuck, which answers
// as the runner does when it cannot.
type keyboardRunner struct {
	mu       sync.Mutex
	up       bool
	stuck    bool
	nodes    []SnapshotNode
	commands []string
	taps     [][2]float64
}

func (k *keyboardRunner) server(t *testing.T) *Driver {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cmd Command
		_ = json.NewDecoder(r.Body).Decode(&cmd)
		k.mu.Lock()
		defer k.mu.Unlock()
		k.commands = append(k.commands, string(cmd.Command))
		reply := map[string]any{"ok": true, "data": map[string]any{}}
		switch cmd.Command {
		case CmdSnapshot:
			nodes := append([]SnapshotNode{}, k.nodes...)
			if k.up {
				nodes = append(nodes, SnapshotNode{Index: len(nodes), Type: "Keyboard", Rect: SnapshotRect{X: 0, Y: 500, Width: 400, Height: 300}})
			}
			reply["data"] = map[string]any{"nodes": nodes}
		case CmdKeyboardDismiss:
			if k.stuck && k.up {
				reply = map[string]any{"ok": false, "error": map[string]any{"code": ErrUnsupportedOperation, "message": "cannot"}}
			} else {
				k.up = false
			}
		case CmdTap:
			k.taps = append(k.taps, [2]float64{*cmd.X, *cmd.Y})
		case CmdTapBySelector:
			reply["data"] = map[string]any{"x": 1, "y": 1}
		case CmdQuerySelector:
			reply = map[string]any{"ok": false, "error": map[string]any{"code": "NOT_FOUND", "message": "no match"}}
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	return NewDriver(&Client{baseURL: srv.URL, httpClient: srv.Client()}, nil, "test-udid", nil)
}

func (k *keyboardRunner) sent(c CommandType) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	n := 0
	for _, s := range k.commands {
		if s == string(c) {
			n++
		}
	}
	return n
}

func submitAt(y, h float64) SnapshotNode {
	return SnapshotNode{Index: 0, Type: "Button", Label: "Submit", Rect: SnapshotRect{X: 20, Y: y, Width: 360, Height: h}}
}

func tapSubmitStep() *flow.TapOnStep {
	return &flow.TapOnStep{Selector: flow.Selector{Text: "Submit"}}
}

// Before anything is typed, a simple tap takes the runner's one-shot path.
func TestTapUsesFastPathBeforeTyping(t *testing.T) {
	k := &keyboardRunner{up: true, nodes: []SnapshotNode{submitAt(700, 50)}}
	d := k.server(t)
	if r := d.Execute(tapSubmitStep()); !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	if k.sent(CmdTapBySelector) != 1 || k.sent(CmdSnapshot) != 0 {
		t.Errorf("commands = %v, want one tapBySelector", k.commands)
	}
}

// After typing, a tap on a button under the keyboard hides it first.
func TestTapUnderKeyboardHidesItFirst(t *testing.T) {
	k := &keyboardRunner{up: true, nodes: []SnapshotNode{submitAt(700, 50)}}
	d := k.server(t)
	d.keyboardMayBeUp = true
	if r := d.Execute(tapSubmitStep()); !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	if k.sent(CmdTapBySelector) != 0 || k.sent(CmdKeyboardDismiss) != 1 {
		t.Errorf("commands = %v, want a dismiss and no one-shot tap", k.commands)
	}
	if len(k.taps) != 1 || k.taps[0] != [2]float64{200, 725} {
		t.Errorf("taps = %v, want the button's centre", k.taps)
	}
	if d.keyboardMayBeUp {
		t.Error("keyboardMayBeUp still set after the keyboard went")
	}
}

// A keyboard that stays: the tap goes above it, or the step fails.
func TestTapUnderStuckKeyboard(t *testing.T) {
	k := &keyboardRunner{up: true, stuck: true, nodes: []SnapshotNode{submitAt(460, 100)}}
	d := k.server(t)
	d.keyboardMayBeUp = true
	if r := d.Execute(tapSubmitStep()); !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	if len(k.taps) != 1 || k.taps[0] != [2]float64{200, 480} {
		t.Errorf("taps = %v, want (200,480)", k.taps)
	}

	k = &keyboardRunner{up: true, stuck: true, nodes: []SnapshotNode{submitAt(700, 50)}}
	d = k.server(t)
	d.keyboardMayBeUp = true
	r := d.Execute(tapSubmitStep())
	if r.Success || !errors.Is(r.Error, core.ErrUnderKeyboard) || !strings.Contains(r.Message, "under the keyboard") {
		t.Fatalf("result %v %v %q, want ErrUnderKeyboard", r.Success, r.Error, r.Message)
	}
	if len(k.taps) != 0 {
		t.Errorf("taps = %v, want none", k.taps)
	}
}

// The keyboard's own Done button is tapped where it is.
func TestTapKeyboardButtonIsNotHidden(t *testing.T) {
	parent := 0
	k := &keyboardRunner{nodes: []SnapshotNode{
		{Index: 0, Type: "Keyboard", Rect: SnapshotRect{X: 0, Y: 500, Width: 400, Height: 300}},
		{Index: 1, Type: "Button", Label: "Submit", ParentIndex: &parent, Rect: SnapshotRect{X: 300, Y: 750, Width: 96, Height: 42}},
	}}
	d := k.server(t)
	d.keyboardMayBeUp = true
	if r := d.Execute(tapSubmitStep()); !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	if k.sent(CmdKeyboardDismiss) != 0 || len(k.taps) != 1 {
		t.Errorf("commands %v taps %v, want the button tapped in place", k.commands, k.taps)
	}
}
