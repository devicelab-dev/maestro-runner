package uiautomator2

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Three rows of buttons: the first row's are all off, the second's on, the
// third's first off and its second on (#280).
const threeRows = `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
<node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,0][1080,2000]" displayed="true" enabled="true">
<node index="0" text="" resource-id="row" class="android.view.ViewGroup" bounds="[0,100][1080,300]" displayed="true" enabled="true">
<node index="0" text="r1b1" resource-id="button_off" class="android.widget.Button" bounds="[40,150][500,250]" displayed="true" enabled="true" clickable="true"/>
<node index="1" text="r1b2" resource-id="button_off" class="android.widget.Button" bounds="[580,150][1040,250]" displayed="true" enabled="true" clickable="true"/>
</node>
<node index="1" text="" resource-id="row" class="android.view.ViewGroup" bounds="[0,400][1080,600]" displayed="true" enabled="true">
<node index="0" text="r2b1" resource-id="button_on" class="android.widget.Button" bounds="[40,450][500,550]" displayed="true" enabled="true" clickable="true"/>
<node index="1" text="r2b2" resource-id="button_on" class="android.widget.Button" bounds="[580,450][1040,550]" displayed="true" enabled="true" clickable="true"/>
</node>
<node index="2" text="" resource-id="row" class="android.view.ViewGroup" bounds="[0,700][1080,900]" displayed="true" enabled="true">
<node index="0" text="r3b1" resource-id="button_off" class="android.widget.Button" bounds="[40,750][500,850]" displayed="true" enabled="true" clickable="true"/>
<node index="1" text="r3b2" resource-id="button_on" class="android.widget.Button" bounds="[580,750][1040,850]" displayed="true" enabled="true" clickable="true"/>
</node>
</node>
</hierarchy>`

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
		_, info, err := d.findElementRelativeWithElements(sel, elements)
		if err != nil || info == nil || info.Text != want {
			t.Errorf("anchor index %q = %+v, %v; want %s", anchorIndex, info, err, want)
		}
	}
	for name, sel := range map[string]flow.Selector{
		"index past the matching rows":              selector("button_on", "2"),
		"descendant that matches nothing":           selector("button_none", "0"),
		"descendant that matches nothing, no index": selector("button_none", ""),
	} {
		_, info, err := d.findElementRelativeWithElements(sel, elements)
		if err == nil || !strings.Contains(err.Error(), "anchor element not found") {
			t.Errorf("%s = %+v, %v; want anchor element not found", name, info, err)
		}
	}
}
