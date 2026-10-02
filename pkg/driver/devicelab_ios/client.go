package devicelab_ios

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// readOnly commands have no effect on the device, so after the agent is
// restarted they may be sent again. An action is not: the agent that died may
// have performed it, and the action that killed it would kill it again.
var readOnly = map[string]bool{
	"status": true, "snapshot": true, "find": true, "settle": true, "screenshot": true,
}

// Reviver restarts a dead agent and returns its new port.
type Reviver func(ctx context.Context) (int, error)

// Client talks to one agent over keep-alive HTTP.
type Client struct {
	mu          sync.Mutex
	base        string
	http        *http.Client
	callTimeout time.Duration
	seq         atomic.Int64
	prefix      string
	reviver     Reviver
}

// NewClient returns a client for the agent on 127.0.0.1:port.
func NewClient(port int) *Client {
	return &Client{
		base: fmt.Sprintf("http://127.0.0.1:%d/", port),
		http: &http.Client{Transport: &http.Transport{
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     60 * time.Second,
			DisableCompression:  true,
		}},
		callTimeout: 60 * time.Second,
		prefix:      fmt.Sprintf("h%x-", time.Now().UnixNano()&0xffffff),
	}
}

// SetReviver installs the function that restarts a dead agent.
func (c *Client) SetReviver(r Reviver) { c.reviver = r }

// SetPort re-points the client at a restarted agent.
func (c *Client) SetPort(port int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.base = fmt.Sprintf("http://127.0.0.1:%d/", port)
}

func (c *Client) url() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base
}

// Call sends one command and returns the envelope. An `ok: false` answer is
// an *AgentError (with the envelope still returned). A dead agent is
// restarted once through the reviver; read-only commands are then re-sent.
func (c *Client) Call(ctx context.Context, cmd string, args *Args) (*Response, error) {
	id := fmt.Sprintf("%s%d", c.prefix, c.seq.Add(1))
	body, err := json.Marshal(struct {
		ID   string `json:"id"`
		Cmd  string `json:"cmd"`
		Args *Args  `json:"args,omitempty"`
	}{id, cmd, args})
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", cmd, err)
	}
	resp, err := c.send(ctx, cmd, id, body)
	if err == nil || !(isTransport(err) || isTimeout(err)) || c.reviver == nil || ctx.Err() != nil {
		return resp, err
	}
	port, rerr := c.reviver(ctx)
	if rerr != nil {
		return nil, fmt.Errorf("%w (restart failed: %v)", err, rerr)
	}
	c.SetPort(port)
	if !readOnly[cmd] {
		return nil, fmt.Errorf("%w (agent restarted; %s not re-sent)", err, cmd)
	}
	return c.send(ctx, cmd, id, body)
}

type transportError struct{ err error }

func (e transportError) Error() string { return "agent unreachable: " + e.err.Error() }
func (e transportError) Unwrap() error { return e.err }

// timeoutError is an agent that took the whole call timeout to answer: as
// good as dead, and restarted like one.
type timeoutError struct{ msg string }

func (e timeoutError) Error() string { return e.msg }

func isTimeout(err error) bool {
	var te timeoutError
	return errors.As(err, &te)
}

func isTransport(err error) bool {
	var te transportError
	return errors.As(err, &te)
}

func (c *Client) send(ctx context.Context, cmd, id string, body []byte) (*Response, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.callTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.url(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// The agent answers a repeated id from its journal, so the request is
	// idempotent; the header lets the transport retry it on a keep-alive
	// connection the agent had already closed.
	req.Header.Set("Idempotency-Key", id)
	start := time.Now()
	res, err := c.http.Do(req)
	if err != nil {
		logCall(cmd, time.Since(start), nil, len(body), 0, err)
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s aborted: %w", cmd, ctx.Err())
		}
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			return nil, timeoutError{fmt.Sprintf("%s: agent did not answer within %s", cmd, c.callTimeout)}
		}
		return nil, transportError{err}
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		logCall(cmd, time.Since(start), nil, len(body), 0, err)
		return nil, transportError{err}
	}
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		logCall(cmd, time.Since(start), nil, len(body), len(raw), err)
		return nil, fmt.Errorf("decode %s response: %w", cmd, err)
	}
	var callErr error
	if !resp.OK {
		callErr = &AgentError{Code: "UNKNOWN", Message: "agent error"}
		if resp.Error != nil {
			callErr = &AgentError{Code: resp.Error.Code, Message: resp.Error.Message}
		}
	}
	logCall(cmd, time.Since(start), &resp, len(body), len(raw), callErr)
	return &resp, callErr
}

// logCall records one round trip: host time, the agent's own time and phases,
// sizes, so a slow step splits into transport, agent work and payload.
func logCall(cmd string, elapsed time.Duration, resp *Response, reqBytes, respBytes int, err error) {
	server := "-"
	phases := ""
	if resp != nil {
		server = fmt.Sprintf("%.0fms", resp.ServerMs)
		var parts []string
		for _, k := range []string{"snapshotMs", "matchMs", "actMs", "quiescenceMs", "quiescenceUnbounded", "settleMs", "framesQuick", "framesFull", "nodes", "reused", "quiescenceStill"} {
			if v, ok := resp.Phases[k]; ok {
				parts = append(parts, fmt.Sprintf("%s=%.0f", k, v))
			}
		}
		// Which app the agent read: a capture of SpringBoard while the app
		// under test is on screen shows here first.
		if resp.Data != nil && resp.Data.BundleID != "" {
			parts = append(parts, "app="+resp.Data.BundleID)
		}
		phases = strings.Join(parts, " ")
	}
	status := "ok"
	if err != nil {
		status = "err=" + err.Error()
	}
	logger.Debug("[devicelab-ios] agent %s %dms server=%s %s req=%dB resp=%dB %s",
		cmd, elapsed.Milliseconds(), server, phases, reqBytes, respBytes, status)
}
