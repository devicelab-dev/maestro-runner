package wda

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestTapOnStaleElementRecovery(t *testing.T) {
	for _, present := range []bool{true, false} {
		name := "element disappeared"
		if present {
			name = "element remains in page source"
		}
		t.Run(name, func(t *testing.T) {
			var reads atomic.Int32
			taps := make(chan [2]float64, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/element"):
					jsonResponse(w, map[string]any{"value": map[string]string{"ELEMENT": "stale"}})
				case strings.Contains(r.URL.Path, "/element/stale/"):
					w.WriteHeader(http.StatusNotFound)
					jsonResponse(w, map[string]any{"value": map[string]string{
						"error": "stale element reference", "message": "element is no longer attached",
					}})
				case strings.HasSuffix(r.URL.Path, "/source"):
					reads.Add(1)
					source := `<AppiumAUT/>`
					if present {
						source = `<AppiumAUT><XCUIElementTypeButton type="XCUIElementTypeButton" label="Cancel" enabled="true" visible="true" x="20" y="60" width="44" height="44"/></AppiumAUT>`
					}
					jsonResponse(w, map[string]any{"value": source})
				case strings.HasSuffix(r.URL.Path, "/wda/tap"):
					var point struct{ X, Y float64 }
					if err := json.NewDecoder(r.Body).Decode(&point); err != nil {
						t.Error(err)
					}
					taps <- [2]float64{point.X, point.Y}
					jsonResponse(w, map[string]any{"value": nil})
				case strings.HasSuffix(r.URL.Path, "/window/size"):
					jsonResponse(w, map[string]any{"value": map[string]int{"width": 390, "height": 844}})
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			result := createTestDriver(server).tapOn(&flow.TapOnStep{
				Selector: flow.Selector{Text: "Cancel"},
				BaseStep: flow.BaseStep{TimeoutMs: 100},
			})
			if result.Success != present {
				t.Fatalf("success = %v, want %v: %s", result.Success, present, result.Message)
			}
			if reads.Load() == 0 {
				t.Error("stale element must be resolved from a fresh page source")
			}
			select {
			case point := <-taps:
				if !present || point != [2]float64{42, 82} {
					t.Errorf("unexpected tap at %v", point)
				}
			default:
				if present {
					t.Error("expected a tap on the recovered element")
				}
			}
		})
	}
}

func TestGetElementInfoRejectsInvalidBounds(t *testing.T) {
	for _, bounds := range []struct {
		name          string
		width, height int
	}{
		{"zero width", 0, 44},
		{"zero height", 44, 0},
		{"negative width", -1, 44},
		{"negative height", 44, -1},
	} {
		t.Run(bounds.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/rect"):
					jsonResponse(w, map[string]any{"value": map[string]int{
						"x": 20, "y": 60, "width": bounds.width, "height": bounds.height,
					}})
				case strings.HasSuffix(r.URL.Path, "/displayed"):
					jsonResponse(w, map[string]any{"value": true})
				default:
					jsonResponse(w, map[string]any{"value": "Cancel"})
				}
			}))
			defer server.Close()
			if _, err := createTestDriver(server).getElementInfo("cancel"); err == nil {
				t.Error("expected invalid element bounds to be rejected")
			}
		})
	}
}
