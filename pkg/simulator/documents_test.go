package simulator

import (
	"os"
	"path/filepath"
	"testing"
)

const metadataPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>MCMMetadataIdentifier</key><string>%s</string>
</dict></plist>`

func writeGroup(t *testing.T, groups, name, identifier string) string {
	t.Helper()
	dir := filepath.Join(groups, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := filepath.Join(dir, ".com.apple.mobile_container_manager.metadata.plist")
	if err := os.WriteFile(plist, []byte(fmtPlist(identifier)), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func fmtPlist(identifier string) string {
	return sprintf(metadataPlist, identifier)
}

// Several app groups carry a "File Provider Storage" directory; only the
// local-storage group is the one the Files app shows as "On My iPhone".
func TestLocalFileStorageDir_PicksTheLocalStorageGroup(t *testing.T) {
	if _, err := os.Stat("/usr/bin/plutil"); err != nil {
		t.Skip("plutil not available")
	}
	devices := t.TempDir()
	groups := filepath.Join(devices, "UDID-1", "data/Containers/Shared/AppGroup")
	other := writeGroup(t, groups, "AAAA", "group.com.apple.iCloudDrive")
	if err := os.MkdirAll(filepath.Join(other, "File Provider Storage"), 0o755); err != nil {
		t.Fatal(err)
	}
	local := writeGroup(t, groups, "BBBB", localFileStorageGroup)

	got, err := localFileStorageDir(devices, "UDID-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(local, "File Provider Storage"); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("the storage directory should be created when missing: %v", err)
	}
}

func TestLocalFileStorageDir_ErrorsWhenAbsent(t *testing.T) {
	if _, err := os.Stat("/usr/bin/plutil"); err != nil {
		t.Skip("plutil not available")
	}
	devices := t.TempDir()
	groups := filepath.Join(devices, "UDID-2", "data/Containers/Shared/AppGroup")
	writeGroup(t, groups, "AAAA", "group.com.apple.iCloudDrive")
	if _, err := localFileStorageDir(devices, "UDID-2"); err == nil {
		t.Fatal("expected an error when no local storage group exists")
	}
	if _, err := localFileStorageDir(devices, "NEVER-BOOTED"); err == nil {
		t.Fatal("expected an error for a simulator with no containers")
	}
}

func sprintf(format, a string) string { return replaceFirst(format, "%s", a) }

func replaceFirst(s, old, repl string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + repl + s[i+len(old):]
		}
	}
	return s
}
