package devicelab_ios

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// startStandIn starts a long-lived process in its own group, as
// launchXcodebuild starts xcodebuild, and records it as the agent's.
func startStandIn(t *testing.T, a *Agent) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	logFile, err := os.Create(filepath.Join(t.TempDir(), "xcodebuild.log"))
	if err != nil {
		t.Fatal(err)
	}
	a.xcodeCmd, a.logFile = cmd, logFile
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	return cmd
}

func running(cmd *exec.Cmd) bool {
	return syscall.Kill(cmd.Process.Pid, 0) == nil
}

// An agent started under xcodebuild stays up after the run, for the next
// process to re-attach to, unless DEVICELAB_IOS_AGENT_KEEP=0.
func TestReleaseKeepsXcodebuildAgent(t *testing.T) {
	t.Setenv("MAESTRO_RUNNER_HOME", t.TempDir())
	t.Setenv("DEVICELAB_IOS_AGENT_KEEP", "")
	var watched int
	watchKeptAgent = func(_ string, pid int) { watched = pid }
	t.Cleanup(func() { watchKeptAgent = watchXcodebuild })
	a := &Agent{opts: AgentOptions{UDID: "SIM-K"}}
	cmd := startStandIn(t, a)
	saveState("SIM-K", agentState{Port: 22999, Version: "v1"})

	a.Release(context.Background(), nil)
	time.Sleep(100 * time.Millisecond)
	if !running(cmd) {
		t.Fatal("Release stopped the xcodebuild agent")
	}
	if _, ok := loadState("SIM-K"); !ok {
		t.Fatal("Release dropped the state the next run re-attaches with")
	}
	if watched != cmd.Process.Pid {
		t.Errorf("the kept agent should be watched, got pid %d", watched)
	}
}

// The watcher stops a kept agent once its simulator is not booted (here it
// never was: no simulator has this id).
func TestWatchXcodebuildStopsAgentOfShutDownSimulator(t *testing.T) {
	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("needs xcrun")
	}
	a := &Agent{}
	cmd := startStandIn(t, a)
	watchXcodebuild("00000000-NOT-A-SIMULATOR", cmd.Process.Pid)
	deadline := time.Now().Add(20 * time.Second)
	for running(cmd) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if running(cmd) {
		t.Fatal("the watcher should stop the agent of a simulator that is not booted")
	}
}

func TestReleaseStopsXcodebuildAgentWhenAsked(t *testing.T) {
	t.Setenv("MAESTRO_RUNNER_HOME", t.TempDir())
	t.Setenv("DEVICELAB_IOS_AGENT_KEEP", "0")
	a := &Agent{opts: AgentOptions{UDID: "SIM-S"}}
	cmd := startStandIn(t, a)

	a.Release(context.Background(), nil)
	deadline := time.Now().Add(3 * time.Second)
	for running(cmd) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if running(cmd) {
		t.Fatal("DEVICELAB_IOS_AGENT_KEEP=0 should stop the agent")
	}
}
