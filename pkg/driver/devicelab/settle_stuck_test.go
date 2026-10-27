package devicelab

import (
	"errors"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
)

// stuckClient scripts settles and the screen's content hash.
type stuckClient struct {
	*richClient
	settled  []bool // per settle call, the last repeats
	settleEr error
	hash     uint64
	hashes   int
	calls    []int // timeoutMs per settle
}

func (c *stuckClient) WaitForSettle(timeoutMs, quietMs int) (bool, error) {
	c.calls = append(c.calls, timeoutMs)
	if c.settleEr != nil {
		return false, c.settleEr
	}
	i := len(c.calls) - 1
	if i >= len(c.settled) {
		i = len(c.settled) - 1
	}
	return c.settled[i], nil
}

func (c *stuckClient) TreeHash() (uint64, error) {
	c.hashes++
	return c.hash, nil
}

func newStuckDriver(settled ...bool) (*Driver, *stuckClient) {
	client := &stuckClient{richClient: &richClient{trackingClient: newTrackingClient()}, settled: settled, hash: 42}
	return New(client, &core.PlatformInfo{}, &mockShell{}), client
}

func wantCaps(t *testing.T, got []int, want ...int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("settle caps %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("settle caps %v, want %v", got, want)
		}
	}
}

// A screen that never settles pays the full cap once; later settles on the
// same screen use the short cap.
func TestSettleShortensOnAScreenThatNeverSettles(t *testing.T) {
	d, client := newStuckDriver(false)
	short := int(stuckSettleCap.Milliseconds())

	for i := 0; i < 3; i++ {
		d.settle(settleAfterTapTimeoutMs, "tap")
	}

	wantCaps(t, client.calls, settleAfterTapTimeoutMs, short, short)
}

// New content (a different screen, or new text) gets the full cap again.
func TestSettleFullCapAfterScreenChanges(t *testing.T) {
	d, client := newStuckDriver(false)
	short := int(stuckSettleCap.Milliseconds())

	d.settle(settleAfterTapTimeoutMs, "tap")
	d.settle(settleAfterTapTimeoutMs, "tap")
	client.hash = 43
	d.settle(settleAfterTapTimeoutMs, "tap")

	wantCaps(t, client.calls, settleAfterTapTimeoutMs, short, settleAfterTapTimeoutMs)
}

// A screen that settles on the short cap is forgotten: the next settle on it
// gets the full cap, so an animation that ended is not cut short later.
func TestSettleFullCapAfterScreenSettles(t *testing.T) {
	d, client := newStuckDriver(false, true, true)
	short := int(stuckSettleCap.Milliseconds())

	d.settle(settleAfterTapTimeoutMs, "tap")
	d.settle(settleAfterTapTimeoutMs, "tap")
	d.settle(settleAfterTapTimeoutMs, "tap")

	wantCaps(t, client.calls, settleAfterTapTimeoutMs, short, settleAfterTapTimeoutMs)
}

// Settles that finish read no hash: a run without a stuck screen makes no
// extra calls.
func TestSettleReadsNoHashWhenScreensSettle(t *testing.T) {
	d, client := newStuckDriver(true)

	d.settle(settleAfterTapTimeoutMs, "tap")
	d.settle(settleAfterTapTimeoutMs, "swipe")

	if client.hashes != 0 {
		t.Errorf("read the hash %d times, want 0", client.hashes)
	}
}

// An agent that cannot say whether the screen settled is not taken for a
// stuck screen.
func TestSettleErrorIsNotAStuckScreen(t *testing.T) {
	d, client := newStuckDriver(false)
	client.settleEr = errors.New("agent gone")

	d.settle(settleAfterTapTimeoutMs, "tap")
	d.settle(settleAfterTapTimeoutMs, "tap")

	wantCaps(t, client.calls, settleAfterTapTimeoutMs, settleAfterTapTimeoutMs)
}

// An unreadable screen (hash 0: no root) is never recorded.
func TestSettleZeroHashIsNotRecorded(t *testing.T) {
	d, client := newStuckDriver(false)
	client.hash = 0

	d.settle(settleAfterTapTimeoutMs, "tap")
	d.settle(settleAfterTapTimeoutMs, "tap")

	wantCaps(t, client.calls, settleAfterTapTimeoutMs, settleAfterTapTimeoutMs)
}

// Page loads (openLink, Enter) keep their full cap even on a stuck screen.
func TestPageSettleKeepsFullCap(t *testing.T) {
	d, client := newStuckDriver(false)

	d.settle(settleAfterTapTimeoutMs, "tap")
	d.settlePage(openLinkSettleTimeoutMs, "openLink")

	wantCaps(t, client.calls, settleAfterTapTimeoutMs, openLinkSettleTimeoutMs)
}
