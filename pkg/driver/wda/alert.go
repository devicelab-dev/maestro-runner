package wda

import (
	"fmt"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// WDA's /source is the app under test only, and a permission prompt belongs to
// SpringBoard, so selectors could not see one (#248). When a selector finds
// nothing in the app, these steps also match it against an open system alert,
// which WDA reads through /alert/text and /wda/alert/buttons.

// Classes of the element a selector resolves to on a system alert. Such an
// element has no bounds: WDA does not report them, and its button is tapped by
// name through /alert/accept.
const (
	systemAlertButtonClass = "SystemAlertButton"
	systemAlertTextClass   = "SystemAlertText"
)

// How often a tap on an alert's button is tried, and how long the alert gets to
// close after each.
const (
	alertAnswerTries = 4
	alertAnswerWait  = 400 * time.Millisecond
)

// alertLookup says what of a system alert the current step's selector may match.
type alertLookup int

const (
	alertLookupNone    alertLookup = iota // the step needs an element with bounds
	alertLookupButtons                    // a plain tap: only a button can be tapped
	alertLookupAll                        // a visibility check: the alert's text too
)

// alertLookupFor returns what a step may match on a system alert. A double
// tap, a long press or a tap at a point has nowhere to go without bounds.
func alertLookupFor(step flow.Step) alertLookup {
	switch s := step.(type) {
	case *flow.TapOnStep:
		if s.DurationMs == 0 && !s.LongPress && s.Point == "" {
			return alertLookupButtons
		}
	case *flow.AssertVisibleStep, *flow.AssertNotVisibleStep, *flow.WaitUntilStep:
		return alertLookupAll
	}
	return alertLookupNone
}

// matchesOnAlert reports whether a selector can be answered from an alert,
// which has only text: anything else it asks for can't be checked there.
func matchesOnAlert(sel flow.Selector) bool {
	return sel.Text != "" && sel.ID == "" && !sel.HasRelativeSelector() && !sel.HasNonZeroIndex() &&
		sel.Width == 0 && sel.Height == 0 && sel.Traits == "" &&
		sel.Enabled == nil && sel.Selected == nil && sel.Checked == nil && sel.Focused == nil
}

// findOnSystemAlert matches sel against the open system alert, if there is
// one. Any failure to read it counts as no alert: the usual answer when none
// is up, and the lookup then reports what it found in the app.
func (d *Driver) findOnSystemAlert(sel flow.Selector) *core.ElementInfo {
	if d.alertLookup == alertLookupNone || !matchesOnAlert(sel) {
		return nil
	}
	text, err := d.client.AlertText()
	if err != nil {
		return nil
	}
	if buttons, err := d.client.AlertButtons(); err == nil {
		if label, ok := pickAlertButton(sel.Text, buttons); ok {
			return &core.ElementInfo{Text: label, Class: systemAlertButtonClass, Visible: true, Enabled: true}
		}
	}
	if d.alertLookup == alertLookupAll && matchesText(straightQuotes(sel.Text), straightQuotes(text)) {
		return &core.ElementInfo{Text: text, Class: systemAlertTextClass, Visible: true, Enabled: true}
	}
	return nil
}

// pickAlertButton finds the button a selector names: the exact label first,
// then one that starts with it, then any match. Literal text matches by
// contains, so with no such order `tapOn: "Allow"` would pick "Don't Allow"
// when that comes first.
func pickAlertButton(pattern string, buttons []string) (string, bool) {
	want := strings.ToLower(straightQuotes(pattern))
	for _, b := range buttons {
		if strings.ToLower(straightQuotes(b)) == want {
			return b, true
		}
	}
	for _, b := range buttons {
		if strings.HasPrefix(strings.ToLower(straightQuotes(b)), want) {
			return b, true
		}
	}
	for _, b := range buttons {
		if matchesText(straightQuotes(pattern), straightQuotes(b)) {
			return b, true
		}
	}
	return "", false
}

// straightQuotes turns the curly apostrophe iOS writes ("Don’t Allow") into
// the straight one a flow is typed with.
func straightQuotes(s string) string { return strings.ReplaceAll(s, "’", "'") }

// answerAlert taps a button of the open alert until the alert has gone. WDA
// answers 200 for a tap that lands while a prompt is still animating in, and
// the prompt stays up, so success can only be read from the alert itself. The
// step fails if it never goes: it must not pass with the prompt still open.
func (d *Driver) answerAlert(tap func() error) error {
	opened, err := d.client.AlertText()
	if err != nil {
		return tap()
	}
	for try := 0; try < alertAnswerTries; try++ {
		if err := tap(); err != nil {
			// The alert went between the check and the tap, so an earlier tap
			// did land. With none before it, the failure is real.
			if try > 0 {
				return nil
			}
			return err
		}
		time.Sleep(alertAnswerWait)
		if now, err := d.client.AlertText(); err != nil || now != opened {
			return nil
		}
	}
	return fmt.Errorf("system alert still open after %d taps: %q", alertAnswerTries, opened)
}

// stepsAnswerAlerts reports whether a flow answers permission prompts itself:
// an acceptAlert or dismissAlert, or a tapOn naming a prompt's button, in its
// own steps or inline ones. A flow in another file (runFlow: file:) isn't seen.
func stepsAnswerAlerts(steps []flow.Step) bool {
	names, any := promptButtonsNamed(steps)
	return any || len(names) > 0
}

// promptButtonsNamed returns the prompt buttons a flow's taps name, in its own
// steps or inline ones, and whether it has an acceptAlert or dismissAlert,
// which answers whatever prompt is up. A flow in another file (runFlow:
// file:) isn't seen.
func promptButtonsNamed(steps []flow.Step) (names []string, any bool) {
	var walk func([]flow.Step)
	walk = func(steps []flow.Step) {
		for _, step := range steps {
			switch s := step.(type) {
			case *flow.AcceptAlertStep, *flow.DismissAlertStep:
				any = true
			case *flow.TapOnStep:
				if namesPromptButton(s.Selector.Text) {
					names = append(names, s.Selector.Text)
				}
			case *flow.RunFlowStep:
				walk(s.Steps)
				walk(s.ElseSteps)
			case *flow.RepeatStep:
				walk(s.Steps)
			case *flow.RetryStep:
				walk(s.Steps)
			}
		}
	}
	walk(steps)
	return names, any
}

// answerUnnamedPrompt answers the open permission prompt as WDA would have,
// when the flow answers some prompts itself and this one is not among them.
// WDA's alert monitor is all or nothing for a session, so a flow that taps one
// prompt's button (the tracking prompt's "Ask App Not to Track", say) turned it
// off for every prompt, and a later one the flow never meant to answer (the
// notification prompt) stayed up over the app. A lookup that finds nothing
// calls this: it leaves the prompt to the flow when one of its buttons is one
// the flow taps, or when the flow has an acceptAlert or dismissAlert; else it
// taps the button WDA's own answer would have (autoClickAlertSelector's) and
// reports whether it did, so the lookup looks again.
func (d *Driver) answerUnnamedPrompt() bool {
	if !d.flowAnswersAlerts || d.flowAnswersAnyPrompt || d.promptAction == "" {
		return false
	}
	text, err := d.client.AlertText()
	if err != nil {
		return false
	}
	buttons, err := d.client.AlertButtons()
	if err != nil {
		return false
	}
	for _, name := range d.flowPromptButtons {
		if _, ok := pickAlertButton(name, buttons); ok {
			return false
		}
	}
	label := ""
	for _, b := range buttons {
		t := strings.ToLower(straightQuotes(b))
		if d.promptAction == "accept" && strings.HasPrefix(t, "allow") ||
			d.promptAction == "dismiss" && (strings.HasPrefix(t, "don't allow") || strings.HasPrefix(t, "ask app not")) {
			label = b
			break
		}
	}
	if label == "" {
		return false
	}
	if err := d.answerAlert(func() error { return d.client.TapAlertButton(label) }); err != nil {
		logger.Warn("[wda] a permission prompt the flow does not answer itself stayed up after %q: %v", label, err)
		return false
	}
	logger.Info("[wda] answered a permission prompt the flow does not answer itself with %q: %q", label, firstLine(text))
	return true
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// namesPromptButton reports whether a selector's text is one of the buttons
// the automatic answer taps, so that the answer would beat the flow's tap.
func namesPromptButton(text string) bool {
	t := strings.ToLower(straightQuotes(text))
	return strings.HasPrefix(t, "allow") || strings.HasPrefix(t, "don't allow") || strings.HasPrefix(t, "ask app not")
}

// flowAlertAction is the alert action for launchApp's permissions: none when
// the flow answers prompts itself, since WDA would answer them first.
func (d *Driver) flowAlertAction(permissions map[string]string) string {
	d.promptAction = ""
	if d.flowAnswersAlerts {
		if !d.flowAnswersAnyPrompt {
			d.promptAction = resolveAlertAction(permissions)
		}
		logger.Info("[wda] the flow answers permission prompts itself, leaving them for its steps")
		return ""
	}
	return resolveAlertAction(permissions)
}
