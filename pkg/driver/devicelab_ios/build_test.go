package devicelab_ios

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTree writes files (relative path -> content) under root.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunnerSourceHash_StableForSameContent(t *testing.T) {
	files := map[string]string{
		"A.swift":            "let x = 1",
		"sub/B.m":            "// objc",
		"Proj.xcodeproj/pbx": "project",
	}
	a := t.TempDir()
	b := t.TempDir()
	writeTree(t, a, files)
	writeTree(t, b, files)

	ha, err := runnerSourceHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := runnerSourceHash(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Errorf("identical trees hashed differently: %s vs %s", ha, hb)
	}
	if len(ha) != 12 {
		t.Errorf("hash len = %d, want 12", len(ha))
	}
}

func TestRunnerSourceHash_ChangesOnEdit(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"A.swift": "let x = 1"})
	before, _ := runnerSourceHash(root)

	// Content change must invalidate the cache key.
	if err := os.WriteFile(filepath.Join(root, "A.swift"), []byte("let x = 2"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterEdit, _ := runnerSourceHash(root)
	if before == afterEdit {
		t.Error("hash unchanged after editing a source file")
	}

	// Adding a new source file must also invalidate.
	writeTree(t, root, map[string]string{"C.swift": "let y = 3"})
	afterAdd, _ := runnerSourceHash(root)
	if afterEdit == afterAdd {
		t.Error("hash unchanged after adding a source file")
	}
}

func TestRunnerSourceHash_IgnoresBuildNoise(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"A.swift": "let x = 1"})
	base, _ := runnerSourceHash(root)

	// Xcode user state / derived data must not affect the hash.
	writeTree(t, root, map[string]string{
		"DevicelabIOSRunner.xcodeproj/xcuserdata/me.xcuserstate": "binary-ish noise",
		"build/Products/Debug/whatever":                          "artifact",
	})
	withNoise, _ := runnerSourceHash(root)
	if base != withNoise {
		t.Errorf("build noise perturbed the hash: %s vs %s", base, withNoise)
	}
}
