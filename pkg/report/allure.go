package report

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// Allure result schema types.

// AllureResult represents a single test result in Allure format.
type AllureResult struct {
	UUID          string              `json:"uuid"`
	HistoryID     string              `json:"historyId"`
	FullName      string              `json:"fullName"`
	Name          string              `json:"name"`
	Status        string              `json:"status"`
	Stage         string              `json:"stage"`
	Start         int64               `json:"start"`
	Stop          int64               `json:"stop"`
	Labels        []AllureLabel       `json:"labels"`
	StatusDetails AllureStatusDetails `json:"statusDetails"`
	Steps         []AllureStep        `json:"steps"`
	Attachments   []AllureAttachment  `json:"attachments"`
}

// AllureStep represents a step within a test result.
type AllureStep struct {
	Name        string             `json:"name"`
	Status      string             `json:"status"`
	Stage       string             `json:"stage"`
	Start       int64              `json:"start"`
	Stop        int64              `json:"stop"`
	Steps       []AllureStep       `json:"steps"`
	Attachments []AllureAttachment `json:"attachments"`
}

// AllureAttachment represents a file attachment.
type AllureAttachment struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Type   string `json:"type"`
}

// AllureLabel represents a label on a test result.
type AllureLabel struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// AllureStatusDetails holds failure message and trace.
type AllureStatusDetails struct {
	Message string `json:"message"`
	Trace   string `json:"trace"`
}

// AllureCategory defines a failure category with regex matching.
type AllureCategory struct {
	Name            string   `json:"name"`
	MatchedStatuses []string `json:"matchedStatuses"`
	MessageRegex    string   `json:"messageRegex"`
}

// AllureExecutor holds executor branding info.
type AllureExecutor struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	ReportURL  string `json:"reportUrl"`
	ReportName string `json:"reportName"`
}

// GenerateAllure generates Allure-compatible report files in <reportDir>/allure-results/.
func GenerateAllure(reportDir string) error {
	index, flows, err := ReadReport(reportDir)
	if err != nil {
		return fmt.Errorf("read report: %w", err)
	}

	allureDir := filepath.Join(reportDir, "allure-results")
	if err := os.MkdirAll(allureDir, 0o755); err != nil {
		return fmt.Errorf("create allure-results dir: %w", err)
	}

	// Write one result file per flow
	for i, entry := range index.Flows {
		var detail *FlowDetail
		if i < len(flows) {
			detail = &flows[i]
		}

		result := buildAllureResult(&entry, detail, index, i)

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal allure result for %s: %w", entry.ID, err)
		}

		resultPath := filepath.Join(allureDir, entry.ID+"-result.json")
		if err := os.WriteFile(resultPath, data, 0o644); err != nil {
			return fmt.Errorf("write allure result %s: %w", entry.ID, err)
		}
	}

	// Write categories.json
	if err := writeAllureCategories(allureDir); err != nil {
		return err
	}

	// Write environment.properties
	if err := writeAllureEnvironment(allureDir, index); err != nil {
		return err
	}

	// Write executor.json
	if err := writeAllureExecutor(allureDir); err != nil {
		return err
	}

	return nil
}

// buildAllureResult builds an AllureResult from a flow entry and its detail.
func buildAllureResult(entry *FlowEntry, detail *FlowDetail, index *Index, flowIndex int) AllureResult {
	status := mapAllureStatus(entry.Status)

	var startMs, stopMs int64
	if entry.StartTime != nil {
		startMs = entry.StartTime.UnixMilli()
	}
	if entry.EndTime != nil {
		stopMs = entry.EndTime.UnixMilli()
	} else if entry.StartTime != nil && entry.Duration != nil {
		stopMs = startMs + *entry.Duration
	}

	// Labels
	dev := resolveDevice(entry, index)
	labels := []AllureLabel{
		{Name: "suite", Value: entry.Name},
		{Name: "parentSuite", Value: filepath.Base(entry.SourceFile)},
		{Name: "framework", Value: "maestro"},
		{Name: "severity", Value: "normal"},
	}
	if dev != nil {
		if dev.Name != "" {
			labels = append(labels, AllureLabel{Name: "host", Value: dev.Name})
		}
		if dev.ID != "" {
			labels = append(labels, AllureLabel{Name: "thread", Value: dev.ID})
		}
	}
	for _, tag := range entry.Tags {
		labels = append(labels, AllureLabel{Name: "tag", Value: tag})
	}

	// Status details
	var statusDetails AllureStatusDetails
	if entry.Error != nil {
		statusDetails.Message = *entry.Error
	}

	// Steps and attachments from detail
	var steps []AllureStep
	var attachments []AllureAttachment
	if detail != nil {
		steps = buildAllureSteps(detail.Commands)
		attachments = collectAttachments(detail.Commands)
	}

	return AllureResult{
		UUID:          entry.ID,
		HistoryID:     fnv32aHash(entry.Name + ":" + entry.SourceFile),
		FullName:      entry.Name,
		Name:          entry.Name,
		Status:        status,
		Stage:         "finished",
		Start:         startMs,
		Stop:          stopMs,
		Labels:        labels,
		StatusDetails: statusDetails,
		Steps:         steps,
		Attachments:   attachments,
	}
}

