package wda

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Three rows of buttons: the first row's are all off, the second's on, the
// third's first off and its second on (#280).
const threeRows = `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" label="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
<XCUIElementTypeOther type="XCUIElementTypeOther" name="row" enabled="true" visible="true" x="0" y="100" width="390" height="80">
<XCUIElementTypeButton type="XCUIElementTypeButton" name="button_off" label="r1b1" enabled="true" visible="true" x="10" y="110" width="170" height="60"/>
<XCUIElementTypeButton type="XCUIElementTypeButton" name="button_off" label="r1b2" enabled="true" visible="true" x="200" y="110" width="170" height="60"/>
</XCUIElementTypeOther>
<XCUIElementTypeOther type="XCUIElementTypeOther" name="row" enabled="true" visible="true" x="0" y="200" width="390" height="80">
<XCUIElementTypeButton type="XCUIElementTypeButton" name="button_on" label="r2b1" enabled="true" visible="true" x="10" y="210" width="170" height="60"/>
<XCUIElementTypeButton type="XCUIElementTypeButton" name="button_on" label="r2b2" enabled="true" visible="true" x="200" y="210" width="170" height="60"/>
</XCUIElementTypeOther>
<XCUIElementTypeOther type="XCUIElementTypeOther" name="row" enabled="true" visible="true" x="0" y="300" width="390" height="80">
<XCUIElementTypeButton type="XCUIElementTypeButton" name="button_off" label="r3b1" enabled="true" visible="true" x="10" y="310" width="170" height="60"/>
<XCUIElementTypeButton type="XCUIElementTypeButton" name="button_on" label="r3b2" enabled="true" visible="true" x="200" y="310" width="170" height="60"/>
</XCUIElementTypeOther>
</XCUIElementTypeApplication>
</AppiumAUT>`

// The anchor's own containsDescendants applies before its index, as in
// Maestro: the index counts only the rows that hold a button_on, and an anchor
// whose descendant matches nothing is not found (#280).
func TestChildOfAnchorContainsDescendants(t *testing.T) {
	elements, err := ParsePageSource(threeRows)
	if err != nil {
		t.Fatal(err)
	}
	d := &Driver{}
	selector := func(descendant, anchorIndex string) flow.Selector {
		return flow.Selector{
			ID: "button_.*",
			ChildOf: &flow.Selector{
				ID:                  "row",
				ContainsDescendants: []*flow.Selector{{ID: descendant}},
				Index:               anchorIndex,
			},
			Index: "0",
		}
	}

	for anchorIndex, want := range map[string]string{
		"":   "r2b1",
		"0":  "r2b1",
		"1":  "r3b1",
		"-1": "r3b1",
	} {
		sel := selector("button_on", anchorIndex)
		info, err := d.resolveRelativeSelector(sel, elements)
		if err != nil || info == nil || info.Text != want {
			t.Errorf("anchor index %q = %+v, %v; want %s", anchorIndex, info, err, want)
		}
	}
	for name, sel := range map[string]flow.Selector{
		"index past the matching rows":              selector("button_on", "2"),
		"descendant that matches nothing":           selector("button_none", "0"),
		"descendant that matches nothing, no index": selector("button_none", ""),
	} {
		info, err := d.resolveRelativeSelector(sel, elements)
		if err == nil || !strings.Contains(err.Error(), "anchor element not found") {
			t.Errorf("%s = %+v, %v; want anchor element not found", name, info, err)
		}
	}
}
