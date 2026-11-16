package core

import (
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestScrollProgressObserve(t *testing.T) {
	cases := []struct {
		name string
		seq  []string
		want []bool
	}{
		{"still advancing", []string{"a", "b", "c", "d", "e"}, []bool{false, false, false, false, false}},
		{"end of content", []string{"a", "a", "a", "a"}, []bool{false, false, true, true}},
		{"moved then held", []string{"a", "b", "b", "b"}, []bool{false, false, false, true}},
		{"rubber band", []string{"a", "b", "a", "b"}, []bool{false, false, false, true}},
		{"one no-change scroll is not stuck", []string{"a", "b", "b", "c"}, []bool{false, false, false, false}},
		{"one no-change scroll then a move", []string{"a", "a", "b", "c"}, []bool{false, false, false, false}},
		{"three signatures never stick", []string{"a", "b", "a", "c"}, []bool{false, false, false, false}},
		{"blip then held", []string{"a", "b", "b", "c", "c", "c"}, []bool{false, false, false, false, false, true}},
	}
	for _, c := range cases {
		var p ScrollProgress
		for i, sig := range c.seq {
			if got := p.Observe(sig); got != c.want[i] {
				t.Errorf("%s: after %v Observe = %v, want %v", c.name, c.seq[:i+1], got, c.want[i])
			}
		}
	}
}

func TestScrollProgressNeedsRepeats(t *testing.T) {
	var p ScrollProgress
	for i := 0; i < scrollStuckRepeats-1; i++ {
		if p.Observe("same") {
			t.Fatalf("stuck reported after %d identical captures; %d are required", i+1, scrollStuckRepeats)
		}
	}
}

func TestScrollSignature(t *testing.T) {
	a := ScrollSignature(`<node text="Row 1" bounds="[0,0][100,50]"/>`)
	moved := ScrollSignature(`<node text="Row 1" bounds="[0,-20][100,30]"/>`)
	if a == moved {
		t.Error("a capture whose bounds moved must not share a signature")
	}
	if a != ScrollSignature(`<node text="Row 1" bounds="[0,0][100,50]"/>`) {
		t.Error("identical captures must share a signature")
	}
	if len(a) != 16 {
		t.Errorf("signature length = %d, want 16 hex chars", len(a))
	}
}

func TestScrollLooksUntilTimeout(t *testing.T) {
	if ScrollLooksUntilTimeout(&flow.ScrollUntilVisibleStep{}) {
		t.Error("a step without a timeout of its own keeps the early stop")
	}
	if !ScrollLooksUntilTimeout(&flow.ScrollUntilVisibleStep{BaseStep: flow.BaseStep{TimeoutMs: 30000}}) {
		t.Error("a step with its own timeout looks until it passes")
	}
}

func TestScrollTimeoutMessage(t *testing.T) {
	got := ScrollTimeoutMessage(`id="row"`, "down", 30*time.Second)
	if !strings.HasPrefix(got, `No visible element found: id="row"`) || !strings.Contains(got, "30000ms") {
		t.Errorf("message = %q", got)
	}
	// A selector the driver cannot describe (only a relative or a state
	// filter) leaves no dangling colon.
	if got := ScrollTimeoutMessage("", "down", time.Second); got != "No visible element found (scrolling down until the 1000ms timeout)" {
		t.Errorf("message without a description = %q", got)
	}
}
