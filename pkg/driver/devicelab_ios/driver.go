package devicelab_ios

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/simulator"
)

// Default budgets.
const (
	defaultFindTimeout     = 17 * time.Second // Maestro's lookupTimeoutMs
	defaultOptionalTimeout = 7 * time.Second
	defaultSettleTimeout   = 3 * time.Second
	pollInterval           = 150 * time.Millisecond
)

// agentAPI is the part of *Client the driver uses; tests substitute it.
type agentAPI interface {
	Call(ctx context.Context, cmd string, args *Args) (*Response, error)
}

// Driver implements core.Driver on the devicelab iOS agent.
type Driver struct {
	agent agentAPI
	info  *core.PlatformInfo
	udid  string
	appID string

	ctx             context.Context
	findTimeout     time.Duration
	optionalTimeout time.Duration
	typingSpeed     int

	// The last step could have set the screen moving, so the next action
	// settles first (asserts poll on their own and do not).
	screenMayMove bool

	// lastTap is where this step tapped; prevTap is where the step before
	// it did, so inputText can re-tap a field whose focus did not take.
	lastTap, prevTap *tapAt
	// foundLate is set when the last findElement needed more than one
	// lookup: the element appeared while we polled, so the screen was still
	// changing (a web page still loading).
	foundLate bool

	mu         sync.Mutex
	stagedApps map[string]stagedApp
	recording  *simulator.Recording

	// realDevice is set on a physical iPhone (SetRealDevice): what simctl
	// does on a simulator goes through the agent or devicectl, or is
	// reported as unavailable. appFile is --app-file, which clearState
	// reinstalls from.
	realDevice bool
	appFile    string

	// runSimctl runs `xcrun simctl args…`; tests replace it.
	runSimctl func(args ...string) (string, error)
	// runSimctlWithin runs `xcrun simctl args…` with its own time limit;
	// tests replace it.
	runSimctlWithin func(timeout time.Duration, args ...string) (string, error)

	// web reads whether the visible web page is still loading (WebKit's
	// inspector); openWeb connects it, and tests replace it.
	web     webState
	openWeb func() (webPages, error)
}

// NewDriver returns a driver for the agent on a simulator.
func NewDriver(agent agentAPI, info *core.PlatformInfo, udid string) *Driver {
	d := &Driver{
		agent:           agent,
		info:            info,
		udid:            udid,
		findTimeout:     defaultFindTimeout,
		optionalTimeout: defaultOptionalTimeout,
		runSimctl: func(args ...string) (string, error) {
			return simctl(context.Background(), 5*time.Minute, args...)
		},
		runSimctlWithin: func(timeout time.Duration, args ...string) (string, error) {
			return simctl(context.Background(), timeout, args...)
		},
	}
	// DL_IOS_WEB_READY=0 turns the web readiness check off (A/B timing).
	if os.Getenv("DL_IOS_WEB_READY") != "0" {
		d.openWeb = d.openSimulatorInspector
	}
	return d
}

// SetAppID names the app under test.
func (d *Driver) SetAppID(id string) { d.appID = id }

// SetContext implements core.Driver.
func (d *Driver) SetContext(ctx context.Context) { d.ctx = ctx }

func (d *Driver) context() context.Context {
	if d.ctx != nil {
		return d.ctx
	}
	return context.Background()
}

// SetFindTimeout implements core.Driver.
func (d *Driver) SetFindTimeout(ms int) {
	if ms > 0 {
		d.findTimeout = time.Duration(ms) * time.Millisecond
	}
}

// SetOptionalFindTimeout sets the budget for optional lookups.
func (d *Driver) SetOptionalFindTimeout(ms int) {
	if ms > 0 {
		d.optionalTimeout = time.Duration(ms) * time.Millisecond
	}
}

// SetWaitForIdleTimeout implements core.Driver. Settling is the agent's own
// (screen at rest), so there is nothing to configure.
func (d *Driver) SetWaitForIdleTimeout(int) error { return nil }

// SetTypingFrequency implements core.TypingFrequencyConfigurer.
func (d *Driver) SetTypingFrequency(freq int) error {
	d.typingSpeed = freq
	return nil
}

// GetPlatformInfo implements core.Driver.
func (d *Driver) GetPlatformInfo() *core.PlatformInfo { return d.info }

