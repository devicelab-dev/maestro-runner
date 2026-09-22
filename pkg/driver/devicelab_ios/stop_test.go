package devicelab_ios

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// startProc starts a real local shell process (no device involved) and
// returns a handle that owns it, the way startOnce builds one.
func startProc(t *testing.T, script string) *RunnerHandle {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	h := &RunnerHandle{cmd: cmd, waitDone: make(chan struct{})}
	go func() { h.waitErr = cmd.Wait(); close(h.waitDone) }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return h
}

// TestStopProcessRealProcess covers the real signal paths: a process that
// exits on SIGTERM, one that ignores it (force-killed after the grace), and
// one that already exited.
func TestStopProcessRealProcess(t *testing.T) {
	orig := stopGrace
	stopGrace = 100 * time.Millisecond
	defer func() { stopGrace = orig }()

	tests := []struct {
		name   string
		script string
		exited bool
	}{
		{name: "exits on SIGTERM", script: "sleep 30"},
		{name: "ignores SIGTERM, killed", script: `trap "" TERM; while :; do sleep 0.05; done`},
		{name: "already exited", script: "exit 0", exited: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := startProc(t, tt.script)
			if tt.exited {
				<-h.waitDone
			} else {
				time.Sleep(50 * time.Millisecond) // let sh install its trap
			}
			if err := h.stopProcess(); err != nil {
				t.Fatalf("stopProcess: %v", err)
			}
			select {
			case <-h.waitDone:
			default:
				t.Error("process still running after stopProcess")
			}
		})
	}
}

// TestStopProcessOwnsWaitWithoutWatcher: a handle built without startOnce's
// watch goroutine still waits for the exit itself.
func TestStopProcessOwnsWaitWithoutWatcher(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	h := &RunnerHandle{cmd: cmd}
	if err := h.stopProcess(); err != nil {
		t.Fatalf("stopProcess: %v", err)
	}
	if cmd.ProcessState == nil {
		t.Error("process was not reaped")
	}
}

// TestStopProcessKillFailure: a process that cannot be killed is reported,
// not silently assumed dead; a SIGTERM failure goes straight to kill.
func TestStopProcessKillFailure(t *testing.T) {
	tests := []struct {
		name    string
		sigErr  error
		killErr error
		wantErr bool
	}{
		{name: "kill fails", killErr: errors.New("operation not permitted"), wantErr: true},
		{name: "sigterm fails, kill fails", sigErr: errors.New("eperm"), killErr: errors.New("eperm"), wantErr: true},
		{name: "kill reports already done", killErr: os.ErrProcessDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origSig, origKill, origGrace := signalProcess, killProcess, stopGrace
			defer func() { signalProcess, killProcess, stopGrace = origSig, origKill, origGrace }()
			stopGrace = 10 * time.Millisecond
			done := make(chan struct{})
			signalProcess = func(*os.Process, os.Signal) error { return tt.sigErr }
			killProcess = func(*os.Process) error {
				if tt.killErr == os.ErrProcessDone {
					close(done)
				}
				return tt.killErr
			}
			h := &RunnerHandle{cmd: &exec.Cmd{Process: &os.Process{Pid: 4242}}, waitDone: done}
			err := h.stopProcess()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "pid 4242") {
				t.Errorf("error should name the pid: %v", err)
			}
		})
	}
}

// TestReviveLogsStopFailureAndRelaunches: a dead runner that cannot be
// killed is logged, and the relaunch still happens.
func TestReviveLogsStopFailureAndRelaunches(t *testing.T) {
	origSig, origKill, origGrace := signalProcess, killProcess, stopGrace
	defer func() { signalProcess, killProcess, stopGrace = origSig, origKill, origGrace }()
	stopGrace = 10 * time.Millisecond
	signalProcess = func(*os.Process, os.Signal) error { return nil }
	killProcess = func(*os.Process) error { return errors.New("operation not permitted") }

	var warn bytes.Buffer
	s := testSupervisor(SetupOptions{}, 1000, startOn(2000))
	s.warn = &warn
	s.handle.Store(&RunnerHandle{port: 1000, cmd: &exec.Cmd{Process: &os.Process{Pid: 4242}}, waitDone: make(chan struct{})})

	port, err := s.revive(context.Background(), 1000)
	if err != nil || port != 2000 {
		t.Fatalf("revive = (%d, %v), want (2000, nil)", port, err)
	}
	if !strings.Contains(warn.String(), "could not stop the dead devicelab runner") {
		t.Errorf("stop failure not logged: %q", warn.String())
	}
}

// TestSupervisorStopSurfacesKillFailure: Stop via the supervisor returns the
// kill failure to the caller.
func TestSupervisorStopSurfacesKillFailure(t *testing.T) {
	origSig, origKill, origGrace := signalProcess, killProcess, stopGrace
	defer func() { signalProcess, killProcess, stopGrace = origSig, origKill, origGrace }()
	stopGrace = 10 * time.Millisecond
	signalProcess = func(*os.Process, os.Signal) error { return nil }
	killProcess = func(*os.Process) error { return errors.New("operation not permitted") }

	h := &RunnerHandle{cmd: &exec.Cmd{Process: &os.Process{Pid: 4242}}, waitDone: make(chan struct{})}
	s := &Supervisor{}
	s.handle.Store(h)
	h.sup = s
	if err := h.Stop(); err == nil {
		t.Error("Stop should surface the kill failure")
	}
}
