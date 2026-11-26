package devicelab

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// stillClient scripts the target's bounds read by FindFirstOf, one per call
// (the last repeats), and records how many reads came before the tap.
type stillClient struct {
	*appLifecycleClient
	rects        []uiautomator2.ElementRect
	findFirstErr error
	reads        int
	readsAtTap   []int
}

func (s *stillClient) FindFirstOf([]string) (*uiautomator2.Element, error) {
	s.reads++
	if s.findFirstErr != nil {
		return nil, s.findFirstErr
	}
	i := s.reads - 1
	if i >= len(s.rects) {
		i = len(s.rects) - 1
	}
	return uiautomator2.NewCachedElement("e", "Start", s.rects[i]), nil
}

func (s *stillClient) FindAndClickChecked(strategy, selector string, w, h int, hitTest bool) (*uiautomator2.Element, bool, string, error) {
	s.readsAtTap = append(s.readsAtTap, s.reads)
	return s.scriptedClient.FindAndClickChecked(strategy, selector, w, h, hitTest)
}

var (
	stillRest  = uiautomator2.ElementRect{X: 45, Y: 328, Width: 990, Height: 366}
	stillMoved = uiautomator2.ElementRect{X: 45, Y: 731, Width: 990, Height: 366}
)

func newStillDriver(t *testing.T, rects ...uiautomator2.ElementRect) (*Driver, *stillClient) {
	t.Helper()
	oldWindow, oldCap := launchStillWindow, launchStillCap
	launchStillWindow, launchStillCap = 150*time.Millisecond, 2*time.Second
	t.Cleanup(func() { launchStillWindow, launchStillCap = oldWindow, oldCap })

	client := &stillClient{
		appLifecycleClient: &appLifecycleClient{scriptedClient: &scriptedClient{trackingClient: newTrackingClient()}},
		rects:              rects,
	}
	client.findAndClickReturn = uiautomator2.NewCachedElement("e", "Start", stillRest)
	return New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, &mockShell{}), client
}

func tapStart(t *testing.T, d *Driver) {
	t.Helper()
	res := d.tapOn(&flow.TapOnStep{Selector: flow.Selector{Text: "Start"}})
	if !res.Success {
		t.Fatalf("tapOn: %v", res.Error)
	}
}

func launch(t *testing.T, d *Driver, step *flow.LaunchAppStep) {
	t.Helper()
	if step.AppID == "" {
		step.AppID = "com.test.app"
	}
	if res := d.launchApp(step); !res.Success {
		t.Fatalf("launchApp: %v", res.Error)
	}
}

// The target is first read where the window starts its slide-in, then where
// it comes to rest: the tap waits until it has stayed at rest for the window.
func TestTapOn_FirstTapAfterLaunch_WaitsForTargetToStayPut(t *testing.T) {
	d, client := newStillDriver(t, stillMoved, stillMoved, stillMoved, stillRest)
	launch(t, d, &flow.LaunchAppStep{})

	start := time.Now()
	tapStart(t, d)

	if len(client.readsAtTap) != 1 {
		t.Fatalf("taps = %d, want 1", len(client.readsAtTap))
	}
	// 3 moved reads, then at rest for 150ms at 50ms a read: 4 more at least.
	if got := client.readsAtTap[0]; got < 7 {
		t.Errorf("tapped after %d reads, want the target at rest for the window first (>= 7)", got)
	}
	if elapsed := time.Since(start); elapsed < 250*time.Millisecond {
		t.Errorf("tapped after %v, before the target had stayed put for the window", elapsed)
	}
}

func TestTapOn_FirstTapAfterLaunch_ClearStateWaitsToo(t *testing.T) {
	d, client := newStillDriver(t, stillRest)
	launch(t, d, &flow.LaunchAppStep{ClearState: true})
	tapStart(t, d)
	if client.reads == 0 {
		t.Error("the first tap after launchApp with clearState read no bounds")
	}
}

func TestTapOn_OnlyFirstTapAfterLaunchWaits(t *testing.T) {
	d, client := newStillDriver(t, stillRest)
	launch(t, d, &flow.LaunchAppStep{})
	tapStart(t, d)
	reads := client.reads
	tapStart(t, d)
	if client.reads != reads {
		t.Errorf("second tap read the bounds %d more times, want none", client.reads-reads)
	}
}

func TestTapOn_NoWaitWithoutFreshLaunch(t *testing.T) {
	stopApp := false
	cases := map[string]func(*Driver){
		"no launch":      func(*Driver) {},
		"stopApp: false": func(d *Driver) { launch(t, d, &flow.LaunchAppStep{StopApp: &stopApp}) },
		"another step in between": func(d *Driver) {
			launch(t, d, &flow.LaunchAppStep{})
			d.Execute(&flow.BackStep{})
		},
		"settling off": func(d *Driver) {
			if err := d.SetWaitForIdleTimeout(0); err != nil {
				t.Fatal(err)
			}
			launch(t, d, &flow.LaunchAppStep{})
		},
		"window 0": func(d *Driver) {
			launchStillWindow = 0
			launch(t, d, &flow.LaunchAppStep{})
		},
	}
	for name, before := range cases {
		t.Run(name, func(t *testing.T) {
			d, client := newStillDriver(t, stillRest)
			before(d)
			tapStart(t, d)
			if client.reads != 0 {
				t.Errorf("tap read the bounds %d times, want no wait", client.reads)
			}
		})
	}
}

// A target that never stops moving (a carousel) is tapped once the cap has
// passed, not after the whole lookup timeout.
func TestTapOn_FirstTapAfterLaunch_CapsMovingTarget(t *testing.T) {
	var rects []uiautomator2.ElementRect
	for i := 0; i < 200; i++ {
		rects = append(rects, uiautomator2.ElementRect{X: i, Y: 328, Width: 990, Height: 366})
	}
	d, client := newStillDriver(t, rects...)
	launchStillCap = 300 * time.Millisecond
	launch(t, d, &flow.LaunchAppStep{})

	start := time.Now()
	tapStart(t, d)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("tapped after %v, want about the 300ms cap", elapsed)
	}
	if len(client.readsAtTap) != 1 {
		t.Errorf("taps = %d, want 1", len(client.readsAtTap))
	}
}

// While the target is not on screen yet the tap keeps polling and, if it
// never shows, fails with the lookup's error as before.
func TestTapOn_FirstTapAfterLaunch_NotFoundKeepsError(t *testing.T) {
	d, client := newStillDriver(t, stillRest)
	client.findFirstErr = errors.New("Element not found in any of 2 strategies")
	launch(t, d, &flow.LaunchAppStep{})

	res := d.tapOn(&flow.TapOnStep{
		Selector: flow.Selector{Text: "Start"},
		BaseStep: flow.BaseStep{TimeoutMs: 300},
	})
	if res.Success {
		t.Fatal("tapOn succeeded with no target on screen")
	}
	if len(client.readsAtTap) != 0 {
		t.Errorf("tapped %d times before the target showed", len(client.readsAtTap))
	}
	if !strings.Contains(res.Message, "any of 2 strategies") {
		t.Errorf("message %q does not carry the lookup error", res.Message)
	}
}
