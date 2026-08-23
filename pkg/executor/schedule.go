package executor

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// The work queue is a dynamic pull queue — workers take the next item as they
// free up — so the schedule is decided entirely by the order things go in.
// Enqueueing in file order lets the longest flow be picked up last, and a run
// cannot finish before its longest remaining flow does, so one slow flow at the
// end of the alphabet sets the wall clock for everyone.
//
// Longest-processing-time-first is the standard answer: sort descending, hand
// out in that order. It is what Flank and Tuist converged on, and with a pull
// queue only the sort is needed — no bin-packing, since the workers balance
// themselves.

// longestFirst returns flow indices ordered by descending known duration.
//
// Indices, not reordered flows: the index ties a work item to its slot in the
// results array and its place in the report, so the schedule may change while
// the numbering must not.
//
// Flows with no recorded duration are treated as the median of those that have
// one. Sorting them first would let an unknown flow monopolise a worker; last
// would risk starting a genuinely long new flow too late.
func longestFirst(flows []flow.Flow, durations map[string]int64) []int {
	order := make([]int, len(flows))
	for i := range order {
		order[i] = i
	}
	if len(durations) == 0 {
		return order
	}

	median := medianDuration(durations)
	weight := func(i int) int64 {
		if d, ok := durations[flows[i].SourcePath]; ok && d > 0 {
			return d
		}
		return median
	}

	// Stable, so equal weights keep file order and a run stays reproducible.
	sort.SliceStable(order, func(a, b int) bool {
		return weight(order[a]) > weight(order[b])
	})
	return order
}

// medianDuration returns the median of the recorded durations, which is the
// least-wrong guess for a flow that has never run.
func medianDuration(durations map[string]int64) int64 {
	vals := make([]int64, 0, len(durations))
	for _, d := range durations {
		if d > 0 {
			vals = append(vals, d)
		}
	}
	if len(vals) == 0 {
		return 0
	}
	sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })
	return vals[len(vals)/2]
}

// priorFlowDurations reads per-flow durations from the most recent report under
// outputDir, keyed by source file.
//
// Best-effort by design: no previous run, an unreadable or half-written report,
// a first run in a fresh directory — all yield nil, and the queue keeps file
// order. Scheduling is an optimisation and must never be able to fail a run.
//
// Must be called before the current run writes its own skeleton, which
// overwrites the index this reads.
func priorFlowDurations(outputDir string) map[string]int64 {
	path := latestReportIndex(outputDir)
	if path == "" {
		return nil
	}
	index, err := report.ReadIndex(path)
	if err != nil || index == nil {
		return nil
	}

	durations := make(map[string]int64, len(index.Flows))
	for _, f := range index.Flows {
		if f.SourceFile == "" || f.Duration == nil || *f.Duration <= 0 {
			continue
		}
		// Keep the longest observation when a flow ran more than once: the
		// point is to start the expensive work early, so overestimating a
		// flow costs less than underestimating it.
		if prev, ok := durations[f.SourceFile]; !ok || *f.Duration > prev {
			durations[f.SourceFile] = *f.Duration
		}
	}
	if len(durations) == 0 {
		return nil
	}
	return durations
}

// latestReportIndex finds the newest report.json under outputDir, covering both
// the flattened layout and the timestamped-subdirectory one.
func latestReportIndex(outputDir string) string {
	if outputDir == "" {
		return ""
	}

	var newest string
	var newestMod int64
	consider := func(path string) {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return
		}
		if mod := info.ModTime().UnixNano(); mod > newestMod {
			newest, newestMod = path, mod
		}
	}

	consider(filepath.Join(outputDir, "report.json"))
	if entries, err := os.ReadDir(outputDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				consider(filepath.Join(outputDir, e.Name(), "report.json"))
			}
		}
	}
	return newest
}
