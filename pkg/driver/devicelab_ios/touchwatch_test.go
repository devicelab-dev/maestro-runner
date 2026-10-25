package devicelab_ios

import (
	"strings"
	"testing"
	"time"
)

// Lines in the shape backboardd logs them (as reported for iOS 26.2 simulators).
const (
	logUnknownSender = `unknown senderID: 0x8000000817319A1C`
	logCancelOpen    = `cancel -- digitizer did disappear:<BKDirectTouchState: 0x600001234; senderID: 0x8000000817319A1C; contacts: contacts: [pathIndex 1 ...]>`
	logCancelEmpty   = `cancel -- digitizer did disappear:<BKDirectTouchState: 0x600001234; senderID: 0x8000000817319A1C; contacts: contacts: []>`
)

func TestParseTouchLine(t *testing.T) {
	for _, tc := range []struct {
		line   string
		want   touchEvent
		sender string
	}{
		{logUnknownSender, touchUnknownSender, "8000000817319a1c"},
		{logCancelOpen, touchCancelled, "8000000817319a1c"},
		{logCancelEmpty, touchNone, ""}, // no open contact: nothing was lost
		{"Service added: IOHIDService transport:Recap", touchNone, ""},
	} {
		got, sender := parseTouchLine(tc.line)
		if got != tc.want || sender != tc.sender {
			t.Errorf("%q: got (%v,%q), want (%v,%q)", tc.line, got, sender, tc.want, tc.sender)
		}
	}
}

// A near miss alone (the touch arrived late but arrived) is not a lost touch;
// a near miss and a cancel with an open contact for the same sender is.
func TestLostSince(t *testing.T) {
	w := newTouchWatch()
	mark := w.mark()
	start := time.Now()
	if _, lost := w.lostSince(mark, time.Second); lost || time.Since(start) > 100*time.Millisecond {
		t.Fatalf("no near miss: lost=%v after %v, want an immediate no", lost, time.Since(start))
	}

	w.record(logUnknownSender)
	start = time.Now()
	if _, lost := w.lostSince(mark, 150*time.Millisecond); lost {
		t.Fatal("a near miss alone counted as lost")
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatal("a near miss did not wait for its cancel")
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		w.record(logCancelOpen)
	}()
	sender, lost := w.lostSince(mark, time.Second)
	if !lost || sender != "8000000817319a1c" {
		t.Fatalf("near miss + cancel: got (%q,%v), want the sender lost", sender, lost)
	}

	// Lines before the mark belong to an earlier gesture.
	if _, lost := w.lostSince(w.mark(), 50*time.Millisecond); lost {
		t.Fatal("an earlier gesture's lost touch was blamed on a later one")
	}
}

func TestTouchWatchReadsNDJSON(t *testing.T) {
	w := newTouchWatch()
	mark := w.mark()
	w.read(strings.NewReader(
		`{"eventMessage":"` + logUnknownSender + `","processImagePath":"/usr/libexec/backboardd"}` + "\n" +
			"Filtering the log data using \"process == backboardd\"\n" +
			`{"eventMessage":"` + logCancelOpen + `"}` + "\n"))
	if _, lost := w.lostSince(mark, 0); !lost {
		t.Fatal("lost touch in an ndjson stream not seen")
	}
}

func TestLostTouchMode(t *testing.T) {
	for env, want := range map[string]string{"": "warn", "warn": "warn", "retry": "retry", "RETRY": "retry", "off": "off", "0": "off"} {
		t.Setenv("DL_IOS_LOST_TOUCH", env)
		if got := lostTouchMode(); got != want {
			t.Errorf("DL_IOS_LOST_TOUCH=%q: got %q, want %q", env, got, want)
		}
	}
}

// actWithLog runs one tap on a driver whose simulator log reports, while the
// tap is played, the given lines for the first act only, and returns how many
// acts reached the agent.
func actWithLog(t *testing.T, kind string, retry bool, lines ...string) int {
	t.Helper()
	w := newTouchWatch()
	first := true
	d, fa, _ := newTestDriver(t, func(cmd string, _ Args) (*Response, error) {
		if cmd == "act" && first {
			first = false
			for _, l := range lines {
				w.record(l)
			}
		}
		return ok(&Payload{}), nil
	})
	d.touch, d.retryLostTouch = w, retry
	if err := d.act(kind, 10, 20, nil); err != nil {
		t.Fatal(err)
	}
	return len(fa.sent("act"))
}

func TestActRedeliversALostTap(t *testing.T) {
	if n := actWithLog(t, "tap", true, logUnknownSender, logCancelOpen); n != 2 {
		t.Errorf("lost tap with retry: %d acts, want 2", n)
	}
	if n := actWithLog(t, "longPress", true, logUnknownSender, logCancelOpen); n != 2 {
		t.Errorf("lost long press with retry: %d acts, want 2", n)
	}
	if n := actWithLog(t, "tap", true); n != 1 {
		t.Errorf("clean tap: %d acts, want 1", n)
	}
	if n := actWithLog(t, "tap", true, logUnknownSender, logCancelEmpty); n != 1 {
		t.Errorf("near miss: %d acts, want 1", n)
	}
	if n := actWithLog(t, "tap", false, logUnknownSender, logCancelOpen); n != 1 {
		t.Errorf("lost tap without retry: %d acts, want 1 (warning only)", n)
	}
	if n := actWithLog(t, "doubleTap", true, logUnknownSender, logCancelOpen); n != 1 {
		t.Errorf("lost double tap: %d acts, want 1 (may be half delivered)", n)
	}
}
