package devicelab_ios

import (
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// fakeSettings is a real iPhone whose Settings app shows the Airplane Mode
// switch; a tap on it flips airplane mode unless stuck.
type fakeSettings struct {
	on       bool
	stuck    bool
	front    string // frontmost bundle the agent reports
	activate []string
	order    []string
}

func (f *fakeSettings) handle(cmd string, a Args) (*Response, error) {
	switch {
	case cmd == "snapshot" && a.App == settingsBundle:
		f.order = append(f.order, "read")
		v := "0"
		if f.on {
			v = "1"
		}
		sw := node(1, "Switch", airplaneModeLabel, 16, 200, 343, 44)
		sw.ID, sw.Value = airplaneModeID, v
		return tree(node(0, "StaticText", "Settings", 16, 100, 200, 40), sw), nil
	case cmd == "snapshot":
		return ok(&Payload{BundleID: f.front}), nil
	case cmd == "app" && a.Action == "activate":
		f.activate = append(f.activate, a.BundleID)
		f.order = append(f.order, "activate "+a.BundleID)
		f.front = a.BundleID
	case cmd == "act":
		f.order = append(f.order, "tap")
		if !f.stuck {
			f.on = !f.on
		}
	}
	return ok(&Payload{}), nil
}

// shortAirplaneBudgets makes the polls fast for one test.
func shortAirplaneBudgets(t *testing.T) {
	t.Helper()
	for _, v := range []*time.Duration{&airplaneFindTimeout, &airplaneVerifyTimeout, &airplanePoll} {
		orig := *v
		t.Cleanup(func() { *v = orig })
	}
	airplaneFindTimeout, airplaneVerifyTimeout, airplanePoll = 50*time.Millisecond, 50*time.Millisecond, 5*time.Millisecond
}

func newAirplaneDriver(t *testing.T, f *fakeSettings) (*Driver, *fakeAgent) {
	t.Helper()
	shortAirplaneBudgets(t)
	d, fa, _, _ := newDeviceDriver(t, "", f.handle)
	return d, fa
}

func TestAirplaneModeSimulatorSkips(t *testing.T) {
	d, fa, _ := newTestDriver(t, nil)
	for _, s := range []flow.Step{&flow.SetAirplaneModeStep{Enabled: true}, &flow.ToggleAirplaneModeStep{}} {
		res := d.Execute(s)
		if !res.Success || !strings.Contains(res.Message, "simulators don't have airplane mode (skipped)") {
			t.Errorf("%T: %v %q", s, res.Success, res.Message)
		}
	}
	if len(fa.calls) != 0 {
		t.Fatalf("simulator should not touch the agent: %v", fa.calls)
	}
}

func TestAirplaneModeAlreadyInState(t *testing.T) {
	f := &fakeSettings{on: true, front: "com.example.app"}
	d, fa := newAirplaneDriver(t, f)
	res := d.Execute(&flow.SetAirplaneModeStep{Enabled: true})
	if !res.Success || res.Message != "Airplane mode already enabled" {
		t.Fatalf("%v %q", res.Success, res.Message)
	}
	if n := len(fa.sent("act")); n != 0 {
		t.Fatalf("taps = %d, want 0", n)
	}
	if strings.Join(f.activate, ",") != settingsBundle+",com.example.app" {
		t.Fatalf("activate = %v", f.activate)
	}
}

func TestAirplaneModeSetVerifiesAndReturns(t *testing.T) {
	f := &fakeSettings{front: "com.example.app"}
	d, fa := newAirplaneDriver(t, f)
	res := d.Execute(&flow.SetAirplaneModeStep{Enabled: true})
	if !res.Success || res.Message != "Airplane mode enabled" {
		t.Fatalf("%v %q", res.Success, res.Message)
	}
	if !f.on {
		t.Fatal("airplane mode should be on")
	}
	want := "activate " + settingsBundle + "|read|tap|read|activate com.example.app"
	if got := strings.Join(f.order, "|"); got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	// The toggle is at the right end of the row, not on the label.
	tap := fa.sent("act")[0]
	if tap.Kind != "tap" || *tap.X != 16+343-30 || *tap.Y != 222 {
		t.Fatalf("tap = %s (%v,%v)", tap.Kind, *tap.X, *tap.Y)
	}
	if !d.screenMayMove {
		t.Fatal("the next action should settle after the app comes back")
	}
}

func TestAirplaneModeVerifyFails(t *testing.T) {
	f := &fakeSettings{front: "com.example.app", stuck: true}
	d, _ := newAirplaneDriver(t, f)
	res := d.Execute(&flow.SetAirplaneModeStep{Enabled: true})
	if res.Success || !strings.Contains(res.Message, "still disabled") {
		t.Fatalf("%v %q", res.Success, res.Message)
	}
	if strings.Contains(res.Message, "simulator") {
		t.Fatalf("real-device message mentions the simulator: %q", res.Message)
	}
	if last := f.activate[len(f.activate)-1]; last != "com.example.app" {
		t.Fatalf("app not returned to the front after a failure: %v", f.activate)
	}
}

func TestAirplaneModeToggle(t *testing.T) {
	f := &fakeSettings{on: true, front: "com.example.app"}
	d, fa := newAirplaneDriver(t, f)
	res := d.Execute(&flow.ToggleAirplaneModeStep{})
	if !res.Success || f.on || !strings.Contains(res.Message, "disabled") {
		t.Fatalf("%v %q on=%v", res.Success, res.Message, f.on)
	}
	if n := len(fa.sent("act")); n != 1 {
		t.Fatalf("taps = %d, want 1", n)
	}
	if last := f.activate[len(f.activate)-1]; last != "com.example.app" {
		t.Fatalf("activate = %v", f.activate)
	}
	for _, a := range fa.sent("app") {
		if a.Action == "launch" || a.Action == "terminate" {
			t.Fatalf("app must not be relaunched or stopped: %+v", a)
		}
	}
}

func TestAirplaneModeFrontmost(t *testing.T) {
	cases := []struct{ front, appID, want string }{
		{"com.other", "com.example.app", "com.other"},
		{"", "com.example.app", "com.example.app"},
		{springboardBundle, "com.example.app", ""},
		{settingsBundle, "com.example.app", ""},
	}
	for _, c := range cases {
		f := &fakeSettings{front: c.front}
		d, _ := newAirplaneDriver(t, f)
		d.SetAppID(c.appID)
		if got := d.frontmostApp(); got != c.want {
			t.Errorf("front %q: got %q, want %q", c.front, got, c.want)
		}
	}
}

func TestAirplaneModeErrors(t *testing.T) {
	shortAirplaneBudgets(t)

	// No switch on screen: the step fails and the app still comes back.
	d, fa, _, _ := newDeviceDriver(t, "", func(cmd string, a Args) (*Response, error) {
		if cmd == "snapshot" && a.App == "" {
			return ok(&Payload{BundleID: "com.example.app"}), nil
		}
		if cmd == "snapshot" {
			return tree(), nil
		}
		return ok(&Payload{}), nil
	})
	res := d.Execute(&flow.SetAirplaneModeStep{Enabled: true})
	if res.Success || !strings.Contains(res.Message, "Failed to find the Airplane Mode switch") {
		t.Fatalf("%v %q", res.Success, res.Message)
	}
	apps := fa.sent("app")
	if last := apps[len(apps)-1]; last.Action != "activate" || last.BundleID != "com.example.app" {
		t.Fatalf("last app call = %+v", last)
	}

	// Settings cannot be opened.
	d, _, _, _ = newDeviceDriver(t, "", func(cmd string, a Args) (*Response, error) {
		if cmd == "app" && a.BundleID == settingsBundle {
			return nil, errBoom
		}
		return ok(&Payload{}), nil
	})
	if res := d.Execute(&flow.ToggleAirplaneModeStep{}); res.Success || !strings.Contains(res.Message, "Failed to open Settings") {
		t.Fatalf("%v %q", res.Success, res.Message)
	}

	// The app cannot be brought back: the step fails though the switch moved.
	f := &fakeSettings{front: "com.example.app"}
	d, _, _, _ = newDeviceDriver(t, "", func(cmd string, a Args) (*Response, error) {
		if cmd == "app" && a.BundleID == "com.example.app" {
			return nil, errBoom
		}
		return f.handle(cmd, a)
	})
	if res := d.Execute(&flow.SetAirplaneModeStep{Enabled: true}); res.Success || !strings.Contains(res.Message, "back to the front failed") {
		t.Fatalf("%v %q", res.Success, res.Message)
	}
	if !f.on {
		t.Fatal("the switch should have moved")
	}
}

func TestSwitchOnAndTapPoint(t *testing.T) {
	for v, want := range map[string]bool{"1": true, "on": true, "0": false, "off": false} {
		if got, err := switchOn(v); err != nil || got != want {
			t.Errorf("%q: %v %v", v, got, err)
		}
	}
	if _, err := switchOn("maybe"); err == nil {
		t.Error("unknown value should fail")
	}
	if x, _ := airplaneTapPoint(Node{X: 10, W: 50, H: 20}); x != 35 {
		t.Errorf("narrow switch tap x = %v, want its centre", x)
	}
}
