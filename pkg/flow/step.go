// Package flow handles parsing and representation of Maestro YAML flow files.
package flow

// StepType represents the type of step.
type StepType string

// Step type constants.
const (
	// Navigation & Interaction
	StepTapOn              StepType = "tapOn"
	StepDoubleTapOn        StepType = "doubleTapOn"
	StepLongPressOn        StepType = "longPressOn"
	StepTapOnPoint         StepType = "tapOnPoint"
	StepSwipe              StepType = "swipe"
	StepScroll             StepType = "scroll"
	StepScrollUntilVisible StepType = "scrollUntilVisible"
	StepBack               StepType = "back"
	StepHideKeyboard       StepType = "hideKeyboard"
	StepAcceptAlert        StepType = "acceptAlert"
	StepDismissAlert       StepType = "dismissAlert"

	// Text
	StepInputText             StepType = "inputText"
	StepInputRandom           StepType = "inputRandom"
	StepInputRandomEmail      StepType = "inputRandomEmail"      // Shorthand for inputRandom: EMAIL
	StepInputRandomNumber     StepType = "inputRandomNumber"     // Shorthand for inputRandom: NUMBER
	StepInputRandomPersonName StepType = "inputRandomPersonName" // Shorthand for inputRandom: PERSON_NAME
	StepInputRandomText       StepType = "inputRandomText"       // Shorthand for inputRandom: TEXT
	StepEraseText             StepType = "eraseText"
	StepCopyTextFrom          StepType = "copyTextFrom"
	StepPasteText             StepType = "pasteText"
	StepSetClipboard          StepType = "setClipboard"

	// Assertions
	StepAssertVisible         StepType = "assertVisible"
	StepAssertNotVisible      StepType = "assertNotVisible"
	StepAssertTrue            StepType = "assertTrue"
	StepAssertCondition       StepType = "assertCondition"
	StepAssertNoDefectsWithAI StepType = "assertNoDefectsWithAI"
	StepAssertWithAI          StepType = "assertWithAI"
	StepExtractTextWithAI     StepType = "extractTextWithAI"
	StepWaitUntil             StepType = "extendedWaitUntil"

	// App Management
	StepLaunchApp      StepType = "launchApp"
	StepStopApp        StepType = "stopApp"
	StepKillApp        StepType = "killApp"
	StepClearState     StepType = "clearState"
	StepClearKeychain  StepType = "clearKeychain"
	StepSetPermissions StepType = "setPermissions"

	// Device Control
	StepSetLocation        StepType = "setLocation"
	StepSetOrientation     StepType = "setOrientation"
	StepSetAirplaneMode    StepType = "setAirplaneMode"
	StepToggleAirplaneMode StepType = "toggleAirplaneMode"
	StepTravel             StepType = "travel"
	StepOpenLink           StepType = "openLink"
	StepOpenBrowser        StepType = "openBrowser"

	// Flow Control
	StepRepeat     StepType = "repeat"
	StepRetry      StepType = "retry"
	StepRunFlow    StepType = "runFlow"
	StepRunScript  StepType = "runScript"
	StepEvalScript StepType = "evalScript"

	// Media
	StepTakeScreenshot StepType = "takeScreenshot"
	StepStartRecording StepType = "startRecording"
	StepStopRecording  StepType = "stopRecording"
	StepAddMedia       StepType = "addMedia"

	// Other
	StepPressKey              StepType = "pressKey"
	StepWaitForAnimationToEnd StepType = "waitForAnimationToEnd"
	StepDefineVariables       StepType = "defineVariables"
)

// Step is the interface for all flow steps.
type Step interface {
	Type() StepType
	IsOptional() bool
	Label() string
	Describe() string
}

// BaseStep contains common fields for all steps.
type BaseStep struct {
	StepType  StepType `yaml:"-"`
	Optional  bool     `yaml:"optional"`
	StepLabel string   `yaml:"label"`
	TimeoutMs int      `yaml:"timeout"`
}

// Type returns the step type.
func (b *BaseStep) Type() StepType { return b.StepType }

// IsOptional returns whether the step is optional.
func (b *BaseStep) IsOptional() bool { return b.Optional }

// Label returns the step label.
func (b *BaseStep) Label() string { return b.StepLabel }

