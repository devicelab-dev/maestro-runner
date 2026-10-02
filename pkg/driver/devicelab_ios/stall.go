package devicelab_ios

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// maxStallCaptures bounds how many stalls one run records.
const maxStallCaptures = 3

var stallCaptures atomic.Int32

// captureStall records why the agent stopped answering, before it is
// restarted: a stack sample of the agent and of each app running on the
// simulator, the process list and a screenshot, into
// <report dir>/diagnostics/stall-<n>-<time>/. The cause of a CI agent death
// (XCTest blocked in a spindump of an app that was not idle) was found only
// from such samples, taken by a hand-written workflow watchdog.
func captureStall(udid, reason string) {
	if runtime.GOOS != "darwin" || udid == "" {
		return
	}
	base := logger.Dir()
	if base == "" {
		return
	}
	n := stallCaptures.Add(1)
	if n > maxStallCaptures {
		return
	}
	dir := filepath.Join(base, "diagnostics", fmt.Sprintf("stall-%d-%s", n, time.Now().Format("150405")))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_ = os.WriteFile(filepath.Join(dir, "reason.txt"), []byte(reason+"\n"), 0o644)
	procs := simulatorProcesses(ctx, udid)
	var list strings.Builder
	for _, p := range procs {
		fmt.Fprintf(&list, "%d %s\n", p.pid, p.command)
	}
	_ = os.WriteFile(filepath.Join(dir, "processes.txt"), []byte(list.String()), 0o644)
	for _, p := range procs {
		out := filepath.Join(dir, fmt.Sprintf("sample-%s-%d.txt", p.name, p.pid))
		_ = exec.CommandContext(ctx, "sample", strconv.Itoa(p.pid), "2", "-file", out).Run()
	}
	_ = exec.CommandContext(ctx, "xcrun", "simctl", "io", udid, "screenshot", filepath.Join(dir, "screen.png")).Run()
	logger.Info("[devicelab-ios] stall diagnostics saved to %s", dir)
}

type simProcess struct {
	pid     int
	name    string
	command string
}

// simulatorProcesses is the agent and the apps running on the simulator: the
// processes whose executable lives in the simulator's device folder, under
// an installed app bundle (the agent's runner is installed the same way).
func simulatorProcesses(ctx context.Context, udid string) []simProcess {
	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return nil
	}
	return parseSimulatorProcesses(out, udid)
}

func parseSimulatorProcesses(psOutput []byte, udid string) []simProcess {
	var procs []simProcess
	scanner := bufio.NewScanner(bytes.NewReader(psOutput))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		pidText, command, ok := strings.Cut(line, " ")
		// The executable itself must be in the folder: a command that only
		// mentions the path (a shell, grep) is not a simulator process.
		at := strings.Index(command, "/Devices/"+udid+"/data/Containers/Bundle/Application/")
		if !ok || at <= 0 || strings.Contains(command[:at], " ") {
			continue
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil {
			continue
		}
		// The executable after the last ".app/" (bundle names may hold spaces).
		name := command[strings.LastIndex(command, ".app/")+len(".app/"):]
		if cut := strings.IndexAny(name, " /"); cut >= 0 {
			name = name[:cut]
		}
		procs = append(procs, simProcess{pid: pid, name: name, command: command})
	}
	return procs
}
