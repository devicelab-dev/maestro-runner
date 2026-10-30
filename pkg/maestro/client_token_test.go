package maestro

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

// Connect sends the agent's token in the upgrade request once SetToken has
// set it, and none before, which an agent started without one accepts.
func TestConnectSendsToken(t *testing.T) {
	var got []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get(TokenHeader))
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		conn.CloseNow()
	}))
	defer server.Close()
	_, portStr, _ := strings.Cut(strings.TrimPrefix(server.URL, "http://"), ":")
	port, _ := strconv.Atoi(portStr)

	c := NewClientTCP(port)
	if err := c.Connect(); err != nil {
		t.Fatalf("connect without token: %v", err)
	}
	c.Close()

	c = NewClientTCP(port)
	c.SetToken("abc123")
	if err := c.Connect(); err != nil {
		t.Fatalf("connect with token: %v", err)
	}
	c.Close()

	if len(got) != 2 || got[0] != "" || got[1] != "abc123" {
		t.Errorf("token headers = %q, want [\"\" \"abc123\"]", got)
	}
}
