package devicelab_ios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the HTTP transport to the on-device runner. agent-device's
// runner accepts JSON-bodied POST requests at any path (the transport
// matches on Content-Length and decodes the body as a Command). There is
// no separate /health endpoint — readiness is probed by sending an
// `uptime` command, which is the runner's built-in lightweight ping.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a Client targeting `host:port`. host is typically
// 127.0.0.1 for simulator and tunneled-device flows.
func NewClient(host string, port int) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        4,
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  true,
			},
		},
	}
}

// Ping probes the runner by sending an `uptime` command. Used by setup
// to wait until the runner is listening and dispatching.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Call(ctx, Command{Command: CmdUptime})
	return err
}

// CallRaw sends an arbitrary JSON-serializable body (instead of a typed
// Command). Used for the one-off case where a handler needs to emit a
// field that the typed Command would `omitempty` away (e.g. eraseText
// needs `"text": ""` to survive the marshal).
func (c *Client) CallRaw(ctx context.Context, body any) (*ResponseData, error) {
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal raw command: %w", err)
	}
	return c.doRequest(ctx, rawBody)
}

// Call sends a command and decodes the response envelope. Errors from the
// runner (`ok: false`) are returned as RunnerError so callers can branch on
// the structured error code.
func (c *Client) Call(ctx context.Context, cmd Command) (*ResponseData, error) {
	body, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal command: %w", err)
	}
	return c.doRequest(ctx, body)
}

func (c *Client) doRequest(ctx context.Context, body []byte) (*ResponseData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/command", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runner request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read runner response: %w", err)
	}

	var envelope Response
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode runner response: %w (body=%q)", err, string(raw))
	}
	if !envelope.Ok {
		code := ""
		msg := "unknown runner error"
		if envelope.Error != nil {
			code = envelope.Error.Code
			msg = envelope.Error.Message
		}
		return envelope.Data, &RunnerError{Code: code, Message: msg}
	}
	return envelope.Data, nil
}

// RunnerError is the typed error returned when the runner responds with
// `ok: false`. Driver code can check Code against the ErrXxx constants to
// branch on specific failure modes (ELEMENT_NOT_FOUND vs APP_NOT_RUNNING).
type RunnerError struct {
	Code    string
	Message string
}

func (e *RunnerError) Error() string {
	return fmt.Sprintf("runner: %s: %s", e.Code, e.Message)
}

// IsRunnerError unwraps an error and reports its code, or "" if not a
// RunnerError. Convenience for branching on the error code.
func IsRunnerError(err error) (*RunnerError, bool) {
	if err == nil {
		return nil, false
	}
	re, ok := err.(*RunnerError)
	return re, ok
}
