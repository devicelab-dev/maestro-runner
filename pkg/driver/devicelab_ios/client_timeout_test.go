package devicelab_ios

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// slowServer answers after delay; if stallBody is set it sends the headers
// immediately and stalls the body instead, so the timeout fires mid-read.
func slowServer(t *testing.T, delay time.Duration, stallBody bool) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if stallBody {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
		}
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, srv.Listener.Addr().(*net.TCPAddr).Port
}

// TestCallTimeoutClassification: the client's own call timeout surfaces as
// ErrCallTimeout and never relaunches; a caller deadline always wins over the
// default bound, and a caller deadline expiring is a plain context error.
func TestCallTimeoutClassification(t *testing.T) {
	tests := []struct {
		name        string
		delay       time.Duration
		stallBody   bool
		callTimeout time.Duration
		ctxTimeout  time.Duration // 0 = no deadline (context.WithoutCancel shape)
		wantErr     error
		wantNot     error
	}{
		{name: "no deadline, runner slow before headers", delay: time.Second, callTimeout: 50 * time.Millisecond, wantErr: ErrCallTimeout},
		{name: "no deadline, runner stalls mid-body", delay: time.Second, stallBody: true, callTimeout: 50 * time.Millisecond, wantErr: ErrCallTimeout},
		{name: "no deadline, runner within bound", delay: 10 * time.Millisecond, callTimeout: time.Second},
		{name: "caller deadline longer than bound wins", delay: 150 * time.Millisecond, callTimeout: 50 * time.Millisecond, ctxTimeout: 2 * time.Second},
		{name: "caller deadline expires", delay: time.Second, callTimeout: 10 * time.Second, ctxTimeout: 50 * time.Millisecond, wantErr: context.DeadlineExceeded, wantNot: ErrCallTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, port := slowServer(t, tt.delay, tt.stallBody)
			c := NewClient("127.0.0.1", port)
			c.SetCallTimeout(tt.callTimeout)
			var revives atomic.Int32
			c.SetReviver(func(_ context.Context, _ int) (int, error) {
				revives.Add(1)
				return port + 1, nil
			})
			ctx := context.WithoutCancel(context.Background())
			if tt.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
				defer cancel()
			}
			_, err := c.Call(ctx, Command{Command: CmdSnapshot})
			assertCallErr(t, err, tt.wantErr, tt.wantNot)
			if revives.Load() != 0 || c.Port() != port {
				t.Errorf("timeout must not relaunch: revives=%d port=%d want %d", revives.Load(), c.Port(), port)
			}
		})
	}
}

func assertCallErr(t *testing.T, err, want, wantNot error) {
	t.Helper()
	if want == nil {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want errors.Is(err, %v)", err, want)
	}
	if wantNot != nil && errors.Is(err, wantNot) {
		t.Errorf("err = %v, must not match %v", err, wantNot)
	}
	if isTransportError(err) {
		t.Errorf("timeout must not be a transport error: %v", err)
	}
}

// TestCallTimeoutIsAlsoDeadlineExceeded: callers that only check the stdlib
// sentinel still see the timeout.
func TestCallTimeoutIsAlsoDeadlineExceeded(t *testing.T) {
	_, port := slowServer(t, time.Second, false)
	c := NewClient("127.0.0.1", port)
	c.SetCallTimeout(20 * time.Millisecond)
	_, err := c.Call(context.Background(), Command{Command: CmdSnapshot})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded too", err)
	}
}

func TestSetCallTimeout(t *testing.T) {
	tests := []struct {
		name string
		set  time.Duration
		want time.Duration
	}{
		{"positive", 5 * time.Second, 5 * time.Second},
		{"zero restores default", 0, DefaultCallTimeout},
		{"negative restores default", -time.Second, DefaultCallTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient("127.0.0.1", 1)
			if got := c.CallTimeout(); got != DefaultCallTimeout {
				t.Fatalf("new client timeout = %v, want %v", got, DefaultCallTimeout)
			}
			c.SetCallTimeout(tt.set)
			if got := c.CallTimeout(); got != tt.want {
				t.Errorf("CallTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestClassifyRequestErrorFallback: a failure with both contexts live is the
// fallback (e.g. a transport error that should revive).
func TestClassifyRequestErrorFallback(t *testing.T) {
	fallback := transportError{errors.New("connection refused")}
	got := classifyRequestError(context.Background(), context.Background(), errors.New("x"), fallback)
	if !isTransportError(got) {
		t.Errorf("got %v, want the transport fallback", got)
	}
}
