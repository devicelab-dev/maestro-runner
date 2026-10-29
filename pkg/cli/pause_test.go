package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/executor"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
	"github.com/urfave/cli/v2"
)

func TestPauseOnFailureFlag(t *testing.T) {
	for _, f := range testCommand.Flags {
		if f.Names()[0] != "pause-on-failure" {
			continue
		}
		bf, ok := f.(*cli.BoolFlag)
		if !ok {
			t.Fatalf("--pause-on-failure is %T, want a bool flag", f)
		}
		if len(bf.EnvVars) != 1 || bf.EnvVars[0] != "MAESTRO_PAUSE_ON_FAILURE" {
			t.Errorf("env vars = %v, want MAESTRO_PAUSE_ON_FAILURE", bf.EnvVars)
		}
		return
	}
	t.Fatal("test has no --pause-on-failure flag")
}

func TestResolvePauseOnFailure(t *testing.T) {
	tests := []struct {
		name     string
		want     bool
		parallel int
		devices  []string
		terminal bool
		on       bool
		warns    string
	}{
		{"off", false, 0, nil, true, false, ""},
		{"off without a terminal says nothing", false, 0, nil, false, false, ""},
		{"on in a terminal", true, 0, []string{"emulator-5554"}, true, true, ""},
		{"no terminal", true, 0, nil, false, false, "stdin is not a terminal"},
		{"parallel", true, 2, nil, true, false, "--parallel"},
		{"several devices", true, 0, []string{"a", "b"}, true, false, "several devices"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			on, warning := resolvePauseOnFailure(tt.want, tt.parallel, tt.devices, tt.terminal)
			if on != tt.on {
				t.Errorf("on = %v, want %v", on, tt.on)
			}
			if (tt.warns == "") != (warning == "") || !strings.Contains(warning, tt.warns) {
				t.Errorf("warning = %q, want one containing %q", warning, tt.warns)
			}
		})
	}
}

func TestFailurePauser_NilWhenOff(t *testing.T) {
	if p := failurePauser(&RunConfig{}, report.Device{}, "devicelab"); p != nil {
		t.Errorf("failurePauser() = %v with the flag off, want nil", p)
	}
}

func TestExternalHierarchyHint(t *testing.T) {
	android := report.Device{ID: "emulator-5554", Platform: "android"}
	if got := externalHierarchyHint(android, "devicelab"); got != "maestro-runner --device emulator-5554 hierarchy --compact" {
		t.Errorf("android hint = %q", got)
	}
	for name, hint := range map[string]string{
		"appium": externalHierarchyHint(android, "appium"),
		"ios":    externalHierarchyHint(report.Device{ID: "X", Platform: "ios"}, "wda"),
		"no id":  externalHierarchyHint(report.Device{Platform: "android"}, "devicelab"),
	} {
		if hint != "" {
			t.Errorf("%s hint = %q, want none", name, hint)
		}
	}
}

func pauseWith(t *testing.T, in io.Reader) (executor.PauseAction, string) {
	t.Helper()
	var out bytes.Buffer
	p := newTerminalPauser(in, &out, t.TempDir(), "maestro-runner --device e hierarchy --compact")
	action := p.Pause(context.Background(), &executor.PausedStep{
		FlowName: "login", FlowFile: "flows/login.yaml", StepIndex: 3, StepCount: 9,
		Step: `tapOn: "Sign in"`, Error: "Element not found", Attempt: 1,
	})
	return action, out.String()
}

func TestTerminalPauser_EnterContinues(t *testing.T) {
	action, out := pauseWith(t, strings.NewReader("\n"))
	if action != executor.PauseContinue {
		t.Errorf("action = %v, want continue", action)
	}
	for _, want := range []string{"PAUSED", "login (flows/login.yaml)", `4 of 9: tapOn: "Sign in"`, "Element not found",
		"From another terminal: maestro-runner --device e hierarchy --compact", "retry the failed step"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestTerminalPauser_CommandsThenRetry(t *testing.T) {
	action, out := pauseWith(t, strings.NewReader("?\nbogus\nx\nr\n"))
	if action != executor.PauseRetry {
		t.Errorf("action = %v, want retry", action)
	}
	for _, want := range []string{`unknown command "bogus"`, "x needs a step", "retrying the step"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestTerminalPauser_EOFContinues(t *testing.T) {
	action, out := pauseWith(t, strings.NewReader(""))
	if action != executor.PauseContinue || !strings.Contains(out, "stdin closed") {
		t.Errorf("action = %v, out = %q; want continue on EOF", action, out)
	}
}

func TestTerminalPauser_CancelledContextContinues(t *testing.T) {
	var out bytes.Buffer
	r, w := io.Pipe()
	defer w.Close()
	p := newTerminalPauser(r, &out, t.TempDir(), "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if action := p.Pause(ctx, &executor.PausedStep{}); action != executor.PauseContinue {
		t.Errorf("action = %v, want continue", action)
	}
}
