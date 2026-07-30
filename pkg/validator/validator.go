// Package validator validates Maestro flow files before execution.
// It parses all files upfront, resolves runFlow references, and detects errors.
package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/config"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// ValidationError represents a validation error with context.
type ValidationError struct {
	File    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.File, e.Message)
}

// Result contains the validation result.
type Result struct {
	// TestCases is the list of top-level test case file paths.
	TestCases []string
	// Errors contains all validation errors found.
	Errors []error
}

// IsValid returns true if there are no validation errors.
func (r *Result) IsValid() bool {
	return len(r.Errors) == 0
}

// Validator validates flow files.
type Validator struct {
	includeTags []string
	excludeTags []string
}

// New creates a new Validator.
func New(includeTags, excludeTags []string) *Validator {
	return &Validator{
		includeTags: includeTags,
		excludeTags: excludeTags,
	}
}

// Validate validates a file or directory.
// It parses all flows, resolves runFlow references, and returns validation results.
func (v *Validator) Validate(path string) *Result {
	result := &Result{}

	info, err := os.Stat(path)
	if err != nil {
		result.Errors = append(result.Errors, &ValidationError{
			File:    path,
			Message: fmt.Sprintf("cannot access: %v", err),
		})
		return result
	}

	var testCases []string
	if info.IsDir() {
		testCases, err = v.collectTestCases(path)
		if err != nil {
			result.Errors = append(result.Errors, &ValidationError{
				File:    path,
				Message: fmt.Sprintf("failed to collect test cases: %v", err),
			})
			return result
		}
	} else {
		testCases = []string{path}
	}

	// Validate each test case and resolve dependencies
	validated := make(map[string]bool)
	testCasesAdded := make(map[string]bool)
	for _, file := range testCases {
		v.validateFile(file, result, validated, testCasesAdded, nil, true)
	}

	return result
}

// collectTestCases finds test case files based on config.yaml or top-level files.
func (v *Validator) collectTestCases(dir string) ([]string, error) {
	// Try to load config.yaml (may not exist)
	cfg, _ := config.LoadFromDir(dir)

	// Determine flow patterns
	patterns := []string{"*"} // Default: top-level files only

	if cfg != nil {
		// Merge config tags with validator tags
		if len(cfg.ExcludeTags) > 0 {
			v.excludeTags = append(v.excludeTags, cfg.ExcludeTags...)
		}
		if len(cfg.IncludeTags) > 0 {
			v.includeTags = append(v.includeTags, cfg.IncludeTags...)
		}
		if len(cfg.Flows) > 0 {
			patterns = cfg.Flows
		}
	}

	// Collect files matching patterns
	return v.collectByPatterns(dir, patterns)
}

// collectByPatterns collects flow files matching glob patterns.
func (v *Validator) collectByPatterns(dir string, patterns []string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		matches, err := v.matchPattern(dir, pattern)
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			if !seen[match] {
				seen[match] = true
				files = append(files, match)
			}
		}
	}

	return files, nil
}

// matchPattern matches a glob pattern and returns flow files.
func (v *Validator) matchPattern(dir, pattern string) ([]string, error) {
	var files []string

	// filepath.Glob does not understand "**", so route any pattern that
	// contains it through the recursive walker.
	if strings.Contains(pattern, "**") {
		return v.collectRecursive(dir, pattern)
	}

	// Standard glob matching
	fullPattern := filepath.Join(dir, pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, err
	}

	// Check if pattern explicitly references subdirectories (e.g., "auth/*")
	patternHasSubdir := strings.Contains(pattern, "/")

	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}

		if info.IsDir() {
			// Only recurse into directories if pattern explicitly includes subdirs
			// e.g., "auth/*" should get files from auth/, but "*" should skip directories
			if patternHasSubdir {
				dirFiles, err := v.getTopLevelFlows(match)
				if err != nil {
					return nil, err
				}
				files = append(files, dirFiles...)
			}
			// Skip directories for patterns like "*" (top-level files only)
		} else if isFlowFile(match) {
			files = append(files, match)
		}
	}

	return files, nil
}

// collectRecursive collects flow files matching a pattern that contains "**".
// The pattern is split on the first "**" into an optional prefix (walked from)
// and an optional suffix (matched per-segment against each file's trailing
// path). E.g. "tests/**/*.yaml" walks from dir/tests and keeps files whose
// name matches "*.yaml". A prefix containing shell wildcards is expanded via
// filepath.Glob so patterns like "flows-*/**/*.yaml" walk each matching
// directory. A prefix that resolves to no existing directory is a silent
// no-match.
func (v *Validator) collectRecursive(dir, pattern string) ([]string, error) {
	idx := strings.Index(pattern, "**")
	if idx < 0 {
		return nil, fmt.Errorf("collectRecursive called without ** in pattern %q", pattern)
	}
	prefix := strings.TrimSuffix(pattern[:idx], "/")
	suffix := strings.TrimPrefix(pattern[idx+2:], "/")

	roots, err := expandPrefixRoots(dir, prefix)
	if err != nil {
		return nil, err
	}

	var files []string
	seen := make(map[string]bool)
	for _, root := range roots {
		matches, err := walkRecursive(root, suffix)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				files = append(files, m)
			}
		}
	}
	return files, nil
}

