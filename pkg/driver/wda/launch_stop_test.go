package wda

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// launchWithSession runs launchApp against a WDA that already has a session, the
// usual case for every launchApp after a flow's first, and returns the app
// lifecycle calls it made, in order.
func launchWithSession(t *testing.T, step *flow.LaunchAppStep) []string {
	t.Helper()
	var mu sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/wda/apps/terminate"):
			calls = append(calls, "terminate")
		case strings.HasSuffix(r.URL.Path, "/wda/apps/launch"):
			calls = append(calls, "launch")
		case r.URL.Path == "/session" && r.Method == http.MethodPost:
			calls = append(calls, "new-session")
		}
		jsonResponse(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"}
	driver := &Driver{client: client, info: &core.PlatformInfo{Platform: "ios"}}
	if result := driver.launchApp(step); !result.Success {
		t.Fatalf("launchApp failed: %s", result.Message)
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), calls...)
}

// Maestro stops the app before launching it by default. A relaunch that only
// activated the running app left it where it was, so a flow asserting what a
// restart preserves passed or failed on the pre-restart screen.
func TestLaunchAppStopsARunningAppFirstByDefault(t *testing.T) {
	got := strings.Join(launchWithSession(t, &flow.LaunchAppStep{AppID: "com.test.app"}), ",")
	if got != "terminate,launch" {
		t.Fatalf("lifecycle calls %q, want terminate,launch", got)
	}
}

func TestLaunchAppStopAppTrueStopsFirst(t *testing.T) {
	stop := true
	got := strings.Join(launchWithSession(t, &flow.LaunchAppStep{AppID: "com.test.app", StopApp: &stop}), ",")
	if got != "terminate,launch" {
		t.Fatalf("lifecycle calls %q, want terminate,launch", got)
	}
}

func TestLaunchAppStopAppFalseOnlyBringsTheAppForward(t *testing.T) {
	stop := false
	got := strings.Join(launchWithSession(t, &flow.LaunchAppStep{AppID: "com.test.app", StopApp: &stop}), ",")
	if got != "launch" {
		t.Fatalf("lifecycle calls %q, want launch alone", got)
	}
}

// Arguments need a real launch, so they still stop the app even with stopApp: false.
func TestLaunchAppArgumentsStillStopTheApp(t *testing.T) {
	stop := false
	step := &flow.LaunchAppStep{AppID: "com.test.app", StopApp: &stop, Arguments: map[string]any{"e2e": true}}
	got := strings.Join(launchWithSession(t, step), ",")
	if got != "terminate,launch" {
		t.Fatalf("lifecycle calls %q, want terminate,launch", got)
	}
}

// launchResumingFrom runs launchApp with stopApp: false on an app WDA reports
// in the given state, and returns the lifecycle calls and screenshots in order.
func launchResumingFrom(t *testing.T, state int) []string {
	t.Helper()
	var mu sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/wda/apps/state"):
			calls = append(calls, "state")
			jsonResponse(w, map[string]interface{}{"value": state})
			return
		case strings.HasSuffix(r.URL.Path, "/wda/apps/terminate"):
			calls = append(calls, "terminate")
		case strings.HasSuffix(r.URL.Path, "/wda/apps/launch"):
			calls = append(calls, "launch")
		case strings.HasSuffix(r.URL.Path, "/screenshot"):
			calls = append(calls, "screenshot")
			jsonResponse(w, map[string]interface{}{"value": "c3RpbGw="}) // a still screen
			return
		}
		jsonResponse(w, map[string]interface{}{"value": nil})
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"}
	driver := &Driver{client: client, info: &core.PlatformInfo{Platform: "ios"}}
	stop := false
	if result := driver.launchApp(&flow.LaunchAppStep{AppID: "com.test.app", StopApp: &stop}); !result.Success {
		t.Fatalf("launchApp failed: %s", result.Message)
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), calls...)
}

// Bringing a backgrounded app back waits for the switch to finish on screen:
// a tap sent while SpringBoard was still switching was reported as done and
// never reached the app.
func TestLaunchAppResumeWaitsForTheSwitch(t *testing.T) {
	for _, state := range []int{2, 3} {
		got := strings.Join(launchResumingFrom(t, state), ",")
		if got != "state,launch,screenshot,screenshot" {
			t.Errorf("state %d: calls %q, want state,launch,screenshot,screenshot", state, got)
		}
	}
}

// An app already in front, or not running, is not resumed: no wait.
func TestLaunchAppNoResumeNoWait(t *testing.T) {
	for _, state := range []int{1, 4} {
		got := strings.Join(launchResumingFrom(t, state), ",")
		if got != "state,launch" {
			t.Errorf("state %d: calls %q, want state,launch", state, got)
		}
	}
}
