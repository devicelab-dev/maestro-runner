package devicelab_ios

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// Lost touches on the simulator.
//
// XCTest plays every synthesized gesture through a new virtual touchscreen.
// When the gesture's first event reaches backboardd before backboardd has
// registered that touchscreen, backboardd logs "unknown senderID: 0x…" for
// it, and when the touchscreen goes away with the touch still open it logs
// "cancel -- digitizer did disappear:<…senderID: 0x…; contacts: contacts:
// [pathIndex …". The app then got touchesBegan and touchesCancelled, no tap,
// while XCTest reports the gesture as delivered. The first line alone is a
// near miss (the touch arrived); both for one sender is a lost touch.
//
// The watcher reads those two lines from the simulator's log. A lost touch
// is always logged as a warning. With DL_IOS_LOST_TOUCH=retry a tap or long
// press whose touch was lost is sent once more: the log shows the app never
// got it, so sending it again does what the step asked and nothing more.
// DL_IOS_LOST_TOUCH=off leaves the log alone.

// lostTouchWait bounds how long a gesture that had a near miss waits for the
// cancel line that would make it a lost touch. A gesture without a near miss
// waits for nothing.
const lostTouchWait = 500 * time.Millisecond

// lostTouchPredicate keeps the stream to the two backboardd lines that matter.
const lostTouchPredicate = `process == "backboardd" AND ` +
	`(eventMessage CONTAINS "unknown senderID" OR eventMessage CONTAINS "digitizer did disappear")`

var (
	unknownSenderRe = regexp.MustCompile(`unknown senderID:?\s*0x([0-9A-Fa-f]+)`)
	// A cancel with an open contact: "[pathIndex" follows the sender.
	cancelledTouchRe = regexp.MustCompile(`digitizer did disappear.*?senderID:\s*0x([0-9A-Fa-f]+).*?\[pathIndex`)
)

// touchEvent is what one log line says about a sender.
type touchEvent int

const (
	touchNone touchEvent = iota
	touchUnknownSender
	touchCancelled
)

// parseTouchLine reads one log message; the sender id comes back in lower
// case (BackBoard prints it in upper case, IOKit in lower).
func parseTouchLine(msg string) (touchEvent, string) {
	if m := cancelledTouchRe.FindStringSubmatch(msg); m != nil {
		return touchCancelled, strings.ToLower(m[1])
	}
	if m := unknownSenderRe.FindStringSubmatch(msg); m != nil {
		return touchUnknownSender, strings.ToLower(m[1])
	}
	return touchNone, ""
}

// lostTouchMode is DL_IOS_LOST_TOUCH: "warn" (the default), "retry" or "off".
func lostTouchMode() string {
	switch mode := strings.ToLower(os.Getenv("DL_IOS_LOST_TOUCH")); mode {
	case "off", "0", "false":
		return "off"
	case "retry":
		return mode
	default:
		return "warn"
	}
}

// touchWatch keeps the senders the log has reported, numbered in arrival
// order so a gesture can ask about lines that came after it started.
type touchWatch struct {
	mu        sync.Mutex
	seq       uint64
	unknown   map[string]uint64 // sender -> seq of its "unknown senderID"
	cancelled map[string]uint64 // sender -> seq of its cancel with a contact
	changed   chan struct{}     // closed and replaced on every event
	stop      func()
}

func newTouchWatch() *touchWatch {
	return &touchWatch{
		unknown:   map[string]uint64{},
		cancelled: map[string]uint64{},
		changed:   make(chan struct{}),
		stop:      func() {},
	}
}

// maxTrackedSenders bounds the maps; near misses are rare, so this is only a
// guard for a simulator that logs them on every gesture.
const maxTrackedSenders = 256

