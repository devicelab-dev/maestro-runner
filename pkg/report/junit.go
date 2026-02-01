package report

import (
	"fmt"
	"os"
	"path/filepath"
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
		xmlEscape(filepath.Base(entry.SourceFile)),
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

	b.WriteString("    </testcase>\n")
	return b.String()
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
