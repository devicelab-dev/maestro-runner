// Package flow handles parsing and representation of Maestro YAML flow files.
package flow

import "fmt"

// Flow represents a parsed Maestro flow file.
type Flow struct {
	SourcePath string // Path to the source file
	Config     Config // Flow configuration (appId, tags, etc.)
	Steps      []Step // Steps to execute
}

// IsSuite reports whether this flow runs as a suite: its header says `suite: true` and its steps can be split
// (SuiteProblem is ""). Each runFlow of a suite then runs as a flow of its own, a test case, with the suite's
// appId and env. Any other flow, one made only of runFlow steps included, is one flow, its runFlows nested in
// it, as Maestro runs every flow file.
func (f *Flow) IsSuite() bool {
	return f.Config.Suite && f.SuiteProblem() == ""
}

// SuiteProblem says why this flow's steps cannot be split into a suite's test cases, or "" when they can.
// A suite is made only of runFlow steps with file references, at least 2 of them: a single runFlow is just a
// wrapper. A runFlow with inline commands, a `when` condition or an else branch is part of the flow itself, not a
// test case: splitting the file would drop the commands and run conditional files unconditionally. Nor can
// a suite have its own onFlowStart or onFlowComplete: its parts run as flows of their own, so the file's hooks
// would never run.
func (f *Flow) SuiteProblem() string {
	if len(f.Config.OnFlowStart) > 0 {
		return "it has onFlowStart"
	}
	if len(f.Config.OnFlowComplete) > 0 {
		return "it has onFlowComplete"
	}
	if len(f.Steps) == 0 {
		return "it has no steps"
	}

	runFlowWithFileCount := 0
	for i, step := range f.Steps {
		if step.Type() != StepRunFlow {
			return fmt.Sprintf("step %d is %s, not a runFlow", i+1, step.Type())
		}
		rf, ok := step.(*RunFlowStep)
		switch {
		case !ok || rf.File == "":
			return fmt.Sprintf("step %d is a runFlow with inline commands", i+1)
		case rf.When != nil:
			return fmt.Sprintf("step %d is a runFlow with a when: condition", i+1)
		case rf.ElseFile != "" || len(rf.ElseSteps) > 0:
			return fmt.Sprintf("step %d is a runFlow with an else", i+1)
		}
		runFlowWithFileCount++
	}

	if runFlowWithFileCount < 2 {
		return "it has only one runFlow"
	}
	return ""
}

// GetTestCases returns the runFlow steps if this is a suite, empty slice otherwise.
// Each runFlow in a suite represents a separate test case.
func (f *Flow) GetTestCases() []*RunFlowStep {
	if !f.IsSuite() {
		return nil
	}

	var testCases []*RunFlowStep
	for _, step := range f.Steps {
		if rf, ok := step.(*RunFlowStep); ok {
			testCases = append(testCases, rf)
		}
	}
	return testCases
}

// EffectiveAppID returns AppID if set, otherwise falls back to URL.
// For web flows, the URL field serves as the app identifier (navigation target).
func (c Config) EffectiveAppID() string {
	if c.AppID != "" {
		return c.AppID
	}
	return c.URL
}

// Config represents flow-level configuration.
type Config struct {
	AppID              string            `yaml:"appId"`
	URL                string            `yaml:"url"` // Web app URL (alternative to appId)
	Name               string            `yaml:"name"`
	Tags               []string          `yaml:"tags"`
	Env                map[string]string `yaml:"env"`
	Timeout            int               `yaml:"timeout"`            // Flow timeout in ms
	CommandTimeout     int               `yaml:"commandTimeout"`     // Default timeout for all commands in ms (overrides driver default)
	WaitForIdleTimeout *int              `yaml:"waitForIdleTimeout"` // Wait for device idle in ms (nil = use global, 0 = disabled)
	DisableAnimations  *bool             `yaml:"disableAnimations"`  // Switch device animations off for this flow (nil = use global)
	TypingFrequency    *int              `yaml:"typingFrequency"`    // WDA typing speed in keys/sec (nil = use global, 0 = disabled)
	StepDelay          *int              `yaml:"stepDelay"`          // Pause between top-level steps in ms (nil = use global, 0 = none)
	Properties         map[string]string `yaml:"properties"`         // Custom key/values emitted as JUnit testcase properties (#84)
	FailOnConsoleError bool              `yaml:"failOnConsoleError"` // Web only: auto-fail the flow if any console.error / uncaught JS exception is captured
	Suite              bool              `yaml:"suite"`              // Run each runFlow as a flow of its own (see Flow.IsSuite); Maestro keeps the key and runs the file as one flow
	OnFlowStart        []Step            `yaml:"-"`                  // Lifecycle hook: runs before commands
	OnFlowComplete     []Step            `yaml:"-"`                  // Lifecycle hook: runs after commands
}
