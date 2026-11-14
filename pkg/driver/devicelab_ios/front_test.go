package devicelab_ios

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// launchdList is `launchctl list` on a simulator (iOS 27.2): Settings and
// Safari running, Maps not, the agent's runner and WebDriverAgent's running,
// and three system processes listed as apps: the home screen's widget
// renderer, Spotlight and the Messages view service.
const launchdList = `PID	Status	Label
-	0	com.apple.progressd
45983	0	com.apple.CoreAuthentication.daemon
25658	0	UIKitApplication:com.apple.Preferences[2b8b][rb-legacy]
45920	0	UIKitApplication:com.apple.mobilesafari[d3ff][rb-legacy]
-	0	UIKitApplication:com.apple.Maps[1c2d][rb-legacy]
48829	0	UIKitApplication:dev.devicelab.agent.uitests.xctrunner[4364][rb-legacy]
51000	0	UIKitApplication:com.facebook.WebDriverAgentRunner.xctrunner[77aa][rb-legacy]
93754	0	UIKitApplication:com.apple.chrono.WidgetRenderer-Default[a3a3][rb-legacy]
93805	0	UIKitApplication:com.apple.Spotlight[8b49][rb-legacy]
46071	0	UIKitApplication:com.apple.iMessageAppsViewService[3a9f][rb-legacy]
`

// listApps is `simctl listapps` on that simulator, cut short: the apps a
// person sees and the agent's, none of the system processes above.
const listApps = `{
    "com.apple.Maps" =     {
        ApplicationType = System;
        CFBundleDisplayName = Maps;
        CFBundleIdentifier = "com.apple.Maps";
        GroupContainers =         {
            "group.com.apple.Maps" = "file:///x/";
        };
    };
    "com.apple.Preferences" =     {
        ApplicationType = System;
        CFBundleIdentifier = "com.apple.Preferences";
    };
    "com.apple.mobilesafari" =     {
        ApplicationType = System;
        CFBundleIdentifier = "com.apple.mobilesafari";
    };
    "dev.devicelab.agent.uitests.xctrunner" =     {
        ApplicationType = User;
        CFBundleIdentifier = "dev.devicelab.agent.uitests.xctrunner";
    };
    "com.facebook.WebDriverAgentRunner.xctrunner" =     {
        ApplicationType = User;
        CFBundleIdentifier = "com.facebook.WebDriverAgentRunner.xctrunner";
    };
}
`

const settings, safari, renderer = 25658, 45920, 93754

// fakeFront stands in for simctl and ps: launchctl answers list (or
// listErr), listapps answers apps (or appsErr), ps answers each pid the
// priority its function gives at that call (absent: not running), and a
// launch is recorded.
type fakeFront struct {
	mu       sync.Mutex
	list     string
	listErr  error
	apps     string
	appsErr  error
	pri      func(pid, call int) (int, bool)
	psErr    error
	psCalls  int
	launches []string
	launch   func(id string) (string, error)
}

