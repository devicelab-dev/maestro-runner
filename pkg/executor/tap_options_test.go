package executor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func boolPtr(b bool) *bool { return &b }

// --- extractTapOptions ---

func TestExtractTapOptions_TapOnStep(t *testing.T) {
	step := &flow.TapOnStep{
		Repeat:                3,
		DelayMs:               2000,
		RetryTapIfNoChange:    boolPtr(true),
		WaitToSettleTimeoutMs: 5000,
	}
	opts, ok := extractTapOptions(step)
	if !ok {
		t.Fatal("expected ok=true for TapOnStep")
	}
	if opts.Repeat != 3 {
		t.Errorf("Repeat=%d, want 3", opts.Repeat)
	}
	if opts.DelayMs != 2000 {
		t.Errorf("DelayMs=%d, want 2000", opts.DelayMs)
	}
	if opts.RetryTapIfNoChange == nil || !*opts.RetryTapIfNoChange {
		t.Error("RetryTapIfNoChange should be true")
	}
	if opts.WaitToSettleTimeoutMs != 5000 {
		t.Errorf("WaitToSettleTimeoutMs=%d, want 5000", opts.WaitToSettleTimeoutMs)
	}
}

func TestExtractTapOptions_DoubleTapOnStep(t *testing.T) {
	step := &flow.DoubleTapOnStep{
		RetryTapIfNoChange:    boolPtr(false),
		WaitToSettleTimeoutMs: 1000,
	}
	opts, ok := extractTapOptions(step)
	if !ok {
		t.Fatal("expected ok=true for DoubleTapOnStep")
	}
	if opts.Repeat != 0 {
		t.Errorf("Repeat=%d, want 0", opts.Repeat)
	}
	if opts.DelayMs != 0 {
		t.Errorf("DelayMs=%d, want 0", opts.DelayMs)
	}
}

func TestExtractTapOptions_LongPressOnStep(t *testing.T) {
	step := &flow.LongPressOnStep{
		RetryTapIfNoChange: boolPtr(true),
	}
	opts, ok := extractTapOptions(step)
	if !ok {
		t.Fatal("expected ok=true for LongPressOnStep")
	}
	if opts.RetryTapIfNoChange == nil || !*opts.RetryTapIfNoChange {
		t.Error("RetryTapIfNoChange should be true")
	}
}

func TestExtractTapOptions_NonTapStep(t *testing.T) {
	step := &flow.SwipeStep{}
	_, ok := extractTapOptions(step)
	if ok {
		t.Error("expected ok=false for SwipeStep")
	}
}

// --- hasTapOptions ---

func TestHasTapOptions(t *testing.T) {
	tests := []struct {
		name string
		opts tapOptions
		want bool
	}{
		{"all zero", tapOptions{}, false},
		{"repeat=1", tapOptions{Repeat: 1}, false},
		{"repeat=2", tapOptions{Repeat: 2}, true},
		{"delay only", tapOptions{DelayMs: 100}, true},
		{"retry true", tapOptions{RetryTapIfNoChange: boolPtr(true)}, true},
		{"retry false", tapOptions{RetryTapIfNoChange: boolPtr(false)}, true},
		{"settle timeout", tapOptions{WaitToSettleTimeoutMs: 1000}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.hasTapOptions(); got != tt.want {
				t.Errorf("hasTapOptions()=%v, want %v", got, tt.want)
			}
		})
	}
}

// --- waitForSettle ---

func TestWaitForSettle_NoTimeout(t *testing.T) {
	driver := &mockDriver{
		hierarchyFunc: func() ([]byte, error) {
			return []byte(`<hierarchy>stable</hierarchy>`), nil
		},
	}
	fr := &FlowRunner{driver: driver}
	result := fr.waitForSettle(0)
	if string(result) != `<hierarchy>stable</hierarchy>` {
		t.Errorf("expected current hierarchy, got %q", result)
	}
}

func TestWaitForSettle_SettlesQuickly(t *testing.T) {
	var calls int32
	driver := &mockDriver{
		hierarchyFunc: func() ([]byte, error) {
			n := atomic.AddInt32(&calls, 1)
			if n <= 2 {
				// First two calls return different values (UI still changing)
				return []byte(string(rune('a' + n))), nil
			}
			// Third call onwards returns stable value
			return []byte("stable"), nil
		},
	}
	fr := &FlowRunner{driver: driver}
	result := fr.waitForSettle(3000)
	if string(result) != "stable" {
		t.Errorf("expected 'stable', got %q", result)
	}
}

func TestWaitForSettle_Timeout(t *testing.T) {
	var calls int32
	driver := &mockDriver{
		hierarchyFunc: func() ([]byte, error) {
			n := atomic.AddInt32(&calls, 1)
			// Always return different content
			return []byte(string(rune('a' + n))), nil
		},
	}
	fr := &FlowRunner{driver: driver}
	start := time.Now()
	fr.waitForSettle(500)
	elapsed := time.Since(start)
	if elapsed < 400*time.Millisecond {
		t.Errorf("expected to wait ~500ms, only waited %v", elapsed)
	}
}

// --- executeTapWithOptions ---

