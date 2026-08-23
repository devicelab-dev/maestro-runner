package devicelab_ios

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/devicelab-dev/maestro-runner/drivers"
	"github.com/devicelab-dev/maestro-runner/pkg/config"
)

// embeddedRunnerRoot is the runner project's path inside drivers.IOSRunnerSource.
const embeddedRunnerRoot = "ios/DevicelabIOSRunner"

// extractEmbeddedRunnerSource materializes the embedded runner source
// tree under the cache, keyed by its content hash so a module upgrade
// lands in a fresh slot while re-runs reuse the existing extraction.
// Returns the extracted project directory. Extraction goes through a
// temp directory + rename so a crash mid-write never leaves a
// half-populated slot that later runs would trust.
func extractEmbeddedRunnerSource() (string, error) {
	hash, err := embeddedSourceHash()
	if err != nil {
		return "", err
	}
	slot := filepath.Join(config.GetCacheDir(), "embedded-runner-src", hash)
	target := filepath.Join(slot, "DevicelabIOSRunner")
	if _, err := os.Stat(filepath.Join(target, "DevicelabIOSRunner.xcodeproj")); err == nil {
		return target, nil
	}

	tmp, err := os.MkdirTemp(filepath.Dir(slot), "extract-*")
	if err != nil {
		if mkErr := os.MkdirAll(filepath.Dir(slot), 0o755); mkErr != nil {
			return "", fmt.Errorf("create extraction dir: %w", mkErr)
		}
		if tmp, err = os.MkdirTemp(filepath.Dir(slot), "extract-*"); err != nil {
			return "", fmt.Errorf("create temp extraction dir: %w", err)
		}
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := extractFS(drivers.IOSRunnerSource, embeddedRunnerRoot, filepath.Join(tmp, "DevicelabIOSRunner")); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, slot); err != nil {
		// A concurrent extraction may have won the rename — trust its result.
		if _, statErr := os.Stat(filepath.Join(target, "DevicelabIOSRunner.xcodeproj")); statErr == nil {
			return target, nil
		}
		return "", fmt.Errorf("finalize extraction: %w", err)
	}
	return target, nil
}

// extractFS writes the subtree of fsys rooted at root into dstDir.
func extractFS(fsys fs.FS, root, dstDir string) error {
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
}

// embeddedSourceHash content-hashes the embedded runner tree, mirroring
// runnerSourceHash's scheme (sorted relative paths mixed with contents)
// so the hash is stable across processes and changes exactly when the
// embedded source changes.
func embeddedSourceHash() (string, error) {
	var files []string
	err := fs.WalkDir(drivers.IOSRunnerSource, embeddedRunnerRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	h := sha256.New()
	for _, p := range files {
		data, err := fs.ReadFile(drivers.IOSRunnerSource, p)
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}