func (f *fakeFront) install(t *testing.T) {
	t.Helper()
	savedSimctl, savedPS := frontSimctl, readPriorities
	savedSettle, savedPoll := frontSettle, frontPoll
	t.Cleanup(func() {
		frontSimctl, readPriorities = savedSimctl, savedPS
		frontSettle, frontPoll = savedSettle, savedPoll
	})
	frontSettle, frontPoll = 300*time.Millisecond, 20*time.Millisecond
	frontSimctl = func(_ context.Context, timeout time.Duration, args ...string) (string, error) {
		if timeout <= 0 {
			t.Errorf("simctl %v without a deadline", args)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch cmd := strings.Join(args, " "); {
		case cmd == "spawn SIM-1 launchctl list":
			return f.list, f.listErr
		case cmd == "listapps SIM-1":
			return f.apps, f.appsErr
		case strings.HasPrefix(cmd, "launch "):
			f.launches = append(f.launches, cmd)
			if f.launch != nil {
				return f.launch(args[len(args)-1])
			}
			return args[len(args)-1] + ": 25658", nil
		default:
			t.Errorf("unexpected simctl %s", cmd)
			return "", errors.New("unexpected")
		}
	}
	readPriorities = func(_ context.Context, pids []int) (string, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.psCalls++
		if f.psErr != nil {
			return "", f.psErr
		}
		var b strings.Builder
		for _, pid := range pids {
			if p, ok := f.pri(pid, f.psCalls); ok {
				fmt.Fprintf(&b, "%5d %3d\n", pid, p)
			}
		}
		return b.String(), nil
	}
}

// priorities answers fixed priorities by pid.
func priorities(m map[int]int) func(int, int) (int, bool) {
	return func(pid, _ int) (int, bool) { p, ok := m[pid]; return p, ok }
}

// captureFront collects the log and the console of the front app's code.
func captureFront(t *testing.T) (log func() string, console *bytes.Buffer) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runner.log")
	if err := logger.Init(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(logger.Close)
	console = &bytes.Buffer{}
	saved := frontConsole
	frontConsole = console
	t.Cleanup(func() { frontConsole = saved })
	return func() string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}, console
}

func TestRunningApps(t *testing.T) {
	got := runningApps(launchdList, installedApps(listApps))
	want := []runningApp{{"com.apple.Preferences", settings}, {"com.apple.mobilesafari", safari}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("runningApps = %v, want %v (no daemon, no app that is not running, no XCTest runner, no system process listapps does not list)", got, want)
	}
	if apps := installedApps(listApps); len(apps) != 5 || !apps["com.apple.Maps"] || apps["group.com.apple.Maps"] {
		t.Errorf("installedApps = %v", apps)
	}
}

func TestFrontApp(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pri     func(pid, call int) (int, bool)
		want    string // the app's id, "" for none
		why     string // a part of why, "" for none
		minWait time.Duration
	}{
		{name: "the app in front", pri: priorities(map[int]int{settings: 53, safari: 4, renderer: 4}), want: "com.apple.Preferences"},
		{name: "Safari's 47 reads as in front", pri: priorities(map[int]int{settings: 4, safari: 47, renderer: 4}), want: "com.apple.mobilesafari"},
		{name: "the home screen: every app in the background", pri: priorities(map[int]int{settings: 4, safari: 4, renderer: 4})},
		{
			// The home screen's page with widgets: its renderer reads 47
			// the whole time, and brought "back" it covers the screen.
			name: "the home screen's widgets: the renderer is no app",
			pri:  priorities(map[int]int{settings: 4, safari: 4, renderer: 47}),
		},
		{name: "a background app that is not throttled is not in front", pri: priorities(map[int]int{settings: 31, safari: 4})},
		{name: "an app that ended is not in front", pri: priorities(map[int]int{safari: 4})},
		{
			name: "two after a switch: the one that left drops",
			pri: func(pid, call int) (int, bool) {
				if pid == settings && call >= 3 {
					return 4, true
				}
				return map[int]int{settings: 53, safari: 47, renderer: 47}[pid], true
			},
			want:    "com.apple.mobilesafari",
			minWait: 2 * 20 * time.Millisecond,
		},
		{
			name:    "two that stay: nothing",
			pri:     priorities(map[int]int{settings: 53, safari: 47}),
			why:     "com.apple.Preferences and com.apple.mobilesafari read as in front for 300ms",
			minWait: 250 * time.Millisecond,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeFront{list: launchdList, apps: listApps, pri: tc.pri}
			f.install(t)
			start := time.Now()
			app, why := frontApp(context.Background(), "SIM-1")
			got := ""
			if app != nil {
				got = app.id
			}
			if got != tc.want || (tc.why == "") != (why == "") || !strings.Contains(why, tc.why) {
				t.Errorf("frontApp = %q, %q; want %q, %q", got, why, tc.want, tc.why)
			}
			if waited := time.Since(start); waited < tc.minWait || waited > 2*time.Second {
				t.Errorf("frontApp took %s, want at least %s", waited, tc.minWait)
			}
		})
	}
}

