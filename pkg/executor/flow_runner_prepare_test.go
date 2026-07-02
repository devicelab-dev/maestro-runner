package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// countLaunchApps reports how many LaunchAppStep entries appear in the slice
// collectStepsForPrepare returns, and the permissions of the first one (the one
// a FlowAware driver like WDA uses to arm defaultAlertAction).
func countLaunchApps(steps []flow.Step) (int, map[string]string) {
	n := 0
	var firstPerms map[string]string
	for _, s := range steps {
		if la, ok := s.(*flow.LaunchAppStep); ok {
			if n == 0 {
				firstPerms = la.Permissions
			}
			n++
		}
	}
	return n, firstPerms
}

func newPrepareTestRunner(f flow.Flow) *FlowRunner {
	se := NewScriptEngine()
	if f.SourcePath != "" {
		se.SetFlowDir(filepath.Dir(f.SourcePath))
	}
	return &FlowRunner{flow: f, script: se}
}

// TestCollectStepsForPrepare_OnFlowStart is the #108 regression: a launchApp
// (with permissions) declared in onFlowStart must be visible to the pre-session
// scan, not just the main body. Previously only fr.flow.Steps was scanned, so on
// a physical iOS device defaultAlertAction stayed empty and permission dialogs
// weren't auto-accepted.
func TestCollectStepsForPrepare_OnFlowStart(t *testing.T) {
	f := flow.Flow{
		Config: flow.Config{
			OnFlowStart: []flow.Step{
				&flow.LaunchAppStep{AppID: "com.example", Permissions: map[string]string{"all": "allow"}},
			},
		},
		Steps: []flow.Step{
			&flow.TapOnStep{},
		},
	}
	fr := newPrepareTestRunner(f)
	defer fr.script.Close()

	n, perms := countLaunchApps(fr.collectStepsForPrepare())
	if n != 1 {
		t.Fatalf("expected the onFlowStart launchApp to be scanned, got %d launchApps", n)
	}
	if perms["all"] != "allow" {
		t.Errorf("expected permissions {all:allow} from onFlowStart launchApp, got %v", perms)
	}
}

// TestCollectStepsForPrepare_InlineRunFlow covers a launchApp inside an inline
// runFlow (commands:) block.
func TestCollectStepsForPrepare_InlineRunFlow(t *testing.T) {
	f := flow.Flow{
		Config: flow.Config{
			OnFlowStart: []flow.Step{
				&flow.RunFlowStep{
					Steps: []flow.Step{
						&flow.LaunchAppStep{AppID: "com.example", Permissions: map[string]string{"all": "allow"}},
					},
				},
			},
		},
		Steps: []flow.Step{&flow.TapOnStep{}},
	}
	fr := newPrepareTestRunner(f)
	defer fr.script.Close()

	n, perms := countLaunchApps(fr.collectStepsForPrepare())
	if n != 1 || perms["all"] != "allow" {
		t.Fatalf("expected inline-runFlow launchApp scanned with {all:allow}, got n=%d perms=%v", n, perms)
	}
}

// TestCollectStepsForPrepare_FileRunFlow covers a launchApp inside a file-based
// runFlow subflow — the exact shape from the #108 repro (onFlowStart -> runFlow
// file: launchApp with permissions).
func TestCollectStepsForPrepare_FileRunFlow(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "on_start.yaml")
	if err := os.WriteFile(sub, []byte("- launchApp:\n    clearState: true\n    permissions:\n      all: allow\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := flow.Flow{
		SourcePath: filepath.Join(dir, "login.yaml"),
		Config: flow.Config{
			OnFlowStart: []flow.Step{
				&flow.RunFlowStep{File: "on_start.yaml"},
			},
		},
		Steps: []flow.Step{&flow.TapOnStep{}},
	}
	fr := newPrepareTestRunner(f)
	defer fr.script.Close()

	n, perms := countLaunchApps(fr.collectStepsForPrepare())
	if n != 1 {
		t.Fatalf("expected file-based runFlow launchApp scanned, got %d", n)
	}
	if perms["all"] != "allow" {
		t.Errorf("expected {all:allow} from subflow launchApp, got %v", perms)
	}
}

// TestCollectStepsForPrepare_OrderOnFlowStartFirst verifies onFlowStart's
// launchApp precedes the body's, so a "first launchApp wins" driver picks the
// one that launches first at runtime.
func TestCollectStepsForPrepare_OrderOnFlowStartFirst(t *testing.T) {
	f := flow.Flow{
		Config: flow.Config{
			OnFlowStart: []flow.Step{
				&flow.LaunchAppStep{AppID: "com.start", Permissions: map[string]string{"all": "allow"}},
			},
		},
		Steps: []flow.Step{
			&flow.LaunchAppStep{AppID: "com.body", Permissions: map[string]string{"all": "deny"}},
		},
	}
	fr := newPrepareTestRunner(f)
	defer fr.script.Close()

	steps := fr.collectStepsForPrepare()
	n, perms := countLaunchApps(steps)
	if n != 2 {
		t.Fatalf("expected both launchApps in scan slice, got %d", n)
	}
	if perms["all"] != "allow" {
		t.Errorf("first launchApp should be the onFlowStart one ({all:allow}), got %v", perms)
	}
}

// TestCollectStepsForPrepare_NoLaunchApp confirms a flow with no launchApp
// anywhere yields no launchApps (so the WDA monitor stays off, unchanged).
func TestCollectStepsForPrepare_NoLaunchApp(t *testing.T) {
	f := flow.Flow{Steps: []flow.Step{&flow.TapOnStep{}, &flow.BackStep{}}}
	fr := newPrepareTestRunner(f)
	defer fr.script.Close()

	if n, _ := countLaunchApps(fr.collectStepsForPrepare()); n != 0 {
		t.Errorf("expected no launchApps, got %d", n)
	}
}

// TestCollectStepsForPrepare_CyclicRunFlowTerminates ensures a self-referential
// file-based runFlow can't loop forever (depth cap + visited set).
func TestCollectStepsForPrepare_CyclicRunFlowTerminates(t *testing.T) {
	dir := t.TempDir()
	// a.yaml runs b.yaml; b.yaml runs a.yaml.
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("- runFlow: b.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("- runFlow: a.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := flow.Flow{
		SourcePath: filepath.Join(dir, "login.yaml"),
		Config:     flow.Config{OnFlowStart: []flow.Step{&flow.RunFlowStep{File: "a.yaml"}}},
		Steps:      []flow.Step{&flow.TapOnStep{}},
	}
	fr := newPrepareTestRunner(f)
	defer fr.script.Close()

	// Must return (not hang); no launchApp exists in the cycle.
	if n, _ := countLaunchApps(fr.collectStepsForPrepare()); n != 0 {
		t.Errorf("expected 0 launchApps from cyclic subflows, got %d", n)
	}
}
