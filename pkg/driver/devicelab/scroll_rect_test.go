package devicelab

import (
	"fmt"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// A scroll confined to a container must stay inside it. A gesture that starts
// or ends outside the container is delivered to whatever is there instead —
// usually the parent list, which then scrolls the wrong thing.
func TestScrollPointsStayInsideTheRect(t *testing.T) {
	const x, y, w, h = 100, 600, 800, 400

	for _, direction := range []string{"up", "down", "left", "right"} {
		t.Run(direction, func(t *testing.T) {
			fromX, fromY, toX, toY := scrollPointsInRect(direction, x, y, w, h, 0.5)
			for _, p := range []struct {
				name string
				px   int
				py   int
			}{{"from", fromX, fromY}, {"to", toX, toY}} {
				if p.px < x || p.px > x+w {
					t.Errorf("%s x=%d outside [%d,%d]", p.name, p.px, x, x+w)
				}
				if p.py < y || p.py > y+h {
					t.Errorf("%s y=%d outside [%d,%d]", p.name, p.py, y, y+h)
				}
			}
		})
	}
}

func TestScrollPointsAreCentredInTheContainer(t *testing.T) {
	fromX, _, toX, _ := scrollPointsInRect("down", 100, 600, 800, 400, 0.5)
	if fromX != 500 || toX != 500 {
		t.Errorf("expected the swipe on the container's centre line x=500, got from=%d to=%d", fromX, toX)
	}
}

// carousel is a full-screen list holding a horizontal carousel at
// [0,800][1080,1200]. Target shows up inside the carousel once it has been
// scrolled `shownAfter` times.
func carousel(client *mockDeviceLabClient, shell *mockShell, shownAfter int) string {
	scrolls := client.scrollCalls
	if shell != nil {
		scrolls = len(swipes(shell))
	}
	target := ""
	if scrolls >= shownAfter {
		target = `<android.view.View class="android.view.View" text="Target" displayed="true" enabled="true" bounds="[300,850][700,1150]"/>`
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout class="android.widget.FrameLayout" displayed="true" enabled="true" bounds="[0,0][1080,2400]">
    <android.widget.ScrollView class="android.widget.ScrollView" scrollable="true" displayed="true" enabled="true" bounds="[0,0][1080,2400]">
      <android.widget.HorizontalScrollView class="android.widget.HorizontalScrollView" resource-id="com.app:id/carousel" scrollable="true" displayed="true" enabled="true" bounds="[0,800][1080,1200]">
        <android.view.View class="android.view.View" text="Card %d" displayed="true" enabled="true" bounds="[0,850][1080,1150]"/>
        %s
      </android.widget.HorizontalScrollView>
    </android.widget.ScrollView>
  </android.widget.FrameLayout>
</hierarchy>`, scrolls, target)
}

// swipes is the `input swipe` commands the driver sent, without the other
// shell calls (screen size and the like).
func swipes(shell *mockShell) []string {
	var out []string
	for _, cmd := range shell.commands {
		if strings.HasPrefix(cmd, "input swipe") {
			out = append(out, cmd)
		}
	}
	return out
}

// `from:` scrolls inside the named container: every adb swipe starts and ends
// within the carousel's bounds, not the middle of the screen.
func TestScrollUntilVisibleFromScrollsInsideContainer(t *testing.T) {
	client := &mockDeviceLabClient{}
	shell := &mockShell{}
	client.sourceFunc = func() (string, error) { return carousel(client, shell, 2), nil }
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, shell)

	result := driver.scrollUntilVisible(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "Target"},
		From:      flow.Selector{ID: ".*carousel"},
		Direction: "right",
		BaseStep:  flow.BaseStep{TimeoutMs: 30000},
	})

	if !result.Success {
		t.Fatalf("expected Target inside the carousel, got: %v - %s", result.Error, result.Message)
	}
	got := swipes(shell)
	if len(got) != 2 {
		t.Fatalf("expected 2 swipes, got %d: %q", len(got), got)
	}
	for _, cmd := range got {
		var fx, fy, tx, ty, ms int
		if _, err := fmt.Sscanf(cmd, "input swipe %d %d %d %d %d", &fx, &fy, &tx, &ty, &ms); err != nil {
			t.Fatalf("unexpected command %q: %v", cmd, err)
		}
		for _, y := range []int{fy, ty} {
			if y < 800 || y > 1200 {
				t.Errorf("swipe %q leaves the carousel (y=%d outside [800,1200])", cmd, y)
			}
		}
		if fx <= tx {
			t.Errorf("right should drag leftward, got %q", cmd)
		}
	}
}

// On the agent engine, the scroll area handed to the agent is the container
// (inset off its edges), not the screen.
func TestScrollUntilVisibleFromAgentEngineUsesContainerArea(t *testing.T) {
	client := &mockDeviceLabClient{}
	client.sourceFunc = func() (string, error) { return carousel(client, nil, 1), nil }
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	result := driver.scrollUntilVisible(&flow.ScrollUntilVisibleStep{
		Element:   flow.Selector{Text: "Target"},
		From:      flow.Selector{ID: ".*carousel"},
		Direction: "right",
		Engine:    "agent",
		BaseStep:  flow.BaseStep{TimeoutMs: 30000},
	})

	if !result.Success {
		t.Fatalf("expected Target, got: %v - %s", result.Error, result.Message)
	}
	if len(client.scrollAreas) != 1 {
		t.Fatalf("expected 1 scroll, got %d", len(client.scrollAreas))
	}
	a := client.scrollAreas[0]
	if a.Top < 800 || a.Top+a.Height > 1200 || a.Left != 0 || a.Width != 1080 {
		t.Errorf("scroll area %+v is not inside the carousel [0,800][1080,1200]", a)
	}
}

// A `from:` container that isn't on screen fails the step with a message that
// names it, rather than scrolling the whole screen instead.
func TestScrollUntilVisibleFromContainerMissing(t *testing.T) {
	client := &mockDeviceLabClient{}
	client.sourceFunc = func() (string, error) { return carousel(client, nil, 99), nil }
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	result := driver.scrollUntilVisible(&flow.ScrollUntilVisibleStep{
		Element:  flow.Selector{Text: "Target"},
		From:     flow.Selector{ID: "no_such_list"},
		BaseStep: flow.BaseStep{TimeoutMs: 1500},
	})

	if result.Success {
		t.Fatal("expected failure when the container is missing")
	}
	if !strings.Contains(result.Message, "Scroll container not found") {
		t.Errorf("message should name the missing container, got %q", result.Message)
	}
	if client.scrollCalls != 0 {
		t.Errorf("must not scroll without the container, got %d scrolls", client.scrollCalls)
	}
}
