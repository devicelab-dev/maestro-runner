package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// ${...} in a runScript file is a JavaScript template literal, not a flow
// variable (Expo's compare-images-http.js builds its URL this way).
func TestRunScriptKeepsTemplateLiterals(t *testing.T) {
	dir := t.TempDir()
	js := "const SERVER_URL = 'http://localhost:7123';\n" +
		"output.url = `${SERVER_URL}/process`;\n" +
		"output.id = `${testID}-x`;\n"
	if err := os.WriteFile(filepath.Join(dir, "s.js"), []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	se := NewScriptEngine()
	se.SetFlowDir(dir)
	res := se.ExecuteRunScript(&flow.RunScriptStep{
		BaseStep: flow.BaseStep{StepType: flow.StepRunScript},
		Script:   "s.js",
		Env:      map[string]string{"testID": "abc"},
	})
	if !res.Success {
		t.Fatalf("runScript failed: %v", res.Error)
	}
	out := se.GetOutput()
	if out["url"] != "http://localhost:7123/process" {
		t.Errorf("url = %v, want http://localhost:7123/process", out["url"])
	}
	if out["id"] != "abc-x" {
		t.Errorf("id = %v, want abc-x (env reaches the script as a name)", out["id"])
	}
}
