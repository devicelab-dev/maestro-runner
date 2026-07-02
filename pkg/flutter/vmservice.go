package flutter

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	dialTimeout    = 10 * time.Second
	requestTimeout = 15 * time.Second
)

// VMServiceClient communicates with the Flutter VM Service over WebSocket JSON-RPC 2.0.
type VMServiceClient struct {
	conn      *websocket.Conn
	isolateID string
	ctx context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	nextID    int
}

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Connect dials the Flutter VM Service WebSocket and discovers the Flutter isolate.
func Connect(wsURL string) (*VMServiceClient, error) {
	ctx, cancel := context.WithCancel(context.Background())

	dialCtx, dialCancel := context.WithTimeout(ctx, dialTimeout)
	defer dialCancel()

	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("dial VM service: %w", err)
	}

	// Increase read limit for large semantics trees
	conn.SetReadLimit(16 * 1024 * 1024) // 16 MB

	c := &VMServiceClient{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}

	// Find the Flutter isolate
	if err := c.findFlutterIsolate(); err != nil {
		conn.Close(websocket.StatusNormalClosure, "")
		cancel()
		return nil, fmt.Errorf("find flutter isolate: %w", err)
	}

	return c, nil
}

// ConnectUnix dials the Flutter VM Service over a Unix socket.
// Used with adb forward localfilesystem:<socketPath> tcp:<port>.
func ConnectUnix(socketPath, token string) (*VMServiceClient, error) {
	ctx, cancel := context.WithCancel(context.Background())

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, dialTimeout)
	defer dialCancel()

	// Host is ignored — transport always dials the Unix socket
	wsURL := fmt.Sprintf("ws://localhost/%s/ws", token)
	conn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("dial VM service via socket: %w", err)
	}

	conn.SetReadLimit(16 * 1024 * 1024) // 16 MB

	c := &VMServiceClient{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}

	if err := c.findFlutterIsolate(); err != nil {
		conn.Close(websocket.StatusNormalClosure, "")
		cancel()
		return nil, fmt.Errorf("find flutter isolate: %w", err)
	}

	return c, nil
}

// Close closes the WebSocket connection. Safe to call on nil receiver.
func (c *VMServiceClient) Close() error {
	if c == nil {
		return nil
	}
	c.cancel()
	return c.conn.Close(websocket.StatusNormalClosure, "")
}

// GetSemanticsTree calls debugDumpSemanticsTreeInTraversalOrder and returns the text dump.
func (c *VMServiceClient) GetSemanticsTree() (string, error) {
	result, err := c.call("ext.flutter.debugDumpSemanticsTreeInTraversalOrder", map[string]string{
		"isolateId": c.isolateID,
	})
	if err != nil {
		return "", err
	}

	var data struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(result, &data); err != nil {
		return "", fmt.Errorf("parse semantics result: %w", err)
	}
	return data.Data, nil
}

// GetWidgetTree calls debugDumpApp and returns the widget tree text dump.
func (c *VMServiceClient) GetWidgetTree() (string, error) {
	result, err := c.call("ext.flutter.debugDumpApp", map[string]string{
		"isolateId": c.isolateID,
	})
	if err != nil {
		return "", err
	}

	var data struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(result, &data); err != nil {
		return "", fmt.Errorf("parse widget tree result: %w", err)
	}
	return data.Data, nil
}

// findFlutterIsolate discovers the isolate that has Flutter extensions.
func (c *VMServiceClient) findFlutterIsolate() error {
	// Call getVM to get list of isolates
	vmResult, err := c.call("getVM", nil)
	if err != nil {
		return fmt.Errorf("getVM: %w", err)
	}

	var vm struct {
		Isolates []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"isolates"`
	}
	if err := json.Unmarshal(vmResult, &vm); err != nil {
		return fmt.Errorf("parse VM result: %w", err)
	}

	// Check each isolate for Flutter extensions
	for _, iso := range vm.Isolates {
		isoResult, err := c.call("getIsolate", map[string]string{
			"isolateId": iso.ID,
		})
		if err != nil {
			continue
		}

		var isolate struct {
			ExtensionRPCs []string `json:"extensionRPCs"`
		}
		if err := json.Unmarshal(isoResult, &isolate); err != nil {
			continue
		}

		for _, ext := range isolate.ExtensionRPCs {
			if ext == "ext.flutter.debugDumpSemanticsTreeInTraversalOrder" {
				c.isolateID = iso.ID
				return nil
			}
		}
	}

	return fmt.Errorf("no Flutter isolate found")
}

// call sends a JSON-RPC 2.0 request and returns the result.
func (c *VMServiceClient) call(method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := strconv.Itoa(c.nextID)
	c.mu.Unlock()

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(c.ctx, requestTimeout)
	defer cancel()

	if err := c.conn.Write(ctx, websocket.MessageText, data); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Read responses until we get the one matching our ID.
	// The VM Service may send events or other responses in between.
	for {
		_, respData, err := c.conn.Read(ctx)
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal(respData, &resp); err != nil {
			continue // Skip non-JSON messages
		}

		if resp.ID != id {
			continue // Skip events and responses for other requests
		}

		if resp.Error != nil {
			if len(resp.Error.Data) > 0 {
				return nil, fmt.Errorf("RPC error %d: %s (data: %s)", resp.Error.Code, resp.Error.Message, string(resp.Error.Data))
			}
			return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}

		return resp.Result, nil
	}
}
