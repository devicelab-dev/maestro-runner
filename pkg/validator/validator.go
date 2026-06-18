// SPDX-FileCopyrightText: 2024–2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

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

	// Handle "**" for recursive matching
	if pattern == "**" || strings.HasPrefix(pattern, "**/") {
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

// collectRecursive collects all flow files recursively.
func (v *Validator) collectRecursive(dir, pattern string) ([]string, error) {
	var files []string

	// Get the suffix after **/ if any
	suffix := ""
	if strings.HasPrefix(pattern, "**/") {
		suffix = pattern[3:]
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !isFlowFile(path) {
			return nil
		}

		// If there's a suffix pattern, check if filename matches
		if suffix != "" {
			matched, _ := filepath.Match(suffix, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		files = append(files, path)
		return nil
	})

	return files, err
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