// buildAllureSteps recursively builds Allure steps from commands.
func buildAllureSteps(commands []Command) []AllureStep {
	steps := make([]AllureStep, 0, len(commands))
	for _, cmd := range commands {
		step := buildAllureStep(cmd)
		steps = append(steps, step)
	}
	return steps
}

func buildAllureStep(cmd Command) AllureStep {
	name := cmd.Type
	if cmd.Label != "" {
		name = cmd.Type + ": " + cmd.Label
	}

	status := mapAllureStatus(cmd.Status)

	var startMs, stopMs int64
	if cmd.StartTime != nil {
		startMs = cmd.StartTime.UnixMilli()
	}
	if cmd.EndTime != nil {
		stopMs = cmd.EndTime.UnixMilli()
	} else if cmd.StartTime != nil && cmd.Duration != nil {
		stopMs = startMs + *cmd.Duration
	}

	var subSteps []AllureStep
	if len(cmd.SubCommands) > 0 {
		subSteps = buildAllureSteps(cmd.SubCommands)
	} else {
		subSteps = []AllureStep{}
	}

	// Step-level attachments from screenshots
	var attachments []AllureAttachment
	if cmd.Artifacts.ScreenshotBefore != "" {
		attachments = append(attachments, AllureAttachment{
			Name:   "Before",
			Source: filepath.Base(cmd.Artifacts.ScreenshotBefore),
			Type:   "image/png",
		})
	}
	if cmd.Artifacts.ScreenshotAfter != "" {
		attachments = append(attachments, AllureAttachment{
			Name:   "After",
			Source: filepath.Base(cmd.Artifacts.ScreenshotAfter),
			Type:   "image/png",
		})
	}

	return AllureStep{
		Name:        name,
		Status:      status,
		Stage:       "finished",
		Start:       startMs,
		Stop:        stopMs,
		Steps:       subSteps,
		Attachments: attachments,
	}
}

// collectAttachments gathers all screenshot attachments from commands (flat list for flow-level).
func collectAttachments(commands []Command) []AllureAttachment {
	var attachments []AllureAttachment
	collectAttachmentsRecursive(commands, &attachments)
	return attachments
}

func collectAttachmentsRecursive(commands []Command, attachments *[]AllureAttachment) {
	for _, cmd := range commands {
		if cmd.Artifacts.ScreenshotBefore != "" {
			*attachments = append(*attachments, AllureAttachment{
				Name:   "Screenshot",
				Source: filepath.Base(cmd.Artifacts.ScreenshotBefore),
				Type:   "image/png",
			})
		}
		if cmd.Artifacts.ScreenshotAfter != "" {
			*attachments = append(*attachments, AllureAttachment{
				Name:   "Screenshot",
				Source: filepath.Base(cmd.Artifacts.ScreenshotAfter),
				Type:   "image/png",
			})
		}
		if len(cmd.SubCommands) > 0 {
			collectAttachmentsRecursive(cmd.SubCommands, attachments)
		}
	}
}

// copyAllureAttachments copies screenshot files from assets subdirs into allure-results/ flat.
func copyAllureAttachments(reportDir, allureDir string, flows []FlowDetail) {
	for _, flow := range flows {
		copyCommandAttachments(reportDir, allureDir, flow.Commands)
	}
}

func copyCommandAttachments(reportDir, allureDir string, commands []Command) {
	for _, cmd := range commands {
		for _, path := range []string{cmd.Artifacts.ScreenshotBefore, cmd.Artifacts.ScreenshotAfter} {
			if path == "" {
				continue
			}
			src := filepath.Join(reportDir, path)
			dst := filepath.Join(allureDir, filepath.Base(path))
			copyFile(src, dst)
		}
		if len(cmd.SubCommands) > 0 {
			copyCommandAttachments(reportDir, allureDir, cmd.SubCommands)
		}
	}
}

