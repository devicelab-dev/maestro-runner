package wda

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// A lookup that falls back to the page source asks WDA to leave out the
// attributes it computes per element and the matcher never reads; the
// hierarchy a report or a tool shows keeps every attribute.
func TestLookupSourceLeavesOutVisibleAndAccessible(t *testing.T) {
	var mu sync.Mutex
	var sources []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/source"):
			mu.Lock()
			sources = append(sources, r.URL.Query().Get("excluded_attributes"))
			mu.Unlock()
			// WDA leaves out what it was asked to: no visible attribute here.
			jsonResponse(w, map[string]any{"value": `<AppiumAUT><XCUIElementTypeApplication type="XCUIElementTypeApplication" name="App" enabled="true" x="0" y="0" width="390" height="844">` +
				`<XCUIElementTypeStaticText type="XCUIElementTypeStaticText" name="greeting" label="Hello" enabled="true" x="20" y="100" width="200" height="30"/>` +
				`</XCUIElementTypeApplication></AppiumAUT>`})
		case strings.HasSuffix(r.URL.Path, "/elements"):
			jsonResponse(w, map[string]any{"value": []any{}})
		case strings.HasSuffix(r.URL.Path, "/element"):
			w.WriteHeader(http.StatusNotFound)
			jsonResponse(w, map[string]any{"value": map[string]string{"error": "no such element", "message": "not found"}})
		default:
			w.WriteHeader(http.StatusNotFound)
			jsonResponse(w, map[string]any{"value": map[string]string{"error": "unknown command", "message": r.URL.Path}})
		}
	}))
	defer server.Close()
	d := createTestDriver(server)

	info, err := d.findElementByPageSourceOnce(flow.Selector{Text: "Hello"})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !info.Visible || info.MatchNote != "" {
		t.Errorf("an element of a source without visible should count as displayed with no note, got visible=%v note=%q", info.Visible, info.MatchNote)
	}
	if _, err := d.Hierarchy(); err != nil {
		t.Fatalf("hierarchy: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(sources) != 2 {
		t.Fatalf("want 2 source reads, got %d", len(sources))
	}
	if sources[0] != "visible,accessible" {
		t.Errorf("lookup's source: excluded_attributes = %q, want %q", sources[0], "visible,accessible")
	}
	if sources[1] != "" {
		t.Errorf("hierarchy's source should keep every attribute, got excluded_attributes = %q", sources[1])
	}
}
