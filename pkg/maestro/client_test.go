package maestro

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

// mockServer creates an httptest.Server that upgrades to WebSocket
// and calls the handler for each incoming text message.
// The handler receives the parsed Request and returns a Response to send back.
type wsHandler func(req Request) interface{}

func newMockWSServer(t *testing.T, handler wsHandler) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("accept error: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		conn.SetReadLimit(32 * 1024 * 1024)

		ctx := r.Context()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}

			var req Request
			if err := json.Unmarshal(data, &req); err != nil {
				t.Logf("unmarshal error: %v", err)
				return
			}

			result := handler(req)

			var respData []byte
			switch v := result.(type) {
			case *ErrorPayload:
				resp := Response{ID: req.ID, Error: v}
				respData, _ = json.Marshal(resp)
			default:
				raw, _ := json.Marshal(v)
				resp := Response{ID: req.ID, Result: json.RawMessage(raw)}
				respData, _ = json.Marshal(resp)
			}

			if err := conn.Write(ctx, websocket.MessageText, respData); err != nil {
				return
			}
		}
	}))
}

// newMockWSServerWithPush creates a server that also supports pushing events.
// Returns the server and a function to push events.
func newMockWSServerWithPush(t *testing.T, handler wsHandler) (*httptest.Server, func(Event)) {
	t.Helper()

	var mu sync.Mutex
	var activeConn *websocket.Conn
	var activeCtx context.Context

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("accept error: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		conn.SetReadLimit(32 * 1024 * 1024)

		mu.Lock()
		activeConn = conn
		activeCtx = r.Context()
		mu.Unlock()

		ctx := r.Context()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}

			var req Request
			if err := json.Unmarshal(data, &req); err != nil {
				return
			}

			result := handler(req)
			var respData []byte
			switch v := result.(type) {
			case *ErrorPayload:
				resp := Response{ID: req.ID, Error: v}
				respData, _ = json.Marshal(resp)
			default:
				raw, _ := json.Marshal(v)
				resp := Response{ID: req.ID, Result: json.RawMessage(raw)}
				respData, _ = json.Marshal(resp)
			}

			if err := conn.Write(ctx, websocket.MessageText, respData); err != nil {
				return
			}
		}
	}))

	pushEvent := func(evt Event) {
		mu.Lock()
		c := activeConn
		ctx := activeCtx
		mu.Unlock()
		if c == nil {
			return
		}
		data, _ := json.Marshal(evt)
		_ = c.Write(ctx, websocket.MessageText, data)
	}

	return server, pushEvent
}

// tcpClientFromServer creates a Client pointing at the test server's TCP address.
func tcpClientFromServer(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	// Extract port from server URL (e.g., "http://127.0.0.1:12345")
	addr := server.Listener.Addr().String()
	parts := strings.Split(addr, ":")
	port := 0
	if len(parts) >= 2 {
		_, _ = json.Number(parts[len(parts)-1]).Int64()
		// Use the full URL approach instead
	}
	_ = port

	// Use a custom dialer client approach - connect directly to the URL
	client := &Client{
		done:   make(chan struct{}),
		logger: newLogger(),
	}

	// Connect using the server URL directly
	client.ctx, client.cancel = context.WithCancel(context.Background())
	dialCtx, dialCancel := context.WithTimeout(client.ctx, 5*time.Second)
	defer dialCancel()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial mock server: %v", err)
	}
	conn.SetReadLimit(32 * 1024 * 1024)
	client.conn = conn
	go client.readLoop()

	return client
}

