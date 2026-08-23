package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func flowsWithPaths(paths ...string) []flow.Flow {
	out := make([]flow.Flow, len(paths))
	for i, p := range paths {
		out[i] = flow.Flow{SourcePath: p}
	}
	return out
}

func TestLongestFirst(t *testing.T) {
	flows := flowsWithPaths("a.yaml", "b.yaml", "c.yaml")

	t.Run("longest known flow is enqueued first", func(t *testing.T) {
		got := longestFirst(flows, map[string]int64{"a.yaml": 100, "b.yaml": 9000, "c.yaml": 500})
		want := []int{1, 2, 0} // b (9000), c (500), a (100)
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("order = %v, want %v", got, want)
			}
		}
	})

	t.Run("file order is kept when nothing is known", func(t *testing.T) {
		got := longestFirst(flows, nil)
		for i := range got {
			if got[i] != i {
				t.Fatalf("order = %v, want file order", got)
			}
		}
	})

	t.Run("every index appears exactly once", func(t *testing.T) {
		got := longestFirst(flows, map[string]int64{"b.yaml": 50})
		seen := map[int]bool{}
		for _, i := range got {
			if seen[i] {
				t.Fatalf("index %d enqueued twice: %v", i, got)
			}
			seen[i] = true
		}
		if len(seen) != len(flows) {
			t.Fatalf("enqueued %d of %d flows: %v", len(seen), len(flows), got)
		}
	})

	t.Run("ties keep file order, so runs stay reproducible", func(t *testing.T) {
		got := longestFirst(flows, map[string]int64{"a.yaml": 500, "b.yaml": 500, "c.yaml": 500})
		for i := range got {
			if got[i] != i {
				t.Fatalf("order = %v, want file order for equal weights", got)
			}
		}
	})

	t.Run("an unknown flow is weighted as the median, never last", func(t *testing.T) {
		// Weights: short=100, long=9000, unknown=median. Whatever the median
		// resolves to, the invariant is that the short flow goes last — an
		// unknown flow must never be scheduled behind a known-quick one.
		f := flowsWithPaths("short.yaml", "unknown.yaml", "long.yaml")
		got := longestFirst(f, map[string]int64{"short.yaml": 100, "long.yaml": 9000})
		if last := got[len(got)-1]; last != 0 {
			t.Errorf("order = %v, want the short flow (index 0) enqueued last", got)
		}
	})
}

func TestMedianDuration(t *testing.T) {
	if got := medianDuration(map[string]int64{"a": 10, "b": 20, "c": 30}); got != 20 {
		t.Errorf("median = %d, want 20", got)
	}
	if got := medianDuration(map[string]int64{}); got != 0 {
		t.Errorf("median of nothing = %d, want 0", got)
	}
	if got := medianDuration(map[string]int64{"a": 0, "b": 42}); got != 42 {
		t.Errorf("zero durations must be ignored, got %d", got)
	}
}

func writeIndex(t *testing.T, dir string, flows []map[string]any) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"flows": flows})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPriorFlowDurations(t *testing.T) {
	t.Run("reads a flattened report", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, []map[string]any{
			{"sourceFile": "a.yaml", "duration": 1200},
			{"sourceFile": "b.yaml", "duration": 300},
		})
		got := priorFlowDurations(dir)
		if got["a.yaml"] != 1200 || got["b.yaml"] != 300 {
			t.Fatalf("durations = %v", got)
		}
	})

	t.Run("keeps the longest observation of a repeated flow", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, []map[string]any{
			{"sourceFile": "a.yaml", "duration": 400},
			{"sourceFile": "a.yaml", "duration": 2500},
		})
		if got := priorFlowDurations(dir)["a.yaml"]; got != 2500 {
			t.Errorf("duration = %d, want the longest (2500)", got)
		}
	})

	t.Run("prefers the newest timestamped subdirectory", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, filepath.Join(dir, "2026-01-01_000000"), []map[string]any{
			{"sourceFile": "a.yaml", "duration": 111},
		})
		newer := filepath.Join(dir, "2026-06-01_000000")
		writeIndex(t, newer, []map[string]any{{"sourceFile": "a.yaml", "duration": 999}})
		// Make the ordering unambiguous regardless of filesystem timestamp
		// granularity.
		future := time.Now().Add(time.Hour)
		if err := os.Chtimes(filepath.Join(newer, "report.json"), future, future); err != nil {
			t.Fatal(err)
		}
		if got := priorFlowDurations(dir)["a.yaml"]; got != 999 {
			t.Errorf("duration = %d, want 999 from the newest report", got)
		}
	})

	t.Run("a missing or unreadable report never fails the run", func(t *testing.T) {
		if got := priorFlowDurations(t.TempDir()); got != nil {
			t.Errorf("expected nil for an empty directory, got %v", got)
		}
		if got := priorFlowDurations(""); got != nil {
			t.Errorf("expected nil for no output dir, got %v", got)
		}
		bad := t.TempDir()
		if err := os.WriteFile(filepath.Join(bad, "report.json"), []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := priorFlowDurations(bad); got != nil {
			t.Errorf("expected nil for a corrupt report, got %v", got)
		}
	})
}
