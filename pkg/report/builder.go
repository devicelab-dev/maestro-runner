package report

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// BuilderConfig contains configuration for building the report skeleton.
type BuilderConfig struct {
	OutputDir     string // Base output directory for reports
	Device        Device // Device information
	App           App    // Application information
	CI            *CI    // CI/CD information (optional)
	RunnerVersion string // Maestro runner version
	DriverName    string // Driver name (appium, native, detox)
}

// BuildSkeleton creates the initial report structure from parsed flows.
// All flows and commands are set to "pending" status.
// This should be called after YAML validation, before execution starts.
func BuildSkeleton(flows []flow.Flow, cfg BuilderConfig) (*Index, []FlowDetail, error) {
	now := time.Now()

	// Build index
	index := &Index{
		Version:     Version,
		UpdateSeq:   0,
		Status:      StatusPending,
		StartTime:   now,
		LastUpdated: now,
		Device:      cfg.Device,
		App:         cfg.App,
		CI:          cfg.CI,
		MaestroRunner: RunnerInfo{
			Version: cfg.RunnerVersion,
			Driver:  cfg.DriverName,
		},
		Summary: Summary{
			Total:   len(flows),
			Pending: len(flows),
		},
		Flows: make([]FlowEntry, len(flows)),
	}

	// Build flow details
	flowDetails := make([]FlowDetail, len(flows))

	for i, f := range flows {
		flowID := fmt.Sprintf("flow-%03d", i)
		flowName := extractFlowName(f)

		// Build commands for this flow
		commands := buildCommands(f.Steps)

		// Create flow entry for index
		index.Flows[i] = FlowEntry{
			Index:      i,
			ID:         flowID,
			Name:       flowName,
			SourceFile: f.SourcePath,
			DataFile:   filepath.Join("flows", flowID+".json"),
			AssetsDir:  filepath.Join("assets", flowID),
			Status:     StatusPending,
			UpdateSeq:  0,
			Commands: CommandSummary{
				Total:   len(commands),
				Pending: len(commands),
			},
			Attempts: 0,
		}

		// Create flow detail
		flowDetails[i] = FlowDetail{
			ID:         flowID,
			Name:       flowName,
			SourceFile: f.SourcePath,
			Tags:       f.Config.Tags,
			Device:     &cfg.Device, // Device that runs this flow (for multi-device support)
			Commands:   commands,
			Artifacts:  FlowArtifacts{},
		}
	}

	return index, flowDetails, nil
}

// extractFlowName extracts a display name from the flow.
func extractFlowName(f flow.Flow) string {
	if f.Config.Name != "" {
		return f.Config.Name
	}
	// Use filename without extension
	base := filepath.Base(f.SourcePath)
	ext := filepath.Ext(base)
	return base[:len(base)-len(ext)]
}

// buildCommands creates Command entries from flow steps.
func buildCommands(steps []flow.Step) []Command {
	commands := make([]Command, len(steps))
	for i, step := range steps {
		commands[i] = Command{
			ID:        fmt.Sprintf("cmd-%03d", i),
			Index:     i,
			Type:      string(step.Type()),
			Label:     step.Label(),
			YAML:      step.Describe(),
			Status:    StatusPending,
			Params:    extractParams(step),
			Artifacts: CommandArtifacts{},
		}
	}
	return commands
}

// extractParams extracts command parameters from a step.
func extractParams(step flow.Step) *CommandParams {
	params := &CommandParams{}
	hasContent := false

	// Extract selector if present
	if sel := extractSelector(step); sel != nil {
		params.Selector = sel
		hasContent = true
	}

	// Extract text for input steps
	if s, ok := step.(*flow.InputTextStep); ok && s.Text != "" {
		params.Text = s.Text
		hasContent = true
	}

	// Extract direction for swipe/scroll steps
	switch s := step.(type) {
	case *flow.SwipeStep:
		if s.Direction != "" {
			params.Direction = s.Direction
			hasContent = true
		}
	case *flow.ScrollStep:
		if s.Direction != "" {
			params.Direction = s.Direction
			hasContent = true
		}
	case *flow.ScrollUntilVisibleStep:
		if s.Direction != "" {
			params.Direction = s.Direction
			hasContent = true
		}
	}

	// Extract timeout
	if base := getBaseStep(step); base != nil && base.TimeoutMs > 0 {
		params.Timeout = base.TimeoutMs
		hasContent = true
	}

	if !hasContent {
		return nil
	}
	return params
}

