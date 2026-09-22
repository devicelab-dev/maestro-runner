package devicelab_ios

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

const (
	snapFailedBody = `{"ok":false,"data":{"appState":"runningBackgroundSuspended"},` +
		`"error":{"code":"SNAPSHOT_FAILED","message":"accessibility snapshot failed or timed out"}}`
	snapRowBody = `{"ok":true,"data":{"source":"xctest","appState":"runningForeground","nodes":[` +
		`{"index":0,"type":"Cell","identifier":"row","rect":{"x":0,"y":0,"width":100,"height":40},` +
		`"enabled":true,"hittable":true,"depth":0}]}}`
	snapEmptyBody = `{"ok":true,"data":{"source":"xctest","appState":"runningForeground","nodes":[]}}`
)

// scriptedDriver serves a runner whose reply to the n-th request (1-based)
// is bodyFn(n), and returns a Driver talking to it.
func scriptedDriver(t *testing.T, bodyFn func(call int64) string) *Driver {
	t.Helper()
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(bodyFn(atomic.AddInt64(&calls, 1))))
	}))
	t.Cleanup(srv.Close)
	client := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	return NewDriver(client, nil, "test-udid", nil)
}

// failFirst answers SNAPSHOT_FAILED to the first n requests, then then.
func failFirst(n int64, then string) func(int64) string {
	return func(call int64) string {
		if call <= n {
			return snapFailedBody
		}
		return then
	}
}

func always(body string) func(int64) string { return func(int64) string { return body } }

// TestSnapshotResponseDecoding: source and a failed snapshot's appState reach
// the caller, and the failure is a RunnerError the helper recognises.
func TestSnapshotResponseDecoding(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantErrCode string
		wantSource  string
		wantState   string
	}{
		{"xctest tree", snapRowBody, "", SnapshotSourceXCTest, "runningForeground"},
		{"private AX tree", `{"ok":true,"data":{"source":"privateAX","nodes":[]}}`, "", SnapshotSourcePrivateAX, ""},
		{"failed read keeps appState", snapFailedBody, ErrSnapshotFailed, "", "runningBackgroundSuspended"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := replyServer(t, tt.body)
			data, err := c.Call(context.Background(), Command{Command: CmdSnapshot})
			checkRunnerErrCode(t, err, tt.wantErrCode)
			if got := isSnapshotFailure(err); got != (tt.wantErrCode == ErrSnapshotFailed) {
				t.Errorf("isSnapshotFailure = %v", got)
			}
			if data == nil {
				t.Fatal("data is nil")
			}
			if data.Source != tt.wantSource || data.AppState != tt.wantState {
				t.Errorf("source=%q appState=%q, want %q %q", data.Source, data.AppState, tt.wantSource, tt.wantState)
			}
		})
	}
}

func TestIsSnapshotFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{"other runner code", &RunnerError{Code: ErrElementNotFound}, false},
		{"snapshot failed", &RunnerError{Code: ErrSnapshotFailed}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSnapshotFailure(tt.err); got != tt.want {
				t.Errorf("isSnapshotFailure(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestElementNotFound(t *testing.T) {
	sel := flow.Selector{ID: "row"}
	plain := elementNotFound(sel, nil)
	if plain.Error() != "element not found: id=row" {
		t.Errorf("plain = %q", plain)
	}
	snapErr := &RunnerError{Code: ErrSnapshotFailed, Message: "timed out"}
	wrapped := elementNotFound(sel, snapErr)
	if !strings.Contains(wrapped.Error(), "last snapshot failed") || !isSnapshotFailure(errors.Unwrap(wrapped)) {
		t.Errorf("wrapped = %q, want the snapshot failure wrapped", wrapped)
	}
}

// TestFindElementPollsThroughSnapshotFailure: a transient unreadable snapshot
// is retried like a miss; a persistent one is named in the timeout error; any
// other runner error still ends the find at once.
func TestFindElementPollsThroughSnapshotFailure(t *testing.T) {
	tests := []struct {
		name     string
		bodyFn   func(int64) string
		optional bool
		wantNode bool
		wantErr  string
	}{
		{"recovers after failures", failFirst(2, snapRowBody), false, true, ""},
		{"persistent failure named", always(snapFailedBody), false, false, "last snapshot failed"},
		{"persistent failure optional", always(snapFailedBody), true, false, ""},
		{"absent element plain error", always(snapEmptyBody), false, false, "element not found: id=row"},
		{
			"other runner error returned",
			always(`{"ok":false,"error":{"code":"XCUI_EXCEPTION","message":"boom"}}`),
			false, false, "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := scriptedDriver(t, tt.bodyFn)
			// A non-simple selector skips the querySelector strategy, so
			// every request is a snapshot.
			node, err := d.findElement(flow.Selector{ID: "row", Enabled: boolPtr(true)}, tt.optional, 300)
			if (node != nil) != tt.wantNode {
				t.Errorf("node = %v, wantNode %v", node, tt.wantNode)
			}
			checkErrContains(t, err, tt.wantErr)
		})
	}
}

// TestAssertVisibleCountSnapshotFailure: a transient unreadable snapshot does
// not end the assertion early, and a persistent one is named as the cause.
func TestAssertVisibleCountSnapshotFailure(t *testing.T) {
	tests := []struct {
		name        string
		bodyFn      func(int64) string
		count       string
		wantSuccess bool
		wantMsg     string
	}{
		{"never readable", always(snapFailedBody), "1", false, "SNAPSHOT_FAILED"},
		{"readable later, wrong count", failFirst(2, snapEmptyBody), "1", false, "found 0"},
		{"one wanted, readable later", failFirst(2, snapRowBody), "1", true, "1 visible"},
		{
			"other runner error ends at once",
			always(`{"ok":false,"error":{"code":"XCUI_EXCEPTION","message":"boom"}}`),
			"1", false, "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := scriptedDriver(t, tt.bodyFn)
			step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "row"}, Count: tt.count}
			step.TimeoutMs = 300
			res := d.handleAssertVisible(step)
			if res.Success != tt.wantSuccess || !strings.Contains(res.Message, tt.wantMsg) {
				t.Errorf("success=%v message=%q, want %v containing %q", res.Success, res.Message, tt.wantSuccess, tt.wantMsg)
			}
		})
	}
}

// TestWaitUntilNotVisibleSnapshotFailure: "not visible" is only concluded from
// a snapshot that was actually read.
func TestWaitUntilNotVisibleSnapshotFailure(t *testing.T) {
	tests := []struct {
		name        string
		bodyFn      func(int64) string
		wantSuccess bool
	}{
		{"never readable times out", always(snapFailedBody), false},
		{"readable later passes", failFirst(1, snapEmptyBody), true},
		{"still visible times out", always(snapRowBody), false},
		{"other runner error fails", always(`{"ok":false,"error":{"code":"X","message":"boom"}}`), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := scriptedDriver(t, tt.bodyFn)
			step := &flow.WaitUntilStep{NotVisible: &flow.Selector{ID: "row"}}
			step.TimeoutMs = 300
			if res := d.handleWaitUntil(step); res.Success != tt.wantSuccess {
				t.Errorf("success=%v (%s), want %v", res.Success, res.Message, tt.wantSuccess)
			}
		})
	}
}

// TestAssertNotVisibleSnapshotFailure: an unreadable screen is an error, not a
// pass. Older runners sent an empty tree here, which asserted absence falsely.
func TestAssertNotVisibleSnapshotFailure(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantSuccess bool
	}{
		{"unreadable fails", snapFailedBody, false},
		{"read and absent passes", snapEmptyBody, true},
		{"read and present fails", snapRowBody, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := scriptedDriver(t, always(tt.body))
			res := d.handleAssertNotVisible(&flow.AssertNotVisibleStep{Selector: flow.Selector{ID: "row"}})
			if res.Success != tt.wantSuccess {
				t.Errorf("success=%v (%s), want %v", res.Success, res.Message, tt.wantSuccess)
			}
		})
	}
}

// checkErrContains fails unless err contains want, or is nil when want is empty.
func checkErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want it to contain %q", err, want)
	}
}

// TestNoTargetAppIsNotASnapshotFailure: an unresolvable frontmost app is its
// own error, not a transient unreadable tree to poll through.
func TestNoTargetAppIsNotASnapshotFailure(t *testing.T) {
	c := replyServer(t, `{"ok":false,"error":{"code":"NO_TARGET_APP","message":"no app"}}`)
	_, err := c.Call(context.Background(), Command{Command: CmdSnapshot})
	checkRunnerErrCode(t, err, ErrNoTargetApp)
	if isSnapshotFailure(err) {
		t.Error("NO_TARGET_APP must not be treated as a snapshot failure")
	}
}
