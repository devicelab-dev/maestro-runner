// Package flow handles parsing and representation of Maestro YAML flow files.
package flow

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// StepType represents the type of step.
type StepType string

// Step type constants.
const (
	// Navigation & Interaction
	StepTapOn              StepType = "tapOn"
	StepDoubleTapOn        StepType = "doubleTapOn"
	StepLongPressOn        StepType = "longPressOn"
	StepTapOnPoint         StepType = "tapOnPoint"
	StepDragAndDrop        StepType = "dragAndDrop"
	StepSwipe              StepType = "swipe"
	StepScroll             StepType = "scroll"
	StepScrollUntilVisible StepType = "scrollUntilVisible"
	StepBack               StepType = "back"
	StepHideKeyboard       StepType = "hideKeyboard"
	StepOpenNotifications  StepType = "openNotifications"
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
	StepSetDarkMode        StepType = "setDarkMode"
	StepToggleDarkMode     StepType = "toggleDarkMode"
	StepAssertDarkMode     StepType = "assertDarkMode"
	StepAssertLightMode    StepType = "assertLightMode"
	StepTravel             StepType = "travel"
	StepOpenLink           StepType = "openLink"
	StepOpenBrowser        StepType = "openBrowser"

	// Flow Control
	StepRepeat            StepType = "repeat"
	StepRetry             StepType = "retry"
	StepRunFlow           StepType = "runFlow"
	StepRunScript         StepType = "runScript"
	StepEvalScript        StepType = "evalScript"
	StepEvalBrowserScript StepType = "evalBrowserScript"
	StepRunBrowserScript  StepType = "runBrowserScript"
	StepEvalWebViewScript StepType = "evalWebViewScript"
	StepRunWebViewScript  StepType = "runWebViewScript"
	StepGetConsoleLogs    StepType = "getConsoleLogs"
	StepClearConsoleLogs  StepType = "clearConsoleLogs"
	StepAssertNoJSErrors  StepType = "assertNoJSErrors"

	// Browser State (web-only)
	StepSetCookies    StepType = "setCookies"
	StepGetCookies    StepType = "getCookies"
	StepSaveAuthState StepType = "saveAuthState"
	StepLoadAuthState StepType = "loadAuthState"

	// Browser File & Permissions (web-only)
	StepUploadFile       StepType = "uploadFile"
	StepWaitForDownload  StepType = "waitForDownload"
	StepGrantPermissions StepType = "grantPermissions"
	StepResetPermissions StepType = "resetPermissions"

	// Browser Tab Management (web-only)
	StepOpenTab   StepType = "openTab"
	StepSwitchTab StepType = "switchTab"
	StepCloseTab  StepType = "closeTab"

	// Browser Network Interception (web-only)
	StepMockNetwork          StepType = "mockNetwork"
	StepBlockNetwork         StepType = "blockNetwork"
	StepSetNetworkConditions StepType = "setNetworkConditions"
	StepWaitForRequest       StepType = "waitForRequest"
	StepClearNetworkMocks    StepType = "clearNetworkMocks"

	// Media
	StepTakeScreenshot   StepType = "takeScreenshot"
	StepAssertScreenshot StepType = "assertScreenshot"
	StepStartRecording   StepType = "startRecording"
	StepStopRecording    StepType = "stopRecording"
	StepAddMedia         StepType = "addMedia"
	StepRemoveMedia      StepType = "removeMedia"

	// Queries
	StepIsKeyboardVisible StepType = "isKeyboardVisible"

	// Other
	StepSleep                 StepType = "sleep"
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
	// PlatformGate returns the platform this step is restricted to
	// ("ios"/"android"/"web", lowercased) or "" to run on all platforms.
	PlatformGate() string
}

// BaseStep contains common fields for all steps.
type BaseStep struct {
	StepType  StepType `yaml:"-" json:"type"`
	Optional  bool     `yaml:"optional" json:"optional,omitempty"`
	StepLabel string   `yaml:"label" json:"label,omitempty"`
	TimeoutMs int      `yaml:"timeout" json:"timeout,omitempty"`
	// Platform restricts this step to a single platform; when set and it
	// doesn't match the running driver, the step is skipped (Maestro #1353).
	Platform string `yaml:"platform" json:"platform,omitempty"`
}

