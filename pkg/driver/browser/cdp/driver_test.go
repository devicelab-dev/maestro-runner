package cdp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// testPage returns a minimal HTML page for testing.
func testPage() string {
	return `<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
	<h1 id="title">Hello World</h1>
	<button id="btn" data-testid="main-btn" aria-label="Click Me">Click Me</button>
	<a href="/other" id="link1">Go to Other</a>
	<input id="input1" type="text" placeholder="Enter text" aria-label="Name Input">
	<div id="hidden" style="display:none">Hidden Text</div>
	<p id="para">Some paragraph text</p>
	<input id="checkbox1" type="checkbox">
	<select id="select1"><option value="a">Option A</option><option value="b">Option B</option></select>
</body>
</html>`
}

func otherPage() string {
	return `<!DOCTYPE html>
<html>
<head><title>Other Page</title></head>
<body>
	<h1>Other Page</h1>
	<a href="/" id="back-link">Go Back</a>
</body>
</html>`
}

// selectorTestPage returns an HTML page with various attributes for testing new selectors.
func selectorTestPage() string {
	return `<!DOCTYPE html>
<html>
<head><title>Selector Test</title></head>
<body>
	<input data-testid="email-input" type="email" placeholder="Enter email" name="user_email">
	<input data-testid="search-input" type="text" placeholder="Search..." name="search_query">
	<button role="button" data-testid="submit-btn">Submit Form</button>
	<a href="/about" title="About us page">About Us</a>
	<a href="/contact">Contact</a>
	<img alt="Company Logo" src="logo.png">
	<img alt="Team Photo" src="team.png" title="Our team">
	<p>Welcome to our website</p>
	<p>Welcome back, user!</p>
	<span>Order #12345 confirmed</span>
	<div class="item">First Item</div>
	<div class="item">Second Item</div>
	<div class="item">Third Item</div>
</body>
</html>`
}

// newTestServer creates a test HTTP server with basic pages.
func newTestServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, testPage())
	})
	mux.HandleFunc("/other", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, otherPage())
	})
	mux.HandleFunc("/selectors", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, selectorTestPage())
	})
	mux.HandleFunc("/alert", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<button id="alert-btn" onclick="alert('Hello!')">Show Alert</button>
		</body></html>`)
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<input id="file-input" type="file" multiple>
			<div id="file-name"></div>
			<script>
				document.getElementById('file-input').addEventListener('change', function(e) {
					var names = Array.from(e.target.files).map(f => f.name).join(', ');
					document.getElementById('file-name').textContent = names;
				});
			</script>
		</body></html>`)
	})
	mux.HandleFunc("/download-page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<a id="download-link" href="/download-file">Download</a>
		</body></html>`)
	})
	mux.HandleFunc("/download-file", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=test-file.txt")
		w.Header().Set("Content-Type", "application/octet-stream")
		fmt.Fprint(w, "download content")
	})
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"users": [{"id": 1, "name": "Real User"}]}`)
	})
	mux.HandleFunc("/api/submit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok": true}`)
	})
	mux.HandleFunc("/fetch-page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<div id="result"></div>
			<script>
				async function fetchAPI(url) {
					try {
						const res = await fetch(url);
						const data = await res.text();
						document.getElementById('result').textContent = data;
					} catch(e) {
						document.getElementById('result').textContent = 'ERROR: ' + e.message;
					}
				}
			</script>
		</body></html>`)
	})
	return httptest.NewServer(mux)
}

// newTestDriver creates a Driver connected to the test server.
func newTestDriver(t *testing.T, serverURL string) *Driver {
	t.Helper()
	d, err := New(Config{
		Headless:  true,
		URL:       serverURL,
		ViewportW: 1024,
		ViewportH: 768,
	})
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}
	// Use shorter timeouts for tests
	d.SetFindTimeout(5000)
	return d
}

func TestNewAndClose(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Verify driver state
	info := d.GetPlatformInfo()
	if info.Platform != "web" {
		t.Errorf("expected platform 'web', got %q", info.Platform)
	}
	if info.ScreenWidth != 1024 {
		t.Errorf("expected width 1024, got %d", info.ScreenWidth)
	}
	if info.ScreenHeight != 768 {
		t.Errorf("expected height 768, got %d", info.ScreenHeight)
	}

	state := d.GetState()
	if state.Orientation != "landscape" {
		t.Errorf("expected orientation 'landscape', got %q", state.Orientation)
	}
}

// TestUserDataDir verifies that a UserDataDir setting causes Chrome to use
// that directory as its profile — localStorage survives a second New() call
// against the same directory.
func TestUserDataDir(t *testing.T) {
	// Manually managed dir — t.TempDir's auto-cleanup races Chrome's
	// lock-file release on macOS and surfaces as a spurious test failure.
	dir, err := os.MkdirTemp("", "maestro-user-data-dir-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	ts := newTestServer()
	defer ts.Close()

	// First session: write to localStorage.
	d1, err := New(Config{
		Headless:    true,
		URL:         ts.URL,
		UserDataDir: dir,
		ViewportW:   1024,
		ViewportH:   768,
	})
	if err != nil {
		t.Fatalf("first driver: %v", err)
	}
	d1.SetFindTimeout(5000)

	_, err = d1.page.Eval(`() => { localStorage.setItem("h5_test", "stayed"); }`)
	if err != nil {
		t.Fatalf("set localStorage: %v", err)
	}
	if err := d1.Close(); err != nil {
		t.Fatalf("close first driver: %v", err)
	}

	// Second session: same UserDataDir → localStorage entry should survive.
	d2, err := New(Config{
		Headless:    true,
		URL:         ts.URL,
		UserDataDir: dir,
		ViewportW:   1024,
		ViewportH:   768,
	})
	if err != nil {
		t.Fatalf("second driver: %v", err)
	}
	d2.SetFindTimeout(5000)
	defer d2.Close()

	obj, err := d2.page.Eval(`() => localStorage.getItem("h5_test")`)
	if err != nil {
		t.Fatalf("read localStorage: %v", err)
	}
	got := obj.Value.Str()
	if got != "stayed" {
		t.Errorf("localStorage not preserved across runs; got %q, want %q", got, "stayed")
	}
}

func TestAssertVisible(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Text visible
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Hello World"},
	})
	if !result.Success {
		t.Errorf("assertVisible 'Hello World' should succeed: %s", result.Message)
	}

	// Button text visible
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Click Me"},
	})
	if !result.Success {
		t.Errorf("assertVisible 'Click Me' should succeed: %s", result.Message)
	}

	// ID visible
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{ID: "title"},
	})
	if !result.Success {
		t.Errorf("assertVisible id='title' should succeed: %s", result.Message)
	}

	// CSS visible
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "#btn"},
	})
	if !result.Success {
		t.Errorf("assertVisible css='#btn' should succeed: %s", result.Message)
	}
}

func TestAssertNotVisible(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Element that doesn't exist
	result := d.Execute(&flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 2000},
		Selector: flow.Selector{Text: "Nonexistent Text"},
	})
	if !result.Success {
		t.Errorf("assertNotVisible 'Nonexistent Text' should succeed: %s", result.Message)
	}

	// Hidden element
	result = d.Execute(&flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 2000},
		Selector: flow.Selector{CSS: "#hidden"},
	})
	if !result.Success {
		t.Errorf("assertNotVisible '#hidden' should succeed: %s", result.Message)
	}

	// Visible element should fail
	result = d.Execute(&flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{Text: "Hello World"},
	})
	if result.Success {
		t.Errorf("assertNotVisible 'Hello World' should fail since element is visible")
	}
}

func TestTapOn(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Tap on button by text
	result := d.Execute(&flow.TapOnStep{
		Selector: flow.Selector{Text: "Click Me"},
	})
	if !result.Success {
		t.Errorf("tapOn 'Click Me' should succeed: %s", result.Message)
	}
	if result.Element == nil {
		t.Error("tapOn should return element info")
	}
}

func TestTapOnLink(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Tap on link navigates to other page
	result := d.Execute(&flow.TapOnStep{
		Selector: flow.Selector{Text: "Go to Other"},
	})
	if !result.Success {
		t.Errorf("tapOn link should succeed: %s", result.Message)
	}

	// Wait for navigation
	time.Sleep(500 * time.Millisecond)

	// Verify we're on the other page
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Other Page"},
	})
	if !result.Success {
		t.Errorf("should be on other page: %s", result.Message)
	}
}

func TestInputText(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Input with selector
	result := d.Execute(&flow.InputTextStep{
		Text:     "hello world",
		Selector: flow.Selector{CSS: "#input1"},
	})
	if !result.Success {
		t.Errorf("inputText should succeed: %s", result.Message)
	}

	// Verify text was entered
	result = d.Execute(&flow.CopyTextFromStep{
		Selector: flow.Selector{CSS: "#input1"},
	})
	if !result.Success {
		t.Errorf("copyTextFrom should succeed: %s", result.Message)
	}
}

func TestEraseText(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// First input some text
	d.Execute(&flow.InputTextStep{
		Text:     "hello",
		Selector: flow.Selector{CSS: "#input1"},
	})

	// Now erase it
	result := d.Execute(&flow.EraseTextStep{Characters: 5})
	if !result.Success {
		t.Errorf("eraseText should succeed: %s", result.Message)
	}
}

func TestBack(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Navigate to other page
	d.Execute(&flow.TapOnStep{
		Selector: flow.Selector{Text: "Go to Other"},
	})
	time.Sleep(500 * time.Millisecond)

	// Verify on other page
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Other Page"},
	})
	if !result.Success {
		t.Fatalf("should be on other page: %s", result.Message)
	}

	// Go back
	result = d.Execute(&flow.BackStep{})
	if !result.Success {
		t.Errorf("back should succeed: %s", result.Message)
	}

	// Verify back on main page
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Hello World"},
	})
	if !result.Success {
		t.Errorf("should be back on main page: %s", result.Message)
	}
}

func TestScreenshot(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.TakeScreenshotStep{})
	if !result.Success {
		t.Errorf("takeScreenshot should succeed: %s", result.Message)
	}

	data, ok := result.Data.([]byte)
	if !ok || len(data) == 0 {
		t.Error("screenshot should return PNG bytes")
	}

	// Also test the Driver.Screenshot() method
	bytes, err := d.Screenshot()
	if err != nil {
		t.Errorf("Screenshot() should succeed: %v", err)
	}
	if len(bytes) == 0 {
		t.Error("Screenshot() should return non-empty bytes")
	}
}

func TestScroll(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.ScrollStep{Direction: "down"})
	if !result.Success {
		t.Errorf("scroll down should succeed: %s", result.Message)
	}

	result = d.Execute(&flow.ScrollStep{Direction: "up"})
	if !result.Success {
		t.Errorf("scroll up should succeed: %s", result.Message)
	}
}

func TestPressKey(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.PressKeyStep{Key: "tab"})
	if !result.Success {
		t.Errorf("pressKey tab should succeed: %s", result.Message)
	}

	result = d.Execute(&flow.PressKeyStep{Key: "enter"})
	if !result.Success {
		t.Errorf("pressKey enter should succeed: %s", result.Message)
	}

	// Unknown key should fail
	result = d.Execute(&flow.PressKeyStep{Key: "unknown_key"})
	if result.Success {
		t.Error("pressKey unknown_key should fail")
	}
}

func TestClearState(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.ClearStateStep{})
	if !result.Success {
		t.Errorf("clearState should succeed: %s", result.Message)
	}
}

func TestLaunchApp(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Navigate to another URL via launchApp
	result := d.Execute(&flow.LaunchAppStep{
		AppID: ts.URL + "/other",
	})
	if !result.Success {
		t.Errorf("launchApp should succeed: %s", result.Message)
	}

	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Other Page"},
	})
	if !result.Success {
		t.Errorf("should be on other page after launchApp: %s", result.Message)
	}
}

func TestCopyTextFrom(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.CopyTextFromStep{
		Selector: flow.Selector{CSS: "#para"},
	})
	if !result.Success {
		t.Errorf("copyTextFrom should succeed: %s", result.Message)
	}

	text, ok := result.Data.(string)
	if !ok {
		t.Fatal("copyTextFrom data should be string")
	}
	if !strings.Contains(text, "Some paragraph text") {
		t.Errorf("expected 'Some paragraph text', got %q", text)
	}

	// Clipboard should be set
	state := d.GetState()
	if !strings.Contains(state.ClipboardText, "Some paragraph text") {
		t.Errorf("clipboard should contain copied text, got %q", state.ClipboardText)
	}
}

