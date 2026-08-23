package core

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// `dumpsys activity exit-info <pkg>` is the only host-reachable account of why
// a process went away, and it works on unrooted user builds. Crucially it is
// the sole source of LOW MEMORY — an lmkd kill leaves nothing in logcat, so
// without this an app killed for memory looks identical to one that simply
// stopped.
//
// It also reports pss and rss as measured at the moment of death, which is
// strictly better than sampling `dumpsys meminfo` on a timer and hoping to
// catch the peak.
//
// Real output, captured from an emulator on API 34:
//
//	        ApplicationExitInfo #0:
//	          timestamp=2026-08-19 13:53:32.876 pid=7434 realUid=10196 ...
//	          process=com.testhiveapp reason=10 (USER REQUESTED) subreason=21 (FORCE STOP) status=0
//	          importance=100 pss=58MB rss=147MB description=stop ... state=empty trace=null
//
// The integer codes are version-dependent and have been renumbered across
// releases; the parenthesised text has not. Parse the text.

// AndroidExitInfo is one recorded process death.
type AndroidExitInfo struct {
	Timestamp   time.Time
	PID         int
	Process     string
	Reason      string // e.g. "CRASH", "CRASH NATIVE", "ANR", "LOW MEMORY"
	Subreason   string // e.g. "FORCE STOP"
	Status      int
	PSS         string // as reported, e.g. "58MB" — a string because the unit is part of it
	RSS         string
	Description string
}

var (
	exitEntryPattern     = regexp.MustCompile(`ApplicationExitInfo #\d+:`)
	exitTimestampPattern = regexp.MustCompile(`timestamp=(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d+)`)
	exitPIDPattern       = regexp.MustCompile(`\bpid=(\d+)`)
	exitProcessPattern   = regexp.MustCompile(`\bprocess=(\S+)`)
	// Reason text nests parentheses — a real crash reports
	// `reason=4 (APP CRASH(EXCEPTION))` — so match greedily up to the closing
	// paren that precedes the next field rather than the first one seen.
	exitReasonPattern     = regexp.MustCompile(`\breason=\d+ \((.*)\)\s+subreason=`)
	exitReasonTailPattern = regexp.MustCompile(`\breason=\d+ \((.*)\)\s*$`)
	exitSubreasonPattern  = regexp.MustCompile(`\bsubreason=\d+ \((.*?)\)\s+status=`)
	exitStatusPattern     = regexp.MustCompile(`\bstatus=(-?\d+)`)
	exitPSSPattern        = regexp.MustCompile(`\bpss=(\S+)`)
	exitRSSPattern        = regexp.MustCompile(`\brss=(\S+)`)
	exitDescPattern       = regexp.MustCompile(`\bdescription=(.*?)\s+state=`)
)

// ParseAndroidExitInfo reads `dumpsys activity exit-info` output, newest first
// (the order the platform prints it).
func ParseAndroidExitInfo(out string) []AndroidExitInfo {
	locs := exitEntryPattern.FindAllStringIndex(out, -1)
	if len(locs) == 0 {
		return nil
	}

	infos := make([]AndroidExitInfo, 0, len(locs))
	for i, loc := range locs {
		end := len(out)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		block := out[loc[1]:end]

		info := AndroidExitInfo{
			Process:     firstSubmatch(exitProcessPattern, block),
			Reason:      exitReason(block),
			Subreason:   firstSubmatch(exitSubreasonPattern, block),
			PSS:         firstSubmatch(exitPSSPattern, block),
			RSS:         firstSubmatch(exitRSSPattern, block),
			Description: strings.TrimSpace(firstSubmatch(exitDescPattern, block)),
		}
		if ts := firstSubmatch(exitTimestampPattern, block); ts != "" {
			// Device-local time, no zone in the output; compare against other
			// device-local stamps rather than host time.
			if t, err := time.ParseInLocation("2006-01-02 15:04:05.000", ts, time.Local); err == nil {
				info.Timestamp = t
			}
		}
		info.PID, _ = strconv.Atoi(firstSubmatch(exitPIDPattern, block))
		info.Status, _ = strconv.Atoi(firstSubmatch(exitStatusPattern, block))
		infos = append(infos, info)
	}
	return infos
}

