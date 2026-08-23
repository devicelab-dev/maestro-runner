package core

import (
	"strings"
	"testing"
)

// Verbatim envelope from an emulator on API 34.
const realDropbox = `Drop box contents: 670 entries
Max entries: 1000
Searching for: data_app_crash

========================================
2026-08-19 13:54:41 data_app_crash (text, 1087 bytes)
Process: com.testhiveapp
PID: 8215
UID: 10196
Package: com.testhiveapp v1 (1.0)
Foreground: Yes
Dropped-Count: 0

android.app.RemoteServiceException$CrashedByAdbException: shell-induced crash
	at android.app.ActivityThread.throwRemoteServiceException(ActivityThread.java:2090)
	at android.app.ActivityThread$H.handleMessage(ActivityThread.java:2369)
`

// A tombstone carries no Process: header — the body names the process.
const realTombstone = `Searching for: SYSTEM_TOMBSTONE

========================================
2026-08-17 18:32:00 SYSTEM_TOMBSTONE (compressed text, 14398 bytes)
isPrevious: true
Build: google/sdk_gphone64_arm64/emu64a:14/UE1A.230829.050/12077443:userdebug/dev-keys
Hardware: goldfish_arm64
Dropped-Count: 3

*** *** *** *** *** *** *** *** *** *** *** *** *** *** *** ***
Build fingerprint: 'google/sdk_gphone64_arm64/emu64a:14'
pid: 4500, tid: 4712, name: bt_stack_manage  >>> com.google.android.bluetooth <<<
signal 6 (SIGABRT), code -1 (SI_QUEUE), fault addr --------
Abort message: 'assertion failed'
`

func TestParseAndroidDropbox_RealCrash(t *testing.T) {
	entries := ParseAndroidDropbox(realDropbox)
	if len(entries) != 1 {
		t.Fatalf("parsed %d entries, want 1", len(entries))
	}
	e := entries[0]

	if e.Tag != "data_app_crash" {
		t.Errorf("Tag = %q", e.Tag)
	}
	if e.Process != "com.testhiveapp" || e.PID != 8215 {
		t.Errorf("process/pid = %q / %d", e.Process, e.PID)
	}
	// "com.testhiveapp v1 (1.0)" — the identifier, not the whole line.
	if e.Package != "com.testhiveapp" {
		t.Errorf("Package = %q", e.Package)
	}
	if e.Timestamp.IsZero() {
		t.Error("timestamp should parse — it is what scopes an entry to a flow")
	}
	want := "android.app.RemoteServiceException$CrashedByAdbException: shell-induced crash"
	if got := e.Cause(); got != want {
		t.Errorf("Cause() = %q, want %q", got, want)
	}
	if !strings.Contains(e.Body, "at android.app.ActivityThread") {
		t.Error("body should retain the stack trace")
	}
	if !e.ConcernsPackage("com.testhiveapp") {
		t.Error("entry should be attributed to its package")
	}
	if e.ConcernsPackage("com.other.app") || e.ConcernsPackage("") {
		t.Error("entry must not be attributed to another package, or to none")
	}
}

func TestParseAndroidDropbox_TombstoneNamesProcessFromBody(t *testing.T) {
	entries := ParseAndroidDropbox(realTombstone)
	if len(entries) != 1 {
		t.Fatalf("parsed %d entries, want 1", len(entries))
	}
	e := entries[0]

	if e.Tag != "SYSTEM_TOMBSTONE" {
		t.Errorf("Tag = %q", e.Tag)
	}
	// No Process: header; recovered from the `>>> name <<<` marker.
	if e.Process != "com.google.android.bluetooth" {
		t.Errorf("Process = %q, want it recovered from the body", e.Process)
	}
	// The preamble banner and fingerprint are not the cause.
	if got := e.Cause(); !strings.HasPrefix(got, "pid: 4500") {
		t.Errorf("Cause() = %q, should skip the banner and fingerprint", got)
	}
}

// The platform rate-limits repeated crashes from one process — exactly what a
// crashing app produces — so a zero entry count never proves nothing crashed.
// Dropped-Count is the platform saying so, and must survive parsing.
func TestParseAndroidDropbox_SurfacesDroppedCount(t *testing.T) {
	entries := ParseAndroidDropbox(realTombstone)
	if entries[0].DroppedCount != 3 {
		t.Errorf("DroppedCount = %d, want 3", entries[0].DroppedCount)
	}
	if ParseAndroidDropbox(realDropbox)[0].DroppedCount != 0 {
		t.Error("a clean entry should report zero drops")
	}
}

func TestParseAndroidDropbox_Empty(t *testing.T) {
	for name, in := range map[string]string{
		"no output":      "",
		"nothing found":  "Drop box contents: 0 entries\nSearching for: data_app_crash\n",
		"separator only": "====================\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := ParseAndroidDropbox(in); len(got) != 0 {
				t.Errorf("expected no entries, got %d", len(got))
			}
		})
	}
}

func TestParseAndroidDropbox_MultipleEntries(t *testing.T) {
	doubled := realDropbox + "\n========================================\n" +
		"2026-08-19 14:00:00 data_app_anr (text, 40 bytes)\nProcess: com.testhiveapp\n\nANR in com.testhiveapp\n"
	entries := ParseAndroidDropbox(doubled)
	if len(entries) != 2 {
		t.Fatalf("parsed %d entries, want 2", len(entries))
	}
	if entries[1].Tag != "data_app_anr" || entries[1].Cause() != "ANR in com.testhiveapp" {
		t.Errorf("second entry = %q / %q", entries[1].Tag, entries[1].Cause())
	}
}