// call sends a command with the driver's context.
func (d *Driver) call(cmd string, args *Args) (*Response, error) {
	return d.agent.Call(d.context(), cmd, args)
}

// Screenshot implements core.Driver.
func (d *Driver) Screenshot() ([]byte, error) {
	resp, err := d.call("screenshot", &Args{Format: "png"})
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(resp.payload().Image)
}

// Hierarchy implements core.Driver: the frontmost app's tree in the flat
// {"nodes":[…]} JSON the report and `hierarchy` command read.
func (d *Driver) Hierarchy() ([]byte, error) {
	resp, err := d.call("snapshot", &Args{Alerts: true})
	if err != nil {
		return nil, err
	}
	type flat struct {
		Index       int            `json:"index"`
		ParentIndex *int           `json:"parentIndex,omitempty"`
		Type        string         `json:"type"`
		Identifier  string         `json:"identifier,omitempty"`
		Label       string         `json:"label,omitempty"`
		Value       string         `json:"value,omitempty"`
		Placeholder string         `json:"placeholderValue,omitempty"`
		Rect        map[string]int `json:"rect"`
		Enabled     bool           `json:"enabled"`
		Selected    bool           `json:"selected"`
		Focused     bool           `json:"focused"`
	}
	// Readers take index as the position in the list and rects as whole
	// points.
	nodes := resp.payload().Nodes
	pos := make(map[int]int, len(nodes))
	for i, n := range nodes {
		pos[n.I] = i
	}
	out := make([]flat, 0, len(nodes))
	for i, n := range nodes {
		b := bounds(n)
		f := flat{
			Index: i, Type: n.Type, Identifier: n.ID, Label: firstNonEmpty(n.Label, n.Title), Value: n.Value,
			Placeholder: n.Placeholder, Enabled: n.Enabled, Selected: n.Selected, Focused: n.Focused,
			Rect: map[string]int{"x": b.X, "y": b.Y, "width": b.Width, "height": b.Height},
		}
		if p, ok := pos[n.P]; ok && n.P >= 0 {
			f.ParentIndex = &p
		}
		out = append(out, f)
	}
	return json.Marshal(map[string]any{"nodes": out})
}

// GetState implements core.Driver.
func (d *Driver) GetState() *core.StateSnapshot {
	state := &core.StateSnapshot{}
	if resp, err := d.call("keyboard", &Args{Action: "state"}); err == nil {
		state.KeyboardVisible = resp.payload().Visible
	}
	if resp, err := d.call("device", &Args{Action: "orientation"}); err == nil {
		state.Orientation = resp.payload().Orientation
	}
	if d.appID != "" {
		if resp, err := d.call("app", &Args{Action: "state", BundleID: d.appID}); err == nil {
			state.AppState = resp.payload().AppState
		}
	}
	return state
}

// Close releases what the driver holds (staged app copies).
func (d *Driver) Close() {
	d.removeStagedApps()
	d.web.mu.Lock()
	ready := d.web.ready
	d.web.mu.Unlock()
	if ready != nil {
		select { // an open still in progress would leak its connection
		case <-ready:
		case <-time.After(2 * webCallTimeout):
		}
	}
	d.web.mu.Lock()
	if d.web.pages != nil {
		_ = d.web.pages.Close()
		d.web.pages = nil
	}
	d.web.mu.Unlock()
}

