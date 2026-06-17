// SPDX-FileCopyrightText: 2024 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package core

import (
	"testing"
	"time"
)

func TestNewScreenshotAttachment(t *testing.T) {
	data := []byte{0x89, 0x50, 0x4E, 0x47} // PNG header
	attachment := NewScreenshotAttachment("step-1-screenshot.png", data)

	if attachment.Name != AttachmentScreenshot {
		t.Errorf("Name = %s, want %s", attachment.Name, AttachmentScreenshot)
	}
	if attachment.ContentType != ContentTypePNG {
		t.Errorf("ContentType = %s, want %s", attachment.ContentType, ContentTypePNG)
	}
	if attachment.Path != "step-1-screenshot.png" {
		t.Errorf("Path = %s, want 'step-1-screenshot.png'", attachment.Path)
	}
	if len(attachment.Body) != 4 {
		t.Errorf("Body length = %d, want 4", len(attachment.Body))
	}
}

func TestNewHierarchyAttachment(t *testing.T) {
	data := []byte(`{"type": "View", "children": []}`)
	attachment := NewHierarchyAttachment("step-1-hierarchy.json", data)

	if attachment.Name != AttachmentHierarchy {
		t.Errorf("Name = %s, want %s", attachment.Name, AttachmentHierarchy)
	}
	if attachment.ContentType != ContentTypeJSON {
		t.Errorf("ContentType = %s, want %s", attachment.ContentType, ContentTypeJSON)
	}
}

func TestDefaultArtifactConfig(t *testing.T) {
	cfg := DefaultArtifactConfig()

	if !cfg.CaptureOnFailure {
		t.Error("CaptureOnFailure should be true by default")
	}
	if cfg.CaptureOnSuccess {
		t.Error("CaptureOnSuccess should be false by default")
	}
	if !cfg.CaptureOnTimeout {
		t.Error("CaptureOnTimeout should be true by default")
	}
	if !cfg.Screenshot {
		t.Error("Screenshot should be true by default")
	}
	if !cfg.UIHierarchy {
		t.Error("UIHierarchy should be true by default")
	}
	if cfg.DeviceLogs {
		t.Error("DeviceLogs should be false by default")
	}
	if cfg.NetworkLogs {
		t.Error("NetworkLogs should be false by default")
	}
}

func TestArtifactConfig_ShouldCapture(t *testing.T) {
	cfg := DefaultArtifactConfig()

	tests := []struct {
		status   StepStatus
		expected bool
	}{
		{StatusFailed, true},
		{StatusErrored, true},
		{StatusPassed, false},
		{StatusWarned, false},
		{StatusSkipped, false},
		{StatusPending, false},
		{StatusRunning, false},
	}

	for _, tt := range tests {
		if got := cfg.ShouldCapture(tt.status); got != tt.expected {
			t.Errorf("ShouldCapture(%s) = %v, want %v", tt.status, got, tt.expected)
		}
	}
}

func TestArtifactConfig_ShouldCapture_CaptureOnSuccess(t *testing.T) {
	cfg := ArtifactConfig{
		CaptureOnSuccess: true,
		CaptureOnFailure: false,
	}

	if !cfg.ShouldCapture(StatusPassed) {
		t.Error("ShouldCapture(StatusPassed) should be true when CaptureOnSuccess is true")
	}
	if cfg.ShouldCapture(StatusFailed) {
		t.Error("ShouldCapture(StatusFailed) should be false when CaptureOnFailure is false")
	}
}

func TestArtifactConfig_ShouldCaptureTimeout(t *testing.T) {
	cfg := ArtifactConfig{CaptureOnTimeout: true}
	if !cfg.ShouldCaptureTimeout() {
		t.Error("ShouldCaptureTimeout() should be true")
	}

	cfg.CaptureOnTimeout = false
	if cfg.ShouldCaptureTimeout() {
		t.Error("ShouldCaptureTimeout() should be false")
	}
}

func TestNullArtifactCollector(t *testing.T) {
	collector := NullArtifactCollector{}

	screenshot, err := collector.CaptureScreenshot()
	if err != nil || screenshot != nil {
		t.Error("CaptureScreenshot() should return nil, nil")
	}

	hierarchy, err := collector.CaptureHierarchy()
	if err != nil || hierarchy != nil {
		t.Error("CaptureHierarchy() should return nil, nil")
	}

	logs, err := collector.CaptureDeviceLogs(time.Now())
	if err != nil || logs != nil {
		t.Error("CaptureDeviceLogs() should return nil, nil")
	}
}

func TestAttachmentConstants(t *testing.T) {
	// Verify constants are defined correctly
	if AttachmentScreenshot != "screenshot" {
		t.Error("AttachmentScreenshot constant mismatch")
	}
	if AttachmentHierarchy != "hierarchy" {
		t.Error("AttachmentHierarchy constant mismatch")
	}
	if ContentTypePNG != "image/png" {
		t.Error("ContentTypePNG constant mismatch")
	}
	if ContentTypeJSON != "application/json" {
		t.Error("ContentTypeJSON constant mismatch")
	}
}
