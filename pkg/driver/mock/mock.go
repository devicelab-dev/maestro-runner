// Package mock provides a mock driver for testing without a real device.
package mock

import (
	"fmt"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// Driver is a mock implementation of core.Driver for testing.
type Driver struct {
	// Configuration
	Config Config

	// Internal state
	stepCount int
}

// Config configures mock driver behavior.
type Config struct {
	// FailOnStep makes step N fail (1-indexed). 0 = never fail.
	FailOnStep int
	// StepDelay adds artificial delay per step
	StepDelay time.Duration
	// Platform info to report
	Platform string
	DeviceID string
}

// New creates a new mock driver.
func New(cfg Config) *Driver {
	if cfg.Platform == "" {
		cfg.Platform = "mock"
	}
	if cfg.DeviceID == "" {
		cfg.DeviceID = "mock-device"
	}
	return &Driver{Config: cfg}
}

// Execute simulates executing a step.
func (d *Driver) Execute(step flow.Step) *core.CommandResult {
	d.stepCount++
	start := time.Now()

	// Simulate delay
	if d.Config.StepDelay > 0 {
		time.Sleep(d.Config.StepDelay)
	}

	// Check if this step should fail
	if d.Config.FailOnStep > 0 && d.stepCount == d.Config.FailOnStep {
		return &core.CommandResult{
			Success:  false,
			Duration: time.Since(start),
			Error:    fmt.Errorf("mock failure on step %d", d.stepCount),
			Message:  fmt.Sprintf("Simulated failure on step %d (%s)", d.stepCount, step.Type()),
		}
	}

	// Success - return element info for tap/assert steps
	result := &core.CommandResult{
		Success:  true,
		Duration: time.Since(start),
		Message:  fmt.Sprintf("Mock executed: %s", step.Type()),
	}

	// Add mock element for relevant steps
	if needsElement(step) {
		result.Element = &core.ElementInfo{
			ID:      "mock-element",
			Text:    "Mock Element",
			Visible: true,
			Enabled: true,
			Bounds:  core.Bounds{X: 100, Y: 200, Width: 200, Height: 50},
		}
	}

	return result
}

// Screenshot returns a mock PNG image.
func (d *Driver) Screenshot() ([]byte, error) {
	// Minimal valid PNG (1x1 transparent pixel)
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}, nil
}

// Hierarchy returns a mock view hierarchy.
func (d *Driver) Hierarchy() ([]byte, error) {
	return []byte(`{
  "type": "View",
  "bounds": {"x": 0, "y": 0, "width": 1080, "height": 2400},
  "children": [
    {
      "type": "Button",
      "id": "mock-element",
      "text": "Mock Element",
      "bounds": {"x": 100, "y": 200, "width": 200, "height": 50}
    }
  ]
}`), nil
}

// GetState returns mock device state.
func (d *Driver) GetState() *core.StateSnapshot {
	return &core.StateSnapshot{
		AppState:    "foreground",
		Orientation: "portrait",
	}
}

// GetPlatformInfo returns mock platform info.
func (d *Driver) GetPlatformInfo() *core.PlatformInfo {
	return &core.PlatformInfo{
		Platform:     d.Config.Platform,
		DeviceID:     d.Config.DeviceID,
		DeviceName:   "Mock Device",
		OSVersion:    "1.0",
		IsSimulator:  true,
		ScreenWidth:  1080,
		ScreenHeight: 2400,
	}
}

// SetFindTimeout is a no-op for mock driver.
func (d *Driver) SetFindTimeout(ms int) {
	// Mock driver doesn't have actual timeouts
}

// SetWaitForIdleTimeout is a no-op for mock driver.
func (d *Driver) SetWaitForIdleTimeout(ms int) error {
	// Mock driver doesn't have actual idle timeout
	return nil
}

// needsElement returns true if the step type typically returns element info.
func needsElement(step flow.Step) bool {
	switch step.Type() {
	case flow.StepTapOn, flow.StepDoubleTapOn, flow.StepLongPressOn,
		flow.StepAssertVisible, flow.StepScrollUntilVisible,
		flow.StepCopyTextFrom:
		return true
	}
	return false
}