// Describe returns a human-readable description.
func (b *BaseStep) Describe() string { return string(b.StepType) }

// ============================================
// Navigation & Interaction Steps
// ============================================

// TapOnStep taps on an element.
type TapOnStep struct {
	BaseStep              `yaml:",inline"`
	Selector              Selector `yaml:",inline"`
	LongPress             bool     `yaml:"longPress"`
	Repeat                int      `yaml:"repeat"`
	DelayMs               int      `yaml:"delay"`
	Point                 string   `yaml:"point"`
	RetryTapIfNoChange    *bool    `yaml:"retryTapIfNoChange"`
	WaitUntilVisible      *bool    `yaml:"waitUntilVisible"`
	WaitToSettleTimeoutMs int      `yaml:"waitToSettleTimeoutMs"`
}

// DoubleTapOnStep double taps on an element (alias for tapOn with repeat=2).
type DoubleTapOnStep struct {
	BaseStep              `yaml:",inline"`
	Selector              Selector `yaml:",inline"`
	RetryTapIfNoChange    *bool    `yaml:"retryTapIfNoChange"`
	WaitUntilVisible      *bool    `yaml:"waitUntilVisible"`
	WaitToSettleTimeoutMs int      `yaml:"waitToSettleTimeoutMs"`
}

// LongPressOnStep long presses on an element (alias for tapOn with longPress=true).
type LongPressOnStep struct {
	BaseStep              `yaml:",inline"`
	Selector              Selector `yaml:",inline"`
	RetryTapIfNoChange    *bool    `yaml:"retryTapIfNoChange"`
	WaitUntilVisible      *bool    `yaml:"waitUntilVisible"`
	WaitToSettleTimeoutMs int      `yaml:"waitToSettleTimeoutMs"`
}

// TapOnPointStep taps on specific coordinates.
type TapOnPointStep struct {
	BaseStep              `yaml:",inline"`
	X                     int    `yaml:"x"`
	Y                     int    `yaml:"y"`
	Point                 string `yaml:"point"`
	LongPress             bool   `yaml:"longPress"`
	Repeat                int    `yaml:"repeat"`
	RetryTapIfNoChange    *bool  `yaml:"retryTapIfNoChange"`
	WaitToSettleTimeoutMs int    `yaml:"waitToSettleTimeoutMs"`
}

// SwipeStep performs a swipe gesture.
type SwipeStep struct {
	BaseStep              `yaml:",inline"`
	Direction             string    `yaml:"direction"` // UP, DOWN, LEFT, RIGHT
	Selector              *Selector `yaml:"selector"`
	Start                 string    `yaml:"start"`    // "x%, y%"
	End                   string    `yaml:"end"`      // "x%, y%"
	StartX                int       `yaml:"startX"`   // Absolute X start
	StartY                int       `yaml:"startY"`   // Absolute Y start
	EndX                  int       `yaml:"endX"`     // Absolute X end
	EndY                  int       `yaml:"endY"`     // Absolute Y end
	Duration              int       `yaml:"duration"` // Duration in ms
	Speed                 int       `yaml:"speed"`    // Speed 0-100
	WaitToSettleTimeoutMs int       `yaml:"waitToSettleTimeoutMs"`
}

// ScrollStep scrolls the screen.
type ScrollStep struct {
	BaseStep  `yaml:",inline"`
	Direction string `yaml:"direction"`
}

// ScrollUntilVisibleStep scrolls until element is visible.
type ScrollUntilVisibleStep struct {
	BaseStep              `yaml:",inline"`
	Element               Selector `yaml:"element"`
	Direction             string   `yaml:"direction"`
	MaxScrolls            int      `yaml:"maxScrolls"` // Legacy: max scroll attempts
	Speed                 int      `yaml:"speed"`
	VisibilityPercentage  int      `yaml:"visibilityPercentage"`
	CenterElement         bool     `yaml:"centerElement"`
	WaitToSettleTimeoutMs int      `yaml:"waitToSettleTimeoutMs"`
}

// BackStep presses back.
type BackStep struct {
	BaseStep `yaml:",inline"`
}

// HideKeyboardStep hides the keyboard.
type HideKeyboardStep struct {
	BaseStep `yaml:",inline"`
}

