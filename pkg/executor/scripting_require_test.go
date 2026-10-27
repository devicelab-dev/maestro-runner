package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func writeScriptFile(t *testing.T, path, src string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The layout from #253: one shared helper script, called from flows at two
// depths, requires its library relative to itself.
func TestExecuteRunScript_RequireFromScriptDir(t *testing.T) {
	flows := t.TempDir()
	writeScriptFile(t, filepath.Join(flows, "common", "lib", "env.js"), `module.exports = { name: "env" };`)
	writeScriptFile(t, filepath.Join(flows, "common", "helpers", "do_thing.js"),
		`const env = require("../lib/env.js"); output.envName = env.name;`)

	cases := []struct {
		flowDir string
		file    string
	}{
		{filepath.Join(flows, "a"), "../common/helpers/do_thing.js"},
		{filepath.Join(flows, "a", "b", "c"), "../../../common/helpers/do_thing.js"},
	}

	// One engine for both callers, as in a run, so the module cache is shared.
	se := NewScriptEngine()
	defer se.Close()
	for _, c := range cases {
		se.SetFlowDir(c.flowDir)
		se.SetVariable("envName", "")
		res := se.ExecuteRunScript(&flow.RunScriptStep{File: c.file})
		if !res.Success {
			t.Fatalf("from %s: %s (%v)", c.flowDir, res.Message, res.Error)
		}
		if got := se.GetVariable("envName"); got != "env" {
			t.Errorf("from %s: envName = %q, want env", c.flowDir, got)
		}
	}
}

// A path that only resolves from the flow's directory keeps working.
func TestExecuteRunScript_RequireFallsBackToFlowDir(t *testing.T) {
	flows := t.TempDir()
	flowDir := filepath.Join(flows, "a")
	writeScriptFile(t, filepath.Join(flowDir, "lib", "env.js"), `module.exports = { name: "flow-env" };`)
	writeScriptFile(t, filepath.Join(flows, "common", "do_thing.js"),
		`output.envName = require("./lib/env.js").name;`)

	se := NewScriptEngine()
	defer se.Close()
	se.SetFlowDir(flowDir)

	res := se.ExecuteRunScript(&flow.RunScriptStep{File: "../common/do_thing.js"})
	if !res.Success {
		t.Fatalf("%s (%v)", res.Message, res.Error)
	}
	if got := se.GetVariable("envName"); got != "flow-env" {
		t.Errorf("envName = %q, want flow-env", got)
	}
}

// An inline script resolves from the flow's directory only, and a script
// file's directory does not outlive the script, even when it throws.
func TestExecuteRunScript_InlineRequireAndScriptDirRestored(t *testing.T) {
	flows := t.TempDir()
	flowDir := filepath.Join(flows, "a")
	writeScriptFile(t, filepath.Join(flowDir, "lib.js"), `module.exports = "flow";`)
	writeScriptFile(t, filepath.Join(flows, "common", "lib.js"), `module.exports = "common";`)
	writeScriptFile(t, filepath.Join(flows, "common", "throws.js"),
		`require("./lib.js"); throw new Error("boom");`)

	se := NewScriptEngine()
	defer se.Close()
	se.SetFlowDir(flowDir)

	if res := se.ExecuteRunScript(&flow.RunScriptStep{File: "../common/throws.js"}); res.Success {
		t.Fatal("expected the throwing script to fail")
	}

	res := se.ExecuteRunScript(&flow.RunScriptStep{Script: `output.lib = require("./lib.js");`})
	if !res.Success {
		t.Fatalf("%s (%v)", res.Message, res.Error)
	}
	if got := se.GetVariable("lib"); got != "flow" {
		t.Errorf("inline require = %q, want the flow's module", got)
	}
}
