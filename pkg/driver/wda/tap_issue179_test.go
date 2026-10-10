package wda

import (
	"encoding/base64"
	"encoding/json"
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

// A key-named text that labels a real element (an alert's Delete) taps the
// element; it is not sent as a backspace (#179).
func TestTapOnKeyNamedButtonTapsElement(t *testing.T) {
	var mu sync.Mutex
	var keys, taps, clicks int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		p := r.URL.Path
		switch {
		case strings.Contains(p, "/wda/keys"):
			keys++
			jsonResponse(w, map[string]interface{}{"status": 0})
		case strings.HasSuffix(p, "/wda/tap"):
			taps++
			jsonResponse(w, map[string]interface{}{"status": 0})
		case strings.HasSuffix(p, "/click"):
			clicks++
			jsonResponse(w, map[string]interface{}{"status": 0})
		case strings.HasSuffix(p, "/rect"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": 200, "y": 500, "width": 120, "height": 44}})
		case strings.HasSuffix(p, "/displayed"):
			jsonResponse(w, map[string]interface{}{"value": true})
		case strings.HasSuffix(p, "/element") && r.Method == "POST":
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "Delete") {
				jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"ELEMENT": "alert-delete"}})
				return
			}
			w.WriteHeader(http.StatusNotFound)
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "no such element"}})
		default:
			jsonResponse(w, map[string]interface{}{"status": 0})
		}
	}))
	defer server.Close()
	d := createTestDriver(server)

	res := d.tapOn(&flow.TapOnStep{Selector: flow.Selector{Text: "Delete"}})
	if !res.Success {
		t.Fatalf("tap failed: %s", res.Message)
	}
	if keys != 0 {
		t.Errorf("sent %d key presses; the Delete button should have been tapped", keys)
	}
	if taps != 1 || clicks != 0 {
		t.Errorf("taps = %d, clicks = %d; want one coordinate tap and no element click", taps, clicks)
	}
}

// Of several elements with an id, the one on screen wins over an earlier one
// that XCUITest reports not displayed (the outgoing screen of a push).
func TestIDPrefersDisplayedElement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/elements"):
			jsonResponse(w, map[string]interface{}{"value": []map[string]interface{}{{"ELEMENT": "old"}, {"ELEMENT": "new"}}})
		case strings.Contains(p, "/element/old/rect"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": -97, "y": 184, "width": 145, "height": 44}})
		case strings.Contains(p, "/element/new/rect"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": 24, "y": 184, "width": 145, "height": 44}})
		case strings.Contains(p, "/element/old/displayed"):
			jsonResponse(w, map[string]interface{}{"value": false})
		case strings.Contains(p, "/element/new/displayed"):
			jsonResponse(w, map[string]interface{}{"value": true})
		default:
			jsonResponse(w, map[string]interface{}{"status": 0})
		}
	}))
	defer server.Close()
	d := createTestDriver(server)

	info, err := d.findElementByWDA(flow.Selector{ID: "open-subfolder"})
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "new" {
		t.Errorf("picked %q, want the displayed element %q", info.ID, "new")
	}
}

// A coordinate tap goes to the centre of the visible part of the bounds and
// never to a point off the screen.
func TestTapPointClipsToScreen(t *testing.T) {
	d := &Driver{info: &core.PlatformInfo{ScreenWidth: 402, ScreenHeight: 874}}
	x, y, ok := d.tapPoint(core.Bounds{X: -97, Y: 184, Width: 145, Height: 44})
	if !ok || x != 24 || y != 206 {
		t.Errorf("partly off-screen: got (%v,%v,%v), want (24,206,true)", x, y, ok)
	}
	if _, _, ok := d.tapPoint(core.Bounds{X: -300, Y: 184, Width: 145, Height: 44}); ok {
		t.Error("fully off-screen bounds should not be tappable")
	}
	x, y, ok = d.tapPoint(core.Bounds{X: 0, Y: 100, Width: 402, Height: 2000})
	if !ok || y != 487 || x != 201 {
		t.Errorf("taller than the screen: got (%v,%v,%v), want (201,487,true)", x, y, ok)
	}
}

