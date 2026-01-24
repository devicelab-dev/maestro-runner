package uiautomator2

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestClick(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/gestures/click") {
			t.Errorf("expected /appium/gestures/click, got %s", r.URL.Path)
		}

		var req ClickRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Offset == nil || req.Offset.X != 100 || req.Offset.Y != 200 {
			t.Errorf("unexpected offset: %+v", req.Offset)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.Click(100, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClickElement(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		var req ClickRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Origin == nil || req.Origin.ELEMENT != "elem-123" {
			t.Errorf("unexpected origin: %+v", req.Origin)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.ClickElement("elem-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLongClick(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/gestures/long_click") {
			t.Errorf("expected /appium/gestures/long_click, got %s", r.URL.Path)
		}

		var req LongClickRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Offset.X != 100 || req.Offset.Y != 200 || req.Duration != 1000 {
			t.Errorf("unexpected request: %+v", req)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.LongClick(100, 200, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLongClickElement(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		var req LongClickRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Origin.ELEMENT != "elem-123" || req.Duration != 500 {
			t.Errorf("unexpected request: %+v", req)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.LongClickElement("elem-123", 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoubleClick(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/gestures/double_click") {
			t.Errorf("expected /appium/gestures/double_click, got %s", r.URL.Path)
		}

		var req ClickRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Offset.X != 150 || req.Offset.Y != 250 {
			t.Errorf("unexpected offset: %+v", req.Offset)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.DoubleClick(150, 250)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDoubleClickElement(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		var req ClickRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Origin.ELEMENT != "elem-456" {
			t.Errorf("unexpected origin: %+v", req.Origin)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.DoubleClickElement("elem-456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSwipe(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/gestures/swipe") {
			t.Errorf("expected /appium/gestures/swipe, got %s", r.URL.Path)
		}

		var req SwipeRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Origin.ELEMENT != "elem-123" {
			t.Errorf("unexpected origin: %+v", req.Origin)
		}
		if req.Direction != "up" {
			t.Errorf("expected direction up, got %s", req.Direction)
		}
		if req.Percent != 0.5 {
			t.Errorf("expected percent 0.5, got %f", req.Percent)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.Swipe("elem-123", "up", 0.5, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSwipeInArea(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		var req SwipeRequest
		json.NewDecoder(r.Body).Decode(&req)
		// NewRect(0, 100, 500, 800) creates Left=0, Top=100, Width=500, Height=800
		if req.Area.Left != 0 || req.Area.Top != 100 || req.Area.Width != 500 || req.Area.Height != 800 {
			t.Errorf("unexpected area: %+v", req.Area)
		}
		if req.Direction != "down" {
			t.Errorf("expected direction down, got %s", req.Direction)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	area := NewRect(0, 100, 500, 800)
	err := client.SwipeInArea(area, "down", 0.8, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScroll(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/gestures/scroll") {
			t.Errorf("expected /appium/gestures/scroll, got %s", r.URL.Path)
		}

		var req ScrollRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Origin.ELEMENT != "scroll-view" {
			t.Errorf("unexpected origin: %+v", req.Origin)
		}
		if req.Direction != "down" || req.Percent != 0.3 {
			t.Errorf("unexpected request: %+v", req)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.Scroll("scroll-view", "down", 0.3, 800)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScrollInArea(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		var req ScrollRequest
		json.NewDecoder(r.Body).Decode(&req)
		// NewRect(10, 20, 100, 200) creates Left=10, Top=20, Width=100, Height=200
		if req.Area.Left != 10 || req.Area.Top != 20 || req.Area.Width != 100 || req.Area.Height != 200 {
			t.Errorf("unexpected area: %+v", req.Area)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	area := NewRect(10, 20, 100, 200)
	err := client.ScrollInArea(area, "up", 0.5, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDrag(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/gestures/drag") {
			t.Errorf("expected /appium/gestures/drag, got %s", r.URL.Path)
		}

		var req DragRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Origin.ELEMENT != "drag-elem" {
			t.Errorf("unexpected origin: %+v", req.Origin)
		}
		if req.EndX != 300 || req.EndY != 400 {
			t.Errorf("unexpected end: %d, %d", req.EndX, req.EndY)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.Drag("drag-elem", 300, 400, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPinchOpen(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/gestures/pinch_open") {
			t.Errorf("expected /appium/gestures/pinch_open, got %s", r.URL.Path)
		}

		var req PinchRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Origin.ELEMENT != "pinch-elem" {
			t.Errorf("unexpected origin: %+v", req.Origin)
		}
		if req.Percent != 0.75 {
			t.Errorf("expected percent 0.75, got %f", req.Percent)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.PinchOpen("pinch-elem", 0.75, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPinchClose(t *testing.T) {
	client, server := newTestClientWithSession(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/appium/gestures/pinch_close") {
			t.Errorf("expected /appium/gestures/pinch_close, got %s", r.URL.Path)
		}

		var req PinchRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Origin.ELEMENT != "zoom-out" {
			t.Errorf("unexpected origin: %+v", req.Origin)
		}
		if req.Percent != 0.25 {
			t.Errorf("expected percent 0.25, got %f", req.Percent)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.PinchClose("zoom-out", 0.25, 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
