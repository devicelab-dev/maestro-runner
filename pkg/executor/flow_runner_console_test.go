package executor

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// consoleReportingMockDriver embeds mockDriver and implements
// consoleLogReporter so collectConsoleLogs picks it up.
type consoleReportingMockDriver struct {
	mockDriver
	logs    []report.ConsoleLog
	cleared int // count of times ClearConsoleLogReport was called
}

func (c *consoleReportingMockDriver) ConsoleLogReport() []report.ConsoleLog {
	return c.logs
}

func (c *consoleReportingMockDriver) ClearConsoleLogReport() {
	c.cleared++
	c.logs = nil
}

func TestCollectConsoleLogs_DriverImplementsInterface(t *testing.T) {
	d := &consoleReportingMockDriver{
		logs: []report.ConsoleLog{
			{Level: "error", Message: "boom"},
			{Level: "warning", Message: "hot stove"},
		},
	}

	got := collectConsoleLogs(d)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Level != "error" || got[0].Message != "boom" {
		t.Errorf("got[0] = %+v, want {error, boom}", got[0])
	}
}

// TestCollectConsoleLogs_DriverWithoutInterface verifies the fallback path:
// mobile / native drivers don't implement consoleLogReporter and the helper
// returns nil instead of panicking.
func TestCollectConsoleLogs_DriverWithoutInterface(t *testing.T) {
	d := &mockDriver{} // plain mock, does not implement ConsoleLogReport

	got := collectConsoleLogs(d)
	if got != nil {
		t.Errorf("expected nil for non-implementing driver, got %v", got)
	}
}

// TestCollectConsoleLogs_EmptyLogs verifies that a driver implementing the
// interface but with no captured entries returns an empty (or nil) slice
// rather than a phantom non-empty result.
func TestCollectConsoleLogs_EmptyLogs(t *testing.T) {
	d := &consoleReportingMockDriver{logs: nil}

	got := collectConsoleLogs(d)
	if len(got) != 0 {
		t.Errorf("expected empty result for empty buffer, got %v", got)
	}
}

// TestResetConsoleLogs_CallsClearer verifies that flow-start reset hits
// ClearConsoleLogReport on drivers that implement the interface.
func TestResetConsoleLogs_CallsClearer(t *testing.T) {
	d := &consoleReportingMockDriver{
		logs: []report.ConsoleLog{{Level: "error", Message: "pre-flow noise"}},
	}

	resetConsoleLogs(d)

	if d.cleared != 1 {
		t.Errorf("ClearConsoleLogReport called %d times, want 1", d.cleared)
	}
	if got := collectConsoleLogs(d); len(got) != 0 {
		t.Errorf("after reset, expected empty buffer, got %d entries", len(got))
	}
}

// TestResetConsoleLogs_NoopForNonImplementer verifies the helper doesn't
// panic on drivers without ClearConsoleLogReport (mobile / native).
func TestResetConsoleLogs_NoopForNonImplementer(t *testing.T) {
	d := &mockDriver{} // plain mock, no interface
	// Should not panic.
	resetConsoleLogs(d)
}

// TestJSErrorSummary_OnlyErrorsAndExceptions checks that the summary
// produced for failOnConsoleError includes only entries at error /
// exception level — not log / warn / info / debug. The threshold matches
// the existing `assertNoJSErrors` flow step.
func TestJSErrorSummary_OnlyErrorsAndExceptions(t *testing.T) {
	logs := []report.ConsoleLog{
		{Level: "log", Message: "i am noise"},
		{Level: "warning", Message: "i am louder noise"},
		{Level: "info", Message: "fine"},
		{Level: "error", Message: "real bug"},
		{Level: "exception", Message: "Error: kaboom"},
	}
	got := jsErrorSummary(logs)
	if got == "" {
		t.Fatal("expected non-empty summary, got empty")
	}
	for _, want := range []string{"real bug", "Error: kaboom", "2 JS error(s)"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q; got: %s", want, got)
		}
	}
	for _, unwanted := range []string{"i am noise", "i am louder noise", "fine"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("summary should not include non-error entry %q; got: %s", unwanted, got)
		}
	}
}

// TestJSErrorSummary_EmptyAndNoErrors verifies the function returns ""
// (no failure) when the buffer is empty or contains only non-error
// levels — preserves the default "don't fail" behaviour.
func TestJSErrorSummary_EmptyAndNoErrors(t *testing.T) {
	if got := jsErrorSummary(nil); got != "" {
		t.Errorf("expected empty for nil input, got %q", got)
	}
	if got := jsErrorSummary([]report.ConsoleLog{}); got != "" {
		t.Errorf("expected empty for empty input, got %q", got)
	}
	logsNoErrors := []report.ConsoleLog{
		{Level: "log", Message: "ok"},
		{Level: "warning", Message: "meh"},
		{Level: "info", Message: "fine"},
	}
	if got := jsErrorSummary(logsNoErrors); got != "" {
		t.Errorf("expected empty when no errors present, got %q", got)
	}
}
