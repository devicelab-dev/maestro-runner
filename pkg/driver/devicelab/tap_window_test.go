package devicelab

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// windowClient is a client whose taps can report another window over the
// point, as maestro.Adapter's do.
type windowClient struct {
	*mockDeviceLabClient
	coveredBy    string
	checked      []core.Bounds
	plainClicks  int
	checkedClick [2]int
}

func (c *windowClient) Click(x, y int) error {
	c.plainClicks++
	return nil
}

func (c *windowClient) ClickChecked(x, y int, target core.Bounds) (string, error) {
	c.checked = append(c.checked, target)
	c.checkedClick = [2]int{x, y}
	return c.coveredBy, nil
}

// A coordinate tap for an element sends the element's bounds, so the agent
// can tell whether another window is over the point. A covered point is only
// a warning: the tap still counts as made.
func TestClickTargetChecksTheWindow(t *testing.T) {
	client := &windowClient{mockDeviceLabClient: &mockDeviceLabClient{}, coveredBy: "keyboard window [0,1500][1080,2400] layer 2"}
	d := New(client, &core.PlatformInfo{}, nil)
	target := core.Bounds{X: 100, Y: 1600, Width: 200, Height: 80}

	if err := d.clickTarget(200, 1640, target, "text=Send"); err != nil {
		t.Fatalf("clickTarget: %v", err)
	}
	if len(client.checked) != 1 || client.checked[0] != target {
		t.Fatalf("checked %v, want one check with %v", client.checked, target)
	}
	if client.checkedClick != [2]int{200, 1640} {
		t.Errorf("tapped %v, want (200,1640)", client.checkedClick)
	}
	if client.plainClicks != 0 {
		t.Errorf("tapped %d more times without the check", client.plainClicks)
	}
}

// MAESTRO_DISABLE_HIT_TEST turns the check off, as it does the
// find-and-click hit test.
func TestClickTargetHitTestDisabled(t *testing.T) {
	t.Setenv("MAESTRO_DISABLE_HIT_TEST", "1")
	client := &windowClient{mockDeviceLabClient: &mockDeviceLabClient{}}
	d := New(client, &core.PlatformInfo{}, nil)

	if err := d.clickTarget(1, 2, core.Bounds{Width: 4, Height: 4}, "x"); err != nil {
		t.Fatalf("clickTarget: %v", err)
	}
	if len(client.checked) != 0 || client.plainClicks != 1 {
		t.Errorf("checked %d, plain %d; want 0 and 1", len(client.checked), client.plainClicks)
	}
}

// A client that can't check taps plainly.
func TestClickTargetWithoutCheckingClient(t *testing.T) {
	client := &trackingClient{mockDeviceLabClient: &mockDeviceLabClient{}}
	d := New(client, &core.PlatformInfo{}, nil)

	if err := d.clickTarget(5, 6, core.Bounds{Width: 4, Height: 4}, "x"); err != nil {
		t.Fatalf("clickTarget: %v", err)
	}
}

// tapOn's coordinate path (a selector with point:) taps through the check,
// with the element's bounds.
func TestTapOnWithPointChecksTheWindow(t *testing.T) {
	inner := &mockDeviceLabClient{sourceFunc: func() (string, error) {
		return `<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout class="android.widget.FrameLayout" displayed="true" enabled="true" bounds="[0,0][1080,2400]">
    <android.widget.Button class="android.widget.Button" resource-id="com.app:id/send" clickable="true" displayed="true" enabled="true" bounds="[100,1600][300,1680]"/>
  </android.widget.FrameLayout>
</hierarchy>`, nil
	}}
	client := &windowClient{mockDeviceLabClient: inner, coveredBy: "keyboard window [0,1500][1080,2400] layer 2"}
	d := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	res := d.tapOn(&flow.TapOnStep{
		BaseStep: flow.BaseStep{TimeoutMs: 200},
		Selector: flow.Selector{ID: ".*send"},
		Point:    "50%,50%",
	})

	if !res.Success {
		t.Fatalf("a covered point is a warning, not a failure: %v - %s", res.Error, res.Message)
	}
	want := core.Bounds{X: 100, Y: 1600, Width: 200, Height: 80}
	if len(client.checked) != 1 || client.checked[0] != want {
		t.Fatalf("checked %v, want one check with %v", client.checked, want)
	}
}
