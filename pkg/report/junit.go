package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GenerateJUnit generates a JUnit XML report from the report directory.
// It reads report.json and flow detail files, then writes junit-report.xml.
func GenerateJUnit(reportDir string) error {
	index, flows, err := ReadReport(reportDir)
	if err != nil {
		return fmt.Errorf("read report: %w", err)
	}

	xml := buildJUnitXML(index, flows)

	outputPath := filepath.Join(reportDir, "junit-report.xml")
	if err := os.WriteFile(outputPath, []byte(xml), 0o644); err != nil {
		return fmt.Errorf("write junit xml: %w", err)
	}

	return nil
}

// buildJUnitXML builds the JUnit XML string from index and flow details.
func buildJUnitXML(index *Index, flows []FlowDetail) string {
	var totalTime float64
	if index.EndTime != nil {
		totalTime = index.EndTime.Sub(index.StartTime).Seconds()
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(fmt.Sprintf(
		`<testsuites tests="%d" failures="%d" skipped="%d" errors="0" time="%.3f">`+"\n",
		index.Summary.Total,
		index.Summary.Failed,
		index.Summary.Skipped,
		totalTime,
	))

	timestamp := index.StartTime.Format(time.RFC3339)
	b.WriteString(fmt.Sprintf(
		`  <testsuite name="maestro-runner" tests="%d" failures="%d" skipped="%d" errors="0" time="%.3f" timestamp="%s">`+"\n",
		index.Summary.Total,
		index.Summary.Failed,
		index.Summary.Skipped,
		totalTime,
		timestamp,
	))

	for i, entry := range index.Flows {
		var flowDetail *FlowDetail
		if i < len(flows) {
			flowDetail = &flows[i]
		}
		b.WriteString(buildTestCase(&entry, flowDetail, index))
	}

	b.WriteString("  </testsuite>\n")
	b.WriteString("</testsuites>\n")

	return b.String()
}

// relSourceFile returns the flow's source path for the JUnit `file` property,
// preserving its subdirectories (e.g. "authentication/flow.yaml") instead of
// flattening it to the bare filename (#96). It reports the path relative to the
// working directory — which, for a config/workspace run, is where the flow tree
// (and config.yaml) lives — so CI tools like action-junit-report can locate the
// file. Falls back to the original path if it can't be made relative (e.g. the
// flow lives outside the working tree).
func relSourceFile(sourceFile string) string {
	if sourceFile == "" {
		return sourceFile
	}
	abs, err := filepath.Abs(sourceFile)
	if err != nil {
		return sourceFile
	}
	cwd, err := os.Getwd()
	if err != nil {
		return sourceFile
	}
	// Resolve symlinks on both sides so a symlinked working directory (e.g.
	// macOS /tmp -> /private/tmp) doesn't defeat the relative computation.
	// Best-effort: in a real run the flow file still exists at report time, so
	// it resolves; if it doesn't, we compare the lexical paths.
	if r, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = r
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		abs = r
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || rel == "" || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return sourceFile
	}
	return filepath.ToSlash(rel)
}