// expandPrefixRoots resolves the "**" prefix into concrete walk roots. If the
// prefix contains no wildcards, the single joined path is returned (or nothing
// if it doesn't exist). If the prefix contains "*"/"?"/"[", filepath.Glob is
// used to expand it and only the resulting directories are returned.
func expandPrefixRoots(dir, prefix string) ([]string, error) {
	if prefix == "" {
		return []string{dir}, nil
	}

	joined := filepath.Join(dir, prefix)
	if !strings.ContainsAny(prefix, "*?[") {
		if _, err := os.Stat(joined); err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		return []string{joined}, nil
	}

	matches, err := filepath.Glob(joined)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(matches))
	for _, m := range matches {
		info, statErr := os.Stat(m)
		if statErr != nil || !info.IsDir() {
			continue
		}
		roots = append(roots, m)
	}
	return roots, nil
}

// walkRecursive walks a single root and returns every flow file whose
// trailing path segments match the suffix (or every flow file if the suffix
// is empty).
func walkRecursive(root, suffix string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !isFlowFile(path) {
			return nil
		}
		if suffix != "" {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			if !matchTail(filepath.ToSlash(rel), suffix) {
				return nil
			}
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

// matchTail reports whether the trailing "/"-separated segments of rel match
// the per-segment glob pattern suffix.
func matchTail(rel, suffix string) bool {
	suffixParts := strings.Split(suffix, "/")
	relParts := strings.Split(rel, "/")
	if len(relParts) < len(suffixParts) {
		return false
	}
	tail := relParts[len(relParts)-len(suffixParts):]
	for i, seg := range suffixParts {
		matched, err := filepath.Match(seg, tail[i])
		if err != nil || !matched {
			return false
		}
	}
	return true
}

// getTopLevelFlows gets flow files directly in a directory (not recursive).
func (v *Validator) getTopLevelFlows(dir string) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if isFlowFile(path) {
			files = append(files, path)
		}
	}

	return files, nil
}

// isFlowFile checks if a file is a valid flow file.
func isFlowFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" {
		return false
	}
	// Exclude config files
	name := strings.ToLower(filepath.Base(path))
	if name == "config.yaml" || name == "config.yml" {
		return false
	}
	return true
}

// validateFile validates a single file and its runFlow dependencies.
func (v *Validator) validateFile(filePath string, result *Result, validated map[string]bool, testCasesAdded map[string]bool, chain []string, isTestCase bool) {
	// Check for circular dependency
	for _, ancestor := range chain {
		if ancestor == filePath {
			cycle := append(chain, filePath)
			result.Errors = append(result.Errors, &ValidationError{
				File:    filePath,
				Message: fmt.Sprintf("circular dependency detected: %s", strings.Join(cycle, " -> ")),
			})
			return
		}
	}

	// Parse the file if not already validated
	var f *flow.Flow
	var err error
	if !validated[filePath] {
		f, err = flow.ParseFile(filePath)
		if err != nil {
			result.Errors = append(result.Errors, &ValidationError{
				File:    filePath,
				Message: fmt.Sprintf("parse error: %v", err),
			})
			return
		}
		validated[filePath] = true

		// Recursively validate runFlow dependencies (not test cases)
		newChain := append(chain, filePath)
		v.validateRunFlowSteps(f.Steps, filePath, result, validated, testCasesAdded, newChain)

		// Also validate lifecycle hooks
		v.validateRunFlowSteps(f.Config.OnFlowStart, filePath, result, validated, testCasesAdded, newChain)
		v.validateRunFlowSteps(f.Config.OnFlowComplete, filePath, result, validated, testCasesAdded, newChain)
	}

	// Add to TestCases if it's a top-level test case and not already added
	if isTestCase && !testCasesAdded[filePath] {
		// Need to re-parse for tag check if we already validated this file earlier as a dependency
		if f == nil {
			f, err = flow.ParseFile(filePath)
			if err != nil {
				// Already reported this error during dependency validation
				return
			}
		}
		// Check tag filters
		if flow.ShouldIncludeFlow(f, v.includeTags, v.excludeTags) {
			result.TestCases = append(result.TestCases, filePath)
			testCasesAdded[filePath] = true
		}
	}
}

// validateRunFlowSteps finds and validates runFlow references in steps.
func (v *Validator) validateRunFlowSteps(steps []flow.Step, parentFile string, result *Result, validated map[string]bool, testCasesAdded map[string]bool, chain []string) {
	parentDir := filepath.Dir(parentFile)

	for _, step := range steps {
		switch s := step.(type) {
		case *flow.RunFlowStep:
			if s.File != "" {
				refPath := resolveFilePath(parentDir, s.File)
				// Dependencies are validated but NOT added as test cases
				v.validateFile(refPath, result, validated, testCasesAdded, chain, false)
			}
			// Also check inline commands
			v.validateRunFlowSteps(s.Steps, parentFile, result, validated, testCasesAdded, chain)

		case *flow.RepeatStep:
			v.validateRunFlowSteps(s.Steps, parentFile, result, validated, testCasesAdded, chain)

		case *flow.RetryStep:
			if s.File != "" {
				refPath := resolveFilePath(parentDir, s.File)
				v.validateFile(refPath, result, validated, testCasesAdded, chain, false)
			}
			v.validateRunFlowSteps(s.Steps, parentFile, result, validated, testCasesAdded, chain)
		}
	}
}

// resolveFilePath resolves a file path relative to a base directory.
func resolveFilePath(baseDir, filePath string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}
	return filepath.Join(baseDir, filePath)
}
