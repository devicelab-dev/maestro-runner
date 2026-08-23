package wda

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Page source with three identically-named row buttons on screen and a fourth
// scrolled fully out of the 390x844 viewport.
const countPageSource = `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="TestApp" label="TestApp" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeWindow type="XCUIElementTypeWindow" enabled="true" visible="true" x="0" y="0" width="390" height="844">
      <XCUIElementTypeButton type="XCUIElementTypeButton" name="product-row" label="Alpha" enabled="true" visible="true" x="0" y="100" width="390" height="60"/>
      <XCUIElementTypeButton type="XCUIElementTypeButton" name="product-row" label="Beta" enabled="true" visible="true" x="0" y="200" width="390" height="60"/>
      <XCUIElementTypeButton type="XCUIElementTypeButton" name="product-row" label="Gamma" enabled="true" visible="true" x="0" y="300" width="390" height="60"/>
      <XCUIElementTypeButton type="XCUIElementTypeButton" name="product-row" label="Offscreen" enabled="true" visible="false" x="0" y="2000" width="390" height="60"/>
      <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" label="Products" enabled="true" visible="true" x="0" y="40" width="390" height="30"/>
    </XCUIElementTypeWindow>
  </XCUIElementTypeApplication>
</AppiumAUT>`

func TestCountVisibleMatches(t *testing.T) {
	elements, err := ParsePageSource(countPageSource)
	if err != nil {
		t.Fatalf("ParsePageSource failed: %v", err)
	}

	// The off-screen row is excluded when the screen size is known.
	if got := CountVisibleMatches(elements, flow.Selector{ID: "product-row"}, 390, 844); got != 3 {
		t.Errorf("count with screen bounds = %d, want 3", got)
	}

	// Without a known screen size all matches count (mirrors the
	// single-match path, which skips FilterOutOfBounds in that case).
	if got := CountVisibleMatches(elements, flow.Selector{ID: "product-row"}, 0, 0); got != 4 {
		t.Errorf("count without screen bounds = %d, want 4", got)
	}

	if got := CountVisibleMatches(elements, flow.Selector{Text: "Beta"}, 390, 844); got != 1 {
		t.Errorf("count by text = %d, want 1", got)
	}

	if got := CountVisibleMatches(elements, flow.Selector{ID: "no-such-id"}, 390, 844); got != 0 {
		t.Errorf("count of absent id = %d, want 0", got)
	}
}

func sourceServer(t *testing.T, pageSource string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			jsonResponse(w, map[string]interface{}{"value": pageSource})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
}

func TestAssertVisibleCount_ExactMatch(t *testing.T) {
	server := sourceServer(t, countPageSource)
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "product-row"}, Count: "3"}
	result := driver.assertVisible(step)
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "3") {
		t.Errorf("message should state the count, got: %s", result.Message)
	}
}

func TestAssertVisibleCount_WrongCount(t *testing.T) {
	server := sourceServer(t, countPageSource)
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "product-row"}, Count: "5"}
	step.TimeoutMs = 600 // keep the polling loop short
	result := driver.assertVisible(step)
	if result.Success {
		t.Fatal("expected failure for a wrong count")
	}
	if !strings.Contains(result.Message, "Expected 5") || !strings.Contains(result.Message, "found 3") {
		t.Errorf("message should state expected and observed counts, got: %s", result.Message)
	}
}

func TestAssertVisibleCount_InvalidCount(t *testing.T) {
	server := sourceServer(t, countPageSource)
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "product-row"}, Count: "${UNEXPANDED}"}
	result := driver.assertVisible(step)
	if result.Success {
		t.Fatal("expected failure for an unparseable count")
	}
	if !strings.Contains(result.Message, "not a number") {
		t.Errorf("message should explain the bad count, got: %s", result.Message)
	}
}

func TestAssertVisibleCount_RelativeSelectorRejected(t *testing.T) {
	server := sourceServer(t, countPageSource)
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertVisibleStep{
		Selector: flow.Selector{ID: "product-row", Below: &flow.Selector{Text: "Products"}},
		Count:    "2",
	}
	result := driver.assertVisible(step)
	if result.Success {
		t.Fatal("expected failure for count with a relative selector")
	}
	if !strings.Contains(result.Message, "relative selectors") {
		t.Errorf("message should name the unsupported combination, got: %s", result.Message)
	}
}

func TestAssertVisibleCount_SourceUnreadable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	driver := createTestDriver(server)

	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "product-row"}, Count: "1"}
	step.TimeoutMs = 600
	result := driver.assertVisible(step)
	if result.Success {
		t.Fatal("expected failure when the page source cannot be read")
	}
	if !strings.Contains(result.Message, "could not read the screen") {
		t.Errorf("message should say the screen was unreadable, got: %s", result.Message)
	}
}
