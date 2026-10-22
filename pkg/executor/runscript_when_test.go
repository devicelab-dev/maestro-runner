package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// runScript takes a when: as Maestro's does, at the top level and inside a
// runFlow file; an unmet condition skips the script. It used to be parsed and
// ignored, so a script guarded by `when: true: ${MAESTRO_CI == 'true'}` ran
// on every machine.
func TestRunScriptWhen(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("set.js", "output.runs = (output.runs || 0) + 1;\n")
	write("sub.yaml", `appId: x
---
- runScript:
    file: set.js
    when:
      true: ${output.mode == 'ci'}
`)
	write("main.yaml", `appId: x
---
- evalScript: ${output.mode = 'local'}
- runScript:
    file: set.js
    when:
      true: ${output.mode == 'ci'}
- assertTrue: ${output.runs === undefined}
- runFlow:
    file: sub.yaml
- assertTrue: ${output.runs === undefined}
- runScript:
    file: set.js
    when:
      true: ${output.mode == 'local'}
- assertTrue: ${output.runs === 1}
- runScript:
    file: set.js
- assertTrue: ${output.runs === 2}
`)
	f, err := flow.ParseFile(filepath.Join(dir, "main.yaml"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var seen []flow.Step
	result := runOneFlow(t, realDriverRejects(&seen), *f)
	if result.Status != report.StatusPassed {
		t.Fatalf("status = %s, want passed: %+v", result.Status, result.FlowResults)
	}
}

// The condition is decoded from YAML onto the step.
func TestParseRunScriptWhen(t *testing.T) {
	f, err := flow.Parse([]byte(`appId: x
---
- runScript:
    file: a.js
    when:
      true: ${MAESTRO_CI == 'true'}
`), "t.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s, ok := f.Steps[0].(*flow.RunScriptStep)
	if !ok {
		t.Fatalf("step = %T", f.Steps[0])
	}
	if s.When == nil || s.When.Script != "${MAESTRO_CI == 'true'}" {
		t.Fatalf("when = %+v", s.When)
	}
}
