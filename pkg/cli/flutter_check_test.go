package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// makeBundle builds a fake .app directory tree and optionally drops a
// kernel_blob.bin marker at the given relative subpath.
func makeBundle(t *testing.T, name string, kernelRelPath string) string {
	t.Helper()
	dir := t.TempDir()
	appPath := filepath.Join(dir, name)
	if err := os.MkdirAll(appPath, 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	if kernelRelPath != "" {
		blobPath := filepath.Join(appPath, kernelRelPath)
		if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
			t.Fatalf("mkdir blob parent: %v", err)
		}
		if err := os.WriteFile(blobPath, []byte("kernel"), 0o644); err != nil {
			t.Fatalf("write blob: %v", err)
		}
	}
	return appPath
}

func TestDetectFlutterDebugBuild_StandardPath(t *testing.T) {
	app := makeBundle(t, "Runner.app", "Frameworks/App.framework/flutter_assets/kernel_blob.bin")

	isDebug, reason := detectFlutterDebugBuild(app)
	if !isDebug {
		t.Fatal("expected isDebug=true for kernel_blob under Frameworks/App.framework")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestDetectFlutterDebugBuild_AltShallowerPath(t *testing.T) {
	app := makeBundle(t, "Runner.app", "flutter_assets/kernel_blob.bin")

	isDebug, _ := detectFlutterDebugBuild(app)
	if !isDebug {
		t.Fatal("expected isDebug=true for kernel_blob under flutter_assets/")
	}
}

func TestDetectFlutterDebugBuild_ReleaseBuild(t *testing.T) {
	// Release/profile builds AOT-compile the kernel into the App binary;
	// kernel_blob.bin is absent.
	app := makeBundle(t, "Runner.app", "")
	// Drop only non-marker files to look like a real release bundle.
	if err := os.MkdirAll(filepath.Join(app, "Frameworks/App.framework/flutter_assets"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(app, "Frameworks/App.framework/flutter_assets/AssetManifest.json"), []byte("{}"), 0o644)

	isDebug, reason := detectFlutterDebugBuild(app)
	if isDebug {
		t.Errorf("expected isDebug=false for release-shaped bundle, got reason: %s", reason)
	}
}

func TestDetectFlutterDebugBuild_NotAFlutterApp(t *testing.T) {
	app := makeBundle(t, "Runner.app", "")
	isDebug, _ := detectFlutterDebugBuild(app)
	if isDebug {
		t.Error("empty bundle should not be flagged as Flutter debug")
	}
}

func TestDetectFlutterDebugBuild_NotAnAppDirectory(t *testing.T) {
	// .ipa or non-directory paths — we don't inspect those.
	if isDebug, _ := detectFlutterDebugBuild("/path/to/missing.app"); isDebug {
		t.Error("missing path should not be flagged")
	}

	tempFile, err := os.CreateTemp(t.TempDir(), "Runner.app")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if isDebug, _ := detectFlutterDebugBuild(tempFile.Name()); isDebug {
		t.Error("file (not dir) should not be flagged")
	}
}

func TestDetectFlutterDebugBuild_EmptyPath(t *testing.T) {
	if isDebug, _ := detectFlutterDebugBuild(""); isDebug {
		t.Error("empty path should not be flagged")
	}
}

func TestDetectFlutterDebugBuild_WrongExtension(t *testing.T) {
	// A directory with the kernel_blob marker but not ending in .app
	// — could be intermediate build output. Skip the inspection.
	dir := t.TempDir()
	bundle := filepath.Join(dir, "Runner.bundle") // not .app
	if err := os.MkdirAll(filepath.Join(bundle, "Frameworks/App.framework/flutter_assets"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(bundle, "Frameworks/App.framework/flutter_assets/kernel_blob.bin"), []byte("k"), 0o644)

	if isDebug, _ := detectFlutterDebugBuild(bundle); isDebug {
		t.Error("non-.app directory should not be flagged")
	}
}
