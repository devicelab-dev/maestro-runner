package executor

import (
	"bytes"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// tapOptions holds tap-related execution options extracted from a step.
type tapOptions struct {
	Repeat                int   // Number of times to execute the tap (0 or 1 = once)
	DelayMs               int   // Delay between repeated taps in ms
	RetryTapIfNoChange    *bool // If true, retry tap if hierarchy unchanged
	WaitToSettleTimeoutMs int   // Wait for UI to settle before/after tap (ms)
}

// extractTapOptions extracts tap options from a tap-type step.
// Returns false for non-tap steps.
func extractTapOptions(step flow.Step) (tapOptions, bool) {
	switch s := step.(type) {
	case *flow.TapOnStep:
		return tapOptions{
			Repeat:                s.Repeat,
			DelayMs:               s.DelayMs,
			RetryTapIfNoChange:    s.RetryTapIfNoChange,
			WaitToSettleTimeoutMs: s.WaitToSettleTimeoutMs,
		}, true
	case *flow.DoubleTapOnStep:
		return tapOptions{
			RetryTapIfNoChange:    s.RetryTapIfNoChange,
			WaitToSettleTimeoutMs: s.WaitToSettleTimeoutMs,
		}, true
	case *flow.LongPressOnStep:
		return tapOptions{
			RetryTapIfNoChange:    s.RetryTapIfNoChange,
			WaitToSettleTimeoutMs: s.WaitToSettleTimeoutMs,
		}, true
	default:
		return tapOptions{}, false
	}
}

// hasTapOptions returns true if any non-default options are set.
func (opts tapOptions) hasTapOptions() bool {
	return opts.Repeat > 1 || opts.DelayMs > 0 ||
		opts.RetryTapIfNoChange != nil || opts.WaitToSettleTimeoutMs > 0
}

const (
	defaultRepeatDelay = 100 // ms, matches Maestro's DEFAULT_REPEAT_DELAY
	settleInterval     = 200 * time.Millisecond
)

// settleAfterAction waits for the UI to settle after a UI-mutating action.
// Matches Maestro's behavior of calling waitForAppToSettle() after every action.
// Uses DeviceLab's native event-based settle when available, falls back to hierarchy comparison.
//nolint:unused
func (fr *FlowRunner) settleAfterAction() {
	// Check if driver supports native settle (DeviceLab)
	if settler, ok := core.Unwrap(fr.driver).(interface {
		WaitForSettle(timeoutMs, quietMs int) (bool, error)
	}); ok {
		settled, err := settler.WaitForSettle(2000, 150)
		if err != nil {
			logger.Warn("settleAfterAction: native settle error: %v", err)
		} else if !settled {
			logger.Debug("settleAfterAction: native settle timed out")
		}
		return
	}

	// Fallback: hierarchy comparison (10 polls × 200ms, same as Maestro default)
	fr.waitForSettle(2000)
}

// isTapAction returns true if the step is a tap that may trigger a screen transition.
//nolint:unused
func isTapAction(step flow.Step) bool {
	switch step.(type) {
	case *flow.TapOnStep, *flow.DoubleTapOnStep, *flow.LongPressOnStep, *flow.TapOnPointStep:
		return true
	default:
		return false
	}
}

// needsPreSettle returns true if the step needs the UI to be settled before executing.
// These steps don't call findElement (which has implicit idle wait), so they need
// explicit settle to avoid timing issues after screen transitions.
//nolint:unused
func needsPreSettle(step flow.Step) bool {
	switch step.(type) {
	case *flow.InputTextStep, *flow.InputRandomStep, *flow.EraseTextStep:
		return true
	default:
		return false
	}
}

// waitForSettle polls Hierarchy() until two consecutive snapshots match,
// or the timeout is reached. Returns the final hierarchy snapshot.
// If timeoutMs <= 0, returns the current hierarchy without polling.
func (fr *FlowRunner) waitForSettle(timeoutMs int) []byte {
	hierarchy, err := fr.driver.Hierarchy()
	if err != nil {
		logger.Debug("waitForSettle: Hierarchy() error: %v", err)
		return nil
	}
	if timeoutMs <= 0 {
		return hierarchy
	}

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(settleInterval)
		next, err := fr.driver.Hierarchy()
		if err != nil {
			logger.Debug("waitForSettle: Hierarchy() error: %v", err)
			return hierarchy
		}
		if bytes.Equal(hierarchy, next) {
			return next
		}
		hierarchy = next
	}
	return hierarchy
}

// executeTapWithOptions wraps a tap step with repeat, delay,
// retryTapIfNoChange, and waitToSettleTimeoutMs logic.
//
// Execution order (matching Maestro):
//  1. hierarchyBefore = waitForSettle(waitToSettleTimeoutMs)
//  2. retryLoop (retryTapIfNoChange ? 2 : 1):
//     a. repeatLoop with delay between taps
//     b. hierarchyAfter = waitForSettle(waitToSettleTimeoutMs)
//     c. if hierarchy changed → return
//  3. return last result
func (fr *FlowRunner) executeTapWithOptions(step flow.Step, opts tapOptions) *core.CommandResult {
	if !opts.hasTapOptions() {
		return fr.driver.Execute(step)
	}

	settleTimeout := opts.WaitToSettleTimeoutMs

	// Capture hierarchy before tap (for settle and/or retry comparison)
	var hierarchyBefore []byte
	if settleTimeout > 0 || opts.RetryTapIfNoChange != nil {
		hierarchyBefore = fr.waitForSettle(settleTimeout)
	}

	// Retry count: 2 if retryTapIfNoChange, else 1
	retryCount := 1
	if opts.RetryTapIfNoChange != nil && *opts.RetryTapIfNoChange {
		retryCount = 2
	}

	repeatCount := opts.Repeat
	if repeatCount <= 0 {
		repeatCount = 1
	}

	delayMs := opts.DelayMs
	if delayMs <= 0 && repeatCount > 1 {
		delayMs = defaultRepeatDelay
	}

	var lastResult *core.CommandResult

	for attempt := 0; attempt < retryCount; attempt++ {
		if fr.ctx.Err() != nil {
			return &core.CommandResult{
				Success: false,
				Error:   fr.ctx.Err(),
				Message: "Tap cancelled",
			}
		}

		// Execute tap (possibly repeated)
		for i := 0; i < repeatCount; i++ {
			tapStart := time.Now()
			lastResult = fr.driver.Execute(step)
			if !lastResult.Success {
				return lastResult
			}

			// Delay between repeated taps (not after the last one)
			if repeatCount > 1 && i < repeatCount-1 {
				sleepTime := time.Duration(delayMs)*time.Millisecond - time.Since(tapStart)
				if sleepTime > 0 {
					time.Sleep(sleepTime)
				}
			}
		}

		// Check if UI changed (for retry and settle logic)
		if settleTimeout > 0 || opts.RetryTapIfNoChange != nil {
			hierarchyAfter := fr.waitForSettle(settleTimeout)
			if hierarchyBefore != nil && hierarchyAfter != nil &&
				!bytes.Equal(hierarchyBefore, hierarchyAfter) {
				logger.Debug("Tap caused UI change (attempt %d)", attempt+1)
				return lastResult
			}
			if attempt < retryCount-1 {
				logger.Debug("Tap had no UI change, retrying (attempt %d/%d)", attempt+1, retryCount)
			}
		}
	}

	return lastResult
}
