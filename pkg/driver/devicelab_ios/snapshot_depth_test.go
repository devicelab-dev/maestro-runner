package devicelab_ios

import (
	"os"
	"path/filepath"
	"testing"
)

// #171: above 62 a deep screen comes back empty, so the agent starts at 62
// unless told otherwise.
func TestAgentSnapshotMaxDepth(t *testing.T) {
	t.Setenv(snapshotDepthVar, "")
	t.Setenv("SIMCTL_CHILD_"+snapshotDepthVar, "")
	if got := agentSnapshotMaxDepth(); got != "62" {
		t.Errorf("default = %s, want 62", got)
	}
	t.Setenv("SIMCTL_CHILD_"+snapshotDepthVar, "80")
	if got := agentSnapshotMaxDepth(); got != "80" {
		t.Errorf("SIMCTL_CHILD_ override = %s, want 80", got)
	}
	t.Setenv(snapshotDepthVar, "40")
	if got := agentSnapshotMaxDepth(); got != "40" {
		t.Errorf("override = %s, want 40", got)
	}
	t.Setenv(snapshotDepthVar, "junk")
	t.Setenv("SIMCTL_CHILD_"+snapshotDepthVar, "")
	if got := agentSnapshotMaxDepth(); got != "62" {
		t.Errorf("bad value = %s, want the default 62", got)
	}
}

func TestRenderDeviceXctestrunSetsSnapshotDepth(t *testing.T) {
	t.Setenv(snapshotDepthVar, "")
	t.Setenv("SIMCTL_CHILD_"+snapshotDepthVar, "")
	out, err := renderDeviceXctestrun([]byte(sampleXctestrun), 22517, agentBundleIDs(""))
	if err != nil {
		t.Fatal(err)
	}
	target := decodeXctestrun(t, out)
	for _, key := range []string{"EnvironmentVariables", "TestingEnvironmentVariables"} {
		if env := target[key].(map[string]interface{}); env[snapshotDepthVar] != "62" {
			t.Errorf("%s[%s] = %v, want 62", key, snapshotDepthVar, env[snapshotDepthVar])
		}
	}
}

func TestSimulatorXctestrunSetsSnapshotDepth(t *testing.T) {
	t.Setenv(snapshotDepthVar, "")
	t.Setenv("SIMCTL_CHILD_"+snapshotDepthVar, "")
	// A built agent.xctestrun has both environment dictionaries; the sample
	// gets the second one from renderDeviceXctestrun.
	built, err := renderDeviceXctestrun([]byte(sampleXctestrun), 1, agentBundleIDs(""))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.xctestrun"), built, 0o644); err != nil {
		t.Fatal(err)
	}
	a := &Agent{opts: AgentOptions{Dir: dir}}
	path, err := a.xctestrunFor(22600)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	env := decodeXctestrun(t, raw)["EnvironmentVariables"].(map[string]interface{})
	if env[snapshotDepthVar] != "62" || env["DL_AGENT_PORT"] != "22600" {
		t.Errorf("EnvironmentVariables = %v, want %s=62 and the port", env, snapshotDepthVar)
	}
}
