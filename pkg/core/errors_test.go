// SPDX-FileCopyrightText: 2025 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package core

import (
	"errors"
	"strings"
	"testing"
)

func TestExecutionError_Error(t *testing.T) {
	err := &ExecutionError{
		Category: ErrCategoryAssertion,
		Code:     "test_error",
		Message:  "test message",
	}

	if got := err.Error(); got != "test message" {
		t.Errorf("Error() = %q, want %q", got, "test message")
	}
}

func TestExecutionError_ErrorWithCause(t *testing.T) {
	cause := errors.New("underlying error")
	err := &ExecutionError{
		Category: ErrCategoryAssertion,
		Code:     "test_error",
		Message:  "test message",
		Cause:    cause,
	}

	got := err.Error()
	if !strings.Contains(got, "test message") {
		t.Errorf("Error() = %q, should contain 'test message'", got)
	}
	if !strings.Contains(got, "underlying error") {
		t.Errorf("Error() = %q, should contain 'underlying error'", got)
	}
}

func TestExecutionError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &ExecutionError{
		Message: "wrapper",
		Cause:   cause,
	}

	if got := err.Unwrap(); got != cause {
		t.Errorf("Unwrap() = %v, want %v", got, cause)
	}
}

func TestExecutionError_WithCause(t *testing.T) {
	original := ErrElementNotFound
	cause := errors.New("custom cause")

	newErr := original.WithCause(cause)

	if newErr.Cause != cause {
		t.Error("WithCause() did not set cause")
	}
	if newErr.Code != original.Code {
		t.Error("WithCause() changed code")
	}
	if original.Cause != nil {
		t.Error("WithCause() modified original error")
	}
}

func TestExecutionError_WithMessage(t *testing.T) {
	original := ErrTimeout
	newErr := original.WithMessage("custom timeout message")

	if newErr.Message != "custom timeout message" {
		t.Errorf("Message = %q, want 'custom timeout message'", newErr.Message)
	}
	if newErr.Code != original.Code {
		t.Error("WithMessage() changed code")
	}
	if original.Message == "custom timeout message" {
		t.Error("WithMessage() modified original error")
	}
}

func TestExecutionError_WithDetails(t *testing.T) {
	original := &ExecutionError{
		Code:    "test",
		Message: "test",
		Details: map[string]interface{}{"existing": "value"},
	}

	newErr := original.WithDetails(map[string]interface{}{
		"selector": "#button",
		"timeout":  5000,
	})

	if newErr.Details["selector"] != "#button" {
		t.Error("WithDetails() did not add new details")
	}
	if newErr.Details["existing"] != "value" {
		t.Error("WithDetails() did not preserve existing details")
	}
	if _, ok := original.Details["selector"]; ok {
		t.Error("WithDetails() modified original error")
	}
}

func TestPredefinedErrors(t *testing.T) {
	tests := []struct {
		err      *ExecutionError
		category ErrorCategory
		code     string
	}{
		{ErrElementNotFound, ErrCategoryAssertion, "element_not_found"},
		{ErrElementNotVisible, ErrCategoryAssertion, "element_not_visible"},
		{ErrTextMismatch, ErrCategoryAssertion, "text_mismatch"},
		{ErrConditionNotMet, ErrCategoryAssertion, "condition_not_met"},
		{ErrTimeout, ErrCategoryTimeout, "timeout"},
		{ErrWaitTimeout, ErrCategoryTimeout, "wait_timeout"},
		{ErrDeviceDisconnected, ErrCategoryConnection, "device_disconnected"},
		{ErrServerUnreachable, ErrCategoryConnection, "server_unreachable"},
		{ErrAppCrashed, ErrCategoryApp, "app_crashed"},
		{ErrAppNotInstalled, ErrCategoryApp, "app_not_installed"},
		{ErrAppNotResponding, ErrCategoryApp, "app_not_responding"},
		{ErrInvalidConfig, ErrCategoryConfig, "invalid_config"},
		{ErrMissingRequired, ErrCategoryConfig, "missing_required"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if tt.err.Category != tt.category {
				t.Errorf("Category = %s, want %s", tt.err.Category, tt.category)
			}
			if tt.err.Code != tt.code {
				t.Errorf("Code = %s, want %s", tt.err.Code, tt.code)
			}
			if tt.err.Message == "" {
				t.Error("Message should not be empty")
			}
		})
	}
}

func TestNewExecutionError(t *testing.T) {
	err := NewExecutionError(ErrCategoryApp, "custom_error", "custom message")

	if err.Category != ErrCategoryApp {
		t.Errorf("Category = %s, want %s", err.Category, ErrCategoryApp)
	}
	if err.Code != "custom_error" {
		t.Errorf("Code = %s, want 'custom_error'", err.Code)
	}
	if err.Message != "custom message" {
		t.Errorf("Message = %s, want 'custom message'", err.Message)
	}
}

func TestExecutionError_ErrorsIs(t *testing.T) {
	cause := errors.New("root cause")
	err := ErrTimeout.WithCause(cause)

	if !errors.Is(err, cause) {
		t.Error("errors.Is() should find the cause")
	}
}