func TestExecuteTapWithOptions_NoOptions(t *testing.T) {
	var execCount int
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			return &core.CommandResult{Success: true}
		},
	}
	fr := &FlowRunner{ctx: context.Background(), driver: driver}

	result := fr.executeTapWithOptions(&flow.TapOnStep{}, tapOptions{})
	if !result.Success {
		t.Error("expected success")
	}
	if execCount != 1 {
		t.Errorf("execCount=%d, want 1", execCount)
	}
}

func TestExecuteTapWithOptions_Repeat(t *testing.T) {
	var execCount int
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			return &core.CommandResult{Success: true}
		},
	}
	fr := &FlowRunner{ctx: context.Background(), driver: driver}

	opts := tapOptions{Repeat: 3}
	result := fr.executeTapWithOptions(&flow.TapOnStep{}, opts)
	if !result.Success {
		t.Error("expected success")
	}
	if execCount != 3 {
		t.Errorf("execCount=%d, want 3", execCount)
	}
}

func TestExecuteTapWithOptions_RepeatWithDelay(t *testing.T) {
	var timestamps []time.Time
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			timestamps = append(timestamps, time.Now())
			return &core.CommandResult{Success: true}
		},
	}
	fr := &FlowRunner{ctx: context.Background(), driver: driver}

	opts := tapOptions{Repeat: 3, DelayMs: 300}
	fr.executeTapWithOptions(&flow.TapOnStep{}, opts)

	if len(timestamps) != 3 {
		t.Fatalf("expected 3 taps, got %d", len(timestamps))
	}
	// Check delay between taps (at least 200ms to account for timing variance)
	for i := 1; i < len(timestamps); i++ {
		gap := timestamps[i].Sub(timestamps[i-1])
		if gap < 200*time.Millisecond {
			t.Errorf("gap between tap %d and %d = %v, want >= 200ms", i-1, i, gap)
		}
	}
}

func TestExecuteTapWithOptions_TapFails(t *testing.T) {
	var execCount int
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			return &core.CommandResult{Success: false, Message: "element not found"}
		},
	}
	fr := &FlowRunner{ctx: context.Background(), driver: driver}

	opts := tapOptions{Repeat: 5}
	result := fr.executeTapWithOptions(&flow.TapOnStep{}, opts)
	if result.Success {
		t.Error("expected failure")
	}
	if execCount != 1 {
		t.Errorf("execCount=%d, want 1 (should stop on first failure)", execCount)
	}
}

func TestExecuteTapWithOptions_RetryOnNoChange(t *testing.T) {
	var execCount int
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			return &core.CommandResult{Success: true}
		},
		hierarchyFunc: func() ([]byte, error) {
			// Hierarchy never changes — should trigger retry
			return []byte("unchanged"), nil
		},
	}
	fr := &FlowRunner{ctx: context.Background(), driver: driver}

	opts := tapOptions{RetryTapIfNoChange: boolPtr(true)}
	result := fr.executeTapWithOptions(&flow.TapOnStep{}, opts)
	if !result.Success {
		t.Error("expected success")
	}
	// Should tap twice (retry once)
	if execCount != 2 {
		t.Errorf("execCount=%d, want 2 (initial + 1 retry)", execCount)
	}
}

func TestExecuteTapWithOptions_RetryOnChange(t *testing.T) {
	var execCount int
	var hierarchyCall int32
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			execCount++
			return &core.CommandResult{Success: true}
		},
		hierarchyFunc: func() ([]byte, error) {
			n := atomic.AddInt32(&hierarchyCall, 1)
			if n == 1 {
				return []byte("before"), nil // before tap
			}
			return []byte("after"), nil // after tap — changed
		},
	}
	fr := &FlowRunner{ctx: context.Background(), driver: driver}

	opts := tapOptions{RetryTapIfNoChange: boolPtr(true)}
	result := fr.executeTapWithOptions(&flow.TapOnStep{}, opts)
	if !result.Success {
		t.Error("expected success")
	}
	// Should tap only once — hierarchy changed, no retry needed
	if execCount != 1 {
		t.Errorf("execCount=%d, want 1 (UI changed, no retry)", execCount)
	}
}

func TestExecuteTapWithOptions_WaitToSettle(t *testing.T) {
	var hierarchyCalls int32
	driver := &mockDriver{
		executeFunc: func(step flow.Step) *core.CommandResult {
			return &core.CommandResult{Success: true}
		},
		hierarchyFunc: func() ([]byte, error) {
			atomic.AddInt32(&hierarchyCalls, 1)
			return []byte("stable"), nil
		},
	}
	fr := &FlowRunner{ctx: context.Background(), driver: driver}

	opts := tapOptions{WaitToSettleTimeoutMs: 1000}
	result := fr.executeTapWithOptions(&flow.TapOnStep{}, opts)
	if !result.Success {
		t.Error("expected success")
	}
	// Should call Hierarchy at least twice (before settle + after settle)
	if atomic.LoadInt32(&hierarchyCalls) < 2 {
		t.Errorf("hierarchyCalls=%d, want >= 2", hierarchyCalls)
	}
}

func TestExecuteTapWithOptions_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	driver := &mockDriver{}
	fr := &FlowRunner{ctx: ctx, driver: driver}

	opts := tapOptions{Repeat: 5}
	result := fr.executeTapWithOptions(&flow.TapOnStep{}, opts)
	if result.Success {
		t.Error("expected failure due to context cancellation")
	}
}
