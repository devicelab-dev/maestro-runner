package wda

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// acceptAlert and dismissAlert get button selectors whatever launchApp's
// permissions resolve to: with none, WDA's positional pick dismissed a
// stacked alert with its first button ("Allow Once").
func TestLaunchAppSetsAlertButtonSelectorsForEveryAction(t *testing.T) {
	for _, perms := range []map[string]string{
		nil,                                     // all: allow
		{"all": "deny"},                         // dismiss
		{"camera": "allow", "location": "deny"}, // mixed: no auto action
	} {
		var settings map[string]interface{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "POST" && r.URL.Path == "/session" {
				jsonResponse(w, map[string]interface{}{"value": map[string]interface{}{"sessionId": "s"}})
				return
			}
			if strings.HasSuffix(r.URL.Path, "/appium/settings") {
				body, _ := io.ReadAll(r.Body)
				var req struct {
					Settings map[string]interface{} `json:"settings"`
				}
				_ = json.Unmarshal(body, &req)
				if _, ok := req.Settings["defaultAlertAction"]; ok {
					settings = req.Settings
				}
			}
			jsonResponse(w, map[string]interface{}{"status": 0})
		}))
		driver := &Driver{
			client: &Client{baseURL: server.URL, httpClient: http.DefaultClient},
			info:   &core.PlatformInfo{Platform: "ios", IsSimulator: false},
			udid:   "REAL-DEVICE-UDID",
		}
		if res := driver.launchApp(&flow.LaunchAppStep{AppID: "com.test.app", Permissions: perms}); !res.Success {
			t.Fatalf("%v: %s", perms, res.Message)
		}
		server.Close()
		if settings["acceptAlertButtonSelector"] != acceptAlertButtonSelector ||
			settings["dismissAlertButtonSelector"] != dismissAlertButtonSelector {
			t.Fatalf("%v: settings = %v", perms, settings)
		}
	}
}

// A quote inside a predicate string ends it: the old "Don't Allow" selector
// never matched, and WDA fell back to its positional pick.
func TestAlertButtonSelectorsHaveNoStrayQuote(t *testing.T) {
	for _, sel := range []string{acceptAlertButtonSelector, dismissAlertButtonSelector, autoAcceptAlertSelector, autoDismissAlertSelector} {
		start, end := strings.Index(sel, "`"), strings.LastIndex(sel, "`")
		if start < 0 || end <= start {
			t.Fatalf("no predicate in %q", sel)
		}
		if n := strings.Count(sel[start+1:end], "'"); n%2 != 0 {
			t.Errorf("odd number of quotes in %q", sel)
		}
	}
	for _, want := range []string{"'Don'", "'Ask App Not'", "'Cancel'", "'Not Now'"} {
		if !strings.Contains(dismissAlertButtonSelector, want) {
			t.Errorf("dismiss selector lacks %s", want)
		}
	}
}
