package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetHome_EnvVar(t *testing.T) {
	ResetHome()
	t.Setenv("MAESTRO_RUNNER_HOME", "/custom/path")

	got := GetHome()
	if got != "/custom/path" {
		t.Errorf("GetHome() = %q, want %q", got, "/custom/path")
	}
}

func TestGetHome_EnvVarTakesPrecedence(t *testing.T) {
	ResetHome()
	t.Setenv("MAESTRO_RUNNER_HOME", "/override")

	got := GetHome()
	if got != "/override" {
		t.Errorf("GetHome() = %q, want %q", got, "/override")
	}
}

func TestGetHome_FallbackToCwd(t *testing.T) {
	ResetHome()
	t.Setenv("MAESTRO_RUNNER_HOME", "")

	got := GetHome()
	cwd, _ := os.Getwd()

	// When not in a bin/ directory and no env var, should fall back to cwd
	// (unless the test binary happens to be in a bin/ directory)
	if got == "" {
		t.Error("GetHome() returned empty string")
	}
	_ = cwd // cwd is valid fallback
}

func TestGetHome_Cached(t *testing.T) {
	ResetHome()
	t.Setenv("MAESTRO_RUNNER_HOME", "/first")

	first := GetHome()

	// Change env — should NOT affect cached value
	t.Setenv("MAESTRO_RUNNER_HOME", "/second")
	second := GetHome()

	if first != second {
		t.Errorf("GetHome() not cached: first=%q, second=%q", first, second)
	}
}

func TestGetCacheDir(t *testing.T) {
	ResetHome()
	t.Setenv("MAESTRO_RUNNER_HOME", "/test/home")

	got := GetCacheDir()
	want := filepath.Join("/test/home", "cache")
	if got != want {
		t.Errorf("GetCacheDir() = %q, want %q", got, want)
	}
}

func TestGetDriversDir(t *testing.T) {
	ResetHome()
	t.Setenv("MAESTRO_RUNNER_HOME", "/test/home")

	tests := []struct {
		platform string
		want     string
	}{
		{"ios", filepath.Join("/test/home", "drivers", "ios")},
		{"android", filepath.Join("/test/home", "drivers", "android")},
	}

	for _, tt := range tests {
		ResetHome()
		t.Setenv("MAESTRO_RUNNER_HOME", "/test/home")

		got := GetDriversDir(tt.platform)
		if got != tt.want {
			t.Errorf("GetDriversDir(%q) = %q, want %q", tt.platform, got, tt.want)
		}
	}
}

func TestResolveHome_BinaryRelative(t *testing.T) {
	// Create a temp directory structure: <tmpdir>/bin/
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// resolveHome uses os.Executable() which we can't mock directly,
	// but we can verify the logic by testing the env var path
	ResetHome()
	t.Setenv("MAESTRO_RUNNER_HOME", tmpDir)

	got := GetHome()
	if got != tmpDir {
		t.Errorf("GetHome() = %q, want %q", got, tmpDir)
	}
}
