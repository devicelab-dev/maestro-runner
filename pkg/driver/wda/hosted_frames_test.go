package wda

import (
	"strings"
	"testing"
)

// hostedSheetSource is shaped like Apple Health's authorization sheet on an iPhone 11 running
// iOS 27: the sheet at y 48 in the screen's coordinates, then a view of the same size that
// restarts at (0,0), and the "Select All" row reported 48 points above where it is drawn.
const hostedSheetSource = `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="App" x="0" y="0" width="414" height="896" visible="true">
  <XCUIElementTypeWindow type="XCUIElementTypeWindow" x="0" y="0" width="414" height="896" visible="true">
    <XCUIElementTypeOther type="XCUIElementTypeOther" name="sheet" x="0" y="48" width="414" height="848" visible="true">
      <XCUIElementTypeOther type="XCUIElementTypeOther" name="hosted" x="0" y="0" width="414" height="848" visible="true">
        <XCUIElementTypeTable type="XCUIElementTypeTable" name="table" x="0" y="0" width="414" height="848" visible="true">
          <XCUIElementTypeCell type="XCUIElementTypeCell" name="AllCategoryButton" x="22" y="348" width="370" height="52" visible="true">
            <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="Select All 12 Topics" label="Select All 12 Topics" x="42" y="348" width="330" height="52" visible="true"/>
          </XCUIElementTypeCell>
        </XCUIElementTypeTable>
      </XCUIElementTypeOther>
    </XCUIElementTypeOther>
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="outside" label="Outside" x="10" y="20" width="50" height="30" visible="true"/>
  </XCUIElementTypeWindow>
</XCUIElementTypeApplication>
</AppiumAUT>`

func parsedByName(t *testing.T, xml string) map[string]*ParsedElement {
	t.Helper()
	elements, err := ParsePageSource(xml)
	if err != nil {
		t.Fatalf("ParsePageSource: %v", err)
	}
	byName := map[string]*ParsedElement{}
	for _, e := range elements {
		if e.Name != "" {
			byName[e.Name] = e
		}
	}
	return byName
}

func TestHostedFrames_OffLeavesTheReportedFrames(t *testing.T) {
	for _, v := range []string{"", "0", "false", "FALSE"} {
		t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", v)
		byName := parsedByName(t, hostedSheetSource)
		if got := byName["Select All 12 Topics"].Bounds.Y; got != 348 {
			t.Errorf("MAESTRO_WDA_HOSTED_FRAMES=%q moved the frame: got y %d, want 348", v, got)
		}
	}
}

// WDA's /source without a scope sends the tree with no <AppiumAUT> wrapper, which
// ParsePageSource reads on another path; the correction applies there too.
func TestHostedFrames_UnwrappedSourceAsWDASendsIt(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	unwrapped := strings.NewReplacer("<AppiumAUT>\n", "", "\n</AppiumAUT>", "").Replace(hostedSheetSource)
	if strings.Contains(unwrapped, "AppiumAUT") {
		t.Fatal("the fixture still has its wrapper")
	}
	byName := parsedByName(t, unwrapped)
	if got := byName["Select All 12 Topics"].Bounds.Y; got != 396 {
		t.Fatalf("Select All y=%d, want 396 (348 + the sheet's 48)", got)
	}
}

// Only a child at (0,0) with exactly its parent's width and height starts a space.
func TestHostedFrames_OnlyAnExactFitStartsASpace(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	for name, child := range map[string]string{
		"not at x 0":        `x="10" y="0" width="414" height="848"`,
		"not at y 0":        `x="0" y="10" width="414" height="848"`,
		"narrower":          `x="0" y="0" width="400" height="848"`,
		"shorter":           `x="0" y="0" width="414" height="800"`,
		"exact fit (moved)": `x="0" y="0" width="414" height="848"`,
	} {
		source := `<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="app" x="0" y="0" width="414" height="896">
  <XCUIElementTypeOther type="XCUIElementTypeOther" name="container" x="0" y="48" width="414" height="848">
    <XCUIElementTypeOther type="XCUIElementTypeOther" name="child" ` + child + `>
      <XCUIElementTypeButton type="XCUIElementTypeButton" name="button" x="20" y="300" width="100" height="44"/>
    </XCUIElementTypeOther>
  </XCUIElementTypeOther>
</XCUIElementTypeApplication>`
		want := 300
		if strings.HasPrefix(name, "exact fit") {
			want = 348
		}
		if got := parsedByName(t, source)["button"].Bounds.Y; got != want {
			t.Errorf("%s: button y=%d, want %d", name, got, want)
		}
	}
}

func TestHostedFrames_MovesAHostedSpaceOntoTheScreen(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	byName := parsedByName(t, hostedSheetSource)

	want := map[string][2]int{ // name → {x, y} on the screen
		"sheet":                {0, 48},   // in the screen's coordinates already
		"hosted":               {0, 48},   // restarts at (0,0): the space starts at the sheet
		"table":                {0, 48},   // inside the hosted space
		"AllCategoryButton":    {22, 396}, // 348 + 48, where the screenshot draws it
		"Select All 12 Topics": {42, 396},
		"outside":              {10, 20}, // a sibling of the sheet is not moved
	}
	for name, xy := range want {
		e := byName[name]
		if e == nil {
			t.Fatalf("no element %q", name)
		}
		if e.Bounds.X != xy[0] || e.Bounds.Y != xy[1] {
			t.Errorf("%s at (%d,%d), want (%d,%d)", name, e.Bounds.X, e.Bounds.Y, xy[0], xy[1])
		}
	}
	// A tap at the row's centre now lands on the row, not on the subtitle above it.
	b := byName["Select All 12 Topics"].Bounds
	if cx, cy := b.X+b.Width/2, b.Y+b.Height/2; cx != 207 || cy != 422 {
		t.Errorf("tap point (%d,%d), want (207,422)", cx, cy)
	}
	if b.Width != 330 || b.Height != 52 {
		t.Errorf("the size must not change: %dx%d", b.Width, b.Height)
	}
}

