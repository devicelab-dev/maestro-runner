package executor

import (
	"strings"
	"testing"
)

func TestRunShellMessage(t *testing.T) {
	tests := []struct {
		name    string
		command string
		output  string
		want    string
	}{
		{"silent command", "adb shell input keyevent 82", "", "Ran: adb shell input keyevent 82"},
		{"single line", "echo hi", "hi", "Ran: echo hi → hi"},
		{"multi-line is elided", "adb devices", "List of devices\nemulator-5554", "Ran: adb devices → List of devices…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runShellMessage(tt.command, tt.output); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncateOutputKeepsTheTail(t *testing.T) {
	// The end of a runaway output is where the failure is explained, so that
	// is the half worth keeping.
	long := strings.Repeat("a", maxCapturedOutput) + "THE-INTERESTING-PART"

	got := truncateOutput(long)

	if !strings.HasSuffix(got, "THE-INTERESTING-PART") {
		t.Error("truncation dropped the tail")
	}
	if !strings.HasPrefix(got, "…(truncated)…") {
		t.Errorf("truncation should announce itself, got %q", got[:30])
	}
	if len(got) > maxCapturedOutput+64 {
		t.Errorf("truncated output is still %d bytes", len(got))
	}
}

func TestTruncateOutputLeavesShortOutputAlone(t *testing.T) {
	if got := truncateOutput("short"); got != "short" {
		t.Errorf("got %q, want it unchanged", got)
	}
}