// settleScreen returns once two back-to-back screenshots are identical, and
// gives up at screenSettleLimit on a screen that keeps changing.
func TestSettleScreen(t *testing.T) {
	old := screenSettleLimit
	screenSettleLimit = 300 * time.Millisecond
	defer func() { screenSettleLimit = old }()
	for _, tc := range []struct {
		name      string
		shots     []string // successive screenshots; the last repeats
		wantCalls int      // 0 = until the limit
	}{
		{"still screen", []string{"a"}, 2},
		{"moves then rests", []string{"a", "b", "c", "c"}, 4},
		{"never rests", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				mu.Lock()
				i := calls
				calls++
				mu.Unlock()
				shot := fmt.Sprintf("frame-%d", i) // a new picture every time
				if tc.shots != nil {
					shot = tc.shots[min(i, len(tc.shots)-1)]
				}
				jsonResponse(w, map[string]interface{}{"value": base64.StdEncoding.EncodeToString([]byte(shot))})
			}))
			defer server.Close()
			d := createTestDriver(server)
			start := time.Now()
			if sig := d.settleScreen(); sig == "" {
				t.Error("no signature returned")
			}
			if tc.wantCalls > 0 && calls != tc.wantCalls {
				t.Errorf("screenshots = %d, want %d", calls, tc.wantCalls)
			}
			if tc.wantCalls == 0 && time.Since(start) > 2*screenSettleLimit {
				t.Errorf("did not stop at the limit: %v", time.Since(start))
			}
		})
	}
}

// A step after one that can move the screen waits for it to settle; an
// assert, or a step after one that moves nothing, does not.
func TestExecuteSettlesOnlyAfterMovingStep(t *testing.T) {
	var mu sync.Mutex
	shots := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/screenshot") {
			mu.Lock()
			shots++
			mu.Unlock()
			jsonResponse(w, map[string]interface{}{"value": base64.StdEncoding.EncodeToString([]byte("same"))})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	d := createTestDriver(server)

	d.Execute(&flow.SwipeStep{Direction: "up"})
	if shots != 0 {
		t.Fatalf("first step settled (%d screenshots); nothing moved before it", shots)
	}
	d.Execute(&flow.SetClipboardStep{Text: "x"}) // does not act on the screen
	if shots != 0 {
		t.Fatalf("a non-acting step settled (%d screenshots)", shots)
	}
	d.Execute(&flow.SwipeStep{Direction: "up"}) // previous step moved nothing
	if shots != 0 {
		t.Fatalf("settled after a step that moves nothing (%d screenshots)", shots)
	}
	d.Execute(&flow.SwipeStep{Direction: "up"}) // previous step was a swipe
	if shots != 2 {
		t.Errorf("screenshots = %d, want 2 (one settle after the swipe)", shots)
	}
}

// A swipe is a touch that moves at once — no stationary hold at the start,
// which iOS takes as a tap on the row under the finger.
func TestSwipeHasNoHoldBeforeMove(t *testing.T) {
	var body string
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		paths = append(paths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/actions") {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	d := createTestDriver(server)

	if res := d.scroll(&flow.ScrollStep{Direction: "down"}); !res.Success {
		t.Fatalf("scroll failed: %s", res.Message)
	}
	for _, p := range paths {
		if strings.Contains(p, "dragfromtoforduration") {
			t.Fatalf("scroll used %s, which holds before moving", p)
		}
	}
	var payload struct {
		Actions []struct {
			Actions []map[string]interface{} `json:"actions"`
		} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil || len(payload.Actions) != 1 {
		t.Fatalf("bad actions body %q: %v", body, err)
	}
	seq := payload.Actions[0].Actions
	var kinds []string
	for _, a := range seq {
		kinds = append(kinds, a["type"].(string))
	}
	if got := strings.Join(kinds, ","); got != "pointerMove,pointerDown,pointerMove,pause,pointerUp" {
		t.Fatalf("sequence = %s", got)
	}
	if seq[2]["duration"].(float64) != swipeMoveMs {
		t.Errorf("move duration = %v, want %d: the move must start right after the touch", seq[2]["duration"], swipeMoveMs)
	}
	// Scroll down swipes up from the centre to 10% from the top (390x844 screen).
	if seq[0]["y"].(float64) != 422 || seq[2]["y"].(float64) != 84.4 {
		t.Errorf("from y=%v to y=%v, want 422 to 84.4", seq[0]["y"], seq[2]["y"])
	}
}

// swipePayload reads a W3C /actions swipe body back into the fields the
// swipe tests check: start, end, and the rest before lift (seconds).
func swipePayload(body []byte) map[string]interface{} {
	var p struct {
		Actions []struct {
			Actions []map[string]interface{} `json:"actions"`
		} `json:"actions"`
	}
	out := map[string]interface{}{}
	if json.Unmarshal(body, &p) != nil || len(p.Actions) == 0 || len(p.Actions[0].Actions) < 5 {
		return out
	}
	seq := p.Actions[0].Actions
	out["fromX"], out["fromY"] = seq[0]["x"], seq[0]["y"]
	out["toX"], out["toY"] = seq[2]["x"], seq[2]["y"]
	if ms, ok := seq[3]["duration"].(float64); ok {
		out["duration"] = ms / 1000
	}
	return out
}

// swipeBody re-encodes a W3C /actions swipe body in the flat shape.
func swipeBody(body []byte) []byte {
	b, _ := json.Marshal(swipePayload(body))
	return b
}

// A SwiftUI Toggle is a row-wide switch holding the real switch, both with
// the same id; the inner one is the one to tap, as Maestro's deepest match.
func TestIDPrefersInnermostOfNestedMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/elements"):
			jsonResponse(w, map[string]interface{}{"value": []map[string]interface{}{{"ELEMENT": "row"}, {"ELEMENT": "switch"}}})
		case strings.Contains(p, "/element/row/rect"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": 20, "y": 160, "width": 353, "height": 44}})
		case strings.Contains(p, "/element/switch/rect"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": 296, "y": 166, "width": 57, "height": 32}})
		case strings.HasSuffix(p, "/displayed"):
			jsonResponse(w, map[string]interface{}{"value": true})
		default:
			jsonResponse(w, map[string]interface{}{"status": 0})
		}
	}))
	defer server.Close()
	d := createTestDriver(server)

	info, err := d.findElementByWDA(flow.Selector{ID: "AutoclearEnabledToggle"})
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "switch" {
		t.Errorf("picked %q, want the inner switch", info.ID)
	}
}

