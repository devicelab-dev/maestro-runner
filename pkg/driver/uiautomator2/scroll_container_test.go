package uiautomator2

import (
	"fmt"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// sheetRow is a bottom sheet whose list ends at 2337, above the gesture bar of
// a 2400px screen, with its last row at rowTop..rowBottom. The row isn't
// clipped to the list: at [2295,2400] it runs 63px past it, as a Compose
// sheet's last row is reported.
func sheetRow(rowTop, rowBottom int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<hierarchy rotation="0">
  <android.widget.FrameLayout class="android.widget.FrameLayout" displayed="true" enabled="true" bounds="[0,0][1080,2400]">
    <android.view.View class="android.view.View" scrollable="true" displayed="true" enabled="true" bounds="[63,63][1017,2337]">
      <android.view.View class="android.view.View" text="Target" displayed="true" enabled="true" bounds="[63,%d][1017,%d]"/>
    </android.view.View>
  </android.widget.FrameLayout>
</hierarchy>`, rowTop, rowBottom)
}

func scrollToTarget(driver *Driver, direction string, maxScrolls int) *core.CommandResult {
	return driver.scrollUntilVisible(&flow.ScrollUntilVisibleStep{
		Element:    flow.Selector{Text: "Target"},
		Direction:  direction,
		MaxScrolls: maxScrolls,
		BaseStep:   flow.BaseStep{TimeoutMs: 30000},
	})
}

// A row on screen but past its list's leading edge is scrolled into the list,
// not accepted where a tap on its centre would land on the gesture bar.
func TestScrollUntilVisibleScrollsRowHangingPastContainer(t *testing.T) {
	client := &MockUIA2Client{}
	client.sourceFunc = func() (string, error) {
		if len(client.scrollCalls) == 0 {
			return sheetRow(2295, 2400), nil
		}
		return sheetRow(1959, 2274), nil
	}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	result := scrollToTarget(driver, "down", 0)

	if !result.Success {
		t.Fatalf("expected the row once inside its list, got: %v - %s", result.Error, result.Message)
	}
	if len(client.scrollCalls) != 1 {
		t.Errorf("expected 1 scroll, got %d", len(client.scrollCalls))
	}
	if result.Element == nil || result.Element.Bounds.Y != 1959 {
		t.Errorf("result should carry the row inside its list (y 1959), got %+v", result.Element)
	}
}

// A row the list can't move any further is accepted after the one scroll that
// shows it doesn't move: the check must not fail a flow that used to pass.
func TestScrollUntilVisibleAcceptsHangingRowAtListEnd(t *testing.T) {
	client := &MockUIA2Client{sourceFunc: func() (string, error) { return sheetRow(2295, 2400), nil }}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	result := scrollToTarget(driver, "down", 0)

	if !result.Success {
		t.Fatalf("a row the list can't move must still be accepted, got: %v - %s", result.Error, result.Message)
	}
	if len(client.scrollCalls) != 1 {
		t.Errorf("expected 1 scroll, got %d", len(client.scrollCalls))
	}
}

// Only the leading edge counts: scrolling up can't bring in a row that hangs
// past the list's bottom, so it is accepted at once, as before.
func TestScrollUntilVisibleUpIgnoresOverhangAtTheBottom(t *testing.T) {
	client := &MockUIA2Client{sourceFunc: func() (string, error) { return sheetRow(2295, 2400), nil }}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	result := scrollToTarget(driver, "up", 0)

	if !result.Success {
		t.Fatalf("expected the row, got: %v - %s", result.Error, result.Message)
	}
	if len(client.scrollCalls) != 0 {
		t.Errorf("expected no scroll, got %d", len(client.scrollCalls))
	}
}

// A row that keeps moving but stays past the edge until the scrolls run out is
// accepted as it is, as before.
func TestScrollUntilVisibleAcceptsHangingRowWhenScrollsRunOut(t *testing.T) {
	client := &MockUIA2Client{}
	client.sourceFunc = func() (string, error) {
		top := 2295 + len(client.scrollCalls) // moves a pixel a scroll, never in
		return sheetRow(top, 2400), nil
	}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	result := scrollToTarget(driver, "down", 2)

	if !result.Success {
		t.Fatalf("expected the row as it is, got: %v - %s", result.Error, result.Message)
	}
	if len(client.scrollCalls) != 2 {
		t.Errorf("expected 2 scrolls, got %d", len(client.scrollCalls))
	}
}

// A row that keeps moving but never comes in is accepted after two scrolls,
// rather than spending every scroll the step allows.
func TestScrollUntilVisibleGivesHangingRowTwoScrolls(t *testing.T) {
	client := &MockUIA2Client{}
	client.sourceFunc = func() (string, error) {
		top := 2295 + len(client.scrollCalls)
		return sheetRow(top, 2400), nil
	}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, nil)

	result := scrollToTarget(driver, "down", 0)

	if !result.Success {
		t.Fatalf("expected the row as it is, got: %v - %s", result.Error, result.Message)
	}
	if len(client.scrollCalls) != 2 {
		t.Errorf("expected 2 scrolls, got %d", len(client.scrollCalls))
	}
}