func TestSetClipboardAndPaste(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Set clipboard
	result := d.Execute(&flow.SetClipboardStep{Text: "clipboard text"})
	if !result.Success {
		t.Errorf("setClipboard should succeed: %s", result.Message)
	}

	// Focus input
	d.Execute(&flow.TapOnStep{
		Selector: flow.Selector{CSS: "#input1"},
	})

	// Paste
	result = d.Execute(&flow.PasteTextStep{})
	if !result.Success {
		t.Errorf("pasteText should succeed: %s", result.Message)
	}
}

func TestHideKeyboard(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// No-op but should succeed
	result := d.Execute(&flow.HideKeyboardStep{})
	if !result.Success {
		t.Errorf("hideKeyboard should succeed: %s", result.Message)
	}
}

func TestFindByID(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Find by direct ID
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{ID: "btn"},
	})
	if !result.Success {
		t.Errorf("find by id 'btn' should succeed: %s", result.Message)
	}

	// Find by data-testid
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{ID: "main-btn"},
	})
	if !result.Success {
		t.Errorf("find by data-testid 'main-btn' should succeed: %s", result.Message)
	}
}

func TestFindByCSS(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "h1#title"},
	})
	if !result.Success {
		t.Errorf("find by CSS 'h1#title' should succeed: %s", result.Message)
	}

	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "button"},
	})
	if !result.Success {
		t.Errorf("find by CSS 'button' should succeed: %s", result.Message)
	}
}

func TestFindByText(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Button text (should be found via AX tree clickable roles)
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Click Me"},
	})
	if !result.Success {
		t.Errorf("find button text 'Click Me' should succeed: %s", result.Message)
	}

	// Link text
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Go to Other"},
	})
	if !result.Success {
		t.Errorf("find link text 'Go to Other' should succeed: %s", result.Message)
	}

	// Heading text
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Hello World"},
	})
	if !result.Success {
		t.Errorf("find heading text 'Hello World' should succeed: %s", result.Message)
	}

	// Paragraph text
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Some paragraph text"},
	})
	if !result.Success {
		t.Errorf("find paragraph text should succeed: %s", result.Message)
	}
}

func TestHierarchy(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	data, err := d.Hierarchy()
	if err != nil {
		t.Fatalf("Hierarchy() should succeed: %v", err)
	}
	if len(data) == 0 {
		t.Error("Hierarchy() should return non-empty data")
	}
}

func TestSetOrientation(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Switch to portrait
	result := d.Execute(&flow.SetOrientationStep{Orientation: "PORTRAIT"})
	if !result.Success {
		t.Errorf("setOrientation PORTRAIT should succeed: %s", result.Message)
	}

	state := d.GetState()
	if state.Orientation != "portrait" {
		t.Errorf("expected orientation 'portrait', got %q", state.Orientation)
	}

	// Switch to landscape
	result = d.Execute(&flow.SetOrientationStep{Orientation: "LANDSCAPE"})
	if !result.Success {
		t.Errorf("setOrientation LANDSCAPE should succeed: %s", result.Message)
	}

	state = d.GetState()
	if state.Orientation != "landscape" {
		t.Errorf("expected orientation 'landscape', got %q", state.Orientation)
	}
}

func TestWaitUntilVisible(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	sel := flow.Selector{Text: "Hello World"}
	result := d.Execute(&flow.WaitUntilStep{
		BaseStep: flow.BaseStep{TimeoutMs: 3000},
		Visible:  &sel,
	})
	if !result.Success {
		t.Errorf("waitUntil visible should succeed: %s", result.Message)
	}
}

func TestWaitForAnimationToEnd(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.WaitForAnimationToEndStep{})
	if !result.Success {
		t.Errorf("waitForAnimationToEnd should succeed: %s", result.Message)
	}
}

func TestEvalBrowserScript(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Return a value
	result := d.Execute(&flow.EvalBrowserScriptStep{
		BaseStep: flow.BaseStep{StepType: flow.StepEvalBrowserScript},
		Script:   "return document.title",
		Output:   "title",
	})
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Data == nil {
		t.Fatal("expected Data to contain return value")
	}
	title, ok := result.Data.(string)
	if !ok {
		t.Fatalf("expected Data to be string, got %T", result.Data)
	}
	if title == "" {
		t.Error("expected non-empty document title")
	}

	// Manipulate DOM and verify
	result = d.Execute(&flow.EvalBrowserScriptStep{
		BaseStep: flow.BaseStep{StepType: flow.StepEvalBrowserScript},
		Script:   `document.body.setAttribute("data-test", "hello"); return document.body.getAttribute("data-test")`,
	})
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Data != "hello" {
		t.Errorf("expected 'hello', got %q", result.Data)
	}

	// No script should fail
	result = d.Execute(&flow.EvalBrowserScriptStep{
		BaseStep: flow.BaseStep{StepType: flow.StepEvalBrowserScript},
		Script:   "",
	})
	if result.Success {
		t.Error("expected failure for empty script")
	}

	// Invalid JS should fail
	result = d.Execute(&flow.EvalBrowserScriptStep{
		BaseStep: flow.BaseStep{StepType: flow.StepEvalBrowserScript},
		Script:   "return {{{invalid",
	})
	if result.Success {
		t.Error("expected failure for invalid JS")
	}
}

func TestEvalBrowserScriptAsync(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Async/await should work
	result := d.Execute(&flow.EvalBrowserScriptStep{
		BaseStep: flow.BaseStep{StepType: flow.StepEvalBrowserScript},
		Script:   "const r = await Promise.resolve(42); return String(r)",
	})
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Data != "42" {
		t.Errorf("expected '42', got %q", result.Data)
	}
}

func TestEvalBrowserScriptLocalStorage(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Write to localStorage
	result := d.Execute(&flow.EvalBrowserScriptStep{
		BaseStep: flow.BaseStep{StepType: flow.StepEvalBrowserScript},
		Script:   `localStorage.setItem("key1", "value1"); return localStorage.getItem("key1")`,
	})
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Data != "value1" {
		t.Errorf("expected 'value1', got %q", result.Data)
	}
}

func TestUnsupportedCommands(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	tests := []struct {
		name string
		step flow.Step
	}{
		{"airplaneMode", &flow.SetAirplaneModeStep{}},
		{"toggleAirplane", &flow.ToggleAirplaneModeStep{}},
		{"travel", &flow.TravelStep{}},
		{"addMedia", &flow.AddMediaStep{}},
		{"startRecording", &flow.StartRecordingStep{}},
		{"stopRecording", &flow.StopRecordingStep{}},
		{"clearKeychain", &flow.ClearKeychainStep{}},
		{"setPermissions", &flow.SetPermissionsStep{}},
		{"assertNoDefectsWithAI", &flow.AssertNoDefectsWithAIStep{}},
		{"assertWithAI", &flow.AssertWithAIStep{}},
		{"extractTextWithAI", &flow.ExtractTextWithAIStep{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Execute(tt.step)
			if result.Success {
				t.Errorf("%s should not succeed on web platform", tt.name)
			}
		})
	}
}

func TestStopAndKillApp(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.StopAppStep{})
	if !result.Success {
		t.Errorf("stopApp should succeed: %s", result.Message)
	}

	// After stop, navigate again
	result = d.Execute(&flow.LaunchAppStep{AppID: ts.URL})
	if !result.Success {
		t.Errorf("launchApp after stop should succeed: %s", result.Message)
	}

	result = d.Execute(&flow.KillAppStep{})
	if !result.Success {
		t.Errorf("killApp should succeed: %s", result.Message)
	}
}

func TestOpenLink(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.OpenLinkStep{Link: ts.URL + "/other"})
	if !result.Success {
		t.Errorf("openLink should succeed: %s", result.Message)
	}

	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Other Page"},
	})
	if !result.Success {
		t.Errorf("should see other page after openLink: %s", result.Message)
	}
}

func TestDriverInterface(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Verify it satisfies the core.Driver interface
	var _ core.Driver = d
}

func TestTapOnPoint(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.TapOnPointStep{X: 100, Y: 100})
	if !result.Success {
		t.Errorf("tapOnPoint should succeed: %s", result.Message)
	}

	// Percentage-based point
	result = d.Execute(&flow.TapOnPointStep{Point: "50%, 50%"})
	if !result.Success {
		t.Errorf("tapOnPoint with percentage should succeed: %s", result.Message)
	}
}

func TestSetLocation(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.SetLocationStep{
		Latitude:  "37.7749",
		Longitude: "-122.4194",
	})
	if !result.Success {
		t.Errorf("setLocation should succeed: %s", result.Message)
	}
}

func TestInputRandom(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Focus input first
	d.Execute(&flow.TapOnStep{Selector: flow.Selector{CSS: "#input1"}})

	result := d.Execute(&flow.InputRandomStep{DataType: "TEXT", Length: 5})
	if !result.Success {
		t.Errorf("inputRandom TEXT should succeed: %s", result.Message)
	}
	if result.Data == nil {
		t.Error("inputRandom should return generated text as data")
	}
}

func TestDoubleTapOn(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.DoubleTapOnStep{
		Selector: flow.Selector{Text: "Click Me"},
	})
	if !result.Success {
		t.Errorf("doubleTapOn should succeed: %s", result.Message)
	}
}

func TestLongPressOn(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.LongPressOnStep{
		Selector: flow.Selector{Text: "Click Me"},
	})
	if !result.Success {
		t.Errorf("longPressOn should succeed: %s", result.Message)
	}
}

func TestSwipe(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	for _, dir := range []string{"up", "down", "left", "right"} {
		result := d.Execute(&flow.SwipeStep{Direction: dir})
		if !result.Success {
			t.Errorf("swipe %s should succeed: %s", dir, result.Message)
		}
	}
}

func TestParsePercentageCoords(t *testing.T) {
	tests := []struct {
		input string
		x, y  float64
		err   bool
	}{
		{"50%, 50%", 0.5, 0.5, false},
		{"0%, 100%", 0, 1.0, false},
		{"85%, 15%", 0.85, 0.15, false},
		{"invalid", 0, 0, true},
		{"50%, ", 0, 0, true},
	}

	for _, tt := range tests {
		x, y, err := parsePercentageCoords(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("parsePercentageCoords(%q) should fail", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePercentageCoords(%q) failed: %v", tt.input, err)
			continue
		}
		if x != tt.x || y != tt.y {
			t.Errorf("parsePercentageCoords(%q) = (%f, %f), want (%f, %f)", tt.input, x, y, tt.x, tt.y)
		}
	}
}

func TestMapKey(t *testing.T) {
	tests := []struct {
		name string
		want bool // true = should map to something
	}{
		{"enter", true},
		{"tab", true},
		{"backspace", true},
		{"escape", true},
		{"space", true},
		{"dpad_up", true},
		{"arrow_down", true},
		{"home", true},
		{"end", true},
		{"dpad_left", true},
		{"dpad_right", true},
		{"page_up", true},
		{"page_down", true},
		{"delete", true},
		{"esc", true},
		{"up", true},
		{"down", true},
		{"left", true},
		{"right", true},
		{"back", true},
		{"arrow_up", true},
		{"arrow_left", true},
		{"arrow_right", true},
		{"unknown", false},
	}

	for _, tt := range tests {
		key := mapKey(tt.name)
		if tt.want && key == 0 {
			t.Errorf("mapKey(%q) should return a key", tt.name)
		}
		if !tt.want && key != 0 {
			t.Errorf("mapKey(%q) should return 0", tt.name)
		}
	}
}

func TestMapModifier(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"ctrl", true},
		{"control", true},
		{"shift", true},
		{"alt", true},
		{"option", true},
		{"meta", true},
		{"cmd", true},
		{"command", true},
		{"win", true},
		{"CTRL", true},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		got := mapModifier(tt.name)
		if tt.want && got == 0 {
			t.Errorf("mapModifier(%q) returned 0, expected a key", tt.name)
		}
		if !tt.want && got != 0 {
			t.Errorf("mapModifier(%q) returned %v, expected 0", tt.name, got)
		}
	}
}

func TestConvertToKeys(t *testing.T) {
	keys := convertToKeys("abc")
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != 'a' || keys[1] != 'b' || keys[2] != 'c' {
		t.Errorf("expected keys for 'abc', got %v", keys)
	}

	// Empty string
	keys = convertToKeys("")
	if len(keys) != 0 {
		t.Errorf("expected 0 keys for empty string, got %d", len(keys))
	}
}

