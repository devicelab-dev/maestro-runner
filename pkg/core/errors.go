// SPDX-FileCopyrightText: 2024–2026 DeviceLab
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package core

import (
	"fmt"
)

// ExecutionError represents a structured error with category and details
type ExecutionError struct {
	Category ErrorCategory
	Code     string                 // Machine-readable code: element_not_found, timeout, etc.
	Message  string                 // Human-readable message
	Details  map[string]interface{} // Additional context
	Cause    error                  // Underlying error
}

// Error implements the error interface
func (e *ExecutionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the underlying error for errors.Is/As support
func (e *ExecutionError) Unwrap() error {
	return e.Cause
}

// WithCause returns a copy of the error with the given cause
func (e *ExecutionError) WithCause(cause error) *ExecutionError {
	return &ExecutionError{
		Category: e.Category,
		Code:     e.Code,
		Message:  e.Message,
		Details:  e.Details,
		Cause:    cause,
	}
}

// WithMessage returns a copy of the error with a custom message
func (e *ExecutionError) WithMessage(msg string) *ExecutionError {
	return &ExecutionError{
		Category: e.Category,
		Code:     e.Code,
		Message:  msg,
		Details:  e.Details,
		Cause:    e.Cause,
	}
}

// WithDetails returns a copy of the error with additional details
func (e *ExecutionError) WithDetails(details map[string]interface{}) *ExecutionError {
	merged := make(map[string]interface{})
	for k, v := range e.Details {
		merged[k] = v
	}
	for k, v := range details {
		merged[k] = v
	}
	return &ExecutionError{
		Category: e.Category,
		Code:     e.Code,
		Message:  e.Message,
		Details:  merged,
		Cause:    e.Cause,
	}
}

// Predefined errors (like Appium W3C error codes)
var (
	// Assertion errors
	ErrElementNotFound = &ExecutionError{
		Category: ErrCategoryAssertion,
		Code:     "element_not_found",
		Message:  "element not found",
	}
	ErrElementNotVisible = &ExecutionError{
		Category: ErrCategoryAssertion,
		Code:     "element_not_visible",
		Message:  "element not visible",
	}
	ErrTextMismatch = &ExecutionError{
		Category: ErrCategoryAssertion,
		Code:     "text_mismatch",
		Message:  "text does not match expected value",
	}
	ErrConditionNotMet = &ExecutionError{
		Category: ErrCategoryAssertion,
		Code:     "condition_not_met",
		Message:  "condition was not met",
	}

	// Timeout errors
	ErrTimeout = &ExecutionError{
		Category: ErrCategoryTimeout,
		Code:     "timeout",
		Message:  "operation timed out",
	}
	ErrWaitTimeout = &ExecutionError{
		Category: ErrCategoryTimeout,
		Code:     "wait_timeout",
		Message:  "wait condition timed out",
	}

	// Connection errors
	ErrDeviceDisconnected = &ExecutionError{
		Category: ErrCategoryConnection,
		Code:     "device_disconnected",
		Message:  "device connection lost",
	}
	ErrServerUnreachable = &ExecutionError{
		Category: ErrCategoryConnection,
		Code:     "server_unreachable",
		Message:  "could not connect to automation server",
	}

	// App errors
	ErrAppCrashed = &ExecutionError{
		Category: ErrCategoryApp,
		Code:     "app_crashed",
		Message:  "application crashed",
	}
	ErrAppNotInstalled = &ExecutionError{
		Category: ErrCategoryApp,
		Code:     "app_not_installed",
		Message:  "application is not installed",
	}
	ErrAppNotResponding = &ExecutionError{
		Category: ErrCategoryApp,
		Code:     "app_not_responding",
		Message:  "application is not responding",
	}

	// Config errors
	ErrInvalidConfig = &ExecutionError{
		Category: ErrCategoryConfig,
		Code:     "invalid_config",
		Message:  "invalid configuration",
	}
	ErrMissingRequired = &ExecutionError{
		Category: ErrCategoryConfig,
		Code:     "missing_required",
		Message:  "missing required field",
	}
)

// NewExecutionError creates a new ExecutionError with the given parameters
func NewExecutionError(category ErrorCategory, code, message string) *ExecutionError {
	return &ExecutionError{
		Category: category,
		Code:     code,
		Message:  message,
	}
}
