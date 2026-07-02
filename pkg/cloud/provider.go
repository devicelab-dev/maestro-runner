// Package cloud provides an abstraction for cloud device providers
// (Sauce Labs, BrowserStack, LambdaTest, TestingBot, etc.).
//
// Each provider registers itself via init(). The Detect function
// selects the matching provider from the Appium URL.
package cloud

import "sync"

// Common metadata keys populated by Appium-backed cloud runs.
const (
	MetaAppiumURL = "appiumURL"
	MetaSessionID = "sessionID"
	MetaDeviceID  = "deviceID"
)

// Provider abstracts cloud device provider operations.
type Provider interface {
	// Name returns the human-readable provider name (e.g., "Sauce Labs").
	Name() string

	// ExtractMeta extracts provider-specific metadata from the Appium session.
	// Called once after the session is created.
	// sessionID is the WebDriver session ID; caps are the merged capabilities
	// from the session response; meta is the output map to populate.
	ExtractMeta(sessionID string, caps map[string]interface{}, meta map[string]string)

	// OnRunStart fires once, after ExtractMeta and before the first flow runs.
	// Providers can use this to record run-level metadata (tags, build ID, total
	// flow count) with the upstream dashboard. Errors are logged but do not
	// abort the run.
	OnRunStart(meta map[string]string, totalFlows int) error

	// OnFlowStart fires before each flow begins executing. Providers can use
	// this to mark a test case as "started" in the upstream dashboard. Errors
	// are logged but do not abort the flow.
	OnFlowStart(meta map[string]string, flowIdx, totalFlows int, name, file string) error

	// OnFlowEnd fires after each flow finishes (pass, fail, or skip).
	// Providers can use this to mark a test case as completed in the upstream
	// dashboard before the full run has finished. Errors are logged but do not
	// abort subsequent flows.
	OnFlowEnd(meta map[string]string, result *FlowResult) error

	// ReportResult reports the test result to the cloud provider.
	// Called once after all flows and reports have completed.
	ReportResult(appiumURL string, meta map[string]string, result *TestResult) error
}

// TestResult contains the test run outcome passed to cloud providers.
type TestResult struct {
	Passed      bool
	Total       int
	PassedCount int
	FailedCount int
	Duration    int64  // total milliseconds
	OutputDir   string // path to log, reports, screenshots
	Flows       []FlowResult
}

// FlowResult contains the outcome of a single flow.
type FlowResult struct {
	Name     string
	File     string // source YAML file path
	Passed   bool
	Duration int64 // milliseconds
	Error    string
}

// ProviderFactory returns a Provider if the Appium URL matches, or nil.
type ProviderFactory func(appiumURL string) Provider

var (
	mu        sync.RWMutex
	factories []ProviderFactory
)

// Register adds a provider factory to the registry.
// Called from init() in each provider implementation file.
func Register(f ProviderFactory) {
	mu.Lock()
	defer mu.Unlock()
	factories = append(factories, f)
}

// Detect returns the first Provider matching the Appium URL, or nil.
func Detect(appiumURL string) Provider {
	mu.RLock()
	defer mu.RUnlock()
	for _, f := range factories {
		if p := f(appiumURL); p != nil {
			return p
		}
	}
	return nil
}
