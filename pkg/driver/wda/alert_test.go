package wda

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// alertServer is a WDA with an app showing nothing that matches and, when
// open, a system alert. It records the button names sent to /alert/accept.
func alertServer(t *testing.T, open bool, buttons ...string) (*Driver, *[]string) {
	t.Helper()
	return alertServerLosing(t, open, 0, buttons...)
}

// alertServerLosing also loses the first lost taps on the alert: they answer
// 200 and it stays up, as on a prompt still animating in. Below zero, every tap.
func alertServerLosing(t *testing.T, open bool, lost int, buttons ...string) (*Driver, *[]string) {
	t.Helper()
	var accepted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		noAlert := map[string]interface{}{"value": map[string]interface{}{"error": "no such alert", "message": "no alert"}}
		switch {
		case strings.HasSuffix(r.URL.Path, "/alert/text"):
			if !open {
				w.WriteHeader(http.StatusBadRequest)
				jsonResponse(w, noAlert)
				return
			}
			jsonResponse(w, map[string]interface{}{"value": "“Lab” Would Like to Access the Microphone\nFor recording"})
		case strings.HasSuffix(r.URL.Path, "/wda/alert/buttons"):
			if !open {
				w.WriteHeader(http.StatusBadRequest)
				jsonResponse(w, noAlert)
				return
			}
			jsonResponse(w, map[string]interface{}{"value": buttons})
		case strings.HasSuffix(r.URL.Path, "/alert/accept"), strings.HasSuffix(r.URL.Path, "/alert/dismiss"):
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(body, &req)
			accepted = append(accepted, req.Name)
			if lost == 0 {
				open = false
			} else if lost > 0 {
				lost--
			}
			jsonResponse(w, map[string]interface{}{"value": nil})
		case strings.HasSuffix(r.URL.Path, "/source"):
			jsonResponse(w, map[string]interface{}{"value": `<?xml version="1.0"?><XCUIElementTypeApplication type="XCUIElementTypeApplication" name="Lab" label="Lab" enabled="true" visible="true" x="0" y="0" width="390" height="844"/>`})
		default:
			w.WriteHeader(http.StatusNotFound)
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"error": "no such element", "message": "not found"}})
		}
	}))
	t.Cleanup(server.Close)
	return createTestDriver(server), &accepted
}

func textSelector(text string) flow.Selector { return flow.Selector{Text: text} }

func TestTapOnTapsSystemAlertButtonByName(t *testing.T) {
	d, accepted := alertServer(t, true, "Don't Allow", "Allow")
	step := &flow.TapOnStep{Selector: textSelector("Allow")}
	step.TimeoutMs = 300
	if res := d.Execute(step); !res.Success {
		t.Fatalf("tapOn: %s", res.Message)
	}
	if len(*accepted) != 1 || (*accepted)[0] != "Allow" {
		t.Errorf("accepted = %v, want [Allow]", *accepted)
	}
}

// The alert's text can be seen but not tapped: a tap on it would report
// success and answer nothing.
func TestTapOnDoesNotMatchSystemAlertText(t *testing.T) {
	d, accepted := alertServer(t, true, "Don't Allow", "Allow")
	step := &flow.TapOnStep{Selector: textSelector("access the Microphone")}
	step.TimeoutMs = 300
	if res := d.Execute(step); res.Success {
		t.Fatalf("tapOn succeeded: %s", res.Message)
	}
	if len(*accepted) != 0 {
		t.Errorf("accepted = %v, want none", *accepted)
	}
}

func TestVisibilityChecksSeeSystemAlert(t *testing.T) {
	d, _ := alertServer(t, true, "Don't Allow", "Allow")
	for _, text := range []string{"access the Microphone", "Don't Allow"} {
		step := &flow.AssertVisibleStep{Selector: textSelector(text)}
		step.TimeoutMs = 300
		if res := d.Execute(step); !res.Success {
			t.Errorf("assertVisible %q: %s", text, res.Message)
		}
	}
	notVisible := &flow.AssertNotVisibleStep{Selector: textSelector("access the Microphone")}
	notVisible.TimeoutMs = 100
	if res := d.Execute(notVisible); res.Success {
		t.Error("assertNotVisible passed with the alert up")
	}
}

