package core

import (
	"fmt"
	"strings"
)

// Dark-mode (night mode / appearance) plumbing shared by the drivers.
//
// The two platforms expose it very differently — Android through `cmd uimode`,
// iOS simulators through `simctl ui appearance` — but the flow-level commands
// are the same four, so the string handling lives here and each driver keeps
// only its own transport.

// AndroidDarkModeCommand returns the shell command that sets night mode.
func AndroidDarkModeCommand(enabled bool) string {
	if enabled {
		return "cmd uimode night yes"
	}
	return "cmd uimode night no"
}

// AndroidDarkModeQuery is the shell command that reports the current mode.
const AndroidDarkModeQuery = "cmd uimode night"

// ParseAndroidNightMode reads the output of `cmd uimode night`, which prints a
// line like "Night mode: yes".
//
// Accepts the bare value too, since the command has answered both ways across
// Android versions, and treats "auto"/"custom" as light: those are schedule
// modes where the effective appearance depends on the time of day, so the only
// honest answer from this query alone is "not currently forced dark".
func ParseAndroidNightMode(output string) (bool, error) {
	value := strings.TrimSpace(output)
	if idx := strings.LastIndex(value, ":"); idx >= 0 {
		value = strings.TrimSpace(value[idx+1:])
	}
	switch strings.ToLower(value) {
	case "yes", "true", "1":
		return true, nil
	case "no", "false", "0", "auto", "custom", "auto_custom", "unknown":
		return false, nil
	default:
		return false, fmt.Errorf("unrecognized night mode %q", strings.TrimSpace(output))
	}
}

// IOSAppearanceValue maps a dark-mode boolean to the simctl appearance name.
func IOSAppearanceValue(enabled bool) string {
	if enabled {
		return "dark"
	}
	return "light"
}

// ParseIOSAppearance reads an iOS appearance name. Both sources spell it the
// same way: the output of `simctl ui <udid> appearance` on a simulator, and the
// devicelab runner's appearance command, which works on physical devices too.
func ParseIOSAppearance(output string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "dark":
		return true, nil
	case "light":
		return false, nil
	default:
		return false, fmt.Errorf("unrecognized appearance %q", strings.TrimSpace(output))
	}
}

// DarkModeStateName renders a mode for user-facing messages.
func DarkModeStateName(enabled bool) string {
	if enabled {
		return "dark"
	}
	return "light"
}

// DarkModeAssertionError builds the failure for assertDarkMode/assertLightMode,
// naming both what was wanted and what was found.
func DarkModeAssertionError(want, got bool) error {
	return fmt.Errorf("expected %s mode but the device is in %s mode",
		DarkModeStateName(want), DarkModeStateName(got))
}
