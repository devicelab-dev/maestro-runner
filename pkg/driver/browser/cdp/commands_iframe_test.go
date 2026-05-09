package cdp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Tests for iframe / shadow-root coordinate-translated tapOn.
//
// Covers:
//   A: top-frame button — uses the existing Rod Click() path; verifies the
//      cross-root branch does not regress simple cases.
//   B: iframe-nested button (no shadow) — exercises topFrameClickPoint coord
//      translation through a single iframe boundary.
//   C: iframe + open shadow root — mirrors the repro from issues #71/#72
//      acting layer (Flutter-Web-shaped accessibility tree inside an iframe).
//   D: iframe-nested button occluded by a same-frame overlay — exercises
//      Playwright-pattern hit-target pre-flight rejection.
//   E: iframe inside a CSS-transformed wrapper — exercises the transform
//      bail (Playwright bails on transforms; we follow that decision).
//
// Issues #71/#72 acting layer.

// iframeTopFramePage returns a top-frame page with a single button. Used by
// case A.
func iframeTopFramePage() string {
	return `<!DOCTYPE html>
<html><body>
<button id="b" onclick="document.title='clicked-top'">DoIt</button>
</body></html>`
}

// iframePageWithButton returns a top-frame page that hosts an iframe (via
// srcdoc) containing a plain <button>. The button's click handler updates
// document.title in the IFRAME so the test can assert the click landed.
// The handler is attached via addEventListener inside a <script> tag rather
// than an inline onclick attribute — onclick value strings get tangled with
// srcdoc's own attribute quoting after one round of HTML entity decoding.
// Used by case B.
func iframePageWithButton() string {
	return `<!DOCTYPE html>
<html><body>
<h1>HEADER</h1>
<iframe id="f" title="inner" srcdoc='<!DOCTYPE html><html><body style="margin:0;padding:24px">
<button id="b">DoIt</button>
<script>
document.getElementById("b").addEventListener("click", function() { document.title = "clicked-iframe"; });
</script>
</body></html>'></iframe>
</body></html>`
}

// iframePageWithShadowDOM returns a top-frame page that hosts an iframe
// whose body mounts an open shadow root containing a button. Mirrors the
// hosted repro page used to reproduce #71/#72 acting-layer bugs.
//
// Quoting is delicate: srcdoc decodes HTML entities once, so the JS inside
// the script tag arrives unencoded. Use template literals (backticks) for
// CSS-selector strings rather than nested double-quoted strings, and use
// `getAttribute("role") === ...` checks rather than complex selectors so
// the script source remains valid after one round of entity decoding.
// Used by case C.
func iframePageWithShadowDOM() string {
	return `<!DOCTYPE html>
<html><body>
<h1>HEADER</h1>
<iframe id="f" title="inner" srcdoc='<!DOCTYPE html><html><body style="margin:0">
<flutter-view><flt-glass-pane id="g"></flt-glass-pane></flutter-view>
<template id="t">
<style>
flt-semantics-host, flt-semantics, flt-semantics-container { display: block; }
flt-semantics[role="dialog"] { position: absolute; left: 50%; top: 40%; transform: translate(-50%, -50%); width: 200px; padding: 24px; background: #fff; border: 1px solid #ddd; }
flt-semantics[role="button"] { display: inline-block; margin-top: 16px; padding: 8px 16px; background: #1976d2; color: #fff; cursor: pointer; }
</style>
<flt-semantics-host>
<flt-semantics role="dialog" aria-label="Welcome dialog">
<flt-semantics-container>
<flt-semantics aria-label="DIALOG_BODY_TEXT">DIALOG_BODY_TEXT</flt-semantics>
<flt-semantics role="button" aria-label="Close" tabindex="0">Close</flt-semantics>
</flt-semantics-container>
</flt-semantics>
</flt-semantics-host>
</template>
<script>
var host = document.getElementById("g");
var sr = host.attachShadow({ mode: "open" });
sr.appendChild(document.getElementById("t").content.cloneNode(true));
var all = sr.querySelectorAll("flt-semantics");
var closeBtn = null, dialog = null;
for (var i = 0; i < all.length; i++) {
  var role = all[i].getAttribute("role");
  if (role === "button") closeBtn = all[i];
  if (role === "dialog") dialog = all[i];
}
closeBtn.addEventListener("click", function() {
  if (dialog) dialog.remove();
  document.title = "dialog-closed";
});
</script>
</body></html>'></iframe>
</body></html>`
}

// iframePageWithOccludedButton returns a top-frame page with an iframe whose
// body has a button covered by a fixed-position overlay div sitting on top.
// Used by case D — pre-flight expectHitTarget should reject before dispatch.
func iframePageWithOccludedButton() string {
	return `<!DOCTYPE html>
<html><body>
<h1>HEADER</h1>
<iframe id="f" title="inner" srcdoc='<!DOCTYPE html><html><body style="margin:0;padding:24px;position:relative">
<button id="b" style="position:absolute;left:24px;top:24px;width:120px;height:40px">DoIt</button>
<div id="overlay" style="position:absolute;left:0;top:0;width:100%;height:100%;background:rgba(0,0,0,0.5);z-index:10"></div>
<script>
document.getElementById("b").addEventListener("click", function() { document.title = "clicked-button"; });
</script>
</body></html>'></iframe>
</body></html>`
}

