package wda

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// settingsHome mirrors the screen from #254 on iOS: static texts that are not
// interactive, and a tab bar of buttons (interactive by type) at the bottom,
// which come first in the tree.
const settingsHome = `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="App" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="tabA" label="Recents" enabled="true" visible="true" x="0" y="780" width="130" height="50"/>
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="tabB" label="Home" enabled="true" visible="true" x="130" y="780" width="130" height="50"/>
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="tabC" label="Back" enabled="true" visible="true" x="260" y="780" width="130" height="50"/>
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="Connections" label="Connections" enabled="true" visible="true" x="0" y="100" width="390" height="30"/>
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="summary" label="Wi-Fi • Bluetooth • Airplane mode" enabled="true" visible="true" x="0" y="140" width="390" height="30"/>
    <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="devices" label="Connected devices" enabled="true" visible="true" x="0" y="200" width="390" height="30"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`

func newSettingsHomeDriver(t *testing.T) *Driver {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			jsonResponse(w, map[string]interface{}{"value": settingsHome})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	t.Cleanup(server.Close)
	return createTestDriver(server)
}

// With an index, candidates are counted by screen position, as in Maestro:
// `below: Connections, text: .+, index: 0` is the summary line under it, not
// an interactive tab-bar button (#254).
func TestRelativeIndexCountsByPosition(t *testing.T) {
	d := newSettingsHomeDriver(t)
	elements, err := ParsePageSource(settingsHome)
	if err != nil {
		t.Fatal(err)
	}
	below := &flow.Selector{Text: "Connections"}

	for index, want := range map[string]string{
		"0":  "Wi-Fi • Bluetooth • Airplane mode",
		"1":  "Connected devices",
		"-1": "Back",
	} {
		info, err := d.resolveRelativeSelector(flow.Selector{Text: ".+", Below: below, Index: index}, elements)
		if err != nil || info.Text != want {
			t.Errorf("below Connections, index %s = %+v, %v; want %q", index, info, err, want)
		}
	}
	if _, err := d.resolveRelativeSelector(flow.Selector{Text: ".+", Below: below, Index: "5"}, elements); err == nil {
		t.Error("index 5 of 5 matches found an element, want an error")
	}
}

// Without an index the interactive-first order still applies.
func TestRelativeWithoutIndexKeepsClickableFirst(t *testing.T) {
	d := newSettingsHomeDriver(t)
	elements, err := ParsePageSource(settingsHome)
	if err != nil {
		t.Fatal(err)
	}
	info, err := d.resolveRelativeSelector(flow.Selector{Text: ".+", Below: &flow.Selector{Text: "Connections"}}, elements)
	if err != nil || info.Bounds.Y != 780 {
		t.Errorf("below Connections, no index = %+v, %v; want a tab-bar button", info, err)
	}
}

// A plain selector with an index counts by position too.
func TestPageSourceIndexCountsByPosition(t *testing.T) {
	d := newSettingsHomeDriver(t)
	for index, want := range map[string]string{
		"0":  "Connections",
		"1":  "Wi-Fi • Bluetooth • Airplane mode",
		"-1": "Back",
	} {
		if info, err := d.findElementByPageSourceOnce(flow.Selector{Text: ".+", Index: index}); err != nil || info.Text != want {
			t.Errorf("index %s = %+v, %v; want %q", index, info, err, want)
		}
	}
}

func TestSortCandidates(t *testing.T) {
	top := &ParsedElement{Label: "top", Type: "XCUIElementTypeStaticText", Bounds: core.Bounds{X: 0, Y: 100, Width: 10, Height: 10}}
	btn := &ParsedElement{Label: "btn", Type: "XCUIElementTypeButton", Bounds: core.Bounds{X: 0, Y: 700, Width: 10, Height: 10}}
	in := []*ParsedElement{top, btn}

	if got := SortCandidates(in, ""); got[0] != btn {
		t.Errorf("no index: first = %q, want the button", got[0].Label)
	}
	if got := SortCandidates(in, "0"); got[0] != top {
		t.Errorf("index: first = %q, want the top-most one", got[0].Label)
	}
}