// PlatformGate returns the step's platform restriction, lowercased, or "".
func (b *BaseStep) PlatformGate() string { return strings.ToLower(strings.TrimSpace(b.Platform)) }

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
	BaseStep              `yaml:",inline" json:",inline"`
	Selector              Selector `yaml:",inline" json:"selector"`
	LongPress             bool     `yaml:"longPress" json:"longPress,omitempty"`
	Repeat                int      `yaml:"repeat" json:"repeat,omitempty"`
	DelayMs               int      `yaml:"delay" json:"delay,omitempty"`
	DurationMs            int      `yaml:"duration" json:"duration,omitempty"`
	Point                 string   `yaml:"point" json:"point,omitempty"`
	RetryTapIfNoChange    *bool    `yaml:"retryTapIfNoChange" json:"retryTapIfNoChange,omitempty"`
	WaitUntilVisible      *bool    `yaml:"waitUntilVisible" json:"waitUntilVisible,omitempty"`
	WaitToSettleTimeoutMs int      `yaml:"waitToSettleTimeoutMs" json:"waitToSettleTimeoutMs,omitempty"`
}

// DoubleTapOnStep double taps on an element (alias for tapOn with repeat=2).
type DoubleTapOnStep struct {
	BaseStep              `yaml:",inline" json:",inline"`
	Selector              Selector `yaml:",inline" json:"selector"`
	RetryTapIfNoChange    *bool    `yaml:"retryTapIfNoChange" json:"retryTapIfNoChange,omitempty"`
	WaitUntilVisible      *bool    `yaml:"waitUntilVisible" json:"waitUntilVisible,omitempty"`
	WaitToSettleTimeoutMs int      `yaml:"waitToSettleTimeoutMs" json:"waitToSettleTimeoutMs,omitempty"`
}

// LongPressOnStep long presses on an element (alias for tapOn with longPress=true).
type LongPressOnStep struct {
	BaseStep              `yaml:",inline" json:",inline"`
	Selector              Selector `yaml:",inline" json:"selector"`
	DurationMs            int      `yaml:"duration" json:"duration,omitempty"`
	RetryTapIfNoChange    *bool    `yaml:"retryTapIfNoChange" json:"retryTapIfNoChange,omitempty"`
	WaitUntilVisible      *bool    `yaml:"waitUntilVisible" json:"waitUntilVisible,omitempty"`
	WaitToSettleTimeoutMs int      `yaml:"waitToSettleTimeoutMs" json:"waitToSettleTimeoutMs,omitempty"`
}

// TapOnPointStep taps on specific coordinates.
type TapOnPointStep struct {
	BaseStep              `yaml:",inline" json:",inline"`
	X                     int    `yaml:"x" json:"x,omitempty"`
	Y                     int    `yaml:"y" json:"y,omitempty"`
	Point                 string `yaml:"point" json:"point,omitempty"`
	LongPress             bool   `yaml:"longPress" json:"longPress,omitempty"`
	Repeat                int    `yaml:"repeat" json:"repeat,omitempty"`
	DurationMs            int    `yaml:"duration" json:"duration,omitempty"`
	RetryTapIfNoChange    *bool  `yaml:"retryTapIfNoChange" json:"retryTapIfNoChange,omitempty"`
	WaitToSettleTimeoutMs int    `yaml:"waitToSettleTimeoutMs" json:"waitToSettleTimeoutMs,omitempty"`
}

// SwipeStep performs a swipe gesture.
//
// `Selector` anchors the swipe on an element. It is populated from either the
// `from:` YAML key (matching upstream Maestro syntax) or the historical
// `selector:` key. Custom `UnmarshalYAML` handles the mapping — both keys
// resolve to the same field, with `from:` taking precedence if both are set.
// DragAndDropStep long-presses one element (or point) and drags it to another,
// the way list-reorder and drag-handle UIs expect: press, hold until the item
// lifts, move, settle, release.
type DragAndDropStep struct {
	BaseStep `yaml:",inline"`
	From     Selector `yaml:"from"` // what to pick up — selector or point
	To       Selector `yaml:"to"`   // where to drop it — selector or point
	// HoldDuration is how long to press before moving (ms). Reorder UIs lift
	// the item only after a long press, so the default is a full second.
	HoldDuration int `yaml:"holdDuration"`
	// Duration is how long the movement itself takes (ms). Drag targets track
	// the finger, so moving too fast skips drop zones; default one second.
	Duration int `yaml:"duration"`
}

// Describe returns a human-readable description of the drag step.
func (s *DragAndDropStep) Describe() string {
	return "dragAndDrop: " + s.From.DescribeQuoted() + " → " + s.To.DescribeQuoted()
}

