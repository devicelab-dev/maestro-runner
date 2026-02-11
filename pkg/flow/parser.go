// Package flow handles parsing and representation of Maestro YAML flow files.
package flow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseError represents a parsing error with location info.
type ParseError struct {
	Path    string
	Line    int
	Message string
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", e.Path, e.Line, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ParseFile parses a single Maestro YAML flow file.
func ParseFile(path string) (*Flow, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- path is user-provided flow file
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return Parse(data, path)
}

// Parse parses Maestro YAML content.
func Parse(data []byte, sourcePath string) (*Flow, error) {
	parts := splitYAMLDocuments(string(data))

	flow := &Flow{
		SourcePath: sourcePath,
	}

	if len(parts) == 0 {
		return nil, &ParseError{
			Path:    sourcePath,
			Line:    1,
			Message: "empty flow file",
		}
	}

	if len(parts) == 1 {
		if err := parseSteps(parts[0], flow); err != nil {
			return nil, err
		}
	} else {
		if err := parseConfig(parts[0], flow); err != nil {
			return nil, err
		}
		if err := parseSteps(parts[1], flow); err != nil {
			return nil, err
		}
	}

	return flow, nil
}

func splitYAMLDocuments(content string) []string {
	var parts []string
	var current strings.Builder
	inMultiline := false
	multilineIndent := 0

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inMultiline {
			if strings.HasSuffix(trimmed, "|") || strings.HasSuffix(trimmed, ">") ||
				strings.HasSuffix(trimmed, "|-") || strings.HasSuffix(trimmed, ">-") {
				inMultiline = true
				if i+1 < len(lines) {
					next := lines[i+1]
					multilineIndent = len(next) - len(strings.TrimLeft(next, " \t"))
				}
			}
		} else {
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			if trimmed != "" && indent < multilineIndent {
				inMultiline = false
			}
		}

		if !inMultiline && trimmed == "---" && strings.TrimLeft(line, " \t") == "---" {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		} else {
			current.WriteString(line)
			current.WriteString("\n")
		}
	}

	if current.Len() > 0 {
		s := strings.TrimSpace(current.String())
		if s != "" {
			parts = append(parts, current.String())
		}
	}

	return parts
}

func parseConfig(content string, flow *Flow) error {
	var config Config
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return &ParseError{
			Path:    flow.SourcePath,
			Message: fmt.Sprintf("invalid config: %v", err),
		}
	}

	// Parse lifecycle hooks (onFlowStart, onFlowComplete)
	var rawConfig struct {
		OnFlowStart    []yaml.Node `yaml:"onFlowStart"`
		OnFlowComplete []yaml.Node `yaml:"onFlowComplete"`
	}
	if err := yaml.Unmarshal([]byte(content), &rawConfig); err != nil {
		return &ParseError{
			Path:    flow.SourcePath,
			Message: fmt.Sprintf("invalid config: %v", err),
		}
	}

	for _, node := range rawConfig.OnFlowStart {
		step, err := parseStep(&node, flow.SourcePath)
		if err != nil {
			return err
		}
		config.OnFlowStart = append(config.OnFlowStart, step)
	}

	for _, node := range rawConfig.OnFlowComplete {
		step, err := parseStep(&node, flow.SourcePath)
		if err != nil {
			return err
		}
		config.OnFlowComplete = append(config.OnFlowComplete, step)
	}

	flow.Config = config
	return nil
}

func parseSteps(content string, flow *Flow) error {
	var rawSteps []yaml.Node
	if err := yaml.Unmarshal([]byte(content), &rawSteps); err != nil {
		return &ParseError{
			Path:    flow.SourcePath,
			Message: fmt.Sprintf("invalid steps: %v", err),
		}
	}

	for _, node := range rawSteps {
		step, err := parseStep(&node, flow.SourcePath)
		if err != nil {
			return err
		}
		flow.Steps = append(flow.Steps, step)
	}

	return nil
}

