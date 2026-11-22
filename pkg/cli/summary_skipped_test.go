package cli

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// A step that never ran after a failure is shown as skipped, not failed.
func TestPrintCommandSkipped(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	printCommand(report.Command{Type: "tapOn", Label: "tapOn", Status: report.StatusSkipped}, 0)
	printCommand(report.Command{Type: "assertTrue", Label: "assertTrue", Status: report.StatusFailed}, 0)
	w.Close()
	os.Stdout = stdout
	out, _ := io.ReadAll(r)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "–") || !strings.Contains(lines[0], "(skipped)") || strings.Contains(lines[0], "✗") {
		t.Errorf("skipped step line = %q, want – … (skipped)", lines[0])
	}
	if !strings.Contains(lines[1], "✗") {
		t.Errorf("failed step line = %q, want ✗", lines[1])
	}
}

// A passed step's outcome message is printed under it (#252).
func TestPrintCommandMessage(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	printCommand(report.Command{Type: "setAirplaneMode", Label: "setAirplaneMode", Status: report.StatusPassed, Message: "Airplane mode already enabled"}, 0)
	printCommand(report.Command{Type: "tapOn", Label: "tapOn", Status: report.StatusPassed}, 0)
	w.Close()
	os.Stdout = stdout
	out, _ := io.ReadAll(r)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[1], "╰─ Airplane mode already enabled") {
		t.Errorf("message line = %q", lines[1])
	}
}
