package simulator

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStartRecording_ImmediateExitIsAnError(t *testing.T) {
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}
	t.Cleanup(func() { execCommand = exec.Command })

	if _, err := StartRecording("FAKE-UDID"); err == nil {
		t.Fatal("expected an error when recordVideo exits immediately")
	}
}

func TestRecording_StopMovesFile(t *testing.T) {
	// A recorder that runs until interrupted, like recordVideo does.
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `trap "exit 0" INT TERM; while true; do sleep 0.05; done`)
	}
	t.Cleanup(func() { execCommand = exec.Command })

	rec, err := StartRecording("FAKE-UDID")
	if err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "recording.mp4")
	if err := rec.Stop(dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("recording not moved to %s: %v", dst, err)
	}
	if _, err := os.Stat(rec.path); !os.IsNotExist(err) {
		t.Errorf("temp file %s should be gone after Stop", rec.path)
	}
}

func TestMoveFile_CopiesAcrossFailures(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.mp4")
	if err := os.WriteFile(src, []byte("video-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "dst.mp4")
	if err := moveFile(src, dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "video-bytes" {
		t.Errorf("dst content = %q, %v", data, err)
	}
}
