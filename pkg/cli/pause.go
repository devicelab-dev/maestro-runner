package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/maestro-runner/pkg/executor"
	"github.com/devicelab-dev/maestro-runner/pkg/report"
)

// --pause-on-failure (#246): when a step fails the flow, the run stops there
// with the device and the app left as they are, and the terminal takes
// commands: look at the screen, run steps on it, then retry the step or let
// the failure stand. The executor side is executor.FailurePauser.

// resolvePauseOnFailure decides whether --pause-on-failure takes effect. It
// never makes a run wait for input nobody can give: without a terminal on
// stdin (CI, a pipe, a harness) or with several devices sharing the one
// terminal, the flag is ignored with a warning.
func resolvePauseOnFailure(want bool, parallel int, devices []string, stdinIsTerminal bool) (bool, string) {
	if !want {
		return false, ""
	}
	if parallel > 0 || len(devices) > 1 {
		return false, "--pause-on-failure is ignored with --parallel or several devices: they share one terminal"
	}
	if !stdinIsTerminal {
		return false, "--pause-on-failure is ignored: stdin is not a terminal, so nobody could answer the pause"
	}
	return true, ""
}

// stdinIsTerminal reports whether stdin is a terminal (a character device).
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// failurePauser returns the pauser for a sequential run, or nil when
// --pause-on-failure is off. A nil interface keeps the executor's default
// (never pause).
func failurePauser(cfg *RunConfig, device report.Device, driverName string) executor.FailurePauser {
	if !cfg.PauseOnFailure {
		return nil
	}
	return newTerminalPauser(os.Stdin, os.Stdout, filepath.Join(cfg.OutputDir, "paused"), externalHierarchyHint(device, driverName))
}

// externalHierarchyHint is the command another terminal can run against the
// paused device. Only local Android drivers answer `hierarchy` from a running
// test (hierarchy_share.go); elsewhere a second session could disturb the
// paused one, so there is no hint.
func externalHierarchyHint(device report.Device, driverName string) string {
	if !strings.EqualFold(device.Platform, "android") || driverName == "appium" || device.ID == "" {
		return ""
	}
	return fmt.Sprintf("maestro-runner --device %s hierarchy --compact", device.ID)
}

// terminalPauser answers pauses from a terminal.
type terminalPauser struct {
	lines    chan string // stdin, one line at a time; closed at EOF
	out      io.Writer
	shotsDir string
	hint     string
	saved    int
}

func newTerminalPauser(in io.Reader, out io.Writer, shotsDir, hint string) *terminalPauser {
	p := &terminalPauser{lines: make(chan string), out: out, shotsDir: shotsDir, hint: hint}
	// One reader for the whole run: a reader per pause would leave one
	// behind, blocked on stdin, to swallow the next pause's first line.
	go func() {
		defer close(p.lines)
		sc := bufio.NewScanner(in)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			p.lines <- sc.Text()
		}
	}()
	return p
}

const pauseHelp = `  Commands:
    Enter        continue: the step stays failed and the run goes on as usual
    r            retry the failed step (after you fixed the screen or the app)
    h [text]     print the screen's elements, or only those matching text
    s            save a screenshot of the screen now
    x <step>     run a step on the device in this flow's context, e.g.  x tapOn: "Next"
    ?            show these commands
`

// Pause prints where the flow stopped and takes commands until one ends the
// pause. EOF on stdin or a cancelled run lets the failure stand.
func (p *terminalPauser) Pause(ctx context.Context, ps *executor.PausedStep) executor.PauseAction {
	p.printHeader(ps)
	for {
		fmt.Fprint(p.out, "  pause> ")
		var line string
		var ok bool
		select {
		case <-ctx.Done():
			fmt.Fprintln(p.out)
			return executor.PauseContinue
		case line, ok = <-p.lines:
		}
		if !ok {
			fmt.Fprintln(p.out, "\n  stdin closed: continuing")
			return executor.PauseContinue
		}
		if action, done := p.handle(ps, strings.TrimSpace(line)); done {
			return action
		}
	}
}