type SwipeStep struct {
	BaseStep              `yaml:",inline" json:",inline"`
	Direction             string    `yaml:"direction" json:"direction,omitempty"` // UP, DOWN, LEFT, RIGHT
	Selector              *Selector `yaml:"-" json:"selector,omitempty"`
	Start                 string    `yaml:"start" json:"start,omitempty"`       // "x%, y%"
	End                   string    `yaml:"end" json:"end,omitempty"`           // "x%, y%"
	StartX                int       `yaml:"startX" json:"startX,omitempty"`     // Absolute X start
	StartY                int       `yaml:"startY" json:"startY,omitempty"`     // Absolute Y start
	EndX                  int       `yaml:"endX" json:"endX,omitempty"`         // Absolute X end
	EndY                  int       `yaml:"endY" json:"endY,omitempty"`         // Absolute Y end
	Duration              int       `yaml:"duration" json:"duration,omitempty"` // Duration in ms
	Speed                 int       `yaml:"speed" json:"speed,omitempty"`       // Speed 0-100
	Distance              float64   `yaml:"distance" json:"distance,omitempty"` // Fraction of screen (0-1) for direction swipes; 0 = default
	WaitToSettleTimeoutMs int       `yaml:"waitToSettleTimeoutMs" json:"waitToSettleTimeoutMs,omitempty"`
}

// UnmarshalYAML decodes SwipeStep and maps both `from:` (upstream Maestro
// spelling) and `selector:` (historical) to the `Selector` field. Without
// this, `from: {id: X}` was silently discarded and direction-swipes fell
// through to a screen-percentage swipe that ignores the anchor element.
// See devicelab-dev/maestro-runner#112.
func (s *SwipeStep) UnmarshalYAML(node *yaml.Node) error {
	type swipeAlias SwipeStep
	var a swipeAlias
	if err := node.Decode(&a); err != nil {
		return err
	}
	*s = SwipeStep(a)

	var anchor struct {
		From     *Selector `yaml:"from"`
		Selector *Selector `yaml:"selector"`
	}
	if err := node.Decode(&anchor); err != nil {
		return err
	}
	if anchor.From != nil {
		s.Selector = anchor.From
	} else if anchor.Selector != nil {
		s.Selector = anchor.Selector
	}
	return nil
}

// ScrollStep scrolls the screen.
type ScrollStep struct {
	BaseStep  `yaml:",inline" json:",inline"`
	Direction string `yaml:"direction" json:"direction,omitempty"`
	// Engine selects the scroll backend on Android.
	// "" (default) and "adb" → adb input swipe (matches upstream Maestro).
	// "agent" → driver's existing on-device gesture path (UIA2 server
	// /appium/gestures/scroll for the uiautomator2 driver, RPC MotionEvent
	// injection for the devicelab driver). Ignored on iOS/web.
	Engine string `yaml:"engine" json:"engine,omitempty"`
}

// ScrollUntilVisibleStep scrolls until element is visible.
type ScrollUntilVisibleStep struct {
	BaseStep              `yaml:",inline" json:",inline"`
	Element               Selector `yaml:"element" json:"element"`
	Direction             string   `yaml:"direction" json:"direction,omitempty"`
	MaxScrolls            int      `yaml:"maxScrolls" json:"maxScrolls,omitempty"` // Legacy: max scroll attempts
	Speed                 int      `yaml:"speed" json:"speed,omitempty"`
	VisibilityPercentage  int      `yaml:"visibilityPercentage" json:"visibilityPercentage,omitempty"`
	CenterElement         bool     `yaml:"centerElement" json:"centerElement,omitempty"`
	WaitToSettleTimeoutMs int      `yaml:"waitToSettleTimeoutMs" json:"waitToSettleTimeoutMs,omitempty"`
	// Engine selects the scroll backend. See ScrollStep.Engine.
	Engine string `yaml:"engine" json:"engine,omitempty"`
}

// BackStep presses back.
type BackStep struct {
	BaseStep `yaml:",inline" json:",inline"`
}

// HideKeyboardStep hides the keyboard.
// Strategy can be empty (try all), "appium", "escape", or "back".
type HideKeyboardStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Strategy string `yaml:"strategy" json:"strategy,omitempty"`
}

// IsKeyboardVisibleStep queries whether the soft keyboard is currently shown.
type IsKeyboardVisibleStep struct {
	BaseStep `yaml:",inline" json:",inline"`
}

// OpenNotificationsStep pulls down the Android notification shade.
// Android-only (no-op on iOS).
type OpenNotificationsStep struct {
	BaseStep `yaml:",inline"`
}

// AcceptAlertStep accepts a system alert dialog (taps Allow/OK).
type AcceptAlertStep struct {
	BaseStep `yaml:",inline" json:",inline"`
}

// DismissAlertStep dismisses a system alert dialog (taps Don't Allow/Cancel).
type DismissAlertStep struct {
	BaseStep `yaml:",inline" json:",inline"`
}

// ============================================
// Text Steps
// ============================================

