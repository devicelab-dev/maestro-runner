package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// writeReport writes a report.json into dir with one entry per (sourceFile, status) pair.
func writeReport(t *testing.T, dir string, entries []report.FlowEntry) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := report.Index{Version: "1.0", Flows: entries}
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "report.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func entry(sourceFile string, status report.Status) report.FlowEntry {
	return report.FlowEntry{SourceFile: sourceFile, Status: status}
}

func TestFindPreviousReport_FlattenedLayout(t *testing.T) {
	base := t.TempDir()
	want := writeReport(t, base, nil)

	got, err := findPreviousReport(base)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestFindPreviousReport_PicksNewestSubfolder(t *testing.T) {
	base := t.TempDir()
	old := writeReport(t, filepath.Join(base, "2026-08-10_10-00-00"), nil)
	newest := writeReport(t, filepath.Join(base, "2026-08-11_09-00-00"), nil)

	// Modification times decide, not names — backdate the older report.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	got, err := findPreviousReport(base)
	if err != nil {
		t.Fatal(err)
	}
	if got != newest {
		t.Errorf("got %s, want %s", got, newest)
	}
}

func TestFindPreviousReport_NewestByModTimeDespiteName(t *testing.T) {
	base := t.TempDir()
	// Lexically "later" folder holds the older report — a renamed folder.
	renamed := writeReport(t, filepath.Join(base, "zzz-renamed"), nil)
	newest := writeReport(t, filepath.Join(base, "2026-08-11_09-00-00"), nil)

	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(renamed, past, past); err != nil {
		t.Fatal(err)
	}

	got, err := findPreviousReport(base)
	if err != nil {
		t.Fatal(err)
	}
	if got != newest {
		t.Errorf("got %s, want %s", got, newest)
	}
}

func TestFindPreviousReport_NoPreviousRun(t *testing.T) {
	base := t.TempDir()
	// A subfolder without a report (e.g. the run being started right now)
	// does not count as a previous run.
	if err := os.MkdirAll(filepath.Join(base, "2026-08-11_09-00-00"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := findPreviousReport(base); err == nil {
		t.Error("expected an error when no report.json exists")
	}
	if _, err := findPreviousReport(filepath.Join(base, "does-not-exist")); err == nil {
		t.Error("expected an error for a missing base directory")
	}
}

func TestFailedFlowPaths(t *testing.T) {
	index := &report.Index{Flows: []report.FlowEntry{
		entry("a.yaml", report.StatusPassed),
		entry("b.yaml", report.StatusFailed),
		entry("c.yaml", report.StatusSkipped),
		entry("d.yaml", report.StatusRunning), // run cut short mid-flow
		entry("e.yaml", report.StatusPending), // run cut short before starting
		entry("b.yaml", report.StatusFailed),  // duplicate source file
		entry("", report.StatusFailed),        // no source recorded
	}}

	got := failedFlowPaths(index)
	want := []string{"b.yaml", "d.yaml", "e.yaml"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
}

func TestSelectFailedFlows_MatchesAcrossPathSpellings(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	b := filepath.Join(dir, "b.yaml")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("appId: x\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// The report recorded absolute paths; this run names the same files
	// relative to the current directory.
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	selected, missing := selectFailedFlows([]string{"a.yaml", "b.yaml"}, []string{a})
	if len(selected) != 1 || selected[0] != "a.yaml" {
		t.Errorf("selected = %v, want [a.yaml]", selected)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
}

func TestSelectFailedFlows_ReportsMissing(t *testing.T) {
	selected, missing := selectFailedFlows(
		[]string{"/flows/a.yaml"},
		[]string{"/flows/a.yaml", "/flows/deleted.yaml"},
	)
	if len(selected) != 1 {
		t.Errorf("selected = %v, want one entry", selected)
	}
	if len(missing) != 1 || missing[0] != "/flows/deleted.yaml" {
		t.Errorf("missing = %v, want [/flows/deleted.yaml]", missing)
	}
}

func TestRetryFailedSelection_CleanPreviousRun(t *testing.T) {
	base := t.TempDir()
	writeReport(t, base, []report.FlowEntry{
		entry("a.yaml", report.StatusPassed),
		entry("b.yaml", report.StatusPassed),
	})

	selected, err := retryFailedSelection(base, []string{"a.yaml", "b.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if selected != nil {
		t.Errorf("selected = %v, want nil for a clean previous run", selected)
	}
}

func TestRetryFailedSelection_AllFailuresMissingIsAnError(t *testing.T) {
	base := t.TempDir()
	writeReport(t, base, []report.FlowEntry{
		entry("/flows/deleted.yaml", report.StatusFailed),
	})

	_, err := retryFailedSelection(base, []string{"/flows/other.yaml"})
	if err == nil {
		t.Fatal("expected an error when no previous failure is in the current selection")
	}
	if !strings.Contains(err.Error(), "previously failed") {
		t.Errorf("error should explain the empty overlap, got: %v", err)
	}
}

func TestNarrowToPreviousFailures(t *testing.T) {
	base := t.TempDir()
	a := filepath.Join(base, "a.yaml")
	b := filepath.Join(base, "b.yaml")
	writeReport(t, filepath.Join(base, "2026-08-11_09-00-00"), []report.FlowEntry{
		entry(a, report.StatusPassed),
		entry(b, report.StatusFailed),
	})

	flows := []flow.Flow{{SourcePath: a}, {SourcePath: b}}
	got, err := narrowToPreviousFailures(base, flows)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SourcePath != b {
		t.Errorf("got %d flow(s), want just %s", len(got), b)
	}
}
