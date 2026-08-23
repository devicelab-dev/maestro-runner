package cloud

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoopback_GatedByEnvAndURL(t *testing.T) {
	t.Setenv("MAESTRO_CLOUD_DEBUG", "")
	if p := newLoopback("http://localhost:4723/wd/hub"); p != nil {
		t.Fatalf("expected nil without MAESTRO_CLOUD_DEBUG; got %T", p)
	}

	t.Setenv("MAESTRO_CLOUD_DEBUG", "1")
	if p := newLoopback("https://ondemand.us-west-1.saucelabs.com/wd/hub"); p != nil {
		t.Fatalf("expected nil for non-local URL; got %T", p)
	}
	for _, u := range []string{
		"http://localhost:4723/wd/hub",
		"http://127.0.0.1:4723/wd/hub",
		"http://USER:KEY@LOCALHOST:4723/wd/hub",
	} {
		if p := newLoopback(u); p == nil {
			t.Errorf("expected loopback provider for %q", u)
		}
	}
}

func TestLoopback_ExtractMeta_FallsBackToSyntheticJobID(t *testing.T) {
	p := &loopbackProvider{}

	meta := map[string]string{}
	p.ExtractMeta("appium-session-xyz", nil, meta)
	if meta["jobID"] != "appium-session-xyz" {
		t.Errorf("jobID = %q, want sessionID passthrough", meta["jobID"])
	}
	if meta["type"] != "loopback" {
		t.Errorf("type = %q, want %q", meta["type"], "loopback")
	}

	meta = map[string]string{}
	p.ExtractMeta("", nil, meta)
	if !strings.HasPrefix(meta["jobID"], "loopback-") {
		t.Errorf("jobID = %q, want loopback-N synthetic id", meta["jobID"])
	}
}

func TestLoopback_ReportResult_WritesPerJobJSON(t *testing.T) {
	tmp := t.TempDir()
	p := &loopbackProvider{}

	meta := map[string]string{"jobID": "session-A", "type": "loopback"}
	result := &TestResult{
		Passed:      false,
		Total:       2,
		PassedCount: 1,
		FailedCount: 1,
		Duration:    1234,
		OutputDir:   tmp,
		Flows: []FlowResult{
			{Name: "Flow A", File: "a.yaml", Passed: true, Duration: 500},
			{Name: "Flow B", File: "b.yaml", Passed: false, Duration: 734, Error: "boom"},
		},
	}
	if err := p.ReportResult("http://localhost:4723", meta, result); err != nil {
		t.Fatalf("ReportResult: %v", err)
	}

	path := filepath.Join(tmp, "loopback-cloud-session-A.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected per-job JSON file at %s: %v", path, err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["jobID"] != "session-A" {
		t.Errorf("jobID in payload = %v, want session-A", got["jobID"])
	}
	res, ok := got["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("result missing/wrong type: %T", got["result"])
	}
	flows, ok := res["Flows"].([]interface{})
	if !ok || len(flows) != 2 {
		t.Fatalf("flows = %v, want 2 entries", flows)
	}
}

func TestLoopback_LifecycleHooks_AppendJSONL(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("MAESTRO_LOOPBACK_OUT", tmp)
	p := &loopbackProvider{}

	meta := map[string]string{"jobID": "session-A", "type": "loopback"}
	if err := p.OnRunStart(meta, 2); err != nil {
		t.Fatalf("OnRunStart: %v", err)
	}
	if err := p.OnFlowStart(meta, 0, 2, "Flow A", "a.yaml"); err != nil {
		t.Fatalf("OnFlowStart: %v", err)
	}
	if err := p.OnFlowEnd(meta, &FlowResult{Name: "Flow A", File: "a.yaml", Passed: true, Duration: 100}); err != nil {
		t.Fatalf("OnFlowEnd: %v", err)
	}
	if err := p.ReportResult("http://localhost:4723", meta, &TestResult{
		Passed: true, Total: 1, PassedCount: 1, OutputDir: tmp,
		Flows: []FlowResult{{Name: "Flow A", File: "a.yaml", Passed: true, Duration: 100}},
	}); err != nil {
		t.Fatalf("ReportResult: %v", err)
	}

	jsonl := filepath.Join(tmp, "loopback-cloud-session-A.events.jsonl")
	data, err := os.ReadFile(jsonl)
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("event count = %d, want 4 (OnRunStart, OnFlowStart, OnFlowEnd, ReportResult); lines=%q", len(lines), lines)
	}

	wantEvents := []string{"OnRunStart", "OnFlowStart", "OnFlowEnd", "ReportResult"}
	for i, line := range lines {
		var ev map[string]interface{}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d not valid JSON: %v: %s", i, err, line)
		}
		if ev["event"] != wantEvents[i] {
			t.Errorf("line %d event = %v, want %s", i, ev["event"], wantEvents[i])
		}
		if ev["jobID"] != "session-A" {
			t.Errorf("line %d jobID = %v, want session-A", i, ev["jobID"])
		}
		if _, ok := ev["ts"].(string); !ok {
			t.Errorf("line %d missing ts field", i)
		}
	}
}

func TestLoopback_SanitizeJobID(t *testing.T) {
	if got := sanitizeJobID("abc-123_XYZ"); got != "abc-123_XYZ" {
		t.Errorf("sanitizeJobID safe input = %q, want unchanged", got)
	}
	if got := sanitizeJobID("a/b\\c d"); got != "a_b_c_d" {
		t.Errorf("sanitizeJobID = %q, want %q", got, "a_b_c_d")
	}
}
