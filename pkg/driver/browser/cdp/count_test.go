package cdp

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// The rejection paths run before any page access, so a bare Driver is enough.

func TestAssertVisibleCount_RejectsStateFilters(t *testing.T) {
	d := &Driver{}
	enabled := true
	result := d.assertVisibleCount(flow.Selector{ID: "row", Enabled: &enabled}, 2, 1000)
	if result.Success {
		t.Fatal("expected failure for a state-filter selector")
	}
	if !strings.Contains(result.Message, "state filters") {
		t.Errorf("message should explain the state-filter limitation, got %q", result.Message)
	}
}

func TestAssertVisibleCount_RejectsRole(t *testing.T) {
	d := &Driver{}
	result := d.assertVisibleCount(flow.Selector{Role: "button"}, 2, 1000)
	if result.Success {
		t.Fatal("expected failure for a role selector")
	}
	if !strings.Contains(result.Message, "role") {
		t.Errorf("message should explain the role limitation, got %q", result.Message)
	}
}

func TestAssertVisibleCount_RejectsEmptySelector(t *testing.T) {
	d := &Driver{}
	result := d.assertVisibleCount(flow.Selector{}, 2, 1000)
	if result.Success {
		t.Fatal("expected failure for an empty selector")
	}
}

// The JS helper is embedded at build time — make sure the count functions
// actually ship in the bundle the driver injects.
func TestJSHelperHasCountFunctions(t *testing.T) {
	for _, fn := range []string{"waitForVisibleCount", "_countVisible"} {
		if !strings.Contains(jsHelperCode, fn) {
			t.Errorf("jshelper.js is missing %s", fn)
		}
	}
}
