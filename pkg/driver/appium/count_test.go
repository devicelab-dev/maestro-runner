package appium

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Page source with three displayed rows and one hidden row sharing an id.
const countPageSourceXML = `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout bounds="[0,0][1080,1920]" class="android.widget.FrameLayout" enabled="true" displayed="true">
    <android.widget.TextView text="Row" resource-id="com.example:id/row" bounds="[0,100][1080,200]" enabled="true" displayed="true" />
    <android.widget.TextView text="Row" resource-id="com.example:id/row" bounds="[0,200][1080,300]" enabled="true" displayed="true" />
    <android.widget.TextView text="Row" resource-id="com.example:id/row" bounds="[0,300][1080,400]" enabled="true" displayed="true" />
    <android.widget.TextView text="Row" resource-id="com.example:id/row" bounds="[0,400][1080,500]" enabled="true" displayed="false" />
  </android.widget.FrameLayout>
</hierarchy>`

func newCountTestDriver() (*Driver, func()) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			writeJSON(w, map[string]interface{}{"value": countPageSourceXML})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	return createTestAppiumDriver(server), server.Close
}

func TestCountDisplayed(t *testing.T) {
	elements := []*ParsedElement{
		{Displayed: true},
		{Displayed: false},
		{Displayed: true},
	}
	if got := countDisplayed(elements); got != 2 {
		t.Errorf("countDisplayed = %d, want 2", got)
	}
	if got := countDisplayed(nil); got != 0 {
		t.Errorf("countDisplayed(nil) = %d, want 0", got)
	}
}

func TestCountVisibleMatches(t *testing.T) {
	d, closeFn := newCountTestDriver()
	defer closeFn()

	// The hidden fourth row matches the selector but is not displayed.
	n, err := d.countVisibleMatches(flow.Selector{ID: "row"})
	if err != nil {
		t.Fatalf("countVisibleMatches failed: %v", err)
	}
	if n != 3 {
		t.Errorf("countVisibleMatches = %d, want 3", n)
	}
}

func TestAssertVisibleCount_Match(t *testing.T) {
	d, closeFn := newCountTestDriver()
	defer closeFn()

	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "row"}, Count: "3"}
	step.TimeoutMs = 1000

	result := d.assertVisible(step)
	if !result.Success {
		t.Fatalf("expected success, got: %v - %s", result.Error, result.Message)
	}
	if !strings.Contains(result.Message, "3") {
		t.Errorf("message should mention the count, got %q", result.Message)
	}
}

func TestAssertVisibleCount_Mismatch(t *testing.T) {
	d, closeFn := newCountTestDriver()
	defer closeFn()

	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "row"}, Count: "2"}
	step.TimeoutMs = 400 // fail fast: the count is stable at 3

	result := d.assertVisible(step)
	if result.Success {
		t.Fatal("expected failure when 3 rows are visible but 2 were expected")
	}
	if !strings.Contains(result.Message, "Expected 2") || !strings.Contains(result.Message, "found 3") {
		t.Errorf("message should state expected and observed counts, got %q", result.Message)
	}
}

func TestAssertVisibleCount_InvalidCount(t *testing.T) {
	d, closeFn := newCountTestDriver()
	defer closeFn()

	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "row"}, Count: "${UNEXPANDED}"}
	result := d.assertVisible(step)
	if result.Success {
		t.Fatal("expected failure for an unresolvable count")
	}
}
