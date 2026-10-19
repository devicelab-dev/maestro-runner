package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/urfave/cli/v2"
)

// swallowSignals registers a channel for SIGINT and SIGTERM for the duration
// of the test.
//
// The server command installs its own signal.NotifyContext handler, but it
// unregisters that handler with a deferred stop() when the command returns. If
// a test then signals the process while no handler is registered, the signal
// takes its *default* disposition and kills the whole test binary — turning an
// ordinary test failure into an undiagnosed dead run with no output.
//
// Holding our own registration for the test's lifetime means the default
// disposition is never reached, so a late or duplicate signal is simply
// discarded instead of aborting the run.
func swallowSignals(t *testing.T) {
	t.Helper()
	sig := make(chan os.Signal, 4)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	t.Cleanup(func() { signal.Stop(sig) })
}

// interruptServer sends SIGINT to the test process, but only while the server
// command is still running. If the command has already returned, its signal
// handler is gone and signalling would be pointless at best.
func interruptServer(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case <-done:
		return // already exited; no handler left to notify
	default:
	}
	if proc, err := os.FindProcess(os.Getpid()); err == nil {
		_ = proc.Signal(os.Interrupt)
	}
}

// safeBuffer is a bytes.Buffer that tolerates concurrent writes, which is what
// a server started on a background goroutine produces.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitReady polls addr until it answers HTTP, failing the test if the server
// exits first or never comes up.
func waitReady(t *testing.T, addr string, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case err := <-done:
			t.Fatalf("server exited before becoming ready: %v", err)
		default:
		}
		if resp, err := http.Get(addr); err == nil {
			_ = resp.Body.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not become ready at %s", addr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// freePort reserves an ephemeral port and releases it so the server can bind.
// There is an unavoidable window before the server starts listening, which is
// why the readiness check below polls rather than assuming the port is open.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}
	return port
}

// expectedEndpoints is the banner contract from the ticket: every route the
// server exposes is announced at startup so an operator can see them without
// reading the source.
var expectedEndpoints = []string{
	"POST   /session",
	"POST   /session/{id}/execute",
	"GET    /session/{id}/screenshot",
	"GET    /session/{id}/source",
	"GET    /session/{id}/device-info",
	"DELETE /session/{id}",
	"GET    /status",
}

// startTestServer runs the real server command on args/env, waits for it to
// answer, and registers cleanup that interrupts it so the test never leaves a
// listener behind.
func startTestServer(t *testing.T, port int, args ...string) string {
	t.Helper()
	swallowSignals(t)

	// Capture the banner the command prints at startup.
	out := &safeBuffer{}
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_, _ = io.Copy(out, r)
	}()

	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{serverCommand},
	}

	runArgs := append([]string{"test-app"}, args...)
	done := make(chan error, 1)
	go func() { done <- app.Run(runArgs) }()

	stop := func() {
		// Interrupt, then let the command finish before tearing down the pipe,
		// otherwise its shutdown message can land on the restored stdout.
		interruptServer(t, done)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		_ = w.Close()
		<-drained
		os.Stdout = oldStdout
		_ = r.Close()
	}
	t.Cleanup(stop)

	// Wait for the listener to come up before deciding the command failed.
	waitReady(t, fmt.Sprintf("http://127.0.0.1:%d/status", port), done)

	// The banner is printed before ListenAndServe blocks, but it reaches us
	// through an io.Copy goroutine that has not necessarily drained yet. Wait
	// for the last banner line to appear rather than assuming it has landed.
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "DELETE /session/{id}") {
		if time.Now().After(deadline) {
			t.Fatalf("startup banner incomplete after %s\nbanner so far:\n%s", deadline, out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	return out.String()
}

// TestServerCommand_StartsAndPrintsEndpoints covers the acceptance criterion
// that `maestro-runner server` comes up and announces its endpoint list.
func TestServerCommand_StartsAndPrintsEndpoints(t *testing.T) {
	port := freePort(t)
	banner := startTestServer(t, port, "server", "--port", fmt.Sprint(port))

	for _, endpoint := range expectedEndpoints {
		if !strings.Contains(banner, endpoint) {
			t.Errorf("startup banner does not advertise %q\nbanner:\n%s", endpoint, banner)
		}
	}
}

// TestServerCommand_ServesStatus asserts the server answers the documented
// status contract: HTTP 200 with {"status":"ok"} and no sessions yet.
func TestServerCommand_ServesStatus(t *testing.T) {
	port := freePort(t)
	startTestServer(t, port, "server", "--port", fmt.Sprint(port))

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/status", port))
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body struct {
		Status   string `json:"status"`
		Sessions int    `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
	if body.Sessions != 0 {
		t.Errorf("sessions = %d, want 0", body.Sessions)
	}
}

// TestServerCommand_PortFromEnv covers MAESTRO_SERVER_PORT mapping to the
// default --port, which is how containerised clients pick the port without
// touching the command line.
func TestServerCommand_PortFromEnv(t *testing.T) {
	port := freePort(t)
	t.Setenv("MAESTRO_SERVER_PORT", fmt.Sprint(port))

	startTestServer(t, port, "server")

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/status", port))
	if err != nil {
		t.Fatalf("GET /status on MAESTRO_SERVER_PORT=%d: %v", port, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /status = %d, want 200", resp.StatusCode)
	}
}

// TestServerCommand_ShutdownOnSignal covers the SIGINT/SIGTERM contract from
// the CLI side: interrupting the process tears the listener down and the
// command exits cleanly rather than reporting an error.
func TestServerCommand_ShutdownOnSignal(t *testing.T) {
	port := freePort(t)
	swallowSignals(t)

	app := &cli.App{
		Name:     "test-app",
		Flags:    GlobalFlags,
		Commands: []*cli.Command{serverCommand},
	}

	oldStdout := os.Stdout
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	os.Stdout = devNull
	defer func() { os.Stdout = oldStdout; _ = devNull.Close() }()

	done := make(chan error, 1)
	go func() { done <- app.Run([]string{"test-app", "server", "--port", fmt.Sprint(port)}) }()

	waitReady(t, fmt.Sprintf("http://127.0.0.1:%d/status", port), done)

	// SIGTERM must reach the running command, so signal before it can exit.
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find process: %v", err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("server exited with error after SIGTERM: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not exit after SIGTERM")
	}
}