// copyFile copies a single file from src to dst, ignoring errors silently
// (screenshots may not exist for passed flows with artifact-on-failure mode).
func copyFile(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer func() {
		if err := in.Close(); err != nil {
			logger.Warn("failed to close source file %s: %v", src, err)
		}
	}()

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		if err := out.Close(); err != nil {
			logger.Warn("failed to close destination file %s: %v", dst, err)
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		logger.Warn("failed to copy %s to %s: %v", src, dst, err)
	}
}

// mapAllureStatus maps report Status to Allure status string.
func mapAllureStatus(s Status) string {
	switch s {
	case StatusPassed:
		return "passed"
	case StatusFailed:
		return "failed"
	case StatusSkipped:
		return "skipped"
	default:
		return "unknown"
	}
}

// fnv32aHash returns a hex-encoded FNV-32a hash of the input string.
func fnv32aHash(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("%08x", h.Sum32())
}

// writeAllureCategories writes categories.json for failure categorization.
func writeAllureCategories(allureDir string) error {
	categories := []AllureCategory{
		{Name: "Element Not Found", MatchedStatuses: []string{"failed"}, MessageRegex: "(?i).*element not found.*"},
		{Name: "Element Not Visible", MatchedStatuses: []string{"failed"}, MessageRegex: "(?i).*not visible.*|.*not displayed.*"},
		{Name: "Timeout", MatchedStatuses: []string{"failed"}, MessageRegex: "(?i).*timeout.*|.*timed out.*"},
		{Name: "Assertion Failed", MatchedStatuses: []string{"failed"}, MessageRegex: "(?i).*assert.*"},
		{Name: "App Launch Failed", MatchedStatuses: []string{"failed"}, MessageRegex: "(?i).*launch.*failed.*|.*app.*crash.*"},
		{Name: "Connection Error", MatchedStatuses: []string{"failed"}, MessageRegex: "(?i).*connection.*|.*socket.*|.*network.*"},
		{Name: "Script Error", MatchedStatuses: []string{"failed"}, MessageRegex: "(?i).*script.*error.*|.*runScript.*"},
		{Name: "Input Error", MatchedStatuses: []string{"failed"}, MessageRegex: "(?i).*input.*|.*keyboard.*"},
	}

	data, err := json.MarshalIndent(categories, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal categories: %w", err)
	}

	path := filepath.Join(allureDir, "categories.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write categories.json: %w", err)
	}

	return nil
}

// writeAllureEnvironment writes environment.properties with device/framework metadata.
func writeAllureEnvironment(allureDir string, index *Index) error {
	var b strings.Builder
	b.WriteString("framework=maestro\n")

	if index.Device.Name != "" {
		b.WriteString(fmt.Sprintf("device.name=%s\n", index.Device.Name))
	}
	if index.Device.Platform != "" {
		b.WriteString(fmt.Sprintf("device.platform=%s\n", index.Device.Platform))
	}
	if index.Device.OSVersion != "" {
		b.WriteString(fmt.Sprintf("device.osVersion=%s\n", index.Device.OSVersion))
	}
	if index.MaestroRunner.Version != "" {
		b.WriteString(fmt.Sprintf("runner.version=%s\n", index.MaestroRunner.Version))
	}
	if index.MaestroRunner.Driver != "" {
		b.WriteString(fmt.Sprintf("runner.driver=%s\n", index.MaestroRunner.Driver))
	}
	if index.App.ID != "" {
		b.WriteString(fmt.Sprintf("app.id=%s\n", index.App.ID))
	}

	path := filepath.Join(allureDir, "environment.properties")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write environment.properties: %w", err)
	}

	return nil
}

// writeAllureExecutor writes executor.json with DeviceLab branding.
func writeAllureExecutor(allureDir string) error {
	executor := AllureExecutor{
		Name:       "DeviceLab",
		Type:       "devicelab",
		ReportURL:  "https://devicelab.dev",
		ReportName: "Powered by DeviceLab",
	}

	data, err := json.MarshalIndent(executor, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal executor: %w", err)
	}

	path := filepath.Join(allureDir, "executor.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write executor.json: %w", err)
	}

	return nil
}
