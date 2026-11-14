package devicelab_ios

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// The agent's start sends the app in front of the simulator to the
// background: its XCTest runner is launched as an app (simctl launch has no
// background launch, and xcodebuild launches it the same way), and once it
// is up the home screen is left. A flow that starts with launchApp never
// notices; a hierarchy read, or a flow that goes on from the screen another
// tool left, found the home screen instead. So a fresh start reads which app
// was in front before it, and launches that app again once the agent
// answers: simctl launch of a running app brings it to the front as it was
// (the same process on the same page, in about 0.25 s).
//
// simctl does not say which app is in front, but each simulator app is a
// process of the Mac, and its scheduling priority does (ps's pri, iOS 27.2):
// 47-53 for the app in front, 4 for an app in the background, 31 for the
// agent's own runner. An app that leaves the front keeps its priority for 1-5
// s, so right after a switch two apps read as in front. launchctl also lists
// system processes as apps, and the home screen's widget renderer reads 47
// while the page with widgets shows: only the apps simctl listapps knows are
// read (it lists Settings and Safari, not the renderer, Spotlight or the
// Messages view service).

// frontPriority is the lowest priority that reads as in front: above the
// background's 4 and the 31 of a background process that is not throttled
// (the agent's runner), below the front's 47.
const frontPriority = 40

// frontSettle bounds the wait for one of two apps that read as in front to
// drop, and frontPoll is how often they are read again meanwhile.
var (
	frontSettle = 5 * time.Second
	frontPoll   = 250 * time.Millisecond
)

// runningApp is an app running on the simulator.
type runningApp struct {
	id  string
	pid int
}

// frontSimctl runs simctl for frontApp and bringBack; tests replace it.
var frontSimctl = simctl

// readPriorities answers `ps -o pid=,pri=` for pids, "" when none of them
// runs any more; tests replace it.
var readPriorities = func(ctx context.Context, pids []int) (string, error) {
	list := make([]string, len(pids))
	for i, pid := range pids {
		list[i] = strconv.Itoa(pid)
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(c, "ps", "-o", "pid=,pri=", "-p", strings.Join(list, ",")).Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && c.Err() == nil && len(strings.TrimSpace(string(out))) == 0 {
		return "", nil // ps exits 1 when it finds none of the pids
	}
	return string(out), err
}

// installedApps parses `simctl listapps` for the bundle ids it lists (each
// entry's CFBundleIdentifier).
func installedApps(listapps string) map[string]bool {
	ids := map[string]bool{}
	for _, line := range strings.Split(listapps, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(k) == "CFBundleIdentifier" {
			ids[strings.Trim(strings.TrimSpace(v), `";`)] = true
		}
	}
	return ids
}

// runningApps parses `launchctl list` for the running apps (each listed as
// PID, status, UIKitApplication:BUNDLE[xxxx][rb-legacy]) that installed
// holds, leaving out XCTest runners (bundle ids ending in .xctrunner): this
// agent's, another tool's, WebDriverAgent's.
func runningApps(list string, installed map[string]bool) []runningApp {
	var apps []runningApp
	for _, line := range strings.Split(list, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || !strings.HasPrefix(f[2], "UIKitApplication:") {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil || pid <= 0 { // "-": not running
			continue
		}
		id := strings.TrimPrefix(f[2], "UIKitApplication:")
		if i := strings.IndexByte(id, '['); i >= 0 {
			id = id[:i]
		}
		if !installed[id] || strings.HasSuffix(id, ".xctrunner") {
			continue
		}
		apps = append(apps, runningApp{id: id, pid: pid})
	}
	return apps
}

// inFront keeps the apps whose priority in ps's answer reads as in front.
func inFront(apps []runningApp, ps string) []runningApp {
	pri := map[int]int{}
	for _, line := range strings.Split(ps, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		p, err2 := strconv.Atoi(f[1])
		if err1 == nil && err2 == nil {
			pri[pid] = p
		}
	}
	var front []runningApp
	for _, app := range apps {
		if p, ok := pri[app.pid]; ok && p >= frontPriority {
			front = append(front, app)
		}
	}
	return front
}

// frontApp reads the app in front of the simulator: nil with an empty why
// for the home screen, nil with why when it cannot tell. While two or more
// apps read as in front it reads them again, up to frontSettle.
func frontApp(ctx context.Context, udid string) (*runningApp, string) {
	list, err := frontSimctl(ctx, 15*time.Second, "spawn", udid, "launchctl", "list")
	if err != nil {
		return nil, fmt.Sprintf("launchctl list: %v", err)
	}
	listapps, err := frontSimctl(ctx, 15*time.Second, "listapps", udid)
	if err != nil {
		return nil, fmt.Sprintf("simctl listapps: %v", err)
	}
	apps := runningApps(list, installedApps(listapps))
	deadline := time.Now().Add(frontSettle)
	for {
		var front []runningApp
		if len(apps) > 0 {
			pids := make([]int, len(apps))
			for i, app := range apps {
				pids[i] = app.pid
			}
			out, err := readPriorities(ctx, pids)
			if err != nil {
				return nil, fmt.Sprintf("ps: %v", err)
			}
			front = inFront(apps, out)
		}
		switch {
		case len(front) == 0:
			return nil, ""
		case len(front) == 1:
			return &front[0], ""
		}
		if !time.Now().Add(frontPoll).Before(deadline) {
			ids := make([]string, len(front))
			for i, app := range front {
				ids[i] = app.id
			}
			return nil, fmt.Sprintf("%s read as in front for %s", strings.Join(ids, " and "), frontSettle)
		}
		apps = front // a switch is under way: wait for the app that left to drop
		select {
		case <-ctx.Done():
			return nil, ctx.Err().Error()
		case <-time.After(frontPoll):
		}
	}
}

// bringBack launches the app that was in front before the agent's start
// (frontApp's answer) again, and says in the log what it did.
func (a *Agent) bringBack(ctx context.Context, app *runningApp, why string) {
	switch {
	case why != "":
		notBroughtBack("which app was in front is not known: " + why)
	case app == nil:
		logger.Info("[devicelab-ios] the home screen was in front before the agent's start: nothing to bring back")
	default:
		// Without --terminate-running-process: the running app comes to
		// the front as it was.
		start := time.Now()
		out, err := frontSimctl(ctx, 30*time.Second, "launch", a.opts.UDID, app.id)
		out = strings.TrimSpace(out)
		if err != nil {
			notBroughtBack(fmt.Sprintf("simctl launch %s: %v: %s", app.id, err, out))
			return
		}
		anew := ""
		if !strings.HasSuffix(out, ": "+strconv.Itoa(app.pid)) {
			anew = ", started anew: " + out
		}
		logger.Info("[devicelab-ios] brought %s back to the front, where it was before the agent's start (%s%s)",
			app.id, time.Since(start).Round(time.Millisecond), anew)
	}
}

// notBroughtBack says why the app in front before the agent's start was not
// brought back, on the console too (notBroughtBackLine): the run may begin on
// the home screen.
func notBroughtBack(why string) {
	logger.Warn("[devicelab-ios] %s%s", notBroughtBackLine, why)
	fmt.Fprintf(frontConsole, "  ⚠ %s%s\n", notBroughtBackLine, oneLine(why))
}

// notBroughtBackLine starts the console line of an app not brought back. A
// tool that drives the runner may look for it to tell its user, so a test
// keeps it as it is.
const notBroughtBackLine = "the app in front before the agent's start was not brought back: "

// frontConsole is where notBroughtBack's line goes; tests replace it.
var frontConsole io.Writer = os.Stderr
