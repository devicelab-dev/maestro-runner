package appium

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// capturedActions decodes the pointer action sequence from a W3C /actions body.
func capturedActions(t *testing.T, body []byte) []map[string]interface{} {
	t.Helper()
	var payload struct {
		Actions []struct {
			Actions []map[string]interface{} `json:"actions"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("bad /actions payload: %v", err)
	}
	if len(payload.Actions) != 1 {
		t.Fatalf("expected one pointer sequence, got %d", len(payload.Actions))
	}
	return payload.Actions[0].Actions
}

func TestClientDragAndDrop_ActionsPayload(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/actions") {
			body, _ = io.ReadAll(r.Body)
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	d := createTestAppiumDriver(server)
	if err := d.client.DragAndDrop(100, 200, 300, 400, 800, 1000); err != nil {
		t.Fatal(err)
	}

	actions := capturedActions(t, body)

	// press at the source, hold, 20 moves, settle, release
	if actions[0]["type"] != "pointerMove" || actions[0]["x"].(float64) != 100 || actions[0]["y"].(float64) != 200 {
		t.Errorf("first action should move to the source, got %v", actions[0])
	}
	if actions[1]["type"] != "pointerDown" {
		t.Errorf("second action should press, got %v", actions[1])
	}
	if actions[2]["type"] != "pause" || actions[2]["duration"].(float64) != 800 {
		t.Errorf("third action should hold for 800ms, got %v", actions[2])
	}

	last := actions[len(actions)-1]
	if last["type"] != "pointerUp" {
		t.Errorf("last action should release, got %v", last)
	}
	settle := actions[len(actions)-2]
	if settle["type"] != "pause" || settle["duration"].(float64) != 250 {
		t.Errorf("release should be preceded by a 250ms settle, got %v", settle)
	}

	moves := actions[3 : len(actions)-2]
	if len(moves) != 20 {
		t.Fatalf("expected 20 interpolated moves, got %d", len(moves))
	}
	final := moves[len(moves)-1]
	if final["x"].(float64) != 300 || final["y"].(float64) != 400 {
		t.Errorf("final move should land on the target, got %v", final)
	}
	if moves[0]["x"].(float64) == 300 {
		t.Error("moves should be interpolated, not a single jump to the target")
	}
}

func TestDragAndDrop_ByPoints(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/actions") {
			body, _ = io.ReadAll(r.Body)
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	d := createTestAppiumDriver(server)
	step := &flow.DragAndDropStep{
		From:         flow.Selector{Point: "50%, 50%"},
		To:           flow.Selector{Point: "50%, 10%"},
		HoldDuration: 500,
		Duration:     600,
	}

	result := d.dragAndDrop(step)
	if !result.Success {
		t.Fatalf("expected success, got: %v - %s", result.Error, result.Message)
	}

	actions := capturedActions(t, body)
	// screen is 1080x2340 in the test driver
	if actions[0]["x"].(float64) != 540 || actions[0]["y"].(float64) != 1170 {
		t.Errorf("source should resolve to (540, 1170), got %v", actions[0])
	}
}

func TestDragAndDrop_MissingElementFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			writeJSON(w, map[string]interface{}{"value": countPageSourceXML})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]interface{}{"value": map[string]interface{}{"error": "no such element"}})
	}))
	defer server.Close()

	d := createTestAppiumDriver(server)
	step := &flow.DragAndDropStep{
		From:         flow.Selector{ID: "does-not-exist"},
		To:           flow.Selector{Point: "50%, 10%"},
		HoldDuration: 100,
		Duration:     100,
	}
	step.TimeoutMs = 300

	if result := d.dragAndDrop(step); result.Success {
		t.Fatal("expected failure for a missing drag source")
	}
}

// scrollTestPageSource places the target half below the 2340px-high screen:
// bounds [0,2240][1080,2440] → 50% visible.
const scrollTestPageSourceXML = `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2340]" class="android.widget.FrameLayout" enabled="true" displayed="true">
    <android.widget.TextView text="Target" resource-id="com.example:id/target" bounds="[0,2240][1080,2440]" enabled="true" displayed="true" />
  </android.widget.FrameLayout>
</hierarchy>`

func newScrollTestDriver() (*Driver, func()) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			writeJSON(w, map[string]interface{}{"value": scrollTestPageSourceXML})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	return createTestAppiumDriver(server), server.Close
}

func TestScrollUntilVisible_RejectsPartiallyVisible(t *testing.T) {
	d, closeFn := newScrollTestDriver()
	defer closeFn()

	step := &flow.ScrollUntilVisibleStep{
		Element:    flow.Selector{Text: "Target"},
		MaxScrolls: 2,
	}
	step.TimeoutMs = 3000

	result := d.scrollUntilVisible(step)
	if result.Success {
		t.Fatal("a half-off-screen element must not satisfy the default fully-visible requirement")
	}
	if !strings.Contains(result.Error.Error(), "sufficiently visible") {
		t.Errorf("error should say the element stayed partially visible, got: %v", result.Error)
	}
}

func TestScrollUntilVisible_HonorsVisibilityPercentage(t *testing.T) {
	d, closeFn := newScrollTestDriver()
	defer closeFn()

	step := &flow.ScrollUntilVisibleStep{
		Element:              flow.Selector{Text: "Target"},
		MaxScrolls:           2,
		VisibilityPercentage: 50,
	}
	step.TimeoutMs = 3000

	result := d.scrollUntilVisible(step)
	if !result.Success {
		t.Fatalf("50%% visible must satisfy visibilityPercentage: 50, got: %v - %s", result.Error, result.Message)
	}
}

// Container-clipped sliver (#164). Android reports a child's bounds already
// clipped to its scroll container, so a 9px sliver of a 126px row at the
// container's bottom edge arrives as a 9px rect wholly on screen and scores
// 100% visible. The first source below is that state; after one swipe the
// server serves the second, where the row has scrolled fully into view.
const clippedSliverPageSourceXML = `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2340]" class="android.widget.FrameLayout" enabled="true" displayed="true">
    <androidx.recyclerview.widget.RecyclerView scrollable="true" bounds="[0,600][1080,2274]" enabled="true" displayed="true">
      <android.widget.TextView text="Target" bounds="[0,2265][1080,2274]" enabled="true" displayed="true" />
    </androidx.recyclerview.widget.RecyclerView>
  </android.widget.FrameLayout>
</hierarchy>`

const revealedRowPageSourceXML = `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,2340]" class="android.widget.FrameLayout" enabled="true" displayed="true">
    <androidx.recyclerview.widget.RecyclerView scrollable="true" bounds="[0,600][1080,2274]" enabled="true" displayed="true">
      <android.widget.TextView text="Target" bounds="[0,1900][1080,2026]" enabled="true" displayed="true" />
    </androidx.recyclerview.widget.RecyclerView>
  </android.widget.FrameLayout>
