package devicelab

import (
	"errors"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestDragAndDrop_PointToPoint(t *testing.T) {
	shell := &mockShell{}
	d := New(&mockDeviceLabClient{}, &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, shell)

	step := &flow.DragAndDropStep{
		From:         flow.Selector{Point: "50%, 80%"},
		To:           flow.Selector{Point: "50%, 20%"},
		HoldDuration: 800, // not controllable via input draganddrop — must not appear in the command
		Duration:     1200,
	}
	result := d.dragAndDrop(step)
	if !result.Success {
		t.Fatalf("dragAndDrop failed: %s", result.Message)
	}

	want := "input draganddrop 500 1600 500 400 1200"
	found := false
	for _, cmd := range shell.commands {
		if cmd == want {
			found = true
		}
	}
	if !found {
		t.Errorf("shell commands = %v, want %q", shell.commands, want)
	}
}

func TestDragAndDrop_ShellErrorIsClear(t *testing.T) {
	shell := &mockShell{err: errors.New("Error: Unknown command: draganddrop")}
	d := New(&mockDeviceLabClient{}, &core.PlatformInfo{ScreenWidth: 1000, ScreenHeight: 2000}, shell)

	step := &flow.DragAndDropStep{
		From:         flow.Selector{Point: "10%, 10%"},
		To:           flow.Selector{Point: "90%, 90%"},
		HoldDuration: 1000,
		Duration:     1000,
	}
	result := d.dragAndDrop(step)
	if result.Success {
		t.Fatal("expected failure when the input command errors")
	}
	if !strings.Contains(result.Message, "Android 12+") {
		t.Errorf("failure should mention the platform requirement: %s", result.Message)
	}
}
