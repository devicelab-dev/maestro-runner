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

// touchClient also checks double taps and long presses.
type touchClient struct {
	windowClient
	doubleChecked []core.Bounds
	longChecked   []int
}

func (c *touchClient) DoubleClickChecked(x, y int, target core.Bounds) (string, error) {
	c.doubleChecked = append(c.doubleChecked, target)
	return c.coveredBy, nil
}

func (c *touchClient) LongClickChecked(x, y, durationMs int, target core.Bounds) (string, error) {
	c.longChecked = append(c.longChecked, durationMs)
	return c.coveredBy, nil
}

// A double tap and a long press send the element's bounds too, and a
// covered point is only a warning.
func TestDoubleAndLongClickTargetCheckTheWindow(t *testing.T) {
	client := &touchClient{windowClient: windowClient{mockDeviceLabClient: &mockDeviceLabClient{}, coveredBy: "keyboard"}}
	d := New(client, &core.PlatformInfo{}, nil)
	target := core.Bounds{X: 10, Y: 20, Width: 30, Height: 40}

	if err := d.doubleClickTarget(25, 40, target, "x"); err != nil {
		t.Fatalf("doubleClickTarget: %v", err)
	}
	if err := d.longClickTarget(25, 40, 900, target, "x"); err != nil {
		t.Fatalf("longClickTarget: %v", err)
	}
	if len(client.doubleChecked) != 1 || client.doubleChecked[0] != target {
		t.Errorf("double checked %v, want one check with %v", client.doubleChecked, target)
	}
	if len(client.longChecked) != 1 || client.longChecked[0] != 900 {
		t.Errorf("long checked %v, want one 900 ms press", client.longChecked)
	}

	t.Setenv("MAESTRO_DISABLE_HIT_TEST", "1")
	if err := d.doubleClickTarget(25, 40, target, "x"); err != nil {
		t.Fatalf("doubleClickTarget: %v", err)
	}
	if len(client.doubleChecked) != 1 {
		t.Errorf("checked with MAESTRO_DISABLE_HIT_TEST set")
	}
}