// InputTextStep inputs text.
type InputTextStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Text     string   `yaml:"text" json:"text,omitempty"`
	KeyPress bool     `yaml:"keyPress" json:"keyPress,omitempty"` // If true, simulate real key presses (Android native only)
	Selector Selector `yaml:",inline" json:"selector,omitempty"`
}

// InputRandomStep generates random input.
type InputRandomStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	DataType string `yaml:"type" json:"dataType,omitempty"` // TEXT, NUMBER, EMAIL, PERSON_NAME, etc.
	Length   int    `yaml:"length" json:"length,omitempty"`
}

// EraseTextStep erases text.
type EraseTextStep struct {
	BaseStep   `yaml:",inline" json:",inline"`
	Characters int `yaml:"characters" json:"charactersToErase,omitempty"`
}

// CopyTextFromStep copies text from element.
type CopyTextFromStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Selector Selector `yaml:",inline" json:"selector"`
}

// PasteTextStep pastes text.
type PasteTextStep struct {
	BaseStep `yaml:",inline" json:",inline"`
}

// SetClipboardStep sets the clipboard to a specific text value.
type SetClipboardStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Text     string `yaml:"text" json:"text,omitempty"`
}

// ============================================
// Assertion Steps
// ============================================

// AssertVisibleStep asserts element is visible.
type AssertVisibleStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Selector Selector `yaml:",inline" json:"selector"`
	// Count asserts that the selector matches exactly N visible elements
	// (Maestro #1363). A string so flows can write `count: ${N}`; empty means
	// the ordinary at-least-one assertion.
	Count string `yaml:"count" json:"count,omitempty"`
}

// ExpectedCount resolves the step's count assertion. Returns (0, false, nil)
// when no count was requested. The value must be a positive integer once
// variables are expanded — zero would silently shadow assertNotVisible, and
// anything unparseable is a flow bug worth failing loudly on.
func (s *AssertVisibleStep) ExpectedCount() (int, bool, error) {
	if s.Count == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(s.Count))
	if err != nil {
		return 0, false, fmt.Errorf("assertVisible count: %q is not a number", s.Count)
	}
	if n < 1 {
		return 0, false, fmt.Errorf("assertVisible count: must be at least 1 (got %d) — use assertNotVisible to assert absence", n)
	}
	return n, true, nil
}

// AssertNotVisibleStep asserts element is not visible.
type AssertNotVisibleStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Selector Selector `yaml:",inline" json:"selector"`
}

// AssertTrueStep asserts a script condition is true (alias for assertCondition).
type AssertTrueStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Script   string `yaml:"condition" json:"condition,omitempty"`
}

// Condition represents a test condition.
type Condition struct {
	Visible    *Selector `yaml:"visible" json:"visible,omitempty"`
	NotVisible *Selector `yaml:"notVisible" json:"notVisible,omitempty"`
	Script     string    `yaml:"true" json:"scriptCondition,omitempty"`
	Platform   string    `yaml:"platform" json:"platform,omitempty"`
	Timeout    int       `yaml:"timeout" json:"timeout,omitempty"` // Timeout in ms for visible/notVisible checks
}

// AssertConditionStep asserts a condition.
// Uses a custom UnmarshalYAML because both BaseStep and Condition have a
// "timeout" yaml tag — Condition.Timeout is the semantically correct one here
// (controls how long to wait for visible/notVisible condition checks).
type AssertConditionStep struct {
	BaseStep  `yaml:"-"`
	Condition Condition `yaml:",inline"`
}

// UnmarshalYAML decodes AssertConditionStep, mapping "timeout" to Condition.Timeout
// and "optional"/"label" to BaseStep fields without a duplicate-key conflict.
func (s *AssertConditionStep) UnmarshalYAML(node *yaml.Node) error {
	if err := node.Decode(&s.Condition); err != nil {
		return err
	}
	type baseFields struct {
		Optional  bool   `yaml:"optional"`
		StepLabel string `yaml:"label"`
	}
	var b baseFields
	if err := node.Decode(&b); err != nil {
		return err
	}
	s.Optional = b.Optional
	s.StepLabel = b.StepLabel
	return nil
}

// AssertNoDefectsWithAIStep uses AI to check for visual defects.
type AssertNoDefectsWithAIStep struct {
	BaseStep `yaml:",inline" json:",inline"`
}

// AssertWithAIStep uses AI to verify an assertion.
type AssertWithAIStep struct {
	BaseStep  `yaml:",inline" json:",inline"`
	Assertion string `yaml:"assertion" json:"assertion,omitempty"`
}

