package wda

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// dragCapture records every body posted to /wda/dragfromtoforduration.
type dragCapture struct {
	bodies []map[string]float64
}

func dragTestServer(t *testing.T, capture *dragCapture, sourceXML func() string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/dragfromtoforduration"):
			body, _ := io.ReadAll(r.Body)
			var payload map[string]float64
			_ = json.Unmarshal(body, &payload)
			capture.bodies = append(capture.bodies, payload)
			jsonResponse(w, map[string]interface{}{"status": 0})
		case strings.HasSuffix(path, "/source"):
			jsonResponse(w, map[string]interface{}{"value": sourceXML()})
		case strings.Contains(path, "/window/size"):
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
		default:
			jsonResponse(w, map[string]interface{}{"status": 0})
		}
	}))
}

func TestDragAndDrop_PointToPoint(t *testing.T) {
	capture := &dragCapture{}
	server := dragTestServer(t, capture, func() string { return "<AppiumAUT></AppiumAUT>" })
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.DragAndDropStep{
		From:         flow.Selector{Point: "50%, 80%"},
		To:           flow.Selector{Point: "50%, 20%"},
		HoldDuration: 800,
	}
	result := driver.dragAndDrop(step)
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}
	if len(capture.bodies) != 1 {
		t.Fatalf("expected 1 drag request, got %d", len(capture.bodies))
	}
	b := capture.bodies[0]
	// 390x844 screen: 50%,80% = (195, 675); 50%,20% = (195, 168)
	if b["fromX"] != 195 || b["fromY"] != 675 {
		t.Errorf("from = (%v, %v), want (195, 675)", b["fromX"], b["fromY"])
	}
	if b["toX"] != 195 || b["toY"] != 168 {
		t.Errorf("to = (%v, %v), want (195, 168)", b["toX"], b["toY"])
	}
	if b["duration"] != 0.8 {
		t.Errorf("duration = %v, want 0.8 (holdDuration in seconds)", b["duration"])
	}
}

func TestDragAndDrop_SelectorResolvesToElementCenter(t *testing.T) {
	capture := &dragCapture{}
	server := dragTestServer(t, capture, func() string {
		return `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeCell type="XCUIElementTypeCell" name="item-3" label="Item 3" enabled="true" visible="true" x="20" y="300" width="350" height="60"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`
	})
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.DragAndDropStep{
		From:         flow.Selector{ID: "item-3"},
		To:           flow.Selector{Point: "50%, 10%"},
		HoldDuration: 1000,
	}
	result := driver.dragAndDrop(step)
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}
	b := capture.bodies[len(capture.bodies)-1]
	// Element center: x = 20+350/2 = 195, y = 300+60/2 = 330
	if b["fromX"] != 195 || b["fromY"] != 330 {
		t.Errorf("from = (%v, %v), want element center (195, 330)", b["fromX"], b["fromY"])
	}
	if b["duration"] != 1.0 {
		t.Errorf("duration = %v, want 1.0", b["duration"])
	}
}

func TestDragAndDrop_MissingFromFails(t *testing.T) {
	capture := &dragCapture{}
	server := dragTestServer(t, capture, func() string { return "<AppiumAUT></AppiumAUT>" })
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.DragAndDropStep{
		From: flow.Selector{ID: "does-not-exist"},
		To:   flow.Selector{Point: "50%, 20%"},
	}
	step.TimeoutMs = 100
	result := driver.dragAndDrop(step)
	if result.Success {
		t.Fatal("expected failure when the from element cannot be found")
	}
	if len(capture.bodies) != 0 {
		t.Errorf("no drag should be attempted, got %d requests", len(capture.bodies))
	}
}

// TestScrollUntilVisible_KeepsScrollingWhenPartiallyVisible: an element half
// below the fold satisfies a bare find, but the step must keep scrolling until
// enough of it is on screen (default: fully visible).
func TestScrollUntilVisible_KeepsScrollingWhenPartiallyVisible(t *testing.T) {
	scrolls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/dragfromtoforduration"):
			scrolls++
			jsonResponse(w, map[string]interface{}{"status": 0})
		case strings.HasSuffix(path, "/source"):
			y := 800 // only 44 of 100 px on an 844-high screen
			if scrolls >= 1 {
				y = 400 // fully visible after one scroll
			}
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="target" label="Target" enabled="true" visible="true" x="50" y="` + strconv.Itoa(y) + `" width="290" height="100"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
		case strings.Contains(path, "/window/size"):
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
		default:
			jsonResponse(w, map[string]interface{}{"status": 0})
		}
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "Target"},
		Direction: "down",
	}
	result := driver.scrollUntilVisible(step)
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}
	if scrolls != 1 {
		t.Errorf("expected exactly 1 scroll past the partially-visible state, got %d", scrolls)
	}
}

// TestScrollUntilVisible_ThresholdAccepted: visibilityPercentage lowers the bar.
func TestScrollUntilVisible_ThresholdAccepted(t *testing.T) {
	scrolls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/dragfromtoforduration"):
			scrolls++
			jsonResponse(w, map[string]interface{}{"status": 0})
		case strings.HasSuffix(path, "/source"):
			// Always half-visible: y=794, height=100 → 50/100 on screen
			jsonResponse(w, map[string]interface{}{
				"value": `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="target" label="Target" enabled="true" visible="true" x="50" y="794" width="290" height="100"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`,
			})
		case strings.Contains(path, "/window/size"):
			jsonResponse(w, map[string]interface{}{
				"value": map[string]interface{}{"width": 390.0, "height": 844.0},
			})
		default:
			jsonResponse(w, map[string]interface{}{"status": 0})
		}
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.ScrollUntilVisibleStep{
		Element:              flow.Selector{Text: "Target"},
		Direction:            "down",
		VisibilityPercentage: 40,
	}
	result := driver.scrollUntilVisible(step)
	if !result.Success {
		t.Fatalf("a 50%%-visible element must satisfy visibilityPercentage: 40, got: %s", result.Message)
	}
	if scrolls != 0 {
		t.Errorf("expected 0 scrolls, got %d", scrolls)
	}
}
