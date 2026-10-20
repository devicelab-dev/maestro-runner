package telemetry

import (
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// warmServer is an HTTPS endpoint that records the events it gets and counts
// the connections opened to it.
type warmServer struct {
	*httptest.Server
	conns atomic.Int32
	mu    sync.Mutex
	got   []string
}

func newWarmServer(t *testing.T) *warmServer {
	t.Helper()
	ws := &warmServer{}
	ws.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ws.mu.Lock()
		ws.got = append(ws.got, string(body))
		ws.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	ws.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			ws.conns.Add(1)
		}
	}
	ws.StartTLS()
	t.Cleanup(ws.Close)

	roots := x509.NewCertPool()
	roots.AddCert(ws.Certificate())
	warmRoots = roots
	t.Cleanup(func() { warmRoots = nil })
	t.Cleanup(resetWarm)
	t.Setenv("MAESTRO_RUNNER_TELEMETRY_URL", ws.URL)
	return ws
}

func (ws *warmServer) events() []string {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return append([]string(nil), ws.got...)
}

func resetWarm() {
	warm.mu.Lock()
	warm.addr, warm.ch = "", nil
	warm.mu.Unlock()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The connection Warm opens at the start is the one the event goes over.
func TestWarmConnectionCarriesTheSend(t *testing.T) {
	clearEnv(t)
	ws := newWarmServer(t)

	Warm()
	waitFor(t, "the warm connection", func() bool { return ws.conns.Load() == 1 })
	if got := ws.events(); len(got) != 0 {
		t.Fatalf("Warm sent something: %v", got)
	}

	Send(Event{RunnerVersion: "1.1.30", Command: "test", RunOutcome: "completed"})
	got := ws.events()
	if len(got) != 1 || !strings.Contains(got[0], `"runner_version":"1.1.30"`) {
		t.Fatalf("server got %v", got)
	}
	if n := ws.conns.Load(); n != 1 {
		t.Errorf("send opened its own connection: %d connections, want 1", n)
	}
}

// A warmed connection the server has closed since is not used: the send
// opens a new one and the event still arrives.
func TestWarmConnectionClosedByServerFallsBack(t *testing.T) {
	clearEnv(t)
	ws := newWarmServer(t)

	Warm()
	waitFor(t, "the warm connection", func() bool { return ws.conns.Load() == 1 })
	ws.CloseClientConnections()
	time.Sleep(50 * time.Millisecond) // let the close reach the client

	Send(Event{RunnerVersion: "1.1.30", Command: "test", RunOutcome: "completed"})
	if got := ws.events(); len(got) != 1 {
		t.Fatalf("server got %v, want the event once", got)
	}
	if n := ws.conns.Load(); n != 2 {
		t.Errorf("%d connections, want the warm one and a new one", n)
	}
}

// Warm opens nothing when telemetry is off or in debug mode.
func TestWarmHonoursOptOuts(t *testing.T) {
	for _, env := range []string{"MAESTRO_RUNNER_NO_TELEMETRY", "DO_NOT_TRACK", "MAESTRO_RUNNER_TELEMETRY_DEBUG"} {
		t.Run(env, func(t *testing.T) {
			clearEnv(t)
			ws := newWarmServer(t)
			t.Setenv(env, "1")
			Warm()
			if warmClient() != nil {
				t.Error("Warm started a dial")
			}
			time.Sleep(50 * time.Millisecond)
			if n := ws.conns.Load(); n != 0 {
				t.Errorf("Warm opened %d connections", n)
			}
		})
	}
}

// Without Warm the send behaves as before.
func TestSendWithoutWarm(t *testing.T) {
	clearEnv(t)
	resetWarm()
	if warmClient() != nil {
		t.Fatal("a warm client without Warm")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Setenv("MAESTRO_RUNNER_TELEMETRY_URL", srv.URL)
	Warm() // plain HTTP: nothing to warm
	if warmClient() != nil {
		t.Error("Warm started a dial for a non-HTTPS endpoint")
	}
}