// ExtractTextWithAIStep uses AI to extract text from screen.
type ExtractTextWithAIStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Query    string `yaml:"query" json:"query,omitempty"`
	Variable string `yaml:"variable" json:"variable,omitempty"` // Variable to store result
}

// WaitUntilStep waits for a condition.
type WaitUntilStep struct {
	BaseStep   `yaml:",inline" json:",inline"`
	Visible    *Selector `yaml:"visible" json:"visible,omitempty"`
	NotVisible *Selector `yaml:"notVisible" json:"notVisible,omitempty"`
}

// ============================================
// App Management Steps
// ============================================

// LaunchAppStep launches an app.
type LaunchAppStep struct {
	BaseStep      `yaml:",inline" json:",inline"`
	AppID         string            `yaml:"appId" json:"appId,omitempty"`
	ClearState    bool              `yaml:"clearState" json:"clearState,omitempty"`
	ClearKeychain bool              `yaml:"clearKeychain" json:"clearKeychain,omitempty"`
	StopApp       *bool             `yaml:"stopApp" json:"stopApp,omitempty"`
	NewSession    bool              `yaml:"newSession" json:"newSession,omitempty"` // Appium only: create fresh session
	Permissions   map[string]string `yaml:"permissions" json:"permissions,omitempty"`
	Arguments     map[string]any    `yaml:"arguments" json:"arguments,omitempty"`     // Launch arguments (-key value pairs)
	Environment   map[string]string `yaml:"environment" json:"environment,omitempty"` // Launch environment variables
}

// StopAppStep stops an app.
type StopAppStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	AppID    string `yaml:"appId" json:"appId,omitempty"`
}

// KillAppStep kills an app.
type KillAppStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	AppID    string `yaml:"appId" json:"appId,omitempty"`
}

// ClearStateStep clears app state.
type ClearStateStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	AppID    string `yaml:"appId" json:"appId,omitempty"`
}

// ClearKeychainStep clears keychain.
type ClearKeychainStep struct {
	BaseStep `yaml:",inline" json:",inline"`
}

// SetPermissionsStep sets app permissions.
// Permission values: "allow", "deny", "unset"
// Permission shortcuts: location, camera, contacts, phone, microphone,
// bluetooth, storage, notifications, medialibrary, calendar, sms, all
type SetPermissionsStep struct {
	BaseStep    `yaml:",inline" json:",inline"`
	AppID       string            `yaml:"appId" json:"appId,omitempty"`
	Permissions map[string]string `yaml:"permissions" json:"permissions,omitempty"`
}

// ============================================
// Device Control Steps
// ============================================

// SetLocationStep sets device location.
type SetLocationStep struct {
	BaseStep  `yaml:",inline" json:",inline"`
	Latitude  string `yaml:"latitude" json:"latitude,omitempty"`   // String for variable support
	Longitude string `yaml:"longitude" json:"longitude,omitempty"` // String for variable support
}

// SetOrientationStep sets device orientation.
type SetOrientationStep struct {
	BaseStep    `yaml:",inline" json:",inline"`
	Orientation string `yaml:"orientation" json:"orientation,omitempty"` // PORTRAIT, LANDSCAPE
}

// SetAirplaneModeStep sets airplane mode.
//
// YAML accepts either a bool (`enabled: true`) or a string that may contain
// variable interpolation (`enabled: ${OFFLINE}`). EnabledRaw captures the raw
// YAML scalar; the executor's variable-expansion pass writes the resolved
// boolean into Enabled before the driver runs the step.
type SetAirplaneModeStep struct {
	BaseStep   `yaml:",inline" json:",inline"`
	Enabled    bool `yaml:"-" json:"enabled,omitempty"`
	EnabledRaw any  `yaml:"enabled" json:"-"`
}

// ToggleAirplaneModeStep toggles airplane mode.
type ToggleAirplaneModeStep struct {
	BaseStep `yaml:",inline" json:",inline"`
}

// SetDarkModeStep switches the system UI between dark and light appearance.
//
// Mirrors SetAirplaneModeStep: YAML accepts a bool (`enabled: true`) or a
// string carrying a variable (`enabled: ${DARK}`), with EnabledRaw holding the
// raw scalar until the executor's expansion pass resolves it.
type SetDarkModeStep struct {
	BaseStep   `yaml:",inline"`
	Enabled    bool `yaml:"-"`
	EnabledRaw any  `yaml:"enabled"`
}

// ToggleDarkModeStep flips the current appearance.
type ToggleDarkModeStep struct {
	BaseStep `yaml:",inline"`
}

// AssertDarkModeStep asserts the system UI is currently in dark appearance.
type AssertDarkModeStep struct {
	BaseStep `yaml:",inline"`
}

