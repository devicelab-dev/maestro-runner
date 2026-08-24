package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// defaultRunShellTimeout bounds a command that never returns. A flow that hangs
// forever on a shell call is worse than one that fails, because CI will sit on
// it until the job times out with nothing to show.
const defaultRunShellTimeout = 30 * time.Second

// maxCapturedOutput caps what is kept from a command that prints without
// stopping. The tail is what matters when something went wrong, so the head is
// what gets dropped.
const maxCapturedOutput = 64 * 1024

// executeRunShell runs a command on the host and, optionally, binds its output
// to a flow variable.
//
// Host rather than device, deliberately: the requests behind this are for adb,
// simctl and xcrun — tooling that runs beside the test, not inside the app. A
// device shell is one `adb shell` away from here, and spelling it out keeps the
// step honest about where the command lands.
func (fr *FlowRunner) executeRunShell(step *flow.RunShellStep) *core.CommandResult {
	command := strings.TrimSpace(step.Command)
	if command == "" {
		return &core.CommandResult{Success: false, Message: "runShell needs a command", Error: fmt.Errorf("empty command")}
	}

	timeout := defaultRunShellTimeout
	if step.TimeoutMs > 0 {
		timeout = time.Duration(step.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(fr.ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = fr.shellEnv(step.Env)

	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(truncateOutput(string(output)))

	if step.Output != "" {
		// Bound even when the command failed: a flow that wants to inspect the
		// failure needs what was printed, and an optional step that swallows
		// the error still has to leave the variable defined.
		fr.script.SetVariable(step.Output, text)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return &core.CommandResult{
			Success: false,
			Message: fmt.Sprintf("runShell timed out after %s: %s", timeout, command),
			Error:   fmt.Errorf("command timed out"),
		}
	}
	if err != nil {
		// Most failing commands explain themselves on stderr; the ones that
		// say nothing should not contribute a blank line to the report.
		message := fmt.Sprintf("runShell failed: %s", command)
		if text != "" {
			message += "\n" + text
		}
		return &core.CommandResult{Success: false, Message: message, Error: err}
	}

	return &core.CommandResult{Success: true, Message: runShellMessage(command, text)}
}

// shellEnv builds the command's environment: the runner's own, then the flow's
// variables, then the step's own env, then the MAESTRO_* description of the
// device under test.
//
// The device values matter more than they look. A parallel run drives several
// devices at once, and a flow that shells out to adb has no other way to know
// which one it belongs to — `adb shell` with two devices attached fails
// outright. `adb -s $MAESTRO_DEVICE_ID shell ...` is correct under --parallel
// and identical to `adb shell` when only one device is attached.
func (fr *FlowRunner) shellEnv(stepEnv map[string]string) []string {
	env := os.Environ()

	for name, value := range fr.script.Variables() {
		env = append(env, name+"="+value)
	}
	for name, value := range stepEnv {
		env = append(env, name+"="+value)
	}

	device := fr.config.Device
	if device.ID != "" {
		env = append(env, "MAESTRO_DEVICE_ID="+device.ID)
	}
	if device.Platform != "" {
		env = append(env, "MAESTRO_PLATFORM="+device.Platform)
	}
	if appID := fr.config.App.ID; appID != "" {
		env = append(env, "MAESTRO_APP_ID="+appID)
	}
	return env
}

// runShellMessage keeps the step line readable. A command that printed nothing
// is the common case — most adb calls are silent on success — and saying so is
// clearer than an empty pair of quotes.
func runShellMessage(command, output string) string {
	if output == "" {
		return fmt.Sprintf("Ran: %s", command)
	}
	if idx := strings.IndexByte(output, '\n'); idx >= 0 {
		return fmt.Sprintf("Ran: %s → %s…", command, output[:idx])
	}
	return fmt.Sprintf("Ran: %s → %s", command, output)
}

// truncateOutput keeps the tail of an oversized output, which is where a
// failure's explanation lives.
func truncateOutput(s string) string {
	if len(s) <= maxCapturedOutput {
		return s
	}
	return "…(truncated)…\n" + s[len(s)-maxCapturedOutput:]
}
