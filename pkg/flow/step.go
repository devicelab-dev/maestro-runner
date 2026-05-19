// Package flow handles parsing and representation of Maestro YAML flow files.
package flow

import "gopkg.in/yaml.v3"

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
	StepTravel             StepType = "travel"
	StepOpenLink           StepType = "openLink"
	StepOpenBrowser        StepType = "openBrowser"

	// Flow Control
	StepRepeat            StepType = "repeat"
	StepRetry             StepType = "retry"
	StepRunFlow           StepType = "runFlow"
	StepRunScript         StepType = "runScript"
	StepEvalScript        StepType = "evalScript"
	StepEvalBrowserScript  StepType = "evalBrowserScript"
	StepRunBrowserScript   StepType = "runBrowserScript"
	StepEvalWebViewScript  StepType = "evalWebViewScript"
	StepRunWebViewScript   StepType = "runWebViewScript"
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
	StepTakeScreenshot StepType = "takeScreenshot"
	StepStartRecording StepType = "startRecording"
	StepStopRecording  StepType = "stopRecording"
	StepAddMedia       StepType = "addMedia"
	StepRemoveMedia    StepType = "removeMedia"

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
	DurationMs            int      `yaml:"duration"`
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
	DurationMs            int      `yaml:"duration"`
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
	DurationMs            int    `yaml:"duration"`
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
	// Engine selects the scroll backend on Android.
	// "" (default) and "adb" → adb input swipe (matches upstream Maestro).
	// "agent" → driver's existing on-device gesture path (UIA2 server
	// /appium/gestures/scroll for the uiautomator2 driver, RPC MotionEvent
	// injection for the devicelab driver). Ignored on iOS/web.
	Engine string `yaml:"engine"`
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
	// Engine selects the scroll backend. See ScrollStep.Engine.
	Engine string `yaml:"engine"`
}

// BackStep presses back.
type BackStep struct {
	BaseStep `yaml:",inline"`
}

// HideKeyboardStep hides the keyboard.
type HideKeyboardStep struct {
	BaseStep `yaml:",inline"`
}

// OpenNotificationsStep pulls down the Android notification shade.
// Android-only (no-op on iOS).
type OpenNotificationsStep struct {
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
	Script     string    `yaml:"true"`
	Platform   string    `yaml:"platform"`
	Timeout    int       `yaml:"timeout"` // Timeout in ms for visible/notVisible checks
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
	NewSession    bool              `yaml:"newSession"` // Appium only: create fresh session
	Permissions   map[string]string `yaml:"permissions"`
	Arguments     map[string]any    `yaml:"arguments"`   // Launch arguments (-key value pairs)
	Environment   map[string]string `yaml:"environment"` // Launch environment variables
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
//
// YAML accepts either a bool (`enabled: true`) or a string that may contain
// variable interpolation (`enabled: ${OFFLINE}`). EnabledRaw captures the raw
// YAML scalar; the executor's variable-expansion pass writes the resolved
// boolean into Enabled before the driver runs the step.
type SetAirplaneModeStep struct {
	BaseStep   `yaml:",inline"`
	Enabled    bool `yaml:"-"`
	EnabledRaw any  `yaml:"enabled"`
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
//
// When `when:` evaluates false, execution falls through to the else branch
// (ElseFile / ElseSteps). If no else branch is set, the step is skipped.
type RunFlowStep struct {
	BaseStep  `yaml:",inline"`
	File      string            `yaml:"file"`
	Steps     []Step            `yaml:"-"` // Inline steps (commands)
	ElseFile  string            `yaml:"-"` // Fallback flow file when `when` is false
	ElseSteps []Step            `yaml:"-"` // Inline fallback steps (else / elseCommands)
	When      *Condition        `yaml:"when"`
	Env       map[string]string `yaml:"env"`
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

// RemoveMediaStep clears media added by addMedia (Android: MediaStore index).
type RemoveMediaStep struct {
	BaseStep `yaml:",inline"`
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
//
// Algorithm (matches upstream Maestro): take two consecutive screenshots, if
// fewer than 0.5% of pixels differ, the screen is considered static. Otherwise
// poll until static or timeout. Timeout comes from the inlined BaseStep
// (`timeout:` YAML key); defaults to 15s when unset.
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
