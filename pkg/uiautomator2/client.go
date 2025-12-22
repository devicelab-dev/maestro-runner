package uiautomator2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Client communicates with UIAutomator2 server.
type Client struct {
	http       *http.Client
	baseURL    string
	sessionID  string
	socketPath string
}

// NewClient creates a client using Unix socket (Linux/Mac).
func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}

	return &Client{
		http: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		baseURL:    "http://localhost",
		socketPath: socketPath,
	}
}

// NewClientTCP creates a client using TCP port (Windows).
func NewClientTCP(port int) *Client {
	return &Client{
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
	}
}

// SessionID returns the current session ID.
func (c *Client) SessionID() string {
	return c.sessionID
}

// HasSession returns true if a session is active.
func (c *Client) HasSession() bool {
	return c.sessionID != ""
}

// request makes an HTTP request to UIAutomator2.
func (c *Client) request(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp Response
		if json.Unmarshal(respBody, &errResp) == nil {
			if errVal, ok := errResp.Value.(map[string]interface{}); ok {
				errMsg, _ := errVal["message"].(string)
				errType, _ := errVal["error"].(string)
				return nil, fmt.Errorf("%s: %s", errType, errMsg)
			}
		}
		return nil, fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// sessionPath returns path with session ID prefix.
func (c *Client) sessionPath(path string) string {
	return fmt.Sprintf("/session/%s%s", c.sessionID, path)
}

// Status checks if the server is ready.
func (c *Client) Status() (bool, error) {
	data, err := c.request("GET", "/status", nil)
	if err != nil {
		return false, err
	}

	var resp struct {
		Value struct {
			Ready   bool   `json:"ready"`
			Message string `json:"message"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return false, err
	}

	return resp.Value.Ready, nil
}

// CreateSession starts a new automation session.
func (c *Client) CreateSession(caps Capabilities) error {
	req := SessionRequest{Capabilities: caps}
	data, err := c.request("POST", "/session", req)
	if err != nil {
		return err
	}

	var resp struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parse session response: %w", err)
	}

	if resp.SessionID == "" {
		// Try alternate response format
		var altResp struct {
			Value struct {
				SessionID string `json:"sessionId"`
			} `json:"value"`
		}
		if json.Unmarshal(data, &altResp) == nil && altResp.Value.SessionID != "" {
			resp.SessionID = altResp.Value.SessionID
		}
	}

	if resp.SessionID == "" {
		return fmt.Errorf("no session ID in response")
	}

	c.sessionID = resp.SessionID
	return nil
}

// GetSession returns the current session info.
func (c *Client) GetSession() (map[string]interface{}, error) {
	if c.sessionID == "" {
		return nil, fmt.Errorf("no active session")
	}

	data, err := c.request("GET", c.sessionPath(""), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Value map[string]interface{} `json:"value"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return resp.Value, nil
}

// DeleteSession ends the current session.
func (c *Client) DeleteSession() error {
	if c.sessionID == "" {
		return nil
	}

	_, err := c.request("DELETE", c.sessionPath(""), nil)
	c.sessionID = ""
	return err
}

// Close ends the session and cleans up.
func (c *Client) Close() error {
	return c.DeleteSession()
}
