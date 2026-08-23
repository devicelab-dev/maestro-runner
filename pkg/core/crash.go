package core

import "strings"

// AndroidCrashSummary scans logcat output (typically the `crash` buffer) for a
// crash or ANR of the given package and returns a short human-readable summary
// plus true when one is found. It recognizes three shapes:
//   - JVM crash: a "FATAL EXCEPTION" block whose "Process: <pkg>" line matches
//   - native crash: a signal line (SIGSEGV/SIGABRT/…) in a dump naming the pkg
//   - ANR: an "ANR in <pkg>" line
//
// Used to turn a post-crash "element not found" into a clear "app crashed"
// failure instead of a misleading generic error.
func AndroidCrashSummary(logcat, pkg string) (string, bool) {
	if pkg == "" || logcat == "" {
		return "", false
	}
	lines := strings.Split(logcat, "\n")

	// ANR — single-line marker.
	for _, ln := range lines {
		if strings.Contains(ln, "ANR in "+pkg) {
			return "ANR (app not responding): " + pkg, true
		}
	}

	// JVM crash — a FATAL EXCEPTION whose Process: line names the package.
	// Scan a small window after the marker for the confirming Process line and
	// the exception cause.
	for i, ln := range lines {
		if !strings.Contains(ln, "FATAL EXCEPTION") {
			continue
		}
		cause := ""
		matched := false
		for j := i; j < len(lines) && j < i+8; j++ {
			if strings.Contains(lines[j], "Process: "+pkg) {
				matched = true
			}
			// First exception/error line after the marker is the cause.
			if cause == "" && (strings.Contains(lines[j], "Exception") || strings.Contains(lines[j], "Error:")) {
				cause = logcatMessage(lines[j])
			}
		}
		if matched {
			if cause == "" {
				cause = "FATAL EXCEPTION"
			}
			return "app crashed: " + cause, true
		}
	}

	// Native crash — a signal line within a dump that names the package.
	nativeSignals := []string{"signal 11 (SIGSEGV)", "signal 6 (SIGABRT)", "signal 7 (SIGBUS)", "signal 4 (SIGILL)"}
	for i, ln := range lines {
		var sig string
		for _, s := range nativeSignals {
			if strings.Contains(ln, s) {
				sig = s
				break
			}
		}
		if sig == "" {
			continue
		}
		// Confirm the surrounding dump names the package (name/process line).
		for j := i - 6; j < len(lines) && j < i+6; j++ {
			if j < 0 {
				continue
			}
			if strings.Contains(lines[j], pkg) {
				return "app crashed (native, " + sig + ")", true
			}
		}
	}

	return "", false
}

// logcatMessage returns the message body of a logcat line — the text after the
// "<tag>: " that follows the "<level> " column — falling back to the trimmed
// line. Kept deliberately simple: it strips the common "AndroidRuntime: "
// prefix so an exception line reads as "java.lang.NullPointerException: boom".
func logcatMessage(line string) string {
	for _, tag := range []string{"AndroidRuntime: ", "E ", "F "} {
		if idx := strings.Index(line, tag); idx >= 0 {
			return strings.TrimSpace(line[idx+len(tag):])
		}
	}
	return strings.TrimSpace(line)
}
