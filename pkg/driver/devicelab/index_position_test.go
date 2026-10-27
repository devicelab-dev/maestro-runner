package devicelab

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// settingsHome is the screen from #254: Settings text views that are not
// clickable, and the 3-button navigation bar whose buttons carry only a
// content-desc (which `.+` matches) and are clickable. The navigation bar
// comes first in the tree, as its window does on a device.
const settingsHome = `<hierarchy rotation="0">
<node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,2300][1080,2400]" displayed="true" enabled="true">
<node index="0" text="" content-desc="Recent apps" resource-id="com.android.systemui:id/recent_apps" class="android.widget.ImageView" clickable="true" bounds="[0,2300][360,2400]" displayed="true" enabled="true"/>
<node index="1" text="" content-desc="Home" resource-id="com.android.systemui:id/home" class="android.widget.ImageView" clickable="true" bounds="[360,2300][720,2400]" displayed="true" enabled="true"/>
<node index="2" text="" content-desc="Back" resource-id="com.android.systemui:id/back" class="android.widget.ImageView" clickable="true" bounds="[720,2300][1080,2400]" displayed="true" enabled="true"/>
</node>
<node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,0][1080,2300]" displayed="true" enabled="true">
<node index="0" text="Connections" resource-id="android:id/title" class="android.widget.TextView" bounds="[0,300][1080,360]" displayed="true" enabled="true"/>
<node index="1" text="Wi-Fi • Bluetooth • Airplane mode" resource-id="android:id/summary" class="android.widget.TextView" bounds="[0,370][1080,420]" displayed="true" enabled="true"/>
<node index="2" text="Connected devices" resource-id="android:id/title" class="android.widget.TextView" bounds="[0,500][1080,560]" displayed="true" enabled="true"/>
</node>
</hierarchy>`

func newSettingsHomeDriver() *Driver {
	c := &splitClient{trackingClient: newTrackingClient(), snapshot: settingsHome, source: settingsHome}
	return New(c, &core.PlatformInfo{}, &mockShell{})
}

// With an index, candidates are counted by screen position, as in Maestro:
// `below: Connections, text: .+, index: 0` is the summary line under it, not
// a clickable navigation-bar button (#254).
func TestRelativeIndexCountsByPosition(t *testing.T) {
	d := newSettingsHomeDriver()
	below := &flow.Selector{Text: "Connections"}

	info, err := d.resolveRelativeSelector(flow.Selector{Text: ".+", Below: below, Index: "0"})
	if err != nil || info.Text != "Wi-Fi • Bluetooth • Airplane mode" {
		t.Errorf("below Connections, index 0 = %+v, %v; want the summary line", info, err)
	}

	elements, err := ParsePageSource(settingsHome)
	if err != nil {
		t.Fatal(err)
	}
	_, info, err = d.findElementRelativeWithElements(flow.Selector{Text: ".+", Below: below, Index: "1"}, elements)
	if err != nil || info.Text != "Connected devices" {
		t.Errorf("below Connections, index 1 = %+v, %v; want Connected devices", info, err)
	}
	_, info, err = d.findElementRelativeWithElements(flow.Selector{Text: ".+", Below: below, Index: "-1"}, elements)
	if err != nil || info.Bounds.X != 720 || info.Bounds.Y != 2300 {
		t.Errorf("below Connections, index -1 = %+v, %v; want Back (last by position)", info, err)
	}
	if _, _, err := d.findElementRelativeWithElements(flow.Selector{Text: ".+", Below: below, Index: "5"}, elements); err == nil {
		t.Error("index 5 of 5 matches found an element, want an error")
	}
}

// Without an index the clickable-first order still applies: a tap below
// Connections goes to the nearest clickable match.
func TestRelativeWithoutIndexKeepsClickableFirst(t *testing.T) {
	d := newSettingsHomeDriver()
	info, err := d.resolveRelativeSelector(flow.Selector{Text: ".+", Below: &flow.Selector{Text: "Connections"}})
	if err != nil || info.Bounds.Y != 2300 {
		t.Errorf("below Connections, no index = %+v, %v; want a navigation-bar button", info, err)
	}
}

// A plain selector with an index counts by position too.
func TestPageSourceIndexCountsByPosition(t *testing.T) {
	d := newSettingsHomeDriver()
	for _, find := range []func(flow.Selector) (*core.ElementInfo, error){
		func(s flow.Selector) (*core.ElementInfo, error) {
			_, i, err := d.findElementByPageSourceOnce(s)
			return i, err
		},
		d.findElementByPageSourceOnceInternal,
	} {
		if info, err := find(flow.Selector{Text: ".+", Index: "0"}); err != nil || info.Text != "Connections" {
			t.Errorf("index 0 = %+v, %v; want Connections (top-most)", info, err)
		}
		if info, err := find(flow.Selector{Text: ".+", Index: "1"}); err != nil || info.Text != "Wi-Fi • Bluetooth • Airplane mode" {
			t.Errorf("index 1 = %+v, %v; want the summary line", info, err)
		}
		if info, err := find(flow.Selector{Text: ".+", Index: "-1"}); err != nil || info.Bounds.X != 720 {
			t.Errorf("index -1 = %+v, %v; want Back (right-most of the bottom row)", info, err)
		}
	}
}

func TestSortCandidates(t *testing.T) {
	top := &ParsedElement{Text: "top", Bounds: core.Bounds{X: 0, Y: 100, Width: 10, Height: 10}}
	btn := &ParsedElement{Text: "btn", Clickable: true, Bounds: core.Bounds{X: 0, Y: 900, Width: 10, Height: 10}}
	in := []*ParsedElement{top, btn}

	if got := SortCandidates(in, ""); got[0] != btn {
		t.Errorf("no index: first = %q, want the clickable one", got[0].Text)
	}
	if got := SortCandidates(in, "0"); got[0] != top {
		t.Errorf("index: first = %q, want the top-most one", got[0].Text)
	}
}