// record takes one log message.
func (w *touchWatch) record(msg string) {
	ev, sender := parseTouchLine(msg)
	if ev == touchNone {
		return
	}
	w.mu.Lock()
	if len(w.unknown) > maxTrackedSenders || len(w.cancelled) > maxTrackedSenders {
		w.unknown, w.cancelled = map[string]uint64{}, map[string]uint64{}
	}
	w.seq++
	lost := false
	switch ev {
	case touchUnknownSender:
		w.unknown[sender] = w.seq
		_, lost = w.cancelled[sender]
	case touchCancelled:
		w.cancelled[sender] = w.seq
		_, lost = w.unknown[sender]
	}
	close(w.changed)
	w.changed = make(chan struct{})
	w.mu.Unlock()
	if lost {
		logger.Warn("[devicelab-ios] the simulator cancelled a synthesized touch (backboardd sender 0x%s was not registered yet): the app got no tap", sender)
	} else {
		logger.Debug("[devicelab-ios] backboardd: %s", oneLine(msg))
	}
}

// mark is where a gesture starts in the event order.
func (w *touchWatch) mark() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.seq
}

// lostSince reports a sender whose touch was lost after mark. It returns at
// once when no near miss came after mark; otherwise it waits, up to wait,
// for the near miss's cancel.
func (w *touchWatch) lostSince(mark uint64, wait time.Duration) (string, bool) {
	deadline := time.Now().Add(wait)
	for {
		w.mu.Lock()
		nearMiss := false
		for sender, at := range w.unknown {
			if at <= mark {
				continue
			}
			nearMiss = true
			if c, ok := w.cancelled[sender]; ok && c > mark {
				w.mu.Unlock()
				return sender, true
			}
		}
		changed := w.changed
		w.mu.Unlock()
		left := time.Until(deadline)
		if !nearMiss || left <= 0 {
			return "", false
		}
		select {
		case <-changed:
		case <-time.After(left):
		}
	}
}

// read feeds the watch from a log stream in ndjson (one JSON object a line,
// the message in eventMessage); a line that is not JSON is read as the
// message itself.
func (w *touchWatch) read(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		var entry struct {
			EventMessage string `json:"eventMessage"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil && entry.EventMessage != "" {
			line = entry.EventMessage
		}
		w.record(line)
	}
}

// startTouchWatch streams the simulator's backboardd log into a new watch.
func startTouchWatch(udid string) (*touchWatch, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "xcrun", "simctl", "spawn", udid, "log", "stream",
		"--style", "ndjson", "--predicate", lostTouchPredicate)
	setProcessGroup(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	w := newTouchWatch()
	done := make(chan struct{})
	go func() {
		w.read(out)
		_ = cmd.Wait()
		close(done)
	}()
	w.stop = func() {
		killProcessGroup(cmd)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	return w, nil
}

// WatchLostTouches starts reading the simulator's log for touches XCTest
// reported as delivered but the simulator cancelled (see touchwatch.go),
// unless DL_IOS_LOST_TOUCH=off. Simulators only; Close stops it.
func (d *Driver) WatchLostTouches() {
	if d.realDevice || d.udid == "" {
		return
	}
	mode := lostTouchMode()
	if mode == "off" {
		return
	}
	w, err := startTouchWatch(d.udid)
	if err != nil {
		logger.Debug("[devicelab-ios] lost-touch watch not started: %v", err)
		return
	}
	d.mu.Lock()
	d.touch, d.retryLostTouch = w, mode == "retry"
	d.mu.Unlock()
}

// stopTouchWatch ends the log stream, if one runs.
func (d *Driver) stopTouchWatch() {
	d.mu.Lock()
	w := d.touch
	d.touch = nil
	d.mu.Unlock()
	if w != nil {
		w.stop()
	}
}

// touchWatch returns the running watch, or nil.
func (d *Driver) touchWatch() *touchWatch {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.touch
}

// redeliverable reports whether a gesture whose touch never completed can be
// sent again as it was: one press at one point. A double tap or a swipe may
// have been partly delivered.
func redeliverable(kind string) bool {
	return kind == "tap" || kind == "longPress"
}
