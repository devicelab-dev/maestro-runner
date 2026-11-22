package wda

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// axServerDownServer answers every page source as WebDriverAgent does when
// the app's accessibility server is not running, and counts the reads.
func axServerDownServer(t *testing.T, reads *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			atomic.AddInt32(reads, 1)
			w.WriteHeader(http.StatusNotFound)
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{
				"error": "stale element reference",
				"message": `The previously found element "Application 'app'" is not present in the current view anymore. ` +
					`Error getting main window kAXErrorServerNotFound`,
			}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "no such element"}})
	}))
}

// kAXErrorServerNotFound does not clear within a flow (#268): the lookups and
// waits stop at the first such read and say what it is, instead of polling
// page sources to their timeout.
func TestAXServerNotFoundFailsAtOnce(t *testing.T) {
	steps := map[string]func(d *Driver) (bool, string){
		"assertVisible": func(d *Driver) (bool, string) {
			r := d.Execute(&flow.AssertVisibleStep{BaseStep: flow.BaseStep{TimeoutMs: 10000}, Selector: flow.Selector{ID: "home"}})
			return r.Success, r.Message
		},
		"assertNotVisible": func(d *Driver) (bool, string) {
			r := d.assertNotVisible(&flow.AssertNotVisibleStep{BaseStep: flow.BaseStep{TimeoutMs: 10000}, Selector: flow.Selector{Text: "Banner"}})
			return r.Success, r.Message
		},
		"extendedWaitUntil visible": func(d *Driver) (bool, string) {
			r := d.Execute(&flow.WaitUntilStep{BaseStep: flow.BaseStep{TimeoutMs: 10000}, Visible: &flow.Selector{ID: "home"}})
			return r.Success, r.Message
		},
		"extendedWaitUntil notVisible": func(d *Driver) (bool, string) {
			r := d.Execute(&flow.WaitUntilStep{BaseStep: flow.BaseStep{TimeoutMs: 10000}, NotVisible: &flow.Selector{ID: "home"}})
			return r.Success, r.Message
		},
		"relative selector": func(d *Driver) (bool, string) {
			_, err := d.findElement(flow.Selector{Text: "Pay", Below: &flow.Selector{Text: "Total"}}, false, 10000)
			if err != nil {
				return false, err.Error()
			}
			return true, ""
		},
	}
	for name, run := range steps {
		t.Run(name, func(t *testing.T) {
			var reads int32
			server := axServerDownServer(t, &reads)
			defer server.Close()
			d := createTestDriver(server)

			start := time.Now()
			ok, msg := run(d)
			if ok {
				t.Fatal("passed although the app could not be read")
			}
			if took := time.Since(start); took > 3*time.Second {
				t.Errorf("took %v: polled instead of failing at once (%d page sources)", took, reads)
			}
			if !strings.Contains(msg, "accessibility server is not running") {
				t.Errorf("message does not say what the error is: %s", msg)
			}
		})
	}
}
