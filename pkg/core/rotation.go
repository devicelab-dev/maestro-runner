package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var currentOrientationRe = regexp.MustCompile(`mCurrentOrientation=(\d)`)

// WaitForDisplayRotation polls `dumpsys display` until the display reports the
// requested rotation (0-3), or timeout passes.
//
// Writing `user_rotation` returns before the display has turned, and the
// window manager reads the accessibility tree mid-rotation without complaint —
// a hierarchy fetched a few hundred milliseconds after setOrientation can
// belong to the old orientation, with bounds that no longer exist on screen.
// Polling the display's own view of the rotation is how agent-device settles
// this; the field is `mCurrentOrientation` in `dumpsys display`.
//
// A device whose dumpsys does not carry the field cannot be observed and is
// not waited on. A device that never reports the rotation is not treated as a
// failure either: an app that locks itself to portrait ignores user_rotation
// entirely, and setOrientation has always succeeded silently there. The error
// says what the display reported so the caller can mention it.
func WaitForDisplayRotation(shell func(string) (string, error), want int, timeout, interval time.Duration) error {
	observed, err := waitForRotation(shell, func(r int) bool { return r == want }, timeout, interval)
	if err != nil || observed < 0 || observed == want {
		return err
	}
	return fmt.Errorf("display still reports rotation %d, wanted %d", observed, want)
}

// WaitForDisplayOrientation polls `dumpsys display` until the display is
// landscape (wider than tall) or portrait, as asked, and returns the rotation
// it reports, or -1 when the device does not report one. natW and natH are
// the display's natural size (`wm size`): a tablet's rotation 0 is landscape.
// Not reaching the orientation is not an error, for the reason
// WaitForDisplayRotation gives; the rotation returned is then the last seen.
func WaitForDisplayOrientation(shell func(string) (string, error), landscape bool, natW, natH int, timeout, interval time.Duration) (int, error) {
	return waitForRotation(shell, func(r int) bool {
		w, h := RotateSize(natW, natH, r)
		return (w > h) == landscape
	}, timeout, interval)
}

// DisplayRotation is the rotation (0-3) `dumpsys display` reports, or -1
// when the device does not report one.
func DisplayRotation(shell func(string) (string, error)) int {
	r, _ := waitForRotation(shell, func(int) bool { return true }, 0, 0)
	return r
}

func waitForRotation(shell func(string) (string, error), done func(int) bool, timeout, interval time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	for {
		out, err := shell("dumpsys display")
		if err != nil {
			return -1, fmt.Errorf("dumpsys display: %w", err)
		}
		m := currentOrientationRe.FindStringSubmatch(out)
		if m == nil {
			return -1, nil // unobservable on this device; do not hold the flow up
		}
		observed, _ := strconv.Atoi(m[1])
		if done(observed) || time.Now().After(deadline) {
			return observed, nil
		}
		time.Sleep(interval)
	}
}

// RotateSize is the screen size at rotation (0-3) of a display whose natural
// size is w x h: a quarter turn swaps width and height.
func RotateSize(w, h, rotation int) (int, int) {
	if rotation%2 == 1 {
		return h, w
	}
	return w, h
}

// ParseWmSize reads the display's natural size from `wm size` output: the
// override size when one is set, else the physical size. It does not change
// with rotation.
func ParseWmSize(out string) (int, int) {
	var pw, ph, ow, oh int
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Physical size:"):
			pw, ph = parseWxH(strings.TrimPrefix(line, "Physical size:"))
		case strings.HasPrefix(line, "Override size:"):
			ow, oh = parseWxH(strings.TrimPrefix(line, "Override size:"))
		}
	}
	if ow > 0 && oh > 0 {
		return ow, oh
	}
	return pw, ph
}

func parseWxH(s string) (int, int) {
	w, h, ok := strings.Cut(strings.TrimSpace(s), "x")
	if !ok {
		return 0, 0
	}
	wi, err1 := strconv.Atoi(strings.TrimSpace(w))
	hi, err2 := strconv.Atoi(strings.TrimSpace(h))
	if err1 != nil || err2 != nil || wi <= 0 || hi <= 0 {
		return 0, 0
	}
	return wi, hi
}
