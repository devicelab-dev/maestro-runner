package devicelab_ios

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/flow"
)

// eraseServer is a fake runner that captures the one command eraseText sends
// and answers it with the given envelope.
func eraseServer(t *testing.T, reply map[string]any) (*Driver, *map[string]any) {
	t.Helper()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode command: %v", err)
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	client := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	return NewDriver(client, nil, "test-udid", nil), &got
}

// TestHandleEraseText pins the wire contract eraseText depends on: an empty
// `text` in replace mode is the runner's clear-field primitive, so the empty
// string must survive marshalling, and a runner that could not clear the
// field (TEXT_ENTRY_MISMATCH, no input to clear) must fail the step rather
// than report "erased".
func TestHandleEraseText(t *testing.T) {
	okReply := map[string]any{"ok": true, "data": map[string]any{"message": "typed"}}
	mismatch := map[string]any{"ok": false, "error": map[string]any{
		"code":    "TEXT_ENTRY_MISMATCH",
		"message": `text entry verification failed: expected "", observed "abc"`,
	}}

	tests := []struct {
		name        string
		reply       map[string]any
		tapID       string
		tapCoords   bool
		wantSuccess bool
		wantKeys    map[string]any
		absentKeys  []string
	}{
		{
			name:        "clears via last tapped identifier and coords",
			reply:       okReply,
			tapID:       "email",
			tapCoords:   true,
			wantSuccess: true,
			wantKeys: map[string]any{
				"selectorKey": "id", "selectorValue": "email",
				"x": 12.0, "y": 34.0, "appBundleId": "com.example.app",
			},
		},
		{
			name:        "no prior tap sends no target hints",
			reply:       okReply,
			wantSuccess: true,
			absentKeys:  []string{"x", "y", "selectorKey", "selectorValue"},
		},
		{
			name:        "field left non-empty fails the step",
			reply:       mismatch,
			tapID:       "email",
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, got := eraseServer(t, tt.reply)
			d.appID = "com.example.app"
			d.lastTappedIdentifier = tt.tapID
			if tt.tapCoords {
				d.lastTapHasCoords, d.lastTappedX, d.lastTappedY = true, 12, 34
			}

			res := d.handleEraseText(&flow.EraseTextStep{})

			if res.Success != tt.wantSuccess {
				t.Fatalf("success = %v, want %v (err %v)", res.Success, tt.wantSuccess, res.Error)
			}
			if !tt.wantSuccess && !strings.Contains(res.Message, "TEXT_ENTRY_MISMATCH") {
				t.Errorf("expected the runner's mismatch in the message, got %q", res.Message)
			}
			assertEraseBody(t, *got, tt.wantKeys, tt.absentKeys)
		})
	}
}

func assertEraseBody(t *testing.T, got, want map[string]any, absent []string) {
	t.Helper()
	text, present := got["text"]
	if !present || text != "" {
		t.Errorf(`text = %v (present %v), want "" present`, text, present)
	}
	if got["command"] != string(CmdType) || got["textEntryMode"] != "replace" {
		t.Errorf("command/mode = %v/%v, want type/replace", got["command"], got["textEntryMode"])
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	for _, k := range absent {
		if _, ok := got[k]; ok {
			t.Errorf("unexpected key %s = %v", k, got[k])
		}
	}
}
