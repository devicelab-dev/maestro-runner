// SPDX-FileCopyrightText: 2025–2026 DeviceLab and the Project Contributors
//
// SPDX-License-Identifier: LicenseRef-maestro-runner

// Package flow handles parsing and representation of Maestro YAML flow files.
package flow

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	// Normalise CRLF first. Splitting on "\n" alone leaves a trailing "\r" on
	// every line, and the separator test below compares the untrimmed line
	// against "---" to tell a real document break from an indented "---" inside
	// a block scalar. "---\r" fails that test, so the break was never found:
	// the whole file parsed as one document and the header's map was handed to
	// the step list, producing "cannot unmarshal !!map into []yaml.Node" on
	// line 1 of every flow. Windows checks out with core.autocrlf=true by
	// default, so this was every flow on Windows (#159).
	content = strings.ReplaceAll(content, "\r\n", "\n")

	var parts []string
	var current strings.Builder
	inMultiline := false
	multilineIndent := 0

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inMultiline {
			if startsBlockScalar(trimmed) {
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

// startsBlockScalar reports whether a YAML line opens a block scalar
// (`script: |`, `text: >-`, `cmd: |2` …). Only the line's last
// whitespace-separated token is considered, and it must be a bare block
// indicator (| or > plus optional chomping/indent), so prose or comments
// that merely end in '>' — e.g. "# navigation: Library ->" — don't flip
// the splitter into multiline mode and swallow the `---` separator (#119).
func startsBlockScalar(trimmed string) bool {
	if strings.HasPrefix(trimmed, "#") {
		return false
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}
	last := fields[len(fields)-1]
	if last[0] != '|' && last[0] != '>' {
		return false
	}
	for _, c := range last[1:] {
		if c != '+' && c != '-' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
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
		StepOpenNotifications, StepAcceptAlert, StepDismissAlert,
		StepInputText, StepInputRandom, StepInputRandomEmail, StepInputRandomNumber,
		StepInputRandomPersonName, StepInputRandomText,
		StepEraseText, StepCopyTextFrom, StepPasteText, StepSetClipboard,
		StepAssertVisible, StepAssertNotVisible, StepAssertTrue, StepAssertCondition, StepAssertScreenshot,
		StepAssertNoDefectsWithAI, StepAssertWithAI, StepExtractTextWithAI, StepWaitUntil,
		StepLaunchApp, StepStopApp, StepKillApp, StepClearState, StepClearKeychain, StepSetPermissions,
		StepSetLocation, StepSetOrientation, StepSetAirplaneMode, StepToggleAirplaneMode,
		StepSetDarkMode, StepToggleDarkMode, StepAssertDarkMode, StepAssertLightMode,
		StepTravel, StepOpenLink, StepOpenBrowser, StepRepeat, StepRetry, StepRunFlow,
		StepRunScript, StepRunShell, StepEvalScript, StepEvalBrowserScript,
		StepRunBrowserScript, StepEvalWebViewScript, StepRunWebViewScript,
		StepGetConsoleLogs, StepClearConsoleLogs, StepAssertNoJSErrors,
		StepSetCookies, StepGetCookies, StepSaveAuthState, StepLoadAuthState,
		StepUploadFile, StepWaitForDownload, StepGrantPermissions, StepResetPermissions,
		StepOpenTab, StepSwitchTab, StepCloseTab,
		StepMockNetwork, StepBlockNetwork, StepSetNetworkConditions, StepWaitForRequest, StepClearNetworkMocks,
		StepTakeScreenshot, StepStartRecording,
		StepStopRecording, StepAddMedia, StepRemoveMedia, StepPressKey, StepWaitForAnimationToEnd,
		StepDefineVariables, StepDragAndDrop:
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
		// Bare `- scroll` scrolls down, matching Maestro's documented
		// default. Normalized here so every driver sees a concrete
		// direction (#120: WDA and devicelab_ios rejected "").
		if s.Direction == "" {
			s.Direction = "down"
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

	case StepOpenNotifications:
		return &OpenNotificationsStep{BaseStep: BaseStep{StepType: stepType}}, nil

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

	case StepDragAndDrop:
		var s DragAndDropStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		if s.From.IsEmpty() && s.From.Point == "" {
			return nil, wrapParseError(sourcePath, valueNode.Line,
				fmt.Errorf("dragAndDrop: from requires a selector or point"))
		}
		if s.To.IsEmpty() && s.To.Point == "" {
			return nil, wrapParseError(sourcePath, valueNode.Line,
				fmt.Errorf("dragAndDrop: to requires a selector or point"))
		}
		if s.HoldDuration == 0 {
			s.HoldDuration = 1000
		}
		if s.Duration == 0 {
			s.Duration = 1000
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
		// A numeric count literal is validated here so the flow fails before a
		// device is touched; a ${VAR} count is validated after expansion.
		if s.Count != "" && !strings.Contains(s.Count, "${") {
			if _, _, err := s.ExpectedCount(); err != nil {
				return nil, wrapParseError(sourcePath, valueNode.Line, err)
			}
		}
		if s.Count != "" && s.Selector.Index != "" {
			return nil, wrapParseError(sourcePath, valueNode.Line,
				fmt.Errorf("assertVisible: count and index cannot be combined — count asserts how many elements match, index picks one of them"))
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
		if valueNode.Kind == yaml.ScalarNode {
			switch valueNode.Value {
			case "enabled":
				s.Enabled = true
			case "disabled":
				s.Enabled = false
			default:
				return nil, wrapParseError(sourcePath, valueNode.Line,
					fmt.Errorf("setAirplaneMode expects 'enabled' or 'disabled', got %q", valueNode.Value))
			}
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		// If `enabled:` was a literal bool, copy it into Enabled now so flows
		// without variable interpolation work without the expand pass.
		if b, ok := s.EnabledRaw.(bool); ok {
			s.Enabled = b
		}
		s.StepType = stepType
		return &s, nil

	case StepToggleAirplaneMode:
		return &ToggleAirplaneModeStep{BaseStep: BaseStep{StepType: stepType}}, nil

	case StepSetDarkMode:
		var s SetDarkModeStep
		if valueNode.Kind == yaml.ScalarNode {
			switch valueNode.Value {
			case "enabled", "dark", "true":
				s.Enabled = true
			case "disabled", "light", "false":
				s.Enabled = false
			default:
				return nil, wrapParseError(sourcePath, valueNode.Line,
					fmt.Errorf("setDarkMode expects 'enabled'/'dark' or 'disabled'/'light', got %q", valueNode.Value))
			}
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		if b, ok := s.EnabledRaw.(bool); ok {
			s.Enabled = b
		}
		s.StepType = stepType
		return &s, nil

	case StepToggleDarkMode:
		return &ToggleDarkModeStep{BaseStep: BaseStep{StepType: stepType}}, nil

	case StepAssertDarkMode:
		return &AssertDarkModeStep{BaseStep: BaseStep{StepType: stepType}}, nil

	case StepAssertLightMode:
		return &AssertLightModeStep{BaseStep: BaseStep{StepType: stepType}}, nil

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

	case StepRunShell:
		var s RunShellStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Command = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		if s.Command == "" {
			return nil, wrapParseError(sourcePath, valueNode.Line, fmt.Errorf("runShell needs a command"))
		}
		s.StepType = StepRunShell
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

	case StepEvalBrowserScript:
		var s EvalBrowserScriptStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Script = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepEvalBrowserScript
		return &s, nil

	case StepRunBrowserScript:
		var s RunBrowserScriptStep
		if valueNode.Kind == yaml.ScalarNode {
			s.File = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepRunBrowserScript
		return &s, nil

	case StepEvalWebViewScript:
		var s EvalWebViewScriptStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Script = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepEvalWebViewScript
		return &s, nil

	case StepRunWebViewScript:
		var s RunWebViewScriptStep
		if valueNode.Kind == yaml.ScalarNode {
			s.File = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepRunWebViewScript
		return &s, nil

	case StepGetConsoleLogs:
		var s GetConsoleLogsStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Output = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepGetConsoleLogs
		return &s, nil

	case StepClearConsoleLogs:
		var s ClearConsoleLogsStep
		s.StepType = StepClearConsoleLogs
		return &s, nil

	case StepAssertNoJSErrors:
		var s AssertNoJSErrorsStep
		s.StepType = StepAssertNoJSErrors
		return &s, nil

	case StepSetCookies:
		var s SetCookiesStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepSetCookies
		return &s, nil

	case StepGetCookies:
		var s GetCookiesStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Output = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepGetCookies
		return &s, nil

	case StepSaveAuthState:
		var s SaveAuthStateStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Path = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepSaveAuthState
		return &s, nil

	case StepLoadAuthState:
		var s LoadAuthStateStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Path = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepLoadAuthState
		return &s, nil

	case StepUploadFile:
		var s UploadFileStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepUploadFile
		return &s, nil

	case StepWaitForDownload:
		var s WaitForDownloadStep
		if valueNode.Kind == yaml.ScalarNode {
			s.SaveTo = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepWaitForDownload
		return &s, nil

	case StepGrantPermissions:
		var s GrantPermissionsStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepGrantPermissions
		return &s, nil

	case StepResetPermissions:
		var s ResetPermissionsStep
		s.StepType = StepResetPermissions
		return &s, nil

	case StepOpenTab:
		var s OpenTabStep
		if valueNode.Kind == yaml.ScalarNode {
			s.URL = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepOpenTab
		return &s, nil

	case StepSwitchTab:
		var s SwitchTabStep
		if valueNode.Kind == yaml.ScalarNode {
			s.TabLabel = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepSwitchTab
		return &s, nil

	case StepCloseTab:
		var s CloseTabStep
		s.StepType = StepCloseTab
		return &s, nil

	case StepMockNetwork:
		var s MockNetworkStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepMockNetwork
		return &s, nil

	case StepBlockNetwork:
		var s BlockNetworkStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepBlockNetwork
		return &s, nil

	case StepSetNetworkConditions:
		var s SetNetworkConditionsStep
		if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepSetNetworkConditions
		return &s, nil

	case StepWaitForRequest:
		var s WaitForRequestStep
		if valueNode.Kind == yaml.ScalarNode {
			s.URL = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		s.StepType = StepWaitForRequest
		return &s, nil

	case StepClearNetworkMocks:
		var s ClearNetworkMocksStep
		s.StepType = StepClearNetworkMocks
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

	case StepAssertScreenshot:
		var s AssertScreenshotStep
		if valueNode.Kind == yaml.ScalarNode {
			s.Path = valueNode.Value
		} else if err := valueNode.Decode(&s); err != nil {
			return nil, wrapParseError(sourcePath, valueNode.Line, err)
		}
		// Resolve a literal numeric threshold now so flows without variable
		// interpolation work without the expand pass. A string value (e.g.
		// "${VAR}") is left for ExpandStep. Absent or 0 → default.
		if f, ok := ThresholdAsFloat(s.ThresholdRaw); ok {
			s.ThresholdPercentage = f
		}
		if _, isStr := s.ThresholdRaw.(string); !isStr && s.ThresholdPercentage == 0 {
			s.ThresholdPercentage = 95.0
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
		// Maestro's canonical syntax is a bare sequence of paths
		// (`addMedia: ["a.jpg", "b.jpg"]` / block list), so decode a sequence
		// straight into Files. A single scalar path and the historical mapping
		// form (`addMedia: { files: [...] }`) are also accepted. (#131)
		switch valueNode.Kind {
		case yaml.SequenceNode:
			if err := valueNode.Decode(&s.Files); err != nil {
				return nil, wrapParseError(sourcePath, valueNode.Line, err)
			}
		case yaml.ScalarNode:
			if valueNode.Value != "" {
				s.Files = []string{valueNode.Value}
			}
		default:
			if err := valueNode.Decode(&s); err != nil {
				return nil, wrapParseError(sourcePath, valueNode.Line, err)
			}
		}
		s.StepType = stepType
		return &s, nil

	case StepRemoveMedia:
		return &RemoveMediaStep{BaseStep: BaseStep{StepType: stepType}}, nil

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

// parseRunFlowStep handles runFlow with optional nested commands and an
// optional else branch (run when `when:` evaluates false).
//
// The else branch accepts three YAML shapes:
//   - else: path/to/fallback.yaml          (string scalar → fallback file)
//   - else: [- tapOn: foo, ...]            (sequence → inline fallback steps)
//   - elseCommands: [- tapOn: foo, ...]    (alias for the sequence form)
func parseRunFlowStep(valueNode *yaml.Node, sourcePath string) (Step, error) {
	s := &RunFlowStep{BaseStep: BaseStep{StepType: StepRunFlow}}

	if valueNode.Kind == yaml.ScalarNode {
		s.File = valueNode.Value
		return s, nil
	}

	var raw struct {
		File         string            `yaml:"file"`
		Commands     []yaml.Node       `yaml:"commands"`
		ElseCommands []yaml.Node       `yaml:"elseCommands"`
		When         *Condition        `yaml:"when"`
		Env          map[string]string `yaml:"env"`
		Optional     bool              `yaml:"optional"`
		Label        string            `yaml:"label"`
		Timeout      int               `yaml:"timeout"`
	}

	if err := valueNode.Decode(&raw); err != nil {
		return nil, wrapParseError(sourcePath, valueNode.Line, err)
	}

	s.File = raw.File
	s.When = raw.When
	s.Env = raw.Env
	s.Optional = raw.Optional
	s.StepLabel = raw.Label
	s.TimeoutMs = raw.Timeout

	for _, cmdNode := range raw.Commands {
		step, err := parseStep(&cmdNode, sourcePath)
		if err != nil {
			return nil, err
		}
		s.Steps = append(s.Steps, step)
	}

	// `else:` is decoded by walking the mapping manually because its value
	// can be either a scalar (fallback file path) or a sequence (inline
	// fallback steps). Decoding into a fixed Go type would force one shape.
	for i := 0; i+1 < len(valueNode.Content); i += 2 {
		keyNode := valueNode.Content[i]
		valNode := valueNode.Content[i+1]
		if keyNode.Value != "else" {
			continue
		}
		switch valNode.Kind {
		case yaml.ScalarNode:
			s.ElseFile = valNode.Value
		case yaml.SequenceNode:
			for _, cmdNode := range valNode.Content {
				step, err := parseStep(cmdNode, sourcePath)
				if err != nil {
					return nil, err
				}
				s.ElseSteps = append(s.ElseSteps, step)
			}
		default:
			return nil, wrapParseError(sourcePath, valNode.Line,
				fmt.Errorf("runFlow.else must be a file path (string) or a list of steps"))
		}
		break
	}

	// elseCommands: [steps...] — alias for else when used as a sequence.
	for _, cmdNode := range raw.ElseCommands {
		step, err := parseStep(&cmdNode, sourcePath)
		if err != nil {
			return nil, err
		}
		s.ElseSteps = append(s.ElseSteps, step)
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

// ThresholdAsFloat coerces a raw YAML `thresholdPercentage:` value into a
// float. YAML decodes a bare number into an int or float64 (via `any`); a
// numeric string is also accepted so a `${VAR}` that resolves to "98.5" works
// once expanded. Returns ok=false for nil or non-numeric strings.
func ThresholdAsFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
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