func parseStep(node *yaml.Node, sourcePath string) (Step, error) {
	// Handle scalar nodes like "- waitForAnimationToEnd" (no colon, no params)
	if node.Kind == yaml.ScalarNode {
		stepType := node.Value
		if !isStepType(stepType) {
			return nil, &ParseError{
				Path:    sourcePath,
				Line:    node.Line,
				Message: fmt.Sprintf("unknown step type: %s", stepType),
			}
		}
		// Create empty value node for steps with no parameters
		emptyNode := &yaml.Node{Kind: yaml.MappingNode}
		return decodeStep(StepType(stepType), emptyNode, sourcePath)
	}

	if node.Kind != yaml.MappingNode {
		return nil, &ParseError{
			Path:    sourcePath,
			Line:    node.Line,
			Message: "step must be a mapping or command name",
		}
	}

	stepType, valueNode := extractStepType(node)
	if stepType == "" || valueNode == nil {
		return nil, &ParseError{
			Path:    sourcePath,
			Line:    node.Line,
			Message: "unknown step type",
		}
	}

	return decodeStep(StepType(stepType), valueNode, sourcePath)
}

func extractStepType(node *yaml.Node) (string, *yaml.Node) {
	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		if isStepType(key) {
			return key, node.Content[i+1]
		}
	}
	return "", nil
}

func isStepType(key string) bool {
	switch StepType(key) {
	case StepTapOn, StepDoubleTapOn, StepLongPressOn, StepTapOnPoint,
		StepSwipe, StepScroll, StepScrollUntilVisible, StepBack, StepHideKeyboard,
		StepAcceptAlert, StepDismissAlert,
		StepInputText, StepInputRandom, StepInputRandomEmail, StepInputRandomNumber,
		StepInputRandomPersonName, StepInputRandomText,
		StepEraseText, StepCopyTextFrom, StepPasteText, StepSetClipboard,
		StepAssertVisible, StepAssertNotVisible, StepAssertTrue, StepAssertCondition,
		StepAssertNoDefectsWithAI, StepAssertWithAI, StepExtractTextWithAI, StepWaitUntil,
		StepLaunchApp, StepStopApp, StepKillApp, StepClearState, StepClearKeychain, StepSetPermissions,
		StepSetLocation, StepSetOrientation, StepSetAirplaneMode, StepToggleAirplaneMode,
		StepTravel, StepOpenLink, StepOpenBrowser, StepRepeat, StepRetry, StepRunFlow,
		StepRunScript, StepEvalScript, StepTakeScreenshot, StepStartRecording,
		StepStopRecording, StepAddMedia, StepPressKey, StepWaitForAnimationToEnd,
		StepDefineVariables:
		return true
	}
	return false
}

