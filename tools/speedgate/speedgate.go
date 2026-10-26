package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// Baseline is the committed record of how long each command type takes.
//
// Medians are per command type, pooled over every passed step of every run
// the baseline was written from. The driver and platform are recorded so a
// comparison against reports from another driver is flagged: a tapOn on
// DeviceLab Android and on WDA are different numbers, and comparing them says
// nothing about a regression.
type Baseline struct {
	Version  int                      `json:"version"`
	Driver   string                   `json:"driver,omitempty"`
	Platform string                   `json:"platform,omitempty"`
	Runs     int                      `json:"runs"`
	Commands map[string]BaselineEntry `json:"commands"`
}

// BaselineEntry is one command type's median and how many steps it came from.
type BaselineEntry struct {
	MedianMs int64 `json:"medianMs"`
	Samples  int   `json:"samples"`
}

// Samples holds the durations read from a set of reports.
type Samples struct {
	Driver   string
	Platform string
	Runs     int
	ByType   map[string][]int64
}

// Thresholds decide when a slower median counts as a regression. Both must be
// exceeded: the percentage alone flags a 10 ms step that became 13 ms, the
// absolute alone flags a 5 s step that drifted by 60 ms.
type Thresholds struct {
	Pct        float64 // allowed slowdown, percent of the baseline median
	AbsMs      int64   // allowed slowdown, milliseconds
	MinSamples int     // command types with fewer samples are not judged
}

// notInBaseline marks a command type the reports have and the baseline does
// not: a new command, or one too rare to be baselined.
const notInBaseline = "not in baseline"

// Result is the comparison for one command type.
type Result struct {
	Type       string
	BaseMs     int64
	CurMs      int64
	Samples    int
	Regressed  bool
	Unjudged   string // why the type was not judged; empty when it was
	DeltaMs    int64
	DeltaPct   float64
	MissingCur bool // in the baseline, absent from the reports
}

// LoadSamples reads each report (a report directory, or a path to its
// report.json) and collects the duration of every passed leaf command by type.
//
// Only passed steps count: a failed step's duration is usually a timeout, and
// would make the median measure the flow's failures rather than the runner's
// speed. Container steps (runFlow, repeat, retry) are skipped and their
// children counted, as report.ComputeStepLatency does, so work is not counted
// twice.
func LoadSamples(paths []string) (*Samples, error) {
	s := &Samples{ByType: map[string][]int64{}}
	for _, p := range paths {
		dir := p
		if filepath.Base(p) == "report.json" {
			dir = filepath.Dir(p)
		}
		index, flows, err := report.ReadReport(dir)
		if err != nil {
			return nil, fmt.Errorf("read report %s: %w", p, err)
		}
		if err := s.noteRunner(index); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		s.Runs++
		for _, f := range flows {
			collect(f.Commands, s.ByType)
		}
	}
	return s, nil
}

// noteRunner records the driver and platform, refusing to pool reports from
// different ones.
func (s *Samples) noteRunner(index *report.Index) error {
	driver, platform := index.MaestroRunner.Driver, index.Device.Platform
	if s.Runs == 0 {
		s.Driver, s.Platform = driver, platform
		return nil
	}
	if driver != s.Driver || platform != s.Platform {
		return fmt.Errorf("report is %s/%s, earlier reports are %s/%s; pool runs of one driver and platform",
			platform, driver, s.Platform, s.Driver)
	}
	return nil
}

func collect(cmds []report.Command, into map[string][]int64) {
	for _, c := range cmds {
		if len(c.SubCommands) > 0 {
			collect(c.SubCommands, into)
			continue
		}
		if c.Status != report.StatusPassed || c.Duration == nil || c.Type == "" {
			continue
		}
		into[c.Type] = append(into[c.Type], *c.Duration)
	}
}

// median returns the median of the values; for an even count, the lower of
// the two middle values, so the result is a duration some step actually took.
func median(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[(len(sorted)-1)/2]
}

// BuildBaseline turns samples into a baseline. Types with fewer than
// minSamples steps are left out: their median is noise.
func BuildBaseline(s *Samples, minSamples int) *Baseline {
	b := &Baseline{
		Version:  1,
		Driver:   s.Driver,
		Platform: s.Platform,
		Runs:     s.Runs,
		Commands: map[string]BaselineEntry{},
	}
	for typ, d := range s.ByType {
		if len(d) < minSamples {
			continue
		}
		b.Commands[typ] = BaselineEntry{MedianMs: median(d), Samples: len(d)}
	}
	return b
}

// ReadBaseline loads a baseline file.
func ReadBaseline(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse baseline %s: %w", path, err)
	}
	if b.Commands == nil {
		b.Commands = map[string]BaselineEntry{}
	}
	return &b, nil
}

// WriteBaseline writes a baseline file, indented and with sorted keys so a
// re-baseline diffs cleanly in review.
func WriteBaseline(path string, b *Baseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Compare judges every command type in the baseline or the samples. Results
// are sorted by type.
func Compare(base *Baseline, s *Samples, th Thresholds) []Result {
	types := map[string]bool{}
	for t := range base.Commands {
		types[t] = true
	}
	for t := range s.ByType {
		types[t] = true
	}
	names := make([]string, 0, len(types))
	for t := range types {
		names = append(names, t)
	}
	sort.Strings(names)

	results := make([]Result, 0, len(names))
	for _, t := range names {
		be, inBase := base.Commands[t]
		cur := s.ByType[t]
		r := Result{Type: t, BaseMs: be.MedianMs, Samples: len(cur)}
		switch {
		case len(cur) == 0:
			r.MissingCur = true
			r.Unjudged = "not in these reports"
		case !inBase:
			r.CurMs = median(cur)
			r.Unjudged = notInBaseline
		case len(cur) < th.MinSamples:
			r.CurMs = median(cur)
			r.Unjudged = fmt.Sprintf("only %d sample(s), need %d", len(cur), th.MinSamples)
		default:
			r.CurMs = median(cur)
			r.DeltaMs = r.CurMs - r.BaseMs
			if r.BaseMs > 0 {
				r.DeltaPct = float64(r.DeltaMs) * 100 / float64(r.BaseMs)
			}
			r.Regressed = r.DeltaMs > th.AbsMs && float64(r.DeltaMs) > float64(r.BaseMs)*th.Pct/100
		}
		results = append(results, r)
	}
	return results
}

// Regressions counts the regressed results.
func Regressions(results []Result) int {
	n := 0
	for _, r := range results {
		if r.Regressed {
			n++
		}
	}
	return n
}

// PrintResults writes a table of the comparison.
func PrintResults(w io.Writer, results []Result) {
	fmt.Fprintf(w, "%-24s %10s %10s %9s %8s  %s\n", "COMMAND", "BASE ms", "NOW ms", "DELTA ms", "DELTA %", "VERDICT")
	for _, r := range results {
		verdict := "ok"
		switch {
		case r.Regressed:
			verdict = "REGRESSED"
		case r.Unjudged != "":
			verdict = "skipped: " + r.Unjudged
		}
		base, cur, delta, pct := "-", "-", "-", "-"
		if r.Unjudged != notInBaseline {
			base = fmt.Sprint(r.BaseMs)
		}
		if !r.MissingCur {
			cur = fmt.Sprint(r.CurMs)
		}
		if r.Unjudged == "" {
			delta = fmt.Sprintf("%+d", r.DeltaMs)
			pct = fmt.Sprintf("%+.1f", r.DeltaPct)
		}
		fmt.Fprintf(w, "%-24s %10s %10s %9s %8s  %s\n", r.Type, base, cur, delta, pct, verdict)
	}
}
