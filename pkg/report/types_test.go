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
