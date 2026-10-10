package cli

import (
	"errors"
	"os"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

type fakeShareDriver struct {
	core.Driver
	tree, shot []byte
	treeErr    error
}

func (f *fakeShareDriver) Hierarchy() ([]byte, error)  { return f.tree, f.treeErr }
func (f *fakeShareDriver) Screenshot() ([]byte, error) { return f.shot, nil }

// useShareDir points the share sockets at a short directory: a Unix socket
// path must stay under about 104 bytes, and t.TempDir() is long on macOS.
func useShareDir(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mrh")
	if err != nil {
		t.Fatal(err)
	}
	old := hierarchyShareDir
	hierarchyShareDir = dir
	t.Cleanup(func() { hierarchyShareDir = old; os.RemoveAll(dir) })
}

func TestHierarchyShare(t *testing.T) {
	useShareDir(t)
	if liveHierarchyShare("") != "" || liveHierarchyShare("emulator-5554") != "" {
		t.Fatal("found a share with no test running")
	}

	driver := &fakeShareDriver{tree: []byte(`{"text":"Home"}`), shot: []byte("\x89PNG")}
	stop := shareHierarchy("emulator-5554", driver)

	path := liveHierarchyShare("emulator-5554")
	if path == "" || liveHierarchyShare("") != path {
		t.Fatalf("share not found by serial (%q) or as the only one (%q)", path, liveHierarchyShare(""))
	}
	if liveHierarchyShare("other-device") != "" {
		t.Error("share found for another device")
	}
	if got, err := askHierarchyShare(path, "hierarchy"); err != nil || string(got) != `{"text":"Home"}` {
		t.Errorf("hierarchy = %q, %v", got, err)
	}
	if got, err := askHierarchyShare(path, "screenshot"); err != nil || string(got) != "\x89PNG" {
		t.Errorf("screenshot = %q, %v", got, err)
	}
	driver.treeErr = errors.New("snapshot failed")
	if _, err := askHierarchyShare(path, "hierarchy"); err == nil {
		t.Error("a driver error was not passed on")
	}

	// A second run on the same device does not take the socket over.
	stopSecond := shareHierarchy("emulator-5554", &fakeShareDriver{tree: []byte("second")})
	stopSecond()
	if liveHierarchyShare("emulator-5554") != path {
		t.Error("the first run's share is gone after a second run stopped")
	}

	stop()
	if liveHierarchyShare("emulator-5554") != "" {
		t.Error("share still live after stop")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("socket file left behind: %v", err)
	}
}