// AcceptAlertStep accepts a system alert dialog (taps Allow/OK).
type AcceptAlertStep struct {
	BaseStep `yaml:",inline"`
}

// DismissAlertStep dismisses a system alert dialog (taps Don't Allow/Cancel).
type DismissAlertStep struct {
	BaseStep `yaml:",inline"`
}

// ============================================
// Text Steps
// ============================================

// InputTextStep inputs text.
type InputTextStep struct {
	BaseStep `yaml:",inline"`
	Text     string   `yaml:"text"`
	KeyPress bool     `yaml:"keyPress"` // If true, simulate real key presses (Android native only)
	Selector Selector `yaml:",inline"`
}

// InputRandomStep generates random input.
type InputRandomStep struct {
	BaseStep `yaml:",inline"`
	DataType string `yaml:"type"` // TEXT, NUMBER, EMAIL, PERSON_NAME, etc.
	Length   int    `yaml:"length"`
}

// EraseTextStep erases text.
type EraseTextStep struct {
	BaseStep   `yaml:",inline"`
	Characters int `yaml:"characters"`
}

// CopyTextFromStep copies text from element.
type CopyTextFromStep struct {
	BaseStep `yaml:",inline"`
	Selector Selector `yaml:",inline"`
}

// PasteTextStep pastes text.
type PasteTextStep struct {
	BaseStep `yaml:",inline"`
}

// SetClipboardStep sets the clipboard to a specific text value.
type SetClipboardStep struct {
	BaseStep `yaml:",inline"`
	Text     string `yaml:"text"`
}

// ============================================
// Assertion Steps
// ============================================

// AssertVisibleStep asserts element is visible.
type AssertVisibleStep struct {
	BaseStep `yaml:",inline"`
	Selector Selector `yaml:",inline"`
}

// AssertNotVisibleStep asserts element is not visible.
type AssertNotVisibleStep struct {
	BaseStep `yaml:",inline"`
	Selector Selector `yaml:",inline"`
}

// AssertTrueStep asserts a script condition is true (alias for assertCondition).
type AssertTrueStep struct {
	BaseStep `yaml:",inline"`
	Script   string `yaml:"condition"`
}

// Condition represents a test condition.
type Condition struct {
	Visible    *Selector `yaml:"visible"`
	NotVisible *Selector `yaml:"notVisible"`
	Script     string    `yaml:"scriptCondition"`
	Platform   string    `yaml:"platform"`
}

// AssertConditionStep asserts a condition.
type AssertConditionStep struct {
	BaseStep  `yaml:",inline"`
	Condition Condition `yaml:",inline"`
}

// AssertNoDefectsWithAIStep uses AI to check for visual defects.
type AssertNoDefectsWithAIStep struct {
	BaseStep `yaml:",inline"`
}

// AssertWithAIStep uses AI to verify an assertion.
type AssertWithAIStep struct {
	BaseStep  `yaml:",inline"`
	Assertion string `yaml:"assertion"`
}

// ExtractTextWithAIStep uses AI to extract text from screen.
type ExtractTextWithAIStep struct {
	BaseStep `yaml:",inline"`
	Query    string `yaml:"query"`
	Variable string `yaml:"variable"` // Variable to store result
}

// WaitUntilStep waits for a condition.
type WaitUntilStep struct {
	BaseStep   `yaml:",inline"`
	Visible    *Selector `yaml:"visible"`
	NotVisible *Selector `yaml:"notVisible"`
}

// ============================================
// App Management Steps
// ============================================

// LaunchAppStep launches an app.
type LaunchAppStep struct {
	BaseStep      `yaml:",inline"`
	AppID         string            `yaml:"appId"`
	ClearState    bool              `yaml:"clearState"`
	ClearKeychain bool              `yaml:"clearKeychain"`
	StopApp       *bool             `yaml:"stopApp"`
	Permissions   map[string]string `yaml:"permissions"`
	Arguments     map[string]any    `yaml:"arguments"` // Can be list or map
}

// StopAppStep stops an app.
type StopAppStep struct {
	BaseStep `yaml:",inline"`
	AppID    string `yaml:"appId"`
}

