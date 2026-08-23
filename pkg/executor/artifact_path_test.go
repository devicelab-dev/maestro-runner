package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateArtifactPath(t *testing.T) {
	// Anchor cwd to a temp project root so the "under project" branch is
	// exercised deterministically (in real use cwd is the project root).
	root := t.TempDir()
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(prevWd) }()
	// macOS /var is a symlink to /private/var, so resolve to match filepath.Abs.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	flowDir := filepath.Join(root, "flows")

	cases := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"inside flow dir", filepath.Join(flowDir, "baseline.png"), false},
		{"nested inside flow dir", filepath.Join(flowDir, "screens", "a.png"), false},
		{"shared dir under project root", filepath.Join(root, "shared", "a.png"), false},
		{"sibling of flow dir via ..", filepath.Join(flowDir, "..", "shared", "a.png"), false},
		{"absolute system path", "/etc/passwd", true},
		{"deep traversal escaping project", filepath.Join(flowDir, "..", "..", "..", "..", "etc", "x.png"), true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateArtifactPath(c.path, flowDir)
			if (err != nil) != c.wantErr {
				t.Errorf("validateArtifactPath(%q) err=%v, wantErr=%v", c.path, err, c.wantErr)
			}
		})
	}
}
