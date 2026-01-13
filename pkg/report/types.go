// Package report provides JSON-based test reporting with real-time updates.
//
// Architecture:
//   - report.json: Main index file (small, frequently updated, mutex-protected)
//   - flows/flow-XXX.json: Per-flow detail files (no lock needed)
//   - assets/flow-XXX/: Per-flow artifacts (screenshots, videos, logs)
//
// The index file serves as single source of truth for status and change tracking.
// Consumers poll report.json and only fetch changed flow details as needed.
package report

import "time"

// Version is the report schema version.
const Version = "1.0.0"

// Status represents the execution status.
type Status string

// Status values.
const (
	StatusPending Status = "pending"
	StatusRunning Status = "running"
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

// IsTerminal returns true if the status is a final state.
func (s Status) IsTerminal() bool {
	return s == StatusPassed || s == StatusFailed || s == StatusSkipped
}

// ============================================================================
// INDEX (report.json)
// ============================================================================

// Index is the main report file that binds everything together.
// It contains minimal info for efficient polling and change detection.
type Index struct {
	Version       string      `json:"version"`
	UpdateSeq     uint64      `json:"updateSeq"`
	Status        Status      `json:"status"`
	StartTime     time.Time   `json:"startTime"`
	EndTime       *time.Time  `json:"endTime,omitempty"`
	LastUpdated   time.Time   `json:"lastUpdated"`
	Device        Device      `json:"device"`
	App           App         `json:"app"`
	CI            *CI         `json:"ci,omitempty"`
	MaestroRunner RunnerInfo  `json:"maestroRunner"`
	Summary       Summary     `json:"summary"`
	Flows         []FlowEntry `json:"flows"`
}

// Device contains device information.
type Device struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Platform    string `json:"platform"` // ios, android
	OSVersion   string `json:"osVersion"`
	Model       string `json:"model,omitempty"`
	IsSimulator bool   `json:"isSimulator"`
}

// App contains application information.
type App struct {
	ID      string `json:"id"` // Bundle ID or package name
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// CI contains CI/CD build information.
type CI struct {
	Provider      string `json:"provider,omitempty"`
	BuildID       string `json:"buildId,omitempty"`
	BuildURL      string `json:"buildUrl,omitempty"`
	Branch        string `json:"branch,omitempty"`
	Commit        string `json:"commit,omitempty"`
	CommitMessage string `json:"commitMessage,omitempty"`
}

// RunnerInfo contains maestro-runner information.
type RunnerInfo struct {
	Version string `json:"version"`
	Driver  string `json:"driver"` // appium, native, detox
}

// Summary contains aggregated counts.
type Summary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	Running int `json:"running"`
	Pending int `json:"pending"`
}

// FlowEntry is the index entry for a flow (minimal info).
type FlowEntry struct {
	Index          int            `json:"index"`      // Original position
	ID             string         `json:"id"`         // Unique flow ID
	Name           string         `json:"name"`       // Display name
	SourceFile     string         `json:"sourceFile"` // Path to YAML file
	DataFile       string         `json:"dataFile"`   // Path to flow detail JSON
	AssetsDir      string         `json:"assetsDir"`  // Path to assets directory
	Status         Status         `json:"status"`
	UpdateSeq      uint64         `json:"updateSeq"`
	StartTime      *time.Time     `json:"startTime,omitempty"`
	EndTime        *time.Time     `json:"endTime,omitempty"`
	Duration       *int64         `json:"duration,omitempty"` // milliseconds
	LastUpdated    *time.Time     `json:"lastUpdated,omitempty"`
	Commands       CommandSummary `json:"commands"`
	Attempts       int            `json:"attempts"`
	AttemptHistory []AttemptEntry `json:"attemptHistory,omitempty"`
	Error          *string        `json:"error,omitempty"`
}

// CommandSummary contains command counts for a flow.
type CommandSummary struct {
	Total   int  `json:"total"`
	Passed  int  `json:"passed"`
	Failed  int  `json:"failed"`
	Skipped int  `json:"skipped"`
	Running int  `json:"running"`
	Pending int  `json:"pending"`
	Current *int `json:"current,omitempty"` // Currently running command index
}