// When the WDA query answers with a copy that never comes on screen (first
// in tree order, parked off the right edge), scrollUntilVisible still finds
// the on-screen copy through the page source instead of scrolling past it.
func TestScrollUntilVisibleTakesOnScreenCopy(t *testing.T) {
	var mu sync.Mutex
	swipes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(p, "/actions"):
			swipes++
			jsonResponse(w, map[string]interface{}{"status": 0})
		case strings.HasSuffix(p, "/element") && r.Method == "POST":
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"ELEMENT": "hidden"}})
		case strings.Contains(p, "/element/hidden/rect"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": 414, "y": 866, "width": 102, "height": 20}})
		case strings.HasSuffix(p, "/displayed"):
			jsonResponse(w, map[string]interface{}{"value": false})
		case strings.HasSuffix(p, "/source"):
			jsonResponse(w, map[string]interface{}{"value": `<AppiumAUT><XCUIElementTypeApplication name="DDG" visible="true" x="0" y="0" width="390" height="844">
<XCUIElementTypeLink name="Terms of Service" label="Terms of Service" visible="false" x="414" y="866" width="102" height="20"/>
<XCUIElementTypeLink name="Terms of Service" label="Terms of Service" visible="true" x="216" y="78" width="115" height="21"/>
</XCUIElementTypeApplication></AppiumAUT>`})
		default:
			jsonResponse(w, map[string]interface{}{"status": 0})
		}
	}))
	defer server.Close()
	d := createTestDriver(server)

	res := d.scrollUntilVisible(&flow.ScrollUntilVisibleStep{Element: flow.Selector{Text: "Terms of Service"}, BaseStep: flow.BaseStep{TimeoutMs: 5000}})
	if !res.Success {
		t.Fatalf("scrollUntilVisible failed: %s", res.Message)
	}
	if res.Element == nil || res.Element.Bounds.X != 216 {
		t.Errorf("picked %+v, want the on-screen footer copy at x=216", res.Element)
	}
	if swipes != 0 {
		t.Errorf("swiped %d times; the on-screen copy was already there", swipes)
	}
}

// The taps of a tapOn with repeat: after the first do not settle before
// them, so they go out back to back as in Maestro.
func TestRepeatTapsDoNotSettleBetween(t *testing.T) {
	var mu sync.Mutex
	shots := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/screenshot") {
			mu.Lock()
			shots++
			mu.Unlock()
			jsonResponse(w, map[string]interface{}{"value": base64.StdEncoding.EncodeToString([]byte("same"))})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	d := createTestDriver(server)

	d.Execute(&flow.SwipeStep{Direction: "up"}) // moves the screen
	d.Execute(&flow.TapOnStep{Point: "50%,50%", RepeatIndex: 0, RepeatCount: 2})
	if shots != 2 {
		t.Fatalf("screenshots = %d, want 2 (the first tap of the repeat settles)", shots)
	}
	d.Execute(&flow.TapOnStep{Point: "50%,50%", RepeatIndex: 1, RepeatCount: 2})
	if shots != 2 {
		t.Errorf("screenshots = %d, want 2: the second tap of the repeat settled", shots)
	}
}