</hierarchy>`

// newFlushRowTestDriver serves `before` until the first swipe, then `after`,
// and reports how many swipes were performed.
func newFlushRowTestDriver(before, after string) (*Driver, *int, func()) {
	swipes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			src := before
			if swipes > 0 {
				src = after
			}
			writeJSON(w, map[string]interface{}{"value": src})
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions") {
			swipes++
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	return createTestAppiumDriver(server), &swipes, server.Close
}

func TestScrollUntilVisible_ContainerClippedSliverGetsOneMoreScroll(t *testing.T) {
	d, swipes, closeFn := newFlushRowTestDriver(clippedSliverPageSourceXML, revealedRowPageSourceXML)
	defer closeFn()

	step := &flow.ScrollUntilVisibleStep{
		Element:    flow.Selector{Text: "Target"},
		MaxScrolls: 3,
	}
	step.TimeoutMs = 5000

	result := d.scrollUntilVisible(step)
	if !result.Success {
		t.Fatalf("expected the revealed row to be accepted, got: %v - %s", result.Error, result.Message)
	}
	if *swipes != 1 {
		t.Errorf("the flush sliver should have earned exactly one confirming scroll, got %d", *swipes)
	}
	if result.Element == nil || result.Element.Bounds.Height != 126 {
		t.Errorf("result should carry the fully revealed row (height 126), got %+v", result.Element)
	}
}

// A row resting at the end of a fully scrolled list is flush too. It gets its
// one confirming scroll, does not grow, and is then accepted — the scroll must
// not loop to the cap on a genuinely visible element.
func TestScrollUntilVisible_FlushRowAtListEndIsAcceptedAfterOneScroll(t *testing.T) {
	d, swipes, closeFn := newFlushRowTestDriver(clippedSliverPageSourceXML, clippedSliverPageSourceXML)
	defer closeFn()

	step := &flow.ScrollUntilVisibleStep{
		Element:    flow.Selector{Text: "Target"},
		MaxScrolls: 5,
	}
	step.TimeoutMs = 5000

	result := d.scrollUntilVisible(step)
	if !result.Success {
		t.Fatalf("an unchanged flush row must be accepted, got: %v - %s", result.Error, result.Message)
	}
	if *swipes != 1 {
		t.Errorf("expected exactly one confirming scroll, got %d", *swipes)
	}
}

func TestParsePageSource_ReadsScrollable(t *testing.T) {
	elements, _, err := ParsePageSource(clippedSliverPageSourceXML)
	if err != nil {
		t.Fatal(err)
	}
	var list, target *ParsedElement
	for _, e := range elements {
		switch {
		case e.Scrollable:
			list = e
		case e.Text == "Target":
			target = e
		}
	}
	if list == nil {
		t.Fatal("scrollable=\"true\" was not parsed")
	}
	if target == nil || target.Parent != list {
		t.Fatal("target's parent should be the scrollable list")
	}
}
