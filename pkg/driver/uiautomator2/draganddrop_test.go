package uiautomator2

import (
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestDragAndDrop_PointToPoint(t *testing.T) {
	client := &MockUIA2Client{}
	d := New(client, &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, &shellMock{})

	step := &flow.DragAndDropStep{
		From:         flow.Selector{Point: "50%, 80%"},
		To:           flow.Selector{Point: "50%, 20%"},
		HoldDuration: 800,
		Duration:     1200,
	}
	result := d.dragAndDrop(step)
	if !result.Success {
		t.Fatalf("dragAndDrop failed: %s", result.Message)
	}

	if len(client.dragAndDropCalls) != 1 {
		t.Fatalf("DragAndDrop calls = %d, want 1", len(client.dragAndDropCalls))
	}
	call := client.dragAndDropCalls[0]
	if call.FromX != 500 || call.FromY != 1600 || call.ToX != 500 || call.ToY != 400 {
		t.Errorf("coordinates = %+v, want (500,1600)→(500,400)", call)
	}
	if call.HoldMs != 800 || call.MoveMs != 1200 {
		t.Errorf("durations = %+v, want hold 800 / move 1200", call)
	}
}

func TestDragAndDrop_InvalidPoint(t *testing.T) {
	client := &MockUIA2Client{}
	d := New(client, &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, &shellMock{})

	step := &flow.DragAndDropStep{
		From:         flow.Selector{Point: "not-a-point"},
		To:           flow.Selector{Point: "50%, 20%"},
		HoldDuration: 1000,
		Duration:     1000,
	}
	result := d.dragAndDrop(step)
	if result.Success {
		t.Fatal("expected failure for an unparseable point")
	}
	if !strings.Contains(result.Message, "from") {
		t.Errorf("failure should say which endpoint was bad: %s", result.Message)
	}
	if len(client.dragAndDropCalls) != 0 {
		t.Error("no gesture should be sent when an endpoint fails to resolve")
	}
}
