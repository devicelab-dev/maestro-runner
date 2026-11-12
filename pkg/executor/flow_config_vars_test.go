package executor

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func newConfigVarsRunner(cfg flow.Config, cliEnv map[string]string) *FlowRunner {
	se := NewScriptEngine()
	se.SetExternalVariables(cliEnv)
	return &FlowRunner{flow: flow.Flow{Config: cfg}, script: se}
}

// appId: ${X} with X from the flow's own env: block resolves to X's value,
// not "undefined".
func TestApplyFlowConfigVariables_AppIDFromFlowEnv(t *testing.T) {
	fr := newConfigVarsRunner(flow.Config{
		AppID: "${PKG}",
		Env:   map[string]string{"PKG": "com.example.app"},
	}, nil)
	defer fr.script.Close()

	fr.applyFlowConfigVariables()

	if fr.flow.Config.AppID != "com.example.app" {
		t.Errorf("AppID = %q, want com.example.app", fr.flow.Config.AppID)
	}
	if got := fr.script.ExpandVariables("${APP_ID}"); got != "com.example.app" {
		t.Errorf("APP_ID = %q, want com.example.app", got)
	}
}

// A -e value still reaches appId: ${X} when the flow does not define X.
func TestApplyFlowConfigVariables_AppIDFromCLIEnv(t *testing.T) {
	fr := newConfigVarsRunner(flow.Config{AppID: "${PKG}"}, map[string]string{"PKG": "com.cli.app"})
	defer fr.script.Close()

	fr.applyFlowConfigVariables()

	if fr.flow.Config.AppID != "com.cli.app" {
		t.Errorf("AppID = %q, want com.cli.app", fr.flow.Config.AppID)
	}
}

// Precedence is unchanged: the flow's env: is applied after -e, so it wins,
// and ${X || "default"} lets a -e value through.
func TestApplyFlowConfigVariables_FlowEnvPrecedence(t *testing.T) {
	fr := newConfigVarsRunner(flow.Config{
		AppID: "${PKG}",
		Env: map[string]string{
			"PKG":  "com.flow.app",
			"USER": `${USER_IN || "nobody"}`,
		},
	}, map[string]string{"PKG": "com.cli.app", "USER_IN": "alice"})
	defer fr.script.Close()

	fr.applyFlowConfigVariables()

	if fr.flow.Config.AppID != "com.flow.app" {
		t.Errorf("AppID = %q, want com.flow.app", fr.flow.Config.AppID)
	}
	if got := fr.script.ExpandVariables("${USER}"); got != "alice" {
		t.Errorf("USER = %q, want alice", got)
	}
}

// A flow env: that sets APP_ID itself keeps it.
func TestApplyFlowConfigVariables_FlowEnvAppIDKept(t *testing.T) {
	fr := newConfigVarsRunner(flow.Config{
		AppID: "com.example.app",
		Env:   map[string]string{"APP_ID": "com.other"},
	}, nil)
	defer fr.script.Close()

	fr.applyFlowConfigVariables()

	if got := fr.script.ExpandVariables("${APP_ID}"); got != "com.other" {
		t.Errorf("APP_ID = %q, want com.other", got)
	}
}
