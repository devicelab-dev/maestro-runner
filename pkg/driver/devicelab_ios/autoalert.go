package devicelab_ios

import (
	"os"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
	"github.com/devicelab-dev/maestro-runner/pkg/simulator"
)

// System permission prompts are answered the way the WDA driver answers them
// (pkg/driver/wda/commands.go, autoClickAlertSelector): launchApp's
// permissions resolve to accept, dismiss or nothing (core.IOSAlertAction, the
// rule WDA uses), and only a SpringBoard alert with a button the rule names
// is tapped. The app's own alerts are never touched: the agent is asked about
// SpringBoard alone, so an app's "Invalid Credentials" alert, or an app's
// screen asking before the system prompt, stays for the flow, as in Maestro.
//
// The agent answers `alert` per request; nothing watches the screen in the
// background. The driver asks where a prompt blocks a flow: before the steps
// in the first seconds after launchApp (where prompts appear), and when an
// element lookup keeps missing (a prompt that comes later, after a tap).

const (
	springboardBundle = "com.apple.springboard"

	// autoAlertLaunchWindow is how long after launchApp (or after a prompt
	// was answered, for the next one) every step asks first.
	autoAlertLaunchWindow = 3 * time.Second
	// autoAlertWindowEvery spaces those reads.
	autoAlertWindowEvery = 500 * time.Millisecond
	// A lookup that has missed for autoAlertMissAfter asks, then at most
	// once per autoAlertMissEvery: an element that appears within a second
	// (most do) costs no read.
	autoAlertMissAfter = time.Second
	autoAlertMissEvery = time.Second
	// autoAlertMaxAnswers bounds the prompts answered in a row, so an alert
	// that comes back after every tap cannot hold a step forever.
	autoAlertMaxAnswers = 5
	// alertFadeOut is the wait after a tap when settling is off.
	alertFadeOut = 400 * time.Millisecond
)

// autoAlertAction is how launchApp's permissions answer a permission prompt:
// "accept", "dismiss" or "" (leave it). DEVICELAB_IOS_AUTO_ALERTS=0 or false
// turns automatic answers off.
func autoAlertAction(permissions map[string]string) string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DEVICELAB_IOS_AUTO_ALERTS"))) {
	case "0", "false":
		return ""
	}
	return core.IOSAlertAction(permissions)
}

// autoAlertButton picks the button to tap for an action, by label, as WDA's
// autoAcceptAlertSelector / autoDismissAlertSelector do (case-insensitive):
// accept taps a label starting with "Allow" ("Allow", "Allow While Using
// App", "Allow Once", "Allow Full Access", App Tracking Transparency's
// "Allow"); dismiss taps "Don't Allow" (either apostrophe) or "Ask App Not
// to Track". On a device in another language, the same labels as iOS words
// them there count (labels, see core.IOSPromptLabels). The first match in the
// alert's order wins. "" when none matches: an alert answered "OK" or
// "Continue" is left to the flow, and there is no fallback to another button.
func autoAlertButton(action string, buttons []string, labels core.IOSPromptLabels) string {
	for _, b := range buttons {
		if labels.Action(b) == action {
			return b
		}
	}
	return ""
}

// localPromptLabels is the simulator's prompt vocabulary in its preferred
// languages, read on first use. A real device, or a simulator whose strings
// cannot be read, keeps the English labels.
func (d *Driver) localPromptLabels() core.IOSPromptLabels {
	d.promptLabelsOnce.Do(func() {
		if d.loadPromptLabels != nil {
			d.promptLabels = d.loadPromptLabels()
			return
		}
		if d.realDevice || d.udid == "" {
			return
		}
		pairs, err := simulator.PromptStringPairs(d.udid)
		if err != nil {
			logger.Debug("[devicelab-ios] permission prompt strings unavailable, English labels only: %v", err)
			return
		}
		d.promptLabels = core.NewIOSPromptLabels(pairs)
	})
	return d.promptLabels
}

// answerSystemAlert asks the agent once for a SpringBoard alert and taps the
// button autoAlertAction calls for. It reports whether it tapped. No action,
// no alert, or no matching button costs the one read and taps nothing.
func (d *Driver) answerSystemAlert() bool {
	if d.autoAlert == "" {
		return false
	}
	d.lastAlertCheck = time.Now()
	resp, err := d.call("alert", &Args{Action: "read", App: springboardBundle})
	if err != nil || !resp.payload().Present {
		return false
	}
	p := resp.payload()
	if aimsAtAlert(d.step, p.Title, p.Buttons) {
		// The flow answers this prompt itself (`tapOn: "Allow"`, acceptAlert,
		// an assert on its title), as a Maestro flow does: leave it.
		logger.Debug("[devicelab-ios] system alert %q left to the step, which is aimed at it", p.Title)
		return false
	}
	button := autoAlertButton(d.autoAlert, p.Buttons, core.IOSPromptLabels{})
	if button == "" {
		// Not English: try the labels in the simulator's language.
		button = autoAlertButton(d.autoAlert, p.Buttons, d.localPromptLabels())
	}
	if button == "" {
		logger.Debug("[devicelab-ios] system alert %q left to the flow: no button to %s in %q", p.Title, d.autoAlert, p.Buttons)
		return false
	}
	if _, err := d.call("alert", &Args{Action: d.autoAlert, App: springboardBundle, Button: button}); err != nil {
		logger.Debug("[devicelab-ios] system alert %q: tapping %q failed: %v", p.Title, button, err)
		return false
	}
	logger.Info("[devicelab-ios] permission prompt %q answered %q (launchApp permissions)", p.Title, button)
	// Another prompt often follows (notifications, then location).
	d.alertWatchUntil = time.Now().Add(autoAlertLaunchWindow)
	return true
}

