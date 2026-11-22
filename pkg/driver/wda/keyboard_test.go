package wda

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// keyboardScreen is a fake WDA with a software keyboard (390x844 screen, the
// keyboard's top at 500 while it is up). It hides after hideAfter flicks, or
// never with hideAfter 0; nil source means /source fails.
type keyboardScreen struct {
	mu        sync.Mutex
	up        bool
	hideAfter int
	source    *string
	// screen, when set, is the page source with the keyboard up or not.
	screen  func(up bool) string
	queries int
	flicks  [][4]float64
	taps    [][2]float64
	keys    []string
}

func (k *keyboardScreen) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k.mu.Lock()
		defer k.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/element"):
			w.WriteHeader(http.StatusNotFound)
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "no such element", "message": "not found"}})
		case strings.HasSuffix(path, "/elements"):
			if strings.Contains(string(body), "XCUIElementTypeKeyboard") {
				k.queries++
			}
			var found []interface{}
			if k.up && strings.Contains(string(body), "XCUIElementTypeKeyboard") {
				found = append(found, map[string]interface{}{"ELEMENT": "kb"})
			}
			jsonResponse(w, map[string]interface{}{"value": found})
		case strings.HasSuffix(path, "/element/kb/rect"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": 0, "y": 500, "width": 390, "height": 344}})
		case strings.HasSuffix(path, "/source"):
			if k.screen != nil {
				jsonResponse(w, map[string]interface{}{"value": k.screen(k.up)})
				return
			}
			if k.source == nil {
				w.WriteHeader(http.StatusInternalServerError)
				jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "unknown error", "message": "source failed"}})
				return
			}
			jsonResponse(w, map[string]interface{}{"value": *k.source})
		case strings.HasSuffix(path, "/actions"):
			var req struct {
				Actions []struct {
					Actions []map[string]interface{} `json:"actions"`
				} `json:"actions"`
			}
			_ = json.Unmarshal(body, &req)
			a := req.Actions[0].Actions
			k.flicks = append(k.flicks, [4]float64{a[0]["x"].(float64), a[0]["y"].(float64), a[2]["x"].(float64), a[2]["y"].(float64)})
			if k.hideAfter > 0 && len(k.flicks) >= k.hideAfter {
				k.up = false
			}
			jsonResponse(w, map[string]interface{}{"value": nil})
		case strings.HasSuffix(path, "/wda/tap"):
			var p struct{ X, Y float64 }
			_ = json.Unmarshal(body, &p)
			k.taps = append(k.taps, [2]float64{p.X, p.Y})
			jsonResponse(w, map[string]interface{}{"value": nil})
		case strings.HasSuffix(path, "/wda/keys"):
			k.keys = append(k.keys, string(body))
			jsonResponse(w, map[string]interface{}{"value": nil})
		default:
			jsonResponse(w, map[string]interface{}{"value": nil})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// screenXML is a 390x844 page source holding body inside a window, and the
