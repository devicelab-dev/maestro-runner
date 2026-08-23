package core

import "testing"

// Shaped after a real .ips: header line, newline, payload.
const segfaultIPS = `{"app_name":"TestHive","timestamp":"2026-08-19 11:04:02.00 +0530","app_version":"1.2.0","name":"TestHive","bundleID":"dev.devicelab.testhive","os_version":"iPhone OS 18.6 (22G86)"}
{"procName":"TestHive","captureTime":"2026-08-19 11:04:01.88 +0530","exception":{"type":"EXC_BAD_ACCESS","signal":"SIGSEGV","subtype":"KERN_INVALID_ADDRESS at 0x0"},"termination":{"namespace":"SIGNAL","indicator":"Segmentation fault: 11","reason":"Namespace SIGNAL, Code 11"},"faultingThread":0,"threads":[{"frames":[{"imageIndex":2},{"imageIndex":0}]}],"usedImages":[{"name":"dyld"},{"name":"libswiftCore.dylib"},{"name":"TestHive"}]}`

const jetsamIPS = `{"timestamp":"2026-08-19 09:15:00.00 +0530","name":"TestHive","bundleID":"dev.devicelab.testhive"}
{"exception":{"type":"EXC_RESOURCE"},"termination":{"namespace":"JETSAM","reason":"per-process-limit"},"faultingThread":0,"threads":[],"usedImages":[]}`

func TestParseIPSReport_Segfault(t *testing.T) {
	r, err := ParseIPSReport([]byte(segfaultIPS))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r.BundleID != "dev.devicelab.testhive" {
		t.Errorf("BundleID = %q", r.BundleID)
	}
	if r.AppVersion != "1.2.0" {
		t.Errorf("AppVersion = %q", r.AppVersion)
	}
	if r.Exception != "EXC_BAD_ACCESS" || r.Signal != "SIGSEGV" {
		t.Errorf("exception = %q / %q", r.Exception, r.Signal)
	}
	// The whole point of resolving through usedImages: naming the culprit
	// without a dSYM. Frame 0 of the faulting thread is image 2.
	if r.FaultingBinary != "TestHive" {
		t.Errorf("FaultingBinary = %q, want TestHive", r.FaultingBinary)
	}
	if r.IsJetsam {
		t.Error("a segfault is not a jetsam kill")
	}
	if r.Timestamp.IsZero() {
		t.Error("timestamp should parse — it is what scopes a report to a flow")
	}
	if got := r.Summary(); got == "" {
		t.Error("summary should not be empty")
	}
}

func TestParseIPSReport_JetsamIsDistinguished(t *testing.T) {
	r, err := ParseIPSReport([]byte(jetsamIPS))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.IsJetsam {
		t.Fatal("a JETSAM namespace must be recognised as an OOM kill, not a crash")
	}
	if got := r.Summary(); got != "dev.devicelab.testhive was terminated for memory (jetsam)" {
		t.Errorf("Summary() = %q", got)
	}
}

func TestParseIPSReport_ToleratesPartialReports(t *testing.T) {
	t.Run("header only", func(t *testing.T) {
		r, err := ParseIPSReport([]byte(`{"bundleID":"com.example.app","name":"Example"}`))
		if err != nil {
			t.Fatalf("a header-only report should still parse: %v", err)
		}
		if r.BundleID != "com.example.app" {
			t.Errorf("BundleID = %q", r.BundleID)
		}
	})

	t.Run("unreadable payload keeps the header", func(t *testing.T) {
		r, err := ParseIPSReport([]byte("{\"bundleID\":\"com.example.app\"}\nnot-json-at-all"))
		if err != nil {
			t.Fatalf("a broken payload must not discard the header: %v", err)
		}
		if r.BundleID != "com.example.app" {
			t.Errorf("BundleID = %q", r.BundleID)
		}
	})

	t.Run("unparseable timestamp is not fatal", func(t *testing.T) {
		r, err := ParseIPSReport([]byte(`{"bundleID":"x","timestamp":"whenever"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !r.Timestamp.IsZero() {
			t.Error("expected the zero time for an unreadable timestamp")
		}
	})

	t.Run("a broken header is rejected", func(t *testing.T) {
		if _, err := ParseIPSReport([]byte("not json\n{}")); err == nil {
			t.Error("expected an error when the header cannot be parsed")
		}
	})
}

func TestFaultingBinary_OutOfRangeIndices(t *testing.T) {
	// Malformed reports must not panic — they arrive from a device, not from us.
	cases := map[string]string{
		"image index past usedImages": `{"bundleID":"x"}
{"faultingThread":0,"threads":[{"frames":[{"imageIndex":9}]}],"usedImages":[{"name":"dyld"}]}`,
		"faulting thread past threads": `{"bundleID":"x"}
{"faultingThread":7,"threads":[],"usedImages":[{"name":"dyld"}]}`,
		"no frames": `{"bundleID":"x"}
{"faultingThread":0,"threads":[{"frames":[]}],"usedImages":[{"name":"dyld"}]}`,
		"no faulting thread": `{"bundleID":"x"}
{"threads":[{"frames":[{"imageIndex":0}]}],"usedImages":[{"name":"dyld"}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := ParseIPSReport([]byte(raw))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.FaultingBinary != "" {
				t.Errorf("FaultingBinary = %q, want empty", r.FaultingBinary)
			}
		})
	}
}

func TestSummary_FallsBackAsFieldsThinOut(t *testing.T) {
	for name, tc := range map[string]struct {
		report IOSCrashReport
		want   string
	}{
		"reason wins":      {IOSCrashReport{BundleID: "a", Reason: "watchdog timeout"}, "a crashed: watchdog timeout"},
		"exception+signal": {IOSCrashReport{BundleID: "a", Exception: "EXC_CRASH", Signal: "SIGABRT"}, "a crashed: EXC_CRASH (SIGABRT)"},
		"exception only":   {IOSCrashReport{BundleID: "a", Exception: "EXC_CRASH"}, "a crashed: EXC_CRASH"},
		"process name":     {IOSCrashReport{ProcessName: "Proc"}, "Proc crashed"},
		"nothing at all":   {IOSCrashReport{}, "an app crashed"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := tc.report.Summary(); got != tc.want {
				t.Errorf("Summary() = %q, want %q", got, tc.want)
			}
		})
	}
}
