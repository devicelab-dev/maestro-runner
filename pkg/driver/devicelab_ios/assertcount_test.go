package devicelab_ios

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// snapshotServer serves a runner that answers every command with the nodes
// produced by nodesFn, called once per request with the request ordinal.
func snapshotServer(t *testing.T, nodesFn func(call int64) []SnapshotNode) *Driver {
	t.Helper()
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&calls, 1)
		resp := map[string]any{
			"ok":   true,
			"data": map[string]any{"nodes": nodesFn(n)},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	client := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	return NewDriver(client, nil, "test-udid", nil)
}

func visibleRow(id string) SnapshotNode {
	return SnapshotNode{
		Type:       "Cell",
		Identifier: id,
		Rect:       SnapshotRect{X: 0, Y: 0, Width: 100, Height: 40},
		Enabled:    true,
	}
}

func TestAssertVisibleCount_ExactMatch(t *testing.T) {
	d := snapshotServer(t, func(int64) []SnapshotNode {
		return []SnapshotNode{
			visibleRow("row"),
			visibleRow("row"),
			visibleRow("row"),
			{Type: "Cell", Identifier: "row"}, // zero-size: matches but not displayed
			visibleRow("other"),
		}
	})

	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "row"}, Count: "3"}
	res := d.handleAssertVisible(step)
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Message)
	}
	if !strings.Contains(res.Message, "3") {
		t.Errorf("Message = %q, want the observed count in it", res.Message)
	}
}

func TestAssertVisibleCount_WrongCountReportsObserved(t *testing.T) {
	d := snapshotServer(t, func(int64) []SnapshotNode {
		return []SnapshotNode{visibleRow("row"), visibleRow("row")}
	})

	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "row"}, Count: "3"}
	step.TimeoutMs = 250
	res := d.handleAssertVisible(step)
	if res.Success {
		t.Fatal("expected failure when only 2 of 3 are visible")
	}
	if !strings.Contains(res.Message, "expected 3") || !strings.Contains(res.Message, "found 2") {
		t.Errorf("Message = %q, want expected and observed counts", res.Message)
	}
	if !strings.Contains(res.Message, "id=row") {
		t.Errorf("Message = %q, want the selector description", res.Message)
	}
}

func TestAssertVisibleCount_PollsUntilCountReached(t *testing.T) {
	// First two snapshots see 1 row; later ones see 2. The poll loop must
	// invalidate the snapshot cache between passes to observe the change.
	d := snapshotServer(t, func(call int64) []SnapshotNode {
		if call <= 2 {
			return []SnapshotNode{visibleRow("row")}
		}
		return []SnapshotNode{visibleRow("row"), visibleRow("row")}
	})

	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "row"}, Count: "2"}
	step.TimeoutMs = 5000
	res := d.handleAssertVisible(step)
	if !res.Success {
		t.Fatalf("expected the poll to observe the second row, got: %s", res.Message)
	}
}

func TestAssertVisibleCount_UnresolvedVariableFails(t *testing.T) {
	d := snapshotServer(t, func(int64) []SnapshotNode { return nil })

	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "row"}, Count: "${N}"}
	res := d.handleAssertVisible(step)
	if res.Success {
		t.Fatal("expected an error result for an unresolved count variable")
	}
	if !strings.Contains(res.Message, "not a number") {
		t.Errorf("Message = %q, want the count parse error", res.Message)
	}
}

func TestCountDisplayed(t *testing.T) {
	nodes := []SnapshotNode{
		visibleRow("a"),
		{Type: "Cell", Identifier: "b"}, // zero-size
		{Type: "Cell", Identifier: "c", Rect: SnapshotRect{Width: 10}}, // zero height
		{Type: "Cell", Identifier: "d", Rect: SnapshotRect{Width: 1, Height: 1}},
	}
	if got := countDisplayed(nodes); got != 2 {
		t.Errorf("countDisplayed = %d, want 2", got)
	}
	if got := countDisplayed(nil); got != 0 {
		t.Errorf("countDisplayed(nil) = %d, want 0", got)
	}
}
