package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// atomicWriteJSON writes JSON to a file atomically.
// It writes to a temp file first, then renames to the target path.
// This ensures readers never see partial writes.
func atomicWriteJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	return atomicWriteFile(path, data, 0644)
}

// atomicWriteFile writes data to a file atomically.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write to temp file
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return err
	}

	// On Windows, rename fails if target exists
	if runtime.GOOS == "windows" {
		os.Remove(path)
	}

	// Atomic rename
	return os.Rename(tmpPath, path)
}

// ensureDir creates a directory if it doesn't exist.
func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