// StartScreenRecording implements core.ScreenRecorder (simulator, host-side).
func (d *Driver) StartScreenRecording() error {
	if d.realDevice {
		return errOnDevice("screen recording")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.recording != nil {
		return fmt.Errorf("a recording is already in progress")
	}
	rec, err := simulator.StartRecording(d.udid)
	if err != nil {
		return err
	}
	d.recording = rec
	return nil
}

// StopScreenRecording implements core.ScreenRecorder.
func (d *Driver) StopScreenRecording(hostPath string) error {
	d.mu.Lock()
	rec := d.recording
	d.recording = nil
	d.mu.Unlock()
	if rec == nil {
		return fmt.Errorf("no recording in progress")
	}
	return rec.Stop(hostPath)
}

// WaitsAroundTaps reports that a tap settles first when the step before it
// moved the screen (see flow.RepeatTapAfterFirst).
func (d *Driver) WaitsAroundTaps() bool { return true }

// Execute implements core.Driver.
func (d *Driver) Execute(step flow.Step) *core.CommandResult {
	start := time.Now()
	if d.screenMayMove && actsOnScreen(step) && !flow.RepeatTapAfterFirst(step) {
		d.settle(defaultSettleTimeout)
	}
	// Asserts and waits between an action and the next one neither settle
	// nor move the screen, so they leave both records alone: an assert that
	// passed early on a page still being pushed must not let the next tap
	// skip its settle (a back tap sent mid-push was lost), and a wait
	// between "tapOn field" and "inputText" keeps the tap point.
	if actsOnScreen(step) || movesScreen(step) {
		d.screenMayMove = movesScreen(step)
		// eraseText types into the tapped field and taps nothing itself, so
		// "tapOn field, eraseText, inputText" keeps the tap point for the
		// re-tap when the field never took focus (seen on a real iPhone).
		if _, erase := step.(*flow.EraseTextStep); !erase {
			d.prevTap, d.lastTap = d.lastTap, nil
		}
	}
	result := d.execute(step)
	if result == nil {
		result = core.ErrorResult(fmt.Errorf("no result"), "step produced no result")
	}
	result.Duration = time.Since(start)
	return result
}

func (d *Driver) execute(step flow.Step) *core.CommandResult {
	switch s := step.(type) {
	case *flow.TapOnStep:
		return d.tapOn(s)
	case *flow.DoubleTapOnStep:
		return d.tapSelector(s.Selector, s.Optional, s.TimeoutMs, "doubleTap", 0)
	case *flow.LongPressOnStep:
		return d.tapSelector(s.Selector, s.Optional, s.TimeoutMs, "longPress", s.DurationMs)
	case *flow.TapOnPointStep:
		return d.tapOnPoint(s)
	case *flow.SwipeStep:
		return d.swipe(s)
	case *flow.ScrollStep:
		return d.scroll(s.Direction, s.Speed)
	case *flow.ScrollUntilVisibleStep:
		return d.scrollUntilVisible(s)
	case *flow.DragAndDropStep:
		return d.dragAndDrop(s)
	case *flow.AssertVisibleStep:
		return d.assertVisible(s)
	case *flow.AssertNotVisibleStep:
		return d.assertNotVisible(s.Selector, s.TimeoutMs)
	case *flow.WaitUntilStep:
		return d.waitUntil(s)
	case *flow.InputTextStep:
		return d.inputText(s)
	case *flow.InputRandomStep:
		return d.inputText(&flow.InputTextStep{Text: randomInput(s.DataType, s.Length)})
	case *flow.EraseTextStep:
		return d.eraseText(s.Characters)
	case *flow.HideKeyboardStep:
		return d.hideKeyboard()
	case *flow.PressKeyStep:
		return d.pressKey(s.Key)
	case *flow.BackStep:
		return d.back()
	case *flow.CopyTextFromStep:
		return d.copyTextFrom(s)
	case *flow.PasteTextStep:
		return d.pasteText()
	case *flow.SetClipboardStep:
		return d.setClipboard(s.Text)
	case *flow.WaitForAnimationToEndStep:
		return d.waitForAnimation(s.TimeoutMs)
	case *flow.TakeScreenshotStep:
		return d.takeScreenshot(s.CropOn)
	case *flow.AssertScreenshotStep:
		return d.takeScreenshot(s.CropOn)
	case *flow.AcceptAlertStep:
		return d.alert("accept", s.TimeoutMs)
	case *flow.DismissAlertStep:
		return d.alert("dismiss", s.TimeoutMs)
	case *flow.LaunchAppStep:
		return d.launchApp(s)
	case *flow.StopAppStep:
		return d.stopApp(s.AppID)
	case *flow.KillAppStep:
		return d.stopApp(s.AppID)
	case *flow.ClearStateStep:
		return d.clearState(s.AppID)
	case *flow.ClearKeychainStep:
		return d.clearKeychain()
	case *flow.SetPermissionsStep:
		return d.setPermissions(s)
	case *flow.OpenLinkStep:
		return d.openLink(s.Link, s.AutoVerify)
	case *flow.OpenBrowserStep:
		return d.openLink(s.URL, nil)
	case *flow.SetLocationStep:
		return d.setLocation(s.Latitude, s.Longitude)
	case *flow.SetOrientationStep:
		return d.setOrientation(s.Orientation)
	case *flow.SetDarkModeStep:
		return d.setDarkMode(s.Enabled)
	case *flow.ToggleDarkModeStep:
		return d.toggleDarkMode()
	case *flow.AssertDarkModeStep:
		return d.assertDarkMode(true)
	case *flow.AssertLightModeStep:
		return d.assertDarkMode(false)
	case *flow.AddMediaStep:
		return d.addMedia(s.Files)
	case *flow.SetAirplaneModeStep, *flow.ToggleAirplaneModeStep:
		err := fmt.Errorf("airplane mode is not available on the iOS simulator")
		return core.ErrorResult(err, err.Error())
	default:
		err := fmt.Errorf("%s is not supported by --driver devicelab on iOS", step.Type())
		return core.ErrorResult(err, err.Error())
	}
}

// actsOnScreen reports whether a step acts on what is on screen and so must
// not run while the previous step's UI is still changing.
func actsOnScreen(step flow.Step) bool {
	switch step.(type) {
	case *flow.TapOnStep, *flow.DoubleTapOnStep, *flow.LongPressOnStep, *flow.TapOnPointStep,
		*flow.DragAndDropStep, *flow.SwipeStep, *flow.ScrollStep, *flow.ScrollUntilVisibleStep,
		*flow.BackStep, *flow.PressKeyStep, *flow.InputTextStep, *flow.InputRandomStep,
		*flow.EraseTextStep, *flow.CopyTextFromStep, *flow.HideKeyboardStep:
		return true
	}
	return false
}

// movesScreen reports whether a step can set the screen moving.
func movesScreen(step flow.Step) bool {
	switch step.(type) {
	case *flow.TapOnStep, *flow.DoubleTapOnStep, *flow.LongPressOnStep, *flow.TapOnPointStep,
		*flow.DragAndDropStep, *flow.SwipeStep, *flow.ScrollStep, *flow.ScrollUntilVisibleStep,
		*flow.BackStep, *flow.PressKeyStep, *flow.InputTextStep, *flow.InputRandomStep,
		*flow.EraseTextStep, *flow.HideKeyboardStep, *flow.LaunchAppStep, *flow.OpenLinkStep,
		*flow.OpenBrowserStep, *flow.AcceptAlertStep, *flow.DismissAlertStep, *flow.SetOrientationStep:
		return true
	}
	return false
}

// settle waits for the visible web page, if any, to finish loading, then
// (on the device) for the screen to stop changing. A failure is not an
// error: the next step polls anyway.
func (d *Driver) settle(timeout time.Duration) {
	d.waitForWebLoad()
	resp, err := d.call("settle", &Args{TimeoutMs: float64(timeout.Milliseconds()), Quiescence: quiescenceMode()})
	if err == nil {
		// Settles decided on the tree and frames are the rule; log the rest
		// (frames only on a slow tree, a page still loading, a timeout).
		if p := resp.payload(); p.Signal != "tree+frame" || (p.Settled != nil && !*p.Settled) {
			logger.Debug("[devicelab-ios] settle signal=%s settled=%v waited=%.0fms", p.Signal, p.Settled != nil && *p.Settled, p.WaitedMs)
		}
	}
}

// quiescenceMode is XCTest's app-idle wait that settle runs first. An app
// can finish a change on its main thread after the screen looks still: DDG
// saves a password, then updates its navigation about a second later and
// undid a back tap sent in between. XCTest's wait (as WDA uses it) closed
// that race, 5 of 5, for about 0.2s per settle; the main-run-loop-only mode
// cost the same. DL_IOS_QUIESCENCE=off turns it off, =main keeps animations
// out.
func quiescenceMode() string {
	switch mode := os.Getenv("DL_IOS_QUIESCENCE"); mode {
	case "off":
		return ""
	case "main":
		return mode
	default:
		return "all"
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func randomInput(dataType string, length int) string {
	switch strings.ToUpper(dataType) {
	case "NUMBER":
		if length <= 0 {
			length = 8
		}
		return core.RandomNumber(length)
	case "EMAIL":
		return core.RandomEmail()
	case "PERSON_NAME":
		return core.RandomPersonName()
	default:
		if length <= 0 {
			length = 8
		}
		return core.RandomString(length)
	}
}