// KillAppStep kills an app.
type KillAppStep struct {
	BaseStep `yaml:",inline"`
	AppID    string `yaml:"appId"`
}

// ClearStateStep clears app state.
type ClearStateStep struct {
	BaseStep `yaml:",inline"`
	AppID    string `yaml:"appId"`
}

// ClearKeychainStep clears keychain.
type ClearKeychainStep struct {
	BaseStep `yaml:",inline"`
}

// SetPermissionsStep sets app permissions.
// Permission values: "allow", "deny", "unset"
// Permission shortcuts: location, camera, contacts, phone, microphone,
// bluetooth, storage, notifications, medialibrary, calendar, sms, all
type SetPermissionsStep struct {
	BaseStep    `yaml:",inline"`
	AppID       string            `yaml:"appId"`
	Permissions map[string]string `yaml:"permissions"`
}

// ============================================
// Device Control Steps
// ============================================

// SetLocationStep sets device location.
type SetLocationStep struct {
	BaseStep  `yaml:",inline"`
	Latitude  string `yaml:"latitude"`  // String for variable support
	Longitude string `yaml:"longitude"` // String for variable support
}

// SetOrientationStep sets device orientation.
type SetOrientationStep struct {
	BaseStep    `yaml:",inline"`
	Orientation string `yaml:"orientation"` // PORTRAIT, LANDSCAPE
}

// SetAirplaneModeStep sets airplane mode.
type SetAirplaneModeStep struct {
	BaseStep `yaml:",inline"`
	Enabled  bool `yaml:"enabled"`
}

// ToggleAirplaneModeStep toggles airplane mode.
type ToggleAirplaneModeStep struct {
	BaseStep `yaml:",inline"`
}

// TravelStep simulates travel.
type TravelStep struct {
	BaseStep `yaml:",inline"`
	Points   []string `yaml:"points"` // "lat, long"
	Speed    float64  `yaml:"speed"`  // km/h
}

// OpenLinkStep opens a URL.
type OpenLinkStep struct {
	BaseStep   `yaml:",inline"`
	Link       string `yaml:"link"`
	AutoVerify *bool  `yaml:"autoVerify"`
	Browser    *bool  `yaml:"browser"`
}

// OpenBrowserStep opens a URL in the browser.
type OpenBrowserStep struct {
	BaseStep `yaml:",inline"`
	URL      string `yaml:"url"`
}

// ============================================
// Flow Control Steps
// ============================================

// RepeatStep repeats steps.
type RepeatStep struct {
	BaseStep `yaml:",inline"`
	Times    string    `yaml:"times"` // String for variable support
	While    Condition `yaml:"while"`
	Steps    []Step    `yaml:"-"`
}

// RetryStep retries steps on failure.
type RetryStep struct {
	BaseStep   `yaml:",inline"`
	MaxRetries string            `yaml:"maxRetries"` // String for variable support
	Steps      []Step            `yaml:"-"`
	File       string            `yaml:"file"`
	Env        map[string]string `yaml:"env"`
}

// RunFlowStep runs another flow.
type RunFlowStep struct {
	BaseStep `yaml:",inline"`
	File     string            `yaml:"file"`
	Steps    []Step            `yaml:"-"` // Inline steps
	When     *Condition        `yaml:"when"`
	Env      map[string]string `yaml:"env"`
}

// RunScriptStep runs a script.
type RunScriptStep struct {
	BaseStep `yaml:",inline"`
	Script   string            `yaml:"script"` // Script content or filename (string form)
	File     string            `yaml:"file"`   // Script filename (map form)
	Env      map[string]string `yaml:"env"`
}

// ScriptPath returns the script path (either Script or File field).
func (s *RunScriptStep) ScriptPath() string {
	if s.File != "" {
		return s.File
	}
	return s.Script
}

// EvalScriptStep evaluates JavaScript.
type EvalScriptStep struct {
	BaseStep `yaml:",inline"`
	Script   string `yaml:"script"`
}

// ============================================
// Media Steps
// ============================================

// TakeScreenshotStep takes a screenshot.
type TakeScreenshotStep struct {
	BaseStep `yaml:",inline"`
	Path     string `yaml:"path"`
}