func TestNoAlertOpenFindsNothing(t *testing.T) {
	d, accepted := alertServer(t, false)
	step := &flow.AssertVisibleStep{Selector: textSelector("Allow")}
	step.TimeoutMs = 200
	if res := d.Execute(step); res.Success {
		t.Errorf("assertVisible passed with no alert: %s", res.Message)
	}
	notVisible := &flow.AssertNotVisibleStep{Selector: textSelector("Allow")}
	if res := d.Execute(notVisible); !res.Success {
		t.Errorf("assertNotVisible failed with no alert: %s", res.Message)
	}
	if len(*accepted) != 0 {
		t.Errorf("accepted = %v, want none", *accepted)
	}
}

// A gesture needing a point has none on an alert, and a selector asking for
// more than text can't be checked there.
func TestAlertOnlyAnswersWhatItCan(t *testing.T) {
	if got := alertLookupFor(&flow.DoubleTapOnStep{}); got != alertLookupNone {
		t.Errorf("doubleTapOn = %v", got)
	}
	if got := alertLookupFor(&flow.TapOnStep{LongPress: true}); got != alertLookupNone {
		t.Errorf("long tapOn = %v", got)
	}
	if got := alertLookupFor(&flow.TapOnStep{Point: "50%,50%"}); got != alertLookupNone {
		t.Errorf("tapOn with point = %v", got)
	}
	enabled := true
	for name, sel := range map[string]flow.Selector{
		"id":      {Text: "Allow", ID: "x"},
		"enabled": {Text: "Allow", Enabled: &enabled},
		"index":   {Text: "Allow", Index: "1"},
		"below":   {Text: "Allow", Below: &flow.Selector{Text: "x"}},
		"no text": {ID: "x"},
	} {
		if matchesOnAlert(sel) {
			t.Errorf("%s: matchesOnAlert = true", name)
		}
	}
}

func TestPickAlertButton(t *testing.T) {
	location := []string{"Allow Once", "Allow While Using App", "Don't Allow"}
	for _, tc := range []struct {
		pattern string
		buttons []string
		want    string
	}{
		{"Allow", []string{"Don't Allow", "Allow"}, "Allow"},
		{"allow", location, "Allow Once"},
		{"Allow While", location, "Allow While Using App"},
		{"Don't Allow", location, "Don't Allow"},
		{"Don't Allow", []string{"Don’t Allow", "Allow"}, "Don’t Allow"},
		{"Don’t Allow", []string{"Don't Allow", "Allow"}, "Don't Allow"},
		{"While Using", location, "Allow While Using App"},
		{"Allow.*App", location, "Allow While Using App"},
		{"Settings", location, ""},
	} {
		got, ok := pickAlertButton(tc.pattern, tc.buttons)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("%q: got %q, %v, want %q", tc.pattern, got, ok, tc.want)
		}
	}
}

func TestStepsAnswerAlerts(t *testing.T) {
	tapAllow := &flow.TapOnStep{Selector: textSelector("Allow")}
	tapOther := &flow.TapOnStep{Selector: textSelector("Continue")}
	for name, tc := range map[string]struct {
		steps []flow.Step
		want  bool
	}{
		"none":        {[]flow.Step{tapOther}, false},
		"tap":         {[]flow.Step{tapAllow}, true},
		"deny":        {[]flow.Step{&flow.TapOnStep{Selector: textSelector("Don’t Allow")}}, true},
		"acceptAlert": {[]flow.Step{&flow.AcceptAlertStep{}}, true},
		"runFlow":     {[]flow.Step{&flow.RunFlowStep{Steps: []flow.Step{tapAllow}}}, true},
		"else":        {[]flow.Step{&flow.RunFlowStep{ElseSteps: []flow.Step{&flow.DismissAlertStep{}}}}, true},
		"repeat":      {[]flow.Step{&flow.RepeatStep{Steps: []flow.Step{tapAllow}}}, true},
		"retry":       {[]flow.Step{&flow.RetryStep{Steps: []flow.Step{tapAllow}}}, true},
	} {
		if got := stepsAnswerAlerts(tc.steps); got != tc.want {
			t.Errorf("%s = %v, want %v", name, got, tc.want)
		}
	}
}

