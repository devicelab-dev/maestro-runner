package wda

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// holdServer is a fake WDA with one element, el1, found by id, that records
// the duration of every touchAndHold.
func holdServer(t *testing.T) (*httptest.Server, func() []float64) {
	t.Helper()
	var mu sync.Mutex
	var holds []float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/wda/touchAndHold"):
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			holds = append(holds, body["duration"].(float64))
			mu.Unlock()
			jsonResponse(w, map[string]interface{}{"value": nil})
		case strings.HasSuffix(p, "/elements"):
			jsonResponse(w, map[string]interface{}{"value": []map[string]interface{}{{"ELEMENT": "el1"}}})
		case strings.HasSuffix(p, "/rect"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": 100, "y": 400, "width": 80, "height": 40}})
		case strings.HasSuffix(p, "/displayed"):
			jsonResponse(w, map[string]interface{}{"value": true})
		default:
			jsonResponse(w, map[string]interface{}{"value": nil})
		}
	}))
	t.Cleanup(server.Close)
	return server, func() []float64 {
		mu.Lock()
		defer mu.Unlock()
		return append([]float64(nil), holds...)
	}
}

// Maestro's iOS driver holds a long press for 3 s (IOSDriver.kt:158-162);
// the runner held 1 s. A duration the flow sets still wins.
func TestLongPressHoldsThreeSecondsLikeMaestro(t *testing.T) {
	sel := flow.Selector{ID: "row"}
	for _, tc := range []struct {
		name string
		step flow.Step
		want float64
	}{
		{"longPressOn", &flow.LongPressOnStep{Selector: sel}, 3},
		{"longPressOn with a duration", &flow.LongPressOnStep{Selector: sel, DurationMs: 500}, 0.5},
		{"tapOn with longPress", &flow.TapOnStep{Selector: sel, LongPress: true}, 3},
		{"tapOnPoint with longPress", &flow.TapOnPointStep{X: 10, Y: 20, LongPress: true}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, holds := holdServer(t)
			if res := createTestDriver(server).Execute(tc.step); !res.Success {
				t.Fatalf("long press failed: %s", res.Message)
			}
			if got := holds(); len(got) != 1 || got[0] != tc.want {
				t.Errorf("held %v, want one hold of %vs", got, tc.want)
			}
		})
	}
}
