package device

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// scriptedADB fakes execCommand with per-command answers keyed by a substring
// of the adb argument list, and records every call so tests can assert which
// installs and uninstalls happened. A value of "!fail" makes the command exit
// non-zero. Unmatched commands succeed with empty output. An uninstalled
// package stops being listed by `pm list packages`, as on a real device.
type scriptedADB struct {
	mu      sync.Mutex
	answers []struct{ match, out string }
	calls   []string
	removed []string
}

func (s *scriptedADB) on(match, out string) *scriptedADB {
	s.answers = append(s.answers, struct{ match, out string }{match, out})
	return s
}

func (s *scriptedADB) exec(_ string, args ...string) *exec.Cmd {
	joined := strings.Join(args, " ")
	s.mu.Lock()
	s.calls = append(s.calls, joined)
	if i := strings.Index(joined, "uninstall "); i >= 0 {
		s.removed = append(s.removed, joined[i+len("uninstall "):])
	}
	removed := s.removed
	s.mu.Unlock()
	for _, a := range s.answers {
		if strings.Contains(joined, a.match) {
			if a.out == "!fail" {
				return exec.Command("false")
			}
			return exec.Command("printf", "%s", dropRemoved(a.out, removed))
		}
	}
	return exec.Command("true")
}

// dropRemoved deletes "package:<pkg>" lines for uninstalled packages.
func dropRemoved(out string, removed []string) string {
	lines := strings.Split(out, "\n")
	kept := lines[:0]
	for _, line := range lines {
		gone := false
		for _, pkg := range removed {
			gone = gone || line == "package:"+pkg
		}
		if !gone {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// count returns how many recorded calls contain sub.
func (s *scriptedADB) count(sub string) int {
	n := 0
	for _, c := range s.calls {
		if strings.Contains(c, sub) {
			n++
		}
	}
	return n
}

// writeDriverAPKs creates the two bundled driver files with the given bodies
// under the fixed names DeviceDeck also writes, and returns the directory and
// the SHA-256 of each file.
func writeDriverAPKs(t *testing.T, server, test string) (dir, serverSum, testSum string) {
	t.Helper()
	dir = t.TempDir()
	sp := filepath.Join(dir, "devicelab-android-driver.apk")
	tp := filepath.Join(dir, "devicelab-android-driver-test.apk")
	for p, body := range map[string]string{sp: server, tp: test} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	serverSum, _ = fileSHA256(sp)
	testSum, _ = fileSHA256(tp)
	return dir, serverSum, testSum
}

const (
	serverAPKOnDevice = "/data/app/~~a==/dev.devicelab.driver.android-b==/base.apk"
	testAPKOnDevice   = "/data/app/~~c==/dev.devicelab.driver.android.test-d==/base.apk"
)

// deviceWithDriver scripts a device where both driver packages are installed
// and their base APKs hash to serverSum and testSum.
func deviceWithDriver(serverSum, testSum string) *scriptedADB {
	s := &scriptedADB{}
	// Order matters: the test package name contains the server's, so the
	// more specific matches come first.
	s.on("pm list packages "+DeviceLabDriverTest, "package:"+DeviceLabDriverTest+"\n")
	s.on("pm list packages "+DeviceLabDriverServer, "package:"+DeviceLabDriverServer+"\n")
	s.on("pm path '"+DeviceLabDriverTest+"'", "package:"+testAPKOnDevice+"\n")
	s.on("pm path '"+DeviceLabDriverServer+"'", "package:"+serverAPKOnDevice+"\n")
	s.on("sha256sum '"+testAPKOnDevice+"'", testSum+"  "+testAPKOnDevice+"\n")
	s.on("sha256sum '"+serverAPKOnDevice+"'", serverSum+"  "+serverAPKOnDevice+"\n")
	return s
}

func TestInstallDeviceLabDriver(t *testing.T) {
	dir, serverSum, testSum := writeDriverAPKs(t, "server-build-1", "test-build-1")
	_, otherServer, otherTest := writeDriverAPKs(t, "server-build-2", "test-build-2")

	tests := []struct {
		name          string
		adb           *scriptedADB
		wantInstalls  int
		wantUninstall []string
	}{
		{
			name:         "identical driver already installed is kept",
			adb:          deviceWithDriver(serverSum, testSum),
			wantInstalls: 0,
		},
		{
			name:          "rebuilt server with same version is replaced along with test",
			adb:           deviceWithDriver(otherServer, testSum),
			wantInstalls:  2,
			wantUninstall: []string{DeviceLabDriverServer, DeviceLabDriverTest},
		},
		{
			name:          "only the test APK differs",
			adb:           deviceWithDriver(serverSum, otherTest),
			wantInstalls:  1,
			wantUninstall: []string{DeviceLabDriverTest},
		},
		{
			name:          "device without sha256sum falls back to reinstall",
			adb:           (&scriptedADB{}).on("sha256sum", "!fail").on("pm list packages", "package:"+DeviceLabDriverServer+"\npackage:"+DeviceLabDriverTest+"\n").on("pm path", "package:/x/base.apk\n"),
			wantInstalls:  2,
			wantUninstall: []string{DeviceLabDriverServer, DeviceLabDriverTest},
		},
		{
			name:         "nothing installed installs both without hashing",
			adb:          &scriptedADB{},
			wantInstalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFakeExec(t, tt.adb.exec)
			d := &AndroidDevice{serial: "emulator-5554", adbPath: "adb"}
			if err := d.InstallDeviceLabDriver(dir); err != nil {
				t.Fatalf("InstallDeviceLabDriver: %v", err)
			}
			if got := tt.adb.count(" install -r -g "); got != tt.wantInstalls {
				t.Errorf("installs = %d, want %d (calls: %q)", got, tt.wantInstalls, tt.adb.calls)
			}
			if got := tt.adb.count(" uninstall "); got != len(tt.wantUninstall) {
				t.Errorf("uninstalls = %d, want %d (calls: %q)", got, len(tt.wantUninstall), tt.adb.calls)
			}
			for _, pkg := range tt.wantUninstall {
				if tt.adb.count("uninstall "+pkg) == 0 {
					t.Errorf("expected uninstall of %s (calls: %q)", pkg, tt.adb.calls)
				}
			}
		})
	}
}

func TestInstallDeviceLabDriverErrors(t *testing.T) {
	t.Run("missing APK", func(t *testing.T) {
		withFakeExec(t, (&scriptedADB{}).exec)
		d := &AndroidDevice{serial: "s", adbPath: "adb"}
		if err := d.InstallDeviceLabDriver(t.TempDir()); err == nil {
			t.Fatal("expected error for empty APK directory")
		}
	})
	t.Run("install fails", func(t *testing.T) {
		dir, _, _ := writeDriverAPKs(t, "a", "b")
		withFakeExec(t, (&scriptedADB{}).on(" install ", "!fail").exec)
		d := &AndroidDevice{serial: "s", adbPath: "adb"}
		if err := d.InstallDeviceLabDriver(dir); err == nil {
			t.Fatal("expected install error")
		}
	})
}

func TestInstalledAPKMatches(t *testing.T) {
	dir, serverSum, _ := writeDriverAPKs(t, "server", "test")
	apk := filepath.Join(dir, "devicelab-android-driver.apk")

	tests := []struct {
		name string
		adb  *scriptedADB
		path string
		want bool
	}{
		{"same bytes", deviceWithDriver(serverSum, ""), apk, true},
		{"uppercase digest from device", deviceWithDriver(strings.ToUpper(serverSum), ""), apk, true},
		{"different bytes", deviceWithDriver(strings.Repeat("0", 64), ""), apk, false},
		{"local file missing", deviceWithDriver(serverSum, ""), filepath.Join(dir, "nope.apk"), false},
		{"pm path fails", (&scriptedADB{}).on("pm path", "!fail"), apk, false},
		{"pm path empty", (&scriptedADB{}).on("pm path", ""), apk, false},
		{"sha256sum fails", (&scriptedADB{}).on("pm path", "package:/x/base.apk").on("sha256sum", "!fail"), apk, false},
		{"sha256sum not found message", (&scriptedADB{}).on("pm path", "package:/x/base.apk").on("sha256sum", "/system/bin/sh: sha256sum: not found"), apk, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFakeExec(t, tt.adb.exec)
			d := &AndroidDevice{serial: "s", adbPath: "adb"}
			if got := d.installedAPKMatches(DeviceLabDriverServer, tt.path); got != tt.want {
				t.Errorf("installedAPKMatches = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBaseAPKPath(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"single", "package:/data/app/x/base.apk\n", "/data/app/x/base.apk"},
		{"windows line endings", "package:/data/app/x/base.apk\r\n", "/data/app/x/base.apk"},
		{"single non-base name", "package:/data/app/x.apk", "/data/app/x.apk"},
		{"split install picks base", "package:/d/split_config.en.apk\npackage:/d/base.apk\n", "/d/base.apk"},
		{"split without base", "package:/d/a.apk\npackage:/d/b.apk\n", ""},
		{"empty", "", ""},
		{"error text", "Error: package not found", ""},
		{"empty path", "package:\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := baseAPKPath(tt.in); got != tt.want {
				t.Errorf("baseAPKPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseSHA256Sum(t *testing.T) {
	valid := strings.Repeat("ab", 32)
	tests := []struct {
		name, in, want string
	}{
		{"toybox format", valid + "  /data/app/x/base.apk\n", valid},
		{"uppercase normalized", strings.ToUpper(valid) + " f", valid},
		{"empty", "", ""},
		{"too short", "abcd  f", ""},
		{"not hex", strings.Repeat("zz", 32) + "  f", ""},
		{"shell error", "sh: sha256sum: not found", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseSHA256Sum(tt.in); got != tt.want {
				t.Errorf("parseSHA256Sum(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFileSHA256(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := fileSHA256(p)
	if err != nil {
		t.Fatal(err)
	}
	// SHA-256("abc"), FIPS 180-2 test vector.
	if want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"; got != want {
		t.Errorf("fileSHA256 = %s, want %s", got, want)
	}
	if _, err := fileSHA256(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("expected error for missing file")
	}
	// A directory opens but cannot be read, exercising the copy error path.
	if _, err := fileSHA256(t.TempDir()); err == nil {
		t.Error("expected error for directory")
	}
}
