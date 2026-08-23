package uiautomator2

import (
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
)

// dumpsys output fragments for the keyboard-visibility check.
const (
	kbShownDumpsys  = `mFrame=[0,1584][1080,2400]`
	kbHiddenDumpsys = `mInputShown=false`
)

// closureShell returns whatever its fn produces — lets a test make the
// keyboard-visibility dumpsys depend on what the driver has done so far.
type closureShell struct {
	fn       func(cmd string) (string, error)
	commands []string
}

func (s *closureShell) Shell(cmd string) (string, error) {
	s.commands = append(s.commands, cmd)
	return s.fn(cmd)
}

// TestHideKeyboard_AppiumNoOp_FallsBackToBack is the #42 regression for the
// uiautomator2 driver: on devices where Appium's hide_keyboard is a no-op (e.g.
// Samsung), the keyboard stays up after HideKeyboard() returns success. The
// driver must verify via dumpsys and fall back to KEYCODE_BACK, which closes the
// IME. Here the keyboard is modeled as closing only once BACK is pressed.
func TestHideKeyboard_AppiumNoOp_FallsBackToBack(t *testing.T) {
	client := &MockUIA2Client{}
	shell := &closureShell{fn: func(string) (string, error) {
		// Keyboard stays shown until a BACK key is pressed (Appium call is a no-op).
		for _, kc := range client.pressKeyCalls {
			if kc == uiautomator2.KeyCodeBack {
				return kbHiddenDumpsys, nil
			}
		}
		return kbShownDumpsys, nil
	}}
	d := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, shell)

	result := d.hideKeyboard(&flow.HideKeyboardStep{})
	if !result.Success {
		t.Fatalf("expected success, got %v", result.Error)
	}
	if client.hideKeyboardCalls != 1 {
		t.Errorf("expected Appium HideKeyboard tried once, got %d", client.hideKeyboardCalls)
	}
	backs := 0
	for _, kc := range client.pressKeyCalls {
		if kc == uiautomator2.KeyCodeBack {
			backs++
		}
	}
	if backs != 1 {
		t.Errorf("expected exactly one BACK fallback, got %d (keyCalls=%v)", backs, client.pressKeyCalls)
	}
}

// TestHideKeyboard_AppiumWorks_NoStrayBack verifies that when Appium's call
// actually closes the keyboard, the driver does NOT press BACK — so it can't
// trigger the stray back-navigation that was reported on the devicelab driver.
func TestHideKeyboard_AppiumWorks_NoStrayBack(t *testing.T) {
	client := &MockUIA2Client{}
	shell := &closureShell{fn: func(string) (string, error) {
		// Keyboard closes as soon as Appium's hide_keyboard is called.
		if client.hideKeyboardCalls > 0 {
			return kbHiddenDumpsys, nil
		}
		return kbShownDumpsys, nil
	}}
	d := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, shell)

	result := d.hideKeyboard(&flow.HideKeyboardStep{})
	if !result.Success {
		t.Fatalf("expected success, got %v", result.Error)
	}
	if client.hideKeyboardCalls != 1 {
		t.Errorf("expected Appium HideKeyboard called once, got %d", client.hideKeyboardCalls)
	}
	if len(client.pressKeyCalls) != 0 {
		t.Errorf("expected NO key-event fallback (no stray BACK), got %v", client.pressKeyCalls)
	}
}

// TestHideKeyboard_NotVisible_NoOp verifies that when the keyboard isn't shown,
// the driver does nothing — no Appium call, no BACK — so a hideKeyboard step on
// a screen with no keyboard can never navigate back.
func TestHideKeyboard_NotVisible_NoOp(t *testing.T) {
	client := &MockUIA2Client{}
	shell := &closureShell{fn: func(string) (string, error) { return kbHiddenDumpsys, nil }}
	d := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2400}, shell)

	result := d.hideKeyboard(&flow.HideKeyboardStep{})
	if !result.Success {
		t.Fatalf("expected success, got %v", result.Error)
	}
	if client.hideKeyboardCalls != 0 {
		t.Errorf("expected no Appium call when keyboard already hidden, got %d", client.hideKeyboardCalls)
	}
	if len(client.pressKeyCalls) != 0 {
		t.Errorf("expected no key events when keyboard already hidden, got %v", client.pressKeyCalls)
	}
}
