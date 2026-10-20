package simulator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// recordOpen fakes xcode-select (printing devDir, or failing when it is
// empty) and records the arguments `open` is run with.
func recordOpen(t *testing.T, devDir string) *[]string {
	t.Helper()
	var opened []string
	withFakeExec(t, func(name string, args ...string) *exec.Cmd {
		switch name {
		case "xcode-select":
			if devDir == "" {
				return exec.Command("false")
			}
			return exec.Command("printf", "%s\n", devDir)
		case "open":
			opened = append(opened, strings.Join(args, " "))
			return exec.Command("true")
		}
		t.Errorf("unexpected command %s %v", name, args)
		return exec.Command("false")
	})
	return &opened
}

func TestOpenSimulatorUI_SimulatorApp(t *testing.T) {
	dev := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dev, "Applications", "Simulator.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	opened := recordOpen(t, dev)
	openSimulatorUI()
	if got := strings.Join(*opened, "|"); got != "-a Simulator" {
		t.Errorf("Xcode with Simulator.app: open %q, want -a Simulator", got)
	}
}

func TestOpenSimulatorUI_DeviceHub(t *testing.T) {
	opened := recordOpen(t, t.TempDir()) // Xcode 27: no Simulator.app
	openSimulatorUI()
	if got := strings.Join(*opened, "|"); got != "-b com.apple.dt.Devices" {
		t.Errorf("Xcode without Simulator.app: open %q, want -b com.apple.dt.Devices", got)
	}
}

func TestOpenSimulatorUI_NoDeveloperDir(t *testing.T) {
	opened := recordOpen(t, "")
	openSimulatorUI()
	if got := strings.Join(*opened, "|"); got != "-a Simulator" {
		t.Errorf("unknown developer dir: open %q, want the old -a Simulator", got)
	}
}

// The simulator is booted whether or not its window opens.
func TestBootSimulator_OpenFailureIgnored(t *testing.T) {
	withFakeLookPath(t, func(string) (string, error) { return "/usr/bin/xcrun", nil })
	var opened []string
	withFakeExec(t, func(name string, args ...string) *exec.Cmd {
		switch {
		case name == "open":
			opened = append(opened, strings.Join(args, " "))
			return exec.Command("false")
		case name == "xcode-select":
			return exec.Command("false")
		case len(args) > 1 && args[1] == "list":
			return exec.Command("printf", "%s", simctlListJSON)
		}
		return exec.Command("true") // simctl boot, bootstatus
	})
	if err := BootSimulator("BBBB-2222", 5*time.Second); err != nil {
		t.Fatalf("BootSimulator: %v", err)
	}
	if len(opened) != 1 {
		t.Errorf("expected one open attempt, got %v", opened)
	}
}
