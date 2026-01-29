package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/executor"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// printUnifiedOutput prints detailed results, summary table, and device summary.
// This unified format works for both single device and parallel execution.
func printUnifiedOutput(outputDir string, result *executor.RunResult) error {
	// Load report index to get device info
	reportIndex, err := loadReportIndex(filepath.Join(outputDir, "report.json"))
	if err != nil {
		// Fallback to old summary if we can't load report
		fmt.Printf("Warning: Could not load report for unified output: %v\n", err)
		printSummary(result)
		return nil
	}

	// 1. Print detailed flow-by-flow results with device info
	if err := printDetailedFlowResults(outputDir, reportIndex); err != nil {
		fmt.Printf("Warning: Could not print detailed results: %v\n", err)
	}

	// 2. Print summary table with device column
	printUnifiedSummaryTable(reportIndex, result)

	// 3. Print device summary stats
	printDeviceSummary(reportIndex)

	return nil
}

// loadReportIndex loads the report index from JSON file.
func loadReportIndex(path string) (*report.Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var index report.Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}

	return &index, nil
}

// loadFlowDetail loads a flow detail file from JSON.
func loadFlowDetail(path string) (*report.FlowDetail, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var detail report.FlowDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, err
	}

	return &detail, nil
}

// formatDeviceLabel formats device info for display.
func formatDeviceLabel(device *report.Device) string {
	if device == nil {
		return "Unknown"
	}

	if device.OSVersion != "" {
		return fmt.Sprintf("%s (%s %s)", device.Name, device.Platform, device.OSVersion)
	}
	return fmt.Sprintf("%s (%s)", device.Name, device.Platform)
}

// printDetailedFlowResults prints flow-by-flow results with all commands.
func printDetailedFlowResults(outputDir string, reportIndex *report.Index) error {
	for i, flowEntry := range reportIndex.Flows {
		// Print flow header with device info
		deviceLabel := formatDeviceLabel(flowEntry.Device)
		fmt.Printf("\n  %s[%d/%d]%s %s%s%s (%s) - Device: %s\n",
			color(colorCyan), i+1, len(reportIndex.Flows), color(colorReset),
			color(colorBold), flowEntry.Name, color(colorReset),
			flowEntry.SourceFile, deviceLabel)
		fmt.Println("  " + strings.Repeat("─", 60))

		// Load flow detail to get commands
		flowDetailPath := filepath.Join(outputDir, flowEntry.DataFile)
		flowDetail, err := loadFlowDetail(flowDetailPath)
		if err != nil {
			fmt.Printf("    (Could not load command details: %v)\n", err)
		} else {
			// Print each command
			for _, cmd := range flowDetail.Commands {
				printCommand(cmd, 0)
			}
		}

		// Print flow result
		duration := int64(0)
		if flowEntry.Duration != nil {
			duration = *flowEntry.Duration
		}

		if flowEntry.Status == report.StatusPassed {
			fmt.Printf("%s✓ %s%s %s%s%s\n",
				color(colorGreen), color(colorReset), flowEntry.Name,
				color(colorGray), formatDuration(duration), color(colorReset))
		} else if flowEntry.Status == report.StatusFailed {
			fmt.Printf("%s✗ %s%s %s%s%s\n",
				color(colorRed), color(colorReset), flowEntry.Name,
				color(colorGray), formatDuration(duration), color(colorReset))
		}
	}

	return nil
}

// printCommand prints a single command with proper indentation.
func printCommand(cmd report.Command, depth int) {
	indent := strings.Repeat("  ", 2+depth) // Base indent of 2, plus depth

	// Get command description (prefer Label, fallback to Type)
	description := cmd.Label
	if description == "" {
		description = cmd.Type
	}
	if description == "" && cmd.YAML != "" {
		description = cmd.YAML
	}

	// Get duration
	duration := int64(0)
	if cmd.Duration != nil {
		duration = *cmd.Duration
	}

	// Check if it's a slow command
	isSlow := duration >= slowThresholdMs && !isCompoundCommand(description)
	passed := cmd.Status == report.StatusPassed

	if passed {
		symbol := "✓"
		symbolColor := color(colorGreen)
		durColor := ""
		if isSlow {
			durColor = color(colorYellow)
			symbol = "⚠"
			symbolColor = color(colorYellow)
		}
		fmt.Printf("%s%s%s%s %s %s(%s)%s\n",
			indent, symbolColor, symbol, color(colorReset),
			description, durColor, formatDuration(duration), color(colorReset))
	} else {
		fmt.Printf("%s%s✗%s %s (%s)\n",
			indent, color(colorRed), color(colorReset),
			description, formatDuration(duration))
		if cmd.Error != nil && cmd.Error.Message != "" {
			fmt.Printf("%s  %s╰─%s %s\n",
				indent, color(colorGray), color(colorReset), cmd.Error.Message)
		}
	}

	// Print sub-commands (for runFlow, repeat, retry)
	for _, subCmd := range cmd.SubCommands {
		printCommand(subCmd, depth+1)
	}
}

// isCompoundCommand checks if a command is a compound command (runFlow, repeat, retry).
func isCompoundCommand(desc string) bool {
	return strings.HasPrefix(desc, "runFlow:") ||
		strings.HasPrefix(desc, "repeat:") ||
		strings.HasPrefix(desc, "retry:")
}

