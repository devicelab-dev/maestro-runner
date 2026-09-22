package devicelab_ios

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// okServer starts an httptest server on 127.0.0.1 that answers every command
// with a success envelope, and returns it plus its port.
func okServer(t *testing.T, hits *atomic.Int32) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	return srv, port
}

// TestReviveRetriesReadOnlyCommand: a read-only command against a dead runner
// relaunches (via the reviver) and re-sends on the new port, succeeding.
func TestReviveRetriesReadOnlyCommand(t *testing.T) {
	var deadHits, liveHits atomic.Int32
	dead, deadPort := okServer(t, &deadHits)
	dead.Close() // runner is gone before the call — connection refused

	live, livePort := okServer(t, &liveHits)
	defer live.Close()

	c := NewClient("127.0.0.1", deadPort)
	var revives atomic.Int32
	c.SetReviver(func(_ context.Context, failedPort int) (int, error) {
		revives.Add(1)
		if failedPort != deadPort {
			t.Errorf("reviver got failedPort %d, want dead port %d", failedPort, deadPort)
		}
		return livePort, nil
	})

	data, err := c.Call(context.Background(), Command{Command: CmdSnapshot})
	if err != nil {
		t.Fatalf("read-only Call after revive should succeed, got %v", err)
	}
	if data == nil {
		// success envelope has no data; that's fine, but Call must not error
		_ = data
	}
	if revives.Load() != 1 {
		t.Errorf("expected exactly 1 revive, got %d", revives.Load())
	}
	if liveHits.Load() != 1 {
		t.Errorf("expected the retry to hit the relaunched runner once, got %d", liveHits.Load())
	}
	if c.Port() != livePort {
		t.Errorf("client should now target the relaunched port %d, got %d", livePort, c.Port())
	}
}

// TestReviveDoesNotReplayAction: an action against a dead runner relaunches
// (so the next command works) but is NOT re-sent, and the original transport
// error surfaces for this step.
func TestReviveDoesNotReplayAction(t *testing.T) {
	var liveHits atomic.Int32
	dead, deadPort := okServer(t, nil)
	dead.Close()

	live, livePort := okServer(t, &liveHits)
	defer live.Close()

	c := NewClient("127.0.0.1", deadPort)
	var revives atomic.Int32
	c.SetReviver(func(_ context.Context, _ int) (int, error) {
		revives.Add(1)
		return livePort, nil
	})

	_, err := c.Call(context.Background(), Command{Command: CmdTap})
	if err == nil {
		t.Fatal("an action against a dead runner must not be replayed; expected an error")
	}
	if revives.Load() != 1 {
		t.Errorf("expected the runner to be relaunched once even for an action, got %d revives", revives.Load())
	}
	if liveHits.Load() != 0 {
		t.Errorf("action must NOT be re-sent to the relaunched runner, but it got %d hits", liveHits.Load())
	}
	if c.Port() != livePort {
		t.Errorf("client should be re-pointed at %d for the next command, got %d", livePort, c.Port())
	}
}

// TestNoReviverReturnsTransportError: without a reviver, a dead runner is just
// an error — the pre-restart behaviour is unchanged.
func TestNoReviverReturnsTransportError(t *testing.T) {
	dead, deadPort := okServer(t, nil)
	dead.Close()
	c := NewClient("127.0.0.1", deadPort)
	if _, err := c.Call(context.Background(), Command{Command: CmdSnapshot}); err == nil {
		t.Fatal("expected a transport error with no reviver installed")
	}
}

// TestReviveFailureSurfacesOriginalError: when the relaunch itself fails, the
// caller gets an error rather than a retry.
func TestReviveFailureSurfacesOriginalError(t *testing.T) {
	dead, deadPort := okServer(t, nil)
	dead.Close()
	c := NewClient("127.0.0.1", deadPort)
	c.SetReviver(func(_ context.Context, _ int) (int, error) {
		return 0, context.DeadlineExceeded
	})
	if _, err := c.Call(context.Background(), Command{Command: CmdSnapshot}); err == nil {
		t.Fatal("expected an error when relaunch fails")
	}
	if c.Port() != deadPort {
		t.Errorf("port should be unchanged when relaunch fails, got %d", c.Port())
	}
}

// hangingServer holds every request until release is closed (or the client
// goes away), counting arrivals, so a test can cancel a request mid-flight.
func hangingServer(t *testing.T, arrived chan<- struct{}, release <-chan struct{}) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	return srv, srv.Listener.Addr().(*net.TCPAddr).Port
}

// resettingServer drops every connection without a response once release is
// closed — the runner process dying mid-request (EOF / connection reset).
func resettingServer(t *testing.T, arrived chan<- struct{}, release <-chan struct{}) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if arrived != nil {
			arrived <- struct{}{}
		}
		<-release
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = conn.Close()
	}))
	return srv, srv.Listener.Addr().(*net.TCPAddr).Port
}

