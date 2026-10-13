package cli

import (
	"os"
	"testing"
)

// TestMain turns telemetry off for every test here. The tests run whole
// `test` commands, and each would otherwise send a usage event to the live
// endpoint from every developer machine and CI run.
func TestMain(m *testing.M) {
	os.Setenv("MAESTRO_RUNNER_NO_TELEMETRY", "1")
	os.Exit(m.Run())
}
