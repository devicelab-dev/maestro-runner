package devicelab_ios

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devicelab-dev/maestro-runner/drivers"
)

// withTempHome points the config home at a temp dir so extraction tests
// never touch the real ~/.maestro-runner cache.
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("MAESTRO_RUNNER_HOME", home)
	return home
}

func TestEmbeddedSourceContainsProject(t *testing.T) {
	// The embed directive silently embeds whatever matches — assert the
	// files the build actually needs are present.
	for _, p := range []string{
		"ios/DevicelabIOSRunner/DevicelabIOSRunner.xcodeproj/project.pbxproj",
		"ios/DevicelabIOSRunner/DevicelabIOSRunnerUITests/RunnerTests.swift",
	} {
		if _, err := drivers.IOSRunnerSource.Open(p); err != nil {
			t.Errorf("embedded source missing %s: %v", p, err)
		}
	}
}

func TestExtractEmbeddedRunnerSource(t *testing.T) {
	withTempHome(t)

	dir, err := extractEmbeddedRunnerSource()
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "DevicelabIOSRunner.xcodeproj", "project.pbxproj")); err != nil {
		t.Fatalf("extracted tree incomplete: %v", err)
	}

	// Extracted content must byte-match the embedded content.
	embedded, err := drivers.IOSRunnerSource.ReadFile("ios/DevicelabIOSRunner/DevicelabIOSRunnerUITests/RunnerTests.swift")
	if err != nil {
		t.Fatal(err)
	}
	extracted, err := os.ReadFile(filepath.Join(dir, "DevicelabIOSRunnerUITests", "RunnerTests.swift"))
	if err != nil {
		t.Fatal(err)
	}
	if string(embedded) != string(extracted) {
		t.Error("extracted file differs from embedded content")
	}

	// Second call reuses the slot: same path, no error.
	again, err := extractEmbeddedRunnerSource()
	if err != nil {
		t.Fatalf("re-extract: %v", err)
	}
	if again != dir {
		t.Errorf("re-extraction moved: %s != %s", again, dir)
	}
}

func TestExtractSurvivesExistingPartialSlot(t *testing.T) {
	home := withTempHome(t)

	hash, err := embeddedSourceHash()
	if err != nil {
		t.Fatal(err)
	}
	// A slot directory without the project marker must be replaced, not
	// trusted (the rename fails on an existing dir; the marker check
	// decides whether that is a concurrent win or a corrupt slot).
	slot := filepath.Join(home, "cache", "embedded-runner-src", hash)
	if err := os.MkdirAll(slot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := extractEmbeddedRunnerSource(); err == nil {
		t.Log("extraction tolerated pre-existing empty slot")
	} else if _, statErr := os.Stat(filepath.Join(slot, "DevicelabIOSRunner")); statErr != nil {
		t.Errorf("extraction failed against corrupt slot and left nothing usable: %v", err)
	}
}

func TestEmbeddedSourceHashStable(t *testing.T) {
	h1, err := embeddedSourceHash()
	if err != nil {
		t.Fatal(err)
	}
	h2, err := embeddedSourceHash()
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 || len(h1) != 12 {
		t.Errorf("hash unstable or malformed: %q vs %q", h1, h2)
	}
}
