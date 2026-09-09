package core

import (
	"strings"
	"time"
)

// WaitForProcessExit polls `pgrep <name>` over the device shell until no
// process by that name remains, or timeout passes. Returns true when the
// process is gone. Used after signalling screenrecord: the MP4 index is
// written as it exits, and a file read before that is unplayable.
func WaitForProcessExit(shell func(string) (string, error), name string, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		out, err := shell("pgrep " + name)
		if err == nil && strings.TrimSpace(out) == "" {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}