// printUnifiedSummaryTable prints the summary table with device column.
func printUnifiedSummaryTable(reportIndex *report.Index, result *executor.RunResult) {
	// Calculate totals
	totalSteps := 0
	passedSteps := 0
	failedSteps := 0
	skippedSteps := 0
	for _, fr := range result.FlowResults {
		totalSteps += fr.StepsTotal
		passedSteps += fr.StepsPassed
		failedSteps += fr.StepsFailed
		skippedSteps += fr.StepsSkipped
	}

	// Print step summary
	fmt.Println()
	if passedSteps > 0 {
		fmt.Printf("  %s%d steps passing%s (%s)\n",
			color(colorGreen), passedSteps, color(colorReset), formatDuration(result.Duration))
	}
	if failedSteps > 0 {
		fmt.Printf("  %s%d steps failing%s\n", color(colorRed), failedSteps, color(colorReset))
	}
	if skippedSteps > 0 {
		fmt.Printf("  %s%d steps skipped%s\n", color(colorCyan), skippedSteps, color(colorReset))
	}
	fmt.Println()

	// Print table header with Device column
	tableWidth := 116 // Increased width for device column
	fmt.Println(strings.Repeat("═", tableWidth))
	fmt.Printf("  %-30s %6s %7s %6s %6s %6s %10s  %s\n",
		"Flow", "Status", "Steps", "Pass", "Fail", "Skip", "Duration", "Device")
	fmt.Println(strings.Repeat("─", tableWidth))

	// Print each flow result with device info
	for _, flowEntry := range reportIndex.Flows {
		// Find corresponding flow result
		var fr *executor.FlowResult
		for i := range result.FlowResults {
			if result.FlowResults[i].Name == flowEntry.Name {
				fr = &result.FlowResults[i]
				break
			}
		}

		if fr == nil {
			continue
		}

		var status string
		var statusColor string
		if fr.Status == report.StatusFailed {
			status = "✗ FAIL"
			statusColor = color(colorRed)
		} else if fr.Status == report.StatusSkipped {
			status = "- SKIP"
			statusColor = color(colorCyan)
		} else {
			status = "✓ PASS"
			statusColor = color(colorGreen)
		}

		// Truncate name if too long
		name := fr.Name
		if len(name) > 30 {
			name = name[:27] + "..."
		}

		// Format device label
		deviceLabel := formatDeviceLabel(flowEntry.Device)
		if len(deviceLabel) > 30 {
			deviceLabel = deviceLabel[:27] + "..."
		}

		fmt.Printf("  %-30s %s%6s%s %7d %6d %6d %6d %10s  %s\n",
			name, statusColor, status, color(colorReset),
			fr.StepsTotal, fr.StepsPassed, fr.StepsFailed, fr.StepsSkipped,
			formatDuration(fr.Duration), deviceLabel)
	}

	// Print totals row
	fmt.Println(strings.Repeat("─", tableWidth))
	statusStr := fmt.Sprintf("%d/%d", result.PassedFlows, result.TotalFlows)
	statusColor := color(colorGreen)
	if result.FailedFlows > 0 {
		statusColor = color(colorRed)
	}
	fmt.Printf("  %s%-30s%s %s%6s%s %7d %6d %6d %6d %10s\n",
		color(colorBold), "TOTAL", color(colorReset),
		statusColor, statusStr, color(colorReset),
		totalSteps, passedSteps, failedSteps, skippedSteps,
		formatDuration(result.Duration))
	fmt.Println(strings.Repeat("═", tableWidth))
}

// groupFlowsByDevice groups flows by their device ID.
func groupFlowsByDevice(flows []report.FlowEntry) map[string][]report.FlowEntry {
	grouped := make(map[string][]report.FlowEntry)

	for _, flow := range flows {
		if flow.Device != nil {
			deviceKey := flow.Device.ID
			grouped[deviceKey] = append(grouped[deviceKey], flow)
		}
	}

	return grouped
}

// printDeviceSummary prints per-device statistics.
func printDeviceSummary(reportIndex *report.Index) {
	// Group flows by device
	deviceFlows := groupFlowsByDevice(reportIndex.Flows)

	if len(deviceFlows) == 0 {
		return
	}

	fmt.Println("\n\nDevice Summary")
	fmt.Println(strings.Repeat("─", 60))

	for _, flows := range deviceFlows {
		if len(flows) == 0 {
			continue
		}

		device := flows[0].Device
		if device == nil {
			continue
		}

		// Count passed/failed
		passed := 0
		failed := 0
		for _, flow := range flows {
			if flow.Status == report.StatusPassed {
				passed++
			} else if flow.Status == report.StatusFailed {
				failed++
			}
		}

		fmt.Printf("\nDevice: %s\n", device.Name)

		// Platform info
		platform := device.Platform
		if device.OSVersion != "" {
			platform = fmt.Sprintf("%s %s", device.Platform, device.OSVersion)
		}
		if device.IsSimulator {
			platform += " (Simulator)"
		}
		fmt.Printf("  Platform: %s\n", platform)

		// Flow stats
		fmt.Printf("  Flows: %d • Passed: %s%d%s • Failed: %s%d%s\n",
			len(flows),
			color(colorGreen), passed, color(colorReset),
			color(colorRed), failed, color(colorReset))
	}

	fmt.Println()
}
