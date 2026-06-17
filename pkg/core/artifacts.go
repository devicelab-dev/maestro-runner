// SPDX-FileCopyrightText: 2024 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

// Package core provides the execution model types for maestro-runner.
package core

import (
	"time"
)

// Attachment represents a debug artifact captured during step execution
type Attachment struct {
	Name        string `json:"name"`        // Descriptive name: screenshot, hierarchy, log
	ContentType string `json:"contentType"` // MIME type: image/png, application/json, text/plain
	Path        string `json:"path"`        // File path relative to output directory
	Body        []byte `json:"-"`           // In-memory content (not serialized to JSON)
}

// Common attachment names
const (
	AttachmentScreenshot = "screenshot"
	AttachmentHierarchy  = "hierarchy"
	AttachmentDeviceLog  = "device_log"
	AttachmentNetworkLog = "network_log"
	AttachmentVideo      = "video"
)

// Common content types
const (
	ContentTypePNG  = "image/png"
	ContentTypeJPEG = "image/jpeg"
	ContentTypeJSON = "application/json"
	ContentTypeText = "text/plain"
	ContentTypeMP4  = "video/mp4"
)

// NewScreenshotAttachment creates a screenshot attachment
func NewScreenshotAttachment(path string, data []byte) Attachment {
	return Attachment{
		Name:        AttachmentScreenshot,
		ContentType: ContentTypePNG,
		Path:        path,
		Body:        data,
	}
}

// NewHierarchyAttachment creates a UI hierarchy attachment
func NewHierarchyAttachment(path string, data []byte) Attachment {
	return Attachment{
		Name:        AttachmentHierarchy,
		ContentType: ContentTypeJSON,
		Path:        path,
		Body:        data,
	}
}

// ArtifactConfig controls when and what artifacts are captured
type ArtifactConfig struct {
	// When to capture
	CaptureOnFailure bool `yaml:"captureOnFailure" json:"captureOnFailure"` // Default: true
	CaptureOnSuccess bool `yaml:"captureOnSuccess" json:"captureOnSuccess"` // Default: false
	CaptureOnTimeout bool `yaml:"captureOnTimeout" json:"captureOnTimeout"` // Default: true

	// What to capture
	Screenshot  bool `yaml:"screenshot" json:"screenshot"`   // Default: true
	UIHierarchy bool `yaml:"uiHierarchy" json:"uiHierarchy"` // Default: true
	DeviceLogs  bool `yaml:"deviceLogs" json:"deviceLogs"`   // Default: false (verbose)
	NetworkLogs bool `yaml:"networkLogs" json:"networkLogs"` // Default: false
}

// DefaultArtifactConfig returns sensible defaults for artifact capture
func DefaultArtifactConfig() ArtifactConfig {
	return ArtifactConfig{
		CaptureOnFailure: true,
		CaptureOnSuccess: false,
		CaptureOnTimeout: true,
		Screenshot:       true,
		UIHierarchy:      true,
		DeviceLogs:       false,
		NetworkLogs:      false,
	}
}

// ShouldCapture returns true if artifacts should be captured for the given status
func (c ArtifactConfig) ShouldCapture(status StepStatus) bool {
	switch status {
	case StatusFailed, StatusErrored:
		return c.CaptureOnFailure
	case StatusPassed:
		return c.CaptureOnSuccess
	default:
		return false
	}
}

// ShouldCaptureTimeout returns true if artifacts should be captured on timeout
func (c ArtifactConfig) ShouldCaptureTimeout() bool {
	return c.CaptureOnTimeout
}

// ArtifactCollector defines the interface for capturing debug artifacts
// Implementations are executor-specific (Appium, native, etc.)
type ArtifactCollector interface {
	// CaptureScreenshot takes a screenshot and returns PNG data
	CaptureScreenshot() ([]byte, error)

	// CaptureHierarchy captures the UI hierarchy as JSON
	CaptureHierarchy() ([]byte, error)

	// CaptureDeviceLogs returns device logs since the given time
	CaptureDeviceLogs(since time.Time) ([]string, error)
}

// NullArtifactCollector is a no-op implementation for testing
type NullArtifactCollector struct{}

// CaptureScreenshot returns nil (no-op)
func (n NullArtifactCollector) CaptureScreenshot() ([]byte, error) { return nil, nil }

// CaptureHierarchy returns nil (no-op)
func (n NullArtifactCollector) CaptureHierarchy() ([]byte, error) { return nil, nil }

// CaptureDeviceLogs returns nil (no-op)
func (n NullArtifactCollector) CaptureDeviceLogs(_ time.Time) ([]string, error) {
	return nil, nil
}
