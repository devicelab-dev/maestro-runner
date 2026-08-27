package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/urfave/cli/v2"
)

// captureAppiumSessionCaps stands up a mock Appium server, runs
// createAppiumDriver with the given config pointed at it, and returns the
// alwaysMatch capabilities block from the outgoing POST /session request.
func captureAppiumSessionCaps(t *testing.T, cfg *RunConfig) map[string]interface{} {
	t.Helper()

	var mu sync.Mutex
	var alwaysMatch map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/session" && r.Method == "POST":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode /session body: %v", err)
			}
			mu.Lock()
			if capsWrap, ok := body["capabilities"].(map[string]interface{}); ok {
				alwaysMatch, _ = capsWrap["alwaysMatch"].(map[string]interface{})
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{
					"sessionId":    "test-session",
					"capabilities": map[string]interface{}{"platformName": "Android"},
				},
			})
		case r.URL.Path == "/session/test-session/window/rect":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]interface{}{"width": 1080.0, "height": 1920.0},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"value": nil})
		}
	}))
	t.Cleanup(server.Close)

	cfg.AppiumURL = server.URL
	_, cleanup, err := createAppiumDriver(cfg)
	if err != nil {
		t.Fatalf("createAppiumDriver: %v", err)
	}
	if cleanup != nil {
		cleanup()
	}

	mu.Lock()
	defer mu.Unlock()
	if alwaysMatch == nil {
		t.Fatal("no alwaysMatch capabilities captured from /session request")
	}
	return alwaysMatch
}

// TestGlobalFlags_NewCommandTimeoutFlag verifies the --new-command-timeout flag
// (with MAESTRO_NEW_COMMAND_TIMEOUT env var) is registered. Issue #124.
func TestGlobalFlags_NewCommandTimeoutFlag(t *testing.T) {
	var found *cli.IntFlag
	for _, f := range GlobalFlags {
		if intFlag, ok := f.(*cli.IntFlag); ok && intFlag.Name == "new-command-timeout" {
			found = intFlag
			break
		}
	}
	if found == nil {
		t.Fatal("expected --new-command-timeout flag to be defined in GlobalFlags")
	}
	hasEnv := false
	for _, e := range found.EnvVars {
		if e == "MAESTRO_NEW_COMMAND_TIMEOUT" {
			hasEnv = true
		}
	}
	if !hasEnv {
		t.Errorf("expected MAESTRO_NEW_COMMAND_TIMEOUT env var on --new-command-timeout, got %v", found.EnvVars)
	}
}

// TestCreateAppiumDriver_NewCommandTimeoutInjectedWhenAbsent verifies that when
// the caps file omits appium:newCommandTimeout, the --new-command-timeout value
// is injected into the outgoing session request. Issue #124 fix (b).
func TestCreateAppiumDriver_NewCommandTimeoutInjectedWhenAbsent(t *testing.T) {
	caps := captureAppiumSessionCaps(t, &RunConfig{
		Platform:          "Android",
		NewCommandTimeout: 300,
		Capabilities: map[string]interface{}{
			"platformName":          "Android",
			"appium:automationName": "UiAutomator2",
		},
	})

	got, ok := caps["appium:newCommandTimeout"]
	if !ok {
		t.Fatalf("expected appium:newCommandTimeout to be injected, got caps: %v", caps)
	}
	if n, _ := got.(float64); n != 300 {
		// json round-trips numbers as float64
		if iv, isInt := got.(int); !isInt || iv != 300 {
			t.Errorf("expected appium:newCommandTimeout=300, got %v (%T)", got, got)
		}
	}
}

// TestCreateAppiumDriver_CapsNewCommandTimeoutPreserved verifies that an
// explicit appium:newCommandTimeout in the caps file is authoritative: it is
// preserved and NOT overridden by the --new-command-timeout flag. This is the
// core scenario from issue #124 — a user who sets 300 gets 300, never a smaller
// forced value.
func TestCreateAppiumDriver_CapsNewCommandTimeoutPreserved(t *testing.T) {
	caps := captureAppiumSessionCaps(t, &RunConfig{
		Platform:          "Android",
		NewCommandTimeout: 90, // flag set to a smaller value must NOT win
		Capabilities: map[string]interface{}{
			"platformName":             "Android",
			"appium:automationName":    "UiAutomator2",
			"appium:newCommandTimeout": float64(300),
		},
	})

	got := caps["appium:newCommandTimeout"]
	n, _ := got.(float64)
	if n != 300 {
		t.Errorf("expected caps-file appium:newCommandTimeout=300 to be preserved, got %v (%T)", got, got)
	}
}

// TestCreateAppiumDriver_NewCommandTimeoutUnsetByDefault verifies the default
// behavior is unchanged: with no flag and no caps value, no newCommandTimeout is
// added to the session request (the Appium server default applies).
func TestCreateAppiumDriver_NewCommandTimeoutUnsetByDefault(t *testing.T) {
	caps := captureAppiumSessionCaps(t, &RunConfig{
		Platform: "Android",
		Capabilities: map[string]interface{}{
			"platformName":          "Android",
			"appium:automationName": "UiAutomator2",
		},
	})

	if v, ok := caps["appium:newCommandTimeout"]; ok {
		t.Errorf("expected no appium:newCommandTimeout by default, got %v", v)
	}
}