// answerAlertsBeforeStep answers prompts before a step that looks at the
// screen, during the window after launchApp. It reports whether it tapped.
func (d *Driver) answerAlertsBeforeStep(step flow.Step) bool {
	if d.autoAlert == "" || !time.Now().Before(d.alertWatchUntil) || !seesScreen(step) {
		return false
	}
	if time.Since(d.lastAlertCheck) < autoAlertWindowEvery {
		return false
	}
	answered := false
	for i := 0; i < autoAlertMaxAnswers && d.answerSystemAlert(); i++ {
		answered = true
		d.afterAlertTap()
	}
	return answered
}

// afterAlertTap lets the answered alert go before anything else reads or taps
// the screen: a tap sent while it fades out would land on the app beneath.
func (d *Driver) afterAlertTap() {
	if d.settleOff {
		time.Sleep(alertFadeOut)
		return
	}
	d.settle(defaultSettleTimeout)
}

// answerAlertOnMiss is called when a lookup missed: once the element has
// been missing since missingSince for autoAlertMissAfter, it asks for a
// prompt at most once per autoAlertMissEvery. It reports whether it tapped.
func (d *Driver) answerAlertOnMiss(missingSince time.Time) bool {
	if d.autoAlert == "" || time.Since(missingSince) < autoAlertMissAfter ||
		time.Since(d.lastAlertCheck) < autoAlertMissEvery {
		return false
	}
	return d.answerSystemAlert()
}

// seesScreen reports whether a step depends on what is on screen, so a
// prompt over the app would change its outcome. acceptAlert and dismissAlert
// answer the alert themselves.
func seesScreen(step flow.Step) bool {
	if actsOnScreen(step) {
		return true
	}
	switch step.(type) {
	case *flow.AssertVisibleStep, *flow.AssertNotVisibleStep, *flow.WaitUntilStep,
		*flow.TakeScreenshotStep, *flow.AssertScreenshotStep:
		return true
	}
	return false
}

// aimsAtAlert reports whether step deals with the alert itself: acceptAlert
// or dismissAlert, or a step whose selector (text or id) matches one of the
// alert's buttons or its title, such as `tapOn: "Allow"` or an assertVisible
// on the prompt's question, matched as the step's own lookup matches (so
// "Don't Allow" typed with a straight apostrophe is not iOS's "Don’t Allow",
// as in Maestro). Flows written for Maestro, which answers nothing,
// tap "Allow" themselves; answering first would leave them nothing to tap.
// The alert's message is not in the agent's read, so a selector on it alone
// does not count. Only asked when a prompt is present.
func aimsAtAlert(step flow.Step, title string, buttons []string) bool {
	switch step.(type) {
	case *flow.AcceptAlertStep, *flow.DismissAlertStep:
		return true
	}
	values := append([]string{title}, buttons...)
	for _, sel := range stepSelectors(step) {
		if sel.Text != "" && core.MatchSelectorText(sel.Text, values...) {
			return true
		}
		if sel.ID != "" {
			for _, v := range values {
				if core.MatchSelectorID(sel.ID, v) {
					return true
				}
			}
		}
	}
	return false
}

// stepSelectors are the selectors a step looks for on screen.
func stepSelectors(step flow.Step) []flow.Selector {
	var out []flow.Selector
	add := func(sel *flow.Selector) {
		if sel != nil {
			out = append(out, *sel)
		}
	}
	switch s := step.(type) {
	case *flow.TapOnStep:
		add(&s.Selector)
	case *flow.DoubleTapOnStep:
		add(&s.Selector)
	case *flow.LongPressOnStep:
		add(&s.Selector)
	case *flow.AssertVisibleStep:
		add(&s.Selector)
	case *flow.AssertNotVisibleStep:
		add(&s.Selector)
	case *flow.CopyTextFromStep:
		add(&s.Selector)
	case *flow.ScrollUntilVisibleStep:
		add(&s.Element)
	case *flow.WaitUntilStep:
		add(s.Visible)
		add(s.NotVisible)
	case *flow.SwipeStep:
		add(s.Selector)
	case *flow.DragAndDropStep:
		add(&s.From)
		add(&s.To)
	}
	return out
}
