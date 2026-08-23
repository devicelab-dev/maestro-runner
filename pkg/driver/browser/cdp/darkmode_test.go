package cdp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// darkModePage styles its body from prefers-color-scheme, so the tests can
// check the emulation actually reaches CSS rather than only reading back the
// value the driver just set. A media query the browser never applied would
// leave the background light while matchMedia happily reported dark.
func darkModePage() string {
	return `<!DOCTYPE html><html><head><style>
body { background-color: rgb(255, 255, 255); }
@media (prefers-color-scheme: dark) {
  body { background-color: rgb(0, 0, 0); }
}
</style></head><body><div id="content">hello</div></body></html>`
}

func newDarkModeServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, darkModePage())
	})
	return httptest.NewServer(mux)
}

func bodyBackground(t *testing.T, d *Driver) string {
	t.Helper()
	obj, err := d.page.Eval(`() => getComputedStyle(document.body).backgroundColor`)
	if err != nil {
		t.Fatalf("reading background colour: %v", err)
	}
	return obj.Value.Str()
}

const (
	lightBackground = "rgb(255, 255, 255)"
	darkBackground  = "rgb(0, 0, 0)"
)

// setMode puts the page in a known appearance so a test never depends on the
// browser's own default. That default is not light: this headless Chrome
// reports prefers-color-scheme: dark out of the box, so a flow that asserts
// dark mode without setting it first can pass for the wrong reason.
func setMode(t *testing.T, d *Driver, dark bool) {
	t.Helper()
	if res := d.Execute(&flow.SetDarkModeStep{Enabled: dark}); !res.Success {
		t.Fatalf("setDarkMode(%v): %v", dark, res.Error)
	}
}

// TestSetDarkModeAppliesToPage is the core of it: setDarkMode has to change
// what the page renders, not just what the driver remembers.
func TestSetDarkModeAppliesToPage(t *testing.T) {
	ts := newDarkModeServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	setMode(t, d, false)
	if got := bodyBackground(t, d); got != lightBackground {
		t.Fatalf("after setDarkMode false the page background is %s, want %s", got, lightBackground)
	}

	if res := d.Execute(&flow.SetDarkModeStep{Enabled: true}); !res.Success {
		t.Fatalf("setDarkMode: %v", res.Error)
	}
	if got := bodyBackground(t, d); got != darkBackground {
		t.Errorf("after setDarkMode the page background is %s, want %s", got, darkBackground)
	}

	if res := d.Execute(&flow.SetDarkModeStep{Enabled: false}); !res.Success {
		t.Fatalf("setDarkMode false: %v", res.Error)
	}
	if got := bodyBackground(t, d); got != lightBackground {
		t.Errorf("after setDarkMode false the page background is %s, want %s", got, lightBackground)
	}
}

// A page loaded after the override was set must still see it — the emulation
// is per-page state, and a navigation that silently dropped it would make dark
// mode work only for the document that happened to be open at the time.
func TestSetDarkModeSurvivesNavigation(t *testing.T) {
	ts := newDarkModeServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	if res := d.Execute(&flow.SetDarkModeStep{Enabled: true}); !res.Success {
		t.Fatalf("setDarkMode: %v", res.Error)
	}
	if err := d.page.Navigate(ts.URL + "/?again=1"); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if err := d.page.WaitLoad(); err != nil {
		t.Fatalf("wait load: %v", err)
	}
	if got := bodyBackground(t, d); got != darkBackground {
		t.Errorf("after navigation the page background is %s, want the override to still apply (%s)", got, darkBackground)
	}
}

func TestToggleDarkMode(t *testing.T) {
	ts := newDarkModeServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Start from a known mode so the assertions do not depend on the browser
	// default, which is dark here.
	setMode(t, d, false)

	if res := d.Execute(&flow.ToggleDarkModeStep{}); !res.Success {
		t.Fatalf("first toggle: %v", res.Error)
	}
	if got := bodyBackground(t, d); got != darkBackground {
		t.Errorf("after one toggle the background is %s, want %s", got, darkBackground)
	}

	if res := d.Execute(&flow.ToggleDarkModeStep{}); !res.Success {
		t.Fatalf("second toggle: %v", res.Error)
	}
	if got := bodyBackground(t, d); got != lightBackground {
		t.Errorf("after two toggles the background is %s, want %s", got, lightBackground)
	}
}

func TestAssertDarkModeAndLightMode(t *testing.T) {
	ts := newDarkModeServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	setMode(t, d, false)
	if res := d.Execute(&flow.AssertLightModeStep{}); !res.Success {
		t.Errorf("expected assertLightMode to pass on a light page: %v", res.Error)
	}
	if res := d.Execute(&flow.AssertDarkModeStep{}); res.Success {
		t.Error("expected assertDarkMode to fail on a light page")
	}

	if res := d.Execute(&flow.SetDarkModeStep{Enabled: true}); !res.Success {
		t.Fatalf("setDarkMode: %v", res.Error)
	}
	if res := d.Execute(&flow.AssertDarkModeStep{}); !res.Success {
		t.Errorf("expected assertDarkMode to pass after setDarkMode: %v", res.Error)
	}
	if res := d.Execute(&flow.AssertLightModeStep{}); res.Success {
		t.Error("expected assertLightMode to fail on a dark page")
	}
}

// The failure message has to say which mode was wanted and which was found —
// "assertion failed" alone leaves the author guessing which way round it went.
func TestAssertDarkModeFailureNamesBothModes(t *testing.T) {
	ts := newDarkModeServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	setMode(t, d, false)
	res := d.Execute(&flow.AssertDarkModeStep{})
	if res.Success {
		t.Fatal("expected assertDarkMode to fail on a light page")
	}
	msg := res.Error.Error()
	for _, want := range []string{"dark", "light"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected %q in the failure message, got %q", want, msg)
		}
	}
}
