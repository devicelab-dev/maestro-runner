package appium

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// settingsHome is the screen from #254: Settings text views that are not
// clickable, and the 3-button navigation bar whose buttons carry only a
// content-desc (which `.+` matches) and are clickable. The navigation bar
// comes first in the tree, as its window does on a device.
const settingsHome = `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,2200][1080,2300]">
    <android.widget.ImageView content-desc="Recent apps" resource-id="com.android.systemui:id/recent_apps" clickable="true" enabled="true" displayed="true" bounds="[0,2200][360,2300]"/>
    <android.widget.ImageView content-desc="Home" resource-id="com.android.systemui:id/home" clickable="true" enabled="true" displayed="true" bounds="[360,2200][720,2300]"/>
    <android.widget.ImageView content-desc="Back" resource-id="com.android.systemui:id/back" clickable="true" enabled="true" displayed="true" bounds="[720,2200][1080,2300]"/>
  </android.widget.FrameLayout>
  <android.widget.FrameLayout bounds="[0,0][1080,2200]">
    <android.widget.TextView text="Connections" enabled="true" displayed="true" bounds="[0,300][1080,360]"/>
    <android.widget.TextView text="Wi-Fi • Bluetooth • Airplane mode" enabled="true" displayed="true" bounds="[0,370][1080,420]"/>
    <android.widget.TextView text="Connected devices" enabled="true" displayed="true" bounds="[0,500][1080,560]"/>
  </android.widget.FrameLayout>
</hierarchy>`

func newSettingsHomeDriver(t *testing.T) *Driver {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			writeJSON(w, map[string]interface{}{"value": settingsHome})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	t.Cleanup(server.Close)
	return createTestAppiumDriver(server)
}

// With an index, candidates are counted by screen position, as in Maestro:
// `below: Connections, text: .+, index: 0` is the summary line under it, not
// a clickable navigation-bar button (#254).
func TestRelativeIndexCountsByPosition(t *testing.T) {
	d := newSettingsHomeDriver(t)
	elements, platform, err := ParsePageSource(settingsHome)
	if err != nil {
		t.Fatal(err)
	}
	below := &flow.Selector{Text: "Connections"}

	cases := []struct {
		index string
		check func(*core.ElementInfo) bool
		want  string
	}{
		{"0", func(i *core.ElementInfo) bool { return i.Text == "Wi-Fi • Bluetooth • Airplane mode" }, "the summary line"},
		{"1", func(i *core.ElementInfo) bool { return i.Text == "Connected devices" }, "Connected devices"},
		{"-1", func(i *core.ElementInfo) bool { return i.Bounds.X == 720 && i.Bounds.Y == 2200 }, "Back (last by position)"},
	}
	for _, c := range cases {
		info, err := d.findElementRelativeWithElements(flow.Selector{Text: ".+", Below: below, Index: c.index}, elements, platform)
		if err != nil || !c.check(info) {
			t.Errorf("below Connections, index %s = %+v, %v; want %s", c.index, info, err, c.want)
		}
	}
	if _, err := d.findElementRelativeWithElements(flow.Selector{Text: ".+", Below: below, Index: "5"}, elements, platform); err == nil {
		t.Error("index 5 of 5 matches found an element, want an error")
	}
}

// Without an index the clickable-first order still applies: a tap below
// Connections goes to the nearest clickable match.
func TestRelativeWithoutIndexKeepsClickableFirst(t *testing.T) {
	d := newSettingsHomeDriver(t)
	elements, platform, err := ParsePageSource(settingsHome)
	if err != nil {
		t.Fatal(err)
	}
	info, err := d.findElementRelativeWithElements(flow.Selector{Text: ".+", Below: &flow.Selector{Text: "Connections"}}, elements, platform)
	if err != nil || info.Bounds.Y != 2200 {
		t.Errorf("below Connections, no index = %+v, %v; want a navigation-bar button", info, err)
	}
}

// A plain selector with an index counts by position too.
func TestPageSourceIndexCountsByPosition(t *testing.T) {
	d := newSettingsHomeDriver(t)
	if info, err := d.findElementByPageSource(flow.Selector{Text: ".+", Index: "0"}); err != nil || info.Text != "Connections" {
		t.Errorf("index 0 = %+v, %v; want Connections (top-most)", info, err)
	}
	if info, err := d.findElementByPageSource(flow.Selector{Text: ".+", Index: "1"}); err != nil || info.Text != "Wi-Fi • Bluetooth • Airplane mode" {
		t.Errorf("index 1 = %+v, %v; want the summary line", info, err)
	}
	if info, err := d.findElementByPageSource(flow.Selector{Text: ".+", Index: "-1"}); err != nil || info.Bounds.X != 720 {
		t.Errorf("index -1 = %+v, %v; want Back (right-most of the bottom row)", info, err)
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
