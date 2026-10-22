package devicelab_ios

import (
	"sync"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestAutoAlertButton(t *testing.T) {
	for _, tc := range []struct {
		action  string
		buttons []string
		want    string
	}{
		{"accept", []string{"Don’t Allow", "Allow"}, "Allow"},
		{"accept", []string{"Allow Once", "Allow While Using App", "Don't Allow"}, "Allow Once"},
		{"accept", []string{"Limit Access…", "Allow Full Access", "Don’t Allow"}, "Allow Full Access"},
		{"accept", []string{"Ask App Not to Track", "Allow"}, "Allow"},
		{"accept", []string{"ALLOW"}, "ALLOW"},
		{"dismiss", []string{"Don’t Allow", "Allow"}, "Don’t Allow"},
		{"dismiss", []string{"Allow", "Don't Allow"}, "Don't Allow"},
		{"dismiss", []string{"Ask App Not to Track", "Allow"}, "Ask App Not to Track"},
		// Not a prompt the rule answers: no fallback to another button.
		{"accept", []string{"OK"}, ""},
		{"accept", []string{"Don’t Allow", "Continue"}, ""},
		{"dismiss", []string{"OK", "Cancel"}, ""},
		{"dismiss", []string{"Done"}, ""},
		{"accept", nil, ""},
		{"", []string{"Allow"}, ""},
	} {
		if got := autoAlertButton(tc.action, tc.buttons); got != tc.want {
			t.Errorf("autoAlertButton(%q, %q) = %q, want %q", tc.action, tc.buttons, got, tc.want)
		}
	}
}

func TestAutoAlertAction(t *testing.T) {
	for _, tc := range []struct {
		perms map[string]string
		want  string
	}{
		{nil, "accept"},
		{map[string]string{"all": "allow"}, "accept"},
		{map[string]string{"all": "deny"}, "dismiss"},
		{map[string]string{"camera": "allow", "location": "deny"}, ""},
		{map[string]string{"all": "allow", "camera": "deny"}, ""},
		{map[string]string{"all": "unset"}, ""},
	} {
		if got := autoAlertAction(tc.perms); got != tc.want {
			t.Errorf("autoAlertAction(%v) = %q, want %q", tc.perms, got, tc.want)
		}
	}
	for _, off := range []string{"0", "false", "FALSE"} {
		t.Setenv("DEVICELAB_IOS_AUTO_ALERTS", off)
		if got := autoAlertAction(nil); got != "" {
			t.Errorf("DEVICELAB_IOS_AUTO_ALERTS=%s: action = %q, want off", off, got)
		}
	}
	t.Setenv("DEVICELAB_IOS_AUTO_ALERTS", "1")
	if got := autoAlertAction(nil); got != "accept" {
		t.Errorf("DEVICELAB_IOS_AUTO_ALERTS=1: action = %q", got)
	}
}

// springboardAlert is a fake agent whose SpringBoard shows one alert with
// buttons until a button is tapped; the app shows "Home" throughout.
type springboardAlert struct {
	mu      sync.Mutex
	buttons []string // nil: no SpringBoard alert
	tapped  []Args
}

func (s *springboardAlert) handle(cmd string, a Args) (*Response, error) {
	switch cmd {
	case "find", "snapshot":
		return tree(node(1, "Button", "Home", 0, 0, 100, 40)), nil
	case "alert":
		s.mu.Lock()
		defer s.mu.Unlock()
		if a.App != springboardBundle || s.buttons == nil {
			return ok(&Payload{}), nil
		}
		if a.Action == "read" {
			return ok(&Payload{Present: true, Title: "“TestHive” Would Like to Send You Notifications", Buttons: s.buttons}), nil
		}
		s.tapped = append(s.tapped, a)
		s.buttons = nil
		return ok(&Payload{Present: true, Message: "tapped " + a.Button}), nil
	}
	return ok(&Payload{}), nil
}

func (s *springboardAlert) taps() []Args {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Args(nil), s.tapped...)
}

// launchThenTap runs launchApp with perms and then a tap, on a SpringBoard
// alert with buttons.
func launchThenTap(t *testing.T, perms map[string]string, buttons []string) (*springboardAlert, *fakeAgent) {
	t.Helper()
	sb := &springboardAlert{buttons: buttons}
	d, fa, _ := newTestDriver(t, sb.handle)
	fakeLaunch(t, nil)
	if res := d.Execute(&flow.LaunchAppStep{AppID: "com.example", Permissions: perms}); !res.Success {
		t.Fatal(res.Message)
	}
	if res := d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "Home"}}); !res.Success {
		t.Fatal(res.Message)
	}
	return sb, fa
}

