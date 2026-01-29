// Package config handles configuration for maestro-runner.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the workspace configuration (config.yaml).
type Config struct {
	// App info
	AppID string `yaml:"appId"` // App bundle ID or package name

	// Flow selection
	Flows       []string `yaml:"flows"`       // Glob patterns for flows
	IncludeTags []string `yaml:"includeTags"` // Tags to include
	ExcludeTags []string `yaml:"excludeTags"` // Tags to exclude

	// Execution settings
	Env map[string]string `yaml:"env"` // Environment variables

	// Device settings
	Platform string `yaml:"platform"` // Target platform
	Device   string `yaml:"device"`   // Target device

	// Driver settings
	WaitForIdleTimeout int `yaml:"waitForIdleTimeout"` // Wait for device idle in ms (0 = disabled, default 5000)
}

// Load loads configuration from a file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- user-provided config file
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadFromDir looks for config.yaml or config.yml in the directory.
func LoadFromDir(dir string) (*Config, error) {
	// Try config.yaml first
	configPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return Load(configPath)
	}

	// Try config.yml
	configPath = filepath.Join(dir, "config.yml")
	if _, err := os.Stat(configPath); err == nil {
		return Load(configPath)
	}

	// No config file found, return empty config
	return &Config{}, nil
}