// iframePageWithTransform returns a top-frame page that wraps the iframe in
// a translated container. Used by case E — describeIFrameStyle bails with
// 'transformed' which surfaces as a clear error from tapOnCrossRoot.
func iframePageWithTransform() string {
	return `<!DOCTYPE html>
<html><body>
<h1>HEADER</h1>
<div style="transform: translateX(20px)">
<iframe id="f" title="inner" srcdoc='<!DOCTYPE html><html><body style="margin:0;padding:24px">
<button id="b">DoIt</button>
<script>
document.getElementById("b").addEventListener("click", function() { document.title = "clicked-transformed"; });
</script>
</body></html>'></iframe>
</div>
</body></html>`
}

// newIframeTestServer wraps a single HTML string in an httptest server.
func newIframeTestServer(html string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, html)
	})
	return httptest.NewServer(mux)
}

// pageTitle reads document.title from the driver's current page.
func pageTitle(t *testing.T, d *Driver) string {
	t.Helper()
	res, err := d.page.Eval(`() => document.title`)
	if err != nil {
		t.Fatalf("page eval document.title: %v", err)
	}
	return res.Value.Str()
}

// iframeTitle reads document.title from inside the iframe whose CSS id is
// "f". Click handlers in the iframe update the iframe's title, not the top
// frame's.
func iframeTitle(t *testing.T, d *Driver) string {
	t.Helper()
	res, err := d.page.Eval(`() => {
		var f = document.getElementById('f');
		return (f && f.contentDocument && f.contentDocument.title) || '';
	}`)
	if err != nil {
		t.Fatalf("page eval iframe title: %v", err)
	}
	return res.Value.Str()
}

// TestTapOnCrossRoot_TopFrame (case A): top-frame button must still work,
// using the existing Rod Click() path, not the new cross-root path.
func TestTapOnCrossRoot_TopFrame(t *testing.T) {
	ts := newIframeTestServer(iframeTopFramePage())
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	res := d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "DoIt"}})
	if !res.Success {
		t.Fatalf("tapOn DoIt failed: %s", res.Message)
	}
	if got := pageTitle(t, d); got != "clicked-top" {
		t.Errorf("expected top-frame title 'clicked-top', got %q", got)
	}
}

// TestTapOnCrossRoot_Iframe (case B): button inside a same-origin iframe
// (no shadow). Verifies coord translation through a single iframe boundary.
func TestTapOnCrossRoot_Iframe(t *testing.T) {
	ts := newIframeTestServer(iframePageWithButton())
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	res := d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "DoIt"}})
	if !res.Success {
		t.Fatalf("tapOn DoIt (iframe) failed: %s", res.Message)
	}
	if got := iframeTitle(t, d); got != "clicked-iframe" {
		t.Errorf("expected iframe title 'clicked-iframe', got %q", got)
	}
}

// TestTapOnCrossRoot_IframeShadow (case C): button inside an open shadow
// root inside an iframe — the repro shape from issues #71/#72 acting layer.
func TestTapOnCrossRoot_IframeShadow(t *testing.T) {
	ts := newIframeTestServer(iframePageWithShadowDOM())
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Sanity: dialog body is visible before tap.
	res := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "DIALOG_BODY_TEXT"},
	})
	if !res.Success {
		t.Fatalf("dialog body should be visible before tap: %s", res.Message)
	}

	// Tap Close button inside the iframe + shadow.
	res = d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "Close"}})
	if !res.Success {
		t.Fatalf("tapOn Close (iframe+shadow) failed: %s", res.Message)
	}

	// Verify the iframe-side click handler ran.
	if got := iframeTitle(t, d); got != "dialog-closed" {
		t.Errorf("expected iframe title 'dialog-closed', got %q", got)
	}
}

// TestTapOnCrossRoot_Occluded (case D): button covered by a same-frame
// overlay. Pre-flight expectHitTarget should reject and tapOn should
// surface the occlusion as an error rather than silently dispatching.
func TestTapOnCrossRoot_Occluded(t *testing.T) {
	ts := newIframeTestServer(iframePageWithOccludedButton())
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	res := d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "DoIt"}})
	if res.Success {
		t.Fatalf("tapOn occluded button should fail; got success message %q", res.Message)
	}
	// Be lenient about the exact phrasing — just require that the error
	// message mentions occlusion / blocked / overlay so future wording
	// tweaks don't break the test.
	low := strings.ToLower(res.Message)
	if !strings.Contains(low, "block") && !strings.Contains(low, "overlay") &&
		!strings.Contains(low, "reach") && !strings.Contains(low, "hit") {
		t.Errorf("expected occlusion-shaped error, got %q", res.Message)
	}
	// And the click handler should NOT have run.
	if got := iframeTitle(t, d); got == "clicked-button" {
		t.Errorf("click handler ran despite occlusion (title=%q)", got)
	}
}

// TestTapOnCrossRoot_Transformed (case E): iframe wrapped in a CSS transform.
// describeIFrameStyle bails with 'transformed' (Playwright pattern); the
// resulting tapOnCrossRoot path returns a clear error rather than computing
// through DOMMatrix.
func TestTapOnCrossRoot_Transformed(t *testing.T) {
	ts := newIframeTestServer(iframePageWithTransform())
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	res := d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "DoIt"}})
	if res.Success {
		t.Fatalf("tapOn through transformed iframe should fail; got success %q", res.Message)
	}
	low := strings.ToLower(res.Message)
	if !strings.Contains(low, "transform") && !strings.Contains(low, "iframe coord") {
		t.Errorf("expected transformed-iframe error, got %q", res.Message)
	}
}
