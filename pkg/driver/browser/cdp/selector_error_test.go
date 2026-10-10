package cdp

import (
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// A CSS selector the browser rejects, or an index that is not a number, fails
// at once: both used to look like "not found yet" until the timeout ran out.
func TestFindElementRejectsBadSelectorsAtOnce(t *testing.T) {
	ts := newActionableTestServer(actionablePage(""))
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	for _, tc := range []struct {
		sel  flow.Selector
		want string
	}{
		{flow.Selector{CSS: "button[id="}, "invalid CSS selector"},
		{flow.Selector{ID: "btn", Index: "undefined"}, "whole number"},
	} {
		start := time.Now()
		_, _, err := d.findElement(tc.sel, false, 10000)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want one containing %q", tc.sel.Describe(), err, tc.want)
		}
		if took := time.Since(start); took > 3*time.Second {
			t.Errorf("%s: took %v, want an immediate failure (timeout was 10s)", tc.sel.Describe(), took)
		}
	}
	// A valid selector still finds the element.
	if _, _, err := d.findElement(flow.Selector{CSS: "button#btn"}, false, 2000); err != nil {
		t.Errorf("valid CSS: %v", err)
	}
}
