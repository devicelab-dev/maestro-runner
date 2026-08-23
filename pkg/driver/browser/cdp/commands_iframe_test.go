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

// === Other gesture commands across iframe boundaries (PR #73 follow-up) ===
// doubleTapOn / longPressOn / scrollUntilVisible all share the same iframe-
// coord root cause as tapOn — coordinates from getBoundingClientRect() are
// frame-local while CDP Input.dispatchMouseEvent uses top-frame viewport
// coords. Each test below verifies the corresponding command now lands the
// gesture inside the iframe rather than at iframe-local coords on the page.

// iframePageWithDblClickButton: iframe-nested button with a dblclick handler
// that uniquely tags document.title so the test can assert the double-click
// (not just any click) reached the target.
func iframePageWithDblClickButton() string {
	return `<!DOCTYPE html>
<html><body>
<h1>HEADER</h1>
<iframe id="f" title="inner" srcdoc='<!DOCTYPE html><html><body style="margin:0;padding:24px">
<button id="b">DoIt</button>
<script>
document.getElementById("b").addEventListener("dblclick", function() { document.title = "dblclicked-iframe"; });
</script>
</body></html>'></iframe>
</body></html>`
}

// iframePageWithLongPressButton: iframe-nested button with mousedown/up
// timing detection. Updates iframe title to "longpressed-iframe" only when
// the press duration exceeds 800ms (longPressOn uses 1s).
func iframePageWithLongPressButton() string {
	return `<!DOCTYPE html>
<html><body>
<h1>HEADER</h1>
<iframe id="f" title="inner" srcdoc='<!DOCTYPE html><html><body style="margin:0;padding:24px">
<button id="b">DoIt</button>
<script>
var t0 = 0;
var btn = document.getElementById("b");
btn.addEventListener("mousedown", function() { t0 = Date.now(); });
btn.addEventListener("mouseup", function() {
  if (t0 && Date.now() - t0 >= 800) document.title = "longpressed-iframe";
  else document.title = "shortpress-iframe";
});
</script>
</body></html>'></iframe>
</body></html>`
}

// TestDoubleTapOnCrossRoot_Iframe: double-click an iframe-nested button.
// Pre-fix this would dispatch two clicks at iframe-local coords on the top
// frame; the iframe never sees the dblclick event.
func TestDoubleTapOnCrossRoot_Iframe(t *testing.T) {
	ts := newIframeTestServer(iframePageWithDblClickButton())
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	res := d.Execute(&flow.DoubleTapOnStep{Selector: flow.Selector{Text: "DoIt"}})
	if !res.Success {
		t.Fatalf("doubleTapOn DoIt (iframe) failed: %s", res.Message)
	}
	if got := iframeTitle(t, d); got != "dblclicked-iframe" {
		t.Errorf("expected iframe title 'dblclicked-iframe', got %q", got)
	}
}

// TestLongPressOnCrossRoot_Iframe: long-press an iframe-nested button.
// Pre-fix this used elem.WaitInteractable() which returns iframe-local
// coords; subsequent Mouse.Down/Up at those coords on the top frame missed
// the iframe button entirely.
func TestLongPressOnCrossRoot_Iframe(t *testing.T) {
	ts := newIframeTestServer(iframePageWithLongPressButton())
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	res := d.Execute(&flow.LongPressOnStep{Selector: flow.Selector{Text: "DoIt"}})
	if !res.Success {
		t.Fatalf("longPressOn DoIt (iframe) failed: %s", res.Message)
	}
	if got := iframeTitle(t, d); got != "longpressed-iframe" {
		t.Errorf("expected iframe title 'longpressed-iframe' (>=800ms press), got %q", got)
	}
}

// === Iframe-clipping in visibility check (B6 / PLAN-maestro-upstream-parity) ===
// The visibility helper used to do intrinsic-only checks (computed style +
// getBoundingClientRect dimensions) and reported elements scrolled outside
// their iframe's content viewport as "visible." That false-positive made
// scrollUntilVisible's loop exit on iteration 0 and never invoke the new
// iframe-internal scrollIntoView branch (B5). It also caused waitForVisible
// / assertVisible to incorrectly pass on iframe-clipped elements.
//
// The fix walks the iframe ancestor chain and intersects the element's rect
// against each ancestor iframe's content viewport. Top-frame "below the
// fold" elements remain visible (matches Maestro CLI semantics and current
// top-frame behaviour); only iframe clipping is added.

