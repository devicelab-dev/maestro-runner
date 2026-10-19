package wda

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func shortPolls(t *testing.T) {
	t.Helper()
	old := attachPollInterval
	attachPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { attachPollInterval = old })
}

// A WDA that was just started refuses the first probes; the wait keeps asking until it answers.
func TestWaitForRunningWaitsForAWDAThatIsStillStarting(t *testing.T) {
	shortPolls(t)
	var asked atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("asked %s, not /status", r.URL.Path)
		}
		if asked.Add(1) < 3 {
			http.Error(w, `{"value":{"error":"starting"}}`, http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"value":{"ready":true,"state":"success"},"sessionId":null}`))
	}))
	defer server.Close()
	c := NewClient(8100)
	c.baseURL = server.URL

	if err := WaitForRunning(c, 5*time.Second); err != nil {
		t.Fatalf("WaitForRunning: %v", err)
	}
	if n := asked.Load(); n != 3 {
		t.Fatalf("asked /status %d times, want 3 (two refusals, then the answer)", n)
	}
}

// Nothing listening: the wait gives up after its timeout and says where it looked.
func TestWaitForRunningGivesUpWhenNoWDAAnswers(t *testing.T) {
	shortPolls(t)
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close() // the port is now closed

	c := NewClient(8100)
	c.baseURL = url
	start := time.Now()
	err := WaitForRunning(c, 200*time.Millisecond)
	if err == nil {
		t.Fatal("WaitForRunning passed with nothing listening")
	}
	if !strings.Contains(err.Error(), url) {
		t.Fatalf("the error does not name where it looked (%s): %v", url, err)
	}
	if took := time.Since(start); took < 200*time.Millisecond || took > 3*time.Second {
		t.Fatalf("gave up after %v, want about the 200ms timeout", took)
	}
}

// A WDA that takes the connection and never answers cannot hold the wait past
// its timeout: each probe has its own short limit.
func TestWaitForRunningBoundsAProbeThatNeverAnswers(t *testing.T) {
	shortPolls(t)
	old := attachProbeTimeout
	attachProbeTimeout = 50 * time.Millisecond
	t.Cleanup(func() { attachProbeTimeout = old })
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)
	c := NewClient(8100)
	c.baseURL = server.URL

	start := time.Now()
	if err := WaitForRunning(c, 300*time.Millisecond); err == nil {
		t.Fatal("WaitForRunning passed with a WDA that never answers")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("gave up after %v, want about the 300ms timeout", took)
	}
}

func TestNewClientURL(t *testing.T) {
	for raw, want := range map[string]string{
		"http://127.0.0.1:8100":   "http://127.0.0.1:8100",
		"http://127.0.0.1:8100/":  "http://127.0.0.1:8100",
		" http://10.0.0.5:8200 ":  "http://10.0.0.5:8200",
		"https://wda.example:443": "https://wda.example:443",
		"http://[::1]:8100":       "http://[::1]:8100",
	} {
		c, err := NewClientURL(raw)
		if err != nil {
			t.Errorf("NewClientURL(%q): %v", raw, err)
			continue
		}
		if c.baseURL != want {
			t.Errorf("NewClientURL(%q) base = %q, want %q", raw, c.baseURL, want)
		}
	}
	for _, raw := range []string{"", "127.0.0.1:8100", "ftp://127.0.0.1:8100", "http://", "http://127.0.0.1:8100/?x=1",
		"http://127.0.0.1:8100?", "http://:8100", "http://127.0.0.1:99999", "http://127.0.0.1:0", "http://user:pw@127.0.0.1:8100",
		"http://127.0.0.1:8100/wd/hub", "http://127.0.0.1:8100#x"} {
		if _, err := NewClientURL(raw); err == nil {
			t.Errorf("NewClientURL(%q) accepted a bad URL", raw)
		}
	}
}

// Ending the session on a WDA that never answers gives up at its limit.
func TestDeleteSessionWithinGivesUp(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)
	c, err := NewClientURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.sessionID = "s1"

	start := time.Now()
	if err := c.DeleteSessionWithin(100 * time.Millisecond); err == nil {
		t.Fatal("DeleteSessionWithin passed with a WDA that never answers")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("gave up after %v, want about 100ms", took)
	}
	if c.HasSession() {
		t.Error("the session is still recorded after ending it")
	}
}