func TestRandomGenerators(t *testing.T) {
	// randomString
	s := randomString(10)
	if len(s) != 10 {
		t.Errorf("randomString(10) length = %d, want 10", len(s))
	}

	// randomNumber
	n := randomNumber(5)
	if len(n) != 5 {
		t.Errorf("randomNumber(5) length = %d, want 5", len(n))
	}
	for _, c := range n {
		if c < '0' || c > '9' {
			t.Errorf("randomNumber contains non-digit: %c", c)
		}
	}

	// randomEmail
	e := randomEmail()
	if !strings.Contains(e, "@") || !strings.Contains(e, ".com") {
		t.Errorf("randomEmail() = %q, doesn't look like an email", e)
	}

	// randomPersonName
	name := randomPersonName()
	parts := strings.Split(name, " ")
	if len(parts) != 2 {
		t.Errorf("randomPersonName() = %q, expected 'First Last'", name)
	}
}

func TestCssEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with-dash", "with-dash"},
		{"with_underscore", "with_underscore"},
		{"has.dot", `has\.dot`},
		{"has:colon", `has\:colon`},
		{"has space", `has\ space`},
	}

	for _, tt := range tests {
		got := cssEscape(tt.input)
		if got != tt.want {
			t.Errorf("cssEscape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsOptional(t *testing.T) {
	tr := true
	fa := false

	if isOptional(nil) {
		t.Error("isOptional(nil) should be false")
	}
	if !isOptional(&tr) {
		t.Error("isOptional(&true) should be true")
	}
	if isOptional(&fa) {
		t.Error("isOptional(&false) should be false")
	}
}

func TestSetWaitForIdleTimeout(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	err := d.SetWaitForIdleTimeout(1000)
	if err != nil {
		t.Errorf("SetWaitForIdleTimeout should succeed: %v", err)
	}
}

func TestOpenBrowser(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.OpenBrowserStep{URL: ts.URL + "/other"})
	if !result.Success {
		t.Errorf("openBrowser should succeed: %s", result.Message)
	}

	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Other Page"},
	})
	if !result.Success {
		t.Errorf("should see other page after openBrowser: %s", result.Message)
	}
}

func TestScrollUntilVisible(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<h1>Top</h1>
			<div style="height:3000px"></div>
			<p id="bottom">Bottom Element</p>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Element is below the fold, need to scroll to find it
	result := d.Execute(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{CSS: "#bottom"},
		Direction: "down",
	})
	if !result.Success {
		t.Errorf("scrollUntilVisible should succeed: %s", result.Message)
	}

	// Test when element not found (should fail after max scrolls)
	result = d.Execute(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{CSS: "#nonexistent"},
		Direction: "down",
	})
	if result.Success {
		t.Error("scrollUntilVisible should fail for nonexistent element")
	}
}

func TestOpenBrowserInvalidURL(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Navigate to a chrome-error URL that triggers Navigate error
	result := d.Execute(&flow.OpenBrowserStep{URL: "chrome://crash"})
	if result.Success {
		t.Error("openBrowser with chrome://crash should fail")
	}
}

func TestAcceptAlertNoDialog(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// No dialog showing — should fail
	result := d.Execute(&flow.AcceptAlertStep{})
	if result.Success {
		t.Error("acceptAlert with no dialog should fail")
	}
}

func TestDismissAlertNoDialog(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// No dialog showing — should fail
	result := d.Execute(&flow.DismissAlertStep{})
	if result.Success {
		t.Error("dismissAlert with no dialog should fail")
	}
}

func TestAcceptAlert(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<button id="alert-btn" onclick="alert('Hello!')">Show Alert</button>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Trigger alert via setTimeout so it doesn't block
	d.page.MustEval(`() => setTimeout(() => alert('Test Alert'), 100)`)
	time.Sleep(300 * time.Millisecond)

	result := d.Execute(&flow.AcceptAlertStep{})
	if !result.Success {
		t.Errorf("acceptAlert should succeed: %s", result.Message)
	}
}

func TestDismissAlert(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<button id="confirm-btn" onclick="confirm('Are you sure?')">Confirm</button>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Trigger confirm dialog via setTimeout
	d.page.MustEval(`() => setTimeout(() => confirm('Test Confirm'), 100)`)
	time.Sleep(300 * time.Millisecond)

	result := d.Execute(&flow.DismissAlertStep{})
	if !result.Success {
		t.Errorf("dismissAlert should succeed: %s", result.Message)
	}
}

func TestInputTextNoSelector(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Focus input first
	d.Execute(&flow.TapOnStep{Selector: flow.Selector{CSS: "#input1"}})

	// Type without selector (uses keyboard)
	result := d.Execute(&flow.InputTextStep{Text: "test"})
	if !result.Success {
		t.Errorf("inputText without selector should succeed: %s", result.Message)
	}
}

func TestInputRandomAllTypes(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	types := []struct {
		dataType string
	}{
		{"TEXT"},
		{"NUMBER"},
		{"EMAIL"},
		{"PERSON_NAME"},
	}

	for _, tt := range types {
		// Focus input for each type
		d.Execute(&flow.TapOnStep{Selector: flow.Selector{CSS: "#input1"}})
		// Select all and clear
		d.page.Keyboard.MustType(input.Backspace)

		result := d.Execute(&flow.InputRandomStep{DataType: tt.dataType, Length: 5})
		if !result.Success {
			t.Errorf("inputRandom %s should succeed: %s", tt.dataType, result.Message)
		}
		if result.Data == nil {
			t.Errorf("inputRandom %s should return data", tt.dataType)
		}
	}
}

func TestWaitUntilNotVisible(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Wait for an element that doesn't exist to be not visible (should succeed)
	sel := flow.Selector{Text: "Nonexistent Text"}
	result := d.Execute(&flow.WaitUntilStep{
		BaseStep:   flow.BaseStep{TimeoutMs: 2000},
		NotVisible: &sel,
	})
	if !result.Success {
		t.Errorf("waitUntil notVisible for nonexistent element should succeed: %s", result.Message)
	}

	// Wait for a visible element to be not visible (should fail/timeout)
	sel2 := flow.Selector{Text: "Hello World"}
	result = d.Execute(&flow.WaitUntilStep{
		BaseStep:   flow.BaseStep{TimeoutMs: 1000},
		NotVisible: &sel2,
	})
	if result.Success {
		t.Error("waitUntil notVisible for visible element should fail")
	}
}

func TestStateFiltersEnabled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<button id="enabled-btn">Enabled</button>
			<button id="disabled-btn" disabled>Disabled</button>
			<input id="checked-cb" type="checkbox" checked>
			<input id="unchecked-cb" type="checkbox">
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	tr := true
	fa := false

	// Find enabled button with enabled=true filter
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "#enabled-btn", Enabled: &tr},
	})
	if !result.Success {
		t.Errorf("enabled button with enabled=true should be found: %s", result.Message)
	}

	// Find enabled button with enabled=false filter (should fail)
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{CSS: "#enabled-btn", Enabled: &fa},
	})
	if result.Success {
		t.Error("enabled button with enabled=false filter should not be found")
	}

	// Find disabled button with enabled=false filter
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "#disabled-btn", Enabled: &fa},
	})
	if !result.Success {
		t.Errorf("disabled button with enabled=false should be found: %s", result.Message)
	}

	// Find disabled button with enabled=true filter (should fail)
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{CSS: "#disabled-btn", Enabled: &tr},
	})
	if result.Success {
		t.Error("disabled button with enabled=true filter should not be found")
	}
}

func TestStateFiltersChecked(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<input id="checked-cb" type="checkbox" checked>
			<input id="unchecked-cb" type="checkbox">
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	tr := true
	fa := false

	// Find checked checkbox with checked=true
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "#checked-cb", Checked: &tr},
	})
	if !result.Success {
		t.Errorf("checked checkbox with checked=true should be found: %s", result.Message)
	}

	// Find checked checkbox with checked=false (should fail)
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{CSS: "#checked-cb", Checked: &fa},
	})
	if result.Success {
		t.Error("checked checkbox with checked=false should not be found")
	}

	// Find unchecked checkbox with checked=false
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "#unchecked-cb", Checked: &fa},
	})
	if !result.Success {
		t.Errorf("unchecked checkbox with checked=false should be found: %s", result.Message)
	}
}

func TestStateFiltersFocused(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	tr := true
	fa := false

	// Focus input first
	d.Execute(&flow.TapOnStep{Selector: flow.Selector{CSS: "#input1"}})

	// Find focused input with focused=true
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "#input1", Focused: &tr},
	})
	if !result.Success {
		t.Errorf("focused input with focused=true should be found: %s", result.Message)
	}

	// Find unfocused button with focused=false
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "#btn", Focused: &fa},
	})
	if !result.Success {
		t.Errorf("unfocused button with focused=false should be found: %s", result.Message)
	}
}

func TestFindByIDViaName(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<input name="username" type="text" value="test">
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Should find via name attribute
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{ID: "username"},
	})
	if !result.Success {
		t.Errorf("find by name 'username' should succeed: %s", result.Message)
	}
}

func TestFindByIDViaAriaLabel(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Should find via aria-label
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{ID: "Click Me"},
	})
	if !result.Success {
		t.Errorf("find by aria-label 'Click Me' should succeed: %s", result.Message)
	}
}

func TestSwipeInvalidDirection(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.SwipeStep{Direction: "diagonal"})
	if result.Success {
		t.Error("swipe with invalid direction should fail")
	}
}

func TestLaunchAppEmptyURL(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.LaunchAppStep{AppID: ""})
	if result.Success {
		t.Error("launchApp with empty URL should fail")
	}
}

func TestLaunchAppWithClearState(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.LaunchAppStep{
		AppID:      ts.URL,
		ClearState: true,
	})
	if !result.Success {
		t.Errorf("launchApp with clearState should succeed: %s", result.Message)
	}
}

func TestPasteTextEmptyClipboard(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Clipboard starts empty
	result := d.Execute(&flow.PasteTextStep{})
	if result.Success {
		t.Error("pasteText with empty clipboard should fail")
	}
}

func TestEraseTextDefaultCharacters(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Focus input
	d.Execute(&flow.TapOnStep{Selector: flow.Selector{CSS: "#input1"}})

	// Erase with 0 characters (should default to 50)
	result := d.Execute(&flow.EraseTextStep{Characters: 0})
	if !result.Success {
		t.Errorf("eraseText with default characters should succeed: %s", result.Message)
	}
}

func TestCalculateTimeout(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// With step timeout
	timeout := d.calculateTimeout(false, 3000)
	if timeout != 3*time.Second {
		t.Errorf("calculateTimeout with stepTimeout=3000 should return 3s, got %v", timeout)
	}

	// Optional
	timeout = d.calculateTimeout(true, 0)
	if timeout != time.Duration(optionalFindTimeoutMs)*time.Millisecond {
		t.Errorf("calculateTimeout optional should return %dms, got %v", optionalFindTimeoutMs, timeout)
	}

	// Default
	timeout = d.calculateTimeout(false, 0)
	if timeout != time.Duration(d.findTimeoutMs)*time.Millisecond {
		t.Errorf("calculateTimeout default should return %dms, got %v", d.findTimeoutMs, timeout)
	}
}

func TestFindElementOnceNoSelector(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// No selector specified
	_, _, err := d.findElementOnce(flow.Selector{})
	if err == nil {
		t.Error("findElementOnce with empty selector should fail")
	}
}

func TestGetStatePortrait(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d, err := New(Config{
		Headless:  true,
		URL:       ts.URL,
		ViewportW: 768,
		ViewportH: 1024,
	})
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}
	defer d.Close()

	state := d.GetState()
	if state.Orientation != "portrait" {
		t.Errorf("expected orientation 'portrait', got %q", state.Orientation)
	}
}

func TestScrollDefaultDirection(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Empty direction defaults to "down"
	result := d.Execute(&flow.ScrollStep{Direction: ""})
	if !result.Success {
		t.Errorf("scroll with empty direction should succeed: %s", result.Message)
	}
}

func TestScrollLeftRight(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.ScrollStep{Direction: "left"})
	if !result.Success {
		t.Errorf("scroll left should succeed: %s", result.Message)
	}

	result = d.Execute(&flow.ScrollStep{Direction: "right"})
	if !result.Success {
		t.Errorf("scroll right should succeed: %s", result.Message)
	}
}

func TestCryptoRandIntn(t *testing.T) {
	// Test basic functionality
	for i := 0; i < 100; i++ {
		n := cryptoRandIntn(10)
		if n < 0 || n >= 10 {
			t.Errorf("cryptoRandIntn(10) = %d, want [0,10)", n)
		}
	}
}

