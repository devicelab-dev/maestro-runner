package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/urfave/cli/v2"
)

var lintCommand = &cli.Command{
	Name:      "lint",
	Usage:     "Check flow files for syntax errors without running them",
	ArgsUsage: "<flow.yaml | directory> [...]",
	Description: `Parse flow files and report syntax errors, without a device.

Every flow is parsed with the same parser the test runner uses, so anything
that would fail at the start of a run is caught here in milliseconds instead —
useful in CI as a pre-flight gate, in an editor save hook, or before handing a
generated flow to a device.

Directories are walked recursively for .yaml/.yml files. Exits non-zero if any
flow fails to parse, so it drops straight into a CI step.

Examples:
  maestro-runner lint flows/
  maestro-runner lint login.yaml checkout.yaml
  maestro-runner lint --quiet flows/`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "quiet",
			Usage: "Only print failures, not the per-file OK lines",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "Emit JSON instead of per-file lines",
		},
	},
	Action: runLint,
}

func runLint(c *cli.Context) error {
	targets := c.Args().Slice()
	if len(targets) == 0 {
		targets = []string{"."}
	}
	if c.Bool("json") {
		return runLintJSON(targets)
	}
	return runLintPaths(targets, c.Bool("quiet"))
}

// LintResult is one file's verdict. The JSON tags are a public contract —
// editors and CI read this shape.
type LintResult struct {
	File  string `json:"file"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// LintReport is the whole --json payload.
type LintReport struct {
	Checked int          `json:"checked"`
	Failed  int          `json:"failed"`
	Results []LintResult `json:"results"`
}

// runLintJSON writes the report to stdout as JSON. Unlike the text form it
// keeps everything on stdout — a JSON consumer wants one stream, and the exit
// code already signals failure.
func runLintJSON(targets []string) error {
	report, err := lintPathsReport(targets)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if encErr := enc.Encode(report); encErr != nil {
		return encErr
	}
	if report.Failed > 0 {
		return cli.Exit("", 1)
	}
	return nil
}

// lintPathsReport parses every target and collects the verdicts.
func lintPathsReport(targets []string) (*LintReport, error) {
	paths, err := collectFlowPaths(targets)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no flow files found in %s", strings.Join(targets, ", "))
	}

	report := &LintReport{Checked: len(paths), Results: make([]LintResult, 0, len(paths))}
	for _, path := range paths {
		if _, perr := flow.ParseFile(path); perr != nil {
			report.Failed++
			report.Results = append(report.Results, LintResult{File: path, Error: perr.Error()})
			continue
		}
		report.Results = append(report.Results, LintResult{File: path, OK: true})
	}
	return report, nil
}

// runLintPaths is runLint without the cli.Context, so the parse-and-report
// behaviour can be tested directly.
func runLintPaths(targets []string, quiet bool) error {
	paths, err := collectFlowPaths(targets)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no flow files found in %s", strings.Join(targets, ", "))
	}

	var failures int
	for _, path := range paths {
		if _, perr := flow.ParseFile(path); perr != nil {
			failures++
			// Errors go to stderr so `lint` can be piped for its file list
			// without the failures contaminating it.
			fmt.Fprintf(os.Stderr, "✗ %s\n  %v\n", path, perr)
			continue
		}
		if !quiet {
			fmt.Printf("✓ %s\n", path)
		}
	}

	fmt.Printf("\n%d flow(s) checked, %d failed\n", len(paths), failures)
	if failures > 0 {
		// cli.Exit sets the process exit code without printing a second error
		// line — the per-file diagnostics above are the useful output.
		return cli.Exit("", 1)
	}
	return nil
}

// collectFlowPaths expands the arguments into a sorted, de-duplicated list of
// flow files. A file argument is taken as-is even if its extension is unusual —
// naming a file explicitly is a clear request to check that file — while a
// directory contributes only .yaml/.yml, walked recursively.
func collectFlowPaths(targets []string) ([]string, error) {
	seen := make(map[string]bool)
	var paths []string

	add := func(p string) {
		if abs, err := filepath.Abs(p); err == nil {
			if seen[abs] {
				return
			}
			seen[abs] = true
		}
		paths = append(paths, p)
	}

	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %w", target, err)
		}
		if !info.IsDir() {
			add(target)
			continue
		}
		walkErr := filepath.Walk(target, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				return nil
			}
			if ext := strings.ToLower(filepath.Ext(p)); ext == ".yaml" || ext == ".yml" {
				add(p)
			}
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("walk %s: %w", target, walkErr)
		}
	}

	sort.Strings(paths)
	return paths, nil
}