// extractSelector extracts selector from steps that have one.
func extractSelector(step flow.Step) *Selector {
	var sel *flow.Selector

	switch s := step.(type) {
	case *flow.TapOnStep:
		sel = &s.Selector
	case *flow.DoubleTapOnStep:
		sel = &s.Selector
	case *flow.LongPressOnStep:
		sel = &s.Selector
	case *flow.AssertVisibleStep:
		sel = &s.Selector
	case *flow.AssertNotVisibleStep:
		sel = &s.Selector
	case *flow.ScrollUntilVisibleStep:
		sel = &s.Element
	case *flow.InputTextStep:
		sel = &s.Selector
	case *flow.CopyTextFromStep:
		sel = &s.Selector
	default:
		return nil
	}

	if sel == nil || sel.IsEmpty() {
		return nil
	}

	return convertSelector(sel)
}

// convertSelector converts flow.Selector to report.Selector.
func convertSelector(sel *flow.Selector) *Selector {
	if sel == nil {
		return nil
	}

	// Determine selector type and value
	var sType, sValue string
	switch {
	case sel.ID != "":
		sType = "id"
		sValue = sel.ID
	case sel.Text != "":
		sType = "text"
		sValue = sel.Text
	case sel.CSS != "":
		sType = "css"
		sValue = sel.CSS
	default:
		return nil
	}

	optional := false
	if sel.Optional != nil {
		optional = *sel.Optional
	}

	return &Selector{
		Type:     sType,
		Value:    sValue,
		Optional: optional,
	}
}

// getBaseStep extracts BaseStep from a step if possible.
func getBaseStep(step flow.Step) *flow.BaseStep {
	switch s := step.(type) {
	case *flow.TapOnStep:
		return &s.BaseStep
	case *flow.DoubleTapOnStep:
		return &s.BaseStep
	case *flow.LongPressOnStep:
		return &s.BaseStep
	case *flow.AssertVisibleStep:
		return &s.BaseStep
	case *flow.AssertNotVisibleStep:
		return &s.BaseStep
	case *flow.InputTextStep:
		return &s.BaseStep
	case *flow.SwipeStep:
		return &s.BaseStep
	case *flow.ScrollStep:
		return &s.BaseStep
	case *flow.ScrollUntilVisibleStep:
		return &s.BaseStep
	case *flow.LaunchAppStep:
		return &s.BaseStep
	default:
		return nil
	}
}

// WriteSkeleton writes the initial skeleton to disk.
// Creates report.json, all flow detail files, and report.html with pending status.
func WriteSkeleton(outputDir string, index *Index, flowDetails []FlowDetail) error {
	// Ensure directories exist
	if err := ensureDir(filepath.Join(outputDir, "flows")); err != nil {
		return fmt.Errorf("create flows dir: %w", err)
	}
	if err := ensureDir(filepath.Join(outputDir, "assets")); err != nil {
		return fmt.Errorf("create assets dir: %w", err)
	}

	// Write each flow detail file
	for _, fd := range flowDetails {
		flowPath := filepath.Join(outputDir, "flows", fd.ID+".json")
		if err := atomicWriteJSON(flowPath, fd); err != nil {
			return fmt.Errorf("write flow %s: %w", fd.ID, err)
		}

		// Create assets directory for this flow
		assetsPath := filepath.Join(outputDir, "assets", fd.ID)
		if err := ensureDir(assetsPath); err != nil {
			return fmt.Errorf("create assets dir for %s: %w", fd.ID, err)
		}
	}

	// Write index file
	indexPath := filepath.Join(outputDir, "report.json")
	if err := atomicWriteJSON(indexPath, index); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	// Generate HTML report (for live viewing)
	if err := GenerateHTML(outputDir, HTMLConfig{
		Title:     "Test Report",
		ReportDir: outputDir,
	}); err != nil {
		return fmt.Errorf("generate html: %w", err)
	}

	return nil
}
