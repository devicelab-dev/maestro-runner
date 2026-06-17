// SPDX-FileCopyrightText: 2024 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package core

import "testing"

func TestStepStatus_String(t *testing.T) {
	tests := []struct {
		status   StepStatus
		expected string
	}{
		{StatusPending, "pending"},
		{StatusRunning, "running"},
		{StatusPassed, "passed"},
		{StatusFailed, "failed"},
		{StatusErrored, "errored"},
		{StatusSkipped, "skipped"},
		{StatusWarned, "warned"},
		{StepStatus(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.expected {
			t.Errorf("StepStatus(%d).String() = %q, want %q", tt.status, got, tt.expected)
		}
	}
}

func TestStepStatus_IsTerminal(t *testing.T) {
	terminalStatuses := []StepStatus{StatusPassed, StatusFailed, StatusErrored, StatusSkipped, StatusWarned}
	nonTerminalStatuses := []StepStatus{StatusPending, StatusRunning}

	for _, s := range terminalStatuses {
		if !s.IsTerminal() {
			t.Errorf("StepStatus(%s).IsTerminal() = false, want true", s)
		}
	}

	for _, s := range nonTerminalStatuses {
		if s.IsTerminal() {
			t.Errorf("StepStatus(%s).IsTerminal() = true, want false", s)
		}
	}
}

func TestStepStatus_IsSuccess(t *testing.T) {
	successStatuses := []StepStatus{StatusPassed, StatusWarned}
	failureStatuses := []StepStatus{StatusPending, StatusRunning, StatusFailed, StatusErrored, StatusSkipped}

	for _, s := range successStatuses {
		if !s.IsSuccess() {
			t.Errorf("StepStatus(%s).IsSuccess() = false, want true", s)
		}
	}

	for _, s := range failureStatuses {
		if s.IsSuccess() {
			t.Errorf("StepStatus(%s).IsSuccess() = true, want false", s)
		}
	}
}

func TestErrorCategory_String(t *testing.T) {
	tests := []struct {
		category ErrorCategory
		expected string
	}{
		{ErrCategoryNone, "none"},
		{ErrCategoryAssertion, "assertion"},
		{ErrCategoryTimeout, "timeout"},
		{ErrCategoryConnection, "connection"},
		{ErrCategoryApp, "app"},
		{ErrCategoryConfig, "config"},
		{ErrorCategory(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.category.String(); got != tt.expected {
			t.Errorf("ErrorCategory(%d).String() = %q, want %q", tt.category, got, tt.expected)
		}
	}
}