// With a flow answering prompts itself, WDA must not answer them first,
// whatever launchApp's permissions resolve to.
func TestPrepareForFlowLeavesPromptsToAFlowThatAnswersThem(t *testing.T) {
	d, _ := alertServer(t, false)
	d.PrepareForFlow([]flow.Step{&flow.LaunchAppStep{}, &flow.TapOnStep{Selector: textSelector("Allow")}})
	if d.alertAction != "" {
		t.Errorf("alertAction = %q, want none", d.alertAction)
	}
	d.PrepareForFlow([]flow.Step{&flow.LaunchAppStep{}, &flow.TapOnStep{Selector: textSelector("Continue")}})
	if d.alertAction != "accept" {
		t.Errorf("alertAction = %q, want accept", d.alertAction)
	}
}

// A tap lost while a prompt animates in is sent again.
func TestAlertTapIsRepeatedUntilTheAlertCloses(t *testing.T) {
	d, accepted := alertServerLosing(t, true, 1, "Don't Allow", "Allow")
	step := &flow.TapOnStep{Selector: textSelector("Allow")}
	step.TimeoutMs = 300
	if res := d.Execute(step); !res.Success {
		t.Fatalf("tapOn: %s", res.Message)
	}
	if len(*accepted) != 2 {
		t.Errorf("accepted = %v, want 2 taps", *accepted)
	}

	d, accepted = alertServerLosing(t, true, 1, "Allow")
	if res := d.Execute(&flow.AcceptAlertStep{}); !res.Success {
		t.Fatalf("acceptAlert: %s", res.Message)
	}
	if len(*accepted) != 2 {
		t.Errorf("accepted = %v, want 2 taps", *accepted)
	}
}

// A step must not pass with the prompt still up.
func TestAlertThatNeverClosesFailsTheStep(t *testing.T) {
	for _, step := range []flow.Step{
		&flow.AcceptAlertStep{},
		&flow.DismissAlertStep{},
		&flow.TapOnStep{Selector: textSelector("Allow")},
	} {
		d, accepted := alertServerLosing(t, true, -1, "Allow")
		if res := d.Execute(step); res.Success {
			t.Errorf("%T passed: %s", step, res.Message)
		}
		if len(*accepted) != alertAnswerTries {
			t.Errorf("%T: %d taps, want %d", step, len(*accepted), alertAnswerTries)
		}
	}
}

func TestPromptButtonsNamed(t *testing.T) {
	optional := &flow.TapOnStep{Selector: textSelector("Ask app not to track")}
	optional.Optional = true
	names, any := promptButtonsNamed([]flow.Step{
		&flow.TapOnStep{Selector: textSelector("Continue")},
		&flow.RunFlowStep{When: &flow.Condition{}, Steps: []flow.Step{optional}},
		&flow.RetryStep{Steps: []flow.Step{&flow.TapOnStep{Selector: textSelector("Don’t Allow")}}},
	})
	if any || len(names) != 2 || names[0] != "Ask app not to track" || names[1] != "Don’t Allow" {
		t.Errorf("names = %q, any = %v", names, any)
	}
	if _, any := promptButtonsNamed([]flow.Step{&flow.RepeatStep{Steps: []flow.Step{&flow.DismissAlertStep{}}}}); !any {
		t.Error("a dismissAlert answers any prompt")
	}
}

// lookFor runs an assertVisible of an id the fake app does not have: a lookup
// that finds nothing.
func lookFor(d *Driver) {
	step := &flow.AssertVisibleStep{Selector: flow.Selector{ID: "home"}}
	step.TimeoutMs = 300
	d.Execute(step)
}

// A flow that answers the tracking prompt itself still gets the notification
// prompt answered: WDA's monitor is off for the whole flow, so a lookup that
// finds nothing answers a prompt whose buttons the flow never names.
func TestLookupAnswersAPromptTheFlowDoesNotName(t *testing.T) {
	d, accepted := alertServer(t, true, "Don’t Allow", "Allow")
	tapTracking := &flow.TapOnStep{Selector: textSelector("Ask app not to track")}
	tapTracking.Optional = true
	d.PrepareForFlow([]flow.Step{&flow.LaunchAppStep{}, tapTracking})
	if d.alertAction != "" || d.promptAction != "accept" {
		t.Fatalf("alertAction = %q, promptAction = %q", d.alertAction, d.promptAction)
	}
	lookFor(d)
	if len(*accepted) != 1 || (*accepted)[0] != "Allow" {
		t.Errorf("accepted = %v, want [Allow]", *accepted)
	}
}

