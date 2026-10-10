package maestro

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const (
	defaultCallTimeout = 30 * time.Second
	defaultDialTimeout = 10 * time.Second
)

// Client communicates with the DeviceLab on-device driver over WebSocket.
type Client struct {
	// connMu guards conn, ctx, cancel and done, which a revive replaces.
	connMu sync.RWMutex
	conn   *websocket.Conn

	// lost is set when the connection to the agent drops; the next call
	// revives it (see SetReviver).
	lost     atomic.Bool
	reviver  func() error
	reviveMu sync.Mutex

	// Connection parameters — exactly one will be set
	socketPath string
	tcpPort    int

	// Request ID counter
	nextID atomic.Int64

	// Pending requests: id → channel
	pending sync.Map // map[int64]chan *Response

	// Event handlers
	events sync.Map // map[string]EventHandler

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	logger *log.Logger
}

// NewClient creates a client that connects via Unix socket.
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		done:       make(chan struct{}),
		logger:     newLogger(),
	}
}

// NewClientTCP creates a client that connects via TCP port.
func NewClientTCP(port int) *Client {
	return &Client{
		tcpPort: port,
		done:    make(chan struct{}),
		logger:  newLogger(),
	}
}

func newLogger() *log.Logger {
	f, err := os.OpenFile("/tmp/devicelab-driver-client.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return log.New(io.Discard, "", 0)
	}
	return log.New(f, "", log.Ltime|log.Lmicroseconds)
}

// SetLogPath sets the log file path.
func (c *Client) SetLogPath(path string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	c.logger = log.New(f, "", log.Ltime|log.Lmicroseconds)
}

// Connect dials the WebSocket server and starts the read loop.
func (c *Client) Connect() error {
	return c.ConnectWithTimeout(defaultDialTimeout)
}

// ConnectWithTimeout dials with a custom timeout.
func (c *Client) ConnectWithTimeout(timeout time.Duration) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.done = make(chan struct{})

	dialCtx, dialCancel := context.WithTimeout(c.ctx, timeout)
	defer dialCancel()

	var conn *websocket.Conn
	var err error

	if c.socketPath != "" {
		httpClient := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", c.socketPath)
				},
			},
		}
		conn, _, err = websocket.Dial(dialCtx, "ws://localhost/ws", &websocket.DialOptions{
			HTTPClient: httpClient,
		})
	} else {
		url := fmt.Sprintf("ws://127.0.0.1:%d/ws", c.tcpPort)
		conn, _, err = websocket.Dial(dialCtx, url, nil)
	}
	if err != nil {
		c.cancel()
		return fmt.Errorf("websocket dial: %w", err)
	}

	// Allow large messages (screenshots can be several MB)
	conn.SetReadLimit(32 * 1024 * 1024) // 32 MB

	c.conn = conn
	c.lost.Store(false)
	go c.readLoop(conn, c.ctx, c.done)
	return nil
}

// SetTCPPort changes the local port a TCP client connects to, for a revive
// whose port forward came back on another port.
func (c *Client) SetTCPPort(port int) { c.tcpPort = port }

// SetReviver installs the function that brings a dropped agent back: it
// restarts the agent on the device, reconnects (Connect) and recreates the
// session. Without one, a dropped connection fails every later call.
func (c *Client) SetReviver(r func() error) { c.reviver = r }

// readOnlyMethods have no effect on the device, so after a revive they are
// sent again. An action is not: the agent that died may have performed it.
var readOnlyMethods = map[string]bool{
	"UI.snapshot": true, "UI.getSource": true, "UI.activeElement": true, "UI.treeHash": true,
	"UI.screenshot": true, "UI.findElement": true, "UI.findElements": true, "UI.waitForSettle": true,
	"UI.detectWebView": true, "UI.viewHierarchy": true,
}

// errConnectionLost is what a call gets when the connection drops under it.
var errConnectionLost = &ErrorPayload{Code: "connection_lost", Message: "the connection to the DeviceLab agent was lost"}

// revive brings a dropped connection back once, however many calls notice.
func (c *Client) revive() error {
	c.reviveMu.Lock()
	defer c.reviveMu.Unlock()
	if !c.lost.Load() {
		return nil // another call already revived it
	}
	c.logger.Printf("connection lost; restarting the agent")
	c.connMu.RLock()
	old, cancel := c.conn, c.cancel
	c.connMu.RUnlock()
	if cancel != nil {
		cancel()
	}
	if old != nil {
		_ = old.CloseNow()
	}
	if err := c.reviver(); err != nil {
		c.logger.Printf("restart failed: %v", err)
		return err
	}
	c.logger.Printf("agent restarted")
	return nil
}

// Call sends a request and waits for the matching response with the default timeout.
func (c *Client) Call(method string, params interface{}) (*Response, error) {
	return c.CallWithTimeout(method, params, defaultCallTimeout)
}

