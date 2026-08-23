package devicelab

import (
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Lazy tap-retry is disabled by default (issue #95): its "screen unchanged +
// target still findable → re-tap" heuristic cannot tell a dropped tap from a
// successful-but-async one, so it could re-issue a tap across a navigation
// boundary. These tests lock the default-off contract and the opt-in path.

func TestLazyRetry_DisabledByDefault(t *testing.T) {
	if lazyRetryEnabled {
		t.Skip("MAESTRO_DEVICELAB_LAZY_RETRY is set in this environment")
	}
	d := &Driver{client: &mockDeviceLabClient{}}

	// recordTap is a no-op when disabled — it must not arm the retry state.
	d.recordTap(flow.Selector{Text: "Confirm"})
	if !d.lastTapTime.IsZero() {
		t.Error("recordTap should be a no-op when lazy retry is disabled")
	}

	// Even if the retry state were armed, the gate must short-circuit.
	d.lastTapTime = time.Now()
	d.lastTapSelector = flow.Selector{Text: "Confirm"}
	if d.maybeLazyRetryTap() {
		t.Error("maybeLazyRetryTap must return false when lazy retry is disabled")
	}
}

func TestLazyRetry_ArmsWhenEnabled(t *testing.T) {
	orig := lazyRetryEnabled
	lazyRetryEnabled = true
	defer func() { lazyRetryEnabled = orig }()

	d := &Driver{client: &mockDeviceLabClient{}}
	d.recordTap(flow.Selector{Text: "Confirm"})
	if d.lastTapTime.IsZero() {
		t.Error("recordTap should arm the retry state when lazy retry is enabled")
	}
}
