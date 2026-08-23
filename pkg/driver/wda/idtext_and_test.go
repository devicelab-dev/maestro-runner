package wda

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// TestFindElementByWDA_DefersCombinedIDText verifies that a selector with BOTH
// id and text is not resolved by the single-field fast paths (which would OR
// them), but deferred so the page-source AND matcher runs instead (#130).
func TestFindElementByWDA_DefersCombinedIDText(t *testing.T) {
	d := &Driver{info: &core.PlatformInfo{Platform: "ios"}} // nil client is fine: defer returns before any call
	_, err := d.findElementByWDA(flow.Selector{ID: "boss.hp", Text: "999 misses"})
	if err == nil {
		t.Error("combined id+text should defer (return error) to the page-source AND matcher, got nil")
	}
}

// TestMatchesSelector_IDAndTextAreANDed is the correctness guarantee behind the
// #130 fix: the page-source matcher requires BOTH id and text on one element.
func TestMatchesSelector_IDAndTextAreANDed(t *testing.T) {
	// Element with the right id but the WRONG displayed text.
	boss := &ParsedElement{Type: "XCUIElementTypeButton", Name: "boss.hp", Label: "7 misses"}
	// A different element whose label merely contains the text.
	image := &ParsedElement{Type: "XCUIElementTypeImage", Name: "amulet.icon", Label: "Cobble, Steady"}

	// id matches boss but text does not → no match (was: passed via id-only OR).
	if matchesSelector(boss, flow.Selector{ID: "boss.hp", Text: "999 misses"}) {
		t.Error("id boss.hp + wrong text '999 misses' must NOT match (AND, not OR)")
	}
	// text matches image but id does not → no match (was: passed via text-only OR).
	if matchesSelector(image, flow.Selector{ID: "amulet.tier", Text: "Steady"}) {
		t.Error("wrong id amulet.tier + substring text 'Steady' must NOT match a different element")
	}
	// Correct: id + real text on the same element → match.
	if !matchesSelector(boss, flow.Selector{ID: "boss.hp", Text: "7 misses"}) {
		t.Error("id boss.hp + correct text '7 misses' should match")
	}
}
