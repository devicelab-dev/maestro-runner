package devicelab_ios

import (
	"context"
	"testing"
	"time"
)

// A kept agent that answers status but not a snapshot is stuck: a run does
// not re-attach to it (it failed every later RNTester flow on CI).
func TestStartAgentSkipsStuckAgent(t *testing.T) {
	t.Setenv("MAESTRO_RUNNER_HOME", t.TempDir())
	wait := reattachProbeWait
	reattachProbeWait = 200 * time.Millisecond
	t.Cleanup(func() { reattachProbeWait = wait })
	dir := t.TempDir()
	writeManifest(t, dir, "1.0")
	_, port := agentServer(t, func(cmd string, _ Args) Response {
		if cmd == "snapshot" {
			time.Sleep(2 * time.Second)
		}
		return Response{OK: true, Data: &Payload{Message: "ready"}}
	})
	saveState("SIM-STUCK", agentState{Port: port, Version: "abc123"})
	a, _, err := StartAgent(context.Background(), AgentOptions{UDID: "SIM-STUCK", Dir: dir})
	if err == nil && a.Mode() == "reattached" {
		t.Fatal("re-attached to an agent that does not answer a snapshot")
	}
	if s, ok := loadState("SIM-STUCK"); ok && s.Port == port {
		t.Error("the stuck agent's state should be dropped")
	}
}

// A call the agent never answers restarts it, as a dropped connection does.
func TestCallRevivesOnTimeout(t *testing.T) {
	_, stuck := agentServer(t, func(cmd string, _ Args) Response {
		time.Sleep(2 * time.Second)
		return Response{OK: true}
	})
	_, healthy := agentServer(t, func(cmd string, _ Args) Response {
		return Response{OK: true, Data: &Payload{Message: "fresh"}}
	})
	c := NewClient(stuck)
	c.callTimeout = 200 * time.Millisecond
	revived := 0
	c.SetReviver(func(ctx context.Context) (int, error) { revived++; return healthy, nil })
	resp, err := c.Call(context.Background(), "status", nil)
	if revived != 1 {
		t.Fatalf("reviver called %d times, want 1 (err=%v)", revived, err)
	}
	if err != nil || resp.Data == nil || resp.Data.Message != "fresh" {
		t.Errorf("after the restart the call should be re-sent to the new agent: resp=%+v err=%v", resp, err)
	}
}