func TestStateFiltersWithTextSelector(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<button id="enabled-btn" aria-label="Submit">Submit</button>
			<button id="disabled-btn" disabled aria-label="Cancel">Cancel</button>
			<input id="cb1" type="checkbox" checked aria-label="Accept Terms">
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	tr := true
	fa := false

	// Find enabled button via text + enabled filter (exercises AX tree path with state filters)
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Submit", Enabled: &tr},
	})
	if !result.Success {
		t.Errorf("text 'Submit' with enabled=true should be found: %s", result.Message)
	}

	// Disabled button with enabled=false via text
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Cancel", Enabled: &fa},
	})
	if !result.Success {
		t.Errorf("text 'Cancel' with enabled=false should be found: %s", result.Message)
	}

	// Checkbox with checked=true via text — exercises AX tree checked filter
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Accept Terms", Checked: &tr},
	})
	if !result.Success {
		t.Errorf("text 'Accept Terms' with checked=true should be found: %s", result.Message)
	}
}

func TestFindByTextJSFallback(t *testing.T) {
	// Create a page where the element won't be found via AX tree or Search
	// by using a data attribute that only the JS helper can find via textContent
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Use a span with unique text that's nested deep — the JS helper
		// finds deepest matching element. Also ensure the AX tree won't
		// give it a standard accessible name by using aria-hidden on an ancestor
		// and role="presentation" to make it non-semantic.
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<div>
				<span>Some visible text for JS fallback</span>
			</div>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// This text should be found through the cascade (AX tree, then Search, then JS)
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Some visible text for JS fallback"},
	})
	if !result.Success {
		t.Errorf("should find text through cascade: %s", result.Message)
	}
}

func TestSelectedStateFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Use size=2 to make options visible (not in a dropdown)
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<select id="sel" size="2">
				<option id="opt-a" value="a" selected>Option A</option>
				<option id="opt-b" value="b">Option B</option>
			</select>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	tr := true
	fa := false

	// Selected option with selected=true
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "#opt-a", Selected: &tr},
	})
	if !result.Success {
		t.Errorf("selected option with selected=true should be found: %s", result.Message)
	}

	// Unselected option with selected=false
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "#opt-b", Selected: &fa},
	})
	if !result.Success {
		t.Errorf("unselected option with selected=false should be found: %s", result.Message)
	}
}

func TestInputTextSelectorNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Non-existent selector — covers the error return path
	result := d.Execute(&flow.InputTextStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Text:     "hello",
		Selector: flow.Selector{CSS: "#nonexistent"},
	})
	if result.Success {
		t.Error("inputText with non-existent selector should fail")
	}
}

func TestInputTextNoSelectorInsertTextFallback(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Focus input first
	d.Execute(&flow.TapOnStep{Selector: flow.Selector{CSS: "#input1"}})

	// Use emoji/unicode that Keyboard.Type may not handle cleanly
	// Even if Type() succeeds, this tests the no-selector path thoroughly
	result := d.Execute(&flow.InputTextStep{Text: "hello world 123"})
	if !result.Success {
		t.Errorf("inputText without selector should succeed: %s", result.Message)
	}
}

func TestStateFiltersFromAXNodeFocused(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<input id="search" type="text" aria-label="Search Input" placeholder="Search">
			<button id="go-btn">Go</button>
			<script>document.getElementById('search').focus();</script>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	tr := true
	fa := false

	// Find the focused input via text selector with focused=true
	// This exercises matchesStateFiltersFromAXNode focused branch
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Search Input", Focused: &tr},
	})
	if !result.Success {
		t.Errorf("focused input with focused=true via text should be found: %s", result.Message)
	}

	// The unfocused button with focused=false via text
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Go", Focused: &fa},
	})
	if !result.Success {
		t.Errorf("unfocused button with focused=false via text should be found: %s", result.Message)
	}
}

func TestStateFiltersFromAXNodeSelected(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<div role="tablist">
				<button role="tab" aria-selected="true" id="tab1">Tab One</button>
				<button role="tab" aria-selected="false" id="tab2">Tab Two</button>
			</div>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	tr := true
	fa := false

	// Find selected tab via text with selected=true
	// This exercises matchesStateFiltersFromAXNode selected branch
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Tab One", Selected: &tr},
	})
	if !result.Success {
		t.Errorf("selected tab with selected=true via text should be found: %s", result.Message)
	}

	// Find unselected tab via text with selected=false
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Tab Two", Selected: &fa},
	})
	if !result.Success {
		t.Errorf("unselected tab with selected=false via text should be found: %s", result.Message)
	}
}

func TestScrollUntilVisibleUpDirection(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<p id="top">Top Element</p>
			<div style="height:3000px"></div>
			<p id="bottom">Bottom</p>
			<script>window.scrollTo(0, 3000);</script>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Scroll up to find the top element
	result := d.Execute(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{CSS: "#top"},
		Direction: "up",
	})
	if !result.Success {
		t.Errorf("scrollUntilVisible up should succeed: %s", result.Message)
	}
}

// --- Direct internal method tests to cover error paths ---

func TestFindByJSDirect(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Success: find existing text via JS
	_, info, err := d.findByJS("Hello World", flow.Selector{Text: "Hello World"})
	if err != nil {
		t.Errorf("findByJS should find 'Hello World': %v", err)
	}
	if info == nil {
		t.Error("findByJS should return element info")
	}

	// Error: text not found — JS throws
	_, _, err = d.findByJS("NONEXISTENT_TEXT_XYZ_999", flow.Selector{Text: "NONEXISTENT_TEXT_XYZ_999"})
	if err == nil {
		t.Error("findByJS should fail for nonexistent text")
	}

	// State filter mismatch: find button but require enabled=false
	fa := false
	_, _, err = d.findByJS("Click Me", flow.Selector{Text: "Click Me", Enabled: &fa})
	if err == nil {
		t.Error("findByJS should fail when state filter doesn't match")
	}
}

func TestFindBySearchDirect(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Success: find existing text via Search
	_, info, err := d.findBySearch("Hello World", flow.Selector{Text: "Hello World"})
	if err != nil {
		t.Errorf("findBySearch should find 'Hello World': %v", err)
	}
	if info == nil {
		t.Error("findBySearch should return element info")
	}

	// State filter mismatch: find button but require enabled=false
	fa := false
	_, _, err = d.findBySearch("Click Me", flow.Selector{Text: "Click Me", Enabled: &fa})
	if err == nil {
		t.Error("findBySearch should fail when state filter doesn't match")
	}
}

func TestFindByAXTreeHiddenElement(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<button style="display:none" aria-label="Hidden Button">Hidden Button</button>
			<button aria-label="Visible Button">Visible Button</button>
			<button disabled aria-label="Only Disabled">Only Disabled</button>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Should find the visible button, skipping the hidden one (covers !visible branch)
	_, info, err := d.findByAXTree("Visible Button", "button", flow.Selector{Text: "Visible Button"})
	if err != nil {
		t.Errorf("findByAXTree should find visible button: %v", err)
	}
	if info == nil || !info.Visible {
		t.Error("findByAXTree should return visible element info")
	}

	// Should fail for text with no AX nodes
	_, _, err = d.findByAXTree("NONEXISTENT_AX_TEXT_999", "", flow.Selector{Text: "NONEXISTENT_AX_TEXT_999"})
	if err == nil {
		t.Error("findByAXTree should fail for nonexistent text")
	}

	// State filter mismatch: button is disabled but we require enabled=true
	// AX tree has "disabled" property, so matchesStateFiltersFromAXNode returns false
	tr := true
	_, _, err = d.findByAXTree("Only Disabled", "button", flow.Selector{Text: "Only Disabled", Enabled: &tr})
	if err == nil {
		t.Error("findByAXTree should fail when AX state filter doesn't match")
	}
}

// TestFindByAXTreeSkipsNonRenderableTags is the 1.1.15 regression guard
// for saucedemo/demoblaze where SPAs put the route label into
// document.title — the AX tree's accessible-name search surfaced
// <title>, the actionability gate then rejected it because <title>
// has display:none. The finder now skips non-renderable tags
// (<title>, <script>, <style>, etc.) so the cascade can reach the
// actual visible button.
func TestFindByAXTreeSkipsNonRenderableTags(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// document.title shares text with the on-page button.
		// Pre-1.1.15 the AX-tree finder could surface <title>; post-fix
		// it must skip <title> and return the button.
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Back to products</title></head>
<body>
  <button aria-label="Back to products">Back to products</button>
</body>
</html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	_, info, err := d.findByAXTree("Back to products", "button", flow.Selector{Text: "Back to products"})
	if err != nil {
		t.Fatalf("findByAXTree should find the button despite <title> sharing the same text: %v", err)
	}
	if info == nil || !info.Visible {
		t.Fatal("expected the resolved element to be the visible button")
	}
}

// TestIsNonRenderableTagDirect exercises the helper directly across the
// tag set we want to filter out, plus a control case.
func TestIsNonRenderableTagDirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
  <title id="t">Doc Title</title>
  <script id="s" type="application/ld+json">{"name":"x"}</script>
  <style id="y">body{color:red}</style>
  <meta id="m" name="x" content="y">
  <link id="l" rel="icon" href="data:,">
</head>
<body>
  <button id="b">Visible</button>
</body>
</html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	type c struct {
		selector string
		want     bool
	}
	for _, tt := range []c{
		{"#t", true},   // <title>
		{"#s", true},   // <script>
		{"#y", true},   // <style>
		{"#m", true},   // <meta>
		{"#l", true},   // <link>
		{"#b", false},  // <button> — should be renderable
		{"body", false},
	} {
		elem, err := d.page.Element(tt.selector)
		if err != nil {
			t.Errorf("could not find %s: %v", tt.selector, err)
			continue
		}
		if got := isNonRenderableTag(elem); got != tt.want {
			t.Errorf("isNonRenderableTag(%s) = %v, want %v", tt.selector, got, tt.want)
		}
	}
}

