package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// flowRetries runs flows that failed again, up to Retries more times, in the
// same run: the driver and the on-device agent stay up, and the report keeps
// every attempt. CI harnesses used to loop over the runner themselves, parse
// report.json and start a new process per round.
type flowRetries struct {
	outputDir string
	retries   int
	details   []report.FlowDetail
	pristine  [][]byte // each flow's detail before its first attempt
	attempts  []int
}

// newFlowRetries snapshots the flow details before they run; nil when
// retries are off.
func newFlowRetries(cfg RunnerConfig, details []report.FlowDetail) *flowRetries {
	if cfg.Retries <= 0 {
		return nil
	}
	fr := &flowRetries{
		outputDir: cfg.OutputDir,
		retries:   cfg.Retries,
		details:   details,
		pristine:  make([][]byte, len(details)),
		attempts:  make([]int, len(details)),
	}
	for i := range details {
		data, err := json.Marshal(details[i])
		if err != nil {
			logger.Warn("retries: snapshot %s: %v", details[i].ID, err)
		}
		fr.pristine[i] = data
	}
	return fr
}

// run reruns the failed flows round by round. rerun runs the given flow
// indexes and writes their results into results.
func (fr *flowRetries) run(ctx context.Context, results []FlowResult, indexWriter *report.IndexWriter, rerun func(indexes []int)) {
	for i := range results {
		if results[i].Status != report.StatusSkipped {
			fr.attempts[i] = 1
		}
	}
	for round := 1; round <= fr.retries && ctx.Err() == nil; round++ {
		failed := failedFlowIndexes(results)
		if len(failed) == 0 {
			break
		}
		fmt.Printf("\n  %s↻ Retrying %d failed flow(s) (retry %d of %d)%s\n",
			color(colorCyan), len(failed), round, fr.retries, color(colorReset))
		logger.Info("Retry %d of %d: %d failed flow(s)", round, fr.retries, len(failed))
		for _, i := range failed {
			archived := fr.archive(i, fr.attempts[i])
			indexWriter.RecordAttempt(fr.details[i].ID, fr.attempts[i], results[i].Status, results[i].Duration, results[i].Error, archived)
			fr.restore(i)
			fr.attempts[i]++
		}
		rerun(failed)
	}
	// The last attempt of a retried flow is the flow's own data file.
	for i := range results {
		if fr.attempts[i] > 1 {
			indexWriter.RecordAttempt(fr.details[i].ID, fr.attempts[i], results[i].Status, results[i].Duration, results[i].Error,
				filepath.Join("flows", fr.details[i].ID+".json"))
		}
		results[i].Attempts = fr.attempts[i]
	}
}

func failedFlowIndexes(results []FlowResult) []int {
	var failed []int
	for i := range results {
		if results[i].Status == report.StatusFailed {
			failed = append(failed, i)
		}
	}
	return failed
}

// archive keeps a finished attempt next to the flow's files, as
// flows/<id>.attempt-<n>.json and assets/<id>.attempt-<n>/, before the next
// attempt overwrites them. Returns the archived data file, relative to the
// output directory ("" when there was nothing to keep).
func (fr *flowRetries) archive(i, attempt int) string {
	id := fr.details[i].ID
	suffix := fmt.Sprintf(".attempt-%d", attempt)
	assets := filepath.Join(fr.outputDir, "assets", id)
	if _, err := os.Stat(assets); err == nil {
		if err := os.Rename(assets, assets+suffix); err != nil {
			logger.Warn("retries: keep assets of %s attempt %d: %v", id, attempt, err)
		}
	}
	dataFile := filepath.Join("flows", id+".json")
	data, err := os.ReadFile(filepath.Join(fr.outputDir, dataFile))
	if err != nil {
		return ""
	}
	// Artifact paths in the detail point into the renamed assets folder.
	from, to := filepath.ToSlash(filepath.Join("assets", id))+"/", filepath.ToSlash(filepath.Join("assets", id+suffix))+"/"
	data = []byte(strings.ReplaceAll(string(data), from, to))
	archived := filepath.Join("flows", id+suffix+".json")
	if err := os.WriteFile(filepath.Join(fr.outputDir, archived), data, 0o644); err != nil {
		logger.Warn("retries: keep detail of %s attempt %d: %v", id, attempt, err)
		return ""
	}
	return archived
}

// restore puts a flow's detail back as it was before its first attempt.
func (fr *flowRetries) restore(i int) {
	if fr.pristine[i] == nil {
		return
	}
	device := fr.details[i].Device
	var fresh report.FlowDetail
	if err := json.Unmarshal(fr.pristine[i], &fresh); err != nil {
		logger.Warn("retries: restore %s: %v", fr.details[i].ID, err)
		return
	}
	fresh.Device = device
	fr.details[i] = fresh
}