//nolint:gocyclo
func decodeStep(stepType StepType, valueNode *yaml.Node, sourcePath string) (Step, error) {
	switch stepType {
	case StepTapOn:
		var s TapOnStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Selector.Text = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepDoubleTapOn:
		var s DoubleTapOnStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Selector.Text = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepLongPressOn:
		var s LongPressOnStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Selector.Text = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepTapOnPoint:
		var s TapOnPointStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepSwipe:
		var s SwipeStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Direction = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepScroll:
		var s ScrollStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Direction = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepScrollUntilVisible:
		var s ScrollUntilVisibleStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Element.Text = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepBack:
		return &BackStep{BaseStep: BaseStep{StepType: stepType}}, nil

	case StepHideKeyboard:
		return &HideKeyboardStep{BaseStep: BaseStep{StepType: stepType}}, nil

	case StepAcceptAlert:
		return &AcceptAlertStep{BaseStep: BaseStep{StepType: stepType}}, nil

	case StepDismissAlert:
		return &DismissAlertStep{BaseStep: BaseStep{StepType: stepType}}, nil

	case StepInputText:
		var s InputTextStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Text = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepInputRandom:
		var s InputRandomStep
		if valueNode.Kind == yaml.ScalarNode {
			s.DataType = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepInputRandomEmail:
		return &InputRandomStep{
			BaseStep: BaseStep{StepType: StepInputRandom},
			DataType: "EMAIL",
		}, nil

	case StepInputRandomNumber:
		return &InputRandomStep{
			BaseStep: BaseStep{StepType: StepInputRandom},
			DataType: "NUMBER",
		}, nil

	case StepInputRandomPersonName:
		return &InputRandomStep{
			BaseStep: BaseStep{StepType: StepInputRandom},
			DataType: "PERSON_NAME",
		}, nil

	case StepInputRandomText:
		return &InputRandomStep{
			BaseStep: BaseStep{StepType: StepInputRandom},
			DataType: "TEXT",
		}, nil

	case StepEraseText:
		var s EraseTextStep
		if valueNode.Kind == yaml.ScalarNode {
			var chars int
			if err := valueNode.Decode(&chars); err == nil {
				s.Characters = chars
			}
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepCopyTextFrom:
		var s CopyTextFromStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Selector.Text = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepPasteText:
		return &PasteTextStep{BaseStep: BaseStep{StepType: stepType}}, nil

	case StepSetClipboard:
		var s SetClipboardStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Text = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepAssertVisible:
		var s AssertVisibleStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Selector.Text = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepAssertNotVisible:
		var s AssertNotVisibleStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Selector.Text = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepAssertTrue:
		var s AssertTrueStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Script = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepAssertCondition:
		var s AssertConditionStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepAssertNoDefectsWithAI:
		var s AssertNoDefectsWithAIStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepAssertWithAI:
		var s AssertWithAIStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Assertion = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepExtractTextWithAI:
		var s ExtractTextWithAIStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepWaitUntil:
		var s WaitUntilStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepLaunchApp:
		var s LaunchAppStep
		if valueNode.Kind == yaml.ScalarNode {
			s.AppID = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepStopApp:
		var s StopAppStep
		if valueNode.Kind == yaml.ScalarNode {
			s.AppID = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepKillApp:
		var s KillAppStep
		if valueNode.Kind == yaml.ScalarNode {
			s.AppID = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepClearState:
		var s ClearStateStep
		if valueNode.Kind == yaml.ScalarNode {
			s.AppID = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepClearKeychain:
		return &ClearKeychainStep{BaseStep: BaseStep{StepType: stepType}}, nil

	case StepSetPermissions:
		var s SetPermissionsStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepSetLocation:
		var s SetLocationStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepSetOrientation:
		var s SetOrientationStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Orientation = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepSetAirplaneMode:
		var s SetAirplaneModeStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepToggleAirplaneMode:
		return &ToggleAirplaneModeStep{BaseStep: BaseStep{StepType: stepType}}, nil

	case StepTravel:
		var s TravelStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepOpenLink:
		var s OpenLinkStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Link = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepOpenBrowser:
		var s OpenBrowserStep
		if valueNode.Kind == yaml.ScalarNode {
			s.URL = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepRepeat:
		return parseRepeatStep(valueNode, sourcePath)

	case StepRetry:
		return parseRetryStep(valueNode, sourcePath)

	case StepRunFlow:
		return parseRunFlowStep(valueNode, sourcePath)

	case StepRunScript:
		var s RunScriptStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Script = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepRunScript
		return &s, nil

	case StepEvalScript:
		var s EvalScriptStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Script = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepEvalScript
		return &s, nil

	case StepTakeScreenshot:
		var s TakeScreenshotStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Path = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepStartRecording:
		var s StartRecordingStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Path = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepStopRecording:
		var s StopRecordingStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Path = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepAddMedia:
		var s AddMediaStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepPressKey:
		var s PressKeyStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Key = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepWaitForAnimationToEnd:
		var s WaitForAnimationToEndStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = stepType
		return &s, nil

	case StepDefineVariables:
		var s DefineVariablesStep
		s.Env = make(map[string]string)
		if valueNode.Kind == yaml.MappingNode {
			for i := 0; i < len(valueNode.Content)-1; i += 2 {
				s.Env[valueNode.Content[i].Value] = valueNode.Content[i+1].Value
			}
		}
		s.StepType = stepType
		return &s, nil

	default:
		return &UnsupportedStep{
			BaseStep: BaseStep{StepType: stepType},
			Reason:   "unknown step type",
		}, nil
	}
}

// parseRepeatStep handles repeat with nested commands.
func parseRepeatStep(valueNode *yaml.Node, sourcePath string) (Step, error) {
	var raw struct {
		Times    string      `yaml:"times"` // String for variable support
		While    Condition   `yaml:"while"`
		Commands []yaml.Node `yaml:"commands"`
		Optional bool        `yaml:"optional"`
		Label    string      `yaml:"label"`
	}

	if err := valueNode.Decode(&raw); err != nil {
		return nil, wrapParseError(sourcePath, valueNode.Line, err)
	}

	s := &RepeatStep{
		BaseStep: BaseStep{
			StepType:  StepRepeat,
			Optional:  raw.Optional,
			StepLabel: raw.Label,
		},
		Times: raw.Times,
		While: raw.While,
	}

	for _, cmdNode := range raw.Commands {
		step, err := parseStep(&cmdNode, sourcePath)
		if err != nil {
			return nil, err
		}
		s.Steps = append(s.Steps, step)
	}

	return s, nil
}

// parseRetryStep handles retry with nested commands.
func parseRetryStep(valueNode *yaml.Node, sourcePath string) (Step, error) {
	var raw struct {
		MaxRetries string            `yaml:"maxRetries"` // String for variable support
		Commands   []yaml.Node       `yaml:"commands"`
		File       string            `yaml:"file"`
		Env        map[string]string `yaml:"env"`
		Optional   bool              `yaml:"optional"`
		Label      string            `yaml:"label"`
	}

	if err := valueNode.Decode(&raw); err != nil {
		return nil, wrapParseError(sourcePath, valueNode.Line, err)
	}

	s := &RetryStep{
		BaseStep: BaseStep{
			StepType:  StepRetry,
			Optional:  raw.Optional,
			StepLabel: raw.Label,
		},
		MaxRetries: raw.MaxRetries,
		File:       raw.File,
		Env:        raw.Env,
	}

	for _, cmdNode := range raw.Commands {
		step, err := parseStep(&cmdNode, sourcePath)
		if err != nil {
			return nil, err
		}
		s.Steps = append(s.Steps, step)
	}

	return s, nil
}

// parseRunFlowStep handles runFlow with optional nested commands.
func parseRunFlowStep(valueNode *yaml.Node, sourcePath string) (Step, error) {
	s := &RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}}

	if valueNode.Kind == yaml.ScalarNode {
		s.File = valueNode.Value
		return s, nil
	}

	var raw struct {
		File     string            `yaml:"file"`
		Commands []yaml.Node       `yaml:"commands"`
		When     *Condition        `yaml:"when"`
		Env      map[string]string `yaml:"env"`
		Optional bool              `yaml:"optional"`
		Label    string            `yaml:"label"`
	}

	if err := valueNode.Decode(&raw); err != nil {
		return nil, wrapParseError(sourcePath, valueNode.Line, err)
	}

	s.File = raw.File
	s.When = raw.When
	s.Env = raw.Env
	s.Optional = raw.Optional
	s.StepLabel = raw.Label

	for _, cmdNode := range raw.Commands {
		step, err := parseStep(&cmdNode, sourcePath)
		if err != nil {
			return nil, err
		}
		s.Steps = append(s.Steps, step)
	}

	return s, nil
}

func wrapParseError(path string, line int, err error) error {
	return &ParseError{
		Path:    path,
		Line:    line,
		Message: err.Error(),
	}
}

// ParseDirectory parses all YAML files in a directory.
func ParseDirectory(dir string, includeTags, excludeTags []string) ([]*Flow, error) {
	var flows []*Flow

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		flow, parseErr := ParseFile(path)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, parseErr)
			return nil
		}

		if ShouldIncludeFlow(flow, includeTags, excludeTags) {
			flows = append(flows, flow)
		}
		return nil
	})

	return flows, err
}

// ShouldIncludeFlow checks if a flow matches tag filters.
func ShouldIncludeFlow(flow *Flow, includeTags, excludeTags []string) bool {
	if len(includeTags) > 0 {
		hasTag := false
		for _, tag := range flow.Config.Tags {
			for _, include := range includeTags {
				if tag == include {
					hasTag = true
					break
				}
			}
		}
		if !hasTag {
			return false
		}
	}

	for _, tag := range flow.Config.Tags {
		for _, exclude := range excludeTags {
			if tag == exclude {
				return false
			}
		}
	}

	return true
}