// buildTestCase builds a single <testcase> element.
func buildTestCase(entry *FlowEntry, detail *FlowDetail, index *Index) string {
	var tcTime float64
	if entry.Duration != nil {
		tcTime = float64(*entry.Duration) / 1000.0
	}

	var b strings.Builder
	name := xmlEscape(entry.Name)
	b.WriteString(fmt.Sprintf(
		`    <testcase name="%s" classname="%s" time="%.3f">`+"\n",
		name, name, tcTime,
	))

	// Properties: file, device info
	b.WriteString("      <properties>\n")
	b.WriteString(fmt.Sprintf(
		`        <property name="file" value="%s"/>`+"\n",
		xmlEscape(relSourceFile(entry.SourceFile)),
	))

	dev := resolveDevice(entry, index)
	if dev != nil {
		if dev.Name != "" {
			b.WriteString(fmt.Sprintf(
				`        <property name="device.name" value="%s"/>`+"\n",
				xmlEscape(dev.Name),
			))
		}
		if dev.ID != "" {
			b.WriteString(fmt.Sprintf(
				`        <property name="device.id" value="%s"/>`+"\n",
				xmlEscape(dev.ID),
			))
		}
		if dev.Platform != "" {
			b.WriteString(fmt.Sprintf(
				`        <property name="device.platform" value="%s"/>`+"\n",
				xmlEscape(dev.Platform),
			))
		}
	}
	// Custom flow properties (`properties:` in the flow config, #84) — sorted
	// so the XML is stable across runs.
	if detail != nil && len(detail.Properties) > 0 {
		keys := make([]string, 0, len(detail.Properties))
		for k := range detail.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf(
				`        <property name="%s" value="%s"/>`+"\n",
				xmlEscape(k), xmlEscape(detail.Properties[k]),
			))
		}
	}
	b.WriteString("      </properties>\n")

	// Status-specific elements
	switch entry.Status {
	case StatusFailed:
		failureType, failureBody := resolveFailure(entry, detail)
		errMsg := ""
		if entry.Error != nil {
			errMsg = *entry.Error
		}
		b.WriteString(fmt.Sprintf(
			`      <failure message="%s" type="%s">%s</failure>`+"\n",
			xmlEscape(errMsg),
			xmlEscape(failureType),
			xmlEscape(failureBody),
		))
	case StatusSkipped:
		b.WriteString("      <skipped/>\n")
	}

	// Attachments via the [[ATTACHMENT|path]] convention (Jenkins JUnit
	// Attachments and compatible CI tooling). Paths are relative to the
	// report directory, which is where junit-report.xml itself lives.
	if atts := junitAttachments(entry, detail); len(atts) > 0 {
		b.WriteString("      <system-out>")
		for _, a := range atts {
			b.WriteString(fmt.Sprintf("[[ATTACHMENT|%s]]", xmlEscape(a)))
		}
		b.WriteString("</system-out>\n")
	}

	b.WriteString("    </testcase>\n")
	return b.String()
}

// junitAttachments gathers report-relative artifact paths worth surfacing in
// CI: the failing step's screenshot (failed flows only — a green run's
// screenshots are noise there) and the flow's screen recording when --record
// produced one.
func junitAttachments(entry *FlowEntry, detail *FlowDetail) []string {
	if detail == nil {
		return nil
	}
	var atts []string
	if entry.Status == StatusFailed {
		if cmd := findFailedCommand(detail.Commands); cmd != nil && cmd.Artifacts.ScreenshotAfter != "" {
			atts = append(atts, cmd.Artifacts.ScreenshotAfter)
		}
	}
	if detail.Artifacts.Video != "" {
		atts = append(atts, detail.Artifacts.Video)
	}
	return atts
}

// resolveDevice returns the device for a flow entry, falling back to the index-level device.
func resolveDevice(entry *FlowEntry, index *Index) *Device {
	if entry.Device != nil {
		return entry.Device
	}
	return &index.Device
}

// resolveFailure determines the failure type and body from the flow detail.
// It finds the first failed command and maps its type to a failure category.
func resolveFailure(entry *FlowEntry, detail *FlowDetail) (failureType, body string) {
	if detail == nil {
		return "TestError", ""
	}

	cmd := findFailedCommand(detail.Commands)
	if cmd == nil {
		return "TestError", ""
	}

	failureType = mapCommandTypeToFailure(cmd.Type)

	// Use the command's label or type as the failure body (step description)
	if cmd.Label != "" {
		body = cmd.Label
	} else {
		body = cmd.Type
	}

	return failureType, body
}

// findFailedCommand finds the first failed command, searching sub-commands recursively.
func findFailedCommand(commands []Command) *Command {
	for i := range commands {
		if commands[i].Status == StatusFailed {
			// Check sub-commands first for more specific failure
			if len(commands[i].SubCommands) > 0 {
				if sub := findFailedCommand(commands[i].SubCommands); sub != nil {
					return sub
				}
			}
			return &commands[i]
		}
	}
	return nil
}

// mapCommandTypeToFailure maps a Maestro command type to a JUnit failure type.
func mapCommandTypeToFailure(cmdType string) string {
	switch cmdType {
	case "assertVisible", "assertNotVisible":
		return "AssertionError"
	case "tapOn", "doubleTapOn", "longPressOn":
		return "ElementInteractionError"
	case "inputText", "eraseText":
		return "InputError"
	case "launchApp", "stopApp":
		return "AppLifecycleError"
	case "runFlow", "runScript":
		return "SubflowError"
	case "scroll", "swipe", "scrollUntilVisible":
		return "ScrollError"
	default:
		return "TestError"
	}
}

// xmlEscape escapes special XML characters in a string.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