func TestHostedFrames_LeavesAnOrdinaryTreeAlone(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	// A child that fills its parent reports the parent's origin in screen coordinates, and a
	// full-screen child of a full-screen parent is at (0,0) in both: neither is a hosted space.
	const source = `<AppiumAUT>
<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="app" x="0" y="0" width="414" height="896">
  <XCUIElementTypeOther type="XCUIElementTypeOther" name="full" x="0" y="0" width="414" height="896">
    <XCUIElementTypeOther type="XCUIElementTypeOther" name="panel" x="0" y="48" width="414" height="848">
      <XCUIElementTypeOther type="XCUIElementTypeOther" name="fills" x="0" y="48" width="414" height="848">
        <XCUIElementTypeButton type="XCUIElementTypeButton" name="button" x="20" y="300" width="100" height="44"/>
      </XCUIElementTypeOther>
      <XCUIElementTypeOther type="XCUIElementTypeOther" name="smaller" x="0" y="0" width="200" height="100"/>
    </XCUIElementTypeOther>
  </XCUIElementTypeOther>
</XCUIElementTypeApplication>
</AppiumAUT>`
	byName := parsedByName(t, source)
	for name, y := range map[string]int{"full": 0, "panel": 48, "fills": 48, "button": 300, "smaller": 0} {
		if got := byName[name].Bounds.Y; got != y {
			t.Errorf("%s y=%d, want %d (unchanged)", name, got, y)
		}
	}
}

func TestHostedFrames_NestedSpacesAddUp(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	const source = `<AppiumAUT>
<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="app" x="0" y="0" width="414" height="896">
  <XCUIElementTypeOther type="XCUIElementTypeOther" name="outer" x="10" y="40" width="400" height="800">
    <XCUIElementTypeOther type="XCUIElementTypeOther" name="outerHosted" x="0" y="0" width="400" height="800">
      <XCUIElementTypeOther type="XCUIElementTypeOther" name="inner" x="5" y="100" width="300" height="200">
        <XCUIElementTypeOther type="XCUIElementTypeOther" name="innerHosted" x="0" y="0" width="300" height="200">
          <XCUIElementTypeButton type="XCUIElementTypeButton" name="button" x="10" y="20" width="50" height="30"/>
        </XCUIElementTypeOther>
      </XCUIElementTypeOther>
    </XCUIElementTypeOther>
  </XCUIElementTypeOther>
</XCUIElementTypeApplication>
</AppiumAUT>`
	byName := parsedByName(t, source)
	// inner is at (5,100) in the outer space, so (15,140) on screen; its hosted space starts
	// there, and the button at (10,20) inside it is at (25,160).
	want := map[string][2]int{"inner": {15, 140}, "innerHosted": {15, 140}, "button": {25, 160}}
	for name, xy := range want {
		b := byName[name].Bounds
		if b.X != xy[0] || b.Y != xy[1] {
			t.Errorf("%s at (%d,%d), want (%d,%d)", name, b.X, b.Y, xy[0], xy[1])
		}
	}
}

func TestHostedFrames_ZeroSizeChildIsNotASpace(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	const source = `<AppiumAUT>
<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="app" x="0" y="0" width="414" height="896">
  <XCUIElementTypeOther type="XCUIElementTypeOther" name="collapsed" x="0" y="48" width="0" height="0">
    <XCUIElementTypeOther type="XCUIElementTypeOther" name="empty" x="0" y="0" width="0" height="0">
      <XCUIElementTypeButton type="XCUIElementTypeButton" name="button" x="20" y="300" width="100" height="44"/>
    </XCUIElementTypeOther>
  </XCUIElementTypeOther>
</XCUIElementTypeApplication>
</AppiumAUT>`
	byName := parsedByName(t, source)
	if got := byName["button"].Bounds.Y; got != 300 {
		t.Fatalf("a zero-size (0,0) child starts no space: button y=%d, want 300", got)
	}
}

func TestHostedFrames_LeavesAWebViewAlone(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	// Shaped like Google's sign-in sheet on the iPhone (iOS 27): a web view at (0,48) whose
	// same-size child sits at (0,0), and links reported where the screen draws them.
	const source = `<AppiumAUT>
<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="app" x="0" y="0" width="414" height="896">
  <XCUIElementTypeOther type="XCUIElementTypeOther" name="sheet" x="0" y="48" width="414" height="848">
    <XCUIElementTypeWebView type="XCUIElementTypeWebView" name="web" x="0" y="48" width="414" height="848">
      <XCUIElementTypeOther type="XCUIElementTypeOther" name="content" x="0" y="0" width="414" height="848">
        <XCUIElementTypeLink type="XCUIElementTypeLink" name="account" label="Tester" x="64" y="339" width="71" height="22"/>
        <XCUIElementTypeLink type="XCUIElementTypeLink" name="another" label="Use another account" x="64" y="410" width="157" height="22"/>
      </XCUIElementTypeOther>
    </XCUIElementTypeWebView>
  </XCUIElementTypeOther>
</XCUIElementTypeApplication>
</AppiumAUT>`
	byName := parsedByName(t, source)
	for name, y := range map[string]int{"web": 48, "content": 0, "account": 339, "another": 410} {
		if got := byName[name].Bounds.Y; got != y {
			t.Errorf("%s y=%d, want %d (a web view's content is in screen coordinates already)", name, got, y)
		}
	}
}
