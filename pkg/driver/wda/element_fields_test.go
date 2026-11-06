package wda

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// elementReadsServer is a WDA whose finds answer with fields (withFields) or
// with the id alone, and which counts the element GETs getElementInfo sends.
func elementReadsServer(t *testing.T, withFields bool) (*httptest.Server, *int) {
	t.Helper()
	var mu sync.Mutex
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		el := map[string]interface{}{"ELEMENT": "btn-1"}
		if withFields {
			el["name"] = "XCUIElementTypeButton"
			el["text"] = "Continue"
			el["rect"] = map[string]interface{}{"x": 20.0, "y": 600.0, "width": 350.0, "height": 48.0}
			el["displayed"] = true
		}
		switch {
		case r.Method == "POST" && strings.HasSuffix(p, "/elements"):
			jsonResponse(w, map[string]interface{}{"value": []interface{}{el}})
		case r.Method == "POST" && strings.HasSuffix(p, "/element"):
			jsonResponse(w, map[string]interface{}{"value": el})
		case r.Method == "GET" && strings.Contains(p, "/element/btn-1/"):
			mu.Lock()
			reads++
			mu.Unlock()
			switch {
			case strings.HasSuffix(p, "/text"):
				jsonResponse(w, map[string]interface{}{"value": "Continue"})
			case strings.HasSuffix(p, "/name"):
				jsonResponse(w, map[string]interface{}{"value": "XCUIElementTypeButton"})
			case strings.HasSuffix(p, "/displayed"):
				jsonResponse(w, map[string]interface{}{"value": true})
			case strings.HasSuffix(p, "/rect"):
				jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": 20, "y": 600, "width": 350, "height": 48}})
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			jsonResponse(w, map[string]interface{}{"status": 0})
		}
	}))
	t.Cleanup(server.Close)
	return server, &reads
}

// A find whose answer carries the element's fields needs no element GETs,
// and gives the same element as the GETs would (#258).
func TestFindAnswerFieldsSkipElementReads(t *testing.T) {
	for _, tc := range []struct {
		name string
		sel  flow.Selector
	}{
		{"text", flow.Selector{Text: "Continue"}},
		{"id", flow.Selector{ID: "continue-button"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, reads := elementReadsServer(t, true)
			d := createTestDriver(server)

			info, err := d.findElementByWDA(tc.sel)
			if err != nil {
				t.Fatalf("findElementByWDA: %v", err)
			}
			if *reads != 0 {
				t.Errorf("sent %d element GETs; the find's answer had every field", *reads)
			}
			if info.Text != "Continue" || info.Class != "XCUIElementTypeButton" || !info.Visible {
				t.Errorf("info = text %q, class %q, visible %v", info.Text, info.Class, info.Visible)
			}
			if info.Bounds.X != 20 || info.Bounds.Y != 600 || info.Bounds.Width != 350 || info.Bounds.Height != 48 {
				t.Errorf("bounds = %+v", info.Bounds)
			}
		})
	}
}

// Without the fields (a WebDriverAgent that ignored the settings) the driver
// reads them as before.
func TestFindAnswerWithoutFieldsReadsThem(t *testing.T) {
	server, reads := elementReadsServer(t, false)
	d := createTestDriver(server)

	info, err := d.findElementByWDA(flow.Selector{Text: "Continue"})
	if err != nil {
		t.Fatalf("findElementByWDA: %v", err)
	}
	if *reads != 4 {
		t.Errorf("sent %d element GETs, want the four reads", *reads)
	}
	if info.Text != "Continue" || info.Class != "XCUIElementTypeButton" || !info.Visible || info.Bounds.Width != 350 {
		t.Errorf("info = %+v", info)
	}
}

func TestElementFromAnswer(t *testing.T) {
	// W3C key only: the id is the element's, never its name or text.
	el, ok := elementFromAnswer(map[string]interface{}{
		"element-6066-11e4-a52e-4f735466cecf": "w3c-id",
		"name":                                "XCUIElementTypeStaticText",
		"text":                                nil,
		"rect":                                map[string]interface{}{"x": 0.0, "y": 10.5, "width": 100.0, "height": 20.0},
		"displayed":                           false,
	})
	if !ok || el.ID != "w3c-id" {
		t.Fatalf("id = %q, %v", el.ID, ok)
	}
	if el.Fields == nil || el.Fields.Text != "" || el.Fields.Y != 10 || el.Fields.Displayed {
		t.Errorf("fields = %+v; a null text is \"\" as GET /text answers", el.Fields)
	}

	// A field missing: no fields, so getElementInfo reads all four.
	el, _ = elementFromAnswer(map[string]interface{}{"ELEMENT": "id", "name": "XCUIElementTypeButton", "text": "Go",
		"rect": map[string]interface{}{"x": 1.0, "y": 2.0, "width": 3.0}, "displayed": true})
	if el.ID != "id" || el.Fields != nil {
		t.Errorf("a rect without height gave fields %+v", el.Fields)
	}
}