// A prompt is left to the step aimed at it (a tap on one of its buttons, a
// check of its text), as DeviceLab's driver leaves it; every prompt is left to
// a flow with an acceptAlert or dismissAlert.
func TestLookupLeavesThePromptToTheStepAimedAtIt(t *testing.T) {
	d, accepted := alertServer(t, true, "Ask App Not to Track", "Allow")
	d.PrepareForFlow([]flow.Step{&flow.LaunchAppStep{}, &flow.TapOnStep{Selector: textSelector("Ask app not to track")}})
	check := &flow.AssertVisibleStep{Selector: textSelector("Ask App Not to Track")}
	check.TimeoutMs = 300
	if res := d.Execute(check); !res.Success {
		t.Errorf("the check of the prompt's button failed: %s", res.Message)
	}
	if len(*accepted) != 0 {
		t.Errorf("the prompt the step checks was answered: %v", *accepted)
	}

	d, accepted = alertServer(t, true, "Don’t Allow", "Allow")
	d.PrepareForFlow([]flow.Step{&flow.LaunchAppStep{}, &flow.AcceptAlertStep{}})
	lookFor(d)
	if len(*accepted) != 0 || d.promptAction != "" {
		t.Errorf("a flow with acceptAlert: accepted %v, promptAction %q", *accepted, d.promptAction)
	}
}

// A prompt whose button the flow taps elsewhere is still answered for a step
// aimed at something else, as DeviceLab's driver answers it: the tracking
// prompt over the onboarding held a 10 s wait for the screen beneath it.
func TestLookupAnswersAPromptTheFlowAnswersElsewhere(t *testing.T) {
	d, accepted := alertServer(t, true, "Ask App Not to Track", "Allow")
	d.PrepareForFlow([]flow.Step{&flow.LaunchAppStep{}, &flow.TapOnStep{Selector: textSelector("Ask app not to track")}})
	lookFor(d)
	if len(*accepted) != 1 || (*accepted)[0] != "Allow" {
		t.Errorf("accepted = %v, want [Allow]", *accepted)
	}
}

// launchApp's permissions decide the answer, as they decide WDA's.
func TestLookupAnswersAnUnnamedPromptAsThePermissionsSay(t *testing.T) {
	d, accepted := alertServer(t, true, "Don’t Allow", "Allow")
	d.PrepareForFlow([]flow.Step{
		&flow.LaunchAppStep{Permissions: map[string]string{"all": "deny"}},
		&flow.TapOnStep{Selector: textSelector("Ask App Not to Track")},
	})
	lookFor(d)
	if len(*accepted) != 1 || (*accepted)[0] != "Don’t Allow" {
		t.Errorf("accepted = %v, want [Don’t Allow]", *accepted)
	}
}

// Without a prompt tap of its own a flow keeps WDA's monitor, and a lookup
// answers nothing itself.
func TestLookupAnswersNothingWhenWDAAnswers(t *testing.T) {
	d, accepted := alertServer(t, true, "Don’t Allow", "Allow")
	d.PrepareForFlow([]flow.Step{&flow.LaunchAppStep{}})
	if d.alertAction != "accept" || d.promptAction != "" {
		t.Fatalf("alertAction = %q, promptAction = %q", d.alertAction, d.promptAction)
	}
	lookFor(d)
	if len(*accepted) != 0 {
		t.Errorf("accepted = %v, want none", *accepted)
	}
}

