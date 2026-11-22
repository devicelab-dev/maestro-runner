package appium

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

func TestIOSLaunchArguments(t *testing.T) {
	args, env := iosLaunchArguments(&flow.LaunchAppStep{
		Arguments:   map[string]any{"e2e-skip-onboarding": "true", "b": false},
		Environment: map[string]string{"FOO": "bar"},
	})
	if want := []string{"-b", "false", "-e2e-skip-onboarding", "true"}; !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}
	if env["FOO"] != "bar" || env["e2e-skip-onboarding"] != "true" {
		t.Errorf("env = %v", env)
	}
}

// recordIOSCalls runs a driver on an iOS session (a real iPhone when real is
// set) and records what it asks the server: "terminate" for terminate_app,
// the script for each mobile: command. Scripts in fail answer with an error.
func recordIOSCalls(t *testing.T, real bool, fail ...string) (*Driver, *[]string) {
	t.Helper()
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		call := ""
		switch {
		case strings.HasSuffix(r.URL.Path, "/terminate_app"):
			call = "terminate"
		case strings.HasSuffix(r.URL.Path, "/execute/sync"):
			var body struct {
				Script string `json:"script"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			call = body.Script
		}
		if call != "" {
			calls = append(calls, call)
		}
		if slices.Contains(fail, call) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"value": map[string]interface{}{"error": "unknown error", "message": "failed"}})
			return
		}
		writeJSON(w, map[string]interface{}{"value": nil})
	}))
	t.Cleanup(server.Close)
	d := createTestAppiumDriver(server)
	d.platform = "ios"
	d.client.platform = "ios"
	d.client.isRealDevice = real
	return d, &calls
}

// With arguments the app is stopped first even with stopApp: false:
// mobile: launchApp on a running app only brings it to the front.
func TestIOSLaunchArgumentsStopTheAppFirst(t *testing.T) {
	d, calls := recordIOSCalls(t, false)
	stop := false
	res := d.launchApp(&flow.LaunchAppStep{AppID: "com.x", StopApp: &stop, Arguments: map[string]any{"e2e": "true"}})
	if !res.Success {
		t.Fatalf("launchApp: %s", res.Message)
	}
	if want := []string{"terminate", "mobile: launchApp"}; !reflect.DeepEqual(*calls, want) {
		t.Errorf("calls = %v, want %v", *calls, want)
	}
}

// clearAppData on a real iPhone: with --app-file the app is removed and
// installed again, and a failed remove still installs; without one only the
// app is stopped, as before.
func TestClearAppDataRealIPhone(t *testing.T) {
	for _, tc := range []struct {
		name    string
		appFile string
		fail    []string
		want    []string
	}{
		{"app file", "/apps/x.ipa", nil, []string{"terminate", "mobile: removeApp", "mobile: installApp"}},
		{"remove fails", "/apps/x.ipa", []string{"mobile: removeApp"}, []string{"terminate", "mobile: removeApp", "mobile: installApp"}},
		{"no app file", "", nil, []string{"terminate"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, calls := recordIOSCalls(t, true, tc.fail...)
			d.SetAppFile(tc.appFile)
			if err := d.clearAppData("com.x"); err != nil {
				t.Fatalf("clearAppData: %v", err)
			}
			if !reflect.DeepEqual(*calls, tc.want) {
				t.Errorf("calls = %v, want %v", *calls, tc.want)
			}
		})
	}
}
