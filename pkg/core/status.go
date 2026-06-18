// SPDX-FileCopyrightText: 2024–2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package core

// StepStatus represents the execution status of a step
type StepStatus int

// StepStatus values
const (
	StatusPending StepStatus = iota // Not yet started
	StatusRunning                   // Currently executing
	StatusPassed                    // Completed successfully
	StatusFailed                    // Assertion failed (expected behavior didn't occur)
	StatusErrored                   // Unexpected error (infrastructure, timeout, crash)
	StatusSkipped                   // Condition not met or previous step failed
	StatusWarned                    // Optional step failed (non-blocking)
)

// String returns the string representation of StepStatus
func (s StepStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusPassed:
		return "passed"
	case StatusFailed:
		return "failed"
	case StatusErrored:
		return "errored"
	case StatusSkipped:
		return "skipped"
	case StatusWarned:
		return "warned"
	default:
		return "unknown"
	}
}

// IsTerminal returns true if the status is a final state
func (s StepStatus) IsTerminal() bool {
	switch s {
	case StatusPassed, StatusFailed, StatusErrored, StatusSkipped, StatusWarned:
		return true
	default:
		return false
	}
}

// IsSuccess returns true if the status indicates success (passed or warned)
func (s StepStatus) IsSuccess() bool {
	return s == StatusPassed || s == StatusWarned
}

// ErrorCategory classifies the type of error for better debugging and reporting
type ErrorCategory int

// ErrorCategory values
const (
	ErrCategoryNone       ErrorCategory = iota // No error
	ErrCategoryAssertion                       // Element not found, text mismatch, visibility check failed
	ErrCategoryTimeout                         // Operation timed out
	ErrCategoryConnection                      // Device/server connection lost
	ErrCategoryApp                             // App crashed, not responding, not installed
	ErrCategoryConfig                          // Invalid configuration, missing required field
)

// String returns the string representation of ErrorCategory
func (c ErrorCategory) String() string {
	switch c {
	case ErrCategoryNone:
		return "none"
	case ErrCategoryAssertion:
		return "assertion"
	case ErrCategoryTimeout:
		return "timeout"
	case ErrCategoryConnection:
		return "connection"
	case ErrCategoryApp:
		return "app"
	case ErrCategoryConfig:
		return "config"
	default:
		return "unknown"
	}
}
