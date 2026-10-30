// Command hierparity checks that the DeviceLab Android agent's hierarchy has
// every labelled element `uiautomator dump` has, in the same place.
//
// A screen's elements with a text or content-desc are read twice: by the
// agent (`maestro-runner hierarchy`) and by Android's own `uiautomator dump`.
// Each element of the dump must appear in the agent's with the same label and
// bounds overlapping by at least --min-iou (recall), and the agent's elements
// in the dump's window must appear in the dump (precision). It catches silent
// loss of part of the tree, such as the a11y-prefetch truncation. Exit status:
// 0 both at or above the thresholds, 1 below, 2 bad usage or unreadable input.
//
// On a device, with the screen to check in front and left alone:
//
//	go run ./tools/hierparity --device <serial> [--runner maestro-runner] [--save dir]
//
// The two reads run one after the other (only one process can hold
// UiAutomation), so a screen that changes between them reads as a loss.
// From two saved reads:
//
//	go run ./tools/hierparity --agent agent.json --dump dump.xml
//
// uiautomator dump reads the active window only, and the agent every window,
// so the agent's elements are counted only inside the dump's window and not
// from SystemUI.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("hierparity", flag.ContinueOnError)
	fl.SetOutput(stderr)
	device := fl.String("device", "", "device serial: read both hierarchies live")
	runner := fl.String("runner", "maestro-runner", "maestro-runner binary for the agent's read")
	save := fl.String("save", "", "directory to save the two reads in (live mode)")
	agentPath := fl.String("agent", "", "saved `maestro-runner hierarchy` JSON")
	dumpPath := fl.String("dump", "", "saved `uiautomator dump` XML")
	minRecall := fl.Float64("min-recall", 0.9, "lowest recall that passes")
	minPrecision := fl.Float64("min-precision", 0.8, "lowest precision that passes")
	minIoU := fl.Float64("min-iou", 0.5, "lowest bounds overlap (IoU) that counts as the same place")
	show := fl.Int("show", 10, "how many missing and extra elements to list")
	if err := fl.Parse(args); err != nil {
		return 2
	}

	var agentRaw, dumpRaw []byte
	var err error
	switch {
	case *device != "":
		agentRaw, dumpRaw, err = readLive(*device, *runner)
		if err == nil && *save != "" {
			err = saveReads(*save, agentRaw, dumpRaw)
		}
	case *agentPath != "" && *dumpPath != "":
		if agentRaw, err = os.ReadFile(*agentPath); err == nil {
			dumpRaw, err = os.ReadFile(*dumpPath)
		}
	default:
		fmt.Fprintln(stderr, "usage: hierparity --device <serial> | --agent agent.json --dump dump.xml")
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "hierparity:", err)
		return 2
	}

	agent, err := agentElements(bytes.NewReader(agentRaw))
	if err != nil {
		fmt.Fprintln(stderr, "hierparity:", err)
		return 2
	}
	ref, window, err := dumpElements(bytes.NewReader(dumpRaw))
	if err != nil {
		fmt.Fprintln(stderr, "hierparity:", err)
		return 2
	}
	res := compare(ref, window, agent, *minIoU)

	fmt.Fprintf(stdout, "recall    %.3f  (%d of %d dump elements found in the agent's)\n", res.Recall, res.Matched, res.Reference)
	fmt.Fprintf(stdout, "precision %.3f  (%d of %d agent elements in the window found in the dump)\n", res.Precision, res.Matched, res.Agent)
	list(stdout, "missing from the agent's", res.Missing, *show)
	list(stdout, "only in the agent's", res.Extra, *show)
	if res.Recall < *minRecall || res.Precision < *minPrecision {
		fmt.Fprintf(stdout, "FAIL: below recall %.2f / precision %.2f\n", *minRecall, *minPrecision)
		return 1
	}
	fmt.Fprintln(stdout, "ok")
	return 0
}

func list(w io.Writer, title string, elems []element, n int) {
	if len(elems) == 0 {
		return
	}
	fmt.Fprintf(w, "%s (%d):\n", title, len(elems))
	for i, e := range elems {
		if i == n {
			fmt.Fprintf(w, "  … %d more\n", len(elems)-n)
			break
		}
		fmt.Fprintf(w, "  %q  [%d,%d %dx%d]  %s\n", e.Label, e.Rect.X, e.Rect.Y, e.Rect.W, e.Rect.H, e.ID)
	}
}

// readLive reads the agent's hierarchy, then stops the agent so that
// uiautomator dump can take UiAutomation, then dumps.
func readLive(serial, runner string) (agent, dump []byte, err error) {
	agent, err = exec.Command(runner, "--device", serial, "hierarchy").Output()
	if err != nil {
		return nil, nil, fmt.Errorf("%s hierarchy: %w", runner, err)
	}
	adb := func(args ...string) ([]byte, error) {
		return exec.Command("adb", append([]string{"-s", serial}, args...)...).Output()
	}
	_, _ = adb("shell", "am", "force-stop", "dev.devicelab.driver.android")
	const path = "/data/local/tmp/hierparity.xml"
	if out, err := adb("shell", "uiautomator", "dump", path); err != nil {
		return nil, nil, fmt.Errorf("uiautomator dump: %w (%s)", err, bytes.TrimSpace(out))
	}
	dump, err = adb("exec-out", "cat", path)
	if err != nil {
		return nil, nil, fmt.Errorf("read the dump: %w", err)
	}
	return agent, dump, nil
}

func saveReads(dir string, agent, dump []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), agent, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "dump.xml"), dump, 0o644)
}