// keyboard at its top of 500.
func screenXML(body string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><AppiumAUT>
<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="app" label="app" enabled="true" visible="true" x="0" y="0" width="390" height="844">
<XCUIElementTypeWindow type="XCUIElementTypeWindow" enabled="true" visible="true" x="0" y="0" width="390" height="844">%s
<XCUIElementTypeKeyboard type="XCUIElementTypeKeyboard" enabled="true" visible="true" x="0" y="500" width="390" height="344">
<XCUIElementTypeKey type="XCUIElementTypeKey" label="q" enabled="true" visible="true" x="3" y="510" width="35" height="42"/>
</XCUIElementTypeKeyboard>
</XCUIElementTypeWindow></XCUIElementTypeApplication></AppiumAUT>`, body)
}

func shortKeyboardPause(t *testing.T) {
	t.Helper()
	old := keyboardDragPause
	keyboardDragPause = time.Millisecond
	t.Cleanup(func() { keyboardDragPause = old })
}

// With no keyboard up, hideKeyboard touches nothing: return would submit a
// focused field's form, and a drag could press a control.
func TestHideKeyboardNoKeyboardDoesNothing(t *testing.T) {
	src := screenXML("")
	k := &keyboardScreen{source: &src}
	d := createTestDriver(k.serve(t))
	result := d.hideKeyboard(&flow.HideKeyboardStep{})
	if !result.Success || result.Message != "No keyboard to hide" {
		t.Fatalf("result = %v %q", result.Success, result.Message)
	}
	if len(k.flicks) != 0 || len(k.keys) != 0 {
		t.Errorf("flicks %v keys %v, want none", k.flicks, k.keys)
	}
}

// hideKeyboard drags instead of pressing return, as Maestro does: up 3% of
// the screen in 50 ms, and stops once the keyboard is gone.
func TestHideKeyboardDragsUpAndStops(t *testing.T) {
	shortKeyboardPause(t)
	src := screenXML("")
	k := &keyboardScreen{up: true, hideAfter: 1, source: &src}
	d := createTestDriver(k.serve(t))
	result := d.hideKeyboard(&flow.HideKeyboardStep{})
	if !result.Success || result.Message != "Keyboard hidden" {
		t.Fatalf("result = %v %q", result.Success, result.Message)
	}
	if len(k.keys) != 0 {
		t.Errorf("keys sent: %v", k.keys)
	}
	if len(k.flicks) != 1 {
		t.Fatalf("flicks = %v, want 1", k.flicks)
	}
	f := k.flicks[0]
	if f[0] != 195 || f[1] != 422 || f[2] != 195 || f[3] >= f[1] {
		t.Errorf("flick %v, want an upward drag from the centre (195,422)", f)
	}
	if dy := f[1] - f[3]; dy < 25 || dy > 26 {
		t.Errorf("drag length %.1f, want 3%% of 844", dy)
	}
}

// The second drag goes left, and a keyboard that stays up leaves the step
// passing, as in Maestro.
func TestHideKeyboardSecondDragGoesLeft(t *testing.T) {
	shortKeyboardPause(t)
	src := screenXML("")
	k := &keyboardScreen{up: true, source: &src}
	d := createTestDriver(k.serve(t))
	result := d.hideKeyboard(&flow.HideKeyboardStep{})
	if !result.Success || !strings.Contains(result.Message, "still visible") {
		t.Fatalf("result = %v %q", result.Success, result.Message)
	}
	if len(k.flicks) != 2 {
		t.Fatalf("flicks = %v, want 2", k.flicks)
	}
	f := k.flicks[1]
	if f[1] != f[3] || f[2] >= f[0] {
		t.Errorf("second flick %v, want a leftward drag", f)
	}
}

// The drag starts clear of a control at the centre of the screen: a
// checkbox there took the drag as a tap.
func TestHideKeyboardDragStartsClearOfControls(t *testing.T) {
	shortKeyboardPause(t)
	src := screenXML(`<XCUIElementTypeSwitch type="XCUIElementTypeSwitch" label="Remember me" enabled="true" visible="true" x="20" y="400" width="350" height="44"/>`)
	k := &keyboardScreen{up: true, hideAfter: 1, source: &src}
	d := createTestDriver(k.serve(t))
	d.hideKeyboard(&flow.HideKeyboardStep{})
	if len(k.flicks) != 1 {
		t.Fatalf("flicks = %v", k.flicks)
	}
	x, y := k.flicks[0][0], k.flicks[0][1]
	if x >= 12 && x <= 378 && y >= 392 && y <= 452 {
		t.Errorf("drag starts at (%.0f,%.0f), on the switch", x, y)
	}
	if y >= 492 {
		t.Errorf("drag starts at (%.0f,%.0f), on the keyboard", x, y)
	}
}

// With controls everywhere above the keyboard and no scroll view, there is
// nowhere safe to drag: the keyboard stays and nothing is pressed.
func TestHideKeyboardNoSafeStart(t *testing.T) {
	shortKeyboardPause(t)
	src := screenXML(`<XCUIElementTypeButton type="XCUIElementTypeButton" label="Big" enabled="true" visible="true" x="0" y="0" width="390" height="500"/>`)
	k := &keyboardScreen{up: true, hideAfter: 1, source: &src}
	d := createTestDriver(k.serve(t))
	result := d.hideKeyboard(&flow.HideKeyboardStep{})
	if !result.Success || len(k.flicks) != 0 || len(k.keys) != 0 {
		t.Errorf("result %v %q, flicks %v keys %v", result.Success, result.Message, k.flicks, k.keys)
	}
}

// A page source that cannot be read leaves Maestro's start, the centre.
func TestHideKeyboardUnreadableScreenUsesCentre(t *testing.T) {
	shortKeyboardPause(t)
	k := &keyboardScreen{up: true, hideAfter: 1}
	d := createTestDriver(k.serve(t))
	d.hideKeyboard(&flow.HideKeyboardStep{})
	if len(k.flicks) != 1 || k.flicks[0][0] != 195 || k.flicks[0][1] != 422 {
		t.Errorf("flicks = %v, want one from the centre", k.flicks)
	}
}

// pageXML is a 390x844 page source holding body, with the keyboard (top 500)
// when up.
func pageXML(body string, up bool) string {
	if up {
		return screenXML(body)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><AppiumAUT>
<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="app" label="app" enabled="true" visible="true" x="0" y="0" width="390" height="844">
<XCUIElementTypeWindow type="XCUIElementTypeWindow" enabled="true" visible="true" x="0" y="0" width="390" height="844">%s
</XCUIElementTypeWindow></XCUIElementTypeApplication></AppiumAUT>`, body)
}

func button(id string, y, h int) string {
	return fmt.Sprintf(`<XCUIElementTypeButton type="XCUIElementTypeButton" name="%s" label="%s" enabled="true" visible="true" x="20" y="%d" width="350" height="%d"/>`, id, id, y, h)
}

func tapSubmit(d *Driver) *core.CommandResult {
	return d.tapOn(&flow.TapOnStep{Selector: flow.Selector{ID: "submit"}})
}