// TestCallerContextEndDoesNotRevive: a request aborted by its own context —
// cancelled mid-flight, past its deadline, or cancelled before it was sent —
// must surface the context error and never relaunch the (healthy) runner.
func TestCallerContextEndDoesNotRevive(t *testing.T) {
	tests := []struct {
		name string
		want error
		ctx  func() (context.Context, context.CancelFunc, func())
	}{
		{
			name: "cancelled mid-flight",
			want: context.Canceled,
			ctx: func() (context.Context, context.CancelFunc, func()) {
				ctx, cancel := context.WithCancel(context.Background())
				return ctx, cancel, cancel // cancel once the server holds the request
			},
		},
		{
			name: "deadline exceeded mid-flight",
			want: context.DeadlineExceeded,
			ctx: func() (context.Context, context.CancelFunc, func()) {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				return ctx, cancel, func() {}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arrived := make(chan struct{}, 1)
			release := make(chan struct{})
			srv, port := hangingServer(t, arrived, release)
			defer srv.Close()
			defer close(release)

			c := NewClient("127.0.0.1", port)
			var revives atomic.Int32
			c.SetReviver(func(_ context.Context, _ int) (int, error) {
				revives.Add(1)
				return port + 1, nil
			})

			ctx, cancel, midFlight := tt.ctx()
			defer cancel()
			go func() {
				<-arrived
				midFlight()
			}()
			_, err := c.Call(ctx, Command{Command: CmdSnapshot})
			assertAbortedNoRevive(t, c, err, tt.want, &revives, port)
		})
	}
}

// TestPreCancelledContextAgainstDeadRunnerDoesNotRevive: even when the runner
// is also unreachable, a caller that already gave up is not evidence enough
// to relaunch — the next live call will detect the death and revive.
func TestPreCancelledContextAgainstDeadRunnerDoesNotRevive(t *testing.T) {
	dead, deadPort := okServer(t, nil)
	dead.Close()
	c := NewClient("127.0.0.1", deadPort)
	var revives atomic.Int32
	c.SetReviver(func(_ context.Context, _ int) (int, error) {
		revives.Add(1)
		return deadPort + 1, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.CallRaw(ctx, map[string]any{"command": "tap"})
	assertAbortedNoRevive(t, c, err, context.Canceled, &revives, deadPort)
}

func assertAbortedNoRevive(t *testing.T, c *Client, err, want error, revives *atomic.Int32, port int) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want errors.Is(err, %v)", err, want)
	}
	if isTransportError(err) {
		t.Errorf("a context abort must not be classified as a transport error: %v", err)
	}
	if n := revives.Load(); n != 0 {
		t.Errorf("caller cancellation must not relaunch the runner, got %d revives", n)
	}
	if c.Port() != port {
		t.Errorf("port changed to %d without a relaunch, want %d", c.Port(), port)
	}
}

// TestConnectionResetWithLiveContextRevives: the runner dropping the
// connection mid-request while the caller is still waiting is a real death
// and still relaunches + retries a read-only command, as before.
func TestConnectionResetWithLiveContextRevives(t *testing.T) {
	release := make(chan struct{})
	close(release)
	dying, dyingPort := resettingServer(t, nil, release)
	defer dying.Close()
	var liveHits atomic.Int32
	live, livePort := okServer(t, &liveHits)
	defer live.Close()

	c := NewClient("127.0.0.1", dyingPort)
	var revives atomic.Int32
	c.SetReviver(func(_ context.Context, failedPort int) (int, error) {
		revives.Add(1)
		if failedPort != dyingPort {
			t.Errorf("failedPort = %d, want %d", failedPort, dyingPort)
		}
		return livePort, nil
	})
	if _, err := c.Call(context.Background(), Command{Command: CmdSnapshot}); err != nil {
		t.Fatalf("read-only call after reset should be retried on the relaunch, got %v", err)
	}
	if revives.Load() != 1 || liveHits.Load() != 1 {
		t.Errorf("revives=%d liveHits=%d, want 1 and 1", revives.Load(), liveHits.Load())
	}
}

// TestConcurrentFailuresRelaunchOnce: two in-flight calls die with the same
// runner. The first relaunches; the second must report the port it actually
// used (the dead one) so the supervisor re-points instead of killing the
// freshly relaunched runner.
func TestConcurrentFailuresRelaunchOnce(t *testing.T) {
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	dying, dyingPort := resettingServer(t, arrived, release)
	defer dying.Close()
	live, livePort := okServer(t, nil)
	defer live.Close()

	c := NewClient("127.0.0.1", dyingPort)
	var mu sync.Mutex
	cur, relaunches := dyingPort, 0
	c.SetReviver(func(_ context.Context, failedPort int) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		if failedPort != cur { // same rule as Supervisor.revive
			return cur, nil
		}
		relaunches++
		cur = livePort
		return cur, nil
	})

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Call(context.Background(), Command{Command: CmdSnapshot})
		}()
	}
	<-arrived
	<-arrived
	close(release)
	wg.Wait()

	if relaunches != 1 {
		t.Errorf("two failures against one dead runner must relaunch once, got %d", relaunches)
	}
	if c.Port() != livePort {
		t.Errorf("client port = %d, want %d", c.Port(), livePort)
	}
}
