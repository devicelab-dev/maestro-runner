package devicelab_ios

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestIdleRequestShape: an explicit 0 cap survives omitempty, and the bundle
// id is passed through.
func TestIdleRequestShape(t *testing.T) {
	tests := []struct {
		name      string
		bundle    string
		timeoutMs float64
		wantJSON  string
	}{
		{"zero cap is sent", "", 0, `{"command":"idle","timeoutMs":0}`},
		{"cap and bundle", "com.example", 1500, `{"command":"idle","appBundleId":"com.example","timeoutMs":1500}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				got = string(body)
				_, _ = w.Write([]byte(`{"ok":true,"data":{"idle":false,"waitedMs":0}}`))
			}))
			defer srv.Close()
			c := NewClient("127.0.0.1", srv.Listener.Addr().(*net.TCPAddr).Port)
			if _, err := c.Idle(context.Background(), tt.bundle, tt.timeoutMs); err != nil {
				t.Fatalf("Idle: %v", err)
			}
			if got != tt.wantJSON {
				t.Errorf("request = %s, want %s", got, tt.wantJSON)
			}
		})
	}
}

// TestIdleResultDecoding: every runner answer maps to an honest IdleResult;
// a response without the idle field is an error, never idle.
func TestIdleResultDecoding(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    IdleResult
		wantErr string
	}{
		{
			name: "quiescent",
			body: `{"ok":true,"data":{"message":"quiescent","appState":"runningForeground","idle":true,"waitedMs":42.5}}`,
			want: IdleResult{Idle: true, WaitedMs: 42.5, AppState: "runningForeground", Reason: "quiescent"},
		},
		{
			name: "cap hit",
			body: `{"ok":true,"data":{"message":"not quiescent within timeoutMs","appState":"runningForeground","idle":false,"waitedMs":1003}}`,
			want: IdleResult{WaitedMs: 1003, AppState: "runningForeground", Reason: "not quiescent within timeoutMs"},
		},
		{
			name: "background app",
			body: `{"ok":true,"data":{"message":"not waited: app is not in the foreground","appState":"runningBackground","idle":false,"waitedMs":0}}`,
			want: IdleResult{AppState: "runningBackground", Reason: "not waited: app is not in the foreground"},
		},
		{
			name: "no waitedMs",
			body: `{"ok":true,"data":{"idle":false}}`,
			want: IdleResult{},
		},
		{name: "missing idle field", body: `{"ok":true,"data":{"message":"x"}}`, wantErr: "no idle field"},
		{name: "no data", body: `{"ok":true}`, wantErr: "no idle field"},
		{name: "runner error", body: `{"ok":false,"error":{"code":"NO_TARGET_APP","message":"no app"}}`, wantErr: "NO_TARGET_APP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := replyServer(t, tt.body)
			got, err := c.Idle(context.Background(), "", 1000)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				if got.Idle {
					t.Error("an error must never come with Idle true")
				}
				return
			}
			if err != nil {
				t.Fatalf("Idle: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestIdleIsReadOnly: idle is replayed after a runner relaunch like the other
// observation commands, since it cannot change the device.
func TestIdleIsReadOnly(t *testing.T) {
	if !readOnlyCommands[CmdIdle] {
		t.Error("CmdIdle must be in readOnlyCommands")
	}
	raw, err := json.Marshal(Command{Command: CmdIdle})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"command":"idle"}` {
		t.Errorf("nil TimeoutMs must be omitted, got %s", raw)
	}
}
