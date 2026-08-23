package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// Re-running only what failed.
//
// A suite that fails three flows out of forty should not cost a full run to
// confirm a fix. --retry-failed reads the previous run's report and narrows the
// selection to the flows that failed, leaving every other flag — tags, paths,
// device — exactly as it was.
//
// The previous report is found rather than configured, because the output
// layout differs by flag: --flatten writes report.json straight into the output
// directory, while the default buries it in a timestamped subfolder. Asking the
// user which one they got would be asking them to know something the tool
// already knows.

// findPreviousReport locates the most recent report.json under baseDir.
//
// Checks the flattened layout first, then each subfolder. The report is the
// last file a run keeps touching, so the most recently modified report.json
// belongs to the most recent run — regardless of what the folder is called,
// which also covers a renamed or hand-made folder. Folder names (the sortable
// "2006-01-02_15-04-05" stamps the runner writes) break modification-time ties.
func findPreviousReport(baseDir string) (string, error) {
	direct := filepath.Join(baseDir, "report.json")
	if _, err := os.Stat(direct); err == nil {
		return direct, nil
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", fmt.Errorf("no previous run found in %s: %w", baseDir, err)
	}

	type candidate struct {
		path string
		name string
		mod  time.Time
	}
	var found []candidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(baseDir, e.Name(), "report.json")
		if info, err := os.Stat(p); err == nil {
			found = append(found, candidate{path: p, name: e.Name(), mod: info.ModTime()})
		}
	}
	if len(found) == 0 {
		return "", fmt.Errorf("no previous run found in %s — run the suite once before retrying its failures", baseDir)
	}

	sort.Slice(found, func(i, j int) bool {
		if !found[i].mod.Equal(found[j].mod) {
			return found[i].mod.After(found[j].mod)
		}
		return found[i].name > found[j].name
	})
	return found[0].path, nil
}

// failedFlowPaths returns the source files of every flow that did not pass in
// the given report.
//
// Anything not explicitly passed counts as failed, so a run cut short mid-flow —
// a crash, a timeout, Ctrl-C — leaves its unfinished flows in the retry set
// rather than silently dropping them.
func failedFlowPaths(index *report.Index) []string {
	var paths []string
	seen := make(map[string]bool)
	for _, f := range index.Flows {
		if f.Status == report.StatusPassed || f.Status == report.StatusSkipped {
			continue
		}
		if f.SourceFile == "" || seen[f.SourceFile] {
			continue
		}
		seen[f.SourceFile] = true
		paths = append(paths, f.SourceFile)
	}
	return paths
}

// selectFailedFlows narrows discovered to the flows that failed last time.
//
// Matching is on resolved absolute paths, since the report records whatever the
// caller passed on the command line and this run may name the same files
// differently. Failures whose file is no longer in the selection are reported
// rather than ignored: a flow that was deleted, renamed, or filtered out by a
// tag is a fact the caller wants to know, not a silently smaller run.
func selectFailedFlows(discovered, failed []string) (selected, missing []string) {
	failedSet := make(map[string]bool, len(failed))
	for _, f := range failed {
		failedSet[resolvePath(f)] = true
	}

	matched := make(map[string]bool, len(failed))
	for _, d := range discovered {
		abs := resolvePath(d)
		if failedSet[abs] {
			selected = append(selected, d)
			matched[abs] = true
		}
	}

	for _, f := range failed {
		if !matched[resolvePath(f)] {
			missing = append(missing, f)
		}
	}
	return selected, missing
}

// resolvePath makes a path comparable across runs. A path that cannot be
// resolved is cleaned and returned as-is so it still compares equal to itself.
func resolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// retryFailedSelection reads the previous report under baseDir and narrows
// discovered to the flows that failed there.
//
// Returns an empty selection when the previous run was clean — the caller
// treats that as success, because "nothing failed last time" is the outcome the
// flag exists to confirm, not an error.
func retryFailedSelection(baseDir string, discovered []string) (selected []string, err error) {
	reportPath, err := findPreviousReport(baseDir)
	if err != nil {
		return nil, err
	}

	index, err := report.ReadIndex(reportPath)
	if err != nil {
		return nil, fmt.Errorf("read previous report %s: %w", reportPath, err)
	}

	failed := failedFlowPaths(index)
	printSetupSuccess(fmt.Sprintf("Previous run: %s (%d failed)", reportPath, len(failed)))
	if len(failed) == 0 {
		return nil, nil
	}

	selected, missing := selectFailedFlows(discovered, failed)
	for _, m := range missing {
		printSetupWarning(fmt.Sprintf("Previously failed flow is no longer in the selection: %s", m))
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("none of the %d previously failed flow(s) are in the current selection — "+
			"pass the same flow paths (and tags) as the previous run", len(failed))
	}
	return selected, nil
}

// narrowToPreviousFailures filters already-parsed flows down to the ones that
// failed in the previous run under baseDir.
//
// An empty result means the previous run was clean; a previous run whose
// failures are all outside the current selection is an error, because running
// nothing would look like success while confirming nothing.
func narrowToPreviousFailures(baseDir string, flows []flow.Flow) ([]flow.Flow, error) {
	discovered := make([]string, len(flows))
	for i := range flows {
		discovered[i] = flows[i].SourcePath
	}

	selected, err := retryFailedSelection(baseDir, discovered)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, nil
	}

	keep := make(map[string]bool, len(selected))
	for _, s := range selected {
		keep[s] = true
	}
	var out []flow.Flow
	for i := range flows {
		if keep[flows[i].SourcePath] {
			out = append(out, flows[i])
		}
	}
	return out, nil
}