// AttemptEntry tracks retry attempts.
type AttemptEntry struct {
	Attempt  int    `json:"attempt"`
	DataFile string `json:"dataFile"`
	Status   Status `json:"status"`
	Duration int64  `json:"duration"` // milliseconds
	Error    string `json:"error,omitempty"`
}

// ============================================================================
// FLOW DETAIL (flows/flow-XXX.json)
// ============================================================================

// FlowDetail contains full flow execution details.
type FlowDetail struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	SourceFile string        `json:"sourceFile"`
	Tags       []string      `json:"tags,omitempty"`
	Device     *Device       `json:"device,omitempty"` // Device that ran this flow (for multi-device runs)
	StartTime  time.Time     `json:"startTime"`
	EndTime    *time.Time    `json:"endTime,omitempty"`
	Duration   *int64        `json:"duration,omitempty"` // milliseconds
	Commands   []Command     `json:"commands"`
	Artifacts  FlowArtifacts `json:"artifacts"`
}

// Command represents a single command execution.
type Command struct {
	ID          string           `json:"id"`
	Index       int              `json:"index"`
	Type        string           `json:"type"`
	Label       string           `json:"label,omitempty"` // Human-readable description from YAML label field
	YAML        string           `json:"yaml,omitempty"`
	Status      Status           `json:"status"`
	StartTime   *time.Time       `json:"startTime,omitempty"`
	EndTime     *time.Time       `json:"endTime,omitempty"`
	Duration    *int64           `json:"duration,omitempty"` // milliseconds
	Params      *CommandParams   `json:"params,omitempty"`
	Element     *Element         `json:"element,omitempty"`
	Error       *Error           `json:"error,omitempty"`
	Artifacts   CommandArtifacts `json:"artifacts"`
	SubCommands []Command        `json:"subCommands,omitempty"` // For runFlow, repeat, retry
}

// CommandParams contains command-specific parameters.
type CommandParams struct {
	Selector  *Selector `json:"selector,omitempty"`
	Text      string    `json:"text,omitempty"`
	Direction string    `json:"direction,omitempty"`
	Timeout   int       `json:"timeout,omitempty"`
}

// Selector represents an element selector.
type Selector struct {
	Type     string `json:"type"` // id, text, accessibilityId, xpath, class, index
	Value    string `json:"value"`
	Optional bool   `json:"optional,omitempty"`
}

// Element contains information about the found element.
type Element struct {
	Found  bool    `json:"found"`
	ID     string  `json:"id,omitempty"`
	Text   string  `json:"text,omitempty"`
	Class  string  `json:"class,omitempty"`
	Bounds *Bounds `json:"bounds,omitempty"`
}

// Bounds represents element bounds.
type Bounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Error contains error details.
type Error struct {
	Type       string `json:"type"` // assertion, timeout, element_not_found, app_crash, network, unknown
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

// ============================================================================
// ARTIFACTS (paths only, never inline data)
// ============================================================================

// FlowArtifacts contains flow-level artifact paths.
type FlowArtifacts struct {
	Video           string           `json:"video,omitempty"`
	VideoTimestamps []VideoTimestamp `json:"videoTimestamps,omitempty"`
	DeviceLog       string           `json:"deviceLog,omitempty"`
	AppLog          string           `json:"appLog,omitempty"`
}

// VideoTimestamp maps command index to video time.
type VideoTimestamp struct {
	CommandIndex int   `json:"commandIndex"`
	VideoTimeMs  int64 `json:"videoTimeMs"`
}

// CommandArtifacts contains command-level artifact paths.
type CommandArtifacts struct {
	ScreenshotBefore string `json:"screenshotBefore,omitempty"`
	ScreenshotAfter  string `json:"screenshotAfter,omitempty"`
	ViewHierarchy    string `json:"viewHierarchy,omitempty"`
}

// ============================================================================
// UPDATE TYPES
// ============================================================================

// FlowUpdate contains the fields to update in index for a flow.
type FlowUpdate struct {
	Status    Status
	StartTime *time.Time
	EndTime   *time.Time
	Duration  *int64
	Commands  CommandSummary
	Error     *string
}
