package devicelab_ios

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// replyServer answers every command with body and returns the client that
// talks to it. The server is closed when the test ends.
func replyServer(t *testing.T, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient("127.0.0.1", srv.Listener.Addr().(*net.TCPAddr).Port)
}

func boolValue(b *bool) string {
	if b == nil {
		return "nil"
	}
	if *b {
		return "true"
	}
	return "false"
}

// TestTypeResultDecoding: the type command's read-back outcome reaches the
// caller on success and on a mismatch, and an unreadable field stays nil
// rather than looking verified.
func TestTypeResultDecoding(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantErrCode  string
		wantVerified string
		wantRepaired string
	}{
		{
			name:         "verified first time",
			body:         `{"ok":true,"data":{"message":"typed","verified":true,"repaired":false}}`,
			wantVerified: "true",
			wantRepaired: "false",
		},
		{
			name:         "verified after repair",
			body:         `{"ok":true,"data":{"message":"typed after repair","verified":true,"repaired":true}}`,
			wantVerified: "true",
			wantRepaired: "true",
		},
		{
			name:         "unreadable field",
			body:         `{"ok":true,"data":{"message":"typed","repaired":false}}`,
			wantVerified: "nil",
			wantRepaired: "false",
		},
		{
			name:         "explicit null verified",
			body:         `{"ok":true,"data":{"message":"typed","verified":null,"repaired":false}}`,
			wantVerified: "nil",
			wantRepaired: "false",
		},
		{
			name: "mismatch keeps data with error",
			body: `{"ok":false,"data":{"message":"typed after repair","verified":false,"repaired":true},` +
				`"error":{"code":"TEXT_ENTRY_MISMATCH","message":"text entry verification failed"}}`,
			wantErrCode:  ErrTextEntryMismatch,
			wantVerified: "false",
			wantRepaired: "true",
		},
		{
			name:         "older runner",
			body:         `{"ok":true,"data":{"message":"typed"}}`,
			wantVerified: "nil",
			wantRepaired: "nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := replyServer(t, tt.body)
			data, err := c.Call(context.Background(), Command{Command: CmdType, Text: "x"})
			checkRunnerErrCode(t, err, tt.wantErrCode)
			if data == nil {
				t.Fatal("data is nil")
			}
			if got := boolValue(data.Verified); got != tt.wantVerified {
				t.Errorf("Verified = %s, want %s", got, tt.wantVerified)
			}
			if got := boolValue(data.Repaired); got != tt.wantRepaired {
				t.Errorf("Repaired = %s, want %s", got, tt.wantRepaired)
			}
		})
	}
}

// checkRunnerErrCode fails the test unless err is a RunnerError with code
// want, or nil when want is empty.
func checkRunnerErrCode(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	re, ok := IsRunnerError(err)
	if !ok {
		t.Fatalf("error = %v, want RunnerError %s", err, want)
	}
	if re.Code != want {
		t.Fatalf("error code = %q, want %q", re.Code, want)
	}
}
