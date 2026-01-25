// Package flow handles parsing and representation of Maestro YAML flow files.
package flow

// Flow represents a parsed Maestro flow file.
type Flow struct {
	SourcePath string // Path to the source file
	Config     Config // Flow configuration (appId, tags, etc.)
	Steps      []Step // Steps to execute
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
