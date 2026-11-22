package devicelab

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Two buttons whose labels are child TextViews: "Submit" in an enabled button,
// "Delete" in a disabled one. Each label is itself enabled, and so is every
// label's own state, so a lookup that takes the deepest element matching the
// state filter first lands on the label and loses the button.
const containsChildStateLayout = `<hierarchy rotation="0">
<node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,0][1080,2000]" displayed="true" enabled="true">
<node index="0" text="" resource-id="submit" class="android.widget.Button" bounds="[0,100][1080,300]" displayed="true" enabled="true" clickable="true">
<node index="0" text="Submit" resource-id="" class="android.widget.TextView" bounds="[400,150][680,250]" displayed="true" enabled="true"/>
</node>
<node index="1" text="" resource-id="delete" class="android.widget.Button" bounds="[0,400][1080,600]" displayed="true" enabled="false" clickable="true">
<node index="0" text="Delete" resource-id="" class="android.widget.TextView" bounds="[400,450][680,550]" displayed="true" enabled="true"/>
</node>
</node>
</hierarchy>`

// containsChild combined with a state filter (enabled) resolves to the
// container with that state, not the matching child and not nothing.
func TestContainsChildWithEnabled(t *testing.T) {
	c := &splitClient{trackingClient: newTrackingClient(), snapshot: containsChildStateLayout, source: containsChildStateLayout}
	d := New(c, &core.PlatformInfo{}, &mockShell{})
	enabled, disabled := true, false

	tests := []struct {
		name string
		sel  flow.Selector
		want core.Bounds
	}{
		{"enabled button containing Submit", flow.Selector{ContainsChild: &flow.Selector{Text: "Submit"}, Enabled: &enabled},
			core.Bounds{X: 0, Y: 100, Width: 1080, Height: 200}},
		{"disabled button containing Delete", flow.Selector{ContainsChild: &flow.Selector{Text: "Delete"}, Enabled: &disabled},
			core.Bounds{X: 0, Y: 400, Width: 1080, Height: 200}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := d.resolveRelativeSelector(tt.sel)
			if err != nil || info == nil {
				t.Fatalf("resolveRelativeSelector = %+v, %v; want the button", info, err)
			}
			if info.Bounds != tt.want {
				t.Errorf("bounds = %+v, want %+v", info.Bounds, tt.want)
			}
		})
	}

	// No disabled element contains "Submit".
	if info, err := d.resolveRelativeSelector(flow.Selector{ContainsChild: &flow.Selector{Text: "Submit"}, Enabled: &disabled}); err == nil {
		t.Errorf("containsChild Submit + enabled:false matched %+v, want no match", info)
	}
}