func TestClientCallSuccess(t *testing.T) {
	server := newMockWSServer(t, func(req Request) interface{} {
		if req.Method != "Session.status" {
			t.Errorf("expected Session.status, got %s", req.Method)
		}
		return map[string]interface{}{"ready": true}
	})
	defer server.Close()

	client := tcpClientFromServer(t, server)
	defer client.Close()

	resp, err := client.Call("Session.status", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	var result struct {
		Ready bool `json:"ready"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.Ready {
		t.Error("expected ready=true")
	}
}

func TestClientCallError(t *testing.T) {
	server := newMockWSServer(t, func(req Request) interface{} {
		return &ErrorPayload{Code: "not_found", Message: "element not found"}
	})
	defer server.Close()

	client := tcpClientFromServer(t, server)
	defer client.Close()

	_, err := client.Call("UI.findElement", map[string]interface{}{
		"strategy": "id", "selector": "missing",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "element not found") {
		t.Errorf("expected error containing 'element not found', got: %v", err)
	}
}

func TestClientCallTimeout(t *testing.T) {
	// Server that never responds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		// Read but never respond
		ctx := r.Context()
		for {
			_, _, err := conn.Read(ctx)
			if err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client := tcpClientFromServer(t, server)
	defer client.Close()

	_, err := client.CallWithTimeout("Session.status", nil, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestClientCallWithParams(t *testing.T) {
	server := newMockWSServer(t, func(req Request) interface{} {
		if req.Method != "Gesture.click" {
			t.Errorf("expected Gesture.click, got %s", req.Method)
		}

		// Verify params were passed
		params, _ := json.Marshal(req.Params)
		var p map[string]interface{}
		json.Unmarshal(params, &p)

		x, _ := p["x"].(float64)
		y, _ := p["y"].(float64)
		if x != 100 || y != 200 {
			t.Errorf("expected x=100,y=200, got x=%v,y=%v", x, y)
		}

		return map[string]interface{}{}
	})
	defer server.Close()

	client := tcpClientFromServer(t, server)
	defer client.Close()

	_, err := client.Call("Gesture.click", map[string]interface{}{
		"x": 100, "y": 200,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientMultipleConcurrentCalls(t *testing.T) {
	server := newMockWSServer(t, func(req Request) interface{} {
		// Simulate some work
		time.Sleep(10 * time.Millisecond)
		return map[string]interface{}{"method": req.Method}
	})
	defer server.Close()

	client := tcpClientFromServer(t, server)
	defer client.Close()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := client.Call("test.method", map[string]interface{}{"n": n})
			if err != nil {
				t.Errorf("call %d failed: %v", n, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestClientClose(t *testing.T) {
	server := newMockWSServer(t, func(req Request) interface{} {
		return map[string]interface{}{}
	})
	defer server.Close()

	client := tcpClientFromServer(t, server)

	// Close should not error
	if err := client.Close(); err != nil {
		// Ignore websocket close errors that may happen due to concurrent close
		t.Logf("close returned: %v (may be expected)", err)
	}

	// Call after close should error
	_, err := client.Call("test", nil)
	if err == nil {
		t.Error("expected error after close")
	}
}

func TestClientEvents(t *testing.T) {
	server, pushEvent := newMockWSServerWithPush(t, func(req Request) interface{} {
		return map[string]interface{}{}
	})
	defer server.Close()

	client := tcpClientFromServer(t, server)
	defer client.Close()

	// Register event handler
	received := make(chan json.RawMessage, 1)
	client.OnEvent("Input.keyboardStateChanged", func(params json.RawMessage) {
		received <- params
	})

	// Make a call first to ensure connection is established
	_, err := client.Call("Session.status", nil)
	if err != nil {
		t.Fatalf("initial call failed: %v", err)
	}

	// Push an event from the server
	pushEvent(Event{
		Event:  "Input.keyboardStateChanged",
		Params: json.RawMessage(`{"visible":true}`),
	})

	// Wait for event
	select {
	case params := <-received:
		var info KeyboardInfo
		if err := json.Unmarshal(params, &info); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if !info.Visible {
			t.Error("expected visible=true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestClientRemoveEvent(t *testing.T) {
	server, pushEvent := newMockWSServerWithPush(t, func(req Request) interface{} {
		return map[string]interface{}{}
	})
	defer server.Close()

	client := tcpClientFromServer(t, server)
	defer client.Close()

	called := make(chan struct{}, 1)
	client.OnEvent("test.event", func(params json.RawMessage) {
		called <- struct{}{}
	})

	// Remove the handler
	client.RemoveEvent("test.event")

	// Make a call to ensure connection is established
	_, _ = client.Call("Session.status", nil)

	// Push an event
	pushEvent(Event{Event: "test.event", Params: json.RawMessage(`{}`)})

	// Handler should NOT be called
	select {
	case <-called:
		t.Error("event handler should not have been called after removal")
	case <-time.After(200 * time.Millisecond):
		// Expected — handler was removed
	}
}

func TestErrorPayloadError(t *testing.T) {
	e := &ErrorPayload{Code: "not_found", Message: "element not found"}
	expected := "not_found: element not found"
	if e.Error() != expected {
		t.Errorf("expected %q, got %q", expected, e.Error())
	}
}

// TestNewClient covers the Unix-socket constructor (just field setup; no connection).
func TestNewClient(t *testing.T) {
	c := NewClient("/tmp/foo.sock")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.socketPath != "/tmp/foo.sock" {
		t.Errorf("socketPath: got %q", c.socketPath)
	}
	if c.done == nil {
		t.Error("done channel must be initialised")
	}
	if c.logger == nil {
		t.Error("logger must be initialised")
	}
}

// TestNewClientTCP covers the TCP constructor.
func TestNewClientTCP(t *testing.T) {
	c := NewClientTCP(12345)
	if c == nil {
		t.Fatal("NewClientTCP returned nil")
	}
	if c.tcpPort != 12345 {
		t.Errorf("tcpPort: got %d", c.tcpPort)
	}
	if c.socketPath != "" {
		t.Errorf("socketPath should be empty for TCP, got %q", c.socketPath)
	}
}

// TestSetLogPath swaps the logger to write to the provided path.
func TestSetLogPath(t *testing.T) {
	c := NewClient("/tmp/x.sock")
	path := t.TempDir() + "/maestro.log"
	c.SetLogPath(path)
	if c.logger == nil {
		t.Fatal("logger nil after SetLogPath")
	}
	// Bad path should not panic (silently swallowed per implementation).
	c.SetLogPath("/nonexistent/cannot/be/created/x.log")
}

// TestConnect_BadPath returns an error rather than panicking.
func TestConnect_BadPath(t *testing.T) {
	c := NewClient("/nonexistent/socket/path.sock")
	if err := c.Connect(); err == nil {
		t.Error("Connect to nonexistent socket should fail")
		_ = c.Close()
	}
}

// TestConnectWithTimeout_BadPath honours the custom timeout.
func TestConnectWithTimeout_BadPath(t *testing.T) {
	c := NewClient("/nonexistent/socket/path.sock")
	start := time.Now()
	err := c.ConnectWithTimeout(50 * time.Millisecond)
	if err == nil {
		t.Error("Connect to nonexistent socket should fail")
		_ = c.Close()
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("ConnectWithTimeout exceeded its budget: %v", elapsed)
	}
}
