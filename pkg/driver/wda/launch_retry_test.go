package wda

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/logger"
)

// How WDA answers a launch while iOS does not know the app, as real iPhones
// answered right after clearState's reinstall: /wda/apps/launch on iOS 17.7.1
// and 26.2.1 (the same words) and on 18.5, a new session on 26.2.1 (27.0.1
// words it the same).
var appNotFoundAnswers = map[string]struct {
	status int
	body   string
}{
	"launch, iOS 17 and 26":  {http.StatusBadRequest, `{"value":{"error":"invalid argument","message":"Error Domain=FBSOpenApplicationServiceErrorDomain Code=1 \"The request to open \"com.test.app\" failed.\" UserInfo={BSErrorCodeDescription=RequestDenied, NSUnderlyingError=0x1 {Error Domain=FBSOpenApplicationErrorDomain Code=4 \"Application \"com.test.app\" is unknown to FrontBoard.\" UserInfo={BSErrorCodeDescription=NotFound}}}"}}`},
	"launch, iOS 18":         {http.StatusBadRequest, `{"value":{"error":"invalid argument","message":"Error Domain=FBSOpenApplicationServiceErrorDomain Code=1 \"The request to open \"com.test.app\" failed.\" UserInfo={BSErrorCodeDescription=RequestDenied, NSUnderlyingError=0x1 {Error Domain=FBSOpenApplicationErrorDomain Code=4 \"Unknown application display identifier com.test.app\" UserInfo={BSErrorCodeDescription=NotFound}}}"}}`},
	"session, iOS 26 and 27": {http.StatusInternalServerError, `{"value":{"error":"session not created","message":"Error Domain=FBSOpenApplicationServiceErrorDomain Code=4 \"The request to open \"com.test.app\" failed.\" UserInfo={BSErrorCodeDescription=InvalidRequest, NSUnderlyingError=0x1 {Error Domain=FBSOpenApplicationErrorDomain Code=4 \"Application info provider (FBSApplicationLibrary) returned nil for \"com.test.app\"\" UserInfo={BSErrorCodeDescription=NotFound}}}"}}`},
}

func shortLaunchRetries(t *testing.T) {
	t.Helper()
	oldFor, oldEvery := launchRetryFor, launchRetryEvery
	launchRetryFor, launchRetryEvery = 300*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { launchRetryFor, launchRetryEvery = oldFor, oldEvery })
}

func captureLog(t *testing.T) func() string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runner.log")
	if err := logger.Init(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(logger.Close)
	return func() string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
}

// fakeLaunchWDA is a WDA whose launches (the session's and /wda/apps/launch)
// answer notFound for the first refusals tries, then succeed. It counts them.
func fakeLaunchWDA(t *testing.T, refusals int32, status int, notFound string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var launches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		launch := strings.HasSuffix(r.URL.Path, "/wda/apps/launch") || (r.URL.Path == "/session" && r.Method == http.MethodPost)
		if launch && launches.Add(1) <= refusals {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(notFound))
			return
		}
		if r.URL.Path == "/session" {
			jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"sessionId": "new-session"}})
			return
		}
		jsonResponse(w, map[string]interface{}{"value": nil})
	}))
	t.Cleanup(server.Close)
	return server, &launches
}

// Right after clearState's reinstall, iOS may not have registered the app yet,
// and WDA's launch fails with FrontBoard's NotFound. launchApp sends it again
// until iOS has, and logs one line saying so.
func TestLaunchAppWaitsForIOSToRegisterTheApp(t *testing.T) {
	shortLaunchRetries(t)
	for name, answer := range appNotFoundAnswers {
		t.Run(name, func(t *testing.T) {
			readLog := captureLog(t)
			server, launches := fakeLaunchWDA(t, 3, answer.status, answer.body)
			client := &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"}
			driver := &Driver{client: client, info: &core.PlatformInfo{Platform: "ios"}}

			if result := driver.launchApp(&flow.LaunchAppStep{AppID: "com.test.app"}); !result.Success {
				t.Fatalf("launchApp failed: %s", result.Message)
			}
			if got := launches.Load(); got != 4 {
				t.Errorf("sent the launch %d times, want 4 (3 refused, then launched)", got)
			}
			log := readLog()
			if n := strings.Count(log, "iOS had not registered the app yet"); n != 1 {
				t.Errorf("logged the retry %d times, want once:\n%s", n, log)
			}
			if !strings.Contains(log, "launched after 4 tries") {
				t.Errorf("the log line does not say how many tries it took:\n%s", log)
			}
		})
	}
}

// The first launchApp of a run has no session yet: it creates one with the
// app's bundleId, and that session launches the app.
func TestLaunchAppWithoutSessionWaitsForIOSToRegisterTheApp(t *testing.T) {
	shortLaunchRetries(t)
	for name, answer := range appNotFoundAnswers {
		t.Run(name, func(t *testing.T) {
			readLog := captureLog(t)
			server, launches := fakeLaunchWDA(t, 3, answer.status, answer.body)
			client := &Client{baseURL: server.URL, httpClient: http.DefaultClient}
			driver := &Driver{client: client, info: &core.PlatformInfo{Platform: "ios"}}

			if result := driver.launchApp(&flow.LaunchAppStep{AppID: "com.test.app"}); !result.Success {
				t.Fatalf("launchApp failed: %s", result.Message)
			}
			if got := launches.Load(); got != 4 {
				t.Errorf("sent the session's launch %d times, want 4 (3 refused, then launched)", got)
			}
			if client.SessionID() != "new-session" {
				t.Errorf("session %q, want new-session", client.SessionID())
			}
			if n := strings.Count(readLog(), "iOS had not registered the app yet"); n != 1 {
				t.Errorf("logged the retry %d times, want once", n)
			}
		})
	}
}

// An app iOS never registers fails with the last error once launchRetryFor is
// over.
func TestLaunchAppGivesUpOnAnAppIOSDoesNotKnow(t *testing.T) {
	shortLaunchRetries(t)
	readLog := captureLog(t)
	answer := appNotFoundAnswers["launch, iOS 18"]
	server, launches := fakeLaunchWDA(t, 1<<30, answer.status, answer.body)
	client := &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"}

	start := time.Now()
	err := client.LaunchApp("com.test.app")
	took := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "Unknown application display identifier") {
		t.Fatalf("err = %v, want WDA's NotFound", err)
	}
	if took < launchRetryFor || took > launchRetryFor+time.Second {
		t.Errorf("gave up after %s, want about %s", took, launchRetryFor)
	}
	if launches.Load() < 10 {
		t.Errorf("sent the launch %d times in %s, want one every %s", launches.Load(), took, launchRetryEvery)
	}
	if !strings.Contains(readLog(), "iOS still did not know the app after") {
		t.Errorf("no log line for the launch that gave up")
	}
}

// Any other failure is the launch's answer at once.
func TestLaunchAppDoesNotRetryOtherErrors(t *testing.T) {
	shortLaunchRetries(t)
	crashed := fmt.Sprintf(`{"value":{"error":"unknown error","message":%q}}`,
		`The application under test with bundle id 'com.test.app' is not running, possibly crashed`)
	server, launches := fakeLaunchWDA(t, 1<<30, http.StatusInternalServerError, crashed)
	client := &Client{baseURL: server.URL, httpClient: http.DefaultClient, sessionID: "test-session"}

	if err := client.LaunchApp("com.test.app"); err == nil {
		t.Fatal("launch succeeded, want the refusal")
	}
	if got := launches.Load(); got != 1 {
		t.Errorf("sent the launch %d times, want once", got)
	}
}
