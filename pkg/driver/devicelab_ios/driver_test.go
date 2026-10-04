package devicelab_ios

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func boolp(b bool) *bool { return &b }

func TestExecuteSettlesOnlyAfterMovingSteps(t *testing.T) {
	d, fa, _ := newTestDriver(t, screenOf(node(1, "Button", "Go", 0, 0, 100, 40)))
	d.Execute(&flow.AssertVisibleStep{Selector: flow.Selector{Text: "Go"}}) // no settle: first step
	d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "Go"}})         // no settle: nothing moved yet
	if n := len(fa.sent("settle")); n != 0 {
		t.Fatalf("settles = %d, want 0 before anything moved", n)
	}
	// An assert can pass on a page that is still being pushed; it is no
	// proof the screen is still, so the next tap still settles.
	d.Execute(&flow.AssertVisibleStep{Selector: flow.Selector{Text: "Go"}})
	if n := len(fa.sent("settle")); n != 0 {
		t.Fatalf("settles = %d, want 0: asserts poll, never settle", n)
	}
	d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "Go"}})
	if n := len(fa.sent("settle")); n != 1 {
		t.Fatalf("settles = %d, want 1 (the tap after tap+assert settles)", n)
	}
	d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "Go"}})
	if n := len(fa.sent("settle")); n != 2 {
		t.Fatalf("settles = %d, want 2", n)
	}
}

func TestExecuteUnsupported(t *testing.T) {
	d, _, _ := newTestDriver(t, nil)
	for _, s := range []flow.Step{&flow.SetAirplaneModeStep{}, &flow.ToggleAirplaneModeStep{}, &flow.EvalScriptStep{}} {
		if res := d.Execute(s); res.Success {
			t.Errorf("%T should fail", s)
		}
	}
}

func TestSettersAndInfo(t *testing.T) {
	d, _, _ := newTestDriver(t, nil)
	d.SetContext(context.Background())
	d.SetFindTimeout(0)
	d.SetOptionalFindTimeout(0)
	if d.findTimeout != 300*1e6 || d.optionalTimeout != 200*1e6 {
		t.Fatal("zero must keep the budget")
	}
	if err := d.SetWaitForIdleTimeout(5); err != nil {
		t.Fatal(err)
	}
	if d.GetPlatformInfo().Platform != "ios" {
		t.Fatal("platform info")
	}
}

func TestHierarchy(t *testing.T) {
	child := node(2, "Button", "", 10, 10, 50, 20)
	child.P = 1
	child.Title = "OK"
	d, fa, _ := newTestDriver(t, screenOf(node(1, "Window", "", 0, 0, 400, 800), child))
	raw, err := d.Hierarchy()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Nodes []struct {
			Index       int                `json:"index"`
			ParentIndex *int               `json:"parentIndex"`
			Label       string             `json:"label"`
			Rect        map[string]float64 `json:"rect"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 2 || got.Nodes[0].ParentIndex != nil || *got.Nodes[1].ParentIndex != 0 ||
		got.Nodes[1].Label != "OK" || got.Nodes[1].Rect["width"] != 50 {
		t.Fatalf("hierarchy = %s", raw)
	}
	if !fa.sent("snapshot")[0].Alerts {
		t.Fatal("hierarchy should include alerts")
	}
}

func TestGetState(t *testing.T) {
	d, _, _ := newTestDriver(t, func(cmd string, a Args) (*Response, error) {
		switch cmd {
		case "keyboard":
			return ok(&Payload{Visible: true}), nil
		case "device":
			return ok(&Payload{Orientation: "portrait"}), nil
		case "app":
			return ok(&Payload{AppState: "foreground"}), nil
		}
		return ok(nil), nil
	})
	d.SetAppID("com.example")
	s := d.GetState()
	if !s.KeyboardVisible || s.Orientation != "portrait" || s.AppState != "foreground" {
		t.Fatalf("state = %+v", s)
	}
}

func TestScreenshotError(t *testing.T) {
	d, _, _ := newTestDriver(t, func(string, Args) (*Response, error) { return nil, errBoom })
	if _, err := d.Screenshot(); err == nil {
		t.Fatal("want error")
	}
	if _, err := d.Hierarchy(); err == nil {
		t.Fatal("want error")
	}
	if res := d.Execute(&flow.TakeScreenshotStep{}); res.Success {
		t.Fatal("want failure")
	}
}

func TestRandomInput(t *testing.T) {
	if len(randomInput("NUMBER", 0)) != 8 || len(randomInput("TEXT", 5)) != 5 || len(randomInput("", 0)) != 8 {
		t.Fatal("lengths")
	}
	if !strings.Contains(randomInput("EMAIL", 0), "@") || randomInput("PERSON_NAME", 0) == "" {
		t.Fatal("email/name")
	}
}

func TestScreenRecordingGuards(t *testing.T) {
	d, _, _ := newTestDriver(t, nil)
	if err := d.StopScreenRecording(filepath.Join(t.TempDir(), "x.mp4")); err == nil {
		t.Fatal("stop without start should fail")
	}
}

func TestCloseRemovesStagedApps(t *testing.T) {
	d, _, _ := newTestDriver(t, nil)
	dir := t.TempDir()
	staged := filepath.Join(dir, "sub", "App.app")
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	d.stagedApps = map[string]stagedApp{"a": {path: staged}}
	d.Close()
	if pathExists(filepath.Dir(staged)) || len(d.stagedApps) != 0 {
		t.Fatal("staged copy should be removed")
	}
}

// The taps of a tapOn with repeat: after the first do not settle before
// them, so they go out back to back as in Maestro; the step after the repeat
// settles as after any tap.
func TestRepeatTapsDoNotSettleBetween(t *testing.T) {
	d, fa, _ := newTestDriver(t, screenOf(node(1, "Button", "Go", 0, 0, 100, 40)))
	d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "Go"}}) // something moved
	d.Execute(&flow.TapOnStep{Point: "50%,50%", RepeatIndex: 0, RepeatCount: 3})
	if n := len(fa.sent("settle")); n != 1 {
		t.Fatalf("settles = %d, want 1 (the first tap of the repeat settles)", n)
	}
	d.Execute(&flow.TapOnStep{Point: "50%,50%", RepeatIndex: 1, RepeatCount: 3})
	d.Execute(&flow.TapOnStep{Point: "50%,50%", RepeatIndex: 2, RepeatCount: 3})
	if n := len(fa.sent("settle")); n != 1 {
		t.Fatalf("settles = %d, want 1: the later taps of a repeat do not settle", n)
	}
	d.Execute(&flow.TapOnStep{Selector: flow.Selector{Text: "Go"}})
	if n := len(fa.sent("settle")); n != 2 {
		t.Fatalf("settles = %d, want 2 (the tap after the repeat settles)", n)
	}
}
