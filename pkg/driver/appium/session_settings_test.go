package appium

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// connectCapturingSettings opens a session against a mock server and returns
// the settings the client sent at session start.
func connectCapturingSettings(t *testing.T, platform string) map[string]interface{} {
	t.Helper()
	settings := map[string]interface{}{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch p := r.URL.Path; {
		case strings.HasSuffix(p, "/session") && r.Method == http.MethodPost:
			writeJSON(w, map[string]interface{}{"value": map[string]interface{}{
				"sessionId":    "s1",
				"capabilities": map[string]interface{}{"platformName": platform},
			}})
		case strings.HasSuffix(p, "/appium/settings"):
			var body struct {
				Settings map[string]interface{} `json:"settings"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			for k, v := range body.Settings {
				settings[k] = v
			}
			writeJSON(w, map[string]interface{}{"value": nil})
		case strings.HasSuffix(p, "/window/rect"):
			writeJSON(w, map[string]interface{}{"value": map[string]interface{}{"width": 1080, "height": 2340}})
		default:
			writeJSON(w, map[string]interface{}{"value": nil})
		}
	}))
	t.Cleanup(server.Close)
	c := NewClient(server.URL)
	if err := c.Connect(map[string]interface{}{"platformName": platform}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return settings
}

// Dialogs, permission prompts and popup menus live in their own window; the
// UiAutomator2 server reads only the focused one unless enableMultiWindows is
// set, so they were missing from search and page source (#93).
func TestAndroidSessionSearchesAllWindows(t *testing.T) {
	if got := connectCapturingSettings(t, "Android")["enableMultiWindows"]; got != true {
		t.Errorf("enableMultiWindows = %v, want true", got)
	}
}

// WebDriverAgent's default snapshot depth of 50 clips deep React Native trees
// (#171); the WDA driver raises it to 100 and Appium sessions now match.
func TestIOSSessionRaisesSnapshotDepth(t *testing.T) {
	if got := connectCapturingSettings(t, "iOS")["snapshotMaxDepth"]; got != float64(62) {
		t.Errorf("snapshotMaxDepth = %v, want 62", got)
	}
	t.Setenv("MAESTRO_WDA_SNAPSHOT_MAX_DEPTH", "70")
	if got := connectCapturingSettings(t, "iOS")["snapshotMaxDepth"]; got != float64(70) {
		t.Errorf("snapshotMaxDepth with override = %v, want 70", got)
	}
}
