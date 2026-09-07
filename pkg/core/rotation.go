package core

import (
	"fmt"
	"regexp"
	"strconv"
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
	deadline := time.Now().Add(timeout)
	observed := -1
	for {
		out, err := shell("dumpsys display")
		if err != nil {
			return fmt.Errorf("dumpsys display: %w", err)
		}
		m := currentOrientationRe.FindStringSubmatch(out)
		if m == nil {
			return nil // unobservable on this device; do not hold the flow up
		}
		observed, _ = strconv.Atoi(m[1])
		if observed == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("display still reports rotation %d, wanted %d", observed, want)
		}
		time.Sleep(interval)
	}
}
