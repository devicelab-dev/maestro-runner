package appium

import "testing"

// A selected tab shows up only in XCUITest's traits.
func TestIOSSelectedFromTraits(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?><AppiumAUT><XCUIElementTypeApplication type="XCUIElementTypeApplication" name="App" x="0" y="0" width="390" height="844" visible="true">` +
		`<XCUIElementTypeButton type="XCUIElementTypeButton" name="tab_home" label="Home" enabled="true" visible="true" x="2" y="770" width="76" height="48" traits="Selected, Button"/>` +
		`<XCUIElementTypeButton type="XCUIElementTypeButton" name="tab_live" label="Live" enabled="true" visible="true" x="80" y="770" width="76" height="48" traits="Button"/>` +
		`</XCUIElementTypeApplication></AppiumAUT>`
	elems, _, err := ParsePageSource(src)
	if err != nil {
		t.Fatal(err)
	}
	sel := map[string]bool{}
	for _, e := range elems {
		sel[e.ResourceID+e.Name] = e.Selected
	}
	if !sel["tab_home"] || sel["tab_live"] {
		t.Fatalf("selected = %v, want only tab_home", sel)
	}
}
