package simulator

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// Recording is an in-flight `simctl io recordVideo` capture of one simulator.
//
// recordVideo finalizes the MP4 only when it exits cleanly, so Stop signals
// SIGINT and waits for the process rather than killing it — a killed recorder
// leaves an unplayable file.
type Recording struct {
	cmd  *exec.Cmd
	path string
	done chan error
}

// StartRecording begins recording the simulator's screen to a temporary file;
// Stop moves it to its final location. An immediately-exiting recorder (device
// not booted, codec unavailable) is reported here rather than at Stop time.
func StartRecording(udid string) (*Recording, error) {
	f, err := os.CreateTemp("", "maestro-runner-recording-*.mp4")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	_ = f.Close()

	cmd := execCommand("xcrun", "simctl", "io", udid, "recordVideo", "--codec", "h264", "--force", path)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("start recordVideo: %w", err)
	}

	rec := &Recording{cmd: cmd, path: path, done: make(chan error, 1)}
	go func() { rec.done <- cmd.Wait() }()

	select {
	case waitErr := <-rec.done:
		_ = os.Remove(path)
		return nil, fmt.Errorf("recordVideo exited immediately: %v", waitErr)
	case <-time.After(500 * time.Millisecond):
	}
	return rec, nil
}

// Stop ends the recording and moves the video to hostPath.
func (r *Recording) Stop(hostPath string) error {
	if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("stop recordVideo: %w", err)
	}
	select {
	case <-r.done:
	case <-time.After(15 * time.Second):
		_ = r.cmd.Process.Kill()
		_ = os.Remove(r.path)
		return fmt.Errorf("recordVideo did not finish writing within 15s")
	}
	return moveFile(r.path, hostPath)
}

// moveFile renames src to dst, copying when they sit on different volumes
// (os.CreateTemp's volume need not be the report's).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