// exitReason pulls the parenthesised reason text, falling back to a
// line-anchored match when no subreason follows it.
func exitReason(block string) string {
	if r := firstSubmatch(exitReasonPattern, block); r != "" {
		return r
	}
	for _, line := range strings.Split(block, "\n") {
		if r := firstSubmatch(exitReasonTailPattern, strings.TrimRight(line, " \t\r")); r != "" {
			return r
		}
	}
	return ""
}

func firstSubmatch(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return ""
}

// IsCrash reports whether the process died rather than being asked to stop.
func (e AndroidExitInfo) IsCrash() bool {
	r := strings.ToUpper(e.Reason)
	return strings.Contains(r, "CRASH")
}

// IsANR reports whether the process was killed for not responding.
func (e AndroidExitInfo) IsANR() bool {
	return strings.Contains(strings.ToUpper(e.Reason), "ANR")
}

// IsLowMemory reports whether the process was killed to reclaim memory. This is
// the Android counterpart to an iOS jetsam kill, and the only place it surfaces.
func (e AndroidExitInfo) IsLowMemory() bool {
	r := strings.ToUpper(e.Reason)
	return strings.Contains(r, "LOW MEMORY") || strings.Contains(r, "LMK")
}

// IsResourceKill reports whether the system killed the process for using too
// much of something — CPU, binder traffic, wakelocks. Observed in the wild as
// `reason=9 (EXCESSIVE RESOURCE USAGE)`; it is the app misbehaving, not the
// runner stopping it, so it belongs in a failure message.
func (e AndroidExitInfo) IsResourceKill() bool {
	return strings.Contains(strings.ToUpper(e.Reason), "EXCESSIVE")
}

// Noteworthy reports whether this death is worth failing or annotating a flow
// with. A force-stop, a clean exit, or the user swiping the app away is the
// runner's or the system's normal business and says nothing about the app.
func (e AndroidExitInfo) Noteworthy() bool {
	return e.IsCrash() || e.IsANR() || e.IsLowMemory() || e.IsResourceKill()
}

// severity orders competing explanations. A process can die more than once in a
// flow — crash, restart, killed again — and the newest entry is not necessarily
// the most informative, so the worst one wins rather than the latest.
func (e AndroidExitInfo) severity() int {
	switch {
	case e.IsCrash():
		return 4
	case e.IsANR():
		return 3
	case e.IsLowMemory():
		return 2
	case e.IsResourceKill():
		return 1
	default:
		return 0
	}
}

// MostSignificant returns the entry that best explains why the app went away,
// or false when nothing noteworthy is recorded.
func MostSignificant(infos []AndroidExitInfo) (AndroidExitInfo, bool) {
	var best AndroidExitInfo
	found := false
	for _, info := range infos {
		if !info.Noteworthy() {
			continue
		}
		// Entries arrive newest first, so > keeps the newest of equals.
		if !found || info.severity() > best.severity() {
			best, found = info, true
		}
	}
	return best, found
}

// Summary renders a one-line explanation for a flow failure.
func (e AndroidExitInfo) Summary() string {
	who := e.Process
	if who == "" {
		who = "the app"
	}

	var what string
	switch {
	case e.IsLowMemory():
		what = "was killed for memory"
	case e.IsANR():
		what = "stopped responding (ANR)"
	case e.IsCrash():
		what = "crashed (" + e.Reason + ")"
	case e.IsResourceKill():
		what = "was killed for excessive resource use (" + e.Subreason + ")"
	default:
		what = "exited: " + e.Reason
	}

	// The platform reports 0.00 when it has no measurement — printing that
	// reads as a broken number rather than as information.
	if meaningfulSize(e.PSS) || meaningfulSize(e.RSS) {
		return who + " " + what + " at pss=" + e.PSS + " rss=" + e.RSS
	}
	return who + " " + what
}

// meaningfulSize reports whether a pss/rss reading carries information. The
// platform prints 0.00 when it has no measurement, and units vary between
// entries ("58MB" on one, "0.00" on the next), so this is a string check rather
// than arithmetic.
func meaningfulSize(v string) bool {
	switch strings.TrimSpace(v) {
	case "", "0", "0.00", "0B", "0MB", "0.00MB":
		return false
	}
	return true
}
