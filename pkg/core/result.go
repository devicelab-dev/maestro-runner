// SPDX-FileCopyrightText: 2024–2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

package core

import (
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// StepResult captures the complete outcome of executing a single step
type StepResult struct {
	// Identity
	Step    flow.Step `json:"-"`       // Reference to the step definition
	Index   int       `json:"index"`   // 0-based position in flow
	Command string    `json:"command"` // Command type: tapOn, assertVisible, etc.

	// Execution context
	ExecutedBy ExecutedBy `json:"executedBy"` // driver or runner

	// Status
	Status   StepStatus    `json:"status"`
	Category ErrorCategory `json:"errorCategory,omitempty"`

	// Timing
	StartTime time.Time     `json:"startTime"`
	Duration  time.Duration `json:"duration"`

	// Output
	Message string       `json:"message,omitempty"` // Human-readable explanation
	Element *ElementInfo `json:"element,omitempty"` // Element interacted with
	Data    interface{}  `json:"data,omitempty"`    // Command-specific data (including expected/actual for assertions)

	// Error Details
	Error string `json:"error,omitempty"` // Technical error message

	// Retry Tracking
	Attempt     int      `json:"attempt"`               // Current attempt (1-based)
	MaxAttempts int      `json:"maxAttempts"`           // Configured max retries + 1
	RetryErrors []string `json:"retryErrors,omitempty"` // Errors from previous attempts
	Flaky       bool     `json:"flaky,omitempty"`       // True if passed after retry

	// Debug Artifacts
	Logs        []LogEntry   `json:"logs,omitempty"`        // Logs captured during step
	Attachments []Attachment `json:"attachments,omitempty"` // Screenshots, hierarchy, etc.
	Debug       interface{}  `json:"-"`                     // Internal debug info (not serialized)

	// Nested results (for runFlow, repeat, retry)
	SubFlowResult *FlowResult  `json:"subFlowResult,omitempty"` // For runFlow
	Iterations    []StepResult `json:"iterations,omitempty"`    // For repeat loops
}

// FlowResult captures the complete outcome of executing a flow
type FlowResult struct {
	// Identity
	Name     string   `json:"name"`
	FilePath string   `json:"filePath"`
	Tags     []string `json:"tags,omitempty"`

	// Platform info (captured once per flow)
	PlatformInfo *PlatformInfo `json:"platformInfo,omitempty"`

	// Status (aggregated from steps)
	Status StepStatus `json:"status"`

	// Timing
	StartTime time.Time     `json:"startTime"`
	Duration  time.Duration `json:"duration"`

	// Results
	Steps          []StepResult `json:"steps"`
	OnFlowStart    []StepResult `json:"onFlowStart,omitempty"`
	OnFlowComplete []StepResult `json:"onFlowComplete,omitempty"`
	OnFlowFailure  []StepResult `json:"onFlowFailure,omitempty"` // Runs when flow fails

	// Summary (computed)
	TotalSteps   int `json:"totalSteps"`
	PassedSteps  int `json:"passedSteps"`
	FailedSteps  int `json:"failedSteps"`
	SkippedSteps int `json:"skippedSteps"`
	WarnedSteps  int `json:"warnedSteps"`
	FlakySteps   int `json:"flakySteps,omitempty"` // Steps that passed after retry

	// Error info (if flow failed)
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// ComputeSummary calculates step counts from the Steps slice
func (f *FlowResult) ComputeSummary() {
	f.TotalSteps = len(f.Steps)
	f.PassedSteps = 0
	f.FailedSteps = 0
	f.SkippedSteps = 0
	f.WarnedSteps = 0
	f.FlakySteps = 0

	for _, step := range f.Steps {
		switch step.Status {
		case StatusPassed:
			f.PassedSteps++
		case StatusFailed, StatusErrored:
			f.FailedSteps++
		case StatusSkipped:
			f.SkippedSteps++
		case StatusWarned:
			f.WarnedSteps++
		}
		if step.Flaky {
			f.FlakySteps++
		}
	}
}

// hasFailure checks if any step in the slice has failed or errored
func hasFailure(steps []StepResult) bool {
	for _, step := range steps {
		if step.Status == StatusFailed || step.Status == StatusErrored {
			return true
		}
	}
	return false
}

// hasWarning checks if any step in the slice has warned status
func hasWarning(steps []StepResult) bool {
	for _, step := range steps {
		if step.Status == StatusWarned {
			return true
		}
	}
	return false
}

// AggregateStatus determines the flow status from step results
// Rules:
// - Any failed/errored step (non-optional) → StatusFailed
// - All passed (with optional warned) → StatusPassed
// - onFlowStart failed → StatusFailed (steps skipped)
func (f *FlowResult) AggregateStatus() StepStatus {
	if hasFailure(f.OnFlowStart) || hasFailure(f.Steps) || hasFailure(f.OnFlowComplete) {
		return StatusFailed
	}
	if hasWarning(f.Steps) {
		return StatusWarned
	}
	return StatusPassed
}

// SuiteResult captures the complete outcome of executing multiple flows
type SuiteResult struct {
	// Identity
	Name  string `json:"name"`
	RunID string `json:"runId"` // Unique execution ID (timestamp or UUID)

	// Timing
	StartTime time.Time     `json:"startTime"`
	Duration  time.Duration `json:"duration"`

	// Results
	Flows []FlowResult `json:"flows"`

	// Summary
	TotalFlows   int `json:"totalFlows"`
	PassedFlows  int `json:"passedFlows"`
	FailedFlows  int `json:"failedFlows"`
	SkippedFlows int `json:"skippedFlows"`
	FlakyFlows   int `json:"flakyFlows,omitempty"` // Flows with flaky steps
}

// ComputeSummary calculates flow counts from the Flows slice
func (s *SuiteResult) ComputeSummary() {
	s.TotalFlows = len(s.Flows)
	s.PassedFlows = 0
	s.FailedFlows = 0
	s.SkippedFlows = 0
	s.FlakyFlows = 0

	for _, flow := range s.Flows {
		switch flow.Status {
		case StatusPassed, StatusWarned:
			s.PassedFlows++
		case StatusFailed, StatusErrored:
			s.FailedFlows++
		case StatusSkipped:
			s.SkippedFlows++
		}
		if flow.FlakySteps > 0 {
			s.FlakyFlows++
		}
	}
}

// Success returns true if all flows passed (including warned)
func (s *SuiteResult) Success() bool {
	for _, flow := range s.Flows {
		if !flow.Status.IsSuccess() {
			return false
		}
	}
	return len(s.Flows) > 0
}