func (p *terminalPauser) printHeader(ps *executor.PausedStep) {
	fmt.Fprintf(p.out, "\n  %sPAUSED on failure%s (--pause-on-failure)", color(colorBold+colorYellow), color(colorReset))
	if ps.Attempt > 1 {
		fmt.Fprintf(p.out, ", attempt %d", ps.Attempt)
	}
	fmt.Fprintln(p.out)
	fmt.Fprintf(p.out, "  Flow:  %s (%s)\n", ps.FlowName, ps.FlowFile)
	fmt.Fprintf(p.out, "  Step:  %d of %d: %s\n", ps.StepIndex+1, ps.StepCount, ps.Step)
	fmt.Fprintf(p.out, "  Error: %s\n", ps.Error)
	fmt.Fprintln(p.out, "  The device and the app are left as the failure left them.")
	if p.hint != "" {
		fmt.Fprintf(p.out, "  From another terminal: %s\n", p.hint)
	}
	fmt.Fprint(p.out, pauseHelp)
}

// handle runs one command. done is true when the pause ends with action.
func (p *terminalPauser) handle(ps *executor.PausedStep, line string) (action executor.PauseAction, done bool) {
	cmd, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)
	switch strings.ToLower(cmd) {
	case "", "c", "continue":
		return executor.PauseContinue, true
	case "r", "retry":
		fmt.Fprintln(p.out, "  retrying the step")
		return executor.PauseRetry, true
	case "h", "hierarchy":
		p.printHierarchy(ps, arg)
	case "s", "screenshot":
		p.saveScreenshot(ps)
	case "x", "run":
		p.runSteps(ps, arg)
	case "?", "help":
		fmt.Fprint(p.out, pauseHelp)
	default:
		fmt.Fprintf(p.out, "  unknown command %q; ? shows the commands\n", cmd)
	}
	return executor.PauseContinue, false
}

func (p *terminalPauser) printHierarchy(ps *executor.PausedStep, find string) {
	raw, err := ps.Hierarchy()
	if err != nil {
		fmt.Fprintf(p.out, "  hierarchy: %v\n", err)
		return
	}
	text, err := formatHierarchy(raw, true, find)
	if err != nil {
		fmt.Fprintf(p.out, "  hierarchy: %v\n", err)
		return
	}
	fmt.Fprintln(p.out, text)
}

func (p *terminalPauser) saveScreenshot(ps *executor.PausedStep) {
	data, err := ps.Screenshot()
	if err == nil {
		err = os.MkdirAll(p.shotsDir, 0o755)
	}
	if err != nil {
		fmt.Fprintf(p.out, "  screenshot: %v\n", err)
		return
	}
	p.saved++
	name := strings.TrimSuffix(filepath.Base(ps.FlowFile), filepath.Ext(ps.FlowFile))
	path := filepath.Join(p.shotsDir, fmt.Sprintf("%s-step%d-%d.png", name, ps.StepIndex+1, p.saved))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fmt.Fprintf(p.out, "  screenshot: %v\n", err)
		return
	}
	fmt.Fprintf(p.out, "  saved %s\n", path)
}

func (p *terminalPauser) runSteps(ps *executor.PausedStep, yamlStep string) {
	if yamlStep == "" {
		fmt.Fprintln(p.out, `  x needs a step, e.g.  x tapOn: "Next"`)
		return
	}
	outcomes, err := ps.Run(yamlStep)
	if err != nil {
		fmt.Fprintf(p.out, "  %v\n", err)
		return
	}
	for _, o := range outcomes {
		if o.Passed {
			fmt.Fprintf(p.out, "  %s✓%s %s\n", color(colorGreen), color(colorReset), o.Step)
		} else {
			fmt.Fprintf(p.out, "  %s✗%s %s: %s\n", color(colorRed), color(colorReset), o.Step, o.Error)
		}
	}
}
