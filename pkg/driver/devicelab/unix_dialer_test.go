package devicelab

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// A peer that accepts the connection and never answers must not hold a read
// past the dial context's deadline (the WebSocket handshake hung 11 minutes);
// once cleared, the connection has no deadline.
func TestUnixDialerDeadline(t *testing.T) {
	dir, err := os.MkdirTemp("", "ud")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			defer c.Close()
			time.Sleep(3 * time.Second) // accept, never answer
		}
	}()

	d := &unixDialer{socketPath: sock}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	conn, err := d.DialContext(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	start := time.Now()
	_, err = conn.Read(make([]byte, 1))
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("read returned %v after %v; want a deadline error within ~200ms", err, time.Since(start))
	}

	d.clearDeadline()
}

// silentForwarder "forwards" to a local socket the test serves itself.
type silentForwarder struct{ path string }

func (f silentForwarder) ForwardToAbstractSocket(string, string) error { return nil }
func (f silentForwarder) ForwardTCPToAbstractSocket(int, string) error { return nil }
func (f silentForwarder) RemoveSocketForward(string) error             { return nil }
func (f silentForwarder) RemoveTCPForward(int) error                   { return nil }
func (f silentForwarder) CDPSocketPath() string                        { return f.path }

// A DevTools end that completes the WebSocket handshake and then never
// answers must fail the WebView connect within the setup deadline: it held
// browser.Connect() for 11 minutes in React Navigation's debug build.
func TestWebViewConnectGivesUpOnASilentDevTools(t *testing.T) {
	old := cdpSetupTimeout
	cdpSetupTimeout = 300 * time.Millisecond
	defer func() { cdpSetupTimeout = old }()

	dir, err := os.MkdirTemp("", "cdp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		// Answer the WebSocket upgrade, then read whatever comes and never reply.
		r := bufio.NewReader(c)
		key := ""
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(strings.ToLower(line), "sec-websocket-key:") {
				key = strings.TrimSpace(line[len("sec-websocket-key:"):])
			}
			if line == "\r\n" {
				break
			}
		}
		sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		fmt.Fprintf(c, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
			base64.StdEncoding.EncodeToString(sum[:]))
		_, _ = io.Copy(io.Discard, r)
	}()

	m := newWebViewManager(silentForwarder{path: sock})
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- m.connectViaUnixSocket(&core.CDPInfo{Socket: "webview_devtools_remote_1"}, "webview") }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("connect succeeded against a DevTools end that never answers")
		}
		if took := time.Since(start); took > 3*time.Second {
			t.Errorf("connect gave up after %v, want about the setup deadline (300ms)", took)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("connect still waiting after 10s: the setup is not bounded")
	}
}
