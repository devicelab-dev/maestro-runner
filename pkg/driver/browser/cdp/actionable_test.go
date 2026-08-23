package cdp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// actionablePage serves a configurable test page. Pass query parameters to
// flip element states:
//   - ?disabled=1     → button has `disabled` attribute
//   - ?aria=1         → button has aria-disabled="true"
//   - ?pointer=1      → button has CSS pointer-events: none
//   - ?hidden=1       → button has display: none
//   - ?enableAfter=N  → button starts disabled then enables after N ms
func actionablePage(state string) string {
	style := ""
	attrs := ""
	scripts := ""
	switch state {
	case "disabled":
		attrs = " disabled"
	case "aria":
		attrs = ` aria-disabled="true"`
	case "pointer":
		style = " style=\"pointer-events: none\""
	case "hidden":
		style = " style=\"display: none\""
	case "enableAfter":
		attrs = " disabled"
		scripts = `<script>setTimeout(() => document.getElementById('btn').removeAttribute('disabled'), 200);</script>`
	}
	return fmt.Sprintf(`<!DOCTYPE html><html><body>
<button id="btn"%s%s onclick="document.title='clicked'">DoIt</button>
%s
</body></html>`, attrs, style, scripts)
}

func newActionableTestServer(html string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, html)
	})
	return httptest.NewServer(mux)
}

// TestIsActionable_HappyPath: a plain visible enabled button reports
// actionable on first probe.
func TestIsActionable_HappyPath(t *testing.T) {
	ts := newActionableTestServer(actionablePage(""))
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	elem, _, err := d.findElement(flow.Selector{ID: "btn"}, false, 2000)
	if err != nil {
		t.Fatalf("findElement: %v", err)
	}

	res, err := elem.Eval(`() => window.__maestro._isActionable(this)`)
	if err != nil {
		t.Fatalf("eval _isActionable: %v", err)
	}
	if !res.Value.Bool() {
		t.Errorf("expected actionable=true for a plain visible button, got false")
	}
}

// TestIsActionable_Disabled: button with the HTML `disabled` attribute is
// not actionable. The check covers form-control disabled state.
func TestIsActionable_Disabled(t *testing.T) {
	ts := newActionableTestServer(actionablePage("disabled"))
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	elem, _, err := d.findElement(flow.Selector{ID: "btn"}, false, 2000)
	if err != nil {
		t.Fatalf("findElement: %v", err)
	}
	res, _ := elem.Eval(`() => window.__maestro._isActionable(this)`)
	if res.Value.Bool() {
		t.Errorf("expected actionable=false for disabled button")
	}
}

// TestIsActionable_AriaDisabled: aria-disabled="true" blocks actionability
// even when the HTML disabled attribute is not set (covers ARIA-only
// custom controls).
func TestIsActionable_AriaDisabled(t *testing.T) {
	ts := newActionableTestServer(actionablePage("aria"))
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	elem, _, err := d.findElement(flow.Selector{ID: "btn"}, false, 2000)
	if err != nil {
		t.Fatalf("findElement: %v", err)
	}
	res, _ := elem.Eval(`() => window.__maestro._isActionable(this)`)
	if res.Value.Bool() {
		t.Errorf("expected actionable=false for aria-disabled button")
	}
}

// TestIsActionable_PointerEventsNone: pointer-events: none blocks
// actionability — covers CSS-disabled controls (common pattern for
// "looks enabled but pre-emptively blocked").
func TestIsActionable_PointerEventsNone(t *testing.T) {
	ts := newActionableTestServer(actionablePage("pointer"))
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	elem, _, err := d.findElement(flow.Selector{ID: "btn"}, false, 2000)
	if err != nil {
		t.Fatalf("findElement: %v", err)
	}
	res, _ := elem.Eval(`() => window.__maestro._isActionable(this)`)
	if res.Value.Bool() {
		t.Errorf("expected actionable=false when pointer-events: none")
	}
}

// TestWaitForActionable_WaitsForEnable: button starts disabled and enables
// after 200ms. waitForActionable must wait through the disable state and
// succeed once the page enables the control.
func TestWaitForActionable_WaitsForEnable(t *testing.T) {
	ts := newActionableTestServer(actionablePage("enableAfter"))
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	elem, _, err := d.findElement(flow.Selector{ID: "btn"}, false, 2000)
	if err != nil {
		t.Fatalf("findElement: %v", err)
	}

	// Wait up to 2s. Page enables at 200ms; should succeed within ~250ms.
	if err := d.waitForActionable(elem, 2000); err != nil {
		t.Fatalf("expected to become actionable after enable; got: %v", err)
	}
}

// TestWaitForActionable_TimesOut: persistently disabled button never
// becomes actionable; waitForActionable returns a clear error.
func TestWaitForActionable_TimesOut(t *testing.T) {
	ts := newActionableTestServer(actionablePage("disabled"))
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	elem, _, err := d.findElement(flow.Selector{ID: "btn"}, false, 2000)
	if err != nil {
		t.Fatalf("findElement: %v", err)
	}

	// Tight timeout — element is permanently disabled.
	err = d.waitForActionable(elem, 200)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "not actionable") {
		t.Errorf("expected error to mention 'not actionable', got: %v", err)
	}
}

// TestTapOn_RejectsDisabled: end-to-end through the tapOn command. A
// disabled button must surface an error rather than silently dispatching
// or hanging on the existing find timeout.
func TestTapOn_RejectsDisabled(t *testing.T) {
	ts := newActionableTestServer(actionablePage("disabled"))
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Short timeout so we don't burn the default find budget.
	step := &flow.TapOnStep{Selector: flow.Selector{ID: "btn"}}
	step.TimeoutMs = 500
	res := d.Execute(step)
	if res.Success {
		t.Fatal("expected tapOn on disabled button to fail")
	}
	if !strings.Contains(strings.ToLower(res.Message), "not actionable") {
		t.Errorf("expected 'not actionable' error; got: %s", res.Message)
	}
}

// TestTapOn_PassesAfterEnable: page enables the button after 200ms; the
// existing find loop + new actionable gate cooperate so tapOn succeeds.
func TestTapOn_PassesAfterEnable(t *testing.T) {
	ts := newActionableTestServer(actionablePage("enableAfter"))
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	res := d.Execute(&flow.TapOnStep{Selector: flow.Selector{ID: "btn"}})
	if !res.Success {
		t.Fatalf("expected tapOn to succeed once button enables; got: %s", res.Message)
	}
}
