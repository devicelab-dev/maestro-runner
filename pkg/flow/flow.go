// Package flow handles parsing and representation of Maestro YAML flow files.
package flow

// Flow represents a parsed Maestro flow file.
type Flow struct {
	SourcePath string // Path to the source file
	Config     Config // Flow configuration (appId, tags, etc.)
	Steps      []Step // Steps to execute
}

// IsSuite returns true if this flow is a suite file (only contains runFlow steps with file references).
// Suite files orchestrate multiple test cases; each runFlow represents a test case.
// Requires at least 2 runFlow steps with file references to be considered a suite.
// A single runFlow is just a wrapper, not a suite.
// Inline runFlow steps (no file, only commands) are NOT considered suite entries.
// onFlowStart/onFlowComplete hooks are ignored for detection as they're setup/teardown.
func (f *Flow) IsSuite() bool {
	if len(f.Steps) == 0 {
		return false
	}

	runFlowWithFileCount := 0
	for _, step := range f.Steps {
		if step.Type() != StepRunFlow {
			// Any non-runFlow step means it's not a pure suite
			return false
		}
		// Check if this runFlow has a file reference (not inline)
		if rf, ok := step.(*RunFlowStep); ok && rf.File != "" {
			runFlowWithFileCount++
		}
	}

	// Must have at least 2 runFlow steps with file references to be a suite
	// A single runFlow is just a wrapper, not a test suite
	return runFlowWithFileCount >= 2
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
	OnFlowStart        []Step            `yaml:"-"`                  // Lifecycle hook: runs before commands
	OnFlowComplete     []Step            `yaml:"-"`                  // Lifecycle hook: runs after commands
}
