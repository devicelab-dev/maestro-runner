package wda

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

const emptyScreenSource = `<AppiumAUT><XCUIElementTypeApplication type="XCUIElementTypeApplication" x="0" y="0" width="390" height="844"/></AppiumAUT>`

// unreadableScreenServer answers the first n page-source reads with a server
// error, then serves an empty screen; n < 0 fails every read. WDA's own
// queries never find anything.
func unreadableScreenServer(t *testing.T, failures int) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	reads := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/source") {
			mu.Lock()
			reads++
			n := reads
			mu.Unlock()
			if failures < 0 || n <= failures {
				w.WriteHeader(http.StatusInternalServerError)
				jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "unknown error", "message": "snapshot failed"}})
				return
			}
			jsonResponse(w, map[string]interface{}{"value": emptyScreenSource})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "no such element"}})
	}))
}

// A page source that cannot be read says nothing about the element, so
// assertNotVisible does not pass on it. Maestro counts only element-not-found
// as gone (Orchestra.kt:1066-1082).
func TestAssertNotVisibleFailsWhenScreenUnreadable(t *testing.T) {
	server := unreadableScreenServer(t, -1)
	defer server.Close()
	d := createTestDriver(server)

	res := d.assertNotVisible(&flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 300},
		Selector: flow.Selector{Text: "Banner"},
	})
	if res.Success {
		t.Fatal("assertNotVisible passed although the screen was never read")
	}
	if !strings.Contains(res.Message, "Could not check") || !strings.Contains(res.Message, "snapshot failed") {
		t.Errorf("message should say the check could not be made, and why: %s", res.Message)
	}
}

// A read that fails once is tried again; the element is gone once a read
// succeeds and finds no match.
func TestAssertNotVisiblePassesAfterUnreadableScreen(t *testing.T) {
	server := unreadableScreenServer(t, 2)
	defer server.Close()
	d := createTestDriver(server)

	res := d.assertNotVisible(&flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 3000},
		Selector: flow.Selector{Text: "Banner"},
	})
	if !res.Success {
		t.Fatalf("assertNotVisible should pass once the screen reads empty: %s", res.Message)
	}
}

// A relative selector whose anchor is not on screen matches nothing: gone.
func TestAssertNotVisibleRelativeAnchorAbsent(t *testing.T) {
	server := unreadableScreenServer(t, 0)
	defer server.Close()
	d := createTestDriver(server)

	res := d.assertNotVisible(&flow.AssertNotVisibleStep{
		BaseStep: flow.BaseStep{TimeoutMs: 300},
		Selector: flow.Selector{Text: "Banner", Below: &flow.Selector{Text: "Header"}},
	})
	if !res.Success {
		t.Fatalf("no anchor on screen means no match: %s", res.Message)
	}
}

// extendedWaitUntil notVisible follows the same rule.
func TestWaitUntilNotVisibleFailsWhenScreenUnreadable(t *testing.T) {
	server := unreadableScreenServer(t, -1)
	defer server.Close()
	d := createTestDriver(server)

	sel := flow.Selector{Text: "Banner"}
	res := d.waitUntil(&flow.WaitUntilStep{NotVisible: &sel, BaseStep: flow.BaseStep{TimeoutMs: 300}})
	if res.Success {
		t.Fatal("waitUntil notVisible passed although the screen was never read")
	}
	if !strings.Contains(res.Message, "Could not check") {
		t.Errorf("message should say the check could not be made: %s", res.Message)
	}

	server2 := unreadableScreenServer(t, 1)
	defer server2.Close()
	d2 := createTestDriver(server2)
	if res := d2.waitUntil(&flow.WaitUntilStep{NotVisible: &sel, BaseStep: flow.BaseStep{TimeoutMs: 3000}}); !res.Success {
		t.Fatalf("waitUntil notVisible should pass once the screen reads empty: %s", res.Message)
	}
}

func TestIsNotFound(t *testing.T) {
	if !isNotFound(notFound("no elements match selector")) {
		t.Error("a notFound error is not found")
	}
	if !isNotFound(fmt.Errorf("context deadline exceeded: %w", notFound("no elements match selector"))) {
		t.Error("a wrapped notFound error is not found")
	}
	if isNotFound(nil) {
		t.Error("nil is not a lookup that found nothing")
	}
	if isNotFound(errors.New("connection reset by peer")) {
		t.Error("a read error is not a lookup that found nothing")
	}
}

// extendedWaitUntil visible that never could read the screen says so, rather
// than only that the element was not visible.
func TestWaitUntilVisibleSaysWhenScreenUnreadable(t *testing.T) {
	server := unreadableScreenServer(t, -1)
	defer server.Close()
	d := createTestDriver(server)

	sel := flow.Selector{Text: "quintePlusParieuse"}
	res := d.waitUntil(&flow.WaitUntilStep{Visible: &sel, BaseStep: flow.BaseStep{TimeoutMs: 300}})
	if res.Success || !strings.Contains(res.Message, "could not read the screen") {
		t.Errorf("message should say WDA could not read the screen: %s", res.Message)
	}

	readable := mockWDAServerForWaitUntil(10000)
	defer readable.Close()
	d2 := createTestDriver(readable)
	if res := d2.waitUntil(&flow.WaitUntilStep{Visible: &sel, BaseStep: flow.BaseStep{TimeoutMs: 200}}); strings.Contains(res.Message, "could not read") {
		t.Errorf("a readable screen without the element must not blame the screen: %s", res.Message)
	}
}
