package core

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// `dumpsys dropbox --print [date] [time] <tag>` reads the platform's crash
// archive, and it works unrooted: the shell package holds DUMP and
// PACKAGE_USAGE_STATS. That matters most for SYSTEM_TOMBSTONE — /data/tombstones
// is unreadable without root, but BootReceiver copies tombstones into dropbox,
// so a native backtrace is reachable on a stock retail phone.
//
// Real envelope, captured from an emulator on API 34:
//
//	========================================
//	2026-08-19 13:54:41 data_app_crash (text, 1087 bytes)
//	Process: com.testhiveapp
//	PID: 8215
//	Dropped-Count: 0
//
//	android.app.RemoteServiceException$CrashedByAdbException: shell-induced crash
//		at android.app.ActivityThread.throwRemoteServiceException(...)
//
// Tombstones use the same envelope with different headers — no Process: line —
// so the process name is recovered from the body's `>>> name <<<` marker.

// Dropbox tags worth collecting after a flow.
const (
	DropboxTagCrash       = "data_app_crash"
	DropboxTagANR         = "data_app_anr"
	DropboxTagNativeCrash = "data_app_native_crash"
	DropboxTagTombstone   = "SYSTEM_TOMBSTONE"
)

// AndroidDropboxEntry is one archived crash, ANR or tombstone.
type AndroidDropboxEntry struct {
	Timestamp time.Time
	Tag       string
	Process   string
	PID       int
	Package   string
	Body      string

	// DroppedCount is the platform's own admission that it discarded entries
	// for this process. DropBoxManagerService rate-limits repeats, which is
	// precisely the pattern a crashing app produces — so a zero entry count is
	// never proof that nothing crashed, and this is how to say so.
	DroppedCount int
}

var (
	dropboxSeparator  = regexp.MustCompile(`(?m)^={20,}\s*$`)
	dropboxHeaderLine = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) (\S+) \(`)
	tombstoneProcess  = regexp.MustCompile(`>>> (\S+) <<<`)
)

// ParseAndroidDropbox reads `dumpsys dropbox --print` output.
func ParseAndroidDropbox(out string) []AndroidDropboxEntry {
	chunks := dropboxSeparator.Split(out, -1)
	if len(chunks) < 2 {
		return nil
	}

	var entries []AndroidDropboxEntry
	// chunks[0] is the preamble before the first separator.
	for _, chunk := range chunks[1:] {
		if entry, ok := parseDropboxChunk(chunk); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func parseDropboxChunk(chunk string) (AndroidDropboxEntry, bool) {
	lines := strings.Split(strings.TrimLeft(chunk, "\n"), "\n")
	if len(lines) == 0 {
		return AndroidDropboxEntry{}, false
	}

	m := dropboxHeaderLine.FindStringSubmatch(strings.TrimSpace(lines[0]))
	if m == nil {
		return AndroidDropboxEntry{}, false
	}

	entry := AndroidDropboxEntry{Tag: m[2]}
	// Device-local time with no zone in the output.
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], time.Local); err == nil {
		entry.Timestamp = t
	}

	// Headers run until the first blank line; the rest is the body.
	bodyStart := len(lines)
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			bodyStart = i + 1
			break
		}
		key, value, found := strings.Cut(line, ": ")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Process":
			entry.Process = strings.TrimSpace(value)
		case "Package":
			// Reported as "com.example v1 (1.0)" — keep the identifier.
			if fields := strings.Fields(value); len(fields) > 0 {
				entry.Package = fields[0]
			}
		case "PID":
			entry.PID, _ = strconv.Atoi(strings.TrimSpace(value))
		case "Dropped-Count":
			entry.DroppedCount, _ = strconv.Atoi(strings.TrimSpace(value))
		}
	}

	if bodyStart < len(lines) {
		entry.Body = strings.TrimRight(strings.Join(lines[bodyStart:], "\n"), "\n \t")
	}
	// Tombstones carry no Process: header — the body names the process instead.
	if entry.Process == "" {
		if pm := tombstoneProcess.FindStringSubmatch(entry.Body); pm != nil {
			entry.Process = pm[1]
		}
	}
	return entry, true
}

// Cause returns the first meaningful line of the body — the exception and its
// message for a JVM crash, the abort message or signal for a tombstone — which
// is what belongs in a failure message. The full body stays available as an
// artifact.
func (e AndroidDropboxEntry) Cause() string {
	for _, line := range strings.Split(e.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "at ") {
			continue
		}
		// Tombstone preamble lines are not the cause.
		if strings.HasPrefix(trimmed, "*** ") || strings.HasPrefix(trimmed, "Build fingerprint") {
			continue
		}
		return trimmed
	}
	return ""
}

// ConcernsPackage reports whether the entry belongs to the given package. An
// empty package matches nothing, so a caller that forgot to pass one does not
// silently collect the whole device's crashes.
func (e AndroidDropboxEntry) ConcernsPackage(pkg string) bool {
	if pkg == "" {
		return false
	}
	return e.Process == pkg || e.Package == pkg ||
		strings.HasPrefix(e.Process, pkg+":") // ":remote" and friends
}
