package wda

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// WDA's source marks a selected tab or segment only in traits ("Selected,
// Button"); there is no selected attribute.
const selectedTraitsSource = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="App" label="App" enabled="true" visible="true" x="0" y="0" width="390" height="844">` +
	`<XCUIElementTypeButton type="XCUIElementTypeButton" name="tab_home" label="Home" enabled="true" visible="true" x="2" y="770" width="76" height="48" traits="Selected, Button"/>` +
	`<XCUIElementTypeButton type="XCUIElementTypeButton" name="tab_live" label="Live" enabled="true" visible="true" x="80" y="770" width="76" height="48" traits="Button"/>` +
	`<XCUIElementTypeButton type="XCUIElementTypeButton" name="tab_odd" label="Odd" enabled="true" visible="true" x="160" y="770" width="76" height="48" traits="NotSelected, Button"/>` +
	`<XCUIElementTypeButton type="XCUIElementTypeButton" name="tab_attr" label="Attr" enabled="true" visible="true" x="240" y="770" width="76" height="48" selected="true"/>` +
	`</XCUIElementTypeApplication>`

func TestPageSourceSelectedFromTraits(t *testing.T) {
	elems, err := ParsePageSource(selectedTraitsSource)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range elems {
		got[e.Name] = e.Selected
	}
	want := map[string]bool{"tab_home": true, "tab_live": false, "tab_odd": false, "tab_attr": true}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s selected = %v, want %v", name, got[name], w)
		}
	}

	yes, no := true, false
	sel := FilterBySelector(elems, flow.Selector{Selected: &yes})
	if len(sel) != 2 || sel[0].Name != "tab_home" || sel[1].Name != "tab_attr" {
		names := []string{}
		for _, e := range sel {
			names = append(names, e.Name)
		}
		t.Errorf("selected: true matched %v, want [tab_home tab_attr]", names)
	}
	for _, e := range FilterBySelector(elems, flow.Selector{Text: "Home", Selected: &no}) {
		t.Errorf("selected: false matched the selected tab %q", e.Name)
	}
}

func TestHasTrait(t *testing.T) {
	cases := []struct {
		traits string
		want   bool
	}{
		{"Selected, Button", true},
		{"Button, Selected", true},
		{"Selected", true},
		{" Selected ,Button", true},
		{"Button", false},
		{"NotSelected, Button", false},
		{"", false},
	}
	for _, c := range cases {
		if got := hasTrait(c.traits, "Selected"); got != c.want {
			t.Errorf("hasTrait(%q) = %v, want %v", c.traits, got, c.want)
		}
	}
}
