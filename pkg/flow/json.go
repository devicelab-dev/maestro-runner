package flow

import (
	"encoding/json"
	"fmt"
)

// UnmarshalStep deserializes a JSON step into the correct concrete Step type.
// It reads the "type" discriminator field first, then unmarshals into the
// appropriate struct — mirroring decodeStep in parser.go for YAML.
func UnmarshalStep(data []byte) (Step, error) {
	var envelope struct {
		Type StepType `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal step type: %w", err)
	}
	if envelope.Type == "" {
		return nil, fmt.Errorf("missing \"type\" field in step JSON")
	}

	return unmarshalStepByType(envelope.Type, data)
}

//nolint:gocyclo
func unmarshalStepByType(stepType StepType, data []byte) (Step, error) {
	switch stepType {
	case StepTapOn:
		var s TapOnStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepDoubleTapOn:
		var s DoubleTapOnStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepLongPressOn:
		var s LongPressOnStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepTapOnPoint:
		var s TapOnPointStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepSwipe:
		var s SwipeStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepScroll:
		var s ScrollStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepScrollUntilVisible:
		var s ScrollUntilVisibleStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepBack:
		var s BackStep
		s.StepType = stepType
		return &s, nil

	case StepHideKeyboard:
		var s HideKeyboardStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepAcceptAlert:
		var s AcceptAlertStep
		s.StepType = stepType
		return &s, nil

	case StepIsKeyboardVisible:
		var s IsKeyboardVisibleStep
		s.StepType = stepType
		return &s, nil

	case StepDismissAlert:
		var s DismissAlertStep
		s.StepType = stepType
		return &s, nil

	case StepInputText:
		var s InputTextStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepInputRandom:
		var s InputRandomStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepEraseText:
		var s EraseTextStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepCopyTextFrom:
		var s CopyTextFromStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepPasteText:
		var s PasteTextStep
		s.StepType = stepType
		return &s, nil

	case StepSetClipboard:
		var s SetClipboardStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepAssertVisible:
		var s AssertVisibleStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepAssertNotVisible:
		var s AssertNotVisibleStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepAssertTrue:
		var s AssertTrueStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepAssertCondition:
		var s AssertConditionStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepLaunchApp:
		var s LaunchAppStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepStopApp:
		var s StopAppStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepKillApp:
		var s KillAppStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepClearState:
		var s ClearStateStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepClearKeychain:
		var s ClearKeychainStep
		s.StepType = stepType
		return &s, nil

	case StepSetPermissions:
		var s SetPermissionsStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepSetLocation:
		var s SetLocationStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepSetOrientation:
		var s SetOrientationStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepOpenLink:
		var s OpenLinkStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepOpenBrowser:
		var s OpenBrowserStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepPressKey:
		var s PressKeyStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepSleep:
		var s SleepStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepWaitForAnimationToEnd:
		var s WaitForAnimationToEndStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	case StepTakeScreenshot:
		var s TakeScreenshotStep
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		s.StepType = stepType
		return &s, nil

	default:
		return nil, fmt.Errorf("unsupported step type: %s", stepType)
	}
}

// MarshalJSON for Selector: if only Text is set, marshal as a plain string.
// Otherwise marshal as object.
func (s Selector) MarshalJSON() ([]byte, error) {
	if s.isTextOnly() {
		return json.Marshal(s.Text)
	}
	// Use alias to avoid infinite recursion
	type selectorAlias Selector
	return json.Marshal(selectorAlias(s))
}

// UnmarshalJSON for Selector: accept plain string or object.
func (s *Selector) UnmarshalJSON(data []byte) error {
	// Try string first
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		s.Text = text
		return nil
	}
	// Fall back to object
	type selectorAlias Selector
	var alias selectorAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*s = Selector(alias)
	return nil
}

// isTextOnly returns true if only the Text field is set (for compact JSON serialization).
func (s *Selector) isTextOnly() bool {
	return s.Text != "" &&
		s.ID == "" &&
		s.CSS == "" &&
		s.Width == 0 &&
		s.Height == 0 &&
		s.Tolerance == 0 &&
		s.Enabled == nil &&
		s.Selected == nil &&
		s.Checked == nil &&
		s.Focused == nil &&
		s.Index == "" &&
		s.Traits == "" &&
		s.ChildOf == nil &&
		s.Below == nil &&
		s.Above == nil &&
		s.LeftOf == nil &&
		s.RightOf == nil &&
		s.ContainsChild == nil &&
		len(s.ContainsDescendants) == 0 &&
		s.InsideOf == nil
}