func TestAutoAlertAfterLaunch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		perms   map[string]string
		buttons []string
		action  string // "" for no tap
		button  string
	}{
		{"no permissions accepts", nil, []string{"Don’t Allow", "Allow"}, "accept", "Allow"},
		{"all allow accepts", map[string]string{"all": "allow"}, []string{"Allow Once", "Allow While Using App", "Don’t Allow"}, "accept", "Allow Once"},
		{"all deny dismisses", map[string]string{"all": "deny"}, []string{"Don’t Allow", "Allow"}, "dismiss", "Don’t Allow"},
		{"ATT allow", nil, []string{"Ask App Not to Track", "Allow"}, "accept", "Allow"},
		{"ATT deny", map[string]string{"all": "deny"}, []string{"Ask App Not to Track", "Allow"}, "dismiss", "Ask App Not to Track"},
		{"OK only is left", nil, []string{"OK"}, "", ""},
		{"Continue is left", nil, []string{"Don’t Allow", "Continue"}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sb, fa := launchThenTap(t, tc.perms, tc.buttons)
			reads := fa.sent("alert")
			if len(reads) == 0 || reads[0].Action != "read" || reads[0].App != springboardBundle {
				t.Fatalf("expected a SpringBoard read before the step, got %+v", reads)
			}
			taps := sb.taps()
			if tc.action == "" {
				if len(taps) != 0 {
					t.Fatalf("no tap expected, got %+v", taps)
				}
				return
			}
			if len(taps) != 1 || taps[0].Action != tc.action || taps[0].Button != tc.button || taps[0].App != springboardBundle {
				t.Fatalf("taps = %+v, want %s %q", taps, tc.action, tc.button)
			}
		})
	}
}

// A mixed list leaves prompts to the flow: the driver never asks.
func TestAutoAlertMixedPermissionsNeverAsks(t *testing.T) {
	_, fa := launchThenTap(t, map[string]string{"camera": "allow", "location": "deny"}, []string{"Don’t Allow", "Allow"})
	if got := fa.sent("alert"); len(got) != 0 {
		t.Fatalf("mixed permissions must not touch alerts: %+v", got)
	}
}

func TestAutoAlertOffByEnv(t *testing.T) {
	t.Setenv("DEVICELAB_IOS_AUTO_ALERTS", "0")
	_, fa := launchThenTap(t, nil, []string{"Allow"})
	if got := fa.sent("alert"); len(got) != 0 {
		t.Fatalf("DEVICELAB_IOS_AUTO_ALERTS=0 must not touch alerts: %+v", got)
	}
}

// The app's own alert is not a SpringBoard alert: the read says nothing is
// there and nothing is tapped, whatever its buttons.
func TestAutoAlertLeavesAppAlerts(t *testing.T) {
	var appReads, taps int
	d, fa, _ := newTestDriver(t, func(cmd string, a Args) (*Response, error) {
		switch cmd {
		case "find", "snapshot":
			return tree(node(1, "Button", "Home", 0, 0, 100, 40)), nil
		case "alert":
			if a.App != springboardBundle {
				appReads++
				return ok(&Payload{Present: true, Buttons: []string{"Allow"}}), nil
			}
			if a.Action != "read" {
				taps++
			}
			return ok(&Payload{}), nil
		}
		return ok(&Payload{}), nil
	})
	fakeLaunch(t, nil)
	if res := d.Execute(&flow.LaunchAppStep{AppID: "com.example"}); !res.Success {
		t.Fatal(res.Message)
	}
	if res := d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "Home"}}); !res.Success {
		t.Fatal(res.Message)
	}
	if len(fa.sent("alert")) == 0 || appReads != 0 || taps != 0 {
		t.Fatalf("reads=%d appReads=%d taps=%d", len(fa.sent("alert")), appReads, taps)
	}
}

// Outside the launch window steps do not ask: no RPC on the common path.
func TestAutoAlertNoReadOutsideLaunchWindow(t *testing.T) {
	sb := &springboardAlert{buttons: []string{"Allow"}}
	d, fa, _ := newTestDriver(t, sb.handle)
	d.autoAlert = "accept"
	if res := d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "Home"}}); !res.Success {
		t.Fatal(res.Message)
	}
	if got := fa.sent("alert"); len(got) != 0 {
		t.Fatalf("found element outside the window must not ask: %+v", got)
	}
}