// AssertLightModeStep asserts the system UI is currently in light appearance.
type AssertLightModeStep struct {
	BaseStep `yaml:",inline"`
}

// TravelStep simulates travel.
type TravelStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Points   []string `yaml:"points" json:"points,omitempty"` // "lat, long"
	Speed    float64  `yaml:"speed" json:"speed,omitempty"`   // km/h
}

// OpenLinkStep opens a URL.
type OpenLinkStep struct {
	BaseStep   `yaml:",inline" json:",inline"`
	Link       string `yaml:"link" json:"link,omitempty"`
	AutoVerify *bool  `yaml:"autoVerify" json:"autoVerify,omitempty"`
	Browser    *bool  `yaml:"browser" json:"browser,omitempty"`
}

// OpenBrowserStep opens a URL in the browser.
type OpenBrowserStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	URL      string `yaml:"url" json:"url,omitempty"`
}

// ============================================
// Flow Control Steps
// ============================================

// RepeatStep repeats steps.
type RepeatStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Times    string    `yaml:"times" json:"times,omitempty"` // String for variable support
	While    Condition `yaml:"while" json:"while,omitempty"`
	Steps    []Step    `yaml:"-" json:"-"`
}

// RetryStep retries steps on failure.
type RetryStep struct {
	BaseStep   `yaml:",inline" json:",inline"`
	MaxRetries string            `yaml:"maxRetries" json:"maxRetries,omitempty"` // String for variable support
	Steps      []Step            `yaml:"-" json:"-"`
	File       string            `yaml:"file" json:"file,omitempty"`
	Env        map[string]string `yaml:"env" json:"env,omitempty"`
}

// RunFlowStep runs another flow.
//
// When `when:` evaluates false, execution falls through to the else branch
// (ElseFile / ElseSteps). If no else branch is set, the step is skipped.
type RunFlowStep struct {
	BaseStep  `yaml:",inline" json:",inline"`
	File      string            `yaml:"file" json:"file,omitempty"`
	Steps     []Step            `yaml:"-" json:"-"` // Inline steps (commands)
	ElseFile  string            `yaml:"-" json:"elseFile,omitempty"` // Fallback flow file when `when` is false
	ElseSteps []Step            `yaml:"-" json:"elseSteps,omitempty"` // Inline fallback steps (else / elseCommands)
	When      *Condition        `yaml:"when" json:"when,omitempty"`
	Env       map[string]string `yaml:"env" json:"env,omitempty"`
}

// RunScriptStep runs a script.
type RunScriptStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Script   string            `yaml:"script" json:"script,omitempty"` // Script content or filename (string form)
	File     string            `yaml:"file" json:"file,omitempty"`     // Script filename (map form)
	Env      map[string]string `yaml:"env" json:"env,omitempty"`
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
	BaseStep `yaml:",inline" json:",inline"`
	Script   string `yaml:"script" json:"script,omitempty"`
}

// EvalBrowserScriptStep executes JavaScript in the browser page context (web only).
// Unlike EvalScriptStep which runs in Maestro's internal JS engine, this runs
// directly in the browser via CDP, with access to window, document, DOM, etc.
type EvalBrowserScriptStep struct {
	BaseStep `yaml:",inline"`
	Script   string `yaml:"script"` // JS code to execute in the browser
	Output   string `yaml:"output"` // Variable name to store the return value
}

// RunBrowserScriptStep loads and executes a JS file in the browser page context.
type RunBrowserScriptStep struct {
	BaseStep `yaml:",inline"`
	File     string            `yaml:"file"`   // Path to JS file
	Env      map[string]string `yaml:"env"`    // Environment variables injected as window.__env
	Output   string            `yaml:"output"` // Variable name to store the return value
}

// EvalWebViewScriptStep executes JavaScript in a mobile WebView via CDP.
// Unlike EvalBrowserScriptStep (desktop browser), this runs in the WebView
// context of a native Android app.
type EvalWebViewScriptStep struct {
	BaseStep `yaml:",inline"`
	Script   string `yaml:"script"` // JS code to execute in the WebView
	Output   string `yaml:"output"` // Variable name to store the return value
}

// RunWebViewScriptStep loads and executes a JS file in a mobile WebView via CDP.
type RunWebViewScriptStep struct {
	BaseStep `yaml:",inline"`
	File     string            `yaml:"file"`   // Path to JS file
	Env      map[string]string `yaml:"env"`    // Environment variables injected as window.__env
	Output   string            `yaml:"output"` // Variable name to store the return value
}

// GetConsoleLogsStep retrieves captured browser console logs as JSON.
type GetConsoleLogsStep struct {
	BaseStep `yaml:",inline"`
	Output   string `yaml:"output"` // Variable name to store JSON result
}

