package uiautomator2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordingShell fakes the device shell + pull for recording tests.
type recordingShell struct {
	commands   []string
	pgrepOut   []string // successive pgrep responses
	pgrepCalls int
	pulled     map[string]string // remote → local
}

func (s *recordingShell) Shell(cmd string) (string, error) {
	s.commands = append(s.commands, cmd)
	if strings.HasPrefix(cmd, "pgrep") {
		out := ""
		if s.pgrepCalls < len(s.pgrepOut) {
			out = s.pgrepOut[s.pgrepCalls]
		}
		s.pgrepCalls++
		return out, nil
	}
	return "", nil
}

func (s *recordingShell) Pull(remotePath, localPath string) error {
	if s.pulled == nil {
		s.pulled = map[string]string{}
	}
	s.pulled[remotePath] = localPath
	return os.WriteFile(localPath, []byte("mp4"), 0o644)
}

func TestStartScreenRecording_VerifiesProcessStarted(t *testing.T) {
	shell := &recordingShell{pgrepOut: []string{"1234"}}
	d := &Driver{device: shell}

	if err := d.StartScreenRecording(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(shell.commands, "\n")
	if !strings.Contains(joined, "screenrecord "+recordingDevicePath) {
		t.Errorf("screenrecord not started:\n%s", joined)
	}
}

func TestStartScreenRecording_FailsWhenProcessAbsent(t *testing.T) {
	shell := &recordingShell{pgrepOut: []string{""}} // screenrecord died instantly
	d := &Driver{device: shell}

	if err := d.StartScreenRecording(); err == nil {
		t.Fatal("expected an error when screenrecord is not running after start")
	}
}

func TestStopScreenRecording_PullsAndCleansUp(t *testing.T) {
	shell := &recordingShell{pgrepOut: []string{""}} // already exited when polled
	d := &Driver{device: shell}

	host := filepath.Join(t.TempDir(), "recording.mp4")
	if err := d.StopScreenRecording(host); err != nil {
		t.Fatal(err)
	}
	if shell.pulled[recordingDevicePath] != host {
		t.Errorf("pulled %v, want %s → %s", shell.pulled, recordingDevicePath, host)
	}
	last := shell.commands[len(shell.commands)-1]
	if last != "rm -f "+recordingDevicePath {
		t.Errorf("device file not removed, last command: %s", last)
	}
}
