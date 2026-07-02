package flutter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

// mockVMServiceHandler handles JSON-RPC requests for testing.
type mockVMServiceHandler struct {
	isolateID      string
	semanticsData  string
	widgetTreeData string
}

func (h *mockVMServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	conn.SetReadLimit(16 * 1024 * 1024)

	ctx := r.Context()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return
		}

		var result interface{}
		switch req.Method {
		case "getVM":
			result = map[string]interface{}{
				"isolates": []map[string]interface{}{
					{"id": h.isolateID, "name": "main"},
				},
			}
		case "getIsolate":
			result = map[string]interface{}{
				"extensionRPCs": []string{
					"ext.flutter.debugDumpSemanticsTreeInTraversalOrder",
					"ext.flutter.debugDumpApp",
				},
			}
		case "ext.flutter.debugDumpSemanticsTreeInTraversalOrder":
			result = map[string]interface{}{
				"data": h.semanticsData,
			}
		case "ext.flutter.debugDumpApp":
			result = map[string]interface{}{
				"data": h.widgetTreeData,
			}
		default:
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error":   map[string]interface{}{"code": -32601, "message": "method not found"},
			}
			respData, _ := json.Marshal(resp)
			_ = conn.Write(ctx, websocket.MessageText, respData)
			continue
		}

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		}
		respData, _ := json.Marshal(resp)
		_ = conn.Write(ctx, websocket.MessageText, respData)
	}
}

func startMockVMService(t *testing.T, semanticsData string) (wsURL string, cleanup func()) {
	t.Helper()
	return startMockVMServiceFull(t, semanticsData, "")
}

func startMockVMServiceFull(t *testing.T, semanticsData, widgetTreeData string) (wsURL string, cleanup func()) {
	t.Helper()
	handler := &mockVMServiceHandler{
		isolateID:      "isolates/123",
		semanticsData:  semanticsData,
		widgetTreeData: widgetTreeData,
	}
	server := httptest.NewServer(handler)

	wsURL = "ws" + strings.TrimPrefix(server.URL, "http")
	return wsURL, server.Close
}

func TestConnect(t *testing.T) {
	wsURL, cleanup := startMockVMService(t, "SemanticsNode#0\n")
	defer cleanup()

	client, err := Connect(wsURL)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	if client.isolateID != "isolates/123" {
		t.Errorf("isolateID = %q, want %q", client.isolateID, "isolates/123")
	}
}

func TestConnect_InvalidURL(t *testing.T) {
	_, err := Connect("ws://127.0.0.1:1/invalid/ws")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestGetSemanticsTree(t *testing.T) {
	expectedDump := `SemanticsNode#0
 Rect.fromLTRB(0.0, 0.0, 411.4, 890.3)
 scaled by 2.6x
 label: "Root"
`
	wsURL, cleanup := startMockVMService(t, expectedDump)
	defer cleanup()

	client, err := Connect(wsURL)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	dump, err := client.GetSemanticsTree()
	if err != nil {
		t.Fatalf("GetSemanticsTree: %v", err)
	}
	if dump != expectedDump {
		t.Errorf("dump = %q, want %q", dump, expectedDump)
	}
}

func TestGetWidgetTree(t *testing.T) {
	expectedDump := `MyApp
 └TextField(decoration: InputDecoration(labelText: "Email", hintText: "Enter your email"))
`
	wsURL, cleanup := startMockVMServiceFull(t, "SemanticsNode#0\n", expectedDump)
	defer cleanup()

	client, err := Connect(wsURL)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	dump, err := client.GetWidgetTree()
	if err != nil {
		t.Fatalf("GetWidgetTree: %v", err)
	}
	if dump != expectedDump {
		t.Errorf("dump = %q, want %q", dump, expectedDump)
	}
}

func TestConnectNoFlutterIsolate(t *testing.T) {
	// Create a server that returns an isolate without Flutter extensions
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx := r.Context()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}

			var req jsonRPCRequest
			if err := json.Unmarshal(data, &req); err != nil {
				return
			}

			var result interface{}
			switch req.Method {
			case "getVM":
				result = map[string]interface{}{
					"isolates": []map[string]interface{}{
						{"id": "isolates/456", "name": "main"},
					},
				}
			case "getIsolate":
				// No Flutter extensions
				result = map[string]interface{}{
					"extensionRPCs": []string{},
				}
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  result,
			}
			respData, _ := json.Marshal(resp)
			_ = conn.Write(ctx, websocket.MessageText, respData)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	_, err := Connect(wsURL)
	if err == nil {
		t.Error("expected error when no Flutter isolate found")
	}
}

func TestCallRPCError(t *testing.T) {
	// Create a server that always returns an error
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx := r.Context()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}

			var req jsonRPCRequest
			_ = json.Unmarshal(data, &req)

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error":   map[string]interface{}{"code": -32000, "message": "test error"},
			}
			respData, _ := json.Marshal(resp)
			_ = conn.Write(ctx, websocket.MessageText, respData)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Manually create client to test call error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	c := &VMServiceClient{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}

	_, err = c.call("someMethod", nil)
	if err == nil {
		t.Error("expected error from RPC")
	}
	if !strings.Contains(err.Error(), "test error") {
		t.Errorf("error = %q, want to contain 'test error'", err.Error())
	}
}