// ClearConsoleLogsStep clears captured browser console logs.
type ClearConsoleLogsStep struct {
	BaseStep `yaml:",inline"`
}

// AssertNoJSErrorsStep asserts that no console errors or uncaught exceptions occurred.
type AssertNoJSErrorsStep struct {
	BaseStep `yaml:",inline"`
}

// ============================================
// Browser State Steps (web-only)
// ============================================

// CookieSpec represents a cookie to set.
type CookieSpec struct {
	Name     string  `yaml:"name"`
	Value    string  `yaml:"value"`
	Domain   string  `yaml:"domain"`
	Path     string  `yaml:"path"`
	HTTPOnly bool    `yaml:"httpOnly"`
	Secure   bool    `yaml:"secure"`
	SameSite string  `yaml:"sameSite"`
	Expires  float64 `yaml:"expires"` // Unix timestamp
}

// SetCookiesStep sets browser cookies via CDP.
type SetCookiesStep struct {
	BaseStep `yaml:",inline"`
	Cookies  []CookieSpec `yaml:"cookies"`
}

// GetCookiesStep retrieves browser cookies and stores them as JSON.
type GetCookiesStep struct {
	BaseStep `yaml:",inline"`
	Output   string `yaml:"output"` // Variable name to store JSON result
}

// SaveAuthStateStep saves cookies + localStorage + sessionStorage to a JSON file.
type SaveAuthStateStep struct {
	BaseStep `yaml:",inline"`
	Path     string `yaml:"path"` // Output file path
}

// LoadAuthStateStep loads cookies + localStorage + sessionStorage from a JSON file.
type LoadAuthStateStep struct {
	BaseStep `yaml:",inline"`
	Path     string `yaml:"path"` // Input file path
}

// ============================================
// Browser File & Permissions Steps (web-only)
// ============================================

// UploadFileStep sets files on a file input element.
type UploadFileStep struct {
	BaseStep `yaml:",inline"`
	Selector Selector `yaml:",inline"`
	Path     string   `yaml:"path"`  // Single file path
	Paths    []string `yaml:"paths"` // Multiple file paths
}

// WaitForDownloadStep waits for a browser download to complete.
type WaitForDownloadStep struct {
	BaseStep       `yaml:",inline"`
	SaveTo         string `yaml:"saveTo"`         // Directory to save downloaded file
	AssertFilename string `yaml:"assertFilename"` // Expected filename (optional)
}

// GrantPermissionsStep grants browser permissions (notifications, camera, etc).
type GrantPermissionsStep struct {
	BaseStep    `yaml:",inline"`
	Permissions []string `yaml:"permissions"`
	Origin      string   `yaml:"origin"` // Optional: specific origin
}

// ResetPermissionsStep resets all browser permissions.
type ResetPermissionsStep struct {
	BaseStep `yaml:",inline"`
}

// ============================================
// Browser Tab Management Steps (web-only)
// ============================================

// OpenTabStep opens a new browser tab.
type OpenTabStep struct {
	BaseStep `yaml:",inline"`
	URL      string `yaml:"url"`
	TabLabel string `yaml:"tabLabel"` // Optional name for switching back
}

// SwitchTabStep switches to another browser tab.
type SwitchTabStep struct {
	BaseStep `yaml:",inline"`
	TabLabel string `yaml:"tabLabel"` // Switch by label
	Index    int    `yaml:"index"`    // Switch by index (0-based)
	URL      string `yaml:"url"`      // Switch by URL pattern match
}

// CloseTabStep closes the current tab and switches to the previous one.
type CloseTabStep struct {
	BaseStep `yaml:",inline"`
}

// ============================================
// Browser Network Interception Steps (web-only)
// ============================================

