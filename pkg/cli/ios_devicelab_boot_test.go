package cli

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseSimulatorState(t *testing.T) {
	list := []byte(`{"devices":{"com.apple.CoreSimulator.SimRuntime.iOS-26-2":[
		{"udid":"A","state":"Booting"},{"udid":"B","state":"Booted"}]}}`)
	for udid, want := range map[string]string{"A": "Booting", "B": "Booted", "C": ""} {
		if got := parseSimulatorState(list, udid); got != want {
			t.Errorf("%s: got %q, want %q", udid, got, want)
		}
	}
	if got := parseSimulatorState([]byte("not json"), "A"); got != "" {
		t.Errorf("bad json: got %q", got)
	}
}

func TestWaitForSimulatorBoot(t *testing.T) {
	origState, origStatus := simulatorState, simctlBootstatus
	t.Cleanup(func() { simulatorState, simctlBootstatus = origState, origStatus })

	calls := 0
	simctlBootstatus = func(ctx context.Context, udid string) ([]byte, error) {
		calls++
		<-ctx.Done()
		return nil, ctx.Err()
	}

	// Shut down or unknown: no wait.
	for _, state := range []string{"Shutdown", ""} {
		simulatorState = func(string) string { return state }
		if err := waitForSimulatorBoot("X", time.Second); err != nil || calls != 0 {
			t.Fatalf("%q: err=%v calls=%d", state, err, calls)
		}
	}

	// Booting, or listed as Booted while still booting: waits, and a boot
	// that never finishes is an error.
	for i, state := range []string{"Booting", "Booted"} {
		simulatorState = func(string) string { return state }
		err := waitForSimulatorBoot("X", 50*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "did not finish booting") || calls != i+1 {
			t.Fatalf("%s: err=%v calls=%d", state, err, calls)
		}
	}

	// Booting, then booted.
	simctlBootstatus = func(context.Context, string) ([]byte, error) { return []byte("ok"), nil }
	if err := waitForSimulatorBoot("X", time.Second); err != nil {
		t.Fatal(err)
	}
}