// promptOverAppServer is a WDA whose app has the element "next" under an open
// notification prompt. It records each WDA call in order, so a test can tell
// whether the prompt was answered before the tap.
func promptOverAppServer(t *testing.T) (*Driver, *[]string) {
	t.Helper()
	open := true
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		noAlert := map[string]interface{}{"value": map[string]interface{}{"error": "no such alert", "message": "no alert"}}
		switch {
		case strings.HasSuffix(p, "/alert/text"):
			calls = append(calls, "alert/text")
			if !open {
				w.WriteHeader(http.StatusNotFound)
				jsonResponse(w, noAlert)
				return
			}
			jsonResponse(w, map[string]interface{}{"value": "“Lab” Would Like to Send You Notifications"})
		case strings.HasSuffix(p, "/wda/alert/buttons"):
			jsonResponse(w, map[string]interface{}{"value": []string{"Don’t Allow", "Allow"}})
		case strings.HasSuffix(p, "/alert/accept"):
			calls = append(calls, "accept")
			open = false
			jsonResponse(w, map[string]interface{}{"value": nil})
		case strings.HasSuffix(p, "/wda/tap"):
			if open {
				calls = append(calls, "tap under the prompt")
			} else {
				calls = append(calls, "tap")
			}
			jsonResponse(w, map[string]interface{}{"value": nil})
		case strings.HasSuffix(p, "/screenshot"):
			jsonResponse(w, map[string]interface{}{"value": "c2FtZQ=="})
		case strings.HasSuffix(p, "/elements"):
			jsonResponse(w, map[string]interface{}{"value": []map[string]interface{}{{
				"ELEMENT": "next", "name": "next", "text": "Next", "displayed": true,
				"rect": map[string]interface{}{"x": 100, "y": 700, "width": 150, "height": 40},
			}}})
		// The element's fields, for a driver that reads them after the find.
		case strings.HasSuffix(p, "/element/next/rect"):
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"x": 100, "y": 700, "width": 150, "height": 40}})
		case strings.HasSuffix(p, "/element/next/text"):
			jsonResponse(w, map[string]interface{}{"value": "Next"})
		case strings.HasSuffix(p, "/element/next/displayed"):
			jsonResponse(w, map[string]interface{}{"value": true})
		case strings.HasSuffix(p, "/element/next/name"):
			jsonResponse(w, map[string]interface{}{"value": "next"})
		default:
			jsonResponse(w, map[string]interface{}{"value": nil})
		}
	}))
	t.Cleanup(server.Close)
	return createTestDriver(server), &calls
}

func tapCalls(calls []string) []string {
	var out []string
	for _, c := range calls {
		if c != "alert/text" {
			out = append(out, c)
		}
	}
	return out
}

// A flow that answers the tracking prompt itself gets the notification prompt
// answered before its next tap, not after a lookup under the prompt misses:
// the tap went to the prompt and was lost (G15's late answer).
func TestTapAnswersAnUnnamedPromptFirst(t *testing.T) {
	d, calls := promptOverAppServer(t)
	tracking := &flow.TapOnStep{Selector: textSelector("Ask App Not to Track")}
	tracking.Optional = true
	d.PrepareForFlow([]flow.Step{&flow.LaunchAppStep{}, tracking})
	d.watchForPrompts(promptLaunchWindow) // as launchApp does
	res := d.Execute(&flow.TapOnStep{Selector: flow.Selector{ID: "next"}})
	if !res.Success {
		t.Fatalf("tap failed: %s", res.Message)
	}
	if got := strings.Join(tapCalls(*calls), ", "); got != "accept, tap" {
		t.Errorf("calls = %s, want the prompt answered, then the tap", got)
	}
}

// A flow that leaves prompts to WDA's monitor asks for no alert before a tap.
func TestTapAsksForNoAlertWhenWDAAnswers(t *testing.T) {
	d, calls := promptOverAppServer(t)
	d.PrepareForFlow([]flow.Step{&flow.LaunchAppStep{}})
	d.Execute(&flow.TapOnStep{Selector: flow.Selector{ID: "next"}})
	for _, c := range *calls {
		if c == "alert/text" || c == "accept" {
			t.Fatalf("calls = %v: a flow WDA answers prompts for asked for the alert", *calls)
		}
	}
}

// Outside the watch after a launch or an answered prompt, a step asks for no
// alert before it acts; inside it, at most once per promptCheckEvery.
func TestTapAsksForPromptsOnlyInTheWatch(t *testing.T) {
	d, calls := promptOverAppServer(t)
	tracking := &flow.TapOnStep{Selector: textSelector("Ask App Not to Track")}
	tracking.Optional = true
	d.PrepareForFlow([]flow.Step{&flow.LaunchAppStep{}, tracking})
	next := &flow.TapOnStep{Selector: flow.Selector{ID: "next"}}
	d.Execute(next)
	if got := strings.Join(*calls, ", "); got != "tap under the prompt" {
		t.Errorf("before any launch: calls = %s, want only the tap", got)
	}
	d.watchForPrompts(promptLaunchWindow)
	*calls = nil
	d.Execute(next)
	if got := strings.Join(tapCalls(*calls), ", "); got != "accept, tap" {
		t.Errorf("in the watch: calls = %s, want the prompt answered, then the tap", got)
	}
	*calls = nil
	d.Execute(next) // within promptCheckEvery of the last ask
	for _, c := range *calls {
		if c == "alert/text" {
			t.Errorf("asked again within %v: %v", promptCheckEvery, *calls)
		}
	}
}
