package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// timeoutsSeen lists the timeout (and a scrollUntilVisible's speed) of each
// step the driver got, in order.
func timeoutsSeen(seen []flow.Step) []string {
	var out []string
	for _, step := range seen {
		switch s := step.(type) {
		case *flow.WaitUntilStep:
			out = append(out, fmt.Sprintf("wait %d", s.TimeoutMs))
		case *flow.WaitForAnimationToEndStep:
			out = append(out, fmt.Sprintf("animation %d", s.TimeoutMs))
		case *flow.ScrollUntilVisibleStep:
			out = append(out, fmt.Sprintf("scroll %d speed %d", s.TimeoutMs, s.Speed))
		}
	}
	return out
}

// Maestro reads extendedWaitUntil's, waitForAnimationToEnd's and
// scrollUntilVisible's timeout, and scrollUntilVisible's speed, as text and
// evaluates them with the step's other scripts (Commands.kt evaluateScripts).
// The step gets the value its variables hold when it runs: at the top level,
// in a sub-flow run twice with different env, and in each pass of a repeat.
func TestNumberFieldExpressionsReachTheDriver(t *testing.T) {
	result, seen := runFlowYAML(t, map[string]string{
		"main.yaml": `appId: x
env:
  PAUSE_MS: 3000
  SPEED: 40
  PADDED: " 1500 "
---
- extendedWaitUntil:
    visible: Done
    timeout: ${PAUSE_MS}
- waitForAnimationToEnd:
    timeout: ${PAUSE_MS * 2}
- scrollUntilVisible:
    element: Row
    timeout: ${PAUSE_MS}
    speed: ${SPEED}
- runFlow:
    file: wait.yaml
    env:
      WAIT_MS: 1000
- runFlow:
    file: wait.yaml
    env:
      WAIT_MS: 2000
- evalScript: ${output.n = 0}
- repeat:
    times: 2
    commands:
      - evalScript: ${output.n = output.n + 1}
      - waitForAnimationToEnd:
          timeout: ${output.n * 500}
- extendedWaitUntil:
    visible: Done
    timeout: ${PADDED}
`,
		"wait.yaml": `appId: x
---
- extendedWaitUntil:
    notVisible: Spinner
    timeout: ${WAIT_MS}
`,
	})
	if result.Status != report.StatusPassed {
		t.Fatalf("status = %s, want passed: %+v", result.Status, result.FlowResults)
	}
	want := []string{
		"wait 3000",
		"animation 6000",
		"scroll 3000 speed 40",
		"wait 1000",
		"wait 2000",
		"animation 500",
		"animation 1000",
		"wait 1500",
	}
	got := timeoutsSeen(seen)
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("the driver got\n  %s\nwant\n  %s", strings.Join(got, ", "), strings.Join(want, ", "))
	}
}

// A value that is not a number fails the step, which does not run. Maestro
// lets an optional extendedWaitUntil or scrollUntilVisible go on (a warning
// there: its timeout error is an InvalidCommand; here the step is reported
// failed and the flow continues), but never a bad scroll
// speed (InvalidNumericFieldValue) or waitForAnimationToEnd timeout (a
// NumberFormatException): Orchestra.kt executeCommands.
func TestNumberFieldExpressionThatIsNotANumber(t *testing.T) {
	for _, c := range []struct {
		name, step string
		want       report.Status
		error      string
		header     string
	}{
		{"extendedWaitUntil", `- extendedWaitUntil:
    visible: Done
    timeout: ${LATER}`, report.StatusFailed, `extendedWaitUntil timeout: ${LATER} is "soon", not a number of milliseconds`, ""},
		{"optional extendedWaitUntil", `- extendedWaitUntil:
    visible: Done
    timeout: ${LATER}
    optional: true`, report.StatusPassed, "", ""},
		{"optional scrollUntilVisible timeout", `- scrollUntilVisible:
    element: Row
    timeout: ${LATER}
    optional: true`, report.StatusPassed, "", ""},
		{"optional waitForAnimationToEnd", `- waitForAnimationToEnd:
    timeout: ${LATER}
    optional: true`, report.StatusFailed, `waitForAnimationToEnd timeout: ${LATER} is "soon", not a number of milliseconds`, ""},
		{"optional scrollUntilVisible speed", `- scrollUntilVisible:
    element: Row
    speed: ${FAST}
    optional: true`, report.StatusFailed, `scrollUntilVisible speed: ${FAST} is "400", not a number between 0 and 100`, ""},
		// Maestro evaluates the speed first, so its error is the step's.
		{"optional scrollUntilVisible, speed and timeout both bad", `- scrollUntilVisible:
    element: Row
    timeout: ${LATER}
    speed: ${FAST}
    optional: true`, report.StatusFailed, `scrollUntilVisible speed: ${FAST} is "400"`, ""},
		// A ${...} that throws fails even an optional step (JsEvaluationException).
		{"optional extendedWaitUntil whose script throws", `- extendedWaitUntil:
    visible: Done
    timeout: ${output.wait.ms}
    optional: true`, report.StatusFailed, "TypeError", ""},
		{"extendedWaitUntil in a runFlow", `- runFlow:
    commands:
      - extendedWaitUntil:
          visible: Done
          timeout: ${LATER}`, report.StatusFailed, `extendedWaitUntil timeout: ${LATER} is "soon"`, ""},
		{"optional extendedWaitUntil in a runFlow", `- runFlow:
    commands:
      - extendedWaitUntil:
          visible: Done
          timeout: ${LATER}
          optional: true`, report.StatusPassed, "", ""},
		{"waitForAnimationToEnd in a repeat", `- repeat:
    times: 2
    commands:
      - waitForAnimationToEnd:
          timeout: ${LATER}`, report.StatusFailed, `waitForAnimationToEnd timeout: ${LATER} is "soon"`, ""},
		{"optional extendedWaitUntil in a repeat", `- repeat:
    times: 2
    commands:
      - extendedWaitUntil:
          visible: Done
          timeout: ${LATER}
          optional: true`, report.StatusPassed, "", ""},
		{"extendedWaitUntil in a retry", `- retry:
    maxRetries: 2
    commands:
      - extendedWaitUntil:
          visible: Done
          timeout: ${LATER}`, report.StatusFailed, `extendedWaitUntil timeout: ${LATER} is "soon"`, ""},
		{"extendedWaitUntil in onFlowStart", `- tapOn: Next`, report.StatusFailed, `extendedWaitUntil timeout: ${LATER} is "soon"`,
			"onFlowStart:\n  - extendedWaitUntil:\n      visible: Done\n      timeout: ${LATER}\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			result, seen := runFlowYAML(t, map[string]string{
				"main.yaml": "appId: x\nenv:\n  LATER: soon\n  FAST: 400\n" + c.header + "---\n" + c.step + "\n",
			})
			if result.Status != c.want {
				t.Fatalf("status = %s, want %s: %+v", result.Status, c.want, result.FlowResults)
			}
			if c.error != "" && !strings.Contains(result.FlowResults[0].Error, c.error) {
				t.Errorf("error = %q, want %q", result.FlowResults[0].Error, c.error)
			}
			for _, step := range seen {
				if _, tap := step.(*flow.TapOnStep); !tap {
					t.Errorf("the driver got %s; want no step with the bad value", step.Describe())
				}
			}
		})
	}
}