// StartRecordingStep starts recording.
type StartRecordingStep struct {
	BaseStep `yaml:",inline"`
	Path     string `yaml:"path"`
}

// StopRecordingStep stops recording.
type StopRecordingStep struct {
	BaseStep `yaml:",inline"`
	Path     string `yaml:"path"`
}

// AddMediaStep adds media files.
type AddMediaStep struct {
	BaseStep `yaml:",inline"`
	Files    []string `yaml:"files"`
}

// ============================================
// Other Steps
// ============================================

// PressKeyStep presses a key.
type PressKeyStep struct {
	BaseStep `yaml:",inline"`
	Key      string `yaml:"key"`
}

// WaitForAnimationToEndStep waits for animations.
type WaitForAnimationToEndStep struct {
	BaseStep `yaml:",inline"`
}

// DefineVariablesStep defines variables.
type DefineVariablesStep struct {
	BaseStep `yaml:",inline"`
	Env      map[string]string `yaml:"env"`
}

// UnsupportedStep represents an unsupported step.
type UnsupportedStep struct {
	BaseStep `yaml:",inline"`
	Reason   string
}

// Describe returns a description including the unsupported reason.
func (s *UnsupportedStep) Describe() string {
	return string(s.StepType) + " (unsupported: " + s.Reason + ")"
}

// ============================================
// Describe() implementations for detailed output
// ============================================

// Describe returns a human-readable description of the tap step.
func (s *TapOnStep) Describe() string {
	return "tapOn: " + s.Selector.DescribeQuoted()
}

// Describe returns a human-readable description of the double tap step.
func (s *DoubleTapOnStep) Describe() string {
	return "doubleTapOn: " + s.Selector.DescribeQuoted()
}

// Describe returns a human-readable description of the long press step.
func (s *LongPressOnStep) Describe() string {
	return "longPressOn: " + s.Selector.DescribeQuoted()
}

// Describe returns a human-readable description of the assert visible step.
func (s *AssertVisibleStep) Describe() string {
	return "assertVisible: " + s.Selector.DescribeQuoted()
}

// Describe returns a human-readable description of the assert not visible step.
func (s *AssertNotVisibleStep) Describe() string {
	return "assertNotVisible: " + s.Selector.DescribeQuoted()
}

// Describe returns a human-readable description of the input text step.
func (s *InputTextStep) Describe() string {
	return "inputText: \"" + s.Text + "\""
}

// Describe returns a human-readable description of the launch app step.
func (s *LaunchAppStep) Describe() string {
	if s.ClearState {
		return "launchApp (clearState)"
	}
	return "launchApp"
}

// Describe returns a human-readable description of the wait until step.
func (s *WaitUntilStep) Describe() string {
	if s.Visible != nil {
		return "extendedWaitUntil: visible " + s.Visible.DescribeQuoted()
	}
	if s.NotVisible != nil {
		return "extendedWaitUntil: notVisible " + s.NotVisible.DescribeQuoted()
	}
	return "extendedWaitUntil"
}

// Describe returns a human-readable description of the scroll until visible step.
func (s *ScrollUntilVisibleStep) Describe() string {
	return "scrollUntilVisible: " + s.Element.DescribeQuoted()
}

// Describe returns a human-readable description of the copy text step.
func (s *CopyTextFromStep) Describe() string {
	return "copyTextFrom: " + s.Selector.DescribeQuoted()
}

// Describe returns a human-readable description of the run flow step.
func (s *RunFlowStep) Describe() string {
	if s.File != "" {
		return "runFlow: " + s.File
	}
	return "runFlow"
}

// Describe returns a human-readable description of the press key step.
func (s *PressKeyStep) Describe() string {
	return "pressKey: " + s.Key
}

// Describe returns a human-readable description of the swipe step.
func (s *SwipeStep) Describe() string {
	if s.Direction != "" {
		return "swipe: " + s.Direction
	}
	return "swipe"
}

// Describe returns a human-readable description of the scroll step.
func (s *ScrollStep) Describe() string {
	if s.Direction != "" {
		return "scroll: " + s.Direction
	}
	return "scroll"
}

// Describe returns a human-readable description of the set permissions step.
func (s *SetPermissionsStep) Describe() string {
	return "setPermissions"
}
