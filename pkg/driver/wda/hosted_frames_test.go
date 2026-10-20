package wda

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
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

// A web view wrapped in more views (SFSafariViewController) is still a web
// view: no space starts anywhere under it.
func TestHostedFrames_LeavesAWrappedWebViewAlone(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	source := `<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="app" x="0" y="0" width="414" height="896">
  <XCUIElementTypeWebView type="XCUIElementTypeWebView" name="web" x="0" y="48" width="414" height="848">
    <XCUIElementTypeScrollView type="XCUIElementTypeScrollView" name="scroll" x="0" y="48" width="414" height="848">
      <XCUIElementTypeOther type="XCUIElementTypeOther" name="content" x="0" y="0" width="414" height="848">
        <XCUIElementTypeLink type="XCUIElementTypeLink" name="account" label="Tester" x="64" y="339" width="71" height="22"/>
      </XCUIElementTypeOther>
    </XCUIElementTypeScrollView>
  </XCUIElementTypeWebView>
</XCUIElementTypeApplication>`
	if got := parsedByName(t, source)["account"].Bounds.Y; got != 339 {
		t.Fatalf("a link under a wrapped web view moved: y=%d, want 339", got)
	}
}

// A hosted space that does not start at x 0 (an iPad form sheet) moves its
// content right as well as down.
func TestHostedFrames_ASpaceAwayFromTheLeftEdge(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	source := `<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="app" x="0" y="0" width="1024" height="1366">
  <XCUIElementTypeOther type="XCUIElementTypeOther" name="sheet" x="212" y="183" width="600" height="1000">
    <XCUIElementTypeOther type="XCUIElementTypeOther" name="hosted" x="0" y="0" width="600" height="1000">
      <XCUIElementTypeButton type="XCUIElementTypeButton" name="allow" label="Allow" x="40" y="900" width="520" height="50"/>
    </XCUIElementTypeOther>
  </XCUIElementTypeOther>
</XCUIElementTypeApplication>`
	b := parsedByName(t, source)["allow"].Bounds
	if b.X != 252 || b.Y != 1083 {
		t.Fatalf("Allow at (%d,%d), want (252,1083): the sheet's origin added to both", b.X, b.Y)
	}
}

// hostedLookupServer answers /source with the Health-shaped sheet and every
// WDA element query with an element at the frame WDA reports, uncorrected.
// It counts the element queries.
type hostedLookupServer struct {
	mu      sync.Mutex
	queries int
	// textFails makes every element text read fail, as a dropped read does.
	textFails bool
}

func (s *hostedLookupServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/source"):
			jsonResponse(w, map[string]interface{}{"value": hostedSheetSource})
		case r.Method == http.MethodPost && (strings.HasSuffix(p, "/element") || strings.HasSuffix(p, "/elements")):
			s.mu.Lock()
			s.queries++
			s.mu.Unlock()
			if strings.HasSuffix(p, "/elements") {
				jsonResponse(w, map[string]interface{}{"value": []map[string]interface{}{{"ELEMENT": "el"}}})
				return
			}
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"ELEMENT": "el"}})
		case strings.HasSuffix(p, "/rect"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": 42, "y": 348, "width": 330, "height": 52}})
		case strings.HasSuffix(p, "/text"):
			if s.textFails {
				w.WriteHeader(http.StatusInternalServerError)
				jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "stale element reference", "message": "stale"}})
				return
			}
			jsonResponse(w, map[string]interface{}{"value": "Select All 12 Topics"})
		case strings.HasSuffix(p, "/displayed"):
			jsonResponse(w, map[string]interface{}{"value": true})
		default:
			jsonResponse(w, map[string]interface{}{"status": 0})
		}
	}))
}

