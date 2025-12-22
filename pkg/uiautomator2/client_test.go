package uiautomator2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(handler http.HandlerFunc) (*Client, *httptest.Server) {
	server := httptest.NewServer(handler)
	client := &Client{
		http:    server.Client(),
		baseURL: server.URL,
	}
	return client, server
}

func TestStatus(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("expected /status, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"ready":   true,
				"message": "ready",
			},
		})
	})
	defer server.Close()

	ready, err := client.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Error("expected ready to be true")
	}
}

func TestStatusNotReady(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"ready":   false,
				"message": "not ready",
			},
		})
	})
	defer server.Close()

	ready, err := client.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Error("expected ready to be false")
	}
}

func TestCreateSession(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session" {
			t.Errorf("expected /session, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req SessionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Capabilities.PlatformName != "Android" {
			t.Errorf("expected Android, got %s", req.Capabilities.PlatformName)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"sessionId": "test-session-123",
		})
	})
	defer server.Close()

	err := client.CreateSession(Capabilities{PlatformName: "Android"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.sessionID != "test-session-123" {
		t.Errorf("expected test-session-123, got %s", client.sessionID)
	}
}

func TestCreateSessionAlternateFormat(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"sessionId": "alt-session-456",
			},
		})
	})
	defer server.Close()

	err := client.CreateSession(Capabilities{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.sessionID != "alt-session-456" {
		t.Errorf("expected alt-session-456, got %s", client.sessionID)
	}
}

func TestCreateSessionNoSessionID(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	err := client.CreateSession(Capabilities{})
	if err == nil {
		t.Error("expected error for missing session ID")
	}
}

func TestGetSession(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/test-session" {
			t.Errorf("expected /session/test-session, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"platformName": "Android",
				"deviceName":   "emulator",
			},
		})
	})
	defer server.Close()

	client.sessionID = "test-session"
	info, err := client.GetSession()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info["platformName"] != "Android" {
		t.Errorf("expected Android, got %v", info["platformName"])
	}
}

func TestGetSessionNoSession(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {})
	defer server.Close()

	_, err := client.GetSession()
	if err == nil {
		t.Error("expected error for no active session")
	}
}

func TestDeleteSession(t *testing.T) {
	called := false
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/session/test-session" {
			t.Errorf("expected /session/test-session, got %s", r.URL.Path)
		}
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	client.sessionID = "test-session"
	err := client.DeleteSession()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected DELETE to be called")
	}
	if client.sessionID != "" {
		t.Error("expected session ID to be cleared")
	}
}

func TestDeleteSessionNoSession(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not be called when no session")
	})
	defer server.Close()

	err := client.DeleteSession()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSessionID(t *testing.T) {
	client := &Client{sessionID: "my-session"}
	if client.SessionID() != "my-session" {
		t.Errorf("expected my-session, got %s", client.SessionID())
	}
}

func TestHasSession(t *testing.T) {
	client := &Client{}
	if client.HasSession() {
		t.Error("expected no session")
	}
	client.sessionID = "test"
	if !client.HasSession() {
		t.Error("expected session")
	}
}

func TestClose(t *testing.T) {
	called := false
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		called = true
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	client.sessionID = "test-session"
	err := client.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected delete to be called")
	}
}

func TestRequestError(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"error":   "unknown error",
				"message": "something went wrong",
			},
		})
	})
	defer server.Close()

	_, err := client.Status()
	if err == nil {
		t.Error("expected error")
	}
}

func TestRequestErrorNonJSON(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	})
	defer server.Close()

	_, err := client.Status()
	if err == nil {
		t.Error("expected error")
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("/tmp/test.sock")
	if client.baseURL != "http://localhost" {
		t.Errorf("expected http://localhost, got %s", client.baseURL)
	}
	if client.socketPath != "/tmp/test.sock" {
		t.Errorf("expected /tmp/test.sock, got %s", client.socketPath)
	}
	if client.http == nil {
		t.Error("expected http client to be set")
	}
}

func TestNewClientTCP(t *testing.T) {
	client := NewClientTCP(6790)
	if client.baseURL != "http://127.0.0.1:6790" {
		t.Errorf("expected http://127.0.0.1:6790, got %s", client.baseURL)
	}
	if client.http == nil {
		t.Error("expected http client to be set")
	}
}

func TestStatusUnmarshalError(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid json"))
	})
	defer server.Close()

	_, err := client.Status()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestCreateSessionUnmarshalError(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid json"))
	})
	defer server.Close()

	err := client.CreateSession(Capabilities{})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetSessionUnmarshalError(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid json"))
	})
	defer server.Close()

	client.sessionID = "test"
	_, err := client.GetSession()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRequestConnectionError(t *testing.T) {
	client := &Client{
		http:    &http.Client{},
		baseURL: "http://localhost:99999", // Invalid port
	}

	_, err := client.Status()
	if err == nil {
		t.Error("expected connection error")
	}
}

// unmarshalableType cannot be marshaled to JSON
type unmarshalableType struct {
	Ch chan int
}

func TestRequestMarshalError(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	defer server.Close()

	client.sessionID = "test"
	// Try to send unmarshalable type
	_, err := client.request("POST", "/test", unmarshalableType{Ch: make(chan int)})
	if err == nil {
		t.Error("expected marshal error")
	}
}

func TestDialContextInNewClient(t *testing.T) {
	// Create client with Unix socket
	client := NewClient("/tmp/nonexistent-test.sock")

	// Try to make a request - will fail on dial
	_, err := client.Status()
	if err == nil {
		t.Error("expected dial error for nonexistent socket")
	}
}

func TestCreateSessionRequestError(t *testing.T) {
	client := &Client{
		http:    &http.Client{},
		baseURL: "http://localhost:99999",
	}
	err := client.CreateSession(Capabilities{})
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetSessionRequestError(t *testing.T) {
	client := &Client{
		http:      &http.Client{},
		baseURL:   "http://localhost:99999",
		sessionID: "test",
	}
	_, err := client.GetSession()
	if err == nil {
		t.Error("expected error")
	}
}

func TestCreateSessionAltFormatEmptySessionID(t *testing.T) {
	client, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		// Return alternate format with empty sessionId
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]interface{}{
				"sessionId": "",
			},
		})
	})
	defer server.Close()

	err := client.CreateSession(Capabilities{})
	if err == nil {
		t.Error("expected error for empty session ID")
	}
}

func TestRequestInvalidURL(t *testing.T) {
	client := &Client{
		http:    &http.Client{},
		baseURL: "://invalid-url", // Invalid URL scheme
	}
	_, err := client.request("GET", "/test", nil)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}
