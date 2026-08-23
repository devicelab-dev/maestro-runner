// This file is a skeleton for adding a new cloud provider.
// Copy this file, rename it, and implement the TODOs.
//
// Steps:
//  1. Copy to <provider>.go (e.g., browserstack.go)
//  2. Replace "example" with your provider name
//  3. Implement the URL match in the factory
//  4. Implement ExtractMeta and ReportResult
//  5. Add tests in <provider>_test.go
//  6. Add docs in docs/cloud-providers/<provider>.md

package cloud

/*
import (
	"fmt"
	"strings"
)

func init() {
	Register(newExampleProvider)
}

func newExampleProvider(appiumURL string) Provider {
	// TODO: match your provider's Appium hub URL
	if !strings.Contains(strings.ToLower(appiumURL), "example") {
		return nil
	}
	return &exampleProvider{}
}

type exampleProvider struct{}

func (p *exampleProvider) Name() string { return "Example" }

func (p *exampleProvider) ExtractMeta(sessionID string, caps map[string]interface{}, meta map[string]string) {
	// TODO: extract provider-specific data from session
	// Most providers just need the session ID as the job ID:
	meta["jobID"] = sessionID
}

func (p *exampleProvider) OnRunStart(meta map[string]string, totalFlows int) error {
	// TODO (optional): signal run start to your provider's dashboard
	// (e.g., create a suite, tag the job with totalFlows, etc.)
	return nil
}

func (p *exampleProvider) OnFlowStart(meta map[string]string, flowIdx, totalFlows int, name, file string) error {
	// TODO (optional): mark test case as "started" in your provider's dashboard
	// flowIdx is 0-based, totalFlows is the count, name is the flow title
	return nil
}

func (p *exampleProvider) OnFlowEnd(meta map[string]string, result *FlowResult) error {
	// TODO (optional): mark test case as completed (pass/fail) in your
	// provider's dashboard. Useful for live updates before the run ends.
	// result.Name, result.File, result.Passed, result.Duration, result.Error
	return nil
}

func (p *exampleProvider) ReportResult(appiumURL string, meta map[string]string, result *TestResult) error {
	jobID := meta["jobID"]
	if jobID == "" {
		return fmt.Errorf("no job ID")
	}

	// TODO: extract credentials from appiumURL userinfo or env vars
	// TODO: PUT/PATCH pass/fail to your provider's REST API
	//
	// Available data:
	//   result.Passed      - overall pass/fail
	//   result.Total       - total flow count
	//   result.PassedCount - flows passed
	//   result.FailedCount - flows failed
	//   result.Duration    - total ms
	//   result.OutputDir   - path to log, reports, screenshots
	//   result.Flows       - per-flow name, file, pass/fail, duration, error

	return nil
}
*/
