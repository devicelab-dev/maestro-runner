package cdp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// consoleNoisyPage emits log/warn/error/exception during page load. Mirrors
// the manual repro we used to validate the feature end-to-end.
func consoleNoisyPage() string {
	return `<!DOCTYPE html><html><body>
<h1>TestPage</h1>
<script>
console.error("boom: something is on fire");
console.warn("hot stove");
console.log("hi");
throw new Error("uncaught explosion");
</script>
</body></html>`
}

func newConsoleNoisyServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, consoleNoisyPage())
	})
	return httptest.NewServer(mux)
}

// TestConsoleLogReport_CapturesAndConverts verifies the CDP driver's
// ConsoleLogReport() method returns the captured browser console entries in
// the shape the report package expects. End-to-end: real Chromium → real
// CDP Runtime events → our handler → our conversion.
func TestConsoleLogReport_CapturesAndConverts(t *testing.T) {
	ts := newConsoleNoisyServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Page load triggers all 4 console events synchronously during script
	// execution. assertVisible doubles as a settle wait.
	res := d.Execute(&flow.AssertVisibleStep{Selector: flow.Selector{Text: "TestPage"}})
	if !res.Success {
		t.Fatalf("page did not render: %s", res.Message)
	}

	logs := d.ConsoleLogReport()
	if len(logs) == 0 {
		t.Fatal("ConsoleLogReport returned no entries — capture path is broken")
	}

	// Build {level → seen} map. Page emits log/warn/error/exception once
	// each; events may be observed more than once depending on Runtime.enable
	// timing (a pre-existing dedupe issue tracked separately), so we assert
	// each level is *present* rather than counting exact occurrences.
	seen := map[string]string{}
	for _, e := range logs {
		seen[e.Level] = e.Message
	}

	checks := []struct {
		level, wantSubstr string
	}{
		{"error", "boom"},
		{"warning", "hot stove"},
		{"log", "hi"},
		{"exception", "uncaught explosion"},
	}
	for _, c := range checks {
		msg, ok := seen[c.level]
		if !ok {
			t.Errorf("expected an entry at level %q; got levels: %v", c.level, keys(seen))
			continue
		}
		if !contains(msg, c.wantSubstr) {
			t.Errorf("level %q: message %q did not contain %q", c.level, msg, c.wantSubstr)
		}
	}
}

// TestConsoleLogReport_EmptyByDefault verifies a clean page (no console
// noise) yields an empty result, not a phantom slice.
func TestConsoleLogReport_EmptyByDefault(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	logs := d.ConsoleLogReport()
	if len(logs) != 0 {
		t.Errorf("expected empty ConsoleLogReport on quiet page, got %d entries: %v", len(logs), logs)
	}
}

// TestConsoleLogReport_ShapeMatchesReportType is a compile-time + runtime
// guard that the driver's return type matches the report package's
// ConsoleLog struct. The interface assertion in pkg/executor relies on this.
func TestConsoleLogReport_ShapeMatchesReportType(t *testing.T) {
	d := &Driver{
		consoleLogs: []ConsoleEntry{
			{Level: "error", Message: "x"},
		},
	}
	got := d.ConsoleLogReport()
	var _ []report.ConsoleLog = got // compile-time check
	if len(got) != 1 || got[0].Level != "error" || got[0].Message != "x" {
		t.Errorf("unexpected conversion result: %+v", got)
	}
}

// TestFlowReport_AutoSurfacesConsoleErrors_EndToEnd is the regression test
// preserving the manual verification I ran during implementation: a flow
// that loads a console-noisy page should produce a per-flow report file
// whose `consoleLogs` field contains the captured entries — *without* the
// flow explicitly invoking `getConsoleLogs`.
//
// This test does NOT use the full Runner+executor pipeline (which would
// require setting up a complete RunnerConfig with index writers etc).
// Instead it verifies the read path: driver captures → ConsoleLogReport
// returns → report.FlowDetail.ConsoleLogs round-trips through JSON.
func TestFlowReport_AutoSurfacesConsoleErrors_EndToEnd(t *testing.T) {
	ts := newConsoleNoisyServer()
	defer ts.Close()

	d := newTestDriver(t, ts.URL)
	defer d.Close()

	// Trigger page load + capture.
	if res := d.Execute(&flow.AssertVisibleStep{
		Selector: flow.Selector{Text: "TestPage"},
	}); !res.Success {
		t.Fatalf("page load failed: %s", res.Message)
	}

	// Build a FlowDetail using the same path the runner would (driver →
	// SetConsoleLogs → JSON file).
	detail := report.FlowDetail{
		ID:          "flow-test",
		Name:        "console-noisy",
		SourceFile:  "synthetic",
		ConsoleLogs: d.ConsoleLogReport(),
	}

	// Round-trip through JSON the way the report writer does.
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var roundTripped report.FlowDetail
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &roundTripped); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(roundTripped.ConsoleLogs) == 0 {
		t.Fatal("expected consoleLogs in round-tripped JSON, got empty")
	}

	// Smoke-check that the JSON literally contains the field name (regression
	// guard for json tag breakage).
	if !contains(string(raw), `"consoleLogs"`) {
		t.Errorf("expected JSON to contain `consoleLogs` field, got: %s", string(raw))
	}
	if !contains(string(raw), "boom") {
		t.Errorf("expected JSON to contain `boom` message, got: %s", string(raw))
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// contains is a substring helper to avoid importing strings just for tests.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