// CallWithTimeout sends a request and waits for the matching response. A
// connection that has dropped is revived first; a call the drop interrupted
// is sent again after the revive when it is read-only.
func (c *Client) CallWithTimeout(method string, params interface{}, timeout time.Duration) (*Response, error) {
	if c.lost.Load() && c.reviver != nil {
		if err := c.revive(); err != nil {
			return nil, fmt.Errorf("%s: agent connection lost (restart failed: %v)", method, err)
		}
	}
	resp, err := c.callOnce(method, params, timeout)
	if err == nil || !c.lost.Load() || c.reviver == nil {
		return resp, err
	}
	if rerr := c.revive(); rerr != nil {
		return nil, fmt.Errorf("%w (restart failed: %v)", err, rerr)
	}
	if !readOnlyMethods[method] {
		return nil, fmt.Errorf("%w (agent restarted; %s not re-sent)", err, method)
	}
	return c.callOnce(method, params, timeout)
}

func (c *Client) callOnce(method string, params interface{}, timeout time.Duration) (*Response, error) {
	id := c.nextID.Add(1)

	req := Request{
		ID:     id,
		Method: method,
		Params: params,
	}

	// Register pending channel before sending
	ch := make(chan *Response, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	start := time.Now()
	c.logger.Printf("→ %s id=%d", method, id)

	c.connMu.RLock()
	conn, ctx := c.conn, c.ctx
	c.connMu.RUnlock()

	writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer writeCancel()

	if err := conn.Write(writeCtx, websocket.MessageText, data); err != nil {
		if ctx.Err() == nil {
			c.lost.Store(true)
		}
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Wait for response
	select {
	case resp := <-ch:
		elapsed := time.Since(start)
		if resp.Error != nil {
			c.logger.Printf("← %s id=%d [%v] ERR: %s", method, id, elapsed, resp.Error.Message)
			return nil, resp.Error
		}
		c.logger.Printf("← %s id=%d [%v] OK", method, id, elapsed)
		return resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for response to %s (id=%d)", method, id)
	case <-ctx.Done():
		if c.lost.Load() {
			return nil, errConnectionLost
		}
		return nil, fmt.Errorf("client closed")
	}
}

// readLoop reads frames from one connection and dispatches them. When the
// connection drops (not a Close), every call waiting on it fails at once
// instead of waiting out its timeout, and the next call revives it.
func (c *Client) readLoop(conn *websocket.Conn, ctx context.Context, done chan struct{}) {
	defer close(done)

	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // normal shutdown, or a revive replaced this connection
			}
			c.logger.Printf("read error: %v", err)
			c.lost.Store(true)
			c.pending.Range(func(_, ch any) bool {
				select {
				case ch.(chan *Response) <- &Response{Error: errConnectionLost}:
				default:
				}
				return true
			})
			return
		}

		if msgType == websocket.MessageBinary {
			c.dispatchBinary(data)
		} else {
			c.dispatch(data)
		}
	}
}

// dispatchBinary handles binary frames (e.g., screenshots).
// Format: [8-byte big-endian request ID][raw payload bytes]
func (c *Client) dispatchBinary(data []byte) {
	if len(data) < 8 {
		c.logger.Printf("binary frame too short: %d bytes", len(data))
		return
	}

	id := int64(binary.BigEndian.Uint64(data[:8]))
	resp := &Response{
		ID:         id,
		BinaryData: data[8:],
	}

	if ch, ok := c.pending.Load(id); ok {
		ch.(chan *Response) <- resp
	}
}

// dispatch routes an incoming frame to the correct handler.
func (c *Client) dispatch(data []byte) {
	// Peek at the JSON to determine type
	var peek rawMessage
	if err := json.Unmarshal(data, &peek); err != nil {
		c.logger.Printf("unmarshal frame error: %v", err)
		return
	}

	if peek.ID != nil {
		// Response — route to pending channel
		var resp Response
		if err := json.Unmarshal(data, &resp); err != nil {
			c.logger.Printf("unmarshal response error: %v", err)
			return
		}
		if ch, ok := c.pending.Load(resp.ID); ok {
			ch.(chan *Response) <- &resp
		}
		return
	}

	if peek.Event != "" {
		// Event — call registered handler
		var evt Event
		if err := json.Unmarshal(data, &evt); err != nil {
			c.logger.Printf("unmarshal event error: %v", err)
			return
		}
		if handler, ok := c.events.Load(evt.Event); ok {
			// Fire handler in a goroutine to avoid blocking readLoop
			go handler.(EventHandler)(evt.Params)
		}
		return
	}

	c.logger.Printf("unknown frame: %s", string(data))
}

// Close cleanly shuts down the connection.
func (c *Client) Close() error {
	c.connMu.RLock()
	conn, cancel, done := c.conn, c.cancel, c.done
	c.connMu.RUnlock()
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		err := conn.Close(websocket.StatusNormalClosure, "client closing")
		// Wait for readLoop to finish
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		return err
	}
	return nil
}
