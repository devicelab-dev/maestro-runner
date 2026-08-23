package report

import "testing"

func TestStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   Status
		terminal bool
	}{
		{StatusPending, false},
		{StatusRunning, false},
		{StatusPassed, true},
		{StatusFailed, true},
		{StatusSkipped, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.IsTerminal(); got != tt.terminal {
				t.Errorf("Status(%q).IsTerminal() = %v, want %v", tt.status, got, tt.terminal)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
	if Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", Version, "1.0.0")
	}
}

// TestAppVersionLabel covers the display rule for #144: a release version alone
// does not identify which CI build was tested, so the build number rides along
// with it wherever the version is shown.
func TestAppVersionLabel(t *testing.T) {
	tests := []struct {
		name string
		app  App
		want string
	}{
		{"version and build", App{Version: "1.16.0", Build: "10009107"}, "v1.16.0 (10009107)"},
		{"version only", App{Version: "1.16.0"}, "v1.16.0"},
		// An app can carry a build number with no marketing version, and the
		// build is the more identifying of the two, so it is still shown.
		{"build only", App{Build: "10009107"}, "build 10009107"},
		{"neither", App{}, ""},
		{"id set but no version", App{ID: "com.example.app"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.app.VersionLabel(); got != tt.want {
				t.Errorf("VersionLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}
