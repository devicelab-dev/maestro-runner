package cli

import (
	"os"
	"path/filepath"
	"testing"
)

const validFlow = `appId: com.example
---
- launchApp
- tapOn:
    id: login
`

// Bad indentation inside a step mapping — the parser rejects it, and this is
// the class of mistake lint exists to catch before a device is involved.
const brokenFlow = `appId: com.example
---
- tapOn:
    id: login
   oops: bad-indent
`

func writeFlow(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestCollectFlowPaths(t *testing.T) {
	dir := t.TempDir()
	writeFlow(t, dir, "a.yaml", validFlow)
	writeFlow(t, dir, "b.yml", validFlow)
	writeFlow(t, dir, "notes.txt", "not a flow")
	nested := filepath.Join(dir, "sub")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFlow(t, nested, "c.yaml", validFlow)

	t.Run("walks directories for yaml and yml only", func(t *testing.T) {
		paths, err := collectFlowPaths([]string{dir})
		if err != nil {
			t.Fatalf("collectFlowPaths() error = %v", err)
		}
		if len(paths) != 3 {
			t.Errorf("got %d paths, want 3 (a.yaml, b.yml, sub/c.yaml): %v", len(paths), paths)
		}
		for _, p := range paths {
			if filepath.Ext(p) == ".txt" {
				t.Errorf("non-flow file collected: %s", p)
			}
		}
	})

	t.Run("an explicitly named file is taken whatever its extension", func(t *testing.T) {
		// Naming a file is an unambiguous request to check it — only directory
		// walking filters by extension.
		paths, err := collectFlowPaths([]string{filepath.Join(dir, "notes.txt")})
		if err != nil {
			t.Fatalf("collectFlowPaths() error = %v", err)
		}
		if len(paths) != 1 {
			t.Errorf("got %v, want the named file", paths)
		}
	})

	t.Run("de-duplicates a file named alongside its directory", func(t *testing.T) {
		paths, err := collectFlowPaths([]string{dir, filepath.Join(dir, "a.yaml")})
		if err != nil {
			t.Fatalf("collectFlowPaths() error = %v", err)
		}
		if len(paths) != 3 {
			t.Errorf("got %d paths, want 3 — a.yaml must not be checked twice: %v", len(paths), paths)
		}
	})

	t.Run("reports an unreadable target", func(t *testing.T) {
		if _, err := collectFlowPaths([]string{filepath.Join(dir, "nope")}); err == nil {
			t.Error("expected an error for a missing path")
		}
	})
}

func TestRunLint(t *testing.T) {
	t.Run("passes a directory of valid flows", func(t *testing.T) {
		dir := t.TempDir()
		writeFlow(t, dir, "a.yaml", validFlow)
		if err := runLintPaths([]string{dir}, true); err != nil {
			t.Errorf("expected valid flows to lint clean, got %v", err)
		}
	})

	t.Run("fails when any flow is broken", func(t *testing.T) {
		dir := t.TempDir()
		writeFlow(t, dir, "ok.yaml", validFlow)
		writeFlow(t, dir, "broken.yaml", brokenFlow)
		if err := runLintPaths([]string{dir}, true); err == nil {
			t.Error("expected a broken flow to fail the lint")
		}
	})

	t.Run("reports when nothing matched", func(t *testing.T) {
		if err := runLintPaths([]string{t.TempDir()}, true); err == nil {
			t.Error("expected an error when no flow files were found")
		}
	})
}