// Before anything is typed, a tap does not look for the keyboard at all.
func TestTapNoKeyboardCheckBeforeTyping(t *testing.T) {
	k := &keyboardScreen{up: true, screen: func(up bool) string { return pageXML(button("submit", 700, 50), up) }}
	d := createTestDriver(k.serve(t))
	if r := tapSubmit(d); !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	if k.queries != 0 {
		t.Errorf("keyboard queried %d times, want 0", k.queries)
	}
	if len(k.taps) != 1 || k.taps[0] != [2]float64{195, 725} {
		t.Errorf("taps = %v", k.taps)
	}
}

// After typing, a tap on a button under the keyboard hides the keyboard
// first and taps the button, not a key.
func TestTapUnderKeyboardHidesItFirst(t *testing.T) {
	shortKeyboardPause(t)
	k := &keyboardScreen{up: true, hideAfter: 1, screen: func(up bool) string { return pageXML(button("submit", 700, 50), up) }}
	d := createTestDriver(k.serve(t))
	d.keyboardMayBeUp = true
	r := tapSubmit(d)
	if !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	if len(k.flicks) != 1 || len(k.keys) != 0 {
		t.Errorf("flicks %v keys %v, want one dismiss drag and no keys", k.flicks, k.keys)
	}
	if len(k.taps) != 1 || k.taps[0] != [2]float64{195, 725} {
		t.Errorf("taps = %v, want the button's centre", k.taps)
	}
	if d.keyboardMayBeUp {
		t.Error("keyboardMayBeUp still set after the keyboard went")
	}
}

// A keyboard that stays up: the tap goes to the part of the button above it.
func TestTapUnderKeyboardAimsAboveIt(t *testing.T) {
	shortKeyboardPause(t)
	k := &keyboardScreen{up: true, screen: func(up bool) string { return pageXML(button("submit", 460, 100), up) }}
	d := createTestDriver(k.serve(t))
	d.keyboardMayBeUp = true
	if r := tapSubmit(d); !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	if len(k.taps) != 1 || k.taps[0] != [2]float64{195, 480} {
		t.Errorf("taps = %v, want (195,480), above the keyboard top", k.taps)
	}
}

// A keyboard that stays up over all of the button: the step fails and
// nothing is tapped.
func TestTapUnderKeyboardFailsWhenCovered(t *testing.T) {
	shortKeyboardPause(t)
	k := &keyboardScreen{up: true, screen: func(up bool) string { return pageXML(button("submit", 700, 50), up) }}
	d := createTestDriver(k.serve(t))
	d.keyboardMayBeUp = true
	r := tapSubmit(d)
	if r.Success || !errors.Is(r.Error, core.ErrUnderKeyboard) {
		t.Fatalf("result %v %v, want ErrUnderKeyboard", r.Success, r.Error)
	}
	if !strings.Contains(r.Message, "under the keyboard") {
		t.Errorf("message %q", r.Message)
	}
	if len(k.taps) != 0 {
		t.Errorf("taps = %v, want none", k.taps)
	}
}

// The keyboard's own return button is tapped where it is.
func TestTapKeyboardButtonIsNotHidden(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?><AppiumAUT>
<XCUIElementTypeApplication type="XCUIElementTypeApplication" name="app" label="app" enabled="true" visible="true" x="0" y="0" width="390" height="844">
<XCUIElementTypeWindow type="XCUIElementTypeWindow" enabled="true" visible="true" x="0" y="0" width="390" height="844">
<XCUIElementTypeKeyboard type="XCUIElementTypeKeyboard" enabled="true" visible="true" x="0" y="500" width="390" height="344">
<XCUIElementTypeButton type="XCUIElementTypeButton" name="submit" label="Done" enabled="true" visible="true" x="290" y="760" width="96" height="42"/>
</XCUIElementTypeKeyboard>
</XCUIElementTypeWindow></XCUIElementTypeApplication></AppiumAUT>`
	k := &keyboardScreen{up: true, screen: func(bool) string { return src }}
	d := createTestDriver(k.serve(t))
	d.keyboardMayBeUp = true
	if r := tapSubmit(d); !r.Success {
		t.Fatalf("tap failed: %s", r.Message)
	}
	if len(k.flicks) != 0 || len(k.taps) != 1 || k.taps[0] != [2]float64{338, 781} {
		t.Errorf("flicks %v taps %v, want the return button tapped in place", k.flicks, k.taps)
	}
}

// Once a check finds no keyboard, later taps stop asking.
func TestTapKeyboardCheckClearsWhenGone(t *testing.T) {
	k := &keyboardScreen{screen: func(up bool) string { return pageXML(button("submit", 700, 50), up) }}
	d := createTestDriver(k.serve(t))
	d.keyboardMayBeUp = true
	tapSubmit(d)
	tapSubmit(d)
	if k.queries != 1 || d.keyboardMayBeUp {
		t.Errorf("queries %d, keyboardMayBeUp %v; want one check, then none", k.queries, d.keyboardMayBeUp)
	}
}