// TestAXNodeToElementWalksUpTextNode exercises the path where the AX
// tree returns a #text node handle: axNodeToElement must walk up to
// the parent Element so callers can dispatch real clicks on it.
func TestAXNodeToElementWalksUpTextNode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Button whose accessible name comes from a child text node.
		// Without the walk-up, findByAXTree could return the #text
		// handle, which Rod's Click() refuses.
		fmt.Fprint(w, `<!DOCTYPE html><html><body><button id="b">Press me</button></body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	_, info, err := d.findByAXTree("Press me", "button", flow.Selector{Text: "Press me"})
	if err != nil {
		t.Fatalf("findByAXTree: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil ElementInfo")
	}
	// We can't assert the underlying tagName from ElementInfo, but we can
	// verify the bounds are non-zero, which would not be the case if a
	// #text handle leaked through (a text node has zero own bounds in
	// most layout situations).
	if info.Bounds.Width == 0 || info.Bounds.Height == 0 {
		t.Errorf("expected non-zero bounds (Element handle); got %+v", info.Bounds)
	}
}

func TestNewInvalidChromeBin(t *testing.T) {
	_, err := New(Config{
		Headless:  true,
		ChromeBin: "/nonexistent/path/to/chrome",
	})
	if err == nil {
		t.Error("New with invalid ChromeBin should fail")
	}
}

func TestCloseNilBrowser(t *testing.T) {
	d := &Driver{
		stopCh: make(chan struct{}),
	}
	err := d.Close()
	if err != nil {
		t.Errorf("Close with nil browser should return nil: %v", err)
	}
}

func TestCloseIdempotent(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)

	// First close should succeed
	if err := d.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	// Second close should not panic and return the same result
	if err := d.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestCloseNilBrowserIdempotent(t *testing.T) {
	d := &Driver{
		stopCh: make(chan struct{}),
	}
	if err := d.Close(); err != nil {
		t.Fatalf("first Close with nil browser failed: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second Close with nil browser failed: %v", err)
	}
}

func TestSetLocationInvalidCoords(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Invalid latitude
	result := d.Execute(&flow.SetLocationStep{
		Latitude:  "not-a-number",
		Longitude: "-122.4194",
	})
	if result.Success {
		t.Error("setLocation with invalid latitude should fail")
	}

	// Invalid longitude
	result = d.Execute(&flow.SetLocationStep{
		Latitude:  "37.7749",
		Longitude: "not-a-number",
	})
	if result.Success {
		t.Error("setLocation with invalid longitude should fail")
	}
}

func TestParsePercentageCoordsInvalidY(t *testing.T) {
	// Invalid Y coordinate (X is valid but Y is not)
	_, _, err := parsePercentageCoords("50%, abc")
	if err == nil {
		t.Error("parsePercentageCoords with invalid Y should fail")
	}
}

func TestTapOnNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.TapOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{Text: "NONEXISTENT_BUTTON_XYZ"},
	})
	if result.Success {
		t.Error("tapOn with nonexistent element should fail")
	}
}

func TestDoubleTapOnNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.DoubleTapOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{Text: "NONEXISTENT_XYZ"},
	})
	if result.Success {
		t.Error("doubleTapOn with nonexistent element should fail")
	}
}

func TestLongPressOnNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.LongPressOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{Text: "NONEXISTENT_XYZ"},
	})
	if result.Success {
		t.Error("longPressOn with nonexistent element should fail")
	}
}

func TestCopyTextFromNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.CopyTextFromStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{CSS: "#nonexistent"},
	})
	if result.Success {
		t.Error("copyTextFrom with nonexistent element should fail")
	}
}

func TestAssertVisibleNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 1000},
		Selector: flow.Selector{CSS: "#nonexistent"},
	})
	if result.Success {
		t.Error("assertVisible with nonexistent element should fail")
	}
}

func TestFindByIDNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	_, _, err := d.findByID(flow.Selector{ID: "COMPLETELY_NONEXISTENT_ID_XYZ"})
	if err == nil {
		t.Error("findByID should fail for nonexistent ID")
	}

	// State filter mismatch: find button by aria-label but require disabled
	fa := false
	_, _, err = d.findByID(flow.Selector{ID: "Click Me", Enabled: &fa})
	if err == nil {
		t.Error("findByID should fail when state filter doesn't match")
	}
}

func TestFindByCSSStateFilterMismatch(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	fa := false
	_, _, err := d.findByCSS(flow.Selector{CSS: "#btn", Enabled: &fa})
	if err == nil {
		t.Error("findByCSS should fail when state filter doesn't match")
	}
}

func TestFindByTextNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Text that doesn't exist anywhere — exercises all 3 cascade stages failing
	_, _, err := d.findByText(flow.Selector{Text: "ZZZZZ_NONEXISTENT_TEXT_99999"})
	if err == nil {
		t.Error("findByText should fail for nonexistent text")
	}
}

func TestExecuteUnknownStepType(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Use an unregistered step type to cover the default case
	result := d.Execute(&unknownStep{})
	if result.Success {
		t.Error("Execute with unknown step type should fail")
	}
	if !strings.Contains(result.Message, "not supported") {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

// unknownStep implements flow.Step for testing the default Execute branch.
type unknownStep struct{}

func (s *unknownStep) Type() flow.StepType { return "unknown" }
func (s *unknownStep) IsOptional() bool    { return false }
func (s *unknownStep) Label() string       { return "" }
func (s *unknownStep) Describe() string    { return "unknown step" }

func TestWaitUntilTimeout(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Wait with default timeout=0 (should use 30000 default, but we override with step timeout)
	sel := flow.Selector{Text: "NONEXISTENT_XYZ_WAIT"}
	result := d.Execute(&flow.WaitUntilStep{
		BaseStep: flow.BaseStep{TimeoutMs: 500},
		Visible:  &sel,
	})
	if result.Success {
		t.Error("waitUntil should fail for nonexistent element")
	}
}

func TestAssertVisibleHiddenElement(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Element exists but is display:none — covers the !info.Visible branch
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: "#hidden"},
	})
	if result.Success {
		t.Error("assertVisible on hidden element should fail")
	}
	if !strings.Contains(result.Message, "not visible") {
		t.Errorf("expected 'not visible' message, got: %s", result.Message)
	}
}

func TestTapOnPointInvalidPercentage(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.TapOnPointStep{Point: "invalid"})
	if result.Success {
		t.Error("tapOnPoint with invalid percentage should fail")
	}
}

func TestInputRandomDefaultLength(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	d.Execute(&flow.TapOnStep{Selector: flow.Selector{CSS: "#input1"}})

	// Length=0 should default to 10
	result := d.Execute(&flow.InputRandomStep{DataType: "TEXT", Length: 0})
	if !result.Success {
		t.Errorf("inputRandom with default length should succeed: %s", result.Message)
	}
	text, ok := result.Data.(string)
	if !ok {
		t.Fatal("inputRandom should return string data")
	}
	if len(text) != 10 {
		t.Errorf("inputRandom default length should be 10, got %d", len(text))
	}
}

// newSelectorTestDriver creates a driver pointed at the /selectors page.
func newSelectorTestDriver(t *testing.T, ts *httptest.Server) *Driver {
	t.Helper()
	d, err := New(Config{
		Headless:  true,
		URL:       ts.URL + "/selectors",
		ViewportW: 1024,
		ViewportH: 768,
	})
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}
	d.SetFindTimeout(3000)
	return d
}

func TestFindByTestID(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	// Find by testId
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{TestID: "email-input"},
	})
	if !result.Success {
		t.Errorf("find by testId should succeed: %s", result.Message)
	}

	// testId not found
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{TestID: "nonexistent"},
	})
	if result.Success {
		t.Error("find by nonexistent testId should fail")
	}
}

func TestFindByPlaceholder(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Placeholder: "Enter email"},
	})
	if !result.Success {
		t.Errorf("find by placeholder should succeed: %s", result.Message)
	}

	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Placeholder: "Search..."},
	})
	if !result.Success {
		t.Errorf("find by placeholder 'Search...' should succeed: %s", result.Message)
	}
}

func TestFindByRole(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	// Find by role alone
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Role: "link"},
	})
	if !result.Success {
		t.Errorf("find by role 'link' should succeed: %s", result.Message)
	}

	// Find by role + text
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Role: "link", Text: "About Us"},
	})
	if !result.Success {
		t.Errorf("find by role 'link' + text 'About Us' should succeed: %s", result.Message)
	}
}

func TestFindByHref(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	// Exact match
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Href: "/about"},
	})
	if !result.Success {
		t.Errorf("find by href '/about' should succeed: %s", result.Message)
	}

	// Partial match
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Href: "contact"},
	})
	if !result.Success {
		t.Errorf("find by href partial 'contact' should succeed: %s", result.Message)
	}
}

func TestFindByAlt(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Alt: "Company Logo"},
	})
	if !result.Success {
		t.Errorf("find by alt should succeed: %s", result.Message)
	}
}

func TestFindByTitle(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Title: "About us page"},
	})
	if !result.Success {
		t.Errorf("find by title should succeed: %s", result.Message)
	}
}

func TestFindByName(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Name: "user_email"},
	})
	if !result.Success {
		t.Errorf("find by name should succeed: %s", result.Message)
	}
}

func TestFindByTextContains(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{TextContains: "Welcome"},
	})
	if !result.Success {
		t.Errorf("find by textContains should succeed: %s", result.Message)
	}
}

func TestFindByTextRegex(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	// Match order number pattern
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{TextRegex: `Order #\d+ confirmed`},
	})
	if !result.Success {
		t.Errorf("find by textRegex should succeed: %s", result.Message)
	}

	// Invalid regex should fail
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{TextRegex: `[invalid`},
	})
	if result.Success {
		t.Error("find by invalid textRegex should fail")
	}
}

func TestFindByNth(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	// Find second .item (nth=1, 0-based)
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{CSS: ".item", Nth: 1},
	})
	if !result.Success {
		t.Errorf("find by CSS with nth=1 should succeed: %s", result.Message)
	}

	// nth out of range
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{CSS: ".item", Nth: 10},
	})
	if result.Success {
		t.Error("find by CSS with nth=10 should fail (only 3 items)")
	}
}

func TestFindByTapOnNewSelectors(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	// Tap on element found by placeholder
	result := d.Execute(&flow.TapOnStep{
		Selector: flow.Selector{Placeholder: "Enter email"},
	})
	if !result.Success {
		t.Errorf("tapOn by placeholder should succeed: %s", result.Message)
	}

	// Tap on element found by testId
	result = d.Execute(&flow.TapOnStep{
		Selector: flow.Selector{TestID: "submit-btn"},
	})
	if !result.Success {
		t.Errorf("tapOn by testId should succeed: %s", result.Message)
	}
}

func TestUnsupportedFieldsWarning(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	// Using width on web should log a warning but still try to find element
	// The selector itself should work based on the supported fields
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "Submit Form", Width: 100},
	})
	if !result.Success {
		t.Errorf("should still find element despite unsupported width field: %s", result.Message)
	}

	// Verify warnedFields was populated
	if !d.warnedFields["width"] {
		t.Error("expected 'width' to be in warnedFields")
	}
}

func TestFindByAttributeNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	// testId not found
	result := d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{Placeholder: "nonexistent-placeholder"},
	})
	if result.Success {
		t.Error("find by nonexistent placeholder should fail")
	}

	// name not found
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{Name: "nonexistent-name"},
	})
	if result.Success {
		t.Error("find by nonexistent name should fail")
	}

	// alt not found
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{Alt: "nonexistent-alt"},
	})
	if result.Success {
		t.Error("find by nonexistent alt should fail")
	}

	// title not found
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{Title: "nonexistent-title"},
	})
	if result.Success {
		t.Error("find by nonexistent title should fail")
	}
}

func TestFindByHrefNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	result := d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{Href: "/nonexistent-page-xyz"},
	})
	if result.Success {
		t.Error("find by nonexistent href should fail")
	}
}

func TestFindByTextContainsNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	result := d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{TextContains: "xyzNonexistentText123"},
	})
	if result.Success {
		t.Error("find by nonexistent textContains should fail")
	}
}

func TestFindByTextRegexNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	result := d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{TextRegex: `^xyzNonexistent\d{99}$`},
	})
	if result.Success {
		t.Error("find by nonexistent textRegex should fail")
	}
}

func TestFindByNthWithAttribute(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	// Find by testId with nth=0 (first, same as default)
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{TestID: "email-input"},
	})
	if !result.Success {
		t.Errorf("find by testId without nth should succeed: %s", result.Message)
	}
}

func TestFindByRoleNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	result := d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{Role: "slider"},
	})
	if result.Success {
		t.Error("find by nonexistent role should fail")
	}
}

// --- Tests for findByCSSWithNth via attribute selectors ---

func TestFindByCSSWithNthViaAttribute(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<input type="text" placeholder="Enter text" value="first">
			<input type="text" placeholder="Enter text" value="second">
			<input type="text" placeholder="Enter text" value="third">
			<input type="text" placeholder="Enter text" disabled value="fourth">
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// nth=1 via attribute selector — covers findByCSSWithNth nth branch
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Placeholder: "Enter text", Nth: 1},
	})
	if !result.Success {
		t.Errorf("placeholder with nth=1 should succeed: %s", result.Message)
	}

	// nth=2 to get the third element
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Placeholder: "Enter text", Nth: 2},
	})
	if !result.Success {
		t.Errorf("placeholder with nth=2 should succeed: %s", result.Message)
	}

	// nth out of range — covers "nth=%d but only %d elements found"
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{Placeholder: "Enter text", Nth: 10},
	})
	if result.Success {
		t.Error("placeholder with nth=10 should fail")
	}

	// nth=3 targets the disabled input, with enabled=true — covers state filter mismatch in nth branch
	tr := true
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{Placeholder: "Enter text", Nth: 3, Enabled: &tr},
	})
	if result.Success {
		t.Error("placeholder nth=3 with enabled=true should fail (4th input is disabled)")
	}
}

func TestFindByHrefWithNth(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<a href="/page">Link One</a>
			<a href="/page">Link Two</a>
			<a href="/page">Link Three</a>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// nth=1 via href exact match — covers findByCSSWithNth nth path through findByHref
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Href: "/page", Nth: 1},
	})
	if !result.Success {
		t.Errorf("href with nth=1 should succeed: %s", result.Message)
	}
}

// --- Tests for resolveAXNodes with nth and state filters ---

func TestResolveAXNodesWithNth(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	// textContains "Welcome" matches two paragraphs, use nth=1 to get the second
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{TextContains: "Welcome", Nth: 1},
	})
	if !result.Success {
		t.Errorf("textContains with nth=1 should succeed: %s", result.Message)
	}

	// nth out of range via resolveAXNodes
	result = d.Execute(&flow.AssertVisibleStep{
		BaseStep: flow.BaseStep{Optional: true, TimeoutMs: 1000},
		Selector: flow.Selector{TextContains: "Welcome", Nth: 10},
	})
	if result.Success {
		t.Error("textContains with nth=10 should fail")
	}
}

func TestTextRegexWithNth(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newSelectorTestDriver(t, ts)
	defer d.Close()

	// textRegex matching "Welcome" with nth=1 — resolveAXNodes nth path for textRegex
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{TextRegex: `Welcome`, Nth: 1},
	})
	if !result.Success {
		t.Errorf("textRegex with nth=1 should succeed: %s", result.Message)
	}
}

func TestResolveAXNodesStateFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<button>Action Button</button>
			<button disabled>Action Button</button>
		</body></html>`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	tr := true

	// textContains with enabled=true state filter — resolveAXNodes should skip disabled match
	result := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{TextContains: "Action", Enabled: &tr},
	})
	if !result.Success {
		t.Errorf("textContains with enabled=true should find enabled button: %s", result.Message)
	}

	// Disabled button should only be found with enabled=false
	fa := false
	result = d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{TextContains: "Action", Enabled: &fa},
	})
	if !result.Success {
		t.Errorf("textContains with enabled=false should find disabled button: %s", result.Message)
	}
}

// --- Direct tests for JS fallback functions ---

func TestFindByJSTextContainsDirect(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Success: find existing text via JS textContent contains
	_, info, err := d.findByJSTextContains("paragraph text", flow.Selector{TextContains: "paragraph text"})
	if err != nil {
		t.Errorf("findByJSTextContains should succeed: %v", err)
	}
	if info == nil {
		t.Error("should return element info")
	}

	// Not found — covers "JS textContains failed" error path
	_, _, err = d.findByJSTextContains("XYZNONEXISTENT999", flow.Selector{TextContains: "XYZNONEXISTENT999"})
	if err == nil {
		t.Error("findByJSTextContains should fail for nonexistent text")
	}

	// State filter mismatch — covers "state filters don't match" path
	fa := false
	_, _, err = d.findByJSTextContains("Click Me", flow.Selector{TextContains: "Click Me", Enabled: &fa})
	if err == nil {
		t.Error("findByJSTextContains should fail when state filter doesn't match")
	}
}

func TestFindByJSTextRegexDirect(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Success: find existing text via JS regex
	re := regexp.MustCompile(`paragraph`)
	_, info, err := d.findByJSTextRegex(`paragraph`, re, flow.Selector{TextRegex: `paragraph`})
	if err != nil {
		t.Errorf("findByJSTextRegex should succeed: %v", err)
	}
	if info == nil {
		t.Error("should return element info")
	}

	// Not found — covers "JS textRegex failed" error path
	reNone := regexp.MustCompile(`^XYZNONEXISTENT\d+$`)
	_, _, err = d.findByJSTextRegex(`^XYZNONEXISTENT\d+$`, reNone, flow.Selector{TextRegex: `^XYZNONEXISTENT\d+$`})
	if err == nil {
		t.Error("findByJSTextRegex should fail for nonexistent pattern")
	}

	// State filter mismatch — covers "state filters don't match" path
	fa := false
	reClick := regexp.MustCompile(`Click Me`)
	_, _, err = d.findByJSTextRegex(`Click Me`, reClick, flow.Selector{TextRegex: `Click Me`, Enabled: &fa})
	if err == nil {
		t.Error("findByJSTextRegex should fail when state filter doesn't match")
	}
}

// --- Test findByCSSWithNth state filter in non-nth path ---

func TestFindByCSSWithNthStateFilterNoNth(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// State filter mismatch via attribute selector (no nth) — covers non-nth state filter path
	fa := false
	_, _, err := d.findByAttribute("id", "btn", flow.Selector{Name: "btn", Enabled: &fa})
	if err == nil {
		t.Error("findByAttribute should fail when state filter doesn't match")
	}
}

// --- Tests for looksLikeRegex ---

func TestLooksLikeRegex(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Should detect regex
		{"Hello.*", true},
		{"foo.+bar", true},
		{"test.?end", true},
		{"a|b", true},
		{"(group)", true},
		{"[abc]", true},
		{"{3}", true},
		{"^Start", true},
		{"End$", true},
		{"foo*bar", true},
		{"foo+bar", true},
		{"foo?bar", true},

		// Should NOT detect regex
		{"Hello World", false},
		{"simple text", false},
		{"no.special", false},    // dot without quantifier
		{"a.b", false},           // dot without quantifier
		{"middle^caret", false},  // caret not at start
		{"dollar$middle", false}, // dollar not at end
		{"escaped\\.dot", false}, // escaped dot with quantifier (backslash prevents detection)
		{"", false},              // empty string
		{"abc123", false},        // alphanumeric only
		{"hello, world!", false}, // normal punctuation
	}

	for _, tt := range tests {
		got := looksLikeRegex(tt.input)
		if got != tt.want {
			t.Errorf("looksLikeRegex(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// --- Tests for jsSelectorTypeValue ---

func TestJsSelectorTypeValue(t *testing.T) {
	tests := []struct {
		name      string
		selector  flow.Selector
		wantType  string
		wantValue string
	}{
		{"css", flow.Selector{CSS: "#btn"}, "css", "#btn"},
		{"testId", flow.Selector{TestID: "submit"}, "testId", "submit"},
		{"name", flow.Selector{Name: "email"}, "name", "email"},
		{"placeholder", flow.Selector{Placeholder: "Search"}, "placeholder", "Search"},
		{"href", flow.Selector{Href: "/about"}, "href", "/about"},
		{"alt", flow.Selector{Alt: "Logo"}, "alt", "Logo"},
		{"title", flow.Selector{Title: "Info"}, "title", "Info"},
		{"role", flow.Selector{Role: "button"}, "role", "button"},
		{"id", flow.Selector{ID: "main"}, "id", "main"},
		{"textRegex", flow.Selector{TextRegex: "Hello.*"}, "textRegex", "Hello.*"},
		{"textContains", flow.Selector{TextContains: "Welcome"}, "textContains", "Welcome"},
		{"text plain", flow.Selector{Text: "Click Me"}, "text", "Click Me"},
		{"text looks like regex", flow.Selector{Text: "Hello.*"}, "textRegex", "Hello.*"},
		{"empty selector", flow.Selector{}, "text", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotValue := jsSelectorTypeValue(tt.selector)
			if gotType != tt.wantType {
				t.Errorf("jsSelectorTypeValue() type = %q, want %q", gotType, tt.wantType)
			}
			if gotValue != tt.wantValue {
				t.Errorf("jsSelectorTypeValue() value = %q, want %q", gotValue, tt.wantValue)
			}
		})
	}
}

// --- Tests for needsDownload ---

func TestNeedsDownload(t *testing.T) {
	// Non-existent directory
	if !needsDownload("/nonexistent/path/xyz") {
		t.Error("needsDownload should return true for non-existent directory")
	}

	// Empty directory
	dir := t.TempDir()
	if !needsDownload(dir) {
		t.Error("needsDownload should return true for empty directory")
	}

	// Directory with files
	if err := os.WriteFile(dir+"/somefile", []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if needsDownload(dir) {
		t.Error("needsDownload should return false for non-empty directory")
	}
}

// --- Tests for EnsureBrowser ---

func TestEnsureBrowser_CustomBinSkipsDownload(t *testing.T) {
	// When a custom ChromeBin is set, EnsureBrowser should return immediately
	// without downloading anything.
	err := EnsureBrowser(Config{ChromeBin: "/usr/bin/some-browser"})
	if err != nil {
		t.Errorf("expected no error for custom ChromeBin, got %v", err)
	}
}

func TestEnsureBrowser_ChromeSkipsDownload(t *testing.T) {
	// When Browser="chrome" and Chrome is installed, no download needed.
	// When not installed, resolveBrowserBin returns "" so it would try to download.
	// Either way, this should not panic.
	_ = EnsureBrowser(Config{Browser: "chrome"})
}

// --- Tests for resolveBrowserBin ---

func TestResolveBrowserBin(t *testing.T) {
	// ChromeBin takes highest priority
	got := resolveBrowserBin(Config{ChromeBin: "/usr/bin/custom-chrome"})
	if got != "/usr/bin/custom-chrome" {
		t.Errorf("expected ChromeBin path, got %q", got)
	}

	// Browser="chromium" returns empty (use Rod's bundled)
	got = resolveBrowserBin(Config{Browser: "chromium"})
	if got != "" {
		t.Errorf("expected empty for chromium, got %q", got)
	}

	// Browser="" returns empty (default to Rod's bundled)
	got = resolveBrowserBin(Config{Browser: ""})
	if got != "" {
		t.Errorf("expected empty for empty browser, got %q", got)
	}

	// Custom binary path that does not exist falls back to empty
	got = resolveBrowserBin(Config{Browser: "/nonexistent/browser"})
	if got != "" {
		t.Errorf("expected empty for nonexistent custom path, got %q", got)
	}

	// Custom binary path that exists
	tmpBin := t.TempDir() + "/my-browser"
	if err := os.WriteFile(tmpBin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	got = resolveBrowserBin(Config{Browser: tmpBin})
	if got != tmpBin {
		t.Errorf("expected %q for existing custom binary, got %q", tmpBin, got)
	}
}

// --- Tests for writeChromePref ---

func TestWriteChromePref(t *testing.T) {
	dir := t.TempDir()
	writeChromePref(dir)

	prefsPath := dir + "/Default/Preferences"
	data, err := os.ReadFile(prefsPath)
	if err != nil {
		t.Fatalf("expected Preferences file to exist: %v", err)
	}
	if len(data) == 0 {
		t.Error("Preferences file should not be empty")
	}
	if !strings.Contains(string(data), "credentials_enable_service") {
		t.Error("Preferences should contain credentials_enable_service")
	}
}

// --- Test for viewportCenter ---

func TestViewportCenter(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	center := d.viewportCenter()
	expectedX := float64(d.viewportW) / 2.0
	expectedY := float64(d.viewportH) / 2.0
	// proto.Point has X and Y fields
	if center.X != expectedX {
		t.Errorf("viewportCenter().X = %f, want %f", center.X, expectedX)
	}
	if center.Y != expectedY {
		t.Errorf("viewportCenter().Y = %f, want %f", center.Y, expectedY)
	}
}

// --- Test for SetFindTimeout edge case ---

func TestSetFindTimeoutZero(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	original := d.findTimeoutMs
	d.SetFindTimeout(0) // Should not change
	if d.findTimeoutMs != original {
		t.Errorf("SetFindTimeout(0) should not change timeout, got %d", d.findTimeoutMs)
	}

	d.SetFindTimeout(-1) // Should not change
	if d.findTimeoutMs != original {
		t.Errorf("SetFindTimeout(-1) should not change timeout, got %d", d.findTimeoutMs)
	}
}

// --- Test for successResult, errorResult, unsupportedResult ---

func TestResultHelpers(t *testing.T) {
	sr := successResult("ok", nil)
	if !sr.Success || sr.Message != "ok" || sr.Element != nil {
		t.Errorf("successResult unexpected: %+v", sr)
	}

	elem := &core.ElementInfo{Text: "hello"}
	sr2 := successResult("found", elem)
	if !sr2.Success || sr2.Element != elem {
		t.Errorf("successResult with element unexpected: %+v", sr2)
	}

	er := errorResult(fmt.Errorf("fail"), "bad")
	if er.Success || er.Message != "bad" || er.Error == nil {
		t.Errorf("errorResult unexpected: %+v", er)
	}

	ur := unsupportedResult("not supported")
	if ur.Success || ur.Message != "not supported" || ur.Error == nil {
		t.Errorf("unsupportedResult unexpected: %+v", ur)
	}
}

// ============================================
// Cookie & Auth State Tests
// ============================================

func TestSetCookies(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	step := &flow.SetCookiesStep{
		BaseStep: flow.BaseStep{StepType: flow.StepSetCookies},
		Cookies: []flow.CookieSpec{
			{Name: "session", Value: "abc123", Domain: "127.0.0.1", Path: "/"},
			{Name: "theme", Value: "dark", Domain: "127.0.0.1", Path: "/"},
		},
	}

	result := d.Execute(step)
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "2 cookie") {
		t.Errorf("expected message about 2 cookies, got: %s", result.Message)
	}
}

func TestSetCookiesEmpty(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	step := &flow.SetCookiesStep{
		BaseStep: flow.BaseStep{StepType: flow.StepSetCookies},
	}

	result := d.Execute(step)
	if result.Success {
		t.Fatal("expected error for empty cookies")
	}
}

func TestGetCookies(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Set a cookie first
	setStep := &flow.SetCookiesStep{
		BaseStep: flow.BaseStep{StepType: flow.StepSetCookies},
		Cookies: []flow.CookieSpec{
			{Name: "test_cookie", Value: "hello", Domain: "127.0.0.1", Path: "/"},
		},
	}
	d.Execute(setStep)

	// Get cookies
	getStep := &flow.GetCookiesStep{
		BaseStep: flow.BaseStep{StepType: flow.StepGetCookies},
		Output:   "cookies",
	}
	result := d.Execute(getStep)
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	data, ok := result.Data.(string)
	if !ok || data == "" {
		t.Fatal("expected non-empty JSON data")
	}
	if !strings.Contains(data, "test_cookie") || !strings.Contains(data, "hello") {
		t.Errorf("expected cookie data in result, got: %s", data)
	}
}

func TestSaveAndLoadAuthState(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Set up some state: cookie + localStorage
	setStep := &flow.SetCookiesStep{
		BaseStep: flow.BaseStep{StepType: flow.StepSetCookies},
		Cookies: []flow.CookieSpec{
			{Name: "auth_token", Value: "secret123", Domain: "127.0.0.1", Path: "/"},
		},
	}
	d.Execute(setStep)

	d.page.MustEval(`() => {
		localStorage.setItem("user", "alice");
		localStorage.setItem("lang", "en");
		sessionStorage.setItem("tab", "home");
	}`)

	// Save auth state
	tmpFile := t.TempDir() + "/auth-state.json"
	saveStep := &flow.SaveAuthStateStep{
		BaseStep: flow.BaseStep{StepType: flow.StepSaveAuthState},
		Path:     tmpFile,
	}
	result := d.Execute(saveStep)
	if !result.Success {
		t.Fatalf("saveAuthState failed: %v", result.Error)
	}
	if !strings.Contains(result.Message, "auth_token") || !strings.Contains(result.Message, "localStorage") {
		// Just check it mentions counts
		if !strings.Contains(result.Message, "cookie") {
			t.Errorf("expected message with cookie info, got: %s", result.Message)
		}
	}

	// Verify file was created
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if !strings.Contains(string(data), "auth_token") {
		t.Error("saved file should contain auth_token cookie")
	}
	if !strings.Contains(string(data), "alice") {
		t.Error("saved file should contain localStorage user=alice")
	}
	if !strings.Contains(string(data), "home") {
		t.Error("saved file should contain sessionStorage tab=home")
	}

	// Clear everything
	d.clearAllState()

	// Verify state is gone
	lsCheck, _ := d.page.Eval(`() => localStorage.getItem("user") || ""`)
	if lsCheck != nil && lsCheck.Value.Str() != "" {
		t.Error("localStorage should be cleared")
	}

	// Load auth state
	loadStep := &flow.LoadAuthStateStep{
		BaseStep: flow.BaseStep{StepType: flow.StepLoadAuthState},
		Path:     tmpFile,
	}
	result = d.Execute(loadStep)
	if !result.Success {
		t.Fatalf("loadAuthState failed: %v", result.Error)
	}

	// Verify cookies were restored
	getStep := &flow.GetCookiesStep{
		BaseStep: flow.BaseStep{StepType: flow.StepGetCookies},
	}
	cookieResult := d.Execute(getStep)
	cookieData, _ := cookieResult.Data.(string)
	if !strings.Contains(cookieData, "auth_token") {
		t.Error("cookie auth_token should be restored")
	}

	// Verify localStorage was restored
	lsVal, _ := d.page.Eval(`() => localStorage.getItem("user")`)
	if lsVal == nil || lsVal.Value.Str() != "alice" {
		t.Error("localStorage user should be restored to 'alice'")
	}

	// Verify sessionStorage was restored
	ssVal, _ := d.page.Eval(`() => sessionStorage.getItem("tab")`)
	if ssVal == nil || ssVal.Value.Str() != "home" {
		t.Error("sessionStorage tab should be restored to 'home'")
	}
}

func TestSaveAuthStateEmptyPath(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	step := &flow.SaveAuthStateStep{BaseStep: flow.BaseStep{StepType: flow.StepSaveAuthState}}
	result := d.Execute(step)
	if result.Success {
		t.Fatal("expected error for empty path")
	}
}

func TestLoadAuthStateEmptyPath(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	step := &flow.LoadAuthStateStep{BaseStep: flow.BaseStep{StepType: flow.StepLoadAuthState}}
	result := d.Execute(step)
	if result.Success {
		t.Fatal("expected error for empty path")
	}
}

func TestLoadAuthStateMissingFile(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	step := &flow.LoadAuthStateStep{
		BaseStep: flow.BaseStep{StepType: flow.StepLoadAuthState},
		Path:     "/nonexistent/auth.json",
	}
	result := d.Execute(step)
	if result.Success {
		t.Fatal("expected error for missing file")
	}
}

// ============================================
// File Upload, Download & Permission Tests
// ============================================

func TestUploadFile(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL+"/upload")
	defer d.Close()

	// Create a temp file to upload
	tmpFile := t.TempDir() + "/test-upload.txt"
	_ = os.WriteFile(tmpFile, []byte("test content"), 0o600)

	step := &flow.UploadFileStep{
		BaseStep: flow.BaseStep{StepType: flow.StepUploadFile},
		Selector: flow.Selector{CSS: "#file-input"},
		Path:     tmpFile,
	}
	result := d.Execute(step)
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "1 file") {
		t.Errorf("expected message about 1 file, got: %s", result.Message)
	}

	// Verify the file name appeared in the page
	time.Sleep(200 * time.Millisecond)
	nameElem, _ := d.page.Element("#file-name")
	text, _ := nameElem.Text()
	if !strings.Contains(text, "test-upload.txt") {
		t.Errorf("expected file name in page, got: %s", text)
	}
}

func TestUploadFileMultiple(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL+"/upload")
	defer d.Close()

	dir := t.TempDir()
	_ = os.WriteFile(dir+"/a.txt", []byte("a"), 0o600)
	_ = os.WriteFile(dir+"/b.txt", []byte("b"), 0o600)

	step := &flow.UploadFileStep{
		BaseStep: flow.BaseStep{StepType: flow.StepUploadFile},
		Selector: flow.Selector{CSS: "#file-input"},
		Paths:    []string{dir + "/a.txt", dir + "/b.txt"},
	}
	result := d.Execute(step)
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "2 file") {
		t.Errorf("expected message about 2 files, got: %s", result.Message)
	}
}

func TestUploadFileNoPaths(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL+"/upload")
	defer d.Close()

	step := &flow.UploadFileStep{
		BaseStep: flow.BaseStep{StepType: flow.StepUploadFile},
		Selector: flow.Selector{CSS: "#file-input"},
	}
	result := d.Execute(step)
	if result.Success {
		t.Fatal("expected error for no paths")
	}
}

func TestUploadFileMissing(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL+"/upload")
	defer d.Close()

	step := &flow.UploadFileStep{
		BaseStep: flow.BaseStep{StepType: flow.StepUploadFile},
		Selector: flow.Selector{CSS: "#file-input"},
		Path:     "/nonexistent/file.txt",
	}
	result := d.Execute(step)
	if result.Success {
		t.Fatal("expected error for missing file")
	}
}

func TestWaitForDownload(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL+"/download-page")
	defer d.Close()

	downloadDir := t.TempDir()

	step := &flow.WaitForDownloadStep{
		BaseStep:       flow.BaseStep{StepType: flow.StepWaitForDownload, TimeoutMs: 10000},
		SaveTo:         downloadDir,
		AssertFilename: "test-file.txt",
	}

	// Click the download link after a short delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		el, err := d.page.Element("#download-link")
		if err == nil {
			_ = el.Click(proto.InputMouseButtonLeft, 1)
		}
	}()

	result := d.Execute(step)
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "test-file.txt") {
		t.Errorf("expected filename in message, got: %s", result.Message)
	}

	// Verify file was saved
	data, err := os.ReadFile(downloadDir + "/test-file.txt")
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != "download content" {
		t.Errorf("unexpected file content: %s", string(data))
	}
}

func TestWaitForDownloadTimeout(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	step := &flow.WaitForDownloadStep{
		BaseStep: flow.BaseStep{StepType: flow.StepWaitForDownload, TimeoutMs: 500},
		SaveTo:   t.TempDir(),
	}

	result := d.Execute(step)
	if result.Success {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(result.Error.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", result.Error)
	}
}

func TestGrantPermissions(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	step := &flow.GrantPermissionsStep{
		BaseStep:    flow.BaseStep{StepType: flow.StepGrantPermissions},
		Permissions: []string{"notifications", "geolocation"},
	}
	result := d.Execute(step)
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "2 permission") {
		t.Errorf("expected message about 2 permissions, got: %s", result.Message)
	}
}

func TestGrantPermissionsEmpty(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	step := &flow.GrantPermissionsStep{
		BaseStep: flow.BaseStep{StepType: flow.StepGrantPermissions},
	}
	result := d.Execute(step)
	if result.Success {
		t.Fatal("expected error for empty permissions")
	}
}

func TestResetPermissions(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Grant first, then reset
	d.Execute(&flow.GrantPermissionsStep{
		BaseStep:    flow.BaseStep{StepType: flow.StepGrantPermissions},
		Permissions: []string{"notifications"},
	})

	step := &flow.ResetPermissionsStep{
		BaseStep: flow.BaseStep{StepType: flow.StepResetPermissions},
	}
	result := d.Execute(step)
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
}

// ============================================
// Tab Management Tests
// ============================================

func TestOpenTab(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	step := &flow.OpenTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepOpenTab},
		URL:      ts.URL + "/other",
		TabLabel: "other",
	}
	result := d.Execute(step)
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if !strings.Contains(result.Message, "label: other") {
		t.Errorf("expected label in message, got: %s", result.Message)
	}

	// Verify we're on the new tab
	title, _ := d.page.Eval(`() => document.title`)
	if title == nil || title.Value.Str() != "Other Page" {
		t.Errorf("expected Other Page title, got: %v", title)
	}
}

func TestSwitchTabByLabel(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Label the initial tab
	d.tabLabels["main"] = d.page

	// Open a new tab
	d.Execute(&flow.OpenTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepOpenTab},
		URL:      ts.URL + "/other",
		TabLabel: "other",
	})

	// Switch back to main
	result := d.Execute(&flow.SwitchTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepSwitchTab},
		TabLabel: "main",
	})
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	title, _ := d.page.Eval(`() => document.title`)
	if title == nil || title.Value.Str() != "Test Page" {
		t.Errorf("expected Test Page, got: %v", title)
	}
}

func TestSwitchTabByIndex(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Open a second tab
	d.Execute(&flow.OpenTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepOpenTab},
		URL:      ts.URL + "/other",
	})

	// Switch to first tab (index 0)
	result := d.Execute(&flow.SwitchTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepSwitchTab},
		Index:    0,
	})
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
}

func TestSwitchTabByURL(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Open a second tab
	d.Execute(&flow.OpenTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepOpenTab},
		URL:      ts.URL + "/other",
	})

	// Switch to tab matching */other
	result := d.Execute(&flow.SwitchTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepSwitchTab},
		URL:      "*/other",
	})
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	title, _ := d.page.Eval(`() => document.title`)
	if title == nil || title.Value.Str() != "Other Page" {
		t.Errorf("expected Other Page, got: %v", title)
	}
}

func TestSwitchTabLabelNotFound(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.SwitchTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepSwitchTab},
		TabLabel: "nonexistent",
	})
	if result.Success {
		t.Fatal("expected error for unknown label")
	}
}

func TestSwitchTabIndexOutOfRange(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.SwitchTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepSwitchTab},
		Index:    99,
	})
	if result.Success {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestCloseTab(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Open a second tab
	d.Execute(&flow.OpenTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepOpenTab},
		URL:      ts.URL + "/other",
		TabLabel: "other",
	})

	// Close current tab (the second one)
	result := d.Execute(&flow.CloseTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepCloseTab},
	})
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	// Should have switched back to the remaining tab
	title, _ := d.page.Eval(`() => document.title`)
	if title == nil || title.Value.Str() != "Test Page" {
		t.Errorf("expected Test Page after close, got: %v", title)
	}

	// Label should be removed
	if _, ok := d.tabLabels["other"]; ok {
		t.Error("expected label 'other' to be removed")
	}
}

func TestCloseLastTab(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.Execute(&flow.CloseTabStep{
		BaseStep: flow.BaseStep{StepType: flow.StepCloseTab},
	})
	if result.Success {
		t.Fatal("expected error when closing last tab")
	}
}

func TestMatchURLPattern(t *testing.T) {
	tests := []struct {
		url, pattern string
		want         bool
	}{
		{"https://example.com/callback", "*/callback", true},
		{"https://example.com/oauth/callback?code=123", "*/oauth/callback*", true},
		{"https://example.com/other", "*/callback", false},
		{"https://example.com/page", "example.com", true},
		{"https://example.com/page", "", false},
	}
	for _, tt := range tests {
		if got := matchURLPattern(tt.url, tt.pattern); got != tt.want {
			t.Errorf("matchURLPattern(%q, %q) = %v, want %v", tt.url, tt.pattern, got, tt.want)
		}
	}
}

// ============================================
// Network Interception Tests
// ============================================

func TestMockNetwork(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL+"/fetch-page")
	defer d.Close()

	// Mock the API response
	result := d.mockNetwork(&flow.MockNetworkStep{
		URL:    "*/api/users",
		Method: "GET",
		Response: flow.MockResponseSpec{
			Status:  200,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    `{"users": [{"id": 99, "name": "Mocked User"}]}`,
		},
	})
	if !result.Success {
		t.Fatalf("mockNetwork failed: %s", result.Message)
	}

	// Fetch the API from the page — should get mocked response
	d.page.MustEval(fmt.Sprintf(`() => fetchAPI("%s/api/users")`, ts.URL))
	time.Sleep(500 * time.Millisecond)
	text := d.page.MustElement("#result").MustText()
	if !strings.Contains(text, "Mocked User") {
		t.Errorf("expected mocked response, got: %s", text)
	}
}

func TestMockNetworkEmptyURL(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.mockNetwork(&flow.MockNetworkStep{})
	if result.Success {
		t.Fatal("expected error for empty URL")
	}
}

func TestMockNetworkMethodFilter(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL+"/fetch-page")
	defer d.Close()

	// Mock only POST to /api/users — GET should pass through
	result := d.mockNetwork(&flow.MockNetworkStep{
		URL:    "*/api/users",
		Method: "POST",
		Response: flow.MockResponseSpec{
			Status: 200,
			Body:   `{"mocked": true}`,
		},
	})
	if !result.Success {
		t.Fatalf("mockNetwork failed: %s", result.Message)
	}

	// GET request should pass through to real server
	d.page.MustEval(fmt.Sprintf(`() => fetchAPI("%s/api/users")`, ts.URL))
	time.Sleep(500 * time.Millisecond)
	text := d.page.MustElement("#result").MustText()
	if !strings.Contains(text, "Real User") {
		t.Errorf("expected real response for GET, got: %s", text)
	}
}

func TestBlockNetwork(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL+"/fetch-page")
	defer d.Close()

	// Block the API endpoint
	result := d.blockNetwork(&flow.BlockNetworkStep{
		Patterns: []string{"*/api/users"},
	})
	if !result.Success {
		t.Fatalf("blockNetwork failed: %s", result.Message)
	}

	// Fetch should fail
	d.page.MustEval(fmt.Sprintf(`() => fetchAPI("%s/api/users")`, ts.URL))
	time.Sleep(500 * time.Millisecond)
	text := d.page.MustElement("#result").MustText()
	if !strings.Contains(text, "ERROR") {
		t.Errorf("expected fetch to fail, got: %s", text)
	}
}

func TestBlockNetworkEmpty(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.blockNetwork(&flow.BlockNetworkStep{})
	if result.Success {
		t.Fatal("expected error for empty patterns")
	}
}

func TestSetNetworkConditionsOffline(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL+"/fetch-page")
	defer d.Close()

	// Set offline
	result := d.setNetworkConditions(&flow.SetNetworkConditionsStep{
		Offline: true,
	})
	if !result.Success {
		t.Fatalf("setNetworkConditions failed: %s", result.Message)
	}
	if !strings.Contains(result.Message, "offline") {
		t.Errorf("expected offline message, got: %s", result.Message)
	}

	// Reset back to normal
	result = d.clearNetworkMocks()
	if !result.Success {
		t.Fatalf("clearNetworkMocks failed: %s", result.Message)
	}
}

func TestSetNetworkConditionsThrottle(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.setNetworkConditions(&flow.SetNetworkConditionsStep{
		Latency:       100,
		DownloadSpeed: 500,
		UploadSpeed:   100,
	})
	if !result.Success {
		t.Fatalf("setNetworkConditions failed: %s", result.Message)
	}
	if !strings.Contains(result.Message, "latency=100ms") {
		t.Errorf("expected latency in message, got: %s", result.Message)
	}
}

func TestWaitForRequest(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL+"/fetch-page")
	defer d.Close()

	// Start waiting for request in background, then trigger it
	doneCh := make(chan *core.CommandResult, 1)
	go func() {
		result := d.waitForRequest(&flow.WaitForRequestStep{
			BaseStep: flow.BaseStep{TimeoutMs: 5000},
			URL:      "*/api/users",
		})
		doneCh <- result
	}()

	// Small delay then trigger the fetch
	time.Sleep(200 * time.Millisecond)
	d.page.MustEval(fmt.Sprintf(`() => fetchAPI("%s/api/users")`, ts.URL))

	result := <-doneCh
	if !result.Success {
		t.Fatalf("waitForRequest failed: %s (err: %v)", result.Message, result.Error)
	}
	if !strings.Contains(result.Message, "api/users") {
		t.Errorf("expected URL in message, got: %s", result.Message)
	}
}

func TestWaitForRequestTimeout(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.waitForRequest(&flow.WaitForRequestStep{
		BaseStep: flow.BaseStep{TimeoutMs: 500},
		URL:      "*/api/nonexistent",
	})
	if result.Success {
		t.Fatal("expected timeout error")
	}
}

func TestWaitForRequestEmptyURL(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.waitForRequest(&flow.WaitForRequestStep{})
	if result.Success {
		t.Fatal("expected error for empty URL")
	}
}

func TestClearNetworkMocks(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL+"/fetch-page")
	defer d.Close()

	// Set up a mock
	d.mockNetwork(&flow.MockNetworkStep{
		URL: "*/api/users",
		Response: flow.MockResponseSpec{
			Status: 200,
			Body:   `{"mocked": true}`,
		},
	})

	// Clear mocks
	result := d.clearNetworkMocks()
	if !result.Success {
		t.Fatalf("clearNetworkMocks failed: %s", result.Message)
	}

	// Verify mocks are cleared
	d.networkMu.Lock()
	mockCount := len(d.networkMocks)
	blockCount := len(d.networkBlocks)
	fetchEnabled := d.fetchEnabled
	d.networkMu.Unlock()

	if mockCount != 0 {
		t.Errorf("expected 0 mocks, got %d", mockCount)
	}
	if blockCount != 0 {
		t.Errorf("expected 0 blocks, got %d", blockCount)
	}
	if fetchEnabled {
		t.Error("expected fetchEnabled to be false")
	}

	// Fetch should now get real response (fetch domain re-enabled on next mock)
	d.page.MustEval(fmt.Sprintf(`() => fetchAPI("%s/api/users")`, ts.URL))
	time.Sleep(500 * time.Millisecond)
	text := d.page.MustElement("#result").MustText()
	if !strings.Contains(text, "Real User") {
		t.Errorf("expected real response after clearing mocks, got: %s", text)
	}
}

func TestMockAndBlockCombined(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL+"/fetch-page")
	defer d.Close()

	// Block takes priority — block /api/submit, mock /api/users
	d.blockNetwork(&flow.BlockNetworkStep{
		Patterns: []string{"*/api/submit"},
	})
	d.mockNetwork(&flow.MockNetworkStep{
		URL: "*/api/users",
		Response: flow.MockResponseSpec{
			Status: 200,
			Body:   `{"mocked": true}`,
		},
	})

	// /api/users should return mocked response
	d.page.MustEval(fmt.Sprintf(`() => fetchAPI("%s/api/users")`, ts.URL))
	time.Sleep(500 * time.Millisecond)
	text := d.page.MustElement("#result").MustText()
	if !strings.Contains(text, "mocked") {
		t.Errorf("expected mocked response for /api/users, got: %s", text)
	}

	// /api/submit should be blocked
	d.page.MustEval(fmt.Sprintf(`() => fetchAPI("%s/api/submit")`, ts.URL))
	time.Sleep(500 * time.Millisecond)
	text = d.page.MustElement("#result").MustText()
	if !strings.Contains(text, "ERROR") {
		t.Errorf("expected /api/submit to be blocked, got: %s", text)
	}
}

// ============================================
// Console Capture & Script Execution Tests
// ============================================

func TestConsoleLogs(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Trigger console.log and console.error
	d.page.MustEval(`() => {
		console.log("hello from test");
		console.error("something went wrong");
		console.warn("this is a warning");
	}`)
	time.Sleep(300 * time.Millisecond)

	logs := d.ConsoleLogs()
	if len(logs) < 3 {
		t.Fatalf("expected at least 3 console entries, got %d", len(logs))
	}

	// Check that we captured the entries
	var foundLog, foundError, foundWarn bool
	for _, entry := range logs {
		if entry.Level == "log" && strings.Contains(entry.Message, "hello from test") {
			foundLog = true
		}
		if entry.Level == "error" && strings.Contains(entry.Message, "something went wrong") {
			foundError = true
		}
		if entry.Level == "warning" && strings.Contains(entry.Message, "this is a warning") {
			foundWarn = true
		}
	}
	if !foundLog {
		t.Error("missing console.log entry")
	}
	if !foundError {
		t.Error("missing console.error entry")
	}
	if !foundWarn {
		t.Error("missing console.warn entry")
	}
}

func TestConsoleException(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Trigger an uncaught exception via setTimeout (so it doesn't fail our eval)
	d.page.MustEval(`() => {
		setTimeout(() => { throw new Error("uncaught test error") }, 10);
	}`)
	time.Sleep(500 * time.Millisecond)

	logs := d.ConsoleLogs()
	var found bool
	for _, entry := range logs {
		if entry.Level == "exception" && strings.Contains(entry.Message, "uncaught test error") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected uncaught exception entry, got: %+v", logs)
	}
}

func TestGetConsoleLogs(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	d.page.MustEval(`() => console.log("test message")`)
	time.Sleep(300 * time.Millisecond)

	result := d.getConsoleLogs()
	if !result.Success {
		t.Fatalf("getConsoleLogs failed: %s", result.Message)
	}

	var logs []ConsoleEntry
	if err := json.Unmarshal([]byte(result.Data.(string)), &logs); err != nil {
		t.Fatalf("failed to parse console logs JSON: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected at least 1 console entry")
	}
}

func TestClearConsoleLogs(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	d.page.MustEval(`() => console.log("before clear")`)
	time.Sleep(300 * time.Millisecond)

	if len(d.ConsoleLogs()) == 0 {
		t.Fatal("expected console entries before clear")
	}

	result := d.clearConsoleLogs()
	if !result.Success {
		t.Fatalf("clearConsoleLogs failed: %s", result.Message)
	}

	if len(d.ConsoleLogs()) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(d.ConsoleLogs()))
	}
}

func TestAssertNoJSErrors_Pass(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Only log and warn — no errors
	d.page.MustEval(`() => {
		console.log("info message");
		console.warn("warning");
	}`)
	time.Sleep(300 * time.Millisecond)

	result := d.assertNoJSErrors()
	if !result.Success {
		t.Fatalf("assertNoJSErrors should pass with no errors, got: %s", result.Message)
	}
}

func TestAssertNoJSErrors_Fail(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	d.page.MustEval(`() => console.error("test error")`)
	time.Sleep(300 * time.Millisecond)

	result := d.assertNoJSErrors()
	if result.Success {
		t.Fatal("assertNoJSErrors should fail when console.error exists")
	}
	if !strings.Contains(result.Message, "test error") {
		t.Errorf("expected error message in result, got: %s", result.Message)
	}
}

func TestRunBrowserScript(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Create a temp JS file
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test-script.js")
	_ = os.WriteFile(scriptPath, []byte(`return document.title;`), 0644)

	result := d.runBrowserScript(&flow.RunBrowserScriptStep{
		File: scriptPath,
	})
	if !result.Success {
		t.Fatalf("runBrowserScript failed: %s (err: %v)", result.Message, result.Error)
	}
	if result.Data != "Test Page" {
		t.Errorf("expected 'Test Page', got: %v", result.Data)
	}
}

func TestRunBrowserScriptWithEnv(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "env-script.js")
	_ = os.WriteFile(scriptPath, []byte(`return window.__env.API_KEY;`), 0644)

	result := d.runBrowserScript(&flow.RunBrowserScriptStep{
		File: scriptPath,
		Env:  map[string]string{"API_KEY": "secret123"},
	})
	if !result.Success {
		t.Fatalf("runBrowserScript failed: %s", result.Message)
	}
	if result.Data != "secret123" {
		t.Errorf("expected 'secret123', got: %v", result.Data)
	}
}

func TestRunBrowserScriptEmptyFile(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.runBrowserScript(&flow.RunBrowserScriptStep{})
	if result.Success {
		t.Fatal("expected error for empty file")
	}
}

func TestRunBrowserScriptMissingFile(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	result := d.runBrowserScript(&flow.RunBrowserScriptStep{
		File: "/nonexistent/script.js",
	})
	if result.Success {
		t.Fatal("expected error for missing file")
	}
}
