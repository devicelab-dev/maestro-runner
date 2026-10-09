package wda

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// An index that is not a number fails the lookup instead of picking the first
// match (`index: ${ROW}` with ROW unset is "undefined"); "2.0" is 2, as in
// Maestro.
func TestPageSourceIndexNotANumber(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?>
<AppiumAUT>
  <XCUIElementTypeApplication type="XCUIElementTypeApplication" name="App" enabled="true" visible="true" x="0" y="0" width="390" height="844">
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="row" label="Row 1" enabled="true" visible="true" x="0" y="100" width="390" height="50"/>
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="row" label="Row 2" enabled="true" visible="true" x="0" y="200" width="390" height="50"/>
    <XCUIElementTypeButton type="XCUIElementTypeButton" name="row" label="Row 3" enabled="true" visible="true" x="0" y="300" width="390" height="50"/>
  </XCUIElementTypeApplication>
</AppiumAUT>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			jsonResponse(w, map[string]interface{}{"value": src})
			return
		}
		jsonResponse(w, map[string]interface{}{"status": 0})
	}))
	defer server.Close()
	driver := createTestDriver(server)

	if info, err := driver.findElementByPageSourceOnce(flow.Selector{ID: "row", Index: "undefined"}); err == nil {
		t.Fatalf("index \"undefined\" found an element at Y=%d, want an error", info.Bounds.Y)
	} else if !strings.Contains(err.Error(), "whole number") {
		t.Errorf("error %q should say the index must be a number", err)
	}
	info, err := driver.findElementByPageSourceOnce(flow.Selector{ID: "row", Index: "2.0"})
	if err != nil || info.Bounds.Y != 300 {
		t.Fatalf("index \"2.0\": %+v, %v; want the third row (Y=300)", info, err)
	}
}
