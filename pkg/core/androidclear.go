package core

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Clearing an app's data (`pm clear`, or the ActivityManager call behind it)
// force-stops the app and removes its task, and removing a task kills every
// process the app has. A launch issued before that kill starts a process that
// is killed with them, so the flow waits on a blank screen:
//
//	I ActivityTaskManager: Destroy timeout of remove-task, attempt to kill Task{… A=10192:com.example}
//	I ActivityManager: Killing 11309:com.example/u0a192 (adj -10000): remove task
//
// The kill comes late in two ways, so launchApp: clearState, which launches
// right after the clear, can land before it:
//
//   - The app's screens are showing. Its activities are still closing when the
//     task is removed, so Android waits for them and kills the processes when
//     that wait times out, about a second later. Nothing in dumpsys shows the
//     pending timeout, so it can't be waited out after the clear. It can be
//     avoided: AndroidStopBeforeClear stops the app first and waits until its
//     activities and windows are gone, which leaves nothing to wait for. A
//     force-stop alone isn't enough; the clear must come after the windows
//     have closed.
//   - Another app's activity sits in the app's task, such as the permission
//     dialog. The force-stop leaves that task in place, and the clear removes
//     it, and kills, only when it has finished clearing the data, after it has
//     returned. AndroidWaitForTaskRemoved waits for the task to go.
//
// Both poll dumpsys over the shell, one call per poll. Each check ends with
// `; true`: grep -c exits 1 when it counts 0, and adb shell passes that on as a
// failure. Package names hold only letters, digits, '_' and '.', so the quoted
// grep patterns can't be broken out of; the package is escaped for grep's regex.
//
// Known limits. The task is found by its affinity, A=<uid>:<package>: an app
// that sets another taskAffinity isn't waited for after the clear, and is no
// worse off than before. Another user's copy of the app (a work profile) is
// seen too, and costs a timeout and a warning.

// Timeouts and interval for the clear waits. Both waits usually end within a
// poll or two; the timeouts are for an app that never lets go (a launcher, a
// kiosk app), which is cleared and launched anyway.
const (
	StopBeforeClearTimeout = 3 * time.Second
	TaskRemovedTimeout     = 2 * time.Second
	ClearPollInterval      = 50 * time.Millisecond
)

// ErrClearWaitTimedOut marks a clear wait that gave up with the app's screens
// or task still there, as opposed to a device that couldn't be read.
var ErrClearWaitTimedOut = errors.New("timed out")

// AndroidStopBeforeClear force-stops appID and waits, up to timeout, until
// none of its activities or windows remain, so that a clear of its data right
// after doesn't make Android kill the next launch. It returns an error naming
// what was still showing when the timeout passed, or when the device can't
// be read; the caller clears the app either way.
func AndroidStopBeforeClear(sh shellRunner, appID string, timeout, interval time.Duration) error {
	if _, err := sh.Shell("am force-stop " + ShellQuote(appID)); err != nil {
		return fmt.Errorf("am force-stop: %w", err)
	}
	pkg := grepPackage(appID)
	check := "dumpsys activity activities | grep -cE 'Hist .* u[0-9]+ " + pkg + "/'; " +
		"dumpsys window windows | grep -cE '^  Window #[0-9]+ Window\\{[^ ]+ u[0-9]+ " + pkg + "/'; true"
	deadline := time.Now().Add(timeout)
	for {
		counts, err := shellCounts(sh, check, 2)
		if err != nil {
			return err
		}
		if counts[0] == 0 && counts[1] == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %s still has %d activities and %d windows after %s", ErrClearWaitTimedOut, appID, counts[0], counts[1], timeout)
		}
		time.Sleep(interval)
	}
}

// AndroidWaitForTaskRemoved waits, up to timeout, until no task of appID
// remains, so that a launch after a clear isn't killed when the clear removes
// the task. It returns an error when the task is still there after timeout, or
// when the device can't be read.
func AndroidWaitForTaskRemoved(sh shellRunner, appID string, timeout, interval time.Duration) error {
	check := "dumpsys activity activities | grep -cE '^ *\\* Task\\{[^ ]+ #[0-9]+ type=[a-z]+ A=[0-9]+:" + grepPackage(appID) + "( |\\}|$)'; true"
	deadline := time.Now().Add(timeout)
	for {
		counts, err := shellCounts(sh, check, 1)
		if err != nil {
			return err
		}
		if counts[0] == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %s's task is still there after %s", ErrClearWaitTimedOut, appID, timeout)
		}
		time.Sleep(interval)
	}
}

// grepPackage escapes a package name for a grep -E pattern.
func grepPackage(appID string) string {
	return regexp.QuoteMeta(appID)
}

// shellCounts runs cmd and reads want counts from its output, one per line, as
// `grep -c` prints them.
func shellCounts(sh shellRunner, cmd string, want int) ([]int, error) {
	out, err := sh.Shell(cmd)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(out)
	if len(fields) != want {
		return nil, fmt.Errorf("unexpected dumpsys check output %q", out)
	}
	counts := make([]int, want)
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("unexpected dumpsys check output %q", out)
		}
		counts[i] = n
	}
	return counts, nil
}
