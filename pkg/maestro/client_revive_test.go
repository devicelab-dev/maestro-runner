package maestro

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// dyingAgent answers every call, except that it drops the connection, the
// way a crashed agent does, on the first call to dropOn.
func dyingAgent(t *testing.T, dropOn string) (port int, connections *atomic.Int32) {
	t.Helper()
	var dropped atomic.Bool
	connections = &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		connections.Add(1)
		ctx := r.Context()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var req Request
			if json.Unmarshal(data, &req) != nil {
				return
			}
			if req.Method == dropOn && !dropped.Swap(true) {
				_ = conn.CloseNow()
				return
			}
			raw, _ := json.Marshal(map[string]string{"method": req.Method})
			out, _ := json.Marshal(Response{ID: req.ID, Result: raw})
			if conn.Write(ctx, websocket.MessageText, out) != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	return server.Listener.Addr().(*net.TCPAddr).Port, connections
}

func revivingClient(t *testing.T, port int, revives *atomic.Int32) *Client {
	t.Helper()
	c := NewClientTCP(port)
	if err := c.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	if revives != nil {
		c.SetReviver(func() error { revives.Add(1); return c.Connect() })
	}
	return c
}

func TestClientRevivesAndResendsReadOnly(t *testing.T) {
	port, connections := dyingAgent(t, "UI.snapshot")
	var revives atomic.Int32
	c := revivingClient(t, port, &revives)

	start := time.Now()
	resp, err := c.Call("UI.snapshot", nil)
	if err != nil || !strings.Contains(string(resp.Result), "UI.snapshot") {
		t.Fatalf("snapshot after the agent dropped: %v %v", resp, err)
	}
	if revives.Load() != 1 || connections.Load() != 2 {
		t.Errorf("revives=%d connections=%d, want 1 and 2", revives.Load(), connections.Load())
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %v: a dropped connection must fail the call at once, not at its timeout", elapsed)
	}
}

func TestClientRevivesButDoesNotResendActions(t *testing.T) {
	port, _ := dyingAgent(t, "Input.sendKeyActions")
	var revives atomic.Int32
	c := revivingClient(t, port, &revives)

	_, err := c.Call("Input.sendKeyActions", map[string]string{"text": "x"})
	if err == nil || !strings.Contains(err.Error(), "not re-sent") {
		t.Fatalf("err = %v, want the action reported as not re-sent", err)
	}
	if revives.Load() != 1 {
		t.Errorf("revives = %d, want 1", revives.Load())
	}
	if _, err := c.Call("UI.treeHash", nil); err != nil {
		t.Errorf("the next call after the revive failed: %v", err)
	}
}

func TestClientWithoutReviverFailsFast(t *testing.T) {
	port, _ := dyingAgent(t, "UI.snapshot")
	c := revivingClient(t, port, nil)

	start := time.Now()
	_, err := c.CallWithTimeout("UI.snapshot", nil, 20*time.Second)
	if err == nil {
		t.Fatal("expected an error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %v, want a fast failure", elapsed)
	}
}