// With the switch on, text and id lookups read the page source and get the
// corrected frame; WDA's own queries, whose rect is not corrected, are not
// asked. Without it they are, and the frame is WDA's.
func TestHostedFrames_LookupsReadThePageSource(t *testing.T) {
	text := flow.Selector{Text: "Select All 12 Topics"}
	id := flow.Selector{ID: "AllCategoryButton"}
	for _, tc := range []struct {
		name string
		find func(d *Driver) (*core.ElementInfo, error)
	}{
		{"text", func(d *Driver) (*core.ElementInfo, error) { return d.findElement(text, false, 2000) }},
		{"id", func(d *Driver) (*core.ElementInfo, error) { return d.findElement(id, false, 2000) }},
		{"text for a tap", func(d *Driver) (*core.ElementInfo, error) { return d.findElementForTap(text, false, 2000) }},
		{"id, once", func(d *Driver) (*core.ElementInfo, error) { return d.findElementOnce(id) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
			s := &hostedLookupServer{}
			server := s.start(t)
			defer server.Close()
			info, err := tc.find(createTestDriver(server))
			if err != nil {
				t.Fatalf("lookup: %v", err)
			}
			if info.Bounds.Y != 396 {
				t.Errorf("y=%d, want 396 (348 corrected by the sheet's 48)", info.Bounds.Y)
			}
			if s.queries != 0 {
				t.Errorf("asked WDA %d element queries; with the switch on only the page source counts", s.queries)
			}
		})
	}

	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "")
	s := &hostedLookupServer{}
	server := s.start(t)
	defer server.Close()
	info, err := createTestDriver(server).findElement(text, false, 2000)
	if err != nil || info.Bounds.Y != 348 || s.queries == 0 {
		t.Fatalf("without the switch: info %+v, err %v, %d queries; want WDA's y=348 from a WDA query", info, err, s.queries)
	}
}

// copyTextFrom on a hosted sheet copies the element's text with the switch on,
// even when WDA's text read fails: the page source has it. Through WDA, the
// fallback compared the corrected page-source frame with WDA's uncorrected
// rect and failed.
func TestHostedFrames_CopyTextFrom(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	s := &hostedLookupServer{textFails: true}
	server := s.start(t)
	defer server.Close()
	res := createTestDriver(server).copyTextFrom(&flow.CopyTextFromStep{Selector: flow.Selector{Text: "Select All 12 Topics"}})
	if !res.Success || res.Data != "Select All 12 Topics" {
		t.Fatalf("copied %q (success=%v): %s", res.Data, res.Success, res.Message)
	}
	if s.queries != 0 {
		t.Errorf("asked WDA %d element queries; with the switch on only the page source counts", s.queries)
	}
}

// A failing page source does not spin: the tap lookup pauses between reads
// and reports why the page source failed.
func TestHostedFrames_TapLookupPausesAndKeepsTheError(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	var reads int
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			mu.Lock()
			reads++
			mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "unknown error", "message": "kAXErrorIllegalArgument"}})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	_, err := createTestDriver(server).findElementForTap(flow.Selector{Text: "Allow"}, false, 500)
	if err == nil {
		t.Fatal("found an element with no page source")
	}
	if !strings.Contains(err.Error(), "kAXErrorIllegalArgument") {
		t.Errorf("err = %v; want the page source's own error", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if reads > 50 {
		t.Errorf("read the page source %d times in 500 ms; want a pause between reads", reads)
	}
}

// scrollUntilVisible judges the element by the same corrected frame: with the
// switch on it never takes WDA's rect.
func TestHostedFrames_ScrollUntilVisible(t *testing.T) {
	t.Setenv("MAESTRO_WDA_HOSTED_FRAMES", "1")
	s := &hostedLookupServer{}
	server := s.start(t)
	defer server.Close()
	d := createTestDriver(server)
	d.info.ScreenWidth, d.info.ScreenHeight = 414, 896 // the fixture's iPhone 11
	res := d.scrollUntilVisible(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "Select All 12 Topics"},
		Direction: "DOWN",
	})
	if !res.Success {
		t.Fatalf("scrollUntilVisible: %s", res.Message)
	}
	if s.queries != 0 {
		t.Errorf("asked WDA %d element queries; with the switch on only the page source counts", s.queries)
	}
	if res.Element == nil || res.Element.Bounds.Y != 396 {
		t.Errorf("element %+v, want the corrected y=396", res.Element)
	}
}
