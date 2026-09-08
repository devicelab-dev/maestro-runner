package simulator

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// localFileStorageGroup is the app group that backs "On My iPhone" in the
// Files app. Every app's document picker browses it, so a file dropped into
// its "File Provider Storage" directory is selectable by any app — the
// document-picker counterpart of `simctl addmedia`, which simctl itself does
// not offer for documents.
const localFileStorageGroup = "group.com.apple.FileProvider.LocalStorage"

// AddDocuments copies files into the simulator's "On My iPhone" storage.
func AddDocuments(udid string, files []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir, err := localFileStorageDir(filepath.Join(home, "Library/Developer/CoreSimulator/Devices"), udid)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := copyFile(f, filepath.Join(dir, filepath.Base(f))); err != nil {
			return err
		}
	}
	return nil
}

// localFileStorageDir finds the "File Provider Storage" directory of the
// local-storage app group under devicesDir/<udid>. Several app groups carry a
// directory of that name; the group is identified by the container manager's
// metadata plist, read with plutil so both binary and XML plists work.
func localFileStorageDir(devicesDir, udid string) (string, error) {
	groups := filepath.Join(devicesDir, udid, "data/Containers/Shared/AppGroup")
	entries, err := os.ReadDir(groups)
	if err != nil {
		return "", fmt.Errorf("simulator %s has no app-group containers (%w) — has it been booted at least once?", udid, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		plist := filepath.Join(groups, e.Name(), ".com.apple.mobile_container_manager.metadata.plist")
		out, err := exec.Command("plutil", "-extract", "MCMMetadataIdentifier", "raw", "-o", "-", plist).Output()
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(out)) == localFileStorageGroup {
			dir := filepath.Join(groups, e.Name(), "File Provider Storage")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
			return dir, nil
		}
	}
	return "", fmt.Errorf("simulator %s has no local file storage container (%s); open the Files app once and retry", udid, localFileStorageGroup)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
