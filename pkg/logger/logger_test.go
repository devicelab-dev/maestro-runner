package logger

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Close()

	// File should exist now.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("log file not created: %v", err)
	}

	// GetWriter should return the file, not io.Discard.
	w := GetWriter()
	if w == io.Discard {
		t.Error("GetWriter returned io.Discard after Init")
	}

	// Subsequent Init replaces the log file. The previous file handle is
	// closed but the file on disk should still exist.
	path2 := filepath.Join(dir, "test2.log")
	if err := Init(path2); err != nil {
		t.Fatalf("re-Init failed: %v", err)
	}
	if _, err := os.Stat(path2); err != nil {
		t.Errorf("re-init log file not created: %v", err)
	}
}

func TestInit_InvalidPath(t *testing.T) {
	if err := Init("/nonexistent/dir/that/cannot/exist/test.log"); err == nil {
		t.Error("Init with invalid path should fail")
		Close()
	}
}

func TestLevels_WithoutInit(t *testing.T) {
	// Ensure logger is not initialised. Subsequent calls must not panic
	// (they should silently no-op when globalLogger is nil).
	Close()
	defer Close()
	Info("info %d", 1)
	Debug("debug %d", 2)
	Error("error %d", 3)
	Warn("warn %d", 4)

	// GetWriter without Init returns io.Discard.
	if GetWriter() != io.Discard {
		t.Error("GetWriter without Init should return io.Discard")
	}
}

func TestLevels_WithInit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "levels.log")
	if err := Init(path); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer Close()

	Info("info-msg %d", 7)
	Debug("debug-msg %s", "x")
	Error("error-msg")
	Warn("warn-msg %v", true)

	// Close before reading so any buffered content is flushed.
	Close()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(body)
	for _, want := range []string{
		"[INFO] info-msg 7",
		"[DEBUG] debug-msg x",
		"[ERROR] error-msg",
		"[WARN] warn-msg true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("log content missing %q\nlog:\n%s", want, got)
		}
	}
}

func TestClose_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := Init(filepath.Join(dir, "c.log")); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Close()
	Close() // second close should not panic
}
