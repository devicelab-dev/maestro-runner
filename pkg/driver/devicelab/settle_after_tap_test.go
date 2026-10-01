package devicelab

import (
	"errors"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// settleCountingClient counts WaitForSettle calls.
type settleCountingClient struct {
	*richClient
	settles int
}

func (c *settleCountingClient) WaitForSettle(timeoutMs, quietMs int) (bool, error) {
	c.settles++
	return true, nil
}

// FindAndClickChecked finds nothing, so a tap fails fast; the settle tests
// only count settles.
func (c *settleCountingClient) FindAndClickChecked(strategy, selector string, screenW, screenH int, hitTest bool) (*uiautomator2.Element, bool, string, error) {
	return nil, false, "", errors.New("not found")
}

// A key press right after a tap waits for the UI to settle first; one that
// does not follow a tap goes out at once.
func TestKeyPressSettlesOnlyAfterTap(t *testing.T) {
	client := &settleCountingClient{richClient: &richClient{trackingClient: newTrackingClient()}}
	d := New(client, &core.PlatformInfo{}, &mockShell{})
	back := &flow.PressKeyStep{BaseStep: flow.BaseStep{StepType: flow.StepPressKey}, Key: "back"}

	d.Execute(back)
	if client.settles != 0 {
		t.Fatalf("key press with no tap before it settled %d times", client.settles)
	}

	d.lastStepWasTap = true
	d.Execute(back)
	if client.settles != 1 {
		t.Fatalf("key press after a tap settled %d times, want 1", client.settles)
	}

	// The key press itself is not a tap, so the next one does not settle.
	d.Execute(&flow.BackStep{BaseStep: flow.BaseStep{StepType: flow.StepBack}})
	if client.settles != 1 {
		t.Fatalf("back after a key press settled again: %d", client.settles)
	}
}

// openLink waits for the app to settle, so the next step reads the page the
// link opened rather than the one it replaced.
func TestOpenLinkSettles(t *testing.T) {
	client := &settleCountingClient{richClient: &richClient{trackingClient: newTrackingClient()}}
	d := New(client, &core.PlatformInfo{}, &mockShell{})

	if res := d.openLink(&flow.OpenLinkStep{Link: "duck://https://duckduckgo.com?q=x"}); !res.Success {
		t.Fatalf("openLink failed: %v", res.Error)
	}
	if client.settles != 1 {
		t.Errorf("openLink settled %d times, want 1", client.settles)
	}
}

// copyTextFrom right after a tap settles first, so it reads what the tap led to.
func TestCopyTextFromSettlesAfterTap(t *testing.T) {
	client := &settleCountingClient{richClient: &richClient{trackingClient: newTrackingClient()}}
	d := New(client, &core.PlatformInfo{}, &mockShell{})
	d.lastStepWasTap = true
	d.Execute(&flow.CopyTextFromStep{BaseStep: flow.BaseStep{TimeoutMs: 1}, Selector: flow.Selector{ID: "omnibarTextInput"}})
	if client.settles != 1 {
		t.Errorf("copyTextFrom after a tap settled %d times, want 1", client.settles)
	}
}

// Enter settles after it is pressed; other keys do not.
func TestEnterSettlesAfterPress(t *testing.T) {
	client := &settleCountingClient{richClient: &richClient{trackingClient: newTrackingClient()}}
	d := New(client, &core.PlatformInfo{}, &mockShell{})

	d.pressKey(&flow.PressKeyStep{Key: "back"})
	if client.settles != 0 {
		t.Fatalf("back settled %d times", client.settles)
	}
	d.pressKey(&flow.PressKeyStep{Key: "Enter"})
	if client.settles != 1 {
		t.Errorf("enter settled %d times, want 1", client.settles)
	}
}

// A tap straight after a tap settles first; a first tap does not.
func TestTapSettlesOnlyAfterTap(t *testing.T) {
	client := &settleCountingClient{richClient: &richClient{trackingClient: newTrackingClient()}}
	d := New(client, &core.PlatformInfo{}, &mockShell{})
	tap := &flow.TapOnStep{BaseStep: flow.BaseStep{StepType: flow.StepTapOn, TimeoutMs: 1}, Selector: flow.Selector{Text: "Menu"}}

	d.Execute(tap)
	if client.settles != 0 {
		t.Fatalf("first tap settled %d times", client.settles)
	}
	d.lastStepWasTap = true
	d.Execute(tap)
	if client.settles != 1 {
		t.Errorf("tap after a tap settled %d times, want 1", client.settles)
	}
}

// Asserts after a tap do not settle: they poll, and an element on both
// screens passes correctly either way. hideKeyboard does not either.
func TestAssertDoesNotSettleAfterTap(t *testing.T) {
	client := &settleCountingClient{richClient: &richClient{trackingClient: newTrackingClient()}}
	d := New(client, &core.PlatformInfo{}, &mockShell{})
	assert := &flow.AssertVisibleStep{BaseStep: flow.BaseStep{StepType: flow.StepAssertVisible, TimeoutMs: 1}, Selector: flow.Selector{Text: "Albums"}}

	d.lastStepWasTap = true
	d.Execute(assert)
	d.lastStepWasTap = true
	d.Execute(&flow.HideKeyboardStep{BaseStep: flow.BaseStep{StepType: flow.StepHideKeyboard}})
	if client.settles != 0 {
		t.Errorf("settled %d times after a tap before a non-action step", client.settles)
	}
}

// The probes before a find's full wait count against its timeout.
func TestRemainingTimeoutMs(t *testing.T) {
	d := New(newTrackingClient(), &core.PlatformInfo{}, &mockShell{})
	start := time.Now().Add(-2 * time.Second)
	if got := d.remainingTimeoutMs(start, true, 7000); got < 4900 || got > 5000 {
		t.Errorf("7s timeout 2s in: %dms left, want ~5000", got)
	}
	if got := d.remainingTimeoutMs(start, true, 1000); got != 1 {
		t.Errorf("1s timeout 2s in: %dms left, want 1", got)
	}
	if got := d.remainingTimeoutMs(start, true, 0); got < OptionalFindTimeout-2100 || got > OptionalFindTimeout-2000 {
		t.Errorf("default optional timeout 2s in: %dms left", got)
	}
}

// The wait after a tap uses the short 200ms quiet window; the wait after a
// page load (Enter, openLink) uses 500ms, since a blank page stays still
// while it downloads: with 200ms duckduckgo's deeplink flow read the address
// bar before the results page replaced it.
func TestSettleWindowsForTapsAndPageLoads(t *testing.T) {
	client := &richClient{trackingClient: newTrackingClient(), settleQuiet: true}
	d := New(client, &core.PlatformInfo{}, &mockShell{})

	d.lastStepWasTap = true
	d.Execute(&flow.PressKeyStep{BaseStep: flow.BaseStep{StepType: flow.StepPressKey}, Key: "back"})
	if len(client.settleCalls) != 1 || client.settleCalls[0] != [2]int{settleAfterTapTimeoutMs, 200} {
		t.Fatalf("after a tap: settle calls %v, want [[%d 200]]", client.settleCalls, settleAfterTapTimeoutMs)
	}

	client.settleCalls = nil
	d.lastStepWasTap = false
	d.Execute(&flow.PressKeyStep{BaseStep: flow.BaseStep{StepType: flow.StepPressKey}, Key: "enter"})
	if len(client.settleCalls) != 1 || client.settleCalls[0] != [2]int{openLinkSettleTimeoutMs, 500} {
		t.Fatalf("after Enter: settle calls %v, want [[%d 500]]", client.settleCalls, openLinkSettleTimeoutMs)
	}

	client.settleCalls = nil
	d.settlePage(openLinkSettleTimeoutMs, "openLink")
	if len(client.settleCalls) != 1 || client.settleCalls[0][1] != 500 {
		t.Fatalf("page settle: calls %v, want a 500ms quiet window", client.settleCalls)
	}

	// waitForIdleTimeout: 0 turns both off.
	client.settleCalls = nil
	d.idleTimeoutSet, d.idleTimeoutMs = true, 0
	d.settle(settleAfterTapTimeoutMs, "tap")
	d.settlePage(openLinkSettleTimeoutMs, "openLink")
	if len(client.settleCalls) != 0 {
		t.Fatalf("waitForIdleTimeout 0: settle calls %v, want none", client.settleCalls)
	}
}

// hashSeqClient returns the tree hashes in seq, one per read, then the last.
type hashSeqClient struct {
	*settleCountingClient
	seq   []uint64
	reads int
}

func (c *hashSeqClient) TreeHash() (uint64, error) {
	i := c.reads
	c.reads++
	if i >= len(c.seq) {
		i = len(c.seq) - 1
	}
	return c.seq[i], nil
}

// A tap waits for the screen to start changing and returns as soon as it
// does; one that changes nothing gives up after tapChangeWait.
func TestTapWaitsForScreenToChange(t *testing.T) {
	base := &settleCountingClient{richClient: &richClient{trackingClient: newTrackingClient()}}
	client := &hashSeqClient{settleCountingClient: base, seq: []uint64{7, 7, 7, 9}}
	d := New(client, &core.PlatformInfo{}, &mockShell{})

	start := time.Now()
	d.waitForTapChange(7)
	if client.reads != 4 {
		t.Errorf("reads = %d, want 4 (stop at the first changed read)", client.reads)
	}
	if took := time.Since(start); took >= tapChangeWait {
		t.Errorf("took %v, want less than tapChangeWait (%v)", took, tapChangeWait)
	}

	client.seq, client.reads = []uint64{7}, 0
	start = time.Now()
	d.waitForTapChange(7)
	if took := time.Since(start); took < tapChangeWait {
		t.Errorf("unchanged screen returned after %v, want tapChangeWait (%v)", took, tapChangeWait)
	}
}

// waitForAnimationToEnd between a tap and a read keeps the tap's settle, so
// copyTextFrom reads after the change has finished.
func TestWaitForAnimationKeepsTapSettle(t *testing.T) {
	client := &settleCountingClient{richClient: &richClient{trackingClient: newTrackingClient()}}
	d := New(client, &core.PlatformInfo{}, &mockShell{})

	d.lastStepWasTap = true
	d.Execute(&flow.WaitForAnimationToEndStep{BaseStep: flow.BaseStep{StepType: flow.StepWaitForAnimationToEnd, TimeoutMs: 50}})
	if !d.lastStepWasTap {
		t.Fatal("waitForAnimationToEnd cleared the pending tap settle")
	}
	d.Execute(&flow.CopyTextFromStep{BaseStep: flow.BaseStep{StepType: flow.StepCopyTextFrom}, Selector: flow.Selector{ID: "label"}})
	if client.settles != 1 {
		t.Errorf("copyTextFrom after tap + wait settled %d times, want 1", client.settles)
	}
}

// openLink right after a tap settles the tap's change first: otherwise that
// change passes for the link taking effect.
func TestOpenLinkSettlesTapFirst(t *testing.T) {
	client := &settleCountingClient{richClient: &richClient{trackingClient: newTrackingClient()}}
	d := New(client, &core.PlatformInfo{}, &mockShell{})
	link := &flow.OpenLinkStep{BaseStep: flow.BaseStep{StepType: flow.StepOpenLink}, Link: "duck://https://duckduckgo.com?q=x"}

	d.Execute(link)
	alone := client.settles
	client.settles = 0

	d.lastStepWasTap = true
	d.Execute(link)
	if client.settles != alone+1 {
		t.Errorf("openLink after a tap settled %d times, want %d (the tap's, then its own)", client.settles, alone+1)
	}
}
