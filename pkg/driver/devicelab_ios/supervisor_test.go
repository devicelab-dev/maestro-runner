package devicelab_ios

import (
	"context"
	"errors"
	"testing"
	"time"
)

// startFunc is the Supervisor.start signature.
type startFunc = func(ctx context.Context, opts SetupOptions, xctestrun, logPath string) (*Client, *RunnerHandle, error)

// testSupervisor builds a supervisor whose live runner is a process-less
// handle on port, launching replacements with start.
func testSupervisor(opts SetupOptions, port int, start startFunc) *Supervisor {
	s := &Supervisor{opts: opts, start: start}
	s.handle.Store(&RunnerHandle{port: port, host: "127.0.0.1"})
	return s
}

// startOn returns a start func that launches a process-less runner on port.
func startOn(port int) startFunc {
	return func(context.Context, SetupOptions, string, string) (*Client, *RunnerHandle, error) {
		return nil, &RunnerHandle{port: port, host: "127.0.0.1"}, nil
	}
}

// hangingStart blocks until its context ends — a wedged xcodebuild.
func hangingStart(ctx context.Context, _ SetupOptions, _, _ string) (*Client, *RunnerHandle, error) {
	<-ctx.Done()
	return nil, nil, ctx.Err()
}

// TestReviveRelaunchIsBounded: a relaunch under a deadline-less context
// still ends within RelaunchTimeout instead of hanging for ReadyTimeout.
func TestReviveRelaunchIsBounded(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		start    startFunc
		wantPort int
		wantErr  error
	}{
		{name: "wedged start times out", timeout: 50 * time.Millisecond, start: hangingStart, wantErr: context.DeadlineExceeded},
		{name: "healthy start returns new port", timeout: time.Second, start: startOn(2000), wantPort: 2000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSupervisor(SetupOptions{RelaunchTimeout: tt.timeout}, 1000, tt.start)
			began := time.Now()
			port, err := s.revive(context.WithoutCancel(context.Background()), 1000)
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
			if elapsed := time.Since(began); elapsed > tt.timeout+time.Second {
				t.Errorf("revive took %v, want bounded by %v", elapsed, tt.timeout)
			}
		})
	}
}

func TestRelaunchTimeoutDefault(t *testing.T) {
	tests := []struct {
		name string
		set  time.Duration
		want time.Duration
	}{
		{"unset uses default", 0, DefaultRelaunchTimeout},
		{"explicit value kept", 5 * time.Second, 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Supervisor{opts: SetupOptions{RelaunchTimeout: tt.set}}
			if got := s.relaunchTimeout(); got != tt.want {
				t.Errorf("relaunchTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGracefulShutdownDoesNotRelaunchDeadRunner: shutting down a runner that
// already died must not relaunch it just to send `shutdown`.
func TestGracefulShutdownDoesNotRelaunchDeadRunner(t *testing.T) {
	dead, deadPort := okServer(t, nil)
	dead.Close()

	var starts int
	s := testSupervisor(SetupOptions{}, deadPort, func(context.Context, SetupOptions, string, string) (*Client, *RunnerHandle, error) {
		starts++
		return nil, &RunnerHandle{port: deadPort + 1}, nil
	})
	h := s.handle.Load()
	h.sup = s
	c := NewClient("127.0.0.1", deadPort)
	c.SetReviver(s.revive)

	if err := GracefulShutdown(context.Background(), c, h); err != nil {
		t.Fatalf("GracefulShutdown: %v", err)
	}
	if starts != 0 {
		t.Errorf("dead runner was relaunched %d times during shutdown, want 0", starts)
	}
	if !s.stopping.Load() {
		t.Error("supervisor should be marked stopping")
	}
}

// TestGracefulShutdownNilArgs: no client, no supervisor, or no handle at all
// are all tolerated.
func TestGracefulShutdownNilArgs(t *testing.T) {
	tests := []struct {
		name string
		h    *RunnerHandle
	}{
		{"nil handle", nil},
		{"handle without supervisor", &RunnerHandle{port: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := GracefulShutdown(context.Background(), nil, tt.h); err != nil {
				t.Errorf("GracefulShutdown: %v", err)
			}
		})
	}
}
