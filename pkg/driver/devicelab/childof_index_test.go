package devicelab

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Two event cards side by side, each with its odd buttons.
const twoEventCards = `<hierarchy rotation="0">
<node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,0][1080,2000]" displayed="true" enabled="true">
<node index="0" text="" resource-id="topEvent" class="android.view.View" bounds="[16,670][404,900]" displayed="true" enabled="true" clickable="true">
<node index="0" text="1.24" resource-id="oddButton" class="android.view.View" bounds="[32,838][128,880]" displayed="true" enabled="true" clickable="true"/>
<node index="1" text="7.10" resource-id="oddButton" class="android.view.View" bounds="[138,838][234,880]" displayed="true" enabled="true" clickable="true"/>
</node>
<node index="1" text="" resource-id="topEvent" class="android.view.View" bounds="[436,670][824,900]" displayed="true" enabled="true" clickable="true">
<node index="0" text="1.20" resource-id="oddButton" class="android.view.View" bounds="[452,838][548,880]" displayed="true" enabled="true" clickable="true"/>
<node index="1" text="5.50" resource-id="oddButton" class="android.view.View" bounds="[558,838][654,880]" displayed="true" enabled="true" clickable="true"/>
</node>
</node>
</hierarchy>`

// The anchor of childOf keeps its own index, as in Maestro: the second card's
// first odd, not the first card's.
func TestChildOfAnchorIndex(t *testing.T) {
	c := &splitClient{trackingClient: newTrackingClient(), snapshot: twoEventCards, source: twoEventCards}
	d := New(c, &core.PlatformInfo{}, &mockShell{})

	for _, tc := range []struct {
		card, odd, want string
	}{
		{"0", "0", "1.24"},
		{"1", "0", "1.20"},
		{"1", "1", "5.50"},
		{"-1", "0", "1.20"},
	} {
		info, err := d.resolveRelativeSelector(flow.Selector{
			ID:      "oddButton",
			ChildOf: &flow.Selector{ID: "topEvent", Index: tc.card},
			Index:   tc.odd,
		})
		if err != nil || info == nil || info.Text != tc.want {
			t.Errorf("card %s odd %s = %+v, %v; want %s", tc.card, tc.odd, info, err, tc.want)
		}
	}
	for index, want := range map[string]string{
		"2":         "no element at index 2: 2 match(es)",
		"undefined": `invalid index "undefined"`,
	} {
		_, err := d.resolveRelativeSelector(flow.Selector{ID: "oddButton", ChildOf: &flow.Selector{ID: "topEvent", Index: index}})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("card index %s: error %v, want %q", index, err, want)
		}
	}
}
