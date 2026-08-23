package report

import "sort"

// StepLatency summarises how long individual commands took across a run.
//
// Wall-clock totals hide the shape of a regression: a run that got 20% slower
// because one command became pathological looks the same as one where every
// command drifted. Percentiles separate those, and being in the report means CI
// can gate on them instead of noticing months later. Durations are milliseconds.
type StepLatency struct {
	Count int   `json:"count"`
	Mean  int64 `json:"mean"`
	P50   int64 `json:"p50"`
	P95   int64 `json:"p95"`
	Max   int64 `json:"max"`
	// SlowestType is the command type holding Max — the first thing you want
	// to know when a threshold trips.
	SlowestType string `json:"slowestType,omitempty"`
}

// ComputeStepLatency walks commands (including nested sub-commands of runFlow,
// repeat and retry) and summarises their durations.
//
// Container commands are excluded: a `repeat` block's duration is the sum of
// its children, so counting both would double-count the work and drag the
// percentiles toward the containers. Commands with no recorded duration are
// skipped rather than counted as zero.
func ComputeStepLatency(commands []Command) StepLatency {
	var (
		durations   []int64
		total       int64
		max         int64
		slowestType string
	)

	var walk func(cmds []Command)
	walk = func(cmds []Command) {
		for _, c := range cmds {
			if len(c.SubCommands) > 0 {
				walk(c.SubCommands)
				continue
			}
			if c.Duration == nil {
				continue
			}
			d := *c.Duration
			durations = append(durations, d)
			total += d
			if d > max {
				max, slowestType = d, c.Type
			}
		}
	}
	walk(commands)

	if len(durations) == 0 {
		return StepLatency{}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	return StepLatency{
		Count:       len(durations),
		Mean:        total / int64(len(durations)),
		P50:         percentile(durations, 50),
		P95:         percentile(durations, 95),
		Max:         max,
		SlowestType: slowestType,
	}
}

// percentile returns the nearest-rank percentile of a sorted slice. Nearest
// rank rather than interpolation: these are observed step durations, so a
// reported p95 should be a duration some step actually took.
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := (p*len(sorted) + 99) / 100 // ceil(p/100 * n)
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}