// iframePageWithScrolledOutButton: iframe-nested button positioned far
// below the iframe's content viewport. The iframe is fixed at 300×200 and
// the button sits at iframe-local Y ≈ 5000 (above a tall spacer), so it
// only enters the viewport after the iframe content scrolls down.
//
// Used by both the unit-level visibility test and the scrollUntilVisible
// end-to-end test.
func iframePageWithScrolledOutButton() string {
	return `<!DOCTYPE html>
<html><body>
<h1>HEADER</h1>
<iframe id="f" title="inner" style="width:300px;height:200px;border:1px solid #888"
 srcdoc='<!DOCTYPE html><html><body style="margin:0;padding:0">
<div style="height:5000px;background:linear-gradient(#fff,#eee);"></div>
<button id="b" style="display:block;margin:8px;width:120px;height:40px">DoIt</button>
<div style="height:1000px"></div>
<script>
document.getElementById("b").addEventListener("click", function() { document.title = "clicked-scrolled"; });
</script>
</body></html>'></iframe>
</body></html>`
}

// TestIsElementVisible_IframeClipped: direct unit-level check on the
// visibility helper. Without B6 the call returns true even though the
// button is scrolled out of view inside the iframe; with B6 it correctly
// returns false until the iframe scrolls the button into its viewport.
func TestIsElementVisible_IframeClipped(t *testing.T) {
	ts := newIframeTestServer(iframePageWithScrolledOutButton())
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Pre-scroll: button is 5000px down inside iframe; iframe scrollTop=0.
	visBefore, err := d.page.Eval(`() => {
		var f = document.getElementById('f');
		var b = f.contentDocument.getElementById('b');
		return window.__maestro._isElementVisible(b);
	}`)
	if err != nil {
		t.Fatalf("eval pre-scroll visibility: %v", err)
	}
	if visBefore.Value.Bool() {
		t.Fatalf("expected iframe-clipped button to report _isElementVisible=false, got true")
	}

	// Scroll iframe content so the button enters its content viewport.
	if _, err := d.page.Eval(`() => {
		var f = document.getElementById('f');
		var b = f.contentDocument.getElementById('b');
		b.scrollIntoView({block: 'center', behavior: 'instant'});
	}`); err != nil {
		t.Fatalf("scrollIntoView eval: %v", err)
	}

	visAfter, err := d.page.Eval(`() => {
		var f = document.getElementById('f');
		var b = f.contentDocument.getElementById('b');
		return window.__maestro._isElementVisible(b);
	}`)
	if err != nil {
		t.Fatalf("eval post-scroll visibility: %v", err)
	}
	if !visAfter.Value.Bool() {
		t.Fatalf("expected scrolled-into-view button to report _isElementVisible=true, got false")
	}
}

// TestScrollUntilVisible_IframeClipped: end-to-end exercise of the B5 +
// B6 combination. Before B6, scrollUntilVisible exited on iteration 0
// because info.Visible was already true (false positive) and the iframe
// content was never scrolled. After B6 the visibility check returns false
// until B5's scrollIntoView path actually advances the iframe's scroll
// position.
func TestScrollUntilVisible_IframeClipped(t *testing.T) {
	ts := newIframeTestServer(iframePageWithScrolledOutButton())
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Sanity: iframe scrollTop should be 0 before scrollUntilVisible runs.
	pre, err := d.page.Eval(`() => {
		var f = document.getElementById('f');
		return f.contentDocument.documentElement.scrollTop ||
		       (f.contentDocument.body && f.contentDocument.body.scrollTop) || 0;
	}`)
	if err != nil {
		t.Fatalf("eval pre scrollTop: %v", err)
	}
	if pre.Value.Num() != 0 {
		t.Fatalf("expected iframe scrollTop=0 pre-test, got %v", pre.Value.Num())
	}

	res := d.Execute(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "DoIt"},
		Direction: "down",
	})
	if !res.Success {
		t.Fatalf("scrollUntilVisible (iframe-clipped) failed: %s", res.Message)
	}

	// After scrollUntilVisible, iframe scrollTop must have advanced — this
	// is the regression guard. Pre-fix it stayed at 0 because the visibility
	// false-positive let the loop exit on iteration 0.
	post, err := d.page.Eval(`() => {
		var f = document.getElementById('f');
		return f.contentDocument.documentElement.scrollTop ||
		       (f.contentDocument.body && f.contentDocument.body.scrollTop) || 0;
	}`)
	if err != nil {
		t.Fatalf("eval post scrollTop: %v", err)
	}
	if post.Value.Num() <= 0 {
		t.Errorf("expected iframe scrollTop>0 after scrollUntilVisible, got %v", post.Value.Num())
	}

	// And the button is reported visible at exit.
	if elemRes := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "DoIt"},
	}); !elemRes.Success {
		t.Errorf("button should be visible after scrollUntilVisible: %s", elemRes.Message)
	}
}
