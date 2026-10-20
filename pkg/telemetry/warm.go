package telemetry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Opening the connection to the endpoint (DNS, TCP, TLS) was most of the
// send's cost, about 0.45 s at the end of every run: a harness running one
// process per flow paid it per flow. Warm opens it in the background when the
// run starts and the send at the end takes it over. Nothing is sent on it
// until then.

// warmMaxAge bounds how long a warmed connection is trusted. Older, the send
// opens a new one, as it did before.
const warmMaxAge = 10 * time.Minute

// warmDialTimeout bounds the background dial.
const warmDialTimeout = 5 * time.Second

// warmRoots verifies the endpoint's certificate: nil for the system's roots.
// Tests set their own.
var warmRoots *x509.CertPool

type warmResult struct {
	conn net.Conn // nil when the dial failed
	at   time.Time
}

// warm is the pending or finished dial Warm started, until a send takes it.
var warm struct {
	mu   sync.Mutex
	addr string
	ch   chan warmResult
}

// Warm starts opening the connection to the endpoint, without waiting. It
// does nothing when telemetry is off, in debug mode, for a non-HTTPS
// endpoint, or when a proxy is configured for it: the send then opens its
// connection as before.
func Warm() {
	if !Enabled() || envSet("MAESTRO_RUNNER_TELEMETRY_DEBUG") {
		return
	}
	u, err := url.Parse(endpointURL())
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return
	}
	if p, err := http.ProxyFromEnvironment(&http.Request{URL: u}); err != nil || p != nil {
		return
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	addr := net.JoinHostPort(u.Hostname(), port)
	ch := make(chan warmResult, 1)
	warm.mu.Lock()
	warm.addr, warm.ch = addr, ch
	warm.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), warmDialTimeout)
		defer cancel()
		conn, err := tlsDialer().DialContext(ctx, "tcp", addr)
		if err != nil {
			ch <- warmResult{}
			return
		}
		ch <- warmResult{conn: conn, at: time.Now()}
	}()
}

// warmClient returns a client that sends over the warmed connection, or nil
// when Warm started none (or a send already took it).
func warmClient() *http.Client {
	warm.mu.Lock()
	pending := warm.ch != nil
	warm.mu.Unlock()
	if !pending {
		return nil
	}
	return &http.Client{Transport: &http.Transport{
		// Warm checked there is no proxy, so the transport dials the
		// endpoint itself.
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if conn := takeWarm(ctx, addr); conn != nil {
				return conn, nil
			}
			return tlsDialer().DialContext(ctx, network, addr)
		},
		DisableKeepAlives: true,
	}}
}

// takeWarm returns the warmed connection to addr, waiting for its dial if it
// is still going, or nil when there is none fit to use.
func takeWarm(ctx context.Context, addr string) net.Conn {
	warm.mu.Lock()
	ch := warm.ch
	if warm.addr != addr {
		ch = nil
	}
	warm.ch = nil
	warm.mu.Unlock()
	if ch == nil {
		return nil
	}
	select {
	case r := <-ch:
		if r.conn == nil {
			return nil
		}
		if time.Since(r.at) > warmMaxAge || !alive(r.conn) {
			_ = r.conn.Close()
			return nil
		}
		return r.conn
	case <-ctx.Done():
		return nil
	}
}

// alive reports whether a connection held since the run started can still
// carry the request: a read finds nothing within a millisecond, where a
// connection the server closed reads EOF.
func alive(conn net.Conn) bool {
	if conn.SetReadDeadline(time.Now().Add(time.Millisecond)) != nil {
		return false
	}
	var b [1]byte
	n, err := conn.Read(b[:])
	_ = conn.SetReadDeadline(time.Time{})
	var ne net.Error
	return n == 0 && errors.As(err, &ne) && ne.Timeout()
}

// tlsDialer opens HTTP/1.1 TLS connections: one request goes over each, so
// HTTP/2 would buy nothing.
func tlsDialer() *tls.Dialer {
	return &tls.Dialer{Config: &tls.Config{
		RootCAs:    warmRoots,
		NextProtos: []string{"http/1.1"},
		MinVersion: tls.VersionTLS12,
	}}
}