// MockResponseSpec describes the mock HTTP response.
type MockResponseSpec struct {
	Status  int               `yaml:"status"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
}

// MockNetworkStep mocks API responses matching URL pattern and method.
type MockNetworkStep struct {
	BaseStep `yaml:",inline"`
	URL      string           `yaml:"url"`
	Method   string           `yaml:"method"` // GET, POST, etc. (empty = match all)
	Response MockResponseSpec `yaml:"response"`
}

// BlockNetworkStep blocks network requests matching URL patterns.
type BlockNetworkStep struct {
	BaseStep `yaml:",inline"`
	Patterns []string `yaml:"patterns"`
}

// SetNetworkConditionsStep simulates network throttling or offline mode.
type SetNetworkConditionsStep struct {
	BaseStep      `yaml:",inline"`
	Offline       bool    `yaml:"offline"`
	Latency       float64 `yaml:"latency"`       // ms
	DownloadSpeed float64 `yaml:"downloadSpeed"` // KB/s (-1 = no throttle)
	UploadSpeed   float64 `yaml:"uploadSpeed"`   // KB/s (-1 = no throttle)
}

// WaitForRequestStep waits for a specific network request to be made.
type WaitForRequestStep struct {
	BaseStep `yaml:",inline"`
	URL      string `yaml:"url"`
	Method   string `yaml:"method"` // Optional: match specific HTTP method
	Output   string `yaml:"output"` // Variable name to store request body
}

// ClearNetworkMocksStep clears all network mocks and blocks.
type ClearNetworkMocksStep struct {
	BaseStep `yaml:",inline"`
}

// ============================================
// Media Steps
// ============================================

// TakeScreenshotStep takes a screenshot. When CropOn is set, the screenshot
// is cropped to the bounds of the matched element instead of capturing the
// whole screen. Mirrors Maestro's takeScreenshot.cropOn (see
// https://docs.maestro.dev/reference/commands-available/takescreenshot).
type TakeScreenshotStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Path     string    `yaml:"path" json:"path,omitempty"`
	CropOn   *Selector `yaml:"cropOn,omitempty" json:"cropOn,omitempty"`
}

// AssertScreenshotStep compares a screenshot with a reference image.
//
// ThresholdRaw captures the raw `thresholdPercentage:` value (a number, or a
// string like "${VAR}"), while ThresholdPercentage holds the resolved float.
// A literal number is resolved at parse time; a string is deferred to the
// expand pass so `${VAR}` interpolation works (Maestro #3444).
type AssertScreenshotStep struct {
	BaseStep            `yaml:",inline" json:",inline"`
	Path                string    `yaml:"path" json:"path,omitempty"`
	CropOn              *Selector `yaml:"cropOn,omitempty" json:"cropOn,omitempty"`
	ThresholdPercentage float64   `yaml:"-" json:"thresholdPercentage,omitempty"`
	ThresholdRaw        any       `yaml:"thresholdPercentage,omitempty" json:"-"`
}

// StartRecordingStep starts recording.
type StartRecordingStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Path     string `yaml:"path" json:"path,omitempty"`
}

// StopRecordingStep stops recording.
type StopRecordingStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Path     string `yaml:"path" json:"path,omitempty"`
}

// AddMediaStep adds media files.
type AddMediaStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Files    []string `yaml:"files" json:"files,omitempty"`
}

// RemoveMediaStep clears media added by addMedia (Android: MediaStore index).
type RemoveMediaStep struct {
	BaseStep `yaml:",inline"`
}

// ============================================
// Other Steps
// ============================================

// SleepStep pauses execution for a given duration in milliseconds.
type SleepStep struct {
	BaseStep   `yaml:",inline" json:",inline"`
	DurationMs int `yaml:"durationMs" json:"durationMs,omitempty"`
}

// Describe returns a human-readable description of the sleep step.
func (s *SleepStep) Describe() string {
	return fmt.Sprintf("sleep: %dms", s.DurationMs)
}

// PressKeyStep presses a key.
type PressKeyStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Key      string `yaml:"key" json:"key,omitempty"`
}

// WaitForAnimationToEndStep waits for animations.
//
// Algorithm (matches upstream Maestro): take two consecutive screenshots, if
// fewer than 0.5% of pixels differ, the screen is considered static. Otherwise
// poll until static or timeout. Timeout comes from the inlined BaseStep
// (`timeout:` YAML key); defaults to 15s when unset.
type WaitForAnimationToEndStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	// SleepMs is the pause inserted between the two consecutive screenshots used
	// to detect motion.  A longer sleep catches slow-moving animations; a shorter
	// sleep speeds up detection of fast-settling screens.  Defaults to 200 ms.
	SleepMs int `yaml:"sleepMs" json:"sleepMs,omitempty"`
	// Threshold is the maximum pixel-difference percentage (0.0–1.0) that is
	// still considered "static".  Lower values are stricter.  Defaults to 0.005
	// (0.5 %).
	Threshold float64 `yaml:"threshold" json:"threshold,omitempty"`
}

// DefineVariablesStep defines variables.
type DefineVariablesStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Env      map[string]string `yaml:"env" json:"env,omitempty"`
}

// UnsupportedStep represents an unsupported step.
type UnsupportedStep struct {
	BaseStep `yaml:",inline" json:",inline"`
	Reason   string `json:"reason,omitempty"`
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
	if s.Count != "" {
		return "assertVisible: " + s.Selector.DescribeQuoted() + " (count: " + s.Count + ")"
	}
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