// A prompt that comes later hides the element: the lookup asks after it has
// missed for a second, at most once a second, then finds the element.
func TestAutoAlertOnLookupMiss(t *testing.T) {
	var mu sync.Mutex
	prompt := true
	d, fa, _ := newTestDriver(t, func(cmd string, a Args) (*Response, error) {
		mu.Lock()
		defer mu.Unlock()
		switch cmd {
		case "find", "snapshot":
			if prompt {
				return tree(), nil
			}
			return tree(node(1, "Button", "Next", 0, 0, 100, 40)), nil
		case "alert":
			if !prompt {
				return ok(&Payload{}), nil
			}
			if a.Action == "read" {
				return ok(&Payload{Present: true, Buttons: []string{"Don’t Allow", "Allow"}}), nil
			}
			prompt = false
			return ok(&Payload{Present: true}), nil
		}
		return ok(&Payload{}), nil
	})
	d.autoAlert = "accept"
	d.SetFindTimeout(3000)
	start := time.Now()
	if res := d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "Next"}}); !res.Success {
		t.Fatal(res.Message)
	}
	if elapsed := time.Since(start); elapsed < autoAlertMissAfter {
		t.Fatalf("asked before the lookup had missed for %v (took %v)", autoAlertMissAfter, elapsed)
	}
	alerts := fa.sent("alert")
	if len(alerts) != 2 || alerts[0].Action != "read" || alerts[1].Action != "accept" || alerts[1].Button != "Allow" {
		t.Fatalf("alerts = %+v", alerts)
	}
}

func TestAutoAlertLookupMissIsRateLimited(t *testing.T) {
	d, fa, _ := newTestDriver(t, screenOf())
	d.autoAlert = "accept"
	res := d.Execute(&flow.TapOnStep{BaseStep: flow.BaseStep{TimeoutMs: 2600}, Selector: flow.Selector{Text: "Nope"}})
	if res.Success {
		t.Fatal("missing element should fail")
	}
	// Missing for 2.6 s: asks at ~1 s and ~2 s, never on every poll.
	if n := len(fa.sent("alert")); n < 1 || n > 2 {
		t.Fatalf("alert reads = %d, want 1 or 2 over %d finds", n, len(fa.sent("find")))
	}

	d2, fa2, _ := newTestDriver(t, screenOf())
	res = d2.Execute(&flow.TapOnStep{BaseStep: flow.BaseStep{TimeoutMs: 1500}, Selector: flow.Selector{Text: "Nope"}})
	if res.Success {
		t.Fatal("missing element should fail")
	}
	if n := len(fa2.sent("alert")); n != 0 {
		t.Fatalf("no launchApp, no action: alert reads = %d, want 0", n)
	}
}

// A prompt over a list takes the scrolls, so the list looks stuck: the
// prompt is answered there and the scrolling goes on.
func TestAutoAlertWhenScrollIsStuck(t *testing.T) {
	var mu sync.Mutex
	prompt, scrolls := true, 0
	d, fa, _ := newTestDriver(t, func(cmd string, a Args) (*Response, error) {
		mu.Lock()
		defer mu.Unlock()
		switch cmd {
		case "find", "snapshot":
			if !prompt && scrolls >= 1 {
				return tree(node(1, "Button", "Row 30", 0, 300, 100, 40)), nil
			}
			return tree(), nil
		case "settle":
			return ok(&Payload{ScreenHash: "same"}), nil
		case "act", "gesture", "swipe", "scroll":
			if !prompt {
				scrolls++
			}
		case "alert":
			if !prompt {
				return ok(&Payload{}), nil
			}
			if a.Action == "read" {
				return ok(&Payload{Present: true, Buttons: []string{"Don’t Allow", "Allow"}}), nil
			}
			prompt = false
			return ok(&Payload{Present: true}), nil
		}
		return ok(&Payload{}), nil
	})
	d.autoAlert = "accept"
	res := d.Execute(&flow.ScrollUntilVisibleStep{BaseStep: flow.BaseStep{TimeoutMs: 5000}, Element: flow.Selector{Text: "Row 30"}})
	if !res.Success {
		t.Fatalf("%s (alerts %+v)", res.Message, fa.sent("alert"))
	}
	if taps := fa.sent("alert"); len(taps) < 2 || taps[1].Button != "Allow" {
		t.Fatalf("alerts = %+v", taps)
	}
}
