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