func TestFrontAppCannotTell(t *testing.T) {
	f := &fakeFront{list: launchdList, apps: listApps, listErr: errors.New("exit status 148"), pri: priorities(nil)}
	f.install(t)
	if app, why := frontApp(context.Background(), "SIM-1"); app != nil || !strings.Contains(why, "launchctl list: exit status 148") {
		t.Errorf("launchctl failing: frontApp = %v, %q", app, why)
	}
	f.listErr, f.appsErr = nil, errors.New("exit status 2")
	if app, why := frontApp(context.Background(), "SIM-1"); app != nil || !strings.Contains(why, "simctl listapps: exit status 2") {
		t.Errorf("listapps failing: frontApp = %v, %q", app, why)
	}
	f.appsErr, f.psErr = nil, errors.New("signal: killed")
	if app, why := frontApp(context.Background(), "SIM-1"); app != nil || !strings.Contains(why, "ps: signal: killed") {
		t.Errorf("ps failing: frontApp = %v, %q", app, why)
	}
	f.psErr = nil
	f.list = "PID\tStatus\tLabel\n" // no app runs: ps is not asked
	if app, why := frontApp(context.Background(), "SIM-1"); app != nil || why != "" || f.psCalls != 1 {
		t.Errorf("no app: frontApp = %v, %q, ps calls %d", app, why, f.psCalls)
	}
}

func TestBringBack(t *testing.T) {
	a := &Agent{opts: AgentOptions{UDID: "SIM-1"}}
	ctx := context.Background()
	log, console := captureFront(t)

	f := &fakeFront{}
	f.install(t)
	a.bringBack(ctx, &runningApp{"com.apple.Preferences", settings}, "")
	if fmt.Sprint(f.launches) != "[launch SIM-1 com.apple.Preferences]" {
		t.Errorf("the app in front: launches = %v, want it launched again (without --terminate-running-process)", f.launches)
	}
	if l := log(); !strings.Contains(l, "brought com.apple.Preferences back to the front") || strings.Contains(l, "anew") || console.Len() != 0 {
		t.Errorf("the app back: log %q, console %q", l, console)
	}

	// An app that ended meanwhile is started again: simctl answers another pid.
	f.launch = func(id string) (string, error) { return id + ": 31337\n", nil }
	a.bringBack(ctx, &runningApp{"com.apple.Preferences", settings}, "")
	if l := log(); !strings.Contains(l, "started anew: com.apple.Preferences: 31337") {
		t.Errorf("an app that ended: log %q", l)
	}

	f.launches = nil
	a.bringBack(ctx, nil, "")
	if len(f.launches) != 0 || !strings.Contains(log(), "the home screen was in front") || console.Len() != 0 {
		t.Errorf("the home screen: launches = %v, console %q", f.launches, console)
	}
	a.bringBack(ctx, nil, "ps: signal: killed")
	if len(f.launches) != 0 {
		t.Errorf("an app not known: launches = %v, want none", f.launches)
	}
	// The console line is a contract: a tool that drives the runner looks
	// for it to tell its user the app was not brought back.
	if want := "  ⚠ the app in front before the agent's start was not brought back: which app was in front is not known: ps: signal: killed\n"; console.String() != want {
		t.Errorf("console = %q, want %q", console, want)
	}

	// A launch that fails is said, not returned: the agent is up.
	console.Reset()
	f.launch = func(string) (string, error) { return "An error was encountered\nmore", errors.New("exit status 4") }
	a.bringBack(ctx, &runningApp{"com.apple.Preferences", settings}, "")
	if want := "  ⚠ the app in front before the agent's start was not brought back: simctl launch com.apple.Preferences: exit status 4: An error was encountered\n"; console.String() != want || len(f.launches) != 1 {
		t.Errorf("a failed launch: console = %q, want %q; launches %v", console, want, f.launches)
	}
	if !strings.Contains(log(), "not brought back: simctl launch com.apple.Preferences: exit status 4") {
		t.Errorf("a failed launch: log %q", log())
	}
}
