package device

import (
	"errors"
	"strings"
	"testing"
)

// fakeShell answers each command with the output of the first prefix it starts with.
type fakeShell struct {
	commands []string
	outputs  map[string]string
	err      error
}

func (f *fakeShell) Shell(cmd string) (string, error) {
	f.commands = append(f.commands, cmd)
	for prefix, out := range f.outputs {
		if strings.HasPrefix(cmd, prefix) {
			return out, f.err
		}
	}
	return "", f.err
}

// installingShell is a fakeShell that can also install Appium Settings, like AndroidDevice.
type installingShell struct {
	fakeShell
	installErr error
	installs   int
}

func (s *installingShell) EnsureAppiumSettings() error {
	s.installs++
	return s.installErr
}

func TestSetMockLocation_StartsLocationService(t *testing.T) {
	sh := &fakeShell{outputs: map[string]string{
		"am start-foreground-service": "Starting service: Intent { cmp=io.appium.settings/.LocationService (has extras) }",
	}}

	if err := SetMockLocation(sh, "39.038177", "-84.736191"); err != nil {
		t.Fatalf("SetMockLocation: %v", err)
	}

	want := []string{
		"appops set io.appium.settings android:mock_location allow",
		"am start-foreground-service --user 0 -n io.appium.settings/.LocationService --es latitude 39.038177 --es longitude -84.736191",
	}
	if strings.Join(sh.commands, "\n") != strings.Join(want, "\n") {
		t.Errorf("commands:\n%s\nwant:\n%s", strings.Join(sh.commands, "\n"), strings.Join(want, "\n"))
	}
}

func TestSetMockLocation_FallsBackToStartServiceOnOldAndroid(t *testing.T) {
	sh := &fakeShell{outputs: map[string]string{
		"am start-foreground-service": "Error: Unknown command: start-foreground-service",
		"am startservice":             "Starting service: Intent { cmp=io.appium.settings/.LocationService (has extras) }",
	}}

	if err := SetMockLocation(sh, "1.5", "2.5"); err != nil {
		t.Fatalf("SetMockLocation: %v", err)
	}
	if len(sh.commands) != 3 || !strings.HasPrefix(sh.commands[2], "am startservice --user 0 -n io.appium.settings/.LocationService") {
		t.Errorf("expected a startservice fallback, got: %v", sh.commands)
	}
}

func TestSetMockLocation_FailsWhenTheServiceDoesNotStart(t *testing.T) {
	// am exits 0 over adb shell and only prints the error, which used to be reported as success.
	sh := &fakeShell{outputs: map[string]string{
		"am start": "Error: Not found; no service started.",
	}}

	err := SetMockLocation(sh, "1", "2")
	if err == nil || !strings.Contains(err.Error(), "no service started") {
		t.Errorf("expected the am error, got: %v", err)
	}
}

func TestSetMockLocation_FailsWhenTheAppopIsRefused(t *testing.T) {
	sh := &fakeShell{outputs: map[string]string{
		"appops set": "Error: No UID for io.appium.settings in user 0",
	}}

	err := SetMockLocation(sh, "1", "2")
	if err == nil || !strings.Contains(err.Error(), "No UID") {
		t.Errorf("expected the appops error, got: %v", err)
	}
	if len(sh.commands) != 1 {
		t.Errorf("expected to stop after appops, got: %v", sh.commands)
	}
}

func TestSetMockLocation_PropagatesShellErrors(t *testing.T) {
	sh := &fakeShell{err: errors.New("device offline")}

	if err := SetMockLocation(sh, "1", "2"); err == nil || !strings.Contains(err.Error(), "device offline") {
		t.Errorf("expected the shell error, got: %v", err)
	}
}

func TestSetMockLocation_RejectsCoordinatesThatAreNotNumbers(t *testing.T) {
	for _, tt := range []struct{ lat, lon string }{
		{"abc", "2"},
		{"1", "2; reboot"},
		{"", "2"},
	} {
		sh := &fakeShell{}
		if err := SetMockLocation(sh, tt.lat, tt.lon); err == nil {
			t.Errorf("expected an error for lat=%q lon=%q", tt.lat, tt.lon)
		}
		if len(sh.commands) != 0 {
			t.Errorf("expected no commands for lat=%q lon=%q, got: %v", tt.lat, tt.lon, sh.commands)
		}
	}
}

func TestSetMockLocation_InstallsAppiumSettingsFirst(t *testing.T) {
	sh := &installingShell{}

	if err := SetMockLocation(sh, "1", "2"); err != nil {
		t.Fatalf("SetMockLocation: %v", err)
	}
	if sh.installs != 1 {
		t.Errorf("expected one install check, got %d", sh.installs)
	}

	failing := &installingShell{installErr: errors.New("APK not found")}
	if err := SetMockLocation(failing, "1", "2"); err == nil || !strings.Contains(err.Error(), "APK not found") {
		t.Errorf("expected the install error, got: %v", err)
	}
	if len(failing.commands) != 0 {
		t.Errorf("expected no commands after a failed install, got: %v", failing.commands)
	}
}
