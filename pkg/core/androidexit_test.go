package core

import "testing"

// Verbatim from an emulator on API 34 — a force-stop and a real crash. The
// crash line is the important one: its reason text nests parentheses, which a
// naive match truncates.
const realExitInfo = `ACTIVITY MANAGER PROCESS EXIT INFO (dumpsys activity exit-info)
Last Timestamp of Persistence Into Persistent Storage: 2026-08-19 13:53:29.794
  package: com.testhiveapp
    Historical Process Exit for uid=10196
        ApplicationExitInfo #0:
          timestamp=2026-08-19 14:02:11.101 pid=8215 realUid=10196 packageUid=10196 definingUid=10196 user=0
          process=com.testhiveapp reason=4 (APP CRASH(EXCEPTION)) subreason=0 (UNKNOWN) status=0
          importance=100 pss=0.00 rss=0.00 description=crash state=empty trace=null
        ApplicationExitInfo #1:
          timestamp=2026-08-19 13:53:32.876 pid=7434 realUid=10196 packageUid=10196 definingUid=10196 user=0
          process=com.testhiveapp reason=10 (USER REQUESTED) subreason=21 (FORCE STOP) status=0
          importance=100 pss=58MB rss=147MB description=stop com.testhiveapp due to from pid 8194 state=empty trace=null`

func TestParseAndroidExitInfo_RealOutput(t *testing.T) {
	got := ParseAndroidExitInfo(realExitInfo)
	if len(got) != 2 {
		t.Fatalf("parsed %d entries, want 2", len(got))
	}

	crash := got[0]
	if crash.Reason != "APP CRASH(EXCEPTION)" {
		t.Errorf("Reason = %q, want the full nested text %q", crash.Reason, "APP CRASH(EXCEPTION)")
	}
	if !crash.IsCrash() || !crash.Noteworthy() {
		t.Error("an APP CRASH must be reported as a crash")
	}
	if crash.PID != 8215 || crash.Process != "com.testhiveapp" {
		t.Errorf("pid/process = %d / %q", crash.PID, crash.Process)
	}
	if crash.Description != "crash" {
		t.Errorf("Description = %q", crash.Description)
	}
	if crash.Timestamp.IsZero() {
		t.Error("timestamp should parse — it is what scopes a death to a flow")
	}

	stop := got[1]
	if stop.Reason != "USER REQUESTED" || stop.Subreason != "FORCE STOP" {
		t.Errorf("reason/subreason = %q / %q", stop.Reason, stop.Subreason)
	}
	if stop.Noteworthy() {
		t.Error("a force-stop is the runner's own doing and must not be reported")
	}
	// Units differ between entries (0.00 vs 58MB), which is why these stay strings.
	if stop.PSS != "58MB" || stop.RSS != "147MB" {
		t.Errorf("pss/rss = %q / %q", stop.PSS, stop.RSS)
	}
}

func TestAndroidExitInfo_Classification(t *testing.T) {
	for name, tc := range map[string]struct {
		reason                      string
		crash, anr, lowmem, notable bool
	}{
		"app crash":    {"APP CRASH(EXCEPTION)", true, false, false, true},
		"native crash": {"APP CRASH(NATIVE)", true, false, false, true},
		"anr":          {"ANR", false, true, false, true},
		"low memory":   {"LOW MEMORY", false, false, true, true},
		"user stop":    {"USER REQUESTED", false, false, false, false},
		"exit self":    {"EXIT SELF", false, false, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			e := AndroidExitInfo{Reason: tc.reason}
			if e.IsCrash() != tc.crash || e.IsANR() != tc.anr ||
				e.IsLowMemory() != tc.lowmem || e.Noteworthy() != tc.notable {
				t.Errorf("%q: crash=%v anr=%v lowmem=%v notable=%v",
					tc.reason, e.IsCrash(), e.IsANR(), e.IsLowMemory(), e.Noteworthy())
			}
		})
	}
}

func TestParseAndroidExitInfo_Empty(t *testing.T) {
	if got := ParseAndroidExitInfo(""); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
	if got := ParseAndroidExitInfo("package: com.example\n  (nothing recorded)"); got != nil {
		t.Errorf("expected nil when no entries are present, got %v", got)
	}
}

func TestAndroidExitInfo_Summary(t *testing.T) {
	lowmem := AndroidExitInfo{Process: "com.example", Reason: "LOW MEMORY", PSS: "412MB", RSS: "500MB"}
	if got := lowmem.Summary(); got != "com.example was killed for memory at pss=412MB rss=500MB" {
		t.Errorf("Summary() = %q", got)
	}
	bare := AndroidExitInfo{Reason: "ANR"}
	if got := bare.Summary(); got != "the app stopped responding (ANR)" {
		t.Errorf("Summary() = %q", got)
	}
}

func TestMostSignificant(t *testing.T) {
	t.Run("a crash outranks a later resource kill", func(t *testing.T) {
		// Observed on a device: `am crash` then the restarted process killed
		// for excessive binder traffic, which lands newest.
		infos := []AndroidExitInfo{
			{Reason: "EXCESSIVE RESOURCE USAGE", Subreason: "EXCESSIVE CPU USAGE"},
			{Reason: "APP CRASH(EXCEPTION)"},
		}
		got, ok := MostSignificant(infos)
		if !ok || !got.IsCrash() {
			t.Errorf("got %+v, want the crash", got)
		}
	})

	t.Run("nothing noteworthy", func(t *testing.T) {
		if _, ok := MostSignificant([]AndroidExitInfo{{Reason: "USER REQUESTED"}}); ok {
			t.Error("a force-stop must not be reported as an explanation")
		}
		if _, ok := MostSignificant(nil); ok {
			t.Error("no entries must yield no explanation")
		}
	})

	t.Run("newest of equal severity wins", func(t *testing.T) {
		infos := []AndroidExitInfo{{Reason: "ANR", Process: "newest"}, {Reason: "ANR", Process: "older"}}
		got, _ := MostSignificant(infos)
		if got.Process != "newest" {
			t.Errorf("got %q, want the newest of equally severe entries", got.Process)
		}
	})

	t.Run("a resource kill is reported when it is all there is", func(t *testing.T) {
		got, ok := MostSignificant([]AndroidExitInfo{{Reason: "EXCESSIVE RESOURCE USAGE", Subreason: "EXCESSIVE CPU USAGE"}})
		if !ok || !got.IsResourceKill() {
			t.Fatalf("got %+v, want the resource kill", got)
		}
		if want := "the app was killed for excessive resource use (EXCESSIVE CPU USAGE)"; got.Summary() != want {
			t.Errorf("Summary() = %q, want %q", got.Summary(), want)
		}
	})
}

func TestAndroidExitInfo_SummaryOmitsEmptyMemoryReadings(t *testing.T) {
	// The platform reports 0.00 when it has no measurement; printing that reads
	// as a broken number rather than as information.
	zeroed := AndroidExitInfo{Process: "com.example", Reason: "APP CRASH(EXCEPTION)", PSS: "0.00", RSS: "0.00"}
	if got := zeroed.Summary(); got != "com.example crashed (APP CRASH(EXCEPTION))" {
		t.Errorf("Summary() = %q, want no pss/rss suffix", got)
	}
	real := AndroidExitInfo{Process: "com.example", Reason: "LOW MEMORY", PSS: "412MB", RSS: "500MB"}
	if got := real.Summary(); got != "com.example was killed for memory at pss=412MB rss=500MB" {
		t.Errorf("Summary() = %q, want the real readings kept", got)
	}
}
